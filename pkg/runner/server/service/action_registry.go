package service

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const runnerActionSchemaVersion = "v1"

type ActionRegistry struct {
	mu    sync.RWMutex
	specs map[string]ActionSpec
}

type actionPluginManifest struct {
	Name  string            `json:"name"`
	AIOps actionPluginAIOps `json:"aiops"`
}

type actionPluginAIOps struct {
	RunnerActions []runnerActionManifest `json:"runner_actions"`
}

type runnerActionManifest struct {
	ID             string             `json:"id"`
	SchemaVersion  string             `json:"schema_version"`
	Title          string             `json:"title"`
	Label          string             `json:"label"`
	Category       string             `json:"category"`
	Description    string             `json:"description"`
	Risk           string             `json:"risk"`
	Approval       string             `json:"approval"`
	NodeType       string             `json:"node_type"`
	Capabilities   []string           `json:"capabilities"`
	ArgsSchema     json.RawMessage    `json:"args_schema"`
	InputSchema    json.RawMessage    `json:"input_schema"`
	OutputSchema   json.RawMessage    `json:"output_schema"`
	InputsSchema   json.RawMessage    `json:"inputs_schema"`
	OutputsSchema  json.RawMessage    `json:"outputs_schema"`
	DefaultPorts   ActionDefaultPorts `json:"default_ports"`
	Defaults       map[string]any     `json:"defaults"`
	RequiredArgs   []string           `json:"required_args"`
	ArgConflicts   [][]string         `json:"arg_conflicts"`
	Outputs        []OutputSpec       `json:"outputs"`
	Examples       []ActionExample    `json:"examples"`
	InputExamples  []ActionIOExample  `json:"input_examples"`
	OutputExamples []ActionIOExample  `json:"output_examples"`
	Experimental   *bool              `json:"experimental"`
	Deprecated     *bool              `json:"deprecated"`
	UI             struct {
		Category string `json:"category"`
		Icon     string `json:"icon"`
	} `json:"ui"`
}

func NewActionRegistry() *ActionRegistry {
	return &ActionRegistry{specs: map[string]ActionSpec{}}
}

func NewDefaultActionRegistry() *ActionRegistry {
	registry := NewActionRegistry()
	_ = registry.LoadPluginManifest(defaultRunnerCoreManifestPath())
	return registry
}

func (r *ActionRegistry) LoadPluginManifest(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return fmt.Errorf("%w: plugin manifest path is required", ErrInvalid)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var manifest actionPluginManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return err
	}
	for i, item := range manifest.AIOps.RunnerActions {
		spec, err := actionSpecFromManifest(item)
		if err != nil {
			return fmt.Errorf("aiops.runner_actions[%d]: %w", i, err)
		}
		if err := r.Register(spec); err != nil {
			return fmt.Errorf("aiops.runner_actions[%d]: %w", i, err)
		}
	}
	return nil
}

func (r *ActionRegistry) Register(spec ActionSpec) error {
	if r == nil {
		return fmt.Errorf("%w: action registry is nil", ErrInvalid)
	}
	normalized, err := normalizeActionSpec(spec)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.specs[normalized.Action] = normalized
	return nil
}

func (r *ActionRegistry) List() []ActionSpec {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]ActionSpec, 0, len(r.specs))
	for _, spec := range r.specs {
		items = append(items, cloneActionSpec(spec))
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Action < items[j].Action
	})
	return items
}

