package runtimekernel

import (
	"testing"
	"time"
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
