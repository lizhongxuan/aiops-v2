package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"runner/scriptstore"
	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/store/eventstore"
	"runner/state"
	"runner/workflow"
	"runner/workflow/visual"
)

func TestVisualWorkflowServiceCompileSequentialGraph(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	graph := sampleVisualGraph()

	compiled, err := svc.Compile(context.Background(), graph)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	if compiled.Workflow.Name != "visual-demo" {
		t.Fatalf("workflow name mismatch: %s", compiled.Workflow.Name)
	}
	if len(compiled.Workflow.Steps) != 2 {
		t.Fatalf("expected 2 compiled steps, got %d", len(compiled.Workflow.Steps))
	}
	if compiled.Workflow.Steps[0].Name != "check disk" || compiled.Workflow.Steps[1].Name != "repair disk" {
		t.Fatalf("step order mismatch: %+v", compiled.Workflow.Steps)
	}
	if compiled.Workflow.Steps[1].When != "vars.disk_full == true" {
		t.Fatalf("condition edge should compile to step.when, got %q", compiled.Workflow.Steps[1].When)
	}
	if len(compiled.Workflow.Steps[1].Notify) != 1 || compiled.Workflow.Steps[1].Notify[0] != "notify ops" {
		t.Fatalf("handler edge should compile to step notify, got %+v", compiled.Workflow.Steps[1].Notify)
	}
	if len(compiled.Workflow.Handlers) != 1 || compiled.Workflow.Handlers[0].Name != "notify ops" {
		t.Fatalf("handler did not compile: %+v", compiled.Workflow.Handlers)
	}
	if !strings.Contains(compiled.YAML, "x_runner_ui:") || !strings.Contains(compiled.YAML, "x_runner_graph:") {
		t.Fatalf("compiled yaml should preserve visual metadata:\n%s", compiled.YAML)
	}
	loaded, err := workflow.Load([]byte(compiled.YAML))
	if err != nil {
		t.Fatalf("compiled yaml should load: %v", err)
	}
	if err := loaded.Validate(); err != nil {
		t.Fatalf("compiled yaml should validate: %v", err)
	}
}

func TestVisualWorkflowServiceParseYAMLToGraph(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	compiled, err := svc.Compile(context.Background(), sampleVisualGraph())
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	graph, err := svc.ParseYAML(context.Background(), compiled.YAML)
	if err != nil {
		t.Fatalf("parse YAML to graph: %v", err)
	}
	if graph.Workflow.Name != "visual-demo" || len(graph.Nodes) != len(sampleVisualGraph().Nodes) {
		t.Fatalf("parsed graph mismatch: %+v", graph)
	}
}

func TestVisualWorkflowServiceValidateAcceptsDAGAndCompilesGraphStrategy(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	graph := sampleVisualGraph()
	graph.Nodes = append(graph.Nodes, visual.Node{
		ID:   "extra",
		Type: visual.NodeTypeAction,
		Step: &workflow.Step{
			Name:    "extra",
			Targets: []string{"local"},
			Action:  "script.shell",
			Args:    map[string]any{"script": "echo extra"},
		},
	})
	graph.Edges = append(graph.Edges, visual.Edge{
		ID:     "check-extra",
		Source: "check",
		Target: "extra",
		Kind:   visual.EdgeKindSuccess,
	})
	graph.Edges = append(graph.Edges, visual.Edge{
		ID:     "extra-end",
		Source: "extra",
		Target: "end",
		Kind:   visual.EdgeKindSuccess,
	})

	result, err := svc.Validate(context.Background(), graph)
	if err != nil {
		t.Fatalf("validate graph: %v", err)
	}
	if !result.Valid {
		t.Fatalf("DAG graph should validate as graph metadata: %+v", result.Errors)
	}
	compiled, err := svc.Compile(context.Background(), graph)
	if err != nil {
		t.Fatalf("compile DAG graph: %v", err)
	}
	if compiled.Workflow.Plan.Strategy != "graph" || compiled.Workflow.XRunnerGraph == nil {
		t.Fatalf("DAG graph should compile as graph strategy with graph metadata: %+v", compiled.Workflow.Plan)
	}
}

