package hostops

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aiops-v2/internal/tooling"
)

func TestManagerLifecycleToolsUseCoreNonMutationGovernance(t *testing.T) {
	tools := managerToolsByName(NewManagerTools(nil))
	expectedOperation := map[string]string{
		ToolSpawnHostAgent:       "spawn_child_agents",
		ToolSendHostAgentMessage: "send_message",
		ToolStopHostAgent:        "stop_child_agent",
	}
	for _, name := range []string{ToolSpawnHostAgent, ToolSendHostAgentMessage, ToolStopHostAgent} {
		tool := tools[name]
		if tool == nil {
			t.Fatalf("manager tool %q is missing", name)
		}
		meta := tool.Metadata()
		if meta.Layer != tooling.ToolLayerCore {
			t.Errorf("%s Layer = %q, want %q", name, meta.Layer, tooling.ToolLayerCore)
		}
		if meta.Mutating {
			t.Errorf("%s Mutating = true, want false for manager-only lifecycle control", name)
		}
		if meta.RiskLevel != tooling.ToolRiskLow {
			t.Errorf("%s RiskLevel = %q, want %q", name, meta.RiskLevel, tooling.ToolRiskLow)
		}
		if meta.RequiresApproval {
			t.Errorf("%s RequiresApproval = true, want false", name)
		}
		discovery := meta.EffectiveDiscovery()
		if discovery.CapabilityKind != "orchestration_control" {
			t.Errorf("%s CapabilityKind = %q, want orchestration_control", name, discovery.CapabilityKind)
		}
		if len(discovery.OperationKinds) != 1 || discovery.OperationKinds[0] != expectedOperation[name] {
			t.Errorf("%s OperationKinds = %v, want [%s]", name, discovery.OperationKinds, expectedOperation[name])
		}
		if tool.IsReadOnly(nil) {
			t.Errorf("%s IsReadOnly = true, want false because it changes child lifecycle state", name)
		}
	}
}

func TestWaitManagerToolRemainsTypedReadCapability(t *testing.T) {
	tool := managerToolsByName(NewManagerTools(nil))[ToolWaitHostAgents]
	if tool == nil {
		t.Fatal("wait manager tool is missing")
	}
	meta := tool.Metadata()
	discovery := meta.EffectiveDiscovery()
	if meta.Mutating || meta.RequiresApproval || meta.RiskLevel != tooling.ToolRiskLow {
		t.Fatalf("wait governance = mutating:%t approval:%t risk:%q, want non-mutating low-risk without approval", meta.Mutating, meta.RequiresApproval, meta.RiskLevel)
	}
	if discovery.CapabilityKind != "read" || discovery.PermissionScope != "read" {
		t.Fatalf("wait discovery = capability:%q permission:%q, want read/read", discovery.CapabilityKind, discovery.PermissionScope)
	}
	if len(discovery.OperationKinds) != 1 || discovery.OperationKinds[0] != "read" {
		t.Fatalf("wait OperationKinds = %v, want [read]", discovery.OperationKinds)
	}
	if !tool.IsReadOnly(nil) {
		t.Fatal("wait IsReadOnly = false, want true")
	}
}

func TestSpawnManagerToolStillRequiresAcceptedPlanBeforeOrchestration(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	if err := store.SaveMission(ctx, HostOperationMission{
		ID:           "mission-unaccepted",
		PlanRequired: true,
		PlanAccepted: false,
		Mentions:     []HostMention{{HostID: "host-a", Resolved: true}},
	}); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	orchestrator := NewOrchestrator(store, NewInMemoryTranscriptStore(), &fakeChildSpawner{})
	tool := managerToolsByName(NewManagerTools(orchestrator))[ToolSpawnHostAgent]

	_, err := tool.Execute(ctx, json.RawMessage(`{
		"missionId":"mission-unaccepted",
		"assignments":[{"hostId":"host-a","task":"inspect host a"}]
	}`))
	if !errors.Is(err, ErrPlanNotAccepted) {
		t.Fatalf("spawn Execute() error = %v, want ErrPlanNotAccepted", err)
	}
}

