package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"
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
