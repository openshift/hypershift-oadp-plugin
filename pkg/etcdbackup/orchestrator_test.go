package etcdbackup

// Test scenario names follow: "When <action or context>, It Should <expected outcome>".

import (
	"context"
	"fmt"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	crclient "sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = hyperv1.AddToScheme(s)
	_ = velerov1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

func testClient(scheme *runtime.Scheme, objs ...crclient.Object) crclient.Client {
	b := fake.NewClientBuilder().WithScheme(scheme)
	if len(objs) > 0 {
		b = b.WithObjects(objs...).WithStatusSubresource(objs...)
	}
	return b.Build()
}

// mapBSLCase describes one row in TestMapBSLToStorage: either mapBSLToStorage (full path
// including key-prefix resolution) or a direct call to mapAWSBSLToStorage / mapAzureBSLToStorage.
type mapBSLCaseHow int

const (
	mapViaBSL mapBSLCaseHow = iota
	mapViaAWS
	mapViaAzure
)

// TestMapBSLToStorage covers mapBSLToStorage (AWS, Azure, unsupported, key prefixes) and
// mapAWSBSLToStorage / mapAzureBSLToStorage in one table.
func TestMapBSLToStorage(t *testing.T) {
	o := &Orchestrator{log: logrus.New()}
	tests := []struct {
		name         string
		how          mapBSLCaseHow
		bsl          *velerov1.BackupStorageLocation
		mapperPrefix string
		wantErr      bool
		errSubstr    string
		assert       func(*GomegaWithT, *hyperv1.HCPEtcdBackupStorage)
	}{
		{
			name: "When mapBSLToStorage runs for AWS BSL with bucket and object storage prefix, It Should set S3 bucket region and key prefix",
			how:  mapViaBSL,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-bucket", Prefix: "velero-backups"},
					},
					Config: map[string]string{"region": "us-east-1"},
				},
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.StorageType).To(Equal(hyperv1.S3BackupStorage))
				g.Expect(s.S3.Bucket).To(Equal("my-bucket"))
				g.Expect(s.S3.Region).To(Equal("us-east-1"))
				g.Expect(s.S3.KeyPrefix).To(Equal("velero-backups/backups/test-backup/etcd-backup"))
			},
		},
		{
			name: "When mapBSLToStorage runs for Azure BSL with container and prefix, It Should set Azure blob container account and key prefix",
			how:  mapViaBSL,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "azure",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-container", Prefix: "velero"},
					},
					Config: map[string]string{"storageAccount": "mystorageaccount"},
				},
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.StorageType).To(Equal(hyperv1.AzureBlobBackupStorage))
				g.Expect(s.AzureBlob.Container).To(Equal("my-container"))
				g.Expect(s.AzureBlob.StorageAccount).To(Equal("mystorageaccount"))
				g.Expect(s.AzureBlob.KeyPrefix).To(Equal("velero/backups/test-backup/etcd-backup"))
			},
		},
		{
			name: "When mapBSLToStorage runs for GCP BSL, It Should return unsupported provider error",
			how:  mapViaBSL,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{Provider: "gcp"},
			},
			wantErr:   true,
			errSubstr: "unsupported BSL provider",
		},
		{
			name: "When mapBSLToStorage runs for AWS BSL without object storage prefix, It Should default key prefix to etcd-backup",
			how:  mapViaBSL,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-bucket"},
					},
					Config: map[string]string{"region": "eu-west-1"},
				},
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.KeyPrefix).To(Equal("backups/test-backup/etcd-backup"))
			},
		},
		{
			name: "When mapBSLToStorage runs for velero.io/aws BSL with prefix, It Should include prefix and backup name in key prefix",
			how:  mapViaBSL,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "velero.io/aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-bucket", Prefix: "my-prefix"},
					},
					Config: map[string]string{"region": "us-west-2"},
				},
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.StorageType).To(Equal(hyperv1.S3BackupStorage))
				g.Expect(s.S3.KeyPrefix).To(Equal("my-prefix/backups/test-backup/etcd-backup"))
			},
		},
		{
			name: "When mapAWSBSLToStorage runs with bucket region and mapper prefix, It Should set S3 bucket region and key prefix",
			how:  mapViaAWS,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-bucket"},
					},
					Config: map[string]string{"region": "eu-central-1"},
				},
			},
			mapperPrefix: "backups/etcd-backup",
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.StorageType).To(Equal(hyperv1.S3BackupStorage))
				g.Expect(s.S3.Bucket).To(Equal("my-bucket"))
				g.Expect(s.S3.Region).To(Equal("eu-central-1"))
				g.Expect(s.S3.KeyPrefix).To(Equal("backups/etcd-backup"))
			},
		},
		{
			name: "When mapAWSBSLToStorage runs without object storage, It Should leave bucket empty and preserve region",
			how:  mapViaAWS,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Config: map[string]string{"region": "us-east-1"},
				},
			},
			mapperPrefix: "etcd-backup",
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.Bucket).To(BeEmpty())
				g.Expect(s.S3.Region).To(Equal("us-east-1"))
			},
		},
		{
			name: "When mapAWSBSLToStorage runs without config, It Should leave region empty",
			how:  mapViaAWS,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "b"},
					},
				},
			},
			mapperPrefix: "etcd-backup",
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.Bucket).To(Equal("b"))
				g.Expect(s.S3.Region).To(BeEmpty())
			},
		},
		{
			name: "When mapAzureBSLToStorage runs with container and storage account, It Should set Azure blob fields and key prefix",
			how:  mapViaAzure,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-container"},
					},
					Config: map[string]string{"storageAccount": "acct"},
				},
			},
			mapperPrefix: "velero/etcd-backup",
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.StorageType).To(Equal(hyperv1.AzureBlobBackupStorage))
				g.Expect(s.AzureBlob.Container).To(Equal("my-container"))
				g.Expect(s.AzureBlob.StorageAccount).To(Equal("acct"))
				g.Expect(s.AzureBlob.KeyPrefix).To(Equal("velero/etcd-backup"))
			},
		},
		{
			name: "When mapAzureBSLToStorage runs without object storage, It Should leave container empty",
			how:  mapViaAzure,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Config: map[string]string{"storageAccount": "sa"},
				},
			},
			mapperPrefix: "etcd-backup",
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.AzureBlob.Container).To(BeEmpty())
				g.Expect(s.AzureBlob.StorageAccount).To(Equal("sa"))
			},
		},
		{
			name: "When mapAzureBSLToStorage runs without config, It Should leave storage account empty",
			how:  mapViaAzure,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "c"},
					},
				},
			},
			mapperPrefix: "etcd-backup",
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.AzureBlob.Container).To(Equal("c"))
				g.Expect(s.AzureBlob.StorageAccount).To(BeEmpty())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			var storage *hyperv1.HCPEtcdBackupStorage
			var err error
			switch tt.how {
			case mapViaBSL:
				storage, err = o.mapBSLToStorage(tt.bsl, "test-backup")
			case mapViaAWS:
				storage = mapAWSBSLToStorage(tt.bsl, tt.mapperPrefix)
			case mapViaAzure:
				storage = mapAzureBSLToStorage(tt.bsl, tt.mapperPrefix)
			}
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				if tt.errSubstr != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				}
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			tt.assert(g, storage)
		})
	}
}

