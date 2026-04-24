package appui

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/store"
)

type agentProfileRepositories interface {
	SkillCatalogRepository
	AgentMCPCatalogRepository
	AgentProfileRepository
}

type mergedAgentProfileRepositories struct {
	skills   SkillCatalogRepository
	mcps     AgentMCPCatalogRepository
	profiles AgentProfileRepository
}

func newAgentProfileRepositories(skills SkillCatalogRepository, mcps AgentMCPCatalogRepository, profiles AgentProfileRepository) agentProfileRepositories {
	if skills == nil || mcps == nil || profiles == nil {
		return nil
	}
	if merged, ok := any(skills).(agentProfileRepositories); ok && any(skills) == any(mcps) && any(skills) == any(profiles) {
		return merged
	}
	return mergedAgentProfileRepositories{
		skills:   skills,
		mcps:     mcps,
		profiles: profiles,
	}
}

func (r mergedAgentProfileRepositories) GetSkillCatalog() ([]store.SkillCatalogEntry, error) {
	return r.skills.GetSkillCatalog()
}

func (r mergedAgentProfileRepositories) SaveSkillCatalog(items []store.SkillCatalogEntry) error {
	return r.skills.SaveSkillCatalog(items)
}

func (r mergedAgentProfileRepositories) GetAgentMCPCatalog() ([]store.AgentMCPCatalogEntry, error) {
	return r.mcps.GetAgentMCPCatalog()
}

func (r mergedAgentProfileRepositories) SaveAgentMCPCatalog(items []store.AgentMCPCatalogEntry) error {
	return r.mcps.SaveAgentMCPCatalog(items)
}

func (r mergedAgentProfileRepositories) GetAgentProfiles() ([]store.AgentProfileRecord, error) {
	return r.profiles.GetAgentProfiles()
}

func (r mergedAgentProfileRepositories) SaveAgentProfiles(items []store.AgentProfileRecord) error {
	return r.profiles.SaveAgentProfiles(items)
}

type defaultAgentProfileService struct {
	repo agentProfileRepositories
}

func NewAgentProfileService(repo agentProfileRepositories) AgentProfileService {
	return &defaultAgentProfileService{repo: repo}
}

func (s *defaultAgentProfileService) ListSkillCatalog(context.Context) ([]SkillCatalogItem, error) {
	entries, err := s.skillCatalogEntries()
	if err != nil {
		return nil, err
	}
	return mapSkillCatalogEntries(entries), nil
}

