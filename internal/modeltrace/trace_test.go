package modeltrace

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/diagnostics"
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

func TestWriteIncludesPromptInputTraceBudgetMetrics(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnabledEnv, "true")
	t.Setenv(DirEnv, dir)

	path, err := Write(Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-ops",
		TurnID:    "turn-ops",
		PromptInputTrace: promptinput.PromptInputTrace{
			OpsContextCapsuleChars: 512,
			SessionFactCount:       5,
			LettaHintCount:         2,
			MemoryItemCount:        3,
			VisibleOpsManualTools:  []string{"search_ops_manuals"},
			DroppedContextReasons:  []string{"letta_hint_limit"},
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	for _, want := range []string{`"opsContextCapsuleChars": 512`, `"sessionFactCount": 5`, `"visibleOpsManualTools"`, `"letta_hint_limit"`} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("json trace missing %q:\n%s", want, string(data))
		}
	}
	markdown, err := os.ReadFile(strings.TrimSuffix(path, filepath.Ext(path)) + ".md")
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "ops_context_capsule_chars") || !strings.Contains(string(markdown), "letta_hint_limit") {
		t.Fatalf("markdown trace missing budget metrics:\n%s", string(markdown))
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
		Metadata:  map[string]string{"eval.caseId": "case-1"},
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
	if !strings.Contains(string(data), `"promptFingerprint"`) || !strings.Contains(string(data), `"developerHash": "developer-hash"`) || !strings.Contains(string(data), `"caseId": "case-1"`) {
		t.Fatalf("json trace missing prompt fingerprint:\n%s", string(data))
	}
	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	if !strings.Contains(string(markdown), "- Prompt fingerprint: `stable-hash`") || !strings.Contains(string(markdown), "- Eval case: `case-1`") {
		t.Fatalf("markdown trace missing prompt fingerprint summary:\n%s", string(markdown))
	}
}

func TestWriteIncludesDiagnosticTraceAndRedactsSecrets(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnabledEnv, "true")
	t.Setenv(DirEnv, dir)

	path, err := Write(Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		DiagnosticTrace: diagnostics.DiagnosticTrace{
			ScopeHash:        "scope-redis",
			ScopeSummary:     "redis redis://:secret@127.0.0.1:6379/0",
			Hypotheses:       []string{"redis unavailable"},
			ObservedEvidence: []string{"PING timeout"},
			RefutingEvidence: []string{"container is running"},
			MissingEvidence:  []string{"need api key sk-test-value"},
			ToolFailures: []diagnostics.ToolFailure{{
				ToolName: "exec_command",
				Semantic: diagnostics.ToolFailurePolicyBlocked,
				Detail:   "policy blocked token=plain-token",
				Critical: true,
			}},
			ManualBindingID:  "manual-redis",
			Confidence:       diagnostics.ConfidenceLow,
			ConfidenceReason: "sensitive value was present in failed probe",
		},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read json trace: %v", err)
	}
	jsonText := string(data)
	for _, want := range []string{`"diagnosticTrace"`, `"scopeHash": "scope-redis"`, `"manualBindingId": "manual-redis"`, `"semantic": "policy_blocked"`} {
		if !strings.Contains(jsonText, want) {
			t.Fatalf("json trace missing %q:\n%s", want, jsonText)
		}
	}
	for _, forbidden := range []string{"secret", "sk-test-value", "plain-token"} {
		if strings.Contains(jsonText, forbidden) {
			t.Fatalf("json trace leaked %q:\n%s", forbidden, jsonText)
		}
	}

	markdownPath := strings.TrimSuffix(path, filepath.Ext(path)) + ".md"
	markdown, err := os.ReadFile(markdownPath)
	if err != nil {
		t.Fatalf("read markdown trace: %v", err)
	}
	md := string(markdown)
	for _, want := range []string{"## Diagnostic Trace", "scope-redis", "redis unavailable", "PING timeout", "policy_blocked", "low"} {
		if !strings.Contains(md, want) {
			t.Fatalf("markdown trace missing %q:\n%s", want, md)
		}
	}
	for _, forbidden := range []string{"secret", "sk-test-value", "plain-token"} {
		if strings.Contains(md, forbidden) {
			t.Fatalf("markdown trace leaked %q:\n%s", forbidden, md)
		}
	}
}

func TestWriteRedactsSecretsFromPromptModelInputAndToolCalls(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(EnabledEnv, "true")
	t.Setenv(DirEnv, dir)

	path, err := Write(Request{
		Kind:      "runtime_model_input",
		SessionID: "sess-1",
		TurnID:    "turn-1",
		Prompt: Prompt{
			Dynamic: "## Runtime Environment Context\nCurrentFocus: target=redis dsn=redis://:secret-pass@127.0.0.1:6379/0",
		},
		ModelInput: []*schema.Message{
			{Role: schema.User, Content: "连接串 redis://:secret-pass@127.0.0.1:6379/0 帮我排查"},
			{Role: schema.Assistant, ToolCalls: []schema.ToolCall{{
				ID:   "call-1",
				Type: "function",
				Function: schema.FunctionCall{
					Name:      "host_exec",
					Arguments: `{"cmd":"redis-cli -a secret-pass PING","token":"plain-token"}`,
				},
			}}},
		},
		PromptInputTrace: promptinput.PromptInputTrace{Items: []promptinput.TraceItem{{
			Source:       "conversation",
			SemanticRole: "user",
			Content:      "redis password=secret-pass",
		}}},
	})
	if err != nil {
		t.Fatalf("Write() error = %v", err)
	}

	for _, filePath := range []string{path, strings.TrimSuffix(path, filepath.Ext(path)) + ".md"} {
		data, err := os.ReadFile(filePath)
		if err != nil {
			t.Fatalf("read %s: %v", filePath, err)
		}
		text := string(data)
		for _, forbidden := range []string{"secret-pass", "plain-token"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s leaked %q:\n%s", filePath, forbidden, text)
			}
		}
		if !strings.Contains(text, "[REDACTED]") {
			t.Fatalf("%s missing redaction marker:\n%s", filePath, text)
		}
	}
}
