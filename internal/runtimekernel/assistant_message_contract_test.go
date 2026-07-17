package runtimekernel

import (
	"encoding/json"
	"testing"
	"time"

	"aiops-v2/internal/agentstate"
)

func TestAssistantMessagePayloadDoesNotExposeCandidateFields(t *testing.T) {
	payload := assistantMessageAgentItemData(assistantMessageData{
		MessageID:        "msg-1",
		Iteration:        2,
		Phase:            AssistantMessagePhaseFinalAnswer,
		StreamState:      AssistantMessageStreamStateComplete,
		EvidenceBoundary: "limited",
		BoundaryAction:   FinalMessageBoundaryConstrain,
		Duration:         1500 * time.Millisecond,
	})
	forbidden := []string{"candidateForFinal", "candidateState", "answerState", "supersededByIteration"}
	for _, key := range forbidden {
		if _, ok := payload[key]; ok {
			t.Fatalf("payload must not contain legacy candidate field %q: %#v", key, payload)
		}
	}
	if payload["phase"] != "final_answer" || payload["streamState"] != "complete" || payload["evidenceBoundary"] != "limited" {
		t.Fatalf("payload = %#v, want final_answer complete limited", payload)
	}
}

func TestUnclassifiedAssistantDraftSurvivesSnapshotRoundTrip(t *testing.T) {
	snapshot := &TurnSnapshot{SessionID: "session-round-trip", ID: "turn-round-trip"}
	upsertAssistantMessageItem(snapshot, assistantMessageItemID(snapshot.ID, 0), agentstate.ItemStatusRunning, "partial", unclassifiedAssistantMessageData(assistantMessageData{
		Iteration: 0,
	}, AssistantMessageStreamStateStreaming))

	raw, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatalf("marshal snapshot: %v", err)
	}
	var restored TurnSnapshot
	if err := json.Unmarshal(raw, &restored); err != nil {
		t.Fatalf("unmarshal snapshot: %v", err)
	}
	if len(restored.AgentItems) != 1 {
		t.Fatalf("restored agent items = %#v, want one assistant draft", restored.AgentItems)
	}
	payload := agentItemPayloadMap(restored.AgentItems[0])
	if payload["phase"] != "unclassified" || payload["streamState"] != "streaming" {
		t.Fatalf("restored payload = %#v, want unclassified streaming", payload)
	}
}

func TestAssistantMessagePayloadRepresentsUnclassifiedStreamingDraft(t *testing.T) {
	payload := assistantMessageAgentItemData(assistantMessageData{
		MessageID:   "msg-draft",
		Iteration:   1,
		Phase:       AssistantMessagePhaseUnclassified,
		StreamState: AssistantMessageStreamStateStreaming,
	})

	if payload["phase"] != "unclassified" || payload["streamState"] != "streaming" {
		t.Fatalf("payload = %#v, want unclassified streaming draft", payload)
	}
	if _, ok := payload["boundaryAction"]; ok {
		t.Fatalf("payload = %#v, unclassified draft must not claim a terminal boundary action", payload)
	}
}
