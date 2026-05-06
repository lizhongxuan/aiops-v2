package visual

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
	"time"

	"runner/state"
	"runner/workflow"

	"gopkg.in/yaml.v3"
)

const sequentialYAML = `version: v0.1
name: demo
inventory:
  hosts:
    local:
      address: local
plan:
  mode: auto
  strategy: sequential
steps:
  - name: prepare
    targets: [local]
    action: shell.run
    args:
      script: echo prepare
  - name: deploy
    targets: [local]
    action: shell.run
    args:
      script: echo deploy
`

func TestParseYAMLToGraphSequentialCompatibility(t *testing.T) {
	g, err := ParseYAMLToGraph([]byte(sequentialYAML))
	if err != nil {
		t.Fatalf("parse graph: %v", err)
	}
	if got, want := len(g.Nodes), 4; got != want {
		t.Fatalf("nodes = %d, want %d", got, want)
	}
	if got, want := len(g.Edges), 3; got != want {
		t.Fatalf("edges = %d, want %d", got, want)
	}
	if g.Nodes[0].Type != NodeTypeStart {
		t.Fatalf("first node type = %q, want start", g.Nodes[0].Type)
	}
	if g.Nodes[1].Step == nil || g.Nodes[1].Step.Name != "prepare" {
		t.Fatalf("first step node was not hydrated from workflow: %+v", g.Nodes[1])
	}

	compiled, err := CompileGraphToYAML(g)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	if !strings.Contains(string(compiled), "x_runner_ui:") {
		t.Fatalf("compiled YAML missing x_runner_ui:\n%s", string(compiled))
	}
	wf, err := workflow.Load(compiled)
	if err != nil {
		t.Fatalf("workflow.Load compiled YAML: %v", err)
	}
	if err := wf.Validate(); err != nil {
		t.Fatalf("compiled workflow validate: %v", err)
	}
	if got := []string{wf.Steps[0].Name, wf.Steps[1].Name}; got[0] != "prepare" || got[1] != "deploy" {
		t.Fatalf("compiled step order = %v", got)
	}
}

func TestParseAndCompileDAGPreservesGraphMetadata(t *testing.T) {
	raw := `version: v0.1
name: dag-demo
x_runner_ui:
  version: v1
  layout:
    direction: LR
    viewport: {x: 10, y: 20, zoom: 0.75}
  nodes:
    - id: start
      type: start
      position: {x: 0, y: 100}
    - id: pre
      type: action
      step: pre
      position: {x: 200, y: 100}
    - id: branch-a
      type: action
      step: branch-a
      position: {x: 400, y: 40}
    - id: branch-b
      type: condition
      step: branch-b
      position: {x: 400, y: 160}
    - id: join
      type: action
      step: join
      position: {x: 640, y: 100}
  edges:
    - id: edge-start-pre
      source: start
      target: pre
      kind: next
    - id: edge-pre-a
      source: pre
      target: branch-a
      kind: success
    - id: edge-pre-b
      source: pre
      target: branch-b
      kind: condition
      condition: vars.deploy == true
    - id: edge-a-join
      source: branch-a
      target: join
      kind: next
    - id: edge-b-join
      source: branch-b
      target: join
      kind: next
plan:
  mode: auto
  strategy: sequential
steps:
  - name: pre
    action: shell.run
  - name: branch-a
    action: shell.run
  - name: branch-b
    action: condition.evaluate
  - name: join
    action: shell.run
`
	g, err := ParseYAMLToGraph([]byte(raw))
	if err != nil {
		t.Fatalf("parse DAG graph: %v", err)
	}
	if got, want := len(g.Edges), 6; got != want {
		t.Fatalf("edges = %d, want %d", got, want)
	}
	if g.Layout.Viewport.Zoom != 0.75 {
		t.Fatalf("viewport zoom = %v, want 0.75", g.Layout.Viewport.Zoom)
	}

	compiled, err := CompileGraphToYAML(g)
	if err != nil {
		t.Fatalf("compile DAG graph: %v", err)
	}
	compiledText := string(compiled)
	if !strings.Contains(compiledText, "kind: condition") || !strings.Contains(compiledText, "condition: vars.deploy == true") {
		t.Fatalf("compiled YAML did not preserve condition edge:\n%s", compiledText)
	}
	wf, err := workflow.Load(compiled)
	if err != nil {
		t.Fatalf("workflow.Load DAG YAML: %v", err)
	}
	if err := wf.Validate(); err != nil {
		t.Fatalf("compiled DAG workflow validate: %v", err)
	}
	if wf.Steps[len(wf.Steps)-1].Name != "join" {
		t.Fatalf("join step should compile after both branches, got order %+v", wf.Steps)
	}
}

