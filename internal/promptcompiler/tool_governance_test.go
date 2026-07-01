package promptcompiler

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aiops-v2/internal/tooling"
)

func TestToolPromptShowsGovernanceMetadata(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			governancePromptTool{
				meta: tooling.ToolMetadata{
					Name:             "restart_service",
					Description:      "restart a service",
					RiskLevel:        tooling.ToolRiskHigh,
					Mutating:         true,
					RequiresApproval: true,
					ResultBudget:     tooling.ResultBudget{MaxInlineResultBytes: 2048},
					FailurePolicy:    tooling.ToolFailurePolicyFailTurn,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Tools.Content
	for _, want := range []string{
		"restart_service: restart a service",
		"mutation",
		"approval_required",
		"not_concurrency_safe",
		"Mutation requires scoped runtime approval and post-check.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool prompt missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{
		"after approval",
		"Requires approval before execution",
		"Use only after confirming",
		"Governance:",
		"resultBudget=",
		"failure=",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("tool prompt should not ask the model to wait for %q:\n%s", forbidden, content)
		}
	}
}

func TestToolPromptCommonPolicyTreatsReadOnlyFailureAsMissingEvidence(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			readOnlyGovernancePromptTool{
				meta: tooling.ToolMetadata{
					Name:        "inspect_metrics",
					Description: "inspect metrics",
					RiskLevel:   tooling.ToolRiskLow,
					Mutating:    false,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Tools.Content
	for _, want := range []string{
		"Common policy:",
		"Only visible tools are callable.",
		"Failure, empty output, denial, or timeout is not proof of healthy state.",
		"Summarize large results and keep raw data behind refs.",
		"inspect_metrics: inspect metrics [read_only]",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("common policy missing %q:\n%s", want, content)
		}
	}
	if strings.Contains(content, "Failure handling:") {
		t.Fatalf("tool prompt should not repeat failure handling per tool:\n%s", content)
	}
}

func TestToolPromptCommonPolicyCoversDestructiveFailureBoundary(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			governancePromptTool{
				meta: tooling.ToolMetadata{
					Name:             "restart_service",
					Description:      "restart a service",
					RiskLevel:        tooling.ToolRiskHigh,
					Mutating:         true,
					RequiresApproval: true,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Tools.Content
	for _, want := range []string{
		"restart_service: restart a service [mutation,approval_required,not_concurrency_safe]",
		"Mutation requires scoped runtime approval and post-check.",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("destructive common policy missing %q:\n%s", want, content)
		}
	}
}

func TestToolPromptCommonPolicyRenderedOnceForMultipleTools(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{Name: "inspect_metrics", Description: "inspect metrics", RiskLevel: tooling.ToolRiskLow}},
			readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{Name: "read_logs", Description: "read logs", RiskLevel: tooling.ToolRiskLow}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "restart_service", Description: "restart service", RiskLevel: tooling.ToolRiskHigh, Mutating: true, RequiresApproval: true}},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Tools.Content
	if got := strings.Count(content, "Common policy:"); got != 1 {
		t.Fatalf("Common policy count = %d, want 1:\n%s", got, content)
	}
	for _, forbidden := range []string{"Usage policy:", "Example:", "Failure handling:"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("compact tool prompt should not include %q:\n%s", forbidden, content)
		}
	}
}

func TestToolPromptSetOmitsHiddenFromPromptTools(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			governancePromptTool{meta: tooling.ToolMetadata{Name: "synthetic.hidden_1", Description: "hidden 1", Discovery: tooling.ToolDiscoveryMetadata{HiddenFromPrompt: true}}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "synthetic.hidden_2", Description: "hidden 2", Discovery: tooling.ToolDiscoveryMetadata{HiddenFromPrompt: true}}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "synthetic.internal", Description: "internal", Layer: tooling.ToolLayerInternal}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "synthetic.visible", Description: "Get visible evidence"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	for _, forbidden := range []string{"synthetic.hidden_1", "synthetic.hidden_2", "synthetic.internal"} {
		if strings.Contains(compiled.Tools.Content, forbidden) {
			t.Fatalf("tool prompt contains removed tool %q:\n%s", forbidden, compiled.Tools.Content)
		}
	}
	if !strings.Contains(compiled.Tools.Content, "synthetic_visible") {
		t.Fatalf("tool prompt should keep visible tool:\n%s", compiled.Tools.Content)
	}
}

