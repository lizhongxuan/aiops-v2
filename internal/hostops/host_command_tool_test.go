package hostops

import (
	"context"
	"errors"
	"testing"

	"aiops-v2/internal/opssemantic"
)

func TestHostCommandToolRejectsInvalidRequest(t *testing.T) {
	tool := NewHostCommandTool(nil, NewCommandPolicy(CommandPolicyConfig{}))
	_, err := tool.Run(context.Background(), HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-a",
	})
	if !errors.Is(err, ErrInvalidHostCommandRequest) {
		t.Fatalf("err = %v, want ErrInvalidHostCommandRequest", err)
	}
}

func TestHostCommandToolRejectsManagerDirectCommand(t *testing.T) {
	tool := NewHostCommandTool(&fakeHostCommandExecutor{}, NewCommandPolicy(CommandPolicyConfig{}))
	_, err := tool.Run(context.Background(), HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindManager},
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "uptime",
		RiskLevel:    opssemantic.RiskReadOnly,
	})
	if !errors.Is(err, ErrManagerDirectHostDenied) {
		t.Fatalf("err = %v, want ErrManagerDirectHostDenied", err)
	}
}

func TestHostCommandToolAuditsCrossHostSecurityRefusal(t *testing.T) {
	transcripts := NewInMemoryTranscriptStore()
	controller := NewCommandApprovalController(CommandApprovalControllerConfig{
		Store:       NewInMemoryCommandApprovalStore(),
		Transcripts: transcripts,
	})
	tool := NewHostCommandToolWithApprovals(&fakeHostCommandExecutor{}, NewCommandPolicy(CommandPolicyConfig{}), controller)
	_, err := tool.Run(context.Background(), HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-b",
		Command:      "uptime",
		RiskLevel:    opssemantic.RiskReadOnly,
	})
	if !errors.Is(err, ErrCrossHostDenied) {
		t.Fatalf("err = %v, want ErrCrossHostDenied", err)
	}
	items, listErr := transcripts.List(context.Background(), "child-1")
	if listErr != nil {
		t.Fatalf("List transcript error = %v", listErr)
	}
	if len(items) != 1 || items[0].Type != TranscriptItemError || items[0].Status != "security_refused" {
		t.Fatalf("transcript = %#v, want security refusal error event", items)
	}
}

func TestHostCommandToolReturnsPendingApprovalForNonWhitelistedCommand(t *testing.T) {
	executor := &fakeHostCommandExecutor{}
	tool := NewHostCommandTool(executor, NewCommandPolicy(CommandPolicyConfig{}))
	result, err := tool.Run(context.Background(), HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "touch /tmp/aiops-check",
		RiskLevel:    opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.ApprovalRequired || result.Executed {
		t.Fatalf("result = %#v, want pending approval without execution", result)
	}
	if executor.calls != 0 {
		t.Fatalf("executor calls = %d, want 0", executor.calls)
	}
}

func TestHostCommandToolExecutesWhitelistedHostChildCommand(t *testing.T) {
	executor := &fakeHostCommandExecutor{}
	tool := NewHostCommandTool(executor, NewCommandPolicy(CommandPolicyConfig{
		GlobalWhitelist: []CommandPolicyRule{{ID: "uptime", Pattern: "uptime", MaxRisk: opssemantic.RiskReadOnly}},
	}))
	result, err := tool.Run(context.Background(), HostCommandToolRequest{
		ToolContext:  ToolContext{AgentKind: AgentKindHostChild, BoundHostID: "host-a"},
		MissionID:    "mission-1",
		ChildAgentID: "child-1",
		PlanStepID:   "step-1",
		HostID:       "host-a",
		Command:      "uptime",
		RiskLevel:    opssemantic.RiskReadOnly,
	})
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if !result.Executed || result.ApprovalRequired {
		t.Fatalf("result = %#v, want executed without approval", result)
	}
	if executor.calls != 1 {
		t.Fatalf("executor calls = %d, want 1", executor.calls)
	}
	if executor.lastReq.HostID != "host-a" || executor.lastReq.Script != "uptime" {
		t.Fatalf("last request = %#v, want host-a uptime", executor.lastReq)
	}
}

type fakeHostCommandExecutor struct {
	calls   int
	lastReq HostCommandRequest
}

func (e *fakeHostCommandExecutor) RunShell(_ context.Context, _ ToolContext, req HostCommandRequest) (HostCommandResult, error) {
	e.calls++
	e.lastReq = req
	return HostCommandResult{Status: "success", Stdout: "ok", ExitCode: 0}, nil
}
