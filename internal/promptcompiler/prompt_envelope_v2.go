package promptcompiler

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
)

type PromptLogicalLayer string

const (
	LayerAbsoluteSystemCore    PromptLogicalLayer = "L0_absolute_system_core"
	LayerRoleProfileCore       PromptLogicalLayer = "L1_role_profile_core"
	LayerStableRuntimeContract PromptLogicalLayer = "L2_stable_runtime_contract"
	LayerTurnStableFacts       PromptLogicalLayer = "L3_turn_stable_facts"
	LayerConversationHistory   PromptLogicalLayer = "L4_conversation_history"
	LayerStepDynamicContext    PromptLogicalLayer = "L5_step_dynamic_context"
	LayerCurrentUserInput      PromptLogicalLayer = "L6_current_user_input"

	DynamicContextTrustTrustedRuntime    = "trusted_runtime"
	DynamicContextTrustRetrievedEvidence = "retrieved_evidence"
	DynamicContextTrustUntrustedContext  = "untrusted_context"

	DynamicContextSourceRuntimeState = "dynamic.runtime_state"
	DynamicContextSourceToolSurface  = "dynamic.tool_surface"
	DynamicContextSourceLegacyExtra  = "dynamic.legacy_extra"
)

type DynamicContextBundle struct {
	StepID           string `json:"stepId"`
	SourceType       string `json:"sourceType"`
	SourceRef        string `json:"sourceRef"`
	RetrievedAt      string `json:"retrievedAt"`
	ContentHash      string `json:"contentHash"`
	TrustLevel       string `json:"trustLevel"`
	TokenCount       int    `json:"tokenCount"`
	TruncationReason string `json:"truncationReason,omitempty"`
	Content          string `json:"content"`
}

func (bundle DynamicContextBundle) Validate() error {
	required := []struct{ name, value string }{
		{"stepId", bundle.StepID}, {"sourceType", bundle.SourceType}, {"sourceRef", bundle.SourceRef}, {"retrievedAt", bundle.RetrievedAt},
		{"contentHash", bundle.ContentHash}, {"trustLevel", bundle.TrustLevel},
	}
	for _, field := range required {
		if strings.TrimSpace(field.value) == "" {
			return fmt.Errorf("dynamic context bundle requires %s", field.name)
		}
	}
	if bundle.TokenCount < 0 {
		return fmt.Errorf("dynamic context bundle tokenCount must be >= 0")
	}
	if strings.TrimSpace(bundle.Content) == "" {
		return fmt.Errorf("dynamic context bundle requires content")
	}
	if !validDynamicContextSourceType(bundle.SourceType) {
		return fmt.Errorf("dynamic context bundle has invalid sourceType")
	}
	if !validDynamicContextTrustLevel(bundle.TrustLevel) {
		return fmt.Errorf("dynamic context bundle has invalid trustLevel")
	}
	if bundle.ContentHash != dynamicContextContentHash(bundle.Content) {
		return fmt.Errorf("dynamic context bundle contentHash mismatch")
	}
	return nil
}

type PromptEnvelopeV2 struct {
	SchemaVersion  string                  `json:"schemaVersion"`
	Sections       []PromptCompiledSection `json:"sections"`
	DynamicContext []DynamicContextBundle  `json:"dynamicContext,omitempty"`
}

const PromptEnvelopeV2SchemaVersion = "aiops.prompt-envelope.v2"

func (envelope PromptEnvelopeV2) Validate() error {
	if envelope.SchemaVersion != PromptEnvelopeV2SchemaVersion {
		return fmt.Errorf("invalid prompt envelope v2 schemaVersion")
	}
	if len(envelope.Sections) < 2 {
		return fmt.Errorf("prompt envelope v2 requires L0 and L1")
	}
	if envelope.Sections[0].LogicalLayer != LayerAbsoluteSystemCore {
		return fmt.Errorf("prompt envelope v2 first section must be L0")
	}
	if envelope.Sections[1].LogicalLayer != LayerRoleProfileCore {
		return fmt.Errorf("prompt envelope v2 second section must be L1")
	}
	bundlesByRef := make(map[string]DynamicContextBundle, len(envelope.DynamicContext))
	for _, bundle := range envelope.DynamicContext {
		if err := bundle.Validate(); err != nil {
			return err
		}
		ref := strings.TrimSpace(bundle.SourceRef)
		if _, exists := bundlesByRef[ref]; exists {
			return fmt.Errorf("prompt envelope v2 dynamic bundle sourceRef duplicated")
		}
		bundlesByRef[ref] = bundle
	}
	seen := map[string]bool{}
	referencedBundles := map[string]int{}
	previousRank := -1
	for i, section := range envelope.Sections {
		id := strings.TrimSpace(section.ID)
		if id == "" || seen[id] {
			return fmt.Errorf("prompt envelope v2 section id missing or duplicated")
		}
		seen[id] = true
		rank, ok := promptLogicalLayerRank(section.LogicalLayer)
		if !ok {
			return fmt.Errorf("prompt envelope v2 section %q has invalid logical layer", id)
		}
		if rank < previousRank {
			return fmt.Errorf("prompt envelope v2 logical layers are out of order")
		}
		if section.LogicalLayer == LayerCurrentUserInput && i != len(envelope.Sections)-1 {
			return fmt.Errorf("prompt envelope v2 L6 must be last")
		}
		if section.LogicalLayer == LayerStepDynamicContext {
			ref := strings.TrimSpace(section.BundleRef)
			bundle, ok := bundlesByRef[ref]
			if ref == "" || !ok {
				return fmt.Errorf("prompt envelope v2 L5 section requires matching dynamic bundle")
			}
			if dynamicContextContentHash(section.Content) != bundle.ContentHash || strings.TrimSpace(section.Source) != bundle.SourceType {
				return fmt.Errorf("prompt envelope v2 L5 section does not match dynamic bundle")
			}
			referencedBundles[ref]++
		}
		previousRank = rank
	}
	for ref := range bundlesByRef {
		if referencedBundles[ref] != 1 {
			return fmt.Errorf("prompt envelope v2 dynamic bundle must bind exactly one L5 section")
		}
	}
	return nil
}

