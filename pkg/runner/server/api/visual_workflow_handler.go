package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"runner/server/service"
	"runner/workflow"
	"runner/workflow/visual"
)

type visualWorkflowHandler struct {
	svc *service.VisualWorkflowService
}

func NewVisualWorkflowHandler(svc *service.VisualWorkflowService) VisualWorkflowHandler {
	return &visualWorkflowHandler{svc: svc}
}

func (h *visualWorkflowHandler) GetGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.svc.GetGraph(r.Context(), strings.TrimSpace(r.PathValue("name")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (h *visualWorkflowHandler) CreateGraph(w http.ResponseWriter, r *http.Request) {
	req, err := decodeVisualGraphCreateRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	created, err := h.svc.CreateGraph(r.Context(), req.Graph, service.VisualWorkflowCreateOptions{
		Labels:      req.Labels,
		SaveNote:    req.SaveNote,
		SaveNoteSet: req.SaveNoteSet,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditPayload := map[string]any{
		"name":  created.Name,
		"nodes": len(req.Graph.Nodes),
		"edges": len(req.Graph.Edges),
	}
	if req.SaveNoteSet {
		auditPayload["save_note"] = req.SaveNote
	}
	if len(req.Labels) > 0 {
		auditPayload["labels"] = req.Labels
	}
	auditLog(r, "workflow.graph.create", created.Name, auditPayload)
	writeJSON(w, http.StatusCreated, created)
}

func (h *visualWorkflowHandler) UpdateGraph(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PathValue("name"))
	req, err := decodeVisualGraphSaveRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	compiled, err := h.svc.SaveGraphWithOptions(r.Context(), name, req.Graph, service.VisualWorkflowSaveOptions{
		SaveNote:    req.SaveNote,
		SaveNoteSet: req.SaveNoteSet,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditPayload := map[string]any{
		"name":  name,
		"nodes": len(req.Graph.Nodes),
		"edges": len(req.Graph.Edges),
	}
	if req.SaveNoteSet {
		auditPayload["save_note"] = req.SaveNote
	}
	auditLog(r, "workflow.graph.update", name, auditPayload)
	savedGraph, err := h.svc.GetGraph(r.Context(), name)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"name":     name,
		"status":   service.WorkflowStatusDraft,
		"workflow": compiled.Workflow,
		"graph":    savedGraph,
		"yaml":     compiled.YAML,
		"warnings": compiled.Warnings,
	})
}

func (h *visualWorkflowHandler) Compile(w http.ResponseWriter, r *http.Request) {
	graph, err := decodeVisualGraphBody(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	compiled, err := h.svc.Compile(r.Context(), graph)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, compiled)
}

func (h *visualWorkflowHandler) ParseYAML(w http.ResponseWriter, r *http.Request) {
	req, err := decodeParseYAMLRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	graph, err := h.svc.ParseYAML(r.Context(), req.YAML)
	if err != nil {
		var parseErr *visual.ParseError
		if errors.As(err, &parseErr) {
			writeJSON(w, http.StatusBadRequest, map[string]any{
				"error": err.Error(),
				"type":  parseErr.Kind,
			})
			return
		}
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (h *visualWorkflowHandler) Validate(w http.ResponseWriter, r *http.Request) {
	graph, err := decodeVisualGraphBody(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.svc.Validate(r.Context(), graph)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *visualWorkflowHandler) DryRun(w http.ResponseWriter, r *http.Request) {
	req, err := decodeGraphRunRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	result, err := h.svc.DryRunWithOptions(r.Context(), req.Graph, req.Vars, service.VisualWorkflowDryRunOptions{
		WorkflowName: req.WorkflowName,
		TriggeredBy:  req.TriggeredBy,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *visualWorkflowHandler) DebugNode(w http.ResponseWriter, r *http.Request) {
	req, err := decodeDebugNodeRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	nodeID := strings.TrimSpace(r.PathValue("node_id"))
	if nodeID == "" {
		nodeID = strings.TrimSpace(req.NodeID)
	}
	result, err := h.svc.DebugNode(r.Context(), nodeID, service.VisualWorkflowDebugRequest{
		Graph:  req.Graph,
		Vars:   req.Vars,
		Target: req.Target,
		Mode:   req.Mode,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *visualWorkflowHandler) SubmitRun(w http.ResponseWriter, r *http.Request) {
	req, err := decodeGraphRunRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	resp, err := h.svc.SubmitGraphRunWithOptions(r.Context(), req.Graph, req.Vars, req.TriggeredBy, req.IdempotencyKey, service.VisualWorkflowRunOptions{
		RiskAcknowledged: req.RiskAcknowledged,
		NodeID:           req.NodeID,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	auditLog(r, "workflow.graph.run.submit", resp.RunID, map[string]any{
		"run_id":            resp.RunID,
		"workflow_name":     resp.WorkflowName,
		"idempotency_key":   req.IdempotencyKey,
		"triggered_by":      req.TriggeredBy,
		"risk_acknowledged": req.RiskAcknowledged,
		"node_id":           req.NodeID,
	})
	writeJSON(w, http.StatusAccepted, resp)
}

func (h *visualWorkflowHandler) ResolveVariables(w http.ResponseWriter, r *http.Request) {
	req, err := decodeVariableResolveRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, visual.ResolveVariableScopes(req.Graph, req.NodeID))
}

func (h *visualWorkflowHandler) ActionCatalog(w http.ResponseWriter, r *http.Request) {
	filter := service.ActionCatalogFilter{
		Category: strings.TrimSpace(r.URL.Query().Get("category")),
	}
	if raw := strings.TrimSpace(r.URL.Query().Get("experimental")); raw != "" {
		value, err := strconv.ParseBool(raw)
		if err != nil {
			writeJSONError(w, http.StatusBadRequest, "experimental must be a boolean")
			return
		}
		filter.Experimental = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"version": "v1",
		"capabilities": map[string]any{
			"schema":               "json_schema",
			"structured_io_schema": true,
			"custom_actions":       true,
			"risk_metadata":        true,
			"examples":             true,
		},
		"items": h.svc.ListActions(r.Context(), filter),
	})
}

func (h *visualWorkflowHandler) AIDraft(w http.ResponseWriter, r *http.Request) {
	req, err := decodeAIDraftRequest(r)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	status := strings.ToLower(strings.TrimSpace(req.WorkflowStatus))
	if status != "" && status != service.WorkflowStatusDraft {
		writeJSONError(w, http.StatusConflict, "AI draft is only allowed for draft workflows")
		return
	}
	response, err := h.svc.GenerateAIDraft(r.Context(), service.VisualWorkflowAIDraftRequest{
		WorkflowName:   req.WorkflowName,
		WorkflowStatus: req.WorkflowStatus,
		Instruction:    req.Instruction,
		Graph:          req.Graph,
	})
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, response)
}

func (h *visualWorkflowHandler) RunGraph(w http.ResponseWriter, r *http.Request) {
	graph, err := h.svc.GetRunGraph(r.Context(), strings.TrimSpace(r.PathValue("id")))
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, graph)
}

func (h *visualWorkflowHandler) ApproveNode(w http.ResponseWriter, r *http.Request) {
	req := decodeApprovalRequest(r)
	runID := strings.TrimSpace(r.PathValue("id"))
	nodeID := strings.TrimSpace(r.PathValue("node_id"))
	auditLog(r, "run.node.approve", runID+"/"+nodeID, map[string]any{"run_id": runID, "node_id": nodeID, "actor": req.Actor, "comment": req.Comment})
	if err := h.svc.ApproveNode(r.Context(), runID, nodeID, req.Actor, req.Comment); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "node_id": nodeID, "status": "approved"})
}

func (h *visualWorkflowHandler) RejectNode(w http.ResponseWriter, r *http.Request) {
	req := decodeApprovalRequest(r)
	runID := strings.TrimSpace(r.PathValue("id"))
	nodeID := strings.TrimSpace(r.PathValue("node_id"))
	auditLog(r, "run.node.reject", runID+"/"+nodeID, map[string]any{"run_id": runID, "node_id": nodeID, "actor": req.Actor, "comment": req.Comment})
	if err := h.svc.RejectNode(r.Context(), runID, nodeID, req.Actor, req.Comment); err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "node_id": nodeID, "status": "rejected"})
}

type graphRunRequest struct {
	WorkflowName     string         `json:"workflow_name"`
	Graph            visual.Graph   `json:"graph"`
	Vars             map[string]any `json:"vars"`
	TriggeredBy      string         `json:"triggered_by"`
	IdempotencyKey   string         `json:"idempotency_key"`
	RiskAcknowledged bool           `json:"risk_acknowledged"`
	NodeID           string         `json:"node_id"`
	RunScope         string         `json:"run_scope"`
}

type debugNodeRequest struct {
	NodeID string         `json:"node_id,omitempty"`
	Graph  visual.Graph   `json:"graph"`
	Vars   map[string]any `json:"vars"`
	Target string         `json:"target"`
	Mode   string         `json:"mode"`
}

type variableResolveRequest struct {
	Graph  visual.Graph `json:"graph"`
	NodeID string       `json:"node_id,omitempty"`
}

type visualGraphSaveRequest struct {
	Graph       visual.Graph `json:"graph"`
	SaveNote    string       `json:"save_note,omitempty"`
	SaveNoteSet bool         `json:"-"`
}

type visualGraphCreateRequest struct {
	Graph       visual.Graph      `json:"graph"`
	Labels      map[string]string `json:"labels,omitempty"`
	SaveNote    string            `json:"save_note,omitempty"`
	SaveNoteSet bool              `json:"-"`
}

type parseYAMLRequest struct {
	YAML string `json:"yaml"`
}

type aiDraftRequest struct {
	WorkflowName   string       `json:"workflow_name"`
	WorkflowStatus string       `json:"workflow_status"`
	Instruction    string       `json:"instruction"`
	Graph          visual.Graph `json:"graph"`
}

type approvalRequest struct {
	Actor   string `json:"actor"`
	Comment string `json:"comment"`
}

func decodeAIDraftRequest(r *http.Request) (aiDraftRequest, error) {
	var req aiDraftRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	if strings.TrimSpace(req.Instruction) == "" {
		return req, fmt.Errorf("instruction is required")
	}
	if isEmptyVisualGraph(req.Graph) {
		name := strings.TrimSpace(req.WorkflowName)
		if name == "" {
			return req, fmt.Errorf("graph or workflow_name is required")
		}
		req.Graph = visual.Graph{
			Version: visual.GraphVersion,
			Workflow: workflow.Workflow{
				Version: "v0.1",
				Name:    name,
			},
		}
	}
	return req, nil
}

func decodeParseYAMLRequest(r *http.Request) (parseYAMLRequest, error) {
	var req parseYAMLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		return req, err
	}
	if strings.TrimSpace(req.YAML) == "" {
		return req, fmt.Errorf("yaml is required")
	}
	return req, nil
}

func decodeGraphRunRequest(r *http.Request) (graphRunRequest, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return graphRunRequest{}, err
	}
	var req graphRunRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return graphRunRequest{}, err
	}
	if isEmptyVisualGraph(req.Graph) {
		return graphRunRequest{}, fmt.Errorf("graph is required")
	}
	return req, nil
}

func decodeDebugNodeRequest(r *http.Request) (debugNodeRequest, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return debugNodeRequest{}, err
	}
	var req debugNodeRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		return debugNodeRequest{}, err
	}
	if isEmptyVisualGraph(req.Graph) {
		return debugNodeRequest{}, fmt.Errorf("graph is required")
	}
	return req, nil
}

