package state

import (
	"testing"
	"time"
)

func TestCloneRunStateClonesGraphState(t *testing.T) {
	input := RunState{
		RunID:        "run-graph",
		WorkflowName: "wf",
		Status:       RunStatusRunning,
		Graph: &GraphRunState{
			GraphVersion: "v1",
			Nodes: map[string]NodeState{
				"node-1": {
					ID:     "node-1",
					Name:   "step-1",
					Type:   "action",
					Status: RunStatusRunning,
					Hosts: map[string]HostResult{
						"local": {
							Host:   "local",
							Status: RunStatusSuccess,
							Output: map[string]any{"stdout": "ok"},
						},
					},
					Output: map[string]any{"vars": map[string]any{"ready": "true"}},
					Iterations: []NodeIterationState{{
						Index:  0,
						Status: RunStatusSuccess,
						Nodes: map[string]NodeState{
							"child": {
								ID:     "child",
								Status: RunStatusSuccess,
								Hosts: map[string]HostResult{
									"local": {Host: "local", Status: RunStatusSuccess, Output: map[string]any{"stdout": "child"}},
								},
							},
						},
					}},
				},
			},
			Edges: map[string]EdgeState{
				"edge-1": {
					ID:     "edge-1",
					Source: "start",
					Target: "node-1",
					Kind:   "next",
					Status: "selected",
				},
			},
		},
	}

	cloned := CloneRunState(input)
	cloned.Graph.Nodes["node-1"] = NodeState{ID: "changed"}
	cloned.Graph.Edges["edge-1"] = EdgeState{ID: "changed"}

	if input.Graph.Nodes["node-1"].ID != "node-1" {
		t.Fatal("expected graph node map to be deep cloned")
	}
	if input.Graph.Edges["edge-1"].ID != "edge-1" {
		t.Fatal("expected graph edge map to be deep cloned")
	}

	cloned = CloneRunState(input)
	host := cloned.Graph.Nodes["node-1"].Hosts["local"]
	host.Output["stdout"] = "changed"
	cloned.Graph.Nodes["node-1"].Output["vars"] = "changed"
	clonedNode := cloned.Graph.Nodes["node-1"]
	clonedNode.Iterations[0].Nodes["child"].Hosts["local"].Output["stdout"] = "changed"

	if input.Graph.Nodes["node-1"].Hosts["local"].Output["stdout"] != "ok" {
		t.Fatal("expected graph host output to be deep cloned")
	}
	if input.Graph.Nodes["node-1"].Output["vars"].(map[string]any)["ready"] != "true" {
		t.Fatal("expected graph node output to be cloned")
	}
	if input.Graph.Nodes["node-1"].Iterations[0].Nodes["child"].Hosts["local"].Output["stdout"] != "child" {
		t.Fatal("expected graph iteration child node output to be cloned")
	}
}

func TestCloneRunStateDetachesEmptyContainers(t *testing.T) {
	input := RunState{
		Args:      map[string]any{},
		Resources: map[string]ResourceState{"resource": {Current: map[string]any{}, Desired: map[string]any{}, Diff: map[string]any{}}},
		Steps:     []StepState{{Name: "step", Hosts: map[string]HostResult{}}},
		Graph: &GraphRunState{
			Nodes: map[string]NodeState{
				"empty": {
					ID:         "empty",
					Hosts:      map[string]HostResult{},
					Output:     map[string]any{},
					Iterations: []NodeIterationState{{Index: 0, Nodes: map[string]NodeState{}}},
				},
				"host": {
					ID: "host",
					Hosts: map[string]HostResult{
						"local": {Host: "local", Output: map[string]any{}},
					},
				},
			},
			Edges: map[string]EdgeState{},
		},
	}

	cloned := CloneRunState(input)
	cloned.Args["arg"] = "value"
	cloned.Resources["new"] = ResourceState{ID: "new"}
	resource := cloned.Resources["resource"]
	resource.Current["current"] = true
	resource.Desired["desired"] = true
	resource.Diff["diff"] = true
	cloned.Steps[0].Hosts["local"] = HostResult{Host: "local"}
	cloned.Graph.Edges["edge"] = EdgeState{ID: "edge"}
	emptyNode := cloned.Graph.Nodes["empty"]
	emptyNode.Hosts["local"] = HostResult{Host: "local"}
	emptyNode.Output["stdout"] = "ok"
	emptyNode.Iterations[0].Nodes["child"] = NodeState{ID: "child"}
	hostNode := cloned.Graph.Nodes["host"]
	host := hostNode.Hosts["local"]
	host.Output["stdout"] = "ok"

	if len(input.Args) != 0 || len(input.Resources) != 1 || len(input.Steps[0].Hosts) != 0 || len(input.Graph.Edges) != 0 {
		t.Fatalf("top-level empty containers share clone mutations: %+v", input)
	}
	inputResource := input.Resources["resource"]
	if len(inputResource.Current) != 0 || len(inputResource.Desired) != 0 || len(inputResource.Diff) != 0 {
		t.Fatalf("resource maps share clone mutations: %+v", inputResource)
	}
	inputEmptyNode := input.Graph.Nodes["empty"]
	if len(inputEmptyNode.Hosts) != 0 || len(inputEmptyNode.Output) != 0 || len(inputEmptyNode.Iterations[0].Nodes) != 0 {
		t.Fatalf("graph node empty containers share clone mutations: %+v", inputEmptyNode)
	}
	if got := len(input.Graph.Nodes["host"].Hosts["local"].Output); got != 0 {
		t.Fatalf("host output shares clone mutations: %+v", input.Graph.Nodes["host"].Hosts["local"].Output)
	}
}

