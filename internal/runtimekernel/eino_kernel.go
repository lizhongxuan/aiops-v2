package runtimekernel

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/envcontext"
	evidencecore "aiops-v2/internal/evidence"
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourceio"
	runtimestate "aiops-v2/internal/runtimekernel/state"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/taskdepth"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// ToolAssemblySource is the interface that the EinoKernel uses to access
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
// AgentManagerSource is the interface that the EinoKernel uses to access
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

// AgentManagerSource provides agent lifecycle management for the EinoKernel.
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
// EinoKernel — the Eino ADK-based RuntimeKernel implementation.
// ---------------------------------------------------------------------------

// EinoKernel implements the RuntimeKernel interface using Eino ADK.
// It is the unique turn runtime kernel that manages Host and Workspace sessions.
type EinoKernel struct {
	tools            ToolAssemblySource
	compiler         promptcompiler.Compiler
	policy           *policyengine.Engine
	permissions      *permissions.Engine
	hooks            *hooks.Registry
	projector        EventEmitter
	modelRouter      *modelrouter.Router
	sessions         *SessionManager
	agentMgr         AgentManagerSource
	spanSource       SpanStreamSource // optional: span tree integration for conversation tracking
	observer         Observer
	resourceLockGate ToolResourceLockGate
	compressor       *spanstream.ContextCompressor
	spillRepo        ToolResultSpillRepository
	artifactRepo     ContextArtifactRepository
	skillRegistry    *skills.Registry
	evidenceService  *evidencecore.Service

	turnCancelMu       sync.Mutex
	inFlightTurnCancel map[string]context.CancelFunc
	pendingTurnCancel  map[string]string
}

// EinoKernelConfig holds the dependencies for creating an EinoKernel.
type EinoKernelConfig struct {
	ToolSource       ToolAssemblySource
	Compiler         promptcompiler.Compiler
	Policy           *policyengine.Engine
	Permissions      *permissions.Engine
	Hooks            *hooks.Registry
	Projector        EventEmitter
	ModelRouter      *modelrouter.Router
	AgentMgr         AgentManagerSource
	Sessions         *SessionManager
	SessionRepo      SessionRepository
	SpanSource       SpanStreamSource // optional: if nil, span tracking is disabled
	Observer         Observer
	ResourceLockGate ToolResourceLockGate
	Compressor       *spanstream.ContextCompressor
	SpillRepo        ToolResultSpillRepository
	ArtifactRepo     ContextArtifactRepository
	SkillRegistry    *skills.Registry
	EvidenceService  *evidencecore.Service
}

// NewEinoKernel creates a new EinoKernel with the given dependencies.
func NewEinoKernel(cfg EinoKernelConfig) *EinoKernel {
	sessions := cfg.Sessions
	if sessions == nil {
		sessions = NewSessionManager(cfg.SessionRepo)
	}
	observer := cfg.Observer
	if observer == nil {
		observer = NoopObserver{}
	}
	return &EinoKernel{
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
		inFlightTurnCancel: make(map[string]context.CancelFunc),
		pendingTurnCancel:  make(map[string]string),
	}
}

func (k *EinoKernel) runtimeObserver() Observer {
	if k == nil || k.observer == nil {
		return NoopObserver{}
	}
	return k.observer
}

func turnExecutionKey(sessionID, turnID string) string {
	return strings.TrimSpace(sessionID) + ":" + strings.TrimSpace(turnID)
}