func TestVisualWorkflowServiceDryRunBuildsRunFriendlyResponse(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})

	result, err := svc.DryRun(context.Background(), sampleVisualGraph(), map[string]any{"operator": "qa"}, "designer")
	if err != nil {
		t.Fatalf("dry run graph: %v", err)
	}
	if !result.Valid {
		t.Fatalf("dry run should be valid: %+v", result.Errors)
	}
	if result.WorkflowName != "visual-demo" || result.StepsCount != 2 {
		t.Fatalf("dry run workflow metadata mismatch: %+v", result)
	}
	if strings.Join(result.TargetHosts, ",") != "local" {
		t.Fatalf("dry run targets mismatch: %+v", result.TargetHosts)
	}
	if strings.Join(result.ActionsUsed, ",") != "script.shell" {
		t.Fatalf("dry run actions mismatch: %+v", result.ActionsUsed)
	}
	if result.RunRequest == nil {
		t.Fatal("dry run should include a run request")
	}
	if result.RunRequest.WorkflowName != "" {
		t.Fatalf("run request should use inline workflow yaml, got workflow_name=%q", result.RunRequest.WorkflowName)
	}
	if result.RunRequest.WorkflowYAML == "" || !strings.Contains(result.RunRequest.WorkflowYAML, "name: visual-demo") {
		t.Fatalf("run request yaml mismatch: %+v", result.RunRequest)
	}
	if result.RunRequest.TriggeredBy != "designer" {
		t.Fatalf("triggered_by mismatch: %q", result.RunRequest.TriggeredBy)
	}
	if result.RunRequest.Vars["operator"] != "qa" {
		t.Fatalf("vars mismatch: %+v", result.RunRequest.Vars)
	}
}

func TestVisualWorkflowServiceDryRunPrechecksCapabilitiesAndUndefinedVars(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "dry-run-precheck",
			Vars: map[string]any{
				"service": "billing-api",
			},
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{
					"app-01": {
						Address: "agent://app-01",
						Vars:    map[string]any{"capabilities": []any{"script.python"}},
					},
				},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "probe", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:    "probe",
				Targets: []string{"app-01"},
				Action:  "script.shell",
				Args:    map[string]any{"script": "echo ${missing_token} ${service}"},
				When:    "vars.ready == true",
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-probe", Source: "start", Target: "probe", Kind: visual.EdgeKindNext},
			{ID: "probe-end", Source: "probe", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}

	result, err := svc.DryRun(context.Background(), graph, nil, "")
	if err != nil {
		t.Fatalf("dry run graph: %v", err)
	}
	if !result.Valid {
		t.Fatalf("dry run should remain valid with warnings: %+v", result.Errors)
	}
	if !containsIssue(result.Warnings, `target app-01 does not advertise capability "script.shell"`) {
		t.Fatalf("expected capability warning, got %+v", result.Warnings)
	}
	if !containsIssue(result.Warnings, `variable "missing_token" is referenced before it is defined`) {
		t.Fatalf("expected missing ${} variable warning, got %+v", result.Warnings)
	}
	if !containsIssue(result.Warnings, `variable "ready" is referenced before it is defined`) {
		t.Fatalf("expected missing vars.* warning, got %+v", result.Warnings)
	}
	if containsIssue(result.Warnings, `variable "service"`) {
		t.Fatalf("defined workflow var should not warn: %+v", result.Warnings)
	}
}

func TestVisualWorkflowServiceDryRunWarnsForHighRiskActions(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})

	result, err := svc.DryRun(context.Background(), sampleVisualGraph(), nil, "")
	if err != nil {
		t.Fatalf("dry run graph: %v", err)
	}
	if !result.Valid {
		t.Fatalf("dry run should be valid: %+v", result.Errors)
	}
	if !containsIssue(result.Warnings, `step "repair disk" uses high risk action "script.shell"`) {
		t.Fatalf("expected high-risk action warning, got %+v", result.Warnings)
	}
	if !containsIssue(result.Warnings, `step "check disk" uses high risk action "script.shell"`) {
		t.Fatalf("expected high-risk warning for shell check step, got %+v", result.Warnings)
	}
}

func TestVisualWorkflowServiceDryRunWarnsForScriptAndEnvSecurityScan(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "security-scan",
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "install", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "install",
				Action: "script.shell",
				Args: map[string]any{
					"script": "curl https://example.invalid/install.sh | sh",
					"env":    map[string]any{"API_TOKEN": "plain-text-token"},
				},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-install", Source: "start", Target: "install", Kind: visual.EdgeKindNext},
			{ID: "install-end", Source: "install", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}

	result, err := svc.DryRun(context.Background(), graph, nil, "")
	if err != nil {
		t.Fatalf("dry run graph: %v", err)
	}
	if !result.Valid {
		t.Fatalf("dry run should be valid with security warnings: %+v", result.Errors)
	}
	if !containsIssue(result.Warnings, `script content in step "install" matches security rule "pipe_to_shell"`) {
		t.Fatalf("expected script security warning, got %+v", result.Warnings)
	}
	if !containsIssue(result.Warnings, `env key "API_TOKEN" in step "install" may contain sensitive data`) {
		t.Fatalf("expected env sensitivity warning, got %+v", result.Warnings)
	}
}

