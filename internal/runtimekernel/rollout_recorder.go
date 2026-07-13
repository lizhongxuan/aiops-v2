package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"aiops-v2/internal/agentstate"
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
	Duplicate       bool
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
	checkpointBy   map[rolloutTurnKey]map[string]modeltrace.CanonicalRolloutEvent
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
		checkpointBy:   make(map[rolloutTurnKey]map[string]modeltrace.CanonicalRolloutEvent),
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
	checkpointKey := canonicalRolloutCheckpointKey(event)
	if prior, ok := r.checkpointBy[key][checkpointKey]; checkpointKey != "" && ok {
		return RolloutAppendResult{Status: RolloutRecordStatusRecorded, Event: prior, Duplicate: true}, nil
	}

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
		if checkpointKey != "" {
			if r.checkpointBy[key] == nil {
				r.checkpointBy[key] = make(map[string]modeltrace.CanonicalRolloutEvent)
			}
			r.checkpointBy[key][checkpointKey] = frozen
		}
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

func canonicalRolloutCheckpointKey(event modeltrace.CanonicalRolloutEvent) string {
	if event.Kind != modeltrace.CanonicalRolloutKindCheckpoint || event.Payload == nil {
		return ""
	}
	id, _ := event.Payload["checkpointId"].(string)
	hash, _ := event.Payload["checkpointHash"].(string)
	id, hash = strings.TrimSpace(id), strings.TrimSpace(hash)
	if id == "" || hash == "" {
		return ""
	}
	return id + "\x00" + hash
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

type canonicalRolloutToolCallFact struct {
	CallID   string `json:"callId"`
	Name     string `json:"name"`
	ArgsHash string `json:"argsHash"`
}

func canonicalRolloutToolCall(call ToolCall) canonicalRolloutToolCallFact {
	return canonicalRolloutToolCallFact{
		CallID: strings.TrimSpace(call.ID),
		Name:   strings.TrimSpace(call.Name), ArgsHash: toolArgumentsHash(call.Arguments),
	}
}

func canonicalRolloutStepEvent(snapshot *TurnSnapshot, kind string, payload map[string]any) modeltrace.CanonicalRolloutEvent {
	event := modeltrace.CanonicalRolloutEvent{Kind: kind, Payload: payload}
	if snapshot == nil {
		return event
	}
	if snapshot.TurnAssembly != nil {
		event.TurnAssemblyHash = snapshot.TurnAssembly.Hash
	}
	if snapshot.LatestStepReference != nil {
		event.StepID = snapshot.LatestStepReference.StepHash
		event.StepContextHash = snapshot.LatestStepReference.StepHash
	}
	return event
}

func (k *RuntimeKernel) recordCanonicalToolProposals(ctx context.Context, snapshot *TurnSnapshot, calls []ToolCall) error {
	for _, call := range calls {
		fact := canonicalRolloutToolCall(call)
		if err := k.appendCanonicalRolloutEvent(ctx, snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindToolProposed, map[string]any{
			"callId": fact.CallID, "name": fact.Name, "argsHash": fact.ArgsHash,
		})); err != nil {
			return err
		}
	}
	return nil
}

func (k *RuntimeKernel) recordCanonicalToolDispatch(ctx context.Context, snapshot *TurnSnapshot, calls []ToolCall) error {
	if len(calls) == 0 {
		return nil
	}
	facts := make([]canonicalRolloutToolCallFact, 0, len(calls))
	for _, call := range calls {
		facts = append(facts, canonicalRolloutToolCall(call))
	}
	payload := map[string]any{"calls": facts}
	if len(facts) == 1 {
		payload = map[string]any{"callId": facts[0].CallID, "name": facts[0].Name, "argsHash": facts[0].ArgsHash}
	}
	return k.appendCanonicalRolloutEvent(ctx, snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindToolDispatched, payload))
}

func (k *RuntimeKernel) recordCanonicalToolResult(ctx context.Context, snapshot *TurnSnapshot, call ToolCall, result ToolResult, errorClass string) error {
	evidenceRefs := evidenceRefsFromToolResultContent(result.Content)
	sourceRefs := append([]string(nil), evidenceRefs...)
	for _, ref := range result.ExternalReferences {
		if id := strings.TrimSpace(ref.ID); id != "" {
			sourceRefs = append(sourceRefs, id)
		}
	}
	for _, ref := range result.References {
		if digest := strings.TrimSpace(ref.Digest); digest != "" {
			sourceRefs = append(sourceRefs, digest)
		}
	}
	payload := map[string]any{
		"callId": call.ID, "name": call.Name, "outcome": string(result.Outcome.Normalize()),
		"contentHash": promptContentHash(result.Content), "errorClass": strings.TrimSpace(errorClass),
		"evidenceRefs": evidenceRefs, "sourceRefs": compactStringList(sourceRefs),
	}
	event := canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindToolResult, payload)
	event.SourceRefs = append(event.SourceRefs, sourceRefs...)
	return k.appendCanonicalRolloutEvent(ctx, snapshot, event)
}

