package appui

import (
	"encoding/json"
	"strings"

	"aiops-v2/internal/envcontext"
	"aiops-v2/internal/hostops"
	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
)

type ChatRuntimeRouteMode string

const (
	ChatRouteAdvisory     ChatRuntimeRouteMode = "chat_advisory"
	ChatRouteEvidenceRCA  ChatRuntimeRouteMode = "evidence_rca"
	ChatRouteHostBoundOps ChatRuntimeRouteMode = "host_bound_ops"
	ChatRouteMultiHostOps ChatRuntimeRouteMode = "multi_host_ops"

	hostOpsManagerProfile        = "host_manager"
	hostOpsManagerRuntimeProfile = "manager_agent_full_runtime"

	intentFrameRoutingTraceOnly = "trace_only"
	intentFrameRoutingShadow    = "shadow"
	intentFrameRoutingActive    = "active"
)

type ChatRuntimeRoute struct {
	Mode                      ChatRuntimeRouteMode
	Reasons                   []string
	UserProhibitedHostExec    bool
	RequiresHostBinding       bool
	AllowsExecCommand         bool
	AllowsWebLearn            bool
	AllowsCorootRCA           bool
	Confidence                string
	TargetRefs                []envcontext.TargetRef
	EnvironmentCompact        string
	EnvironmentReadOnlyReason string
}

func BuildChatRuntimeRoute(input string, mentions []hostops.HostMention, evidence UserEvidenceExtraction) ChatRuntimeRoute {
	return BuildChatRuntimeRouteWithEnvironment(input, mentions, evidence, envcontext.ResolveEnvironmentFacts(envcontextResolverInput(input, mentions)))
}

func BuildChatRuntimeRouteWithEnvironment(input string, mentions []hostops.HostMention, evidence UserEvidenceExtraction, environment envcontext.ResolverOutput) ChatRuntimeRoute {
	route := ChatRuntimeRoute{
		Mode:               ChatRouteAdvisory,
		Reasons:            []string{"no host mentions"},
		AllowsCorootRCA:    hasExplicitCorootMention(input),
		Confidence:         "high",
		TargetRefs:         append([]envcontext.TargetRef(nil), environment.TargetRefs...),
		EnvironmentCompact: environment.CompactContext(),
	}
	intentFrame := BuildIntentFrame(input, evidenceEnvelopeFromUserEvidence(evidence), nil)
	route.AllowsWebLearn = intentFrameAllowsPublicWeb(intentFrame)
	if evidence.HasEvidence {
		route.Mode = ChatRouteEvidenceRCA
		route.Reasons = []string{"user evidence present"}
	}
	route.UserProhibitedHostExec = hasIntentFrameConstraint(intentFrame, "no_host_exec")
	validHostCount := countResolvedHostMentions(mentions)
	if validHostCount == 1 {
		route.Mode = ChatRouteHostBoundOps
		route.Reasons = []string{"single explicit host mention"}
		route.RequiresHostBinding = true
		route.AllowsExecCommand = true
	} else if validHostCount >= 2 {
		route.Mode = ChatRouteMultiHostOps
		route.Reasons = []string{"multiple explicit host mentions"}
		route.RequiresHostBinding = true
		route.AllowsExecCommand = false
	}
	if route.UserProhibitedHostExec {
		route.AllowsExecCommand = false
		if route.Mode == ChatRouteHostBoundOps || route.Mode == ChatRouteMultiHostOps {
			route.Mode = ChatRouteEvidenceRCA
			route.Reasons = appendUniqueEvidenceString(route.Reasons, "user prohibited host execution")
		}
	}
	if !environment.ExecutionAllowed {
		route.Mode = ChatRouteEvidenceRCA
		route.AllowsExecCommand = false
		route.RequiresHostBinding = false
		route.EnvironmentReadOnlyReason = environment.ReadOnlyReason
		route.Reasons = appendUniqueEvidenceString(route.Reasons, "environment target conflict")
	}
	return route
}

