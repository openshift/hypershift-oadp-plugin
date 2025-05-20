package core

import (
	"context"
	"fmt"
	"slices"

	hive "github.com/openshift/hive/apis/hive/v1"
	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
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
func NewRestorePlugin() (*RestorePlugin, error) {
	var (
		err error
		log = logrus.New()
	)
	log.SetLevel(logrus.DebugLevel)

	log.Info("initializing hypershift OADP restore plugin")
	client, err := common.GetClient()
	if err != nil {
		return nil, fmt.Errorf("error recovering the k8s client: %s", err.Error())
	}
	log.Debug("client recovered")

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
		log.Info("configuration for hypershift OADP plugin not found")
	}

	rp := &RestorePlugin{
		log:      log,
		ctx:      ctx,
		client:   client,
		fsBackup: false,
		config:   pluginConfig.Data,
		validator: &validation.RestorePluginValidator{
			Log: log,
		},
	}

	if rp.RestoreOptions, err = rp.validator.ValidatePluginConfig(rp.config); err != nil {
		return nil, fmt.Errorf("error validating plugin configuration: %s", err.Error())
	}

	// Set the log level to pluginVerbosityLevel if set, keep debug level if not set
	if rp.RestoreOptions.PluginVerbosityLevel != "" {
		parsedLevel, err := logrus.ParseLevel(rp.RestoreOptions.PluginVerbosityLevel)
		if err != nil {
			return nil, fmt.Errorf("error parsing pluginVerbosityLevel: %s", err.Error())
		}
		log.Infof("pluginVerbosityLevel set to %s", parsedLevel)
		log.SetLevel(parsedLevel)
	}

	rp.log = log.WithField("type", "core-restore")

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

	// logging may not be working :)
	p.log.Info("WESHAY: get backup from restore")
	p.log.Debug("WESHAY: get backup from restore")

	// get the backup associated with the restore
	backup := new(velerov1api.Backup)
	err := p.client.Get(
		context.TODO(),
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

	p.log.Info("WESHAY:BEGIN: if return early")
	p.log.Debug("WESHAY:BEGIN: if return early")

	// if the backup is nil or the included namespaces are nil, return early
	if backup == nil || backup.Spec.IncludedNamespaces == nil {
		p.log.Error("Backup or IncludedNamespaces is nil")
		return nil, fmt.Errorf("backup or included namespaces is nil")
	}

	// if the backup is not a hypershift backup, return early
	if returnEarly := common.ShouldEndPluginExecution(backup.Spec.IncludedNamespaces, p.client, p.log); returnEarly {
		p.log.Info("Skipping plugin execution - not a hypershift backup")
		return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
	}

	kind := input.Item.GetObjectKind().GroupVersionKind().Kind
	switch {
	case common.MatchSuffixKind(kind, "clusters", "machines"):
		metadata, err := meta.Accessor(input.Item)
		if err != nil {
			return nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		p.log.Debugf("Removing Annotation: %s to %s", common.CAPIPausedAnnotationName, metadata.GetName())
		common.RemoveAnnotation(metadata, common.CAPIPausedAnnotationName)

	case kind == "HostedControlPlane":
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), hcp); err != nil {
			return nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}
		if err := p.validator.ValidatePlatformConfig(hcp, p.config); err != nil {
			return nil, fmt.Errorf("error checking platform configuration: %v", err)
		}

	case kind == "Pod":
		p.log.Debugf("Pod found, skipping restore")
		return &velero.RestoreItemActionExecuteOutput{
			SkipRestore: true,
		}, nil

	case common.MainKinds[kind]:
		// Unpausing NodePools
		if err := common.ManagePauseNodepools(ctx, p.client, p.log, "false", input.Restore.Spec.IncludedNamespaces); err != nil {
			return nil, fmt.Errorf("error unpausing NodePools: %v", err)
		}

		// Unpausing HostedClusters
		if err := common.ManagePauseHostedCluster(ctx, p.client, p.log, "false", input.Restore.Spec.IncludedNamespaces); err != nil {
			return nil, fmt.Errorf("error unpausing HostedClusters: %v", err)
		}

	case kind == "ClusterDeployment":
		clusterdDeployment := &hive.ClusterDeployment{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), clusterdDeployment); err != nil {
			return nil, fmt.Errorf("error converting item to CusterdDeployment: %v", err)
		}

		clusterDeploymentCP := clusterdDeployment.DeepCopy()
		clusterDeploymentCP.Spec.PreserveOnDelete = true

		if err := p.client.Update(ctx, clusterDeploymentCP); err != nil {
			return nil, fmt.Errorf("error updating ClusterDeployment resource with PreserveOnDelete option: %w", err)
		}

	}

	return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
}
