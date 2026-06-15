package appui

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/opssemantic"
)

func TestApprovalServiceListsHostCommandApprovals(t *testing.T) {
	ctx := context.Background()
	approvals := hostops.NewInMemoryCommandApprovalStore()
	controller := hostops.NewCommandApprovalController(hostops.CommandApprovalControllerConfig{
		Store: approvals,
	})
	approval, err := controller.RequestApproval(ctx, hostops.CommandApprovalRequest{
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	service := NewApprovalServiceWithHostCommandApprovals(ctx, nil, nil, NewSnapshotBuilder(), controller)
	views, err := service.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want 1", len(views))
	}
	view := views[0]
	if view.ID != approval.ID || view.ToolName != "host_command" || view.Command != "touch /tmp/aiops-check" || view.HostID != "host-a" || view.Source != "host_command_policy" {
		t.Fatalf("approval view = %#v, want host command metadata", view)
	}
}

func TestApprovalServiceDecidesHostCommandApprovalWhenRuntimeApprovalMissing(t *testing.T) {
	ctx := context.Background()
	executor := &hostOpsApprovalExecutor{}
	approvals := hostops.NewInMemoryCommandApprovalStore()
	controller := hostops.NewCommandApprovalController(hostops.CommandApprovalControllerConfig{
		Store:    approvals,
		Executor: executor,
	})
	approval, err := controller.RequestApproval(ctx, hostops.CommandApprovalRequest{
		ToolContext:  hostops.ToolContext{AgentKind: hostops.AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	service := NewApprovalServiceWithHostCommandApprovals(ctx, nil, nil, NewSnapshotBuilder(), controller)
	result, err := service.Decide(ctx, ApprovalDecision{ID: approval.ID, Decision: "approved"})
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if result.Status != string(hostops.CommandApprovalStatusExecuted) {
		t.Fatalf("result status = %q, want executed", result.Status)
	}
	if executor.calls != 1 || executor.lastReq.Script != "touch /tmp/aiops-check" {
		t.Fatalf("executor = %d/%#v, want approved host command execution", executor.calls, executor.lastReq)
	}
}

func TestApprovalServiceListsAndDecidesHostCommandApprovalGroup(t *testing.T) {
	ctx := context.Background()
	executor := &hostOpsApprovalExecutor{}
	approvals := hostops.NewInMemoryCommandApprovalStore()
	controller := hostops.NewCommandApprovalController(hostops.CommandApprovalControllerConfig{
		Store:    approvals,
		Executor: executor,
	})
	first, err := controller.RequestApproval(ctx, hostops.CommandApprovalRequest{
		ToolContext:  hostops.ToolContext{AgentKind: hostops.AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check-a",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval(first) error = %v", err)
	}
	_, err = controller.RequestApproval(ctx, hostops.CommandApprovalRequest{
		ToolContext:  hostops.ToolContext{AgentKind: hostops.AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check-b",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval(second) error = %v", err)
	}

	service := NewApprovalServiceWithHostCommandApprovals(ctx, nil, nil, NewSnapshotBuilder(), controller)
	views, err := service.List(ctx)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	if len(views) != 1 {
		t.Fatalf("len(views) = %d, want one grouped approval", len(views))
	}
	view := views[0]
	if view.ID != first.GroupID || view.GroupID != first.GroupID || view.GroupSize != 2 || view.Source != "host_command_group_policy" {
		t.Fatalf("group view = %#v, want group id/size/source", view)
	}
	if view.MissionID != "mission-1" || view.PlanStepID != "step-1" || view.HostID != "host-a" {
		t.Fatalf("group view scope = %#v, want mission/step/host metadata", view)
	}

	result, err := service.Decide(ctx, ApprovalDecision{ID: first.GroupID, Decision: "approved"})
	if err != nil {
		t.Fatalf("Decide(group) error = %v", err)
	}
	if result.Status != string(hostops.CommandApprovalStatusExecuted) {
		t.Fatalf("result status = %q, want executed", result.Status)
	}
	if executor.calls != 2 {
		t.Fatalf("executor calls = %d, want grouped approval to execute two commands", executor.calls)
	}
}

func TestApprovalServiceDecideAsyncHostCommandApprovalReturnsBeforeExecution(t *testing.T) {
	ctx := context.Background()
	executor := newHostOpsBlockingApprovalExecutor()
	approvals := hostops.NewInMemoryCommandApprovalStore()
	controller := hostops.NewCommandApprovalController(hostops.CommandApprovalControllerConfig{
		Store:    approvals,
		Executor: executor,
	})
	approval, err := controller.RequestApproval(ctx, hostops.CommandApprovalRequest{
		ToolContext:  hostops.ToolContext{AgentKind: hostops.AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	service := NewApprovalServiceWithHostCommandApprovals(ctx, nil, nil, NewSnapshotBuilder(), controller)
	asyncService, ok := service.(interface {
		DecideAsync(context.Context, ApprovalDecision) (ActionResult, error)
	})
	if !ok {
		t.Fatal("ApprovalService does not implement DecideAsync")
	}

	started := time.Now()
	result, err := asyncService.DecideAsync(ctx, ApprovalDecision{ID: approval.ID, Decision: "approved"})
	if err != nil {
		t.Fatalf("DecideAsync() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("DecideAsync() took %s, want immediate return before host command execution completes", elapsed)
	}
	if result.Status != string(hostops.CommandApprovalStatusApproved) {
		t.Fatalf("result status = %q, want approved", result.Status)
	}

	select {
	case req := <-executor.started:
		if req.Script != "touch /tmp/aiops-check" {
			t.Fatalf("executor request = %#v, want approved host command", req)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("host command executor was not started asynchronously")
	}
	executor.release(hostops.HostCommandResult{Status: "success", Stdout: "ok", ExitCode: 0}, nil)
}

type hostOpsApprovalExecutor struct {
	calls   int
	lastReq hostops.HostCommandRequest
}

func (e *hostOpsApprovalExecutor) RunShell(_ context.Context, _ hostops.ToolContext, req hostops.HostCommandRequest) (hostops.HostCommandResult, error) {
	e.calls++
	e.lastReq = req
	return hostops.HostCommandResult{Status: "success", Stdout: "ok", ExitCode: 0}, nil
}

type hostOpsBlockingApprovalExecutor struct {
	started chan hostops.HostCommandRequest
	done    chan hostOpsBlockingApprovalExecutorResult
}

type hostOpsBlockingApprovalExecutorResult struct {
	result hostops.HostCommandResult
	err    error
}

func newHostOpsBlockingApprovalExecutor() *hostOpsBlockingApprovalExecutor {
	return &hostOpsBlockingApprovalExecutor{
		started: make(chan hostops.HostCommandRequest, 1),
		done:    make(chan hostOpsBlockingApprovalExecutorResult, 1),
	}
}

func (e *hostOpsBlockingApprovalExecutor) RunShell(_ context.Context, _ hostops.ToolContext, req hostops.HostCommandRequest) (hostops.HostCommandResult, error) {
	e.started <- req
	next := <-e.done
	return next.result, next.err
}

func (e *hostOpsBlockingApprovalExecutor) release(result hostops.HostCommandResult, err error) {
	e.done <- hostOpsBlockingApprovalExecutorResult{result: result, err: err}
}
