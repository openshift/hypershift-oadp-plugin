package common

import (
	configv1 "github.com/openshift/api/config/v1"
	imageregistryv1 "github.com/openshift/api/imageregistry/v1"
	oauthv1 "github.com/openshift/api/oauth/v1"
	openshiftcpv1 "github.com/openshift/api/openshiftcontrolplane/v1"
	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/api/operator/v1alpha1"
	osinv1 "github.com/openshift/api/osin/v1"
	routev1 "github.com/openshift/api/route/v1"
	securityv1 "github.com/openshift/api/security/v1"
	certificatesv1alpha1 "github.com/openshift/hypershift/api/certificates/v1alpha1"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	schedulingv1alpha1 "github.com/openshift/hypershift/api/scheduling/v1alpha1"
	operatorsv1alpha1 "github.com/operator-framework/api/pkg/operators/v1alpha1"
	prometheusoperatorv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	velero "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	apiserverconfigv1 "k8s.io/apiserver/pkg/apis/apiserver/v1"
	kasv1beta1 "k8s.io/apiserver/pkg/apis/apiserver/v1beta1"
	auditv1 "k8s.io/apiserver/pkg/apis/audit/v1"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	capiv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

var (
	CustomScheme = runtime.NewScheme()
)

func init() {
	capiv1.AddToScheme(CustomScheme)
	clientgoscheme.AddToScheme(CustomScheme)
	auditv1.AddToScheme(CustomScheme)
	apiregistrationv1.AddToScheme(CustomScheme)
	hyperv1.AddToScheme(CustomScheme)
	schedulingv1alpha1.AddToScheme(CustomScheme)
	certificatesv1alpha1.AddToScheme(CustomScheme)
	configv1.AddToScheme(CustomScheme)
	securityv1.AddToScheme(CustomScheme)
	operatorv1.AddToScheme(CustomScheme)
	oauthv1.AddToScheme(CustomScheme)
	osinv1.AddToScheme(CustomScheme)
	routev1.AddToScheme(CustomScheme)
	rbacv1.AddToScheme(CustomScheme)
	corev1.AddToScheme(CustomScheme)
	apiextensionsv1.AddToScheme(CustomScheme)
	kasv1beta1.AddToScheme(CustomScheme)
	openshiftcpv1.AddToScheme(CustomScheme)
	v1alpha1.AddToScheme(CustomScheme)
	apiserverconfigv1.AddToScheme(CustomScheme)
	prometheusoperatorv1.AddToScheme(CustomScheme)
	imageregistryv1.AddToScheme(CustomScheme)
	operatorsv1alpha1.AddToScheme(CustomScheme)
	velero.AddToScheme(CustomScheme)

}
