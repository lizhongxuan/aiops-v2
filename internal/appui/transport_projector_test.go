package appui

import (
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/specialinputmemory"
)

func TestTransportProjectorPreservesCanonicalTurnFactsLosslessly(t *testing.T) {
	now := time.Date(2026, 7, 12, 9, 30, 0, 0, time.UTC)
	items := []agentstate.TurnItem{
		{
			ID:     "model-call-1",
			Type:   agentstate.TurnItemTypeModelCall,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "model_call",
				Summary: "sampled model",
				Data:    json.RawMessage(`{"promptFingerprint":{"stableHash":"sha256:stable","developerHash":"sha256:developer"},"visibleTools":["inspect_service","restart_service"]}`),
			},
			CreatedAt: now,
			UpdatedAt: now.Add(time.Second),
		},
		{
			ID:     "tool-call-1",
			Type:   agentstate.TurnItemTypeToolCall,
			Status: agentstate.ItemStatusRunning,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "tool_call",
				Summary: "inspect service",
				Data:    json.RawMessage(`{"toolCallId":"call-1","toolName":"inspect_service","arguments":{"target":{"hostId":"host-a"},"checks":["health","latency"]}}`),
			},
			CreatedAt: now.Add(2 * time.Second),
			UpdatedAt: now.Add(3 * time.Second),
		},
		{
			ID:     "approval-1",
			Type:   agentstate.TurnItemTypeApprovalRequested,
			Status: agentstate.ItemStatusBlocked,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "approval",
				Summary: "approval required",
				Data:    json.RawMessage(`{"approvalId":"approval-1","toolCallId":"call-1","risk":{"level":"high","reasons":["mutation"]},"rollback":{"tool":"restore_service","arguments":{"hostId":"host-a"}}}`),
			},
			CreatedAt: now.Add(4 * time.Second),
			UpdatedAt: now.Add(5 * time.Second),
		},
		{
			ID:     "evidence-1",
			Type:   agentstate.TurnItemTypeEvidenceCollected,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "evidence",
				Summary: "service health evidence",
				Data:    json.RawMessage(`{"evidenceId":"evidence-1","refs":[{"id":"metric-1","kind":"metric","labels":{"service":"api"}}],"facts":{"healthy":true}}`),
			},
			CreatedAt: now.Add(6 * time.Second),
			UpdatedAt: now.Add(7 * time.Second),
		},
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:              "turn-lossless-facts",
		ClientTurnID:    "client-turn-1",
		ClientMessageID: "client-message-1",
		SessionID:       "session-lossless-facts",
		Lifecycle:       runtimekernel.TurnLifecycleRunning,
		StartedAt:       now,
		UpdatedAt:       now.Add(8 * time.Second),
		AgentItems:      items,
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(
		NewAiopsTransportState(turn.SessionID, "thread-lossless-facts"),
		turn,
	)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	if got.ClientTurnID != turn.ClientTurnID || got.ClientMessageID != turn.ClientMessageID {
		t.Fatalf("client ids = %q/%q, want %q/%q", got.ClientTurnID, got.ClientMessageID, turn.ClientTurnID, turn.ClientMessageID)
	}
	assertTransportAgentItemsMatchCanonical(t, got.AgentItems, items)
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("json.Marshal(projected) error = %v", err)
	}
	var roundTripped AiopsTransportState
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal(projected) error = %v", err)
	}
	if got := roundTripped.Turns[turn.ID]; got.ClientTurnID != turn.ClientTurnID || got.ClientMessageID != turn.ClientMessageID || !reflect.DeepEqual(got.AgentItems, projected.Turns[turn.ID].AgentItems) {
		t.Fatalf("transport JSON round trip lost canonical facts: %#v", got)
	}
}

func TestTransportProjectorClearsStaleRunningFinalWhenCanonicalSnapshotOnlyContainsCommentary(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 30, 0, 0, time.UTC)
	turnID := "turn-stale-running-final"
	state := NewAiopsTransportState("session-stale-running-final", "thread-stale-running-final")
	state.TurnOrder = []string{turnID}
	state.Turns[turnID] = AiopsTransportTurn{
		ID:     turnID,
		Status: AiopsTransportTurnStatusWorking,
		Final: &AiopsTransportFinal{
			ID:     "stale-final",
			Status: AiopsTransportFinalStatusRunning,
			Text:   "让我先查看是否有 Coroot 相关的上下文数据。",
		},
	}
	commentaryData, err := json.Marshal(map[string]any{
		"phase":       "commentary",
		"streamState": "complete",
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:        turnID,
		SessionID: state.SessionID,
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{{
			ID:     "commentary-1",
			Type:   agentstate.TurnItemTypeAssistantMessage,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "assistant_message",
				Summary: "让我先查看是否有 Coroot 相关的上下文数据。",
				Data:    commentaryData,
			},
			CreatedAt: now,
			UpdatedAt: now.Add(time.Second),
		}},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turnID]
	if got.Final != nil {
		t.Fatalf("Final = %#v, want stale running final cleared", got.Final)
	}
	if len(got.BlockOrder) != 1 {
		t.Fatalf("block order = %#v, want one canonical commentary block", got.BlockOrder)
	}
	block := got.BlocksByID[got.BlockOrder[0]]
	if block.Type != AiopsTransportBlockTypeCommentary || block.Text != "让我先查看是否有 Coroot 相关的上下文数据。" {
		t.Fatalf("canonical block = %#v, want only commentary", block)
	}
}

func assertTransportAgentItemsMatchCanonical(t *testing.T, got []AiopsTransportAgentItem, want []agentstate.TurnItem) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("AgentItems = %d, want %d", len(got), len(want))
	}
	for i := range want {
		if got[i].SchemaVersion != AiopsTransportAgentItemSchemaVersion || got[i].ID != want[i].ID || got[i].Type != string(want[i].Type) || got[i].Status != string(want[i].Status) || got[i].Payload.Kind != want[i].Payload.Kind || got[i].Payload.Summary != want[i].Payload.Summary || got[i].CreatedAt != transportTimestamp(want[i].CreatedAt) || got[i].UpdatedAt != transportTimestamp(want[i].UpdatedAt) {
			t.Fatalf("AgentItems[%d] = %#v, want canonical metadata %#v", i, got[i], want[i])
		}
		var gotData, wantData any
		if json.Unmarshal(got[i].Payload.Data, &gotData) != nil || json.Unmarshal(want[i].Payload.Data, &wantData) != nil || !reflect.DeepEqual(gotData, wantData) {
			t.Fatalf("AgentItems[%d].Payload.Data = %s, want %s", i, got[i].Payload.Data, want[i].Payload.Data)
		}
	}
}

func TestTransportProjectorCanonicalTurnFactsAreMutationIsolated(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:              "turn-isolated-facts",
		ClientTurnID:    "client-turn-isolated",
		ClientMessageID: "client-message-isolated",
		SessionID:       "session-isolated-facts",
		Lifecycle:       runtimekernel.TurnLifecycleRunning,
		StartedAt:       now,
		UpdatedAt:       now,
		AgentItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: json.RawMessage(`{"promptFingerprint":{"stableHash":"sha256:original"}}`)}},
			{ID: "tool-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Data: json.RawMessage(`{"arguments":{"hosts":["host-a","host-b"]}}`)}},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(
		NewAiopsTransportState(turn.SessionID, "thread-isolated-facts"),
		turn,
	)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns[turn.ID]

	turn.AgentItems[0].ID = "source-mutated"
	turn.AgentItems[0].Payload.Data[0] = '['
	if transportTurn.AgentItems[0].ID != "model-1" || string(transportTurn.AgentItems[0].Payload.Data) != `{"promptFingerprint":{"stableHash":"sha256:original"}}` {
		t.Fatalf("transport AgentItems changed after source mutation: %#v", transportTurn.AgentItems[0])
	}

	transportTurn.AgentItems[1].ID = "projection-mutated"
	transportTurn.AgentItems[1].Payload.Data[0] = '['
	if turn.AgentItems[1].ID != "tool-1" || string(turn.AgentItems[1].Payload.Data) != `{"arguments":{"hosts":["host-a","host-b"]}}` {
		t.Fatalf("source AgentItems changed after projection mutation: %#v", turn.AgentItems[1])
	}
}

func TestTransportProjectorCanonicalTurnFactsRedactAndBoundPayload(t *testing.T) {
	now := time.Date(2026, 7, 12, 10, 30, 0, 0, time.UTC)
	secret := "story-super-secret-value"
	large := strings.Repeat("payload-", transportAgentItemPayloadByteBudget)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-private-facts",
		SessionID: "session-private-facts",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now,
		AgentItems: []agentstate.TurnItem{{
			ID:     "tool-private",
			Type:   agentstate.TurnItemTypeToolCall,
			Status: agentstate.ItemStatusRunning,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "tool_call",
				Summary: "Authorization: Bearer " + secret + " password=" + secret,
				Data:    json.RawMessage(`{"toolCallId":"call-private","toolName":"inspect_service","arguments":{"authorization":"Bearer ` + secret + `","password":"` + secret + `","note":"token=` + secret + `"},"large":"` + large + `"}`),
			},
		}},
	}

	project := func() AiopsTransportTurn {
		projected, err := NewTransportProjector().ProjectTurnSnapshot(
			NewAiopsTransportState(turn.SessionID, "thread-private-facts"),
			turn,
		)
		if err != nil {
			t.Fatalf("ProjectTurnSnapshot() error = %v", err)
		}
		return projected.Turns[turn.ID]
	}
	first := project()
	second := project()
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("privacy/budget projection is not deterministic\nfirst=%#v\nsecond=%#v", first, second)
	}
	if len(first.AgentItems) != 1 {
		t.Fatalf("AgentItems = %d, want 1", len(first.AgentItems))
	}
	item := first.AgentItems[0]
	if item.SchemaVersion != AiopsTransportAgentItemSchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", item.SchemaVersion, AiopsTransportAgentItemSchemaVersion)
	}
	if !item.Truncated || item.OriginalBytes <= transportAgentItemPayloadByteBudget || item.ContentHash == "" || item.Ref == "" {
		t.Fatalf("bounded item facts missing: %#v", item)
	}
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("json.Marshal(turn) error = %v", err)
	}
	text := string(encoded)
	for _, forbidden := range []string{secret, "Bearer " + secret, "password=" + secret, "token=" + secret} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("transport leaked %q: %s", forbidden, text)
		}
	}
	for _, required := range []string{"call-private", "inspect_service", "arguments", "[REDACTED]"} {
		if !strings.Contains(text, required) {
			t.Fatalf("transport lost required redacted fact %q: %s", required, text)
		}
	}
	var roundTripped AiopsTransportTurn
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal(turn) error = %v", err)
	}
	wantWire := first
	wantWire.Process = nil
	wantWire.Final = nil
	if !reflect.DeepEqual(roundTripped, wantWire) {
		t.Fatalf("transport JSON round trip changed bounded facts\nwant=%#v\n got=%#v", wantWire, roundTripped)
	}
}

func TestTransportProjectorRejectsInvalidOrTrailingAgentPayloadWithoutHashingSecrets(t *testing.T) {
	project := func(secret string) AiopsTransportTurn {
		turn := &runtimekernel.TurnSnapshot{
			ID: "turn-invalid-private", SessionID: "session-invalid-private", Lifecycle: runtimekernel.TurnLifecycleRunning,
			AgentItems: []agentstate.TurnItem{
				{ID: "invalid", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: json.RawMessage(`password="` + secret + `" Authorization:"Bearer ` + secret + `"`)}},
				{ID: "trailing", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: json.RawMessage(`{"toolCallId":"call-trailing"} trailing password="` + secret + `"`)}},
			},
		}
		projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-invalid-private"), turn)
		if err != nil {
			t.Fatalf("ProjectTurnSnapshot() error = %v", err)
		}
		return projected.Turns[turn.ID]
	}
	first := project("secret value")
	second := project("public value")
	encoded, err := json.Marshal(first)
	if err != nil {
		t.Fatalf("json.Marshal(turn) error = %v", err)
	}
	for _, forbidden := range []string{`secret value`, `Bearer secret value`, `password=`, `trailing password`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("invalid payload leaked %q: %s", forbidden, encoded)
		}
	}
	for i, item := range first.AgentItems {
		if !item.Truncated || item.ContentHash == "" || !strings.Contains(string(item.Payload.Data), "_transportInvalidPayload") {
			t.Fatalf("AgentItems[%d] invalid payload facts = %#v", i, item)
		}
		if item.ContentHash != second.AgentItems[i].ContentHash {
			t.Fatalf("AgentItems[%d] hash depends on secret: %q != %q", i, item.ContentHash, second.AgentItems[i].ContentHash)
		}
	}
}

func TestTransportProjectorRedactsIterationSearchResultFallback(t *testing.T) {
	secret := "secret value"
	turn := &runtimekernel.TurnSnapshot{
		ID: "turn-private-search", SessionID: "session-private-search", Lifecycle: runtimekernel.TurnLifecycleCompleted,
		Iterations: []runtimekernel.IterationState{{
			ID: "iteration-private-search", ToolResults: []runtimekernel.ToolResult{{
				ToolCallID: "call-private-search",
				Content:    `{"results":[{"title":"password=\"` + secret + `\"","url":"https://example.com","snippet":"Authorization:\"Bearer ` + secret + `\"","text":"token=` + secret + `"}]}`,
			}},
		}},
		AgentItems: []agentstate.TurnItem{{
			ID: "search-private", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Data: json.RawMessage(`{"toolCallId":"call-private-search","toolName":"web_search","displayKind":"browser.search","inputSummary":"safe query"}`)},
		}},
	}
	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-private-search"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(turn) error = %v", err)
	}
	if strings.Contains(string(encoded), secret) {
		t.Fatalf("iteration fallback leaked secret: %s", encoded)
	}
	block := findTransportProcessBlock(t, got.Process, AiopsTransportProcessKindSearch)
	if len(block.Results) != 1 || !strings.Contains(block.Results[0].Title+block.Results[0].Snippet+block.Results[0].Text, "[REDACTED]") {
		t.Fatalf("search results = %#v, want structured redaction", block.Results)
	}
}

func TestTransportProjectorRejectsTrailingJSONInIterationFallback(t *testing.T) {
	secret := "secret value"
	turn := &runtimekernel.TurnSnapshot{Iterations: []runtimekernel.IterationState{{
		ToolResults: []runtimekernel.ToolResult{{
			ToolCallID: "call-invalid-fallback",
			Content:    `{"password":"` + secret + `"} trailing`,
		}},
	}}}
	previews, payloads := transportToolResultFacts(turn)
	encoded, err := json.Marshal(map[string]any{"previews": previews, "payloads": payloads})
	if err != nil {
		t.Fatalf("json.Marshal(fallback facts) error = %v", err)
	}
	if strings.Contains(string(encoded), secret) || !strings.Contains(string(encoded), "_transportInvalidPayload") {
		t.Fatalf("invalid iteration fallback was not fail-closed: %s", encoded)
	}
}

func TestTransportProjectorHardCapsHugeAgentItemAndTurnJSON(t *testing.T) {
	huge := strings.Repeat("huge-field-", transportAgentItemsTurnByteBudget)
	turn := &runtimekernel.TurnSnapshot{
		ID: "turn-huge-agent-item", SessionID: "session-huge-agent-item", Lifecycle: runtimekernel.TurnLifecycleRunning,
		AgentItems: []agentstate.TurnItem{{
			ID: huge, Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{Kind: huge, Summary: huge, Data: json.RawMessage(`{"toolCallId":` + strconv.Quote(huge) + `,"outputSummary":` + strconv.Quote(huge) + `}`)},
		}},
	}
	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-huge-agent-item"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	itemJSON, err := json.Marshal(got.AgentItems[0])
	if err != nil {
		t.Fatalf("json.Marshal(item) error = %v", err)
	}
	if len(itemJSON) > 32*1024 {
		t.Fatalf("agent item bytes = %d, hard cap = %d", len(itemJSON), 32*1024)
	}
	turnJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(turn) error = %v", err)
	}
	if len(turnJSON) > transportAgentItemsTurnByteBudget {
		t.Fatalf("turn bytes = %d, hard cap = %d", len(turnJSON), transportAgentItemsTurnByteBudget)
	}
	if !got.AgentItems[0].Truncated || got.AgentItems[0].ContentHash == "" || got.AgentItems[0].Ref == "" {
		t.Fatalf("huge item truncation facts missing: %#v", got.AgentItems[0])
	}
}

func TestTransportProjectorCanonicalTurnFactsEnforceDeterministicTurnBudget(t *testing.T) {
	now := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	items := make([]agentstate.TurnItem, 0, transportAgentItemsPerTurnBudget+17)
	for i := 0; i < transportAgentItemsPerTurnBudget+17; i++ {
		items = append(items, agentstate.TurnItem{
			ID:     "item-" + strconv.Itoa(i),
			Type:   agentstate.TurnItemTypeEvidence,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Kind:    "evidence",
				Summary: "evidence " + strconv.Itoa(i),
				Data:    json.RawMessage(`{"evidenceId":"evidence-` + strconv.Itoa(i) + `","facts":{"healthy":true}}`),
			},
		})
	}
	turn := &runtimekernel.TurnSnapshot{ID: "turn-budget", SessionID: "session-budget", Lifecycle: runtimekernel.TurnLifecycleRunning, StartedAt: now, UpdatedAt: now, AgentItems: items}
	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-budget"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	if !got.AgentItemsTruncated || got.AgentItemsOriginalCount != len(items) || got.AgentItemsOriginalBytes <= 0 || got.AgentItemsHash == "" || got.AgentItemsRef == "" {
		t.Fatalf("turn truncation facts missing: %#v", got)
	}
	if len(got.AgentItems) > transportAgentItemsPerTurnBudget {
		t.Fatalf("AgentItems = %d, budget = %d", len(got.AgentItems), transportAgentItemsPerTurnBudget)
	}
	encoded, err := json.Marshal(got.AgentItems)
	if err != nil {
		t.Fatalf("json.Marshal(AgentItems) error = %v", err)
	}
	if len(encoded) > transportAgentItemsTurnByteBudget {
		t.Fatalf("AgentItems bytes = %d, budget = %d", len(encoded), transportAgentItemsTurnByteBudget)
	}
	if got.AgentItems[0].ID != "item-0" || got.AgentItems[len(got.AgentItems)-1].ID != "item-"+strconv.Itoa(len(items)-1) {
		t.Fatalf("turn budget must deterministically preserve head/tail facts: first=%q last=%q", got.AgentItems[0].ID, got.AgentItems[len(got.AgentItems)-1].ID)
	}
}

