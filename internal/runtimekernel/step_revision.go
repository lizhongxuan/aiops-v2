package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/agentassembly"
	"aiops-v2/internal/mcp"
)

const (
	StepRevisionKindInitial               = "initial_step"
	StepRevisionKindLegacyPreviousUnknown = "legacy_previous_step_unknown"
	StepRevisionKindSkillLoaded           = "skill_loaded"
	StepRevisionKindToolSurfaceChanged    = "tool_surface_changed"
	StepRevisionKindMCPHealthChanged      = "mcp_health_changed"
	StepRevisionKindMCPInstructionChanged = "mcp_instruction_changed"
	StepRevisionKindApprovalResumed       = "approval_resumed"
	StepRevisionKindUserInputResumed      = "user_input_resumed"
	StepRevisionKindModelRetryResumed     = "model_retry_resumed"
	StepRevisionKindContextCompacted      = "context_compacted"
	StepRevisionUnknownPreviousHash       = "legacy:previous-step-unknown"
)

type StepRevision struct {
	Kind          string   `json:"kind"`
	SourceRefs    []string `json:"sourceRefs,omitempty"`
	PreviousHash  string   `json:"previousHash,omitempty"`
	NextHash      string   `json:"nextHash"`
	ChangedFields []string `json:"changedFields,omitempty"`
	Hash          string   `json:"hash"`
}

type StepTransition struct {
	PreviousHash string         `json:"previousHash,omitempty"`
	NextHash     string         `json:"nextHash"`
	Revisions    []StepRevision `json:"revisions,omitempty"`
	Hash         string         `json:"hash"`
}

type StepRevisionCause struct {
	Kind         string `json:"kind,omitempty"`
	ApprovalID   string `json:"approvalId,omitempty"`
	ToolCallID   string `json:"toolCallId,omitempty"`
	CheckpointID string `json:"checkpointId,omitempty"`
}

func (cause StepRevisionCause) Validate() error {
	cause = normalizeStepRevisionCause(cause)
	switch cause.Kind {
	case StepRevisionKindApprovalResumed:
		if cause.ApprovalID == "" || cause.ToolCallID == "" {
			return fmt.Errorf("approval resume cause requires approval and tool call ids")
		}
	case StepRevisionKindUserInputResumed, StepRevisionKindModelRetryResumed:
	case "":
		return fmt.Errorf("step revision cause kind is required")
	default:
		return fmt.Errorf("invalid step revision cause kind %q", cause.Kind)
	}
	return nil
}

type StepRevisionFacts struct {
	TurnAssemblyHash      string            `json:"turnAssemblyHash"`
	IntentHash            string            `json:"intentHash,omitempty"`
	SessionTargetHash     string            `json:"sessionTargetHash,omitempty"`
	ResourceBindingsHash  string            `json:"resourceBindingsHash,omitempty"`
	RoleBindingsHash      string            `json:"roleBindingsHash,omitempty"`
	PermissionProfileHash string            `json:"permissionProfileHash,omitempty"`
	LoadedSkillsHash      string            `json:"loadedSkillsHash,omitempty"`
	LoadedSkillRefs       []string          `json:"loadedSkillRefs,omitempty"`
	LoadedToolsHash       string            `json:"loadedToolsHash,omitempty"`
	LoadedToolRefs        []string          `json:"loadedToolRefs,omitempty"`
	LoadedPacksHash       string            `json:"loadedPacksHash,omitempty"`
	LoadedPackRefs        []string          `json:"loadedPackRefs,omitempty"`
	ToolRouterFingerprint string            `json:"toolRouterFingerprint,omitempty"`
	MCPInstructionHash    string            `json:"mcpInstructionHash,omitempty"`
	MCPHealthHash         string            `json:"mcpHealthHash,omitempty"`
	MCPServerRefs         []string          `json:"mcpServerRefs,omitempty"`
	CompactionHash        string            `json:"compactionHash,omitempty"`
	CompactedSegmentRefs  []string          `json:"compactedSegmentRefs,omitempty"`
	Cause                 StepRevisionCause `json:"cause,omitempty"`
	Hash                  string            `json:"hash"`
}

