package common

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroapiv2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestGetMetadataAndAnnotations(t *testing.T) {
	tests := []struct {
		name              string
		item              *unstructured.Unstructured
		expectError       bool
		expectAnnotations map[string]string
	}{
		{
			name: "valid metadata with annotations",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
						"annotations": map[string]interface{}{
							"test": "value",
						},
					},
				},
			},
			expectError: false,
			expectAnnotations: map[string]string{
				"test": "value",
			},
		},
		{
			name: "valid metadata without annotations",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			expectError:       false,
			expectAnnotations: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			metadata, annotations, err := getMetadataAndAnnotations(tt.item)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(metadata).NotTo(BeNil())
				g.Expect(annotations).To(Equal(tt.expectAnnotations))
			}
		})
	}
}

func TestManagePauseHostedCluster(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		paused     string
		hcList     *hyperv1.HostedClusterList
		expectErr  bool
	}{
		{
			name:       "Pause HostedCluster",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			hcList: &hyperv1.HostedClusterList{
				Items: []hyperv1.HostedCluster{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-hc",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.HostedClusterSpec{
							PausedUntil: nil,
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:       "Already paused HostedCluster",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			hcList: &hyperv1.HostedClusterList{
				Items: []hyperv1.HostedCluster{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-hc",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.HostedClusterSpec{
							PausedUntil: ptr.To("true"),
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:       "No HostedCluster found",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			hcList:     &hyperv1.HostedClusterList{},
			expectErr:  false,
		},
	}

	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(tt.hcList).Build()
			log := logrus.New()

			err := UpdateHostedCluster(context.TODO(), client, log, tt.paused, tt.namespaces)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				for _, hc := range tt.hcList.Items {
					updatedHC := &hyperv1.HostedCluster{}
					err := client.Get(context.TODO(), types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, updatedHC)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(updatedHC.Spec.PausedUntil).To(Equal(ptr.To(tt.paused)))
					g.Expect(updatedHC.Annotations[HostedClusterRestoredFromBackupAnnotation]).To(BeEmpty())
				}
			}
		})
	}
}

func TestManagePauseNodepools(t *testing.T) {
	tests := []struct {
		name       string
		namespaces []string
		paused     string
		header     string
		npList     *hyperv1.NodePoolList
		expectErr  bool
	}{
		{
			name:       "Pause NodePool",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			header:     "TestHeader",
			npList: &hyperv1.NodePoolList{
				Items: []hyperv1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-np",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.NodePoolSpec{
							PausedUntil: nil,
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:       "Already paused NodePool",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			header:     "TestHeader",
			npList: &hyperv1.NodePoolList{
				Items: []hyperv1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-np",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.NodePoolSpec{
							PausedUntil: ptr.To("true"),
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:       "No NodePool found",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			header:     "TestHeader",
			npList:     &hyperv1.NodePoolList{},
			expectErr:  false,
		},
	}

	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(tt.npList).Build()
			log := logrus.New()

			err := UpdateNodepools(context.TODO(), client, log, tt.paused, tt.namespaces)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				for _, np := range tt.npList.Items {
					updatedNP := &hyperv1.NodePool{}
					err := client.Get(context.TODO(), types.NamespacedName{Name: np.Name, Namespace: np.Namespace}, updatedNP)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(updatedNP.Spec.PausedUntil).To(Equal(ptr.To(tt.paused)))
				}
			}
		})
	}
}

func TestWaitForPausedPropagated(t *testing.T) {
	waitForPausedTimeout := 1 * time.Second

	tests := []struct {
		name      string
		hc        *hyperv1.HostedCluster
		hcp       *hyperv1.HostedControlPlane
		expectErr bool
	}{
		{
			name: "HCP is paused",
			hc: &hyperv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace",
				},
				Spec: hyperv1.HostedClusterSpec{
					PausedUntil: ptr.To("true"),
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
				Spec: hyperv1.HostedControlPlaneSpec{
					PausedUntil: ptr.To("true"),
				},
			},
			expectErr: false,
		},
		{
			name: "HCP is not paused",
			hc: &hyperv1.HostedCluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace",
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
				Spec: hyperv1.HostedControlPlaneSpec{
					PausedUntil: nil,
				},
			},
			expectErr: true,
		},
	}

	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			if tt.hcp != nil {
				clientBuilder = clientBuilder.WithObjects(tt.hcp)
			}
			client := clientBuilder.Build()
			log := logrus.New()

			err := WaitForPausedPropagated(context.TODO(), client, log, tt.hc, waitForPausedTimeout)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
		})
	}
}