func TestTransportProjectorProjectsStructuredTurnItems(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")

	planData := json.RawMessage(`{
		"title":"排查计划",
		"steps":[
			{"id":"inspect","text":"Inspect payment-api logs","status":"completed"},
			{"id":"rollback","text":"Prepare rollback command","status":"running"}
		]
	}`)
	searchData := json.RawMessage(`{
		"toolCallId":"search-1",
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"payment-api 5xx",
		"outputSummary":"found 2 results",
		"outputPreview":{"results":[
			{"title":"Error budget burn","url":"https://example.com/burn","snippet":"budget burn detected"},
			{"title":"Prometheus panel","url":"https://example.com/prom","snippet":"5xx raised"}
		]}
	}`)
	commandData := json.RawMessage(`{
		"toolCallId":"cmd-1",
		"toolName":"exec_command",
		"displayKind":"command",
		"inputSummary":"kubectl rollout undo deployment/payment-api -n prod",
		"outputSummary":"rollout undo started",
		"exitCode":0,
		"durationMs":2300
	}`)
	evidenceData := json.RawMessage(`{
		"id":"metric-1",
		"kind":"metric",
		"title":"5xx rate",
		"summary":"payment-api 5xx increased",
		"source":"prometheus",
		"confidence":"high",
		"window":"15m",
		"rawRef":"promql:5xx"
	}`)
	approvalData := json.RawMessage(`{
		"approvalId":"approval-1",
		"approvalType":"command",
		"command":"kubectl rollout undo deployment/payment-api -n prod",
		"reason":"high risk action"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-1",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now.Add(5 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "排查 payment-api 5xx"}, CreatedAt: now},
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "analyzing rollout telemetry"}, CreatedAt: now.Add(500 * time.Millisecond)},
			{ID: "plan-1", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "排查计划", Data: planData}, CreatedAt: now.Add(time.Second)},
			{ID: "search-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "payment-api 5xx", Data: searchData}, CreatedAt: now.Add(2 * time.Second)},
			{ID: "cmd-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "rollback command", Data: commandData}, CreatedAt: now.Add(3 * time.Second)},
			{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "payment-api 5xx increased", Data: evidenceData}, CreatedAt: now.Add(4 * time.Second)},
			{ID: "approval-1", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "需要审批", Data: approvalData}, CreatedAt: now.Add(5 * time.Second)},
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "等待审批完成后执行回滚",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(6 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.CurrentTurnID != "turn-1" {
		t.Fatalf("CurrentTurnID = %q, want turn-1", projected.CurrentTurnID)
	}
	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("Status = %q, want %q", projected.Status, AiopsTransportStatusBlocked)
	}
	if len(projected.TurnOrder) != 1 || projected.TurnOrder[0] != "turn-1" {
		t.Fatalf("TurnOrder = %#v, want [turn-1]", projected.TurnOrder)
	}

	transportTurn := projected.Turns["turn-1"]
	if transportTurn.User == nil || transportTurn.User.Text != "排查 payment-api 5xx" {
		t.Fatalf("turn.User = %+v, want projected user message", transportTurn.User)
	}
	if transportTurn.Status != AiopsTransportTurnStatusBlocked {
		t.Fatalf("turn.Status = %q, want %q", transportTurn.Status, AiopsTransportTurnStatusBlocked)
	}
	if transportTurn.Final == nil || transportTurn.Final.Text != "等待审批完成后执行回滚" {
		t.Fatalf("turn.Final = %+v, want final text", transportTurn.Final)
	}
	if len(transportTurn.Process) != 6 {
		t.Fatalf("len(turn.Process) = %d, want 6", len(transportTurn.Process))
	}

	reasoningBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindReasoning)
	if reasoningBlock.Text != "analyzing rollout telemetry" {
		t.Fatalf("reasoning block = %+v, want model call summary", reasoningBlock)
	}

	planBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindPlan)
	if len(planBlock.Steps) != 2 || planBlock.Steps[1].Status != "running" {
		t.Fatalf("plan steps = %+v, want preserved plan steps", planBlock.Steps)
	}

	searchBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindSearch)
	if len(searchBlock.Queries) != 1 || searchBlock.Queries[0] != "payment-api 5xx" {
		t.Fatalf("search queries = %#v, want input summary", searchBlock.Queries)
	}
	if len(searchBlock.Results) != 2 || searchBlock.Results[0].Title != "Error budget burn" {
		t.Fatalf("search results = %#v, want decoded results", searchBlock.Results)
	}
	if searchBlock.FoldGroupKind != "web_lookup" || searchBlock.FoldGroupID == "" {
		t.Fatalf("search fold group = %q/%q, want web_lookup metadata", searchBlock.FoldGroupKind, searchBlock.FoldGroupID)
	}

	commandBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "kubectl rollout undo deployment/payment-api -n prod" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if commandBlock.FoldGroupKind != "command" || commandBlock.FoldGroupID == "" {
		t.Fatalf("command fold group = %q/%q, want command metadata", commandBlock.FoldGroupKind, commandBlock.FoldGroupID)
	}
	if commandBlock.Command == "exec_command" {
		t.Fatal("command block should not expose tool name as user-visible command")
	}
	if commandBlock.OutputPreview != "" {
		t.Fatalf("command block output = %q, want no preview without explicit outputPreview", commandBlock.OutputPreview)
	}
	if commandBlock.ExitCode == nil || *commandBlock.ExitCode != 0 {
		t.Fatalf("command exit code = %#v, want 0", commandBlock.ExitCode)
	}
	if commandBlock.DurationMs != 2300 {
		t.Fatalf("command duration = %d, want 2300", commandBlock.DurationMs)
	}

	evidenceBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindEvidence)
	if evidenceBlock.Source != "prometheus" || evidenceBlock.Confidence != "high" || evidenceBlock.Window != "15m" || evidenceBlock.RawRef != "promql:5xx" {
		t.Fatalf("evidence block = %+v, want source/confidence/window/rawRef", evidenceBlock)
	}

	approvalBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindApproval)
	if approvalBlock.ApprovalID != "approval-1" || approvalBlock.Status != AiopsTransportProcessStatusBlocked {
		t.Fatalf("approval block = %+v, want blocked approval", approvalBlock)
	}
	for _, block := range transportTurn.Process {
		if block.Kind == AiopsTransportProcessKindAssistant {
			t.Fatalf("final assistant_message must not duplicate into process: %#v", transportTurn.Process)
		}
	}
	if _, ok := projected.PendingApprovals["approval-1"]; !ok {
		t.Fatalf("PendingApprovals = %#v, want approval-1", projected.PendingApprovals)
	}
}

func TestTransportProjectorEmitsCanonicalOrderedBlocksWithoutLegacyTranscriptFields(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-canonical-blocks",
		SessionID: "session-canonical-blocks",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "commentary-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "先检查运行状态。", Data: json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete"}`)}, CreatedAt: now},
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "检查完成。", Data: json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`)}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState(turn.SessionID, "thread-canonical-blocks"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	if len(got.BlockOrder) != 2 || got.BlockOrder[1] != "final-1" {
		t.Fatalf("BlockOrder = %#v, want commentary then final", got.BlockOrder)
	}
	if got.BlocksByID[got.BlockOrder[0]].Type != AiopsTransportBlockTypeCommentary {
		t.Fatalf("commentary block = %#v", got.BlocksByID[got.BlockOrder[0]])
	}
	if final := got.BlocksByID["final-1"]; final.Type != AiopsTransportBlockTypeFinalAnswer || final.FinalContract == nil || final.Text != "检查完成。" {
		t.Fatalf("final block = %#v", final)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatalf("json.Marshal(turn) error = %v", err)
	}
	var wire map[string]any
	if err := json.Unmarshal(encoded, &wire); err != nil {
		t.Fatalf("json.Unmarshal(turn) error = %v", err)
	}
	if _, ok := wire["process"]; ok {
		t.Fatalf("legacy process leaked into wire: %s", encoded)
	}
	if _, ok := wire["final"]; ok {
		t.Fatalf("legacy final leaked into wire: %s", encoded)
	}
}

func TestTransportProjectorProjectsSpecialInputContext(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 0, 0, 0, time.UTC)
	state := NewAiopsTransportState("session-special-input", "thread-special-input")
	grant := specialinputmemory.ExecutionScopeGrant{
		ID:             "grant-host-a",
		ResourceKind:   specialinputmemory.ResourceKindHost,
		ResourceID:     "host-a",
		CanonicalKey:   "host:host-a",
		Display:        "host-a",
		Status:         specialinputmemory.GrantStatusActive,
		AllowedActions: []string{specialinputmemory.ActionExecLowRisk, specialinputmemory.ActionInspect, specialinputmemory.ActionRead},
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-special-input",
		SessionID:   "session-special-input",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now,
		SpecialInputReadPlan: &specialinputmemory.MemoryReadPlan{
			SchemaVersion:        specialinputmemory.SchemaVersion,
			TurnID:               "turn-special-input",
			ActiveExecutionScope: &grant,
			CandidateFacts: []specialinputmemory.MentionFact{{
				ID:           "fact-raw",
				Kind:         specialinputmemory.FactKindHost,
				ResourceKind: specialinputmemory.ResourceKindHost,
				ResourceID:   "1.1.1.1",
				CanonicalKey: "host:1.1.1.1",
				Display:      "1.1.1.1",
				TrustLevel:   specialinputmemory.TrustLevelRawTyped,
				Status:       specialinputmemory.FactStatusActive,
			}},
			PendingConfirmations: []specialinputmemory.PendingConfirmation{{
				ID:     "pending-target",
				Kind:   "target",
				Reason: "active_grant_revalidate_failed",
			}},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if projected.SpecialInputContext == nil {
		t.Fatal("SpecialInputContext is nil, want projected context")
	}
	if projected.SpecialInputContext.ActiveGrant == nil || projected.SpecialInputContext.ActiveGrant.ResourceID != "host-a" {
		t.Fatalf("ActiveGrant = %#v, want host-a", projected.SpecialInputContext.ActiveGrant)
	}
	if len(projected.SpecialInputContext.CandidateFacts) != 1 || projected.SpecialInputContext.CandidateFacts[0].TrustLevel != specialinputmemory.TrustLevelRawTyped {
		t.Fatalf("CandidateFacts = %#v, want raw typed candidate", projected.SpecialInputContext.CandidateFacts)
	}
	if len(projected.SpecialInputContext.PendingConfirmations) != 1 {
		t.Fatalf("PendingConfirmations = %#v, want one", projected.SpecialInputContext.PendingConfirmations)
	}
}

func TestTransportProjectorKeepsSpecialInputContextWhenLaterTurnHasNoReadPlan(t *testing.T) {
	now := time.Date(2026, 7, 3, 10, 5, 0, 0, time.UTC)
	state := NewAiopsTransportState("session-special-input-carry", "thread-special-input-carry")
	state.SpecialInputContext = &specialinputmemory.TransportContext{
		SchemaVersion: specialinputmemory.SchemaVersion,
		TurnID:        "turn-with-read-plan",
		ActiveGrant: &specialinputmemory.TransportGrant{
			ID:           "grant-host-a",
			ResourceKind: specialinputmemory.ResourceKindHost,
			ResourceID:   "host-a",
			CanonicalKey: "host:host-a",
			Display:      "host-a",
			Status:       specialinputmemory.GrantStatusActive,
		},
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-without-read-plan",
		SessionID:   "session-special-input-carry",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if projected.SpecialInputContext == nil || projected.SpecialInputContext.ActiveGrant == nil {
		t.Fatalf("SpecialInputContext = %#v, want previous active grant preserved", projected.SpecialInputContext)
	}
	if projected.SpecialInputContext.ActiveGrant.ResourceID != "host-a" {
		t.Fatalf("ActiveGrant.ResourceID = %q, want host-a", projected.SpecialInputContext.ActiveGrant.ResourceID)
	}
}

func TestTransportProjectorProjectsAssistantMessageByPhase(t *testing.T) {
	now := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	commentaryData := json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete","iteration":0}`)
	finalData := json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete","iteration":1,"durationMs":1234}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-assistant-message-phase",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTime(now.Add(2 * time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "assistant-commentary", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "我先查公开来源。", Data: commentaryData}, CreatedAt: now},
			{ID: "assistant-final", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "这是最终回答。", Data: finalData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedTurn := projected.Turns[turn.ID]
	if projectedTurn.Final == nil || projectedTurn.Final.Text != "这是最终回答。" || projectedTurn.Final.DurationMs != 1234 {
		t.Fatalf("final=%#v, want assistant_message final", projectedTurn.Final)
	}
	if len(projectedTurn.Process) != 1 {
		t.Fatalf("process=%#v, want commentary only", projectedTurn.Process)
	}
	block := projectedTurn.Process[0]
	if block.Text != "我先查公开来源。" || block.DisplayKind != "assistant.message" || block.Phase != "commentary" {
		t.Fatalf("commentary block=%#v, want assistant_message commentary", block)
	}
}

func TestTransportProjectorAddsFoldGroupOnlyForWebLookupAndCommands(t *testing.T) {
	now := time.Date(2026, 6, 26, 11, 0, 0, 0, time.UTC)
	toolPayload := func(callID, toolName, displayKind, input string) json.RawMessage {
		return json.RawMessage(`{
			"toolCallId":` + strconv.Quote(callID) + `,
			"toolName":` + strconv.Quote(toolName) + `,
			"displayKind":` + strconv.Quote(displayKind) + `,
			"inputSummary":` + strconv.Quote(input) + `,
			"outputSummary":"ok"
		}`)
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-fold-metadata",
		SessionID: "session-fold-metadata",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "browse", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "open docs", Data: toolPayload("browse", "browse_url", "browse_url", "https://example.com/docs")}, CreatedAt: now},
			{ID: "find", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "find text", Data: toolPayload("find", "browser.find", "browser.find", "pattern=timeout")}, CreatedAt: now.Add(100 * time.Millisecond)},
			{ID: "mcp", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "read mcp", Data: toolPayload("mcp", "read_mcp_resource", "read_mcp_resource", "ops://manual")}, CreatedAt: now.Add(200 * time.Millisecond)},
			{ID: "skill", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "skill search", Data: toolPayload("skill", "skill_search", "skill_search", "diagnose")}, CreatedAt: now.Add(300 * time.Millisecond)},
			{ID: "cmd", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "run command", Data: toolPayload("cmd", "exec_command", "command", "uptime")}, CreatedAt: now.Add(400 * time.Millisecond)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-fold-metadata", "thread-fold-metadata"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	byCallID := map[string]AiopsProcessBlock{}
	for _, block := range projected.Turns["turn-fold-metadata"].Process {
		byCallID[block.ToolCallID] = block
	}
	for _, callID := range []string{"browse", "find"} {
		if byCallID[callID].Kind != AiopsTransportProcessKindSearch || byCallID[callID].FoldGroupKind != "web_lookup" || byCallID[callID].FoldGroupID == "" {
			t.Fatalf("block %q = %#v, want web_lookup search fold metadata", callID, byCallID[callID])
		}
	}
	for _, callID := range []string{"mcp", "skill"} {
		if byCallID[callID].FoldGroupKind != "" || byCallID[callID].FoldGroupID != "" {
			t.Fatalf("block %q = %#v, should not have fold metadata", callID, byCallID[callID])
		}
	}
	if byCallID["cmd"].FoldGroupKind != "command" || byCallID["cmd"].FoldGroupID == "" {
		t.Fatalf("command block = %#v, want command fold metadata", byCallID["cmd"])
	}
}

func TestDecodeTransportSearchResultsFromContentText(t *testing.T) {
	raw := json.RawMessage(`{
		"content":"Public web search results for \"postgres standby join\". Use these results as evidence and cite URLs:\n1. PostgreSQL official docs: continuous archiving and point-in-time recovery\n   URL: https://www.postgresql.org/docs/current/continuous-archiving.html\n   Snippet: Official PostgreSQL recovery guidance, including timeline behavior during archive recovery.\n2. pg_auto_failover official docs: operations\n   URL: https://pg-auto-failover.readthedocs.io/en/main/operations.html\n   Snippet: Official pg_auto_failover operations guidance for monitor and failover workflows.\n",
		"query":"postgres standby join",
		"source":"public_web_search"
	}`)

	results := decodeTransportSearchResults(raw)
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2: %#v", len(results), results)
	}
	if results[0].Title != "PostgreSQL official docs: continuous archiving and point-in-time recovery" {
		t.Fatalf("results[0].Title = %q", results[0].Title)
	}
	if results[0].URL != "https://www.postgresql.org/docs/current/continuous-archiving.html" {
		t.Fatalf("results[0].URL = %q", results[0].URL)
	}
	if !strings.Contains(results[0].Snippet, "timeline behavior") {
		t.Fatalf("results[0].Snippet = %q, want timeline text", results[0].Snippet)
	}
	if results[1].URL != "https://pg-auto-failover.readthedocs.io/en/main/operations.html" {
		t.Fatalf("results[1].URL = %q", results[1].URL)
	}
}

func TestDecodeTransportSearchResultsDedupeByURLDomainAndLimit(t *testing.T) {
	raw := json.RawMessage(`{
		"results":[
			{"title":"pg_auto_failover operations","url":"https://pg-auto-failover.readthedocs.io/en/main/operations.html","snippet":"first"},
			{"title":"pg_auto_failover operations","url":"https://pg-auto-failover.readthedocs.io/en/main/operations.html#monitor","snippet":"duplicate same page"},
			{"title":"pg_auto_failover operations","url":"https://pg-auto-failover.readthedocs.io/en/main/ref/pg_autoctl_create_postgres.html","snippet":"same title same domain"},
			{"title":"pg_auto_failover state machine","url":"https://pg-auto-failover.readthedocs.io/en/main/failover-state-machine.html","snippet":"second same domain"},
			{"title":"pg_auto_failover FAQ","url":"https://pg-auto-failover.readthedocs.io/en/main/faq.html","snippet":"third same domain hidden"},
			{"title":"pgBackRest restore","url":"https://pgbackrest.org/user-guide.html#restore","snippet":"restore"},
			{"title":"pgBackRest command restore","url":"https://pgbackrest.org/command.html#command-restore","snippet":"command"},
			{"title":"PostgreSQL PITR","url":"https://www.postgresql.org/docs/current/continuous-archiving.html","snippet":"pitr"},
			{"title":"PostgreSQL recovery target timeline","url":"https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-RECOVERY-TARGET-TIMELINE","snippet":"timeline"}
		]
	}`)

	results := decodeTransportSearchResults(raw)
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5 capped results: %#v", len(results), results)
	}
	var pgAutoCount int
	for _, result := range results {
		if strings.Contains(result.URL, "pg-auto-failover.readthedocs.io") {
			pgAutoCount++
		}
		if result.Snippet == "duplicate same page" || result.Snippet == "same title same domain" || result.Snippet == "third same domain hidden" {
			t.Fatalf("unexpected duplicate/noisy result kept: %#v", result)
		}
	}
	if pgAutoCount != 2 {
		t.Fatalf("pg_auto_failover result count = %d, want 2 per domain cap: %#v", pgAutoCount, results)
	}
	if results[0].Title != "pg_auto_failover operations" || results[4].Title != "PostgreSQL PITR" {
		t.Fatalf("results order = %#v, want first useful sources preserved and capped", results)
	}
}

func TestTransportProjectorHidesUserProvidedEvidenceFromProcessTimeline(t *testing.T) {
	now := time.Date(2026, 6, 25, 9, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-user-evidence", "thread-user-evidence")
	userEvidenceData := json.RawMessage(`{
		"source":"user",
		"ref":"user-evidence:turn-user-evidence",
		"kinds":"command_output,log",
		"signals":"archive_recovery_active,archive_recovery_completed"
	}`)
	toolEvidenceData := json.RawMessage(`{
		"id":"metric-1",
		"kind":"metric",
		"summary":"replication lag increased",
		"source":"prometheus"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-user-evidence",
		SessionID:   "session-user-evidence",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "分析 pgBackRest 恢复后的 timeline 问题"}, CreatedAt: now},
			{ID: "user-evidence", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "user_provided", Summary: "user-provided evidence; kinds=command_output,log; signals=archive_recovery_active,archive_recovery_completed", Data: userEvidenceData}, CreatedAt: now.Add(100 * time.Millisecond)},
			{ID: "tool-evidence", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "metric", Summary: "replication lag increased", Data: toolEvidenceData}, CreatedAt: now.Add(200 * time.Millisecond)},
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "需要先核对恢复后的 timeline 历史和备份链。", Data: json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`)}, CreatedAt: now.Add(300 * time.Millisecond)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	transportTurn := projected.Turns["turn-user-evidence"]
	for _, block := range transportTurn.Process {
		if block.Text == "user-provided evidence; kinds=command_output,log; signals=archive_recovery_active,archive_recovery_completed" {
			t.Fatalf("user-provided evidence leaked into process block: %#v", block)
		}
		if block.Kind == AiopsTransportProcessKindEvidence && block.Source == "user" {
			t.Fatalf("user-provided evidence projected as visible evidence block: %#v", block)
		}
	}
	if evidenceBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindEvidence); evidenceBlock.Source != "prometheus" {
		t.Fatalf("tool evidence block = %#v, want real tool/source evidence preserved", evidenceBlock)
	}
	for _, item := range transportTurn.Timeline {
		if item.PayloadKind == "user_provided" || item.Text == "user-provided evidence; kinds=command_output,log; signals=archive_recovery_active,archive_recovery_completed" {
			t.Fatalf("user-provided evidence leaked into timeline: %#v", item)
		}
	}
}

