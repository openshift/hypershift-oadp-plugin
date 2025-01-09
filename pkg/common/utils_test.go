package common

import (
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
		header     string
		hcList     *hyperv1.HostedClusterList
		expectErr  bool
	}{
		{
			name:       "Pause HostedCluster",
			namespaces: []string{"test-namespace"},
			paused:     "true",
			header:     "TestHeader",
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
			header:     "TestHeader",
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
			header:     "TestHeader",
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

			err := ManagePauseHostedCluster(context.TODO(), client, log, tt.paused, tt.header, tt.namespaces)
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

			err := ManagePauseNodepools(context.TODO(), client, log, tt.paused, tt.header, tt.namespaces)
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
