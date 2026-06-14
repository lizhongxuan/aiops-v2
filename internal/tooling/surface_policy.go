package tooling

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type ToolHiddenReason struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type ToolVisibleReason struct {
	Name        string `json:"name"`
	Reason      string `json:"reason,omitempty"`
	SummaryOnly bool   `json:"summaryOnly,omitempty"`
}

type ToolSurfacePolicySnapshot struct {
	Mode              string              `json:"mode"`
	Profile           string              `json:"profile,omitempty"`
	ApprovalPolicy    string              `json:"approvalPolicy,omitempty"`
	ResourceScopeHash string              `json:"resourceScopeHash,omitempty"`
	PermissionHash    string              `json:"permissionHash,omitempty"`
	Hash              string              `json:"hash,omitempty"`
	HiddenTools       []ToolHiddenReason  `json:"hiddenTools,omitempty"`
	VisibleTools      []ToolVisibleReason `json:"visibleTools,omitempty"`
	SurfaceDecisions  []SurfaceDecision   `json:"surfaceDecisions,omitempty"`
}

type ToolSurfacePolicyOptions struct {
	Mode                string
	AgentRole           string
	Profile             string
	ApprovalPolicy      string
	ResourceScopeHash   string
	PermissionHash      string
	ActiveSkillPolicies []SkillToolPolicy
	RuntimeDecisions    []SurfaceRuntimeDecision
}

type SkillToolPolicy struct {
	SkillName    string
	AllowedTools []string
	DeniedTools  []string
	RiskCeiling  string
}

type SurfaceDispatchAction string

const (
	SurfaceDispatchAllow        SurfaceDispatchAction = "allow"
	SurfaceDispatchNeedApproval SurfaceDispatchAction = "need_approval"
	SurfaceDispatchNeedEvidence SurfaceDispatchAction = "need_evidence"
	SurfaceDispatchDeny         SurfaceDispatchAction = "deny"
)

func (a SurfaceDispatchAction) IsValid() bool {
	switch a {
	case SurfaceDispatchAllow, SurfaceDispatchNeedApproval, SurfaceDispatchNeedEvidence, SurfaceDispatchDeny:
		return true
	default:
		return false
	}
}

type SurfaceDecision struct {
	Name           string                `json:"name"`
	Visible        bool                  `json:"visible"`
	SummaryOnly    bool                  `json:"summaryOnly,omitempty"`
	DispatchAction SurfaceDispatchAction `json:"dispatchAction"`
	Reason         string                `json:"reason,omitempty"`
}

type SurfaceRuntimeDecision struct {
	Name           string                `json:"name"`
	DispatchAction SurfaceDispatchAction `json:"dispatchAction"`
	Reason         string                `json:"reason,omitempty"`
}

var planArtifactToolNames = map[string]struct{}{
	"update_plan":           {},
	"enter_plan_mode":       {},
	"exit_plan_mode":        {},
	"request_plan_approval": {},
	"claim_next_task":       {},
}

// IsPlanArtifactTool reports whether name is one of the exact tools allowed
// to write plan-mode artifacts. It intentionally does not use substring
// matching, so names such as draft_config_write or propose_restart stay denied.
func IsPlanArtifactTool(name string) bool {
	_, ok := planArtifactToolNames[normalizePlanArtifactToolName(name)]
	return ok
}

func normalizePlanArtifactToolName(name string) string {
	name = strings.TrimSpace(strings.ToLower(name))
	name = strings.ReplaceAll(name, ".", "_")
	name = strings.ReplaceAll(name, "-", "_")
	return name
}

