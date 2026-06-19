package runtimekernel

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/opsmanual"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/taskdepth"
)

type ModelInputDebugTraceRequest struct {
	SessionID                     string
	TurnID                        string
	Iteration                     int
	Metadata                      map[string]string
	Compiled                      promptcompiler.CompiledPrompt
	ModelInput                    []*schema.Message
	VisibleTools                  []string
	PromptInputTrace              promptinput.PromptInputTrace
	PromptInputDiff               *promptinput.TraceDiff
	DiagnosticTrace               diagnostics.DiagnosticTrace
	TaskDepth                     taskdepth.Profile
	UXProgressTrace               *UXProgressTrace
	EvidenceCoverage              *EvidenceCoverageDecision
	GenericityTrace               *promptinput.GenericityTrace
	PlanRequirementDecision       *promptinput.PlanRequirementDecisionTrace
	PlanCompletionGate            *promptinput.PlanCompletionGateTrace
	ReasoningEffort               string
	ToolSurfaceFingerprint        string
	ToolSurfacePolicySnapshotHash string
	LoadedToolsDelta              []string
	LoadedPacksDelta              []string
	SkillIndexHash                string
	LoadedSkillsDelta             []string
	ToolSearchEvents              []promptinput.ToolSearchTraceEvent
	ToolSelectionEvents           []promptinput.ToolSelectionTraceEvent
	RejectedToolCalls             []promptinput.RejectedToolCallTraceEvent
	SkillSearchEvents             []promptinput.SkillSearchTraceEvent
	SkillReadEvents               []promptinput.SkillReadTraceEvent
	RejectedSkillActivations      []promptinput.RejectedSkillActivationTraceEvent
	MCPInstructionDeltas          []promptinput.MCPInstructionDeltaTrace
	ParallelDispatchGroups        []promptinput.ParallelDispatchTraceGroup
	TaskClaims                    []promptinput.TaskClaimTrace
	FailedToolSummaries           []promptinput.FailedToolSummary
	AgentIndexHash                string
	AgentIndexEntries             []promptinput.AgentIndexEntryTrace
	AgentIndexDropped             []promptinput.DroppedAgentIndexEntryTrace
	AgentIndexDelta               []string
	AgentDelegationDecision       *promptinput.AgentDelegationDecisionTrace
	AgentAssignmentLint           []promptinput.AgentAssignmentLintTrace
	AgentParallelTraceGroups      []promptinput.AgentParallelTraceGroup
	ResourceLocks                 []promptinput.ResourceLockTrace
	AgentFinalGate                *promptinput.AgentFinalGateDecisionTrace
	AgentNotifications            []promptinput.AgentNotificationTrace
	VerificationAgentReport       *promptinput.VerificationAgentReportTrace
	VerificationReportRef         string
	VerificationStatus            string
	CompletionGate                *promptinput.CompletionGateTrace
	SafetySignals                 []promptinput.SafetySignalTrace
	UnexpectedStateGate           *promptinput.UnexpectedStateGateTrace
	ApprovalScope                 *promptinput.ApprovalScopeTrace
	FinalEvidenceState            *FinalEvidenceState
}

func buildModelInput(history []Message, compiled promptcompiler.CompiledPrompt) ([]*schema.Message, error) {
	result, err := buildPromptInput(history, compiled)
	if err != nil {
		return nil, err
	}
	return result.Messages, nil
}

func buildPromptInput(history []Message, compiled promptcompiler.CompiledPrompt) (promptinput.BuildResult, error) {
	return buildPromptInputWithContextGovernance(history, compiled, nil)
}

func buildPromptInputWithContextGovernance(history []Message, compiled promptcompiler.CompiledPrompt, governance []ContextGovernanceEvent) (promptinput.BuildResult, error) {
	result, err := promptinput.Builder{}.Build(promptinput.BuildRequest{
		History:           promptInputMessagesFromRuntime(history),
		Compiled:          compiled,
		ContextGovernance: promptInputContextGovernanceFromRuntime(governance),
	})
	if err != nil {
		return promptinput.BuildResult{}, err
	}
	result.Trace.ContextUsage = AnalyzeContextUsage(ContextUsageInput{
		Compiled:   compiled,
		Messages:   result.Messages,
		Governance: governance,
	})
	return result, nil
}

func modelVisibleMessagesWithObservationDedupe(session *SessionState, history []Message) ([]Message, []ContextGovernanceEvent) {
	if session == nil {
		return append([]Message(nil), history...), nil
	}
	out := append([]Message(nil), history...)
	var events []ContextGovernanceEvent
	for i, msg := range out {
		resourceRecord, resourceOK := resourceReadRecordFromMessage(msg)
		if resourceOK {
			result := session.ObservationState.CheckResource(resourceRecord)
			if result.Event.Layer != "" && result.Event.Kind != "" {
				result.Event.ID = fmt.Sprintf("ctxgov-%s-%d-l2-resource-%d", firstNonBlankRuntimeString(msg.ClientTurnID, msg.ID, "message"), i, len(events))
				result.Event.SessionID = session.ID
				result.Event.TurnID = msg.ClientTurnID
				result.Event = BuildContextGovernanceEvent(result.Event)
				events = append(events, result.Event)
			}
			if result.ModelVisibleContent != "" && msg.ToolResult != nil {
				cp := *msg.ToolResult
				cp.Content = result.ModelVisibleContent
				out[i].ToolResult = &cp
				out[i].Content = result.ModelVisibleContent
			}
			continue
		}
		record, ok := observationRecordFromMessage(msg)
		if !ok {
			continue
		}
		result := session.ObservationState.Check(record)
		if result.Event.Layer != "" && result.Event.Kind != "" {
			result.Event.ID = fmt.Sprintf("ctxgov-%s-%d-l2-%d", firstNonBlankRuntimeString(msg.ClientTurnID, msg.ID, "message"), i, len(events))
			result.Event.SessionID = session.ID
			result.Event.TurnID = msg.ClientTurnID
			result.Event = BuildContextGovernanceEvent(result.Event)
			events = append(events, result.Event)
		}
		if result.ModelVisibleContent == "" || msg.ToolResult == nil {
			continue
		}
		cp := *msg.ToolResult
		cp.Content = result.ModelVisibleContent
		out[i].ToolResult = &cp
		out[i].Content = result.ModelVisibleContent
	}
	return out, events
}

