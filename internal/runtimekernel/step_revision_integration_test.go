package runtimekernel

import (
	"context"
	"encoding/json"
	"os"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modeltrace"
	"aiops-v2/internal/tooling"
)

func TestStepRevisionSkillReadProducesNextStepWithoutMutatingTurnAssembly(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-skill-read", Type: "function",
			Function: schema.FunctionCall{Name: "skill_read", Arguments: `{"name":"synthetic.triage"}`},
		}}),
		schema.AssistantMessage("synthetic triage skill applied", nil),
	}}
	readPayload := json.RawMessage(`{"loadedSkills":[{"name":"synthetic.triage","source":"skill_read","reason":"bounded triage","range":{"offset":0,"limit":128},"hash":"sha256:synthetic-triage","riskCeiling":"read_only","allowedTools":["skill_read"]}]}`)
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "skill_read", Description: "Read a selected skill body", Layer: tooling.ToolLayerCore,
			AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{PermissionScope: "read"},
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)}, Modes: []string{string(ModeChat), string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: string(readPayload), Display: &tooling.ToolDisplayPayload{Type: "skill_read", Data: readPayload}}, nil
		},
	}
	registry := tooling.NewRegistry()
	if err := registry.Register(toolDef); err != nil {
		t.Fatalf("Register(skill_read) error = %v", err)
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, model)
	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "sess-step-revision-skill", SessionType: SessionTypeHost, Mode: ModeChat,
		TurnID: "turn-step-revision-skill", Input: "load and apply the synthetic triage skill",
		Metadata: map[string]string{"enableTool": "skill_read"},
	})
	if err != nil {
		t.Fatalf("RunTurn() error = %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result = %#v, want completed", result)
	}
	session := kernel.sessions.Get("sess-step-revision-skill")
	if session == nil || session.CurrentTurn == nil || session.CurrentTurn.TurnAssembly == nil || len(session.CurrentTurn.Iterations) < 2 {
		t.Fatalf("skill turn state = %#v, want two step iterations", session)
	}
	first := session.CurrentTurn.Iterations[0].StepReference
	second := session.CurrentTurn.Iterations[1].StepReference
	if first == nil || second == nil {
		t.Fatalf("skill step references = first:%#v second:%#v", first, second)
	}
	assemblyHash := session.CurrentTurn.TurnAssembly.Hash
	if first.TurnAssemblyHash != assemblyHash || second.TurnAssemblyHash != assemblyHash {
		t.Fatalf("skill load mutated turn assembly: first=%q second=%q assembly=%q", first.TurnAssemblyHash, second.TurnAssemblyHash, assemblyHash)
	}
	if second.Transition.PreviousHash != first.StepHash || second.Transition.NextHash != second.StepHash || first.StepHash == second.StepHash {
		t.Fatalf("skill step chain = first:%#v second:%#v", first, second)
	}
	if !stepTransitionHasKind(second.Transition, StepRevisionKindSkillLoaded) || !containsString(second.Facts.LoadedSkillRefs, "skill:synthetic.triage") {
		t.Fatalf("skill revision = %#v facts=%#v activation=%#v toolResults=%#v compileTools=%v", second.Transition.Revisions, second.Facts, session.SkillActivation, session.CurrentTurn.Iterations[0].ToolResults, toolNames(compiler.contexts[0].AssembledTools))
	}
}

func TestStepRevisionTraceCarriesHashOnlyTransition(t *testing.T) {
	step := mustFreezeRuntimeStepContextForTest(t, validRuntimeStepContextForHashTest())
	facts := mustFreezeStepRevisionFactsForTest(t, StepRevisionFacts{TurnAssemblyHash: step.TurnAssemblyHash})
	ref, err := BuildStepReference(nil, step, facts)
	if err != nil {
		t.Fatalf("BuildStepReference() error = %v", err)
	}
	path, err := writeRuntimeStepTrace(modeltrace.Config{Enabled: true, RootDir: t.TempDir()}, step, RuntimeTraceDebugRequest{}, &ref)
	if err != nil {
		t.Fatalf("writeRuntimeStepTrace() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile(trace) error = %v", err)
	}
	var payload struct {
		StepContext struct {
			StepReference *StepReference `json:"stepReference"`
		} `json:"stepContext"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("decode trace: %v", err)
	}
	if payload.StepContext.StepReference == nil || payload.StepContext.StepReference.Transition.NextHash != step.Hash {
		t.Fatalf("trace step reference = %#v", payload.StepContext.StepReference)
	}
}
