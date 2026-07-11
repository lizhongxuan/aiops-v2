package appui

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/specialinputmemory"
)

type transportCommandChatServiceStub struct {
	sendCmd ChatCommand
	sendRes TurnResponse
	sendErr error

	stopCmd StopCommand
	stopRes TurnResponse
	stopErr error
}

func (s *transportCommandChatServiceStub) SendMessage(_ context.Context, cmd ChatCommand) (TurnResponse, error) {
	s.sendCmd = cmd
	return s.sendRes, s.sendErr
}

func (s *transportCommandChatServiceStub) ResumeTurn(_ context.Context, cmd ResumeCommand) (TurnResponse, error) {
	return TurnResponse{SessionID: cmd.SessionID, TurnID: cmd.TurnID, Status: "completed"}, nil
}

func (s *transportCommandChatServiceStub) CancelTurn(_ context.Context, cmd CancelCommand) (TurnResponse, error) {
	return TurnResponse{SessionID: cmd.SessionID, TurnID: cmd.TurnID, Status: "cancelled"}, nil
}

func (s *transportCommandChatServiceStub) StopTurn(_ context.Context, cmd StopCommand) (TurnResponse, error) {
	s.stopCmd = cmd
	return s.stopRes, s.stopErr
}

type transportCommandApprovalServiceStub struct {
	decision ApprovalDecision
	result   ActionResult
	err      error
}

func (s *transportCommandApprovalServiceStub) List(context.Context) ([]ApprovalView, error) {
	return nil, nil
}

func (s *transportCommandApprovalServiceStub) Decide(_ context.Context, decision ApprovalDecision) (ActionResult, error) {
	s.decision = decision
	return s.result, s.err
}

type transportCommandAsyncApprovalServiceStub struct {
	decision ApprovalDecision
	result   ActionResult
	err      error
}

func (s *transportCommandAsyncApprovalServiceStub) List(context.Context) ([]ApprovalView, error) {
	return nil, nil
}

func (s *transportCommandAsyncApprovalServiceStub) Decide(context.Context, ApprovalDecision) (ActionResult, error) {
	return ActionResult{}, fmt.Errorf("Decide should not be called when DecideAsync is available")
}

func (s *transportCommandAsyncApprovalServiceStub) DecideAsync(_ context.Context, decision ApprovalDecision) (ActionResult, error) {
	s.decision = decision
	return s.result, s.err
}

type transportCommandChoiceServiceStub struct {
	answer ChoiceAnswer
	result ActionResult
	err    error
}

func (s *transportCommandChoiceServiceStub) Answer(_ context.Context, answer ChoiceAnswer) (ActionResult, error) {
	s.answer = answer
	return s.result, s.err
}

type transportCommandMCPServiceStub struct {
	actName   string
	actAction string
	actResult MCPServersPayload
	actErr    error

	refreshCalled bool
	refreshResult MCPServersPayload
	refreshErr    error
}

type transportCommandHostOpsServiceStub struct {
	getMissionID string
	getView      HostOperationView

	acceptedMissionID string
	acceptedPlanID    string

	revisedMissionID string
	revisionText     string

	childMessageID   string
	childMessageText string

	stoppedChildID string
}

func (s *transportCommandHostOpsServiceStub) CreateMission(context.Context, HostMissionCreateCommand) (HostOperationView, error) {
	return HostOperationView{}, nil
}

func (s *transportCommandHostOpsServiceStub) GetMission(_ context.Context, missionID string) (HostOperationView, error) {
	s.getMissionID = missionID
	return s.getView, nil
}

func (s *transportCommandHostOpsServiceStub) AcceptPlan(_ context.Context, missionID, planID string) (HostOperationView, error) {
	s.acceptedMissionID = missionID
	s.acceptedPlanID = planID
	return HostOperationView{ID: missionID, Status: "spawning_children"}, nil
}

func (s *transportCommandHostOpsServiceStub) RevisePlan(_ context.Context, missionID, instruction string) (HostOperationView, error) {
	s.revisedMissionID = missionID
	s.revisionText = instruction
	return HostOperationView{ID: missionID, Status: "planning"}, nil
}

func (s *transportCommandHostOpsServiceStub) SendChildMessage(_ context.Context, childAgentID, content string) (HostChildAgentView, error) {
	s.childMessageID = childAgentID
	s.childMessageText = content
	return HostChildAgentView{ID: childAgentID, Status: "running"}, nil
}

func (s *transportCommandHostOpsServiceStub) StopChildAgent(_ context.Context, childAgentID string) (HostChildAgentView, error) {
	s.stoppedChildID = childAgentID
	return HostChildAgentView{ID: childAgentID, Status: "cancelled"}, nil
}

