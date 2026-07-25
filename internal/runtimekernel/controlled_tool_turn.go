package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"reflect"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/runtimecontract"
	runtimestate "aiops-v2/internal/runtimekernel/state"
	"aiops-v2/internal/tooling"
)

const controlledToolTurnSchemaVersion = "aiops.controlled-tool-turn.v1"

// ControlledToolTurnRequest is a server-only admission contract for exactly
// one already-resolved tool call. The caller supplies trusted turn identity;
// RuntimeKernel still owns all approval, checkpoint, and ActionToken state.
type ControlledToolTurnRequest struct {
	Turn             TurnRequest                  `json:"-"`
	ToolCall         ToolCall                     `json:"-"`
	ExecutionContext tooling.ToolExecutionContext `json:"-"`
	Source           string                       `json:"-"`
	IdempotencyKey   string                       `json:"-"`
}

// ControlledToolTurnRecord is persisted in the TurnSnapshot so recovery can
// distinguish a server-owned exact-tool turn from a model turn. It contains no
// approval authority and is not accepted through TurnRequest.
type ControlledToolTurnRecord struct {
	SchemaVersion        string `json:"schemaVersion"`
	Source               string `json:"source"`
	IdempotencyKey       string `json:"idempotencyKey"`
	ToolCallID           string `json:"toolCallId"`
	ToolName             string `json:"toolName"`
	ArgumentsHash        string `json:"argumentsHash"`
	ExecutionContextHash string `json:"executionContextHash"`
}

func (r ControlledToolTurnRecord) Validate() error {
	if r.SchemaVersion != controlledToolTurnSchemaVersion {
		return fmt.Errorf("invalid controlled tool turn schema version")
	}
	for name, value := range map[string]string{
		"source": r.Source, "idempotency key": r.IdempotencyKey,
		"tool call id": r.ToolCallID, "tool name": r.ToolName,
	} {
		if err := validateControlledIdentifier(name, value); err != nil {
			return err
		}
	}
	if err := validateCanonicalRolloutRef(r.ArgumentsHash, "sha256:", "controlled arguments hash"); err != nil {
		return err
	}
	if err := validateCanonicalRolloutRef(r.ExecutionContextHash, "sha256:", "controlled execution context hash"); err != nil {
		return err
	}
	return nil
}

// ControlledToolApproval is the browser-safe binding returned to a trusted
// server adapter. The ActionToken intentionally remains only in persisted
// RuntimeKernel state.
type ControlledToolApproval struct {
	ApprovalID    string     `json:"approvalId"`
	SessionID     string     `json:"sessionId"`
	TurnID        string     `json:"turnId"`
	ToolCallID    string     `json:"toolCallId"`
	ToolName      string     `json:"toolName"`
	ArgumentsHash string     `json:"argumentsHash"`
	TargetRefs    []string   `json:"targetRefs,omitempty"`
	ExpiresAt     *time.Time `json:"expiresAt,omitempty"`
}

// ControlledToolTurnResult reports the durable turn boundary without exposing
// the server-owned ActionToken.
type ControlledToolTurnResult struct {
	TurnResult
	Approval   *ControlledToolApproval `json:"approval,omitempty"`
	ToolResult *ToolResult             `json:"toolResult,omitempty"`
}

var controlledTurnOperationStripes [64]sync.Mutex

func lockControlledTurnOperation(sessionID, turnID string) func() {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(turnID)))
	lock := &controlledTurnOperationStripes[int(h.Sum32())%len(controlledTurnOperationStripes)]
	lock.Lock()
	return lock.Unlock
}

