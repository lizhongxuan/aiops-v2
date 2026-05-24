package opsmanual

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"aiops-v2/internal/memory"
)

type RedactedLearningSummary struct {
	ManualID      string
	WorkflowID    string
	WorkflowRunID string
	ObjectType    string
	Action        string
	Environment   string
	TargetAlias   string
	ResultSummary string
	UserFeedback  string
	Text          string
	Redacted      bool
	CreatedAt     time.Time
}

type LearningSummaryWriteRequest struct {
	RunRecord                   *RunRecord
	ManualGuidedEvent           *ManualGuidedChatEvent
	UserFeedback                string
	AllowWrite                  bool
	SkippedReason               string
	EnvironmentProfileCandidate bool
	Scope                       memory.Scope
	SessionID                   string
	ProjectID                   string
	Now                         time.Time
}

type LearningSummaryWriteResult struct {
	Written       bool
	SkippedReason string
	Summary       RedactedLearningSummary
	Item          memory.Item
}

type LearningMemoryWriter interface {
	Put(context.Context, memory.Item) (memory.Item, error)
}

func BuildRedactedLearningSummaryFromRunRecord(record RunRecord, userFeedback string) RedactedLearningSummary {
	frame := record.OperationFrame
	summary := RedactedLearningSummary{
		ManualID:      strings.TrimSpace(record.ManualID),
		WorkflowID:    strings.TrimSpace(record.WorkflowID),
		WorkflowRunID: strings.TrimSpace(record.ID),
		ObjectType:    strings.TrimSpace(firstNonEmpty(frame.Target.Type, frame.Operation.TargetType, frame.ObjectType)),
		Action:        strings.TrimSpace(firstNonEmpty(frame.Operation.Action, frame.OperationType)),
		Environment:   learningEnvironment(frame.Environment),
		TargetAlias:   safeLearningAlias(firstNonEmpty(learningParamString(record.RedactedParameters, "target_instance"), frame.Target.Name)),
		ResultSummary: learningRunResult(record),
		UserFeedback:  sanitizeLearningText(userFeedback),
		Redacted:      true,
		CreatedAt:     learningTime(record.CompletedAt, record.StartedAt),
	}
	summary.Text = learningSummaryText(summary)
	return summary
}

func BuildRedactedLearningSummaryFromManualGuidedEvent(event ManualGuidedChatEvent, userFeedback string) RedactedLearningSummary {
	summary := RedactedLearningSummary{
		ManualID:      strings.TrimSpace(event.ManualID),
		WorkflowID:    strings.TrimSpace(event.WorkflowID),
		WorkflowRunID: strings.TrimSpace(event.WorkflowRunID),
		ResultSummary: "manual_guided_chat",
		UserFeedback:  sanitizeLearningText(userFeedback),
		Text:          sanitizeLearningText(event.StageSummary),
		Redacted:      true,
		CreatedAt:     learningTime(event.CreatedAt),
	}
	if summary.Text == "" {
		summary.Text = learningSummaryText(summary)
	}
	return summary
}

func WriteRedactedLearningSummary(ctx context.Context, store LearningMemoryWriter, req LearningSummaryWriteRequest) (LearningSummaryWriteResult, error) {
	if !req.AllowWrite {
		return LearningSummaryWriteResult{SkippedReason: firstNonEmpty(strings.TrimSpace(req.SkippedReason), "policy_disabled")}, nil
	}
	if store == nil {
		return LearningSummaryWriteResult{SkippedReason: "memory_store_unavailable"}, nil
	}
	if req.Scope == memory.ScopeProject && !req.EnvironmentProfileCandidate && strings.TrimSpace(req.UserFeedback) == "" {
		return LearningSummaryWriteResult{SkippedReason: "not_environment_profile_candidate"}, nil
	}
	summary := RedactedLearningSummary{}
	switch {
	case req.RunRecord != nil:
		summary = BuildRedactedLearningSummaryFromRunRecord(*req.RunRecord, req.UserFeedback)
	case req.ManualGuidedEvent != nil:
		summary = BuildRedactedLearningSummaryFromManualGuidedEvent(*req.ManualGuidedEvent, req.UserFeedback)
	default:
		return LearningSummaryWriteResult{SkippedReason: "no_learning_source"}, nil
	}
	now := req.Now
	if now.IsZero() {
		now = time.Now().UTC()
	}
	scope := req.Scope
	if scope == "" {
		scope = memory.ScopeProject
	}
	item := memory.Item{
		Scope:           scope,
		SessionID:       strings.TrimSpace(req.SessionID),
		ProjectID:       strings.TrimSpace(req.ProjectID),
		Kind:            memory.KindOpsManualManualHint,
		ObjectType:      summary.ObjectType,
		OperationAction: summary.Action,
		TargetAlias:     summary.TargetAlias,
		ManualID:        summary.ManualID,
		WorkflowID:      summary.WorkflowID,
		Source:          "memory_hint",
		Redacted:        true,
		Text:            summary.Text,
		CreatedAt:       now,
	}
	written, err := store.Put(ctx, item)
	if err != nil {
		return LearningSummaryWriteResult{}, err
	}
	return LearningSummaryWriteResult{Written: true, Summary: summary, Item: written}, nil
}

func learningEnvironment(env EnvironmentProfile) string {
	return sanitizeLearningText(firstNonEmpty(env.Platform, env.Runtime, env.OS, env.Env, env.ExecutionSurface))
}

func learningRunResult(record RunRecord) string {
	return sanitizeLearningText(firstNonEmpty(record.ValidationStatus, record.ExecutionStatus, record.DryRunStatus, record.FailureReason))
}

func learningParamString(params map[string]any, key string) string {
	if params == nil {
		return ""
	}
	value, ok := params[key]
	if !ok || value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func learningTime(values ...string) time.Time {
	for _, value := range values {
		if parsed, err := time.Parse(time.RFC3339, strings.TrimSpace(value)); err == nil {
			return parsed
		}
	}
	return time.Time{}
}

func learningSummaryText(summary RedactedLearningSummary) string {
	parts := []string{}
	for _, part := range []string{
		"manual_id=" + summary.ManualID,
		"workflow_id=" + summary.WorkflowID,
		"object=" + summary.ObjectType,
		"action=" + summary.Action,
		"environment=" + summary.Environment,
		"target=" + summary.TargetAlias,
		"result=" + summary.ResultSummary,
		"feedback=" + summary.UserFeedback,
	} {
		keyValue := strings.SplitN(part, "=", 2)
		if len(keyValue) == 2 && strings.TrimSpace(keyValue[1]) != "" {
			parts = append(parts, part)
		}
	}
	return sanitizeLearningText(strings.Join(parts, "; "))
}

func safeLearningAlias(value string) string {
	value = sanitizeLearningText(value)
	if isSensitiveParameterKey(value) {
		return ""
	}
	return value
}

var learningSecretPattern = regexp.MustCompile(`(?i)\b(api[_-]?key|token|secret|password|passwd|private[_-]?key|credential)\s*[:=]\s*[^\s,;]+`)

func sanitizeLearningText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = learningSecretPattern.ReplaceAllString(value, RedactedValue)
	return strings.TrimSpace(value)
}
