package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/cloudwego/eino/components/tool"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptcompiler"
)

// ---------------------------------------------------------------------------
// CapabilitySource is the interface that the EinoKernel uses to access
// assembled tools without importing the capability package directly (avoids
// circular imports since capability imports runtimekernel).
// ---------------------------------------------------------------------------

// CapabilitySource provides tool-assembly context and tool pool assembly.
// Implemented by *capability.Registry via an adapter.
type CapabilitySource interface {
	// CompileContext returns a CompileContext with assembled tools populated.
	CompileContext(session SessionType, mode Mode) promptcompiler.CompileContext

	// AssembleToolPool returns Eino tool.BaseTool instances for the given session/mode.
	// These can be directly passed to adk.ChatModelAgent's ToolsConfig.
	AssembleToolPool(session SessionType, mode Mode) []tool.BaseTool
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
	registry    CapabilitySource
	compiler    promptcompiler.Compiler
	policy      *policyengine.Engine
	projector   EventEmitter
	modelRouter *modelrouter.Router
	sessions    *SessionManager
	agentMgr    AgentManagerSource
	spanSource  SpanStreamSource // optional: span tree integration for conversation tracking
}

// EinoKernelConfig holds the dependencies for creating an EinoKernel.
type EinoKernelConfig struct {
	Registry    CapabilitySource
	Compiler    promptcompiler.Compiler
	Policy      *policyengine.Engine
	Projector   EventEmitter
	ModelRouter *modelrouter.Router
	AgentMgr    AgentManagerSource
	SpanSource  SpanStreamSource // optional: if nil, span tracking is disabled
}

