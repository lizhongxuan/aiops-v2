package runtimekernel

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ---------------------------------------------------------------------------
// Workspace Request Router — implements three-way request routing for
// Workspace sessions (Requirement 9.1):
//
//  1. State queries → read directly from projection (no agent execution)
//  2. Single-host readonly → complete in current turn (ChatModelAgent)
//  3. Complex tasks → route through PlanExecuteAgent (Plan-Execute-Replan)
// ---------------------------------------------------------------------------

// RequestCategory identifies the routing category for a workspace request.
type RequestCategory string

const (
	// CategoryStateQuery is for status/state questions that can be answered
	// directly from the projection layer without agent execution.
	CategoryStateQuery RequestCategory = "state_query"

	// CategorySingleHostReadonly is for single-host readonly operations
	// that can be completed within the current turn using a ChatModelAgent.
	CategorySingleHostReadonly RequestCategory = "single_host_readonly"

	// CategoryComplexTask is for complex multi-host tasks that require
	// PlanExecuteAgent with Plan-Execute-Replan flow.
	CategoryComplexTask RequestCategory = "complex_task"
)

// AllRequestCategories returns all valid request categories.
func AllRequestCategories() []RequestCategory {
	return []RequestCategory{
		CategoryStateQuery,
		CategorySingleHostReadonly,
		CategoryComplexTask,
	}
}

