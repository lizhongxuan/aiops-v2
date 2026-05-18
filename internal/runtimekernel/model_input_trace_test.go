package runtimekernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
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

func TestModelInputDebugTraceWritesDiagnosticTrace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Iteration: 1,
		DiagnosticTrace: diagnostics.DiagnosticTrace{
			ScopeHash:        "scope-host-redis",
			ScopeSummary:     "server-local redis",
			Hypotheses:       []string{"redis down"},
			ObservedEvidence: []string{"PING failed"},
			MissingEvidence:  []string{"lsof blocked"},
			ToolFailures: []diagnostics.ToolFailure{{
				ToolName: "exec_command",
				Semantic: diagnostics.ToolFailureCommandNotAllowed,
				Detail:   "command not allowed",
				Critical: true,
			}},
			Confidence: diagnostics.ConfidenceLow,
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	if !strings.Contains(string(data), `"diagnosticTrace"`) || !strings.Contains(string(data), `"scopeHash": "scope-host-redis"`) {
		t.Fatalf("json trace missing diagnostic trace:\n%s", string(data))
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	if !strings.Contains(string(markdown), "## Diagnostic Trace") || !strings.Contains(string(markdown), "command_not_allowed") {
		t.Fatalf("markdown trace missing diagnostic trace:\n%s", string(markdown))
	}
}

func TestRunTurnPopulatesDiagnosticTraceInDebugTrace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)
	t.Setenv("AIOPS_DIAGNOSTIC_PROTOCOL", "1")

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("诊断完成", nil),
	}}
	registry := tooling.NewRegistry()
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-runtime-diagnostic-trace",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		HostID:      "server-local",
		TurnID:      "turn-runtime-diagnostic-trace",
		Input:       "排查 Redis 是否异常",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-runtime-diagnostic-trace")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) == 0 {
		t.Fatalf("missing current turn iterations: %#v", session)
	}
	tracePath := session.CurrentTurn.Iterations[0].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read runtime trace %q: %v", tracePath, err)
	}
	for _, want := range []string{`"diagnosticTrace"`, `"scopeSummary": "host:server-local"`, `"confidence": "low"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("runtime trace missing %q:\n%s", want, string(data))
		}
	}
	markdown, err := os.ReadFile(modelTraceMarkdownPath(tracePath))
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "## Diagnostic Trace") || !strings.Contains(string(markdown), "host:server-local") {
		t.Fatalf("runtime markdown trace missing diagnostic section:\n%s", string(markdown))
	}
}

func TestRunTurnInjectsRuntimeEnvironmentContextInDebugTrace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("诊断完成", nil),
	}}
	registry := tooling.NewRegistry()
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-runtime-env-context",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		HostID:      "host-a",
		TurnID:      "turn-runtime-env-context",
		Input:       "当前排查主机A上的 Docker Redis，容器 aiops-redis，镜像 redis:7-alpine，端口 36379",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-runtime-env-context")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) == 0 {
		t.Fatalf("missing current turn iterations: %#v", session)
	}
	tracePath := session.CurrentTurn.Iterations[0].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read runtime trace %q: %v", tracePath, err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal runtime trace: %v", err)
	}
	promptPayload, _ := payload["prompt"].(map[string]any)
	dynamicPrompt, _ := promptPayload["dynamic"].(string)
	for _, want := range []string{
		"Runtime Environment Context",
		"ContextIntent: switch",
		"CurrentFocus:",
		"host=host-a",
		"target=redis",
		"deployment=docker",
		"aiops-redis",
	} {
		if !strings.Contains(dynamicPrompt, want) {
			t.Fatalf("runtime prompt.dynamic missing %q:\n%s", want, dynamicPrompt)
		}
	}
}

func TestBuildRuntimeDiagnosticTraceCarriesRuntimeEnvironmentContext(t *testing.T) {
	trace := buildRuntimeDiagnosticTrace("turn-env", &SessionState{ID: "sess-env", Type: SessionTypeHost, HostID: "host-a"}, TurnRequest{
		SessionType: SessionTypeHost,
		HostID:      "host-a",
	}, promptcompiler.CompileContext{ExtraSections: []promptcompiler.PromptSection{{
		Title:   "Runtime Environment Context",
		Content: "CurrentFocus host=host-a target=redis deployment=docker version=7-alpine",
	}}})

	if trace.ScopeSummary != "host:host-a" {
		t.Fatalf("scope summary = %q, want host:host-a", trace.ScopeSummary)
	}
	if len(trace.ObservedEvidence) != 1 || !strings.Contains(trace.ObservedEvidence[0], "deployment=docker") {
		t.Fatalf("observed evidence = %#v, want runtime environment context", trace.ObservedEvidence)
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
	hostCtx := enrichCompileContext(promptcompiler.CompileContext{}, SessionTypeHost, "host-1", nil, fixedModelInputTraceTime())
	if hostCtx.AgentKind != promptcompiler.AgentKindWorker {
		t.Fatalf("host AgentKind = %q, want worker", hostCtx.AgentKind)
	}

	workspaceCtx := enrichCompileContext(promptcompiler.CompileContext{}, SessionTypeWorkspace, "", nil, fixedModelInputTraceTime())
	if workspaceCtx.AgentKind != promptcompiler.AgentKindPlanner {
		t.Fatalf("workspace AgentKind = %q, want planner", workspaceCtx.AgentKind)
	}
}

func TestApplyRuntimeFeatureFlagsCanDisableDiagnosticProtocolPromptOnly(t *testing.T) {
	ctx := applyRuntimeFeatureFlags(promptcompiler.CompileContext{}, featureflag.Flags{DiagnosticProtocol: false})
	if !ctx.DisableDiagnosticProtocol {
		t.Fatalf("DisableDiagnosticProtocol = false, want true")
	}
	ctx = applyRuntimeFeatureFlags(promptcompiler.CompileContext{DisableDiagnosticProtocol: true}, featureflag.Flags{DiagnosticProtocol: true})
	if ctx.DisableDiagnosticProtocol {
		t.Fatalf("DisableDiagnosticProtocol should be cleared when diagnostic protocol flag is enabled")
	}
}

func fixedModelInputTraceTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