func BuildChatRuntimeRouteFromIntentFrame(frame runtimecontract.IntentFrame, existing ChatRuntimeRoute) ChatRuntimeRoute {
	frame = runtimecontract.NormalizeIntentFrame(frame)
	if frame.Kind == runtimecontract.IntentKindUnknown {
		return existing
	}
	route := existing
	route.Confidence = firstNonEmptyString(frame.Confidence, route.Confidence, "medium")
	route.Reasons = []string{"intent frame: " + string(frame.Kind)}
	route.AllowsWebLearn = intentFrameAllowsPublicWeb(frame)
	if runtimecontract.ContainsActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec) && !existing.RequiresHostBinding && !existing.AllowsExecCommand {
		route.AllowsExecCommand = false
	}
	if hasIntentFrameConstraint(frame, "no_host_exec") {
		route.UserProhibitedHostExec = true
		route.AllowsExecCommand = false
	}
	if frame.Evidence.HasUserProvidedEvidence && (route.Mode == "" || route.Mode == ChatRouteAdvisory) {
		route.Mode = ChatRouteEvidenceRCA
		route.Reasons = appendUniqueEvidenceString(route.Reasons, "intent frame evidence present")
	}
	if runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime) && route.Mode == ChatRouteAdvisory {
		route.RequiresHostBinding = existing.RequiresHostBinding
	}
	return route
}

func selectActiveChatRuntimeRoute(legacyRoute ChatRuntimeRoute, intentRoute ChatRuntimeRoute, frame runtimecontract.IntentFrame, mode string) (ChatRuntimeRoute, string) {
	mode = intentFrameRoutingMode(mode)
	if mode != intentFrameRoutingActive {
		return legacyRoute, mode
	}
	frame = runtimecontract.NormalizeIntentFrame(frame)
	if frame.Kind == runtimecontract.IntentKindUnknown || frame.Confidence == runtimecontract.ConfidenceLow {
		return legacyRoute, mode
	}
	return intentRoute, mode
}

func intentFrameRoutingMode(mode string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case intentFrameRoutingShadow:
		return intentFrameRoutingShadow
	case intentFrameRoutingActive:
		return intentFrameRoutingActive
	default:
		return intentFrameRoutingTraceOnly
	}
}

func intentFrameAllowsPublicWeb(frame runtimecontract.IntentFrame) bool {
	frame = runtimecontract.NormalizeIntentFrame(frame)
	return frame.Kind == runtimecontract.IntentKindResearch ||
		runtimecontract.ContainsDataScope(frame.DataScopes, runtimecontract.DataScopePublicWeb) ||
		runtimecontract.ContainsDataScope(frame.Evidence.DataScopes, runtimecontract.DataScopePublicWeb)
}

func envcontextResolverInput(input string, mentions []hostops.HostMention) envcontext.ResolverInput {
	facts := make([]envcontext.EnvironmentFact, 0, len(mentions))
	for _, mention := range mentions {
		value := strings.TrimSpace(mention.HostID)
		if value == "" {
			value = strings.TrimSpace(mention.Address)
		}
		if value == "" && mention.Source == hostops.HostMentionSourceLocalAlias {
			value = "server-local"
		}
		if value == "" {
			continue
		}
		facts = append(facts, envcontext.EnvironmentFact{
			Kind:       envcontext.FactKindHostIdentity,
			Subject:    firstNonEmptyString(strings.TrimSpace(mention.Raw), strings.TrimSpace(mention.DisplayName), value),
			Value:      value,
			Source:     envcontext.FactSourceUser,
			SourceRef:  strings.TrimSpace(mention.TokenID),
			Confidence: envcontext.FactConfidenceConfirmed,
		})
	}
	return envcontext.ResolverInput{Input: input, UserFacts: facts}
}

func countResolvedHostMentions(mentions []hostops.HostMention) int {
	seen := map[string]struct{}{}
	for _, mention := range mentions {
		key := strings.TrimSpace(mention.HostID)
		if key == "" && mention.Source == hostops.HostMentionSourceLocalAlias {
			key = "server-local"
		}
		if key == "" && mention.Resolved {
			key = strings.TrimSpace(mention.Raw)
		}
		if key == "" && mention.Source == hostops.HostMentionSourceIPLiteral && strings.TrimSpace(mention.Address) != "" {
			key = "addr:" + strings.TrimSpace(mention.Address)
		}
		if key == "" {
			continue
		}
		seen[strings.ToLower(key)] = struct{}{}
	}
	return len(seen)
}