func (s *transportCommandHostOpsServiceStub) ChildTranscript(context.Context, string) (HostChildTranscriptView, error) {
	return HostChildTranscriptView{}, nil
}

func (s *transportCommandMCPServiceStub) List(context.Context) (MCPServersPayload, error) {
	return MCPServersPayload{}, nil
}

func (s *transportCommandMCPServiceStub) Health(context.Context) (MCPHealthPayload, error) {
	return MCPHealthPayload{}, nil
}

func (s *transportCommandMCPServiceStub) HealthOne(context.Context, string) (MCPHealthView, error) {
	return MCPHealthView{}, nil
}

func (s *transportCommandMCPServiceStub) Create(context.Context, MCPServerUpsert) (MCPServersPayload, error) {
	return MCPServersPayload{}, fmt.Errorf("not implemented")
}

func (s *transportCommandMCPServiceStub) Update(context.Context, string, MCPServerUpsert) (MCPServersPayload, error) {
	return MCPServersPayload{}, fmt.Errorf("not implemented")
}

func (s *transportCommandMCPServiceStub) Delete(context.Context, string) (MCPServersPayload, error) {
	return MCPServersPayload{}, fmt.Errorf("not implemented")
}

func (s *transportCommandMCPServiceStub) Act(_ context.Context, name, action string) (MCPServersPayload, error) {
	s.actName = name
	s.actAction = action
	return s.actResult, s.actErr
}

func (s *transportCommandMCPServiceStub) Refresh(context.Context) (MCPServersPayload, error) {
	s.refreshCalled = true
	return s.refreshResult, s.refreshErr
}

func TestTransportCommandsAddMessageCallsChatService(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-1", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			HostID:   "host-prod-07",
			Message:  TransportUserMessage{Text: "investigate payment-api"},
			Metadata: map[string]string{"aiops.target.kind": "label", "aiops.target.labelKey": "env", "aiops.target.labelValue": "prod"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if chat.sendCmd.Content != "investigate payment-api" {
		t.Fatalf("SendMessage content = %q, want investigate payment-api", chat.sendCmd.Content)
	}
	if chat.sendCmd.HostID != "host-prod-07" {
		t.Fatalf("SendMessage hostId = %q, want host-prod-07", chat.sendCmd.HostID)
	}
	if got := chat.sendCmd.Metadata["aiops.target.labelValue"]; got != "prod" {
		t.Fatalf("SendMessage metadata labelValue = %q, want prod", got)
	}
	if nextState.SessionID != "sess-1" || nextState.ThreadID != "thread-1" {
		t.Fatalf("nextState = %+v, want sess-1/thread-1", nextState)
	}
	if len(nextState.TurnOrder) != 1 || nextState.TurnOrder[0] != "turn-1" {
		t.Fatalf("TurnOrder = %#v, want [turn-1]", nextState.TurnOrder)
	}
	turn := nextState.Turns["turn-1"]
	if turn.User == nil || turn.User.Text != "investigate payment-api" {
		t.Fatalf("turn.User = %+v, want accepted user message", turn.User)
	}
	if !nextState.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("ActiveTurns = %#v, want turn-1 active", nextState.RuntimeLiveness.ActiveTurns)
	}
	if result.SessionID != "sess-1" || result.TurnID != "turn-1" {
		t.Fatalf("result = %+v, want sess-1/turn-1", result)
	}
}

func TestTransportCommandsAddMessagePassesRuntimeControlsAndClientIDs(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-controls", TurnID: "turn-controls", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-controls")

	_, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			SessionType:     "workspace",
			Mode:            "plan",
			ClientMessageID: "client-message-controls",
			ClientTurnID:    "client-turn-controls",
			Message:         TransportUserMessage{Text: "inspect runtime controls"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if chat.sendCmd.SessionType != "workspace" || chat.sendCmd.Mode != "plan" {
		t.Fatalf("SendMessage runtime controls = %q/%q, want workspace/plan", chat.sendCmd.SessionType, chat.sendCmd.Mode)
	}
	if chat.sendCmd.ClientMessageID != "client-message-controls" || chat.sendCmd.ClientTurnID != "client-turn-controls" {
		t.Fatalf("SendMessage client ids = %q/%q", chat.sendCmd.ClientMessageID, chat.sendCmd.ClientTurnID)
	}
}

func TestTransportCommandsAddMessageLeavesMissingRuntimeControlsEmpty(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-defaults", TurnID: "turn-defaults", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-defaults")

	_, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:       TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{Message: TransportUserMessage{Text: "use defaults"}},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if chat.sendCmd.SessionType != "" || chat.sendCmd.Mode != "" || chat.sendCmd.ClientMessageID != "" || chat.sendCmd.ClientTurnID != "" {
		t.Fatalf("missing controls were synthesized: %+v", chat.sendCmd)
	}
}

