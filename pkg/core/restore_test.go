package core

import (
	"context"
	"net/url"
	"os"
	"strings"
	"testing"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	plugtypes "github.com/openshift/hypershift-oadp-plugin/pkg/core/types"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	veleroapiv1 "github.com/vmware-tanzu/velero/pkg/plugin/velero"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
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

func TestPresignS3URL(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)
	_ = velerov1api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	credentialData := []byte(`[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
`)

	defaultBSL := &velerov1api.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
		Spec: velerov1api.BackupStorageLocationSpec{
			Config: map[string]string{"region": "us-east-1"},
			Credential: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "cloud",
			},
		},
	}

	defaultSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
		Data:       map[string][]byte{"cloud": credentialData},
	}

	defaultBackup := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
		Spec:       velerov1api.BackupSpec{StorageLocation: "default"},
	}

	origSAPath := common.DefaultK8sSAFilePath
	nsDir := t.TempDir()
	if err := os.WriteFile(nsDir+"/namespace", []byte("openshift-adp"), 0644); err != nil {
		t.Fatalf("failed to write namespace file: %v", err)
	}
	common.SetK8sSAFilePath(nsDir)
	t.Cleanup(func() { common.SetK8sSAFilePath(origSAPath) })

	tests := []struct {
		name    string
		setup   func() (*RestorePlugin, *velerov1api.Backup)
		s3URL   string
		wantErr bool
		assert  func(*testing.T, string)
	}{
		{
			name: "When presigning a valid s3 URL, it should produce a pre-signed HTTPS URL with all SigV4 params",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultBSL, defaultSecret).Build()
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, defaultBackup
			},
			s3URL: "s3://my-bucket/path/to/snapshot.db",
			assert: func(t *testing.T, result string) {
				parsed, err := url.Parse(result)
				if err != nil {
					t.Fatalf("failed to parse result URL: %v", err)
				}
				if parsed.Scheme != "https" {
					t.Errorf("expected https scheme, got %s", parsed.Scheme)
				}
				for _, p := range []string{"X-Amz-Algorithm", "X-Amz-Credential", "X-Amz-Date", "X-Amz-Expires", "X-Amz-Signature", "X-Amz-SignedHeaders"} {
					if parsed.Query().Get(p) == "" {
						t.Errorf("missing required query param %s", p)
					}
				}
			},
		},
		{
			name: "When presigning an invalid URL, it should return error",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultBSL, defaultSecret).Build()
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, defaultBackup
			},
			s3URL:   "not-an-s3-url",
			wantErr: true,
		},
		{
			name: "When BSL does not exist, it should return error",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultBSL, defaultSecret).Build()
				backup := defaultBackup.DeepCopy()
				backup.Spec.StorageLocation = "nonexistent"
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, backup
			},
			s3URL:   "s3://bucket/key",
			wantErr: true,
		},
		{
			name: "When BSL has no credential ref, it should use fallback cloud-credentials and presign successfully",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				bsl := defaultBSL.DeepCopy()
				bsl.Name = "no-cred-bsl"
				bsl.Spec.Credential = nil
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bsl, defaultSecret).Build()
				backup := defaultBackup.DeepCopy()
				backup.Spec.StorageLocation = "no-cred-bsl"
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, backup
			},
			s3URL: "s3://bucket/key",
			assert: func(t *testing.T, result string) {
				parsed, err := url.Parse(result)
				if err != nil {
					t.Fatalf("failed to parse result URL: %v", err)
				}
				if parsed.Scheme != "https" {
					t.Errorf("expected https scheme, got %s", parsed.Scheme)
				}
			},
		},
		{
			name: "When BSL has no credential ref and fallback secret is missing, it should return error",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				bsl := defaultBSL.DeepCopy()
				bsl.Name = "no-cred-bsl"
				bsl.Spec.Credential = nil
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bsl).Build()
				backup := defaultBackup.DeepCopy()
				backup.Spec.StorageLocation = "no-cred-bsl"
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, backup
			},
			s3URL:   "s3://bucket/key",
			wantErr: true,
		},
		{
			name: "When credential secret has wrong key, it should return error",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				wrongSecret := &corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "wrong-key-secret", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"wrong-key": credentialData},
				}
				bsl := defaultBSL.DeepCopy()
				bsl.Name = "wrong-key-bsl"
				bsl.Spec.Credential = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "wrong-key-secret"},
					Key:                  "cloud",
				}
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bsl, wrongSecret).Build()
				backup := defaultBackup.DeepCopy()
				backup.Spec.StorageLocation = "wrong-key-bsl"
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, backup
			},
			s3URL:   "s3://bucket/key",
			wantErr: true,
		},
		{
			name: "When URL is already https, it should return error since presignS3URL only handles s3:// scheme",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(defaultBSL, defaultSecret).Build()
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, defaultBackup
			},
			s3URL:   "https://my-bucket.s3.us-east-1.amazonaws.com/path/to/snapshot.db?X-Amz-Signature=abc",
			wantErr: true,
		},
		{
			name: "When BSL has custom endpoint, it should use that endpoint in the pre-signed URL",
			setup: func() (*RestorePlugin, *velerov1api.Backup) {
				bsl := defaultBSL.DeepCopy()
				bsl.Name = "minio-bsl"
				bsl.Spec.Config["s3Url"] = "https://minio.example.com"
				bsl.Spec.Config["s3ForcePathStyle"] = "true"
				client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(bsl, defaultSecret).Build()
				backup := defaultBackup.DeepCopy()
				backup.Spec.StorageLocation = "minio-bsl"
				return &RestorePlugin{log: logrus.New(), ctx: context.Background(), client: client}, backup
			},
			s3URL: "s3://bucket/key",
			assert: func(t *testing.T, result string) {
				parsed, _ := url.Parse(result)
				if parsed.Host != "minio.example.com" {
					t.Errorf("expected minio.example.com host, got %s", parsed.Host)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plugin, backup := tt.setup()
			result, err := plugin.presignS3URL(context.Background(), backup, tt.s3URL, "test-hc")
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, result)
			}
		})
	}
}