func TestCopyCredentialSecret(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name      string
		objects   []crclient.Object
		credRef   *corev1.SecretKeySelector
		wantErr   bool
		errSubstr string
		assert    func(*GomegaWithT, string, crclient.Client)
	}{
		{
			name: "When copyCredentialSecret runs with credential key cloud, It Should remap to credentials key in destination",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"cloud": []byte("aws-creds-data")},
				},
			},
			credRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "cloud",
			},
			assert: func(g *GomegaWithT, dstName string, client crclient.Client) {
				g.Expect(dstName).To(Equal("etcd-backup-creds-my-backup"))
				copied := &corev1.Secret{}
				err := client.Get(context.TODO(), types.NamespacedName{Name: dstName, Namespace: "hypershift"}, copied)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(copied.Data).To(HaveKey("credentials"))
				g.Expect(copied.Data["credentials"]).To(Equal([]byte("aws-creds-data")))
				// Original key "cloud" is preserved alongside "credentials"
				// so the controller can auto-detect credential type.
				g.Expect(copied.Data).To(HaveKey("cloud"))
				g.Expect(copied.Data["cloud"]).To(Equal([]byte("aws-creds-data")))
				g.Expect(copied.Labels["hypershift.openshift.io/etcd-backup"]).To(Equal("true"))
			},
		},
		{
			name: "When copyCredentialSecret runs and destination secret already exists, It Should reuse it",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "etcd-backup-creds-my-backup", Namespace: "hypershift"},
					Data:       map[string][]byte{"credentials": []byte("existing-data")},
				},
			},
			credRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "cloud",
			},
			assert: func(g *GomegaWithT, dstName string, _ crclient.Client) {
				g.Expect(dstName).To(Equal("etcd-backup-creds-my-backup"))
			},
		},
		{
			name: "When copyCredentialSecret runs with credential key credentials, It Should not duplicate the key",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"credentials": []byte("creds-data")},
				},
			},
			credRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "credentials",
			},
			assert: func(g *GomegaWithT, dstName string, client crclient.Client) {
				copied := &corev1.Secret{}
				err := client.Get(context.TODO(), types.NamespacedName{Name: dstName, Namespace: "hypershift"}, copied)
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(copied.Data).To(HaveKey("credentials"))
				g.Expect(copied.Data["credentials"]).To(Equal([]byte("creds-data")))
				// When srcKey is already "credentials", no extra key should be added
				g.Expect(copied.Data).To(HaveLen(1))
			},
		},
		{
			name: "When copyCredentialSecret runs and source secret lacks the expected key, It Should return error",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"wrong-key": []byte("data")},
				},
			},
			credRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
				Key:                  "cloud",
			},
			wantErr:   true,
			errSubstr: "does not contain key",
		},
		{
			name: "When copyCredentialSecret runs and source secret does not exist, It Should return error",
			credRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: "nonexistent-secret"},
				Key:                  "cloud",
			},
			wantErr:   true,
			errSubstr: "failed to get credential Secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := testClient(scheme, tt.objects...)
			o := &Orchestrator{log: logrus.New(), client: client}

			dstName, err := o.copyCredentialSecret(context.TODO(), tt.credRef, "openshift-adp", "hypershift", "my-backup")
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			if tt.assert != nil {
				tt.assert(g, dstName, client)
			}
		})
	}
}

