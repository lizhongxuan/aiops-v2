package runtimecontract

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/resourcebinding"
)

type AdmissionInput struct {
	Intent            *IntentFrame
	UserConstraints   []string
	SessionTarget     resourcebinding.ResourceRef
	ResourceBindings  []resourcebinding.ResourceBindingSnapshot
	RoleBindings      []resourcebinding.ResourceRoleBinding
	AgentKind         string
	Profile           string
	PermissionProfile string
	SourceRefs        []string
	Metadata          map[string]string
}

type AdmissionFacts struct {
	Intent                IntentFrame                               `json:"intent"`
	UserConstraints       []string                                  `json:"userConstraints,omitempty"`
	SessionTarget         resourcebinding.ResourceRef               `json:"sessionTarget,omitempty"`
	ResourceBindings      []resourcebinding.ResourceBindingSnapshot `json:"resourceBindings,omitempty"`
	RoleBindings          []resourcebinding.ResourceRoleBinding     `json:"roleBindings,omitempty"`
	AgentKind             string                                    `json:"agentKind,omitempty"`
	Profile               string                                    `json:"profile,omitempty"`
	PermissionProfile     string                                    `json:"permissionProfile,omitempty"`
	SourceRefs            []string                                  `json:"sourceRefs,omitempty"`
	CompatibilityOnlyKeys []string                                  `json:"compatibilityOnlyKeys,omitempty"`
	Hash                  string                                    `json:"hash"`
}

type admissionMetadataFacts struct {
	Intent            *IntentFrame
	UserConstraints   []string
	AgentKind         string
	Profile           string
	PermissionProfile string
	SourceRefs        []string
	CompatibilityOnly []string
}

func BuildAdmissionFacts(input AdmissionInput) (AdmissionFacts, error) {
	compatibility, err := adaptAdmissionMetadata(input.Metadata)
	if err != nil {
		return AdmissionFacts{}, err
	}
	intent := compatibility.Intent
	if input.Intent != nil {
		copy := *input.Intent
		intent = &copy
	}
	if intent == nil {
		intent = &IntentFrame{}
	}
	facts := AdmissionFacts{
		Intent:                normalizeAdmissionIntent(*intent),
		UserConstraints:       uniqueSortedAdmissionStrings(append(append([]string(nil), compatibility.UserConstraints...), input.UserConstraints...)),
		SessionTarget:         resourcebinding.NormalizeRef(input.SessionTarget),
		ResourceBindings:      normalizeAdmissionResourceBindings(input.ResourceBindings),
		RoleBindings:          normalizeAdmissionRoleBindings(input.RoleBindings),
		AgentKind:             firstAdmissionValue(input.AgentKind, compatibility.AgentKind),
		Profile:               firstAdmissionValue(input.Profile, compatibility.Profile),
		PermissionProfile:     firstAdmissionValue(input.PermissionProfile, compatibility.PermissionProfile),
		SourceRefs:            uniqueSortedAdmissionStrings(append(append([]string(nil), input.SourceRefs...), compatibility.SourceRefs...)),
		CompatibilityOnlyKeys: append([]string(nil), compatibility.CompatibilityOnly...),
	}
	for _, constraint := range facts.Intent.Constraints {
		value := strings.TrimSpace(constraint.Name)
		if strings.TrimSpace(constraint.Value) != "" {
			value += "=" + strings.TrimSpace(constraint.Value)
		}
		facts.UserConstraints = uniqueSortedAdmissionStrings(append(facts.UserConstraints, value))
	}
	facts.Hash = resourcebinding.StableTraceHash("runtimecontract.admission-facts", map[string]any{
		"intent":            facts.Intent,
		"userConstraints":   facts.UserConstraints,
		"sessionTarget":     facts.SessionTarget,
		"resourceBindings":  facts.ResourceBindings,
		"roleBindings":      facts.RoleBindings,
		"agentKind":         facts.AgentKind,
		"profile":           facts.Profile,
		"permissionProfile": facts.PermissionProfile,
		"sourceRefs":        facts.SourceRefs,
	})
	if err := ValidateAdmissionFacts(facts); err != nil {
		return facts, err
	}
	return facts, nil
}

