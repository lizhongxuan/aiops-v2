package appui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"

	"aiops-v2/internal/store"
)

type CapabilitySnapshotInput struct {
	TenantID     string
	UserID       string
	Profile      store.AgentProfileRecord
	SkillCatalog []store.SkillCatalogEntry
	McpCatalog   []store.AgentMCPCatalogEntry
	Policy       AgentProfilePolicySettings
}

func BuildCapabilitySnapshot(input CapabilitySnapshotInput) CapabilitySnapshot {
	snapshot := CapabilitySnapshot{
		TenantID:  strings.TrimSpace(input.TenantID),
		UserID:    strings.TrimSpace(input.UserID),
		ProfileID: strings.TrimSpace(stringField(input.Profile, "id")),
	}
	skillCatalog := skillCatalogMap(input.SkillCatalog)
	mcpCatalog := mcpCatalogMap(input.McpCatalog)

	for _, raw := range asAnySlice(input.Profile["skills"]) {
		binding := asAnyMap(raw)
		id := strings.TrimSpace(profileString(binding["id"]))
		if id == "" {
			continue
		}
		item := buildSkillCapabilitySnapshotItem(id, binding, skillCatalog[id], input.Policy)
		snapshot.Items = append(snapshot.Items, item)
	}
	for _, raw := range asAnySlice(firstNonNil(input.Profile["mcps"], input.Profile["mcpServers"])) {
		binding := asAnyMap(raw)
		id := strings.TrimSpace(profileString(binding["id"]))
		if id == "" {
			continue
		}
		item := buildMcpCapabilitySnapshotItem(id, binding, mcpCatalog[id], input.Policy)
		snapshot.Items = append(snapshot.Items, item)
	}

	sort.Slice(snapshot.Items, func(i, j int) bool {
		if snapshot.Items[i].Kind != snapshot.Items[j].Kind {
			return snapshot.Items[i].Kind < snapshot.Items[j].Kind
		}
		return snapshot.Items[i].ID < snapshot.Items[j].ID
	})
	snapshot.Fingerprint = fingerprintCapabilitySnapshot(snapshot)
	return snapshot
}

func buildSkillCapabilitySnapshotItem(id string, binding map[string]any, catalog store.SkillCatalogEntry, policy AgentProfilePolicySettings) CapabilitySnapshotItem {
	source := firstNonEmpty(profileString(binding["source"]), catalog.Source, "profile")
	risk := firstNonEmpty(profileString(binding["risk"]), catalog.Risk, "low")
	mode := firstNonEmpty(profileString(binding["invocationMode"]), profileString(binding["activationMode"]), catalog.InvocationMode, catalog.DefaultActivationMode)
	if mode == "" {
		if strings.EqualFold(risk, "high") || strings.EqualFold(risk, "critical") {
			mode = "user_only"
		} else {
			mode = "explicit_only"
		}
	}
	item := CapabilitySnapshotItem{
		ID:             id,
		Kind:           "skill",
		Enabled:        profileBool(binding["enabled"]),
		Source:         source,
		SourceScope:    firstNonEmpty(profileString(binding["sourceScope"]), catalog.SourceScope, sourceScope(source)),
		InvocationMode: mode,
		Risk:           risk,
		RuntimeStatus:  firstNonEmpty(profileString(binding["runtimeStatus"]), "available"),
		AllowedTools:   firstNonEmptyStringSlice(stringSliceFromAny(binding["allowedTools"]), catalog.AllowedTools),
		DeniedTools:    firstNonEmptyStringSlice(stringSliceFromAny(binding["deniedTools"]), catalog.DeniedTools),
	}
	if item.Enabled {
		item.Reason = firstNonEmpty(profileString(binding["reason"]), "enabled by "+item.Source)
	} else {
		item.Reason = firstNonEmpty(profileString(binding["unavailableReason"]), profileString(binding["reason"]), "disabled by profile")
		item.Policy = firstNonEmpty(profileString(binding["policy"]), "profile_disabled")
	}
	if reason := strings.TrimSpace(policy.DisabledSkills[id]); reason != "" {
		item.Enabled = false
		item.Policy = "admin_deny"
		item.RuntimeStatus = "disabled"
		item.Reason = reason
	}
	if !profileBool(binding["available"]) && binding["available"] != nil && item.Policy != "admin_deny" {
		item.Enabled = false
		item.Policy = firstNonEmpty(item.Policy, "unavailable")
		item.RuntimeStatus = "unavailable"
		item.Reason = firstNonEmpty(profileString(binding["unavailableReason"]), item.Reason, "skill unavailable")
	}
	return item
}

