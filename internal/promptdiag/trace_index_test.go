package promptdiag

import (
	"path/filepath"
	"testing"
)

func TestTraceIndexLookupByMarkdownAndFindByCase(t *testing.T) {
	root := filepath.Join(t.TempDir(), "traces")
	jsonPath := filepath.Join(root, "eval-run-my-case", "turn-1", "iteration-000.json")
	writeTestJSON(t, jsonPath, map[string]any{
		"createdAt":    "2026-05-03T00:00:00Z",
		"sessionId":    "eval-run-my-case",
		"turnId":       "turn-1",
		"iteration":    0,
		"visibleTools": []string{"exec_command"},
		"promptFingerprint": map[string]string{
			"stableHash": "stable",
		},
		"prompt": map[string]string{
			"stable": "abc",
		},
		"modelInput": []map[string]string{{"providerRole": "user", "content": "hello"}},
	})
	index, err := BuildTraceIndex(root)
	if err != nil {
		t.Fatalf("BuildTraceIndex: %v", err)
	}
	trace, ok := index.Lookup("eval-run-my-case/turn-1/iteration-000.md")
	if !ok {
		t.Fatalf("Lookup by markdown failed")
	}
	if !trace.HasUserMessage || trace.PromptSizeChars != 3 || trace.StableHash != "stable" {
		t.Fatalf("trace = %#v", trace)
	}
	found := index.FindByCaseID("my-case")
	if len(found) != 1 {
		t.Fatalf("FindByCaseID found %d, want 1", len(found))
	}
}
