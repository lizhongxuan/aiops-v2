package appui

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptTraceServiceListsAndReadsTraceFiles(t *testing.T) {
	root := t.TempDir()
	traceDir := filepath.Join(root, "sess-1", "turn-1")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace dir: %v", err)
	}
	jsonPath := filepath.Join(traceDir, "iteration-000-20260502T000000.000000000Z.json")
	mdPath := filepath.Join(traceDir, "iteration-000-20260502T000000.000000000Z.md")
	diffPath := filepath.Join(traceDir, "iteration-000-20260502T000000.000000000Z.diff.md")
	if err := os.WriteFile(jsonPath, []byte(`{
  "kind": "runtime_model_input",
  "createdAt": "2026-05-02T00:00:00Z",
  "sessionId": "sess-1",
  "turnId": "turn-1",
  "iteration": 0,
  "caseId": "case-1",
  "visibleTools": ["exec_command"],
  "toolSurfaceTrace": {
    "initialTools": ["exec_command", "tool_search"],
    "baseRegistryCount": 2,
    "deferredFamilies": [
      {"pack": "external_metrics", "mcpServerId": "observability", "healthStatus": "unavailable", "toolCount": 4}
    ],
    "loadedTools": ["generic.metrics.read"],
    "loadedPacks": ["generic_metrics"],
    "filteredTools": [
      {"toolName": "external.metrics.read", "reason": "mcp_unavailable token=service-secret"}
    ],
    "mcpHealth": {"observability": "unavailable: https://user:service-pass@metrics.example.internal/api"},
    "toolSearchEvents": [{"mode": "search", "matchCount": 1}],
    "selectedTools": ["generic.metrics.read"],
    "rejectedToolReasons": [
      {"toolName": "external.metrics.read", "errorType": "mcp_unavailable", "reason": "api_key=reject-secret"}
    ]
  },
  "promptFingerprint": {"stableHash": "stable-hash"},
  "llmRequests": [
    {"id": "llm-1", "usage": {"prompt_tokens": 21, "completion_tokens": 8, "total_tokens": 29}, "duration_ms": 456},
    {"id": "llm-2", "usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}, "duration_ms": 544}
  ],
  "modelInput": [
    {"providerRole": "system"},
    {"providerRole": "user", "content": "检查 checkout p95 延迟 token=super-secret"},
    {"role": "user", "content": "再次查看 payment 状态 password=super-secret"}
  ]
}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
	if err := os.WriteFile(mdPath, []byte("# Model Input Trace\n\nprompt"), 0o644); err != nil {
		t.Fatalf("write markdown: %v", err)
	}
	if err := os.WriteFile(diffPath, []byte("# Prompt Input Trace Diff"), 0o644); err != nil {
		t.Fatalf("write diff: %v", err)
	}

	service := NewPromptTraceService(root)
	list, err := service.ListModelInputTraces(context.Background(), PromptTraceListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListModelInputTraces() error = %v", err)
	}
	if len(list.Traces) != 1 {
		t.Fatalf("traces = %d, want 1", len(list.Traces))
	}
	trace := list.Traces[0]
	if trace.SessionID != "sess-1" || trace.TurnID != "turn-1" || trace.CaseID != "case-1" || trace.MessageCount != 3 {
		t.Fatalf("trace metadata = %#v", trace)
	}
	if trace.MarkdownPath == "" || trace.DiffPath == "" {
		t.Fatalf("trace paths missing markdown/diff: %#v", trace)
	}
	if trace.LLMRequestCount != 2 || trace.AverageDurationMs != 500 {
		t.Fatalf("trace llm stats = count %d avg %d, want 2 and 500", trace.LLMRequestCount, trace.AverageDurationMs)
	}
	if trace.Usage == nil || trace.Usage.PromptTokens != 31 || trace.Usage.CompletionTokens != 13 || trace.Usage.TotalTokens != 44 {
		t.Fatalf("trace usage = %#v, want summed token usage", trace.Usage)
	}
	if trace.ToolSurface == nil {
		t.Fatalf("trace tool surface summary missing: %#v", trace)
	}
	if trace.ToolSurface.InitialToolCount != 2 ||
		trace.ToolSurface.DeferredFamilyCount != 1 ||
		trace.ToolSurface.LoadedToolCount != 1 ||
		trace.ToolSurface.FilteredToolCount != 1 ||
		trace.ToolSurface.RejectedToolCount != 1 ||
		trace.ToolSurface.MCPHealth["observability"] == "" {
		t.Fatalf("trace tool surface summary = %#v", trace.ToolSurface)
	}
	if strings.Contains(trace.ToolSurface.MCPHealth["observability"], "service-pass") ||
		strings.Contains(trace.ToolSurface.FilteredReasons["external.metrics.read"], "service-secret") {
		t.Fatalf("trace tool surface summary leaked secret: %#v", trace.ToolSurface)
	}
	if !strings.Contains(trace.UserPromptPreview, "再次查看 payment 状态") {
		t.Fatalf("user prompt preview = %q, want latest turn user message", trace.UserPromptPreview)
	}
	if strings.Contains(trace.UserPromptPreview, "super-secret") {
		t.Fatalf("user prompt preview leaked secret: %q", trace.UserPromptPreview)
	}

	file, err := service.GetModelInputTraceFile(context.Background(), PromptTraceFileRequest{Path: trace.MarkdownPath})
	if err != nil {
		t.Fatalf("GetModelInputTraceFile() error = %v", err)
	}
	if file.Format != "markdown" || file.Content != "# Model Input Trace\n\nprompt" {
		t.Fatalf("file = %#v", file)
	}
}

func TestPromptTraceServiceAllowsLargeHistoryWindow(t *testing.T) {
	if got := normalizePromptTraceLimit(2000); got != 2000 {
		t.Fatalf("normalizePromptTraceLimit(2000) = %d, want 2000", got)
	}
	if got := normalizePromptTraceLimit(5000); got != 2000 {
		t.Fatalf("normalizePromptTraceLimit(5000) = %d, want max 2000", got)
	}
}

func TestPromptTraceServiceFiltersByCaseAndSelectsTrace(t *testing.T) {
	root := t.TempDir()
	for _, fixture := range []struct {
		session string
		caseID  string
		stamp   string
	}{
		{session: "sess-a", caseID: "case-a", stamp: "20260502T000000.000000000Z"},
		{session: "sess-b", caseID: "case-b", stamp: "20260502T000001.000000000Z"},
	} {
		traceDir := filepath.Join(root, fixture.session, "turn-1")
		if err := os.MkdirAll(traceDir, 0o755); err != nil {
			t.Fatalf("mkdir trace dir: %v", err)
		}
		jsonPath := filepath.Join(traceDir, "iteration-000-"+fixture.stamp+".json")
		if err := os.WriteFile(jsonPath, []byte(`{
  "createdAt": "2026-05-02T00:00:00Z",
  "sessionId": "`+fixture.session+`",
  "turnId": "turn-1",
  "caseId": "`+fixture.caseID+`",
  "modelInput": [{"providerRole": "user"}]
}`), 0o644); err != nil {
			t.Fatalf("write json: %v", err)
		}
	}

	service := NewPromptTraceService(root)
	list, err := service.ListModelInputTraces(context.Background(), PromptTraceListRequest{
		Limit:  10,
		CaseID: "case-b",
		Trace:  "sess-b/turn-1/iteration-000-20260502T000001.000000000Z.json",
	})
	if err != nil {
		t.Fatalf("ListModelInputTraces() error = %v", err)
	}
	if len(list.Traces) != 1 || list.Traces[0].CaseID != "case-b" {
		t.Fatalf("filtered traces = %#v", list.Traces)
	}
	if list.SelectedID != list.Traces[0].ID {
		t.Fatalf("selectedID = %q, want %q", list.SelectedID, list.Traces[0].ID)
	}
}

func TestPromptTraceServiceRejectsEscapingPath(t *testing.T) {
	root := t.TempDir()
	service := NewPromptTraceService(root)
	if _, err := service.GetModelInputTraceFile(context.Background(), PromptTraceFileRequest{Path: "../secret.md"}); err == nil {
		t.Fatal("GetModelInputTraceFile() succeeded for path traversal")
	}
}
