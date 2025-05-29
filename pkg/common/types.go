package common

import "time"

const (
	CAPIPausedAnnotationName    string = "cluster.x-k8s.io/paused"
	CommonBackupAnnotationName  string = "hypershift.openshift.io/common-backup-plugin"
	CommonRestoreAnnotationName string = "hypershift.openshift.io/common-restore-plugin"
	FSBackupLabelName           string = "hypershift.openshift.io/fsbackup"

	BackupStatusInProgress BackupStatus  = "InProgress"
	BackupStatusCompleted  BackupStatus  = "Completed"
	RestoreDone            RestoreStatus = "true"

	PluginConfigMapName string = "hypershift-oadp-plugin-config"

	// Default values for the backup plugin.
	defaultDataUploadTimeout    time.Duration = 30 // Minutes
	defaultDataUploadCheckPace  time.Duration = 10 // Seconds
	defaultWaitForPausedTimeout time.Duration = 2 * time.Minute
	defaultWaitForTimeout       time.Duration = 5 * time.Minute

	DefaultK8sSAFilePath string = "/var/run/secrets/kubernetes.io/serviceaccount"
	KubevirtRHCOSLabel   string = "hypershift.openshift.io/is-kubevirt-rhcos"

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
