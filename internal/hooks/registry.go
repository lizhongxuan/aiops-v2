package hooks

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"aiops-v2/internal/settings"
	"aiops-v2/internal/tooling"
)

type Stage string

const (
	StagePreToolUse         Stage = "pre_tool_use"
	StagePostToolUse        Stage = "post_tool_use"
	StagePreTurn            Stage = "pre_turn"
	StagePostTurn           Stage = "post_turn"
	StageUserPromptSubmit   Stage = "user_prompt_submit"
	StageSessionStart       Stage = "session_start"
	StageSetup              Stage = "setup"
	StageSubagentStart      Stage = "subagent_start"
	StagePostToolUseFailure Stage = "post_tool_use_failure"
	StagePermissionDenied   Stage = "permission_denied"
	StageNotification       Stage = "notification"
	StagePermissionRequest  Stage = "permission_request"
	StageElicitation        Stage = "elicitation"
	StageElicitationResult  Stage = "elicitation_result"
	StageCwdChanged         Stage = "cwd_changed"
	StageFileChanged        Stage = "file_changed"
	StageWorktreeCreate     Stage = "worktree_create"
)

type ToolEvent struct {
	ToolCallID           string
	SessionID            string
	TurnID               string
	SessionType          string
	Mode                 string
	Tool                 tooling.ToolMetadata
	Arguments            json.RawMessage
	Result               *tooling.ToolResult
	Err                  error
	UpdatedInput         json.RawMessage
	AdditionalContext    []string
	UpdatedMCPToolOutput *tooling.ToolResult
	UpdatedPermissions   *tooling.PermissionDecision
	WatchPaths           []string
	HideTools            []string
}

type TurnEvent struct {
	SessionID         string
	TurnID            string
	SessionType       string
	Mode              string
	Input             string
	Output            string
	Err               error
	UpdatedInput      string
	AdditionalContext []string
	WatchPaths        []string
}

type ToolMatcher struct {
	ToolNames     []string
	Sources       []tooling.ToolSource
	Modes         []string
	SessionTypes  []string
	InputContains []string
}

func (m ToolMatcher) Matches(event ToolEvent) bool {
	if len(m.ToolNames) > 0 && !matchToolName(m.ToolNames, event.Tool) {
		return false
	}
	if len(m.Sources) > 0 && !matchSource(m.Sources, event.Tool) {
		return false
	}
	if len(m.Modes) > 0 && !matchString(m.Modes, event.Mode) {
		return false
	}
	if len(m.SessionTypes) > 0 && !matchString(m.SessionTypes, event.SessionType) {
		return false
	}
	if len(m.InputContains) > 0 && !matchInputContains(m.InputContains, event.Arguments) {
		return false
	}
	return true
}

type ToolHook func(context.Context, *ToolEvent) error
type TurnHook func(context.Context, *TurnEvent) error

type ToolRegistration struct {
	Name    string
	Source  string
	Stage   Stage
	Matcher ToolMatcher
	Hook    ToolHook
}

type TurnRegistration struct {
	Name   string
	Source string
	Stage  Stage
	Hook   TurnHook
}

type Registry struct {
	mu         sync.RWMutex
	governance *settings.Governance
	tools      []ToolRegistration
	turns      []TurnRegistration
}

func NewRegistry() *Registry {
	return &Registry{}
}

// SetGovernance attaches a live governance snapshot source to the registry.
func (r *Registry) SetGovernance(governance *settings.Governance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governance = governance
}

func (r *Registry) RegisterTool(reg ToolRegistration) error {
	reg.Source = normalizeRegistrationSource(reg.Source)
	if err := r.validateToolSource(reg); err != nil {
		return err
	}
	if err := validateToolRegistration(reg); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools = append(r.tools, reg)
	return nil
}

func (r *Registry) RegisterTurn(reg TurnRegistration) error {
	reg.Source = normalizeRegistrationSource(reg.Source)
	if err := r.validateTurnSource(reg); err != nil {
		return err
	}
	if err := validateTurnRegistration(reg); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.turns = append(r.turns, reg)
	return nil
}

