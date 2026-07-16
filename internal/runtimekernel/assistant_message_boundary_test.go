package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
)

func TestConstrainFinalMessageRemovesRawToolCallMarkup(t *testing.T) {
	raw := `证据边界：当前证据仍受限，以下内容只能作为待核实判断。

<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="web_search">
<｜｜DSML｜｜parameter name="query" string="true">official configuration guide</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
</｜｜DSML｜｜tool_calls>`

	got := constrainFinalMessageForEvidenceBoundary(raw, FinalEvidenceVerification{
		Action:     FinalEvidenceActionDowngrade,
		Confidence: FinalEvidenceConfidenceLow,
		State: FinalEvidenceState{
			Checked: []CheckedEvidence{{ToolName: "web_search", Summary: "已读取公开网页搜索结果。"}},
		},
	})

	for _, forbidden := range []string{"DSML", "tool_calls", "invoke name", "parameter name"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("constrained final leaked raw tool markup %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "已读取公开网页搜索结果") {
		t.Fatalf("constrained final = %q, want checked evidence summary preserved", got)
	}
}

func TestSanitizeFinalAssistantContentBlocksRawToolCallMarkupEvenWhenEvidenceAllowed(t *testing.T) {
	raw := `<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="web_search">
<｜｜DSML｜｜parameter name="operation" string="true">open</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
</｜｜DSML｜｜tool_calls>`

	got, changed := sanitizeFinalAssistantContentForCommit(raw, FinalEvidenceVerification{
		Action:     FinalEvidenceActionAllow,
		Confidence: FinalEvidenceConfidenceHigh,
		State: FinalEvidenceState{
			Checked: []CheckedEvidence{{ToolName: "web_search", Summary: "已读取官方配置文档。"}},
		},
	})

	if !changed {
		t.Fatal("sanitizeFinalAssistantContentForCommit changed=false, want raw tool markup blocked")
	}
	for _, forbidden := range []string{"DSML", "tool_calls", "invoke name", "parameter name"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized final leaked raw tool markup %q:\n%s", forbidden, got)
		}
	}
	if !strings.Contains(got, "已读取官方配置文档") {
		t.Fatalf("sanitized final = %q, want checked evidence summary preserved", got)
	}
}

func TestAssistantMessageBoundaryUsesTypedPendingIntentInsteadOfWording(t *testing.T) {
	texts := []string{
		"我会继续解释这个概念，并在下一段给出例子。",
		"我先说明文件布局，结论是配置应放在项目根目录。",
		"Let me summarize the web documentation and its evidence.",
	}
	for _, text := range texts {
		decision := evaluateFinalMessageBoundary(finalMessageBoundaryInput{
			Text:                text,
			FinishReason:        "stop",
			PendingToolIntent:   false,
			FinalEvidenceAction: string(FinalEvidenceActionAllow),
		})
		if decision.Action != FinalMessageBoundaryAllow {
			t.Fatalf("text %q action=%q, want wording-neutral allow: %#v", text, decision.Action, decision)
		}
	}

	decision := evaluateFinalMessageBoundary(finalMessageBoundaryInput{
		Text:                "这里是一段完整且不含过程词的回答。",
		FinishReason:        "stop",
		PendingToolIntent:   true,
		FinalEvidenceAction: string(FinalEvidenceActionAllow),
	})
	if decision.Action != FinalMessageBoundaryBlock || !containsString(decision.Reasons, "pending_tool_or_process_intent") {
		t.Fatalf("typed pending intent decision = %#v, want block", decision)
	}
}

func TestConstrainedFinalDoesNotInferProcessLeakFromToolVocabulary(t *testing.T) {
	raw := `文档说明 read_context_artifact 会返回 store://tool-spills 引用；示例 {"content":"ok"}，并解释 final contract、kinds= 与 signals= 字段。`
	got := constrainFinalMessageForEvidenceBoundary(raw, FinalEvidenceVerification{
		Action:     FinalEvidenceActionDowngrade,
		Confidence: FinalEvidenceConfidenceLow,
	})

	if !strings.Contains(got, raw) {
		t.Fatalf("constrained final = %q, want legitimate tool vocabulary preserved", got)
	}
}

func TestAssistantMessageBoundaryDoesNotTreatDomainWordingAsProtocolLeak(t *testing.T) {
	raw := "SERVICE_NAME=message-worker。让我获取运行状态并继续说明原因。"
	got := sanitizeIncompleteFinalUserLine(raw)

	if got != raw {
		t.Fatalf("sanitized line = %q, want domain wording preserved without protocol marker", got)
	}
}

