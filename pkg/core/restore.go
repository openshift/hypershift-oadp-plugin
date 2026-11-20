package core

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path"
	"slices"
	"strconv"
	"strings"
	"time"

	hive "github.com/openshift/hive/apis/hive/v1"
	"github.com/openshift/hypershift-oadp-plugin/pkg/azblobsas"
	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
	"github.com/openshift/hypershift-oadp-plugin/pkg/s3presign"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// RestorePlugin is a plugin to restore hypershift resources.
type RestorePlugin struct {
	log logrus.FieldLogger
	ctx context.Context

	client    crclient.Client
	config    map[string]string
	validator validation.RestoreValidator
	fsBackup  bool
	hasDPA    bool // true when OADP+DPA is detected, false for standalone Velero

	newTokenProvider func(creds *azblobsas.AADCredentials) (azblobsas.TokenProvider, error)
	newSTSClient     func() s3presign.STSAssumeRoler

	*plugtypes.RestoreOptions
}

type RestoreOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	readoptNodes bool
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	managedServices bool
	// AWSRegenPrivateLink is a flag to indicate if the PrivateLink should be regenerated in AWS.
	awsRegenPrivateLink bool
}

// NewRestorePlugin instantiates RestorePlugin.
func NewRestorePlugin(logger logrus.FieldLogger) (*RestorePlugin, error) {
	var (
		err error
	)

	logger = logger.WithFields(logrus.Fields{
		"process": "restore",
	})

	logger.Info("Initializing HCP Restore Plugin")
	client, err := common.GetClient()
	if err != nil {
		return nil, fmt.Errorf("error recovering the k8s client: %s", err.Error())
	}
	logger.Debug("client recovered")

	pluginConfig := corev1.ConfigMap{}
	ns, err := common.GetCurrentNamespace()
	if err != nil {
		return nil, fmt.Errorf("error getting current namespace: %s", err.Error())
	}

	ctx := context.Background()

	err = client.Get(ctx, types.NamespacedName{Name: common.PluginConfigMapName, Namespace: ns}, &pluginConfig)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("error getting plugin configuration: %s", err.Error())
		}
		logger.Info("configuration for hypershift OADP plugin not found")
	}

	hasDPA, dpaErr := common.CRDExists(ctx, common.DPACRDName, client)
	if dpaErr != nil {
		logger.Warnf("Could not check for DPA CRD: %v", dpaErr)
	}
	if hasDPA {
		logger.Info("OADP+DPA detected, using BSL credential references")
	} else {
		logger.Info("Standalone Velero detected, will use fallback credentials when BSL has no credential reference")
	}

	validator := &validation.RestorePluginValidator{
		Log:       logger,
		Client:    client,
		LogHeader: "restore",
	}

	rp := &RestorePlugin{
		log:              logger,
		ctx:              ctx,
		client:           client,
		fsBackup:         false,
		hasDPA:           hasDPA,
		config:           pluginConfig.Data,
		validator:        validator,
		newTokenProvider: azblobsas.NewAADTokenProvider,
		newSTSClient:     func() s3presign.STSAssumeRoler { return s3presign.NewSTSClient() },
	}

	if rp.RestoreOptions, err = rp.validator.ValidatePluginConfig(rp.config); err != nil {
		return nil, fmt.Errorf("error validating plugin configuration: %s", err.Error())
	}

	rp.log = logger.WithField("type", "hcp-restore")

	return rp, nil
}

func (p *RestorePlugin) Name() string {
	return "HCPRestorePlugin"
}

func (p *RestorePlugin) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{
		IncludedResources: slices.Concat(
			plugtypes.BackupCommonResources,
			plugtypes.BackupAWSResources,
			plugtypes.BackupAzureResources,
			plugtypes.BackupIBMPowerVSResources,
			plugtypes.BackupOpenStackResources,
			plugtypes.BackupKubevirtResources,
			plugtypes.BackupAgentResources,
		),
	}, nil
}

