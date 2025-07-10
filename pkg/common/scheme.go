package common

import (
	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	hive "github.com/openshift/hive/apis/hive/v1"
	hyperv1beta1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroapiv2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var (
	CustomScheme = runtime.NewScheme()
)

func init() {
	var errs []error

	if err := hyperv1beta1.AddToScheme(CustomScheme); err != nil {
		errs = append(errs, err)
	}
	if err := corev1.AddToScheme(CustomScheme); err != nil {
		errs = append(errs, err)
	}
	if err := veleroapiv2alpha1.AddToScheme(CustomScheme); err != nil {
		errs = append(errs, err)
	}
	if err := veleroapiv1.AddToScheme(CustomScheme); err != nil {
		errs = append(errs, err)
	}
	if err := snapshotv1.AddToScheme(CustomScheme); err != nil {
		errs = append(errs, err)
	}
	if err := hive.AddToScheme(CustomScheme); err != nil {
		errs = append(errs, err)
	}

	if len(errs) > 0 {
		panic(errs)
	}
}
