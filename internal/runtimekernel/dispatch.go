package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/runtimekernel/toolfailure"
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

type DeferredToolCatalogLookup interface {
	LookupDeferredTool(name string) (tooling.ToolMetadata, bool)
}

// ToolExecutor executes a tool with the given arguments.
type ToolExecutor interface {
	Execute(ctx context.Context, args json.RawMessage) (result tooling.ToolResult, err error)
}

// ToolPermissionChecker is implemented by tool executors that expose the
// unified tool-scoped permission gate in addition to their execution function.
type ToolPermissionChecker interface {
	CheckPermissions(ctx context.Context, args json.RawMessage) tooling.PermissionDecision
}

// ToolDescriptor carries the unified metadata used across policy, hooks, and permissions.
type ToolDescriptor struct {
	Metadata    tooling.ToolMetadata
	InputSchema json.RawMessage
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
	lookup             ToolLookup
	policy             *policyengine.Engine
	permissions        *permissions.Engine
	hooks              *hooks.Registry
	projector          EventEmitter
	spanSource         SpanStreamSource // optional: span tracking for tool calls
	observer           Observer
	progressSink       ToolProgressSink
	approvalGrants     []SessionApprovalGrant
	planMode           PlanModeState
	planScopes         []PlanApprovalScope
	unexpectedStates   []UnexpectedStateSignal
	resourceLockGate   ToolResourceLockGate
	toolSurfaceFP      string
	permissionHash     string
	surfacePolicy      *tooling.ToolSurfacePolicySnapshot
	runtimeToolSurface *RuntimeToolRouterSnapshot
	deferredCatalog    DeferredToolCatalogLookup
	visibleTools       []tooling.ToolMetadata
	retryConfig        ReadOnlyRetryConfig
	retryMu            sync.Mutex
	retriesThisTurn    int
}

// NewToolDispatcher creates a new ToolDispatcher.
func NewToolDispatcher(lookup ToolLookup, policy *policyengine.Engine, projector EventEmitter) *ToolDispatcher {
	return &ToolDispatcher{
		lookup:    lookup,
		policy:    policy,
		projector: projector,
		observer:  NoopObserver{},
	}
}

// NewToolDispatcherWithSpans creates a ToolDispatcher with span tracking enabled.
func NewToolDispatcherWithSpans(lookup ToolLookup, policy *policyengine.Engine, projector EventEmitter, spanSource SpanStreamSource) *ToolDispatcher {
	return &ToolDispatcher{
		lookup:     lookup,
		policy:     policy,
		projector:  projector,
		spanSource: spanSource,
		observer:   NoopObserver{},
	}
}

// WithPermissions attaches a tool-scoped permission engine to the dispatcher.
func (d *ToolDispatcher) WithPermissions(engine *permissions.Engine) *ToolDispatcher {
	d.permissions = engine
	return d
}

// WithSessionApprovalGrants attaches exact-input approvals granted for this session.
func (d *ToolDispatcher) WithSessionApprovalGrants(grants []SessionApprovalGrant) *ToolDispatcher {
	d.approvalGrants = cloneSessionApprovalGrants(grants)
	return d
}

func (d *ToolDispatcher) WithPlanApprovalContext(mode PlanModeState, scopes []PlanApprovalScope) *ToolDispatcher {
	d.planMode = mode
	d.planScopes = append([]PlanApprovalScope(nil), scopes...)
	return d
}

func (d *ToolDispatcher) WithUnexpectedStateSignals(signals []UnexpectedStateSignal) *ToolDispatcher {
	d.unexpectedStates = append([]UnexpectedStateSignal(nil), signals...)
	return d
}

// WithHooks attaches a lifecycle hook registry to the dispatcher.
func (d *ToolDispatcher) WithHooks(registry *hooks.Registry) *ToolDispatcher {
	d.hooks = registry
	return d
}

// WithObserver attaches runtime-owned observability hooks to the dispatcher.
func (d *ToolDispatcher) WithObserver(observer Observer) *ToolDispatcher {
	if observer == nil {
		observer = NoopObserver{}
	}
	d.observer = observer
	return d
}

// WithProgressSink attaches an incremental progress sink to the dispatcher.
func (d *ToolDispatcher) WithProgressSink(sink ToolProgressSink) *ToolDispatcher {
	d.progressSink = sink
	return d
}

func (d *ToolDispatcher) WithToolSurfaceFingerprint(fingerprint string) *ToolDispatcher {
	d.toolSurfaceFP = strings.TrimSpace(fingerprint)
	return d
}

func (d *ToolDispatcher) WithPermissionSnapshotHash(hash string) *ToolDispatcher {
	d.permissionHash = strings.TrimSpace(hash)
	return d
}

func (d *ToolDispatcher) WithToolSurfacePolicySnapshot(snapshot *tooling.ToolSurfacePolicySnapshot) *ToolDispatcher {
	if snapshot == nil {
		d.surfacePolicy = nil
		return d
	}
	cp := *snapshot
	cp.HiddenTools = append([]tooling.ToolHiddenReason(nil), snapshot.HiddenTools...)
	cp.VisibleTools = append([]tooling.ToolVisibleReason(nil), snapshot.VisibleTools...)
	d.surfacePolicy = &cp
	return d
}

func (d *ToolDispatcher) WithRuntimeToolRouterSnapshot(snapshot RuntimeToolRouterSnapshot) *ToolDispatcher {
	cp := RuntimeToolRouterSnapshot{
		RegisteredTools:   append([]string(nil), snapshot.RegisteredTools...),
		ModelVisibleTools: append([]string(nil), snapshot.ModelVisibleTools...),
		DispatchableTools: append([]string(nil), snapshot.DispatchableTools...),
		HiddenReasons:     copyRuntimeToolHiddenReasons(snapshot.HiddenReasons),
		PolicyHash:        strings.TrimSpace(snapshot.PolicyHash),
		Fingerprint:       strings.TrimSpace(snapshot.Fingerprint),
	}
	d.runtimeToolSurface = &cp
	if cp.Fingerprint != "" {
		d.toolSurfaceFP = cp.Fingerprint
	}
	return d
}