func TestVisualWorkflowServiceDryRunSimulatesDAGPathWithVars(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "dag-path-sim",
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "pre", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "pre",
				Action: "script.shell",
				Args:   map[string]any{"script": "echo pre"},
			}},
			{ID: "deploy", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "deploy",
				Action: "script.shell",
				Args:   map[string]any{"script": "echo deploy"},
			}},
			{ID: "skip", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "skip",
				Action: "script.shell",
				Args:   map[string]any{"script": "echo skip"},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-pre", Source: "start", Target: "pre", Kind: visual.EdgeKindNext},
			{ID: "pre-deploy", Source: "pre", Target: "deploy", Kind: visual.EdgeKindCondition, Condition: "vars.deploy == true"},
			{ID: "pre-skip", Source: "pre", Target: "skip", Kind: visual.EdgeKindCondition, Condition: "vars.deploy == false"},
			{ID: "deploy-end", Source: "deploy", Target: "end", Kind: visual.EdgeKindSuccess},
			{ID: "skip-end", Source: "skip", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}

	result, err := svc.DryRun(context.Background(), graph, map[string]any{"deploy": true}, "")
	if err != nil {
		t.Fatalf("dry run graph: %v", err)
	}
	if !result.Valid {
		t.Fatalf("dry run should be valid: %+v", result.Errors)
	}
	if result.PathSimulation == nil {
		t.Fatalf("dry run should include DAG path simulation")
	}
	if !stringSliceContains(result.PathSimulation.SelectedEdgeIDs, "pre-deploy") {
		t.Fatalf("expected deploy branch to be selected: %+v", result.PathSimulation)
	}
	if stringSliceContains(result.PathSimulation.SelectedEdgeIDs, "pre-skip") {
		t.Fatalf("skip branch should not be selected: %+v", result.PathSimulation)
	}
	if len(result.PathSimulation.Paths) != 1 || strings.Join(result.PathSimulation.Paths[0].NodeIDs, "->") != "start->pre->deploy->end" {
		t.Fatalf("unexpected simulated path: %+v", result.PathSimulation.Paths)
	}
}

func TestVisualWorkflowServiceDryRunReportsScriptReferenceIssues(t *testing.T) {
	scriptSvc := NewScriptService(scriptstore.NewFileStore(t.TempDir()))
	if err := scriptSvc.Create(context.Background(), &ScriptRecord{
		Name:     "shell-only",
		Language: "shell",
		Content:  "echo ok",
	}); err != nil {
		t.Fatalf("create script: %v", err)
	}
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{
		Preprocessor: NewPreprocessor(scriptSvc, nil, nil),
	})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "script-ref-precheck",
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "missing", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "missing",
				Action: "script.shell",
				Args:   map[string]any{"script_ref": "missing-script"},
			}},
			{ID: "mismatch", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "mismatch",
				Action: "script.python",
				Args:   map[string]any{"script_ref": "shell-only"},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-missing", Source: "start", Target: "missing", Kind: visual.EdgeKindNext},
			{ID: "missing-mismatch", Source: "missing", Target: "mismatch", Kind: visual.EdgeKindSuccess},
			{ID: "mismatch-end", Source: "mismatch", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}

	result, err := svc.DryRun(context.Background(), graph, nil, "")
	if err != nil {
		t.Fatalf("dry run graph: %v", err)
	}
	if result.Valid {
		t.Fatalf("dry run should be invalid for broken script refs")
	}
	if !containsIssue(result.Errors, `script_ref "missing-script" not found`) {
		t.Fatalf("expected missing script_ref error, got %+v", result.Errors)
	}
	if !containsIssue(result.Errors, `script_ref "shell-only" language "shell" does not match action "script.python"`) {
		t.Fatalf("expected language mismatch error, got %+v", result.Errors)
	}
	if !containsIssue(result.Errors, `step.args.script_ref`) {
		t.Fatalf("expected script_ref field in errors, got %+v", result.Errors)
	}
}

