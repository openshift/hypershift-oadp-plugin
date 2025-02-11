package common

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"context"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/utils/ptr"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
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
func TestValidateCronSchedule(t *testing.T) {
	tests := []struct {
		name      string
		schedule  string
		expectErr bool
	}{
		{"valid cron schedule", "0 0 * * *", false},
		{"valid cron schedule, every 5 min", "*/5 * * * *", false},
		{"Yearly cron schedule", "@yearly", false},
		{"Annually cron schedule", "@annually", false},
		{"Monthly cron schedule", "@monthly", false},
		{"Weekly cron schedule", "@weekly", false},
		{"daily cron schedule", "@daily", false},
		{"at midnight cron schedule", "@midnight", false},
		{"hourly cron schedule", "@hourly", false},
		{"invalid cron schedule with typo", "0 12 * * ?", true},
		{"invalid cron schedule", "invalid schedule", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := ValidateCronSchedule(tt.schedule)
			g.Expect(err != nil).To(Equal(tt.expectErr))
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

			err := ManagePauseHostedCluster(context.TODO(), client, log, tt.paused, tt.namespaces)
			if tt.expectErr {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				for _, hc := range tt.hcList.Items {
					updatedHC := &hyperv1.HostedCluster{}
					err := client.Get(context.TODO(), types.NamespacedName{Name: hc.Name, Namespace: hc.Namespace}, updatedHC)
					g.Expect(err).NotTo(HaveOccurred())
					g.Expect(updatedHC.Spec.PausedUntil).To(Equal(ptr.To(tt.paused)))
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

			err := ManagePauseNodepools(context.TODO(), client, log, tt.paused, tt.namespaces)
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
					Namespace: "test-namespace",
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
					Namespace: "test-namespace",
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
