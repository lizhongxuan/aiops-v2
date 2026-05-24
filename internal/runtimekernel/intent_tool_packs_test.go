package runtimekernel

import (
	"context"
	"encoding/json"
	"slices"
	"testing"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/tooling"
)

func TestSmallContextModeLimitsVisibleTools(t *testing.T) {
	tools := visibleToolsForContextMode([]string{
		"tool_search",
		"mcp.list_resources",
		"mcp.read_resource",
		"coroot.service_metrics",
		"logs.search",
		"runner.execute",
		"debug.dump_all_metrics",
	}, ContextBudgetThresholds{SmallContextMode: true})
	if slices.Contains(tools, "debug.dump_all_metrics") {
		t.Fatalf("small context tools include high-volume debug tool: %#v", tools)
	}
	for _, want := range []string{"tool_search", "mcp.list_resources", "mcp.read_resource"} {
		if !slices.Contains(tools, want) {
			t.Fatalf("small context tools missing %s: %#v", want, tools)
		}
	}
}

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
			wantTools: []string{"coroot.list_services", "coroot.collect_rca_context"},
			forbidden: []string{"coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "named service abnormality",
			input:     "分析 aiops-host-agent 异常情况",
			wantTools: []string{"coroot.list_services", "coroot.collect_rca_context"},
			forbidden: []string{"coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot cpu chart",
			input:     "查看 aiops-host-agent 的 cpu 图表",
			wantTools: []string{"coroot.list_services", "coroot.service_metrics"},
			forbidden: []string{"coroot.collect_rca_context", "coroot.application_logs", "coroot.application_traces", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot topology",
			input:     "查看 checkout 的服务拓扑和依赖图",
			wantTools: []string{"coroot.list_services", "coroot.service_topology"},
			forbidden: []string{"coroot.service_metrics", "coroot.application_logs", "coroot.application_traces", "list_mcp_resources"},
		},
		{
			name:      "coroot logs",
			input:     "查看 checkout 最近的错误日志",
			wantTools: []string{"coroot.list_services", "coroot.application_logs"},
			forbidden: []string{"coroot.service_metrics", "coroot.application_traces", "coroot.application_profiling", "list_mcp_resources"},
		},
		{
			name:      "coroot traces",
			input:     "查看 checkout 的 trace 调用链",
			wantTools: []string{"coroot.list_services", "coroot.traces_overview", "coroot.application_traces"},
			forbidden: []string{"coroot.service_metrics", "coroot.application_logs", "coroot.application_profiling", "list_mcp_resources"},
		},
		{
			name:      "coroot dashboard panel",
			input:     "读取 Coroot dashboard panel 数据",
			wantTools: []string{"coroot.list_services", "coroot.list_dashboards", "coroot.get_dashboard", "coroot.get_panel_data"},
			forbidden: []string{"coroot.service_metrics", "coroot.application_logs", "coroot.list_integrations", "list_mcp_resources"},
		},
		{
			name:      "coroot config",
			input:     "看下 Coroot integrations 和 inspection 配置",
			wantTools: []string{"coroot.list_services", "coroot.list_integrations", "coroot.get_integration", "coroot.list_inspections", "coroot.get_inspection_config"},
			forbidden: []string{"coroot.service_metrics", "coroot.application_logs", "coroot.health_check", "list_mcp_resources"},
		},
		{
			name:      "coroot project status",
			input:     "检查 Coroot project status 和 agent status",
			wantTools: []string{"coroot.list_services", "coroot.health_check", "coroot.list_projects", "coroot.get_project_status"},
			forbidden: []string{"coroot.service_metrics", "coroot.application_logs", "coroot.list_integrations", "list_mcp_resources"},
		},
		{
			name:      "coroot incidents",
			input:     "看下 Coroot incidents 最近有哪些事件",
			wantTools: []string{"coroot.list_services", "coroot.incidents"},
			forbidden: []string{"coroot.service_metrics", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "business impact",
			input:     "order-api 故障会影响哪些业务能力和租户？",
			wantTools: []string{"opsgraph.business_impact"},
			forbidden: []string{"coroot.service_metrics", "list_mcp_resources"},
		},
		{
			name:      "diagnosis does not enable ops manuals",
			input:     "排查mservice异常问题",
			wantTools: []string{"coroot.list_services", "coroot.collect_rca_context"},
			forbidden: []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"},
		},
		{
			name:      "repair intent enables ops manual search",
			input:     "帮我修复 Redis 内存上涨问题，先找能用的运维手册",
			wantTools: []string{"search_ops_manuals"},
			forbidden: []string{"resolve_ops_manual_params", "run_ops_manual_preflight"},
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

func TestRunTurn_EnablesOnlyTopToolSearchPackHit(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-tool-search-coroot",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tool_search",
						Arguments: `{"query":"coroot logs"}`,
					},
				},
			}),
			schema.AssistantMessage("coroot logs available", nil),
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
				return tooling.ToolResult{Content: `{"matches":[{"kind":"pack","name":"coroot_logs","pack":"coroot_logs","tools":["coroot.application_logs"]},{"kind":"pack","name":"coroot_traces","pack":"coroot_traces","tools":["coroot.application_traces"]}]}`}, nil
			},
		},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.application_logs", Layer: tooling.ToolLayerDeferred, Pack: "coroot_logs", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.application_traces", Layer: tooling.ToolLayerDeferred, Pack: "coroot_traces", DeferByDefault: true}},
	} {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-tool-search-coroot-one-pack",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-tool-search-coroot-one-pack",
		Input:       "查一下 Coroot logs 能力",
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
	second := toolNames(compiler.contexts[1].AssembledTools)
	if !containsString(second, "coroot.application_logs") {
		t.Fatalf("second iteration tools = %v, want coroot.application_logs after top pack hit", second)
	}
	if containsString(second, "coroot.application_traces") {
		t.Fatalf("second iteration tools = %v, should not enable non-top tool_search pack hit", second)
	}
}