// BeginControlledToolTurn creates or replays a persistent, exact one-tool
// approval turn. It always stops before execution; only ResumeTurn with a
// matching verified ActionToken can execute the tool.
func (k *RuntimeKernel) BeginControlledToolTurn(ctx context.Context, request ControlledToolTurnRequest) (ControlledToolTurnResult, error) {
	if err := validateControlledToolTurnRequest(request); err != nil {
		return ControlledToolTurnResult{}, err
	}
	if k == nil || k.sessions == nil || k.tools == nil {
		return ControlledToolTurnResult{}, fmt.Errorf("runtime kernel controlled tool dependencies are required")
	}
	unlock := lockControlledTurnOperation(request.Turn.SessionID, request.Turn.TurnID)
	defer unlock()

	session := k.sessions.GetOrCreate(request.Turn.SessionID, request.Turn.SessionType, request.Turn.Mode)
	record := controlledToolTurnRecord(request)
	if existing := controlledSnapshotByID(session, request.Turn.TurnID); existing != nil {
		if err := matchControlledToolTurnRecord(existing, record); err != nil {
			return ControlledToolTurnResult{}, err
		}
		return controlledToolTurnResultFromSnapshot(existing), nil
	}
	if current := session.CurrentTurn; current != nil && !current.Lifecycle.IsTerminal() {
		return ControlledToolTurnResult{}, fmt.Errorf("session %q already has active turn %q", session.ID, current.ID)
	}

	persistSessionTargetRequestState(session, request.Turn)
	if strings.TrimSpace(session.HostID) != strings.TrimSpace(request.ExecutionContext.HostID) {
		return ControlledToolTurnResult{}, fmt.Errorf("controlled execution context resolved host mismatch")
	}
	turnMetadata := applyDefaultRuntimePromptProfile(cloneTurnMetadata(request.Turn.Metadata), request.Turn.SessionType, session.HostID)
	if !reflect.DeepEqual(normalizeControlledMetadata(turnMetadata), normalizeControlledMetadata(request.Turn.Metadata)) {
		return ControlledToolTurnResult{}, fmt.Errorf("controlled turn metadata must already contain the resolved runtime profile")
	}
	request.Turn.Metadata = turnMetadata
	snapshot := k.ensureCurrentTurnSnapshot(session, request.Turn, request.Turn.TurnID)
	snapshot.ControlledToolTurn = &record
	snapshot.Metadata = cloneTurnMetadata(turnMetadata)
	snapshot.TurnArtifacts = cloneRuntimeTurnArtifacts(request.Turn.TurnArtifacts)
	snapshot.TrustedIdentity = cloneTrustedTurnIdentity(request.Turn.TrustedIdentity)

	dispatchTools, router, permissionHash, err := k.prepareControlledToolTurn(ctx, session, snapshot, request.Turn)
	if err != nil {
		return ControlledToolTurnResult{}, err
	}
	if !router.AllowsModelDispatch(request.ToolCall.Name) && !router.AllowsHostInternalDispatch(request.ToolCall.Name) {
		return ControlledToolTurnResult{}, fmt.Errorf("controlled tool %q is not dispatchable in the frozen tool surface", request.ToolCall.Name)
	}
	dispatcher := k.newIterationDispatcher(session, snapshot, 0, dispatchTools, router, dispatcherPermissionBinding{
		expected: permissionHash,
		current:  permissionHash,
	})
	desc, executor, found := dispatcher.lookup.LookupTool(request.ToolCall.Name)
	if !found {
		return ControlledToolTurnResult{}, fmt.Errorf("controlled tool %q is not registered", request.ToolCall.Name)
	}
	governance := desc.Metadata.EffectiveGovernance(0)
	if !governance.Mutating || !governance.RequiresApproval || !toolExecutionRequiresMutationSafety(desc, executor, request.ToolCall.Arguments) {
		return ControlledToolTurnResult{}, fmt.Errorf("controlled tool %q must be a mutation that requires approval", request.ToolCall.Name)
	}

	now := time.Now()
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, 0, nextCheckpointSequence(snapshot), "controlled_tool_proposed", TurnLifecycleRunning, TurnResumeStateNone)
	toolSurface := snapshot.ToolSurfaceSnapshot
	iteration := IterationState{
		ID: fmt.Sprintf("%s-iter-0", snapshot.ID), SessionID: session.ID, TurnID: snapshot.ID,
		Iteration: 0, Lifecycle: TurnLifecycleRunning, ResumeState: TurnResumeStateNone,
		ToolCalls:              []ToolCall{cloneControlledToolCall(request.ToolCall)},
		ToolSurfaceFingerprint: snapshot.StableToolFingerprint, ToolSurfaceSnapshot: toolSurface,
		VisibleTools: append([]string(nil), router.ModelVisibleTools...), Checkpoint: checkpoint,
		StartedAt: now, UpdatedAt: now,
	}
	snapshot.Iteration = 0
	snapshot.Iterations = []IterationState{iteration}
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	queueToolInvocation(snapshot, 0, request.ToolCall, desc.Metadata)
	appendAgentItem(snapshot, newAgentItem(toolCallItemID(snapshot.ID, request.ToolCall), agentstate.TurnItemTypeToolCall, agentstate.ItemStatusRunning, request.ToolCall.Name, request.ToolCall))
	if err := k.recordCanonicalControlledToolProposal(ctx, snapshot, request.ToolCall, request.Source); err != nil {
		return ControlledToolTurnResult{}, err
	}
	if err := k.recordCanonicalToolDispatch(ctx, snapshot, []ToolCall{request.ToolCall}); err != nil {
		return ControlledToolTurnResult{}, err
	}
	markToolInvocationRunning(snapshot, request.ToolCall.ID)
	k.persistTurnSnapshot(session, snapshot)

	dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForSnapshot(session.HostID, snapshot.Metadata, snapshot))
	dispatchResult := dispatcher.RequestApproval(dispatchCtx, session.ID, snapshot.ID, request.ToolCall, session.Type, session.Mode)
	appendResourceLockTraces(snapshot, dispatchResult.ResourceLocks)
	appendToolAttemptStates(snapshot, request.ToolCall.ID, dispatchResult.Attempts)
	if dispatchResult.Blocked && (dispatchResult.Outcome == "approval_needed" || dispatchResult.Outcome == "evidence_needed") {
		updateAgentItem(snapshot, toolCallItemID(snapshot.ID, request.ToolCall), agentstate.ItemStatusBlocked, dispatchResult.Reason)
		markToolInvocationBlocked(snapshot, request.ToolCall.ID)
		if err := k.markTurnBlocked(session, snapshot, request.ToolCall, dispatchResult); err != nil {
			return ControlledToolTurnResult{}, err
		}
		return controlledToolTurnResultFromSnapshot(snapshot), nil
	}

	failure := strings.TrimSpace(firstNonEmpty(dispatchResult.Error, dispatchResult.Reason))
	if failure == "" {
		failure = "controlled approval request crossed the no-execution boundary"
	}
	dispatchResult.Error = failure
	if dispatchResult.Outcome == "" {
		dispatchResult.Outcome = "controlled_approval_failed"
	}
	updateAgentItem(snapshot, toolCallItemID(snapshot.ID, request.ToolCall), agentstate.ItemStatusFailed, failure)
	markToolInvocationFailed(snapshot, request.ToolCall.ID, failureKindForDispatchResult(dispatchResult))
	if err := k.markTurnFailed(session, snapshot, request.ToolCall, dispatchResult); err != nil {
		return ControlledToolTurnResult{}, err
	}
	return ControlledToolTurnResult{}, fmt.Errorf("controlled approval request failed: %s", failure)
}

