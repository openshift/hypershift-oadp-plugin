package common

import (
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	CustomScheme = runtime.NewScheme()
)

func init() {
	hyperv1beta1.AddToScheme(CustomScheme)
	corev1.AddToScheme(CustomScheme)
}
