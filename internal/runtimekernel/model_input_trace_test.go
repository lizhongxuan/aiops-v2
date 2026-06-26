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
	"aiops-v2/internal/taskdepth"
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

func TestModelInputDedupeRepeatedLongUserEvidence(t *testing.T) {
	longEvidence := strings.Join([]string{
		"我用备份系统对主机A做了几次备份，然后选择某个备份记录恢复了主机A。",
		"现在想把主机A加入主机C的监控，并且把主机B当做从节点加入集群，为什么从节点执行加入命令失败？",
		"已有证据：主机B报 timeline 高于主节点，恢复后存在历史分支。",
		"参考流程：",
		"| 步骤 | 命令 | 说明 |",
		"| 准备 | ssh host82.example.internal | 参考环境，不是当前目标 |",
		"| 加入 | run join node_16 --from backup | 文档示例，不是当前目标 |",
		"日志片段：",
		strings.Repeat("service join failed after restore; retry waits for lineage validation\n", 80),
	}, "\n")
	repeatedWithDelta := longEvidence + "\n?"

	result, err := buildPromptInput([]Message{
		{Role: "user", Content: longEvidence},
		{Role: "assistant", Content: "需要确认恢复后的 timeline 和从节点数据目录状态。"},
		{Role: "user", Content: repeatedWithDelta},
	}, promptcompiler.CompiledPrompt{})
	if err != nil {
		t.Fatalf("build prompt input: %v", err)
	}
	joined := joinedModelInputItemContent(result.Items)
	if got := strings.Count(joined, "service join failed after restore"); got > 81 {
		t.Fatalf("expected repeated large evidence body to be deduped, got %d occurrences\n%s", got, joined)
	}
	for _, want := range []string{
		"User evidence capsule",
		"User evidence repeated from previous turn.",
		"digest=sha256:",
		"delta_user_request=?",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("model input missing %q:\n%s", want, joined)
		}
	}
	if result.Trace.ContextDedupe == nil {
		t.Fatal("expected contextDedupe trace")
	}
	if result.Trace.ContextDedupe.RepeatedUserMessageCount != 1 {
		t.Fatalf("repeated count = %d, want 1", result.Trace.ContextDedupe.RepeatedUserMessageCount)
	}
	if result.Trace.ContextDedupe.SavedChars <= 0 {
		t.Fatalf("saved chars = %d, want > 0", result.Trace.ContextDedupe.SavedChars)
	}
	if result.Trace.ContextDedupe.RetainedDeltaChars != 1 {
		t.Fatalf("retained delta chars = %d, want 1", result.Trace.ContextDedupe.RetainedDeltaChars)
	}
}