func TestToolPromptSetRendersCompactDeferredDirectoryWithoutSchemas(t *testing.T) {
	compiler := NewCompiler()
	deferred := schemaPromptTool{
		readOnlyGovernancePromptTool: readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{
			Name:           "coroot.service_metrics",
			Description:    "Read service metric summaries from an external observability source",
			Layer:          tooling.ToolLayerDeferred,
			Pack:           "coroot_metrics",
			DeferByDefault: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:     "metrics",
				ResourceTypes:      []string{"service", "resource"},
				OperationKinds:     []string{"read", "query"},
				LoadingPolicy:      tooling.ToolLoadingPolicyDeferred,
				MCPServerID:        "coroot",
				RequiresHealthyMCP: true,
				RequiresSelect:     true,
				PermissionScope:    "read",
				SchemaBudgetClass:  "on_demand",
			},
		}},
		inputSchema: json.RawMessage(`{"type":"object","properties":{"appId":{"type":"string"},"fromTimestamp":{"type":"integer"}},"required":["appId"]}`),
	}
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{Name: "tool_search", Description: "Search and select deferred tools", Layer: tooling.ToolLayerCore}},
			readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{Name: "exec_command", Description: "Run bounded direct host inspection", Layer: tooling.ToolLayerCore}},
		},
		DeferredToolCatalog: []Tool{deferred},
		MCPHealthSnapshot:   map[string]string{"coroot": "unavailable"},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Tools.Content
	for _, want := range []string{
		"## Deferred Tool Directory",
		"coroot_metrics",
		"capability=metrics",
		"health=unavailable",
		"select=required",
		"Use tool_search/select before calling any deferred tool",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("deferred directory missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{"coroot.service_metrics", "appId", "fromTimestamp", `"properties"`, `"required"`} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("deferred directory leaked schema/tool detail %q:\n%s", forbidden, content)
		}
	}
	if len(compiled.Tools.DeferredDirectory) != 1 {
		t.Fatalf("deferred directory entries = %d, want 1", len(compiled.Tools.DeferredDirectory))
	}
	if got := compiled.Tools.DeferredDirectory[0].Pack; got != "coroot_metrics" {
		t.Fatalf("deferred directory pack = %q, want coroot_metrics", got)
	}
}

func TestDeferredDirectorySortedByMentionRelevanceAndCapped(t *testing.T) {
	compiler := NewCompiler()
	var catalog []Tool
	catalog = append(catalog,
		deferredDirectoryToolForTest("aa_low_priority", "generic.metrics", "metrics"),
		deferredDirectoryToolForTest("coroot_metrics", "coroot.service_metrics", "metrics"),
	)
	for i := 0; i < maxDeferredToolDirectoryEntries+3; i++ {
		catalog = append(catalog, deferredDirectoryToolForTest(fmt.Sprintf("synthetic_pack_%02d", i), fmt.Sprintf("synthetic.tool_%02d", i), "generic"))
	}

	compiled, err := compiler.Compile(CompileContext{
		CorootState:         "requested",
		DeferredToolCatalog: catalog,
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}

	if len(compiled.Tools.DeferredDirectory) != maxDeferredToolDirectoryEntries {
		t.Fatalf("deferred directory entries = %d, want cap %d", len(compiled.Tools.DeferredDirectory), maxDeferredToolDirectoryEntries)
	}
	if got := compiled.Tools.DeferredDirectory[0].Pack; got != "coroot_metrics" {
		t.Fatalf("first deferred pack = %q, want coroot_metrics; entries=%#v", got, compiled.Tools.DeferredDirectory)
	}
}

func TestBaseRuntimeContractThinSections(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{Mode: "chat"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Policy.Content
	for _, want := range []string{
		"# Runtime State",
		"mode: chat",
		"profile: default",
		"mutation: read-only",
		"web: not_requested",
		"ops_graph: not_requested",
		"coroot: not_requested",
		"ops_manus: not_requested",
		"pending_approvals: 0",
		"pending_evidence: 0",
		"visible_tool_fingerprint: unknown",
		"user_constraints: none",
		"timeout_recovery_state: none",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("runtime state missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{"OpsManual", "Coroot", "HostOps", "AIOps Investigation Loop", "host_manager", "workflow long rules"} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("base runtime contract contains domain/profile rule %q:\n%s", forbidden, content)
		}
	}
}

func TestRuntimePolicyPromptIncludesThinBaseContract(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{Mode: "execute"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Policy.Content
	for _, want := range []string{
		"# Runtime State",
		"mode: execute",
		"mutation: approval_required",
		"pending_approvals: 0",
		"timeout_recovery_state: none",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("runtime policy missing %q:\n%s", want, content)
		}
	}
}

func TestDomainRulesAdvisorPromptOmitsCorootAndOpsManualLongRules(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{Mode: "chat", Profile: "advisor"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := strings.Join([]string{compiled.Policy.Content, compiled.Tools.Content, compiled.Developer.Content}, "\n\n")
	for _, forbidden := range []string{
		"Coroot edge evidence",
		"Z依赖异常导致X异常",
		"resolve_ops_manual_params",
		"Workflow preflight failure",
		"run_ops_manual_preflight",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("advisor prompt leaked domain long rule %q:\n%s", forbidden, content)
		}
	}
}

func TestToolGovernanceRendersLoadedDomainGuidance(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{
		Mode: "inspect",
		AssembledTools: []Tool{
			promptGuidanceTool{
				readOnlyGovernancePromptTool: readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{
					Name:        "coroot.collect_rca_context",
					Description: "Collect Coroot RCA context",
					Domain:      "coroot",
					RiskLevel:   tooling.ToolRiskLow,
				}},
				prompt: "Use Coroot edge evidence as RCA evidence and keep missing observability separate from system health.",
			},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	content := compiled.Tools.Content
	for _, want := range []string{
		"coroot_collect_rca_context",
		"Collect Coroot RCA context",
		"[read_only]",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool prompt missing loaded domain guidance %q:\n%s", want, content)
		}
	}
}

func runtimePolicySectionTitles(content string) []string {
	lines := strings.Split(content, "\n")
	titles := make([]string, 0, 9)
	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			titles = append(titles, strings.TrimSpace(strings.TrimPrefix(line, "## ")))
		}
	}
	return titles
}