func validateControlledToolTurnRequest(request ControlledToolTurnRequest) error {
	if err := request.Turn.Validate(); err != nil {
		return fmt.Errorf("invalid controlled turn: %w", err)
	}
	if request.Turn.Mode != ModeExecute {
		return fmt.Errorf("controlled tool turn requires execute mode")
	}
	if request.Turn.SessionID == "" || request.Turn.TurnID == "" {
		return fmt.Errorf("controlled tool turn requires exact session and turn ids")
	}
	if request.Turn.TrustedIdentity == nil {
		return fmt.Errorf("controlled tool turn requires trusted identity")
	}
	for name, value := range map[string]string{
		"source": request.Source, "idempotency key": request.IdempotencyKey,
		"tool call id": request.ToolCall.ID, "tool name": request.ToolCall.Name,
	} {
		if err := validateControlledIdentifier(name, value); err != nil {
			return err
		}
	}
	if len(request.ToolCall.Arguments) == 0 || !json.Valid(request.ToolCall.Arguments) {
		return fmt.Errorf("controlled tool arguments must be valid JSON")
	}
	return validateControlledExecutionContext(request)
}

func validateControlledExecutionContext(request ControlledToolTurnRequest) error {
	execution := request.ExecutionContext
	identity := request.Turn.TrustedIdentity
	if execution.VerifiedApproval != nil || strings.TrimSpace(execution.ActionToken) != "" {
		return fmt.Errorf("controlled tool request cannot provide approval authority")
	}
	expected := map[string][2]string{
		"session":   {execution.SessionID, request.Turn.SessionID},
		"turn":      {execution.TurnID, request.Turn.TurnID},
		"tool call": {execution.ToolCallID, request.ToolCall.ID},
		"tool":      {execution.ToolName, request.ToolCall.Name},
		"host":      {execution.HostID, request.Turn.HostID},
		"tenant":    {execution.TenantID, identity.TenantID},
		"user":      {execution.UserID, identity.UserID},
	}
	for name, pair := range expected {
		if strings.TrimSpace(pair[0]) != strings.TrimSpace(pair[1]) {
			return fmt.Errorf("controlled execution context %s mismatch", name)
		}
	}
	if len(execution.OriginalInput) > 0 && toolArgumentsHash(execution.OriginalInput) != toolArgumentsHash(request.ToolCall.Arguments) {
		return fmt.Errorf("controlled execution context original input mismatch")
	}
	if len(execution.SanitizedInput) > 0 && toolArgumentsHash(execution.SanitizedInput) != toolArgumentsHash(request.ToolCall.Arguments) {
		return fmt.Errorf("controlled execution context sanitized input mismatch")
	}
	if !reflect.DeepEqual(normalizeControlledMetadata(execution.Metadata), normalizeControlledMetadata(request.Turn.Metadata)) {
		return fmt.Errorf("controlled execution context metadata mismatch")
	}
	if controlledArtifactsHash(execution.TurnArtifacts) != controlledArtifactsHash(request.Turn.TurnArtifacts) {
		return fmt.Errorf("controlled execution context artifacts mismatch")
	}
	return nil
}

