package runtimekernel

import (
	"encoding/json"
	"testing"
)

func TestOpsManualSearchDirectExecuteEnablesPreflightMetadata(t *testing.T) {
	raw, err := json.Marshal(map[string]any{
		"decision": "direct_execute",
		"manuals": []map[string]any{
			{"manual": map[string]any{"id": "manual-pg-backup"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	metadata := updateOpsManualFlowTurnMetadata(nil, ToolResult{
		Display: &ToolDisplayPayload{
			Type: opsManualSearchResultType,
			Data: raw,
		},
	})

	if metadata["opsManualMatched"] != "true" {
		t.Fatalf("metadata = %#v, want opsManualMatched", metadata)
	}
	if metadata["opsManualDirectExecute"] != "true" {
		t.Fatalf("metadata = %#v, want opsManualDirectExecute", metadata)
	}
	if !metadataListHas(metadata["enableTool"], opsManualPreflightToolName) {
		t.Fatalf("metadata = %#v, want preflight enabled for direct_execute", metadata)
	}
}

func metadataListHas(raw string, want string) bool {
	for _, value := range metadataListValuesForTest(raw) {
		if value == want {
			return true
		}
	}
	return false
}

func metadataListValuesForTest(raw string) []string {
	var out []string
	current := ""
	for _, r := range raw {
		switch r {
		case ',', ';', '\n', '\t', ' ':
			if current != "" {
				out = append(out, current)
				current = ""
			}
		default:
			current += string(r)
		}
	}
	if current != "" {
		out = append(out, current)
	}
	return out
}
