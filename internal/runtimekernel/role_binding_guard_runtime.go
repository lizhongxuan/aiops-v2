package runtimekernel

import (
	"os"
	"strings"
)

const (
	metadataRoleBindingGuardEnabled = "aiops.roleBinding.guard.enabled"
	envRoleBindingGuardEnabled      = "AIOPS_ROLE_BINDING_GUARD"
)

func roleBindingGuardConfigFromSession(session *SessionState, snapshot *TurnSnapshot) RoleBindingGuardConfig {
	metadata := map[string]string{}
	if snapshot != nil && len(snapshot.Metadata) > 0 {
		metadata = snapshot.Metadata
	}
	if !roleBindingGuardEnabled(metadata) {
		return RoleBindingGuardConfig{}
	}
	config := RoleBindingGuardConfig{
		Enabled:         true,
		BoundHostID:     firstMetadataValue(metadata, "boundHostId", "bound_host_id", "hostId", "hostID"),
		BoundRole:       firstMetadataValue(metadata, "boundRole", "bound_role"),
		RoleBindingHash: firstMetadataValue(metadata, "roleBindingHash", "role_binding_hash"),
	}
	if config.BoundHostID == "" && session != nil {
		config.BoundHostID = strings.TrimSpace(session.HostID)
	}
	if session != nil {
		config.RoleBindings = append(config.RoleBindings, session.ResourceRoleBindings...)
		config.RoleConflicts = append(config.RoleConflicts, session.RoleBindingConflicts...)
	}
	return normalizeRoleBindingGuardConfig(config)
}

func roleBindingGuardEnabled(metadata map[string]string) bool {
	if metadataBool(metadata[metadataRoleBindingGuardEnabled]) {
		return true
	}
	return metadataBool(os.Getenv(envRoleBindingGuardEnabled))
}
