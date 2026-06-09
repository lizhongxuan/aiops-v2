package skills

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Loader reads SKILL.md files from the filesystem.
type Loader struct{}

// NewLoader creates a loader with default behavior.
func NewLoader() *Loader {
	return &Loader{}
}

// LoadDir recursively scans root for SKILL.md files and converts them into definitions.
func (l *Loader) LoadDir(root string) ([]Definition, error) {
	var defs []Definition

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if d.Name() != "SKILL.md" {
			return nil
		}

		def, err := loadSkillFile(path)
		if err != nil {
			return fmt.Errorf("load %s: %w", path, err)
		}
		defs = append(defs, def)
		return nil
	})
	if err != nil {
		return nil, err
	}

	return defs, nil
}

func loadSkillFile(path string) (Definition, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Definition{}, err
	}

	content := strings.TrimSpace(string(data))
	loadedFrom := normalizeLoadedFrom(path)
	def := Definition{
		Source:     inferSkillSource(loadedFrom),
		LoadedFrom: loadedFrom,
		FileID:     ResolveFileIdentity(loadedFrom),
	}

	meta, body := parseFrontmatter(content)
	if meta.name != "" {
		def.Name = meta.name
	}
	if meta.description != "" {
		def.Description = meta.description
	}
	if len(meta.tools) > 0 {
		def.Tools = append([]string(nil), meta.tools...)
	}
	def.Discovery = meta.discovery
	def.Governance = meta.governance

	def.Prompt = strings.TrimSpace(body)
	if def.Prompt == "" {
		def.Prompt = content
	}
	if def.Name == "" {
		def.Name = filepath.Base(filepath.Dir(path))
	}

	return normalizeDefinition(def), nil
}

type frontmatter struct {
	name        string
	description string
	tools       []string
	discovery   SkillDiscoveryMetadata
	governance  SkillGovernanceMetadata
}

func parseFrontmatter(content string) (frontmatter, string) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return frontmatter{}, content
	}

	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return frontmatter{}, content
	}

	meta := parseMetadataLines(lines[1:end])
	body := strings.Join(lines[end+1:], "\n")
	return meta, body
}

func parseMetadataLines(lines []string) frontmatter {
	var meta frontmatter
	var currentKey string

	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		if strings.HasPrefix(line, "- ") {
			value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
			if value != "" {
				addFrontmatterListValue(&meta, currentKey, value)
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		currentKey = canonicalFrontmatterKey(key)
		value = strings.TrimSpace(value)

		switch currentKey {
		case "name":
			meta.name = trimQuotes(value)
		case "description":
			meta.description = trimQuotes(value)
		case "whentouse":
			meta.discovery.WhenToUse = trimQuotes(value)
		case "preview":
			meta.discovery.Preview = trimQuotes(value)
		case "activationmode":
			meta.discovery.ActivationMode = trimQuotes(value)
		case "userinvocable":
			meta.discovery.UserInvocable = parseFrontmatterBool(value)
		case "modelinvocable":
			meta.discovery.ModelInvocable = parseFrontmatterBool(value)
		case "requiredformatch":
			meta.discovery.RequiredForMatch = parseFrontmatterBool(value)
		case "risk":
			meta.governance.Risk = trimQuotes(value)
		case "tools":
			if value != "" {
				addFrontmatterListValue(&meta, currentKey, value)
			}
		case "resourcetypes", "taskintents", "paths", "modes", "allowedtools", "deniedtools":
			if value != "" {
				addFrontmatterListValue(&meta, currentKey, value)
			}
		default:
			currentKey = ""
		}
	}

	return meta
}

func canonicalFrontmatterKey(key string) string {
	key = strings.TrimSpace(key)
	key = strings.ReplaceAll(key, "_", "")
	key = strings.ReplaceAll(key, "-", "")
	return strings.ToLower(key)
}

func addFrontmatterListValue(meta *frontmatter, key, value string) {
	values := splitMetadataListValue(value)
	switch key {
	case "tools":
		meta.tools = append(meta.tools, values...)
	case "resourcetypes":
		meta.discovery.ResourceTypes = append(meta.discovery.ResourceTypes, values...)
	case "taskintents":
		meta.discovery.TaskIntents = append(meta.discovery.TaskIntents, values...)
	case "paths":
		meta.discovery.Paths = append(meta.discovery.Paths, values...)
	case "modes":
		meta.discovery.Modes = append(meta.discovery.Modes, values...)
	case "allowedtools":
		meta.governance.AllowedTools = append(meta.governance.AllowedTools, values...)
	case "deniedtools":
		meta.governance.DeniedTools = append(meta.governance.DeniedTools, values...)
	}
}

func parseFrontmatterBool(value string) bool {
	switch strings.ToLower(trimQuotes(value)) {
	case "true", "yes", "y", "1", "on":
		return true
	default:
		return false
	}
}

func trimQuotes(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	value = strings.TrimPrefix(value, "'")
	value = strings.TrimSuffix(value, "'")
	return strings.TrimSpace(value)
}
