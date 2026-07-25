package runtimekernel

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func (d *ToolDispatcher) evaluateMutationSafetyGuard(tc ToolCall, desc ToolDescriptor, executor ToolExecutor, sessionType SessionType, approved bool) (string, bool) {
	if !toolExecutionRequiresMutationSafety(desc, executor, tc.Arguments) {
		return "", false
	}
	missing := make([]string, 0, 4)
	if len(desc.Metadata.ResourceLocks) == 0 && desc.Metadata.ResourceLockResolver == nil {
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

// toolResultFailureErrorMessage distinguishes an authoritative, typed domain
// rejection returned by a completed tool call from an execution/transport
// failure whose mutation side effect may be unknown. The former must keep its
// structured recovery contract; the latter still receives conservative
// side-effect-unknown hardening.
func toolResultFailureErrorMessage(tc ToolCall, desc ToolDescriptor, executor ToolExecutor, result tooling.ToolResult) string {
	if result.IsError && len(result.StructuredContent) > 0 && json.Valid(result.StructuredContent) {
		return result.FailureText()
	}
	return mutationFailureErrorMessage(tc, desc, executor, result.FailureText())
}

func (d *ToolDispatcher) acquireToolResourceLocks(ctx context.Context, sessionID, turnID string, tc ToolCall, desc ToolDescriptor, executor ToolExecutor) (func(), []promptinput.ResourceLockTrace, string) {
	if d == nil || d.resourceLockGate == nil || (len(desc.Metadata.ResourceLocks) == 0 && desc.Metadata.ResourceLockResolver == nil) {
		return nil, nil, ""
	}
	if toolExecutionIsClearlyReadOnly(desc, executor, tc.Arguments) {
		return nil, nil, ""
	}
	execCtx, _ := tooling.ToolExecutionContextFrom(ctx)
	keys, err := resolveToolResourceLocks(desc.Metadata, tc.Arguments, execCtx)
	if err != nil || len(keys) == 0 {
		decision := ToolResourceLockDecision{Action: "denied", Reason: "resource lock scope is unavailable"}
		if err != nil {
			decision.Reason = err.Error()
		}
		return nil, nil, resourceLockFailurePayload(tc.Name, decision)
	}
	req := ToolResourceLockRequest{
		SessionID: sessionID,
		TurnID:    turnID,
		OwnerID:   toolResourceLockOwnerID(sessionID, turnID, tc, execCtx),
		ToolCall:  tc,
		Tool:      desc.Metadata,
		Keys:      keys,
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

// bindResolvedToolResourceLocks freezes a dynamic resource resolver into the
// descriptor used by this dispatch. Approval requests must bind the exact
// server-resolved resource, not only a broad discovery type, and the approved
// execution must acquire that same frozen key rather than resolving it again.
// This function resolves only metadata; it never acquires a lock or executes
// the tool.
func bindResolvedToolResourceLocks(ctx context.Context, tc ToolCall, desc ToolDescriptor, executor ToolExecutor) (ToolDescriptor, error) {
	if desc.Metadata.ResourceLockResolver == nil || toolExecutionIsClearlyReadOnly(desc, executor, tc.Arguments) {
		return desc, nil
	}
	execCtx, _ := tooling.ToolExecutionContextFrom(ctx)
	keys, err := resolveToolResourceLocks(desc.Metadata, tc.Arguments, execCtx)
	if err != nil {
		return ToolDescriptor{}, err
	}
	if len(keys) == 0 {
		return ToolDescriptor{}, fmt.Errorf("resource lock scope is unavailable")
	}
	desc.Metadata.ResourceLocks = append([]tooling.ToolResourceLockKey(nil), keys...)
	desc.Metadata.ResourceLockResolver = nil
	return desc, nil
}

func resolveToolResourceLocks(meta tooling.ToolMetadata, args json.RawMessage, execCtx tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
	keys := append([]tooling.ToolResourceLockKey(nil), meta.ResourceLocks...)
	if meta.ResourceLockResolver != nil {
		resolved, err := meta.ResourceLockResolver(args, execCtx)
		if err != nil {
			return nil, err
		}
		keys = resolved
	}
	seen := make(map[string]struct{}, len(keys))
	normalized := make([]tooling.ToolResourceLockKey, 0, len(keys))
	for _, key := range keys {
		resourceType := strings.TrimSpace(key.ResourceType)
		resourceID := strings.TrimSpace(key.ResourceID)
		operationKind := strings.TrimSpace(key.OperationKind)
		if resourceType == "" || resourceID == "" || resourceType != key.ResourceType || resourceID != key.ResourceID || operationKind != key.OperationKind {
			return nil, fmt.Errorf("resource lock resolver returned a non-canonical key")
		}
		key.ResourceType = resourceType
		key.ResourceID = resourceID
		key.OperationKind = operationKind
		identity := strings.Join([]string{key.ResourceType, key.ResourceID, key.OperationKind}, "\x00")
		if _, duplicate := seen[identity]; duplicate {
			continue
		}
		seen[identity] = struct{}{}
		normalized = append(normalized, key)
	}
	return normalized, nil
}