type StepReference struct {
	StepHash         string            `json:"stepHash"`
	Iteration        int               `json:"iteration"`
	TurnAssemblyHash string            `json:"turnAssemblyHash"`
	Facts            StepRevisionFacts `json:"facts"`
	Transition       StepTransition    `json:"transition"`
	Hash             string            `json:"hash"`
}

func FreezeStepRevisionFacts(input StepRevisionFacts) (StepRevisionFacts, error) {
	input.Hash = ""
	input.TurnAssemblyHash = strings.TrimSpace(input.TurnAssemblyHash)
	input.LoadedSkillRefs = uniqueSortedTraceStrings(input.LoadedSkillRefs)
	input.LoadedToolRefs = uniqueSortedTraceStrings(input.LoadedToolRefs)
	input.LoadedPackRefs = uniqueSortedTraceStrings(input.LoadedPackRefs)
	input.MCPServerRefs = uniqueSortedTraceStrings(input.MCPServerRefs)
	input.CompactedSegmentRefs = uniqueSortedTraceStrings(input.CompactedSegmentRefs)
	input.Cause = normalizeStepRevisionCause(input.Cause)
	if input.Cause.Kind != "" {
		if err := input.Cause.Validate(); err != nil {
			return StepRevisionFacts{}, err
		}
	}
	input.Hash = stepRevisionFactsHash(input)
	if err := input.Validate(); err != nil {
		return StepRevisionFacts{}, err
	}
	return input, nil
}

func (facts StepRevisionFacts) Validate() error {
	if strings.TrimSpace(facts.TurnAssemblyHash) == "" {
		return fmt.Errorf("step revision facts require turn assembly hash")
	}
	if strings.TrimSpace(facts.Hash) == "" || facts.Hash != stepRevisionFactsHash(facts) {
		return fmt.Errorf("step revision facts hash mismatch")
	}
	return nil
}

func stepRevisionFactsHash(facts StepRevisionFacts) string {
	facts.Hash = ""
	return agentassembly.StableHash("step-revision-facts", facts)
}

func BuildStepReference(previous *StepReference, step RuntimeStepContext, facts StepRevisionFacts) (StepReference, error) {
	if err := step.Validate(); err != nil {
		return StepReference{}, fmt.Errorf("step reference context: %w", err)
	}
	if err := facts.Validate(); err != nil {
		return StepReference{}, err
	}
	if facts.TurnAssemblyHash != step.TurnAssemblyHash {
		return StepReference{}, fmt.Errorf("step revision turn assembly hash mismatch")
	}
	previousHash := ""
	var revisions []StepRevision
	if previous == nil {
		kind := StepRevisionKindInitial
		if step.Iteration > 0 {
			kind = StepRevisionKindLegacyPreviousUnknown
			previousHash = StepRevisionUnknownPreviousHash
		}
		revisions = append(revisions, newStepRevision(kind, []string{facts.TurnAssemblyHash}, previousHash, step.Hash, nil))
	} else {
		if err := previous.Validate(); err != nil {
			return StepReference{}, fmt.Errorf("previous step reference: %w", err)
		}
		previousHash = previous.StepHash
		if err := validateImmutableStepRevisionFacts(previous.Facts, facts); err != nil {
			return StepReference{}, err
		}
		revisions = dynamicStepRevisions(previous.Facts, facts, previousHash, step.Hash)
	}
	transition := StepTransition{PreviousHash: previousHash, NextHash: step.Hash, Revisions: revisions}
	transition.Hash = stepTransitionHash(transition)
	ref := StepReference{
		StepHash: step.Hash, Iteration: step.Iteration, TurnAssemblyHash: step.TurnAssemblyHash,
		Facts: facts, Transition: transition,
	}
	ref.Hash = stepReferenceHash(ref)
	if err := ref.Validate(); err != nil {
		return StepReference{}, err
	}
	return ref, nil
}

