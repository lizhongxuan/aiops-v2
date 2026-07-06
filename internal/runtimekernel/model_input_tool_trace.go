package runtimekernel

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/policyengine"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/specialinputmemory"
	"aiops-v2/internal/tooling"
)

type ModelInputToolTraceFields struct {
	AssemblySource                string
	PromptCompilerSource          string
	ToolSurfaceSource             string
	AdapterName                   string
	ToolSurfaceFingerprint        string
	ToolSurfacePolicySnapshotHash string
	ToolSurfaceSnapshot           *promptinput.ToolSurfaceSnapshot
	PublicWebBudget               *promptinput.PublicWebBudgetTrace
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
	ResourceBindings              []resourcebinding.ResourceBindingSnapshot
	ResourceRoleBindings          []resourcebinding.ResourceRoleBinding
	ResourceCapabilities          []resourcebinding.ResourceCapability
	ResourceEvidenceRefs          []resourcebinding.EvidenceRef
	SpecialInputWorldState        *specialinputmemory.SpecialInputWorldStateSection
	ResourceLocks                 []promptinput.ResourceLockTrace
	OwnerWriteTraces              []OwnerWriteTrace
	FailedToolSummaries           []promptinput.FailedToolSummary
	SafetySignals                 []promptinput.SafetySignalTrace
	UnexpectedStateGate           *promptinput.UnexpectedStateGateTrace
	ApprovalScope                 *promptinput.ApprovalScopeTrace
}

func buildModelInputToolTraceFields(session *SessionState, snapshot *TurnSnapshot, toolSurfaceFingerprint, policySnapshotHash string) ModelInputToolTraceFields {
	fields := ModelInputToolTraceFields{
		AssemblySource:                "runtimekernel.buildModelInputTraceRequest",
		PromptCompilerSource:          "promptcompiler.Compiler",
		ToolSurfaceSource:             "runtimekernel.applyToolSurfacePolicyToCompileContext",
		AdapterName:                   "eino",
		ToolSurfaceFingerprint:        strings.TrimSpace(toolSurfaceFingerprint),
		ToolSurfacePolicySnapshotHash: strings.TrimSpace(policySnapshotHash),
		PublicWebBudget:               promptInputPublicWebBudgetTrace(DefaultPublicWebBudget()),
	}
	if session != nil {
		fields.LoadedToolsDelta = session.ToolDiscovery.EnabledTools()
		fields.LoadedPacksDelta = session.ToolDiscovery.EnabledPacks()
		fields.ToolSearchEvents = toolSearchTraceEventsFromDiscovery(session.ToolDiscovery)
		fields.ToolSelectionEvents = toolSelectionTraceEventsFromDiscovery(session.ToolDiscovery)
		fields.RejectedToolCalls = rejectedToolCallTraceEventsFromDiscovery(session.ToolDiscovery)
		fields.SkillIndexHash = session.SkillActivation.SkillIndexHash
		fields.LoadedSkillsDelta = session.SkillActivation.EnabledSkills()
		fields.SkillSearchEvents = skillSearchTraceEventsFromActivation(session.SkillActivation)
		fields.SkillReadEvents = skillReadTraceEventsFromActivation(session.SkillActivation)
		fields.RejectedSkillActivations = rejectedSkillActivationTraceEventsFromActivation(session.SkillActivation)
		fields.MCPInstructionDeltas = mcpInstructionDeltaTraceEvents(session.MCPInstructions)
		fields.SafetySignals = safetySignalTracesFromPendingApprovals(sessionPendingApprovals(session))
		fields.UnexpectedStateGate = unexpectedStateGateTraceFromSignals(collectUnexpectedStateSignalsFromSession(session))
		fields.ApprovalScope = approvalScopeTraceFromSession(session)
		fields.OwnerWriteTraces = append(fields.OwnerWriteTraces, session.OwnerWriteTraces...)
	}
	if snapshot != nil {
		if snapshot.ToolSurfaceSnapshot != nil {
			fields.ToolSurfaceSnapshot = promptToolSurfaceSnapshotFromRuntime(snapshot.ToolSurfaceSnapshot, fields.LoadedPacksDelta)
		}
		fields.ParallelDispatchGroups = parallelDispatchTraceGroupsFromSnapshot(snapshot)
		fields.ResourceLocks = resourceLockTracesFromSnapshot(snapshot)
		fields.FailedToolSummaries = failedToolSummariesFromSnapshot(snapshot)
		fields.OwnerWriteTraces = append(fields.OwnerWriteTraces, snapshot.OwnerWriteTraces...)
		if snapshot.SpecialInputReadPlan != nil {
			fields.SpecialInputWorldState = specialinputmemory.BuildWorldStateSection(*snapshot.SpecialInputReadPlan)
		}
	}
	return fields
}

