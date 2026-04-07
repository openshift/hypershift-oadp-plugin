package etcdbackup

// Test scenario names follow: "When <action or context>, It Should <expected outcome>".

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func testScheme() *runtime.Scheme {
	s := runtime.NewScheme()
	_ = hyperv1.AddToScheme(s)
	_ = velerov1.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	return s
}

// mapBSLCase describes one row in TestMapBSLToStorage: either mapBSLToStorage (full path
// including key-prefix resolution) or a direct call to mapAWSBSLToStorage / mapAzureBSLToStorage.
type mapBSLCase struct {
	name         string
	how          mapBSLCaseHow // mapBSL | mapAWS | mapAzure
	bsl          *velerov1.BackupStorageLocation
	mapperPrefix string // second arg to mapAWS/mapAzure; ignored for mapBSL
	wantErr      bool
	errSubstr    string
	assert       func(*GomegaWithT, *hyperv1.HCPEtcdBackupStorage)
}

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
	tests := []mapBSLCase{
		{
			name: "When mapBSLToStorage runs for AWS BSL with bucket and object storage prefix, It Should set S3 bucket region and key prefix",
			how:  mapViaBSL,
			bsl: &velerov1.BackupStorageLocation{
				Spec: velerov1.BackupStorageLocationSpec{
					Provider: "aws",
					StorageType: velerov1.StorageType{
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-bucket",
							Prefix: "velero-backups",
						},
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
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-container",
							Prefix: "velero",
						},
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
						ObjectStorage: &velerov1.ObjectStorageLocation{
							Bucket: "my-bucket",
							Prefix: "my-prefix",
						},
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
				g.Expect(s.S3.KeyPrefix).To(Equal("etcd-backup"))
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

	t.Run("When copyCredentialSecret runs with BSL credential key cloud, It Should remap to credentials key in destination", func(t *testing.T) {
		g := NewWithT(t)

		srcSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cloud-credentials",
				Namespace: "openshift-adp",
			},
			Data: map[string][]byte{"cloud": []byte("aws-creds-data")},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(srcSecret).Build()
		o := &Orchestrator{log: logrus.New(), client: client}

		bsl := &velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: velerov1.BackupStorageLocationSpec{
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
					Key:                  "cloud",
				},
			},
		}

		dstName, err := o.copyCredentialSecret(context.TODO(), bsl, "openshift-adp", "hypershift", "my-backup")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dstName).To(Equal("etcd-backup-creds-my-backup"))

		copied := &corev1.Secret{}
		err = client.Get(context.TODO(), types.NamespacedName{Name: dstName, Namespace: "hypershift"}, copied)
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(copied.Data).To(HaveKey("credentials"))
		g.Expect(copied.Data["credentials"]).To(Equal([]byte("aws-creds-data")))
		g.Expect(copied.Data).NotTo(HaveKey("cloud"))
		g.Expect(copied.Labels["hypershift.openshift.io/etcd-backup"]).To(Equal("true"))
	})

	t.Run("When copyCredentialSecret runs and destination secret already exists, It Should reuse it", func(t *testing.T) {
		g := NewWithT(t)

		existingDst := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "etcd-backup-creds-my-backup",
				Namespace: "hypershift",
			},
			Data: map[string][]byte{"credentials": []byte("existing-data")},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(existingDst).Build()
		o := &Orchestrator{log: logrus.New(), client: client}

		bsl := &velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: velerov1.BackupStorageLocationSpec{
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
					Key:                  "cloud",
				},
			},
		}

		dstName, err := o.copyCredentialSecret(context.TODO(), bsl, "openshift-adp", "hypershift", "my-backup")
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(dstName).To(Equal("etcd-backup-creds-my-backup"))
	})

	t.Run("When copyCredentialSecret runs and source secret lacks the BSL key, It Should return error", func(t *testing.T) {
		g := NewWithT(t)

		srcSecret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "cloud-credentials",
				Namespace: "openshift-adp",
			},
			Data: map[string][]byte{"wrong-key": []byte("data")},
		}

		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(srcSecret).Build()
		o := &Orchestrator{log: logrus.New(), client: client}

		bsl := &velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec: velerov1.BackupStorageLocationSpec{
				Credential: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: "cloud-credentials"},
					Key:                  "cloud",
				},
			},
		}

		_, err := o.copyCredentialSecret(context.TODO(), bsl, "openshift-adp", "hypershift", "my-backup")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("does not contain key"))
	})

	t.Run("When copyCredentialSecret runs without BSL credential reference, It Should return error", func(t *testing.T) {
		g := NewWithT(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		o := &Orchestrator{log: logrus.New(), client: client}

		bsl := &velerov1.BackupStorageLocation{
			ObjectMeta: metav1.ObjectMeta{Name: "default"},
			Spec:       velerov1.BackupStorageLocationSpec{},
		}

		_, err := o.copyCredentialSecret(context.TODO(), bsl, "openshift-adp", "hypershift", "my-backup")
		g.Expect(err).To(HaveOccurred())
		g.Expect(err.Error()).To(ContainSubstring("no credential reference"))
	})
}

