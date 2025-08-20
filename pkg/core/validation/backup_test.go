package validation

import (
	"context"
	"testing"
	"time"

	snapshotv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	. "github.com/onsi/gomega"
	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	"github.com/sirupsen/logrus"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"
	velerov2alpha1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v2alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestValidatePluginConfig(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		config      map[string]string
		expectError bool
	}{
		{
			name:        "empty config",
			config:      map[string]string{},
			expectError: false,
		},
		{
			name: "valid config with all options",
			config: map[string]string{
				"migration":           "true",
				"readoptNodes":        "true",
				"managedServices":     "true",
				"dataUploadTimeout":   "60",
				"dataUploadCheckPace": "30",
			},
			expectError: false,
		},
		{
			name: "invalid dataUploadTimeout",
			config: map[string]string{
				"dataUploadTimeout": "invalid",
			},
			expectError: true,
		},
		{
			name: "invalid dataUploadCheckPace",
			config: map[string]string{
				"dataUploadCheckPace": "invalid",
			},
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &BackupPluginValidator{
				Log: logrus.New(),
			}

			_, err := p.ValidatePluginConfig(tt.config)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateDataMoverPlatformValidation(t *testing.T) {
	g := NewWithT(t)

	// Helper function to create a test HCP
	createTestHCP := func(platformType hyperv1.PlatformType) *hyperv1.HostedControlPlane {
		return &hyperv1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hcp",
				Namespace: "test-namespace",
			},
			Spec: hyperv1.HostedControlPlaneSpec{
				Platform: hyperv1.PlatformSpec{
					Type: platformType,
				},
			},
		}
	}

	// Helper function to create a test backup
	createTestBackup := func() *velerov1.Backup {
		return &velerov1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-backup",
				Namespace: "test-namespace",
			},
			Spec: velerov1.BackupSpec{
				DefaultVolumesToFsBackup: ptr.To(false),
			},
		}
	}

	tests := []struct {
		name             string
		platformType     hyperv1.PlatformType
		pvBackupFinished bool
		duFinished       bool
		expectError      bool
		errorContains    string
	}{
		{
			name:             "AWS platform with both PV and DU finished",
			platformType:     hyperv1.AWSPlatform,
			pvBackupFinished: true,
			duFinished:       true,
			expectError:      false,
		},
		{
			name:             "Azure platform with PV finished",
			platformType:     hyperv1.AzurePlatform,
			pvBackupFinished: true,
			duFinished:       false,
			expectError:      false,
		},
		{
			name:             "IBM Cloud platform with both PV and DU finished",
			platformType:     hyperv1.IBMCloudPlatform,
			pvBackupFinished: true,
			duFinished:       true,
			expectError:      false,
		},
		{
			name:             "Kubevirt platform with both PV and DU finished",
			platformType:     hyperv1.KubevirtPlatform,
			pvBackupFinished: true,
			duFinished:       true,
			expectError:      false,
		},
		{
			name:             "OpenStack platform with both PV and DU finished",
			platformType:     hyperv1.OpenStackPlatform,
			pvBackupFinished: true,
			duFinished:       true,
			expectError:      false,
		},
		{
			name:             "Agent platform with both PV and DU finished",
			platformType:     hyperv1.AgentPlatform,
			pvBackupFinished: true,
			duFinished:       true,
			expectError:      false,
		},
		{
			name:             "None platform with both PV and DU finished",
			platformType:     hyperv1.NonePlatform,
			pvBackupFinished: true,
			duFinished:       true,
			expectError:      false,
		},
		{
			name:             "Unsupported platform type",
			platformType:     "UnsupportedPlatform",
			pvBackupFinished: false,
			duFinished:       false,
			expectError:      true,
			errorContains:    "unsupported platform type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hcp := createTestHCP(tt.platformType)
			backup := createTestBackup()

			validator := &BackupPluginValidator{
				Log:                 logrus.New(),
				PVBackupFinished:    ptr.To(tt.pvBackupFinished),
				DUFinished:          ptr.To(tt.duFinished),
				DataUploadTimeout:   15 * time.Minute,
				DataUploadCheckPace: 30 * time.Second,
			}

			ctx := context.Background()
			err := validator.ValidateDataMover(ctx, hcp, backup, ptr.To(tt.pvBackupFinished), ptr.To(tt.duFinished))

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
				if tt.errorContains != "" {
					g.Expect(err.Error()).To(ContainSubstring(tt.errorContains))
				}
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateDataMoverWithDifferentPlatforms(t *testing.T) {
	g := NewWithT(t)

	platforms := []hyperv1.PlatformType{
		hyperv1.AWSPlatform,
		hyperv1.AzurePlatform,
		hyperv1.IBMCloudPlatform,
		hyperv1.KubevirtPlatform,
		hyperv1.OpenStackPlatform,
		hyperv1.AgentPlatform,
		hyperv1.NonePlatform,
	}

	for _, platform := range platforms {
		t.Run(string(platform), func(t *testing.T) {
			hcp := &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: hyperv1.HostedControlPlaneSpec{
					Platform: hyperv1.PlatformSpec{
						Type: platform,
					},
				},
			}

			backup := &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test-namespace",
				},
				Spec: velerov1.BackupSpec{
					DefaultVolumesToFsBackup: ptr.To(false),
				},
			}

			validator := &BackupPluginValidator{
				Log:                 logrus.New(),
				PVBackupFinished:    ptr.To(true),
				DUFinished:          ptr.To(true),
				DataUploadTimeout:   15 * time.Minute,
				DataUploadCheckPace: 30 * time.Second,
			}

			ctx := context.Background()
			err := validator.ValidateDataMover(ctx, hcp, backup, ptr.To(true), ptr.To(true))

			// All supported platforms should not error
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestValidateDataMoverWithFinishedStates(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	_ = velerov1.AddToScheme(scheme)
	_ = velerov2alpha1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	tests := []struct {
		name              string
		platformType      hyperv1.PlatformType
		pvBackupFinished  bool
		duFinished        bool
		shouldReturnEarly bool
	}{
		{
			name:              "AWS with both finished - should return early",
			platformType:      hyperv1.AWSPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			shouldReturnEarly: true,
		},
		{
			name:              "AWS with only PV finished - should continue",
			platformType:      hyperv1.AWSPlatform,
			pvBackupFinished:  true,
			duFinished:        false,
			shouldReturnEarly: false,
		},
		{
			name:              "Azure with PV finished - should return early",
			platformType:      hyperv1.AzurePlatform,
			pvBackupFinished:  true,
			duFinished:        false,
			shouldReturnEarly: true,
		},
		{
			name:              "Azure with PV not finished - should continue",
			platformType:      hyperv1.AzurePlatform,
			pvBackupFinished:  false,
			duFinished:        false,
			shouldReturnEarly: false,
		},
		{
			name:              "IBM Cloud with both finished - should return early",
			platformType:      hyperv1.IBMCloudPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			shouldReturnEarly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hcp := &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: hyperv1.HostedControlPlaneSpec{
					Platform: hyperv1.PlatformSpec{
						Type: tt.platformType,
					},
				},
			}

			backup := &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test-namespace",
				},
				Spec: velerov1.BackupSpec{
					DefaultVolumesToFsBackup: ptr.To(false),
				},
			}

			// Create a fake client
			client := fake.NewClientBuilder().WithScheme(scheme).Build()

			validator := &BackupPluginValidator{
				Log:                 logrus.New(),
				Client:              client,
				PVBackupStarted:     ptr.To(false),
				PVBackupFinished:    ptr.To(tt.pvBackupFinished),
				DUStarted:           ptr.To(false),
				DUFinished:          ptr.To(tt.duFinished),
				DataUploadTimeout:   15 * time.Minute,
				DataUploadCheckPace: 30 * time.Second,
				NPaused:             ptr.To(false),
				HCPaused:            ptr.To(false),
			}

			ctx := context.Background()
			err := validator.ValidateDataMover(ctx, hcp, backup, ptr.To(tt.pvBackupFinished), ptr.To(tt.duFinished))

			// Should not error in any case
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestValidateDataMoverWithClient(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	_ = velerov1.AddToScheme(scheme)
	_ = velerov2alpha1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	hcp := &hyperv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: hyperv1.HostedControlPlaneSpec{
			Platform: hyperv1.PlatformSpec{
				Type: hyperv1.AWSPlatform,
			},
		},
	}

	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "test-namespace",
		},
		Spec: velerov1.BackupSpec{
			DefaultVolumesToFsBackup: ptr.To(false),
		},
	}

	// Create a fake client
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	validator := &BackupPluginValidator{
		Log:                 logrus.New(),
		Client:              client,
		PVBackupStarted:     ptr.To(false),
		PVBackupFinished:    ptr.To(false),
		DUStarted:           ptr.To(false),
		DUFinished:          ptr.To(false),
		DataUploadTimeout:   15 * time.Minute,
		DataUploadCheckPace: 30 * time.Second,
		NPaused:             ptr.To(false),
		HCPaused:            ptr.To(false),
	}

	ctx := context.Background()
	err := validator.ValidateDataMover(ctx, hcp, backup, ptr.To(false), ptr.To(false))

	// Should not error even with a real client
	g.Expect(err).ToNot(HaveOccurred())
}

