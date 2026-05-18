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

func TestModelInputDebugTraceRecordsPromptSizeMetrics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	compiled := promptcompiler.CompiledPrompt{
		Tools: promptcompiler.ToolPromptSet{Content: "# Tool Index\n\n- read_file: Read files."},
	}
	input := []*schema.Message{
		{Role: schema.System, Content: "system prompt"},
		{Role: schema.User, Content: "user asks"},
	}

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:    "sess-metrics",
		TurnID:       "turn-metrics",
		Iteration:    1,
		Compiled:     compiled,
		ModelInput:   input,
		VisibleTools: []string{"read_file", "tool_search"},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	var payload struct {
		PromptCharCount       int      `json:"promptCharCount"`
		ToolRegistryCharCount int      `json:"toolRegistryCharCount"`
		VisibleToolCount      int      `json:"visibleToolCount"`
		VisibleTools          []string `json:"visibleTools"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal trace json: %v", err)
	}
	if payload.PromptCharCount != len("system prompt")+len("user asks") {
		t.Fatalf("promptCharCount = %d, want model input content length", payload.PromptCharCount)
	}
	if payload.ToolRegistryCharCount != len(compiled.Tools.Content) {
		t.Fatalf("toolRegistryCharCount = %d, want %d", payload.ToolRegistryCharCount, len(compiled.Tools.Content))
	}
	if payload.VisibleToolCount != 2 {
		t.Fatalf("visibleToolCount = %d, want 2", payload.VisibleToolCount)
	}
	if got := strings.Join(payload.VisibleTools, ","); got != "read_file,tool_search" {
		t.Fatalf("visibleTools = %q, want read_file,tool_search", got)
	}
}

func TestModelInputTraceG01FirstTurnBaselineMetrics(t *testing.T) {
	metrics := buildG01FirstTurnPromptMetrics(t)
	if metrics.PromptCharCount == 0 {
		t.Fatal("prompt char count should be recorded")
	}
	if metrics.ToolRegistryCharCount == 0 {
		t.Fatal("tool registry char count should be recorded")
	}
	if metrics.VisibleToolCount == 0 {
		t.Fatal("visible tool count should be recorded")
	}
	for _, want := range []string{"exec_command", "tool_search", "search_ops_manuals"} {
		if !containsString(metrics.VisibleToolNames, want) {
			t.Fatalf("visible tool names = %v, want %q", metrics.VisibleToolNames, want)
		}
	}
	t.Logf("G01 first-turn baseline: prompt=%d toolRegistry=%d visibleTools=%d names=%v", metrics.PromptCharCount, metrics.ToolRegistryCharCount, metrics.VisibleToolCount, metrics.VisibleToolNames)
}

func TestModelInputTraceG01FirstTurnP0PromptSizeBudget(t *testing.T) {
	metrics := buildG01FirstTurnPromptMetrics(t)
	const maxFirstTurnPromptChars = 25000
	const maxFirstTurnToolRegistryChars = 10000
	if metrics.PromptCharCount > maxFirstTurnPromptChars {
		t.Fatalf("prompt char count = %d, want <= %d", metrics.PromptCharCount, maxFirstTurnPromptChars)
	}
	if metrics.ToolRegistryCharCount > maxFirstTurnToolRegistryChars {
		t.Fatalf("tool registry char count = %d, want <= %d", metrics.ToolRegistryCharCount, maxFirstTurnToolRegistryChars)
	}
}

func TestModelInputTraceG01FirstTurnFinalTargetBudget(t *testing.T) {
	metrics := buildG01FirstTurnPromptMetrics(t)
	const maxFirstTurnPromptChars = 25000
	const maxFirstTurnToolRegistryChars = 6000
	const maxFirstTurnVisibleTools = 8
	if metrics.PromptCharCount > maxFirstTurnPromptChars {
		t.Fatalf("prompt char count = %d, want <= %d", metrics.PromptCharCount, maxFirstTurnPromptChars)
	}
	if metrics.ToolRegistryCharCount > maxFirstTurnToolRegistryChars {
		t.Fatalf("tool registry char count = %d, want <= %d", metrics.ToolRegistryCharCount, maxFirstTurnToolRegistryChars)
	}
	if metrics.VisibleToolCount > maxFirstTurnVisibleTools {
		t.Fatalf("visible tool count = %d, want <= %d; tools=%v", metrics.VisibleToolCount, maxFirstTurnVisibleTools, metrics.VisibleToolNames)
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

type firstTurnPromptMetrics struct {
	PromptCharCount       int
	ToolRegistryCharCount int
	VisibleToolCount      int
	VisibleToolNames      []string
}

func buildG01FirstTurnPromptMetrics(t *testing.T) firstTurnPromptMetrics {
	t.Helper()

	tools := []promptcompiler.Tool{
		staticTraceTool("exec_command", "Execute a local terminal command on the selected host", tooling.ToolRiskHigh, true),
		staticTraceTool("get_current_model_config", "Read currently configured LLM provider and model", tooling.ToolRiskMedium, false),
		staticTraceTool("web_search", "Search the web for current information with source URLs", tooling.ToolRiskMedium, false),
		staticTraceTool("browse_url", "Fetch a specific http or https URL as readable page text", tooling.ToolRiskMedium, false),
		staticTraceTool("tool_search", "Search available operational tools by name, description, domain, and governance metadata", tooling.ToolRiskLow, false),
		staticTraceTool("search_ops_manuals", "Search verified ops manuals for an operations request and return an auditable decision", tooling.ToolRiskLow, false),
	}
	ctx := promptcompiler.CompileContext{
		SessionType:    "host",
		Mode:           "inspect",
		AssembledTools: tools,
	}
	compiler := promptcompiler.NewCompiler()
	compiled, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput, err := compiler.CompileForEino(ctx)
	if err != nil {
		t.Fatalf("CompileForEino() error = %v", err)
	}
	modelInput = append(modelInput, schema.UserMessage("G01: 排查 ERP 订单提交异常，先收集证据，不要执行变更"))

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return firstTurnPromptMetrics{
		PromptCharCount:       schemaMessageCharCount(modelInput),
		ToolRegistryCharCount: len(compiled.Tools.Content),
		VisibleToolCount:      len(names),
		VisibleToolNames:      names,
	}
}

func staticTraceTool(name, description string, risk tooling.ToolRiskLevel, mutating bool) promptcompiler.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: description,
			RiskLevel:   risk,
			Mutating:    mutating,
		},
		ReadOnlyFunc: func(json.RawMessage) bool {
			return !mutating
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return mutating
		},
	}
}

func schemaMessageCharCount(messages []*schema.Message) int {
	total := 0
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		total += len(msg.Content)
	}
	return total
}

func fixedModelInputTraceTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
