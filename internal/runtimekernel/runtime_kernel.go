package runtimekernel

import (
	"context"
	"crypto/sha256"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/cloudwego/eino/components/tool"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/envcontext"
	evidencecore "aiops-v2/internal/evidence"
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/resourceio"
	"aiops-v2/internal/runtimecontract"
	runtimestate "aiops-v2/internal/runtimekernel/state"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/tooling"
)

//go:embed prompt_assets/web_search_policy.md
var webSearchPolicyPromptAsset string

// ---------------------------------------------------------------------------
// ToolAssemblySource is the interface that the RuntimeKernel uses to access
// assembled tools without importing the capability package directly (avoids
// circular imports since capability imports runtimekernel).
// ---------------------------------------------------------------------------

// ToolAssemblySource provides tool-assembly context and tool pool assembly.
// Implemented by unified tooling assemblers via a thin adapter.
type ToolAssemblySource interface {
	// CompileContext returns a CompileContext with assembled tools populated.
	CompileContext(session SessionType, mode Mode) promptcompiler.CompileContext

	// AssembleToolPool returns Eino tool.BaseTool instances for the given session/mode.
	// These can be directly passed to adk.ChatModelAgent's ToolsConfig.
	AssembleToolPool(session SessionType, mode Mode) []tool.BaseTool
}

type metadataToolAssemblySource interface {
	CompileContextWithMetadata(session SessionType, mode Mode, metadata map[string]string) []promptcompiler.Tool
	AssembleToolPoolWithMetadata(session SessionType, mode Mode, metadata map[string]string) []tool.BaseTool
}

type fullToolCatalogSource interface {
	AssembleToolsWithOptions(session, mode string, opts tooling.AssembleOptions) []tooling.Tool
}

type toolRefreshAwareSource interface {
	RefreshToken(session SessionType, mode Mode) string
}

// ---------------------------------------------------------------------------
// AgentManagerSource is the interface that the RuntimeKernel uses to access
// AgentManager without importing the agentmgr package directly (avoids
// circular imports since agentmgr imports projection which imports runtimekernel).
// ---------------------------------------------------------------------------

// WorkspaceAgentConfig holds the assembled configuration for a workspace
// PlanExecuteAgent, returned by AgentManagerSource.CreateWorkspaceAgent.
type WorkspaceAgentConfig struct {
	// PlannerConfig is the configuration for the Planner agent.
	PlannerConfig interface{}
	// MissionID is the workspace mission this agent serves.
	MissionID string
}

// AgentResult represents the execution result of an agent, used for projection.
type AgentResult struct {
	// AgentID is the unique identifier of the agent instance.
	AgentID string
	// HostID is the host this agent was bound to.
	HostID string
	// Status is the final status string (completed/failed/killed).
	Status string
	// Output is the execution summary text.
	Output string
	// Error contains error information if the agent failed.
	Error string
	// DurationMs is the total execution time in milliseconds.
	DurationMs int64
}

// AgentManagerSource provides agent lifecycle management for the RuntimeKernel.
// Implemented by an adapter wrapping *agentmgr.AgentManager.
type AgentManagerSource interface {
	// CreateWorkspaceAgent creates a workspace PlanExecuteAgent config via AgentFactory.
	CreateWorkspaceAgent(ctx context.Context, missionID string) error

	// SpawnAndRunPlanner spawns a planner agent, runs it, and returns the output.
	SpawnAndRunPlanner(ctx context.Context, missionID, sessionID, task string) (output string, err error)

	// CollectResults returns all terminal agent results for the given mission.
	CollectResults(missionID string) []AgentResult
}

// ---------------------------------------------------------------------------
// RuntimeKernel — the Eino ADK-based RuntimeKernel implementation.
// ---------------------------------------------------------------------------

// RuntimeKernel implements the RuntimeKernel interface using Eino ADK.
// It is the unique turn runtime kernel that manages Host and Workspace sessions.
type RuntimeKernel struct {
	tools              ToolAssemblySource
	compiler           promptcompiler.Compiler
	policy             *policyengine.Engine
	permissions        *permissions.Engine
	hooks              *hooks.Registry
	projector          EventEmitter
	modelRouter        *modelrouter.Router
	sessions           *SessionManager
	agentMgr           AgentManagerSource
	spanSource         SpanStreamSource // optional: span tree integration for conversation tracking
	observer           Observer
	resourceLockGate   ToolResourceLockGate
	compressor         *spanstream.ContextCompressor
	spillRepo          ToolResultSpillRepository
	artifactRepo       ContextArtifactRepository
	skillRegistry      *skills.Registry
	evidenceService    *evidencecore.Service
	rolloutRecorder    *RolloutRecorder
	replayArtifactSink ReplayArtifactSink
	featureFlags       func(context.Context) featureflag.Flags
	debugConfig        func(context.Context) RuntimeDebugConfig

	turnCancelMu       sync.Mutex
	inFlightTurnCancel map[string]context.CancelFunc
	pendingTurnCancel  map[string]string
}

// RuntimeKernelConfig holds the dependencies for creating an RuntimeKernel.
type RuntimeKernelConfig struct {
	ToolSource         ToolAssemblySource
	Compiler           promptcompiler.Compiler
	Policy             *policyengine.Engine
	Permissions        *permissions.Engine
	Hooks              *hooks.Registry
	Projector          EventEmitter
	ModelRouter        *modelrouter.Router
	AgentMgr           AgentManagerSource
	Sessions           *SessionManager
	SessionRepo        SessionRepository
	SpanSource         SpanStreamSource // optional: if nil, span tracking is disabled
	Observer           Observer
	ResourceLockGate   ToolResourceLockGate
	Compressor         *spanstream.ContextCompressor
	SpillRepo          ToolResultSpillRepository
	ArtifactRepo       ContextArtifactRepository
	SkillRegistry      *skills.Registry
	EvidenceService    *evidencecore.Service
	RolloutRecorder    *RolloutRecorder
	ReplayArtifactSink ReplayArtifactSink
	FeatureFlags       func(context.Context) featureflag.Flags
	DebugConfig        func(context.Context) RuntimeDebugConfig
}

type RuntimeDebugConfig struct {
	ModelInputTrace      bool
	ModelInputTraceRoot  string
	FinalState           bool
	TransportProjection  bool
	TranscriptProjection bool
}

func DefaultRuntimeDebugConfig() RuntimeDebugConfig {
	return RuntimeDebugConfig{
		ModelInputTrace:     true,
		ModelInputTraceRoot: modeltrace.DefaultRootDir(""),
	}
}

// NewRuntimeKernel creates a new RuntimeKernel with the given dependencies.
func NewRuntimeKernel(cfg RuntimeKernelConfig) *RuntimeKernel {
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = NewSessionManager(cfg.SessionRepo)
	}
	observer := cfg.Observer
	if observer == nil {
		observer = NoopObserver{}
	}
	featureFlags := cfg.FeatureFlags
	if featureFlags == nil {
		featureFlags = func(context.Context) featureflag.Flags { return featureflag.Default() }
	}
	debugConfig := cfg.DebugConfig
	if debugConfig == nil {
		debugConfig = func(context.Context) RuntimeDebugConfig { return DefaultRuntimeDebugConfig() }
	}
	rolloutRecorder := cfg.RolloutRecorder
	if rolloutRecorder == nil {
		var err error
		rolloutRecorder, err = NewRolloutRecorder(RolloutRecorderConfig{
			Store:         NewMemoryRolloutStore(),
			FailurePolicy: RolloutFailurePolicyFailClosed,
		})
		if err != nil {
			panic("create default canonical rollout recorder: " + err.Error())
		}
	}
	return &RuntimeKernel{
		tools:              cfg.ToolSource,
		compiler:           cfg.Compiler,
		policy:             cfg.Policy,
		permissions:        cfg.Permissions,
		hooks:              cfg.Hooks,
		projector:          cfg.Projector,
		modelRouter:        cfg.ModelRouter,
		sessions:           sessions,
		agentMgr:           cfg.AgentMgr,
		spanSource:         cfg.SpanSource,
		observer:           observer,
		resourceLockGate:   cfg.ResourceLockGate,
		compressor:         cfg.Compressor,
		spillRepo:          cfg.SpillRepo,
		artifactRepo:       cfg.ArtifactRepo,
		skillRegistry:      cfg.SkillRegistry,
		evidenceService:    cfg.EvidenceService,
		rolloutRecorder:    rolloutRecorder,
		replayArtifactSink: cfg.ReplayArtifactSink,
		featureFlags:       featureFlags,
		debugConfig:        debugConfig,
		inFlightTurnCancel: make(map[string]context.CancelFunc),
		pendingTurnCancel:  make(map[string]string),
	}
}

// CanonicalRolloutEvents exposes immutable replay facts without routing them
// through turn metadata, external references, or model input.
func (k *RuntimeKernel) CanonicalRolloutEvents(ctx context.Context, sessionID, turnID string) ([]modeltrace.CanonicalRolloutEvent, error) {
	if k == nil || k.rolloutRecorder == nil {
		return nil, ErrRolloutStoreRequired
	}
	return k.rolloutRecorder.Events(ctx, sessionID, turnID)
}

func (k *RuntimeKernel) appendCanonicalRolloutEvent(
	ctx context.Context,
	snapshot *TurnSnapshot,
	event modeltrace.CanonicalRolloutEvent,
) error {
	if k == nil || snapshot == nil {
		return ErrRolloutStoreRequired
	}
	if k.rolloutRecorder == nil {
		recorder, err := NewRolloutRecorder(RolloutRecorderConfig{
			Store: NewMemoryRolloutStore(), FailurePolicy: RolloutFailurePolicyFailClosed,
		})
		if err != nil {
			return err
		}
		k.rolloutRecorder = recorder
	}
	event.SessionID = snapshot.SessionID
	event.TurnID = snapshot.ID
	if head := snapshot.CanonicalRolloutHead; head != nil {
		event.SourceRefs = append(event.SourceRefs, head.EventID)
	}
	// Audit persistence must outlive an execution cancellation so the terminal
	// provider outcome remains observable instead of being misclassified as a
	// recorder failure. Store failure policy still governs the append itself.
	result, err := k.rolloutRecorder.Append(context.WithoutCancel(ctx), event)
	if err != nil {
		return err
	}
	if result.Duplicate {
		return nil
	}
	if result.Status == RolloutRecordStatusDegraded && !result.MarkerPersisted {
		return fmt.Errorf("canonical rollout degraded marker was not persisted")
	}
	if result.Status != RolloutRecordStatusRecorded && result.Status != RolloutRecordStatusDegraded {
		return fmt.Errorf("canonical rollout append failed closed")
	}
	if err := result.Event.Validate(); err != nil {
		return fmt.Errorf("canonical rollout result validation failed: %w", err)
	}
	head := CanonicalRolloutHeadRef{
		SchemaVersion: result.Event.SchemaVersion,
		EventID:       result.Event.EventID,
		Hash:          result.Event.Hash,
		Sequence:      result.Event.Sequence,
		Status:        result.Status,
	}
	if err := head.Validate(); err != nil {
		return err
	}
	snapshot.CanonicalRolloutHead = &head
	return nil
}

func (k *RuntimeKernel) runtimeFeatureFlags(ctx context.Context) featureflag.Flags {
	if k == nil || k.featureFlags == nil {
		return featureflag.Default()
	}
	return k.featureFlags(ctx).Clone()
}

func (k *RuntimeKernel) runtimeDebugConfig(ctx context.Context) RuntimeDebugConfig {
	if k == nil || k.debugConfig == nil {
		return DefaultRuntimeDebugConfig()
	}
	cfg := k.debugConfig(ctx)
	if strings.TrimSpace(cfg.ModelInputTraceRoot) == "" {
		cfg.ModelInputTraceRoot = modeltrace.DefaultRootDir("")
	}
	return cfg
}

func (k *RuntimeKernel) runtimeObserver() Observer {
	if k == nil || k.observer == nil {
		return NoopObserver{}
	}
	return k.observer
}

func (k *RuntimeKernel) observeRuntimeStage(ctx context.Context, sessionID, turnID string, iteration int, stage string) {
	_, span := k.runtimeObserver().StartStage(ctx, StageSpanAttrs{
		SessionID: sessionID,
		TurnID:    turnID,
		Stage:     stage,
		Iteration: iteration,
	})
	if span != nil {
		span.End()
	}
}

func turnExecutionKey(sessionID, turnID string) string {
	return strings.TrimSpace(sessionID) + ":" + strings.TrimSpace(turnID)
}

func (k *RuntimeKernel) registerTurnExecution(sessionID, turnID string, cancel context.CancelFunc) string {
	if k == nil || cancel == nil {
		return ""
	}
	key := turnExecutionKey(sessionID, turnID)
	k.turnCancelMu.Lock()
	defer k.turnCancelMu.Unlock()
	k.inFlightTurnCancel[key] = cancel
	reason := strings.TrimSpace(k.pendingTurnCancel[key])
	delete(k.pendingTurnCancel, key)
	return reason
}

func (k *RuntimeKernel) releaseTurnExecution(sessionID, turnID string, cancel context.CancelFunc) {
	if k == nil {
		return
	}
	key := turnExecutionKey(sessionID, turnID)
	k.turnCancelMu.Lock()
	defer k.turnCancelMu.Unlock()
	if k.inFlightTurnCancel[key] == nil {
		return
	}
	delete(k.inFlightTurnCancel, key)
}

func (k *RuntimeKernel) requestTurnCancel(sessionID, turnID, reason string) bool {
	if k == nil {
		return false
	}
	key := turnExecutionKey(sessionID, turnID)
	k.turnCancelMu.Lock()
	cancel := k.inFlightTurnCancel[key]
	if cancel == nil {
		k.pendingTurnCancel[key] = strings.TrimSpace(reason)
	}
	k.turnCancelMu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func validateTurnLifecycleTransition(snapshot *TurnSnapshot, transition runtimestate.TurnTransitionType, to TurnLifecycleState) error {
	if snapshot == nil {
		return fmt.Errorf("turn snapshot is required")
	}
	fromState, ok := turnLifecycleStateForValidator(snapshot.Lifecycle)
	if !ok {
		return fmt.Errorf("unsupported turn lifecycle %q", snapshot.Lifecycle)
	}
	toState, ok := turnLifecycleStateForValidator(to)
	if !ok {
		return fmt.Errorf("unsupported turn lifecycle %q", to)
	}
	return runtimestate.NewValidator().Validate(fromState, transition, toState)
}

func turnLifecycleStateForValidator(lifecycle TurnLifecycleState) (runtimestate.TurnLifecycle, bool) {
	switch lifecycle {
	case TurnLifecyclePending:
		return runtimestate.LifecycleCreated, true
	case TurnLifecycleRunning:
		return runtimestate.LifecycleRunning, true
	case TurnLifecycleSuspended, TurnLifecycleResumable:
		return runtimestate.LifecycleBlocked, true
	case TurnLifecycleCompleted:
		return runtimestate.LifecycleCompleted, true
	case TurnLifecycleFailed:
		return runtimestate.LifecycleFailed, true
	case TurnLifecycleCanceled:
		return runtimestate.LifecycleCancelled, true
	default:
		return "", false
	}
}

func (k *RuntimeKernel) markTurnCanceled(session *SessionState, snapshot *TurnSnapshot, reason string) bool {
	ok, _ := k.markTurnCanceledRecorded(context.Background(), session, snapshot, reason)
	return ok
}

func (k *RuntimeKernel) markTurnCanceledRecorded(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, reason string) (bool, error) {
	if session == nil || snapshot == nil {
		return false, nil
	}
	if snapshot.Lifecycle == TurnLifecycleCanceled {
		return false, nil
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCancelled, TurnLifecycleCanceled); err != nil {
		return false, nil
	}
	now := time.Now()
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), "turn_cancelled", TurnLifecycleCanceled, TurnResumeStateNone)
	candidate := *snapshot
	candidate.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
	candidate.Lifecycle = TurnLifecycleCanceled
	candidate.ResumeState = TurnResumeStateNone
	candidate.PendingApprovals = nil
	candidate.PendingEvidence = nil
	candidate.LatestCheckpoint = checkpoint
	cancelActiveAgentItems(&candidate)
	if err := k.recordCanonicalTerminalBoundary(context.WithoutCancel(ctx), &candidate, checkpoint, FinalContractStatusCancelled, "turn_cancelled"); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return false, err
	}

	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleCanceled
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = strings.TrimSpace(reason)
	snapshot.UpdatedAt = now
	snapshot.CompletedAt = &now
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	cancelActiveAgentItems(snapshot)
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleCanceled
		last.ResumeState = TurnResumeStateNone
		last.UpdatedAt = now
		last.CompletedAt = &now
	}
	appendAbortedToolResultsForCancel(session, snapshot, reason, now)
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	k.persistTurnSnapshot(session, snapshot)
	if k.projector != nil {
		k.projector.Emit(LifecycleEvent{
			Type:      EventTurnAborted,
			SessionID: session.ID,
			TurnID:    snapshot.ID,
			Timestamp: now,
		})
	}
	return true, nil
}

func appendAbortedToolResultsForCancel(session *SessionState, snapshot *TurnSnapshot, reason string, now time.Time) {
	if session == nil || snapshot == nil {
		return
	}
	iter := latestIteration(snapshot)
	if iter == nil {
		return
	}
	normalizedReason := strings.TrimSpace(reason)
	if normalizedReason == "" {
		normalizedReason = "user_cancelled"
	}
	for idx := range iter.ToolInvocations {
		inv := &iter.ToolInvocations[idx]
		if inv.Status != ToolInvocationRunning && inv.Status != ToolInvocationQueued {
			continue
		}
		toolCallID := strings.TrimSpace(inv.ToolCallID)
		if toolCallID == "" || toolResultExists(iter.ToolResults, toolCallID) {
			continue
		}
		payload, _ := json.Marshal(map[string]any{
			"schemaVersion":        "aiops.tool_aborted/v1",
			"reason":               normalizedReason,
			"partialExecutionRisk": true,
		})
		result := ToolResult{
			ToolCallID: toolCallID,
			Content:    string(payload),
			Summary:    "tool aborted",
		}
		iter.ToolResults = append(iter.ToolResults, result)
		inv.Status = ToolInvocationFailed
		inv.FailureKind = normalizedReason
		inv.UpdatedAt = now
		inv.CompletedAt = &now
		session.Messages = append(session.Messages, Message{
			ID:         fmt.Sprintf("msg-%d", now.UnixNano()+int64(idx)),
			Role:       "tool",
			Content:    result.Content,
			ToolResult: &result,
			Timestamp:  now,
			Metadata: map[string]string{
				"abort.reason": normalizedReason,
			},
		})
	}
}

func toolResultExists(results []ToolResult, toolCallID string) bool {
	for _, result := range results {
		if strings.TrimSpace(result.ToolCallID) == toolCallID {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// RunTurn — the main turn execution pipeline.
// ---------------------------------------------------------------------------

// RunTurn executes a complete turn pipeline:
//  1. Receive TurnRequest
//  2. Get/create session
//  3. Assemble context (messages + trimming)
//  4. Compile prompt via PromptCompiler
//  5. Get assembled tools from ToolAssemblySource
//  6. Get model from ModelRouter
//  7. Create adk.ChatModelAgent with model + tools + instruction
//  8. Execute via adk.Runner.Run()
//  9. Process AgentEvents via callback → Projection
//  10. Final gate check via PolicyEngine.CompletionPolicy
//  11. Return TurnResult
func (k *RuntimeKernel) RunTurn(ctx context.Context, req TurnRequest) (result TurnResult, err error) {
	var observedTurnSpan ObservedSpan
	observedTurnDone := false
	finishObservedTurn := func(status, message string) {
		if observedTurnSpan == nil || observedTurnDone {
			return
		}
		attrs := map[string]any{"turn.status": status}
		if strings.TrimSpace(message) != "" {
			attrs["error"] = message
		}
		observedTurnSpan.SetAttributes(attrs)
		observedTurnSpan.SetStatus(status, message)
		observedTurnSpan.End()
		observedTurnDone = true
	}
	defer func() {
		status, message := observedTurnStatus(result, err)
		finishObservedTurn(status, message)
	}()

	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			result = TurnResult{
				SessionType:     req.SessionType,
				Mode:            req.Mode,
				SessionID:       req.SessionID,
				TurnID:          req.TurnID,
				ClientTurnID:    req.ClientTurnID,
				ClientMessageID: req.ClientMessageID,
				Status:          "error",
				Error:           fmt.Sprintf("panic recovered: %v", r),
			}
			err = nil
		}
	}()

	// Validate request
	if valErr := req.Validate(); valErr != nil {
		return TurnResult{}, fmt.Errorf("invalid turn request: %w", valErr)
	}

	// Assign turn ID if not provided
	turnID := req.TurnID
	if turnID == "" {
		turnID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}

	// Create root span for this turn (if span source is configured)
	var turnSpanID string
	if k.spanSource != nil {
		turnSpanID = k.spanSource.StartTurnSpan(turnID, req.Input)
	}

	// Step 1: Get/create session
	session := k.sessions.GetOrCreate(req.SessionID, req.SessionType, req.Mode)
	if activeTurn, ok := runningRegularTurnForPendingInput(session, turnID); ok {
		pending := appendPendingInputToActiveTurn(session, activeTurn, req)
		k.persistTurnSnapshot(session, activeTurn)
		return TurnResult{
			SessionType:     req.SessionType,
			Mode:            req.Mode,
			SessionID:       session.ID,
			TurnID:          activeTurn.ID,
			ClientTurnID:    pending.ClientTurnID,
			ClientMessageID: pending.ClientMessageID,
			Status:          "pending_input",
		}, nil
	}
	req.HostID = strings.TrimSpace(req.HostID)
	persistSessionTargetRequestState(session, req)
	runCtx, runCancel := context.WithCancel(ctx)
	if observedCtx, span := k.runtimeObserver().StartTurn(runCtx, TurnSpanAttrs{
		SessionID:       session.ID,
		TurnID:          turnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionType:     string(req.SessionType),
		Mode:            string(req.Mode),
		HostID:          req.HostID,
		Input:           req.Input,
	}); observedCtx != nil {
		runCtx = observedCtx
		observedTurnSpan = span
	} else {
		observedTurnSpan = span
	}
	if observedTurnSpan != nil {
		if carrier := observedTurnSpan.TraceContext(); len(carrier) > 0 {
			snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
			snapshot.TraceContext = copyTraceContextCarrier(carrier)
			k.persistTurnSnapshot(session, snapshot)
		}
	}
	pendingCancelReason := k.registerTurnExecution(session.ID, turnID, runCancel)
	defer func() {
		k.releaseTurnExecution(session.ID, turnID, runCancel)
		runCancel()
	}()
	k.emitRuntimeEvent(EventTurnStarted, session.ID, turnID, map[string]any{
		"clientTurnId":    req.ClientTurnID,
		"clientMessageId": req.ClientMessageID,
		"hostId":          req.HostID,
	})
	preTurnEvent, err := k.runTurnHook(runCtx, hooks.StagePreTurn, session, req, turnID, "", nil)
	if err != nil {
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, "pre_turn: "+err.Error())
		}
		return TurnResult{}, fmt.Errorf("pre_turn: %w", err)
	}
	if preTurnEvent.UpdatedInput != "" {
		req.Input = preTurnEvent.UpdatedInput
	}

	// Step 2: Assemble context (add user message, trim if needed)
	if req.Input != "" {
		messageID := strings.TrimSpace(req.ClientMessageID)
		if messageID == "" {
			messageID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
		}
		msg := Message{
			ID:              messageID,
			ClientMessageID: req.ClientMessageID,
			ClientTurnID:    req.ClientTurnID,
			Role:            "user",
			Content:         req.Input,
			Timestamp:       time.Now(),
		}
		session.Messages = append(session.Messages, msg)
		updateRuntimeEnvironmentContext(session, req, msg.Timestamp)
		snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
		appendAgentItem(snapshot, newAgentItem(
			turnID+"-user-message",
			agentstate.TurnItemTypeUserMessage,
			agentstate.ItemStatusCompleted,
			truncateString(req.Input, 240),
			map[string]string{
				"messageId": msg.ID,
				"prompt":    req.Input,
			},
		))
		if evidenceItem, ok := userEvidenceAgentItemFromMetadata(turnID, req.Metadata); ok {
			appendAgentItem(snapshot, evidenceItem)
		}
		k.persistTurnSnapshot(session, snapshot)
	}
	recomputeContextWindow(&session.Context, session.Messages)
	if pendingCancelReason != "" {
		snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
		if _, cancelErr := k.markTurnCanceledRecorded(ctx, session, snapshot, pendingCancelReason); cancelErr != nil {
			return TurnResult{}, cancelErr
		}
		return TurnResult{
			SessionType:     req.SessionType,
			Mode:            req.Mode,
			SessionID:       session.ID,
			TurnID:          turnID,
			ClientTurnID:    req.ClientTurnID,
			ClientMessageID: req.ClientMessageID,
			Status:          "cancelled",
		}, nil
	}

	// Step 3: Compile prompt via PromptCompiler
	// Step 3: Get model from ModelRouter
	agentKind := modelrouter.AgentKindWorker
	if req.SessionType == SessionTypeWorkspace {
		agentKind = modelrouter.AgentKindPlanner
	}
	chatModel, modelErr := k.modelRouter.GetModel(agentKind, modelrouter.ProviderConfig{})
	if modelErr != nil {
		snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
		appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, -1), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, modelErr.Error(), nil))
		if transitionErr := k.markTurnFailedFromError(session, snapshot, modelErr, "get_model_failed"); transitionErr != nil {
			return TurnResult{}, transitionErr
		}
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, modelErr.Error())
		}
		return TurnResult{}, fmt.Errorf("get model: %w", modelErr)
	}

	// Step 4: Execute the shared iteration loop.
	// Host and workspace sessions now converge here; workspace keeps its
	// planner model kind, but it no longer has a separate runtime loop.
	var agentOutput string
	var runErr error
	var blocked *TurnResult
	agentOutput, blocked, runErr = k.runHostIterationLoop(runCtx, chatModel, agentKind, req, session, turnID, preTurnEvent, turnSpanID)
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
			if _, cancelErr := k.markTurnCanceledRecorded(ctx, session, snapshot, "user stop"); cancelErr != nil {
				return TurnResult{}, cancelErr
			}
			return TurnResult{
				SessionType:     req.SessionType,
				Mode:            req.Mode,
				SessionID:       session.ID,
				TurnID:          turnID,
				ClientTurnID:    req.ClientTurnID,
				ClientMessageID: req.ClientMessageID,
				Status:          "cancelled",
			}, nil
		}
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, runErr.Error())
		}
		if snapshot := session.CurrentTurn; snapshot != nil && snapshot.ID == turnID {
			if transitionErr := k.markTurnFailedFromError(session, snapshot, runErr, inferTurnFailureCheckpointKind(snapshot)); transitionErr != nil {
				return TurnResult{}, transitionErr
			}
		}
		return TurnResult{}, fmt.Errorf("run agent: %w", runErr)
	}
	completionPolicyBlocked := blocked != nil && session.CurrentTurn != nil && session.CurrentTurn.Lifecycle == TurnLifecycleCompleted
	if blocked != nil && !completionPolicyBlocked {
		return *blocked, nil
	}

	// Step 5: Emit projection events
	turnCompletePayload, _ := json.Marshal(map[string]any{
		"watchPaths": append([]string(nil), preTurnEvent.WatchPaths...),
	})
	k.projector.Emit(LifecycleEvent{
		Type:      EventTurnComplete,
		SessionID: session.ID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   turnCompletePayload,
	})
	if completionPolicyBlocked {
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, "blocked: "+blocked.Error)
		}
		return *blocked, nil
	}

	// CompletionPolicy is evaluated once inside FinalRuntimeFacts before the
	// final contract is committed. runHostIterationLoop returns the same typed
	// decision as blocked when it is not allow.
	if _, err := k.runTurnHook(runCtx, hooks.StagePostTurn, session, req, turnID, agentOutput, nil); err != nil {
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, "post_turn: "+err.Error())
		}
		return TurnResult{}, fmt.Errorf("post_turn: %w", err)
	}

	// Complete the turn span on success
	if k.spanSource != nil && turnSpanID != "" {
		summary := "Turn completed"
		if agentOutput != "" {
			summary = truncateString(agentOutput, 100)
		}
		k.spanSource.CompleteSpan(turnSpanID, summary, agentOutput)
	}

	return TurnResult{
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		SessionID:       session.ID,
		TurnID:          turnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		Status:          "completed",
		Output:          agentOutput,
	}, nil
}

func observedTurnStatus(result TurnResult, err error) (string, string) {
	if err != nil {
		return "failed", err.Error()
	}
	status := strings.TrimSpace(result.Status)
	message := strings.TrimSpace(result.Error)
	switch status {
	case "":
		return "completed", message
	case "completed":
		return "completed", message
	case "failed", "error":
		return "failed", firstNonEmpty(message, "turn failed")
	default:
		return status, message
	}
}

func finishObservedSpan(span ObservedSpan, status, message string, attrs map[string]any) {
	if span == nil {
		return
	}
	if attrs != nil {
		span.SetAttributes(attrs)
	}
	span.SetStatus(status, message)
	span.End()
}

func modelNameForTrace(chatModel modelrouter.ChatModel) string {
	if chatModel == nil {
		return ""
	}
	return strings.TrimPrefix(fmt.Sprintf("%T", chatModel), "*")
}

func modelTraceMarkdownPath(tracePath string) string {
	tracePath = strings.TrimSpace(tracePath)
	if tracePath == "" {
		return ""
	}
	if strings.HasSuffix(tracePath, ".json") {
		return strings.TrimSuffix(tracePath, ".json") + ".md"
	}
	return tracePath
}

func modelTraceDiffPath(tracePath string) string {
	tracePath = strings.TrimSpace(tracePath)
	if tracePath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(tracePath), "input.diff.md")
}

type modelInputTracePayload struct {
	PromptInputTrace promptinput.PromptInputTrace `json:"promptInputTrace,omitempty"`
}

func latestModelInputPromptTrace(snapshot *TurnSnapshot) *promptinput.PromptInputTrace {
	if snapshot == nil {
		return nil
	}
	for i := len(snapshot.Iterations) - 1; i >= 0; i-- {
		trace, err := readModelInputPromptTrace(snapshot.Iterations[i].ModelInputTraceFile)
		if err != nil || trace == nil || len(trace.Items) == 0 {
			continue
		}
		return trace
	}
	return nil
}

func readModelInputPromptTrace(path string) (*promptinput.PromptInputTrace, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var payload modelInputTracePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return nil, err
	}
	trace := payload.PromptInputTrace
	return &trace, nil
}

// ---------------------------------------------------------------------------
// ResumeTurn resumes a turn that was interrupted (e.g. by approval).
// Uses adk.Runner.Resume via checkpoint store.
// ---------------------------------------------------------------------------

// ResumeTurn resumes a turn that was interrupted by approval or user input.
func (k *RuntimeKernel) ResumeTurn(ctx context.Context, req ResumeRequest) (TurnResult, error) {
	if err := req.Validate(); err != nil {
		return TurnResult{}, fmt.Errorf("invalid resume request: %w", err)
	}

	session := k.sessions.Get(req.SessionID)
	if session == nil {
		return TurnResult{}, fmt.Errorf("session %q not found", req.SessionID)
	}
	runCtx, runCancel := context.WithCancel(ctx)
	pendingCancelReason := k.registerTurnExecution(session.ID, req.TurnID, runCancel)
	defer func() {
		k.releaseTurnExecution(session.ID, req.TurnID, runCancel)
		runCancel()
	}()

	snapshot := session.CurrentTurn
	if snapshot == nil || snapshot.ID != req.TurnID {
		return TurnResult{}, fmt.Errorf("turn %q is not suspended", req.TurnID)
	}
	if err := ValidateTurnRecoveryPreconditions(snapshot); err != nil {
		return TurnResult{}, err
	}
	if err := validateResumeImmutableControlMetadata(snapshot, req.Metadata); err != nil {
		return TurnResult{}, err
	}
	decisionApproval, err := exactApprovalForResumeDecision(session, snapshot, req)
	if err != nil {
		return TurnResult{}, err
	}
	snapshot.Metadata = mergeResumeTurnMetadata(snapshot.Metadata, req.Metadata)
	if len(snapshot.TraceContext) > 0 {
		runCtx = k.runtimeObserver().ContextWithTraceContext(runCtx, snapshot.TraceContext)
	}
	if planApproval := decisionApproval; planApproval.Source == PlanModeEntryApprovalSource || planApproval.Source == PlanExitApprovalSource {
		appendApprovalRequestedAgentItem(snapshot, planApproval)
		k.persistTurnSnapshot(session, snapshot)
		decisionReason := firstNonEmpty(req.Metadata["approval.reason"], req.Metadata["rejection.reason"], req.Metadata["reason"])
		now := time.Now()
		var err error
		status := "completed"
		decisionStatus := map[bool]string{true: "approved", false: "denied"}[isApprovedResumeDecision(req.Decision)]
		if err := k.emitApprovalDecided(runCtx, session, snapshot, planApproval, approvedResumeDecisionLabel(req.Decision), decisionStatus, now); err != nil {
			return TurnResult{}, err
		}
		switch planApproval.Source {
		case PlanModeEntryApprovalSource:
			_, err = ApplyPlanModeEntryDecision(session, planApproval.ID, req.Decision, decisionReason, now)
			if !isApprovedResumeDecision(req.Decision) {
				status = "blocked"
			}
		case PlanExitApprovalSource:
			artifact := RuntimePlanArtifact{ID: session.PlanMode.PlanID, Status: PlanArtifactPendingApproval}
			_, _, err = ApplyPlanApprovalDecision(session, artifact, planApproval.ID, req.Decision, decisionReason, now)
			if !isApprovedResumeDecision(req.Decision) {
				status = "blocked"
			}
		}
		if err != nil {
			return TurnResult{}, err
		}
		snapshot.ResumeState = TurnResumeStateNone
		snapshot.PendingApprovals = removePendingApproval(snapshot.PendingApprovals, planApproval.ID)
		snapshot.UpdatedAt = now
		if !isApprovedResumeDecision(req.Decision) {
			recordRejectedApproval(session, planApproval, req.Decision, decisionReason, now)
		}
		session.PendingApprovals = removePendingApproval(session.PendingApprovals, planApproval.ID)
		k.persistTurnSnapshot(session, snapshot)
		return TurnResult{
			SessionType:     session.Type,
			Mode:            session.Mode,
			SessionID:       session.ID,
			TurnID:          req.TurnID,
			ClientTurnID:    snapshot.ClientTurnID,
			ClientMessageID: snapshot.ClientMessageID,
			Status:          status,
		}, nil
	}
	if req.Decision != "" && !isApprovedResumeDecision(req.Decision) {
		now := time.Now()
		approval := decisionApproval
		decisionReason := firstNonEmpty(req.Metadata["approval.reason"], req.Metadata["rejection.reason"], req.Metadata["reason"])
		if err := k.emitApprovalDecided(runCtx, session, snapshot, approval, req.Decision, "denied", now); err != nil {
			return TurnResult{}, err
		}
		recordRejectedApproval(session, approval, req.Decision, decisionReason, now)
		return k.completeDeniedApprovalTurn(ctx, session, snapshot, approval, decisionReason, now)
	}
	agentKind := modelrouter.AgentKindWorker
	if session.Type == SessionTypeWorkspace {
		agentKind = modelrouter.AgentKindPlanner
	}
	chatModel, modelErr := k.modelRouter.GetModel(agentKind, modelrouter.ProviderConfig{})
	if modelErr != nil {
		return TurnResult{}, fmt.Errorf("get model: %w", modelErr)
	}
	if pendingCancelReason != "" {
		if _, cancelErr := k.markTurnCanceledRecorded(ctx, session, snapshot, pendingCancelReason); cancelErr != nil {
			return TurnResult{}, cancelErr
		}
		return TurnResult{
			SessionType:     session.Type,
			Mode:            session.Mode,
			SessionID:       session.ID,
			TurnID:          req.TurnID,
			ClientTurnID:    snapshot.ClientTurnID,
			ClientMessageID: snapshot.ClientMessageID,
			Status:          "cancelled",
		}, nil
	}

	resumeInput := resumeInputFromMetadata(req.Metadata)
	if resumeInput != "" {
		snapshot.PendingStepCause = &StepRevisionCause{Kind: StepRevisionKindUserInputResumed, CheckpointID: checkpointIDForStepCause(snapshot)}
		appendResumeInputMessage(session, resumeInput)
		updateRuntimeEnvironmentContext(session, TurnRequest{
			SessionType: session.Type,
			Mode:        session.Mode,
			SessionID:   session.ID,
			TurnID:      req.TurnID,
			HostID:      session.HostID,
			Input:       resumeInput,
			Metadata:    req.Metadata,
		}, time.Now())
		if err := k.markSnapshotResuming(session, snapshot, "resume_user_input"); err != nil {
			return TurnResult{}, err
		}
	} else if toolCall, ok := gatedPendingToolCall(snapshot); ok {
		execution, tokenErr := k.prepareApprovalResumeExecution(session, snapshot, decisionApproval, toolCall)
		if tokenErr != nil {
			return k.blockStaleApprovalContext(runCtx, session, snapshot, decisionApproval, toolCall, tokenErr)
		}
		if err := k.emitApprovalDecided(runCtx, session, snapshot, decisionApproval, approvedResumeDecisionLabel(req.Decision), "approved", time.Now()); err != nil {
			return TurnResult{}, err
		}
		execution.approvalID = firstNonEmpty(decisionApproval.ID, req.ApprovalID)
		execution.rememberSessionGrant = isSessionApprovalResumeDecision(req.Decision)
		blocked, err := k.resumePendingToolCall(runCtx, session, snapshot, execution)
		if err != nil {
			return TurnResult{}, err
		}
		if blocked != nil {
			return *blocked, nil
		}
	} else {
		if snapshot.LatestCheckpoint != nil && strings.EqualFold(strings.TrimSpace(snapshot.LatestCheckpoint.Kind), "model_timeout") {
			snapshot.PendingStepCause = &StepRevisionCause{Kind: StepRevisionKindModelRetryResumed, CheckpointID: snapshot.LatestCheckpoint.ID}
		}
		if err := k.markSnapshotResuming(session, snapshot, "resume_checkpoint"); err != nil {
			return TurnResult{}, err
		}
	}

	resumeReq := TurnRequest{
		SessionType:     session.Type,
		Mode:            session.Mode,
		SessionID:       session.ID,
		TurnID:          req.TurnID,
		ClientTurnID:    snapshot.ClientTurnID,
		ClientMessageID: snapshot.ClientMessageID,
		HostID:          session.HostID,
		Metadata:        mergeResumeTurnMetadata(snapshot.Metadata, req.Metadata),
	}
	agentOutput, blocked, runErr := k.runHostIterationLoop(runCtx, chatModel, agentKind, resumeReq, session, req.TurnID, hooks.TurnEvent{}, "")
	if runErr != nil {
		if errors.Is(runErr, context.Canceled) {
			if _, cancelErr := k.markTurnCanceledRecorded(ctx, session, snapshot, "user stop"); cancelErr != nil {
				return TurnResult{}, cancelErr
			}
			return TurnResult{
				SessionType:     session.Type,
				Mode:            session.Mode,
				SessionID:       session.ID,
				TurnID:          req.TurnID,
				ClientTurnID:    snapshot.ClientTurnID,
				ClientMessageID: snapshot.ClientMessageID,
				Status:          "cancelled",
			}, nil
		}
		if transitionErr := k.markTurnFailedFromError(session, snapshot, runErr, inferTurnFailureCheckpointKind(snapshot)); transitionErr != nil {
			return TurnResult{}, transitionErr
		}
		return TurnResult{}, fmt.Errorf("resume turn: %w", runErr)
	}
	if blocked != nil {
		return *blocked, nil
	}
	turnCompletePayload, _ := json.Marshal(map[string]any{
		"watchPaths": []string{},
	})
	k.projector.Emit(LifecycleEvent{
		Type:      EventTurnComplete,
		SessionID: session.ID,
		TurnID:    req.TurnID,
		Timestamp: time.Now(),
		Payload:   turnCompletePayload,
	})
	return TurnResult{
		SessionType:     session.Type,
		Mode:            session.Mode,
		SessionID:       session.ID,
		TurnID:          req.TurnID,
		ClientTurnID:    snapshot.ClientTurnID,
		ClientMessageID: snapshot.ClientMessageID,
		Status:          "completed",
		Output:          agentOutput,
	}, nil
}

