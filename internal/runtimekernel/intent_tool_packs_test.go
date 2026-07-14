package runtimekernel

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
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
		metadata  map[string]string
		wantTools []string
		forbidden []string
	}{
		{
			name:      "chinese rca",
			input:     "分析 checkout 服务最近 30 分钟延迟升高的根因",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.rca_report", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:  "explicit coroot rca",
			input: "@Coroot 分析 checkout 服务最近 30 分钟延迟升高的根因",
			metadata: map[string]string{
				"aiops.coroot.explicitRCA":                    "true",
				"aiops.tool.corootRCAAllowed":                 "true",
				"aiops.toolPack.coroot_rca.allowed":           "true",
				"aiops.toolPack.coroot_rca_reference.allowed": "true",
			},
			wantTools: []string{"coroot.list_services", "coroot.collect_rca_context", "coroot.service_metrics"},
			forbidden: []string{"coroot.rca_report", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "named service abnormality",
			input:     "分析 aiops-host-agent 异常情况",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.rca_report", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot cpu chart",
			input:     "查看 aiops-host-agent 的 cpu 图表",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.application_logs", "coroot.application_traces", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot chinese cpu usage",
			input:     "看下 mservice CPU占用",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.application_logs", "coroot.application_traces", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot chinese resource usage",
			input:     "看下 mservice 的资源占用和内存使用率",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.application_logs", "coroot.application_traces", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot chinese service resources",
			input:     "看下 mservice 的资源",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.application_logs", "coroot.application_traces", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot chinese service observation lets model decide charts",
			input:     "看下 mservice 的情况",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "coroot.application_logs", "coroot.application_traces", "coroot.service_topology", "coroot.alert_rules", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "coroot topology",
			input:     "查看 checkout 的服务拓扑和依赖图",
			forbidden: []string{"coroot.list_services", "coroot.service_topology", "coroot.service_metrics", "coroot.application_logs", "coroot.application_traces", "list_mcp_resources"},
		},
		{
			name:      "coroot logs",
			input:     "查看 checkout 最近的错误日志",
			forbidden: []string{"coroot.list_services", "coroot.application_logs", "coroot.service_metrics", "coroot.application_traces", "coroot.application_profiling", "list_mcp_resources"},
		},
		{
			name:      "coroot traces",
			input:     "查看 checkout 的 trace 调用链",
			forbidden: []string{"coroot.list_services", "coroot.traces_overview", "coroot.application_traces", "coroot.service_metrics", "coroot.application_logs", "coroot.application_profiling", "list_mcp_resources"},
		},
		{
			name:      "coroot dashboard panel",
			input:     "读取 Coroot dashboard panel 数据",
			forbidden: []string{"coroot.list_services", "coroot.list_dashboards", "coroot.get_dashboard", "coroot.get_panel_data", "coroot.service_metrics", "coroot.application_logs", "coroot.list_integrations", "list_mcp_resources"},
		},
		{
			name:      "coroot config",
			input:     "看下 Coroot integrations 和 inspection 配置",
			forbidden: []string{"coroot.list_services", "coroot.list_integrations", "coroot.get_integration", "coroot.list_inspections", "coroot.get_inspection_config", "coroot.service_metrics", "coroot.application_logs", "coroot.health_check", "list_mcp_resources"},
		},
		{
			name:      "coroot project status",
			input:     "检查 Coroot project status 和 agent status",
			forbidden: []string{"coroot.list_services", "coroot.health_check", "coroot.list_projects", "coroot.get_project_status", "coroot.service_metrics", "coroot.application_logs", "coroot.list_integrations", "list_mcp_resources"},
		},
		{
			name:      "coroot incidents",
			input:     "看下 Coroot incidents 最近有哪些事件",
			forbidden: []string{"coroot.list_services", "coroot.incidents", "coroot.service_metrics", "opsgraph.business_impact", "list_mcp_resources"},
		},
		{
			name:      "business impact",
			input:     "order-api 故障会影响哪些业务能力和租户？",
			forbidden: []string{"opsgraph.business_impact", "coroot.service_metrics", "list_mcp_resources"},
		},
		{
			name:      "diagnosis does not enable ops manuals",
			input:     "排查mservice异常问题",
			forbidden: []string{"coroot.list_services", "coroot.service_metrics", "coroot.collect_rca_context", "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"},
		},
		{
			name:      "repair intent does not auto enable ops manual search",
			input:     "帮我修复 Redis 内存上涨问题，先找能用的运维手册",
			forbidden: []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"},
		},
		{
			name:      "restore stateful cluster intent does not auto enable ops manual search",
			input:     "主机A和主机B的PG主从集群异常,请帮忙恢复,数据可以不要,只需要PG主从集群可以正常运行,他们的pg_mon部署在主机C.",
			forbidden: []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"},
		},
		{
			name:  "explicit ops manual trigger enables ops manual search",
			input: "@ops_manuals 帮我修复 Redis 内存上涨问题",
			metadata: map[string]string{
				"enableToolPack":                   "ops_manual_flow",
				"enableTool":                       "search_ops_manuals",
				"aiops.opsManuals.explicitMention": "true",
			},
			wantTools: []string{"search_ops_manuals"},
			forbidden: []string{"resolve_ops_manual_params", "run_ops_manual_preflight"},
		},
		{
			name:      "mcp resource",
			input:     "读取 MCP resource ops://manuals/redis",
			wantTools: []string{"list_mcp_resources", "read_mcp_resource"},
			forbidden: []string{"coroot.service_metrics", "opsgraph.business_impact"},
		},
		{
			name:      "runtime model config",
			input:     "Tell me current model name only. Do not mention any api key.",
			wantTools: []string{"get_current_model_config"},
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

			metadata := map[string]string{
				"taskDepth":                                   "simple_read",
				"aiops.toolPack.coroot_rca.allowed":           "false",
				"aiops.toolPack.coroot_rca_reference.allowed": "false",
			}
			for key, value := range tc.metadata {
				metadata[key] = value
			}
			result, err := kernel.RunTurn(context.Background(), TurnRequest{
				SessionID:   "sess-intent-" + tc.name,
				SessionType: SessionTypeHost,
				Mode:        ModeChat,
				TurnID:      "turn-intent-" + tc.name,
				Input:       tc.input,
				Metadata:    metadata,
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

func TestRunTurn_EnablesExecForSelectedHostResourceInspection(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("ok", nil)}}
	registry := tooling.NewRegistry()
	for _, toolDef := range []tooling.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:       "exec_command",
				Layer:      tooling.ToolLayerCore,
				AlwaysLoad: true,
				RiskLevel:  tooling.ToolRiskHigh,
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind:  "host_fact",
					ResourceTypes:   []string{"host", "system"},
					OperationKinds:  []string{"inspect", "read", "execute"},
					PermissionScope: "argument_scoped",
				},
			},
			DestructiveFunc: func(_ json.RawMessage) bool {
				return true
			},
		},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "tool_search", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "grep", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "list_mcp_resources", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "read_mcp_resource", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskLow}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "web_search", Layer: tooling.ToolLayerCore, AlwaysLoad: true, RiskLevel: tooling.ToolRiskMedium}},
	} {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register(%s) failed: %v", toolDef.Metadata().Name, err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-selected-host-resource-inspection",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-selected-host-resource-inspection",
		Input:       "查看 CPU 情况",
		Metadata: map[string]string{
			"taskDepth":                       "simple_read",
			"aiops.target.binding":            "host",
			"aiops.target.hostId":             "server-local",
			"aiops.tool.execCommandAllowed":   "true",
			"aiops.route.mode":                "host_bound_ops",
			"aiops.route.requiresHostBinding": "true",
		},
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
	if !containsString(names, "exec_command") {
		t.Fatalf("tools = %v, want exec_command visible for direct selected-host resource inspection", names)
	}
}

