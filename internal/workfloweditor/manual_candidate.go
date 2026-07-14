package workfloweditor

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/opsmanual"
	"gopkg.in/yaml.v3"
)

func (s *Service) ProposeOpsManualCandidate(ctx context.Context, req WorkflowManualCandidateRequest) (WorkflowManualCandidateResult, error) {
	record, err := s.store.GetWorkflow(ctx, req.WorkflowID)
	if err != nil {
		return WorkflowManualCandidateResult{}, err
	}
	raw, err := yaml.Marshal(record.Graph.Workflow)
	if err != nil {
		return WorkflowManualCandidateResult{}, fmt.Errorf("marshal workflow: %w", err)
	}
	ref := opsmanual.WorkflowRef{
		WorkflowID:      record.ID,
		WorkflowVersion: record.Revision,
		WorkflowDigest:  RevisionDigest(record.Graph),
	}
	candidate, err := opsmanual.GenerateCandidateFromWorkflowDraft(opsmanual.WorkflowDraftInput{
		WorkflowID:      ref.WorkflowID,
		WorkflowVersion: ref.WorkflowVersion,
		WorkflowDigest:  ref.WorkflowDigest,
		YAML:            string(raw),
		Metadata: map[string]any{
			"source_type": "workflow_ai_generated",
			"source":      "workflow_ai_chat",
		},
	})
	if err != nil {
		return WorkflowManualCandidateResult{}, err
	}
	if candidate.ProposedManual.Metadata == nil {
		candidate.ProposedManual.Metadata = map[string]any{}
	}
	candidate.SourceType = "workflow_ai_generated"
	candidate.SourceRefs = appendUniqueManualRefs(candidate.SourceRefs, record.ID)
	candidate.ReviewStatus = "pending"
	candidate.ProposedManual.Status = opsmanual.ManualStatusDraft
	candidate.ProposedManual.Metadata["source_type"] = "workflow_ai_generated"
	stale, reason := detectStaleManualBinding(record.Graph.UI, ref.WorkflowDigest, req.ExpectedWorkflowDigest)
	return WorkflowManualCandidateResult{Candidate: candidate, WorkflowRef: ref, StaleBinding: stale, StaleReason: reason}, nil
}

func (s *Service) ProposeOpsManualUpdate(ctx context.Context, req WorkflowManualCandidateRequest) (WorkflowManualCandidateResult, error) {
	result, err := s.ProposeOpsManualCandidate(ctx, req)
	if err != nil {
		return WorkflowManualCandidateResult{}, err
	}
	manualID := strings.TrimSpace(req.ManualID)
	if manualID == "" {
		manualID = result.Candidate.ProposedManual.ID
	}
	result.Candidate.SourceType = "workflow_ai_update_candidate"
	result.Candidate.SourceRefs = appendUniqueManualRefs(result.Candidate.SourceRefs, manualID, req.PreviousManualVersion, result.WorkflowRef.WorkflowID)
	result.Candidate.ProposedManual.ID = manualID
	result.Candidate.ProposedManual.ManualFamilyID = firstNonEmpty(result.Candidate.ProposedManual.ManualFamilyID, manualID)
	if result.Candidate.ProposedManual.Metadata == nil {
		result.Candidate.ProposedManual.Metadata = map[string]any{}
	}
	result.Candidate.ProposedManual.Metadata["source_type"] = "workflow_ai_update_candidate"
	result.Candidate.ProposedManual.Metadata["previous_manual_version"] = strings.TrimSpace(req.PreviousManualVersion)
	result.Candidate.ReviewStatus = "pending"
	result.Candidate.ProposedManual.Status = opsmanual.ManualStatusDraft
	return result, nil
}

func detectStaleManualBinding(ui map[string]any, currentDigest string, expectedDigest string) (bool, string) {
	expectedDigest = strings.TrimSpace(expectedDigest)
	if expectedDigest != "" && expectedDigest != strings.TrimSpace(currentDigest) {
		return true, "expected workflow digest does not match current workflow digest"
	}
	raw, ok := ui["ops_manual_candidate"].(map[string]any)
	if !ok {
		return false, ""
	}
	boundDigest := strings.TrimSpace(stringFromAny(raw["workflow_digest"]))
	if boundDigest == "" || boundDigest == strings.TrimSpace(currentDigest) {
		return false, ""
	}
	return true, "bound manual workflow digest is stale"
}

func appendUniqueManualRefs(values []string, next ...string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values)+len(next))
	for _, value := range append(values, next...) {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