func TestMatchSuffixKind(t *testing.T) {
	tests := []struct {
		name     string
		kind     string
		suffixes []string
		expected bool
	}{
		{
			name:     "Match AWS machine suffix",
			kind:     "awsmachines",
			suffixes: []string{"machines", "clusters"},
			expected: true,
		},
		{
			name:     "Match Azure cluster suffix",
			kind:     "azureclusters",
			suffixes: []string{"machines", "clusters"},
			expected: true,
		},
		{
			name:     "No match for random string",
			kind:     "randomstring",
			suffixes: []string{"machines", "clusters"},
			expected: false,
		},
		{
			name:     "Match ROSA cluster suffix",
			kind:     "rosaclusters",
			suffixes: []string{"clusters"},
			expected: true,
		},
		{
			name:     "No match for partial suffix",
			kind:     "aws",
			suffixes: []string{"machines", "clusters"},
			expected: false,
		},
		{
			name:     "Match with multiple suffixes",
			kind:     "testmachines",
			suffixes: []string{"machines", "clusters", "nodes"},
			expected: true,
		},
		{
			name:     "No match with empty suffixes",
			kind:     "testmachines",
			suffixes: []string{},
			expected: false,
		},
		{
			name:     "Match same kind as suffix",
			kind:     "clusters",
			suffixes: []string{"clusters"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			result := MatchSuffixKind(tt.kind, tt.suffixes...)
			g.Expect(result).To(Equal(tt.expected))
		})
	}
}

func TestAddAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		metadata   metav1.Object
		key        string
		value      string
		expectAnno map[string]string
	}{
		{
			name: "add annotation to empty annotations",
			metadata: &metav1.ObjectMeta{
				Name: "test",
			},
			key:   "test-key",
			value: "test-value",
			expectAnno: map[string]string{
				"test-key": "test-value",
			},
		},
		{
			name: "add annotation to existing annotations",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					"existing-key": "existing-value",
				},
			},
			key:   "test-key",
			value: "test-value",
			expectAnno: map[string]string{
				"existing-key": "existing-value",
				"test-key":     "test-value",
			},
		},
		{
			name: "overwrite existing annotation",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					"test-key": "old-value",
				},
			},
			key:   "test-key",
			value: "new-value",
			expectAnno: map[string]string{
				"test-key": "new-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			AddAnnotation(tt.metadata, tt.key, tt.value)
			g.Expect(tt.metadata.GetAnnotations()).To(Equal(tt.expectAnno))
		})
	}
}

func TestRemoveAnnotation(t *testing.T) {
	tests := []struct {
		name       string
		metadata   metav1.Object
		key        string
		expectAnno map[string]string
	}{
		{
			name: "remove existing annotation",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					"test-key": "test-value",
				},
			},
			key:        "test-key",
			expectAnno: map[string]string{},
		},
		{
			name: "remove non-existing annotation",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Annotations: map[string]string{
					"existing-key": "existing-value",
				},
			},
			key: "non-existing-key",
			expectAnno: map[string]string{
				"existing-key": "existing-value",
			},
		},
		{
			name: "remove annotation from empty annotations",
			metadata: &metav1.ObjectMeta{
				Name: "test",
			},
			key:        "test-key",
			expectAnno: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			RemoveAnnotation(tt.metadata, tt.key)
			g.Expect(tt.metadata.GetAnnotations()).To(Equal(tt.expectAnno))
		})
	}
}

