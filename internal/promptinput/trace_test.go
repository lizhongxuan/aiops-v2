package promptinput

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestPromptInputTraceJSONAndMarkdownExplainSources(t *testing.T) {
	result, err := Builder{}.Build(BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
			Tools:  promptcompiler.ToolPromptSet{Content: "tool index"},
			Dynamic: promptcompiler.DynamicPromptDelta{
				ProtocolState: promptcompiler.ProtocolPromptState{
					Items: []promptcompiler.ProtocolPromptItem{
						{Kind: "plan", ID: "step-1", Status: "in_progress", Text: "inspect logs"},
					},
				},
			},
		},
		History: []Message{
			{Role: "user", Content: "triage"},
			{Role: "tool", Content: "log output", ToolResult: &ToolResult{ToolCallID: "call-1", Content: "log output"}},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}

	data, err := json.Marshal(result.Trace)
	if err != nil {
		t.Fatalf("marshal trace: %v", err)
	}
	jsonTrace := string(data)
	for _, want := range []string{
		`"source":"stable_prompt"`,
		`"semanticRole":"tool_index"`,
		`"providerRole":"system"`,
		`"source":"protocol_state"`,
		`"semanticRole":"tool_result"`,
	} {
		if !strings.Contains(jsonTrace, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonTrace)
		}
	}

	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{
		"# Prompt Input Trace",
		"stable_prompt",
		"tool_index",
		"protocol_state",
		"tool_result",
		"provider",
	} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
}