func TestRunTurn_ContinuationInheritsRecentCorootRCAPack(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("continue", nil)}}
	registry := tooling.NewRegistry()
	for _, toolDef := range intentPackRuntimeTestTools() {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	now := time.Now()
	session := kernel.sessions.GetOrCreate("sess-continuation-coroot", SessionTypeHost, ModeChat)
	session.TurnHistory = []TurnSnapshot{{
		ID:          "turn-coroot-before",
		SessionID:   session.ID,
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		Lifecycle:   TurnLifecycleCompleted,
		ResumeState: TurnResumeStateNone,
		Iteration:   1,
		StartedAt:   now.Add(-time.Minute),
		UpdatedAt:   now.Add(-time.Minute),
		CompletedAt: &now,
		AgentItems: []agentstate.TurnItem{
			newAgentItem(
				"turn-coroot-before-tool-call",
				agentstate.TurnItemTypeToolCall,
				agentstate.ItemStatusCompleted,
				"coroot.service_metrics",
				map[string]string{"toolName": "coroot.service_metrics"},
			),
		},
	}}

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   session.ID,
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-continuation-coroot",
		Input:       "继续",
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
	for _, want := range []string{"coroot.service_metrics"} {
		if !containsString(names, want) {
			t.Fatalf("tools = %v, want continuation to inherit %s", names, want)
		}
	}
	for _, forbidden := range []string{"coroot.rca_report", "coroot.service_topology", "coroot.application_logs", "coroot.application_traces"} {
		if containsString(names, forbidden) {
			t.Fatalf("tools = %v, continuation should not broaden from metrics into %s", names, forbidden)
		}
	}
	if containsString(names, "search_ops_manuals") {
		t.Fatalf("tools = %v, continuation should not enable ops manual search", names)
	}
}

func TestRunTurn_GenericContinuationDoesNotEnableCorootRCAPack(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("continue", nil)}}
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
		SessionID:   "sess-continuation-empty",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-continuation-empty",
		Input:       "继续",
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
	for _, forbidden := range []string{"coroot.service_metrics", "coroot.rca_report", "coroot.service_topology", "search_ops_manuals"} {
		if containsString(names, forbidden) {
			t.Fatalf("tools = %v, generic continuation should not include %s", names, forbidden)
		}
	}
}

func intentPackRuntimeTestTools() []tooling.Tool {
	return []tooling.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_services", Layer: tooling.ToolLayerCore}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.collect_rca_context", Layer: tooling.ToolLayerDeferred, Pack: "coroot_rca", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.service_metrics", Layer: tooling.ToolLayerDeferred, Pack: "coroot_metrics", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.slo_status", Layer: tooling.ToolLayerDeferred, Pack: "coroot_metrics", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.rca_report", Layer: tooling.ToolLayerDeferred, Pack: "coroot_rca_reference", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.service_topology", Layer: tooling.ToolLayerDeferred, Pack: "coroot_topology", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.nodes_overview", Layer: tooling.ToolLayerDeferred, Pack: "coroot_nodes", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_node", Layer: tooling.ToolLayerDeferred, Pack: "coroot_nodes", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.traces_overview", Layer: tooling.ToolLayerDeferred, Pack: "coroot_traces", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.application_traces", Layer: tooling.ToolLayerDeferred, Pack: "coroot_traces", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.deployments_overview", Layer: tooling.ToolLayerDeferred, Pack: "coroot_deployments", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.risks_overview", Layer: tooling.ToolLayerDeferred, Pack: "coroot_risks", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.application_logs", Layer: tooling.ToolLayerDeferred, Pack: "coroot_logs", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.application_profiling", Layer: tooling.ToolLayerDeferred, Pack: "coroot_profiling", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.alert_rules", Layer: tooling.ToolLayerDeferred, Pack: "coroot_incident", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.incidents", Layer: tooling.ToolLayerDeferred, Pack: "coroot_incident", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.incident_timeline", Layer: tooling.ToolLayerDeferred, Pack: "coroot_incident", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_dashboards", Layer: tooling.ToolLayerDeferred, Pack: "coroot_dashboard", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_dashboard", Layer: tooling.ToolLayerDeferred, Pack: "coroot_dashboard", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_panel_data", Layer: tooling.ToolLayerDeferred, Pack: "coroot_dashboard", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_integrations", Layer: tooling.ToolLayerDeferred, Pack: "coroot_config_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_integration", Layer: tooling.ToolLayerDeferred, Pack: "coroot_config_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_inspections", Layer: tooling.ToolLayerDeferred, Pack: "coroot_config_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_inspection_config", Layer: tooling.ToolLayerDeferred, Pack: "coroot_config_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_application_categories", Layer: tooling.ToolLayerDeferred, Pack: "coroot_config_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_custom_applications", Layer: tooling.ToolLayerDeferred, Pack: "coroot_config_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.health_check", Layer: tooling.ToolLayerDeferred, Pack: "coroot_admin_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.list_projects", Layer: tooling.ToolLayerDeferred, Pack: "coroot_admin_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "coroot.get_project_status", Layer: tooling.ToolLayerDeferred, Pack: "coroot_admin_read", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "opsgraph.business_impact", Layer: tooling.ToolLayerDeferred, Pack: "opsgraph", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "list_mcp_resources", Layer: tooling.ToolLayerDeferred, Pack: "mcp_resource", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_mcp_resource", Layer: tooling.ToolLayerDeferred, Pack: "mcp_resource", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "search_ops_manuals", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "resolve_ops_manual_params", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "run_ops_manual_preflight", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true}},
	}
}
