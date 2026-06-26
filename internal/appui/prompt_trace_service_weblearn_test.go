package appui

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestPromptTraceServiceSeparatesWebLearnExternalKnowledge(t *testing.T) {
	root := t.TempDir()
	traceDir := filepath.Join(root, "sess-web", "turn-web")
	if err := os.MkdirAll(traceDir, 0o755); err != nil {
		t.Fatalf("mkdir trace dir: %v", err)
	}
	jsonPath := filepath.Join(traceDir, "iteration-000-20260623T130000.000000000Z.json")
	if err := os.WriteFile(jsonPath, []byte(`{
  "kind": "runtime_model_input",
  "createdAt": "2026-06-23T13:00:00Z",
  "sessionId": "sess-web",
  "turnId": "turn-web",
  "iteration": 0,
  "webLearnEvidence": [{
    "id": "web-redis-latency",
    "kind": "external_knowledge",
    "query": "redis 7.2 latency doctor official docs",
    "sourceUrl": "https://redis.io/docs/latest/commands/latency-doctor/",
    "sourceTitle": "LATENCY DOCTOR",
    "sourceKind": "official_docs",
    "product": "Redis",
    "version": "7.2",
    "retrievedAt": "2026-06-23T13:00:00Z",
    "relevantExcerpt": "Redis official docs describe LATENCY DOCTOR output semantics.",
    "applicability": "Applies only to Redis latency subsystem command semantics.",
    "confidence": "high"
  }],
  "modelInput": [{"providerRole": "user", "content": "Redis latency doctor 输出是什么意思"}]
}`), 0o644); err != nil {
		t.Fatalf("write json: %v", err)
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
	if len(trace.WebLearnEvidence) != 1 {
		t.Fatalf("WebLearnEvidence = %#v, want one external knowledge item", trace.WebLearnEvidence)
	}
	ev := trace.WebLearnEvidence[0]
	if ev.Kind != "external_knowledge" ||
		ev.SourceKind != "official_docs" ||
		ev.SourceURL != "https://redis.io/docs/latest/commands/latency-doctor/" ||
		ev.Confidence != "high" {
		t.Fatalf("WebLearn evidence = %#v", ev)
	}
	if trace.ToolSurface != nil {
		t.Fatalf("ToolSurface = %#v, want WebLearn evidence separated from tool surface", trace.ToolSurface)
	}
}