func BuildPromptEnvelopeV2(compiled CompiledPrompt, ctx CompileContext) PromptEnvelopeV2 {
	sections := []PromptCompiledSection{
		newPromptEnvelopeV2Section("absolute.system.core", LayerAbsoluteSystemCore, "system", absoluteSystemCoreContent(), "runtime"),
		newPromptEnvelopeV2Section("role.profile.core", LayerRoleProfileCore, "system", roleProfileCoreContent(compiled, ctx), "profile"),
		newPromptEnvelopeV2Section("stable.runtime.contract", LayerStableRuntimeContract, "system", buildBaseRuntimeContract(""), "runtime_contract"),
		newPromptEnvelopeV2Section("turn.stable.facts", LayerTurnStableFacts, "system", turnStableFactsContent(ctx), "turn"),
	}
	bundles := dynamicContextBundlesForEnvelopeV2(compiled, ctx)
	for i, bundle := range bundles {
		section := newPromptEnvelopeV2Section(
			fmt.Sprintf("step.dynamic.%02d", i), LayerStepDynamicContext, "system", bundle.Content, bundle.SourceType,
		)
		section.BundleRef = bundle.SourceRef
		sections = append(sections, section)
	}
	return PromptEnvelopeV2{SchemaVersion: PromptEnvelopeV2SchemaVersion, Sections: sections, DynamicContext: bundles}
}

func newPromptEnvelopeV2Section(id string, layer PromptLogicalLayer, role, content, source string) PromptCompiledSection {
	rank, _ := promptLogicalLayerRank(layer)
	stability := PromptSectionKindDynamic
	if rank <= 3 {
		stability = PromptSectionKindStable
	}
	return PromptCompiledSection{
		ID: id, Layer: string(layer), LogicalLayer: layer, Role: role, Content: strings.TrimSpace(content),
		Stability: stability, Source: source, Required: rank <= 3,
	}
}

func absoluteSystemCoreContent() string {
	return "# Absolute System Core\nYou are the AIOps runtime assistant. Follow system safety and runtime authority boundaries.\n" + defaultBehaviorBaseline()
}

func roleProfileCoreContent(compiled CompiledPrompt, ctx CompileContext) string {
	parts := []string{"# Role Profile Core", strings.TrimSpace(compiled.System.Role)}
	if profile := buildProfileFragment(resolvePromptEnvelopeProfile(ctx), ""); strings.TrimSpace(profile) != "" {
		parts = append(parts, profile)
	}
	return joinNonEmpty(parts...)
}

func turnStableFactsContent(ctx CompileContext) string {
	lines := []string{"# Turn Stable Facts"}
	values := []struct{ name, value string }{
		{"session_type", strings.TrimSpace(ctx.SessionType)}, {"mode", strings.TrimSpace(ctx.Mode)},
		{"profile", normalizePromptProfile(ctx.Profile)}, {"agent_kind", strings.TrimSpace(string(ctx.AgentKind))},
		{"host", strings.TrimSpace(ctx.HostContext)}, {"workspace", strings.TrimSpace(ctx.WorkspaceContext)},
		{"task_depth", strings.TrimSpace(string(ctx.TaskDepth.Level))},
	}
	for _, field := range values {
		if field.value != "" {
			lines = append(lines, "- "+field.name+": "+field.value)
		}
	}
	constraints := append([]string(nil), ctx.UserConstraints...)
	sort.Strings(constraints)
	if value := runtimeListState(constraints); value != "none" {
		lines = append(lines, "- user_constraints: "+value)
	}
	return strings.Join(lines, "\n")
}

