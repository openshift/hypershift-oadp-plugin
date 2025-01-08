package common

import (
	"testing"

	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGetMetadataAndAnnotations(t *testing.T) {
	tests := []struct {
		name           string
		item           *unstructured.Unstructured
		expectError    bool
		expectAnnotations map[string]string
	}{
		{
			name: "valid metadata with annotations",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
						"annotations": map[string]interface{}{
							"test": "value",
						},
					},
				},
			},
			expectError: false,
			expectAnnotations: map[string]string{
				"test": "value",
			},
		},
		{
			name: "valid metadata without annotations",
			item: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"metadata": map[string]interface{}{
						"name": "test",
					},
				},
			},
			expectError: false,
			expectAnnotations: map[string]string{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			metadata, annotations, err := getMetadataAndAnnotations(tt.item)
			if tt.expectError {
				g.Expect(err).To(HaveOccurred())
			} else {
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(metadata).NotTo(BeNil())
				g.Expect(annotations).To(Equal(tt.expectAnnotations))
			}
		})
	}
}

func TestValidateCronSchedule(t *testing.T) {
	tests := []struct {
		name      string
		schedule  string
		expectErr bool
	}{
		{"valid cron schedule", "0 0 * * *", false},
		{"valid cron schedule, every 5 min", "*/5 * * * *", false},
		{"Yearly cron schedule", "@yearly", false},
		{"Annually cron schedule", "@annually", false},
		{"Monthly cron schedule", "@monthly", false},
		{"Weekly cron schedule", "@weekly", false},
		{"daily cron schedule", "@daily", false},
		{"at midnight cron schedule", "@midnight", false},
		{"hourly cron schedule", "@hourly", false},
		{"invalid cron schedule with typo", "0 12 * * ?", true},
		{"invalid cron schedule", "invalid schedule", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewWithT(t)
			err := ValidateCronSchedule(tt.schedule)
			g.Expect(err != nil).To(Equal(tt.expectErr))
		})
	}
}
