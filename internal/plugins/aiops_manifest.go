package plugins

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AIOpsManifest captures AIOps-specific plugin contributions. The base plugin
// loader parses and validates these fields, while domain registries decide how
// to consume them.
type AIOpsManifest struct {
	OpsManualCapabilityPacks []OpsManualCapabilityPackManifest `json:"opsmanual_capability_packs,omitempty"`
	RunnerActions            []RunnerActionManifest            `json:"runner_actions,omitempty"`
	AgentUIRenderers         []AgentUIRendererManifest         `json:"agent_ui_renderers,omitempty"`
	AgentProfiles            []AgentProfileManifest            `json:"agent_profiles,omitempty"`
	SettingsSchemas          []SettingsSchemaManifest          `json:"settings_schemas,omitempty"`
	PermissionDefaults       []PermissionDefaultManifest       `json:"permission_defaults,omitempty"`
}

type OpsManualCapabilityPackManifest struct {
	ID            string   `json:"id"`
	Surfaces      []string `json:"surfaces,omitempty"`
	ResourceTypes []string `json:"resource_types,omitempty"`
	MCPServer     string   `json:"mcp_server,omitempty"`
	Skill         string   `json:"skill,omitempty"`
}

type RunnerActionManifest struct {
	ID          string                 `json:"id"`
	Module      string                 `json:"module,omitempty"`
	Handler     string                 `json:"handler,omitempty"`
	InputSchema string                 `json:"input_schema,omitempty"`
	Risk        string                 `json:"risk,omitempty"`
	Approval    string                 `json:"approval,omitempty"`
	UI          RunnerActionUIManifest `json:"ui,omitempty"`
}

type RunnerActionUIManifest struct {
	Category string `json:"category,omitempty"`
	Icon     string `json:"icon,omitempty"`
}

type AgentUIRendererManifest struct {
	ID            string                         `json:"id"`
	ArtifactTypes []string                       `json:"artifact_types,omitempty"`
	SchemaVersion string                         `json:"schema_version,omitempty"`
	Component     string                         `json:"component,omitempty"`
	Fallback      string                         `json:"fallback,omitempty"`
	Display       AgentUIRendererDisplayManifest `json:"display,omitempty"`
}

type AgentUIRendererDisplayManifest struct {
	TitleField string `json:"title_field,omitempty"`
	Icon       string `json:"icon,omitempty"`
	HideFooter bool   `json:"hide_footer,omitempty"`
}

type AgentProfileManifest struct {
	ID                    string   `json:"id"`
	DisplayName           string   `json:"display_name,omitempty"`
	RecommendedSkills     []string `json:"recommended_skills,omitempty"`
	RecommendedMCPServers []string `json:"recommended_mcp_servers,omitempty"`
	Mode                  string   `json:"mode,omitempty"`
}

type SettingsSchemaManifest struct {
	ID   string `json:"id"`
	Path string `json:"path,omitempty"`
}

type PermissionDefaultManifest struct {
	ID     string `json:"id"`
	Target string `json:"target,omitempty"`
	Mode   string `json:"mode,omitempty"`
}

func parseAIOpsManifest(raw json.RawMessage, strict bool) (AIOpsManifest, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return AIOpsManifest{}, nil
	}
	if strict {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(raw, &fields); err != nil {
			return AIOpsManifest{}, fmt.Errorf("aiops: %w", err)
		}
		allowed := map[string]struct{}{
			"opsmanual_capability_packs": {},
			"runner_actions":             {},
			"agent_ui_renderers":         {},
			"agent_profiles":             {},
			"settings_schemas":           {},
			"permission_defaults":        {},
		}
		for field := range fields {
			if _, ok := allowed[field]; !ok {
				return AIOpsManifest{}, fmt.Errorf("aiops.%s: unknown field", field)
			}
		}
	}

	var manifest AIOpsManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return AIOpsManifest{}, fmt.Errorf("aiops: %w", err)
	}
	if err := validateAIOpsManifest(manifest); err != nil {
		return AIOpsManifest{}, err
	}
	return manifest, nil
}

func validateAIOpsManifest(manifest AIOpsManifest) error {
	for i, item := range manifest.OpsManualCapabilityPacks {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("aiops.opsmanual_capability_packs[%d].id: required", i)
		}
	}
	for i, item := range manifest.RunnerActions {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("aiops.runner_actions[%d].id: required", i)
		}
	}
	for i, item := range manifest.AgentUIRenderers {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("aiops.agent_ui_renderers[%d].id: required", i)
		}
		if len(item.ArtifactTypes) == 0 {
			return fmt.Errorf("aiops.agent_ui_renderers[%d].artifact_types: required", i)
		}
		if strings.TrimSpace(item.SchemaVersion) == "" {
			return fmt.Errorf("aiops.agent_ui_renderers[%d].schema_version: required", i)
		}
		if strings.TrimSpace(item.Fallback) == "" {
			return fmt.Errorf("aiops.agent_ui_renderers[%d].fallback: required", i)
		}
	}
	for i, item := range manifest.AgentProfiles {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("aiops.agent_profiles[%d].id: required", i)
		}
	}
	for i, item := range manifest.SettingsSchemas {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("aiops.settings_schemas[%d].id: required", i)
		}
	}
	for i, item := range manifest.PermissionDefaults {
		if strings.TrimSpace(item.ID) == "" {
			return fmt.Errorf("aiops.permission_defaults[%d].id: required", i)
		}
	}
	return nil
}

func cloneAIOpsManifest(manifest AIOpsManifest) AIOpsManifest {
	manifest.OpsManualCapabilityPacks = append([]OpsManualCapabilityPackManifest(nil), manifest.OpsManualCapabilityPacks...)
	manifest.RunnerActions = append([]RunnerActionManifest(nil), manifest.RunnerActions...)
	manifest.AgentUIRenderers = append([]AgentUIRendererManifest(nil), manifest.AgentUIRenderers...)
	manifest.AgentProfiles = append([]AgentProfileManifest(nil), manifest.AgentProfiles...)
	manifest.SettingsSchemas = append([]SettingsSchemaManifest(nil), manifest.SettingsSchemas...)
	manifest.PermissionDefaults = append([]PermissionDefaultManifest(nil), manifest.PermissionDefaults...)
	for i := range manifest.OpsManualCapabilityPacks {
		manifest.OpsManualCapabilityPacks[i].Surfaces = append([]string(nil), manifest.OpsManualCapabilityPacks[i].Surfaces...)
		manifest.OpsManualCapabilityPacks[i].ResourceTypes = append([]string(nil), manifest.OpsManualCapabilityPacks[i].ResourceTypes...)
	}
	for i := range manifest.AgentUIRenderers {
		manifest.AgentUIRenderers[i].ArtifactTypes = append([]string(nil), manifest.AgentUIRenderers[i].ArtifactTypes...)
	}
	for i := range manifest.AgentProfiles {
		manifest.AgentProfiles[i].RecommendedSkills = append([]string(nil), manifest.AgentProfiles[i].RecommendedSkills...)
		manifest.AgentProfiles[i].RecommendedMCPServers = append([]string(nil), manifest.AgentProfiles[i].RecommendedMCPServers...)
	}
	return manifest
}
