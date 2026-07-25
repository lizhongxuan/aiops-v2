package service

import (
	"context"
	"testing"

	"runner/workflow"
	"runner/workflow/visual"
)

func TestVisualWorkflowSemanticHashIncludesContractFields(t *testing.T) {
	baseGraph := visualWorkflowHashContractGraph()
	base := mustVisualWorkflowHashes(t, baseGraph)

	tests := []struct {
		name   string
		mutate func(*visual.Graph)
	}{
		{
			name: "node port contract",
			mutate: func(graph *visual.Graph) {
				graph.Nodes[0].Ports[0].Required = true
			},
		},
		{
			name: "edge source handle",
			mutate: func(graph *visual.Graph) {
				graph.Edges[0].SourcePort = "other-output"
			},
		},
		{
			name: "edge target handle",
			mutate: func(graph *visual.Graph) {
				graph.Edges[0].TargetPort = "other-input"
			},
		},
		{
			name: "edge condition",
			mutate: func(graph *visual.Graph) {
				graph.Edges[1].Condition = "vars.disk_full == false"
			},
		},
		{
			name: "input binding",
			mutate: func(graph *visual.Graph) {
				graph.Nodes[1].Inputs[0].ValueSource.Value = 95
			},
		},
		{
			name: "action arguments",
			mutate: func(graph *visual.Graph) {
				graph.Nodes[1].Step.Args["script"] = "df -h /var/lib"
			},
		},
		{
			name: "workflow extension",
			mutate: func(graph *visual.Graph) {
				graph.Workflow.Extensions = map[string]any{"plugin_contract": map[string]any{"mode": "strict"}}
			},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := visualWorkflowHashContractGraph()
			testCase.mutate(&changed)
			hashes := mustVisualWorkflowHashes(t, changed)
			if hashes.SemanticHash == base.SemanticHash {
				t.Fatalf("semantic hash ignored %s", testCase.name)
			}
			if hashes.LayoutHash != base.LayoutHash {
				t.Fatalf("layout hash changed for semantic-only %s: base=%s changed=%s", testCase.name, base.LayoutHash, hashes.LayoutHash)
			}
		})
	}
}

func TestVisualWorkflowHashesExcludeVolatileState(t *testing.T) {
	baseGraph := visualWorkflowHashContractGraph()
	base := mustVisualWorkflowHashes(t, baseGraph)
	changed := visualWorkflowHashContractGraph()
	changed.UI = map[string]any{graphResourceVersionKey: "new-resource-version"}
	changed.Nodes[0].Ports[0].UI = map[string]any{"hovered": true}
	changed.Nodes[1].State = &visual.NodeRunState{RunID: "run-1", Status: "running"}
	changed.Edges[0].State = &visual.EdgeRunState{RunID: "run-1", Status: "selected"}
	hashes := mustVisualWorkflowHashes(t, changed)
	if hashes != base {
		t.Fatalf("volatile state changed persisted hashes: base=%+v changed=%+v", base, hashes)
	}
}

func TestVisualWorkflowSemanticHashIgnoresStaleCompiledStepProjection(t *testing.T) {
	t.Parallel()
	service := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	compiled, err := service.Compile(context.Background(), sampleVisualGraph())
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	loaded, err := visual.ParseYAMLToGraph([]byte(compiled.YAML))
	if err != nil {
		t.Fatalf("parse committed graph: %v", err)
	}
	if len(loaded.Workflow.Steps) == 0 || loaded.Nodes[1].Step == nil {
		t.Fatalf("fixture must contain both compiled and visual step projections: %+v", loaded)
	}

	loaded.Nodes[1].Step.Args["script"] = "echo edited-from-canvas"
	direct := mustVisualWorkflowHashes(t, loaded)
	recompiled, err := service.Compile(context.Background(), loaded)
	if err != nil {
		t.Fatalf("compile edited graph: %v", err)
	}
	roundTrip, err := workflowYAMLGraphHashes([]byte(recompiled.YAML))
	if err != nil {
		t.Fatalf("hash compiled graph: %v", err)
	}
	if direct != roundTrip {
		t.Fatalf("compiled projection changed graph identity: direct=%+v roundTrip=%+v", direct, roundTrip)
	}

	staleProjection := loaded
	staleProjection.Workflow.Steps = append([]workflow.Step(nil), loaded.Workflow.Steps...)
	staleProjection.Workflow.Steps[0].Args = map[string]any{"script": "stale compiled copy"}
	if got := mustVisualWorkflowHashes(t, staleProjection); got != direct {
		t.Fatalf("stale compiled step copy changed semantic identity: direct=%+v stale=%+v", direct, got)
	}
}