func TestVisualWorkflowServiceSubmitGraphRunExecutesLinearGraph(t *testing.T) {
	collector := metrics.NewCollector()
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), collector)
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "linear-submit",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "run", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:    "run",
				Targets: []string{"local"},
				Action:  "script.shell",
				Args:    map[string]any{"script": "echo ok"},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-run", Source: "start", Target: "run", Kind: visual.EdgeKindNext},
			{ID: "run-end", Source: "run", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), graph, nil, "tester", "", VisualWorkflowRunOptions{RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("submit graph run: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 3*time.Second)
	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if detail.Status != state.RunStatusSuccess {
		t.Fatalf("run status = %s, want success", detail.Status)
	}
	renderedMetrics := collector.RenderPrometheus()
	for _, name := range []string{
		"runner_server_graph_runs_finished_total",
		"runner_server_graph_run_duration_seconds_avg",
		"runner_server_graph_node_duration_seconds_avg",
		"runner_server_graph_node_failures_total",
		"runner_server_graph_approval_wait_seconds_avg",
	} {
		if !strings.Contains(renderedMetrics, name) {
			t.Fatalf("graph metrics missing %s:\n%s", name, renderedMetrics)
		}
	}
}

func TestVisualWorkflowServiceSubmitGraphRunCanScopeToSingleNode(t *testing.T) {
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "single-node-submit",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
		},
		Nodes: []visual.Node{
			{ID: "first", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "first", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo first"}}},
			{ID: "second", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "second", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo second"}}},
		},
		Edges: []visual.Edge{{ID: "first-second", Source: "first", Target: "second", Kind: visual.EdgeKindNext}},
	}

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), graph, nil, "tester", "", VisualWorkflowRunOptions{NodeID: "second", RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("submit single node graph run: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 3*time.Second)
	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if got, want := len(detail.Steps), 1; got != want {
		t.Fatalf("steps = %d, want %d: %+v", got, want, detail.Steps)
	}
	if detail.Steps[0].Name != "second" {
		t.Fatalf("single node run step = %q, want second", detail.Steps[0].Name)
	}
	if strings.Contains(detail.WorkflowYAML, "echo first") {
		t.Fatalf("single node run yaml should not include upstream step: %s", detail.WorkflowYAML)
	}
}

func TestVisualWorkflowServiceSubmitGraphRunExecutesDAG(t *testing.T) {
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})
	graph := visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "dag-submit",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "fork", Type: visual.NodeTypeParallel},
			{ID: "a", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "a", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo a"}}},
			{ID: "b", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "b", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo b"}}},
			{ID: "join", Type: visual.NodeTypeJoin, Join: &visual.JoinSpec{Strategy: "all_success"}},
			{ID: "finish", Type: visual.NodeTypeAction, Step: &workflow.Step{Name: "finish", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo done"}}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-fork", Source: "start", Target: "fork", Kind: visual.EdgeKindNext},
			{ID: "fork-a", Source: "fork", Target: "a", Kind: visual.EdgeKindNext},
			{ID: "fork-b", Source: "fork", Target: "b", Kind: visual.EdgeKindNext},
			{ID: "a-join", Source: "a", Target: "join", Kind: visual.EdgeKindSuccess},
			{ID: "b-join", Source: "b", Target: "join", Kind: visual.EdgeKindSuccess},
			{ID: "join-finish", Source: "join", Target: "finish", Kind: visual.EdgeKindSuccess},
			{ID: "finish-end", Source: "finish", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), graph, nil, "tester", "", VisualWorkflowRunOptions{RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("submit DAG graph run: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 3*time.Second)
	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if detail.Status != state.RunStatusSuccess {
		t.Fatalf("run status = %s, want success", detail.Status)
	}
	runGraph, err := svc.GetRunGraph(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run graph: %v", err)
	}
	if len(runGraph.Nodes) != len(graph.Nodes) {
		t.Fatalf("run graph nodes = %d, want %d", len(runGraph.Nodes), len(graph.Nodes))
	}
	if node := visualNodeByID(runGraph, "start"); node == nil || node.State == nil || node.State.Status != state.RunStatusSuccess {
		t.Fatalf("start node state not overlaid: %+v", node)
	}
	if edge := visualEdgeByID(runGraph, "start-fork"); edge == nil || edge.State == nil || edge.State.Status != "selected" {
		t.Fatalf("selected edge state not overlaid: %+v", edge)
	}
}

func TestVisualWorkflowServiceManualApprovalApproveResumesRun(t *testing.T) {
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1, EventStore: eventstore.NewFileStore(t.TempDir())}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), manualApprovalGraph(), nil, "tester", "", VisualWorkflowRunOptions{RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("submit manual approval graph: %v", err)
	}
	waitRunNodeStatus(t, runSvc, resp.RunID, "approve", "waiting", 3*time.Second)

	if err := svc.ApproveNode(context.Background(), resp.RunID, "approve", "sre", "approved for prod"); err != nil {
		t.Fatalf("approve node: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 3*time.Second)
	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if detail.Status != state.RunStatusSuccess {
		t.Fatalf("run status = %s, want success", detail.Status)
	}
	if got := detail.Graph.Nodes["approve"].Status; got != state.RunStatusSuccess {
		t.Fatalf("approval node status = %q, want success", got)
	}
	if got := detail.Graph.Nodes["after"].Status; got != state.RunStatusSuccess {
		t.Fatalf("after node status = %q, want success", got)
	}
	history, err := runSvc.History(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("history: %v", err)
	}
	if !eventTypesContain(history, "approval_waiting", "approval_resolved") {
		t.Fatalf("approval events missing from history: %+v", history)
	}
}

func TestVisualWorkflowServiceManualApprovalRejectUsesRejectedEdge(t *testing.T) {
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), manualApprovalGraph(), nil, "tester", "", VisualWorkflowRunOptions{RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("submit manual approval graph: %v", err)
	}
	waitRunNodeStatus(t, runSvc, resp.RunID, "approve", "waiting", 3*time.Second)

	if err := svc.RejectNode(context.Background(), resp.RunID, "approve", "sre", "blocked"); err != nil {
		t.Fatalf("reject node: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 3*time.Second)
	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if detail.Status != state.RunStatusSuccess {
		t.Fatalf("run status = %s, want success via rejected edge", detail.Status)
	}
	if got := detail.Graph.Nodes["approve"].Status; got != state.RunStatusFailed {
		t.Fatalf("approval node status = %q, want failed", got)
	}
	if got := detail.Graph.Nodes["notify"].Status; got != state.RunStatusSuccess {
		t.Fatalf("notify node status = %q, want success", got)
	}
	if got := detail.Graph.Nodes["after"].Status; got != state.RunStatusQueued {
		t.Fatalf("approved branch should remain queued, after status=%q", got)
	}
}

func TestVisualWorkflowServiceSubflowRunsSavedWorkflow(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	if err := workflowSvc.Create(context.Background(), &WorkflowRecord{
		Name: "child-flow",
		RawYAML: []byte(`
version: v0.1
name: child-flow
inventory:
  hosts:
    local:
      address: local
steps:
  - id: child-step
    name: child-step
    targets: [local]
    action: script.shell
    args:
      script: echo child
`),
	}); err != nil {
		t.Fatalf("create child workflow: %v", err)
	}
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, workflowSvc, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc, RunService: runSvc})

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), subflowGraph(), nil, "tester", "", VisualWorkflowRunOptions{RiskAcknowledged: true})
	if err != nil {
		t.Fatalf("submit subflow graph: %v", err)
	}
	waitRunTerminal(t, runSvc, resp.RunID, 3*time.Second)
	detail, err := runSvc.Get(context.Background(), resp.RunID)
	if err != nil {
		t.Fatalf("get run detail: %v", err)
	}
	if detail.Status != state.RunStatusSuccess {
		t.Fatalf("run status = %s, want success", detail.Status)
	}
	if got := detail.Graph.Nodes["child"].Status; got != state.RunStatusSuccess {
		t.Fatalf("subflow node status = %q, want success", got)
	}
	if got := detail.Graph.Nodes["after"].Status; got != state.RunStatusSuccess {
		t.Fatalf("after node status = %q, want success", got)
	}
	if stepStatus(detail.Steps, "child-step") != state.RunStatusSuccess {
		t.Fatalf("child workflow step should be included in run trace: %+v", detail.Steps)
	}
}

func TestVisualWorkflowServiceSubmitGraphRunRequiresRiskAcknowledgement(t *testing.T) {
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})

	_, err := svc.SubmitGraphRun(context.Background(), sampleVisualGraph(), nil, "tester", "")
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("submit high-risk graph without acknowledgement error = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "risk_acknowledged") {
		t.Fatalf("high-risk submit error should mention risk_acknowledged, got %v", err)
	}

	resp, err := svc.SubmitGraphRunWithOptions(context.Background(), sampleVisualGraph(), nil, "tester", "", VisualWorkflowRunOptions{
		RiskAcknowledged: true,
	})
	if err != nil {
		t.Fatalf("submit high-risk graph with acknowledgement: %v", err)
	}
	if resp.RunID == "" {
		t.Fatal("acknowledged high-risk submit returned empty run id")
	}
}

