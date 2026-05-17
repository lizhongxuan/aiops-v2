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

func TestToolPromptSetOmitsRemovedOpsTools(t *testing.T) {
	compiler := NewCompiler()
	compiled, err := compiler.Compile(CompileContext{
		AssembledTools: []Tool{
			governancePromptTool{meta: tooling.ToolMetadata{Name: "runbook.match", Description: "old runbook"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "fallback.plan_exec", Description: "old fallback"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "erp.business_metric", Description: "old erp"}},
			governancePromptTool{meta: tooling.ToolMetadata{Name: "coroot.service_metrics", Description: "Get service metrics"}},
		},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	for _, forbidden := range []string{"runbook.match", "fallback.plan_exec", "erp.business_metric"} {
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