func applyChatRuntimeRouteHostBinding(req *runtimekernel.TurnRequest, route ChatRuntimeRoute, mentions []hostops.HostMention) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	switch route.Mode {
	case ChatRouteHostBoundOps:
		hostID := firstRouteTargetHostID(route.TargetRefs, mentions)
		req.HostID = hostID
		req.SessionType = runtimekernel.SessionTypeHost
		req.Metadata["aiops.target.binding"] = "host"
		req.Metadata["aiops.target.hostId"] = hostID
		req.Metadata["aiops.target.summary"] = firstNonEmptyString(hostID, "已选择主机")
	case ChatRouteMultiHostOps:
		req.HostID = ""
		req.SessionType = runtimekernel.SessionTypeWorkspace
		req.Mode = runtimekernel.ModePlan
		req.Metadata["aiops.target.binding"] = "multi_host"
		req.Metadata["aiops.target.summary"] = "多主机"
	default:
		req.HostID = ""
		req.SessionType = runtimekernel.SessionTypeWorkspace
		req.Mode = runtimekernel.ModeChat
		req.Metadata["aiops.target.binding"] = "none"
		req.Metadata["aiops.target.summary"] = "未绑定主机"
	}
}

func applyExplicitSelectedHostContext(req *runtimekernel.TurnRequest, route ChatRuntimeRoute, selectedHostID string, requestedSessionType runtimekernel.SessionType, requestedMode runtimekernel.Mode) {
	if req == nil || (route.Mode != ChatRouteAdvisory && route.Mode != ChatRouteEvidenceRCA) {
		return
	}
	selectedHostID = strings.TrimSpace(selectedHostID)
	if selectedHostID == "" || selectedHostID == serverLocalHostID {
		return
	}
	req.HostID = selectedHostID
	if requestedSessionType.IsValid() {
		req.SessionType = requestedSessionType
	}
	if requestedMode.IsValid() {
		req.Mode = requestedMode
	}
	req.Metadata["aiops.target.selectedHostId"] = selectedHostID
	req.Metadata["aiops.target.selectedHostContext"] = "true"
	if strings.TrimSpace(req.Metadata["aiops.target.summary"]) == "" || req.Metadata["aiops.target.summary"] == "未绑定主机" {
		req.Metadata["aiops.target.summary"] = "已选择主机上下文: " + selectedHostID
	}
}

func applyChatRuntimeResourceProjection(req *runtimekernel.TurnRequest, mentions []hostops.HostMention) {
	if req == nil {
		return
	}
	bindings := make([]resourcebinding.ResourceBindingSnapshot, 0, len(mentions)+1)
	for _, mention := range mentions {
		binding := resourcebinding.HostBindingFromMention(hostops.ResourceBindingProjectionFromMention(mention))
		if binding.Ref.ID == "" {
			continue
		}
		bindings = append(bindings, binding)
	}
	if hostID := strings.TrimSpace(req.HostID); hostID != "" && !resourceBindingsContainHost(bindings, hostID) {
		source := resourcebinding.BindingSourceRouteMetadata
		verifiedBy := "appui.route_metadata"
		if req.Metadata != nil && req.Metadata["aiops.target.selectedHostContext"] == "true" {
			source = resourcebinding.BindingSourceSessionTarget
			verifiedBy = "appui.selected_host_context"
		} else if req.Metadata != nil && req.Metadata["aiops.sessionTarget.route.applied"] == "true" {
			source = resourcebinding.BindingSourceSessionTarget
			verifiedBy = "appui.session_target_route"
		}
		bindings = append(bindings, resourcebinding.BuildHostBinding(resourcebinding.HostBindingInput{
			HostID:      hostID,
			DisplayName: hostID,
			Source:      source,
			VerifiedBy:  verifiedBy,
			Verified:    true,
		}))
	}
	if len(bindings) == 0 {
		return
	}
	req.ResourceBindings = mergeResourceBindings(req.ResourceBindings, bindings)
}