func ApplyToolSurfacePolicy(tools []Tool, opts ToolSurfacePolicyOptions) ([]Tool, ToolSurfacePolicySnapshot) {
	snapshot := ToolSurfacePolicySnapshot{
		Mode:              strings.TrimSpace(opts.Mode),
		Profile:           strings.TrimSpace(opts.Profile),
		ApprovalPolicy:    strings.TrimSpace(opts.ApprovalPolicy),
		ResourceScopeHash: strings.TrimSpace(opts.ResourceScopeHash),
		PermissionHash:    strings.TrimSpace(opts.PermissionHash),
	}
	filtered := make([]Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if IsAlwaysModelCallableTool(meta) {
			const reason = "always_model_callable"
			snapshot.VisibleTools = append(snapshot.VisibleTools, ToolVisibleReason{Name: meta.Name, Reason: reason})
			snapshot.SurfaceDecisions = append(snapshot.SurfaceDecisions, SurfaceDecision{Name: meta.Name, Visible: true, DispatchAction: SurfaceDispatchAllow, Reason: reason})
			filtered = append(filtered, tool)
			continue
		}
		if runtimeDecision, ok := surfaceRuntimeDecisionForTool(meta, opts.RuntimeDecisions); ok && runtimeDecision.DispatchAction == SurfaceDispatchDeny {
			reason := surfaceDecisionReason(runtimeDecision.Reason, "runtime_denied")
			snapshot.HiddenTools = append(snapshot.HiddenTools, ToolHiddenReason{Name: meta.Name, Reason: reason})
			snapshot.SurfaceDecisions = append(snapshot.SurfaceDecisions, SurfaceDecision{Name: meta.Name, Visible: false, DispatchAction: SurfaceDispatchDeny, Reason: reason})
			continue
		}
		if reason := toolSurfaceHiddenReason(tool, meta, opts); reason != "" {
			snapshot.HiddenTools = append(snapshot.HiddenTools, ToolHiddenReason{Name: meta.Name, Reason: reason})
			snapshot.SurfaceDecisions = append(snapshot.SurfaceDecisions, SurfaceDecision{Name: meta.Name, Visible: false, DispatchAction: SurfaceDispatchDeny, Reason: reason})
			continue
		}
		if runtimeDecision, ok := surfaceRuntimeDecisionForTool(meta, opts.RuntimeDecisions); ok && runtimeDecision.DispatchAction != SurfaceDispatchAllow {
			reason := surfaceDecisionReason(runtimeDecision.Reason, string(runtimeDecision.DispatchAction))
			snapshot.VisibleTools = append(snapshot.VisibleTools, ToolVisibleReason{Name: meta.Name, Reason: reason, SummaryOnly: true})
			snapshot.SurfaceDecisions = append(snapshot.SurfaceDecisions, SurfaceDecision{Name: meta.Name, Visible: true, SummaryOnly: true, DispatchAction: runtimeDecision.DispatchAction, Reason: reason})
			continue
		}
		if reason := toolSurfaceSummaryOnlyReason(tool, meta); reason != "" {
			snapshot.VisibleTools = append(snapshot.VisibleTools, ToolVisibleReason{Name: meta.Name, Reason: reason, SummaryOnly: true})
			snapshot.SurfaceDecisions = append(snapshot.SurfaceDecisions, SurfaceDecision{Name: meta.Name, Visible: true, SummaryOnly: true, DispatchAction: SurfaceDispatchNeedApproval, Reason: reason})
			continue
		}
		reason := toolSurfaceVisibleReason(tool, meta)
		if runtimeDecision, ok := surfaceRuntimeDecisionForTool(meta, opts.RuntimeDecisions); ok && runtimeDecision.Reason != "" {
			reason = runtimeDecision.Reason
		}
		snapshot.VisibleTools = append(snapshot.VisibleTools, ToolVisibleReason{Name: meta.Name, Reason: reason})
		snapshot.SurfaceDecisions = append(snapshot.SurfaceDecisions, SurfaceDecision{Name: meta.Name, Visible: true, DispatchAction: SurfaceDispatchAllow, Reason: reason})
		filtered = append(filtered, tool)
	}
	sort.Slice(snapshot.HiddenTools, func(i, j int) bool {
		if snapshot.HiddenTools[i].Name != snapshot.HiddenTools[j].Name {
			return snapshot.HiddenTools[i].Name < snapshot.HiddenTools[j].Name
		}
		return snapshot.HiddenTools[i].Reason < snapshot.HiddenTools[j].Reason
	})
	sort.Slice(snapshot.VisibleTools, func(i, j int) bool {
		return snapshot.VisibleTools[i].Name < snapshot.VisibleTools[j].Name
	})
	sort.Slice(snapshot.SurfaceDecisions, func(i, j int) bool {
		if snapshot.SurfaceDecisions[i].Name != snapshot.SurfaceDecisions[j].Name {
			return snapshot.SurfaceDecisions[i].Name < snapshot.SurfaceDecisions[j].Name
		}
		return snapshot.SurfaceDecisions[i].Reason < snapshot.SurfaceDecisions[j].Reason
	})
	snapshot.Hash = toolSurfacePolicyHash(snapshot)
	return filtered, snapshot
}

