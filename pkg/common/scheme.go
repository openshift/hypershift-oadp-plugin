package common

import (
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	CustomScheme = runtime.NewScheme()
)

func init() {
	hyperv1beta1.AddToScheme(CustomScheme)
}