func validateControlledIdentifier(name, value string) error {
	if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) || strings.ContainsAny(value, "\x00\r\n") {
		return fmt.Errorf("invalid controlled %s", name)
	}
	return nil
}

func normalizeControlledMetadata(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	return cloneTurnMetadata(input)
}

func controlledArtifactsHash(artifacts []runtimecontract.TurnArtifactEnvelope) string {
	values := make([]map[string]string, 0, len(artifacts))
	for _, artifact := range artifacts {
		values = append(values, map[string]string{"id": artifact.ID, "kind": artifact.Kind, "digest": artifact.Digest})
	}
	return agentassembly.StableHash("controlled-tool-artifacts", values)
}

func controlledToolTurnRecord(request ControlledToolTurnRequest) ControlledToolTurnRecord {
	executionHash := agentassembly.StableHash("controlled-tool-execution-context", map[string]any{
		"sessionId": request.ExecutionContext.SessionID, "turnId": request.ExecutionContext.TurnID,
		"tenantId": request.ExecutionContext.TenantID, "userId": request.ExecutionContext.UserID,
		"toolCallId": request.ExecutionContext.ToolCallID, "toolName": request.ExecutionContext.ToolName,
		"hostId": request.ExecutionContext.HostID, "incidentId": request.ExecutionContext.IncidentID,
		"metadata":      normalizeControlledMetadata(request.ExecutionContext.Metadata),
		"artifactsHash": controlledArtifactsHash(request.ExecutionContext.TurnArtifacts),
	})
	return ControlledToolTurnRecord{
		SchemaVersion: controlledToolTurnSchemaVersion, Source: request.Source, IdempotencyKey: request.IdempotencyKey,
		ToolCallID: request.ToolCall.ID, ToolName: request.ToolCall.Name,
		ArgumentsHash: toolArgumentsHash(request.ToolCall.Arguments), ExecutionContextHash: executionHash,
	}
}

func cloneControlledToolCall(call ToolCall) ToolCall {
	call.Arguments = append(json.RawMessage(nil), call.Arguments...)
	return call
}

func validateControlledToolTurnSnapshot(snapshot *TurnSnapshot) error {
	if snapshot == nil || snapshot.ControlledToolTurn == nil {
		return nil
	}
	if snapshot.SystemTurn != nil {
		return fmt.Errorf("controlled tool turn cannot also be a system turn")
	}
	record := snapshot.ControlledToolTurn
	if err := record.Validate(); err != nil {
		return fmt.Errorf("controlled tool turn: %w", err)
	}
	var calls []ToolCall
	var results int
	for _, iteration := range snapshot.Iterations {
		calls = append(calls, iteration.ToolCalls...)
		results += len(iteration.ToolResults)
	}
	if len(calls) != 1 || results > 1 {
		return fmt.Errorf("controlled tool turn must contain exactly one tool call and at most one result")
	}
	call := calls[0]
	if call.ID != record.ToolCallID || call.Name != record.ToolName || toolArgumentsHash(call.Arguments) != record.ArgumentsHash {
		return fmt.Errorf("controlled tool turn call binding mismatch")
	}
	for _, approval := range snapshot.PendingApprovals {
		if approval.ToolCallID != record.ToolCallID || approval.ToolName != record.ToolName || approval.ArgumentsHash != record.ArgumentsHash {
			return fmt.Errorf("controlled tool turn approval binding mismatch")
		}
	}
	return nil
}

