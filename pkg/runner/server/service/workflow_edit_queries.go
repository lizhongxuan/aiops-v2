package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"strconv"
	"strings"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

type WorkflowContextView string

const (
	WorkflowContextSummaryView      WorkflowContextView = "summary"
	WorkflowContextSelectionView    WorkflowContextView = "selection"
	WorkflowContextNeighborhoodView WorkflowContextView = "neighborhood"
	WorkflowContextFullView         WorkflowContextView = "full"
)

type WorkflowContextSelection struct {
	Revision    edit.Revision `json:"revision"`
	NodeIDs     []string      `json:"nodeIds,omitempty"`
	EdgeIDs     []string      `json:"edgeIds,omitempty"`
	Environment string        `json:"environment,omitempty"`
}
type WorkflowContextQuery struct {
	View        WorkflowContextView      `json:"view"`
	Selection   WorkflowContextSelection `json:"selection,omitempty"`
	EdgeDepth   int                      `json:"edgeDepth,omitempty"`
	Cursor      string                   `json:"cursor,omitempty"`
	Limit       int                      `json:"limit,omitempty"`
	BudgetBytes int                      `json:"budgetBytes,omitempty"`
}
type WorkflowContextSummary struct {
	NodeCount int `json:"nodeCount"`
	EdgeCount int `json:"edgeCount"`
}
type WorkflowContextSelectionResult struct {
	WorkflowContextSelection
	MissingNodeIDs []string `json:"missingNodeIds,omitempty"`
	MissingEdgeIDs []string `json:"missingEdgeIds,omitempty"`
	Stale          bool     `json:"stale"`
}
type WorkflowContextEnvironment struct {
	ValidationEnvironment string   `json:"validationEnvironment,omitempty"`
	Packages              []string `json:"packages,omitempty"`
	HostNames             []string `json:"hostNames,omitempty"`
}
type WorkflowContextGraphPage struct {
	Nodes        []visual.Node `json:"nodes,omitempty"`
	Edges        []visual.Edge `json:"edges,omitempty"`
	NextCursor   string        `json:"nextCursor,omitempty"`
	ContentBytes int           `json:"contentBytes"`
}
type WorkflowContextResult struct {
	Key         edit.WorkflowKey               `json:"workflowKey"`
	Revision    edit.Revision                  `json:"revision"`
	Summary     WorkflowContextSummary         `json:"summary"`
	Selection   WorkflowContextSelectionResult `json:"selection"`
	Environment WorkflowContextEnvironment     `json:"environment"`
	Graph       WorkflowContextGraphPage       `json:"graph"`
	Validation  *WorkflowEditValidationResult  `json:"validation"`
}

func (a *WorkflowEditApplication) GetContext(ctx context.Context, key edit.WorkflowKey, query WorkflowContextQuery) (WorkflowContextResult, error) {
	if a == nil || a.backend == nil {
		return WorkflowContextResult{}, ErrWorkflowEditConfiguration
	}
	snapshot, err := a.readSnapshot(ctx, key)
	if err != nil {
		return WorkflowContextResult{}, err
	}
	view := query.View
	if view == "" {
		view = WorkflowContextSummaryView
	}
	if view != WorkflowContextSummaryView && view != WorkflowContextSelectionView && view != WorkflowContextNeighborhoodView && view != WorkflowContextFullView {
		return WorkflowContextResult{}, fmt.Errorf("%w: unsupported context view", ErrWorkflowEditSnapshot)
	}
	selection, nodes, edges := workflowSelection(snapshot, query.Selection)
	items := workflowContextItems(snapshot.Graph, view, nodes, edges, query.EdgeDepth)
	binding, err := workflowQueryDigest([]any{snapshot.Revision.ResourceVersion, view, query.Selection, query.EdgeDepth})
	if err != nil {
		return WorkflowContextResult{}, err
	}
	page, err := workflowContextPage(items, snapshot.Graph, query, binding)
	if err != nil {
		return WorkflowContextResult{}, err
	}
	validation, err := a.validateSnapshot(ctx, snapshot, snapshot.Graph, snapshot.Revision.ResourceVersion)
	if err != nil {
		return WorkflowContextResult{}, err
	}
	return WorkflowContextResult{Key: key, Revision: snapshot.Revision, Summary: workflowSummary(snapshot.Graph), Selection: selection,
		Environment: workflowEnvironment(snapshot.Graph), Graph: page, Validation: &validation}, nil
}

type workflowContextItem struct{ node, edge int }

