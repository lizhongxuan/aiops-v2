package runtimekernel

import (
	"testing"

	"aiops-v2/internal/promptinput"
)

func TestAgentFinalGateBlocksPendingWorkerClaim(t *testing.T) {
	decision := EvaluateRuntimeAgentFinalGate(
		"synthetic-worker-1 confirmed bounded summary",
		[]promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "running",
			Summary: "bounded summary",
		}},
	)
	if decision.Action != "require_wait" {
		t.Fatalf("action = %q, want require_wait: %#v", decision.Action, decision)
	}
}

func TestAgentFinalGateAllowsPendingStatusDisclosure(t *testing.T) {
	decision := EvaluateRuntimeAgentFinalGate(
		"synthetic-worker-1 is still running and not confirmed",
		[]promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "running",
		}},
	)
	if decision.Action != "allow" {
		t.Fatalf("action = %q, want allow: %#v", decision.Action, decision)
	}
}
