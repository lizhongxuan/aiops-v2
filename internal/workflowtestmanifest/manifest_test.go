package workflowtestmanifest

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func repositoryManifestPath(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "testdata", "workflow_canvas_mcp", "test-manifest.json")
}

func TestRepositoryManifestEnumeratesEveryDesignCase(t *testing.T) {
	manifest, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	if err := Validate(manifest); err != nil {
		t.Fatal(err)
	}
	if got, want := len(manifest.Cases), 169; got != want {
		t.Fatalf("case count = %d, want %d", got, want)
	}
	if got, want := len(ExpectedDesignCaseIDs()), 169; got != want {
		t.Fatalf("design case count = %d, want %d", got, want)
	}
}

func TestValidateRejectsUnknownFixtureAndDuplicateCase(t *testing.T) {
	manifest, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	manifest.Cases[0].Fixtures = []string{"missing-fixture"}
	manifest.Cases = append(manifest.Cases, manifest.Cases[0])
	err = Validate(manifest)
	if err == nil || !strings.Contains(err.Error(), "unknown fixture") || !strings.Contains(err.Error(), "duplicate case") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestResolvePhaseOnlyAllowsUpwardPreview(t *testing.T) {
	if got, err := ResolvePhase(2, "4"); err != nil || got != 4 {
		t.Fatalf("ResolvePhase(2, 4) = %d, %v", got, err)
	}
	if _, err := ResolvePhase(2, "1"); err == nil {
		t.Fatal("ResolvePhase allowed a phase downgrade")
	}
}

func TestCompareProtectedBaselineRejectsRegressions(t *testing.T) {
	previous, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	current := previous
	current.Cases = append([]Case(nil), previous.Cases...)
	current.CurrentActivationPhase--
	current.Cases[0].Priority = "P2"
	for index := range current.Cases {
		if current.Cases[index].ActivationState == ActivationActive {
			current.Cases[index].ActivationState = ActivationPlanned
			break
		}
	}
	current.Cases = current.Cases[:len(current.Cases)-1]
	err = CompareProtectedBaseline(previous, current)
	if err == nil {
		t.Fatal("CompareProtectedBaseline accepted protected regressions")
	}
	for _, fragment := range []string{"currentActivationPhase", "priority downgraded", "active -> planned", "case deleted"} {
		if !strings.Contains(err.Error(), fragment) {
			t.Errorf("error %q does not include %q", err, fragment)
		}
	}
}

func TestGateAcceptsOnlyExpectedKnownFailureSignature(t *testing.T) {
	manifest, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	selected, err := Select(manifest, SelectOptions{Priorities: []string{"P0", "P1"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(selected) == 0 {
		t.Fatal("repository phase must activate explicit phase-0 regression cases")
	}
	results := make([]CaseResult, 0, len(selected))
	knownFailureIndex := -1
	passCount := 0
	for _, c := range selected {
		switch c.ExpectedOutcome {
		case OutcomePass:
			results = append(results, CaseResult{ID: c.ID, Outcome: ResultPassed})
			passCount++
		case OutcomeKnownFailure:
			if knownFailureIndex == -1 {
				knownFailureIndex = len(results)
			}
			results = append(results, CaseResult{ID: c.ID, Outcome: ResultFailed, FailureSignature: c.ExpectedFailureSignature})
		default:
			t.Fatalf("phase-0 case %s outcome = %q", c.ID, c.ExpectedOutcome)
		}
	}
	if passCount == 0 {
		t.Fatal("repository phase must exercise at least one passing case")
	}
	if knownFailureIndex == -1 {
		t.Fatal("repository phase must retain at least one expected known failure")
	}
	if err := Gate(manifest, results, SelectOptions{Priorities: []string{"P0", "P1"}}); err != nil {
		t.Fatal(err)
	}
	results[knownFailureIndex].FailureSignature = "unexpected failure"
	if err := Gate(manifest, results, SelectOptions{Priorities: []string{"P0", "P1"}}); err == nil {
		t.Fatal("Gate accepted an unexpected failure signature")
	}
}

func TestPhaseFiveRequiresApprovedCapabilityTag(t *testing.T) {
	manifest, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	manifest.CurrentActivationPhase = 5
	manifest.Cases[0].ActivationPhase = 5
	manifest.Cases[0].ActivationState = ActivationActive
	manifest.Cases[0].ExpectedOutcome = OutcomePass
	manifest.Cases[0].ExpectedFailureSignature = ""
	manifest.Cases[0].GreenActivationPhase = nil
	manifest.Cases[0].CapabilityTags = []string{"optional:publish-control"}
	if err := Validate(manifest); err == nil || !strings.Contains(err.Error(), "approved capabilityTag") {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestRepositoryManifestKeepsCaseLevelPhaseExceptions(t *testing.T) {
	manifest, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	byID := make(map[string]Case, len(manifest.Cases))
	for _, testCase := range manifest.Cases {
		byID[testCase.ID] = testCase
	}
	wants := map[string]int{
		"MCP-007": 4, "MCP-015": 5, "TXN-016": 2,
		"SEC-008": 3, "SEC-009": 3, "SEC-010": 4,
		"UI-025": 4, "UI-026": 4, "UI-027": 4,
		"PW-031": 4, "PW-032": 5, "PW-033": 4,
		"PERF-001": 3, "PERF-005": 4, "FI-001": 3, "FI-008": 1,
	}
	for id, want := range wants {
		testCase := byID[id]
		got := testCase.ActivationPhase
		if testCase.ExpectedOutcome == OutcomeKnownFailure {
			if testCase.GreenActivationPhase == nil {
				t.Fatalf("%s is known_failure without greenActivationPhase", id)
			}
			got = *testCase.GreenActivationPhase
		}
		if got != want {
			t.Errorf("%s effective green phase = %d, want %d", id, got, want)
		}
	}
}

func TestProfileOptionsSelectExpectedPriorities(t *testing.T) {
	tests := []struct {
		profile    string
		priorities []string
		redOnly    bool
	}{
		{ProfilePR, []string{"P0"}, false},
		{ProfileNightly, []string{"P0", "P1"}, false},
		{ProfileRC, []string{"P0", "P1"}, false},
		{ProfileRedEvidence, []string{"P0", "P1", "P2"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.profile, func(t *testing.T) {
			profile, err := ResolveProfile(tt.profile)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Join(profile.Priorities, ",") != strings.Join(tt.priorities, ",") || profile.KnownFailuresOnly != tt.redOnly {
				t.Fatalf("profile = %#v", profile)
			}
		})
	}
	if _, err := ResolveProfile("unknown"); err == nil {
		t.Fatal("ResolveProfile accepted an unknown profile")
	}
}

func TestBuildGateReportDistinguishesEveryOutcome(t *testing.T) {
	manifest, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Cases {
		if manifest.Cases[index].ActivationState == ActivationActive {
			manifest.Cases[index].ExpectedOutcome = OutcomePass
			manifest.Cases[index].ExpectedFailureSignature = ""
			manifest.Cases[index].GreenActivationPhase = nil
			break
		}
	}
	selected, err := Select(manifest, SelectOptions{Priorities: []string{"P0", "P1"}})
	if err != nil {
		t.Fatal(err)
	}
	results := make([]CaseResult, 0, len(selected))
	for index, testCase := range selected {
		result := CaseResult{ID: testCase.ID, Outcome: ResultFailed, FailureSignature: testCase.ExpectedFailureSignature}
		switch index {
		case 0:
			result = CaseResult{ID: testCase.ID, Outcome: ResultPassed}
		case 1:
			result = CaseResult{ID: testCase.ID, Outcome: ResultSkipped}
		case 2:
			continue // missing
		}
		results = append(results, result)
	}
	report, gateErr := BuildGateReport(manifest, results, SelectOptions{Priorities: []string{"P0", "P1"}})
	if gateErr == nil {
		t.Fatal("report accepted skipped, unexpected pass, and missing active cases")
	}
	for _, status := range []string{StatusPlanned, StatusExpectedRed, StatusMissing, StatusPassed, StatusSkipped} {
		if report.Counts[status] == 0 {
			t.Fatalf("report counts = %#v, want %s", report.Counts, status)
		}
	}
	var junit bytes.Buffer
	if err := WriteJUnit(&junit, report); err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"<testsuite", "<skipped", "<failure"} {
		if !strings.Contains(junit.String(), fragment) {
			t.Fatalf("JUnit report missing %q: %s", fragment, junit.String())
		}
	}
}

func TestCompareProtectedBaselineRejectsExpectedPassToKnownFailure(t *testing.T) {
	previous, err := Load(repositoryManifestPath(t))
	if err != nil {
		t.Fatal(err)
	}
	current := previous
	current.Cases = append([]Case(nil), previous.Cases...)
	for index := range current.Cases {
		if current.Cases[index].ExpectedOutcome == OutcomePass {
			green := 5
			current.Cases[index].ExpectedOutcome = OutcomeKnownFailure
			current.Cases[index].ExpectedFailureSignature = "new red"
			current.Cases[index].GreenActivationPhase = &green
			break
		}
	}
	if err := CompareProtectedBaseline(previous, current); err == nil || !strings.Contains(err.Error(), "pass -> known_failure") {
		t.Fatalf("CompareProtectedBaseline() error = %v", err)
	}
}
