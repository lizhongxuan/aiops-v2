package skills

import (
	"path/filepath"
	"strings"
	"sync"

	"aiops-v2/internal/commands"
)

var skillSourcePrecedence = []string{
	commands.SourceBuiltin,
	commands.SourceBundled,
	commands.SourcePlugin,
	commands.SourceUserSettings,
	commands.SourceProjectSettings,
	commands.SourceLocalSettings,
	commands.SourceFlagSettings,
	commands.SourcePolicySettings,
	commands.SourceMCP,
}

// Definition describes a skill definition and its prompt asset.
type Definition struct {
	Name        string
	Description string
	Prompt      string
	Tools       []string
	Source      string
	LoadedFrom  string
	FileID      string
}

type skillRecord struct {
	def   Definition
	order int
}

// Registry stores skill definitions as a source-aware catalog.
type Registry struct {
	mu     sync.RWMutex
	items  []skillRecord
	nextID int
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// Register stores a definition unless it is a duplicate of an existing file-backed skill.
func (r *Registry) Register(def Definition) {
	r.RegisterWithStatus(def)
}

// RegisterWithStatus stores a definition and reports whether it was added.
func (r *Registry) RegisterWithStatus(def Definition) bool {
	def = normalizeDefinition(def)
	if def.Name == "" {
		return false
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if hasDuplicateSkillFile(r.items, def) {
		return false
	}

	r.items = append(r.items, skillRecord{
		def:   cloneDefinition(def),
		order: r.nextID,
	})
	r.nextID++
	return true
}

// RegisterBatch stores multiple definitions.
func (r *Registry) RegisterBatch(defs []Definition) {
	for _, def := range defs {
		r.RegisterWithStatus(def)
	}
}

// Get returns the active definition for a skill name.
func (r *Registry) Get(name string) (Definition, bool) {
	name = strings.TrimSpace(name)

	r.mu.RLock()
	defer r.mu.RUnlock()

	var (
		best  skillRecord
		found bool
	)
	for _, rec := range r.items {
		if rec.def.Name != name {
			continue
		}
		if !found || compareSkillRecords(rec, best) < 0 {
			best = rec
			found = true
		}
	}
	if !found {
		return Definition{}, false
	}
	return cloneDefinition(best.def), true
}

// List returns all registered definitions in registration order.
func (r *Registry) List() []Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]Definition, 0, len(r.items))
	for _, rec := range r.items {
		out = append(out, cloneDefinition(rec.def))
	}
	return out
}

// Unregister removes all definitions for a skill name.
func (r *Registry) Unregister(name string) {
	name = strings.TrimSpace(name)

	r.mu.Lock()
	defer r.mu.Unlock()

	filtered := r.items[:0]
	for _, rec := range r.items {
		if rec.def.Name == name {
			continue
		}
		filtered = append(filtered, rec)
	}
	r.items = filtered
}

// UnregisterDefinition removes a previously registered definition that matches exactly.
func (r *Registry) UnregisterDefinition(def Definition) bool {
	def = normalizeDefinition(def)

	r.mu.Lock()
	defer r.mu.Unlock()

	for i, rec := range r.items {
		if !sameDefinition(rec.def, def) {
			continue
		}
		r.items = append(r.items[:i], r.items[i+1:]...)
		return true
	}
	return false
}

// promptCommands projects skill definitions onto the command surface that SkillTool consumes.
// It is intentionally package-private so runtime callers go through commands.Registry.
func (r *Registry) promptCommands(defaultSource string) []commands.PromptCommand {
	defs := r.List()
	out := make([]commands.PromptCommand, 0, len(defs))
	for _, def := range defs {
		out = append(out, PromptCommandForDefinition(def, defaultSource))
	}
	return out
}

// PromptCommandForDefinition projects a single skill definition onto the command surface.
func PromptCommandForDefinition(def Definition, defaultSource string) commands.PromptCommand {
	def = normalizeDefinition(def)
	source := normalizeSkillSource(def.Source)
	if source == "" {
		source = normalizeSkillSource(defaultSource)
	}
	if source == "" {
		source = commands.SourceProjectSettings
	}
	return commands.PromptCommand{
		Name:        def.Name,
		Description: def.Description,
		Prompt:      def.Prompt,
		Tools:       append([]string(nil), def.Tools...),
		Source:      source,
		LoadedFrom:  inferCommandLoadedFrom(def.LoadedFrom, source),
		WhenToUse:   def.Description,
	}
}

func normalizeDefinition(def Definition) Definition {
	def.Name = strings.TrimSpace(def.Name)
	def.Description = strings.TrimSpace(def.Description)
	def.Prompt = strings.TrimSpace(def.Prompt)
	def.Source = strings.TrimSpace(def.Source)
	def.LoadedFrom = normalizeLoadedFrom(def.LoadedFrom)
	def.FileID = strings.TrimSpace(def.FileID)
	def.Tools = append([]string(nil), def.Tools...)
	for i := range def.Tools {
		def.Tools[i] = strings.TrimSpace(def.Tools[i])
	}

	if def.LoadedFrom == "" && looksLikePath(def.Source) {
		def.LoadedFrom = normalizeLoadedFrom(def.Source)
		def.Source = ""
	}

	if def.Source == "" {
		def.Source = inferSkillSource(def.LoadedFrom)
	}
	def.Source = normalizeSkillSource(def.Source)

	if def.LoadedFrom == "" {
		switch def.Source {
		case commands.SourcePlugin:
			def.LoadedFrom = commands.LoadedFromPlugin
		case commands.SourceBundled:
			def.LoadedFrom = commands.LoadedFromBundled
		case commands.SourcePolicySettings:
			def.LoadedFrom = commands.LoadedFromManaged
		case commands.SourceMCP:
			def.LoadedFrom = commands.LoadedFromMCP
		default:
			def.LoadedFrom = commands.LoadedFromSkills
		}
	}
	if def.FileID == "" && looksLikePath(def.LoadedFrom) {
		def.FileID = ResolveFileIdentity(def.LoadedFrom)
	}
	return def
}