func (k *RuntimeKernel) recordCanonicalApprovalRequested(ctx context.Context, snapshot *TurnSnapshot, approval PendingApproval) error {
	if approval.ActionToken == nil {
		return fmt.Errorf("approval_requested requires validated ActionToken")
	}
	token := *approval.ActionToken
	if err := token.Validate(); err != nil {
		return fmt.Errorf("approval_requested ActionToken: %w", err)
	}
	if err := k.captureReplayApprovalActionToken(ctx, snapshot, token); err != nil {
		return err
	}
	payload := map[string]any{
		"approvalId": token.ApprovalID, "toolCallId": token.ToolCallID, "toolName": token.ToolName,
		"argsHash": token.ArgumentsHash, "targetRefs": append([]string(nil), token.TargetRefs...), "status": "pending",
		"actionTokenHash": token.Hash, "toolSurfaceFingerprint": token.ToolSurfaceFingerprint,
		"permissionHash": token.PermissionHash, "checkpointId": token.CheckpointID, "rollbackHash": token.RollbackHash,
	}
	return k.appendCanonicalRolloutEvent(ctx, snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindApprovalRequested, payload))
}

func (k *RuntimeKernel) recordCanonicalApprovalDecided(ctx context.Context, snapshot *TurnSnapshot, approval PendingApproval, decision, status string) error {
	payload := map[string]any{
		"approvalId": approval.ID, "toolCallId": approval.ToolCallID, "toolName": approval.ToolName,
		"decision": strings.TrimSpace(decision), "status": strings.TrimSpace(status),
	}
	return k.appendCanonicalRolloutEvent(ctx, snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindApprovalDecided, payload))
}

const canonicalTransportContractVersion = "aiops.transport.v2"

func canonicalRolloutFactHash(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal canonical rollout fact: %w", err)
	}
	hash := toolArgumentsHash(data)
	if strings.TrimSpace(hash) == "" {
		return "", fmt.Errorf("canonical rollout fact hash is empty")
	}
	return hash, nil
}

func canonicalRolloutStringHashes(values []string) ([]string, error) {
	values = compactStringList(values)
	sort.Strings(values)
	hashes := make([]string, 0, len(values))
	for _, value := range values {
		hash, err := canonicalRolloutFactHash(value)
		if err != nil {
			return nil, err
		}
		hashes = append(hashes, hash)
	}
	return hashes, nil
}

func canonicalFinalContractFactHash(contract *FinalContract) (string, error) {
	if contract == nil {
		return canonicalRolloutFactHash(nil)
	}
	typed := *contract
	typed.AnswerText = ""
	return canonicalRolloutFactHash(typed)
}

func (k *RuntimeKernel) recordCanonicalCheckpoint(ctx context.Context, snapshot *TurnSnapshot, checkpoint *CheckpointMetadata) error {
	if checkpoint == nil {
		return fmt.Errorf("canonical rollout checkpoint is required")
	}
	facts := struct {
		ID         string             `json:"checkpointId"`
		Kind       string             `json:"checkpointKind"`
		Lifecycle  TurnLifecycleState `json:"lifecycle"`
		Resume     TurnResumeState    `json:"resumeState"`
		Hash       string             `json:"checkpointHash"`
		Checkpoint string             `json:"checkpointRef"`
	}{
		ID: strings.TrimSpace(checkpoint.ID), Kind: strings.TrimSpace(checkpoint.Kind),
		Lifecycle: checkpoint.Lifecycle, Resume: checkpoint.ResumeState,
		Checkpoint: strings.TrimSpace(checkpoint.ID),
	}
	checkpointHash, err := canonicalRolloutFactHash(struct {
		ID        string             `json:"id"`
		Kind      string             `json:"kind"`
		Lifecycle TurnLifecycleState `json:"lifecycle"`
		Resume    TurnResumeState    `json:"resumeState"`
	}{facts.ID, facts.Kind, facts.Lifecycle, facts.Resume})
	if err != nil {
		return err
	}
	facts.Hash = checkpointHash
	event := canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindCheckpoint, map[string]any{
		"checkpointId": facts.ID, "checkpointKind": facts.Kind, "lifecycle": facts.Lifecycle,
		"resumeState": facts.Resume, "checkpointHash": facts.Hash, "checkpointRef": facts.Checkpoint,
	})
	event.SourceRefs = append(event.SourceRefs, facts.Checkpoint)
	return k.appendCanonicalRolloutEvent(ctx, snapshot, event)
}

func (k *RuntimeKernel) recordCanonicalFinalFacts(ctx context.Context, snapshot *TurnSnapshot, facts FinalRuntimeFacts, contract FinalContract) error {
	if err := k.captureReplayFinalFacts(ctx, snapshot, facts, contract); err != nil {
		return err
	}
	failureHashes, err := canonicalRolloutStringHashes(facts.FailureCodes)
	if err != nil {
		return err
	}
	factsHash, err := canonicalRolloutFactHash(facts)
	if err != nil {
		return err
	}
	contractHash, err := canonicalFinalContractFactHash(&contract)
	if err != nil {
		return err
	}
	payload := map[string]any{
		"completionStatus": facts.CompletionStatus, "postcheckStatus": facts.PostcheckStatus,
		"rollbackStatus": facts.RollbackStatus, "finalContractStatus": string(contract.Status),
		"finalRuntimeFactsHash": factsHash, "finalContractHash": contractHash,
		"evidenceRefCount": len(facts.EvidenceRefs), "checkedEvidenceCount": len(contract.CheckedEvidenceRefs),
		"uncheckedRequirementCount": len(contract.UncheckedRequirements), "failureCodeHashes": failureHashes,
	}
	return k.appendCanonicalRolloutEvent(ctx, snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindFinalFacts, payload))
}

