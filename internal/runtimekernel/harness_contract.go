package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/resourcebinding"
)

const HarnessTurnSchemaVersion = "aiops.harness.turn.v1"

type HarnessTurnTrace struct {
	SchemaVersion string                  `json:"schemaVersion"`
	SessionID     string                  `json:"sessionId"`
	TurnID        string                  `json:"turnId"`
	Iteration     int                     `json:"iteration,omitempty"`
	SessionType   SessionType             `json:"sessionType,omitempty"`
	Mode          Mode                    `json:"mode,omitempty"`
	Lifecycle     TurnLifecycleState      `json:"lifecycle,omitempty"`
	ResumeState   TurnResumeState         `json:"resumeState,omitempty"`
	Route         HarnessRouteTrace       `json:"route,omitempty"`
	Target        HarnessTargetTrace      `json:"target,omitempty"`
	ToolSurface   HarnessToolSurfaceTrace `json:"toolSurface,omitempty"`
	Context       HarnessContextTrace     `json:"context,omitempty"`
	Final         HarnessFinalTrace       `json:"final,omitempty"`
}

type HarnessRouteTrace struct {
	Mode    string `json:"mode,omitempty"`
	HostID  string `json:"hostId,omitempty"`
	Profile string `json:"profile,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

type HarnessTargetTrace struct {
	Binding  string   `json:"binding,omitempty"`
	Refs     []string `json:"refs,omitempty"`
	Source   string   `json:"source,omitempty"`
	Verified bool     `json:"verified,omitempty"`
}

type HarnessToolSurfaceTrace struct {
	Fingerprint  string                   `json:"fingerprint,omitempty"`
	PolicyHash   string                   `json:"policyHash,omitempty"`
	Visible      []string                 `json:"visible,omitempty"`
	Dispatchable []string                 `json:"dispatchable,omitempty"`
	Hidden       []HarnessHiddenToolTrace `json:"hidden,omitempty"`
}

type HarnessHiddenToolTrace struct {
	Name    string   `json:"name,omitempty"`
	Reasons []string `json:"reasons,omitempty"`
}

type HarnessContextTrace struct {
	PromptHash string   `json:"promptHash,omitempty"`
	Sections   []string `json:"sections,omitempty"`
}

type HarnessFinalTrace struct {
	Status                string             `json:"status,omitempty"`
	Confidence            string             `json:"confidence,omitempty"`
	CheckedEvidenceRefs   []string           `json:"checkedEvidenceRefs,omitempty"`
	UncheckedRequirements []string           `json:"uncheckedRequirements,omitempty"`
	FailedToolImpacts     []FailedToolImpact `json:"failedToolImpacts,omitempty"`
	Limitations           []string           `json:"limitations,omitempty"`
}

func BuildHarnessTurnTrace(snapshot *TurnSnapshot, step RuntimeStepContext, final FinalEvidenceVerification) HarnessTurnTrace {
	trace := HarnessTurnTrace{
		SchemaVersion: HarnessTurnSchemaVersion,
		SessionID:     firstNonEmptyHarness(step.Turn.SessionID, snapshotString(snapshot, func(s *TurnSnapshot) string { return s.SessionID })),
		TurnID:        firstNonEmptyHarness(step.Turn.TurnID, snapshotString(snapshot, func(s *TurnSnapshot) string { return s.ID })),
		Iteration:     step.Iteration,
		SessionType:   firstSessionTypeHarness(step.Turn.SessionType, snapshotSessionType(snapshot)),
		Mode:          firstModeHarness(step.Turn.Mode, snapshotMode(snapshot)),
		Lifecycle:     snapshotLifecycle(snapshot),
		ResumeState:   snapshotResumeState(snapshot),
		Route:         buildHarnessRouteTrace(snapshot, step),
		Target:        buildHarnessTargetTrace(snapshot, step, final),
		ToolSurface:   buildHarnessToolSurfaceTrace(snapshot, step.ToolSurface),
		Context:       buildHarnessContextTrace(snapshot, step),
		Final:         buildHarnessFinalTrace(final),
	}
	if trace.Iteration == 0 && snapshot != nil {
		trace.Iteration = snapshot.Iteration
	}
	if trace.Lifecycle == "" {
		trace.Lifecycle = TurnLifecycleRunning
	}
	if trace.ResumeState == "" {
		trace.ResumeState = TurnResumeStateNone
	}
	return trace
}

func (t HarnessTurnTrace) Validate() error {
	if strings.TrimSpace(t.SchemaVersion) != HarnessTurnSchemaVersion {
		return fmt.Errorf("schemaVersion must be %q", HarnessTurnSchemaVersion)
	}
	if strings.TrimSpace(t.SessionID) == "" {
		return fmt.Errorf("sessionId is required")
	}
	if strings.TrimSpace(t.TurnID) == "" {
		return fmt.Errorf("turnId is required")
	}
	if t.Iteration < 0 {
		return fmt.Errorf("iteration must be non-negative")
	}
	return nil
}

func HarnessTargetTraceFromSessionTarget(target *resourcebinding.SessionTargetSnapshot, fallback HarnessTargetTrace) HarnessTargetTrace {
	if target == nil || len(target.HostIDs) == 0 {
		return fallback
	}
	out := fallback
	switch target.BindingMode {
	case resourcebinding.BindingModeSingleHost:
		out.Binding = "host"
	case resourcebinding.BindingModeMultiHost:
		out.Binding = "multi_host"
	default:
		out.Binding = "resource"
	}
	out.Source = "session_target"
	out.Refs = hostRefs(target.HostIDs)
	out.Verified = target.Confidence >= 0.8 && !target.RequiresConfirmation
	return out
}

func buildHarnessRouteTrace(snapshot *TurnSnapshot, step RuntimeStepContext) HarnessRouteTrace {
	metadata := mergedHarnessMetadata(snapshot, step)
	routeMode := firstNonEmptyHarness(step.Turn.Route.Route, metadata["aiops.route.mode"], metadata["runtimeRoute"], string(step.Turn.SessionType), snapshotString(snapshot, func(s *TurnSnapshot) string {
		return string(s.SessionType)
	}))
	return HarnessRouteTrace{
		Mode:    routeMode,
		HostID:  firstNonEmptyHarness(step.Turn.Route.HostID, step.Turn.HostID, metadata["aiops.target.hostId"]),
		Profile: firstNonEmptyHarness(step.Turn.Route.Profile, step.Turn.Profile, metadata["profile"], metadata["toolProfile"], metadata["agentProfile"]),
		Reason:  firstNonEmptyHarness(metadata["aiops.route.reason"], metadata["route.reason"]),
	}
}

func buildHarnessTargetTrace(snapshot *TurnSnapshot, step RuntimeStepContext, final FinalEvidenceVerification) HarnessTargetTrace {
	metadata := mergedHarnessMetadata(snapshot, step)
	hostID := firstNonEmptyHarness(step.Turn.HostID, step.Turn.Route.HostID, metadata["aiops.target.hostId"])
	refs := splitHarnessRefs(metadata["aiops.target.refs"])
	if hostID != "" {
		refs = append(refs, "host:"+hostID)
	}
	binding := strings.TrimSpace(metadata["aiops.target.binding"])
	if binding == "" && hostID != "" {
		binding = "host"
	}
	if binding == "" && len(refs) > 0 {
		binding = "resource"
	}
	return HarnessTargetTrace{
		Binding: strings.TrimSpace(binding),
		Refs:    uniqueSortedHarnessStrings(refs),
		Source:  firstNonEmptyHarness(metadata["aiops.target.source"], metadata["source"]),
		Verified: final.State.TargetBound ||
			strings.EqualFold(metadata["aiops.target.confidence"], "confirmed") ||
			strings.EqualFold(metadata["aiops.target.verified"], "true"),
	}
}

func buildHarnessToolSurfaceTrace(snapshot *TurnSnapshot, surface RuntimeToolRouterSnapshot) HarnessToolSurfaceTrace {
	if surface.Fingerprint == "" && snapshot != nil && snapshot.ToolSurfaceSnapshot != nil {
		surface = runtimeToolRouterSnapshotFromTurnSnapshot(snapshot)
	}
	return HarnessToolSurfaceTrace{
		Fingerprint:  strings.TrimSpace(surface.Fingerprint),
		PolicyHash:   strings.TrimSpace(surface.PolicyHash),
		Visible:      uniqueSortedHarnessStrings(surface.ModelVisibleTools),
		Dispatchable: uniqueSortedHarnessStrings(surface.DispatchableTools),
		Hidden:       hiddenToolTraceFromReasons(surface.HiddenReasons),
	}
}

func buildHarnessContextTrace(snapshot *TurnSnapshot, step RuntimeStepContext) HarnessContextTrace {
	promptHash := strings.TrimSpace(step.Compiled.Fingerprint.StableHash)
	if promptHash == "" && snapshot != nil {
		promptHash = strings.TrimSpace(snapshot.StablePromptHash)
	}
	return HarnessContextTrace{
		PromptHash: promptHash,
		Sections:   snapshotSections(snapshot),
	}
}

func buildHarnessFinalTrace(final FinalEvidenceVerification) HarnessFinalTrace {
	state := final.State
	return HarnessFinalTrace{
		Status:                harnessFinalStatus(final),
		Confidence:            firstNonEmptyHarness(final.Confidence, state.Confidence),
		CheckedEvidenceRefs:   checkedEvidenceRefs(state.Checked),
		UncheckedRequirements: uncheckedRequirementRefs(state.NotChecked),
		FailedToolImpacts:     append([]FailedToolImpact(nil), state.FailedTools...),
		Limitations:           uniqueSortedHarnessStrings(final.Reasons),
	}
}

func harnessFinalStatus(final FinalEvidenceVerification) string {
	state := final.State
	switch final.Action {
	case FinalEvidenceActionBlock:
		if len(state.NotChecked) > 0 || state.MutationIntentWithoutTarget {
			return "needs_evidence"
		}
		return "blocked"
	case FinalEvidenceActionDowngrade:
		return "partial"
	case FinalEvidenceActionAllow:
		if len(state.Checked) > 0 && len(state.NotChecked) == 0 && len(state.FailedTools) == 0 {
			return "verified"
		}
	}
	return "unknown"
}

func checkedEvidenceRefs(items []CheckedEvidence) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		ref := firstNonEmptyHarness(item.ToolCallID, item.ToolName, item.Summary)
		if ref != "" {
			out = append(out, ref)
		}
	}
	return uniqueSortedHarnessStrings(out)
}

func uncheckedRequirementRefs(items []NotCheckedItem) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		ref := firstNonEmptyHarness(item.ToolCallID, item.ToolName, item.RequiredAction, item.Reason)
		if ref != "" {
			out = append(out, ref)
		}
	}
	return uniqueSortedHarnessStrings(out)
}

func hiddenToolTraceFromReasons(reasons map[string][]string) []HarnessHiddenToolTrace {
	if len(reasons) == 0 {
		return nil
	}
	names := make([]string, 0, len(reasons))
	for name := range reasons {
		if strings.TrimSpace(name) != "" {
			names = append(names, strings.TrimSpace(name))
		}
	}
	sort.Strings(names)
	out := make([]HarnessHiddenToolTrace, 0, len(names))
	for _, name := range names {
		out = append(out, HarnessHiddenToolTrace{Name: name, Reasons: uniqueSortedHarnessStrings(reasons[name])})
	}
	return out
}

func mergedHarnessMetadata(snapshot *TurnSnapshot, step RuntimeStepContext) map[string]string {
	out := map[string]string{}
	if snapshot != nil {
		for key, value := range snapshot.Metadata {
			out[key] = value
		}
	}
	for key, value := range step.Turn.Metadata {
		out[key] = value
	}
	return out
}

func snapshotSections(snapshot *TurnSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	return uniqueSortedHarnessStrings(snapshot.PromptSections)
}

func snapshotString(snapshot *TurnSnapshot, fn func(*TurnSnapshot) string) string {
	if snapshot == nil {
		return ""
	}
	return strings.TrimSpace(fn(snapshot))
}

func snapshotSessionType(snapshot *TurnSnapshot) SessionType {
	if snapshot == nil {
		return ""
	}
	return snapshot.SessionType
}

func snapshotMode(snapshot *TurnSnapshot) Mode {
	if snapshot == nil {
		return ""
	}
	return snapshot.Mode
}

func snapshotLifecycle(snapshot *TurnSnapshot) TurnLifecycleState {
	if snapshot == nil {
		return ""
	}
	return snapshot.Lifecycle
}

func snapshotResumeState(snapshot *TurnSnapshot) TurnResumeState {
	if snapshot == nil {
		return ""
	}
	return snapshot.ResumeState
}

func firstSessionTypeHarness(values ...SessionType) SessionType {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return ""
}

func firstModeHarness(values ...Mode) Mode {
	for _, value := range values {
		if strings.TrimSpace(string(value)) != "" {
			return value
		}
	}
	return ""
}

func splitHarnessRefs(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return r == ',' || r == ';' || r == ' ' || r == '\n' || r == '\t'
	})
	return uniqueSortedHarnessStrings(fields)
}

func hostRefs(hostIDs []string) []string {
	out := make([]string, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		hostID = strings.TrimSpace(hostID)
		if hostID != "" {
			out = append(out, "host:"+hostID)
		}
	}
	return uniqueSortedHarnessStrings(out)
}

func firstNonEmptyHarness(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func uniqueSortedHarnessStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
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
	sort.Strings(out)
	return out
}
