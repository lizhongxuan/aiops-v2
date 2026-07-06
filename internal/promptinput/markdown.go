package promptinput

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

// RenderMarkdown renders a human-readable semantic trace for a model input.
func RenderMarkdown(trace PromptInputTrace) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Prompt Input Trace")
	fmt.Fprintln(&b)
	renderTraceMetrics(&b, trace)
	renderPromptSectionsMarkdown(&b, trace)
	renderContextUsageMarkdown(&b, trace.ContextUsage)
	renderWebSearchMarkdown(&b, trace)
	renderDepthCoverageGenericityMarkdown(&b, trace)
	renderVerificationSafetyMarkdown(&b, trace)
	renderPlanModeMarkdown(&b, trace)
	renderAgentAssemblyMarkdown(&b, trace)
	renderResourceBindingMarkdown(&b, trace)
	renderSpecialInputWorldStateMarkdown(&b, trace)
	renderToolDiscoveryMarkdown(&b, trace)
	renderSkillDiscoveryMarkdown(&b, trace)
	renderAgentSchedulingMarkdown(&b, trace)
	renderOwnerWriteTraceMarkdown(&b, trace.OwnerWriteTraces)
	if len(trace.Items) == 0 {
		fmt.Fprintln(&b, "_No prompt input trace items._")
		return b.String()
	}

	fmt.Fprintln(&b, "| # | source | semantic | provider | layer | id | status | content |")
	fmt.Fprintln(&b, "|---:|---|---|---|---|---|---|---|")
	for i, item := range trace.Items {
		fmt.Fprintf(
			&b,
			"| %d | %s | %s | %s | %s | %s | %s | %s |\n",
			i,
			escapeMarkdownCell(item.Source),
			escapeMarkdownCell(item.SemanticRole),
			escapeMarkdownCell(item.ProviderRole),
			escapeMarkdownCell(item.PromptLayer),
			escapeMarkdownCell(item.ID),
			escapeMarkdownCell(item.Status),
			escapeMarkdownCell(redactSecrets(item.Content)),
		)
	}
	return b.String()
}

func renderWebSearchMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if trace.WebSearchPolicy == nil && trace.WebSearch == nil && trace.Final == nil {
		return
	}
	fmt.Fprintln(b, "### Web Search")
	if trace.WebSearchPolicy != nil {
		policy := trace.WebSearchPolicy
		if policy.Level != "" {
			fmt.Fprintf(b, "policy_level: %s\n", escapeMarkdownLine(redactSecrets(policy.Level)))
		}
		if policy.Reason != "" {
			fmt.Fprintf(b, "policy_reason: %s\n", escapeMarkdownLine(redactSecrets(policy.Reason)))
		}
		if len(policy.ReasonCodes) > 0 {
			fmt.Fprintf(b, "reason_codes: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(policy.ReasonCodes, ", "))))
		}
		if len(policy.QuerySeeds) > 0 {
			fmt.Fprintf(b, "query_seeds: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(policy.QuerySeeds, " | "))))
		}
		if policy.DisabledBy != "" {
			fmt.Fprintf(b, "disabled_by: %s\n", escapeMarkdownLine(redactSecrets(policy.DisabledBy)))
		}
		if policy.RequireCitations {
			fmt.Fprintln(b, "require_citations: true")
		}
	}
	if trace.WebSearch != nil {
		search := trace.WebSearch
		fmt.Fprintf(b, "attempted: %t\n", search.Attempted)
		if search.RetryCount > 0 {
			fmt.Fprintf(b, "retry_count: %d\n", search.RetryCount)
		}
		if search.Adapter != "" {
			fmt.Fprintf(b, "adapter: %s\n", escapeMarkdownLine(redactSecrets(search.Adapter)))
		}
		if search.SourceCount > 0 {
			fmt.Fprintf(b, "source_count: %d\n", search.SourceCount)
		}
		if search.FailureReason != "" {
			fmt.Fprintf(b, "failure_reason: %s\n", escapeMarkdownLine(redactSecrets(search.FailureReason)))
		}
	}
	if trace.Final != nil && trace.Final.PublicWebLimitation {
		fmt.Fprintln(b, "final.public_web_limitation: true")
	}
	fmt.Fprintln(b)
}

func renderDepthCoverageGenericityMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if trace.TaskDepth != nil {
		depth := trace.TaskDepth
		fmt.Fprintln(b, "### Task Depth")
		if depth.Level != "" {
			fmt.Fprintf(b, "task_depth: %s\n", escapeMarkdownLine(redactSecrets(depth.Level)))
		}
		if len(depth.Reasons) > 0 {
			fmt.Fprintf(b, "reasons: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(depth.Reasons, ", "))))
		}
		fmt.Fprintf(b, "required_gates: %s\n", escapeMarkdownLine(strings.Join(requiredGateNames(depth), ",")))
		fmt.Fprintln(b)
	}
	if trace.EvidenceCoverage != nil {
		coverage := trace.EvidenceCoverage
		fmt.Fprintln(b, "### Evidence Coverage")
		if coverage.Action != "" {
			fmt.Fprintf(b, "coverage_action: %s\n", escapeMarkdownLine(redactSecrets(coverage.Action)))
		}
		if coverage.Coverage > 0 {
			fmt.Fprintf(b, "coverage: %.2f\n", coverage.Coverage)
		}
		if len(coverage.RequiredDimensions) > 0 {
			fmt.Fprintf(b, "required_dimensions: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(coverage.RequiredDimensions, ", "))))
		}
		if len(coverage.CoveredDimensions) > 0 {
			fmt.Fprintf(b, "covered_dimensions: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(coverage.CoveredDimensions, ", "))))
		}
		if len(coverage.MissingDimensions) > 0 {
			fmt.Fprintf(b, "missing_dimensions: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(coverage.MissingDimensions, ", "))))
		}
		if len(coverage.OpenQuestions) > 0 {
			fmt.Fprintf(b, "open_questions: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(coverage.OpenQuestions, ", "))))
		}
		if coverage.VerificationStatus != "" {
			fmt.Fprintf(b, "verification_status: %s\n", escapeMarkdownLine(redactSecrets(coverage.VerificationStatus)))
		}
		if len(coverage.Reasons) > 0 {
			fmt.Fprintf(b, "reasons: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(coverage.Reasons, ", "))))
		}
		fmt.Fprintln(b)
	}
	if trace.GenericityTrace != nil {
		genericity := trace.GenericityTrace
		fmt.Fprintln(b, "### Genericity Trace")
		if genericity.ResourceIDSource != "" {
			fmt.Fprintf(b, "resource_id_source: %s\n", escapeMarkdownLine(redactSecrets(genericity.ResourceIDSource)))
		}
		if len(genericity.CoreRuleDomainTerms) > 0 {
			fmt.Fprintf(b, "core_rule_domain_terms: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(genericity.CoreRuleDomainTerms, ", "))))
		}
		if len(genericity.AllowedFixtureTerms) > 0 {
			fmt.Fprintf(b, "allowed_fixture_terms: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(genericity.AllowedFixtureTerms, ", "))))
		}
		if len(genericity.AllowedPluginTerms) > 0 {
			fmt.Fprintf(b, "allowed_plugin_terms: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(genericity.AllowedPluginTerms, ", "))))
		}
		if len(genericity.Violations) > 0 {
			fmt.Fprintf(b, "violations: %s\n", escapeMarkdownLine(redactSecrets(strings.Join(genericity.Violations, ", "))))
		}
		fmt.Fprintln(b)
	}
}

func requiredGateNames(depth *TaskDepthTrace) []string {
	if depth == nil {
		return nil
	}
	var gates []string
	if depth.RequiresPlan {
		gates = append(gates, "plan")
	}
	if depth.RequiresEvidence {
		gates = append(gates, "evidence")
	}
	if depth.RequiresValidation {
		gates = append(gates, "validation")
	}
	if len(gates) == 0 {
		return []string{"none"}
	}
	return gates
}

func renderPlanModeMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if trace.PlanModeState != nil {
		fmt.Fprintln(b, "### Plan Mode State")
		state := trace.PlanModeState
		if state.State != "" {
			fmt.Fprintf(b, "state: %s\n", escapeMarkdownLine(redactSecrets(state.State)))
		}
		if state.PlanID != "" {
			fmt.Fprintf(b, "plan_id: %s\n", escapeMarkdownLine(redactSecrets(state.PlanID)))
		}
		if state.ArtifactStatus != "" {
			fmt.Fprintf(b, "artifact_status: %s\n", escapeMarkdownLine(redactSecrets(state.ArtifactStatus)))
		}
		if state.ApprovalStatus != "" {
			fmt.Fprintf(b, "approval_status: %s\n", escapeMarkdownLine(redactSecrets(state.ApprovalStatus)))
		}
		if state.ReminderLevel != "" {
			fmt.Fprintf(b, "reminder_level: %s\n", escapeMarkdownLine(redactSecrets(state.ReminderLevel)))
		}
		if state.PendingQuestions > 0 {
			fmt.Fprintf(b, "pending_questions: %d\n", state.PendingQuestions)
		}
		if state.OpenQuestions > 0 {
			fmt.Fprintf(b, "open_questions: %d\n", state.OpenQuestions)
		}
		if state.RejectionReason != "" {
			fmt.Fprintf(b, "rejection_reason: %s\n", escapeMarkdownLine(redactSecrets(state.RejectionReason)))
		}
		fmt.Fprintln(b)
	}
	if trace.TaskTodoState == nil || len(trace.TaskTodoState.Items) == 0 {
		return
	}
	fmt.Fprintln(b, "### Task/Todo State")
	for _, item := range trace.TaskTodoState.Items {
		if item.Status != "" && item.ID != "" {
			parts := []string{fmt.Sprintf("%s: %s", item.Status, item.ID)}
			if item.Owner != "" {
				parts = append(parts, "owner="+item.Owner)
			}
			if item.BlockedBy != "" {
				parts = append(parts, "blocked_by="+item.BlockedBy)
			}
			fmt.Fprintln(b, escapeMarkdownLine(redactSecrets(strings.Join(parts, " "))))
		}
		if item.PendingEvidence != "" {
			fmt.Fprintf(b, "pending_evidence: %s %s\n", escapeMarkdownLine(redactSecrets(item.ID)), escapeMarkdownLine(redactSecrets(item.PendingEvidence)))
		}
	}
	fmt.Fprintln(b)
}

func renderVerificationSafetyMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if verificationSafetyTraceEmpty(trace) {
		return
	}
	fmt.Fprintln(b, "### Verification/Safety State")
	if trace.VerificationStatus != "" {
		fmt.Fprintf(b, "verification_status: %s\n", escapeMarkdownLine(redactSecrets(trace.VerificationStatus)))
	}
	if trace.VerificationReportRef != "" {
		fmt.Fprintf(b, "verification_report_ref: %s\n", escapeMarkdownLine(redactSecrets(trace.VerificationReportRef)))
	}
	if gate := trace.CompletionGate; gate != nil {
		fmt.Fprintf(b, "completion_gate: %s", escapeMarkdownLine(redactSecrets(gate.Decision)))
		if len(gate.Reasons) > 0 {
			fmt.Fprintf(b, " reasons=%s", escapeMarkdownLine(redactSecrets(strings.Join(gate.Reasons, ", "))))
		}
		fmt.Fprintln(b)
	}
	for _, signal := range trace.SafetySignals {
		label := firstNonEmpty(signal.Category, "unknown")
		if signal.Severity != "" {
			label += "/" + signal.Severity
		}
		fmt.Fprintf(b, "safety: %s", escapeMarkdownLine(redactSecrets(label)))
		if signal.Action != "" {
			fmt.Fprintf(b, " action=%s", escapeMarkdownLine(redactSecrets(signal.Action)))
		}
		if len(signal.Reasons) > 0 {
			fmt.Fprintf(b, " reasons=%s", escapeMarkdownLine(redactSecrets(strings.Join(signal.Reasons, ", "))))
		}
		fmt.Fprintln(b)
	}
	if gate := trace.UnexpectedStateGate; gate != nil {
		fmt.Fprintf(b, "unexpected_state_gate: %s", escapeMarkdownLine(redactSecrets(gate.Action)))
		if gate.BlockedAction != "" {
			fmt.Fprintf(b, " blocked_action=%s", escapeMarkdownLine(redactSecrets(gate.BlockedAction)))
		}
		if len(gate.AffectedScopes) > 0 {
			fmt.Fprintf(b, " scopes=%s", escapeMarkdownLine(redactSecrets(strings.Join(gate.AffectedScopes, ", "))))
		}
		if len(gate.Reasons) > 0 {
			fmt.Fprintf(b, " reasons=%s", escapeMarkdownLine(redactSecrets(strings.Join(gate.Reasons, ", "))))
		}
		fmt.Fprintln(b)
	}
	if scope := trace.ApprovalScope; scope != nil {
		fmt.Fprintf(b, "approval_scope: %s", escapeMarkdownLine(redactSecrets(scope.Status)))
		if scope.GrantID != "" {
			fmt.Fprintf(b, " grant=%s", escapeMarkdownLine(redactSecrets(scope.GrantID)))
		}
		if len(scope.AllowedActions) > 0 {
			fmt.Fprintf(b, " actions=%s", escapeMarkdownLine(redactSecrets(strings.Join(scope.AllowedActions, ", "))))
		}
		if len(scope.ResourceScopes) > 0 {
			fmt.Fprintf(b, " scopes=%s", escapeMarkdownLine(redactSecrets(strings.Join(scope.ResourceScopes, ", "))))
		}
		if scope.RiskCeiling != "" {
			fmt.Fprintf(b, " risk=%s", escapeMarkdownLine(redactSecrets(scope.RiskCeiling)))
		}
		if scope.ExpiresAt != "" {
			fmt.Fprintf(b, " expires_at=%s", escapeMarkdownLine(redactSecrets(scope.ExpiresAt)))
		}
		if scope.InputHash != "" {
			fmt.Fprintf(b, " input_hash=%s", escapeMarkdownLine(redactSecrets(scope.InputHash)))
		}
		if len(scope.Reasons) > 0 {
			fmt.Fprintf(b, " reasons=%s", escapeMarkdownLine(redactSecrets(strings.Join(scope.Reasons, ", "))))
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b)
}

func renderPromptSectionsMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if len(trace.PromptSections) > 0 {
		fmt.Fprintln(b, "### Prompt Sections")
		fmt.Fprintln(b, "| id | kind | source | hash | bytes | tokens | cache |")
		fmt.Fprintln(b, "|---|---|---|---|---:|---:|---|")
		for _, section := range trace.PromptSections {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s | %d | %d | %s |\n",
				escapeMarkdownCell(section.ID),
				escapeMarkdownCell(section.Kind),
				escapeMarkdownCell(section.Source),
				escapeMarkdownCell(section.Hash),
				section.Bytes,
				section.TokensEstimate,
				escapeMarkdownCell(section.Cache),
			)
		}
		fmt.Fprintln(b)
	}
	if len(trace.ChangedSections) == 0 {
		return
	}
	fmt.Fprintln(b, "### Changed Sections")
	fmt.Fprintln(b, "| id | reason | previous | current |")
	fmt.Fprintln(b, "|---|---|---|---|")
	for _, section := range trace.ChangedSections {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s |\n",
			escapeMarkdownCell(section.ID),
			escapeMarkdownCell(section.Reason),
			escapeMarkdownCell(section.PreviousHash),
			escapeMarkdownCell(section.CurrentHash),
		)
	}
	fmt.Fprintln(b)
}

func renderContextUsageMarkdown(b *strings.Builder, usage ContextUsage) {
	if contextUsageEmpty(usage) {
		return
	}
	fmt.Fprintln(b, "### Context Usage")
	if usage.MaxContextTokens > 0 {
		fmt.Fprintf(b, "- max_context_tokens: `%d`\n", usage.MaxContextTokens)
	}
	if usage.ReservedOutputTokens > 0 {
		fmt.Fprintf(b, "- reserved_output_tokens: `%d`\n", usage.ReservedOutputTokens)
	}
	if usage.EstimatedInputTokens > 0 {
		fmt.Fprintf(b, "- estimated_input_tokens: `%d`\n", usage.EstimatedInputTokens)
	}
	if len(usage.Categories) > 0 {
		fmt.Fprintln(b, "\n| category | bytes | tokens | items |")
		fmt.Fprintln(b, "|---|---:|---:|---:|")
		for _, category := range usage.Categories {
			fmt.Fprintf(
				b,
				"| %s | %d | %d | %d |\n",
				escapeMarkdownCell(category.Name),
				category.Bytes,
				category.TokensEstimate,
				category.Items,
			)
		}
	}
	if len(usage.TopContributors) > 0 {
		fmt.Fprintln(b, "\n#### Top Contributors")
		fmt.Fprintln(b, "| kind | id | bytes | tokens | action |")
		fmt.Fprintln(b, "|---|---|---:|---:|---|")
		for _, contributor := range usage.TopContributors {
			fmt.Fprintf(
				b,
				"| %s | %s | %d | %d | %s |\n",
				escapeMarkdownCell(contributor.Kind),
				escapeMarkdownCell(contributor.ID),
				contributor.Bytes,
				contributor.TokensEstimate,
				escapeMarkdownCell(contributor.Action),
			)
		}
	}
	fmt.Fprintln(b)
}

func renderTraceMetrics(b *strings.Builder, trace PromptInputTrace) {
	var lines []string
	if trace.OpsContextCapsuleChars > 0 {
		lines = append(lines, fmt.Sprintf("- ops_context_capsule_chars: `%d`", trace.OpsContextCapsuleChars))
	}
	if trace.SessionFactCount > 0 {
		lines = append(lines, fmt.Sprintf("- session_fact_count: `%d`", trace.SessionFactCount))
	}
	if trace.LettaHintCount > 0 {
		lines = append(lines, fmt.Sprintf("- letta_hint_count: `%d`", trace.LettaHintCount))
	}
	if trace.MemoryItemCount > 0 {
		lines = append(lines, fmt.Sprintf("- memory_item_count: `%d`", trace.MemoryItemCount))
	}
	if len(trace.VisibleOpsManualTools) > 0 {
		lines = append(lines, fmt.Sprintf("- visible_ops_manual_tools: `%s`", escapeBackticks(strings.Join(trace.VisibleOpsManualTools, ", "))))
	}
	if len(trace.DroppedContextReasons) > 0 {
		lines = append(lines, fmt.Sprintf("- dropped_context_reasons: `%s`", escapeBackticks(strings.Join(trace.DroppedContextReasons, ", "))))
	}
	if trace.ToolSurfaceFingerprint != "" {
		lines = append(lines, fmt.Sprintf("- tool_surface_fingerprint: `%s`", escapeBackticks(trace.ToolSurfaceFingerprint)))
	}
	if trace.ToolSurfacePolicySnapshotHash != "" {
		lines = append(lines, fmt.Sprintf("- tool_surface_policy_snapshot_hash: `%s`", escapeBackticks(trace.ToolSurfacePolicySnapshotHash)))
	}
	if len(trace.LoadedToolsDelta) > 0 {
		lines = append(lines, fmt.Sprintf("- loaded_tools_delta: `%s`", escapeBackticks(strings.Join(trace.LoadedToolsDelta, ", "))))
	}
	if len(trace.LoadedPacksDelta) > 0 {
		lines = append(lines, fmt.Sprintf("- loaded_packs_delta: `%s`", escapeBackticks(strings.Join(trace.LoadedPacksDelta, ", "))))
	}
	if trace.SkillIndexHash != "" {
		lines = append(lines, fmt.Sprintf("- skill_index_hash: `%s`", escapeBackticks(trace.SkillIndexHash)))
	}
	if len(trace.LoadedSkillsDelta) > 0 {
		lines = append(lines, fmt.Sprintf("- loaded_skills_delta: `%s`", escapeBackticks(strings.Join(trace.LoadedSkillsDelta, ", "))))
	}
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(b, "## Metrics")
	for _, line := range lines {
		fmt.Fprintln(b, line)
	}
	fmt.Fprintln(b)
}

func renderToolDiscoveryMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if len(trace.DeferredToolDirectory) == 0 &&
		len(trace.ToolSearchEvents) == 0 &&
		len(trace.ToolSelectionEvents) == 0 &&
		len(trace.RejectedToolCalls) == 0 &&
		len(trace.ParallelDispatchGroups) == 0 &&
		len(trace.FailedToolSummaries) == 0 {
		return
	}
	fmt.Fprintln(b, "## Tool Discovery Trace")
	renderDeferredToolDirectoryMarkdown(b, trace.DeferredToolDirectory)
	renderToolSearchEventsMarkdown(b, trace.ToolSearchEvents)
	renderToolSelectionEventsMarkdown(b, trace.ToolSelectionEvents)
	renderRejectedToolCallsMarkdown(b, trace.RejectedToolCalls)
	renderParallelDispatchGroupsMarkdown(b, trace.ParallelDispatchGroups)
	renderFailedToolSummariesMarkdown(b, trace.FailedToolSummaries)
	fmt.Fprintln(b)
}

func renderResourceBindingMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if len(trace.ResourceBindings) == 0 &&
		len(trace.ResourceRoleBindings) == 0 &&
		len(trace.ResourceCapabilities) == 0 &&
		len(trace.ResourceEvidenceRefs) == 0 &&
		trace.SessionTargetSnapshot == nil &&
		len(trace.RoleBindingConflicts) == 0 {
		return
	}
	fmt.Fprintln(b, "## Resource Binding Trace")
	if trace.SessionTargetSnapshot != nil {
		target := trace.SessionTargetSnapshot
		fmt.Fprintln(b, "### Session Target")
		fmt.Fprintf(b, "- target_set: `%s`\n", escapeBackticks(redactSecrets(target.ActiveTargetSetID)))
		fmt.Fprintf(b, "- binding_mode: `%s`\n", escapeBackticks(redactSecrets(target.BindingMode)))
		if len(target.HostIDs) > 0 {
			fmt.Fprintf(b, "- host_ids: `%s`\n", escapeBackticks(redactSecrets(strings.Join(target.HostIDs, ", "))))
		}
		if target.SourceTurnID != "" {
			fmt.Fprintf(b, "- source_turn: `%s`\n", escapeBackticks(redactSecrets(target.SourceTurnID)))
		}
		fmt.Fprintf(b, "- expires_after_turns: `%d`\n", target.ExpiresAfterTurns)
		fmt.Fprintf(b, "- requires_confirmation: `%t`\n", target.RequiresConfirmation)
		fmt.Fprintln(b)
	}
	if len(trace.ResourceBindings) > 0 {
		fmt.Fprintln(b, "### Bindings")
		fmt.Fprintln(b, "| resource | source | trust | verified by | fail closed | trace |")
		fmt.Fprintln(b, "|---|---|---|---|---|---|")
		for _, binding := range trace.ResourceBindings {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s | %t | %s |\n",
				escapeMarkdownCell(resourceRefLabel(binding.Ref.Type, binding.Ref.ID, binding.Ref.DisplayName)),
				escapeMarkdownCell(redactSecrets(binding.Source)),
				escapeMarkdownCell(redactSecrets(binding.TrustLevel)),
				escapeMarkdownCell(redactSecrets(binding.VerifiedBy)),
				binding.FailClosed,
				escapeMarkdownCell(redactSecrets(binding.TraceHash)),
			)
		}
		fmt.Fprintln(b)
	}
	if len(trace.ResourceRoleBindings) > 0 {
		fmt.Fprintln(b, "### Role Bindings")
		fmt.Fprintln(b, "| resource | role | alias | confidence | conflict policy | trace |")
		fmt.Fprintln(b, "|---|---|---|---:|---|---|")
		for _, binding := range trace.ResourceRoleBindings {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %.2f | %s | %s |\n",
				escapeMarkdownCell(resourceRefLabel(binding.ResourceRef.Type, binding.ResourceRef.ID, binding.ResourceRef.DisplayName)),
				escapeMarkdownCell(redactSecrets(binding.Role)),
				escapeMarkdownCell(redactSecrets(strings.Join(binding.RoleAlias, ", "))),
				binding.Confidence,
				escapeMarkdownCell(redactSecrets(binding.ConflictPolicy)),
				escapeMarkdownCell(redactSecrets(binding.TraceHash)),
			)
		}
		fmt.Fprintln(b)
	}
	if len(trace.ResourceCapabilities) > 0 {
		fmt.Fprintln(b, "### Capabilities")
		fmt.Fprintln(b, "| resource | capability | tools | approval | policy | binding trace |")
		fmt.Fprintln(b, "|---|---|---|---|---|---|")
		for _, capability := range trace.ResourceCapabilities {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %t | %s | %s |\n",
				escapeMarkdownCell(resourceRefLabel(capability.ResourceRef.Type, capability.ResourceRef.ID, capability.ResourceRef.DisplayName)),
				escapeMarkdownCell(redactSecrets(capability.Capability)),
				escapeMarkdownCell(redactSecrets(strings.Join(capability.ToolNames, ", "))),
				capability.RequiresApproval,
				escapeMarkdownCell(redactSecrets(capability.PolicyHash)),
				escapeMarkdownCell(redactSecrets(capability.BindingTraceHash)),
			)
		}
		fmt.Fprintln(b)
	}
	if len(trace.ResourceEvidenceRefs) > 0 {
		fmt.Fprintln(b, "### Evidence")
		fmt.Fprintln(b, "| id | resource | source | kind | trace |")
		fmt.Fprintln(b, "|---|---|---|---|---|")
		for _, evidence := range trace.ResourceEvidenceRefs {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s | %s |\n",
				escapeMarkdownCell(redactSecrets(evidence.ID)),
				escapeMarkdownCell(resourceRefLabel(evidence.ResourceRef.Type, evidence.ResourceRef.ID, evidence.ResourceRef.DisplayName)),
				escapeMarkdownCell(redactSecrets(evidence.Source)),
				escapeMarkdownCell(redactSecrets(evidence.Kind)),
				escapeMarkdownCell(redactSecrets(evidence.TraceHash)),
			)
		}
		fmt.Fprintln(b)
	}
	if len(trace.RoleBindingConflicts) > 0 {
		fmt.Fprintln(b, "### Role Conflicts")
		fmt.Fprintln(b, "| resource | role | reasons | trace |")
		fmt.Fprintln(b, "|---|---|---|---|")
		for _, conflict := range trace.RoleBindingConflicts {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s |\n",
				escapeMarkdownCell(redactSecrets(conflict.ResourceID)),
				escapeMarkdownCell(redactSecrets(conflict.Role)),
				escapeMarkdownCell(redactSecrets(strings.Join(conflict.Reasons, ", "))),
				escapeMarkdownCell(redactSecrets(conflict.TraceHash)),
			)
		}
		fmt.Fprintln(b)
	}
}

