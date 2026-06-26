package appui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/projection"
	"aiops-v2/internal/runtimekernel"
)

func lifecycleEvent(eventType runtimekernel.EventType) runtimekernel.LifecycleEvent {
	payload, _ := json.Marshal(map[string]any{"text": "hello"})
	return runtimekernel.LifecycleEvent{
		Type:      eventType,
		SessionID: "session-1",
		TurnID:    "turn-1",
		Timestamp: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
		Payload:   payload,
	}
}

func TestNormalizeRuntimeLifecycleEvent(t *testing.T) {
	tests := []struct {
		name    string
		rawType runtimekernel.EventType
		kind    AgentEventKind
		phase   AgentEventPhase
		status  AgentEventStatus
		channel string
	}{
		{name: "turn started", rawType: runtimekernel.EventTurnStarted, kind: AgentEventTurn, phase: AgentEventPhaseStarted, status: AgentEventStatusRunning},
		{name: "assistant intent", rawType: runtimekernel.EventAssistantIntent, kind: AgentEventAssistant, phase: AgentEventPhaseDelta, status: AgentEventStatusRunning, channel: "intent"},
		{name: "assistant final", rawType: runtimekernel.EventAssistantFinalDelta, kind: AgentEventAssistant, phase: AgentEventPhaseDelta, status: AgentEventStatusRunning, channel: "final"},
		{name: "phase end", rawType: runtimekernel.EventPhaseEnd, kind: AgentEventSystem, phase: AgentEventPhaseCompleted, status: AgentEventStatusCompleted},
		{name: "process summary", rawType: runtimekernel.EventProcessSummary, kind: AgentEventAssistant, phase: AgentEventPhaseCompleted, status: AgentEventStatusCompleted, channel: "summary"},
		{name: "turn complete", rawType: runtimekernel.EventTurnComplete, kind: AgentEventTurn, phase: AgentEventPhaseCompleted, status: AgentEventStatusCompleted},
		{name: "turn error", rawType: runtimekernel.EventTurnError, kind: AgentEventTurn, phase: AgentEventPhaseFailed, status: AgentEventStatusFailed},
		{name: "turn aborted", rawType: runtimekernel.EventTurnAborted, kind: AgentEventTurn, phase: AgentEventPhaseCanceled, status: AgentEventStatusCanceled},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := NormalizeRuntimeLifecycleEvent(lifecycleEvent(tt.rawType))
			if err != nil {
				t.Fatalf("NormalizeRuntimeLifecycleEvent() error = %v", err)
			}
			if len(events) != 1 {
				t.Fatalf("len(events) = %d, want 1", len(events))
			}
			got := events[0]
			if got.Kind != tt.kind || got.Phase != tt.phase || got.Status != tt.status {
				t.Fatalf("event = %s/%s/%s, want %s/%s/%s", got.Kind, got.Phase, got.Status, tt.kind, tt.phase, tt.status)
			}
			if tt.channel != "" {
				var payload map[string]any
				if err := json.Unmarshal(got.Payload, &payload); err != nil {
					t.Fatalf("payload decode error = %v", err)
				}
				if payload["channel"] != tt.channel {
					t.Fatalf("payload.channel = %v, want %q", payload["channel"], tt.channel)
				}
			}
		})
	}
}

