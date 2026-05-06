package service

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"runner/state"
	"runner/workflow"
	"runner/workflow/visual"
)

type RunnerGraph = visual.Graph
type RunnerNode = visual.Node
type RunnerEdge = visual.Edge
type RunnerPosition = visual.Position

type VisualWorkflowService struct {
	workflowSvc *WorkflowService
	runSvc      *RunService
	pre         *Preprocessor
	catalog     *ActionCatalog
}

type VisualWorkflowServiceConfig struct {
	WorkflowService *WorkflowService
	RunService      *RunService
	Preprocessor    *Preprocessor
	ActionCatalog   *ActionCatalog
}

type VisualWorkflowSaveOptions struct {
	SaveNote    string `json:"save_note,omitempty"`
	SaveNoteSet bool   `json:"-"`
}

type VisualWorkflowCreateOptions struct {
	Labels      map[string]string `json:"labels,omitempty"`
	SaveNote    string            `json:"save_note,omitempty"`
	SaveNoteSet bool              `json:"-"`
}

type VisualWorkflowRunOptions struct {
	RiskAcknowledged bool   `json:"risk_acknowledged,omitempty"`
	NodeID           string `json:"node_id,omitempty"`
}

type VisualWorkflowDryRunOptions struct {
	WorkflowName string `json:"workflow_name,omitempty"`
	TriggeredBy  string `json:"triggered_by,omitempty"`
}

type CompiledVisualWorkflow struct {
	Workflow workflow.Workflow     `json:"workflow"`
	YAML     string                `json:"yaml"`
	Warnings []VisualWorkflowIssue `json:"warnings,omitempty"`
}

type CreatedVisualWorkflow struct {
	Name     string                `json:"name"`
	Status   string                `json:"status"`
	Workflow workflow.Workflow     `json:"workflow"`
	Graph    visual.Graph          `json:"graph"`
	YAML     string                `json:"yaml"`
	Warnings []VisualWorkflowIssue `json:"warnings,omitempty"`
}

type VisualWorkflowValidationResult struct {
	Valid    bool                  `json:"valid"`
	Errors   []VisualWorkflowIssue `json:"errors"`
	Warnings []VisualWorkflowIssue `json:"warnings"`
	Summary  string                `json:"summary,omitempty"`
}

type VisualWorkflowDryRunResult struct {
	Valid              bool                          `json:"valid"`
	Status             string                        `json:"status,omitempty"`
	WorkflowName       string                        `json:"workflow_name,omitempty"`
	ValidatedGraphHash string                        `json:"validated_graph_hash,omitempty"`
	DryRunGraphHash    string                        `json:"dry_run_graph_hash,omitempty"`
	DryRunAt           string                        `json:"dry_run_at,omitempty"`
	StepsCount         int                           `json:"steps_count"`
	TargetHosts        []string                      `json:"target_hosts"`
	ActionsUsed        []string                      `json:"actions_used"`
	AgentsStatus       map[string]any                `json:"agents_status"`
	PathSimulation     *VisualWorkflowPathSimulation `json:"path_simulation,omitempty"`
	Warnings           []VisualWorkflowIssue         `json:"warnings"`
	Errors             []VisualWorkflowIssue         `json:"errors"`
	Summary            string                        `json:"summary"`
	YAML               string                        `json:"yaml,omitempty"`
	RunRequest         *RunRequest                   `json:"run_request,omitempty"`
}

type WorkflowGraphHashes struct {
	SemanticHash string `json:"semantic_hash"`
	LayoutHash   string `json:"layout_hash"`
}

type VisualWorkflowPathSimulation struct {
	ReachableNodeIDs  []string                      `json:"reachable_node_ids"`
	SelectedEdgeIDs   []string                      `json:"selected_edge_ids"`
	SkippedEdgeIDs    []string                      `json:"skipped_edge_ids,omitempty"`
	UnresolvedEdgeIDs []string                      `json:"unresolved_edge_ids,omitempty"`
	Paths             []VisualWorkflowSimulatedPath `json:"paths"`
	Conditions        []VisualWorkflowPathCondition `json:"conditions,omitempty"`
	Summary           string                        `json:"summary"`
}

type VisualWorkflowSimulatedPath struct {
	NodeIDs        []string `json:"node_ids"`
	EdgeIDs        []string `json:"edge_ids"`
	TerminalNodeID string   `json:"terminal_node_id,omitempty"`
	Status         string   `json:"status"`
}

type VisualWorkflowPathCondition struct {
	EdgeID     string `json:"edge_id"`
	Expression string `json:"expression,omitempty"`
	Result     *bool  `json:"result,omitempty"`
	Error      string `json:"error,omitempty"`
}

type VisualWorkflowIssue struct {
	Severity   string `json:"severity,omitempty"`
	Type       string `json:"type"`
	Code       string `json:"code,omitempty"`
	NodeID     string `json:"node_id,omitempty"`
	EdgeID     string `json:"edge_id,omitempty"`
	Field      string `json:"field,omitempty"`
	Message    string `json:"message"`
	Suggestion string `json:"suggestion,omitempty"`
}

const graphResourceVersionKey = "resource_version"

func NewVisualWorkflowService(cfg VisualWorkflowServiceConfig) *VisualWorkflowService {
	catalog := cfg.ActionCatalog
	if catalog == nil {
		catalog = NewActionCatalog()
	}
	return &VisualWorkflowService{
		workflowSvc: cfg.WorkflowService,
		runSvc:      cfg.RunService,
		pre:         cfg.Preprocessor,
		catalog:     catalog,
	}
}

