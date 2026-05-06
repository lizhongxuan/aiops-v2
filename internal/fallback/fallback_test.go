package fallback

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
)

func TestPlanExecCreatesReadOnlyTerminalProposalWhenNoRunbookMatches(t *testing.T) {
	now := time.Date(2026, 5, 3, 13, 0, 0, 0, time.UTC)
	service := NewService(actionproposal.NewSigner([]byte("fallback-secret"), func() time.Time { return now }), NewInMemoryStore(), func() time.Time { return now })
	result, err := service.PlanExec(PlanExecRequest{
		SessionID:    "sess-1",
		TurnID:       "turn-1",
		IncidentID:   "inc-1",
		Goal:         "检查磁盘",
		WhyNoRunbook: "runbook.match returned no high coverage candidate",
		EvidenceRefs: []string{"coroot:incident:1"},
		Actions: []ProposedAction{{
			ToolName:  "exec_" + "command",
			ToolInput: json.RawMessage(`{"command":"df","args":["-h"]}`),
			Reason:    "检查磁盘空间",
		}},
	})
	if err != nil {
		t.Fatalf("PlanExec() error = %v", err)
	}
	if result.Plan.ID == "" || len(result.Plan.Actions) != 1 {
		t.Fatalf("plan result = %#v, want one action", result)
	}
	action := result.Plan.Actions[0]
	if action.ToolName != "exec_"+"command" || action.Risk != actionproposal.RiskLow || action.ApprovalRequired || action.ActionToken == "" {
		t.Fatalf("action = %#v, want read-only terminal proposal with token", action)
	}
	inputHash, err := actionproposal.NormalizedInputHash(action.ToolInput)
	if err != nil {
		t.Fatalf("hash input: %v", err)
	}
	if _, err := actionproposal.NewSigner([]byte("fallback-secret"), func() time.Time { return now }).Verify(action.ActionToken, actionproposal.ActionTokenClaims{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		IncidentID: "inc-1",
		ToolName:   action.ToolName,
		InputHash:  inputHash,
		Source:     actionproposal.SourceFallback,
		Risk:       actionproposal.RiskLow,
	}); err != nil {
		t.Fatalf("Verify(action token) error = %v", err)
	}
}

func TestPlanExecRejectsNonReadOnlyWithoutEvidence(t *testing.T) {
	service := NewService(actionproposal.NewSigner([]byte("fallback-secret"), time.Now), NewInMemoryStore(), time.Now)
	_, err := service.PlanExec(PlanExecRequest{
		SessionID:    "sess-1",
		TurnID:       "turn-1",
		IncidentID:   "inc-1",
		Goal:         "restart service",
		WhyNoRunbook: "no runbook",
		Actions: []ProposedAction{{
			ToolName:  "exec_" + "command",
			ToolInput: json.RawMessage(`{"command":"systemctl","args":["restart","erp.service"]}`),
		}},
	})
	if err == nil {
		t.Fatal("PlanExec(non-read-only without evidence) error = nil")
	}
}

func TestPlanExecRejectsForbiddenCommandEvenWithEvidence(t *testing.T) {
	service := NewService(actionproposal.NewSigner([]byte("fallback-secret"), time.Now), NewInMemoryStore(), time.Now)
	_, err := service.PlanExec(PlanExecRequest{
		SessionID:    "sess-1",
		TurnID:       "turn-1",
		IncidentID:   "inc-1",
		Goal:         "danger",
		WhyNoRunbook: "no runbook",
		EvidenceRefs: []string{"evidence:1"},
		Actions: []ProposedAction{{
			ToolName:  "exec_" + "command",
			ToolInput: json.RawMessage(`{"command":"rm","args":["-rf","/"]}`),
		}},
	})
	if err == nil {
		t.Fatal("PlanExec(forbidden command) error = nil")
	}
}

func TestPlanExecRejectsHighCoverageRunbookCandidate(t *testing.T) {
	service := NewService(actionproposal.NewSigner([]byte("fallback-secret"), time.Now), NewInMemoryStore(), time.Now)
	_, err := service.PlanExec(PlanExecRequest{
		SessionID:    "sess-1",
		TurnID:       "turn-1",
		IncidentID:   "inc-1",
		Goal:         "fallback",
		WhyNoRunbook: "ignored",
		EvidenceRefs: []string{"evidence:1"},
		RunbookMatches: []RunbookMatchSummary{{
			RunbookID: "order-submit-slow",
			Score:     90,
			Coverage:  "high",
		}},
		Actions: []ProposedAction{{
			ToolName:  "exec_" + "command",
			ToolInput: json.RawMessage(`{"command":"df","args":["-h"]}`),
		}},
	})
	if err == nil {
		t.Fatal("PlanExec(high coverage runbook candidate) error = nil")
	}
}