func TestParseAndCompilePreservesUnknownTopLevelFields(t *testing.T) {
	raw := `version: v0.1
name: extension-demo
x_vendor_policy:
  change_ticket: CHG-1234
  reviewer: sre
owner: platform
steps:
  - name: run
    action: cmd.run
    args:
      cmd: echo ok
`
	g, err := ParseYAMLToGraph([]byte(raw))
	if err != nil {
		t.Fatalf("parse graph: %v", err)
	}
	if g.Workflow.Extensions["owner"] != "platform" {
		t.Fatalf("workflow extensions not loaded: %+v", g.Workflow.Extensions)
	}

	compiled, err := CompileGraphToYAML(g)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	compiledText := string(compiled)
	if !strings.Contains(compiledText, "x_vendor_policy:") || !strings.Contains(compiledText, "change_ticket: CHG-1234") || !strings.Contains(compiledText, "owner: platform") {
		t.Fatalf("compiled YAML dropped unknown top-level fields:\n%s", compiledText)
	}
	wf, err := workflow.Load(compiled)
	if err != nil {
		t.Fatalf("load compiled YAML: %v", err)
	}
	if wf.Extensions["owner"] != "platform" {
		t.Fatalf("compiled workflow extensions not preserved: %+v", wf.Extensions)
	}
}

func TestParseLegacyYAMLLayoutsWrappedChainAndNotifyLane(t *testing.T) {
	raw := `version: v0.1
name: legacy-controls
vars:
  enabled: true
steps:
  - name: prepare
    targets: [local]
    action: cmd.run
    when: vars.enabled == true
    loop: [a, b]
    must_vars: [token]
    expect_vars: [ready]
    continue_on_error: true
    notify: [notify-ops]
    args:
      cmd: echo prepare
  - name: step-2
    action: cmd.run
  - name: step-3
    action: cmd.run
  - name: step-4
    action: cmd.run
  - name: step-5
    action: cmd.run
  - name: step-6
    action: cmd.run
  - name: step-7
    action: cmd.run
handlers:
  - name: notify-ops
    action: cmd.run
    args:
      cmd: echo notify
`
	g, err := ParseYAMLToGraph([]byte(raw))
	if err != nil {
		t.Fatalf("parse graph: %v", err)
	}
	prepare := findNodeByStepName(t, g, "prepare")
	if prepare.Step.When != "vars.enabled == true" || !prepare.Step.ContinueOnError {
		t.Fatalf("step controls not hydrated: %+v", prepare.Step)
	}
	if got := strings.Join(prepare.Step.MustVars, ","); got != "token" {
		t.Fatalf("must vars = %q", got)
	}
	if got := strings.Join(prepare.Step.ExpectVars, ","); got != "ready" {
		t.Fatalf("expect vars = %q", got)
	}
	if got := len(prepare.Step.Loop); got != 2 {
		t.Fatalf("loop items = %d, want 2", got)
	}

	step6 := findNodeByStepName(t, g, "step-6")
	if step6.Position.Y <= prepare.Position.Y {
		t.Fatalf("long legacy chain was not wrapped: prepare=%+v step6=%+v", prepare.Position, step6.Position)
	}
	notify := findNodeByHandlerName(t, g, "notify-ops")
	if notify.Position.Y <= step6.Position.Y {
		t.Fatalf("handler node not placed in auxiliary lane: handler=%+v wrapped=%+v", notify.Position, step6.Position)
	}
	if !hasEdge(g, prepare.ID, notify.ID, EdgeKindAlways) {
		t.Fatalf("notify edge from %q to %q was not generated: %+v", prepare.ID, notify.ID, g.Edges)
	}

	compiled, err := CompileGraphToYAML(g)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	compiledWF, err := workflow.Load(compiled)
	if err != nil {
		t.Fatalf("load compiled workflow: %v", err)
	}
	if got := strings.Join(compiledWF.Steps[0].Notify, ","); got != "notify-ops" {
		t.Fatalf("compiled notify = %q", got)
	}
}

