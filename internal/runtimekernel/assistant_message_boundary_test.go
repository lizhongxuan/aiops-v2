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
<｜｜DSML｜｜parameter name="query" string="true">postgres timeline</｜｜DSML｜｜parameter>
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
			Checked: []CheckedEvidence{{ToolName: "web_search", Summary: "已读取 pg_auto_failover 官方文档。"}},
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
	if !strings.Contains(got, "已读取 pg_auto_failover 官方文档") {
		t.Fatalf("sanitized final = %q, want checked evidence summary preserved", got)
	}
}

func TestFinalMessageHasProcessIntentBlocksLeakedContextArtifactChatter(t *testing.T) {
	raw := `让我直接读取证据引用： greaseardereread_context_artifact with the evidence IDs:So let me try reading the evidence refs directly. I'll also try one more level of the spill chain. theringatherread_context_artifact with evidence IDs:Let me try reading the evidence refs directly. I can see from the initial summaries that there's some useful data already. Let me try one more level. theevidenceThere's useful summary data already. Let me also try to get the incidents more directly. read_context_artifact`

	if !finalMessageHasProcessIntent(raw) {
		t.Fatalf("finalMessageHasProcessIntent=false, want leaked context-artifact chatter blocked")
	}

	decision := evaluateFinalMessageBoundary(finalMessageBoundaryInput{
		Text:                raw,
		FinishReason:        "stop",
		PendingToolIntent:   finalMessageHasProcessIntent(raw),
		FinalEvidenceAction: string(FinalEvidenceActionAllow),
	})
	if decision.Action != FinalMessageBoundaryBlock {
		t.Fatalf("boundary action=%q, want block: %#v", decision.Action, decision)
	}
	if !containsString(decision.Reasons, "pending_tool_or_process_intent") {
		t.Fatalf("boundary reasons=%v, want pending_tool_or_process_intent", decision.Reasons)
	}
}

func TestFinalMessageHasProcessIntentBlocksRepeatedCorootRCAChatter(t *testing.T) {
	raw := strings.Repeat("SERVICE_NAME=rabbitmq-server。让我获取RCA上下文。", 8)

	if !finalMessageHasProcessIntent(raw) {
		t.Fatalf("finalMessageHasProcessIntent=false, want repeated Coroot RCA chatter blocked")
	}

	decision := evaluateFinalMessageBoundary(finalMessageBoundaryInput{
		Text:                raw,
		FinishReason:        "stop",
		PendingToolIntent:   finalMessageHasProcessIntent(raw),
		FinalEvidenceAction: string(FinalEvidenceActionAllow),
	})
	if decision.Action != FinalMessageBoundaryBlock {
		t.Fatalf("boundary action=%q, want block: %#v", decision.Action, decision)
	}
	if !containsString(decision.Reasons, "pending_tool_or_process_intent") {
		t.Fatalf("boundary reasons=%v, want pending_tool_or_process_intent", decision.Reasons)
	}
}

func TestSanitizeIncompleteFinalUserLineHidesLeakedContextArtifactChatter(t *testing.T) {
	raw := `让我直接读取证据引用： read_context_artifact with the evidence IDs: Let me try reading the evidence refs directly.`
	got := sanitizeIncompleteFinalUserLine(raw)

	for _, forbidden := range []string{"read_context_artifact", "evidence IDs", "Let me try", "try reading"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized line leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "工具读取过程") {
		t.Fatalf("sanitized line = %q, want user-visible tool-process fallback", got)
	}
}

func TestSanitizeIncompleteFinalUserLineHidesRepeatedCorootRCAChatter(t *testing.T) {
	raw := strings.Repeat("SERVICE_NAME=rabbitmq-server。让我获取RCA上下文。", 4)
	got := sanitizeIncompleteFinalUserLine(raw)

	for _, forbidden := range []string{"SERVICE_NAME", "ERVICE_NAME", "rabbitmq-server。让我获取RCA上下文"} {
		if strings.Contains(got, forbidden) {
			t.Fatalf("sanitized line leaked %q: %s", forbidden, got)
		}
	}
	if !strings.Contains(got, "工具读取过程") {
		t.Fatalf("sanitized line = %q, want user-visible tool-process fallback", got)
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
