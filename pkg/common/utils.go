package common

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	veleroapiv2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"
	cr "sigs.k8s.io/controller-runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type blackList struct {
	kind       string
	duObjects  []*veleroapiv2alpha1.DataUpload
	vscObjects []*snapshotv1.VolumeSnapshotContent
	vsObjects  []*snapshotv1.VolumeSnapshot
}

var (
	appBlackList = []string{
		"openshift-apiserver",
	}
	k8sSAFilePath                          = DefaultK8sSAFilePath
	vscBlackList, vsBlackList, duBlackList blackList
)

func getMetadataAndAnnotations(item runtime.Unstructured) (metav1.Object, map[string]string, error) {
	metadata, err := meta.Accessor(item)
	if err != nil {
		return nil, nil, err
	}

	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}

	return metadata, annotations, nil
}

// GetClient creates a controller-runtime client for Kubernetes
func GetClient() (crclient.Client, error) {
	config, err := GetConfig()
	if err != nil {
		return nil, fmt.Errorf("unable to get kubernetes config: %w", err)
	}
	client, err := crclient.New(config, crclient.Options{Scheme: CustomScheme})
	if err != nil {
		return nil, fmt.Errorf("unable to get kubernetes client: %w", err)
	}
	return client, nil
}

// GetConfig retrieves the Kubernetes REST configuration using the client-go library.
func GetConfig() (*rest.Config, error) {
	cfg, err := cr.GetConfig()
	if err != nil {
		return nil, err
	}
	cfg.QPS = 100
	cfg.Burst = 100
	return cfg, nil
}

// CheckDataUpload checks if the dataUpload is started and finished.
// The first return value is true if the dataUpload is started, false otherwise.
// The second return value is true if the dataUpload is finished, false otherwise.
// The third return value is an error if the dataUpload is failed, nil otherwise.
// The function supports blacklisting of DataUpload objects to exclude them from processing.
func CheckDataUpload(ctx context.Context, client crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool, duBlackList blackList) (bool, bool, error) {
	var (
		started, finished bool = false, false
		completed         int
		nodes             int = 1
	)

	dataUploadSlice := &veleroapiv2alpha1.DataUploadList{}
	if err := client.List(ctx, dataUploadSlice, crclient.InNamespace(backup.Namespace), crclient.MatchingLabels{veleroapiv1.BackupNameLabel: backup.Name}); err != nil {
		log.Error(err, "failed to get dataUploadList")
		return started, finished, err
	}

	if ha {
		log.Debugf("HA mode detected, setting nodes to 3")
		nodes = 3
	}

	for _, dataUpload := range dataUploadSlice.Items {
		if strings.Contains(dataUpload.ObjectMeta.GenerateName, backup.Name) {
			if duBlackList.IsBlackListed(&dataUpload, log) {
				log.Debugf("dataUpload found but blacklisted. Name: %s", dataUpload.Name)
				continue
			}
			started = true
			log.Infof("dataUpload found. Waiting for completion... StatusPhase: %s Name: %s, backup: %s", dataUpload.Status.Phase, dataUpload.Name, backup.Name)
			switch dataUpload.Status.Phase {
			case veleroapiv2alpha1.DataUploadPhaseCompleted:
				log.Infof("dataUpload details. Name: %s Status: %s, backup: %s", dataUpload.Name, dataUpload.Status.Phase, backup.Name)
				completed++
			case veleroapiv2alpha1.DataUploadPhaseFailed:
				return started, finished, fmt.Errorf("dataUpload failed. Name: %s Status: %s, backup: %s", dataUpload.Name, dataUpload.Status.Phase, backup.Name)
			case veleroapiv2alpha1.DataUploadPhaseInProgress, veleroapiv2alpha1.DataUploadPhaseNew:
				return started, finished, nil
			}
		}
	}

	if completed == nodes {
		log.Infof("dataUpload process is done. Name: %s, started: %v, finished: %v, completed: %d/%d", backup.Name, started, finished, completed, nodes)
		finished = true
	}

	log.Debugf("dataUpload process is not finished yet for backup %s, started: %v, finished: %v, completed: %d/%d", backup.Name, started, finished, completed, nodes)
	return started, finished, nil
}

