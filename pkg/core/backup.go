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
	finished          bool

	*plugtypes.BackupOptions
}

// NewBackupPlugin instantiates BackupPlugin.
func NewBackupPlugin(log logrus.FieldLogger) (*BackupPlugin, error) {
	var (
		err error
	)
	log.Infof("initializing hypershift OADP backup plugin")
	client, err := common.GetClient()
	if err != nil {
		return nil, fmt.Errorf("error recovering the k8s client: %s", err.Error())
	}
	log.Infof("client recovered")
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
		log.Infof("configuration for hypershift OADP plugin not found")
	}

	bp := &BackupPlugin{
		log:       log.WithField("type", "core-backup"),
		client:    client,
		config:    pluginConfig.Data,
		finished:  false,
		ctx:       ctx,
		validator: &validation.BackupPluginValidator{Log: log},
	}

	if bp.BackupOptions, err = bp.validator.ValidatePluginConfig(bp.config); err != nil {
		return nil, fmt.Errorf("error validating plugin configuration: %s", err.Error())
	}

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

	if returnEarly := common.ShouldEndPluginExecution(backup.Spec.IncludedNamespaces, p.client, p.log); returnEarly {
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
		} else {
			p.ha = false
		}
	}

	kind := item.GetObjectKind().GroupVersionKind().Kind
	switch {
	case common.MatchSuffixKind(kind, "clusters", "machines"):
		metadata, err := meta.Accessor(item)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		p.log.Debugf("Adding Annotation: %s to %s", common.CAPIPausedAnnotationName, metadata.GetName())
		common.AddAnnotation(metadata, common.CAPIPausedAnnotationName, "true")

	case kind == "HostedControlPlane":
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), hcp); err != nil {
			return nil, nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}

		if err := p.validator.ValidatePlatformConfig(hcp); err != nil {
			return nil, nil, fmt.Errorf("error checking platform configuration: %v", err)
		}

	case common.MainKinds[kind]:
		// Pausing HostedClusters
		if err := common.ManagePauseHostedCluster(ctx, p.client, p.log, "true", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error pausing HostedClusters: %v", err)
		}

		// Pausing NodePools
		if err := common.ManagePauseNodepools(ctx, p.client, p.log, "true", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error pausing NodePools: %v", err)
		}

		defer func() {
			p.pvTriggered = true
		}()

	case kind == "ClusterDeployment":
		if p.Migration && p.hcp.Spec.Platform.Type == hyperv1.AgentPlatform {
			if err := agent.MigrationTasks(ctx, item, p.client, p.log, p.config, backup); err != nil {
				return nil, nil, fmt.Errorf("error performing migration tasks for agent platform: %v", err)
			}
		}

	case kind == "DataVolume" || kind == "PersistentVolumeClaim":
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
		if !p.finished {
			var err error
			p.log.Debug("DataUpload not finished yet")
			if p.pvTriggered {
				if p.finished, err = common.WaitForDataUpload(ctx, p.client, p.log, backup, p.dataUploadTimeout, p.DataUploadCheckPace); err != nil {
					return nil, nil, err
				}
			}
		}
	} else {
		p.log.Debug("checking PodVolumeBackup")
		if kind == "Pod" {
			metadata, err := meta.Accessor(item)
			if err != nil {
				return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
			}

			if strings.Contains(metadata.GetName(), "etcd-") {
				common.AddLabel(metadata, common.FSBackupLabelName, "true")
			}
		}

		if p.pvTriggered {
			result, err := common.WaitForPodVolumeBackup(ctx, p.client, p.log, backup, p.dataUploadTimeout, p.DataUploadCheckPace, p.ha)
			if err != nil {
				return nil, nil, err
			}

			if result {
				p.finished = result

			}
		}
	}

	if p.finished && !p.Migration {
		p.log.Debug("Volume backup is done, unpausing HC and NPs")
		// Unpausing NodePools
		if err := common.ManagePauseNodepools(ctx, p.client, p.log, "false", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error unpausing NodePools: %v", err)
		}

		// Unpausing HostedClusters
		if err := common.ManagePauseHostedCluster(ctx, p.client, p.log, "false", backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error unpausing HostedClusters: %v", err)
		}
	}

	return item, nil, nil
}