func TestSetEncryptionFields(t *testing.T) {
	tests := []struct {
		name    string
		storage *hyperv1.HCPEtcdBackupStorage
		hc      *hyperv1.HostedCluster
		assert  func(*GomegaWithT, *hyperv1.HCPEtcdBackupStorage)
	}{
		{
			name:    "When setEncryptionFields runs with S3 storage and HostedCluster AWS KMS, It Should set S3 KMS key ARN",
			storage: &hyperv1.HCPEtcdBackupStorage{StorageType: hyperv1.S3BackupStorage, S3: hyperv1.HCPEtcdBackupS3{}},
			hc: &hyperv1.HostedCluster{
				Spec: hyperv1.HostedClusterSpec{
					Etcd: hyperv1.EtcdSpec{
						Managed: &hyperv1.ManagedEtcdSpec{
							Backup: hyperv1.HCPEtcdBackupConfig{
								Platform: hyperv1.AWSBackupConfigPlatform,
								AWS:      hyperv1.HCPEtcdBackupConfigAWS{KMSKeyARN: "arn:aws:kms:us-east-1:123456789012:key/test-key"},
							},
						},
					},
				},
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.KMSKeyARN).To(Equal("arn:aws:kms:us-east-1:123456789012:key/test-key"))
			},
		},
		{
			name:    "When setEncryptionFields runs with Azure storage and HostedCluster encryption URL, It Should set Azure encryption key URL",
			storage: &hyperv1.HCPEtcdBackupStorage{StorageType: hyperv1.AzureBlobBackupStorage, AzureBlob: hyperv1.HCPEtcdBackupAzureBlob{}},
			hc: &hyperv1.HostedCluster{
				Spec: hyperv1.HostedClusterSpec{
					Etcd: hyperv1.EtcdSpec{
						Managed: &hyperv1.ManagedEtcdSpec{
							Backup: hyperv1.HCPEtcdBackupConfig{
								Platform: hyperv1.AzureBackupConfigPlatform,
								Azure:    hyperv1.HCPEtcdBackupConfigAzure{EncryptionKeyURL: "https://myvault.vault.azure.net/keys/mykey/version1"},
							},
						},
					},
				},
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.AzureBlob.EncryptionKeyURL).To(Equal("https://myvault.vault.azure.net/keys/mykey/version1"))
			},
		},
		{
			name:    "When setEncryptionFields runs with S3 storage and no managed etcd, It Should leave S3 KMS key ARN empty",
			storage: &hyperv1.HCPEtcdBackupStorage{StorageType: hyperv1.S3BackupStorage, S3: hyperv1.HCPEtcdBackupS3{}},
			hc:      &hyperv1.HostedCluster{Spec: hyperv1.HostedClusterSpec{Etcd: hyperv1.EtcdSpec{}}},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.KMSKeyARN).To(BeEmpty())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setEncryptionFields(tt.storage, tt.hc)
			tt.assert(g, tt.storage)
		})
	}
}