// WaitForDataUpload waits for the dataUpload to be completed.
// It returns true if the dataUpload was completed successfully, false otherwise.
// The function supports blacklisting of DataUpload objects to exclude them from processing.
func WaitForDataUpload(ctx context.Context, client crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, dataUploadTimeout time.Duration, dataUploadCheckPace time.Duration, ha bool, duBlackList blackList) (bool, error) {
	if dataUploadTimeout == 0 {
		dataUploadTimeout = defaultPVBackupTimeout
	}

	if dataUploadCheckPace == 0 {
		dataUploadCheckPace = defaultPVBackupCheckPace
	}

	waitCtx, cancel := context.WithTimeout(ctx, dataUploadTimeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, dataUploadCheckPace, true, func(ctx context.Context) (bool, error) {
		log.Info("waiting for dataUpload to be completed...")
		_, duFinished, err := CheckDataUpload(ctx, client, log, backup, ha, duBlackList)
		if err != nil {
			return false, err
		}

		if duFinished {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("giving up, dataUpload was not finished in the expected timeout. Err: %v", err)
		return false, err
	}

	return true, err
}

// CheckVolumeSnapshotContent checks if the volumeSnapshotContent is started and finished.
// The first return value is true if the volumeSnapshotContent is started, false otherwise.
// The second return value is true if the volumeSnapshotContent is finished, false otherwise.
// The third return value is an error if the volumeSnapshotContent is failed, nil otherwise.
// The function supports blacklisting of VolumeSnapshotContent objects to exclude them from processing.
func CheckVolumeSnapshotContent(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool, hcp *hyperv1.HostedControlPlane, pvBackupStarted *bool, pvBackupFinished *bool, vscBlackList blackList) (bool, bool, error) {
	var (
		started   = *pvBackupStarted
		finished  = false
		completed = 0
		nodes     = 1
		relevant  = 0
	)

	if *pvBackupFinished {
		return true, true, nil
	}

	volumeSnapshotContentSlice := &snapshotv1.VolumeSnapshotContentList{}
	if err := c.List(ctx, volumeSnapshotContentSlice); err != nil {
		return started, finished, fmt.Errorf("failed to get volumeSnapshotContentList: %w", err)
	}

	if ha {
		log.Debugf("HA mode detected, setting nodes to 3")
		nodes = 3
	}

	for _, vsc := range volumeSnapshotContentSlice.Items {
		if vsc.Spec.VolumeSnapshotRef.Namespace == hcp.Namespace && (vsc.Status != nil && vsc.Status.ReadyToUse != nil) {
			if vscBlackList.IsBlackListed(&vsc, log) {
				log.Debugf("volumeSnapshotContent found but blacklisted. Name: %s", vsc.Name)
				continue
			}

			object := vsc.DeepCopy()
			if err := c.Get(ctx, types.NamespacedName{Name: vsc.Name}, object); err != nil {
				return started, finished, fmt.Errorf("failed to get volumeSnapshotContent: %w", err)
			}

			started = true
			if !*object.Status.ReadyToUse {
				relevant++
				*pvBackupStarted = true
				log.Debugf("volumeSnapshotContent found. Waiting for completion... ReadyToUse: %v Name: %s Total Relevant: %d/%d", *object.Status.ReadyToUse, vsc.Name, relevant, nodes)
			}

			if *object.Status.ReadyToUse {
				completed++
				log.Infof("volumeSnapshotContent details. Name: %s ReadyToUse: %v Total: %d/%d, backup: %s", vsc.Name, *object.Status.ReadyToUse, completed, nodes, backup.Name)
			}
		}
	}

	if completed == nodes {
		log.Infof("volumeSnapshotContent process is done. Name: %s, started: %v, finished: %v, completed: %d/%d", backup.Name, started, finished, completed, nodes)
		finished = true
	}

	log.Debugf("volumeSnapshotContent process is not finished yet for backup %s, started: %v, finished: %v, completed: %d/%d", backup.Name, started, finished, completed, nodes)
	return started, finished, nil
}

// WaitForVolumeSnapshotContent waits for the volumeSnapshotContent to be completed.
// It returns true if the volumeSnapshotContent was completed successfully, false otherwise.
// The function supports blacklisting of VolumeSnapshotContent objects to exclude them from processing.
func WaitForVolumeSnapshotContent(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, vscTimeout time.Duration, vscCheckPace time.Duration, ha bool, hcp *hyperv1.HostedControlPlane, pvBackupStarted *bool, pvBackupFinished *bool, vscBlackList blackList) (bool, error) {
	if vscTimeout == 0 {
		vscTimeout = defaultPVBackupTimeout
	}

	if vscCheckPace == 0 {
		vscCheckPace = defaultPVBackupCheckPace
	}

	waitCtx, cancel := context.WithTimeout(ctx, vscTimeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, vscCheckPace, true, func(ctx context.Context) (bool, error) {
		log.Info("waiting for volumeSnapshotContent to be completed...")
		_, vscFinished, err := CheckVolumeSnapshotContent(ctx, c, log, backup, ha, hcp, pvBackupStarted, pvBackupFinished, vscBlackList)
		if err != nil {
			return false, err
		}

		if vscFinished {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return false, fmt.Errorf("giving up, volumeSnapshotContent was not finished in the expected timeout. Err: %w", err)
	}

	return true, err
}

// CheckVolumeSnapshot checks if the volumeSnapshot is started and finished.
// The first return value is true if the volumeSnapshot is started, false otherwise.
// The second return value is true if the volumeSnapshot is finished, false otherwise.
// The third return value is an error if the volumeSnapshot is failed, nil otherwise.
// The function supports blacklisting of VolumeSnapshot objects to exclude them from processing.
func CheckVolumeSnapshot(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool, hcp *hyperv1.HostedControlPlane, pvBackupStarted *bool, pvBackupFinished *bool, vsBlackList blackList) (bool, bool, error) {
	var (
		started   = *pvBackupStarted
		finished  = false
		completed = 0
		nodes     = 1
		relevant  = 0
	)

	if *pvBackupFinished {
		return started, true, nil
	}

	volumeSnapshotSlice := &snapshotv1.VolumeSnapshotList{}
	if err := c.List(ctx, volumeSnapshotSlice, crclient.InNamespace(hcp.Namespace)); err != nil {
		return started, finished, fmt.Errorf("failed to get volumeSnapshotList: %w", err)
	}

	if ha {
		nodes = 3
	}

	for _, vs := range volumeSnapshotSlice.Items {
		if vs.Labels[veleroapiv1.BackupNameLabel] == backup.Name && (vs.Status != nil && vs.Status.ReadyToUse != nil) {
			if vsBlackList.IsBlackListed(&vs, log) {
				log.Debugf("volumeSnapshot found but blacklisted. Name: %s", vs.Name)
				continue
			}

			object := vs.DeepCopy()
			if err := c.Get(ctx, types.NamespacedName{Name: vs.Name, Namespace: vs.Namespace}, object); err != nil {
				return started, finished, fmt.Errorf("failed to get volumeSnapshot: %w", err)
			}
			relevant++
			started = true
			*pvBackupStarted = true
			log.Debugf("volumeSnapshot found. Waiting for completion... ReadyToUse: %v Name: %s Total Relevant: %d/%d", *object.Status.ReadyToUse, vs.Name, relevant, nodes)

			if *object.Status.ReadyToUse {
				completed++
				log.Infof("volumeSnapshot details. Name: %s ReadyToUse: %v Total: %d/%d, backup: %s", vs.Name, *object.Status.ReadyToUse, completed, nodes, backup.Name)
			}
		}
	}

	if completed == nodes {
		log.Infof("volumeSnapshot is done. Name: %s", backup.Name)
		return true, true, nil
	}

	log.Debugf("volumeSnapshot not finished yet for backup. Name: %s Started: %v Finished: %v Completed: %d/%d", backup.Name, started, finished, completed, nodes)
	return started, finished, nil
}

// WaitForVolumeSnapshot waits for the volumeSnapshot to be completed.
// It returns true if the volumeSnapshot was completed successfully, false otherwise.
// The function supports blacklisting of VolumeSnapshot objects to exclude them from processing.
func WaitForVolumeSnapshot(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, vsTimeout time.Duration, vsCheckPace time.Duration, ha bool, hcp *hyperv1.HostedControlPlane, pvBackupStarted *bool, pvBackupFinished *bool, vsBlackList blackList) (bool, error) {
	if vsTimeout == 0 {
		vsTimeout = defaultPVBackupTimeout
	}

	if vsCheckPace == 0 {
		vsCheckPace = defaultPVBackupCheckPace
	}

	waitCtx, cancel := context.WithTimeout(ctx, vsTimeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, vsCheckPace, true, func(ctx context.Context) (bool, error) {
		log.Info("waiting for volumeSnapshot to be completed...")
		_, vsFinished, err := CheckVolumeSnapshot(ctx, c, log, backup, ha, hcp, pvBackupStarted, pvBackupFinished, vsBlackList)
		if err != nil {
			return false, err
		}

		if vsFinished {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		return false, fmt.Errorf("giving up, VolumeSnapshot was not finished in the expected timeout. Err: %w", err)
	}

	return true, err
}

func CheckPodVolumeBackup(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool) (bool, bool, error) {
	var (
		started, finished bool = false, false
		completed         int
		nodes             int = 1
	)

	podVolumeBackupList := &veleroapiv1.PodVolumeBackupList{}
	if err := c.List(ctx, podVolumeBackupList, crclient.InNamespace(backup.Namespace)); err != nil {
		log.Error(err, "failed to get PodVolumeBackupList")
		return started, finished, err
	}

	if ha {
		nodes = 3
	}

	if len(podVolumeBackupList.Items) != nodes {
		log.Infof("Only %d PodVolumeBackups found for backup %s, expecting to have %d PodVolumeBackup objects. Waiting...", len(podVolumeBackupList.Items), backup.Name, nodes)
		return started, finished, nil
	}

	started = true

	for _, pvb := range podVolumeBackupList.Items {
		if pvb.ObjectMeta.Labels[veleroapiv1.BackupNameLabel] == backup.Name {
			log.Debugf("PodVolumeBackup found. Name: %s", pvb.Name)

			switch pvb.Status.Phase {
			case veleroapiv1.PodVolumeBackupPhaseCompleted:
				log.Infof("PodVolumeBackup is done. Name: %s Status: %s Volume: %s", pvb.Name, pvb.Status.Phase, pvb.Spec.Volume)
				if pvb.Spec.Volume == "data" {
					completed++
				}
			case veleroapiv1.PodVolumeBackupPhaseFailed:
				return started, finished, fmt.Errorf("PodVolumeBackup failed. Name: %s Status: %s Pod: %v", pvb.Name, pvb.Status.Phase, &pvb.Spec.Pod.Name)
			case veleroapiv1.PodVolumeBackupPhaseInProgress, veleroapiv1.PodVolumeBackupPhaseNew:
				return started, finished, nil
			}
		}
	}

	if len(podVolumeBackupList.Items) == completed {
		log.Debugf("PodVolumeBackup is done. Name: %s", backup.Name)
		finished = true
	}

	log.Debugf("PodVolumeBackup not finished yet for backup %s", backup.Name)
	return started, finished, nil
}

// WaitForPodVolumeBackup waits for the backup to be completed and uploaded to the destination backend.
func WaitForPodVolumeBackup(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, podVolumeBackupTimeout time.Duration, podVolumeBackupCheckPace time.Duration, ha bool) (bool, error) {
	if podVolumeBackupTimeout == 0 {
		podVolumeBackupTimeout = defaultPVBackupTimeout
	}

	if podVolumeBackupCheckPace == 0 {
		podVolumeBackupCheckPace = defaultPVBackupCheckPace
	}

	waitCtx, cancel := context.WithTimeout(ctx, podVolumeBackupTimeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, podVolumeBackupCheckPace, true, func(ctx context.Context) (bool, error) {
		_, pvbFinished, err := CheckPodVolumeBackup(ctx, c, log, backup, ha)
		if err != nil {
			return false, err
		}

		if pvbFinished {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("giving up waiting podVolumeBackup", "error", err)
		return false, err
	}

	return true, nil
}

// WaitForPausedPropagated waits for the HostedControlPlane (HCP) associated with the given HostedCluster (HC)
// to be paused. It polls the status of the HCP at regular intervals until the HCP is found to be paused or the
// specified timeout is reached.
func WaitForPausedPropagated(ctx context.Context, c crclient.Client, log logrus.FieldLogger, hc *hyperv1.HostedCluster, timeout time.Duration, paused string) error {
	if timeout == 0 {
		timeout = defaultWaitForPausedTimeout
	}
	hcpNamespace := GetHCPNamespace(hc.Name, hc.Namespace)

	log = log.WithFields(logrus.Fields{
		"namespace": hc.Namespace,
		"name":      hc.Name,
	})

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		hcp := &hyperv1.HostedControlPlane{}
		if err := c.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hcpNamespace}, hcp); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			log.Info(err, "failed to get HostedControlPlane")
			return false, err
		}
		log.Infof("waiting for HCP to set PausedUntil to %s", paused)

		if hcp.Spec.PausedUntil != nil {
			log.Debug("HostedControlPlane is paused")
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("giving up, HCP was not updated in the expected timeout: %v", err)
		return err
	}

	return err
}

// UpdateHostedCluster updates the HostedCluster's necessary fields in the specified namespaces.
func UpdateHostedCluster(ctx context.Context, c crclient.Client, log logrus.FieldLogger, paused string, namespaces []string) error {
	log.Debug("listing HostedClusters")
	hostedClusters := &hyperv1.HostedClusterList{
		Items: []hyperv1.HostedCluster{},
	}

	for _, ns := range namespaces {
		if err := c.List(ctx, hostedClusters, crclient.InNamespace(ns)); err != nil {
			return fmt.Errorf("failed to list HostedClusters in namespace %s: %w", ns, err)
		}

		if len(hostedClusters.Items) > 0 {
			log.Debug("found HostedClusters in namespace %s", ns)
			break
		}
	}

	for _, hc := range hostedClusters.Items {
		// Create a retry loop with exponential backoff
		backoff := wait.Backoff{
			Steps:    5,
			Duration: 1 * time.Second,
			Factor:   2.0,
			Jitter:   0.1,
		}

		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			// Get the latest version of the HostedCluster
			currentHC := &hyperv1.HostedCluster{}
			if err := c.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, currentHC); err != nil {
				return false, err
			}

			// Update PausedUntil if needed
			if currentHC.Spec.PausedUntil == nil || *currentHC.Spec.PausedUntil != paused {
				log.Infof("setting PauseUntil to %s in HostedCluster %s", paused, currentHC.Name)
				currentHC.Spec.PausedUntil = ptr.To(paused)
				if err := c.Update(ctx, currentHC); err != nil {
					if apierrors.IsConflict(err) {
						log.Infof("Conflict detected pausing the HostedCluster %s, retrying...", currentHC.Name)
						return false, nil
					}
					return false, err
				}

				// Checking the hc Object to validate the propagation of the PausedUntil field
				log.Debug("checking paused state propagation")
				if err := WaitForPausedPropagated(ctx, c, log, currentHC, defaultWaitForPausedTimeout, paused); err != nil {
					return false, err
				}
			}

			return true, nil
		})

		if err != nil {
			return fmt.Errorf("failed to update HostedCluster %s after retries: %w", hc.Name, err)
		}
	}

	return nil
}

// UpdateNodepools updates the NodePool's necessary fields in the specified namespaces.
func UpdateNodepools(ctx context.Context, c crclient.Client, log logrus.FieldLogger, paused string, namespaces []string) error {
	log.Debug("listing NodePools, checking namespaces to inspect")
	nodepools := &hyperv1.NodePoolList{}

	for _, ns := range namespaces {
		if err := c.List(ctx, nodepools, crclient.InNamespace(ns)); err != nil {
			return fmt.Errorf("failed to list NodePools in namespace %s: %w", ns, err)
		}

		if len(nodepools.Items) > 0 {
			log.Debug("found NodePools in namespace %s", ns)
			break
		}
	}

	for _, np := range nodepools.Items {
		// Create a retry loop with exponential backoff
		backoff := wait.Backoff{
			Steps:    5,
			Duration: 1 * time.Second,
			Factor:   2.0,
			Jitter:   0.1,
		}

		err := wait.ExponentialBackoff(backoff, func() (bool, error) {
			// Get the latest version of the NodePool
			currentNP := &hyperv1.NodePool{}
			if err := c.Get(ctx, types.NamespacedName{Name: np.Name, Namespace: np.Namespace}, currentNP); err != nil {
				return false, err
			}

			if currentNP.Spec.PausedUntil == nil || *currentNP.Spec.PausedUntil != paused {
				log.Infof("%s setting PauseUntil to %s in NodePool: %s", paused, currentNP.Name)
				currentNP.Spec.PausedUntil = ptr.To(paused)
				if err := c.Update(ctx, currentNP); err != nil {
					if apierrors.IsConflict(err) {
						log.Infof("Conflict detected pausing the NodePool %s, retrying...", currentNP.Name)
						return false, nil
					}
					return false, err
				}
			}

			return true, nil
		})

		if err != nil {
			return fmt.Errorf("failed to update NodePool %s after retries: %w", np.Name, err)
		}
	}

	return nil
}

// GetCurrentNamespace reads the namespace from the Kubernetes service account
// token file and returns it as a string. The file is expected to be located at
// "/var/run/secrets/kubernetes.io/serviceaccount/namespace". If there is an error
// reading the file, it returns an empty string and the error.
func GetCurrentNamespace() (string, error) {
	namespaceFilePath := filepath.Join(k8sSAFilePath, "namespace")
	namespace, err := os.ReadFile(namespaceFilePath)
	if err != nil {
		return "", err
	}
	return string(namespace), nil
}

// MatchSuffixKind checks if the given kind string ends with any of the provided suffixes.
// It returns true if a match is found, otherwise it returns false.
func MatchSuffixKind(kind string, suffixes ...string) bool {
	for _, suffix := range suffixes {
		if strings.HasSuffix(kind, suffix) {
			return true
		}
	}
	return false

}

// AddAnnotation adds an annotation to the given metadata object.
// If the annotations map is nil, it initializes it before adding the annotation.
func AddAnnotation(metadata metav1.Object, key, value string) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[key] = value
	metadata.SetAnnotations(annotations)
}

// RemoveAnnotation removes the annotation with the specified key from the given metadata object.
// If the annotations map is nil, the function returns without making any changes.
func RemoveAnnotation(metadata metav1.Object, key string) {
	annotations := metadata.GetAnnotations()
	if annotations == nil {
		return
	}
	delete(annotations, key)
	metadata.SetAnnotations(annotations)
}

// AddLabel adds a label with the specified key and value to the given metadata object.
// If the metadata object does not have any labels, a new map is created to store the label.
func AddLabel(metadata metav1.Object, key, value string) {
	labels := metadata.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}
	labels[key] = value
	metadata.SetLabels(labels)
}