func resourceReadRecordFromMessage(msg Message) (ResourceReadRecord, bool) {
	if msg.ToolResult == nil || len(msg.ToolResult.ExternalReferences) != 1 {
		return ResourceReadRecord{}, false
	}
	ref := msg.ToolResult.ExternalReferences[0]
	uri := firstNonBlankRuntimeString(ref.URI, ref.FilePath, ref.CardRef, ref.ID)
	if strings.TrimSpace(uri) == "" || strings.TrimSpace(ref.Digest) == "" {
		return ResourceReadRecord{}, false
	}
	return ResourceReadRecord{
		Identity: ResourceIdentity{
			URI:     uri,
			Version: firstNonBlankRuntimeString(ref.Version, ref.ID),
			Digest:  ref.Digest,
			Range:   ref.Range,
		},
		SourceRef:   firstNonBlankRuntimeString(ref.ID, uri),
		Summary:     firstNonBlankRuntimeString(ref.Summary, msg.ToolResult.Summary),
		Preview:     contextArtifactBoundedSnippet(msg.ToolResult.Content),
		Content:     msg.ToolResult.Content,
		ContentType: ref.ContentType,
		Bytes:       ref.Bytes,
	}, true
}

func observationRecordFromMessage(msg Message) (ObservationRecord, bool) {
	if msg.ToolResult == nil {
		return ObservationRecord{}, false
	}
	content := strings.TrimSpace(firstNonBlankRuntimeString(msg.ToolResult.Content, msg.Content))
	if content == "" || !strings.HasPrefix(content, "{") {
		return ObservationRecord{}, false
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(content), &payload); err != nil {
		return ObservationRecord{}, false
	}
	key := runtimeStringFromMap(payload, "observationKey")
	if key == "" {
		key = runtimeStringFromMap(payload, "observation_key")
	}
	if key == "" {
		return ObservationRecord{}, false
	}
	sourceRef := firstNonBlankRuntimeString(
		runtimeStringFromMap(payload, "evidenceRef"),
		runtimeStringFromMap(payload, "evidence_ref"),
		runtimeStringFromMap(payload, "sourceRef"),
		runtimeStringFromMap(payload, "source_ref"),
	)
	summary := runtimeStringFromMap(payload, "summary")
	if summary == "" {
		summary = summarizeSnippet(content)
	}
	digest := firstNonBlankRuntimeString(
		runtimeStringFromMap(payload, "digest"),
		runtimeStringFromMap(payload, "contentDigest"),
		runtimeStringFromMap(payload, "content_digest"),
	)
	if digest == "" {
		digest = ObservationDigest(summary)
	}
	return ObservationRecord{
		Key:       key,
		Digest:    digest,
		SourceRef: sourceRef,
		Summary:   summary,
		ToolName:  runtimeStringFromMap(payload, "tool"),
		Target:    runtimeStringFromMap(payload, "target"),
		Window:    runtimeStringFromMap(payload, "window"),
	}, true
}

func promptInputContextGovernanceFromRuntime(events []ContextGovernanceEvent) []promptinput.ContextGovernanceTraceItem {
	if len(events) == 0 {
		return nil
	}
	out := make([]promptinput.ContextGovernanceTraceItem, 0, len(events))
	for _, event := range SortContextGovernanceEvents(events) {
		if event.Layer == "" || event.Kind == "" {
			continue
		}
		item := promptinput.ContextGovernanceTraceItem{
			Layer:        string(event.Layer),
			Kind:         event.Kind,
			Message:      event.Message,
			Budget:       contextBudgetTraceMap(event.Budget),
			ReferenceIDs: append([]string(nil), event.ReferenceIDs...),
			Resource:     promptInputResourceTraceFromRuntime(event.Resource),
			RetryAttempt: event.RetryAttempt,
			RetryMax:     event.RetryMax,
		}
		if len(event.DroppedGroupIDs) > 0 {
			item.ReferenceIDs = append(item.ReferenceIDs, event.DroppedGroupIDs...)
		}
		out = append(out, item)
	}
	return out
}

func promptInputResourceTraceFromRuntime(resource *ContextGovernanceResource) *promptinput.ResourceTraceItem {
	if resource == nil {
		return nil
	}
	return &promptinput.ResourceTraceItem{
		URI:         resource.URI,
		Digest:      resource.Digest,
		ContentType: resource.ContentType,
		Bytes:       resource.Bytes,
		Range:       resource.Range,
	}
}

func contextBudgetTraceMap(budget ContextBudgetThresholds) map[string]int {
	if budget.MaxContextTokens == 0 &&
		budget.ReservedOutputTokens == 0 &&
		budget.EffectiveContextWindow == 0 &&
		budget.WarningThreshold == 0 &&
		budget.AutoCompactThreshold == 0 &&
		budget.BlockingLimit == 0 {
		return nil
	}
	return map[string]int{
		"maxContextTokens":       budget.MaxContextTokens,
		"reservedOutputTokens":   budget.ReservedOutputTokens,
		"effectiveContextWindow": budget.EffectiveContextWindow,
		"warningThreshold":       budget.WarningThreshold,
		"autoCompactThreshold":   budget.AutoCompactThreshold,
		"blockingLimit":          budget.BlockingLimit,
	}
}

func messagesForCurrentTurnModelInput(history []Message) []Message {
	filtered := promptinput.MessagesForCurrentTurnModelInput(promptInputMessagesFromRuntime(history))
	return runtimeMessagesFromPromptInput(filtered)
}

func promptInputMessagesFromRuntime(history []Message) []promptinput.Message {
	out := make([]promptinput.Message, 0, len(history))
	for _, msg := range history {
		if msg.Role == "user" {
			if context := generalOpsOperationFrameContext(msg.Content); context != "" {
				out = append(out, promptinput.Message{
					Role:    "system",
					Content: context,
				})
			}
			if context := generalOpsObservabilityContext(msg.Content); context != "" {
				out = append(out, promptinput.Message{
					Role:    "system",
					Content: context,
				})
			}
		}
		content := msg.Content
		if msg.Role == "tool" {
			content = compactChartPayloadForModel(content)
		}
		toolResult := promptInputToolResultFromRuntime(msg.ToolResult)
		if toolResult != nil {
			toolResult.Content = compactChartPayloadForModel(toolResult.Content)
		}
		out = append(out, promptinput.Message{
			Role:       msg.Role,
			Content:    content,
			ToolCalls:  promptInputToolCallsFromRuntime(msg.ToolCalls),
			ToolResult: toolResult,
		})
	}
	return out
}

