package runtimekernel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
)

func TestModelInputDebugTraceWritesJSONAndMarkdownWhenEnabled(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	compiled := promptcompiler.CompiledPrompt{
		System: promptcompiler.SystemPrompt{Content: "system layer"},
		Dynamic: promptcompiler.DynamicPromptDelta{
			Content: "dynamic prompt delta",
		},
	}
	input := []*schema.Message{
		{Role: schema.System, Content: "system layer", Extra: map[string]any{"semantic_role": "system"}},
		{Role: schema.User, Content: "user asks"},
	}

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:  "sess-1",
		TurnID:     "turn-1",
		Iteration:  2,
		Metadata:   map[string]string{"eval.caseId": "case-runtime"},
		Compiled:   compiled,
		ModelInput: input,
		VisibleTools: []string{
			"read_file",
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	if path == "" {
		t.Fatal("expected trace path when enabled")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("trace json should be readable: %v", err)
	}
	if payload["sessionId"] != "sess-1" || payload["turnId"] != "turn-1" || payload["caseId"] != "case-runtime" {
		t.Fatalf("trace metadata missing: %#v", payload)
	}
	if !strings.Contains(string(data), "dynamic prompt delta") {
		t.Fatalf("trace json missing prompt delta: %s", string(data))
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	if !strings.Contains(string(markdown), "## Model Input") || !strings.Contains(string(markdown), "dynamic prompt delta") {
		t.Fatalf("markdown trace missing visual sections:\n%s", string(markdown))
	}
}

func TestModelInputDebugTraceWritesPromptInputTraceAndDiff(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	diff := promptinput.DiffTrace(
		promptinput.PromptInputTrace{Items: []promptinput.TraceItem{
			{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "pending", Content: "inspect"},
		}},
		promptinput.PromptInputTrace{Items: []promptinput.TraceItem{
			{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "completed", Content: "inspect"},
			{Source: "conversation", SemanticRole: "tool_result", ID: "call-1", Content: "ok"},
		}},
	)
	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Iteration: 2,
		PromptInputTrace: promptinput.PromptInputTrace{Items: []promptinput.TraceItem{
			{Source: "protocol_state", SemanticRole: "plan", ID: "step-1", Status: "completed", Content: "inspect"},
		}},
		PromptInputDiff: &diff,
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	if !strings.Contains(string(data), `"promptInputTrace"`) || !strings.Contains(string(data), `protocol_state`) {
		t.Fatalf("json trace missing prompt input trace:\n%s", string(data))
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	if !strings.Contains(string(markdown), "## Prompt Input Trace") {
		t.Fatalf("markdown trace missing prompt input trace:\n%s", string(markdown))
	}
	diffMarkdown, err := os.ReadFile(filepath.Join(filepath.Dir(path), "input.diff.md"))
	if err != nil {
		t.Fatalf("read input.diff.md: %v", err)
	}
	if !strings.Contains(string(diffMarkdown), "tool_result") || !strings.Contains(string(diffMarkdown), "completed") {
		t.Fatalf("diff markdown missing semantic delta:\n%s", string(diffMarkdown))
	}
}

func TestModelInputDebugTraceDisabledByDefault(t *testing.T) {
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", t.TempDir())

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-1",
		TurnID:    "turn-1",
	})
	if err != nil {
		t.Fatalf("disabled trace should not error: %v", err)
	}
	if path != "" {
		t.Fatalf("disabled trace path = %q, want empty", path)
	}
}

func TestEnrichCompileContextSetsAgentKindFromSessionType(t *testing.T) {
	hostCtx := enrichCompileContext(promptcompiler.CompileContext{}, SessionTypeHost, "host-1", fixedModelInputTraceTime())
	if hostCtx.AgentKind != promptcompiler.AgentKindWorker {
		t.Fatalf("host AgentKind = %q, want worker", hostCtx.AgentKind)
	}

	workspaceCtx := enrichCompileContext(promptcompiler.CompileContext{}, SessionTypeWorkspace, "", fixedModelInputTraceTime())
	if workspaceCtx.AgentKind != promptcompiler.AgentKindPlanner {
		t.Fatalf("workspace AgentKind = %q, want planner", workspaceCtx.AgentKind)
	}
}

func fixedModelInputTraceTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