func ValidateAdmissionFacts(facts AdmissionFacts) error {
	if err := validateAdmissionIntent(facts.Intent); err != nil {
		return err
	}
	if admissionIntentMutates(facts.Intent) && !admissionHasVerifiedTarget(facts) {
		return fmt.Errorf("mutation admission requires a verified target")
	}
	if conflicts := resourcebinding.DetectRoleBindingConflicts(facts.RoleBindings); len(conflicts) > 0 {
		return fmt.Errorf("conflicting role/resource bindings")
	}
	known := map[string]struct{}{}
	if hash := facts.SessionTarget.IdentityHash(); hash != "" {
		known[hash] = struct{}{}
	}
	for _, binding := range facts.ResourceBindings {
		if hash := resourcebinding.NormalizeRef(binding.Ref).IdentityHash(); hash != "" && binding.Verified() {
			known[hash] = struct{}{}
		}
	}
	for _, binding := range facts.RoleBindings {
		hash := resourcebinding.NormalizeRef(binding.ResourceRef).IdentityHash()
		if hash == "" {
			return fmt.Errorf("role binding is missing a resource identity")
		}
		if _, ok := known[hash]; !ok {
			return fmt.Errorf("role binding references an unbound resource")
		}
	}
	return nil
}

func adaptAdmissionMetadata(metadata map[string]string) (admissionMetadataFacts, error) {
	out := admissionMetadataFacts{}
	for key, value := range metadata {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		use, registered := admissionMetadataKeyUse(key)
		if !registered || use == admissionMetadataCompatibility {
			out.CompatibilityOnly = append(out.CompatibilityOnly, key)
			continue
		}
		if strings.TrimSpace(value) != "" {
			out.SourceRefs = append(out.SourceRefs, "metadata:"+key)
		}
	}
	if raw := strings.TrimSpace(metadata[MetadataIntentFrame]); raw != "" {
		var frame IntentFrame
		if err := json.Unmarshal([]byte(raw), &frame); err != nil {
			return admissionMetadataFacts{}, fmt.Errorf("invalid %s: %w", MetadataIntentFrame, err)
		}
		out.Intent = &frame
	} else {
		frame := IntentFrame{
			Kind:       IntentKind(strings.TrimSpace(metadata[MetadataIntentKind])),
			DataScopes: admissionMetadataDataScopes(metadata[MetadataIntentDataScopes]),
			RiskBudget: admissionMetadataActionRisks(metadata[MetadataIntentRiskBudget]),
			Confidence: strings.TrimSpace(metadata[MetadataIntentConfidence]),
		}
		frame.Evidence.EvidenceKinds = splitAdmissionMetadataList(metadata[MetadataEvidenceKinds])
		for _, name := range splitAdmissionMetadataList(metadata[MetadataWeakSignals]) {
			frame.Evidence.WeakSignals = append(frame.Evidence.WeakSignals, WeakSignal{
				Name: name, Source: "metadata", Confidence: ConfidenceLow,
			})
		}
		if frame.Kind != "" || len(frame.DataScopes) > 0 || len(frame.RiskBudget) > 0 || frame.Confidence != "" || len(frame.Evidence.EvidenceKinds) > 0 || len(frame.Evidence.WeakSignals) > 0 {
			out.Intent = &frame
		}
	}
	out.UserConstraints = splitAdmissionMetadataList(metadata[MetadataUserConstraints])
	out.AgentKind = strings.TrimSpace(metadata[MetadataAgentKind])
	out.Profile = firstAdmissionValue(metadata[MetadataProfile], metadata[MetadataToolProfile], metadata[MetadataAgentProfile])
	out.PermissionProfile = strings.TrimSpace(metadata[MetadataPermissionProfile])
	out.CompatibilityOnly = uniqueSortedAdmissionStrings(out.CompatibilityOnly)
	out.SourceRefs = uniqueSortedAdmissionStrings(out.SourceRefs)
	return out, nil
}

