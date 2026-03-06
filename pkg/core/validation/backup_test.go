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
				"migration":       "true",
				"readoptNodes":    "true",
				"managedServices": "true",
			},
			expectError: false,
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
