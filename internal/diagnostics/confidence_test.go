package diagnostics

import "testing"

func TestCalibrateConfidence(t *testing.T) {
	tests := []struct {
		name  string
		input ConfidenceInput
		want  ConfidenceLevel
	}{
		{
			name: "unconfirmed scope max low",
			input: ConfidenceInput{
				ScopeConfirmed:        false,
				HasDirectSupport:      true,
				CriticalRefuteChecked: true,
			},
			want: ConfidenceLow,
		},
		{
			name: "critical tool failure max medium",
			input: ConfidenceInput{
				ScopeConfirmed:        true,
				HasDirectSupport:      true,
				CriticalRefuteChecked: true,
				HasToolFailure:        true,
			},
			want: ConfidenceMedium,
		},
		{
			name: "stale context max low",
			input: ConfidenceInput{
				ScopeConfirmed:        true,
				HasDirectSupport:      true,
				CriticalRefuteChecked: true,
				HasStaleContext:       true,
			},
			want: ConfidenceLow,
		},
		{
			name: "approval required max medium",
			input: ConfidenceInput{
				ScopeConfirmed:        true,
				HasDirectSupport:      true,
				CriticalRefuteChecked: true,
				RequiresApproval:      true,
			},
			want: ConfidenceMedium,
		},
		{
			name: "critical missing max medium",
			input: ConfidenceInput{
				ScopeConfirmed:        true,
				HasDirectSupport:      true,
				CriticalRefuteChecked: true,
				HasCriticalMissing:    true,
			},
			want: ConfidenceMedium,
		},
		{
			name: "confirmed complete evidence allows high",
			input: ConfidenceInput{
				ScopeConfirmed:        true,
				HasDirectSupport:      true,
				CriticalRefuteChecked: true,
			},
			want: ConfidenceHigh,
		},
		{
			name: "missing direct support defaults low",
			input: ConfidenceInput{
				ScopeConfirmed:        true,
				CriticalRefuteChecked: true,
			},
			want: ConfidenceLow,
		},
		{
			name: "missing critical refute check defaults low",
			input: ConfidenceInput{
				ScopeConfirmed:   true,
				HasDirectSupport: true,
			},
			want: ConfidenceLow,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CalibrateConfidence(tt.input); got != tt.want {
				t.Fatalf("CalibrateConfidence() = %q, want %q", got, tt.want)
			}
		})
	}
}
