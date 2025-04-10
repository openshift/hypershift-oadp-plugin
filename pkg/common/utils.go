package common

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	cronv3 "github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroapiv2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
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

var (
	appBlackList = []string{
		"openshift-apiserver",
	}

	k8sSAFilePath = DefaultK8sSAFilePath
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

// WaitForBackupCompleted waits for the backup to be completed and uploaded to the destination backend
// it returns true if the backup was completed successfully, false otherwise.
func WaitForDataUpload(ctx context.Context, client crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, dataUploadTimeout time.Duration, dataUploadCheckPace time.Duration) (bool, error) {
	if dataUploadTimeout == 0 {
		dataUploadTimeout = defaultDataUploadTimeout
	}

	if dataUploadCheckPace == 0 {
		dataUploadCheckPace = defaultDataUploadCheckPace
	}

	waitCtx, cancel := context.WithTimeout(ctx, dataUploadTimeout*time.Minute)
	defer cancel()
	chosenOne := veleroapiv2alpha1.DataUpload{}

	err := wait.PollUntilContextCancel(waitCtx, dataUploadCheckPace*time.Second, true, func(ctx context.Context) (bool, error) {
		dataUploadSlice := &veleroapiv2alpha1.DataUploadList{}
		log.Info("waiting for dataUpload to be completed...")
		if err := client.List(ctx, dataUploadSlice, crclient.InNamespace("openshift-adp")); err != nil {
			log.Error(err, "failed to get dataUploadList")
			return false, err
		}

		for _, dataUpload := range dataUploadSlice.Items {
			if strings.Contains(dataUpload.ObjectMeta.GenerateName, backup.Name) {
				log.Infof("dataUpload found. Waiting for completion... StatusPhase: %s Name: %s", dataUpload.Status.Phase, dataUpload.Name)
				chosenOne = dataUpload
				if dataUpload.Status.Phase == veleroapiv2alpha1.DataUploadPhaseCompleted {
					log.Infof("dataUpload is done. Name: %s Status: %s", dataUpload.Name, dataUpload.Status.Phase)
					return true, nil
				}
			}
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("giving up, dataUpload was not finished in the expected timeout. StatusPhase: %s Err: %v", chosenOne.Status.Phase, err)
		return false, err
	}

	return true, err
}

// WaitForPausedPropagated waits for the HostedControlPlane (HCP) associated with the given HostedCluster (HC)
// to be paused. It polls the status of the HCP at regular intervals until the HCP is found to be paused or the
// specified timeout is reached.
func WaitForPausedPropagated(ctx context.Context, client crclient.Client, log logrus.FieldLogger, hc *hyperv1.HostedCluster, timeout time.Duration) error {
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
		if err := client.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hcpNamespace}, hcp); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			log.Info(err, "failed to get HostedControlPlane")
			return false, err
		}
		log.Infof("waiting for HCP to be paused")

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

// WaitForPodVolumeBackup waits for the backup to be completed and uploaded to the destination backend.
func WaitForPodVolumeBackup(ctx context.Context, client crclient.Client, log logrus.FieldLogger, backup *veleroapiv1.Backup, dataUploadTimeout time.Duration, dataUploadCheckPace time.Duration, ha bool) (bool, error) {
	var (
		shouldWait bool
		etcdPvbs   []*veleroapiv1.PodVolumeBackup
	)

	if dataUploadTimeout == 0 {
		dataUploadTimeout = defaultDataUploadTimeout
	}

	if dataUploadCheckPace == 0 {
		dataUploadCheckPace = defaultDataUploadCheckPace
	}

	podVolumeBackupList := &veleroapiv1.PodVolumeBackupList{}
	if err := client.List(ctx, podVolumeBackupList, crclient.InNamespace("openshift-adp")); err != nil {
		log.Error(err, "failed to get PodVolumeBackupList")
		return false, err
	}

	for _, pvb := range podVolumeBackupList.Items {
		if strings.Contains(pvb.Spec.Pod.Name, "etcd-") {
			etcdPvbs = append(etcdPvbs, &pvb)
			shouldWait = true
		}
	}

	var etcds int
	if ha {
		etcds = 3
	} else {
		etcds = 1
	}

	if shouldWait {
		succeed := false
		waitCtx, cancel := context.WithTimeout(ctx, dataUploadTimeout*time.Minute)
		defer cancel()
		err := wait.PollUntilContextCancel(waitCtx, dataUploadCheckPace*time.Second, true, func(ctx context.Context) (bool, error) {
			completed := 0
			for _, pvb := range etcdPvbs {
				if err := client.Get(ctx, crclient.ObjectKeyFromObject(pvb), pvb); err != nil {
					log.Info(err, "failed to get PodVolumeBackup", "name", pvb.Spec.Pod.Name)
					return false, nil
				}
				switch pvb.Status.Phase {
				case veleroapiv1.PodVolumeBackupPhaseCompleted:
					log.Infof("PodVolumeBackup is done. Name: %s Status: %s Volume: %s", pvb.Name, pvb.Status.Phase, pvb.Spec.Volume)
					if pvb.Spec.Volume == "data" {
						succeed = true
						completed++
					}
				case veleroapiv1.PodVolumeBackupPhaseFailed:
					return true, fmt.Errorf("PodVolumeBackup failed. Name: %s Status: %s Pod: %v", pvb.Name, pvb.Status.Phase, &pvb.Spec.Pod.Name)
				case veleroapiv1.PodVolumeBackupPhaseInProgress, veleroapiv1.PodVolumeBackupPhaseNew:
					return false, nil
				}
			}

			if etcds != completed {
				log.Info("etcd backup in progress...", "etcds", etcds, "completed", completed)
				return false, nil
			}
			return true, nil
		})

		if err != nil {
			log.Errorf("giving up waiting podVolumeBackup", "error", err)
			return false, err
		}

		if succeed {
			log.Info("etcd backup done!!")
			return true, nil
		}
	}

	return false, nil
}

// ManagePauseHostedCluster manages the pause state of HostedClusters in the specified namespaces.
func ManagePauseHostedCluster(ctx context.Context, client crclient.Client, log logrus.FieldLogger, paused string, namespaces []string) error {
	log.Debug("listing HostedClusters")
	hostedClusters := &hyperv1.HostedClusterList{
		Items: []hyperv1.HostedCluster{},
	}

	for _, ns := range namespaces {
		if err := client.List(ctx, hostedClusters, crclient.InNamespace(ns)); err != nil {
			return err
		}

		if len(hostedClusters.Items) > 0 {
			log.Debug("found HostedClusters in namespace %s", ns)
			break
		}
	}

	for _, hc := range hostedClusters.Items {
		if hc.Spec.PausedUntil == nil || *hc.Spec.PausedUntil != paused {
			log.Infof("setting PauseUntil to %s in HostedCluster %s", paused, hc.Name)
			hc.Spec.PausedUntil = ptr.To(paused)
			if err := client.Update(ctx, &hc); err != nil {
				return err
			}

			// Checking the hc Object to validate the propagation of the PausedUntil field
			log.Debug("checking paused state propagation")
			if err := WaitForPausedPropagated(ctx, client, log, &hc, defaultWaitForPausedTimeout); err != nil {
				return err
			}
		}
	}

	return nil
}

// ManagePauseNodepools updates the PausedUntil field of NodePools in the specified namespaces.
func ManagePauseNodepools(ctx context.Context, client crclient.Client, log logrus.FieldLogger, paused string, namespaces []string) error {
	log.Debug("listing NodePools, checking namespaces to inspect")
	nodepools := &hyperv1.NodePoolList{}

	for _, ns := range namespaces {
		if err := client.List(ctx, nodepools, crclient.InNamespace(ns)); err != nil {
			return err
		}

		if len(nodepools.Items) > 0 {
			log.Debug("found NodePools in namespace %s", ns)
			break
		}
	}

	for _, np := range nodepools.Items {
		if np.Spec.PausedUntil == nil || *np.Spec.PausedUntil != paused {
			log.Infof("%s setting PauseUntil to %s in NodePool: %s", paused, np.Name)
			np.Spec.PausedUntil = ptr.To(paused)
			if err := client.Update(ctx, &np); err != nil {
				return err
			}
		}
	}

	return nil
}

// ValidateCronSchedule validates if a string is a valid cron schedule and rejects disallowed characters.
func ValidateCronSchedule(schedule string) error {
	_, err := cronv3.ParseStandard(schedule)
	if err != nil {
		return fmt.Errorf("invalid format according to cron standard: %w", err)
	}

	// Reject specific disallowed characters
	if strings.ContainsAny(schedule, "?LV") {
		return errors.New("the schedule contains disallowed characters: ?, L, or V")
	}

	// Reject specific patterns using regular expressions
	disallowedPatterns := []string{
		`\bL\b`,
		`\bV\b`,
		`\?`,
	}

	for _, pattern := range disallowedPatterns {
		matched, _ := regexp.MatchString(pattern, schedule)
		if matched {
			return fmt.Errorf("the schedule contains a disallowed pattern: %s", pattern)
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
func GetHCP(ctx context.Context, nsList []string, client crclient.Client, log logrus.FieldLogger) (*hyperv1.HostedControlPlane, error) {
	for _, ns := range nsList {
		hcpList := &hyperv1.HostedControlPlaneList{}
		if err := client.List(ctx, hcpList, crclient.InNamespace(ns)); err != nil {
			return nil, fmt.Errorf("error getting HostedControlPlane: %v", err)
		}

		if len(hcpList.Items) <= 0 {
			log.Info("HostedControlPlane not found, retrying in the rest of the namespaces", "namespace", ns)
			continue
		}
		log.Info("found hostedcontrolplane %s/%s", hcpList.Items[0].Namespace, hcpList.Items[0].Name)

		return &hcpList.Items[0], nil
	}
	return nil, fmt.Errorf("no HostedControlPlane found")
}

// checkPodsAndRestart restarts pods that are stuck in waiting after a restore of the cluster.
// This situation only happens when the FSBackup method is used.
func CheckPodsAndRestart(ctx context.Context, log logrus.FieldLogger, client crclient.Client, ns string) error {
	log.Info("The recovery process is in progress...")
	// This is necessary to let the pods to be ready.
	// If the BlackListed pods get stuck in a waiting state, that pod needs to be restarted.
	time.Sleep(time.Minute * 2)

	waitCtx, cancel := context.WithTimeout(ctx, defaultWaitForTimeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, 10*time.Second, true, func(ctx context.Context) (bool, error) {
		wait := 0
		podList := &corev1.PodList{}
		if err := client.List(ctx, podList, crclient.InNamespace(ns)); err != nil {
			log.Error(err, "failed to get HostedControlPlane pods")
			return false, nil
		}
		log.Infof("waiting for HostedControlPlane pods to be running")

		for _, pod := range podList.Items {
			// Security measure to avoid restarting the wrong pods.
			if slices.Contains(appBlackList, pod.Labels["app"]) {
				for _, status := range pod.Status.InitContainerStatuses {
					if status.State.Waiting != nil {
						if err := client.Delete(ctx, &pod); err != nil {
							log.Error("error restarting the pod", "name", pod.Name, "error", err)
						}
						wait++
						log.Info("pod restarted", "name", pod.Name)
					}
				}
			}
		}

		if wait <= 0 {
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("timeout waiting for HostedControlPlane pods: %v", err)
		return err
	}

	return nil
}

// ForceRestartETCDPodsIfNeeded restarts the ETCD pods in the HCP namespace if they are not running after a timeout.
// This only happens in certain circumstances when the ETCD STS has NodeAffinity labels. The Velero PodPlugin removes
// the NodeAffinity labels from the pods, so the pods are scheduled to the wrong node and get stuck.
// After a restart of the pods, the NodeAffinity labels are added again and the pods are scheduled to the correct node.
func ForceRestartETCDPodsIfNeeded(ctx context.Context, log logrus.FieldLogger, c crclient.Client, ns string, timeout time.Duration) error {
	log.Debugf("Checking if ETCD pods need to be restarted")
	success := false

	if timeout == 0 {
		timeout = 2 * time.Minute
	}

	log = log.WithFields(logrus.Fields{
		"namespace": ns,
	})

	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		etcdPods := &corev1.PodList{}
		if err := c.List(ctx, etcdPods, crclient.InNamespace(ns), crclient.MatchingLabels{"app": "etcd"}); err != nil {
			log.Info(err, "error getting ETCD pods")
			return false, nil
		}

		if len(etcdPods.Items) <= 0 {
			log.Info("no ETCD pods found, retrying...")
			return false, nil
		}

		for _, pod := range etcdPods.Items {
			log.Infof("checking ETCD pod %s", pod.Name)
			if pod.Status.Phase != corev1.PodRunning {
				return false, nil
			}
			log.Infof("ETCD pod %s is running", pod.Name)
		}

		log.Info("all ETCD pods are running")
		success = true
		return true, nil
	})

	// If the ETCD pods are running, we don't need to restart them.
	if success && err == nil {
		log.Info("ETCD pods are running, skipping restart")
		return nil
	}

	if err != nil && !wait.Interrupted(err) {
		return err
	}

	log.Info("deleting ETCD pods to allow STS to regenerate them with the affinity labels")
	etcdPods := &corev1.PodList{}
	if err := c.List(ctx, etcdPods, crclient.InNamespace(ns), crclient.MatchingLabels{"app": "etcd"}); err != nil {
		log.Info(err, "error getting ETCD pods")
		return fmt.Errorf("error getting ETCD pods for restart: %v", err)
	}
	for _, pod := range etcdPods.Items {
		if err := c.Delete(ctx, &pod); err != nil {
			log.Error("error deleting ETCD pod", "name", pod.Name, "error", err)
		}
	}

	return nil
}

func GetHCPNamespace(name, namespace string) string {
	return fmt.Sprintf("%s-%s", namespace, name)
}