func TestTransportProjectorPrefersFullUserMessageTextOverSummary(t *testing.T) {
	now := time.Date(2026, 5, 6, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-full-user", "thread-full-user")
	fullText := "请分析这个生产故障：请求只是让 AI 先根据已有日志和依赖信息判断原因，不是要求直接执行命令。这个输入较长，展示时必须完整保留，不应该落成摘要里的省略号。"
	userData := json.RawMessage(`{"messageId":"msg-1","prompt":` + strconv.Quote(fullText) + `}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-full-user",
		SessionID: "session-full-user",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now,
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "user-1",
				Type:   agentstate.TurnItemTypeUserMessage,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: "请分析这个生产故障：请求只是让 AI 先根据已有日志和依赖信息判断原因...",
					Data:    userData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	got := projected.Turns["turn-full-user"].User
	if got == nil {
		t.Fatal("turn.User = nil, want projected user message")
	}
	if got.Text != fullText {
		t.Fatalf("turn.User.Text = %q, want full text %q", got.Text, fullText)
	}
	if strings.Contains(got.Text, "...") {
		t.Fatalf("turn.User.Text should not use summary preview: %q", got.Text)
	}
}

func TestTransportProjectorProjectsOpsRunFromTurnMetadata(t *testing.T) {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-opsrun", "thread-opsrun")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-pg-status",
		"toolName":"exec_command",
		"displayKind":"tool",
		"inputSummary":"read-only pg replication status",
		"outputSummary":"LSN lag detected",
		"evidenceRefs":["evidence:pg:lsn","evidence:pg:network"]
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:           "turn-opsrun",
		SessionID:    "session-opsrun",
		ClientTurnID: "client-turn-opsrun",
		SessionType:  runtimekernel.SessionTypeHost,
		Mode:         runtimekernel.ModeInspect,
		Lifecycle:    runtimekernel.TurnLifecycleRunning,
		ResumeState:  runtimekernel.TurnResumeStateNone,
		StartedAt:    now,
		UpdatedAt:    now.Add(time.Second),
		Metadata: map[string]string{
			"aiops.opsRunId":                "opsrun-turn-opsrun",
			"aiops.chat.source":             "chat",
			"aiops.sessionId":               "session-opsrun",
			"aiops.turnId":                  "turn-opsrun",
			"aiops.clientTurnId":            "client-turn-opsrun",
			"aiops.route.mode":              "multi_host_ops",
			"aiops.target.summary":          "主机A/主机B PG 与主机C pg_mon",
			"aiops.tool.execCommandAllowed": "false",
			"enableToolPack":                "host_ops,public_web",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "主机A跟主机B上PG不同步，pg_mon部署在主机C，请修复"}, CreatedAt: now},
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "正在只读采集 PG 同步证据"}, CreatedAt: now.Add(500 * time.Millisecond)},
			{ID: "tool-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "LSN lag detected", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.OpsRun == nil {
		t.Fatal("OpsRun = nil, want projected ops run view")
	}
	if projected.OpsRun.ID != "opsrun-turn-opsrun" {
		t.Fatalf("OpsRun.ID = %q", projected.OpsRun.ID)
	}
	if projected.OpsRun.Status != "working" {
		t.Fatalf("OpsRun.Status = %q, want working", projected.OpsRun.Status)
	}
	if projected.OpsRun.Title != "主机A跟主机B上PG不同步，pg_mon部署在主机C，请修复" {
		t.Fatalf("OpsRun.Title = %q", projected.OpsRun.Title)
	}
	if projected.OpsRun.TargetSummary != "主机A/主机B PG 与主机C pg_mon" {
		t.Fatalf("OpsRun.TargetSummary = %q", projected.OpsRun.TargetSummary)
	}
	if projected.OpsRun.RouteMode != "multi_host_ops" {
		t.Fatalf("OpsRun.RouteMode = %q", projected.OpsRun.RouteMode)
	}
	if projected.OpsRun.ToolSurfaceSummary != "无主机执行 / WebLearn / HostOps" {
		t.Fatalf("OpsRun.ToolSurfaceSummary = %q", projected.OpsRun.ToolSurfaceSummary)
	}
	if projected.OpsRun.CurrentStep != "正在只读采集 PG 同步证据" {
		t.Fatalf("OpsRun.CurrentStep = %q", projected.OpsRun.CurrentStep)
	}
	if projected.OpsRun.EvidenceCount != 2 {
		t.Fatalf("OpsRun.EvidenceCount = %d, want 2", projected.OpsRun.EvidenceCount)
	}
	if len(projected.OpsRun.PostRunSuggestions) != 0 {
		t.Fatalf("OpsRun.PostRunSuggestions = %#v, want none while run is still working", projected.OpsRun.PostRunSuggestions)
	}
}

func TestTransportProjectorDoesNotExposeRouteSummaryBeforeModelFinal(t *testing.T) {
	now := time.Date(2026, 6, 23, 16, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-route-summary", "thread-route-summary")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-route-summary",
		SessionID:   "session-route-summary",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		StartedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]string{
			"aiops.route.mode":                      string(ChatRouteEvidenceRCA),
			"aiops.tool.execCommandAllowed":         "false",
			"aiops.tool.corootRCAAllowed":           "false",
			"aiops.weblearn.enabled":                "true",
			"aiops.weblearn.sourcePolicy":           "official_first",
			"aiops.weblearn.requiredWhenUnfamiliar": "true",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "分析 PG timeline 问题"}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	text := transportProjectionText(projected)
	for _, forbidden := range []string{
		"已识别为证据分析",
		"不会执行主机命令",
		"优先检索官方资料",
		"已进入咨询分析",
		"遇到不熟悉的中间件",
	} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("progress text leaked route summary %q:\n%s", forbidden, text)
		}
	}
	if strings.Contains(text, "Coroot") || strings.Contains(text, "@Coroot") {
		t.Fatalf("progress text should not expose Coroot routing policy:\n%s", text)
	}
}

func TestTransportProjectorHidesRuntimeInternalGateText(t *testing.T) {
	now := time.Date(2026, 6, 23, 16, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-gate", "thread-gate")
	gate := "verification completion gate: block_success_final: execution_required,missing_verification_report"
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-gate",
		SessionID:   "session-gate",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "启动容器"}, CreatedAt: now},
			{ID: "gate-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: gate}, CreatedAt: now.Add(time.Second)},
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: gate,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	text := transportProjectionText(projected)
	for _, forbidden := range []string{"verification completion gate", "block_success_final", "missing_verification_report"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("projection leaked internal gate text %q:\n%s", forbidden, text)
		}
	}
	if projected.Turns["turn-gate"].Final != nil {
		t.Fatalf("Final = %+v, want internal gate final hidden", projected.Turns["turn-gate"].Final)
	}
}

func TestTransportProjectorHidesRiskyFinalDraft(t *testing.T) {
	now := time.Date(2026, 6, 23, 16, 45, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-risky-draft", "thread-risky-draft")
	risky := "可以直接执行 rm -rf $PG_DATA/recovery/repos/archive/paf/15-1/* 清理 archive 后继续。"
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-risky-draft",
		SessionID: "session-risky-draft",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-risky", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: risky, Data: json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`)}, CreatedAt: now.Add(time.Second)},
			{ID: "reasoning-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "正在修正最终回答"}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	text := transportProjectionText(projected)
	if strings.Contains(text, "rm -rf $PG_DATA") || strings.Contains(text, "archive/paf") {
		t.Fatalf("projected text leaked risky draft: %q", text)
	}
	if projected.Turns["turn-risky-draft"].Final != nil {
		t.Fatalf("Final = %+v, want risky final draft hidden", projected.Turns["turn-risky-draft"].Final)
	}
}