func TestRunTurn_DoesNotEnableObservationPacksForSelectedHostResourceInspection(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("ok", nil)}}
	registry := tooling.NewRegistry()
	for _, toolDef := range intentPackRuntimeTestTools() {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register(%s) failed: %v", toolDef.Metadata().Name, err)
		}
	}
	if err := registry.Register(&tooling.StaticTool{Meta: tooling.ToolMetadata{
		Name:           "external_observability.host_metrics",
		Domain:         "external_observability",
		Layer:          tooling.ToolLayerDeferred,
		Pack:           "external_observability_metrics",
		DeferByDefault: true,
		Triggers:       []string{"CPU", "memory", "资源", "主机"},
		RiskLevel:      tooling.ToolRiskLow,
		Discovery: tooling.ToolDiscoveryMetadata{
			DiscoveryGroup: "observability",
			CapabilityKind: "metrics",
			ResourceTypes:  []string{"host", "resource"},
			OperationKinds: []string{"read", "query"},
		},
	}}); err != nil {
		t.Fatalf("Register(external_observability.host_metrics) failed: %v", err)
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-selected-host-resource-no-observation",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-selected-host-resource-no-observation",
		Input:       "只读检查当前选中远程主机的 CPU、内存、磁盘资源情况",
		Metadata: map[string]string{
			"taskDepth":           "simple_read",
			"aiops.target.hostId": "remote-linux-01",
		},
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
	for _, forbidden := range []string{
		"coroot.collect_rca_context",
		"coroot.service_metrics",
		"coroot.slo_status",
		"coroot.nodes_overview",
		"coroot.get_node",
		"external_observability.host_metrics",
	} {
		if containsString(names, forbidden) {
			t.Fatalf("tools = %v, should not include %s for direct selected-host inspection", names, forbidden)
		}
	}
}