func applySessionTargetRouteResourceProjection(req *runtimekernel.TurnRequest, decision sessionTargetRouteDecision) {
	if req == nil || !decision.Applied || len(decision.HostIDs) == 0 {
		return
	}
	bindings := make([]resourcebinding.ResourceBindingSnapshot, 0, len(decision.HostIDs))
	for _, hostID := range decision.HostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID == "" {
			continue
		}
		bindings = append(bindings, resourcebinding.BuildHostBinding(resourcebinding.HostBindingInput{
			HostID:      hostID,
			DisplayName: hostID,
			Source:      resourcebinding.BindingSourceSessionTarget,
			VerifiedBy:  "appui.session_target_route",
			Verified:    true,
		}))
	}
	req.ResourceBindings = mergeResourceBindings(req.ResourceBindings, bindings)
}

func applyChatRuntimeSessionTargetRoleTrace(req *runtimekernel.TurnRequest, session *runtimekernel.SessionState, input string, mentions []hostops.HostMention) {
	if req == nil {
		return
	}
	if userClearsSessionTarget(input) {
		req.SessionTargetSnapshot = resourcebinding.SessionTargetCleared(req.TurnID)
		req.ResourceRoleBindings = nil
		req.RoleBindingConflicts = nil
		return
	}
	mentionIDs := hostMentionTokenIDs(mentions)
	if len(mentionIDs) > 0 {
		target := resourcebinding.SessionTargetFromVerifiedBindings(req.ResourceBindings, req.TurnID, mentionIDs)
		req.SessionTargetSnapshot = target
	} else if session != nil && session.SessionTargetSnapshot != nil {
		next := session.SessionTargetSnapshot.NextTurn()
		if next != nil && !next.Expired() {
			req.SessionTargetSnapshot = next
		}
	}
	extraction := resourcebinding.ExtractRoleBindings(input, resourcebinding.RoleCandidatesFromBindings(req.ResourceBindings), req.TurnID)
	if len(extraction.Bindings) > 0 {
		req.ResourceRoleBindings = extraction.Bindings
	}
	if len(extraction.Conflicts) > 0 {
		req.RoleBindingConflicts = extraction.Conflicts
	}
}

