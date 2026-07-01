package hostops

import (
	"context"
	"errors"
	"testing"
	"time"

	"aiops-v2/internal/opssemantic"
)

func TestHostCommandToolCreatesPendingApprovalAndBlocksHostOpsState(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	executor := &fakeHostCommandExecutor{}
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	tool := NewHostCommandToolWithApprovals(executor, NewCommandPolicy(CommandPolicyConfig{}), controller)

	result, err := tool.Run(ctx, HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.ApprovalRequired || result.ApprovalID == "" || result.Executed {
		t.Fatalf("result = %#v, want pending approval id without execution", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 before approval", executor.calls)
	}
	approval, err := approvals.Get(ctx, result.ApprovalID)
	if err != nil {
		t.Fatalf("Get approval error = %v", err)
	}
	if approval.MissionID != "mission-1" || approval.ChildAgentID != "child-a" || approval.PlanStepID != "step-1" || approval.HostID != "host-a" || approval.Command != "touch /tmp/aiops-check" {
		t.Fatalf("approval = %#v, want mission/child/step/host/command metadata", approval)
	}
	if approval.RiskLevel != opssemantic.RiskLowWrite || approval.Status != CommandApprovalStatusPending {
		t.Fatalf("approval risk/status = %q/%q, want low_write/pending", approval.RiskLevel, approval.Status)
	}
	child, err := missions.GetChildAgent(ctx, "child-a")
	if err != nil {
		t.Fatalf("GetChildAgent error = %v", err)
	}
	if child.Status != HostChildAgentStatusApprovalRequired {
		t.Fatalf("child status = %q, want approval_required", child.Status)
	}
	mission, err := missions.GetMission(ctx, "mission-1")
	if err != nil {
		t.Fatalf("GetMission error = %v", err)
	}
	if mission.Status != HostMissionStatusWaitingApproval {
		t.Fatalf("mission status = %q, want waiting_approval", mission.Status)
	}
	if len(mission.Plan.Steps) != 1 || mission.Plan.Steps[0].Status != PlanStepStatusBlocked || !mission.Plan.Steps[0].ApprovalRequired {
		t.Fatalf("plan step = %#v, want blocked approval-required step", mission.Plan.Steps)
	}
	items, err := transcripts.List(ctx, "child-a")
	if err != nil {
		t.Fatalf("transcript list error = %v", err)
	}
	if len(items) == 0 || items[len(items)-1].Type != TranscriptItemApproval || items[len(items)-1].ApprovalID != result.ApprovalID {
		t.Fatalf("transcript = %#v, want approval event with approval id", items)
	}
}

func TestHostCommandToolBlocksHighRiskWriteUntilApproval(t *testing.T) {
	ctx := context.Background()
	missions, _ := hostCommandApprovalFixture(t)
	executor := &fakeHostCommandExecutor{}
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:    approvals,
		Missions: missions,
		Executor: executor,
	})
	tool := NewHostCommandToolWithApprovals(executor, NewCommandPolicy(CommandPolicyConfig{}), controller)

	result, err := tool.Run(ctx, HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "systemctl restart postgresql",
		RiskLevel:    opssemantic.RiskHighWrite,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.ApprovalRequired || result.Executed || result.ApprovalID == "" {
		t.Fatalf("result = %#v, want blocked high-risk command approval", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0 before approval", executor.calls)
	}
	approval, err := approvals.Get(ctx, result.ApprovalID)
	if err != nil {
		t.Fatalf("Get approval error = %v", err)
	}
	if approval.RiskLevel != opssemantic.RiskHighWrite || approval.Status != CommandApprovalStatusPending {
		t.Fatalf("approval risk/status = %q/%q, want high_write/pending", approval.RiskLevel, approval.Status)
	}
	mission, err := missions.GetMission(ctx, "mission-1")
	if err != nil {
		t.Fatalf("GetMission error = %v", err)
	}
	if mission.Status != HostMissionStatusWaitingApproval || len(mission.Plan.Steps) != 1 || mission.Plan.Steps[0].Status != PlanStepStatusBlocked {
		t.Fatalf("mission = %#v, want waiting approval with blocked plan step", mission)
	}
}

func TestCommandApprovalDecisionDeniedBlocksChildAndWritesTranscript(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    &fakeHostCommandExecutor{},
	})
	approval, err := controller.RequestApproval(ctx, CommandApprovalRequest{
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

	decided, _, err := controller.Decide(ctx, approval.ID, "denied")
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decided.Status != CommandApprovalStatusDenied || decided.Decision != "denied" {
		t.Fatalf("approval = %#v, want denied decision", decided)
	}
	child, err := missions.GetChildAgent(ctx, "child-a")
	if err != nil {
		t.Fatalf("GetChildAgent error = %v", err)
	}
	if child.Status != HostChildAgentStatusBlocked || child.Error == "" {
		t.Fatalf("child = %#v, want blocked with error", child)
	}
	items, err := transcripts.List(ctx, "child-a")
	if err != nil {
		t.Fatalf("transcript list error = %v", err)
	}
	if last := items[len(items)-1]; last.Type != TranscriptItemApproval || last.Status != "denied" {
		t.Fatalf("last transcript item = %#v, want denied approval event", last)
	}
}

func TestCommandApprovalDecisionApprovedExecutesCommandAndWritesResult(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	executor := &fakeHostCommandExecutor{}
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	approval, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
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

	decided, result, err := controller.Decide(ctx, approval.ID, "approved")
	if err != nil {
		t.Fatalf("Decide() error = %v", err)
	}
	if decided.Status != CommandApprovalStatusExecuted || decided.Decision != "approved" {
		t.Fatalf("approval = %#v, want approved/executed decision", decided)
	}
	if executor.calls != 1 || executor.lastReq.Script != "touch /tmp/aiops-check" {
		t.Fatalf("executor calls/request = %d/%#v, want one approved command execution", executor.calls, executor.lastReq)
	}
	if result.Status != "success" {
		t.Fatalf("command result = %#v, want success", result)
	}
	child, err := missions.GetChildAgent(ctx, "child-a")
	if err != nil {
		t.Fatalf("GetChildAgent error = %v", err)
	}
	if child.Status != HostChildAgentStatusRunning {
		t.Fatalf("child status = %q, want running after approved execution", child.Status)
	}
	items, err := transcripts.List(ctx, "child-a")
	if err != nil {
		t.Fatalf("transcript list error = %v", err)
	}
	if len(items) < 2 || items[len(items)-1].Type != TranscriptItemToolResult || items[len(items)-1].Status != "success" {
		t.Fatalf("transcript = %#v, want tool result after approved execution", items)
	}
}

func TestCommandApprovalDecisionGroupIsScopedToSamePlanStepHostAndRisk(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	executor := &fakeHostCommandExecutor{}
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	first, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
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
	second, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
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
	crossHost, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-b"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-b",
		Command:      "touch /tmp/aiops-cross-host",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval(crossHost) error = %v", err)
	}
	crossRisk, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "systemctl restart synthetic.service",
		RiskLevel:    opssemantic.RiskHighWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval(crossRisk) error = %v", err)
	}
	if first.GroupID == "" || first.GroupID != second.GroupID {
		t.Fatalf("group ids = %q/%q, want same non-empty group for same step/host/risk", first.GroupID, second.GroupID)
	}
	if crossHost.GroupID == first.GroupID || crossRisk.GroupID == first.GroupID {
		t.Fatalf("group ids crossed host/risk: base=%q crossHost=%q crossRisk=%q", first.GroupID, crossHost.GroupID, crossRisk.GroupID)
	}

	group, results, err := controller.DecideGroup(ctx, first.GroupID, "approved")
	if err != nil {
		t.Fatalf("DecideGroup() error = %v", err)
	}
	if group.ID != first.GroupID || group.Total != 2 || group.Status != CommandApprovalStatusExecuted {
		t.Fatalf("group = %#v, want executed group with two approvals", group)
	}
	if len(results) != 2 || executor.calls != 2 {
		t.Fatalf("results/calls = %d/%d, want two approved executions", len(results), executor.calls)
	}
	unchangedHost, err := approvals.Get(ctx, crossHost.ID)
	if err != nil {
		t.Fatalf("Get(crossHost) error = %v", err)
	}
	unchangedRisk, err := approvals.Get(ctx, crossRisk.ID)
	if err != nil {
		t.Fatalf("Get(crossRisk) error = %v", err)
	}
	if unchangedHost.Status != CommandApprovalStatusPending || unchangedRisk.Status != CommandApprovalStatusPending {
		t.Fatalf("out-of-group approvals = %q/%q, want pending", unchangedHost.Status, unchangedRisk.Status)
	}
}

