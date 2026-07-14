package eval

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
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

func TestHarnessContractReplayCorpusCoversApprovalResumeAndToolNotFound(t *testing.T) {
	for _, name := range []string{"approval_resume", "tool_not_found"} {
		path := filepath.Join("testdata", "rollout_replay", name+".json")
		fixture, loadErr := LoadRolloutReplayFixtureFile(path)
		if loadErr != nil {
			t.Fatalf("load replay fixture %s: %v", path, loadErr)
		}
		if fixture.Name != name {
			t.Fatalf("replay fixture %s name = %q", path, fixture.Name)
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

func TestHarnessServerAgentUsesAssistantTransportFactsOnly(t *testing.T) {
	paths, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatalf("glob internal/eval Go source: %v", err)
	}
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(data)
		for _, forbidden := range []string{"serverStateSnapshot", "serverChatResponse"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s retains legacy symbol %q", path, forbidden)
			}
		}
		for _, value := range evalGoStringConstants(t, path) {
			for _, forbidden := range []string{"/api/v1/state", "/api/v1/chat/message"} {
				if strings.Contains(value, forbidden) {
					t.Fatalf("%s retains legacy endpoint constant %q", path, value)
				}
			}
		}
	}
	transport, err := os.ReadFile("server_agent_transport.go")
	if err != nil {
		t.Fatalf("read server_agent_transport.go: %v", err)
	}
	for _, required := range []string{
		"/api/v1/assistant/transport", "appui.AiopsTransportState", "serverTransportSettled", "RuntimeLiveness", "PendingApprovals",
	} {
		if !strings.Contains(string(transport), required) {
			t.Fatalf("server eval source missing AssistantTransport contract %q", required)
		}
	}
}

func TestEvalGoStringConstantDetectsConcatenatedLegacyEndpoints(t *testing.T) {
	tests := map[string]string{
		`"/api/v1/" + "state"`:               "/api/v1/state",
		`("/api/v1/" + "chat/") + "message"`: "/api/v1/chat/message",
	}
	for source, want := range tests {
		expression, err := parser.ParseExpr(source)
		if err != nil {
			t.Fatalf("parse %s: %v", source, err)
		}
		if got, ok := evalGoStringConstant(expression); !ok || got != want {
			t.Fatalf("evalGoStringConstant(%s) = %q, %v; want %q, true", source, got, ok, want)
		}
	}
}

func evalGoStringConstants(t *testing.T, path string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	values := []string{}
	ast.Inspect(parsed, func(node ast.Node) bool {
		if expression, ok := node.(ast.Expr); ok {
			if value, ok := evalGoStringConstant(expression); ok {
				values = append(values, value)
			}
		}
		return true
	})
	return values
}

func evalGoStringConstant(expression ast.Expr) (string, bool) {
	switch value := expression.(type) {
	case *ast.BasicLit:
		if value.Kind != token.STRING {
			return "", false
		}
		decoded, err := strconv.Unquote(value.Value)
		return decoded, err == nil
	case *ast.BinaryExpr:
		if value.Op != token.ADD {
			return "", false
		}
		left, leftOK := evalGoStringConstant(value.X)
		right, rightOK := evalGoStringConstant(value.Y)
		return left + right, leftOK && rightOK
	case *ast.ParenExpr:
		return evalGoStringConstant(value.X)
	default:
		return "", false
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
