package appui

import (
	"strings"
	"time"
)

type PostRunSuggestionType string

const (
	PostRunSuggestionRunRecord           PostRunSuggestionType = "run_record"
	PostRunSuggestionProcessingRecord    PostRunSuggestionType = "processing_record"
	PostRunSuggestionExperienceCandidate PostRunSuggestionType = "experience_candidate"
	PostRunSuggestionCase                PostRunSuggestionType = "case"
	PostRunSuggestionPostmortem          PostRunSuggestionType = "postmortem"
)

type PostRunSourceCategory string

const (
	PostRunSourceObservedFacts     PostRunSourceCategory = "observed_facts"
	PostRunSourceInferredFacts     PostRunSourceCategory = "inferred_facts"
	PostRunSourceExternalKnowledge PostRunSourceCategory = "external_knowledge"
	PostRunSourceUserDecisions     PostRunSourceCategory = "user_decisions"
	PostRunSourceExecutedCommands  PostRunSourceCategory = "executed_commands"
	PostRunSourceSkippedActions    PostRunSourceCategory = "skipped_actions"
)

type PostRunSuggestion struct {
	Type   PostRunSuggestionType `json:"type"`
	Label  string                `json:"label"`
	Reason string                `json:"reason,omitempty"`
}

type PostRunDecisionSource string

const (
	PostRunDecisionByLLM                PostRunDecisionSource = "llm"
	PostRunDecisionBySystemEvidenceGate PostRunDecisionSource = "system_evidence_gate"
)

type PostRunUsefulnessDecision struct {
	ShouldSuggest  bool                    `json:"shouldSuggest"`
	Reason         string                  `json:"reason,omitempty"`
	DecidedBy      PostRunDecisionSource   `json:"decidedBy,omitempty"`
	SuggestedTypes []PostRunSuggestionType `json:"suggestedTypes,omitempty"`
}

type PostRunEvidenceSource struct {
	Category     PostRunSourceCategory `json:"category"`
	StepID       string                `json:"stepId,omitempty"`
	StepKind     AgentStepKind         `json:"stepKind,omitempty"`
	StepStatus   AgentStepStatus       `json:"stepStatus,omitempty"`
	ToolName     string                `json:"toolName,omitempty"`
	Summary      string                `json:"summary,omitempty"`
	TargetRefs   []string              `json:"targetRefs,omitempty"`
	EvidenceRefs []string              `json:"evidenceRefs,omitempty"`
}

type PostRunAssetCandidate struct {
	ID              string                  `json:"id"`
	OpsRunID        string                  `json:"opsRunId"`
	Type            PostRunSuggestionType   `json:"type"`
	Status          string                  `json:"status"`
	Title           string                  `json:"title"`
	Summary         string                  `json:"summary,omitempty"`
	TargetSummary   string                  `json:"targetSummary,omitempty"`
	SessionID       string                  `json:"sessionId,omitempty"`
	RootTurnID      string                  `json:"rootTurnId,omitempty"`
	StartedAt       time.Time               `json:"startedAt,omitempty"`
	UpdatedAt       time.Time               `json:"updatedAt,omitempty"`
	Sources         []PostRunEvidenceSource `json:"sources,omitempty"`
	VerifiedSuccess bool                    `json:"verifiedSuccess,omitempty"`
}

func BuildPostRunSuggestionsFromAgentRun(run AgentRunView) []PostRunSuggestion {
	return BuildPostRunSuggestionsFromAgentRunDecision(run, BuildPostRunUsefulnessDecisionFromAgentRun(run))
}

func BuildPostRunUsefulnessDecisionFromAgentRun(run AgentRunView) PostRunUsefulnessDecision {
	if !postRunTerminal(run.Status) {
		return PostRunUsefulnessDecision{
			ShouldSuggest: false,
			Reason:        "运维运行尚未结束，暂不生成沉淀入口。",
			DecidedBy:     PostRunDecisionBySystemEvidenceGate,
		}
	}
	sources := postRunSourcesFromAgentRun(run)
	if !postRunHasReusableValue(sources) {
		return PostRunUsefulnessDecision{
			ShouldSuggest: false,
			Reason:        "本次对话没有可追溯证据、执行记录或用户决策，暂不生成沉淀入口。",
			DecidedBy:     PostRunDecisionBySystemEvidenceGate,
		}
	}
	return PostRunUsefulnessDecision{
		ShouldSuggest:  true,
		Reason:         "本次处理包含可追溯证据、用户决策或执行记录，可沉淀为后续复用/审计材料。",
		DecidedBy:      PostRunDecisionBySystemEvidenceGate,
		SuggestedTypes: defaultPostRunSuggestionTypes(),
	}
}

