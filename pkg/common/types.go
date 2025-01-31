package common

import "time"

const (
	CAPIPausedAnnotationName string = "cluster.x-k8s.io/paused"

	CommonBackupAnnotationName  string = "hypershift.openshift.io/common-backup-plugin"
	CommonRestoreAnnotationName string = "hypershift.openshift.io/common-restore-plugin"

	BackupStatusInProgress BackupStatus  = "InProgress"
	BackupStatusCompleted  BackupStatus  = "Completed"
	RestoreDone            RestoreStatus = "true"

	PluginConfigMapName string = "hypershift-oadp-plugin-config"

	// Default values for the backup plugin.
	defaultDataUploadTimeout    time.Duration = 30 // Minutes
	defaultDataUploadCheckPace  time.Duration = 10 // Seconds
	defaultWaitForPausedTimeout time.Duration = 2 * time.Minute
)

var (
	MainKinds = map[string]bool{
		"HostedCluster":         true,
		"NodePool":              true,
		"PersistentVolume":      true,
		"PersistentVolumeClaim": true,
	}
)

type BackupStatus string
type RestoreStatus string
