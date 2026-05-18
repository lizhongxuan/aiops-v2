package runnerembed

import (
	"context"
	"testing"
	"time"

	"runner/workflow"
	"runner/workflow/visual"
)

func TestBootstrapClientSubmitGraphRunReturnsRunID(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer runtime.Close(context.Background())

	client := NewBootstrapClient(runtime)
	run, err := client.SubmitHostInstallGraph(context.Background(), bootstrapClientSmokeGraph(), map[string]any{"host_id": "host-a"}, "install:host-a")
	if err != nil {
		t.Fatalf("SubmitHostInstallGraph() error = %v", err)
	}
	if run.RunID == "" {
		t.Fatal("RunID is empty")
	}
	if run.WorkflowID != "bootstrap-client-smoke" {
		t.Fatalf("WorkflowID = %q, want bootstrap-client-smoke", run.WorkflowID)
	}
}

func TestBootstrapClientGetRunReturnsCurrentStep(t *testing.T) {
	runtime, err := NewRuntime(context.Background(), Options{DataDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewRuntime() error = %v", err)
	}
	defer runtime.Close(context.Background())

	client := NewBootstrapClient(runtime)
	submitted, err := client.SubmitHostInstallGraph(context.Background(), bootstrapClientSmokeGraph(), map[string]any{"host_id": "host-a"}, "install:host-a")
	if err != nil {
		t.Fatalf("SubmitHostInstallGraph() error = %v", err)
	}

	var detail HostInstallRunForTest
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		run, err := client.GetHostInstallRun(context.Background(), submitted.RunID)
		if err != nil {
			t.Fatalf("GetHostInstallRun() error = %v", err)
		}
		detail = HostInstallRunForTest{
			RunID:       run.RunID,
			Status:      run.Status,
			CurrentStep: run.CurrentStep,
		}
		if run.Status == "success" {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if detail.RunID != submitted.RunID {
		t.Fatalf("RunID = %q, want %q", detail.RunID, submitted.RunID)
	}
	if detail.CurrentStep != "echo" {
		t.Fatalf("CurrentStep = %q, want echo", detail.CurrentStep)
	}
	if detail.Status != "success" {
		t.Fatalf("Status = %q, want success", detail.Status)
	}
}

type HostInstallRunForTest struct {
	RunID       string
	Status      string
	CurrentStep string
}

func bootstrapClientSmokeGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Name:    "bootstrap-client-smoke",
			Version: "v0.1",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{
				ID:   "echo",
				Type: visual.NodeTypeAction,
				Step: &workflow.Step{
					Name:    "echo",
					Targets: []string{"local"},
					Action:  "script.shell",
					Args:    map[string]any{"script": "set -euo pipefail\necho ok"},
				},
			},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-echo", Source: "start", Target: "echo", Kind: visual.EdgeKindNext},
			{ID: "echo-end", Source: "echo", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}