type governancePromptTool struct {
	meta tooling.ToolMetadata
}

func deferredDirectoryToolForTest(pack, name, capability string) Tool {
	return readOnlyGovernancePromptTool{meta: tooling.ToolMetadata{
		Name:           name,
		Description:    "Deferred " + capability,
		Layer:          tooling.ToolLayerDeferred,
		Pack:           pack,
		DeferByDefault: true,
		Discovery: tooling.ToolDiscoveryMetadata{
			CapabilityKind:    capability,
			LoadingPolicy:     tooling.ToolLoadingPolicyDeferred,
			RequiresSelect:    true,
			SchemaBudgetClass: "on_demand",
		},
	}}
}

func (t governancePromptTool) Metadata() tooling.ToolMetadata { return t.meta }
func (t governancePromptTool) InputSchema() json.RawMessage   { return nil }
func (t governancePromptTool) OutputSchema() json.RawMessage  { return nil }
func (t governancePromptTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return t.meta.Description
}
func (t governancePromptTool) Prompt(tooling.PromptContext) string { return t.meta.Description }
func (t governancePromptTool) IsEnabled(tooling.ToolContext) bool  { return true }
func (t governancePromptTool) IsReadOnly(json.RawMessage) bool     { return false }
func (t governancePromptTool) IsDestructive(json.RawMessage) bool  { return true }
func (t governancePromptTool) IsConcurrencySafe(json.RawMessage) bool {
	return false
}
func (t governancePromptTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}
func (t governancePromptTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (t governancePromptTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{}, nil
}

type readOnlyGovernancePromptTool struct {
	meta tooling.ToolMetadata
}

func (t readOnlyGovernancePromptTool) Metadata() tooling.ToolMetadata { return t.meta }
func (t readOnlyGovernancePromptTool) InputSchema() json.RawMessage   { return nil }
func (t readOnlyGovernancePromptTool) OutputSchema() json.RawMessage  { return nil }
func (t readOnlyGovernancePromptTool) Description(json.RawMessage, tooling.DescribeContext) string {
	return t.meta.Description
}
func (t readOnlyGovernancePromptTool) Prompt(tooling.PromptContext) string { return t.meta.Description }
func (t readOnlyGovernancePromptTool) IsEnabled(tooling.ToolContext) bool  { return true }
func (t readOnlyGovernancePromptTool) IsReadOnly(json.RawMessage) bool     { return true }
func (t readOnlyGovernancePromptTool) IsDestructive(json.RawMessage) bool  { return false }
func (t readOnlyGovernancePromptTool) IsConcurrencySafe(json.RawMessage) bool {
	return true
}
func (t readOnlyGovernancePromptTool) ValidateInput(context.Context, json.RawMessage) error {
	return nil
}
func (t readOnlyGovernancePromptTool) CheckPermissions(context.Context, json.RawMessage) tooling.PermissionDecision {
	return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
}
func (t readOnlyGovernancePromptTool) Execute(context.Context, json.RawMessage) (tooling.ToolResult, error) {
	return tooling.ToolResult{}, nil
}

type promptGuidanceTool struct {
	readOnlyGovernancePromptTool
	prompt string
}

func (t promptGuidanceTool) Prompt(tooling.PromptContext) string { return t.prompt }

type schemaPromptTool struct {
	readOnlyGovernancePromptTool
	inputSchema json.RawMessage
}

func (t schemaPromptTool) InputSchema() json.RawMessage { return t.inputSchema }
