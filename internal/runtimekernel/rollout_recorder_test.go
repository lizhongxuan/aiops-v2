package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/tooling"
)

func TestRuntimeKernelCanonicalRolloutOrdersFirstLoopAndDoesNotRepeatAssembly(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("inspect", []schema.ToolCall{{
			ID: "call-read", Type: "function",
			Function: schema.FunctionCall{Name: "rollout_read", Arguments: `{}`},
		}}),
		schema.AssistantMessage("safe-final", nil),
	}}
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "rollout_read", Description: "read typed state"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "healthy"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	const sessionID = "session-runtime-rollout-order"
	const turnID = "turn-runtime-rollout-order"
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: turnID, Input: "input-secret-canary",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	wantKinds := []string{
		modeltrace.CanonicalRolloutKindAdmission,
		modeltrace.CanonicalRolloutKindAssembly,
		modeltrace.CanonicalRolloutKindPrompt,
		modeltrace.CanonicalRolloutKindProviderRequest,
		modeltrace.CanonicalRolloutKindProviderResponse,
		modeltrace.CanonicalRolloutKindPrompt,
		modeltrace.CanonicalRolloutKindProviderRequest,
		modeltrace.CanonicalRolloutKindProviderResponse,
	}
	if len(events) < len(wantKinds) {
		t.Fatalf("event kinds = %v, want prefix %v", canonicalRolloutKindsForTest(events), wantKinds)
	}
	for index, want := range wantKinds {
		if events[index].Kind != want || events[index].Sequence != int64(index+1) {
			t.Fatalf("events[%d] = kind %q sequence %d, want %q/%d; all=%v", index, events[index].Kind, events[index].Sequence, want, index+1, canonicalRolloutKindsForTest(events))
		}
	}
	if countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindAdmission) != 1 ||
		countCanonicalRolloutKind(events, modeltrace.CanonicalRolloutKindAssembly) != 1 {
		t.Fatalf("admission/assembly repeated across existing assembly: %v", canonicalRolloutKindsForTest(events))
	}

	session := kernel.sessions.Get(sessionID)
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.CanonicalRolloutHead == nil {
		t.Fatal("persisted turn is missing canonical rollout head")
	}
	head := session.CurrentTurn.CanonicalRolloutHead
	last := events[len(events)-1]
	if err := head.Validate(); err != nil {
		t.Fatalf("CanonicalRolloutHead.Validate() error = %v", err)
	}
	if head.SchemaVersion != last.SchemaVersion || head.EventID != last.EventID || head.Hash != last.Hash ||
		head.Sequence != last.Sequence || head.Status != RolloutRecordStatusRecorded {
		t.Fatalf("head = %+v, want last event ref %+v", head, last)
	}

	encoded, err := json.Marshal(events)
	if err != nil {
		t.Fatalf("json.Marshal(events) error = %v", err)
	}
	for _, forbidden := range []string{"input-secret-canary", "safe-final", "healthy", `\"rollout_read\"`, `\"Arguments\"`} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("canonical rollout leaked provider/model/tool payload %q: %s", forbidden, encoded)
		}
	}
}

func TestRuntimeKernelProviderRequestRolloutFailureCallsProviderZeroTimes(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindProviderRequest)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("must-not-run", nil)}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-provider-request-fail", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-provider-request-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want fail closed rollout append")
	}
	if len(model.inputs) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(model.inputs))
	}
	events, readErr := kernel.CanonicalRolloutEvents(context.Background(), "session-provider-request-fail", "turn-provider-request-fail")
	if readErr != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", readErr)
	}
	if got := canonicalRolloutKindsForTest(events); !equalCanonicalRolloutKinds(got, []string{
		modeltrace.CanonicalRolloutKindAdmission,
		modeltrace.CanonicalRolloutKindAssembly,
		modeltrace.CanonicalRolloutKindPrompt,
	}) {
		t.Fatalf("persisted kinds = %v, want admission/assembly/prompt only", got)
	}
}

