package modeltrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/promptinput"
)

func TestWriteLocksJSONAndMarkdownTraceSchema(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnabledEnv, "1")
	t.Setenv(DirEnv, dir)

	path, err := Write(Request{
		Kind:         "runtime_model_input",
		SessionID:    "sess-1",
		TurnID:       "turn-1",
		Iteration:    1,
		VisibleTools: []string{"read_file"},
		Prompt: Prompt{
			StableHash: "stable-hash",
			Dynamic:    "dynamic delta",
		},
		ModelInput: []*schema.Message{
			{
				Role:    schema.System,
				Content: "developer instructions",
				Extra: map[string]any{
					"semantic_role": "developer",
					"prompt_layer":  "developer",
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if path == "" {
		t.Fatal("Write returned empty path")
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	var payload struct {
		SchemaVersion int `json:"schemaVersion"`
		ModelInput    []struct {
			ProviderRole string `json:"providerRole"`
			SemanticRole string `json:"semanticRole"`
			PromptLayer  string `json:"promptLayer"`
		} `json:"modelInput"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatalf("unmarshal json trace: %v", err)
	}
	if payload.SchemaVersion != 1 {
		t.Fatalf("schemaVersion = %d, want 1", payload.SchemaVersion)
	}
	if len(payload.ModelInput) != 1 {
		t.Fatalf("modelInput length = %d, want 1", len(payload.ModelInput))
	}
	msg := payload.ModelInput[0]
	if msg.ProviderRole != "system" || msg.SemanticRole != "developer" || msg.PromptLayer != "developer" {
		t.Fatalf("trace message roles = %#v, want provider=system semantic=developer layer=developer", msg)
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "- Schema: `1`") {
		t.Fatalf("markdown trace missing schema version:\n%s", string(markdown))
	}
}

func TestWriteDisabledReturnsEmptyPath(t *testing.T) {
	t.Setenv(EnabledEnv, "")
	t.Setenv(DirEnv, t.TempDir())

	path, err := Write(Request{Kind: "runtime_model_input"})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	if path != "" {
		t.Fatalf("Write() path = %q, want empty when disabled", path)
	}
}

func TestWriteIncludesPromptInputTraceAndDiffMarkdown(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnabledEnv, "true")
	t.Setenv(DirEnv, dir)

	diff := promptinput.DiffTrace(
		promptinput.PromptInputTrace{Items: []promptinput.TraceItem{{Source: "conversation", SemanticRole: "user", Content: "old"}}},
		promptinput.PromptInputTrace{Items: []promptinput.TraceItem{{Source: "conversation", SemanticRole: "tool_result", ID: "call-1", Content: "token=secret-value"}}},
	)
	path, err := Write(Request{
		Kind:      "runtime model input",
		TraceID:   "trace/with spaces",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		PromptInputTrace: promptinput.PromptInputTrace{Items: []promptinput.TraceItem{
			{Source: "memory", SemanticRole: "memory", ID: "mem-1", Content: "prior note"},
		}},
		PromptInputDiff: &diff,
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	if !strings.Contains(string(data), "promptInputTrace") || !strings.Contains(string(data), "mem-1") {
		t.Fatalf("json trace missing prompt input trace:\n%s", string(data))
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "## Prompt Input Trace") || !strings.Contains(string(markdown), "memory") {
		t.Fatalf("markdown trace missing prompt input trace:\n%s", string(markdown))
	}

	diffMarkdown, err := os.ReadFile(filepath.Join(filepath.Dir(path), "input.diff.md"))
	if err != nil {
		t.Fatalf("read input.diff.md: %v", err)
	}
	if !strings.Contains(string(diffMarkdown), "tool_result") || strings.Contains(string(diffMarkdown), "secret-value") {
		t.Fatalf("diff markdown missing semantic delta or leaked secret:\n%s", string(diffMarkdown))
	}
}

func TestWriteIncludesPromptFingerprintSummary(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnabledEnv, "true")
	t.Setenv(DirEnv, dir)

	path, err := Write(Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		PromptFingerprint: map[string]string{
			"version":       "prompt-fingerprint-v1",
			"stableHash":    "stable-hash",
			"developerHash": "developer-hash",
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	if !strings.Contains(string(data), `"promptFingerprint"`) || !strings.Contains(string(data), `"developerHash": "developer-hash"`) {
		t.Fatalf("json trace missing prompt fingerprint:\n%s", string(data))
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "- Prompt fingerprint: `stable-hash`") {
		t.Fatalf("markdown trace missing prompt fingerprint summary:\n%s", string(markdown))
	}
}
