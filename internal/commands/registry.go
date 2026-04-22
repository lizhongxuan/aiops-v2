package commands

import (
	"fmt"
	"path/filepath"
	"strings"
	"sync"

	"aiops-v2/internal/settings"
)

const (
	SourceBuiltin         = "builtin"
	SourceMCP             = "mcp"
	SourcePlugin          = "plugin"
	SourceBundled         = "bundled"
	SourceUserSettings    = "userSettings"
	SourceProjectSettings = "projectSettings"
	SourceLocalSettings   = "localSettings"
	SourceFlagSettings    = "flagSettings"
	SourcePolicySettings  = "policySettings"
)

const (
	LoadedFromCommandsDeprecated = "commands_DEPRECATED"
	LoadedFromSkills             = "skills"
	LoadedFromPlugin             = "plugin"
	LoadedFromManaged            = "managed"
	LoadedFromBundled            = "bundled"
	LoadedFromMCP                = "mcp"
)

var promptSourcePrecedence = []string{
	SourceBuiltin,
	SourceBundled,
	SourcePlugin,
	SourceUserSettings,
	SourceProjectSettings,
	SourceLocalSettings,
	SourceFlagSettings,
	SourcePolicySettings,
	SourceMCP,
}

// PromptCommand describes a prompt or slash-style command that can be surfaced to the model.
type PromptCommand struct {
	Name        string
	Description string
	Prompt      string
	Tools       []string
	Source      string
	LoadedFrom  string
	WhenToUse   string
}

// LocalCommand is intentionally the same shape as PromptCommand for the minimal registry skeleton.
type LocalCommand = PromptCommand

type promptRecord struct {
	cmd   PromptCommand
	order int
}

type localRecord struct {
	cmd   LocalCommand
	order int
}

// CommandRegistry stores prompt and local commands separately.
type CommandRegistry struct {
	mu          sync.RWMutex
	governance  *settings.Governance
	prompts     map[string][]promptRecord
	promptOrder []string
	locals      map[string][]localRecord
	localOrder  []string
	nextOrder   int
}

// NewRegistry creates an empty command registry.
func NewRegistry() *CommandRegistry {
	return &CommandRegistry{
		prompts: make(map[string][]promptRecord),
		locals:  make(map[string][]localRecord),
	}
}

// SetGovernance attaches a live governance snapshot source to the registry.
func (r *CommandRegistry) SetGovernance(governance *settings.Governance) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.governance = governance
}

// RegisterPrompt stores a prompt command record.
func (r *CommandRegistry) RegisterPrompt(cmd PromptCommand) error {
	cmd = normalizePromptCommand(cmd)
	if cmd.Name == "" {
		return fmt.Errorf("commands: prompt command name is required")
	}
	if err := r.validatePromptCommand(cmd); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.prompts[cmd.Name]; !exists {
		r.promptOrder = append(r.promptOrder, cmd.Name)
	}
	r.prompts[cmd.Name] = append(r.prompts[cmd.Name], promptRecord{
		cmd:   clonePromptCommand(cmd),
		order: r.nextOrder,
	})
	r.nextOrder++
	return nil
}

// RegisterLocal stores a local command record.
func (r *CommandRegistry) RegisterLocal(cmd LocalCommand) error {
	cmd = normalizePromptCommand(cmd)
	if cmd.Name == "" {
		return fmt.Errorf("commands: local command name is required")
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.locals[cmd.Name]; !exists {
		r.localOrder = append(r.localOrder, cmd.Name)
	}
	r.locals[cmd.Name] = append(r.locals[cmd.Name], localRecord{
		cmd:   clonePromptCommand(cmd),
		order: r.nextOrder,
	})
	r.nextOrder++
	return nil
}

// GetPrompt returns the active prompt command for a name.
func (r *CommandRegistry) GetPrompt(name string) (PromptCommand, bool) {
	name = strings.TrimSpace(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	rec, ok := activePromptRecord(r.prompts[name])
	if !ok {
		return PromptCommand{}, false
	}
	return clonePromptCommand(rec.cmd), true
}

// ListPrompt returns the active prompt view in first-registration name order.
func (r *CommandRegistry) ListPrompt() []PromptCommand {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]PromptCommand, 0, len(r.promptOrder))
	for _, name := range r.promptOrder {
		rec, ok := activePromptRecord(r.prompts[name])
		if !ok {
			continue
		}
		out = append(out, clonePromptCommand(rec.cmd))
	}
	return out
}

