package skills

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillLoaderParsesDiscoveryMetadata(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	body := `---
name: synthetic.triage
description: "Short triage skill"
when_to_use: "Use when generic diagnosis needs a structured checklist"
resource_types:
  - log
  - metric
taskIntents: diagnose, explain
paths:
  - "services/*"
modes: read_only, plan
activationMode: model
userInvocable: true
modelInvocable: true
requiredForMatch: true
risk: read
allowed_tools:
  - list_resources
deniedTools: write_resource, delete_resource
tools:
  - list_resources
---
Skill body.
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	def, err := loadSkillFile(path)
	if err != nil {
		t.Fatalf("loadSkillFile() error = %v", err)
	}

	if def.Discovery.WhenToUse != "Use when generic diagnosis needs a structured checklist" {
		t.Fatalf("WhenToUse = %q", def.Discovery.WhenToUse)
	}
	if got := strings.Join(def.Discovery.ResourceTypes, ","); got != "log,metric" {
		t.Fatalf("ResourceTypes = %q", got)
	}
	if got := strings.Join(def.Discovery.TaskIntents, ","); got != "diagnose,explain" {
		t.Fatalf("TaskIntents = %q", got)
	}
	if got := strings.Join(def.Discovery.Paths, ","); got != "services/*" {
		t.Fatalf("Paths = %q", got)
	}
	if got := strings.Join(def.Discovery.Modes, ","); got != "read_only,plan" {
		t.Fatalf("Modes = %q", got)
	}
	if !def.Discovery.UserInvocable || !def.Discovery.ModelInvocable || !def.Discovery.RequiredForMatch {
		t.Fatalf("unexpected invocation flags: %+v", def.Discovery)
	}
	if def.Discovery.ActivationMode != "model" {
		t.Fatalf("ActivationMode = %q", def.Discovery.ActivationMode)
	}
	if def.Governance.Risk != "read" {
		t.Fatalf("Risk = %q", def.Governance.Risk)
	}
	if got := strings.Join(def.Governance.AllowedTools, ","); got != "list_resources" {
		t.Fatalf("AllowedTools = %q", got)
	}
	if got := strings.Join(def.Governance.DeniedTools, ","); got != "write_resource,delete_resource" {
		t.Fatalf("DeniedTools = %q", got)
	}
}

func TestSkillLoaderCapsDescriptionAndPreview(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "SKILL.md")
	longDescription := strings.Repeat("d", MaxSkillDescriptionChars+80)
	longWhenToUse := strings.Repeat("w", MaxSkillWhenToUseChars+80)
	longPreview := strings.Repeat("p", MaxSkillPreviewChars+80)

	content := `---
name: synthetic.long
description: "` + longDescription + `"
whenToUse: "` + longWhenToUse + `"
preview: "` + longPreview + `"
---
Body.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	def, err := loadSkillFile(path)
	if err != nil {
		t.Fatalf("loadSkillFile() error = %v", err)
	}

	if len(def.Description) > MaxSkillDescriptionChars {
		t.Fatalf("Description length = %d, want <= %d", len(def.Description), MaxSkillDescriptionChars)
	}
	if len(def.Discovery.WhenToUse) > MaxSkillWhenToUseChars {
		t.Fatalf("WhenToUse length = %d, want <= %d", len(def.Discovery.WhenToUse), MaxSkillWhenToUseChars)
	}
	if len(def.Discovery.Preview) > MaxSkillPreviewChars {
		t.Fatalf("Preview length = %d, want <= %d", len(def.Discovery.Preview), MaxSkillPreviewChars)
	}
	if !def.Truncated.Description || !def.Truncated.WhenToUse || !def.Truncated.Preview {
		t.Fatalf("Truncated = %+v, want all true", def.Truncated)
	}
}

func TestSkillIndexBudgetSelectsRelevantSkills(t *testing.T) {
	defs := []Definition{
		{
			Name:        "synthetic.logs",
			Description: "Handles log diagnosis.",
			Prompt:      strings.Repeat("never indexed", 100),
			Discovery: SkillDiscoveryMetadata{
				WhenToUse:      "Use for log diagnosis.",
				ResourceTypes:  []string{"log"},
				TaskIntents:    []string{"diagnose"},
				Paths:          []string{"services/*"},
				Modes:          []string{"read_only"},
				ModelInvocable: true,
			},
		},
		{
			Name:        "synthetic.deploy",
			Description: "Handles deployment planning.",
			Prompt:      strings.Repeat("never indexed", 100),
			Discovery: SkillDiscoveryMetadata{
				WhenToUse:      "Use for release work.",
				ResourceTypes:  []string{"deployment"},
				TaskIntents:    []string{"deploy"},
				Paths:          []string{"deployments/*"},
				Modes:          []string{"write"},
				ModelInvocable: true,
			},
		},
	}

	result := BuildSkillIndex(defs, SkillIndexOptions{
		Query:       "diagnose log issue",
		ResourceURI: "services/api/runtime.log",
		Mode:        "read_only",
		MaxChars:    420,
	})

	if len(result.Entries) != 1 {
		t.Fatalf("Entries length = %d, want 1: %+v", len(result.Entries), result)
	}
	if result.Entries[0].Name != "synthetic.logs" {
		t.Fatalf("selected %q, want synthetic.logs", result.Entries[0].Name)
	}
	if strings.Contains(result.Entries[0].Preview, "never indexed") {
		t.Fatalf("index leaked prompt body in preview: %q", result.Entries[0].Preview)
	}
	if result.Hash == "" {
		t.Fatal("expected stable index hash")
	}
}

func TestSkillIndexReportsDroppedSkills(t *testing.T) {
	defs := []Definition{
		{
			Name:        "synthetic.first",
			Description: strings.Repeat("a", 100),
			Discovery:   SkillDiscoveryMetadata{TaskIntents: []string{"diagnose"}, ModelInvocable: true},
		},
		{
			Name:        "synthetic.second",
			Description: strings.Repeat("b", 100),
			Discovery:   SkillDiscoveryMetadata{TaskIntents: []string{"diagnose"}, ModelInvocable: true},
		},
		{
			Name:        "synthetic.hidden",
			Description: "user only",
			Discovery:   SkillDiscoveryMetadata{TaskIntents: []string{"diagnose"}, ModelInvocable: false},
		},
	}

	result := BuildSkillIndex(defs, SkillIndexOptions{Query: "diagnose", MaxChars: 220})
	if len(result.Entries) == 0 {
		t.Fatalf("expected at least one retained entry: %+v", result)
	}
	reasons := map[string]bool{}
	for _, dropped := range result.Dropped {
		reasons[dropped.Reason] = true
	}
	if !reasons["budget_exceeded"] {
		t.Fatalf("expected budget_exceeded drop, got %+v", result.Dropped)
	}
	if !reasons["model_disabled"] {
		t.Fatalf("expected model_disabled drop, got %+v", result.Dropped)
	}
}