// RemoveLabel removes a label from the metadata of a Kubernetes object.
// If the label does not exist, the function does nothing.
func RemoveLabel(metadata metav1.Object, key string) {
	labels := metadata.GetLabels()
	if labels == nil {
		return
	}
	delete(labels, key)
	metadata.SetLabels(labels)
}

// GetHCP retrieves the first HostedControlPlane object from the provided list of namespaces.
// It iterates through the namespaces and attempts to list HostedControlPlane objects in each namespace.
// If a HostedControlPlane is found, it returns the first one encountered.
// If no HostedControlPlane is found in any of the namespaces, it returns an error.
func GetHCP(ctx context.Context, nsList []string, c crclient.Client, log logrus.FieldLogger) (*hyperv1.HostedControlPlane, error) {
	for _, ns := range nsList {
		hcpList := &hyperv1.HostedControlPlaneList{}
		if err := c.List(ctx, hcpList, crclient.InNamespace(ns)); err != nil {
			return nil, fmt.Errorf("error getting HostedControlPlane: %v", err)
		}

		if len(hcpList.Items) <= 0 {
			log.Infof("HostedControlPlane not found, retrying in the rest of the namespaces: %s", ns)
			continue
		}
		log.Infof("found hostedcontrolplane %s/%s", hcpList.Items[0].Namespace, hcpList.Items[0].Name)

		return &hcpList.Items[0], nil
	}
	return nil, fmt.Errorf("no HostedControlPlane found")
}