func renderSpecialInputWorldStateMarkdown(b *strings.Builder, trace PromptInputTrace) {
	state := trace.SpecialInputWorldState
	if state == nil {
		return
	}
	fmt.Fprintln(b, "## Special Input Memory")
	if state.ModelSummary != "" {
		fmt.Fprintf(b, "- summary: %s\n", escapeMarkdownLine(redactSecrets(state.ModelSummary)))
	}
	if state.TurnID != "" {
		fmt.Fprintf(b, "- turn: `%s`\n", escapeBackticks(redactSecrets(state.TurnID)))
	}
	if state.ActiveExecutionScope != nil {
		grant := state.ActiveExecutionScope
		fmt.Fprintln(b, "### Active Execution Scope")
		fmt.Fprintf(b, "- grant: `%s`\n", escapeBackticks(redactSecrets(grant.ID)))
		fmt.Fprintf(b, "- resource: `%s/%s`\n", escapeBackticks(redactSecrets(grant.ResourceKind)), escapeBackticks(redactSecrets(grant.ResourceID)))
		if len(grant.AllowedActions) > 0 {
			fmt.Fprintf(b, "- allowed_actions: `%s`\n", escapeBackticks(redactSecrets(strings.Join(grant.AllowedActions, ", "))))
		}
		if grant.ValidationHash != "" {
			fmt.Fprintf(b, "- validation_hash: `%s`\n", escapeBackticks(redactSecrets(grant.ValidationHash)))
		}
	}
	if len(state.ActiveRoleBindings) > 0 {
		fmt.Fprintln(b, "### Role Bindings")
		fmt.Fprintln(b, "| env | cluster | role | runtime | resource | hash |")
		fmt.Fprintln(b, "|---|---|---|---|---|---|")
		for _, binding := range state.ActiveRoleBindings {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s | %s/%s | %s |\n",
				escapeMarkdownCell(redactSecrets(binding.EnvironmentKey)),
				escapeMarkdownCell(redactSecrets(binding.ClusterKey)),
				escapeMarkdownCell(redactSecrets(binding.RoleKey)),
				escapeMarkdownCell(redactSecrets(binding.RuntimeName)),
				escapeMarkdownCell(redactSecrets(binding.ResourceKind)),
				escapeMarkdownCell(redactSecrets(binding.ResourceID)),
				escapeMarkdownCell(redactSecrets(binding.BindingHash)),
			)
		}
	}
	if len(state.PendingConfirmations) > 0 {
		fmt.Fprintln(b, "### Pending Confirmation")
		fmt.Fprintln(b, "| id | kind | reason | candidates |")
		fmt.Fprintln(b, "|---|---|---|---|")
		for _, pending := range state.PendingConfirmations {
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s |\n",
				escapeMarkdownCell(redactSecrets(pending.ID)),
				escapeMarkdownCell(redactSecrets(pending.Kind)),
				escapeMarkdownCell(redactSecrets(pending.Reason)),
				escapeMarkdownCell(redactSecrets(strings.Join(pending.CandidateIDs, ", "))),
			)
		}
	}
	if len(state.Conflicts) > 0 {
		fmt.Fprintln(b, "### Conflicts")
		fmt.Fprintln(b, "| id | kind | scope | resources | reasons |")
		fmt.Fprintln(b, "|---|---|---|---|---|")
		for _, conflict := range state.Conflicts {
			scope := strings.Join([]string{conflict.EnvironmentKey, conflict.ClusterKey, conflict.RoleKey}, "/")
			fmt.Fprintf(
				b,
				"| %s | %s | %s | %s | %s |\n",
				escapeMarkdownCell(redactSecrets(conflict.ID)),
				escapeMarkdownCell(redactSecrets(conflict.Kind)),
				escapeMarkdownCell(redactSecrets(scope)),
				escapeMarkdownCell(redactSecrets(strings.Join(conflict.ResourceIDs, ", "))),
				escapeMarkdownCell(redactSecrets(strings.Join(conflict.Reasons, ", "))),
			)
		}
	}
	if state.ReadPlan != nil {
		plan := state.ReadPlan
		fmt.Fprintln(b, "### Read Plan")
		if plan.ActiveGrantID != "" {
			fmt.Fprintf(b, "- active_grant: `%s`\n", escapeBackticks(redactSecrets(plan.ActiveGrantID)))
		}
		if plan.ActiveResourceID != "" {
			fmt.Fprintf(b, "- active_resource: `%s/%s`\n", escapeBackticks(redactSecrets(plan.ActiveResourceKind)), escapeBackticks(redactSecrets(plan.ActiveResourceID)))
		}
		if len(plan.RoleBindingHashes) > 0 {
			fmt.Fprintf(b, "- role_binding_hashes: `%s`\n", escapeBackticks(redactSecrets(strings.Join(plan.RoleBindingHashes, ", "))))
		}
		if len(plan.PendingConfirmationIDs) > 0 {
			fmt.Fprintf(b, "- pending_confirmations: `%s`\n", escapeBackticks(redactSecrets(strings.Join(plan.PendingConfirmationIDs, ", "))))
		}
	}
	fmt.Fprintln(b)
}

func renderAgentAssemblyMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if trace.AgentAssemblySnapshot == nil {
		return
	}
	snapshot := trace.AgentAssemblySnapshot
	fmt.Fprintln(b, "## Agent Assembly Snapshot")
	if snapshot.AgentKind != "" {
		fmt.Fprintf(b, "- agent_kind: `%s`\n", escapeBackticks(redactSecrets(snapshot.AgentKind)))
	}
	if snapshot.Profile != "" {
		fmt.Fprintf(b, "- profile: `%s`\n", escapeBackticks(redactSecrets(snapshot.Profile)))
	}
	if snapshot.RuntimeRole != "" {
		fmt.Fprintf(b, "- runtime_role: `%s`\n", escapeBackticks(redactSecrets(snapshot.RuntimeRole)))
	}
	if len(snapshot.RouteReason) > 0 {
		fmt.Fprintf(b, "- route_reason: `%s`\n", escapeBackticks(redactSecrets(strings.Join(snapshot.RouteReason, ", "))))
	}
	if snapshot.SpecHash != "" {
		fmt.Fprintf(b, "- spec_hash: `%s`\n", escapeBackticks(redactSecrets(snapshot.SpecHash)))
	}
	if snapshot.ToolSurface.Hash != "" {
		fmt.Fprintf(b, "- tool_surface_hash: `%s`\n", escapeBackticks(redactSecrets(snapshot.ToolSurface.Hash)))
	}
	if snapshot.ToolSurface.Fingerprint != "" {
		fmt.Fprintf(b, "- tool_surface_fingerprint: `%s`\n", escapeBackticks(redactSecrets(snapshot.ToolSurface.Fingerprint)))
	}
	if snapshot.PromptSections.Hash != "" {
		fmt.Fprintf(b, "- prompt_sections_hash: `%s`\n", escapeBackticks(redactSecrets(snapshot.PromptSections.Hash)))
	}
	fmt.Fprintln(b)
}