func (ref StepReference) Validate() error {
	if strings.TrimSpace(ref.StepHash) == "" || ref.Iteration < 0 || ref.TurnAssemblyHash == "" {
		return fmt.Errorf("step reference identity is incomplete")
	}
	if err := ref.Facts.Validate(); err != nil {
		return err
	}
	if ref.Facts.TurnAssemblyHash != ref.TurnAssemblyHash || ref.Transition.NextHash != ref.StepHash {
		return fmt.Errorf("step reference facts or transition mismatch")
	}
	if ref.Transition.Hash != stepTransitionHash(ref.Transition) {
		return fmt.Errorf("step transition hash mismatch")
	}
	for _, revision := range ref.Transition.Revisions {
		if revision.NextHash != ref.StepHash || revision.PreviousHash != ref.Transition.PreviousHash || revision.Hash != stepRevisionHash(revision) {
			return fmt.Errorf("step revision hash chain mismatch")
		}
	}
	if ref.Hash != stepReferenceHash(ref) {
		return fmt.Errorf("step reference hash mismatch")
	}
	return nil
}

func BuildRuntimeStepRevisionFacts(assembly *agentassembly.TurnAssembly, step RuntimeStepContext, session *SessionState, snapshot *TurnSnapshot) (StepRevisionFacts, error) {
	if assembly == nil || session == nil || snapshot == nil {
		return StepRevisionFacts{}, fmt.Errorf("step revision requires assembly, session, and snapshot")
	}
	if err := assembly.Validate(); err != nil {
		return StepRevisionFacts{}, fmt.Errorf("step revision turn assembly: %w", err)
	}
	if assembly.Hash != step.TurnAssemblyHash {
		return StepRevisionFacts{}, fmt.Errorf("step revision turn assembly does not match step")
	}
	if agentassembly.StableHash("step-revision.admission", assembly.AdmissionFacts) != agentassembly.StableHash("step-revision.admission", step.Turn.AdmissionFacts) {
		return StepRevisionFacts{}, fmt.Errorf("step revision admission facts drifted from turn assembly")
	}
	loadedSkillsHash, loadedSkillRefs := currentSkillRevisionFacts(session.SkillActivation)
	loadedToolsHash, loadedToolRefs, loadedPacksHash, loadedPackRefs := currentToolRevisionFacts(session.ToolDiscovery)
	mcpInstructionHash, mcpInstructionRefs := currentMCPInstructionRevisionFacts(session.MCPInstructions)
	mcpHealthHash, mcpHealthRefs := currentMCPRevisionFacts()
	compactionHash, compactedRefs := currentCompactionRevisionFacts(snapshot.CompactedSegments, step.ContextState.CompactedSegments)
	facts := StepRevisionFacts{
		TurnAssemblyHash:      assembly.Hash,
		IntentHash:            agentassembly.StableHash("step-revision.intent", assembly.AdmissionFacts.Intent),
		SessionTargetHash:     agentassembly.StableHash("step-revision.session-target", assembly.AdmissionFacts.SessionTarget),
		ResourceBindingsHash:  agentassembly.StableHash("step-revision.resource-bindings", assembly.AdmissionFacts.ResourceBindings),
		RoleBindingsHash:      agentassembly.StableHash("step-revision.role-bindings", assembly.AdmissionFacts.RoleBindings),
		PermissionProfileHash: agentassembly.StableHash("step-revision.permission-profile", []string{assembly.PermissionProfile, assembly.AdmissionFacts.PermissionProfile}),
		LoadedSkillsHash:      loadedSkillsHash,
		LoadedSkillRefs:       loadedSkillRefs,
		LoadedToolsHash:       loadedToolsHash,
		LoadedToolRefs:        loadedToolRefs,
		LoadedPacksHash:       loadedPacksHash,
		LoadedPackRefs:        loadedPackRefs,
		ToolRouterFingerprint: step.ToolSurface.Fingerprint,
		MCPInstructionHash:    mcpInstructionHash,
		MCPHealthHash:         mcpHealthHash,
		MCPServerRefs:         append(mcpInstructionRefs, mcpHealthRefs...),
		CompactionHash:        compactionHash,
		CompactedSegmentRefs:  compactedRefs,
	}
	if snapshot.PendingStepCause != nil {
		facts.Cause = *snapshot.PendingStepCause
	}
	return FreezeStepRevisionFacts(facts)
}