func (k *EinoKernel) registerTurnExecution(sessionID, turnID string, cancel context.CancelFunc) string {
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

func (k *EinoKernel) releaseTurnExecution(sessionID, turnID string, cancel context.CancelFunc) {
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

func (k *EinoKernel) requestTurnCancel(sessionID, turnID, reason string) bool {
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

func (k *EinoKernel) markTurnCanceled(session *SessionState, snapshot *TurnSnapshot, reason string) bool {
	if session == nil || snapshot == nil {
		return false
	}
	if snapshot.Lifecycle == TurnLifecycleCanceled {
		return false
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCancelled, TurnLifecycleCanceled); err != nil {
		return false
	}
	now := time.Now()
	snapshot.Lifecycle = TurnLifecycleCanceled
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = strings.TrimSpace(reason)
	snapshot.UpdatedAt = now
	snapshot.CompletedAt = &now
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	if snapshot.LatestCheckpoint != nil {
		snapshot.LatestCheckpoint.Lifecycle = TurnLifecycleCanceled
		snapshot.LatestCheckpoint.ResumeState = TurnResumeStateNone
		snapshot.LatestCheckpoint.UpdatedAt = now
	}
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleCanceled
		last.ResumeState = TurnResumeStateNone
		last.UpdatedAt = now
		last.CompletedAt = &now
	}
	session.PendingApprovals = nil
	session.PendingEvidence = nil
	k.persistTurnSnapshot(session, snapshot)
	if k.projector != nil {
		k.projector.Emit(LifecycleEvent{
			Type:      EventTurnAborted,
			SessionID: session.ID,
			TurnID:    snapshot.ID,
			Timestamp: now,
		})
	}
	return true
}

// ---------------------------------------------------------------------------
// PipelineStep tracks which steps of the turn pipeline have been executed.
// Used for testing and observability.
// ---------------------------------------------------------------------------

// PipelineStep identifies a step in the turn pipeline.
type PipelineStep string

const (
	StepAssembleContext PipelineStep = "assemble_context"
	StepCompilePrompt   PipelineStep = "compile_prompt"
	StepAssembleTools   PipelineStep = "assemble_tools"
	StepCreateAgent     PipelineStep = "create_agent"
	StepRunnerRun       PipelineStep = "runner_run"
	StepCallbackEvents  PipelineStep = "callback_events"
	StepProjection      PipelineStep = "projection"
	StepFinalGate       PipelineStep = "final_gate"
)

// AllPipelineSteps returns the canonical pipeline step order.
func AllPipelineSteps() []PipelineStep {
	return []PipelineStep{
		StepAssembleContext,
		StepCompilePrompt,
		StepAssembleTools,
		StepCreateAgent,
		StepRunnerRun,
		StepCallbackEvents,
		StepProjection,
		StepFinalGate,
	}
}

// ---------------------------------------------------------------------------
// PipelineRecorder records pipeline step execution order (for testing).
// ---------------------------------------------------------------------------

// PipelineRecorder records the order of pipeline steps executed during a turn.
type PipelineRecorder struct {
	Steps []PipelineStep
}

// Record appends a step to the recorder.
func (r *PipelineRecorder) Record(step PipelineStep) {
	r.Steps = append(r.Steps, step)
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
func (k *EinoKernel) RunTurn(ctx context.Context, req TurnRequest) (result TurnResult, err error) {
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
	if hostID := strings.TrimSpace(req.HostID); hostID != "" {
		session.HostID = hostID
		req.HostID = hostID
	} else if hostID := strings.TrimSpace(session.HostID); hostID != "" {
		req.HostID = hostID
	}
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
			map[string]string{"messageId": msg.ID},
		))
		k.persistTurnSnapshot(session, snapshot)
	}
	recomputeContextWindow(&session.Context, session.Messages)
	if pendingCancelReason != "" {
		snapshot := k.ensureCurrentTurnSnapshot(session, req, turnID)
		k.markTurnCanceled(session, snapshot, pendingCancelReason)
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
			k.markTurnCanceled(session, snapshot, "user stop")
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
	if blocked != nil {
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

	// Step 6: Final gate check via PolicyEngine.CompletionPolicy
	if k.policy.CompletionPolicy != nil {
		turnState := policyengine.TurnState{
			SessionID: session.ID,
			TurnID:    turnID,
			Completed: true,
		}
		decision := k.policy.CompletionPolicy.CheckCompletion(runCtx, turnState)
		if decision.Action != policyengine.PolicyActionAllow {
			if k.spanSource != nil && turnSpanID != "" {
				k.spanSource.FailSpan(turnSpanID, "blocked: "+decision.Reason)
			}
			return TurnResult{
				SessionType:     req.SessionType,
				Mode:            req.Mode,
				SessionID:       session.ID,
				TurnID:          turnID,
				ClientTurnID:    req.ClientTurnID,
				ClientMessageID: req.ClientMessageID,
				Status:          "blocked",
				Error:           decision.Reason,
			}, nil
		}
	}
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
func (k *EinoKernel) ResumeTurn(ctx context.Context, req ResumeRequest) (TurnResult, error) {
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
	snapshot.Metadata = mergeResumeTurnMetadata(snapshot.Metadata, req.Metadata)
	if len(snapshot.TraceContext) > 0 {
		runCtx = k.runtimeObserver().ContextWithTraceContext(runCtx, snapshot.TraceContext)
	}
	if planApproval := pendingApprovalByID(session, snapshot, req.ApprovalID); planApproval.Source == PlanModeEntryApprovalSource || planApproval.Source == PlanExitApprovalSource {
		decisionReason := firstNonEmpty(req.Metadata["approval.reason"], req.Metadata["rejection.reason"], req.Metadata["reason"])
		now := time.Now()
		var err error
		status := "completed"
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
		k.emitApprovalDecided(session, snapshot, planApproval.ID, approvedResumeDecisionLabel(req.Decision), map[bool]string{true: "approved", false: "denied"}[isApprovedResumeDecision(req.Decision)], now)
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
		if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnFailed, TurnLifecycleFailed); err != nil {
			return TurnResult{}, err
		}
		now := time.Now()
		snapshot.Lifecycle = TurnLifecycleFailed
		snapshot.ResumeState = TurnResumeStateNone
		snapshot.Error = "approval denied"
		snapshot.UpdatedAt = now
		snapshot.CompletedAt = &now
		approval := pendingApprovalByID(session, snapshot, req.ApprovalID)
		recordRejectedApproval(session, approval, req.Decision, firstNonEmpty(req.Metadata["approval.reason"], req.Metadata["rejection.reason"], req.Metadata["reason"]), now)
		session.PendingApprovals = nil
		session.PendingEvidence = nil
		k.persistTurnSnapshot(session, snapshot)
		k.emitApprovalDecided(session, snapshot, req.ApprovalID, req.Decision, "denied", now)
		return TurnResult{
			SessionType:     session.Type,
			Mode:            session.Mode,
			SessionID:       session.ID,
			TurnID:          req.TurnID,
			ClientTurnID:    snapshot.ClientTurnID,
			ClientMessageID: snapshot.ClientMessageID,
			Status:          "blocked",
			Error:           "approval denied",
		}, nil
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
		k.markTurnCanceled(session, snapshot, pendingCancelReason)
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
	} else if toolCall, ok := pendingToolCall(snapshot); ok {
		if isSessionApprovalResumeDecision(req.Decision) {
			rememberSessionApprovalGrant(session, toolCall, req.ApprovalID)
		}
		k.emitApprovalDecided(session, snapshot, req.ApprovalID, approvedResumeDecisionLabel(req.Decision), "approved", time.Now())
		blocked, err := k.resumePendingToolCall(runCtx, session, snapshot)
		if err != nil {
			return TurnResult{}, err
		}
		if blocked != nil {
			return *blocked, nil
		}
	} else {
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
			k.markTurnCanceled(session, snapshot, "user stop")
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

func (k *EinoKernel) emitApprovalDecided(session *SessionState, snapshot *TurnSnapshot, approvalID, decision, status string, at time.Time) {
	if k == nil || k.projector == nil || session == nil || snapshot == nil {
		return
	}
	if at.IsZero() {
		at = time.Now()
	}
	approval := pendingApprovalByID(session, snapshot, approvalID)
	id := strings.TrimSpace(firstNonEmpty(approvalID, approval.ID, currentBlockedID(snapshot)))
	if id == "" {
		return
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
func (k *EinoKernel) CancelTurn(_ context.Context, req CancelRequest) (TurnResult, error) {
	if err := req.Validate(); err != nil {
		return TurnResult{}, fmt.Errorf("invalid cancel request: %w", err)
	}

	session := k.sessions.Get(req.SessionID)
	if session == nil {
		return TurnResult{}, fmt.Errorf("session %q not found", req.SessionID)
	}
	k.requestTurnCancel(req.SessionID, req.TurnID, req.Reason)

	var clientTurnID, clientMessageID string
	if snapshot := session.CurrentTurn; snapshot != nil && snapshot.ID == req.TurnID {
		clientTurnID = snapshot.ClientTurnID
		clientMessageID = snapshot.ClientMessageID
		k.markTurnCanceled(session, snapshot, req.Reason)
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

// RunTurnWithRecorder executes RunTurn while recording pipeline steps for testing.
func (k *EinoKernel) RunTurnWithRecorder(ctx context.Context, req TurnRequest, recorder *PipelineRecorder) (result TurnResult, err error) {
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

	if valErr := req.Validate(); valErr != nil {
		return TurnResult{}, fmt.Errorf("invalid turn request: %w", valErr)
	}

	turnID := req.TurnID
	if turnID == "" {
		turnID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}

	// Step 1: Assemble context
	recorder.Record(StepAssembleContext)
	session := k.sessions.GetOrCreate(req.SessionID, req.SessionType, req.Mode)
	preTurnEvent, err := k.runTurnHook(ctx, hooks.StagePreTurn, session, req, turnID, "", nil)
	if err != nil {
		return TurnResult{}, fmt.Errorf("pre_turn: %w", err)
	}
	if preTurnEvent.UpdatedInput != "" {
		req.Input = preTurnEvent.UpdatedInput
	}
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
	}
	recomputeContextWindow(&session.Context, session.Messages)

	// Step 2: Compile prompt
	recorder.Record(StepCompilePrompt)
	turnMetadata := k.applyProgressiveToolPackMetadata(cloneTurnMetadata(req.Metadata), req.Input, req.SessionType, req.Mode, session)
	depthProfile := depthProfileFromTurnRequest(TurnRequest{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		Input:       req.Input,
		Metadata:    turnMetadata,
	})
	compileCtx := enrichCompileContext(k.compileContext(req.SessionType, req.Mode, turnMetadata), req.SessionType, req.HostID, turnMetadata, time.Now())
	compileCtx = applyDepthProfileToCompileContext(compileCtx, depthProfile, firstMetadataValue(turnMetadata, "reasoningEffort", "reasoning_effort"))
	compileCtx = appendRuntimeEnvironmentContextSection(compileCtx, session)
	compileCtx = appendSkillActivationContext(compileCtx, session)
	compileCtx = appendMCPInstructionContext(compileCtx, session)
	if len(preTurnEvent.AdditionalContext) > 0 {
		compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, preTurnEvent.AdditionalContext...)
	}
	compileCtx, _ = applyToolSurfacePolicyToCompileContext(compileCtx, req.Mode, firstMetadataValue(turnMetadata, "profile", "toolProfile"), session)
	compiled, compileErr := k.compiler.Compile(compileCtx)
	if compileErr != nil {
		return TurnResult{}, fmt.Errorf("compile prompt: %w", compileErr)
	}

	// Step 3: Assemble tools
	recorder.Record(StepAssembleTools)
	toolPool := tooling.AssembleEinoToolPool(compileCtx.AssembledTools)

	// Step 4: Create agent
	recorder.Record(StepCreateAgent)
	agentKind := modelrouter.AgentKindWorker
	if req.SessionType == SessionTypeWorkspace {
		agentKind = modelrouter.AgentKindPlanner
	}
	chatModel, modelErr := k.modelRouter.GetModel(agentKind, modelrouter.ProviderConfig{})
	if modelErr != nil {
		return TurnResult{}, fmt.Errorf("get model: %w", modelErr)
	}

	// Step 5: Runner.Run()
	recorder.Record(StepRunnerRun)
	agentOutput, runErr := executeAgent(ctx, chatModel, compiled, toolPool, session.Messages)
	if runErr != nil {
		return TurnResult{}, fmt.Errorf("run agent: %w", runErr)
	}

	// Step 6: Callback events
	recorder.Record(StepCallbackEvents)

	// Step 7: Projection
	recorder.Record(StepProjection)
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

	// Step 8: Final gate
	recorder.Record(StepFinalGate)
	if k.policy.CompletionPolicy != nil {
		turnState := policyengine.TurnState{
			SessionID: session.ID,
			TurnID:    turnID,
			Completed: true,
		}
		decision := k.policy.CompletionPolicy.CheckCompletion(ctx, turnState)
		if decision.Action != policyengine.PolicyActionAllow {
			return TurnResult{
				SessionType:     req.SessionType,
				Mode:            req.Mode,
				SessionID:       session.ID,
				TurnID:          turnID,
				ClientTurnID:    req.ClientTurnID,
				ClientMessageID: req.ClientMessageID,
				Status:          "blocked",
				Error:           decision.Reason,
			}, nil
		}
	}
	if _, err := k.runTurnHook(ctx, hooks.StagePostTurn, session, req, turnID, agentOutput, nil); err != nil {
		return TurnResult{}, fmt.Errorf("post_turn: %w", err)
	}

	assistantMsg := Message{
		ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
		Role:      "assistant",
		Content:   agentOutput,
		Timestamp: time.Now(),
	}
	session.Messages = append(session.Messages, assistantMsg)
	session.UpdatedAt = time.Now()
	k.sessions.Update(session)

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

func (k *EinoKernel) runTurnHook(ctx context.Context, stage hooks.Stage, session *SessionState, req TurnRequest, turnID, output string, turnErr error) (hooks.TurnEvent, error) {
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

func (k *EinoKernel) compileContext(session SessionType, mode Mode, metadata map[string]string) promptcompiler.CompileContext {
	flags := featureflag.FromEnv(os.Getenv)
	if source, ok := k.tools.(metadataToolAssemblySource); ok {
		compileCtx := promptcompiler.CompileContext{
			SessionType:    string(session),
			Mode:           string(mode),
			AssembledTools: source.CompileContextWithMetadata(session, mode, metadata),
		}
		compileCtx = k.attachDeferredToolDirectoryContext(compileCtx, session, mode)
		compileCtx.AssembledTools = appendContextArtifactTools(compileCtx.AssembledTools, k.contextArtifactToolsForMetadata(metadata)...)
		return applyRuntimeFeatureFlags(compileCtx, flags)
	}
	compileCtx := k.tools.CompileContext(session, mode)
	compileCtx = k.attachDeferredToolDirectoryContext(compileCtx, session, mode)
	if opsManualsOptedOut(metadata) {
		compileCtx.AssembledTools = filterOpsManualTools(compileCtx.AssembledTools)
	}
	compileCtx.AssembledTools = appendContextArtifactTools(compileCtx.AssembledTools, k.contextArtifactToolsForMetadata(metadata)...)
	return applyRuntimeFeatureFlags(compileCtx, flags)
}

func (k *EinoKernel) attachDeferredToolDirectoryContext(ctx promptcompiler.CompileContext, session SessionType, mode Mode) promptcompiler.CompileContext {
	if len(ctx.DeferredToolCatalog) == 0 {
		ctx.DeferredToolCatalog = k.progressiveDiscoveryCatalog(session, mode)
	}
	if ctx.MCPHealthSnapshot == nil {
		ctx.MCPHealthSnapshot = mcpHealthSnapshotForPrompt()
	}
	return ctx
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

func (k *EinoKernel) contextArtifactToolsForMetadata(metadata map[string]string) []promptcompiler.Tool {
	if !contextArtifactToolsEnabled(metadata) {
		return nil
	}
	return k.contextArtifactTools()
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

func (k *EinoKernel) contextArtifactTools() []promptcompiler.Tool {
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
				MaxInlineResultBytes: 4096,
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
					MaxInlineResultBytes: 4096,
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
	filtered, snapshot := tooling.ApplyToolSurfacePolicy(ctx.AssembledTools, tooling.ToolSurfacePolicyOptions{
		Mode:                string(mode),
		Profile:             profile,
		ActiveSkillPolicies: activeSkillToolPolicies(session),
	})
	ctx.AssembledTools = filtered
	return ctx, snapshot
}

func (k *EinoKernel) applyProgressiveToolPackMetadata(metadata map[string]string, input string, sessionType SessionType, mode Mode, session *SessionState) map[string]string {
	return applyProgressiveToolPackMetadata(metadata, input, session, k.progressiveDiscoveryCatalog(sessionType, mode))
}

func (k *EinoKernel) progressiveDiscoveryCatalog(session SessionType, mode Mode) []tooling.Tool {
	source, ok := k.tools.(fullToolCatalogSource)
	if !ok {
		return nil
	}
	return source.AssembleToolsWithOptions(string(session), string(mode), tooling.AssembleOptions{IncludeDeferredCatalog: true})
}

func (k *EinoKernel) contextBudgetPolicyForSession(session *SessionState, agentKind modelrouter.AgentKind) ContextBudgetPolicy {
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

func (k *EinoKernel) assembleToolPool(session SessionType, mode Mode, metadata map[string]string) []tool.BaseTool {
	if source, ok := k.tools.(metadataToolAssemblySource); ok {
		return source.AssembleToolPoolWithMetadata(session, mode, metadata)
	}
	if !opsManualsOptedOut(metadata) {
		return k.tools.AssembleToolPool(session, mode)
	}
	return tooling.AssembleEinoToolPool(filterOpsManualTools(k.tools.CompileContext(session, mode).AssembledTools))
}

func (k *EinoKernel) runHostIterationLoop(
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
	previousPromptInputTrace := latestModelInputPromptTrace(snapshot)
	var lastReasoningPersist time.Time
	turnMetadata := k.applyProgressiveToolPackMetadata(cloneTurnMetadata(req.Metadata), req.Input, req.SessionType, req.Mode, session)
	depthProfile := depthProfileFromTurnRequest(TurnRequest{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		Input:       req.Input,
		Metadata:    turnMetadata,
	})
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
			SessionID:        session.ID,
			TurnID:           turnID,
			Iteration:        iteration,
			Compressor:       k.compressor,
			PendingApprovals: session.PendingApprovals,
			PendingEvidence:  session.PendingEvidence,
			BudgetPolicy:     budgetPolicy,
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
		k.emitIterationStage(session.ID, turnID, iteration, "compile_prompt", turnSpanID)
		compileCtx := enrichCompileContext(k.compileContext(req.SessionType, req.Mode, turnMetadata), req.SessionType, session.HostID, turnMetadata, time.Now())
		compileCtx = applyDepthProfileToCompileContext(compileCtx, snapshot.TaskDepth, firstMetadataValue(turnMetadata, "reasoningEffort", "reasoning_effort"))
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
		var surfacePolicy tooling.ToolSurfacePolicySnapshot
		compileCtx, surfacePolicy = applyToolSurfacePolicyToCompileContext(compileCtx, req.Mode, firstMetadataValue(turnMetadata, "profile", "toolProfile"), session)
		if shouldSwitchToSynthesisOnlyForTurn(req.Mode, snapshot.TaskDepth, req.Input, session, snapshot, toolDispatches, compileCtx.AssembledTools) {
			applyHiddenTools(snapshot, toolNames(compileCtx.AssembledTools))
			compileCtx.AssembledTools = nil
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, synthesisOnlyPromptAsset(toolDispatches))
		}
		if len(additionalContext) > 0 {
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, additionalContext...)
		}
		if evidencePrompt := evidenceAwareFinalAnswerPromptAsset(snapshot); evidencePrompt != "" {
			compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, evidencePrompt)
		}
		compileCtx.EvidenceReminders = compileEvidenceReminders(req.Mode, session.PendingEvidence)
		compileCtx.ToolDelta = iterationToolDelta(snapshot, compileCtx.AssembledTools)
		compileCtx.ProtocolState = buildProtocolPromptState(snapshot, compileCtx.ToolDelta, session.PendingApprovals, session.PendingEvidence, session.RejectedApprovals)
		compiled, compileErr := k.compiler.Compile(compileCtx)
		if compileErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, compileErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("compile prompt: %w", compileErr)
		}
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
		stablePromptHash := promptContentHash(compiled.Stable.Content)
		promptFingerprint := promptFingerprintMap(compiled.Fingerprint)
		toolFingerprint := assembledToolFingerprint(k.tools, req.SessionType, req.Mode, compileCtx.AssembledTools)
		visibleToolNames := toolNames(compileCtx.AssembledTools)
		toolSurfaceSnapshot := ToolSurfaceSnapshotRef{
			ID:                 fmt.Sprintf("toolsurface-%s-%d", turnID, iteration),
			Fingerprint:        toolFingerprint,
			ToolNames:          append([]string(nil), visibleToolNames...),
			PolicySnapshotHash: surfacePolicy.Hash,
			PolicySnapshot:     &surfacePolicy,
			CreatedAt:          time.Now(),
		}
		refreshedTools := refreshedToolNames(snapshot, toolFingerprint, compileCtx.AssembledTools)

		k.emitIterationStage(session.ID, turnID, iteration, "assemble_tools", turnSpanID)
		toolPool := tooling.AssembleEinoToolPool(compileCtx.AssembledTools)
		k.emitIterationStage(session.ID, turnID, iteration, "call_model", turnSpanID)
		promptBuild, modelErr := buildPromptInputWithContextGovernance(contextMessages, compiled, append([]ContextGovernanceEvent(nil), session.ContextGovernanceEvents...))
		if modelErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, modelErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, modelErr
		}
		modelInput := promptBuild.Messages
		var promptInputDiff *promptinput.TraceDiff
		if previousPromptInputTrace != nil {
			diff := promptinput.DiffTrace(*previousPromptInputTrace, promptBuild.Trace)
			promptInputDiff = &diff
		}
		toolTraceFields := buildModelInputToolTraceFields(session, snapshot, toolFingerprint, surfacePolicy.Hash)
		finalEvidenceTrace := BuildFinalEvidenceState(snapshot, session)
		planRequirementTrace := planRequirementDecisionTrace(EvaluatePlanRequirement(snapshot.TaskDepth, snapshot, false))
		planCompletionDecision, planCompletionPresent := evaluateRuntimePlanCompletionGate(session, snapshot)
		planCompletionTrace := planCompletionGateTrace(planCompletionDecision, planCompletionPresent)
		verificationCompletionDecision := EvaluateVerificationCompletionGate(snapshot.TaskDepth, snapshot)
		verificationCompletionTrace := verificationCompletionGateTrace(verificationCompletionDecision)
		uxProgressTrace := BuildUXProgressTrace(snapshot)
		evidenceCoverageDecision := EvaluateEvidenceCoverageGate(snapshot)
		tracePath, _ := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
			SessionID:                     session.ID,
			TurnID:                        turnID,
			Iteration:                     iteration,
			Metadata:                      turnMetadata,
			Compiled:                      compiled,
			ModelInput:                    modelInput,
			VisibleTools:                  visibleToolNames,
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
			ToolSurfaceFingerprint:        toolTraceFields.ToolSurfaceFingerprint,
			ToolSurfacePolicySnapshotHash: toolTraceFields.ToolSurfacePolicySnapshotHash,
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
			ResourceLocks:                 toolTraceFields.ResourceLocks,
			FailedToolSummaries:           toolTraceFields.FailedToolSummaries,
			FinalEvidenceState:            &finalEvidenceTrace,
		})
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
				"taskDepth":                snapshot.TaskDepth,
				"uxProgressTrace":          uxProgressTrace,
				"evidenceCoverageDecision": evidenceCoverageDecision,
			},
		))
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
			MessageCount:      len(modelInput),
			TraceFile:         traceFile,
			TraceDiffFile:     traceDiffFile,
		})
		if modelSpanCtx != nil {
			modelCtx = modelSpanCtx
		}
		finalItemID := fmt.Sprintf("%s-final-answer-%d", turnID, iteration)
		iterationAssistantOutput := ""
		modelCallStartedAt := time.Now()
		response, genErr := generateModelResponse(modelCtx, chatModel, modelInput, toolPool, func(delta string) {
			if delta != "" {
				iterationAssistantOutput += delta
				snapshot.FinalOutput += delta
				if strings.TrimSpace(iterationAssistantOutput) != "" {
					if hasAgentItemID(snapshot.AgentItems, finalItemID) {
						updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, iterationAssistantOutput)
					} else {
						appendAgentItem(snapshot, newAgentItem(
							finalItemID,
							agentstate.TurnItemTypeFinalAnswer,
							agentstate.ItemStatusRunning,
							iterationAssistantOutput,
							nil,
						))
					}
				}
				snapshot.UpdatedAt = time.Now()
				k.persistTurnSnapshot(session, snapshot)
			}
			k.emitRuntimeEvent(EventAssistantFinalDelta, session.ID, turnID, map[string]any{
				"text": delta,
			})
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
		if genErr != nil {
			appendModelTraceResponse(tracePath, modelItemID, nil, time.Since(modelCallStartedAt), genErr)
			finishObservedSpan(modelSpan, "failed", genErr.Error(), map[string]any{"error": genErr.Error()})
			updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusFailed, genErr.Error())
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, genErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, genErr
		}
		appendModelTraceResponse(tracePath, modelItemID, response, time.Since(modelCallStartedAt), nil)
		toolCallCount := 0
		if response != nil {
			toolCallCount = len(response.ToolCalls)
		}
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
			PromptDelta:             compiled.Dynamic.Content,
			PromptFingerprint:       promptFingerprint,
			ModelInputTraceFile:     tracePath,
			TokenBudget:             session.Context.MaxTokens,
			Checkpoint:              checkpoint,
			CompactedSegments:       append([]CompactedSegment(nil), contextState.CompactedSegments...),
			ExternalReferences:      append([]ExternalReference(nil), contextState.ExternalReferences...),
			ContextGovernanceEvents: append([]ContextGovernanceEvent(nil), contextState.GovernanceEvents...),
			StartedAt:               time.Now(),
			UpdatedAt:               time.Now(),
		}

		assistantMsg := runtimeMessageFromSchema(response)
		if assistantMsg.ID == "" {
			assistantMsg.ID = fmt.Sprintf("msg-%d", time.Now().UnixNano())
		}
		if assistantMsg.Timestamp.IsZero() {
			assistantMsg.Timestamp = time.Now()
		}
		iterState.ToolCalls = append(iterState.ToolCalls, assistantMsg.ToolCalls...)
		session.Messages = append(session.Messages, assistantMsg)
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
				snapshot.FinalOutput = ""
				if hasAgentItemID(snapshot.AgentItems, finalItemID) {
					updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
				}
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
				snapshot.FinalOutput = ""
				if hasAgentItemID(snapshot.AgentItems, finalItemID) {
					updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
				}
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
				snapshot.FinalOutput = ""
				if hasAgentItemID(snapshot.AgentItems, finalItemID) {
					updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
				}
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
				snapshot.FinalOutput = ""
				if hasAgentItemID(snapshot.AgentItems, finalItemID) {
					updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
				}
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
					snapshot.FinalOutput = ""
					if hasAgentItemID(snapshot.AgentItems, finalItemID) {
						updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
					}
					k.persistTurnSnapshot(session, snapshot)
					continue
				}
			}
			mandatorySkillDecision := EvaluateMandatorySkillActivation(k.mandatorySkillDefinitionsForInput(req.Input), req.Input, assistantContent, session.SkillActivation)
			if mandatorySkillDecision.Action == "require_skill_read" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				if snapshot.Metadata["mandatorySkillActivationRetry"] != "1" {
					snapshot.Metadata["mandatorySkillActivationRetry"] = "1"
					additionalContext = append(additionalContext, mandatorySkillRetryPrompt(mandatorySkillDecision))
					snapshot.FinalOutput = ""
					if hasAgentItemID(snapshot.AgentItems, finalItemID) {
						updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
					}
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
			finalCompletenessDecision := EvaluateFinalCompleteness(assistantContent)
			if finalCompletenessDecision.Action == "retry_complete_final" && snapshot.Metadata[finalCompletenessRetryMetadataKey] != "1" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata[finalCompletenessRetryMetadataKey] = "1"
				additionalContext = append(additionalContext, finalCompletenessRetryPrompt(finalCompletenessDecision))
				snapshot.FinalOutput = ""
				if hasAgentItemID(snapshot.AgentItems, finalItemID) {
					updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
				}
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			finalEvidence := BuildFinalEvidenceState(snapshot, session)
			finalEvidenceDecision := VerifyFinalEvidence(assistantContent, finalEvidence)
			if finalEvidenceDecision.Action != FinalEvidenceActionAllow && snapshot.Metadata["finalEvidenceVerifierRetry"] != "1" {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata["finalEvidenceVerifierRetry"] = "1"
				additionalContext = append(additionalContext, finalEvidenceRetryPrompt(finalEvidenceDecision))
				snapshot.FinalOutput = ""
				if hasAgentItemID(snapshot.AgentItems, finalItemID) {
					updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusRunning, assistantContent)
				}
				k.persistTurnSnapshot(session, snapshot)
				continue
			}
			if blocker, blocked := missingEvidenceFinalBlocker(snapshot.TaskDepth, snapshot, assistantContent); blocked {
				if snapshot.Metadata == nil {
					snapshot.Metadata = map[string]string{}
				}
				snapshot.Metadata["taskDepth.missingEvidenceFinalBlocked"] = "true"
				assistantContent = blocker
			}
			if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnCompleted, TurnLifecycleCompleted); err != nil {
				return "", nil, err
			}
			now := time.Now()
			snapshot.Lifecycle = TurnLifecycleCompleted
			snapshot.ResumeState = TurnResumeStateNone
			snapshot.FinalOutput = assistantContent
			if hasAgentItemID(snapshot.AgentItems, finalItemID) {
				updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusCompleted, assistantContent)
			} else {
				appendAgentItem(snapshot, newAgentItem(
					finalItemID,
					agentstate.TurnItemTypeFinalAnswer,
					agentstate.ItemStatusCompleted,
					assistantContent,
					map[string]string{"messageId": assistantMsg.ID},
				))
			}
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
			k.persistTurnSnapshot(session, snapshot)
			return assistantContent, nil, nil
		}

		if assistantContent != "" {
			if hasAgentItemID(snapshot.AgentItems, finalItemID) {
				updateAgentItem(snapshot, finalItemID, agentstate.ItemStatusCompleted, assistantContent)
			} else {
				appendAgentItem(snapshot, newAgentItem(
					finalItemID,
					agentstate.TurnItemTypeFinalAnswer,
					agentstate.ItemStatusCompleted,
					assistantContent,
					map[string]string{"messageId": assistantMsg.ID},
				))
			}
		}
		if snapshot.FinalOutput != "" {
			snapshot.FinalOutput = ""
			snapshot.UpdatedAt = time.Now()
			k.persistTurnSnapshot(session, snapshot)
		}
		if intentText := toolIntentPrelude(req.Input, assistantMsg); intentText != "" {
			k.emitRuntimeEvent(EventAssistantIntent, session.ID, turnID, map[string]any{
				"text": intentText,
			})
		}

		dispatcher := k.newIterationDispatcher(session, snapshot, iteration, dispatchTools)
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
			k.applyAggregateToolResultBudget(session, snapshot, iteration, dispatchTools)
			snapshot.LatestCheckpoint = newCheckpointMetadata(session.ID, snapshot.ID, iteration, nextCheckpointSequence(snapshot), "tool_result", TurnLifecycleRunning, snapshot.ResumeState)
			appendExternalReferences(&snapshot.ExternalReferences, recordedResult.ExternalReferences...)
			appendExternalReferences(&session.ExternalReferences, recordedResult.ExternalReferences...)
			if snapshot.LatestCheckpoint != nil {
				appendCheckpointExternalRefs(snapshot.LatestCheckpoint, recordedResult.ExternalReferences)
				snapshot.LatestCheckpoint.Incremental = true
			}
			if last := latestIteration(snapshot); last != nil {
				last.Checkpoint = snapshot.LatestCheckpoint
			}
			session.LatestCheckpoint = snapshot.LatestCheckpoint
			k.persistTurnSnapshot(session, snapshot)
			return nil, nil
		}

		for i := 0; i < len(assistantMsg.ToolCalls); {
			tc := assistantMsg.ToolCalls[i]
			if canDispatchToolCallInParallel(dispatchTools, tc) && toolDispatches < defaultMaxToolDispatchesPerTurn {
				remaining := defaultMaxToolDispatchesPerTurn - toolDispatches
				batch := make([]ToolCall, 0, remaining)
				for i < len(assistantMsg.ToolCalls) && len(batch) < remaining && canDispatchToolCallInParallel(dispatchTools, assistantMsg.ToolCalls[i]) {
					batch = append(batch, assistantMsg.ToolCalls[i])
					i++
				}
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
				var wg sync.WaitGroup
				for j, batchCall := range batch {
					wg.Add(1)
					go func(index int, call ToolCall) {
						defer wg.Done()
						dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(req.HostID, turnMetadata))
						results[index] = dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, turnID, call, req.SessionType, req.Mode, turnSpanID)
					}(j, batchCall)
				}
				wg.Wait()
				toolDispatches += countToolCallsTowardBudget(batch)

				for j, batchCall := range batch {
					blocked, err := processDispatchResult(batchCall, toolItemIDs[j], results[j])
					if blocked != nil || err != nil {
						return "", blocked, err
					}
				}
				continue
			}

			i++
			toolItemID := appendToolCallState(tc)
			k.persistTurnSnapshot(session, snapshot)
			markToolInvocationRunning(snapshot, tc.ID)
			k.persistTurnSnapshot(session, snapshot)
			dispatchResult := DispatchResult{
				ToolCallID: tc.ID,
				Metadata:   toolMetadataForToolCall(dispatchTools, tc),
			}
			if countsTowardToolBudget(tc) && toolDispatches >= defaultMaxToolDispatchesPerTurn {
				dispatchResult.Result = toolBudgetReachedResultForModel(tc, toolDispatches)
				applyHiddenTools(snapshot, toolNames(compileCtx.AssembledTools))
			} else {
				dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(req.HostID, turnMetadata))
				dispatchResult = dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, turnID, tc, req.SessionType, req.Mode, turnSpanID)
				if countsTowardToolBudget(tc) {
					toolDispatches++
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
	snapshot.Lifecycle = TurnLifecycleFailed
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = "iteration limit exceeded"
	snapshot.UpdatedAt = now
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, maxIterations, nextCheckpointSequence(snapshot), "iteration_limit", TurnLifecycleFailed, TurnResumeStateNone)
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint
	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleFailed
		last.ResumeState = TurnResumeStateNone
		last.Checkpoint = checkpoint
		last.UpdatedAt = now
	}
	appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, maxIterations), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, "iteration limit exceeded", nil))
	k.persistTurnSnapshot(session, snapshot)
	return "", nil, fmt.Errorf("iteration limit exceeded")
}

