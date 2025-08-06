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

	validator := &validation.RestorePluginValidator{
		Log:       logger,
		Client:    client,
		LogHeader: "restore",
	}

	rp := &RestorePlugin{
		log:       logger,
		ctx:       ctx,
		client:    client,
		fsBackup:  false,
		config:    pluginConfig.Data,
		validator: validator,
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
	if returnEarly := common.ShouldEndPluginExecution(ctx, backup, p.client, p.log); returnEarly {
		p.log.Info("Skipping hypershift plugin execution - not a hypershift backup")
		return velero.NewRestoreItemActionExecuteOutput(input.Item), nil
	}

	// if the backup is nil or the included namespaces are nil, return early
	if backup.Spec.IncludedNamespaces == nil {
		p.log.Error("IncludedNamespaces from backup object is nil")
		return nil, fmt.Errorf("included namespaces from backup object is nil")
	}

	kind := input.Item.GetObjectKind().GroupVersionKind().Kind
	switch {
	case common.MatchSuffixKind(kind, "clusters", "machines"):
		metadata, err := meta.Accessor(input.Item)
		if err != nil {
			return nil, fmt.Errorf("error getting metadata accessor: %v", err)
		}
		common.RemoveAnnotation(metadata, common.CAPIPausedAnnotationName)
		p.log.Infof("Removed CAPI Paused Annotation: %s from %s", common.CAPIPausedAnnotationName, metadata.GetName())

	case kind == common.HostedControlPlaneKind:
		hcp := &hyperv1.HostedControlPlane{}
		if err := runtime.DefaultUnstructuredConverter.FromUnstructured(input.Item.UnstructuredContent(), hcp); err != nil {
			return nil, fmt.Errorf("error converting item to HostedControlPlane: %v", err)
		}
		if err := p.validator.ValidatePlatformConfig(hcp, p.config); err != nil {
			return nil, fmt.Errorf("error checking platform configuration: %v", err)
		}

	case kind == "Pod":
		p.log.Debugf("Pod found, skipping restore")
		return velero.NewRestoreItemActionExecuteOutput(input.Item).WithoutRestore(), nil

	case common.MainKinds[kind]:
		// updating NodePools
		if err := common.UpdateNodepools(ctx, p.client, p.log, "false", input.Restore.Spec.IncludedNamespaces); err != nil {
			return nil, fmt.Errorf("error updating NodePools: %v", err)
		}

		// updating HostedClusters
		if err := common.UpdateHostedCluster(ctx, p.client, p.log, "false", input.Restore.Spec.IncludedNamespaces); err != nil {
			return nil, fmt.Errorf("error updating HostedClusters: %v", err)
		}

		if kind == common.HostedClusterKind {
			metadata, err := meta.Accessor(input.Item)
			if err != nil {
				return nil, fmt.Errorf("error getting metadata accessor: %v", err)
			}
			common.AddAnnotation(metadata, common.HostedClusterRestoredFromBackupAnnotation, "")
			p.log.Infof("Added restore annotation to HostedCluster %s", metadata.GetName())
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