func TestGetHCP(t *testing.T) {
	tests := []struct {
		name      string
		nsList    []string
		hcpList   *hyperv1.HostedControlPlaneList
		expectErr bool
	}{
		{
			name:   "HCP found in first namespace",
			nsList: []string{"namespace1", "namespace2"},
			hcpList: &hyperv1.HostedControlPlaneList{
				Items: []hyperv1.HostedControlPlane{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "hcp1",
							Namespace: "namespace1",
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:   "HCP found in second namespace",
			nsList: []string{"namespace1", "namespace2"},
			hcpList: &hyperv1.HostedControlPlaneList{
				Items: []hyperv1.HostedControlPlane{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "hcp2",
							Namespace: "namespace2",
						},
					},
				},
			},
			expectErr: false,
		},
		{
			name:      "No HCP found",
			nsList:    []string{"namespace1", "namespace2"},
			hcpList:   &hyperv1.HostedControlPlaneList{},
			expectErr: true,
		},
		{
			name:   "Error listing HCPs",
			nsList: []string{"namespace1"},
			hcpList: &hyperv1.HostedControlPlaneList{
				Items: []hyperv1.HostedControlPlane{},
			},
			expectErr: true,
		},
	}

	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(tt.hcpList).Build()
			log := logrus.New()

			hcp, err := GetHCP(context.TODO(), tt.nsList, client, log)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(hcp).To(BeNil())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(hcp).NotTo(BeNil())
				g.Expect(hcp.Namespace).To(BeElementOf(tt.nsList))
			}
		})
	}
}

func TestAddLabel(t *testing.T) {
	tests := []struct {
		name      string
		metadata  metav1.Object
		key       string
		value     string
		expectLbl map[string]string
	}{
		{
			name: "add label to empty labels",
			metadata: &metav1.ObjectMeta{
				Name: "test",
			},
			key:   "test-key",
			value: "test-value",
			expectLbl: map[string]string{
				"test-key": "test-value",
			},
		},
		{
			name: "add label to existing labels",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Labels: map[string]string{
					"existing-key": "existing-value",
				},
			},
			key:   "test-key",
			value: "test-value",
			expectLbl: map[string]string{
				"existing-key": "existing-value",
				"test-key":     "test-value",
			},
		},
		{
			name: "overwrite existing label",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Labels: map[string]string{
					"test-key": "old-value",
				},
			},
			key:   "test-key",
			value: "new-value",
			expectLbl: map[string]string{
				"test-key": "new-value",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			AddLabel(tt.metadata, tt.key, tt.value)
			g.Expect(tt.metadata.GetLabels()).To(Equal(tt.expectLbl))
		})
	}
}

func TestRemoveLabel(t *testing.T) {
	tests := []struct {
		name      string
		metadata  metav1.Object
		key       string
		expectLbl map[string]string
	}{
		{
			name: "remove existing label",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Labels: map[string]string{
					"test-key": "test-value",
				},
			},
			key:       "test-key",
			expectLbl: map[string]string{},
		},
		{
			name: "remove non-existing label",
			metadata: &metav1.ObjectMeta{
				Name: "test",
				Labels: map[string]string{
					"existing-key": "existing-value",
				},
			},
			key: "non-existing-key",
			expectLbl: map[string]string{
				"existing-key": "existing-value",
			},
		},
		{
			name: "remove label from empty labels",
			metadata: &metav1.ObjectMeta{
				Name: "test",
			},
			key:       "test-key",
			expectLbl: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			RemoveLabel(tt.metadata, tt.key)
			g.Expect(tt.metadata.GetLabels()).To(Equal(tt.expectLbl))
		})
	}
}

func TestGetCurrentNamespace(t *testing.T) {
	tests := []struct {
		name          string
		fileContent   string
		expectError   bool
		expectedValue string
	}{
		{
			name:          "valid namespace file",
			fileContent:   "test-namespace",
			expectError:   false,
			expectedValue: "test-namespace",
		},
		{
			name:          "empty namespace file",
			fileContent:   "",
			expectError:   true,
			expectedValue: "",
		},
		{
			name:        "namespace file does not exist",
			fileContent: "",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			// Create a temporary directory to simulate the service account file path
			tempDir := t.TempDir()
			k8sSAFilePath = tempDir

			// Create the namespace file if fileContent is provided
			if tt.fileContent != "" {
				namespaceFilePath := filepath.Join(tempDir, "namespace")
				err := os.WriteFile(namespaceFilePath, []byte(tt.fileContent), 0644)
				g.Expect(err).NotTo(HaveOccurred())
			}

			// Call the function
			namespace, err := GetCurrentNamespace()

			// Validate the results
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(namespace).To(Equal(tt.expectedValue))
			}
		})
	}
}

