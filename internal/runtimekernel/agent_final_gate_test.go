package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/tooling"
)

func TestAgentFinalGateBlocksPendingWorkerClaim(t *testing.T) {
	decision := EvaluateRuntimeAgentFinalGate(
		"synthetic-worker-1 confirmed bounded summary",
		[]promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "running",
			Summary: "bounded summary",
		}},
	)
	if decision.Action != "require_wait" {
		t.Fatalf("action = %q, want require_wait: %#v", decision.Action, decision)
	}
}

func TestBuildDeterministicIncompleteFinal(t *testing.T) {
	got := buildDeterministicIncompleteFinal(incompleteFinalInput{
		ConfirmedFacts: []string{
			"用户描述主机A由 pgBackRest 恢复",
			"用户描述主机B作为从节点加入失败",
		},
		MissingEvidence: []string{
			"主机B完整错误输出",
			"主机A和主机B的 pg_control timeline",
		},
		LikelyDirection: "timeline 或恢复残留配置不一致",
		Confidence:      "low",
	})
	for _, want := range []string{"还不能给最终结论", "已确认", "仍缺少", "下一步只读检查"} {
		if !strings.Contains(got, want) {
			t.Fatalf("incomplete final missing %q: %s", want, got)
		}
	}
	if strings.Contains(got, "置信度") || strings.Contains(got, "confidence") {
		t.Fatalf("incomplete final must not expose confidence labels: %s", got)
	}
}

func TestBuildDeterministicIncompleteFinalCompactsRawStructuredEvidenceAndTranslatesToolNames(t *testing.T) {
	got := buildDeterministicIncompleteFinal(incompleteFinalInput{
		ConfirmedFacts: []string{
			`{"categoryCounts":{"application":25,"control-plane":14,"monitoring":10},"evidenceRefs":["ev-services"]}`,
			`{"evidenceRefs":["ev-incidents"],"incidents":[{"application":"rabbitmq-server"}]}`,
		},
		MissingEvidence: []string{
			"read_mcp_resource 未成功返回证据；不能当作已检查结果。",
			"read_mcp_resource 未成功返回证据；不能当作已检查结果。",
		},
		ReadOnlyChecks: []string{
			"重新读取或替代核对 read_mcp_resource 对应的只读证据。",
		},
	})

	for _, want := range []string{
		"Coroot 服务概览已返回结构化证据。",
		"Coroot 异常事件已返回结构化证据。",
		"读取 MCP 资源 未成功返回证据；不能当作已检查结果。",
		"重新读取或替代核对 读取 MCP 资源 对应的只读证据。",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("incomplete final missing %q: %s", want, got)
		}
	}
	for _, unwanted := range []string{`"categoryCounts"`, "read_mcp_resource"} {
		if strings.Contains(got, unwanted) {
			t.Fatalf("incomplete final exposed %q: %s", unwanted, got)
		}
	}
	if count := strings.Count(got, "读取 MCP 资源 未成功返回证据"); count != 1 {
		t.Fatalf("duplicate missing evidence count = %d, want 1: %s", count, got)
	}
}

func TestEvaluateFinalMessageBoundaryConstrainDoesNotRequestRewriteForConfidenceOnly(t *testing.T) {
	decision := evaluateFinalMessageBoundary(finalMessageBoundaryInput{
		Text:                   "基于当前证据，更可能是 timeline 分叉。",
		FinalEvidenceAction:    string(FinalEvidenceActionDowngrade),
		EvidenceCoverageAction: "continue_gathering",
		RequiresEvidence:       true,
	})
	if decision.Action != FinalMessageBoundaryConstrain {
		t.Fatalf("action=%q, want constrain", decision.Action)
	}
	if decision.EvidenceBoundary != "limited" {
		t.Fatalf("boundary=%q, want limited", decision.EvidenceBoundary)
	}
	if decision.Retry {
		t.Fatal("confidence/evidence boundary downgrade must not force model rewrite")
	}
}

func TestAgentFinalGateDoesNotLetPendingStatusDisclosureBypassFacts(t *testing.T) {
	decision := EvaluateRuntimeAgentFinalGate(
		"synthetic-worker-1 is still running and not confirmed",
		[]promptinput.AgentNotificationTrace{{
			AgentID: "synthetic-worker-1",
			Status:  "running",
		}},
	)
	if decision.Action != "require_wait" {
		t.Fatalf("action = %q, want require_wait: %#v", decision.Action, decision)
	}
}