func TestRunTurn_EnablesMCPResourcePackAfterToolSearchSelect(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-tool-search-resource",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tool_search",
						Arguments: `{"mode":"search","query":"synthetic resource"}`,
					},
				},
			}),
			schema.AssistantMessage("", []schema.ToolCall{
				{
					ID:   "call-tool-select-resource",
					Type: "function",
					Function: schema.FunctionCall{
						Name:      "tool_search",
						Arguments: `{"mode":"select","packs":["mcp_resource"],"reason":"need bounded resource evidence"}`,
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
			ExecuteFunc: func(_ context.Context, raw json.RawMessage) (tooling.ToolResult, error) {
				var req struct {
					Mode string `json:"mode"`
				}
				_ = json.Unmarshal(raw, &req)
				if req.Mode == "select" {
					return tooling.ToolResult{Content: `{"mode":"select","selection":{"loadedPacks":["mcp_resource"],"reason":"need bounded resource evidence"}}`}, nil
				}
				return tooling.ToolResult{Content: `{"mode":"search","matches":[{"kind":"pack","name":"mcp_resource","pack":"mcp_resource","tools":["list_mcp_resources","read_mcp_resource"],"requiresSelect":true}]}`}, nil
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
		Input:       "use tool_search to discover deferred synthetic resource tools",
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) != 3 {
		t.Fatalf("compiler contexts = %d, want 3", len(compiler.contexts))
	}
	first := toolNames(compiler.contexts[0].AssembledTools)
	if !containsString(first, "tool_search") || containsString(first, "list_mcp_resources") || containsString(first, "read_mcp_resource") {
		t.Fatalf("first iteration tools = %v, want tool_search only for resource discovery", first)
	}
	second := toolNames(compiler.contexts[1].AssembledTools)
	if containsString(second, "list_mcp_resources") || containsString(second, "read_mcp_resource") {
		t.Fatalf("second iteration tools = %v, search alone should not enable resource tools", second)
	}
	third := toolNames(compiler.contexts[2].AssembledTools)
	for _, want := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if !containsString(third, want) {
			t.Fatalf("third iteration tools = %v, want %s after tool_search select", third, want)
		}
	}
	if !containsString(compiler.contexts[2].ToolDelta.NewlyAvailablePacks, "mcp_resource") {
		t.Fatalf("third iteration tool delta packs = %v, want mcp_resource", compiler.contexts[2].ToolDelta.NewlyAvailablePacks)
	}
}

func TestIntentPackDoesNotRequireCoreNameMapping(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("ok", nil)}}
	registry := tooling.NewRegistry()
	for _, toolDef := range syntheticIntentPackRuntimeTestTools() {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-synthetic-intent",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-synthetic-intent",
		Input:       "inspect synthetic metric latency evidence",
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
	if !containsString(names, "synthetic.metrics.read") {
		t.Fatalf("tools = %v, want synthetic.metrics.read from metadata trigger", names)
	}
	for _, forbidden := range []string{"synthetic.logs.search", "synthetic.hidden.write"} {
		if containsString(names, forbidden) {
			t.Fatalf("tools = %v, should not include %s", names, forbidden)
		}
	}
}

func TestContinuationInheritsPackFromRecentToolMetadata(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{schema.AssistantMessage("continue", nil)}}
	registry := tooling.NewRegistry()
	for _, toolDef := range syntheticIntentPackRuntimeTestTools() {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	source := &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, source, compiler, model)

	now := time.Now()
	session := kernel.sessions.GetOrCreate("sess-synthetic-continuation", SessionTypeHost, ModeChat)
	session.TurnHistory = []TurnSnapshot{{
		ID:          "turn-synthetic-before",
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
				"turn-synthetic-before-tool-call",
				agentstate.TurnItemTypeToolCall,
				agentstate.ItemStatusCompleted,
				"synthetic.metrics.read",
				map[string]string{"toolName": "synthetic.metrics.read"},
			),
		},
	}}

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   session.ID,
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-synthetic-continuation",
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
	if !containsString(names, "synthetic.metrics.read") {
		t.Fatalf("tools = %v, want continuation to inherit synthetic.metrics.read pack", names)
	}
	if containsString(names, "synthetic.logs.search") {
		t.Fatalf("tools = %v, continuation should not broaden into logs pack", names)
	}
}

