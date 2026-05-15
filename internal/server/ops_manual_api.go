package server

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/opsmanual"
)

const (
	opsManualErrNotFound              = "ops_manual_not_found"
	opsManualErrCandidateNotFound     = "ops_manual_candidate_not_found"
	opsManualErrWorkflowInUse         = "workflow_in_use"
	opsManualErrWorkflowDigest        = "workflow_digest_mismatch"
	opsManualErrWorkflowVersion       = "workflow_version_locked"
	opsManualErrInvalidOperationFrame = "invalid_operation_frame"
)

type opsManualHTTPServices interface {
	OpsManualService() appui.OpsManualService
}

func (s *HTTPServer) handleOpsManuals(w http.ResponseWriter, r *http.Request) {
	service := s.opsManualService()
	if service == nil {
		writeOpsManualError(w, http.StatusNotFound, opsManualErrNotFound, "ops manual service is not configured")
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/ops-manuals"), "/")
	switch {
	case r.Method == http.MethodGet && path == "":
		status := opsmanual.ManualStatus(strings.TrimSpace(r.URL.Query().Get("status")))
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := service.ListManuals(appui.OpsManualListRequest{
			Status:           status,
			TargetType:       r.URL.Query().Get("target_type"),
			Action:           r.URL.Query().Get("action"),
			Middleware:       r.URL.Query().Get("middleware"),
			ExecutionSurface: r.URL.Query().Get("execution_surface"),
			Limit:            limit,
		})
		if err != nil {
			writeOpsManualError(w, http.StatusInternalServerError, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodPost && path == "retrieve":
		var req appui.OpsManualRetrieveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "invalid request body")
			return
		}
		if req.Text == "" && req.OperationFrame.Target.Type == "" && req.OperationFrame.Operation.Action == "" {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "operation frame or text is required")
			return
		}
		result, err := service.RetrieveManuals(req)
		if err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodPost && path == "search":
		var req opsmanual.SearchOpsManualsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "invalid request body")
			return
		}
		if req.Text == "" && req.OperationFrame.Target.Type == "" && req.OperationFrame.Operation.Action == "" {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "operation frame or text is required")
			return
		}
		result, err := service.SearchOpsManuals(req)
		if err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodGet && path == "candidates":
		result, err := service.ListCandidates()
		if err != nil {
			writeOpsManualError(w, http.StatusInternalServerError, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodGet && path == "run-records":
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := service.ListRunRecords(appui.OpsManualRunRecordsRequest{
			WorkflowID: r.URL.Query().Get("workflow_id"),
			Limit:      limit,
		})
		if err != nil {
			writeOpsManualError(w, http.StatusInternalServerError, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodPost && path == "candidates/prepare":
		var req appui.OpsManualPrepareCandidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "invalid request body")
			return
		}
		candidate, err := service.PrepareManualCandidate(req)
		if err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"candidate": candidate})
	case r.Method == http.MethodPost && strings.HasPrefix(path, "candidates/") && strings.HasSuffix(path, "/confirm"):
		id := strings.TrimSuffix(strings.TrimPrefix(path, "candidates/"), "/confirm")
		var req appui.OpsManualReviewRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "invalid request body")
			return
		}
		manual, err := service.ConfirmManualCandidate(id, req)
		if err != nil {
			writeOpsManualError(w, http.StatusNotFound, opsManualErrCandidateNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"manual": manual})
	case r.Method == http.MethodGet && strings.HasSuffix(path, "/run-records"):
		id := strings.TrimSuffix(path, "/run-records")
		limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
		result, err := service.ListRunRecords(appui.OpsManualRunRecordsRequest{
			ManualID:   id,
			WorkflowID: r.URL.Query().Get("workflow_id"),
			Limit:      limit,
		})
		if err != nil {
			writeOpsManualError(w, http.StatusInternalServerError, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodPost && strings.HasSuffix(path, "/evaluate"):
		id := strings.TrimSuffix(path, "/evaluate")
		var req appui.OpsManualRetrieveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, "invalid request body")
			return
		}
		result, err := service.RetrieveManuals(req)
		if err != nil {
			writeOpsManualError(w, http.StatusBadRequest, opsManualErrInvalidOperationFrame, err.Error())
			return
		}
		filtered := result.Matches[:0]
		for _, match := range result.Matches {
			if match.Manual.ID == id {
				filtered = append(filtered, match)
			}
		}
		result.Matches = filtered
		writeJSON(w, http.StatusOK, result)
	case r.Method == http.MethodGet && path != "":
		manual, err := service.GetManual(path)
		if err != nil {
			writeOpsManualError(w, http.StatusNotFound, opsManualErrNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"manual": manual})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) opsManualService() appui.OpsManualService {
	if provider, ok := s.ui.(opsManualHTTPServices); ok {
		return provider.OpsManualService()
	}
	return nil
}

func writeOpsManualError(w http.ResponseWriter, status int, code string, message string) {
	writeJSON(w, status, map[string]string{"error_code": code, "error": message})
}
