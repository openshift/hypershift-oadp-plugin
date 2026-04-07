package etcdbackup

import (
	"context"
	"fmt"
	"time"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	utilrand "k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/apimachinery/pkg/util/wait"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	verifyTimeout     = 30 * time.Second
	completionTimeout = 10 * time.Minute
	pollInterval      = 5 * time.Second
)

// Orchestrator manages the lifecycle of HCPEtcdBackup CRs during OADP backup.
type Orchestrator struct {
	log    logrus.FieldLogger
	client crclient.Client

	// State tracked across Execute() calls
	BackupName      string
	BackupNamespace string
	HONamespace     string
	OADPNamespace   string
	CredSecretName  string
}

// NewOrchestrator creates a new Orchestrator.
func NewOrchestrator(log logrus.FieldLogger, client crclient.Client, hoNamespace, oadpNamespace string) *Orchestrator {
	return &Orchestrator{
		log:           log.WithField("component", "etcdbackup-orchestrator"),
		client:        client,
		HONamespace:   hoNamespace,
		OADPNamespace: oadpNamespace,
	}
}

// IsCreated returns true if an HCPEtcdBackup CR was created.
func (o *Orchestrator) IsCreated() bool {
	return o.BackupName != ""
}

// CreateEtcdBackup creates an HCPEtcdBackup CR in the HCP namespace.
// It fetches the BSL, copies credentials, and maps storage config.
func (o *Orchestrator) CreateEtcdBackup(ctx context.Context, backup *velerov1.Backup, hcpNamespace string, hc *hyperv1.HostedCluster) error {
	bsl, err := o.fetchBSL(ctx, backup.Spec.StorageLocation, o.OADPNamespace)
	if err != nil {
		return fmt.Errorf("failed to fetch BackupStorageLocation %q: %w", backup.Spec.StorageLocation, err)
	}

	storage, err := o.mapBSLToStorage(bsl, backup.Name)
	if err != nil {
		return fmt.Errorf("failed to map BSL to HCPEtcdBackup storage: %w", err)
	}

	credSecretName, err := o.copyCredentialSecret(ctx, bsl, o.OADPNamespace, o.HONamespace, backup.Name)
	if err != nil {
		return fmt.Errorf("failed to copy credential Secret: %w", err)
	}
	o.CredSecretName = credSecretName

	// Set credential reference on the storage config
	setCredentialRef(storage, credSecretName)

	// Set encryption fields from HostedCluster if available
	if hc != nil {
		setEncryptionFields(storage, hc)
	}

	crName := fmt.Sprintf("oadp-%s-%s", backup.Name, utilrand.String(4))
	etcdBackup := &hyperv1.HCPEtcdBackup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      crName,
			Namespace: hcpNamespace,
		},
		Spec: hyperv1.HCPEtcdBackupSpec{
			Storage: *storage,
		},
	}

	if err := o.client.Create(ctx, etcdBackup); err != nil {
		if apierrors.IsAlreadyExists(err) {
			o.log.Infof("HCPEtcdBackup %s/%s already exists, reusing", hcpNamespace, crName)
		} else {
			return fmt.Errorf("failed to create HCPEtcdBackup: %w", err)
		}
	} else {
		o.log.Infof("Created HCPEtcdBackup %s/%s", hcpNamespace, crName)
	}

	o.BackupName = crName
	o.BackupNamespace = hcpNamespace
	return nil
}

// VerifyInProgress polls the HCPEtcdBackup until the controller acknowledges it.
func (o *Orchestrator) VerifyInProgress(ctx context.Context) error {
	return o.pollCondition(ctx, verifyTimeout, func(cond *metav1.Condition) (bool, error) {
		if cond == nil {
			return false, nil // no condition yet, keep polling
		}
		switch cond.Reason {
		case common.BackupInProgressReason:
			o.log.Info("HCPEtcdBackup is in progress")
			return true, nil
		case hyperv1.BackupSucceededReason:
			o.log.Info("HCPEtcdBackup already succeeded")
			return true, nil
		case hyperv1.BackupFailedReason:
			return false, fmt.Errorf("HCPEtcdBackup failed: %s", cond.Message)
		case common.BackupRejectedReason:
			return false, fmt.Errorf("HCPEtcdBackup rejected: %s", cond.Message)
		case hyperv1.EtcdUnhealthyReason:
			return false, fmt.Errorf("etcd unhealthy: %s", cond.Message)
		}
		return false, nil
	})
}

