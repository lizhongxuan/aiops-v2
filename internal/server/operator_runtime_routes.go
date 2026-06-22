package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/operatorruntime"
)

type operatorRuntimeHTTPServices interface {
	OperatorRuntimeService() *appui.OperatorRuntimeService
}

type operatorRuntimeRunEventRequest struct {
	Message string `json:"message,omitempty"`
}

func (s *HTTPServer) handleOperatorRuntime(w http.ResponseWriter, r *http.Request) {
	provider, ok := s.ui.(operatorRuntimeHTTPServices)
	if !ok || provider.OperatorRuntimeService() == nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime service is not configured"})
		return
	}
	service := provider.OperatorRuntimeService()
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/guards"), "/")

	switch {
	case path == "resources":
		s.handleOperatorRuntimeResources(w, r, service)
	case path == "pg/clusters":
		s.handleOperatorRuntimePGClusters(w, r, service)
	case path == "inspection-templates":
		s.handleOperatorRuntimeInspectionTemplates(w, r, service)
	case path == "problem-types":
		s.handleOperatorRuntimeProblemTypes(w, r, service)
	case path == "actions":
		s.handleOperatorRuntimeActions(w, r, service)
	case path == "workflow-bindings":
		s.handleOperatorRuntimeWorkflowBindings(w, r, service)
	case path == "rules":
		s.handleOperatorRuntimeRules(w, r, service)
	case strings.HasPrefix(path, "rules/"):
		s.handleOperatorRuntimeRuleAction(w, r, service, strings.TrimPrefix(path, "rules/"))
	case path == "runs":
		s.handleOperatorRuntimeRuns(w, r, service)
	case strings.HasPrefix(path, "runs/"):
		s.handleOperatorRuntimeRun(w, r, service, strings.TrimPrefix(path, "runs/"))
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime endpoint not found"})
	}
}

func (s *HTTPServer) handleOperatorRuntimeResources(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListResources(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.ManagedResource
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreateResource(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimePGClusters(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListPGClusters(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.PGCluster
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreatePGCluster(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimeInspectionTemplates(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListInspectionTemplates(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.InspectionTemplate
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreateInspectionTemplate(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimeProblemTypes(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListProblemTypes(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.ProblemType
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreateProblemType(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimeActions(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListActions(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.ActionCatalogItem
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreateAction(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimeWorkflowBindings(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListWorkflowBindings(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.WorkflowBinding
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreateWorkflowBinding(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimeRules(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	switch r.Method {
	case http.MethodGet:
		items, err := service.ListRules(r.Context())
		writeOperatorRuntimeList(w, items, err)
	case http.MethodPost:
		var item operatorruntime.GuardRule
		if !decodeOperatorRuntimeCreateBody(w, r, &item) {
			return
		}
		created, err := service.CreateRule(r.Context(), item)
		writeOperatorRuntimeItem(w, created, err)
	default:
		writeOperatorRuntimeMethodNotAllowed(w)
	}
}

func (s *HTTPServer) handleOperatorRuntimeRuleAction(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService, path string) {
	if r.Method != http.MethodPost {
		writeOperatorRuntimeMethodNotAllowed(w)
		return
	}
	id, action, ok := splitOperatorRuntimeActionPath(path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime rule action not found"})
		return
	}
	switch action {
	case "enable":
		item, err := service.EnableRule(r.Context(), id)
		writeOperatorRuntimeItem(w, item, err)
	case "disable":
		item, err := service.DisableRule(r.Context(), id)
		writeOperatorRuntimeItem(w, item, err)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime rule action not found"})
	}
}

func (s *HTTPServer) handleOperatorRuntimeRuns(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService) {
	if r.Method != http.MethodGet {
		writeOperatorRuntimeMethodNotAllowed(w)
		return
	}
	items, err := service.ListRuns(r.Context())
	writeOperatorRuntimeList(w, items, err)
}

func (s *HTTPServer) handleOperatorRuntimeRun(w http.ResponseWriter, r *http.Request, service *appui.OperatorRuntimeService, path string) {
	if r.Method == http.MethodGet {
		if strings.Contains(strings.Trim(path, "/"), "/") {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime run endpoint not found"})
			return
		}
		item, ok, err := service.GetRun(r.Context(), path)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "guard run not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"item": item})
		return
	}
	if r.Method != http.MethodPost {
		writeOperatorRuntimeMethodNotAllowed(w)
		return
	}
	id, action, ok := splitOperatorRuntimeActionPath(path)
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime run action not found"})
		return
	}
	var eventType string
	switch action {
	case "approve":
		eventType = "approval.approved"
	case "reject":
		eventType = "approval.rejected"
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "operator runtime run action not found"})
		return
	}
	req, ok := decodeOperatorRuntimeRunEventRequest(w, r)
	if !ok {
		return
	}
	item, err := service.AppendRunEvent(r.Context(), id, operatorruntime.GuardRunEvent{
		Type:    eventType,
		Message: req.Message,
	})
	writeOperatorRuntimeItem(w, item, err)
}

func splitOperatorRuntimeActionPath(path string) (string, string, bool) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) != 2 {
		return "", "", false
	}
	id := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	return id, action, id != "" && action != ""
}

func decodeOperatorRuntimeCreateBody(w http.ResponseWriter, r *http.Request, target any) bool {
	if err := decodeJSONBody(r, target); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return false
	}
	return true
}

func decodeOperatorRuntimeRunEventRequest(w http.ResponseWriter, r *http.Request) (operatorRuntimeRunEventRequest, bool) {
	if r.Body == nil {
		return operatorRuntimeRunEventRequest{}, true
	}
	defer r.Body.Close()
	var req operatorRuntimeRunEventRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil && !errors.Is(err, io.EOF) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return operatorRuntimeRunEventRequest{}, false
	}
	return req, true
}

func writeOperatorRuntimeList[T any](w http.ResponseWriter, items []T, err error) {
	if err != nil {
		writeOperatorRuntimeError(w, http.StatusInternalServerError, err)
		return
	}
	if items == nil {
		items = []T{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func writeOperatorRuntimeItem[T any](w http.ResponseWriter, item T, err error) {
	if err != nil {
		writeOperatorRuntimeError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"item": item})
}

func writeOperatorRuntimeError(w http.ResponseWriter, status int, err error) {
	payload := map[string]any{"error": err.Error()}
	if validationErr, ok := operatorruntime.ValidationErrorDetails(err); ok {
		payload["fieldErrors"] = validationErr.FieldErrors
	}
	writeJSON(w, status, payload)
}

func writeOperatorRuntimeMethodNotAllowed(w http.ResponseWriter) {
	writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
}
