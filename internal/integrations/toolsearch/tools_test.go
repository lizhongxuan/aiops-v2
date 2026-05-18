package toolsearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestToolSearchToolIsReadOnlyAndReturnsToolMatches(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, fakeStaticTool("coroot.service_metrics", "Get service metrics"))
	mustRegister(t, registry, fakeStaticTool("k8s.get_events", "Read Kubernetes events"))

	tool := NewToolSearchTool(registry)
	input := json.RawMessage(`{"query":"redis metrics","limit":5}`)

	if !tool.IsReadOnly(input) || tool.IsDestructive(input) {
		t.Fatal("tool_search should be read-only")
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Content, "coroot.service_metrics") {
		t.Fatalf("result = %s, want coroot.service_metrics", result.Content)
	}
}

func TestToolSearchOmitsRemovedAndInternalTools(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, fakeStaticTool("runbook.match", "old runbook"))
	mustRegister(t, registry, fakeStaticTool("fallback.plan_exec", "old fallback"))
	mustRegister(t, registry, fakeStaticTool("erp.business_metric", "mock erp"))
	mustRegister(t, registry, fakeStaticTool("update_plan", "internal plan"))
	mustRegister(t, registry, fakeStaticTool("changes.recent_deployments", "deployment changes"))

	result := runToolSearch(t, registry, "plan metric changes")
	for _, forbidden := range []string{"runbook.match", "fallback.plan_exec", "erp.business_metric", "update_plan"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("tool_search returned forbidden tool %q: %s", forbidden, result)
		}
	}
	if !strings.Contains(result, "changes.recent_deployments") {
		t.Fatalf("tool_search should return allowed changes tool: %s", result)
	}
}

func TestToolSearchReturnsGovernanceMetadata(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             "k8s.scale_workload",
			Description:      "Scale workload",
			Mock:             true,
			Domain:           "kubernetes",
			RiskLevel:        tooling.ToolRiskHigh,
			Mutating:         true,
			RequiresApproval: true,
		},
	})

	content := runToolSearch(t, registry, "scale workload")
	for _, want := range []string{`"mock":true`, `"domain":"kubernetes"`, `"riskLevel":"high"`, `"mutating":true`, `"requiresApproval":true`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}
}

func mustRegister(t *testing.T, registry *tooling.Registry, tool tooling.Tool) {
	t.Helper()
	if err := registry.Register(tool); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
}

func fakeStaticTool(name, description string) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: description,
		},
	}
}

func runToolSearch(t *testing.T, registry *tooling.Registry, query string) string {
	t.Helper()
	tool := NewToolSearchTool(registry)
	input, err := json.Marshal(map[string]any{"query": query, "limit": 10})
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result.Content
}
