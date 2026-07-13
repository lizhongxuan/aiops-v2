package runtimekernel

import (
	"strings"

	"aiops-v2/internal/resourcebinding"
)

const (
	metadataRoleBindingGuardEnabled = "aiops.roleBinding.guard.enabled"
)

func roleBindingGuardConfigFromSession(_ *SessionState, snapshot *TurnSnapshot) RoleBindingGuardConfig {
	config := RoleBindingGuardConfig{Enabled: true}
	if snapshot == nil || snapshot.TurnAssembly == nil || snapshot.TurnAssembly.Validate() != nil {
		config.RoleConflicts = []resourcebinding.RoleBindingConflict{{
			Reasons: []string{"frozen_turn_assembly_unavailable"},
		}}
		return normalizeRoleBindingGuardConfig(config)
	}

	facts := snapshot.TurnAssembly.AdmissionFacts
	targets := append([]resourcebinding.ResourceRef(nil), facts.TargetRefs...)
	if len(targets) == 0 && facts.SessionTarget.IdentityHash() != "" {
		targets = append(targets, facts.SessionTarget)
	}
	if len(targets) == 1 {
		target := resourcebinding.NormalizeRef(targets[0])
		if target.Type == resourcebinding.ResourceTypeHost {
			config.BoundHostID = target.ID
		}
	}
	config.RoleBindings = append([]resourcebinding.ResourceRoleBinding(nil), facts.RoleBindings...)
	config.RoleConflicts = append([]resourcebinding.RoleBindingConflict(nil), facts.RoleConflicts...)
	config.BoundRole, config.RoleBindingHash = frozenRoleBindingAuthority(config.BoundHostID, config.RoleBindings)
	return normalizeRoleBindingGuardConfig(config)
}

func frozenRoleBindingAuthority(hostID string, bindings []resourcebinding.ResourceRoleBinding) (string, string) {
	hostID = strings.TrimSpace(hostID)
	if hostID == "" {
		return "", ""
	}
	var matched []resourcebinding.ResourceRoleBinding
	for _, binding := range bindings {
		ref := resourcebinding.NormalizeRef(binding.ResourceRef)
		if ref.Type == resourcebinding.ResourceTypeHost && strings.EqualFold(ref.ID, hostID) {
			matched = append(matched, binding)
		}
	}
	if len(matched) != 1 {
		return "", ""
	}
	return resourcebinding.NormalizeRole(matched[0].Role), strings.TrimSpace(matched[0].TraceHash)
}
