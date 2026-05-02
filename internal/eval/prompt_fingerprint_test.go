package eval

import (
	"encoding/json"
	"testing"

	"aiops-v2/internal/agentstate"
)

func TestScoreCaseCarriesPromptFingerprints(t *testing.T) {
	data, err := json.Marshal(map[string]any{
		"iteration": 0,
		"promptFingerprint": map[string]string{
			"version":       "prompt-fingerprint-v1",
			"stableHash":    "stable-hash",
			"developerHash": "developer-hash",
		},
	})
	if err != nil {
		t.Fatalf("marshal data: %v", err)
	}
	score := ScoreCase(Case{ID: "case-1", Category: "prompt"}, RunOutput{
		Answer: "ok 验证方式 go test ./internal/eval",
		TurnItems: []agentstate.TurnItem{{
			ID:      "model-0",
			Type:    agentstate.TurnItemTypeModelCall,
			Status:  agentstate.ItemStatusCompleted,
			Payload: agentstate.PayloadEnvelope{Data: data},
		}},
	})
	if len(score.PromptFingerprints) != 1 {
		t.Fatalf("prompt fingerprints = %d, want 1", len(score.PromptFingerprints))
	}
	if score.PromptFingerprints[0]["developerHash"] != "developer-hash" {
		t.Fatalf("prompt fingerprint = %#v", score.PromptFingerprints[0])
	}
}