func checkpointIDForStepCause(snapshot *TurnSnapshot) string {
	if snapshot == nil || snapshot.LatestCheckpoint == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.LatestCheckpoint.ID)
}

func validateResumeImmutableControlMetadata(snapshot *TurnSnapshot, resumeMetadata map[string]string) error {
	if snapshot == nil || len(resumeMetadata) == 0 {
		return nil
	}
	for key, value := range resumeMetadata {
		if !runtimecontract.IsAdmissionControlMetadataKey(key) {
			continue
		}
		expected, ok := immutableResumeControlMetadataValue(snapshot, key)
		if !ok || strings.TrimSpace(value) != expected {
			return fmt.Errorf("immutable control metadata drift: %s", strings.TrimSpace(key))
		}
	}
	return nil
}

func immutableResumeControlMetadataValue(snapshot *TurnSnapshot, key string) (string, bool) {
	key = strings.TrimSpace(key)
	if snapshot != nil && snapshot.TurnAssembly != nil {
		assembly := snapshot.TurnAssembly
		switch key {
		case runtimecontract.MetadataProfile, runtimecontract.MetadataToolProfile, runtimecontract.MetadataAgentProfile:
			return strings.TrimSpace(assembly.AdmissionFacts.Profile), true
		case runtimecontract.MetadataAgentKind:
			return strings.TrimSpace(assembly.AdmissionFacts.AgentKind), true
		case runtimecontract.MetadataPermissionProfile:
			return strings.TrimSpace(firstNonEmptyString(assembly.PermissionProfile, assembly.AdmissionFacts.PermissionProfile)), true
		}
	}
	if snapshot == nil || snapshot.Metadata == nil {
		return "", false
	}
	value, ok := snapshot.Metadata[key]
	return strings.TrimSpace(value), ok
}

func (k *RuntimeKernel) emitApprovalDecided(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, approval PendingApproval, decision, status string, at time.Time) error {
	if k == nil || session == nil || snapshot == nil {
		return nil
	}
	if at.IsZero() {
		at = time.Now()
	}
	id := strings.TrimSpace(approval.ID)
	if id == "" {
		return nil
	}
	approval.ID = id
	if err := k.recordCanonicalApprovalDecided(ctx, snapshot, approval, decision, status); err != nil {
		return fmt.Errorf("record approval decision: %w", err)
	}
	appendApprovalDecidedAgentItem(snapshot, approval, decision, status)
	k.persistTurnSnapshot(session, snapshot)
	if k.projector == nil {
		return nil
	}
	payload, _ := json.Marshal(map[string]any{
		"id":       id,
		"toolName": approval.ToolName,
		"command":  approval.Command,
		"hostId":   approval.HostID,
		"decision": strings.TrimSpace(decision),
		"status":   strings.TrimSpace(status),
	})
	k.projector.Emit(LifecycleEvent{
		Type:      EventApprovalDecided,
		SessionID: session.ID,
		TurnID:    snapshot.ID,
		Timestamp: at,
		Payload:   payload,
	})
	return nil
}

func exactApprovalForResumeDecision(session *SessionState, snapshot *TurnSnapshot, req ResumeRequest) (PendingApproval, error) {
	if snapshot != nil && snapshot.ResumeState == TurnResumeStatePendingEvidence {
		if req.ResumeState != "" && req.ResumeState != TurnResumeStatePendingEvidence {
			return PendingApproval{}, fmt.Errorf("turn %q requires pending evidence resume", req.TurnID)
		}
		if strings.TrimSpace(req.Decision) == "" {
			return PendingApproval{}, nil
		}
		evidenceID := strings.TrimSpace(req.ApprovalID)
		if evidenceID == "" {
			return PendingApproval{}, fmt.Errorf("evidence id is required for decision %q", req.Decision)
		}
		evidence := pendingEvidenceByID(session, snapshot, evidenceID)
		if evidence.ID != evidenceID {
			return PendingApproval{}, fmt.Errorf("evidence %q is not pending for turn %q", evidenceID, req.TurnID)
		}
		toolCall, ok := pendingToolCall(snapshot)
		if !ok || strings.TrimSpace(evidence.ToolCallID) == "" || evidence.ToolCallID != toolCall.ID {
			return PendingApproval{}, fmt.Errorf("evidence %q does not match pending tool call", evidenceID)
		}
		return PendingApproval{
			ID:         evidence.ID,
			SessionID:  evidence.SessionID,
			TurnID:     evidence.TurnID,
			Iteration:  evidence.Iteration,
			ToolName:   evidence.ToolName,
			ToolCallID: evidence.ToolCallID,
			Reason:     evidence.Reason,
			Source:     "pending_evidence",
			Status:     evidence.Status,
			CreatedAt:  evidence.CreatedAt,
			UpdatedAt:  evidence.UpdatedAt,
		}, nil
	}
	if req.ResumeState == TurnResumeStatePendingEvidence {
		return PendingApproval{}, fmt.Errorf("turn %q is not pending evidence", req.TurnID)
	}
	if strings.TrimSpace(req.Decision) == "" {
		if snapshot != nil && snapshot.ResumeState == TurnResumeStatePendingApproval {
			return PendingApproval{}, fmt.Errorf("approval decision is required to resume pending approval for turn %q", req.TurnID)
		}
		return PendingApproval{}, nil
	}
	approvalID := strings.TrimSpace(req.ApprovalID)
	if approvalID == "" {
		return PendingApproval{}, fmt.Errorf("approval id is required for decision %q", req.Decision)
	}
	approval := pendingApprovalByID(session, snapshot, approvalID)
	if approval.ID != approvalID {
		return PendingApproval{}, fmt.Errorf("approval %q is not pending for turn %q", approvalID, req.TurnID)
	}
	if approval.Source == PlanModeEntryApprovalSource || approval.Source == PlanExitApprovalSource {
		return approval, nil
	}
	toolCall, ok := pendingToolCall(snapshot)
	if !ok || strings.TrimSpace(approval.ToolCallID) == "" || approval.ToolCallID != toolCall.ID {
		return PendingApproval{}, fmt.Errorf("approval %q does not match pending tool call", approvalID)
	}
	return approval, nil
}

func pendingEvidenceByID(session *SessionState, snapshot *TurnSnapshot, evidenceID string) PendingEvidence {
	target := strings.TrimSpace(evidenceID)
	if snapshot != nil {
		for _, evidence := range snapshot.PendingEvidence {
			if target == "" || evidence.ID == target {
				return evidence
			}
		}
	}
	if session != nil {
		for _, evidence := range session.PendingEvidence {
			if target == "" || evidence.ID == target {
				return evidence
			}
		}
	}
	return PendingEvidence{}
}

func pendingApprovalByID(session *SessionState, snapshot *TurnSnapshot, approvalID string) PendingApproval {
	target := strings.TrimSpace(approvalID)
	if snapshot != nil {
		for _, approval := range snapshot.PendingApprovals {
			if target == "" || approval.ID == target {
				return approval
			}
		}
	}
	if session != nil {
		for _, approval := range session.PendingApprovals {
			if target == "" || approval.ID == target {
				return approval
			}
		}
	}
	return PendingApproval{}
}

func recordRejectedApproval(session *SessionState, approval PendingApproval, decision, reason string, at time.Time) {
	if session == nil {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	rejected := RejectedApproval{
		ID:         strings.TrimSpace(approval.ID),
		TurnID:     strings.TrimSpace(approval.TurnID),
		ToolName:   strings.TrimSpace(approval.ToolName),
		ToolCallID: strings.TrimSpace(approval.ToolCallID),
		Reason:     firstNonEmpty(reason, approval.Reason, "approval denied"),
		Decision:   strings.TrimSpace(decision),
		Source:     strings.TrimSpace(approval.Source),
		InputHash:  strings.TrimSpace(approval.InputHash),
		RejectedAt: at,
	}
	if rejected.ID == "" && rejected.ToolName == "" && rejected.InputHash == "" {
		return
	}
	key := firstNonEmpty(rejected.ID, rejected.InputHash, rejected.ToolCallID, rejected.ToolName)
	for i := range session.RejectedApprovals {
		existingKey := firstNonEmpty(session.RejectedApprovals[i].ID, session.RejectedApprovals[i].InputHash, session.RejectedApprovals[i].ToolCallID, session.RejectedApprovals[i].ToolName)
		if existingKey == key {
			session.RejectedApprovals[i] = rejected
			return
		}
	}
	session.RejectedApprovals = append(session.RejectedApprovals, rejected)
}

func (k *RuntimeKernel) completeDeniedApprovalTurn(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, approval PendingApproval, reason string, at time.Time) (TurnResult, error) {
	if session == nil || snapshot == nil {
		return TurnResult{}, fmt.Errorf("session and snapshot are required")
	}
	transitionSnapshot := *snapshot
	if snapshot.Lifecycle == TurnLifecycleSuspended {
		if err := validateTurnLifecycleTransition(&transitionSnapshot, runtimestate.TransitionTurnResumed, TurnLifecycleRunning); err != nil {
			return TurnResult{}, err
		}
		transitionSnapshot.Lifecycle = TurnLifecycleRunning
	}
	if err := validateTurnLifecycleTransition(&transitionSnapshot, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted); err != nil {
		return TurnResult{}, err
	}
	finalText := deniedApprovalFinalText(approval, reason)
	message := Message{
		ID:        fmt.Sprintf("msg-%d", at.UnixNano()),
		Role:      "assistant",
		Content:   finalText,
		Timestamp: at,
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), "approval_denied", TurnLifecycleCompleted, TurnResumeStateNone)
	itemID := fmt.Sprintf("%s-approval-denied-final", snapshot.ID)
	recordSnapshot := transitionSnapshot
	recordSnapshot.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
	recordSnapshot.Lifecycle = TurnLifecycleCompleted
	recordSnapshot.ResumeState = TurnResumeStateNone
	recordSnapshot.PendingApprovals = nil
	recordSnapshot.PendingEvidence = nil
	recordSnapshot.LatestCheckpoint = checkpoint
	recordSession := *session
	recordSession.PendingApprovals = nil
	recordSession.PendingEvidence = nil
	finalRuntimeFacts := BuildFinalRuntimeFactsWithContext(ctx, &recordSnapshot, &recordSession, k.finalCompletionEvaluator())
	finalContract := BuildFinalContract(finalText, finalRuntimeFacts)
	finalCommit := assistantOutputCommitInput{
		TurnID:           snapshot.ID,
		Iteration:        snapshot.Iteration,
		ItemID:           itemID,
		MessageID:        message.ID,
		AssistantText:    finalText,
		EvidenceBoundary: "blocked",
		BoundaryAction:   FinalMessageBoundaryBlock,
		FinalContract:    &finalContract,
	}
	commitFinalAssistantOutput(&recordSnapshot, finalCommit)
	if err := k.recordCanonicalCheckpoint(ctx, &recordSnapshot, checkpoint); err != nil {
		snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
		return TurnResult{}, err
	}
	if err := k.recordCanonicalFinalFacts(ctx, &recordSnapshot, finalRuntimeFacts, finalContract); err != nil {
		snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
		return TurnResult{}, err
	}
	if err := k.recordCanonicalTransportProjection(ctx, &recordSnapshot, TurnLifecycleCompleted, TurnResumeStateNone, checkpoint.ID, &finalContract); err != nil {
		snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
		return TurnResult{}, err
	}

	session.Messages = append(session.Messages, message)
	snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleCompleted
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = ""
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	snapshot.UpdatedAt = at
	snapshot.CompletedAt = &at
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleCompleted
		last.ResumeState = TurnResumeStateNone
		last.Checkpoint = checkpoint
		last.UpdatedAt = at
		last.CompletedAt = &at
	}
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	commitFinalAssistantOutput(snapshot, finalCommit)
	snapshot.FinalOutput = FinalTextFromAssistantMessage(snapshot)
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteAssistantMessage, OwnerRuntimeKernel)
	syncActiveTurnState(session, snapshot)
	k.persistTurnSnapshot(session, snapshot)
	return TurnResult{
		SessionType:     session.Type,
		Mode:            session.Mode,
		SessionID:       session.ID,
		TurnID:          snapshot.ID,
		ClientTurnID:    snapshot.ClientTurnID,
		ClientMessageID: snapshot.ClientMessageID,
		Status:          "completed",
		Output:          finalText,
	}, nil
}

func deniedApprovalFinalText(approval PendingApproval, reason string) string {
	payload := map[string]any{
		"status":     "approval_denied",
		"approvalId": strings.TrimSpace(approval.ID),
		"tool":       firstNonEmpty(approval.ToolName, approval.Command),
		"scope":      firstNonEmpty(approval.HostID, strings.Join(approval.TargetRefs, ","), approval.RequestedScope),
		"reason":     firstNonEmpty(strings.TrimSpace(reason), approval.Reason, "approval denied"),
		"allowedNextSteps": []string{
			"continue_with_existing_evidence",
			"ask_for_read_only_alternative",
			"provide_limited_hypothesis",
		},
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return `{"status":"approval_denied","reason":"approval denied","allowedNextSteps":["continue_with_existing_evidence","ask_for_read_only_alternative","provide_limited_hypothesis"]}`
	}
	return string(data)
}

func isApprovedResumeDecision(decision string) bool {
	switch strings.ToLower(strings.TrimSpace(decision)) {
	case "", "approved", "approved_for_session":
		return true
	default:
		return false
	}
}

func isSessionApprovalResumeDecision(decision string) bool {
	return strings.EqualFold(strings.TrimSpace(decision), "approved_for_session")
}

func approvedResumeDecisionLabel(decision string) string {
	if isSessionApprovalResumeDecision(decision) {
		return "approved_for_session"
	}
	return "approved"
}

func rememberSessionApprovalGrant(session *SessionState, toolCall ToolCall, approvalID string) {
	if session == nil {
		return
	}
	inputHash, err := actionproposal.NormalizedInputHash(toolCall.Arguments)
	if err != nil || strings.TrimSpace(inputHash) == "" {
		return
	}
	toolName := strings.TrimSpace(toolCall.Name)
	if toolName == "" {
		return
	}
	now := time.Now()
	command := strings.TrimSpace(approvalCommandForToolCall(toolCall))
	for idx := range session.ApprovalGrants {
		grant := &session.ApprovalGrants[idx]
		if strings.TrimSpace(grant.ToolName) != toolName || strings.TrimSpace(grant.InputHash) != inputHash {
			continue
		}
		grant.Command = firstNonEmpty(command, grant.Command)
		grant.Source = "session"
		grant.UpdatedAt = now
		return
	}
	session.ApprovalGrants = append(session.ApprovalGrants, SessionApprovalGrant{
		ID:        firstNonEmpty(strings.TrimSpace(approvalID), strings.TrimSpace(toolCall.ID)),
		ToolName:  toolName,
		InputHash: inputHash,
		Command:   command,
		Source:    "session",
		CreatedAt: now,
		UpdatedAt: now,
	})
}

// ---------------------------------------------------------------------------
// CancelTurn cancels an active turn.
// ---------------------------------------------------------------------------

// CancelTurn cancels an active turn and returns the cancellation result.
func (k *RuntimeKernel) CancelTurn(ctx context.Context, req CancelRequest) (TurnResult, error) {
	if err := req.Validate(); err != nil {
		return TurnResult{}, fmt.Errorf("invalid cancel request: %w", err)
	}

	session := k.sessions.Get(req.SessionID)
	if session == nil {
		return TurnResult{}, fmt.Errorf("session %q not found", req.SessionID)
	}
	inFlight := k.requestTurnCancel(req.SessionID, req.TurnID, req.Reason)

	var clientTurnID, clientMessageID string
	if !inFlight {
		snapshot := session.CurrentTurn
		if snapshot == nil || snapshot.ID != req.TurnID {
			return TurnResult{
				SessionType: session.Type,
				Mode:        session.Mode,
				SessionID:   session.ID,
				TurnID:      req.TurnID,
				Status:      "cancelled",
			}, nil
		}
		clientTurnID = snapshot.ClientTurnID
		clientMessageID = snapshot.ClientMessageID
		if _, cancelErr := k.markTurnCanceledRecorded(ctx, session, snapshot, req.Reason); cancelErr != nil {
			return TurnResult{}, cancelErr
		}
	}

	return TurnResult{
		SessionType:     session.Type,
		Mode:            session.Mode,
		SessionID:       session.ID,
		TurnID:          req.TurnID,
		ClientTurnID:    clientTurnID,
		ClientMessageID: clientMessageID,
		Status:          "cancelled",
	}, nil
}

func (k *RuntimeKernel) runTurnHook(ctx context.Context, stage hooks.Stage, session *SessionState, req TurnRequest, turnID, output string, turnErr error) (hooks.TurnEvent, error) {
	if k.hooks == nil {
		return hooks.TurnEvent{}, nil
	}
	event := hooks.TurnEvent{
		SessionID:   session.ID,
		TurnID:      turnID,
		SessionType: string(req.SessionType),
		Mode:        string(req.Mode),
		Input:       req.Input,
		Output:      output,
		Err:         turnErr,
	}
	if err := k.hooks.RunTurnStage(ctx, stage, &event); err != nil {
		return hooks.TurnEvent{}, err
	}
	return event, nil
}

func (k *RuntimeKernel) compileContext(session SessionType, mode Mode, metadata map[string]string) promptcompiler.CompileContext {
	flags := k.runtimeFeatureFlags(context.Background())
	assemblyMode := effectiveToolAssemblyMode(session, mode, metadata)
	if source, ok := k.tools.(metadataToolAssemblySource); ok {
		compileCtx := promptcompiler.CompileContext{
			SessionType:    string(session),
			Mode:           string(mode),
			AssembledTools: source.CompileContextWithMetadata(session, assemblyMode, metadata),
		}
		compileCtx = k.attachDeferredToolDirectoryContext(compileCtx, session, assemblyMode, metadata)
		compileCtx.AssembledTools = appendContextArtifactTools(compileCtx.AssembledTools, k.contextArtifactToolsForMetadata(metadata)...)
		return applyRuntimeFeatureFlags(compileCtx, flags)
	}
	compileCtx := k.tools.CompileContext(session, assemblyMode)
	compileCtx.Mode = string(mode)
	compileCtx = k.attachDeferredToolDirectoryContext(compileCtx, session, assemblyMode, metadata)
	if opsManualsOptedOut(metadata) {
		compileCtx.AssembledTools = filterOpsManualTools(compileCtx.AssembledTools)
	}
	compileCtx.AssembledTools = appendContextArtifactTools(compileCtx.AssembledTools, k.contextArtifactToolsForMetadata(metadata)...)
	return applyRuntimeFeatureFlags(compileCtx, flags)
}

func (k *RuntimeKernel) attachDeferredToolDirectoryContext(ctx promptcompiler.CompileContext, session SessionType, mode Mode, metadata map[string]string) promptcompiler.CompileContext {
	if !runtimeToolSearchEnabled(metadata) {
		ctx.DeferredToolCatalog = nil
		if ctx.MCPHealthSnapshot == nil {
			ctx.MCPHealthSnapshot = mcpHealthSnapshotForPrompt()
		}
		return ctx
	}
	if len(ctx.DeferredToolCatalog) == 0 {
		ctx.DeferredToolCatalog = k.progressiveDiscoveryCatalog(session, mode)
	}
	ctx.DeferredToolCatalog = filterDeferredDiscoveryCatalog(ctx.DeferredToolCatalog)
	ctx.DeferredToolCatalog = tooling.FilterToolsByPackMetadata(ctx.DeferredToolCatalog, metadata)
	if ctx.MCPHealthSnapshot == nil {
		ctx.MCPHealthSnapshot = mcpHealthSnapshotForPrompt()
	}
	return ctx
}

func runtimeToolSearchEnabled(metadata map[string]string) bool {
	return metadataBool(metadata["aiops.toolSearch.enabled"]) ||
		metadataListContains(metadata["enableTool"], "tool_search")
}

func mcpHealthSnapshotForPrompt() map[string]string {
	snapshots := mcp.DefaultRegistry().ListServerHealthSnapshots()
	if len(snapshots) == 0 {
		return nil
	}
	out := make(map[string]string, len(snapshots))
	for _, snapshot := range snapshots {
		if strings.TrimSpace(snapshot.ServerID) == "" || strings.TrimSpace(string(snapshot.Status)) == "" {
			continue
		}
		out[strings.TrimSpace(snapshot.ServerID)] = strings.TrimSpace(string(snapshot.Status))
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func (k *RuntimeKernel) contextArtifactToolsForMetadata(metadata map[string]string) []promptcompiler.Tool {
	if !contextArtifactToolsEnabled(metadata) {
		return nil
	}
	return k.contextArtifactTools()
}

func (k *RuntimeKernel) appendContextArtifactToolsForExternalRefs(ctx promptcompiler.CompileContext, session *SessionState, snapshot *TurnSnapshot) promptcompiler.CompileContext {
	if !hasReadableContextArtifactReference(session, snapshot) {
		return ctx
	}
	ctx.AssembledTools = appendContextArtifactTools(ctx.AssembledTools, k.contextArtifactTools()...)
	return ctx
}

func hasReadableContextArtifactReference(session *SessionState, snapshot *TurnSnapshot) bool {
	if snapshotHasReadableContextArtifactReference(snapshot) {
		return true
	}
	if session == nil {
		return false
	}
	for _, ref := range session.ExternalReferences {
		if externalReferenceReadableByContextArtifact(ref) {
			return true
		}
	}
	if snapshotHasReadableContextArtifactReference(session.CurrentTurn) {
		return true
	}
	for i := len(session.TurnHistory) - 1; i >= 0 && len(session.TurnHistory)-i <= 8; i-- {
		if snapshotHasReadableContextArtifactReference(&session.TurnHistory[i]) {
			return true
		}
	}
	return false
}

func snapshotHasReadableContextArtifactReference(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, ref := range snapshot.ExternalReferences {
		if externalReferenceReadableByContextArtifact(ref) {
			return true
		}
	}
	for _, iteration := range snapshot.Iterations {
		for _, ref := range iteration.ExternalReferences {
			if externalReferenceReadableByContextArtifact(ref) {
				return true
			}
		}
		for _, result := range iteration.ToolResults {
			for _, ref := range result.ExternalReferences {
				if externalReferenceReadableByContextArtifact(ref) {
					return true
				}
			}
		}
	}
	return false
}

func externalReferenceReadableByContextArtifact(ref ExternalReference) bool {
	uri := strings.TrimSpace(ref.URI)
	if uri == "" {
		return false
	}
	return strings.HasPrefix(uri, "store://tool-spills/") || strings.HasPrefix(uri, "store://artifacts/")
}

func contextArtifactToolsEnabled(metadata map[string]string) bool {
	if metadataBool(metadata["contextArtifactAvailable"]) ||
		metadataBool(metadata["hasContextArtifact"]) ||
		metadataBool(metadata["contextArtifactEnabled"]) {
		return true
	}
	if metadataListContains(metadata["enableToolPack"], "context_artifact") {
		return true
	}
	return metadataListContains(metadata["enableTool"], "read_context_artifact")
}

const defaultContextArtifactInlineResultBytes = defaultContextArtifactReadBytes * 2

func (k *RuntimeKernel) contextArtifactTools() []promptcompiler.Tool {
	if k == nil || (k.artifactRepo == nil && k.spillRepo == nil) {
		return nil
	}
	reader := NewContextArtifactReader(ContextArtifactReaderOptions{
		Repository:      k.artifactRepo,
		SpillRepository: k.spillRepo,
	})
	return []promptcompiler.Tool{&tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_context_artifact",
			Description: "Read a bounded range, query match, or metadata view from a previously externalized context artifact or tool result spill.",
			Origin:      tooling.ToolOriginBuiltin,
			Layer:       tooling.ToolLayerConditional,
			Pack:        "context_artifact",
			RiskLevel:   tooling.ToolRiskLow,
			Mutating:    false,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:    "context_artifact",
				ResourceTypes:     []string{"context_artifact", "tool_result_spill"},
				OperationKinds:    []string{"read", "query", "inspect"},
				RequiresSelect:    true,
				PromptBudgetClass: "small",
				SchemaBudgetClass: "compact",
			},
			ResultBudget: tooling.ResultBudget{
				MaxInlineResultBytes: defaultContextArtifactInlineResultBytes,
				SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
				SummarizeLargeResult: true,
			},
		},
		InputSchemaData: json.RawMessage(`{"type":"object","properties":{"id":{"type":"string"},"offset":{"type":"integer"},"limit":{"type":"integer"},"query":{"type":"string"},"page":{"type":"integer"},"format":{"type":"string","enum":["text","json","metadata"]},"range":{"type":"object","properties":{"offset":{"type":"integer"},"limit":{"type":"integer"},"page":{"type":"integer"},"query":{"type":"string"},"format":{"type":"string"}}}},"required":["id"],"additionalProperties":false}`),
		ReadOnlyFunc: func(json.RawMessage) bool {
			return true
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var req ContextArtifactReadRequest
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, fmt.Errorf("parse read_context_artifact input: %w", err)
			}
			result, err := reader.Read(req)
			if err != nil {
				return tooling.ToolResult{Error: err.Error()}, nil
			}
			data, err := json.Marshal(result)
			if err != nil {
				return tooling.ToolResult{}, fmt.Errorf("marshal context artifact read result: %w", err)
			}
			return tooling.ToolResult{
				Content: string(data),
				References: []tooling.ResultReference{{
					Kind:        tooling.ResultReferenceKindBlob,
					URI:         result.Artifact.URI,
					Title:       result.Artifact.ID,
					Summary:     result.Artifact.Summary,
					ContentType: result.Artifact.ContentType,
					Digest:      result.Artifact.Digest,
					Bytes:       result.Artifact.Bytes,
					Range:       result.Range,
				}},
				ResultBudget: tooling.ResultBudget{
					MaxInlineResultBytes: defaultContextArtifactInlineResultBytes,
					SpillPolicy:          tooling.ResultSpillPolicySummaryInline,
					SummarizeLargeResult: true,
				},
			}, nil
		},
	}}
}

func appendContextArtifactTools(tools []promptcompiler.Tool, extras ...promptcompiler.Tool) []promptcompiler.Tool {
	if len(extras) == 0 {
		return tools
	}
	seen := make(map[string]bool, len(tools)+len(extras))
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		seen[toolDef.Metadata().Name] = true
	}
	out := append([]promptcompiler.Tool(nil), tools...)
	for _, toolDef := range extras {
		if toolDef == nil {
			continue
		}
		name := toolDef.Metadata().Name
		if seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, toolDef)
	}
	return out
}

func applyToolSurfacePolicyToCompileContext(ctx promptcompiler.CompileContext, mode Mode, profile string, session *SessionState) (promptcompiler.CompileContext, tooling.ToolSurfacePolicySnapshot) {
	ctx.Profile = strings.TrimSpace(profile)
	filtered, snapshot := tooling.ApplyToolSurfacePolicy(ctx.AssembledTools, tooling.ToolSurfacePolicyOptions{
		Mode:                string(mode),
		Profile:             ctx.Profile,
		ActiveSkillPolicies: activeSkillToolPolicies(session),
	})
	ctx.AssembledTools = filtered
	return ctx, snapshot
}

func applyDefaultRuntimePromptProfile(metadata map[string]string, sessionType SessionType, hostID string) map[string]string {
	if metadata == nil {
		metadata = map[string]string{}
	}
	if firstMetadataValue(metadata, "profile", "toolProfile") != "" {
		return metadata
	}
	if sessionType == SessionTypeHost {
		metadata["profile"] = RuntimePromptProfileHostWorker
		metadata["toolProfile"] = RuntimePromptProfileHostWorker
		if strings.TrimSpace(hostID) != "" && strings.TrimSpace(metadata["aiops.host.id"]) == "" {
			metadata["aiops.host.id"] = strings.TrimSpace(hostID)
		}
	}
	return metadata
}

func (k *RuntimeKernel) applyProgressiveToolPackMetadata(metadata map[string]string, input string, sessionType SessionType, mode Mode, session *SessionState) map[string]string {
	return applyProgressiveToolPackMetadata(metadata, input, session, k.progressiveDiscoveryCatalog(sessionType, mode))
}

func (k *RuntimeKernel) progressiveDiscoveryCatalog(session SessionType, mode Mode) []tooling.Tool {
	source, ok := k.tools.(fullToolCatalogSource)
	if !ok {
		return nil
	}
	return filterDeferredDiscoveryCatalog(source.AssembleToolsWithOptions(string(session), string(mode), tooling.AssembleOptions{IncludeDeferredCatalog: true}))
}

func filterDeferredDiscoveryCatalog(tools []tooling.Tool) []tooling.Tool {
	if len(tools) == 0 {
		return nil
	}
	out := make([]tooling.Tool, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		if tooling.ToolExcludedFromDeferredDiscovery(toolDef.Metadata()) {
			continue
		}
		out = append(out, toolDef)
	}
	return out
}

func (k *RuntimeKernel) contextBudgetPolicyForSession(session *SessionState, agentKind modelrouter.AgentKind) ContextBudgetPolicy {
	caps := modelrouter.ModelCapabilities{
		MaxContextTokens: DefaultMaxTokens,
		MaxOutputTokens:  20000,
	}
	if k != nil && k.modelRouter != nil {
		caps = k.modelRouter.ResolveModelCapabilities(agentKind, modelrouter.ProviderConfig{})
	}
	if caps.MaxContextTokens <= 0 {
		caps.MaxContextTokens = DefaultMaxTokens
	}
	if caps.MaxContextTokens < 10000 {
		caps.MaxContextTokens = 10000
	}
	if caps.MaxOutputTokens <= 0 {
		caps.MaxOutputTokens = 20000
	}
	effectiveMaxContextTokens := caps.MaxContextTokens
	if session != nil {
		if shouldAdoptModelContextWindow(session.Context.MaxTokens, caps.MaxContextTokens) {
			session.Context.MaxTokens = caps.MaxContextTokens
		}
		if session.Context.MaxTokens > 0 {
			effectiveMaxContextTokens = session.Context.MaxTokens
		}
		recomputeContextWindow(&session.Context, session.Messages)
	}
	return DefaultContextBudgetPolicy(effectiveMaxContextTokens, caps.MaxOutputTokens)
}

func shouldAdoptModelContextWindow(current, modelWindow int) bool {
	if modelWindow <= 0 {
		return false
	}
	return current <= 0 || current == 128000 || current > modelWindow
}

func agentKindForSession(session *SessionState) modelrouter.AgentKind {
	if session != nil && session.Type == SessionTypeWorkspace {
		return modelrouter.AgentKindPlanner
	}
	return modelrouter.AgentKindWorker
}

func applyRuntimeFeatureFlags(ctx promptcompiler.CompileContext, flags featureflag.Flags) promptcompiler.CompileContext {
	ctx.DisableDiagnosticProtocol = !flags.DiagnosticProtocol
	return ctx
}

func effectiveToolAssemblyMode(session SessionType, mode Mode, metadata map[string]string) Mode {
	if session != SessionTypeHost || mode != ModeChat {
		return mode
	}
	profile := strings.TrimSpace(firstMetadataValue(metadata, "profile", "toolProfile", "agentProfile"))
	if profile == RuntimePromptProfileHostWorker {
		return ModeInspect
	}
	return mode
}