func resourceCapabilitiesFromAssembledTools(bindings []resourcebinding.ResourceBindingSnapshot, tools []tooling.Tool, policyHash string) []resourcebinding.ResourceCapability {
	if len(bindings) == 0 || len(tools) == 0 {
		return nil
	}
	metas := make([]tooling.ToolMetadata, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		metas = append(metas, tool.Metadata())
	}
	inputs := resourcebinding.ToolCapabilityInputsFromMetadata(metas, policyHash)
	if len(inputs) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]resourcebinding.ResourceCapability, 0, len(inputs))
	for _, binding := range bindings {
		for _, capability := range resourcebinding.BuildCapabilities(binding, inputs) {
			key := capability.TraceHash
			if key == "" {
				key = capability.ResourceRef.IdentityHash() + "\x00" + capability.Capability + "\x00" + strings.Join(capability.ToolNames, "\x00")
			}
			if key == "" {
				continue
			}
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			out = append(out, capability)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ResourceRef.ID != out[j].ResourceRef.ID {
			return out[i].ResourceRef.ID < out[j].ResourceRef.ID
		}
		if out[i].Capability != out[j].Capability {
			return out[i].Capability < out[j].Capability
		}
		return strings.Join(out[i].ToolNames, "\x00") < strings.Join(out[j].ToolNames, "\x00")
	})
	return out
}

func promptInputPublicWebBudgetTrace(budget PublicWebBudget) *promptinput.PublicWebBudgetTrace {
	budget = normalizePublicWebBudget(budget)
	return &promptinput.PublicWebBudgetTrace{
		MaxSearchCalls:        budget.MaxSearchCalls,
		MaxQueries:            budget.MaxQueries,
		MaxResults:            budget.MaxResults,
		MaxCallsPerTurn:       budget.MaxCallsPerTurn,
		MaxQueriesPerCall:     budget.MaxQueriesPerCall,
		MaxResultsPerDomain:   budget.MaxResultsPerDomain,
		ExplicitUserRequested: budget.ExplicitUserRequested,
	}
}

func promptToolSurfaceSnapshotFromRuntime(ref *ToolSurfaceSnapshotRef, loadedPacksDelta []string) *promptinput.ToolSurfaceSnapshot {
	if ref == nil {
		return nil
	}
	out := &promptinput.ToolSurfaceSnapshot{
		Fingerprint:      strings.TrimSpace(ref.Fingerprint),
		VisibleTools:     uniqueSortedTraceStrings(ref.ToolNames),
		LoadedPacksDelta: uniqueSortedTraceStrings(loadedPacksDelta),
		PolicyHash:       firstNonEmpty(ref.PolicySnapshotHash, policyHashFromToolSurfacePolicy(ref.PolicySnapshot)),
	}
	if ref.PolicySnapshot != nil {
		out.HiddenReasons = hiddenReasonsFromToolSurfacePolicy(*ref.PolicySnapshot)
		for name := range out.HiddenReasons {
			out.HiddenTools = append(out.HiddenTools, name)
		}
		sort.Strings(out.HiddenTools)
	}
	if toolSurfaceSnapshotEmpty(out) {
		return nil
	}
	return out
}

