package core

import (
	"context"
	"fmt"
	"strings"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
	"github.com/openshift/hypershift-oadp-plugin/pkg/etcdbackup"
	"github.com/openshift/hypershift-oadp-plugin/pkg/platform/agent"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BackupPlugin is a backup item action plugin for Hypershift common objects.
type BackupPlugin struct {
	log logrus.FieldLogger
	ctx context.Context

	client    crclient.Client
	config    map[string]string
	validator validation.BackupValidator
	hcp       *hyperv1.HostedControlPlane
	*plugtypes.BackupOptions

	// Etcd backup orchestration
	etcdOrchestrator  *etcdbackup.Orchestrator
	hoNamespace       string
	etcdBackupMethod  string
	etcdSnapshotURL   string // populated after HCPEtcdBackup completes
}

// NewBackupPlugin instantiates BackupPlugin.
func NewBackupPlugin(logger logrus.FieldLogger) (*BackupPlugin, error) {
	var (
		err error
	)

	logger = logger.WithFields(logrus.Fields{
		"process": "backup",
	})

	logger.Info("Initializing HCP Backup Plugin")

	client, err := common.GetClient()
	if err != nil {
		return nil, fmt.Errorf("error recovering the k8s client: %s", err.Error())
	}
	logger.Infof("client recovered")
	ctx := context.Background()

	pluginConfig := corev1.ConfigMap{}
	ns, err := common.GetCurrentNamespace()
	if err != nil {
		return nil, fmt.Errorf("error getting current namespace: %s", err.Error())
	}

	err = client.Get(ctx, types.NamespacedName{Name: common.PluginConfigMapName, Namespace: ns}, &pluginConfig)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("error getting plugin configuration: %s", err.Error())
		}
		logger.Infof("configuration for hypershift OADP plugin not found")
	}

	validator := &validation.BackupPluginValidator{
		Log:    logger,
		Client: client,
	}

	hoNamespace := common.DefaultHONamespace
	if v, ok := pluginConfig.Data[common.ConfigKeyHONamespace]; ok && v != "" {
		hoNamespace = v
	}

	etcdBackupMethod := common.EtcdBackupMethodVolume
	if v, ok := pluginConfig.Data[common.ConfigKeyEtcdBackupMethod]; ok && v != "" {
		etcdBackupMethod = v
	}
	if etcdBackupMethod != common.EtcdBackupMethodVolume && etcdBackupMethod != common.EtcdBackupMethodEtcdSnapshot {
		return nil, fmt.Errorf("invalid etcdBackupMethod %q: must be %q or %q", etcdBackupMethod, common.EtcdBackupMethodVolume, common.EtcdBackupMethodEtcdSnapshot)
	}

	bp := &BackupPlugin{
		log:              logger,
		client:           client,
		config:           pluginConfig.Data,
		ctx:              ctx,
		validator:        validator,
		hoNamespace:      hoNamespace,
		etcdBackupMethod: etcdBackupMethod,
	}

	if bp.BackupOptions, err = bp.validator.ValidatePluginConfig(bp.config); err != nil {
		return nil, fmt.Errorf("error validating plugin configuration: %s", err.Error())
	}

	bp.log.Infof("Backup plugin initialized with log level: %s", logrus.GetLevel())

	return bp, nil
}

// Name is required to implement the interface, but the Velero pod does not delegate this
// method -- it's used to tell velero what name it was registered under. The plugin implementation
// must define it, but it will never actually be called.
func (p *BackupPlugin) Name() string {
	return "HCPBackupPlugin"
}