func (k *RuntimeKernel) runHostIterationLoop(
	ctx context.Context,
	chatModel modelrouter.ChatModel,
	agentKind modelrouter.AgentKind,
	req TurnRequest,
	session *SessionState,
	turnID string,
	preTurnEvent hooks.TurnEvent,
	turnSpanID string,
) (string, *TurnResult, error) {
	additionalContext := append([]string(nil), preTurnEvent.AdditionalContext...)
	snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
	const maxIterations = 16
	toolDispatches := countActualToolDispatches(snapshot)
	publicWebDispatches := 0
	publicWebQueries := 0
	publicWebDispatchesInitialized := false
	previousPromptInputTrace := latestModelInputPromptTrace(snapshot)
	var lastReasoningPersist time.Time
	turnMetadata := k.applyProgressiveToolPackMetadata(mergeResumeTurnMetadata(snapshot.Metadata, req.Metadata), req.Input, req.SessionType, req.Mode, session)
	turnMetadata = applyDefaultRuntimePromptProfile(turnMetadata, req.SessionType, session.HostID)
	req.Metadata = turnMetadata
	snapshot.Metadata = turnMetadata
	depthReq := TurnRequest{
		SessionID:             session.ID,
		TurnID:                turnID,
		SessionType:           req.SessionType,
		Mode:                  req.Mode,
		Input:                 req.Input,
		HostID:                req.HostID,
		IntentFrame:           req.IntentFrame,
		PermissionProfile:     req.PermissionProfile,
		Metadata:              turnMetadata,
		ResourceBindings:      req.ResourceBindings,
		ResourceRoleBindings:  req.ResourceRoleBindings,
		SessionTargetSnapshot: req.SessionTargetSnapshot,
		RoleBindingConflicts:  req.RoleBindingConflicts,
	}
	var admissionContext RuntimeTurnContext
	var admissionFacts runtimecontract.AdmissionFacts
	if snapshot.TurnAssembly != nil {
		if assemblyErr := snapshot.TurnAssembly.Validate(); assemblyErr != nil {
			return "", nil, fmt.Errorf("turn assembly validation failed")
		}
		admissionFacts = snapshot.TurnAssembly.AdmissionFacts
	} else {
		var admissionErr error
		admissionContext, admissionErr = BuildRuntimeTurnContext(depthReq, session, RuntimeTurnContextOptions{
			Lineage: RuntimeLineageSnapshot{AgentKind: string(agentKind)},
		})
		if admissionErr != nil {
			return "", nil, fmt.Errorf("turn admission context: %w", admissionErr)
		}
		admissionFacts = admissionContext.AdmissionFacts
		if recordErr := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
			Kind: modeltrace.CanonicalRolloutKindAdmission,
			Payload: map[string]any{
				"factsHash":   admissionFacts.Hash,
				"intentKind":  admissionFacts.Intent.Kind,
				"targetCount": len(admissionFacts.TargetRefs),
			},
		}); recordErr != nil {
			return "", nil, fmt.Errorf("record turn admission: %w", recordErr)
		}
		k.persistTurnSnapshot(session, snapshot)
		if admissionContext.AdmissionError == runtimeAdmissionErrorTargetRequired {
			finalText, completeErr := k.completeAdmissionTargetRequiredTurn(ctx, session, snapshot)
			return finalText, nil, completeErr
		}
	}
	depthProfile := depthProfileFromAdmissionFacts(depthReq, admissionFacts)
	if snapshot.TaskDepth.Level == "" {
		snapshot.TaskDepth = depthProfile
	}
	if snapshot.ResumeState != TurnResumeStateNone || (snapshot.Metadata != nil && strings.TrimSpace(snapshot.Metadata["resume.nextStepId"]) != "") {
		resumePolicy := EvaluateResumeContinuationPolicy(snapshot, req.Input)
		additionalContext = append(additionalContext, resumeContinuationPrompt(resumePolicy))
	}
	budgetPolicy := k.contextBudgetPolicyForSession(session, agentKind)
	thresholds := budgetPolicy.Thresholds()

	for iteration := len(snapshot.Iterations); iteration < maxIterations; iteration++ {
		k.emitIterationStage(session.ID, turnID, iteration, "context_pipeline", turnSpanID)
		contextState, contextErr := ApplyContextPipeline(ctx, &session.Context, session.Messages, ContextPipelineOptions{
			SessionID:         session.ID,
			TurnID:            turnID,
			Iteration:         iteration,
			Compressor:        k.compressor,
			Profile:           firstMetadataValue(turnMetadata, "profile", "toolProfile"),
			TargetRefs:        compactionTargetRefs(snapshot),
			PendingApprovals:  session.PendingApprovals,
			PendingEvidence:   session.PendingEvidence,
			RejectedApprovals: session.RejectedApprovals,
			ToolPacksLoaded:   session.ToolDiscovery.EnabledPacks(),
			BudgetPolicy:      budgetPolicy,
		})
		if contextErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, contextErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("context pipeline: %w", contextErr)
		}
		contextMessages, observationEvents := modelVisibleMessagesWithObservationDedupe(session, contextState.Messages)
		contextState.GovernanceEvents = append(contextState.GovernanceEvents, observationEvents...)
		appendContextGovernanceEvents(&snapshot.ContextGovernanceEvents, contextState.GovernanceEvents...)
		appendContextGovernanceEvents(&session.ContextGovernanceEvents, contextState.GovernanceEvents...)
		if len(contextState.CompactedSegments) > 0 {
			appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteContextCompaction, OwnerContextPipeline)
		}
		k.emitIterationStage(session.ID, turnID, iteration, "compile_prompt", turnSpanID)
		compileCtx := enrichCompileContext(k.compileContext(req.SessionType, req.Mode, turnMetadata), req.SessionType, session.HostID, turnMetadata, time.Now())
		compileCtx = applyDepthProfileToCompileContext(compileCtx, snapshot.TaskDepth, firstMetadataValue(turnMetadata, "reasoningEffort", "reasoning_effort"))
		compileCtx = applyTurnPromptProfileMetadata(compileCtx, turnMetadata)
		compileCtx = k.appendContextArtifactToolsForExternalRefs(compileCtx, session, snapshot)
		compileCtx = appendRuntimeEnvironmentContextSection(compileCtx, session)
		compileCtx = appendSkillActivationContext(compileCtx, session)
		compileCtx = appendMCPInstructionContext(compileCtx, session)
		compileCtx.AssembledTools = filterToolsForContextMode(compileCtx.AssembledTools, thresholds)
		if thresholds.SmallContextMode {
			appendContextGovernanceEvents(&snapshot.ContextGovernanceEvents, ContextGovernanceEvent{
				ID:        fmt.Sprintf("ctxgov-%s-%d-small-context", turnID, iteration),
				Layer:     ContextGovernanceLayerL1,
				Kind:      "context.small_context.enabled",
				SessionID: session.ID,
				TurnID:    turnID,
				Iteration: iteration,
				Message:   "当前模型上下文较小，系统会优先保留当前任务和关键证据",
				Budget:    thresholds,
			})
			appendContextGovernanceEvents(&session.ContextGovernanceEvents, snapshot.ContextGovernanceEvents...)
		}
		compileCtx.AssembledTools = filterHiddenTools(compileCtx.AssembledTools, snapshot.HiddenTools)
		dispatchTools := append([]promptcompiler.Tool(nil), compileCtx.AssembledTools...)
		if !publicWebDispatchesInitialized {
			publicWebDispatches = countPublicWebDispatches(snapshot, dispatchTools)
			publicWebQueries = countPublicWebQueries(snapshot, dispatchTools)
			publicWebDispatchesInitialized = true
		}
		var surfacePolicy tooling.ToolSurfacePolicySnapshot
		compileCtx, surfacePolicy = applyToolSurfacePolicyToCompileContext(compileCtx, req.Mode, firstMetadataValue(turnMetadata, "profile", "toolProfile"), session)
		if shouldSwitchToSynthesisOnlyForTurn(req.Mode, snapshot.TaskDepth, req.Input, session, snapshot, toolDispatches, compileCtx.AssembledTools) {
			applyHiddenTools(snapshot, toolNames(compileCtx.AssembledTools))
			compileCtx.AssembledTools = nil
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, synthesisOnlyPromptAsset(toolDispatches))
		} else if shouldSwitchToPublicWebSynthesisOnly(publicWebDispatches, compileCtx.AssembledTools) {
			applyHiddenTools(snapshot, publicWebToolNames(compileCtx.AssembledTools))
			compileCtx.AssembledTools = filterHiddenTools(compileCtx.AssembledTools, snapshot.HiddenTools)
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, publicWebSynthesisOnlyPromptAsset(publicWebDispatches))
		}
		if len(additionalContext) > 0 {
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, additionalContext...)
		}
		if evidencePrompt := evidenceAwareFinalAnswerPromptAsset(snapshot, 8); evidencePrompt != "" {
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, evidencePrompt)
		}
		compileCtx.EvidenceReminders = compileEvidenceReminders(req.Mode, session.PendingEvidence)
		compileCtx.ToolDelta = iterationToolDelta(snapshot, compileCtx.AssembledTools)
		compileCtx.ProtocolState = buildProtocolPromptState(snapshot, compileCtx.ToolDelta, session.PendingApprovals, session.PendingEvidence, session.RejectedApprovals)
		sourceToolFingerprint := assembledToolFingerprint(k.tools, req.SessionType, req.Mode, compileCtx.AssembledTools)
		stepToolRouter, routerErr := BuildStepToolRouter(StepToolRouterInput{
			Registered:        toolNames(dispatchTools),
			ModelVisible:      toolNames(compileCtx.AssembledTools),
			Dispatchable:      toolNames(compileCtx.AssembledTools),
			HiddenReasons:     hiddenReasonsFromToolSurfacePolicy(surfacePolicy),
			PolicyHash:        surfacePolicy.Hash,
			SourceFingerprint: sourceToolFingerprint,
		})
		if routerErr != nil {
			return "", nil, fmt.Errorf("step tool router: %w", routerErr)
		}
		compileCtx.VisibleToolFingerprint = stepToolRouter.Fingerprint
		compileCtx = applyRuntimeStateMetadata(compileCtx, turnMetadata, session, snapshot)
		if snapshot.TurnAssembly == nil {
			modelCaps := modelrouter.ModelCapabilities{
				Provider:         string(agentKind),
				Model:            modelNameForTrace(chatModel),
				MaxContextTokens: thresholds.MaxContextTokens,
				MaxOutputTokens:  thresholds.ReservedOutputTokens,
			}
			if k.modelRouter != nil {
				modelCaps = k.modelRouter.ResolveModelCapabilities(agentKind, modelrouter.ProviderConfig{})
			}
			turnContext := admissionContext
			turnContext.Model = modelCaps
			turnContext.ContextBudget = RuntimeContextBudgetSnapshot{
				MaxTokens:    thresholds.MaxContextTokens,
				TargetTokens: thresholds.EffectiveContextWindow,
			}
			turnContext.ToolPolicyHash = surfacePolicy.Hash
			assembly, assemblyErr := buildRuntimeTurnAssembly(runtimeTurnAssemblyInput{
				TurnContext:            turnContext,
				CompileContext:         compileCtx,
				ToolSurfacePolicy:      surfacePolicy,
				ToolSurfaceFingerprint: compileCtx.VisibleToolFingerprint,
				ResourceBindings:       req.ResourceBindings,
				RollbackPolicy:         req.RollbackPolicy,
				Mode:                   req.Mode,
				MaxIterations:          maxIterations,
			})
			if assemblyErr != nil {
				appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, "turn assembly validation failed", nil))
				k.persistTurnSnapshot(session, snapshot)
				return "", nil, assemblyErr
			}
			if captureErr := k.captureReplayTurnAssembly(ctx, snapshot, assembly); captureErr != nil {
				return "", nil, captureErr
			}
			snapshot.TurnAssembly = assembly
			if recordErr := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
				Kind:             modeltrace.CanonicalRolloutKindAssembly,
				TurnAssemblyHash: assembly.Hash,
				Payload: map[string]any{
					"schemaVersion":        assembly.SchemaVersion,
					"capabilityPolicyHash": assembly.CapabilityPolicy.Hash,
				},
			}); recordErr != nil {
				return "", nil, fmt.Errorf("record turn assembly: %w", recordErr)
			}
			k.persistTurnSnapshot(session, snapshot)
			k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "turn_assembly_built")
		} else if assemblyErr := snapshot.TurnAssembly.Validate(); assemblyErr != nil {
			return "", nil, fmt.Errorf("turn assembly validation failed")
		}
		compiled, compileErr := k.compiler.Compile(compileCtx)
		if compileErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, compileErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("compile prompt: %w", compileErr)
		}
		k.observeRuntimeStage(ctx, session.ID, turnID, iteration, "prompt_compiled")
		var retentionDecisions []ContextRetentionDecision
		compiled.PromptSections, retentionDecisions, compileErr = ApplyPromptSectionRetentionPolicy(compiled.PromptSections, DefaultContextRetentionPolicy())
		if compileErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, compileErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("prompt section retention: %w", compileErr)
		}
		retentionEvents := PromptSectionRetentionGovernanceEvents(session.ID, turnID, iteration, retentionDecisions)
		contextState.GovernanceEvents = append(contextState.GovernanceEvents, retentionEvents...)
		appendContextGovernanceEvents(&snapshot.ContextGovernanceEvents, retentionEvents...)
		appendContextGovernanceEvents(&session.ContextGovernanceEvents, retentionEvents...)
		if previousPromptInputTrace != nil {
			compiled.ChangedSections = promptcompiler.ChangedPromptSections(previousPromptInputTrace.PromptSections, compiled.PromptSections)
			compiled.PromptSections = promptcompiler.ApplyPromptSectionCache(previousPromptInputTrace.PromptSections, compiled.PromptSections)
		} else {
			compiled.ChangedSections = promptcompiler.ChangedPromptSections(nil, compiled.PromptSections)
			compiled.PromptSections = promptcompiler.ApplyPromptSectionCache(nil, compiled.PromptSections)
		}
		stablePromptHash := promptContentHash(promptcompiler.CompiledPromptStableText(compiled))
		promptFingerprint := promptFingerprintMap(compiled.Fingerprint)
		toolFingerprint := compileCtx.VisibleToolFingerprint
		visibleToolNames := toolNames(compileCtx.AssembledTools)
		frozenStepToolRouter := cloneStepToolRouter(stepToolRouter)
		toolSurfaceSnapshot := ToolSurfaceSnapshotRef{
			ID:                 fmt.Sprintf("toolsurface-%s-%d", turnID, iteration),
			Fingerprint:        toolFingerprint,
			ToolNames:          append([]string(nil), visibleToolNames...),
			StepRouter:         &frozenStepToolRouter,
			PolicySnapshotHash: surfacePolicy.Hash,
			PolicySnapshot:     &surfacePolicy,
			CreatedAt:          time.Now(),
		}
		snapshot.ToolSurfaceSnapshot = &toolSurfaceSnapshot
		refreshedTools := refreshedToolNames(snapshot, toolFingerprint, compileCtx.AssembledTools)

		k.emitIterationStage(session.ID, turnID, iteration, "assemble_tools", turnSpanID)
		modelVisibleStepTools := modelVisibleToolsForStep(dispatchTools, stepToolRouter)
		toolPool := tooling.AssembleEinoToolPool(modelVisibleStepTools)
		k.emitIterationStage(session.ID, turnID, iteration, "call_model", turnSpanID)
		runtimeToolSurface := stepToolRouter
		checkpointRef := ""
		if snapshot.LatestCheckpoint != nil {
			checkpointRef = snapshot.LatestCheckpoint.ID
		}
		stepPermissionHash := k.runtimePermissionSnapshotHash(snapshot.TurnAssembly, surfacePolicy)
		stepCtx, promptBuild, modelErr := k.buildRuntimeStepContext(req, session, agentKind, iteration, contextState, contextMessages, compiled, runtimeToolSurface, RuntimeStepControlFacts{
			TurnAssemblyHash: snapshot.TurnAssembly.Hash,
			PermissionHash:   stepPermissionHash,
			CheckpointRef:    checkpointRef,
		}, thresholds, modelNameForTrace(chatModel), snapshot.TurnAssembly)
		if modelErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, modelErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, modelErr
		}
		if captureErr := k.captureReplayStepContext(ctx, stepCtx); captureErr != nil {
			return "", nil, captureErr
		}
		compiled.Fingerprint = stepCtx.ProviderRequest.PromptFingerprint
		promptFingerprint = promptFingerprintMap(stepCtx.ProviderRequest.PromptFingerprint)
		stablePromptHash = firstNonBlankRuntimeString(stepCtx.ProviderRequest.PromptFingerprint.StablePrefixHash, stablePromptHash)
		stepRevisionFacts, revisionErr := BuildRuntimeStepRevisionFacts(snapshot.TurnAssembly, stepCtx, session, snapshot)
		if revisionErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, revisionErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, revisionErr
		}
		stepReference, revisionErr := BuildStepReference(snapshot.LatestStepReference, stepCtx, stepRevisionFacts)
		if revisionErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, revisionErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, revisionErr
		}
		frozenStepReference := cloneStepReference(stepReference)
		snapshot.LatestStepReference = &frozenStepReference
		snapshot.PendingStepCause = nil
		stepRevisionKinds := make([]string, 0, len(stepReference.Transition.Revisions))
		for _, revision := range stepReference.Transition.Revisions {
			if kind := strings.TrimSpace(revision.Kind); kind != "" {
				stepRevisionKinds = append(stepRevisionKinds, kind)
			}
		}
		stepRevisionKind := strings.TrimSpace(stepRevisionFacts.Cause.Kind)
		if stepRevisionKind == "" && len(stepRevisionKinds) > 0 {
			stepRevisionKind = stepRevisionKinds[0]
		}
		if recordErr := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
			StepID:           stepReference.StepHash,
			Kind:             modeltrace.CanonicalRolloutKindPrompt,
			TurnAssemblyHash: snapshot.TurnAssembly.Hash,
			StepContextHash:  stepCtx.Hash,
			Payload: map[string]any{
				"modelInputHash":     stepCtx.ProviderRequest.ModelInputHash,
				"promptSectionCount": len(compiled.PromptSections),
				"visibleToolCount":   len(visibleToolNames),
				"stepRevisionKind":   stepRevisionKind,
				"stepRevisionKinds":  stepRevisionKinds,
			},
		}); recordErr != nil {
			return "", nil, fmt.Errorf("record compiled prompt: %w", recordErr)
		}
		k.persistTurnSnapshot(session, snapshot)
		var promptInputDiff *promptinput.TraceDiff
		if previousPromptInputTrace != nil {
			diff := promptinput.DiffTrace(*previousPromptInputTrace, promptBuild.Trace)
			promptInputDiff = &diff
		}
		toolTraceFields := buildModelInputToolTraceFields(session, snapshot, toolFingerprint, surfacePolicy.Hash)
		toolTraceFields.ResourceBindings = append([]resourcebinding.ResourceBindingSnapshot(nil), req.ResourceBindings...)
		toolTraceFields.ResourceRoleBindings = append([]resourcebinding.ResourceRoleBinding(nil), req.ResourceRoleBindings...)
		toolTraceFields.ResourceEvidenceRefs = append([]resourcebinding.EvidenceRef(nil), req.ResourceEvidenceRefs...)
		toolTraceFields.ResourceCapabilities = append([]resourcebinding.ResourceCapability(nil), req.ResourceCapabilities...)
		if len(toolTraceFields.ResourceCapabilities) == 0 {
			toolTraceFields.ResourceCapabilities = resourceCapabilitiesFromAssembledTools(toolTraceFields.ResourceBindings, compileCtx.AssembledTools, surfacePolicy.Hash)
		}
		webSearchTrace := promptInputWebSearchTrace(snapshot, dispatchTools)
		finalTrace := promptInputFinalTrace(snapshot)
		finalEvidenceTrace := BuildFinalEvidenceState(snapshot, session)
		planRequirementTrace := planRequirementDecisionTrace(EvaluatePlanRequirement(snapshot.TaskDepth, snapshot, false))
		planCompletionDecision, planCompletionPresent := evaluateRuntimePlanCompletionGate(session, snapshot)
		planCompletionTrace := planCompletionGateTrace(planCompletionDecision, planCompletionPresent)
		verificationCompletionDecision := EvaluateVerificationCompletionGate(snapshot.TaskDepth, snapshot)
		verificationCompletionTrace := verificationCompletionGateTrace(verificationCompletionDecision)
		uxProgressTrace := BuildUXProgressTrace(snapshot)
		evidenceCoverageDecision := EvaluateEvidenceCoverageGate(snapshot)
		assemblyTrace, assemblyTraceErr := buildAgentAssemblyTraceSnapshots(agentAssemblyTraceInput{
			AgentKind:              agentKind,
			SessionType:            req.SessionType,
			Mode:                   req.Mode,
			Metadata:               turnMetadata,
			CompileContext:         compileCtx,
			Compiled:               compiled,
			ToolSurfacePolicy:      surfacePolicy,
			ToolSurfaceFingerprint: toolFingerprint,
			ResourceBindings:       toolTraceFields.ResourceBindings,
			SessionTargetSnapshot:  req.SessionTargetSnapshot,
			RoleBindings:           toolTraceFields.ResourceRoleBindings,
			TurnAssembly:           snapshot.TurnAssembly,
		})
		if assemblyTraceErr != nil {
			return "", nil, fmt.Errorf("turn assembly validation failed")
		}
		snapshot.TurnAssemblyShadow = assemblyTrace.Shadow
		debugConfig := k.runtimeDebugConfig(ctx)
		tracePath, _ := writeRuntimeStepTrace(modeltrace.Config{
			Enabled: debugConfig.ModelInputTrace,
			RootDir: debugConfig.ModelInputTraceRoot,
		}, stepCtx, RuntimeTraceDebugRequest{
			Metadata:                      turnMetadata,
			ModelInput:                    append([]promptinput.ModelInputItem(nil), stepCtx.ProviderRequest.Input...),
			PreviousPromptFingerprint:     latestRuntimePromptFingerprint(snapshot),
			PromptInputTrace:              promptBuild.Trace,
			PromptInputDiff:               promptInputDiff,
			DiagnosticTrace:               buildRuntimeDiagnosticTrace(turnID, session, req, compileCtx),
			TaskDepth:                     snapshot.TaskDepth,
			UXProgressTrace:               &uxProgressTrace,
			EvidenceCoverage:              &evidenceCoverageDecision,
			PlanRequirementDecision:       planRequirementTrace,
			PlanCompletionGate:            planCompletionTrace,
			VerificationReportRef:         verificationCompletionDecision.ReportRef,
			VerificationStatus:            string(verificationCompletionDecision.Status),
			CompletionGate:                verificationCompletionTrace,
			SafetySignals:                 toolTraceFields.SafetySignals,
			UnexpectedStateGate:           toolTraceFields.UnexpectedStateGate,
			ApprovalScope:                 toolTraceFields.ApprovalScope,
			ReasoningEffort:               compileCtx.ReasoningEffort,
			AnswerStyle:                   compileCtx.AnswerStyle,
			AssemblySource:                toolTraceFields.AssemblySource,
			PromptCompilerSource:          toolTraceFields.PromptCompilerSource,
			ToolSurfaceSource:             toolTraceFields.ToolSurfaceSource,
			AdapterName:                   toolTraceFields.AdapterName,
			ToolSurfaceFingerprint:        toolTraceFields.ToolSurfaceFingerprint,
			ToolSurfacePolicySnapshotHash: toolTraceFields.ToolSurfacePolicySnapshotHash,
			ToolSurfaceSnapshot:           toolTraceFields.ToolSurfaceSnapshot,
			PublicWebBudget:               toolTraceFields.PublicWebBudget,
			WebSearch:                     webSearchTrace,
			Final:                         finalTrace,
			LoadedToolsDelta:              toolTraceFields.LoadedToolsDelta,
			LoadedPacksDelta:              toolTraceFields.LoadedPacksDelta,
			SkillIndexHash:                toolTraceFields.SkillIndexHash,
			LoadedSkillsDelta:             toolTraceFields.LoadedSkillsDelta,
			ToolSearchEvents:              toolTraceFields.ToolSearchEvents,
			ToolSelectionEvents:           toolTraceFields.ToolSelectionEvents,
			RejectedToolCalls:             toolTraceFields.RejectedToolCalls,
			SkillSearchEvents:             toolTraceFields.SkillSearchEvents,
			SkillReadEvents:               toolTraceFields.SkillReadEvents,
			RejectedSkillActivations:      toolTraceFields.RejectedSkillActivations,
			MCPInstructionDeltas:          toolTraceFields.MCPInstructionDeltas,
			ParallelDispatchGroups:        toolTraceFields.ParallelDispatchGroups,
			TaskClaims:                    taskClaimTracesFromSnapshot(snapshot),
			ResourceBindings:              toolTraceFields.ResourceBindings,
			ResourceRoleBindings:          toolTraceFields.ResourceRoleBindings,
			ResourceCapabilities:          toolTraceFields.ResourceCapabilities,
			ResourceEvidenceRefs:          toolTraceFields.ResourceEvidenceRefs,
			SpecialInputWorldState:        toolTraceFields.SpecialInputWorldState,
			SessionTargetSnapshot:         req.SessionTargetSnapshot,
			RoleBindingConflicts:          req.RoleBindingConflicts,
			AgentAssemblySnapshot:         assemblyTrace.Projected,
			LegacyAgentAssemblySnapshot:   assemblyTrace.Legacy,
			TurnAssembly:                  snapshot.TurnAssembly,
			TurnAssemblyShadow:            assemblyTrace.Shadow,
			ResourceLocks:                 toolTraceFields.ResourceLocks,
			OwnerWriteTraces:              toolTraceFields.OwnerWriteTraces,
			FailedToolSummaries:           toolTraceFields.FailedToolSummaries,
			FinalEvidenceState:            &finalEvidenceTrace,
		}, &stepReference)
		traceFile := modelTraceMarkdownPath(tracePath)
		traceDiffFile := ""
		if promptInputDiff != nil {
			traceDiffFile = modelTraceDiffPath(tracePath)
		}
		traceCopy := promptBuild.Trace
		previousPromptInputTrace = &traceCopy
		modelItemID := modelCallItemID(turnID, iteration)
		appendAgentItem(snapshot, newAgentItem(
			modelItemID,
			agentstate.TurnItemTypeModelCall,
			agentstate.ItemStatusRunning,
			"calling model",
			map[string]any{
				"iteration":                iteration,
				"visibleTools":             visibleToolNames,
				"traceFile":                traceFile,
				"traceDiffFile":            traceDiffFile,
				"promptFingerprint":        promptFingerprint,
				"promptShadowParity":       stepCtx.PromptShadowParity,
				"taskDepth":                snapshot.TaskDepth,
				"uxProgressTrace":          uxProgressTrace,
				"evidenceCoverageDecision": evidenceCoverageDecision,
			},
		))
		k.persistTurnSnapshot(session, snapshot)
		validatedProviderRequest, validationErr := stepCtx.ValidatedProviderRequest()
		if validationErr != nil {
			return "", nil, fmt.Errorf("runtime step validation failed")
		}
		if recordErr := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
			StepID:           stepReference.StepHash,
			Kind:             modeltrace.CanonicalRolloutKindProviderRequest,
			TurnAssemblyHash: snapshot.TurnAssembly.Hash,
			StepContextHash:  stepCtx.Hash,
			Payload: map[string]any{
				"iteration":             iteration,
				"modelInputHash":        stepCtx.ProviderRequest.ModelInputHash,
				"requestPropertiesHash": stepCtx.ProviderRequest.RequestPropertiesHash,
				"toolCount":             len(stepCtx.ProviderRequest.Tools),
			},
		}); recordErr != nil {
			return "", nil, fmt.Errorf("record provider request: %w", recordErr)
		}
		k.persistTurnSnapshot(session, snapshot)
		reasoningSummaries := map[string]modelrouter.ReasoningStreamEvent{}
		reasoningOrder := make([]string, 0, 2)
		modelCtx := ctx
		modelSpanCtx, modelSpan := k.runtimeObserver().StartModelCall(ctx, ModelCallSpanAttrs{
			SessionID:         session.ID,
			TurnID:            turnID,
			Iteration:         iteration,
			ModelName:         modelNameForTrace(chatModel),
			PromptStableHash:  stablePromptHash,
			PromptFingerprint: promptFingerprint,
			VisibleTools:      visibleToolNames,
			MessageCount:      len(promptBuild.Items),
			TraceFile:         traceFile,
			TraceDiffFile:     traceDiffFile,
		})
		if modelSpanCtx != nil {
			modelCtx = modelSpanCtx
		}
		assistantMessageID := assistantMessageItemID(turnID, iteration)
		iterationAssistantOutput := ""
		modelCallStartedAt := time.Now()
		streamStats := ModelStreamStats{}
		var firstDeltaAt time.Time
		var lastDeltaAt time.Time
		effectiveProviderConfig := k.modelRouter.ResolveEffectiveProviderConfig(agentKind, modelrouter.ProviderConfig{})
		providerAdapter := modelrouter.NewEinoProviderAdapter(chatModel, modelrouter.WithEinoTools(toolPool), modelrouter.WithEinoRequestTimeoutMs(effectiveProviderConfig.RequestTimeoutMs))
		k.observeRuntimeStage(modelCtx, session.ID, turnID, iteration, "provider_request_started")
		providerResponse, genErr := providerAdapter.Call(modelCtx, validatedProviderRequest, func(delta string) {
			if delta != "" {
				now := time.Now()
				if firstDeltaAt.IsZero() {
					firstDeltaAt = now
					streamStats.FirstDeltaMs = durationMilliseconds(now.Sub(modelCallStartedAt))
				}
				lastDeltaAt = now
				streamStats.DeltaCount++
				streamStats.OutputChars += utf8.RuneCountInString(delta)
				iterationAssistantOutput += delta
				upsertAssistantMessageItem(snapshot, assistantMessageID, agentstate.ItemStatusRunning, iterationAssistantOutput, unclassifiedAssistantMessageData(assistantMessageData{
					Iteration:          iteration,
					TextHash:           debugTextHash(iterationAssistantOutput),
					GenerationDuration: time.Since(modelCallStartedAt),
				}, AssistantMessageStreamStateStreaming))
				if streamStats.DeltaCount == 1 || streamStats.DeltaCount%500 == 0 {
					fields := debugTextFacts(iterationAssistantOutput)
					fields["assistantMessageID"] = assistantMessageID
					fields["deltaCount"] = streamStats.DeltaCount
					fields["phase"] = string(AssistantMessagePhaseUnclassified)
					fields["streamState"] = string(AssistantMessageStreamStateStreaming)
					debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "assistant_output_accumulating", snapshot, fields)
				}
				snapshot.UpdatedAt = time.Now()
				k.persistTurnSnapshot(session, snapshot)
			}
		}, func(event modelrouter.ReasoningStreamEvent) {
			if event.Raw || event.PartAdded || strings.TrimSpace(event.Delta) == "" {
				return
			}
			key := reasoningSummaryKey(event)
			current, found := reasoningSummaries[key]
			if !found {
				current = event
				current.Delta = ""
				reasoningOrder = append(reasoningOrder, key)
			}
			current.Delta = event.Delta
			current.Summary += event.Delta
			current.ItemID = event.ItemID
			current.ThreadID = event.ThreadID
			current.TurnID = event.TurnID
			current.SummaryIndex = event.SummaryIndex
			current.Method = event.Method
			reasoningSummaries[key] = current
			// Update the in-memory TurnSnapshot reasoning AgentItem with accumulated text.
			updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusRunning, current.Summary)
			// Throttled persistence: persist at most every 100ms so the transport
			// polling loop detects fingerprint changes without overwhelming storage.
			if time.Since(lastReasoningPersist) >= 100*time.Millisecond {
				snapshot.UpdatedAt = time.Now()
				k.persistTurnSnapshot(session, snapshot)
				lastReasoningPersist = time.Now()
			}
			k.emitRuntimeEvent(EventReasoningSummaryDelta, session.ID, turnID, map[string]any{
				"itemId":       reasoningItemID(event),
				"summaryIndex": event.SummaryIndex,
				"delta":        event.Delta,
				"summary":      current.Summary,
				"foldable":     true,
			})
		})
		providerStatus := "completed"
		if genErr != nil {
			providerStatus = "failed"
		}
		if recordErr := k.appendCanonicalRolloutEvent(ctx, snapshot, modeltrace.CanonicalRolloutEvent{
			StepID:           stepReference.StepHash,
			Kind:             modeltrace.CanonicalRolloutKindProviderResponse,
			TurnAssemblyHash: snapshot.TurnAssembly.Hash,
			StepContextHash:  stepCtx.Hash,
			Payload: map[string]any{
				"finishReason":  canonicalRolloutProviderFinishReason(providerResponse),
				"iteration":     iteration,
				"outputHash":    promptContentHash(iterationAssistantOutput),
				"status":        providerStatus,
				"toolCallCount": len(providerResponse.ToolCalls),
			},
		}); recordErr != nil {
			finishObservedSpan(modelSpan, "failed", "canonical rollout provider response append failed", nil)
			updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusFailed, "canonical rollout provider response append failed")
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("record provider response: %w", recordErr)
		}
		k.persistTurnSnapshot(session, snapshot)
		modelCallDuration := time.Since(modelCallStartedAt)
		if !firstDeltaAt.IsZero() {
			streamEnd := time.Now()
			if streamEnd.Before(lastDeltaAt) {
				streamEnd = lastDeltaAt
			}
			streamStats.StreamMs = durationMilliseconds(streamEnd.Sub(firstDeltaAt))
		}
		{
			fields := debugTextFacts(iterationAssistantOutput)
			fields["assistantMessageID"] = assistantMessageID
			fields["finishReason"] = providerResponseFinishReason(providerResponse)
			fields["durationMs"] = modelCallDuration.Milliseconds()
			fields["firstDeltaMs"] = streamStats.FirstDeltaMs
			fields["streamMs"] = streamStats.StreamMs
			fields["deltaCount"] = streamStats.DeltaCount
			debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "model_response_complete", snapshot, fields)
		}
		if genErr != nil {
			appendModelTraceResponse(tracePath, stepCtx.ProviderRequest.ModelInputHash, modelrouter.ProviderResponse{}, modelCallDuration, genErr, streamStats)
			hasAssistantMessageDraft := strings.TrimSpace(iterationAssistantOutput) != ""
			if errors.Is(genErr, context.Canceled) || snapshot.Lifecycle == TurnLifecycleCanceled {
				finishObservedSpan(modelSpan, "cancelled", genErr.Error(), map[string]any{"error": genErr.Error()})
				updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusCancelled, "模型调用已取消")
				if hasAssistantMessageDraft {
					upsertAssistantMessageItem(snapshot, assistantMessageID, agentstate.ItemStatusCancelled, iterationAssistantOutput, unclassifiedAssistantMessageData(assistantMessageData{
						Iteration:        iteration,
						EvidenceBoundary: "blocked",
						BoundaryAction:   FinalMessageBoundaryBlock,
						Duration:         modelCallDuration,
					}, AssistantMessageStreamStateIncomplete))
				}
				if snapshot.Lifecycle != TurnLifecycleCanceled {
					if _, cancelErr := k.markTurnCanceledRecorded(ctx, session, snapshot, "user stop"); cancelErr != nil {
						return "", nil, cancelErr
					}
				} else {
					snapshot.UpdatedAt = time.Now()
					k.persistTurnSnapshot(session, snapshot)
				}
				return "", nil, genErr
			}
			if isRecoverableModelTimeout(genErr) && !hasAssistantMessageDraft {
				finishObservedSpan(modelSpan, "blocked", genErr.Error(), map[string]any{"error": genErr.Error(), "recoverable": true})
				updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusFailed, genErr.Error())
				appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, genErr.Error(), map[string]any{
					"recoverable": true,
					"checkpoint":  "model_timeout",
				}))
				blockedResult, transitionErr := k.markTurnResumableFromModelTimeout(session, snapshot, iteration, genErr)
				if transitionErr != nil {
					return "", nil, transitionErr
				}
				return "", &blockedResult, nil
			}
			finishObservedSpan(modelSpan, "failed", genErr.Error(), map[string]any{"error": genErr.Error()})
			updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusFailed, genErr.Error())
			if hasAssistantMessageDraft {
				failAssistantMessageItem(snapshot, assistantMessageID, firstNonEmptyString(iterationAssistantOutput, genErr.Error()), unclassifiedAssistantMessageData(assistantMessageData{
					Iteration:        iteration,
					EvidenceBoundary: "blocked",
					BoundaryAction:   FinalMessageBoundaryBlock,
					Duration:         modelCallDuration,
				}, AssistantMessageStreamStateIncomplete))
			}
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, genErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, genErr
		}
		appendModelTraceResponse(tracePath, modelItemID, providerResponse, modelCallDuration, nil, streamStats)
		toolCallCount := len(providerResponse.ToolCalls)
		finishObservedSpan(modelSpan, "completed", "", map[string]any{
			"output.has_tool_calls":  toolCallCount > 0,
			"output.tool_call_count": toolCallCount,
		})
		// Mark the reasoning AgentItem as completed, preserving the accumulated
		// reasoning summary text (pass empty summary so updateAgentItem keeps existing).
		updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusCompleted, "")
		snapshot.UpdatedAt = time.Now()
		k.persistTurnSnapshot(session, snapshot)
		for _, key := range reasoningOrder {
			event := reasoningSummaries[key]
			summary := strings.TrimSpace(event.Summary)
			if summary == "" {
				continue
			}
			k.emitRuntimeEvent(EventReasoningSummaryCompleted, session.ID, turnID, map[string]any{
				"itemId":       reasoningItemID(event),
				"summaryIndex": event.SummaryIndex,
				"summary":      summary,
				"foldable":     true,
				"autoCollapse": true,
			})
		}

		checkpoint := newCheckpointMetadata(session.ID, turnID, iteration, len(snapshot.Iterations)+1, "assistant_response", TurnLifecycleRunning, TurnResumeStateNone)
		iterState := IterationState{
			ID:                      fmt.Sprintf("%s-iter-%d", turnID, iteration),
			SessionID:               session.ID,
			TurnID:                  turnID,
			Iteration:               iteration,
			Lifecycle:               TurnLifecycleRunning,
			ResumeState:             TurnResumeStateNone,
			MessagesForModel:        append([]Message(nil), contextMessages...),
			ToolProgress:            nil,
			ToolSurfaceFingerprint:  toolFingerprint,
			ToolSurfaceSnapshot:     &toolSurfaceSnapshot,
			VisibleTools:            visibleToolNames,
			RefreshedTools:          refreshedTools,
			PromptDelta:             promptcompiler.CompiledPromptDynamicText(compiled),
			PromptFingerprint:       promptFingerprint,
			PromptShadowParity:      stepCtx.PromptShadowParity,
			ModelInputTraceFile:     tracePath,
			TokenBudget:             session.Context.MaxTokens,
			Checkpoint:              checkpoint,
			StepReference:           &stepReference,
			CompactedSegments:       append([]CompactedSegment(nil), contextState.CompactedSegments...),
			ExternalReferences:      append([]ExternalReference(nil), contextState.ExternalReferences...),
			ContextGovernanceEvents: append([]ContextGovernanceEvent(nil), contextState.GovernanceEvents...),
			StartedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}

		assistantMsg := runtimeMessageFromProviderResponse(providerResponse)
		if assistantMsg.ID == "" {
			assistantMsg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
		}
		if assistantMsg.Timestamp.IsZero() {
			assistantMsg.Timestamp = time.Now()
		}
		if len(assistantMsg.ToolCalls) == 0 {
			if rawCalls := rawToolCallsFromAssistantText(assistantMsg.Content, turnID, iteration); len(rawCalls) > 0 {
				assistantMsg.ToolCalls = rawCalls
				assistantMsg.Content = ""
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata["rawToolCallMarkupParsed"] = "true"
			}
		}
		if err := k.recordCanonicalToolProposals(ctx, snapshot, assistantMsg.ToolCalls); err != nil {
			return "", nil, fmt.Errorf("record tool proposals: %w", err)
		}
		iterState.ToolCalls = append(iterState.ToolCalls, assistantMsg.ToolCalls...)
		appendProviderNativeWebSearchTurnItems(snapshot, &iterState, turnID, providerResponse.NativeWebSearchEvents)
		assistantMessageCommitted := false
		if len(assistantMsg.ToolCalls) > 0 {
			session.Messages = append(session.Messages, assistantMsg)
			assistantMessageCommitted = true
		}
		snapshot.Iteration = iteration
		snapshot.StablePromptHash = stablePromptHash
		snapshot.StableToolFingerprint = toolFingerprint
		snapshot.ToolSurfaceSnapshot = &toolSurfaceSnapshot
		snapshot.UpdatedAt = time.Now()
		snapshot.LatestCheckpoint = checkpoint
		appendCompactedSegments(&snapshot.CompactedSegments, contextState.CompactedSegments...)
		appendCompactedSegments(&session.CompactedSegments, contextState.CompactedSegments...)
		appendExternalReferences(&snapshot.ExternalReferences, contextState.ExternalReferences...)
		appendExternalReferences(&session.ExternalReferences, contextState.ExternalReferences...)
		snapshot.Iterations = append(snapshot.Iterations, iterState)
		session.LatestCheckpoint = checkpoint
		k.persistTurnSnapshot(session, snapshot)

		assistantContent := strings.TrimSpace(iterationAssistantOutput)
		if assistantContent == "" {
			assistantContent = strings.TrimSpace(assistantMsg.Content)
		}

		if len(assistantMsg.ToolCalls) == 0 {
			if shouldGuardPrematureFinal(snapshot.TaskDepth, snapshot, iteration, assistantContent) {
				markPrematureFinalGuard(snapshot)
				additionalContext = append(additionalContext, prematureFinalGuardPrompt(snapshot.TaskDepth))
				markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			planCompletionDecision, planCompletionPresent := evaluateRuntimePlanCompletionGate(session, snapshot)
			if planCompletionPresent && !planCompletionGateAllowsFinal(assistantContent, planCompletionDecision) && snapshot.Metadata[planCompletionGateRetryMetadataKey] != "1" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata[planCompletionGateRetryMetadataKey] = "1"
				additionalContext = append(additionalContext, planCompletionGateRetryPrompt(planCompletionDecision))
				markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			verificationCompletionDecision := EvaluateVerificationCompletionGate(snapshot.TaskDepth, snapshot)
			appendVerificationCompletionGateItem(snapshot, turnID, iteration, verificationCompletionDecision)
			if !verificationCompletionGateAllowsFinal(assistantContent, verificationCompletionDecision, snapshot) && snapshot.Metadata[verificationCompletionGateRetryMetadataKey] != "1" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata[verificationCompletionGateRetryMetadataKey] = "1"
				additionalContext = append(additionalContext, verificationCompletionGateRetryPrompt(verificationCompletionDecision))
				markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			managerGate := EvaluateManagerSynthesisGate(snapshot, assistantContent)
			if managerGate.Action != "allow_final" && snapshot.Metadata["managerSynthesisRetry"] != "1" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata["managerSynthesisRetry"] = "1"
				additionalContext = append(additionalContext, managerSynthesisRetryPrompt(managerGate))
				markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			if coverageGateMetadataPresent(snapshot) {
				completionDecision := EvaluateCompletionReadiness(snapshot, assistantContent)
				if completionDecision.Action == "block_success_final" && snapshot.Metadata["completionReadinessRetry"] != "1" {
					if snapshot.Metadata == nil {
						snapshot.Metadata = map[string]string{}
					}
					snapshot.Metadata["completionReadinessRetry"] = "1"
					additionalContext = append(additionalContext, completionReadinessRetryPrompt(completionDecision))
					markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
					fields := debugTextFacts(assistantContent)
					fields["assistantMessageID"] = assistantMessageID
					fields["retryReason"] = "completion_readiness"
					fields["streamState"] = string(AssistantMessageStreamStateIncomplete)
					fields["decisionAction"] = completionDecision.Action
					fields["decisionReasons"] = completionDecision.Reasons
					debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "assistant_message_replaced_for_retry", snapshot, fields)
					k.persistTurnSnapshot(session, snapshot)
					continue
				}
			}
			mandatorySkillDecision := EvaluateMandatorySkillActivation(k.mandatorySkillDefinitionsForInput(req.Input), req.Input, assistantContent, session.SkillActivation)
			if mandatorySkillDecision.Action == "require_skill_read" && !mandatorySkillActivationSatisfiedByToolSurface(mandatorySkillDecision, compileCtx.AssembledTools) {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				if snapshot.Metadata["mandatorySkillActivationRetry"] != "1" {
					snapshot.Metadata["mandatorySkillActivationRetry"] = "1"
					additionalContext = append(additionalContext, mandatorySkillRetryPrompt(mandatorySkillDecision))
					markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
					fields := debugTextFacts(assistantContent)
					fields["assistantMessageID"] = assistantMessageID
					fields["retryReason"] = "mandatory_skill_activation"
					fields["streamState"] = string(AssistantMessageStreamStateIncomplete)
					fields["decisionAction"] = mandatorySkillDecision.Action
					fields["decisionReasons"] = mandatorySkillDecision.Reasons
					debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "assistant_message_replaced_for_retry", snapshot, fields)
					k.persistTurnSnapshot(session, snapshot)
					continue
				}
				session.SkillActivation.AddRejectedActivation(RejectedSkillActivation{
					SkillName:      strings.Join(mandatorySkillDecision.RequiredSkills, ", "),
					Reason:         strings.Join(mandatorySkillDecision.Reasons, ", "),
					RequiredAction: "skill_read",
					TurnID:         turnID,
				}, time.Now())
			}
			agentFinalGate := EvaluateRuntimeAgentFinalGate("", runtimeAgentNotifications(snapshot))
			if agentFinalGate.Action == "require_wait" {
				appendAgentItem(snapshot, newAgentItem(
					fmt.Sprintf("%s-agent-final-gate-%d", turnID, iteration),
					agentstate.TurnItemTypeEvidence,
					agentstate.ItemStatusBlocked,
					"worker completion gate: require_wait",
					agentFinalGate,
				))
				if snapshot.Metadata[runtimeAgentFinalGateRetryMetadataKey] != "1" {
					if snapshot.Metadata == nil {
						snapshot.Metadata = map[string]string{}
					}
					snapshot.Metadata[runtimeAgentFinalGateRetryMetadataKey] = "1"
					additionalContext = append(additionalContext, runtimeAgentFinalGateRetryPrompt(agentFinalGate))
					markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "blocked", FinalMessageBoundaryRetryOnce)
					k.persistTurnSnapshot(session, snapshot)
					continue
				}
			}
			finalCompletenessDecision := EvaluateFinalCompleteness(assistantContent, providerResponseFinishReason(providerResponse))
			if finalCompletenessDecision.Action == "retry_complete_final" && snapshot.Metadata[finalCompletenessRetryMetadataKey] != "1" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata[finalCompletenessRetryMetadataKey] = "1"
				additionalContext = append(additionalContext, finalCompletenessRetryPrompt(finalCompletenessDecision))
				markAssistantMessageReplacedForRetry(snapshot, assistantMessageID, assistantContent, assistantMsg.ID, iteration, modelCallDuration, "limited", FinalMessageBoundaryRetryOnce)
				fields := debugTextFacts(assistantContent)
				fields["assistantMessageID"] = assistantMessageID
				fields["retryReason"] = "final_completeness"
				fields["streamState"] = string(AssistantMessageStreamStateIncomplete)
				fields["decisionAction"] = finalCompletenessDecision.Action
				fields["decisionReasons"] = finalCompletenessDecision.Reasons
				debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "assistant_message_replaced_for_retry", snapshot, fields)
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			if finalCompletenessDecision.Action == "retry_complete_final" {
				incompleteErr := finalCompletenessFailureError(finalCompletenessDecision)
				snapshot.FinalOutput = ""
				failAssistantMessageItem(snapshot, assistantMessageID, assistantContent, unclassifiedAssistantMessageData(assistantMessageData{
					MessageID:        assistantMsg.ID,
					Iteration:        iteration,
					EvidenceBoundary: "blocked",
					BoundaryAction:   FinalMessageBoundaryBlock,
					TextHash:         debugTextHash(assistantContent),
					Duration:         modelCallDuration,
				}, AssistantMessageStreamStateIncomplete))
				if !assistantMessageCommitted {
					assistantMsg.Content = assistantContent
					session.Messages = append(session.Messages, assistantMsg)
					assistantMessageCommitted = true
				}
				appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, incompleteErr.Error(), nil))
				if transitionErr := k.markTurnFailedFromError(session, snapshot, incompleteErr, "assistant_message_incomplete"); transitionErr != nil {
					return "", nil, transitionErr
				}
				return "", nil, incompleteErr
			}
			finalRuntimeFacts := BuildFinalRuntimeFactsWithContext(ctx, snapshot, session, k.finalCompletionEvaluator())
			finalEvidenceDecision := finalRuntimeFacts.EvidenceDecision
			boundaryDecision := finalMessageBoundaryDecision{
				Action:           FinalMessageBoundaryAllow,
				EvidenceBoundary: finalEvidenceBoundaryFromFacts(finalRuntimeFacts),
			}
			if sanitized, changed := sanitizeFinalAssistantContentForCommit(assistantContent, finalEvidenceDecision); changed {
				recordRawToolCallMarkupFinalSanitized(snapshot, turnID, iteration, assistantContent)
				fields := debugTextFacts(assistantContent)
				fields["replaceReason"] = "raw_tool_call_markup_final"
				debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "assistant_message_constrained_before_final", snapshot, fields)
				assistantContent = sanitized
				boundaryDecision.Action = FinalMessageBoundaryConstrain
				boundaryDecision.Reasons = []string{"raw_tool_call_markup_sanitized"}
			}
			finalContract := BuildFinalContract(assistantContent, finalRuntimeFacts)
			if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted); err != nil {
				return "", nil, err
			}
			finalCommit := assistantOutputCommitInput{
				TurnID:           turnID,
				Iteration:        iteration,
				MessageID:        assistantMsg.ID,
				AssistantText:    assistantContent,
				Duration:         modelCallDuration,
				FinishReason:     providerResponseFinishReason(providerResponse),
				EvidenceBoundary: boundaryDecision.EvidenceBoundary,
				BoundaryAction:   boundaryDecision.Action,
				EvidenceRefs:     assistantMessageEvidenceRefsFromSnapshot(snapshot),
				FinalContract:    &finalContract,
			}
			recordSnapshot := *snapshot
			recordSnapshot.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
			recordSnapshot.Lifecycle = TurnLifecycleCompleted
			recordSnapshot.ResumeState = TurnResumeStateNone
			recordSnapshot.PendingApprovals = nil
			recordSnapshot.PendingEvidence = nil
			commitFinalAssistantOutput(&recordSnapshot, finalCommit)
			if err := k.recordCanonicalFinalFacts(ctx, &recordSnapshot, finalRuntimeFacts, finalContract); err != nil {
				snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
				return "", nil, err
			}
			checkpointRef := checkpointIDForStepCause(snapshot)
			if err := k.recordCanonicalTransportProjection(ctx, &recordSnapshot, TurnLifecycleCompleted, TurnResumeStateNone, checkpointRef, &finalContract); err != nil {
				snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
				return "", nil, err
			}
			snapshot.CanonicalRolloutHead = recordSnapshot.CanonicalRolloutHead
			now := time.Now()
			if !assistantMessageCommitted {
				assistantMsg.Content = assistantContent
				session.Messages = append(session.Messages, assistantMsg)
				assistantMessageCommitted = true
			}
			snapshot.Lifecycle = TurnLifecycleCompleted
			snapshot.ResumeState = TurnResumeStateNone
			commitFinalAssistantOutput(snapshot, finalCommit)
			snapshot.FinalOutput = FinalTextFromAssistantMessage(snapshot)
			appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
			appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteAssistantMessage, OwnerRuntimeKernel)
			snapshot.UpdatedAt = now
			snapshot.CompletedAt = &now
			if last := latestIteration(snapshot); last != nil {
				last.Lifecycle = TurnLifecycleCompleted
				last.ResumeState = TurnResumeStateNone
				last.UpdatedAt = now
				last.CompletedAt = &now
			}
			session.PendingApprovals = nil
			session.PendingEvidence = nil
			{
				fields := debugAssistantMessageFacts(snapshot, assistantMessageID, assistantContent, map[string]any{
					"finalContract":       "final",
					"finalEvidenceAction": string(finalEvidenceDecision.Action),
					"commitAllowed":       true,
				})
				fields["assistantMessageID"] = assistantMessageID
				fields["finalStatus"] = string(agentstate.ItemStatusCompleted)
				fields["durationMs"] = modelCallDuration.Milliseconds()
				debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "final_committed", snapshot, fields)
			}
			k.persistTurnSnapshot(session, snapshot)
			if finalRuntimeFacts.PolicyCompletion.Action != policyengine.PolicyActionAllow {
				return assistantContent, &TurnResult{
					SessionType:     req.SessionType,
					Mode:            req.Mode,
					SessionID:       session.ID,
					TurnID:          turnID,
					ClientTurnID:    req.ClientTurnID,
					ClientMessageID: req.ClientMessageID,
					Status:          "blocked",
					Error:           finalRuntimeFacts.PolicyCompletion.Reason,
				}, nil
			}
			return assistantContent, nil, nil
		}

		commitResult := commitAssistantOutputForIteration(snapshot, assistantOutputCommitInput{
			TurnID:        turnID,
			Iteration:     iteration,
			MessageID:     assistantMsg.ID,
			UserInput:     req.Input,
			AssistantText: assistantContent,
			ToolCalls:     assistantMsg.ToolCalls,
			Duration:      modelCallDuration,
			FinishReason:  providerResponseFinishReason(providerResponse),
		})
		if len(assistantMsg.ToolCalls) > 0 {
			fields := debugTextFacts(firstNonEmptyString(commitResult.Text, assistantContent))
			fields["assistantMessageID"] = assistantMessageID
			fields["phase"] = string(AssistantMessagePhaseCommentary)
			fields["commentarySource"] = commitResult.CommentarySource
			fields["suppressedRawDraft"] = commitResult.SuppressedRawDraft
			fields["hasToolCalls"] = true
			fields["toolCallCount"] = len(assistantMsg.ToolCalls)
			debugFinalStateLog(debugConfig, session.ID, turnID, iteration, "assistant_commentary_before_tools_committed", snapshot, fields)
			k.persistTurnSnapshot(session, snapshot)
		} else if snapshot.FinalOutput != "" {
			snapshot.FinalOutput = ""
			snapshot.UpdatedAt = time.Now()
			k.persistTurnSnapshot(session, snapshot)
		}

		dispatchExpectedPermissionHash := stepPermissionHash
		if snapshot.ToolSurfaceSnapshot != nil && snapshot.ToolSurfaceSnapshot.PolicySnapshot != nil {
			dispatchExpectedPermissionHash = k.runtimePermissionSnapshotHash(snapshot.TurnAssembly, *snapshot.ToolSurfaceSnapshot.PolicySnapshot)
		}
		dispatcher := k.newIterationDispatcher(session, snapshot, iteration, dispatchTools, runtimeToolSurface, dispatcherPermissionBinding{
			expected: dispatchExpectedPermissionHash,
			current:  stepCtx.PermissionHash,
		})
		k.emitIterationStage(session.ID, turnID, iteration, "dispatch_tools", turnSpanID)

		appendToolCallState := func(tc ToolCall) string {
			toolItemID := toolCallItemID(turnID, tc)
			queueToolInvocation(snapshot, iteration, tc, toolMetadataForToolCall(dispatchTools, tc))
			appendAgentItem(snapshot, newAgentItem(
				toolItemID,
				agentstate.TurnItemTypeToolCall,
				agentstate.ItemStatusRunning,
				tc.Name,
				tc,
			))
			return toolItemID
		}

		processDispatchResult := func(tc ToolCall, toolItemID string, dispatchResult DispatchResult) (*TurnResult, error) {
			if dispatchResult.ToolCallID == "" {
				dispatchResult.ToolCallID = tc.ID
			}
			if strings.TrimSpace(dispatchResult.Metadata.Name) == "" {
				dispatchResult.Metadata = toolMetadataForToolCall(dispatchTools, tc)
			}
			appendResourceLockTraces(snapshot, dispatchResult.ResourceLocks)
			appendToolAttemptStates(snapshot, tc.ID, dispatchResult.Attempts)
			recordRejectedToolCallFromDispatch(session, turnID, tc, dispatchResult, time.Now())
			if dispatchResult.Blocked {
				updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusBlocked, dispatchResult.Reason)
				markToolInvocationBlocked(snapshot, tc.ID)
				if transitionErr := k.markTurnBlocked(session, snapshot, tc, dispatchResult); transitionErr != nil {
					return nil, transitionErr
				}
				return &TurnResult{
					SessionType:     req.SessionType,
					Mode:            req.Mode,
					SessionID:       session.ID,
					TurnID:          turnID,
					ClientTurnID:    req.ClientTurnID,
					ClientMessageID: req.ClientMessageID,
					Status:          "blocked",
					Error:           dispatchResult.Reason,
				}, nil
			}
			if dispatchResult.Error != "" {
				if !shouldFeedToolFailureBackToModel(dispatchResult) {
					updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusFailed, dispatchResult.Error)
					appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, dispatchResult.Error, map[string]string{"toolCallId": tc.ID, "toolName": tc.Name}))
					markToolInvocationFailed(snapshot, tc.ID, failureKindForDispatchResult(dispatchResult))
					if transitionErr := k.markTurnFailed(session, snapshot, tc, dispatchResult); transitionErr != nil {
						return nil, transitionErr
					}
					return nil, fmt.Errorf("tool %q failed: %s", tc.Name, dispatchResult.Error)
				}
				dispatchResult.Result = failedToolResultForModel(tc, dispatchResult)
				if decision := repeatedToolFailureSignatureDecision(snapshot, dispatchTools, tc, dispatchResult.Result); decision.Action == "switch_path" {
					dispatchResult.Result = withFailureSignatureDecision(dispatchResult.Result, decision)
					dispatchResult.HiddenTools = mergeStringSets(dispatchResult.HiddenTools, []string{canonicalRuntimeToolName(dispatchTools, tc)})
				}
				if strings.TrimSpace(dispatchResult.Metadata.Name) == "" {
					dispatchResult.Metadata.Name = tc.Name
				}
			}
			applyHiddenTools(snapshot, dispatchResult.HiddenTools)
			dispatchResult.Result = k.autoRecordToolResultEvidence(ctx, session, turnID, tc, dispatchResult.Metadata, dispatchResult.Result)
			recordedResult, materializeErr := k.materializeToolResult(session, snapshot, iteration, tc, dispatchResult.Metadata, dispatchResult.Result)
			if materializeErr != nil {
				updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusFailed, materializeErr.Error())
				appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, materializeErr.Error(), map[string]string{"toolCallId": tc.ID, "toolName": tc.Name}))
				markToolInvocationFailed(snapshot, tc.ID, "")
				k.persistTurnSnapshot(session, snapshot)
				return nil, fmt.Errorf("materialize tool result %q: %w", tc.Name, materializeErr)
			}
			if err := k.recordCanonicalToolResult(ctx, snapshot, tc, recordedResult, failureKindForDispatchResult(dispatchResult)); err != nil {
				return nil, fmt.Errorf("record tool result %q: %w", tc.Name, err)
			}
			checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, iteration, nextCheckpointSequence(snapshot), "tool_result", TurnLifecycleRunning, snapshot.ResumeState)
			checkpoint.Incremental = true
			if err := k.recordCanonicalCheckpoint(ctx, snapshot, checkpoint); err != nil {
				return nil, fmt.Errorf("record tool-result checkpoint: %w", err)
			}
			turnMetadata = updateOpsManualFlowTurnMetadata(turnMetadata, recordedResult)
			turnMetadata = updateToolSearchPackTurnMetadata(turnMetadata, tc.Name, recordedResult)
			applyToolSearchDiscoveryState(session, tc.Name, recordedResult, turnID)
			applySkillDiscoveryState(session, tc.Name, recordedResult, turnID)
			updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusCompleted, tc.Name)
			appendAgentItem(snapshot, newAgentItem(
				toolResultItemID(turnID, tc),
				agentstate.TurnItemTypeToolResult,
				toolResultItemStatus(recordedResult),
				truncateString(recordedResult.Content, 240),
				toolResultAgentItemData(turnID, tc, recordedResult),
			))
			if planItem, ok := planItemFromToolCall(turnID, tc); ok {
				appendAgentItem(snapshot, planItem)
			}
			if recordedResult.Error != "" {
				markToolInvocationFailed(snapshot, tc.ID, failureKindForDispatchResult(dispatchResult))
			} else if recordedResult.Outcome.Normalize() == tooling.ToolResultOutcomePartial {
				markToolInvocationPartial(snapshot, tc.ID)
			} else {
				markToolInvocationCompleted(snapshot, tc.ID)
			}
			toolMsg := Message{
				ID:         fmt.Sprintf("msg-%d", time.Now().UnixNano()),
				Role:       "tool",
				Content:    recordedResult.Content,
				Timestamp:  time.Now(),
				ToolResult: &recordedResult,
			}
			session.Messages = append(session.Messages, toolMsg)
			if last := latestIteration(snapshot); last != nil {
				last.ToolResults = append(last.ToolResults, recordedResult)
				appendExternalReferences(&last.ExternalReferences, recordedResult.ExternalReferences...)
				appendContextGovernanceEvents(&last.ContextGovernanceEvents, latestToolResultGovernanceEvents(session, tc.ID)...)
				last.UpdatedAt = time.Now()
			}
			appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteToolResult, OwnerToolDispatcher)
			k.applyAggregateToolResultBudget(session, snapshot, iteration, dispatchTools)
			snapshot.LatestCheckpoint = checkpoint
			appendExternalReferences(&snapshot.ExternalReferences, recordedResult.ExternalReferences...)
			appendExternalReferences(&session.ExternalReferences, recordedResult.ExternalReferences...)
			if snapshot.LatestCheckpoint != nil {
				appendCheckpointExternalRefs(snapshot.LatestCheckpoint, recordedResult.ExternalReferences)
			}
			if last := latestIteration(snapshot); last != nil {
				last.Checkpoint = snapshot.LatestCheckpoint
			}
			session.LatestCheckpoint = snapshot.LatestCheckpoint
			if err := k.recordCanonicalTransportProjection(ctx, snapshot, TurnLifecycleRunning, snapshot.ResumeState, checkpoint.ID, nil); err != nil {
				return nil, fmt.Errorf("record tool-result projection source: %w", err)
			}
			k.persistTurnSnapshot(session, snapshot)
			return nil, nil
		}

		for i := 0; i < len(assistantMsg.ToolCalls); {
			tc := assistantMsg.ToolCalls[i]
			if canDispatchToolCallInParallel(dispatchTools, tc) && toolDispatches < defaultMaxToolDispatchesPerTurn {
				remaining := defaultMaxToolDispatchesPerTurn - toolDispatches
				batch := make([]ToolCall, 0, remaining)
				pendingPublicWebDispatches := 0
				pendingPublicWebQueries := 0
				for i < len(assistantMsg.ToolCalls) && len(batch) < remaining && canDispatchToolCallInParallel(dispatchTools, assistantMsg.ToolCalls[i]) {
					nextCall := assistantMsg.ToolCalls[i]
					if publicWebBudgetReached(dispatchTools, nextCall, publicWebDispatches+pendingPublicWebDispatches, publicWebQueries+pendingPublicWebQueries, DefaultPublicWebBudget()) {
						break
					}
					nextCall = limitPublicWebToolCall(dispatchTools, nextCall, DefaultPublicWebBudget())
					batch = append(batch, nextCall)
					if isPublicWebToolCall(dispatchTools, nextCall) {
						pendingPublicWebDispatches++
						pendingPublicWebQueries += publicWebQueryCountForToolCall(nextCall)
					}
					i++
				}
				if len(batch) == 0 {
					// Fall through to the sequential path so this call gets a
					// structured budget result instead of silently disappearing.
				} else {
					batch = broadenCoveredReadBatch(dispatchTools, batch)
					toolItemIDs := make([]string, len(batch))
					for j, batchCall := range batch {
						toolItemIDs[j] = appendToolCallState(batchCall)
					}
					recordParallelDispatchGroup(snapshot, turnID, iteration, batch, dispatchTools)
					k.persistTurnSnapshot(session, snapshot)
					for _, batchCall := range batch {
						markToolInvocationRunning(snapshot, batchCall.ID)
					}
					k.persistTurnSnapshot(session, snapshot)

					results := make([]DispatchResult, len(batch))
					dispatchBatch := make([]struct {
						index int
						call  ToolCall
					}, 0, len(batch))
					deferredReuses := make([]coveredReadBatchReuse, 0)
					for j, batchCall := range batch {
						if reused, ok := maybeReuseCoveredReadResult(snapshot, dispatchTools, batchCall); ok {
							results[j] = reused
							continue
						}
						if priorIndex, ok := coveredReadReusePriorIndex(dispatchTools, batch[:j], batchCall); ok {
							deferredReuses = append(deferredReuses, coveredReadBatchReuse{index: j, priorIndex: priorIndex})
							continue
						}
						dispatchBatch = append(dispatchBatch, struct {
							index int
							call  ToolCall
						}{index: j, call: batchCall})
					}
					dispatchedBatchCalls := make([]ToolCall, 0, len(dispatchBatch))
					for _, task := range dispatchBatch {
						dispatchedBatchCalls = append(dispatchedBatchCalls, task.call)
					}
					if err := k.recordCanonicalToolDispatch(ctx, snapshot, dispatchedBatchCalls); err != nil {
						return "", nil, fmt.Errorf("record parallel tool dispatch: %w", err)
					}
					var wg sync.WaitGroup
					for _, task := range dispatchBatch {
						wg.Add(1)
						go func(index int, call ToolCall) {
							defer wg.Done()
							dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(req.HostID, turnMetadata))
							results[index] = dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, turnID, call, req.SessionType, req.Mode, turnSpanID)
						}(task.index, task.call)
					}
					wg.Wait()
					for _, deferred := range deferredReuses {
						if reused, ok := coveredReadReuseFromBatchResult(dispatchTools, batch, results, deferred); ok {
							results[deferred.index] = reused
							continue
						}
						fallbackCall := batch[deferred.index]
						if err := k.recordCanonicalToolDispatch(ctx, snapshot, []ToolCall{fallbackCall}); err != nil {
							return "", nil, fmt.Errorf("record deferred tool dispatch: %w", err)
						}
						dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(req.HostID, turnMetadata))
						results[deferred.index] = dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, turnID, fallbackCall, req.SessionType, req.Mode, turnSpanID)
						dispatchedBatchCalls = append(dispatchedBatchCalls, fallbackCall)
					}
					toolDispatches += countToolCallsTowardBudget(dispatchedBatchCalls)
					publicWebDispatches += countPublicWebToolCalls(dispatchTools, dispatchedBatchCalls)
					publicWebQueries += countPublicWebQueriesForToolCalls(dispatchTools, dispatchedBatchCalls)

					for j, batchCall := range batch {
						blocked, err := processDispatchResult(batchCall, toolItemIDs[j], results[j])
						if blocked != nil || err != nil {
							return "", blocked, err
						}
					}
					continue
				}
			}

			i++
			tc = limitPublicWebToolCall(dispatchTools, tc, DefaultPublicWebBudget())
			toolItemID := appendToolCallState(tc)
			k.persistTurnSnapshot(session, snapshot)
			markToolInvocationRunning(snapshot, tc.ID)
			k.persistTurnSnapshot(session, snapshot)
			dispatchResult := DispatchResult{
				ToolCallID: tc.ID,
				Metadata:   toolMetadataForToolCall(dispatchTools, tc),
			}
			if countsTowardToolBudget(tc) && publicWebBudgetReached(dispatchTools, tc, publicWebDispatches, publicWebQueries, DefaultPublicWebBudget()) {
				dispatchResult.Result = publicWebBudgetReachedResultForModel(tc, publicWebDispatches, publicWebQueries, DefaultPublicWebBudget())
				applyHiddenTools(snapshot, publicWebToolNames(compileCtx.AssembledTools))
			} else if countsTowardToolBudget(tc) && toolDispatches >= defaultMaxToolDispatchesPerTurn {
				dispatchResult.Result = toolBudgetReachedResultForModel(tc, toolDispatches)
				applyHiddenTools(snapshot, toolNames(compileCtx.AssembledTools))
			} else if reused, ok := maybeReuseCoveredReadResult(snapshot, dispatchTools, tc); ok {
				dispatchResult = reused
			} else {
				if err := k.recordCanonicalToolDispatch(ctx, snapshot, []ToolCall{tc}); err != nil {
					return "", nil, fmt.Errorf("record tool dispatch: %w", err)
				}
				dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(req.HostID, turnMetadata))
				dispatchResult = dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, turnID, tc, req.SessionType, req.Mode, turnSpanID)
				if countsTowardToolBudget(tc) {
					toolDispatches++
				}
				if isPublicWebToolCall(dispatchTools, tc) {
					publicWebDispatches++
					publicWebQueries += publicWebQueryCountForToolCall(tc)
				}
			}
			blocked, err := processDispatchResult(tc, toolItemID, dispatchResult)
			if blocked != nil || err != nil {
				return "", blocked, err
			}
		}
		k.emitRuntimeEvent(EventPhaseEnd, session.ID, turnID, map[string]any{
			"phaseId": fmt.Sprintf("%s-iteration-%d", turnID, iteration),
			"summary": "tool iteration completed",
		})
		k.emitRuntimeEvent(EventProcessSummary, session.ID, turnID, map[string]any{
			"phaseId": fmt.Sprintf("%s-iteration-%d", turnID, iteration),
			"summary": "tool iteration completed",
		})
		k.emitIterationStage(session.ID, turnID, iteration, "finalize_iteration", turnSpanID)
	}

	now := time.Now()
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnFailed, TurnLifecycleFailed); err != nil {
		return "", nil, err
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, maxIterations, nextCheckpointSequence(snapshot), "iteration_limit", TurnLifecycleFailed, TurnResumeStateNone)
	candidate := *snapshot
	candidate.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
	candidate.Lifecycle = TurnLifecycleFailed
	candidate.ResumeState = TurnResumeStateNone
	candidate.PendingApprovals = nil
	candidate.PendingEvidence = nil
	candidate.LatestCheckpoint = checkpoint
	appendAgentItem(&candidate, newAgentItem(errorItemID(turnID, maxIterations), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, "iteration limit exceeded", nil))
	if err := k.recordCanonicalTerminalBoundary(ctx, &candidate, checkpoint, FinalContractStatusFailed, "iteration_limit"); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return "", nil, err
	}
	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleFailed
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = "iteration limit exceeded"
	snapshot.UpdatedAt = now
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleFailed
		last.ResumeState = TurnResumeStateNone
		last.Checkpoint = checkpoint
		last.UpdatedAt = now
	}
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, maxIterations), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, "iteration limit exceeded", nil))
	k.persistTurnSnapshot(session, snapshot)
	return "", nil, fmt.Errorf("iteration limit exceeded")
}