func TestSetCredentialRef(t *testing.T) {
	tests := []struct {
		name    string
		storage *hyperv1.HCPEtcdBackupStorage
		assert  func(*GomegaWithT, *hyperv1.HCPEtcdBackupStorage)
	}{
		{
			name:    "When setCredentialRef runs on S3 backup storage, It Should set S3 credentials reference name",
			storage: &hyperv1.HCPEtcdBackupStorage{StorageType: hyperv1.S3BackupStorage, S3: hyperv1.HCPEtcdBackupS3{}},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.Credentials.Name).To(Equal("my-creds"))
			},
		},
		{
			name:    "When setCredentialRef runs on Azure blob backup storage, It Should set Azure credentials reference name",
			storage: &hyperv1.HCPEtcdBackupStorage{StorageType: hyperv1.AzureBlobBackupStorage, AzureBlob: hyperv1.HCPEtcdBackupAzureBlob{}},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.AzureBlob.Credentials.Name).To(Equal("my-creds"))
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setCredentialRef(tt.storage, "my-creds")
			tt.assert(g, tt.storage)
		})
	}
}

func TestCleanupCredentialSecret(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name           string
		objects        []crclient.Object
		credSecretName string
		assert         func(*GomegaWithT, crclient.Client)
	}{
		{
			name: "When CleanupCredentialSecret runs and credential secret exists, It Should delete the secret",
			objects: []crclient.Object{
				&corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "etcd-backup-creds-test", Namespace: "hypershift"}},
			},
			credSecretName: "etcd-backup-creds-test",
			assert: func(g *GomegaWithT, client crclient.Client) {
				err := client.Get(context.TODO(), types.NamespacedName{Name: "etcd-backup-creds-test", Namespace: "hypershift"}, &corev1.Secret{})
				g.Expect(err).To(HaveOccurred())
			},
		},
		{
			name:           "When CleanupCredentialSecret runs and credential secret is already absent, It Should succeed",
			credSecretName: "nonexistent",
		},
		{
			name:           "When CleanupCredentialSecret runs with empty credential name, It Should succeed without error",
			credSecretName: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := testClient(scheme, tt.objects...)
			o := &Orchestrator{
				log:            logrus.New(),
				client:         client,
				HONamespace:    "hypershift",
				CredSecretName: tt.credSecretName,
			}

			err := o.CleanupCredentialSecret(context.TODO())
			g.Expect(err).NotTo(HaveOccurred())
			if tt.assert != nil {
				tt.assert(g, client)
			}
		})
	}
}

