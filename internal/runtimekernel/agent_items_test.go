package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

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
			schema.AssistantMessage("final answer with enough detail", nil),
			schema.AssistantMessage("final answer with enough detail", nil),
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
		agentstate.TurnItemTypeAssistantMessage,
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

func TestRunTurn_WritesFullUserMessageInAgentItemData(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("已收到完整输入，后续展示应保留原文。", nil),
		},
	}
	kernel := newLoopKernel(t, model, nil, nil, nil)
	input := "这是一段普通的长用户输入，用来确认聊天记录会保存完整正文，而不是只保存短摘要。它包含多个句子和足够的长度，前端展示时也不应该变成带省略号的预览。请原样保留这段文本。"

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-full-user-message",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-full-user-message",
		Input:       input,
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-agent-items-full-user-message")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	item := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeUserMessage)
	if item.ID == "" {
		t.Fatalf("agent items = %#v, want user message item", session.CurrentTurn.AgentItems)
	}
	var payload struct {
		Prompt string `json:"prompt"`
		Text   string `json:"text"`
	}
	if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
		t.Fatalf("unmarshal user message payload: %v", err)
	}
	if payload.Prompt != input {
		t.Fatalf("user message prompt = %q, want full input %q", payload.Prompt, input)
	}
	if strings.Contains(item.Payload.Summary, "...") && item.Payload.Summary == payload.Prompt {
		t.Fatalf("summary should remain a preview separate from full prompt: %q", item.Payload.Summary)
	}
}

func TestRunTurn_WritesAgentEvidenceItemForUserProvidedEvidence(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("final answer using provided evidence", nil),
		},
	}
	kernel := newLoopKernel(t, model, nil, nil, nil)

	_, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-user-evidence",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-user-evidence",
		Input:       "Analyze this pasted evidence only.",
		Metadata: map[string]string{
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.kinds":      "command_output,log",
			"aiops.userEvidence.signals":    "runtime_error,recovery_state",
			"aiops.userEvidence.rawExcerpt": "command output and log excerpt",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-agent-items-user-evidence")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	want := []agentstate.TurnItemType{
		agentstate.TurnItemTypeUserMessage,
		agentstate.TurnItemTypeEvidence,
		agentstate.TurnItemTypeModelCall,
		agentstate.TurnItemTypeAssistantMessage,
	}
	if got := agentItemTypes(session.CurrentTurn.AgentItems); !sameTurnItemTypes(got, want) {
		t.Fatalf("agent item types = %v, want %v", got, want)
	}
	evidenceItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeEvidence)
	if evidenceItem.ID == "" {
		t.Fatalf("agent items = %#v, want evidence item", session.CurrentTurn.AgentItems)
	}
	if evidenceItem.Status != agentstate.ItemStatusCompleted {
		t.Fatalf("evidence item status = %q, want completed", evidenceItem.Status)
	}
	if evidenceItem.Payload.Kind != "user_provided" {
		t.Fatalf("evidence item kind = %q, want user_provided", evidenceItem.Payload.Kind)
	}
	if !strings.Contains(evidenceItem.Payload.Summary, "command_output,log") ||
		!strings.Contains(evidenceItem.Payload.Summary, "runtime_error,recovery_state") {
		t.Fatalf("evidence summary = %q, want kinds and signals", evidenceItem.Payload.Summary)
	}
	var payload struct {
		Source  string `json:"source"`
		Kinds   string `json:"kinds"`
		Signals string `json:"signals"`
		Excerpt string `json:"excerpt"`
	}
	if err := json.Unmarshal(evidenceItem.Payload.Data, &payload); err != nil {
		t.Fatalf("unmarshal evidence payload: %v", err)
	}
	if payload.Source != "user" || payload.Kinds != "command_output,log" ||
		payload.Signals != "runtime_error,recovery_state" ||
		payload.Excerpt != "command output and log excerpt" {
		t.Fatalf("evidence payload = %#v, want structured user evidence", payload)
	}
	finalItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	if finalItem.ID == "" {
		t.Fatalf("agent items = %#v, want assistant_message final item", session.CurrentTurn.AgentItems)
	}
	var finalPayload struct {
		EvidenceRefs []string `json:"evidenceRefs"`
	}
	if err := json.Unmarshal(finalItem.Payload.Data, &finalPayload); err != nil {
		t.Fatalf("unmarshal final payload: %v", err)
	}
	if len(finalPayload.EvidenceRefs) != 1 || finalPayload.EvidenceRefs[0] != evidenceItem.ID {
		t.Fatalf("final evidenceRefs = %#v, want %q", finalPayload.EvidenceRefs, evidenceItem.ID)
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
		agentstate.TurnItemTypeAssistantMessage,
	}
	if got := agentItemTypes(session.CurrentTurn.AgentItems); !sameTurnItemTypes(got, want) {
		t.Fatalf("agent item types = %v, want %v", got, want)
	}
}

