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

	tools := registry.AssembleTools("host", "inspect")
	for _, name := range []string{"evidence.record", "evidence.get", "evidence.link_incident"} {
		if !hasTool(tools, name) {
			t.Fatalf("missing %s in assembled tools", name)
		}
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
