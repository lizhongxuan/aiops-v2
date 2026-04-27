package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/spanstream"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// ToolDispatcher handles tool call dispatch with PolicyEngine integration.
// In production, this integrates with adk.Runner's tool execution callback.
// ---------------------------------------------------------------------------

// ToolLookup is the interface for looking up tools in the registry.
// This avoids circular imports with the capability package.
type ToolLookup interface {
	// LookupTool returns the tool descriptor and executor for a given tool call name.
	LookupTool(name string) (desc ToolDescriptor, executor ToolExecutor, found bool)
}

// ToolExecutor executes a tool with the given arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, args json.RawMessage) (result tooling.ToolResult, err error)
}

// ToolDescriptor carries the unified metadata used across policy, hooks, and permissions.
type ToolDescriptor struct {
	Metadata tooling.ToolMetadata
}

// EventEmitter is the interface for emitting lifecycle events.
// Implemented by *projection.Projector.
type EventEmitter interface {
	Emit(event LifecycleEvent)
}

// ToolProgressSink receives incremental progress updates from a long-running
// tool so runtime state can checkpoint them immediately.
type ToolProgressSink func(update ToolProgressUpdate)

// ToolDispatcher dispatches tool calls through the PolicyEngine and
// Capability Registry, emitting lifecycle events to the Projector.
type ToolDispatcher struct {
	lookup       ToolLookup
	policy       *policyengine.Engine
	permissions  *permissions.Engine
	hooks        *hooks.Registry
	projector    EventEmitter
	spanSource   SpanStreamSource // optional: span tracking for tool calls
	progressSink ToolProgressSink
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

// WithPermissions attaches a tool-scoped permission engine to the dispatcher.
func (d *ToolDispatcher) WithPermissions(engine *permissions.Engine) *ToolDispatcher {
	d.permissions = engine
	return d
}

// WithHooks attaches a lifecycle hook registry to the dispatcher.
func (d *ToolDispatcher) WithHooks(registry *hooks.Registry) *ToolDispatcher {
	d.hooks = registry
	return d
}

// WithProgressSink attaches an incremental progress sink to the dispatcher.
func (d *ToolDispatcher) WithProgressSink(sink ToolProgressSink) *ToolDispatcher {
	d.progressSink = sink
	return d
}

// DispatchResult is the outcome of a tool dispatch.
type DispatchResult struct {
	ToolCallID  string
	Content     string
	Error       string
	Blocked     bool
	Reason      string
	Metadata    tooling.ToolMetadata
	Result      tooling.ToolResult
	Outcome     string
	Source      string
	HiddenTools []string
}

// Dispatch executes a tool call through the policy pipeline:
//  1. Look up tool in registry
//  2. Check PolicyEngine (Allow/Deny/NeedApproval)
//  3. If allowed, execute the tool
//  4. Emit lifecycle events to Projector
//  5. Return result
func (d *ToolDispatcher) Dispatch(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode) DispatchResult {
	return d.dispatch(ctx, sessionID, turnID, tc, sessionType, mode, "", false)
}

// DispatchApproved executes a tool call after an explicit approval/resume decision.
// The tool still flows through the dispatcher, but guard checks are skipped because
// the approval gate has already been satisfied for this call.
func (d *ToolDispatcher) DispatchApproved(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode) DispatchResult {
	return d.dispatch(ctx, sessionID, turnID, tc, sessionType, mode, "", true)
}

// DispatchWithParentSpan executes a tool call with optional span tracking.
// If parentSpanID is non-empty and spanSource is configured, a child span
// is created for this tool call.
func (d *ToolDispatcher) DispatchWithParentSpan(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode, parentSpanID string) DispatchResult {
	return d.dispatch(ctx, sessionID, turnID, tc, sessionType, mode, parentSpanID, false)
}

func (d *ToolDispatcher) dispatch(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode, parentSpanID string, approved bool) DispatchResult {
	// Create tool span if span source is available
	var toolSpanID string
	if d.spanSource != nil && parentSpanID != "" {
		toolSpanID = d.spanSource.StartToolSpan(parentSpanID, tc.Name)
	}

	// Look up tool in registry
	desc, executor, found := d.lookup.LookupTool(tc.Name)
	if !found {
		errResult := d.emitToolFailed(sessionID, turnID, tc, "tool not found: "+tc.Name, "runtime", "tool_failed")
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "tool not found: "+tc.Name)
		}
		return errResult
	}

	toolEvent := hooks.ToolEvent{
		ToolCallID:  tc.ID,
		SessionID:   sessionID,
		TurnID:      turnID,
		SessionType: string(sessionType),
		Mode:        string(mode),
		Tool:        desc.Metadata,
		Arguments:   tc.Arguments,
	}
	if d.hooks != nil {
		if err := d.hooks.RunToolStage(ctx, hooks.StagePreToolUse, &toolEvent); err != nil {
			result := d.emitToolFailed(sessionID, turnID, tc, "pre_tool_use: "+err.Error(), "hook", "tool_failed")
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "pre_tool_use: "+err.Error())
			}
			return result
		}
	}
	if len(toolEvent.UpdatedInput) > 0 {
		tc.Arguments = append(json.RawMessage(nil), toolEvent.UpdatedInput...)
		toolEvent.Arguments = tc.Arguments
	}

	// Check PolicyEngine
	if !approved && d.policy != nil {
		policyInput := policyengine.PolicyInput{
			ToolName:    tc.Name,
			Tool:        desc.Metadata,
			SessionType: string(sessionType),
			Mode:        string(mode),
			Arguments:   tc.Arguments,
		}
		decision := d.policy.CheckToolCall(ctx, policyInput)

		switch decision.Action {
		case policyengine.PolicyActionDeny:
			result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+decision.Reason, "policy", "tool_denied")
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
				Outcome:    "approval_needed",
				Source:     "policy",
			}

		case policyengine.PolicyActionNeedEvidence:
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "evidence required: "+decision.Reason)
			}
			return DispatchResult{
				ToolCallID: tc.ID,
				Blocked:    true,
				Reason:     "evidence required: " + decision.Reason,
				Outcome:    "evidence_needed",
				Source:     "policy",
			}
		}
	}

	if !approved {
		if override := toolEvent.UpdatedPermissions; override != nil {
			switch override.Action {
			case tooling.PermissionActionDeny:
				result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+override.Reason, "hook", "tool_denied")
				if d.spanSource != nil && toolSpanID != "" {
					d.spanSource.FailSpan(toolSpanID, "denied: "+override.Reason)
				}
				return result
			case tooling.PermissionActionNeedApproval:
				if d.spanSource != nil && toolSpanID != "" {
					d.spanSource.FailSpan(toolSpanID, "awaiting approval: "+override.Reason)
				}
				return DispatchResult{
					ToolCallID: tc.ID,
					Blocked:    true,
					Reason:     override.Reason,
					Outcome:    "approval_needed",
					Source:     "hook",
				}
			case tooling.PermissionActionNeedEvidence:
				if d.spanSource != nil && toolSpanID != "" {
					d.spanSource.FailSpan(toolSpanID, "evidence required: "+override.Reason)
				}
				return DispatchResult{
					ToolCallID: tc.ID,
					Blocked:    true,
					Reason:     "evidence required: " + override.Reason,
					Outcome:    "evidence_needed",
					Source:     "hook",
				}
			}
		}
	}

	if !approved && d.permissions != nil {
		decision := d.permissions.Decide(ctx, permissions.Request{
			Tool:        desc.Metadata,
			SessionType: string(sessionType),
			Mode:        string(mode),
			Arguments:   tc.Arguments,
		})
		switch decision.Action {
		case permissions.ActionDeny:
			result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+decision.Reason, "permissions", "tool_denied")
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "denied: "+decision.Reason)
			}
			return result
		case permissions.ActionAsk:
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "awaiting approval: "+decision.Reason)
			}
			return DispatchResult{
				ToolCallID: tc.ID,
				Blocked:    true,
				Reason:     decision.Reason,
				Outcome:    "approval_needed",
				Source:     "permissions",
			}
		}
	}

	// Emit tool.started only after permission and approval gates have passed.
	startPayload, _ := json.Marshal(map[string]any{
		"id":       tc.ID,
		"toolName": tc.Name,
		"args":     tc.Arguments,
	})
	d.projector.Emit(LifecycleEvent{
		Type:      EventToolStarted,
		SessionID: sessionID,
		TurnID:    turnID,
		Timestamp: time.Now(),
		Payload:   startPayload,
	})

	// Execute tool
	if executor == nil {
		result := d.emitToolFailed(sessionID, turnID, tc, "tool has no runtime implementation", "runtime", "tool_failed")
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "tool has no runtime implementation")
		}
		return result
	}

	toolResult, execErr := executor.Execute(ctx, tc.Arguments)
	if execErr != nil {
		result := d.emitToolFailed(sessionID, turnID, tc, execErr.Error(), "tool", "tool_failed")
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, execErr.Error())
		}
		return result
	}
	if toolResult.ToolCallID == "" {
		toolResult.ToolCallID = tc.ID
	}
	if toolResult.Error != "" {
		result := d.emitToolFailed(sessionID, turnID, tc, toolResult.Error, "tool", "tool_failed")
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, toolResult.Error)
		}
		return result
	}
	if toolResult.HasStream() {
		streamedResult, streamErr := d.consumeStreamingToolResult(sessionID, turnID, tc, toolSpanID, toolResult)
		if streamErr != nil {
			result := d.emitToolFailed(sessionID, turnID, tc, streamErr.Error(), "tool", "tool_failed")
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, streamErr.Error())
			}
			return result
		}
		toolResult = streamedResult
	}
	toolEvent.Arguments = tc.Arguments
	if d.hooks != nil {
		toolEvent.Result = &toolResult
		if err := d.hooks.RunToolStage(ctx, hooks.StagePostToolUse, &toolEvent); err != nil {
			result := d.emitToolFailed(sessionID, turnID, tc, "post_tool_use: "+err.Error(), "hook", "tool_failed")
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "post_tool_use: "+err.Error())
			}
			return result
		}
		if toolEvent.UpdatedMCPToolOutput != nil {
			toolResult = *toolEvent.UpdatedMCPToolOutput
			if toolResult.ToolCallID == "" {
				toolResult.ToolCallID = tc.ID
			}
			toolEvent.Result = &toolResult
		}
	}
	content := toolResult.Content
	outputSummary, resultForEvent, outputPreview, rawRef, resultBytes, resultTruncated := summarizeToolLifecycleResultForEvent(turnID, tc.ID, content)

	// Emit tool.completed event
	completedPayloadMap := map[string]any{
		"id":                tc.ID,
		"toolName":          tc.Name,
		"args":              tc.Arguments,
		"result":            resultForEvent,
		"outputSummary":     outputSummary,
		"rawRef":            rawRef,
		"resultBytes":       resultBytes,
		"resultTruncated":   resultTruncated,
		"additionalContext": append([]string(nil), toolEvent.AdditionalContext...),
		"watchPaths":        append([]string(nil), toolEvent.WatchPaths...),
		"hiddenTools":       append([]string(nil), toolEvent.HideTools...),
	}
	if len(outputPreview) > 0 {
		completedPayloadMap["outputPreview"] = outputPreview
	}
	completedPayload, _ := json.Marshal(completedPayloadMap)
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
		ToolCallID:  tc.ID,
		Content:     content,
		Metadata:    desc.Metadata,
		Result:      toolResult,
		Outcome:     "tool_result",
		Source:      "tool",
		HiddenTools: append([]string(nil), toolEvent.HideTools...),
	}
}

