package appui

import (
	"encoding/json"
	"os"
	"strings"

	"aiops-v2/internal/envcontext"
	"aiops-v2/internal/hostops"
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

func selectActiveChatRuntimeRoute(legacyRoute ChatRuntimeRoute, intentRoute ChatRuntimeRoute, frame runtimecontract.IntentFrame) (ChatRuntimeRoute, string) {
	mode := intentFrameRoutingMode()
	if mode != intentFrameRoutingActive {
		return legacyRoute, mode
	}
	frame = runtimecontract.NormalizeIntentFrame(frame)
	if frame.Kind == runtimecontract.IntentKindUnknown || frame.Confidence == runtimecontract.ConfidenceLow {
		return legacyRoute, mode
	}
	return intentRoute, mode
}

func intentFrameRoutingMode() string {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("AIOPS_INTENT_FRAME_ROUTING"))) {
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

func applyIntentFrameRouteMetadata(req *runtimekernel.TurnRequest, legacyRoute ChatRuntimeRoute, intentRoute ChatRuntimeRoute, frame runtimecontract.IntentFrame) {
	if req == nil {
		return
	}
	if req.Metadata == nil {
		req.Metadata = map[string]string{}
	}
	frame = runtimecontract.NormalizeIntentFrame(frame)
	req.Metadata[runtimecontract.MetadataIntentKind] = string(frame.Kind)
	req.Metadata[runtimecontract.MetadataIntentConfidence] = firstNonEmptyString(frame.Confidence, runtimecontract.ConfidenceLow)
	req.Metadata["aiops.intent.routingMode"] = intentFrameRoutingMode()
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
	return firstRouteMentionHostID(mentions)
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
