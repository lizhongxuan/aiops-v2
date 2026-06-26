package hostops

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
)

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
