package skills

import (
	"path/filepath"
	"strings"
)

// ResolveFileIdentity returns a stable identity for a file-backed skill path.
// It prefers the symlink-resolved absolute path and falls back to a cleaned absolute path.
func ResolveFileIdentity(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return filepath.Clean(abs)
	}
	return filepath.Clean(resolved)
}
