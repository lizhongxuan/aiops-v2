package commands

import (
	"strings"
	"testing"

	"aiops-v2/internal/settings"
)

func TestRegistry_RegisterPromptAndGetPrompt(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterPrompt(PromptCommand{
		Name:        "deploy",
		Description: "project version",
		Prompt:      "Deploy from project.",
		Tools:       []string{"kubectl"},
		Source:      SourceProjectSettings,
		LoadedFrom:  LoadedFromSkills,
		WhenToUse:   "Use for repo deploys.",
	}); err != nil {
		t.Fatalf("RegisterPrompt failed: %v", err)
	}

	if err := r.RegisterPrompt(PromptCommand{
		Name:        "deploy",
		Description: "plugin version",
		Prompt:      "Deploy from plugin.",
		Tools:       []string{"kubectl", "helm"},
		Source:      SourcePlugin,
		LoadedFrom:  LoadedFromPlugin,
	}); err != nil {
		t.Fatalf("RegisterPrompt second source failed: %v", err)
	}

	got, ok := r.GetPrompt("deploy")
	if !ok {
		t.Fatal("expected deploy prompt command to be present")
	}
	if got.Description != "plugin version" {
		t.Fatalf("expected higher-precedence source to win, got description %q", got.Description)
	}
	if len(got.Tools) != 2 || got.Tools[0] != "kubectl" || got.Tools[1] != "helm" {
		t.Fatalf("unexpected tools %#v", got.Tools)
	}
	if got.LoadedFrom != LoadedFromPlugin {
		t.Fatalf("expected plugin loaded_from to survive active view, got %q", got.LoadedFrom)
	}

	if len(r.ListPrompt()) != 1 {
		t.Fatalf("expected 1 prompt command, got %d", len(r.ListPrompt()))
	}
}

func TestRegistry_RegisterPromptFirstWinsWithinSameSourceAndLoadedFrom(t *testing.T) {
	r := NewRegistry()

	first := PromptCommand{
		Name:        "deploy",
		Description: "first version",
		Prompt:      "Deploy carefully.",
		Source:      SourceProjectSettings,
		LoadedFrom:  LoadedFromSkills,
	}
	second := PromptCommand{
		Name:        "deploy",
		Description: "second version",
		Prompt:      "Deploy with more checks.",
		Source:      SourceProjectSettings,
		LoadedFrom:  LoadedFromSkills,
	}

	mustRegisterPrompt(t, r, first)
	mustRegisterPrompt(t, r, second)

	got, ok := r.GetPrompt("deploy")
	if !ok {
		t.Fatal("expected deploy prompt command to be present")
	}
	if got.Description != first.Description {
		t.Fatalf("expected first registration to win within same precedence bucket, got %q", got.Description)
	}
}

func TestRegistry_RegisterLocalDoesNotLeakIntoPromptSurface(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterPrompt(PromptCommand{Name: "repo-skill", Source: "repo"}); err != nil {
		t.Fatalf("RegisterPrompt failed: %v", err)
	}
	if err := r.RegisterLocal(LocalCommand{Name: "local-only", Source: "local"}); err != nil {
		t.Fatalf("RegisterLocal failed: %v", err)
	}

	prompts := r.ListPrompt()
	if len(prompts) != 1 {
		t.Fatalf("expected only prompt commands in prompt surface, got %d", len(prompts))
	}
	if prompts[0].Name != "repo-skill" {
		t.Fatalf("unexpected prompt command %q", prompts[0].Name)
	}

	if _, ok := r.GetPrompt("local-only"); ok {
		t.Fatal("expected local command to stay out of prompt lookup")
	}
}

func TestRegistry_ListSkillLikePromptCommandsFiltersBySource(t *testing.T) {
	r := NewRegistry()

	mustRegisterPrompt(t, r, PromptCommand{Name: "repo-skill", Source: SourceProjectSettings})
	mustRegisterPrompt(t, r, PromptCommand{Name: "plugin-skill", Source: SourcePlugin})
	mustRegisterPrompt(t, r, PromptCommand{Name: "bundled-skill", Source: SourceBundled})
	mustRegisterPrompt(t, r, PromptCommand{Name: "project-skill", Source: SourceProjectSettings})
	mustRegisterPrompt(t, r, PromptCommand{Name: "ordinary-local", Source: "local"})
	mustRegisterPrompt(t, r, PromptCommand{Name: "empty-source"})

	got := r.ListSkillLikePromptCommands()
	if len(got) != 4 {
		t.Fatalf("expected 4 skill-like prompt commands, got %d", len(got))
	}

	names := []string{got[0].Name, got[1].Name, got[2].Name, got[3].Name}
	want := []string{"repo-skill", "plugin-skill", "bundled-skill", "project-skill"}
	for i := range want {
		if names[i] != want[i] {
			t.Fatalf("unexpected skill-like command order: got %v want %v", names, want)
		}
	}
	for _, cmd := range got {
		if cmd.Source == "local" || cmd.Name == "empty-source" {
			t.Fatalf("unexpected non-skill command in skill-like surface: %#v", cmd)
		}
	}
}