const (
	defaultMaxToolDispatchesPerTurn    = 12
	defaultSynthesisOnlyToolDispatches = 5
)

func shouldSwitchToSynthesisOnly(mode Mode, profile taskdepth.Profile, toolDispatches int, tools []promptcompiler.Tool) bool {
	if len(tools) == 0 {
		return false
	}
	if mode == ModeExecute {
		return toolDispatches >= defaultMaxToolDispatchesPerTurn
	}
	switch profile.Level {
	case taskdepth.LevelInvestigation:
		return toolDispatches >= 8
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
		"## Synthesis-only phase\n已收集 %d 个工具结果。停止继续调用工具，基于已有工具证据直接给用户回答；如果证据不足，给出 budget-limited conclusion，明确已用证据和仍缺证据。不要把证据不足包装成高置信度根因，也不要假装调查完整。",
		toolDispatches,
	)
}

func evidenceAwareFinalAnswerPromptAsset(snapshot *TurnSnapshot) string {
	summaries := collectedToolEvidenceSummaries(snapshot, 8)
	if len(summaries) == 0 {
		return ""
	}
	lines := []string{
		"## Evidence-aware final answer",
		"Use the collected tool evidence summaries below when preparing the final answer. Do not invent evidence; if evidence is incomplete, state the limitation briefly.",
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
			text := strings.TrimSpace(result.Summary)
			if text == "" {
				text = firstNonEmptyLine(result.Content)
			}
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
			return fmt.Sprintf("我会先搜索网页核对「%s」，必要时再读取来源或用只读命令校验，最后给出简洁结论。", truncateRunes(query, 80))
		}
		return "我会先搜索网页核对关键信息，必要时再读取来源或用只读命令校验，最后给出简洁结论。"
	case "open_page", "find_in_page":
		target := toolCallStringField(firstTool, "url", "pattern", "query")
		if target != "" {
			return fmt.Sprintf("我会先浏览或检索「%s」里的关键信息，再基于证据给出结论。", truncateRunes(target, 80))
		}
		return "我会先浏览或检索网页里的关键信息，再基于证据给出结论。"
	case "shell_command", "exec_command", "execute_command", "execute_readonly_query", "code_mode":
		command := toolCallStringField(firstTool, "command", "cmd", "query")
		if command != "" {
			return fmt.Sprintf("我会先执行只读命令「%s」获取证据，再根据输出给出结论。", truncateRunes(command, 80))
		}
		return "我会先执行只读命令获取证据，再根据输出给出结论。"
	case "read_file", "list_files", "list_dir", "search_files", "grep":
		target := toolCallStringField(firstTool, "path", "file", "query", "pattern")
		if target != "" {
			return fmt.Sprintf("我会先检查「%s」相关内容，再整理必要证据和结论。", truncateRunes(target, 80))
		}
		return "我会先检查相关文件和上下文，再整理必要证据和结论。"
	default:
		if toolName != "" {
			return fmt.Sprintf("我会先用 %s 核对关键信息，再给出结论。", toolName)
		}
		return "我会先用可用工具核对关键信息，再给出结论。"
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
	body, marshalErr := json.Marshal(map[string]any{
		"type":               "tool_error",
		"toolCallId":         tc.ID,
		"toolName":           toolName,
		"failureKind":        string(decision.Kind),
		"retryable":          false,
		"userActionRequired": decision.RequiresUser,
		"message":            fmt.Sprintf("%s failed: %s", toolName, errText),
		"allowedNextActions": allowedNextActions,
	})
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

func (k *EinoKernel) resumePendingToolCall(ctx context.Context, session *SessionState, snapshot *TurnSnapshot) (*TurnResult, error) {
	toolCall, ok := pendingToolCall(snapshot)
	if !ok {
		return nil, fmt.Errorf("turn %q has no pending tool call", snapshot.ID)
	}
	if err := k.markSnapshotResuming(session, snapshot, "resume_tool_approval"); err != nil {
		return nil, err
	}
	compileCtx := enrichCompileContext(k.compileContext(session.Type, session.Mode, snapshot.Metadata), session.Type, session.HostID, snapshot.Metadata, time.Now())
	dispatcher := k.newIterationDispatcher(session, snapshot, snapshot.Iteration, compileCtx.AssembledTools)
	dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(session.HostID, snapshot.Metadata))
	markToolInvocationRunning(snapshot, toolCall.ID)
	k.persistTurnSnapshot(session, snapshot)
	result := dispatcher.DispatchApproved(dispatchCtx, session.ID, snapshot.ID, toolCall, session.Type, session.Mode)
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
	recordedResult, materializeErr := k.recordResumedToolResult(session, snapshot, snapshot.Iteration, toolCall, result.Metadata, result.Result)
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
	} else {
		markToolInvocationCompleted(snapshot, toolCall.ID)
	}
	k.persistTurnSnapshot(session, snapshot)
	return k.drainRemainingToolCallsAfterResume(ctx, session, snapshot, compileCtx, dispatcher)
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

