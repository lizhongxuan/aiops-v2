package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
)

func TestMigrateAssistantMessageCLIConvertsEnvelopeInput(t *testing.T) {
	input := []byte(`{
		"finalOutput": "最终回答。",
		"agentItems": [
			{"id":"progress-1","type":"assistant_progress","status":"completed","payload":{"summary":"我先查资料。"}},
			{"id":"answer-1","type":"assistant_answer","status":"completed","payload":{"summary":"旧候选答案","data":{"answerState":"superseded"}}},
			{"id":"final-1","type":"final_answer","status":"completed","payload":{"summary":"最终回答。"}}
		]
	}`)
	var out bytes.Buffer
	if err := runCLI(nil, bytes.NewReader(input), &out, &bytes.Buffer{}); err != nil {
		t.Fatalf("runCLI() error = %v", err)
	}
	var items []agentstate.TurnItem
	if err := json.Unmarshal(out.Bytes(), &items); err != nil {
		t.Fatalf("decode output: %v\n%s", err, out.String())
	}
	if len(items) != 2 {
		t.Fatalf("items = %#v, want commentary and final assistant_message", items)
	}
	for _, item := range items {
		if item.Type != agentstate.TurnItemTypeAssistantMessage {
			t.Fatalf("item = %#v, want assistant_message", item)
		}
	}
	if !strings.Contains(out.String(), `"phase": "commentary"`) || !strings.Contains(out.String(), `"phase": "final_answer"`) {
		t.Fatalf("output missing migrated phases:\n%s", out.String())
	}
}

func TestMigrateAssistantMessageCLIRejectsEmptyInput(t *testing.T) {
	err := runCLI(nil, strings.NewReader(" "), &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "input JSON is required") {
		t.Fatalf("runCLI() error = %v, want input required error", err)
	}
}