func resourceRefLabel(resourceType, id, displayName string) string {
	label := strings.TrimSpace(resourceType)
	if id = strings.TrimSpace(id); id != "" {
		label += ":" + id
	}
	if displayName = strings.TrimSpace(displayName); displayName != "" && displayName != id {
		label += " (" + displayName + ")"
	}
	return redactSecrets(label)
}

func renderOwnerWriteTraceMarkdown(b *strings.Builder, traces []OwnerWriteTrace) {
	if len(traces) == 0 {
		return
	}
	fmt.Fprintln(b, "## Owner Write Trace")
	fmt.Fprintln(b, "| responsibility | owner | writer | outcome | session | turn |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|")
	for _, trace := range traces {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s | %s | %s |\n",
			escapeMarkdownCell(trace.Responsibility),
			escapeMarkdownCell(trace.Owner),
			escapeMarkdownCell(trace.Writer),
			escapeMarkdownCell(trace.Outcome),
			escapeMarkdownCell(trace.SessionID),
			escapeMarkdownCell(trace.TurnID),
		)
	}
	fmt.Fprintln(b)
}

func renderDeferredToolDirectoryMarkdown(b *strings.Builder, entries []promptcompiler.DeferredToolDirectoryEntry) {
	if len(entries) == 0 {
		return
	}
	fmt.Fprintln(b, "### Deferred Tool Directory")
	fmt.Fprintln(b, "| pack | capability | source | health | approval | select | unavailable | tools |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|---|---:|")
	for _, entry := range entries {
		approval := "not_required"
		if entry.RequiresApproval {
			approval = "required"
		}
		selectStatus := "not_required"
		if entry.RequiresSelect {
			selectStatus = "required"
		}
		health := entry.HealthStatus
		if entry.RequiresHealth && health == "" {
			health = "unknown"
		}
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s | %s | %s | %s | %d |\n",
			escapeMarkdownCell(entry.Pack),
			escapeMarkdownCell(redactSecrets(entry.Capability)),
			escapeMarkdownCell(entry.Source),
			escapeMarkdownCell(health),
			escapeMarkdownCell(approval),
			escapeMarkdownCell(selectStatus),
			escapeMarkdownCell(redactSecrets(entry.UnavailableReason)),
			entry.ToolCount,
		)
	}
	fmt.Fprintln(b)
}

func renderSkillDiscoveryMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if len(trace.SkillSearchEvents) == 0 &&
		len(trace.SkillReadEvents) == 0 &&
		len(trace.RejectedSkillActivations) == 0 {
		return
	}
	fmt.Fprintln(b, "## Skill Discovery Trace")
	renderSkillSearchEventsMarkdown(b, trace.SkillSearchEvents)
	renderSkillReadEventsMarkdown(b, trace.SkillReadEvents)
	renderRejectedSkillActivationsMarkdown(b, trace.RejectedSkillActivations)
	fmt.Fprintln(b)
}

func renderSkillSearchEventsMarkdown(b *strings.Builder, events []SkillSearchTraceEvent) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintln(b, "### Skill Search Events")
	fmt.Fprintln(b, "| mode | query | matches | reason |")
	fmt.Fprintln(b, "|---|---|---|---|")
	for _, event := range events {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s |\n",
			escapeMarkdownCell(event.Mode),
			escapeMarkdownCell(redactSecrets(event.Query)),
			escapeMarkdownCell(strings.Join(event.Matches, ", ")),
			escapeMarkdownCell(redactSecrets(event.Reason)),
		)
	}
	fmt.Fprintln(b)
}

func renderSkillReadEventsMarkdown(b *strings.Builder, events []SkillReadTraceEvent) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintln(b, "### Skill Read Events")
	fmt.Fprintln(b, "| skill | source | reason | range |")
	fmt.Fprintln(b, "|---|---|---|---|")
	for _, event := range events {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s |\n",
			escapeMarkdownCell(event.Skill),
			escapeMarkdownCell(event.Source),
			escapeMarkdownCell(redactSecrets(event.Reason)),
			escapeMarkdownCell(event.Range),
		)
	}
	fmt.Fprintln(b)
}

func renderRejectedSkillActivationsMarkdown(b *strings.Builder, events []RejectedSkillActivationTraceEvent) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintln(b, "### Rejected Skill Activations")
	fmt.Fprintln(b, "| skill | reason | required action | turn |")
	fmt.Fprintln(b, "|---|---|---|---|")
	for _, event := range events {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s |\n",
			escapeMarkdownCell(event.SkillName),
			escapeMarkdownCell(redactSecrets(event.Reason)),
			escapeMarkdownCell(event.RequiredAction),
			escapeMarkdownCell(event.TurnID),
		)
	}
	fmt.Fprintln(b)
}

func renderToolSearchEventsMarkdown(b *strings.Builder, events []ToolSearchTraceEvent) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintln(b, "### Tool Search Events")
	fmt.Fprintln(b, "| mode | query | ranker | matches | rejected | reason |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|")
	for _, event := range events {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s | %s | %s |\n",
			escapeMarkdownCell(event.Mode),
			escapeMarkdownCell(redactSecrets(event.Query)),
			escapeMarkdownCell(redactSecrets(event.Ranker)),
			escapeMarkdownCell(strings.Join(event.Matches, ", ")),
			escapeMarkdownCell(redactSecrets(formatToolSearchRejectedReasons(event.RejectedReasons))),
			escapeMarkdownCell(redactSecrets(event.Reason)),
		)
	}
	fmt.Fprintln(b)
}

func formatToolSearchRejectedReasons(reasons []ToolSearchRejectedReason) string {
	if len(reasons) == 0 {
		return ""
	}
	parts := make([]string, 0, len(reasons))
	for _, reason := range reasons {
		name := strings.TrimSpace(reason.ToolName)
		value := strings.TrimSpace(reason.Reason)
		if value == "" {
			value = strings.TrimSpace(reason.FilteredReason)
		}
		if name == "" && value == "" {
			continue
		}
		if name == "" {
			parts = append(parts, value)
			continue
		}
		if value == "" {
			parts = append(parts, name)
			continue
		}
		parts = append(parts, name+"="+value)
	}
	return strings.Join(parts, ", ")
}

func renderToolSelectionEventsMarkdown(b *strings.Builder, events []ToolSelectionTraceEvent) {
	if len(events) == 0 {
		return
	}
	fmt.Fprintln(b, "### Tool Selection Events")
	fmt.Fprintln(b, "| source | reason | loaded tools | loaded packs | not loaded | not loaded reasons |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|")
	for _, event := range events {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s | %s | %s |\n",
			escapeMarkdownCell(event.Source),
			escapeMarkdownCell(redactSecrets(event.Reason)),
			escapeMarkdownCell(strings.Join(event.LoadedTools, ", ")),
			escapeMarkdownCell(strings.Join(event.LoadedPacks, ", ")),
			escapeMarkdownCell(strings.Join(event.NotLoaded, ", ")),
			escapeMarkdownCell(redactSecrets(formatKeyValueMap(event.NotLoadedReasons))),
		)
	}
	fmt.Fprintln(b)
}

