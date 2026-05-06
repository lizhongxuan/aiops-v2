package fallback

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
)

func TestERPSREFallbackPlanExecCreatesReadOnlyExecProposalWhenRunbookMatchIsEmpty(t *testing.T) {
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	service := NewService(actionproposal.NewSigner([]byte("fallback-flow-secret"), func() time.Time { return now }), NewInMemoryStore(), func() time.Time { return now })
	result, err := service.PlanExec(PlanExecRequest{
		SessionID:      "sess-1",
		TurnID:         "turn-1",
		IncidentID:     "inc-erp-1",
		Goal:           "collect read-only host evidence",
		WhyNoRunbook:   "runbook.match returned no high coverage candidate",
		RunbookMatches: nil,
		Actions: []ProposedAction{{
			ToolName:       "exec_command",
			ToolInput:      json.RawMessage(`{"command":"df","args":["-h"],"intent":"collect disk usage evidence"}`),
			Reason:         "检查磁盘空间",
			ExpectedEffect: "确认是否存在磁盘压力",
		}},
	})
	if err != nil {
		t.Fatalf("PlanExec() error = %v", err)
	}
	if len(result.Plan.Actions) != 1 {
		t.Fatalf("actions = %d, want 1", len(result.Plan.Actions))
	}
	action := result.Plan.Actions[0]
	if action.ToolName != "exec_command" || action.Risk != actionproposal.RiskLow || action.ApprovalRequired {
		t.Fatalf("fallback action = %#v, want read-only exec without approval", action)
	}
	if action.Source != actionproposal.SourceFallback || action.ActionToken == "" {
		t.Fatalf("fallback action missing governed token/source: %#v", action)
	}
}
