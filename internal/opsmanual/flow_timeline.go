package opsmanual

import (
	"fmt"
	"sort"
	"strings"
)

type FlowTimelineEvent struct {
	ID              string `json:"id"`
	Type            string `json:"type"`
	OpsManualFlowID string `json:"ops_manual_flow_id"`
	ManualID        string `json:"manual_id,omitempty"`
	WorkflowID      string `json:"workflow_id,omitempty"`
	SessionID       string `json:"session_id,omitempty"`
	Summary         string `json:"summary,omitempty"`
	RedactionStatus string `json:"redaction_status"`
	CreatedAt       string `json:"created_at,omitempty"`
}

type FlowTimelineResult struct {
	Items []FlowTimelineEvent `json:"items"`
	Total int                 `json:"total"`
}

func (s *Service) FlowTimeline(flowID string) (FlowTimelineResult, error) {
	flowID = strings.TrimSpace(flowID)
	if flowID == "" {
		return FlowTimelineResult{}, fmt.Errorf("ops_manual_flow_id is required")
	}
	var items []FlowTimelineEvent
	if repo, ok := s.repo.(ParamResolutionEventRepository); ok {
		events, err := repo.ListParamResolutionEvents(ListParamResolutionEventsRequest{OpsManualFlowID: flowID, Limit: 1000})
		if err != nil {
			return FlowTimelineResult{}, err
		}
		for _, event := range events {
			items = append(items, FlowTimelineEvent{
				ID:              "search:" + event.ID,
				Type:            "search",
				OpsManualFlowID: event.OpsManualFlowID,
				ManualID:        event.ManualID,
				WorkflowID:      event.WorkflowID,
				SessionID:       event.SessionID,
				Summary:         "manual_selected",
				RedactionStatus: "redacted",
				CreatedAt:       event.CreatedAt,
			})
			items = append(items, FlowTimelineEvent{
				ID:              event.ID,
				Type:            "param_resolution",
				OpsManualFlowID: event.OpsManualFlowID,
				ManualID:        event.ManualID,
				WorkflowID:      event.WorkflowID,
				SessionID:       event.SessionID,
				Summary:         string(event.Result.Status),
				RedactionStatus: "redacted",
				CreatedAt:       event.CreatedAt,
			})
			if summary := userFormSubmitSummary(event.Result); summary != "" {
				items = append(items, FlowTimelineEvent{
					ID:              "user-form-submit:" + event.ID,
					Type:            "user_form_submit",
					OpsManualFlowID: event.OpsManualFlowID,
					ManualID:        event.ManualID,
					WorkflowID:      event.WorkflowID,
					SessionID:       event.SessionID,
					Summary:         summary,
					RedactionStatus: "redacted",
					CreatedAt:       event.CreatedAt,
				})
			}
		}
	}
	if repo, ok := s.repo.(ManualRepository); ok {
		records, err := repo.ListRunRecords(ListRunRecordsRequest{OpsManualFlowID: flowID, Limit: 1000})
		if err != nil {
			return FlowTimelineResult{}, err
		}
		for _, record := range records {
			items = append(items, runRecordTimelineEvents(record)...)
		}
	}
	if repo, ok := s.repo.(ManualGuidedChatEventRepository); ok {
		events, err := repo.ListManualGuidedChatEvents(ListManualGuidedChatEventsRequest{OpsManualFlowID: flowID, Limit: 1000})
		if err != nil {
			return FlowTimelineResult{}, err
		}
		for _, event := range events {
			items = append(items, FlowTimelineEvent{
				ID:              event.ID,
				Type:            "manual_guided_reference",
				OpsManualFlowID: event.OpsManualFlowID,
				ManualID:        event.ManualID,
				WorkflowID:      event.WorkflowID,
				SessionID:       event.SessionID,
				Summary:         event.StageSummary,
				RedactionStatus: event.RedactionStatus,
				CreatedAt:       event.CreatedAt,
			})
		}
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].CreatedAt == items[j].CreatedAt {
			return items[i].ID < items[j].ID
		}
		return items[i].CreatedAt < items[j].CreatedAt
	})
	return FlowTimelineResult{Items: items, Total: len(items)}, nil
}

func userFormSubmitSummary(result ParamResolutionResult) string {
	var ids []string
	for _, param := range result.ResolvedParams {
		if strings.EqualFold(strings.TrimSpace(param.Source), "user_form") {
			ids = appendUnique(ids, param.ID)
		}
	}
	return strings.Join(ids, ",")
}

func runRecordTimelineEvents(record RunRecord) []FlowTimelineEvent {
	base := FlowTimelineEvent{
		OpsManualFlowID: record.OpsManualFlowID,
		ManualID:        record.ManualID,
		WorkflowID:      record.WorkflowID,
		SessionID:       record.SessionID,
		RedactionStatus: "redacted",
		CreatedAt:       firstNonEmpty(record.CompletedAt, record.StartedAt),
	}
	var out []FlowTimelineEvent
	if strings.TrimSpace(record.PreflightStatus) != "" {
		event := base
		event.ID = "preflight:" + record.ID
		event.Type = "preflight"
		event.Summary = strings.TrimSpace(record.PreflightStatus)
		out = append(out, event)
	}
	if strings.TrimSpace(record.DryRunStatus) != "" {
		event := base
		event.ID = "dry-run:" + record.ID
		event.Type = "dry_run"
		event.Summary = strings.TrimSpace(record.DryRunStatus)
		out = append(out, event)
	}
	if strings.TrimSpace(record.ExecutionStatus) != "" {
		event := base
		event.ID = "execution:" + record.ID
		event.Type = "execution"
		event.Summary = strings.TrimSpace(record.ExecutionStatus)
		out = append(out, event)
	}
	if strings.TrimSpace(record.ValidationStatus) != "" {
		event := base
		event.ID = "verification:" + record.ID
		event.Type = "verification"
		event.Summary = strings.TrimSpace(record.ValidationStatus)
		out = append(out, event)
	}
	if strings.TrimSpace(record.UserFeedback) != "" {
		event := base
		event.ID = "user-feedback:" + record.ID
		event.Type = "user_feedback"
		event.Summary = strings.TrimSpace(record.UserFeedback)
		out = append(out, event)
	}
	return out
}