func TestValidateDataMoverEdgeCases(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name        string
		hcp         *hyperv1.HostedControlPlane
		backup      *velerov1.Backup
		validator   *BackupPluginValidator
		expectError bool
	}{
		{
			name: "nil HCP",
			hcp:  nil,
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test-namespace",
				},
				Spec: velerov1.BackupSpec{
					DefaultVolumesToFsBackup: ptr.To(false),
				},
			},
			validator: &BackupPluginValidator{
				Log:                 logrus.New(),
				PVBackupFinished:    ptr.To(false),
				DUFinished:          ptr.To(false),
				DataUploadTimeout:   15 * time.Minute,
				DataUploadCheckPace: 30 * time.Second,
				NPaused:             ptr.To(false),
				HCPaused:            ptr.To(false),
			},
			expectError: true,
		},
		{
			name: "nil backup",
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: hyperv1.HostedControlPlaneSpec{
					Platform: hyperv1.PlatformSpec{
						Type: hyperv1.AWSPlatform,
					},
				},
			},
			backup: nil,
			validator: &BackupPluginValidator{
				Log:                 logrus.New(),
				PVBackupFinished:    ptr.To(false),
				DUFinished:          ptr.To(false),
				DataUploadTimeout:   15 * time.Minute,
				DataUploadCheckPace: 30 * time.Second,
			},
			expectError: true,
		},
		{
			name: "nil validator",
			hcp: &hyperv1.HostedControlPlane{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-hcp",
					Namespace: "test-namespace",
				},
				Spec: hyperv1.HostedControlPlaneSpec{
					Platform: hyperv1.PlatformSpec{
						Type: hyperv1.AWSPlatform,
					},
				},
			},
			backup: &velerov1.Backup{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-backup",
					Namespace: "test-namespace",
				},
				Spec: velerov1.BackupSpec{
					DefaultVolumesToFsBackup: ptr.To(false),
				},
			},
			validator:   nil,
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()

			if tt.validator == nil || tt.hcp == nil || tt.backup == nil {
				// Test with nil validator, hcp, or backup - this should panic
				g.Expect(func() {
					err := tt.validator.ValidateDataMover(ctx, tt.hcp, tt.backup, ptr.To(false), ptr.To(false))
					g.Expect(err).To(HaveOccurred())
				}).To(Panic())
				return
			}

			err := tt.validator.ValidateDataMover(ctx, tt.hcp, tt.backup, ptr.To(false), ptr.To(false))

			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).ToNot(HaveOccurred())
			}
		})
	}
}