func TestNewOrchestrator(t *testing.T) {
	g := NewWithT(t)
	client := testClient(testScheme())
	o := NewOrchestrator(logrus.New(), client, "hypershift", "openshift-adp")

	g.Expect(o.HONamespace).To(Equal("hypershift"))
	g.Expect(o.OADPNamespace).To(Equal("openshift-adp"))
	g.Expect(o.BackupName).To(BeEmpty())
	g.Expect(o.BackupNamespace).To(BeEmpty())
	g.Expect(o.CredSecretName).To(BeEmpty())
}

func TestIsCreated(t *testing.T) {
	tests := []struct {
		name       string
		backupName string
		want       bool
	}{
		{
			name:       "When BackupName is empty, It Should return false",
			backupName: "",
			want:       false,
		},
		{
			name:       "When BackupName is set, It Should return true",
			backupName: "my-backup",
			want:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			o := &Orchestrator{BackupName: tt.backupName}
			g.Expect(o.IsCreated()).To(Equal(tt.want))
		})
	}
}

func TestFetchBSL(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name      string
		objects   []crclient.Object
		bslName   string
		wantErr   bool
		assert    func(*GomegaWithT, *velerov1.BackupStorageLocation)
	}{
		{
			name: "When BSL exists, It Should return it",
			objects: []crclient.Object{
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
					Spec:       velerov1.BackupStorageLocationSpec{Provider: "aws"},
				},
			},
			bslName: "default",
			assert: func(g *GomegaWithT, bsl *velerov1.BackupStorageLocation) {
				g.Expect(bsl.Name).To(Equal("default"))
				g.Expect(bsl.Spec.Provider).To(Equal("aws"))
			},
		},
		{
			name:    "When BSL does not exist, It Should return error",
			bslName: "nonexistent",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := testClient(scheme, tt.objects...)
			o := &Orchestrator{log: logrus.New(), client: client}

			result, err := o.fetchBSL(context.TODO(), tt.bslName, "openshift-adp")
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			tt.assert(g, result)
		})
	}
}