func TestRuntimeKernelProviderResponseRolloutFailureStopsBeforeToolAndFinal(t *testing.T) {
	store := newKindFailingRolloutStore(modeltrace.CanonicalRolloutKindProviderResponse)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyFailClosed})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	toolCalls := 0
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "rollout_mutation", Description: "must not execute"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			toolCalls++
			return tooling.ToolResult{Content: "mutated"}, nil
		},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-mutation", Type: "function",
			Function: schema.FunctionCall{Name: "rollout_mutation", Arguments: `{}`},
		}}),
	}}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-provider-response-fail", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-provider-response-fail", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want provider_response append failure")
	}
	if toolCalls != 0 {
		t.Fatalf("tool calls = %d, want 0", toolCalls)
	}
	session := kernel.sessions.Get("session-provider-response-fail")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("missing persisted turn snapshot")
	}
	if len(session.CurrentTurn.Iterations) != 0 || strings.TrimSpace(session.CurrentTurn.FinalOutput) != "" {
		t.Fatalf("turn entered tool/final processing after recorder failure: iterations=%d final=%q", len(session.CurrentTurn.Iterations), session.CurrentTurn.FinalOutput)
	}
}

func TestRuntimeKernelConfigInjectsRecorderAndNilCreatesReadableFailClosedMemory(t *testing.T) {
	injected, err := NewRolloutRecorder(RolloutRecorderConfig{Store: NewMemoryRolloutStore()})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	if kernel := NewRuntimeKernel(RuntimeKernelConfig{RolloutRecorder: injected}); kernel.rolloutRecorder != injected {
		t.Fatal("RuntimeKernelConfig.RolloutRecorder was not injected")
	}

	kernel := NewRuntimeKernel(RuntimeKernelConfig{})
	if kernel.rolloutRecorder == nil || kernel.rolloutRecorder.failurePolicy != RolloutFailurePolicyFailClosed {
		t.Fatalf("default rollout recorder = %#v, want real fail-closed in-memory recorder", kernel.rolloutRecorder)
	}
	if _, err := kernel.CanonicalRolloutEvents(context.Background(), "missing-session", "missing-turn"); err != nil {
		t.Fatalf("default CanonicalRolloutEvents() error = %v", err)
	}
}

func TestCanonicalRolloutHeadRefRejectsInvalidHashAndEventReference(t *testing.T) {
	input := canonicalRecorderEvent("session-head", "turn-head", modeltrace.CanonicalRolloutKindPrompt)
	input.Sequence = 1
	event, err := modeltrace.FreezeCanonicalRolloutEvent(input)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	valid := CanonicalRolloutHeadRef{
		SchemaVersion: event.SchemaVersion,
		EventID:       event.EventID,
		Hash:          event.Hash,
		Sequence:      event.Sequence,
		Status:        RolloutRecordStatusRecorded,
	}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid head error = %v", err)
	}
	for name, mutate := range map[string]func(*CanonicalRolloutHeadRef){
		"event":  func(ref *CanonicalRolloutHeadRef) { ref.EventID = "event:not-a-digest" },
		"hash":   func(ref *CanonicalRolloutHeadRef) { ref.Hash = "sha256:not-a-digest" },
		"status": func(ref *CanonicalRolloutHeadRef) { ref.Status = RolloutRecordStatusFailed },
	} {
		t.Run(name, func(t *testing.T) {
			candidate := valid
			mutate(&candidate)
			if err := candidate.Validate(); err == nil {
				t.Fatalf("Validate(%s) error = nil, want rejection", name)
			}
		})
	}
}