func TestValidateDataMoverWithHighAvailability(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	_ = velerov1.AddToScheme(scheme)
	_ = velerov2alpha1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	hcp := &hyperv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: hyperv1.HostedControlPlaneSpec{
			Platform: hyperv1.PlatformSpec{
				Type: hyperv1.AWSPlatform,
			},
		},
	}

	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "test-namespace",
		},
		Spec: velerov1.BackupSpec{
			DefaultVolumesToFsBackup: ptr.To(false),
		},
	}

	// Create a fake client
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	validator := &BackupPluginValidator{
		Log:                 logrus.New(),
		Client:              client,
		Backup:              backup,
		HA:                  true, // Enable HA
		DataUploadTimeout:   15 * time.Minute,
		DataUploadCheckPace: 30 * time.Second,
		PVBackupStarted:     ptr.To(false),
		PVBackupFinished:    ptr.To(false),
		DUStarted:           ptr.To(false),
		DUFinished:          ptr.To(false),
		NPaused:             ptr.To(false),
		HCPaused:            ptr.To(false),
	}

	ctx := context.Background()
	err := validator.ValidateDataMover(ctx, hcp, backup, ptr.To(false), ptr.To(false))

	// Should not error even with HA enabled
	g.Expect(err).ToNot(HaveOccurred())
}