// UnregisterPrompt removes all prompt command records for a name.
func (r *CommandRegistry) UnregisterPrompt(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.prompts, name)
	r.promptOrder = filterName(r.promptOrder, name)
}

// UnregisterPromptRecord removes one prompt command record that matches exactly.
func (r *CommandRegistry) UnregisterPromptRecord(cmd PromptCommand) bool {
	cmd = normalizePromptCommand(cmd)

	r.mu.Lock()
	defer r.mu.Unlock()

	records := r.prompts[cmd.Name]
	for i, rec := range records {
		if !samePromptCommand(rec.cmd, cmd) {
			continue
		}
		records = append(records[:i], records[i+1:]...)
		if len(records) == 0 {
			delete(r.prompts, cmd.Name)
			r.promptOrder = filterName(r.promptOrder, cmd.Name)
		} else {
			r.prompts[cmd.Name] = records
		}
		return true
	}
	return false
}

// UnregisterLocal removes all local command records for a name.
func (r *CommandRegistry) UnregisterLocal(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	delete(r.locals, name)
	r.localOrder = filterName(r.localOrder, name)
}

// UnregisterLocalRecord removes one local command record that matches exactly.
func (r *CommandRegistry) UnregisterLocalRecord(cmd LocalCommand) bool {
	cmd = normalizePromptCommand(cmd)

	r.mu.Lock()
	defer r.mu.Unlock()

	records := r.locals[cmd.Name]
	for i, rec := range records {
		if !samePromptCommand(rec.cmd, cmd) {
			continue
		}
		records = append(records[:i], records[i+1:]...)
		if len(records) == 0 {
			delete(r.locals, cmd.Name)
			r.localOrder = filterName(r.localOrder, cmd.Name)
		} else {
			r.locals[cmd.Name] = records
		}
		return true
	}
	return false
}

// ListSkillLikePromptCommands returns the prompt commands that should be surfaced to SkillTool.
func (r *CommandRegistry) ListSkillLikePromptCommands() []PromptCommand {
	prompts := r.ListPrompt()
	out := make([]PromptCommand, 0, len(prompts))
	for _, cmd := range prompts {
		if cmd.IsSkillLike() {
			out = append(out, cmd)
		}
	}
	return out
}

// RegisterPromptBatch stores multiple prompt commands.
func (r *CommandRegistry) RegisterPromptBatch(cmds []PromptCommand) error {
	for _, cmd := range cmds {
		if err := r.RegisterPrompt(cmd); err != nil {
			return err
		}
	}
	return nil
}

// IsSkillLike reports whether the command should be surfaced through the skill command surface.
func (c PromptCommand) IsSkillLike() bool {
	if isSkillLikeSource(c.Source) {
		return true
	}
	switch normalizeLoadedFrom(c.LoadedFrom) {
	case LoadedFromSkills,
		LoadedFromCommandsDeprecated,
		LoadedFromPlugin,
		LoadedFromManaged,
		LoadedFromBundled,
		LoadedFromMCP:
		return true
	default:
		return false
	}
}

func normalizePromptCommand(cmd PromptCommand) PromptCommand {
	cmd.Name = strings.TrimSpace(cmd.Name)
	cmd.Description = strings.TrimSpace(cmd.Description)
	cmd.Prompt = strings.TrimSpace(cmd.Prompt)
	cmd.Source = normalizeCommandSource(cmd.Source)
	cmd.LoadedFrom = normalizeLoadedFrom(cmd.LoadedFrom)
	cmd.WhenToUse = strings.TrimSpace(cmd.WhenToUse)
	cmd.Tools = append([]string(nil), cmd.Tools...)
	for i := range cmd.Tools {
		cmd.Tools[i] = strings.TrimSpace(cmd.Tools[i])
	}
	return cmd
}

func clonePromptCommand(cmd PromptCommand) PromptCommand {
	cmd.Tools = append([]string(nil), cmd.Tools...)
	return cmd
}

func activePromptRecord(records []promptRecord) (promptRecord, bool) {
	if len(records) == 0 {
		return promptRecord{}, false
	}
	best := records[0]
	for _, rec := range records[1:] {
		if comparePromptRecords(rec, best) < 0 {
			best = rec
		}
	}
	return best, true
}