func TestRuntimeKernelDegradedRecorderContinuesOnlyWithPersistedTypedMarker(t *testing.T) {
	store := newFailKindOnceRolloutStore(modeltrace.CanonicalRolloutKindProviderResponse)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyDegraded})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("typed-final", nil)}}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.rolloutRecorder = recorder

	const sessionID = "session-runtime-rollout-degraded"
	const turnID = "turn-runtime-rollout-degraded"
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: sessionID, SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: turnID, Input: "inspect",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	events, err := kernel.CanonicalRolloutEvents(context.Background(), sessionID, turnID)
	if err != nil {
		t.Fatalf("CanonicalRolloutEvents() error = %v", err)
	}
	if len(events) == 0 || events[len(events)-1].Kind != modeltrace.CanonicalRolloutKindRecorderDegraded {
		t.Fatalf("events = %v, want persisted recorder_degraded tail", canonicalRolloutKindsForTest(events))
	}
	session := kernel.sessions.Get(sessionID)
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.CanonicalRolloutHead == nil ||
		session.CurrentTurn.CanonicalRolloutHead.Status != RolloutRecordStatusDegraded ||
		session.CurrentTurn.CanonicalRolloutHead.EventID != events[len(events)-1].EventID {
		t.Fatalf("canonical rollout head = %#v, want observable degraded marker", session)
	}
}

func TestRuntimeKernelDegradedRecorderWithoutPersistedMarkerBlocksSideEffects(t *testing.T) {
	store := newFailKindAndMarkerRolloutStore(modeltrace.CanonicalRolloutKindProviderResponse)
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store, FailurePolicy: RolloutFailurePolicyDegraded})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}
	toolCalls := 0
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "degraded_mutation", Description: "must not execute"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			toolCalls++
			return tooling.ToolResult{Content: "mutated"}, nil
		},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("", []schema.ToolCall{{
		ID: "call-degraded", Type: "function",
		Function: schema.FunctionCall{Name: "degraded_mutation", Arguments: `{}`},
	}})}}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.rolloutRecorder = recorder

	_, err = kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-runtime-rollout-unobservable", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-runtime-rollout-unobservable", Input: "inspect",
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want unpersisted degraded marker to fail closed")
	}
	if len(model.inputs) != 1 || toolCalls != 0 {
		t.Fatalf("provider/tool calls = %d/%d, want 1/0", len(model.inputs), toolCalls)
	}
	session := kernel.sessions.Get("session-runtime-rollout-unobservable")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) != 0 || strings.TrimSpace(session.CurrentTurn.FinalOutput) != "" {
		t.Fatalf("turn crossed side-effect boundary after unpersisted marker: %#v", session)
	}
}

func TestRolloutRecorderAssignsStrictSequenceAndStableIdentity(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         store,
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	firstInput := canonicalRecorderEvent("session-1", "turn-1", modeltrace.CanonicalRolloutKindAssembly)
	first, err := recorder.Append(context.Background(), firstInput)
	if err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	second, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-1", "turn-1", modeltrace.CanonicalRolloutKindPrompt))
	if err != nil {
		t.Fatalf("Append(second) error = %v", err)
	}
	if first.Status != RolloutRecordStatusRecorded || second.Status != RolloutRecordStatusRecorded {
		t.Fatalf("statuses = %q, %q; want recorded", first.Status, second.Status)
	}
	if first.Event.Sequence != 1 || second.Event.Sequence != 2 {
		t.Fatalf("sequences = %d, %d; want 1, 2", first.Event.Sequence, second.Event.Sequence)
	}
	if first.Event.EventID == "" || first.Event.Hash == "" || first.Event.EventID == second.Event.EventID {
		t.Fatalf("unstable identities: first=%+v second=%+v", first.Event, second.Event)
	}

	// Replaying the same fact at the same sequence in a fresh recorder produces
	// the same identity; wall-clock time and storage do not participate.
	replay, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         NewMemoryRolloutStore(),
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder(replay) error = %v", err)
	}
	replayed, err := replay.Append(context.Background(), firstInput)
	if err != nil {
		t.Fatalf("Append(replay) error = %v", err)
	}
	if replayed.Event.EventID != first.Event.EventID || replayed.Event.Hash != first.Event.Hash {
		t.Fatalf("replayed identity = %q/%q; want %q/%q", replayed.Event.EventID, replayed.Event.Hash, first.Event.EventID, first.Event.Hash)
	}
}

