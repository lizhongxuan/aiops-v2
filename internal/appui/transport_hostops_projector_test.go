package appui

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func TestTransportProjectorIncludesHostMissionAndChildAgents(t *testing.T) {
	now := time.Date(2026, 6, 4, 10, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-hostops", "thread-hostops")
	mentions := json.RawMessage(`[
		{"tokenId":"mention-1","raw":"@1.1.1.1","hostId":"host-a","address":"1.1.1.1","displayName":"Franklin","source":"inventory","resolved":true},
		{"tokenId":"mention-2","raw":"@1.1.1.2","hostId":"host-b","address":"1.1.1.2","displayName":"Harriet","source":"inventory","resolved":true}
	]`)
	planData := json.RawMessage(`{
		"title":"多主机通用运维计划",
		"steps":[
			{"id":"prepare-a","index":1,"title":"执行主机 A 准备步骤","status":"completed","risk":"read_only","hostIds":["host-a"]},
			{"id":"prepare-b","index":2,"title":"执行主机 B 配置步骤","status":"running","risk":"medium_write","hostIds":["host-b"],"approvalRequired":true}
		]
	}`)
	spawnData := json.RawMessage(`{
		"toolCallId":"call-spawn-hosts",
		"toolName":"spawn_host_agent",
		"displayKind":"hostops.spawn_host_agent",
		"inputSummary":"为每台主机启动子 Agent",
		"outputSummary":"spawned 2 host agents",
		"outputPreview":{"children":[
			{"id":"child-a","missionId":"hostops:turn-hostops","sessionId":"host-child:a","hostId":"host-a","hostAddress":"1.1.1.1","hostDisplayName":"Franklin","role":"host_child","task":"执行主机 A 准备步骤","currentStepTitle":"执行主机 A 准备步骤","status":"running","startedAt":"2026-06-04T10:00:01Z","updatedAt":"2026-06-04T10:00:02Z"},
			{"id":"child-b","missionId":"hostops:turn-hostops","sessionId":"host-child:b","hostId":"host-b","hostAddress":"1.1.1.2","hostDisplayName":"Harriet","role":"host_child","task":"执行主机 B 配置步骤","currentStepTitle":"执行主机 B 配置步骤","status":"waiting","startedAt":"2026-06-04T10:00:01Z","updatedAt":"2026-06-04T10:00:02Z"}
		]}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-hostops",
		SessionID:   "session-hostops",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeExecute,
		Metadata: map[string]string{
			"aiops.hostops.routeKind":      "host_ops",
			"aiops.hostops.planRequired":   "true",
			"aiops.hostops.mentions":       string(mentions),
			"aiops.hostops.managerAgentId": "manager-1",
		},
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(2 * time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "plan-hostops", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "多主机通用运维计划", Data: planData}, CreatedAt: now},
			{ID: "spawn-hostops", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "spawned 2 host agents", Data: spawnData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}

	if projected.ActiveHostMissionID != "hostops:turn-hostops" {
		t.Fatalf("ActiveHostMissionID = %q, want hostops:turn-hostops", projected.ActiveHostMissionID)
	}
	mission := projected.HostMissions["hostops:turn-hostops"]
	if mission.ID == "" {
		t.Fatal("hostops mission was not projected")
	}
	if !mission.PlanRequired || mission.PlanAccepted {
		t.Fatalf("mission plan flags = required:%v accepted:%v, want required true accepted false", mission.PlanRequired, mission.PlanAccepted)
	}
	if len(mission.MentionedHosts) != 2 || mission.MentionedHosts[0].HostID != "host-a" {
		t.Fatalf("mission mentions = %#v, want two projected hosts", mission.MentionedHosts)
	}
	if len(mission.PlanSteps) != 2 || mission.PlanSteps[0].Title != "执行主机 A 准备步骤" {
		t.Fatalf("mission plan steps = %#v, want plan steps copied from process plan", mission.PlanSteps)
	}
	if mission.PlanSteps[1].Index != 2 || mission.PlanSteps[1].Risk != "medium_write" || !mission.PlanSteps[1].ApprovalRequired {
		t.Fatalf("mission plan step details = %#v, want index/risk/approval projected", mission.PlanSteps[1])
	}
	if len(mission.ChildAgentIDs) != 2 || mission.ChildAgentIDs[0] != "child-a" || mission.ChildAgentIDs[1] != "child-b" {
		t.Fatalf("mission child ids = %#v, want child-a/child-b", mission.ChildAgentIDs)
	}
	if len(projected.ChildAgents) != 2 {
		t.Fatalf("len(ChildAgents) = %d, want 2", len(projected.ChildAgents))
	}
	if projected.ChildAgents["child-a"].HostID != "host-a" || projected.ChildAgents["child-b"].Status != "waiting" {
		t.Fatalf("ChildAgents = %#v, want projected host-bound children", projected.ChildAgents)
	}
	if projected.ChildAgents["child-a"].CurrentStepTitle != "执行主机 A 准备步骤" {
		t.Fatalf("child current step = %q, want projected current step", projected.ChildAgents["child-a"].CurrentStepTitle)
	}
	subagentBlock := findTransportProcessBlock(t, projected.Turns["turn-hostops"].Process, AiopsTransportProcessKindSubagent)
	if subagentBlock.DisplayKind != "hostops.spawn_host_agent" || subagentBlock.Status != AiopsTransportProcessStatusCompleted {
		t.Fatalf("subagent block = %+v, want completed hostops spawn block", subagentBlock)
	}
}

func TestTransportProjectorMarksHostOpsApprovalBlockAsBlocked(t *testing.T) {
	now := time.Date(2026, 6, 11, 12, 0, 0, 0, time.UTC)
	projector := NewTransportProjector()
	state := NewAiopsTransportState("session-hostops", "thread-hostops")
	planData := json.RawMessage(`{
		"title":"通用多主机运维计划",
		"steps":[
			{"id":"step-blocked","index":1,"title":"等待主机命令审批","status":"blocked","risk":"low_write","hostIds":["host-a"],"childAgentIds":["child-a"],"approvalRequired":true}
		]
	}`)
	spawnData := json.RawMessage(`{
		"toolCallId":"call-spawn-hosts",
		"toolName":"spawn_host_agent",
		"displayKind":"hostops.spawn_host_agent",
		"outputPreview":{"children":[
			{"id":"child-a","missionId":"mission-approval","sessionId":"host-child:a","hostId":"host-a","hostAddress":"1.1.1.1","hostDisplayName":"Franklin","role":"host_child","task":"执行通用主机命令","currentStepTitle":"等待主机命令审批","status":"approval_required","planStepIds":["step-blocked"],"updatedAt":"2026-06-11T12:00:01Z"}
		]}
	}`)
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-hostops",
		SessionID:   "session-hostops",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeExecute,
		Metadata: map[string]string{
			"aiops.hostops.routeKind":    "host_ops",
			"aiops.hostops.missionId":    "mission-approval",
			"aiops.hostops.planRequired": "true",
			"aiops.hostops.planAccepted": "true",
		},
		Lifecycle: runtimekernel.TurnLifecycleRunning,
		StartedAt: now,
		UpdatedAt: now.Add(time.Second),
		AgentItems: []agentstate.TurnItem{
			{ID: "plan-hostops", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusBlocked, Payload: agentstate.PayloadEnvelope{Summary: "通用多主机运维计划", Data: planData}, CreatedAt: now},
			{ID: "spawn-hostops", Type: agentstate.TurnItemTypeToolResult, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Kind: "tool", Summary: "spawned host agent", Data: spawnData}, CreatedAt: now.Add(time.Second)},
		},
	}

	projected, err := projector.ProjectTurnSnapshot(state, turn)
	if err != nil {
		t.Fatalf("ProjectTurnSnapshot() error = %v", err)
	}
	if projected.Status != AiopsTransportStatusBlocked {
		t.Fatalf("transport status = %q, want blocked", projected.Status)
	}
	if projected.Turns["turn-hostops"].Status != AiopsTransportTurnStatusBlocked {
		t.Fatalf("turn status = %q, want blocked", projected.Turns["turn-hostops"].Status)
	}
	mission := projected.HostMissions["mission-approval"]
	if mission.Status != "waiting_approval" {
		t.Fatalf("mission status = %q, want waiting_approval", mission.Status)
	}
	if projected.ChildAgents["child-a"].Status != "approval_required" {
		t.Fatalf("child status = %q, want approval_required", projected.ChildAgents["child-a"].Status)
	}
}
