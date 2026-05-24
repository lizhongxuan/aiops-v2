package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/hooks"
	"aiops-v2/internal/planning"
	"aiops-v2/internal/tooling"
)

func TestRunTurn_WritesAgentItemsForNoToolTurn(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("final answer", nil),
		},
	}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-no-tool",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-no-tool",
		Input:       "hello",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-agent-items-no-tool")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	want := []agentstate.TurnItemType{
		agentstate.TurnItemTypeUserMessage,
		agentstate.TurnItemTypeModelCall,
		agentstate.TurnItemTypeFinalAnswer,
	}
	if got := agentItemTypes(session.CurrentTurn.AgentItems); !sameTurnItemTypes(got, want) {
		t.Fatalf("agent item types = %v, want %v", got, want)
	}
	for _, item := range session.CurrentTurn.AgentItems {
		if item.Status != agentstate.ItemStatusCompleted {
			t.Fatalf("agent item %s status = %q, want completed", item.ID, item.Status)
		}
	}
}

func TestRunTurn_WritesAgentItemsForToolTurn(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"path":"/tmp"}`,
					},
				},
			}),
			schema.AssistantMessage("final answer", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Inspect disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeExecute)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok:" + string(input)}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-tool",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-tool",
		Input:       "inspect disks",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-agent-items-tool")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	want := []agentstate.TurnItemType{
		agentstate.TurnItemTypeUserMessage,
		agentstate.TurnItemTypeModelCall,
		agentstate.TurnItemTypeToolCall,
		agentstate.TurnItemTypeToolResult,
		agentstate.TurnItemTypeModelCall,
		agentstate.TurnItemTypeFinalAnswer,
	}
	if got := agentItemTypes(session.CurrentTurn.AgentItems); !sameTurnItemTypes(got, want) {
		t.Fatalf("agent item types = %v, want %v", got, want)
	}
}

func TestRunTurn_PreservesAssistantTextBeforeToolCalls(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("我先在本机读取当前工作路径。", []schema.ToolCall{
				{
					ID:   "call-pwd",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "exec_readonly",
						Arguments: `{"cmd":"pwd"}`,
					},
				},
			}),
			schema.AssistantMessage("我再看下时间。", []schema.ToolCall{
				{
					ID:   "call-date",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "exec_readonly",
						Arguments: `{"cmd":"date"}`,
					},
				},
			}),
			schema.AssistantMessage("检查完成。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_readonly",
			Description: "Run a read-only command",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok:" + string(input)}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-assistant-tool-text",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-agent-items-assistant-tool-text",
		Input:       "查看路径和时间",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-agent-items-assistant-tool-text")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	items := session.CurrentTurn.AgentItems
	firstText := findAgentItemBySummary(items, agentstate.TurnItemTypeFinalAnswer, "我先在本机读取当前工作路径。")
	firstTool := findAgentItemBySummary(items, agentstate.TurnItemTypeToolCall, "exec_readonly")
	secondText := findAgentItemBySummary(items, agentstate.TurnItemTypeFinalAnswer, "我再看下时间。")
	secondToolIndex := indexAgentItemByID(items, "turn-agent-items-assistant-tool-text-tool-call-call-date")
	if firstText < 0 || firstTool < 0 || secondText < 0 || secondToolIndex < 0 {
		t.Fatalf("agent items = %#v, want assistant text items before each tool call", items)
	}
	if !(firstText < firstTool && firstTool < secondText && secondText < secondToolIndex) {
		t.Fatalf("agent item order firstText=%d firstTool=%d secondText=%d secondTool=%d, want text/tool/text/tool", firstText, firstTool, secondText, secondToolIndex)
	}
}

func TestToolResultAgentItemDataPreservesInputSummary(t *testing.T) {
	tc := ToolCall{
		ID:        "call-cpu-count",
		Name:      "exec_command",
		Arguments: json.RawMessage(`{"cmd":"sysctl","args":["-n","hw.ncpu"]}`),
	}
	result := ToolResult{
		ToolCallID: "call-cpu-count",
		Content:    "10",
	}

	data := toolResultAgentItemData("turn-1", tc, result)

	if got := data["inputSummary"]; got != "sysctl -n hw.ncpu" {
		t.Fatalf("inputSummary = %#v, want command arguments", got)
	}
	if got := strings.TrimSpace(string(data["arguments"].(json.RawMessage))); got != `{"cmd":"sysctl","args":["-n","hw.ncpu"]}` {
		t.Fatalf("arguments = %s, want original arguments", got)
	}
	if got := data["outputSummary"]; got != "10" {
		t.Fatalf("outputSummary = %#v, want terminal output", got)
	}
}

func TestToolResultAgentItemDataExtractsEvidenceRefsFromStructuredOutput(t *testing.T) {
	tc := ToolCall{
		ID:   "call-terminal",
		Name: "exec_command",
	}
	result := ToolResult{
		ToolCallID: "call-terminal",
		Content:    `{"schemaVersion":"aiops.terminal/v1","tool":"exec_command","status":"ok","command":"curl data:,ok","stdout":"ok","evidenceRefs":["ev-terminal-1"]}`,
	}

	data := toolResultAgentItemData("turn-1", tc, result)

	refs, ok := data["evidenceRefs"].([]string)
	if !ok {
		t.Fatalf("evidenceRefs = %#v, want []string", data["evidenceRefs"])
	}
	if strings.Join(refs, ",") != "ev-terminal-1" {
		t.Fatalf("evidenceRefs = %#v, want ev-terminal-1", refs)
	}
	if got := data["outputSummary"]; got != "curl data:,ok" {
		t.Fatalf("outputSummary = %#v, want terminal command", got)
	}
	if got := strings.TrimSpace(string(data["outputPreview"].(json.RawMessage))); got != `"ok"` {
		t.Fatalf("outputPreview = %s, want stdout preview", got)
	}
}

func TestToolResultAgentItemDataKeepsCorootDisplayDataOutOfOutputPreview(t *testing.T) {
	tc := ToolCall{
		ID:   "call-coroot",
		Name: "coroot.service_metrics",
	}
	result := ToolResult{
		ToolCallID: "call-coroot",
		Content:    `{"schemaVersion":"aiops.coroot/v1","tool":"coroot.service_metrics","status":"ok","service":"checkout","chartSummary":{"service":"checkout","reports":[{"name":"CPU","pointCount":2}]}}`,
		Display: &ToolDisplayPayload{
			Type: "coroot",
			Data: json.RawMessage(`{"schemaVersion":"aiops.coroot/v1","tool":"coroot.service_metrics","status":"ok","service":"checkout","chartReports":[{"name":"CPU","widgets":[{"chart":{"series":[{"name":"checkout","data":[0.1,0.2]}]}}]}]}`),
		},
	}

	data := toolResultAgentItemData("turn-1", tc, result)

	preview := strings.TrimSpace(string(data["outputPreview"].(json.RawMessage)))
	if strings.Contains(preview, "chartReports") || strings.Contains(preview, `"data"`) {
		t.Fatalf("outputPreview leaked raw Coroot display data: %s", preview)
	}
	if !strings.Contains(preview, "chartSummary") {
		t.Fatalf("outputPreview = %s, want compact model-facing summary", preview)
	}
	displayData := strings.TrimSpace(string(data["displayData"].(json.RawMessage)))
	if !strings.Contains(displayData, "chartReports") {
		t.Fatalf("displayData = %s, want native chart payload retained for UI artifacts", displayData)
	}
}

func TestRunTurn_UpdatePlanToolWritesPlanTurnItem(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "plan-call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "update_plan",
						Arguments: `{"steps":[{"id":"inspect","text":"Inspect host symptoms","status":"in_progress"},{"id":"summarize","text":"Summarize findings","status":"pending"}]}`,
					},
				},
			}),
			schema.AssistantMessage("plan noted", nil),
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{planning.NewUpdatePlanTool()}, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-update-plan",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-update-plan",
		Input:       "triage this incident",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-update-plan")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypePlan, agentstate.ItemStatusCompleted) {
		t.Fatalf("agent items = %#v, want completed plan item", session.CurrentTurn.AgentItems)
	}
	planItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypePlan)
	if planItem.Payload.Summary != "plan updated: active (1/2 in_progress)" {
		t.Fatalf("plan summary = %q, want compact update summary", planItem.Payload.Summary)
	}
	var payload planning.PlanState
	if err := json.Unmarshal(planItem.Payload.Data, &payload); err != nil {
		t.Fatalf("plan item payload is not PlanState JSON: %v", err)
	}
	if len(payload.Steps) != 2 || payload.Steps[0].Status != planning.StepStatusInProgress {
		t.Fatalf("plan payload = %#v, want two steps with first in progress", payload)
	}
}

func TestRunTurn_UpdatePlanDoesNotCreateResponsePatchItems(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "plan-call-1",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "update_plan",
						Arguments: `{"steps":[{"id":"inspect","text":"Inspect service","status":"in_progress"}]}`,
					},
				},
			}),
			schema.AssistantMessage("done", nil),
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{planning.NewUpdatePlanTool()}, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-plan-no-patch",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeExecute,
		TurnID:      "turn-plan-no-patch",
		Input:       "triage",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-plan-no-patch")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	for _, item := range session.CurrentTurn.AgentItems {
		itemType := string(item.Type)
		if strings.Contains(itemType, "patch") || strings.Contains(item.Payload.Kind, "response_events") {
			t.Fatalf("unexpected UI patch item: %#v", item)
		}
	}
}

func TestRunTurn_MaxIterationsWritesFailedAgentError(t *testing.T) {
	const maxIterations = 16
	responses := make([]*schema.Message, 0, maxIterations)
	for i := 0; i < maxIterations; i++ {
		responses = append(responses, schema.AssistantMessage("", []schema.ToolCall{
			{
				ID:   "call-loop",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "read_disk_usage",
					Arguments: `{"path":"/tmp"}`,
				},
			},
		}))
	}
	model := &sequentialLoopModel{responses: responses}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Inspect disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-max-iterations",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-max-iterations",
		Input:       "keep looping",
	})
	if err == nil {
		t.Fatal("expected max iteration error")
	}

	session := kernel.sessions.Get("sess-agent-items-max-iterations")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if session.CurrentTurn.Lifecycle != TurnLifecycleFailed {
		t.Fatalf("turn lifecycle = %q, want failed", session.CurrentTurn.Lifecycle)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeError, agentstate.ItemStatusFailed) {
		t.Fatalf("agent items = %#v, want failed error item", session.CurrentTurn.AgentItems)
	}
}

func TestRunTurn_ToolFailureWritesFailedToolResultWithoutBlindRetry(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-fail",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"cmd":"date -v"}`,
					},
				},
			}),
			schema.AssistantMessage("final after failure", nil),
		},
	}
	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Read disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{}, errors.New("date: illegal option")
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-tool-failure",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-tool-failure",
		Input:       "run failing command",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if executed != 1 {
		t.Fatalf("tool executions = %d, want one attempt", executed)
	}

	session := kernel.sessions.Get("sess-agent-items-tool-failure")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeToolResult, agentstate.ItemStatusFailed) {
		t.Fatalf("agent items = %#v, want failed tool_result", session.CurrentTurn.AgentItems)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeFinalAnswer, agentstate.ItemStatusCompleted) {
		t.Fatalf("agent items = %#v, want completed final answer", session.CurrentTurn.AgentItems)
	}
}