func TestTransportCommandsSpecialInputClearCallsChatServiceAndClearsContext(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-special", TurnID: "turn-clear", Status: "completed"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("sess-special", "thread-special")
	state.SpecialInputContext = &specialinputmemory.TransportContext{
		SchemaVersion: specialinputmemory.SchemaVersion,
		ActiveGrant: &specialinputmemory.TransportGrant{
			ID:           "grant-host-a",
			ResourceKind: specialinputmemory.ResourceKindHost,
			ResourceID:   "host-a",
		},
	}

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:              TransportCommandTypeSpecialInputClear,
		SpecialInputClear: &TransportSpecialInputClearCommand{},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if chat.sendCmd.Content != "忘记当前主机" {
		t.Fatalf("chat content = %q, want forget command", chat.sendCmd.Content)
	}
	if chat.sendCmd.Metadata["aiops.specialInput.command"] != "clear" {
		t.Fatalf("chat metadata = %#v, want clear marker", chat.sendCmd.Metadata)
	}
	if nextState.SpecialInputContext != nil {
		t.Fatalf("SpecialInputContext = %#v, want nil", nextState.SpecialInputContext)
	}
	if result.Status != "completed" || result.TurnID != "turn-clear" {
		t.Fatalf("result = %#v, want completed turn-clear", result)
	}
}

func TestTransportCommandsSpecialInputConfirmCallsChatService(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-special", TurnID: "turn-confirm", Status: "completed"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("sess-special", "thread-special")

	_, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:                TransportCommandTypeSpecialInputConfirm,
		SpecialInputConfirm: &TransportSpecialInputConfirmCommand{},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if chat.sendCmd.Content != "确认" {
		t.Fatalf("chat content = %q, want confirmation command", chat.sendCmd.Content)
	}
	if chat.sendCmd.Metadata["aiops.specialInput.command"] != "confirm" {
		t.Fatalf("chat metadata = %#v, want confirm marker", chat.sendCmd.Metadata)
	}
	if result.Status != "completed" || result.TurnID != "turn-confirm" {
		t.Fatalf("result = %#v, want completed turn-confirm", result)
	}
}

func TestTransportCommandsAddMessageUsesTargetHostMetadataWhenHostIDMissing(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-1", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	_, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "检查 CPU"},
			Metadata: map[string]string{
				"aiops.target.kind":   "host",
				"aiops.target.hostId": "remote-linux-01",
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if chat.sendCmd.HostID != "remote-linux-01" {
		t.Fatalf("SendMessage hostId = %q, want remote-linux-01", chat.sendCmd.HostID)
	}
}

func TestTransportCommandsAddMessageCreatesMultiHostMissionRoute(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-1", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "@1.1.1.1和@1.1.1.2作为pg节点，搭建主从集群"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if chat.sendCmd.Metadata["aiops.hostops.routeKind"] != "host_ops" {
		t.Fatalf("routeKind metadata = %q, want host_ops", chat.sendCmd.Metadata["aiops.hostops.routeKind"])
	}
	if chat.sendCmd.Metadata["aiops.hostops.planRequired"] != "true" {
		t.Fatalf("planRequired metadata = %q, want true", chat.sendCmd.Metadata["aiops.hostops.planRequired"])
	}
	if chat.sendCmd.Metadata["aiops.hostops.serverDetectedMultiHost"] != "true" {
		t.Fatalf("serverDetectedMultiHost metadata = %q, want true", chat.sendCmd.Metadata["aiops.hostops.serverDetectedMultiHost"])
	}
	if !metadataListContainsValueForTest(chat.sendCmd.Metadata["enableToolPack"], hostops.ToolPackHostOps) {
		t.Fatalf("enableToolPack metadata = %q, want %q", chat.sendCmd.Metadata["enableToolPack"], hostops.ToolPackHostOps)
	}
	if chat.sendCmd.Metadata["aiops.hostops.mentions"] == "" {
		t.Fatal("expected serialized server-side mentions metadata")
	}

	missionID := nextState.ActiveHostMissionID
	if missionID == "" {
		t.Fatal("ActiveHostMissionID is empty")
	}
	mission := nextState.HostMissions[missionID]
	if mission.ID != missionID || mission.TurnID != "turn-1" {
		t.Fatalf("mission identity = %+v, want active mission for turn-1", mission)
	}
	if mission.Status != "waiting_plan_acceptance" || !mission.PlanRequired || mission.PlanAccepted {
		t.Fatalf("mission status/plan = %+v, want waiting required unaccepted", mission)
	}
	if len(mission.MentionedHosts) != 2 {
		t.Fatalf("mentioned hosts = %+v, want 2", mission.MentionedHosts)
	}
}

