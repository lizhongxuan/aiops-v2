package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"
)

const CompactSummarySchemaVersionV1 = "compact_summary_v1"

type CompactSummaryV1 struct {
	SchemaVersion      string                        `json:"schemaVersion"`
	UserGoal           string                        `json:"userGoal"`
	CurrentProfile     string                        `json:"currentProfile,omitempty"`
	TargetRefs         []string                      `json:"targetRefs,omitempty"`
	LatestUserMessages []CompactSummaryMessageRefV1  `json:"latestUserMessages"`
	ActiveConstraints  []string                      `json:"activeConstraints"`
	CurrentTask        CompactSummaryCurrentTaskV1   `json:"currentTask"`
	ConfirmedFacts     []CompactSummaryFactV1        `json:"confirmedFacts"`
	Inferences         []CompactSummaryInferenceV1   `json:"inferences,omitempty"`
	OpenQuestions      []string                      `json:"openQuestions"`
	Decisions          []CompactSummaryDecisionV1    `json:"decisions"`
	Artifacts          []CompactSummaryArtifactV1    `json:"artifacts"`
	PendingApprovals   []CompactSummaryPendingItemV1 `json:"pendingApprovals"`
	PendingEvidence    []CompactSummaryPendingItemV1 `json:"pendingEvidence"`
	RejectedApprovals  []CompactSummaryPendingItemV1 `json:"rejectedApprovals,omitempty"`
	ToolPacksLoaded    []string                      `json:"toolPacksLoaded,omitempty"`
	PlanState          CompactSummaryPlanStateV1     `json:"planState"`
	NextStep           CompactSummaryNextStepV1      `json:"nextStep"`
}

type CompactSummaryMessageRefV1 struct {
	TurnID string `json:"turnId"`
	Quote  string `json:"quote"`
}

type CompactSummaryCurrentTaskV1 struct {
	Description  string `json:"description"`
	SourceTurnID string `json:"sourceTurnId,omitempty"`
}

type CompactSummaryFactV1 struct {
	Statement string `json:"statement"`
	SourceRef string `json:"sourceRef"`
}

type CompactSummaryDecisionV1 struct {
	Decision  string `json:"decision"`
	SourceRef string `json:"sourceRef,omitempty"`
}

type CompactSummaryInferenceV1 struct {
	Statement  string `json:"statement"`
	Confidence string `json:"confidence,omitempty"`
	SourceRef  string `json:"sourceRef,omitempty"`
}

type CompactSummaryArtifactV1 struct {
	ID        string `json:"id"`
	SourceRef string `json:"sourceRef,omitempty"`
	Summary   string `json:"summary,omitempty"`
}

type CompactSummaryPendingItemV1 struct {
	ID        string `json:"id"`
	SourceRef string `json:"sourceRef,omitempty"`
}

type CompactSummaryPlanStateV1 struct {
	Status      string `json:"status,omitempty"`
	CurrentStep string `json:"currentStep,omitempty"`
}

type CompactSummaryNextStepV1 struct {
	Action          string `json:"action"`
	SourceTurnID    string `json:"sourceTurnId"`
	RecentUserQuote string `json:"recentUserQuote"`
}

func (s CompactSummaryV1) Validate() error {
	if strings.TrimSpace(s.SchemaVersion) != CompactSummarySchemaVersionV1 {
		return fmt.Errorf("schemaVersion must be %q", CompactSummarySchemaVersionV1)
	}
	if strings.TrimSpace(s.UserGoal) == "" {
		return fmt.Errorf("userGoal is required")
	}
	if len(s.LatestUserMessages) == 0 {
		return fmt.Errorf("latestUserMessages is required")
	}
	for i, msg := range s.LatestUserMessages {
		if strings.TrimSpace(msg.TurnID) == "" {
			return fmt.Errorf("latestUserMessages[%d].turnId is required", i)
		}
		if strings.TrimSpace(msg.Quote) == "" {
			return fmt.Errorf("latestUserMessages[%d].quote is required", i)
		}
	}
	if strings.TrimSpace(s.CurrentTask.Description) == "" {
		return fmt.Errorf("currentTask.description is required")
	}
	for i, fact := range s.ConfirmedFacts {
		if strings.TrimSpace(fact.SourceRef) == "" {
			return fmt.Errorf("confirmedFacts[%d].sourceRef is required", i)
		}
	}
	if strings.TrimSpace(s.NextStep.Action) == "" {
		return fmt.Errorf("nextStep.action is required")
	}
	if strings.TrimSpace(s.NextStep.SourceTurnID) == "" {
		return fmt.Errorf("nextStep.sourceTurnId is required")
	}
	if strings.TrimSpace(s.NextStep.RecentUserQuote) == "" {
		return fmt.Errorf("nextStep.recentUserQuote is required")
	}
	return nil
}

func ParseCompactSummaryV1(raw string) (CompactSummaryV1, error) {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	var summary CompactSummaryV1
	if err := json.Unmarshal([]byte(raw), &summary); err != nil {
		return CompactSummaryV1{}, fmt.Errorf("parse compact summary v1: %w", err)
	}
	if err := summary.Validate(); err != nil {
		return CompactSummaryV1{}, err
	}
	return summary, nil
}
