package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"

	"aiops-v2/internal/modeltrace"
)

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
