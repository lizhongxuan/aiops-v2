package runtimekernel

import (
	"os"
	"strings"
	"testing"
)

func TestGenericityClassifiesCoreRuleAndAllowedContexts(t *testing.T) {
	tests := []struct {
		name string
		path string
		want GenericityFindingCategory
	}{
		{
			name: "core runtime rule is blocked",
			path: "internal/runtimekernel/task_depth_gate.go",
			want: GenericityBlockedCoreRule,
		},
		{
			name: "prompt compiler core rule is blocked",
			path: "internal/promptcompiler/developer_rules.go",
			want: GenericityBlockedCoreRule,
		},
		{
			name: "tool metadata is allowed plugin metadata",
			path: "internal/tooling/discovery_metadata.go",
			want: GenericityAllowedPluginMetadata,
		},
		{
			name: "skill metadata is allowed plugin metadata",
			path: "internal/skills/discovery_metadata.go",
			want: GenericityAllowedPluginMetadata,
		},
		{
			name: "eval testdata is allowed fixture",
			path: "internal/eval/testdata/ux_model_generality_cases.json",
			want: GenericityAllowedTestFixture,
		},
		{
			name: "user fixture is allowed user fixture",
			path: "internal/runtimekernel/testdata/user_fixture/turn.json",
			want: GenericityAllowedUserFixture,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			finding := ClassifyGenericityFinding(tt.path, "synthetic_rule", "synthetic_resource_a")
			if finding.Category != tt.want {
				t.Fatalf("Category = %q, want %q; finding=%#v", finding.Category, tt.want, finding)
			}
			if finding.Path != tt.path || finding.Symbol != "synthetic_rule" || finding.Text != "synthetic_resource_a" {
				t.Fatalf("finding did not preserve context: %#v", finding)
			}
		})
	}
}

func TestGenericityFindingBlockedCoreRuleForUnknownInternalPath(t *testing.T) {
	finding := ClassifyGenericityFinding("internal/runtimekernel/model_input.go", "", "synthetic_resource_b")
	if finding.Category != GenericityBlockedCoreRule {
		t.Fatalf("Category = %q, want %q", finding.Category, GenericityBlockedCoreRule)
	}
	if len(finding.Reasons) == 0 {
		t.Fatalf("finding should explain classification: %#v", finding)
	}
}

func TestCoreRuntimeProductionFilesAvoidProviderSpecificTerms(t *testing.T) {
	term := "coroot"
	for _, path := range []string{
		"model_input.go",
		"eino_kernel.go",
		"tool_pack_intent.go",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if strings.Contains(strings.ToLower(string(data)), term) {
			t.Fatalf("%s contains provider-specific term %q; core runtime rules must use generic metadata/capability/resource signals", path, term)
		}
	}
}

func TestCoreProductionFilesAvoidScenarioSpecificTerms(t *testing.T) {
	terms := []string{"pg_mon", "主机a", "主机b", "主机c", "服务a", "服务b", "服务c"}
	paths := []string{
		"eino_kernel.go",
		"model_input.go",
		"tool_pack_intent.go",
	}
	for _, path := range paths {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		lower := strings.ToLower(string(data))
		for _, term := range terms {
			if strings.Contains(lower, term) {
				t.Fatalf("%s contains scenario-specific term %q; use generic metadata/capability/resource signals", path, term)
			}
		}
	}
}
