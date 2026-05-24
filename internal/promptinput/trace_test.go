package promptinput

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/tooling"
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

func TestPromptInputTraceIncludesOpsContextBudgetMetrics(t *testing.T) {
	result, err := Builder{}.Build(BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
		},
		Tools: []promptcompiler.Tool{
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "search_ops_manuals"}},
			&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "host_read"}},
		},
		Memories:              []MemoryItem{{ID: "mem-1", Text: "prior target"}},
		OpsContextCapsule:     "flow: flow-1\ncurrent_target: redis",
		SessionFactCount:      5,
		LettaHintCount:        2,
		DroppedContextReasons: []string{"letta_hint_limit", "artifact_ref_only"},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	if result.Trace.OpsContextCapsuleChars == 0 ||
		result.Trace.SessionFactCount != 5 ||
		result.Trace.LettaHintCount != 2 ||
		result.Trace.MemoryItemCount != 1 ||
		!containsString(result.Trace.VisibleOpsManualTools, "search_ops_manuals") ||
		!containsString(result.Trace.DroppedContextReasons, "artifact_ref_only") {
		t.Fatalf("trace metrics = %#v", result.Trace)
	}
	markdown := RenderMarkdown(result.Trace)
	for _, want := range []string{"ops_context_capsule_chars", "session_fact_count", "visible_ops_manual_tools", "artifact_ref_only"} {
		if !strings.Contains(markdown, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, markdown)
		}
	}
}

func TestPromptInputTraceCarriesContextGovernance(t *testing.T) {
	req := BuildRequest{
		Compiled: promptcompiler.CompiledPrompt{
			System: promptcompiler.SystemPrompt{Content: "system layer"},
		},
		ContextGovernance: []ContextGovernanceTraceItem{{
			Layer:        "L4",
			Kind:         "context.compaction.started",
			Message:      "compacting context",
			Budget:       map[string]int{"autoCompactThreshold": 167000, "blockingLimit": 177000},
			ReferenceIDs: []string{"ref-1", "ref-2"},
			RetryAttempt: 1,
			RetryMax:     3,
		}},
	}
	result, err := Builder{}.Build(req)
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	req.ContextGovernance[0].Budget["autoCompactThreshold"] = 1
	req.ContextGovernance[0].ReferenceIDs[0] = "mutated"

	got := result.Trace.ContextGovernance
	if len(got) != 1 {
		t.Fatalf("context governance length = %d, want 1", len(got))
	}
	if got[0].Layer != "L4" ||
		got[0].Kind != "context.compaction.started" ||
		got[0].Budget["autoCompactThreshold"] != 167000 ||
		got[0].ReferenceIDs[0] != "ref-1" ||
		got[0].RetryAttempt != 1 ||
		got[0].RetryMax != 3 {
		t.Fatalf("context governance trace = %#v", got[0])
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