func toolSurfaceHiddenReason(tool Tool, meta ToolMetadata, opts ToolSurfacePolicyOptions) string {
	if meta.Layer == ToolLayerInternal {
		return "internal_tool"
	}
	if ToolHiddenFromPrompt(meta) {
		return "hidden_from_prompt"
	}
	if !surfacePolicyProfileAllows(meta, opts.Profile) {
		return "profile_denied"
	}
	if reason := agentRoleSurfaceHiddenReason(meta, opts.AgentRole); reason != "" {
		return reason
	}
	if reason := skillSurfaceHiddenReason(meta, opts.ActiveSkillPolicies); reason != "" {
		return reason
	}
	governance := meta.EffectiveGovernance(4096)
	if governance.Mutating && modeDisallowsMutation(opts.Mode) && !modeAllowsPlanArtifactTool(opts.Mode, meta.Name) {
		return "mode_denied"
	}
	return ""
}

func agentRoleSurfaceHiddenReason(meta ToolMetadata, role string) string {
	governance := meta.EffectiveGovernance(4096)
	switch strings.TrimSpace(strings.ToLower(role)) {
	case "explore", "plan", "verify":
		if governance.Mutating && !(strings.TrimSpace(strings.ToLower(role)) == "plan" && IsPlanArtifactTool(meta.Name)) {
			return "agent_role_read_only"
		}
	case "execute", "host_child":
		if governance.Mutating && !governance.RequiresApproval {
			return "agent_role_mutation_requires_approval"
		}
	}
	return ""
}

func modeAllowsPlanArtifactTool(mode, name string) bool {
	return strings.TrimSpace(strings.ToLower(mode)) == "plan" && IsPlanArtifactTool(name)
}

func skillSurfaceHiddenReason(meta ToolMetadata, policies []SkillToolPolicy) string {
	if len(policies) == 0 {
		return ""
	}
	name := strings.TrimSpace(meta.Name)
	for _, policy := range policies {
		if stringListMatchesTool(policy.DeniedTools, meta, name) {
			return "skill_denied_tool"
		}
	}
	hasAllowedList := false
	for _, policy := range policies {
		allowed := normalizePolicyToolNames(policy.AllowedTools)
		if len(allowed) == 0 {
			continue
		}
		hasAllowedList = true
		if stringListMatchesTool(allowed, meta, name) {
			hasAllowedList = false
			break
		}
	}
	if hasAllowedList {
		return "skill_allowed_tools"
	}
	for _, policy := range policies {
		if riskAboveCeiling(meta.EffectiveGovernance(4096).RiskLevel, policy.RiskCeiling) {
			return "skill_risk_ceiling"
		}
	}
	return ""
}

func normalizePolicyToolNames(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" {
			out = append(out, value)
		}
	}
	return out
}

func stringListMatchesTool(values []string, meta ToolMetadata, name string) bool {
	for _, value := range values {
		if policyToolNameMatches(meta, value, name) {
			return true
		}
	}
	return false
}

func policyToolNameMatches(meta ToolMetadata, policyName, calledName string) bool {
	policyName = strings.TrimSpace(policyName)
	calledName = strings.TrimSpace(calledName)
	if policyName == "" || calledName == "" {
		return false
	}
	for _, candidate := range []string{meta.Name, ProviderSafeToolName(meta.Name)} {
		if strings.EqualFold(policyName, candidate) || strings.EqualFold(policyName, ProviderSafeToolName(candidate)) {
			return true
		}
	}
	for _, alias := range meta.Aliases {
		if strings.EqualFold(policyName, alias) || strings.EqualFold(policyName, ProviderSafeToolName(alias)) {
			return true
		}
	}
	return strings.EqualFold(policyName, calledName) || strings.EqualFold(ProviderSafeToolName(policyName), calledName)
}

func riskAboveCeiling(risk ToolRiskLevel, ceiling string) bool {
	ceiling = strings.TrimSpace(strings.ToLower(ceiling))
	if ceiling == "" {
		return false
	}
	return riskRank(risk.Normalize()) > riskRank(ToolRiskLevel(ceiling).Normalize())
}

func riskRank(risk ToolRiskLevel) int {
	switch risk.Normalize() {
	case ToolRiskLow:
		return 1
	case ToolRiskMedium:
		return 2
	case ToolRiskHigh:
		return 3
	case ToolRiskCritical:
		return 4
	default:
		return 2
	}
}