func (d *ToolDispatcher) WithDeferredCatalogLookup(lookup DeferredToolCatalogLookup) *ToolDispatcher {
	d.deferredCatalog = lookup
	return d
}

func (d *ToolDispatcher) WithVisibleToolMetadata(tools []tooling.ToolMetadata) *ToolDispatcher {
	d.visibleTools = append([]tooling.ToolMetadata(nil), tools...)
	return d
}

func (d *ToolDispatcher) WithReadOnlyRetryConfig(config ReadOnlyRetryConfig) *ToolDispatcher {
	d.retryConfig = normalizeReadOnlyRetryConfig(config)
	return d
}

func (d *ToolDispatcher) WithResourceLockGate(gate ToolResourceLockGate) *ToolDispatcher {
	d.resourceLockGate = gate
	return d
}

func (d *ToolDispatcher) runtimeObserver() Observer {
	if d == nil || d.observer == nil {
		return NoopObserver{}
	}
	return d.observer
}

// DispatchResult is the outcome of a tool dispatch.
type DispatchResult struct {
	ToolCallID    string
	Content       string
	Error         string
	Blocked       bool
	Reason        string
	Metadata      tooling.ToolMetadata
	Result        tooling.ToolResult
	Outcome       string
	Source        string
	Approval      *tooling.PermissionApprovalPayload
	HiddenTools   []string
	Attempts      []ToolAttemptState
	ResourceLocks []promptinput.ResourceLockTrace
	DecisionTrace promptinput.DispatchDecisionTrace
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

func (d *ToolDispatcher) dispatchDecisionTrace(tc ToolCall) promptinput.DispatchDecisionTrace {
	toolSurfaceFingerprint := ""
	if d != nil {
		toolSurfaceFingerprint = strings.TrimSpace(d.toolSurfaceFP)
	}
	return promptinput.DispatchDecisionTrace{
		ToolName:               strings.TrimSpace(tc.Name),
		ToolCallID:             strings.TrimSpace(tc.ID),
		ToolSurfaceFingerprint: toolSurfaceFingerprint,
		PermissionSnapshotHash: d.effectivePermissionSnapshotHash(),
		ArgumentsHash:          toolArgumentsHash(tc.Arguments),
	}
}

func (d *ToolDispatcher) effectivePermissionSnapshotHash() string {
	if d == nil {
		return "sha256:dispatcher-nil"
	}
	if strings.TrimSpace(d.permissionHash) != "" {
		return strings.TrimSpace(d.permissionHash)
	}
	payload, _ := json.Marshal(map[string]any{
		"hasPermissionEngine": d.permissions != nil,
		"approvalGrants":      d.approvalGrants,
		"planMode":            d.planMode.State,
		"planScopes":          d.planScopes,
	})
	return toolArgumentsHash(payload)
}

func (d *ToolDispatcher) dispatch(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode, parentSpanID string, approved bool) (result DispatchResult) {
	var resourceLockTraces []promptinput.ResourceLockTrace
	decisionTrace := d.dispatchDecisionTrace(tc)
	defer func() {
		if len(resourceLockTraces) > 0 {
			result.ResourceLocks = append(result.ResourceLocks, resourceLockTraces...)
		}
		if strings.TrimSpace(result.DecisionTrace.ArgumentsHash) == "" {
			result.DecisionTrace = decisionTrace
		}
	}()

	// Create tool span if span source is available
	var toolSpanID string
	if d.spanSource != nil && parentSpanID != "" {
		toolSpanID = d.spanSource.StartToolSpan(parentSpanID, tc.Name)
	}

	// Look up tool in registry
	desc, executor, found := d.lookup.LookupTool(tc.Name)
	if !found {
		errMsg, meta := d.structuredMissingToolError(tc.Name)
		if meta.Name != "" {
			errResult := d.emitToolFailed(sessionID, turnID, tc, errMsg, "runtime", "tool_failed", meta)
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, errMsg)
			}
			return errResult
		}
		errResult := d.emitToolFailed(sessionID, turnID, tc, errMsg, "runtime", "tool_failed")
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return errResult
	}
	if hidden, ok := d.hiddenByRuntimeToolSurface(tc, desc.Metadata); ok {
		result := d.hiddenToolUnavailableResult(tc, hidden, d.runtimeToolSurfacePolicySnapshot(), desc.Metadata)
		result.Blocked = true
		result.Reason = firstNonEmpty(hidden.Reason, "tool_not_dispatchable")
		return result
	}
	if hidden, ok := d.hiddenByToolSurfacePolicy(tc, desc.Metadata, approved || d.hasSessionApprovalGrant(tc, desc)); ok {
		return d.hiddenToolUnavailableResult(tc, hidden, d.surfacePolicy, desc.Metadata)
	}
	if errMsg, blocked := toolMCPUnavailableError(tc.Name, desc.Metadata); blocked {
		errResult := d.emitToolFailed(sessionID, turnID, tc, errMsg, "mcp", "tool_failed", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return errResult
	}
	argsHash := toolArgumentsHash(tc.Arguments)
	mcpServerID, mcpServerState := mcpObserverAttrs(desc.Metadata)
	observedCtx, observedToolSpan := d.runtimeObserver().StartToolCall(ctx, ToolCallSpanAttrs{
		SessionID:              sessionID,
		TurnID:                 turnID,
		ToolName:               firstNonEmpty(tc.Name, desc.Metadata.Name),
		ToolCallID:             tc.ID,
		Risk:                   string(desc.Metadata.RiskLevel.Normalize()),
		ArgumentsHash:          argsHash,
		ToolSurfaceFingerprint: d.toolSurfaceFP,
		MCPServerID:            mcpServerID,
		MCPServerState:         mcpServerState,
	})
	if observedCtx != nil {
		ctx = observedCtx
	}
	defer func() {
		finishObservedToolSpan(observedToolSpan, turnID, tc, result, argsHash, d.toolSurfaceFP, mcpServerID, mcpServerState)
	}()

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
			result := d.emitToolFailed(sessionID, turnID, tc, "pre_tool_use: "+err.Error(), "hook", "tool_failed", desc.Metadata)
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "pre_tool_use: "+err.Error())
			}
			return result
		}
	}
	if preference := d.evaluateDedicatedToolPreference(tc); preference.Action != policyengine.DedicatedToolPreferenceAllow {
		errMsg := dedicatedToolPreferredError(tc.Name, preference)
		result := d.emitToolFailed(sessionID, turnID, tc, errMsg, "policy", "tool_failed", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return result
	}
	if len(toolEvent.UpdatedInput) > 0 {
		tc.Arguments = append(json.RawMessage(nil), toolEvent.UpdatedInput...)
		toolEvent.Arguments = tc.Arguments
	}

	if err := toolfailure.ValidateArguments(desc.InputSchema, tc.Arguments); err != nil {
		errMsg := "invalid arguments: " + err.Error()
		result := d.emitToolFailed(sessionID, turnID, tc, errMsg, "runtime", "tool_failed", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return result
	}

	ctx, tc.Arguments = enrichToolExecutionContext(ctx, sessionID, turnID, tc, desc)
	toolEvent.Arguments = tc.Arguments

	if !approved && d.hasSessionApprovalGrant(tc, desc) {
		approved = true
	}

	if decision, blocked := d.checkPlanApprovalPrecedence(tc, desc, mode, approved); blocked {
		result := d.emitToolFailed(sessionID, turnID, tc, decision.Reason, "policy", "tool_denied", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, decision.Reason)
		}
		return result
	} else if decision.Allowed {
		approved = true
	}

	if decision := EvaluateUnexpectedStateGate(d.unexpectedStates, tc, desc.Metadata); decision.Action == UnexpectedStateActionBlockMutation {
		reason := "denied: " + strings.Join(decision.Reasons, ", ")
		result := d.emitToolFailed(sessionID, turnID, tc, reason, "policy", "tool_denied", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, reason)
		}
		return result
	}

	if !approved {
		if checker, ok := executor.(ToolPermissionChecker); ok {
			decision := checker.CheckPermissions(ctx, tc.Arguments)
			switch decision.Action {
			case tooling.PermissionActionDeny:
				result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+decision.Reason, "tool", "tool_denied", desc.Metadata)
				if d.spanSource != nil && toolSpanID != "" {
					d.spanSource.FailSpan(toolSpanID, "denied: "+decision.Reason)
				}
				return result
			case tooling.PermissionActionNeedApproval:
				if d.spanSource != nil && toolSpanID != "" {
					d.spanSource.FailSpan(toolSpanID, "awaiting approval: "+decision.Reason)
				}
				return DispatchResult{
					ToolCallID: tc.ID,
					Blocked:    true,
					Reason:     decision.Reason,
					Metadata:   desc.Metadata,
					Outcome:    "approval_needed",
					Source:     "tool",
					Approval:   decision.Approval,
				}
			case tooling.PermissionActionNeedEvidence:
				if d.spanSource != nil && toolSpanID != "" {
					d.spanSource.FailSpan(toolSpanID, "evidence required: "+decision.Reason)
				}
				return DispatchResult{
					ToolCallID: tc.ID,
					Blocked:    true,
					Reason:     "evidence required: " + decision.Reason,
					Metadata:   desc.Metadata,
					Outcome:    "evidence_needed",
					Source:     "tool",
				}
			}
		}
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
			result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+decision.Reason, "policy", "tool_denied", desc.Metadata)
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
				Metadata:   desc.Metadata,
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
				Metadata:   desc.Metadata,
				Outcome:    "evidence_needed",
				Source:     "policy",
			}
		}
	}

	if !approved {
		if override := toolEvent.UpdatedPermissions; override != nil {
			switch override.Action {
			case tooling.PermissionActionDeny:
				result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+override.Reason, "hook", "tool_denied", desc.Metadata)
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
					Metadata:   desc.Metadata,
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
					Metadata:   desc.Metadata,
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
			result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+decision.Reason, "permissions", "tool_denied", desc.Metadata)
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
				Metadata:   desc.Metadata,
				Outcome:    "approval_needed",
				Source:     "permissions",
			}
		}
	}

	if guardErr, blocked := d.evaluateMutationSafetyGuard(tc, desc, executor, sessionType, approved); blocked {
		result := d.emitToolFailed(sessionID, turnID, tc, guardErr, "runtime", "tool_denied", desc.Metadata)
		result.Content = guardErr
		result.Result = tooling.ToolResult{ToolCallID: tc.ID, Content: guardErr, Error: guardErr}
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, guardErr)
		}
		return result
	}

	if releaseLocks, traces, lockErr := d.acquireToolResourceLocks(ctx, sessionID, turnID, tc, desc, executor); lockErr != "" {
		resourceLockTraces = append(resourceLockTraces, traces...)
		result := d.emitToolFailed(sessionID, turnID, tc, lockErr, "runtime", "tool_failed", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, lockErr)
		}
		return result
	} else {
		resourceLockTraces = append(resourceLockTraces, traces...)
		if releaseLocks != nil {
			defer releaseLocks()
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
		result := d.emitToolFailed(sessionID, turnID, tc, "tool has no runtime implementation", "runtime", "tool_failed", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, "tool has no runtime implementation")
		}
		return result
	}

	toolResult, retryAttempts, execErr := d.executeToolWithReadOnlyRetry(ctx, tc, desc, executor)
	if execErr != nil {
		errMsg := mutationFailureErrorMessage(tc, desc, executor, execErr.Error())
		result := d.emitToolFailed(sessionID, turnID, tc, errMsg, "tool", "tool_failed", desc.Metadata)
		result.Attempts = append(result.Attempts, retryAttempts...)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return result
	}
	if toolResult.ToolCallID == "" {
		toolResult.ToolCallID = tc.ID
	}
	if toolResult.Error != "" {
		errMsg := mutationFailureErrorMessage(tc, desc, executor, toolResult.Error)
		result := d.emitToolFailed(sessionID, turnID, tc, errMsg, "tool", "tool_failed", desc.Metadata)
		result.Attempts = append(result.Attempts, retryAttempts...)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return result
	}
	if toolResult.HasStream() {
		streamedResult, streamErr := d.consumeStreamingToolResult(sessionID, turnID, tc, toolSpanID, toolResult)
		if streamErr != nil {
			result := d.emitToolFailed(sessionID, turnID, tc, streamErr.Error(), "tool", "tool_failed", desc.Metadata)
			result.Attempts = append(result.Attempts, retryAttempts...)
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
			result := d.emitToolFailed(sessionID, turnID, tc, "post_tool_use: "+err.Error(), "hook", "tool_failed", desc.Metadata)
			result.Attempts = append(result.Attempts, retryAttempts...)
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
		Attempts:    append([]ToolAttemptState(nil), retryAttempts...),
	}
}