type fakeClient struct {
	crclient.Client
	deletedPods map[string]bool
}

func (f *fakeClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	f.deletedPods[obj.GetName()] = true
	return nil
}

func TestShouldEndPluginExecution(t *testing.T) {
	tests := []struct {
		name               string
		objects            []client.Object
		includedNamespaces []string
		includedResources  []string
		expectedResult     bool
	}{
		{
			name: "CRD exists",
			objects: []client.Object{
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hostedcontrolplanes.hypershift.openshift.io",
					},
				},
			},
			includedNamespaces: []string{"test-namespace"},
			includedResources:  []string{"hostedcontrolplanes", "hostedclusters"},
			expectedResult:     false,
		},
		{
			name:               "CRD does not exist",
			objects:            []client.Object{},
			includedNamespaces: []string{"test-namespace"},
			includedResources:  []string{"hostedcontrolplanes", "hostedclusters"},
			expectedResult:     true,
		},
		{
			name:               "No namespaces provided",
			objects:            []client.Object{},
			includedNamespaces: []string{},
			includedResources:  []string{"hostedcontrolplanes", "hostedclusters"},
			expectedResult:     true,
		},
		{
			name:               "No resources provided",
			objects:            []client.Object{},
			includedNamespaces: []string{"test-namespace"},
			includedResources:  []string{},
			expectedResult:     true,
		},
	}

	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			log := logrus.New()

			result := ShouldEndPluginExecution(context.TODO(), &veleroapiv1.Backup{
				Spec: veleroapiv1.BackupSpec{
					IncludedNamespaces: tt.includedNamespaces,
					IncludedResources:  tt.includedResources,
				},
			}, c, log)
			g.Expect(result).To(Equal(tt.expectedResult))
		})
	}
}

func TestCRDExists(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)
	_ = apiextensionsv1.AddToScheme(scheme)

	tests := []struct {
		name           string
		objects        []client.Object
		expectedResult bool
	}{
		{
			name: "CRD exists",
			objects: []client.Object{
				&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{
						Name: "hostedcontrolplanes.hypershift.openshift.io",
					},
				},
			},
			expectedResult: true,
		},
		{
			name:           "CRD does not exist",
			objects:        []client.Object{},
			expectedResult: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()
			result, err := CRDExists(context.TODO(), "hostedcontrolplanes.hypershift.openshift.io", c)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(result).To(Equal(tt.expectedResult))
		})
	}
}

func TestWaitForPodVolumeBackup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)

	tests := []struct {
		name                     string
		backup                   *veleroapiv1.Backup
		podVolumeBackups         []veleroapiv1.PodVolumeBackup
		ha                       bool
		podVolumeBackupTimeout   time.Duration
		podVolumeBackupCheckPace time.Duration
		expectSuccess            bool
		expectError              bool
	}{
		{
			name: "Single node backup completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
			},
			ha:                       false,
			podVolumeBackupTimeout:   5 * time.Second,
			podVolumeBackupCheckPace: 100 * time.Millisecond,
			expectSuccess:            true,
			expectError:              false,
		},
		{
			name: "HA backup completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-1",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-2",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-2",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-3",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-3",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
			},
			ha:                       true,
			podVolumeBackupTimeout:   5 * time.Second,
			podVolumeBackupCheckPace: 100 * time.Millisecond,
			expectSuccess:            true,
			expectError:              false,
		},
		{
			name: "PodVolumeBackup failed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseFailed,
					},
				},
			},
			ha:                       false,
			podVolumeBackupTimeout:   5 * time.Second,
			podVolumeBackupCheckPace: 100 * time.Millisecond,
			expectSuccess:            false,
			expectError:              true,
		},
		{
			name: "Timeout waiting for PodVolumeBackup",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseInProgress,
					},
				},
			},
			ha:                       false,
			podVolumeBackupTimeout:   200 * time.Millisecond,
			podVolumeBackupCheckPace: 50 * time.Millisecond,
			expectSuccess:            false,
			expectError:              true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&veleroapiv1.PodVolumeBackupList{
				Items: tt.podVolumeBackups,
			}).Build()
			log := logrus.New()

			success, err := WaitForPodVolumeBackup(context.TODO(), client, log, tt.backup, tt.podVolumeBackupTimeout, tt.podVolumeBackupCheckPace, tt.ha)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(success).To(Equal(tt.expectSuccess))
		})
	}
}