func TestAssistantMessageBoundaryConstrainedFinalUsesStructuredActionAcrossDomains(t *testing.T) {
	tests := []struct {
		name string
		text string
	}{
		{name: "consultation", text: "接口隔离能降低调用方与实现之间的耦合。"},
		{name: "file", text: "配置文件已经写入 workspace，并通过格式校验。"},
		{name: "host", text: "主机负载处于预期范围，当前无需变更。"},
		{name: "web", text: "官方网页说明该选项从当前版本开始可用。"},
		{name: "database", text: "数据库连接池已恢复稳定，未观察到新的超时。"},
		{name: "legacy vocabulary", text: "根因、证据和下一步只是本段讨论的术语，不是消息边界。"},
		{name: "process wording", text: "I will explain the result now; this is a complete final response, not pending tool intent."},
	}
	decision := FinalEvidenceVerification{Action: FinalEvidenceActionDowngrade, Confidence: FinalEvidenceConfidenceLow}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := constrainFinalMessageForEvidenceBoundary(tt.text, decision)
			if !strings.Contains(got, tt.text) {
				t.Fatalf("constrained final = %q, want original text preserved", got)
			}
			if !strings.Contains(got, "证据边界") {
				t.Fatalf("constrained final = %q, want structured limitation prefix", got)
			}
		})
	}
}

func TestAssistantMessageBoundaryStructuredBlockOverridesDomainWording(t *testing.T) {
	text := "这是一条完整的通用咨询答复。"
	got := constrainFinalMessageForEvidenceBoundary(text, FinalEvidenceVerification{
		Action:     FinalEvidenceActionBlock,
		Confidence: FinalEvidenceConfidenceLow,
	})
	if strings.Contains(got, text) || !strings.Contains(got, "还不能给最终结论") {
		t.Fatalf("blocked final = %q, want deterministic structured fallback", got)
	}
}

func TestAssistantMessageBoundaryKeepsDelimiterAndCodeFenceGuards(t *testing.T) {
	decision := FinalEvidenceVerification{Action: FinalEvidenceActionDowngrade, Confidence: FinalEvidenceConfidenceLow}
	for _, text := range []string{
		"配置位于（workspace/config.yaml",
		"检查命令如下：\n```sh\nstatus --json",
	} {
		got := constrainFinalMessageForEvidenceBoundary(text, decision)
		if strings.Contains(got, text) || !strings.Contains(got, "还不能给最终结论") {
			t.Fatalf("machine-incomplete final = %q, want deterministic fallback for %q", got, text)
		}
	}
}

func TestSynthesisOnlyPromptsForbidToolCallMarkup(t *testing.T) {
	for _, prompt := range []string{synthesisOnlyPromptAsset(2), publicWebSynthesisOnlyPromptAsset(2)} {
		for _, want := range []string{"不要输出 tool_calls", "DSML", "invoke"} {
			if !strings.Contains(prompt, want) {
				t.Fatalf("prompt = %q, want marker %q", prompt, want)
			}
		}
	}
}

func TestRawToolCallsFromAssistantTextParsesDSMLInvokes(t *testing.T) {
	raw := `<｜｜DSML｜｜tool_calls>
<｜｜DSML｜｜invoke name="web_search">
<｜｜DSML｜｜parameter name="operation" string="true">open</｜｜DSML｜｜parameter>
<｜｜DSML｜｜parameter name="url" string="true">https://pg-auto-failover.readthedocs.io/en/main/operations.html</｜｜DSML｜｜parameter>
<｜｜DSML｜｜parameter name="fetch_content" string="false">true</｜｜DSML｜｜parameter>
<｜｜DSML｜｜parameter name="max_results" string="false">8</｜｜DSML｜｜parameter>
</｜｜DSML｜｜invoke>
</｜｜DSML｜｜tool_calls>`

	calls := rawToolCallsFromAssistantText(raw, "turn-raw", 2)
	if len(calls) != 1 {
		t.Fatalf("calls = %#v, want one parsed tool call", calls)
	}
	if calls[0].Name != "web_search" || calls[0].ID != "turn-raw-raw-dsml-2-0" {
		t.Fatalf("call = %#v, want stable web_search call", calls[0])
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].Arguments, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["operation"] != "open" || args["url"] == "" || args["fetch_content"] != true || args["max_results"].(float64) != 8 {
		t.Fatalf("args = %#v, want parsed string/bool/number params", args)
	}
}

