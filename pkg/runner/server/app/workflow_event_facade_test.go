package app

import (
	"context"
	"path/filepath"
	"testing"

	"runner/server/config"
	"runner/server/service"
	"runner/workflow/edit"
)

func TestWorkflowEventFacadeExposesReplayRecoverySubscriptionAndShutdown(t *testing.T) {
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
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	facade, err := runtime.OpenWorkflowEventFacade()
	if err != nil {
		t.Fatal(err)
	}
	key := edit.WorkflowKey{TenantID: "default", WorkflowID: "missing"}
	if _, err := facade.Replay(context.Background(), key, 0, 10); err != nil {
		t.Fatal(err)
	}
	stream, cancel, err := facade.Subscribe(context.Background(), key, 0)
	if err != nil || stream == nil || cancel == nil {
		t.Fatalf("Subscribe() streamNil=%t cancelNil=%t err=%v", stream == nil, cancel == nil, err)
	}
	cancel()
	if err := facade.Shutdown(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := facade.Subscribe(context.Background(), key, 0); err == nil {
		t.Fatal("closed facade accepted a subscription")
	}
}

func TestWorkflowEventFacadeSnapshotsGraphCreatedThroughVisualAPI(t *testing.T) {
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
	runtime, err := NewRuntime(context.Background(), RuntimeOptions{Config: cfg})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })

	const workflowID = "visual-event-snapshot"
	if _, err := runtime.visualWorkflowSvc.CreateGraph(
		context.Background(),
		workflowEditFacadeTestGraph(workflowID, "echo ok"),
		service.VisualWorkflowCreateOptions{},
	); err != nil {
		t.Fatal(err)
	}
	facade, err := runtime.OpenWorkflowEventFacade()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = facade.Shutdown(context.Background()) })
	snapshot, err := facade.Snapshot(context.Background(), edit.WorkflowKey{
		TenantID:   service.VisualWorkflowDefaultTenantID,
		WorkflowID: workflowID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Key.WorkflowID != workflowID || snapshot.Graph.Workflow.Name != workflowID || snapshot.Revision.ResourceVersion == "" {
		t.Fatalf("unexpected Workflow snapshot: key=%+v name=%q revision=%+v", snapshot.Key, snapshot.Graph.Workflow.Name, snapshot.Revision)
	}
}
