package workfloweditor

import (
	"testing"

	"aiops-v2/internal/opsmanual"
)

func TestProposeOpsManualCandidateUsesWorkflowRefAndPendingReview(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	result, err := service.ProposeOpsManualCandidate(testContext(), WorkflowManualCandidateRequest{WorkflowID: record.ID})
	if err != nil {
		t.Fatalf("ProposeOpsManualCandidate() error = %v", err)
	}
	if result.WorkflowRef.WorkflowID != record.ID || result.WorkflowRef.WorkflowDigest == "" {
		t.Fatalf("workflow ref = %#v, want workflow id and digest", result.WorkflowRef)
	}
	if result.Candidate.SourceType != "workflow_ai_generated" ||
		result.Candidate.ProposedManual.Metadata["source_type"] != "workflow_ai_generated" ||
		result.Candidate.ReviewStatus != "pending" ||
		result.Candidate.ProposedManual.Status == opsmanual.ManualStatusVerified {
		t.Fatalf("candidate = %#v, want pending non-verified candidate", result.Candidate)
	}
}

func TestProposeOpsManualCandidateReportsStaleBinding(t *testing.T) {
	service, store, record := newWorkflowEditorTestService()
	graph := workflowEditorTestGraph()
	graph.UI["ops_manual_candidate"] = map[string]any{
		"candidate_id":    "candidate-old",
		"review_status":   "pending",
		"workflow_digest": "sha256:old",
	}
	store.PutWorkflow(WorkflowRecord{ID: record.ID, Graph: graph})
	result, err := service.ProposeOpsManualCandidate(testContext(), WorkflowManualCandidateRequest{WorkflowID: record.ID})
	if err != nil {
		t.Fatalf("ProposeOpsManualCandidate() error = %v", err)
	}
	if !result.StaleBinding || result.StaleReason == "" {
		t.Fatalf("result = %#v, want stale binding reason", result)
	}
}

func TestProposeOpsManualUpdateReturnsPendingUpdateCandidate(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	result, err := service.ProposeOpsManualUpdate(testContext(), WorkflowManualCandidateRequest{
		WorkflowID:             record.ID,
		ManualID:               "manual-redis-memory",
		PreviousManualVersion:  "v1",
		ExpectedWorkflowDigest: record.Revision,
	})
	if err != nil {
		t.Fatalf("ProposeOpsManualUpdate() error = %v", err)
	}
	if result.Candidate.SourceType != "workflow_ai_update_candidate" ||
		result.Candidate.ReviewStatus != "pending" ||
		result.Candidate.ProposedManual.Status == opsmanual.ManualStatusVerified ||
		!containsString(result.Candidate.SourceRefs, "manual-redis-memory") ||
		!containsString(result.Candidate.SourceRefs, "v1") {
		t.Fatalf("candidate = %#v, want pending update candidate with source refs", result.Candidate)
	}
}
