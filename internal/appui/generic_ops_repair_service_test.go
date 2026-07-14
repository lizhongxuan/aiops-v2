package appui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func TestGenericOpsRepairCommitsDeterministicPlanThroughRuntimeKernel(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	kernel := runtimekernel.NewRuntimeKernel(runtimekernel.RuntimeKernelConfig{Sessions: sessions})
	service := NewChatService(kernel, sessions, NewAgentEventService(nil))

	result, err := service.SendMessage(context.Background(), ChatCommand{
		SessionID:       "sess-generic-ops-system-turn",
		ClientTurnID:    "client-turn-generic-ops-system-turn",
		ClientMessageID: "client-message-generic-ops-system-turn",
		Content:         "主机A和主机B的PG主从集群异常，请帮忙恢复，数据可以不要，只需要PG主从集群正常运行，pg_mon部署在主机C。",
		Metadata: map[string]string{
			"aiops.genericOpsRepairDraftOnly": "true",
		},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	if result.Status != "completed" || !strings.Contains(result.Output, "stateful_middleware_cluster_repair") {
		t.Fatalf("result = %#v, want fixed completed generic repair draft", result)
	}
	session := sessions.Get(result.SessionID)
	if session == nil || session.CurrentTurn == nil {
		t.Fatal("generic repair did not persist a runtime turn")
	}
	snapshot := session.CurrentTurn
	if snapshot.SystemTurn == nil || snapshot.SystemTurn.Kind != runtimekernel.SystemTurnKindDeterministicPlan ||
		snapshot.SystemTurn.ContractStatus != runtimekernel.FinalContractStatusNeedsEvidence {
		t.Fatalf("SystemTurn = %#v, want deterministic plan requiring evidence", snapshot.SystemTurn)
	}
	if snapshot.FinalOutput != result.Output {
		t.Fatalf("FinalOutput = %q, want fixed response %q", snapshot.FinalOutput, result.Output)
	}

	counts := map[agentstate.TurnItemType]int{}
	artifacts := map[string]agentstate.TurnItemType{}
	for _, item := range snapshot.AgentItems {
		counts[item.Type]++
		switch item.Type {
		case agentstate.TurnItemTypeModelCall, agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
			t.Fatalf("generic system turn fabricated model/tool item: %#v", item)
		case agentstate.TurnItemTypeEvidenceCollected, agentstate.TurnItemTypePlan:
			var payload struct {
				ArtifactType string `json:"artifactType"`
			}
			if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
				t.Fatalf("decode evidence artifact %s: %v", item.ID, err)
			}
			if payload.ArtifactType != "" {
				artifacts[payload.ArtifactType] = item.Type
			}
		}
	}
	for typ, want := range map[agentstate.TurnItemType]int{
		agentstate.TurnItemTypeUserMessage:       1,
		agentstate.TurnItemTypePlan:              3,
		agentstate.TurnItemTypeEvidence:          1,
		agentstate.TurnItemTypeEvidenceCollected: 1,
		agentstate.TurnItemTypeAssistantMessage:  1,
		agentstate.TurnItemTypeFinalResponse:     1,
	} {
		if counts[typ] != want {
			t.Fatalf("item type %s count = %d, want %d; items=%#v", typ, counts[typ], want, snapshot.AgentItems)
		}
	}
	for artifactType, wantType := range map[string]agentstate.TurnItemType{
		"ops_manual_search":   agentstate.TurnItemTypeEvidenceCollected,
		"read_only_preflight": agentstate.TurnItemTypePlan,
		"host_probe_plan":     agentstate.TurnItemTypePlan,
	} {
		if artifacts[artifactType] != wantType {
			t.Fatalf("artifacts = %#v, want %q typed as %q", artifacts, artifactType, wantType)
		}
	}
}