func TestRunTurn_RecordsFinalGenerationDurationAfterToolBudget(t *testing.T) {
	toolCalls := make([]schema.ToolCall, 0, defaultMaxToolDispatchesPerTurn+1)
	for i := 0; i < defaultMaxToolDispatchesPerTurn+1; i++ {
		toolCalls = append(toolCalls, schema.ToolCall{
			ID:   "call-budget-" + string(rune('a'+i)),
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "read_disk_usage",
				Arguments: `{"path":"/tmp"}`,
			},
		})
	}
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", toolCalls),
			schema.AssistantMessage("final answer after budget", nil),
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
		ExecuteFunc: func(_ context.Context, _ json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}
	kernel := newLoopKernel(t, model, []tooling.Tool{toolDef}, nil, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-agent-items-tool-budget-final-duration",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-agent-items-tool-budget-final-duration",
		Input:       "inspect disks",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}

	session := kernel.sessions.Get("sess-agent-items-tool-budget-final-duration")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	finalItem := findAgentItem(session.CurrentTurn.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	if finalItem.ID == "" {
		t.Fatalf("agent items = %#v, want assistant_message final", session.CurrentTurn.AgentItems)
	}
	var payload struct {
		DurationMs int64 `json:"durationMs"`
	}
	if err := json.Unmarshal(finalItem.Payload.Data, &payload); err != nil {
		t.Fatalf("unmarshal final item payload: %v", err)
	}
	if payload.DurationMs <= 0 {
		t.Fatalf("final item durationMs = %d, want positive", payload.DurationMs)
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
	firstText := findAgentItemBySummary(items, agentstate.TurnItemTypeAssistantMessage, "我先在本机读取当前工作路径。")
	firstTool := findAgentItemBySummary(items, agentstate.TurnItemTypeToolCall, "exec_readonly")
	secondText := findAgentItemBySummary(items, agentstate.TurnItemTypeAssistantMessage, "我再看下时间。")
	secondToolIndex := indexAgentItemByID(items, "turn-agent-items-assistant-tool-text-tool-call-call-date")
	if firstText < 0 || firstTool < 0 || secondText < 0 || secondToolIndex < 0 {
		t.Fatalf("agent items = %#v, want assistant text items before each tool call", items)
	}
	for _, index := range []int{firstText, secondText} {
		if assistantMessagePhaseForAgentItemsTest(items[index]) != "commentary" {
			t.Fatalf("assistant item = %#v, want commentary phase", items[index])
		}
	}
	if !(firstText < firstTool && firstTool < secondText && secondText < secondToolIndex) {
		t.Fatalf("agent item order firstText=%d firstTool=%d secondText=%d secondTool=%d, want text/tool/text/tool", firstText, firstTool, secondText, secondToolIndex)
	}
	if preludeFinal := findAssistantMessageBySummaryAndPhase(items, "我先在本机读取当前工作路径。", "final_answer"); preludeFinal >= 0 {
		t.Fatalf("agent items = %#v, tool prelude must not be stored as final_answer phase", items)
	}
}

func TestRunTurn_LongAssistantTextBeforeToolCallIsNotProgress(t *testing.T) {
	longDraft := strings.Join([]string{
		"结论（待验证）：服务异常最可能与数据库连接池耗尽有关。",
		"",
		"机制链路：请求进入应用后需要获取数据库连接，但连接池长时间没有可用连接，导致上游请求排队并出现超时。这个判断目前只是模型草稿，因为还没有工具证据验证连接池、数据库会话数和应用错误日志。",
		"",
		"下一步：先读取只读状态，再基于证据给最终回答。",
	}, "\n")
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage(longDraft, []schema.ToolCall{{
				ID:   "call-status",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "exec_readonly",
					Arguments: `{"cmd":"status"}`,
				},
			}}),
			schema.AssistantMessage("结论：只读状态检查返回 ok，暂未看到数据库连接池耗尽证据。证据：exec_readonly 返回 ok。下一步：继续补充应用日志和连接池指标。", nil),
		},
	}
	toolDef := &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "exec_readonly",
			Description: "Run a read-only status command",
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
		SessionID:   "sess-long-tool-prelude",
		SessionType: SessionTypeHost,
		Mode:        ModeExecute,
		TurnID:      "turn-long-tool-prelude",
		Input:       "检查服务异常",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}

	session := kernel.sessions.Get("sess-long-tool-prelude")
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("expected current turn")
	}
	items := session.CurrentTurn.AgentItems
	if messageIndex := findAgentItemBySummary(items, agentstate.TurnItemTypeAssistantMessage, longDraft); messageIndex >= 0 {
		t.Fatalf("agent items = %#v, long draft before tool call must not be stored as assistant_message", items)
	}
	if toolIndex := indexAgentItemByID(items, "turn-long-tool-prelude-tool-call-call-status"); toolIndex < 0 {
		t.Fatalf("agent items = %#v, want tool call recorded", items)
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

func TestAssistantMessageAgentItemDataContract(t *testing.T) {
	data := assistantMessageAgentItemData(assistantMessageData{
		MessageID:        "msg-1",
		Iteration:        2,
		Phase:            AssistantMessagePhaseFinalAnswer,
		StreamState:      AssistantMessageStreamStateStreaming,
		EvidenceBoundary: "limited",
		BoundaryAction:   FinalMessageBoundaryConstrain,
		TextHash:         "hash-1",
		Duration:         1500 * time.Millisecond,
	})
	raw, err := json.Marshal(data)
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{
		"displayKind":      "assistant.message",
		"messageId":        "msg-1",
		"phase":            "final_answer",
		"streamState":      "streaming",
		"evidenceBoundary": "limited",
		"boundaryAction":   "constrain",
		"textHash":         "hash-1",
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("%s = %#v, want %#v in %#v", key, got[key], value, got)
		}
	}
	if got["iteration"] != float64(2) {
		t.Fatalf("iteration = %#v, want 2", got["iteration"])
	}
	if got["durationMs"] != float64(1500) {
		t.Fatalf("durationMs = %#v, want 1500", got["durationMs"])
	}
}