func (p *BackupPlugin) AppliesTo() (velero.ResourceSelector, error) {
	return velero.ResourceSelector{}, nil
}

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
func (p *BackupPlugin) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Debug("Entering Hypershift backup plugin")
	ctx := context.Context(p.ctx)

	if returnEarly, err := common.ShouldEndPluginExecution(ctx, backup, p.client, p.log); returnEarly {
		p.log.Infof("Skipping hypershift plugin execution - not a hypershift backup: %v", err)
		return item, nil, nil
	}

	if p.hcp == nil {
		var err error
		p.hcp, err = common.GetHCP(ctx, backup.Spec.IncludedNamespaces, p.client, p.log)
		if err != nil {
			if apierrors.IsNotFound(err) {
				p.log.Infof("HCP not found, assuming not hypershift cluster to backup")
				return item, nil, nil
			}
			return nil, nil, fmt.Errorf("error getting HCP namespace: %v", err)
		}
	}

	kind := item.GetObjectKind().GroupVersionKind().Kind
	switch {
	case kind == common.HostedControlPlaneKind:
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), hcp); err != nil {
			return nil, nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}

		if err := p.validator.ValidatePlatformConfig(hcp, backup); err != nil {
			return nil, nil, fmt.Errorf("error checking platform configuration: %v", err)
		}

		// Etcd backup: create after validation, wait for completion
		if p.etcdBackupMethod == common.EtcdBackupMethodEtcdSnapshot {
			if err := p.createEtcdBackup(ctx, backup); err != nil {
				return nil, nil, fmt.Errorf("error creating HCPEtcdBackup: %v", err)
			}
		}
		if err := p.waitForEtcdBackupCompletion(ctx); err != nil {
			return nil, nil, err
		}

	case kind == common.HostedClusterKind:
		metadata, err := meta.Accessor(item)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		common.AddAnnotation(metadata, common.HostedClusterRestoredFromBackupAnnotation, "")
		p.log.Infof("Added restore annotation to HostedCluster %s", metadata.GetName())

		// Etcd backup: create if not yet created (HC may arrive before HCP),
		// wait for completion, and inject snapshotURL into the HC item.
		// Velero captures the item as-is from the API server before the HCPEtcdBackup
		// controller updates the HC status with lastSuccessfulEtcdBackupURL.
		// We must inject it here so the backed-up HC contains the URL for restore.
		if p.etcdBackupMethod == common.EtcdBackupMethodEtcdSnapshot {
			if err := p.createEtcdBackup(ctx, backup); err != nil {
				return nil, nil, fmt.Errorf("error creating HCPEtcdBackup: %v", err)
			}
		}
		if err := p.waitForEtcdBackupCompletion(ctx); err != nil {
			return nil, nil, err
		}
		if p.etcdSnapshotURL != "" {
			// Persist as annotation so the restore plugin can read it
			// (Velero strips status from items during restore)
			common.AddAnnotation(metadata, common.EtcdSnapshotURLAnnotation, p.etcdSnapshotURL)
			p.log.Infof("Added etcd snapshot URL annotation to HostedCluster %s: %s", metadata.GetName(), p.etcdSnapshotURL)

			unstructuredContent := item.UnstructuredContent()
			status, ok := unstructuredContent["status"].(map[string]interface{})
			if !ok {
				status = map[string]interface{}{}
				unstructuredContent["status"] = status
			}
			status["lastSuccessfulEtcdBackupURL"] = p.etcdSnapshotURL
			item.SetUnstructuredContent(unstructuredContent)
			p.log.Infof("Injected lastSuccessfulEtcdBackupURL into HostedCluster %s: %s", metadata.GetName(), p.etcdSnapshotURL)
		}

	case kind == "Pod":
		metadata, err := meta.Accessor(item)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}

		if strings.Contains(metadata.GetName(), "etcd-") {
			switch p.etcdBackupMethod {
			case common.EtcdBackupMethodEtcdSnapshot:
				// Skip etcd pods entirely, snapshot is handled by HCPEtcdBackup.
				// This prevents both FSBackup and CSI VolumeSnapshots of etcd volumes.
				p.log.Infof("Skipping etcd pod %s from backup (using etcdSnapshot method)", metadata.GetName())
				return nil, nil, nil
			case common.EtcdBackupMethodVolume:
				if backup.Spec.DefaultVolumesToFsBackup != nil && !*backup.Spec.DefaultVolumesToFsBackup {
					common.AddLabel(metadata, common.FSBackupLabelName, "true")
				}
			}
		}

	// Agent requirements
	case kind == common.ClusterDeploymentKind:
		if p.hcp.Spec.Platform.Type == hyperv1.AgentPlatform {
			if err := agent.MigrationTasks(ctx, item, p.client, p.log, p.config, backup); err != nil {
				return nil, nil, fmt.Errorf("error performing migration tasks for agent platform: %v", err)
			}
		}

	case kind == common.DataVolumeKind || kind == common.PersistentVolumeClaimKind:
		metadata, err := meta.Accessor(item)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		labels := metadata.GetLabels()
		if _, exists := labels[common.KubevirtRHCOSLabel]; exists {
			return nil, nil, nil
		}

		// Exclude etcd data PVCs when using etcdSnapshot method.
		// PVC names follow the StatefulSet pattern: data-etcd-{index}
		if kind == common.PersistentVolumeClaimKind &&
			strings.HasPrefix(metadata.GetName(), common.EtcdPVCPrefix) &&
			p.etcdBackupMethod == common.EtcdBackupMethodEtcdSnapshot {
			p.log.Infof("Excluding etcd PVC %s from backup (using etcdSnapshot method)", metadata.GetName())
			return nil, nil, nil
		}
	}

	return item, nil, nil
}

