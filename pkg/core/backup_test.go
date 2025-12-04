package core

import (
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

func TestShouldLogProgress(t *testing.T) {
	g := NewWithT(t)

	tests := []struct {
		name               string
		plugin             *BackupPlugin
		currentProgress    float64
		expectedResult     bool
		expectedLoggedProgress float64
		setupFunc          func(*BackupPlugin)
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