func generalOpsOperationFrameContext(input string) string {
	input = strings.TrimSpace(input)
	if input == "" {
		return ""
	}
	frame := opsmanual.BuildOperationFrame(input, nil)
	if len(frame.Roles) == 0 &&
		len(frame.Relationships) == 0 &&
		len(frame.ObservationPoints) == 0 &&
		!frame.RiskPreference.DataLossAcceptable &&
		len(frame.EvidenceRequirements) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Operation Frame v2\n")
	if generalOpsLooksLikeStatefulRepair(frame) {
		b.WriteString("- capability_path: stateful_middleware_cluster_repair\n")
		b.WriteString("- generic_ops_contract: read_only_evidence_first,approval_before_mutation,run_record_required,verify_after_repair\n")
		b.WriteString("- recommended_tool_flow: search_ops_manuals -> run_ops_manual_preflight -> read_only_host_or_provider_probes -> propose_repair_options -> approval_gate -> execute -> verify\n")
		b.WriteString("- answer_requirements: 诊断,恢复方案,风险与审批,验证方式,缺失证据\n")
	}
	if frame.Target.Type != "" || frame.Target.Name != "" {
		b.WriteString("- target: ")
		b.WriteString(firstNonBlankRuntimeString(frame.Target.Type, "unknown"))
		if frame.Target.Name != "" {
			b.WriteString("/")
			b.WriteString(frame.Target.Name)
		}
		b.WriteString("\n")
	}
	if len(frame.Roles) > 0 {
		b.WriteString("- roles:")
		for _, role := range frame.Roles {
			b.WriteString(" ")
			b.WriteString(firstNonBlankRuntimeString(role.Kind, "unknown"))
			b.WriteString(":")
			b.WriteString(firstNonBlankRuntimeString(role.ResourceRef, role.UserLabel, role.ID, "unknown"))
			if role.RuntimeName != "" {
				b.WriteString("(")
				b.WriteString(role.RuntimeName)
				b.WriteString(")")
			}
		}
		b.WriteString("\n")
	}
	if len(frame.Relationships) > 0 {
		b.WriteString("- relationships:")
		for _, rel := range frame.Relationships {
			b.WriteString(" ")
			b.WriteString(rel.From)
			b.WriteString("->")
			b.WriteString(rel.To)
			if rel.Type != "" {
				b.WriteString(":")
				b.WriteString(rel.Type)
			}
		}
		b.WriteString("\n")
	}
	if len(frame.ObservationPoints) > 0 {
		b.WriteString("- observation_points:")
		for _, point := range frame.ObservationPoints {
			b.WriteString(" ")
			b.WriteString(firstNonBlankRuntimeString(point.Kind, "unknown"))
			b.WriteString(":")
			b.WriteString(firstNonBlankRuntimeString(point.ResourceRef, "unknown"))
			if point.Role != "" {
				b.WriteString("(")
				b.WriteString(point.Role)
				b.WriteString(")")
			}
		}
		b.WriteString("\n")
	}
	if frame.ExecutionSurfaceV2.Kind != "" {
		b.WriteString("- execution_surface: ")
		b.WriteString(frame.ExecutionSurfaceV2.Kind)
		if len(frame.ExecutionSurfaceV2.Resources) > 0 {
			b.WriteString(" resources=")
			b.WriteString(strings.Join(frame.ExecutionSurfaceV2.Resources, ","))
		}
		b.WriteString("\n")
	}
	if frame.RiskPreference.DataLossAcceptable || frame.RiskPreference.StillRequiresApproval {
		b.WriteString("- risk_preference: ")
		b.WriteString(fmt.Sprintf("data_loss_acceptable=%t still_requires_approval=%t", frame.RiskPreference.DataLossAcceptable, frame.RiskPreference.StillRequiresApproval))
		b.WriteString("\n")
	}
	if len(frame.EvidenceRequirements) > 0 {
		b.WriteString("- evidence_requirements: ")
		b.WriteString(strings.Join(frame.EvidenceRequirements, ","))
		b.WriteString("\n")
	}
	return strings.TrimSpace(b.String())
}

func generalOpsLooksLikeStatefulRepair(frame opsmanual.OperationFrame) bool {
	if frame.Operation.Stateful || len(frame.Roles) > 0 || len(frame.ObservationPoints) > 0 {
		switch strings.TrimSpace(frame.Operation.Action) {
		case "rca_or_repair", "restore", "repair", "recover":
			return true
		}
		if frame.Risk.DataMutation || frame.RiskPreference.DataLossAcceptable {
			return true
		}
	}
	return false
}

var (
	observabilityDependencyChainPattern = regexp.MustCompile(`(?:调用链|依赖链|服务链)?(?:是|:|：)?\s*([A-Za-z0-9_\-一-龥]+服务(?:\s*->\s*[A-Za-z0-9_\-一-龥]+服务)+)`)
	observabilityEnvironmentPattern     = regexp.MustCompile(`环境([^\s的，。；,;]+)`)
	observabilityTargetServicePattern   = regexp.MustCompile(`(?:环境[^\s的，。；,;]+的|的)\s*([A-Za-z0-9_\-一-龥]+服务)`)
	observabilityProviderMentionPattern = regexp.MustCompile(`@([A-Za-z0-9_\-]+)`)
)

