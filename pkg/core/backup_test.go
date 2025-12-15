package core

import (
	"context"
	"testing"
	"time"

	hyperv1 "github.com/openshift/hypershift/api/hypershift/v1beta1"
	velerov1 "github.com/vmware-tanzu/velero/pkg/apis/velero/v1"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestShouldLogProgress(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name                   string
		plugin                 *BackupPlugin
		currentProgress        float64
		expectedResult         bool
		expectedLoggedProgress float64
		setupFunc              func(*BackupPlugin)
	}{
		{
			name: "first time logging - should log",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        10.0,
			expectedResult:         true,
			expectedLoggedProgress: 10.0,
		},
		{
			name: "progress increased by 5% - should log",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        15.0,
			expectedResult:         true,
			expectedLoggedProgress: 15.0,
			setupFunc: func(p *BackupPlugin) {
				p.lastLoggedProgress = 10.0
				p.lastLogTime = time.Now().Add(-10 * time.Second)
			},
		},
		{
			name: "progress increased by less than 5% - should not log",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        12.0,
			expectedResult:         false,
			expectedLoggedProgress: 10.0,
			setupFunc: func(p *BackupPlugin) {
				p.lastLoggedProgress = 10.0
				p.lastLogTime = time.Now().Add(-10 * time.Second)
			},
		},
		{
			name: "progress same but 30+ seconds passed - should log",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        10.0,
			expectedResult:         true,
			expectedLoggedProgress: 10.0,
			setupFunc: func(p *BackupPlugin) {
				p.lastLoggedProgress = 10.0
				p.lastLogTime = time.Now().Add(-35 * time.Second)
			},
		},
		{
			name: "progress same but less than 30 seconds passed - should not log",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        10.0,
			expectedResult:         false,
			expectedLoggedProgress: 10.0,
			setupFunc: func(p *BackupPlugin) {
				p.lastLoggedProgress = 10.0
				p.lastLogTime = time.Now().Add(-15 * time.Second)
			},
		},
		{
			name: "large progress jump - should log",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        50.0,
			expectedResult:         true,
			expectedLoggedProgress: 50.0,
			setupFunc: func(p *BackupPlugin) {
				p.lastLoggedProgress = 10.0
				p.lastLogTime = time.Now().Add(-5 * time.Second)
			},
		},
		{
			name: "progress decreased (edge case) - should log if 5% difference",
			plugin: &BackupPlugin{
				log: logrus.New(),
			},
			currentProgress:        5.0,
			expectedResult:         false, // 10-5 = 5, but the condition is currentProgress-lastLogged >= 5.0
			expectedLoggedProgress: 10.0,
			setupFunc: func(p *BackupPlugin) {
				p.lastLoggedProgress = 10.0
				p.lastLogTime = time.Now().Add(-5 * time.Second)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupFunc != nil {
				tt.setupFunc(tt.plugin)
			}

			initialLoggedProgress := tt.plugin.lastLoggedProgress
			initialLogTime := tt.plugin.lastLogTime

			result := tt.plugin.shouldLogProgress(tt.currentProgress)
			g.Expect(result).To(Equal(tt.expectedResult))

			if tt.expectedResult {
				// If we expected to log, check that the values were updated
				g.Expect(tt.plugin.lastLoggedProgress).To(Equal(tt.currentProgress))
				g.Expect(tt.plugin.lastLogTime).To(BeTemporally(">", initialLogTime))
			} else {
				// If we didn't expect to log, values should remain unchanged
				g.Expect(tt.plugin.lastLoggedProgress).To(Equal(initialLoggedProgress))
				g.Expect(tt.plugin.lastLogTime).To(Equal(initialLogTime))
			}
		})
	}
}

