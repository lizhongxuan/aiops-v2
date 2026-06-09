package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"path/filepath"
	"sort"
	"strings"
)

const (
	MaxSkillDescriptionChars = 240
	MaxSkillWhenToUseChars   = 360
	MaxSkillPreviewChars     = 600
	MaxSkillIndexChars       = 6000
	MaxSkillIndexContextPct  = 1
)

// SkillDiscoveryMetadata is the compact, prompt-safe index metadata for a skill.
// It must never require loading the full SKILL.md body.
type SkillDiscoveryMetadata struct {
	WhenToUse        string   `json:"whenToUse,omitempty"`
	Preview          string   `json:"preview,omitempty"`
	ResourceTypes    []string `json:"resourceTypes,omitempty"`
	TaskIntents      []string `json:"taskIntents,omitempty"`
	Paths            []string `json:"paths,omitempty"`
	Modes            []string `json:"modes,omitempty"`
	ActivationMode   string   `json:"activationMode,omitempty"`
	UserInvocable    bool     `json:"userInvocable,omitempty"`
	ModelInvocable   bool     `json:"modelInvocable,omitempty"`
	RequiredForMatch bool     `json:"requiredForMatch,omitempty"`
}

// SkillGovernanceMetadata carries generic safety constraints contributed by a skill.
type SkillGovernanceMetadata struct {
	Risk         string   `json:"risk,omitempty"`
	AllowedTools []string `json:"allowedTools,omitempty"`
	DeniedTools  []string `json:"deniedTools,omitempty"`
}

// SkillTruncationState reports compact-index truncation performed during load.
type SkillTruncationState struct {
	Description bool `json:"description,omitempty"`
	WhenToUse   bool `json:"whenToUse,omitempty"`
	Preview     bool `json:"preview,omitempty"`
}

type SkillIndexOptions struct {
	Query            string
	ResourceURI      string
	Mode             string
	MaxChars         int
	MaxContextPct    int
	MaxContextTokens int
}

type SkillIndexResult struct {
	Entries []SkillIndexEntry
	Dropped []DroppedSkillIndexEntry
	Bytes   int
	Hash    string
}

type SkillIndexEntry struct {
	Name             string   `json:"name"`
	Description      string   `json:"description,omitempty"`
	WhenToUse        string   `json:"whenToUse,omitempty"`
	Preview          string   `json:"preview,omitempty"`
	ResourceTypes    []string `json:"resourceTypes,omitempty"`
	TaskIntents      []string `json:"taskIntents,omitempty"`
	Paths            []string `json:"paths,omitempty"`
	Modes            []string `json:"modes,omitempty"`
	Source           string   `json:"source,omitempty"`
	LoadedFrom       string   `json:"loadedFrom,omitempty"`
	Risk             string   `json:"risk,omitempty"`
	RequiredForMatch bool     `json:"requiredForMatch,omitempty"`
	Score            int      `json:"score,omitempty"`
}

type DroppedSkillIndexEntry struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type scoredSkillIndexEntry struct {
	entry SkillIndexEntry
	order int
}

