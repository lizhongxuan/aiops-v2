package appui

import (
	"testing"
	"time"
)

func TestPostRunSuggestionsOnlyWhenUseful(t *testing.T) {
	completedWithoutReusableEvidence := AgentRunView{
		ID:       "run-chat-only",
		UserGoal: "解释 nginx 502 可能原因",
		Status:   AgentRunStatusCompleted,
		Steps: []AgentStepView{{
			ID:            "final-1",
			Kind:          AgentStepKindFinalResponse,
			Status:        AgentStepStatusCompleted,
			OutputSummary: "需要继续收集证据。",
		}},
	}
	if suggestions := BuildPostRunSuggestionsFromAgentRun(completedWithoutReusableEvidence); len(suggestions) != 0 {
		t.Fatalf("suggestions = %#v, want none for a low-evidence chat", suggestions)
	}

	runWithReusableEvidence := AgentRunView{
		ID:       "run-redis-repair",
		UserGoal: "修复 redis 主从复制异常",
		Status:   AgentRunStatusCompleted,
		Steps: []AgentStepView{
			{ID: "evidence-1", Kind: AgentStepKindEvidence, Status: AgentStepStatusCompleted, OutputSummary: "redis replica_link_status=down"},
			{ID: "tool-1", Kind: AgentStepKindToolCall, Status: AgentStepStatusCompleted, ToolName: "host_exec", OutputSummary: "执行 systemctl restart redis"},
			{ID: "approval-1", Kind: AgentStepKindApproval, Status: AgentStepStatusCompleted, OutputSummary: "用户批准重启 redis"},
			{ID: "final-1", Kind: AgentStepKindFinalResponse, Status: AgentStepStatusCompleted, OutputSummary: "已恢复复制并记录风险边界。"},
		},
	}
	suggestions := BuildPostRunSuggestionsFromAgentRun(runWithReusableEvidence)
	if !hasPostRunSuggestion(suggestions, PostRunSuggestionRunRecord) ||
		!hasPostRunSuggestion(suggestions, PostRunSuggestionExperienceCandidate) ||
		!hasPostRunSuggestion(suggestions, PostRunSuggestionCase) {
		t.Fatalf("suggestions = %#v, want run record, experience candidate, and case suggestions", suggestions)
	}
}

func TestPostRunSuggestionsRequireUsefulnessDecision(t *testing.T) {
	run := AgentRunView{
		ID:       "run-useful-but-declined",
		UserGoal: "修复 redis 主从复制异常",
		Status:   AgentRunStatusCompleted,
		Steps: []AgentStepView{
			{ID: "evidence-1", Kind: AgentStepKindEvidence, Status: AgentStepStatusCompleted, OutputSummary: "redis replica_link_status=down"},
			{ID: "tool-1", Kind: AgentStepKindToolCall, Status: AgentStepStatusCompleted, ToolName: "host_exec", OutputSummary: "执行 systemctl restart redis"},
			{ID: "final-1", Kind: AgentStepKindFinalResponse, Status: AgentStepStatusCompleted, OutputSummary: "已恢复复制。"},
		},
	}

	declined := PostRunUsefulnessDecision{
		ShouldSuggest: false,
		Reason:        "LLM 判断本次处理没有沉淀价值",
		DecidedBy:     PostRunDecisionByLLM,
	}
	if suggestions := BuildPostRunSuggestionsFromAgentRunDecision(run, declined); len(suggestions) != 0 {
		t.Fatalf("suggestions = %#v, want none when usefulness decision declines", suggestions)
	}

	accepted := PostRunUsefulnessDecision{
		ShouldSuggest:  true,
		Reason:         "LLM 判断本次处理有复用和审计价值",
		DecidedBy:      PostRunDecisionByLLM,
		SuggestedTypes: []PostRunSuggestionType{PostRunSuggestionRunRecord, PostRunSuggestionExperienceCandidate},
	}
	suggestions := BuildPostRunSuggestionsFromAgentRunDecision(run, accepted)
	if len(suggestions) != 2 ||
		!hasPostRunSuggestion(suggestions, PostRunSuggestionRunRecord) ||
		!hasPostRunSuggestion(suggestions, PostRunSuggestionExperienceCandidate) ||
		hasPostRunSuggestion(suggestions, PostRunSuggestionCase) {
		t.Fatalf("suggestions = %#v, want only LLM-selected suggestions", suggestions)
	}
	for _, item := range suggestions {
		if item.Reason != accepted.Reason {
			t.Fatalf("suggestion reason = %q, want LLM decision reason %q", item.Reason, accepted.Reason)
		}
	}
}