func decodeVisualGraphBody(r *http.Request) (visual.Graph, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return visual.Graph{}, err
	}
	var wrapped struct {
		Graph *visual.Graph `json:"graph"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Graph != nil {
		if isEmptyVisualGraph(*wrapped.Graph) {
			return visual.Graph{}, fmt.Errorf("graph is required")
		}
		return *wrapped.Graph, nil
	}
	var graph visual.Graph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return visual.Graph{}, err
	}
	if isEmptyVisualGraph(graph) {
		return visual.Graph{}, fmt.Errorf("graph is required")
	}
	return graph, nil
}

func decodeVariableResolveRequest(r *http.Request) (variableResolveRequest, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return variableResolveRequest{}, err
	}
	var wrapped struct {
		Graph  *visual.Graph `json:"graph"`
		NodeID string        `json:"node_id"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Graph != nil {
		if isEmptyVisualGraph(*wrapped.Graph) {
			return variableResolveRequest{}, fmt.Errorf("graph is required")
		}
		return variableResolveRequest{Graph: *wrapped.Graph, NodeID: wrapped.NodeID}, nil
	}
	var graph visual.Graph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return variableResolveRequest{}, err
	}
	if isEmptyVisualGraph(graph) {
		return variableResolveRequest{}, fmt.Errorf("graph is required")
	}
	return variableResolveRequest{Graph: graph}, nil
}

