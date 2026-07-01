package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

type codexRuntimeContractTraceCase struct {
	Name     string                          `json:"name"`
	Input    string                          `json:"input"`
	Expected codexRuntimeContractTraceExpect `json:"expected"`
}

type codexRuntimeContractTraceExpect struct {
	Route                                       string   `json:"route"`
	Profile                                     string   `json:"profile"`
	VisibleTools                                []string `json:"visibleTools"`
	HiddenTools                                 []string `json:"hiddenTools"`
	ToolCalls                                   []string `json:"toolCalls"`
	ApprovalEvents                              []string `json:"approvalEvents,omitempty"`
	EvidenceRefs                                []string `json:"evidenceRefs"`
	Timeline                                    []string `json:"timeline"`
	FinalVerificationStatus                     string   `json:"finalVerificationStatus"`
	PermissionToolSurfaceFingerprint            string   `json:"permissionToolSurfaceFingerprint"`
	NoSyntheticCompletedTurn                    bool     `json:"noSyntheticCompletedTurn"`
	NoDualMechanism                             bool     `json:"noDualMechanism"`
	NoConcurrentRegularTurns                    bool     `json:"noConcurrentRegularTurns"`
	PartialUnknownNotReportedAsConfirmedHealthy bool     `json:"partialUnknownNotReportedAsConfirmedHealthy"`
}

func TestCodexRuntimeContractTraceCasesLoad(t *testing.T) {
	dir := filepath.Join("..", "..", "testdata", "eval_cases", "aiops_codex_runtime_contract_v3")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read runtime contract trace cases: %v", err)
	}
	expectedNames := []string{
		"advisor_no_host_no_exec",
		"evidence_rca_user_logs_no_exec",
		"single_host_readonly_exec",
		"single_host_mutation_approval_approved",
		"single_host_mutation_approval_denied",
		"multi_host_manager_spawn_wait_synthesis",
		"tool_search_deferred_coroot_load",
		"long_turn_compaction_resume",
		"active_turn_pending_input_steer",
		"turn_cancel_aborts_tool_and_resumes_next_turn",
		"approval_permission_snapshot_drift",
		"mutation_resource_lock_conflict",
		"mutation_partial_failure_requires_postcheck",
		"multi_host_partial_child_results",
		"unknown_host_degrades_to_evidence_or_blocker",
	}
	want := make(map[string]bool, len(expectedNames))
	for _, name := range expectedNames {
		want[name] = true
	}

	seen := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read case %s: %v", path, err)
		}
		var tc codexRuntimeContractTraceCase
		if err := json.Unmarshal(data, &tc); err != nil {
			t.Fatalf("parse case %s: %v", path, err)
		}
		if strings.TrimSpace(tc.Name) == "" {
			t.Fatalf("case %s: name is required", path)
		}
		if strings.TrimSuffix(entry.Name(), ".json") != tc.Name {
			t.Fatalf("case %s: filename must match name %q", path, tc.Name)
		}
		if !want[tc.Name] {
			t.Fatalf("case %s: unexpected name %q", path, tc.Name)
		}
		if seen[tc.Name] {
			t.Fatalf("duplicate runtime contract case %q", tc.Name)
		}
		seen[tc.Name] = true
		if strings.TrimSpace(tc.Input) == "" {
			t.Fatalf("case %s: input is required", path)
		}
		if strings.TrimSpace(tc.Expected.Profile) == "" {
			t.Fatalf("case %s: expected.profile is required", path)
		}
		if len(tc.Expected.HiddenTools) == 0 {
			t.Fatalf("case %s: expected.hiddenTools is required", path)
		}
		if !tc.Expected.NoSyntheticCompletedTurn || !tc.Expected.NoDualMechanism || !tc.Expected.NoConcurrentRegularTurns {
			t.Fatalf("case %s: core runtime invariants must be true: %#v", path, tc.Expected)
		}
	}
	if len(seen) != len(want) {
		var missing []string
		for name := range want {
			if !seen[name] {
				missing = append(missing, name)
			}
		}
		sort.Strings(missing)
		t.Fatalf("runtime contract cases = %d, want %d; missing=%v", len(seen), len(want), missing)
	}
}

func TestCodexRuntimeContractV3GoldenTrace(t *testing.T) {
	cases := loadCodexRuntimeContractTraceCases(t)
	if len(cases) != 15 {
		t.Fatalf("golden trace cases = %d, want 15", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			validateCodexRuntimeContractGoldenTrace(t, tc)
		})
	}
}