const (
	defaultMaxToolDispatchesPerTurn      = 12
	defaultMaxPublicWebDispatchesPerTurn = 3
	defaultPublicWebSynthesisDispatches  = 2
	defaultSynthesisOnlyToolDispatches   = 5
)

type PublicWebBudget struct {
	MaxSearchCalls        int
	MaxQueries            int
	MaxResults            int
	MaxCallsPerTurn       int
	MaxQueriesPerCall     int
	MaxResultsPerDomain   int
	ExplicitUserRequested bool
}

func DefaultPublicWebBudget() PublicWebBudget {
	return PublicWebBudget{
		MaxSearchCalls:      3,
		MaxQueries:          6,
		MaxResults:          8,
		MaxCallsPerTurn:     3,
		MaxQueriesPerCall:   2,
		MaxResultsPerDomain: 2,
	}
}

func shouldSwitchToSynthesisOnly(mode Mode, profile taskdepth.Profile, toolDispatches int, tools []promptcompiler.Tool) bool {
	if len(tools) == 0 {
		return false
	}
	if mode == ModeExecute {
		return toolDispatches >= defaultMaxToolDispatchesPerTurn
	}
	switch profile.Level {
	case taskdepth.LevelInvestigation:
		return toolDispatches >= 5
	case taskdepth.LevelOperations, taskdepth.LevelMultiAgent:
		return toolDispatches >= 12
	case taskdepth.LevelMultiStep:
		return toolDispatches >= 6
	default:
		return toolDispatches >= defaultSynthesisOnlyToolDispatches
	}
}

func shouldSwitchToSynthesisOnlyForTurn(mode Mode, profile taskdepth.Profile, input string, session *SessionState, snapshot *TurnSnapshot, toolDispatches int, tools []promptcompiler.Tool) bool {
	if shouldSwitchToSynthesisOnly(mode, profile, toolDispatches, tools) {
		return true
	}
	return shouldSwitchToHostResourceSynthesis(profile, input, session, snapshot, toolDispatches, tools)
}

func shouldSwitchToPublicWebSynthesisOnly(publicWebDispatches int, tools []promptcompiler.Tool) bool {
	if publicWebDispatches < defaultPublicWebSynthesisDispatches {
		return false
	}
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		if strings.TrimSpace(toolDef.Metadata().Pack) == "public_web" {
			return true
		}
	}
	return false
}

