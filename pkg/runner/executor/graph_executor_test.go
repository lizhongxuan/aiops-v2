package executor

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"runner/state"
	"runner/workflow"
)

func TestGraphExecutorParallelJoin(t *testing.T) {
	runner := &fakeRunner{}
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "a", Name: "a", Action: "cmd.run", Targets: []string{"local"}},
			{ID: "b", Name: "b", Action: "cmd.run", Targets: []string{"local"}},
			{ID: "finish", Name: "finish", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "fork", Type: "parallel"},
			{ID: "a", Type: "action", StepID: "a", Step: "a"},
			{ID: "b", Type: "action", StepID: "b", Step: "b"},
			{ID: "join", Type: "join", Data: workflow.GraphNodeDataSpec{Join: &workflow.GraphJoinSpec{Strategy: "all_success"}}},
			{ID: "finish", Type: "action", StepID: "finish", Step: "finish"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-fork", Source: "start", Target: "fork", Kind: "next"},
			{ID: "fork-a", Source: "fork", Target: "a", Kind: "next"},
			{ID: "fork-b", Source: "fork", Target: "b", Kind: "next"},
			{ID: "a-join", Source: "a", Target: "join", Kind: "success"},
			{ID: "b-join", Source: "b", Target: "join", Kind: "success"},
			{ID: "join-finish", Source: "join", Target: "finish", Kind: "success"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("graph run failed: %v", err)
	}
	calls := append([]string{}, runner.calls...)
	sort.Strings(calls[:2])
	if len(calls) != 3 || calls[0] != "a" || calls[1] != "b" || runner.calls[2] != "finish" {
		t.Fatalf("unexpected graph calls: %v", runner.calls)
	}
}

func TestGraphExecutorConditionEdgeUsesExportedVars(t *testing.T) {
	runner := &captureRunner{
		outputByStep: map[string]map[string]any{
			"pre": {"vars": map[string]any{"deploy": true}},
		},
	}
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "pre", Name: "pre", Action: "cmd.run", Targets: []string{"local"}},
			{ID: "deploy", Name: "deploy", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "pre", Type: "action", StepID: "pre", Step: "pre"},
			{ID: "deploy", Type: "action", StepID: "deploy", Step: "deploy"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-pre", Source: "start", Target: "pre", Kind: "next"},
			{ID: "pre-deploy", Source: "pre", Target: "deploy", Kind: "condition", Condition: "deploy == true"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("graph run failed: %v", err)
	}
	if _, ok := runner.varsByStep["deploy"]; !ok {
		t.Fatalf("deploy should run after exported condition, calls=%v", runner.varsByStep)
	}
}

func TestGraphExecutorFailureEdgeHandlesFailedBranch(t *testing.T) {
	runner := &fakeRunner{failOn: "restore"}
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "restore", Name: "restore", Action: "cmd.run", Targets: []string{"local"}},
			{ID: "notify", Name: "notify", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "restore", Type: "action", StepID: "restore", Step: "restore"},
			{ID: "notify", Type: "action", StepID: "notify", Step: "notify"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-restore", Source: "start", Target: "restore", Kind: "next"},
			{ID: "restore-notify", Source: "restore", Target: "notify", Kind: "failure"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("failure edge should handle branch without failing graph: %v", err)
	}
	if len(runner.calls) != 2 || runner.calls[1] != "notify" {
		t.Fatalf("notify should run after failure edge, calls=%v", runner.calls)
	}
}

func TestGraphExecutorContinueOnErrorFallsThrough(t *testing.T) {
	runner := &fakeRunner{failOn: "check"}
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "check", Name: "check", Action: "cmd.run", Targets: []string{"local"}, ContinueOnError: true},
			{ID: "cleanup", Name: "cleanup", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "check", Type: "action", StepID: "check", Step: "check"},
			{ID: "cleanup", Type: "action", StepID: "cleanup", Step: "cleanup"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-check", Source: "start", Target: "check", Kind: "next"},
			{ID: "check-cleanup", Source: "check", Target: "cleanup", Kind: "next"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("continue_on_error should allow graph to continue: %v", err)
	}
	if len(runner.calls) != 2 || runner.calls[1] != "cleanup" {
		t.Fatalf("cleanup should run after continue_on_error failure, calls=%v", runner.calls)
	}
}