func renderRejectedToolCallsMarkdown(b *strings.Builder, calls []RejectedToolCallTraceEvent) {
	if len(calls) == 0 {
		return
	}
	fmt.Fprintln(b, "### Rejected Tool Calls")
	fmt.Fprintln(b, "| tool | error | reason | action | suggested search | call |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|")
	for _, call := range calls {
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s | %s | %s |\n",
			escapeMarkdownCell(call.ToolName),
			escapeMarkdownCell(call.ErrorType),
			escapeMarkdownCell(redactSecrets(call.Reason)),
			escapeMarkdownCell(redactSecrets(call.RequiredAction)),
			escapeMarkdownCell(redactSecrets(call.SuggestedSearchQuery)),
			escapeMarkdownCell(call.ToolCallID),
		)
	}
	fmt.Fprintln(b)
}

func formatKeyValueMap(values map[string]string) string {
	if len(values) == 0 {
		return ""
	}
	keys := make([]string, 0, len(values))
	for key := range values {
		if strings.TrimSpace(key) != "" {
			keys = append(keys, strings.TrimSpace(key))
		}
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		value := strings.TrimSpace(values[key])
		if value == "" {
			continue
		}
		parts = append(parts, key+"="+value)
	}
	return strings.Join(parts, ", ")
}

func renderParallelDispatchGroupsMarkdown(b *strings.Builder, groups []ParallelDispatchTraceGroup) {
	if len(groups) == 0 {
		return
	}
	fmt.Fprintln(b, "### Parallel Dispatch Groups")
	fmt.Fprintln(b, "| group | decision | reasons | tools | shared resource keys | excluded |")
	fmt.Fprintln(b, "|---|---|---|---|---|---|")
	for _, group := range groups {
		tools := make([]string, 0, len(group.ToolCalls))
		for _, call := range group.ToolCalls {
			tools = append(tools, call.ToolName)
		}
		excluded := make([]string, 0, len(group.Excluded))
		for _, call := range group.Excluded {
			label := call.ToolName
			if len(call.Reasons) > 0 {
				label += " (" + strings.Join(call.Reasons, ", ") + ")"
			}
			excluded = append(excluded, label)
		}
		fmt.Fprintf(
			b,
			"| %s | %s | %s | %s | %s | %s |\n",
			escapeMarkdownCell(group.GroupID),
			escapeMarkdownCell(group.Decision),
			escapeMarkdownCell(strings.Join(group.Reasons, ", ")),
			escapeMarkdownCell(strings.Join(tools, ", ")),
			escapeMarkdownCell(strings.Join(group.SharedResourceKeys, ", ")),
			escapeMarkdownCell(strings.Join(excluded, ", ")),
		)
	}
	fmt.Fprintln(b)
}

func renderFailedToolSummariesMarkdown(b *strings.Builder, summaries []FailedToolSummary) {
	if len(summaries) == 0 {
		return
	}
	fmt.Fprintln(b, "### Failed Tool Summaries")
	fmt.Fprintln(b, "| tool | failureClass | attempts | finalStatus | safeToRetry | modelGuidance |")
	fmt.Fprintln(b, "|---|---|---:|---|---|---|")
	for _, summary := range summaries {
		fmt.Fprintf(
			b,
			"| %s | %s | %d | %s | %t | %s |\n",
			escapeMarkdownCell(summary.Tool),
			escapeMarkdownCell(summary.FailureClass),
			summary.Attempts,
			escapeMarkdownCell(summary.FinalStatus),
			summary.SafeToRetry,
			escapeMarkdownCell(redactSecrets(summary.ModelGuidance)),
		)
	}
	fmt.Fprintln(b)
}