func compareSkillRecords(left, right skillRecord) int {
	if sourceDiff := skillSourcePriority(left.def.Source) - skillSourcePriority(right.def.Source); sourceDiff != 0 {
		return sourceDiff
	}
	if loadedFromDiff := loadedFromPriority(left.def.LoadedFrom) - loadedFromPriority(right.def.LoadedFrom); loadedFromDiff != 0 {
		return loadedFromDiff
	}
	return left.order - right.order
}

func hasDuplicateSkillFile(records []skillRecord, def Definition) bool {
	identity := skillIdentity(def)
	if identity == "" {
		return false
	}
	for _, rec := range records {
		if skillIdentity(rec.def) == identity {
			return true
		}
	}
	return false
}

func skillIdentity(def Definition) string {
	if def.FileID != "" {
		return "file:" + def.FileID
	}
	if looksLikePath(def.LoadedFrom) {
		return "path:" + normalizeLoadedFrom(def.LoadedFrom)
	}
	return ""
}

func sameDefinition(left, right Definition) bool {
	if left.Name != right.Name ||
		left.Description != right.Description ||
		left.Prompt != right.Prompt ||
		left.Source != right.Source ||
		left.LoadedFrom != right.LoadedFrom ||
		left.FileID != right.FileID ||
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

func cloneDefinition(def Definition) Definition {
	def.Tools = append([]string(nil), def.Tools...)
	return def
}

func inferSkillSource(loadedFrom string) string {
	switch strings.TrimSpace(loadedFrom) {
	case commands.LoadedFromPlugin:
		return commands.SourcePlugin
	case commands.LoadedFromBundled:
		return commands.SourceBundled
	case commands.LoadedFromManaged:
		return commands.SourcePolicySettings
	case commands.LoadedFromMCP:
		return commands.SourceMCP
	}

	normalized := filepath.ToSlash(strings.TrimSpace(loadedFrom))
	switch {
	case normalized == "":
		return ""
	case strings.Contains(normalized, "/.codex/plugins/"),
		strings.Contains(normalized, "/plugins/cache/"):
		return commands.SourcePlugin
	case strings.Contains(normalized, "/.codex/skills/.system/"),
		strings.Contains(normalized, "/skills/.system/"):
		return commands.SourceBundled
	case strings.Contains(normalized, "/.codex/skills/"):
		return commands.SourceUserSettings
	case looksLikePath(normalized):
		return commands.SourceProjectSettings
	default:
		return ""
	}
}

func normalizeSkillSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "":
		return ""
	case "skill", "repo", "repo-skill":
		return commands.SourceProjectSettings
	case "builtin", "built-in", strings.ToLower(commands.SourceBuiltin):
		return commands.SourceBuiltin
	case "plugin", "plugin-skill", strings.ToLower(commands.SourcePlugin):
		return commands.SourcePlugin
	case "bundled", "bundled-skill", strings.ToLower(commands.SourceBundled):
		return commands.SourceBundled
	case strings.ToLower(commands.SourceUserSettings):
		return commands.SourceUserSettings
	case strings.ToLower(commands.SourceProjectSettings):
		return commands.SourceProjectSettings
	case strings.ToLower(commands.SourceLocalSettings):
		return commands.SourceLocalSettings
	case strings.ToLower(commands.SourceFlagSettings):
		return commands.SourceFlagSettings
	case strings.ToLower(commands.SourcePolicySettings):
		return commands.SourcePolicySettings
	case strings.ToLower(commands.SourceMCP):
		return commands.SourceMCP
	default:
		return strings.TrimSpace(source)
	}
}

func skillSourcePriority(source string) int {
	source = normalizeSkillSource(source)
	for i, candidate := range skillSourcePrecedence {
		if candidate == source {
			return i
		}
	}
	return len(skillSourcePrecedence)
}

func inferCommandLoadedFrom(loadedFrom, source string) string {
	normalized := normalizeLoadedFrom(loadedFrom)
	switch normalized {
	case commands.LoadedFromPlugin,
		commands.LoadedFromBundled,
		commands.LoadedFromManaged,
		commands.LoadedFromMCP,
		commands.LoadedFromSkills,
		commands.LoadedFromCommandsDeprecated:
		return normalized
	}
	switch source {
	case commands.SourcePlugin:
		return commands.LoadedFromPlugin
	case commands.SourceBundled:
		return commands.LoadedFromBundled
	case commands.SourcePolicySettings:
		return commands.LoadedFromManaged
	case commands.SourceMCP:
		return commands.LoadedFromMCP
	default:
		return commands.LoadedFromSkills
	}
}

func normalizeLoadedFrom(loadedFrom string) string {
	loadedFrom = strings.TrimSpace(loadedFrom)
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
	switch strings.TrimSpace(loadedFrom) {
	case commands.LoadedFromManaged:
		return 0
	case commands.LoadedFromBundled:
		return 1
	case commands.LoadedFromPlugin:
		return 2
	case commands.LoadedFromSkills:
		return 3
	case commands.LoadedFromCommandsDeprecated:
		return 4
	case commands.LoadedFromMCP:
		return 5
	default:
		if looksLikePath(loadedFrom) {
			return 6
		}
		return 7
	}
}

func looksLikePath(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && (strings.ContainsRune(value, filepath.Separator) ||
		strings.Contains(value, "/") ||
		strings.HasSuffix(strings.ToUpper(value), ".MD"))
}