func hiddenReasonsFromToolSurfacePolicy(snapshot tooling.ToolSurfacePolicySnapshot) map[string][]string {
	reasons := map[string][]string{}
	for _, hidden := range snapshot.HiddenTools {
		name := strings.TrimSpace(hidden.Name)
		reason := strings.TrimSpace(hidden.Reason)
		if name == "" || reason == "" {
			continue
		}
		reasons[name] = appendUniqueTraceStrings(reasons[name], reason)
	}
	for name := range reasons {
		sort.Strings(reasons[name])
	}
	if len(reasons) == 0 {
		return nil
	}
	return reasons
}

func policyHashFromToolSurfacePolicy(snapshot *tooling.ToolSurfacePolicySnapshot) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(snapshot.Hash)
}

func uniqueSortedTraceStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func toolSurfaceSnapshotEmpty(snapshot *promptinput.ToolSurfaceSnapshot) bool {
	return snapshot == nil ||
		strings.TrimSpace(snapshot.Fingerprint) == "" &&
			len(snapshot.VisibleTools) == 0 &&
			len(snapshot.DeferredTools) == 0 &&
			len(snapshot.HiddenTools) == 0 &&
			len(snapshot.HiddenReasons) == 0 &&
			len(snapshot.LoadedPacksDelta) == 0 &&
			strings.TrimSpace(snapshot.PolicyHash) == ""
}

func safetySignalTracesFromPendingApprovals(approvals []PendingApproval) []promptinput.SafetySignalTrace {
	if len(approvals) == 0 {
		return nil
	}
	seen := map[string]promptinput.SafetySignalTrace{}
	for _, approval := range approvals {
		args, _ := json.Marshal(map[string]string{
			"command": approval.Command,
			"reason":  approval.Reason,
			"risk":    approval.Risk,
			"source":  approval.Source,
		})
		signals := policyengine.DetectSafetySignals(policyengine.PolicyInput{
			ToolName:  approval.ToolName,
			Tool:      tooling.ToolMetadata{Name: approval.ToolName, RiskLevel: tooling.ToolRiskLevel(approval.Risk)},
			Arguments: args,
		})
		for _, signal := range signals {
			key := strings.TrimSpace(string(signal.Category)) + "\x00" + strings.TrimSpace(string(signal.Severity))
			trace := seen[key]
			trace.Category = strings.TrimSpace(string(signal.Category))
			trace.Severity = strings.TrimSpace(string(signal.Severity))
			trace.Action = "require_approval"
			trace.Reasons = appendUniqueTraceStrings(trace.Reasons, signal.Reasons...)
			if strings.TrimSpace(approval.Reason) != "" {
				trace.Reasons = appendUniqueTraceStrings(trace.Reasons, strings.TrimSpace(approval.Reason))
			}
			seen[key] = trace
		}
	}
	if len(seen) == 0 {
		return nil
	}
	out := make([]promptinput.SafetySignalTrace, 0, len(seen))
	for _, trace := range seen {
		sort.Strings(trace.Reasons)
		out = append(out, trace)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Severity == out[j].Severity {
			return out[i].Category < out[j].Category
		}
		return safetyTraceSeverityRank(out[i].Severity) > safetyTraceSeverityRank(out[j].Severity)
	})
	return out
}

func unexpectedStateGateTraceFromSignals(signals []UnexpectedStateSignal) *promptinput.UnexpectedStateGateTrace {
	var unresolved []UnexpectedStateSignal
	for _, signal := range signals {
		if signal.Resolved || !unexpectedStateStatuses[normalizeUnexpectedStateStatus(signal.Status)] {
			continue
		}
		unresolved = append(unresolved, signal)
	}
	if len(unresolved) == 0 {
		return nil
	}
	trace := &promptinput.UnexpectedStateGateTrace{
		Action:        UnexpectedStateActionBlockMutation,
		BlockedAction: "mutation",
		Reasons:       []string{"unresolved_unexpected_state_blocks_mutation"},
	}
	for _, signal := range unresolved {
		trace.Sources = appendUniqueTraceStrings(trace.Sources, firstNonEmpty(signal.SourceTool, signal.ToolCallID))
		scope := firstNonEmpty(signal.ResourcePath, signal.ResourceID, signal.ResourceType)
		trace.AffectedScopes = appendUniqueTraceStrings(trace.AffectedScopes, scope)
		trace.Reasons = appendUniqueTraceStrings(trace.Reasons, normalizeUnexpectedStateStatus(signal.Status))
	}
	sort.Strings(trace.Sources)
	sort.Strings(trace.AffectedScopes)
	sort.Strings(trace.Reasons)
	return trace
}