func BuildSkillIndex(defs []Definition, opts SkillIndexOptions) SkillIndexResult {
	maxChars := opts.MaxChars
	if maxChars <= 0 {
		maxChars = MaxSkillIndexChars
	}
	if opts.MaxContextPct > 0 && opts.MaxContextTokens > 0 {
		tokenBudgetChars := opts.MaxContextTokens * 4 * opts.MaxContextPct / 100
		if tokenBudgetChars > 0 && tokenBudgetChars < maxChars {
			maxChars = tokenBudgetChars
		}
	}

	scored := make([]scoredSkillIndexEntry, 0, len(defs))
	dropped := make([]DroppedSkillIndexEntry, 0)
	for i, def := range defs {
		def = normalizeDefinition(def)
		if def.Name == "" {
			continue
		}
		if !skillModelInvocable(def.Discovery) {
			dropped = append(dropped, DroppedSkillIndexEntry{Name: def.Name, Reason: "model_disabled"})
			continue
		}
		if opts.Mode != "" && len(def.Discovery.Modes) > 0 && !stringListContainsFold(def.Discovery.Modes, opts.Mode) {
			dropped = append(dropped, DroppedSkillIndexEntry{Name: def.Name, Reason: "mode_mismatch"})
			continue
		}
		if opts.ResourceURI != "" && len(def.Discovery.Paths) > 0 && !matchesAnySkillPath(def.Discovery.Paths, opts.ResourceURI) {
			dropped = append(dropped, DroppedSkillIndexEntry{Name: def.Name, Reason: "path_mismatch"})
			continue
		}
		entry := skillIndexEntryForDefinition(def)
		entry.Score = scoreSkillIndexEntry(entry, opts)
		scored = append(scored, scoredSkillIndexEntry{entry: entry, order: i})
	}

	sort.SliceStable(scored, func(i, j int) bool {
		if scored[i].entry.Score != scored[j].entry.Score {
			return scored[i].entry.Score > scored[j].entry.Score
		}
		return scored[i].order < scored[j].order
	})

	entries := make([]SkillIndexEntry, 0, len(scored))
	total := 0
	for _, candidate := range scored {
		size := skillIndexEntrySize(candidate.entry)
		if total > 0 && total+size > maxChars {
			dropped = append(dropped, DroppedSkillIndexEntry{Name: candidate.entry.Name, Reason: "budget_exceeded"})
			continue
		}
		if total == 0 && size > maxChars {
			candidate.entry.Preview = ""
			candidate.entry.WhenToUse = capString(candidate.entry.WhenToUse, maxChars/3)
			candidate.entry.Description = capString(candidate.entry.Description, maxChars/3)
			size = skillIndexEntrySize(candidate.entry)
		}
		entries = append(entries, candidate.entry)
		total += size
	}

	result := SkillIndexResult{
		Entries: entries,
		Dropped: dropped,
		Bytes:   total,
	}
	result.Hash = hashSkillIndex(entries, dropped)
	return result
}

func skillIndexEntryForDefinition(def Definition) SkillIndexEntry {
	preview := strings.TrimSpace(def.Discovery.Preview)
	if preview == "" {
		preview = strings.TrimSpace(def.Discovery.WhenToUse)
	}
	return SkillIndexEntry{
		Name:             def.Name,
		Description:      def.Description,
		WhenToUse:        def.Discovery.WhenToUse,
		Preview:          preview,
		ResourceTypes:    append([]string(nil), def.Discovery.ResourceTypes...),
		TaskIntents:      append([]string(nil), def.Discovery.TaskIntents...),
		Paths:            append([]string(nil), def.Discovery.Paths...),
		Modes:            append([]string(nil), def.Discovery.Modes...),
		Source:           def.Source,
		LoadedFrom:       def.LoadedFrom,
		Risk:             def.Governance.Risk,
		RequiredForMatch: def.Discovery.RequiredForMatch,
	}
}

func skillIndexEntrySize(entry SkillIndexEntry) int {
	data, err := json.Marshal(entry)
	if err != nil {
		return len(entry.Name) + len(entry.Description) + len(entry.WhenToUse) + len(entry.Preview)
	}
	return len(data)
}

func scoreSkillIndexEntry(entry SkillIndexEntry, opts SkillIndexOptions) int {
	score := 0
	query := strings.ToLower(opts.Query)
	for _, intent := range entry.TaskIntents {
		if query != "" && strings.Contains(query, strings.ToLower(intent)) {
			score += 30
		}
	}
	for _, resourceType := range entry.ResourceTypes {
		if query != "" && strings.Contains(query, strings.ToLower(resourceType)) {
			score += 20
		}
		if opts.ResourceURI != "" && strings.Contains(strings.ToLower(opts.ResourceURI), strings.ToLower(resourceType)) {
			score += 20
		}
	}
	if opts.Mode != "" && stringListContainsFold(entry.Modes, opts.Mode) {
		score += 15
	}
	if opts.ResourceURI != "" && matchesAnySkillPath(entry.Paths, opts.ResourceURI) {
		score += 15
	}
	searchable := strings.ToLower(strings.Join([]string{entry.Name, entry.Description, entry.WhenToUse, entry.Preview}, " "))
	for _, term := range strings.Fields(query) {
		if strings.Contains(searchable, term) {
			score += 5
		}
	}
	return score
}