func TestAgentFinalGateRequiresWaitForEveryNonTerminalWorkerStatus(t *testing.T) {
	for _, status := range []string{"planned", "spawning", "running", "waiting", "approval_required", "blocked", "queued", "superseded", "unknown"} {
		decision := EvaluateRuntimeAgentFinalGate("", []promptinput.AgentNotificationTrace{{AgentID: "worker-" + status, Status: status}})
		if decision.Action != "require_wait" {
			t.Errorf("status %q action = %q, want require_wait", status, decision.Action)
		}
	}
}

func TestRunTurnAgentFinalGateRequiresWaitToolBeforeFinal(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-spawn", Type: "function", Function: schema.FunctionCall{Name: "spawn_host_agent", Arguments: `{}`},
		}}),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-fake", Type: "function", Function: schema.FunctionCall{Name: "read_status", Arguments: `{}`},
		}}),
		schema.AssistantMessage("所有 worker 已完成。", nil),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-wait", Type: "function", Function: schema.FunctionCall{Name: "wait_host_agents", Arguments: `{}`},
		}}),
		schema.AssistantMessage("worker 结果已聚合。", nil),
	}}
	toolResult := func(schemaVersion, status string) tooling.ToolResult {
		content := `{"schemaVersion":"` + schemaVersion + `","children":[{"childAgentId":"worker-1","status":"` + status + `"}]}`
		return tooling.ToolResult{
			Content: content,
			Display: &tooling.ToolDisplayPayload{Type: "hostops.child-status", Data: json.RawMessage(content)},
		}
	}
	toolDef := func(name, schemaVersion, status string) tooling.Tool {
		return &tooling.StaticTool{
			Meta: tooling.ToolMetadata{Name: name, Description: name, RecordEvidence: true},
			Visibility: tooling.Visibility{
				SessionTypes: []string{string(SessionTypeWorkspace)},
				Modes:        []string{string(ModeExecute)},
			},
			ReadOnlyFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				return toolResult(schemaVersion, status), nil
			},
		}
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{
		toolDef("spawn_host_agent", "aiops.hostops.child/v1", "spawning"),
		toolDef("read_status", "aiops.hostops.wait/v1", "completed"),
		toolDef("wait_host_agents", "aiops.hostops.wait/v1", "completed"),
	}, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-worker-gate", SessionType: SessionTypeWorkspace, Mode: ModeExecute,
		TurnID: "turn-worker-gate", Input: "delegate and aggregate",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "worker 结果已聚合。" {
		t.Fatalf("output = %q, want post-wait final", result.Output)
	}
	if len(model.inputs) != 5 {
		t.Fatalf("model calls = %d, want retry plus wait before final", len(model.inputs))
	}
	session := kernel.sessions.Get("session-worker-gate")
	if session == nil || session.CurrentTurn == nil || len(session.CurrentTurn.Iterations) < 4 {
		t.Fatalf("turn iterations = %#v, want typed worker retry path", session)
	}
	if got := session.CurrentTurn.Iterations[3].ToolCalls; len(got) != 1 || got[0].Name != "wait_host_agents" {
		t.Fatalf("post-gate tool calls = %#v, want wait_host_agents", got)
	}
}

func TestRunTurnAgentFinalGateTerminalWorkerFailureCannotVerify(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID: "call-spawn-failed", Type: "function", Function: schema.FunctionCall{Name: "spawn_host_agent", Arguments: `{}`},
		}}),
		schema.AssistantMessage("全部成功。", nil),
	}}
	content := `{"schemaVersion":"aiops.hostops.child/v1","children":[{"childAgentId":"worker-failed","status":"failed"}]}`
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "spawn_host_agent", Description: "spawn worker", RecordEvidence: true},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace)}, Modes: []string{string(ModeExecute)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: content, Display: &tooling.ToolDisplayPayload{Data: json.RawMessage(content)}}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)
	if _, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID: "session-worker-failed", SessionType: SessionTypeWorkspace, Mode: ModeExecute,
		TurnID: "turn-worker-failed", Input: "delegate work",
	}); err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	session := kernel.sessions.Get("session-worker-failed")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("current turn is missing")
	}
	var contract FinalContract
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Type != agentstate.TurnItemTypeFinalResponse {
			continue
		}
		var payload struct {
			FinalContract FinalContract `json:"finalContract"`
		}
		if json.Unmarshal(item.Payload.Data, &payload) == nil {
			contract = payload.FinalContract
		}
	}
	if contract.Status != FinalContractStatusPartial || !containsFinalRuntimeCode(contract.Limitations, "non_completed_worker_evidence") {
		t.Fatalf("final contract = %#v, want typed partial worker failure", contract)
	}
}