func approvalScopeTraceFromSession(session *SessionState) *promptinput.ApprovalScopeTrace {
	if session == nil {
		return nil
	}
	if len(session.PlanApprovalScopes) > 0 {
		scope := session.PlanApprovalScopes[len(session.PlanApprovalScopes)-1]
		trace := &promptinput.ApprovalScopeTrace{
			GrantID:        strings.TrimSpace(scope.ApprovalID),
			Status:         "approved",
			AllowedActions: append([]string(nil), scope.AllowedActions...),
			RiskCeiling:    strings.TrimSpace(scope.RiskCeiling),
			InputHash:      strings.TrimSpace(scope.InputHash),
			Reasons:        []string{"approved_plan_scope"},
		}
		if scope.ExpiresAt != nil {
			trace.ExpiresAt = scope.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		for _, resourceScope := range scope.ResourceScopes {
			trace.ResourceScopes = appendUniqueTraceStrings(trace.ResourceScopes, formatPlanApprovalResourceScope(resourceScope))
		}
		sort.Strings(trace.AllowedActions)
		sort.Strings(trace.ResourceScopes)
		return trace
	}
	approvals := sessionPendingApprovals(session)
	if len(approvals) > 0 {
		approval := approvals[0]
		trace := &promptinput.ApprovalScopeTrace{
			GrantID:        strings.TrimSpace(approval.ID),
			Status:         "pending",
			AllowedActions: append([]string(nil), approval.AllowedActions...),
			ResourceScopes: append([]string(nil), approval.ResourceScopes...),
			RiskCeiling:    firstNonEmpty(approval.RiskCeiling, approval.Risk),
			InputHash:      strings.TrimSpace(approval.InputHash),
			Reasons:        appendUniqueTraceStrings(nil, approval.Reason, approval.Source),
		}
		if len(trace.AllowedActions) == 0 && strings.TrimSpace(approval.ToolName) != "" {
			trace.AllowedActions = []string{strings.TrimSpace(approval.ToolName)}
		}
		if approval.ExpiresAt != nil {
			trace.ExpiresAt = approval.ExpiresAt.UTC().Format("2006-01-02T15:04:05Z")
		}
		sort.Strings(trace.AllowedActions)
		sort.Strings(trace.ResourceScopes)
		return trace
	}
	switch session.PlanMode.State {
	case PlanModeStateActive, PlanModeStateApproved, PlanModeStatePendingExitApproval:
		return &promptinput.ApprovalScopeTrace{
			Status:  string(session.PlanMode.State),
			Reasons: []string{"plan_mode_requires_matching_approval_scope"},
		}
	default:
		return nil
	}
}

func formatPlanApprovalResourceScope(scope PlanApprovalResourceScope) string {
	parts := []string{}
	if strings.TrimSpace(scope.Type) != "" {
		parts = append(parts, "type="+strings.TrimSpace(scope.Type))
	}
	if strings.TrimSpace(scope.ID) != "" {
		parts = append(parts, "id="+strings.TrimSpace(scope.ID))
	}
	if strings.TrimSpace(scope.Path) != "" {
		parts = append(parts, "path="+strings.TrimSpace(scope.Path))
	}
	if strings.TrimSpace(scope.Pattern) != "" {
		parts = append(parts, "pattern="+strings.TrimSpace(scope.Pattern))
	}
	return strings.Join(parts, " ")
}

func appendUniqueTraceStrings(values []string, next ...string) []string {
	for _, value := range next {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		exists := false
		for _, existing := range values {
			if existing == value {
				exists = true
				break
			}
		}
		if !exists {
			values = append(values, value)
		}
	}
	return values
}

func safetyTraceSeverityRank(severity string) int {
	switch strings.ToLower(strings.TrimSpace(severity)) {
	case "critical":
		return 4
	case "high":
		return 3
	case "medium":
		return 2
	case "low":
		return 1
	default:
		return 0
	}
}

func toolSearchTraceEventsFromDiscovery(discovery ToolDiscoverySessionState) []promptinput.ToolSearchTraceEvent {
	if len(discovery.LastSearchResults) == 0 && discovery.LastSearchRequest == nil && discovery.LastSearchResponse == nil && len(discovery.LastRejectedSearchResults) == 0 {
		return nil
	}
	matches := make([]string, 0, len(discovery.LastSearchResults))
	for _, match := range discovery.LastSearchResults {
		name := strings.TrimSpace(match.Name)
		if name == "" {
			continue
		}
		if strings.TrimSpace(match.Pack) != "" {
			name += " pack=" + strings.TrimSpace(match.Pack)
		}
		matches = append(matches, name)
	}
	sort.Strings(matches)
	rejectedReasons := toolSearchRejectedReasonsFromDiscovery(discovery.LastRejectedSearchResults)
	query := ""
	ranker := ""
	if discovery.LastSearchRequest != nil {
		query = strings.TrimSpace(discovery.LastSearchRequest.Query)
		ranker = strings.TrimSpace(discovery.LastSearchRequest.Ranker)
	}
	matchCount := len(matches)
	rejectedCount := len(rejectedReasons)
	if discovery.LastSearchResponse != nil {
		ranker = firstNonEmptyString(discovery.LastSearchResponse.Ranker, ranker)
		if discovery.LastSearchResponse.MatchCount > 0 {
			matchCount = discovery.LastSearchResponse.MatchCount
		}
		if discovery.LastSearchResponse.RejectedCount > 0 {
			rejectedCount = discovery.LastSearchResponse.RejectedCount
		}
	}
	if query == "" && ranker == "" && matchCount == 0 && rejectedCount == 0 {
		return nil
	}
	return []promptinput.ToolSearchTraceEvent{{
		Mode:            "search",
		Query:           query,
		Ranker:          ranker,
		MatchCount:      matchCount,
		RejectedCount:   rejectedCount,
		Matches:         matches,
		RejectedReasons: rejectedReasons,
		Reason:          "last_tool_search_results",
	}}
}

func toolSearchRejectedReasonsFromDiscovery(rejected []tooling.RejectedToolCandidate) []promptinput.ToolSearchRejectedReason {
	if len(rejected) == 0 {
		return nil
	}
	out := make([]promptinput.ToolSearchRejectedReason, 0, len(rejected))
	for _, item := range rejected {
		reason := promptinput.ToolSearchRejectedReason{
			ToolName:       strings.TrimSpace(item.Name),
			Reason:         strings.TrimSpace(item.Reason),
			Status:         strings.TrimSpace(item.Status),
			Source:         strings.TrimSpace(item.Source),
			MCPServerID:    strings.TrimSpace(item.MCPServerID),
			HealthStatus:   strings.TrimSpace(item.HealthStatus),
			FilteredReason: strings.TrimSpace(item.FilteredReason),
		}
		if reason.ToolName == "" && reason.Reason == "" && reason.FilteredReason == "" {
			continue
		}
		out = append(out, reason)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].ToolName != out[j].ToolName {
			return out[i].ToolName < out[j].ToolName
		}
		return out[i].Reason < out[j].Reason
	})
	return out
}

