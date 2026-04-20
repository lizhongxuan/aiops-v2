package skills

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRegistry_RegisterAndDeduplicate(t *testing.T) {
	r := NewRegistry()

	r.Register(Definition{
		Name:        "deploy",
		Description: "first version",
		Prompt:      "Deploy carefully.",
		Tools:       []string{"kubectl"},
		Source:      "first",
	})
	r.Register(Definition{
		Name:        "deploy",
		Description: "second version",
		Prompt:      "Deploy with checks.",
		Tools:       []string{"kubectl", "helm"},
		Source:      "second",
	})

	got, ok := r.Get("deploy")
	if !ok {
		t.Fatal("expected deploy to be registered")
	}
	if got.Description != "second version" {
		t.Fatalf("expected last registration to win, got description %q", got.Description)
	}
	if len(r.List()) != 1 {
		t.Fatalf("expected deduplicated list size 1, got %d", len(r.List()))
	}
}

func TestRegistry_PromptAssetsFiltersEmptyPrompts(t *testing.T) {
	r := NewRegistry()
	r.RegisterBatch([]Definition{
		{Name: "empty", Prompt: "", Source: "empty"},
		{Name: "non-empty", Prompt: "Use this prompt.", Source: "non-empty"},
	})

	assets := r.PromptAssets()
	if len(assets) != 1 {
		t.Fatalf("expected 1 prompt asset, got %d", len(assets))
	}
	if assets[0] != "Use this prompt." {
		t.Fatalf("unexpected prompt asset %q", assets[0])
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
	if beta.Source == "" {
		t.Fatal("expected beta source to be populated")
	}
}