func TestCreateEtcdBackup(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name      string
		objects   []crclient.Object
		backup    *velerov1.Backup
		hc        *hyperv1.HostedCluster
		wantErr   bool
		errSubstr string
		assert    func(*GomegaWithT, *Orchestrator)
	}{
		{
			name: "When CreateEtcdBackup runs with valid AWS BSL, It Should create HCPEtcdBackup CR",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"cloud": []byte("aws-creds")},
				},
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-bucket", Prefix: "velero"},
						},
						Config: map[string]string{"region": "us-east-1"},
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
							Key:                  "cloud",
						},
					},
				},
			},
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
				Spec:       velerov1.BackupSpec{StorageLocation: "default"},
			},
			hc: &hyperv1.HostedCluster{
				Spec: hyperv1.HostedClusterSpec{
					Etcd: hyperv1.EtcdSpec{
						Managed: &hyperv1.ManagedEtcdSpec{
							Backup: hyperv1.HCPEtcdBackupConfig{
								Platform: hyperv1.AWSBackupConfigPlatform,
								AWS:      hyperv1.HCPEtcdBackupConfigAWS{KMSKeyARN: "arn:aws:kms:us-east-1:123:key/k1"},
							},
						},
					},
				},
			},
			assert: func(g *GomegaWithT, o *Orchestrator) {
				g.Expect(o.BackupName).NotTo(BeEmpty())
				g.Expect(o.BackupNamespace).To(Equal("clusters-test"))
				g.Expect(o.CredSecretName).NotTo(BeEmpty())
			},
		},
		{
			name: "When CreateEtcdBackup runs with nil HostedCluster, It Should succeed without encryption",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"cloud": []byte("aws-creds")},
				},
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "b"},
						},
						Config: map[string]string{"region": "eu-west-1"},
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
							Key:                  "cloud",
						},
					},
				},
			},
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup"},
				Spec:       velerov1.BackupSpec{StorageLocation: "default"},
			},
			assert: func(g *GomegaWithT, o *Orchestrator) {
				g.Expect(o.IsCreated()).To(BeTrue())
			},
		},
		{
			name: "When CreateEtcdBackup runs and BSL not found, It Should return error",
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup"},
				Spec:       velerov1.BackupSpec{StorageLocation: "nonexistent"},
			},
			wantErr:   true,
			errSubstr: "failed to fetch BackupStorageLocation",
		},
		{
			name: "When CreateEtcdBackup runs with unsupported provider, It Should return error",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "creds", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"cloud": []byte("data")},
				},
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "gcp",
						Credential: &corev1.SecretKeySelector{
							LocalObjectReference: corev1.LocalObjectReference{Name: "creds"},
							Key:                  "cloud",
						},
					},
				},
			},
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup"},
				Spec:       velerov1.BackupSpec{StorageLocation: "default"},
			},
			wantErr:   true,
			errSubstr: "failed to map BSL",
		},
		{
			name: "When CreateEtcdBackup runs with BSL without credential ref, It Should use fallback cloud-credentials",
			objects: []crclient.Object{
				&corev1.Secret{
					ObjectMeta: metav1.ObjectMeta{Name: "cloud-credentials", Namespace: "openshift-adp"},
					Data:       map[string][]byte{"cloud": []byte("arn:aws:iam::123456789012:role/my-role")},
				},
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "my-bucket", Prefix: "velero"},
						},
						Config: map[string]string{"region": "us-east-1"},
					},
				},
			},
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup", Namespace: "openshift-adp"},
				Spec:       velerov1.BackupSpec{StorageLocation: "default"},
			},
			assert: func(g *GomegaWithT, o *Orchestrator) {
				g.Expect(o.IsCreated()).To(BeTrue())
				g.Expect(o.CredSecretName).To(Equal("etcd-backup-creds-test-backup"))
			},
		},
		{
			name: "When CreateEtcdBackup runs with BSL without credential ref and fallback secret missing, It Should return error",
			objects: []crclient.Object{
				&velerov1.BackupStorageLocation{
					ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "openshift-adp"},
					Spec: velerov1.BackupStorageLocationSpec{
						Provider: "aws",
						StorageType: velerov1.StorageType{
							ObjectStorage: &velerov1.ObjectStorageLocation{Bucket: "b"},
						},
						Config: map[string]string{"region": "us-east-1"},
					},
				},
			},
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-backup"},
				Spec:       velerov1.BackupSpec{StorageLocation: "default"},
			},
			wantErr:   true,
			errSubstr: "failed to copy credential Secret",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			client := testClient(scheme, tt.objects...)
			o := NewOrchestrator(logrus.New(), client, "hypershift", "openshift-adp")

			err := o.CreateEtcdBackup(context.TODO(), tt.backup, "clusters-test", tt.hc)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			if tt.assert != nil {
				tt.assert(g, o)
			}
		})
	}
}

