package opsgraph

import (
	"context"
	"encoding/json"
	"testing"

	graph "aiops-v2/internal/opsgraph"
	"aiops-v2/internal/tooling"
)

func TestRegisterBuiltinsAddsReadOnlyOpsGraphTools(t *testing.T) {
	store := manualOpsGraphStoreForTest()
	registry := tooling.NewRegistry()
	if err := RegisterBuiltins(registry, store); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}
	if tools := registry.AssembleTools("host", "inspect"); len(tools) != 0 {
		t.Fatalf("default AssembleTools() = %v, want opsgraph deferred by default", toolNamesForOpsGraphTest(tools))
	}
	tools := registry.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{EnabledPacks: []string{"opsgraph"}})
	if len(tools) != 4 {
		t.Fatalf("AssembleToolsWithOptions(opsgraph) len = %d, want 4", len(tools))
	}
	if chatTools := registry.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{EnabledPacks: []string{"opsgraph"}}); len(chatTools) != 4 {
		t.Fatalf("chat opsgraph tools = %v, want 4 tools when pack is enabled", toolNamesForOpsGraphTest(chatTools))
	}
	for _, tool := range tools {
		meta := tool.Metadata()
		if meta.Layer != tooling.ToolLayerDeferred || meta.Pack != "opsgraph" || !meta.DeferByDefault {
			t.Fatalf("%s metadata = layer:%q pack:%q defer:%v, want deferred opsgraph", meta.Name, meta.Layer, meta.Pack, meta.DeferByDefault)
		}
		discovery := meta.EffectiveDiscovery()
		if discovery.DiscoveryGroup != "opsgraph" || discovery.LoadingPolicy != tooling.ToolLoadingPolicyDeferred || !discovery.RequiresSelect {
			t.Fatalf("%s discovery = %+v, want deferred opsgraph select-only discovery", meta.Name, discovery)
		}
		for _, want := range []string{"opsgraph", "service", "dependency"} {
			if !containsOpsGraphString(discovery.ResourceTypes, want) {
				t.Fatalf("%s resource types = %#v, missing %q", meta.Name, discovery.ResourceTypes, want)
			}
		}
		if len(discovery.OperationKinds) == 0 {
			t.Fatalf("%s operation kinds empty in discovery metadata", meta.Name)
		}
		if !tool.IsReadOnly(nil) {
			t.Fatalf("%s should be read-only", tool.Metadata().Name)
		}
		if tool.IsDestructive(nil) {
			t.Fatalf("%s should not be destructive", tool.Metadata().Name)
		}
	}

	lookup := toolByName(t, tools, "opsgraph.lookup")
	result, err := lookup.Execute(context.Background(), json.RawMessage(`{"query":"订单服务"}`))
	if err != nil {
		t.Fatalf("lookup Execute() error = %v", err)
	}
	var body struct {
		Status  string         `json:"status"`
		Matches []graph.Entity `json:"matches"`
	}
	if err := json.Unmarshal([]byte(result.Content), &body); err != nil {
		t.Fatalf("decode lookup result: %v", err)
	}
	if body.Status != "ok" || len(body.Matches) == 0 {
		t.Fatalf("lookup result = %#v, want ok matches", body)
	}

	impact := toolByName(t, tools, "opsgraph.business_impact")
	result, err = impact.Execute(context.Background(), json.RawMessage(`{"entityId":"service.order-api"}`))
	if err != nil {
		t.Fatalf("business_impact Execute() error = %v", err)
	}
	var impactBody struct {
		Status string               `json:"status"`
		Impact graph.BusinessImpact `json:"impact"`
	}
	if err := json.Unmarshal([]byte(result.Content), &impactBody); err != nil {
		t.Fatalf("decode impact result: %v", err)
	}
	if impactBody.Status != "ok" || len(impactBody.Impact.Capabilities) != 1 || impactBody.Impact.Capabilities[0].ID != "business.order-submit" {
		t.Fatalf("impact result = %#v, want business.order-submit", impactBody)
	}

	runbooks := toolByName(t, tools, "opsgraph.related_runbooks")
	result, err = runbooks.Execute(context.Background(), json.RawMessage(`{"entityId":"service.order-api"}`))
	if err != nil {
		t.Fatalf("related_runbooks Execute() error = %v", err)
	}
	var runbookBody struct {
		Status   string               `json:"status"`
		Runbooks []graph.RunbookMatch `json:"runbooks"`
	}
	if err := json.Unmarshal([]byte(result.Content), &runbookBody); err != nil {
		t.Fatalf("decode runbook result: %v", err)
	}
	if runbookBody.Status != "ok" || len(runbookBody.Runbooks) != 1 || runbookBody.Runbooks[0].Runbook.ID != "workflow.order-restart" {
		t.Fatalf("runbook result = %#v, want workflow.order-restart", runbookBody)
	}
}

func manualOpsGraphStoreForTest() *graph.Store {
	record := graph.GraphRecord{
		ID:        "graph.default",
		Name:      "默认图谱",
		IsDefault: true,
		Nodes: []graph.Node{
			{ID: "service.order-api", Type: graph.NodeService, Name: "order-api", Aliases: []string{"订单服务"}},
			{ID: "business.order-submit", Type: graph.NodeBusiness, Name: "订单提交"},
			{ID: "workflow.order-restart", Type: graph.NodeWorkflow, Name: "订单服务重启 Workflow"},
		},
		Edges: []graph.Edge{
			{ID: "e1", From: "service.order-api", Type: graph.RelAffects, To: "business.order-submit"},
			{ID: "e2", From: "service.order-api", Type: graph.RelHandledBy, To: "workflow.order-restart", Reason: "服务重启和回滚"},
		},
	}
	return graph.CompileGraphStore(record)
}

func toolNamesForOpsGraphTest(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
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

func containsOpsGraphString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
