package appui

import (
	"sort"
	"strings"

	"aiops-v2/internal/plugins"
)

func capabilityRecordFromSkillCatalogItem(item SkillCatalogItem) CapabilityRecord {
	return CapabilityRecord{
		ID:             item.ID,
		Kind:           "capability",
		Category:       "skill",
		Name:           firstNonEmpty(item.Name, item.ID),
		Description:    item.Description,
		Source:         item.Source,
		SourceScope:    item.SourceScope,
		Enabled:        item.Enabled,
		DefaultEnabled: item.DefaultEnabled,
		Status:         "available",
		Risk:           item.Risk,
		Tags:           capabilityTags(item.ResourceTypes, item.TaskIntents, item.Modes),
		Facets: CapabilityFacets{Skill: &CapabilitySkillFacet{
			ActivationMode: item.ActivationMode,
			InvocationMode: item.InvocationMode,
			AllowedTools:   cloneAppUIStrings(item.AllowedTools),
			DeniedTools:    cloneAppUIStrings(item.DeniedTools),
			UserInvocable:  item.UserInvocable,
			ModelInvocable: item.ModelInvocable,
		}},
	}
}

func capabilityRecordFromMcpCatalogItem(item McpCatalogItem) CapabilityRecord {
	status := firstNonEmpty(item.RuntimeStatus, item.ApprovalStatus, "available")
	return CapabilityRecord{
		ID:             item.ID,
		Kind:           "capability",
		Category:       "data",
		Name:           firstNonEmpty(item.Name, item.ID),
		Source:         item.Source,
		SourceScope:    item.SourceScope,
		Enabled:        item.Enabled,
		DefaultEnabled: item.DefaultEnabled,
		Status:         status,
		Risk:           item.Risk,
		Facets: CapabilityFacets{Connection: &CapabilityConnectionFacet{
			Type:                         item.Type,
			Permission:                   item.Permission,
			ApprovalStatus:               item.ApprovalStatus,
			RuntimeStatus:                item.RuntimeStatus,
			RequiresExplicitUserApproval: item.RequiresExplicitUserApproval,
		}},
	}
}

func capabilityRecordFromPluginSpec(spec plugins.Spec) CapabilityRecord {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = strings.TrimSpace(spec.Manifest.Name)
	}
	if name == "" {
		return CapabilityRecord{}
	}
	return CapabilityRecord{
		ID:          "plugin:" + name,
		Kind:        "capability",
		Category:    "extension",
		Name:        name,
		Source:      pluginSourceLabel(name),
		SourceScope: "plugin",
		Status:      "available",
		Facets: CapabilityFacets{Plugin: &CapabilityPluginFacet{
			Name:           name,
			SkillCount:     len(spec.Skills),
			MCPServerCount: len(spec.MCPServers),
			CommandCount:   len(spec.Commands),
			AgentCount:     len(spec.Agents),
			ManifestPath:   spec.Manifest.ManifestPath,
			Root:           spec.Manifest.Root,
		}},
	}
}

func capabilityTags(groups ...[]string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, group := range groups {
		for _, value := range group {
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
	}
	sort.Strings(out)
	return out
}