func (k *EinoKernel) drainRemainingToolCallsAfterResume(
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

		dispatchCtx := tooling.ContextWithToolExecution(ctx, toolExecutionContextForDispatch(session.HostID, snapshot.Metadata))
		dispatchResult := dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, snapshot.ID, tc, session.Type, session.Mode, "")
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
		recordedResult, materializeErr := k.recordResumedToolResult(session, snapshot, last.Iteration, tc, dispatchResult.Metadata, dispatchResult.Result)
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
	outputSummary, _, outputPreview, rawRef, resultBytes, resultTruncated := summarizeToolLifecycleResultForEvent(turnID, tc.ID, result.Content)
	if terminal := terminalEnvelopeFromToolResultContent(result.Content); terminal != nil {
		if terminal.Command != "" {
			outputSummary = terminal.Command
		}
		if terminal.Stdout != "" {
			outputPreview, _ = json.Marshal(terminal.Stdout)
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
	if inputSummary := strings.TrimSpace(approvalCommandForToolCall(tc)); inputSummary != "" {
		payload["inputSummary"] = inputSummary
	}
	if len(tc.Arguments) > 0 {
		payload["arguments"] = json.RawMessage(append([]byte(nil), tc.Arguments...))
	}
	if len(outputPreview) > 0 {
		payload["outputPreview"] = outputPreview
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
			if toolDisplayDataShouldStayOutOfPreview(result.Display.Type) {
				payload["displayData"] = append(json.RawMessage(nil), result.Display.Data...)
			} else {
				payload["outputPreview"] = append(json.RawMessage(nil), result.Display.Data...)
			}
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

func toolDisplayDataShouldStayOutOfPreview(displayType string) bool {
	return strings.TrimSpace(displayType) != ""
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

func (k *EinoKernel) recordResumedToolResult(session *SessionState, snapshot *TurnSnapshot, iteration int, toolCall ToolCall, meta tooling.ToolMetadata, result tooling.ToolResult) (ToolResult, error) {
	recordedResult, materializeErr := k.materializeToolResult(session, snapshot, iteration, toolCall, meta, result)
	if materializeErr != nil {
		return ToolResult{}, materializeErr
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
	snapshot.LatestCheckpoint = newCheckpointMetadata(session.ID, snapshot.ID, iteration, nextCheckpointSequence(snapshot), "resume_tool_result", TurnLifecycleRunning, TurnResumeStateCheckpointReady)
	appendExternalReferences(&snapshot.ExternalReferences, recordedResult.ExternalReferences...)
	appendExternalReferences(&session.ExternalReferences, recordedResult.ExternalReferences...)
	appendCheckpointExternalRefs(snapshot.LatestCheckpoint, recordedResult.ExternalReferences)
	snapshot.LatestCheckpoint.Incremental = true
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

func (k *EinoKernel) markSnapshotResuming(session *SessionState, snapshot *TurnSnapshot, checkpointKind string) error {
	if session == nil || snapshot == nil {
		return fmt.Errorf("session and snapshot are required")
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnResumed, TurnLifecycleRunning); err != nil {
		return err
	}
	now := time.Now()
	snapshot.Lifecycle = TurnLifecycleRunning
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = ""
	snapshot.UpdatedAt = now
	snapshot.PendingApprovals = nil
	snapshot.PendingEvidence = nil
	snapshot.LatestCheckpoint = newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), checkpointKind, TurnLifecycleRunning, TurnResumeStateNone)
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
	return nil
}

func (k *EinoKernel) newIterationDispatcher(session *SessionState, snapshot *TurnSnapshot, iteration int, tools []promptcompiler.Tool) *ToolDispatcher {
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
		WithToolSurfaceFingerprint(snapshot.StableToolFingerprint).
		WithVisibleToolMetadata(toolMetadataList(tools)).
		WithReadOnlyRetryConfig(ReadOnlyRetryConfigFromFlags(featureflag.FromEnv(os.Getenv))).
		WithResourceLockGate(k.resourceLockGate).
		WithProgressSink(k.progressSink(session, snapshot, iteration))
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

func (k *EinoKernel) deferredCatalogLookup(session SessionType, mode Mode) DeferredToolCatalogLookup {
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
		if meta.Name == "" || tooling.ToolHiddenFromDiscovery(meta) {
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

func generateModelResponse(
	ctx context.Context,
	chatModel modelrouter.ChatModel,
	input []*schema.Message,
	toolPool []tool.BaseTool,
	onFinalDelta func(string),
	onReasoning func(modelrouter.ReasoningStreamEvent),
) (*schema.Message, error) {
	toolInfos, err := toolInfosFromPool(ctx, toolPool)
	if err != nil {
		return nil, fmt.Errorf("tool info: %w", err)
	}
	opts := modelOptionsForTools(toolInfos)

	stream, streamErr := chatModel.Stream(ctx, input, opts...)
	if streamErr == nil && stream != nil {
		defer stream.Close()
		chunks := make([]*schema.Message, 0, 8)
		for {
			msg, err := stream.Recv()
			if err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				// Fast exit on context cancellation — propagate immediately so
				// the caller (runHostIterationLoop) can mark the turn cancelled.
				if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
					return nil, err
				}
				return nil, err
			}
			if msg == nil {
				continue
			}
			if onReasoning != nil && len(msg.Extra) > 0 {
				event, err := modelrouter.ParseOpenAIReasoningExtra(msg.Extra, false)
				if err != nil {
					return nil, err
				}
				if event != nil {
					onReasoning(*event)
				}
			}
			if onFinalDelta != nil && msg.Content != "" {
				onFinalDelta(msg.Content)
			}
			chunks = append(chunks, msg)
		}
		response, err := schema.ConcatMessages(chunks)
		if err != nil {
			return nil, err
		}
		if isEmptyAssistantResponse(response) {
			return generateFallbackResponse(ctx, chatModel, input, opts, onFinalDelta)
		}
		return response, nil
	}

	response, err := chatModel.Generate(ctx, input, opts...)
	if err != nil {
		if streamErr != nil {
			return nil, streamErr
		}
		return nil, err
	}
	if isEmptyAssistantResponse(response) {
		if fallback := fallbackResponseFromToolEvidence(input); fallback != nil {
			if onFinalDelta != nil && fallback.Content != "" {
				onFinalDelta(fallback.Content)
			}
			return fallback, nil
		}
		return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls")
	}
	if onFinalDelta != nil && response.Content != "" {
		onFinalDelta(response.Content)
	}
	return response, nil
}

func generateFallbackResponse(
	ctx context.Context,
	chatModel modelrouter.ChatModel,
	input []*schema.Message,
	opts []einomodel.Option,
	onFinalDelta func(string),
) (*schema.Message, error) {
	const fallbackAttempts = 2
	var response *schema.Message

	for attempt := 0; attempt < fallbackAttempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		var err error
		response, err = chatModel.Generate(ctx, input, opts...)
		if err != nil {
			return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls; generate fallback failed: %w", err)
		}
		if !isEmptyAssistantResponse(response) {
			break
		}
		if len(opts) == 0 {
			continue
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err = chatModel.Generate(ctx, input)
		if err != nil {
			return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls; no-tool generate fallback failed: %w", err)
		}
		if !isEmptyAssistantResponse(response) {
			break
		}
	}
	if isEmptyAssistantResponse(response) {
		if fallback := fallbackResponseFromToolEvidence(input); fallback != nil {
			if onFinalDelta != nil && fallback.Content != "" {
				onFinalDelta(fallback.Content)
			}
			return fallback, nil
		}
		return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls")
	}
	if onFinalDelta != nil && response.Content != "" {
		onFinalDelta(response.Content)
	}
	return response, nil
}

func modelOptionsForTools(toolInfos []*schema.ToolInfo) []einomodel.Option {
	if len(toolInfos) == 0 {
		return nil
	}
	return []einomodel.Option{
		einomodel.WithTools(toolInfos),
		einomodel.WithToolChoice(schema.ToolChoiceAllowed),
	}
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

func isEmptyAssistantResponse(msg *schema.Message) bool {
	if msg == nil {
		return true
	}
	return strings.TrimSpace(msg.Content) == "" &&
		len(msg.ToolCalls) == 0 &&
		len(msg.MultiContent) == 0 &&
		len(msg.AssistantGenMultiContent) == 0
}

func fallbackResponseFromToolEvidence(input []*schema.Message) *schema.Message {
	userRequest := latestUserContent(input)
	content := latestSuccessfulToolEvidenceContent(input)
	if content == "" {
		return nil
	}
	fallback := fallbackTextFromToolEvidence(userRequest, content)
	if strings.TrimSpace(fallback) == "" {
		return nil
	}
	return schema.AssistantMessage(fallback, nil)
}

func latestUserContent(input []*schema.Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		msg := input[i]
		if msg == nil || msg.Role != schema.User {
			continue
		}
		if text := strings.TrimSpace(msg.Content); text != "" {
			return text
		}
	}
	return ""
}

func latestSuccessfulToolEvidenceContent(input []*schema.Message) string {
	for i := len(input) - 1; i >= 0; i-- {
		msg := input[i]
		if msg == nil || msg.Role != schema.Tool {
			continue
		}
		content := strings.TrimSpace(msg.Content)
		if content == "" || toolEvidenceLooksFailed(content) {
			continue
		}
		return content
	}
	return ""
}

func toolEvidenceLooksFailed(content string) bool {
	var obj map[string]any
	if json.Unmarshal([]byte(content), &obj) == nil {
		if errorValue, ok := obj["error"]; ok && strings.TrimSpace(fmt.Sprint(errorValue)) != "" {
			return true
		}
		status := strings.ToLower(strings.TrimSpace(fmt.Sprint(obj["status"])))
		return status == "error" || status == "failed" || status == "failure"
	}
	lower := strings.ToLower(content)
	return strings.Contains(lower, "tool not found") ||
		strings.Contains(lower, "permission denied") ||
		strings.Contains(lower, "failed to") ||
		strings.Contains(lower, "执行失败")
}

func fallbackTextFromToolEvidence(userRequest, content string) string {
	if answer, ok := scalarJSONFieldAnswerFromRequest(userRequest, content); ok {
		return answer
	}
	sanitized := sanitizeToolEvidenceForFallback(content)
	if sanitized == "" {
		return ""
	}
	return "已获取工具结果：\n\n" + truncateRunes(sanitized, 1200)
}

func scalarJSONFieldAnswerFromRequest(userRequest, content string) (string, bool) {
	var obj map[string]any
	if json.Unmarshal([]byte(content), &obj) != nil || len(obj) == 0 {
		return "", false
	}
	request := normalizePromptLookupText(userRequest)
	for key, value := range obj {
		if isSensitiveEvidenceKey(key) || !requestMentionsJSONField(request, key) {
			continue
		}
		text, ok := scalarJSONValueText(value)
		if !ok || strings.TrimSpace(text) == "" {
			continue
		}
		return fmt.Sprintf("%s 字段是 %s。", key, text), true
	}
	return "", false
}

func requestMentionsJSONField(normalizedRequest, key string) bool {
	normalizedKey := normalizePromptLookupText(key)
	if normalizedKey != "" && strings.Contains(normalizedRequest, normalizedKey) {
		return true
	}
	switch strings.ToLower(key) {
	case "model":
		return strings.Contains(normalizedRequest, "模型")
	case "provider":
		return strings.Contains(normalizedRequest, "供应商") || strings.Contains(normalizedRequest, "提供商") || strings.Contains(normalizedRequest, "接入")
	case "status":
		return strings.Contains(normalizedRequest, "状态")
	default:
		return false
	}
}

func scalarJSONValueText(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case float64, bool:
		return fmt.Sprint(v), true
	case nil:
		return "", false
	default:
		return "", false
	}
}

func sanitizeToolEvidenceForFallback(content string) string {
	var value any
	if json.Unmarshal([]byte(content), &value) == nil {
		redacted := redactSensitiveEvidenceValue(value)
		encoded, err := json.MarshalIndent(redacted, "", "  ")
		if err == nil {
			return string(encoded)
		}
	}
	return redactSensitiveEvidenceText(content)
}

func redactSensitiveEvidenceValue(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, child := range v {
			if isSensitiveEvidenceKey(key) {
				out[key] = "[REDACTED]"
				continue
			}
			out[key] = redactSensitiveEvidenceValue(child)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		for _, child := range v {
			out = append(out, redactSensitiveEvidenceValue(child))
		}
		return out
	default:
		return value
	}
}

func isSensitiveEvidenceKey(key string) bool {
	normalized := normalizePromptLookupText(key)
	return strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "authorization")
}

func normalizePromptLookupText(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer("_", "", "-", "", " ", "", ".", "", ":", "", "/", "")
	return replacer.Replace(value)
}

func redactSensitiveEvidenceText(content string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if isSensitiveEvidenceLine(line) {
			lines[i] = "[REDACTED]"
		}
	}
	return strings.Join(lines, "\n")
}

func isSensitiveEvidenceLine(line string) bool {
	normalized := normalizePromptLookupText(line)
	return strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "passwd") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "authorization")
}

