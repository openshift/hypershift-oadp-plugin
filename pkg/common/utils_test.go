package common

import (
	"testing"

	. "github.com/onsi/gomega"
)

func TestValidaetCronSchedule(t *testing.T) {
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