func TestRolloutRecorderKeepsConcurrentTurnsIsolated(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	const eventsPerTurn = 40
	var wg sync.WaitGroup
	for index := 0; index < eventsPerTurn; index++ {
		for _, turnID := range []string{"turn-a", "turn-b"} {
			wg.Add(1)
			go func(index int, turnID string) {
				defer wg.Done()
				event := canonicalRecorderEvent("session-concurrent", turnID, modeltrace.CanonicalRolloutKindCheckpoint)
				event.Payload = map[string]any{"index": index}
				if result, appendErr := recorder.Append(context.Background(), event); appendErr != nil {
					t.Errorf("Append(%s, %d) error = %v (status=%q)", turnID, index, appendErr, result.Status)
				}
			}(index, turnID)
		}
	}
	wg.Wait()

	for _, turnID := range []string{"turn-a", "turn-b"} {
		events, readErr := store.Events(context.Background(), "session-concurrent", turnID)
		if readErr != nil {
			t.Fatalf("Events(%s) error = %v", turnID, readErr)
		}
		if len(events) != eventsPerTurn {
			t.Fatalf("Events(%s) length = %d; want %d", turnID, len(events), eventsPerTurn)
		}
		for index, event := range events {
			if want := int64(index + 1); event.Sequence != want {
				t.Fatalf("Events(%s)[%d].Sequence = %d; want %d", turnID, index, event.Sequence, want)
			}
		}
	}
}

func TestRolloutRecorderDeepCopiesAndRedactsBeforeAppend(t *testing.T) {
	store := NewMemoryRolloutStore()
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	sourceRefs := []string{"source-b", "source-a"}
	payload := map[string]any{
		"apiKey": "secret-canary",
		"nested": map[string]any{"authorization": "Bearer secret-canary", "safe": "before"},
	}
	result, err := recorder.Append(context.Background(), modeltrace.CanonicalRolloutEvent{
		SessionID:  "session-copy",
		TurnID:     "turn-copy",
		Kind:       modeltrace.CanonicalRolloutKindProviderRequest,
		SourceRefs: sourceRefs,
		Payload:    payload,
	})
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	sourceRefs[0] = "mutated-source"
	payload["apiKey"] = "mutated-secret"
	payload["nested"].(map[string]any)["safe"] = "after"

	events, err := store.Events(context.Background(), "session-copy", "turn-copy")
	if err != nil {
		t.Fatalf("Events() error = %v", err)
	}
	if len(events) != 1 {
		t.Fatalf("Events() length = %d; want 1", len(events))
	}
	encoded, err := json.Marshal([]any{result.Event, events[0]})
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, forbidden := range []string{"secret-canary", "mutated-secret", "mutated-source", "after"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("recorded rollout contains %q: %s", forbidden, encoded)
		}
	}
	marker, ok := events[0].Payload["apiKey"].(map[string]any)
	if !ok || marker["redacted"] != true || marker["sha256"] == "" {
		t.Fatalf("Payload[apiKey] = %#v; want hash-backed redaction marker", events[0].Payload["apiKey"])
	}
}

func TestRolloutRecorderDoesNotAliasStoreAndReturnedEvent(t *testing.T) {
	store := &retainingRolloutStore{}
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{Store: store})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	input := canonicalRecorderEvent("session-boundary-copy", "turn-boundary-copy", modeltrace.CanonicalRolloutKindCheckpoint)
	input.Payload = map[string]any{"nested": map[string]any{"status": "before"}}
	result, err := recorder.Append(context.Background(), input)
	if err != nil {
		t.Fatalf("Append() error = %v", err)
	}
	result.Event.Payload["nested"].(map[string]any)["status"] = "after"
	if got := store.event.Payload["nested"].(map[string]any)["status"]; got != "before" {
		t.Fatalf("store event was mutated through returned result: got %#v, want before", got)
	}
}