func TestParseYAMLToGraphClassifiesFailures(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		kind ParseErrorKind
	}{
		{
			name: "yaml syntax",
			raw:  "version: [",
			kind: ParseErrorKindYAMLSyntax,
		},
		{
			name: "workflow validation",
			raw: `version: v0.1
steps: []
`,
			kind: ParseErrorKindWorkflowValidation,
		},
		{
			name: "graph validation",
			raw: `version: v0.1
name: graph-invalid
steps:
  - name: run
    action: cmd.run
x_runner_graph:
  version: v1
  nodes:
    - id: start
      type: start
    - id: run
      type: action
      step: run
  edges:
    - id: self
      source: run
      target: run
      kind: next
`,
			kind: ParseErrorKindGraphValidation,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseYAMLToGraph([]byte(tc.raw))
			if err == nil {
				t.Fatal("expected parse error")
			}
			var parseErr *ParseError
			if !errors.As(err, &parseErr) {
				t.Fatalf("expected ParseError, got %T %v", err, err)
			}
			if parseErr.Kind != tc.kind {
				t.Fatalf("parse kind = %q, want %q; err=%v", parseErr.Kind, tc.kind, err)
			}
		})
	}
}

func TestValidateGraphRejectsCycles(t *testing.T) {
	stepA := workflow.Step{Name: "a", Action: "shell.run"}
	stepB := workflow.Step{Name: "b", Action: "shell.run"}
	g := Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "cycle",
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{ID: "a", Type: NodeTypeAction, StepName: "a", Step: &stepA},
			{ID: "b", Type: NodeTypeAction, StepName: "b", Step: &stepB},
		},
		Edges: []Edge{
			{ID: "start-a", Source: "start", Target: "a", Kind: EdgeKindNext},
			{ID: "a-b", Source: "a", Target: "b", Kind: EdgeKindNext},
			{ID: "b-a", Source: "b", Target: "a", Kind: EdgeKindNext},
		},
	}
	err := ValidateGraph(g)
	if err == nil {
		t.Fatal("expected cycle validation error")
	}
	verr, ok := err.(*ValidationError)
	if !ok {
		t.Fatalf("expected ValidationError, got %T", err)
	}
	assertVisualIssue(t, verr.Issues, "graph_cycle")
}

func TestCompileGraphSupportsAdvancedNodeTypes(t *testing.T) {
	approval := workflow.Step{Name: "approve", Action: "manual.approval"}
	subflow := workflow.Step{Name: "child", Action: "workflow.run", Args: map[string]any{"workflow": "child-flow"}}
	condition := workflow.Step{Name: "gate", Action: "condition.evaluate", When: "vars.enabled == true"}
	handler := workflow.Handler{Name: "notify", Action: "shell.run"}
	g := Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "advanced",
			Plan:    workflow.Plan{Mode: "manual-approve", Strategy: "sequential"},
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart},
			{ID: "group", Type: NodeTypeGroup, Label: "Release"},
			{ID: "approve", Type: NodeTypeManualApproval, StepName: approval.Name, Step: &approval, Approval: &ApprovalSpec{Subjects: []string{"ops"}, Timeout: "30m", OnTimeout: "reject"}, ParentID: "group"},
			{ID: "gate", Type: NodeTypeCondition, StepName: condition.Name, Step: &condition, ParentID: "group"},
			{ID: "child", Type: NodeTypeSubflow, StepName: subflow.Name, Step: &subflow, Subflow: &SubflowSpec{WorkflowName: "child-flow"}, ParentID: "group"},
			{ID: "handler-notify", Type: NodeTypeHandler, HandlerName: handler.Name, Handler: &handler},
			{ID: "end", Type: NodeTypeEnd},
		},
		Edges: []Edge{
			{ID: "start-approve", Source: "start", Target: "approve", Kind: EdgeKindNext},
			{ID: "approve-gate", Source: "approve", Target: "gate", Kind: EdgeKindNext},
			{ID: "gate-child", Source: "gate", Target: "child", Kind: EdgeKindCondition, Condition: "approved"},
			{ID: "child-end", Source: "child", Target: "end", Kind: EdgeKindSuccess},
		},
	}
	raw, err := CompileGraphToYAML(g)
	if err != nil {
		t.Fatalf("compile advanced graph: %v", err)
	}
	wf, err := workflow.Load(raw)
	if err != nil {
		t.Fatalf("load advanced YAML: %v", err)
	}
	if got := []string{wf.Steps[0].Name, wf.Steps[1].Name, wf.Steps[2].Name}; got[0] != "approve" || got[1] != "gate" || got[2] != "child" {
		t.Fatalf("compiled advanced step order = %v", got)
	}
	if len(wf.Handlers) != 1 || wf.Handlers[0].Name != "notify" {
		t.Fatalf("compiled handlers = %+v", wf.Handlers)
	}
}