func TestRawToolCallsFromAssistantTextParsesSpacedASCIIDSMLInvokes(t *testing.T) {
	raw := `< | | DSML | | tool_calls>
< | | DSML | | invoke name="exec_command">
< | | DSML | | parameter name="command" string="true">ls< / | | DSML | | parameter>
< | | DSML | | parameter name="args" string="false">["-la","/var/log"]< / | | DSML | | parameter>
< / | | DSML | | invoke>
< / | | DSML | | tool_calls>`

	calls := rawToolCallsFromAssistantText(raw, "turn-spaced-raw", 0)
	if len(calls) != 1 {
		t.Fatalf("calls = %#v, want one parsed tool call from spaced ASCII DSML", calls)
	}
	if calls[0].Name != "exec_command" {
		t.Fatalf("call name = %q, want exec_command", calls[0].Name)
	}
	var args map[string]any
	if err := json.Unmarshal(calls[0].Arguments, &args); err != nil {
		t.Fatalf("unmarshal args: %v", err)
	}
	if args["command"] != "ls" {
		t.Fatalf("command arg = %#v, want ls", args["command"])
	}
	if got, ok := args["args"].([]any); !ok || len(got) != 2 || got[0] != "-la" || got[1] != "/var/log" {
		t.Fatalf("args = %#v, want parsed argument array", args["args"])
	}
}

func TestRawToolCallsFromAssistantTextParsesMultipleDSMLProductionVariants(t *testing.T) {
	raw := `<｜｜ DSML ｜｜ tool_calls>
<｜｜ DSML ｜｜ invoke name="exec_command">
<｜｜ DSML ｜｜ parameter name="command" string="true">ls< / ｜｜ DSML ｜｜ parameter >
<｜｜ DSML ｜｜ parameter name="args" string="false">["-la","/var/log"]< / ｜｜ DSML ｜｜ parameter >
< / ｜｜ DSML ｜｜ invoke >
< | | DSML | | invoke name="web_search">
< | | DSML | | parameter name="query" string="true">postgres timeline< / | | DSML | | parameter >
< / | | DSML | | invoke >
< / ｜｜ DSML ｜｜ tool_calls >`

	calls := rawToolCallsFromAssistantText(raw, "turn-multi-raw", 1)
	if len(calls) != 2 {
		t.Fatalf("calls = %#v, want two parsed tool calls", calls)
	}
	if calls[0].Name != "exec_command" || calls[1].Name != "web_search" {
		t.Fatalf("call names = %q, %q", calls[0].Name, calls[1].Name)
	}
	var execArgs map[string]any
	if err := json.Unmarshal(calls[0].Arguments, &execArgs); err != nil {
		t.Fatalf("unmarshal exec args: %v", err)
	}
	if got, ok := execArgs["args"].([]any); !ok || len(got) != 2 || got[0] != "-la" || got[1] != "/var/log" {
		t.Fatalf("exec args = %#v, want parsed array", execArgs["args"])
	}
}

func TestRecordRawToolCallMarkupFinalSanitizedAddsTraceableErrorItem(t *testing.T) {
	snapshot := &TurnSnapshot{
		ID:          "turn-raw-final",
		SessionID:   "sess-raw-final",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleRunning,
		ResumeState: TurnResumeStateNone,
	}

	recordRawToolCallMarkupFinalSanitized(snapshot, "turn-raw-final", 2, `< | | DSML | | invoke name="exec_command">`)

	if snapshot.Metadata["rawToolCallMarkupSanitized"] != "true" {
		t.Fatalf("metadata rawToolCallMarkupSanitized = %q", snapshot.Metadata["rawToolCallMarkupSanitized"])
	}
	var found bool
	for _, item := range snapshot.AgentItems {
		if item.Type != agentstate.TurnItemTypeError {
			continue
		}
		found = true
		if item.Status != agentstate.ItemStatusFailed {
			t.Fatalf("error item status = %q", item.Status)
		}
		if !strings.Contains(item.Payload.Summary, "raw_tool_call_markup_final") {
			t.Fatalf("error summary = %q", item.Payload.Summary)
		}
		if strings.Contains(item.Payload.Summary, "DSML") || strings.Contains(item.Payload.Summary, "invoke") {
			t.Fatalf("error summary leaked markup: %q", item.Payload.Summary)
		}
	}
	if !found {
		t.Fatalf("missing raw markup error item: %#v", snapshot.AgentItems)
	}
}
