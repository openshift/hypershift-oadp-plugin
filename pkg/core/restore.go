package core

import (
	"context"
	"fmt"
	"time"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
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

// RestorePlugin is a plugin to restore hypershift resources.
type RestorePlugin struct {
	log logrus.FieldLogger

	client      crclient.Client
	config      map[string]string
	validator   validation.RestoreValidator
	pvTriggered bool

	// uploadTimeout is the time in minutes to wait for the data upload to finish.
	dataUploadTimeout time.Duration
	dataUploadDone    bool

	*plugtypes.RestoreOptions
}

type RestoreOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	readoptNodes bool
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	managedServices bool
}

// NewRestorePlugin instantiates RestorePlugin.
func NewRestorePlugin(log logrus.FieldLogger) (*RestorePlugin, error) {
	var err error

	log.Infof("%s initializing hypershift OADP restore plugin", restoreLogHeader)
	client, err := common.GetClient()
	if err != nil {
		return nil, fmt.Errorf("error recovering the k8s client: %s", err.Error())
	}
	log.Debugf("%s client recovered", restoreLogHeader)

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
		log.Infof("%s configuration for hypershift OADP plugin not found", restoreLogHeader)
	}

	rp := &RestorePlugin{
		log:    log,
		client: client,
		config: pluginConfig.Data,
		validator: &validation.RestorePluginValidator{
			Log:       log,
			LogHeader: restoreLogHeader,
		},
	}

	if rp.RestoreOptions, err = rp.validator.ValidatePluginConfig(rp.config); err != nil {
		return nil, fmt.Errorf("error validating plugin configuration: %s", err.Error())
	}

	return rp, nil
}

func (p *RestorePlugin) Name() string {
	return "HCPRestorePlugin"
}

func (p *RestorePlugin) AppliesTo() (velero.ResourceSelector, error) {
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
			"persistentvolumes",
			"persistentvolumeclaims",
			"pods",
		},
	}, nil
}

func (p *RestorePlugin) Execute(item runtime.Unstructured, backup *velerov1.Backup) (runtime.Unstructured, []velero.ResourceIdentifier, error) {
	p.log.Debugf("%s Entering Hypershift backup plugin", restoreLogHeader)
	ctx := context.Context(context.Background())

	switch item.GetObjectKind().GroupVersionKind().Kind {
	case "HostedControlPlane":
		p.log.Debugf("%s HostedControlPlane section reached", restoreLogHeader)
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), hcp); err != nil {
			return nil, nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}
		if err := p.validator.ValidatePlatformConfig(hcp); err != nil {
			return nil, nil, fmt.Errorf("error checking platform configuration: %v", err)
		}
	case "HostedCluster", "NodePool", "pv", "pvc":
		// Unpausing NodePools
		if err := common.ManagePauseNodepools(ctx, p.client, p.log, "false", restoreLogHeader, backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error unpausing NodePools: %v", err)
		}

		// Unpausing HostedClusters
		if err := common.ManagePauseHostedCluster(ctx, p.client, p.log, "false", restoreLogHeader, backup.Spec.IncludedNamespaces); err != nil {
			return nil, nil, fmt.Errorf("error unpausing HostedClusters: %v", err)
		}

	}

	return item, nil, nil
}