func TestRegistry_UnregisterPromptRecordRevealsNextCandidate(t *testing.T) {
	r := NewRegistry()

	project := PromptCommand{
		Name:        "deploy",
		Description: "project version",
		Prompt:      "Deploy from project.",
		Source:      SourceProjectSettings,
		LoadedFrom:  LoadedFromSkills,
	}
	plugin := PromptCommand{
		Name:        "deploy",
		Description: "plugin version",
		Prompt:      "Deploy from plugin.",
		Source:      SourcePlugin,
		LoadedFrom:  LoadedFromPlugin,
	}

	mustRegisterPrompt(t, r, project)
	mustRegisterPrompt(t, r, plugin)

	r.UnregisterPromptRecord(plugin)

	got, ok := r.GetPrompt("deploy")
	if !ok {
		t.Fatal("expected project deploy prompt command to remain")
	}
	if got.Description != project.Description {
		t.Fatalf("expected fallback prompt after exact unregister, got %q", got.Description)
	}
}

func TestPromptCommandIsSkillLikeByLoadedFrom(t *testing.T) {
	cmd := PromptCommand{
		Name:       "managed-skill",
		LoadedFrom: LoadedFromManaged,
	}

	if !cmd.IsSkillLike() {
		t.Fatal("expected managed skill command to be considered skill-like")
	}
}

func TestRegistry_PromptCommandPreservesSkillDiscoveryAndGovernance(t *testing.T) {
	r := NewRegistry()
	cmd := PromptCommand{
		Name:        "synthetic.skill",
		Description: "synthetic skill",
		Source:      SourceProjectSettings,
		LoadedFrom:  LoadedFromSkills,
		Discovery: SkillDiscoveryMetadata{
			WhenToUse:        "Use for synthetic checks.",
			ResourceTypes:    []string{"log"},
			TaskIntents:      []string{"diagnose"},
			Paths:            []string{"services/*"},
			Modes:            []string{"read_only"},
			ModelInvocable:   true,
			RequiredForMatch: true,
		},
		Governance: SkillGovernanceMetadata{
			Risk:         "read",
			AllowedTools: []string{"list_resources"},
			DeniedTools:  []string{"write_resource"},
		},
	}

	mustRegisterPrompt(t, r, cmd)
	got, ok := r.GetPrompt("synthetic.skill")
	if !ok {
		t.Fatal("expected synthetic.skill prompt")
	}
	if got.Discovery.WhenToUse != cmd.Discovery.WhenToUse || got.Discovery.ResourceTypes[0] != "log" {
		t.Fatalf("discovery metadata not preserved: %+v", got.Discovery)
	}
	if got.Governance.Risk != "read" || got.Governance.DeniedTools[0] != "write_resource" {
		t.Fatalf("governance metadata not preserved: %+v", got.Governance)
	}

	got.Discovery.ResourceTypes[0] = "mutated"
	got.Governance.DeniedTools[0] = "mutated"
	gotAgain, _ := r.GetPrompt("synthetic.skill")
	if gotAgain.Discovery.ResourceTypes[0] != "log" || gotAgain.Governance.DeniedTools[0] != "write_resource" {
		t.Fatalf("metadata slices were not cloned: %+v %+v", gotAgain.Discovery, gotAgain.Governance)
	}
}

func TestPromptCommandIsSkillLikeByDeprecatedCommandsLoadedFrom(t *testing.T) {
	cmd := PromptCommand{
		Name:       "legacy-skill",
		LoadedFrom: LoadedFromCommandsDeprecated,
	}

	if !cmd.IsSkillLike() {
		t.Fatal("expected deprecated command-backed skill to be considered skill-like")
	}
}

func TestRegistry_RejectsEmptyNames(t *testing.T) {
	r := NewRegistry()

	if err := r.RegisterPrompt(PromptCommand{}); err == nil {
		t.Fatal("expected empty prompt name to be rejected")
	}
	if err := r.RegisterLocal(LocalCommand{}); err == nil {
		t.Fatal("expected empty local name to be rejected")
	}
}

func TestRegistryRejectsCustomSkillLikeCommandsWhenStrictPluginOnlyEnabled(t *testing.T) {
	governance := settings.NewGovernance()
	if err := governance.Register("managed", settings.GovernanceContribution{
		RestrictToPluginOnly: []settings.CustomizationSurface{settings.SurfaceSkills},
	}); err != nil {
		t.Fatalf("governance Register() error = %v", err)
	}

	r := NewRegistry()
	r.SetGovernance(governance)

	err := r.RegisterPrompt(PromptCommand{
		Name:       "custom-skill",
		Source:     SourceUserSettings,
		LoadedFrom: LoadedFromSkills,
	})
	if err == nil {
		t.Fatal("expected strict plugin-only policy to reject userSettings skill command")
	}
	if !strings.Contains(err.Error(), "strictPluginOnlyCustomization") {
		t.Fatalf("expected strict plugin-only error, got %v", err)
	}
}

func mustRegisterPrompt(t *testing.T, r *CommandRegistry, cmd PromptCommand) {
	t.Helper()
	if err := r.RegisterPrompt(cmd); err != nil {
		t.Fatalf("RegisterPrompt(%q) failed: %v", cmd.Name, err)
	}
}