func TestVisualWorkflowServiceSubmitGraphRunEnforcesAgentCapabilities(t *testing.T) {
	runSvc := NewRunService(RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{RunService: runSvc})

	graph := capabilityMismatchGraph()
	_, err := svc.SubmitGraphRunWithOptions(context.Background(), graph, nil, "tester", "", VisualWorkflowRunOptions{RiskAcknowledged: true})
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("submit graph with mismatched capability error = %v, want ErrInvalid", err)
	}
	if !strings.Contains(err.Error(), "capability") {
		t.Fatalf("capability submit error should mention capability, got %v", err)
	}
}

func TestVisualWorkflowServiceDryRunReturnsValidationErrors(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	graph := sampleVisualGraph()
	graph.Nodes[1].Step.Args = nil

	result, err := svc.DryRun(context.Background(), graph, nil, "")
	if err != nil {
		t.Fatalf("dry run invalid graph: %v", err)
	}
	if result.Valid {
		t.Fatalf("dry run should be invalid")
	}
	if result.YAML != "" || result.RunRequest != nil {
		t.Fatalf("invalid dry run should not include executable yaml: %+v", result)
	}
	if !containsIssue(result.Errors, "requires args.script") {
		t.Fatalf("expected missing action arg issue, got %+v", result.Errors)
	}
	if result.Errors[0].Code == "" || result.Errors[0].Code != result.Errors[0].Type {
		t.Fatalf("validation issue should expose code alias matching type: %+v", result.Errors[0])
	}
	if result.Errors[0].Suggestion == "" {
		t.Fatalf("validation issue should include actionable suggestion: %+v", result.Errors[0])
	}
}

func TestVisualWorkflowServiceValidateUsesPreprocessor(t *testing.T) {
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{
		Preprocessor: NewPreprocessor(nil, nil, []string{"script.python"}),
	})

	result, err := svc.Validate(context.Background(), sampleVisualGraph())
	if err != nil {
		t.Fatalf("validate graph with preprocessor: %v", err)
	}
	if result.Valid {
		t.Fatal("graph should be invalid when preprocessor action whitelist rejects script.shell")
	}
	if !containsIssue(result.Errors, "not allowed") {
		t.Fatalf("expected preprocessor action whitelist issue, got %+v", result.Errors)
	}
}