func newHCUnstructured(name, namespace string, annotations map[string]string) *unstructured.Unstructured {
	hc := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "hypershift.openshift.io/v1beta1",
			"kind":       "HostedCluster",
			"metadata": map[string]any{
				"name":      name,
				"namespace": namespace,
			},
			"spec": map[string]any{
				"etcd": map[string]any{
					"managementType": "Managed",
					"managed": map[string]any{
						"storage": map[string]any{
							"type": "PersistentVolume",
							"persistentVolume": map[string]any{
								"size": "8Gi",
							},
						},
					},
				},
				"platform": map[string]any{"type": "AWS"},
				"release":  map[string]any{"image": "quay.io/openshift-release-dev/ocp-release:4.16.0-x86_64"},
				"networking": map[string]any{
					"clusterNetwork": []any{map[string]any{"cidr": "10.132.0.0/14"}},
					"serviceNetwork": []any{map[string]any{"cidr": "172.31.0.0/16"}},
				},
				"pullSecret": map[string]any{"name": "pull-secret"},
				"infraID":    "test-infra",
				"services":   []any{},
			},
		},
	}
	if annotations != nil {
		hc.Object["metadata"].(map[string]any)["annotations"] = func() map[string]any {
			m := make(map[string]any, len(annotations))
			for k, v := range annotations {
				m[k] = v
			}
			return m
		}()
	}
	return hc
}