func TestGraphExecutorManualApprovalWaitsAndContinues(t *testing.T) {
	runner := &fakeRunner{}
	approvals := newFakeApprovalRuntime()
	exec := &GraphExecutor{Runner: runner, Approvals: approvals}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "after", Name: "after", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "approve", Type: "manual_approval", Data: workflow.GraphNodeDataSpec{Approval: &workflow.GraphApprovalSpec{Subjects: []string{"sre"}, Timeout: "30m"}}},
			{ID: "after", Type: "action", StepID: "after", Step: "after"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-approve", Source: "start", Target: "approve", Kind: "next"},
			{ID: "approve-after", Source: "approve", Target: "after", Kind: "approval_approved"},
		},
	)

	done := make(chan error, 1)
	go func() {
		done <- exec.Run(context.Background(), wf)
	}()
	if nodeID := approvals.waitNode(t); nodeID != "approve" {
		t.Fatalf("approval node = %q, want approve", nodeID)
	}
	select {
	case err := <-done:
		t.Fatalf("graph completed before approval: %v", err)
	default:
	}
	approvals.approve("sre", "looks good")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("graph run failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("graph did not resume after approval")
	}
	if len(runner.calls) != 1 || runner.calls[0] != "after" {
		t.Fatalf("approved path should run after node, calls=%v", runner.calls)
	}
}

func TestGraphExecutorManualApprovalRejectsFailureEdge(t *testing.T) {
	runner := &fakeRunner{}
	approvals := newFakeApprovalRuntime()
	exec := &GraphExecutor{Runner: runner, Approvals: approvals}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "notify", Name: "notify", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "approve", Type: "manual_approval", Data: workflow.GraphNodeDataSpec{Approval: &workflow.GraphApprovalSpec{Subjects: []string{"sre"}, Timeout: "30m"}}},
			{ID: "notify", Type: "action", StepID: "notify", Step: "notify"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-approve", Source: "start", Target: "approve", Kind: "next"},
			{ID: "approve-notify", Source: "approve", Target: "notify", Kind: "approval_rejected"},
		},
	)

	done := make(chan error, 1)
	go func() {
		done <- exec.Run(context.Background(), wf)
	}()
	approvals.waitNode(t)
	approvals.reject("sre", "blocked")

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("rejection edge should handle approval rejection: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("graph did not resume after rejection")
	}
	if len(runner.calls) != 1 || runner.calls[0] != "notify" {
		t.Fatalf("rejected path should run notify node, calls=%v", runner.calls)
	}
}

func TestGraphExecutorSubflowRunsChildWorkflowAndContinues(t *testing.T) {
	runner := &captureRunner{}
	subflows := &fakeSubflowRuntime{
		workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "child-flow",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
			Steps: []workflow.Step{
				{ID: "child-step", Name: "child-step", Action: "cmd.run", Targets: []string{"local"}},
			},
		},
	}
	exec := &GraphExecutor{Runner: runner, Subflows: subflows}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "after", Name: "after", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "child", Type: "subflow", Data: workflow.GraphNodeDataSpec{Subflow: &workflow.GraphSubflowSpec{WorkflowName: "child-flow", Vars: map[string]any{"region": "hz"}}}},
			{ID: "after", Type: "action", StepID: "after", Step: "after"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-child", Source: "start", Target: "child", Kind: "next"},
			{ID: "child-after", Source: "child", Target: "after", Kind: "success"},
		},
	)
	wf.Vars = map[string]any{"parent": "p"}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("graph run failed: %v", err)
	}
	if subflows.request.WorkflowName != "child-flow" || subflows.request.Vars["region"] != "hz" || subflows.request.Vars["parent"] != "p" {
		t.Fatalf("subflow request mismatch: %+v", subflows.request)
	}
	if _, ok := runner.varsByStep["child-step"]; !ok {
		t.Fatalf("child workflow step should run, vars=%v", runner.varsByStep)
	}
	if _, ok := runner.varsByStep["after"]; !ok {
		t.Fatalf("parent continuation should run, vars=%v", runner.varsByStep)
	}
}