func TestModelInputTraceWritesContextDedupe(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	input := []*schema.Message{
		{Role: schema.User, Content: "User evidence repeated from previous turn.\ndigest=sha256:abc\nsummary=restore issue\ndelta_user_request=?"},
	}
	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:  "sess-context-dedupe",
		TurnID:     "turn-context-dedupe",
		Iteration:  0,
		ModelInput: input,
		PromptInputTrace: promptinput.PromptInputTrace{
			ContextDedupe: &promptinput.ContextDedupeTrace{
				RepeatedUserMessageCount: 1,
				SavedChars:               3931,
				RetainedDeltaChars:       1,
			},
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	var payload struct {
		ContextDedupe struct {
			RepeatedUserMessageCount int `json:"repeatedUserMessageCount"`
			SavedChars               int `json:"savedChars"`
			RetainedDeltaChars       int `json:"retainedDeltaChars"`
		} `json:"contextDedupe"`
		PromptInputTrace struct {
			ContextDedupe struct {
				RepeatedUserMessageCount int `json:"repeatedUserMessageCount"`
				SavedChars               int `json:"savedChars"`
				RetainedDeltaChars       int `json:"retainedDeltaChars"`
			} `json:"contextDedupe"`
		} `json:"promptInputTrace"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal trace json: %v", err)
	}
	if payload.ContextDedupe.RepeatedUserMessageCount != 1 || payload.ContextDedupe.SavedChars != 3931 || payload.ContextDedupe.RetainedDeltaChars != 1 {
		t.Fatalf("root contextDedupe mismatch: %#v", payload.ContextDedupe)
	}
	if payload.PromptInputTrace.ContextDedupe.RepeatedUserMessageCount != 1 {
		t.Fatalf("promptInputTrace contextDedupe missing: %#v", payload.PromptInputTrace.ContextDedupe)
	}
}

func joinedSchemaMessageContent(messages []*schema.Message) string {
	var b strings.Builder
	for _, msg := range messages {
		if msg == nil {
			continue
		}
		b.WriteString(msg.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func joinedModelInputItemContent(items []promptinput.ModelInputItem) string {
	var b strings.Builder
	for _, item := range items {
		b.WriteString(item.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func TestModelInputDebugTraceRecordsPlanRequirementDecision(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-plan-requirement",
		TurnID:    "turn-plan-requirement",
		Iteration: 0,
		Compiled:  promptcompiler.CompiledPrompt{},
		ModelInput: []*schema.Message{
			{Role: schema.User, Content: "排查一个复杂问题"},
		},
		PlanRequirementDecision: &promptinput.PlanRequirementDecisionTrace{
			Required: true,
			Decision: "soft",
			Reason:   "task_depth_requires_plan",
			Signals:  []string{"plan", "evidence"},
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"planRequirementDecision"`, `"task_depth_requires_plan"`, `"evidence"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace missing %s:\n%s", want, string(data))
		}
	}
}

func TestModelInputDebugTraceIncludesOwnerWriteTrace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	ownerTrace := NewOwnerWriteTrace(OwnerWriteTraceInput{
		Responsibility: OwnerWriteTurnLifecycle,
		Writer:         OwnerRuntimeKernel,
		SessionID:      "sess-owner-trace",
		TurnID:         "turn-owner-trace",
		CreatedAt:      time.Date(2026, 6, 24, 8, 0, 0, 0, time.UTC),
	})
	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-owner-trace",
		TurnID:    "turn-owner-trace",
		Iteration: 1,
		Compiled:  promptcompiler.CompiledPrompt{},
		ModelInput: []*schema.Message{
			{Role: schema.User, Content: "check owner trace"},
		},
		OwnerWriteTraces: []OwnerWriteTrace{ownerTrace},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"ownerWriteTraces"`, `"turn_lifecycle"`, `"runtimekernel.RuntimeKernel"`, `"accepted"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace missing %s:\n%s", want, string(data))
		}
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	for _, want := range []string{"Owner Write Trace", "turn_lifecycle", "runtimekernel.RuntimeKernel"} {
		if !strings.Contains(string(markdown), want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, string(markdown))
		}
	}
}

func TestBuildModelInputToolTraceFieldsIncludesOwnerWriteTrace(t *testing.T) {
	sessionTrace := NewOwnerWriteTrace(OwnerWriteTraceInput{
		Responsibility: OwnerWriteApprovalLedger,
		Writer:         OwnerPendingApproval,
		SessionID:      "sess-owner-fields",
		TurnID:         "turn-owner-fields",
	})
	turnTrace := NewOwnerWriteTrace(OwnerWriteTraceInput{
		Responsibility: OwnerWriteToolResult,
		Writer:         OwnerToolDispatcher,
		SessionID:      "sess-owner-fields",
		TurnID:         "turn-owner-fields",
	})
	fields := buildModelInputToolTraceFields(
		&SessionState{ID: "sess-owner-fields", OwnerWriteTraces: []OwnerWriteTrace{sessionTrace}},
		&TurnSnapshot{ID: "turn-owner-fields", OwnerWriteTraces: []OwnerWriteTrace{turnTrace}},
		"surface-owner",
		"policy-owner",
	)

	if len(fields.OwnerWriteTraces) != 2 {
		t.Fatalf("owner write traces = %#v, want session and turn traces", fields.OwnerWriteTraces)
	}
	if fields.OwnerWriteTraces[0].Responsibility != OwnerWriteApprovalLedger || fields.OwnerWriteTraces[1].Responsibility != OwnerWriteToolResult {
		t.Fatalf("owner write traces = %#v, want session trace followed by turn trace", fields.OwnerWriteTraces)
	}
}

func TestModelInputDebugTraceIncludesToolSurfaceSnapshot(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:                     "sess-tool-surface",
		TurnID:                        "turn-tool-surface",
		Iteration:                     1,
		Compiled:                      promptcompiler.CompiledPrompt{},
		ModelInput:                    []*schema.Message{{Role: schema.User, Content: "inspect tool surface"}},
		VisibleTools:                  []string{"web_search"},
		ToolSurfaceFingerprint:        "tools:abc",
		ToolSurfacePolicySnapshotHash: "policy:def",
		LoadedPacksDelta:              []string{"generic_metrics"},
		ToolSurfaceSnapshot: &promptinput.ToolSurfaceSnapshot{
			HiddenTools: []string{"exec_command"},
			HiddenReasons: map[string][]string{
				"exec_command": {"profile_denied"},
			},
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	var payload struct {
		ToolSurfaceSnapshot *promptinput.ToolSurfaceSnapshot `json:"toolSurfaceSnapshot"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal trace json: %v", err)
	}
	if payload.ToolSurfaceSnapshot == nil {
		t.Fatalf("toolSurfaceSnapshot missing:\n%s", string(data))
	}
	snapshot := payload.ToolSurfaceSnapshot
	if snapshot.Fingerprint != "tools:abc" || snapshot.PolicyHash != "policy:def" {
		t.Fatalf("snapshot hashes = %#v, want tools:abc/policy:def", snapshot)
	}
	if got := strings.Join(snapshot.VisibleTools, ","); got != "web_search" {
		t.Fatalf("visible tools = %q, want web_search", got)
	}
	if got := strings.Join(snapshot.HiddenTools, ","); got != "exec_command" {
		t.Fatalf("hidden tools = %q, want exec_command", got)
	}
	if got := strings.Join(snapshot.HiddenReasons["exec_command"], ","); got != "profile_denied" {
		t.Fatalf("hidden reasons = %q, want profile_denied", got)
	}
	if got := strings.Join(snapshot.LoadedPacksDelta, ","); got != "generic_metrics" {
		t.Fatalf("loaded packs delta = %q, want generic_metrics", got)
	}
}

func TestBuildModelInputToolTraceFieldsIncludesToolSurfaceSnapshot(t *testing.T) {
	fields := buildModelInputToolTraceFields(
		&SessionState{ID: "sess-tool-surface"},
		&TurnSnapshot{
			ID: "turn-tool-surface",
			ToolSurfaceSnapshot: &ToolSurfaceSnapshotRef{
				Fingerprint:        "tools:runtime",
				ToolNames:          []string{"web_search"},
				PolicySnapshotHash: "policy:runtime",
				PolicySnapshot: &tooling.ToolSurfacePolicySnapshot{
					HiddenTools: []tooling.ToolHiddenReason{{Name: "exec_command", Reason: "profile_denied"}},
				},
			},
		},
		"tools:runtime",
		"policy:runtime",
	)

	if fields.ToolSurfaceSnapshot == nil {
		t.Fatal("ToolSurfaceSnapshot = nil, want normalized snapshot")
	}
	if got := strings.Join(fields.ToolSurfaceSnapshot.VisibleTools, ","); got != "web_search" {
		t.Fatalf("visible tools = %q, want web_search", got)
	}
	if got := strings.Join(fields.ToolSurfaceSnapshot.HiddenTools, ","); got != "exec_command" {
		t.Fatalf("hidden tools = %q, want exec_command", got)
	}
	if got := strings.Join(fields.ToolSurfaceSnapshot.HiddenReasons["exec_command"], ","); got != "profile_denied" {
		t.Fatalf("hidden reason = %q, want profile_denied", got)
	}
	if fields.ToolSurfaceSnapshot.PolicyHash != "policy:runtime" {
		t.Fatalf("policy hash = %q, want policy:runtime", fields.ToolSurfaceSnapshot.PolicyHash)
	}
}

func TestPermissionSnapshotTrace(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:  "sess-permission-trace",
		TurnID:     "turn-permission-trace",
		Iteration:  1,
		Compiled:   promptcompiler.CompiledPrompt{},
		ModelInput: []*schema.Message{{Role: schema.User, Content: "check permission trace"}},
		DispatchDecisions: []promptinput.DispatchDecisionTrace{{
			ToolName:               "exec_command",
			ToolCallID:             "call-exec",
			ToolSurfaceFingerprint: "surface-fp-1",
			PermissionSnapshotHash: "permission-fp-1",
			ArgumentsHash:          "sha256:args",
		}},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"dispatchDecisions"`, `"surface-fp-1"`, `"permission-fp-1"`, `"sha256:args"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace missing %s:\n%s", want, string(data))
		}
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
	for _, want := range []string{"exec_command", "grep", "list_mcp_resources", "read_mcp_resource", "skill_read", "skill_search", "tool_search", "web_search"} {
		if !containsString(metrics.VisibleToolNames, want) {
			t.Fatalf("visible tool names = %v, want %q", metrics.VisibleToolNames, want)
		}
	}
	for _, forbidden := range []string{"get_current_model_config", "browse_url", "read_context_artifact", "search_ops_manuals", "coroot.service_metrics"} {
		if containsString(metrics.VisibleToolNames, forbidden) {
			t.Fatalf("visible tool names = %v, should not include deferred/internal tool %q", metrics.VisibleToolNames, forbidden)
		}
	}
	t.Logf("G01 first-turn baseline: prompt=%d toolRegistry=%d visibleTools=%d names=%v", metrics.PromptCharCount, metrics.ToolRegistryCharCount, metrics.VisibleToolCount, metrics.VisibleToolNames)
}