func BuildPostRunSuggestionsFromAgentRunDecision(run AgentRunView, decision PostRunUsefulnessDecision) []PostRunSuggestion {
	if !postRunTerminal(run.Status) || !decision.ShouldSuggest {
		return nil
	}
	if !postRunHasReusableValue(postRunSourcesFromAgentRun(run)) {
		return nil
	}
	reason := strings.TrimSpace(decision.Reason)
	if reason == "" {
		reason = "本次处理包含可追溯证据、用户决策或执行记录，可沉淀为后续复用/审计材料。"
	}
	types := normalizePostRunSuggestionTypes(decision.SuggestedTypes)
	out := make([]PostRunSuggestion, 0, len(types))
	for _, typ := range types {
		out = append(out, PostRunSuggestion{
			Type:   typ,
			Label:  postRunSuggestionLabel(typ),
			Reason: reason,
		})
	}
	return out
}

func BuildRunRecordCandidateFromAgentRun(run AgentRunView) PostRunAssetCandidate {
	return buildPostRunAssetCandidate(run, PostRunSuggestionRunRecord, "run-record-", "Chat 运维处理记录")
}

func BuildExperienceCandidateFromAgentRun(run AgentRunView) PostRunAssetCandidate {
	return buildPostRunAssetCandidate(run, PostRunSuggestionExperienceCandidate, "experience-candidate-", "Chat 运维经验候选")
}

func BuildCaseCandidateFromAgentRun(run AgentRunView) PostRunAssetCandidate {
	return buildPostRunAssetCandidate(run, PostRunSuggestionCase, "case-candidate-", "Chat 运维 Case 候选")
}

func buildPostRunAssetCandidate(run AgentRunView, typ PostRunSuggestionType, idPrefix, fallbackTitle string) PostRunAssetCandidate {
	opsRunID := strings.TrimSpace(run.ID)
	return PostRunAssetCandidate{
		ID:            idPrefix + safeArchiveIDPart(opsRunID),
		OpsRunID:      opsRunID,
		Type:          typ,
		Status:        "candidate",
		Title:         firstNonEmptyString(strings.TrimSpace(run.UserGoal), strings.TrimSpace(run.NormalizedGoal), fallbackTitle),
		Summary:       postRunFinalSummary(run),
		TargetSummary: strings.TrimSpace(run.TargetSummary),
		SessionID:     strings.TrimSpace(run.SessionID),
		RootTurnID:    strings.TrimSpace(run.RootTurnID),
		StartedAt:     run.StartedAt,
		UpdatedAt:     run.UpdatedAt,
		Sources:       postRunSourcesFromAgentRun(run),
	}
}

func postRunSourcesFromAgentRun(run AgentRunView) []PostRunEvidenceSource {
	out := make([]PostRunEvidenceSource, 0, len(run.Steps))
	for _, step := range run.Steps {
		category, ok := postRunSourceCategory(step)
		if !ok {
			continue
		}
		summary := firstNonEmptyString(strings.TrimSpace(step.OutputSummary), strings.TrimSpace(step.InputSummary), strings.TrimSpace(step.Title))
		if summary == "" {
			continue
		}
		out = append(out, PostRunEvidenceSource{
			Category:     category,
			StepID:       strings.TrimSpace(step.ID),
			StepKind:     step.Kind,
			StepStatus:   step.Status,
			ToolName:     strings.TrimSpace(step.ToolName),
			Summary:      summary,
			TargetRefs:   cloneStringSlice(step.TargetRefs),
			EvidenceRefs: cloneStringSlice(step.EvidenceRefs),
		})
	}
	return out
}