func GetHCPNamespace(name, namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, name)
}

// ShouldEndPluginExecution checks if the plugin should end execution by verifying if the required
// Hypershift resources (HostedControlPlane and HostedCluster) exist in the cluster.
// Returns true if the plugin should end execution (i.e., if this is not a Hypershift cluster).
func ShouldEndPluginExecution(ctx context.Context, backup *veleroapiv1.Backup, c crclient.Client, log logrus.FieldLogger) bool {
	if len(backup.Spec.IncludedNamespaces) == 0 {
		log.Debug("No namespaces provided")
		return true
	}

	if slices.Contains(backup.Spec.IncludedResources, "hostedcluster") || slices.Contains(backup.Spec.IncludedResources, "hostedcontrolplane") {
		log.Debug("Hypershift resources found")
		return false
	}

	exists, err := CRDExists(ctx, "hostedcontrolplanes.hypershift.openshift.io", c)
	if err != nil {
		log.Debugf("Error checking for HostedControlPlane CRD: %v", err)
		return true
	}

	if exists {
		log.Debug("HostedControlPlane CRD found")
		return false
	}

	log.Debug("No Hypershift CRDs resources found")
	return true
}

func CRDExists(ctx context.Context, crdName string, c crclient.Client) (bool, error) {
	crd := &apiextensionsv1.CustomResourceDefinition{}
	err := c.Get(ctx, client.ObjectKey{Name: crdName}, crd)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func ReconcileVolumeSnapshotContent(ctx context.Context, hcp *hyperv1.HostedControlPlane, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool, dataUploadTimeout time.Duration, dataUploadCheckPace time.Duration, pvBackupStarted *bool, pvBackupFinished *bool) (bool, error) {
	var (
		vscFinished = false
	)

	if *pvBackupFinished {
		return true, nil
	}

	if vscBlackList.kind == "" {
		var err error

		volumeSnapshotContentSlice := &snapshotv1.VolumeSnapshotContentList{}
		if err := c.List(ctx, volumeSnapshotContentSlice); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to get volumeSnapshotContentList: %w", err)
			}
			return false, nil
		}

		vscBlackList, err = NewBlackList("VolumeSnapshotContent", volumeSnapshotContentSlice, log)
		if err != nil {
			log.Errorf("Error creating volumeSnapshotContent blacklist: %v", err)
		}
	}

	log.Debug("Reconciling VolumeSnapshotContent")
	switch {
	case !*pvBackupStarted:
		var err error

		log.Debug("Checking if VolumeSnapshotContent exists")
		*pvBackupStarted, vscFinished, err = CheckVolumeSnapshotContent(ctx, c, log, backup, ha, hcp, pvBackupStarted, pvBackupFinished, vscBlackList)
		if err != nil {
			return false, err
		}

	case *pvBackupStarted && !*pvBackupFinished || *pvBackupStarted && !vscFinished:
		var err error

		log.Debug("VolumeSnapshotContent exists, waiting for it to be completed")
		vscFinished, err = WaitForVolumeSnapshotContent(ctx, c, log, backup, dataUploadTimeout, dataUploadCheckPace, ha, hcp, pvBackupStarted, pvBackupFinished, vscBlackList)
		if err != nil {
			return false, err
		}
	}
	return vscFinished, nil
}