func applySessionTargetRouteMetadata(req *runtimekernel.TurnRequest, decision sessionTargetRouteDecision) {
	if req == nil || !decision.Enabled {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["aiops.sessionTarget.route.enabled"] = "true"
	req.Metadata["aiops.sessionTarget.route.applied"] = boolMetadataString(decision.Applied)
	req.Metadata["aiops.sessionTarget.route.requiresClarification"] = boolMetadataString(decision.RequiresClarification)
	if strings.TrimSpace(decision.Reason) != "" {
		req.Metadata["aiops.sessionTarget.route.reason"] = strings.TrimSpace(decision.Reason)
	}
	if strings.TrimSpace(decision.TargetSetID) != "" {
		req.Metadata["aiops.sessionTarget.route.targetSetId"] = strings.TrimSpace(decision.TargetSetID)
	}
	if strings.TrimSpace(decision.SourceTurnID) != "" {
		req.Metadata["aiops.sessionTarget.route.sourceTurnId"] = strings.TrimSpace(decision.SourceTurnID)
	}
	if len(decision.HostIDs) > 0 {
		req.Metadata["aiops.sessionTarget.route.hostIds"] = strings.Join(decision.HostIDs, ",")
	}
}

func userClearsSessionTarget(input string) bool {
	input = strings.ToLower(strings.TrimSpace(input))
	for _, phrase := range []string{"清除主机上下文", "清空主机上下文", "不要用刚才", "不要用上次", "换一台"} {
		if strings.Contains(input, phrase) {
			return true
		}
	}
	return false
}

func hostMentionTokenIDs(mentions []hostops.HostMention) []string {
	var ids []string
	for _, mention := range mentions {
		if id := strings.TrimSpace(mention.TokenID); id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func resourceBindingsContainHost(bindings []resourcebinding.ResourceBindingSnapshot, hostID string) bool {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return false
	}
	for _, binding := range bindings {
		if binding.Ref.Type == resourcebinding.ResourceTypeHost && strings.TrimSpace(binding.Ref.ID) == hostID {
			return true
		}
	}
	return false
}

func mergeResourceBindings(base, extra []resourcebinding.ResourceBindingSnapshot) []resourcebinding.ResourceBindingSnapshot {
	if len(extra) == 0 {
		return base
	}
	seen := map[string]struct{}{}
	out := make([]resourcebinding.ResourceBindingSnapshot, 0, len(base)+len(extra))
	for _, binding := range append(append([]resourcebinding.ResourceBindingSnapshot(nil), base...), extra...) {
		key := strings.TrimSpace(binding.TraceHash)
		if key == "" {
			key = binding.Ref.IdentityHash() + "\x00" + binding.Source + "\x00" + binding.TrustLevel
		}
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, binding)
	}
	return out
}

func firstRouteMentionHostID(mentions []hostops.HostMention) string {
	for _, mention := range mentions {
		if hostID := strings.TrimSpace(mention.HostID); hostID != "" {
			return hostID
		}
		if mention.Source == hostops.HostMentionSourceLocalAlias {
			return "server-local"
		}
	}
	return ""
}

func applyChatRuntimeRouteMetadata(req *runtimekernel.TurnRequest, route ChatRuntimeRoute) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["aiops.route.mode"] = string(route.Mode)
	req.Metadata["aiops.route.confidence"] = firstNonEmptyString(route.Confidence, "medium")
	req.Metadata["aiops.route.allowsExecCommand"] = boolMetadataString(route.AllowsExecCommand)
	req.Metadata["aiops.route.allowsWebLearn"] = boolMetadataString(route.AllowsWebLearn)
	req.Metadata["aiops.route.allowsCorootRCA"] = boolMetadataString(route.AllowsCorootRCA)
	req.Metadata["aiops.route.requiresHostBinding"] = boolMetadataString(route.RequiresHostBinding)
	req.Metadata["aiops.route.userProhibitedHostExec"] = boolMetadataString(route.UserProhibitedHostExec)
	req.Metadata["aiops.coroot.explicitRCA"] = boolMetadataString(route.AllowsCorootRCA)
	req.Metadata["aiops.tool.corootRCAAllowed"] = boolMetadataString(route.AllowsCorootRCA)
	if route.AllowsCorootRCA {
		req.Metadata[metadataObservabilityProvider] = "coroot"
	} else {
		delete(req.Metadata, metadataObservabilityProvider)
	}
	if len(route.TargetRefs) > 0 {
		req.Metadata["aiops.target.refs"] = strings.Join(routeTargetRefIDs(route.TargetRefs), ",")
	}
	if strings.TrimSpace(route.EnvironmentReadOnlyReason) != "" {
		req.Metadata["aiops.env.readOnlyReason"] = strings.TrimSpace(route.EnvironmentReadOnlyReason)
	}
	if strings.TrimSpace(route.EnvironmentCompact) != "" {
		req.Metadata["aiops.env.compactContext"] = strings.TrimSpace(route.EnvironmentCompact)
	}
	if len(route.Reasons) > 0 {
		if data, err := json.Marshal(route.Reasons); err == nil {
			req.Metadata["aiops.route.reasons"] = string(data)
		}
	}
}

