package taskdepth

import (
	"testing"

	"aiops-v2/internal/runtimecontract"
)

func TestClassifyKeepsSimpleQuestionTrivial(t *testing.T) {
	profile := Classify(Options{Input: "你好，AIOps 是什么？", Mode: "chat"})
	if profile.Level != LevelTrivial {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelTrivial, profile)
	}
	if profile.RequiresPlan || profile.RequiresEvidence || !profile.AllowsFirstTurnFinal {
		t.Fatalf("simple profile should allow direct final: %+v", profile)
	}
}

func TestClassifyRCARequiresPlanAndEvidence(t *testing.T) {
	profile := Classify(Options{Input: "排查目标服务在指定时间窗内关键指标异常的根因", Mode: "chat"})
	if profile.Level != LevelInvestigation {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelInvestigation, profile)
	}
	if !profile.RequiresPlan || !profile.RequiresEvidence || profile.AllowsFirstTurnFinal {
		t.Fatalf("RCA profile should require plan/evidence and block first-turn final: %+v", profile)
	}
}

func TestClassifyOperationsRequiresValidation(t *testing.T) {
	profile := Classify(Options{Input: "帮我重启目标服务并确认恢复", Mode: "execute"})
	if profile.Level != LevelOperations {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelOperations, profile)
	}
	if !profile.RequiresPlan || !profile.RequiresEvidence || !profile.RequiresValidation {
		t.Fatalf("operations profile should require plan/evidence/validation: %+v", profile)
	}
}

func TestClassifyAnalysisOnlyOpsQuestionDoesNotRequireExecutionValidation(t *testing.T) {
	profile := Classify(Options{
		Input: "我做了几次备份并恢复了一个节点，现在从节点加入后同步异常，为什么会这样？先只做原理分析和证据清单，不要连接或执行任何主机命令。",
		Mode:  "chat",
		Metadata: map[string]string{
			"aiops.route.mode":                   "chat_advisory",
			"aiops.tool.execCommandAllowed":      "false",
			"aiops.route.userProhibitedHostExec": "true",
		},
	})
	if profile.Level == LevelOperations {
		t.Fatalf("level = %s, want analysis/investigation level without operations validation; profile=%+v", profile.Level, profile)
	}
	if !profile.AnalysisOnly || !profile.ExecutionProhibited {
		t.Fatalf("analysis-only flags missing: %+v", profile)
	}
	if profile.RequiresValidation {
		t.Fatalf("RequiresValidation = true, want false for analysis-only no-exec request: %+v", profile)
	}
}

func TestClassifyEvidenceRCANoExecDoesNotRequireExecutionValidation(t *testing.T) {
	profile := Classify(Options{
		Input: "请只基于下面证据分析复制异常，不要执行主机命令。\nLatest checkpoint TimeLineID: 9\nFATAL: requested timeline 7 is not a child of this server's history",
		Mode:  "chat",
		Metadata: map[string]string{
			"aiops.route.mode":                   "evidence_rca",
			"aiops.tool.execCommandAllowed":      "false",
			"aiops.route.userProhibitedHostExec": "true",
			"aiops.userEvidence.present":         "true",
		},
	})
	if profile.Level == LevelOperations {
		t.Fatalf("level = %s, want analysis/investigation level for evidence RCA no-exec: %+v", profile.Level, profile)
	}
	if !profile.AnalysisOnly || !profile.ExecutionProhibited {
		t.Fatalf("analysis-only flags missing: %+v", profile)
	}
	if profile.RequiresValidation {
		t.Fatalf("RequiresValidation = true, want false for evidence RCA no-exec: %+v", profile)
	}
}

func TestClassifyReadOnlyHostInspectionInExecuteModeStaysSimpleRead(t *testing.T) {
	profile := Classify(Options{Input: "查看主机 CPU 和内存资源", Mode: "execute"})
	if profile.Level != LevelSimpleRead {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelSimpleRead, profile)
	}
	if profile.RequiresPlan || profile.RequiresEvidence || profile.RequiresValidation {
		t.Fatalf("read-only host inspection should not require operations gates: %+v", profile)
	}
}

func TestClassifyMultiHostIsMultiAgent(t *testing.T) {
	profile := Classify(Options{Input: "同时排查多个目标主机的资源使用异常", Mode: "chat", Metadata: map[string]string{"hostMentionCount": "2"}})
	if profile.Level != LevelMultiAgent {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelMultiAgent, profile)
	}
}

func TestClassifyMetadataOverride(t *testing.T) {
	profile := Classify(Options{Input: "看一下", Mode: "chat", Metadata: map[string]string{"taskDepth": "investigation"}})
	if profile.Level != LevelInvestigation {
		t.Fatalf("level = %s, want metadata override investigation", profile.Level)
	}
}

func TestClassifyIntentFrameExplainIsShallowOrNormal(t *testing.T) {
	profile := ClassifyFromIntentFrame(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindExplain,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeOpsKnowledge},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
	}, Options{Input: "解释一下这个指标是什么意思"})

	if AtLeast(profile.Level, LevelInvestigation) {
		t.Fatalf("level = %s, want shallow/normal below investigation; profile=%+v", profile.Level, profile)
	}
	if profile.RequiresEvidence || profile.RequiresValidation || profile.RequiresApproval {
		t.Fatalf("explain profile should not require evidence/validation/approval: %+v", profile)
	}
}