func normalizeAdmissionIntent(frame IntentFrame) IntentFrame {
	frame.DataScopes = append([]DataScope(nil), frame.DataScopes...)
	frame.RiskBudget = append([]ActionRisk(nil), frame.RiskBudget...)
	frame.Constraints = append([]IntentConstraint(nil), frame.Constraints...)
	frame.Capabilities = append([]CapabilityCandidate(nil), frame.Capabilities...)
	for index := range frame.Capabilities {
		frame.Capabilities[index].DataScopes = append([]DataScope(nil), frame.Capabilities[index].DataScopes...)
		frame.Capabilities[index].Risks = append([]ActionRisk(nil), frame.Capabilities[index].Risks...)
		frame.Capabilities[index].Reasons = append([]string(nil), frame.Capabilities[index].Reasons...)
	}
	frame.Evidence.EvidenceKinds = append([]string(nil), frame.Evidence.EvidenceKinds...)
	frame.Evidence.DataScopes = append([]DataScope(nil), frame.Evidence.DataScopes...)
	frame.Evidence.WeakSignals = append([]WeakSignal(nil), frame.Evidence.WeakSignals...)
	frame = NormalizeIntentFrame(frame)
	frame.Confidence = strings.TrimSpace(frame.Confidence)
	sort.Slice(frame.DataScopes, func(i, j int) bool { return frame.DataScopes[i] < frame.DataScopes[j] })
	sort.Slice(frame.RiskBudget, func(i, j int) bool { return frame.RiskBudget[i] < frame.RiskBudget[j] })
	sort.Slice(frame.Evidence.DataScopes, func(i, j int) bool { return frame.Evidence.DataScopes[i] < frame.Evidence.DataScopes[j] })
	frame.Evidence.EvidenceKinds = uniqueSortedAdmissionStrings(frame.Evidence.EvidenceKinds)
	for index := range frame.Constraints {
		frame.Constraints[index].Name = strings.TrimSpace(frame.Constraints[index].Name)
		frame.Constraints[index].Value = strings.TrimSpace(frame.Constraints[index].Value)
		frame.Constraints[index].Confidence = strings.TrimSpace(frame.Constraints[index].Confidence)
		frame.Constraints[index].Source = strings.TrimSpace(frame.Constraints[index].Source)
	}
	sort.Slice(frame.Constraints, func(i, j int) bool {
		left := frame.Constraints[i].Name + "\x00" + resourcebinding.StableTraceHash("admission-constraint", frame.Constraints[i])
		right := frame.Constraints[j].Name + "\x00" + resourcebinding.StableTraceHash("admission-constraint", frame.Constraints[j])
		return left < right
	})
	for index := range frame.Capabilities {
		frame.Capabilities[index].Name = strings.TrimSpace(frame.Capabilities[index].Name)
		sort.Slice(frame.Capabilities[index].DataScopes, func(i, j int) bool {
			return frame.Capabilities[index].DataScopes[i] < frame.Capabilities[index].DataScopes[j]
		})
		sort.Slice(frame.Capabilities[index].Risks, func(i, j int) bool {
			return frame.Capabilities[index].Risks[i] < frame.Capabilities[index].Risks[j]
		})
		frame.Capabilities[index].Reasons = uniqueSortedAdmissionStrings(frame.Capabilities[index].Reasons)
	}
	sort.Slice(frame.Capabilities, func(i, j int) bool {
		left := frame.Capabilities[i].Name + "\x00" + resourcebinding.StableTraceHash("admission-capability", frame.Capabilities[i])
		right := frame.Capabilities[j].Name + "\x00" + resourcebinding.StableTraceHash("admission-capability", frame.Capabilities[j])
		return left < right
	})
	for index := range frame.Evidence.WeakSignals {
		frame.Evidence.WeakSignals[index].Name = strings.TrimSpace(frame.Evidence.WeakSignals[index].Name)
		frame.Evidence.WeakSignals[index].Source = strings.TrimSpace(frame.Evidence.WeakSignals[index].Source)
		frame.Evidence.WeakSignals[index].Confidence = strings.TrimSpace(frame.Evidence.WeakSignals[index].Confidence)
		frame.Evidence.WeakSignals[index].Summary = strings.TrimSpace(frame.Evidence.WeakSignals[index].Summary)
	}
	sort.Slice(frame.Evidence.WeakSignals, func(i, j int) bool {
		left := frame.Evidence.WeakSignals[i].Name + "\x00" + resourcebinding.StableTraceHash("admission-weak-signal", frame.Evidence.WeakSignals[i])
		right := frame.Evidence.WeakSignals[j].Name + "\x00" + resourcebinding.StableTraceHash("admission-weak-signal", frame.Evidence.WeakSignals[j])
		return left < right
	})
	return frame
}