func TestTransportCommandsAddMessageDoesNotCreateHostOpsPlaceholderForSingleHostBoundChat(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-1", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "@remote-120-77-239-90 只读检查 hostname 和系统版本"},
			Metadata: map[string]string{
				"aiops.hostops.mentions": `[{"raw":"@remote-120-77-239-90","hostId":"remote-120-77-239-90","address":"120.77.239.90","displayName":"remote-120-77-239-90","source":"inventory","resolved":true}]`,
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if chat.sendCmd.HostID != "remote-120-77-239-90" {
		t.Fatalf("SendMessage HostID = %q, want remote-120-77-239-90", chat.sendCmd.HostID)
	}
	if chat.sendCmd.Metadata["aiops.route.mode"] != string(ChatRouteHostBoundOps) {
		t.Fatalf("route mode = %q, want host_bound_ops; metadata=%#v", chat.sendCmd.Metadata["aiops.route.mode"], chat.sendCmd.Metadata)
	}
	if nextState.ActiveHostMissionID != "" || len(nextState.HostMissions) != 0 {
		t.Fatalf("host mission placeholder = %q/%#v, want none for single host-bound chat", nextState.ActiveHostMissionID, nextState.HostMissions)
	}
	if nextState.Status != AiopsTransportStatusWorking {
		t.Fatalf("state status = %q, want working for real AI Chat turn", nextState.Status)
	}
	if !nextState.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("activeTurns[turn-1] = false, want true for real AI Chat turn")
	}
}

func TestTransportCommandsAddMessageMergesHostOpsMissionView(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-1", Status: "accepted"},
	}
	hostOps := &transportCommandHostOpsServiceStub{
		getView: HostOperationView{
			ID:           "hostops:turn-1",
			UserTurnID:   "turn-1",
			Status:       "waiting_plan_acceptance",
			PlanRequired: true,
			MentionedHosts: []HostMentionView{
				{Raw: "@pg-a", HostID: "host-a", DisplayName: "@pg-a", Source: "inventory", Resolved: true},
				{Raw: "@pg-b", HostID: "host-b", DisplayName: "@pg-b", Source: "inventory", Resolved: true},
			},
			Plan: &HostPlanView{ID: "plan-1", Steps: []HostPlanStepView{
				{ID: "step-a", Index: 1, Title: "Run assigned host operation", Status: "pending", HostIDs: []string{"host-a"}},
				{ID: "step-b", Index: 2, Title: "Run assigned host operation", Status: "pending", HostIDs: []string{"host-b"}},
			}},
		},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil).WithHostOpsService(hostOps)
	state := NewAiopsTransportState("", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "@pg-a 和 @pg-b 执行通用运维检查"},
			Metadata: map[string]string{
				"aiops.hostops.mentions": `[{"raw":"@pg-a","hostId":"host-a","displayName":"@pg-a","source":"inventory","resolved":true},{"raw":"@pg-b","hostId":"host-b","displayName":"@pg-b","source":"inventory","resolved":true}]`,
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if hostOps.getMissionID != "hostops:turn-1" {
		t.Fatalf("GetMission missionID = %q, want hostops:turn-1", hostOps.getMissionID)
	}
	mission := nextState.HostMissions["hostops:turn-1"]
	if len(mission.PlanSteps) != 2 {
		t.Fatalf("planSteps = %#v, want 2 steps from HostOpsService view", mission.PlanSteps)
	}
	if len(mission.MentionedHosts) != 2 || mission.MentionedHosts[0].HostID != "host-a" {
		t.Fatalf("mentionedHosts = %#v, want service-resolved hosts", mission.MentionedHosts)
	}
	if nextState.Status != AiopsTransportStatusWorking {
		t.Fatalf("state status = %q, want working while runtime owns host-ops turn", nextState.Status)
	}
	if !nextState.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("activeTurns[turn-1] = false, want true while runtime owns host-ops turn")
	}
	if nextState.Turns["turn-1"].Status != AiopsTransportTurnStatusSubmitted {
		t.Fatalf("turn status = %q, want submitted until runtime projection completes it", nextState.Turns["turn-1"].Status)
	}
}

func TestTransportCommandsAddMessageDoesNotRouteUnresolvedToolMentionToHostOps(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-coroot", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "@coroot 分析环境A的A服务为什么异常"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if chat.sendCmd.Metadata["aiops.hostops.routeKind"] != "" {
		t.Fatalf("routeKind metadata = %q, want empty for @coroot", chat.sendCmd.Metadata["aiops.hostops.routeKind"])
	}
	if nextState.ActiveHostMissionID != "" || len(nextState.HostMissions) != 0 {
		t.Fatalf("host missions = %#v active=%q, want none", nextState.HostMissions, nextState.ActiveHostMissionID)
	}
}

