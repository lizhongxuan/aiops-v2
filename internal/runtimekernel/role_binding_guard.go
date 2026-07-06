package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/tooling"
)

type RoleBindingGuardConfig struct {
	Enabled         bool
	BoundHostID     string
	BoundRole       string
	RoleBindingHash string
	RoleBindings    []resourcebinding.ResourceRoleBinding
	RoleConflicts   []resourcebinding.RoleBindingConflict
}

func (d *ToolDispatcher) WithRoleBindingGuard(config RoleBindingGuardConfig) *ToolDispatcher {
	d.roleBindingGuard = normalizeRoleBindingGuardConfig(config)
	return d
}

func normalizeRoleBindingGuardConfig(config RoleBindingGuardConfig) RoleBindingGuardConfig {
	config.BoundHostID = strings.TrimSpace(config.BoundHostID)
	config.BoundRole = resourcebinding.NormalizeRole(config.BoundRole)
	config.RoleBindingHash = strings.TrimSpace(config.RoleBindingHash)
	config.RoleBindings = append([]resourcebinding.ResourceRoleBinding(nil), config.RoleBindings...)
	config.RoleConflicts = append([]resourcebinding.RoleBindingConflict(nil), config.RoleConflicts...)
	return config
}

func (d *ToolDispatcher) checkRoleBindingGuard(tc ToolCall, meta tooling.ToolMetadata) (string, bool) {
	if d == nil || !d.roleBindingGuard.Enabled {
		return "", false
	}
	config := d.roleBindingGuard
	args := roleBindingGuardArgs(tc.Arguments)
	if len(config.RoleConflicts) > 0 && roleBindingGuardToolIsMutating(meta) {
		return "role_binding_guard: role conflict blocks mutation", true
	}
	if config.BoundHostID != "" && args.hostID != "" && !strings.EqualFold(args.hostID, config.BoundHostID) {
		return fmt.Sprintf("role_binding_guard: requested host %s differs from bound host %s", args.hostID, config.BoundHostID), true
	}
	if config.RoleBindingHash != "" && args.roleBindingHash != "" && args.roleBindingHash != config.RoleBindingHash {
		return fmt.Sprintf("role_binding_guard: role binding hash %s differs from bound hash %s", args.roleBindingHash, config.RoleBindingHash), true
	}
	if args.targetRole == "" {
		return "", false
	}
	if config.BoundRole != "" && args.targetRole != config.BoundRole {
		return fmt.Sprintf("role_binding_guard: target role %s differs from bound role %s", args.targetRole, config.BoundRole), true
	}
	hostIDs := roleBindingGuardHostsForRole(config.RoleBindings, args.targetRole)
	if len(hostIDs) > 1 {
		return fmt.Sprintf("role_binding_guard: target role %s resolves to multiple hosts", args.targetRole), true
	}
	if len(hostIDs) == 1 {
		if config.BoundHostID != "" && !strings.EqualFold(hostIDs[0], config.BoundHostID) {
			return fmt.Sprintf("role_binding_guard: target role %s resolves to host %s, bound host is %s", args.targetRole, hostIDs[0], config.BoundHostID), true
		}
		if args.hostID != "" && !strings.EqualFold(args.hostID, hostIDs[0]) {
			return fmt.Sprintf("role_binding_guard: target role %s resolves to host %s, requested host is %s", args.targetRole, hostIDs[0], args.hostID), true
		}
	}
	return "", false
}

type roleBindingGuardToolArgs struct {
	hostID          string
	targetRole      string
	roleBindingHash string
}

func roleBindingGuardArgs(raw json.RawMessage) roleBindingGuardToolArgs {
	var payload map[string]any
	if len(raw) == 0 || json.Unmarshal(raw, &payload) != nil {
		return roleBindingGuardToolArgs{}
	}
	return roleBindingGuardToolArgs{
		hostID:          firstRoleBindingGuardString(payload, "hostId", "host_id", "targetHostId", "target_host_id"),
		targetRole:      resourcebinding.NormalizeRole(firstRoleBindingGuardString(payload, "targetRole", "target_role", "role")),
		roleBindingHash: firstRoleBindingGuardString(payload, "roleBindingHash", "role_binding_hash"),
	}
}

func firstRoleBindingGuardString(payload map[string]any, keys ...string) string {
	for _, key := range keys {
		value, ok := payload[key]
		if !ok || value == nil {
			continue
		}
		switch typed := value.(type) {
		case string:
			if strings.TrimSpace(typed) != "" {
				return strings.TrimSpace(typed)
			}
		case fmt.Stringer:
			if strings.TrimSpace(typed.String()) != "" {
				return strings.TrimSpace(typed.String())
			}
		}
	}
	return ""
}

func roleBindingGuardHostsForRole(bindings []resourcebinding.ResourceRoleBinding, role string) []string {
	role = resourcebinding.NormalizeRole(role)
	if role == "" {
		return nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, binding := range bindings {
		if resourcebinding.NormalizeRole(binding.Role) != role {
			continue
		}
		ref := resourcebinding.NormalizeRef(binding.ResourceRef)
		if ref.Type != resourcebinding.ResourceTypeHost || strings.TrimSpace(ref.ID) == "" {
			continue
		}
		key := strings.ToLower(ref.ID)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, ref.ID)
	}
	return out
}

func roleBindingGuardToolIsMutating(meta tooling.ToolMetadata) bool {
	return meta.Mutating || meta.Layer == tooling.ToolLayerMutation || meta.RequiresApproval
}
