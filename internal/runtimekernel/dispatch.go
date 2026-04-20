package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aiops-v2/internal/policyengine"
)

// ---------------------------------------------------------------------------
// ToolDispatcher handles tool call dispatch with PolicyEngine integration.
// In production, this integrates with adk.Runner's tool execution callback.
// ---------------------------------------------------------------------------

// ToolLookup is the interface for looking up tools in the registry.
// This avoids circular imports with the capability package.
type ToolLookup interface {
	// LookupTool returns the tool kind, and executor for a given tool call name.
	LookupTool(name string) (kind string, executor ToolExecutor, found bool)
}

// ToolExecutor executes a tool with the given arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, args json.RawMessage) (content string, err error)
}

// EventEmitter is the interface for emitting lifecycle events.
// Implemented by *projection.Projector.
type EventEmitter interface {
	Emit(event LifecycleEvent)
}

// ToolDispatcher dispatches tool calls through the PolicyEngine and
// Capability Registry, emitting lifecycle events to the Projector.
type ToolDispatcher struct {
	lookup     ToolLookup
	policy     *policyengine.Engine
	projector  EventEmitter
	spanSource SpanStreamSource // optional: span tracking for tool calls
}

// NewToolDispatcher creates a new ToolDispatcher.
func NewToolDispatcher(lookup ToolLookup, policy *policyengine.Engine, projector EventEmitter) *ToolDispatcher {
	return &ToolDispatcher{
		lookup:    lookup,
		policy:    policy,
		projector: projector,
	}
}

// NewToolDispatcherWithSpans creates a ToolDispatcher with span tracking enabled.
func NewToolDispatcherWithSpans(lookup ToolLookup, policy *policyengine.Engine, projector EventEmitter, spanSource SpanStreamSource) *ToolDispatcher {
	return &ToolDispatcher{
		lookup:     lookup,
		policy:     policy,
		projector:  projector,
		spanSource: spanSource,
	}
}

// DispatchResult is the outcome of a tool dispatch.
type DispatchResult struct {
	ToolCallID string
	Content    string
	Error      string
	Blocked    bool
	Reason     string
}

// Dispatch executes a tool call through the policy pipeline:
//  1. Look up tool in registry
//  2. Check PolicyEngine (Allow/Deny/NeedApproval)
//  3. If allowed, execute the tool
//  4. Emit lifecycle events to Projector
//  5. Return result
func (d *ToolDispatcher) Dispatch(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode) DispatchResult {
	return d.DispatchWithParentSpan(ctx, sessionID, turnID, tc, sessionType, mode, "")
}

// DispatchWithParentSpan executes a tool call with optional span tracking.
// If parentSpanID is non-empty and spanSource is configured, a child span
// is created for this tool call.
func (d *ToolDispatcher) DispatchWithParentSpan(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode, parentSpanID string) DispatchResult {
	// Create tool span if span source is available
	var toolSpanID string
	if d.spanSource != nil && parentSpanID != "" {
		toolSpanID = d.spanSource.StartToolSpan(parentSpanID, tc.Name)
	}

	// Emit tool.started event
	startPayload, _ := json.Marshal(map[string]string{
		"id":       tc.ID,
		"toolName": tc.Name,
	})
	d.projector.Emit(LifecycleEvent{
		Type:      EventToolStarted,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   startPayload,
	})

	// Look up tool in registry
	kind, executor, found := d.lookup.LookupTool(tc.Name)
	if !found {
		errResult := d.emitToolFailed(sessionID, turnID, tc, "tool not found: "+tc.Name)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "tool not found: "+tc.Name)
		}
		return errResult
	}

	// Check PolicyEngine
	policyInput := policyengine.NewPolicyInput(tc.Name, kind, string(sessionType), string(mode), tc.Arguments)
	decision := d.policy.CheckToolCall(ctx, policyInput)

	switch decision.Action {
	case policyengine.PolicyActionDeny:
		result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+decision.Reason)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "denied: "+decision.Reason)
		}
		return result

	case policyengine.PolicyActionNeedApproval:
		// In production, this would trigger adk.Runner interrupt/resume
		// via compose.CheckPointStore. For now, return blocked status.
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "awaiting approval: "+decision.Reason)
		}
		return DispatchResult{
			ToolCallID: tc.ID,
			Blocked:    true,
			Reason:     decision.Reason,
		}

	case policyengine.PolicyActionNeedEvidence:
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "evidence required: "+decision.Reason)
		}
		return DispatchResult{
			ToolCallID: tc.ID,
			Blocked:    true,
			Reason:     "evidence required: " + decision.Reason,
		}
	}

	// Execute tool
	if executor == nil {
		result := d.emitToolFailed(sessionID, turnID, tc, "tool has no runtime implementation")
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "tool has no runtime implementation")
		}
		return result
	}

	content, execErr := executor.Execute(ctx, tc.Arguments)
	if execErr != nil {
		result := d.emitToolFailed(sessionID, turnID, tc, execErr.Error())
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, execErr.Error())
		}
		return result
	}

	// Emit tool.completed event
	completedPayload, _ := json.Marshal(map[string]string{
		"id":       tc.ID,
		"toolName": tc.Name,
		"result":   content,
	})
	d.projector.Emit(LifecycleEvent{
		Type:      EventToolCompleted,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   completedPayload,
	})

	// Complete the tool span
	if d.spanSource != nil && toolSpanID != "" {
		summary := fmt.Sprintf("%s completed", tc.Name)
		d.spanSource.CompleteSpan(toolSpanID, summary, content)
	}

	return DispatchResult{
		ToolCallID: tc.ID,
		Content:    content,
	}
}

