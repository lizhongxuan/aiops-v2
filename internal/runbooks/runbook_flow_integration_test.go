package runbooks

import (
	"path/filepath"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
	opsgraph "aiops-v2/internal/opsgraph"
)

func TestERPSRERunbookFlowFromOpsGraphMatchToObservedResult(t *testing.T) {
	graphStore, err := opsgraph.LoadSeedFile(filepath.Join("..", "..", "data", "opsgraph", "erp.seed.yaml"))
	if err != nil {
		t.Fatalf("LoadSeedFile() error = %v", err)
	}
	matches := graphStore.Lookup(opsgraph.LookupRequest{Query: "订单提交", Limit: 1})
	if len(matches) == 0 {
		t.Fatal("opsgraph lookup returned no ERP entity")
	}

	catalog, err := LoadCatalog(filepath.Join("..", "..", "runbooks", "erp", "*.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	now := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	service := NewService(catalog, actionproposal.NewSigner([]byte("runbook-flow-secret"), func() time.Time { return now }), NewInMemoryInstanceStore(), func() time.Time { return now })

	candidates := service.Match(MatchRequest{Capability: "订单提交", Service: "order-api", Environment: "prod", Limit: 1})
	if len(candidates) == 0 {
		t.Fatal("runbook.match returned no candidate")
	}
	instance, err := service.Start(StartRequest{
		RunbookID:  candidates[0].Runbook.ID,
		IncidentID: "inc-erp-1",
		Context: map[string]any{
			"service":   "order-api",
			"timeRange": "5m",
		},
		Evidence: map[string]any{
			"db_connection_pressure": true,
			"report_job_active":      true,
		},
	})
	if err != nil {
		t.Fatalf("runbook.start error = %v", err)
	}
	proposal, ok, err := service.NextAction(NextActionRequest{InstanceID: instance.ID, SessionID: "sess-1", TurnID: "turn-1"})
	if err != nil {
		t.Fatalf("runbook.next_action error = %v", err)
	}
	if !ok || proposal.ToolName == "" || proposal.ActionToken == "" {
		t.Fatalf("next action = %#v, want governed action proposal", proposal)
	}
	if proposal.Source != actionproposal.SourceRunbook || proposal.RunbookID != candidates[0].Runbook.ID {
		t.Fatalf("proposal = %#v, want runbook source/id", proposal)
	}

	if err := service.ObserveResult(ObserveResultRequest{
		InstanceID:    instance.ID,
		StepID:        proposal.RunbookStepID,
		ToolResultRef: "tool-result://coroot/slo-status/1",
		EvidenceRef:   "evidence://coroot/slo-status/1",
	}); err != nil {
		t.Fatalf("runbook.observe_result error = %v", err)
	}
	next, ok, err := service.NextAction(NextActionRequest{InstanceID: instance.ID, SessionID: "sess-1", TurnID: "turn-1"})
	if err != nil {
		t.Fatalf("second next_action error = %v", err)
	}
	if !ok || next.RunbookStepID == proposal.RunbookStepID {
		t.Fatalf("next action after observe = %#v, should advance beyond %q", next, proposal.RunbookStepID)
	}
}