func TestModelInputTraceG01FirstTurnDeferredDirectoryNoSchemas(t *testing.T) {
	metrics := buildG01FirstTurnPromptMetrics(t)
	content := metrics.ToolPromptContent
	for _, want := range []string{
		"## Deferred Tool Directory",
		"coroot_metrics",
		"runtime_config",
		"select=required",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool prompt missing deferred directory marker %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{
		"coroot.service_metrics",
		"get_current_model_config:",
		"appId",
		"fromTimestamp",
		`"properties"`,
		`"required"`,
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("initial prompt leaked deferred schema/tool detail %q:\n%s", forbidden, content)
		}
	}
	if !deferredDirectoryContainsPack(metrics.PromptInputTrace.DeferredToolDirectory, "coroot_metrics") {
		t.Fatalf("prompt input trace deferred directory = %#v, want coroot_metrics", metrics.PromptInputTrace.DeferredToolDirectory)
	}
	if len(metrics.PromptInputTrace.DeferredToolDirectory) == 0 {
		t.Fatal("prompt input trace should record deferred tool families")
	}
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

func TestModelInputTraceCarriesPromptSectionsAndContextUsage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-sections",
		TurnID:    "turn-sections",
		Iteration: 2,
		Compiled: promptcompiler.CompiledPrompt{
			PromptSections: []promptcompiler.PromptSectionTrace{{
				ID:             "protocol.state",
				Kind:           "dynamic",
				Source:         "protocol-state",
				Hash:           "sha256:abc",
				Bytes:          32,
				TokensEstimate: 8,
			}},
			ChangedSections: []promptcompiler.ChangedPromptSection{{
				ID:          "protocol.state",
				Reason:      promptcompiler.PromptSectionChangeProtocolStateChanged,
				CurrentHash: "sha256:abc",
			}},
		},
		ModelInput: []*schema.Message{
			{Role: schema.System, Content: "system"},
			{Role: schema.User, Content: "user"},
		},
		PromptInputTrace: promptinput.PromptInputTrace{
			ContextUsage: promptinput.ContextUsage{
				MaxContextTokens:     1000,
				ReservedOutputTokens: 200,
				EstimatedInputTokens: 20,
				Categories: []promptinput.ContextUsageCategory{{
					Name:           "messages",
					Bytes:          4,
					TokensEstimate: 1,
				}},
				TopContributors: []promptinput.ContextContributor{{
					Kind:           "messages",
					ID:             "user",
					Bytes:          4,
					TokensEstimate: 1,
					Action:         "keep_inline",
				}},
			},
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"promptSections"`, `"changedSections"`, `"protocol.state"`, `"contextUsage"`, `"topContributors"`, `"modelInputStats"`, `"promptBytes"`, `"messages"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("trace json missing %q:\n%s", want, jsonText)
		}
	}
	var payload struct {
		ModelInputStats struct {
			PromptBytes  int `json:"promptBytes"`
			MessageCount int `json:"messageCount"`
		} `json:"modelInputStats"`
		PromptInputTrace struct {
			PromptSections []struct {
				ID             string `json:"id"`
				Bytes          int    `json:"bytes"`
				TokensEstimate int    `json:"tokensEstimate"`
			} `json:"promptSections"`
			ContextUsage struct {
				TopContributors []struct {
					Kind           string `json:"kind"`
					ID             string `json:"id"`
					Bytes          int    `json:"bytes"`
					TokensEstimate int    `json:"tokensEstimate"`
				} `json:"topContributors"`
			} `json:"contextUsage"`
		} `json:"promptInputTrace"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal trace json: %v", err)
	}
	if payload.ModelInputStats.PromptBytes != len("system")+len("user") || payload.ModelInputStats.MessageCount != 2 {
		t.Fatalf("modelInputStats = %#v, want prompt bytes and message count", payload.ModelInputStats)
	}
	if len(payload.PromptInputTrace.PromptSections) != 1 ||
		payload.PromptInputTrace.PromptSections[0].ID != "protocol.state" ||
		payload.PromptInputTrace.PromptSections[0].Bytes != 32 ||
		payload.PromptInputTrace.PromptSections[0].TokensEstimate != 8 {
		t.Fatalf("prompt section trace mismatch: %#v", payload.PromptInputTrace.PromptSections)
	}
	if len(payload.PromptInputTrace.ContextUsage.TopContributors) != 1 ||
		payload.PromptInputTrace.ContextUsage.TopContributors[0].ID != "user" {
		t.Fatalf("top contributors mismatch: %#v", payload.PromptInputTrace.ContextUsage.TopContributors)
	}
}

func TestTraceIncludesToolDiscoveryEvents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:                     "sess-tool-discovery",
		TurnID:                        "turn-tool-discovery",
		Iteration:                     1,
		ToolSurfaceFingerprint:        "tools:abc",
		ToolSurfacePolicySnapshotHash: "policy:def",
		LoadedToolsDelta:              []string{"generic.metrics.read"},
		LoadedPacksDelta:              []string{"generic_metrics"},
		ToolSearchEvents: []promptinput.ToolSearchTraceEvent{{
			Mode:       "search",
			Query:      "metrics read",
			MatchCount: 1,
			Matches:    []string{"generic_metrics"},
		}},
		ToolSelectionEvents: []promptinput.ToolSelectionTraceEvent{{
			Source:      "tool_search.select",
			Reason:      "need read-only metrics evidence",
			LoadedTools: []string{"generic.metrics.read"},
			LoadedPacks: []string{"generic_metrics"},
		}},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{
		`"toolSurfaceFingerprint": "tools:abc"`,
		`"toolSurfacePolicySnapshotHash": "policy:def"`,
		`"loadedToolsDelta"`,
		`"loadedPacksDelta"`,
		`"toolSearchEvents"`,
		`"toolSelectionEvents"`,
		`"generic.metrics.read"`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("trace json missing %q:\n%s", want, jsonText)
		}
	}
	markdown, err := os.ReadFile(modelTraceMarkdownPath(path))
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	for _, want := range []string{"## Tool Discovery Trace", "tool_surface_fingerprint", "generic.metrics.read", "tool_search.select"} {
		if !strings.Contains(string(markdown), want) {
			t.Fatalf("trace markdown missing %q:\n%s", want, string(markdown))
		}
	}
}

func TestTraceIncludesSkillDiscoveryEvents(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:         "sess-skill-discovery",
		TurnID:            "turn-skill-discovery",
		Iteration:         1,
		SkillIndexHash:    "sha256:index",
		LoadedSkillsDelta: []string{"synthetic.triage"},
		SkillSearchEvents: []promptinput.SkillSearchTraceEvent{{
			Mode:       "search",
			Query:      "diagnose log",
			MatchCount: 1,
			Matches:    []string{"synthetic.triage"},
		}},
		SkillReadEvents: []promptinput.SkillReadTraceEvent{{
			Skill:  "synthetic.triage",
			Source: "skill_read",
			Reason: "need bounded checklist",
			Range:  "0:128",
		}},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{
		`"skillIndexHash": "sha256:index"`,
		`"loadedSkillsDelta"`,
		`"skillSearchEvents"`,
		`"skillReadEvents"`,
		`"synthetic.triage"`,
	} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("trace json missing %q:\n%s", want, jsonText)
		}
	}
}

func TestTraceIncludesRejectedToolCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-rejected",
		TurnID:    "turn-rejected",
		RejectedToolCalls: []promptinput.RejectedToolCallTraceEvent{{
			ToolName:             "generic.hidden.read",
			ErrorType:            "tool_hidden_by_policy",
			Reason:               "tool is hidden from current prompt surface",
			RequiredAction:       "request permission, switch mode, or choose a safer alternative",
			SuggestedSearchQuery: "read-only generic evidence",
			TurnID:               "turn-rejected",
			ToolCallID:           "call-hidden",
		}},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"rejectedToolCalls"`, `"tool_hidden_by_policy"`, `"requiredAction"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace json missing %q:\n%s", want, string(data))
		}
	}
}

func TestTraceIncludesParallelDispatchGroups(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-parallel",
		TurnID:    "turn-parallel",
		ParallelDispatchGroups: []promptinput.ParallelDispatchTraceGroup{{
			GroupID:  "turn-parallel-iter-0-parallel-0",
			Decision: "parallel",
			Reasons:  []string{"read_only", "non_destructive", "concurrency_safe", "no_approval_required", "shared_resource_key"},
			ToolCalls: []promptinput.ParallelDispatchToolCall{{
				ToolCallID:        "call-1",
				ToolName:          "generic.metrics.read",
				SharedResourceKey: "host:alpha",
			}},
		}},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"parallelDispatchGroups"`, `"read_only"`, `"non_destructive"`, `"concurrency_safe"`, `"no_approval_required"`, `"shared_resource_key"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace json missing %q:\n%s", want, string(data))
		}
	}
}

func TestTraceIncludesAgentSchedulingState(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:      "sess-agent-scheduling",
		TurnID:         "turn-agent-scheduling",
		AgentIndexHash: "agent-index-sha256:synthetic",
		AgentDelegationDecision: &promptinput.AgentDelegationDecisionTrace{
			Action:         "spawn_new",
			Reason:         "independent_evidence_surface",
			CandidateAgent: "synthetic.explorer",
		},
		AgentAssignmentLint: []promptinput.AgentAssignmentLintTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "pass",
		}},
		AgentParallelTraceGroups: []promptinput.AgentParallelTraceGroup{{
			MissionID:      "synthetic-mission",
			RequestedCount: 2,
			SpawnedInTurn:  []string{"synthetic-worker-1", "synthetic-worker-2"},
		}},
		ResourceLocks: []promptinput.ResourceLockTrace{{
			AgentID: "synthetic-worker-1",
			Action:  "acquired",
			Key: promptinput.ResourceLockKeyTrace{
				ResourceType:  "generic_resource",
				ResourceID:    "synthetic-resource",
				OperationKind: "read",
			},
		}},
		AgentFinalGate: &promptinput.AgentFinalGateDecisionTrace{
			Action:        "require_wait",
			PendingAgents: []string{"synthetic-worker-2"},
		},
		AgentNotifications: []promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "completed",
		}},
		VerificationAgentReport: &promptinput.VerificationAgentReportTrace{
			Status:       "PASS",
			Summary:      "verified bounded evidence",
			EvidenceRefs: []string{"artifact://synthetic/verification"},
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"agentIndexHash"`, `"agentDelegationDecision"`, `"spawn_new"`, `"agentAssignmentLint"`, `"agentParallelTraceGroups"`, `"resourceLocks"`, `"agentFinalGate"`, `"agentNotifications"`, `"verificationAgentReport"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace json missing %q:\n%s", want, string(data))
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read trace markdown: %v", err)
	}
	for _, want := range []string{"### Agent Scheduling", "delegation decision: spawn_new", "resource lock acquired", "verification agent: PASS"} {
		if !strings.Contains(string(markdown), want) {
			t.Fatalf("trace markdown missing %q:\n%s", want, string(markdown))
		}
	}
}

func TestTraceIncludesFailedToolSummaries(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-failed-summary",
		TurnID:    "turn-failed-summary",
		FailedToolSummaries: []promptinput.FailedToolSummary{{
			Tool:          "generic.metrics.read",
			FailureClass:  "timeout",
			Attempts:      2,
			FinalStatus:   "failed",
			SafeToRetry:   true,
			ModelGuidance: "Retry only with the same arguments and same tool surface, or choose another read-only evidence source.",
		}},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	for _, want := range []string{`"failedToolSummaries"`, `"failureClass": "timeout"`, `"safeToRetry": true`, `"modelGuidance"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("trace json missing %q:\n%s", want, string(data))
		}
	}
}

