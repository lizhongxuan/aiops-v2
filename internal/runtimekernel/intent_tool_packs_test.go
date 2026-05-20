package runtimekernel

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/tooling"
)

func TestRunTurn_EnablesDeferredPacksFromTurnIntent(t *testing.T) {
	cases := []struct {
		name      string
		input     string
		wantTools []string
		forbidden []string
	}{
		{
			name:      "chinese rca",
			input:     "分析 checkout 服务最近 30 分钟延迟升高的根因",
			wantTools: []string{"coroot.list_services", "coroot.service_metrics", "coroot.rca_report", "coroot.service_topology"},
			forbidden: []string{"coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "named service abnormality",
			input:     "分析 aiops-host-agent 异常情况",
			wantTools: []string{"coroot.list_services", "coroot.service_metrics", "coroot.rca_report", "coroot.service_topology"},
			forbidden: []string{"coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot cpu chart",
			input:     "查看 aiops-host-agent 的 cpu 图表",
			wantTools: []string{"coroot.list_services", "coroot.service_metrics"},
			forbidden: []string{"coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "business impact",
			input:     "order-api 故障会影响哪些业务能力和租户？",
			wantTools: []string{"opsgraph.business_impact"},
			forbidden: []string{"coroot.service_metrics", "list_mcp_resources"},
		},
		{
			name:      "mcp resource",
			input:     "读取 MCP resource ops://manuals/redis",
			wantTools: []string{"list_mcp_resources", "read_mcp_resource"},
			forbidden: []string{"coroot.service_metrics", "opsgraph.business_impact"},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("ok", nil)}}
			registry := tooling.NewRegistry()
			for _, toolDef := range intentPackRuntimeTestTools() {
				if err := registry.Register(toolDef); err != nil {
					t.Fatalf("Register tool failed: %v", err)
				}
			}
			source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
			compiler := newRecordingCompiler()
			kernel, _ := newKernelForLoopTests(t, source, compiler, model)

			result, err := kernel.RunTurn(context.Background(), TurnRequest{
				SessionID:   "sess-intent-" + tc.name,
				SessionType: SessionTypeHost,
				Mode:        ModeChat,
				TurnID:      "turn-intent-" + tc.name,
				Input:       tc.input,
			})
			if err != nil {
				t.Fatalf("RunTurn failed: %v", err)
			}
			if result.Status != "completed" {
				t.Fatalf("result status = %q, want completed", result.Status)
			}
			if len(compiler.contexts) != 1 {
				t.Fatalf("compiler contexts = %d, want 1", len(compiler.contexts))
			}
			names := toolNames(compiler.contexts[0].AssembledTools)
			for _, want := range tc.wantTools {
				if !containsString(names, want) {
					t.Fatalf("tools = %v, want %s", names, want)
				}
			}
			for _, forbidden := range tc.forbidden {
				if containsString(names, forbidden) {
					t.Fatalf("tools = %v, should not include %s", names, forbidden)
				}
			}
		})
	}
}

func TestRunTurn_EnablesMCPResourcePackAfterToolSearchHit(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-tool-search-resource",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tool_search",
						Arguments: `{"query":"redis resource"}`,
					},
				},
			}),
			schema.AssistantMessage("resource tools available", nil),
		},
	}
	registry := tooling.NewRegistry()
	for _, toolDef := range []tooling.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{Name: "tool_search", Layer: tooling.ToolLayerCore},
			ReadOnlyFunc: func(json.RawMessage) bool {
				return true
			},
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				return tooling.ToolResult{Content: `{"matches":[{"kind":"pack","name":"mcp_resource","tools":["list_mcp_resources","read_mcp_resource"]}]}`}, nil
			},
		},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "list_mcp_resources", Layer: tooling.ToolLayerDeferred, Pack: "mcp_resource", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_mcp_resource", Layer: tooling.ToolLayerDeferred, Pack: "mcp_resource", DeferByDefault: true}},
	} {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-tool-search-resource",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-tool-search-resource",
		Input:       "查一下 redis 相关资源",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 2 {
		t.Fatalf("compiler contexts = %d, want 2", len(compiler.contexts))
	}
	first := toolNames(compiler.contexts[0].AssembledTools)
	if !containsString(first, "tool_search") || containsString(first, "list_mcp_resources") || containsString(first, "read_mcp_resource") {
		t.Fatalf("first iteration tools = %v, want tool_search only for resource discovery", first)
	}
	second := toolNames(compiler.contexts[1].AssembledTools)
	for _, want := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if !containsString(second, want) {
			t.Fatalf("second iteration tools = %v, want %s after tool_search pack hit", second, want)
		}
	}
}

func intentPackRuntimeTestTools() []tooling.Tool {
	return []tooling.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_services", Layer: tooling.ToolLayerCore}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.service_metrics", Layer: tooling.ToolLayerDeferred, Pack: "coroot_rca", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.rca_report", Layer: tooling.ToolLayerDeferred, Pack: "coroot_rca", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.service_topology", Layer: tooling.ToolLayerDeferred, Pack: "coroot_rca", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.alert_rules", Layer: tooling.ToolLayerDeferred, Pack: "coroot_incident", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "opsgraph.business_impact", Layer: tooling.ToolLayerDeferred, Pack: "opsgraph", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "list_mcp_resources", Layer: tooling.ToolLayerDeferred, Pack: "mcp_resource", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_mcp_resource", Layer: tooling.ToolLayerDeferred, Pack: "mcp_resource", DeferByDefault: true}},
	}
}