func decodeVisualGraphSaveRequest(r *http.Request) (visualGraphSaveRequest, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return visualGraphSaveRequest{}, err
	}
	var wrapped struct {
		Graph    *visual.Graph `json:"graph"`
		SaveNote *string       `json:"save_note"`
	}
	if err := json.Unmarshal(raw, &wrapped); err == nil && wrapped.Graph != nil {
		if isEmptyVisualGraph(*wrapped.Graph) {
			return visualGraphSaveRequest{}, fmt.Errorf("graph is required")
		}
		req := visualGraphSaveRequest{Graph: *wrapped.Graph}
		if wrapped.SaveNote != nil {
			req.SaveNote = *wrapped.SaveNote
			req.SaveNoteSet = true
		}
		return req, nil
	}
	var graph visual.Graph
	if err := json.Unmarshal(raw, &graph); err != nil {
		return visualGraphSaveRequest{}, err
	}
	if isEmptyVisualGraph(graph) {
		return visualGraphSaveRequest{}, fmt.Errorf("graph is required")
	}
	return visualGraphSaveRequest{Graph: graph}, nil
}

func decodeVisualGraphCreateRequest(r *http.Request) (visualGraphCreateRequest, error) {
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return visualGraphCreateRequest{}, err
	}
	var wrapped struct {
		Graph    *visual.Graph     `json:"graph"`
		Labels   map[string]string `json:"labels"`
		SaveNote *string           `json:"save_note"`
	}
	if err := json.Unmarshal(raw, &wrapped); err != nil {
		return visualGraphCreateRequest{}, err
	}
	if wrapped.Graph == nil || isEmptyVisualGraph(*wrapped.Graph) {
		return visualGraphCreateRequest{}, fmt.Errorf("graph is required")
	}
	req := visualGraphCreateRequest{
		Graph:  *wrapped.Graph,
		Labels: wrapped.Labels,
	}
	if wrapped.SaveNote != nil {
		req.SaveNote = *wrapped.SaveNote
		req.SaveNoteSet = true
	}
	return req, nil
}

func decodeApprovalRequest(r *http.Request) approvalRequest {
	req := approvalRequest{Actor: actorFromRequest(r)}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if strings.TrimSpace(req.Actor) == "" {
		req.Actor = actorFromRequest(r)
	}
	return req
}

func isEmptyVisualGraph(graph visual.Graph) bool {
	return strings.TrimSpace(graph.Version) == "" &&
		strings.TrimSpace(graph.Workflow.Name) == "" &&
		len(graph.Nodes) == 0 &&
		len(graph.Edges) == 0
}
