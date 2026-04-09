package core

import (
	"context"
	"net/url"
	"os"
	"testing"

	common "github.com/openshift/hypershift-oadp-plugin/pkg/common"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1api "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestPresignS3URL(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = hyperv1.AddToScheme(scheme)
	_ = velerov1api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	credentialData := []byte(`[default]
aws_access_key_id = AKIAIOSFODNN7EXAMPLE
aws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
`)

	bsl := &velerov1api.BackupStorageLocation{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: "openshift-adp",
		},
		Spec: velerov1api.BackupStorageLocationSpec{
			Config: map[string]string{
				"region": "us-east-1",
			},
			Credential: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: "cloud-credentials",
				},
				Key: "cloud",
			},
		},
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "cloud-credentials",
			Namespace: "openshift-adp",
		},
		Data: map[string][]byte{
			"cloud": credentialData,
		},
	}

	backup := &velerov1api.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "openshift-adp",
		},
		Spec: velerov1api.BackupSpec{
			StorageLocation: "default",
		},
	}

	// Override namespace resolution for tests
	origSAPath := common.DefaultK8sSAFilePath
	nsDir := t.TempDir()
	if err := os.WriteFile(nsDir+"/namespace", []byte("openshift-adp"), 0644); err != nil {
		t.Fatalf("failed to write namespace file: %v", err)
	}
	common.SetK8sSAFilePath(nsDir)
	t.Cleanup(func() { common.SetK8sSAFilePath(origSAPath) })

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(bsl, secret).
		Build()

	plugin := &RestorePlugin{
		log:    logrus.New(),
		ctx:    context.Background(),
		client: fakeClient,
	}

	t.Run("valid s3 URL produces pre-signed HTTPS URL", func(t *testing.T) {
		result, err := plugin.presignS3URL(context.Background(), backup, "s3://my-bucket/path/to/snapshot.db")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if result == "" {
			t.Fatal("expected non-empty URL")
		}

		parsed, err := url.Parse(result)
		if err != nil {
			t.Fatalf("failed to parse result URL: %v", err)
		}

		if parsed.Scheme != "https" {
			t.Errorf("expected https scheme, got %s", parsed.Scheme)
		}

		requiredParams := []string{
			"X-Amz-Algorithm",
			"X-Amz-Credential",
			"X-Amz-Date",
			"X-Amz-Expires",
			"X-Amz-Signature",
			"X-Amz-SignedHeaders",
		}
		for _, p := range requiredParams {
			if parsed.Query().Get(p) == "" {
				t.Errorf("missing required query param %s", p)
			}
		}
	})

	t.Run("invalid s3 URL returns error", func(t *testing.T) {
		_, err := plugin.presignS3URL(context.Background(), backup, "not-an-s3-url")
		if err == nil {
			t.Error("expected error for invalid S3 URL")
		}
	})

	t.Run("BSL not found returns error", func(t *testing.T) {
		badBackup := backup.DeepCopy()
		badBackup.Spec.StorageLocation = "nonexistent"

		_, err := plugin.presignS3URL(context.Background(), badBackup, "s3://bucket/key")
		if err == nil {
			t.Error("expected error for nonexistent BSL")
		}
	})

	t.Run("custom endpoint uses endpoint URL", func(t *testing.T) {
		bslCustom := bsl.DeepCopy()
		bslCustom.Name = "minio-bsl"
		bslCustom.Spec.Config["s3Url"] = "https://minio.example.com"
		bslCustom.Spec.Config["s3ForcePathStyle"] = "true"

		fakeClientCustom := fake.NewClientBuilder().
			WithScheme(scheme).
			WithObjects(bslCustom, secret).
			Build()

		pluginCustom := &RestorePlugin{
			log:    logrus.New(),
			ctx:    context.Background(),
			client: fakeClientCustom,
		}

		backupCustom := backup.DeepCopy()
		backupCustom.Spec.StorageLocation = "minio-bsl"

		result, err := pluginCustom.presignS3URL(context.Background(), backupCustom, "s3://bucket/key")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		parsed, _ := url.Parse(result)
		if parsed.Host != "minio.example.com" {
			t.Errorf("expected minio.example.com host, got %s", parsed.Host)
		}
	})
}
