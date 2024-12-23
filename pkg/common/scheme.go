package common

import (
	configv1 "github.com/openshift/api/config/v1"
	machinev1beta1 "github.com/openshift/api/machine/v1beta1"
	agentv1 "github.com/openshift/cluster-api-provider-agent/api/v1beta1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	oadpv1alpha2 "github.com/openshift/oadp-operator/api/v1alpha1"
	velero1alpha2 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	apiserverconfigv1 "k8s.io/apiserver/pkg/apis/apiserver/v1"
	kasv1beta1 "k8s.io/apiserver/pkg/apis/apiserver/v1beta1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	capipowervs "sigs.k8s.io/cluster-api-provider-ibmcloud/api/v1beta2"
	capiopenstackv1alpha1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1alpha1"
	capiopenstackv1beta1 "sigs.k8s.io/cluster-api-provider-openstack/api/v1beta1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	CustomScheme = runtime.NewScheme()
)

func init() {
	capiv1.AddToScheme(CustomScheme)
	apiregistrationv1.AddToScheme(CustomScheme)
	hyperv1beta1.AddToScheme(CustomScheme)
	configv1.AddToScheme(CustomScheme)
	kasv1beta1.AddToScheme(CustomScheme)
	apiserverconfigv1.AddToScheme(CustomScheme)
	velero1alpha2.AddToScheme(CustomScheme)
	oadpv1alpha2.AddToScheme(CustomScheme)
	agentv1.AddToScheme(CustomScheme)
	capipowervs.AddToScheme(CustomScheme)
	machinev1beta1.AddToScheme(CustomScheme)
	capiopenstackv1alpha1.AddToScheme(CustomScheme)
	capiopenstackv1beta1.AddToScheme(CustomScheme)
}
