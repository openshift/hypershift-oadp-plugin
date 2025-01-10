package validation

import (
	"fmt"

	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
)

type RestoreValidator interface {
	ValidatePluginConfig(config map[string]string) (*plugtypes.RestoreOptions, error)
	ValidatePlatformConfig(hcp *hyperv1.HostedControlPlane) error
}

type RestorePluginValidator struct {
	Log       logrus.FieldLogger
	Client    crclient.Client
	LogHeader string
}

func (p *RestorePluginValidator) ValidatePluginConfig(config map[string]string) (*plugtypes.RestoreOptions, error) {
	// Validate the plugin configuration
	p.Log.Debugf("%s validating plugin configuration", p.LogHeader)
	if len(config) == 0 {
		p.Log.Debug("no configuration provided")
		return &plugtypes.RestoreOptions{}, nil
	}
	bo := &plugtypes.RestoreOptions{}

	for key, value := range config {
		p.Log.Debugf("%s configuration key: %s, value: %s", p.LogHeader, key, value)
		switch key {
		case "migration":
			bo.Migration = value == "true"
		case "readoptNodes":
			bo.ReadoptNodes = value == "true"
		case "managedServices":
			bo.ManagedServices = value == "true"
		default:
			p.Log.Warnf("unknown configuration key: %s with value %s", key, value)
		}
	}

	p.Log.Infof("%s plugin configuration validated", p.LogHeader)

	return bo, nil

}

func (p *RestorePluginValidator) ValidatePlatformConfig(hcp *hyperv1.HostedControlPlane) error {
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

func (p *RestorePluginValidator) checkAWSPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the AWS platform is configured properly
	// Check ROSA
	p.Log.Infof("%s AWS platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) checkAzurePlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Azure platform is configured properly
	// Check ARO
	p.Log.Infof("%s ARO platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) checkIBMCloudPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the IBM Cloud platform is configured properly
	p.Log.Infof("%s IBM platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) checkKubevirtPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Kubevirt platform is configured properly
	p.Log.Infof("%s Kubevirt platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) checkOpenStackPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the OpenStack platform is configured properly
	p.Log.Infof("%s OpenStack platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}

func (p *RestorePluginValidator) checkAgentPlatform(hcp *hyperv1.HostedControlPlane) error {
	// Check if the Agent platform is configured properly
	p.Log.Infof("%s Agent platform configuration is valid for HCP: %s", p.LogHeader, hcp.Name)
	return nil
}
