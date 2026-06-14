package runbooks

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
)

func testSigner(now time.Time) *actionproposal.Signer {
	return actionproposal.NewSigner([]byte("runbook-test-secret"), func() time.Time { return now })
}

func TestLoadCatalogMatchStartNextActionSignsProposal(t *testing.T) {
	now := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	catalog, err := LoadCatalog(filepath.Join("..", "..", "runbooks", "erp", "*.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	matches := catalog.Match(MatchRequest{
		Symptom:     "订单提交慢",
		Capability:  "capability.order.submit",
		Service:     "order-api",
		Environment: "prod",
	})
	if len(matches) == 0 {
		t.Fatal("Match() returned no candidates")
	}

	service := NewService(catalog, testSigner(now), NewInMemoryInstanceStore(), func() time.Time { return now })
	instance, err := service.Start(StartRequest{
		RunbookID:  matches[0].Runbook.ID,
		IncidentID: "inc-1",
		Context: map[string]any{
			"service":   "order-api",
			"timeRange": "15m",
		},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}

	proposal, ok, err := service.NextAction(NextActionRequest{
		InstanceID: instance.ID,
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		TenantID:   "tenant-a",
		UserID:     "user-a",
	})
	if err != nil {
		t.Fatalf("NextAction() error = %v", err)
	}
	if !ok {
		t.Fatal("NextAction() ok = false, want proposal")
	}
	if proposal.ToolName != "coroot.slo_status" {
		t.Fatalf("proposal.ToolName = %q, want coroot.slo_status", proposal.ToolName)
	}
	if proposal.Source != actionproposal.SourceRunbook || proposal.ActionToken == "" || proposal.ExpiresAt.IsZero() {
		t.Fatalf("proposal missing source/token/expiresAt: %#v", proposal)
	}
	if proposal.TenantID != "tenant-a" || proposal.UserID != "user-a" {
		t.Fatalf("proposal tenant/user = %q/%q, want tenant-a/user-a", proposal.TenantID, proposal.UserID)
	}
	var input map[string]any
	if err := json.Unmarshal(proposal.ToolInput, &input); err != nil {
		t.Fatalf("decode proposal input: %v", err)
	}
	if input["service"] != "order-api" || input["timeRange"] != "15m" {
		t.Fatalf("proposal input = %#v, want rendered context vars", input)
	}
	inputHash, err := actionproposal.NormalizedInputHash(proposal.ToolInput)
	if err != nil {
		t.Fatalf("hash proposal input: %v", err)
	}
	_, err = testSigner(now).Verify(proposal.ActionToken, actionproposal.ActionTokenClaims{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		TenantID:   "tenant-a",
		UserID:     "user-a",
		IncidentID: "inc-1",
		ToolName:   proposal.ToolName,
		InputHash:  inputHash,
		Source:     actionproposal.SourceRunbook,
		Risk:       proposal.Risk,
	})
	if err != nil {
		t.Fatalf("Verify(action token) error = %v", err)
	}
}

func TestRequiredStepCannotBeSkippedUnlessConditionFalse(t *testing.T) {
	catalog, err := LoadCatalogBytes([]byte(`
id: guarded
name: Guarded runbook
scope:
  capabilities: ["capability.order.submit"]
risk: medium
steps:
  - id: gated
    title: gated required step
    tool: coroot.slo_status
    required: true
    condition: "evidence.should_run == true"
    input:
      service: "{{ service }}"
  - id: second
    title: second required step
    tool: opsgraph.business_impact
    required: true
    input:
      entityId: "{{ entity }}"
`))
	if err != nil {
		t.Fatalf("LoadCatalogBytes() error = %v", err)
	}
	service := NewService(catalog, testSigner(time.Now()), NewInMemoryInstanceStore(), time.Now)
	instance, err := service.Start(StartRequest{
		RunbookID: "guarded",
		Context:   map[string]any{"service": "order-api", "entity": "service.order-api"},
		Evidence:  map[string]any{"should_run": true},
	})
	if err != nil {
		t.Fatalf("Start() error = %v", err)
	}
	if err := service.ObserveResult(ObserveResultRequest{InstanceID: instance.ID, StepID: "second", ToolResultRef: "tool:second"}); err == nil {
		t.Fatal("ObserveResult(second before gated) error = nil, want required step protection")
	}

	instance, err = service.Start(StartRequest{
		RunbookID: "guarded",
		Context:   map[string]any{"service": "order-api", "entity": "service.order-api"},
		Evidence:  map[string]any{"should_run": false},
	})
	if err != nil {
		t.Fatalf("Start(false condition) error = %v", err)
	}
	proposal, ok, err := service.NextAction(NextActionRequest{InstanceID: instance.ID, SessionID: "sess", TurnID: "turn"})
	if err != nil {
		t.Fatalf("NextAction(false condition) error = %v", err)
	}
	if !ok || proposal.ToolName != "opsgraph.business_impact" {
		t.Fatalf("NextAction(false condition) = %#v ok=%v, want second step", proposal, ok)
	}
}

func TestTemplateAndConditionAreRestricted(t *testing.T) {
	if _, err := RenderTemplateString("{{ shell.exec }}", map[string]any{"service": "order-api"}); err == nil {
		t.Fatal("RenderTemplateString(shell.exec) error = nil")
	}
	if _, err := EvalCondition("evidence.latency_ms > 100 && evidence.status == 'critical'", map[string]any{"latency_ms": 120, "status": "critical"}); err != nil {
		t.Fatalf("EvalCondition(simple compare) error = %v", err)
	}
	if _, err := EvalCondition("exec('rm -rf /')", map[string]any{}); err == nil {
		t.Fatal("EvalCondition(script) error = nil")
	}
}