func TestTransportProjectorRedactsRiskyOperationsWithoutDroppingFinalAnalysis(t *testing.T) {
	now := time.Date(2026, 6, 24, 9, 45, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-risky-analysis", "thread-risky-analysis")
	final := strings.Join([]string{
		"# PG timeline 分析",
		"",
		"结论：主机A和主机B已经走到不同 timeline 分支。",
		"",
		"## 修复方向",
		"可以执行 rm -rf $PG_DATA/recovery/repos/archive/paf/15-1/* 清理 archive。",
		"下一步先收集 pgbackrest info 和恢复日志。",
	}, "\n")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-risky-analysis",
		SessionID: "session-risky-analysis",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: final,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedFinal := projected.Turns["turn-risky-analysis"].Final
	if projectedFinal == nil {
		t.Fatal("Final = nil, want RCA analysis with risky operation redacted")
	}
	if !strings.Contains(projectedFinal.Text, "不同 timeline 分支") || !strings.Contains(projectedFinal.Text, "下一步先收集") {
		t.Fatalf("Final.Text = %q, want safe analysis preserved", projectedFinal.Text)
	}
	for _, forbidden := range []string{"rm -rf", "archive/paf"} {
		if strings.Contains(projectedFinal.Text, forbidden) {
			t.Fatalf("Final.Text leaked risky operation %q: %q", forbidden, projectedFinal.Text)
		}
	}
	if strings.Contains(projectedFinal.Text, "高风险操作已隐藏") || strings.Contains(projectedFinal.Text, "涉及清空数据目录或归档清理") {
		t.Fatalf("Final.Text = %q, want risky operation removed without appended notice", projectedFinal.Text)
	}
}

func TestTransportProjectorPreservesGatedRCAFinalWithRiskKeywords(t *testing.T) {
	now := time.Date(2026, 6, 25, 23, 45, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-gated-rca", "thread-gated-rca")
	final := strings.Join([]string{
		"结论（置信度：低）：从节点 timeline 高于主节点，表明从节点经历了额外的 promote 或沿用了不同的 timeline 分支，但当前无任何主机直接证据，所有原因均为假设。最可能的三条机制路径是：①主机 B 的 $PGDATA 未清空，残留旧 timeline history 文件，pg_autoctl create postgres 跳过了 pg_basebackup 而沿用旧数据；②pgbackrest 存档中存在比主节点当前 timeline 更高的 WAL 分支，从节点 restore_command 或 recovery_target_timeline='latest' 使其 replay 到更高端；③从节点重复执行 pg_autoctl create postgres 时被 monitor 短暂分配为 single/wait_primary 状态而自动 promote。",
		"",
		"关键证据边界：以上均为推断，未验证。缺失的只读证据包括：主节点和从节点的 pg_control_checkpoint() 输出；从节点 $PGDATA/pg_wal/ 下的 .history 文件内容；从节点 postgresql.auto.conf 中的 restore_command、recovery_target_timeline、primary_conninfo 设置；pgbackrest 存档中各 timeline 的 WAL 列表；pg_auto_failover monitor 上 pg_autoctl show state 的完整输出。",
		"",
		"下一步只读检查（在不做任何变更的前提下）：1) 在主机 A 和 B 上分别执行 SELECT timeline_id, redo_wal FROM pg_control_checkpoint(); 2) 在主机 B 上检查 $PGDATA/pg_wal/*.history 和 postgresql.auto.conf；3) 在主机 C 上执行 pg_autoctl show state；4) 用 pgbackrest info 查看存档中存在的 timeline 数量及分支。确认根因后，若需修复从节点，必须选定变更窗口、确认主节点为权威数据源、备份从节点现有数据目录后，再考虑通过 pg_basebackup 从主节点重建从节点；切勿在未验证数据权威性的情况下直接 promote 或切换 timeline。",
	}, "\n")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-gated-rca",
		SessionID: "session-gated-rca",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: final,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedFinal := projected.Turns["turn-gated-rca"].Final
	if projectedFinal == nil {
		t.Fatal("Final = nil, want gated RCA answer preserved")
	}
	for _, want := range []string{
		"结论（置信度：低）",
		"$PGDATA 未清空",
		"关键证据边界",
		"下一步只读检查",
		"变更窗口",
		"切勿",
	} {
		if !strings.Contains(projectedFinal.Text, want) {
			t.Fatalf("Final.Text missing %q:\n%s", want, projectedFinal.Text)
		}
	}
}

func TestTransportProjectorPreservesResidualPGDataCauseLine(t *testing.T) {
	now := time.Date(2026, 6, 26, 0, 25, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-residual-pgdata", "thread-residual-pgdata")
	final := strings.Join([]string{
		"可能导致 B timeline 更高的具体原因：",
		"1. **B 的 `$PGDATA` 未完全清空**：步骤 4.2 要求清空，但若残留旧 `.history` 文件或 WAL 段，PostgreSQL 启动时可能识别到更高 timeline 起点并沿其分支。",
		"2. **pg_autoctl 将 B 初始化为独立主库而非 standby**：若 monitor 中有旧节点残留，可能触发 promote 产生新 timeline。",
		"",
		"下一步（只读检查，确认根因后再考虑修复）：检查 B 的 pg_controldata 和 .history 文件。",
	}, "\n")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-residual-pgdata",
		SessionID: "session-residual-pgdata",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: final,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedFinal := projected.Turns["turn-residual-pgdata"].Final
	if projectedFinal == nil {
		t.Fatal("Final = nil, want RCA final preserved")
	}
	for _, want := range []string{"1. **B 的 `$PGDATA` 未完全清空**", "2. **pg_autoctl 将 B 初始化为独立主库"} {
		if !strings.Contains(projectedFinal.Text, want) {
			t.Fatalf("Final.Text missing %q:\n%s", want, projectedFinal.Text)
		}
	}
}

func TestTransportProjectorClearsStaleHostOpsMissionForHostBoundChat(t *testing.T) {
	now := time.Date(2026, 6, 24, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-host-bound", "thread-host-bound")
	state.ActiveHostMissionID = "hostops:turn-host-bound"
	state.HostMissions["hostops:turn-host-bound"] = AiopsTransportHostMission{
		ID:           "hostops:turn-host-bound",
		TurnID:       "turn-host-bound",
		Status:       "planning",
		PlanRequired: false,
		MentionedHosts: []AiopsTransportHostMention{
			{Raw: "@remote-120-77-239-90", HostID: "remote-120-77-239-90", Resolved: true},
		},
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-host-bound",
		SessionID: "session-host-bound",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		Metadata: map[string]string{
			"aiops.route.mode":           string(ChatRouteHostBoundOps),
			"aiops.target.binding":       "host",
			"aiops.target.hostId":        "remote-120-77-239-90",
			"aiops.hostops.routeKind":    "host_ops",
			"aiops.hostops.planRequired": "false",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "只读采集完成。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if projected.ActiveHostMissionID != "" {
		t.Fatalf("ActiveHostMissionID = %q, want stale mission cleared", projected.ActiveHostMissionID)
	}
	if _, ok := projected.HostMissions["hostops:turn-host-bound"]; ok {
		t.Fatalf("HostMissions still contains stale mission: %#v", projected.HostMissions)
	}
}

func TestTransportProjectorMergesCanonicalHostMissionFromTypedToolPayload(t *testing.T) {
	now := time.Date(2026, 7, 12, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-host-mission", "thread-host-mission")
	state.ActiveHostMissionID = "mission-runtime"
	state.HostMissions["mission-runtime"] = AiopsTransportHostMission{
		ID:           "mission-runtime",
		TurnID:       "turn-host-mission",
		Status:       "planning",
		PlanRequired: true,
		PlanAccepted: false,
	}
	toolPayload := json.RawMessage(`{
		"toolCallId":"call-wait",
		"toolName":"wait_host_agents",
		"displayKind":"hostops.wait_host_agents",
		"displayData":{
			"schemaVersion":"aiops.hostops.wait/v1",
			"mission":{"id":"mission-runtime","planRequired":true,"planAccepted":true,"status":"spawning_children"},
			"children":[{"id":"child-a","childAgentId":"child-a","missionId":"mission-runtime","hostId":"host-a","hostDisplayName":"host-a","sessionId":"host-child:mission-runtime:host-a","status":"completed"}]
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-host-mission",
		SessionID: "session-host-mission",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		Metadata: map[string]string{
			"aiops.route.mode":             "multi_host_ops",
			"aiops.hostops.missionId":      "mission-runtime",
			"aiops.hostops.planRequired":   "true",
			"aiops.hostops.planAccepted":   "false",
			"aiops.hostops.managerAgentId": "manager-runtime",
		},
		AgentItems: []agentstate.TurnItem{{
			ID: "wait-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted,
			Payload:   agentstate.PayloadEnvelope{Kind: "tool", Summary: "host agents complete", Data: toolPayload},
			CreatedAt: now.Add(time.Second), UpdatedAt: now.Add(time.Second),
		}},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	mission, ok := projected.HostMissions["mission-runtime"]
	if !ok {
		t.Fatalf("HostMissions = %#v, want mission-runtime", projected.HostMissions)
	}
	if projected.ActiveHostMissionID != "mission-runtime" || !mission.PlanRequired || !mission.PlanAccepted {
		t.Fatalf("projected mission = %#v, active = %q; want canonical accepted mission", mission, projected.ActiveHostMissionID)
	}
	if len(projected.HostMissions) != 1 || len(mission.ChildAgentIDs) != 1 || mission.ChildAgentIDs[0] != "child-a" {
		t.Fatalf("projected mission = %#v, want one mission preserving child projection", mission)
	}
}

func TestTransportProjectorAddsPostRunSuggestionsOnlyForUsefulTerminalOpsRun(t *testing.T) {
	now := time.Date(2026, 6, 23, 15, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-post-run", "thread-post-run")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-post-run",
		SessionID:   "session-post-run",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Minute),
		Metadata: map[string]string{
			"aiops.opsRunId":    "opsrun-post-run",
			"aiops.chat.source": "chat",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "修复 redis 主从复制异常"}, CreatedAt: now},
			{ID: "evidence-1", Type: agentstate.TurnItemTypeEvidence, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "redis replica_link_status=down"}, CreatedAt: now.Add(time.Second)},
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "已恢复复制，建议沉淀处理记录。", Data: json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`)}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if projected.OpsRun == nil {
		t.Fatal("OpsRun = nil, want projected ops run")
	}
	if !transportOpsRunHasPostRunSuggestion(projected.OpsRun.PostRunSuggestions, PostRunSuggestionRunRecord) ||
		!transportOpsRunHasPostRunSuggestion(projected.OpsRun.PostRunSuggestions, PostRunSuggestionExperienceCandidate) ||
		!transportOpsRunHasPostRunSuggestion(projected.OpsRun.PostRunSuggestions, PostRunSuggestionCase) {
		t.Fatalf("PostRunSuggestions = %#v, want reusable post-run actions", projected.OpsRun.PostRunSuggestions)
	}
}

func TestTransportProjectorKeepsCheckpointInAgentRunWithoutChatProcessLabel(t *testing.T) {
	now := time.Date(2026, 6, 23, 14, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-checkpoint", "thread-checkpoint")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-checkpoint",
		SessionID:   "session-checkpoint",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		Iteration:   1,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		Metadata: map[string]string{
			"aiops.opsRunId":       "opsrun-checkpoint",
			"aiops.sessionId":      "session-checkpoint",
			"aiops.turnId":         "turn-checkpoint",
			"aiops.target.summary": "service:checkout",
		},
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:                 "checkpoint-approval-1",
			SessionID:          "session-checkpoint",
			TurnID:             "turn-checkpoint",
			Iteration:          1,
			Sequence:           2,
			Kind:               "approval_needed",
			Lifecycle:          runtimekernel.TurnLifecycleSuspended,
			ResumeState:        runtimekernel.TurnResumeStatePendingApproval,
			ToolSurfaceSummary: "HostOps / Coroot RCA",
			TargetRefs:         []string{"service:checkout"},
			EvidenceRefs:       []string{"evidence-1"},
			CreatedAt:          now,
			UpdatedAt:          now,
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedTurn := projected.Turns["turn-checkpoint"]
	for _, block := range projectedTurn.Process {
		if block.Kind == AiopsTransportProcessKindSystem && strings.Contains(block.DisplayKind, "checkpoint") {
			t.Fatalf("checkpoint leaked as chat process block: %#v", block)
		}
	}
	if projected.OpsRun == nil || projected.OpsRun.AgentRun == nil {
		t.Fatalf("projected OpsRun/AgentRun missing: %#v", projected.OpsRun)
	}
	var checkpointStep *AgentStepView
	for i := range projected.OpsRun.AgentRun.Steps {
		if projected.OpsRun.AgentRun.Steps[i].CheckpointID == "checkpoint-approval-1" {
			checkpointStep = &projected.OpsRun.AgentRun.Steps[i]
			break
		}
	}
	if checkpointStep == nil ||
		checkpointStep.Kind != AgentStepKindCheckpoint ||
		checkpointStep.Status != AgentStepStatusWaitingApproval ||
		checkpointStep.CheckpointID != "checkpoint-approval-1" {
		t.Fatalf("checkpoint agent step = %#v", checkpointStep)
	}
}

func TestTransportProjectorHidesNonActionableCheckpointProcessBlocks(t *testing.T) {
	now := time.Date(2026, 6, 23, 14, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-checkpoint-hidden", "thread-checkpoint-hidden")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-checkpoint-hidden",
		SessionID: "session-checkpoint-hidden",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		LatestCheckpoint: &runtimekernel.CheckpointMetadata{
			ID:        "checkpoint-final",
			Kind:      runtimekernel.CheckpointKindFinalResponse,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Iterations: []runtimekernel.IterationState{{
			Checkpoint: &runtimekernel.CheckpointMetadata{
				ID:        "checkpoint-after-tool",
				Kind:      runtimekernel.CheckpointKindAfterToolCall,
				CreatedAt: now.Add(500 * time.Millisecond),
				UpdatedAt: now.Add(500 * time.Millisecond),
			},
		}},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	text := transportProjectionText(projected)
	for _, forbidden := range []string{"checkpoint", "工具调用后", "最终响应"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("projection leaked non-actionable checkpoint text %q:\n%s", forbidden, text)
		}
	}
}

func TestTransportProjectorDoesNotExposeFallbackRetryManualStates(t *testing.T) {
	now := time.Date(2026, 6, 2, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-harness", "thread-harness")
	completedTool := json.RawMessage(`{
		"toolCallId":"tool-completed",
		"toolName":"coroot.service_metrics",
		"displayKind":"tool",
		"inputSummary":"payment-api metrics",
		"outputSummary":"metrics collected"
	}`)
	failedTool := json.RawMessage(`{
		"toolCallId":"tool-failed",
		"toolName":"coroot.missing_tool",
		"displayKind":"tool",
		"inputSummary":"missing tool",
		"outputSummary":"tool not found"
	}`)
	approvalData := json.RawMessage(`{
		"approvalId":"approval-blocked",
		"approvalType":"command",
		"command":"kubectl rollout restart deployment/payment-api",
		"reason":"mutating command"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-harness-states",
		SessionID:   "session-harness",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "reasoning-running", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "checking current tool surface"}, CreatedAt: now},
			{ID: "tool-completed", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "metrics collected", Data: completedTool}, CreatedAt: now.Add(100 * time.Millisecond)},
			{ID: "tool-failed", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "tool not found", Data: failedTool}, CreatedAt: now.Add(200 * time.Millisecond)},
			{ID: "approval-blocked", Type: agentstate.TurnItemTypeApproval, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "需要审批", Data: approvalData}, CreatedAt: now.Add(300 * time.Millisecond)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	assertTransportProjectionHasProcessStatuses(t, projected, []AiopsTransportProcessStatus{
		AiopsTransportProcessStatusRunning,
		AiopsTransportProcessStatusCompleted,
		AiopsTransportProcessStatusFailed,
		AiopsTransportProcessStatusBlocked,
	})
	assertNoForbiddenTransportProjectionStates(t, projected, []string{
		"fallback_planned",
		"retry_scheduled",
		"manual_reconcile",
	})
}

func TestTransportProjectorProjectsContextGovernanceAndExternalizedToolResult(t *testing.T) {
	now := time.Date(2026, 5, 22, 8, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-context", "thread-context")
	toolData := json.RawMessage(`{
		"toolCallId":"call-large-logs",
		"toolName":"logs.search",
		"displayKind":"logs.search",
		"inputSummary":"nginx timeout logs",
		"outputSummary":"large logs externalized",
		"outputPreview":"Large nginx log result was externalized.",
		"materializationTier":"large",
		"originalBytes":48213,
		"inlineBytes":920,
		"externalReferences":[{
			"id":"spill-1",
			"kind":"blob",
			"uri":"store://tool-spills/spill-1",
			"title":"nginx raw logs",
			"summary":"17 upstream timeout lines",
			"contentType":"text/plain",
			"digest":"sha256:abc",
			"bytes":48213
		}]
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-context",
		SessionID:   "session-context",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		ContextGovernanceEvents: []runtimekernel.ContextGovernanceEvent{
			{
				ID:        "ctxgov-l4-started",
				Layer:     runtimekernel.ContextGovernanceLayerL4,
				Kind:      "context.compaction.started",
				Message:   "正在压缩上下文，当前任务会继续",
				Budget:    runtimekernel.DefaultContextBudgetPolicy(20000, 8000).Thresholds(),
				CreatedAt: now,
			},
			{
				ID:        "ctxgov-l5-failed",
				Layer:     runtimekernel.ContextGovernanceLayerL5,
				Kind:      "context.compaction.failed",
				Message:   "上下文过长，已使用本地摘要继续",
				CreatedAt: now.Add(time.Second),
			},
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-large-logs", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "logs.search", Summary: "large logs", Data: toolData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedTurn := projected.Turns["turn-context"]
	if len(projectedTurn.ContextGovernance) != 2 {
		t.Fatalf("context governance = %#v, want 2 events", projectedTurn.ContextGovernance)
	}
	if projectedTurn.ContextGovernance[1].RetryAttempt != 0 || projectedTurn.ContextGovernance[1].RetryMax != 0 {
		t.Fatalf("retry counters = %#v, want none", projectedTurn.ContextGovernance[1])
	}
	if got := projectedTurn.ContextGovernance[0].Budget["smallContextMode"]; got != true {
		t.Fatalf("smallContextMode budget = %#v, want true", got)
	}
	if len(projectedTurn.Process) != 1 {
		t.Fatalf("process blocks = %d, want 1", len(projectedTurn.Process))
	}
	block := projectedTurn.Process[0]
	if block.MaterializationTier != "large" || block.OriginalBytes != 48213 || block.InlineBytes != 920 {
		t.Fatalf("materialization = %#v", block)
	}
	if len(block.ExternalReferences) != 1 || block.ExternalReferences[0].ID != "spill-1" {
		t.Fatalf("external references = %#v", block.ExternalReferences)
	}
}

func TestTransportProjectorPreservesCommandWhenToolResultOnlyHasOutput(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 38, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolCallData := json.RawMessage(`{
		"id":"call-uptime",
		"name":"exec_command",
		"arguments":{"command":"uptime"}
	}`)
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-uptime",
		"toolName":"exec_command"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-command-output",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTime(now.Add(2 * time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "exec_command", Data: toolCallData}, CreatedAt: now},
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "22:38 up 22 days, 8:23, 1 user", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-command-output"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "uptime" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if commandBlock.OutputPreview != "" {
		t.Fatalf("command block output = %q, want no preview when tool result only has summary", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorProjectsToolMockAndEvidenceRefs(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-metrics",
		"toolName":"coroot.service_metrics",
		"displayKind":"coroot.metrics",
		"inputSummary":"redis-local-01 memory",
		"outputSummary":"rss/used_memory ratio is 1.8",
		"mock":true,
		"evidenceRefs":["evidence:redis:rss","evidence:redis:events"]
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-tool-evidence",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTime(now.Add(time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "rss/used_memory ratio is 1.8", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	toolBlock := findTransportProcessBlock(t, projected.Turns["turn-tool-evidence"].Process, AiopsTransportProcessKindTool)
	if toolBlock.Source != "coroot.service_metrics" || !strings.Contains(toolBlock.InputSummary, "redis-local-01 memory") {
		t.Fatalf("tool block source/input = %q/%q, want tool name and input summary", toolBlock.Source, toolBlock.InputSummary)
	}
	if !toolBlock.Mock {
		t.Fatalf("tool block Mock = false, want true: %+v", toolBlock)
	}
	if got, want := strings.Join(toolBlock.EvidenceRefs, ","), "evidence:redis:rss,evidence:redis:events"; got != want {
		t.Fatalf("tool block EvidenceRefs = %q, want %q", got, want)
	}
}

func TestTransportProjectorBackfillsCommandPreviewFromSnapshotToolResult(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 39, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	fullOutput := "PID PPID %MEM RSS STAT COMM\n1 0 0.1 1024 S launchd\n2 1 1.3 204800 S Google Chrome Helper"
	toolCallData := json.RawMessage(`{
		"id":"call-ps",
		"name":"exec_command",
		"arguments":{"command":"ps","args":["-axo","pid,ppid,%mem,rss,state,comm"]}
	}`)
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-ps",
		"toolName":"exec_command",
		"outputSummary":"PID PPID %MEM RSS STAT COMM"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-command-preview",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTime(now.Add(2 * time.Second)),
		Iterations: []runtimekernel.IterationState{
			{
				ID:          "iter-1",
				SessionID:   "session-1",
				TurnID:      "turn-command-preview",
				Iteration:   0,
				Lifecycle:   runtimekernel.TurnLifecycleCompleted,
				ResumeState: runtimekernel.TurnResumeStateNone,
				ToolResults: []runtimekernel.ToolResult{{ToolCallID: "call-ps", Content: fullOutput}},
				StartedAt:   now,
				UpdatedAt:   now.Add(2 * time.Second),
			},
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "exec_command", Data: toolCallData}, CreatedAt: now},
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "PID PPID %MEM RSS STAT COMM", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-command-preview"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "ps -axo pid,ppid,%mem,rss,state,comm" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if !strings.Contains(commandBlock.OutputPreview, "launchd") || !strings.Contains(commandBlock.OutputPreview, "Google Chrome Helper") {
		t.Fatalf("command block output preview = %q, want full multi-line preview", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorUsesTerminalStreamsFromStructuredCommandPreview(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 41, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-hostname",
		"toolName":"exec_command",
		"inputSummary":"hostname",
		"outputPreview":{
			"command":"hostname",
			"status":"ok",
			"stdout":"host-a\n",
			"stderr":"",
			"exitCode":0,
			"tool":"exec_command"
		},
		"exitCode":0
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-command-structured-preview",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTime(now.Add(time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "host-a", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-command-structured-preview"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.OutputPreview != "host-a" {
		t.Fatalf("command output preview = %q, want stdout only", commandBlock.OutputPreview)
	}
	if strings.Contains(commandBlock.OutputPreview, "stdout") || strings.Contains(commandBlock.OutputPreview, "tool") {
		t.Fatalf("command output preview leaked structured envelope: %q", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorKeepsCommandTitleSeparateFromResultOnlyOutput(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 40, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-cpu-brand",
		"toolName":"exec_command",
		"inputSummary":"sysctl -n machdep.cpu.brand_string",
		"outputSummary":"Apple M5",
		"outputPreview":"Apple M5"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-result-only-command",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTime(now.Add(time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "cmd-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "command", Summary: "Apple M5", Data: toolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-result-only-command"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.Command != "sysctl -n machdep.cpu.brand_string" {
		t.Fatalf("command block command = %q, want real command", commandBlock.Command)
	}
	if commandBlock.Text == "Apple M5" {
		t.Fatalf("command block text = %q, should not use stdout as title", commandBlock.Text)
	}
	if commandBlock.OutputPreview != "Apple M5" {
		t.Fatalf("command block output = %q, want stdout in output preview", commandBlock.OutputPreview)
	}
}

func TestTransportProjectorProjectsSnapshotPendingApprovalWithoutApprovalItem(t *testing.T) {
	now := time.Date(2026, 5, 7, 14, 42, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-pending-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingApproval,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "查看系统版本"}, CreatedAt: now},
		},
		PendingApprovals: []runtimekernel.PendingApproval{
			{
				ID:             "approval-inline-1",
				SessionID:      "session-1",
				TurnID:         "turn-pending-approval",
				ToolName:       "exec_command",
				ToolCallID:     "call-sw-vers",
				HostID:         "server-local",
				Command:        "sw_vers",
				Reason:         "需要确认后执行命令",
				Risk:           "medium",
				Source:         "ai_chat_direct",
				ExpectedEffect: "读取系统版本",
				Rollback:       "无需回滚",
				Validation:     "确认 sw_vers 返回 ProductVersion",
				ResourceScopes: []string{"host:server-local", "os:darwin"},
				Status:         "pending",
				CreatedAt:      now.Add(time.Second),
				UpdatedAt:      now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("projected.Status = %q, want blocked", projected.Status)
	}
	if _, ok := projected.PendingApprovals["approval-inline-1"]; !ok {
		t.Fatalf("PendingApprovals = %#v, want snapshot approval", projected.PendingApprovals)
	}
	approval := projected.PendingApprovals["approval-inline-1"]
	if approval.Type != "command" || approval.Command != "sw_vers" || approval.Reason != "需要确认后执行命令" {
		t.Fatalf("approval = %+v, want command and reason from snapshot", approval)
	}
	if approval.Validation != "确认 sw_vers 返回 ProductVersion" {
		t.Fatalf("approval validation = %q, want validation from snapshot", approval.Validation)
	}
	block := findTransportProcessBlock(t, projected.Turns["turn-pending-approval"].Process, AiopsTransportProcessKindApproval)
	if block.ApprovalID != "approval-inline-1" || block.Command != "sw_vers" || block.Status != AiopsTransportProcessStatusBlocked {
		t.Fatalf("approval block = %+v, want inline blocked approval block", block)
	}
	if block.TargetSummary != "host:server-local；os:darwin" || block.Risk != "medium" || block.Source != "ai_chat_direct" {
		t.Fatalf("approval block scope/risk/source = %+v", block)
	}
	if block.ExpectedEffect != "读取系统版本" || block.Rollback != "无需回滚" || block.Validation != "确认 sw_vers 返回 ProductVersion" || !strings.Contains(block.RiskSummary, "medium") {
		t.Fatalf("approval block effect/rollback/risk summary = %+v", block)
	}
}

func TestTransportProjectorProjectsSnapshotPendingEvidenceAsInlineApproval(t *testing.T) {
	now := time.Date(2026, 5, 8, 1, 12, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	toolCallData := json.RawMessage(`{
		"toolCallId":"call-ifconfig-down",
		"toolName":"exec_command",
		"inputSummary":"ifconfig en0 down"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-pending-evidence",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleSuspended,
		ResumeState: runtimekernel.TurnResumeStatePendingEvidence,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "运行 ifconfig en0 down"}, CreatedAt: now},
			{ID: "tool-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "evidence required", Data: toolCallData}, CreatedAt: now.Add(time.Second)},
		},
		PendingEvidence: []runtimekernel.PendingEvidence{
			{
				ID:         "evidence-inline-1",
				SessionID:  "session-1",
				TurnID:     "turn-pending-evidence",
				ToolName:   "exec_command",
				ToolCallID: "call-ifconfig-down",
				Reason:     "需要确认后执行命令",
				Status:     "pending",
				CreatedAt:  now.Add(time.Second),
				UpdatedAt:  now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("projected.Status = %q, want blocked", projected.Status)
	}
	approval := projected.PendingApprovals["evidence-inline-1"]
	if approval.ID == "" || approval.Type != "command" || approval.Command != "ifconfig en0 down" {
		t.Fatalf("evidence approval = %+v, want command approval projection", approval)
	}
	commandBlock := findTransportProcessBlock(t, projected.Turns["turn-pending-evidence"].Process, AiopsTransportProcessKindCommand)
	if commandBlock.ApprovalID != "evidence-inline-1" {
		t.Fatalf("command block approvalId = %q, want evidence-inline-1", commandBlock.ApprovalID)
	}
}

func TestTransportProjectorPrunesStalePendingApprovalsForTurn(t *testing.T) {
	now := time.Date(2026, 5, 8, 2, 10, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	state.PendingApprovals["stale-approval"] = AiopsTransportApproval{
		ID:     "stale-approval",
		TurnID: "turn-stale-approval",
		Type:   "command",
		Status: string(AiopsTransportProcessStatusBlocked),
	}
	state.RuntimeLiveness.PendingApprovals["stale-approval"] = true
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-stale-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		CompletedAt: ptrTransportProjectorTime(now.Add(time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "approval no longer pending",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if _, ok := projected.PendingApprovals["stale-approval"]; ok {
		t.Fatalf("stale approval was not pruned: %#v", projected.PendingApprovals)
	}
	if projected.RuntimeLiveness.PendingApprovals["stale-approval"] {
		t.Fatalf("stale approval liveness was not pruned: %#v", projected.RuntimeLiveness.PendingApprovals)
	}
}

func TestTransportProjectorIgnoresSnapshotPendingGatesForTerminalTurn(t *testing.T) {
	now := time.Date(2026, 5, 8, 3, 10, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-denied-approval",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeExecute,
		Lifecycle:   runtimekernel.TurnLifecycleFailed,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		Error:       "approval denied",
		PendingApprovals: []runtimekernel.PendingApproval{{
			ID:        "approval-stale",
			TurnID:    "turn-denied-approval",
			Command:   "ifconfig en0 down",
			Status:    "pending",
			CreatedAt: now,
			UpdatedAt: now,
		}},
		PendingEvidence: []runtimekernel.PendingEvidence{{
			ID:         "evidence-stale",
			TurnID:     "turn-denied-approval",
			ToolCallID: "tool-call-1",
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}},
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "approval-item-stale",
				Type:      agentstate.TurnItemTypeApproval,
				Status:    agentstate.ItemStatusBlocked,
				Payload:   agentstate.PayloadEnvelope{Summary: "等待审批"},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if len(projected.PendingApprovals) != 0 {
		t.Fatalf("PendingApprovals = %#v, want terminal turn pending gates ignored", projected.PendingApprovals)
	}
	if len(projected.RuntimeLiveness.PendingApprovals) != 0 {
		t.Fatalf("RuntimeLiveness.PendingApprovals = %#v, want cleared", projected.RuntimeLiveness.PendingApprovals)
	}
	for _, block := range projected.Turns["turn-denied-approval"].Process {
		if block.Kind == AiopsTransportProcessKindApproval && block.Status == AiopsTransportProcessStatusBlocked {
			t.Fatalf("terminal turn kept blocked approval block: %+v", block)
		}
	}
}

func TestTransportProjectorProjectsFailedTurnState(t *testing.T) {
	now := time.Date(2026, 5, 6, 13, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-failed",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleFailed,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		Error:       "command failed: exit status 1",
		AgentItems: []agentstate.TurnItem{
			{ID: "err-1", Type: agentstate.TurnItemTypeError, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Summary: "command failed: exit status 1"}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusFailed {
		t.Fatalf("Status = %q, want %q", projected.Status, AiopsTransportStatusFailed)
	}
	if projected.LastError != "command failed: exit status 1" {
		t.Fatalf("LastError = %q, want runtime error", projected.LastError)
	}
	if projected.Turns["turn-failed"].Status != AiopsTransportTurnStatusFailed {
		t.Fatalf("turn status = %q, want failed", projected.Turns["turn-failed"].Status)
	}
}

func TestTransportProjectorDeduplicatesTerminalRuntimeErrors(t *testing.T) {
	now := time.Date(2026, 6, 30, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-duplicate-error", "thread-duplicate-error")
	errorText := `模型请求超时：约 20s 未收到模型服务响应，请检查 LLM 地址、网络连通性或代理配置: Post "https://provider.invalid/v1/chat/completions": net/http: TLS handshake timeout`
	wantVisibleError := "模型服务连接超时，未能建立连接。上下文较大或模型服务繁忙时可能需要更长时间，请稍后重试。"
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-duplicate-error",
		SessionID: "session-duplicate-error",
		Lifecycle: runtimekernel.TurnLifecycleFailed,
		StartedAt: now,
		UpdatedAt: now.Add(time.Minute),
		Error:     errorText,
		AgentItems: []agentstate.TurnItem{
			{ID: "err-1", Type: agentstate.TurnItemTypeError, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Summary: errorText}, CreatedAt: now.Add(time.Second)},
			{ID: "err-2", Type: agentstate.TurnItemTypeError, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Summary: errorText}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	var runtimeErrors []AiopsProcessBlock
	for _, block := range projected.Turns["turn-duplicate-error"].Process {
		if block.DisplayKind == "runtime.error" {
			runtimeErrors = append(runtimeErrors, block)
		}
	}
	if len(runtimeErrors) != 1 {
		t.Fatalf("runtime error blocks = %#v, want one deduplicated error", runtimeErrors)
	}
	if runtimeErrors[0].Text != wantVisibleError {
		t.Fatalf("runtime error text = %q, want %q", runtimeErrors[0].Text, wantVisibleError)
	}
	if projected.LastError != wantVisibleError {
		t.Fatalf("LastError = %q, want sanitized runtime error", projected.LastError)
	}
	for _, forbidden := range []string{"provider.invalid", "chat/completions", "Post ", "TLS handshake timeout", "约 20s"} {
		if strings.Contains(runtimeErrors[0].Text, forbidden) || strings.Contains(projected.LastError, forbidden) {
			t.Fatalf("visible runtime error leaked %q: block=%q lastError=%q", forbidden, runtimeErrors[0].Text, projected.LastError)
		}
	}
}

func TestTransportProjectorProjectsCanceledTurnState(t *testing.T) {
	now := time.Date(2026, 5, 6, 14, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-canceled",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCanceled,
		ResumeState: runtimekernel.TurnResumeStateNone,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.Status != AiopsTransportStatusCanceled {
		t.Fatalf("Status = %q, want %q", projected.Status, AiopsTransportStatusCanceled)
	}
	if projected.Turns["turn-canceled"].Status != AiopsTransportTurnStatusCanceled {
		t.Fatalf("turn status = %q, want canceled", projected.Turns["turn-canceled"].Status)
	}
}

func TestTransportProjectorUsesAssistantMessageFinalWhenSnapshotFinalOutputIsMissing(t *testing.T) {
	now := time.Date(2026, 5, 7, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-output",
		SessionID:   "session-1",
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTransportProjectorTime(now.Add(2 * time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "user-1", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "ping"}, CreatedAt: now},
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "这是来自 assistant_message 的最终回答",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	transportTurn := projected.Turns["turn-final-output"]
	if projected.Status != AiopsTransportStatusIdle || transportTurn.Status != AiopsTransportTurnStatusCompleted {
		t.Fatalf("projected status = %q turn=%q, want idle/completed", projected.Status, transportTurn.Status)
	}
	if transportTurn.Final == nil || transportTurn.Final.Text != "这是来自 assistant_message 的最终回答" {
		t.Fatalf("turn.Final = %+v, want assistant_message final", transportTurn.Final)
	}
	if projected.RuntimeLiveness.ActiveTurns["turn-final-output"] {
		t.Fatalf("ActiveTurns = %#v, want terminal turn inactive", projected.RuntimeLiveness.ActiveTurns)
	}
}

func TestTransportProjectorUsesAssistantMessageAsOnlyTranscriptFinalSource(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-single-assistant-final",
		SessionID:   "session-single-assistant-final",
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTransportProjectorTime(now.Add(2 * time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "assistant-final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "来自 committed assistant message 的最终回答。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
			{ID: "final-response-1", Type: agentstate.TurnItemTypeFinalResponse, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "legacy final_response 不得覆盖 transcript final。",
			}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(
		NewAiopsTransportState(turn.SessionID, "thread-single-assistant-final"),
		turn,
	)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	if got.Final == nil || got.Final.ID != "assistant-final-1" || got.Final.Text != "来自 committed assistant message 的最终回答。" {
		t.Fatalf("turn.Final = %#v, want committed assistant_message as the only transcript final", got.Final)
	}
	var finalBlockIDs []string
	for _, id := range got.BlockOrder {
		if got.BlocksByID[id].Type == AiopsTransportBlockTypeFinalAnswer {
			finalBlockIDs = append(finalBlockIDs, id)
		}
	}
	if !reflect.DeepEqual(finalBlockIDs, []string{"assistant-final-1"}) {
		t.Fatalf("final block ids = %#v, want only assistant-final-1; blockOrder=%#v", finalBlockIDs, got.BlockOrder)
	}
	if _, exists := got.BlocksByID["final-response-1"]; exists {
		t.Fatalf("legacy final_response entered transcript blocks: %#v", got.BlocksByID["final-response-1"])
	}
}

func TestTransportProjectorProjectsFinalGenerationDuration(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	finalData := json.RawMessage(`{"messageId":"msg-final","displayKind":"assistant.message","phase":"final_answer","streamState":"complete","durationMs":456}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-duration",
		SessionID:   "session-1",
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(2 * time.Second),
		CompletedAt: ptrTransportProjectorTime(now.Add(2 * time.Second)),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "这是最终回答", Data: finalData}, CreatedAt: now, UpdatedAt: now.Add(456 * time.Millisecond)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	transportTurn := projected.Turns["turn-final-duration"]
	if transportTurn.Final == nil || transportTurn.Final.DurationMs != 456 {
		t.Fatalf("turn.Final = %+v, want durationMs=456", transportTurn.Final)
	}
	for _, block := range transportTurn.Process {
		if block.Kind == AiopsTransportProcessKindAssistant {
			t.Fatalf("final assistant_message must not duplicate into process: %#v", transportTurn.Process)
		}
	}
}

func TestTransportProjectorDoesNotProjectRunningAssistantMessageAsFinal(t *testing.T) {
	now := time.Date(2026, 5, 7, 15, 0, 0, 0, time.UTC)
	finalText := "第一段第二段完整流式输出"
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-streaming-final",
		SessionID:   "session-1",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleRunning,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-running", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{
				Summary: finalText,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"streaming","durationMs":456}`),
			}, CreatedAt: now},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-1", "thread-1"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-streaming-final"]
	if transportTurn.Final != nil {
		t.Fatalf("turn.Final = %+v, want running assistant_message excluded until terminal commit", transportTurn.Final)
	}
	if len(transportTurn.Process) != 0 || len(transportTurn.BlockOrder) != 0 || len(transportTurn.BlocksByID) != 0 {
		t.Fatalf("running assistant_message entered transcript: process=%#v blockOrder=%#v blocksById=%#v", transportTurn.Process, transportTurn.BlockOrder, transportTurn.BlocksByID)
	}
	if len(transportTurn.AgentItems) != 1 || transportTurn.AgentItems[0].ID != "final-running" {
		t.Fatalf("AgentItems = %#v, want running draft retained as canonical trace fact", transportTurn.AgentItems)
	}
}

func TestTransportProjectorHidesUnclassifiedAndReplacedAssistantMessages(t *testing.T) {
	now := time.Date(2026, 7, 16, 9, 30, 0, 0, time.UTC)
	tests := []struct {
		name    string
		itemID  string
		status  agentstate.ItemStatus
		payload json.RawMessage
	}{
		{
			name:    "unclassified streaming draft",
			itemID:  "assistant-unclassified-1",
			status:  agentstate.ItemStatusRunning,
			payload: json.RawMessage(`{"displayKind":"assistant.message","phase":"unclassified","streamState":"streaming"}`),
		},
		{
			name:    "replaced retry draft",
			itemID:  "assistant-replaced-1",
			status:  agentstate.ItemStatusFailed,
			payload: json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"incomplete","boundaryAction":"retry_once","replacedByMessageId":"assistant-final-2"}`),
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			turn := &runtimekernel.TurnSnapshot{
				ID:        "turn-" + test.itemID,
				SessionID: "session-hidden-assistant-drafts",
				Lifecycle: runtimekernel.TurnLifecycleRunning,
				StartedAt: now,
				UpdatedAt: now.Add(time.Second),
				AgentItems: []agentstate.TurnItem{{
					ID:     test.itemID,
					Type:   agentstate.TurnItemTypeAssistantMessage,
					Status: test.status,
					Payload: agentstate.PayloadEnvelope{
						Summary: "尚未确认的 assistant 草稿。",
						Data:    test.payload,
					},
					CreatedAt: now,
					UpdatedAt: now.Add(time.Second),
				}},
			}

			projected, err := NewTransportProjector().ProjectTurnSnapshot(
				NewAiopsTransportState(turn.SessionID, "thread-hidden-assistant-drafts"),
				turn,
			)
			if err != nil {
				t.Fatalf("ProjectTurnSnapshot() error = %v", err)
			}
			got := projected.Turns[turn.ID]
			if got.Final != nil || len(got.Process) != 0 || len(got.BlockOrder) != 0 || len(got.BlocksByID) != 0 {
				t.Fatalf("draft entered Chat transcript: final=%#v process=%#v blockOrder=%#v blocksById=%#v", got.Final, got.Process, got.BlockOrder, got.BlocksByID)
			}
			if len(got.AgentItems) != 1 || got.AgentItems[0].ID != test.itemID {
				t.Fatalf("AgentItems = %#v, want hidden draft retained for trace/replay", got.AgentItems)
			}
		})
	}
}

func TestTransportProjectorProjectsFinalContractStatusAndEvidence(t *testing.T) {
	now := time.Date(2026, 7, 2, 10, 0, 0, 0, time.UTC)
	finalText := "无法在目标主机执行命令：host agent 不可用。"
	finalData := json.RawMessage(`{
		"displayKind":"assistant.message",
		"phase":"final_answer",
		"streamState":"complete",
		"durationMs":321,
		"finalContract":{
			"schemaVersion":"aiops.harness.final.v1",
			"status":"tool_unavailable",
			"confidence":"low",
			"answerText":"无法在目标主机执行命令：host agent 不可用。",
			"checkedEvidenceRefs":["call-dns"],
			"uncheckedRequirements":["exec_command:needs_host_agent"],
			"failedToolImpacts":[{"toolName":"exec_command","toolCallId":"call-exec","failureClass":"needs_host_agent","impact":"host agent 7072 refused"}],
			"limitations":["host_agent_unavailable"]
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-contract",
		SessionID:   "session-final-contract",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "final-contract-1", Type: agentstate.TurnItemTypeFinalResponse, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: finalText,
				Data:    finalData,
			}, CreatedAt: now, UpdatedAt: now.Add(time.Second)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-final-contract", "thread-final-contract"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	final := projected.Turns["turn-final-contract"].Final
	if final == nil {
		t.Fatal("turn.Final is nil, want projected final contract")
	}
	if final.Status != AiopsTransportFinalStatusToolUnavailable {
		t.Fatalf("final status = %q, want tool_unavailable: %#v", final.Status, final)
	}
	if final.SchemaVersion != "aiops.harness.final.v1" || final.Confidence != "low" || final.DurationMs != 321 {
		t.Fatalf("final contract fields = %#v", final)
	}
	if !containsString(final.CheckedEvidenceRefs, "call-dns") || !containsString(final.UncheckedRequirements, "exec_command:needs_host_agent") {
		t.Fatalf("final evidence refs = checked=%#v unchecked=%#v", final.CheckedEvidenceRefs, final.UncheckedRequirements)
	}
	if len(final.FailedToolImpacts) != 1 || final.FailedToolImpacts[0].FailureClass != "needs_host_agent" {
		t.Fatalf("failedToolImpacts = %#v", final.FailedToolImpacts)
	}
}

func TestTransportProjectorTypedFactsOnlyFinalStatusIgnoresVerificationProse(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	project := func(status runtimekernel.FinalContractStatus, evidenceRefs []string) AiopsTransportFinal {
		t.Helper()
		data, err := json.Marshal(map[string]any{
			"displayKind": "assistant.message",
			"phase":       "final_answer",
			"streamState": "complete",
			"finalContract": runtimekernel.FinalContract{
				SchemaVersion:       runtimekernel.FinalContractSchemaVersion,
				Status:              status,
				AnswerText:          "所有检查均已完成。",
				CheckedEvidenceRefs: evidenceRefs,
			},
		})
		if err != nil {
			t.Fatalf("json.Marshal() error = %v", err)
		}
		turnID := "turn-typed-final-" + string(status)
		turn := &runtimekernel.TurnSnapshot{
			ID:        turnID,
			SessionID: "session-typed-final",
			Lifecycle: runtimekernel.TurnLifecycleCompleted,
			StartedAt: now,
			UpdatedAt: now,
			AgentItems: []agentstate.TurnItem{{
				ID:     "final-typed",
				Type:   agentstate.TurnItemTypeAssistantMessage,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: "已验证：所有检查均已完成，证据充分。",
					Data:    data,
				},
			}},
		}
		projected, err := NewTransportProjector().ProjectTurnSnapshot(
			NewAiopsTransportState("session-typed-final", "thread-typed-final"),
			turn,
		)
		if err != nil {
			t.Fatalf("ProjectTurnSnapshot() error = %v", err)
		}
		if projected.Turns[turnID].Final == nil {
			t.Fatal("turn.Final is nil")
		}
		return *projected.Turns[turnID].Final
	}

	verified := project(runtimekernel.FinalContractStatusVerified, []string{"evidence-verified"})
	needsEvidence := project(runtimekernel.FinalContractStatusNeedsEvidence, nil)
	if verified.Status != AiopsTransportFinalStatusVerified {
		t.Fatalf("verified final status = %q, want verified", verified.Status)
	}
	if needsEvidence.Status != AiopsTransportFinalStatusNeedsEvidence {
		t.Fatalf("needs-evidence final status = %q, want needs_evidence", needsEvidence.Status)
	}
}

func TestCanonicalTransportUnknownFinalContractIsCompletedBlock(t *testing.T) {
	turn := AiopsTransportTurn{
		ID:     "turn-unknown-final",
		Status: AiopsTransportTurnStatusCompleted,
		Final: &AiopsTransportFinal{
			ID:            "final-unknown",
			SchemaVersion: runtimekernel.FinalContractSchemaVersion,
			Status:        AiopsTransportFinalStatusUnknown,
			Text:          "只读说明已经完成。",
			AnswerText:    "只读说明已经完成。",
		},
	}
	order, blocks := projectCanonicalTransportBlocks(turn)
	if len(order) != 1 || order[0] != "final-unknown" {
		t.Fatalf("block order = %#v, want final-unknown", order)
	}
	block := blocks["final-unknown"]
	if block.Status != AiopsTransportProcessStatusCompleted || block.StreamState != "complete" {
		t.Fatalf("unknown final block status/stream = %q/%q, want completed/complete", block.Status, block.StreamState)
	}
	if block.FinalContract == nil || block.FinalContract.Status != AiopsTransportFinalStatusUnknown {
		t.Fatalf("final contract = %#v, want preserved unknown evidence status", block.FinalContract)
	}
}

func TestTransportProjectorNormalizesPersistedVerifiedContractInvariant(t *testing.T) {
	now := time.Date(2026, 7, 14, 10, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		contract   runtimekernel.FinalContract
		wantStatus AiopsTransportFinalStatus
	}{
		{
			name: "missing evidence is downgraded",
			contract: runtimekernel.FinalContract{
				SchemaVersion: runtimekernel.FinalContractSchemaVersion,
				Status:        runtimekernel.FinalContractStatusVerified,
			},
			wantStatus: AiopsTransportFinalStatusNeedsEvidence,
		},
		{
			name: "unchecked requirement is downgraded",
			contract: runtimekernel.FinalContract{
				SchemaVersion:         runtimekernel.FinalContractSchemaVersion,
				Status:                runtimekernel.FinalContractStatusVerified,
				CheckedEvidenceRefs:   []string{"evidence://health"},
				UncheckedRequirements: []string{"metrics:unavailable"},
			},
			wantStatus: AiopsTransportFinalStatusNeedsEvidence,
		},
		{
			name: "outstanding postcheck is downgraded",
			contract: runtimekernel.FinalContract{
				SchemaVersion:       runtimekernel.FinalContractSchemaVersion,
				Status:              runtimekernel.FinalContractStatusVerified,
				CheckedEvidenceRefs: []string{"evidence://precheck"},
				RequiredPostChecks:  []string{"service_health"},
			},
			wantStatus: AiopsTransportFinalStatusNeedsEvidence,
		},
		{
			name: "complete verified contract stays verified",
			contract: runtimekernel.FinalContract{
				SchemaVersion:       runtimekernel.FinalContractSchemaVersion,
				Status:              runtimekernel.FinalContractStatusVerified,
				CheckedEvidenceRefs: []string{"evidence://health"},
				PostChecks:          []string{"service_health"},
				RequiredPostChecks:  []string{"service_health"},
			},
			wantStatus: AiopsTransportFinalStatusVerified,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{
				"displayKind":   "assistant.message",
				"phase":         "final_answer",
				"streamState":   "complete",
				"finalContract": tt.contract,
			})
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			turn := &runtimekernel.TurnSnapshot{
				ID:        "turn-persisted-final-" + strings.ReplaceAll(tt.name, " ", "-"),
				SessionID: "session-persisted-final",
				Lifecycle: runtimekernel.TurnLifecycleCompleted,
				StartedAt: now,
				UpdatedAt: now,
				AgentItems: []agentstate.TurnItem{{
					ID:     "persisted-final",
					Type:   agentstate.TurnItemTypeAssistantMessage,
					Status: agentstate.ItemStatusCompleted,
					Payload: agentstate.PayloadEnvelope{
						Summary: "显示文本不得改变结构化状态。",
						Data:    data,
					},
				}},
			}
			projected, err := NewTransportProjector().ProjectTurnSnapshot(
				NewAiopsTransportState(turn.SessionID, "thread-persisted-final"),
				turn,
			)
			if err != nil {
				t.Fatalf("ProjectTurnSnapshot() error = %v", err)
			}
			final := projected.Turns[turn.ID].Final
			if final == nil || final.Status != tt.wantStatus {
				t.Fatalf("projected final = %#v, want status %q", final, tt.wantStatus)
			}
			if tt.wantStatus == AiopsTransportFinalStatusNeedsEvidence && !containsString(final.Limitations, "invalid_verified_contract_facts") {
				t.Fatalf("projected limitations = %#v, want invalid verified facts limitation", final.Limitations)
			}
		})
	}
}

func TestTransportProjectorTypedFactsOnlyMarkdownDoesNotChangeApprovalToolEvidenceStatus(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 10, 0, 0, time.UTC)
	contract := runtimekernel.FinalContract{
		SchemaVersion:       runtimekernel.FinalContractSchemaVersion,
		Status:              runtimekernel.FinalContractStatusPartial,
		Confidence:          "medium",
		AnswerText:          "结构化结论保持不变。",
		CheckedEvidenceRefs: []string{"evidence-typed-1"},
		ApprovedActions:     []string{"approval-typed-1"},
		PerformedActions:    []string{"tool-call-typed-1"},
		PostChecks:          []string{"postcheck-typed-1"},
	}
	data, err := json.Marshal(map[string]any{
		"displayKind":   "assistant.message",
		"phase":         "final_answer",
		"streamState":   "complete",
		"finalContract": contract,
	})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	project := func(turnID, markdown string) AiopsTransportState {
		t.Helper()
		turn := &runtimekernel.TurnSnapshot{
			ID:        turnID,
			SessionID: "session-markdown-independence",
			Lifecycle: runtimekernel.TurnLifecycleCompleted,
			StartedAt: now,
			UpdatedAt: now,
			Metadata: map[string]string{
				"aiops.coroot.explicitRCA": "true",
			},
			AgentItems: []agentstate.TurnItem{{
				ID:     "final-markdown-independent",
				Type:   agentstate.TurnItemTypeAssistantMessage,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: markdown,
					Data:    data,
				},
			}},
		}
		projected, projectErr := NewTransportProjector().ProjectTurnSnapshot(
			NewAiopsTransportState("session-markdown-independence", "thread-markdown-independence"),
			turn,
		)
		if projectErr != nil {
			t.Fatalf("ProjectTurnSnapshot() error = %v", projectErr)
		}
		return projected
	}

	rcaShapedMarkdown := `{"schemaVersion":"aiops.rca_report/v1","status":"partial","source":"coroot","evidenceRefs":["prose-only-evidence"],"conclusion":{"summaryZh":"文本伪造的 RCA"}}`
	plainMarkdown := "同一结构化事实下的普通最终回答。"
	rcaProjection := project("turn-markdown-rca-shaped", rcaShapedMarkdown)
	plainProjection := project("turn-markdown-plain", plainMarkdown)
	rcaTurn := rcaProjection.Turns["turn-markdown-rca-shaped"]
	plainTurn := plainProjection.Turns["turn-markdown-plain"]

	if len(rcaTurn.AgentUIArtifacts) != 0 || len(plainTurn.AgentUIArtifacts) != 0 {
		t.Fatalf("final markdown created artifacts: rca=%#v plain=%#v", rcaTurn.AgentUIArtifacts, plainTurn.AgentUIArtifacts)
	}
	if rcaTurn.Final == nil || plainTurn.Final == nil {
		t.Fatalf("projected finals missing: rca=%#v plain=%#v", rcaTurn.Final, plainTurn.Final)
	}
	if rcaTurn.Final.Status != plainTurn.Final.Status ||
		!reflect.DeepEqual(rcaTurn.Final.CheckedEvidenceRefs, plainTurn.Final.CheckedEvidenceRefs) ||
		!reflect.DeepEqual(rcaTurn.Final.ApprovedActions, plainTurn.Final.ApprovedActions) ||
		!reflect.DeepEqual(rcaTurn.Final.PerformedActions, plainTurn.Final.PerformedActions) ||
		!reflect.DeepEqual(rcaTurn.Final.PostChecks, plainTurn.Final.PostChecks) ||
		!reflect.DeepEqual(rcaTurn.Process, plainTurn.Process) ||
		!reflect.DeepEqual(rcaProjection.PendingApprovals, plainProjection.PendingApprovals) {
		t.Fatalf("markdown changed typed projection: rca=%#v plain=%#v", rcaTurn, plainTurn)
	}
}

func TestTransportProjectorTypedFactsOnlyApprovalDeniedComesFromFinalContract(t *testing.T) {
	now := time.Date(2026, 7, 14, 9, 20, 0, 0, time.UTC)
	for _, tc := range []struct {
		name           string
		contractStatus runtimekernel.FinalContractStatus
		errorText      string
		wantStatus     AiopsTransportProcessStatus
	}{
		{
			name:           "denial prose cannot override typed failed status",
			contractStatus: runtimekernel.FinalContractStatusFailed,
			errorText:      "approval denied",
			wantStatus:     AiopsTransportProcessStatusFailed,
		},
		{
			name:           "typed approval denial does not require denial prose",
			contractStatus: runtimekernel.FinalContractStatusApprovalDenied,
			errorText:      "runtime stopped",
			wantStatus:     AiopsTransportProcessStatusRejected,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			data, err := json.Marshal(map[string]any{
				"displayKind": "assistant.message",
				"phase":       "final_answer",
				"streamState": "complete",
				"finalContract": runtimekernel.FinalContract{
					SchemaVersion: runtimekernel.FinalContractSchemaVersion,
					Status:        tc.contractStatus,
					AnswerText:    "控制状态由结构化事实提供。",
				},
			})
			if err != nil {
				t.Fatalf("json.Marshal() error = %v", err)
			}
			turn := &runtimekernel.TurnSnapshot{
				ID:        "turn-typed-approval-" + string(tc.contractStatus),
				SessionID: "session-typed-approval",
				Lifecycle: runtimekernel.TurnLifecycleFailed,
				StartedAt: now,
				UpdatedAt: now,
				Error:     tc.errorText,
				AgentItems: []agentstate.TurnItem{
					{
						ID:     "approval-block",
						Type:   agentstate.TurnItemTypeApproval,
						Status: agentstate.ItemStatusBlocked,
						Payload: agentstate.PayloadEnvelope{
							Summary: "等待审批",
							Data:    json.RawMessage(`{"approvalId":"approval-typed","approvalType":"tool"}`),
						},
					},
					{
						ID:     "final-typed-approval",
						Type:   agentstate.TurnItemTypeAssistantMessage,
						Status: agentstate.ItemStatusFailed,
						Payload: agentstate.PayloadEnvelope{
							Summary: "审批控制结论。",
							Data:    data,
						},
					},
				},
			}
			projected, err := NewTransportProjector().ProjectTurnSnapshot(
				NewAiopsTransportState("session-typed-approval", "thread-typed-approval"),
				turn,
			)
			if err != nil {
				t.Fatalf("ProjectTurnSnapshot() error = %v", err)
			}
			block := findTransportProcessBlock(t, projected.Turns[turn.ID].Process, AiopsTransportProcessKindApproval)
			if block.Status != tc.wantStatus {
				t.Fatalf("approval block status = %q, want %q (contract=%q error=%q)", block.Status, tc.wantStatus, tc.contractStatus, tc.errorText)
			}
		})
	}
}

func TestTransportProjectorKeepsCompletedAndRequiredPostChecksDistinct(t *testing.T) {
	now := time.Date(2026, 7, 12, 14, 0, 0, 0, time.UTC)
	finalData := json.RawMessage(`{
		"displayKind":"assistant.message",
		"phase":"final_answer",
		"streamState":"complete",
		"finalContract":{
			"schemaVersion":"aiops.harness.final.v1",
			"status":"partial",
			"answerText":"变更已执行，仍需完成外部核验。",
			"postChecks":[" check-complete ","","check-complete","check-secondary"],
			"requiredPostChecks":["check-required"," check-required ","check-fallback"]
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-post-check-contract",
		SessionID: "session-post-check-contract",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{{
			ID:     "final-post-check-contract",
			Type:   agentstate.TurnItemTypeFinalResponse,
			Status: agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{
				Summary: "变更已执行，仍需完成外部核验。",
				Data:    finalData,
			},
		}},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(
		NewAiopsTransportState(turn.SessionID, "thread-post-check-contract"),
		turn,
	)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	final := projected.Turns[turn.ID].Final
	if final == nil {
		t.Fatal("turn.Final is nil, want projected final contract")
	}
	if want := []string{"check-complete", "check-secondary"}; !reflect.DeepEqual(final.PostChecks, want) {
		t.Fatalf("postChecks = %#v, want completed checks %#v", final.PostChecks, want)
	}
	if want := []string{"check-required", "check-fallback"}; !reflect.DeepEqual(final.RequiredPostChecks, want) {
		t.Fatalf("requiredPostChecks = %#v, want outstanding checks %#v", final.RequiredPostChecks, want)
	}

	turn.AgentItems[0].Payload.Data[0] = '['
	if !reflect.DeepEqual(final.RequiredPostChecks, []string{"check-required", "check-fallback"}) {
		t.Fatalf("requiredPostChecks changed after source payload mutation: %#v", final.RequiredPostChecks)
	}

	encoded, err := json.Marshal(final)
	if err != nil {
		t.Fatalf("json.Marshal(final) error = %v", err)
	}
	var roundTripped AiopsTransportFinal
	if err := json.Unmarshal(encoded, &roundTripped); err != nil {
		t.Fatalf("json.Unmarshal(final) error = %v", err)
	}
	if !reflect.DeepEqual(roundTripped, *final) {
		t.Fatalf("transport final JSON round trip changed post-check facts\nwant=%#v\n got=%#v", *final, roundTripped)
	}
}

func TestTransportProjectorProjectsIncompleteAssistantMessageFinalAndStreamError(t *testing.T) {
	now := time.Date(2026, 6, 25, 16, 12, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-incomplete-answer", "thread-incomplete-answer")
	answer := "根因：已经生成的分析草稿必须保留。"
	rawError := "failed to receive stream chunk: context deadline exceeded"
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-incomplete-answer",
		SessionID: "session-incomplete-answer",
		Lifecycle: runtimekernel.TurnLifecycleFailed,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		Error:     rawError,
		AgentItems: []agentstate.TurnItem{
			{ID: "answer-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{
				Summary: answer,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"incomplete","evidenceBoundary":"limited","iteration":0}`),
			}, CreatedAt: now.Add(time.Second)},
			{ID: "error-1", Type: agentstate.TurnItemTypeError, Status: agentstate.ItemStatusFailed, Payload: agentstate.PayloadEnvelope{Summary: rawError}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-incomplete-answer"]
	if transportTurn.Final == nil || transportTurn.Final.Text != answer || transportTurn.Final.Status != AiopsTransportFinalStatusFailed {
		t.Fatalf("turn.Final = %+v, want failed incomplete assistant_message final", transportTurn.Final)
	}
	errorBlock := findTransportProcessBlock(t, transportTurn.Process, AiopsTransportProcessKindSystem)
	if errorBlock.DisplayKind != "runtime.error" || errorBlock.Text != "模型流中断，已保留已生成内容" || errorBlock.Status != AiopsTransportProcessStatusFailed {
		t.Fatalf("error block = %#v, want friendly runtime error status block", errorBlock)
	}
	if strings.Contains(transportProjectionText(projected), rawError) {
		t.Fatalf("projection leaked raw stream error:\n%s", transportProjectionText(projected))
	}
}

func TestTransportProjectorReordersProcessFromLatestAgentItems(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	searchData := json.RawMessage(`{
		"toolCallId":"search-1",
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"BTC current price USD 24h change"
	}`)
	firstSnapshot := &runtimekernel.TurnSnapshot{
		ID:        "turn-process-order",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "search-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "BTC current price USD 24h change", Data: searchData}, CreatedAt: now.Add(time.Second)},
		},
	}
	projected, err := projector.ProjectTurnSnapshot(state, firstSnapshot)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot(first) error = %v", err)
	}
	if len(projected.Turns["turn-process-order"].Process) != 1 || projected.Turns["turn-process-order"].Process[0].Kind != AiopsTransportProcessKindSearch {
		t.Fatalf("first process = %#v, want only search", projected.Turns["turn-process-order"].Process)
	}

	secondSnapshot := *firstSnapshot
	secondSnapshot.UpdatedAt = now.Add(2 * time.Second)
	secondSnapshot.AgentItems = []agentstate.TurnItem{
		{ID: "assistant-commentary-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
			Summary: "我将先用实时网页搜索获取当前BTC价格、24小时涨跌与主要来源报价，并据此给你一个简明行情摘要。",
			Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete"}`),
		}, CreatedAt: now.Add(500 * time.Millisecond)},
		{ID: "search-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "BTC current price USD 24h change", Data: searchData}, CreatedAt: now.Add(time.Second)},
	}
	projected, err = projector.ProjectTurnSnapshot(projected, &secondSnapshot)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot(second) error = %v", err)
	}

	process := projected.Turns["turn-process-order"].Process
	if len(process) != 2 {
		t.Fatalf("len(process) = %d, want assistant and search: %#v", len(process), process)
	}
	if process[0].Kind != AiopsTransportProcessKindAssistant || process[0].Text == "" {
		t.Fatalf("process[0] = %+v, want assistant prelude", process[0])
	}
	if process[0].DisplayKind != "assistant.message" || process[0].Phase != "commentary" {
		t.Fatalf("process[0] = %+v, want assistant_message commentary", process[0])
	}
	if process[1].Kind != AiopsTransportProcessKindSearch {
		t.Fatalf("process[1] = %+v, want search after assistant", process[1])
	}

	thirdSnapshot := secondSnapshot
	thirdSnapshot.Lifecycle = runtimekernel.TurnLifecycleCompleted
	thirdSnapshot.UpdatedAt = now.Add(3 * time.Second)
	thirdSnapshot.AgentItems = append(thirdSnapshot.AgentItems, agentstate.TurnItem{
		ID: "final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
			Summary: "最终行情摘要。",
			Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
		}, CreatedAt: now.Add(3 * time.Second),
	})
	projected, err = projector.ProjectTurnSnapshot(projected, &thirdSnapshot)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot(third) error = %v", err)
	}
	process = projected.Turns["turn-process-order"].Process
	if len(process) != 2 {
		t.Fatalf("len(process) = %d, want assistant commentary and search: %#v", len(process), process)
	}
	if process[0].DisplayKind != "assistant.message" || process[0].Phase != "commentary" || !strings.Contains(process[0].Text, "实时网页搜索") {
		t.Fatalf("process[0] = %+v, want retained assistant commentary after final output", process[0])
	}
	if final := projected.Turns["turn-process-order"].Final; final == nil || final.Text != "最终行情摘要。" {
		t.Fatalf("Final = %+v, want final answer", final)
	}
}

func TestTransportProjectorProjectsAssistantCommentaryMetadataBeforeTool(t *testing.T) {
	now := time.Date(2026, 6, 27, 10, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-commentary-metadata",
		SessionID: "session-commentary-metadata",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "assistant-commentary-0", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "我会先执行只读命令获取证据，再根据输出整理回答。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete","commentarySource":"runtime_tool_intent","toolCallIds":["call-cpu"]}`),
			}, CreatedAt: now.Add(time.Second)},
			{ID: "turn-commentary-metadata-tool-call-call-cpu", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "exec_command",
				Data:    json.RawMessage(`{"toolCallId":"call-cpu","toolName":"exec_command","displayKind":"terminal.command","inputSummary":"top -l 1"}`),
			}, CreatedAt: now.Add(2 * time.Second)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-commentary-metadata", "thread-commentary-metadata"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	process := projected.Turns["turn-commentary-metadata"].Process
	if len(process) < 2 {
		t.Fatalf("process = %#v, want commentary before tool", process)
	}
	if process[0].Kind != AiopsTransportProcessKindAssistant || process[0].Phase != "commentary" {
		t.Fatalf("process[0] = %+v, want assistant commentary", process[0])
	}
	if process[0].CommentarySource != "runtime_tool_intent" {
		t.Fatalf("commentarySource = %q, want runtime_tool_intent", process[0].CommentarySource)
	}
	if len(process[0].ToolCallIDs) != 1 || process[0].ToolCallIDs[0] != "call-cpu" {
		t.Fatalf("toolCallIDs = %#v, want call-cpu", process[0].ToolCallIDs)
	}
	if process[1].Kind == AiopsTransportProcessKindAssistant {
		t.Fatalf("process = %#v, final/tool order broken", process)
	}
}

func TestTransportProjectorPreservesLedgerInterleavingWithApprovalAndFinalResponse(t *testing.T) {
	now := time.Date(2026, 7, 2, 9, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-ledger-interleave",
		SessionID: "session-ledger-interleave",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(9 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "assistant-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "先检查 agent 端口。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
			{ID: "tool-call-a", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "exec_command",
				Data:    json.RawMessage(`{"toolCallId":"call-a","toolName":"exec_command","displayKind":"terminal.command","inputSummary":"ss -tlnp"}`),
			}, CreatedAt: now.Add(2 * time.Second)},
			{ID: "tool-result-a", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "ss unavailable",
				Data:    json.RawMessage(`{"toolCallId":"call-a","toolName":"exec_command","displayKind":"terminal.command","inputSummary":"ss -tlnp","outputSummary":"ss unavailable"}`),
			}, CreatedAt: now.Add(3 * time.Second)},
			{ID: "assistant-2", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "换用 /proc 继续查。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete"}`),
			}, CreatedAt: now.Add(4 * time.Second)},
			{ID: "tool-call-b", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "exec_command",
				Data:    json.RawMessage(`{"toolCallId":"call-b","toolName":"exec_command","displayKind":"terminal.command","inputSummary":"cat /proc/net/tcp"}`),
			}, CreatedAt: now.Add(5 * time.Second)},
			{ID: "tool-result-b", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "tcp rows",
				Data:    json.RawMessage(`{"toolCallId":"call-b","toolName":"exec_command","displayKind":"terminal.command","inputSummary":"cat /proc/net/tcp","outputSummary":"tcp rows"}`),
			}, CreatedAt: now.Add(6 * time.Second)},
			{ID: "approval-requested-1", Type: agentstate.TurnItemTypeApprovalRequested, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{
				Summary: "需要审批 restart",
				Data:    json.RawMessage(`{"approvalId":"approval-1","approvalType":"command","command":"systemctl restart aiops-host-agent","reason":"需要重启 agent"}`),
			}, CreatedAt: now.Add(7 * time.Second)},
			{ID: "approval-decided-1", Type: agentstate.TurnItemTypeApprovalDecided, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "审批通过",
				Data:    json.RawMessage(`{"approvalId":"approval-1","approvalType":"command","command":"systemctl restart aiops-host-agent","reason":"用户批准"}`),
			}, CreatedAt: now.Add(8 * time.Second)},
			{ID: "final-response-1", Type: agentstate.TurnItemTypeFinalResponse, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "最终结论。",
			}, CreatedAt: now.Add(9 * time.Second)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(NewAiopsTransportState("session-ledger-interleave", "thread-ledger-interleave"), turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	process := projected.Turns["turn-ledger-interleave"].Process
	if len(process) != 5 {
		t.Fatalf("process len = %d, want assistant/tool/assistant/tool/approval: %#v", len(process), process)
	}
	if process[0].Kind != AiopsTransportProcessKindAssistant || !strings.Contains(process[0].Text, "先检查") {
		t.Fatalf("process[0] = %+v", process[0])
	}
	if process[1].Kind != AiopsTransportProcessKindCommand || process[1].ToolCallID != "call-a" {
		t.Fatalf("process[1] = %+v", process[1])
	}
	if process[2].Kind != AiopsTransportProcessKindAssistant || !strings.Contains(process[2].Text, "换用") {
		t.Fatalf("process[2] = %+v", process[2])
	}
	if process[3].Kind != AiopsTransportProcessKindCommand || process[3].ToolCallID != "call-b" {
		t.Fatalf("process[3] = %+v", process[3])
	}
	if process[4].Kind != AiopsTransportProcessKindApproval {
		t.Fatalf("process[4] = %+v, want approval after second tool", process[4])
	}
	if final := projected.Turns["turn-ledger-interleave"].Final; final == nil || final.Text != "最终结论。" {
		t.Fatalf("final = %+v", final)
	}
}

func TestTransportProjectorPreservesCommentaryToolArtifactFinalOrder(t *testing.T) {
	now := time.Date(2026, 7, 16, 10, 0, 0, 0, time.UTC)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-commentary-tool-artifact-final",
		SessionID: "session-commentary-tool-artifact-final",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(4 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "commentary-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "先检查执行前置条件。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"commentary","streamState":"complete"}`),
			}, CreatedAt: now.Add(time.Second)},
			{ID: "tool-call-preflight-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{
				Kind:    "tool",
				Summary: "运行预检",
				Data:    json.RawMessage(`{"toolCallId":"call-preflight-1","toolName":"run_ops_manual_preflight","displayKind":"ops_manual_preflight_result","inputSummary":"检查执行前置条件"}`),
			}, CreatedAt: now.Add(2 * time.Second)},
			{ID: "tool-result-preflight-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Kind:    "tool",
				Summary: "预检通过",
				Data:    json.RawMessage(`{"toolCallId":"call-preflight-1","toolName":"run_ops_manual_preflight","displayKind":"ops_manual_preflight_result","inputSummary":"检查执行前置条件","outputPreview":{"status":"passed","next_action":"confirm_execution"}}`),
			}, CreatedAt: now.Add(3 * time.Second)},
			{ID: "assistant-final-1", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: "预检已通过。",
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now.Add(4 * time.Second)},
		},
	}

	projected, err := NewTransportProjector().ProjectTurnSnapshot(
		NewAiopsTransportState(turn.SessionID, "thread-commentary-tool-artifact-final"),
		turn,
	)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns[turn.ID]
	if len(got.AgentUIArtifacts) != 1 || got.AgentUIArtifacts[0].Type != "ops_manual_preflight_result" {
		t.Fatalf("AgentUIArtifacts = %#v, want one preflight artifact", got.AgentUIArtifacts)
	}
	wantOrder := []string{
		TransportProcessBlockStableID(turn.ID, string(AiopsTransportProcessKindAssistant), "commentary-1"),
		TransportProcessBlockStableID(turn.ID, string(AiopsTransportProcessKindTool), "call-preflight-1"),
		"ops-manual-preflight:" + turn.ID + ":tool-result-preflight-1",
		"assistant-final-1",
	}
	if !reflect.DeepEqual(got.BlockOrder, wantOrder) {
		t.Fatalf("BlockOrder = %#v, want commentary -> tool -> artifact -> final %#v", got.BlockOrder, wantOrder)
	}
	wantTypes := []AiopsTransportBlockType{
		AiopsTransportBlockTypeCommentary,
		AiopsTransportBlockType(AiopsTransportProcessKindTool),
		AiopsTransportBlockTypeArtifact,
		AiopsTransportBlockTypeFinalAnswer,
	}
	for index, id := range wantOrder {
		if got.BlocksByID[id].Type != wantTypes[index] {
			t.Fatalf("BlocksByID[%q].Type = %q, want %q", id, got.BlocksByID[id].Type, wantTypes[index])
		}
	}
}

func TestTransportProjectorRunningModelCallDoesNotEnterChatBlocks(t *testing.T) {
	now := time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-model-waiting",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "calling model"}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns["turn-model-waiting"]
	if len(got.Process) != 0 || len(got.BlockOrder) != 0 || len(got.BlocksByID) != 0 {
		t.Fatalf("model_call entered Chat blocks: process=%#v blockOrder=%#v blocksById=%#v", got.Process, got.BlockOrder, got.BlocksByID)
	}
	if len(got.AgentItems) != 1 || got.AgentItems[0].ID != "model-1" || got.AgentItems[0].Type != string(agentstate.TurnItemTypeModelCall) {
		t.Fatalf("AgentItems = %#v, want model_call retained outside Chat transcript", got.AgentItems)
	}
}

func TestTransportProjectorHidesCompletedModelCallPlaceholder(t *testing.T) {
	now := time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-model-completed",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "model response received"}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if process := projected.Turns["turn-model-completed"].Process; len(process) != 0 {
		t.Fatalf("process = %#v, want completed model placeholder hidden", process)
	}
}

func TestTransportProjectorCanceledModelCallDoesNotEnterChatBlocks(t *testing.T) {
	now := time.Date(2026, 6, 25, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-model-canceled",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleCanceled,
		Error:     "user stop",
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "calling model"}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	got := projected.Turns["turn-model-canceled"]
	if len(got.Process) != 0 || len(got.BlockOrder) != 0 || len(got.BlocksByID) != 0 {
		t.Fatalf("canceled model_call entered Chat blocks: process=%#v blockOrder=%#v blocksById=%#v", got.Process, got.BlockOrder, got.BlocksByID)
	}
	if got.Status != AiopsTransportTurnStatusCanceled {
		t.Fatalf("turn status = %q, want canceled without a model wait transcript block", got.Status)
	}
	if len(got.AgentItems) != 1 || got.AgentItems[0].ID != "model-1" || got.AgentItems[0].Type != string(agentstate.TurnItemTypeModelCall) {
		t.Fatalf("AgentItems = %#v, want canceled model_call retained outside Chat transcript", got.AgentItems)
	}
}

func TestTransportProjectorDedupesProviderNativeWebSearchBlocks(t *testing.T) {
	now := time.Date(2026, 5, 7, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	searchCallData := json.RawMessage(`{
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"2026-05-07 A股 行情"
	}`)
	searchResultData := json.RawMessage(`{
		"toolName":"web_search",
		"displayKind":"browser.search",
		"inputSummary":"2026-05-07 A股 行情",
		"outputSummary":"{\"content\":\"provider-native web_search completed for query \\\"2026-05-07 A股 行情\\\"; provider returned no textual summary and public fallback found no relevant result.\"}"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-search",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "search-call-1", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "2026-05-07 A股 行情", Data: searchCallData}, CreatedAt: now},
			{ID: "search-result-1", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "2026-05-07 A股 行情", Data: searchResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	process := projected.Turns["turn-search"].Process
	if len(process) != 1 {
		t.Fatalf("len(process) = %d, want 1 deduped search block: %#v", len(process), process)
	}
	block := process[0]
	if block.DisplayKind != "web_search" {
		t.Fatalf("DisplayKind = %q, want web_search", block.DisplayKind)
	}
	if block.Text != "2026-05-07 A股 行情" || block.OutputPreview != "2026-05-07 A股 行情" {
		t.Fatalf("block text/output = %q/%q, want cleaned query", block.Text, block.OutputPreview)
	}
	if block.Status != AiopsTransportProcessStatusCompleted {
		t.Fatalf("Status = %q, want completed", block.Status)
	}
	if block.Operation != "search" {
		t.Fatalf("Operation = %q, want search", block.Operation)
	}
}

func TestTransportProjectorExtractsRuntimeToolCallQueryAndMergesSearchResult(t *testing.T) {
	now := time.Date(2026, 5, 7, 12, 31, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	query := "2026-05-07 BTC price now 24h change market cap official exchange or market data"
	searchCallData := json.RawMessage(`{
		"id":"call-search-1",
		"name":"web_search",
		"arguments":{"query":"` + query + `"}
	}`)
	searchResultSummary := `{"content":"provider-native web_search completed for query \"` + query + `\"; provider returned no textual summary and public fallback found no relevant result. Do not repeat this exact query; refine with more specific entities, dates, or authoritative domains, or answer with explicit limitations if evidence is sufficient."}`
	searchResultData := json.RawMessage(`{
		"toolCallId":"call-search-1",
		"toolName":"web_search"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-runtime-search",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:        "turn-runtime-search-tool-call",
				Type:      agentstate.TurnItemTypeToolCall,
				Status:    agentstate.ItemStatusRunning,
				Payload:   agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "web_search", Data: searchCallData},
				CreatedAt: now,
			},
			{
				ID:        "turn-runtime-search-tool-result",
				Type:      agentstate.TurnItemTypeToolResult,
				Status:    agentstate.ItemStatusCompleted,
				Payload:   agentstate.PayloadEnvelope{Kind: "browser.search", Summary: searchResultSummary, Data: searchResultData},
				CreatedAt: now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	process := projected.Turns["turn-runtime-search"].Process
	if len(process) != 1 {
		t.Fatalf("len(process) = %d, want 1 merged search block: %#v", len(process), process)
	}
	block := process[0]
	if block.Kind != AiopsTransportProcessKindSearch {
		t.Fatalf("Kind = %q, want search", block.Kind)
	}
	if block.Status != AiopsTransportProcessStatusCompleted {
		t.Fatalf("Status = %q, want completed", block.Status)
	}
	if block.InputSummary != query {
		t.Fatalf("InputSummary = %q, want %q", block.InputSummary, query)
	}
	if len(block.Queries) != 1 || block.Queries[0] != query {
		t.Fatalf("Queries = %#v, want [%q]", block.Queries, query)
	}
	if block.Text != query {
		t.Fatalf("Text = %q, want %q", block.Text, query)
	}
	if block.Operation != "search" {
		t.Fatalf("Operation = %q, want search", block.Operation)
	}
}

func TestDecodeTransportSearchResultsUsesStructuredResultsWithFetchedStatus(t *testing.T) {
	raw := json.RawMessage(`{
		"operation":"search",
		"query":"postgres docs",
		"results":[
			{"title":"PostgreSQL docs","url":"https://www.postgresql.org/docs/current/","snippet":"docs","text":"bounded text","fetched":true},
			{"title":"Blog","url":"https://example.com/post","snippet":"ignored extra domain","fetchError":"blocked"}
		],
		"meta":{"backend":"lightweight_search+internal_fetch","fetchedCount":1}
	}`)
	results := decodeTransportSearchResults(raw)
	if len(results) != 2 || results[0].Title != "PostgreSQL docs" {
		t.Fatalf("results = %#v", results)
	}
	if !results[0].Fetched || results[0].Text != "bounded text" {
		t.Fatalf("first result = %#v, want fetched text preserved", results[0])
	}
	if results[1].FetchError == "" {
		t.Fatalf("second result = %#v, want fetch error preserved", results[1])
	}
}

func TestTransportProjectorSummarizesWebSearchOpenByURL(t *testing.T) {
	now := time.Date(2026, 6, 29, 11, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	openResult := `{"operation":"open","url":"https://www.postgresql.org/docs/current/","source":"custom_public_web:open","content":"Readable docs","results":[{"title":"PostgreSQL docs","url":"https://www.postgresql.org/docs/current/","snippet":"Readable docs","text":"Readable docs","fetched":true}],"meta":{"backend":"internal_fetch","fetchedCount":1}}`
	callData := json.RawMessage(`{
		"id":"call-open-1",
		"name":"web_search",
		"arguments":{"operation":"open","url":"https://www.postgresql.org/docs/current/"}
	}`)
	resultData := json.RawMessage(`{
		"toolCallId":"call-open-1",
		"toolName":"web_search",
		"outputPreview":` + strconv.Quote(openResult) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-open-url",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "open-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "tool_call", Summary: "web_search", Data: callData}, CreatedAt: now},
			{ID: "open-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool_result", Summary: openResult, Data: resultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	process := projected.Turns["turn-open-url"].Process
	if len(process) != 1 {
		t.Fatalf("len(process) = %d, want one merged search block: %#v", len(process), process)
	}
	block := process[0]
	if block.Kind != AiopsTransportProcessKindSearch || block.DisplayKind != "web_search" {
		t.Fatalf("block = %#v, want web_search block", block)
	}
	if !strings.Contains(block.Text, "postgresql.org/docs") {
		t.Fatalf("Text = %q, want opened URL summary", block.Text)
	}
	if len(block.Results) != 1 || !block.Results[0].Fetched {
		t.Fatalf("Results = %#v, want fetched structured result", block.Results)
	}
	if block.Operation != "open" || block.URL != "https://www.postgresql.org/docs/current/" {
		t.Fatalf("operation/url = %q/%q, want open URL", block.Operation, block.URL)
	}
	if block.Adapter != "custom_public_web:open" || block.Backend != "internal_fetch" || block.SourceCount != 1 {
		t.Fatalf("adapter/backend/sourceCount = %q/%q/%d", block.Adapter, block.Backend, block.SourceCount)
	}
}

func TestTransportProjectorPreservesSearchQueryWhenCompletedResultIsSparse(t *testing.T) {
	now := time.Date(2026, 6, 29, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-1", "thread-1")
	query := "pg_autoctl standby timeline higher than primary official docs"
	callData := json.RawMessage(`{
		"id":"call-search-sparse",
		"name":"web_search",
		"arguments":{"operation":"search","query":"` + query + `"}
	}`)
	resultData := json.RawMessage(`{
		"toolCallId":"call-search-sparse",
		"toolName":"web_search",
		"outputSummary":"web_search completed"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-sparse-search",
		SessionID: "session-1",
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "sparse-call", Type: agentstate.TurnItemTypeToolCall, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "web_search", Data: callData}, CreatedAt: now},
			{ID: "sparse-result", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "browser.search", Summary: "web_search completed", Data: resultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	process := projected.Turns["turn-sparse-search"].Process
	if len(process) != 1 {
		t.Fatalf("len(process) = %d, want one merged search block: %#v", len(process), process)
	}
	block := process[0]
	if block.Text != query || block.InputSummary != query {
		t.Fatalf("text/inputSummary = %q/%q, want preserved query %q", block.Text, block.InputSummary, query)
	}
	if len(block.Queries) != 1 || block.Queries[0] != query {
		t.Fatalf("Queries = %#v, want preserved query", block.Queries)
	}
	if block.Operation != "search" {
		t.Fatalf("Operation = %q, want search", block.Operation)
	}
}

func TestTransportProjectorRendersOpsManualPreflightArtifact(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 20, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-preflight", "thread-preflight")
	preflightPayload, _ := json.Marshal(map[string]any{
		"status":      "blocked",
		"ready":       false,
		"manual_id":   "manual-redis-rca",
		"workflow_id": "workflow-redis-rca",
		"reason":      "preflight probe permission is missing",
		"next_action": "request_permission",
		"missing_permissions": []string{
			"redis-readonly-probe",
		},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-preflight",
		"toolName":"run_ops_manual_preflight",
		"displayKind":"ops_manual_preflight_result",
		"outputPreview":` + string(preflightPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-preflight",
		SessionID: "session-preflight",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-preflight",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_preflight_result",
					Summary: "blocked",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := projected.Turns["turn-preflight"].AgentUIArtifacts
	if len(artifacts) != 1 || artifacts[0].Type != "ops_manual_preflight_result" {
		t.Fatalf("artifacts = %#v, want one ops_manual_preflight_result", artifacts)
	}
	if artifacts[0].Status != "blocked" || artifacts[0].Severity != "warning" {
		t.Fatalf("artifact = %#v, want blocked warning", artifacts[0])
	}
	if len(artifacts[0].Actions) != 1 || artifacts[0].Actions[0]["id"] != "request_permission" {
		t.Fatalf("actions = %#v, want request_permission", artifacts[0].Actions)
	}
}

func TestTransportProjectorRendersOpsManualPreflightPassedConfirmationAction(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 20, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-preflight-passed", "thread-preflight-passed")
	preflightPayload, _ := json.Marshal(map[string]any{
		"status":      "passed",
		"ready":       true,
		"manual_id":   "manual-redis-rca",
		"workflow_id": "workflow-redis-rca",
		"next_action": "confirm_execution",
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-preflight-passed",
		"toolName":"run_ops_manual_preflight",
		"displayKind":"ops_manual_preflight_result",
		"outputPreview":` + string(preflightPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-preflight-passed",
		SessionID: "session-preflight-passed",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-preflight-passed",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_preflight_result",
					Summary: "passed",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := projected.Turns["turn-preflight-passed"].AgentUIArtifacts
	if len(artifacts) != 1 || artifacts[0].Type != "ops_manual_preflight_result" {
		t.Fatalf("artifacts = %#v, want one ops_manual_preflight_result", artifacts)
	}
	if actions := artifacts[0].Actions; len(actions) != 1 || actions[0]["id"] != "confirm_execution" {
		t.Fatalf("preflight passed actions = %#v, want confirm_execution", actions)
	}
}

func TestTransportProjectorRendersOpsManualParamResolutionArtifact(t *testing.T) {
	now := time.Date(2026, 5, 17, 9, 20, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-param", "thread-param")
	payload, _ := json.Marshal(map[string]any{
		"status":      "ambiguous",
		"manual_id":   "manual-redis-rca",
		"workflow_id": "workflow-redis-rca",
		"fields": []map[string]any{{
			"id":         "target_instance",
			"label":      "Redis 实例",
			"type":       "resource_ref",
			"ui_control": "select",
		}},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-param-resolution",
		"toolName":"resolve_ops_manual_params",
		"displayKind":"ops_manual_param_resolution",
		"outputPreview":` + string(payload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-param",
		SessionID: "session-param",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-param",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_param_resolution",
					Summary: "ambiguous",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatal(err)
	}
	artifacts := projected.Turns["turn-param"].AgentUIArtifacts
	if len(artifacts) != 1 || artifacts[0].Type != "ops_manual_param_resolution" {
		t.Fatalf("artifacts = %#v, want one param resolution artifact", artifacts)
	}
	if artifacts[0].Status != "ambiguous" || artifacts[0].Severity != "warning" {
		t.Fatalf("artifact = %#v, want ambiguous warning", artifacts[0])
	}
	if len(artifacts[0].Actions) != 1 || artifacts[0].Actions[0]["id"] != "fill_params" {
		t.Fatalf("actions = %#v, want fill_params", artifacts[0].Actions)
	}
}

func TestTransportProjectorProjectsRCAReportArtifact(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-rca", "thread-rca")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-artifact-1",
		"toolName":"aiops.ui_artifact_emit",
		"displayKind":"rca_report",
		"outputPreview":{
			"schemaVersion":"aiops.agent_ui_artifact/v1",
			"type":"rca_report",
			"titleZh":"checkout 根因分析",
			"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。",
			"status":"ok",
			"severity":"high",
			"source":"coroot",
			"evidenceRef":"ev-coroot-latency",
			"permissionScope":"read",
			"redactionStatus":"redacted",
			"inlineData":{
				"schemaVersion":"aiops.rca_report/v1",
				"source":"coroot",
				"status":"ok",
				"target":{"service":"checkout"},
				"window":{"timeRange":"30m"},
				"conclusion":{"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。","confidence":0.72},
				"hypotheses":[],
				"sections":[],
				"evidenceRefs":["ev-coroot-latency"],
				"rawRefs":[]
			}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-rca",
		SessionID:   "session-rca",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]string{
			"aiops.coroot.rcaDisplayAllowed": "true",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-rca", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "RCA report", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-rca"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "rca_report" || artifact.Source != "coroot" || artifact.PermissionScope != "read" {
		t.Fatalf("artifact = %+v, want rca_report from coroot", artifact)
	}
	if artifact.InlineData == nil {
		t.Fatal("artifact inline data is nil")
	}
	if artifact.Metadata["evidenceRef"] != "ev-coroot-latency" {
		t.Fatalf("artifact metadata = %#v, want evidenceRef copied into metadata", artifact.Metadata)
	}
}

func TestTransportProjectorProjectsUnavailableRCAReportAsSkipped(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-rca-unavailable", "thread-rca-unavailable")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-artifact-unavailable",
		"toolName":"aiops.ui_artifact_emit",
		"displayKind":"rca_report",
		"outputPreview":{
			"schemaVersion":"aiops.agent_ui_artifact/v1",
			"type":"rca_report",
			"titleZh":"checkout 根因分析",
			"summaryZh":"coroot_mcp_unavailable",
			"status":"unavailable",
			"severity":"info",
			"source":"coroot",
			"permissionScope":"read",
			"inlineData":{
				"schemaVersion":"aiops.rca_report/v1",
				"source":"coroot",
				"status":"unavailable",
				"summaryZh":"Coroot MCP 当前不可用"
			}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-rca-unavailable",
		SessionID:   "session-rca-unavailable",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		Metadata: map[string]string{
			"aiops.coroot.rcaDisplayAllowed": "true",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-rca-unavailable", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "RCA unavailable", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-rca-unavailable"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Status != "skipped" {
		t.Fatalf("artifact status = %q, want skipped: %+v", artifact.Status, artifact)
	}
	if artifact.Metadata["skipReason"] == "" || artifact.InlineData["skipReason"] == "" {
		t.Fatalf("artifact = %+v, want skip reason in metadata and inline data", artifact)
	}
}

func TestTransportProjectorSuppressesRCAReportArtifactWithoutExplicitCorootGate(t *testing.T) {
	now := time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-rca-suppressed", "thread-rca-suppressed")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-artifact-1",
		"toolName":"aiops.ui_artifact_emit",
		"displayKind":"rca_report",
		"outputPreview":{
			"schemaVersion":"aiops.agent_ui_artifact/v1",
			"type":"rca_report",
			"titleZh":"checkout 根因分析",
			"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。",
			"status":"ok",
			"source":"coroot",
			"inlineData":{
				"schemaVersion":"aiops.rca_report/v1",
				"source":"coroot",
				"status":"ok",
				"conclusion":{"summaryZh":"checkout 延迟升高最可能来自 catalog 依赖。","confidence":0.72},
				"evidenceRefs":["ev-coroot-latency"],
				"rawRefs":[]
			}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-rca-suppressed",
		SessionID:   "session-rca-suppressed",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-rca", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "RCA report", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if artifacts := projected.Turns["turn-rca-suppressed"].AgentUIArtifacts; len(artifacts) != 0 {
		t.Fatalf("AgentUIArtifacts = %#v, want no RCA artifact without explicit @Coroot gate", artifacts)
	}
}

func TestTransportProjectorProjectsRunnerWorkflowGenerationArtifact(t *testing.T) {
	now := time.Date(2026, 5, 25, 10, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-workflowgen", "thread-workflowgen")
	toolResultData := json.RawMessage(`{
		"toolCallId":"workflow-generation-plan",
		"toolName":"workflow_generation.plan",
		"displayKind":"runner_workflow_generation",
		"outputPreview":{
			"schemaVersion":"aiops.runner_workflow_generation/v1",
			"workflowTitle":"AI 新闻摘要工作流",
			"workflowId":"wfgen-1",
			"status":"plan_ready",
			"steps":[
				{"id":"search-news","title":"搜索 AI 新闻","status":"waiting"},
				{"id":"extract-key-news","title":"提取关键新闻","status":"waiting"}
			]
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-workflowgen",
		SessionID:   "session-workflowgen",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-workflowgen", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "workflow generation plan", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-workflowgen"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "runner_workflow_generation" || artifact.TitleZh != "Runner Workflow 生成进度" {
		t.Fatalf("artifact = %+v, want runner workflow generation artifact", artifact)
	}
	if artifact.InlineData["workflowTitle"] != "AI 新闻摘要工作流" {
		t.Fatalf("inlineData = %#v", artifact.InlineData)
	}
}

func TestTransportProjectorProjectsWorkflowEditorToolResultCards(t *testing.T) {
	now := time.Date(2026, 7, 6, 11, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-workflow-editor", "thread-workflow-editor")
	workflowPayload, _ := json.Marshal(map[string]any{
		"status":        "changed",
		"summary":       "Patch applied to Redis restart workflow.",
		"workflow_id":   "workflow-redis",
		"patch_id":      "patch-1",
		"effect_status": "changed",
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-workflow",
		"toolName":"workflow.apply_patch",
		"displayKind":"workflow_patch_result",
		"outputPreview":` + string(workflowPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-workflow-editor",
		SessionID:   "session-workflow-editor",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-workflow-editor",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "workflow_patch_result",
					Summary: "workflow patch applied",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-workflow-editor"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1: %#v", len(artifacts), artifacts)
	}
	artifact := artifacts[0]
	if artifact.Type != "workflow_patch_result" || artifact.Source != "workflow_editor" || artifact.PermissionScope != "draft" {
		t.Fatalf("artifact = %+v, want workflow editor patch result artifact", artifact)
	}
	if artifact.InlineData["patch_id"] != "patch-1" || artifact.InlineData["effect_status"] != "changed" {
		t.Fatalf("inlineData = %#v, want patch and effect status", artifact.InlineData)
	}
}

func TestTransportProjectorDoesNotCreateWorkflowSuccessCardFromFinalText(t *testing.T) {
	now := time.Date(2026, 7, 6, 11, 35, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-workflow-final-only", "thread-workflow-final-only")
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-workflow-final-only",
		SessionID:   "session-workflow-final-only",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "final-workflow-only",
				Type:   agentstate.TurnItemTypeAssistantMessage,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Summary: "Workflow patch_result 已应用，patch_id=patch-1。",
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-workflow-final-only"].AgentUIArtifacts
	for _, artifact := range artifacts {
		if strings.HasPrefix(artifact.Type, "workflow_") {
			t.Fatalf("AgentUIArtifacts = %#v, want no workflow artifact from final prose", artifacts)
		}
	}
}

func TestTransportProjectorProjectsCorootServiceMetricsChartArtifact(t *testing.T) {
	now := time.Date(2026, 5, 19, 9, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-chart", "thread-coroot-chart")
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-metrics",
		"toolName":"coroot.service_metrics",
		"displayKind":"coroot",
		"inputSummary":"checkout net",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:smecloud:Deployment:web",
			"metrics":[
				{"name":"cpu","status":"ok","unit":"cores","chartTitle":"CPU usage <selector>, cores","values":[[1710000000000,0.4],[1710000030000,0.6]],"series":[{"name":"web-1","values":[[1710000000000,0.4],[1710000030000,0.6]]}]},
				{"name":"memory","status":"warning","unit":"bytes","chartTitle":"Memory usage <selector>, bytes","values":[[1710000000000,1024],[1710000030000,2048]],"series":[{"name":"web-1","values":[[1710000000000,1024],[1710000030000,2048]]}]}
			],
			"chartReports":[
				{"name":"CPU","status":"ok","widgets":[{"chart_group":{"title":"CPU usage <selector>, cores","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"container: web","series":[{"name":"web-1","data":[0.4,0.6]}]}]}}]},
				{"name":"Net","status":"warning","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Failed TCP connections, per second","series":[{"name":"postgres","data":[0,1]}]}}]}
			],
			"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/web","digest":"sha256:abc","bytes":1024}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-chart",
		SessionID:   "session-coroot-chart",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-coroot-metrics", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "检查 checkout 网络异常"}, CreatedAt: now},
			{ID: "tool-result-coroot-metrics", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-coroot-chart"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "coroot_chart" || artifact.Source != "coroot" || artifact.PermissionScope != "read" {
		t.Fatalf("artifact = %+v, want coroot_chart from coroot", artifact)
	}
	card := artifact.InlineData["mcpCard"].(map[string]any)
	visual := card["visual"].(map[string]any)
	series, ok := visual["series"].([]map[string]any)
	if !ok {
		t.Fatalf("series type = %T, want []map[string]any", visual["series"])
	}
	if len(series) != 2 {
		t.Fatalf("series len = %d, want cpu and memory series", len(series))
	}
	chartReports, ok := artifact.InlineData["chartReports"].([]any)
	if !ok {
		t.Fatalf("chartReports type = %T, want []any", artifact.InlineData["chartReports"])
	}
	if len(chartReports) != 2 {
		t.Fatalf("chartReports len = %d, want native CPU and Net reports", len(chartReports))
	}
	if card["visual"].(map[string]any)["kind"] != "coroot_report_charts" {
		t.Fatalf("visual kind = %#v, want coroot_report_charts", card["visual"].(map[string]any)["kind"])
	}
	if artifact.Metadata["service"] != "5hxbfx6p:smecloud:Deployment:web" || artifact.DataRef != "http://coroot/api/project/5hxbfx6p/app/web" {
		t.Fatalf("artifact metadata=%#v dataRef=%q, want service and raw ref", artifact.Metadata, artifact.DataRef)
	}
	placement, ok := artifact.Metadata["placement"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.placement type = %T, want map", artifact.Metadata["placement"])
	}
	if placement["topic"] != "net" || placement["priority"] != "primary" || placement["service"] != "5hxbfx6p:smecloud:Deployment:web" {
		t.Fatalf("metadata.placement = %#v, want root_cause/evidence net primary placement", placement)
	}
	if !transportTestStringListContains(placement["supports"], "root_cause") {
		t.Fatalf("metadata.placement.supports = %#v, want root_cause", placement["supports"])
	}
	if !transportTestStringListContains(placement["preferredAfter"], "root_cause") {
		t.Fatalf("metadata.placement.preferredAfter = %#v, want root_cause", placement["preferredAfter"])
	}
	if !transportTestStringListContains(placement["preferredBefore"], "evidence") {
		t.Fatalf("metadata.placement.preferredBefore = %#v, want evidence", placement["preferredBefore"])
	}
	chartSummary, ok := artifact.Metadata["chartSummary"].(map[string]any)
	if !ok {
		t.Fatalf("metadata.chartSummary type = %T, want map", artifact.Metadata["chartSummary"])
	}
	if chartSummary["service"] != "5hxbfx6p:smecloud:Deployment:web" || chartSummary["defaultReportName"] != "Net" {
		t.Fatalf("metadata.chartSummary = %#v, want service and preferred Net report", chartSummary)
	}
	summaryJSON, err := json.Marshal(chartSummary)
	if err != nil {
		t.Fatalf("marshal metadata.chartSummary: %v", err)
	}
	if strings.Contains(string(summaryJSON), `"data"`) || strings.Contains(string(summaryJSON), `"values"`) {
		t.Fatalf("metadata.chartSummary leaked raw series arrays: %s", summaryJSON)
	}
}

func TestTransportProjectorUsesCorootDisplayDataForChartsWithoutProcessPreviewLeak(t *testing.T) {
	now := time.Date(2026, 5, 19, 9, 45, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-display-chart", "thread-coroot-display-chart")
	displayData := json.RawMessage(`{
		"schemaVersion":"aiops.coroot/v1",
		"tool":"coroot.service_metrics",
		"status":"ok",
		"project":"5hxbfx6p",
		"service":"5hxbfx6p:smecloud:Deployment:web",
		"metrics":[],
		"chartReports":[
			{"name":"CPU","status":"ok","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"CPU usage","series":[{"name":"web-1","data":[0.4,0.6]}]}}]}
		],
		"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/web","digest":"sha256:abc","bytes":1024}
	}`)
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-display-metrics",
		"toolName":"coroot.service_metrics",
		"displayKind":"coroot",
		"inputSummary":"web metrics",
		"outputPreview":{"schemaVersion":"aiops.coroot/v1","tool":"coroot.service_metrics","status":"ok","service":"5hxbfx6p:smecloud:Deployment:web","chartSummary":{"service":"5hxbfx6p:smecloud:Deployment:web","reports":[{"name":"CPU","pointCount":2}]}},
		"displayData":` + string(displayData) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-display-chart",
		SessionID:   "session-coroot-display-chart",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "user-coroot-display-metrics", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "查看 web CPU 图表"}, CreatedAt: now},
			{ID: "tool-result-coroot-display-metrics", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	projectedTurn := projected.Turns["turn-coroot-display-chart"]
	if len(projectedTurn.AgentUIArtifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want coroot chart artifact from displayData", len(projectedTurn.AgentUIArtifacts))
	}
	artifact := projectedTurn.AgentUIArtifacts[0]
	chartReports, ok := artifact.InlineData["chartReports"].([]any)
	if !ok || len(chartReports) != 1 {
		t.Fatalf("artifact chartReports = %#v, want native charts from displayData", artifact.InlineData["chartReports"])
	}
	if len(projectedTurn.Process) != 1 {
		t.Fatalf("process len = %d, want one tool block", len(projectedTurn.Process))
	}
	if strings.Contains(projectedTurn.Process[0].OutputPreview, "chartReports") || strings.Contains(projectedTurn.Process[0].OutputPreview, `"data"`) {
		t.Fatalf("process output preview leaked raw Coroot chart payload: %s", projectedTurn.Process[0].OutputPreview)
	}
}

func transportTestStringListContains(value any, want string) bool {
	switch typed := value.(type) {
	case []string:
		for _, item := range typed {
			if item == want {
				return true
			}
		}
	case []any:
		for _, item := range typed {
			if s, ok := item.(string); ok && s == want {
				return true
			}
		}
	}
	return false
}

func TestTransportProjectorDeduplicatesCorootChartArtifactPerService(t *testing.T) {
	now := time.Date(2026, 5, 20, 2, 50, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-chart-dedupe", "thread-coroot-chart-dedupe")
	firstToolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-metrics-24h",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"{\"service\":\"aiops-host-agent\",\"timeRange\":\"24h\",\"metrics\":[\"cpu\",\"memory\"]}",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
			"metrics":[],
			"chartReports":[{"name":"CPU","status":"ok","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"CPU usage","series":[{"name":"node-1","data":[0.1,0.2]}]}}]}],
			"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/aiops-host-agent?range=24h","digest":"sha256:24h","bytes":2048}
		}
	}`)
	secondToolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-metrics-1h",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"{\"service\":\"aiops-host-agent\",\"timeRange\":\"1h\",\"metrics\":[\"cpu\"]}",
		"outputPreview":{
			"schemaVersion":"aiops.coroot/v1",
			"tool":"coroot.service_metrics",
			"status":"ok",
			"project":"5hxbfx6p",
			"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
			"metrics":[],
			"chartReports":[{"name":"CPU","status":"ok","widgets":[{"chart":{"ctx":{"from":1710003600000,"step":30000},"title":"CPU usage","series":[{"name":"node-1","data":[0.3,0.4]}]}}]}],
			"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/aiops-host-agent?range=1h","digest":"sha256:1h","bytes":1024}
		}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-chart-dedupe",
		SessionID:   "session-coroot-chart-dedupe",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{
			{ID: "tool-result-coroot-metrics-24h", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics 24h", Data: firstToolResultData}, CreatedAt: now},
			{ID: "tool-result-coroot-metrics-1h", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics 1h", Data: secondToolResultData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-coroot-chart-dedupe"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want one deduplicated coroot_chart", len(artifacts))
	}
	if artifacts[0].DataRef != "http://coroot/api/project/5hxbfx6p/app/aiops-host-agent?range=1h" {
		t.Fatalf("DataRef = %q, want latest service metrics artifact", artifacts[0].DataRef)
	}
}

func TestTransportProjectorProjectsCorootChartArtifactFromRawToolResultContent(t *testing.T) {
	now := time.Date(2026, 5, 19, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-coroot-raw-chart", "thread-coroot-raw-chart")
	rawContent := `{
		"schemaVersion":"aiops.coroot/v1",
		"tool":"coroot.service_metrics",
		"status":"ok",
		"project":"5hxbfx6p",
		"service":"5hxbfx6p:_:Unknown:aiops-host-agent",
		"metrics":[],
		"chartReports":[
			{"name":"Instances","status":"ok","widgets":[{"chart":{"ctx":{"from":1710000000000,"step":30000},"title":"Instances","series":[{"name":"up","data":[2,2]}]}}]},
			{"name":"CPU","status":"ok","widgets":[{"chart_group":{"title":"CPU usage <selector>, cores","charts":[{"ctx":{"from":1710000000000,"step":30000},"title":"container: aiops-host-agent","series":[{"name":"aiops-host-agent@node-1","data":[0.0004,0.0006]}]}]}}]}
		],
		"rawRef":{"uri":"http://coroot/api/project/5hxbfx6p/app/aiops-host-agent","digest":"sha256:abc","bytes":4096}
	}`
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-coroot-raw-metrics",
		"toolName":"coroot_service_metrics",
		"displayKind":"coroot",
		"inputSummary":"aiops-host-agent metrics"
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-coroot-raw-chart",
		SessionID:   "session-coroot-raw-chart",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		Iterations: []runtimekernel.IterationState{{
			ID:          "iter-coroot-raw-chart",
			SessionID:   "session-coroot-raw-chart",
			TurnID:      "turn-coroot-raw-chart",
			Iteration:   0,
			Lifecycle:   runtimekernel.TurnLifecycleCompleted,
			ResumeState: runtimekernel.TurnResumeStateNone,
			ToolResults: []runtimekernel.ToolResult{{ToolCallID: "call-coroot-raw-metrics", Content: rawContent}},
			StartedAt:   now,
			UpdatedAt:   now,
		}},
		AgentItems: []agentstate.TurnItem{
			{ID: "user-coroot-raw-metrics", Type: agentstate.TurnItemTypeUserMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Summary: "查看 aiops-host-agent 的 CPU 图表"}, CreatedAt: now},
			{ID: "tool-result-coroot-raw-metrics", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "Coroot metrics", Data: toolResultData}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-coroot-raw-chart"].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("AgentUIArtifacts len = %d, want 1 coroot_chart from raw tool result content", len(artifacts))
	}
	artifact := artifacts[0]
	if artifact.Type != "coroot_chart" {
		t.Fatalf("artifact.Type = %q, want coroot_chart", artifact.Type)
	}
	chartReports := artifact.InlineData["chartReports"].([]any)
	if len(chartReports) != 2 {
		t.Fatalf("chartReports len = %d, want Instances and CPU reports", len(chartReports))
	}
	if artifact.InlineData["defaultReportName"] != "CPU" {
		t.Fatalf("defaultReportName = %#v, want CPU", artifact.InlineData["defaultReportName"])
	}
	if artifact.DataRef != "http://coroot/api/project/5hxbfx6p/app/aiops-host-agent" {
		t.Fatalf("DataRef = %q, want rawRef URI", artifact.DataRef)
	}
}

func TestTransportProjectorDoesNotProjectRCAReportArtifactFromFinalPayload(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-final-rca", "thread-final-rca")
	finalPayload := `{
		"schemaVersion":"aiops.rca_report/v1",
		"status":"partial",
		"source":"coroot",
		"target":{"service":"checkout"},
		"conclusion":{"summaryZh":"checkout 延迟升高与 catalog 依赖相关。","confidence":0.72},
		"evidenceRefs":["ev-coroot-latency"],
		"rawRefs":[{"uri":"coroot://raw/latency","digest":"abc123"}]
	}`
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-rca",
		SessionID:   "session-final-rca",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
		Metadata: map[string]string{
			"aiops.coroot.explicitRCA": "true",
		},
		AgentItems: []agentstate.TurnItem{
			{ID: "final-rca", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: finalPayload,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	artifacts := projected.Turns["turn-final-rca"].AgentUIArtifacts
	if len(artifacts) != 0 {
		t.Fatalf("AgentUIArtifacts = %#v, want final text to remain presentation-only", artifacts)
	}
}

func TestTransportProjectorSuppressesRCAReportArtifactFromFinalPayloadWithoutExplicitCorootGate(t *testing.T) {
	now := time.Date(2026, 5, 18, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-final-rca-suppressed", "thread-final-rca-suppressed")
	finalPayload := `{
		"schemaVersion":"aiops.rca_report/v1",
		"status":"partial",
		"source":"coroot",
		"conclusion":{"summaryZh":"checkout 延迟升高与 catalog 依赖相关。","confidence":0.72},
		"evidenceRefs":["ev-coroot-latency"],
		"rawRefs":[{"uri":"coroot://raw/latency","digest":"abc123"}]
	}`
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-final-rca-suppressed",
		SessionID:   "session-final-rca-suppressed",
		SessionType: runtimekernel.SessionTypeHost,
		Mode:        runtimekernel.ModeInspect,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		CompletedAt: &now,
		AgentItems: []agentstate.TurnItem{
			{ID: "final-rca", Type: agentstate.TurnItemTypeAssistantMessage, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{
				Summary: finalPayload,
				Data:    json.RawMessage(`{"displayKind":"assistant.message","phase":"final_answer","streamState":"complete"}`),
			}, CreatedAt: now},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if artifacts := projected.Turns["turn-final-rca-suppressed"].AgentUIArtifacts; len(artifacts) != 0 {
		t.Fatalf("AgentUIArtifacts = %#v, want no RCA artifact without explicit @Coroot gate", artifacts)
	}
}

func TestTransportProjectorCompactsOpsManualSearchProcessPreview(t *testing.T) {
	now := time.Date(2026, 5, 15, 9, 30, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-ops-manual-search", "thread-ops-manual-search")
	searchPayload, _ := json.Marshal(map[string]any{
		"decision": "need_info",
		"summary":  "信息不足，不能直接使用工作流。",
		"manuals": []map[string]any{
			{
				"manual": map[string]any{
					"id":    "manual-redis-rca-ssh",
					"title": "Redis SSH 排障运维手册",
				},
				"missing_fields": []string{"environment", "execution_surface", "symptom", "metrics"},
			},
		},
		"operation_frame": map[string]any{
			"evidence": map[string]any{"missing": []string{"environment", "execution_surface", "symptom", "metrics"}},
		},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-search",
		"toolName":"search_ops_manuals",
		"displayKind":"ops_manual_search_result",
		"outputPreview":` + string(searchPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-ops-manual-search",
		SessionID: "session-ops-manual-search",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-search",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_search_result",
					Summary: "need_info",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-ops-manual-search"]
	if len(transportTurn.AgentUIArtifacts) != 1 || transportTurn.AgentUIArtifacts[0].Type != "ops_manual_search_result" {
		t.Fatalf("artifacts = %#v, want ops manual search artifact", transportTurn.AgentUIArtifacts)
	}
	if len(transportTurn.Process) != 1 {
		t.Fatalf("process = %#v, want one compact tool block", transportTurn.Process)
	}
	block := transportTurn.Process[0]
	if block.OutputPreview != "" {
		t.Fatalf("output preview = %q, want hidden preview for ops manual search", block.OutputPreview)
	}
	if !strings.Contains(block.Text, "运维手册匹配：手册缺上下文") || strings.Contains(block.Text, "need_info") || strings.Contains(block.Text, "execution_surface") {
		t.Fatalf("block text = %q, want human compact decision without internal missing fields", block.Text)
	}
}

func TestTransportProjectorSkipsOpsManualNoMatchArtifact(t *testing.T) {
	now := time.Date(2026, 5, 19, 18, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-ops-manual-no-match", "thread-ops-manual-no-match")
	searchPayload, _ := json.Marshal(map[string]any{
		"decision": "no_match",
		"summary":  "没有找到适用于 service 的可用运维手册。",
		"manuals":  []map[string]any{},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-search-no-match",
		"toolName":"search_ops_manuals",
		"displayKind":"ops_manual_search_result",
		"outputPreview":` + string(searchPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-ops-manual-no-match",
		SessionID: "session-ops-manual-no-match",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-search-no-match",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_search_result",
					Summary: "no_match",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-ops-manual-no-match"]
	if len(transportTurn.AgentUIArtifacts) != 0 {
		t.Fatalf("artifacts = %#v, want no Agent UI artifact for no_match ops manual search", transportTurn.AgentUIArtifacts)
	}
	if len(transportTurn.Process) != 0 {
		t.Fatalf("process = %#v, want no process row for no_match ops manual search", transportTurn.Process)
	}
}

func TestTransportProjectorSkipsOpsManualNeedInfoWithoutManual(t *testing.T) {
	now := time.Date(2026, 5, 19, 18, 5, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-ops-manual-need-info-empty", "thread-ops-manual-need-info-empty")
	searchPayload, _ := json.Marshal(map[string]any{
		"decision": "need_info",
		"summary":  "缺少运维对象和操作类型。",
		"manuals":  []map[string]any{},
	})
	toolResultData := json.RawMessage(`{
		"toolCallId":"call-search-need-info-empty",
		"toolName":"search_ops_manuals",
		"displayKind":"ops_manual_search_result",
		"outputPreview":` + string(searchPayload) + `
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-ops-manual-need-info-empty",
		SessionID: "session-ops-manual-need-info-empty",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-search-need-info-empty",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "ops_manual_search_result",
					Summary: "need_info",
					Data:    toolResultData,
				},
				CreatedAt: now,
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	transportTurn := projected.Turns["turn-ops-manual-need-info-empty"]
	if len(transportTurn.AgentUIArtifacts) != 0 {
		t.Fatalf("artifacts = %#v, want no Agent UI artifact without a matched manual", transportTurn.AgentUIArtifacts)
	}
	if len(transportTurn.Process) != 0 {
		t.Fatalf("process = %#v, want no process row without a matched manual", transportTurn.Process)
	}
}

func TestOpsManualSearchReferenceOnlySummaryPromotesReadOnlyInvestigation(t *testing.T) {
	if got := opsManualSearchSummaryZh("reference_only"); !strings.Contains(got, "没有可直接运行的 Workflow") || !strings.Contains(got, "继续只读自动化排查") {
		t.Fatalf("summary = %q, want read-only continuation without runnable Workflow", got)
	}
	if actions := opsManualSearchArtifactActions("reference_only"); len(actions) != 0 {
		t.Fatalf("actions = %#v, want no executable or step-by-step action for reference_only search", actions)
	}
}

func assertTransportProjectionHasProcessStatuses(t *testing.T, projected AiopsTransportState, required []AiopsTransportProcessStatus) {
	t.Helper()
	seen := map[AiopsTransportProcessStatus]bool{}
	for _, turn := range projected.Turns {
		for _, block := range turn.Process {
			seen[block.Status] = true
		}
	}
	for _, status := range required {
		if !seen[status] {
			t.Fatalf("transport process statuses = %#v, missing %q", seen, status)
		}
	}
}

func assertNoForbiddenTransportProjectionStates(t *testing.T, projected AiopsTransportState, forbidden []string) {
	t.Helper()
	raw, err := json.Marshal(projected)
	if err != nil {
		t.Fatalf("marshal transport projection: %v", err)
	}
	body := string(raw)
	for _, state := range forbidden {
		if strings.Contains(body, state) {
			t.Fatalf("transport projection exposed forbidden state %q: %s", state, body)
		}
	}
}

func transportProjectionText(projected AiopsTransportState) string {
	var parts []string
	for _, turnID := range projected.TurnOrder {
		turn := projected.Turns[turnID]
		if turn.User != nil {
			parts = append(parts, turn.User.Text)
		}
		for _, block := range turn.Process {
			parts = append(parts, block.Text, block.Command, block.InputSummary, block.OutputPreview)
			for _, step := range block.Steps {
				parts = append(parts, step.Text, step.Title, step.Summary)
			}
		}
		if turn.Final != nil {
			parts = append(parts, turn.Final.Text)
		}
	}
	return strings.Join(parts, "\n")
}

func ptrTransportProjectorTime(value time.Time) *time.Time {
	return &value
}

func findTransportProcessBlock(t *testing.T, blocks []AiopsProcessBlock, kind AiopsTransportProcessKind) AiopsProcessBlock {
	t.Helper()
	for _, block := range blocks {
		if block.Kind == kind {
			return block
		}
	}
	t.Fatalf("missing process block kind %q in %+v", kind, blocks)
	return AiopsProcessBlock{}
}

func TestIsHTMLContent(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  bool
	}{
		{"DOCTYPE uppercase", "<!DOCTYPE html>", true},
		{"html lowercase tag", "<html><body></body></html>", true},
		{"HTML uppercase tag", "<HTML><BODY></BODY></HTML>", true},
		{"leading whitespace with DOCTYPE", "  \t\n<!DOCTYPE html>", true},
		{"leading whitespace with html tag", "   <html>", true},
		{"leading whitespace with HTML tag", "\n\n  <HTML>", true},
		{"plain text", "hello world", false},
		{"empty string", "", false},
		{"partial match", "<ht", false},
		{"json content", `{"key":"value"}`, false},
		{"markdown content", "# Title\n\nSome text", false},
		{"xml but not html", "<?xml version=\"1.0\"?>", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isHTMLContent(tt.input)
			if got != tt.want {
				t.Errorf("isHTMLContent(%q) = %v, want %v", tt.input, got, tt.want)
			}
		})
	}
}

func TestSanitizeOutputPreview(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"short plain text", "hello world", "hello world"},
		{"plain text under 500 runes", "short content", "short content"},
		{"plain text exactly 500 runes", string(make([]rune, 500)), string(make([]rune, 500))},
		{"plain text over 500 runes truncated", func() string {
			r := make([]rune, 510)
			for i := range r {
				r[i] = 'a'
			}
			return string(r)
		}(), func() string {
			r := make([]rune, 500)
			for i := range r {
				r[i] = 'a'
			}
			return string(r) + "…"
		}()},
		{"HTML content stripped and under 200 runes", "<!DOCTYPE html><html><body><p>Hello</p></body></html>", "Hello"},
		{"HTML content stripped and over 200 runes truncated", func() string {
			// Build HTML with >200 runes of text content
			r := make([]rune, 250)
			for i := range r {
				r[i] = '中'
			}
			return "<!DOCTYPE html><html><body><p>" + string(r) + "</p></body></html>"
		}(), func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = '中'
			}
			return string(r) + "…"
		}()},
		{"HTML content stripped exactly 200 runes", func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = 'x'
			}
			return "<html><body>" + string(r) + "</body></html>"
		}(), func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = 'x'
			}
			return string(r)
		}()},
		{"multi-byte rune-aware truncation for non-HTML", func() string {
			r := make([]rune, 510)
			for i := range r {
				r[i] = '日'
			}
			return string(r)
		}(), func() string {
			r := make([]rune, 500)
			for i := range r {
				r[i] = '日'
			}
			return string(r) + "…"
		}()},
		{"multi-byte rune-aware truncation for HTML", func() string {
			r := make([]rune, 210)
			for i := range r {
				r[i] = '本'
			}
			return "<html><body>" + string(r) + "</body></html>"
		}(), func() string {
			r := make([]rune, 200)
			for i := range r {
				r[i] = '本'
			}
			return string(r) + "…"
		}()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeOutputPreview(tt.input)
			if got != tt.want {
				t.Errorf("sanitizeOutputPreview() got len=%d, want len=%d", len([]rune(got)), len([]rune(tt.want)))
				if len(got) < 100 && len(tt.want) < 100 {
					t.Errorf("  got=%q, want=%q", got, tt.want)
				}
			}
		})
	}
}

func TestTransportProjectorSanitizesHTMLInToolOutput(t *testing.T) {
	now := time.Date(2026, 5, 8, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-html", "thread-html")

	rawHTML := `<!DOCTYPE html><html><body><h1>Title</h1><p>Some content here</p></body></html>`
	toolResultData := json.RawMessage(`{
		"toolCallId":"tool-html-1",
		"toolName":"fetch_page",
		"displayKind":"tool",
		"inputSummary":"https://example.com",
		"outputSummary":"` + rawHTML + `",
		"outputPreview":"` + rawHTML + `"
	}`)

	turn := &runtimekernel.TurnSnapshot{
		ID:        "turn-html",
		SessionID: "session-html",
		Lifecycle: runtimekernel.TurnLifecycleCompleted,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "tool-result-html",
				Type:   agentstate.TurnItemTypeToolResult,
				Status: agentstate.ItemStatusCompleted,
				Payload: agentstate.PayloadEnvelope{
					Kind:    "tool",
					Summary: rawHTML,
					Data:    toolResultData,
				},
				CreatedAt: now.Add(time.Second),
			},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	transportTurn := projected.Turns["turn-html"]
	if len(transportTurn.Process) == 0 {
		t.Fatal("expected at least one process block")
	}

	block := transportTurn.Process[0]

	// OutputPreview must not contain raw HTML tags
	if strings.Contains(block.OutputPreview, "<") || strings.Contains(block.OutputPreview, ">") {
		t.Errorf("OutputPreview contains raw HTML tags: %q", block.OutputPreview)
	}
	// Text must not contain raw HTML tags
	if strings.Contains(block.Text, "<") || strings.Contains(block.Text, ">") {
		t.Errorf("Text contains raw HTML tags: %q", block.Text)
	}

	// Verify the text content is preserved (stripped of tags)
	if !strings.Contains(block.OutputPreview, "Title") || !strings.Contains(block.OutputPreview, "Some content here") {
		t.Errorf("OutputPreview should contain stripped text content, got: %q", block.OutputPreview)
	}
	if !strings.Contains(block.Text, "Title") || !strings.Contains(block.Text, "Some content here") {
		t.Errorf("Text should contain stripped text content, got: %q", block.Text)
	}
}

func TestStripHTMLTags(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"empty string", "", ""},
		{"plain text no tags", "hello world", "hello world"},
		{"simple paragraph", "<p>hello</p>", "hello"},
		{"nested tags", "<div><p>hello</p></div>", "hello"},
		{"multiple elements", "<h1>Title</h1><p>Body text</p>", "Title Body text"},
		{"self-closing tags", "before<br/>after", "before after"},
		{"attributes in tags", `<a href="https://example.com">link</a>`, "link"},
		{"full HTML document", "<!DOCTYPE html><html><head><title>Test</title></head><body><p>Content</p></body></html>", "Test Content"},
		{"whitespace between tags", "<p>  hello  </p>  <p>  world  </p>", "hello world"},
		{"newlines and tabs", "<div>\n\t<span>text</span>\n</div>", "text"},
		{"only tags no content", "<br/><hr/><img src='x'/>", ""},
		{"mixed content and tags", "Start <b>bold</b> middle <i>italic</i> end", "Start bold middle italic end"},
		{"tag with multiline attributes", "<div\n  class=\"foo\"\n  id=\"bar\">content</div>", "content"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripHTMLTags(tt.input)
			if got != tt.want {
				t.Errorf("stripHTMLTags(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func transportOpsRunHasPostRunSuggestion(items []PostRunSuggestion, typ PostRunSuggestionType) bool {
	for _, item := range items {
		if item.Type == typ {
			return true
		}
	}
	return false
}
