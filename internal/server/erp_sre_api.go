package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"aiops-v2/internal/appui"
)

type runbookHTTPServices interface {
	RunbookService() appui.RunbookService
}

type opsGraphHTTPServices interface {
	OpsGraphService() appui.OpsGraphService
}

type erpContextHTTPServices interface {
	ERPContextService() appui.ERPContextService
}

type changeContextHTTPServices interface {
	ChangeContextService() appui.ChangeContextService
}

func (s *HTTPServer) handleRunbooks(w http.ResponseWriter, r *http.Request) {
	service, ok := s.runbookService()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "runbook service is not configured"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/runbooks"), "/")
	switch {
	case r.Method == http.MethodGet && path == "":
		items, err := service.List(r.Context())
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runbooks": summarizeRunbooks(items)})
	case r.Method == http.MethodGet && path == "instances":
		instances, err := service.Instances(r.Context(), strings.TrimSpace(r.URL.Query().Get("status")))
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"instances": instances})
	case r.Method == http.MethodPost && path == "match":
		var req appui.RunbookMatchCommand
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		matches, err := service.Match(r.Context(), req)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"matches": summarizeRunbookCandidates(matches)})
	case r.Method == http.MethodGet && path != "":
		id, _ := url.PathUnescape(path)
		runbook, ok := service.Get(r.Context(), id)
		if !ok {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "runbook not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"runbook": detailRunbook(runbook)})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleOpsGraphLookup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.opsGraphService()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph service is not configured"})
		return
	}
	var req appui.OpsGraphLookupCommand
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	matches, err := service.Lookup(r.Context(), req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

func (s *HTTPServer) handleOpsGraphEntity(w http.ResponseWriter, r *http.Request) {
	service, ok := s.opsGraphService()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph service is not configured"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/opsgraph/entities/"), "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph endpoint not found"})
		return
	}
	entityID, _ := url.PathUnescape(parts[0])
	switch {
	case r.Method == http.MethodGet && parts[1] == "neighborhood":
		depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
		neighborhood, found, err := service.Neighborhood(r.Context(), entityID, depth)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph entity not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"neighborhood": neighborhood})
	case r.Method == http.MethodGet && parts[1] == "business-impact":
		impact, found, err := service.BusinessImpact(r.Context(), entityID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph entity not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"impact": impact})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleERPContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.erpContextService()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "erp context service is not configured"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/erp"), "/")
	query := r.URL.Query()
	switch path {
	case "health":
		health, err := service.Health(r.Context(), appui.ERPHealthCommand{Environment: query.Get("environment")})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"health": health})
	case "business-metrics":
		metrics, err := service.BusinessMetrics(r.Context(), appui.ERPMetricCommand{Capability: query.Get("capability"), Service: query.Get("service"), Environment: query.Get("environment")})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"metrics": metrics})
	case "tenant-impact":
		tenants, err := service.TenantImpact(r.Context(), appui.ERPTenantImpactCommand{Capability: query.Get("capability"), Environment: query.Get("environment")})
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"tenants": tenants})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "erp endpoint not found"})
	}
}

func (s *HTTPServer) handleChanges(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	service, ok := s.changeContextService()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "change context service is not configured"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/changes"), "/")
	query := r.URL.Query()
	cmd := appui.ChangeQueryCommand{Service: query.Get("service"), Environment: query.Get("environment"), Window: query.Get("window")}
	switch path {
	case "deployments":
		deployments, err := service.RecentDeployments(r.Context(), cmd)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deployments": deployments})
	case "config":
		changes, err := service.RecentConfigChanges(r.Context(), cmd)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"changes": changes})
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "changes endpoint not found"})
	}
}

func (s *HTTPServer) runbookService() (appui.RunbookService, bool) {
	provider, ok := s.ui.(runbookHTTPServices)
	if !ok || provider.RunbookService() == nil {
		return nil, false
	}
	return provider.RunbookService(), true
}

func (s *HTTPServer) opsGraphService() (appui.OpsGraphService, bool) {
	provider, ok := s.ui.(opsGraphHTTPServices)
	if !ok || provider.OpsGraphService() == nil {
		return nil, false
	}
	return provider.OpsGraphService(), true
}

func (s *HTTPServer) erpContextService() (appui.ERPContextService, bool) {
	provider, ok := s.ui.(erpContextHTTPServices)
	if !ok || provider.ERPContextService() == nil {
		return nil, false
	}
	return provider.ERPContextService(), true
}

func (s *HTTPServer) changeContextService() (appui.ChangeContextService, bool) {
	provider, ok := s.ui.(changeContextHTTPServices)
	if !ok || provider.ChangeContextService() == nil {
		return nil, false
	}
	return provider.ChangeContextService(), true
}

func summarizeRunbooks(items []appui.RunbookView) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, rb := range items {
		out = append(out, map[string]any{
			"id":           rb.ID,
			"name":         rb.Name,
			"title":        rb.Name,
			"description":  rb.Description,
			"risk":         rb.Risk,
			"scope":        strings.Join(nonEmptySlices(rb.Scope.Modules, rb.Scope.Services, rb.Scope.Environments), " / "),
			"modules":      rb.Scope.Modules,
			"capabilities": rb.Scope.Capabilities,
			"services":     rb.Scope.Services,
			"environment":  strings.Join(rb.Scope.Environments, ", "),
			"updatedAt":    "2026-05-04",
		})
	}
	return out
}

func detailRunbook(rb appui.RunbookView) map[string]any {
	verifications := make([]map[string]any, 0)
	for _, step := range rb.Steps {
		for _, verify := range step.Verify {
			verifications = append(verifications, map[string]any{
				"id":    step.ID + ":" + verify.Tool,
				"title": verify.Tool,
				"input": verify.Input,
			})
		}
	}
	return map[string]any{
		"id":            rb.ID,
		"name":          rb.Name,
		"title":         rb.Name,
		"description":   rb.Description,
		"scope":         rb.Scope,
		"risk":          rb.Risk,
		"steps":         rb.Steps,
		"verifications": verifications,
		"proposals":     []any{},
	}
}

func summarizeRunbookCandidates(items []appui.RunbookCandidateView) []map[string]any {
	out := make([]map[string]any, 0, len(items))
	for _, item := range items {
		out = append(out, map[string]any{
			"id":        item.Runbook.ID,
			"runbookId": item.Runbook.ID,
			"name":      item.Runbook.Name,
			"title":     item.Runbook.Name,
			"risk":      item.Runbook.Risk,
			"score":     float64(item.Score) / 100,
			"reason":    item.Reason,
			"status":    "matched",
		})
	}
	return out
}

func nonEmptySlices(groups ...[]string) []string {
	out := []string{}
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		joined := strings.Join(group, ", ")
		if strings.TrimSpace(joined) != "" {
			out = append(out, joined)
		}
	}
	return out
}
