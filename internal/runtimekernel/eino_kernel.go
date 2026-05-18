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
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/spanstream"
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
	tools       ToolAssemblySource
	compiler    promptcompiler.Compiler
	policy      *policyengine.Engine
	permissions *permissions.Engine
	hooks       *hooks.Registry
	projector   EventEmitter
	modelRouter *modelrouter.Router
	sessions    *SessionManager
	agentMgr    AgentManagerSource
	spanSource  SpanStreamSource // optional: span tree integration for conversation tracking
	observer    Observer
	compressor  *spanstream.ContextCompressor
	spillRepo   ToolResultSpillRepository

	turnCancelMu       sync.Mutex
	inFlightTurnCancel map[string]context.CancelFunc
	pendingTurnCancel  map[string]string
}

// EinoKernelConfig holds the dependencies for creating an EinoKernel.
type EinoKernelConfig struct {
	ToolSource  ToolAssemblySource
	Compiler    promptcompiler.Compiler
	Policy      *policyengine.Engine
	Permissions *permissions.Engine
	Hooks       *hooks.Registry
	Projector   EventEmitter
	ModelRouter *modelrouter.Router
	AgentMgr    AgentManagerSource
	Sessions    *SessionManager
	SessionRepo SessionRepository
	SpanSource  SpanStreamSource // optional: if nil, span tracking is disabled
	Observer    Observer
	Compressor  *spanstream.ContextCompressor
	SpillRepo   ToolResultSpillRepository
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
		compressor:         cfg.Compressor,
		spillRepo:          cfg.SpillRepo,
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

func (k *EinoKernel) markTurnCanceled(session *SessionState, snapshot *TurnSnapshot, reason string) bool {
	if session == nil || snapshot == nil {
		return false
	}
	if snapshot.Lifecycle == TurnLifecycleCanceled {
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
		k.markTurnFailedFromError(session, snapshot, modelErr, "get_model_failed")
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
	agentOutput, blocked, runErr = k.runHostIterationLoop(runCtx, chatModel, req, session, turnID, preTurnEvent, turnSpanID)
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
			k.markTurnFailedFromError(session, snapshot, runErr, inferTurnFailureCheckpointKind(snapshot))
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
	if len(snapshot.TraceContext) > 0 {
		runCtx = k.runtimeObserver().ContextWithTraceContext(runCtx, snapshot.TraceContext)
	}
	if req.Decision != "" && !isApprovedResumeDecision(req.Decision) {
		now := time.Now()
		snapshot.Lifecycle = TurnLifecycleFailed
		snapshot.ResumeState = TurnResumeStateNone
		snapshot.Error = "approval denied"
		snapshot.UpdatedAt = now
		snapshot.CompletedAt = &now
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
		k.markSnapshotResuming(session, snapshot, "resume_user_input")
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
		k.markSnapshotResuming(session, snapshot, "resume_checkpoint")
	}

	resumeReq := TurnRequest{
		SessionType:     session.Type,
		Mode:            session.Mode,
		SessionID:       session.ID,
		TurnID:          req.TurnID,
		ClientTurnID:    snapshot.ClientTurnID,
		ClientMessageID: snapshot.ClientMessageID,
		HostID:          session.HostID,
	}
	agentOutput, blocked, runErr := k.runHostIterationLoop(runCtx, chatModel, resumeReq, session, req.TurnID, hooks.TurnEvent{}, "")
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
		k.markTurnFailedFromError(session, snapshot, runErr, inferTurnFailureCheckpointKind(snapshot))
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
	for _, approval := range snapshot.PendingApprovals {
		if target == "" || approval.ID == target {
			return approval
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
	compileCtx := enrichCompileContext(k.compileContext(req.SessionType, req.Mode, req.Metadata), req.SessionType, req.HostID, req.Metadata, time.Now())
	compileCtx = appendRuntimeEnvironmentContextSection(compileCtx, session)
	if len(preTurnEvent.AdditionalContext) > 0 {
		compileCtx.SkillPromptAssets = append(compileCtx.SkillPromptAssets, preTurnEvent.AdditionalContext...)
	}
	compiled, compileErr := k.compiler.Compile(compileCtx)
	if compileErr != nil {
		return TurnResult{}, fmt.Errorf("compile prompt: %w", compileErr)
	}

	// Step 3: Assemble tools
	recorder.Record(StepAssembleTools)
	toolPool := k.assembleToolPool(req.SessionType, req.Mode, req.Metadata)

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
		return applyRuntimeFeatureFlags(promptcompiler.CompileContext{
			SessionType:    string(session),
			Mode:           string(mode),
			AssembledTools: source.CompileContextWithMetadata(session, mode, metadata),
		}, flags)
	}
	compileCtx := k.tools.CompileContext(session, mode)
	if opsManualsOptedOut(metadata) {
		compileCtx.AssembledTools = filterOpsManualTools(compileCtx.AssembledTools)
	}
	return applyRuntimeFeatureFlags(compileCtx, flags)
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

	for iteration := len(snapshot.Iterations); iteration < maxIterations; iteration++ {
		k.emitIterationStage(session.ID, turnID, iteration, "context_pipeline", turnSpanID)
		contextState, contextErr := ApplyContextPipeline(ctx, &session.Context, session.Messages, ContextPipelineOptions{
			SessionID:        session.ID,
			TurnID:           turnID,
			Iteration:        iteration,
			Compressor:       k.compressor,
			PendingApprovals: session.PendingApprovals,
			PendingEvidence:  session.PendingEvidence,
		})
		if contextErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, contextErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("context pipeline: %w", contextErr)
		}
		contextMessages := contextState.Messages
		k.emitIterationStage(session.ID, turnID, iteration, "compile_prompt", turnSpanID)
		compileCtx := enrichCompileContext(k.compileContext(req.SessionType, req.Mode, req.Metadata), req.SessionType, session.HostID, req.Metadata, time.Now())
		compileCtx = appendRuntimeEnvironmentContextSection(compileCtx, session)
		compileCtx.AssembledTools = filterHiddenTools(compileCtx.AssembledTools, snapshot.HiddenTools)
		if shouldSwitchToSynthesisOnly(req.Mode, toolDispatches, compileCtx.AssembledTools) {
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
		compileCtx.ProtocolState = buildProtocolPromptState(snapshot, compileCtx.ToolDelta, session.PendingApprovals, session.PendingEvidence)
		compiled, compileErr := k.compiler.Compile(compileCtx)
		if compileErr != nil {
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, compileErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, fmt.Errorf("compile prompt: %w", compileErr)
		}
		stablePromptHash := promptContentHash(compiled.Stable.Content)
		promptFingerprint := promptFingerprintMap(compiled.Fingerprint)
		toolFingerprint := assembledToolFingerprint(k.tools, req.SessionType, req.Mode, compileCtx.AssembledTools)
		refreshedTools := refreshedToolNames(snapshot, toolFingerprint, compileCtx.AssembledTools)

		k.emitIterationStage(session.ID, turnID, iteration, "assemble_tools", turnSpanID)
		toolPool := tooling.AssembleEinoToolPool(compileCtx.AssembledTools)
		k.emitIterationStage(session.ID, turnID, iteration, "call_model", turnSpanID)
		promptBuild, modelErr := buildPromptInput(contextMessages, compiled)
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
		tracePath, _ := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
			SessionID:        session.ID,
			TurnID:           turnID,
			Iteration:        iteration,
			Metadata:         req.Metadata,
			Compiled:         compiled,
			ModelInput:       modelInput,
			VisibleTools:     toolNames(compileCtx.AssembledTools),
			PromptInputTrace: promptBuild.Trace,
			PromptInputDiff:  promptInputDiff,
			DiagnosticTrace:  buildRuntimeDiagnosticTrace(turnID, session, req, compileCtx),
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
				"iteration":         iteration,
				"visibleTools":      toolNames(compileCtx.AssembledTools),
				"traceFile":         traceFile,
				"traceDiffFile":     traceDiffFile,
				"promptFingerprint": promptFingerprint,
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
			VisibleTools:      toolNames(compileCtx.AssembledTools),
			MessageCount:      len(modelInput),
			TraceFile:         traceFile,
			TraceDiffFile:     traceDiffFile,
		})
		if modelSpanCtx != nil {
			modelCtx = modelSpanCtx
		}
		finalItemID := fmt.Sprintf("%s-final-answer-%d", turnID, iteration)
		iterationAssistantOutput := ""
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
			finishObservedSpan(modelSpan, "failed", genErr.Error(), map[string]any{"error": genErr.Error()})
			updateAgentItem(snapshot, modelItemID, agentstate.ItemStatusFailed, genErr.Error())
			appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, genErr.Error(), nil))
			k.persistTurnSnapshot(session, snapshot)
			return "", nil, genErr
		}
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
			ID:                  fmt.Sprintf("%s-iter-%d", turnID, iteration),
			SessionID:           session.ID,
			TurnID:              turnID,
			Iteration:           iteration,
			Lifecycle:           TurnLifecycleRunning,
			ResumeState:         TurnResumeStateNone,
			MessagesForModel:    append([]Message(nil), contextMessages...),
			ToolProgress:        nil,
			VisibleTools:        toolNames(compileCtx.AssembledTools),
			RefreshedTools:      refreshedTools,
			PromptDelta:         compiled.Dynamic.Content,
			PromptFingerprint:   promptFingerprint,
			ModelInputTraceFile: tracePath,
			TokenBudget:         session.Context.MaxTokens,
			Checkpoint:          checkpoint,
			CompactedSegments:   append([]CompactedSegment(nil), contextState.CompactedSegments...),
			ExternalReferences:  append([]ExternalReference(nil), contextState.ExternalReferences...),
			StartedAt:           time.Now(),
			UpdatedAt:           time.Now(),
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

		dispatcher := k.newIterationDispatcher(session, snapshot, iteration, compileCtx.AssembledTools)
		k.emitIterationStage(session.ID, turnID, iteration, "dispatch_tools", turnSpanID)

		appendToolCallState := func(tc ToolCall) string {
			toolItemID := toolCallItemID(turnID, tc)
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
				dispatchResult.Metadata = toolMetadataForToolCall(compileCtx.AssembledTools, tc)
			}
			if dispatchResult.Blocked {
				updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusBlocked, dispatchResult.Reason)
				k.markTurnBlocked(session, snapshot, tc, dispatchResult)
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
					k.markTurnFailed(session, snapshot, tc, dispatchResult)
					return nil, fmt.Errorf("tool %q failed: %s", tc.Name, dispatchResult.Error)
				}
				dispatchResult.Result = failedToolResultForModel(tc, dispatchResult)
				if strings.TrimSpace(dispatchResult.Metadata.Name) == "" {
					dispatchResult.Metadata.Name = tc.Name
				}
			}
			applyHiddenTools(snapshot, dispatchResult.HiddenTools)
			recordedResult, materializeErr := k.materializeToolResult(session, snapshot, iteration, tc, dispatchResult.Metadata, dispatchResult.Result)
			if materializeErr != nil {
				updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusFailed, materializeErr.Error())
				appendAgentItem(snapshot, newAgentItem(errorItemID(turnID, iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, materializeErr.Error(), map[string]string{"toolCallId": tc.ID, "toolName": tc.Name}))
				k.persistTurnSnapshot(session, snapshot)
				return nil, fmt.Errorf("materialize tool result %q: %w", tc.Name, materializeErr)
			}
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
				last.UpdatedAt = time.Now()
			}
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
			if canDispatchToolCallInParallel(compileCtx.AssembledTools, tc) && toolDispatches < defaultMaxToolDispatchesPerTurn {
				remaining := defaultMaxToolDispatchesPerTurn - toolDispatches
				batch := make([]ToolCall, 0, remaining)
				for i < len(assistantMsg.ToolCalls) && len(batch) < remaining && canDispatchToolCallInParallel(compileCtx.AssembledTools, assistantMsg.ToolCalls[i]) {
					batch = append(batch, assistantMsg.ToolCalls[i])
					i++
				}
				toolItemIDs := make([]string, len(batch))
				for j, batchCall := range batch {
					toolItemIDs[j] = appendToolCallState(batchCall)
				}
				k.persistTurnSnapshot(session, snapshot)

				results := make([]DispatchResult, len(batch))
				var wg sync.WaitGroup
				for j, batchCall := range batch {
					wg.Add(1)
					go func(index int, call ToolCall) {
						defer wg.Done()
						dispatchCtx := tooling.ContextWithToolExecution(ctx, tooling.ToolExecutionContext{HostID: req.HostID})
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
			dispatchResult := DispatchResult{
				ToolCallID: tc.ID,
				Metadata:   toolMetadataForToolCall(compileCtx.AssembledTools, tc),
			}
			if countsTowardToolBudget(tc) && toolDispatches >= defaultMaxToolDispatchesPerTurn {
				dispatchResult.Result = toolBudgetReachedResultForModel(tc, toolDispatches)
				applyHiddenTools(snapshot, toolNames(compileCtx.AssembledTools))
			} else {
				dispatchCtx := tooling.ContextWithToolExecution(ctx, tooling.ToolExecutionContext{HostID: req.HostID})
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

func shouldSwitchToSynthesisOnly(mode Mode, toolDispatches int, tools []promptcompiler.Tool) bool {
	if len(tools) == 0 {
		return false
	}
	if mode == ModeExecute {
		return toolDispatches >= defaultMaxToolDispatchesPerTurn
	}
	return toolDispatches >= defaultSynthesisOnlyToolDispatches
}

func synthesisOnlyPromptAsset(toolDispatches int) string {
	return fmt.Sprintf(
		"## Synthesis-only phase\n已收集 %d 个工具结果。停止继续调用工具，基于已有工具证据直接给用户完整回答；如果证据不足，明确说明限制，不要等待更多工具。",
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
	toolDef := toolForToolCall(tools, tc)
	if toolDef == nil {
		return false
	}
	governance := toolDef.Metadata().EffectiveGovernance(defaultMaxInlineResultBytes)
	if governance.Mutating || governance.RequiresApproval {
		return false
	}
	return toolDef.IsReadOnly(tc.Arguments) && !toolDef.IsDestructive(tc.Arguments) && toolDef.IsConcurrencySafe(tc.Arguments)
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
	return tooling.ToolResult{
		ToolCallID: tc.ID,
		Content:    fmt.Sprintf("%s failed: %s", toolName, errText),
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
	add("Coroot project", "aiops.coroot.project", "coroot.project")
	if len(lines) == 0 {
		return ""
	}
	lines = append(lines, "Use these session bindings when selecting read-only evidence tools. For Coroot, pass the bound project/environment when present; if it is absent, use the Coroot tool default and report unavailability from the tool result instead of asking whether Coroot exists.")
	return strings.Join(lines, "\n")
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
	k.markSnapshotResuming(session, snapshot, "resume_tool_approval")
	compileCtx := enrichCompileContext(k.tools.CompileContext(session.Type, session.Mode), session.Type, session.HostID, nil, time.Now())
	dispatcher := k.newIterationDispatcher(session, snapshot, snapshot.Iteration, compileCtx.AssembledTools)
	dispatchCtx := tooling.ContextWithToolExecution(ctx, tooling.ToolExecutionContext{HostID: session.HostID})
	result := dispatcher.DispatchApproved(dispatchCtx, session.ID, snapshot.ID, toolCall, session.Type, session.Mode)
	if result.Blocked {
		k.markTurnBlocked(session, snapshot, toolCall, result)
		return blockedTurnResult(session, snapshot, result.Reason), nil
	}
	if result.Error != "" {
		if !shouldFeedToolFailureBackToModel(result) {
			k.markTurnFailed(session, snapshot, toolCall, result)
			return nil, fmt.Errorf("tool %q failed: %s", toolCall.Name, result.Error)
		}
		result.Result = failedToolResultForModel(toolCall, result)
		if strings.TrimSpace(result.Metadata.Name) == "" {
			result.Metadata.Name = toolCall.Name
		}
	}
	recordedResult, materializeErr := k.recordResumedToolResult(session, snapshot, snapshot.Iteration, toolCall, result.Metadata, result.Result)
	if materializeErr != nil {
		return nil, fmt.Errorf("materialize resumed tool result %q: %w", toolCall.Name, materializeErr)
	}
	appendAgentItem(snapshot, newAgentItem(
		toolResultItemID(snapshot.ID, toolCall),
		agentstate.TurnItemTypeToolResult,
		toolResultItemStatus(recordedResult),
		truncateString(recordedResult.Content, 240),
		map[string]string{"toolCallId": toolCall.ID, "toolName": toolCall.Name},
	))
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
		k.persistTurnSnapshot(session, snapshot)

		dispatchCtx := tooling.ContextWithToolExecution(ctx, tooling.ToolExecutionContext{HostID: session.HostID})
		dispatchResult := dispatcher.DispatchWithParentSpan(dispatchCtx, session.ID, snapshot.ID, tc, session.Type, session.Mode, "")
		if dispatchResult.ToolCallID == "" {
			dispatchResult.ToolCallID = tc.ID
		}
		if strings.TrimSpace(dispatchResult.Metadata.Name) == "" {
			dispatchResult.Metadata = toolMetadataForToolCall(compileCtx.AssembledTools, tc)
		}
		if dispatchResult.Blocked {
			updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusBlocked, dispatchResult.Reason)
			k.markTurnBlocked(session, snapshot, tc, dispatchResult)
			return blockedTurnResult(session, snapshot, dispatchResult.Reason), nil
		}
		if dispatchResult.Error != "" {
			if !shouldFeedToolFailureBackToModel(dispatchResult) {
				updateAgentItem(snapshot, toolItemID, agentstate.ItemStatusFailed, dispatchResult.Error)
				appendAgentItem(snapshot, newAgentItem(errorItemID(snapshot.ID, snapshot.Iteration), agentstate.TurnItemTypeError, agentstate.ItemStatusFailed, dispatchResult.Error, map[string]string{"toolCallId": tc.ID, "toolName": tc.Name}))
				k.markTurnFailed(session, snapshot, tc, dispatchResult)
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
			payload["outputPreview"] = append(json.RawMessage(nil), result.Display.Data...)
		}
	}
	return payload
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

func (k *EinoKernel) markSnapshotResuming(session *SessionState, snapshot *TurnSnapshot, checkpointKind string) {
	if session == nil || snapshot == nil {
		return
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
		WithHooks(k.hooks).
		WithObserver(k.runtimeObserver()).
		WithProgressSink(k.progressSink(session, snapshot, iteration))
	return dispatcher
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
			return nil, fmt.Errorf("empty model response: provider returned no assistant content or tool calls")
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
	budget := mergeResultBudget(meta.EffectiveResultBudget(defaultMaxInlineResultBytes), toolResult.ResultBudget, defaultMaxInlineResultBytes)
	appendExternalReferences(&result.ExternalReferences, externalReferencesFromToolResultRefs(session, snapshot, iteration, result.References)...)

	if toolResult.HasSpill() {
		ref, err := k.persistToolResultSpill(session, snapshot, iteration, tc, meta, toolResult.Spill)
		if err != nil {
			return ToolResult{}, err
		}
		result.Spilled = true
		result.Summary = fallbackSummary(toolResult.Spill.Summary, toolResult.Content, budget.MaxInlineResultBytes)
		tier := classifyToolResultTier(spillContentBytes(toolResult.Spill, toolResult.Content), budget)
		result.Content = materializedInlineContent(toolResult.Content, result.Summary, ref, budget, tier)
		result.References = appendToolResultReferences(result.References, toolResultReferenceFromExternalRef(ref))
		appendExternalReferences(&result.ExternalReferences, ref)
		return result, nil
	}

	inlineBytes := len(toolResult.Content)
	tier := classifyToolResultTier(inlineBytes, budget)
	if tier == toolResultTierSmall {
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
	switch ref.Kind {
	case ToolResultReferenceKindBlob:
		if ref.URI != "" {
			return string(ref.Kind) + ":" + ref.URI
		}
	case ToolResultReferenceKindCard:
		if ref.CardRef != "" {
			return string(ref.Kind) + ":" + ref.CardRef
		}
	case ToolResultReferenceKindFile:
		if ref.FilePath != "" {
			return string(ref.Kind) + ":" + ref.FilePath
		}
	}
	return ""
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

func (k *EinoKernel) markTurnBlocked(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, result DispatchResult) {
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
			Status:     "pending",
			CreatedAt:  now,
			UpdatedAt:  now,
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
}

func (k *EinoKernel) markTurnFailed(session *SessionState, snapshot *TurnSnapshot, tc ToolCall, result DispatchResult) {
	if session == nil || snapshot == nil {
		return
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
}

func (k *EinoKernel) markTurnFailedFromError(session *SessionState, snapshot *TurnSnapshot, err error, checkpointKind string) {
	if session == nil || snapshot == nil || snapshot.Lifecycle.IsTerminal() {
		return
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
	delta.TemporarilyUnavailable = mergeStringSets(diffStrings(previous.VisibleTools, current), snapshot.HiddenTools)
	return delta
}

func buildProtocolPromptState(snapshot *TurnSnapshot, delta promptcompiler.ToolPromptDelta, approvals []PendingApproval, evidence []PendingEvidence) promptcompiler.ProtocolPromptState {
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