// NewEinoKernel creates a new EinoKernel with the given dependencies.
func NewEinoKernel(cfg EinoKernelConfig) *EinoKernel {
	return &EinoKernel{
		registry:    cfg.Registry,
		compiler:    cfg.Compiler,
		policy:      cfg.Policy,
		projector:   cfg.Projector,
		modelRouter: cfg.ModelRouter,
		sessions:    NewSessionManager(),
		agentMgr:    cfg.AgentMgr,
		spanSource:  cfg.SpanSource,
	}
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
//  5. Get assembled tools from Registry
//  6. Get model from ModelRouter
//  7. Create adk.ChatModelAgent with model + tools + instruction
//  8. Execute via adk.Runner.Run()
//  9. Process AgentEvents via callback → Projection
//  10. Final gate check via PolicyEngine.CompletionPolicy
//  11. Return TurnResult
func (k *EinoKernel) RunTurn(ctx context.Context, req TurnRequest) (result TurnResult, err error) {
	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			result = TurnResult{
				SessionType: req.SessionType,
				Mode:        req.Mode,
				SessionID:   req.SessionID,
				TurnID:      req.TurnID,
				Status:      "error",
				Error:       fmt.Sprintf("panic recovered: %v", r),
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

	// Step 2: Assemble context (add user message, trim if needed)
	if req.Input != "" {
		msg := Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			Role:      "user",
			Content:   req.Input,
			Timestamp: time.Now(),
		}
		session.Messages = append(session.Messages, msg)
	}
	TrimContext(&session.Context, session.Messages)

	// Step 3: Compile prompt via PromptCompiler
	compileCtx := k.registry.CompileContext(req.SessionType, req.Mode)
	compiled, compileErr := k.compiler.Compile(compileCtx)
	if compileErr != nil {
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, compileErr.Error())
		}
		return TurnResult{}, fmt.Errorf("compile prompt: %w", compileErr)
	}

	// Step 4: Get assembled tools for execution
	toolPool := k.registry.AssembleToolPool(req.SessionType, req.Mode)

	// Step 5: Get model from ModelRouter
	agentKind := modelrouter.AgentKindWorker
	if req.SessionType == SessionTypeWorkspace {
		agentKind = modelrouter.AgentKindPlanner
	}
	chatModel, modelErr := k.modelRouter.GetModel(agentKind, modelrouter.ProviderConfig{})
	if modelErr != nil {
		if k.spanSource != nil && turnSpanID != "" {
			k.spanSource.FailSpan(turnSpanID, modelErr.Error())
		}
		return TurnResult{}, fmt.Errorf("get model: %w", modelErr)
	}

	// Step 6: Execute agent via model
	// For workspace sessions with AgentManager available, use AgentFactory
	// to create a PlanExecuteAgent and route through AgentManager.
	var agentOutput string
	if req.SessionType == SessionTypeWorkspace && k.agentMgr != nil {
		wsOutput, wsErr := k.runWorkspaceAgent(ctx, req, session, turnID)
		if wsErr != nil {
			if k.spanSource != nil && turnSpanID != "" {
				k.spanSource.FailSpan(turnSpanID, wsErr.Error())
			}
			return TurnResult{}, fmt.Errorf("run workspace agent: %w", wsErr)
		}
		agentOutput = wsOutput
	} else {
		var runErr error
		agentOutput, runErr = executeAgent(ctx, chatModel, compiled, toolPool, session.Messages)
		if runErr != nil {
			if k.spanSource != nil && turnSpanID != "" {
				k.spanSource.FailSpan(turnSpanID, runErr.Error())
			}
			return TurnResult{}, fmt.Errorf("run agent: %w", runErr)
		}
	}

	// Step 7: Emit projection events
	k.projector.Emit(LifecycleEvent{
		Type:      EventTurnComplete,
		SessionID: session.ID,
		TurnID:    turnID,
		Timestamp: time.Now(),
	})

	// Step 8: Final gate check via PolicyEngine.CompletionPolicy
	if k.policy.CompletionPolicy != nil {
		turnState := policyengine.TurnState{
			SessionID: session.ID,
			TurnID:    turnID,
			Completed: true,
		}
		decision := k.policy.CompletionPolicy.CheckCompletion(ctx, turnState)
		if decision.Action != policyengine.PolicyActionAllow {
			if k.spanSource != nil && turnSpanID != "" {
				k.spanSource.FailSpan(turnSpanID, "blocked: "+decision.Reason)
			}
			return TurnResult{
				SessionType: req.SessionType,
				Mode:        req.Mode,
				SessionID:   session.ID,
				TurnID:      turnID,
				Status:      "blocked",
				Error:       decision.Reason,
			}, nil
		}
	}

	// Complete the turn span on success
	if k.spanSource != nil && turnSpanID != "" {
		summary := "Turn completed"
		if agentOutput != "" {
			summary = truncateString(agentOutput, 100)
		}
		k.spanSource.CompleteSpan(turnSpanID, summary, agentOutput)
	}

	// Append assistant message to session
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
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   session.ID,
		TurnID:      turnID,
		Status:      "completed",
		Output:      agentOutput,
	}, nil
}

// ---------------------------------------------------------------------------
// ResumeTurn resumes a turn that was interrupted (e.g. by approval).
// Uses adk.Runner.Resume via checkpoint store.
// ---------------------------------------------------------------------------

// ResumeTurn resumes a turn that was interrupted by approval or user input.
func (k *EinoKernel) ResumeTurn(_ context.Context, req ResumeRequest) (TurnResult, error) {
	if err := req.Validate(); err != nil {
		return TurnResult{}, fmt.Errorf("invalid resume request: %w", err)
	}

	session := k.sessions.Get(req.SessionID)
	if session == nil {
		return TurnResult{}, fmt.Errorf("session %q not found", req.SessionID)
	}

	// In production, this would call adk.Runner.Resume() with the checkpoint.
	// For now, return a stub result indicating the turn was resumed.
	return TurnResult{
		SessionType: session.Type,
		Mode:        session.Mode,
		SessionID:   session.ID,
		TurnID:      req.TurnID,
		Status:      "completed",
		Output:      "turn resumed",
	}, nil
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

	// In production, this would cancel the adk.Runner context.
	return TurnResult{
		SessionType: session.Type,
		Mode:        session.Mode,
		SessionID:   session.ID,
		TurnID:      req.TurnID,
		Status:      "cancelled",
	}, nil
}