// emitToolFailed emits a tool.failed event and returns an error DispatchResult.
func (d *ToolDispatcher) emitToolFailed(sessionID, turnID string, tc ToolCall, errMsg string) DispatchResult {
	failPayload, _ := json.Marshal(map[string]string{
		"id":       tc.ID,
		"toolName": tc.Name,
		"error":    errMsg,
	})
	d.projector.Emit(LifecycleEvent{
		Type:      EventToolFailed,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   failPayload,
	})
	return DispatchResult{
		ToolCallID: tc.ID,
		Error:      errMsg,
	}
}

// ---------------------------------------------------------------------------
// RunnerCallback — bridges adk.Runner events to Projection layer.
// In production, this implements the adk.RunnerCallback interface.
// ---------------------------------------------------------------------------

// RunnerCallback bridges agent execution events to the Projection layer.
// It receives events from adk.Runner and emits corresponding LifecycleEvents.
type RunnerCallback struct {
	sessionID string
	turnID    string
	projector EventEmitter
}

// NewRunnerCallback creates a RunnerCallback for the given session/turn.
func NewRunnerCallback(sessionID, turnID string, projector EventEmitter) *RunnerCallback {
	return &RunnerCallback{
		sessionID: sessionID,
		turnID:    turnID,
		projector: projector,
	}
}

// OnToolStart is called when a tool execution begins.
func (cb *RunnerCallback) OnToolStart(toolName string, args json.RawMessage) {
	payload, _ := json.Marshal(map[string]interface{}{
		"toolName": toolName,
		"args":     args,
	})
	cb.projector.Emit(LifecycleEvent{
		Type:      EventToolStarted,
		SessionID: cb.sessionID,
		TurnID:    cb.turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// OnToolComplete is called when a tool execution completes successfully.
func (cb *RunnerCallback) OnToolComplete(toolName, result string) {
	payload, _ := json.Marshal(map[string]string{
		"toolName": toolName,
		"result":   result,
	})
	cb.projector.Emit(LifecycleEvent{
		Type:      EventToolCompleted,
		SessionID: cb.sessionID,
		TurnID:    cb.turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}

// OnToolFailed is called when a tool execution fails.
func (cb *RunnerCallback) OnToolFailed(toolName string, err error) {
	payload, _ := json.Marshal(map[string]string{
		"toolName": toolName,
		"error":    fmt.Sprintf("%v", err),
	})
	cb.projector.Emit(LifecycleEvent{
		Type:      EventToolFailed,
		SessionID: cb.sessionID,
		TurnID:    cb.turnID,
		Timestamp: time.Now(),
		Payload:   payload,
	})
}
