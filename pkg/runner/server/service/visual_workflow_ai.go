package service

import (
	"context"
	"fmt"
	"strings"

	"runner/workflow/visual"
)

type VisualWorkflowAIDraftRequest struct {
	WorkflowName   string       `json:"workflow_name,omitempty"`
	WorkflowStatus string       `json:"workflow_status,omitempty"`
	Instruction    string       `json:"instruction"`
	Graph          visual.Graph `json:"graph"`
}

type VisualWorkflowGraphPatch struct {
	Operations []map[string]any `json:"operations"`
}

type VisualWorkflowAIDraftResponse struct {
	GraphPatch     VisualWorkflowGraphPatch `json:"graph_patch"`
	CandidateGraph visual.Graph             `json:"candidate_graph"`
	DiffSummary    map[string]any           `json:"diff_summary"`
}

func (s *VisualWorkflowService) GenerateAIDraft(_ context.Context, req VisualWorkflowAIDraftRequest) (*VisualWorkflowAIDraftResponse, error) {
	if strings.TrimSpace(req.Instruction) == "" {
		return nil, fmt.Errorf("%w: instruction is required", ErrInvalid)
	}
	if status := strings.ToLower(strings.TrimSpace(req.WorkflowStatus)); status != "" && status != WorkflowStatusDraft {
		return nil, fmt.Errorf("%w: AI draft is only allowed for draft workflows", ErrConflict)
	}

	return nil, fmt.Errorf("%w: legacy AI draft generation is retired; use AssistantTransport workflow_agent with the Runner action catalog", ErrInvalid)
}