func newHCPUnstructured(t *testing.T, name, namespace string, annotations map[string]string) *unstructured.Unstructured {
	t.Helper()
	hcp := &hyperv1.HostedControlPlane{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "hypershift.openshift.io/v1beta1",
			Kind:       "HostedControlPlane",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:        name,
			Namespace:   namespace,
			Annotations: annotations,
		},
		Spec: hyperv1.HostedControlPlaneSpec{
			ReleaseImage: "quay.io/openshift-release-dev/ocp-release:4.16.0-x86_64",
			Platform:     hyperv1.PlatformSpec{Type: hyperv1.AWSPlatform},
			PullSecret:   corev1.LocalObjectReference{Name: "pull-secret"},
			IssuerURL:    "https://kubernetes.default.svc",
			SSHKey:       corev1.LocalObjectReference{Name: "ssh-key"},
			InfraID:      "test-infra",
			Etcd: hyperv1.EtcdSpec{
				ManagementType: hyperv1.Managed,
				Managed: &hyperv1.ManagedEtcdSpec{
					Storage: hyperv1.ManagedEtcdStorageSpec{
						Type: hyperv1.PersistentVolumeEtcdStorage,
						PersistentVolume: &hyperv1.PersistentVolumeEtcdStorageSpec{
							Size: func() *resource.Quantity { q := resource.MustParse("8Gi"); return &q }(),
						},
					},
				},
			},
		},
	}
	raw, err := runtime.DefaultUnstructuredConverter.ToUnstructured(hcp)
	if err != nil {
		t.Fatalf("failed to convert HCP to unstructured: %v", err)
	}
	return &unstructured.Unstructured{Object: raw}
}

func TestRestoreExecuteSnapshotURL(t *testing.T) {
	s := common.CustomScheme

	credentialData := []byte("[default]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n")

	hcpCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "hostedcontrolplanes.hypershift.openshift.io"},
	}

	bsl := &velerov1api.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
		Spec: velerov1api.BackupStorageLocationSpec{
			Config: map[string]string{"region": "us-east-1"},
			Credential: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "cloud",
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
		Data:       map[string][]byte{"cloud": credentialData},
	}
	backup := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
		Spec: velerov1api.BackupSpec{
			StorageLocation:    "default",
			IncludedNamespaces: []string{"clusters", "clusters-test"},
		},
	}
	restore := &velerov1api.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "test-restore", Namespace: "openshift-adp"},
		Spec:       velerov1api.RestoreSpec{BackupName: "test-backup"},
	}

	origSAPath := common.DefaultK8sSAFilePath
	nsDir := t.TempDir()
	if err := os.WriteFile(nsDir+"/namespace", []byte("openshift-adp"), 0644); err != nil {
		t.Fatalf("failed to write namespace file: %v", err)
	}
	common.SetK8sSAFilePath(nsDir)
	t.Cleanup(func() { common.SetK8sSAFilePath(origSAPath) })

	// extractRestoreSnapshotURL is a helper to navigate the unstructured HC output
	extractRestoreSnapshotURL := func(output *veleroapiv1.RestoreItemActionExecuteOutput) ([]any, bool) {
		spec := output.UpdatedItem.UnstructuredContent()["spec"].(map[string]any)
		etcd := spec["etcd"].(map[string]any)
		managed := etcd["managed"].(map[string]any)
		storage := managed["storage"].(map[string]any)
		urls, ok := storage["restoreSnapshotURL"].([]any)
		return urls, ok
	}

	tests := []struct {
		name        string
		annotations map[string]string
		wantErr     bool
		assert      func(*testing.T, *veleroapiv1.RestoreItemActionExecuteOutput)
	}{
		{
			name: "When HC has etcd-snapshot-url annotation with s3 scheme, it should inject pre-signed restoreSnapshotURL",
			annotations: map[string]string{
				common.EtcdSnapshotURLAnnotation: "s3://my-bucket/path/to/snapshot.db",
			},
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				urls, ok := extractRestoreSnapshotURL(output)
				if !ok || len(urls) == 0 {
					t.Fatal("expected restoreSnapshotURL to be set")
				}
				presignedURL, ok := urls[0].(string)
				if !ok {
					t.Fatal("expected restoreSnapshotURL[0] to be a string")
				}
				if !strings.HasPrefix(presignedURL, "https://") {
					t.Errorf("expected pre-signed URL to start with https://, got %s", presignedURL)
				}
				if !strings.Contains(presignedURL, "X-Amz-Signature") {
					t.Errorf("expected pre-signed URL to contain X-Amz-Signature, got %s", presignedURL)
				}
			},
		},
		{
			name:        "When HC has no etcd-snapshot-url annotation, it should not inject restoreSnapshotURL",
			annotations: nil,
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				spec := output.UpdatedItem.UnstructuredContent()["spec"].(map[string]any)
				etcd := spec["etcd"].(map[string]any)
				managed := etcd["managed"].(map[string]any)
				storage := managed["storage"].(map[string]any)
				if _, exists := storage["restoreSnapshotURL"]; exists {
					t.Error("expected restoreSnapshotURL to NOT be set when no annotation is present")
				}
			},
		},
		{
			name: "When HC has https annotation, it should inject it directly without presigning",
			annotations: map[string]string{
				common.EtcdSnapshotURLAnnotation: "https://my-bucket.s3.us-east-1.amazonaws.com/path/to/snapshot.db?X-Amz-Signature=abc123",
			},
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				urls, ok := extractRestoreSnapshotURL(output)
				if !ok || len(urls) == 0 {
					t.Fatal("expected restoreSnapshotURL to be set for https URL")
				}
				expected := "https://my-bucket.s3.us-east-1.amazonaws.com/path/to/snapshot.db?X-Amz-Signature=abc123"
				if urls[0].(string) != expected {
					t.Errorf("expected URL to pass through unchanged, got %s", urls[0])
				}
			},
		},
		{
			name: "When HC has Azure Blob https annotation, it should pass through without presigning",
			annotations: map[string]string{
				common.EtcdSnapshotURLAnnotation: "https://mystorageaccount.blob.core.windows.net/backups/etcd-snapshot/snapshot.db",
			},
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				urls, ok := extractRestoreSnapshotURL(output)
				if !ok || len(urls) == 0 {
					t.Fatal("expected restoreSnapshotURL to be set for Azure Blob URL")
				}
				expected := "https://mystorageaccount.blob.core.windows.net/backups/etcd-snapshot/snapshot.db"
				if urls[0].(string) != expected {
					t.Errorf("expected Azure Blob URL to pass through unchanged, got %s", urls[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fakeClient := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(hcpCRD, bsl, secret, backup).
				Build()

			plugin := &RestorePlugin{
				log:       logrus.New(),
				ctx:       context.Background(),
				client:    fakeClient,
				validator: &mockRestoreValidator{},
			}

			hc := newHCUnstructured("my-hc", "clusters", tt.annotations)

			output, err := plugin.Execute(&veleroapiv1.RestoreItemActionExecuteInput{
				Item:    hc,
				Restore: restore,
			})
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, output)
			}
		})
	}
}

