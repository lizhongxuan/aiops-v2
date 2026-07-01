package toolsearch

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func TestToolSearchToolIsReadOnlyAndReturnsToolMatches(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, fakeDeferredTool("coroot.service_metrics", "Get service metrics"))
	mustRegister(t, registry, fakeDeferredTool("opsgraph.business_impact", "Read business impact"))

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
	for _, name := range []string{
		"synthetic.legacy_match",
		"synthetic.fallback_exec",
		"synthetic.business_metric",
		"synthetic.cluster_events",
		"synthetic.recent_changes",
	} {
		mustRegister(t, registry, &tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:        name,
				Description: "synthetic hidden tool",
				Discovery: tooling.ToolDiscoveryMetadata{
					HiddenFromDiscovery: true,
				},
			},
		})
	}
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "synthetic.internal_plan",
			Description: "internal plan",
			Layer:       tooling.ToolLayerInternal,
		},
	})

	result := runToolSearch(t, registry, "synthetic metric changes")
	for _, forbidden := range []string{"synthetic.legacy_match", "synthetic.fallback_exec", "synthetic.business_metric", "synthetic.cluster_events", "synthetic.recent_changes", "synthetic.internal_plan"} {
		if strings.Contains(result, forbidden) {
			t.Fatalf("tool_search returned forbidden tool %q: %s", forbidden, result)
		}
	}
}

func TestToolSearchReturnsDiscoveryMetadata(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "synthetic.resource_reader",
			Description: "Read bounded resources",
			SearchHint:  "bounded resource evidence",
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"resource"},
				OperationKinds: []string{"read"},
				RequiresSelect: true,
			},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
	})

	content := runToolSearch(t, registry, "resource evidence")
	for _, want := range []string{`"capabilityKind":"read"`, `"resourceTypes":["resource"]`, `"operationKinds":["read"]`, `"requiresSelect":true`, `"selectHint"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}
}

func TestToolSearchUsesRequestedSessionModeAndProfileCatalog(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.host_inspect", Description: "Inspect host resource", Layer: tooling.ToolLayerDeferred},
		Visibility: tooling.Visibility{
			SessionTypes: []string{"host"},
			Modes:        []string{"chat"},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.workspace_inspect", Description: "Inspect workspace resource", Layer: tooling.ToolLayerDeferred},
		Visibility: tooling.Visibility{
			SessionTypes: []string{"workspace"},
			Modes:        []string{"chat"},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "agent", Description: "Delegate task to agent", Layer: tooling.ToolLayerProfile, Profiles: []string{"manager"}},
	})

	hostContent := runToolSearchWithInput(t, registry, map[string]any{
		"query":        "inspect resource",
		"session_type": "host",
		"runtime_mode": "chat",
	})
	if !strings.Contains(hostContent, "synthetic.host_inspect") || strings.Contains(hostContent, "synthetic.workspace_inspect") {
		t.Fatalf("host scoped content = %s", hostContent)
	}

	workspaceContent := runToolSearchWithInput(t, registry, map[string]any{
		"query":        "inspect resource",
		"session_type": "workspace",
		"runtime_mode": "chat",
	})
	if !strings.Contains(workspaceContent, "synthetic.workspace_inspect") || strings.Contains(workspaceContent, "synthetic.host_inspect") {
		t.Fatalf("workspace scoped content = %s", workspaceContent)
	}

	managerContent := runToolSearchWithInput(t, registry, map[string]any{
		"query":         "delegate task",
		"session_type":  "host",
		"runtime_mode":  "chat",
		"agent_profile": "manager",
	})
	if !strings.Contains(managerContent, `"name":"agent"`) {
		t.Fatalf("manager scoped content missing agent: %s", managerContent)
	}
}

func TestToolSearchReportsUnavailableMCPStatusAndRejectsSelect(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "observability.service_metrics",
			Description: "Read service metrics from observability MCP",
			Layer:       tooling.ToolLayerMCP,
			Pack:        "observability",
			IsMCP:       true,
			MCPInfo:     tooling.MCPInfo{ServerID: "synthetic_obs", ToolName: "service_metrics"},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithInput(t, registry, map[string]any{
		"query":               "service metrics",
		"include_unavailable": true,
		"mcp_health": map[string]string{
			"synthetic_obs": "unavailable",
		},
	})
	for _, want := range []string{`"status":"unavailable"`, `"source":"mcp"`, `"mcpServerId":"synthetic_obs"`, `"healthStatus":"unavailable"`, `"filteredReason":"mcp_unavailable"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}

	selectContent := runToolSearchWithInput(t, registry, map[string]any{
		"mode":   "select",
		"tools":  []string{"observability.service_metrics"},
		"reason": "need service metrics",
		"mcp_health": map[string]string{
			"synthetic_obs": "unavailable",
		},
	})
	if strings.Contains(selectContent, `"loadedTools":["observability.service_metrics"]`) || !strings.Contains(selectContent, `"notLoaded":["observability.service_metrics"]`) {
		t.Fatalf("unavailable MCP select should not load: %s", selectContent)
	}
}