// createEtcdBackup creates an HCPEtcdBackup CR in the HCP namespace.
// It is idempotent: if the orchestrator already created a backup, it returns immediately.
// Requires the HCPEtcdBackup CRD to exist in the cluster (safenet check).
func (p *BackupPlugin) createEtcdBackup(ctx context.Context, backup *velerov1.Backup) error {
	// Already created by a previous Execute() call
	if p.etcdOrchestrator != nil && p.etcdOrchestrator.IsCreated() {
		return nil
	}

	crdExists, err := common.CRDExists(ctx, "hcpetcdbackups.hypershift.openshift.io", p.client)
	if err != nil {
		return fmt.Errorf("failed to check for HCPEtcdBackup CRD: %w", err)
	}
	if !crdExists {
		return fmt.Errorf("etcdBackupMethod is %q but HCPEtcdBackup CRD not found in the cluster", common.EtcdBackupMethodEtcdSnapshot)
	}

	oadpNS, err := common.GetCurrentNamespace()
	if err != nil {
		return fmt.Errorf("failed to get OADP namespace: %w", err)
	}

	p.etcdOrchestrator = etcdbackup.NewOrchestrator(p.log, p.client, p.hoNamespace, oadpNS)

	// Fetch the HostedCluster for encryption config
	hc, err := common.GetHostedCluster(ctx, p.client, backup.Spec.IncludedNamespaces, p.hcp.Namespace)
	if err != nil {
		p.log.Warnf("Could not find HostedCluster for encryption config: %v", err)
	}

	if err := p.etcdOrchestrator.CreateEtcdBackup(ctx, backup, p.hcp.Namespace, hc); err != nil {
		if cleanupErr := p.etcdOrchestrator.CleanupCredentialSecret(ctx); cleanupErr != nil {
			p.log.Warnf("Failed to cleanup credential Secret after create error: %v", cleanupErr)
		}
		return err
	}

	if err := p.etcdOrchestrator.VerifyInProgress(ctx); err != nil {
		if cleanupErr := p.etcdOrchestrator.CleanupCredentialSecret(ctx); cleanupErr != nil {
			p.log.Warnf("Failed to cleanup credential Secret after verify error: %v", cleanupErr)
		}
		return err
	}

	return nil
}

// waitForEtcdBackupCompletion waits for the HCPEtcdBackup to finish and cleans up
// the copied credential Secret. Caches the snapshotURL on the plugin struct so it
// is available regardless of item processing order (HC before HCP or vice versa).
// It is a no-op if no etcd backup was created.
func (p *BackupPlugin) waitForEtcdBackupCompletion(ctx context.Context) error {
	if p.etcdOrchestrator == nil || !p.etcdOrchestrator.IsCreated() {
		return nil
	}

	// Already completed in a previous Execute() call
	if p.etcdSnapshotURL != "" {
		return nil
	}

	snapshotURL, err := p.etcdOrchestrator.WaitForCompletion(ctx)
	if err != nil {
		return fmt.Errorf("HCPEtcdBackup failed: %v", err)
	}
	p.etcdSnapshotURL = snapshotURL
	p.log.Infof("HCPEtcdBackup completed, snapshotURL: %s", snapshotURL)

	if cleanupErr := p.etcdOrchestrator.CleanupCredentialSecret(ctx); cleanupErr != nil {
		p.log.Warnf("Failed to cleanup etcd backup credential Secret: %v", cleanupErr)
	}

	return nil
}
