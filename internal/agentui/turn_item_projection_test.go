package agentui

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
)

func TestProjectTurnItemsToAgentEventsIsStable(t *testing.T) {
	createdAt := time.Date(2026, 4, 28, 9, 0, 0, 0, time.UTC)
	items := []agentstate.TurnItem{
		{
			ID:        "model-1",
			Type:      agentstate.TurnItemTypeModelCall,
			Status:    agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Summary: "model response received"},
			CreatedAt: createdAt,
		},
		{
			ID:        "final-1",
			Type:      agentstate.TurnItemTypeFinalAnswer,
			Status:    agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Summary: "done"},
			CreatedAt: createdAt,
		},
	}

	first := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 10)
	second := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 10)

	if len(first) != len(second) {
		t.Fatalf("projection length changed: %d vs %d", len(first), len(second))
	}
	for i := range first {
		firstJSON, _ := json.Marshal(first[i])
		secondJSON, _ := json.Marshal(second[i])
		if string(firstJSON) != string(secondJSON) {
			t.Fatalf("projection[%d] changed:\nfirst=%#v\nsecond=%#v", i, first[i], second[i])
		}
		if err := first[i].Validate(); err != nil {
			t.Fatalf("projected event invalid: %v", err)
		}
	}
}

func TestProjectTurnItemsToAgentEventsPreservesModelCallDebugData(t *testing.T) {
	data := json.RawMessage(`{"iteration":0,"promptFingerprint":{"stableHash":"stable-hash","developerHash":"developer-hash"}}`)
	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", []agentstate.TurnItem{{
		ID:      "model-1",
		Type:    agentstate.TurnItemTypeModelCall,
		Status:  agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{Summary: "model response received", Data: data},
	}}, 0)

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Kind != AgentEventSystem {
		t.Fatalf("kind = %q, want system", events[0].Kind)
	}
	var payload struct {
		Title             string            `json:"title"`
		Summary           string            `json:"summary"`
		PromptFingerprint map[string]string `json:"promptFingerprint"`
	}
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload: %v", err)
	}
	if payload.Title != "model_call" || payload.PromptFingerprint["developerHash"] != "developer-hash" {
		t.Fatalf("payload = %#v, want model_call title and prompt fingerprint", payload)
	}
}

func TestProjectTurnItemsToAgentEventsDeduplicatesFinalAnswer(t *testing.T) {
	items := []agentstate.TurnItem{
		{ID: "final-1", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "first"}},
		{ID: "final-2", Type: agentstate.TurnItemTypeFinalAnswer, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "second"}},
	}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	if len(events) != 1 {
		t.Fatalf("events = %d, want one final answer event", len(events))
	}
	if events[0].Kind != AgentEventAssistant {
		t.Fatalf("final event kind = %q, want assistant", events[0].Kind)
	}
}

func TestProjectTurnItemsToAgentEventsKeepsToolStartBeforeCompletion(t *testing.T) {
	items := []agentstate.TurnItem{
		{ID: "tool-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "read_file"}},
		{ID: "tool-result-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "ok"}},
	}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	if len(events) != 2 {
		t.Fatalf("events = %d, want two tool events", len(events))
	}
	if events[0].Kind != AgentEventTool || events[0].Phase != AgentEventPhaseStarted {
		t.Fatalf("first event = %#v, want tool started", events[0])
	}
	if events[1].Kind != AgentEventTool || events[1].Phase != AgentEventPhaseCompleted {
		t.Fatalf("second event = %#v, want tool completed", events[1])
	}
}

func TestProjectTurnItemsToAgentEventsProjectsPlanSteps(t *testing.T) {
	planData := json.RawMessage(`{
		"status":"active",
		"steps":[
			{"id":"inspect","text":"Inspect host symptoms","status":"in_progress","summary":"checking metrics"},
			{"id":"summarize","text":"Summarize findings","status":"pending"}
		]
	}`)
	items := []agentstate.TurnItem{{
		ID:     "plan-1",
		Type:   agentstate.TurnItemTypePlan,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Summary: "plan updated: active (1/2 in_progress)",
			Data:    planData,
		},
	}}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	if events[0].Kind != AgentEventPlan {
		t.Fatalf("kind = %q, want plan", events[0].Kind)
	}
	var payload PlanPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload decode failed: %v", err)
	}
	if payload.Title != "plan updated: active (1/2 in_progress)" {
		t.Fatalf("title = %q", payload.Title)
	}
	if len(payload.Steps) != 2 {
		t.Fatalf("steps = %#v, want two structured steps", payload.Steps)
	}
	if payload.Steps[0].ID != "inspect" || payload.Steps[0].Text != "Inspect host symptoms" || payload.Steps[0].Status != "in_progress" || payload.Steps[0].Summary != "checking metrics" {
		t.Fatalf("first step = %#v", payload.Steps[0])
	}
}