// WaitForCompletion polls the HCPEtcdBackup until it reaches a terminal state.
// Returns the snapshotURL on success.
func (o *Orchestrator) WaitForCompletion(ctx context.Context) (string, error) {
	var snapshotURL string

	err := o.pollCondition(ctx, completionTimeout, func(cond *metav1.Condition) (bool, error) {
		if cond == nil {
			return false, nil
		}

		// Success: status=True
		if cond.Status == metav1.ConditionTrue && cond.Reason == hyperv1.BackupSucceededReason {
			// Re-fetch to get snapshotURL
			eb := &hyperv1.HCPEtcdBackup{}
			if err := o.client.Get(ctx, types.NamespacedName{Name: o.BackupName, Namespace: o.BackupNamespace}, eb); err != nil {
				return false, fmt.Errorf("failed to get HCPEtcdBackup for snapshotURL: %w", err)
			}
			snapshotURL = eb.Status.SnapshotURL
			o.log.Infof("HCPEtcdBackup completed successfully, snapshotURL: %s", snapshotURL)
			return true, nil
		}

		// Terminal failures
		switch cond.Reason {
		case hyperv1.BackupFailedReason:
			return false, fmt.Errorf("HCPEtcdBackup failed: %s", cond.Message)
		case common.BackupRejectedReason:
			return false, fmt.Errorf("HCPEtcdBackup rejected: %s", cond.Message)
		}

		// Still in progress
		return false, nil
	})

	if err != nil {
		return "", err
	}
	return snapshotURL, nil
}

// CleanupCredentialSecret removes the copied credential Secret from the HO namespace.
func (o *Orchestrator) CleanupCredentialSecret(ctx context.Context) error {
	if o.CredSecretName == "" {
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      o.CredSecretName,
			Namespace: o.HONamespace,
		},
	}
	if err := o.client.Delete(ctx, secret); err != nil && !apierrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete credential Secret %s/%s: %w", o.HONamespace, o.CredSecretName, err)
	}
	o.log.Infof("Cleaned up credential Secret %s/%s", o.HONamespace, o.CredSecretName)
	return nil
}

// fetchBSL retrieves the BackupStorageLocation from the OADP namespace.
func (o *Orchestrator) fetchBSL(ctx context.Context, bslName, namespace string) (*velerov1.BackupStorageLocation, error) {
	bsl := &velerov1.BackupStorageLocation{}
	if err := o.client.Get(ctx, types.NamespacedName{Name: bslName, Namespace: namespace}, bsl); err != nil {
		return nil, err
	}
	return bsl, nil
}

// mapBSLToStorage translates a Velero BackupStorageLocation into hyperv1.HCPEtcdBackupStorage
// so the etcd backup controller can use the same object store as OADP/Velero.
//
// The KeyPrefix follows Velero's backup directory layout:
//   {bsl-prefix}/backups/{backup-name}/etcd-backup
// so the etcd snapshot is stored alongside the rest of the backup data.
func (o *Orchestrator) mapBSLToStorage(bsl *velerov1.BackupStorageLocation, backupName string) (*hyperv1.HCPEtcdBackupStorage, error) {
	keyPrefix := fmt.Sprintf("backups/%s/etcd-backup", backupName)
	if bsl.Spec.ObjectStorage != nil && bsl.Spec.ObjectStorage.Prefix != "" {
		keyPrefix = fmt.Sprintf("%s/backups/%s/etcd-backup", bsl.Spec.ObjectStorage.Prefix, backupName)
	}

	switch bsl.Spec.Provider {
	case "aws", "velero.io/aws":
		return mapAWSBSLToStorage(bsl, keyPrefix), nil
	case "azure", "velero.io/azure":
		return mapAzureBSLToStorage(bsl, keyPrefix), nil
	}

	return nil, fmt.Errorf("unsupported BSL provider %q: only aws and azure are supported", bsl.Spec.Provider)
}

// mapAWSBSLToStorage maps a Velero AWS BackupStorageLocation to HCPEtcdBackup S3 storage.
func mapAWSBSLToStorage(bsl *velerov1.BackupStorageLocation, keyPrefix string) *hyperv1.HCPEtcdBackupStorage {
	bucket := ""
	if bsl.Spec.ObjectStorage != nil {
		bucket = bsl.Spec.ObjectStorage.Bucket
	}
	region := ""
	if bsl.Spec.Config != nil {
		region = bsl.Spec.Config["region"]
	}
	return &hyperv1.HCPEtcdBackupStorage{
		StorageType: hyperv1.S3BackupStorage,
		S3: hyperv1.HCPEtcdBackupS3{
			Bucket:    bucket,
			Region:    region,
			KeyPrefix: keyPrefix,
		},
	}
}

// mapAzureBSLToStorage maps a Velero Azure BackupStorageLocation to HCPEtcdBackup Azure Blob storage.
func mapAzureBSLToStorage(bsl *velerov1.BackupStorageLocation, keyPrefix string) *hyperv1.HCPEtcdBackupStorage {
	container := ""
	if bsl.Spec.ObjectStorage != nil {
		container = bsl.Spec.ObjectStorage.Bucket
	}
	storageAccount := ""
	if bsl.Spec.Config != nil {
		storageAccount = bsl.Spec.Config["storageAccount"]
	}
	return &hyperv1.HCPEtcdBackupStorage{
		StorageType: hyperv1.AzureBlobBackupStorage,
		AzureBlob: hyperv1.HCPEtcdBackupAzureBlob{
			Container:      container,
			StorageAccount: storageAccount,
			KeyPrefix:      keyPrefix,
		},
	}
}

