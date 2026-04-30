package common

const (
	CommonBackupAnnotationName  string = "hypershift.openshift.io/common-backup-plugin"
	CommonRestoreAnnotationName string = "hypershift.openshift.io/common-restore-plugin"
	FSBackupLabelName           string = "hypershift.openshift.io/fsbackup"

	BackupStatusInProgress BackupStatus  = "InProgress"
	BackupStatusCompleted  BackupStatus  = "Completed"
	RestoreDone            RestoreStatus = "true"

	PluginConfigMapName string = "hypershift-oadp-plugin-config"

	DefaultK8sSAFilePath string = "/var/run/secrets/kubernetes.io/serviceaccount"
	KubevirtRHCOSLabel   string = "hypershift.openshift.io/is-kubevirt-rhcos"

	// Integration with Hypershift, more info here: https://github.com/openshift/hypershift/pull/6195
	HostedClusterRestoredFromBackupAnnotation string = "hypershift.openshift.io/restored-from-backup"
	// Etcd snapshot URL annotation: set during backup so the restore plugin can read it
	// (Velero strips status from items during restore, so we persist it as an annotation)
	EtcdSnapshotURLAnnotation string = "hypershift.openshift.io/etcd-snapshot-url"

	// hypershift/cluster-api kinds
	HostedClusterKind         string = "HostedCluster"
	HostedControlPlaneKind    string = "HostedControlPlane"
	NodePoolKind              string = "NodePool"
	PersistentVolumeKind      string = "PersistentVolume"
	PersistentVolumeClaimKind string = "PersistentVolumeClaim"
	ClusterDeploymentKind     string = "ClusterDeployment"
	DataVolumeKind            string = "DataVolume"
	HCPEtcdBackupKind         string = "HCPEtcdBackup"

	// Default HyperShift Operator namespace
	DefaultHONamespace string = "hypershift"
	// ConfigMap key to override the HO namespace
	ConfigKeyHONamespace string = "hoNamespace"

	// Etcd backup method configuration
	ConfigKeyEtcdBackupMethod    string = "etcdBackupMethod"
	EtcdBackupMethodVolume       string = "volumeSnapshot"
	EtcdBackupMethodEtcdSnapshot string = "etcdSnapshot"

	// Fallback credential secret for standalone Velero (no DPA).
	// Both ARO (Azure WI) and future ROSA (IRSA) use this convention.
	DefaultCredentialSecretName string = "cloud-credentials"
	DefaultCredentialSecretKey  string = "cloud"

	// DPA CRD name used to detect OADP+DPA vs standalone Velero
	DPACRDName string = "dataprotectionapplications.oadp.openshift.io"

	// Velero annotation to exclude specific volumes from backup
	BackupVolumesExcludesAnnotation string = "backup.velero.io/backup-volumes-excludes"
	// Etcd data volume name in the StatefulSet pod
	EtcdDataVolumeName string = "data"
	// Etcd PVC name prefix (StatefulSet pattern: {volumeName}-{stsName}-{index})
	EtcdPVCPrefix string = "data-etcd-"

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
