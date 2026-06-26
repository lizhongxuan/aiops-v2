package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/runtimecontract"
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

func TestDepthProfileFromTurnRequestUsesIntentFrameMetadata(t *testing.T) {
	frame := runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
	}
	data, err := json.Marshal(frame)
	if err != nil {
		t.Fatalf("Marshal(IntentFrame) error = %v", err)
	}

	profile := depthProfileFromTurnRequest(TurnRequest{
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		Input:       "compare common scheduling tradeoffs",
		Metadata: map[string]string{
			runtimecontract.MetadataIntentFrame: string(data),
		},
	})

	if profile.Level != taskdepth.LevelInvestigation {
		t.Fatalf("level = %s, want investigation from public_web intent frame; profile=%+v", profile.Level, profile)
	}
	if !profile.RequiresPlan || !profile.RequiresEvidence {
		t.Fatalf("profile should require plan/evidence from structured intent frame: %+v", profile)
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

func TestApplyTurnPromptProfileMetadata(t *testing.T) {
	ctx := applyTurnPromptProfileMetadata(emptyCompileContextForDepthTest(), map[string]string{
		"reasoningEffort": "low",
		"answerStyle":     "concise",
	})
	if ctx.ReasoningEffort != "low" {
		t.Fatalf("ReasoningEffort = %q, want low", ctx.ReasoningEffort)
	}
	if ctx.AnswerStyle != "concise" {
		t.Fatalf("AnswerStyle = %q, want concise", ctx.AnswerStyle)
	}
}

func TestApplyRuntimeStateMetadata(t *testing.T) {
	ctx := applyRuntimeStateMetadata(
		promptcompiler.CompileContext{VisibleToolFingerprint: "tools:abc"},
		map[string]string{
			"enableToolPack":                   "public_web",
			"aiops.opsGraph.explicitMention":   "true",
			"aiops.coroot.explicitRCA":         "true",
			"aiops.opsManuals.explicitMention": "true",
			"userConstraints":                  "read_only; no_restart",
		},
		&SessionState{
			PendingApprovals: []PendingApproval{{ID: "approval-1"}},
			PendingEvidence:  []PendingEvidence{{ID: "evidence-1"}, {ID: "evidence-2"}},
		},
		&TurnSnapshot{ResumeState: TurnResumeStatePendingEvidence},
	)

	if ctx.WebState != "requested" || ctx.OpsGraphState != "requested" || ctx.CorootState != "requested" || ctx.OpsManusState != "requested" {
		t.Fatalf("feature states = web:%q opsGraph:%q coroot:%q opsManus:%q", ctx.WebState, ctx.OpsGraphState, ctx.CorootState, ctx.OpsManusState)
	}
	if ctx.PendingApprovals != 1 || ctx.PendingEvidence != 2 {
		t.Fatalf("pending counts = approvals:%d evidence:%d", ctx.PendingApprovals, ctx.PendingEvidence)
	}
	if ctx.VisibleToolFingerprint != "tools:abc" {
		t.Fatalf("VisibleToolFingerprint = %q", ctx.VisibleToolFingerprint)
	}
	if strings.Join(ctx.UserConstraints, ",") != "read_only,no_restart" {
		t.Fatalf("UserConstraints = %#v", ctx.UserConstraints)
	}
	if ctx.TimeoutRecoveryState != string(TurnResumeStatePendingEvidence) {
		t.Fatalf("TimeoutRecoveryState = %q", ctx.TimeoutRecoveryState)
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

func TestTaskDepthGateTreatsUserProvidedEvidenceAsEvidence(t *testing.T) {
	decision := EvaluatePlanRequirement(
		taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresPlan: true, RequiresEvidence: true},
		&TurnSnapshot{Metadata: map[string]string{
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.rawExcerpt": "pg_controldata timeline 7; pg_autoctl timeline 9",
		}},
		true,
	)
	if containsRuntimeTestString(decision.Missing, "evidence") {
		t.Fatalf("missing = %#v, want user-provided evidence to satisfy evidence requirement", decision.Missing)
	}
}

func TestMissingEvidenceFinalBlockerAllowsUserProvidedEvidence(t *testing.T) {
	text, blocked := missingEvidenceFinalBlocker(
		taskdepth.Profile{Level: taskdepth.LevelInvestigation, RequiresEvidence: true},
		&TurnSnapshot{Metadata: map[string]string{
			prematureFinalGuardMetadataKey:  "true",
			"aiops.userEvidence.present":    "true",
			"aiops.userEvidence.rawExcerpt": "pg_controldata timeline 7; pg_autoctl timeline 9",
		}},
		"基于用户粘贴的命令输出，A 与 B 是不同 timeline 分支。",
	)
	if blocked {
		t.Fatalf("blocked = true with %q, want pasted evidence final answer allowed", text)
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