func TestPostRunUsefulnessDecisionFromAgentRunIsEvidenceGated(t *testing.T) {
	chatOnly := AgentRunView{
		ID:     "run-chat-only",
		Status: AgentRunStatusCompleted,
		Steps: []AgentStepView{{
			ID:            "final-1",
			Kind:          AgentStepKindFinalResponse,
			Status:        AgentStepStatusCompleted,
			OutputSummary: "普通解释。",
		}},
	}
	if decision := BuildPostRunUsefulnessDecisionFromAgentRun(chatOnly); decision.ShouldSuggest {
		t.Fatalf("decision = %#v, want no suggestions for chat-only run", decision)
	}

	evidenceRun := AgentRunView{
		ID:     "run-with-evidence",
		Status: AgentRunStatusCompleted,
		Steps: []AgentStepView{
			{ID: "evidence-1", Kind: AgentStepKindEvidence, Status: AgentStepStatusCompleted, OutputSummary: "观测到 redis down。"},
			{ID: "final-1", Kind: AgentStepKindFinalResponse, Status: AgentStepStatusCompleted, OutputSummary: "给出处理建议。"},
		},
	}
	decision := BuildPostRunUsefulnessDecisionFromAgentRun(evidenceRun)
	if !decision.ShouldSuggest || decision.DecidedBy != PostRunDecisionBySystemEvidenceGate {
		t.Fatalf("decision = %#v, want deterministic evidence gate to allow suggestions", decision)
	}
}

func TestRunRecordCandidateUsesAgentRunEvidence(t *testing.T) {
	now := time.Date(2026, 6, 23, 10, 0, 0, 0, time.UTC)
	run := AgentRunView{
		ID:            "run-post-assets",
		SessionID:     "sess-1",
		RootTurnID:    "turn-1",
		UserGoal:      "排查 nginx upstream 502",
		Status:        AgentRunStatusCompleted,
		TargetSummary: "@web-1 nginx",
		StartedAt:     now,
		UpdatedAt:     now.Add(2 * time.Minute),
		Steps: []AgentStepView{
			{ID: "reason-1", Kind: AgentStepKindReasoning, Status: AgentStepStatusCompleted, OutputSummary: "初步判断 upstream 连接失败。"},
			{ID: "search-1", Kind: AgentStepKindToolSearch, Status: AgentStepStatusCompleted, ToolName: "web_search", OutputSummary: "查阅 nginx upstream 502 与 keepalive 文档。"},
			{ID: "evidence-1", Kind: AgentStepKindEvidence, Status: AgentStepStatusCompleted, OutputSummary: "nginx error.log 出现 connect() failed (111: Connection refused)。"},
			{ID: "approval-1", Kind: AgentStepKindApproval, Status: AgentStepStatusCompleted, OutputSummary: "用户确认允许重启 upstream 服务。"},
			{ID: "tool-1", Kind: AgentStepKindToolCall, Status: AgentStepStatusCompleted, ToolName: "host_exec", OutputSummary: "执行 systemctl restart app。"},
			{ID: "skip-1", Kind: AgentStepKindToolCall, Status: AgentStepStatusSkipped, ToolName: "workflow", OutputSummary: "跳过不匹配的 Runner Workflow。"},
			{ID: "final-1", Kind: AgentStepKindFinalResponse, Status: AgentStepStatusCompleted, OutputSummary: "502 消失，但未声明自动验证成功。"},
		},
	}

	candidate := BuildRunRecordCandidateFromAgentRun(run)
	if candidate.ID != "run-record-run-post-assets" || candidate.OpsRunID != "run-post-assets" {
		t.Fatalf("candidate identity = %#v, want run-bound run record candidate", candidate)
	}
	if candidate.Status != "candidate" || candidate.TargetSummary != "@web-1 nginx" {
		t.Fatalf("candidate status/target = %#v, want candidate preserving target summary", candidate)
	}
	for _, category := range []PostRunSourceCategory{
		PostRunSourceObservedFacts,
		PostRunSourceInferredFacts,
		PostRunSourceExternalKnowledge,
		PostRunSourceUserDecisions,
		PostRunSourceExecutedCommands,
		PostRunSourceSkippedActions,
	} {
		if !postRunCandidateHasSourceCategory(candidate, category) {
			t.Fatalf("candidate sources = %#v, missing category %q", candidate.Sources, category)
		}
	}
	if candidate.VerifiedSuccess {
		t.Fatalf("candidate must not claim verified success without an explicit source: %#v", candidate)
	}
}

func hasPostRunSuggestion(items []PostRunSuggestion, typ PostRunSuggestionType) bool {
	for _, item := range items {
		if item.Type == typ {
			return true
		}
	}
	return false
}

func postRunCandidateHasSourceCategory(candidate PostRunAssetCandidate, category PostRunSourceCategory) bool {
	for _, source := range candidate.Sources {
		if source.Category == category {
			return true
		}
	}
	return false
}