func shouldSwitchToHostResourceSynthesis(profile taskdepth.Profile, input string, session *SessionState, snapshot *TurnSnapshot, toolDispatches int, tools []promptcompiler.Tool) bool {
	if len(tools) == 0 || toolDispatches == 0 {
		return false
	}
	switch taskdepth.NormalizeLevel(string(profile.Level)) {
	case taskdepth.LevelTrivial, taskdepth.LevelSimpleRead:
	default:
		return false
	}
	if profile.RequiresPlan || profile.RequiresValidation {
		return false
	}
	if !isDirectHostResourceInspection(input, session) {
		return false
	}
	requested := requestedHostResourceDimensions(input)
	if len(requested) == 0 {
		return false
	}
	covered := coveredHostResourceDimensions(snapshot)
	for _, dimension := range requested {
		if !covered[dimension] {
			return false
		}
	}
	return true
}

func requestedHostResourceDimensions(input string) []string {
	text := strings.ToLower(strings.TrimSpace(input))
	dimensions := make([]string, 0, 3)
	if containsAnyFold(text, []string{"cpu", "processor", "load", "uptime", "负载", "使用率"}) {
		dimensions = append(dimensions, "cpu")
	}
	if containsAnyFold(text, []string{"memory", "mem", "swap", "内存"}) {
		dimensions = append(dimensions, "memory")
	}
	if containsAnyFold(text, []string{"disk", "filesystem", "volume", "磁盘", "文件系统"}) {
		dimensions = append(dimensions, "disk")
	}
	if len(dimensions) == 0 && containsAnyFold(text, []string{"resource", "resources", "资源", "系统状态", "状态", "情况"}) {
		dimensions = append(dimensions, "cpu", "memory", "disk")
	}
	return uniqueStrings(dimensions)
}

func coveredHostResourceDimensions(snapshot *TurnSnapshot) map[string]bool {
	covered := map[string]bool{}
	if snapshot == nil {
		return covered
	}
	for _, iteration := range snapshot.Iterations {
		toolCalls := make(map[string]ToolCall, len(iteration.ToolCalls))
		for _, call := range iteration.ToolCalls {
			toolCalls[call.ID] = call
		}
		for _, result := range iteration.ToolResults {
			if result.Error != "" || isToolBudgetResult(result) {
				continue
			}
			call := toolCalls[result.ToolCallID]
			if isNonBudgetToolResult(result, call.Name) {
				continue
			}
			text := strings.ToLower(strings.Join([]string{
				call.Name,
				string(call.Arguments),
				result.Summary,
				result.Content,
			}, "\n"))
			for _, dimension := range resourceDimensionsFromEvidenceText(text) {
				covered[dimension] = true
			}
		}
	}
	return covered
}

func resourceDimensionsFromEvidenceText(text string) []string {
	dimensions := make([]string, 0, 3)
	if containsAnyFold(text, []string{
		"nproc", "/proc/loadavg", "load average", "uptime", "mpstat", "top -", "cpu", "processor", "hw.ncpu", "负载",
	}) {
		dimensions = append(dimensions, "cpu")
	}
	if containsAnyFold(text, []string{
		"free -", "mem:", "swap:", "memory", "vm_stat", "内存",
	}) {
		dimensions = append(dimensions, "memory")
	}
	if containsAnyFold(text, []string{
		"df -", "filesystem", "mounted on", "disk", "volume", "磁盘", "文件系统",
	}) {
		dimensions = append(dimensions, "disk")
	}
	return uniqueStrings(dimensions)
}

func uniqueStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func synthesisOnlyPromptAsset(toolDispatches int) string {
	return fmt.Sprintf(
		"## Synthesis-only phase\n已收集 %d 个工具结果。停止继续调用工具，基于已有工具证据直接给用户回答；如果证据不足，给出 budget-limited conclusion，明确已用证据和仍缺证据。不要把证据不足包装成已验证根因，也不要假装调查完整。不要输出 tool_calls、DSML、invoke 或任何工具调用语法。",
		toolDispatches,
	)
}

func publicWebSynthesisOnlyPromptAsset(publicWebDispatches int) string {
	return fmt.Sprintf(
		"## Public-web synthesis-only phase\n已完成 %d 次公开网页/文档检索。停止继续调用 web_search，基于已收集的权威来源直接给用户最终回答；只引用真正支撑结论的链接，不要继续扩展同义查询。如果证据不足，给出受限结论并列出仍缺的只读证据。不要输出 tool_calls、DSML、invoke 或任何工具调用语法。",
		publicWebDispatches,
	)
}

func evidenceAwareFinalAnswerPromptAsset(snapshot *TurnSnapshot, limit int) string {
	summaries := collectedToolEvidenceSummaries(snapshot, limit)
	if len(summaries) == 0 {
		return ""
	}
	lines := []string{
		"## Evidence-aware final answer",
		"Use the collected tool evidence summaries below when preparing the final answer. Do not invent evidence; if evidence is incomplete, state the limitation briefly.",
		"When using web_search evidence, including operation=open evidence, cite the specific supporting source inline as `[参考: source title](URL)` immediately after the claim. Do not cite raw search queries as evidence.",
		"",
		"Collected evidence summaries:",
	}
	lines = append(lines, summaries...)
	lines = append(lines,
		"",
		"For AIOps/RCA or incident-analysis requests, structure the final answer with these exact section labels:",
		"根因：",
		"证据：",
		"影响面：",
		"下一步：",
		"",
		"Exception: when the user only asked for a read-only status/RCA check and the collected evidence shows no abnormality, Keep the final answer short: one conclusion plus key evidence. Do not expand 下一步, and do not suggest remediation, workflow execution, rollback, or operations manual generation.",
	)
	return strings.Join(lines, "\n")
}

func collectedToolEvidenceSummaries(snapshot *TurnSnapshot, limit int) []string {
	if snapshot == nil || limit <= 0 {
		return nil
	}
	out := make([]string, 0, limit)
	seen := map[string]bool{}
	for _, iteration := range snapshot.Iterations {
		for _, result := range iteration.ToolResults {
			if len(out) >= limit {
				return out
			}
			text := toolEvidenceSummaryForFinalPrompt(result)
			if text == "" {
				continue
			}
			text = truncateRunes(text, 220)
			key := strings.TrimSpace(result.ToolCallID + ":" + text)
			if seen[key] {
				continue
			}
			seen[key] = true
			label := strings.TrimSpace(result.ToolCallID)
			if label == "" {
				label = fmt.Sprintf("tool-%d", len(out)+1)
			}
			out = append(out, fmt.Sprintf("- %s: %s", label, text))
		}
	}
	return out
}

func toolEvidenceSummaryForFinalPrompt(result ToolResult) string {
	if refs := toolReferenceSummaryForFinalPrompt(result); refs != "" {
		return refs
	}
	if webSources := webSourcesSummaryForFinalPrompt(result.Content); webSources != "" {
		return webSources
	}
	text := strings.TrimSpace(result.Summary)
	if text == "" {
		text = firstNonEmptyLine(result.Content)
	}
	return text
}

func toolReferenceSummaryForFinalPrompt(result ToolResult) string {
	if len(result.References) == 0 {
		return ""
	}
	webParts := make([]string, 0, len(result.References))
	resourceParts := make([]string, 0, len(result.References))
	for _, ref := range result.References {
		uri := strings.TrimSpace(ref.URI)
		if uri == "" {
			continue
		}
		title := strings.TrimSpace(ref.Title)
		if title == "" {
			title = uri
		}
		part := fmt.Sprintf("[参考: %s](%s)", title, uri)
		if strings.HasPrefix(strings.ToLower(uri), "http://") || strings.HasPrefix(strings.ToLower(uri), "https://") {
			webParts = append(webParts, part)
		} else {
			resourceParts = append(resourceParts, part)
		}
		if len(webParts)+len(resourceParts) >= 4 {
			break
		}
	}
	if len(webParts) == 0 && len(resourceParts) == 0 {
		return ""
	}
	lines := make([]string, 0, 2)
	if len(webParts) > 0 {
		lines = append(lines, "网页来源: "+strings.Join(webParts, "; "))
	}
	if len(resourceParts) > 0 {
		lines = append(lines, "工具证据引用: "+strings.Join(resourceParts, "; "))
	}
	return strings.Join(lines, "\n")
}

type finalPromptWebSource struct {
	Title string
	URL   string
}

func webSourcesSummaryForFinalPrompt(content string) string {
	sources := extractWebSourcesForFinalPrompt(content)
	if len(sources) == 0 {
		return ""
	}
	parts := make([]string, 0, len(sources))
	for _, source := range sources {
		title := strings.TrimSpace(source.Title)
		if title == "" {
			title = source.URL
		}
		parts = append(parts, fmt.Sprintf("[参考: %s](%s)", title, source.URL))
		if len(parts) >= 4 {
			break
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "网页来源: " + strings.Join(parts, "; ")
}

func extractWebSourcesForFinalPrompt(content string) []finalPromptWebSource {
	lines := strings.Split(content, "\n")
	var out []finalPromptWebSource
	seen := map[string]bool{}
	pendingTitle := ""
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if title := numberedSearchTitle(line); title != "" {
			pendingTitle = title
			continue
		}
		if strings.HasPrefix(line, "- ") {
			title, sourceURL := splitTitleAndURL(strings.TrimSpace(strings.TrimPrefix(line, "- ")))
			if sourceURL != "" {
				out = appendUniqueFinalPromptWebSource(out, seen, title, sourceURL)
				continue
			}
		}
		if sourceURL := urlFromSearchLine(line); sourceURL != "" {
			out = appendUniqueFinalPromptWebSource(out, seen, pendingTitle, sourceURL)
			pendingTitle = ""
		}
	}
	return out
}

func appendUniqueFinalPromptWebSource(out []finalPromptWebSource, seen map[string]bool, title, sourceURL string) []finalPromptWebSource {
	sourceURL = strings.TrimSpace(sourceURL)
	if sourceURL == "" || seen[sourceURL] {
		return out
	}
	seen[sourceURL] = true
	out = append(out, finalPromptWebSource{Title: strings.TrimSpace(title), URL: sourceURL})
	return out
}

func numberedSearchTitle(line string) string {
	i := 0
	for i < len(line) && line[i] >= '0' && line[i] <= '9' {
		i++
	}
	if i == 0 {
		return ""
	}
	rest := strings.TrimSpace(line[i:])
	if !strings.HasPrefix(rest, ".") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(rest, "."))
}

func splitTitleAndURL(line string) (string, string) {
	sourceURL := extractFirstHTTPURL(line)
	if sourceURL == "" {
		return "", ""
	}
	title := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(strings.Split(line, sourceURL)[0]), ":"))
	return title, sourceURL
}

func urlFromSearchLine(line string) string {
	if idx := strings.Index(strings.ToLower(line), "url:"); idx >= 0 {
		return extractFirstHTTPURL(strings.TrimSpace(line[idx+len("url:"):]))
	}
	return extractFirstHTTPURL(line)
}

func extractFirstHTTPURL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	start := -1
	for _, prefix := range []string{"https://", "http://"} {
		if idx := strings.Index(strings.ToLower(value), prefix); idx >= 0 && (start == -1 || idx < start) {
			start = idx
		}
	}
	if start < 0 {
		return ""
	}
	end := len(value)
	for i, r := range value[start:] {
		if i == 0 {
			continue
		}
		if r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\'' || r == '<' || r == '>' || r == ')' || r == ']' || r == '}' {
			end = start + i
			break
		}
	}
	return strings.TrimRight(value[start:end], ".,;:")
}

func toolIntentPrelude(userInput string, assistantMsg Message) string {
	if len(assistantMsg.ToolCalls) == 0 {
		return ""
	}
	if content := firstNonEmptyLine(assistantMsg.Content); content != "" {
		return truncateRunes(content, 160)
	}
	firstTool := assistantMsg.ToolCalls[0]
	toolName := strings.TrimSpace(firstTool.Name)
	switch toolName {
	case "web_search", "search_web":
		query := toolCallStringField(firstTool, "query", "q", "keywords", "search")
		if query == "" {
			query = firstNonEmptyLine(userInput)
		}
		if query != "" {
			return fmt.Sprintf("我会先搜索网页核对「%s」，必要时再读取来源或用只读命令校验，最后整理简洁回答。", truncateRunes(query, 80))
		}
		return "我会先搜索网页核对关键信息，必要时再读取来源或用只读命令校验，最后整理简洁回答。"
	case "open_page", "find_in_page":
		target := toolCallStringField(firstTool, "url", "pattern", "query")
		if target != "" {
			return fmt.Sprintf("我会先浏览或检索「%s」里的关键信息，再基于证据整理回答。", truncateRunes(target, 80))
		}
		return "我会先浏览或检索网页里的关键信息，再基于证据整理回答。"
	case "shell_command", "exec_command", "exec_readonly", "execute_command", "execute_readonly_query", "code_mode":
		command := toolCallStringField(firstTool, "command", "cmd", "query")
		if command != "" {
			return fmt.Sprintf("我会先执行只读命令「%s」获取证据，再根据输出整理回答。", truncateRunes(command, 80))
		}
		return "我会先执行只读命令获取证据，再根据输出整理回答。"
	case "read_file", "list_files", "list_dir", "search_files", "grep":
		target := toolCallStringField(firstTool, "path", "file", "query", "pattern")
		if target != "" {
			return fmt.Sprintf("我会先检查「%s」相关内容，再整理必要证据和回答。", truncateRunes(target, 80))
		}
		return "我会先检查相关文件和上下文，再整理必要证据和回答。"
	default:
		if toolName != "" {
			return fmt.Sprintf("我会先用 %s 核对关键信息，再整理回答。", toolName)
		}
		return "我会先用可用工具核对关键信息，再整理回答。"
	}
}

func toolCallStringField(call ToolCall, names ...string) string {
	if len(call.Arguments) == 0 {
		return ""
	}
	var payload map[string]any
	if err := json.Unmarshal(call.Arguments, &payload); err != nil {
		return ""
	}
	for _, name := range names {
		value, ok := payload[name]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case string:
			if text := strings.TrimSpace(typed); text != "" {
				return text
			}
		case []any:
			parts := make([]string, 0, len(typed))
			for _, item := range typed {
				if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
					parts = append(parts, text)
				}
			}
			if len(parts) > 0 {
				return strings.Join(parts, " ")
			}
		default:
			text := strings.TrimSpace(fmt.Sprint(typed))
			if text != "" && text != "<nil>" {
				return text
			}
		}
	}
	return ""
}

func approvalCommandForToolCall(call ToolCall) string {
	command := toolCallStringField(call, "command", "cmd", "script")
	args := toolCallStringField(call, "args", "argv")
	if command != "" && args != "" {
		return strings.TrimSpace(command + " " + args)
	}
	if command != "" {
		return command
	}
	if detail := toolCallStringField(call, "query", "q", "path", "file", "url", "pattern"); detail != "" {
		return detail
	}
	raw := strings.TrimSpace(string(call.Arguments))
	if raw == "" {
		return strings.TrimSpace(call.Name)
	}
	return truncateRunes(raw, 180)
}

func firstNonEmptyLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if text := strings.TrimSpace(line); text != "" {
			return text
		}
	}
	return ""
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 {
		return ""
	}
	runes := []rune(strings.TrimSpace(value))
	if len(runes) <= limit {
		return string(runes)
	}
	return string(runes[:limit]) + "..."
}

func countActualToolDispatches(snapshot *TurnSnapshot) int {
	if snapshot == nil {
		return 0
	}
	count := 0
	for _, iteration := range snapshot.Iterations {
		toolNamesByID := make(map[string]string, len(iteration.ToolCalls))
		for _, call := range iteration.ToolCalls {
			toolNamesByID[call.ID] = call.Name
		}
		for _, result := range iteration.ToolResults {
			if isToolBudgetResult(result) || isNonBudgetToolResult(result, toolNamesByID[result.ToolCallID]) {
				continue
			}
			count++
		}
	}
	return count
}

func countToolCallsTowardBudget(calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		if countsTowardToolBudget(call) {
			count++
		}
	}
	return count
}

func countPublicWebDispatches(snapshot *TurnSnapshot, tools []promptcompiler.Tool) int {
	if snapshot == nil {
		return 0
	}
	count := 0
	for _, iteration := range snapshot.Iterations {
		toolCallsByID := make(map[string]ToolCall, len(iteration.ToolCalls))
		for _, call := range iteration.ToolCalls {
			toolCallsByID[call.ID] = call
		}
		for _, result := range iteration.ToolResults {
			call := toolCallsByID[result.ToolCallID]
			if !isPublicWebToolCall(tools, call) || isToolBudgetResult(result) || isNonBudgetToolResult(result, call.Name) {
				continue
			}
			count++
		}
	}
	return count
}

func countPublicWebToolCalls(tools []promptcompiler.Tool, calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		if isPublicWebToolCall(tools, call) {
			count++
		}
	}
	return count
}

func countPublicWebQueries(snapshot *TurnSnapshot, tools []promptcompiler.Tool) int {
	if snapshot == nil {
		return 0
	}
	count := 0
	for _, iteration := range snapshot.Iterations {
		toolCallsByID := make(map[string]ToolCall, len(iteration.ToolCalls))
		for _, call := range iteration.ToolCalls {
			toolCallsByID[call.ID] = call
		}
		for _, result := range iteration.ToolResults {
			call := toolCallsByID[result.ToolCallID]
			if !isPublicWebToolCall(tools, call) || isToolBudgetResult(result) || isNonBudgetToolResult(result, call.Name) {
				continue
			}
			count += publicWebQueryCountForToolCall(call)
		}
	}
	return count
}

func countPublicWebQueriesForToolCalls(tools []promptcompiler.Tool, calls []ToolCall) int {
	count := 0
	for _, call := range calls {
		if isPublicWebToolCall(tools, call) {
			count += publicWebQueryCountForToolCall(call)
		}
	}
	return count
}

func publicWebBudgetReached(tools []promptcompiler.Tool, call ToolCall, currentCalls, currentQueries int, budget PublicWebBudget) bool {
	if !isPublicWebToolCall(tools, call) {
		return false
	}
	budget = normalizePublicWebBudget(budget)
	if currentCalls >= budget.MaxSearchCalls {
		return true
	}
	return currentQueries+publicWebQueryCountForToolCall(call) > budget.MaxQueries
}

func isPublicWebToolCall(tools []promptcompiler.Tool, call ToolCall) bool {
	name := strings.TrimSpace(call.Name)
	if name == "" {
		return false
	}
	if toolDef := toolForToolCall(tools, call); toolDef != nil {
		return strings.TrimSpace(toolDef.Metadata().Pack) == "public_web"
	}
	switch tooling.ProviderSafeToolName(name) {
	case "web_search", "browse_url":
		return true
	default:
		return false
	}
}

func publicWebToolNames(tools []promptcompiler.Tool) []string {
	names := make([]string, 0, 2)
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		meta := toolDef.Metadata()
		if strings.TrimSpace(meta.Pack) == "public_web" {
			names = append(names, meta.Name)
		}
	}
	if len(names) == 0 {
		names = append(names, "web_search")
	}
	return uniqueStrings(names)
}

func promptInputWebSearchTrace(snapshot *TurnSnapshot, tools []promptcompiler.Tool) *promptinput.WebSearchTrace {
	if snapshot == nil {
		return nil
	}
	trace := &promptinput.WebSearchTrace{}
	for _, iteration := range snapshot.Iterations {
		callsByID := make(map[string]ToolCall, len(iteration.ToolCalls))
		for _, call := range iteration.ToolCalls {
			callsByID[call.ID] = call
			if isPublicWebToolCall(tools, call) {
				trace.Attempted = true
			}
		}
		for _, result := range iteration.ToolResults {
			call := callsByID[result.ToolCallID]
			if !isPublicWebToolCall(tools, call) {
				continue
			}
			trace.Attempted = true
			if strings.TrimSpace(result.Error) != "" && strings.TrimSpace(trace.FailureReason) == "" {
				trace.FailureReason = truncateWebSearchSeed(result.Error)
			}
			adapter, sourceCount := webSearchTraceAdapterAndSourceCount(result.Content)
			if trace.Adapter == "" {
				trace.Adapter = adapter
			}
			if sourceCount > trace.SourceCount {
				trace.SourceCount = sourceCount
			}
		}
	}
	if !trace.Attempted && trace.RetryCount == 0 && trace.Adapter == "" && trace.SourceCount == 0 && trace.FailureReason == "" {
		return nil
	}
	return trace
}

func promptInputFinalTrace(snapshot *TurnSnapshot) *promptinput.FinalTrace {
	if snapshot == nil || !metadataBool(snapshot.Metadata["aiops.webSearch.finalLimited"]) {
		return nil
	}
	return &promptinput.FinalTrace{PublicWebLimitation: true}
}

func webSearchTraceAdapterAndSourceCount(content string) (string, int) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", 0
	}
	var payload struct {
		Source  string `json:"source"`
		Results []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"results"`
		Meta map[string]any `json:"meta"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return "", 0
	}
	adapter := strings.TrimSpace(payload.Source)
	if adapter == "" && payload.Meta != nil {
		if source, ok := payload.Meta["source"].(string); ok {
			adapter = strings.TrimSpace(source)
		}
	}
	sourceCount := 0
	for _, result := range payload.Results {
		if strings.TrimSpace(result.Title) != "" || strings.TrimSpace(result.URL) != "" || strings.TrimSpace(result.Snippet) != "" {
			sourceCount++
		}
	}
	return adapter, sourceCount
}

func limitPublicWebToolCall(tools []promptcompiler.Tool, call ToolCall, budget PublicWebBudget) ToolCall {
	if !isPublicWebToolCall(tools, call) || len(call.Arguments) == 0 {
		return call
	}
	budget = normalizePublicWebBudget(budget)
	var payload map[string]any
	if err := json.Unmarshal(call.Arguments, &payload); err != nil {
		return call
	}
	changed := false
	resultKeys := []string{"max_results", "maxResults", "num_results", "numResults", "limit", "count"}
	foundResultLimit := false
	for _, key := range resultKeys {
		value, ok := payload[key]
		if !ok {
			continue
		}
		foundResultLimit = true
		if numeric, ok := numericJSONValue(value); ok && numeric > budget.MaxResults {
			payload[key] = budget.MaxResults
			changed = true
		}
	}
	if !foundResultLimit {
		payload["max_results"] = budget.MaxResults
		changed = true
	}
	for _, key := range []string{"queries", "search_query", "searchQuery", "q", "keywords"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		trimmed, didTrim := trimPublicWebQueryList(value, budget.MaxQueriesPerCall)
		if didTrim {
			payload[key] = trimmed
			changed = true
		}
	}
	if !changed {
		return call
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return call
	}
	call.Arguments = data
	return call
}

func trimPublicWebQueryList(value any, limit int) (any, bool) {
	if limit <= 0 {
		return value, false
	}
	switch typed := value.(type) {
	case []any:
		if len(typed) <= limit {
			return value, false
		}
		return typed[:limit], true
	case []string:
		if len(typed) <= limit {
			return value, false
		}
		return typed[:limit], true
	default:
		return value, false
	}
}

func publicWebQueryCountForToolCall(call ToolCall) int {
	if len(call.Arguments) == 0 {
		return 1
	}
	var payload map[string]any
	if err := json.Unmarshal(call.Arguments, &payload); err != nil {
		return 1
	}
	for _, key := range []string{"queries", "query", "search_query", "searchQuery", "q", "keywords"} {
		value, ok := payload[key]
		if !ok {
			continue
		}
		switch typed := value.(type) {
		case []any:
			count := 0
			for _, item := range typed {
				if strings.TrimSpace(fmt.Sprint(item)) != "" {
					count++
				}
			}
			if count > 0 {
				return count
			}
		case []string:
			count := 0
			for _, item := range typed {
				if strings.TrimSpace(item) != "" {
					count++
				}
			}
			if count > 0 {
				return count
			}
		default:
			if strings.TrimSpace(fmt.Sprint(typed)) != "" {
				return 1
			}
		}
	}
	return 1
}

func numericJSONValue(value any) (int, bool) {
	switch typed := value.(type) {
	case int:
		return typed, true
	case int64:
		return int(typed), true
	case float64:
		return int(typed), true
	case json.Number:
		n, err := typed.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	default:
		return 0, false
	}
}

func normalizePublicWebBudget(budget PublicWebBudget) PublicWebBudget {
	if budget.MaxCallsPerTurn > 0 && budget.MaxSearchCalls <= 0 {
		budget.MaxSearchCalls = budget.MaxCallsPerTurn
	}
	if budget.MaxSearchCalls <= 0 {
		budget.MaxSearchCalls = 3
	}
	if budget.MaxCallsPerTurn <= 0 {
		budget.MaxCallsPerTurn = budget.MaxSearchCalls
	}
	if budget.MaxQueries <= 0 {
		budget.MaxQueries = 6
	}
	if budget.MaxResults <= 0 {
		budget.MaxResults = 8
	}
	if budget.MaxQueriesPerCall <= 0 {
		budget.MaxQueriesPerCall = 2
	}
	if budget.MaxResultsPerDomain <= 0 {
		budget.MaxResultsPerDomain = 2
	}
	return budget
}

func countsTowardToolBudget(call ToolCall) bool {
	return !isUpdatePlanToolName(call.Name)
}

func isNonBudgetToolResult(result ToolResult, toolName string) bool {
	if isUpdatePlanToolName(toolName) {
		return true
	}
	return result.Display != nil && result.Display.Type == "plan"
}

func isToolBudgetResult(result ToolResult) bool {
	return result.Display != nil && result.Display.Type == "tool_budget"
}

func canDispatchToolCallInParallel(tools []promptcompiler.Tool, tc ToolCall) bool {
	eligible, _, _ := parallelDispatchEligibility(tools, tc)
	return eligible
}

func toolForToolCall(tools []promptcompiler.Tool, tc ToolCall) promptcompiler.Tool {
	toolName := strings.TrimSpace(tc.Name)
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		meta := toolDef.Metadata()
		if toolCallNameMatchesCandidate(toolName, meta.Name) {
			return toolDef
		}
		for _, alias := range meta.Aliases {
			if toolCallNameMatchesCandidate(toolName, alias) {
				return toolDef
			}
		}
	}
	return nil
}

func addToolLookupName(byName map[string]tooling.Tool, name string, toolDef tooling.Tool) {
	name = strings.TrimSpace(name)
	if name == "" || toolDef == nil {
		return
	}
	byName[name] = toolDef
	providerName := tooling.ProviderSafeToolName(name)
	if providerName != "" {
		byName[providerName] = toolDef
	}
}

func toolCallNameMatchesCandidate(toolName, candidate string) bool {
	toolName = strings.TrimSpace(toolName)
	candidate = strings.TrimSpace(candidate)
	if toolName == "" || candidate == "" {
		return false
	}
	return toolName == candidate || toolName == tooling.ProviderSafeToolName(candidate)
}

func toolMetadataForToolCall(tools []promptcompiler.Tool, tc ToolCall) tooling.ToolMetadata {
	if toolDef := toolForToolCall(tools, tc); toolDef != nil {
		return toolDef.Metadata()
	}
	toolName := strings.TrimSpace(tc.Name)
	return tooling.ToolMetadata{Name: toolName}
}

func toolBudgetReachedResultForModel(tc ToolCall, executed int) tooling.ToolResult {
	toolName := strings.TrimSpace(tc.Name)
	if toolName == "" {
		toolName = "tool"
	}
	return tooling.ToolResult{
		ToolCallID: tc.ID,
		Content: fmt.Sprintf(
			"Tool budget reached after %d executed tool calls. Do not call more tools in this turn. Answer now using the evidence already collected; if the evidence is incomplete, state the limitation briefly.",
			executed,
		),
		Display: &tooling.ToolDisplayPayload{
			Type:  "tool_budget",
			Title: toolName,
		},
	}
}

func publicWebBudgetReachedResultForModel(tc ToolCall, executedCalls, executedQueries int, budget PublicWebBudget) tooling.ToolResult {
	toolName := strings.TrimSpace(tc.Name)
	if toolName == "" {
		toolName = "public_web"
	}
	budget = normalizePublicWebBudget(budget)
	return tooling.ToolResult{
		ToolCallID: tc.ID,
		Content: fmt.Sprintf(
			"Public web retrieval budget reached after %d public web tool calls and %d queries. Turn budget is max_search_calls=%d, max_queries=%d, max_results=%d. Do not call more public web tools in this turn. Answer now using the collected source evidence; if evidence is incomplete, state the limitation briefly and list the most useful remaining query or source.",
			executedCalls,
			executedQueries,
			budget.MaxSearchCalls,
			budget.MaxQueries,
			budget.MaxResults,
		),
		Display: &tooling.ToolDisplayPayload{
			Type:  "tool_budget",
			Title: toolName,
		},
	}
}

func shouldFeedToolFailureBackToModel(result DispatchResult) bool {
	if result.Blocked || strings.TrimSpace(result.Error) == "" {
		return false
	}
	if result.Metadata.EffectiveGovernance(defaultMaxInlineResultBytes).FailurePolicy == tooling.ToolFailurePolicyFailTurn {
		return false
	}
	switch result.Outcome {
	case "tool_failed":
		return true
	case "tool_denied":
		return result.Source == "tool"
	default:
		return false
	}
}

func failedToolResultForModel(tc ToolCall, result DispatchResult) tooling.ToolResult {
	toolName := strings.TrimSpace(tc.Name)
	if toolName == "" {
		toolName = "tool"
	}
	errText := strings.TrimSpace(result.Error)
	if errText == "" {
		errText = "tool execution failed"
	}
	decision := toolfailure.NewClassifier().Classify(toolfailure.ClassificationInput{
		Source:  result.Source,
		Outcome: result.Outcome,
		Error:   errText,
	})
	allowedNextActions := []string{}
	if decision.RequiresUser || decision.Kind == toolfailure.KindToolNotFound {
		allowedNextActions = append(allowedNextActions, string(toolfailure.ActionAskUser))
	}
	bodyMap := map[string]any{
		"type":               "tool_error",
		"toolCallId":         tc.ID,
		"toolName":           toolName,
		"failureKind":        string(decision.Kind),
		"retryable":          false,
		"userActionRequired": decision.RequiresUser,
		"message":            fmt.Sprintf("%s failed: %s", toolName, errText),
		"allowedNextActions": allowedNextActions,
	}
	if decision.Kind == toolfailure.KindSideEffectUnknown {
		bodyMap["postCheckRequired"] = true
		if refs := normalizedPostCheckRefs(result.Metadata); len(refs) > 0 {
			bodyMap["postCheckRefs"] = refs
		}
	}
	body, marshalErr := json.Marshal(bodyMap)
	content := string(body)
	if marshalErr != nil {
		content = fmt.Sprintf(`{"type":"tool_error","toolCallId":%q,"toolName":%q,"failureKind":"%s","retryable":false,"userActionRequired":false,"message":%q,"allowedNextActions":[]}`, tc.ID, toolName, decision.Kind, fmt.Sprintf("%s failed: %s", toolName, errText))
	}
	return tooling.ToolResult{
		ToolCallID: tc.ID,
		Content:    content,
		Error:      errText,
		Display: &tooling.ToolDisplayPayload{
			Type:  "tool_error",
			Title: toolName,
		},
	}
}

func repeatedToolFailureSignatureDecision(snapshot *TurnSnapshot, tools []promptcompiler.Tool, current ToolCall, currentResult tooling.ToolResult) FailureSignatureDecision {
	toolName := canonicalRuntimeToolName(tools, current)
	signature := BuildFailureSignature(toolName, current.Arguments, ToolResult{
		ToolCallID: currentResult.ToolCallID,
		Content:    currentResult.Content,
		Error:      currentResult.Error,
	})
	seenCount := 1
	if snapshot != nil {
		for _, iteration := range snapshot.Iterations {
			callsByID := make(map[string]ToolCall, len(iteration.ToolCalls))
			for _, call := range iteration.ToolCalls {
				callsByID[strings.TrimSpace(call.ID)] = call
			}
			for _, result := range iteration.ToolResults {
				if strings.TrimSpace(result.Error) == "" {
					continue
				}
				call, ok := callsByID[strings.TrimSpace(result.ToolCallID)]
				if !ok {
					continue
				}
				if BuildFailureSignature(canonicalRuntimeToolName(tools, call), call.Arguments, result) == signature {
					seenCount++
				}
			}
		}
	}
	return EvaluateFailureSignatureDecision(signature, seenCount)
}

func withFailureSignatureDecision(result tooling.ToolResult, decision FailureSignatureDecision) tooling.ToolResult {
	if decision.Action != "switch_path" {
		return result
	}
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		payload["message"] = strings.TrimSpace(result.Content)
	}
	payload["failureSignature"] = decision.Signature
	payload["failureSignatureSeenCount"] = decision.SeenCount
	payload["failureSignatureAction"] = decision.Action
	payload["modelGuidance"] = decision.SwitchPathReason
	if content, err := json.Marshal(payload); err == nil {
		result.Content = string(content)
	}
	return result
}

func enrichCompileContext(
	compileCtx promptcompiler.CompileContext,
	sessionType SessionType,
	hostID string,
	metadata map[string]string,
	now time.Time,
) promptcompiler.CompileContext {
	if compileCtx.AgentKind == "" {
		switch sessionType {
		case SessionTypeWorkspace:
			compileCtx.AgentKind = promptcompiler.AgentKindPlanner
		default:
			compileCtx.AgentKind = promptcompiler.AgentKindWorker
		}
	}
	hostID = strings.TrimSpace(hostID)
	if hostID != "" && strings.TrimSpace(compileCtx.HostContext) == "" {
		compileCtx.HostContext = hostID
	}
	if hostOpsManagerRequested(metadata) {
		compileCtx.HostOpsManager = true
		compileCtx.HostOpsPlanRequired = hostOpsPlanRequired(metadata)
	}
	if sessionBinding := sessionBindingPromptSection(metadata); sessionBinding != "" {
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "Session Binding",
			Content: sessionBinding,
		})
	}
	if selectedHostInventory := selectedHostInventoryPromptSection(metadata); selectedHostInventory != "" {
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "Selected Host Inventory",
			Content: selectedHostInventory,
		})
	}
	if opsManualOptOut := opsManualOptOutPromptSection(metadata); opsManualOptOut != "" {
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "Ops Manual Opt-Out",
			Content: opsManualOptOut,
		})
	}
	if opsManualReference := opsManualReferencePromptSection(metadata); opsManualReference != "" {
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "Ops Manual Reference",
			Content: opsManualReference,
		})
	}
	if webSearchPolicy := webSearchPolicyPromptSection(metadata); webSearchPolicy != "" {
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "Web Search Policy",
			Content: webSearchPolicy,
		})
	}
	if sessionType != SessionTypeHost || hostID != "server-local" {
		return compileCtx
	}
	compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
		Title: "Current Time",
		Content: fmt.Sprintf(
			"Current ai-server local time: %s. When the selected host is server-local, treat this as the host's current time.",
			now.Format("2006-01-02 15:04:05 MST -0700"),
		),
	})
	compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
		Title:   "Server-local Port Safety",
		Content: "The AIOps UI/API for this session may be served on 127.0.0.1:8080. For service/container changes on server-local, check host ports with lsof before binding and do not bind new workloads to 127.0.0.1:8080 or host port 8080. Choose a free alternate port such as 18080 when exposing test services.",
	})
	return compileCtx
}

func hostOpsManagerRequested(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	if strings.TrimSpace(metadata["aiops.hostops.mentions"]) != "" {
		return true
	}
	return hostOpsPlanRequired(metadata)
}

func hostOpsPlanRequired(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	for _, key := range []string{
		"aiops.hostops.planRequired",
		"aiops.hostops.clientDetectedMultiHost",
		"aiops.hostops.serverDetectedMultiHost",
	} {
		if metadataBool(metadata[key]) {
			return true
		}
	}
	return false
}

func metadataBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "true", "1", "yes", "y", "on":
		return true
	default:
		return false
	}
}

func metadataListContains(raw, want string) bool {
	want = strings.ToLower(strings.TrimSpace(want))
	if want == "" {
		return false
	}
	values := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == want {
			return true
		}
	}
	return false
}

func updateRuntimeEnvironmentContext(session *SessionState, req TurnRequest, now time.Time) {
	if session == nil || strings.TrimSpace(req.Input) == "" {
		return
	}
	session.EnvironmentContext = envcontext.ApplyUserTurn(session.EnvironmentContext, envcontext.UserTurn{
		SessionID: session.ID,
		HostID:    firstNonEmpty(strings.TrimSpace(req.HostID), strings.TrimSpace(session.HostID)),
		Input:     req.Input,
		Metadata:  req.Metadata,
		Now:       now,
	})
}