func cloneStepReference(ref StepReference) StepReference {
	cloned := ref
	cloned.Facts.LoadedSkillRefs = append([]string(nil), ref.Facts.LoadedSkillRefs...)
	cloned.Facts.LoadedToolRefs = append([]string(nil), ref.Facts.LoadedToolRefs...)
	cloned.Facts.LoadedPackRefs = append([]string(nil), ref.Facts.LoadedPackRefs...)
	cloned.Facts.MCPServerRefs = append([]string(nil), ref.Facts.MCPServerRefs...)
	cloned.Facts.CompactedSegmentRefs = append([]string(nil), ref.Facts.CompactedSegmentRefs...)
	cloned.Transition.Revisions = append([]StepRevision(nil), ref.Transition.Revisions...)
	for i := range cloned.Transition.Revisions {
		cloned.Transition.Revisions[i].SourceRefs = append([]string(nil), ref.Transition.Revisions[i].SourceRefs...)
		cloned.Transition.Revisions[i].ChangedFields = append([]string(nil), ref.Transition.Revisions[i].ChangedFields...)
	}
	return cloned
}

func dynamicStepRevisions(previous, current StepRevisionFacts, previousHash, nextHash string) []StepRevision {
	var out []StepRevision
	if previous.LoadedSkillsHash != current.LoadedSkillsHash {
		out = append(out, newStepRevision(StepRevisionKindSkillLoaded, current.LoadedSkillRefs, previousHash, nextHash, []string{"loadedSkills"}))
	}
	if previous.LoadedToolsHash != current.LoadedToolsHash || previous.LoadedPacksHash != current.LoadedPacksHash || previous.ToolRouterFingerprint != current.ToolRouterFingerprint {
		refs := append(append([]string(nil), current.LoadedToolRefs...), current.LoadedPackRefs...)
		if current.ToolRouterFingerprint != "" {
			refs = append(refs, current.ToolRouterFingerprint)
		}
		out = append(out, newStepRevision(StepRevisionKindToolSurfaceChanged, refs, previousHash, nextHash, []string{"loadedTools", "loadedPacks", "toolRouterFingerprint"}))
	}
	if previous.MCPHealthHash != current.MCPHealthHash {
		out = append(out, newStepRevision(StepRevisionKindMCPHealthChanged, current.MCPServerRefs, previousHash, nextHash, []string{"mcpHealth"}))
	}
	if previous.MCPInstructionHash != current.MCPInstructionHash {
		out = append(out, newStepRevision(StepRevisionKindMCPInstructionChanged, current.MCPServerRefs, previousHash, nextHash, []string{"mcpInstructions"}))
	}
	if previous.CompactionHash != current.CompactionHash {
		out = append(out, newStepRevision(StepRevisionKindContextCompacted, current.CompactedSegmentRefs, previousHash, nextHash, []string{"compactedSegments"}))
	}
	if current.Cause.Kind != "" && current.Cause != previous.Cause {
		out = append(out, newStepRevision(current.Cause.Kind, stepRevisionCauseRefs(current.Cause), previousHash, nextHash, []string{"resumeCause"}))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Kind < out[j].Kind })
	return out
}