func dynamicContextBundlesForEnvelopeV2(compiled CompiledPrompt, ctx CompileContext) []DynamicContextBundle {
	var bundles []DynamicContextBundle
	if content := strings.TrimSpace(compiled.Dynamic.Policy.Content); content != "" {
		bundles = append(bundles, newDynamicContextBundle(DynamicContextSourceRuntimeState, "runtime://state", DynamicContextTrustTrustedRuntime, content, ""))
	}
	if content := strings.TrimSpace(compiled.Stable.Tools.Content); content != "" {
		ref := "runtime://tool-surface/" + firstNonEmpty(strings.TrimSpace(ctx.VisibleToolFingerprint), dynamicContextContentHash(content))
		bundles = append(bundles, newDynamicContextBundle(DynamicContextSourceToolSurface, ref, DynamicContextTrustTrustedRuntime, content, ""))
	}
	for _, source := range compiled.Dynamic.Sources {
		if source.ID == DynamicContextSourceEvidence || source.ID == DynamicContextSourceMemory || source.ID == DynamicContextSourceHistoryCompacted {
			continue
		}
		content := strings.TrimSpace(source.Content)
		if content == "" {
			continue
		}
		ref := strings.TrimSpace(source.SourceRef)
		if ref == "" {
			ref = "prompt-source://" + strings.TrimSpace(source.ID) + "/" + dynamicContextContentHash(content)
		}
		truncation := ""
		if source.Overflowed {
			truncation = "token_budget_exceeded"
		}
		bundles = append(bundles, newDynamicContextBundle(source.ID, ref, dynamicContextTrustForSource(source.ID), content, truncation))
	}
	if content := evidenceDynamicContext(ctx.EvidenceReminders, nil); strings.TrimSpace(content) != "" {
		bundles = append(bundles, newDynamicContextBundle(
			DynamicContextSourceEvidence, "runtime://evidence-reminders/"+dynamicContextContentHash(content),
			DynamicContextTrustRetrievedEvidence, content, "",
		))
	}
	for index, section := range ctx.ExtraSections {
		content := renderPromptSection(section)
		if strings.TrimSpace(content) == "" {
			continue
		}
		if strings.TrimSpace(section.SourceType) == "" {
			bundles = append(bundles, newDynamicContextBundle(
				DynamicContextSourceLegacyExtra, fmt.Sprintf("legacy-extra://%d/%s", index, dynamicContextContentHash(content)),
				DynamicContextTrustUntrustedContext, content, "",
			))
			continue
		}
		bundles = append(bundles, DynamicContextBundle{
			StepID: "shadow:step_unbound", SourceType: strings.TrimSpace(section.SourceType), SourceRef: strings.TrimSpace(section.SourceRef),
			RetrievedAt: strings.TrimSpace(section.RetrievedAt), ContentHash: dynamicContextContentHash(content),
			TrustLevel: strings.TrimSpace(section.TrustLevel), TokenCount: estimateDynamicContextTokens(content), Content: strings.TrimSpace(content),
		})
	}
	return bundles
}

func newDynamicContextBundle(sourceType, sourceRef, trust, content, truncation string) DynamicContextBundle {
	return DynamicContextBundle{
		StepID: "shadow:step_unbound", SourceType: strings.TrimSpace(sourceType), SourceRef: strings.TrimSpace(sourceRef),
		RetrievedAt: "not_recorded", ContentHash: dynamicContextContentHash(content), TrustLevel: trust,
		TokenCount: estimateDynamicContextTokens(content), TruncationReason: truncation, Content: strings.TrimSpace(content),
	}
}

func dynamicContextTrustForSource(sourceType string) string {
	switch strings.TrimSpace(sourceType) {
	case DynamicContextSourceSkill, DynamicContextSourceHostTask, DynamicContextSourceProtocol:
		return DynamicContextTrustTrustedRuntime
	case DynamicContextSourceEvidence:
		return DynamicContextTrustRetrievedEvidence
	default:
		return DynamicContextTrustUntrustedContext
	}
}

func validDynamicContextSourceType(sourceType string) bool {
	switch strings.TrimSpace(sourceType) {
	case DynamicContextSourceRuntimeState, DynamicContextSourceToolSurface, DynamicContextSourceLegacyExtra,
		DynamicContextSourceEvidence, DynamicContextSourceSkill, DynamicContextSourceHostTask,
		DynamicContextSourceProtocol, DynamicContextSourceMemory, DynamicContextSourceHistoryCompacted:
		return true
	default:
		return false
	}
}

func validDynamicContextTrustLevel(trust string) bool {
	switch strings.TrimSpace(trust) {
	case DynamicContextTrustTrustedRuntime, DynamicContextTrustRetrievedEvidence, DynamicContextTrustUntrustedContext:
		return true
	default:
		return false
	}
}

func dynamicContextContentHash(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func promptLogicalLayerRank(layer PromptLogicalLayer) (int, bool) {
	layers := []PromptLogicalLayer{
		LayerAbsoluteSystemCore, LayerRoleProfileCore, LayerStableRuntimeContract, LayerTurnStableFacts,
		LayerConversationHistory, LayerStepDynamicContext, LayerCurrentUserInput,
	}
	for i, candidate := range layers {
		if layer == candidate {
			return i, true
		}
	}
	return 0, false
}