func TestChildAgentResultContractTransportCompatibleJSONRoundTrip(t *testing.T) {
	startedAt := time.Date(2026, 7, 12, 1, 2, 3, 0, time.UTC)
	updatedAt := startedAt.Add(time.Minute)
	completedAt := updatedAt.Add(time.Minute)
	child := HostChildAgent{
		ID:              "child-a",
		MissionID:       "mission-a",
		ParentAgentID:   "manager-a",
		SessionID:       "host-child:a",
		HostID:          "host-a",
		HostAddress:     "10.0.0.1",
		HostDisplayName: "Franklin",
		Role:            "host_child",
		Task:            "inspect host a",
		Status:          HostChildAgentStatusCompleted,
		PlanStepIDs:     []string{"step-a"},
		StartedAt:       startedAt,
		UpdatedAt:       updatedAt,
		CompletedAt:     &completedAt,
	}

	data, err := json.Marshal(childAgentResultContracts([]HostChildAgent{child}, true))
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	var roundTrip []struct {
		ID              string     `json:"id"`
		ChildAgentID    string     `json:"childAgentId"`
		MissionID       string     `json:"missionId"`
		ParentAgentID   string     `json:"parentAgentId"`
		SessionID       string     `json:"sessionId"`
		HostID          string     `json:"hostId"`
		HostAddress     string     `json:"hostAddress"`
		HostDisplayName string     `json:"hostDisplayName"`
		Role            string     `json:"role"`
		Task            string     `json:"task"`
		Status          string     `json:"status"`
		PlanStepIDs     []string   `json:"planStepIds"`
		StartedAt       time.Time  `json:"startedAt"`
		UpdatedAt       time.Time  `json:"updatedAt"`
		CompletedAt     *time.Time `json:"completedAt"`
		NoHostMutation  bool       `json:"noHostMutation"`
	}
	if err := json.Unmarshal(data, &roundTrip); err != nil {
		t.Fatalf("Unmarshal() error = %v\n%s", err, data)
	}
	if len(roundTrip) != 1 {
		t.Fatalf("roundTrip len = %d, want 1", len(roundTrip))
	}
	got := roundTrip[0]
	if got.ID != child.ID || got.ChildAgentID != child.ID || got.MissionID != child.MissionID || got.ParentAgentID != child.ParentAgentID || got.SessionID != child.SessionID {
		t.Fatalf("identity fields = %#v, want transport identity plus childAgentId alias", got)
	}
	if got.HostID != child.HostID || got.HostAddress != child.HostAddress || got.HostDisplayName != child.HostDisplayName {
		t.Fatalf("host fields = %#v, want %#v", got, child)
	}
	if got.Role != child.Role || got.Task != child.Task || got.Status != string(child.Status) || len(got.PlanStepIDs) != 1 || got.PlanStepIDs[0] != "step-a" {
		t.Fatalf("execution fields = %#v, want role/task/status/planStepIds", got)
	}
	if !got.StartedAt.Equal(startedAt) || !got.UpdatedAt.Equal(updatedAt) || got.CompletedAt == nil || !got.CompletedAt.Equal(completedAt) || !got.NoHostMutation {
		t.Fatalf("timestamps/guarantee = %#v, want complete JSON round trip", got)
	}
}

func TestHostAgentFullRuntimeManagerToolsReturnChildContracts(t *testing.T) {
	ctx := context.Background()
	store := NewInMemoryMissionStore()
	transcripts := NewInMemoryTranscriptStore()
	orchestrator := NewOrchestrator(store, transcripts, &fakeChildSpawner{})
	mission := HostOperationMission{
		ID:             "mission-tools",
		ManagerAgentID: "manager-tools",
		PlanRequired:   true,
		PlanAccepted:   true,
		Mentions: []HostMention{{
			HostID:      "host-a",
			Address:     "10.0.0.1",
			DisplayName: "host-a",
			Resolved:    true,
		}},
	}
	if err := store.SaveMission(ctx, mission); err != nil {
		t.Fatalf("SaveMission() error = %v", err)
	}
	tools := managerToolsByName(NewManagerTools(orchestrator))

	spawnResult, err := tools[ToolSpawnHostAgent].Execute(ctx, json.RawMessage(`{
		"missionId":"mission-tools",
		"assignments":[{"hostId":"host-a","hostAddress":"10.0.0.1","task":"inspect host a","planStepId":"step-a"}]
	}`))
	if err != nil {
		t.Fatalf("spawn Execute() error = %v", err)
	}
	var spawnPayload struct {
		SchemaVersion string `json:"schemaVersion"`
		Children      []struct {
			ChildAgentID   string `json:"childAgentId"`
			TargetRef      string `json:"targetRef"`
			Status         string `json:"status"`
			NoHostMutation bool   `json:"noHostMutation"`
		} `json:"children"`
	}
	if err := json.Unmarshal([]byte(spawnResult.Content), &spawnPayload); err != nil {
		t.Fatalf("spawn result is not JSON: %v\n%s", err, spawnResult.Content)
	}
	if spawnPayload.SchemaVersion != "aiops.hostops.child/v1" || len(spawnPayload.Children) != 1 {
		t.Fatalf("spawn payload = %#v, want child schema with one child", spawnPayload)
	}
	child := spawnPayload.Children[0]
	if child.ChildAgentID == "" || child.TargetRef != "host-a" || !child.NoHostMutation {
		t.Fatalf("spawn child = %#v, want targetRef and noHostMutation guarantee", child)
	}

	waitResult, err := tools[ToolWaitHostAgents].Execute(ctx, json.RawMessage(`{"missionId":"mission-tools"}`))
	if err != nil {
		t.Fatalf("wait Execute() error = %v", err)
	}
	var waitPayload struct {
		SchemaVersion string `json:"schemaVersion"`
		Children      []struct {
			ChildAgentID string   `json:"childAgentId"`
			TargetRef    string   `json:"targetRef"`
			Status       string   `json:"status"`
			EvidenceRefs []string `json:"evidenceRefs"`
			BlockerRefs  []string `json:"blockerRefs"`
		} `json:"children"`
	}
	if err := json.Unmarshal([]byte(waitResult.Content), &waitPayload); err != nil {
		t.Fatalf("wait result is not JSON: %v\n%s", err, waitResult.Content)
	}
	if waitPayload.SchemaVersion != "aiops.hostops.wait/v1" || len(waitPayload.Children) != 1 {
		t.Fatalf("wait payload = %#v, want wait schema with one child", waitPayload)
	}
	if waitPayload.Children[0].ChildAgentID == "" || waitPayload.Children[0].TargetRef != "host-a" {
		t.Fatalf("wait child = %#v, want child result contract", waitPayload.Children[0])
	}
	if waitPayload.Children[0].EvidenceRefs == nil || waitPayload.Children[0].BlockerRefs == nil {
		t.Fatalf("wait child = %#v, want explicit evidenceRefs/blockerRefs arrays", waitPayload.Children[0])
	}
}

func managerToolsByName(tools []tooling.Tool) map[string]tooling.Tool {
	out := map[string]tooling.Tool{}
	for _, tool := range tools {
		out[tool.Metadata().Name] = tool
	}
	return out
}
