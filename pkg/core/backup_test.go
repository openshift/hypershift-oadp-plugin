package core

// Test scenario names follow: "When <action or context>, It Should <expected outcome>".

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

// mockValidator implements validation.BackupValidator for testing.
type mockValidator struct {
	validatePlatformErr error
}

func (m *mockValidator) ValidatePluginConfig(_ map[string]string) (*plugtypes.BackupOptions, error) {
	return &plugtypes.BackupOptions{}, nil
}

func (m *mockValidator) ValidatePlatformConfig(_ *hyperv1.HostedControlPlane, _ *velerov1.Backup) error {
	return m.validatePlatformErr
}

func newTestBackupPlugin(objects ...runtime.Object) *BackupPlugin {
	scheme := common.CustomScheme

	hcpCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "hostedcontrolplanes.hypershift.openshift.io"},
	}

	hcp := &hyperv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{Name: "test-hcp", Namespace: "clusters-test"},
		Spec: hyperv1.HostedControlPlaneSpec{
			Platform: hyperv1.PlatformSpec{Type: hyperv1.AWSPlatform},
		},
	}

	allObjects := []runtime.Object{hcpCRD, hcp}
	allObjects = append(allObjects, objects...)

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(allObjects...).
		Build()

	return &BackupPlugin{
		log:              logrus.New(),
		ctx:              context.Background(),
		client:           client,
		config:           map[string]string{},
		validator:        &mockValidator{},
		hcp:              hcp,
		BackupOptions:    &plugtypes.BackupOptions{},
		hoNamespace:      "hypershift",
		etcdBackupMethod: common.EtcdBackupMethodVolume,
	}
}

func newUnstructuredItem(kind, apiVersion, name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": apiVersion,
			"kind":       kind,
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func newTestBackup() *velerov1.Backup {
	return &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{"clusters", "clusters-test"},
		},
	}
}

func TestAppliesToReturnsSpecificResources(t *testing.T) {
	g := NewWithT(t)
	bp := newTestBackupPlugin()

	selector, err := bp.AppliesTo()
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(selector.IncludedResources).NotTo(BeEmpty(), "AppliesTo must not return an empty resource selector")

	// Per-provider happy-path assertions: at least one representative resource per platform.
	// These ensure that accidentally dropping a platform slice is caught immediately.
	for _, tc := range []struct {
		provider  string
		resources []string
	}{
		{"common", plugtypes.BackupCommonResources},
		{"aws", plugtypes.BackupAWSResources},
		{"azure", plugtypes.BackupAzureResources},
		{"ibmpowervs", plugtypes.BackupIBMPowerVSResources},
		{"openstack", plugtypes.BackupOpenStackResources},
		{"kubevirt", plugtypes.BackupKubevirtResources},
		{"agent", plugtypes.BackupAgentResources},
	} {
		for _, r := range tc.resources {
			g.Expect(selector.IncludedResources).To(
				ContainElement(r),
				"provider %q resource %q must appear in AppliesTo", tc.provider, r,
			)
		}
	}

	// Sad-path assertion: unknown resources must NOT appear.
	g.Expect(selector.IncludedResources).NotTo(ContainElement("completelyunknownresource"))
}

func TestExecuteSkipsWhenHCPNotFound(t *testing.T) {
	g := NewWithT(t)

	scheme := common.CustomScheme

	hcpCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "hostedcontrolplanes.hypershift.openshift.io"},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(hcpCRD).
		Build()

	bp := &BackupPlugin{
		log:              logrus.New(),
		ctx:              context.Background(),
		client:           client,
		config:           map[string]string{},
		validator:        &mockValidator{},
		BackupOptions:    &plugtypes.BackupOptions{},
		hoNamespace:      "hypershift",
		etcdBackupMethod: common.EtcdBackupMethodVolume,
	}

	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
		Spec: velerov1.BackupSpec{
			IncludedNamespaces: []string{"clusters-test"},
			IncludedResources:  []string{"hostedcontrolplanes"},
		},
	}

	item := newUnstructuredItem("Secret", "v1", "my-secret", "clusters-test")

	// First call should gracefully skip (no HCP in namespace)
	result, _, err := bp.Execute(item, backup)
	g.Expect(err).NotTo(HaveOccurred())
	g.Expect(result).NotTo(BeNil(), "should return item, not error")
	g.Expect(bp.hcpNotFound).To(BeTrue(), "should cache hcpNotFound=true")

	// Second call should return immediately without re-querying
	result2, _, err2 := bp.Execute(item, backup)
	g.Expect(err2).NotTo(HaveOccurred())
	g.Expect(result2).NotTo(BeNil())
}