func TestOverlayRunStateMapsByStepNameWithoutMutatingInput(t *testing.T) {
	g, err := ParseYAMLToGraph([]byte(sequentialYAML))
	if err != nil {
		t.Fatalf("parse graph: %v", err)
	}
	started := time.Date(2026, 5, 3, 10, 0, 0, 0, time.UTC)
	finished := started.Add(time.Minute)
	overlaid := OverlayRunState(g, state.RunState{
		RunID:  "run-1",
		Status: state.RunStatusRunning,
		Steps: []state.StepState{{
			Name:       "deploy",
			Status:     state.RunStatusSuccess,
			Message:    "done",
			StartedAt:  started,
			FinishedAt: finished,
			Hosts: map[string]state.HostResult{
				"local": {Host: "local", Status: state.RunStatusSuccess, Output: map[string]any{"changed": true}},
			},
		}},
	})
	if g.Nodes[2].State != nil {
		t.Fatal("OverlayRunState mutated the input graph")
	}
	if overlaid.Nodes[2].State == nil {
		t.Fatalf("deploy node missing state: %+v", overlaid.Nodes[2])
	}
	if overlaid.Nodes[2].State.RunID != "run-1" || overlaid.Nodes[2].State.Status != state.RunStatusSuccess {
		t.Fatalf("unexpected node state: %+v", overlaid.Nodes[2].State)
	}
	overlaid.Nodes[2].State.Hosts["local"] = state.HostResult{Host: "local", Status: state.RunStatusFailed}
	if g.Nodes[2].State != nil {
		t.Fatal("mutating overlay state affected original graph")
	}
}

func TestGraphModelRoundTripPreservesVisualFields(t *testing.T) {
	g := Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        "round-trip",
			Description: "graph model",
			Vars:        map[string]any{"deploy": true},
			Plan:        workflow.Plan{Mode: "auto", Strategy: "graph"},
		},
		Layout: Layout{
			Direction: "LR",
			Viewport:  Viewport{X: 11, Y: 22, Zoom: 0.9},
			UI:        map[string]any{"lane_gap": 48},
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Position: Position{X: 0, Y: 120}, Ports: []Port{{ID: "out", Type: "source", Label: "next"}}, UI: map[string]any{"tone": "neutral"}},
			{ID: "approve", Type: NodeTypeManualApproval, Position: Position{X: 240, Y: 120}, Step: &workflow.Step{Name: "approve", Action: "manual.approval"}, Approval: &ApprovalSpec{Subjects: []string{"ops"}, Timeout: "30m", OnTimeout: "reject"}},
			{ID: "join", Type: NodeTypeJoin, Position: Position{X: 480, Y: 120}, Join: &JoinSpec{Strategy: "failure_threshold", FailureThreshold: 1}},
			{ID: "child", Type: NodeTypeSubflow, Position: Position{X: 720, Y: 120}, Step: &workflow.Step{Name: "child", Action: "workflow.run"}, Subflow: &SubflowSpec{WorkflowName: "child-flow", Vars: map[string]any{"region": "hz"}}},
			{ID: "end", Type: NodeTypeEnd, Position: Position{X: 960, Y: 120}},
		},
		Edges: []Edge{
			{ID: "start-approve", Source: "start", SourcePort: "out", Target: "approve", Kind: EdgeKindNext},
			{ID: "approve-join", Source: "approve", Target: "join", Kind: EdgeKindApprovalApproved},
			{ID: "join-child", Source: "join", Target: "child", Kind: EdgeKindSuccess},
			{ID: "child-end", Source: "child", Target: "end", Kind: EdgeKindSuccess, UI: map[string]any{"label": "done"}},
		},
		UI: map[string]any{"resource": "draft"},
	}

	rawJSON, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	var fromJSON Graph
	if err := json.Unmarshal(rawJSON, &fromJSON); err != nil {
		t.Fatalf("unmarshal json: %v", err)
	}
	assertGraphVisualFields(t, fromJSON)

	rawYAML, err := yaml.Marshal(g)
	if err != nil {
		t.Fatalf("marshal yaml: %v", err)
	}
	var fromYAML Graph
	if err := yaml.Unmarshal(rawYAML, &fromYAML); err != nil {
		t.Fatalf("unmarshal yaml: %v", err)
	}
	assertGraphVisualFields(t, fromYAML)
}