func TestGraphExecutorSubflowFailureSelectsFailureEdge(t *testing.T) {
	runner := &fakeRunner{failOn: "child-step"}
	subflows := &fakeSubflowRuntime{
		workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "child-flow",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
			Steps: []workflow.Step{
				{ID: "child-step", Name: "child-step", Action: "cmd.run", Targets: []string{"local"}},
			},
		},
	}
	exec := &GraphExecutor{Runner: runner, Subflows: subflows}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "notify", Name: "notify", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "child", Type: "subflow", Data: workflow.GraphNodeDataSpec{Subflow: &workflow.GraphSubflowSpec{WorkflowName: "child-flow"}}},
			{ID: "notify", Type: "action", StepID: "notify", Step: "notify"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-child", Source: "start", Target: "child", Kind: "next"},
			{ID: "child-notify", Source: "child", Target: "notify", Kind: "failure"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("failure edge should handle subflow failure: %v", err)
	}
	if len(runner.calls) != 2 || runner.calls[0] != "child-step" || runner.calls[1] != "notify" {
		t.Fatalf("unexpected calls after subflow failure: %v", runner.calls)
	}
}

func TestGraphExecutorCancelStopsRunningNode(t *testing.T) {
	runner := newCancelAwareRunner()
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "slow", Name: "slow", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "slow", Type: "action", StepID: "slow", Step: "slow"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-slow", Source: "start", Target: "slow", Kind: "next"},
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- exec.Run(ctx, wf)
	}()
	runner.waitStarted(t)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("graph cancel error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("graph did not stop after cancellation")
	}
}

func TestGraphExecutorCancelStopsLoopBeforeNextIteration(t *testing.T) {
	runner := newCancelAwareRunner()
	observer := &iterationObserver{}
	exec := &GraphExecutor{Runner: runner, Observer: observer}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "body", Name: "body", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "loop", Type: "loop", Data: workflow.GraphNodeDataSpec{Loop: &workflow.GraphLoopSpec{
				Mode:          "for_each",
				Items:         []any{"a", "b", "c"},
				MaxIterations: 3,
			}}},
			{ID: "body", Type: "action", ParentID: "loop", StepID: "body", Step: "body"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-loop", Source: "start", Target: "loop", Kind: "next"},
			{ID: "loop-body", Source: "loop", Target: "body", Kind: "next"},
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- exec.Run(ctx, wf)
	}()
	runner.waitStarted(t)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("graph loop cancel error = %v, want context.Canceled", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("graph loop did not stop after cancellation")
	}
	if got := len(observer.iterations); got != 1 {
		t.Fatalf("loop iterations after cancellation = %d, want 1: %+v", got, observer.iterations)
	}
}

func TestGraphExecutorTimeoutCanUseFailureEdge(t *testing.T) {
	runner := &timeoutRunner{}
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "slow", Name: "slow", Action: "cmd.run", Targets: []string{"local"}, Timeout: "10ms"},
			{ID: "notify", Name: "notify", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "slow", Type: "action", StepID: "slow", Step: "slow"},
			{ID: "notify", Type: "action", StepID: "notify", Step: "notify"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-slow", Source: "start", Target: "slow", Kind: "next"},
			{ID: "slow-notify", Source: "slow", Target: "notify", Kind: "failure"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("failure edge should handle timed out node: %v", err)
	}
	if len(runner.calls) != 2 || runner.calls[0] != "slow" || runner.calls[1] != "notify" {
		t.Fatalf("unexpected timeout calls: %v", runner.calls)
	}
}

