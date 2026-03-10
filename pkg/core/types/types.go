package types

var (
	BackupCommonResources = []string{
		"hostedclusters", "hostedcluster", "hostedcontrolplanes", "hostedcontrolplane", "nodepools", "nodepool",
		"secrets", "secret", "configmaps", "configmap", "persistentvolumes", "persistentvolume", "persistentvolumeclaims", "persistentvolumeclaim", "pods", "pod", "statefulsets", "statefulset", "deployments", "deployment",
		"clusters", "cluster", "machines", "machine", "machinedeployments", "machinedeployment", "machinesets", "machineset",
		"serviceaccounts", "serviceaccount", "roles", "role", "rolebindings", "rolebinding",
		"priorityclasses", "priorityclass", "poddisruptionbudgets", "poddisruptionbudget",
	}

	BackupAWSResources        = []string{"awsmachinepools", "awsmachines", "awsmachinetemplates", "awsmanagedmachinepools", "awsmanagedmachinepooltemplates"}
	BackupAzureResources      = []string{"azuremachines", "azuremachinetemplates", "azuremanagedmachinepools", "azuremanagedmachinepooltemplates"}
	BackupIBMPowerVSResources = []string{"ibmpowervsmachines", "ibmpowervsmachinetemplates", "ibmpowervsclusters", "ibmpowervsclustertemplates"}
	BackupOpenStackResources  = []string{"openstackmachines", "openstackmachinetemplates", "openstackclusters", "openstackclustertemplates"}
	BackupKubevirtResources   = []string{"kubevirtcluster", "kubevirtmachinetemplate", "datavolume"}
	BackupAgentResources      = []string{"agents", "agentmachines", "agentmachinetemplates", "agentmachinepools", "agentclusters", "nmstateconfigs", "nmstateconfig", "infraenvs", "infraenv"}
)

type BackupOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	Migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	ReadoptNodes bool
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	ManagedServices bool
}

type RestoreOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	Migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	ReadoptNodes bool
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	ManagedServices bool
}
