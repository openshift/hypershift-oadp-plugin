package core

import (
	"fmt"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	oadpv1 "github.com/openshift/oadp-operator/api/v1alpha1"
)

func checkPlatformConfig(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	switch hcp.Spec.Platform.Type {
	case hyperv1.AWSPlatform:
		return checkAWSPlatform(hcp, dpa)
	case hyperv1.AzurePlatform:
		return checkAzurePlatform(hcp, dpa)
	case hyperv1.IBMCloudPlatform:
		return checkIBMCloudPlatform(hcp, dpa)
	case hyperv1.KubevirtPlatform:
		return checkKubevirtPlatform(hcp, dpa)
	case hyperv1.OpenStackPlatform:
		return checkOpenStackPlatform(hcp, dpa)
	case hyperv1.AgentPlatform:
		return checkAgentPlatform(hcp, dpa)
	default:
		return fmt.Errorf("unsupported platform type %s", hcp.Spec.Platform.Type)
	}
}

func checkAWSPlatform(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	// Check if the AWS platform is configured properly

	// Check ROSA
	return nil
}

func checkAzurePlatform(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	// Check if the Azure platform is configured properly

	// Check ARO
	return nil
}

func checkIBMCloudPlatform(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	// Check if the IBM Cloud platform is configured properly
	return nil
}

func checkKubevirtPlatform(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	// Check if the Kubevirt platform is configured properly
	return nil
}

func checkOpenStackPlatform(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	// Check if the OpenStack platform is configured properly
	return nil
}

func checkAgentPlatform(hcp *hyperv1.HostedControlPlane, dpa oadpv1.DataProtectionApplication) error {
	// Check if the Agent platform is configured properly
	return nil
}

func (p *BackupPlugin) validatePluginConfig() error {
	// Validate the plugin configuration
	p.log.Debugf("%s validating plugin configuration", logHeader)
	if len(p.config) == 0 {
		p.log.Debug("no configuration provided")
		return nil
	}

	for key, value := range p.config {
		p.log.Debugf("%s configuration key: %s, value: %s", logHeader, key, value)
		switch key {
		case "migration":
			p.BackupOptions.migration = value == "true"
		case "readoptNodes":
			p.BackupOptions.readoptNodes = value == "true"
		case "configureJob":
			p.BackupOptions.configureJob.Name = value
			p.BackupOptions.configureJob.Schedule = p.config["schedule"]
		default:
			p.log.Warnf("unknown configuration key: %s with value %s", key, value)
		}
	}

	return nil
}
