package toolsearch

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func TestToolSearchToolIsReadOnlyAndReturnsToolMatches(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, fakeStaticTool("coroot.service_metrics", "Get service metrics"))
	mustRegister(t, registry, fakeStaticTool("opsgraph.business_impact", "Read business impact"))

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
		Meta: tooling.ToolMetadata{Name: "synthetic.host_inspect", Description: "Inspect host resource", Layer: tooling.ToolLayerCore},
		Visibility: tooling.Visibility{
			SessionTypes: []string{"host"},
			Modes:        []string{"chat"},
		},
	})
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{Name: "synthetic.workspace_inspect", Description: "Inspect workspace resource", Layer: tooling.ToolLayerCore},
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

func TestToolSearchReturnsGovernanceMetadata(t *testing.T) {
	registry := tooling.NewRegistry()
	mustRegister(t, registry, &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             "opsgraph.scale_impact",
			Description:      "Scale workload",
			Mock:             true,
			Domain:           "opsgraph",
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

	content := runToolSearchWithProvider(t, tooling.NewAssembler(registry, mcpRegistry), "coroot metrics")
	for _, want := range []string{`"kind":"pack"`, `"tools":["coroot.service_metrics"]`, `"domain":"coroot"`, `"requiresSelect":true`} {
		if !strings.Contains(content, want) {
			t.Fatalf("content missing %s: %s", want, content)
		}
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