func TestProjectTurnItemsToAgentEventsProjectsToolPayloadFields(t *testing.T) {
	data := json.RawMessage(`{
		"id":"search-1",
		"name":"web_search",
		"toolName":"web_search",
		"displayKind":"browser.search",
		"displayName":"Web search",
		"title":"Search web",
		"inputSummary":"BTC price",
		"outputSummary":"Found 2 results",
		"arguments":{"query":"BTC price"},
		"outputPreview":[{"title":"BTC Price","url":"https://example.test/btc"}],
		"exitCode":0,
		"durationMs":1234
	}`)
	items := []agentstate.TurnItem{{
		ID:     "tool-call-1",
		Type:   agentstate.TurnItemTypeToolCall,
		Status: agentstate.ItemStatusRunning,
		Payload: agentstate.PayloadEnvelope{
			Kind:    "browser.search",
			Summary: "fallback summary",
			Data:    data,
		},
	}}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	var payload ToolPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload decode failed: %v", err)
	}
	if payload.ToolCallID != "search-1" || payload.ToolName != "web_search" || payload.DisplayKind != "browser.search" {
		t.Fatalf("tool identity = %#v", payload)
	}
	if payload.InputSummary != "BTC price" || payload.OutputSummary != "Found 2 results" {
		t.Fatalf("tool summaries = %#v", payload)
	}
	if string(payload.InputPreview) != `{"query":"BTC price"}` {
		t.Fatalf("input preview = %s", payload.InputPreview)
	}
	if payload.ExitCode == nil || *payload.ExitCode != 0 || payload.DurationMs != 1234 {
		t.Fatalf("execution metadata = %#v", payload)
	}
}

func TestProjectTurnItemsToAgentEventsProjectsApprovalFields(t *testing.T) {
	data := json.RawMessage(`{
		"approvalId":"approval-1",
		"approvalType":"command",
		"title":"Rollback payment-api",
		"command":"kubectl rollout undo deployment/payment-api -n prod",
		"reason":"5xx rose after deploy",
		"risk":"high",
		"targets":["prod/payment-api"]
	}`)
	items := []agentstate.TurnItem{{
		ID:     "approval-1",
		Type:   agentstate.TurnItemTypeApproval,
		Status: agentstate.ItemStatusBlocked,
		Payload: agentstate.PayloadEnvelope{
			Summary: "Rollback payment-api",
			Data:    data,
		},
	}}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	var payload ApprovalPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload decode failed: %v", err)
	}
	if payload.ApprovalID != "approval-1" || payload.ApprovalType != "command" {
		t.Fatalf("approval identity = %#v", payload)
	}
	if payload.Command != "kubectl rollout undo deployment/payment-api -n prod" || payload.Reason != "5xx rose after deploy" || payload.Risk != "high" {
		t.Fatalf("approval fields = %#v", payload)
	}
	if len(payload.Targets) != 1 || payload.Targets[0] != "prod/payment-api" {
		t.Fatalf("approval targets = %#v", payload.Targets)
	}
}

func TestProjectTurnItemsToAgentEventsProjectsEvidenceFields(t *testing.T) {
	data := json.RawMessage(`{
		"id":"metric-1",
		"kind":"metric",
		"title":"5xx rate",
		"summary":"payment-api 5xx rate increased",
		"source":"prometheus",
		"confidence":"high",
		"window":"15m",
		"rawRef":"promql:sum(rate(http_requests_total{status=~\"5..\"}[5m]))",
		"data":{"series":"payment-api"}
	}`)
	items := []agentstate.TurnItem{{
		ID:     "evidence-1",
		Type:   agentstate.TurnItemTypeEvidence,
		Status: agentstate.ItemStatusCompleted,
		Payload: agentstate.PayloadEnvelope{
			Summary: "payment-api 5xx rate increased",
			Data:    data,
		},
	}}

	events := ProjectTurnItemsToAgentEvents("session-1", "turn-1", items, 0)

	var payload EvidencePayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload decode failed: %v", err)
	}
	if payload.ID != "metric-1" || payload.Kind != "metric" || payload.Source != "prometheus" {
		t.Fatalf("evidence identity = %#v", payload)
	}
	if payload.Summary != "payment-api 5xx rate increased" || payload.Confidence != "high" || payload.Window != "15m" {
		t.Fatalf("evidence fields = %#v", payload)
	}
	if payload.RawRef == "" || len(payload.Data) == 0 {
		t.Fatalf("evidence trace data missing: %#v", payload)
	}
}