func skillSearchTraceEventsFromActivation(activation SkillActivationSessionState) []promptinput.SkillSearchTraceEvent {
	if len(activation.LastSearchResults) == 0 {
		return nil
	}
	matches := make([]string, 0, len(activation.LastSearchResults))
	for _, match := range activation.LastSearchResults {
		if strings.TrimSpace(match.Name) != "" {
			matches = append(matches, strings.TrimSpace(match.Name))
		}
	}
	if len(matches) == 0 {
		return nil
	}
	sort.Strings(matches)
	return []promptinput.SkillSearchTraceEvent{{
		Mode:       "search",
		MatchCount: len(matches),
		Matches:    matches,
		Reason:     "last_skill_search_results",
	}}
}

func skillReadTraceEventsFromActivation(activation SkillActivationSessionState) []promptinput.SkillReadTraceEvent {
	if len(activation.LoadedSkills) == 0 {
		return nil
	}
	names := activation.EnabledSkills()
	out := make([]promptinput.SkillReadTraceEvent, 0, len(names))
	for _, name := range names {
		ref := activation.LoadedSkills[name]
		out = append(out, promptinput.SkillReadTraceEvent{
			Skill:  strings.TrimSpace(ref.Name),
			Source: strings.TrimSpace(ref.Source),
			Reason: strings.TrimSpace(ref.Reason),
			Range:  fmt.Sprintf("%d:%d", ref.Range.Offset, ref.Range.Limit),
			Hash:   strings.TrimSpace(ref.Hash),
		})
	}
	return out
}