func postRunSourceCategory(step AgentStepView) (PostRunSourceCategory, bool) {
	if step.Status == AgentStepStatusSkipped {
		return PostRunSourceSkippedActions, true
	}
	switch step.Kind {
	case AgentStepKindEvidence, AgentStepKindMCPHealth:
		return PostRunSourceObservedFacts, true
	case AgentStepKindApproval:
		return PostRunSourceUserDecisions, true
	case AgentStepKindToolCall:
		if postRunClassifiesExecutedCommand(step.ToolName) {
			return PostRunSourceExecutedCommands, true
		}
		return PostRunSourceObservedFacts, true
	case AgentStepKindToolSearch:
		if postRunClassifiesExternalKnowledge(step.ToolName, step.Title, step.InputSummary, step.OutputSummary) {
			return PostRunSourceExternalKnowledge, true
		}
		return "", false
	case AgentStepKindReasoning, AgentStepKindFinalResponse:
		return PostRunSourceInferredFacts, true
	default:
		return "", false
	}
}

func postRunClassifiesExecutedCommand(toolName string) bool {
	lower := strings.ToLower(strings.TrimSpace(toolName))
	for _, marker := range []string{"exec", "command", "shell", "ssh", "kubectl", "systemctl", "host_"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	return false
}

func postRunClassifiesExternalKnowledge(values ...string) bool {
	joined := strings.ToLower(strings.Join(values, " "))
	for _, marker := range []string{"web", "http", "docs", "documentation", "learn", "search", "external"} {
		if strings.Contains(joined, marker) {
			return true
		}
	}
	return false
}

func postRunHasReusableValue(sources []PostRunEvidenceSource) bool {
	for _, source := range sources {
		switch source.Category {
		case PostRunSourceObservedFacts, PostRunSourceExternalKnowledge, PostRunSourceUserDecisions, PostRunSourceExecutedCommands, PostRunSourceSkippedActions:
			return true
		}
	}
	return false
}

func postRunTerminal(status AgentRunStatus) bool {
	return status == AgentRunStatusCompleted || status == AgentRunStatusFailed || status == AgentRunStatusCancelled
}

func defaultPostRunSuggestionTypes() []PostRunSuggestionType {
	return []PostRunSuggestionType{
		PostRunSuggestionRunRecord,
		PostRunSuggestionProcessingRecord,
		PostRunSuggestionExperienceCandidate,
		PostRunSuggestionCase,
		PostRunSuggestionPostmortem,
	}
}

func normalizePostRunSuggestionTypes(types []PostRunSuggestionType) []PostRunSuggestionType {
	if len(types) == 0 {
		return defaultPostRunSuggestionTypes()
	}
	seen := map[PostRunSuggestionType]bool{}
	out := make([]PostRunSuggestionType, 0, len(types))
	for _, typ := range types {
		if !validPostRunSuggestionType(typ) || seen[typ] {
			continue
		}
		seen[typ] = true
		out = append(out, typ)
	}
	if len(out) == 0 {
		return defaultPostRunSuggestionTypes()
	}
	return out
}

func validPostRunSuggestionType(typ PostRunSuggestionType) bool {
	switch typ {
	case PostRunSuggestionRunRecord,
		PostRunSuggestionProcessingRecord,
		PostRunSuggestionExperienceCandidate,
		PostRunSuggestionCase,
		PostRunSuggestionPostmortem:
		return true
	default:
		return false
	}
}

func postRunSuggestionLabel(typ PostRunSuggestionType) string {
	switch typ {
	case PostRunSuggestionRunRecord:
		return "生成 Run Record"
	case PostRunSuggestionProcessingRecord:
		return "生成处理记录"
	case PostRunSuggestionExperienceCandidate:
		return "生成经验候选"
	case PostRunSuggestionCase:
		return "生成 Case"
	case PostRunSuggestionPostmortem:
		return "生成复盘"
	default:
		return "生成沉淀"
	}
}

func postRunFinalSummary(run AgentRunView) string {
	for i := len(run.Steps) - 1; i >= 0; i-- {
		step := run.Steps[i]
		if step.Kind != AgentStepKindFinalResponse {
			continue
		}
		if summary := firstNonEmptyString(strings.TrimSpace(step.OutputSummary), strings.TrimSpace(step.InputSummary), strings.TrimSpace(step.Title)); summary != "" {
			return summary
		}
	}
	for i := len(run.Steps) - 1; i >= 0; i-- {
		if summary := firstNonEmptyString(strings.TrimSpace(run.Steps[i].OutputSummary), strings.TrimSpace(run.Steps[i].InputSummary), strings.TrimSpace(run.Steps[i].Title)); summary != "" {
			return summary
		}
	}
	return ""
}

func cloneStringSlice(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}