func (r *ActionRegistry) Get(action string) (ActionSpec, bool) {
	if r == nil {
		return ActionSpec{}, false
	}
	action = strings.TrimSpace(action)
	if action == "" {
		return ActionSpec{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	spec, ok := r.specs[action]
	if !ok {
		return ActionSpec{}, false
	}
	return cloneActionSpec(spec), true
}

func actionSpecFromManifest(item runnerActionManifest) (ActionSpec, error) {
	id := strings.TrimSpace(item.ID)
	if id == "" {
		return ActionSpec{}, fmt.Errorf("%w: id is required", ErrInvalid)
	}
	version := strings.TrimSpace(item.SchemaVersion)
	if version == "" {
		version = runnerActionSchemaVersion
	}
	if version != runnerActionSchemaVersion {
		return ActionSpec{}, fmt.Errorf("%w: action %q schema_version %q is not supported", ErrInvalid, id, version)
	}

	spec, hasTemplate := builtinActionSpecTemplate(id)
	if !hasTemplate {
		spec = ActionSpec{Action: id}
	}
	if value := strings.TrimSpace(item.Title); value != "" {
		spec.Title = value
	}
	if value := strings.TrimSpace(item.Label); value != "" {
		spec.Label = value
	}
	if value := strings.TrimSpace(item.Category); value != "" {
		spec.Category = value
	} else if value := strings.TrimSpace(item.UI.Category); value != "" {
		spec.Category = value
	}
	if value := strings.TrimSpace(item.Description); value != "" {
		spec.Description = value
	}
	if value := strings.TrimSpace(item.Risk); value != "" {
		spec.Risk = value
	}
	if value := strings.TrimSpace(item.NodeType); value != "" {
		spec.NodeType = value
	}
	if len(item.Capabilities) > 0 {
		spec.Capabilities = append([]string{}, item.Capabilities...)
	}
	if !hasTemplate {
		spec.ArgsSchema = firstRawMessage(item.ArgsSchema, item.InputsSchema, item.InputSchema)
	}
	if !hasTemplate {
		spec.InputSchema = firstRawMessage(item.InputSchema, item.InputsSchema, item.ArgsSchema)
	}
	if !hasTemplate {
		spec.InputsSchema = firstRawMessage(item.InputsSchema, item.InputSchema, item.ArgsSchema)
	}
	if !hasTemplate {
		spec.OutputSchema = firstRawMessage(item.OutputSchema, item.OutputsSchema)
	}
	if !hasTemplate {
		spec.OutputsSchema = firstRawMessage(item.OutputsSchema, item.OutputSchema)
	}
	if len(item.DefaultPorts.Inputs) > 0 || len(item.DefaultPorts.Outputs) > 0 {
		spec.DefaultPorts = item.DefaultPorts
	}
	if item.Defaults != nil {
		spec.Defaults = item.Defaults
	}
	if len(item.RequiredArgs) > 0 {
		spec.RequiredArgs = append([]string{}, item.RequiredArgs...)
	}
	if len(item.ArgConflicts) > 0 {
		spec.ArgConflicts = cloneStringMatrix(item.ArgConflicts)
	}
	if len(item.Outputs) > 0 {
		spec.Outputs = append([]OutputSpec{}, item.Outputs...)
	}
	if len(item.Examples) > 0 {
		spec.Examples = cloneActionExamples(item.Examples)
	}
	if len(item.InputExamples) > 0 {
		spec.InputExamples = cloneActionIOExamples(item.InputExamples)
	}
	if len(item.OutputExamples) > 0 {
		spec.OutputExamples = cloneActionIOExamples(item.OutputExamples)
	}
	if item.Experimental != nil {
		spec.Experimental = *item.Experimental
	}
	if item.Deprecated != nil {
		spec.Deprecated = *item.Deprecated
	}
	return spec, nil
}

func builtinActionSpecTemplate(action string) (ActionSpec, bool) {
	for _, spec := range RunnerCoreActionTemplates() {
		if spec.Action == action {
			return cloneActionSpec(spec), true
		}
	}
	return ActionSpec{}, false
}

func firstRawMessage(items ...json.RawMessage) json.RawMessage {
	for _, item := range items {
		if len(item) == 0 || string(item) == "null" {
			continue
		}
		return append(json.RawMessage{}, item...)
	}
	return nil
}

func defaultRunnerCoreManifestPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return filepath.Join("plugins", "builtin", "runner-core", ".codex-plugin", "plugin.json")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "plugins", "builtin", "runner-core", ".codex-plugin", "plugin.json"))
}