func TestRunTurn_ToolSearchSearchDoesNotAutoEnableTopPack(t *testing.T) {
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
			schema.AssistantMessage("coroot search results only", nil),
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
		SessionID:   "sess-tool-search-no-auto-pack",
		SessionType: SessionTypeHost,
		Mode:        ModeChat,
		TurnID:      "turn-tool-search-no-auto-pack",
		Input:       "use tool_search to discover deferred synthetic log tools",
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
	for _, forbidden := range []string{"coroot.application_logs", "coroot.application_traces"} {
		if containsString(second, forbidden) {
			t.Fatalf("second iteration tools = %v, search result should not enable %s without select", second, forbidden)
		}
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

func TestRunTurn_GenericPGQuestionDoesNotEnableCorootRCAWithoutExplicitMetadata(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("根因（置信度：低）：需要更多 PostgreSQL 证据。缺失证据：pg_controldata 输出、postgresql.auto.conf、pg_autoctl show state。", nil),
		schema.AssistantMessage("最终结论：没有 @Coroot 时只做文本证据分析。", nil),
	}}
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
		SessionID:   "sess-pg-no-coroot-rca",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-pg-no-coroot-rca",
		Input:       "pgBackRest 恢复后从节点 pg_autoctl create postgres 加入集群 timeline 比主节点高，分析根因和异常原因。",
		Metadata: map[string]string{
			"aiops.coroot.explicitRCA":                    "false",
			"aiops.tool.corootRCAAllowed":                 "false",
			"aiops.route.mode":                            "evidence_rca",
			"aiops.tool.execCommandAllowed":               "false",
			"aiops.toolPack.coroot_rca.allowed":           "false",
			"aiops.toolPack.coroot_rca_reference.allowed": "false",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) == 0 {
		t.Fatalf("compiler contexts = 0, want at least 1")
	}
	for idx, ctx := range compiler.contexts {
		names := toolNames(ctx.AssembledTools)
		for _, forbidden := range []string{"coroot.collect_rca_context", "coroot.rca_report"} {
			if containsString(names, forbidden) {
				t.Fatalf("context %d tools = %v, generic PG question should not include %s without explicit @Coroot metadata", idx, names, forbidden)
			}
		}
	}
}

func syntheticIntentPackRuntimeTestTools() []tooling.Tool {
	return []tooling.Tool{
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "tool_search", Layer: tooling.ToolLayerCore}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{
			Name:           "synthetic.metrics.read",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_metrics",
			DeferByDefault: true,
			Triggers:       []string{"metric latency"},
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"metric"},
				OperationKinds: []string{"inspect", "read"},
			},
		}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{
			Name:           "synthetic.logs.search",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_logs",
			DeferByDefault: true,
			Triggers:       []string{"log stream"},
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "search",
				ResourceTypes:  []string{"log"},
				OperationKinds: []string{"search"},
			},
		}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{
			Name:           "synthetic.resource.list",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_resource",
			DeferByDefault: true,
			Triggers:       []string{"resource inventory"},
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"resource"},
				OperationKinds: []string{"list"},
			},
		}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{
			Name:           "synthetic.hidden.write",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_hidden",
			DeferByDefault: true,
			Mutating:       true,
			RiskLevel:      tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				HiddenFromDiscovery: true,
				CapabilityKind:      "write",
				ResourceTypes:       []string{"resource"},
				OperationKinds:      []string{"write"},
			},
		}},
	}
}