func applyIntentFrameRouteMetadata(req *runtimekernel.TurnRequest, legacyRoute ChatRuntimeRoute, intentRoute ChatRuntimeRoute, activeRoute ChatRuntimeRoute, frame runtimecontract.IntentFrame, routingMode string) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	frame = intentFrameForActiveRoute(frame, activeRoute)
	req.Metadata[runtimecontract.MetadataIntentKind] = string(frame.Kind)
	req.Metadata[runtimecontract.MetadataIntentConfidence] = firstNonEmptyString(frame.Confidence, runtimecontract.ConfidenceLow)
	req.Metadata["aiops.intent.routingMode"] = intentFrameRoutingMode(routingMode)
	req.Metadata[runtimecontract.MetadataIntentDataScopes] = strings.Join(intentDataScopeStrings(frame.DataScopes), ",")
	req.Metadata[runtimecontract.MetadataIntentRiskBudget] = strings.Join(intentRiskStrings(frame.RiskBudget), ",")
	req.Metadata[runtimecontract.MetadataEvidenceKinds] = strings.Join(frame.Evidence.EvidenceKinds, ",")
	req.Metadata[runtimecontract.MetadataWeakSignals] = strings.Join(intentWeakSignalNames(frame.Evidence.WeakSignals), ",")
	if data, err := json.Marshal(frame); err == nil {
		req.Metadata[runtimecontract.MetadataIntentFrame] = string(data)
	}
	if data, err := json.Marshal(chatRouteMetadataSnapshot(legacyRoute)); err == nil {
		req.Metadata[runtimecontract.MetadataLegacyRoute] = string(data)
	}
	if data, err := json.Marshal(chatRouteMetadataSnapshot(intentRoute)); err == nil {
		req.Metadata[runtimecontract.MetadataIntentRoute] = string(data)
	}
	if diff := chatRouteDiff(legacyRoute, intentRoute); len(diff) > 0 {
		if data, err := json.Marshal(diff); err == nil {
			req.Metadata[runtimecontract.MetadataRouteDiff] = string(data)
		}
	} else {
		req.Metadata[runtimecontract.MetadataRouteDiff] = "[]"
	}
}

func intentFrameForActiveRoute(frame runtimecontract.IntentFrame, route ChatRuntimeRoute) runtimecontract.IntentFrame {
	frame = runtimecontract.NormalizeIntentFrame(frame)
	if route.Mode != ChatRouteHostBoundOps || !route.AllowsExecCommand || route.UserProhibitedHostExec {
		return frame
	}
	frame.DataScopes = runtimecontract.AppendDataScope(frame.DataScopes, runtimecontract.DataScopeLocalRuntime)
	frame.RiskBudget = runtimecontract.AppendActionRisk(frame.RiskBudget, runtimecontract.ActionRiskHostExec)
	if frame.Kind == runtimecontract.IntentKindUnknown {
		frame.Kind = runtimecontract.IntentKindVerify
		frame.Confidence = runtimecontract.ConfidenceMedium
	}
	frame.Capabilities = appendCapabilityCandidate(frame.Capabilities, runtimecontract.CapabilityCandidate{
		Name:       "host_runtime_inspection",
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
		Risks:      []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
		Reasons:    []string{"selected host route allows read-only runtime inspection"},
	})
	return runtimecontract.NormalizeIntentFrame(frame)
}

type chatRouteSnapshot struct {
	Mode                   string `json:"mode"`
	AllowsExecCommand      bool   `json:"allowsExecCommand"`
	AllowsWebLearn         bool   `json:"allowsWebLearn"`
	AllowsCorootRCA        bool   `json:"allowsCorootRCA"`
	RequiresHostBinding    bool   `json:"requiresHostBinding"`
	UserProhibitedHostExec bool   `json:"userProhibitedHostExec"`
	Confidence             string `json:"confidence,omitempty"`
}

type chatRouteDiffEntry struct {
	Field  string `json:"field"`
	Legacy any    `json:"legacy"`
	Intent any    `json:"intent"`
}

func chatRouteMetadataSnapshot(route ChatRuntimeRoute) chatRouteSnapshot {
	return chatRouteSnapshot{
		Mode:                   string(route.Mode),
		AllowsExecCommand:      route.AllowsExecCommand,
		AllowsWebLearn:         route.AllowsWebLearn,
		AllowsCorootRCA:        route.AllowsCorootRCA,
		RequiresHostBinding:    route.RequiresHostBinding,
		UserProhibitedHostExec: route.UserProhibitedHostExec,
		Confidence:             route.Confidence,
	}
}