func TestRolloutRecorderFailClosedReturnsTypedAppendError(t *testing.T) {
	storeErr := errors.New("store unavailable")
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         rolloutStoreFunc(func(context.Context, modeltrace.CanonicalRolloutEvent) error { return storeErr }),
		FailurePolicy: RolloutFailurePolicyFailClosed,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	result, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-fail", "turn-fail", modeltrace.CanonicalRolloutKindToolResult))
	if err == nil || !errors.Is(err, storeErr) {
		t.Fatalf("Append() error = %v; want store error", err)
	}
	var typed *RolloutRecorderAppendError
	if !errors.As(err, &typed) {
		t.Fatalf("Append() error type = %T; want *RolloutRecorderAppendError", err)
	}
	if result.Status != RolloutRecordStatusFailed || result.ObservedError == nil {
		t.Fatalf("Append() result = %+v; want failed status with observed error", result)
	}
}

func TestRolloutRecorderDegradedPersistsTypedMarker(t *testing.T) {
	storeErr := errors.New("primary append unavailable")
	memory := NewMemoryRolloutStore()
	store := &failFirstRolloutStore{err: storeErr, fallback: memory}
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         store,
		FailurePolicy: RolloutFailurePolicyDegraded,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	result, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-degraded", "turn-degraded", modeltrace.CanonicalRolloutKindProviderResponse))
	if err != nil {
		t.Fatalf("Append() fatal error = %v; degraded policy must return a typed outcome", err)
	}
	if result.Status != RolloutRecordStatusDegraded || !errors.Is(result.ObservedError, storeErr) {
		t.Fatalf("Append() result = %+v; want degraded status with observable store error", result)
	}
	if result.Event.Kind != modeltrace.CanonicalRolloutKindRecorderDegraded || !result.MarkerPersisted {
		t.Fatalf("Append() marker = %+v; want persisted recorder_degraded marker", result)
	}
	events, readErr := memory.Events(context.Background(), "session-degraded", "turn-degraded")
	if readErr != nil {
		t.Fatalf("Events() error = %v", readErr)
	}
	if len(events) != 1 || events[0].EventID != result.Event.EventID || events[0].Sequence != 1 {
		t.Fatalf("stored degraded events = %+v; want returned sequence-1 marker", events)
	}
	if got := events[0].Payload["failedKind"]; got != modeltrace.CanonicalRolloutKindProviderResponse {
		t.Fatalf("degraded marker failedKind = %#v; want provider_response", got)
	}
}

func TestRolloutRecorderDegradedCannotMasqueradeAsSuccessWhenStoreIsUnwritable(t *testing.T) {
	storeErr := errors.New("store unavailable password=secret-canary")
	recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
		Store:         rolloutStoreFunc(func(context.Context, modeltrace.CanonicalRolloutEvent) error { return storeErr }),
		FailurePolicy: RolloutFailurePolicyDegraded,
	})
	if err != nil {
		t.Fatalf("NewRolloutRecorder() error = %v", err)
	}

	result, err := recorder.Append(context.Background(), canonicalRecorderEvent("session-unwritable", "turn-unwritable", modeltrace.CanonicalRolloutKindFinalFacts))
	if err != nil {
		t.Fatalf("Append() fatal error = %v; degraded state is carried by result", err)
	}
	if result.Status != RolloutRecordStatusDegraded || result.MarkerPersisted || result.ObservedError == nil {
		t.Fatalf("Append() result = %+v; want explicit unpersisted degraded state", result)
	}
	if strings.Contains(result.ObservedError.Error(), "secret-canary") {
		t.Fatalf("observable degraded error leaked store secret: %v", result.ObservedError)
	}
	if result.Event.Kind != modeltrace.CanonicalRolloutKindRecorderDegraded || result.Event.Sequence != 1 {
		t.Fatalf("Append() marker = %+v; want sequence-1 degraded marker", result.Event)
	}
	encoded, marshalErr := json.Marshal(result.Event)
	if marshalErr != nil {
		t.Fatalf("json.Marshal(marker) error = %v", marshalErr)
	}
	if strings.Contains(string(encoded), "secret-canary") {
		t.Fatalf("degraded marker persisted raw store error: %s", encoded)
	}
}