func TestToolSearchUsesMCPRegistryHealthWhenRequestOmitsSnapshot(t *testing.T) {
	registry := tooling.NewRegistry()
	mcpRegistry := mcp.NewRegistry()
	if err := mcpRegistry.RegisterServer(mcp.ServerConfig{ID: "synthetic_obs", Transport: "builtin"}); err != nil {
		t.Fatalf("RegisterServer() error = %v", err)
	}
	if err := mcpRegistry.OnServerConnected("synthetic_obs", []tooling.Tool{&tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "observability.service_metrics",
			Description:    "Read service metrics from observability MCP",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "observability-readonly",
			DeferByDefault: true,
			IsMCP:          true,
			MCPInfo:        tooling.MCPInfo{ServerID: "synthetic_obs", ToolName: "service_metrics"},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "metrics",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
				ToolPackIDs:    []string{"observability-readonly"},
			},
		},
	}}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}
	mcpRegistry.SetServerHealthSnapshot(mcp.HealthSnapshot{ServerID: "synthetic_obs", Status: mcp.HealthUnavailable})
	_ = mcp.NewRegistry()
	provider := tooling.NewAssembler(registry, mcpRegistry)

	content := runToolSearchWithInput(t, provider, map[string]any{
		"query":               "service metrics",
		"include_unavailable": true,
	})
	for _, want := range []string{`"status":"unavailable"`, `"mcpServerId":"synthetic_obs"`, `"healthStatus":"unavailable"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}

	selectContent := runToolSearchWithInput(t, provider, map[string]any{
		"mode":   "select",
		"tools":  []string{"observability.service_metrics"},
		"reason": "need service metrics",
	})
	if strings.Contains(selectContent, `"loadedTools":["observability.service_metrics"]`) || !strings.Contains(selectContent, `"notLoadedReasons":{"observability.service_metrics":"mcp_unavailable"}`) {
		t.Fatalf("unavailable MCP select should use registry health and reject: %s", selectContent)
	}
}

func TestToolSearchSelectReturnsSelectionDelta(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "synthetic.resource_reader",
			Description:    "Read bounded resources",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_resources",
			DeferByDefault: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"resource"},
				OperationKinds: []string{"read"},
				RequiresSelect: true,
			},
		},
		ReadOnlyFunc: func(json.RawMessage) bool { return true },
	})

	tool := NewToolSearchTool(registry)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"mode":"select","tools":["synthetic.resource_reader"],"packs":["synthetic_resources"],"reason":"read evidence for current task"}`))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"mode":"select"`, `"loadedTools":["synthetic.resource_reader"]`, `"loadedPacks":["synthetic_resources"]`, `"reason":"read evidence for current task"`} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("select output missing %s: %s", want, result.Content)
		}
	}
}

func TestToolSearchSearchDoesNotReturnSelection(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "synthetic.resource_reader",
			Description:    "Read bounded resources",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_resources",
			DeferByDefault: true,
		},
	})

	content := runToolSearch(t, registry, "bounded resources")
	if strings.Contains(content, `"selection"`) || strings.Contains(content, `"loadedPacks"`) {
		t.Fatalf("search should not select tools or packs: %s", content)
	}
	if !strings.Contains(content, `"mode":"search"`) {
		t.Fatalf("search output should include mode=search: %s", content)
	}
}

func TestToolSearchDefaultSearchOnlyReturnsDeferredMCPOrDynamicTools(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "selected_host.inspect",
			Description: "Inspect CPU memory and disk on the selected host",
			Layer:       tooling.ToolLayerCore,
			AlwaysLoad:  true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"host"},
				OperationKinds: []string{"inspect"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "observability.service_metrics",
			Description:    "Read service CPU and memory metrics",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "observability_metrics",
			DeferByDefault: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithProvider(t, registry, "cpu memory metrics")
	if strings.Contains(content, `"name":"selected_host.inspect"`) {
		t.Fatalf("default tool_search should not return already-loaded core tools: %s", content)
	}
	if !strings.Contains(content, `"name":"observability_metrics"`) {
		t.Fatalf("default tool_search should return deferred pack: %s", content)
	}

	withLoaded := runToolSearchWithInput(t, registry, map[string]any{
		"query":         "cpu memory metrics",
		"includeLoaded": true,
	})
	if !strings.Contains(withLoaded, `"name":"selected_host.inspect"`) {
		t.Fatalf("includeLoaded=true should expose already-loaded tools for diagnostics: %s", withLoaded)
	}
}

func TestToolSearchNeverDiscoversPublicWebOrToolSearch(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "tool_search", Description: "Search deferred tools", Layer: tooling.ToolLayerCore, AlwaysLoad: true},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "web_search", Description: "Search public web", Layer: tooling.ToolLayerCore, Pack: "public_web", AlwaysLoad: true},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "browse_url", Description: "Open public URL", Layer: tooling.ToolLayerDeferred, Pack: "public_web", DeferByDefault: true},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.deferred", Description: "Read deferred evidence", Layer: tooling.ToolLayerDeferred, Pack: "synthetic_pack", DeferByDefault: true},
	})

	content := runToolSearchWithInput(t, registry, map[string]any{
		"query":         "web search browse deferred",
		"includeLoaded": true,
	})
	for _, forbidden := range []string{`"name":"tool_search"`, `"name":"web_search"`, `"name":"browse_url"`, `"name":"public_web"`} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("tool_search result must not expose public-web/tool-search entry %s: %s", forbidden, content)
		}
	}
	if !strings.Contains(content, `"name":"synthetic_pack"`) {
		t.Fatalf("tool_search result = %s, want ordinary deferred pack still discoverable", content)
	}
}

func TestToolSearchReturnsGovernanceMetadata(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             "opsgraph.scale_impact",
			Description:      "Scale workload",
			Mock:             true,
			Domain:           "opsgraph",
			Layer:            tooling.ToolLayerDeferred,
			RiskLevel:        tooling.ToolRiskHigh,
			Mutating:         true,
			RequiresApproval: true,
		},
	})

	content := runToolSearch(t, registry, "scale workload")
	for _, want := range []string{`"mock":true`, `"domain":"opsgraph"`, `"riskLevel":"high"`, `"mutating":true`, `"requiresApproval":true`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}
}

func TestToolSearchSearchesAssemblerDynamicMCPTools(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, fakeStaticTool("tool_search", "Search deferred tools"))
	mcpRegistry := mcp.NewRegistry()
	if err := mcpRegistry.OnServerConnected("coroot", []tooling.Tool{
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:        "coroot.service_metrics",
				Description: "Read Coroot service metrics",
				Domain:      "coroot",
			},
		},
	}); err != nil {
		t.Fatalf("OnServerConnected() error = %v", err)
	}
	provider := tooling.NewAssembler(registry, mcpRegistry)

	initialNames := toolNamesForToolSearchTest(provider.AssembleToolsWithOptions("host", "chat", tooling.AssembleOptions{}))
	if !containsToolSearchName(initialNames, "tool_search") {
		t.Fatalf("initial names = %v, want tool_search visible", initialNames)
	}
	if containsToolSearchName(initialNames, "coroot.service_metrics") {
		t.Fatalf("initial names = %v, should defer dynamic MCP tool until tool_search select", initialNames)
	}

	content := runToolSearchWithProvider(t, provider, "coroot metrics")
	for _, want := range []string{`"kind":"pack"`, `"tools":["coroot.service_metrics"]`, `"domain":"coroot"`, `"requiresSelect":true`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}

	selectContent := runToolSearchWithInput(t, provider, map[string]any{
		"mode":   "select",
		"tools":  []string{"coroot.service_metrics"},
		"reason": "need coroot service metrics",
	})
	if !strings.Contains(selectContent, `"loadedTools":["coroot.service_metrics"]`) {
		t.Fatalf("select content = %s, want dynamic MCP tool loadable after selection", selectContent)
	}
}

func TestToolSearchReturnsDeferredPackSummaryWithoutExpandingPromptCatalog(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, fakeStaticTool("tool_search", "Search tools"))
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "coroot.service_metrics",
			Description:    "Read Coroot metrics",
			Domain:         "coroot",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "coroot_rca",
			DeferByDefault: true,
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "coroot.rca_report",
			Description:    "Build Coroot RCA report",
			Domain:         "coroot",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "coroot_rca",
			DeferByDefault: true,
		},
	})

	normalNames := toolNamesForToolSearchTest(registry.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{}))
	if containsToolSearchName(normalNames, "coroot.service_metrics") || containsToolSearchName(normalNames, "coroot.rca_report") {
		t.Fatalf("normal assembled names = %v, should not expand deferred coroot_rca pack", normalNames)
	}

	content := runToolSearchWithProvider(t, registry, "coroot rca")
	for _, want := range []string{`"kind":"pack"`, `"name":"coroot_rca"`, `"tools":["coroot.rca_report","coroot.service_metrics"]`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}
}

func TestToolSearchRespectsExecutionMetadataPackGate(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "synthetic.metrics_read",
			Description:    "Read synthetic metrics",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "synthetic_metrics",
			DeferByDefault: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "metrics",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})

	tool := NewToolSearchTool(registry)
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		Metadata: map[string]string{
			tooling.ToolPackAllowedMetadataKey("synthetic_metrics"): "false",
		},
	})
	searchInput, _ := json.Marshal(map[string]any{"query": "synthetic metrics", "limit": 10})
	searchResult, err := tool.Execute(ctx, searchInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(searchResult.Content, "synthetic_metrics") || strings.Contains(searchResult.Content, "synthetic.metrics_read") {
		t.Fatalf("search leaked gated pack/tool: %s", searchResult.Content)
	}

	selectInput, _ := json.Marshal(map[string]any{
		"mode":   "select",
		"packs":  []string{"synthetic_metrics"},
		"reason": "need metrics",
	})
	selectResult, err := tool.Execute(ctx, selectInput)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(selectResult.Content, `"loadedPacks":["synthetic_metrics"]`) {
		t.Fatalf("select loaded gated pack: %s", selectResult.Content)
	}
	if !strings.Contains(selectResult.Content, `"notLoaded":["synthetic_metrics"]`) {
		t.Fatalf("select result missing notLoaded gated pack: %s", selectResult.Content)
	}
}

func TestToolSearchScoresDeferredPacksByBestToolMatch(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "coroot.application_logs",
			Description:    "Read Coroot application logs",
			SearchHint:     "logs error log logging",
			Domain:         "coroot",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "coroot_logs",
			DeferByDefault: true,
		},
	})
	for _, toolName := range []string{"coroot.traces_overview", "coroot.application_traces", "coroot.nodes_overview", "coroot.get_node"} {
		mustRegister(t, registry, &tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:           toolName,
				Description:    "Read Coroot operational data",
				SearchHint:     "coroot overview",
				Domain:         "coroot",
				Layer:          tooling.ToolLayerDeferred,
				Pack:           "coroot_large",
				DeferByDefault: true,
			},
		})
	}

	content := runToolSearchWithProvider(t, registry, "coroot logs")
	logsPos := strings.Index(content, `"name":"coroot_logs"`)
	largePos := strings.Index(content, `"name":"coroot_large"`)
	if logsPos < 0 {
		t.Fatalf("content missing coroot_logs: %s", content)
	}
	if largePos >= 0 && largePos < logsPos {
		t.Fatalf("content ranked broad larger pack ahead of specific logs pack: %s", content)
	}
}

func TestToolSearchRanksDirectHostInspectionAheadOfGenericMetricsPack(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "selected_host.inspect",
			Description: "Inspect resources on the selected host",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "execute",
				ResourceTypes:  []string{"host", "system"},
				OperationKinds: []string{"inspect", "read"},
				DiscoveryTags:  []string{"cpu", "memory", "disk", "load", "resource"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "observability.service_metrics",
			Description:    "Read service CPU memory and resource metrics",
			SearchHint:     "metrics cpu memory resource usage status",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "observability_metrics",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service", "application"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithProvider(t, registry, "host cpu resource metrics")
	hostPos := strings.Index(content, `"name":"selected_host.inspect"`)
	metricsPos := strings.Index(content, `"name":"observability_metrics"`)
	if hostPos < 0 {
		t.Fatalf("content missing selected host inspection: %s", content)
	}
	if metricsPos >= 0 && metricsPos < hostPos {
		t.Fatalf("content ranked generic metrics pack ahead of direct host inspection: %s", content)
	}
}

func TestToolSearchRanksSelectedHostForChineseHostResourceQuery(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "selected_host.inspect",
			Description: "Inspect resources on the selected host",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "execute",
				ResourceTypes:  []string{"host", "system"},
				OperationKinds: []string{"inspect", "read"},
				DiscoveryTags:  []string{"cpu", "memory", "disk", "load", "resource"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "observability.service_metrics",
			Description:    "Read service CPU memory and resource metrics",
			SearchHint:     "CPU 使用率 资源 信息 监控 状态",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "observability_metrics",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service", "application"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithProvider(t, registry, "主机 CPU 资源 信息 监控")
	hostPos := strings.Index(content, `"name":"selected_host.inspect"`)
	metricsPos := strings.Index(content, `"name":"observability_metrics"`)
	if hostPos < 0 {
		t.Fatalf("content missing selected host inspection: %s", content)
	}
	if metricsPos >= 0 && metricsPos < hostPos {
		t.Fatalf("content ranked generic metrics pack ahead of selected host inspection: %s", content)
	}
}

func TestToolSearchKeepsExplicitObservationDomainForHostQuery(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "selected_host.inspect",
			Description: "Inspect resources on the selected host",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "execute",
				ResourceTypes:  []string{"host", "system"},
				OperationKinds: []string{"inspect", "read"},
				DiscoveryTags:  []string{"cpu", "memory", "disk", "load", "resource"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "observability.nodes",
			Description:    "Read infrastructure node metrics from Observability",
			SearchHint:     "host node CPU resource metrics",
			Domain:         "observability",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "observability_nodes",
			DeferByDefault: true,
			RiskLevel:      tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"synthetic_observation"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithProvider(t, registry, "observability host cpu resource metrics")
	for _, want := range []string{`"name":"selected_host.inspect"`, `"name":"observability_nodes"`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}
}

func TestToolSearchBM25IndexesInputSchemaPropertyNames(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "backup.generic_status",
			Description:    "Read generic backup status",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "backup_generic",
			DeferByDefault: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"backup"},
				OperationKinds: []string{"read"},
			},
		},
		InputSchemaData: json.RawMessage(`{"type":"object","properties":{"target":{"type":"string"}}}`),
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           "backup.restore_window",
			Description:    "Inspect backup restore window",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "backup_restore",
			DeferByDefault: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"backup"},
				OperationKinds: []string{"inspect"},
			},
		},
		InputSchemaData: json.RawMessage(`{"type":"object","properties":{"stanza":{"type":"string"},"repo_path":{"type":"string"},"timeline":{"type":"string"}}}`),
	})

	content := runToolSearchWithProvider(t, registry, "stanza repo path timeline")
	restorePos := strings.Index(content, `"name":"backup_restore"`)
	genericPos := strings.Index(content, `"name":"backup_generic"`)
	if restorePos < 0 {
		t.Fatalf("content missing backup_restore matched by schema properties: %s", content)
	}
	if genericPos >= 0 && genericPos < restorePos {
		t.Fatalf("generic backup pack ranked ahead of schema-specific restore pack: %s", content)
	}
}

func TestToolSearchLimitsDefaultResultsPerMCPServerBucket(t *testing.T) {
	registry := tooling.NewRegistry()
	for i := 0; i < 12; i++ {
		mustRegister(t, registry, &tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:        fmt.Sprintf("coroot.synthetic_signal_%02d", i),
				Description: "Read synthetic diagnostics signal",
				Layer:       tooling.ToolLayerMCP,
				IsMCP:       true,
				MCPInfo:     tooling.MCPInfo{ServerID: "coroot", ToolName: fmt.Sprintf("synthetic_signal_%02d", i)},
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind: "read",
					ResourceTypes:  []string{"service"},
					OperationKinds: []string{"read"},
				},
			},
		})
	}
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "opsgraph.synthetic_diagnostics",
			Description: "Read synthetic diagnostics from topology graph",
			Layer:       tooling.ToolLayerMCP,
			IsMCP:       true,
			MCPInfo:     tooling.MCPInfo{ServerID: "opsgraph", ToolName: "synthetic_diagnostics"},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithInput(t, registry, map[string]any{"query": "synthetic diagnostics"})
	if strings.Count(content, `"mcpServerId":"coroot"`) > defaultMCPServerBucketLimit {
		t.Fatalf("content returned too many coroot bucket results: %s", content)
	}
	if !strings.Contains(content, `"mcpServerId":"opsgraph"`) {
		t.Fatalf("content should preserve other MCP server result under default bucket limit: %s", content)
	}
}

func TestToolSearchV3ReturnsRequestRankerAndRejectedCandidates(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "observability.service_metrics",
			Description: "Read service metrics from observability MCP",
			Layer:       tooling.ToolLayerMCP,
			IsMCP:       true,
			MCPInfo:     tooling.MCPInfo{ServerID: "observability", ToolName: "service_metrics"},
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "local.service_logs",
			Description: "Read local service logs",
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithInput(t, registry, map[string]any{
		"query":          "service metrics",
		"intent":         "rca",
		"resource_scope": "service",
		"limit":          5,
		"mcp_health": map[string]string{
			"observability": "unavailable",
		},
	})

	for _, want := range []string{
		`"ranker":"bm25"`,
		`"request"`,
		`"query":"service metrics"`,
		`"intent":"rca"`,
		`"rejected"`,
		`"name":"observability.service_metrics"`,
		`"reason":"mcp_unavailable"`,
		`"mcpServerId":"observability"`,
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
	}
	if strings.Contains(content, `"matches":[{"kind":"tool","name":"observability.service_metrics"`) {
		t.Fatalf("unavailable MCP should be rejected, not returned as selectable match: %s", content)
	}
}

func TestToolSearchV3ConsidersTargetRiskCapabilitiesAndEnvironmentFacts(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "observability.service_metrics",
			Description: "Read checkout service metrics",
			SearchHint:  "checkout latency p95",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "host.service_logs",
			Description: "Read host service logs",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"host"},
				OperationKinds: []string{"read"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "service.restart",
			Description: "Restart checkout service",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "execute",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"execute"},
			},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "service.audit_export",
			Description: "Read checkout service audit export",
			Layer:       tooling.ToolLayerDeferred,
			RiskLevel:   tooling.ToolRiskHigh,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: "read",
				ResourceTypes:  []string{"service"},
				OperationKinds: []string{"read"},
			},
		},
	})

	content := runToolSearchWithInput(t, registry, map[string]any{
		"query":             "checkout service metrics",
		"intent":            "service metrics",
		"target_refs":       []string{"service:checkout"},
		"required_caps":     []string{"read"},
		"forbidden_caps":    []string{"execute"},
		"risk_level":        "low",
		"environment_facts": []string{"checkout service p95 latency is high"},
	})

	for _, want := range []string{
		`"targetRefs":["service:checkout"]`,
		`"requiredCaps":["read"]`,
		`"forbiddenCaps":["execute"]`,
		`"riskLevel":"low"`,
		`"environmentFacts":["checkout service p95 latency is high"]`,
		`"name":"observability.service_metrics"`,
		`"targetCompatibility":"matched"`,
		`"riskDecision":"allowed"`,
		`"matchReasons":["bm25"`,
		`"target_compatible"`,
		`"capability_match"`,
		`"intent_match"`,
		`"risk_allowed"`,
		`"environment_fact_match"`,
		`"name":"host.service_logs"`,
		`"reason":"target_incompatible"`,
		`"name":"service.restart"`,
		`"reason":"forbidden_capability"`,
		`"name":"service.audit_export"`,
		`"reason":"risk_exceeds_request"`,
	} {
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

func fakeDeferredTool(name, description string) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: description,
			Layer:       tooling.ToolLayerDeferred,
		},
	}
}

func runToolSearch(t *testing.T, registry *tooling.Registry, query string) string {
	t.Helper()
	return runToolSearchWithProvider(t, registry, query)
}

func runToolSearchWithProvider(t *testing.T, provider tooling.ToolCatalogProvider, query string) string {
	t.Helper()
	return runToolSearchWithInput(t, provider, map[string]any{"query": query, "limit": 10})
}

func runToolSearchWithInput(t *testing.T, provider tooling.ToolCatalogProvider, inputPayload map[string]any) string {
	t.Helper()
	tool := NewToolSearchTool(provider)
	input, err := json.Marshal(inputPayload)
	if err != nil {
		t.Fatal(err)
	}
	result, err := tool.Execute(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	return result.Content
}

func toolNamesForToolSearchTest(tools []tooling.Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Metadata().Name)
	}
	return out
}

func containsToolSearchName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