func matchControlledToolTurnRecord(snapshot *TurnSnapshot, expected ControlledToolTurnRecord) error {
	if snapshot == nil || snapshot.ControlledToolTurn == nil || !reflect.DeepEqual(*snapshot.ControlledToolTurn, expected) {
		return fmt.Errorf("controlled tool turn idempotency binding mismatch")
	}
	return validateControlledToolTurnSnapshot(snapshot)
}

func controlledSnapshotByID(session *SessionState, turnID string) *TurnSnapshot {
	if session == nil {
		return nil
	}
	if session.CurrentTurn != nil && session.CurrentTurn.ID == turnID {
		return session.CurrentTurn
	}
	for i := len(session.TurnHistory) - 1; i >= 0; i-- {
		if session.TurnHistory[i].ID == turnID {
			return &session.TurnHistory[i]
		}
	}
	return nil
}

func controlledToolTurnResultFromSnapshot(snapshot *TurnSnapshot) ControlledToolTurnResult {
	if snapshot == nil {
		return ControlledToolTurnResult{}
	}
	result := ControlledToolTurnResult{TurnResult: TurnResult{
		SessionType: snapshot.SessionType, Mode: snapshot.Mode, SessionID: snapshot.SessionID, TurnID: snapshot.ID,
		ClientTurnID: snapshot.ClientTurnID, ClientMessageID: snapshot.ClientMessageID,
	}}
	switch snapshot.Lifecycle {
	case TurnLifecycleSuspended, TurnLifecycleResumable:
		result.Status = "blocked"
		result.Error = snapshot.Error
	case TurnLifecycleCompleted:
		result.Status = "completed"
		result.Output = snapshot.FinalOutput
	case TurnLifecycleFailed, TurnLifecycleCanceled:
		result.Status = "error"
		result.Error = snapshot.Error
	default:
		result.Status = "running"
	}
	if len(snapshot.PendingApprovals) == 1 {
		approval := snapshot.PendingApprovals[0]
		result.Approval = &ControlledToolApproval{
			ApprovalID: approval.ID, SessionID: approval.SessionID, TurnID: approval.TurnID,
			ToolCallID: approval.ToolCallID, ToolName: approval.ToolName, ArgumentsHash: approval.ArgumentsHash,
			TargetRefs: append([]string(nil), approval.TargetRefs...), ExpiresAt: cloneControlledTime(approval.ExpiresAt),
		}
	}
	if latest := latestIteration(snapshot); latest != nil && len(latest.ToolResults) == 1 {
		toolResult := latest.ToolResults[0]
		result.ToolResult = &toolResult
		if result.Output == "" {
			result.Output = toolResult.Content
		}
	}
	return result
}

