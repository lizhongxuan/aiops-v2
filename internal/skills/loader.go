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

	def.Prompt = strings.TrimSpace(body)
	if def.Prompt == "" {
		def.Prompt = content
	}
	if def.Name == "" {
		def.Name = filepath.Base(filepath.Dir(path))
	}

	return def, nil
}

type frontmatter struct {
	name        string
	description string
	tools       []string
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
			if currentKey == "tools" {
				value := strings.TrimSpace(strings.TrimPrefix(line, "- "))
				if value != "" {
					meta.tools = append(meta.tools, value)
				}
			}
			continue
		}

		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}

		currentKey = strings.TrimSpace(strings.ToLower(key))
		value = strings.TrimSpace(value)

		switch currentKey {
		case "name":
			meta.name = trimQuotes(value)
		case "description":
			meta.description = trimQuotes(value)
		case "tools":
			if value != "" {
				for _, item := range strings.Split(value, ",") {
					item = strings.TrimSpace(trimQuotes(item))
					if item != "" {
						meta.tools = append(meta.tools, item)
					}
				}
			}
		default:
			currentKey = ""
		}
	}

	return meta
}

func trimQuotes(value string) string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "\"")
	value = strings.TrimSuffix(value, "\"")
	value = strings.TrimPrefix(value, "'")
	value = strings.TrimSuffix(value, "'")
	return strings.TrimSpace(value)
}