func appendRuntimeEnvironmentContextSection(
	compileCtx promptcompiler.CompileContext,
	session *SessionState,
) promptcompiler.CompileContext {
	if session == nil || runtimeEnvironmentContextEmpty(session.EnvironmentContext) {
		return compileCtx
	}
	section, ok := envcontext.BuildRuntimeEnvironmentSection(session.EnvironmentContext)
	if !ok {
		return compileCtx
	}
	compileCtx.ExtraSections = append(compileCtx.ExtraSections, section)
	return compileCtx
}

func runtimeEnvironmentContextEmpty(state envcontext.State) bool {
	return state.LastIntent == "" && state.CurrentFocus == nil && state.ManualContext == nil
}

func sessionBindingPromptSection(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	lines := make([]string, 0, 6)
	add := func(label string, keys ...string) {
		for _, key := range keys {
			value := strings.TrimSpace(metadata[key])
			if value == "" {
				continue
			}
			lines = append(lines, fmt.Sprintf("%s: %s", label, value))
			return
		}
	}
	add("Target kind", "aiops.target.kind")
	add("Target host", "aiops.target.hostId")
	add("Target label", "aiops.target.label")
	add("Environment", "aiops.environment", "aiops.target.environment")
	lines = addDynamicMetadataBinding(lines, metadata, "Project", ".project")
	if len(lines) == 0 {
		return ""
	}
	lines = append(lines, "Use these session bindings when selecting read-only evidence tools. Pass bound provider, project, environment, and target identifiers when a selected tool supports them; if a binding is absent, rely on the tool default and report unavailability from the tool result instead of inventing environment facts.")
	return strings.Join(lines, "\n")
}

func selectedHostInventoryPromptSection(metadata map[string]string) string {
	if len(metadata) == 0 || !metadataBool(metadata["aiops.host.metadataAvailable"]) {
		return ""
	}
	lines := make([]string, 0, 10)
	add := func(label, key string) {
		value := strings.TrimSpace(metadata[key])
		if value == "" {
			return
		}
		lines = append(lines, fmt.Sprintf("%s: %s", label, value))
	}
	add("Host ID", "aiops.host.id")
	add("Display name", "aiops.host.label")
	add("Address", "aiops.host.address")
	add("SSH user", "aiops.host.sshUser")
	add("SSH port", "aiops.host.sshPort")
	add("OS", "aiops.host.os")
	add("Arch", "aiops.host.arch")
	add("Transport", "aiops.host.transport")
	add("Status", "aiops.host.status")
	add("Agent status", "aiops.host.agentStatus")
	add("SSH status", "aiops.host.sshStatus")
	add("Runtime reachability", "aiops.host.runtimeReachability")
	if len(lines) == 0 {
		return ""
	}
	lines = append([]string{
		"The current host session is bound to this inventory record. Use these values when answering host identity, address, SSH user, and connection questions. Do not guess missing fields. Treat Status as the host-agent heartbeat status; use SSH status and runtime reachability to decide whether read-only SSH evidence may still be available.",
	}, lines...)
	return strings.Join(lines, "\n")
}

func addDynamicMetadataBinding(lines []string, metadata map[string]string, label, suffix string) []string {
	suffix = strings.TrimSpace(suffix)
	if suffix == "" {
		return lines
	}
	keys := make([]string, 0, len(metadata))
	for key := range metadata {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		value := metadata[key]
		if !strings.HasSuffix(strings.ToLower(strings.TrimSpace(key)), strings.ToLower(suffix)) {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		return append(lines, fmt.Sprintf("%s: %s", label, value))
	}
	return lines
}

func opsManualOptOutPromptSection(metadata map[string]string) string {
	if !opsManualsOptedOut(metadata) {
		return ""
	}
	lines := []string{
		"User explicitly skipped operations manuals for this continuation.",
		"Do not call search_ops_manuals, resolve_ops_manual_params, or run_ops_manual_preflight in this turn.",
		"Continue ordinary safe read-only investigation with non-manual tools and concise status updates.",
	}
	if manualTitle := strings.TrimSpace(metadata["opsManualManualTitle"]); manualTitle != "" {
		lines = append(lines, fmt.Sprintf("Skipped manual title: %s", manualTitle))
	}
	if manualID := firstMetadataValue(metadata, "opsManualManualId", "manualId"); manualID != "" {
		lines = append(lines, fmt.Sprintf("Skipped manual id: %s", manualID))
	}
	if workflowID := firstMetadataValue(metadata, "opsManualWorkflowId", "workflowId"); workflowID != "" {
		lines = append(lines, fmt.Sprintf("Skipped workflow id: %s", workflowID))
	}
	return strings.Join(lines, "\n")
}

func opsManualReferencePromptSection(metadata map[string]string) string {
	if len(metadata) == 0 || !strings.EqualFold(strings.TrimSpace(metadata["opsManualAction"]), "reference_ops_manual") {
		return ""
	}
	lines := []string{
		"User chose to reference the operations manual without entering Workflow preflight.",
		"Use the manual as read-only guidance for manual-guided chat; do not call run_ops_manual_preflight or claim Workflow execution is available from this continuation.",
	}
	if manualTitle := strings.TrimSpace(metadata["opsManualManualTitle"]); manualTitle != "" {
		lines = append(lines, fmt.Sprintf("Referenced manual title: %s", manualTitle))
	}
	if manualID := firstMetadataValue(metadata, "opsManualManualId", "manualId", "manual_id"); manualID != "" {
		lines = append(lines, fmt.Sprintf("Referenced manual id: %s", manualID))
	}
	if workflowID := firstMetadataValue(metadata, "opsManualWorkflowId", "workflowId", "workflow_id"); workflowID != "" {
		lines = append(lines, fmt.Sprintf("Workflow id for reference only: %s", workflowID))
	}
	return strings.Join(lines, "\n")
}

func webSearchPolicyPromptSection(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	policy := webSearchPolicyLevelFromMetadata(metadata)
	if policy == "" || policy == WebSearchDisabled {
		return ""
	}
	lines := []string{strings.TrimSpace(webSearchPolicyPromptAsset), ""}
	lines = append(lines, fmt.Sprintf("WebSearchPolicy: %s", policy))
	lines = append(lines, "Tool availability: web_search is enabled for this turn.")
	if reason := strings.TrimSpace(metadata["aiops.webSearch.reason"]); reason != "" {
		lines = append(lines, fmt.Sprintf("Reason: %s", reason))
	}
	if reasonCodes := strings.TrimSpace(metadata["aiops.webSearch.reasonCodes"]); reasonCodes != "" {
		lines = append(lines, fmt.Sprintf("Reason codes: %s", reasonCodes))
	}
	lines = append(lines, "Use web_search when public evidence would materially improve correctness; cite source URLs when used.")
	if querySeeds := webSearchQuerySeedsPromptLines(metadata["aiops.webSearch.querySeeds"]); len(querySeeds) > 0 {
		lines = append(lines, "Query seeds:")
		lines = append(lines, querySeeds...)
	}
	if strings.EqualFold(strings.TrimSpace(metadata["aiops.weblearn.sourcePolicy"]), "official_first") {
		lines = append(lines, "Source policy: official_first / 官方来源优先。")
	}
	if evidence := webLearnEvidencePromptSection(metadata["aiops.weblearn.evidence"]); evidence != "" {
		lines = append(lines, "", "Web Search Evidence", evidence)
	}
	return strings.Join(lines, "\n")
}

func webSearchPolicyLevelFromMetadata(metadata map[string]string) WebSearchPolicyLevel {
	policy := WebSearchPolicyLevel(strings.ToLower(strings.TrimSpace(metadata["aiops.webSearch.policy"])))
	switch policy {
	case WebSearchDisabled, WebSearchEnabled:
		return policy
	case "must_search":
		return WebSearchEnabled
	}
	if metadataBool(metadata["aiops.weblearn.enabled"]) {
		return WebSearchEnabled
	}
	return ""
}

func webSearchQuerySeedsPromptLines(raw string) []string {
	lines := make([]string, 0, 3)
	for _, seed := range strings.FieldsFunc(raw, func(r rune) bool {
		return r == '\n' || r == '\r'
	}) {
		seed = sanitizeWebSearchQuerySeed(seed)
		if seed == "" {
			continue
		}
		lines = append(lines, "- "+truncateWebSearchSeed(seed))
		if len(lines) >= 3 {
			break
		}
	}
	return lines
}

type runtimeWebLearnEvidence struct {
	Kind            string `json:"kind"`
	Query           string `json:"query"`
	SourceURL       string `json:"sourceUrl"`
	SourceTitle     string `json:"sourceTitle"`
	SourceKind      string `json:"sourceKind"`
	Product         string `json:"product"`
	Version         string `json:"version"`
	RelevantExcerpt string `json:"relevantExcerpt"`
	Applicability   string `json:"applicability"`
	Confidence      string `json:"confidence"`
}

func webLearnEvidencePromptSection(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var items []runtimeWebLearnEvidence
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return ""
	}
	lines := make([]string, 0, len(items)*4)
	for _, item := range items {
		item = normalizeRuntimeWebLearnEvidence(item)
		if !runtimeWebLearnEvidenceInjectable(item) {
			continue
		}
		title := firstNonEmpty(item.SourceTitle, item.SourceURL)
		head := "- " + title
		var attrs []string
		if item.Product != "" {
			attrs = append(attrs, item.Product)
		}
		if item.Version != "" {
			attrs = append(attrs, "version "+item.Version)
		}
		if item.SourceKind != "" {
			attrs = append(attrs, item.SourceKind)
		}
		if item.Confidence != "" {
			attrs = append(attrs, item.Confidence+" confidence")
		}
		if len(attrs) > 0 {
			head += " (" + strings.Join(attrs, ", ") + ")"
		}
		lines = append(lines, head)
		if item.SourceURL != "" {
			lines = append(lines, "  URL: "+item.SourceURL)
		}
		if item.Applicability != "" {
			lines = append(lines, "  Applicability: "+item.Applicability)
		}
		if item.RelevantExcerpt != "" {
			lines = append(lines, "  Summary: "+truncateForBudget(item.RelevantExcerpt, 420))
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return strings.Join(lines, "\n")
}

func normalizeRuntimeWebLearnEvidence(item runtimeWebLearnEvidence) runtimeWebLearnEvidence {
	item.Kind = strings.ToLower(strings.TrimSpace(item.Kind))
	item.Query = strings.TrimSpace(item.Query)
	item.SourceURL = strings.TrimSpace(item.SourceURL)
	item.SourceTitle = strings.TrimSpace(item.SourceTitle)
	item.SourceKind = strings.ToLower(strings.TrimSpace(item.SourceKind))
	item.Product = strings.TrimSpace(item.Product)
	item.Version = strings.TrimSpace(item.Version)
	item.RelevantExcerpt = strings.TrimSpace(item.RelevantExcerpt)
	item.Applicability = strings.TrimSpace(item.Applicability)
	item.Confidence = strings.ToLower(strings.TrimSpace(item.Confidence))
	return item
}

func runtimeWebLearnEvidenceInjectable(item runtimeWebLearnEvidence) bool {
	if item.Kind != "external_knowledge" {
		return false
	}
	switch item.Confidence {
	case "high", "medium":
	default:
		return false
	}
	switch item.SourceKind {
	case "official_docs", "vendor_docs", "project_docs", "source_repo", "release_notes", "manual", "man_page":
	default:
		return false
	}
	return item.SourceURL != "" && (item.RelevantExcerpt != "" || item.Applicability != "")
}

func opsManualsOptedOut(metadata map[string]string) bool {
	if len(metadata) == 0 {
		return false
	}
	action := strings.TrimSpace(metadata["opsManualAction"])
	if strings.EqualFold(action, "skip_ops_manual") {
		return true
	}
	return strings.EqualFold(strings.TrimSpace(metadata["opsManualSkipped"]), "true")
}

func firstMetadataValue(metadata map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(metadata[key]); value != "" {
			return value
		}
	}
	return ""
}

func filterOpsManualTools(tools []promptcompiler.Tool) []promptcompiler.Tool {
	if len(tools) == 0 {
		return tools
	}
	filtered := make([]promptcompiler.Tool, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		switch toolDef.Metadata().Name {
		case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
			continue
		default:
			filtered = append(filtered, toolDef)
		}
	}
	return filtered
}

func (k *RuntimeKernel) prepareApprovalResumeExecution(session *SessionState, snapshot *TurnSnapshot, approval PendingApproval, toolCall ToolCall) (approvalResumeExecution, error) {
	if session == nil || snapshot == nil {
		return approvalResumeExecution{}, newApprovalContextStaleError("turn")
	}
	if approval.ActionToken == nil {
		return approvalResumeExecution{}, newApprovalContextStaleError("token")
	}
	world, err := k.currentApprovalResumeWorld(session, snapshot, approval, toolCall)
	if err != nil {
		return approvalResumeExecution{}, newApprovalContextStaleError("tool_router")
	}
	verified, err := VerifyActionToken(*approval.ActionToken, world.facts, time.Now())
	if err != nil {
		return approvalResumeExecution{}, err
	}
	return approvalResumeExecution{compileContext: world.compileContext, dispatcher: world.dispatcher, authorization: verified}, nil
}

type approvalCurrentWorld struct {
	compileContext promptcompiler.CompileContext
	dispatcher     *ToolDispatcher
	facts          ActionTokenCurrentFacts
	resourceScopes []string
}

func (k *RuntimeKernel) currentApprovalResumeWorld(session *SessionState, snapshot *TurnSnapshot, approval PendingApproval, toolCall ToolCall) (approvalCurrentWorld, error) {
	metadata := applyDefaultRuntimePromptProfile(cloneTurnMetadata(snapshot.Metadata), session.Type, session.HostID)
	compileCtx := enrichCompileContext(k.compileContext(session.Type, session.Mode, metadata), session.Type, session.HostID, metadata, time.Now())
	compileCtx = applyDepthProfileToCompileContext(compileCtx, snapshot.TaskDepth, firstMetadataValue(metadata, "reasoningEffort", "reasoning_effort"))
	compileCtx = applyTurnPromptProfileMetadata(compileCtx, metadata)
	compileCtx = k.appendContextArtifactToolsForExternalRefs(compileCtx, session, snapshot)
	compileCtx = appendRuntimeEnvironmentContextSection(compileCtx, session)
	compileCtx = appendSkillActivationContext(compileCtx, session)
	compileCtx = appendMCPInstructionContext(compileCtx, session)
	compileCtx.AssembledTools = filterToolsForContextMode(compileCtx.AssembledTools, k.contextBudgetPolicyForSession(session, agentKindForSession(session)).Thresholds())
	compileCtx.AssembledTools = filterHiddenTools(compileCtx.AssembledTools, snapshot.HiddenTools)
	dispatchTools := append([]promptcompiler.Tool(nil), compileCtx.AssembledTools...)
	var surfacePolicy tooling.ToolSurfacePolicySnapshot
	compileCtx, surfacePolicy = applyToolSurfacePolicyToCompileContext(compileCtx, session.Mode, firstMetadataValue(metadata, "profile", "toolProfile"), session)
	currentRouter, err := BuildStepToolRouter(StepToolRouterInput{
		Registered: toolNames(dispatchTools), ModelVisible: toolNames(compileCtx.AssembledTools), Dispatchable: toolNames(compileCtx.AssembledTools),
		HiddenReasons: hiddenReasonsFromToolSurfacePolicy(surfacePolicy), PolicyHash: surfacePolicy.Hash,
		SourceFingerprint: assembledToolFingerprint(k.tools, session.Type, session.Mode, compileCtx.AssembledTools),
	})
	if err != nil {
		return approvalCurrentWorld{}, err
	}
	compileCtx.VisibleToolFingerprint = currentRouter.Fingerprint
	currentPermissionHash := k.runtimePermissionSnapshotHash(snapshot.TurnAssembly, surfacePolicy)
	dispatcher := k.newIterationDispatcher(session, snapshot, snapshot.Iteration, compileCtx.AssembledTools, currentRouter, dispatcherPermissionBinding{
		expected: currentPermissionHash,
		current:  currentPermissionHash,
	})
	decision := dispatcher.dispatchDecisionTrace(toolCall)
	meta := toolMetadataForToolCall(compileCtx.AssembledTools, toolCall)
	resourceScopes := pendingApprovalResourceScopes(meta)
	currentTargets := pendingApprovalTargetRefs(session.HostID, resourceScopes)
	if len(currentTargets) == 0 {
		currentTargets = approvalActionTokenTargetRefs(PendingApproval{ToolName: toolCall.Name})
	}
	checkpointID := checkpointIDForStepCause(snapshot)
	current := ActionTokenCurrentFacts{
		ApprovalID: approval.ID, TurnID: snapshot.ID, ToolCallID: toolCall.ID, ToolName: toolCall.Name,
		ArgumentsHash: decision.ArgumentsHash, TargetRefs: currentTargets,
		ToolSurfaceFingerprint: decision.ToolSurfaceFingerprint, PermissionHash: decision.PermissionSnapshotHash,
		RollbackHash: approvalRollbackContractHash(approval), CheckpointID: checkpointID,
	}
	return approvalCurrentWorld{compileContext: compileCtx, dispatcher: dispatcher, facts: current, resourceScopes: resourceScopes}, nil
}

func (k *RuntimeKernel) blockStaleApprovalContext(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, approval PendingApproval, toolCall ToolCall, cause error) (TurnResult, error) {
	fields := []string{"token"}
	if stale, ok := cause.(*ApprovalContextStaleError); ok && len(stale.MismatchFields) > 0 {
		fields = append([]string(nil), stale.MismatchFields...)
	}
	now := time.Now()
	if reissued, checkpoint, err := k.reissueStaleApprovalBinding(session, snapshot, approval, toolCall, now); err == nil {
		approval = reissued
		if err := k.recordCanonicalApprovalRequestedWithMismatches(ctx, snapshot, approval, fields); err != nil {
			return TurnResult{}, fmt.Errorf("record reissued approval request: %w", err)
		}
		snapshot.LatestCheckpoint = checkpoint
		session.LatestCheckpoint = checkpoint
	}
	payload, _ := json.Marshal(map[string]any{
		"schemaVersion": "aiops.approval-context-stale/v1", "code": ApprovalContextStaleCode,
		"approvalId": approval.ID, "decision": "requires_reapproval", "mismatchFields": uniqueSortedTraceStrings(fields),
	})
	reason := string(payload)
	approval.Reason = reason
	approval.Status = "pending"
	approval.UpdatedAt = now
	snapshot.Lifecycle = TurnLifecycleSuspended
	snapshot.ResumeState = TurnResumeStatePendingApproval
	snapshot.Error = reason
	snapshot.UpdatedAt = now
	snapshot.PendingApprovals = []PendingApproval{approval}
	snapshot.PendingEvidence = nil
	session.PendingApprovals = []PendingApproval{approval}
	session.PendingEvidence = nil
	if last := latestIteration(snapshot); last != nil {
		last.PendingApprovals = []PendingApproval{approval}
		last.PendingEvidence = nil
		last.Checkpoint = snapshot.LatestCheckpoint
		last.ResumeState = TurnResumeStatePendingApproval
		last.Lifecycle = TurnLifecycleSuspended
		last.UpdatedAt = now
	}
	k.persistTurnSnapshot(session, snapshot)
	if k.projector != nil {
		k.projector.Emit(LifecycleEvent{Type: EventApprovalNeeded, SessionID: session.ID, TurnID: snapshot.ID, Timestamp: now, Payload: payload})
	}
	return TurnResult{
		SessionType: session.Type, Mode: session.Mode, SessionID: session.ID, TurnID: snapshot.ID,
		ClientTurnID: snapshot.ClientTurnID, ClientMessageID: snapshot.ClientMessageID,
		Status: "blocked", Error: reason,
	}, nil
}

func (k *RuntimeKernel) reissueStaleApprovalBinding(session *SessionState, snapshot *TurnSnapshot, approval PendingApproval, toolCall ToolCall, now time.Time) (PendingApproval, *CheckpointMetadata, error) {
	world, err := k.currentApprovalResumeWorld(session, snapshot, approval, toolCall)
	if err != nil {
		return PendingApproval{}, nil, err
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), ApprovalContextStaleCode, TurnLifecycleSuspended, TurnResumeStatePendingApproval)
	checkpoint.Source = "runtime"
	approval.ID = fmt.Sprintf("approval-reissued-%d", now.UnixNano())
	if approval.Source == "pending_evidence" {
		approval.Source = "runtime_reapproval"
	}
	approval.ToolName = toolCall.Name
	approval.ToolCallID = toolCall.ID
	approval.HostID = session.HostID
	approval.Command = approvalCommandForToolCall(toolCall)
	approval.ArgumentsHash = world.facts.ArgumentsHash
	approval.InputHash = world.facts.ArgumentsHash
	approval.TargetRefs = append([]string(nil), world.facts.TargetRefs...)
	approval.ResourceScopes = append([]string(nil), world.resourceScopes...)
	approval.RequestedScope = strings.Join(world.facts.TargetRefs, ",")
	approval.ApprovalScope = approval.RequestedScope
	approval.ToolSurfaceFingerprint = world.facts.ToolSurfaceFingerprint
	approval.PermissionSnapshotHash = world.facts.PermissionHash
	approval.IdempotencyKey = world.facts.ArgumentsHash
	approval.Status = "pending"
	approval.CreatedAt = now
	approval.UpdatedAt = now
	approval.DecidedAt = nil
	approval.Decision = ""
	approval.ExpiresAt = pendingApprovalDefaultExpiry(now)
	approval.ActionToken = nil
	contract := BuildActionRollbackContractFromApproval(approval)
	contract.ActionID = approval.ID
	contract.ToolName = approval.ToolName
	contract.TargetRefs = append([]string(nil), approval.TargetRefs...)
	contract.InputHash = approval.InputHash
	contract.ApprovalScope = approval.ApprovalScope
	contract.ResourceScopes = append([]string(nil), approval.ResourceScopes...)
	contract.IdempotencyKey = approval.IdempotencyKey
	contract.ToolSurfaceFingerprint = approval.ToolSurfaceFingerprint
	contract.PermissionSnapshotHash = approval.PermissionSnapshotHash
	approval.RollbackContract = contract.Normalize()
	if err := ValidatePendingApprovalRollbackContract(approval); err != nil {
		return PendingApproval{}, nil, err
	}
	token, err := BuildPendingApprovalActionToken(approval, checkpoint.ID)
	if err != nil {
		return PendingApproval{}, nil, err
	}
	approval.ActionToken = &token
	return approval, checkpoint, nil
}

type approvalResumeExecution struct {
	compileContext       promptcompiler.CompileContext
	dispatcher           *ToolDispatcher
	authorization        VerifiedActionToken
	approvalID           string
	rememberSessionGrant bool
}

func (k *RuntimeKernel) resumePendingToolCall(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, execution approvalResumeExecution) (*TurnResult, error) {
	toolCall, ok := pendingToolCall(snapshot)
	if !ok {
		return nil, fmt.Errorf("turn %q has no pending tool call", snapshot.ID)
	}
	if err := validateSnapshotResuming(session, snapshot); err != nil {
		return nil, err
	}
	if err := k.recordCanonicalToolDispatch(ctx, snapshot, []ToolCall{toolCall}); err != nil {
		return nil, fmt.Errorf("record approved tool dispatch: %w", err)
	}
	checkpoint, err := k.prepareSnapshotResumeBoundary(ctx, snapshot, "resume_tool_approval")
	if err != nil {
		return nil, err
	}
	snapshot.PendingStepCause = &StepRevisionCause{
		Kind: StepRevisionKindApprovalResumed, ApprovalID: execution.approvalID,
		ToolCallID: toolCall.ID, CheckpointID: checkpointIDForStepCause(snapshot),
	}
	if execution.rememberSessionGrant {
		rememberSessionApprovalGrant(session, toolCall, execution.approvalID)
	}
	k.commitSnapshotResuming(session, snapshot, checkpoint)
	dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(session.HostID, snapshot.Metadata))
	markToolInvocationRunning(snapshot, toolCall.ID)
	k.persistTurnSnapshot(session, snapshot)
	result := execution.dispatcher.DispatchApproved(dispatchCtx, session.ID, snapshot.ID, toolCall, session.Type, session.Mode, execution.authorization)
	appendResourceLockTraces(snapshot, result.ResourceLocks)
	appendToolAttemptStates(snapshot, toolCall.ID, result.Attempts)
	if result.Blocked {
		markToolInvocationBlocked(snapshot, toolCall.ID)
		if err := k.markTurnBlocked(session, snapshot, toolCall, result); err != nil {
			return nil, err
		}
		return blockedTurnResult(session, snapshot, result.Reason), nil
	}
	if result.Error != "" {
		if !shouldFeedToolFailureBackToModel(result) {
			markToolInvocationFailed(snapshot, toolCall.ID, failureKindForDispatchResult(result))
			if err := k.markTurnFailed(session, snapshot, toolCall, result); err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("tool %q failed: %s", toolCall.Name, result.Error)
		}
		result.Result = failedToolResultForModel(toolCall, result)
		if strings.TrimSpace(result.Metadata.Name) == "" {
			result.Metadata.Name = toolCall.Name
		}
	}
	recordedResult, materializeErr := k.recordResumedToolResult(ctx, session, snapshot, snapshot.Iteration, toolCall, result.Metadata, result.Result, failureKindForDispatchResult(result))
	if materializeErr != nil {
		markToolInvocationFailed(snapshot, toolCall.ID, "")
		return nil, fmt.Errorf("materialize resumed tool result %q: %w", toolCall.Name, materializeErr)
	}
	appendAgentItem(snapshot, newAgentItem(
		toolResultItemID(snapshot.ID, toolCall),
		agentstate.TurnItemTypeToolResult,
		toolResultItemStatus(recordedResult),
		truncateString(recordedResult.Content, 240),
		map[string]string{"toolCallId": toolCall.ID, "toolName": toolCall.Name},
	))
	if recordedResult.Error != "" {
		markToolInvocationFailed(snapshot, toolCall.ID, failureKindForDispatchResult(result))
	} else if recordedResult.Outcome.Normalize() == tooling.ToolResultOutcomePartial {
		markToolInvocationPartial(snapshot, toolCall.ID)
	} else {
		markToolInvocationCompleted(snapshot, toolCall.ID)
	}
	checkpointRef := ""
	if snapshot.LatestCheckpoint != nil {
		checkpointRef = snapshot.LatestCheckpoint.ID
	}
	if err := k.recordCanonicalTransportProjection(ctx, snapshot, snapshot.Lifecycle, snapshot.ResumeState, checkpointRef, nil); err != nil {
		return nil, err
	}
	k.persistTurnSnapshot(session, snapshot)
	return k.drainRemainingToolCallsAfterResume(ctx, session, snapshot, execution.compileContext, execution.dispatcher)
}

func blockedTurnResult(session *SessionState, snapshot *TurnSnapshot, reason string) *TurnResult {
	return &TurnResult{
		SessionType:     session.Type,
		Mode:            session.Mode,
		SessionID:       session.ID,
		TurnID:          snapshot.ID,
		ClientTurnID:    snapshot.ClientTurnID,
		ClientMessageID: snapshot.ClientMessageID,
		Status:          "blocked",
		Error:           reason,
	}
}

func (k *RuntimeKernel) drainRemainingToolCallsAfterResume(
	ctx context.Context,
	session *SessionState,
	snapshot *TurnSnapshot,
	compileCtx promptcompiler.CompileContext,
	dispatcher *ToolDispatcher,
) (*TurnResult, error) {
	last := latestIteration(snapshot)
	if last == nil {
		return nil, nil
	}
	for _, tc := range last.ToolCalls {
		if iterationHasToolResult(last, tc.ID) {
			continue
		}
		toolItemID := toolCallItemID(snapshot.ID, tc)
		appendAgentItem(snapshot, newAgentItem(
			toolItemID,
			agentstate.TurnItemTypeToolCall,
			agentstate.ItemStatusRunning,
			tc.Name,
			tc,
		))
		queueToolInvocation(snapshot, snapshot.Iteration, tc, toolMetadataForToolCall(compileCtx.AssembledTools, tc))
		k.persistTurnSnapshot(session, snapshot)
		markToolInvocationRunning(snapshot, tc.ID)
		k.persistTurnSnapshot(session, snapshot)

		dispatchResult, reused := maybeReuseCoveredReadResult(snapshot, compileCtx.AssembledTools, tc)
		if !reused {
			if err := k.recordCanonicalToolDispatch(ctx, snapshot, []ToolCall{tc}); err != nil {
				return nil, fmt.Errorf("record resumed tool dispatch: %w", err)
			}
			dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(session.HostID, snapshot.Metadata))
			dispatchResult = dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, snapshot.ID, tc, session.Type, session.Mode, "")
		}
		if dispatchResult.ToolCallID == "" {
			dispatchResult.ToolCallID = tc.ID
		}
		if strings.TrimSpace(dispatchResult.Metadata.Name) == "" {
			dispatchResult.Metadata = toolMetadataForToolCall(compileCtx.AssembledTools, tc)
		}
		appendResourceLockTraces(snapshot, dispatchResult.ResourceLocks)
		appendToolAttemptStates(snapshot, tc.ID, dispatchResult.Attempts)
		if dispatchResult.Blocked {
			updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusBlocked, dispatchResult.Reason)
			markToolInvocationBlocked(snapshot, tc.ID)
			if err := k.markTurnBlocked(session, snapshot, tc, dispatchResult); err != nil {
				return nil, err
			}
			return blockedTurnResult(session, snapshot, dispatchResult.Reason), nil
		}
		if dispatchResult.Error != "" {
			if !shouldFeedToolFailureBackToModel(dispatchResult) {
				updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusFailed, dispatchResult.Error)
				appendAgentItem(snapshot, newAgentItem(errorItemID(snapshot.ID, snapshot.Iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, dispatchResult.Error, map[string]string{"toolCallId": tc.ID, "toolName": tc.Name}))
				markToolInvocationFailed(snapshot, tc.ID, failureKindForDispatchResult(dispatchResult))
				if err := k.markTurnFailed(session, snapshot, tc, dispatchResult); err != nil {
					return nil, err
				}
				return nil, fmt.Errorf("tool %q failed: %s", tc.Name, dispatchResult.Error)
			}
			dispatchResult.Result = failedToolResultForModel(tc, dispatchResult)
			if strings.TrimSpace(dispatchResult.Metadata.Name) == "" {
				dispatchResult.Metadata.Name = tc.Name
			}
		}
		applyHiddenTools(snapshot, dispatchResult.HiddenTools)
		recordedResult, materializeErr := k.recordResumedToolResult(ctx, session, snapshot, last.Iteration, tc, dispatchResult.Metadata, dispatchResult.Result, failureKindForDispatchResult(dispatchResult))
		if materializeErr != nil {
			updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusFailed, materializeErr.Error())
			appendAgentItem(snapshot, newAgentItem(errorItemID(snapshot.ID, snapshot.Iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, materializeErr.Error(), map[string]string{"toolCallId": tc.ID, "toolName": tc.Name}))
			markToolInvocationFailed(snapshot, tc.ID, "")
			k.persistTurnSnapshot(session, snapshot)
			return nil, fmt.Errorf("materialize resumed tool result %q: %w", tc.Name, materializeErr)
		}
		updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusCompleted, tc.Name)
		appendAgentItem(snapshot, newAgentItem(
			toolResultItemID(snapshot.ID, tc),
			agentstate.TurnItemTypeToolResult,
			toolResultItemStatus(recordedResult),
			truncateString(recordedResult.Content, 240),
			toolResultAgentItemData(snapshot.ID, tc, recordedResult),
		))
		if planItem, ok := planItemFromToolCall(snapshot.ID, tc); ok {
			appendAgentItem(snapshot, planItem)
		}
		if recordedResult.Error != "" {
			markToolInvocationFailed(snapshot, tc.ID, failureKindForDispatchResult(dispatchResult))
		} else if recordedResult.Outcome.Normalize() == tooling.ToolResultOutcomePartial {
			markToolInvocationPartial(snapshot, tc.ID)
		} else {
			markToolInvocationCompleted(snapshot, tc.ID)
		}
		k.persistTurnSnapshot(session, snapshot)
	}
	return nil, nil
}

func iterationHasToolResult(iter *IterationState, toolCallID string) bool {
	if iter == nil || strings.TrimSpace(toolCallID) == "" {
		return false
	}
	for _, result := range iter.ToolResults {
		if result.ToolCallID == toolCallID {
			return true
		}
	}
	return false
}

func toolResultAgentItemData(turnID string, tc ToolCall, result ToolResult) map[string]any {
	outputSummary, _, _, rawRef, resultBytes, resultTruncated := summarizeToolLifecycleResultForEvent(turnID, tc.ID, result.Content)
	if terminal := terminalEnvelopeFromToolResultContent(result.Content); terminal != nil {
		if terminal.Command != "" {
			outputSummary = terminal.Command
		}
	}
	payload := map[string]any{
		"toolCallId":      tc.ID,
		"toolName":        tc.Name,
		"outputSummary":   outputSummary,
		"rawRef":          rawRef,
		"resultBytes":     resultBytes,
		"resultTruncated": resultTruncated,
	}
	if result.Outcome != "" {
		payload["outcome"] = result.Outcome
	}
	if inputSummary := strings.TrimSpace(approvalCommandForToolCall(tc)); inputSummary != "" {
		payload["inputSummary"] = inputSummary
	}
	if len(tc.Arguments) > 0 {
		payload["arguments"] = json.RawMessage(append([]byte(nil), tc.Arguments...))
	}
	if evidenceRefs := evidenceRefsFromToolResultContent(result.Content); len(evidenceRefs) > 0 {
		payload["evidenceRefs"] = evidenceRefs
	}
	if result.Error != "" {
		payload["error"] = result.Error
	}
	if result.Display != nil {
		payload["displayKind"] = result.Display.Type
		if len(result.Display.Data) > 0 {
			payload["displayData"] = append(json.RawMessage(nil), result.Display.Data...)
		}
	}
	if result.MaterializationTier != "" {
		payload["materializationTier"] = result.MaterializationTier
	}
	if result.OriginalBytes > 0 {
		payload["originalBytes"] = result.OriginalBytes
	}
	if result.InlineBytes > 0 {
		payload["inlineBytes"] = result.InlineBytes
	}
	if result.Spilled {
		payload["spilled"] = true
	}
	if len(result.ExternalReferences) > 0 {
		payload["externalReferences"] = result.ExternalReferences
	}
	return payload
}

func finalCompletenessFailureError(decision FinalCompletenessDecision) error {
	reasons := strings.Join(compactStringList(decision.Reasons), ", ")
	if reasons == "" {
		reasons = "possible_incomplete_assistant_message"
	}
	return fmt.Errorf("assistant message incomplete: %s", reasons)
}

func providerResponseFinishReason(response modelrouter.ProviderResponse) string {
	return strings.TrimSpace(response.FinishReason)
}

func canonicalRolloutProviderFinishReason(response modelrouter.ProviderResponse) string {
	switch reason := strings.ToLower(providerResponseFinishReason(response)); reason {
	case "stop", "tool_calls", "length", "content_filter", "cancelled", "error":
		return reason
	case "":
		return "unspecified"
	default:
		return "unknown"
	}
}