func ReconcileVolumeSnapshots(ctx context.Context, hcp *hyperv1.HostedControlPlane, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool, dataUploadTimeout time.Duration, dataUploadCheckPace time.Duration, pvBackupStarted *bool, pvBackupFinished *bool) (bool, error) {
	var (
		vsFinished = false
	)

	if *pvBackupFinished {
		return true, nil
	}

	if vsBlackList.kind == "" {
		var err error

		volumeSnapshotSlice := &snapshotv1.VolumeSnapshotList{}
		if err := c.List(ctx, volumeSnapshotSlice, crclient.InNamespace(hcp.Namespace)); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to get volumeSnapshotList: %w", err)
			}
			return false, nil
		}

		vsBlackList, err = NewBlackList("VolumeSnapshot", volumeSnapshotSlice, log)
		if err != nil {
			log.Errorf("Error creating volumeSnapshot blacklist: %v", err)
		}
	}

	log.Debug("Reconciling VolumeSnapshots")
	switch {
	case !*pvBackupStarted:
		var err error

		log.Debug("Checking if VolumeSnapshot exists")
		*pvBackupStarted, vsFinished, err = CheckVolumeSnapshot(ctx, c, log, backup, ha, hcp, pvBackupStarted, pvBackupFinished, vsBlackList)
		if err != nil {
			return false, err
		}

	case *pvBackupStarted && !*pvBackupFinished || *pvBackupStarted && !vsFinished:
		var err error

		log.Debug("VolumeSnapshot exists, waiting for it to be completed")
		vsFinished, err = WaitForVolumeSnapshot(ctx, c, log, backup, dataUploadTimeout, dataUploadCheckPace, ha, hcp, pvBackupStarted, pvBackupFinished, vsBlackList)
		if err != nil {
			return false, err
		}
	}

	return vsFinished, nil
}