func rejectedSkillActivationTraceEventsFromActivation(activation SkillActivationSessionState) []promptinput.RejectedSkillActivationTraceEvent {
	if len(activation.RejectedActivations) == 0 {
		return nil
	}
	out := make([]promptinput.RejectedSkillActivationTraceEvent, 0, len(activation.RejectedActivations))
	for _, rejected := range activation.RejectedActivations {
		out = append(out, promptinput.RejectedSkillActivationTraceEvent{
			SkillName:      strings.TrimSpace(rejected.SkillName),
			Reason:         strings.TrimSpace(rejected.Reason),
			RequiredAction: strings.TrimSpace(rejected.RequiredAction),
			TurnID:         strings.TrimSpace(rejected.TurnID),
		})
	}
	return out
}

func mcpInstructionDeltaTraceEvents(state mcp.MCPInstructionSessionState) []promptinput.MCPInstructionDeltaTrace {
	if len(state.LastDelta) == 0 {
		return nil
	}
	out := make([]promptinput.MCPInstructionDeltaTrace, 0, len(state.LastDelta))
	for _, delta := range state.LastDelta {
		out = append(out, promptinput.MCPInstructionDeltaTrace{
			ServerID: strings.TrimSpace(delta.ServerID),
			Action:   strings.TrimSpace(delta.Action),
			Hash:     strings.TrimSpace(delta.Hash),
			Chars:    delta.Chars,
			Summary:  strings.TrimSpace(delta.Summary),
		})
	}
	return out
}

func toolSelectionTraceEventsFromDiscovery(discovery ToolDiscoverySessionState) []promptinput.ToolSelectionTraceEvent {
	if discovery.LastSelection != nil {
		selection := discovery.LastSelection
		loadedTools := loadedToolRefNames(selection.LoadedTools)
		loadedPacks := loadedPackRefNames(selection.LoadedPacks)
		notLoaded := cloneSortedStrings(selection.NotLoaded)
		if len(loadedTools) == 0 && len(loadedPacks) == 0 && len(notLoaded) == 0 {
			return nil
		}
		return []promptinput.ToolSelectionTraceEvent{{
			Source:           "tool_search.select",
			Reason:           firstNonEmptyString(selection.Reason, "session_enabled_tool_surface"),
			LoadedTools:      loadedTools,
			LoadedPacks:      loadedPacks,
			NotLoaded:        notLoaded,
			NotLoadedReasons: cloneStringMap(selection.NotLoadedReasons),
		}}
	}
	tools := discovery.EnabledTools()
	packs := discovery.EnabledPacks()
	if len(tools) == 0 && len(packs) == 0 {
		return nil
	}
	return []promptinput.ToolSelectionTraceEvent{{
		Source:      "tool_search.select",
		Reason:      "session_enabled_tool_surface",
		LoadedTools: tools,
		LoadedPacks: packs,
	}}
}

