package diagnostics

import (
	"strings"
	"testing"
)

func TestRedactSensitiveTextRemovesSecretsButKeepsEvidence(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		forbidden []string
		mustKeep  []string
	}{
		{
			name:      "redis url credential",
			input:     "redis slowlog from redis://:secret@127.0.0.1:6379/0 shows long commands",
			forbidden: []string{"secret"},
			mustKeep:  []string{"redis slowlog", "127.0.0.1:6379/0", "shows long commands"},
		},
		{
			name:      "bearer token",
			input:     "Authorization: Bearer abc.def for metrics API returned 401",
			forbidden: []string{"abc.def"},
			mustKeep:  []string{"Authorization: Bearer", "metrics API returned 401"},
		},
		{
			name:      "password key value",
			input:     "probe failed with password=my-pass while checking mysql",
			forbidden: []string{"my-pass"},
			mustKeep:  []string{"probe failed", "password=", "checking mysql"},
		},
		{
			name:      "standalone api key",
			input:     "missing evidence needs sk-test-value for replay but must not leak",
			forbidden: []string{"sk-test-value"},
			mustKeep:  []string{"missing evidence needs", "for replay"},
		},
		{
			name:      "plain evidence",
			input:     "node cpu saturation observed for pod checkout-api",
			forbidden: nil,
			mustKeep:  []string{"node cpu saturation", "checkout-api"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSensitiveText(tc.input)
			for _, forbidden := range tc.forbidden {
				if strings.Contains(got, forbidden) {
					t.Fatalf("redacted text still contains %q: %q", forbidden, got)
				}
			}
			for _, keep := range tc.mustKeep {
				if !strings.Contains(got, keep) {
					t.Fatalf("redacted text %q does not preserve useful evidence %q", got, keep)
				}
			}
		})
	}
}

func TestRedactTraceRedactsAllTextualEvidenceAndFailures(t *testing.T) {
	trace := DiagnosticTrace{
		ScopeSummary:     "db url mysql://root:secret@db.internal:3306/app",
		Hypotheses:       []string{"password=my-pass caused auth errors"},
		ObservedEvidence: []string{"Authorization: Bearer abc.def returned 403"},
		RefutingEvidence: []string{"token=plain-token did not reproduce on staging"},
		MissingEvidence:  []string{"need API key api_key=sk-live-value to validate"},
		ToolFailures: []ToolFailure{{
			ToolName: "mysql",
			Semantic: ToolFailurePermissionDenied,
			Detail:   "permission denied for mysql://root:secret@db.internal:3306/app",
		}},
		ConfidenceReason: "secret was visible in failed command",
	}

	redacted := RedactTrace(trace)
	encoded := strings.Join([]string{
		redacted.ScopeSummary,
		strings.Join(redacted.Hypotheses, " "),
		strings.Join(redacted.ObservedEvidence, " "),
		strings.Join(redacted.RefutingEvidence, " "),
		strings.Join(redacted.MissingEvidence, " "),
		redacted.ToolFailures[0].Detail,
		redacted.ConfidenceReason,
	}, " ")

	for _, forbidden := range []string{"my-pass", "abc.def", "plain-token", "sk-live-value", "root:secret"} {
		if strings.Contains(encoded, forbidden) {
			t.Fatalf("redacted trace still contains %q in %q", forbidden, encoded)
		}
	}
	if !strings.Contains(encoded, "db.internal:3306/app") {
		t.Fatalf("redacted trace lost useful URL location: %q", encoded)
	}
}