func TestRunTurn_ApprovalBlockedAgentItemsDoNotCompleteTool(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-block",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "read_disk_usage",
						Arguments: `{"path":"/tmp"}`,
					},
				},
			}),
		},
	}
	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "read_disk_usage",
			Description: "Inspect disk usage",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}
	registry := hooks.NewRegistry()
	if err := registry.RegisterTool(hooks.ToolRegistration{
		Name:  "approval-gate",
		Stage: hooks.StagePreToolUse,
		Hook: func(_ context.Context, event *hooks.ToolEvent) error {
			event.UpdatedPermissions = &tooling.PermissionDecision{
				Action: tooling.PermissionActionNeedApproval,
				Reason: "approval required",
			}
			return nil
		},
	}); err != nil {
		t.Fatalf("RegisterTool failed: %v", err)
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, registry, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-approval",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-approval",
		Input:       "inspect disk",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "blocked" {
		t.Fatalf("result status = %q, want blocked", result.Status)
	}
	if executed != 0 {
		t.Fatalf("tool executions = %d, want none before approval", executed)
	}

	session := kernel.sessions.Get("sess-agent-items-approval")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeToolCall, agentstate.ItemStatusBlocked) {
		t.Fatalf("agent items = %#v, want blocked tool_call", session.CurrentTurn.AgentItems)
	}
	if hasAgentItemType(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeToolResult) {
		t.Fatalf("agent items = %#v, should not contain tool_result before approval", session.CurrentTurn.AgentItems)
	}
}