func loadedToolRefNames(refs []LoadedToolRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if name := strings.TrimSpace(ref.Name); name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func loadedPackRefNames(refs []LoadedPackRef) []string {
	if len(refs) == 0 {
		return nil
	}
	out := make([]string, 0, len(refs))
	for _, ref := range refs {
		if name := strings.TrimSpace(ref.Name); name != "" {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func cloneStringMap(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}
	out := make(map[string]string, len(values))
	for key, value := range values {
		key = strings.TrimSpace(key)
		value = strings.TrimSpace(value)
		if key == "" || value == "" {
			continue
		}
		out[key] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func rejectedToolCallTraceEventsFromDiscovery(discovery ToolDiscoverySessionState) []promptinput.RejectedToolCallTraceEvent {
	if len(discovery.RejectedCalls) == 0 {
		return nil
	}
	out := make([]promptinput.RejectedToolCallTraceEvent, 0, len(discovery.RejectedCalls))
	for _, call := range discovery.RejectedCalls {
		out = append(out, promptinput.RejectedToolCallTraceEvent{
			ToolName:             strings.TrimSpace(call.ToolName),
			ErrorType:            strings.TrimSpace(call.ErrorType),
			Reason:               strings.TrimSpace(call.Reason),
			RequiredAction:       strings.TrimSpace(call.RequiredAction),
			SuggestedSearchQuery: strings.TrimSpace(call.SuggestedSearchQuery),
			TurnID:               strings.TrimSpace(call.TurnID),
			ToolCallID:           strings.TrimSpace(call.ToolCallID),
		})
	}
	return out
}

func parallelDispatchTraceGroupsFromSnapshot(snapshot *TurnSnapshot) []promptinput.ParallelDispatchTraceGroup {
	if snapshot == nil {
		return nil
	}
	var out []promptinput.ParallelDispatchTraceGroup
	for _, iter := range snapshot.Iterations {
		out = append(out, iter.ParallelDispatchGroups...)
	}
	return out
}

func resourceLockTracesFromSnapshot(snapshot *TurnSnapshot) []promptinput.ResourceLockTrace {
	if snapshot == nil {
		return nil
	}
	var out []promptinput.ResourceLockTrace
	for _, iter := range snapshot.Iterations {
		out = append(out, iter.ResourceLocks...)
	}
	return out
}

func failedToolSummariesFromSnapshot(snapshot *TurnSnapshot) []promptinput.FailedToolSummary {
	if snapshot == nil {
		return nil
	}
	var out []promptinput.FailedToolSummary
	seen := map[string]bool{}
	for _, iter := range snapshot.Iterations {
		for _, invocation := range iter.ToolInvocations {
			if invocation.Status != ToolInvocationFailed && invocation.Status != ToolInvocationBlocked && strings.TrimSpace(invocation.FailureKind) == "" {
				continue
			}
			key := invocation.ToolCallID + "\x00" + invocation.ToolName
			if seen[key] {
				continue
			}
			seen[key] = true
			attempts := len(invocation.Attempts)
			if attempts == 0 {
				attempts = 1
			}
			out = append(out, promptinput.FailedToolSummary{
				Tool:          firstNonBlankRuntimeString(invocation.ToolName, invocation.ToolCallID),
				FailureClass:  firstNonBlankRuntimeString(invocation.FailureKind, string(invocation.Status)),
				Attempts:      attempts,
				FinalStatus:   string(invocation.Status),
				SafeToRetry:   failedToolSafeToRetry(invocation.Mutating, invocation.FailureKind),
				ModelGuidance: failedToolModelGuidance(invocation.Mutating, invocation.FailureKind, string(invocation.Status)),
			})
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Tool == out[j].Tool {
			return out[i].FailureClass < out[j].FailureClass
		}
		return out[i].Tool < out[j].Tool
	})
	return out
}
