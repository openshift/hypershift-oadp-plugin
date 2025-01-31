package common

import (
	hive "github.com/openshift/hive/apis/hive/v1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	veleroapiv2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	CustomScheme = runtime.NewScheme()
)

func init() {
	hyperv1beta1.AddToScheme(CustomScheme)
	corev1.AddToScheme(CustomScheme)
	veleroapiv2alpha1.AddToScheme(CustomScheme)
	hive.AddToScheme(CustomScheme)
}