func (p *RestorePlugin) Execute(input *velero.RestoreItemActionExecuteInput) (*velero.RestoreItemActionExecuteOutput, error) {
	p.log.Debugf("Entering Hypershift restore plugin")
	ctx := context.Context(p.ctx)

	// get the backup associated with the restore
	backup := new(velerov1api.Backup)
	err := p.client.Get(
		ctx,
		types.NamespacedName{
			Namespace: input.Restore.Namespace,
			Name:      input.Restore.Spec.BackupName,
		},
		backup,
	)

	if err != nil {
		p.log.Error("Fail to get backup for restore.")
		return nil, fmt.Errorf("fail to get backup for restore: %s", err.Error())
	}

	// if the backup is not a hypershift backup, return early
	if returnEarly, err := common.ShouldEndPluginExecution(backup); returnEarly {
		p.log.Infof("Skipping hypershift plugin execution - not a hypershift backup: %v", err)
		return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
	}

	kind := input.Item.GetObjectKind().GroupVersionKind().Kind
	switch {
	case kind == common.HostedControlPlaneKind || kind == common.HostedClusterKind:
		if kind == common.HostedControlPlaneKind {
			hcp := &hyperv1.HostedControlPlane{}
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), hcp); err != nil {
				return nil, fmt.Errorf("error converting item to HostedControlPlane: %w", err)
			}
			if err := p.validator.ValidatePlatformConfig(hcp, p.config); err != nil {
				return nil, fmt.Errorf("error checking platform configuration: %w", err)
			}
		}
		metadata, err := meta.Accessor(input.Item)
		if err != nil {
			return nil, fmt.Errorf("error getting metadata accessor: %w", err)
		}
		if kind == common.HostedClusterKind {
			common.AddAnnotation(metadata, common.HostedClusterRestoredFromBackupAnnotation, "")
			p.log.Infof("Added restore annotation to HostedCluster %s", metadata.GetName())
		}

		content := input.Item.UnstructuredContent()
		managed, _, err := unstructured.NestedFieldNoCopy(content, "spec", "etcd", "managed")
		if err != nil {
			return nil, fmt.Errorf("error reading managed etcd configuration: %w", err)
		}
		// Velero strips status. The annotation and the immutable spec field may
		// have survived an earlier restore. Do not carry either into a PVC restore.
		snapshotURL := metadata.GetAnnotations()[common.EtcdSnapshotURLAnnotation]
		if managed == nil || !isSnapshotForBackup(snapshotURL, backup) {
			// The logical snapshot workflow explicitly disables volume backup and restore.
			if managed != nil && backup.Spec.SnapshotVolumes != nil && !*backup.Spec.SnapshotVolumes &&
				backup.Spec.DefaultVolumesToFsBackup != nil && !*backup.Spec.DefaultVolumesToFsBackup &&
				input.Restore.Spec.RestorePVs != nil && !*input.Restore.Spec.RestorePVs {
				return nil, fmt.Errorf("no valid etcd snapshot URL for Velero backup %q with volume backup and restore disabled", backup.Name)
			}
			common.RemoveAnnotation(metadata, common.EtcdSnapshotURLAnnotation)
			content = input.Item.UnstructuredContent()
			unstructured.RemoveNestedField(content, "spec", "etcd", "managed", "storage", "restoreSnapshotURL")
			input.Item.SetUnstructuredContent(content)
			break
		}
		signedURL, err := p.signSnapshotURL(ctx, backup, snapshotURL, metadata.GetName())
		if err != nil {
			return nil, err
		}
		if err := unstructured.SetNestedStringSlice(content, []string{signedURL}, "spec", "etcd", "managed", "storage", "restoreSnapshotURL"); err != nil {
			return nil, fmt.Errorf("error setting restoreSnapshotURL: %w", err)
		}
		input.Item.SetUnstructuredContent(content)
		p.log.Infof("Injected restoreSnapshotURL into %s %s", kind, metadata.GetName())

	case kind == "Pod":
		p.log.Debugf("Pod found, skipping restore")
		return velero.NewRestoreItemActionExecuteOutput(input.Item).WithoutRestore(), nil

	case kind == "StatefulSet":
		metadata, err := meta.Accessor(input.Item)
		if err != nil {
			return nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		if metadata.GetName() == "etcd" && p.config[common.ConfigKeyEtcdBackupMethod] == common.EtcdBackupMethodEtcdSnapshot {
			p.log.Infof("etcd StatefulSet found, skipping restore (using etcdSnapshot method)")
			return velero.NewRestoreItemActionExecuteOutput(input.Item).WithoutRestore(), nil
		}

	case kind == "Secret":
		metadata, err := meta.Accessor(input.Item)
		if err != nil {
			return nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		if strings.HasSuffix(metadata.GetName(), common.ImportSecretSuffix) {
			p.log.Infof("Skipping restore of ACM/MCE import secret %s/%s (contains stale bootstrap token)", metadata.GetNamespace(), metadata.GetName())
			return velero.NewRestoreItemActionExecuteOutput(input.Item).WithoutRestore(), nil
		}

	case kind == common.ClusterDeploymentKind:
		clusterdDeployment := &hive.ClusterDeployment{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), clusterdDeployment); err != nil {
			return nil, fmt.Errorf("error converting item to clusterdDeployment: %v", err)
		}

		clusterDeploymentCP := clusterdDeployment.DeepCopy()
		clusterDeploymentCP.Spec.PreserveOnDelete = true

		if err := p.client.Update(ctx, clusterDeploymentCP); err != nil {
			return nil, fmt.Errorf("error updating ClusterDeployment resource with PreserveOnDelete option: %w", err)
		}

	}

	return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
}

