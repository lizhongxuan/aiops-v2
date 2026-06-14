package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"aiops-v2/internal/agentstate"
)

func TestUXModelGeneralitySyntheticCasesLoad(t *testing.T) {
	cases, err := LoadCases(filepath.Join("testdata", "ux_model_generality_cases.json"))
	if err != nil {
		t.Fatalf("LoadCases() error = %v", err)
	}
	wantIDs := []string{
		"UMG01_simple_no_plan",
		"UMG02_complex_progress_visible",
		"UMG03_resume_continue_next_step",
		"UMG04_manager_synthesis_no_worker_dump",
		"UMG05_reasoning_fallback_provider",
		"UMG06_coverage_blocks_success_final",
		"UMG07_repeated_failure_switch_path",
		"UMG08_generic_resource_type_matrix",
		"UMG09_domain_terms_not_core_rules",
		"UMG10_parameterized_permission_blocker",
	}
	if len(cases) != len(wantIDs) {
		t.Fatalf("cases length = %d, want %d", len(cases), len(wantIDs))
	}
	for i, want := range wantIDs {
		if cases[i].ID != want {
			t.Fatalf("case %d id = %q, want %q", i, cases[i].ID, want)
		}
	}
	assertUXModelGeneralityCasesStaySynthetic(t)
	if len(cases[0].Expected.ExpectedTaskDepth) == 0 || !cases[0].Expected.MustNotHavePlan {
		t.Fatalf("UMG01 missing simple no-plan expectations: %#v", cases[0].Expected)
	}
	if len(cases[5].Expected.ExpectedCoverageAction) == 0 ||
		len(cases[8].Expected.ExpectedGenericityFindings) == 0 ||
		len(cases[9].Expected.ExpectedApprovalScope) == 0 {
		t.Fatalf("synthetic cases missing coverage/genericity/permission expectations: %#v", cases)
	}
}

func TestUXModelGeneralityScorerChecksTraceExpectations(t *testing.T) {
	modelPayload, _ := json.Marshal(map[string]any{
		"taskDepth": map[string]any{
			"level":              "investigation",
			"reasons":            []string{"cross_resource_evidence"},
			"requiresPlan":       true,
			"requiresEvidence":   true,
			"requiresValidation": true,
		},
		"evidenceCoverage": map[string]any{
			"action":             "continue_gathering",
			"missingDimensions":  []string{"verification"},
			"verificationStatus": "PARTIAL",
		},
		"reasoningFallback": map[string]any{
			"policy": "prompt_policy",
		},
		"resumeAction": map[string]any{
			"action": "continue_next_step",
		},
		"managerSynthesis": map[string]any{
			"action": "require_manager_synthesis",
		},
		"failureSignature": map[string]any{
			"action": "switch_path",
		},
		"genericityTrace": map[string]any{
			"allowedPluginTerms": []string{"synthetic_component"},
			"resourceIdSource":   "plugin_metadata",
			"findings":           []string{"allowed_plugin_metadata", "blocked_core_rule:0"},
			"violations":         []string{},
		},
	})
	tc := Case{
		ID:       "UMG-trace",
		Category: "ux_model_generality",
		Input:    "Investigate synthetic_resource_a with generic trace expectations.",
		Expected: Expected{
			ExpectedTaskDepth:          []string{"investigation", "cross_resource_evidence"},
			ExpectedRequiredGates:      []string{"requiresPlan:true", "requiresEvidence:true", "requiresValidation:true"},
			ExpectedCoverageAction:     []string{"continue_gathering", "verification"},
			ExpectedReasoningFallback:  []string{"prompt_policy"},
			ExpectedResumeAction:       []string{"continue_next_step"},
			ExpectedManagerSynthesis:   []string{"require_manager_synthesis"},
			ExpectedFailureAction:      []string{"switch_path"},
			ExpectedGenericityFindings: []string{"allowed_plugin_metadata", "blocked_core_rule:0"},
			ExpectedResourceIDSource:   []string{"plugin_metadata"},
		},
	}
	output := RunOutput{
		Answer: "Trace expectations are present for synthetic_resource_a. 验证方式：go test ./internal/eval。",
		TurnItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: modelPayload}},
		},
	}

	score := ScoreCase(tc, output)

	if !score.Passed {
		t.Fatalf("expected trace checks to pass, got %#v", score)
	}
	for _, name := range []string{
		"expectedTaskDepth",
		"expectedRequiredGates",
		"expectedCoverageAction",
		"expectedReasoningFallback",
		"expectedResumeAction",
		"expectedManagerSynthesis",
		"expectedFailureAction",
		"expectedGenericityFindings",
		"expectedResourceIdSource",
	} {
		if check := findCheck(score.Checks, name); !check.Passed {
			t.Fatalf("check %s = %#v, want pass", name, check)
		}
	}
}

func TestUXModelGeneralityOverPlanningPenaltyFailsSimpleTaskWithPlanTool(t *testing.T) {
	modelPayload, _ := json.Marshal(map[string]any{
		"taskDepth": map[string]any{
			"level":              "simple_read",
			"requiresPlan":       false,
			"requiresEvidence":   false,
			"requiresValidation": false,
		},
	})
	tc := Case{
		ID:       "UMG-overplanning",
		Category: "ux_model_generality",
		Input:    "Read synthetic_resource_b and answer directly.",
		Expected: Expected{
			ExpectedTaskDepth: []string{"simple_read"},
			MustNotHavePlan:   true,
		},
	}
	output := RunOutput{
		Answer:    "synthetic_resource_b is summarized directly. 验证方式：go test ./internal/eval。",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "update_plan"}},
		TurnItems: []agentstate.TurnItem{
			{ID: "model-1", Type: agentstate.TurnItemTypeModelCall, Status: agentstate.ItemStatusCompleted, Payload: agentstate.PayloadEnvelope{Data: modelPayload}},
		},
	}

	score := ScoreCase(tc, output)

	if check := findCheck(score.Checks, "overPlanningPenalty"); check.Passed || len(check.Unexpected) == 0 {
		t.Fatalf("overPlanningPenalty check = %#v, want unexpected planning signal", check)
	}
}

func assertUXModelGeneralityCasesStaySynthetic(t *testing.T) {
	t.Helper()
	path := filepath.Join("testdata", "ux_model_generality_cases.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	text := string(data)
	forbidden := regexp.MustCompile(`(?i)(\b\d{1,3}(?:\.\d{1,3}){3}\b|sk-[A-Za-z0-9_-]{8,}|password\s*=|token\s*=|hardcoded_(site|host|service|cluster|namespace|incident)|fixed_(site|host|service|cluster|namespace|incident))`)
	if forbidden.MatchString(text) {
		t.Fatalf("%s contains non-generic fixture data:\n%s", path, text)
	}
	allowedSyntheticIDs := map[string]bool{
		"synthetic_resource_a": true,
		"synthetic_resource_b": true,
		"synthetic_component":  true,
		"synthetic_endpoint":   true,
		"synthetic_artifact":   true,
	}
	syntheticID := regexp.MustCompile(`synthetic_[a-z0-9_]+`)
	for _, match := range syntheticID.FindAllString(text, -1) {
		if !allowedSyntheticIDs[match] {
			t.Fatalf("unexpected synthetic fixture id %q in %s", match, path)
		}
	}
	for _, realish := range []string{"localhost", "example.com"} {
		if strings.Contains(strings.ToLower(text), realish) {
			t.Fatalf("fixture should not contain %q:\n%s", realish, text)
		}
	}
}