func TestGraphExecutorLoopForEachRecordsIterations(t *testing.T) {
	runner := &captureRunner{}
	observer := &iterationObserver{}
	exec := &GraphExecutor{Runner: runner, Observer: observer}
	wf := graphWorkflow(
		[]workflow.Step{
			{ID: "body", Name: "body", Action: "cmd.run", Targets: []string{"local"}},
			{ID: "after", Name: "after", Action: "cmd.run", Targets: []string{"local"}},
		},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "loop", Type: "loop", Data: workflow.GraphNodeDataSpec{Loop: &workflow.GraphLoopSpec{
				Mode:          "for_each",
				Items:         []any{"a", "b"},
				MaxIterations: 3,
				ItemVar:       "batch",
				IndexVar:      "batch_index",
			}}},
			{ID: "body", Type: "action", ParentID: "loop", StepID: "body", Step: "body"},
			{ID: "after", Type: "action", StepID: "after", Step: "after"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-loop", Source: "start", Target: "loop", Kind: "next"},
			{ID: "loop-body", Source: "loop", Target: "body", Kind: "next"},
			{ID: "loop-after", Source: "loop", Target: "after", Kind: "success"},
		},
	)

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("graph loop run failed: %v", err)
	}
	if got := len(observer.iterations); got != 2 {
		t.Fatalf("recorded iterations = %d, want 2: %+v", got, observer.iterations)
	}
	if runner.varsByStep["body"]["batch"] != "b" || runner.varsByStep["body"]["batch_index"] != 1 {
		t.Fatalf("body should receive loop item/index vars, got %+v", runner.varsByStep["body"])
	}
	if _, ok := runner.varsByStep["after"]; !ok {
		t.Fatalf("after node should run after loop, vars=%v", runner.varsByStep)
	}
}

func TestGraphExecutorLoopWhileConditionStopsAtMaxIterations(t *testing.T) {
	runner := &captureRunner{
		outputByStep: map[string]map[string]any{
			"body": {"vars": map[string]any{"ready": true}},
		},
	}
	observer := &iterationObserver{}
	exec := &GraphExecutor{Runner: runner, Observer: observer}
	wf := graphWorkflow(
		[]workflow.Step{{ID: "body", Name: "body", Action: "cmd.run", Targets: []string{"local"}}},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "loop", Type: "loop", Data: workflow.GraphNodeDataSpec{Loop: &workflow.GraphLoopSpec{
				Mode:           "while_condition",
				WhileCondition: "ready == true",
				MaxIterations:  2,
			}}},
			{ID: "body", Type: "action", ParentID: "loop", StepID: "body", Step: "body"},
			{ID: "end", Type: "end"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-loop", Source: "start", Target: "loop", Kind: "next"},
			{ID: "loop-body", Source: "loop", Target: "body", Kind: "next"},
			{ID: "loop-end", Source: "loop", Target: "end", Kind: "success"},
		},
	)
	wf.Vars = map[string]any{"ready": true}

	if err := exec.Run(context.Background(), wf); err != nil {
		t.Fatalf("graph while loop run failed: %v", err)
	}
	if got := len(observer.iterations); got != 2 {
		t.Fatalf("recorded iterations = %d, want max 2: %+v", got, observer.iterations)
	}
}

func TestGraphExecutorRejectsLegacyStepLoopInsideGraphLoop(t *testing.T) {
	runner := &captureRunner{}
	exec := &GraphExecutor{Runner: runner}
	wf := graphWorkflow(
		[]workflow.Step{{ID: "body", Name: "body", Action: "cmd.run", Targets: []string{"local"}, Loop: []any{"legacy"}}},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "loop", Type: "loop", Data: workflow.GraphNodeDataSpec{Loop: &workflow.GraphLoopSpec{
				Mode:          "for_each",
				Items:         []any{"a"},
				MaxIterations: 1,
			}}},
			{ID: "body", Type: "action", ParentID: "loop", StepID: "body", Step: "body"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-loop", Source: "start", Target: "loop", Kind: "next"},
			{ID: "loop-body", Source: "loop", Target: "body", Kind: "next"},
		},
	)

	err := exec.Run(context.Background(), wf)
	if err == nil || !strings.Contains(err.Error(), "legacy step.loop") {
		t.Fatalf("expected legacy step.loop rejection, got %v", err)
	}
}

func TestGraphExecutorAllowsLegacyStepLoopInsideNonLoopGroup(t *testing.T) {
	wf := graphWorkflow(
		[]workflow.Step{{ID: "body", Name: "body", Action: "cmd.run", Targets: []string{"local"}, Loop: []any{"legacy"}}},
		[]workflow.GraphNodeSpec{
			{ID: "start", Type: "start"},
			{ID: "group", Type: "group"},
			{ID: "body", Type: "action", ParentID: "group", StepID: "body", Step: "body"},
		},
		[]workflow.GraphEdgeSpec{
			{ID: "start-body", Source: "start", Target: "body", Kind: "next"},
		},
	)

	if _, err := newExecutionGraph(wf); err != nil {
		t.Fatalf("legacy step.loop should only be rejected inside graph loop bodies, got %v", err)
	}
}

