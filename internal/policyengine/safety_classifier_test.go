package policyengine

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/tooling"

	"pgregory.net/rapid"
)

func TestSafetyClassifierDetectsDestructiveWorkaroundCategories(t *testing.T) {
	cases := []struct {
		name     string
		args     string
		category SafetySignalCategory
		severity SafetySeverity
	}{
		{
			name:     "force",
			args:     `{"command":"apply change --force","action":"force overwrite"}`,
			category: SafetySignalForce,
			severity: SafetySeverityHigh,
		},
		{
			name:     "skip validation",
			args:     `{"command":"deploy --no-verify","validation":"skip preflight checks"}`,
			category: SafetySignalSkipValidation,
			severity: SafetySeverityHigh,
		},
		{
			name:     "delete lock",
			args:     `{"action":"remove lock file","target":"synthetic.lock"}`,
			category: SafetySignalDeleteLock,
			severity: SafetySeverityCritical,
		},
		{
			name:     "disable guard",
			args:     `{"action":"disable safety guard","reason":"bypass protection"}`,
			category: SafetySignalDisableGuard,
			severity: SafetySeverityCritical,
		},
		{
			name:     "reset state",
			args:     `{"command":"reset state to baseline","mode":"hard reset"}`,
			category: SafetySignalResetState,
			severity: SafetySeverityHigh,
		},
		{
			name:     "overwrite unexpected",
			args:     `{"action":"overwrite conflict","state":"unexpected drift detected"}`,
			category: SafetySignalOverwriteUnexpected,
			severity: SafetySeverityCritical,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			signals := DetectSafetySignals(PolicyInput{
				ToolName:  "synthetic.read_tool",
				Tool:      tooling.ToolMetadata{Name: "synthetic.read_tool", RiskLevel: tooling.ToolRiskLow},
				Arguments: json.RawMessage(tc.args),
			})

			signal, ok := safetySignalForTest(signals, tc.category)
			if !ok {
				t.Fatalf("DetectSafetySignals() = %#v, want category %s", signals, tc.category)
			}
			if signal.Severity != tc.severity {
				t.Fatalf("signal severity = %s, want %s", signal.Severity, tc.severity)
			}
			if len(signal.Reasons) == 0 {
				t.Fatalf("signal reasons empty for %s", tc.category)
			}
		})
	}
}

func TestSafetyClassifierEscalatesReadOnlyToolWithBypassArguments(t *testing.T) {
	input := PolicyInput{
		ToolName: "synthetic.status",
		Tool: tooling.ToolMetadata{
			Name:      "synthetic.status",
			RiskLevel: tooling.ToolRiskLow,
			Mutating:  false,
		},
		Arguments: json.RawMessage(`{"action":"skip validation and force overwrite","destination":"synthetic.resource"}`),
	}

	decision := (&ExecuteModePolicy{}).CheckTool(input)
	if decision.Action != PolicyActionNeedApproval {
		t.Fatalf("ExecuteModePolicy.CheckTool() = %#v, want approval for read-only bypass args", decision)
	}
	if !strings.Contains(decision.Reason, "safety signal") {
		t.Fatalf("decision reason = %q, want safety signal", decision.Reason)
	}
	if decision.Approval == nil || len(decision.Approval.SafetySignals) == 0 {
		t.Fatalf("approval = %#v, want safety signals attached", decision.Approval)
	}
}

func TestSafetyClassifierDeniesDestructiveWorkaroundOutsideExecuteMode(t *testing.T) {
	input := PolicyInput{
		ToolName:  "synthetic.status",
		Tool:      tooling.ToolMetadata{Name: "synthetic.status", RiskLevel: tooling.ToolRiskLow},
		Arguments: json.RawMessage(`{"action":"disable guard and reset state"}`),
	}

	decision := (&PlanModePolicy{}).CheckTool(input)
	if decision.Action != PolicyActionDeny {
		t.Fatalf("PlanModePolicy.CheckTool() = %#v, want deny for safety signal", decision)
	}
	if len(decision.SafetySignals) == 0 {
		t.Fatalf("decision safety signals empty: %#v", decision)
	}
}

