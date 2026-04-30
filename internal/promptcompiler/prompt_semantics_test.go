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

func TestToolIndexIncludesUsageExamplesAndFailureHandling(t *testing.T) {
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
	for _, want := range []string{"Usage policy:", "Example:", "Failure handling:"} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool index missing %q:\n%s", want, content)
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