func TestRunTurn_PolicyDeniedAgentItemsRecordFailedToolAndError(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-deny",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "write_file",
						Arguments: `{"path":"/tmp/demo","content":"hi"}`,
					},
				},
			}),
		},
	}
	executed := 0
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "write_file",
			Description: "Write file",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeHost)},
			Modes:        []string{string(ModeInspect)},
		},
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			executed++
			return tooling.ToolResult{Content: "should not run"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-policy-deny",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-policy-deny",
		Input:       "write a file",
	})
	if err == nil {
		t.Fatal("expected policy denial error")
	}
	if executed != 0 {
		t.Fatalf("tool executions = %d, want none for denied tool", executed)
	}

	session := kernel.sessions.Get("sess-agent-items-policy-deny")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeToolCall, agentstate.ItemStatusFailed) {
		t.Fatalf("agent items = %#v, want failed tool_call", session.CurrentTurn.AgentItems)
	}
	if !hasAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeError, agentstate.ItemStatusFailed) {
		t.Fatalf("agent items = %#v, want failed error item", session.CurrentTurn.AgentItems)
	}
}

func agentItemTypes(items []agentstate.TurnItem) []agentstate.TurnItemType {
	out := make([]agentstate.TurnItemType, 0, len(items))
	for _, item := range items {
		out = append(out, item.Type)
	}
	return out
}

func hasAgentItem(items []agentstate.TurnItem, typ agentstate.TurnItemType, status agentstate.ItemStatus) bool {
	for _, item := range items {
		if item.Type == typ && item.Status == status {
			return true
		}
	}
	return false
}

func hasAgentItemType(items []agentstate.TurnItem, typ agentstate.TurnItemType) bool {
	for _, item := range items {
		if item.Type == typ {
			return true
		}
	}
	return false
}

func findAgentItem(items []agentstate.TurnItem, typ agentstate.TurnItemType) agentstate.TurnItem {
	for _, item := range items {
		if item.Type == typ {
			return item
		}
	}
	return agentstate.TurnItem{}
}

func findAgentItemBySummary(items []agentstate.TurnItem, typ agentstate.TurnItemType, summary string) int {
	for idx, item := range items {
		if item.Type == typ && item.Payload.Summary == summary {
			return idx
		}
	}
	return -1
}

func indexAgentItemByID(items []agentstate.TurnItem, id string) int {
	for idx, item := range items {
		if item.ID == id {
			return idx
		}
	}
	return -1
}

func sameTurnItemTypes(a, b []agentstate.TurnItemType) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
