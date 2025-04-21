package validation

import (
	"testing"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
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
