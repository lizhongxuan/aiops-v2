package runtimekernel

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"

	"aiops-v2/internal/modeltrace"
)

var (
	ErrRolloutStoreRequired         = errors.New("rollout store is required")
	ErrRolloutReaderUnavailable     = errors.New("rollout reader is unavailable")
	ErrRolloutSequenceNotIncreasing = errors.New("rollout sequence must be strictly increasing")
)

// RolloutFailurePolicy controls whether a durable append failure stops the
// caller or is surfaced as an explicit degraded control fact.
type RolloutFailurePolicy string

const (
	RolloutFailurePolicyFailClosed RolloutFailurePolicy = "fail_closed"
	RolloutFailurePolicyDegraded   RolloutFailurePolicy = "degraded"
)

// RolloutRecordStatus prevents a degraded recorder from masquerading as a
// successful append when the backing store is unavailable.
type RolloutRecordStatus string

const (
	RolloutRecordStatusRecorded RolloutRecordStatus = "recorded"
	RolloutRecordStatusDegraded RolloutRecordStatus = "degraded"
	RolloutRecordStatusFailed   RolloutRecordStatus = "failed"
)

// RolloutStore appends one canonical event atomically. An error means the
// event was not appended; implementations must not report an error after a
// partial or successful write.
type RolloutStore interface {
	Append(context.Context, modeltrace.CanonicalRolloutEvent) error
}

// RolloutReader is the read side used by replay and debug projections.
type RolloutReader interface {
	Events(context.Context, string, string) ([]modeltrace.CanonicalRolloutEvent, error)
}

type RolloutRecorderConfig struct {
	Store         RolloutStore
	FailurePolicy RolloutFailurePolicy
}

type RolloutAppendResult struct {
	Status          RolloutRecordStatus
	Event           modeltrace.CanonicalRolloutEvent
	ObservedError   error
	MarkerPersisted bool
}

// RolloutRecorderAppendError is fatal under fail-closed policy. It deliberately
// omits payloads and store error text from its own message; callers can inspect
// the wrapped error without copying it into a persisted rollout event.
type RolloutRecorderAppendError struct {
	SessionID string
	TurnID    string
	Sequence  int64
	cause     error
}

func (e *RolloutRecorderAppendError) Error() string {
	if e == nil {
		return "canonical rollout append failed"
	}
	return fmt.Sprintf("canonical rollout append failed for session %q turn %q sequence %d", e.SessionID, e.TurnID, e.Sequence)
}

