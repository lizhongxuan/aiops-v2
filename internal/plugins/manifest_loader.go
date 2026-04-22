package plugins

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"aiops-v2/internal/agents"
	"aiops-v2/internal/commands"
	"aiops-v2/internal/lsp"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/outputstyle"
	"aiops-v2/internal/settings"
	"aiops-v2/internal/skills"
)

const manifestRelativePath = ".codex-plugin/plugin.json"

// ManifestLoader loads plugin specs from plugin manifests on disk.
type ManifestLoader struct {
	roots []string
}

// NewManifestLoader creates a loader that scans the provided roots for plugin manifests.
func NewManifestLoader(roots ...string) *ManifestLoader {
	return &ManifestLoader{roots: dedupeNonEmptyPaths(roots)}
}

// Load scans configured roots, parses manifests, and assembles plugin specs.
func (l *ManifestLoader) Load() ([]Spec, error) {
	manifestPaths, err := l.discoverManifestPaths()
	if err != nil {
		return nil, err
	}
	sort.Strings(manifestPaths)

	out := make([]Spec, 0, len(manifestPaths))
	for _, manifestPath := range manifestPaths {
		spec, err := loadManifestSpec(manifestPath)
		if err != nil {
			return nil, fmt.Errorf("plugins: load manifest %s: %w", manifestPath, err)
		}
		out = append(out, spec)
	}
	return out, nil
}

func (l *ManifestLoader) discoverManifestPaths() ([]string, error) {
	if len(l.roots) == 0 {
		return nil, nil
	}

	var out []string
	for _, root := range l.roots {
		paths, err := discoverManifestPaths(root)
		if err != nil {
			return nil, err
		}
		out = append(out, paths...)
	}
	return dedupeNonEmptyPaths(out), nil
}

type rawManifest struct {
	Name                          string             `json:"name"`
	CommandsPath                  string             `json:"commandsPath"`
	CommandsPaths                 []string           `json:"commandsPaths"`
	AgentsPath                    string             `json:"agentsPath"`
	AgentsPaths                   []string           `json:"agentsPaths"`
	SkillsPath                    string             `json:"skillsPath"`
	SkillsPaths                   []string           `json:"skillsPaths"`
	OutputStylesPath              string             `json:"outputStylesPath"`
	OutputStylesPaths             []string           `json:"outputStylesPaths"`
	HooksConfig                   string             `json:"hooksConfig"`
	MCPServers                    []mcp.ServerConfig `json:"mcpServers"`
	LSPServers                    []lsp.ServerConfig `json:"lspServers"`
	Settings                      []settings.Entry   `json:"settings"`
	StrictPluginOnlyCustomization bool               `json:"strictPluginOnlyCustomization"`
	AllowedMCPServers             []string           `json:"allowedMcpServers"`
	AdditionalDirectories         []string           `json:"additionalDirectories"`
}

func discoverManifestPaths(root string) ([]string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return nil, nil
	}

	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() && isPluginManifest(absRoot) {
		return []string{absRoot}, nil
	}

	var manifests []string
	err = filepath.WalkDir(absRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if isPluginManifest(path) {
			manifests = append(manifests, path)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return manifests, nil
}

func isPluginManifest(path string) bool {
	return filepath.Base(path) == "plugin.json" && filepath.Base(filepath.Dir(path)) == ".codex-plugin"
}

func loadManifestSpec(manifestPath string) (Spec, error) {
	raw, err := readRawManifest(manifestPath)
	if err != nil {
		return Spec{}, err
	}

	manifest, err := normalizeManifest(manifestPath, raw)
	if err != nil {
		return Spec{}, err
	}

	spec := Spec{
		Name:     manifest.Name,
		Manifest: manifest,
	}

	if spec.Commands, err = loadPromptCommandsFromPaths(manifest.CommandsPaths); err != nil {
		return Spec{}, err
	}
	if spec.Agents, err = loadAgentDefinitionsFromPaths(manifest.AgentsPaths); err != nil {
		return Spec{}, err
	}
	if spec.Skills, err = loadSkillDefinitionsFromPaths(manifest.SkillsPaths); err != nil {
		return Spec{}, err
	}
	if spec.OutputStyles, err = loadOutputStylesFromPaths(manifest.OutputStylesPaths); err != nil {
		return Spec{}, err
	}

	spec.LSPServers = cloneLSPServers(manifest.LSPServers)
	for i := range spec.LSPServers {
		if strings.TrimSpace(spec.LSPServers[i].Source) == "" {
			spec.LSPServers[i].Source = commands.SourcePlugin
		}
	}

	spec.Settings = cloneSettingsEntries(manifest.Settings)
	spec.MCPServers = make([]MCPServerSpec, 0, len(manifest.MCPServers))
	for _, cfg := range manifest.MCPServers {
		spec.MCPServers = append(spec.MCPServers, MCPServerSpec{Config: cloneMCPServerConfig(cfg)})
	}
	return spec, nil
}

func readRawManifest(manifestPath string) (rawManifest, error) {
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return rawManifest{}, err
	}

	var raw rawManifest
	if err := json.Unmarshal(data, &raw); err != nil {
		return rawManifest{}, err
	}
	return raw, nil
}