func TestAppendModelTraceResponseRecordsOutputUsageAndDuration(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID: "sess-response",
		TurnID:    "turn-response",
		Iteration: 1,
		ModelInput: []*schema.Message{
			schema.UserMessage("show trace"),
		},
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}

	response := &schema.Message{
		Role:    schema.Assistant,
		Content: "模型输出 api_key=ak-output-123",
		ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{
			PromptTokens:     21,
			CompletionTokens: 8,
			TotalTokens:      29,
		}},
	}
	stats := ModelStreamStats{
		FirstDeltaMs: 123,
		StreamMs:     234,
		DeltaCount:   3,
		OutputChars:  8,
	}
	if err := appendModelTraceResponseFile(path, "llm-1", response, 456*time.Millisecond, nil, stats); err != nil {
		t.Fatalf("append response: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace json: %v", err)
	}
	var payload struct {
		Output       string `json:"output"`
		DurationMs   int64  `json:"duration_ms"`
		FirstDeltaMs int64  `json:"first_delta_ms"`
		StreamMs     int64  `json:"stream_ms"`
		DeltaCount   int    `json:"delta_count"`
		OutputChars  int    `json:"output_chars"`
		Usage        struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		LLMRequests []struct {
			ID           string `json:"id"`
			Output       string `json:"output"`
			DurationMs   int64  `json:"duration_ms"`
			FirstDeltaMs int64  `json:"first_delta_ms"`
			StreamMs     int64  `json:"stream_ms"`
			DeltaCount   int    `json:"delta_count"`
			OutputChars  int    `json:"output_chars"`
			Usage        struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		} `json:"llmRequests"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal trace json: %v", err)
	}
	if len(payload.LLMRequests) != 1 || payload.LLMRequests[0].ID != "llm-1" {
		t.Fatalf("llmRequests = %#v, want appended llm request", payload.LLMRequests)
	}
	if !strings.Contains(payload.Output, "[REDACTED]") || strings.Contains(payload.Output, "ak-output-123") {
		t.Fatalf("output was not redacted: %q", payload.Output)
	}
	if payload.DurationMs != 456 || payload.LLMRequests[0].DurationMs != 456 {
		t.Fatalf("duration_ms root/request = %d/%d, want 456", payload.DurationMs, payload.LLMRequests[0].DurationMs)
	}
	if payload.FirstDeltaMs != 123 || payload.StreamMs != 234 || payload.DeltaCount != 3 || payload.OutputChars != 8 {
		t.Fatalf("root stream stats = first=%d stream=%d deltas=%d chars=%d, want 123/234/3/8", payload.FirstDeltaMs, payload.StreamMs, payload.DeltaCount, payload.OutputChars)
	}
	if payload.LLMRequests[0].FirstDeltaMs != 123 || payload.LLMRequests[0].StreamMs != 234 || payload.LLMRequests[0].DeltaCount != 3 || payload.LLMRequests[0].OutputChars != 8 {
		t.Fatalf("request stream stats = first=%d stream=%d deltas=%d chars=%d, want 123/234/3/8", payload.LLMRequests[0].FirstDeltaMs, payload.LLMRequests[0].StreamMs, payload.LLMRequests[0].DeltaCount, payload.LLMRequests[0].OutputChars)
	}
	if payload.Usage.TotalTokens != 29 || payload.LLMRequests[0].Usage.PromptTokens != 21 || payload.LLMRequests[0].Usage.CompletionTokens != 8 {
		t.Fatalf("usage root/request = %#v/%#v, want token usage", payload.Usage, payload.LLMRequests[0].Usage)
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
		Metadata:    map[string]string{"taskDepth": "simple_read"},
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

func TestRunTurnRecordsModelTraceResponseUsage(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	model := &sequentialLoopModel{responses: []*schema.Message{{
		Role:    schema.Assistant,
		Content: "诊断完成 token=secret-output",
		ResponseMeta: &schema.ResponseMeta{Usage: &schema.TokenUsage{
			PromptTokens:     30,
			CompletionTokens: 12,
			TotalTokens:      42,
		}},
	}}}
	registry := tooling.NewRegistry()
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-runtime-response-trace",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		HostID:      "server-local",
		TurnID:      "turn-runtime-response-trace",
		Input:       "排查 Redis 是否异常",
		Metadata:    map[string]string{"taskDepth": "simple_read"},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-runtime-response-trace")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) == 0 {
		t.Fatalf("missing current turn iterations: %#v", session)
	}
	tracePath := session.CurrentTurn.Iterations[0].ModelInputTraceFile
	data, err := os.ReadFile(tracePath)
	if err != nil {
		t.Fatalf("read runtime trace %q: %v", tracePath, err)
	}
	var payload struct {
		Output       string `json:"output"`
		DurationMs   int64  `json:"duration_ms"`
		FirstDeltaMs int64  `json:"first_delta_ms"`
		StreamMs     int64  `json:"stream_ms"`
		DeltaCount   int    `json:"delta_count"`
		OutputChars  int    `json:"output_chars"`
		Usage        struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			TotalTokens      int `json:"total_tokens"`
		} `json:"usage"`
		LLMRequests []struct {
			ID           string `json:"id"`
			Output       string `json:"output"`
			DurationMs   int64  `json:"duration_ms"`
			FirstDeltaMs int64  `json:"first_delta_ms"`
			StreamMs     int64  `json:"stream_ms"`
			DeltaCount   int    `json:"delta_count"`
			OutputChars  int    `json:"output_chars"`
			Usage        struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		} `json:"llmRequests"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal runtime trace: %v", err)
	}
	if len(payload.LLMRequests) != 1 {
		t.Fatalf("llmRequests len = %d, want 1\n%s", len(payload.LLMRequests), string(data))
	}
	if payload.Usage.TotalTokens != 42 || payload.LLMRequests[0].Usage.PromptTokens != 30 {
		t.Fatalf("usage root/request = %#v/%#v, want model usage", payload.Usage, payload.LLMRequests[0].Usage)
	}
	if payload.DurationMs <= 0 || payload.LLMRequests[0].DurationMs <= 0 {
		t.Fatalf("duration root/request = %d/%d, want positive", payload.DurationMs, payload.LLMRequests[0].DurationMs)
	}
	if payload.FirstDeltaMs <= 0 || payload.LLMRequests[0].FirstDeltaMs <= 0 {
		t.Fatalf("first_delta_ms root/request = %d/%d, want positive", payload.FirstDeltaMs, payload.LLMRequests[0].FirstDeltaMs)
	}
	if payload.StreamMs <= 0 || payload.LLMRequests[0].StreamMs <= 0 {
		t.Fatalf("stream_ms root/request = %d/%d, want positive", payload.StreamMs, payload.LLMRequests[0].StreamMs)
	}
	if payload.DeltaCount <= 0 || payload.LLMRequests[0].DeltaCount <= 0 || payload.OutputChars <= 0 || payload.LLMRequests[0].OutputChars <= 0 {
		t.Fatalf("stream counts root/request = deltas %d/%d chars %d/%d, want positive", payload.DeltaCount, payload.LLMRequests[0].DeltaCount, payload.OutputChars, payload.LLMRequests[0].OutputChars)
	}
	if !strings.Contains(payload.Output, "[REDACTED]") || strings.Contains(payload.Output, "secret-output") {
		t.Fatalf("output was not redacted: %q", payload.Output)
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
		Metadata:    map[string]string{"taskDepth": "simple_read"},
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

func TestModelInputDebugTraceRecordsTaskDepthAndReasoningEffort(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE", "1")
	t.Setenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR", dir)

	path, err := writeModelInputDebugTrace(ModelInputDebugTraceRequest{
		SessionID:  "sess-depth",
		TurnID:     "turn-depth",
		Iteration:  0,
		Compiled:   promptcompiler.CompiledPrompt{},
		ModelInput: []*schema.Message{{Role: schema.User, Content: "排查异常"}},
		TaskDepth:  taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresPlan: true, RequiresEvidence: true},
		EvidenceCoverage: &EvidenceCoverageDecision{
			Action:             "continue_gathering",
			Coverage:           0.5,
			RequiredDimensions: []string{"plan_context", "tool_evidence"},
			CoveredDimensions:  []string{"plan_context"},
			MissingDimensions:  []string{"tool_evidence"},
			Reasons:            []string{"missing_coverage_dimension"},
		},
		ReasoningEffort: "high",
		AnswerStyle:     "concise",
	})
	if err != nil {
		t.Fatalf("write trace: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read trace: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal trace: %v", err)
	}
	metadata := payload["metadata"].(map[string]any)
	if metadata["taskDepth.level"] != "investigation" || metadata["reasoningEffort.configured"] != "high" || metadata["answerStyle.configured"] != "concise" {
		t.Fatalf("metadata = %#v, want taskDepth and reasoning", metadata)
	}
	trace := payload["promptInputTrace"].(map[string]any)
	if trace["taskDepth"] == nil || trace["evidenceCoverage"] == nil {
		t.Fatalf("promptInputTrace missing taskDepth/evidenceCoverage: %#v", trace)
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

func TestEnrichCompileContextAddsWebLearnOfficialSourceGuidance(t *testing.T) {
	ctx := enrichCompileContext(promptcompiler.CompileContext{}, SessionTypeWorkspace, "", map[string]string{
		"aiops.weblearn.enabled":      "true",
		"aiops.weblearn.sourcePolicy": "official_first",
	}, fixedModelInputTraceTime())
	content := promptSectionsContentForModelInputTraceTest(ctx.ExtraSections)
	for _, want := range []string{
		"WebLearn Source Policy",
		"web_search/browse_url",
		"official documentation",
		"version-specific documentation",
		"source URL",
		"Do not use exec_command",
		"官方来源优先",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("WebLearn prompt sections missing %q:\n%s", want, content)
		}
	}
}

func TestEnrichCompileContextInjectsOnlyApplicableWebLearnEvidence(t *testing.T) {
	longExcerpt := strings.Repeat("Redis official docs explain latency doctor output. ", 40)
	ctx := enrichCompileContext(promptcompiler.CompileContext{}, SessionTypeWorkspace, "", map[string]string{
		"aiops.weblearn.enabled":      "true",
		"aiops.weblearn.sourcePolicy": "official_first",
		"aiops.weblearn.evidence": `[
			{
				"kind": "external_knowledge",
				"query": "redis 7.2 latency doctor official docs",
				"sourceUrl": "https://redis.io/docs/latest/commands/latency-doctor/",
				"sourceTitle": "LATENCY DOCTOR",
				"sourceKind": "official_docs",
				"product": "Redis",
				"version": "7.2",
				"relevantExcerpt": "` + longExcerpt + `",
				"applicability": "applies to Redis 7.2 latency subsystem command semantics",
				"confidence": "high"
			},
			{
				"kind": "external_knowledge",
				"query": "redis random forum answer",
				"sourceUrl": "https://example.com/forum",
				"sourceTitle": "forum answer",
				"sourceKind": "community_post",
				"product": "Redis",
				"confidence": "high"
			},
			{
				"kind": "environment_fact",
				"query": "host redis version",
				"sourceUrl": "https://redis.io/",
				"sourceKind": "official_docs",
				"product": "Redis",
				"confidence": "low"
			}
		]`,
	}, fixedModelInputTraceTime())

	content := promptSectionsContentForModelInputTraceTest(ctx.ExtraSections)
	for _, want := range []string{
		"WebLearn Evidence",
		"LATENCY DOCTOR",
		"https://redis.io/docs/latest/commands/latency-doctor/",
		"applies to Redis 7.2",
		"Redis official docs explain latency doctor output",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("WebLearn evidence prompt section missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{
		"forum answer",
		"environment_fact",
		strings.Repeat("Redis official docs explain latency doctor output. ", 20),
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("WebLearn evidence prompt section contains forbidden %q:\n%s", forbidden, content)
		}
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
	ToolPromptContent     string
	PromptInputTrace      promptinput.PromptInputTrace
}

func buildG01FirstTurnPromptMetrics(t *testing.T) firstTurnPromptMetrics {
	t.Helper()

	registry := tooling.NewRegistry()
	for _, tool := range []promptcompiler.Tool{
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "exec_command", Description: "Execute a local terminal command on the selected host", Layer: tooling.ToolLayerCore, RiskLevel: tooling.ToolRiskHigh, Mutating: true}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "get_current_model_config", Description: "Read currently configured LLM provider and model", Layer: tooling.ToolLayerDeferred, Pack: "runtime_config", DeferByDefault: true, RiskLevel: tooling.ToolRiskMedium}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "web_search", Description: "Search the web for current information with source URLs", Layer: tooling.ToolLayerCore, Pack: "public_web", AlwaysLoad: true, RiskLevel: tooling.ToolRiskMedium}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "browse_url", Description: "Fetch a specific http or https URL as readable page text", Layer: tooling.ToolLayerDeferred, Pack: "public_web", DeferByDefault: true, RiskLevel: tooling.ToolRiskMedium}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "grep", Description: "Search local files and logs", Layer: tooling.ToolLayerCore, Pack: "filesystem_search", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "list_mcp_resources", Description: "List readable MCP resources", Layer: tooling.ToolLayerCore, Pack: "mcp_resource", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "read_mcp_resource", Description: "Read an MCP resource", Layer: tooling.ToolLayerCore, Pack: "mcp_resource", AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "skill_search", Description: "Search available skills", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "skill_read", Description: "Read a selected skill body", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "read_context_artifact", Description: "Read externalized context artifacts", Layer: tooling.ToolLayerConditional, Pack: "context_artifact", RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "tool_search", Description: "Search available operational tools by name, description, domain, and governance metadata", Layer: tooling.ToolLayerCore, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithMetadata(tooling.ToolMetadata{Name: "search_ops_manuals", Description: "Search verified ops manuals for an operations request and return an auditable decision", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true, RiskLevel: tooling.ToolRiskLow}),
		staticTraceToolWithSchema(tooling.ToolMetadata{
			Name:           "coroot.service_metrics",
			Description:    "Read service metric summaries from an external observability source",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "coroot_metrics",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:     "metrics",
				ResourceTypes:      []string{"service", "resource"},
				OperationKinds:     []string{"read", "query"},
				RequiresHealthyMCP: true,
				RequiresSelect:     true,
				MCPServerID:        "coroot",
				SchemaBudgetClass:  "on_demand",
			},
		}, json.RawMessage(`{"type":"object","properties":{"appId":{"type":"string"},"fromTimestamp":{"type":"integer"}},"required":["appId"]}`)),
	} {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("Register(%s) error = %v", tool.Metadata().Name, err)
		}
	}
	tools := registry.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{})
	deferredCatalog := registry.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{IncludeDeferredCatalog: true})
	ctx := promptcompiler.CompileContext{
		SessionType:         "host",
		Mode:                "chat",
		AssembledTools:      tools,
		DeferredToolCatalog: deferredCatalog,
		MCPHealthSnapshot:   map[string]string{"coroot": "unavailable"},
	}
	compiler := promptcompiler.NewCompiler()
	compiled, err := compiler.Compile(ctx)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	promptBuild, err := buildPromptInput([]Message{{
		Role:    "user",
		Content: "G01: 排查 ERP 订单提交异常，先收集证据，不要执行变更",
	}}, compiled)
	if err != nil {
		t.Fatalf("build prompt input error = %v", err)
	}

	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return firstTurnPromptMetrics{
		PromptCharCount:       modelInputItemCharCount(promptBuild.Items),
		ToolRegistryCharCount: len(compiled.Tools.Content),
		VisibleToolCount:      len(names),
		VisibleToolNames:      names,
		ToolPromptContent:     compiled.Tools.Content,
		PromptInputTrace:      promptBuild.Trace,
	}
}

