package runtimekernel

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/taskdepth"
)

func TestDepthProfileFromTurnRequestClassifiesRCA(t *testing.T) {
	profile := depthProfileFromTurnRequest(TurnRequest{
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Input:       "排查目标服务异常根因",
	})
	if profile.Level != taskdepth.LevelInvestigation {
		t.Fatalf("level = %s, want investigation; profile=%+v", profile.Level, profile)
	}
	if !profile.RequiresPlan || !profile.RequiresEvidence {
		t.Fatalf("profile should require plan and evidence: %+v", profile)
	}
}

func TestApplyDepthProfileToCompileContext(t *testing.T) {
	ctx := applyDepthProfileToCompileContext(
		emptyCompileContextForDepthTest(),
		taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresPlan: true, RequiresEvidence: true},
		"high",
	)
	if ctx.TaskDepth.Level != taskdepth.LevelInvestigation {
		t.Fatalf("ctx.TaskDepth = %+v", ctx.TaskDepth)
	}
	if ctx.ReasoningEffort != "high" {
		t.Fatalf("ctx.ReasoningEffort = %q, want high", ctx.ReasoningEffort)
	}
}

func TestTaskDepthGateRequiresPlanForMultiStepInvestigationOperationsAndMultiAgent(t *testing.T) {
	for _, level := range []taskdepth.Level{
		taskdepth.LevelMultiStep,
		taskdepth.LevelInvestigation,
		taskdepth.LevelOperations,
		taskdepth.LevelMultiAgent,
	} {
		t.Run(string(level), func(t *testing.T) {
			decision := EvaluatePlanRequirement(
				taskdepth.Profile{Level: level, RequiresPlan: true, RequiresEvidence: taskdepth.AtLeast(level, taskdepth.LevelInvestigation), RequiresValidation: taskdepth.AtLeast(level, taskdepth.LevelOperations)},
				&TurnSnapshot{Metadata: map[string]string{}},
				false,
			)
			if !decision.Required || decision.ReminderLevel != "soft" {
				t.Fatalf("decision = %#v, want soft required", decision)
			}
			if !containsRuntimeTestString(decision.Missing, "plan") {
				t.Fatalf("missing = %#v, want plan", decision.Missing)
			}
		})
	}
}

func TestTaskDepthGateHardBlocksFinalAttemptWithoutPlan(t *testing.T) {
	decision := EvaluatePlanRequirement(
		taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresPlan: true, RequiresEvidence: true},
		&TurnSnapshot{Metadata: map[string]string{}},
		true,
	)
	if !decision.Required || decision.ReminderLevel != "hard" {
		t.Fatalf("decision = %#v, want hard required", decision)
	}
	if !containsRuntimeTestString(decision.Missing, "evidence") {
		t.Fatalf("missing = %#v, want evidence", decision.Missing)
	}
}

func TestTaskDepthGateDoesNotRequirePlanForSimpleQuestion(t *testing.T) {
	for _, level := range []taskdepth.Level{taskdepth.LevelTrivial, taskdepth.LevelSimpleRead} {
		decision := EvaluatePlanRequirement(taskdepth.Profile{Level: level}, &TurnSnapshot{}, true)
		if decision.Required || decision.ReminderLevel != "none" {
			t.Fatalf("decision for %s = %#v, want no requirement", level, decision)
		}
	}
}

func TestMissingEvidenceFinalBlockerGivesActionableSuggestions(t *testing.T) {
	text, blocked := missingEvidenceFinalBlocker(
		taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresEvidence: true},
		&TurnSnapshot{Metadata: map[string]string{prematureFinalGuardMetadataKey: "true"}},
		"检查完成，目标正常。",
	)
	if !blocked {
		t.Fatal("blocked = false, want missing evidence blocker")
	}
	for _, want := range []string{"建议", "确认目标", "选择可用工具", "只读"} {
		if !strings.Contains(text, want) {
			t.Fatalf("blocker text = %q, want %q", text, want)
		}
	}
}

func TestMissingEvidenceFinalBlockerAllowsSelectedHostInventoryAnswer(t *testing.T) {
	text, blocked := missingEvidenceFinalBlocker(
		taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresEvidence: true},
		&TurnSnapshot{Metadata: map[string]string{
			prematureFinalGuardMetadataKey: "true",
			"aiops.host.metadataAvailable": "true",
			"aiops.host.label":             "test-120-77-239-90",
			"aiops.host.address":           "120.77.239.90",
			"aiops.host.sshUser":           "root",
			"aiops.host.sshPort":           "22",
		}},
		"- 主机名称：test-120-77-239-90\n- 地址：120.77.239.90\n- SSH用户：root\n- SSH端口：22",
	)
	if blocked {
		t.Fatalf("blocked = true with %q, want selected host inventory answer allowed", text)
	}
}

func emptyCompileContextForDepthTest() promptcompiler.CompileContext {
	return promptcompiler.CompileContext{SessionType: string(SessionTypeHost), Mode: string(ModeChat)}
}

func containsRuntimeTestString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