func normalizeManifest(manifestPath string, raw rawManifest) (Manifest, error) {
	absManifestPath, err := filepath.Abs(manifestPath)
	if err != nil {
		return Manifest{}, err
	}
	root := filepath.Dir(filepath.Dir(absManifestPath))
	name := strings.TrimSpace(raw.Name)
	if name == "" {
		name = filepath.Base(root)
	}

	commandsPaths, err := resolvePaths(root, raw.CommandsPath, raw.CommandsPaths)
	if err != nil {
		return Manifest{}, err
	}
	agentsPaths, err := resolvePaths(root, raw.AgentsPath, raw.AgentsPaths)
	if err != nil {
		return Manifest{}, err
	}
	skillsPaths, err := resolvePaths(root, raw.SkillsPath, raw.SkillsPaths)
	if err != nil {
		return Manifest{}, err
	}
	outputStylesPaths, err := resolvePaths(root, raw.OutputStylesPath, raw.OutputStylesPaths)
	if err != nil {
		return Manifest{}, err
	}

	hooksConfig := strings.TrimSpace(raw.HooksConfig)
	if hooksConfig != "" {
		hooksConfig = resolvePath(root, hooksConfig)
		if _, err := os.Stat(hooksConfig); err != nil {
			return Manifest{}, fmt.Errorf("hooksConfig %q: %w", hooksConfig, err)
		}
	}

	return Manifest{
		Name:                          name,
		ManifestPath:                  absManifestPath,
		Root:                          root,
		CommandsPaths:                 commandsPaths,
		AgentsPaths:                   agentsPaths,
		SkillsPaths:                   skillsPaths,
		OutputStylesPaths:             outputStylesPaths,
		HooksConfig:                   hooksConfig,
		MCPServers:                    cloneMCPServerConfigs(raw.MCPServers),
		LSPServers:                    cloneLSPServers(raw.LSPServers),
		Settings:                      cloneSettingsEntries(raw.Settings),
		StrictPluginOnlyCustomization: raw.StrictPluginOnlyCustomization,
		AllowedMCPServers:             dedupeNonEmptyStrings(raw.AllowedMCPServers),
		AdditionalDirectories:         dedupeNonEmptyStrings(raw.AdditionalDirectories),
	}, nil
}

func loadPromptCommandsFromPaths(paths []string) ([]commands.PromptCommand, error) {
	paths, err := expandJSONPaths(paths)
	if err != nil {
		return nil, err
	}
	paths = dedupeFileIdentityPaths(paths)

	var out []commands.PromptCommand
	for _, path := range paths {
		items, err := decodeJSONItems[commands.PromptCommand](path)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			item.Name = strings.TrimSpace(item.Name)
			item.Description = strings.TrimSpace(item.Description)
			item.Prompt = strings.TrimSpace(item.Prompt)
			if strings.TrimSpace(item.Source) == "" {
				item.Source = commands.SourcePlugin
			}
			if strings.TrimSpace(item.LoadedFrom) == "" {
				item.LoadedFrom = path
			}
			out = append(out, item)
		}
	}
	return out, nil
}

func loadAgentDefinitionsFromPaths(paths []string) ([]agents.Definition, error) {
	paths, err := expandJSONPaths(paths)
	if err != nil {
		return nil, err
	}

	var out []agents.Definition
	for _, path := range paths {
		items, err := decodeJSONItems[agents.Definition](path)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.TrimSpace(item.Source) == "" {
				item.Source = string(agents.SourcePlugin)
			}
			out = append(out, item)
		}
	}
	return out, nil
}

func loadSkillDefinitionsFromPaths(paths []string) ([]skills.Definition, error) {
	paths = dedupeNonEmptyPaths(paths)
	if len(paths) == 0 {
		return nil, nil
	}

	loader := skills.NewLoader()
	var out []skills.Definition
	for _, path := range paths {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if info.IsDir() {
			items, err := loader.LoadDir(path)
			if err != nil {
				return nil, err
			}
			for i := range items {
				items[i].Source = commands.SourcePlugin
			}
			out = append(out, items...)
			continue
		}
		if filepath.Base(path) != "SKILL.md" {
			return nil, fmt.Errorf("skill path %q must be a directory or SKILL.md file", path)
		}
		items, err := loader.LoadDir(filepath.Dir(path))
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			item.Source = commands.SourcePlugin
			if filepath.Clean(item.LoadedFrom) == filepath.Clean(path) {
				out = append(out, item)
			}
		}
	}
	return dedupeSkillDefinitionsByFileIdentity(out), nil
}