func TestSetEncryptionFields(t *testing.T) {
	tests := []struct {
		name   string
		setup  func() (*hyperv1.HCPEtcdBackupStorage, *hyperv1.HostedCluster)
		assert func(*GomegaWithT, *hyperv1.HCPEtcdBackupStorage)
	}{
		{
			name: "When setEncryptionFields runs with S3 storage and HostedCluster AWS KMS, It Should set S3 KMS key ARN",
			setup: func() (*hyperv1.HCPEtcdBackupStorage, *hyperv1.HostedCluster) {
				return &hyperv1.HCPEtcdBackupStorage{
						StorageType: hyperv1.S3BackupStorage,
						S3:          hyperv1.HCPEtcdBackupS3{},
					}, &hyperv1.HostedCluster{
						Spec: hyperv1.HostedClusterSpec{
							Etcd: hyperv1.EtcdSpec{
								Managed: &hyperv1.ManagedEtcdSpec{
									Backup: hyperv1.HCPEtcdBackupConfig{
										Platform: hyperv1.AWSBackupConfigPlatform,
										AWS: hyperv1.HCPEtcdBackupConfigAWS{
											KMSKeyARN: "arn:aws:kms:us-east-1:123456789012:key/test-key",
										},
									},
								},
							},
						},
					}
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.KMSKeyARN).To(Equal("arn:aws:kms:us-east-1:123456789012:key/test-key"))
			},
		},
		{
			name: "When setEncryptionFields runs with Azure storage and HostedCluster encryption URL, It Should set Azure encryption key URL",
			setup: func() (*hyperv1.HCPEtcdBackupStorage, *hyperv1.HostedCluster) {
				return &hyperv1.HCPEtcdBackupStorage{
						StorageType: hyperv1.AzureBlobBackupStorage,
						AzureBlob:   hyperv1.HCPEtcdBackupAzureBlob{},
					}, &hyperv1.HostedCluster{
						Spec: hyperv1.HostedClusterSpec{
							Etcd: hyperv1.EtcdSpec{
								Managed: &hyperv1.ManagedEtcdSpec{
									Backup: hyperv1.HCPEtcdBackupConfig{
										Platform: hyperv1.AzureBackupConfigPlatform,
										Azure: hyperv1.HCPEtcdBackupConfigAzure{
											EncryptionKeyURL: "https://myvault.vault.azure.net/keys/mykey/version1",
										},
									},
								},
							},
						},
					}
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.AzureBlob.EncryptionKeyURL).To(Equal("https://myvault.vault.azure.net/keys/mykey/version1"))
			},
		},
		{
			name: "When setEncryptionFields runs with S3 storage and no managed etcd, It Should leave S3 KMS key ARN empty",
			setup: func() (*hyperv1.HCPEtcdBackupStorage, *hyperv1.HostedCluster) {
				return &hyperv1.HCPEtcdBackupStorage{
						StorageType: hyperv1.S3BackupStorage,
						S3:          hyperv1.HCPEtcdBackupS3{},
					}, &hyperv1.HostedCluster{
						Spec: hyperv1.HostedClusterSpec{Etcd: hyperv1.EtcdSpec{}},
					}
			},
			assert: func(g *GomegaWithT, s *hyperv1.HCPEtcdBackupStorage) {
				g.Expect(s.S3.KMSKeyARN).To(BeEmpty())
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			storage, hc := tt.setup()
			setEncryptionFields(storage, hc)
			tt.assert(g, storage)
		})
	}
}

func TestSetCredentialRef(t *testing.T) {
	tests := []struct {
		name    string
		storage *hyperv1.HCPEtcdBackupStorage
		want    string // credential name on the right branch
		isAzure bool
	}{
		{
			name: "When setCredentialRef runs on S3 backup storage, It Should set S3 credentials reference name",
			storage: &hyperv1.HCPEtcdBackupStorage{
				StorageType: hyperv1.S3BackupStorage,
				S3:          hyperv1.HCPEtcdBackupS3{},
			},
			want: "my-creds",
		},
		{
			name: "When setCredentialRef runs on Azure blob backup storage, It Should set Azure credentials reference name",
			storage: &hyperv1.HCPEtcdBackupStorage{
				StorageType: hyperv1.AzureBlobBackupStorage,
				AzureBlob:   hyperv1.HCPEtcdBackupAzureBlob{},
			},
			want:    "my-creds",
			isAzure: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			setCredentialRef(tt.storage, "my-creds")
			if tt.isAzure {
				g.Expect(tt.storage.AzureBlob.Credentials.Name).To(Equal(tt.want))
			} else {
				g.Expect(tt.storage.S3.Credentials.Name).To(Equal(tt.want))
			}
		})
	}
}

func TestCleanupCredentialSecret(t *testing.T) {
	scheme := testScheme()

	t.Run("When CleanupCredentialSecret runs and credential secret exists, It Should delete the secret", func(t *testing.T) {
		g := NewWithT(t)
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{Name: "etcd-backup-creds-test", Namespace: "hypershift"},
		}
		client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(secret).Build()
		o := &Orchestrator{
			log:            logrus.New(),
			client:         client,
			HONamespace:    "hypershift",
			CredSecretName: "etcd-backup-creds-test",
		}

		err := o.CleanupCredentialSecret(context.TODO())
		g.Expect(err).NotTo(HaveOccurred())

		err = client.Get(context.TODO(), types.NamespacedName{Name: "etcd-backup-creds-test", Namespace: "hypershift"}, &corev1.Secret{})
		g.Expect(err).To(HaveOccurred())
	})

	t.Run("When CleanupCredentialSecret runs and credential secret is already absent, It Should succeed", func(t *testing.T) {
		g := NewWithT(t)
		client := fake.NewClientBuilder().WithScheme(scheme).Build()
		o := &Orchestrator{
			log:            logrus.New(),
			client:         client,
			HONamespace:    "hypershift",
			CredSecretName: "nonexistent",
		}

		err := o.CleanupCredentialSecret(context.TODO())
		g.Expect(err).NotTo(HaveOccurred())
	})

	t.Run("When CleanupCredentialSecret runs with empty credential name, It Should succeed without error", func(t *testing.T) {
		g := NewWithT(t)
		o := &Orchestrator{log: logrus.New()}
		err := o.CleanupCredentialSecret(context.TODO())
		g.Expect(err).NotTo(HaveOccurred())
	})
}