func generalOpsObservabilityContext(input string) string {
	input = strings.TrimSpace(input)
	if input == "" || !generalOpsLooksLikeObservabilityRCA(input) {
		return ""
	}
	provider := extractExplicitObservabilityProvider(input)
	var b strings.Builder
	b.WriteString("Observability RCA contract\n")
	b.WriteString("- capability_path: observability_dependency_chain_rca\n")
	b.WriteString("- generic_ops_contract: provider_neutral_observability,read_only_evidence_first\n")
	b.WriteString("- observability_evidence: dependency_edges,hypotheses,missing_evidence\n")
	if target := extractObservabilityTargetService(input); target != "" {
		b.WriteString("- target_service: ")
		b.WriteString(target)
		b.WriteString("\n")
	}
	if env := extractObservabilityEnvironment(input); env != "" {
		b.WriteString("- environment_hint: ")
		b.WriteString(env)
		b.WriteString("\n")
		b.WriteString("- provider_project_rule: environment_hint is not a provider project unless session metadata or provider discovery explicitly maps it; omit provider project when ambiguous\n")
	}
	if chain := extractObservabilityDependencyChain(input); chain != "" {
		b.WriteString("- dependency_chain_from_user: ")
		b.WriteString(chain)
		b.WriteString("\n")
	}
	if provider != "" && provider != "observability" {
		providerDisplay := observabilityProviderDisplayName(provider)
		b.WriteString("- provider_hint: explicit observability provider requested: ")
		b.WriteString(provider)
		b.WriteString("\n")
		b.WriteString("- first_tool: provider aggregate RCA context tool if visible\n")
		b.WriteString("- tool_order: collect RCA evidence first; use service discovery only when project or target resolution is ambiguous\n")
		b.WriteString("- confidence_calibration_template: high confidence only with ")
		b.WriteString(providerDisplay)
		b.WriteString(" edge evidence\n")
		b.WriteString("- safety_guardrail_template: read-only ")
		b.WriteString(providerDisplay)
		b.WriteString(" evidence first\n")
	}
	b.WriteString("- evidence_rules: use read-only provider evidence before final RCA; state missing_evidence when dependency edges, metrics, logs, traces, incidents, or deployment evidence are unavailable\n")
	b.WriteString("- confidence_rule: high confidence only with provider dependency edge evidence and supporting status or hypothesis evidence\n")
	b.WriteString("- output_signals: capability_path=observability_dependency_chain_rca; generic_ops_contract=provider_neutral_observability,read_only_evidence_first; observability_evidence=dependency_edges,hypotheses,missing_evidence\n")
	b.WriteString("- chain_candidate_template: for a chain X->Y->Z, include root cause candidates exactly as Z依赖异常导致X异常, Y传播上游异常, X自身资源或发布异常 when supported or as hypotheses with evidence limits\n")
	b.WriteString("- output_requirements: include literal machine-signal lines for capability_path, generic_ops_contract, observability_evidence, then include 依赖链,根因,证据,置信度,缺失证据,解决方案,验证方式\n")
	return strings.TrimSpace(b.String())
}

func generalOpsLooksLikeObservabilityRCA(input string) bool {
	lower := strings.ToLower(input)
	if extractExplicitObservabilityProvider(input) != "" {
		return true
	}
	return strings.Contains(input, "调用链") ||
		strings.Contains(input, "依赖链") ||
		strings.Contains(input, "服务链") ||
		(strings.Contains(input, "服务") && strings.Contains(input, "异常")) ||
		strings.Contains(lower, "root cause") ||
		strings.Contains(lower, "rca")
}

func extractExplicitObservabilityProvider(input string) string {
	match := observabilityProviderMentionPattern.FindStringSubmatch(input)
	if len(match) <= 1 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(match[1]))
}

func observabilityProviderDisplayName(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "provider"
	}
	return strings.ToUpper(provider[:1]) + provider[1:]
}

func extractObservabilityDependencyChain(input string) string {
	match := observabilityDependencyChainPattern.FindStringSubmatch(input)
	if len(match) <= 1 {
		return ""
	}
	return strings.ReplaceAll(strings.TrimSpace(match[1]), " ", "")
}

func extractObservabilityEnvironment(input string) string {
	match := observabilityEnvironmentPattern.FindStringSubmatch(input)
	if len(match) <= 1 {
		return ""
	}
	return strings.TrimSpace(strings.Trim(match[1], "，。；,; "))
}

func extractObservabilityTargetService(input string) string {
	if chain := extractObservabilityDependencyChain(input); chain != "" {
		parts := strings.Split(chain, "->")
		if len(parts) > 0 {
			return strings.TrimSpace(parts[0])
		}
	}
	matches := observabilityTargetServicePattern.FindAllStringSubmatch(input, -1)
	for _, match := range matches {
		if len(match) <= 1 {
			continue
		}
		candidate := strings.TrimSpace(strings.Trim(match[1], "，。；,; "))
		if candidate != "" && candidate != "服务" {
			return candidate
		}
	}
	return ""
}

func compactChartPayloadForModel(content string) string {
	trimmed := strings.TrimSpace(content)
	if trimmed == "" {
		return content
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(trimmed), &payload); err != nil {
		return content
	}
	chartSummary := runtimeGenericChartSummaryFromPayload(payload)
	if len(chartSummary) == 0 {
		return content
	}
	out := map[string]any{
		"schemaVersion": "aiops.chart_summary/v1",
		"chartSummary":  chartSummary,
	}
	if toolName := runtimeStringFromMap(payload, "tool"); toolName != "" {
		out["tool"] = toolName
	}
	for _, key := range []string{"status", "project", "service", "source", "resource", "resourceId", "resourceType"} {
		if value := runtimeStringFromMap(payload, key); value != "" {
			out[key] = value
		}
	}
	if rawRef := runtimeStringAnyMap(payload["rawRef"]); len(rawRef) > 0 {
		compactRef := map[string]any{}
		for _, key := range []string{"uri", "digest", "bytes"} {
			if value, ok := rawRef[key]; ok {
				compactRef[key] = value
			}
		}
		if len(compactRef) > 0 {
			out["rawRef"] = compactRef
		}
	}
	data, err := json.Marshal(out)
	if err != nil {
		return content
	}
	return string(data)
}

func runtimeGenericChartSummaryFromPayload(payload map[string]any) map[string]any {
	summary := runtimeCloneStringAnyMap(runtimeStringAnyMap(payload["chartSummary"]))
	if len(summary) == 0 {
		summary = map[string]any{}
		if metricSummaries := runtimeGenericMetricSummaries(payload["metrics"]); len(metricSummaries) > 0 {
			summary["metricSummaries"] = metricSummaries
		}
		if reports := runtimeGenericReportSummaries(payload["chartReports"]); len(reports) > 0 {
			summary["reports"] = reports
		}
	}
	if service := runtimeStringFromMap(payload, "service"); service != "" {
		summary["service"] = service
	}
	return summary
}