func loadOutputStylesFromPaths(paths []string) ([]outputstyle.Definition, error) {
	paths, err := expandJSONPaths(paths)
	if err != nil {
		return nil, err
	}

	var out []outputstyle.Definition
	for _, path := range paths {
		items, err := decodeJSONItems[outputstyle.Definition](path)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			if strings.TrimSpace(item.Source) == "" {
				item.Source = commands.SourcePlugin
			}
			out = append(out, item)
		}
	}
	return out, nil
}

func resolvePaths(root, singular string, plural []string) ([]string, error) {
	items := make([]string, 0, 1+len(plural))
	if strings.TrimSpace(singular) != "" {
		items = append(items, singular)
	}
	items = append(items, plural...)

	var out []string
	for _, item := range dedupeNonEmptyStrings(items) {
		resolved := resolvePath(root, item)
		if _, err := os.Stat(resolved); err != nil {
			return nil, fmt.Errorf("component path %q: %w", resolved, err)
		}
		out = append(out, resolved)
	}
	return out, nil
}

func resolvePath(root, path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}
	return filepath.Clean(filepath.Join(root, path))
}

func expandJSONPaths(paths []string) ([]string, error) {
	var out []string
	for _, path := range dedupeNonEmptyPaths(paths) {
		info, err := os.Stat(path)
		if err != nil {
			return nil, err
		}
		if !info.IsDir() {
			out = append(out, path)
			continue
		}

		var files []string
		err = filepath.WalkDir(path, func(candidate string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(candidate), ".json") {
				files = append(files, candidate)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		sort.Strings(files)
		out = append(out, files...)
	}
	return out, nil
}

func decodeJSONItems[T any](path string) ([]T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return nil, nil
	}

	if data[0] == '[' {
		var items []T
		if err := json.Unmarshal(data, &items); err != nil {
			return nil, fmt.Errorf("decode %s: %w", path, err)
		}
		return items, nil
	}

	var item T
	if err := json.Unmarshal(data, &item); err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return []T{item}, nil
}

func dedupeNonEmptyPaths(items []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if abs, err := filepath.Abs(item); err == nil {
			item = filepath.Clean(abs)
		} else {
			item = filepath.Clean(item)
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeNonEmptyStrings(items []string) []string {
	seen := make(map[string]struct{})
	var out []string
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	return out
}

func dedupeFileIdentityPaths(paths []string) []string {
	seen := make(map[string]struct{}, len(paths))
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		identity := skills.ResolveFileIdentity(path)
		if identity == "" {
			identity = path
		}
		if _, ok := seen[identity]; ok {
			continue
		}
		seen[identity] = struct{}{}
		out = append(out, path)
	}
	return out
}

func dedupeSkillDefinitionsByFileIdentity(defs []skills.Definition) []skills.Definition {
	seen := make(map[string]struct{}, len(defs))
	out := make([]skills.Definition, 0, len(defs))
	for _, def := range defs {
		identity := strings.TrimSpace(def.FileID)
		if identity == "" {
			identity = strings.TrimSpace(def.LoadedFrom)
		}
		if identity != "" {
			if _, ok := seen[identity]; ok {
				continue
			}
			seen[identity] = struct{}{}
		}
		out = append(out, def)
	}
	return out
}

func cloneMCPServerConfigs(items []mcp.ServerConfig) []mcp.ServerConfig {
	if len(items) == 0 {
		return nil
	}
	out := make([]mcp.ServerConfig, len(items))
	for i, item := range items {
		out[i] = cloneMCPServerConfig(item)
	}
	return out
}

func cloneMCPServerConfig(item mcp.ServerConfig) mcp.ServerConfig {
	item.Command = append([]string(nil), item.Command...)
	return item
}

func cloneLSPServers(items []lsp.ServerConfig) []lsp.ServerConfig {
	if len(items) == 0 {
		return nil
	}
	out := make([]lsp.ServerConfig, len(items))
	for i, item := range items {
		item.Command = append([]string(nil), item.Command...)
		item.Languages = append([]string(nil), item.Languages...)
		item.Roots = append([]string(nil), item.Roots...)
		out[i] = item
	}
	return out
}

func cloneSettingsEntries(items []settings.Entry) []settings.Entry {
	if len(items) == 0 {
		return nil
	}
	out := make([]settings.Entry, len(items))
	for i, item := range items {
		values := make(map[string]any, len(item.Values))
		for key, value := range item.Values {
			values[key] = value
		}
		out[i] = settings.Entry{Name: item.Name, Values: values}
	}
	return out
}