func TestCheckPodVolumeBackup(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)

	tests := []struct {
		name             string
		backup           *veleroapiv1.Backup
		podVolumeBackups []veleroapiv1.PodVolumeBackup
		ha               bool
		expectStarted    bool
		expectFinished   bool
		expectError      bool
	}{
		{
			name: "Single node backup not started",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{},
			ha:               false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "Single node backup in progress",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseInProgress,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: false,
			expectError:    false,
		},
		{
			name: "Single node backup completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: true,
			expectError:    false,
		},
		{
			name: "HA backup completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-1",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-2",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-2",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-3",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-3",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
			},
			ha:             true,
			expectStarted:  true,
			expectFinished: true,
			expectError:    false,
		},
		{
			name: "PodVolumeBackup failed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseFailed,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: false,
			expectError:    true,
		},
		{
			name: "HA backup waiting for more PodVolumeBackups",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			podVolumeBackups: []veleroapiv1.PodVolumeBackup{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-pvb-1",
						Namespace: "velero",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Spec: veleroapiv1.PodVolumeBackupSpec{
						Volume: "data",
						Pod: corev1.ObjectReference{
							Name: "test-pod-1",
						},
					},
					Status: veleroapiv1.PodVolumeBackupStatus{
						Phase: veleroapiv1.PodVolumeBackupPhaseCompleted,
					},
				},
			},
			ha:             true,
			expectStarted:  false,
			expectFinished: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&veleroapiv1.PodVolumeBackupList{
				Items: tt.podVolumeBackups,
			}).Build()
			log := logrus.New()

			started, finished, err := CheckPodVolumeBackup(context.TODO(), client, log, tt.backup, tt.ha)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(started).To(Equal(tt.expectStarted))
			g.Expect(finished).To(Equal(tt.expectFinished))
		})
	}
}

func TestWaitForDataUpload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv2alpha1.AddToScheme(scheme)

	tests := []struct {
		name                string
		backup              *veleroapiv1.Backup
		dataUploads         []veleroapiv2alpha1.DataUpload
		ha                  bool
		dataUploadTimeout   time.Duration
		dataUploadCheckPace time.Duration
		expectSuccess       bool
		expectError         bool
	}{
		{
			name: "Single node data upload completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
			},
			ha:                  false,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       true,
			expectError:         false,
		},
		{
			name: "HA data upload completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-2",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-3",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
			},
			ha:                  true,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       true,
			expectError:         false,
		},
		{
			name: "Data upload failed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseFailed,
					},
				},
			},
			ha:                  false,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       false,
			expectError:         true,
		},
		{
			name: "Timeout waiting for data upload",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseInProgress,
					},
				},
			},
			ha:                  false,
			dataUploadTimeout:   200 * time.Millisecond,
			dataUploadCheckPace: 50 * time.Millisecond,
			expectSuccess:       false,
			expectError:         true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&veleroapiv2alpha1.DataUploadList{
				Items: tt.dataUploads,
			}).Build()
			log := logrus.New()

			success, err := WaitForDataUpload(context.TODO(), client, log, tt.backup, tt.dataUploadTimeout, tt.dataUploadCheckPace, tt.ha)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(success).To(Equal(tt.expectSuccess))
		})
	}
}