func runtimeGenericMetricSummaries(value any) []map[string]any {
	var out []map[string]any
	for _, metric := range runtimeStringAnyMapList(value) {
		name := runtimeStringFromMap(metric, "name")
		item := map[string]any{
			"name":  name,
			"topic": runtimeGenericTopicFromName(firstNonBlankRuntimeString(name, runtimeStringFromMap(metric, "chartTitle"))),
		}
		for _, key := range []string{"status", "value", "unit", "chartTitle"} {
			if text := runtimeStringFromMap(metric, key); text != "" {
				item[key] = text
			}
		}
		series := runtimeStringAnyMapList(metric["series"])
		if len(series) > 0 {
			item["seriesCount"] = len(series)
			pointCount := 0
			var seriesNames []string
			for _, seriesMap := range series {
				pointCount += len(runtimeAnyList(seriesMap["values"]))
				seriesNames = appendRuntimeUniqueString(seriesNames, runtimeStringFromMap(seriesMap, "name"), 5)
			}
			if pointCount > 0 {
				item["pointCount"] = pointCount
			}
			if len(seriesNames) > 0 {
				item["seriesNames"] = seriesNames
			}
		} else if pointCount := len(runtimeAnyList(metric["values"])); pointCount > 0 {
			item["seriesCount"] = 1
			item["pointCount"] = pointCount
		}
		out = append(out, item)
	}
	return out
}

func runtimeGenericReportSummaries(value any) []map[string]any {
	var out []map[string]any
	for _, report := range runtimeStringAnyMapList(value) {
		name := runtimeStringFromMap(report, "name")
		item := map[string]any{
			"name":  name,
			"topic": runtimeGenericTopicFromName(name),
		}
		if status := runtimeStringFromMap(report, "status"); status != "" {
			item["status"] = status
		}
		chartCount := 0
		seriesCount := 0
		pointCount := 0
		var titles []string
		var seriesNames []string
		for _, widget := range runtimeStringAnyMapList(report["widgets"]) {
			if chart := runtimeStringAnyMap(widget["chart"]); len(chart) > 0 {
				chartCount++
				title := firstNonBlankRuntimeString(runtimeStringFromMap(widget, "title"), runtimeStringFromMap(chart, "title"))
				titles = appendRuntimeUniqueString(titles, title, 5)
				if item["topic"] == "" {
					item["topic"] = runtimeGenericTopicFromName(title)
				}
				sc, pc, names := runtimeGenericSeriesCounts(chart)
				seriesCount += sc
				pointCount += pc
				for _, name := range names {
					seriesNames = appendRuntimeUniqueString(seriesNames, name, 5)
				}
			}
			group := runtimeStringAnyMap(widget["chart_group"])
			if len(group) == 0 {
				continue
			}
			groupTitle := runtimeStringFromMap(group, "title")
			for _, chart := range runtimeStringAnyMapList(group["charts"]) {
				chartCount++
				title := firstNonBlankRuntimeString(groupTitle, runtimeStringFromMap(chart, "title"))
				titles = appendRuntimeUniqueString(titles, title, 5)
				if item["topic"] == "" {
					item["topic"] = runtimeGenericTopicFromName(title)
				}
				sc, pc, names := runtimeGenericSeriesCounts(chart)
				seriesCount += sc
				pointCount += pc
				for _, name := range names {
					seriesNames = appendRuntimeUniqueString(seriesNames, name, 5)
				}
			}
		}
		if chartCount > 0 {
			item["chartCount"] = chartCount
		}
		if seriesCount > 0 {
			item["seriesCount"] = seriesCount
		}
		if pointCount > 0 {
			item["pointCount"] = pointCount
		}
		if len(titles) > 0 {
			item["titles"] = titles
		}
		if len(seriesNames) > 0 {
			item["seriesNames"] = seriesNames
		}
		out = append(out, item)
	}
	return out
}

func runtimeGenericSeriesCounts(chart map[string]any) (int, int, []string) {
	seriesCount := 0
	pointCount := 0
	var names []string
	for _, series := range runtimeStringAnyMapList(chart["series"]) {
		seriesCount++
		pointCount += len(runtimeAnyList(series["data"]))
		names = appendRuntimeUniqueString(names, runtimeStringFromMap(series, "name"), 5)
	}
	if threshold := runtimeStringAnyMap(chart["threshold"]); len(threshold) > 0 {
		pointCount += len(runtimeAnyList(threshold["data"]))
	}
	return seriesCount, pointCount, names
}

func runtimeGenericTopicFromName(name string) string {
	normalized := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(normalized, "net"), strings.Contains(normalized, "network"), strings.Contains(normalized, "tcp"):
		return "net"
	case strings.Contains(normalized, "cpu"):
		return "cpu"
	case strings.Contains(normalized, "memory"), strings.Contains(normalized, "mem"), strings.Contains(normalized, "rss"):
		return "memory"
	case strings.Contains(normalized, "instances"), strings.Contains(normalized, "instance"):
		return "instances"
	default:
		return ""
	}
}

func runtimeStringAnyMap(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	return nil
}

func runtimeStringAnyMapList(value any) []map[string]any {
	items, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		if record, ok := item.(map[string]any); ok {
			out = append(out, record)
		}
	}
	return out
}

func runtimeAnyList(value any) []any {
	if typed, ok := value.([]any); ok {
		return typed
	}
	return nil
}

func runtimeCloneStringAnyMap(source map[string]any) map[string]any {
	if source == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(source))
	for key, value := range source {
		out[key] = value
	}
	return out
}

func runtimeStringFromMap(payload map[string]any, key string) string {
	raw, ok := payload[key]
	if !ok {
		return ""
	}
	if text, ok := raw.(string); ok {
		return strings.TrimSpace(text)
	}
	return ""
}

