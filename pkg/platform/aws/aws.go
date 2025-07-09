package aws

import (
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

func RestoreTasks(hcp *hyperv1.HostedControlPlane, client crclient.Client) error {
	return nil
}