func (s *defaultAgentProfileService) SaveSkillCatalogItem(_ context.Context, item SkillCatalogItem) (SkillCatalogPayload, error) {
	entry, err := normalizeSkillCatalogItem(item)
	if err != nil {
		return SkillCatalogPayload{}, err
	}
	entries, err := s.mutableSkillCatalogEntries()
	if err != nil {
		return SkillCatalogPayload{}, err
	}
	replaced := false
	for i := range entries {
		if entries[i].ID == entry.ID {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	if err := s.saveSkillCatalog(entries); err != nil {
		return SkillCatalogPayload{}, err
	}
	return SkillCatalogPayload{
		Item:  mapSkillCatalogEntry(entry),
		Items: mapSkillCatalogEntries(entries),
	}, nil
}

func (s *defaultAgentProfileService) DeleteSkillCatalogItem(_ context.Context, id string) (SkillCatalogPayload, error) {
	target := strings.TrimSpace(id)
	if target == "" {
		return SkillCatalogPayload{}, fmt.Errorf("skill id is required")
	}
	entries, err := s.mutableSkillCatalogEntries()
	if err != nil {
		return SkillCatalogPayload{}, err
	}
	filtered := make([]store.SkillCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ID == target {
			continue
		}
		filtered = append(filtered, entry)
	}
	if err := s.saveSkillCatalog(filtered); err != nil {
		return SkillCatalogPayload{}, err
	}
	return SkillCatalogPayload{Items: mapSkillCatalogEntries(filtered)}, nil
}

func (s *defaultAgentProfileService) ListMcpCatalog(context.Context) ([]McpCatalogItem, error) {
	entries, err := s.mcpCatalogEntries()
	if err != nil {
		return nil, err
	}
	return mapMcpCatalogEntries(entries), nil
}

func (s *defaultAgentProfileService) SaveMcpCatalogItem(_ context.Context, item McpCatalogItem) (McpCatalogPayload, error) {
	entry, err := normalizeMcpCatalogItem(item)
	if err != nil {
		return McpCatalogPayload{}, err
	}
	entries, err := s.mutableMcpCatalogEntries()
	if err != nil {
		return McpCatalogPayload{}, err
	}
	replaced := false
	for i := range entries {
		if entries[i].ID == entry.ID {
			entries[i] = entry
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	if err := s.saveMcpCatalog(entries); err != nil {
		return McpCatalogPayload{}, err
	}
	return McpCatalogPayload{
		Item:  mapMcpCatalogEntry(entry),
		Items: mapMcpCatalogEntries(entries),
	}, nil
}

func (s *defaultAgentProfileService) DeleteMcpCatalogItem(_ context.Context, id string) (McpCatalogPayload, error) {
	target := strings.TrimSpace(id)
	if target == "" {
		return McpCatalogPayload{}, fmt.Errorf("mcp id is required")
	}
	entries, err := s.mutableMcpCatalogEntries()
	if err != nil {
		return McpCatalogPayload{}, err
	}
	filtered := make([]store.AgentMCPCatalogEntry, 0, len(entries))
	for _, entry := range entries {
		if entry.ID == target {
			continue
		}
		filtered = append(filtered, entry)
	}
	if err := s.saveMcpCatalog(filtered); err != nil {
		return McpCatalogPayload{}, err
	}
	return McpCatalogPayload{Items: mapMcpCatalogEntries(filtered)}, nil
}

func (s *defaultAgentProfileService) ListAgentProfiles(ctx context.Context) (AgentProfilesList, error) {
	skills, err := s.ListSkillCatalog(ctx)
	if err != nil {
		return AgentProfilesList{}, err
	}
	mcps, err := s.ListMcpCatalog(ctx)
	if err != nil {
		return AgentProfilesList{}, err
	}
	profiles, err := s.profileEntries()
	if err != nil {
		return AgentProfilesList{}, err
	}
	return AgentProfilesList{
		Items:        profiles,
		SkillCatalog: skills,
		McpCatalog:   mcps,
	}, nil
}

func (s *defaultAgentProfileService) GetAgentProfile(context.Context) (store.AgentProfileRecord, error) {
	profiles, err := s.profileEntries()
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if strings.TrimSpace(stringField(profile, "id")) == "main-agent" {
			return cloneProfile(profile), nil
		}
	}
	if len(profiles) == 0 {
		return nil, fmt.Errorf("agent profile not found")
	}
	return cloneProfile(profiles[0]), nil
}

func (s *defaultAgentProfileService) SaveAgentProfile(_ context.Context, profile store.AgentProfileRecord) (store.AgentProfileRecord, error) {
	id := strings.TrimSpace(stringField(profile, "id"))
	if id == "" {
		return nil, fmt.Errorf("agent profile id is required")
	}
	next := cloneProfile(profile)
	if next == nil {
		next = store.AgentProfileRecord{}
	}
	entries, err := s.profileEntries()
	if err != nil {
		return nil, err
	}
	base := findProfile(entries, id)
	if base == nil {
		base = defaultAgentProfiles()[id]
	}
	merged := mergeProfile(base, next)
	merged["id"] = id
	if strings.TrimSpace(stringField(merged, "type")) == "" {
		merged["type"] = id
	}
	replaced := false
	for i := range entries {
		if strings.TrimSpace(stringField(entries[i], "id")) == id {
			entries[i] = merged
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, merged)
	}
	if err := s.saveProfiles(entries); err != nil {
		return nil, err
	}
	return cloneProfile(merged), nil
}

func (s *defaultAgentProfileService) ResetAgentProfile(_ context.Context, profileID string) (store.AgentProfileRecord, error) {
	target := strings.TrimSpace(firstNonEmpty(profileID, "main-agent"))
	defaults := defaultAgentProfiles()
	base, ok := defaults[target]
	if !ok {
		return nil, fmt.Errorf("profile %q not found", target)
	}
	entries, err := s.profileEntries()
	if err != nil {
		return nil, err
	}
	replaced := false
	for i := range entries {
		if strings.TrimSpace(stringField(entries[i], "id")) == target {
			entries[i] = cloneProfile(base)
			replaced = true
			break
		}
	}
	if !replaced {
		entries = append(entries, cloneProfile(base))
	}
	if err := s.saveProfiles(entries); err != nil {
		return nil, err
	}
	return cloneProfile(base), nil
}

func (s *defaultAgentProfileService) PreviewAgentProfile(_ context.Context, profileID string) (AgentProfilePreview, error) {
	profile, err := s.profileByID(profileID)
	if err != nil {
		return AgentProfilePreview{}, err
	}
	systemPrompt := nestedStringField(profile, "systemPrompt", "content")
	return AgentProfilePreview{
		ProfileID:         stringField(profile, "id"),
		ProfileType:       firstNonEmpty(stringField(profile, "type"), stringField(profile, "id")),
		SystemPrompt:      systemPrompt,
		SystemPromptLines: countLines(systemPrompt),
		CommandSummary:    summarizePermissions(profile["commandPermissions"], "categoryPolicies"),
		CapabilitySummary: summarizeCapabilities(profile["capabilityPermissions"]),
		EnabledSkills:     enabledBindings(profile["skills"]),
		EnabledMcps:       enabledBindings(firstNonNil(profile["mcps"], profile["mcpServers"])),
		Runtime:           cloneAnyMap(mapField(profile, "runtime")),
	}, nil
}

func (s *defaultAgentProfileService) ExportAgentProfiles(_ context.Context) (AgentProfilesExportPayload, error) {
	profiles, err := s.profileEntries()
	if err != nil {
		return AgentProfilesExportPayload{}, err
	}
	return AgentProfilesExportPayload{
		Version:       1,
		ConfigVersion: 1,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Count:         len(profiles),
		Profiles:      profiles,
	}, nil
}

func (s *defaultAgentProfileService) ImportAgentProfiles(_ context.Context, payload AgentProfilesImportPayload) (AgentProfilesExportPayload, error) {
	if len(payload.Profiles) == 0 {
		return AgentProfilesExportPayload{}, fmt.Errorf("profiles are required")
	}
	profiles := make([]store.AgentProfileRecord, 0, len(payload.Profiles))
	for _, profile := range payload.Profiles {
		if strings.TrimSpace(stringField(profile, "id")) == "" {
			continue
		}
		profiles = append(profiles, cloneProfile(profile))
	}
	if len(profiles) == 0 {
		return AgentProfilesExportPayload{}, fmt.Errorf("profiles are required")
	}
	if err := s.saveProfiles(profiles); err != nil {
		return AgentProfilesExportPayload{}, err
	}
	return AgentProfilesExportPayload{
		Version:       1,
		ConfigVersion: 1,
		ExportedAt:    time.Now().UTC().Format(time.RFC3339),
		Count:         len(profiles),
		Profiles:      profiles,
	}, nil
}

func (s *defaultAgentProfileService) skillCatalogEntries() ([]store.SkillCatalogEntry, error) {
	if s.repo == nil {
		return defaultSkillCatalogEntries(), nil
	}
	items, err := s.repo.GetSkillCatalog()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return defaultSkillCatalogEntries(), nil
	}
	return append([]store.SkillCatalogEntry(nil), items...), nil
}

func (s *defaultAgentProfileService) mcpCatalogEntries() ([]store.AgentMCPCatalogEntry, error) {
	if s.repo == nil {
		return defaultMcpCatalogEntries(), nil
	}
	items, err := s.repo.GetAgentMCPCatalog()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return defaultMcpCatalogEntries(), nil
	}
	return append([]store.AgentMCPCatalogEntry(nil), items...), nil
}

func (s *defaultAgentProfileService) profileEntries() ([]store.AgentProfileRecord, error) {
	if s.repo == nil {
		return defaultAgentProfilesList(), nil
	}
	items, err := s.repo.GetAgentProfiles()
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return defaultAgentProfilesList(), nil
	}
	out := make([]store.AgentProfileRecord, 0, len(items))
	for _, item := range items {
		out = append(out, cloneProfile(item))
	}
	return out, nil
}