func workflowContextItems(graph visual.Graph, view WorkflowContextView, nodes, edges map[string]bool, depth int) []workflowContextItem {
	if view == WorkflowContextSummaryView {
		return nil
	}
	if view == WorkflowContextFullView {
		for _, node := range graph.Nodes {
			nodes[node.ID] = true
		}
		for _, edge := range graph.Edges {
			edges[edge.ID] = true
		}
	} else {
		for _, edge := range graph.Edges {
			if edges[edge.ID] {
				nodes[edge.Source], nodes[edge.Target] = true, true
			}
		}
		if view == WorkflowContextNeighborhoodView {
			if depth <= 0 {
				depth = 1
			} else if depth > 5 {
				depth = 5
			}
			frontier := maps.Clone(nodes)
			for step := 0; step < depth; step++ {
				next := map[string]bool{}
				for _, edge := range graph.Edges {
					if frontier[edge.Source] || frontier[edge.Target] {
						edges[edge.ID], nodes[edge.Source], nodes[edge.Target] = true, true, true
						next[edge.Source], next[edge.Target] = true, true
					}
				}
				frontier = next
			}
		}
	}
	items := make([]workflowContextItem, 0, len(nodes)+len(edges))
	for i := range graph.Nodes {
		if nodes[graph.Nodes[i].ID] {
			items = append(items, workflowContextItem{i, -1})
		}
	}
	for i := range graph.Edges {
		if edges[graph.Edges[i].ID] {
			items = append(items, workflowContextItem{-1, i})
		}
	}
	return items
}

func workflowContextPage(items []workflowContextItem, graph visual.Graph, query WorkflowContextQuery, binding string) (WorkflowContextGraphPage, error) {
	limit, budget := workflowPageLimit(query.Limit), query.BudgetBytes
	if budget == 0 || budget > 256<<10 {
		budget = 64 << 10
	} else if budget < 0 {
		return WorkflowContextGraphPage{}, ErrWorkflowEditBudget
	}
	offset, err := workflowCursorOffset(query.Cursor, binding, len(items))
	if err != nil {
		return WorkflowContextGraphPage{}, err
	}
	page := WorkflowContextGraphPage{}
	for offset < len(items) && len(page.Nodes)+len(page.Edges) < limit {
		item := items[offset]
		var value any
		if item.node >= 0 {
			value = graph.Nodes[item.node]
		} else {
			value = graph.Edges[item.edge]
		}
		size, err := workflowSafeJSONSize(value)
		if err != nil {
			return WorkflowContextGraphPage{}, err
		}
		if page.ContentBytes+size > budget {
			if page.ContentBytes == 0 {
				return WorkflowContextGraphPage{}, ErrWorkflowEditBudget
			}
			break
		}
		if item.node >= 0 {
			page.Nodes = append(page.Nodes, graph.Nodes[item.node])
		} else {
			page.Edges = append(page.Edges, graph.Edges[item.edge])
		}
		page.ContentBytes += size
		offset++
	}
	if offset < len(items) {
		page.NextCursor = workflowEncodeCursor(offset, binding)
	}
	return page, nil
}

func workflowSelection(snapshot WorkflowEditSnapshot, selection WorkflowContextSelection) (WorkflowContextSelectionResult, map[string]bool, map[string]bool) {
	result := WorkflowContextSelectionResult{WorkflowContextSelection: selection}
	nodes, edges := map[string]bool{}, map[string]bool{}
	knownNodes, knownEdges := map[string]bool{}, map[string]bool{}
	for _, node := range snapshot.Graph.Nodes {
		knownNodes[node.ID] = true
	}
	for _, edge := range snapshot.Graph.Edges {
		knownEdges[edge.ID] = true
	}
	for _, id := range selection.NodeIDs {
		if knownNodes[id] {
			nodes[id] = true
		} else {
			result.MissingNodeIDs = append(result.MissingNodeIDs, id)
		}
	}
	for _, id := range selection.EdgeIDs {
		if knownEdges[id] {
			edges[id] = true
		} else {
			result.MissingEdgeIDs = append(result.MissingEdgeIDs, id)
		}
	}
	result.Stale = workflowRevisionStale(selection.Revision, snapshot.Revision) || len(result.MissingNodeIDs)+len(result.MissingEdgeIDs) > 0
	return result, nodes, edges
}

func workflowRevisionStale(old, current edit.Revision) bool {
	return (old.ResourceVersion != "" && old.ResourceVersion != current.ResourceVersion) || (old.SemanticHash != "" && old.SemanticHash != current.SemanticHash) || (old.LayoutHash != "" && old.LayoutHash != current.LayoutHash)
}