func ReconcileDataUpload(ctx context.Context, c crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, ha bool, dataUploadTimeout time.Duration, dataUploadCheckPace time.Duration, duStarted *bool, duFinished *bool) (bool, error) {
	var (
		finished = false
	)

	if *duFinished {
		return true, nil
	}

	if duBlackList.kind == "" {
		var err error

		dataUploadSlice := &veleroapiv2alpha1.DataUploadList{}
		if err := c.List(ctx, dataUploadSlice, crclient.InNamespace(backup.Namespace)); err != nil {
			if !apierrors.IsNotFound(err) {
				return false, fmt.Errorf("failed to get dataUploadList: %w", err)
			}
			return false, nil
		}

		duBlackList, err = NewBlackList("DataUpload", dataUploadSlice, log)
		if err != nil {
			log.Errorf("Error creating dataUpload blacklist: %v", err)
		}
	}

	switch {
	case !*duStarted:
		var err error

		log.Debug("Checking if DataUpload exists")
		*duStarted, finished, err = CheckDataUpload(ctx, c, log, backup, ha, duBlackList)
		if err != nil {
			return false, err
		}

	// If the DataUpload is started, we need to wait for it to be completed, if not, continue with the backup
	// This is a security measure to avoid deadlocks in the backup process, when the plugin waits for the DataUpload
	// to be completed but the DataUpload is not started yet.
	case *duStarted && !*duFinished:
		var err error

		log.Debug("DataUpload exists, waiting for it to be completed")
		finished, err = WaitForDataUpload(ctx, c, log, backup, dataUploadTimeout, dataUploadCheckPace, ha, duBlackList)
		if err != nil {
			return false, err
		}
	}

	if finished {
		*duFinished = true
	}

	return finished, nil
}

