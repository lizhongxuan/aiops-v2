package skills

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type PathScopedDiscoveryOptions struct {
	WorkspaceRoot string
	ResourcePath  string
	MaxDepth      int
	MaxSkills     int
}

type SkillDiscoveryTrace struct {
	ScannedDirs map[string]bool `json:"scannedDirs,omitempty"`
	SkillFiles  []string        `json:"skillFiles,omitempty"`
	Skipped     []string        `json:"skipped,omitempty"`
}

func DiscoverPathScopedSkills(loader *Loader, opts PathScopedDiscoveryOptions) ([]Definition, SkillDiscoveryTrace, error) {
	if loader == nil {
		loader = NewLoader()
	}
	root, err := filepath.Abs(opts.WorkspaceRoot)
	if err != nil {
		return nil, SkillDiscoveryTrace{}, err
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return nil, SkillDiscoveryTrace{}, err
	}
	resource := strings.TrimSpace(opts.ResourcePath)
	if resource == "" {
		resource = root
	}
	resourceAbs, err := filepath.Abs(resource)
	if err != nil {
		return nil, SkillDiscoveryTrace{}, err
	}
	if info, statErr := os.Stat(resourceAbs); statErr == nil && !info.IsDir() {
		resourceAbs = filepath.Dir(resourceAbs)
	}
	resourceAbs, err = filepath.EvalSymlinks(resourceAbs)
	if err != nil {
		return nil, SkillDiscoveryTrace{}, err
	}
	if !isPathInside(root, resourceAbs) {
		return nil, SkillDiscoveryTrace{}, fmt.Errorf("resource path %q is outside workspace root %q", resourceAbs, root)
	}
	maxDepth := opts.MaxDepth
	if maxDepth <= 0 {
		maxDepth = 8
	}
	maxSkills := opts.MaxSkills
	if maxSkills <= 0 {
		maxSkills = 16
	}
	trace := SkillDiscoveryTrace{ScannedDirs: map[string]bool{}}
	var defs []Definition
	for dir, depth := resourceAbs, 0; ; dir, depth = filepath.Dir(dir), depth+1 {
		if depth > maxDepth {
			trace.Skipped = append(trace.Skipped, "max_depth")
			break
		}
		if !isPathInside(root, dir) {
			break
		}
		trace.ScannedDirs[filepath.Clean(dir)] = true
		files := skillFilesUnderDir(dir)
		sort.Strings(files)
		for _, file := range files {
			realFile, err := filepath.EvalSymlinks(file)
			if err != nil || !isPathInside(root, realFile) {
				trace.Skipped = append(trace.Skipped, file)
				continue
			}
			def, err := loadSkillFile(realFile)
			if err != nil {
				return nil, trace, err
			}
			defs = append(defs, def)
			trace.SkillFiles = append(trace.SkillFiles, realFile)
			if len(defs) >= maxSkills {
				trace.Skipped = append(trace.Skipped, "max_skills")
				return defs, trace, nil
			}
		}
		if filepath.Clean(dir) == filepath.Clean(root) {
			break
		}
	}
	_ = loader
	return defs, trace, nil
}

func skillFilesUnderDir(dir string) []string {
	var files []string
	for _, base := range []string{
		filepath.Join(dir, "skills"),
		filepath.Join(dir, ".codex", "skills"),
	} {
		entries, err := os.ReadDir(base)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			candidate := filepath.Join(base, entry.Name(), "SKILL.md")
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				files = append(files, candidate)
			}
		}
	}
	return files
}

func isPathInside(root, path string) bool {
	root = filepath.Clean(root)
	path = filepath.Clean(path)
	if path == root {
		return true
	}
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
