package server

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/opsgraph"
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
	matches, err := service.Lookup(r.Context(), "", req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

func (s *HTTPServer) handleOpsGraphGraphs(w http.ResponseWriter, r *http.Request) {
	service, ok := s.opsGraphService()
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph service is not configured"})
		return
	}
	path := strings.Trim(strings.TrimPrefix(r.URL.Path, "/api/v1/opsgraph/graphs"), "/")
	if path == "" {
		switch r.Method {
		case http.MethodGet:
			graphs, err := service.ListGraphs(r.Context())
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"graphs": graphs})
		case http.MethodPost:
			var graph opsgraph.GraphRecord
			if err := json.NewDecoder(r.Body).Decode(&graph); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			created, err := service.CreateGraph(r.Context(), graph)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"graph": created})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}

	parts := strings.Split(path, "/")
	graphID, err := url.PathUnescape(parts[0])
	if err != nil || strings.TrimSpace(graphID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid graph id"})
		return
	}

	if len(parts) == 1 {
		s.handleOpsGraphGraph(w, r, service, graphID)
		return
	}
	switch parts[1] {
	case "duplicate":
		if r.Method != http.MethodPost || len(parts) != 2 {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		graph, found, err := service.DuplicateGraph(r.Context(), graphID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph graph not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"graph": graph})
	case "entities":
		s.handleOpsGraphGraphEntities(w, r, service, graphID, parts)
	case "relationships":
		s.handleOpsGraphGraphRelationships(w, r, service, graphID, parts)
	case "lookup":
		s.handleOpsGraphGraphLookup(w, r, service, graphID, parts)
	case "layout":
		s.handleOpsGraphGraphLayout(w, r, service, graphID, parts)
	case "validate":
		s.handleOpsGraphGraphValidate(w, r, service, graphID, parts)
	default:
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph endpoint not found"})
	}
}

func (s *HTTPServer) handleOpsGraphGraphLookup(w http.ResponseWriter, r *http.Request, service appui.OpsGraphService, graphID string, parts []string) {
	if len(parts) != 2 || r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req appui.OpsGraphLookupCommand
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	matches, err := service.Lookup(r.Context(), graphID, req)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"matches": matches})
}

func (s *HTTPServer) handleOpsGraphGraph(w http.ResponseWriter, r *http.Request, service appui.OpsGraphService, graphID string) {
	switch r.Method {
	case http.MethodGet:
		graph, found, err := service.GetGraph(r.Context(), graphID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph graph not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"graph": graph})
	case http.MethodPut, http.MethodPatch:
		var graph opsgraph.GraphRecord
		if err := json.NewDecoder(r.Body).Decode(&graph); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		updated, found, err := service.UpdateGraph(r.Context(), graphID, graph)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph graph not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"graph": updated})
	case http.MethodDelete:
		deleted, err := service.DeleteGraph(r.Context(), graphID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph graph not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleOpsGraphGraphEntities(w http.ResponseWriter, r *http.Request, service appui.OpsGraphService, graphID string, parts []string) {
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var node opsgraph.Node
		if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		created, err := service.CreateNode(r.Context(), graphID, node)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"entity": created})
		return
	}
	if len(parts) < 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph endpoint not found"})
		return
	}
	nodeID, err := url.PathUnescape(parts[2])
	if err != nil || strings.TrimSpace(nodeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid entity id"})
		return
	}
	if len(parts) == 3 {
		switch r.Method {
		case http.MethodPut, http.MethodPatch:
			var node opsgraph.Node
			if err := json.NewDecoder(r.Body).Decode(&node); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
				return
			}
			updated, found, err := service.UpdateNode(r.Context(), graphID, nodeID, node)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if !found {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph entity not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"entity": updated})
		case http.MethodDelete:
			cascade, _ := strconv.ParseBool(r.URL.Query().Get("cascade"))
			deleted, err := service.DeleteNode(r.Context(), graphID, nodeID, cascade)
			if err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
				return
			}
			if !deleted {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph entity not found"})
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
		return
	}
	if len(parts) != 4 || r.Method != http.MethodGet {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph endpoint not found"})
		return
	}
	switch parts[3] {
	case "neighborhood":
		depth, _ := strconv.Atoi(r.URL.Query().Get("depth"))
		neighborhood, found, err := service.Neighborhood(r.Context(), graphID, nodeID, depth)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph entity not found"})
			return
		}
		writeJSON(w, http.StatusOK, opsGraphNeighborhoodResponse(neighborhood))
	case "business-impact":
		impact, found, err := service.BusinessImpact(r.Context(), graphID, nodeID)
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
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph endpoint not found"})
	}
}

func (s *HTTPServer) handleOpsGraphGraphRelationships(w http.ResponseWriter, r *http.Request, service appui.OpsGraphService, graphID string, parts []string) {
	if len(parts) == 2 {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var edge opsgraph.Edge
		if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		created, err := service.CreateRelationship(r.Context(), graphID, edge)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"relationship": created})
		return
	}
	if len(parts) != 3 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph endpoint not found"})
		return
	}
	edgeID, err := url.PathUnescape(parts[2])
	if err != nil || strings.TrimSpace(edgeID) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid relationship id"})
		return
	}
	switch r.Method {
	case http.MethodPut, http.MethodPatch:
		var edge opsgraph.Edge
		if err := json.NewDecoder(r.Body).Decode(&edge); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
			return
		}
		updated, found, err := service.UpdateRelationship(r.Context(), graphID, edgeID, edge)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph relationship not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"relationship": updated})
	case http.MethodDelete:
		deleted, err := service.DeleteRelationship(r.Context(), graphID, edgeID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !deleted {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph relationship not found"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *HTTPServer) handleOpsGraphGraphLayout(w http.ResponseWriter, r *http.Request, service appui.OpsGraphService, graphID string, parts []string) {
	if len(parts) != 2 || (r.Method != http.MethodPost && r.Method != http.MethodPut) {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req struct {
		Nodes    []opsgraph.Node    `json:"nodes"`
		Viewport *opsgraph.Viewport `json:"viewport"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if err := service.SaveLayout(r.Context(), graphID, req.Nodes, req.Viewport); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"saved": true})
}

func (s *HTTPServer) handleOpsGraphGraphValidate(w http.ResponseWriter, r *http.Request, service appui.OpsGraphService, graphID string, parts []string) {
	if len(parts) != 2 || r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	issues, found, err := service.Validate(r.Context(), graphID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph graph not found"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"issues": issues})
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
		neighborhood, found, err := service.Neighborhood(r.Context(), "", entityID, depth)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}
		if !found {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "opsgraph entity not found"})
			return
		}
		writeJSON(w, http.StatusOK, opsGraphNeighborhoodResponse(neighborhood))
	case r.Method == http.MethodGet && parts[1] == "business-impact":
		impact, found, err := service.BusinessImpact(r.Context(), "", entityID)
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

func opsGraphNeighborhoodResponse(neighborhood appui.OpsGraphNeighborhoodView) map[string]any {
	return map[string]any{
		"entity":        neighborhood.Entity,
		"depth":         neighborhood.Depth,
		"neighbors":     neighborhood.Neighbors,
		"entities":      neighborhood.Entities,
		"relationships": neighborhood.Relationships,
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