func isSnapshotForBackup(rawURL string, backup *velerov1api.Backup) bool {
	if rawURL == "" {
		return false
	}
	u, err := url.Parse(rawURL)
	if err != nil || u.Host == "" {
		return false
	}
	// The backup orchestrator stores files directly under backups/<name>/etcd-backup/.
	// The storage location may add a prefix, and Azure URLs also include the container.
	dir, file := path.Split(u.Path)
	if file == "" || !strings.HasSuffix(dir, "/backups/"+backup.Name+"/etcd-backup/") {
		return false
	}
	// etcd-upload names snapshots <unix-seconds>.db. A reused backup name is not
	// enough to match if the file's timestamp predates this Velero backup.
	if start := backup.Status.StartTimestamp; start != nil {
		if stamp, ok := strings.CutSuffix(file, ".db"); ok {
			if uploaded, err := strconv.ParseInt(stamp, 10, 64); err == nil && uploaded < start.Unix() {
				return false
			}
		}
	}
	return true
}

// signSnapshotURL converts a raw snapshot URL (s3:// or Azure Blob https://) into a
// signed HTTPS URL. If the URL is already an HTTPS URL that is not Azure Blob, it is
// returned as-is.
func (p *RestorePlugin) signSnapshotURL(ctx context.Context, backup *velerov1api.Backup, rawURL, resourceName string) (string, error) {
	if strings.HasPrefix(rawURL, "s3://") {
		signed, err := p.presignS3URL(ctx, backup, rawURL, resourceName)
		if err != nil {
			return "", fmt.Errorf("error generating pre-signed URL for etcd snapshot: %w", err)
		}
		p.log.Infof("Converted s3:// URL to pre-signed HTTPS URL for %s restore", resourceName)
		return signed, nil
	}
	if azblobsas.IsAzBlobURL(rawURL) {
		signed, err := p.presignAzBlobUrl(ctx, backup, rawURL)
		if err != nil {
			return "", fmt.Errorf("error generating SAS URL for etcd snapshot: %w", err)
		}
		p.log.Infof("Generated SAS URL for Azure Blob snapshot for %s restore", resourceName)
		return signed, nil
	}
	return rawURL, nil
}