func TestExecute(t *testing.T) {
	falseVal := false

	tests := []struct {
		name            string
		setup           func(*BackupPlugin)
		item            func() *unstructured.Unstructured
		backup          func() *velerov1.Backup
		wantNilResult   bool
		wantErr         bool
		assert          func(*GomegaWithT, runtime.Unstructured, *BackupPlugin)
	}{
		// HostedCluster cases
		{
			name: "When Execute processes a HostedCluster item, It Should add restore annotation",
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("HostedCluster", "hypershift.openshift.io/v1beta1", "my-hc", "clusters")
			},
			backup: newTestBackup,
			assert: func(g *GomegaWithT, result runtime.Unstructured, _ *BackupPlugin) {
				metadata := result.UnstructuredContent()["metadata"].(map[string]any)
				annotations := metadata["annotations"].(map[string]any)
				_, exists := annotations[common.HostedClusterRestoredFromBackupAnnotation]
				g.Expect(exists).To(BeTrue())
			},
		},
		{
			name: "When Execute processes a HostedCluster with cached etcdSnapshotURL, It Should inject URL into status and annotation",
			setup: func(bp *BackupPlugin) {
				bp.etcdSnapshotURL = "s3://bucket/backups/test/etcd-backup/snapshot.db"
			},
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("HostedCluster", "hypershift.openshift.io/v1beta1", "my-hc", "clusters")
			},
			backup: newTestBackup,
			assert: func(g *GomegaWithT, result runtime.Unstructured, _ *BackupPlugin) {
				metadata := result.UnstructuredContent()["metadata"].(map[string]any)
				annotations := metadata["annotations"].(map[string]any)
				g.Expect(annotations[common.EtcdSnapshotURLAnnotation]).To(Equal("s3://bucket/backups/test/etcd-backup/snapshot.db"))

				status := result.UnstructuredContent()["status"].(map[string]any)
				g.Expect(status["lastSuccessfulEtcdBackupURL"]).To(Equal("s3://bucket/backups/test/etcd-backup/snapshot.db"))
			},
		},
		// HostedControlPlane cases
		{
			name: "When Execute processes a HostedControlPlane with cached etcdSnapshotURL, It Should add etcd snapshot URL annotation",
			setup: func(bp *BackupPlugin) {
				bp.etcdSnapshotURL = "s3://bucket/backups/test/etcd-backup/snapshot.db"
			},
			item: func() *unstructured.Unstructured {
				item := newUnstructuredItem("HostedControlPlane", "hypershift.openshift.io/v1beta1", "test-hcp", "clusters-test")
				item.Object["spec"] = map[string]any{
					"platform": map[string]any{"type": "AWS"},
				}
				return item
			},
			backup: newTestBackup,
			assert: func(g *GomegaWithT, result runtime.Unstructured, _ *BackupPlugin) {
				metadata := result.UnstructuredContent()["metadata"].(map[string]any)
				annotations := metadata["annotations"].(map[string]any)
				g.Expect(annotations[common.EtcdSnapshotURLAnnotation]).To(Equal("s3://bucket/backups/test/etcd-backup/snapshot.db"))
			},
		},
		{
			name: "When Execute processes a HostedControlPlane with volumeSnapshot method, It Should not create etcd backup",
			item: func() *unstructured.Unstructured {
				item := newUnstructuredItem("HostedControlPlane", "hypershift.openshift.io/v1beta1", "test-hcp", "clusters-test")
				item.Object["spec"] = map[string]any{
					"platform": map[string]any{"type": "AWS"},
				}
				return item
			},
			backup: newTestBackup,
			assert: func(g *GomegaWithT, _ runtime.Unstructured, bp *BackupPlugin) {
				g.Expect(bp.etcdOrchestrator).To(BeNil())
			},
		},
		// Pod cases
		{
			name: "When Execute processes an etcd Pod with volumeSnapshot method and fsBackup disabled, It Should add FSBackup label",
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("Pod", "v1", "etcd-0", "clusters-test")
			},
			backup: func() *velerov1.Backup {
				b := newTestBackup()
				b.Spec.DefaultVolumesToFsBackup = &falseVal
				return b
			},
			assert: func(g *GomegaWithT, result runtime.Unstructured, _ *BackupPlugin) {
				metadata := result.UnstructuredContent()["metadata"].(map[string]any)
				labels := metadata["labels"].(map[string]any)
				g.Expect(labels[common.FSBackupLabelName]).To(Equal("true"))
			},
		},
		{
			name: "When Execute processes an etcd Pod with etcdSnapshot method, It Should skip the pod",
			setup: func(bp *BackupPlugin) {
				bp.etcdBackupMethod = common.EtcdBackupMethodEtcdSnapshot
			},
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("Pod", "v1", "etcd-0", "clusters-test")
			},
			backup:        newTestBackup,
			wantNilResult: true,
		},
		{
			name: "When Execute processes a non-etcd Pod, It Should pass through unchanged",
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("Pod", "v1", "kube-apiserver-0", "clusters-test")
			},
			backup: newTestBackup,
			assert: func(g *GomegaWithT, result runtime.Unstructured, _ *BackupPlugin) {
				metadata := result.UnstructuredContent()["metadata"].(map[string]any)
				g.Expect(metadata["name"]).To(Equal("kube-apiserver-0"))
			},
		},
		// PVC cases
		{
			name: "When Execute processes an etcd PVC with etcdSnapshot method, It Should skip the PVC",
			setup: func(bp *BackupPlugin) {
				bp.etcdBackupMethod = common.EtcdBackupMethodEtcdSnapshot
			},
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("PersistentVolumeClaim", "v1", "data-etcd-0", "clusters-test")
			},
			backup:        newTestBackup,
			wantNilResult: true,
		},
		{
			name: "When Execute processes a PVC with kubevirt RHCOS label, It Should skip the PVC",
			item: func() *unstructured.Unstructured {
				item := newUnstructuredItem("PersistentVolumeClaim", "v1", "rhcos-disk", "clusters-test")
				item.Object["metadata"].(map[string]any)["labels"] = map[string]any{
					common.KubevirtRHCOSLabel: "true",
				}
				return item
			},
			backup:        newTestBackup,
			wantNilResult: true,
		},
		{
			name: "When Execute processes a regular PVC, It Should pass through unchanged",
			item: func() *unstructured.Unstructured {
				return newUnstructuredItem("PersistentVolumeClaim", "v1", "some-data", "clusters-test")
			},
			backup: newTestBackup,
		},
		// DataVolume cases
		{
			name: "When Execute processes a DataVolume with kubevirt RHCOS label, It Should skip it",
			item: func() *unstructured.Unstructured {
				item := newUnstructuredItem("DataVolume", "cdi.kubevirt.io/v1beta1", "rhcos-dv", "clusters-test")
				item.Object["metadata"].(map[string]any)["labels"] = map[string]any{
					common.KubevirtRHCOSLabel: "true",
				}
				return item
			},
			backup:        newTestBackup,
			wantNilResult: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			bp := newTestBackupPlugin()
			if tt.setup != nil {
				tt.setup(bp)
			}

			result, _, err := bp.Execute(tt.item(), tt.backup())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())

			if tt.wantNilResult {
				g.Expect(result).To(BeNil())
				return
			}

			g.Expect(result).NotTo(BeNil())
			if tt.assert != nil {
				tt.assert(g, result, bp)
			}
		})
	}
}

func TestWaitForEtcdBackupCompletion(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*BackupPlugin)
	}{
		{
			name: "When orchestrator is nil, It Should return nil",
			setup: func(bp *BackupPlugin) {
				bp.etcdOrchestrator = nil
			},
		},
		{
			name: "When snapshotURL is already cached, It Should return nil without polling",
			setup: func(bp *BackupPlugin) {
				bp.etcdSnapshotURL = "s3://cached-url"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			bp := newTestBackupPlugin()
			tt.setup(bp)

			err := bp.waitForEtcdBackupCompletion(context.TODO())
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}
