package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestUnexpectedStateGateDetectsGenericToolResultStatus(t *testing.T) {
	result := ToolResult{
		ToolCallID: "call-read",
		Content:    `{"status":"conflict","resourceType":"synthetic_resource","resourceId":"resource-a","summary":"observed state differs from planned state"}`,
	}
	signals := DetectUnexpectedStateFromToolResult("synthetic.read", result)
	if len(signals) != 1 {
		t.Fatalf("signals = %#v, want one", signals)
	}
	if signals[0].Status != "conflict" || signals[0].ResourceID != "resource-a" {
		t.Fatalf("signal = %#v, want conflict for resource-a", signals[0])
	}
}

func TestUnexpectedStateGateBlocksMutationButAllowsPlanTool(t *testing.T) {
	signals := []UnexpectedStateSignal{{Status: "precondition_failed", ResourceType: "synthetic_resource", ResourceID: "resource-a"}}
	mutation := EvaluateUnexpectedStateGate(signals, ToolCall{Name: "synthetic.write"}, tooling.ToolMetadata{Name: "synthetic.write", Mutating: true})
	if mutation.Action != UnexpectedStateActionBlockMutation {
		t.Fatalf("mutation decision = %#v, want block", mutation)
	}
	planTool := EvaluateUnexpectedStateGate(signals, ToolCall{Name: "update_plan"}, tooling.ToolMetadata{Name: "update_plan", Mutating: true, Layer: tooling.ToolLayerMutation})
	if planTool.Action == UnexpectedStateActionBlockMutation {
		t.Fatalf("plan tool decision = %#v, want non-blocking plan update", planTool)
	}
	readOnly := EvaluateUnexpectedStateGate(signals, ToolCall{Name: "synthetic.read"}, tooling.ToolMetadata{Name: "synthetic.read", RiskLevel: tooling.ToolRiskLow})
	if readOnly.Action == UnexpectedStateActionBlockMutation {
		t.Fatalf("read-only decision = %#v, want inspect allowed", readOnly)
	}
}

func TestToolDispatcherUnexpectedStateBlocksMutation(t *testing.T) {
	emitter := &testMockEventEmitter{}
	executor := &mockToolExecutor{result: "mutated"}
	lookup := &mockToolLookup{tools: map[string]mockToolEntry{
		"synthetic.write": {
			desc: ToolDescriptor{Metadata: tooling.ToolMetadata{
				Name:      "synthetic.write",
				Mutating:  true,
				RiskLevel: tooling.ToolRiskMedium,
			}},
			executor: executor,
		},
	}}
	dispatcher := NewToolDispatcher(lookup, nil, emitter).
		WithUnexpectedStateSignals([]UnexpectedStateSignal{{Status: "drift", ResourceType: "synthetic_resource", ResourceID: "resource-a"}})

	result := dispatcher.Dispatch(context.Background(), "sess-unexpected", "turn-unexpected", ToolCall{
		ID:        "call-write",
		Name:      "synthetic.write",
		Arguments: json.RawMessage(`{"resourceId":"resource-a"}`),
	}, SessionTypeWorkspace, ModeExecute)

	if !result.Blocked && !strings.Contains(result.Error, "unexpected_state") && !strings.Contains(result.Error, "drift") {
		t.Fatalf("result = %#v, want unexpected state denial", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want blocked before execution", executor.calls)
	}
}
