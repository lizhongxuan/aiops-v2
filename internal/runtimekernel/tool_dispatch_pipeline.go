package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/hooks"
	"aiops-v2/internal/permissions"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/runtimekernel/toolfailure"
	"aiops-v2/internal/tooling"
)

func (d *ToolDispatcher) dispatch(ctx context.Context, sessionID, turnID string, tc ToolCall, sessionType SessionType, mode Mode, parentSpanID string, approved bool, authorization *VerifiedActionToken) (result DispatchResult) {
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
	explicitApproval := approved
	if explicitApproval && (authorization == nil || authorization.verificationHash == "") {
		return approvalContextStaleDispatchResult(tc, newApprovalContextStaleError("token"))
	}
	if blocked, reject := d.rejectModelToolOutsideStep(sessionID, turnID, tc); reject {
		return blocked
	}

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
	if d.stepToolRouter == nil {
		if hidden, ok := d.hiddenByToolSurfacePolicy(tc, desc.Metadata, approved || d.hasSessionApprovalGrant(tc, desc)); ok {
			return d.hiddenToolUnavailableResult(tc, hidden, d.surfacePolicy, desc.Metadata)
		}
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
	mutatingDispatch := toolExecutionRequiresMutationSafety(desc, executor, tc.Arguments)
	if reason, blocked := d.validateMutationPermissionBinding(tc, desc, executor); blocked {
		return d.emitToolFailed(sessionID, turnID, tc, reason, "runtime", "permission_binding_invalid", desc.Metadata)
	}
	if explicitApproval {
		permissionHash := d.effectivePermissionSnapshotHash()
		if mutatingDispatch {
			// Mutation authorization never accepts the compatibility/display fallback.
			permissionHash = strings.TrimSpace(d.permissionHash)
		}
		if err := authorization.revalidateDispatch(turnID, tc.ID, tc.Name, toolArgumentsHash(tc.Arguments), d.toolSurfaceFP, permissionHash, time.Now()); err != nil {
			return approvalContextStaleDispatchResult(tc, err)
		}
	}

	if err := toolfailure.ValidateArguments(desc.InputSchema, tc.Arguments); err != nil {
		errMsg := "invalid arguments: " + err.Error()
		result := d.emitToolFailed(sessionID, turnID, tc, errMsg, "runtime", "tool_failed", desc.Metadata)
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, errMsg)
		}
		return result
	}

	if reason, blocked := d.checkExecutionScopeGuard(tc, desc.Metadata); blocked {
		result := d.emitToolFailed(sessionID, turnID, tc, reason, "policy", "tool_denied", desc.Metadata)
		result.Blocked = true
		result.Reason = reason
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, reason)
		}
		return result
	}

	if reason, blocked := d.checkRoleBindingGuard(tc, desc.Metadata); blocked {
		result := d.emitToolFailed(sessionID, turnID, tc, reason, "policy", "tool_denied", desc.Metadata)
		result.Blocked = true
		result.Reason = reason
		if d.spanSource != nil && toolSpanID != "" {
			d.spanSource.FailSpan(toolSpanID, reason)
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
			if approved {
				break
			}
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

	// Check PolicyEngine
	if d.policy != nil {
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
			if approved {
				break
			}
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

	if override := toolEvent.UpdatedPermissions; override != nil {
		switch override.Action {
		case tooling.PermissionActionDeny:
			result := d.emitToolFailed(sessionID, turnID, tc, "denied: "+override.Reason, "hook", "tool_denied", desc.Metadata)
			if d.spanSource != nil && toolSpanID != "" {
				d.spanSource.FailSpan(toolSpanID, "denied: "+override.Reason)
			}
			return result
		case tooling.PermissionActionNeedApproval:
			if approved {
				break
			}
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

	if d.permissions != nil {
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
			if approved {
				break
			}
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