func TestPollCondition(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name      string
		reason    string
		status    metav1.ConditionStatus
		message   string
		check     func(*metav1.Condition) (bool, error)
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "When condition is already satisfied, It Should return immediately",
			reason: hyperv1.BackupSucceededReason,
			status: metav1.ConditionTrue,
			check: func(cond *metav1.Condition) (bool, error) {
				return cond != nil && cond.Reason == hyperv1.BackupSucceededReason, nil
			},
		},
		{
			name:    "When check returns error, It Should propagate the error",
			reason:  hyperv1.BackupFailedReason,
			status:  metav1.ConditionFalse,
			message: "etcd snapshot failed",
			check: func(cond *metav1.Condition) (bool, error) {
				if cond != nil && cond.Reason == hyperv1.BackupFailedReason {
					return false, fmt.Errorf("backup failed: %s", cond.Message)
				}
				return false, nil
			},
			wantErr:   true,
			errSubstr: "backup failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			eb := &hyperv1.HCPEtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-eb", Namespace: "clusters-test"},
			}
			meta.SetStatusCondition(&eb.Status.Conditions, metav1.Condition{
				Type:    string(hyperv1.BackupCompleted),
				Status:  tt.status,
				Reason:  tt.reason,
				Message: tt.message,
			})

			client := testClient(scheme, eb)
			o := &Orchestrator{
				log:             logrus.New(),
				client:          client,
				BackupName:      "test-eb",
				BackupNamespace: "clusters-test",
			}

			err := o.pollCondition(context.TODO(), 5*time.Second, tt.check)
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func TestVerifyInProgress(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name      string
		reason    string
		wantErr   bool
		errSubstr string
	}{
		{
			name:   "When HCPEtcdBackup is InProgress, It Should return nil",
			reason: hyperv1.BackupInProgressReason,
		},
		{
			name:   "When HCPEtcdBackup already succeeded, It Should return nil",
			reason: hyperv1.BackupSucceededReason,
		},
		{
			name:      "When HCPEtcdBackup failed, It Should return error",
			reason:    hyperv1.BackupFailedReason,
			wantErr:   true,
			errSubstr: "HCPEtcdBackup failed",
		},
		{
			name:      "When HCPEtcdBackup is rejected, It Should return error",
			reason:    hyperv1.BackupRejectedReason,
			wantErr:   true,
			errSubstr: "rejected",
		},
		{
			name:      "When etcd is unhealthy, It Should return error",
			reason:    hyperv1.EtcdUnhealthyReason,
			wantErr:   true,
			errSubstr: "etcd unhealthy",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			eb := &hyperv1.HCPEtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-eb", Namespace: "clusters-test"},
			}
			meta.SetStatusCondition(&eb.Status.Conditions, metav1.Condition{
				Type:    string(hyperv1.BackupCompleted),
				Status:  metav1.ConditionFalse,
				Reason:  tt.reason,
				Message: "test message",
			})

			client := testClient(scheme, eb)
			o := &Orchestrator{
				log:             logrus.New(),
				client:          client,
				BackupName:      "test-eb",
				BackupNamespace: "clusters-test",
			}

			err := o.VerifyInProgress(context.TODO())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
		})
	}
}

func TestWaitForCompletion(t *testing.T) {
	scheme := testScheme()

	tests := []struct {
		name        string
		reason      string
		status      metav1.ConditionStatus
		snapshotURL string
		message     string
		wantErr     bool
		errSubstr   string
		wantURL     string
	}{
		{
			name:        "When HCPEtcdBackup succeeds with snapshotURL, It Should return the URL",
			reason:      hyperv1.BackupSucceededReason,
			status:      metav1.ConditionTrue,
			snapshotURL: "s3://my-bucket/backups/test/etcd-backup/snapshot.db",
			wantURL:     "s3://my-bucket/backups/test/etcd-backup/snapshot.db",
		},
		{
			name:      "When HCPEtcdBackup fails, It Should return error",
			reason:    hyperv1.BackupFailedReason,
			status:    metav1.ConditionFalse,
			message:   "etcd snapshot upload failed",
			wantErr:   true,
			errSubstr: "HCPEtcdBackup failed",
		},
		{
			name:      "When HCPEtcdBackup is rejected, It Should return error",
			reason:    hyperv1.BackupRejectedReason,
			status:    metav1.ConditionFalse,
			message:   "another backup in progress",
			wantErr:   true,
			errSubstr: "rejected",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)

			eb := &hyperv1.HCPEtcdBackup{
				ObjectMeta: metav1.ObjectMeta{Name: "test-eb", Namespace: "clusters-test"},
				Status:     hyperv1.HCPEtcdBackupStatus{SnapshotURL: tt.snapshotURL},
			}
			meta.SetStatusCondition(&eb.Status.Conditions, metav1.Condition{
				Type:    string(hyperv1.BackupCompleted),
				Status:  tt.status,
				Reason:  tt.reason,
				Message: tt.message,
			})

			client := testClient(scheme, eb)
			o := &Orchestrator{
				log:             logrus.New(),
				client:          client,
				BackupName:      "test-eb",
				BackupNamespace: "clusters-test",
			}

			url, err := o.WaitForCompletion(context.TODO())
			if tt.wantErr {
				g.Expect(err).To(HaveOccurred())
				g.Expect(err.Error()).To(ContainSubstring(tt.errSubstr))
				return
			}
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(url).To(Equal(tt.wantURL))
		})
	}
}
