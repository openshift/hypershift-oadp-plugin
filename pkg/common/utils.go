package common

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	cronv3 "github.com/robfig/cron/v3"
	"github.com/sirupsen/logrus"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
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

func WaitForPausedPropagated(ctx context.Context, client crclient.Client, log logrus.FieldLogger, hc *hyperv1.HostedCluster) error {
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	err := wait.PollUntilContextCancel(waitCtx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		hcp := &hyperv1.HostedControlPlane{}
		if err := client.Get(ctx, types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, hcp); err != nil {
			if apierrors.IsNotFound(err) {
				return true, nil
			}
			log.Error(err, "failed to get HostedControlPlane", "namespace", hc.Namespace, "name", hc.Name)
			return false, err
		}
		log.Infof("waiting for HCP to be paused", "namespace", hc.Namespace, "name", hc.Name)

		if hcp.Spec.PausedUntil != nil {
			log.Debug("HostedControlPlane is paused", "namespace", hc.Namespace, "name", hc.Name)
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Error(err, "giving up, HCP was not updated in the expecteed timeout", "namespace", hc.Namespace, "name", hc.Name)
		return err
	}

	return err
}

func ManagePauseHostedCluster(ctx context.Context, client crclient.Client, log logrus.FieldLogger, paused string, header string, namespaces []string) error {
	log.Debugf("%s listing HostedClusters", header)
	log.Debug("checking namespaces to inspect")
	hostedClusters := &hyperv1.HostedClusterList{}

	for _, ns := range namespaces {
		if err := client.List(ctx, hostedClusters, crclient.InNamespace(ns)); err != nil {
			return err
		}

		if len(hostedClusters.Items) > 0 {
			log.Debugf("%s found HostedClusters in namespace %s", header, ns)
			break
		}
	}

	for _, hc := range hostedClusters.Items {
		if hc.Spec.PausedUntil == nil || *hc.Spec.PausedUntil != paused {
			log.Infof("%s setting PauseUntil to %s in HostedCluster %s", header, paused, hc.Name)
			hc.Spec.PausedUntil = ptr.To(paused)
			if err := client.Update(ctx, &hc); err != nil {
				return err
			}

			// Checking the hc Object to validate the propagation of the PausedUntil field
			log.Debugf("%s checking paused state propagation", header)
			if err := WaitForPausedPropagated(ctx, client, log, &hc); err != nil {
				return err
			}
		}
	}

	return nil
}

func ManagePauseNodepools(ctx context.Context, client crclient.Client, log logrus.FieldLogger, paused string, header string, namespaces []string) error {
	log.Debugf("%s listing NodePools, checking namespaces to inspect", header)
	nodepools := &hyperv1.NodePoolList{}

	for _, ns := range namespaces {
		if err := client.List(ctx, nodepools, crclient.InNamespace(ns)); err != nil {
			return err
		}

		if len(nodepools.Items) > 0 {
			log.Debugf("%s found NodePools in namespace %s", header, ns)
			break
		}
	}

	for _, np := range nodepools.Items {
		if np.Spec.PausedUntil == nil || *np.Spec.PausedUntil != paused {
			log.Infof("%s setting PauseUntil to %s in NodePool: %s", header, paused, np.Name)
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