func TestCompileParseRoundTripPreservesDifyStyleBranchPortsAndConditionData(t *testing.T) {
	g := Graph{
		Version: GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "condition-round-trip",
			Plan:    workflow.Plan{Mode: "auto", Strategy: "graph"},
			Vars:    map[string]any{"disk_free": true},
		},
		Layout: Layout{
			Direction: "LR",
			Viewport:  Viewport{X: 12, Y: 24, Zoom: 0.8},
		},
		Nodes: []Node{
			{ID: "start", Type: NodeTypeStart, Position: Position{X: 0, Y: 120}, Ports: []Port{{ID: "next", Type: "output", Label: "下一步"}}},
			{
				ID:       "gate",
				Type:     NodeTypeCondition,
				Position: Position{X: 260, Y: 120},
				Ports: []Port{
					{ID: "in", Type: "input", Label: "输入"},
					{ID: "if", Type: "output", Label: "IF"},
					{ID: "else", Type: "output", Label: "ELSE"},
				},
				Step:      &workflow.Step{Name: "gate", Action: "condition.evaluate", Args: map[string]any{"expression": "vars.disk_free == true"}},
				Condition: &ConditionSpec{If: "vars.disk_free == true", Elif: []ConditionBranchSpec{{Expression: "vars.force == true"}}, Else: true},
			},
			{
				ID:       "approve",
				Type:     NodeTypeManualApproval,
				Position: Position{X: 560, Y: 40},
				Ports: []Port{
					{ID: "in", Type: "input", Label: "输入"},
					{ID: "approved", Type: "output", Label: "通过"},
					{ID: "rejected", Type: "output", Label: "拒绝"},
				},
				Step:     &workflow.Step{Name: "approve", Action: "manual.approval"},
				Approval: &ApprovalSpec{Subjects: []string{"ops"}, Timeout: "30m", OnTimeout: "reject"},
			},
			{
				ID:       "notify",
				Type:     NodeTypeAction,
				Position: Position{X: 560, Y: 220},
				Ports:    []Port{{ID: "in", Type: "input", Label: "输入"}, {ID: "next", Type: "output", Label: "下一步"}},
				Step:     &workflow.Step{Name: "notify", Action: "shell.run", Args: map[string]any{"script": "echo notify"}},
			},
			{ID: "end", Type: NodeTypeEnd, Position: Position{X: 860, Y: 120}},
		},
		Edges: []Edge{
			{ID: "start-gate", Source: "start", SourcePort: "next", Target: "gate", TargetPort: "in", Kind: EdgeKindNext},
			{ID: "gate-approve-if", Source: "gate", SourcePort: "if", Target: "approve", TargetPort: "in", Kind: EdgeKindIf},
			{ID: "gate-notify-else", Source: "gate", SourcePort: "else", Target: "notify", TargetPort: "in", Kind: EdgeKindElse},
			{ID: "approve-end", Source: "approve", SourcePort: "approved", Target: "end", Kind: EdgeKindApprovalApproved},
			{ID: "notify-end", Source: "notify", Target: "end", Kind: EdgeKindSuccess},
		},
	}

	raw, err := CompileGraphToYAML(g)
	if err != nil {
		t.Fatalf("compile graph: %v", err)
	}
	roundTrip, err := ParseYAMLToGraph(raw)
	if err != nil {
		t.Fatalf("parse compiled graph: %v\n%s", err, string(raw))
	}

	gate := findNodeByID(t, roundTrip, "gate")
	if gate.Condition == nil || gate.Condition.If != "vars.disk_free == true" || len(gate.Condition.Elif) != 1 || !gate.Condition.Else {
		t.Fatalf("condition data not preserved: %+v", gate.Condition)
	}
	if got := gate.Ports; len(got) != 3 || got[1].ID != "if" || got[2].ID != "else" {
		t.Fatalf("condition ports not preserved: %+v", got)
	}
	approve := findNodeByID(t, roundTrip, "approve")
	if approve.Approval == nil || approve.Approval.Subjects[0] != "ops" || approve.Position.X != 560 {
		t.Fatalf("approval or position not preserved: %+v", approve)
	}
	if edge := findEdgeByID(t, roundTrip, "gate-approve-if"); edge.Kind != EdgeKindIf || edge.SourcePort != "if" || edge.TargetPort != "in" {
		t.Fatalf("if edge not preserved: %+v", edge)
	}
	if edge := findEdgeByID(t, roundTrip, "gate-notify-else"); edge.Kind != EdgeKindElse || edge.SourcePort != "else" {
		t.Fatalf("else edge not preserved: %+v", edge)
	}
}