func (k *RuntimeKernel) completeControlledToolTurn(ctx context.Context, session *SessionState, snapshot *TurnSnapshot) (TurnResult, error) {
	if session == nil || snapshot == nil || snapshot.ControlledToolTurn == nil {
		return TurnResult{}, fmt.Errorf("controlled tool turn state is required")
	}
	if err := validateControlledToolTurnSnapshot(snapshot); err != nil {
		return TurnResult{}, err
	}
	latest := latestIteration(snapshot)
	if latest == nil || len(latest.ToolResults) != 1 {
		return TurnResult{}, fmt.Errorf("controlled tool turn requires exactly one persisted tool result")
	}
	toolResult := latest.ToolResults[0]
	if toolResult.IsError || strings.TrimSpace(toolResult.Error) != "" {
		call := latest.ToolCalls[0]
		failure := strings.TrimSpace(firstNonEmpty(toolResult.Error, toolResult.Summary, toolResult.Content))
		if failure == "" {
			failure = "controlled tool returned an error"
		}
		updateAgentItem(snapshot, toolCallItemID(snapshot.ID, call), agentstate.ItemStatusFailed, failure)
		if err := k.markTurnFailed(session, snapshot, call, DispatchResult{
			ToolCallID: call.ID,
			Outcome:    "controlled_tool_failed",
			Error:      failure,
		}); err != nil {
			return TurnResult{}, err
		}
		return TurnResult{
			SessionType: session.Type, Mode: session.Mode, SessionID: session.ID, TurnID: snapshot.ID,
			ClientTurnID: snapshot.ClientTurnID, ClientMessageID: snapshot.ClientMessageID,
			Status: "error", Error: failure, Output: toolResult.Content,
		}, nil
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted); err != nil {
		return TurnResult{}, err
	}

	now := time.Now()
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), "controlled_tool_completed", TurnLifecycleCompleted, TurnResumeStateNone)
	candidate := *snapshot
	candidate.Lifecycle = TurnLifecycleCompleted
	candidate.ResumeState = TurnResumeStateNone
	candidate.PendingApprovals = nil
	candidate.PendingEvidence = nil
	candidate.LatestCheckpoint = checkpoint
	candidate.UpdatedAt = now
	candidate.CompletedAt = &now
	if err := k.recordCanonicalCheckpoint(ctx, &candidate, checkpoint); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return TurnResult{}, err
	}
	if err := k.recordCanonicalTransportProjection(ctx, &candidate, TurnLifecycleCompleted, TurnResumeStateNone, checkpoint.ID, nil); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return TurnResult{}, err
	}

	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleCompleted
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	snapshot.Error = ""
	snapshot.LatestCheckpoint = checkpoint
	snapshot.UpdatedAt = now
	snapshot.CompletedAt = &now
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	session.LatestCheckpoint = checkpoint
	latest.Lifecycle = TurnLifecycleCompleted
	latest.ResumeState = TurnResumeStateNone
	latest.PendingApprovals = nil
	latest.PendingEvidence = nil
	latest.Checkpoint = checkpoint
	latest.UpdatedAt = now
	latest.CompletedAt = &now
	call, _ := pendingToolCall(snapshot)
	if call.ID == "" && len(latest.ToolCalls) == 1 {
		call = latest.ToolCalls[0]
	}
	updateAgentItem(snapshot, toolCallItemID(snapshot.ID, call), agentstate.ItemStatusCompleted, call.Name)
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	k.persistTurnSnapshot(session, snapshot)
	k.emitRuntimeEvent(EventTurnComplete, session.ID, snapshot.ID, map[string]any{"source": "runtime_controlled", "toolCallId": call.ID})
	return TurnResult{
		SessionType: session.Type, Mode: session.Mode, SessionID: session.ID, TurnID: snapshot.ID,
		ClientTurnID: snapshot.ClientTurnID, ClientMessageID: snapshot.ClientMessageID,
		Status: "completed", Output: toolResult.Content,
	}, nil
}

func cloneControlledTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func (k *RuntimeKernel) prepareControlledToolTurn(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, request TurnRequest) ([]tooling.Tool, StepToolRouter, string, error) {
	agentKind := agentKindForSession(session)
	thresholds := k.contextBudgetPolicyForSession(session, agentKind).Thresholds()
	turnContext, err := BuildRuntimeTurnContext(request, session, RuntimeTurnContextOptions{
		Model:         modelrouter.ModelCapabilities{Provider: "runtime_controlled", Model: "server_exact"},
		ContextBudget: RuntimeContextBudgetSnapshot{MaxTokens: thresholds.MaxContextTokens, TargetTokens: thresholds.EffectiveContextWindow},
		Lineage:       RuntimeLineageSnapshot{AgentKind: string(agentKind)},
	})
	if err != nil {
		return nil, StepToolRouter{}, "", err
	}
	if turnContext.AdmissionError != "" {
		return nil, StepToolRouter{}, "", fmt.Errorf("controlled tool admission failed: %s", turnContext.AdmissionError)
	}
	snapshot.TaskDepth = depthProfileFromAdmissionFacts(request, turnContext.AdmissionFacts)
	compileContext := enrichCompileContext(k.compileContext(request.SessionType, request.Mode, request.Metadata), request.SessionType, session.HostID, request.Metadata, time.Now())
	compileContext = applyDepthProfileToCompileContext(compileContext, snapshot.TaskDepth, firstMetadataValue(request.Metadata, "reasoningEffort", "reasoning_effort"))
	compileContext = applyTurnPromptProfileMetadata(compileContext, request.Metadata)
	compileContext = k.appendContextArtifactToolsForExternalRefs(compileContext, session, snapshot)
	compileContext = appendRuntimeEnvironmentContextSection(compileContext, session)
	compileContext = appendSkillActivationContext(compileContext, session)
	compileContext = appendMCPInstructionContext(compileContext, session)
	compileContext.AssembledTools = filterToolsForContextMode(compileContext.AssembledTools, thresholds)
	compileContext.AssembledTools = filterHiddenTools(compileContext.AssembledTools, snapshot.HiddenTools)
	dispatchTools := append([]tooling.Tool(nil), compileContext.AssembledTools...)
	var surface tooling.ToolSurfacePolicySnapshot
	compileContext, surface = applyToolSurfacePolicyToCompileContext(compileContext, request.Mode, firstMetadataValue(request.Metadata, "profile", "toolProfile"), session)
	router, err := BuildStepToolRouter(StepToolRouterInput{
		Registered: toolNames(dispatchTools), ModelVisible: toolNames(compileContext.AssembledTools), Dispatchable: toolNames(compileContext.AssembledTools),
		HiddenReasons: hiddenReasonsFromToolSurfacePolicy(surface), PolicyHash: surface.Hash,
		SourceFingerprint: assembledToolFingerprint(k.tools, request.SessionType, request.Mode, compileContext.AssembledTools),
	})
	if err != nil {
		return nil, StepToolRouter{}, "", err
	}
	compileContext.VisibleToolFingerprint = router.Fingerprint
	turnContext.ToolPolicyHash = surface.Hash
	assembly, err := assembleRuntimeTurn(runtimeTurnAssemblyInput{
		TurnContext: turnContext, CompileContext: compileContext, ToolSurfacePolicy: surface,
		ToolSurfaceFingerprint: router.Fingerprint, ResourceBindings: request.ResourceBindings,
		RollbackPolicy: request.RollbackPolicy, Mode: request.Mode, MaxIterations: 1,
	})
	if err != nil {
		return nil, StepToolRouter{}, "", err
	}
	if err := k.captureReplayTurnAssembly(ctx, snapshot, assembly); err != nil {
		return nil, StepToolRouter{}, "", err
	}
	snapshot.TurnAssembly = assembly
	if err := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
		Kind:    modeltrace.CanonicalRolloutKindAdmission,
		Payload: map[string]any{"factsHash": turnContext.AdmissionFacts.Hash, "source": "runtime_controlled"},
	}); err != nil {
		return nil, StepToolRouter{}, "", err
	}
	if err := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
		Kind: modeltrace.CanonicalRolloutKindAssembly, TurnAssemblyHash: assembly.Hash,
		Payload: map[string]any{"schemaVersion": assembly.SchemaVersion, "source": "runtime_controlled"},
	}); err != nil {
		return nil, StepToolRouter{}, "", err
	}
	now := time.Now()
	surfaceRef := ToolSurfaceSnapshotRef{
		ID: fmt.Sprintf("toolsurface-%s-controlled", snapshot.ID), Fingerprint: router.Fingerprint,
		ToolNames: append([]string(nil), router.ModelVisibleTools...), StepRouter: &router,
		PolicySnapshotHash: surface.Hash, PolicySnapshot: &surface, CreatedAt: now,
	}
	snapshot.ToolSurfaceSnapshot = &surfaceRef
	snapshot.StableToolFingerprint = router.Fingerprint
	permissionHash := k.runtimePermissionSnapshotHash(assembly, surface)
	if permissionHash == "" {
		return nil, StepToolRouter{}, "", fmt.Errorf("controlled tool permission binding is empty")
	}
	k.persistTurnSnapshot(session, snapshot)
	return dispatchTools, router, permissionHash, nil
}

func (k *RuntimeKernel) recordCanonicalControlledToolProposal(ctx context.Context, snapshot *TurnSnapshot, call ToolCall, source string) error {
	fact := canonicalRolloutToolCall(call)
	return k.appendCanonicalRolloutEvent(ctx, snapshot, canonicalRolloutStepEvent(snapshot, modeltrace.CanonicalRolloutKindToolProposed, map[string]any{
		"callId": fact.CallID, "name": fact.Name, "argsHash": fact.ArgsHash,
		"source": "runtime_controlled", "controlledSource": strings.TrimSpace(source),
	}))
}