func TestTransportCommandsAddMessageInjectsChatRuntimeRouteMetadata(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-route", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "pg_auto_failover timeline 为什么会比主库高？"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if got := chat.sendCmd.Metadata["aiops.route.mode"]; got != string(ChatRouteAdvisory) {
		t.Fatalf("aiops.route.mode = %q, want %q", got, ChatRouteAdvisory)
	}
	if got := chat.sendCmd.Metadata["aiops.tool.corootRCAAllowed"]; got != "false" {
		t.Fatalf("aiops.tool.corootRCAAllowed = %q, want false", got)
	}
	if got := chat.sendCmd.Metadata["aiops.toolPack.coroot_rca.allowed"]; got != "false" {
		t.Fatalf("coroot_rca pack gate = %q, want false", got)
	}
	if !metadataListContainsValueForTest(chat.sendCmd.Metadata["enableToolPack"], "public_web") {
		t.Fatalf("enableToolPack = %q, want public_web for operational mechanism diagnosis", chat.sendCmd.Metadata["enableToolPack"])
	}
	projected := transportProjectionText(nextState)
	for _, forbidden := range []string{
		"已进入咨询分析",
		"不会执行主机命令",
		"优先检索官方资料",
		"遇到不熟悉的中间件",
		"Coroot",
		"@Coroot",
	} {
		if strings.Contains(projected, forbidden) {
			t.Fatalf("projected text = %q, should not expose route summary %q", projected, forbidden)
		}
	}
}

func TestTransportCommandsAddMessageAllowsCorootOnlyWithExplicitMention(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-coroot-rca", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	_, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "@Coroot 分析 checkout 服务异常"},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if got := chat.sendCmd.Metadata["aiops.tool.corootRCAAllowed"]; got != "true" {
		t.Fatalf("aiops.tool.corootRCAAllowed = %q, want true", got)
	}
	if got := chat.sendCmd.Metadata["aiops.toolPack.coroot_rca.allowed"]; got != "true" {
		t.Fatalf("coroot_rca pack gate = %q, want true", got)
	}
}

func TestTransportCommandsAddMessageDoesNotPreRouteWorkflowWritingToHostOps(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-workflow", Status: "completed", Output: "Workflow 计划已生成"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeAddMessage,
		AddMessage: &TransportAddMessageCommand{
			Message: TransportUserMessage{Text: "帮我写一个workflow，让主机A=@pg-a和主机B=@pg-b通过主机C=@pg-mon形成PG集群"},
			Metadata: map[string]string{
				"aiops.hostops.mentions": `[{"raw":"@pg-a","hostId":"host-a","displayName":"@pg-a","source":"inventory","resolved":true},{"raw":"@pg-b","hostId":"host-b","displayName":"@pg-b","source":"inventory","resolved":true},{"raw":"@pg-mon","hostId":"host-c","displayName":"@pg-mon","source":"inventory","resolved":true}]`,
			},
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if chat.sendCmd.Metadata["aiops.hostops.routeKind"] != "" {
		t.Fatalf("routeKind metadata = %q, want empty for workflow writing request", chat.sendCmd.Metadata["aiops.hostops.routeKind"])
	}
	if nextState.ActiveHostMissionID != "" || len(nextState.HostMissions) != 0 {
		t.Fatalf("host missions = %#v active=%q, want none for workflow writing request", nextState.HostMissions, nextState.ActiveHostMissionID)
	}
}

func metadataListContainsValueForTest(raw, want string) bool {
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ';' || r == '\n' || r == '\t' || r == ' '
	})
	for _, field := range fields {
		if strings.TrimSpace(field) == want {
			return true
		}
	}
	return false
}

