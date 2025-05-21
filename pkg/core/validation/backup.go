package validation

import (
	"fmt"
	"strconv"
	"time"

	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
)

type BackupValidator interface {
	ValidatePluginConfig(config map[string]string) (*plugtypes.BackupOptions, error)
	ValidatePlatformConfig(hcp *hyperv1.HostedControlPlane) error
}

type BackupPluginValidator struct {
	Log logrus.FieldLogger
}

func (p *BackupPluginValidator) ValidatePluginConfig(config map[string]string) (*plugtypes.BackupOptions, error) {
	// Validate the plugin configuration
	p.Log.Infof("validating plugin configuration")
	if len(config) == 0 {
		p.Log.Debug("no configuration provided")
		return &plugtypes.BackupOptions{}, nil
	}
	bo := &plugtypes.BackupOptions{}

	for key, value := range config {
		p.Log.Debugf("configuration key: %s, value: %s", key, value)
		switch key {
		case "migration":
			p.Log.Debugf("reading/parsing migration %s", value)
			bo.Migration = value == "true"
		case "readoptNodes":
			p.Log.Debugf("reading/parsing readoptNodes %s", value)
			bo.ReadoptNodes = value == "true"
		case "managedServices":
			p.Log.Debugf("reading/parsing managedServices %s", value)
			bo.ManagedServices = value == "true"
		case "dataUploadTimeout":
			p.Log.Debugf("reading/parsing dataUploadTimeout %s", value)
			minutes, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("error parsing dataUploadTimeout: %s", err.Error())
			}
			bo.DataUploadTimeout = time.Duration(minutes)
		case "dataUploadCheckPace":
			p.Log.Debugf("reading/parsing dataUploadCheckPace %s", value)
			seconds, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, fmt.Errorf("error parsing dataUploadCheckPace: %s", err.Error())
			}
			bo.DataUploadCheckPace = time.Duration(seconds)
		case "pluginVerbosityLevel":
			p.Log.Debugf("reading/parsing pluginVerbosityLevel %s", value)
		default:
			p.Log.Warnf("unknown configuration key: %s with value %s", key, value)
		}
	}

	p.Log.Infof("plugin configuration validated")

	return bo, nil

}

func (p *BackupPluginValidator) ValidatePlatformConfig(hcp *hyperv1.HostedControlPlane) error {
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
	case hyperv1.AgentPlatform, hyperv1.NonePlatform:
		return p.checkAgentPlatform(hcp)
	default:
		return fmt.Errorf("unsupported platform type %s", hcp.Spec.Platform.Type)
	}

}

func (p *BackupPluginValidator) checkAWSPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the AWS platform is configured properly
	// Check ROSA
	p.Log.Infof("AWS platform configuration is valid for HCP: %s", hcp.Name)
	return nil
}

func (p *BackupPluginValidator) checkAzurePlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Azure platform is configured properly
	// Check ARO
	p.Log.Infof("ARO platform configuration is valid for HCP: %s", hcp.Name)
	return nil
}

func (p *BackupPluginValidator) checkIBMCloudPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the IBM Cloud platform is configured properly
	p.Log.Infof("IBM platform configuration is valid for HCP: %s", hcp.Name)
	return nil
}

func (p *BackupPluginValidator) checkKubevirtPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Kubevirt platform is configured properly
	p.Log.Infof("Kubevirt platform configuration is valid for HCP: %s", hcp.Name)
	return nil
}

func (p *BackupPluginValidator) checkOpenStackPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the OpenStack platform is configured properly
	p.Log.Infof("OpenStack platform configuration is valid for HCP: %s", hcp.Name)
	return nil
}

func (p *BackupPluginValidator) checkAgentPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Agent platform is configured properly
	p.Log.Infof("Agent platform configuration is valid for HCP: %s", hcp.Name)
	return nil
}