func firstNonBlankRuntimeString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func appendRuntimeUniqueString(values []string, value string, limit int) []string {
	value = strings.TrimSpace(value)
	if value == "" || (limit > 0 && len(values) >= limit) {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}

func promptInputToolCallsFromRuntime(toolCalls []ToolCall) []promptinput.ToolCall {
	out := make([]promptinput.ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, promptinput.ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func promptInputToolResultFromRuntime(result *ToolResult) *promptinput.ToolResult {
	if result == nil {
		return nil
	}
	return &promptinput.ToolResult{
		ToolCallID: result.ToolCallID,
		Content:    result.Content,
	}
}

func runtimeMessagesFromPromptInput(messages []promptinput.Message) []Message {
	out := make([]Message, 0, len(messages))
	for _, msg := range messages {
		out = append(out, Message{
			Role:       msg.Role,
			Content:    msg.Content,
			ToolCalls:  runtimeToolCallsFromPromptInput(msg.ToolCalls),
			ToolResult: runtimeToolResultFromPromptInput(msg.ToolResult),
		})
	}
	return out
}

func runtimeToolCallsFromPromptInput(toolCalls []promptinput.ToolCall) []ToolCall {
	out := make([]ToolCall, 0, len(toolCalls))
	for _, call := range toolCalls {
		out = append(out, ToolCall{
			ID:        call.ID,
			Name:      call.Name,
			Arguments: call.Arguments,
		})
	}
	return out
}

func runtimeToolResultFromPromptInput(result *promptinput.ToolResult) *ToolResult {
	if result == nil {
		return nil
	}
	return &ToolResult{
		ToolCallID: result.ToolCallID,
		Content:    result.Content,
	}
}

func writeModelInputDebugTrace(req ModelInputDebugTraceRequest) (string, error) {
	promptTrace := req.PromptInputTrace
	if len(promptTrace.PromptSections) == 0 {
		promptTrace.PromptSections = append([]promptcompiler.PromptSectionTrace(nil), req.Compiled.PromptSections...)
	}
	if len(promptTrace.ChangedSections) == 0 {
		promptTrace.ChangedSections = append([]promptcompiler.ChangedPromptSection(nil), req.Compiled.ChangedSections...)
	}
	if promptInputContextUsageEmpty(promptTrace.ContextUsage) {
		promptTrace.ContextUsage = AnalyzeContextUsage(ContextUsageInput{
			Compiled: req.Compiled,
			Messages: req.ModelInput,
		})
	}
	if len(promptTrace.VisibleOpsManualTools) == 0 {
		promptTrace.VisibleOpsManualTools = visibleOpsManualToolsFromNames(req.VisibleTools)
	}
	if promptTrace.TaskDepth == nil && req.TaskDepth.Level != "" {
		promptTrace.TaskDepth = promptInputTaskDepthTrace(req.TaskDepth)
	}
	if promptTrace.EvidenceCoverage == nil && req.EvidenceCoverage != nil {
		promptTrace.EvidenceCoverage = promptInputEvidenceCoverageTrace(*req.EvidenceCoverage)
	}
	if promptTrace.GenericityTrace == nil && req.GenericityTrace != nil {
		genericity := *req.GenericityTrace
		genericity.CoreRuleDomainTerms = append([]string(nil), req.GenericityTrace.CoreRuleDomainTerms...)
		genericity.AllowedFixtureTerms = append([]string(nil), req.GenericityTrace.AllowedFixtureTerms...)
		genericity.AllowedPluginTerms = append([]string(nil), req.GenericityTrace.AllowedPluginTerms...)
		genericity.Violations = append([]string(nil), req.GenericityTrace.Violations...)
		promptTrace.GenericityTrace = &genericity
	}
	if promptTrace.ToolSurfaceFingerprint == "" {
		promptTrace.ToolSurfaceFingerprint = req.ToolSurfaceFingerprint
	}
	if promptTrace.ToolSurfacePolicySnapshotHash == "" {
		promptTrace.ToolSurfacePolicySnapshotHash = req.ToolSurfacePolicySnapshotHash
	}
	if len(promptTrace.DeferredToolDirectory) == 0 {
		promptTrace.DeferredToolDirectory = cloneDeferredToolDirectoryForTrace(req.Compiled.Tools.DeferredDirectory)
	}
	if len(promptTrace.LoadedToolsDelta) == 0 {
		promptTrace.LoadedToolsDelta = append([]string(nil), req.LoadedToolsDelta...)
	}
	if len(promptTrace.LoadedPacksDelta) == 0 {
		promptTrace.LoadedPacksDelta = append([]string(nil), req.LoadedPacksDelta...)
	}
	if strings.TrimSpace(promptTrace.SkillIndexHash) == "" {
		promptTrace.SkillIndexHash = strings.TrimSpace(req.SkillIndexHash)
	}
	if len(promptTrace.LoadedSkillsDelta) == 0 {
		promptTrace.LoadedSkillsDelta = append([]string(nil), req.LoadedSkillsDelta...)
	}
	if len(promptTrace.ToolSearchEvents) == 0 {
		promptTrace.ToolSearchEvents = append([]promptinput.ToolSearchTraceEvent(nil), req.ToolSearchEvents...)
	}
	if len(promptTrace.ToolSelectionEvents) == 0 {
		promptTrace.ToolSelectionEvents = append([]promptinput.ToolSelectionTraceEvent(nil), req.ToolSelectionEvents...)
	}
	if len(promptTrace.RejectedToolCalls) == 0 {
		promptTrace.RejectedToolCalls = append([]promptinput.RejectedToolCallTraceEvent(nil), req.RejectedToolCalls...)
	}
	if len(promptTrace.SkillSearchEvents) == 0 {
		promptTrace.SkillSearchEvents = append([]promptinput.SkillSearchTraceEvent(nil), req.SkillSearchEvents...)
	}
	if len(promptTrace.SkillReadEvents) == 0 {
		promptTrace.SkillReadEvents = append([]promptinput.SkillReadTraceEvent(nil), req.SkillReadEvents...)
	}
	if len(promptTrace.RejectedSkillActivations) == 0 {
		promptTrace.RejectedSkillActivations = append([]promptinput.RejectedSkillActivationTraceEvent(nil), req.RejectedSkillActivations...)
	}
	if len(promptTrace.MCPInstructionDeltas) == 0 {
		promptTrace.MCPInstructionDeltas = append([]promptinput.MCPInstructionDeltaTrace(nil), req.MCPInstructionDeltas...)
	}
	if len(promptTrace.ParallelDispatchGroups) == 0 {
		promptTrace.ParallelDispatchGroups = append([]promptinput.ParallelDispatchTraceGroup(nil), req.ParallelDispatchGroups...)
	}
	if len(promptTrace.TaskClaims) == 0 {
		promptTrace.TaskClaims = append([]promptinput.TaskClaimTrace(nil), req.TaskClaims...)
	}
	if len(promptTrace.FailedToolSummaries) == 0 {
		promptTrace.FailedToolSummaries = append([]promptinput.FailedToolSummary(nil), req.FailedToolSummaries...)
	}
	if strings.TrimSpace(promptTrace.AgentIndexHash) == "" {
		promptTrace.AgentIndexHash = strings.TrimSpace(req.AgentIndexHash)
	}
	if len(promptTrace.AgentIndexEntries) == 0 {
		promptTrace.AgentIndexEntries = append([]promptinput.AgentIndexEntryTrace(nil), req.AgentIndexEntries...)
	}
	if len(promptTrace.AgentIndexDropped) == 0 {
		promptTrace.AgentIndexDropped = append([]promptinput.DroppedAgentIndexEntryTrace(nil), req.AgentIndexDropped...)
	}
	if len(promptTrace.AgentIndexDelta) == 0 {
		promptTrace.AgentIndexDelta = append([]string(nil), req.AgentIndexDelta...)
	}
	if promptTrace.AgentDelegationDecision == nil && req.AgentDelegationDecision != nil {
		decision := *req.AgentDelegationDecision
		promptTrace.AgentDelegationDecision = &decision
	}
	if len(promptTrace.AgentAssignmentLint) == 0 {
		promptTrace.AgentAssignmentLint = append([]promptinput.AgentAssignmentLintTrace(nil), req.AgentAssignmentLint...)
	}
	if len(promptTrace.AgentParallelTraceGroups) == 0 {
		promptTrace.AgentParallelTraceGroups = append([]promptinput.AgentParallelTraceGroup(nil), req.AgentParallelTraceGroups...)
	}
	if len(promptTrace.ResourceLocks) == 0 {
		promptTrace.ResourceLocks = append([]promptinput.ResourceLockTrace(nil), req.ResourceLocks...)
	}
	if promptTrace.AgentFinalGate == nil && req.AgentFinalGate != nil {
		gate := *req.AgentFinalGate
		promptTrace.AgentFinalGate = &gate
	}
	if len(promptTrace.AgentNotifications) == 0 {
		promptTrace.AgentNotifications = append([]promptinput.AgentNotificationTrace(nil), req.AgentNotifications...)
	}
	if promptTrace.VerificationAgentReport == nil && req.VerificationAgentReport != nil {
		report := *req.VerificationAgentReport
		promptTrace.VerificationAgentReport = &report
	}
	if strings.TrimSpace(promptTrace.VerificationReportRef) == "" {
		promptTrace.VerificationReportRef = strings.TrimSpace(req.VerificationReportRef)
	}
	if strings.TrimSpace(promptTrace.VerificationStatus) == "" {
		promptTrace.VerificationStatus = strings.TrimSpace(req.VerificationStatus)
	}
	if promptTrace.CompletionGate == nil && req.CompletionGate != nil {
		gate := *req.CompletionGate
		gate.Reasons = append([]string(nil), req.CompletionGate.Reasons...)
		promptTrace.CompletionGate = &gate
	}
	if len(promptTrace.SafetySignals) == 0 {
		promptTrace.SafetySignals = append([]promptinput.SafetySignalTrace(nil), req.SafetySignals...)
	}
	if promptTrace.UnexpectedStateGate == nil && req.UnexpectedStateGate != nil {
		gate := *req.UnexpectedStateGate
		gate.Sources = append([]string(nil), req.UnexpectedStateGate.Sources...)
		gate.AffectedScopes = append([]string(nil), req.UnexpectedStateGate.AffectedScopes...)
		gate.Reasons = append([]string(nil), req.UnexpectedStateGate.Reasons...)
		promptTrace.UnexpectedStateGate = &gate
	}
	if promptTrace.ApprovalScope == nil && req.ApprovalScope != nil {
		scope := *req.ApprovalScope
		scope.AllowedActions = append([]string(nil), req.ApprovalScope.AllowedActions...)
		scope.ResourceScopes = append([]string(nil), req.ApprovalScope.ResourceScopes...)
		scope.Reasons = append([]string(nil), req.ApprovalScope.Reasons...)
		promptTrace.ApprovalScope = &scope
	}
	if promptTrace.PlanRequirementDecision == nil && req.PlanRequirementDecision != nil {
		decision := *req.PlanRequirementDecision
		decision.Signals = append([]string(nil), req.PlanRequirementDecision.Signals...)
		promptTrace.PlanRequirementDecision = &decision
	}
	if promptTrace.PlanCompletionGate == nil && req.PlanCompletionGate != nil {
		gate := *req.PlanCompletionGate
		gate.Reasons = append([]string(nil), req.PlanCompletionGate.Reasons...)
		promptTrace.PlanCompletionGate = &gate
	}
	metadata := map[string]string{}
	for key, value := range req.Metadata {
		metadata[key] = value
	}
	if req.TaskDepth.Level != "" {
		metadata["taskDepth.level"] = string(req.TaskDepth.Level)
		metadata["taskDepth.requiresPlan"] = fmt.Sprint(req.TaskDepth.RequiresPlan)
		metadata["taskDepth.requiresEvidence"] = fmt.Sprint(req.TaskDepth.RequiresEvidence)
	}
	if req.UXProgressTrace != nil {
		metadata["uxProgress.phase"] = strings.TrimSpace(req.UXProgressTrace.Phase)
		metadata["uxProgress.currentStepId"] = strings.TrimSpace(req.UXProgressTrace.CurrentStepID)
		metadata["uxProgress.pendingApprovals"] = strings.Join(req.UXProgressTrace.PendingApprovals, ",")
	}
	if req.EvidenceCoverage != nil {
		metadata["evidenceCoverage.action"] = strings.TrimSpace(req.EvidenceCoverage.Action)
		metadata["evidenceCoverage.missingDimensions"] = strings.Join(req.EvidenceCoverage.MissingDimensions, ",")
	}
	if effort := strings.TrimSpace(req.ReasoningEffort); effort != "" {
		metadata["reasoningEffort.configured"] = effort
	}
	return modeltrace.Write(modeltrace.Request{
		Kind:                          "runtime_model_input",
		SessionID:                     req.SessionID,
		TurnID:                        req.TurnID,
		Iteration:                     req.Iteration,
		Metadata:                      metadata,
		VisibleTools:                  req.VisibleTools,
		PromptFingerprint:             promptFingerprintMap(req.Compiled.Fingerprint),
		ToolSurfaceFingerprint:        promptTrace.ToolSurfaceFingerprint,
		ToolSurfacePolicySnapshotHash: promptTrace.ToolSurfacePolicySnapshotHash,
		LoadedToolsDelta:              promptTrace.LoadedToolsDelta,
		LoadedPacksDelta:              promptTrace.LoadedPacksDelta,
		SkillIndexHash:                promptTrace.SkillIndexHash,
		LoadedSkillsDelta:             promptTrace.LoadedSkillsDelta,
		ToolSearchEvents:              promptTrace.ToolSearchEvents,
		ToolSelectionEvents:           promptTrace.ToolSelectionEvents,
		RejectedToolCalls:             promptTrace.RejectedToolCalls,
		SkillSearchEvents:             promptTrace.SkillSearchEvents,
		SkillReadEvents:               promptTrace.SkillReadEvents,
		RejectedSkillActivations:      promptTrace.RejectedSkillActivations,
		MCPInstructionDeltas:          promptTrace.MCPInstructionDeltas,
		ParallelDispatchGroups:        promptTrace.ParallelDispatchGroups,
		TaskClaims:                    promptTrace.TaskClaims,
		FailedToolSummaries:           promptTrace.FailedToolSummaries,
		VerificationReportRef:         promptTrace.VerificationReportRef,
		VerificationStatus:            promptTrace.VerificationStatus,
		CompletionGate:                promptTrace.CompletionGate,
		SafetySignals:                 promptTrace.SafetySignals,
		UnexpectedStateGate:           promptTrace.UnexpectedStateGate,
		ApprovalScope:                 promptTrace.ApprovalScope,
		PlanRequirementDecision:       promptTrace.PlanRequirementDecision,
		PlanCompletionGate:            promptTrace.PlanCompletionGate,
		FinalEvidenceState:            req.FinalEvidenceState,
		Prompt: modeltrace.Prompt{
			StableHash: promptContentHash(req.Compiled.Stable.Content),
			Stable:     req.Compiled.Stable.Content,
			Dynamic:    req.Compiled.Dynamic.Content,
			System:     effectiveSystemPrompt(req.Compiled).Content,
			Developer:  effectiveDeveloperInstructions(req.Compiled).Content,
			Tools:      effectiveToolPromptSet(req.Compiled).Content,
			Policy:     effectiveRuntimePolicyPrompt(req.Compiled).Content,
		},
		ModelInput:       req.ModelInput,
		PromptInputTrace: promptTrace,
		PromptInputDiff:  req.PromptInputDiff,
		DiagnosticTrace:  req.DiagnosticTrace,
	})
}

func cloneDeferredToolDirectoryForTrace(entries []promptcompiler.DeferredToolDirectoryEntry) []promptcompiler.DeferredToolDirectoryEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]promptcompiler.DeferredToolDirectoryEntry, 0, len(entries))
	for _, entry := range entries {
		entry.ResourceTypes = append([]string(nil), entry.ResourceTypes...)
		entry.OperationKinds = append([]string(nil), entry.OperationKinds...)
		out = append(out, entry)
	}
	return out
}