func TestVisualWorkflowLayoutHashExcludesSemanticState(t *testing.T) {
	baseGraph := visualWorkflowHashContractGraph()
	base := mustVisualWorkflowHashes(t, baseGraph)

	tests := []struct {
		name   string
		mutate func(*visual.Graph)
	}{
		{"position", func(graph *visual.Graph) { graph.Nodes[1].Position.X += 120 }},
		{"collapsed", func(graph *visual.Graph) { graph.Nodes[1].Collapsed = !graph.Nodes[1].Collapsed }},
		{"viewport", func(graph *visual.Graph) { graph.Layout.Viewport.Zoom = 0.75 }},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			changed := visualWorkflowHashContractGraph()
			testCase.mutate(&changed)
			hashes := mustVisualWorkflowHashes(t, changed)
			if hashes.SemanticHash != base.SemanticHash {
				t.Fatalf("semantic hash changed for layout-only %s", testCase.name)
			}
			if hashes.LayoutHash == base.LayoutHash {
				t.Fatalf("layout hash ignored %s", testCase.name)
			}
		})
	}
}

func TestVisualWorkflowPortHashInvalidatesValidatedArtifact(t *testing.T) {
	workflowService := NewWorkflowService(t.TempDir())
	visualService := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowService})
	graph := visualWorkflowHashContractGraph()
	graph.Workflow.Name = "port-contract-validation"
	created, err := visualService.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{})
	if err != nil {
		t.Fatalf("create graph: %v", err)
	}
	validated, err := workflowService.ValidateWorkflow(context.Background(), created.Name, WorkflowValidateOptions{Actor: "fixture-user"})
	if err != nil {
		t.Fatalf("validate workflow: %v", err)
	}
	if validated.ValidatedGraphHash == "" {
		t.Fatal("validation artifact did not bind a semantic hash")
	}

	loaded, err := visualService.GetGraph(context.Background(), created.Name)
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	loaded.Nodes[0].Ports[0].ID = "next-v2"
	loaded.Edges[0].SourcePort = "next-v2"
	if _, err := visualService.SaveGraph(context.Background(), created.Name, loaded); err != nil {
		t.Fatalf("save changed port contract: %v", err)
	}
	record, err := workflowService.Get(context.Background(), created.Name)
	if err != nil {
		t.Fatalf("get saved record: %v", err)
	}
	if record.Status != WorkflowStatusDraft || record.ValidatedGraphHash != "" || !record.ValidatedAt.IsZero() {
		t.Fatalf("port contract change reused stale validation artifact: status=%s validated_hash=%s validated_at=%s", record.Status, record.ValidatedGraphHash, record.ValidatedAt)
	}
}

func visualWorkflowHashContractGraph() visual.Graph {
	graph := sampleVisualGraph()
	graph.Nodes[0].Ports = []visual.Port{{ID: "next", Type: "output", Label: "Next"}}
	graph.Nodes[1].Ports = []visual.Port{{ID: "input", Type: "input", Label: "Input", Required: true}}
	graph.Nodes[1].Inputs = []visual.InputParamSpec{{
		Key: "threshold", Type: "integer", Required: true,
		ValueSource: visual.ValueSource{Type: "literal", Value: 90},
	}}
	graph.Nodes[1].Outputs = []visual.OutputParamSpec{{Key: "usage", Type: "integer"}}
	graph.Edges[0].SourcePort = "next"
	graph.Edges[0].TargetPort = "input"
	return graph
}

func mustVisualWorkflowHashes(t *testing.T, graph visual.Graph) WorkflowGraphHashes {
	t.Helper()
	hashes, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatalf("graph hashes: %v", err)
	}
	return hashes
}
