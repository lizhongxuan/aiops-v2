package runbooks

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
	core "aiops-v2/internal/runbooks"
	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsAddsReadOnlyRunbookPlanTools(t *testing.T) {
	catalog, err := core.LoadCatalog(filepath.Join("..", "..", "..", "runbooks", "erp", "*.yaml"))
	if err != nil {
		t.Fatalf("LoadCatalog() error = %v", err)
	}
	service := core.NewService(catalog, actionproposal.NewSigner([]byte("test-secret"), time.Now), core.NewInMemoryInstanceStore(), time.Now)
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry, service); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	tools := registry.AssembleTools("host", "plan")
	if len(tools) != 5 {
		t.Fatalf("AssembleTools() len = %d, want 5", len(tools))
	}
	for _, tool := range tools {
		if !tool.IsReadOnly(nil) {
			t.Fatalf("%s should be read-only", tool.Metadata().Name)
		}
		if tool.IsDestructive(nil) {
			t.Fatalf("%s should not be destructive", tool.Metadata().Name)
		}
	}

	matchTool := toolByName(t, tools, "runbook.match")
	result, err := matchTool.Execute(context.Background(), json.RawMessage(`{"symptom":"订单提交慢","service":"order-api"}`))
	if err != nil {
		t.Fatalf("runbook.match Execute() error = %v", err)
	}
	var body struct {
		Status     string           `json:"status"`
		Candidates []core.Candidate `json:"candidates"`
	}
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode match result: %v", err)
	}
	if body.Status != "ok" || len(body.Candidates) == 0 {
		t.Fatalf("match result = %#v, want ok candidates", body)
	}
}

func toolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return nil
}
