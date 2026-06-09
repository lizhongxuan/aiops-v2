package taskdepth

import "testing"

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