func (s *defaultAgentProfileService) saveSkillCatalog(items []store.SkillCatalogEntry) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.SaveSkillCatalog(items)
}

func (s *defaultAgentProfileService) saveMcpCatalog(items []store.AgentMCPCatalogEntry) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.SaveAgentMCPCatalog(items)
}

func (s *defaultAgentProfileService) saveProfiles(items []store.AgentProfileRecord) error {
	if s.repo == nil {
		return nil
	}
	return s.repo.SaveAgentProfiles(items)
}

func (s *defaultAgentProfileService) mutableSkillCatalogEntries() ([]store.SkillCatalogEntry, error) {
	if s.repo == nil {
		return []store.SkillCatalogEntry{}, nil
	}
	items, err := s.repo.GetSkillCatalog()
	if err != nil {
		return nil, err
	}
	return append([]store.SkillCatalogEntry(nil), items...), nil
}

func (s *defaultAgentProfileService) mutableMcpCatalogEntries() ([]store.AgentMCPCatalogEntry, error) {
	if s.repo == nil {
		return []store.AgentMCPCatalogEntry{}, nil
	}
	items, err := s.repo.GetAgentMCPCatalog()
	if err != nil {
		return nil, err
	}
	return append([]store.AgentMCPCatalogEntry(nil), items...), nil
}

