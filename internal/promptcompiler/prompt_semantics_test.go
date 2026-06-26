package promptcompiler

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/tooling"
)

func TestCompiledPromptToMessagesCarriesSemanticLayerMetadata(t *testing.T) {
	messages := CompiledPromptToMessages(CompiledPrompt{
		System:    SystemPrompt{Content: "system"},
		Developer: DeveloperInstructions{Content: "developer"},
		Tools:     ToolPromptSet{Content: "tools"},
		Policy:    RuntimePolicyPrompt{Content: "policy"},
	})

	if len(messages) != 4 {
		t.Fatalf("messages len = %d, want 4", len(messages))
	}
	want := []struct {
		layer string
		role  string
	}{
		{"system", "system"},
		{"developer", "developer"},
		{"tool_index", "tool"},
		{"runtime_policy", "context"},
	}
	for i, msg := range messages {
		if msg.Role != schema.System {
			t.Fatalf("provider role at %d = %q, want system fallback", i, msg.Role)
		}
		if msg.Extra["prompt_layer"] != want[i].layer || msg.Extra["semantic_role"] != want[i].role {
			t.Fatalf("message %d extra = %#v, want layer=%s role=%s", i, msg.Extra, want[i].layer, want[i].role)
		}
	}
}

func TestToolIndexIncludesCommonPolicyAndCompactEntries(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		AssembledTools: []Tool{fakePromptTool{
			name:        "read_file",
			description: "Read a workspace file.",
			readOnly:    true,
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	content := compiled.Tools.Content
	for _, want := range []string{"Common policy:", "Failure, empty output, denial, or timeout is not proof of healthy state.", "read_file", "Read a workspace file."} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool index missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{"Usage policy:", "Example:", "Failure handling:"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("compact tool index should not include %q:\n%s", forbidden, content)
		}
	}
}

func TestSkillPromptAssetsRenderAsProgressiveDisclosureContext(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		SkillPromptAssets: []string{"Full activated skill body"},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(compiled.Dynamic.Content, "Active Skill Context") {
		t.Fatalf("dynamic prompt missing skill context wrapper:\n%s", compiled.Dynamic.Content)
	}
	if !strings.Contains(compiled.Dynamic.Content, "activated skills") {
		t.Fatalf("dynamic prompt missing progressive disclosure guidance:\n%s", compiled.Dynamic.Content)
	}
	if strings.HasPrefix(strings.TrimSpace(compiled.Dynamic.Content), "Full activated skill body") {
		t.Fatalf("skill body was appended raw without wrapper:\n%s", compiled.Dynamic.Content)
	}
}

func TestLongSkillPromptAssetRendersOnlyProgressiveDisclosure(t *testing.T) {
	longBody := strings.Join([]string{
		"---",
		"name: synthetic.long",
		"description: Use this skill for long operational diagnosis.",
		"when_to_use: Use when the model needs a detailed checklist.",
		"---",
		strings.Repeat("FULL_SKILL_BODY_SHOULD_NOT_INLINE ", 120),
	}, "\n")
	compiled, err := NewCompiler().Compile(CompileContext{
		SkillPromptAssets: []string{longBody},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{
		"summary_type: progressive_disclosure",
		"name: activated-skill-1",
		"trigger: active_skill",
		"capability_summary: Use this skill for long operational diagnosis.",
		"entry_tool_or_source_ref: skill_read or prompt_trace://dynamic.skill/asset-1",
	} {
		if !strings.Contains(compiled.Dynamic.Content, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, compiled.Dynamic.Content)
		}
	}
	if strings.Contains(compiled.Dynamic.Content, "FULL_SKILL_BODY_SHOULD_NOT_INLINE") {
		t.Fatalf("dynamic prompt leaked full skill body:\n%s", compiled.Dynamic.Content)
	}
}

func TestLoadedSkillRefsRenderAsDynamicDelta(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		LoadedSkillRefs: []LoadedSkillPromptRef{{
			Name:   "synthetic.triage",
			Source: "skill_read",
			Reason: "Need relevant checklist",
			Range:  "0:128",
			Hash:   "sha256:body",
		}},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if !strings.Contains(compiled.Dynamic.Content, "## Newly loaded skills") {
		t.Fatalf("dynamic prompt missing loaded skill marker:\n%s", compiled.Dynamic.Content)
	}
	if !strings.Contains(compiled.Dynamic.Content, "synthetic.triage: loaded by skill_read; reason=Need relevant checklist") {
		t.Fatalf("dynamic prompt missing loaded skill detail:\n%s", compiled.Dynamic.Content)
	}
	if strings.Contains(compiled.Dynamic.Content, "Full activated skill body") {
		t.Fatalf("loaded skill marker leaked body:\n%s", compiled.Dynamic.Content)
	}
}

func TestProtocolPromptStateRendersStructuredItems(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		ProtocolState: ProtocolPromptState{
			Items: []ProtocolPromptItem{
				{Kind: "approval", ID: "approval-1", Status: "pending", Text: "write_file requires approval"},
				{Kind: "todo", ID: "todo-1", Status: "in_progress", Text: "verify trace output"},
			},
		},
	})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	for _, want := range []string{"## Protocol State", "kind=approval", "kind=todo", "approval-1", "verify trace output"} {
		if !strings.Contains(compiled.Dynamic.Content, want) {
			t.Fatalf("dynamic prompt missing %q:\n%s", want, compiled.Dynamic.Content)
		}
	}
}

func TestPromptDeveloperRulesIncludeGenericityHardcodingBoundary(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	content := compiled.Developer.Content
	for _, want := range []string{
		"Do not encode product, environment, resource, address, credential, or incident examples as core rules.",
		"current runtime state, model-visible tools, user input, and registered evidence",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("developer prompt missing genericity rule %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{"synthetic_resource_a", "synthetic_endpoint", "blocked_core_rule:0"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("genericity boundary should not depend on fixture examples %q:\n%s", forbidden, content)
		}
	}
}

type fakePromptTool struct {
	name        string
	description string
	readOnly    bool
}

func (t fakePromptTool) Metadata() tooling.ToolMetadata {
	return tooling.ToolMetadata{Name: t.name, Description: t.description}
}

func (t fakePromptTool) InputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t fakePromptTool) OutputSchema() json.RawMessage {
	return json.RawMessage(`{"type":"object"}`)
}

func (t fakePromptTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return t.description
}

func (t fakePromptTool) Prompt(tooling.PromptContext) string {
	return "Use when local file content is needed before answering."
}

func (t fakePromptTool) IsEnabled(tooling.ToolContext) bool {
	return true
}

func (t fakePromptTool) IsReadOnly(json.RawMessage) bool {
	return t.readOnly
}

func (t fakePromptTool) IsDestructive(json.RawMessage) bool {
	return false
}

func (t fakePromptTool) IsConcurrencySafe(json.RawMessage) bool {
	return true
}

func (t fakePromptTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}

func (t fakePromptTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}

func (t fakePromptTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{Content: "ok"}, nil
}