func (d *ToolDispatcher) executeToolWithReadOnlyRetry(ctx context.Context, tc ToolCall, desc ToolDescriptor, executor ToolExecutor) (tooling.ToolResult, []ToolAttemptState, error) {
	result, err := executor.Execute(ctx, tc.Arguments)
	attempts := make([]ToolAttemptState, 0)
	retriesForCall := 0
	for {
		failureText := toolExecutionFailureText(result, err)
		if strings.TrimSpace(failureText) == "" {
			if retriesForCall > 0 {
				attempts = append(attempts, d.newRetryAttempt(tc, desc, ToolAttemptOutcomeCompleted, "", "read-only retry completed", 0))
			}
			return result, attempts, err
		}
		failureKind := classifyToolExecutionFailure(desc, failureText)
		failureSignature := BuildFailureSignature(firstNonEmpty(desc.Metadata.Name, tc.Name), tc.Arguments, ToolResult{
			Content: result.Content,
			Error:   firstNonEmpty(result.Error, failureText),
		})
		decision := DecideReadOnlyRetry(ReadOnlyRetryInput{
			Config:                          d.retryConfig,
			Mutating:                        !toolExecutionIsClearlyReadOnly(desc, executor, tc.Arguments),
			FailureKind:                     failureKind,
			FailureSignature:                failureSignature,
			FailureSignatureSeenCount:       retriesForCall + 1,
			OriginalArgumentsHash:           toolArgumentsHash(tc.Arguments),
			EffectiveArgumentsHash:          toolArgumentsHash(tc.Arguments),
			OriginalToolSurfaceFingerprint:  d.toolSurfaceFP,
			EffectiveToolSurfaceFingerprint: d.toolSurfaceFP,
			CompletedRetryAttemptsForCall:   retriesForCall,
			CompletedRetryAttemptsForTurn:   d.currentReadOnlyRetriesThisTurn(),
		})
		if !decision.Allowed {
			if decision.Eligible {
				attempts = append(attempts, d.newRetryAttempt(tc, desc, ToolAttemptOutcomeSkipped, failureKind, decision.Reason, int(decision.Backoff/time.Millisecond)))
			}
			return result, attempts, err
		}
		if decision.Backoff > 0 {
			timer := time.NewTimer(decision.Backoff)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				attempts = append(attempts, d.newRetryAttempt(tc, desc, ToolAttemptOutcomeSkipped, failureKind, "context canceled during retry backoff", int(decision.Backoff/time.Millisecond)))
				return result, attempts, err
			case <-timer.C:
			}
		}
		if !d.reserveReadOnlyRetryForTurn() {
			attempts = append(attempts, d.newRetryAttempt(tc, desc, ToolAttemptOutcomeSkipped, failureKind, "retry per-turn budget exhausted", int(decision.Backoff/time.Millisecond)))
			return result, attempts, err
		}
		attempts = append(attempts, d.newRetryAttempt(tc, desc, ToolAttemptOutcomeStarted, failureKind, decision.Reason, int(decision.Backoff/time.Millisecond)))
		retriesForCall++
		result, err = executor.Execute(ctx, tc.Arguments)
		if strings.TrimSpace(toolExecutionFailureText(result, err)) != "" && retriesForCall >= normalizeReadOnlyRetryConfig(d.retryConfig).MaxPerCall {
			nextKind := classifyToolExecutionFailure(desc, toolExecutionFailureText(result, err))
			attempts = append(attempts, d.newRetryAttempt(tc, desc, ToolAttemptOutcomeFailed, nextKind, "read-only retry failed", 0))
			return result, attempts, err
		}
	}
}

