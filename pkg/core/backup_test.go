package core

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newTestBackupPlugin(t *testing.T, hcp *hyperv1.HostedControlPlane) *BackupPlugin {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)
	_ = velerov1.AddToScheme(scheme)

	objects := []runtime.Object{}
	if hcp != nil {
		objects = append(objects, hcp)
	}
	objects = append(objects, &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: "hostedcontrolplanes.hypershift.openshift.io",
		},
	})

	client := fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objects...).Build()
	logger := logrus.New()

	return &BackupPlugin{
		log:    logger,
		ctx:    context.Background(),
		client: client,
		config: map[string]string{},
		validator: &validation.BackupPluginValidator{
			Log:    logger,
			Client: client,
		},
		hcp:           hcp,
		BackupOptions: &plugtypes.BackupOptions{},
	}
}

func TestExecute_PVCAndDataVolume(t *testing.T) {
	hcp := &hyperv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "clusters-test",
		},
		Spec: hyperv1.HostedControlPlaneSpec{
			Platform: hyperv1.PlatformSpec{
				Type: hyperv1.KubevirtPlatform,
			},
		},
	}

	tests := []struct {
		name           string
		item           *unstructured.Unstructured
		gvk            schema.GroupVersionKind
		expectExcluded bool
	}{
		{
			name: "when PVC has RHCOS label should be excluded from backup",
			item: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "PersistentVolumeClaim",
					"metadata": map[string]any{
						"name":      "test-pvc",
						"namespace": "clusters-test",
						"labels": map[string]any{
							common.KubevirtRHCOSLabel: "true",
						},
					},
				},
			},
			gvk:            schema.GroupVersionKind{Kind: "PersistentVolumeClaim"},
			expectExcluded: true,
		},
		{
			name: "when PVC is owned by DataVolume should be excluded from backup",
			item: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "PersistentVolumeClaim",
					"metadata": map[string]any{
						"name":      "test-vm-disk-pvc",
						"namespace": "clusters-test",
						"ownerReferences": []any{
							map[string]any{
								"apiVersion": "cdi.kubevirt.io/v1beta1",
								"kind":       "DataVolume",
								"name":       "test-vm-disk",
								"uid":        "test-uid",
							},
						},
					},
				},
			},
			gvk:            schema.GroupVersionKind{Kind: "PersistentVolumeClaim"},
			expectExcluded: true,
		},
		{
			name: "when PVC has CDI populated-for annotation should be excluded from backup",
			item: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "PersistentVolumeClaim",
					"metadata": map[string]any{
						"name":      "test-vm-disk-pvc",
						"namespace": "clusters-test",
						"annotations": map[string]any{
							common.CDIPopulatedForAnnotation: "test-vm-disk",
						},
					},
				},
			},
			gvk:            schema.GroupVersionKind{Kind: "PersistentVolumeClaim"},
			expectExcluded: true,
		},
		{
			name: "when PVC is a regular volume like etcd should be included in backup",
			item: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "PersistentVolumeClaim",
					"metadata": map[string]any{
						"name":      "etcd-data-0",
						"namespace": "clusters-test",
					},
				},
			},
			gvk:            schema.GroupVersionKind{Kind: "PersistentVolumeClaim"},
			expectExcluded: false,
		},
		{
			name: "when DataVolume has RHCOS label should be excluded from backup",
			item: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataVolume",
					"metadata": map[string]any{
						"name":      "test-datavolume",
						"namespace": "clusters-test",
						"labels": map[string]any{
							common.KubevirtRHCOSLabel: "true",
						},
					},
				},
			},
			gvk:            schema.GroupVersionKind{Kind: "DataVolume"},
			expectExcluded: true,
		},
		{
			name: "when DataVolume has no RHCOS label should be included in backup",
			item: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "cdi.kubevirt.io/v1beta1",
					"kind":       "DataVolume",
					"metadata": map[string]any{
						"name":      "test-datavolume",
						"namespace": "clusters-test",
					},
				},
			},
			gvk:            schema.GroupVersionKind{Kind: "DataVolume"},
			expectExcluded: false,
		},
	}

	backup := &velerov1.Backup{
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{"clusters", "clusters-test"},
			IncludedResources:  []string{"hostedcontrolplanes", "hostedclusters", "persistentvolumeclaims", "datavolumes"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			plugin := newTestBackupPlugin(t, hcp)
			tt.item.SetGroupVersionKind(tt.gvk)

			result, _, err := plugin.Execute(tt.item, backup)
			g.Expect(err).NotTo(HaveOccurred())
			if tt.expectExcluded {
				g.Expect(result).To(BeNil(), "item should be excluded from backup")
			} else {
				g.Expect(result).NotTo(BeNil(), "item should be included in backup")
			}
		})
	}
}
