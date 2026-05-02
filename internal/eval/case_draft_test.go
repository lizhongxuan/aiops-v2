package eval

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
)

func TestDraftCaseFromRunOutput(t *testing.T) {
	data, _ := json.Marshal(map[string]any{"iteration": 0})
	draft := DraftCaseFromRunOutput(DraftCaseInput{
		ID:       "draft-case",
		Category: "agent-debug",
		Input:    "检查 README 前 5 行",
		Output: RunOutput{
			Answer:    "README 前 5 行说明项目是 AIOps 后端。",
			ToolCalls: []ToolCall{{ID: "call-1", Name: "exec_command"}},
			TurnItems: []agentstate.TurnItem{{
				ID:      "model-0",
				Type:    agentstate.TurnItemTypeModelCall,
				Payload: agentstate.PayloadEnvelope{Data: data},
			}},
		},
	})
	if draft.Case.ID != "draft-case" {
		t.Fatalf("case id = %q", draft.Case.ID)
	}
	if draft.Case.Priority != "P1" {
		t.Fatalf("priority = %q, want P1", draft.Case.Priority)
	}
	if len(draft.Case.Expected.ExpectedToolCalls) != 1 || draft.Case.Expected.ExpectedToolCalls[0] != "exec_command" {
		t.Fatalf("expected tool calls = %#v", draft.Case.Expected.ExpectedToolCalls)
	}
	if !strings.Contains(draft.SidecarMarkdown, "README 前 5 行说明项目是 AIOps 后端") {
		t.Fatalf("sidecar markdown missing actual answer:\n%s", draft.SidecarMarkdown)
	}
}