func compactStringList(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

type terminalToolResultEnvelope struct {
	Command string `json:"command"`
	Stdout  string `json:"stdout"`
}

func terminalEnvelopeFromToolResultContent(content string) *terminalToolResultEnvelope {
	content = strings.TrimSpace(content)
	if content == "" || !strings.HasPrefix(content, "{") {
		return nil
	}
	var payload struct {
		SchemaVersion string `json:"schemaVersion"`
		Tool          string `json:"tool"`
		Command       string `json:"command"`
		Stdout        string `json:"stdout"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil
	}
	if payload.SchemaVersion != "aiops.terminal/v1" && payload.Tool != "exec_command" {
		return nil
	}
	return &terminalToolResultEnvelope{
		Command: strings.TrimSpace(payload.Command),
		Stdout:  strings.TrimSpace(payload.Stdout),
	}
}

func evidenceRefsFromToolResultContent(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" || !strings.HasPrefix(content, "{") {
		return nil
	}
	var payload struct {
		EvidenceRefs []string `json:"evidenceRefs"`
		Data         struct {
			EvidenceRefs []string `json:"evidenceRefs"`
			EvidenceRef  string   `json:"evidenceRef"`
		} `json:"data"`
	}
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return nil
	}
	refs := append([]string(nil), payload.EvidenceRefs...)
	refs = append(refs, payload.Data.EvidenceRefs...)
	if strings.TrimSpace(payload.Data.EvidenceRef) != "" {
		refs = append(refs, payload.Data.EvidenceRef)
	}
	return cleanEvidenceRefs(refs)
}

func assistantMessageEvidenceRefsFromSnapshot(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	var refs []string
	for _, item := range snapshot.AgentItems {
		switch item.Type {
		case agentstate.TurnItemTypeEvidence:
			if id := strings.TrimSpace(item.ID); id != "" {
				refs = append(refs, id)
			}
		case agentstate.TurnItemTypeToolResult:
			var payload struct {
				EvidenceRefs []string `json:"evidenceRefs"`
			}
			if len(item.Payload.Data) > 0 && json.Unmarshal(item.Payload.Data, &payload) == nil {
				refs = append(refs, payload.EvidenceRefs...)
			}
		}
	}
	return cleanEvidenceRefs(refs)
}

func cleanEvidenceRefs(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		ref := strings.TrimSpace(value)
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, ref)
	}
	return out
}

func (k *RuntimeKernel) recordResumedToolResult(ctx context.Context, session *SessionState, snapshot *TurnSnapshot, iteration int, toolCall ToolCall, meta tooling.ToolMetadata, result tooling.ToolResult, errorClass string) (ToolResult, error) {
	recordedResult, materializeErr := k.materializeToolResult(session, snapshot, iteration, toolCall, meta, result)
	if materializeErr != nil {
		return ToolResult{}, materializeErr
	}
	if err := k.recordCanonicalToolResult(ctx, snapshot, toolCall, recordedResult, errorClass); err != nil {
		return ToolResult{}, err
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, iteration, nextCheckpointSequence(snapshot), "resume_tool_result", TurnLifecycleRunning, TurnResumeStateCheckpointReady)
	checkpoint.Incremental = true
	if err := k.recordCanonicalCheckpoint(ctx, snapshot, checkpoint); err != nil {
		return ToolResult{}, err
	}
	now := time.Now()
	snapshot.Lifecycle = TurnLifecycleRunning
	snapshot.ResumeState = TurnResumeStateCheckpointReady
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	snapshot.UpdatedAt = now
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	toolMsg := Message{
		ID:         fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Role:       "tool",
		Content:    recordedResult.Content,
		Timestamp:  now,
		ToolResult: &recordedResult,
	}
	session.Messages = append(session.Messages, toolMsg)
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleRunning
		last.ResumeState = TurnResumeStateCheckpointReady
		last.PendingApprovals = nil
		last.PendingEvidence = nil
		last.ToolResults = append(last.ToolResults, recordedResult)
		appendExternalReferences(&last.ExternalReferences, recordedResult.ExternalReferences...)
		last.UpdatedAt = now
	}
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteToolResult, OwnerToolDispatcher)
	snapshot.LatestCheckpoint = checkpoint
	appendExternalReferences(&snapshot.ExternalReferences, recordedResult.ExternalReferences...)
	appendExternalReferences(&session.ExternalReferences, recordedResult.ExternalReferences...)
	appendCheckpointExternalRefs(snapshot.LatestCheckpoint, recordedResult.ExternalReferences)
	if last := latestIteration(snapshot); last != nil {
		last.Checkpoint = snapshot.LatestCheckpoint
	}
	session.LatestCheckpoint = snapshot.LatestCheckpoint
	return recordedResult, nil
}

func appendResumeInputMessage(session *SessionState, input string) {
	if session == nil {
		return
	}
	text := strings.TrimSpace(input)
	if text == "" {
		return
	}
	msg := Message{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Role:      "user",
		Content:   text,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, msg)
	recomputeContextWindow(&session.Context, session.Messages)
}

func (k *RuntimeKernel) markSnapshotResuming(session *SessionState, snapshot *TurnSnapshot, checkpointKind string) error {
	if err := validateSnapshotResuming(session, snapshot); err != nil {
		return err
	}
	checkpoint, err := k.prepareSnapshotResumeBoundary(context.Background(), snapshot, checkpointKind)
	if err != nil {
		return err
	}
	k.commitSnapshotResuming(session, snapshot, checkpoint)
	return nil
}

func validateSnapshotResuming(session *SessionState, snapshot *TurnSnapshot) error {
	if session == nil || snapshot == nil {
		return fmt.Errorf("session and snapshot are required")
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnResumed, TurnLifecycleRunning); err != nil {
		return err
	}
	return nil
}

func (k *RuntimeKernel) prepareSnapshotResumeBoundary(ctx context.Context, snapshot *TurnSnapshot, checkpointKind string) (*CheckpointMetadata, error) {
	checkpoint := newCheckpointMetadata(snapshot.SessionID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), checkpointKind, TurnLifecycleRunning, TurnResumeStateNone)
	candidate := *snapshot
	candidate.Lifecycle = TurnLifecycleRunning
	candidate.ResumeState = TurnResumeStateNone
	candidate.PendingApprovals = nil
	candidate.PendingEvidence = nil
	candidate.LatestCheckpoint = checkpoint
	if err := k.recordCanonicalCheckpoint(ctx, &candidate, checkpoint); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return nil, err
	}
	if err := k.recordCanonicalTransportProjection(ctx, &candidate, candidate.Lifecycle, candidate.ResumeState, checkpoint.ID, nil); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return nil, err
	}
	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	return checkpoint, nil
}

func (k *RuntimeKernel) commitSnapshotResuming(session *SessionState, snapshot *TurnSnapshot, checkpoint *CheckpointMetadata) {
	now := time.Now()
	snapshot.Lifecycle = TurnLifecycleRunning
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = ""
	snapshot.UpdatedAt = now
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	snapshot.LatestCheckpoint = checkpoint
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	session.LatestCheckpoint = snapshot.LatestCheckpoint
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleRunning
		last.ResumeState = TurnResumeStateNone
		last.PendingApprovals = nil
		last.PendingEvidence = nil
		last.Checkpoint = snapshot.LatestCheckpoint
		last.UpdatedAt = now
	}
	k.persistTurnSnapshot(session, snapshot)
}

type dispatcherPermissionBinding struct {
	expected string
	current  string
}

func (k *RuntimeKernel) runtimePermissionSnapshotHash(assembly *agentassembly.TurnAssembly, policy tooling.ToolSurfacePolicySnapshot) string {
	if assembly == nil || assembly.Validate() != nil {
		return ""
	}
	base := runtimeStepPermissionHash(assembly, policy)
	if base == "" {
		return ""
	}
	var rules []permissions.Rule
	if k != nil && k.permissions != nil {
		rules = k.permissions.Rules()
	}
	payload, err := json.Marshal(map[string]any{
		"turnPermissionHash": base,
		"permissionRules":    rules,
	})
	if err != nil {
		return ""
	}
	return toolArgumentsHash(payload)
}

func (k *RuntimeKernel) newIterationDispatcher(session *SessionState, snapshot *TurnSnapshot, iteration int, tools []promptcompiler.Tool, runtimeToolSurface RuntimeToolRouterSnapshot, permissionBindings ...dispatcherPermissionBinding) *ToolDispatcher {
	runtimeToolSurface = hydrateStepToolRouterForDispatch(tools, runtimeToolSurface)
	lookup := assembledToolLookup{byName: make(map[string]tooling.Tool, len(tools))}
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		meta := toolDef.Metadata()
		addToolLookupName(lookup.byName, meta.Name, toolDef)
		for _, alias := range meta.Aliases {
			addToolLookupName(lookup.byName, alias, toolDef)
		}
	}
	dispatcher := NewToolDispatcher(lookup, k.policy, k.projector)
	if k.spanSource != nil {
		dispatcher = NewToolDispatcherWithSpans(lookup, k.policy, k.projector, k.spanSource)
	}
	dispatcher = dispatcher.
		WithPermissions(k.permissions).
		WithSessionApprovalGrants(session.ApprovalGrants).
		WithPlanApprovalContext(session.PlanMode, session.PlanApprovalScopes).
		WithUnexpectedStateSignals(collectUnexpectedStateSignalsFromSession(session)).
		WithHooks(k.hooks).
		WithObserver(k.runtimeObserver()).
		WithStepToolRouter(runtimeToolSurface).
		WithVisibleToolMetadata(toolMetadataList(tools)).
		WithReadOnlyRetryConfig(ReadOnlyRetryConfigFromFlags(k.runtimeFeatureFlags(context.Background()))).
		WithExecutionScopeGuard(executionScopeGuardConfigFromSnapshot(snapshot)).
		WithRoleBindingGuard(roleBindingGuardConfigFromSession(session, snapshot)).
		WithResourceLockGate(k.resourceLockGate).
		WithProgressSink(k.progressSink(session, snapshot, iteration))
	if len(permissionBindings) > 0 {
		binding := permissionBindings[0]
		dispatcher = dispatcher.WithPermissionBinding(binding.expected, binding.current)
	}
	if snapshot.ToolSurfaceSnapshot != nil && snapshot.ToolSurfaceSnapshot.PolicySnapshot != nil {
		dispatcher = dispatcher.WithToolSurfacePolicySnapshot(snapshot.ToolSurfaceSnapshot.PolicySnapshot)
	}
	if catalog := k.deferredCatalogLookup(session.Type, session.Mode); catalog != nil {
		dispatcher = dispatcher.WithDeferredCatalogLookup(catalog)
	}
	return dispatcher
}

func toolMetadataList(tools []promptcompiler.Tool) []tooling.ToolMetadata {
	out := make([]tooling.ToolMetadata, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef != nil {
			out = append(out, toolDef.Metadata())
		}
	}
	return out
}

func (k *RuntimeKernel) deferredCatalogLookup(session SessionType, mode Mode) DeferredToolCatalogLookup {
	source, ok := k.tools.(fullToolCatalogSource)
	if !ok {
		return nil
	}
	tools := source.AssembleToolsWithOptions(string(session), string(mode), tooling.AssembleOptions{IncludeDeferredCatalog: true})
	catalog := deferredCatalogLookup{byName: make(map[string]tooling.ToolMetadata)}
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		meta := toolDef.Metadata()
		if meta.Name == "" || tooling.ToolHiddenFromDiscovery(meta) || tooling.ToolExcludedFromDeferredDiscovery(meta) {
			continue
		}
		if meta.Layer != tooling.ToolLayerDeferred && !meta.DeferByDefault && meta.Pack == "" {
			continue
		}
		catalog.byName[meta.Name] = meta
		for _, alias := range meta.Aliases {
			if strings.TrimSpace(alias) != "" {
				catalog.byName[alias] = meta
			}
		}
	}
	if len(catalog.byName) == 0 {
		return nil
	}
	return catalog
}

type deferredCatalogLookup struct {
	byName map[string]tooling.ToolMetadata
}

func (l deferredCatalogLookup) LookupDeferredTool(name string) (tooling.ToolMetadata, bool) {
	meta, ok := l.byName[name]
	return meta, ok
}

func reasoningSummaryKey(event modelrouter.ReasoningStreamEvent) string {
	itemID := reasoningItemID(event)
	return fmt.Sprintf("%s:%d", itemID, event.SummaryIndex)
}

func reasoningItemID(event modelrouter.ReasoningStreamEvent) string {
	itemID := strings.TrimSpace(event.ItemID)
	if itemID != "" {
		return itemID
	}
	turnID := strings.TrimSpace(event.TurnID)
	if turnID == "" {
		turnID = "turn"
	}
	return fmt.Sprintf("%s:reasoning:%d", turnID, event.SummaryIndex)
}

func runtimeMessageFromProviderResponse(response modelrouter.ProviderResponse) Message {
	return Message{
		Role:             "assistant",
		Content:          response.Output,
		ReasoningContent: response.ReasoningContent,
		ToolCalls:        runtimeToolCallsFromModelInput(response.ToolCalls),
		Timestamp:        time.Now(),
	}
}

func runtimeToolCallsFromModelInput(toolCalls []promptinput.ModelInputToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		args := append(json.RawMessage(nil), call.Arguments...)
		out = append(out, ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: args,
		})
	}
	return out
}

const (
	defaultMaxInlineResultBytes  = 4096
	defaultLargeResultMultiplier = 4
)

type toolResultMaterializationTier string

const (
	toolResultTierSmall  toolResultMaterializationTier = "small"
	toolResultTierMedium toolResultMaterializationTier = "medium"
	toolResultTierLarge  toolResultMaterializationTier = "large"
)

func (k *RuntimeKernel) materializeToolResult(session *SessionState, snapshot *TurnSnapshot, iteration int, tc ToolCall, meta tooling.ToolMetadata, toolResult tooling.ToolResult) (ToolResult, error) {
	result := ToolResult{
		ToolCallID:         tc.ID,
		TargetIdentityHash: frozenResourceTargetIdentityHashFromSnapshot(snapshot),
		Content:            toolResult.Content,
		Display:            copyToolDisplay(toolResult.Display),
		Error:              toolResult.Error,
		Outcome:            toolResult.Outcome,
		References:         normalizeToolResultReferences(toolResult.References, toolResult.Display),
	}
	defaultInlineBytes := defaultInlineResultBytesForContext(k.contextBudgetPolicyForSession(session, agentKindForSession(session)).Thresholds())
	budget := mergeResultBudget(meta.EffectiveResultBudget(defaultInlineBytes), toolResult.ResultBudget, defaultInlineBytes)
	appendExternalReferences(&result.ExternalReferences, externalReferencesFromToolResultRefs(session, snapshot, iteration, result.References)...)

	if toolResult.HasSpill() {
		originalBytes := spillContentBytes(toolResult.Spill, toolResult.Content)
		ref, err := k.persistToolResultSpill(session, snapshot, iteration, tc, meta, toolResult.Spill)
		if err != nil {
			return ToolResult{}, err
		}
		result.Spilled = true
		result.Summary = fallbackSummary(toolResult.Spill.Summary, toolResult.Content, budget.MaxInlineResultBytes)
		tier := classifyToolResultTier(originalBytes, budget)
		result.Content = materializedInlineContent(toolResult.Content, result.Summary, ref, budget, tier)
		result.References = appendToolResultReferences(result.References, toolResultReferenceFromExternalRef(ref))
		appendExternalReferences(&result.ExternalReferences, ref)
		finalizeToolResultMaterialization(session, snapshot, iteration, tc, &result, tier, originalBytes)
		return result, nil
	}

	inlineBytes := len(toolResult.Content)
	tier := classifyToolResultTier(inlineBytes, budget)
	if tier == toolResultTierSmall {
		finalizeToolResultMaterialization(session, snapshot, iteration, tc, &result, tier, inlineBytes)
		return result, nil
	}

	summary := fallbackSummary("", toolResult.Content, budget.MaxInlineResultBytes)
	spill := &tooling.ResultSpill{
		ID:          spillID(snapshot.ID, iteration, tc.ID, toolResult.Content),
		ToolCallID:  tc.ID,
		ToolName:    tc.Name,
		SessionID:   session.ID,
		TurnID:      snapshot.ID,
		ContentType: detectResultContentType(toolResult.Content),
		Summary:     summary,
		Content:     []byte(toolResult.Content),
		Bytes:       int64(len(toolResult.Content)),
		CreatedAt:   time.Now().UTC(),
	}
	ref, err := k.persistToolResultSpill(session, snapshot, iteration, tc, meta, spill)
	if err != nil {
		return ToolResult{}, err
	}

	result.Spilled = true
	result.Summary = summary
	result.Content = materializedInlineContent(toolResult.Content, summary, ref, budget, tier)
	result.References = appendToolResultReferences(result.References, toolResultReferenceFromExternalRef(ref))
	appendExternalReferences(&result.ExternalReferences, ref)
	finalizeToolResultMaterialization(session, snapshot, iteration, tc, &result, tier, inlineBytes)
	return result, nil
}

func frozenResourceTargetIdentityHashFromSnapshot(snapshot *TurnSnapshot) string {
	if snapshot == nil || snapshot.TurnAssembly == nil || snapshot.TurnAssembly.Validate() != nil {
		return ""
	}
	return resourceTargetIdentityHash(snapshot.TurnAssembly.AdmissionFacts)
}

func (k *RuntimeKernel) persistToolResultSpill(session *SessionState, snapshot *TurnSnapshot, iteration int, tc ToolCall, meta tooling.ToolMetadata, spill *tooling.ResultSpill) (ExternalReference, error) {
	if spill == nil {
		return ExternalReference{}, fmt.Errorf("tool result spill is nil")
	}
	if spill.ID == "" {
		spill.ID = spillID(snapshot.ID, iteration, tc.ID, string(spill.Content))
	}
	if spill.ToolCallID == "" {
		spill.ToolCallID = tc.ID
	}
	if spill.ToolName == "" {
		spill.ToolName = tc.Name
	}
	if spill.SessionID == "" {
		spill.SessionID = session.ID
	}
	if spill.TurnID == "" {
		spill.TurnID = snapshot.ID
	}
	if spill.CreatedAt.IsZero() {
		spill.CreatedAt = time.Now().UTC()
	}
	if spill.Bytes == 0 {
		spill.Bytes = int64(len(spill.Content))
	}
	if spill.ContentType == "" {
		spill.ContentType = detectResultContentType(string(spill.Content))
	}
	if spill.Summary == "" {
		spill.Summary = fallbackSummary("", string(spill.Content), defaultMaxInlineResultBytes)
	}
	if k.spillRepo == nil {
		return ExternalReference{}, fmt.Errorf("tool result spill repository is not configured")
	}
	if err := k.spillRepo.SaveToolResultSpill(spill); err != nil {
		return ExternalReference{}, err
	}

	return ExternalReference{
		ID:          spill.ID,
		SessionID:   session.ID,
		TurnID:      snapshot.ID,
		Iteration:   iteration,
		Kind:        string(ToolResultReferenceKindBlob),
		URI:         "store://tool-spills/" + spill.ID,
		Title:       meta.Name,
		Summary:     spill.Summary,
		ContentType: spill.ContentType,
		Digest:      digestContent(string(spill.Content)),
		Bytes:       spill.Bytes,
		CreatedAt:   spill.CreatedAt,
	}, nil
}

func (k *RuntimeKernel) applyAggregateToolResultBudget(session *SessionState, snapshot *TurnSnapshot, iteration int, assembledTools []promptcompiler.Tool) {
	if session == nil || snapshot == nil {
		return
	}
	last := latestIteration(snapshot)
	if last == nil || len(last.ToolResults) < 2 {
		return
	}
	thresholds := k.contextBudgetPolicyForSession(session, agentKindForSession(session)).Thresholds()
	applied := ApplyAggregateToolResultBudget(AggregateToolResultBudgetInput{
		SessionID:   session.ID,
		TurnID:      snapshot.ID,
		Iteration:   iteration,
		Results:     last.ToolResults,
		Thresholds:  thresholds,
		Externalize: aggregateToolResultExternalizer(k, session, snapshot, iteration, last.ToolCalls, assembledTools),
	})
	if !applied.Applied {
		return
	}
	last.ToolResults = applied.Results
	last.UpdatedAt = time.Now()
	for _, result := range applied.Results {
		appendExternalReferences(&last.ExternalReferences, result.ExternalReferences...)
		appendExternalReferences(&snapshot.ExternalReferences, result.ExternalReferences...)
		appendExternalReferences(&session.ExternalReferences, result.ExternalReferences...)
	}
	appendContextGovernanceEvents(&last.ContextGovernanceEvents, applied.Events...)
	appendContextGovernanceEvents(&snapshot.ContextGovernanceEvents, applied.Events...)
	appendContextGovernanceEvents(&session.ContextGovernanceEvents, applied.Events...)
	updateSessionToolMessages(session, applied.Results)
	if snapshot.LatestCheckpoint != nil {
		for _, result := range applied.Results {
			appendCheckpointExternalRefs(snapshot.LatestCheckpoint, result.ExternalReferences)
		}
		snapshot.LatestCheckpoint.Incremental = true
	}
}

func aggregateToolResultExternalizer(k *RuntimeKernel, session *SessionState, snapshot *TurnSnapshot, iteration int, calls []ToolCall, assembledTools []promptcompiler.Tool) func(ToolResult) (ExternalReference, error) {
	return func(result ToolResult) (ExternalReference, error) {
		tc := toolCallByID(calls, result.ToolCallID)
		if tc.ID == "" {
			tc = ToolCall{ID: result.ToolCallID, Name: result.ToolCallID}
		}
		meta := toolMetadataForToolCall(assembledTools, tc)
		summary := fallbackSummary(result.Summary, result.Content, defaultMaxInlineResultBytes)
		spill := &tooling.ResultSpill{
			ID:          spillID(snapshot.ID, iteration, "aggregate-"+result.ToolCallID, result.Content),
			ToolCallID:  result.ToolCallID,
			ToolName:    firstNonBlankRuntimeString(meta.Name, tc.Name),
			SessionID:   session.ID,
			TurnID:      snapshot.ID,
			ContentType: detectResultContentType(result.Content),
			Summary:     summary,
			Content:     []byte(result.Content),
			Bytes:       int64(len(result.Content)),
			CreatedAt:   time.Now().UTC(),
		}
		return k.persistToolResultSpill(session, snapshot, iteration, tc, meta, spill)
	}
}

func toolCallByID(calls []ToolCall, id string) ToolCall {
	for _, call := range calls {
		if call.ID == id {
			return call
		}
	}
	return ToolCall{}
}

func updateSessionToolMessages(session *SessionState, results []ToolResult) {
	if session == nil || len(results) == 0 {
		return
	}
	byID := make(map[string]ToolResult, len(results))
	for _, result := range results {
		if result.ToolCallID != "" {
			byID[result.ToolCallID] = result
		}
	}
	for i := range session.Messages {
		if session.Messages[i].ToolResult == nil {
			continue
		}
		result, ok := byID[session.Messages[i].ToolResult.ToolCallID]
		if !ok {
			continue
		}
		cp := result
		session.Messages[i].ToolResult = &cp
		session.Messages[i].Content = result.Content
	}
}

func copyToolDisplay(display *tooling.ToolDisplayPayload) *ToolDisplayPayload {
	if display == nil {
		return nil
	}
	return &ToolDisplayPayload{
		Type:    display.Type,
		Title:   display.Title,
		Data:    append(json.RawMessage(nil), display.Data...),
		CardRef: display.CardRef,
	}
}

func mergeResultBudget(meta tooling.ResultBudget, override tooling.ResultBudget, defaultInlineBytes int) tooling.ResultBudget {
	budget := meta.Normalize(defaultInlineBytes)
	if override.MaxInlineResultBytes > 0 {
		budget.MaxInlineResultBytes = override.MaxInlineResultBytes
	}
	if override.SpillPolicy != "" {
		budget.SpillPolicy = override.SpillPolicy
	}
	if override.SummarizeLargeResult {
		budget.SummarizeLargeResult = true
	}
	return budget.Normalize(defaultInlineBytes)
}

func classifyToolResultTier(resultBytes int, budget tooling.ResultBudget) toolResultMaterializationTier {
	if resultBytes <= budget.MaxInlineResultBytes || budget.SpillPolicy == tooling.ResultSpillPolicyInline {
		return toolResultTierSmall
	}
	if budget.SpillPolicy == tooling.ResultSpillPolicyExternalize {
		return toolResultTierLarge
	}
	largeThreshold := budget.MaxInlineResultBytes * defaultLargeResultMultiplier
	if largeThreshold <= budget.MaxInlineResultBytes {
		largeThreshold = budget.MaxInlineResultBytes + 1
	}
	if resultBytes <= largeThreshold {
		return toolResultTierMedium
	}
	return toolResultTierLarge
}

func materializedInlineContent(original, summary string, ref ExternalReference, budget tooling.ResultBudget, tier toolResultMaterializationTier) string {
	switch tier {
	case toolResultTierSmall:
		return truncateForBudget(original, budget.MaxInlineResultBytes)
	case toolResultTierMedium:
		if summary == "" {
			summary = fallbackSummary("", original, budget.MaxInlineResultBytes)
		}
		return fmt.Sprintf("Summary: %s\nExternal ref: %s.", summary, externalReferenceLabel(ref))
	default:
		if summary == "" {
			summary = fallbackSummary("", original, budget.MaxInlineResultBytes)
		}
		return fmt.Sprintf("Summary: %s\nExternal ref: %s.", summary, externalReferenceLabel(ref))
	}
}

func fallbackSummary(existing, content string, maxInlineBytes int) string {
	if strings.TrimSpace(existing) != "" {
		return existing
	}
	if maxInlineBytes <= 0 {
		maxInlineBytes = 256
	}
	limit := maxInlineBytes / 4
	if limit < 96 {
		limit = 96
	}
	return summarizeSnippet(truncateForBudget(content, limit))
}

func spillID(turnID string, iteration int, toolCallID, content string) string {
	return fmt.Sprintf("spill-%s-%d-%s-%s", turnID, iteration, toolCallID, digestContent(content)[:12])
}

func digestContent(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func detectResultContentType(content string) string {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "[") {
		return "application/json"
	}
	return "text/plain"
}

func spillContentBytes(spill *tooling.ResultSpill, fallback string) int {
	if spill == nil {
		return len(fallback)
	}
	if spill.Bytes > 0 {
		return int(spill.Bytes)
	}
	if len(spill.Content) > 0 {
		return len(spill.Content)
	}
	return len(fallback)
}

func normalizeToolResultReferences(refs []tooling.ResultReference, display *tooling.ToolDisplayPayload) []ToolResultReference {
	out := make([]ToolResultReference, 0, len(refs)+1)
	for _, ref := range refs {
		out = appendToolResultReferences(out, ToolResultReference{
			Kind:        ToolResultReferenceKind(ref.Kind),
			URI:         ref.URI,
			CardRef:     ref.CardRef,
			FilePath:    ref.FilePath,
			Title:       ref.Title,
			Summary:     ref.Summary,
			ContentType: ref.ContentType,
			Digest:      ref.Digest,
			Bytes:       ref.Bytes,
			Version:     ref.Version,
			Range:       ref.Range,
		})
	}
	if display != nil && display.CardRef != "" {
		out = appendToolResultReferences(out, ToolResultReference{
			Kind:    ToolResultReferenceKindCard,
			CardRef: display.CardRef,
			Title:   display.Title,
		})
	}
	return out
}

func appendToolResultReferences(target []ToolResultReference, refs ...ToolResultReference) []ToolResultReference {
	if len(refs) == 0 {
		return target
	}
	seen := make(map[string]struct{}, len(target))
	for _, ref := range target {
		if key := toolResultReferenceIdentity(ref); key != "" {
			seen[key] = struct{}{}
		}
	}
	for _, ref := range refs {
		if key := toolResultReferenceIdentity(ref); key != "" {
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
		}
		target = append(target, ref)
	}
	return target
}

func toolResultReferenceIdentity(ref ToolResultReference) string {
	rangeKey := toolResultReferenceRangeIdentity(ref.Range)
	switch ref.Kind {
	case ToolResultReferenceKindBlob, ToolResultReferenceKindMCPResource:
		if ref.URI != "" {
			return string(ref.Kind) + ":" + ref.URI + rangeKey
		}
	case ToolResultReferenceKindCard:
		if ref.CardRef != "" {
			return string(ref.Kind) + ":" + ref.CardRef + rangeKey
		}
	case ToolResultReferenceKindFile:
		if ref.FilePath != "" {
			return string(ref.Kind) + ":" + ref.FilePath + rangeKey
		}
	}
	return ""
}

func toolResultReferenceRangeIdentity(rng resourceio.Range) string {
	if rng.Offset == 0 && rng.Limit == 0 && strings.TrimSpace(rng.Query) == "" && rng.Page == 0 && strings.TrimSpace(rng.Format) == "" {
		return ""
	}
	rng = resourceio.NormalizeRangeValue(rng, resourceio.DefaultMaxReadBytes)
	return fmt.Sprintf("|range=offset:%d,limit:%d,page:%d,query:%s,format:%s", rng.Offset, rng.Limit, rng.Page, rng.Query, rng.Format)
}

func externalReferencesFromToolResultRefs(session *SessionState, snapshot *TurnSnapshot, iteration int, refs []ToolResultReference) []ExternalReference {
	if len(refs) == 0 {
		return nil
	}
	createdAt := time.Now().UTC()
	out := make([]ExternalReference, 0, len(refs))
	for _, ref := range refs {
		key := toolResultReferenceIdentity(ref)
		if key == "" {
			continue
		}
		out = append(out, ExternalReference{
			ID:          "ref-" + digestContent(key)[:12],
			SessionID:   session.ID,
			TurnID:      snapshot.ID,
			Iteration:   iteration,
			Kind:        string(ref.Kind),
			URI:         ref.URI,
			CardRef:     ref.CardRef,
			FilePath:    ref.FilePath,
			Title:       ref.Title,
			Summary:     ref.Summary,
			ContentType: ref.ContentType,
			Digest:      ref.Digest,
			Bytes:       ref.Bytes,
			Version:     ref.Version,
			Range:       ref.Range,
			CreatedAt:   createdAt,
		})
	}
	return out
}

func toolResultReferenceFromExternalRef(ref ExternalReference) ToolResultReference {
	return ToolResultReference{
		Kind:        ToolResultReferenceKind(ref.Kind),
		URI:         ref.URI,
		CardRef:     ref.CardRef,
		FilePath:    ref.FilePath,
		Title:       ref.Title,
		Summary:     ref.Summary,
		ContentType: ref.ContentType,
		Digest:      ref.Digest,
		Bytes:       ref.Bytes,
		Version:     ref.Version,
		Range:       ref.Range,
	}
}

func externalReferenceLabel(ref ExternalReference) string {
	switch {
	case ref.CardRef != "":
		return ref.CardRef
	case ref.FilePath != "":
		return ref.FilePath
	case ref.URI != "":
		return ref.URI
	case ref.ID != "":
		return ref.ID
	default:
		return "external-reference"
	}
}

func defaultInlineResultBytesForContext(thresholds ContextBudgetThresholds) int {
	if thresholds.SmallContextMode {
		return 1024
	}
	return defaultMaxInlineResultBytes
}

func finalizeToolResultMaterialization(session *SessionState, snapshot *TurnSnapshot, iteration int, tc ToolCall, result *ToolResult, tier toolResultMaterializationTier, originalBytes int) {
	if result == nil {
		return
	}
	result.MaterializationTier = string(tier)
	result.OriginalBytes = int64(originalBytes)
	result.InlineBytes = int64(len(result.Content))
	if session == nil || snapshot == nil {
		return
	}
	event := BuildContextGovernanceEvent(ContextGovernanceEvent{
		ID:                  fmt.Sprintf("ctxgov-%s-%d-%s-l1", snapshot.ID, iteration, tc.ID),
		Layer:               ContextGovernanceLayerL1,
		Kind:                "tool_result.materialized",
		SessionID:           session.ID,
		TurnID:              snapshot.ID,
		Iteration:           iteration,
		ToolCallID:          tc.ID,
		ToolName:            firstNonBlankRuntimeString(tc.Name, result.ToolCallID),
		MaterializationTier: string(tier),
		OriginalBytes:       result.OriginalBytes,
		InlineBytes:         result.InlineBytes,
		Message:             "工具结果已按上下文预算整理",
		ReferenceIDs:        referenceIDsFromExternalReferences(result.ExternalReferences),
	})
	appendContextGovernanceEvents(&snapshot.ContextGovernanceEvents, event)
	appendContextGovernanceEvents(&session.ContextGovernanceEvents, event)
}

func latestToolResultGovernanceEvents(session *SessionState, toolCallID string) []ContextGovernanceEvent {
	if session == nil || strings.TrimSpace(toolCallID) == "" {
		return nil
	}
	var out []ContextGovernanceEvent
	for _, event := range session.ContextGovernanceEvents {
		if event.ToolCallID == toolCallID {
			out = append(out, event)
		}
	}
	return out
}

func appendContextGovernanceEvents(target *[]ContextGovernanceEvent, events ...ContextGovernanceEvent) {
	if target == nil || len(events) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(*target))
	for _, event := range *target {
		if event.ID != "" {
			seen[event.ID] = struct{}{}
		}
	}
	for _, event := range events {
		if event.Layer == "" || event.Kind == "" {
			continue
		}
		event = BuildContextGovernanceEvent(event)
		key := event.ID
		if key == "" {
			key = fmt.Sprintf("%s:%s:%s:%d:%s", event.Layer, event.Kind, event.TurnID, event.Iteration, event.ToolCallID)
			event.ID = "ctxgov-" + digestContent(key)[:12]
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		*target = append(*target, event)
	}
}

func appendExternalReferences(target *[]ExternalReference, refs ...ExternalReference) {
	if target == nil || len(refs) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(*target))
	for _, ref := range *target {
		if ref.ID != "" {
			seen[ref.ID] = struct{}{}
		}
	}
	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}
		if _, ok := seen[ref.ID]; ok {
			continue
		}
		seen[ref.ID] = struct{}{}
		*target = append(*target, ref)
	}
}

func appendCompactedSegments(target *[]CompactedSegment, segments ...CompactedSegment) {
	if target == nil || len(segments) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(*target))
	for _, segment := range *target {
		if segment.ID != "" {
			seen[segment.ID] = struct{}{}
		}
	}
	for _, segment := range segments {
		if segment.ID == "" {
			continue
		}
		if _, ok := seen[segment.ID]; ok {
			continue
		}
		seen[segment.ID] = struct{}{}
		*target = append(*target, segment)
	}
}

func appendCheckpointExternalRefs(checkpoint *CheckpointMetadata, refs []ExternalReference) {
	if checkpoint == nil || len(refs) == 0 {
		return
	}
	seen := make(map[string]struct{}, len(checkpoint.ExternalRefs))
	for _, id := range checkpoint.ExternalRefs {
		if id != "" {
			seen[id] = struct{}{}
		}
	}
	for _, ref := range refs {
		if ref.ID == "" {
			continue
		}
		if _, ok := seen[ref.ID]; ok {
			continue
		}
		seen[ref.ID] = struct{}{}
		checkpoint.ExternalRefs = append(checkpoint.ExternalRefs, ref.ID)
	}
}

func (k *RuntimeKernel) ensureCurrentTurnSnapshot(session *SessionState, req TurnRequest, turnID string) *TurnSnapshot {
	if session.CurrentTurn != nil && session.CurrentTurn.ID == turnID {
		if req.SpecialInputReadPlan != nil {
			session.CurrentTurn.SpecialInputReadPlan = req.SpecialInputReadPlan
		}
		syncActiveTurnState(session, session.CurrentTurn)
		return session.CurrentTurn
	}
	now := time.Now()
	snapshot := &TurnSnapshot{
		ID:                   turnID,
		ClientTurnID:         req.ClientTurnID,
		ClientMessageID:      req.ClientMessageID,
		SessionID:            session.ID,
		SessionType:          req.SessionType,
		Mode:                 req.Mode,
		Metadata:             cloneTurnMetadata(req.Metadata),
		SpecialInputReadPlan: req.SpecialInputReadPlan,
		Lifecycle:            TurnLifecycleRunning,
		ResumeState:          TurnResumeStateNone,
		StartedAt:            now,
		UpdatedAt:            now,
	}
	session.CurrentTurn = snapshot
	syncActiveTurnState(session, snapshot)
	return snapshot
}

func (k *RuntimeKernel) persistTurnSnapshot(session *SessionState, snapshot *TurnSnapshot) {
	if session == nil || snapshot == nil {
		return
	}
	syncPendingApprovalAgentItems(snapshot)
	session.CurrentTurn = snapshot
	syncActiveTurnState(session, snapshot)
	upsertTurnHistory(&session.TurnHistory, *snapshot)
	k.sessions.Update(session)
}

func runningRegularTurnForPendingInput(session *SessionState, requestedTurnID string) (*TurnSnapshot, bool) {
	if session == nil || session.CurrentTurn == nil {
		return nil, false
	}
	current := session.CurrentTurn
	if strings.TrimSpace(current.ID) == "" || strings.TrimSpace(current.ID) == strings.TrimSpace(requestedTurnID) {
		return nil, false
	}
	if current.Lifecycle != TurnLifecycleRunning || current.ResumeState != TurnResumeStateNone {
		return nil, false
	}
	return current, true
}

func appendPendingInputToActiveTurn(session *SessionState, turn *TurnSnapshot, req TurnRequest) PendingTurnInput {
	now := time.Now()
	id := strings.TrimSpace(req.ClientMessageID)
	if id == "" {
		id = fmt.Sprintf("pending-input-%d", now.UnixNano())
	}
	pending := PendingTurnInput{
		ID:              id,
		ClientTurnID:    strings.TrimSpace(req.ClientTurnID),
		ClientMessageID: strings.TrimSpace(req.ClientMessageID),
		Content:         strings.TrimSpace(req.Input),
		CreatedAt:       now,
	}
	turn.PendingInputs = append(turn.PendingInputs, pending)
	turn.UpdatedAt = now
	if session != nil {
		session.UpdatedAt = now
	}
	return pending
}

func syncActiveTurnState(session *SessionState, snapshot *TurnSnapshot) {
	if session == nil || snapshot == nil {
		return
	}
	session.ActiveTurn = ActiveTurnState{
		TurnID: strings.TrimSpace(snapshot.ID),
		Kind:   "regular",
		Status: string(snapshot.Lifecycle),
	}
}

func (k *RuntimeKernel) markTurnBlocked(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, result DispatchResult) error {
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionToolInvocationBlocked, TurnLifecycleSuspended); err != nil {
		return err
	}
	now := time.Now()
	reason := result.Reason
	command := approvalCommandForToolCall(tc)
	if result.Approval != nil {
		command = firstNonEmpty(result.Approval.Command, command)
		reason = firstNonEmpty(result.Approval.Reason, reason)
	}
	resumeState := TurnResumeStatePendingApproval
	if result.Outcome == "evidence_needed" || containsPhrase(reason, "evidence") {
		resumeState = TurnResumeStatePendingEvidence
	}
	kind := result.Outcome
	if kind == "" {
		kind = "suspended"
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), kind, TurnLifecycleSuspended, resumeState)
	if result.Source != "" {
		checkpoint.Source = result.Source
	}
	last := latestIteration(snapshot)
	commitBlockedState := func(state TurnResumeState, blockedReason string) {
		snapshot.Lifecycle = TurnLifecycleSuspended
		snapshot.ResumeState = state
		snapshot.Error = blockedReason
		snapshot.UpdatedAt = now
		snapshot.LatestCheckpoint = checkpoint
		session.LatestCheckpoint = checkpoint
		if last != nil {
			last.Lifecycle = TurnLifecycleSuspended
			last.ResumeState = state
			last.Checkpoint = checkpoint
			last.UpdatedAt = now
		}
	}
	recordBlockedBoundary := func(candidate *TurnSnapshot, approval *PendingApproval) error {
		if approval != nil {
			if err := k.recordCanonicalApprovalRequested(context.Background(), candidate, *approval); err != nil {
				return fmt.Errorf("record approval request: %w", err)
			}
		}
		if err := k.recordCanonicalCheckpoint(context.Background(), candidate, checkpoint); err != nil {
			return fmt.Errorf("record blocked checkpoint: %w", err)
		}
		if err := k.recordCanonicalTransportProjection(context.Background(), candidate, candidate.Lifecycle, candidate.ResumeState, checkpoint.ID, nil); err != nil {
			return fmt.Errorf("record blocked projection source: %w", err)
		}
		return nil
	}
	newBlockedCandidate := func(state TurnResumeState) TurnSnapshot {
		candidate := *snapshot
		candidate.Lifecycle = TurnLifecycleSuspended
		candidate.ResumeState = state
		candidate.LatestCheckpoint = checkpoint
		return candidate
	}

	if resumeState == TurnResumeStatePendingEvidence {
		evidence := PendingEvidence{
			ID:         fmt.Sprintf("evidence-%d", now.UnixNano()),
			SessionID:  session.ID,
			TurnID:     snapshot.ID,
			Iteration:  snapshot.Iteration,
			ToolName:   tc.Name,
			ToolCallID: tc.ID,
			Reason:     reason,
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
		}
		candidate := newBlockedCandidate(resumeState)
		candidate.PendingEvidence = []PendingEvidence{evidence}
		candidate.PendingApprovals = nil
		if err := recordBlockedBoundary(&candidate, nil); err != nil {
			snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
			return err
		}
		commitBlockedState(resumeState, reason)
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		snapshot.PendingEvidence = candidate.PendingEvidence
		snapshot.PendingApprovals = candidate.PendingApprovals
		session.PendingEvidence = []PendingEvidence{evidence}
		session.PendingApprovals = nil
	} else {
		resourceScopes := pendingApprovalResourceScopes(result.Metadata)
		argumentsHash := firstNonEmpty(result.DecisionTrace.ArgumentsHash, toolArgumentsHash(tc.Arguments))
		targetRefs := pendingApprovalTargetRefs(session.HostID, resourceScopes)
		iterationID := ""
		if last != nil {
			iterationID = last.ID
		}
		approval := PendingApproval{
			ID:            pendingApprovalID(session.ID, snapshot.ID, snapshot.Iteration, tc, argumentsHash, targetRefs),
			SessionID:     session.ID,
			TurnID:        snapshot.ID,
			Iteration:     snapshot.Iteration,
			IterationID:   iterationID,
			ToolName:      tc.Name,
			ToolCallID:    tc.ID,
			TargetRefs:    targetRefs,
			HostID:        session.HostID,
			Command:       command,
			ArgumentsHash: argumentsHash,
			Reason:        reason,
			AllowedActions: []string{
				strings.TrimSpace(tc.Name),
			},
			ResourceScopes:         resourceScopes,
			RiskCeiling:            firstNonEmpty(approvalPayloadField(result.Approval, "risk"), string(result.Metadata.EffectiveGovernance(0).RiskLevel)),
			RequestedScope:         strings.Join(targetRefs, ","),
			ApprovalScope:          strings.Join(targetRefs, ","),
			ApprovalOptions:        []string{"approved", "denied"},
			ToolSurfaceFingerprint: result.DecisionTrace.ToolSurfaceFingerprint,
			PermissionSnapshotHash: result.DecisionTrace.PermissionSnapshotHash,
			ExpiresAt:              pendingApprovalDefaultExpiry(now),
			InputHash:              argumentsHash,
			PreChangeEvidenceRefs:  assistantMessageEvidenceRefsFromSnapshot(snapshot),
			PostCheck:              strings.Join(normalizedPostCheckRefs(result.Metadata), "; "),
			StopCondition:          "stop if validation fails or observed state diverges",
			IdempotencyKey:         argumentsHash,
			Mutating:               dispatchResultRequiresRollbackContract(result),
			Status:                 "pending",
			CreatedAt:              now,
			UpdatedAt:              now,
		}
		if result.Approval != nil {
			approval.Risk = result.Approval.Risk
			approval.Source = result.Approval.Source
			approval.RunbookID = result.Approval.RunbookID
			approval.RunbookStep = result.Approval.RunbookStep
			approval.ExpectedEffect = result.Approval.ExpectedEffect
			approval.Rollback = result.Approval.Rollback
			approval.Validation = result.Approval.Validation
		}
		approval.RollbackContract = BuildActionRollbackContractFromApproval(approval)
		if err := ValidatePendingApprovalRollbackContract(approval); err != nil {
			reason := err.Error()
			evidence := PendingEvidence{
				ID:         fmt.Sprintf("evidence-%d", now.UnixNano()),
				SessionID:  session.ID,
				TurnID:     snapshot.ID,
				Iteration:  snapshot.Iteration,
				ToolName:   tc.Name,
				ToolCallID: tc.ID,
				Reason:     reason,
				Status:     "pending",
				CreatedAt:  now,
				UpdatedAt:  now,
			}
			if checkpoint != nil {
				checkpoint.Kind = "rollback_contract_invalid"
				checkpoint.ResumeState = TurnResumeStatePendingEvidence
			}
			candidate := newBlockedCandidate(TurnResumeStatePendingEvidence)
			candidate.PendingEvidence = []PendingEvidence{evidence}
			candidate.PendingApprovals = nil
			if recordErr := recordBlockedBoundary(&candidate, nil); recordErr != nil {
				snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
				return recordErr
			}
			commitBlockedState(TurnResumeStatePendingEvidence, reason)
			snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
			snapshot.PendingEvidence = candidate.PendingEvidence
			snapshot.PendingApprovals = candidate.PendingApprovals
			session.PendingEvidence = []PendingEvidence{evidence}
			session.PendingApprovals = nil
			appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
			k.persistTurnSnapshot(session, snapshot)
			return nil
		}
		actionToken, tokenErr := BuildPendingApprovalActionToken(approval, checkpoint.ID)
		if tokenErr != nil {
			return fmt.Errorf("freeze approval action token: %w", tokenErr)
		}
		approval.ActionToken = &actionToken
		candidate := newBlockedCandidate(resumeState)
		candidate.PendingApprovals = []PendingApproval{approval}
		candidate.PendingEvidence = nil
		if err := recordBlockedBoundary(&candidate, &approval); err != nil {
			snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
			return err
		}
		commitBlockedState(resumeState, reason)
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		snapshot.PendingApprovals = candidate.PendingApprovals
		snapshot.PendingEvidence = candidate.PendingEvidence
		session.PendingApprovals = []PendingApproval{approval}
		session.PendingEvidence = nil
		appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteApprovalLedger, OwnerPendingApproval)
	}

	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	k.persistTurnSnapshot(session, snapshot)
	if k.projector != nil {
		payload, _ := json.Marshal(map[string]any{
			"id":             currentBlockedID(snapshot),
			"toolName":       tc.Name,
			"command":        command,
			"reason":         reason,
			"risk":           approvalPayloadField(result.Approval, "risk"),
			"source":         approvalPayloadField(result.Approval, "source"),
			"runbookId":      approvalPayloadField(result.Approval, "runbookId"),
			"runbookStep":    approvalPayloadField(result.Approval, "runbookStep"),
			"expectedEffect": approvalPayloadField(result.Approval, "expectedEffect"),
			"rollback":       approvalPayloadField(result.Approval, "rollback"),
			"validation":     approvalPayloadField(result.Approval, "validation"),
			"status":         "pending",
		})
		eventType := EventApprovalNeeded
		if resumeState == TurnResumeStatePendingEvidence {
			eventType = EventEvidenceCollected
		}
		k.projector.Emit(LifecycleEvent{
			Type:      eventType,
			SessionID: session.ID,
			TurnID:    snapshot.ID,
			Timestamp: now,
			Payload:   payload,
		})
	}
	return nil
}

func (k *RuntimeKernel) markTurnFailed(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, result DispatchResult) error {
	if session == nil || snapshot == nil {
		return fmt.Errorf("session and snapshot are required")
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnFailed, TurnLifecycleFailed); err != nil {
		return err
	}
	now := time.Now()
	checkpointKind := result.Outcome
	if checkpointKind == "" {
		checkpointKind = "tool_failed"
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), checkpointKind, TurnLifecycleFailed, TurnResumeStateNone)
	if result.Source != "" {
		checkpoint.Source = result.Source
	}
	candidate := *snapshot
	candidate.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
	candidate.Lifecycle = TurnLifecycleFailed
	candidate.ResumeState = TurnResumeStateNone
	candidate.PendingApprovals = nil
	candidate.PendingEvidence = nil
	candidate.LatestCheckpoint = checkpoint
	if err := k.recordCanonicalTerminalBoundary(context.Background(), &candidate, checkpoint, FinalContractStatusFailed, checkpointKind); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return err
	}
	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleFailed
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = result.Error
	snapshot.UpdatedAt = now
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleFailed
		last.ResumeState = TurnResumeStateNone
		last.Checkpoint = checkpoint
		last.UpdatedAt = now
	}
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	k.persistTurnSnapshot(session, snapshot)
	return nil
}

func (k *RuntimeKernel) markTurnFailedFromError(session *SessionState, snapshot *TurnSnapshot, err error, checkpointKind string) error {
	if session == nil || snapshot == nil || snapshot.Lifecycle.IsTerminal() {
		return nil
	}
	if transitionErr := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnFailed, TurnLifecycleFailed); transitionErr != nil {
		return transitionErr
	}
	now := time.Now()
	errText := ""
	if err != nil {
		errText = err.Error()
	}
	checkpointKind = strings.TrimSpace(checkpointKind)
	if checkpointKind == "" {
		checkpointKind = "turn_failed"
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), checkpointKind, TurnLifecycleFailed, TurnResumeStateNone)
	candidate := *snapshot
	candidate.AgentItems = append([]agentstate.TurnItem(nil), snapshot.AgentItems...)
	candidate.Lifecycle = TurnLifecycleFailed
	candidate.ResumeState = TurnResumeStateNone
	candidate.PendingApprovals = nil
	candidate.PendingEvidence = nil
	candidate.LatestCheckpoint = checkpoint
	if recordErr := k.recordCanonicalTerminalBoundary(context.Background(), &candidate, checkpoint, FinalContractStatusFailed, checkpointKind); recordErr != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return recordErr
	}
	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleFailed
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = errText
	snapshot.UpdatedAt = now
	snapshot.CompletedAt = &now
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleFailed
		last.ResumeState = TurnResumeStateNone
		last.Checkpoint = checkpoint
		last.UpdatedAt = now
		last.CompletedAt = &now
	}
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	k.persistTurnSnapshot(session, snapshot)
	if k.projector != nil {
		payload, _ := json.Marshal(map[string]any{
			"error":          errText,
			"checkpointKind": checkpointKind,
		})
		k.projector.Emit(LifecycleEvent{
			Type:      EventTurnError,
			SessionID: session.ID,
			TurnID:    snapshot.ID,
			Timestamp: now,
			Payload:   payload,
		})
	}
	return nil
}

func isRecoverableModelTimeout(err error) bool {
	return err != nil && errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled)
}

func (k *RuntimeKernel) markTurnResumableFromModelTimeout(session *SessionState, snapshot *TurnSnapshot, iteration int, err error) (TurnResult, error) {
	if session == nil || snapshot == nil {
		return TurnResult{}, fmt.Errorf("session and snapshot are required")
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionToolInvocationBlocked, TurnLifecycleResumable); err != nil {
		return TurnResult{}, err
	}
	now := time.Now()
	errText := "model response timeout"
	if err != nil {
		errText = err.Error()
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, iteration, nextCheckpointSequence(snapshot), "model_timeout", TurnLifecycleResumable, TurnResumeStateResumable)
	checkpoint.Incremental = false
	candidate := *snapshot
	candidate.Lifecycle = TurnLifecycleResumable
	candidate.ResumeState = TurnResumeStateResumable
	candidate.LatestCheckpoint = checkpoint
	if err := k.recordCanonicalCheckpoint(context.Background(), &candidate, checkpoint); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return TurnResult{}, err
	}
	if err := k.recordCanonicalTransportProjection(context.Background(), &candidate, candidate.Lifecycle, candidate.ResumeState, checkpoint.ID, nil); err != nil {
		snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
		return TurnResult{}, err
	}
	snapshot.CanonicalRolloutHead = candidate.CanonicalRolloutHead
	snapshot.Lifecycle = TurnLifecycleResumable
	snapshot.ResumeState = TurnResumeStateResumable
	snapshot.Error = errText
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]string{}
	}
	snapshot.Metadata["recovery.reason"] = "model_timeout"
	snapshot.Metadata["recovery.recoverable"] = "true"
	snapshot.UpdatedAt = now
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleResumable
		last.ResumeState = TurnResumeStateResumable
		last.Checkpoint = checkpoint
		last.UpdatedAt = now
	}
	appendAcceptedOwnerWriteTrace(session, snapshot, OwnerWriteTurnLifecycle, OwnerRuntimeKernel)
	k.persistTurnSnapshot(session, snapshot)
	return TurnResult{
		SessionType:     snapshot.SessionType,
		Mode:            snapshot.Mode,
		SessionID:       session.ID,
		TurnID:          snapshot.ID,
		ClientTurnID:    snapshot.ClientTurnID,
		ClientMessageID: snapshot.ClientMessageID,
		Status:          "blocked",
		Error:           errText,
	}, nil
}

func inferTurnFailureCheckpointKind(snapshot *TurnSnapshot) string {
	if snapshot == nil {
		return "turn_failed"
	}
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		if item.Status != agentstate.ItemStatusFailed {
			continue
		}
		switch item.Type {
		case agentstate.TurnItemTypeModelCall:
			return "model_call_failed"
		case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
			return "tool_failed"
		}
	}
	return "turn_failed"
}

func currentBlockedID(snapshot *TurnSnapshot) string {
	if snapshot == nil {
		return ""
	}
	if len(snapshot.PendingApprovals) > 0 {
		return snapshot.PendingApprovals[0].ID
	}
	if len(snapshot.PendingEvidence) > 0 {
		return snapshot.PendingEvidence[0].ID
	}
	return ""
}

func (k *RuntimeKernel) emitIterationStage(sessionID, turnID string, iteration int, stage string, turnSpanID string) {
	if k.projector == nil && (k.spanSource == nil || turnSpanID == "") {
		return
	}
	now := time.Now()
	payload, _ := json.Marshal(map[string]any{
		"iteration": iteration,
		"stage":     stage,
	})
	if k.projector != nil {
		k.projector.Emit(LifecycleEvent{
			Type:      EventActivityUpdate,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: now,
			Payload:   payload,
		})
	}
	if k.spanSource != nil && turnSpanID != "" {
		k.spanSource.EmitText(fmt.Sprintf("[iteration %d] %s", iteration, stage))
	}
}

func (k *RuntimeKernel) emitRuntimeEvent(eventType EventType, sessionID, turnID string, payload any) {
	if k == nil || k.projector == nil {
		return
	}
	data, _ := json.Marshal(payload)
	k.projector.Emit(LifecycleEvent{
		Type:      eventType,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   data,
	})
}

func latestIteration(snapshot *TurnSnapshot) *IterationState {
	if snapshot == nil || len(snapshot.Iterations) == 0 {
		return nil
	}
	return &snapshot.Iterations[len(snapshot.Iterations)-1]
}

func (k *RuntimeKernel) progressSink(session *SessionState, snapshot *TurnSnapshot, iteration int) ToolProgressSink {
	if session == nil || snapshot == nil {
		return nil
	}
	return func(update ToolProgressUpdate) {
		now := update.Timestamp
		if now.IsZero() {
			now = time.Now()
			update.Timestamp = now
		}
		last := latestIteration(snapshot)
		if last == nil || last.Iteration != iteration {
			return
		}
		last.ToolProgress = append(last.ToolProgress, update)
		last.UpdatedAt = now
		k.upsertPartialToolProgressMessage(session, snapshot, iteration, update, now)
		sequence := nextCheckpointSequence(snapshot)
		checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, iteration, sequence, "tool_progress", TurnLifecycleRunning, snapshot.ResumeState)
		checkpoint.Incremental = true
		checkpoint.UpdatedAt = now
		last.Checkpoint = checkpoint
		snapshot.LatestCheckpoint = checkpoint
		snapshot.UpdatedAt = now
		session.LatestCheckpoint = checkpoint
		k.persistTurnSnapshot(session, snapshot)
	}
}

func (k *RuntimeKernel) upsertPartialToolProgressMessage(session *SessionState, snapshot *TurnSnapshot, iteration int, update ToolProgressUpdate, now time.Time) {
	if session == nil || snapshot == nil {
		return
	}
	if strings.TrimSpace(update.Delta) == "" && !update.Done {
		return
	}

	messageID := partialToolProgressMessageID(snapshot.ID, iteration, update.ToolCallID)
	for i := range session.Messages {
		if session.Messages[i].ID != messageID {
			continue
		}
		if session.Messages[i].Metadata == nil {
			session.Messages[i].Metadata = map[string]string{}
		}
		session.Messages[i].Metadata["runtime.context.kind"] = promptinput.ContextKindToolProgress
		session.Messages[i].Metadata["runtime.context.ref"] = update.ToolCallID
		if delta := strings.TrimSpace(update.Delta); delta != "" {
			raw := partialToolProgressBody(session.Messages[i].Content)
			session.Messages[i].Content = partialToolProgressContent(update.ToolName, raw+update.Delta)
		}
		session.Messages[i].Timestamp = now
		recomputeContextWindow(&session.Context, session.Messages)
		return
	}

	content := partialToolProgressContent(update.ToolName, update.Delta)
	session.Messages = append(session.Messages, Message{
		ID:        messageID,
		Role:      "system",
		Content:   content,
		Timestamp: now,
		Metadata: map[string]string{
			"runtime.context.kind": promptinput.ContextKindToolProgress,
			"runtime.context.ref":  update.ToolCallID,
		},
	})
	recomputeContextWindow(&session.Context, session.Messages)
}

func partialToolProgressMessageID(turnID string, iteration int, toolCallID string) string {
	return fmt.Sprintf("partial-%s-%d-%s", turnID, iteration, toolCallID)
}

func partialToolProgressContent(toolName, body string) string {
	body = truncateForBudget(strings.TrimSpace(body), 2048)
	if toolName == "" {
		toolName = "tool"
	}
	return fmt.Sprintf("Partial tool result [%s]: %s", toolName, body)
}

func partialToolProgressBody(content string) string {
	content = strings.TrimSpace(content)
	if idx := strings.Index(content, "]: "); idx >= 0 {
		return content[idx+3:]
	}
	return content
}

func newCheckpointMetadata(sessionID, turnID string, iteration, sequence int, kind string, lifecycle TurnLifecycleState, resumeState TurnResumeState) *CheckpointMetadata {
	now := time.Now()
	digest := strings.TrimPrefix(agentassembly.StableHash("runtime-checkpoint-id", map[string]any{
		"sessionId": sessionID, "turnId": turnID, "iteration": iteration, "sequence": sequence,
		"kind": kind, "lifecycle": lifecycle, "resumeState": resumeState,
	}), "sha256:")
	if len(digest) > 24 {
		digest = digest[:24]
	}
	return &CheckpointMetadata{
		ID:          "chk-" + digest,
		SessionID:   sessionID,
		TurnID:      turnID,
		Iteration:   iteration,
		Sequence:    sequence,
		Kind:        kind,
		Source:      "runtimekernel",
		Lifecycle:   lifecycle,
		ResumeState: resumeState,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func pendingApprovalID(sessionID, turnID string, iteration int, call ToolCall, argumentsHash string, targetRefs []string) string {
	digest := strings.TrimPrefix(agentassembly.StableHash("runtime-pending-approval-id", map[string]any{
		"sessionId": sessionID, "turnId": turnID, "iteration": iteration,
		"toolCallId": call.ID, "toolName": call.Name, "argumentsHash": argumentsHash,
		"targetRefs": uniqueSortedTraceStrings(targetRefs),
	}), "sha256:")
	if len(digest) > 24 {
		digest = digest[:24]
	}
	return "approval-" + digest
}

func nextCheckpointSequence(snapshot *TurnSnapshot) int {
	if snapshot == nil || snapshot.LatestCheckpoint == nil {
		return 1
	}
	return snapshot.LatestCheckpoint.Sequence + 1
}

func toolNames(tools []promptcompiler.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		if name := toolDef.Metadata().Name; name != "" {
			out = append(out, name)
		}
	}
	return out
}

func promptContentHash(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return hex.EncodeToString(sum[:])
}

func assembledToolFingerprint(source ToolAssemblySource, session SessionType, mode Mode, tools []promptcompiler.Tool) string {
	if refresher, ok := source.(toolRefreshAwareSource); ok {
		if token := strings.TrimSpace(refresher.RefreshToken(session, mode)); token != "" {
			return token
		}
	}
	return promptContentHash(strings.Join(toolNames(tools), "\n"))
}

func refreshedToolNames(snapshot *TurnSnapshot, currentFingerprint string, tools []promptcompiler.Tool) []string {
	names := toolNames(tools)
	if snapshot == nil || snapshot.StableToolFingerprint == "" || snapshot.StableToolFingerprint != currentFingerprint {
		return names
	}
	return nil
}

func iterationToolDelta(snapshot *TurnSnapshot, tools []promptcompiler.Tool) promptcompiler.ToolPromptDelta {
	current := toolNames(tools)
	delta := promptcompiler.ToolPromptDelta{
		ApprovalRequired: destructiveToolNames(tools),
	}
	if snapshot == nil || len(snapshot.Iterations) == 0 {
		if snapshot != nil {
			delta.TemporarilyUnavailable = append([]string(nil), snapshot.HiddenTools...)
		}
		return delta
	}

	previous := latestIteration(snapshot)
	if previous == nil {
		return delta
	}
	delta.NewlyAvailable = diffStrings(current, previous.VisibleTools)
	delta.NewlyAvailablePacks = newlyAvailablePacks(delta.NewlyAvailable, tools)
	delta.TemporarilyUnavailable = mergeStringSets(diffStrings(previous.VisibleTools, current), snapshot.HiddenTools)
	return delta
}

func newlyAvailablePacks(newToolNames []string, tools []promptcompiler.Tool) []string {
	if len(newToolNames) == 0 {
		return nil
	}
	newNames := make(map[string]bool, len(newToolNames))
	for _, name := range newToolNames {
		newNames[name] = true
	}
	packs := make([]string, 0)
	seen := map[string]bool{}
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if !newNames[meta.Name] || meta.Pack == "" || seen[meta.Pack] {
			continue
		}
		seen[meta.Pack] = true
		packs = append(packs, meta.Pack)
	}
	sort.Strings(packs)
	return packs
}

func buildProtocolPromptState(snapshot *TurnSnapshot, delta promptcompiler.ToolPromptDelta, approvals []PendingApproval, evidence []PendingEvidence, rejectedApprovals []RejectedApproval) promptcompiler.ProtocolPromptState {
	var items []promptcompiler.ProtocolPromptItem
	for _, name := range delta.NewlyAvailable {
		items = append(items, promptcompiler.ProtocolPromptItem{
			Kind:   "tool_delta",
			ID:     name,
			Status: "newly_available",
			Text:   "Tool became available in this model-call iteration.",
		})
	}
	for _, name := range delta.TemporarilyUnavailable {
		items = append(items, promptcompiler.ProtocolPromptItem{
			Kind:   "tool_delta",
			ID:     name,
			Status: "temporarily_unavailable",
			Text:   "Tool is hidden or unavailable for this model-call iteration.",
		})
	}
	items = append(items, planProtocolPromptItems(snapshot)...)
	for _, approval := range approvals {
		text := strings.TrimSpace(approval.Reason)
		if text == "" {
			text = fmt.Sprintf("%s requires approval before execution.", approval.ToolName)
		}
		items = append(items, promptcompiler.ProtocolPromptItem{
			Kind:   "approval",
			ID:     approval.ID,
			Status: firstNonEmpty(approval.Status, "pending"),
			Text:   text,
		})
	}
	for _, pending := range evidence {
		text := strings.TrimSpace(pending.Reason)
		if text == "" {
			text = fmt.Sprintf("%s requires additional evidence before final answer.", pending.ToolName)
		}
		items = append(items, promptcompiler.ProtocolPromptItem{
			Kind:   "evidence",
			ID:     pending.ID,
			Status: firstNonEmpty(pending.Status, "pending"),
			Text:   text,
		})
	}
	for _, rejected := range rejectedApprovals {
		text := strings.TrimSpace(rejected.Reason)
		if text == "" {
			text = fmt.Sprintf("%s approval was rejected.", rejected.ToolName)
		}
		items = append(items, promptcompiler.ProtocolPromptItem{
			Kind:   "approval",
			ID:     firstNonEmpty(rejected.ID, rejected.InputHash, rejected.ToolCallID),
			Status: "denied",
			Text:   text,
		})
	}
	return promptcompiler.ProtocolPromptState{Items: items}
}

func planProtocolPromptItems(snapshot *TurnSnapshot) []promptcompiler.ProtocolPromptItem {
	if snapshot == nil {
		return nil
	}
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		if item.Type != agentstate.TurnItemTypePlan {
			continue
		}
		var plan planning.PlanState
		if err := json.Unmarshal(item.Payload.Data, &plan); err != nil || len(plan.Steps) == 0 {
			return nil
		}
		out := make([]promptcompiler.ProtocolPromptItem, 0, len(plan.Steps))
		for stepIdx, step := range plan.Steps {
			id := strings.TrimSpace(step.ID)
			if id == "" {
				id = fmt.Sprintf("step-%d", stepIdx+1)
			}
			text := strings.TrimSpace(step.Text)
			if summary := strings.TrimSpace(step.Summary); summary != "" {
				text = text + " - " + summary
			}
			out = append(out, promptcompiler.ProtocolPromptItem{
				Kind:   "plan",
				ID:     id,
				Status: string(step.Status),
				Text:   text,
			})
		}
		return out
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func pendingApprovalDefaultExpiry(now time.Time) *time.Time {
	expires := now.Add(15 * time.Minute)
	return &expires
}

func pendingApprovalResourceScopes(meta tooling.ToolMetadata) []string {
	scopes := make([]string, 0, len(meta.Discovery.ResourceTypes)+len(meta.ResourceLocks))
	for _, resourceType := range meta.Discovery.ResourceTypes {
		if strings.TrimSpace(resourceType) != "" {
			scopes = append(scopes, "type="+strings.TrimSpace(resourceType))
		}
	}
	for _, lock := range meta.ResourceLocks {
		parts := []string{}
		if strings.TrimSpace(lock.ResourceType) != "" {
			parts = append(parts, "type="+strings.TrimSpace(lock.ResourceType))
		}
		if strings.TrimSpace(lock.ResourceID) != "" {
			parts = append(parts, "id="+strings.TrimSpace(lock.ResourceID))
		}
		if strings.TrimSpace(lock.OperationKind) != "" {
			parts = append(parts, "op="+strings.TrimSpace(lock.OperationKind))
		}
		if len(parts) > 0 {
			scopes = append(scopes, strings.Join(parts, " "))
		}
	}
	sort.Strings(scopes)
	return uniqueRuntimeStrings(scopes)
}

func pendingApprovalTargetRefs(hostID string, resourceScopes []string) []string {
	refs := make([]string, 0, 1+len(resourceScopes))
	if hostID = strings.TrimSpace(hostID); hostID != "" {
		refs = append(refs, "host:"+hostID)
	}
	refs = append(refs, resourceScopes...)
	return uniqueRuntimeStrings(refs)
}

func dispatchResultRequiresRollbackContract(result DispatchResult) bool {
	meta := result.Metadata
	governance := meta.EffectiveGovernance(0)
	return governance.Mutating || meta.Layer == tooling.ToolLayerMutation
}

func approvalPayloadField(payload *tooling.PermissionApprovalPayload, field string) string {
	if payload == nil {
		return ""
	}
	switch field {
	case "risk":
		return payload.Risk
	case "source":
		return payload.Source
	case "runbookId":
		return payload.RunbookID
	case "runbookStep":
		return payload.RunbookStep
	case "expectedEffect":
		return payload.ExpectedEffect
	case "rollback":
		return payload.Rollback
	case "validation":
		return payload.Validation
	default:
		return ""
	}
}

func filterHiddenTools(tools []promptcompiler.Tool, hidden []string) []promptcompiler.Tool {
	if len(tools) == 0 || len(hidden) == 0 {
		return tools
	}
	hiddenSet := make(map[string]struct{}, len(hidden))
	for _, name := range hidden {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		hiddenSet[name] = struct{}{}
	}
	if len(hiddenSet) == 0 {
		return tools
	}
	filtered := make([]promptcompiler.Tool, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		if _, ok := hiddenSet[toolDef.Metadata().Name]; ok {
			continue
		}
		filtered = append(filtered, toolDef)
	}
	return filtered
}

func applyHiddenTools(snapshot *TurnSnapshot, hidden []string) {
	if snapshot == nil || len(hidden) == 0 {
		return
	}
	snapshot.HiddenTools = mergeStringSets(snapshot.HiddenTools, hidden)
}

func mergeStringSets(left, right []string) []string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(left)+len(right))
	out := make([]string, 0, len(left)+len(right))
	for _, items := range [][]string{left, right} {
		for _, item := range items {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if _, ok := seen[item]; ok {
				continue
			}
			seen[item] = struct{}{}
			out = append(out, item)
		}
	}
	return out
}

func destructiveToolNames(tools []promptcompiler.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef == nil || !toolDef.IsDestructive(nil) {
			continue
		}
		if name := toolDef.Metadata().Name; name != "" {
			names = append(names, name)
		}
	}
	return names
}

func diffStrings(left, right []string) []string {
	if len(left) == 0 {
		return nil
	}
	rightSet := make(map[string]struct{}, len(right))
	for _, item := range right {
		rightSet[item] = struct{}{}
	}
	out := make([]string, 0, len(left))
	for _, item := range left {
		if _, ok := rightSet[item]; ok {
			continue
		}
		out = append(out, item)
	}
	return out
}

func compileEvidenceReminders(mode Mode, pending []PendingEvidence) []string {
	out := make([]string, 0, len(pending)+1)
	if mode == ModeExecute {
		out = append(out, "Capture before/after evidence for every approved mutation.")
	}
	for _, item := range pending {
		reason := strings.TrimSpace(item.Reason)
		if reason == "" {
			if item.ToolName == "" {
				continue
			}
			reason = fmt.Sprintf("Outstanding evidence required for %s.", item.ToolName)
		}
		out = append(out, reason)
	}
	return out
}

func pendingToolCall(snapshot *TurnSnapshot) (ToolCall, bool) {
	if snapshot == nil || len(snapshot.Iterations) == 0 {
		return ToolCall{}, false
	}
	targetID := ""
	if len(snapshot.PendingApprovals) > 0 {
		targetID = snapshot.PendingApprovals[0].ToolCallID
	}
	if targetID == "" && len(snapshot.PendingEvidence) > 0 {
		targetID = snapshot.PendingEvidence[0].ToolCallID
	}
	for i := len(snapshot.Iterations) - 1; i >= 0; i-- {
		iter := snapshot.Iterations[i]
		for _, tc := range iter.ToolCalls {
			if targetID == "" || tc.ID == targetID {
				return tc, true
			}
		}
	}
	return ToolCall{}, false
}

func gatedPendingToolCall(snapshot *TurnSnapshot) (ToolCall, bool) {
	if snapshot == nil || (len(snapshot.PendingApprovals) == 0 && len(snapshot.PendingEvidence) == 0) {
		return ToolCall{}, false
	}
	return pendingToolCall(snapshot)
}

func resumeInputFromMetadata(metadata map[string]string) string {
	if len(metadata) == 0 {
		return ""
	}
	if input := strings.TrimSpace(metadata["resume.input"]); input != "" {
		return input
	}
	type indexedAnswer struct {
		index int
		value string
	}
	answers := make([]indexedAnswer, 0)
	for key, value := range metadata {
		if !strings.HasPrefix(key, "choice.answer.") {
			continue
		}
		indexText := strings.TrimPrefix(key, "choice.answer.")
		var index int
		if _, err := fmt.Sscanf(indexText, "%d", &index); err != nil {
			continue
		}
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		answers = append(answers, indexedAnswer{index: index, value: trimmed})
	}
	if len(answers) == 0 {
		return ""
	}
	sort.SliceStable(answers, func(i, j int) bool {
		return answers[i].index < answers[j].index
	})
	lines := make([]string, 0, len(answers)+1)
	if requestID := strings.TrimSpace(metadata["choice.requestId"]); requestID != "" {
		lines = append(lines, fmt.Sprintf("choice request %s", requestID))
	}
	for idx, answer := range answers {
		lines = append(lines, fmt.Sprintf("answer %d: %s", idx+1, answer.value))
	}
	return strings.Join(lines, "\n")
}

func upsertTurnHistory(history *[]TurnSnapshot, snapshot TurnSnapshot) {
	if history == nil {
		return
	}
	for i := range *history {
		if (*history)[i].ID == snapshot.ID {
			(*history)[i] = snapshot
			return
		}
	}
	*history = append(*history, snapshot)
}

func containsPhrase(value, needle string) bool {
	return strings.Contains(strings.ToLower(value), strings.ToLower(needle))
}

type assembledToolLookup struct {
	byName map[string]tooling.Tool
}

func (l assembledToolLookup) LookupTool(name string) (ToolDescriptor, ToolExecutor, bool) {
	toolDef, ok := l.byName[name]
	if !ok {
		return ToolDescriptor{}, nil, false
	}
	return ToolDescriptor{Metadata: toolDef.Metadata(), InputSchema: toolDef.InputSchema()}, toolExecutorAdapter{tool: toolDef}, true
}

type toolExecutorAdapter struct {
	tool tooling.Tool
}

func (a toolExecutorAdapter) CheckPermissions(ctx context.Context, args json.RawMessage) tooling.PermissionDecision {
	return a.tool.CheckPermissions(ctx, args)
}

func (a toolExecutorAdapter) Execute(ctx context.Context, args json.RawMessage) (tooling.ToolResult, error) {
	result, err := a.tool.Execute(ctx, args)
	if err != nil {
		return tooling.ToolResult{}, err
	}
	if result.Error != "" {
		return tooling.ToolResult{}, errors.New(result.Error)
	}
	return result, nil
}