func promptInputTaskDepthTrace(profile taskdepth.Profile) *promptinput.TaskDepthTrace {
	if profile.Level == "" {
		return nil
	}
	return &promptinput.TaskDepthTrace{
		Level:              string(profile.Level),
		Reasons:            append([]string(nil), profile.Reasons...),
		RequiresPlan:       profile.RequiresPlan,
		RequiresEvidence:   profile.RequiresEvidence,
		RequiresValidation: profile.RequiresValidation,
	}
}

func promptInputEvidenceCoverageTrace(decision EvidenceCoverageDecision) *promptinput.EvidenceCoverageTrace {
	return &promptinput.EvidenceCoverageTrace{
		Action:             strings.TrimSpace(decision.Action),
		Coverage:           decision.Coverage,
		RequiredDimensions: append([]string(nil), decision.RequiredDimensions...),
		CoveredDimensions:  append([]string(nil), decision.CoveredDimensions...),
		MissingDimensions:  append([]string(nil), decision.MissingDimensions...),
		OpenQuestions:      append([]string(nil), decision.OpenQuestions...),
		VerificationStatus: strings.TrimSpace(decision.VerificationStatus),
		Reasons:            append([]string(nil), decision.Reasons...),
	}
}

func visibleOpsManualToolsFromNames(names []string) []string {
	var out []string
	for _, name := range names {
		switch strings.TrimSpace(name) {
		case "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight":
			out = append(out, strings.TrimSpace(name))
		}
	}
	return out
}

