package skilltools

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/commands"
	"aiops-v2/internal/skills"
	"aiops-v2/internal/tooling"
)

func TestSkillToolsAreInitialBaseTools(t *testing.T) {
	reg := skills.NewRegistry()
	for _, tool := range []tooling.Tool{NewSkillSearchTool(reg), NewSkillReadTool(reg)} {
		meta := tool.Metadata()
		if meta.Layer != tooling.ToolLayerCore || !meta.AlwaysLoad {
			t.Fatalf("%s metadata = layer:%q alwaysLoad:%v, want core always-load", meta.Name, meta.Layer, meta.AlwaysLoad)
		}
		if meta.EffectiveDiscovery().RequiresSelect {
			t.Fatalf("%s discovery = %+v, want initial callable tool", meta.Name, meta.EffectiveDiscovery())
		}
	}
}

func TestSkillSearchReturnsCompactMatches(t *testing.T) {
	reg := skills.NewRegistry()
	reg.Register(skills.Definition{
		Name:        "synthetic.triage",
		Description: "Generic triage checklist.",
		Prompt:      strings.Repeat("full-body-should-not-appear ", 20),
		Source:      commands.SourceProjectSettings,
		Discovery: skills.SkillDiscoveryMetadata{
			WhenToUse:      "Use for synthetic diagnosis.",
			ResourceTypes:  []string{"log"},
			TaskIntents:    []string{"diagnose"},
			ModelInvocable: true,
		},
		Governance: skills.SkillGovernanceMetadata{Risk: "read"},
	})

	result, err := NewSkillSearchTool(reg).Execute(context.Background(), json.RawMessage(`{"mode":"search","query":"diagnose log","limit":5}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var out searchOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.SchemaVersion != "aiops.skill_discovery/v1" {
		t.Fatalf("SchemaVersion = %q", out.SchemaVersion)
	}
	if len(out.Matches) != 1 || out.Matches[0].Name != "synthetic.triage" {
		t.Fatalf("matches = %+v", out.Matches)
	}
	if out.Matches[0].RequiresRead != true || out.Matches[0].Risk != "read" {
		t.Fatalf("match metadata = %+v", out.Matches[0])
	}
	if strings.Contains(result.Content, "full-body-should-not-appear") {
		t.Fatalf("skill_search leaked full prompt body: %s", result.Content)
	}
	if result.Display == nil || result.Display.Type != "skill_search" {
		t.Fatalf("display = %+v, want skill_search", result.Display)
	}
}

func TestSkillReadReturnsBoundedBody(t *testing.T) {
	reg := skills.NewRegistry()
	body := "0123456789abcdefghijklmnopqrstuvwxyz"
	reg.Register(skills.Definition{
		Name:        "synthetic.triage",
		Description: "Generic triage checklist.",
		Prompt:      body,
		Source:      commands.SourceProjectSettings,
		Discovery:   skills.SkillDiscoveryMetadata{ModelInvocable: true},
		Governance: skills.SkillGovernanceMetadata{
			Risk:         "low",
			AllowedTools: []string{"synthetic.read"},
			DeniedTools:  []string{"synthetic.write"},
		},
	})

	result, err := NewSkillReadTool(reg).Execute(context.Background(), json.RawMessage(`{
		"skill":"synthetic.triage",
		"range":{"offset":10,"limit":8},
		"reason":"Need the relevant checklist before final answer"
	}`))
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	var out readOutput
	if err := json.Unmarshal([]byte(result.Content), &out); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if out.SchemaVersion != "aiops.skill_discovery/v1" {
		t.Fatalf("SchemaVersion = %q", out.SchemaVersion)
	}
	if out.Body != "abcdefgh" {
		t.Fatalf("Body = %q, want bounded range", out.Body)
	}
	if len(out.LoadedSkills) != 1 || out.LoadedSkills[0].Name != "synthetic.triage" || out.LoadedSkills[0].Source != "skill_read" {
		t.Fatalf("LoadedSkills = %+v", out.LoadedSkills)
	}
	if out.LoadedSkills[0].Reason == "" || out.LoadedSkills[0].Hash == "" {
		t.Fatalf("LoadedSkills missing reason/hash: %+v", out.LoadedSkills[0])
	}
	if out.LoadedSkills[0].RiskCeiling != "low" || out.LoadedSkills[0].AllowedTools[0] != "synthetic.read" || out.LoadedSkills[0].DeniedTools[0] != "synthetic.write" {
		t.Fatalf("LoadedSkills governance = %+v", out.LoadedSkills[0])
	}
	if result.Display == nil || result.Display.Type != "skill_read" {
		t.Fatalf("display = %+v, want skill_read", result.Display)
	}
}

func TestSkillReadRequiresReason(t *testing.T) {
	reg := skills.NewRegistry()
	reg.Register(skills.Definition{Name: "synthetic.triage", Prompt: "body"})

	_, err := NewSkillReadTool(reg).Execute(context.Background(), json.RawMessage(`{"skill":"synthetic.triage"}`))
	if err == nil {
		t.Fatal("Execute() error = nil, want reason required")
	}
}