func (e *RolloutRecorderAppendError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

type rolloutTurnKey struct {
	sessionID string
	turnID    string
}

// RolloutRecorder serializes assignment and persistence so append order cannot
// race sequence order. Sequences are scoped to a (session, turn) pair.
type RolloutRecorder struct {
	mu             sync.Mutex
	store          RolloutStore
	failurePolicy  RolloutFailurePolicy
	lastSequenceBy map[rolloutTurnKey]int64
}

func NewRolloutRecorder(cfg RolloutRecorderConfig) (*RolloutRecorder, error) {
	if cfg.Store == nil {
		return nil, ErrRolloutStoreRequired
	}
	policy := cfg.FailurePolicy
	if policy == "" {
		policy = RolloutFailurePolicyFailClosed
	}
	if policy != RolloutFailurePolicyFailClosed && policy != RolloutFailurePolicyDegraded {
		return nil, fmt.Errorf("unsupported rollout failure policy %q", policy)
	}
	return &RolloutRecorder{
		store:          cfg.Store,
		failurePolicy:  policy,
		lastSequenceBy: make(map[rolloutTurnKey]int64),
	}, nil
}

func (r *RolloutRecorder) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) (RolloutAppendResult, error) {
	if r == nil || r.store == nil {
		err := ErrRolloutStoreRequired
		return RolloutAppendResult{Status: RolloutRecordStatusFailed, ObservedError: err}, err
	}
	key, err := canonicalRolloutTurnKey(event.SessionID, event.TurnID)
	if err != nil {
		return RolloutAppendResult{Status: RolloutRecordStatusFailed, ObservedError: err}, err
	}
	if strings.TrimSpace(event.Kind) == "" {
		err := errors.New("canonical rollout kind is required")
		return RolloutAppendResult{Status: RolloutRecordStatusFailed, ObservedError: err}, err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	sequence := r.lastSequenceBy[key] + 1
	event.Sequence = sequence
	frozen, err := modeltrace.FreezeCanonicalRolloutEvent(event)
	if err != nil {
		return RolloutAppendResult{Status: RolloutRecordStatusFailed, ObservedError: err}, err
	}
	storeEvent, err := modeltrace.FreezeCanonicalRolloutEvent(frozen)
	if err != nil {
		return RolloutAppendResult{Status: RolloutRecordStatusFailed, Event: frozen, ObservedError: err}, err
	}
	if err := r.store.Append(ctx, storeEvent); err == nil {
		r.lastSequenceBy[key] = sequence
		return RolloutAppendResult{Status: RolloutRecordStatusRecorded, Event: frozen}, nil
	} else if r.failurePolicy == RolloutFailurePolicyFailClosed {
		appendErr := newRolloutRecorderAppendError(frozen, err)
		return RolloutAppendResult{
			Status:        RolloutRecordStatusFailed,
			Event:         frozen,
			ObservedError: appendErr,
		}, appendErr
	} else {
		return r.appendDegradedMarker(ctx, key, frozen, err)
	}
}

// Events returns immutable copies for replay and tests. Recording stores remain
// write-only unless they explicitly implement RolloutReader.
func (r *RolloutRecorder) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	if r == nil || r.store == nil {
		return nil, ErrRolloutStoreRequired
	}
	reader, ok := r.store.(RolloutReader)
	if !ok {
		return nil, ErrRolloutReaderUnavailable
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	return reader.Events(ctx, sessionID, turnID)
}

func (r *RolloutRecorder) appendDegradedMarker(
	ctx context.Context,
	key rolloutTurnKey,
	failed modeltrace.CanonicalRolloutEvent,
	storeErr error,
) (RolloutAppendResult, error) {
	observedStoreErr := error(newRolloutRecorderAppendError(failed, storeErr))
	marker, freezeErr := modeltrace.FreezeCanonicalRolloutEvent(modeltrace.CanonicalRolloutEvent{
		Sequence:  failed.Sequence,
		SessionID: failed.SessionID,
		TurnID:    failed.TurnID,
		StepID:    failed.StepID,
		Kind:      modeltrace.CanonicalRolloutKindRecorderDegraded,
		SourceRefs: []string{
			failed.EventID,
		},
		Payload: map[string]any{
			"failedKind":  failed.Kind,
			"failureType": fmt.Sprintf("%T", storeErr),
			"status":      string(RolloutRecordStatusDegraded),
		},
	})
	if freezeErr != nil {
		combined := errors.Join(observedStoreErr, freezeErr)
		return RolloutAppendResult{Status: RolloutRecordStatusDegraded, ObservedError: combined}, nil
	}

	// Consume the attempted sequence even if the store remains unavailable. A
	// later successful append then exposes a sequence gap instead of rewriting
	// the missing degraded fact as if recording had remained healthy.
	r.lastSequenceBy[key] = failed.Sequence
	storeMarker, copyErr := modeltrace.FreezeCanonicalRolloutEvent(marker)
	if copyErr != nil {
		return RolloutAppendResult{
			Status:        RolloutRecordStatusDegraded,
			Event:         marker,
			ObservedError: errors.Join(observedStoreErr, copyErr),
		}, nil
	}
	markerErr := r.store.Append(ctx, storeMarker)
	observedErr := observedStoreErr
	markerPersisted := markerErr == nil
	if markerErr != nil {
		observedErr = errors.Join(observedStoreErr, newRolloutRecorderAppendError(marker, markerErr))
	}
	return RolloutAppendResult{
		Status:          RolloutRecordStatusDegraded,
		Event:           marker,
		ObservedError:   observedErr,
		MarkerPersisted: markerPersisted,
	}, nil
}

func newRolloutRecorderAppendError(event modeltrace.CanonicalRolloutEvent, cause error) *RolloutRecorderAppendError {
	return &RolloutRecorderAppendError{
		SessionID: event.SessionID,
		TurnID:    event.TurnID,
		Sequence:  event.Sequence,
		cause:     cause,
	}
}

func canonicalRolloutTurnKey(sessionID, turnID string) (rolloutTurnKey, error) {
	key := rolloutTurnKey{
		sessionID: strings.TrimSpace(sessionID),
		turnID:    strings.TrimSpace(turnID),
	}
	if key.sessionID == "" || key.turnID == "" {
		return rolloutTurnKey{}, errors.New("canonical rollout sessionId and turnId are required")
	}
	return key, nil
}

// MemoryRolloutStore is a concurrency-safe append-only store for tests and
// in-memory replay. It freezes every write and read to prevent aliasing.
type MemoryRolloutStore struct {
	mu     sync.RWMutex
	events map[rolloutTurnKey][]modeltrace.CanonicalRolloutEvent
}

func NewMemoryRolloutStore() *MemoryRolloutStore {
	return &MemoryRolloutStore{events: make(map[rolloutTurnKey][]modeltrace.CanonicalRolloutEvent)}
}

func (s *MemoryRolloutStore) Append(ctx context.Context, event modeltrace.CanonicalRolloutEvent) error {
	if err := contextError(ctx); err != nil {
		return err
	}
	frozen, err := modeltrace.FreezeCanonicalRolloutEvent(event)
	if err != nil {
		return err
	}
	key, err := canonicalRolloutTurnKey(frozen.SessionID, frozen.TurnID)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if s.events == nil {
		s.events = make(map[rolloutTurnKey][]modeltrace.CanonicalRolloutEvent)
	}
	events := s.events[key]
	if len(events) > 0 && frozen.Sequence <= events[len(events)-1].Sequence {
		return fmt.Errorf("%w: previous=%d next=%d", ErrRolloutSequenceNotIncreasing, events[len(events)-1].Sequence, frozen.Sequence)
	}
	s.events[key] = append(events, frozen)
	return nil
}

func (s *MemoryRolloutStore) Events(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	if err := contextError(ctx); err != nil {
		return nil, err
	}
	key, err := canonicalRolloutTurnKey(sessionID, turnID)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, nil
	}

	s.mu.RLock()
	defer s.mu.RUnlock()
	stored := s.events[key]
	out := make([]modeltrace.CanonicalRolloutEvent, 0, len(stored))
	for _, event := range stored {
		cloned, freezeErr := modeltrace.FreezeCanonicalRolloutEvent(event)
		if freezeErr != nil {
			return nil, freezeErr
		}
		out = append(out, cloned)
	}
	return out, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}