func TestCheckDataUpload(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv2alpha1.AddToScheme(scheme)

	tests := []struct {
		name           string
		backup         *veleroapiv1.Backup
		dataUploads    []veleroapiv2alpha1.DataUpload
		ha             bool
		expectStarted  bool
		expectFinished bool
		expectError    bool
	}{
		{
			name: "Single node data upload not started",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads:    []veleroapiv2alpha1.DataUpload{},
			ha:             false,
			expectStarted:  false,
			expectFinished: false,
			expectError:    false,
		},
		{
			name: "Single node data upload in progress",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseInProgress,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: false,
			expectError:    false,
		},
		{
			name: "Single node data upload completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: true,
			expectError:    false,
		},
		{
			name: "HA data upload completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-2",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-3",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
			},
			ha:             true,
			expectStarted:  true,
			expectFinished: true,
			expectError:    false,
		},
		{
			name: "Data upload failed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseFailed,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: false,
			expectError:    true,
		},
		{
			name: "HA data upload waiting for more uploads",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "test-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
			},
			ha:             true,
			expectStarted:  false,
			expectFinished: false,
			expectError:    false,
		},
		{
			name: "Data upload with different generate name",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			dataUploads: []veleroapiv2alpha1.DataUpload{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:         "test-du-1",
						GenerateName: "different-backup-",
						Namespace:    "velero",
					},
					Status: veleroapiv2alpha1.DataUploadStatus{
						Phase: veleroapiv2alpha1.DataUploadPhaseCompleted,
					},
				},
			},
			ha:             false,
			expectStarted:  true,
			expectFinished: false,
			expectError:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&veleroapiv2alpha1.DataUploadList{
				Items: tt.dataUploads,
			}).Build()
			log := logrus.New()

			started, finished, err := CheckDataUpload(context.TODO(), client, log, tt.backup, tt.ha)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(started).To(Equal(tt.expectStarted))
			g.Expect(finished).To(Equal(tt.expectFinished))
		})
	}
}

func TestWaitForVolumeSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name             string
		backup           *veleroapiv1.Backup
		volumeSnapshots  []snapshotv1.VolumeSnapshot
		hcp              *hyperv1.HostedControlPlane
		ha               bool
		pvBackupStarted  bool
		pvBackupFinished bool
		vsTimeout        time.Duration
		vsCheckPace      time.Duration
		expectSuccess    bool
		expectError      bool
	}{
		{
			name: "Single node volume snapshot completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: false,
			vsTimeout:        5 * time.Second,
			vsCheckPace:      100 * time.Millisecond,
			expectSuccess:    true,
			expectError:      false,
		},
		{
			name: "HA volume snapshot completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-2",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-3",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               true,
			pvBackupStarted:  true,
			pvBackupFinished: false,
			vsTimeout:        5 * time.Second,
			vsCheckPace:      100 * time.Millisecond,
			expectSuccess:    true,
			expectError:      false,
		},
		{
			name: "Volume snapshot not ready",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(false),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: false,
			vsTimeout:        200 * time.Millisecond,
			vsCheckPace:      50 * time.Millisecond,
			expectSuccess:    false,
			expectError:      true,
		},
		{
			name: "Already finished",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: true,
			vsTimeout:        5 * time.Second,
			vsCheckPace:      100 * time.Millisecond,
			expectSuccess:    true,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&snapshotv1.VolumeSnapshotList{
				Items: tt.volumeSnapshots,
			}).Build()
			log := logrus.New()

			success, err := WaitForVolumeSnapshot(context.TODO(), client, log, tt.backup, tt.vsTimeout, tt.vsCheckPace, tt.ha, tt.hcp, &tt.pvBackupStarted, &tt.pvBackupFinished)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(success).To(Equal(tt.expectSuccess))
		})
	}
}

