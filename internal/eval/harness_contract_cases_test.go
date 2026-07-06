package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const harnessGoldenSchemaVersion = "aiops.harness.golden.v1"

type harnessGoldenCase struct {
	Name            string                   `json:"name"`
	ExpectedStatus  string                   `json:"expectedStatus"`
	ExpectedHarness harnessGoldenExpectation `json:"expectedHarness"`
}

type harnessGoldenExpectation struct {
	SchemaVersion            string           `json:"schemaVersion"`
	RouteMode                string           `json:"routeMode"`
	TargetBinding            string           `json:"targetBinding"`
	ModelVisibleTools        []string         `json:"modelVisibleTools,omitempty"`
	HiddenTools              []map[string]any `json:"hiddenTools,omitempty"`
	ToolCalls                []string         `json:"toolCalls,omitempty"`
	FinalStatus              string           `json:"finalStatus,omitempty"`
	TimelineOrder            []string         `json:"timelineOrder"`
	RollbackContractRequired *bool            `json:"rollbackContractRequired,omitempty"`
}

func TestHarnessContractGoldenCasesCoverRequiredScenarios(t *testing.T) {
	dir := filepath.Join("..", "runtimekernel", "testdata", "aichat_harness_golden")
	cases := loadHarnessGoldenCasesForEvalTest(t, dir)

	required := []string{
		"basic_no_tool",
		"single_readonly_tool",
		"tool_not_found",
		"invalid_arguments",
		"approval_resume",
		"approval_denied",
		"host_bound_readonly",
		"evidence_rca_no_exec",
		"multi_host_manager",
		"mutation_missing_target",
		"mutation_missing_rollback",
		"partial_mutation_postcheck_failed",
		"context_compaction_resume",
		"cancelled_running_tool",
		"raw_dsml_markup_sanitized",
		"host_agent_unavailable_fallback_denied",
		"same_session_host_carryover",
	}

	for _, name := range required {
		if _, ok := cases[name]; !ok {
			t.Fatalf("missing harness golden case %q in %s", name, dir)
		}
	}
}

func TestHarnessContractGoldenCasesDeclareTraceExpectations(t *testing.T) {
	dir := filepath.Join("..", "runtimekernel", "testdata", "aichat_harness_golden")
	for _, tc := range loadHarnessGoldenCasesForEvalTest(t, dir) {
		expected := tc.ExpectedHarness
		if expected.SchemaVersion != harnessGoldenSchemaVersion {
			t.Fatalf("%s expectedHarness.schemaVersion = %q, want %q", tc.Name, expected.SchemaVersion, harnessGoldenSchemaVersion)
		}
		if strings.TrimSpace(expected.RouteMode) == "" {
			t.Fatalf("%s missing expectedHarness.routeMode", tc.Name)
		}
		if strings.TrimSpace(expected.TargetBinding) == "" {
			t.Fatalf("%s missing expectedHarness.targetBinding", tc.Name)
		}
		if len(expected.TimelineOrder) == 0 {
			t.Fatalf("%s missing expectedHarness.timelineOrder", tc.Name)
		}
		if len(expected.ModelVisibleTools) == 0 &&
			len(expected.HiddenTools) == 0 &&
			len(expected.ToolCalls) == 0 &&
			strings.TrimSpace(expected.FinalStatus) == "" &&
			expected.RollbackContractRequired == nil {
			t.Fatalf("%s expectedHarness does not assert any runtime contract facts", tc.Name)
		}
	}
}

func loadHarnessGoldenCasesForEvalTest(t *testing.T, dir string) map[string]harnessGoldenCase {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read golden dir %s: %v", dir, err)
	}
	out := make(map[string]harnessGoldenCase, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		var tc harnessGoldenCase
		if err := json.Unmarshal(data, &tc); err != nil {
			t.Fatalf("decode %s: %v", path, err)
		}
		if strings.TrimSpace(tc.Name) == "" {
			t.Fatalf("%s missing name", path)
		}
		if _, exists := out[tc.Name]; exists {
			t.Fatalf("duplicate harness golden case %q", tc.Name)
		}
		out[tc.Name] = tc
	}
	if len(out) == 0 {
		t.Fatalf("no harness golden cases found in %s", dir)
	}
	return out
}