func (d *ToolDispatcher) structuredMissingToolError(name string) (string, tooling.ToolMetadata) {
	if d != nil && d.deferredCatalog != nil {
		if meta, ok := d.deferredCatalog.LookupDeferredTool(name); ok {
			if errMsg, blocked := toolMCPUnavailableError(name, meta); blocked {
				return errMsg, meta
			}
			payload := map[string]string{
				"errorType":            "tool_unloaded",
				"toolName":             name,
				"reason":               "tool exists in deferred catalog but is not loaded in current tool surface",
				"requiredAction":       "call tool_search with mode=search, then mode=select",
				"suggestedSearchQuery": suggestedToolSearchQuery(meta),
			}
			data, _ := json.Marshal(payload)
			return string(data), meta
		}
	}
	payload := map[string]string{
		"errorType": "tool_not_found",
		"toolName":  name,
		"reason":    "tool not found: " + name + "; no registered or deferred tool matches this name",
	}
	data, _ := json.Marshal(payload)
	return string(data), tooling.ToolMetadata{}
}

func toolMCPUnavailableError(toolName string, meta tooling.ToolMetadata) (string, bool) {
	discovery := meta.EffectiveDiscovery()
	if !discovery.RequiresHealthyMCP {
		return "", false
	}
	serverID := strings.TrimSpace(discovery.MCPServerID)
	if serverID == "" {
		return "", false
	}
	registry := mcp.DefaultRegistry()
	if registry == nil {
		return "", false
	}
	snapshot, ok := registry.GetServerHealthSnapshot(serverID)
	if !ok {
		return "", false
	}
	status := mcp.HealthStatus(strings.TrimSpace(string(snapshot.Status)))
	switch status {
	case mcp.HealthUnavailable, mcp.HealthDisabled:
	default:
		return "", false
	}
	reason := fmt.Sprintf("skipped due to mcp_unavailable: server %s health=%s", serverID, status)
	if strings.TrimSpace(snapshot.LastError) != "" {
		reason += "; " + mcp.RedactHealthError(snapshot.LastError)
	}
	payload := map[string]string{
		"errorType":            "mcp_unavailable",
		"toolName":             strings.TrimSpace(toolName),
		"reason":               reason,
		"requiredAction":       "use another direct evidence source or wait until the external source is healthy",
		"suggestedSearchQuery": suggestedToolSearchQuery(meta),
		"mcpServerId":          serverID,
		"healthStatus":         string(status),
	}
	data, _ := json.Marshal(payload)
	return string(data), true
}

