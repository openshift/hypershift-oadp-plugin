package common

import (
	"context"
	"fmt"
	"strings"
	"time"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	v1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
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
func WaitForDataUpload(ctx context.Context, client crclient.Client, log logrus.FieldLogger, backup *v1.Backup) (bool, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	chosenOne := v2alpha1.DataUpload{}

	err := wait.PollUntilContextCancel(waitCtx, 5*time.Second, true, func(ctx context.Context) (bool, error) {
		dul := &v2alpha1.DataUploadList{}
		log.Info("Waiting for DataUpload to be completed...")
		if err := client.List(ctx, dul, crclient.InNamespace("openshift-adp")); err != nil {
			log.Error(err, "failed to get DataUploadList")

			return false, err
		}

		for _, du := range dul.Items {
			if strings.Contains(du.ObjectMeta.GenerateName, backup.Name) {
				log.Infof("Data Upload found. Waiting for completion... StatusPhase: %s Name: %s", du.Status.Phase, du.Name)
				chosenOne = du
				if du.Status.Phase == "Completed" {
					log.Infof("DataUpload is done. Name: %s Status: %s", du.Name, du.Status.Phase)
					return true, nil
				}
			}
		}

		return false, nil
	})

	if err != nil {
		log.Errorf("Giving up, DataUpload was not finished in the expected timeout. StatusPhase: %s Err: %v", chosenOne.Status.Phase, err)
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
		log.Infof("Waiting for Pause propagation to HCP", "namespace", hc.Namespace, "name", hc.Name)

		if hcp.Spec.PausedUntil != nil {
			log.Debug("HostedControlPlane is paused", "namespace", hc.Namespace, "name", hc.Name)
			return true, nil
		}

		return false, nil
	})

	if err != nil {
		log.Error(err, "Giving up, HCP was not updated in the expecteed timeout", "namespace", hc.Namespace, "name", hc.Name)
		return err
	}

	return err
}

func ManagePauseHostedCluster(ctx context.Context, client crclient.Client, log logrus.FieldLogger, paused string, header string, namespaces []string) error {
	log.Debugf("%s Listing HostedClusters", header)
	log.Debug("Checking namespaces to inspect")
	hcs := &hyperv1.HostedClusterList{}

	for _, ns := range namespaces {
		if err := client.List(ctx, hcs, crclient.InNamespace(ns)); err != nil {
			return err
		}

		if len(hcs.Items) > 0 {
			log.Debugf("%s Found HostedClusters in namespace %s", header, ns)
			break
		}
	}

	for _, hc := range hcs.Items {
		if hc.Spec.PausedUntil == nil || *hc.Spec.PausedUntil != paused {
			log.Infof("%s Setting PauseUntil to %s in HostedCluster %s", header, paused, hc.Name)
			hc.Spec.PausedUntil = ptr.To(paused)
			if err := client.Update(ctx, &hc); err != nil {
				return err
			}

			// Checking the hc Object to validate the propagation of the PausedUntil field
			log.Debugf("%s Waiting for Pause propagation", header)
			if err := WaitForPausedPropagated(ctx, client, log, &hc); err != nil {
				return err
			}
		}
	}

	return nil
}

func ManagePauseNodepools(ctx context.Context, client crclient.Client, log logrus.FieldLogger, paused string, header string, namespaces []string) error {
	log.Debugf("%s Listing NodePools", header)
	log.Debug("Checking namespaces to inspect")
	nps := &hyperv1.NodePoolList{}

	for _, ns := range namespaces {
		if err := client.List(ctx, nps, crclient.InNamespace(ns)); err != nil {
			return err
		}

		if len(nps.Items) > 0 {
			log.Debugf("%s Found NodePools in namespace %s", header, ns)
			break
		}
	}

	for _, np := range nps.Items {
		if np.Spec.PausedUntil == nil || *np.Spec.PausedUntil != paused {
			log.Infof("%s Setting PauseUntil to %s in NodePool: %s", header, paused, np.Name)
			np.Spec.PausedUntil = ptr.To(paused)
			if err := client.Update(ctx, &np); err != nil {
				return err
			}
		}
	}

	return nil
}