func validateImmutableStepRevisionFacts(previous, current StepRevisionFacts) error {
	fields := []struct{ name, before, after string }{
		{"turnAssembly", previous.TurnAssemblyHash, current.TurnAssemblyHash},
		{"intent", previous.IntentHash, current.IntentHash},
		{"sessionTarget", previous.SessionTargetHash, current.SessionTargetHash},
		{"resourceBindings", previous.ResourceBindingsHash, current.ResourceBindingsHash},
		{"roleBindings", previous.RoleBindingsHash, current.RoleBindingsHash},
		{"permissionProfile", previous.PermissionProfileHash, current.PermissionProfileHash},
	}
	for _, field := range fields {
		if field.before != field.after {
			return fmt.Errorf("step revision cannot modify immutable %s", field.name)
		}
	}
	return nil
}

func newStepRevision(kind string, refs []string, previousHash, nextHash string, changed []string) StepRevision {
	revision := StepRevision{
		Kind: strings.TrimSpace(kind), SourceRefs: uniqueSortedTraceStrings(refs), PreviousHash: strings.TrimSpace(previousHash),
		NextHash: strings.TrimSpace(nextHash), ChangedFields: uniqueSortedTraceStrings(changed),
	}
	revision.Hash = stepRevisionHash(revision)
	return revision
}

func stepRevisionHash(revision StepRevision) string {
	revision.Hash = ""
	return agentassembly.StableHash("step-revision", revision)
}

func stepTransitionHash(transition StepTransition) string {
	transition.Hash = ""
	return agentassembly.StableHash("step-transition", transition)
}

func stepReferenceHash(ref StepReference) string {
	ref.Hash = ""
	return agentassembly.StableHash("step-reference", ref)
}

func stepTransitionHasKind(transition StepTransition, kind string) bool {
	for _, revision := range transition.Revisions {
		if revision.Kind == kind {
			return true
		}
	}
	return false
}

func normalizeStepRevisionCause(cause StepRevisionCause) StepRevisionCause {
	cause.Kind = strings.TrimSpace(cause.Kind)
	cause.ApprovalID = strings.TrimSpace(cause.ApprovalID)
	cause.ToolCallID = strings.TrimSpace(cause.ToolCallID)
	cause.CheckpointID = strings.TrimSpace(cause.CheckpointID)
	return cause
}

func stepRevisionCauseRefs(cause StepRevisionCause) []string {
	return uniqueSortedTraceStrings([]string{cause.ApprovalID, cause.ToolCallID, cause.CheckpointID})
}

func currentSkillRevisionFacts(state SkillActivationSessionState) (string, []string) {
	type skillFact struct {
		Name, Source, Reason, Hash, RiskCeiling string
		Range                                   SkillReadRange
		AllowedTools, DeniedTools               []string
	}
	facts := make([]skillFact, 0, len(state.LoadedSkills))
	refs := make([]string, 0, len(state.LoadedSkills))
	for _, ref := range state.LoadedSkills {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		facts = append(facts, skillFact{
			Name: name, Source: strings.TrimSpace(ref.Source), Reason: strings.TrimSpace(ref.Reason), Range: ref.Range,
			Hash: strings.TrimSpace(ref.Hash), RiskCeiling: strings.TrimSpace(ref.RiskCeiling),
			AllowedTools: uniqueSortedTraceStrings(ref.AllowedTools), DeniedTools: uniqueSortedTraceStrings(ref.DeniedTools),
		})
		refs = append(refs, "skill:"+name)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].Name < facts[j].Name })
	return agentassembly.StableHash("step-revision.skills", facts), uniqueSortedTraceStrings(refs)
}