// RunTurnWithRecorder executes RunTurn while recording pipeline steps for testing.
func (k *EinoKernel) RunTurnWithRecorder(ctx context.Context, req TurnRequest, recorder *PipelineRecorder) (result TurnResult, err error) {
	// Panic recovery
	defer func() {
		if r := recover(); r != nil {
			result = TurnResult{
				SessionType: req.SessionType,
				Mode:        req.Mode,
				SessionID:   req.SessionID,
				TurnID:      req.TurnID,
				Status:      "error",
				Error:       fmt.Sprintf("panic recovered: %v", r),
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
	if req.Input != "" {
		msg := Message{
			ID:        fmt.Sprintf("msg-%d", time.Now().UnixNano()),
			Role:      "user",
			Content:   req.Input,
			Timestamp: time.Now(),
		}
		session.Messages = append(session.Messages, msg)
	}
	TrimContext(&session.Context, session.Messages)

	// Step 2: Compile prompt
	recorder.Record(StepCompilePrompt)
	compileCtx := k.registry.CompileContext(req.SessionType, req.Mode)
	compiled, compileErr := k.compiler.Compile(compileCtx)
	if compileErr != nil {
		return TurnResult{}, fmt.Errorf("compile prompt: %w", compileErr)
	}

	// Step 3: Assemble tools
	recorder.Record(StepAssembleTools)
	toolPool := k.registry.AssembleToolPool(req.SessionType, req.Mode)

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
	k.projector.Emit(LifecycleEvent{
		Type:      EventTurnComplete,
		SessionID: session.ID,
		TurnID:    turnID,
		Timestamp: time.Now(),
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
				SessionType: req.SessionType,
				Mode:        req.Mode,
				SessionID:   session.ID,
				TurnID:      turnID,
				Status:      "blocked",
				Error:       decision.Reason,
			}, nil
		}
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
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   session.ID,
		TurnID:      turnID,
		Status:      "completed",
		Output:      agentOutput,
	}, nil
}

// ---------------------------------------------------------------------------
// runWorkspaceAgent — workspace session execution via AgentManager.
//
// Uses AgentManagerSource to create a PlanExecuteAgent config, spawn and run
// the planner agent, and project worker results through the Projection layer.
// ---------------------------------------------------------------------------

// runWorkspaceAgent executes a workspace session turn through the AgentManager.
// It creates a PlanExecuteAgent via AgentFactory, runs the planner, and
// projects worker agent results to the frontend via Projection events.
func (k *EinoKernel) runWorkspaceAgent(ctx context.Context, req TurnRequest, session *SessionState, turnID string) (string, error) {
	// Derive missionID from session ID (workspace sessions map 1:1 to missions).
	missionID := session.ID

	// Create workspace agent config via AgentFactory.CreateWorkspaceAgent.
	if err := k.agentMgr.CreateWorkspaceAgent(ctx, missionID); err != nil {
		return "", fmt.Errorf("create workspace agent: %w", err)
	}

	// Spawn and run the planner agent via AgentManager.
	output, err := k.agentMgr.SpawnAndRunPlanner(ctx, missionID, session.ID, req.Input)
	if err != nil {
		return "", fmt.Errorf("run planner agent: %w", err)
	}

	// Collect all worker results for this mission and project them.
	workerResults := k.agentMgr.CollectResults(missionID)
	k.projectWorkerResults(workerResults, session.ID, turnID)

	return output, nil
}

// projectWorkerResults emits projection events for each worker agent result,
// making worker execution outcomes visible to the frontend via the Projection layer.
func (k *EinoKernel) projectWorkerResults(results []AgentResult, sessionID, turnID string) {
	for _, r := range results {
		// Build payload with worker result details.
		payload := map[string]interface{}{
			"agentId":  r.AgentID,
			"hostId":   r.HostID,
			"status":   r.Status,
			"output":   r.Output,
			"error":    r.Error,
			"duration": r.DurationMs,
		}
		payloadBytes, _ := json.Marshal(payload)

		// Determine event type based on worker status.
		eventType := EventToolCompleted
		if r.Status == "failed" {
			eventType = EventToolFailed
		}

		k.projector.Emit(LifecycleEvent{
			Type:      eventType,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: time.Now(),
			Payload:   payloadBytes,
		})
	}
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