func matchesAnySkillPath(patterns []string, resourceURI string) bool {
	resource := filepath.ToSlash(strings.TrimSpace(resourceURI))
	resource = strings.TrimPrefix(resource, "file://")
	for _, pattern := range patterns {
		pattern = filepath.ToSlash(strings.TrimSpace(pattern))
		if pattern == "" {
			continue
		}
		if ok, _ := filepath.Match(pattern, resource); ok {
			return true
		}
		if strings.HasSuffix(pattern, "/*") && strings.HasPrefix(resource, strings.TrimSuffix(pattern, "*")) {
			return true
		}
		if strings.Contains(resource, strings.Trim(pattern, "*")) {
			return true
		}
	}
	return false
}

func skillModelInvocable(meta SkillDiscoveryMetadata) bool {
	if meta.ModelInvocable {
		return true
	}
	if isEmptySkillDiscoveryMetadata(meta) {
		return true
	}
	if meta.UserInvocable || strings.EqualFold(meta.ActivationMode, "user") || strings.EqualFold(meta.ActivationMode, "manual") {
		return false
	}
	return false
}

func isEmptySkillDiscoveryMetadata(meta SkillDiscoveryMetadata) bool {
	return meta.WhenToUse == "" &&
		meta.Preview == "" &&
		len(meta.ResourceTypes) == 0 &&
		len(meta.TaskIntents) == 0 &&
		len(meta.Paths) == 0 &&
		len(meta.Modes) == 0 &&
		meta.ActivationMode == "" &&
		!meta.UserInvocable &&
		!meta.ModelInvocable &&
		!meta.RequiredForMatch
}

func normalizeSkillDiscoveryMetadata(meta SkillDiscoveryMetadata) (SkillDiscoveryMetadata, SkillTruncationState) {
	var truncated SkillTruncationState
	meta.WhenToUse, truncated.WhenToUse = capStringWithState(meta.WhenToUse, MaxSkillWhenToUseChars)
	meta.Preview, truncated.Preview = capStringWithState(meta.Preview, MaxSkillPreviewChars)
	meta.ResourceTypes = normalizeStringSlice(meta.ResourceTypes)
	meta.TaskIntents = normalizeStringSlice(meta.TaskIntents)
	meta.Paths = normalizeStringSlice(meta.Paths)
	meta.Modes = normalizeStringSlice(meta.Modes)
	meta.ActivationMode = strings.TrimSpace(meta.ActivationMode)
	return meta, truncated
}

func normalizeSkillGovernanceMetadata(meta SkillGovernanceMetadata) SkillGovernanceMetadata {
	meta.Risk = strings.TrimSpace(meta.Risk)
	meta.AllowedTools = normalizeStringSlice(meta.AllowedTools)
	meta.DeniedTools = normalizeStringSlice(meta.DeniedTools)
	return meta
}

func capStringWithState(value string, limit int) (string, bool) {
	value = strings.TrimSpace(value)
	if limit <= 0 || len(value) <= limit {
		return value, false
	}
	return value[:limit], true
}

func capString(value string, limit int) string {
	out, _ := capStringWithState(value, limit)
	return out
}

func normalizeStringSlice(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		for _, part := range splitMetadataListValue(value) {
			trimmed := strings.TrimSpace(trimQuotes(part))
			if trimmed == "" {
				continue
			}
			key := strings.ToLower(trimmed)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, trimmed)
		}
	}
	return out
}

func splitMetadataListValue(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	if value == "" {
		return nil
	}
	return strings.Split(value, ",")
}

func stringListContainsFold(values []string, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	for _, value := range values {
		if strings.ToLower(strings.TrimSpace(value)) == target {
			return true
		}
	}
	return false
}

func hashSkillIndex(entries []SkillIndexEntry, dropped []DroppedSkillIndexEntry) string {
	data, _ := json.Marshal(struct {
		Entries []SkillIndexEntry        `json:"entries"`
		Dropped []DroppedSkillIndexEntry `json:"dropped"`
	}{Entries: entries, Dropped: dropped})
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}
