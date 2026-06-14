package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func recordParallelDispatchGroup(snapshot *TurnSnapshot, turnID string, iteration int, calls []ToolCall, tools []promptcompiler.Tool) {
	if snapshot == nil || len(calls) == 0 {
		return
	}
	iter := latestIteration(snapshot)
	if iter == nil || iter.Iteration != iteration {
		return
	}
	group := promptinput.ParallelDispatchTraceGroup{
		GroupID:  fmt.Sprintf("%s-iter-%d-parallel-%d", turnID, iteration, len(iter.ParallelDispatchGroups)),
		Decision: "parallel",
	}
	sharedKeys := map[string]bool{}
	reasonSet := map[string]bool{}
	for _, call := range calls {
		_, reasons, sharedKey := parallelDispatchEligibility(tools, call)
		for _, reason := range reasons {
			reasonSet[reason] = true
		}
		if sharedKey != "" {
			sharedKeys[sharedKey] = true
		}
		group.ToolCalls = append(group.ToolCalls, promptinput.ParallelDispatchToolCall{
			ToolCallID:        call.ID,
			ToolName:          call.Name,
			SharedResourceKey: sharedKey,
		})
	}
	group.Reasons = sortedStringSet(reasonSet)
	group.SharedResourceKeys = sortedStringSet(sharedKeys)
	iter.ParallelDispatchGroups = append(iter.ParallelDispatchGroups, group)
}

func parallelDispatchEligibility(tools []promptcompiler.Tool, tc ToolCall) (bool, []string, string) {
	toolDef := toolForToolCall(tools, tc)
	sharedKey := parallelDispatchSharedResourceKey(tc)
	if toolDef == nil {
		return false, []string{"tool_not_visible", "shared_resource_key"}, sharedKey
	}
	meta := toolDef.Metadata()
	governance := meta.EffectiveGovernance(defaultMaxInlineResultBytes)
	readOnly := toolDef.IsReadOnly(tc.Arguments)
	nonDestructive := !toolDef.IsDestructive(tc.Arguments)
	concurrencySafe := toolDef.IsConcurrencySafe(tc.Arguments)
	noApprovalRequired := !governance.Mutating && !governance.RequiresApproval

	var reasons []string
	if readOnly {
		reasons = append(reasons, "read_only")
	} else {
		reasons = append(reasons, "not_read_only")
	}
	if nonDestructive {
		reasons = append(reasons, "non_destructive")
	} else {
		reasons = append(reasons, "destructive")
	}
	if concurrencySafe {
		reasons = append(reasons, "concurrency_safe")
	} else {
		reasons = append(reasons, "not_concurrency_safe")
	}
	if noApprovalRequired {
		reasons = append(reasons, "no_approval_required")
	}
	if governance.Mutating {
		reasons = append(reasons, "mutating")
	}
	if governance.RequiresApproval || meta.RiskLevel.Normalize() == tooling.ToolRiskHigh {
		reasons = append(reasons, "requires_approval")
	}
	if sharedKey != "" {
		reasons = append(reasons, "shared_resource_key")
	}
	return readOnly && nonDestructive && concurrencySafe && noApprovalRequired, reasons, sharedKey
}

func parallelDispatchSharedResourceKey(tc ToolCall) string {
	fields := []string{
		"resourceKey", "resource_key", "resource", "uri", "url", "path", "file",
		"host", "target", "namespace", "container", "service", "database", "cluster",
	}
	var payload map[string]any
	if err := json.Unmarshal(tc.Arguments, &payload); err == nil {
		for _, field := range fields {
			if value := parallelDispatchStringValue(payload[field]); value != "" {
				return field + ":" + shortTraceHash(value)
			}
		}
	}
	name := strings.TrimSpace(tc.Name)
	if name == "" {
		name = "tool"
	}
	hash := strings.TrimPrefix(toolArgumentsHash(tc.Arguments), "sha256:")
	if len(hash) > 12 {
		hash = hash[:12]
	}
	return "tool:" + name + ":args:" + hash
}

func parallelDispatchStringValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case fmt.Stringer:
		return strings.TrimSpace(v.String())
	default:
		return ""
	}
}

func shortTraceHash(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:12]
}

func sortedStringSet(values map[string]bool) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}