// IsValid reports whether the category is one of the canonical values.
func (c RequestCategory) IsValid() bool {
	switch c {
	case CategoryStateQuery, CategorySingleHostReadonly, CategoryComplexTask:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// WorkspaceRouter classifies incoming workspace requests and routes them
// to the appropriate execution path.
// ---------------------------------------------------------------------------

// WorkspaceRouter classifies and routes workspace session requests.
type WorkspaceRouter struct {
	projector ProjectionReader
}

// ProjectionReader provides read access to the projection layer for state queries.
type ProjectionReader interface {
	// ReadState returns the current projected state for the given session.
	ReadState(sessionID string) (string, error)
}

// NewWorkspaceRouter creates a new WorkspaceRouter.
func NewWorkspaceRouter(projector ProjectionReader) *WorkspaceRouter {
	return &WorkspaceRouter{
		projector: projector,
	}
}

// RoutingDecision contains the classification result and routing metadata.
type RoutingDecision struct {
	Category    RequestCategory `json:"category"`
	Reason      string          `json:"reason"`
	TargetHosts []string        `json:"targetHosts,omitempty"`
}

// ClassifyRequest analyzes a workspace TurnRequest and determines the
// appropriate routing category based on the request content and context.
//
// Classification rules:
//   - State queries: questions about current status, state, or projection data
//   - Single-host readonly: operations targeting one host with readonly intent
//   - Complex tasks: multi-host operations, mutations, or tasks requiring planning
func (wr *WorkspaceRouter) ClassifyRequest(req TurnRequest) RoutingDecision {
	input := strings.TrimSpace(req.Input)
	inputLower := strings.ToLower(input)

	// Rule 1: State queries — questions about status/state that can be
	// answered from projection without agent execution.
	if isStateQuery(inputLower) {
		return RoutingDecision{
			Category: CategoryStateQuery,
			Reason:   "request is a state/status query answerable from projection",
		}
	}

	// Rule 2: Single-host readonly — targets exactly one host with readonly intent.
	if req.HostID != "" && isReadonlyIntent(inputLower, req.Mode) {
		return RoutingDecision{
			Category:    CategorySingleHostReadonly,
			Reason:      "single-host readonly operation completable in current turn",
			TargetHosts: []string{req.HostID},
		}
	}

	// Rule 3: Complex task — multi-host, mutation, or requires planning.
	targetHosts := extractTargetHosts(req)
	return RoutingDecision{
		Category:    CategoryComplexTask,
		Reason:      "complex task requiring PlanExecuteAgent orchestration",
		TargetHosts: targetHosts,
	}
}

// RouteRequest executes the workspace request according to its classification.
// Returns the TurnResult from the appropriate execution path.
func (wr *WorkspaceRouter) RouteRequest(ctx context.Context, req TurnRequest, kernel *EinoKernel) (TurnResult, error) {
	decision := wr.ClassifyRequest(req)

	switch decision.Category {
	case CategoryStateQuery:
		return wr.handleStateQuery(ctx, req, kernel)
	case CategorySingleHostReadonly:
		return wr.handleSingleHostReadonly(ctx, req, kernel)
	case CategoryComplexTask:
		return wr.handleComplexTask(ctx, req, kernel, decision)
	default:
		return TurnResult{}, fmt.Errorf("unknown request category: %s", decision.Category)
	}
}

// ---------------------------------------------------------------------------
// Route handlers
// ---------------------------------------------------------------------------

// handleStateQuery reads directly from the projection layer without
// executing any agent. This is the fastest path for status questions.
func (wr *WorkspaceRouter) handleStateQuery(ctx context.Context, req TurnRequest, kernel *EinoKernel) (TurnResult, error) {
	_ = ctx // projection read is synchronous

	turnID := req.TurnID
	if turnID == "" {
		turnID = fmt.Sprintf("turn-%d", time.Now().UnixNano())
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = fmt.Sprintf("sess-%d", time.Now().UnixNano())
	}

	// Read state from projection
	var output string
	if wr.projector != nil {
		state, err := wr.projector.ReadState(sessionID)
		if err != nil {
			return TurnResult{}, fmt.Errorf("read projection state: %w", err)
		}
		output = state
	}

	// Emit turn complete event
	kernel.projector.Emit(LifecycleEvent{
		Type:      EventTurnComplete,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: time.Now(),
	})

	return TurnResult{
		SessionType: req.SessionType,
		Mode:        req.Mode,
		SessionID:   sessionID,
		TurnID:      turnID,
		Status:      "completed",
		Output:      output,
	}, nil
}

// handleSingleHostReadonly completes the request in the current turn using
// the shared runtime iteration loop.
func (wr *WorkspaceRouter) handleSingleHostReadonly(ctx context.Context, req TurnRequest, kernel *EinoKernel) (TurnResult, error) {
	return kernel.RunTurn(ctx, req)
}

// handleComplexTask routes the request through the same runtime iteration
// loop so workspace sessions do not fork into a second orchestration chain.
func (wr *WorkspaceRouter) handleComplexTask(ctx context.Context, req TurnRequest, kernel *EinoKernel, decision RoutingDecision) (TurnResult, error) {
	_ = decision
	return kernel.RunTurn(ctx, req)
}

// ---------------------------------------------------------------------------
// Classification helpers
// ---------------------------------------------------------------------------

// stateQueryPatterns are keywords/phrases that indicate a state query.
var stateQueryPatterns = []string{
	"状态", "status", "state",
	"当前", "current",
	"查看", "show", "display",
	"列出", "list",
	"有哪些", "what are",
	"多少", "how many",
	"运行中", "running",
	"在线", "online",
	"离线", "offline",
}

// isStateQuery checks if the input is a state/status query that can be
// answered directly from the projection layer.
func isStateQuery(inputLower string) bool {
	// Must contain a state-related keyword
	hasStateKeyword := false
	for _, pattern := range stateQueryPatterns {
		if strings.Contains(inputLower, pattern) {
			hasStateKeyword = true
			break
		}
	}
	if !hasStateKeyword {
		return false
	}

	// Must NOT contain action/mutation keywords
	actionKeywords := []string{
		"执行", "execute",
		"清理", "clean", "delete", "remove",
		"修复", "fix", "repair",
		"部署", "deploy",
		"安装", "install",
		"更新", "update", "upgrade",
		"停止", "stop", "kill",
		"restart",
	}
	for _, kw := range actionKeywords {
		if strings.Contains(inputLower, kw) {
			return false
		}
	}

	return true
}

// readonlyIntentKeywords indicate readonly operations.
var readonlyIntentKeywords = []string{
	"查看", "check", "inspect", "look", "show",
	"读取", "read", "get", "fetch",
	"日志", "log", "logs",
	"监控", "monitor",
	"诊断", "diagnose",
	"分析", "analyze",
}

// isReadonlyIntent checks if the request has readonly intent.
func isReadonlyIntent(inputLower string, mode Mode) bool {
	// In chat or inspect mode, default to readonly
	if mode == ModeChat || mode == ModeInspect {
		return true
	}

	// Check for readonly keywords
	for _, kw := range readonlyIntentKeywords {
		if strings.Contains(inputLower, kw) {
			return true
		}
	}

	return false
}

// extractTargetHosts extracts target host IDs from the request.
func extractTargetHosts(req TurnRequest) []string {
	if req.HostID != "" {
		return []string{req.HostID}
	}
	// In production, this would parse the input to identify mentioned hosts.
	// For now, return empty (PlanExecuteAgent's Planner will determine targets).
	return nil
}

// classifyTaskType determines the WorkspaceTask type from the routing decision.
func classifyTaskType(decision RoutingDecision) string {
	if len(decision.TargetHosts) > 1 {
		return "multi_host"
	}
	if len(decision.TargetHosts) == 1 {
		return "host_exec"
	}
	return "plan"
}
