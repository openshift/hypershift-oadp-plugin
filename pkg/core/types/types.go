package types

import (
	"time"
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
}

type RestoreOptions struct {
	// Migration is a flag to indicate if the backup is for migration purposes.
	Migration bool
	// Readopt Nodes is a flag to indicate if the nodes should be reprovisioned or not during restore.
	ReadoptNodes bool
	// ManagedServices is a flag to indicate if the backup is done for ManagedServices like ROSA, ARO, etc.
	ManagedServices bool
}
