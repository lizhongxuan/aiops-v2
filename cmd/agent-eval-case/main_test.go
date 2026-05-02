package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/eval"
)

func TestRunCLIMissingOutReturnsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{}, &stdout, &stderr)
	if code != 2 {
		t.Fatalf("exit code = %d, want 2; stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stderr.String(), "-out is required") {
		t.Fatalf("stderr missing out requirement:\n%s", stderr.String())
	}
}

func TestRunCLIGeneratesDraftCaseAndSidecar(t *testing.T) {
	dir := t.TempDir()
	answerPath := filepath.Join(dir, "answer.txt")
	toolCallsPath := filepath.Join(dir, "tool_calls.json")
	turnItemsPath := filepath.Join(dir, "turn_items.json")
	outPath := filepath.Join(dir, "draft-case.json")

	writeTextFile(t, answerPath, "实际答案包含 README 结论。")
	writeJSONFile(t, toolCallsPath, []eval.ToolCall{{ID: "call-1", Name: "read_file"}})
	writeJSONFile(t, turnItemsPath, []agentstate.TurnItem{{
		ID:      "model-0",
		Type:    agentstate.TurnItemTypeModelCall,
		Payload: agentstate.PayloadEnvelope{Data: json.RawMessage(`{"iteration":0}`)},
	}})

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-id", "draft-case",
		"-category", "agent-debug",
		"-input", "检查 README",
		"-answer-file", answerPath,
		"-tool-calls-file", toolCallsPath,
		"-turn-items-file", turnItemsPath,
		"-out", outPath,
	}, &stdout, &stderr)
	if code != 0 {
		t.Fatalf("exit code = %d, stderr=%s", code, stderr.String())
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read draft case: %v", err)
	}
	var c eval.Case
	if err := json.Unmarshal(data, &c); err != nil {
		t.Fatalf("unmarshal draft case: %v", err)
	}
	if c.Priority != "P1" {
		t.Fatalf("priority = %q, want P1", c.Priority)
	}
	if len(c.Expected.ExpectedToolCalls) != 1 || c.Expected.ExpectedToolCalls[0] != "read_file" {
		t.Fatalf("expected tool calls = %#v", c.Expected.ExpectedToolCalls)
	}
	sidecar, err := os.ReadFile(outPath + ".draft.md")
	if err != nil {
		t.Fatalf("read sidecar: %v", err)
	}
	sidecarText := string(sidecar)
	for _, want := range []string{"实际答案包含 README 结论", "Human Review Required", "expected.mustInclude"} {
		if !strings.Contains(sidecarText, want) {
			t.Fatalf("sidecar missing %q:\n%s", want, sidecarText)
		}
	}
	if !strings.Contains(stdout.String(), outPath) {
		t.Fatalf("stdout missing output path:\n%s", stdout.String())
	}
}

func writeTextFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir: %v", err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