func TestValidateDataMoverWithDifferentTimeouts(t *testing.T) {
	g := NewWithT(t)

	scheme := runtime.NewScheme()
	_ = velerov1.AddToScheme(scheme)
	_ = velerov2alpha1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	hcp := &hyperv1.HostedControlPlane{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-hcp",
			Namespace: "test-namespace",
		},
		Spec: hyperv1.HostedControlPlaneSpec{
			Platform: hyperv1.PlatformSpec{
				Type: hyperv1.AWSPlatform,
			},
		},
	}

	backup := &velerov1.Backup{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-backup",
			Namespace: "test-namespace",
		},
		Spec: velerov1.BackupSpec{
			DefaultVolumesToFsBackup: ptr.To(false),
		},
	}

	// Create a fake client
	client := fake.NewClientBuilder().WithScheme(scheme).Build()

	timeoutTests := []struct {
		name                string
		dataUploadTimeout   time.Duration
		dataUploadCheckPace time.Duration
	}{
		{
			name:                "Short timeouts",
			dataUploadTimeout:   5 * time.Minute,
			dataUploadCheckPace: 10 * time.Second,
		},
		{
			name:                "Long timeouts",
			dataUploadTimeout:   60 * time.Minute,
			dataUploadCheckPace: 60 * time.Second,
		},
		{
			name:                "Zero timeouts",
			dataUploadTimeout:   0,
			dataUploadCheckPace: 0,
		},
	}

	for _, tt := range timeoutTests {
		t.Run(tt.name, func(t *testing.T) {
			validator := &BackupPluginValidator{
				Log:                 logrus.New(),
				Client:              client,
				Backup:              backup,
				HA:                  false,
				DataUploadTimeout:   tt.dataUploadTimeout,
				DataUploadCheckPace: tt.dataUploadCheckPace,
				PVBackupStarted:     ptr.To(false),
				PVBackupFinished:    ptr.To(false),
				DUStarted:           ptr.To(false),
				DUFinished:          ptr.To(false),
				NPaused:             ptr.To(false),
				HCPaused:            ptr.To(false),
			}

			ctx := context.Background()
			err := validator.ValidateDataMover(ctx, hcp, backup, ptr.To(false), ptr.To(false))

			// Should not error with different timeout configurations
			g.Expect(err).ToNot(HaveOccurred())
		})
	}
}