func promptInputContextUsageEmpty(usage promptinput.ContextUsage) bool {
	return usage.MaxContextTokens == 0 &&
		usage.ReservedOutputTokens == 0 &&
		usage.EstimatedInputTokens == 0 &&
		len(usage.Categories) == 0 &&
		len(usage.TopContributors) == 0
}

func promptFingerprintMap(fp promptcompiler.PromptFingerprint) map[string]string {
	out := map[string]string{}
	add := func(key, value string) {
		if strings.TrimSpace(value) != "" {
			out[key] = value
		}
	}
	add("version", fp.Version)
	add("compilerVersion", fp.CompilerVersion)
	add("stableHash", fp.StableHash)
	add("systemHash", fp.SystemHash)
	add("developerHash", fp.DeveloperHash)
	add("toolRegistryHash", fp.ToolRegistryHash)
	add("runtimePolicyHash", fp.RuntimePolicyHash)
	add("protocolStateHash", fp.ProtocolStateHash)
	if len(out) == 0 {
		return nil
	}
	return out
}

func effectiveSystemPrompt(compiled promptcompiler.CompiledPrompt) promptcompiler.SystemPrompt {
	if compiled.System.Content != "" || compiled.System.Role != "" || compiled.System.Environment != "" {
		return compiled.System
	}
	return compiled.Stable.System
}

func effectiveDeveloperInstructions(compiled promptcompiler.CompiledPrompt) promptcompiler.DeveloperInstructions {
	if compiled.Developer.Content != "" || len(compiled.Developer.Constraints) > 0 {
		return compiled.Developer
	}
	return compiled.Stable.Developer
}

func effectiveToolPromptSet(compiled promptcompiler.CompiledPrompt) promptcompiler.ToolPromptSet {
	if compiled.Tools.Content != "" || len(compiled.Tools.Entries) > 0 {
		return compiled.Tools
	}
	return compiled.Stable.Tools
}

func effectiveRuntimePolicyPrompt(compiled promptcompiler.CompiledPrompt) promptcompiler.RuntimePolicyPrompt {
	if compiled.Policy.Content != "" || compiled.Policy.Mode != "" {
		return compiled.Policy
	}
	return compiled.Dynamic.Policy
}