func TestUpsertGraphNodeIterationHostResultPreservesChildStatus(t *testing.T) {
	now := time.Date(2026, 5, 5, 12, 0, 0, 0, time.UTC)
	run := RunState{
		Graph: &GraphRunState{
			Nodes: map[string]NodeState{
				"loop": {ID: "loop", Type: "loop", Status: RunStatusRunning},
				"body": {ID: "body", Type: "action", ParentID: "loop", Status: RunStatusQueued},
			},
		},
	}

	run.UpsertGraphNodeIterationStartByID("loop", 0, "a", now)
	run.UpsertGraphNodeIterationNodeStartByID("loop", 0, "body", now)
	run.UpsertGraphNodeIterationHostResultByID("loop", 0, "body", HostResult{
		Host:   "local",
		Status: RunStatusSuccess,
		Output: map[string]any{"stdout": "a"},
	})

	body := run.Graph.Nodes["loop"].Iterations[0].Nodes["body"]
	if body.Status != RunStatusRunning {
		t.Fatalf("iteration body status regressed after host result: %+v", body)
	}
	if body.Hosts["local"].Output["stdout"] != "a" {
		t.Fatalf("iteration body host output missing: %+v", body.Hosts)
	}
	if topLevel := run.Graph.Nodes["body"]; topLevel.Status != RunStatusQueued || len(topLevel.Hosts) != 0 {
		t.Fatalf("top-level body should remain queued without host result, got %+v", topLevel)
	}
}

func TestSynthesizeStepStatesFromGraph(t *testing.T) {
	started := time.Date(2026, 5, 3, 12, 0, 0, 0, time.UTC)
	graph := &GraphRunState{
		Nodes: map[string]NodeState{
			"start": {
				ID:     "start",
				Name:   "Start",
				Type:   "start",
				Status: RunStatusSuccess,
			},
			"verify": {
				ID:         "verify",
				Name:       "verify-db",
				Type:       "action",
				Status:     RunStatusSuccess,
				StartedAt:  started.Add(time.Second),
				FinishedAt: started.Add(2 * time.Second),
				Hosts: map[string]HostResult{
					"local": {Host: "local", Status: RunStatusSuccess, Output: map[string]any{"stdout": "ok"}},
				},
			},
			"restore": {
				ID:        "restore",
				Name:      "restore-db",
				Type:      "action",
				Status:    RunStatusRunning,
				StartedAt: started,
			},
			"join": {
				ID:     "join",
				Name:   "join",
				Type:   "join",
				Status: RunStatusSuccess,
			},
		},
	}

	steps := SynthesizeStepStatesFromGraph(graph)
	if got, want := len(steps), 2; got != want {
		t.Fatalf("synthesized steps = %d, want %d: %+v", got, want, steps)
	}
	if steps[0].Name != "restore-db" || steps[1].Name != "verify-db" {
		t.Fatalf("steps should preserve started order and skip control nodes: %+v", steps)
	}
	steps[1].Hosts["local"] = HostResult{Host: "local", Status: RunStatusFailed}
	if graph.Nodes["verify"].Hosts["local"].Status != RunStatusSuccess {
		t.Fatal("mutating synthesized host state affected graph state")
	}
}
