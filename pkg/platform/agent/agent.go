package agent

import (
	"context"
	"fmt"

	hive "github.com/openshift/hive/apis/hive/v1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	"k8s.io/apimachinery/pkg/runtime"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func MigrationTasks(ctx context.Context, item runtime.Unstructured, client crclient.Client, log logrus.FieldLogger, config map[string]string, backup *velerov1.Backup) error {
	log.Debug("Migration backup detected, adding preserverOnDelete to ClusterDeployment object")
	clusterdDeployment := &hive.ClusterDeployment{}

	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(item.UnstructuredContent(), clusterdDeployment); err != nil {
		return fmt.Errorf("error converting item to CusterdDeployment: %v", err)
	}

	clusterDeploymentCP := clusterdDeployment.DeepCopy()
	clusterDeploymentCP.Spec.PreserveOnDelete = false

	if err := client.Update(ctx, clusterDeploymentCP); err != nil {
		return fmt.Errorf("error updating ClusterDeployment resource with PreserveOnDelete option: %w", err)
	}

	return nil
}
