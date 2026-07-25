package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkflowScorerGEN001ChecksTypedGraphContracts(t *testing.T) {
	fixture := workflowTestFixture()
	attempt := workflowTestAttempt()
	score, err := ScoreWorkflowGenerationCase("GEN-001", fixture, []WorkflowGenerationAttempt{attempt})
	if err != nil {
		t.Fatal(err)
	}
	if !score.Passed || score.MeanScore != 1 {
		t.Fatalf("score = %#v", score)
	}
	for _, name := range []string{"nodeTypes", "catalogActions", "argumentSchemas", "ports", "requiredPaths", "forbiddenPaths", "placeholders", "validation", "riskApproval", "semanticInvariants", "budgets"} {
		if check := workflowCheck(score.Checks, name); !check.Passed {
			t.Fatalf("check %s = %#v", name, check)
		}
	}
}

func TestWorkflowScorerGEN012FailsClosedOnInventedActionSchemaPortPlaceholderAndApproval(t *testing.T) {
	fixture := workflowTestFixture()
	attempt := workflowTestAttempt()
	attempt.Final.Graph.Nodes[1].Action = "invented.action"
	attempt.Final.Graph.Nodes[1].Args = map[string]any{"threshold": "not-a-number", "script": "<todo>"}
	attempt.Final.Graph.Nodes[1].OutputPorts = nil
	attempt.Final.Risk = "high"
	attempt.Final.Approval = WorkflowEvalApproval{Required: true, Granted: false}

	score, err := ScoreWorkflowGenerationCase("GEN-012", fixture, []WorkflowGenerationAttempt{attempt})
	if err != nil {
		t.Fatal(err)
	}
	if score.Passed || score.PlaceholderCount == 0 {
		t.Fatalf("unsafe output passed: %#v", score)
	}
	for _, name := range []string{"catalogActions", "argumentSchemas", "ports", "placeholders", "riskApproval"} {
		if workflowCheck(score.Checks, name).Passed {
			t.Fatalf("%s unexpectedly passed: %#v", name, score.Checks)
		}
	}
}

func TestWorkflowScorerGEN006RejectsMissingActionAndDanglingPort(t *testing.T) {
	fixture := workflowTestFixture()
	attempt := workflowTestAttempt()
	attempt.Final.Graph.Nodes[1].Action = ""
	attempt.Final.Graph.Edges[1].SourcePort = "unknown"
	score, err := ScoreWorkflowGenerationCase("GEN-006", fixture, []WorkflowGenerationAttempt{attempt})
	if err != nil {
		t.Fatal(err)
	}
	if score.Passed || workflowCheck(score.Checks, "placeholders").Passed || workflowCheck(score.Checks, "ports").Passed {
		t.Fatalf("invalid action/port passed: %#v", score)
	}
}

func TestWorkflowScorerGEN001RejectsUnknownNodeKind(t *testing.T) {
	fixture := workflowTestFixture()
	attempt := workflowTestAttempt()
	attempt.Final.Graph.Nodes = append(attempt.Final.Graph.Nodes, WorkflowEvalNode{ID: "invented", Type: "invented-kind"})
	score, err := ScoreWorkflowGenerationCase("GEN-001", fixture, []WorkflowGenerationAttempt{attempt})
	if err != nil {
		t.Fatal(err)
	}
	check := workflowCheck(score.Checks, "nodeTypes")
	if check.Passed || !containsString(check.Unexpected, "invented-kind") {
		t.Fatalf("node type check = %#v", check)
	}
}

func TestWorkflowScorerGEN008PreservesRequiredPathsAndSemanticBaseline(t *testing.T) {
	fixture := workflowTestFixture()
	fixture.Invariants = append(fixture.Invariants, WorkflowEvalInvariant{Kind: "preserve_path", Path: "/graph/nodes/0"})
	attempt := workflowTestAttempt()
	baseline := workflowTestAttempt().Final
	attempt.Baseline = &baseline
	attempt.Final.Graph.Nodes[0].Label = "mutated start"

	score, err := ScoreWorkflowGenerationCase("GEN-008", fixture, []WorkflowGenerationAttempt{attempt})
	if err != nil {
		t.Fatal(err)
	}
	check := workflowCheck(score.Checks, "semanticInvariants")
	if check.Passed || !containsString(check.Missing, "preserve_path:/graph/nodes/0") {
		t.Fatalf("semantic check = %#v", check)
	}
}

