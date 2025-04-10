package types

import (
	"time"
)

var (
	BackupCommonResources = []string{
		"hostedclusters", "hostedcontrolplanes", "nodepool",
		"secrets", "configmaps", "persistentvolumes", "persistentvolumeclaims", "pods", "statefulsets", "deployments",
		"clusters", "machines", "machinedeployments", "machinesets",
	}
	BackupAWSResources        = []string{"awsmachinepools", "awsmachines", "awsmachinetemplates", "awsmanagedmachinepools", "awsmanagedmachinepooltemplates"}
	BackupAzureResources      = []string{"azuremachines", "azuremachinetemplates", "azuremanagedmachinepools", "azuremanagedmachinepooltemplates"}
	BackupIBMPowerVSResources = []string{"ibmpowervsmachines", "ibmpowervsmachinetemplates", "ibmpowervsclusters", "ibmpowervsclustertemplates"}
	BackupOpenStackResources  = []string{"openstackmachines", "openstackmachinetemplates", "openstackclusters", "openstackclustertemplates"}
	BackupKubevirtResources   = []string{"kubevirtmachines", "kubevirtmachinetemplates", "kubevirtmachinepools", "kubevirtclusters"}
	BackupAgentResources      = []string{"agents", "agentmachines", "agentmachinetemplates", "agentmachinepools", "agentclusters"}
)

type BackupOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	Migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	ReadoptNodes bool
	// DataUploadTimeout is the time in minutes to wait for the data upload to finish.
	DataUploadTimeout time.Duration
	// DataUploadCheckPace is the time in seconds to check if the data upload is finished.
	DataUploadCheckPace time.Duration
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	ManagedServices bool
	// PluginVerbosityLevel is the verbosity level of the plugin.
	PluginVerbosityLevel string
}

type RestoreOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	Migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	ReadoptNodes bool
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	ManagedServices bool
	// PluginVerbosityLevel is the verbosity level of the plugin.
	PluginVerbosityLevel string
}
