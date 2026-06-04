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
		"title":"PostgreSQL 主从计划",
		"steps":[
			{"id":"prepare-primary","text":"准备主库","status":"completed"},
			{"id":"prepare-standby","text":"准备从库","status":"running"}
		]
	}`)
	spawnData := json.RawMessage(`{
		"toolCallId":"call-spawn-hosts",
		"toolName":"spawn_host_agent",
		"displayKind":"hostops.spawn_host_agent",
		"inputSummary":"为每台主机启动子 Agent",
		"outputSummary":"spawned 2 host agents",
		"outputPreview":{"children":[
			{"id":"child-a","missionId":"hostops:turn-hostops","sessionId":"host-child:a","hostId":"host-a","hostAddress":"1.1.1.1","hostDisplayName":"Franklin","role":"pg primary","task":"准备主库","status":"running","startedAt":"2026-06-04T10:00:01Z","updatedAt":"2026-06-04T10:00:02Z"},
			{"id":"child-b","missionId":"hostops:turn-hostops","sessionId":"host-child:b","hostId":"host-b","hostAddress":"1.1.1.2","hostDisplayName":"Harriet","role":"pg standby","task":"准备从库","status":"waiting","startedAt":"2026-06-04T10:00:01Z","updatedAt":"2026-06-04T10:00:02Z"}
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
			{ID: "plan-hostops", Type: agentstate.TurnItemTypePlan, Status: agentstate.ItemStatusRunning, Payload: agentstate.PayloadEnvelope{Summary: "PostgreSQL 主从计划", Data: planData}, CreatedAt: now},
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
	if len(mission.PlanSteps) != 2 || mission.PlanSteps[0].Text != "准备主库" {
		t.Fatalf("mission plan steps = %#v, want plan steps copied from process plan", mission.PlanSteps)
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
	subagentBlock := findTransportProcessBlock(t, projected.Turns["turn-hostops"].Process, AiopsTransportProcessKindSubagent)
	if subagentBlock.DisplayKind != "hostops.spawn_host_agent" || subagentBlock.Status != AiopsTransportProcessStatusCompleted {
		t.Fatalf("subagent block = %+v, want completed hostops spawn block", subagentBlock)
	}
}