func TestUpsertAssistantMessageItemUsesSingleAssistantMessageType(t *testing.T) {
	snapshot := &TurnSnapshot{SessionID: "sess-1", ID: "turn-1"}
	itemID := assistantMessageItemID(snapshot.ID, 0)
	upsertAssistantMessageItem(snapshot, itemID, agentstate.ItemStatusRunning, "第一段", assistantMessageData{
		MessageID:   "msg-1",
		Iteration:   0,
		Phase:       AssistantMessagePhaseFinalAnswer,
		StreamState: AssistantMessageStreamStateStreaming,
	})
	upsertAssistantMessageItem(snapshot, itemID, agentstate.ItemStatusCompleted, "第一段第二段", assistantMessageData{
		MessageID:   "msg-1",
		Iteration:   0,
		Phase:       AssistantMessagePhaseFinalAnswer,
		StreamState: AssistantMessageStreamStateComplete,
	})
	if len(snapshot.AgentItems) != 1 {
		t.Fatalf("items = %#v, want one updated assistant_message item", snapshot.AgentItems)
	}
	item := snapshot.AgentItems[0]
	if item.Type != agentstate.TurnItemTypeAssistantMessage || item.Payload.Summary != "第一段第二段" {
		t.Fatalf("item = %#v, want completed assistant_message with updated text", item)
	}
	var data map[string]any
	if err := json.Unmarshal(item.Payload.Data, &data); err != nil {
		t.Fatal(err)
	}
	if data["phase"] != "final_answer" || data["streamState"] != "complete" {
		t.Fatalf("data = %#v, want final_answer complete", data)
	}
}

