package app

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"runner/server/config"
	"runner/server/service"
	"runner/workflow"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestRuntimeInjectsVisualWorkflowCommitStore(t *testing.T) {
	base := t.TempDir()
	cfg := config.Default()
	cfg.Auth.Enabled, cfg.UI.Enabled = false, false
	cfg.Stores.WorkflowsDir = filepath.Join(base, "workflows")
	cfg.Stores.ScriptsDir = filepath.Join(base, "scripts")
	cfg.Stores.SkillsDir = filepath.Join(base, "skills")
	cfg.Stores.EnvironmentsDir = filepath.Join(base, "environments")
	cfg.Stores.MCPDir = filepath.Join(base, "mcp")
	cfg.Stores.RunStateFile = filepath.Join(base, "run-state.json")
	cfg.Stores.AgentStateFile = filepath.Join(base, "agents.json")
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{Config: cfg, WorkflowTenantID: "tenant-runtime"})
	if err != nil {
		t.Fatalf("new runtime: %v", err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	graph := visual.Graph{
		Version:  visual.GraphVersion,
		Workflow: workflow.Workflow{Name: "runtime-commit", Version: "v0.1", ValidationEnv: "staging", Plan: workflow.Plan{Mode: "auto", Strategy: "sequential"}},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "run", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "run", Action: "script.shell", Args: map[string]any{"script": "true"}}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-run", Source: "start", Target: "run", Kind: visual.EdgeKindNext},
			{ID: "run-end", Source: "run", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
	if _, err := runtime.visualWorkflowSvc.CreateGraph(context.Background(), graph, service.VisualWorkflowCreateOptions{}); err != nil {
		t.Fatalf("create visual graph: %v", err)
	}
	if _, err := runtime.visualWorkflowSvc.GetGraph(context.Background(), graph.Workflow.Name); err != nil {
		t.Fatalf("read committed visual graph: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Stores.WorkflowsDir, graph.Workflow.Name+".yaml")); !os.IsNotExist(err) {
		t.Fatalf("runtime retained legacy visual graph file: %v", err)
	}

	runtime.SetWorkflowReferenceChecker(&workflowEditNoReferences{})
	snapshot, err := runtime.WorkflowEditFacade().GetGraph(context.Background(), edit.WorkflowKey{TenantID: "tenant-runtime", WorkflowID: graph.Workflow.Name})
	if err != nil {
		t.Fatalf("read runtime workflow edit snapshot: %v", err)
	}
	position := visual.Position{X: 240, Y: 120}
	result, err := runtime.WorkflowEditFacade().ExecuteManualCommand(context.Background(), service.WorkflowManualCommandRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: snapshot.Key, ExpectedRevision: snapshot.Revision.ResourceVersion,
		IdempotencyKey: "runtime-manual-drag-stop", ActorID: "runtime-user", CanvasSessionID: "runtime-canvas",
		SessionID: "runtime-session", Reason: "move run node", Command: service.WorkflowManualCommand{
			Kind: service.WorkflowManualMove, Layout: &edit.LayoutPatch{Nodes: []edit.LayoutNodePatch{{NodeID: "run", Position: position}}},
		},
	})
	if err != nil || result.Receipt.RevisionAfter == nil {
		t.Fatalf("runtime manual command = %#v, %v", result, err)
	}
	committed, err := runtime.visualWorkflowSvc.GetGraph(context.Background(), graph.Workflow.Name)
	if err != nil || committed.Nodes[1].Position != position {
		t.Fatalf("runtime manual graph position = %#v, %v", committed.Nodes[1].Position, err)
	}
	dryRun, err := runtime.WorkflowEditApplication().DryRun(context.Background(), service.WorkflowEditDryRunRequest{
		Key: snapshot.Key, ExpectedBasis: result.Receipt.Validation.Basis, Environment: "staging",
	})
	if err != nil || dryRun.Status != service.WorkflowDryRunStatusPassed || dryRun.Mode != service.WorkflowDryRunModeDraftNonProduction || dryRun.Basis != result.Receipt.Validation.Basis {
		t.Fatalf("runtime workflow edit dry-run = %#v, %v", dryRun, err)
	}
}
