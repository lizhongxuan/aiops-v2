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
  "checkpoints": [{
    "id": "checkpoint-approval-1",
    "kind": "checkpoint_approval_waiting",
    "stepId": "step-approval",
    "turnId": "turn-1",
    "iteration": 0,
    "resumable": true,
    "targetRefs": ["service:checkout"],
    "evidenceRefs": ["evidence-coroot-1"],
    "approvalState": "waiting",
    "toolSurfaceSummary": "generic.metrics.read",
    "createdAt": "2026-05-02T00:00:00Z"
  }],
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
    "toolSearchEvents": [{
      "mode": "search",
      "query": "service metrics token=tool-search-secret",
      "ranker": "bm25",
      "matchCount": 1,
      "rejectedCount": 3,
      "targetCompatibility": "matched",
      "riskDecision": "allowed",
      "matchReasons": ["bm25", "target_compatible", "risk_allowed"],
      "request": {
        "intent": "rca",
        "targetRefs": ["service:checkout"],
        "requiredCaps": ["read"],
        "forbiddenCaps": ["execute"],
        "riskLevel": "low",
        "environmentFacts": ["checkout service p95 latency"],
        "mcpHealth": {"observability": "unavailable"}
      },
      "rejectedReasons": [
        {"toolName": "external.metrics.read", "reason": "mcp_unavailable password=search-secret", "mcpServerId": "observability", "healthStatus": "unavailable"},
        {"toolName": "host.logs", "reason": "target_incompatible"},
        {"toolName": "service.restart", "reason": "risk_exceeds_request"}
      ]
    }],
    "selectedTools": ["generic.metrics.read"],
    "rejectedToolReasons": [
      {"toolName": "external.metrics.read", "errorType": "mcp_unavailable", "reason": "api_key=reject-secret"}
    ]
  },
  "promptFingerprint": {"stableHash": "stable-hash"},
  "metadata": {
    "aiops.target.refs": "host:10.0.0.1,service:checkout",
    "aiops.env.readOnlyReason": "target_conflict_requires_clarification password=env-reason-secret",
    "aiops.env.compactContext": "EnvironmentFactsContext:\nConfirmedFacts:\n- host_identity=10.0.0.1 source=user_explicit\nConflictFacts:\n- target_conflict service:checkout -> host:10.0.0.2 token=env-compact-secret"
  },
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
	if len(trace.Checkpoints) != 1 ||
		trace.Checkpoints[0].ID != "checkpoint-approval-1" ||
		trace.Checkpoints[0].Kind != "checkpoint_approval_waiting" ||
		trace.Checkpoints[0].StepID != "step-approval" ||
		!trace.Checkpoints[0].Resumable ||
		trace.Checkpoints[0].ApprovalState != "waiting" ||
		len(trace.Checkpoints[0].TargetRefs) != 1 ||
		trace.Checkpoints[0].TargetRefs[0] != "service:checkout" ||
		len(trace.Checkpoints[0].EvidenceRefs) != 1 ||
		trace.Checkpoints[0].EvidenceRefs[0] != "evidence-coroot-1" {
		t.Fatalf("trace checkpoints = %#v, want approval checkpoint summary", trace.Checkpoints)
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
	if trace.EnvironmentContext == nil {
		t.Fatalf("trace environment context missing: %#v", trace)
	}
	if len(trace.EnvironmentContext.TargetRefs) != 2 ||
		trace.EnvironmentContext.TargetRefs[0] != "host:10.0.0.1" ||
		trace.EnvironmentContext.TargetRefs[1] != "service:checkout" ||
		!trace.EnvironmentContext.HasConflict ||
		!strings.Contains(trace.EnvironmentContext.ReadOnlyReason, "target_conflict_requires_clarification") ||
		!strings.Contains(trace.EnvironmentContext.CompactContext, "ConflictFacts") {
		t.Fatalf("trace environment context = %#v, want target refs and conflict facts", trace.EnvironmentContext)
	}
	if strings.Contains(trace.EnvironmentContext.ReadOnlyReason, "env-reason-secret") ||
		strings.Contains(trace.EnvironmentContext.CompactContext, "env-compact-secret") {
		t.Fatalf("trace environment context leaked secret: %#v", trace.EnvironmentContext)
	}
	if trace.ToolSurface.InitialToolCount != 2 ||
		trace.ToolSurface.DeferredFamilyCount != 1 ||
		trace.ToolSurface.LoadedToolCount != 1 ||
		trace.ToolSurface.FilteredToolCount != 1 ||
		trace.ToolSurface.ToolSearchEventCount != 1 ||
		trace.ToolSurface.RejectedToolCount != 4 ||
		trace.ToolSurface.MCPHealth["observability"] == "" {
		t.Fatalf("trace tool surface summary = %#v", trace.ToolSurface)
	}
	if len(trace.ToolSurface.ToolSearches) != 1 {
		t.Fatalf("trace tool search summaries = %#v, want 1", trace.ToolSurface.ToolSearches)
	}
	search := trace.ToolSurface.ToolSearches[0]
	if search.Query == "" || search.Ranker != "bm25" || search.Intent != "rca" || search.RejectedCount != 3 || search.RiskLevel != "low" {
		t.Fatalf("trace tool search summary = %#v, want query/ranker/intent/rejected count", search)
	}
	if len(search.TargetRefs) != 1 || search.TargetRefs[0] != "service:checkout" || len(search.RequiredCaps) != 1 || search.RequiredCaps[0] != "read" || len(search.ForbiddenCaps) != 1 || search.ForbiddenCaps[0] != "execute" {
		t.Fatalf("trace tool search request fields = %#v", search)
	}
	if search.TargetCompatibility != "matched" || search.RiskDecision != "allowed" || len(search.MatchReasons) < 3 {
		t.Fatalf("trace tool search impact fields = %#v", search)
	}
	if len(search.RejectedReasons) != 3 ||
		search.RejectedReasons[0].ToolName != "external.metrics.read" ||
		!strings.Contains(search.RejectedReasons[0].Reason, "mcp_unavailable") ||
		!strings.Contains(search.RejectedReasons[0].Reason, "[REDACTED]") {
		t.Fatalf("trace tool search rejected reasons = %#v", search.RejectedReasons)
	}
	if strings.Contains(trace.ToolSurface.MCPHealth["observability"], "service-pass") ||
		strings.Contains(trace.ToolSurface.FilteredReasons["external.metrics.read"], "service-secret") ||
		strings.Contains(trace.ToolSurface.ToolSearches[0].Query, "tool-search-secret") ||
		strings.Contains(trace.ToolSurface.ToolSearches[0].RejectedReasons[0].Reason, "search-secret") {
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

func TestPromptTraceServiceReturnsSetupHintWhenTraceDirectoryEmpty(t *testing.T) {
	root := t.TempDir()
	service := NewPromptTraceService(root)
	list, err := service.ListModelInputTraces(context.Background(), PromptTraceListRequest{Limit: 10})
	if err != nil {
		t.Fatalf("ListModelInputTraces() error = %v", err)
	}
	if len(list.Traces) != 0 {
		t.Fatalf("traces = %d, want 0 for empty trace root", len(list.Traces))
	}
	if !strings.Contains(list.SetupHint, "runtime settings") ||
		!strings.Contains(list.SetupHint, "Model Input Trace") ||
		!strings.Contains(list.SetupHint, root) {
		t.Fatalf("setup hint = %q, want runtime settings guidance and root path", list.SetupHint)
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