func workflowSummary(graph visual.Graph) WorkflowContextSummary {
	return WorkflowContextSummary{NodeCount: len(graph.Nodes), EdgeCount: len(graph.Edges)}
}

func workflowEnvironment(graph visual.Graph) WorkflowContextEnvironment {
	return WorkflowContextEnvironment{ValidationEnvironment: graph.Workflow.ValidationEnv, Packages: slices.Clone(graph.Workflow.EnvPackages),
		HostNames: slices.Sorted(maps.Keys(graph.Workflow.Inventory.Hosts))}
}

type WorkflowActionCatalogQuery struct {
	Names         []string `json:"names,omitempty"`
	Category      string   `json:"category,omitempty"`
	IncludeSchema *bool    `json:"includeSchema,omitempty"`
	Cursor        string   `json:"cursor,omitempty"`
	Limit         int      `json:"limit,omitempty"`
}
type WorkflowActionCatalogItem struct {
	Spec   ActionSpec           `json:"spec"`
	Access WorkflowActionAccess `json:"access"`
}
type WorkflowActionCatalogPage struct {
	Items         []WorkflowActionCatalogItem `json:"items"`
	Total         int                         `json:"total"`
	CatalogDigest string                      `json:"catalogDigest"`
	NextCursor    string                      `json:"nextCursor,omitempty"`
}

func (a *WorkflowEditApplication) GetActionCatalog(ctx context.Context, key edit.WorkflowKey, query WorkflowActionCatalogQuery) (WorkflowActionCatalogPage, error) {
	if a == nil || a.backend == nil || key.Validate() != nil {
		return WorkflowActionCatalogPage{}, ErrWorkflowEditConfiguration
	}
	specs, err := a.backend.ListWorkflowActions(ctx, key, ActionCatalogFilter{Category: query.Category})
	if err != nil {
		return WorkflowActionCatalogPage{}, err
	}
	names := map[string]bool{}
	for _, name := range query.Names {
		names[strings.TrimSpace(name)] = true
	}
	filtered := make([]ActionSpec, 0, len(specs))
	for _, spec := range specs {
		if len(names) == 0 || names[spec.Action] {
			filtered = append(filtered, spec)
		}
	}
	digest, err := workflowQueryDigest(filtered)
	if err != nil {
		return WorkflowActionCatalogPage{}, err
	}
	offset, err := workflowCursorOffset(query.Cursor, digest, len(filtered))
	if err != nil {
		return WorkflowActionCatalogPage{}, err
	}
	end := offset + workflowPageLimit(query.Limit)
	if end > len(filtered) {
		end = len(filtered)
	}
	page := WorkflowActionCatalogPage{Total: len(filtered), CatalogDigest: digest}
	for _, spec := range filtered[offset:end] {
		if query.IncludeSchema != nil && !*query.IncludeSchema {
			spec.ArgsSchema, spec.InputSchema, spec.OutputSchema, spec.InputsSchema, spec.OutputsSchema = nil, nil, nil, nil, nil
		}
		access, err := a.actionAccess.ResolveWorkflowActionAccess(ctx, key, spec)
		if err != nil {
			return WorkflowActionCatalogPage{}, err
		}
		page.Items = append(page.Items, WorkflowActionCatalogItem{Spec: spec, Access: access})
	}
	if end < len(filtered) {
		page.NextCursor = workflowEncodeCursor(end, digest)
	}
	return page, nil
}

func workflowCursorOffset(raw, binding string, length int) (int, error) {
	if raw == "" {
		return 0, nil
	}
	separator := strings.LastIndexByte(raw, '|')
	if separator < 0 || raw[:separator] != binding {
		return 0, ErrWorkflowEditCursor
	}
	offset, err := strconv.Atoi(raw[separator+1:])
	if err != nil || offset < 0 || offset > length {
		return 0, ErrWorkflowEditCursor
	}
	return offset, nil
}

func workflowEncodeCursor(offset int, binding string) string {
	return binding + "|" + strconv.Itoa(offset)
}

func workflowSafeJSONSize(value any) (int, error) {
	raw, err := json.Marshal(value)
	var decoded any
	if err != nil || json.Unmarshal(raw, &decoded) != nil {
		return 0, ErrWorkflowEditSnapshot
	}
	if walkWorkflowCandidateSecrets(decoded) != nil {
		return 0, ErrWorkflowEditSecret
	}
	return len(raw), nil
}

func workflowQueryDigest(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return fmt.Sprintf("sha256:%x", sum), nil
}

func workflowPageLimit(limit int) int {
	if limit <= 0 || limit > 200 {
		return 50
	}
	return limit
}