func NewBlackList(kind string, objects any, log logrus.FieldLogger) (blackList, error) {
	switch kind {
	case "DataUpload":
		if dataUploadList, ok := objects.(*veleroapiv2alpha1.DataUploadList); ok {
			duObjects := make([]*veleroapiv2alpha1.DataUpload, len(dataUploadList.Items))
			for i, item := range dataUploadList.Items {
				duObjects[i] = &item
			}
			return blackList{
				kind:      kind,
				duObjects: duObjects,
			}, nil
		}
		return blackList{}, fmt.Errorf("invalid object type for DataUpload blacklist: expected DataUpload list, got %T", objects)
	case "VolumeSnapshotContent":
		if vscList, ok := objects.(*snapshotv1.VolumeSnapshotContentList); ok {
			vscObjects := make([]*snapshotv1.VolumeSnapshotContent, len(vscList.Items))
			for i, item := range vscList.Items {
				vscObjects[i] = &item
			}
			return blackList{
				kind:       kind,
				vscObjects: vscObjects,
			}, nil
		}
		return blackList{}, fmt.Errorf("invalid object type for VolumeSnapshotContent blacklist: expected VolumeSnapshotContent list, got %T", objects)
	case "VolumeSnapshot":
		if vsList, ok := objects.(*snapshotv1.VolumeSnapshotList); ok {
			vsObjects := make([]*snapshotv1.VolumeSnapshot, len(vsList.Items))
			for i, item := range vsList.Items {
				vsObjects[i] = &item
			}
			return blackList{
				kind:      kind,
				vsObjects: vsObjects,
			}, nil
		}
		return blackList{}, fmt.Errorf("invalid object type for VolumeSnapshot blacklist: expected VolumeSnapshot list, got %T", objects)
	}
	return blackList{}, fmt.Errorf("unsupported resource type: %s (supported types: DataUpload, VolumeSnapshotContent, VolumeSnapshot)", kind)
}

