package common

import "time"

const (
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

type BackupStatus string
type RestoreStatus string