func currentToolRevisionFacts(state ToolDiscoverySessionState) (string, []string, string, []string) {
	type toolFact struct{ Name, Pack, Fingerprint, Source, Reason string }
	type packFact struct{ Name, Fingerprint, Source, Reason string }
	tools := make([]toolFact, 0, len(state.LoadedTools))
	toolRefs := make([]string, 0, len(state.LoadedTools))
	for _, ref := range state.LoadedTools {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		tools = append(tools, toolFact{Name: name, Pack: strings.TrimSpace(ref.Pack), Fingerprint: strings.TrimSpace(ref.Fingerprint), Source: strings.TrimSpace(ref.Source), Reason: strings.TrimSpace(ref.Reason)})
		toolRefs = append(toolRefs, "tool:"+name)
	}
	packs := make([]packFact, 0, len(state.LoadedPacks))
	packRefs := make([]string, 0, len(state.LoadedPacks))
	for _, ref := range state.LoadedPacks {
		name := strings.TrimSpace(ref.Name)
		if name == "" {
			continue
		}
		packs = append(packs, packFact{Name: name, Fingerprint: strings.TrimSpace(ref.Fingerprint), Source: strings.TrimSpace(ref.Source), Reason: strings.TrimSpace(ref.Reason)})
		packRefs = append(packRefs, "pack:"+name)
	}
	sort.Slice(tools, func(i, j int) bool { return tools[i].Name < tools[j].Name })
	sort.Slice(packs, func(i, j int) bool { return packs[i].Name < packs[j].Name })
	return agentassembly.StableHash("step-revision.tools", tools), uniqueSortedTraceStrings(toolRefs),
		agentassembly.StableHash("step-revision.packs", packs), uniqueSortedTraceStrings(packRefs)
}

func currentMCPInstructionRevisionFacts(state mcp.MCPInstructionSessionState) (string, []string) {
	type instructionFact struct{ ServerID, Hash, Summary string }
	facts := make([]instructionFact, 0, len(state.Announced))
	refs := make([]string, 0, len(state.Announced))
	for _, announced := range state.Announced {
		serverID := strings.TrimSpace(announced.ServerID)
		if serverID == "" {
			continue
		}
		facts = append(facts, instructionFact{ServerID: serverID, Hash: strings.TrimSpace(announced.Hash), Summary: strings.TrimSpace(announced.Summary)})
		refs = append(refs, "mcp:"+serverID)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ServerID < facts[j].ServerID })
	return agentassembly.StableHash("step-revision.mcp-instructions", facts), uniqueSortedTraceStrings(refs)
}

func currentCompactionRevisionFacts(previous, current []CompactedSegment) (string, []string) {
	type compactionFact struct {
		ID, SessionID, TurnID string
		Iteration, Start, End int
		ReferenceIDs          []string
	}
	byID := make(map[string]compactionFact, len(previous)+len(current))
	for _, segment := range append(append([]CompactedSegment(nil), previous...), current...) {
		id := strings.TrimSpace(segment.ID)
		if id == "" {
			continue
		}
		byID[id] = compactionFact{ID: id, SessionID: strings.TrimSpace(segment.SessionID), TurnID: strings.TrimSpace(segment.TurnID), Iteration: segment.Iteration, Start: segment.StartIndex, End: segment.EndIndex, ReferenceIDs: uniqueSortedTraceStrings(segment.ReferenceIDs)}
	}
	facts := make([]compactionFact, 0, len(byID))
	refs := make([]string, 0, len(byID))
	for id, fact := range byID {
		facts = append(facts, fact)
		refs = append(refs, "compaction:"+id)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ID < facts[j].ID })
	return agentassembly.StableHash("step-revision.compaction", facts), uniqueSortedTraceStrings(refs)
}

func currentMCPRevisionFacts() (string, []string) {
	registry := mcp.DefaultRegistry()
	if registry == nil {
		return agentassembly.StableHash("step-revision.mcp-health", nil), nil
	}
	snapshots := registry.ListServerHealthSnapshots()
	type healthFact struct{ ServerID, Status string }
	facts := make([]healthFact, 0, len(snapshots))
	refs := make([]string, 0, len(snapshots))
	for _, snapshot := range snapshots {
		serverID := strings.TrimSpace(snapshot.ServerID)
		if serverID == "" {
			continue
		}
		facts = append(facts, healthFact{ServerID: serverID, Status: strings.TrimSpace(string(snapshot.Status))})
		refs = append(refs, serverID)
	}
	sort.Slice(facts, func(i, j int) bool { return facts[i].ServerID < facts[j].ServerID })
	return agentassembly.StableHash("step-revision.mcp-health", facts), uniqueSortedTraceStrings(refs)
}