func suggestedToolSearchQuery(meta tooling.ToolMetadata) string {
	discovery := meta.EffectiveDiscovery()
	parts := []string{
		discovery.CapabilityKind,
		strings.Join(discovery.ResourceTypes, " "),
		strings.Join(discovery.OperationKinds, " "),
		meta.SearchHint,
		meta.Description,
	}
	query := strings.Join(parts, " ")
	query = strings.Join(strings.Fields(query), " ")
	if query == "" {
		query = meta.Name
	}
	return query
}

func (d *ToolDispatcher) evaluateDedicatedToolPreference(tc ToolCall) policyengine.DedicatedToolPreferenceDecision {
	if d == nil {
		return policyengine.DedicatedToolPreferenceDecision{Action: policyengine.DedicatedToolPreferenceAllow}
	}
	return policyengine.EvaluateDedicatedToolPreference(tc.Name, tc.Arguments, d.visibleTools, fallbackReasonFromToolArgs(tc.Arguments))
}

func fallbackReasonFromToolArgs(args json.RawMessage) string {
	var payload struct {
		FallbackReason string `json:"fallbackReason"`
		Reason         string `json:"reason"`
		Justification  string `json:"justification"`
	}
	if err := json.Unmarshal(args, &payload); err != nil {
		return ""
	}
	for _, value := range []string{payload.FallbackReason, payload.Reason, payload.Justification} {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func dedicatedToolPreferredError(toolName string, decision policyengine.DedicatedToolPreferenceDecision) string {
	payload := map[string]any{
		"errorType":      "dedicated_tool_preferred",
		"toolName":       toolName,
		"reason":         decision.Reason,
		"requiredAction": "use a visible dedicated tool or provide a specific fallbackReason",
		"preferredTools": decision.PreferredTools,
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func (d *ToolDispatcher) hiddenByToolSurfacePolicy(tc ToolCall, meta tooling.ToolMetadata, approvalResolved bool) (tooling.ToolHiddenReason, bool) {
	if d == nil || d.surfacePolicy == nil || approvalResolved {
		return tooling.ToolHiddenReason{}, false
	}
	return tooling.ToolHiddenBySurfacePolicy(*d.surfacePolicy, meta, tc.Name)
}

func (d *ToolDispatcher) hiddenByRuntimeToolSurface(tc ToolCall, meta tooling.ToolMetadata) (tooling.ToolHiddenReason, bool) {
	if d == nil || d.runtimeToolSurface == nil || !runtimeToolSurfaceDispatchGuardEnabled(*d.runtimeToolSurface) {
		return tooling.ToolHiddenReason{}, false
	}
	if runtimeToolSurfaceContains(d.runtimeToolSurface.DispatchableTools, meta, tc.Name) {
		return tooling.ToolHiddenReason{}, false
	}
	reason := "tool_not_dispatchable"
	for _, candidate := range runtimeToolSurfaceNameCandidates(meta, tc.Name) {
		if reasons := d.runtimeToolSurface.HiddenReasons[candidate]; len(reasons) > 0 && strings.TrimSpace(reasons[0]) != "" {
			reason = strings.TrimSpace(reasons[0])
			break
		}
	}
	return tooling.ToolHiddenReason{Name: firstNonEmpty(tc.Name, meta.Name), Reason: reason}, true
}

func (d *ToolDispatcher) runtimeToolSurfacePolicySnapshot() *tooling.ToolSurfacePolicySnapshot {
	if d == nil || d.runtimeToolSurface == nil || strings.TrimSpace(d.runtimeToolSurface.PolicyHash) == "" {
		return nil
	}
	return &tooling.ToolSurfacePolicySnapshot{Hash: strings.TrimSpace(d.runtimeToolSurface.PolicyHash)}
}

func runtimeToolSurfaceDispatchGuardEnabled(surface RuntimeToolRouterSnapshot) bool {
	return len(surface.ModelVisibleTools) > 0 || len(surface.DispatchableTools) > 0 || len(surface.HiddenReasons) > 0
}

func runtimeToolSurfaceContains(names []string, meta tooling.ToolMetadata, called string) bool {
	candidates := runtimeToolSurfaceNameCandidates(meta, called)
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		for _, candidate := range candidates {
			if strings.EqualFold(name, candidate) {
				return true
			}
		}
	}
	return false
}

func runtimeToolSurfaceNameCandidates(meta tooling.ToolMetadata, called string) []string {
	values := []string{called, meta.Name, tooling.ProviderSafeToolName(called), tooling.ProviderSafeToolName(meta.Name)}
	values = append(values, meta.Aliases...)
	for _, alias := range meta.Aliases {
		values = append(values, tooling.ProviderSafeToolName(alias))
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}

func copyRuntimeToolHiddenReasons(in map[string][]string) map[string][]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string][]string, len(in))
	for name, reasons := range in {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		copied := make([]string, 0, len(reasons))
		for _, reason := range reasons {
			reason = strings.TrimSpace(reason)
			if reason != "" {
				copied = append(copied, reason)
			}
		}
		if len(copied) > 0 {
			out[name] = copied
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func toolHiddenByPolicyError(toolName string, hidden tooling.ToolHiddenReason, snapshot *tooling.ToolSurfacePolicySnapshot) string {
	payload := map[string]string{
		"errorType":      "tool_hidden_by_policy",
		"toolName":       strings.TrimSpace(toolName),
		"reason":         hidden.Reason,
		"requiredAction": "request permission, switch mode, or choose a safer alternative",
	}
	if snapshot != nil {
		payload["policySnapshotHash"] = strings.TrimSpace(snapshot.Hash)
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func (d *ToolDispatcher) hiddenToolUnavailableResult(tc ToolCall, hidden tooling.ToolHiddenReason, snapshot *tooling.ToolSurfacePolicySnapshot, meta tooling.ToolMetadata) DispatchResult {
	reason := strings.TrimSpace(hidden.Reason)
	if reason == "" {
		reason = "tool_hidden_by_policy"
	}
	toolName := firstNonEmpty(tc.Name, hidden.Name, meta.Name)
	payload := map[string]string{
		"schemaVersion": "aiops.tool_unavailable/v1",
		"toolName":      strings.TrimSpace(toolName),
		"reason":        reason,
		"instruction":   "Continue without this tool or ask for explicit host target.",
	}
	if snapshot != nil && strings.TrimSpace(snapshot.Hash) != "" {
		payload["policySnapshotHash"] = strings.TrimSpace(snapshot.Hash)
	}
	content, _ := json.Marshal(payload)
	return DispatchResult{
		ToolCallID:  tc.ID,
		Content:     string(content),
		Metadata:    meta,
		Result:      tooling.ToolResult{ToolCallID: tc.ID, Content: string(content)},
		Outcome:     "tool_unavailable",
		Source:      "policy",
		HiddenTools: []string{strings.TrimSpace(toolName)},
	}
}

type dynamicReadOnlyChecker interface {
	IsReadOnly(json.RawMessage) bool
}

type dynamicDestructiveChecker interface {
	IsDestructive(json.RawMessage) bool
}

func toolExecutionIsClearlyReadOnly(desc ToolDescriptor, executor ToolExecutor, args json.RawMessage) bool {
	if desc.Metadata.EffectiveGovernance(0).Mutating {
		return false
	}
	if checker, ok := executor.(dynamicDestructiveChecker); ok && checker.IsDestructive(args) {
		return false
	}
	checker, ok := executor.(dynamicReadOnlyChecker)
	if !ok {
		return false
	}
	return checker.IsReadOnly(args)
}

func toolExecutionRequiresMutationSafety(desc ToolDescriptor, executor ToolExecutor, args json.RawMessage) bool {
	toolName := firstNonEmpty(desc.Metadata.Name, "")
	if tooling.IsPlanArtifactTool(toolName) {
		return false
	}
	if toolExecutionIsClearlyReadOnly(desc, executor, args) {
		return false
	}
	governance := desc.Metadata.EffectiveGovernance(0)
	if governance.Mutating || desc.Metadata.Layer == tooling.ToolLayerMutation {
		return true
	}
	if checker, ok := executor.(dynamicDestructiveChecker); ok && checker.IsDestructive(args) {
		return true
	}
	return false
}

func (d *ToolDispatcher) evaluateMutationSafetyGuard(tc ToolCall, desc ToolDescriptor, executor ToolExecutor, sessionType SessionType, approved bool) (string, bool) {
	if !toolExecutionRequiresMutationSafety(desc, executor, tc.Arguments) {
		return "", false
	}
	missing := make([]string, 0, 4)
	if len(desc.Metadata.ResourceLocks) == 0 {
		missing = append(missing, "resourceLocks")
	}
	if !toolIdempotencyDeclared(desc.Metadata) {
		missing = append(missing, "idempotency")
	}
	if !d.mutationApprovalBoundaryPresent(desc, executor, approved) {
		missing = append(missing, "approvalScope")
	}
	if sessionType == SessionTypeHost && len(normalizedPostCheckRefs(desc.Metadata)) == 0 {
		missing = append(missing, "postCheckRefs")
	}
	if len(missing) == 0 {
		return "", false
	}
	toolName := firstNonEmpty(tc.Name, desc.Metadata.Name, "tool")
	payload := map[string]any{
		"errorType":      "mutation_safety_guard",
		"toolName":       toolName,
		"reason":         "denied: mutation_safety_guard missing " + strings.Join(missing, ", "),
		"missing":        missing,
		"requiredAction": "declare resourceLocks, idempotency, approval boundary, and host mutation postCheckRefs before executing this mutation",
	}
	data, _ := json.Marshal(payload)
	return string(data), true
}

func toolIdempotencyDeclared(meta tooling.ToolMetadata) bool {
	strategy := strings.TrimSpace(string(meta.Idempotency.Strategy))
	switch tooling.ToolIdempotencyStrategy(strategy) {
	case tooling.ToolIdempotencyStrategyArgumentsHash:
		return true
	case tooling.ToolIdempotencyStrategyExplicitKey:
		return len(nonEmptyStrings(meta.Idempotency.KeyFields)) > 0
	}
	return len(nonEmptyStrings(meta.Idempotency.KeyFields)) > 0
}

func (d *ToolDispatcher) mutationApprovalBoundaryPresent(desc ToolDescriptor, executor ToolExecutor, approved bool) bool {
	if approved {
		return true
	}
	governance := desc.Metadata.EffectiveGovernance(0)
	if governance.RequiresApproval {
		return true
	}
	if _, ok := executor.(ToolPermissionChecker); ok {
		return true
	}
	return d != nil && (d.permissions != nil || d.policy != nil)
}

func normalizedPostCheckRefs(meta tooling.ToolMetadata) []string {
	return nonEmptyStrings(meta.Idempotency.PostCheckRefs)
}

func nonEmptyStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func mutationFailureErrorMessage(tc ToolCall, desc ToolDescriptor, executor ToolExecutor, errText string) string {
	errText = strings.TrimSpace(errText)
	if errText == "" || strings.Contains(strings.ToLower(errText), "side_effect_unknown") {
		return errText
	}
	if !toolExecutionRequiresMutationSafety(desc, executor, tc.Arguments) {
		return errText
	}
	refs := normalizedPostCheckRefs(desc.Metadata)
	postChecks := "unspecified"
	if len(refs) > 0 {
		postChecks = strings.Join(refs, "; ")
	}
	toolName := firstNonEmpty(tc.Name, desc.Metadata.Name, "tool")
	return fmt.Sprintf("side_effect_unknown: mutating tool %s failed after start: %s; postCheckRequired=true; postCheckRefs=%s", toolName, errText, postChecks)
}

func (d *ToolDispatcher) acquireToolResourceLocks(
	ctx context.Context,
	sessionID string,
	turnID string,
	tc ToolCall,
	desc ToolDescriptor,
	executor ToolExecutor,
) (func(), []promptinput.ResourceLockTrace, string) {
	if d == nil || d.resourceLockGate == nil || len(desc.Metadata.ResourceLocks) == 0 {
		return nil, nil, ""
	}
	if toolExecutionIsClearlyReadOnly(desc, executor, tc.Arguments) {
		return nil, nil, ""
	}
	execCtx, _ := tooling.ToolExecutionContextFrom(ctx)
	req := ToolResourceLockRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		OwnerID:   toolResourceLockOwnerID(sessionID, turnID, tc, execCtx),
		ToolCall:  tc,
		Tool:      desc.Metadata,
		Keys:      append([]tooling.ToolResourceLockKey(nil), desc.Metadata.ResourceLocks...),
		Context:   execCtx,
	}
	decision, release, err := d.resourceLockGate.AcquireToolResourceLocks(ctx, req)
	if err != nil {
		decision = ToolResourceLockDecision{Action: "denied", Reason: err.Error()}
		return nil, toolResourceLockTrace(req, decision), resourceLockFailurePayload(tc.Name, decision)
	}
	decision = normalizeToolResourceLockDecision(decision)
	traces := toolResourceLockTrace(req, decision)
	if decision.Action != "acquired" {
		return nil, traces, resourceLockFailurePayload(tc.Name, decision)
	}
	return release, traces, ""
}

func toolExecutionFailureText(result tooling.ToolResult, err error) string {
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(result.Error)
}

func classifyToolExecutionFailure(desc ToolDescriptor, failureText string) string {
	source := "tool"
	if desc.Metadata.IsMCP || strings.TrimSpace(desc.Metadata.MCPInfo.ServerID) != "" {
		source = "mcp"
	}
	decision := toolfailure.NewClassifier().Classify(toolfailure.ClassificationInput{
		Source:  source,
		Outcome: "tool_failed",
		Error:   failureText,
	})
	return string(decision.Kind)
}

func (d *ToolDispatcher) newRetryAttempt(tc ToolCall, desc ToolDescriptor, outcome ToolAttemptOutcome, failureKind, reason string, backoffMillis int) ToolAttemptState {
	now := time.Now()
	toolName := firstNonEmpty(desc.Metadata.Name, tc.Name)
	argsHash := toolArgumentsHash(tc.Arguments)
	return ToolAttemptState{
		ToolCallID:             tc.ID,
		ToolName:               toolName,
		Action:                 ToolAttemptActionRetry,
		TriggerFailureKind:     strings.TrimSpace(failureKind),
		OriginalArgumentsHash:  argsHash,
		EffectiveArgumentsHash: argsHash,
		ToolSurfaceFingerprint: d.toolSurfaceFP,
		DecisionReason:         strings.TrimSpace(reason),
		BackoffMillis:          backoffMillis,
		Outcome:                outcome,
		StartedAt:              now,
		CompletedAt:            now,
	}
}

func (d *ToolDispatcher) currentReadOnlyRetriesThisTurn() int {
	if d == nil {
		return 0
	}
	d.retryMu.Lock()
	defer d.retryMu.Unlock()
	return d.retriesThisTurn
}

func (d *ToolDispatcher) reserveReadOnlyRetryForTurn() bool {
	if d == nil {
		return false
	}
	cfg := normalizeReadOnlyRetryConfig(d.retryConfig)
	d.retryMu.Lock()
	defer d.retryMu.Unlock()
	if d.retriesThisTurn >= cfg.MaxPerTurn {
		return false
	}
	d.retriesThisTurn++
	return true
}

func cloneSessionApprovalGrants(grants []SessionApprovalGrant) []SessionApprovalGrant {
	if len(grants) == 0 {
		return nil
	}
	out := make([]SessionApprovalGrant, len(grants))
	copy(out, grants)
	return out
}

func (d *ToolDispatcher) hasSessionApprovalGrant(tc ToolCall, desc ToolDescriptor) bool {
	if d == nil || len(d.approvalGrants) == 0 {
		return false
	}
	inputHash, err := actionproposal.NormalizedInputHash(tc.Arguments)
	if err != nil || strings.TrimSpace(inputHash) == "" {
		return false
	}
	for _, grant := range d.approvalGrants {
		if !sessionApprovalToolNameMatches(grant.ToolName, tc.Name, desc.Metadata) {
			continue
		}
		if strings.TrimSpace(grant.InputHash) == inputHash {
			return true
		}
	}
	return false
}

func (d *ToolDispatcher) checkPlanApprovalPrecedence(tc ToolCall, desc ToolDescriptor, mode Mode, approved bool) (PlanApprovalScopeMatch, bool) {
	if d == nil {
		return PlanApprovalScopeMatch{}, false
	}
	governance := desc.Metadata.EffectiveGovernance(0)
	mutating := governance.Mutating || governance.RequiresApproval || desc.Metadata.Layer == tooling.ToolLayerMutation
	if planModeRequiresMutationBlock(d.planMode, mode) && mutating && !tooling.IsPlanArtifactTool(firstNonEmpty(tc.Name, desc.Metadata.Name)) {
		return PlanApprovalScopeMatch{Allowed: false, NeedsApproval: false, Reason: "denied: plan_active blocks non-plan mutation before approval scope"}, true
	}
	if !approved || !mutating || len(d.planScopes) == 0 {
		return PlanApprovalScopeMatch{}, false
	}
	call := planScopedToolCallFromDispatch(tc, desc, d.planMode)
	for _, scope := range d.planScopes {
		match := scope.Match(call, time.Now())
		if match.Allowed {
			return match, false
		}
	}
	return PlanApprovalScopeMatch{Allowed: false, NeedsApproval: true, Reason: "denied: approved call is outside the plan approval scope"}, true
}

func planModeRequiresMutationBlock(state PlanModeState, mode Mode) bool {
	if mode != ModePlan && state.State == "" {
		return false
	}
	switch state.State {
	case PlanModeStateRequested, PlanModeStateActive, PlanModeStatePendingExitApproval, PlanModeStateRejected:
		return true
	case "":
		return mode == ModePlan
	default:
		return false
	}
}

func planScopedToolCallFromDispatch(tc ToolCall, desc ToolDescriptor, state PlanModeState) PlanScopedToolCall {
	inputHash, _ := actionproposal.NormalizedInputHash(tc.Arguments)
	resourceType, resourceID, resourcePath := planResourceScopeFromToolCall(tc, desc)
	return PlanScopedToolCall{
		PlanID:       firstNonEmpty(state.ApprovedPlanID, state.PlanID),
		ToolName:     firstNonEmpty(tc.Name, desc.Metadata.Name),
		Action:       firstNonEmpty(tc.Name, desc.Metadata.Name),
		ResourceType: resourceType,
		ResourceID:   resourceID,
		ResourcePath: resourcePath,
		Risk:         string(desc.Metadata.RiskLevel.Normalize()),
		InputHash:    inputHash,
	}
}

func planResourceScopeFromToolCall(tc ToolCall, desc ToolDescriptor) (string, string, string) {
	var payload map[string]any
	_ = json.Unmarshal(tc.Arguments, &payload)
	resourceType := firstStringValue(payload, "resourceType", "type", "kind")
	resourceID := firstStringValue(payload, "resourceId", "resourceID", "id", "target", "name")
	resourcePath := firstStringValue(payload, "resourcePath", "path", "file", "destination")
	if resourceType == "" && len(desc.Metadata.Discovery.ResourceTypes) > 0 {
		resourceType = desc.Metadata.Discovery.ResourceTypes[0]
	}
	for _, lock := range desc.Metadata.ResourceLocks {
		if resourceType == "" {
			resourceType = lock.ResourceType
		}
		if resourceID == "" {
			resourceID = lock.ResourceID
		}
	}
	return resourceType, resourceID, resourcePath
}

func firstStringValue(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		if value, ok := payload[key]; ok {
			switch typed := value.(type) {
			case string:
				if trimmed := strings.TrimSpace(typed); trimmed != "" {
					return trimmed
				}
			case fmt.Stringer:
				if trimmed := strings.TrimSpace(typed.String()); trimmed != "" {
					return trimmed
				}
			}
		}
	}
	return ""
}

func sessionApprovalToolNameMatches(grantToolName, callName string, meta tooling.ToolMetadata) bool {
	target := strings.TrimSpace(grantToolName)
	if target == "" {
		return false
	}
	for _, candidate := range []string{strings.TrimSpace(callName), strings.TrimSpace(meta.Name)} {
		if candidate != "" && candidate == target {
			return true
		}
	}
	for _, alias := range meta.Aliases {
		if strings.TrimSpace(alias) == target {
			return true
		}
	}
	return false
}

func finishObservedToolSpan(span ObservedSpan, turnID string, tc ToolCall, result DispatchResult, argsHash, toolSurfaceFingerprint, mcpServerID, mcpServerState string) {
	if span == nil {
		return
	}
	_, _, _, rawRef, resultBytes, resultTruncated := summarizeToolLifecycleResultForEvent(turnID, tc.ID, result.Content)
	outcome := result.Outcome
	if outcome == "" {
		switch {
		case result.Blocked:
			outcome = "blocked"
		case result.Error != "":
			outcome = "tool_failed"
		default:
			outcome = "tool_result"
		}
	}
	attrs := map[string]any{
		"tool.outcome":          outcome,
		"tool.result_bytes":     resultBytes,
		"tool.result_truncated": resultTruncated,
		"tool.raw_ref":          rawRef,
	}
	if argsHash != "" {
		attrs["tool.args_hash"] = argsHash
	}
	if toolSurfaceFingerprint != "" {
		attrs["tool.surface_fingerprint"] = toolSurfaceFingerprint
	}
	if mcpServerID != "" {
		attrs["mcp.server_id"] = mcpServerID
	}
	if mcpServerState != "" {
		attrs["mcp.server_state"] = mcpServerState
	}
	if result.Error != "" {
		attrs["error"] = result.Error
		if failureKind := failureKindForDispatchResult(result); failureKind != "" {
			attrs["tool.failure_kind"] = failureKind
		}
	}
	if len(result.Attempts) > 0 {
		lastAttempt := result.Attempts[len(result.Attempts)-1]
		attrs["tool.attempt_count"] = len(result.Attempts)
		attrs["tool.last_attempt_action"] = string(lastAttempt.Action)
		attrs["tool.last_attempt_outcome"] = string(lastAttempt.Outcome)
	}
	span.SetAttributes(attrs)
	switch {
	case result.Error != "":
		span.SetStatus("failed", result.Error)
	case result.Blocked:
		span.SetStatus("blocked", result.Reason)
	default:
		span.SetStatus("completed", "")
	}
	span.End()
}

func mcpObserverAttrs(meta tooling.ToolMetadata) (serverID string, serverState string) {
	if !meta.HasMCPSource() {
		return "", ""
	}
	serverID = strings.TrimSpace(meta.MCPInfo.ServerID)
	if serverID == "" {
		serverID = strings.TrimSpace(meta.MCPInfo.ServerName)
	}
	return serverID, "connected"
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
func (d *ToolDispatcher) emitToolFailed(sessionID, turnID string, tc ToolCall, errMsg string, source string, outcome string, meta ...tooling.ToolMetadata) DispatchResult {
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
	result := DispatchResult{
		ToolCallID: tc.ID,
		Error:      errMsg,
		Outcome:    outcome,
		Source:     source,
	}
	if len(meta) > 0 {
		result.Metadata = meta[0]
	}
	return result
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