func TestCheckVolumeSnapshot(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name             string
		backup           *veleroapiv1.Backup
		volumeSnapshots  []snapshotv1.VolumeSnapshot
		hcp              *hyperv1.HostedControlPlane
		ha               bool
		pvBackupStarted  bool
		pvBackupFinished bool
		expectStarted    bool
		expectFinished   bool
		expectError      bool
	}{
		{
			name: "Single node volume snapshot not started",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "Single node volume snapshot in progress",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(false),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    true,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "Single node volume snapshot completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    true,
			expectFinished:   true,
			expectError:      false,
		},
		{
			name: "HA volume snapshot completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-2",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-3",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               true,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    true,
			expectFinished:   true,
			expectError:      false,
		},
		{
			name: "Already finished",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: true,
			expectStarted:    true,
			expectFinished:   true,
			expectError:      false,
		},

		{
			name: "Empty HCP namespace",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&snapshotv1.VolumeSnapshotList{
				Items: tt.volumeSnapshots,
			}).Build()
			log := logrus.New()

			started, finished, err := CheckVolumeSnapshot(context.TODO(), client, log, tt.backup, tt.ha, tt.hcp, &tt.pvBackupStarted, &tt.pvBackupFinished)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(started).To(Equal(tt.expectStarted))
			g.Expect(finished).To(Equal(tt.expectFinished))
		})
	}
}

func TestWaitForVolumeSnapshotContent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name                   string
		backup                 *veleroapiv1.Backup
		volumeSnapshotContents []snapshotv1.VolumeSnapshotContent
		hcp                    *hyperv1.HostedControlPlane
		ha                     bool
		pvBackupStarted        bool
		pvBackupFinished       bool
		vscTimeout             time.Duration
		vscCheckPace           time.Duration
		expectSuccess          bool
		expectError            bool
	}{
		{
			name: "Single node volume snapshot content completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: false,
			vscTimeout:       5 * time.Second,
			vscCheckPace:     100 * time.Millisecond,
			expectSuccess:    true,
			expectError:      false,
		},
		{
			name: "HA volume snapshot content completed successfully",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-2",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-3",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               true,
			pvBackupStarted:  true,
			pvBackupFinished: false,
			vscTimeout:       5 * time.Second,
			vscCheckPace:     100 * time.Millisecond,
			expectSuccess:    true,
			expectError:      false,
		},
		{
			name: "Volume snapshot content not ready",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(false),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: false,
			vscTimeout:       200 * time.Millisecond,
			vscCheckPace:     50 * time.Millisecond,
			expectSuccess:    false,
			expectError:      true,
		},
		{
			name: "Already finished",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: true,
			vscTimeout:       5 * time.Second,
			vscCheckPace:     100 * time.Millisecond,
			expectSuccess:    true,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&snapshotv1.VolumeSnapshotContentList{
				Items: tt.volumeSnapshotContents,
			}).Build()
			log := logrus.New()

			success, err := WaitForVolumeSnapshotContent(context.TODO(), client, log, tt.backup, tt.vscTimeout, tt.vscCheckPace, tt.ha, tt.hcp, &tt.pvBackupStarted, &tt.pvBackupFinished)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(success).To(Equal(tt.expectSuccess))
		})
	}
}

func TestCheckVolumeSnapshotContent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name                   string
		backup                 *veleroapiv1.Backup
		volumeSnapshotContents []snapshotv1.VolumeSnapshotContent
		hcp                    *hyperv1.HostedControlPlane
		ha                     bool
		pvBackupStarted        bool
		pvBackupFinished       bool
		expectStarted          bool
		expectFinished         bool
		expectError            bool
	}{
		{
			name: "Single node volume snapshot content not started",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "Single node volume snapshot content in progress",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(false),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    true,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "Single node volume snapshot content completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    true,
			expectFinished:   true,
			expectError:      false,
		},
		{
			name: "HA volume snapshot content completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-2",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-3",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               true,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    true,
			expectFinished:   true,
			expectError:      false,
		},
		{
			name: "Already finished",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  true,
			pvBackupFinished: true,
			expectStarted:    true,
			expectFinished:   true,
			expectError:      false,
		},
		{
			name: "Volume snapshot content with different namespace",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "different-namespace",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "Empty HCP namespace",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
		{
			name: "VolumeSnapshotContent with empty VolumeSnapshotRef namespace",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:               false,
			pvBackupStarted:  false,
			pvBackupFinished: false,
			expectStarted:    false,
			expectFinished:   false,
			expectError:      false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&snapshotv1.VolumeSnapshotContentList{
				Items: tt.volumeSnapshotContents,
			}).Build()
			log := logrus.New()

			started, finished, err := CheckVolumeSnapshotContent(context.TODO(), client, log, tt.backup, tt.ha, tt.hcp, &tt.pvBackupStarted, &tt.pvBackupFinished)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(started).To(Equal(tt.expectStarted))
			g.Expect(finished).To(Equal(tt.expectFinished))
		})
	}
}

