package promptcompiler

import (
	"context"
	"encoding/json"
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
		"Governance: risk=high",
		"mutating=true",
		"approval=required",
		"resultBudget=2048",
		"failure=fail_turn",
		"runtime approval gate",
	} {
		if !strings.Contains(content, want) {
			t.Fatalf("tool prompt missing %q:\n%s", want, content)
		}
	}
	for _, forbidden := range []string{
		"after approval",
		"Requires approval before execution",
		"Use only after confirming",
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
		"Read-only tool failure is missing or blocked evidence, not proof of target state.",
		"Permission denied or policy blocked does not prove system health.",
		"Non-zero exit requires stderr and exit code interpretation.",
		"Empty output does not prove no abnormality.",
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
		"Mutating tools require explicit user intent, scoped target, runtime approval gate, and verification.",
		"Failed mutations must stop at the scoped action and must not broaden scope.",
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

func TestToolPromptSetOmitsRemovedOpsTools(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			governancePromptTool{meta: tooling.ToolMetadata{Name: "k8s.get_events", Description: "old k8s"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "changes.recent_deployments", Description: "old changes"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "runbook.match", Description: "old runbook"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "fallback.plan_exec", Description: "old fallback"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "erp.business_metric", Description: "old erp"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "coroot.service_metrics", Description: "Get service metrics"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	for _, forbidden := range []string{"k8s.get_events", "changes.recent_deployments", "runbook.match", "fallback.plan_exec", "erp.business_metric"} {
		if strings.Contains(compiled.Tools.Content, forbidden) {
			t.Fatalf("tool prompt contains removed tool %q:\n%s", forbidden, compiled.Tools.Content)
		}
	}
	if !strings.Contains(compiled.Tools.Content, "coroot.service_metrics") {
		t.Fatalf("tool prompt should keep coroot tool:\n%s", compiled.Tools.Content)
	}
}

type governancePromptTool struct {
	meta tooling.ToolMetadata
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
