package tooling

import "testing"

func TestTurnMetadataHidesExecCommandWhenRouteDisallowsHostExec(t *testing.T) {
	meta := ToolMetadata{Name: "exec_command"}
	metadata := map[string]string{"aiops.tool.execCommandAllowed": "false"}
	if IsToolVisibleForTurnMetadata(meta, metadata) {
		t.Fatalf("exec_command should be hidden when route disallows host exec")
	}
	decision := ToolVisibilityDecisionForTurnMetadata(meta, metadata)
	if decision.Visible || decision.Reason != "host_exec_disallowed" {
		t.Fatalf("decision = %#v, want hidden host_exec_disallowed", decision)
	}
}

func TestTurnMetadataAllowsExecCommandWhenHostBound(t *testing.T) {
	meta := ToolMetadata{Name: "exec_command"}
	metadata := map[string]string{"aiops.tool.execCommandAllowed": "true"}
	if !IsToolVisibleForTurnMetadata(meta, metadata) {
		t.Fatalf("exec_command should be visible for host-bound route")
	}
}

func TestTurnMetadataKeepsWebSearchVisibleForAdvisor(t *testing.T) {
	meta := ToolMetadata{Name: "web_search"}
	metadata := map[string]string{
		"aiops.route.mode":              "chat_advisory",
		"aiops.tool.execCommandAllowed": "false",
		"aiops.route.allowsWebLearn":    "true",
		"aiops.weblearn.sourcePolicy":   "official_first",
	}
	if !IsToolVisibleForTurnMetadata(meta, metadata) {
		t.Fatalf("web_search should remain visible for advisory route")
	}
}

func TestTurnMetadataFilterHidesAlwaysLoadExecCommandDuringAssembly(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&StaticTool{
		Meta: ToolMetadata{Name: "exec_command", AlwaysLoad: true, Layer: ToolLayerCore},
	}); err != nil {
		t.Fatalf("Register(exec_command) error = %v", err)
	}
	if err := registry.Register(&StaticTool{
		Meta: ToolMetadata{Name: "web_search", AlwaysLoad: true, Layer: ToolLayerCore},
	}); err != nil {
		t.Fatalf("Register(web_search) error = %v", err)
	}

	tools := registry.CompileContextWithMetadata("workspace", "chat", map[string]string{
		"aiops.tool.execCommandAllowed": "false",
		"enableToolPack":                "public_web",
	})
	names := toolNamesForTurnMetadataTest(tools)
	if containsStringForTurnMetadataTest(names, "exec_command") {
		t.Fatalf("assembled tools = %v, want exec_command hidden by turn metadata", names)
	}
	if !containsStringForTurnMetadataTest(names, "web_search") {
		t.Fatalf("assembled tools = %v, want web_search still visible", names)
	}
}