func toolSurfaceSummaryOnlyReason(tool Tool, meta ToolMetadata) string {
	governance := meta.EffectiveGovernance(4096)
	if governance.RequiresApproval || tool.IsDestructive(nil) {
		if toolSurfaceUsesArgumentScopedPermission(meta) {
			return ""
		}
		return "approval_required_schema_hidden"
	}
	return ""
}

func toolSurfaceVisibleReason(tool Tool, meta ToolMetadata) string {
	governance := meta.EffectiveGovernance(4096)
	if (governance.RequiresApproval || tool.IsDestructive(nil)) && toolSurfaceUsesArgumentScopedPermission(meta) {
		return "argument_scoped_permission"
	}
	return "policy_allowed"
}

func toolSurfaceUsesArgumentScopedPermission(meta ToolMetadata) bool {
	switch meta.EffectiveDiscovery().PermissionScope {
	case "argument_scoped", "argument", "per_call", "per_argument", "dynamic":
		return true
	default:
		return false
	}
}

func surfacePolicyProfileAllows(meta ToolMetadata, profile string) bool {
	if len(meta.Profiles) == 0 {
		return true
	}
	for _, candidate := range meta.Profiles {
		if strings.TrimSpace(candidate) == strings.TrimSpace(profile) {
			return true
		}
	}
	return false
}

func modeDisallowsMutation(mode string) bool {
	switch strings.TrimSpace(strings.ToLower(mode)) {
	case "execute":
		return false
	default:
		return true
	}
}

func ToolHiddenBySurfacePolicy(snapshot ToolSurfacePolicySnapshot, meta ToolMetadata, name string) (ToolHiddenReason, bool) {
	name = strings.TrimSpace(name)
	for _, hidden := range snapshot.HiddenTools {
		if surfacePolicyToolNameMatches(meta, hidden.Name, name) {
			return hidden, true
		}
	}
	return ToolHiddenReason{}, false
}

func SurfaceDecisionForTool(snapshot ToolSurfacePolicySnapshot, meta ToolMetadata, name string) (SurfaceDecision, bool) {
	name = strings.TrimSpace(name)
	for _, decision := range snapshot.SurfaceDecisions {
		if policyToolNameMatches(meta, decision.Name, name) {
			return decision, true
		}
	}
	return SurfaceDecision{}, false
}

func ValidateSurfaceDispatchConsistency(snapshot ToolSurfacePolicySnapshot) error {
	seen := map[string]struct{}{}
	for _, decision := range snapshot.SurfaceDecisions {
		name := strings.TrimSpace(decision.Name)
		if name == "" {
			return fmt.Errorf("surface decision missing tool name")
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate surface decision for %s", name)
		}
		seen[name] = struct{}{}
		if !decision.DispatchAction.IsValid() {
			return fmt.Errorf("surface decision for %s has invalid dispatch action %q", name, decision.DispatchAction)
		}
		if decision.Visible && decision.DispatchAction == SurfaceDispatchDeny {
			return fmt.Errorf("visible tool %s has denied dispatch action: %s", name, decision.Reason)
		}
	}
	return nil
}

func surfaceRuntimeDecisionForTool(meta ToolMetadata, decisions []SurfaceRuntimeDecision) (SurfaceRuntimeDecision, bool) {
	for _, decision := range decisions {
		action := decision.DispatchAction
		if action == "" {
			action = SurfaceDispatchAllow
		}
		if !action.IsValid() {
			continue
		}
		if policyToolNameMatches(meta, decision.Name, meta.Name) {
			decision.DispatchAction = action
			return decision, true
		}
	}
	return SurfaceRuntimeDecision{}, false
}

func surfaceDecisionReason(reason, fallback string) string {
	reason = strings.TrimSpace(reason)
	if reason != "" {
		return reason
	}
	return fallback
}

func surfacePolicyToolNameMatches(meta ToolMetadata, hiddenName, calledName string) bool {
	if calledName == "" {
		return false
	}
	for _, candidate := range []string{hiddenName, ProviderSafeToolName(hiddenName), meta.Name, ProviderSafeToolName(meta.Name)} {
		if strings.EqualFold(calledName, strings.TrimSpace(candidate)) {
			return true
		}
	}
	for _, alias := range meta.Aliases {
		if strings.EqualFold(calledName, alias) || strings.EqualFold(calledName, ProviderSafeToolName(alias)) {
			return true
		}
	}
	return false
}

func toolSurfacePolicyHash(snapshot ToolSurfacePolicySnapshot) string {
	snapshot.Hash = ""
	data, _ := json.Marshal(snapshot)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