func toolInfosFromPool(ctx context.Context, toolPool []tool.BaseTool) ([]*schema.ToolInfo, error) {
	infos := make([]*schema.ToolInfo, 0, len(toolPool))
	for _, baseTool := range toolPool {
		if baseTool == nil {
			continue
		}
		info, err := baseTool.Info(ctx)
		if err != nil {
			return nil, err
		}
		if info != nil {
			infos = append(infos, info)
		}
	}
	return infos, nil
}

func runtimeMessagesToSchema(messages []Message) ([]*schema.Message, error) {
	out := make([]*schema.Message, 0, len(messages))
	for _, msg := range messages {
		switch msg.Role {
		case "system":
			out = append(out, schema.SystemMessage(msg.Content))
		case "user":
			out = append(out, schema.UserMessage(msg.Content))
		case "assistant":
			out = append(out, schema.AssistantMessage(msg.Content, schemaToolCallsFromRuntime(msg.ToolCalls)))
		case "tool":
			toolCallID := ""
			if msg.ToolResult != nil {
				toolCallID = msg.ToolResult.ToolCallID
			}
			out = append(out, schema.ToolMessage(msg.Content, toolCallID))
		default:
			return nil, fmt.Errorf("unsupported runtime message role %q", msg.Role)
		}
	}
	return out, nil
}

