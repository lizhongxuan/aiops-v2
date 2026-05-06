package appui

import (
	"os"
	"path/filepath"
	"strings"
)

func projectRelativePath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" || filepath.IsAbs(rel) {
		return rel
	}
	wd, err := os.Getwd()
	if err != nil {
		return rel
	}
	for dir := wd; dir != ""; dir = filepath.Dir(dir) {
		candidate := filepath.Join(dir, rel)
		if pathOrGlobExists(candidate) {
			return candidate
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
	}
	return rel
}

func pathOrGlobExists(path string) bool {
	if strings.ContainsAny(path, "*?[") {
		matches, err := filepath.Glob(path)
		return err == nil && len(matches) > 0
	}
	_, err := os.Stat(path)
	return err == nil
}