func TestAssistantMessageItemHelpersPreserveRetryLifecycle(t *testing.T) {
	snapshot := &TurnSnapshot{
		SessionID: "sess-message-helper",
		ID:        "turn-message-helper",
	}
	firstID := assistantMessageItemID(snapshot.ID, 0)
	upsertAssistantMessageItem(snapshot, firstID, agentstate.ItemStatusRunning, "第一段", assistantMessageData{
		MessageID:   "msg-0",
		Iteration:   0,
		Phase:       AssistantMessagePhaseFinalAnswer,
		StreamState: AssistantMessageStreamStateStreaming,
		TextHash:    "hash-0",
	})
	upsertAssistantMessageItem(snapshot, firstID, agentstate.ItemStatusRunning, "第一段第二段", assistantMessageData{
		MessageID:   "msg-0",
		Iteration:   0,
		Phase:       AssistantMessagePhaseFinalAnswer,
		StreamState: AssistantMessageStreamStateStreaming,
		TextHash:    "hash-0b",
	})

	if len(snapshot.AgentItems) != 1 {
		t.Fatalf("agent item count = %d, want one upserted message item: %#v", len(snapshot.AgentItems), snapshot.AgentItems)
	}
	if got := snapshot.AgentItems[0].Payload.Summary; got != "第一段第二段" {
		t.Fatalf("assistant message summary = %q, want updated full draft", got)
	}

	markAssistantMessageReplacedForRetry(snapshot, firstID, "第一段第二段", "msg-0", 0, 1500*time.Millisecond, "limited", FinalMessageBoundaryRetryOnce)
	if len(snapshot.AgentItems) != 1 {
		t.Fatalf("agent item count after replacement = %d, want one preserved message item", len(snapshot.AgentItems))
	}
	if snapshot.AgentItems[0].Status != agentstate.ItemStatusFailed {
		t.Fatalf("replaced message status = %q, want failed", snapshot.AgentItems[0].Status)
	}
	if got := snapshot.AgentItems[0].Payload.Summary; got != "第一段第二段" {
		t.Fatalf("replaced message summary = %q, want preserved draft", got)
	}
	var replaced map[string]any
	if err := json.Unmarshal(snapshot.AgentItems[0].Payload.Data, &replaced); err != nil {
		t.Fatalf("unmarshal replaced message data: %v", err)
	}
	if replaced["phase"] != "final_answer" || replaced["streamState"] != "incomplete" || replaced["boundaryAction"] != "retry_once" || replaced["evidenceBoundary"] != "limited" {
		t.Fatalf("replaced payload = %#v, want final_answer incomplete retry_once limited", replaced)
	}
	if replaced["replacedByMessageId"] != assistantMessageItemID(snapshot.ID, 1) {
		t.Fatalf("replacedByMessageId = %#v, want next message id", replaced["replacedByMessageId"])
	}

	committedID := assistantMessageItemID(snapshot.ID, 1)
	completeAssistantMessageItem(snapshot, committedID, "最终回答", assistantMessageData{
		MessageID: "msg-1",
		Iteration: 1,
		Phase:     AssistantMessagePhaseFinalAnswer,
		TextHash:  "hash-1",
	})
	if len(snapshot.AgentItems) != 2 {
		t.Fatalf("agent item count after commit = %d, want replaced and committed messages", len(snapshot.AgentItems))
	}
	latest, ok := latestAssistantFinalMessageItem(snapshot)
	if !ok {
		t.Fatal("latestAssistantFinalMessageItem returned false")
	}
	if latest.ID != committedID || latest.Status != agentstate.ItemStatusCompleted || latest.Payload.Summary != "最终回答" {
		t.Fatalf("latest message = %#v, want committed final item %s", latest, committedID)
	}
	var committed map[string]any
	if err := json.Unmarshal(latest.Payload.Data, &committed); err != nil {
		t.Fatalf("unmarshal committed message data: %v", err)
	}
	if committed["phase"] != "final_answer" || committed["streamState"] != "complete" {
		t.Fatalf("committed payload = %#v, want final_answer/complete", committed)
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
			schema.AssistantMessage("缺少执行结果，计划仍在进行。", nil),
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
			schema.AssistantMessage("缺少执行结果，计划仍在进行。", nil),
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
	if findAssistantMessageBySummaryAndPhase(session.CurrentTurn.AgentItems, "final after failure", "final_answer") < 0 {
		t.Fatalf("agent items = %#v, want completed assistant_message final answer", session.CurrentTurn.AgentItems)
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

func findAssistantMessageBySummaryAndPhase(items []agentstate.TurnItem, summary, phase string) int {
	for idx, item := range items {
		if item.Type == agentstate.TurnItemTypeAssistantMessage && item.Payload.Summary == summary && assistantMessagePhaseForAgentItemsTest(item) == phase {
			return idx
		}
	}
	return -1
}

func assistantMessagePhaseForAgentItemsTest(item agentstate.TurnItem) string {
	var payload struct {
		Phase string `json:"phase"`
	}
	if len(item.Payload.Data) == 0 {
		return ""
	}
	_ = json.Unmarshal(item.Payload.Data, &payload)
	return payload.Phase
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