// copyCredentialSecret copies the BSL credential Secret to the HO namespace,
// remapping the data key from the BSL's key (typically "cloud") to "credentials"
// as expected by the HCPEtcdBackup controller.
// If the destination Secret already exists, it is reused. The credential data
// contains an STS IAM Role ARN (not rotatable keys), so it is safe to reuse.
func (o *Orchestrator) copyCredentialSecret(ctx context.Context, bsl *velerov1.BackupStorageLocation, fromNS, toNS, backupName string) (string, error) {
	if bsl.Spec.Credential == nil {
		return "", fmt.Errorf("BSL %q has no credential reference", bsl.Name)
	}

	dstName := fmt.Sprintf("etcd-backup-creds-%s", backupName)

	// Check if the destination Secret already exists
	if err := o.client.Get(ctx, types.NamespacedName{Name: dstName, Namespace: toNS}, &corev1.Secret{}); err == nil {
		o.log.Infof("Credential Secret %s/%s already exists, reusing", toNS, dstName)
		return dstName, nil
	}

	srcSecret := &corev1.Secret{}
	if err := o.client.Get(ctx, types.NamespacedName{
		Name:      bsl.Spec.Credential.Name,
		Namespace: fromNS,
	}, srcSecret); err != nil {
		return "", fmt.Errorf("failed to get BSL credential Secret %s/%s: %w", fromNS, bsl.Spec.Credential.Name, err)
	}

	// The BSL references a specific key in the Secret (e.g. "cloud").
	// The HCPEtcdBackup controller mounts the Secret as a volume and reads
	// the file at /etc/etcd-backup-creds/credentials, so we remap the key.
	srcKey := bsl.Spec.Credential.Key
	credData, ok := srcSecret.Data[srcKey]
	if !ok {
		return "", fmt.Errorf("BSL credential Secret %s/%s does not contain key %q", fromNS, bsl.Spec.Credential.Name, srcKey)
	}

	dstSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dstName,
			Namespace: toNS,
			Labels: map[string]string{
				"hypershift.openshift.io/etcd-backup": "true",
			},
		},
		Data: map[string][]byte{
			"credentials": credData,
		},
	}

	if err := o.client.Create(ctx, dstSecret); err != nil {
		return "", fmt.Errorf("failed to create credential Secret %s/%s: %w", toNS, dstName, err)
	}

	o.log.Infof("Copied credential Secret to %s/%s (remapped key %q -> credentials)", toNS, dstName, srcKey)
	return dstName, nil
}

// setCredentialRef sets the credential reference on the storage config.
func setCredentialRef(storage *hyperv1.HCPEtcdBackupStorage, secretName string) {
	ref := hyperv1.SecretReference{Name: secretName}
	switch storage.StorageType {
	case hyperv1.S3BackupStorage:
		storage.S3.Credentials = ref
	case hyperv1.AzureBlobBackupStorage:
		storage.AzureBlob.Credentials = ref
	}
}

// setEncryptionFields sets encryption config from HostedCluster's ManagedEtcdSpec.Backup.
func setEncryptionFields(storage *hyperv1.HCPEtcdBackupStorage, hc *hyperv1.HostedCluster) {
	if hc.Spec.Etcd.Managed == nil {
		return
	}
	backupConfig := hc.Spec.Etcd.Managed.Backup

	switch storage.StorageType {
	case hyperv1.S3BackupStorage:
		if backupConfig.Platform == hyperv1.AWSBackupConfigPlatform && backupConfig.AWS.KMSKeyARN != "" {
			storage.S3.KMSKeyARN = backupConfig.AWS.KMSKeyARN
		}
	case hyperv1.AzureBlobBackupStorage:
		if backupConfig.Platform == hyperv1.AzureBackupConfigPlatform && backupConfig.Azure.EncryptionKeyURL != "" {
			storage.AzureBlob.EncryptionKeyURL = backupConfig.Azure.EncryptionKeyURL
		}
	}
}

// pollCondition polls the HCPEtcdBackup's BackupCompleted condition until the check function
// returns true (done) or an error (terminal failure), or until timeout.
// The first check runs immediately (before the first interval wait).
func (o *Orchestrator) pollCondition(ctx context.Context, timeout time.Duration, check func(*metav1.Condition) (bool, error)) error {
	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		eb := &hyperv1.HCPEtcdBackup{}
		if err := o.client.Get(ctx, types.NamespacedName{Name: o.BackupName, Namespace: o.BackupNamespace}, eb); err != nil {
			if apierrors.IsNotFound(err) {
				return false, nil
			}
			return false, fmt.Errorf("failed to get HCPEtcdBackup: %w", err)
		}

		cond := meta.FindStatusCondition(eb.Status.Conditions, string(hyperv1.BackupCompleted))
		return check(cond)
	})
}