func (s *defaultAgentProfileService) profileByID(profileID string) (store.AgentProfileRecord, error) {
	target := strings.TrimSpace(firstNonEmpty(profileID, "main-agent"))
	profiles, err := s.profileEntries()
	if err != nil {
		return nil, err
	}
	for _, profile := range profiles {
		if strings.TrimSpace(stringField(profile, "id")) == target || strings.TrimSpace(stringField(profile, "type")) == target {
			return cloneProfile(profile), nil
		}
	}
	if base, ok := defaultAgentProfiles()[target]; ok {
		return cloneProfile(base), nil
	}
	return nil, fmt.Errorf("profile %q not found", target)
}

func mapSkillCatalogEntries(entries []store.SkillCatalogEntry) []SkillCatalogItem {
	out := make([]SkillCatalogItem, 0, len(entries))
	for _, entry := range entries {
		out = append(out, mapSkillCatalogEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func mapSkillCatalogEntry(entry store.SkillCatalogEntry) SkillCatalogItem {
	mode := strings.TrimSpace(entry.DefaultActivationMode)
	if mode == "" {
		if entry.DefaultEnabled {
			mode = "default_enabled"
		} else {
			mode = "disabled"
		}
	}
	return SkillCatalogItem{
		ID:                    entry.ID,
		Name:                  entry.Name,
		Description:           entry.Description,
		Source:                firstNonEmpty(entry.Source, "local"),
		Enabled:               entry.DefaultEnabled,
		DefaultEnabled:        entry.DefaultEnabled,
		ActivationMode:        mode,
		DefaultActivationMode: mode,
	}
}

func normalizeSkillCatalogItem(item SkillCatalogItem) (store.SkillCatalogEntry, error) {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		return store.SkillCatalogEntry{}, fmt.Errorf("skill id is required")
	}
	mode := strings.TrimSpace(firstNonEmpty(item.DefaultActivationMode, item.ActivationMode))
	if mode == "" {
		if item.Enabled || item.DefaultEnabled {
			mode = "default_enabled"
		} else {
			mode = "disabled"
		}
	}
	return store.SkillCatalogEntry{
		ID:                    id,
		Name:                  strings.TrimSpace(firstNonEmpty(item.Name, id)),
		Description:           strings.TrimSpace(item.Description),
		Source:                strings.TrimSpace(firstNonEmpty(item.Source, "local")),
		DefaultEnabled:        item.Enabled || item.DefaultEnabled,
		DefaultActivationMode: mode,
	}, nil
}

func mapMcpCatalogEntries(entries []store.AgentMCPCatalogEntry) []McpCatalogItem {
	out := make([]McpCatalogItem, 0, len(entries))
	for _, entry := range entries {
		out = append(out, mapMcpCatalogEntry(entry))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func mapMcpCatalogEntry(entry store.AgentMCPCatalogEntry) McpCatalogItem {
	return McpCatalogItem{
		ID:                           entry.ID,
		Name:                         entry.Name,
		Type:                         firstNonEmpty(entry.Type, "stdio"),
		Source:                       firstNonEmpty(entry.Source, "local"),
		Enabled:                      entry.DefaultEnabled,
		DefaultEnabled:               entry.DefaultEnabled,
		Permission:                   firstNonEmpty(entry.Permission, "readonly"),
		RequiresExplicitUserApproval: entry.RequiresExplicitUserApproval,
	}
}

func normalizeMcpCatalogItem(item McpCatalogItem) (store.AgentMCPCatalogEntry, error) {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		return store.AgentMCPCatalogEntry{}, fmt.Errorf("mcp id is required")
	}
	return store.AgentMCPCatalogEntry{
		ID:                           id,
		Name:                         strings.TrimSpace(firstNonEmpty(item.Name, id)),
		Type:                         strings.TrimSpace(firstNonEmpty(item.Type, "stdio")),
		Source:                       strings.TrimSpace(firstNonEmpty(item.Source, "local")),
		DefaultEnabled:               item.Enabled || item.DefaultEnabled,
		Permission:                   strings.TrimSpace(firstNonEmpty(item.Permission, "readonly")),
		RequiresExplicitUserApproval: item.RequiresExplicitUserApproval,
	}, nil
}

func defaultSkillCatalogEntries() []store.SkillCatalogEntry {
	return []store.SkillCatalogEntry{
		{ID: "ops-triage", Name: "Ops Triage", Description: "快速归类问题并给出最小干预路径。", Source: "built-in", DefaultEnabled: true, DefaultActivationMode: "default_enabled"},
		{ID: "incident-summary", Name: "Incident Summary", Description: "把诊断过程整理成可交付摘要。", Source: "local", DefaultEnabled: true, DefaultActivationMode: "default_enabled"},
		{ID: "safe-change-review", Name: "Safe Change Review", Description: "在执行前做变更影响检查。", Source: "built-in", DefaultEnabled: false, DefaultActivationMode: "disabled"},
	}
}

func defaultMcpCatalogEntries() []store.AgentMCPCatalogEntry {
	return []store.AgentMCPCatalogEntry{
		{ID: "filesystem", Name: "Filesystem MCP", Type: "stdio", Source: "built-in", DefaultEnabled: true, Permission: "readonly"},
		{ID: "docs", Name: "Docs MCP", Type: "http", Source: "local", DefaultEnabled: true, Permission: "readonly", RequiresExplicitUserApproval: true},
		{ID: "metrics", Name: "Metrics MCP", Type: "http", Source: "built-in", DefaultEnabled: false, Permission: "readwrite", RequiresExplicitUserApproval: true},
	}
}

func defaultAgentProfilesList() []store.AgentProfileRecord {
	defaults := defaultAgentProfiles()
	keys := make([]string, 0, len(defaults))
	for key := range defaults {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]store.AgentProfileRecord, 0, len(keys))
	for _, key := range keys {
		out = append(out, cloneProfile(defaults[key]))
	}
	return out
}

func defaultAgentProfiles() map[string]store.AgentProfileRecord {
	return map[string]store.AgentProfileRecord{
		"main-agent": {
			"id":          "main-agent",
			"name":        "main-agent",
			"type":        "main-agent",
			"description": "系统默认主 Agent 配置，用于会话编排、规划和结果收敛。",
			"runtime": map[string]any{
				"model":           "gpt-5.4",
				"reasoningEffort": "medium",
				"approvalPolicy":  "untrusted",
				"sandboxMode":     "workspace-write",
			},
			"systemPrompt": map[string]any{
				"content": "你是主 Agent。优先收敛目标、分解任务、控制风险，并在输出中保持清晰、可执行和可回溯。",
				"preview": "你是主 Agent。优先收敛目标、分解任务、控制风险。",
			},
			"commandPermissions": map[string]any{
				"enabled":     true,
				"defaultMode": "approval_required",
				"categoryPolicies": map[string]any{
					"system_inspection":   "allow",
					"service_read":        "allow",
					"network_read":        "approval_required",
					"file_read":           "allow",
					"filesystem_mutation": "approval_required",
					"service_mutation":    "approval_required",
					"package_mutation":    "deny",
				},
			},
			"capabilityPermissions": map[string]any{
				"commandExecution": "enabled",
				"fileRead":         "enabled",
				"fileSearch":       "enabled",
				"fileChange":       "approval_required",
				"terminal":         "approval_required",
				"webSearch":        "enabled",
				"approval":         "enabled",
				"multiAgent":       "enabled",
			},
			"skills": []any{
				map[string]any{"id": "ops-triage", "name": "Ops Triage", "enabled": true, "activationMode": "default_enabled"},
				map[string]any{"id": "incident-summary", "name": "Incident Summary", "enabled": true, "activationMode": "default_enabled"},
				map[string]any{"id": "safe-change-review", "name": "Safe Change Review", "enabled": false, "activationMode": "disabled"},
			},
			"mcps": []any{
				map[string]any{"id": "filesystem", "name": "Filesystem MCP", "enabled": true, "permission": "readonly"},
				map[string]any{"id": "docs", "name": "Docs MCP", "enabled": true, "permission": "readonly"},
				map[string]any{"id": "metrics", "name": "Metrics MCP", "enabled": false, "permission": "readwrite"},
			},
		},
		"host-agent-default": {
			"id":          "host-agent-default",
			"name":        "host-agent-default",
			"type":        "host-agent-default",
			"description": "默认 host-agent 静态配置，偏向安全读取和受限执行。",
			"runtime": map[string]any{
				"model":           "gpt-5.4-mini",
				"reasoningEffort": "low",
				"approvalPolicy":  "untrusted",
				"sandboxMode":     "workspace-write",
			},
			"systemPrompt": map[string]any{
				"content": "你是 host-agent。只负责在受控边界内执行局部操作，优先只读、低风险和可回滚的动作。",
				"preview": "你是 host-agent。只负责在受控边界内执行局部操作。",
			},
			"commandPermissions": map[string]any{
				"enabled":     true,
				"defaultMode": "readonly_only",
				"categoryPolicies": map[string]any{
					"system_inspection":   "allow",
					"service_read":        "allow",
					"network_read":        "allow",
					"file_read":           "allow",
					"filesystem_mutation": "approval_required",
					"service_mutation":    "approval_required",
					"package_mutation":    "deny",
				},
			},
			"capabilityPermissions": map[string]any{
				"commandExecution": "approval_required",
				"fileRead":         "enabled",
				"fileSearch":       "enabled",
				"terminal":         "enabled",
				"approval":         "enabled",
				"multiAgent":       "disabled",
			},
			"skills": []any{
				map[string]any{"id": "ops-triage", "name": "Ops Triage", "enabled": true, "activationMode": "default_enabled"},
			},
			"mcps": []any{
				map[string]any{"id": "filesystem", "name": "Filesystem MCP", "enabled": true, "permission": "readonly"},
			},
		},
	}
}

func summarizePermissions(raw any, key string) []string {
	value := mapValue(asAnyMap(raw), key)
	if mapping := asAnyMap(value); len(mapping) > 0 {
		keys := make([]string, 0, len(mapping))
		for field := range mapping {
			keys = append(keys, field)
		}
		sort.Strings(keys)
		out := make([]string, 0, len(keys))
		for _, field := range keys {
			out = append(out, field+": "+profileString(mapping[field]))
		}
		return out
	}
	var out []string
	for _, item := range asAnySlice(value) {
		entry := asAnyMap(item)
		label := firstNonEmpty(profileString(entry["label"]), profileString(entry["id"]))
		state := firstNonEmpty(profileString(entry["mode"]), profileString(entry["state"]))
		if label != "" {
			out = append(out, label+": "+state)
		}
	}
	return out
}

func summarizeCapabilities(raw any) []string {
	if mapping := asAnyMap(raw); len(mapping) > 0 {
		keys := make([]string, 0, len(mapping))
		for field := range mapping {
			keys = append(keys, field)
		}
		sort.Strings(keys)
		out := make([]string, 0, len(keys))
		for _, field := range keys {
			out = append(out, field+": "+profileString(mapping[field]))
		}
		return out
	}
	var out []string
	for _, item := range asAnySlice(raw) {
		entry := asAnyMap(item)
		label := firstNonEmpty(profileString(entry["label"]), profileString(entry["id"]))
		state := profileString(entry["state"])
		if label != "" {
			out = append(out, label+": "+state)
		}
	}
	return out
}

func enabledBindings(raw any) []map[string]any {
	items := asAnySlice(raw)
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		entry := asAnyMap(item)
		if !profileBool(entry["enabled"]) {
			continue
		}
		out = append(out, cloneAnyMap(entry))
	}
	return out
}

func countLines(text string) int {
	normalized := strings.ReplaceAll(text, "\r\n", "\n")
	if normalized == "" {
		return 0
	}
	return len(strings.Split(normalized, "\n"))
}

func stringField(record store.AgentProfileRecord, key string) string {
	return profileString(record[key])
}

func nestedStringField(record store.AgentProfileRecord, parent, key string) string {
	return profileString(mapField(record, parent)[key])
}

func mapField(record store.AgentProfileRecord, key string) map[string]any {
	return asAnyMap(record[key])
}

func cloneProfile(record store.AgentProfileRecord) store.AgentProfileRecord {
	if record == nil {
		return nil
	}
	raw, err := json.Marshal(record)
	if err != nil {
		cp := make(store.AgentProfileRecord, len(record))
		for key, value := range record {
			cp[key] = value
		}
		return cp
	}
	var out store.AgentProfileRecord
	if err := json.Unmarshal(raw, &out); err != nil {
		cp := make(store.AgentProfileRecord, len(record))
		for key, value := range record {
			cp[key] = value
		}
		return cp
	}
	return out
}

func mergeProfile(base, override store.AgentProfileRecord) store.AgentProfileRecord {
	merged := cloneProfile(base)
	if merged == nil {
		merged = store.AgentProfileRecord{}
	}
	for key, value := range override {
		merged[key] = value
	}
	return merged
}

func cloneAnyMap(src map[string]any) map[string]any {
	if src == nil {
		return nil
	}
	raw, err := json.Marshal(src)
	if err != nil {
		out := make(map[string]any, len(src))
		for key, value := range src {
			out[key] = value
		}
		return out
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		out = make(map[string]any, len(src))
		for key, value := range src {
			out[key] = value
		}
	}
	return out
}

func asAnyMap(value any) map[string]any {
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case store.AgentProfileRecord:
		return map[string]any(typed)
	default:
		return nil
	}
}

func asAnySlice(value any) []any {
	switch typed := value.(type) {
	case []any:
		return typed
	case []map[string]any:
		out := make([]any, 0, len(typed))
		for _, item := range typed {
			out = append(out, item)
		}
		return out
	default:
		return nil
	}
}

func profileString(value any) string {
	if value == nil {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return typed
	default:
		return fmt.Sprintf("%v", typed)
	}
}

func profileBool(value any) bool {
	typed, ok := value.(bool)
	return ok && typed
}

func mapValue(values map[string]any, key string) any {
	if values == nil {
		return nil
	}
	return values[key]
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func findProfile(entries []store.AgentProfileRecord, id string) store.AgentProfileRecord {
	target := strings.TrimSpace(id)
	for _, entry := range entries {
		if strings.TrimSpace(stringField(entry, "id")) == target || strings.TrimSpace(stringField(entry, "type")) == target {
			return cloneProfile(entry)
		}
	}
	return nil
}
