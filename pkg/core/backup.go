package core

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
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
	"k8s.io/utils/ptr"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

// BackupPlugin is a backup item action plugin for Hypershift common objects.
type BackupPlugin struct {
	log logrus.FieldLogger
	ctx context.Context

	client       crclient.Client
	config       map[string]string
	validator    validation.BackupValidator
	pvTriggered  bool
	hcp          *hyperv1.HostedControlPlane
	hcpNamespace string
	ha           bool

	// uploadTimeout is the time in minutes to wait for the data upload to finish.
	dataUploadTimeout time.Duration
	pvBackupStarted   bool
	pvBackupFinished  bool
	duStarted         bool
	duFinished        bool

	*plugtypes.BackupOptions
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
		Log:                 logger,
		Client:              client,
		HA:                  false,
		DataUploadTimeout:   0,
		DataUploadCheckPace: 0,
		PVBackupStarted:     ptr.To(false),
		PVBackupFinished:    ptr.To(false),
		DUStarted:           ptr.To(false),
		DUFinished:          ptr.To(false),
	}

	bp := &BackupPlugin{
		log:       logger,
		client:    client,
		config:    pluginConfig.Data,
		ctx:       ctx,
		validator: validator,
	}

	if bp.BackupOptions, err = bp.validator.ValidatePluginConfig(bp.config); err != nil {
		return nil, fmt.Errorf("error validating plugin configuration: %s", err.Error())
	}

	// Configurar los timeouts en el validator
	if validator, ok := bp.validator.(*validation.BackupPluginValidator); ok {
		validator.DataUploadTimeout = bp.DataUploadTimeout
		validator.DataUploadCheckPace = bp.DataUploadCheckPace
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

// AppliesTo returns information about which resources this action should be invoked for.
// The IncludedResources and ExcludedResources slices can include both resources
// and resources with group names. These work: "ingresses", "ingresses.extensions".
// A BackupPlugin's Execute function will only be invoked on items that match the returned
// selector. A zero-valued ResourceSelector matches all resources.
func (p *BackupPlugin) AppliesTo() (velero.ResourceSelector, error) {
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

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
func (p *BackupPlugin) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Debug("Entering Hypershift backup plugin")
	ctx := context.Context(p.ctx)

	if returnEarly := common.ShouldEndPluginExecution(ctx, backup, p.client, p.log); returnEarly {
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

		if p.hcp.Spec.ControllerAvailabilityPolicy == hyperv1.HighlyAvailable {
			p.ha = true
			if validator, ok := p.validator.(*validation.BackupPluginValidator); ok {
				validator.HA = true
			}
		} else {
			p.ha = false
			if validator, ok := p.validator.(*validation.BackupPluginValidator); ok {
				validator.HA = false
			}
		}
	}

	kind := item.GetObjectKind().GroupVersionKind().Kind
	switch {
	case common.MatchSuffixKind(kind, "clusters", "machines"):
		metadata, err := meta.Accessor(item)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		common.AddAnnotation(metadata, common.CAPIPausedAnnotationName, "true")
		p.log.Infof("Added CAPI Paused Annotation: %s to %s", common.CAPIPausedAnnotationName, metadata.GetName())

	case kind == common.HostedControlPlaneKind:
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), hcp); err != nil {
			return nil, nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}

		if err := p.validator.ValidatePlatformConfig(hcp, backup); err != nil {
			return nil, nil, fmt.Errorf("error checking platform configuration: %v", err)
		}

	case common.MainKinds[kind]:
		// Updating HostedClusters
		if err := common.UpdateHostedCluster(ctx, p.client, p.log, "true", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error updating HostedClusters: %v", err)
		}

		// Updating NodePools
		if err := common.UpdateNodepools(ctx, p.client, p.log, "true", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error updating NodePools: %v", err)
		}

		if kind == common.HostedClusterKind {
			metadata, err := meta.Accessor(item)
			if err != nil {
				return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
			}
			common.AddAnnotation(metadata, common.HostedClusterRestoredFromBackupAnnotation, "")
			p.log.Infof("Added restore annotation to HostedCluster %s", metadata.GetName())
		}

		if kind == "Pod" {
			metadata, err := meta.Accessor(item)
			if err != nil {
				return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
			}

			if strings.Contains(metadata.GetName(), "etcd-") {
				common.AddLabel(metadata, common.FSBackupLabelName, "true")
			}
		}

	case kind == common.ClusterDeploymentKind:
		if p.Migration && p.hcp.Spec.Platform.Type == hyperv1.AgentPlatform {
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
	}

	if (backup.Spec.DefaultVolumesToFsBackup != nil && !*backup.Spec.DefaultVolumesToFsBackup) || backup.Spec.DefaultVolumesToFsBackup == nil {
		p.log.Debug("Validating DataMover")
		if err := p.validator.ValidateDataMover(ctx, p.hcp, backup); err != nil {
			return nil, nil, fmt.Errorf("error validating DataMover: %v", err)
		}

	} else {
		p.log.Debug("checking PodVolumeBackup")
		switch {
		case !p.pvBackupStarted:
			var err error

			p.log.Debug("Checking if PodVolumeBackup exists")
			p.pvBackupStarted, p.pvBackupFinished, err = common.CheckPodVolumeBackup(ctx, p.client, p.log, backup, p.ha)
			if err != nil {
				return nil, nil, err
			}
		// If the PodVolumeBackup is started, we need to wait for it to be completed, if not, continue with the backup
		// This is a security measure to avoid deadlocks in the backup process, when the plugin waits for the PodVolumeBackup
		// to be completed but the PodVolumeBackup is not started yet.
		case p.pvBackupStarted && !p.pvBackupFinished:
			var err error

			p.log.Debug("PodVolumeBackup exists, waiting for it to be completed")
			p.pvBackupFinished, err = common.WaitForPodVolumeBackup(ctx, p.client, p.log, backup, p.dataUploadTimeout, p.DataUploadCheckPace, p.ha)
			if err != nil {
				return nil, nil, err
			}
		}
	}

	if p.pvBackupFinished || p.duFinished && !p.Migration {
		p.log.Debug("Volume backup is done, updating HC and NPs")
		// updating NodePools
		if err := common.UpdateNodepools(ctx, p.client, p.log, "false", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error updating NodePools: %v", err)
		}

		// updating HostedClusters
		if err := common.UpdateHostedCluster(ctx, p.client, p.log, "false", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error updating HostedClusters: %v", err)
		}
	}

	return item, nil, nil
}