func normalizeAdmissionResourceBindings(values []resourcebinding.ResourceBindingSnapshot) []resourcebinding.ResourceBindingSnapshot {
	out := make([]resourcebinding.ResourceBindingSnapshot, 0, len(values))
	for _, value := range values {
		out = append(out, resourcebinding.NewBindingSnapshot(value.Ref, resourcebinding.BindingOptions{
			Source: value.Source, VerifiedBy: value.VerifiedBy, TrustLevel: value.TrustLevel,
		}))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TraceHash < out[j].TraceHash })
	return out
}

func normalizeAdmissionRoleBindings(values []resourcebinding.ResourceRoleBinding) []resourcebinding.ResourceRoleBinding {
	out := make([]resourcebinding.ResourceRoleBinding, 0, len(values))
	for _, value := range values {
		out = append(out, resourcebinding.NewRoleBinding(resourcebinding.RoleBindingInput{
			BindingID: value.BindingID, ResourceRef: value.ResourceRef, Role: value.Role,
			RoleAlias: append([]string(nil), value.RoleAlias...), SourceTurnID: value.SourceTurnID,
			SourceSpan: value.SourceSpan, Confidence: value.Confidence, ConflictPolicy: value.ConflictPolicy,
		}))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].TraceHash < out[j].TraceHash })
	return out
}

func validateAdmissionIntent(frame IntentFrame) error {
	switch frame.Kind {
	case IntentKindUnknown, IntentKindDiagnose, IntentKindExplain, IntentKindPlan, IntentKindChange, IntentKindVerify, IntentKindResearch, IntentKindConfigure, IntentKindRunbookAuthoring:
	default:
		return fmt.Errorf("invalid admission intent kind %q", frame.Kind)
	}
	if frame.Confidence != ConfidenceLow && frame.Confidence != ConfidenceMedium && frame.Confidence != ConfidenceHigh {
		return fmt.Errorf("invalid admission confidence %q", frame.Confidence)
	}
	for _, risk := range frame.RiskBudget {
		if err := validateAdmissionActionRisk(risk); err != nil {
			return err
		}
	}
	for _, scope := range append(append([]DataScope(nil), frame.DataScopes...), frame.Evidence.DataScopes...) {
		if err := validateAdmissionDataScope(scope); err != nil {
			return err
		}
	}
	for _, capability := range frame.Capabilities {
		for _, scope := range capability.DataScopes {
			if err := validateAdmissionDataScope(scope); err != nil {
				return err
			}
		}
		for _, risk := range capability.Risks {
			if err := validateAdmissionActionRisk(risk); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateAdmissionActionRisk(risk ActionRisk) error {
	switch risk {
	case ActionRiskReadOnly, ActionRiskWrite, ActionRiskHostExec, ActionRiskNetwork, ActionRiskDestruct:
		return nil
	default:
		return fmt.Errorf("invalid admission action risk %q", risk)
	}
}

func validateAdmissionDataScope(scope DataScope) error {
	switch scope {
	case DataScopeLocalRuntime, DataScopeWorkspace, DataScopeOpsKnowledge, DataScopePublicWeb, DataScopeExternalMCP:
		return nil
	default:
		return fmt.Errorf("invalid admission data scope %q", scope)
	}
}

func admissionIntentMutates(frame IntentFrame) bool {
	if frame.Kind == IntentKindChange || frame.Kind == IntentKindConfigure {
		return true
	}
	return ContainsActionRisk(frame.RiskBudget, ActionRiskWrite) || ContainsActionRisk(frame.RiskBudget, ActionRiskHostExec) || ContainsActionRisk(frame.RiskBudget, ActionRiskDestruct)
}

func admissionHasVerifiedTarget(facts AdmissionFacts) bool {
	if facts.SessionTarget.IdentityHash() != "" {
		return true
	}
	for _, binding := range facts.ResourceBindings {
		if binding.Verified() {
			return true
		}
	}
	return false
}

func admissionMetadataDataScopes(raw string) []DataScope {
	values := splitAdmissionMetadataList(raw)
	out := make([]DataScope, 0, len(values))
	for _, value := range values {
		out = append(out, DataScope(value))
	}
	return out
}

func admissionMetadataActionRisks(raw string) []ActionRisk {
	values := splitAdmissionMetadataList(raw)
	out := make([]ActionRisk, 0, len(values))
	for _, value := range values {
		out = append(out, ActionRisk(value))
	}
	return out
}

func splitAdmissionMetadataList(raw string) []string {
	return uniqueSortedAdmissionStrings(strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	}))
}

func uniqueSortedAdmissionStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func firstAdmissionValue(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}
