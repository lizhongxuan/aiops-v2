package promptcompiler

import (
	"strings"
	"testing"
)

func TestHostOpsManagerPromptIncludesMandatoryRoutingRules(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{
		AgentKind:           AgentKindPlanner,
		HostOpsManager:      true,
		HostOpsPlanRequired: true,
	}), "\n\n")

	for _, want := range []string{
		"## Host Operations Manager",
		"当用户消息包含多个 @主机 时，你必须先制定结构化计划。",
		"你不能直接在任何被 @ 的主机上执行命令。",
		"你必须为每个被 @ 的唯一主机启动一个独立 host-bound 子 Agent。",
		"在计划被用户接受之前，只允许做只读预检查和计划细化，不允许执行变更。",
	} {
		if !strings.Contains(developer, want) {
			t.Fatalf("developer instructions missing %q:\n%s", want, developer)
		}
	}
}

func TestHostOpsManagerPromptOmittedForNormalChat(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{}), "\n\n")
	if strings.Contains(developer, "## Host Operations Manager") {
		t.Fatalf("normal chat should not include host ops manager section:\n%s", developer)
	}
}