func TestCommandApprovalDecisionAsyncReturnsBeforeCommandExecutionCompletes(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	executor := newBlockingHostCommandExecutor()
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	approval, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
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

	started := time.Now()
	decided, err := controller.DecideAsync(ctx, approval.ID, "approved")
	if err != nil {
		t.Fatalf("DecideAsync() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("DecideAsync() took %s, want immediate return before command execution completes", elapsed)
	}
	if decided.Status != CommandApprovalStatusApproved || decided.Decision != "approved" {
		t.Fatalf("approval = %#v, want approved decision before execution", decided)
	}
	stored, err := approvals.Get(ctx, approval.ID)
	if err != nil {
		t.Fatalf("Get approval error = %v", err)
	}
	if stored.Status != CommandApprovalStatusApproved {
		t.Fatalf("stored status = %q, want approved while async execution is blocked", stored.Status)
	}

	select {
	case req := <-executor.started:
		if req.Script != "touch /tmp/aiops-check" {
			t.Fatalf("executor request = %#v, want approved command script", req)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("async command execution did not start")
	}
	executor.release(HostCommandResult{Status: "success", Stdout: "ok", ExitCode: 0}, nil)
	eventuallyHostCommandApprovalStatus(t, approvals, approval.ID, CommandApprovalStatusExecuted)
}

func TestCommandApprovalDecisionAsyncRecordsExecutionFailure(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	executor := newBlockingHostCommandExecutor()
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	approval, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-a",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "systemctl restart synthetic.service",
		RiskLevel:    opssemantic.RiskHighWrite,
	})
	if err != nil {
		t.Fatalf("RequestApproval() error = %v", err)
	}

	if _, err := controller.DecideAsync(ctx, approval.ID, "approved"); err != nil {
		t.Fatalf("DecideAsync() error = %v", err)
	}
	select {
	case <-executor.started:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("async command execution did not start")
	}
	executor.release(HostCommandResult{}, errors.New("host unreachable"))
	failed := eventuallyHostCommandApprovalStatus(t, approvals, approval.ID, CommandApprovalStatusFailed)
	if failed.Result.Status != "failed" || failed.Result.Error != "host unreachable" {
		t.Fatalf("failed result = %#v, want failed host unreachable result", failed.Result)
	}
	child, err := missions.GetChildAgent(ctx, "child-a")
	if err != nil {
		t.Fatalf("GetChildAgent error = %v", err)
	}
	if child.Status != HostChildAgentStatusFailed || child.Error != "host unreachable" {
		t.Fatalf("child = %#v, want failed with execution error", child)
	}
	items, err := transcripts.List(ctx, "child-a")
	if err != nil {
		t.Fatalf("transcript list error = %v", err)
	}
	if last := items[len(items)-1]; last.Type != TranscriptItemApproval || last.Status != "failed" || last.Content != "host unreachable" {
		t.Fatalf("last transcript item = %#v, want failed approval event", last)
	}
}

