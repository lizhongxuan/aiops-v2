package runtimekernel

import (
	"strings"
	"testing"
)

func TestBuildAIOpsCompactPromptContainsRequiredSections(t *testing.T) {
	prompt := BuildAIOpsCompactPrompt(AIOpsCompactPromptInput{
		SessionID: "s1",
		TurnID:    "t1",
	})
	for _, want := range []string{
		"用户当前目标",
		"当前事故",
		"已确认事实和 evidenceRefs",
		"已排除假设",
		"root cause",
		"已执行工具",
		"pending approvals",
		"Runner / OpsManual / MCP / Skills",
		"下一步",
		"用户明确反馈",
		"Do NOT call tools",
		"transcript",
		"external reference IDs",
	} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("compact prompt missing %q:\n%s", want, prompt)
		}
	}
}