func (b *blackList) IsBlackListed(obj any, log logrus.FieldLogger) bool {
	switch o := obj.(type) {
	case *veleroapiv2alpha1.DataUpload:
		if b.kind != "DataUpload" {
			return false
		}
		log.Debugf("Checking if DataUpload %s/%s is blacklisted", o.Namespace, o.Name)
		for _, du := range b.duObjects {
			if du.Name == o.Name && du.Namespace == o.Namespace {
				return true
			}
		}
	case *snapshotv1.VolumeSnapshotContent:
		if b.kind != "VolumeSnapshotContent" {
			return false
		}
		log.Debugf("Checking if VolumeSnapshotContent %s/%s is blacklisted", o.Namespace, o.Name)
		for _, vsc := range b.vscObjects {
			if vsc.Name == o.Name && vsc.Namespace == o.Namespace {
				return true
			}
		}
	case *snapshotv1.VolumeSnapshot:
		if b.kind != "VolumeSnapshot" {
			return false
		}
		log.Debugf("Checking if VolumeSnapshot %s/%s is blacklisted", o.Namespace, o.Name)
		for _, vs := range b.vsObjects {
			if vs.Name == o.Name && vs.Namespace == o.Namespace {
				return true
			}
		}
	default:
		log.Debugf("Unsupported object type: %T", obj)
		return false
	}
	return false
}