func graphWorkflow(steps []workflow.Step, nodes []workflow.GraphNodeSpec, edges []workflow.GraphEdgeSpec) workflow.Workflow {
	return workflow.Workflow{
		Version: "v0.1",
		Name:    "graph-test",
		Plan:    workflow.Plan{Mode: "auto", Strategy: "graph"},
		Inventory: workflow.Inventory{
			Hosts: map[string]workflow.Host{"local": {Address: "local"}},
		},
		Steps: steps,
		XRunnerGraph: &workflow.GraphSpec{
			Version: "v1",
			Nodes:   nodes,
			Edges:   edges,
		},
	}
}

type iterationObserver struct {
	iterations []int
}

func (o *iterationObserver) StepStart(workflow.Step, []workflow.HostSpec) {}

func (o *iterationObserver) StepFinish(workflow.Step, string) {}

func (o *iterationObserver) GraphNodeStart(string) {}

func (o *iterationObserver) GraphNodeFinish(string, string, string) {}

func (o *iterationObserver) GraphEdgeSelected(workflow.GraphEdgeSpec) {}

func (o *iterationObserver) GraphNodeIterationStart(nodeID string, iteration int, item any) {
	if nodeID == "loop" {
		o.iterations = append(o.iterations, iteration)
	}
}

func (o *iterationObserver) GraphNodeIterationFinish(string, int, string, string) {}

type cancelAwareRunner struct {
	started chan struct{}
	once    sync.Once
}

func newCancelAwareRunner() *cancelAwareRunner {
	return &cancelAwareRunner{started: make(chan struct{})}
}

func (r *cancelAwareRunner) Run(ctx context.Context, _ workflow.Step, _ workflow.HostSpec, _ map[string]any) (RunResult, error) {
	r.once.Do(func() {
		close(r.started)
	})
	<-ctx.Done()
	return RunResult{}, ctx.Err()
}

func (r *cancelAwareRunner) waitStarted(t *testing.T) {
	t.Helper()
	select {
	case <-r.started:
	case <-time.After(2 * time.Second):
		t.Fatal("runner did not start")
	}
}

type timeoutRunner struct {
	calls []string
}

func (r *timeoutRunner) Run(ctx context.Context, step workflow.Step, _ workflow.HostSpec, _ map[string]any) (RunResult, error) {
	r.calls = append(r.calls, step.Name)
	if step.Name == "slow" {
		<-ctx.Done()
		return RunResult{}, ctx.Err()
	}
	return RunResult{}, nil
}

type fakeSubflowRuntime struct {
	workflow workflow.Workflow
	request  SubflowRequest
}

func (r *fakeSubflowRuntime) LoadSubflow(_ context.Context, _ workflow.Workflow, _ workflow.GraphNodeSpec, request SubflowRequest) (workflow.Workflow, error) {
	r.request = request
	return r.workflow, nil
}

type fakeApprovalRuntime struct {
	waiting  chan string
	decision chan ApprovalDecision
}

func newFakeApprovalRuntime() *fakeApprovalRuntime {
	return &fakeApprovalRuntime{
		waiting:  make(chan string, 1),
		decision: make(chan ApprovalDecision, 1),
	}
}

func (r *fakeApprovalRuntime) WaitForApproval(ctx context.Context, _ workflow.Workflow, node workflow.GraphNodeSpec) (ApprovalDecision, error) {
	r.waiting <- node.ID
	select {
	case decision := <-r.decision:
		return decision, nil
	case <-ctx.Done():
		return ApprovalDecision{}, ctx.Err()
	}
}

func (r *fakeApprovalRuntime) approve(actor, comment string) {
	r.decision <- ApprovalDecision{Status: state.RunStatusSuccess, Actor: actor, Comment: comment}
}

func (r *fakeApprovalRuntime) reject(actor, comment string) {
	r.decision <- ApprovalDecision{Status: state.RunStatusFailed, Actor: actor, Comment: comment}
}

func (r *fakeApprovalRuntime) waitNode(t *testing.T) string {
	t.Helper()
	select {
	case nodeID := <-r.waiting:
		return nodeID
	case <-time.After(2 * time.Second):
		t.Fatal("approval was not requested")
		return ""
	}
}