func TestMemoryRolloutStoreRejectsNonIncreasingSequence(t *testing.T) {
	store := NewMemoryRolloutStore()
	event := canonicalRecorderEvent("session-order", "turn-order", modeltrace.CanonicalRolloutKindCheckpoint)
	event.Sequence = 1
	frozen, err := modeltrace.FreezeCanonicalRolloutEvent(event)
	if err != nil {
		t.Fatalf("FreezeCanonicalRolloutEvent() error = %v", err)
	}
	if err := store.Append(context.Background(), frozen); err != nil {
		t.Fatalf("Append(first) error = %v", err)
	}
	if err := store.Append(context.Background(), frozen); !errors.Is(err, ErrRolloutSequenceNotIncreasing) {
		t.Fatalf("Append(duplicate) error = %v; want ErrRolloutSequenceNotIncreasing", err)
	}
}

func canonicalRecorderEvent(sessionID, turnID, kind string) modeltrace.CanonicalRolloutEvent {
	return modeltrace.CanonicalRolloutEvent{
		SessionID: sessionID,
		TurnID:    turnID,
		Kind:      kind,
		SourceRefs: []string{
			"turn-assembly:sha256:abc",
		},
		Payload: map[string]any{"fact": "value"},
	}
}

type rolloutStoreFunc func(context.Context, modeltrace.CanonicalRolloutEvent) error

func (fn rolloutStoreFunc) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	return fn(ctx, event)
}

type retainingRolloutStore struct {
	event modeltrace.CanonicalRolloutEvent
}

func (s *retainingRolloutStore) Append(_ context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.event = event
	return nil
}

type failFirstRolloutStore struct {
	mu       sync.Mutex
	failed   bool
	err      error
	fallback RolloutStore
}

func (s *failFirstRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return s.err
	}
	if s.fallback == nil {
		return fmt.Errorf("fallback store is nil")
	}
	return s.fallback.Append(ctx, event)
}

type kindFailingRolloutStore struct {
	mu       sync.Mutex
	failKind string
	memory   *MemoryRolloutStore
}

type failKindOnceRolloutStore struct {
	mu       sync.Mutex
	failKind string
	failed   bool
	memory   *MemoryRolloutStore
}

func newFailKindOnceRolloutStore(kind string) *failKindOnceRolloutStore {
	return &failKindOnceRolloutStore{failKind: kind, memory: NewMemoryRolloutStore()}
}

func (s *failKindOnceRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind && !s.failed {
		s.failed = true
		return errors.New("injected primary append failure")
	}
	return s.memory.Append(ctx, event)
}

func (s *failKindOnceRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

type failKindAndMarkerRolloutStore struct {
	mu       sync.Mutex
	failKind string
	failing  bool
	memory   *MemoryRolloutStore
}

func newFailKindAndMarkerRolloutStore(kind string) *failKindAndMarkerRolloutStore {
	return &failKindAndMarkerRolloutStore{failKind: kind, memory: NewMemoryRolloutStore()}
}

func (s *failKindAndMarkerRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind {
		s.failing = true
	}
	if s.failing {
		return errors.New("injected append and marker failure")
	}
	return s.memory.Append(ctx, event)
}

func (s *failKindAndMarkerRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

func newKindFailingRolloutStore(kind string) *kindFailingRolloutStore {
	return &kindFailingRolloutStore{failKind: kind, memory: NewMemoryRolloutStore()}
}

func (s *kindFailingRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if event.Kind == s.failKind {
		return errors.New("injected canonical rollout append failure")
	}
	return s.memory.Append(ctx, event)
}

func (s *kindFailingRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	return s.memory.Events(ctx, sessionID, turnID)
}

func canonicalRolloutKindsForTest(events []modeltrace.CanonicalRolloutEvent) []string {
	kinds := make([]string, 0, len(events))
	for _, event := range events {
		kinds = append(kinds, event.Kind)
	}
	return kinds
}

func countCanonicalRolloutKind(events []modeltrace.CanonicalRolloutEvent, kind string) int {
	count := 0
	for _, event := range events {
		if event.Kind == kind {
			count++
		}
	}
	return count
}

func equalCanonicalRolloutKinds(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range want {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}