func (s *VisualWorkflowService) GetGraph(ctx context.Context, name string) (visual.Graph, error) {
	if s == nil || s.workflowSvc == nil {
		return visual.Graph{}, fmt.Errorf("%w: workflow service is not configured", ErrUnavailable)
	}
	record, err := s.workflowSvc.Get(ctx, name)
	if err != nil {
		return visual.Graph{}, err
	}
	graph, err := visual.ParseYAMLToGraph(record.RawYAML)
	if err != nil {
		return visual.Graph{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	graph = normalizeGraph(graph)
	graph.UI = withGraphResourceVersion(graph.UI, graphResourceVersion(record.RawYAML))
	return graph, nil
}

func (s *VisualWorkflowService) SaveGraph(ctx context.Context, name string, graph visual.Graph) (*CompiledVisualWorkflow, error) {
	return s.SaveGraphWithOptions(ctx, name, graph, VisualWorkflowSaveOptions{})
}

func (s *VisualWorkflowService) CreateGraph(ctx context.Context, graph visual.Graph, opts VisualWorkflowCreateOptions) (*CreatedVisualWorkflow, error) {
	if s == nil || s.workflowSvc == nil {
		return nil, fmt.Errorf("%w: workflow service is not configured", ErrUnavailable)
	}
	graph = normalizeGraph(graph)
	name := strings.TrimSpace(graph.Workflow.Name)
	if name == "" {
		return nil, fmt.Errorf("%w: workflow name is required", ErrInvalid)
	}
	graph.Workflow.Name = name
	graph.UI = withoutGraphResourceVersion(graph.UI)
	compiled, err := s.Compile(ctx, graph)
	if err != nil {
		return nil, err
	}
	record := &WorkflowRecord{
		Name:        name,
		Description: compiled.Workflow.Description,
		Version:     compiled.Workflow.Version,
		RawYAML:     []byte(compiled.YAML),
		Labels:      copyStringMap(opts.Labels),
	}
	if opts.SaveNoteSet || strings.TrimSpace(opts.SaveNote) != "" {
		record.SaveNote = opts.SaveNote
		record.SaveNoteSet = true
	}
	if err := s.workflowSvc.Create(ctx, record); err != nil {
		return nil, err
	}
	createdRecord, err := s.workflowSvc.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	createdGraph, err := s.GetGraph(ctx, name)
	if err != nil {
		return nil, err
	}
	return &CreatedVisualWorkflow{
		Name:     name,
		Status:   normalizeWorkflowStatus(createdRecord.Status),
		Workflow: cloneWorkflow(compiled.Workflow),
		Graph:    createdGraph,
		YAML:     compiled.YAML,
		Warnings: append([]VisualWorkflowIssue{}, compiled.Warnings...),
	}, nil
}

func (s *VisualWorkflowService) SaveGraphWithOptions(ctx context.Context, name string, graph visual.Graph, opts VisualWorkflowSaveOptions) (*CompiledVisualWorkflow, error) {
	if s == nil || s.workflowSvc == nil {
		return nil, fmt.Errorf("%w: workflow service is not configured", ErrUnavailable)
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, fmt.Errorf("%w: workflow name is required", ErrInvalid)
	}
	graph = normalizeGraph(graph)
	if strings.TrimSpace(graph.Workflow.Name) == "" {
		graph.Workflow.Name = name
	}
	if strings.TrimSpace(graph.Workflow.Name) != name {
		return nil, fmt.Errorf("%w: workflow name mismatch", ErrInvalid)
	}
	expectedVersion := graphExpectedResourceVersion(graph)
	if expectedVersion != "" {
		current, err := s.workflowSvc.Get(ctx, name)
		if err != nil {
			return nil, err
		}
		if currentVersion := graphResourceVersion(current.RawYAML); currentVersion != expectedVersion {
			return nil, fmt.Errorf("%w: workflow graph changed since it was loaded", ErrConflict)
		}
	}
	graph.UI = withoutGraphResourceVersion(graph.UI)
	compiled, err := s.Compile(ctx, graph)
	if err != nil {
		return nil, err
	}
	record := &WorkflowRecord{
		Name:        name,
		Description: compiled.Workflow.Description,
		RawYAML:     []byte(compiled.YAML),
	}
	if opts.SaveNoteSet || strings.TrimSpace(opts.SaveNote) != "" {
		record.SaveNote = opts.SaveNote
		record.SaveNoteSet = true
	}
	if err := s.workflowSvc.Update(ctx, name, record); err != nil {
		return nil, err
	}
	return compiled, nil
}

func graphResourceVersion(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func graphExpectedResourceVersion(graph visual.Graph) string {
	if len(graph.UI) == 0 {
		return ""
	}
	value, ok := graph.UI[graphResourceVersionKey]
	if !ok {
		return ""
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed)
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func withGraphResourceVersion(ui map[string]any, version string) map[string]any {
	out := cloneAnyMap(ui)
	if out == nil {
		out = map[string]any{}
	}
	out[graphResourceVersionKey] = strings.TrimSpace(version)
	return out
}

func withoutGraphResourceVersion(ui map[string]any) map[string]any {
	if len(ui) == 0 {
		return nil
	}
	out := cloneAnyMap(ui)
	delete(out, graphResourceVersionKey)
	if len(out) == 0 {
		return nil
	}
	return out
}

func VisualWorkflowGraphHashes(graph visual.Graph) (WorkflowGraphHashes, error) {
	graph = normalizeGraph(graph)
	semanticGraph, err := cloneVisualGraphForHash(graph)
	if err != nil {
		return WorkflowGraphHashes{}, err
	}
	scrubVisualGraphSemanticHashInput(&semanticGraph)
	semanticHash, err := canonicalWorkflowHash(semanticGraph)
	if err != nil {
		return WorkflowGraphHashes{}, err
	}

	layoutInput := visualGraphLayoutHashInput(graph)
	layoutHash, err := canonicalWorkflowHash(layoutInput)
	if err != nil {
		return WorkflowGraphHashes{}, err
	}
	return WorkflowGraphHashes{SemanticHash: semanticHash, LayoutHash: layoutHash}, nil
}

func workflowYAMLGraphHashes(raw []byte) (WorkflowGraphHashes, error) {
	graph, err := visual.ParseYAMLToGraph(raw)
	if err != nil {
		return WorkflowGraphHashes{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return VisualWorkflowGraphHashes(graph)
}

func cloneVisualGraphForHash(graph visual.Graph) (visual.Graph, error) {
	raw, err := json.Marshal(graph)
	if err != nil {
		return visual.Graph{}, err
	}
	var out visual.Graph
	if err := json.Unmarshal(raw, &out); err != nil {
		return visual.Graph{}, err
	}
	return out, nil
}

func scrubVisualGraphSemanticHashInput(graph *visual.Graph) {
	graph.Layout = visual.Layout{}
	graph.UI = nil
	graph.Workflow.Description = ""
	graph.Workflow.XRunnerUI = nil
	graph.Workflow.XRunnerGraph = nil
	for i := range graph.Nodes {
		node := &graph.Nodes[i]
		node.Position = visual.Position{}
		node.Label = ""
		node.Collapsed = false
		node.Ports = nil
		node.UI = nil
		node.State = nil
		if node.Approval != nil {
			node.Approval.UI = nil
		}
		if node.Subflow != nil {
			node.Subflow.UI = nil
		}
		if node.Join != nil {
			node.Join.UI = nil
		}
		if node.Loop != nil {
			node.Loop.UI = nil
		}
		for j := range node.Inputs {
			node.Inputs[j].UI = nil
		}
		for j := range node.Outputs {
			node.Outputs[j].UI = nil
		}
	}
	for i := range graph.Edges {
		graph.Edges[i].SourcePort = ""
		graph.Edges[i].TargetPort = ""
		graph.Edges[i].UI = nil
		graph.Edges[i].State = nil
	}
}

type visualGraphLayoutHash struct {
	Version string                      `json:"version"`
	Layout  visual.Layout               `json:"layout,omitempty"`
	Nodes   []visualGraphLayoutHashNode `json:"nodes,omitempty"`
	Edges   []visualGraphLayoutHashEdge `json:"edges,omitempty"`
	UI      map[string]any              `json:"ui,omitempty"`
}

type visualGraphLayoutHashNode struct {
	ID        string          `json:"id"`
	Position  visual.Position `json:"position"`
	Label     string          `json:"label,omitempty"`
	Collapsed bool            `json:"collapsed,omitempty"`
	Ports     []visual.Port   `json:"ports,omitempty"`
	UI        map[string]any  `json:"ui,omitempty"`
}

type visualGraphLayoutHashEdge struct {
	ID         string         `json:"id"`
	SourcePort string         `json:"source_port,omitempty"`
	TargetPort string         `json:"target_port,omitempty"`
	UI         map[string]any `json:"ui,omitempty"`
}

func visualGraphLayoutHashInput(graph visual.Graph) visualGraphLayoutHash {
	out := visualGraphLayoutHash{
		Version: strings.TrimSpace(graph.Version),
		Layout:  graph.Layout,
		UI:      withoutGraphResourceVersion(graph.UI),
		Nodes:   make([]visualGraphLayoutHashNode, 0, len(graph.Nodes)),
		Edges:   make([]visualGraphLayoutHashEdge, 0, len(graph.Edges)),
	}
	for _, node := range graph.Nodes {
		out.Nodes = append(out.Nodes, visualGraphLayoutHashNode{
			ID:        strings.TrimSpace(node.ID),
			Position:  node.Position,
			Label:     strings.TrimSpace(node.Label),
			Collapsed: node.Collapsed,
			Ports:     append([]visual.Port{}, node.Ports...),
			UI:        cloneAnyMap(node.UI),
		})
	}
	for _, edge := range graph.Edges {
		out.Edges = append(out.Edges, visualGraphLayoutHashEdge{
			ID:         strings.TrimSpace(edge.ID),
			SourcePort: strings.TrimSpace(edge.SourcePort),
			TargetPort: strings.TrimSpace(edge.TargetPort),
			UI:         cloneAnyMap(edge.UI),
		})
	}
	return out
}

func canonicalWorkflowHash(input any) (string, error) {
	raw, err := json.Marshal(input)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

func (s *VisualWorkflowService) Compile(ctx context.Context, graph visual.Graph) (*CompiledVisualWorkflow, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: visual workflow service is not configured", ErrUnavailable)
	}
	graph = normalizeGraph(graph)
	validation, err := s.Validate(ctx, graph)
	if err != nil {
		return nil, err
	}
	if !validation.Valid {
		return nil, fmt.Errorf("%w: %s", ErrInvalid, firstIssueMessage(validation.Errors))
	}
	raw, err := visual.CompileGraphToYAML(graph)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	wf, err := workflow.Load(raw)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	if err := wf.Validate(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return &CompiledVisualWorkflow{
		Workflow: cloneWorkflow(wf),
		YAML:     string(raw),
		Warnings: append([]VisualWorkflowIssue{}, validation.Warnings...),
	}, nil
}

func (s *VisualWorkflowService) ParseYAML(_ context.Context, rawYAML string) (visual.Graph, error) {
	if s == nil {
		return visual.Graph{}, fmt.Errorf("%w: visual workflow service is not configured", ErrUnavailable)
	}
	if strings.TrimSpace(rawYAML) == "" {
		return visual.Graph{}, fmt.Errorf("%w: workflow yaml is required", ErrInvalid)
	}
	graph, err := visual.ParseYAMLToGraph([]byte(rawYAML))
	if err != nil {
		return visual.Graph{}, fmt.Errorf("%w: %w", ErrInvalid, err)
	}
	return normalizeGraph(graph), nil
}

func (s *VisualWorkflowService) Validate(ctx context.Context, graph visual.Graph) (*VisualWorkflowValidationResult, error) {
	if s == nil {
		return nil, fmt.Errorf("%w: visual workflow service is not configured", ErrUnavailable)
	}
	graph = normalizeGraph(graph)
	var errorsOut []VisualWorkflowIssue
	var warningsOut []VisualWorkflowIssue
	if err := visual.ValidateGraph(graph); err != nil {
		errorsOut = append(errorsOut, issuesFromVisualError(err)...)
		if len(errorsOut) == 0 {
			return nil, fmt.Errorf("%w: %v", ErrInvalid, err)
		}
	}
	if len(errorsOut) == 0 {
		raw, err := visual.CompileGraphToYAML(graph)
		if err != nil {
			errorsOut = append(errorsOut, issue("validation", "", "", "", err.Error(), "Fix graph structure before compiling."))
		} else {
			wf, err := workflow.Load(raw)
			if err != nil {
				errorsOut = append(errorsOut, issue("parse", "", "", "", err.Error(), "Fix generated workflow YAML."))
			} else if err := wf.Validate(); err != nil {
				errorsOut = append(errorsOut, issue("validation", "", "", "", err.Error(), "Fix required workflow fields before saving."))
			} else {
				errorsOut = append(errorsOut, s.catalogIssues(graph, wf)...)
				errorsOut = append(errorsOut, s.scriptReferenceIssues(ctx, graph, wf)...)
				if s.pre != nil && len(errorsOut) == 0 {
					wfCopy := cloneWorkflow(wf)
					if err := s.pre.Process(ctx, &wfCopy); err != nil {
						errorsOut = append(errorsOut, issue("preprocess", "", "", "", err.Error(), "Fix script references, agent targets, or action permissions."))
					}
				}
			}
		}
	}
	result := &VisualWorkflowValidationResult{
		Valid:    len(errorsOut) == 0,
		Errors:   dedupeIssues(errorsOut),
		Warnings: dedupeIssues(warningsOut),
	}
	result.Summary = buildVisualValidationSummary(*result)
	return result, nil
}

func (s *VisualWorkflowService) DryRun(ctx context.Context, graph visual.Graph, vars map[string]any, triggeredBy string) (*VisualWorkflowDryRunResult, error) {
	return s.DryRunWithOptions(ctx, graph, vars, VisualWorkflowDryRunOptions{TriggeredBy: triggeredBy})
}

func (s *VisualWorkflowService) DryRunWithOptions(ctx context.Context, graph visual.Graph, vars map[string]any, opts VisualWorkflowDryRunOptions) (*VisualWorkflowDryRunResult, error) {
	validation, err := s.Validate(ctx, graph)
	if err != nil {
		return nil, err
	}
	if !validation.Valid {
		return &VisualWorkflowDryRunResult{
			Valid:        false,
			AgentsStatus: map[string]any{},
			Warnings:     validation.Warnings,
			Errors:       validation.Errors,
			Summary:      "工作流图校验未通过。",
		}, nil
	}
	compiled, err := s.Compile(ctx, graph)
	if err != nil {
		return nil, err
	}
	wf := cloneWorkflow(compiled.Workflow)
	if wf.Vars == nil {
		wf.Vars = map[string]any{}
	}
	for key, value := range vars {
		wf.Vars[key] = value
	}
	targetHosts := collectVisualDryRunTargets(wf)
	actionsUsed := collectVisualDryRunActions(wf)
	warnings := append([]VisualWorkflowIssue{}, validation.Warnings...)
	warnings = append(warnings, compiled.Warnings...)
	warnings = append(warnings, collectVisualDryRunWarnings(wf)...)
	warnings = append(warnings, s.collectVisualDryRunPrecheckWarnings(wf)...)
	pathSimulation, pathWarnings := simulateVisualWorkflowPaths(graph, wf.Vars)
	warnings = append(warnings, pathWarnings...)
	runReq := &RunRequest{
		WorkflowYAML: compiled.YAML,
		Vars:         cloneAnyMap(vars),
		TriggeredBy:  defaultTriggeredBy(opts.TriggeredBy),
	}
	hashes, hashErr := workflowYAMLGraphHashes([]byte(compiled.YAML))
	if hashErr != nil {
		return nil, hashErr
	}
	status := ""
	validatedGraphHash := ""
	dryRunGraphHash := ""
	dryRunAt := ""
	if workflowName := strings.TrimSpace(opts.WorkflowName); workflowName != "" && s.workflowSvc != nil {
		record, err := s.workflowSvc.MarkDryRunPassed(ctx, workflowName, WorkflowDryRunOptions{
			Actor:             defaultTriggeredBy(opts.TriggeredBy),
			ExpectedGraphHash: hashes.SemanticHash,
		})
		if err != nil {
			return nil, err
		}
		status = record.Status
		validatedGraphHash = record.ValidatedGraphHash
		dryRunGraphHash = record.DryRunGraphHash
		if !record.DryRunAt.IsZero() {
			dryRunAt = record.DryRunAt.Format(time.RFC3339)
		}
	} else {
		status = WorkflowStatusDryRunPassed
		validatedGraphHash = hashes.SemanticHash
		dryRunGraphHash = hashes.SemanticHash
	}
	return &VisualWorkflowDryRunResult{
		Valid:              true,
		Status:             status,
		WorkflowName:       wf.Name,
		ValidatedGraphHash: validatedGraphHash,
		DryRunGraphHash:    dryRunGraphHash,
		DryRunAt:           dryRunAt,
		StepsCount:         len(wf.Steps),
		TargetHosts:        targetHosts,
		ActionsUsed:        actionsUsed,
		AgentsStatus:       map[string]any{},
		PathSimulation:     pathSimulation,
		Warnings:           dedupeIssues(warnings),
		Errors:             []VisualWorkflowIssue{},
		Summary:            buildVisualDryRunSummary(wf.Name, len(wf.Steps), len(targetHosts)),
		YAML:               compiled.YAML,
		RunRequest:         runReq,
	}, nil
}

func (s *VisualWorkflowService) SubmitGraphRun(ctx context.Context, graph visual.Graph, vars map[string]any, triggeredBy, idempotencyKey string) (*RunResponse, error) {
	return s.SubmitGraphRunWithOptions(ctx, graph, vars, triggeredBy, idempotencyKey, VisualWorkflowRunOptions{})
}

func (s *VisualWorkflowService) SubmitGraphRunWithOptions(ctx context.Context, graph visual.Graph, vars map[string]any, triggeredBy, idempotencyKey string, opts VisualWorkflowRunOptions) (*RunResponse, error) {
	if s == nil || s.runSvc == nil {
		return nil, fmt.Errorf("%w: run service is not configured", ErrUnavailable)
	}
	if strings.TrimSpace(opts.NodeID) != "" {
		var err error
		graph, err = singleNodeRunGraph(graph, opts.NodeID)
		if err != nil {
			return nil, err
		}
	}
	compiled, err := s.Compile(ctx, graph)
	if err != nil {
		return nil, err
	}
	riskWarnings := s.collectRiskPrecheckWarnings(compiled.Workflow)
	if len(riskWarnings) > 0 && !opts.RiskAcknowledged {
		return nil, fmt.Errorf("%w: high risk actions require risk_acknowledged=true before running: %s", ErrInvalid, firstIssueMessage(riskWarnings))
	}
	capabilityWarnings := collectCapabilityPrecheckWarnings(compiled.Workflow)
	if len(capabilityWarnings) > 0 {
		return nil, fmt.Errorf("%w: agent capability constraints failed before running: %s", ErrInvalid, firstIssueMessage(capabilityWarnings))
	}
	return s.runSvc.Submit(ctx, &RunRequest{
		WorkflowYAML:   compiled.YAML,
		Vars:           cloneAnyMap(vars),
		TriggeredBy:    defaultTriggeredBy(triggeredBy),
		IdempotencyKey: strings.TrimSpace(idempotencyKey),
	})
}

func singleNodeRunGraph(graph visual.Graph, nodeID string) (visual.Graph, error) {
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return graph, nil
	}
	for _, node := range graph.Nodes {
		if strings.TrimSpace(node.ID) != nodeID {
			continue
		}
		switch node.Type {
		case visual.NodeTypeAction, visual.NodeTypeCondition, visual.NodeTypeSubflow:
			next := graph
			startID := "__single_run_start"
			endID := "__single_run_end"
			next.Nodes = []visual.Node{
				{ID: startID, Type: visual.NodeTypeStart},
				node,
				{ID: endID, Type: visual.NodeTypeEnd},
			}
			next.Edges = []visual.Edge{
				{ID: startID + "-" + node.ID, Source: startID, Target: node.ID, Kind: visual.EdgeKindNext},
				{ID: node.ID + "-" + endID, Source: node.ID, Target: endID, Kind: visual.EdgeKindSuccess},
			}
			return next, nil
		default:
			return visual.Graph{}, fmt.Errorf("%w: node %q of type %q cannot be run alone", ErrInvalid, nodeID, node.Type)
		}
	}
	return visual.Graph{}, fmt.Errorf("%w: node %q not found", ErrNotFound, nodeID)
}

func (s *VisualWorkflowService) GetRunGraph(ctx context.Context, runID string) (visual.Graph, error) {
	if s == nil || s.runSvc == nil {
		return visual.Graph{}, fmt.Errorf("%w: run service is not configured", ErrUnavailable)
	}
	detail, err := s.runSvc.Get(ctx, runID)
	if err != nil {
		return visual.Graph{}, err
	}
	raw := strings.TrimSpace(detail.WorkflowYAML)
	if raw == "" {
		return visual.Graph{}, fmt.Errorf("%w: run workflow yaml is not available", ErrNotFound)
	}
	graph, err := visual.ParseYAMLToGraph([]byte(raw))
	if err != nil {
		return visual.Graph{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	runState := state.RunState{
		RunID:        detail.RunID,
		WorkflowName: detail.WorkflowName,
		Status:       detail.Status,
		Message:      detail.Message,
		StartedAt:    detail.StartedAt,
		FinishedAt:   detail.FinishedAt,
		UpdatedAt:    detail.UpdatedAt,
		Version:      detail.Version,
		Steps:        append([]state.StepState{}, detail.Steps...),
		Graph:        detail.Graph,
	}
	return visual.OverlayRunState(normalizeGraph(graph), runState), nil
}

func (s *VisualWorkflowService) ApproveNode(ctx context.Context, runID, nodeID, actor, comment string) error {
	if s == nil || s.runSvc == nil {
		return fmt.Errorf("%w: run service is not configured", ErrUnavailable)
	}
	return s.runSvc.ApproveNode(ctx, runID, nodeID, actor, comment)
}

func (s *VisualWorkflowService) RejectNode(ctx context.Context, runID, nodeID, actor, comment string) error {
	if s == nil || s.runSvc == nil {
		return fmt.Errorf("%w: run service is not configured", ErrUnavailable)
	}
	return s.runSvc.RejectNode(ctx, runID, nodeID, actor, comment)
}

func (s *VisualWorkflowService) ListActions(ctx context.Context, filter ActionCatalogFilter) []ActionSpec {
	if s == nil || s.catalog == nil {
		return NewActionCatalog().List(ctx, filter)
	}
	return s.catalog.List(ctx, filter)
}

func (s *VisualWorkflowService) catalogIssues(graph visual.Graph, wf workflow.Workflow) []VisualWorkflowIssue {
	catalog := s.catalog
	if catalog == nil {
		catalog = NewActionCatalog()
	}
	nodeByStep := map[string]string{}
	nodeByHandler := map[string]string{}
	for _, node := range graph.Nodes {
		if name := strings.TrimSpace(stepNameForNode(node)); name != "" {
			nodeByStep[name] = node.ID
		}
		if name := strings.TrimSpace(handlerNameForNode(node)); name != "" {
			nodeByHandler[name] = node.ID
		}
	}
	var issuesOut []VisualWorkflowIssue
	for _, step := range wf.Steps {
		for _, item := range catalog.ValidateStep(step) {
			issuesOut = append(issuesOut, issue(item.Type, nodeByStep[step.Name], "", "step."+item.Field, item.Message, "Update the action configuration."))
		}
	}
	for _, handler := range wf.Handlers {
		step := workflow.Step{Name: handler.Name, Action: handler.Action, Args: handler.Args}
		for _, item := range catalog.ValidateStep(step) {
			issuesOut = append(issuesOut, issue(item.Type, nodeByHandler[handler.Name], "", "handler."+item.Field, item.Message, "Update the handler configuration."))
		}
	}
	return issuesOut
}

func (s *VisualWorkflowService) scriptReferenceIssues(ctx context.Context, graph visual.Graph, wf workflow.Workflow) []VisualWorkflowIssue {
	nodeByStep := map[string]string{}
	for _, node := range graph.Nodes {
		if name := strings.TrimSpace(stepNameForNode(node)); name != "" {
			nodeByStep[name] = node.ID
		}
	}

	var issuesOut []VisualWorkflowIssue
	for _, step := range wf.Steps {
		expectedLanguage, ok := scriptActionLanguage(step.Action)
		if !ok {
			continue
		}
		nodeID := nodeByStep[step.Name]
		if step.Args == nil {
			continue
		}
		refRaw, hasRef := step.Args["script_ref"]
		if !hasRef {
			continue
		}
		field := "step.args.script_ref"
		if step.Args["script"] != nil && refRaw != nil {
			issuesOut = append(issuesOut, issue(
				"script_ref",
				nodeID,
				"",
				field,
				fmt.Sprintf("step %q cannot use script and script_ref together", step.Name),
				"Choose either inline script content or a stored script reference.",
			))
			continue
		}
		ref := strings.TrimSpace(fmt.Sprint(refRaw))
		if ref == "" {
			issuesOut = append(issuesOut, issue(
				"script_ref",
				nodeID,
				"",
				field,
				fmt.Sprintf("step %q script_ref is empty", step.Name),
				"Choose an existing stored script or provide inline script content.",
			))
			continue
		}
		if s == nil || s.pre == nil || s.pre.scripts == nil {
			issuesOut = append(issuesOut, issue(
				"script_ref",
				nodeID,
				"",
				field,
				fmt.Sprintf("script_ref %q cannot be resolved because script service is not configured", ref),
				"Configure the script service, choose a stored script, or provide inline script content.",
			))
			continue
		}
		record, err := s.pre.scripts.Get(ctx, ref)
		if err != nil {
			message := fmt.Sprintf("script_ref %q lookup failed: %v", ref, err)
			if errors.Is(err, ErrNotFound) {
				message = fmt.Sprintf("script_ref %q not found", ref)
			}
			issuesOut = append(issuesOut, issue(
				"script_ref",
				nodeID,
				"",
				field,
				message,
				"Create the stored script, choose an existing script_ref, or provide inline script content.",
			))
			continue
		}
		actualLanguage := strings.TrimSpace(record.Language)
		if actualLanguage != expectedLanguage {
			issuesOut = append(issuesOut, issue(
				"script_ref",
				nodeID,
				"",
				field,
				fmt.Sprintf("script_ref %q language %q does not match action %q", ref, actualLanguage, strings.TrimSpace(step.Action)),
				"Use a stored script with the expected language or change the action type.",
			))
		}
	}
	return issuesOut
}

func scriptActionLanguage(action string) (string, bool) {
	switch strings.TrimSpace(action) {
	case "script.shell":
		return "shell", true
	case "script.python":
		return "python", true
	default:
		return "", false
	}
}

func normalizeGraph(graph visual.Graph) visual.Graph {
	if strings.TrimSpace(graph.Version) == "" {
		graph.Version = visual.GraphVersion
	}
	if strings.TrimSpace(graph.Workflow.Version) == "" {
		graph.Workflow.Version = "v0.1"
	}
	if strings.TrimSpace(graph.Workflow.Plan.Mode) == "" {
		graph.Workflow.Plan.Mode = "auto"
	}
	if strings.TrimSpace(graph.Workflow.Plan.Strategy) == "" {
		if graphRequiresDAGExecutor(graph) {
			graph.Workflow.Plan.Strategy = "graph"
		} else {
			graph.Workflow.Plan.Strategy = "sequential"
		}
	}
	return graph
}

func graphRequiresDAGExecutor(graph visual.Graph) bool {
	incoming := map[string]int{}
	outgoing := map[string]int{}
	for _, node := range graph.Nodes {
		switch node.Type {
		case visual.NodeTypeParallel, visual.NodeTypeJoin, visual.NodeTypeLoop, visual.NodeTypeManualApproval, visual.NodeTypeSubflow:
			return true
		}
	}
	for _, edge := range graph.Edges {
		switch edge.Kind {
		case visual.EdgeKindFailure, visual.EdgeKindAlways, visual.EdgeKindApprovalApproved, visual.EdgeKindApprovalRejected:
			return true
		}
		if isExecutableGraphNode(graph, edge.Source) && isExecutableGraphNode(graph, edge.Target) {
			outgoing[edge.Source]++
			incoming[edge.Target]++
		}
	}
	for nodeID, count := range outgoing {
		if nodeID != "" && count > 1 {
			return true
		}
	}
	for nodeID, count := range incoming {
		if nodeID != "" && count > 1 {
			return true
		}
	}
	return false
}

func isExecutableGraphNode(graph visual.Graph, nodeID string) bool {
	for _, node := range graph.Nodes {
		if node.ID != nodeID {
			continue
		}
		switch node.Type {
		case visual.NodeTypeAction, visual.NodeTypeCondition, visual.NodeTypeSubflow, visual.NodeTypeManualApproval, visual.NodeTypeParallel, visual.NodeTypeJoin, visual.NodeTypeLoop:
			return true
		default:
			return false
		}
	}
	return false
}

func issuesFromVisualError(err error) []VisualWorkflowIssue {
	var validationErr *visual.ValidationError
	if !errors.As(err, &validationErr) {
		return nil
	}
	out := make([]VisualWorkflowIssue, 0, len(validationErr.Issues))
	for _, item := range validationErr.Issues {
		out = append(out, VisualWorkflowIssue{
			Severity:   defaultIssueSeverity(item.Severity),
			Type:       item.Code,
			Code:       item.Code,
			NodeID:     item.NodeID,
			EdgeID:     item.EdgeID,
			Field:      item.Field,
			Message:    item.Message,
			Suggestion: item.Suggestion,
		})
	}
	return out
}

func stepNameForNode(node visual.Node) string {
	if node.Step != nil && strings.TrimSpace(node.Step.Name) != "" {
		return node.Step.Name
	}
	return node.StepName
}

func handlerNameForNode(node visual.Node) string {
	if node.Handler != nil && strings.TrimSpace(node.Handler.Name) != "" {
		return node.Handler.Name
	}
	return node.HandlerName
}

func collectVisualDryRunTargets(wf workflow.Workflow) []string {
	targets := map[string]struct{}{}
	for _, step := range wf.Steps {
		for _, target := range step.Targets {
			target = strings.TrimSpace(target)
			if target != "" {
				targets[target] = struct{}{}
			}
		}
	}
	if len(targets) == 0 {
		for host := range wf.Inventory.ResolveHosts() {
			targets[host] = struct{}{}
		}
	}
	return sortedKeys(targets)
}

func collectVisualDryRunActions(wf workflow.Workflow) []string {
	actions := map[string]struct{}{}
	for _, step := range wf.Steps {
		if action := strings.TrimSpace(step.Action); action != "" {
			actions[action] = struct{}{}
		}
	}
	for _, handler := range wf.Handlers {
		if action := strings.TrimSpace(handler.Action); action != "" {
			actions[action] = struct{}{}
		}
	}
	return sortedKeys(actions)
}

func collectVisualDryRunWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	hostSet := wf.Inventory.ResolveHosts()
	var warnings []VisualWorkflowIssue
	for _, step := range wf.Steps {
		if len(step.Targets) == 0 {
			warnings = append(warnings, warningIssue("dry_run", "", "", "steps.targets", "step "+step.Name+" has no explicit targets", "Confirm runtime target scope before running."))
			continue
		}
		for _, target := range step.Targets {
			target = strings.TrimSpace(target)
			if target == "" || target == "local" {
				continue
			}
			if _, ok := hostSet[target]; !ok {
				warnings = append(warnings, warningIssue("dry_run", "", "", "steps.targets", "target "+target+" is not explicitly declared in inventory", "Declare the host or group in inventory for predictable execution."))
			}
		}
	}
	return warnings
}

func simulateVisualWorkflowPaths(graph visual.Graph, vars map[string]any) (*VisualWorkflowPathSimulation, []VisualWorkflowIssue) {
	graph = normalizeGraph(graph)
	nodeByID := map[string]visual.Node{}
	outgoing := map[string][]visual.Edge{}
	startID := ""
	for _, node := range graph.Nodes {
		nodeByID[node.ID] = node
		if node.Type == visual.NodeTypeStart && startID == "" {
			startID = node.ID
		}
	}
	for _, edge := range graph.Edges {
		outgoing[edge.Source] = append(outgoing[edge.Source], edge)
	}
	if startID == "" {
		return nil, nil
	}

	selected := map[string]struct{}{}
	skipped := map[string]struct{}{}
	unresolved := map[string]struct{}{}
	reachable := map[string]struct{}{}
	conditionByEdge := map[string]VisualWorkflowPathCondition{}
	warningByEdge := map[string]VisualWorkflowIssue{}
	var paths []VisualWorkflowSimulatedPath

	var walk func(nodeID string, nodePath, edgePath []string, seen map[string]struct{})
	walk = func(nodeID string, nodePath, edgePath []string, seen map[string]struct{}) {
		if len(paths) >= 256 {
			return
		}
		if _, repeated := seen[nodeID]; repeated {
			paths = append(paths, VisualWorkflowSimulatedPath{
				NodeIDs:        append([]string{}, nodePath...),
				EdgeIDs:        append([]string{}, edgePath...),
				TerminalNodeID: nodeID,
				Status:         "cycle_guard",
			})
			return
		}
		reachable[nodeID] = struct{}{}
		nextSeen := cloneStringSet(seen)
		nextSeen[nodeID] = struct{}{}
		node := nodeByID[nodeID]
		edges := outgoing[nodeID]
		var selectedEdges []visual.Edge
		for _, edge := range edges {
			decision := evaluateDryRunEdge(edge, vars)
			if decision.condition != nil {
				conditionByEdge[edge.ID] = *decision.condition
			}
			switch {
			case decision.selected:
				selected[edge.ID] = struct{}{}
				selectedEdges = append(selectedEdges, edge)
			case decision.unresolved:
				unresolved[edge.ID] = struct{}{}
				warningByEdge[edge.ID] = warningIssue(
					"dry_run_path",
					edge.Target,
					edge.ID,
					"edges.condition",
					fmt.Sprintf("condition edge %q cannot be simulated: %s", edge.ID, decision.message),
					"Provide dry-run vars or simplify the condition expression before running.",
				)
			default:
				skipped[edge.ID] = struct{}{}
			}
		}
		if node.Type == visual.NodeTypeEnd || len(selectedEdges) == 0 {
			status := "terminal"
			if node.Type != visual.NodeTypeEnd {
				status = "stopped"
			}
			paths = append(paths, VisualWorkflowSimulatedPath{
				NodeIDs:        append([]string{}, nodePath...),
				EdgeIDs:        append([]string{}, edgePath...),
				TerminalNodeID: nodeID,
				Status:         status,
			})
			return
		}
		for _, edge := range selectedEdges {
			walk(edge.Target, append(append([]string{}, nodePath...), edge.Target), append(append([]string{}, edgePath...), edge.ID), nextSeen)
		}
	}

	walk(startID, []string{startID}, nil, nil)
	conditions := make([]VisualWorkflowPathCondition, 0, len(conditionByEdge))
	warnings := make([]VisualWorkflowIssue, 0, len(warningByEdge))
	for _, edge := range graph.Edges {
		if condition, ok := conditionByEdge[edge.ID]; ok {
			conditions = append(conditions, condition)
		}
		if warning, ok := warningByEdge[edge.ID]; ok {
			warnings = append(warnings, warning)
		}
	}
	return &VisualWorkflowPathSimulation{
		ReachableNodeIDs:  orderedNodeIDs(graph, reachable),
		SelectedEdgeIDs:   orderedEdgeIDs(graph, selected),
		SkippedEdgeIDs:    orderedEdgeIDs(graph, skipped),
		UnresolvedEdgeIDs: orderedEdgeIDs(graph, unresolved),
		Paths:             paths,
		Conditions:        conditions,
		Summary:           buildVisualPathSimulationSummary(paths, selected, unresolved),
	}, warnings
}

type dryRunEdgeDecision struct {
	selected   bool
	unresolved bool
	message    string
	condition  *VisualWorkflowPathCondition
}

func evaluateDryRunEdge(edge visual.Edge, vars map[string]any) dryRunEdgeDecision {
	kind := strings.TrimSpace(string(edge.Kind))
	if kind == "" {
		kind = string(visual.EdgeKindNext)
	}
	switch visual.EdgeKind(kind) {
	case visual.EdgeKindNext, visual.EdgeKindSuccess, visual.EdgeKindAlways, visual.EdgeKindApprovalApproved:
		return dryRunEdgeDecision{selected: true}
	case visual.EdgeKindFailure, visual.EdgeKindApprovalRejected:
		return dryRunEdgeDecision{}
	case visual.EdgeKindCondition:
		ok, err := workflow.EvalWhen(edge.Condition, vars)
		condition := VisualWorkflowPathCondition{
			EdgeID:     edge.ID,
			Expression: strings.TrimSpace(edge.Condition),
		}
		if err != nil {
			condition.Error = err.Error()
			return dryRunEdgeDecision{unresolved: true, message: err.Error(), condition: &condition}
		}
		condition.Result = boolPtr(ok)
		return dryRunEdgeDecision{selected: ok, condition: &condition}
	default:
		return dryRunEdgeDecision{
			unresolved: true,
			message:    fmt.Sprintf("edge kind %q is not supported by dry-run path simulation", kind),
		}
	}
}

func orderedNodeIDs(graph visual.Graph, set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for _, node := range graph.Nodes {
		if _, ok := set[node.ID]; ok {
			out = append(out, node.ID)
		}
	}
	return out
}

func orderedEdgeIDs(graph visual.Graph, set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for _, edge := range graph.Edges {
		if _, ok := set[edge.ID]; ok {
			out = append(out, edge.ID)
		}
	}
	return out
}

func buildVisualPathSimulationSummary(paths []VisualWorkflowSimulatedPath, selected, unresolved map[string]struct{}) string {
	if len(paths) == 0 {
		return "DAG path simulation found no reachable path."
	}
	if len(unresolved) > 0 {
		return fmt.Sprintf("DAG path simulation found %d reachable path(s), %d selected edge(s), and %d unresolved edge(s).", len(paths), len(selected), len(unresolved))
	}
	return fmt.Sprintf("DAG path simulation found %d reachable path(s) and %d selected edge(s).", len(paths), len(selected))
}

func cloneStringSet(input map[string]struct{}) map[string]struct{} {
	out := make(map[string]struct{}, len(input)+1)
	for key := range input {
		out[key] = struct{}{}
	}
	return out
}

func boolPtr(value bool) *bool {
	return &value
}

func (s *VisualWorkflowService) collectVisualDryRunPrecheckWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	var warnings []VisualWorkflowIssue
	warnings = append(warnings, collectCapabilityPrecheckWarnings(wf)...)
	warnings = append(warnings, collectUndefinedVariableWarnings(wf)...)
	warnings = append(warnings, s.collectRiskPrecheckWarnings(wf)...)
	warnings = append(warnings, collectScriptSecurityScanWarnings(wf)...)
	return warnings
}

func (s *VisualWorkflowService) collectRiskPrecheckWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	catalog := s.catalog
	if catalog == nil {
		catalog = NewActionCatalog()
	}
	var warnings []VisualWorkflowIssue
	for _, step := range wf.Steps {
		spec, ok := catalog.Get(context.Background(), step.Action)
		if !ok || strings.TrimSpace(spec.Risk) != "high" {
			continue
		}
		warnings = append(warnings, warningIssue(
			"dry_run_risk",
			"",
			"",
			"steps.action",
			fmt.Sprintf("step %q uses high risk action %q", step.Name, step.Action),
			"Require explicit review or stronger permissions before publishing or running this workflow.",
		))
	}
	return warnings
}

type scriptSecurityRule struct {
	name    string
	pattern *regexp.Regexp
}

var (
	scriptSecurityRules = []scriptSecurityRule{
		{name: "pipe_to_shell", pattern: regexp.MustCompile(`(?i)\b(curl|wget)\b[^\n|]*\|\s*(sh|bash)\b`)},
		{name: "destructive_root_delete", pattern: regexp.MustCompile(`(?i)\brm\s+-[^\n]*r[^\n]*f[^\n]*(/|\$\{?[A-Za-z_][A-Za-z0-9_]*\}?)`)},
		{name: "raw_disk_write", pattern: regexp.MustCompile(`(?i)\bdd\s+[^;\n]*(of=|if=)`)},
		{name: "world_writable", pattern: regexp.MustCompile(`(?i)\bchmod\s+777\b`)},
	}
	sensitiveEnvKeyPattern = regexp.MustCompile(`(?i)(password|passwd|secret|token|api[_-]?key|access[_-]?key|private[_-]?key|credential)`)
)

func collectScriptSecurityScanWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	var warnings []VisualWorkflowIssue
	for _, step := range wf.Steps {
		warnings = append(warnings, scanActionArgsForSecurity("step", step.Name, "steps.args", step.Args)...)
	}
	for _, handler := range wf.Handlers {
		warnings = append(warnings, scanActionArgsForSecurity("handler", handler.Name, "handlers.args", handler.Args)...)
	}
	return warnings
}

func scanActionArgsForSecurity(scope, name, fieldPrefix string, args map[string]any) []VisualWorkflowIssue {
	if len(args) == 0 {
		return nil
	}
	var warnings []VisualWorkflowIssue
	for _, key := range []string{"script", "cmd", "command"} {
		content, ok := stringArg(args, key)
		if !ok || strings.TrimSpace(content) == "" {
			continue
		}
		contentKind := "script"
		if key != "script" {
			contentKind = "command"
		}
		for _, rule := range scriptSecurityRules {
			if !rule.pattern.MatchString(content) {
				continue
			}
			warnings = append(warnings, warningIssue(
				"dry_run_security",
				"",
				"",
				fieldPrefix+"."+key,
				fmt.Sprintf("%s content in %s %q matches security rule %q", contentKind, scope, name, rule.name),
				"Review the script before publishing or running, and require an explicit approval for risky operations.",
			))
		}
	}
	for key := range anyMapArg(args, "env") {
		key = strings.TrimSpace(key)
		if key == "" || !sensitiveEnvKeyPattern.MatchString(key) {
			continue
		}
		warnings = append(warnings, warningIssue(
			"dry_run_security",
			"",
			"",
			fieldPrefix+".env",
			fmt.Sprintf("env key %q in %s %q may contain sensitive data", key, scope, name),
			"Store sensitive values in managed variables and keep dry-run or UI output redacted.",
		))
	}
	return warnings
}

func stringArg(args map[string]any, key string) (string, bool) {
	value, ok := args[key]
	if !ok || value == nil {
		return "", false
	}
	text, ok := value.(string)
	if ok {
		return text, true
	}
	return fmt.Sprint(value), true
}

func anyMapArg(args map[string]any, key string) map[string]any {
	value, ok := args[key]
	if !ok || value == nil {
		return nil
	}
	switch typed := value.(type) {
	case map[string]any:
		return typed
	case map[any]any:
		out := make(map[string]any, len(typed))
		for key, value := range typed {
			out[strings.TrimSpace(fmt.Sprint(key))] = value
		}
		return out
	default:
		return nil
	}
}

func collectCapabilityPrecheckWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	hosts := wf.Inventory.ResolveHosts()
	var warnings []VisualWorkflowIssue
	for _, step := range wf.Steps {
		action := strings.TrimSpace(step.Action)
		if action == "" {
			continue
		}
		for _, hostName := range resolveTargetHostNames(step, wf.Inventory) {
			host, ok := hosts[hostName]
			if !ok {
				continue
			}
			capabilities := stringListFromAny(firstPresent(host.Vars["capabilities"], host.Vars["runner_capabilities"]))
			if len(capabilities) == 0 || stringListContains(capabilities, action) {
				continue
			}
			warnings = append(warnings, warningIssue(
				"dry_run_capability",
				"",
				"",
				"steps.targets",
				fmt.Sprintf("target %s does not advertise capability %q", hostName, action),
				"Choose a target with matching agent capabilities, or update inventory capability metadata before running.",
			))
		}
	}
	return warnings
}

func collectUndefinedVariableWarnings(wf workflow.Workflow) []VisualWorkflowIssue {
	defined := collectInitialWorkflowVars(wf)
	var warnings []VisualWorkflowIssue
	for _, step := range wf.Steps {
		refs := collectStepVariableRefs(step)
		for _, ref := range refs {
			if _, ok := defined[ref]; ok {
				continue
			}
			warnings = append(warnings, warningIssue(
				"dry_run_variable",
				"",
				"",
				"steps.vars",
				fmt.Sprintf("variable %q is referenced before it is defined", ref),
				"Define the variable in workflow vars, inventory vars, dry-run vars, or export it from an earlier step.",
			))
		}
		for _, exported := range step.ExpectVars {
			exported = strings.TrimSpace(exported)
			if exported != "" {
				defined[exported] = struct{}{}
			}
		}
	}
	return warnings
}

func collectInitialWorkflowVars(wf workflow.Workflow) map[string]struct{} {
	out := map[string]struct{}{}
	for key := range wf.Vars {
		addVarName(out, key)
	}
	for key := range wf.Inventory.Vars {
		addVarName(out, key)
	}
	for _, group := range wf.Inventory.Groups {
		for key := range group.Vars {
			addVarName(out, key)
		}
	}
	for _, host := range wf.Inventory.Hosts {
		for key := range host.Vars {
			addVarName(out, key)
		}
	}
	return out
}

func collectStepVariableRefs(step workflow.Step) []string {
	refs := map[string]struct{}{}
	for _, must := range step.MustVars {
		addVarName(refs, must)
	}
	collectVariableRefsFromString(refs, step.When)
	collectVariableRefsFromAny(refs, step.Args)
	return sortedKeys(refs)
}

func collectVariableRefsFromAny(out map[string]struct{}, value any) {
	switch typed := value.(type) {
	case string:
		collectVariableRefsFromString(out, typed)
	case []any:
		for _, item := range typed {
			collectVariableRefsFromAny(out, item)
		}
	case []string:
		for _, item := range typed {
			collectVariableRefsFromString(out, item)
		}
	case map[string]any:
		for _, item := range typed {
			collectVariableRefsFromAny(out, item)
		}
	}
}

var (
	templateVarRefPattern = regexp.MustCompile(`\$\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}`)
	expressionVarPattern  = regexp.MustCompile(`\bvars\.([A-Za-z_][A-Za-z0-9_.-]*)`)
)

func collectVariableRefsFromString(out map[string]struct{}, value string) {
	for _, match := range templateVarRefPattern.FindAllStringSubmatch(value, -1) {
		if len(match) > 1 {
			addVarName(out, match[1])
		}
	}
	for _, match := range expressionVarPattern.FindAllStringSubmatch(value, -1) {
		if len(match) > 1 {
			addVarName(out, match[1])
		}
	}
}

func addVarName(out map[string]struct{}, value string) {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "vars.")
	if value == "" || value == "capabilities" || value == "runner_capabilities" {
		return
	}
	out[value] = struct{}{}
}

func firstPresent(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func stringListFromAny(value any) []string {
	switch typed := value.(type) {
	case []string:
		return normalizeStringList(typed)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			out = append(out, strings.TrimSpace(fmt.Sprint(item)))
		}
		return normalizeStringList(out)
	case string:
		return normalizeStringList(strings.Split(typed, ","))
	default:
		return nil
	}
}

func normalizeStringList(input []string) []string {
	out := input[:0]
	for _, item := range input {
		item = strings.TrimSpace(item)
		if item != "" {
			out = append(out, item)
		}
	}
	return out
}

func stringListContains(input []string, value string) bool {
	value = strings.TrimSpace(value)
	for _, item := range input {
		if strings.TrimSpace(item) == value {
			return true
		}
	}
	return false
}

func buildVisualDryRunSummary(name string, stepsCount, targetCount int) string {
	workflowName := strings.TrimSpace(name)
	if workflowName == "" {
		workflowName = "未命名工作流"
	}
	return workflowName + " 校验通过，包含 " + strconv.Itoa(stepsCount) + " 个步骤，覆盖 " + strconv.Itoa(targetCount) + " 个目标对象。"
}

func buildVisualValidationSummary(result VisualWorkflowValidationResult) string {
	if result.Valid {
		if len(result.Warnings) == 0 {
			return "工作流图校验通过。"
		}
		return "工作流图校验通过，包含 " + strconv.Itoa(len(result.Warnings)) + " 条警告。"
	}
	return "工作流图校验失败，包含 " + strconv.Itoa(len(result.Errors)) + " 个错误。"
}

func cloneWorkflow(wf workflow.Workflow) workflow.Workflow {
	wf.EnvPackages = append([]string{}, wf.EnvPackages...)
	wf.Inventory = cloneInventory(wf.Inventory)
	wf.Vars = cloneAnyMap(wf.Vars)
	steps := wf.Steps
	wf.Steps = make([]workflow.Step, len(steps))
	for i, step := range steps {
		wf.Steps[i] = cloneStep(step)
	}
	handlers := wf.Handlers
	wf.Handlers = make([]workflow.Handler, len(handlers))
	for i, handler := range handlers {
		wf.Handlers[i] = cloneHandler(handler)
	}
	tests := wf.Tests
	wf.Tests = make([]workflow.Test, len(tests))
	for i, test := range tests {
		wf.Tests[i] = test
		wf.Tests[i].Args = cloneAnyMap(test.Args)
	}
	return wf
}

func cloneInventory(inv workflow.Inventory) workflow.Inventory {
	out := workflow.Inventory{
		Groups: map[string]workflow.Group{},
		Hosts:  map[string]workflow.Host{},
		Vars:   cloneAnyMap(inv.Vars),
	}
	for name, group := range inv.Groups {
		out.Groups[name] = workflow.Group{
			Hosts: append([]string{}, group.Hosts...),
			Vars:  cloneAnyMap(group.Vars),
		}
	}
	for name, host := range inv.Hosts {
		out.Hosts[name] = workflow.Host{
			Address: host.Address,
			Vars:    cloneAnyMap(host.Vars),
		}
	}
	if len(out.Groups) == 0 {
		out.Groups = nil
	}
	if len(out.Hosts) == 0 {
		out.Hosts = nil
	}
	return out
}

func cloneStep(step workflow.Step) workflow.Step {
	step.Targets = append([]string{}, step.Targets...)
	step.Args = cloneAnyMap(step.Args)
	step.MustVars = append([]string{}, step.MustVars...)
	step.Loop = append([]any{}, step.Loop...)
	step.ExpectVars = append([]string{}, step.ExpectVars...)
	step.Notify = append([]string{}, step.Notify...)
	return step
}

func cloneHandler(handler workflow.Handler) workflow.Handler {
	handler.Args = cloneAnyMap(handler.Args)
	return handler
}

func sortedKeys(input map[string]struct{}) []string {
	items := make([]string, 0, len(input))
	for key := range input {
		items = append(items, key)
	}
	sort.Strings(items)
	return items
}

func firstIssueMessage(issues []VisualWorkflowIssue) string {
	if len(issues) == 0 {
		return "visual workflow validation failed"
	}
	return issues[0].Message
}

func dedupeIssues(input []VisualWorkflowIssue) []VisualWorkflowIssue {
	seen := map[string]struct{}{}
	out := make([]VisualWorkflowIssue, 0, len(input))
	for _, item := range input {
		key := item.Severity + "\x00" + item.Type + "\x00" + item.Code + "\x00" + item.NodeID + "\x00" + item.EdgeID + "\x00" + item.Field + "\x00" + item.Message
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, item)
	}
	return out
}

func issue(kind, nodeID, edgeID, field, message, suggestion string) VisualWorkflowIssue {
	kind = strings.TrimSpace(kind)
	return VisualWorkflowIssue{
		Severity:   "error",
		Type:       kind,
		Code:       kind,
		NodeID:     strings.TrimSpace(nodeID),
		EdgeID:     strings.TrimSpace(edgeID),
		Field:      strings.TrimSpace(field),
		Message:    strings.TrimSpace(message),
		Suggestion: strings.TrimSpace(suggestion),
	}
}

func warningIssue(kind, nodeID, edgeID, field, message, suggestion string) VisualWorkflowIssue {
	item := issue(kind, nodeID, edgeID, field, message, suggestion)
	item.Severity = "warning"
	return item
}

func defaultIssueSeverity(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "error"
	}
	return value
}