func TestSafetyClassifierHighRiskMetadataRequiresApproval(t *testing.T) {
	input := PolicyInput{
		ToolName: "synthetic.high_risk_read",
		Tool: tooling.ToolMetadata{
			Name:      "synthetic.high_risk_read",
			RiskLevel: tooling.ToolRiskHigh,
		},
		Arguments: json.RawMessage(`{"query":"synthetic status"}`),
	}

	decision := (&ExecuteModePolicy{}).CheckTool(input)
	if decision.Action != PolicyActionNeedApproval {
		t.Fatalf("ExecuteModePolicy.CheckTool() = %#v, want approval for high-risk metadata", decision)
	}
	if !hasSafetySeverityForTest(decision.SafetySignals, SafetySeverityHigh) {
		t.Fatalf("decision safety signals = %#v, want high severity", decision.SafetySignals)
	}
}

func TestGatewayPolicySafetySignalsPrecedeWhitelistBypass(t *testing.T) {
	now := time.Now().UTC()
	manager := NewWhitelistManager()
	if err := manager.Create(WhitelistEntry{
		ID:        "synthetic-allow",
		HostID:    "synthetic-host",
		ToolName:  "service_restart",
		Command:   "restart synthetic --force",
		TTL:       time.Hour,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("Create() error = %v", err)
	}

	decision := (&GatewayPolicy{Whitelist: manager}).CheckApproval("service_restart", "restart synthetic --force", "synthetic-host", now)
	if decision.Action != PolicyActionNeedApproval {
		t.Fatalf("CheckApproval() = %#v, want safety signal to require approval despite whitelist", decision)
	}
	if decision.Approval == nil || len(decision.Approval.SafetySignals) == 0 {
		t.Fatalf("approval = %#v, want safety signals", decision.Approval)
	}
}

func TestPropertyDestructiveWorkaroundCategoriesCannotAllowWithoutApproval(t *testing.T) {
	categoryArgs := map[SafetySignalCategory]string{
		SafetySignalForce:               `{"action":"force overwrite"}`,
		SafetySignalSkipValidation:      `{"action":"skip validation"}`,
		SafetySignalDeleteLock:          `{"action":"delete lock"}`,
		SafetySignalDisableGuard:        `{"action":"disable guard"}`,
		SafetySignalResetState:          `{"action":"reset state"}`,
		SafetySignalOverwriteUnexpected: `{"action":"overwrite unexpected conflict"}`,
	}

	rapid.Check(t, func(t *rapid.T) {
		category := rapid.SampledFrom([]SafetySignalCategory{
			SafetySignalForce,
			SafetySignalSkipValidation,
			SafetySignalDeleteLock,
			SafetySignalDisableGuard,
			SafetySignalResetState,
			SafetySignalOverwriteUnexpected,
		}).Draw(t, "category")

		decision := (&ExecuteModePolicy{}).CheckTool(PolicyInput{
			ToolName:  "synthetic.read_tool",
			Tool:      tooling.ToolMetadata{Name: "synthetic.read_tool", RiskLevel: tooling.ToolRiskLow},
			Arguments: json.RawMessage(categoryArgs[category]),
		})
		if decision.Action == PolicyActionAllow {
			t.Fatalf("category %s should not allow without approval: %#v", category, decision)
		}
	})
}

func safetySignalForTest(signals []SafetySignal, category SafetySignalCategory) (SafetySignal, bool) {
	for _, signal := range signals {
		if signal.Category == category {
			return signal, true
		}
	}
	return SafetySignal{}, false
}

func hasSafetySeverityForTest(signals []SafetySignal, severity SafetySeverity) bool {
	for _, signal := range signals {
		if signal.Severity == severity {
			return true
		}
	}
	return false
}