func (d *ToolDispatcher) consumeStreamingToolResult(sessionID, turnID string, tc ToolCall, toolSpanID string, result tooling.ToolResult) (tooling.ToolResult, error) {
	stream := result.Stream
	if stream == nil || stream.Reader == nil {
		return result, nil
	}

	emitProgress := func(delta string, totalRead int, done bool) {
		now := time.Now()
		update := ToolProgressUpdate{
			ToolCallID: tc.ID,
			ToolName:   tc.Name,
			Delta:      delta,
			TotalRead:  totalRead,
			Done:       done,
			Timestamp:  now,
		}
		if d.progressSink != nil {
			d.progressSink(update)
		}
		payload, _ := json.Marshal(map[string]any{
			"id":        tc.ID,
			"toolName":  tc.Name,
			"result":    delta,
			"totalRead": totalRead,
			"done":      done,
		})
		d.projector.Emit(LifecycleEvent{
			Type:      EventToolProgress,
			SessionID: sessionID,
			TurnID:    turnID,
			Timestamp: now,
			Payload:   payload,
		})
		if d.spanSource != nil && toolSpanID != "" && delta != "" {
			d.spanSource.EmitText(delta)
		}
	}

	op := spanstream.NewStreamingToolOperation(tc.Name, stream.Reader, stream.ChunkSize, func(chunk []byte, totalRead int) error {
		emitProgress(string(chunk), totalRead, false)
		return nil
	})
	content, err := op.Reader.ReadAll()
	if err != nil {
		return result, err
	}
	if result.Content == "" {
		result.Content = string(content)
	} else {
		result.Content += string(content)
	}
	result.Stream = nil
	emitProgress("", len(content), true)
	return result, nil
}

// emitToolFailed emits a tool.failed event and returns an error DispatchResult.
func (d *ToolDispatcher) emitToolFailed(sessionID, turnID string, tc ToolCall, errMsg string, source string, outcome string) DispatchResult {
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
		Outcome:    outcome,
		Source:     source,
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
	outputSummary, resultForEvent, outputPreview, rawRef, resultBytes, resultTruncated := summarizeToolLifecycleResultForEvent(cb.turnID, toolName, result)
	payloadMap := map[string]any{
		"toolName":        toolName,
		"result":          resultForEvent,
		"outputSummary":   outputSummary,
		"rawRef":          rawRef,
		"resultBytes":     resultBytes,
		"resultTruncated": resultTruncated,
	}
	if len(outputPreview) > 0 {
		payloadMap["outputPreview"] = outputPreview
	}
	payload, _ := json.Marshal(payloadMap)
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