func TestReconcileVolumeSnapshotContent(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name                   string
		backup                 *veleroapiv1.Backup
		volumeSnapshotContents []snapshotv1.VolumeSnapshotContent
		hcp                    *hyperv1.HostedControlPlane
		ha                     bool
		pvBackupStarted        bool
		pvBackupFinished       bool
		dataUploadTimeout      time.Duration
		dataUploadCheckPace    time.Duration
		expectSuccess          bool
		expectError            bool
	}{
		{
			name: "Already finished",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:                  false,
			pvBackupStarted:     true,
			pvBackupFinished:    true,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       true,
			expectError:         false,
		},
		{
			name: "Volume snapshot content completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshotContents: []snapshotv1.VolumeSnapshotContent{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-vsc-1",
					},
					Spec: snapshotv1.VolumeSnapshotContentSpec{
						VolumeSnapshotRef: corev1.ObjectReference{
							Namespace: "test-namespace-test-hc",
						},
					},
					Status: &snapshotv1.VolumeSnapshotContentStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:                  false,
			pvBackupStarted:     false,
			pvBackupFinished:    false,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       true,
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&snapshotv1.VolumeSnapshotContentList{
				Items: tt.volumeSnapshotContents,
			}).Build()
			log := logrus.New()

			success, err := ReconcileVolumeSnapshotContent(context.TODO(), tt.hcp, client, log, tt.backup, tt.ha, tt.dataUploadTimeout, tt.dataUploadCheckPace, &tt.pvBackupStarted, &tt.pvBackupFinished)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(success).To(Equal(tt.expectSuccess))
		})
	}
}

func TestReconcileVolumeSnapshots(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = veleroapiv1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name                string
		backup              *veleroapiv1.Backup
		volumeSnapshots     []snapshotv1.VolumeSnapshot
		hcp                 *hyperv1.HostedControlPlane
		ha                  bool
		pvBackupStarted     bool
		pvBackupFinished    bool
		dataUploadTimeout   time.Duration
		dataUploadCheckPace time.Duration
		expectSuccess       bool
		expectError         bool
	}{
		{
			name: "Already finished",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:                  false,
			pvBackupStarted:     true,
			pvBackupFinished:    true,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       true,
			expectError:         false,
		},
		{
			name: "Volume snapshot completed",
			backup: &veleroapiv1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "velero",
				},
			},
			volumeSnapshots: []snapshotv1.VolumeSnapshot{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-vs-1",
						Namespace: "test-namespace-test-hc",
						Labels: map[string]string{
							veleroapiv1.BackupNameLabel: "test-backup",
						},
					},
					Status: &snapshotv1.VolumeSnapshotStatus{
						ReadyToUse: ptr.To(true),
					},
				},
			},
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hc",
					Namespace: "test-namespace-test-hc",
				},
			},
			ha:                  false,
			pvBackupStarted:     false,
			pvBackupFinished:    false,
			dataUploadTimeout:   5 * time.Second,
			dataUploadCheckPace: 100 * time.Millisecond,
			expectSuccess:       true,
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(&snapshotv1.VolumeSnapshotList{
				Items: tt.volumeSnapshots,
			}).Build()
			log := logrus.New()

			success, err := ReconcileVolumeSnapshots(context.TODO(), tt.hcp, client, log, tt.backup, tt.ha, tt.dataUploadTimeout, tt.dataUploadCheckPace, &tt.pvBackupStarted, &tt.pvBackupFinished)

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
			}
			g.Expect(success).To(Equal(tt.expectSuccess))
		})
	}
}
