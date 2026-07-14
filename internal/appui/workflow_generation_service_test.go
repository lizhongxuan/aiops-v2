package appui

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/workflowgen"
)

func TestWorkflowGenerationCommitsPlanAndDraftThroughRuntimeKernel(t *testing.T) {
	sessions := runtimekernel.NewSessionManager()
	kernel := runtimekernel.NewRuntimeKernel(runtimekernel.RuntimeKernelConfig{Sessions: sessions})
	service := NewWorkflowGenerationChatService(
		sessions,
		workflowgen.NewMemorySessionStore(),
		workflowgen.DeterministicPlanBuilder{},
		workflowgen.RunnerGraphGenerator{},
		NewAgentEventService(nil),
	).WithSystemTurnGateway(kernel)

	initialRequest := runtimekernel.TurnRequest{
		SessionType:     runtimekernel.SessionTypeHost,
		Mode:            runtimekernel.ModeChat,
		SessionID:       "sess-workflow-generation-system-turn",
		TurnID:          "turn-workflow-generation-plan",
		ClientTurnID:    "client-turn-workflow-generation-plan",
		ClientMessageID: "client-message-workflow-generation-plan",
		Input:           "@add_workflow 每天早上8点自动抓取AI行业新闻，提取三条关键内容直接返回给我",
		HostID:          "server-local",
	}
	initial, handled, err := service.Handle(context.Background(), ChatCommand{SessionID: initialRequest.SessionID}, initialRequest)
	if err != nil {
		t.Fatalf("initial Handle() error = %v", err)
	}
	if !handled || initial.Status != "completed" || !strings.Contains(initial.Output, "初始生成大纲") {
		t.Fatalf("initial response = %#v, handled=%v, want fixed completed workflow plan", initial, handled)
	}
	planSnapshot := sessions.Get(initialRequest.SessionID).CurrentTurn
	assertWorkflowGenerationSystemTurn(t, planSnapshot, runtimekernel.SystemTurnKindDeterministicPlan, runtimekernel.FinalContractStatusNeedsEvidence, map[agentstate.TurnItemType]int{
		agentstate.TurnItemTypeUserMessage:      1,
		agentstate.TurnItemTypeRouteSelected:    1,
		agentstate.TurnItemTypePlan:             2,
		agentstate.TurnItemTypeEvidence:         1,
		agentstate.TurnItemTypeAssistantMessage: 1,
		agentstate.TurnItemTypeFinalResponse:    1,
	})
	assertWorkflowGenerationTypedFacts(t, planSnapshot, "plan")

	confirmationRequest := runtimekernel.TurnRequest{
		SessionType:     runtimekernel.SessionTypeHost,
		Mode:            runtimekernel.ModeChat,
		SessionID:       initialRequest.SessionID,
		TurnID:          "turn-workflow-generation-draft",
		ClientTurnID:    "client-turn-workflow-generation-draft",
		ClientMessageID: "client-message-workflow-generation-draft",
		Input:           "确认生成工作流候选：AI 新闻摘要工作流",
		HostID:          "server-local",
		Metadata:        map[string]string{"opsManualAction": "generate_runner_workflow_candidate"},
	}
	confirmation, handled, err := service.Handle(context.Background(), ChatCommand{
		SessionID: confirmationRequest.SessionID,
		Metadata:  confirmationRequest.Metadata,
	}, confirmationRequest)
	if err != nil {
		t.Fatalf("confirmation Handle() error = %v", err)
	}
	if !handled || confirmation.Status != "completed" || !strings.Contains(confirmation.Output, "Runner Workflow 草稿已生成") {
		t.Fatalf("confirmation response = %#v, handled=%v, want fixed completed draft", confirmation, handled)
	}
	session := sessions.Get(confirmationRequest.SessionID)
	assertWorkflowGenerationSystemTurn(t, session.CurrentTurn, runtimekernel.SystemTurnKindDeterministicArtifact, runtimekernel.FinalContractStatusPartial, map[agentstate.TurnItemType]int{
		agentstate.TurnItemTypeUserMessage:       1,
		agentstate.TurnItemTypeRouteSelected:     1,
		agentstate.TurnItemTypeEvidenceCollected: 1,
		agentstate.TurnItemTypeAssistantMessage:  1,
		agentstate.TurnItemTypeFinalResponse:     1,
	})
	assertWorkflowGenerationTypedFacts(t, session.CurrentTurn, "draft_generation")
	retainedPlan := false
	turnIDs := make([]string, 0, len(session.TurnHistory))
	for _, turn := range session.TurnHistory {
		turnIDs = append(turnIDs, turn.ID)
		retainedPlan = retainedPlan || turn.ID == initialRequest.TurnID
	}
	if !retainedPlan {
		t.Fatalf("turn history ids = %#v, want the plan turn retained", turnIDs)
	}
}

func assertWorkflowGenerationSystemTurn(t *testing.T, snapshot *runtimekernel.TurnSnapshot, kind runtimekernel.SystemTurnKind, status runtimekernel.FinalContractStatus, wantCounts map[agentstate.TurnItemType]int) {
	t.Helper()
	if snapshot == nil || snapshot.SystemTurn == nil {
		t.Fatal("workflow generation did not persist a system turn")
	}
	if snapshot.SystemTurn.Kind != kind || snapshot.SystemTurn.ContractStatus != status {
		t.Fatalf("SystemTurn = %#v, want %s/%s", snapshot.SystemTurn, kind, status)
	}
	counts := map[agentstate.TurnItemType]int{}
	for _, item := range snapshot.AgentItems {
		counts[item.Type]++
		switch item.Type {
		case agentstate.TurnItemTypeModelCall, agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
			t.Fatalf("workflow generation fabricated model/tool item: %#v", item)
		}
	}
	for itemType, want := range wantCounts {
		if counts[itemType] != want {
			t.Fatalf("item type %s count = %d, want %d; items=%#v", itemType, counts[itemType], want, snapshot.AgentItems)
		}
	}
}

func assertWorkflowGenerationTypedFacts(t *testing.T, snapshot *runtimekernel.TurnSnapshot, wantStage string) {
	t.Helper()
	var routeStage string
	var artifactSchema string
	for _, item := range snapshot.AgentItems {
		switch item.Type {
		case agentstate.TurnItemTypeRouteSelected:
			var payload struct {
				Stage string `json:"stage"`
			}
			if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
				t.Fatalf("decode route item: %v", err)
			}
			routeStage = payload.Stage
		case agentstate.TurnItemTypePlan, agentstate.TurnItemTypeEvidenceCollected:
			var payload struct {
				ArtifactType string `json:"artifactType"`
				Payload      struct {
					SchemaVersion string `json:"schemaVersion"`
				} `json:"payload"`
			}
			if err := json.Unmarshal(item.Payload.Data, &payload); err != nil {
				t.Fatalf("decode workflow artifact item: %v", err)
			}
			if payload.ArtifactType == "runner_workflow_generation" {
				artifactSchema = payload.Payload.SchemaVersion
			}
		}
	}
	if routeStage != wantStage {
		t.Fatalf("route stage = %q, want %q", routeStage, wantStage)
	}
	if artifactSchema != "aiops.runner_workflow_generation/v1" {
		t.Fatalf("artifact schema = %q, want typed workflow artifact", artifactSchema)
	}
}