func runtimeMessageFromSchema(msg *schema.Message) Message {
	if msg == nil {
		return Message{}
	}
	return Message{
		Role:      string(msg.Role),
		Content:   msg.Content,
		ToolCalls: runtimeToolCallsFromSchema(msg.ToolCalls),
		Timestamp: time.Now(),
	}
}

func runtimeToolCallsFromSchema(toolCalls []schema.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: json.RawMessage(call.Function.Arguments),
		})
	}
	return out
}

func schemaToolCallsFromRuntime(toolCalls []ToolCall) []schema.ToolCall {
	out := make([]schema.ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, schema.ToolCall{
			ID:   call.ID,
			Type: "function",
			Function: schema.FunctionCall{
				Name:      call.Name,
				Arguments: string(call.Arguments),
			},
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

func (k *EinoKernel) materializeToolResult(session *SessionState, snapshot *TurnSnapshot, iteration int, tc ToolCall, meta tooling.ToolMetadata, toolResult tooling.ToolResult) (ToolResult, error) {
	result := ToolResult{
		ToolCallID: tc.ID,
		Content:    toolResult.Content,
		Display:    copyToolDisplay(toolResult.Display),
		Error:      toolResult.Error,
		References: normalizeToolResultReferences(toolResult.References, toolResult.Display),
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

func (k *EinoKernel) persistToolResultSpill(session *SessionState, snapshot *TurnSnapshot, iteration int, tc ToolCall, meta tooling.ToolMetadata, spill *tooling.ResultSpill) (ExternalReference, error) {
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

func (k *EinoKernel) applyAggregateToolResultBudget(session *SessionState, snapshot *TurnSnapshot, iteration int, assembledTools []promptcompiler.Tool) {
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

func aggregateToolResultExternalizer(k *EinoKernel, session *SessionState, snapshot *TurnSnapshot, iteration int, calls []ToolCall, assembledTools []promptcompiler.Tool) func(ToolResult) (ExternalReference, error) {
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
		preview := truncateForBudget(original, budget.MaxInlineResultBytes)
		if strings.TrimSpace(preview) == "" {
			preview = summary
		}
		return fmt.Sprintf("Summary: %s\nPreview:\n%s\nExternal ref: %s.", summary, preview, externalReferenceLabel(ref))
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
		ID:           fmt.Sprintf("ctxgov-%s-%d-%s-l1", snapshot.ID, iteration, tc.ID),
		Layer:        ContextGovernanceLayerL1,
		Kind:         "tool_result.materialized",
		SessionID:    session.ID,
		TurnID:       snapshot.ID,
		Iteration:    iteration,
		ToolCallID:   tc.ID,
		ToolName:     firstNonBlankRuntimeString(tc.Name, result.ToolCallID),
		Message:      "工具结果已按上下文预算整理",
		ReferenceIDs: referenceIDsFromExternalReferences(result.ExternalReferences),
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

func (k *EinoKernel) ensureCurrentTurnSnapshot(session *SessionState, req TurnRequest, turnID string) *TurnSnapshot {
	if session.CurrentTurn != nil && session.CurrentTurn.ID == turnID {
		return session.CurrentTurn
	}
	now := time.Now()
	snapshot := &TurnSnapshot{
		ID:              turnID,
		ClientTurnID:    req.ClientTurnID,
		ClientMessageID: req.ClientMessageID,
		SessionID:       session.ID,
		SessionType:     req.SessionType,
		Mode:            req.Mode,
		Metadata:        cloneTurnMetadata(req.Metadata),
		Lifecycle:       TurnLifecycleRunning,
		ResumeState:     TurnResumeStateNone,
		StartedAt:       now,
		UpdatedAt:       now,
	}
	session.CurrentTurn = snapshot
	return snapshot
}

func (k *EinoKernel) persistTurnSnapshot(session *SessionState, snapshot *TurnSnapshot) {
	if session == nil || snapshot == nil {
		return
	}
	session.CurrentTurn = snapshot
	upsertTurnHistory(&session.TurnHistory, *snapshot)
	k.sessions.Update(session)
}

func (k *EinoKernel) markTurnBlocked(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, result DispatchResult) error {
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
	snapshot.Lifecycle = TurnLifecycleSuspended
	snapshot.ResumeState = resumeState
	snapshot.Error = reason
	snapshot.UpdatedAt = now
	kind := result.Outcome
	if kind == "" {
		kind = "suspended"
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), kind, TurnLifecycleSuspended, resumeState)
	if result.Source != "" {
		checkpoint.Source = result.Source
	}
	snapshot.LatestCheckpoint = checkpoint
	session.LatestCheckpoint = checkpoint

	if last := latestIteration(snapshot); last != nil {
		last.Lifecycle = TurnLifecycleSuspended
		last.ResumeState = resumeState
		last.Checkpoint = checkpoint
		last.UpdatedAt = now
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
		snapshot.PendingEvidence = []PendingEvidence{evidence}
		snapshot.PendingApprovals = nil
		session.PendingEvidence = []PendingEvidence{evidence}
		session.PendingApprovals = nil
	} else {
		approval := PendingApproval{
			ID:         fmt.Sprintf("approval-%d", now.UnixNano()),
			SessionID:  session.ID,
			TurnID:     snapshot.ID,
			Iteration:  snapshot.Iteration,
			ToolName:   tc.Name,
			ToolCallID: tc.ID,
			Command:    command,
			Reason:     reason,
			AllowedActions: []string{
				strings.TrimSpace(tc.Name),
			},
			ResourceScopes: pendingApprovalResourceScopes(result.Metadata),
			RiskCeiling:    firstNonEmpty(approvalPayloadField(result.Approval, "risk"), string(result.Metadata.EffectiveGovernance(0).RiskLevel)),
			ExpiresAt:      pendingApprovalDefaultExpiry(now),
			InputHash:      toolArgumentsHash(tc.Arguments),
			Status:         "pending",
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		if result.Approval != nil {
			approval.Risk = result.Approval.Risk
			approval.Source = result.Approval.Source
			approval.RunbookID = result.Approval.RunbookID
			approval.RunbookStep = result.Approval.RunbookStep
			approval.ExpectedEffect = result.Approval.ExpectedEffect
			approval.Rollback = result.Approval.Rollback
		}
		snapshot.PendingApprovals = []PendingApproval{approval}
		snapshot.PendingEvidence = nil
		session.PendingApprovals = []PendingApproval{approval}
		session.PendingEvidence = nil
	}

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

func (k *EinoKernel) markTurnFailed(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, result DispatchResult) error {
	if session == nil || snapshot == nil {
		return fmt.Errorf("session and snapshot are required")
	}
	if err := validateTurnLifecycleTransition(snapshot, runtimestate.TransitionTurnFailed, TurnLifecycleFailed); err != nil {
		return err
	}
	now := time.Now()
	snapshot.Lifecycle = TurnLifecycleFailed
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = result.Error
	snapshot.UpdatedAt = now
	checkpointKind := result.Outcome
	if checkpointKind == "" {
		checkpointKind = "tool_failed"
	}
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), checkpointKind, TurnLifecycleFailed, TurnResumeStateNone)
	if result.Source != "" {
		checkpoint.Source = result.Source
	}
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
	k.persistTurnSnapshot(session, snapshot)
	return nil
}

func (k *EinoKernel) markTurnFailedFromError(session *SessionState, snapshot *TurnSnapshot, err error, checkpointKind string) error {
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
	snapshot.Lifecycle = TurnLifecycleFailed
	snapshot.ResumeState = TurnResumeStateNone
	snapshot.Error = errText
	snapshot.UpdatedAt = now
	snapshot.CompletedAt = &now
	checkpoint := newCheckpointMetadata(session.ID, snapshot.ID, snapshot.Iteration, nextCheckpointSequence(snapshot), checkpointKind, TurnLifecycleFailed, TurnResumeStateNone)
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
	k.persistTurnSnapshot(session, snapshot)
	return nil
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

func (k *EinoKernel) emitIterationStage(sessionID, turnID string, iteration int, stage string, turnSpanID string) {
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

func (k *EinoKernel) emitRuntimeEvent(eventType EventType, sessionID, turnID string, payload any) {
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

func (k *EinoKernel) progressSink(session *SessionState, snapshot *TurnSnapshot, iteration int) ToolProgressSink {
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

func (k *EinoKernel) upsertPartialToolProgressMessage(session *SessionState, snapshot *TurnSnapshot, iteration int, update ToolProgressUpdate, now time.Time) {
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
	return &CheckpointMetadata{
		ID:          fmt.Sprintf("chk-%d", now.UnixNano()),
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

// ---------------------------------------------------------------------------
// executeAgent — simplified agent execution (real ADK integration in dispatch.go)
// ---------------------------------------------------------------------------

// executeAgent runs the model with the compiled prompt and tools.
// This is a simplified version; full ADK Runner integration is in dispatch.go.
func executeAgent(
	_ context.Context,
	_ modelrouter.ChatModel,
	_ promptcompiler.CompiledPrompt,
	_ []tool.BaseTool,
	_ []Message,
) (string, error) {
	// In production, this would:
	// 1. Create adk.ChatModelAgent with model + tools + instruction
	// 2. Create adk.Runner and execute via runner.Run()
	// 3. Process AgentEvents via callback
	// For now, return empty output (real implementation uses dispatch.go)
	return "", nil
}
