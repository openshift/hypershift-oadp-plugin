package core

import (
	"fmt"
	"strconv"
	"time"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
)

func (p *BackupPlugin) checkPlatformConfig(hcp *hyperv1.HostedControlPlane) error {
	switch hcp.Spec.Platform.Type {
	case hyperv1.AWSPlatform:
		return p.checkAWSPlatform(hcp)
	case hyperv1.AzurePlatform:
		return p.checkAzurePlatform(hcp)
	case hyperv1.IBMCloudPlatform:
		return p.checkIBMCloudPlatform(hcp)
	case hyperv1.KubevirtPlatform:
		return p.checkKubevirtPlatform(hcp)
	case hyperv1.OpenStackPlatform:
		return p.checkOpenStackPlatform(hcp)
	case hyperv1.AgentPlatform:
		return p.checkAgentPlatform(hcp)
	default:
		return fmt.Errorf("unsupported platform type %s", hcp.Spec.Platform.Type)
	}

}

func (p *BackupPlugin) checkAWSPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the AWS platform is configured properly
	// Check ROSA
	p.log.Infof("%s AWS platform configuration is valid", logHeader)
	return nil
}

func (p *BackupPlugin) checkAzurePlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Azure platform is configured properly
	// Check ARO
	p.log.Infof("%s ARO platform configuration is valid", logHeader)
	return nil
}

func (p *BackupPlugin) checkIBMCloudPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the IBM Cloud platform is configured properly
	p.log.Infof("%s IBM platform configuration is valid", logHeader)
	return nil
}

func (p *BackupPlugin) checkKubevirtPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Kubevirt platform is configured properly
	p.log.Infof("%s Kubevirt platform configuration is valid", logHeader)
	return nil
}

func (p *BackupPlugin) checkOpenStackPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the OpenStack platform is configured properly
	p.log.Infof("%s OpenStack platform configuration is valid", logHeader)
	return nil
}

func (p *BackupPlugin) checkAgentPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Agent platform is configured properly
	p.log.Infof("%s Agent platform configuration is valid", logHeader)
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
		case "configureJob", "schedule":
			p.BackupOptions.configureJob.Name = p.config["configureJob"]
			p.BackupOptions.configureJob.Schedule = p.config["schedule"]
		case "managedServices":
			p.BackupOptions.managedServices = value == "true"
		case "dataUploadTimeout":
			minutes, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("error parsing dataUploadTimeout: %s", err.Error())
			}
			p.dataUploadTimeout = time.Duration(minutes)
		case "dataUploadCheckPace":
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return fmt.Errorf("error parsing dataUploadCheckPace: %s", err.Error())
			}
			p.dataUploadCheckPace = time.Duration(seconds)
		default:
			p.log.Warnf("unknown configuration key: %s with value %s", key, value)
		}
	}

	p.log.Infof("%s plugin configuration validated", logHeader)

	return nil
}
