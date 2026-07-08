package server

import (
	"encoding/json"
	"net/http"
	"strings"

	"aiops-v2/internal/workfloweditor"
)

const runnerStudioWorkflowAIPrefix = "/api/runner-studio/workflow-ai/"

func (s *HTTPServer) handleRunnerStudioWorkflowAI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := s.workflowEditor
	if service == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workflow editor service is not available"})
		return
	}
	action := strings.Trim(strings.TrimPrefix(r.URL.EscapedPath(), runnerStudioWorkflowAIPrefix), "/")
	switch action {
	case "sessions":
		var req workfloweditor.CreateSessionRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.CreateSession(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "snapshot":
		var req workfloweditor.GetSnapshotRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.GetSnapshot(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "step":
		var req workfloweditor.GetStepRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.GetStep(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "describe":
		var req workfloweditor.DescribeRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.Describe(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "plan":
		var req workfloweditor.ProposeEditPlanRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.ProposeEditPlan(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "patch":
		var req workfloweditor.ProposePatchRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.ProposePatch(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "validate":
		var req workfloweditor.ValidatePatchRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.ValidatePatch(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "preview":
		var req workfloweditor.PreviewPatchRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.PreviewPatch(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "effect":
		var req workfloweditor.DetectPatchEffectRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.DetectPatchEffect(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "apply":
		var req workfloweditor.ApplyPatchRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.ApplyPatch(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "undo":
		var req workfloweditor.UndoLastAIPatchRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.UndoLastAIPatch(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "manual-candidate":
		var req workfloweditor.WorkflowManualCandidateRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.ProposeOpsManualCandidate(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "manual-update":
		var req workfloweditor.WorkflowManualCandidateRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.ProposeOpsManualUpdate(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	case "create-draft":
		var req workfloweditor.WorkflowDraftFromPlanRequest
		if !decodeRunnerStudioWorkflowAIRequest(w, r, &req) {
			return
		}
		result, err := service.CreateDraftFromConfirmedPlan(r.Context(), req)
		writeRunnerStudioWorkflowAIResult(w, result, err)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runner studio workflow ai endpoint not found"})
	}
}

func decodeRunnerStudioWorkflowAIRequest(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func writeRunnerStudioWorkflowAIResult(w http.ResponseWriter, result any, err error) {
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, result)
}