// fetchBSLCredentials fetches the BackupStorageLocation and its credential secret data.
func (p *RestorePlugin) fetchBSLCredentials(ctx context.Context, backup *velerov1api.Backup) (
	bsl *velerov1api.BackupStorageLocation, credData []byte, oadpNS string, err error,
) {
	oadpNS, err = common.GetCurrentNamespace()
	if err != nil {
		return nil, nil, "", fmt.Errorf("error getting current namespace: %w", err)
	}

	bsl = &velerov1api.BackupStorageLocation{}
	bslName := backup.Spec.StorageLocation
	if err := p.client.Get(ctx, types.NamespacedName{Name: bslName, Namespace: oadpNS}, bsl); err != nil {
		return nil, nil, "", fmt.Errorf("error getting BackupStorageLocation %q: %w", bslName, err)
	}

	credRef := common.ResolveCredentialRef(bsl)
	if bsl.Spec.Credential == nil {
		p.log.Infof("BSL %q has no credential reference, using fallback %s/%s (key: %s)", bsl.Name, oadpNS, credRef.Name, credRef.Key)
	}

	secret := &corev1.Secret{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: credRef.Name, Namespace: oadpNS}, secret); err != nil {
		return bsl, nil, oadpNS, fmt.Errorf("error getting credential secret %q: %w", credRef.Name, err)
	}

	credData, ok := secret.Data[credRef.Key]
	if !ok {
		return bsl, nil, oadpNS, fmt.Errorf("key %q not found in secret %q", credRef.Key, credRef.Name)
	}

	return bsl, credData, oadpNS, nil
}

// presignAzBlobUrl converts an Azure Blob Storage URL into a SAS-signed URL
// using credentials from the Velero BackupStorageLocation.
func (p *RestorePlugin) presignAzBlobUrl(ctx context.Context, backup *velerov1api.Backup, azURL string) (string, error) {
	account, container, blob, err := azblobsas.ParseAzBlobURL(azURL)
	if err != nil {
		return "", fmt.Errorf("error parsing Azure Blob URL %q: %w", azURL, err)
	}

	bsl, credData, _, err := p.fetchBSLCredentials(ctx, backup)
	if err != nil {
		if bsl == nil {
			return "", err
		}
		// fetchBSLCredentials failed at the secret level; for Azure WI without
		// a credential secret this is expected when useAAD=true and BSL has no ref.
		if bsl.Spec.Credential == nil && bsl.Spec.Config["useAAD"] == "true" {
			p.log.Infof("BSL %q has no credential reference, using Azure Workload Identity from pod environment", bsl.Name)
			return p.presignAzBlobUrlWithAAD(ctx, account, container, blob, nil, bsl.Spec.Config["storageAccountURI"])
		}
		return "", err
	}

	endpoint := bsl.Spec.Config["storageAccountURI"]

	if bsl.Spec.Config["useAAD"] == "true" {
		return p.presignAzBlobUrlWithAAD(ctx, account, container, blob, credData, endpoint)
	}

	keyEnvVar := bsl.Spec.Config["storageAccountKeyEnvVar"]
	creds, err := azblobsas.ParseAzureCredentials(credData, keyEnvVar)
	if err != nil {
		return "", fmt.Errorf("error parsing Azure credentials: %w", err)
	}

	return azblobsas.GenerateSASBlobURL(azblobsas.SASOptions{
		Account:    account,
		Container:  container,
		Blob:       blob,
		AccountKey: creds.StorageAccountAccessKey,
		Expiry:     azblobsas.DefaultSASExpiry,
		Endpoint:   endpoint,
	})
}