func TestValidateDataMoverWithDifferentDataUploadStates(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = velerov1.AddToScheme(scheme)
	_ = velerov2alpha1.AddToScheme(scheme)
	_ = hyperv1.AddToScheme(scheme)
	_ = snapshotv1.AddToScheme(scheme)

	// Helper function to create a test HCP
	createTestHCP := func(platformType hyperv1.PlatformType) *hyperv1.HostedControlPlane {
		return &hyperv1.HostedControlPlane{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-hcp",
				Namespace: "test-namespace",
			},
			Spec: hyperv1.HostedControlPlaneSpec{
				Platform: hyperv1.PlatformSpec{
					Type: platformType,
				},
			},
		}
	}

	// Helper function to create a test backup
	createTestBackup := func() *velerov1.Backup {
		return &velerov1.Backup{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-backup",
				Namespace: "velero",
			},
			Spec: velerov1.BackupSpec{
				DefaultVolumesToFsBackup: ptr.To(false),
			},
		}
	}

	tests := []struct {
		name              string
		platformType      hyperv1.PlatformType
		pvBackupFinished  bool
		duFinished        bool
		expectedBehavior  string
		shouldReturnEarly bool
		objects           []client.Object
	}{
		{
			name:              "AWS platform - both PV and DU finished - early return path",
			platformType:      hyperv1.AWSPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			expectedBehavior:  "should return early when both PV and DU are finished",
			shouldReturnEarly: true,
		},
		{
			name:              "AWS platform - only PV finished - continue path",
			platformType:      hyperv1.AWSPlatform,
			pvBackupFinished:  true,
			duFinished:        false,
			expectedBehavior:  "should continue processing when only PV is finished",
			shouldReturnEarly: false,
		},
		{
			name:              "Azure platform - PV finished - early return path",
			platformType:      hyperv1.AzurePlatform,
			pvBackupFinished:  true,
			duFinished:        false,
			expectedBehavior:  "should return early when PV is finished (Azure only needs PV)",
			shouldReturnEarly: true,
		},
		{
			name:              "IBM Cloud platform - both PV and DU finished - early return path",
			platformType:      hyperv1.IBMCloudPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			expectedBehavior:  "should return early when both PV and DU are finished",
			shouldReturnEarly: true,
		},
		{
			name:              "IBM Cloud platform - only PV finished - continue path",
			platformType:      hyperv1.IBMCloudPlatform,
			pvBackupFinished:  true,
			duFinished:        false,
			expectedBehavior:  "should continue processing when only DU is finished",
			shouldReturnEarly: false,
		},
		{
			name:              "Kubevirt platform - both PV and DU finished - early return path",
			platformType:      hyperv1.KubevirtPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			expectedBehavior:  "should return early when both PV and DU are finished",
			shouldReturnEarly: true,
		},
		{
			name:              "OpenStack platform - both PV and DU finished - early return path",
			platformType:      hyperv1.OpenStackPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			expectedBehavior:  "should return early when both PV and DU are finished",
			shouldReturnEarly: true,
		},
		{
			name:              "Agent platform - both PV and DU finished - early return path",
			platformType:      hyperv1.AgentPlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			expectedBehavior:  "should return early when both PV and DU are finished",
			shouldReturnEarly: true,
		},
		{
			name:              "None platform - both PV and DU finished - early return path",
			platformType:      hyperv1.NonePlatform,
			pvBackupFinished:  true,
			duFinished:        true,
			expectedBehavior:  "should return early when both PV and DU are finished",
			shouldReturnEarly: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			hcp := createTestHCP(tt.platformType)
			backup := createTestBackup()

			// Create a fake client
			client := fake.NewClientBuilder().WithScheme(scheme).WithObjects(tt.objects...).Build()

			validator := &BackupPluginValidator{
				Log:                 logrus.New(),
				Client:              client,
				Backup:              backup,
				HA:                  false,
				DataUploadTimeout:   15 * time.Minute,
				DataUploadCheckPace: 30 * time.Second,
				PVBackupStarted:     ptr.To(false),
				PVBackupFinished:    ptr.To(tt.pvBackupFinished),
				DUStarted:           ptr.To(false),
				DUFinished:          ptr.To(tt.duFinished),
				NPaused:             ptr.To(false),
				HCPaused:            ptr.To(false),
			}

			ctx := context.Background()

			// Create pointers for the function parameters
			pvBackupFinishedParam := ptr.To(tt.pvBackupFinished)
			duFinishedParam := ptr.To(tt.duFinished)

			err := validator.ValidateDataMover(ctx, hcp, backup, pvBackupFinishedParam, duFinishedParam)

			// Should not error in any case
			g.Expect(err).ToNot(HaveOccurred())

			// Verify the behavior based on the expected path
			if tt.shouldReturnEarly {
				// For early return cases, the parameters should be updated to reflect the finished state
				if tt.platformType == hyperv1.AzurePlatform {
					// Azure only needs PV to be finished
					g.Expect(*pvBackupFinishedParam).To(BeTrue())
				} else {
					// Other platforms need both PV and DU to be finished
					g.Expect(*pvBackupFinishedParam).To(BeTrue())
					g.Expect(*duFinishedParam).To(BeTrue())
				}
			} else {
				// For continue cases, the parameters should remain as they were
				g.Expect(*pvBackupFinishedParam).To(Equal(tt.pvBackupFinished))
				g.Expect(*duFinishedParam).To(Equal(tt.duFinished))
			}
		})
	}
}