func renderAgentSchedulingMarkdown(b *strings.Builder, trace PromptInputTrace) {
	if agentSchedulingTraceEmpty(trace) {
		return
	}
	fmt.Fprintln(b, "### Agent Scheduling")
	if trace.AgentIndexHash != "" {
		fmt.Fprintf(b, "- agent index hash: `%s`\n", escapeMarkdownCell(trace.AgentIndexHash))
	}
	for _, entry := range trace.AgentIndexEntries {
		label := firstNonEmpty(entry.Name, entry.Kind)
		if label == "" {
			continue
		}
		fmt.Fprintf(b, "- agent listing loaded: %s", escapeMarkdownCell(label))
		if entry.WhenToUse != "" {
			fmt.Fprintf(b, " - %s", escapeMarkdownCell(redactSecrets(entry.WhenToUse)))
		}
		fmt.Fprintln(b)
	}
	for _, dropped := range trace.AgentIndexDropped {
		label := firstNonEmpty(dropped.Name, "unknown")
		fmt.Fprintf(b, "- agent listing dropped: %s", escapeMarkdownCell(label))
		if dropped.Reason != "" {
			fmt.Fprintf(b, " reason=%s", escapeMarkdownCell(dropped.Reason))
		}
		fmt.Fprintln(b)
	}
	if len(trace.AgentIndexDelta) > 0 {
		fmt.Fprintf(b, "- agent listing delta: %s\n", escapeMarkdownCell(strings.Join(trace.AgentIndexDelta, ", ")))
	}
	if decision := trace.AgentDelegationDecision; decision != nil {
		fmt.Fprintf(b, "- delegation decision: %s", escapeMarkdownCell(decision.Action))
		if decision.Reason != "" {
			fmt.Fprintf(b, " reason=%s", escapeMarkdownCell(decision.Reason))
		}
		if decision.CandidateAgent != "" {
			fmt.Fprintf(b, " candidate=%s", escapeMarkdownCell(decision.CandidateAgent))
		}
		if decision.ExistingAgentID != "" {
			fmt.Fprintf(b, " existing=%s", escapeMarkdownCell(decision.ExistingAgentID))
		}
		fmt.Fprintln(b)
	}
	for _, lint := range trace.AgentAssignmentLint {
		status := firstNonEmpty(lint.Status, "unknown")
		fmt.Fprintf(b, "- assignment lint: %s", escapeMarkdownCell(status))
		if lint.AgentID != "" {
			fmt.Fprintf(b, " agent=%s", escapeMarkdownCell(lint.AgentID))
		}
		if len(lint.MissingFields) > 0 {
			fmt.Fprintf(b, " missing=%s", escapeMarkdownCell(strings.Join(lint.MissingFields, ", ")))
		}
		fmt.Fprintln(b)
	}
	for _, group := range trace.AgentParallelTraceGroups {
		fmt.Fprintf(b, "- parallel agents requested: %d", group.RequestedCount)
		if group.MissionID != "" {
			fmt.Fprintf(b, " mission=%s", escapeMarkdownCell(group.MissionID))
		}
		if len(group.SpawnedInTurn) > 0 {
			fmt.Fprintf(b, " spawned in same turn: %d", len(group.SpawnedInTurn))
		}
		if len(group.Queued) > 0 {
			fmt.Fprintf(b, " queued by budget: %s", escapeMarkdownCell(strings.Join(group.Queued, ", ")))
		}
		fmt.Fprintln(b)
		for _, reason := range group.SerialReasons {
			if reason.Reason == "" {
				continue
			}
			fmt.Fprintf(b, "  - serial reason: %s", escapeMarkdownCell(reason.Reason))
			if reason.AgentID != "" {
				fmt.Fprintf(b, " agent=%s", escapeMarkdownCell(reason.AgentID))
			}
			fmt.Fprintln(b)
		}
	}
	for _, lock := range trace.ResourceLocks {
		action := firstNonEmpty(lock.Action, "unknown")
		fmt.Fprintf(b, "- resource lock %s", escapeMarkdownCell(action))
		if lock.AgentID != "" {
			fmt.Fprintf(b, " agent=%s", escapeMarkdownCell(lock.AgentID))
		}
		if lock.Key.ResourceType != "" || lock.Key.ResourceID != "" || lock.Key.OperationKind != "" {
			fmt.Fprintf(
				b,
				" key=%s/%s/%s",
				escapeMarkdownCell(lock.Key.ResourceType),
				escapeMarkdownCell(lock.Key.ResourceID),
				escapeMarkdownCell(lock.Key.OperationKind),
			)
		}
		if lock.Holder != "" {
			fmt.Fprintf(b, " holder=%s", escapeMarkdownCell(lock.Holder))
		}
		fmt.Fprintln(b)
	}
	if gate := trace.AgentFinalGate; gate != nil {
		fmt.Fprintf(b, "- pending agent final gate: %s", escapeMarkdownCell(gate.Action))
		if len(gate.PendingAgents) > 0 {
			fmt.Fprintf(b, " pending=%s", escapeMarkdownCell(strings.Join(gate.PendingAgents, ", ")))
		}
		if len(gate.Reasons) > 0 {
			fmt.Fprintf(b, " reasons=%s", escapeMarkdownCell(strings.Join(gate.Reasons, ", ")))
		}
		fmt.Fprintln(b)
	}
	for _, notification := range trace.AgentNotifications {
		fmt.Fprintf(b, "- wait_agent notifications: %s", escapeMarkdownCell(notification.Status))
		if notification.AgentID != "" {
			fmt.Fprintf(b, " agent=%s", escapeMarkdownCell(notification.AgentID))
		}
		if notification.Summary != "" {
			fmt.Fprintf(b, " summary=%s", escapeMarkdownCell(redactSecrets(notification.Summary)))
		}
		if len(notification.ResultRefs) > 0 {
			fmt.Fprintf(b, " refs=%s", escapeMarkdownCell(strings.Join(notification.ResultRefs, ", ")))
		}
		fmt.Fprintln(b)
	}
	if report := trace.VerificationAgentReport; report != nil {
		fmt.Fprintf(b, "- verification agent: %s", escapeMarkdownCell(report.Status))
		if report.Summary != "" {
			fmt.Fprintf(b, " summary=%s", escapeMarkdownCell(redactSecrets(report.Summary)))
		}
		if len(report.EvidenceRefs) > 0 {
			fmt.Fprintf(b, " refs=%s", escapeMarkdownCell(strings.Join(report.EvidenceRefs, ", ")))
		}
		fmt.Fprintln(b)
	}
	fmt.Fprintln(b)
}

func agentSchedulingTraceEmpty(trace PromptInputTrace) bool {
	return strings.TrimSpace(trace.AgentIndexHash) == "" &&
		len(trace.AgentIndexEntries) == 0 &&
		len(trace.AgentIndexDropped) == 0 &&
		len(trace.AgentIndexDelta) == 0 &&
		trace.AgentDelegationDecision == nil &&
		len(trace.AgentAssignmentLint) == 0 &&
		len(trace.AgentParallelTraceGroups) == 0 &&
		len(trace.ResourceLocks) == 0 &&
		trace.AgentFinalGate == nil &&
		len(trace.AgentNotifications) == 0 &&
		trace.VerificationAgentReport == nil
}

func verificationSafetyTraceEmpty(trace PromptInputTrace) bool {
	return strings.TrimSpace(trace.VerificationReportRef) == "" &&
		strings.TrimSpace(trace.VerificationStatus) == "" &&
		trace.CompletionGate == nil &&
		len(trace.SafetySignals) == 0 &&
		trace.UnexpectedStateGate == nil &&
		trace.ApprovalScope == nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func contextUsageEmpty(usage ContextUsage) bool {
	return usage.MaxContextTokens == 0 &&
		usage.ReservedOutputTokens == 0 &&
		usage.EstimatedInputTokens == 0 &&
		len(usage.Categories) == 0 &&
		len(usage.TopContributors) == 0
}

// RenderDiffMarkdown renders a redacted human-readable semantic diff.
func RenderDiffMarkdown(diff TraceDiff) string {
	var b strings.Builder
	fmt.Fprintln(&b, "# Prompt Input Diff")
	renderDiffItems(&b, "Added", "+", diff.Added)
	renderDiffItems(&b, "Removed", "-", diff.Removed)
	return b.String()
}

func renderDiffItems(b *strings.Builder, title, marker string, items []TraceItem) {
	fmt.Fprintf(b, "\n## %s\n\n", title)
	if len(items) == 0 {
		fmt.Fprintln(b, "_None._")
		return
	}
	for _, item := range items {
		fmt.Fprintf(
			b,
			"%s `%s/%s`",
			marker,
			item.Source,
			item.SemanticRole,
		)
		if item.ID != "" {
			fmt.Fprintf(b, " id=`%s`", escapeBackticks(item.ID))
		}
		if item.Status != "" {
			fmt.Fprintf(b, " status=`%s`", escapeBackticks(item.Status))
		}
		content := strings.TrimSpace(redactSecrets(item.Content))
		if content != "" {
			fmt.Fprintf(b, "\n\n```text\n%s\n```\n", content)
		} else {
			fmt.Fprintln(b)
		}
	}
}

func escapeMarkdownCell(value string) string {
	value = strings.ReplaceAll(value, "\n", "\\n")
	value = strings.ReplaceAll(value, "|", "\\|")
	return value
}

func escapeMarkdownLine(value string) string {
	value = strings.ReplaceAll(value, "\n", " ")
	return escapeBackticks(value)
}

func escapeBackticks(value string) string {
	return strings.ReplaceAll(value, "`", "'")
}

var (
	secretAssignmentPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password)\s*[:=]\s*[^\s,;]+`)
	bearerPattern           = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
	openAIKeyPattern        = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{8,}\b`)
)

func redactSecrets(content string) string {
	content = secretAssignmentPattern.ReplaceAllString(content, "$1=[REDACTED]")
	content = bearerPattern.ReplaceAllString(content, "Bearer [REDACTED]")
	content = openAIKeyPattern.ReplaceAllString(content, "sk-[REDACTED]")
	return content
}