func canonicalAgentItemsFactHash(snapshot *TurnSnapshot) (string, error) {
	type itemFact struct {
		ID     string `json:"id"`
		Type   string `json:"type"`
		Status string `json:"status"`
	}
	var facts []itemFact
	if snapshot != nil {
		candidate := *snapshot
		candidate.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
		syncPendingApprovalAgentItems(&candidate)
		facts = make([]itemFact, 0, len(candidate.AgentItems))
		for _, item := range candidate.AgentItems {
			facts = append(facts, itemFact{ID: item.ID, Type: string(item.Type), Status: string(item.Status)})
		}
	}
	return canonicalRolloutFactHash(facts)
}

func canonicalPendingIDs(snapshot *TurnSnapshot) (approvals, evidence, inputs []string) {
	if snapshot == nil {
		return nil, nil, nil
	}
	for _, item := range snapshot.PendingApprovals {
		approvals = append(approvals, item.ID)
	}
	for _, item := range snapshot.PendingEvidence {
		evidence = append(evidence, item.ID)
	}
	for _, item := range snapshot.PendingInputs {
		inputs = append(inputs, item.ID)
	}
	return compactStringList(approvals), compactStringList(evidence), compactStringList(inputs)
}

func (k *RuntimeKernel) recordCanonicalTransportProjection(
	ctx context.Context,
	snapshot *TurnSnapshot,
	lifecycle TurnLifecycleState,
	resume TurnResumeState,
	checkpointRef string,
	contract *FinalContract,
) error {
	payload, err := canonicalTransportProjectionPayload(snapshot, lifecycle, resume, checkpointRef, contract)
	if err != nil {
		return err
	}
	event := canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindTransportProjection, payload)
	if ref := strings.TrimSpace(checkpointRef); ref != "" {
		event.SourceRefs = append(event.SourceRefs, ref)
	}
	return k.appendCanonicalRolloutEvent(ctx, snapshot, event)
}

func canonicalTransportProjectionPayload(
	snapshot *TurnSnapshot,
	lifecycle TurnLifecycleState,
	resume TurnResumeState,
	checkpointRef string,
	contract *FinalContract,
) (map[string]any, error) {
	approvals, evidence, inputs := canonicalPendingIDs(snapshot)
	if lifecycle.IsTerminal() {
		approvals, evidence = nil, nil
	}
	contractHash, err := canonicalFinalContractFactHash(contract)
	if err != nil {
		return nil, err
	}
	agentItemsHash, err := canonicalAgentItemsFactHash(snapshot)
	if err != nil {
		return nil, err
	}
	approvalHash, err := canonicalRolloutFactHash(approvals)
	if err != nil {
		return nil, err
	}
	evidenceHash, err := canonicalRolloutFactHash(evidence)
	if err != nil {
		return nil, err
	}
	inputHash, err := canonicalRolloutFactHash(inputs)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{
		"lifecycle": lifecycle, "resumeState": resume, "checkpointRef": strings.TrimSpace(checkpointRef),
		"finalContractHash": contractHash, "agentItemsFactHash": agentItemsHash,
		"pendingApprovalIDsHash": approvalHash, "pendingEvidenceIDsHash": evidenceHash,
		"pendingInputIDsHash": inputHash, "transportContractVersion": canonicalTransportContractVersion,
	}
	projectionHash, err := canonicalRolloutFactHash(payload)
	if err != nil {
		return nil, err
	}
	payload["projectionInputHash"] = projectionHash
	return payload, nil
}

func (k *RuntimeKernel) recordCanonicalTerminalBoundary(
	ctx context.Context,
	snapshot *TurnSnapshot,
	checkpoint *CheckpointMetadata,
	status FinalContractStatus,
	failureCode string,
) error {
	facts := FinalRuntimeFacts{
		CompletionStatus: FinalCompletionStatusFailed,
		PostcheckStatus:  FinalPostcheckStatusNotRequired,
		RollbackStatus:   FinalRollbackStatusNotRequired,
		FailureCodes:     compactStringList([]string{failureCode}),
	}
	contract := BuildTerminalFinalContract("", status, facts.FailureCodes)
	if err := k.recordCanonicalCheckpoint(ctx, snapshot, checkpoint); err != nil {
		return err
	}
	if err := k.recordCanonicalFinalFacts(ctx, snapshot, facts, contract); err != nil {
		return err
	}
	return k.recordCanonicalTransportProjection(ctx, snapshot, checkpoint.Lifecycle, checkpoint.ResumeState, checkpoint.ID, &contract)
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
