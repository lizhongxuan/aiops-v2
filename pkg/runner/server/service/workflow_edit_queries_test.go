package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"runner/workflow"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditContextSummaryUsesRunnerRevisionWithoutFullGraph(t *testing.T) {
	t.Parallel()
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	result, err := app.GetContext(context.Background(), workflowEditTestKey(), WorkflowContextQuery{View: WorkflowContextSummaryView})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != backend.snapshot.Revision || result.Summary.NodeCount != 4 || result.Summary.EdgeCount != 3 {
		t.Fatalf("summary facts = %#v", result)
	}
	if len(result.Graph.Nodes) != 0 || len(result.Graph.Edges) != 0 || result.Validation == nil || !result.Validation.Validation.Valid {
		t.Fatalf("summary copied graph or lost validation: %#v", result)
	}
}

func TestWorkflowEditContextNeighborhoodFreezesSelectionAndReportsStale(t *testing.T) {
	t.Parallel()
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	result, err := app.GetContext(context.Background(), workflowEditTestKey(), WorkflowContextQuery{
		View:      WorkflowContextNeighborhoodView,
		EdgeDepth: 1,
		Selection: WorkflowContextSelection{Revision: edit.Revision{ResourceVersion: "old"}, NodeIDs: []string{"check", "deleted"}, Environment: "staging"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Selection.Stale || len(result.Selection.MissingNodeIDs) != 1 || result.Selection.Environment != "staging" {
		t.Fatalf("selection stale facts = %#v", result.Selection)
	}
	if got := workflowEditNodeIDs(result.Graph.Nodes); !equalStrings(got, []string{"start", "check", "repair"}) {
		t.Fatalf("neighborhood nodes = %v", got)
	}
	if len(result.Graph.Edges) != 2 || len(result.Environment.HostNames) != 1 || result.Environment.HostNames[0] != "local" {
		t.Fatalf("neighborhood/environment = %#v / %#v", result.Graph, result.Environment)
	}
}

func TestWorkflowEditContextFullPaginatesAndKeepsHiddenEndAndPorts(t *testing.T) {
	t.Parallel()
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	query := WorkflowContextQuery{View: WorkflowContextFullView, Limit: 2, BudgetBytes: 64 << 10}
	var nodes []visual.Node
	var edges []visual.Edge
	for {
		page, err := app.GetContext(context.Background(), workflowEditTestKey(), query)
		if err != nil {
			t.Fatal(err)
		}
		nodes = append(nodes, page.Graph.Nodes...)
		edges = append(edges, page.Graph.Edges...)
		if page.Graph.NextCursor == "" {
			break
		}
		query.Cursor = page.Graph.NextCursor
	}
	if len(nodes) != 4 || len(edges) != 3 {
		t.Fatalf("full pages nodes=%d edges=%d", len(nodes), len(edges))
	}
	end := nodes[3]
	if end.Type != visual.NodeTypeEnd || len(end.Ports) != 1 || end.Ports[0].ID != "in" {
		t.Fatalf("hidden End contract lost: %#v", end)
	}

	first, err := app.GetContext(context.Background(), workflowEditTestKey(), WorkflowContextQuery{View: WorkflowContextFullView, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	backend.snapshot.Revision.ResourceVersion = "runner-rv-2"
	if _, err := app.GetContext(context.Background(), workflowEditTestKey(), WorkflowContextQuery{View: WorkflowContextFullView, Limit: 1, Cursor: first.Graph.NextCursor}); !errors.Is(err, ErrWorkflowEditCursor) {
		t.Fatalf("stale context cursor error = %v", err)
	}
}

func TestWorkflowEditContextEnforcesResponseBudget(t *testing.T) {
	t.Parallel()
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	_, err := app.GetContext(context.Background(), workflowEditTestKey(), WorkflowContextQuery{View: WorkflowContextFullView, Limit: 10, BudgetBytes: 1})
	if !errors.Is(err, ErrWorkflowEditBudget) {
		t.Fatalf("budget error = %v", err)
	}
}

func TestWorkflowEditContextRejectsInlineSecretAndCrossTenantSnapshot(t *testing.T) {
	t.Parallel()
	graph := workflowEditTestGraph()
	secret := "context-secret-must-not-leak"
	graph.Nodes[1].Step.Args = map[string]any{"password": secret}
	backend := newWorkflowEditTestBackend(t, graph)
	app := newWorkflowEditTestApplication(t, backend, workflowEditTestAccess{}, &workflowEditTestReferences{})
	_, err := app.GetContext(context.Background(), workflowEditTestKey(), WorkflowContextQuery{
		View: WorkflowContextSelectionView, Selection: WorkflowContextSelection{NodeIDs: []string{"check"}},
	})
	if !errors.Is(err, ErrWorkflowEditSecret) || strings.Contains(err.Error(), secret) {
		t.Fatalf("inline secret error = %v", err)
	}
	crossTenant := edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: workflowEditTestKey().WorkflowID}
	if _, err := app.GetContext(context.Background(), crossTenant, WorkflowContextQuery{}); !errors.Is(err, ErrWorkflowEditSnapshot) {
		t.Fatalf("cross-tenant snapshot error = %v", err)
	}
}

func workflowEditTestKey() edit.WorkflowKey {
	return edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
}

func workflowEditTestGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Name: "workflow-storage-name", Version: "v1", ValidationEnv: "staging", EnvPackages: []string{"curl"},
			Inventory: workflow.Inventory{Hosts: map[string]workflow.Host{"local": {Address: "127.0.0.1"}}},
			Vars:      map[string]any{"region": "test", "secret_value": "must-not-be-copied"},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "check", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "check", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo check"}}, Ports: []visual.Port{{ID: "in", Type: "input"}, {ID: "next", Type: "output"}}},
			{ID: "repair", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "repair", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo repair"}}},
			{ID: "end", Type: visual.NodeTypeEnd, Ports: []visual.Port{{ID: "in", Type: "input", Required: true}}},
		},
		Edges: []visual.Edge{
			{ID: "start-check", Source: "start", Target: "check", Kind: visual.EdgeKindNext},
			{ID: "check-repair", Source: "check", Target: "repair", Kind: visual.EdgeKindSuccess},
			{ID: "repair-end", Source: "repair", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func workflowEditNodeIDs(nodes []visual.Node) []string {
	out := make([]string, len(nodes))
	for i := range nodes {
		out[i] = nodes[i].ID
	}
	return out
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
