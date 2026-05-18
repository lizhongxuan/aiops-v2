package evidence

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	core "aiops-v2/internal/evidence"
	"aiops-v2/internal/tooling"
)

func TestEvidenceRecordToolReturnsEvidenceRef(t *testing.T) {
	svc := core.NewService(core.NewInMemoryStore(), time.Now)
	tool := NewRecordTool(svc)

	input := json.RawMessage(`{
		"incidentId":"inc-redis-1",
		"sourceTool":"k8s.get_events",
		"source":"kubernetes",
		"kind":"event",
		"summary":"No OOM events found in last 30m"
	}`)
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, `"evidenceRef"`) {
		t.Fatalf("result = %s", result.Content)
	}
	if !tool.IsReadOnly(input) {
		t.Fatal("evidence.record should be read-only from production-change perspective")
	}
	if tool.IsDestructive(input) {
		t.Fatal("evidence.record should not be destructive")
	}
}

func TestEvidenceGetAndLinkIncidentTools(t *testing.T) {
	svc := core.NewService(core.NewInMemoryStore(), fixedClock())
	rec, err := svc.Record(context.Background(), core.RecordRequest{
		IncidentID: "inc-redis-1",
		Kind:       core.KindMetric,
		Summary:    "RSS grows faster than used_memory",
	})
	if err != nil {
		t.Fatal(err)
	}

	getTool := NewGetTool(svc)
	getResult, err := getTool.Execute(context.Background(), json.RawMessage(`{"evidenceRef":"`+rec.Ref+`"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(getResult.Content, rec.Ref) || !strings.Contains(getResult.Content, rec.Summary) {
		t.Fatalf("get result = %s, want evidence record", getResult.Content)
	}

	linkTool := NewLinkIncidentTool(svc)
	linkResult, err := linkTool.Execute(context.Background(), json.RawMessage(`{"incidentId":"inc-redis-2","evidenceRefs":["`+rec.Ref+`"],"relation":"supports"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(linkResult.Content, `"status":"ok"`) {
		t.Fatalf("link result = %s, want ok", linkResult.Content)
	}
}

func TestRegisterBuiltinsAddsEvidenceTools(t *testing.T) {
	registry := tooling.NewRegistry()
	svc := core.NewService(core.NewInMemoryStore(), fixedClock())

	if err := RegisterBuiltins(registry, svc); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	if tools := registry.AssembleTools("host", "inspect"); len(tools) != 0 {
		t.Fatalf("default evidence tools = %v, want no evidence tools in first surface", evidenceToolNames(tools))
	}
	readTools := registry.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{EnabledPacks: []string{"evidence_read"}})
	if !hasTool(readTools, "evidence.get") {
		t.Fatalf("missing evidence.get when evidence_read pack is enabled: %v", evidenceToolNames(readTools))
	}
	for _, name := range []string{"evidence.record", "evidence.link_incident"} {
		tool := mustEvidenceTool(t, registry, name)
		meta := tool.Metadata()
		if meta.Layer != tooling.ToolLayerInternal {
			t.Fatalf("%s layer = %q, want internal", name, meta.Layer)
		}
		if hasTool(readTools, name) {
			t.Fatalf("%s should remain hidden even when evidence_read pack is enabled", name)
		}
	}
	getMeta := mustEvidenceTool(t, registry, "evidence.get").Metadata()
	if getMeta.Layer != tooling.ToolLayerDeferred || getMeta.Pack != "evidence_read" || !getMeta.DeferByDefault {
		t.Fatalf("evidence.get metadata = layer:%q pack:%q defer:%v, want deferred evidence_read", getMeta.Layer, getMeta.Pack, getMeta.DeferByDefault)
	}
}

func fixedClock() func() time.Time {
	return func() time.Time {
		return time.Date(2026, 5, 15, 10, 30, 0, 0, time.UTC)
	}
}

func hasTool(tools []tooling.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return true
		}
	}
	return false
}

func mustEvidenceTool(t *testing.T, registry *tooling.Registry, name string) tooling.Tool {
	t.Helper()
	tool, ok := registry.Get(name)
	if !ok {
		t.Fatalf("missing registered tool %s", name)
	}
	return tool
}

func evidenceToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
}
