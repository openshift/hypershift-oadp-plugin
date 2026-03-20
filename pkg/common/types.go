package common

const (
	CommonBackupAnnotationName  string = "hypershift.openshift.io/common-backup-plugin"
	CommonRestoreAnnotationName string = "hypershift.openshift.io/common-restore-plugin"
	FSBackupLabelName           string = "hypershift.openshift.io/fsbackup"

	BackupStatusInProgress BackupStatus  = "InProgress"
	BackupStatusCompleted  BackupStatus  = "Completed"
	RestoreDone            RestoreStatus = "true"

	PluginConfigMapName string = "hypershift-oadp-plugin-config"

	DefaultK8sSAFilePath      string = "/var/run/secrets/kubernetes.io/serviceaccount"
	KubevirtRHCOSLabel        string = "hypershift.openshift.io/is-kubevirt-rhcos"
	CDIPopulatedForAnnotation string = "cdi.kubevirt.io/storage.populatedFor"
	CDIAPIGroup               string = "cdi.kubevirt.io"

	// Integration with Hypershift, more info here: https://github.com/openshift/hypershift/pull/6195
	HostedClusterRestoredFromBackupAnnotation string = "hypershift.openshift.io/restored-from-backup"

	// hypershift/cluster-api kinds
	HostedClusterKind         string = "HostedCluster"
	HostedControlPlaneKind    string = "HostedControlPlane"
	NodePoolKind              string = "NodePool"
	PersistentVolumeKind      string = "PersistentVolume"
	PersistentVolumeClaimKind string = "PersistentVolumeClaim"
	ClusterDeploymentKind     string = "ClusterDeployment"
	DataVolumeKind            string = "DataVolume"
)

var (
	MainKinds = map[string]bool{
		HostedClusterKind:         true,
		NodePoolKind:              true,
		PersistentVolumeKind:      true,
		PersistentVolumeClaimKind: true,
	}
)

type BackupStatus string
type RestoreStatus string