func TestRestoreExecuteHCPSnapshotURL(t *testing.T) {
	s := common.CustomScheme

	credentialData := []byte("[default]\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\n")

	hcpCRD := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{Name: "hostedcontrolplanes.hypershift.openshift.io"},
	}

	bsl := &velerov1api.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
		Spec: velerov1api.BackupStorageLocationSpec{
			Config: map[string]string{"region": "us-east-1"},
			Credential: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "cloud",
			},
		},
	}
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
		Data:       map[string][]byte{"cloud": credentialData},
	}
	backup := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
		Spec: velerov1api.BackupSpec{
			StorageLocation:    "default",
			IncludedNamespaces: []string{"clusters", "clusters-test"},
		},
	}
	restore := &velerov1api.Restore{
		ObjectMeta: metav1.ObjectMeta{Name: "test-restore", Namespace: "openshift-adp"},
		Spec:       velerov1api.RestoreSpec{BackupName: "test-backup"},
	}

	origSAPath := common.DefaultK8sSAFilePath
	nsDir := t.TempDir()
	if err := os.WriteFile(nsDir+"/namespace", []byte("openshift-adp"), 0644); err != nil {
		t.Fatalf("failed to write namespace file: %v", err)
	}
	common.SetK8sSAFilePath(nsDir)
	t.Cleanup(func() { common.SetK8sSAFilePath(origSAPath) })

	extractRestoreSnapshotURL := func(output *veleroapiv1.RestoreItemActionExecuteOutput) ([]any, bool) {
		spec := output.UpdatedItem.UnstructuredContent()["spec"].(map[string]any)
		etcd := spec["etcd"].(map[string]any)
		managed := etcd["managed"].(map[string]any)
		storage := managed["storage"].(map[string]any)
		urls, ok := storage["restoreSnapshotURL"].([]any)
		return urls, ok
	}

	tests := []struct {
		name        string
		annotations map[string]string
		missingBSL  bool
		wantErr     bool
		assert      func(*testing.T, *veleroapiv1.RestoreItemActionExecuteOutput)
	}{
		{
			name: "When HCP has s3 annotation and BSL is missing, it should return presign error",
			annotations: map[string]string{
				common.EtcdSnapshotURLAnnotation: "s3://my-bucket/path/to/snapshot.db",
			},
			missingBSL: true,
			wantErr:    true,
		},
		{
			name: "When HCP has etcd-snapshot-url annotation with s3 scheme, it should inject pre-signed restoreSnapshotURL",
			annotations: map[string]string{
				common.EtcdSnapshotURLAnnotation: "s3://my-bucket/path/to/snapshot.db",
			},
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				urls, ok := extractRestoreSnapshotURL(output)
				if !ok || len(urls) == 0 {
					t.Fatal("expected restoreSnapshotURL to be set")
				}
				presignedURL, ok := urls[0].(string)
				if !ok {
					t.Fatal("expected restoreSnapshotURL[0] to be a string")
				}
				if !strings.HasPrefix(presignedURL, "https://") {
					t.Errorf("expected pre-signed URL to start with https://, got %s", presignedURL)
				}
				if !strings.Contains(presignedURL, "X-Amz-Signature") {
					t.Errorf("expected pre-signed URL to contain X-Amz-Signature, got %s", presignedURL)
				}
			},
		},
		{
			name:        "When HCP has no etcd-snapshot-url annotation, it should not inject restoreSnapshotURL",
			annotations: nil,
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				spec := output.UpdatedItem.UnstructuredContent()["spec"].(map[string]any)
				etcd := spec["etcd"].(map[string]any)
				managed := etcd["managed"].(map[string]any)
				storage := managed["storage"].(map[string]any)
				if _, exists := storage["restoreSnapshotURL"]; exists {
					t.Error("expected restoreSnapshotURL to NOT be set when no annotation is present")
				}
			},
		},
		{
			name: "When HCP has https annotation, it should inject it directly without presigning",
			annotations: map[string]string{
				common.EtcdSnapshotURLAnnotation: "https://my-bucket.s3.us-east-1.amazonaws.com/path/to/snapshot.db?X-Amz-Signature=abc123",
			},
			assert: func(t *testing.T, output *veleroapiv1.RestoreItemActionExecuteOutput) {
				urls, ok := extractRestoreSnapshotURL(output)
				if !ok || len(urls) == 0 {
					t.Fatal("expected restoreSnapshotURL to be set for https URL")
				}
				expected := "https://my-bucket.s3.us-east-1.amazonaws.com/path/to/snapshot.db?X-Amz-Signature=abc123"
				if urls[0].(string) != expected {
					t.Errorf("expected URL to pass through unchanged, got %s", urls[0])
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			builder := fake.NewClientBuilder().
				WithScheme(s).
				WithObjects(hcpCRD, backup)
			if !tt.missingBSL {
				builder = builder.WithObjects(bsl, secret)
			}
			fakeClient := builder.Build()

			plugin := &RestorePlugin{
				log:       logrus.New(),
				ctx:       context.Background(),
				client:    fakeClient,
				validator: &mockRestoreValidator{},
			}

			hcp := newHCPUnstructured(t, "my-hcp", "clusters-test", tt.annotations)

			output, err := plugin.Execute(&veleroapiv1.RestoreItemActionExecuteInput{
				Item:    hcp,
				Restore: restore,
			})
			if tt.wantErr {
				if err == nil {
					t.Error("expected error but got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.assert != nil {
				tt.assert(t, output)
			}
		})
	}
}