func TestVisualWorkflowGraphHashSeparatesSemanticAndLayout(t *testing.T) {
	graph := sampleVisualGraph()
	base, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatalf("graph hashes: %v", err)
	}
	if base.SemanticHash == "" || base.LayoutHash == "" {
		t.Fatalf("hashes should be populated: %+v", base)
	}

	layoutOnly := sampleVisualGraph()
	layoutOnly.Nodes[1].Position.X += 240
	layoutOnly.Layout.Viewport.Zoom = 0.72
	layoutOnly.Nodes[1].UI = map[string]any{"selected": true}
	layoutHashes, err := VisualWorkflowGraphHashes(layoutOnly)
	if err != nil {
		t.Fatalf("layout graph hashes: %v", err)
	}
	if layoutHashes.SemanticHash != base.SemanticHash {
		t.Fatalf("layout-only change should not alter semantic hash: base=%s layout=%s", base.SemanticHash, layoutHashes.SemanticHash)
	}
	if layoutHashes.LayoutHash == base.LayoutHash {
		t.Fatalf("layout hash should change when positions or viewport change")
	}

	semanticChange := sampleVisualGraph()
	semanticChange.Nodes[1].Step.Args["script"] = "df -h /var/lib/postgresql"
	semanticHashes, err := VisualWorkflowGraphHashes(semanticChange)
	if err != nil {
		t.Fatalf("semantic graph hashes: %v", err)
	}
	if semanticHashes.SemanticHash == base.SemanticHash {
		t.Fatalf("semantic change should alter semantic hash")
	}
}