func TestCommandApprovalDecisionGroupAsyncReturnsBeforeExecutionsComplete(t *testing.T) {
	ctx := context.Background()
	missions, transcripts := hostCommandApprovalFixture(t)
	executor := newBlockingHostCommandExecutor()
	approvals := NewInMemoryCommandApprovalStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       approvals,
		Missions:    missions,
		Transcripts: transcripts,
		Executor:    executor,
	})
	first, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
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
	second, err := controller.RequestApproval(ctx, CommandApprovalRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
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

	started := time.Now()
	group, err := controller.DecideGroupAsync(ctx, first.GroupID, "approved")
	if err != nil {
		t.Fatalf("DecideGroupAsync() error = %v", err)
	}
	if elapsed := time.Since(started); elapsed > 50*time.Millisecond {
		t.Fatalf("DecideGroupAsync() took %s, want immediate return before command executions complete", elapsed)
	}
	if group.ID != first.GroupID || group.Total != 2 || group.Status != CommandApprovalStatusApproved {
		t.Fatalf("group = %#v, want approved group with two approvals", group)
	}
	firstStored, err := approvals.Get(ctx, first.ID)
	if err != nil {
		t.Fatalf("Get(first) error = %v", err)
	}
	secondStored, err := approvals.Get(ctx, second.ID)
	if err != nil {
		t.Fatalf("Get(second) error = %v", err)
	}
	if firstStored.Status != CommandApprovalStatusApproved || secondStored.Status != CommandApprovalStatusApproved {
		t.Fatalf("statuses = %q/%q, want both approved while executions are blocked", firstStored.Status, secondStored.Status)
	}
}

