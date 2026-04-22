package skills

import (
	"os"
	"path/filepath"
	"testing"

	"aiops-v2/internal/commands"
)

func TestRegistry_RegisterByNameKeepsMultipleSourceAwareRecords(t *testing.T) {
	r := NewRegistry()

	r.Register(Definition{
		Name:        "deploy",
		Description: "project version",
		Prompt:      "Deploy carefully.",
		Tools:       []string{"kubectl"},
		Source:      commands.SourceProjectSettings,
		LoadedFrom:  "/repo/.codex/skills/deploy/SKILL.md",
		FileID:      "project:deploy",
	})
	r.Register(Definition{
		Name:        "deploy",
		Description: "plugin version",
		Prompt:      "Deploy with checks.",
		Tools:       []string{"kubectl", "helm"},
		Source:      commands.SourcePlugin,
		LoadedFrom:  "/plugins/deploy/SKILL.md",
		FileID:      "plugin:deploy",
	})

	got, ok := r.Get("deploy")
	if !ok {
		t.Fatal("expected deploy to be registered")
	}
	if got.Description != "plugin version" {
		t.Fatalf("expected higher-precedence source to win, got description %q", got.Description)
	}
	if len(r.List()) != 2 {
		t.Fatalf("expected both source-aware records to remain in catalog, got %d", len(r.List()))
	}
}

func TestRegistry_RegisterDeduplicatesByFileIdentity(t *testing.T) {
	r := NewRegistry()

	r.Register(Definition{
		Name:        "deploy",
		Description: "first version",
		Prompt:      "Deploy carefully.",
		Source:      commands.SourceProjectSettings,
		LoadedFrom:  "/repo/.codex/skills/deploy/SKILL.md",
		FileID:      "file-123",
	})
	r.Register(Definition{
		Name:        "deploy-copy",
		Description: "duplicate file",
		Prompt:      "Deploy carefully.",
		Source:      commands.SourceProjectSettings,
		LoadedFrom:  "/repo/.codex/skills-link/deploy/SKILL.md",
		FileID:      "file-123",
	})

	defs := r.List()
	if len(defs) != 1 {
		t.Fatalf("expected duplicate file identity to be skipped, got %d defs", len(defs))
	}
	if defs[0].Name != "deploy" {
		t.Fatalf("expected first file-backed definition to win, got %q", defs[0].Name)
	}
}

func TestRegistry_promptCommandsCarriesPromptBodies(t *testing.T) {
	r := NewRegistry()
	r.RegisterBatch([]Definition{
		{Name: "empty", Prompt: "", Source: "empty"},
		{Name: "non-empty", Prompt: "Use this prompt.", Source: "non-empty"},
	})

	cmds := r.promptCommands(commands.SourceProjectSettings)
	if len(cmds) != 2 {
		t.Fatalf("expected 2 prompt commands, got %d", len(cmds))
	}
	if cmds[0].Prompt != "" {
		t.Fatalf("expected empty prompt to stay empty, got %q", cmds[0].Prompt)
	}
	if cmds[1].Prompt != "Use this prompt." {
		t.Fatalf("unexpected prompt body %q", cmds[1].Prompt)
	}
}

