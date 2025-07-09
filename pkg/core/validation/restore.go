package validation

import (
	"fmt"

	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	aws "github.com/openshift/hypershift-oadp-plugin/pkg/platform/aws"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type RestoreValidator interface {
	ValidatePluginConfig(config map[string]string) (*plugtypes.RestoreOptions, error)
	ValidatePlatformConfig(hcp *hyperv1.HostedControlPlane, config map[string]string) error
}

type RestorePluginValidator struct {
	Log       logrus.FieldLogger
	Client    crclient.Client
	LogHeader string
}

func (p *RestorePluginValidator) ValidatePluginConfig(config map[string]string) (*plugtypes.RestoreOptions, error) {
	// Validate the plugin configuration
	p.Log.Debug("validating plugin configuration")
	if len(config) == 0 {
		p.Log.Debug("no configuration provided")
		return &plugtypes.RestoreOptions{}, nil
	}
	bo := &plugtypes.RestoreOptions{}

	for key, value := range config {
		p.Log.Debugf("%s configuration key: %s, value: %s", p.LogHeader, key, value)
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
		default:
			p.Log.Warnf("unknown configuration key: %s with value %s", key, value)
		}
	}

	p.Log.Infof("%s plugin configuration validated", p.LogHeader)

	return bo, nil

}

func (p *RestorePluginValidator) ValidatePlatformConfig(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	switch hcp.Spec.Platform.Type {
	case hyperv1.AWSPlatform:
		return p.validateAWSPlatform(hcp, config)
	case hyperv1.AzurePlatform:
		return p.validateAzurePlatform(hcp, config)
	case hyperv1.IBMCloudPlatform:
		return p.validateIBMCloudPlatform(hcp, config)
	case hyperv1.KubevirtPlatform:
		return p.validateKubevirtPlatform(hcp, config)
	case hyperv1.OpenStackPlatform:
		return p.validateOpenStackPlatform(hcp, config)
	case hyperv1.AgentPlatform, hyperv1.NonePlatform:
		return p.validateAgentPlatform(hcp, config)
	default:
		return fmt.Errorf("unsupported platform type %s", hcp.Spec.Platform.Type)
	}

}

func (p *RestorePluginValidator) validateAWSPlatform(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	// Validate if the AWS platform is configured properly
	// Validate ROSA
	p.Log.Infof("%s AWS platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)

	if config["managedServices"] == "true" || config["awsRegenPrivateLink"] == "true" {
		p.Log.Infof("%s AWS platform restore tasks for HCP: %s", p.LogHeader, hcp.Name)
		if err := aws.RestoreTasks(hcp, p.Client); err != nil {
			return fmt.Errorf("error executing ROSA platform restore tasks: %s", err.Error())
		}
	}
	return nil
}

func (p *RestorePluginValidator) validateAzurePlatform(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	// Validate if the Azure platform is configured properly
	// Validate ARO
	p.Log.Infof("%s ARO platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) validateIBMCloudPlatform(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	// Validate if the IBM Cloud platform is configured properly
	p.Log.Infof("%s IBM platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) validateKubevirtPlatform(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	// Validate if the Kubevirt platform is configured properly
	p.Log.Infof("%s Kubevirt platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) validateOpenStackPlatform(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	// Validate if the OpenStack platform is configured properly
	p.Log.Infof("%s OpenStack platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) validateAgentPlatform(hcp *hyperv1.HostedControlPlane, config map[string]string) error {
	// Validate if the Agent platform is configured properly
	p.Log.Infof("%s Agent platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}