func hostCommandApprovalFixture(t *testing.T) (*InMemoryMissionStore, *InMemoryTranscriptStore) {
	t.Helper()
	ctx := context.Background()
	missions := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	mission := HostOperationMission{
		ID:           "mission-1",
		ThreadID:     "thread-1",
		Status:       HostMissionStatusRunning,
		PlanRequired: true,
		PlanAccepted: true,
		Plan: HostOperationPlan{
			ID:      "plan-1",
			Version: 1,
			Status:  PlanStatusRunning,
			Steps: []PlanStep{{
				ID:         "step-1",
				Index:      1,
				Title:      "执行通用主机操作",
				Status:     PlanStepStatusRunning,
				HostIDs:    []string{"host-a"},
				RiskLevel:  opssemantic.RiskLowWrite,
				ActionType: opssemantic.ActionWrite,
			}},
		},
		Mentions: []HostMention{{
			Raw:         "@host-a",
			HostID:      "host-a",
			DisplayName: "host-a",
			Resolved:    true,
			Source:      HostMentionSourceInventory,
		}},
	}
	if err := missions.SaveMission(ctx, mission); err != nil {
		t.Fatalf("SaveMission error = %v", err)
	}
	if err := missions.SaveChildAgent(ctx, HostChildAgent{
		ID:          "child-a",
		MissionID:   "mission-1",
		SessionID:   "session-child-a",
		HostID:      "host-a",
		Status:      HostChildAgentStatusRunning,
		PlanStepIDs: []string{"step-1"},
	}); err != nil {
		t.Fatalf("SaveChildAgent error = %v", err)
	}
	return missions, transcripts
}

type blockingHostCommandExecutor struct {
	started chan HostCommandRequest
	done    chan blockingHostCommandExecutorResult
}

type blockingHostCommandExecutorResult struct {
	result HostCommandResult
	err    error
}

func newBlockingHostCommandExecutor() *blockingHostCommandExecutor {
	return &blockingHostCommandExecutor{
		started: make(chan HostCommandRequest, 8),
		done:    make(chan blockingHostCommandExecutorResult, 8),
	}
}

func (e *blockingHostCommandExecutor) RunShell(_ context.Context, _ ToolContext, req HostCommandRequest) (HostCommandResult, error) {
	e.started <- req
	next := <-e.done
	return next.result, next.err
}

func (e *blockingHostCommandExecutor) release(result HostCommandResult, err error) {
	e.done <- blockingHostCommandExecutorResult{result: result, err: err}
}

func eventuallyHostCommandApprovalStatus(t *testing.T, store *InMemoryCommandApprovalStore, approvalID string, want CommandApprovalStatus) CommandApproval {
	t.Helper()
	deadline := time.Now().Add(500 * time.Millisecond)
	for {
		approval, err := store.Get(context.Background(), approvalID)
		if err == nil && approval.Status == want {
			return approval
		}
		if time.Now().After(deadline) {
			if err != nil {
				t.Fatalf("Get(%q) error = %v", approvalID, err)
			}
			t.Fatalf("approval %q status = %q, want %q", approvalID, approval.Status, want)
		}
		time.Sleep(10 * time.Millisecond)
	}
}