func TestWorkflowScorerGEN014ReportsThresholdsVarianceFailuresAndW4Budgets(t *testing.T) {
	cases := []Case{
		{ID: "GEN-001", Priority: "P0", WorkflowGeneration: ptrWorkflowFixture(workflowTestFixture())},
		{ID: "GEN-014", Priority: "P0", WorkflowGeneration: ptrWorkflowFixture(workflowTestFixture())},
	}
	cases[0].WorkflowGeneration.RecordedAttempts = []WorkflowGenerationAttempt{workflowTestAttempt()}
	a, b, c := workflowTestAttempt(), workflowTestAttempt(), workflowTestAttempt()
	a.Metrics.PreviewMS, b.Metrics.PreviewMS, c.Metrics.PreviewMS = 10, 20, 30
	a.Metrics.ValidateMS, b.Metrics.ValidateMS, c.Metrics.ValidateMS = 15, 25, 35
	c.Final.Validation.Status = "invalid"
	c.Final.Validation.Valid = false
	cases[1].WorkflowGeneration.PerformanceFixture = "W4"
	cases[1].WorkflowGeneration.RecordedAttempts = []WorkflowGenerationAttempt{a, b, c}
	report := Report{Cases: []CaseScore{{CaseID: "GEN-001"}, {CaseID: "GEN-014"}}}

	result, err := ScoreWorkflowGenerationReport(report, cases, WorkflowGenerationScoreOptions{Mode: "recorded"})
	if err != nil {
		t.Fatal(err)
	}
	workflow := result.WorkflowGeneration
	if workflow == nil || workflow.Metrics.FinalValidRate != .75 || workflow.Metrics.FirstPreviewValidRate != 1 {
		t.Fatalf("metrics = %#v", workflow)
	}
	if workflow.Gate.Passed || workflow.Metrics.ScoreVariance <= 0 || len(workflow.FailureSamples) == 0 {
		t.Fatalf("gate/variance/failures = %#v", workflow)
	}
	if workflow.Metrics.PreviewP95MS != 30 || workflow.Metrics.ValidateP95MS != 35 {
		t.Fatalf("p95 metrics = %#v", workflow.Metrics)
	}
}

func TestWorkflowScorerLiveRequiresThreeRepetitionsAndReadsVersionedArtifact(t *testing.T) {
	fixture := workflowTestFixture()
	caseDef := Case{ID: "GEN-live", WorkflowGeneration: &fixture}
	if _, err := ScoreWorkflowGenerationReport(Report{Repetitions: 2, Cases: []CaseScore{{CaseID: caseDef.ID}}}, []Case{caseDef}, WorkflowGenerationScoreOptions{Mode: "live"}); err == nil || !strings.Contains(err.Error(), "at least 3") {
		t.Fatalf("live repetition error = %v", err)
	}

	dir := t.TempDir()
	paths := make([]IterationArtifact, 3)
	for i := range paths {
		path := filepath.Join(dir, string(rune('a'+i))+".json")
		data, _ := json.Marshal(workflowTestAttempt())
		if err := os.WriteFile(path, data, 0o600); err != nil {
			t.Fatal(err)
		}
		paths[i] = IterationArtifact{Iteration: i + 1, AnswerPath: path}
	}
	report := Report{Repetitions: 3, Cases: []CaseScore{{CaseID: caseDef.ID, IterationArtifacts: paths}}}
	result, err := ScoreWorkflowGenerationReport(report, []Case{caseDef}, WorkflowGenerationScoreOptions{Mode: "live"})
	if err != nil || result.WorkflowGeneration == nil || !result.WorkflowGeneration.Gate.Passed {
		t.Fatalf("live report = %#v err=%v", result.WorkflowGeneration, err)
	}
	missing := Report{Repetitions: 3, Cases: []CaseScore{{CaseID: caseDef.ID, IterationArtifacts: []IterationArtifact{{Iteration: 1}, {Iteration: 2}, {Iteration: 3}}}}}
	result, err = ScoreWorkflowGenerationReport(missing, []Case{caseDef}, WorkflowGenerationScoreOptions{Mode: "live"})
	if err != nil || result.WorkflowGeneration == nil || result.WorkflowGeneration.Gate.Passed || len(result.WorkflowGeneration.FailureSamples) == 0 {
		t.Fatalf("missing live output should be a reported failure: %#v err=%v", result.WorkflowGeneration, err)
	}
}