func TestNormalizeRuntimeLifecycleEventKeepsAssistantFinalDeltaSingleChannel(t *testing.T) {
	payload, _ := json.Marshal(map[string]any{
		"text":        "最终回答流式片段",
		"phase":       "final_answer",
		"streamState": "streaming",
		"iteration":   0,
	})
	events, err := NormalizeRuntimeLifecycleEvent(runtimekernel.LifecycleEvent{
		Type:      runtimekernel.EventAssistantFinalDelta,
		SessionID: "session-1",
		TurnID:    "turn-1",
		Timestamp: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
		Payload:   payload,
	})
	if err != nil {
		t.Fatalf("NormalizeRuntimeLifecycleEvent() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("events = %d, want 1", len(events))
	}
	var normalized map[string]any
	if err := json.Unmarshal(events[0].Payload, &normalized); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if normalized["channel"] != "final" {
		t.Fatalf("payload.channel = %v, want final", normalized["channel"])
	}
	if normalized["phase"] != "final_answer" || normalized["streamState"] != "streaming" {
		t.Fatalf("payload = %#v, want assistant message phase metadata preserved", normalized)
	}
}

func TestNormalizeToolInvocation(t *testing.T) {
	tests := []struct {
		status projection.ToolInvocationStatus
		phase  AgentEventPhase
	}{
		{status: projection.ToolInvocationStarted, phase: AgentEventPhaseStarted},
		{status: projection.ToolInvocationProgress, phase: AgentEventPhaseUpdated},
		{status: projection.ToolInvocationCompleted, phase: AgentEventPhaseCompleted},
		{status: projection.ToolInvocationFailed, phase: AgentEventPhaseFailed},
	}
	for _, tt := range tests {
		t.Run(string(tt.status), func(t *testing.T) {
			events, err := NormalizeToolInvocation(projection.ToolInvocation{
				ID:        "tool-1",
				SessionID: "session-1",
				TurnID:    "turn-1",
				ToolName:  "web_search",
				Status:    tt.status,
				StartedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("NormalizeToolInvocation() error = %v", err)
			}
			got := events[0]
			if got.Kind != AgentEventTool || got.Phase != tt.phase {
				t.Fatalf("event = %s/%s, want tool/%s", got.Kind, got.Phase, tt.phase)
			}
			var payload ToolPayload
			if err := json.Unmarshal(got.Payload, &payload); err != nil {
				t.Fatalf("payload decode error = %v", err)
			}
			if payload.ToolCallID != "tool-1" {
				t.Fatalf("toolCallId = %q, want tool-1", payload.ToolCallID)
			}
		})
	}
}

func TestNormalizeToolInvocationSummarizesToolPayloadsForAgentEvents(t *testing.T) {
	events, err := NormalizeToolInvocation(projection.ToolInvocation{
		ID:        "tool-search-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		ToolName:  "web_search",
		Args:      json.RawMessage(`{"query":"market status","search_context_size":"medium"}`),
		Result:    `{"query":"market status","source":"public_web_search:bing_fallback","content":"Public web search results for \"market status\".\n1. Market report\n   URL: https://example.com/market\n   Snippet: Index moved higher."}`,
		Status:    projection.ToolInvocationCompleted,
		StartedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NormalizeToolInvocation() error = %v", err)
	}
	var payload ToolPayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload.InputSummary != "market status" {
		t.Fatalf("inputSummary = %q, want query only", payload.InputSummary)
	}
	if payload.OutputSummary != "找到 1 条网页结果：Market report" {
		t.Fatalf("outputSummary = %q, want natural search summary", payload.OutputSummary)
	}
	if payload.OutputSummary == "" || payload.OutputSummary[0] == '{' {
		t.Fatalf("outputSummary = %q, should not expose raw JSON", payload.OutputSummary)
	}
}

func TestNormalizeToolInvocationAddsCodexLikeDisplayMetadata(t *testing.T) {
	events, err := NormalizeToolInvocation(projection.ToolInvocation{
		ID:        "tool-exec-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		ToolName:  "exec_command",
		Args:      json.RawMessage(`{"command":"printf ok"}`),
		Result:    `ok`,
		Status:    projection.ToolInvocationCompleted,
		StartedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
		EndedAt:   ptrTime(time.Date(2026, 4, 24, 0, 0, 1, 500000000, time.UTC)),
	})
	if err != nil {
		t.Fatalf("NormalizeToolInvocation() error = %v", err)
	}
	got := events[0]
	if got.EventID != "turn-1:tool:tool-exec-1:completed" {
		t.Fatalf("eventId = %q, want stable completed id", got.EventID)
	}
	if got.DurationMs != 1500 {
		t.Fatalf("durationMs = %d, want 1500", got.DurationMs)
	}
	var payload ToolPayload
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload.DisplayKind != "host.command" {
		t.Fatalf("displayKind = %q, want host.command", payload.DisplayKind)
	}
	if payload.Title == "" {
		t.Fatal("title should be populated for codex-like tool rows")
	}
	if !payload.Foldable || !payload.AutoCollapse {
		t.Fatalf("foldable/autoCollapse = %v/%v, want true/true", payload.Foldable, payload.AutoCollapse)
	}
	if payload.DurationMs != 1500 {
		t.Fatalf("payload.durationMs = %d, want 1500", payload.DurationMs)
	}
}

func TestNormalizeToolInvocationBudgetsToolResultPayloads(t *testing.T) {
	tests := []struct {
		name        string
		result      string
		wantPreview bool
		wantRawRef  bool
		wantFull    bool
	}{
		{
			name:        "inline result",
			result:      "short result",
			wantPreview: true,
			wantRawRef:  false,
			wantFull:    true,
		},
		{
			name:        "medium result",
			result:      strings.Repeat("m", inlineToolResultBytes+256),
			wantPreview: true,
			wantRawRef:  true,
			wantFull:    false,
		},
		{
			name:        "large result",
			result:      strings.Repeat("l", maxAgentEventPayloadBytes+256),
			wantPreview: true,
			wantRawRef:  true,
			wantFull:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events, err := NormalizeToolInvocation(projection.ToolInvocation{
				ID:        "tool-budget-1",
				SessionID: "session-1",
				TurnID:    "turn-1",
				ToolName:  "exec_command",
				Args:      json.RawMessage(`{"command":"cat large.log"}`),
				Result:    tt.result,
				Status:    projection.ToolInvocationCompleted,
				StartedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
			})
			if err != nil {
				t.Fatalf("NormalizeToolInvocation() error = %v", err)
			}
			var payload ToolPayload
			if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
				t.Fatalf("payload decode error = %v", err)
			}
			if payload.OutputSummary == "" {
				t.Fatal("outputSummary should always be populated for non-empty results")
			}
			if got := len(payload.OutputPreview) > 0; got != tt.wantPreview {
				t.Fatalf("has outputPreview = %v, want %v", got, tt.wantPreview)
			}
			if got := payload.RawRef != ""; got != tt.wantRawRef {
				t.Fatalf("has rawRef = %v, want %v", got, tt.wantRawRef)
			}
			if len(payload.OutputPreview) > 0 {
				var preview string
				if err := json.Unmarshal(payload.OutputPreview, &preview); err != nil {
					t.Fatalf("outputPreview decode error = %v", err)
				}
				if tt.wantFull && preview != tt.result {
					t.Fatalf("preview = %q, want full result", preview)
				}
				if !tt.wantFull && len(preview) >= len(tt.result) {
					t.Fatalf("preview len = %d, want shorter than full result len %d", len(preview), len(tt.result))
				}
				if !tt.wantFull && len([]byte(preview)) > inlineToolResultBytes+len("...") {
					t.Fatalf("preview bytes = %d, want bounded preview", len([]byte(preview)))
				}
			}
		})
	}
}

func TestNormalizeEvidenceProjectsAgentEvidenceEvent(t *testing.T) {
	events, err := NormalizeEvidence(projection.Evidence{
		ID:        "evidence-1",
		SessionID: "session-1",
		TurnID:    "turn-1",
		Type:      "log",
		Summary:   "Redis connection refused 出现 12 次",
		CreatedAt: time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NormalizeEvidence() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1", len(events))
	}
	got := events[0]
	if got.Kind != AgentEventEvidence || got.Phase != AgentEventPhaseCompleted || got.Status != AgentEventStatusCompleted {
		t.Fatalf("event = %s/%s/%s, want evidence/completed/completed", got.Kind, got.Phase, got.Status)
	}
	var payload EvidencePayload
	if err := json.Unmarshal(got.Payload, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload.ID != "evidence-1" || payload.Kind != "log" || payload.Summary == "" {
		t.Fatalf("payload = %#v, want populated evidence payload", payload)
	}
}

func TestNormalizeEvidenceKeepsSourceConfidenceAndRawRef(t *testing.T) {
	events, err := NormalizeEvidence(projection.Evidence{
		ID:         "evidence-1",
		SessionID:  "session-1",
		TurnID:     "turn-1",
		Type:       "tool_result",
		Title:      "nginx 日志",
		Summary:    "upstream timeout for service-a",
		Source:     "tool:exec_command",
		Confidence: "high",
		CreatedAt:  time.Date(2026, 4, 24, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("NormalizeEvidence() error = %v", err)
	}
	var payload EvidencePayload
	if err := json.Unmarshal(events[0].Payload, &payload); err != nil {
		t.Fatalf("payload decode error = %v", err)
	}
	if payload.Title != "nginx 日志" {
		t.Fatalf("title = %q, want nginx 日志", payload.Title)
	}
	if payload.Source != "tool:exec_command" {
		t.Fatalf("source = %q, want tool:exec_command", payload.Source)
	}
	if payload.Confidence != "high" {
		t.Fatalf("confidence = %q, want high", payload.Confidence)
	}
	if payload.RawRef != "evidence://turn-1/evidence-1" {
		t.Fatalf("rawRef = %q, want evidence://turn-1/evidence-1", payload.RawRef)
	}
}

func ptrTime(value time.Time) *time.Time {
	return &value
}