func staticTraceTool(name, description string, risk tooling.ToolRiskLevel, mutating bool) promptcompiler.Tool {
	return staticTraceToolWithMetadata(tooling.ToolMetadata{
		Name:        name,
		Description: description,
		RiskLevel:   risk,
		Mutating:    mutating,
	})
}

func staticTraceToolWithMetadata(meta tooling.ToolMetadata) promptcompiler.Tool {
	return &tooling.StaticTool{
		Meta: meta,
		ReadOnlyFunc: func(json.RawMessage) bool {
			return !meta.Mutating
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return meta.Mutating
		},
	}
}

func staticTraceToolWithSchema(meta tooling.ToolMetadata, inputSchema json.RawMessage) promptcompiler.Tool {
	tool := staticTraceToolWithMetadata(meta).(*tooling.StaticTool)
	tool.InputSchemaData = inputSchema
	return tool
}

func promptSectionsContentForModelInputTraceTest(sections []promptcompiler.PromptSection) string {
	var b strings.Builder
	for _, section := range sections {
		if section.Title != "" {
			b.WriteString(section.Title)
			b.WriteString("\n")
		}
		b.WriteString(section.Content)
		b.WriteString("\n")
	}
	return b.String()
}

func deferredDirectoryContainsPack(entries []promptcompiler.DeferredToolDirectoryEntry, pack string) bool {
	for _, entry := range entries {
		if entry.Pack == pack {
			return true
		}
	}
	return false
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

func modelInputItemCharCount(items []promptinput.ModelInputItem) int {
	total := 0
	for _, item := range items {
		total += len(item.Content)
		if item.ToolResult != nil {
			total += len(item.ToolResult.Content)
		}
	}
	return total
}

func fixedModelInputTraceTime() time.Time {
	return time.Unix(1700000000, 0).UTC()
}