func buildMcpCapabilitySnapshotItem(id string, binding map[string]any, catalog store.AgentMCPCatalogEntry, policy AgentProfilePolicySettings) CapabilitySnapshotItem {
	source := firstNonEmpty(profileString(binding["source"]), catalog.Source, "profile")
	approvalStatus := firstNonEmpty(profileString(binding["approvalStatus"]), profileString(binding["approval"]), catalog.ApprovalStatus)
	if approvalStatus == "" && catalog.RequiresExplicitUserApproval && !isBuiltinSource(source) {
		approvalStatus = "pending_approval"
	}
	runtimeStatus := firstNonEmpty(profileString(binding["runtimeStatus"]), profileString(binding["status"]), catalog.RuntimeStatus, "available")
	item := CapabilitySnapshotItem{
		ID:             id,
		Kind:           "mcp_server",
		Enabled:        profileBool(binding["enabled"]),
		Source:         source,
		SourceScope:    firstNonEmpty(profileString(binding["sourceScope"]), catalog.SourceScope, sourceScope(source)),
		Reason:         firstNonEmpty(profileString(binding["reason"]), "enabled by "+source),
		RuntimeStatus:  runtimeStatus,
		Risk:           firstNonEmpty(profileString(binding["risk"]), catalog.Risk, mcpRisk(firstNonEmpty(profileString(binding["permission"]), catalog.Permission))),
		ApprovalStatus: approvalStatus,
	}
	if !item.Enabled {
		item.Policy = firstNonEmpty(profileString(binding["policy"]), "profile_disabled")
		item.Reason = firstNonEmpty(profileString(binding["unavailableReason"]), profileString(binding["reason"]), "disabled by profile")
	}
	if approvalStatus == "pending_approval" {
		item.Enabled = false
		item.Policy = "pending_approval"
		item.RuntimeStatus = "pending_approval"
		item.Reason = "pending explicit approval"
	}
	if approvalStatus == "denied" {
		item.Enabled = false
		item.Policy = "admin_deny"
		item.RuntimeStatus = "disabled"
		item.Reason = "denied by approval policy"
	}
	if isUnavailableRuntimeStatus(runtimeStatus) && item.Policy != "pending_approval" && item.Policy != "admin_deny" {
		item.Enabled = false
		item.Policy = "runtime_unavailable"
		item.RuntimeStatus = "unavailable"
		item.Reason = firstNonEmpty(profileString(binding["unavailableReason"]), "MCP server runtime unavailable")
	}
	if reason := strings.TrimSpace(policy.DisabledMCPServers[id]); reason != "" {
		item.Enabled = false
		item.Policy = "admin_deny"
		item.RuntimeStatus = "disabled"
		item.Reason = reason
	}
	if !profileBool(binding["available"]) && binding["available"] != nil && item.Policy != "admin_deny" && item.Policy != "pending_approval" {
		item.Enabled = false
		item.Policy = firstNonEmpty(item.Policy, "unavailable")
		item.RuntimeStatus = "unavailable"
		item.Reason = firstNonEmpty(profileString(binding["unavailableReason"]), item.Reason, "MCP server unavailable")
	}
	return item
}

func fingerprintCapabilitySnapshot(snapshot CapabilitySnapshot) string {
	payload := CapabilitySnapshot{
		TenantID:  snapshot.TenantID,
		UserID:    snapshot.UserID,
		ProfileID: snapshot.ProfileID,
		Items:     snapshot.Items,
	}
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func enabledCapabilityIDs(snapshot CapabilitySnapshot, kind string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, item := range snapshot.Items {
		if item.Kind == kind && item.Enabled {
			out[item.ID] = struct{}{}
		}
	}
	return out
}

func enabledBindingsForSnapshot(raw any, enabledIDs map[string]struct{}) []map[string]any {
	items := asAnySlice(raw)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := asAnyMap(item)
		id := strings.TrimSpace(profileString(entry["id"]))
		if _, ok := enabledIDs[id]; !ok {
			continue
		}
		out = append(out, cloneAnyMap(entry))
	}
	return out
}

func sourceScope(source string) string {
	source = strings.TrimSpace(strings.ToLower(source))
	switch {
	case source == "":
		return "profile"
	case strings.HasPrefix(source, "plugin"):
		return "plugin"
	case strings.Contains(source, "builtin"), strings.Contains(source, "built-in"):
		return "builtin"
	case strings.Contains(source, "project"):
		return "project"
	case strings.Contains(source, "tenant"):
		return "tenant"
	case strings.Contains(source, "managed"):
		return "managed"
	case strings.Contains(source, "user"), strings.Contains(source, "profile"):
		return "user"
	default:
		return source
	}
}

func isBuiltinSource(source string) bool {
	scope := sourceScope(source)
	return scope == "builtin"
}

func mcpRisk(permission string) string {
	switch strings.TrimSpace(strings.ToLower(permission)) {
	case "readwrite", "write", "admin":
		return "high"
	case "readonly", "read":
		return "low"
	default:
		return "medium"
	}
}

func isUnavailableRuntimeStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case "disconnected", "unavailable", "offline", "error", "disabled":
		return true
	default:
		return false
	}
}

func stringSliceFromAny(raw any) []string {
	values := asAnySlice(raw)
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if text := strings.TrimSpace(profileString(value)); text != "" {
			out = append(out, text)
		}
	}
	return out
}

func firstNonEmptyStringSlice(values ...[]string) []string {
	for _, value := range values {
		if len(value) > 0 {
			out := make([]string, len(value))
			copy(out, value)
			return out
		}
	}
	return nil
}

func cloneAppUIStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, len(values))
	copy(out, values)
	return out
}