func (p *RestorePlugin) presignAzBlobUrlWithAAD(ctx context.Context, account, container, blob string, credData []byte, endpoint string) (string, error) {
	var aadCreds *azblobsas.AADCredentials
	var err error
	if credData != nil {
		aadCreds, err = azblobsas.ParseAzureAADCredentials(credData)
		if err != nil {
			return "", fmt.Errorf("error parsing Azure AAD credentials: %w", err)
		}
	} else {
		aadCreds = &azblobsas.AADCredentials{
			TenantID:  os.Getenv("AZURE_TENANT_ID"),
			ClientID:  os.Getenv("AZURE_CLIENT_ID"),
			CloudName: os.Getenv("AZURE_CLOUD_NAME"),
		}
		if aadCreds.TenantID == "" || aadCreds.ClientID == "" {
			return "", fmt.Errorf("AZURE_TENANT_ID and AZURE_CLIENT_ID env vars are required when no credential data is provided")
		}
	}

	tokenProvider, err := p.newTokenProvider(aadCreds)
	if err != nil {
		return "", fmt.Errorf("error creating AAD token provider: %w", err)
	}

	scope := azblobsas.StorageScopeForCloud(aadCreds.CloudName)
	token, err := tokenProvider.GetToken(ctx, scope)
	if err != nil {
		return "", fmt.Errorf("error acquiring AAD token for storage: %w", err)
	}

	now := time.Now().UTC()
	delegationKey, err := azblobsas.GetUserDelegationKey(ctx, account, token, now, now.Add(azblobsas.DefaultSASExpiry), endpoint)
	if err != nil {
		return "", fmt.Errorf("error getting user delegation key: %w", err)
	}

	return azblobsas.GenerateUserDelegationSASBlobURL(azblobsas.UserDelegationSASOptions{
		Account:       account,
		Container:     container,
		Blob:          blob,
		DelegationKey: delegationKey,
		Expiry:        azblobsas.DefaultSASExpiry,
		Endpoint:      endpoint,
	})
}

// presignS3URL converts an s3:// URL into a pre-signed HTTPS GET URL using
// credentials from the Velero BackupStorageLocation.
func (p *RestorePlugin) presignS3URL(ctx context.Context, backup *velerov1api.Backup, s3URL, hcName string) (string, error) {
	bucket, key, err := s3presign.ParseS3URL(s3URL)
	if err != nil {
		return "", fmt.Errorf("error parsing S3 URL %q: %w", s3URL, err)
	}

	bsl, credData, _, err := p.fetchBSLCredentials(ctx, backup)
	if err != nil {
		return "", err
	}

	region := bsl.Spec.Config["region"]
	endpoint := bsl.Spec.Config["s3Url"]
	forcePathStyle := bsl.Spec.Config["s3ForcePathStyle"] == "true"

	parsed, err := s3presign.ParseAWSCredentialData(credData, "default")
	if err != nil {
		return "", fmt.Errorf("error parsing AWS credentials: %w", err)
	}

	var creds *s3presign.AWSCredentials
	switch parsed.Type {
	case s3presign.STSRoleCredentialType:
		stsClient := p.newSTSClient()
		sessionName := fmt.Sprintf("restore-%s", hcName)
		creds, err = stsClient.AssumeRoleWithWebIdentity(
			ctx,
			parsed.STSRole.RoleARN,
			parsed.STSRole.WebIdentityTokenFile,
			sessionName,
		)
		if err != nil {
			return "", fmt.Errorf("error assuming role via STS: %w", err)
		}
		p.log.Infof("Assumed role %s via STS for pre-signing", parsed.STSRole.RoleARN)
	case s3presign.StaticCredentialType:
		creds = parsed.Static
	default:
		return "", fmt.Errorf("unsupported credential type %q", parsed.Type)
	}

	return s3presign.GeneratePresignedGetURL(s3presign.PresignOptions{
		Bucket:          bucket,
		Key:             key,
		Region:          region,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		Expiry:          s3presign.DefaultPresignExpiry,
		Endpoint:        endpoint,
		ForcePathStyle:  forcePathStyle,
	})
}
