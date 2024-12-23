package core

import (
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
)

func checkPlatform(hcp *hyperv1.HostedControlPlane) error {
	switch hcp.Spec.Platform.Type {
	case hyperv1.AWSPlatform:
		return checkAWSPlatform(hcp)
	}
	return nil
}

func checkAWSPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the AWS platform is configured properly
	return nil
}