func TestClassifyIntentFrameDiagnosisWithEvidenceIsDeep(t *testing.T) {
	profile := ClassifyFromIntentFrame(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindDiagnose,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
		Evidence: runtimecontract.EvidenceEnvelope{
			HasUserProvidedEvidence: true,
			EvidenceKinds:           []string{"log_like_text"},
			DataScopes:              []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
		},
	}, Options{Input: "帮我看下这段输出"})

	if profile.Level != LevelInvestigation {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelInvestigation, profile)
	}
	if !profile.RequiresPlan || !profile.RequiresEvidence || profile.AllowsFirstTurnFinal {
		t.Fatalf("diagnosis with evidence should require deep investigation gates: %+v", profile)
	}
}

func TestClassifyIntentFrameChatAdvisoryDoesNotEscalateHostExecRiskToOperations(t *testing.T) {
	profile := ClassifyFromIntentFrame(runtimecontract.IntentFrame{
		Kind: runtimecontract.IntentKindDiagnose,
		DataScopes: []runtimecontract.DataScope{
			runtimecontract.DataScopeOpsKnowledge,
			runtimecontract.DataScopePublicWeb,
			runtimecontract.DataScopeLocalRuntime,
		},
		RiskBudget: []runtimecontract.ActionRisk{
			runtimecontract.ActionRiskNetwork,
			runtimecontract.ActionRiskHostExec,
		},
		Capabilities: []runtimecontract.CapabilityCandidate{{
			Name:       "host_runtime_inspection",
			DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeLocalRuntime},
			Risks:      []runtimecontract.ActionRisk{runtimecontract.ActionRiskHostExec},
		}},
	}, Options{
		Input: "为什么从节点加入失败，还需要哪些只读信息？",
		Mode:  "chat",
		Metadata: map[string]string{
			"aiops.route.mode":               "chat_advisory",
			"aiops.tool.execCommandAllowed":  "false",
			"aiops.tool.hostMutationAllowed": "false",
		},
	})

	if profile.Level == LevelOperations {
		t.Fatalf("level = %s, want advisory analysis below operations; profile=%+v", profile.Level, profile)
	}
	if !profile.AnalysisOnly || !profile.ExecutionProhibited {
		t.Fatalf("advisory analysis flags missing: %+v", profile)
	}
	if profile.RequiresValidation || profile.RequiresApproval {
		t.Fatalf("advisory analysis must not require validation/approval: %+v", profile)
	}
}

func TestClassifyIntentFrameChangeWithRiskIsDeepAndRequiresApproval(t *testing.T) {
	profile := ClassifyFromIntentFrame(runtimecontract.IntentFrame{
		Kind: runtimecontract.IntentKindChange,
		RiskBudget: []runtimecontract.ActionRisk{
			runtimecontract.ActionRiskWrite,
			runtimecontract.ActionRiskHostExec,
		},
	}, Options{Mode: "execute"})

	if profile.Level != LevelOperations {
		t.Fatalf("level = %s, want %s; profile=%+v", profile.Level, LevelOperations, profile)
	}
	if !profile.RequiresValidation || !profile.RequiresApproval {
		t.Fatalf("change with write/host-exec risk should require validation and approval: %+v", profile)
	}
	if !hasReason(profile, "risk budget includes write") || !hasReason(profile, "risk budget includes host_exec") {
		t.Fatalf("risk budget should be visible in reasons: %+v", profile.Reasons)
	}
}

func TestClassifyIntentFramePublicResearchUsesScopeNotKeyword(t *testing.T) {
	withoutPublicWeb := ClassifyFromIntentFrame(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopeOpsKnowledge},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly},
	}, Options{Input: "compare common scheduling tradeoffs"})

	withPublicWeb := ClassifyFromIntentFrame(runtimecontract.IntentFrame{
		Kind:       runtimecontract.IntentKindResearch,
		DataScopes: []runtimecontract.DataScope{runtimecontract.DataScopePublicWeb},
		RiskBudget: []runtimecontract.ActionRisk{runtimecontract.ActionRiskReadOnly, runtimecontract.ActionRiskNetwork},
	}, Options{Input: "compare common scheduling tradeoffs"})

	if withoutPublicWeb.Level != LevelMultiStep {
		t.Fatalf("level without public web = %s, want %s; profile=%+v", withoutPublicWeb.Level, LevelMultiStep, withoutPublicWeb)
	}
	if withPublicWeb.Level != LevelInvestigation {
		t.Fatalf("level with public web = %s, want %s; profile=%+v", withPublicWeb.Level, LevelInvestigation, withPublicWeb)
	}
	if withPublicWeb.RequiresApproval {
		t.Fatalf("public-web research should not require approval without write/exec/destructive risk: %+v", withPublicWeb)
	}
}

func hasReason(profile Profile, reason string) bool {
	for _, got := range profile.Reasons {
		if got == reason {
			return true
		}
	}
	return false
}
