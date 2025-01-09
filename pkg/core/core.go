package core

import (
	"context"
	"fmt"
	"time"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	component = "core"
)

var (
	logHeader = fmt.Sprintf("[%s]", component)
)

// BackupPlugin is a backup item action plugin for Hypershift common objects.
type BackupPlugin struct {
	log logrus.FieldLogger

	client   crclient.Client
	config   map[string]string
	platform string

	// uploadTimeout is the time in minutes to wait for the data upload to finish.
	dataUploadTimeout time.Duration
	dataUploadDone    bool

	BackupOptions
}

type BackupOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	readoptNodes bool
	// ConfigureJob is a struct to hold the cron job configuration to schedule it the desired pace.
	configureJob cronJob
	// DataUploadTimeout is the time in minutes to wait for the data upload to finish.
	dataUploadTimeout time.Duration
	// DataUploadCheckPace is the time in seconds to check if the data upload is finished.
	dataUploadCheckPace time.Duration
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	managedServices bool
}

type cronJob struct {
	Name     string
	Schedule string
}

// NewBackupPlugin instantiates BackupPlugin.
func NewBackupPlugin(log logrus.FieldLogger) (*BackupPlugin, error) {
	log.Infof("%s initializing hypershift OADP plugin", logHeader)
	client, err := common.GetClient()
	if err != nil {
		return nil, fmt.Errorf("error recovering the k8s client: %s", err.Error())
	}
	log.Debugf("%s client recovered", logHeader)

	pluginConfig := corev1.ConfigMap{}
	ns, err := common.GetCurrentNamespace()
	if err != nil {
		return nil, fmt.Errorf("error getting current namespace: %s", err.Error())
	}

	err = client.Get(context.TODO(), types.NamespacedName{Name: common.PluginConfigMapName, Namespace: ns}, &pluginConfig)
	if err != nil {
		if !apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("error getting plugin configuration: %s", err.Error())
		}
		log.Warnf("%s configuration for hypershift OADP plugin not found", logHeader)
	}

	bp := &BackupPlugin{
		log:    log,
		client: client,
		config: pluginConfig.Data,
	}

	if err := bp.validatePluginConfig(); err != nil {
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
		IncludedResources: []string{
			"hostedcluster",
			"nodepool",
			"secrets",
			"hostedcontrolplane",
			"cluster",
			"machinedeployment",
			"machineset",
			"machine",
			"machinepools",
			"agentmachines",
			"agentmachinetemplates",
			"awsmachinepools",
			"awsmachines",
			"awsmachinetemplates",
			"azuremachines",
			"azuremachinetemplates",
			"azuremanagedmachinepools",
			"azuremanagedmachinepooltemplates",
			"controllerconfigs",
			"ibmpowervsmachines",
			"ibmpowervsmachinetemplates",
			"ibmvpcmachines",
			"ibmvpcmachinetemplates",
			"kubevirtmachines",
			"kubevirtmachinetemplates",
			"openstackmachines",
			"openstackmachinetemplates",
		},
	}, nil
}

// Execute allows the ItemAction to perform arbitrary logic with the item being backed up,
func (p *BackupPlugin) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Debugf("%s Entering Hypershift backup plugin", logHeader)
	//var err error

	ctx := context.Context(context.Background())

	switch item.GetObjectKind().GroupVersionKind().Kind {
	case "HostedControlPlane":
		p.log.Debugf("%s HostedControlPlane section reached", logHeader)
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), hcp); err != nil {
			return nil, nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}
		if err := p.checkPlatformConfig(hcp); err != nil {
			return nil, nil, fmt.Errorf("error checking platform configuration: %v", err)
		}
	case "HostedCluster", "NodePool", "pv", "pvc":
		// Pausing HostedClusters
		if err := common.ManagePauseHostedCluster(ctx, p.client, p.log, "true", logHeader, backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error pausing HostedClusters: %v", err)
		}

		// Pausing NodePools
		if err := common.ManagePauseNodepools(ctx, p.client, p.log, "true", logHeader, backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error pausing NodePools: %v", err)
		}
	}

	//if !p.dataUploadDone {
	//	p.log.Debugf("%s DataUpload not finished yet", logHeader)
	//	if item.GetObjectKind().GroupVersionKind().Kind == "Secret" {
	//		p.log.Infof("%s Secret section reached", logHeader)
	//		// This function will wait before the secrets got backed up.
	//		// This is a workaround because of the limitations of velero plugins and hooks.
	//		// We need to think how to acomplish that in a better way in the final solution.
	//		if p.dataUploadDone, err = common.WaitForDataUpload(ctx, p.client, p.log, backup, p.dataUploadTimeout, p.dataUploadCheckPace); err != nil {
	//			return nil, nil, err
	//		}
	//	}
	//} else {
	//	p.log.Debugf("%s DataUpload done, unpausing HC and NPs", logHeader)
	//	// Unpausing NodePools
	//	if err := common.ManagePauseNodepools(ctx, p.client, p.log, "false", logHeader, backup.Spec.IncludedNamespaces); err != nil {
	//		return nil, nil, fmt.Errorf("error unpausing NodePools: %v", err)
	//	}

	//	// Unpausing HostedClusters
	//	if err := common.ManagePauseHostedCluster(ctx, p.client, p.log, "false", logHeader, backup.Spec.IncludedNamespaces); err != nil {
	//		return nil, nil, fmt.Errorf("error unpausing HostedClusters: %v", err)
	//	}
	//}

	return item, nil, nil
}
