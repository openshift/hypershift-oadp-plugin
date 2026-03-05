package core

import (
	"context"
	"fmt"
	"strings"

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

	client    crclient.Client
	config    map[string]string
	validator validation.BackupValidator
	hcp       *hyperv1.HostedControlPlane
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
		Log:    logger,
		Client: client,
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

	case kind == common.HostedClusterKind:
		metadata, err := meta.Accessor(item)
		if err != nil {
			return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		common.AddAnnotation(metadata, common.HostedClusterRestoredFromBackupAnnotation, "")
		p.log.Infof("Added restore annotation to HostedCluster %s", metadata.GetName())

	case kind == "Pod":
		// In case of FSBackup, we need to add the label to the pod
		if backup.Spec.DefaultVolumesToFsBackup != nil && !*backup.Spec.DefaultVolumesToFsBackup {
			metadata, err := meta.Accessor(item)
			if err != nil {
				return nil, nil, fmt.Errorf("error getting metadata accessor: %v", err)
			}

			if strings.Contains(metadata.GetName(), "etcd-") {
				common.AddLabel(metadata, common.FSBackupLabelName, "true")
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
	}

	return item, nil, nil
}
