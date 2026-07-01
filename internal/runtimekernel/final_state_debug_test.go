package runtimekernel

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/agentstate"
)

func TestDebugAssistantMessageFactsIncludesStructuredFields(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"displayKind": "assistant.message",
		"phase":       "final_answer",
		"streamState": "streaming",
	})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := &TurnSnapshot{
		AgentItems: []agentstate.TurnItem{
			{
				ID:     "message-1",
				Type:   agentstate.TurnItemTypeAssistantMessage,
				Status: agentstate.ItemStatusRunning,
				Payload: agentstate.PayloadEnvelope{
					Summary: "正在形成根因分析",
					Data:    data,
				},
			},
		},
	}

	fields := debugAssistantMessageFacts(snapshot, "message-1", "最终回答正文", map[string]any{
		"finalContract":       "final",
		"finalEvidenceAction": "allow",
		"commitAllowed":       true,
	})

	for _, key := range []string{
		"assistantMessageID",
		"assistantMessageType",
		"assistantMessagePhase",
		"assistantMessageStreamState",
		"assistantMessageHash",
		"finalContract",
		"finalEvidenceAction",
		"commitAllowed",
	} {
		if _, ok := fields[key]; !ok {
			t.Fatalf("debug fields missing %q: %#v", key, fields)
		}
	}
	if fields["assistantMessageID"] != "message-1" {
		t.Fatalf("assistantMessageID = %#v, want message-1", fields["assistantMessageID"])
	}
	if fields["assistantMessageType"] != string(agentstate.TurnItemTypeAssistantMessage) {
		t.Fatalf("assistantMessageType = %#v, want assistant_message", fields["assistantMessageType"])
	}
	if fields["assistantMessagePhase"] != "final_answer" || fields["assistantMessageStreamState"] != "streaming" {
		t.Fatalf("assistant message fields = %#v, want final_answer/streaming", fields)
	}
	if fields["assistantMessageHash"] != debugTextHash("最终回答正文") {
		t.Fatalf("assistantMessageHash = %#v, want %s", fields["assistantMessageHash"], debugTextHash("最终回答正文"))
	}
}
