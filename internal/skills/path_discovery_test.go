package skills

import (
	"os"
	"path/filepath"
	"testing"

	"aiops-v2/internal/commands"
)

func TestDiscoverPathScopedSkillsWalksUpward(t *testing.T) {
	root := t.TempDir()
	serviceDir := filepath.Join(root, "services", "api")
	writeSkillFile(t, filepath.Join(root, ".codex", "skills", "workspace-triage", "SKILL.md"), "workspace.triage")
	writeSkillFile(t, filepath.Join(root, "services", "skills", "service-triage", "SKILL.md"), "service.triage")
	if err := os.MkdirAll(serviceDir, 0o755); err != nil {
		t.Fatalf("mkdir resource dir: %v", err)
	}
	resource := filepath.Join(serviceDir, "runtime.log")
	if err := os.WriteFile(resource, []byte("log"), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}

	defs, trace, err := DiscoverPathScopedSkills(NewLoader(), PathScopedDiscoveryOptions{
		WorkspaceRoot: root,
		ResourcePath:  resource,
		MaxDepth:      8,
		MaxSkills:     8,
	})
	if err != nil {
		t.Fatalf("DiscoverPathScopedSkills() error = %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("defs = %d, trace=%+v", len(defs), trace)
	}
	canonicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatalf("canonical root: %v", err)
	}
	if !trace.ScannedDirs[filepath.Clean(canonicalRoot)] || !trace.ScannedDirs[filepath.Join(canonicalRoot, "services")] {
		t.Fatalf("trace scanned dirs = %+v", trace.ScannedDirs)
	}
}

func TestDiscoverPathScopedSkillsStopsAtWorkspaceRoot(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "workspace")
	if err := os.MkdirAll(filepath.Join(root, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}
	writeSkillFile(t, filepath.Join(parent, ".codex", "skills", "outside", "SKILL.md"), "outside")
	writeSkillFile(t, filepath.Join(root, ".codex", "skills", "inside", "SKILL.md"), "inside")
	resource := filepath.Join(root, "nested", "file.txt")
	if err := os.WriteFile(resource, []byte("file"), 0o644); err != nil {
		t.Fatalf("write resource: %v", err)
	}

	defs, _, err := DiscoverPathScopedSkills(NewLoader(), PathScopedDiscoveryOptions{
		WorkspaceRoot: root,
		ResourcePath:  resource,
		MaxDepth:      8,
		MaxSkills:     8,
	})
	if err != nil {
		t.Fatalf("DiscoverPathScopedSkills() error = %v", err)
	}
	if len(defs) != 1 || defs[0].Name != "inside" {
		t.Fatalf("defs = %+v, want only inside workspace skill", defs)
	}
}

func TestSkillResolutionTraceReportsWinnerAndShadowed(t *testing.T) {
	reg := NewRegistry()
	reg.Register(Definition{Name: "synthetic.triage", Source: commands.SourceProjectSettings, LoadedFrom: "/workspace/skills/triage/SKILL.md"})
	reg.Register(Definition{Name: "synthetic.triage", Source: commands.SourcePlugin, LoadedFrom: "/plugin/skills/triage/SKILL.md"})

	trace := reg.ResolutionTrace("synthetic.triage")
	if trace.Winner != "/plugin/skills/triage/SKILL.md" {
		t.Fatalf("Winner = %q, want plugin path: %+v", trace.Winner, trace)
	}
	if len(trace.Shadowed) != 1 || trace.Shadowed[0] != "/workspace/skills/triage/SKILL.md" {
		t.Fatalf("Shadowed = %+v", trace.Shadowed)
	}
	if trace.Reason == "" {
		t.Fatalf("Reason empty: %+v", trace)
	}
}

func writeSkillFile(t *testing.T, path, name string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir skill dir: %v", err)
	}
	content := "---\nname: " + name + "\ndescription: synthetic skill\n---\nBody.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill %s: %v", path, err)
	}
}