func (r *Registry) RunToolStage(ctx context.Context, stage Stage, event *ToolEvent) error {
	if r == nil {
		return nil
	}
	if event == nil {
		event = &ToolEvent{}
	}
	r.mu.RLock()
	hooks := append([]ToolRegistration(nil), r.tools...)
	r.mu.RUnlock()
	for _, reg := range hooks {
		if reg.Stage != stage || !reg.Matcher.Matches(*event) {
			continue
		}
		if err := reg.Hook(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

func (r *Registry) RunTurnStage(ctx context.Context, stage Stage, event *TurnEvent) error {
	if r == nil {
		return nil
	}
	if event == nil {
		event = &TurnEvent{}
	}
	r.mu.RLock()
	hooks := append([]TurnRegistration(nil), r.turns...)
	r.mu.RUnlock()
	for _, reg := range hooks {
		if reg.Stage != stage {
			continue
		}
		if err := reg.Hook(ctx, event); err != nil {
			return err
		}
	}
	return nil
}

// ListTool returns all registered tool hooks.
func (r *Registry) ListTool() []ToolRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]ToolRegistration(nil), r.tools...)
}

// ListTurn returns all registered turn hooks.
func (r *Registry) ListTurn() []TurnRegistration {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return append([]TurnRegistration(nil), r.turns...)
}

// UnregisterTool removes all tool hook registrations matching the provided name.
func (r *Registry) UnregisterTool(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := r.tools[:0]
	for _, reg := range r.tools {
		if reg.Name == name {
			continue
		}
		filtered = append(filtered, reg)
	}
	r.tools = filtered
}

// UnregisterTurn removes all turn hook registrations matching the provided name.
func (r *Registry) UnregisterTurn(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := r.turns[:0]
	for _, reg := range r.turns {
		if reg.Name == name {
			continue
		}
		filtered = append(filtered, reg)
	}
	r.turns = filtered
}

func validateToolRegistration(reg ToolRegistration) error {
	if strings.TrimSpace(reg.Name) == "" {
		return fmt.Errorf("tool registration name is required")
	}
	if strings.TrimSpace(string(reg.Stage)) == "" {
		return fmt.Errorf("tool registration stage is required")
	}
	if reg.Hook == nil {
		return fmt.Errorf("tool registration hook is required")
	}
	return nil
}

func validateTurnRegistration(reg TurnRegistration) error {
	if strings.TrimSpace(reg.Name) == "" {
		return fmt.Errorf("turn registration name is required")
	}
	if strings.TrimSpace(string(reg.Stage)) == "" {
		return fmt.Errorf("turn registration stage is required")
	}
	if reg.Hook == nil {
		return fmt.Errorf("turn registration hook is required")
	}
	return nil
}

func normalizeRegistrationSource(source string) string {
	source = strings.TrimSpace(source)
	if source == "" {
		return "builtin"
	}
	return source
}

func (r *Registry) validateToolSource(reg ToolRegistration) error {
	r.mu.RLock()
	governance := r.governance
	r.mu.RUnlock()

	if governance == nil {
		return nil
	}
	if governance.Snapshot().AllowsSource(settings.SurfaceHooks, reg.Source) {
		return nil
	}
	return fmt.Errorf("tool registration %q blocked by strictPluginOnlyCustomization for hooks", reg.Name)
}

func (r *Registry) validateTurnSource(reg TurnRegistration) error {
	r.mu.RLock()
	governance := r.governance
	r.mu.RUnlock()

	if governance == nil {
		return nil
	}
	if governance.Snapshot().AllowsSource(settings.SurfaceHooks, reg.Source) {
		return nil
	}
	return fmt.Errorf("turn registration %q blocked by strictPluginOnlyCustomization for hooks", reg.Name)
}

func matchToolName(expected []string, meta tooling.ToolMetadata) bool {
	if matchString(expected, meta.Name) {
		return true
	}
	for _, alias := range meta.Aliases {
		if matchString(expected, alias) {
			return true
		}
	}
	return false
}

func matchSource(expected []tooling.ToolSource, actual tooling.ToolMetadata) bool {
	for _, source := range expected {
		if actual.HasSource(source) {
			return true
		}
	}
	return false
}

func matchString(expected []string, actual string) bool {
	actual = strings.TrimSpace(actual)
	for _, candidate := range expected {
		if strings.TrimSpace(candidate) == actual {
			return true
		}
	}
	return false
}

func matchInputContains(expected []string, input json.RawMessage) bool {
	s := string(input)
	for _, needle := range expected {
		if strings.Contains(s, needle) {
			return true
		}
	}
	return false
}