func TestTurnMetadataFilterKeepsNoHostAdvisorToolSurfaceDiscoveryOnly(t *testing.T) {
	registry := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "tool_search", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "exec_command", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "web_search", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "public_web"},
		{Name: "browse_url", DeferByDefault: true, Layer: ToolLayerDeferred, Pack: "public_web"},
		{Name: "grep", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "filesystem_search"},
		{Name: "list_mcp_resources", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "mcp_resource"},
		{Name: "read_mcp_resource", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "mcp_resource"},
	} {
		if err := registry.Register(&StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	tools := registry.CompileContextWithMetadata("workspace", "chat", map[string]string{
		"aiops.route.mode":              "chat_advisory",
		"aiops.route.allowsWebLearn":    "false",
		"aiops.tool.execCommandAllowed": "false",
		"aiops.target.binding":          "none",
	})
	names := toolNamesForTurnMetadataTest(tools)
	if len(names) != 1 || names[0] != "tool_search" {
		t.Fatalf("assembled tools = %v, want discovery-only tool_search for no-host advisory", names)
	}
}

func TestTurnMetadataFilterPublicWebAdvisorUsesDirectWebToolsWithoutToolSearch(t *testing.T) {
	registry := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "tool_search", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "exec_command", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "web_search", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "public_web"},
		{Name: "browse_url", DeferByDefault: true, Layer: ToolLayerDeferred, Pack: "public_web"},
		{Name: "grep", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "filesystem_search"},
	} {
		if err := registry.Register(&StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	tools := registry.CompileContextWithMetadata("workspace", "chat", map[string]string{
		"aiops.route.mode":              "chat_advisory",
		"aiops.route.allowsWebLearn":    "true",
		"aiops.tool.execCommandAllowed": "false",
		"aiops.target.binding":          "none",
	})
	names := toolNamesForTurnMetadataTest(tools)
	for _, want := range []string{"web_search", "browse_url"} {
		if !containsStringForTurnMetadataTest(names, want) {
			t.Fatalf("assembled tools = %v, missing direct public web tool %s", names, want)
		}
	}
	for _, forbidden := range []string{"tool_search", "exec_command", "grep"} {
		if containsStringForTurnMetadataTest(names, forbidden) {
			t.Fatalf("assembled tools = %v, should not include %s for direct public web advisor", names, forbidden)
		}
	}
}

func TestTurnMetadataFilterKeepsUserEvidenceRCAInitialSurfaceEvidenceOnly(t *testing.T) {
	registry := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "tool_search", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "exec_command", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "web_search", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "public_web"},
		{Name: "grep", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "filesystem_search"},
		{Name: "list_mcp_resources", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "mcp_resource"},
		{Name: "read_mcp_resource", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "mcp_resource"},
		{Name: "search_ops_manuals", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
		{Name: "resolve_ops_manual_params", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
		{Name: "run_ops_manual_preflight", Layer: ToolLayerDeferred, Pack: "ops_manual_flow", DeferByDefault: true},
	} {
		if err := registry.Register(&StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	tools := registry.CompileContextWithMetadata("workspace", "chat", map[string]string{
		"aiops.route.mode":              "evidence_rca",
		"aiops.userEvidence.present":    "true",
		"aiops.tool.execCommandAllowed": "false",
		"aiops.target.binding":          "none",
		"enableToolPack":                "ops_manual_flow",
	})
	names := toolNamesForTurnMetadataTest(tools)
	for _, want := range []string{"tool_search"} {
		if !containsStringForTurnMetadataTest(names, want) {
			t.Fatalf("assembled tools = %v, missing %s", names, want)
		}
	}
	for _, forbidden := range []string{"exec_command", "web_search", "grep", "list_mcp_resources", "read_mcp_resource", "search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"} {
		if containsStringForTurnMetadataTest(names, forbidden) {
			t.Fatalf("assembled tools = %v, should not include %s for initial user-evidence RCA", names, forbidden)
		}
	}
}

func TestTurnMetadataFilterAllowsOpsManualSearchOnlyWhenExplicitlyRequested(t *testing.T) {
	meta := ToolMetadata{Name: "search_ops_manuals", Layer: ToolLayerDeferred, Pack: "ops_manual_flow"}
	baseMetadata := map[string]string{
		"aiops.route.mode":              "evidence_rca",
		"aiops.userEvidence.present":    "true",
		"aiops.tool.execCommandAllowed": "false",
		"aiops.target.binding":          "none",
		"enableToolPack":                "ops_manual_flow",
	}

	notRequested := ToolVisibilityDecisionForTurnMetadata(meta, baseMetadata)
	if notRequested.Visible || notRequested.Reason != "opsmanual_not_requested" {
		t.Fatalf("notRequested = %#v, want hidden opsmanual_not_requested", notRequested)
	}

	explicitMetadata := map[string]string{}
	for key, value := range baseMetadata {
		explicitMetadata[key] = value
	}
	explicitMetadata["enableTool"] = "search_ops_manuals"
	requested := ToolVisibilityDecisionForTurnMetadata(meta, explicitMetadata)
	if !requested.Visible {
		t.Fatalf("requested = %#v, want visible when search_ops_manuals is explicitly enabled", requested)
	}
}

func TestTurnMetadataFilterKeepsOpsManualToolsHiddenUntilExplicitMention(t *testing.T) {
	for _, name := range []string{"search_ops_manuals", "resolve_ops_manual_params", "run_ops_manual_preflight"} {
		decision := ToolVisibilityDecisionForTurnMetadata(
			ToolMetadata{Name: name, Layer: ToolLayerDeferred, Pack: "ops_manual_flow"},
			map[string]string{"enableToolPack": "ops_manual_flow"},
		)
		if decision.Visible || decision.Reason != "opsmanual_not_requested" {
			t.Fatalf("%s decision = %#v, want hidden opsmanual_not_requested", name, decision)
		}
	}
	requested := ToolVisibilityDecisionForTurnMetadata(
		ToolMetadata{Name: "search_ops_manuals", Layer: ToolLayerDeferred, Pack: "ops_manual_flow"},
		map[string]string{
			"enableToolPack":                   "ops_manual_flow",
			"aiops.opsManuals.explicitMention": "true",
		},
	)
	if !requested.Visible {
		t.Fatalf("requested decision = %#v, want visible for structured explicit mention metadata", requested)
	}
}

func TestTurnMetadataFilterKeepsHostBoundOpsDirectSurfaceScoped(t *testing.T) {
	registry := NewRegistry()
	for _, meta := range []ToolMetadata{
		{Name: "tool_search", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "exec_command", AlwaysLoad: true, Layer: ToolLayerCore},
		{Name: "web_search", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "public_web"},
		{Name: "grep", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "filesystem_search"},
		{Name: "list_mcp_resources", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "mcp_resource"},
		{Name: "read_mcp_resource", AlwaysLoad: true, Layer: ToolLayerCore, Pack: "mcp_resource"},
	} {
		if err := registry.Register(&StaticTool{Meta: meta}); err != nil {
			t.Fatalf("Register(%s) error = %v", meta.Name, err)
		}
	}

	tools := registry.CompileContextWithMetadata("host", "chat", map[string]string{
		"aiops.route.mode":                "host_bound_ops",
		"aiops.route.requiresHostBinding": "true",
		"aiops.tool.execCommandAllowed":   "true",
		"aiops.target.binding":            "host",
		"aiops.target.hostId":             "host-a",
	})
	names := toolNamesForTurnMetadataTest(tools)
	for _, want := range []string{"exec_command", "tool_search"} {
		if !containsStringForTurnMetadataTest(names, want) {
			t.Fatalf("assembled tools = %v, missing %s", names, want)
		}
	}
	for _, forbidden := range []string{"web_search", "grep", "list_mcp_resources", "read_mcp_resource"} {
		if containsStringForTurnMetadataTest(names, forbidden) {
			t.Fatalf("assembled tools = %v, should not include %s for direct host-bound ops", names, forbidden)
		}
	}
}

func TestToolMetadataFilterHidesHostExecWithoutExplicitTarget(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&StaticTool{
		Meta: ToolMetadata{Name: "exec_command", AlwaysLoad: true, Layer: ToolLayerCore},
	}); err != nil {
		t.Fatalf("Register(exec_command) error = %v", err)
	}
	if err := registry.Register(&StaticTool{
		Meta: ToolMetadata{Name: "web_search", AlwaysLoad: true, Layer: ToolLayerCore},
	}); err != nil {
		t.Fatalf("Register(web_search) error = %v", err)
	}

	tools := registry.CompileContextWithMetadata("workspace", "chat", map[string]string{
		"aiops.route.requiresHostBinding": "false",
		"aiops.tool.execCommandAllowed":   "false",
		"aiops.target.binding":            "none",
	})
	names := toolNamesForTurnMetadataTest(tools)
	if containsStringForTurnMetadataTest(names, "exec_command") {
		t.Fatalf("assembled tools = %v, want exec_command hidden without explicit target", names)
	}
	if !containsStringForTurnMetadataTest(names, "web_search") {
		t.Fatalf("assembled tools = %v, want non-host web_search still visible", names)
	}
}

func TestTurnMetadataHidesCorootRCAToolsWithoutExplicitMention(t *testing.T) {
	metadata := map[string]string{"aiops.tool.corootRCAAllowed": "false"}
	for _, meta := range []ToolMetadata{
		{Name: "coroot.collect_rca_context", Domain: "coroot", Pack: "coroot_rca"},
		{Name: "coroot_collect_rca_context", Domain: "coroot", Pack: "coroot_rca"},
		{Name: "coroot.rca_report", Domain: "coroot", Pack: "coroot_rca_reference"},
	} {
		if IsToolVisibleForTurnMetadata(meta, metadata) {
			t.Fatalf("%s should be hidden without explicit Coroot RCA", meta.Name)
		}
		decision := ToolVisibilityDecisionForTurnMetadata(meta, metadata)
		if decision.Visible || decision.Reason != "coroot_rca_not_allowed" {
			t.Fatalf("%s decision = %#v, want hidden coroot_rca_not_allowed", meta.Name, decision)
		}
	}
}

func TestTurnMetadataDecisionReportsOpsManualReasons(t *testing.T) {
	optedOut := ToolVisibilityDecisionForTurnMetadata(
		ToolMetadata{Name: "search_ops_manuals"},
		map[string]string{"opsManualAction": "skip_ops_manual"},
	)
	if optedOut.Visible || optedOut.Reason != "opsmanual_opted_out" {
		t.Fatalf("optedOut = %#v, want opsmanual_opted_out", optedOut)
	}

	referenceOnly := ToolVisibilityDecisionForTurnMetadata(
		ToolMetadata{Name: "run_ops_manual_preflight"},
		map[string]string{"opsManualAction": "reference_ops_manual"},
	)
	if referenceOnly.Visible || referenceOnly.Reason != "opsmanual_reference_only" {
		t.Fatalf("referenceOnly = %#v, want opsmanual_reference_only", referenceOnly)
	}

	paramsMissing := ToolVisibilityDecisionForTurnMetadata(
		ToolMetadata{Name: "resolve_ops_manual_params"},
		map[string]string{},
	)
	if paramsMissing.Visible || paramsMissing.Reason != "opsmanual_not_requested" {
		t.Fatalf("paramsMissing = %#v, want opsmanual_not_requested", paramsMissing)
	}
}

func TestTurnMetadataHidesCorootReadOnlyEvidenceWithoutMention(t *testing.T) {
	metadata := map[string]string{"aiops.tool.corootRCAAllowed": "false"}
	meta := ToolMetadata{Name: "coroot.list_services", Domain: "coroot", Pack: "mcp_dynamic_coroot"}
	decision := ToolVisibilityDecisionForTurnMetadata(meta, metadata)
	if decision.Visible || decision.Reason != "coroot_not_requested" {
		t.Fatalf("decision = %#v, want hidden coroot_not_requested", decision)
	}
}

func TestTurnMetadataAllowsCorootReadOnlyEvidenceWhenExplicitMentioned(t *testing.T) {
	metadata := map[string]string{"aiops.coroot.explicitMention": "true"}
	meta := ToolMetadata{Name: "coroot.list_services", Domain: "coroot", Pack: "mcp_dynamic_coroot"}
	if !IsToolVisibleForTurnMetadata(meta, metadata) {
		t.Fatalf("coroot.list_services should be visible when Coroot is explicitly mentioned")
	}
}

func TestTurnMetadataAllowsCorootRCAWhenExplicitMentioned(t *testing.T) {
	metadata := map[string]string{"aiops.tool.corootRCAAllowed": "true"}
	meta := ToolMetadata{Name: "coroot.collect_rca_context", Domain: "coroot", Pack: "coroot_rca"}
	if !IsToolVisibleForTurnMetadata(meta, metadata) {
		t.Fatalf("coroot.collect_rca_context should be visible for explicit Coroot RCA")
	}
}

func toolNamesForTurnMetadataTest(tools []Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		names = append(names, tool.Metadata().Name)
	}
	return names
}

func containsStringForTurnMetadataTest(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
