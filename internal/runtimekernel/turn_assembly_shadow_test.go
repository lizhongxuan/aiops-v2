package runtimekernel

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/tooling"
)

func TestTurnAssemblyShadowBuildsOnceBeforePromptAndProvider(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("先读取状态。", []schema.ToolCall{{
			ID: "call-read", Type: "function",
			Function: schema.FunctionCall{Name: "host_read", Arguments: `{}`},
		}}),
		schema.AssistantMessage("读取完成。", nil),
	}}
	toolDef := &tooling.StaticTool{
		Meta:       tooling.ToolMetadata{Name: "host_read", Description: "Read host state"},
		Visibility: tooling.Visibility{SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeInspect)}},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "healthy"}, nil
		},
	}
	observer := &turnAssemblyRecordingObserver{}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	kernel.observer = observer
	traceRoot := t.TempDir()
	kernel.debugConfig = func(context.Context) RuntimeDebugConfig {
		return RuntimeDebugConfig{ModelInputTrace: true, ModelInputTraceRoot: traceRoot}
	}

	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-turn-assembly-order", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-assembly-order", Input: "读取主机状态",
	}); err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}

	wantPrefix := []string{"turn_assembly_built", "prompt_compiled", "provider_request_started"}
	if len(observer.stages) < len(wantPrefix) {
		t.Fatalf("stages = %#v, want prefix %#v", observer.stages, wantPrefix)
	}
	for index, want := range wantPrefix {
		if observer.stages[index] != want {
			t.Fatalf("stages = %#v, want strict prefix %#v", observer.stages, wantPrefix)
		}
	}
	if got := countTurnAssemblyStage(observer.stages, "turn_assembly_built"); got != 1 {
		t.Fatalf("turn_assembly_built count = %d, want 1 for a multi-iteration turn", got)
	}
	if len(model.inputs) != 2 {
		t.Fatalf("provider calls = %d, want 2", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-turn-assembly-order")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.TurnAssembly == nil {
		t.Fatal("persisted turn snapshot is missing TurnAssembly")
	}
	if err := session.CurrentTurn.TurnAssembly.Validate(); err != nil {
		t.Fatalf("persisted TurnAssembly.Validate() error = %v", err)
	}
	shadow := session.CurrentTurn.TurnAssemblyShadow
	if shadow == nil || shadow.AssemblyHash != session.CurrentTurn.TurnAssembly.Hash {
		t.Fatalf("TurnAssemblyShadow = %#v, want persisted assembly hash", shadow)
	}
	if shadow.LegacySpecHash == "" || shadow.ProjectedSpecHash == "" || len(shadow.FieldDiffs) == 0 {
		t.Fatalf("TurnAssemblyShadow = %#v, want legacy/projected field comparison", shadow)
	}
	for _, diff := range shadow.FieldDiffs {
		if diff.Field == "" || diff.LegacyHash == "" || diff.ProjectedHash == "" {
			t.Fatalf("field diff exposes values or misses hashes: %#v", diff)
		}
	}
	paths, err := filepath.Glob(filepath.Join(modeltrace.TraceDocumentV2Directory(traceRoot, "sess-turn-assembly-order", "turn-assembly-order"), "iteration-*.json"))
	if err != nil || len(paths) != 2 {
		t.Fatalf("trace paths = %#v, err = %v, want two iterations", paths, err)
	}
	data, err := os.ReadFile(paths[0])
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	var tracePayload struct {
		TurnAssembly                map[string]any `json:"turnAssembly"`
		LegacyAgentAssemblySnapshot map[string]any `json:"legacyAgentAssemblySnapshot"`
		TurnAssemblyShadow          map[string]any `json:"turnAssemblyShadow"`
	}
	if err := json.Unmarshal(data, &tracePayload); err != nil {
		t.Fatalf("json.Unmarshal(trace) error = %v", err)
	}
	if tracePayload.TurnAssembly["hash"] != session.CurrentTurn.TurnAssembly.Hash ||
		tracePayload.LegacyAgentAssemblySnapshot["specHash"] == nil ||
		tracePayload.TurnAssemblyShadow["assemblyHash"] != session.CurrentTurn.TurnAssembly.Hash {
		t.Fatalf("trace assembly projection missing: %#v", tracePayload)
	}
}

func TestTurnAssemblyShadowAdmissionFailureMakesZeroProviderCalls(t *testing.T) {
	model := &sequentialLoopModel{}
	observer := &turnAssemblyRecordingObserver{}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	kernel.observer = observer

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-turn-assembly-invalid", SessionType: SessionTypeHost,
		Mode: ModeInspect, TurnID: "turn-assembly-invalid", Input: "inspect",
		Metadata: map[string]string{"aiops.intent.kind": "secret-canary-invalid"},
	})
	if err == nil {
		t.Fatal("RunTurn() error = nil, want admission failure")
	}
	if strings.Contains(err.Error(), "secret-canary-invalid") {
		t.Fatalf("RunTurn() leaked raw admission value: %v", err)
	}
	if len(model.inputs) != 0 {
		t.Fatalf("provider calls = %d, want 0", len(model.inputs))
	}
	if countTurnAssemblyStage(observer.stages, "prompt_compiled") != 0 || countTurnAssemblyStage(observer.stages, "provider_request_started") != 0 {
		t.Fatalf("stages after admission failure = %#v, want no prompt/provider stage", observer.stages)
	}
}

type turnAssemblyRecordingObserver struct {
	NoopObserver
	stages []string
}

func (o *turnAssemblyRecordingObserver) StartStage(ctx context.Context, attrs StageSpanAttrs) (context.Context, ObservedSpan) {
	o.stages = append(o.stages, attrs.Stage)
	return normalizeObserverContext(ctx), noopObservedSpan{}
}

func countTurnAssemblyStage(stages []string, want string) int {
	count := 0
	for _, stage := range stages {
		if stage == want {
			count++
		}
	}
	return count
}