func TestValidateGraphRejectsProductionInvalidGraphs(t *testing.T) {
	step := func(name string) *workflow.Step {
		return &workflow.Step{Name: name, Action: "shell.run", Args: map[string]any{"script": "echo ok"}}
	}
	cases := []struct {
		name  string
		graph Graph
		codes []string
	}{
		{
			name:  "empty graph",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "empty"}},
			codes: []string{"nodes_required", "start_required", "executable_node_required"},
		},
		{
			name: "duplicate node id",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "dup"}, Nodes: []Node{
				{ID: "start", Type: NodeTypeStart},
				{ID: "run", Type: NodeTypeAction, Step: step("run")},
				{ID: "run", Type: NodeTypeAction, Step: step("run-2")},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{
				{ID: "start-run", Source: "start", Target: "run", Kind: EdgeKindNext},
				{ID: "run-end", Source: "run", Target: "end", Kind: EdgeKindNext},
			}},
			codes: []string{"node_id_duplicate"},
		},
		{
			name: "missing start",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "no-start"}, Nodes: []Node{
				{ID: "run", Type: NodeTypeAction, Step: step("run")},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{{ID: "run-end", Source: "run", Target: "end", Kind: EdgeKindNext}}},
			codes: []string{"start_required"},
		},
		{
			name: "multiple starts",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "multi-start"}, Nodes: []Node{
				{ID: "start-a", Type: NodeTypeStart},
				{ID: "start-b", Type: NodeTypeStart},
				{ID: "run", Type: NodeTypeAction, Step: step("run")},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{
				{ID: "start-run", Source: "start-a", Target: "run", Kind: EdgeKindNext},
				{ID: "run-end", Source: "run", Target: "end", Kind: EdgeKindNext},
			}},
			codes: []string{"start_duplicate"},
		},
		{
			name: "orphan executable",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "orphan"}, Nodes: []Node{
				{ID: "start", Type: NodeTypeStart},
				{ID: "run", Type: NodeTypeAction, Step: step("run")},
				{ID: "orphan", Type: NodeTypeAction, Step: step("orphan")},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{
				{ID: "start-run", Source: "start", Target: "run", Kind: EdgeKindNext},
				{ID: "run-end", Source: "run", Target: "end", Kind: EdgeKindNext},
				{ID: "orphan-end", Source: "orphan", Target: "end", Kind: EdgeKindNext},
			}},
			codes: []string{"node_unreachable"},
		},
		{
			name: "illegal join",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "bad-join"}, Nodes: []Node{
				{ID: "start", Type: NodeTypeStart},
				{ID: "a", Type: NodeTypeAction, Step: step("a")},
				{ID: "b", Type: NodeTypeAction, Step: step("b")},
				{ID: "join", Type: NodeTypeJoin, Join: &JoinSpec{Strategy: "unsupported"}},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{
				{ID: "start-a", Source: "start", Target: "a", Kind: EdgeKindNext},
				{ID: "start-b", Source: "start", Target: "b", Kind: EdgeKindNext},
				{ID: "a-join", Source: "a", Target: "join", Kind: EdgeKindSuccess},
				{ID: "b-join", Source: "b", Target: "join", Kind: EdgeKindSuccess},
				{ID: "join-end", Source: "join", Target: "end", Kind: EdgeKindSuccess},
			}},
			codes: []string{"join_strategy_invalid"},
		},
		{
			name: "illegal approval",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "bad-approval"}, Nodes: []Node{
				{ID: "start", Type: NodeTypeStart},
				{ID: "approve", Type: NodeTypeManualApproval, Step: &workflow.Step{Name: "approve", Action: "manual.approval"}},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{
				{ID: "start-approve", Source: "start", Target: "approve", Kind: EdgeKindNext},
				{ID: "approve-end", Source: "approve", Target: "end", Kind: EdgeKindNext},
			}},
			codes: []string{"approval_required"},
		},
		{
			name: "illegal subflow",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "bad-subflow"}, Nodes: []Node{
				{ID: "start", Type: NodeTypeStart},
				{ID: "child", Type: NodeTypeSubflow, Step: &workflow.Step{Name: "child", Action: "workflow.run"}},
				{ID: "end", Type: NodeTypeEnd},
			}, Edges: []Edge{
				{ID: "start-child", Source: "start", Target: "child", Kind: EdgeKindNext},
				{ID: "child-end", Source: "child", Target: "end", Kind: EdgeKindNext},
			}},
			codes: []string{"subflow_workflow_required"},
		},
		{
			name: "missing continuation",
			graph: Graph{Version: GraphVersion, Workflow: workflow.Workflow{Version: "v0.1", Name: "missing-outgoing"}, Nodes: []Node{
				{ID: "start", Type: NodeTypeStart},
				{ID: "run", Type: NodeTypeAction, Step: step("run")},
			}, Edges: []Edge{{ID: "start-run", Source: "start", Target: "run", Kind: EdgeKindNext}}},
			codes: []string{"node_outgoing_required"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateGraph(tc.graph)
			if err == nil {
				t.Fatal("expected validation error")
			}
			verr, ok := err.(*ValidationError)
			if !ok {
				t.Fatalf("expected ValidationError, got %T", err)
			}
			for _, code := range tc.codes {
				assertVisualIssue(t, verr.Issues, code)
			}
			for _, item := range verr.Issues {
				if item.Field == "" {
					t.Fatalf("issue %q missing field locator: %+v", item.Code, item)
				}
				if item.Suggestion == "" {
					t.Fatalf("issue %q missing suggestion: %+v", item.Code, item)
				}
			}
		})
	}
}