func TestBackupPluginPauseAll(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		initialHCPaused  bool
		initialNPPaused  bool
		expectedHCPaused bool
		expectedNPPaused bool
		setupHCWithError bool
		setupNPWithError bool
		description      string
	}{
		{
			name:             "pause_both_when_neither_paused",
			initialHCPaused:  false,
			initialNPPaused:  false,
			expectedHCPaused: true,
			expectedNPPaused: true,
			setupHCWithError: false,
			setupNPWithError: false,
			description:      "Should pause both resources when neither is paused",
		},
		{
			name:             "pause_only_hc_when_np_already_paused",
			initialHCPaused:  false,
			initialNPPaused:  true,
			expectedHCPaused: true,
			expectedNPPaused: true,
			setupHCWithError: false,
			setupNPWithError: false,
			description:      "Should pause only HostedCluster when NodePool is already paused",
		},
		{
			name:             "pause_only_np_when_hc_already_paused",
			initialHCPaused:  true,
			initialNPPaused:  false,
			expectedHCPaused: true,
			expectedNPPaused: true,
			setupHCWithError: false,
			setupNPWithError: false,
			description:      "Should pause only NodePool when HostedCluster is already paused",
		},
		{
			name:             "no_action_when_both_already_paused",
			initialHCPaused:  true,
			initialNPPaused:  true,
			expectedHCPaused: true,
			expectedNPPaused: true,
			setupHCWithError: false,
			setupNPWithError: false,
			description:      "Should take no action when both resources are already paused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test case: %s", tt.description)

			// Create scheme and fake client
			scheme := runtime.NewScheme()
			_ = hyperv1.AddToScheme(scheme)

			// Setup initial objects
			hcList := &hyperv1.HostedClusterList{
				Items: []hyperv1.HostedCluster{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-hc",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.HostedClusterSpec{
							PausedUntil: func() *string {
								if tt.initialHCPaused {
									return ptr.To("true")
								}
								return nil
							}(),
						},
					},
				},
			}

			npList := &hyperv1.NodePoolList{
				Items: []hyperv1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-np",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.NodePoolSpec{
							PausedUntil: func() *string {
								if tt.initialNPPaused {
									return ptr.To("true")
								}
								return nil
							}(),
						},
					},
				},
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(hcList, npList).Build()

			plugin := &BackupPlugin{
				log:      logrus.New(),
				client:   client,
				hcPaused: tt.initialHCPaused,
				npPaused: tt.initialNPPaused,
			}

			backup := &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedNamespaces: []string{"test-namespace"},
				},
			}

			// Execute pauseAll
			err := plugin.pauseAll(context.Background(), backup)

			// For this simplified test, we expect no errors since we're using a working fake client
			g.Expect(err).NotTo(HaveOccurred())

			// Verify internal state changes
			g.Expect(plugin.hcPaused).To(Equal(tt.expectedHCPaused),
				"HostedCluster pause state should match expected")

			g.Expect(plugin.npPaused).To(Equal(tt.expectedNPPaused),
				"NodePool pause state should match expected")
		})
	}
}

func TestBackupPluginUnPauseAll(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name             string
		initialHCPaused  bool
		initialNPPaused  bool
		expectedHCPaused bool
		expectedNPPaused bool
		description      string
	}{
		{
			name:             "unpause_both_when_both_paused",
			initialHCPaused:  true,
			initialNPPaused:  true,
			expectedHCPaused: false,
			expectedNPPaused: false,
			description:      "Should unpause both resources when both are paused",
		},
		{
			name:             "unpause_only_hc_when_np_not_paused",
			initialHCPaused:  true,
			initialNPPaused:  false,
			expectedHCPaused: false,
			expectedNPPaused: false,
			description:      "Should unpause only HostedCluster when NodePool is not paused",
		},
		{
			name:             "unpause_only_np_when_hc_not_paused",
			initialHCPaused:  false,
			initialNPPaused:  true,
			expectedHCPaused: false,
			expectedNPPaused: false,
			description:      "Should unpause only NodePool when HostedCluster is not paused",
		},
		{
			name:             "no_action_when_neither_paused",
			initialHCPaused:  false,
			initialNPPaused:  false,
			expectedHCPaused: false,
			expectedNPPaused: false,
			description:      "Should take no action when neither resource is paused",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Logf("Test case: %s", tt.description)

			// Create scheme and fake client
			scheme := runtime.NewScheme()
			_ = hyperv1.AddToScheme(scheme)

			// Setup initial objects
			hcList := &hyperv1.HostedClusterList{
				Items: []hyperv1.HostedCluster{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-hc",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.HostedClusterSpec{
							PausedUntil: func() *string {
								if tt.initialHCPaused {
									return ptr.To("true")
								}
								return nil
							}(),
						},
					},
				},
			}

			npList := &hyperv1.NodePoolList{
				Items: []hyperv1.NodePool{
					{
						ObjectMeta: metav1.ObjectMeta{
							Name:      "test-np",
							Namespace: "test-namespace",
						},
						Spec: hyperv1.NodePoolSpec{
							PausedUntil: func() *string {
								if tt.initialNPPaused {
									return ptr.To("true")
								}
								return nil
							}(),
						},
					},
				},
			}

			client := fake.NewClientBuilder().WithScheme(scheme).WithLists(hcList, npList).Build()

			plugin := &BackupPlugin{
				log:      logrus.New(),
				client:   client,
				hcPaused: tt.initialHCPaused,
				npPaused: tt.initialNPPaused,
			}

			backup := &velerov1.Backup{
				Spec: velerov1.BackupSpec{
					IncludedNamespaces: []string{"test-namespace"},
				},
			}

			// Execute unPauseAll
			err := plugin.unPauseAll(context.Background(), backup)

			// For this simplified test, we expect no errors since we're using a working fake client
			g.Expect(err).NotTo(HaveOccurred())

			// Verify internal state changes
			g.Expect(plugin.hcPaused).To(Equal(tt.expectedHCPaused),
				"HostedCluster pause state should match expected")

			g.Expect(plugin.npPaused).To(Equal(tt.expectedNPPaused),
				"NodePool pause state should match expected")
		})
	}
}