func comparePromptRecords(left, right promptRecord) int {
	if sourceDiff := sourcePriority(left.cmd.Source) - sourcePriority(right.cmd.Source); sourceDiff != 0 {
		return sourceDiff
	}
	if loadedFromDiff := loadedFromPriority(left.cmd.LoadedFrom) - loadedFromPriority(right.cmd.LoadedFrom); loadedFromDiff != 0 {
		return loadedFromDiff
	}
	return left.order - right.order
}

func samePromptCommand(left, right PromptCommand) bool {
	if left.Name != right.Name ||
		left.Description != right.Description ||
		left.Prompt != right.Prompt ||
		left.Source != right.Source ||
		left.LoadedFrom != right.LoadedFrom ||
		left.WhenToUse != right.WhenToUse ||
		len(left.Tools) != len(right.Tools) {
		return false
	}
	for i := range left.Tools {
		if left.Tools[i] != right.Tools[i] {
			return false
		}
	}
	return true
}

func filterName(items []string, name string) []string {
	filtered := items[:0]
	for _, candidate := range items {
		if candidate == name {
			continue
		}
		filtered = append(filtered, candidate)
	}
	return filtered
}

func isSkillLikeSource(source string) bool {
	switch normalizeCommandSource(source) {
	case SourcePlugin,
		SourceBundled,
		SourceUserSettings,
		SourceProjectSettings,
		SourceLocalSettings,
		SourcePolicySettings:
		return true
	default:
		return false
	}
}

func (r *CommandRegistry) validatePromptCommand(cmd PromptCommand) error {
	r.mu.RLock()
	governance := r.governance
	r.mu.RUnlock()

	if governance == nil || !cmd.IsSkillLike() {
		return nil
	}
	if governance.Snapshot().AllowsSource(settings.SurfaceSkills, cmd.Source) {
		return nil
	}
	return fmt.Errorf("commands: prompt command %q blocked by strictPluginOnlyCustomization for skills", cmd.Name)
}

func normalizeCommandSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "":
		return ""
	case "skill", "repo", "repo-skill":
		return SourceProjectSettings
	case "builtin", "built-in", strings.ToLower(SourceBuiltin):
		return SourceBuiltin
	case "plugin", "plugin-skill", strings.ToLower(SourcePlugin):
		return SourcePlugin
	case "bundled", "bundled-skill", strings.ToLower(SourceBundled):
		return SourceBundled
	case strings.ToLower(SourceUserSettings):
		return SourceUserSettings
	case strings.ToLower(SourceProjectSettings):
		return SourceProjectSettings
	case strings.ToLower(SourceLocalSettings):
		return SourceLocalSettings
	case strings.ToLower(SourceFlagSettings):
		return SourceFlagSettings
	case strings.ToLower(SourcePolicySettings):
		return SourcePolicySettings
	case strings.ToLower(SourceMCP):
		return SourceMCP
	default:
		return strings.TrimSpace(source)
	}
}

func sourcePriority(source string) int {
	source = normalizeCommandSource(source)
	for i, candidate := range promptSourcePrecedence {
		if candidate == source {
			return i
		}
	}
	return len(promptSourcePrecedence)
}

func normalizeLoadedFrom(loadedFrom string) string {
	loadedFrom = strings.TrimSpace(loadedFrom)
	switch loadedFrom {
	case LoadedFromCommandsDeprecated,
		LoadedFromSkills,
		LoadedFromPlugin,
		LoadedFromManaged,
		LoadedFromBundled,
		LoadedFromMCP:
		return loadedFrom
	}
	if loadedFrom == "" {
		return ""
	}
	if !looksLikePath(loadedFrom) {
		return loadedFrom
	}
	if abs, err := filepath.Abs(loadedFrom); err == nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(loadedFrom)
}

func loadedFromPriority(loadedFrom string) int {
	switch normalizeLoadedFrom(loadedFrom) {
	case LoadedFromManaged:
		return 0
	case LoadedFromBundled:
		return 1
	case LoadedFromPlugin:
		return 2
	case LoadedFromSkills:
		return 3
	case LoadedFromCommandsDeprecated:
		return 4
	case LoadedFromMCP:
		return 5
	default:
		if loadedFrom != "" {
			return 6
		}
		return 7
	}
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && (strings.ContainsRune(value, filepath.Separator) ||
		strings.Contains(value, "/") ||
		strings.HasSuffix(strings.ToUpper(value), ".JSON") ||
		strings.HasSuffix(strings.ToUpper(value), ".MD"))
}