func loadCodexRuntimeContractTraceCases(t *testing.T) []codexRuntimeContractTraceCase {
	t.Helper()
	dir := filepath.Join("..", "..", "testdata", "eval_cases", "aiops_codex_runtime_contract_v3")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read runtime contract trace cases: %v", err)
	}
	var cases []codexRuntimeContractTraceCase
	for _, entry := range entries {
		if entry.IsDir() || !strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read case %s: %v", path, err)
		}
		var tc codexRuntimeContractTraceCase
		if err := json.Unmarshal(data, &tc); err != nil {
			t.Fatalf("parse case %s: %v", path, err)
		}
		cases = append(cases, tc)
	}
	sort.Slice(cases, func(i, j int) bool {
		return cases[i].Name < cases[j].Name
	})
	return cases
}

func validateCodexRuntimeContractGoldenTrace(t *testing.T, tc codexRuntimeContractTraceCase) {
	t.Helper()
	expect := tc.Expected
	requireRuntimeTraceValue(t, tc.Name, "route", expect.Route)
	requireRuntimeTraceValue(t, tc.Name, "profile", expect.Profile)
	requireRuntimeTraceList(t, tc.Name, "visibleTools", expect.VisibleTools)
	requireRuntimeTraceList(t, tc.Name, "hiddenTools", expect.HiddenTools)
	requireRuntimeTraceList(t, tc.Name, "toolCalls", expect.ToolCalls)
	requireRuntimeTraceList(t, tc.Name, "evidenceRefs", expect.EvidenceRefs)
	requireRuntimeTraceList(t, tc.Name, "timeline", expect.Timeline)
	requireRuntimeTraceValue(t, tc.Name, "finalVerificationStatus", expect.FinalVerificationStatus)
	requireRuntimeTraceValue(t, tc.Name, "permissionToolSurfaceFingerprint", expect.PermissionToolSurfaceFingerprint)

	if !expect.NoSyntheticCompletedTurn || !expect.NoDualMechanism || !expect.NoConcurrentRegularTurns {
		t.Fatalf("case %s: core runtime invariants must be true: %#v", tc.Name, expect)
	}
	for _, required := range []string{"route_selected", "tool_surface_snapshot", "assistant_message(final_answer)"} {
		if !runtimeTraceContains(expect.Timeline, required) {
			t.Fatalf("case %s: timeline missing %q in %#v", tc.Name, required, expect.Timeline)
		}
	}
	if strings.Contains(tc.Name, "approval") && len(expect.ApprovalEvents) == 0 {
		t.Fatalf("case %s: approvalEvents are required for approval scenario", tc.Name)
	}
	if strings.Contains(tc.Name, "permission_snapshot_drift") {
		if expect.PermissionToolSurfaceFingerprint != "requires_reapproval" {
			t.Fatalf("case %s: fingerprint = %q, want requires_reapproval", tc.Name, expect.PermissionToolSurfaceFingerprint)
		}
	} else if expect.PermissionToolSurfaceFingerprint != "stable" {
		t.Fatalf("case %s: fingerprint = %q, want stable", tc.Name, expect.PermissionToolSurfaceFingerprint)
	}
	if strings.Contains(tc.Name, "partial") || strings.Contains(tc.Name, "unknown") || strings.Contains(tc.Name, "denied") || strings.Contains(tc.Name, "cancel") {
		if !expect.PartialUnknownNotReportedAsConfirmedHealthy {
			t.Fatalf("case %s: partial/unknown safety invariant must be true", tc.Name)
		}
		if expect.FinalVerificationStatus == "confirmed_healthy" || expect.FinalVerificationStatus == "success_without_postcheck" {
			t.Fatalf("case %s: unsafe finalVerificationStatus %q", tc.Name, expect.FinalVerificationStatus)
		}
	}
	if expect.Profile == "advisor" || expect.Profile == "evidence_rca" {
		for _, tool := range expect.VisibleTools {
			if tool == "exec_command" {
				t.Fatalf("case %s: exec_command must not be model-visible in profile %s", tc.Name, expect.Profile)
			}
		}
	}
}

func requireRuntimeTraceValue(t *testing.T, caseName, field, value string) {
	t.Helper()
	if strings.TrimSpace(value) == "" {
		t.Fatalf("case %s: expected.%s is required", caseName, field)
	}
}

func requireRuntimeTraceList(t *testing.T, caseName, field string, values []string) {
	t.Helper()
	if len(values) == 0 {
		t.Fatalf("case %s: expected.%s is required", caseName, field)
	}
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			t.Fatalf("case %s: expected.%s contains empty value", caseName, field)
		}
		if seen[value] {
			t.Fatalf("case %s: expected.%s duplicate value %q", caseName, field, value)
		}
		seen[value] = true
	}
}

func runtimeTraceContains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