func TestVisualWorkflowServiceSaveAndGetGraph(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	initial := sampleVisualGraph()
	compiled, err := NewVisualWorkflowService(VisualWorkflowServiceConfig{}).Compile(context.Background(), initial)
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	if err := workflowSvc.Create(context.Background(), &WorkflowRecord{
		Name:    initial.Workflow.Name,
		RawYAML: []byte(compiled.YAML),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	loaded, err := svc.GetGraph(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	if got, want := len(loaded.Nodes), len(initial.Nodes); got != want {
		t.Fatalf("loaded nodes = %d, want %d", got, want)
	}
	if loaded.UI[graphResourceVersionKey] == "" {
		t.Fatalf("loaded graph missing resource version: %+v", loaded.UI)
	}
	loaded.Workflow.Description = "updated"
	if _, err := svc.SaveGraph(context.Background(), initial.Workflow.Name, loaded); err != nil {
		t.Fatalf("save graph: %v", err)
	}
	record, err := workflowSvc.Get(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get saved workflow: %v", err)
	}
	if !strings.Contains(string(record.RawYAML), "description: updated") {
		t.Fatalf("saved yaml did not include update:\n%s", string(record.RawYAML))
	}
	if strings.Contains(string(record.RawYAML), graphResourceVersionKey) {
		t.Fatalf("resource version should not be persisted into workflow yaml:\n%s", string(record.RawYAML))
	}
}

func TestVisualWorkflowServiceSaveGraphPersistsSaveNote(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	initial := sampleVisualGraph()
	compiled, err := NewVisualWorkflowService(VisualWorkflowServiceConfig{}).Compile(context.Background(), initial)
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	if err := workflowSvc.Create(context.Background(), &WorkflowRecord{
		Name:    initial.Workflow.Name,
		RawYAML: []byte(compiled.YAML),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	loaded, err := svc.GetGraph(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get graph: %v", err)
	}
	loaded.Workflow.Description = "updated with note"
	if _, err := svc.SaveGraphWithOptions(context.Background(), initial.Workflow.Name, loaded, VisualWorkflowSaveOptions{
		SaveNote: "operator explained restore target change",
	}); err != nil {
		t.Fatalf("save graph with note: %v", err)
	}

	record, err := workflowSvc.Get(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get saved workflow: %v", err)
	}
	if got := record.SaveNote; got != "operator explained restore target change" {
		t.Fatalf("save note = %q, want %q", got, "operator explained restore target change")
	}
}

func TestVisualWorkflowServiceSaveGraphDetectsVersionConflict(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	initial := sampleVisualGraph()
	compiled, err := NewVisualWorkflowService(VisualWorkflowServiceConfig{}).Compile(context.Background(), initial)
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	if err := workflowSvc.Create(context.Background(), &WorkflowRecord{
		Name:    initial.Workflow.Name,
		RawYAML: []byte(compiled.YAML),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	first, err := svc.GetGraph(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get first graph: %v", err)
	}
	second, err := svc.GetGraph(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get second graph: %v", err)
	}

	first.Workflow.Description = "first writer"
	if _, err := svc.SaveGraph(context.Background(), initial.Workflow.Name, first); err != nil {
		t.Fatalf("save first graph: %v", err)
	}

	second.Workflow.Description = "stale writer"
	if _, err := svc.SaveGraph(context.Background(), initial.Workflow.Name, second); !errors.Is(err, ErrConflict) {
		t.Fatalf("expected ErrConflict for stale graph save, got %v", err)
	}

	latest, err := svc.GetGraph(context.Background(), initial.Workflow.Name)
	if err != nil {
		t.Fatalf("get latest graph: %v", err)
	}
	latest.Workflow.Description = "latest writer"
	if _, err := svc.SaveGraph(context.Background(), initial.Workflow.Name, latest); err != nil {
		t.Fatalf("save latest graph after refresh: %v", err)
	}
}

func TestVisualWorkflowServiceCreateGraphPersistsDraft(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	graph := sampleVisualGraph()
	graph.Workflow.Name = "visual-create"
	graph.Workflow.Description = "created from visual UI"
	graph.UI = map[string]any{graphResourceVersionKey: "sha256:stale"}

	created, err := svc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{
		Labels:   map[string]string{"source": "visual-ui"},
		SaveNote: "initial visual workflow draft",
	})
	if err != nil {
		t.Fatalf("create graph: %v", err)
	}
	if created.Name != "visual-create" || created.Status != WorkflowStatusDraft {
		t.Fatalf("created metadata mismatch: %+v", created)
	}
	if created.Workflow.Name != "visual-create" || created.Graph.Workflow.Name != "visual-create" {
		t.Fatalf("created workflow/graph name mismatch: workflow=%q graph=%q", created.Workflow.Name, created.Graph.Workflow.Name)
	}
	if created.YAML == "" || !strings.Contains(created.YAML, "name: visual-create") {
		t.Fatalf("created yaml mismatch:\n%s", created.YAML)
	}
	if created.Graph.UI[graphResourceVersionKey] == "" {
		t.Fatalf("created graph missing resource version: %+v", created.Graph.UI)
	}

	record, err := workflowSvc.Get(context.Background(), "visual-create")
	if err != nil {
		t.Fatalf("get created workflow: %v", err)
	}
	if record.Status != WorkflowStatusDraft {
		t.Fatalf("created workflow status = %q, want draft", record.Status)
	}
	if record.Labels["source"] != "visual-ui" {
		t.Fatalf("created labels mismatch: %+v", record.Labels)
	}
	if record.SaveNote != "initial visual workflow draft" {
		t.Fatalf("save note = %q", record.SaveNote)
	}
	if strings.Contains(string(record.RawYAML), graphResourceVersionKey) {
		t.Fatalf("resource version should not be persisted into workflow yaml:\n%s", string(record.RawYAML))
	}
}

func TestVisualWorkflowServiceCreateGraphRejectsInvalidGraph(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	graph := sampleVisualGraph()
	graph.Workflow.Name = ""

	if _, err := svc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expected ErrInvalid for missing workflow name, got %v", err)
	}
}

func TestVisualWorkflowServiceCreateGraphDetectsDuplicateName(t *testing.T) {
	workflowSvc := NewWorkflowService(t.TempDir())
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	graph := sampleVisualGraph()
	graph.Workflow.Name = "visual-create-duplicate"

	if _, err := svc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{}); err != nil {
		t.Fatalf("first create graph: %v", err)
	}
	if _, err := svc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{}); !errors.Is(err, ErrAlreadyExists) {
		t.Fatalf("expected ErrAlreadyExists for duplicate graph create, got %v", err)
	}
}

func capabilityMismatchGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "capability-submit",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{
					"app-01": {
						Address: "agent://app-01",
						Vars:    map[string]any{"capabilities": []any{"script.python"}},
					},
				},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "probe", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:    "probe",
				Targets: []string{"app-01"},
				Action:  "script.shell",
				Args:    map[string]any{"script": "echo ok"},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-probe", Source: "start", Target: "probe", Kind: visual.EdgeKindNext},
			{ID: "probe-end", Source: "probe", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func subflowGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "subflow-submit",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{
					"local": {Address: "local"},
				},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{
				ID:   "child",
				Type: visual.NodeTypeSubflow,
				Step: &workflow.Step{
					Name:   "child",
					Action: "workflow.run",
					Args:   map[string]any{"workflow": "child-flow"},
				},
				Subflow: &visual.SubflowSpec{
					WorkflowName: "child-flow",
					Vars:         map[string]any{"region": "hz"},
				},
			},
			{
				ID:   "after",
				Type: visual.NodeTypeAction,
				Step: &workflow.Step{
					Name:    "after",
					Targets: []string{"local"},
					Action:  "script.shell",
					Args:    map[string]any{"script": "echo after"},
				},
			},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-child", Source: "start", Target: "child", Kind: visual.EdgeKindNext},
			{ID: "child-after", Source: "child", Target: "after", Kind: visual.EdgeKindSuccess},
			{ID: "after-end", Source: "after", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func manualApprovalGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "manual-approval-submit",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{
					"local": {Address: "local"},
				},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{
				ID:   "approve",
				Type: visual.NodeTypeManualApproval,
				Step: &workflow.Step{
					Name:   "approve",
					Action: "manual.approval",
				},
				Approval: &visual.ApprovalSpec{
					Subjects:  []string{"sre"},
					Timeout:   "30m",
					OnTimeout: "reject",
				},
			},
			{
				ID:   "after",
				Type: visual.NodeTypeAction,
				Step: &workflow.Step{
					Name:    "after",
					Targets: []string{"local"},
					Action:  "script.shell",
					Args:    map[string]any{"script": "echo after"},
				},
			},
			{
				ID:   "notify",
				Type: visual.NodeTypeAction,
				Step: &workflow.Step{
					Name:    "notify",
					Targets: []string{"local"},
					Action:  "script.shell",
					Args:    map[string]any{"script": "echo notify"},
				},
			},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-approve", Source: "start", Target: "approve", Kind: visual.EdgeKindNext},
			{ID: "approve-after", Source: "approve", Target: "after", Kind: visual.EdgeKindApprovalApproved},
			{ID: "approve-notify", Source: "approve", Target: "notify", Kind: visual.EdgeKindApprovalRejected},
			{ID: "after-end", Source: "after", Target: "end", Kind: visual.EdgeKindSuccess},
			{ID: "notify-end", Source: "notify", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func sampleVisualGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Name:    "visual-demo",
			Version: "v0.1",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{
					"local": {Address: "127.0.0.1"},
				},
			},
		},
		Nodes: []visual.Node{
			{
				ID:    "start",
				Type:  visual.NodeTypeStart,
				Label: "Start",
			},
			{
				ID:   "check",
				Type: visual.NodeTypeAction,
				Step: &workflow.Step{
					Name:    "check disk",
					Targets: []string{"local"},
					Action:  "script.shell",
					Args:    map[string]any{"script": "df -h"},
				},
			},
			{
				ID:   "repair",
				Type: visual.NodeTypeAction,
				Step: &workflow.Step{
					Name:    "repair disk",
					Targets: []string{"local"},
					Action:  "script.shell",
					Args:    map[string]any{"script": "echo cleanup"},
				},
			},
			{
				ID:   "notify",
				Type: visual.NodeTypeHandler,
				Handler: &workflow.Handler{
					Name:   "notify ops",
					Action: "script.shell",
					Args:   map[string]any{"script": "echo notify"},
				},
			},
			{
				ID:    "end",
				Type:  visual.NodeTypeEnd,
				Label: "End",
			},
		},
		Edges: []visual.Edge{
			{ID: "start-check", Source: "start", Target: "check", Kind: visual.EdgeKindNext},
			{ID: "check-repair", Source: "check", Target: "repair", Kind: visual.EdgeKindCondition, Condition: "vars.disk_full == true"},
			{ID: "repair-notify", Source: "repair", Target: "notify", Kind: visual.EdgeKindSuccess},
			{ID: "repair-end", Source: "repair", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func waitRunNodeStatus(t *testing.T, svc *RunService, runID, nodeID, status string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		detail, err := svc.Get(context.Background(), runID)
		if err == nil && detail.Graph != nil {
			if node, ok := detail.Graph.Nodes[nodeID]; ok && node.Status == status {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	detail, err := svc.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("run %s did not reach node %s status %s; get failed: %v", runID, nodeID, status, err)
	}
	if detail.Graph == nil {
		t.Fatalf("run %s did not reach node %s status %s; graph state is nil", runID, nodeID, status)
	}
	t.Fatalf("run %s node %s status = %q, want %q", runID, nodeID, detail.Graph.Nodes[nodeID].Status, status)
}

func eventTypesContain(items []events.Event, expected ...string) bool {
	seen := map[string]struct{}{}
	for _, item := range items {
		seen[item.Type] = struct{}{}
	}
	for _, eventType := range expected {
		if _, ok := seen[eventType]; !ok {
			return false
		}
	}
	return true
}

func stepStatus(items []state.StepState, name string) string {
	for _, item := range items {
		if item.Name == name {
			return item.Status
		}
	}
	return ""
}

func containsIssue(issues []VisualWorkflowIssue, text string) bool {
	for _, item := range issues {
		if strings.Contains(item.Message, text) ||
			strings.Contains(item.Field, text) ||
			strings.Contains(item.Type, text) ||
			strings.Contains(item.NodeID, text) {
			return true
		}
	}
	return false
}

func stringSliceContains(items []string, value string) bool {
	for _, item := range items {
		if item == value {
			return true
		}
	}
	return false
}

func visualNodeByID(graph visual.Graph, id string) *visual.Node {
	for i := range graph.Nodes {
		if graph.Nodes[i].ID == id {
			return &graph.Nodes[i]
		}
	}
	return nil
}

func visualEdgeByID(graph visual.Graph, id string) *visual.Edge {
	for i := range graph.Edges {
		if graph.Edges[i].ID == id {
			return &graph.Edges[i]
		}
	}
	return nil
}