func intentPackRuntimeTestTools() []tooling.Tool {
	tools := []tooling.Tool{
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
		&tooling.StaticTool{Meta: tooling.ToolMetadata{
			Name:           "get_current_model_config",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "runtime_config",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Triggers:       []string{"current model", "model name", "model config", "模型配置", "当前模型"},
			SearchHint:     "current model provider config context size reasoning effort",
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "runtime_config",
				ResourceTypes:  []string{"model", "runtime", "configuration"},
				OperationKinds: []string{"read", "inspect"},
				RequiresSelect: true,
			},
		}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "search_ops_manuals", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "resolve_ops_manual_params", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true}},
		&tooling.StaticTool{Meta: tooling.ToolMetadata{Name: "run_ops_manual_preflight", Layer: tooling.ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true}},
	}
	return withIntentRuntimeMetadata(tools)
}

func withIntentRuntimeMetadata(tools []tooling.Tool) []tooling.Tool {
	triggersByPack := map[string][]string{
		"coroot_rca":           {"rca", "root cause", "根因", "异常", "warning", "告警", "延迟升高", "error rate", "排查", "诊断", "故障", "问题", "diagnose", "diagnosis", "outage", "incident analysis"},
		"coroot_metrics":       {"metric", "metrics", "指标", "图表", "chart", "timeseries", "趋势", "时序", "slo status", "service metrics", "latency", "error rate", "延迟升高", "异常", "cpu usage", "cpu utilization", "CPU占用", "CPU 使用率", "memory usage", "memory utilization", "内存使用", "内存占用", "resource usage", "resource utilization", "资源", "资源占用", "使用率", "情况", "状态", "健康"},
		"coroot_rca_reference": {"coroot rca report", "native rca", "rca reference", "root cause report", "coroot 根因报告"},
		"coroot_topology":      {"topology", "service topology", "dependency graph", "dependencies", "拓扑图", "依赖图", "服务拓扑"},
		"coroot_nodes":         {"node", "nodes", "host", "hosts", "infrastructure", "infra", "主机", "节点", "机器", "基础设施"},
		"coroot_traces":        {"trace", "traces", "tracing", "span", "spans", "链路", "调用链", "trace id"},
		"coroot_deployments":   {"deployment", "deployments", "deploy", "release", "rollout", "rollback", "发布", "部署", "变更"},
		"coroot_risks":         {"risk", "risks", "风险", "隐患"},
		"coroot_logs":          {"log", "logs", "logging", "日志", "错误日志"},
		"coroot_profiling":     {"profile", "profiling", "flamegraph", "cpu profile", "memory profile", "pprof", "火焰图", "性能剖析"},
		"coroot_incident":      {"incident", "incidents", "alert", "alerts", "告警", "事件", "timeline"},
		"coroot_dashboard":     {"dashboard", "dashboards", "panel", "coroot panel", "仪表盘", "看板", "面板"},
		"coroot_config_read":   {"integration", "integrations", "inspection", "inspections", "configuration", "config", "category", "custom application", "集成", "巡检", "配置", "应用分类", "自定义应用"},
		"coroot_admin_read":    {"coroot health", "health check", "project status", "projects", "项目", "项目状态", "agent status", "prometheus status"},
		"opsgraph":             {"业务影响", "影响哪些业务", "业务能力", "租户", "依赖关系", "服务图谱", "runbook 关联", "business impact", "tenant", "dependency graph"},
		"mcp_resource":         {"mcp resource", "mcp_resource", "resource uri", "mcp://"},
		"ops_manual_flow":      {"runbook", "manual", "repair", "fix", "recover", "restore", "修复", "恢复", "手册", "运维手册"},
	}
	for _, toolDef := range tools {
		staticTool, ok := toolDef.(*tooling.StaticTool)
		if !ok {
			continue
		}
		triggers := triggersByPack[staticTool.Meta.Pack]
		if len(triggers) == 0 {
			continue
		}
		staticTool.Meta.Triggers = append([]string(nil), triggers...)
		staticTool.Meta.SearchHint = strings.Join(triggers, " ")
		staticTool.Meta.RiskLevel = tooling.ToolRiskLow
		staticTool.Meta.Discovery.CapabilityKind = "read"
		staticTool.Meta.Discovery.ResourceTypes = []string{"synthetic_observation"}
		staticTool.Meta.Discovery.OperationKinds = []string{"read"}
	}
	return tools
}