func TestTransportCommandsHostPlanAcceptCallsHostOpsService(t *testing.T) {
	hostOps := &transportCommandHostOpsServiceStub{}
	handler := NewTransportCommandHandler(nil, nil, nil, nil).WithHostOpsService(hostOps)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.HostMissions["mission-1"] = AiopsTransportHostMission{ID: "mission-1", Status: "waiting_plan_acceptance"}

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:           TransportCommandTypeHostPlanAccept,
		HostPlanAccept: &TransportHostPlanAcceptCommand{MissionID: "mission-1", PlanID: "plan-1"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if hostOps.acceptedMissionID != "mission-1" || hostOps.acceptedPlanID != "plan-1" {
		t.Fatalf("hostOps accept = %q/%q, want mission-1/plan-1", hostOps.acceptedMissionID, hostOps.acceptedPlanID)
	}
	if !nextState.HostMissions["mission-1"].PlanAccepted || nextState.HostMissions["mission-1"].Status != "spawning_children" {
		t.Fatalf("mission state = %+v, want accepted/spawning_children", nextState.HostMissions["mission-1"])
	}
	if result.Status != "spawning_children" {
		t.Fatalf("result.Status = %q, want spawning_children", result.Status)
	}
}

func TestTransportCommandsChildAgentMessageCallsHostOpsService(t *testing.T) {
	hostOps := &transportCommandHostOpsServiceStub{}
	handler := NewTransportCommandHandler(nil, nil, nil, nil).WithHostOpsService(hostOps)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.ChildAgents["agent-1"] = AiopsTransportChildAgent{ID: "agent-1", Status: "waiting"}

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:              TransportCommandTypeChildAgentMessage,
		ChildAgentMessage: &TransportChildAgentMessageCommand{ChildAgentID: "agent-1", Content: "只读检查，不要修改"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if hostOps.childMessageID != "agent-1" || hostOps.childMessageText != "只读检查，不要修改" {
		t.Fatalf("child message = %q/%q, want agent-1/content", hostOps.childMessageID, hostOps.childMessageText)
	}
	if nextState.ChildAgents["agent-1"].Status != "running" {
		t.Fatalf("child state = %+v, want running", nextState.ChildAgents["agent-1"])
	}
	if result.Status != "running" {
		t.Fatalf("result.Status = %q, want running", result.Status)
	}
}

func TestTransportCommandsStopCallsChatServiceStopImmediately(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		stopRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-1", Status: "cancelled"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.Status = AiopsTransportStatusWorking

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeStop,
		Stop: &TransportStopCommand{SessionID: "sess-1", TurnID: "turn-1"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if chat.stopCmd.SessionID != "sess-1" || chat.stopCmd.TurnID != "turn-1" {
		t.Fatalf("StopTurn cmd = %+v, want sess-1/turn-1", chat.stopCmd)
	}
	if nextState.Status != AiopsTransportStatusCanceled {
		t.Fatalf("nextState.Status = %q, want canceled", nextState.Status)
	}
	if result.Status != "cancelled" {
		t.Fatalf("result.Status = %q, want cancelled", result.Status)
	}
}

func TestTransportCommandsStopMarksReturnedTurnWhenCurrentTurnIsStale(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		stopRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-2", Status: "cancelled"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.Status = AiopsTransportStatusWorking
	state.CurrentTurnID = "turn-1"
	state.Turns["turn-1"] = AiopsTransportTurn{ID: "turn-1", Status: AiopsTransportTurnStatusCompleted}
	state.Turns["turn-2"] = AiopsTransportTurn{ID: "turn-2", Status: AiopsTransportTurnStatusWorking}
	state.RuntimeLiveness.ActiveTurns["turn-2"] = true
	state.RuntimeLiveness.ActiveCommandStreams["call-1"] = true
	state.RuntimeLiveness.PendingApprovals["approval-1"] = true
	state.PendingApprovals["approval-1"] = AiopsTransportApproval{ID: "approval-1", TurnID: "turn-2"}

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeStop,
		Stop: &TransportStopCommand{SessionID: "sess-1", TurnID: "turn-2"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if nextState.Turns["turn-2"].Status != AiopsTransportTurnStatusCanceled {
		t.Fatalf("turn-2 status = %q, want canceled", nextState.Turns["turn-2"].Status)
	}
	if nextState.Turns["turn-1"].Status != AiopsTransportTurnStatusCompleted {
		t.Fatalf("turn-1 status = %q, should not be changed", nextState.Turns["turn-1"].Status)
	}
	if len(nextState.RuntimeLiveness.ActiveTurns) != 0 || len(nextState.RuntimeLiveness.ActiveCommandStreams) != 0 || len(nextState.RuntimeLiveness.PendingApprovals) != 0 || len(nextState.PendingApprovals) != 0 {
		t.Fatalf("liveness after stop = %+v pending=%+v, want cleared", nextState.RuntimeLiveness, nextState.PendingApprovals)
	}
}

func TestTransportCommandsRetryReusesOriginalUserMessage(t *testing.T) {
	chat := &transportCommandChatServiceStub{
		sendRes: TurnResponse{SessionID: "sess-1", TurnID: "turn-2", Status: "accepted"},
	}
	handler := NewTransportCommandHandler(chat, nil, nil, nil)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.CurrentTurnID = "turn-1"
	state.TurnOrder = []string{"turn-1"}
	state.Turns["turn-1"] = AiopsTransportTurn{
		ID:   "turn-1",
		User: &AiopsTransportMessage{ID: "user-1", Text: "retry the deployment diagnosis"},
	}

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:  TransportCommandTypeRetry,
		Retry: &TransportRetryCommand{SessionID: "sess-1", TurnID: "turn-1"},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if chat.sendCmd.Content != "retry the deployment diagnosis" {
		t.Fatalf("SendMessage content = %q, want retry the deployment diagnosis", chat.sendCmd.Content)
	}
	if nextState.CurrentTurnID != "turn-2" || nextState.Status != AiopsTransportStatusWorking {
		t.Fatalf("nextState = %+v, want current turn turn-2 working", nextState)
	}
	if result.TurnID != "turn-2" || result.Status != "accepted" {
		t.Fatalf("result = %+v, want accepted retry turn", result)
	}
}

func TestTransportCommandsApprovalRejectUsesApprovalService(t *testing.T) {
	approvals := &transportCommandApprovalServiceStub{
		result: ActionResult{Status: "failed", SessionID: "sess-1", TurnID: "turn-1"},
	}
	handler := NewTransportCommandHandler(nil, approvals, nil, nil)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.CurrentTurnID = "turn-1"
	state.Turns["turn-1"] = AiopsTransportTurn{ID: "turn-1", Status: AiopsTransportTurnStatusBlocked}
	state.PendingApprovals["approval-1"] = AiopsTransportApproval{ID: "approval-1", TurnID: "turn-1", Status: "blocked"}
	state.RuntimeLiveness.PendingApprovals["approval-1"] = true

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeApprovalDecision,
		ApprovalDecision: &TransportApprovalDecisionCommand{
			ApprovalID: "approval-1",
			Decision:   "reject",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if approvals.decision.ID != "approval-1" || approvals.decision.Decision != "reject" {
		t.Fatalf("approval decision = %+v, want approval-1/reject", approvals.decision)
	}
	if _, ok := nextState.PendingApprovals["approval-1"]; ok {
		t.Fatalf("PendingApprovals still contains approval-1: %#v", nextState.PendingApprovals)
	}
	if nextState.Status != AiopsTransportStatusFailed {
		t.Fatalf("nextState.Status = %q, want failed after reject", nextState.Status)
	}
	if nextState.Turns["turn-1"].Status != AiopsTransportTurnStatusFailed {
		t.Fatalf("turn status = %q, want failed after reject", nextState.Turns["turn-1"].Status)
	}
}

func TestTransportCommandsApprovalAcceptClearsPendingAndReturnsWorkingImmediately(t *testing.T) {
	approvals := &transportCommandAsyncApprovalServiceStub{
		result: ActionResult{Status: "accepted", SessionID: "sess-1", TurnID: "turn-1"},
	}
	handler := NewTransportCommandHandler(nil, approvals, nil, nil)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.Status = AiopsTransportStatusBlocked
	state.CurrentTurnID = "turn-1"
	state.Turns["turn-1"] = AiopsTransportTurn{
		ID:     "turn-1",
		Status: AiopsTransportTurnStatusBlocked,
		Process: []AiopsProcessBlock{
			{
				ID:         "cmd-1",
				Kind:       AiopsTransportProcessKindCommand,
				Status:     AiopsTransportProcessStatusBlocked,
				Command:    "ifconfig en0 down",
				ApprovalID: "approval-1",
			},
		},
	}
	state.PendingApprovals["approval-1"] = AiopsTransportApproval{ID: "approval-1", TurnID: "turn-1", Status: "blocked"}
	state.RuntimeLiveness.PendingApprovals["approval-1"] = true

	nextState, result, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeApprovalDecision,
		ApprovalDecision: &TransportApprovalDecisionCommand{
			ApprovalID: "approval-1",
			Decision:   "accept",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if approvals.decision.SessionID != "sess-1" || approvals.decision.TurnID != "turn-1" || approvals.decision.ID != "approval-1" || approvals.decision.Decision != "accept" {
		t.Fatalf("approval decision = %+v, want sess-1/turn-1/approval-1/accept", approvals.decision)
	}
	if result.Status != "accepted" {
		t.Fatalf("result.Status = %q, want accepted", result.Status)
	}
	if _, ok := nextState.PendingApprovals["approval-1"]; ok {
		t.Fatalf("PendingApprovals still contains approval-1: %#v", nextState.PendingApprovals)
	}
	if nextState.RuntimeLiveness.PendingApprovals["approval-1"] {
		t.Fatalf("RuntimeLiveness.PendingApprovals still contains approval-1: %#v", nextState.RuntimeLiveness.PendingApprovals)
	}
	if nextState.Status != AiopsTransportStatusWorking {
		t.Fatalf("nextState.Status = %q, want working", nextState.Status)
	}
	if !nextState.RuntimeLiveness.ActiveTurns["turn-1"] {
		t.Fatalf("ActiveTurns = %#v, want turn-1 active after accept", nextState.RuntimeLiveness.ActiveTurns)
	}
	turn := nextState.Turns["turn-1"]
	if turn.Status != AiopsTransportTurnStatusWorking {
		t.Fatalf("turn status = %q, want working", turn.Status)
	}
	if len(turn.Process) != 1 || turn.Process[0].Status != AiopsTransportProcessStatusRunning {
		t.Fatalf("process = %+v, want approved command marked running while backend resumes", turn.Process)
	}
}

func TestTransportCommandsChoiceAnswerUsesChoiceService(t *testing.T) {
	choices := &transportCommandChoiceServiceStub{
		result: ActionResult{Status: "completed", SessionID: "sess-1", TurnID: "turn-1"},
	}
	handler := NewTransportCommandHandler(nil, nil, choices, nil)

	_, _, err := handler.Apply(context.Background(), NewAiopsTransportState("sess-1", "thread-1"), TransportCommand{
		Type: TransportCommandTypeChoiceAnswer,
		ChoiceAnswer: &TransportChoiceAnswerCommand{
			RequestID: "choice-1",
			Answer:    "continue",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}
	if choices.answer.RequestID != "choice-1" || len(choices.answer.Answers) != 1 || choices.answer.Answers[0] != "continue" {
		t.Fatalf("choice answer = %+v, want single continue answer", choices.answer)
	}
}

func TestTransportCommandsMCPActionUsesMCPService(t *testing.T) {
	mcps := &transportCommandMCPServiceStub{}
	handler := NewTransportCommandHandler(nil, nil, nil, mcps)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.McpSurfaces["surface-1"] = AiopsTransportMcpSurface{ID: "surface-1", Status: "ready"}

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeMCPAction,
		MCPAction: &TransportMCPActionCommand{
			SurfaceID: "surface-1",
			ActionID:  "refresh",
		},
	})
	if err != nil {
		t.Fatalf("Apply() error = %v", err)
	}

	if mcps.actName != "surface-1" || mcps.actAction != "refresh" {
		t.Fatalf("MCP act = %q/%q, want surface-1/refresh", mcps.actName, mcps.actAction)
	}
	if nextState.McpSurfaces["surface-1"].UpdatedAt == "" {
		t.Fatalf("surface updatedAt empty after action: %+v", nextState.McpSurfaces["surface-1"])
	}
}

func TestTransportCommandsMCPActionRejectsUnknownSurfaceBeforeServiceCall(t *testing.T) {
	mcps := &transportCommandMCPServiceStub{}
	handler := NewTransportCommandHandler(nil, nil, nil, mcps)
	state := NewAiopsTransportState("sess-1", "thread-1")

	nextState, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type: TransportCommandTypeMCPAction,
		MCPAction: &TransportMCPActionCommand{
			SurfaceID: "missing-surface",
			ActionID:  "open",
		},
	})
	if err == nil {
		t.Fatal("Apply() error = nil, want unknown surface error")
	}
	if mcps.actName != "" || mcps.actAction != "" {
		t.Fatalf("MCP service was called as %q/%q before authorization", mcps.actName, mcps.actAction)
	}
	if nextState.Status != state.Status {
		t.Fatalf("nextState.Status = %q, want unchanged %q", nextState.Status, state.Status)
	}
}

func TestTransportCommandsMCPRefreshAndPinUpdateState(t *testing.T) {
	mcps := &transportCommandMCPServiceStub{}
	handler := NewTransportCommandHandler(nil, nil, nil, mcps)
	state := NewAiopsTransportState("sess-1", "thread-1")
	state.McpSurfaces["surface-1"] = AiopsTransportMcpSurface{ID: "surface-1", Status: "ready"}

	refreshed, _, err := handler.Apply(context.Background(), state, TransportCommand{
		Type:       TransportCommandTypeMCPRefresh,
		MCPRefresh: &TransportMCPRefreshCommand{SurfaceID: "surface-1"},
	})
	if err != nil {
		t.Fatalf("Apply(refresh) error = %v", err)
	}
	if mcps.actName != "surface-1" || mcps.actAction != "refresh" {
		t.Fatalf("MCP refresh act = %q/%q, want surface-1/refresh", mcps.actName, mcps.actAction)
	}

	pinned, _, err := handler.Apply(context.Background(), refreshed, TransportCommand{
		Type:   TransportCommandTypeMCPPin,
		MCPPin: &TransportMCPPinCommand{SurfaceID: "surface-1", Pinned: true},
	})
	if err != nil {
		t.Fatalf("Apply(pin) error = %v", err)
	}
	if !pinned.McpSurfaces["surface-1"].Pinned {
		t.Fatalf("surface pinned = %+v, want pinned=true", pinned.McpSurfaces["surface-1"])
	}
}
