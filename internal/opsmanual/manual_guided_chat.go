package opsmanual

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type ManualGuidedChatEvent struct {
	ID                string   `json:"id"`
	SessionID         string   `json:"session_id,omitempty"`
	TurnID            string   `json:"turn_id,omitempty"`
	OpsManualFlowID   string   `json:"ops_manual_flow_id,omitempty"`
	ManualID          string   `json:"manual_id,omitempty"`
	WorkflowID        string   `json:"workflow_id,omitempty"`
	ReferenceMode     string   `json:"reference_mode"`
	StageSummary      string   `json:"stage_summary,omitempty"`
	EvidenceRefs      []string `json:"evidence_refs,omitempty"`
	MutationRequested bool     `json:"mutation_requested,omitempty"`
	WorkflowRunID     string   `json:"workflow_run_id,omitempty"`
	RedactionStatus   string   `json:"redaction_status"`
	CreatedAt         string   `json:"created_at"`
}

type ListManualGuidedChatEventsRequest struct {
	OpsManualFlowID string
	SessionID       string
	ManualID        string
	WorkflowID      string
	Limit           int
}

type ManualGuidedChatEventRepository interface {
	SaveManualGuidedChatEvent(ManualGuidedChatEvent) error
	ListManualGuidedChatEvents(ListManualGuidedChatEventsRequest) ([]ManualGuidedChatEvent, error)
}

func (s *Service) RecordManualGuidedChatEventFromMetadata(ctx context.Context, sessionID string, requestText string, metadata map[string]any) error {
	repo, ok := s.repo.(ManualGuidedChatEventRepository)
	if !ok {
		return nil
	}
	event := ManualGuidedChatEvent{
		ID:                "manual-guided-" + time.Now().UTC().Format("20060102T150405.000000000Z"),
		SessionID:         strings.TrimSpace(sessionID),
		TurnID:            firstMetadataAnyValue(metadata, "turn_id", "turnId"),
		OpsManualFlowID:   firstMetadataAnyValue(metadata, "opsManualFlowId", "ops_manual_flow_id"),
		ManualID:          firstMetadataAnyValue(metadata, "opsManualManualId", "manualId", "manual_id"),
		WorkflowID:        firstMetadataAnyValue(metadata, "opsManualWorkflowId", "workflowId", "workflow_id"),
		ReferenceMode:     "manual_guided_chat",
		StageSummary:      strings.TrimSpace(requestText),
		EvidenceRefs:      metadataStringSliceFromAny(firstAny(metadata["evidence_refs"], metadata["evidenceRefs"])),
		MutationRequested: metadataBool(metadata, "mutation_requested"),
		WorkflowRunID:     "",
		RedactionStatus:   "redacted",
		CreatedAt:         time.Now().UTC().Format(time.RFC3339),
	}
	if event.OpsManualFlowID == "" && event.ManualID == "" {
		return fmt.Errorf("manual guided chat event requires flow id or manual id")
	}
	return repo.SaveManualGuidedChatEvent(event)
}

func manualGuidedChatEventMatchesRequest(event ManualGuidedChatEvent, req ListManualGuidedChatEventsRequest) bool {
	if req.OpsManualFlowID != "" && event.OpsManualFlowID != req.OpsManualFlowID {
		return false
	}
	if req.SessionID != "" && event.SessionID != req.SessionID {
		return false
	}
	if req.ManualID != "" && event.ManualID != req.ManualID {
		return false
	}
	if req.WorkflowID != "" && event.WorkflowID != req.WorkflowID {
		return false
	}
	return true
}

func cloneManualGuidedChatEvent(in ManualGuidedChatEvent) ManualGuidedChatEvent {
	out := in
	out.EvidenceRefs = cloneStrings(in.EvidenceRefs)
	return out
}