func chatRouteDiff(legacyRoute ChatRuntimeRoute, intentRoute ChatRuntimeRoute) []chatRouteDiffEntry {
	var diff []chatRouteDiffEntry
	if legacyRoute.Mode != intentRoute.Mode {
		diff = append(diff, chatRouteDiffEntry{Field: "mode", Legacy: string(legacyRoute.Mode), Intent: string(intentRoute.Mode)})
	}
	if legacyRoute.AllowsExecCommand != intentRoute.AllowsExecCommand {
		diff = append(diff, chatRouteDiffEntry{Field: "allowsExecCommand", Legacy: legacyRoute.AllowsExecCommand, Intent: intentRoute.AllowsExecCommand})
	}
	if legacyRoute.AllowsWebLearn != intentRoute.AllowsWebLearn {
		diff = append(diff, chatRouteDiffEntry{Field: "allowsWebLearn", Legacy: legacyRoute.AllowsWebLearn, Intent: intentRoute.AllowsWebLearn})
	}
	if legacyRoute.AllowsCorootRCA != intentRoute.AllowsCorootRCA {
		diff = append(diff, chatRouteDiffEntry{Field: "allowsCorootRCA", Legacy: legacyRoute.AllowsCorootRCA, Intent: intentRoute.AllowsCorootRCA})
	}
	if legacyRoute.RequiresHostBinding != intentRoute.RequiresHostBinding {
		diff = append(diff, chatRouteDiffEntry{Field: "requiresHostBinding", Legacy: legacyRoute.RequiresHostBinding, Intent: intentRoute.RequiresHostBinding})
	}
	if legacyRoute.UserProhibitedHostExec != intentRoute.UserProhibitedHostExec {
		diff = append(diff, chatRouteDiffEntry{Field: "userProhibitedHostExec", Legacy: legacyRoute.UserProhibitedHostExec, Intent: intentRoute.UserProhibitedHostExec})
	}
	return diff
}

func intentDataScopeStrings(values []runtimecontract.DataScope) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			out = append(out, string(value))
		}
	}
	return out
}

func intentRiskStrings(values []runtimecontract.ActionRisk) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			out = append(out, string(value))
		}
	}
	return out
}

func intentWeakSignalNames(values []runtimecontract.WeakSignal) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		if strings.TrimSpace(value.Name) != "" {
			out = append(out, strings.TrimSpace(value.Name))
		}
	}
	return out
}

func hasIntentFrameConstraint(frame runtimecontract.IntentFrame, name string) bool {
	for _, constraint := range frame.Constraints {
		if constraint.Name == name {
			return true
		}
	}
	return false
}

func applyHostOpsManagerRuntimeMetadata(metadata map[string]string) {
	if metadata == nil {
		return
	}
	metadata["profile"] = hostOpsManagerProfile
	metadata["agentProfile"] = hostOpsManagerProfile
	metadata["runtimeProfile"] = hostOpsManagerRuntimeProfile
	metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], hostops.ToolPackHostOps)
}

func firstRouteTargetHostID(targets []envcontext.TargetRef, mentions []hostops.HostMention) string {
	if hostID := firstRouteMentionHostID(mentions); hostID != "" {
		return hostID
	}
	for _, target := range targets {
		if target.Kind != envcontext.TargetKindHost {
			continue
		}
		if strings.TrimSpace(target.Address) != "" {
			return strings.TrimSpace(target.Address)
		}
		if strings.HasPrefix(strings.TrimSpace(target.ID), "host:") {
			return strings.TrimPrefix(strings.TrimSpace(target.ID), "host:")
		}
	}
	return ""
}

func routeTargetRefIDs(targets []envcontext.TargetRef) []string {
	ids := make([]string, 0, len(targets))
	for _, target := range targets {
		id := strings.TrimSpace(target.ID)
		if id != "" {
			ids = append(ids, id)
		}
	}
	return ids
}

func applyUserEvidenceMetadata(req *runtimekernel.TurnRequest, evidence UserEvidenceExtraction) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	req.Metadata["aiops.userEvidence.present"] = boolMetadataString(evidence.HasEvidence)
	if len(evidence.EvidenceKinds) > 0 {
		req.Metadata["aiops.userEvidence.kinds"] = strings.Join(evidence.EvidenceKinds, ",")
	}
	if len(evidence.Signals) > 0 {
		req.Metadata["aiops.userEvidence.signals"] = strings.Join(evidence.Signals, ",")
	}
	if strings.TrimSpace(evidence.RawExcerpt) != "" {
		req.Metadata["aiops.userEvidence.rawExcerpt"] = strings.TrimSpace(evidence.RawExcerpt)
	}
}