func TestExamplesYAMLParseAndRoundTripAsGraph(t *testing.T) {
	examplesDir := filepath.Join("..", "..", "examples")
	var files []string
	err := filepath.WalkDir(examplesDir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.HasSuffix(path, ".yaml") || strings.HasSuffix(path, ".yml") {
			files = append(files, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk examples: %v", err)
	}
	sort.Strings(files)
	if len(files) == 0 {
		t.Fatalf("no example YAML files found under %s", examplesDir)
	}

	for _, path := range files {
		t.Run(filepath.ToSlash(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read example: %v", err)
			}
			g, err := ParseYAMLToGraph(raw)
			if err != nil {
				t.Fatalf("parse example graph: %v", err)
			}
			if !containsNodeType(g.Nodes, NodeTypeEnd) {
				t.Fatalf("parsed graph missing terminal end node: %+v", g.Nodes)
			}
			compiled, err := CompileGraphToYAML(g)
			if err != nil {
				t.Fatalf("compile example graph: %v", err)
			}
			roundTrip, err := ParseYAMLToGraph(compiled)
			if err != nil {
				t.Fatalf("parse compiled graph: %v", err)
			}
			if got, want := stepSignatures(roundTrip.Workflow.Steps), stepSignatures(g.Workflow.Steps); !reflect.DeepEqual(got, want) {
				t.Fatalf("step semantics changed\ngot:  %v\nwant: %v", got, want)
			}
			if got, want := len(roundTrip.Nodes), len(g.Nodes); got != want {
				t.Fatalf("node count after round-trip = %d, want %d", got, want)
			}
			if got, want := len(roundTrip.Edges), len(g.Edges); got != want {
				t.Fatalf("edge count after round-trip = %d, want %d", got, want)
			}
			if got, want := roundTrip.Layout.Direction, g.Layout.Direction; got != want {
				t.Fatalf("layout direction after round-trip = %q, want %q", got, want)
			}
		})
	}
}

func assertVisualIssue(t *testing.T, issues []Issue, code string) {
	t.Helper()
	for _, issue := range issues {
		if issue.Code == code {
			return
		}
	}
	t.Fatalf("expected issue code %q, got %+v", code, issues)
}

func assertGraphVisualFields(t *testing.T, g Graph) {
	t.Helper()
	if g.Layout.Viewport.Zoom != 0.9 || numericValue(g.Layout.UI["lane_gap"]) != 48 {
		t.Fatalf("layout fields not preserved: %+v", g.Layout)
	}
	if got := g.Nodes[0].Ports[0].ID; got != "out" {
		t.Fatalf("port not preserved: %+v", g.Nodes[0].Ports)
	}
	if got := g.Nodes[1].Approval.Subjects[0]; got != "ops" {
		t.Fatalf("approval not preserved: %+v", g.Nodes[1].Approval)
	}
	if got := g.Nodes[2].Join.FailureThreshold; got != 1 {
		t.Fatalf("join not preserved: %+v", g.Nodes[2].Join)
	}
	if got := g.Nodes[3].Subflow.WorkflowName; got != "child-flow" {
		t.Fatalf("subflow not preserved: %+v", g.Nodes[3].Subflow)
	}
	if got := g.Edges[3].UI["label"]; got != "done" {
		t.Fatalf("edge ui not preserved: %+v", g.Edges[3].UI)
	}
	if got := g.UI["resource"]; got != "draft" {
		t.Fatalf("graph ui not preserved: %+v", g.UI)
	}
}

func findEdgeByID(t *testing.T, g Graph, id string) Edge {
	t.Helper()
	for _, edge := range g.Edges {
		if edge.ID == id {
			return edge
		}
	}
	t.Fatalf("edge %q not found in %+v", id, g.Edges)
	return Edge{}
}

func findNodeByStepName(t *testing.T, g Graph, name string) Node {
	t.Helper()
	for _, node := range g.Nodes {
		if node.Step != nil && node.Step.Name == name {
			return node
		}
	}
	t.Fatalf("step node %q not found in %+v", name, g.Nodes)
	return Node{}
}

func findNodeByHandlerName(t *testing.T, g Graph, name string) Node {
	t.Helper()
	for _, node := range g.Nodes {
		if node.Handler != nil && node.Handler.Name == name {
			return node
		}
	}
	t.Fatalf("handler node %q not found in %+v", name, g.Nodes)
	return Node{}
}

func hasEdge(g Graph, source, target string, kind EdgeKind) bool {
	for _, edge := range g.Edges {
		if edge.Source == source && edge.Target == target && edge.Kind == kind {
			return true
		}
	}
	return false
}

func numericValue(value any) float64 {
	switch typed := value.(type) {
	case int:
		return float64(typed)
	case int64:
		return float64(typed)
	case float64:
		return typed
	default:
		return 0
	}
}

func stepSignatures(steps []workflow.Step) []string {
	out := make([]string, 0, len(steps))
	for _, step := range steps {
		out = append(out, step.Name+"\x00"+step.Action+"\x00"+strings.Join(step.Targets, ","))
	}
	return out
}