func TestWorkflowScorerRedactsEnvironmentSecretsFromArtifacts(t *testing.T) {
	secret := "super-secret-eval-value"
	text := RedactWorkflowEvalText(`{"Authorization":"Bearer `+secret+`","password":"plain","accessToken":"another-secret"} api_key=leaked-value`, []string{secret})
	if strings.Contains(text, secret) || strings.Contains(text, "plain") || strings.Contains(text, "another-secret") || strings.Contains(text, "leaked-value") || !strings.Contains(text, "[REDACTED]") {
		t.Fatalf("redacted text = %q", text)
	}
	path := filepath.Join(t.TempDir(), "answer.txt")
	if err := os.WriteFile(path, []byte(`api_key=`+secret), 0o644); err != nil {
		t.Fatal(err)
	}
	report := Report{Cases: []CaseScore{{CaseID: "GEN-secret", AnswerPath: path, Error: "Bearer " + secret}}}
	if err := RedactWorkflowEvalArtifacts(report, []string{secret}); err != nil {
		t.Fatal(err)
	}
	artifact, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(artifact), secret) {
		t.Fatalf("artifact = %q err=%v", artifact, err)
	}
	redacted := RedactWorkflowEvalReport(report, []string{secret})
	if strings.Contains(redacted.Cases[0].Error, secret) {
		t.Fatalf("report error was not redacted: %q", redacted.Cases[0].Error)
	}
}

func TestWorkflowScorerPERF005RejectsPrettyGraphInTextContent(t *testing.T) {
	fixture := workflowTestFixture()
	attempt := workflowTestAttempt()
	graph, _ := json.MarshalIndent(attempt.Final.Graph, "", "  ")
	attempt.TextContent = []string{"```json\n" + string(graph) + "\n```"}
	score, err := ScoreWorkflowGenerationCase("PERF-005", fixture, []WorkflowGenerationAttempt{attempt})
	if err != nil {
		t.Fatal(err)
	}
	if score.Passed || workflowCheck(score.Checks, "graphTextBoundary").Passed {
		t.Fatalf("graph copy passed: %#v", score)
	}
}

func BenchmarkWorkflowScorerPERF005W4(b *testing.B) {
	fixture, attempt := workflowW4BenchmarkFixture()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := ScoreWorkflowGenerationCase("PERF-005", fixture, []WorkflowGenerationAttempt{attempt}); err != nil {
			b.Fatal(err)
		}
	}
}

func workflowTestFixture() WorkflowGenerationFixture {
	return WorkflowGenerationFixture{
		SchemaVersion:     WorkflowGenerationCaseSchemaVersion,
		RequiredNodeTypes: []string{"start", "action", "end"}, AllowedActions: []string{"host.inspect"},
		ArgumentSchemas: map[string]WorkflowEvalArgumentSchema{"host.inspect": {Required: []string{"threshold"}, Properties: map[string]string{"threshold": "number"}, AdditionalProperties: false}},
		RequiredPorts:   map[string]WorkflowEvalPortExpectation{"inspect": {Inputs: []string{"in"}, Outputs: []string{"success"}}},
		RequiredPaths:   []string{"/graph/nodes/*/id", "/graph/edges/*/source"}, ForbiddenPaths: []string{"/graph/nodes/*/args/password"},
		ForbiddenPlaceholders: []string{"<todo>", "replace_me", "script.python"},
		Validation:            WorkflowEvalValidationExpectation{FirstPreviewStatus: "valid", FinalStatus: "valid", RateEligible: true},
		Risk:                  WorkflowEvalRiskExpectation{Level: "low", RequiresApproval: false},
		Invariants:            []WorkflowEvalInvariant{{Kind: "acyclic"}, {Kind: "all_paths_end"}, {Kind: "edge", Source: "inspect", SourcePort: "success", Target: "end"}},
		Budgets:               WorkflowEvalBudgets{MaxRepairCount: 2, MaxFirstFeedbackMS: 100, MaxPreviewP95MS: 100, MaxValidateP95MS: 100, MaxInputTokens: 1000, MaxContextTokens: 2000},
	}
}