func TestRegistry_promptCommandsInfersClaudeLikeSources(t *testing.T) {
	r := NewRegistry()
	r.RegisterBatch([]Definition{
		{
			Name:        "plugin-skill",
			Description: "plugin prompt",
			Prompt:      "Plugin prompt body.",
			LoadedFrom:  "/Users/me/.codex/plugins/cache/example/skills/plugin-skill/SKILL.md",
		},
		{
			Name:        "bundled-skill",
			Description: "bundled prompt",
			Prompt:      "Bundled prompt body.",
			LoadedFrom:  "/Users/me/.codex/skills/.system/bundled-skill/SKILL.md",
		},
		{
			Name:        "project-skill",
			Description: "project prompt",
			Prompt:      "Project prompt body.",
			LoadedFrom:  "/repo/docs/skills/project-skill/SKILL.md",
		},
	})

	cmds := r.promptCommands(commands.SourceProjectSettings)
	if len(cmds) != 3 {
		t.Fatalf("expected 3 prompt commands, got %d", len(cmds))
	}
	if cmds[0].Source != commands.SourcePlugin {
		t.Fatalf("plugin skill source = %q, want %q", cmds[0].Source, commands.SourcePlugin)
	}
	if cmds[1].Source != commands.SourceBundled {
		t.Fatalf("bundled skill source = %q, want %q", cmds[1].Source, commands.SourceBundled)
	}
	if cmds[2].Source != commands.SourceProjectSettings {
		t.Fatalf("project skill source = %q, want %q", cmds[2].Source, commands.SourceProjectSettings)
	}
	if cmds[2].LoadedFrom != commands.LoadedFromSkills {
		t.Fatalf("project skill loadedFrom = %q, want %q", cmds[2].LoadedFrom, commands.LoadedFromSkills)
	}
}

func TestPromptCommandForDefinitionHonorsExplicitSource(t *testing.T) {
	cmd := PromptCommandForDefinition(Definition{
		Name:        "deploy",
		Description: "deploy",
		Prompt:      "Use deploy.",
		Source:      commands.SourcePolicySettings,
		LoadedFrom:  "/repo/skills/deploy/SKILL.md",
	}, commands.SourceProjectSettings)

	if cmd.Source != commands.SourcePolicySettings {
		t.Fatalf("explicit source should win, got %q", cmd.Source)
	}
	if cmd.LoadedFrom != commands.LoadedFromManaged {
		t.Fatalf("loadedFrom marker should still reflect skill projection, got %q", cmd.LoadedFrom)
	}
}

func TestLoader_LoadDir(t *testing.T) {
	root := t.TempDir()

	alphaDir := filepath.Join(root, "alpha")
	betaDir := filepath.Join(root, "nested", "beta")
	if err := os.MkdirAll(alphaDir, 0o755); err != nil {
		t.Fatalf("mkdir alpha: %v", err)
	}
	if err := os.MkdirAll(betaDir, 0o755); err != nil {
		t.Fatalf("mkdir beta: %v", err)
	}

	if err := os.WriteFile(filepath.Join(alphaDir, "SKILL.md"), []byte(`---
name: alpha
description: Alpha skill
tools:
  - bash
  - git
---
Alpha prompt body.
`), 0o644); err != nil {
		t.Fatalf("write alpha skill: %v", err)
	}

	if err := os.WriteFile(filepath.Join(betaDir, "SKILL.md"), []byte("Beta prompt body.\n"), 0o644); err != nil {
		t.Fatalf("write beta skill: %v", err)
	}

	defs, err := NewLoader().LoadDir(root)
	if err != nil {
		t.Fatalf("LoadDir failed: %v", err)
	}
	if len(defs) != 2 {
		t.Fatalf("expected 2 skills, got %d", len(defs))
	}

	reg := NewRegistry()
	reg.RegisterBatch(defs)

	alpha, ok := reg.Get("alpha")
	if !ok {
		t.Fatal("expected alpha to be loaded")
	}
	if alpha.Description != "Alpha skill" {
		t.Fatalf("unexpected alpha description %q", alpha.Description)
	}
	if alpha.Prompt != "Alpha prompt body." {
		t.Fatalf("unexpected alpha prompt %q", alpha.Prompt)
	}
	if len(alpha.Tools) != 2 || alpha.Tools[0] != "bash" || alpha.Tools[1] != "git" {
		t.Fatalf("unexpected alpha tools %#v", alpha.Tools)
	}

	beta, ok := reg.Get("beta")
	if !ok {
		t.Fatal("expected beta to be loaded from directory name")
	}
	if beta.Prompt != "Beta prompt body." {
		t.Fatalf("unexpected beta prompt %q", beta.Prompt)
	}
	if beta.LoadedFrom == "" {
		t.Fatal("expected beta loadedFrom to be populated")
	}
	if beta.FileID == "" {
		t.Fatal("expected beta file identity to be populated")
	}
}
