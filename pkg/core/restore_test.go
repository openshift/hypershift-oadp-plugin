package core

import (
	"context"
	"os"
	"testing"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	validation "github.com/openshift/hypershift-oadp-plugin/pkg/core/validation"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/plugin/velero"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

type mockRestoreValidator struct {
	validatePlatformErr error
}

func (m *mockRestoreValidator) ValidatePluginConfig(_ map[string]string) (*plugtypes.RestoreOptions, error) {
	return &plugtypes.RestoreOptions{}, nil
}

func (m *mockRestoreValidator) ValidatePlatformConfig(_ *hyperv1.HostedControlPlane, _ map[string]string) error {
	return m.validatePlatformErr
}

var _ validation.RestoreValidator = &mockRestoreValidator{}

var testHCPResources = []string{
	"hostedclusters.hypershift.openshift.io",
	"hostedcontrolplanes.hypershift.openshift.io",
	"nodepools.hypershift.openshift.io",
	"secrets",
	"configmaps",
	"persistentvolumeclaims",
	"persistentvolumes",
}

func newSecretUnstructured(name, namespace string) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "v1",
			"kind":       "Secret",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
		},
	}
}

func TestRestoreExecuteImportSecretExclusion(t *testing.T) {
	s := common.CustomScheme

	backup := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
		Spec: velerov1api.BackupSpec{
			StorageLocation:    "default",
			IncludedNamespaces: []string{"clusters", "clusters-test"},
			IncludedResources:  testHCPResources,
		},
	}
	restore := &velerov1api.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "test-restore", Namespace: "openshift-adp"},
		Spec:       velerov1api.RestoreSpec{BackupName: "test-backup"},
	}

	nsDir := t.TempDir()
	if err := os.WriteFile(nsDir+"/namespace", []byte("openshift-adp"), 0644); err != nil {
		t.Fatalf("failed to write namespace file: %v", err)
	}

	tests := []struct {
		name        string
		secretName  string
		wantSkipped bool
	}{
		{
			name:        "import secret should be skipped",
			secretName:  "my-hosted-cluster-import",
			wantSkipped: true,
		},
		{
			name:        "regular secret should restore normally",
			secretName:  "pull-secret",
			wantSkipped: false,
		},
		{
			name:        "secret with import prefix should restore normally",
			secretName:  "import-credentials",
			wantSkipped: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(backup).
				Build()
			plugin := &RestorePlugin{
				log:       logrus.New(),
				ctx:       context.Background(),
				client:    fakeClient,
				validator: &mockRestoreValidator{},
				config:    map[string]string{},
			}

			secret := newSecretUnstructured(tt.secretName, "clusters")

			output, err := plugin.Execute(&veleroapiv1.RestoreItemActionExecuteInput{
				Item:    secret,
				Restore: restore,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if output.SkipRestore != tt.wantSkipped {
				t.Errorf("expected SkipRestore=%v, got %v", tt.wantSkipped, output.SkipRestore)
			}
		})
	}
}