func workflowTestAttempt() WorkflowGenerationAttempt {
	return WorkflowGenerationAttempt{
		SchemaVersion: WorkflowGenerationOutputSchemaVersion,
		FirstPreview:  WorkflowEvalSnapshot{Validation: WorkflowEvalValidation{Status: "valid", Valid: true}},
		Final: WorkflowEvalSnapshot{
			Graph: WorkflowEvalGraph{
				Nodes: []WorkflowEvalNode{{ID: "start", Type: "start", OutputPorts: []string{"next"}}, {ID: "inspect", Type: "action", Action: "host.inspect", Args: map[string]any{"threshold": 80.0}, InputPorts: []string{"in"}, OutputPorts: []string{"success"}}, {ID: "end", Type: "end", InputPorts: []string{"in"}}},
				Edges: []WorkflowEvalEdge{{ID: "e1", Source: "start", SourcePort: "next", Target: "inspect", TargetPort: "in"}, {ID: "e2", Source: "inspect", SourcePort: "success", Target: "end", TargetPort: "in"}},
			},
			Validation: WorkflowEvalValidation{Status: "valid", Valid: true}, Risk: "low", Approval: WorkflowEvalApproval{Required: false},
		},
		Metrics: WorkflowEvalMetrics{FirstFeedbackMS: 20, PreviewMS: 30, ValidateMS: 40, RepairCount: 0, InputTokens: 300, ContextTokens: 500},
	}
}

func ptrWorkflowFixture(value WorkflowGenerationFixture) *WorkflowGenerationFixture { return &value }
func workflowCheck(checks []CheckResult, name string) CheckResult {
	for _, check := range checks {
		if check.Name == name {
			return check
		}
	}
	return CheckResult{Name: name}
}
func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func workflowW4BenchmarkFixture() (WorkflowGenerationFixture, WorkflowGenerationAttempt) {
	fixture := workflowTestFixture()
	fixture.RequiredPorts = nil
	fixture.Invariants = []WorkflowEvalInvariant{{Kind: "acyclic"}, {Kind: "all_paths_end"}}
	attempt := workflowTestAttempt()
	attempt.Final.Graph = WorkflowEvalGraph{}
	for i := 0; i < 500; i++ {
		node := WorkflowEvalNode{ID: "node-" + string(rune(0x1000+i)), Type: "action", Action: "host.inspect", Args: map[string]any{"threshold": 80.0}, InputPorts: []string{"in"}, OutputPorts: []string{"success"}}
		if i == 0 {
			node.Type, node.Action, node.Args, node.InputPorts = "start", "", nil, nil
		}
		if i == 499 {
			node.Type, node.Action, node.Args, node.OutputPorts = "end", "", nil, nil
		}
		attempt.Final.Graph.Nodes = append(attempt.Final.Graph.Nodes, node)
		if i > 0 {
			attempt.Final.Graph.Edges = append(attempt.Final.Graph.Edges, WorkflowEvalEdge{ID: "edge-" + string(rune(0x2000+i)), Source: attempt.Final.Graph.Nodes[i-1].ID, SourcePort: "success", Target: node.ID, TargetPort: "in"})
		}
	}
	attempt.Final.Graph.Nodes[0].OutputPorts = []string{"success"}
	for i := 0; i < 201; i++ {
		attempt.Final.Graph.Edges = append(attempt.Final.Graph.Edges, WorkflowEvalEdge{ID: "shortcut-" + string(rune(0x3000+i)), Source: attempt.Final.Graph.Nodes[i].ID, SourcePort: "success", Target: attempt.Final.Graph.Nodes[i+2].ID, TargetPort: "in"})
	}
	return fixture, attempt
}
