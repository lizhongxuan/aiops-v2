package eval

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestScoreCaseAppliesContentToolFileAndQualityChecks(t *testing.T) {
	tc := Case{
		ID:       "code-analysis",
		Category: "代码分析",
		Input:    "分析 runtime kernel",
		Expected: Expected{
			MustInclude:       []string{"RuntimeKernel", "AgentEvent"},
			MustNotInclude:    []string{"不知道"},
			ExpectedToolCalls: []string{"read_file"},
			MustMentionFiles:  []string{"internal/runtimekernel/eino_kernel.go"},
		},
	}
	output := RunOutput{
		Answer: "结论：RuntimeKernel 通过 internal/runtimekernel/eino_kernel.go 驱动 turn，并把 AgentEvent 作为验证链路。验证方式：go test ./internal/runtimekernel ./internal/eval。",
		ToolCalls: []ToolCall{
			{ID: "call-1", Name: "read_file", Arguments: json.RawMessage(`{"path":"internal/runtimekernel/eino_kernel.go"}`)},
		},
	}

	score := ScoreCase(tc, output)

	if !score.Passed {
		t.Fatalf("expected score to pass, got %#v", score)
	}
	if score.Score != 1 {
		t.Fatalf("expected full score, got %v", score.Score)
	}
	for _, check := range score.Checks {
		if !check.Passed {
			t.Fatalf("expected check %q to pass: %#v", check.Name, check)
		}
	}
}

func TestScoreCaseDetectsRegressions(t *testing.T) {
	tc := Case{
		ID:       "debug",
		Category: "Debug 排错",
		Input:    "定位失败原因",
		Expected: Expected{
			MustInclude:       []string{"根因"},
			MustNotInclude:    []string{"直接重启"},
			ExpectedToolCalls: []string{"run_command"},
			MustMentionFiles:  []string{"internal/runtimekernel/dispatch.go"},
		},
	}
	output := RunOutput{
		Answer:    "可能是环境问题，建议直接重启。",
		ToolCalls: []ToolCall{{ID: "call-1", Name: "read_file"}},
	}

	score := ScoreCase(tc, output)

	if score.Passed {
		t.Fatalf("expected regression score to fail, got %#v", score)
	}
	failed := map[string]bool{}
	for _, check := range score.Checks {
		if !check.Passed {
			failed[check.Name] = true
		}
	}
	for _, name := range []string{"mustInclude", "mustNotInclude", "expectedToolCalls", "mustMentionFiles", "notVague", "hasVerification"} {
		if !failed[name] {
			t.Fatalf("expected %s to fail, failed checks: %#v", name, failed)
		}
	}
}

func TestRunnerWritesArtifactsAndReport(t *testing.T) {
	casesDir := t.TempDir()
	outDir := t.TempDir()
	writeJSON(t, filepath.Join(casesDir, "code-analysis.json"), Case{
		ID:       "code-analysis",
		Category: "代码分析",
		Input:    "请分析 runtime kernel 的事件链路",
		Expected: Expected{
			MustInclude:       []string{"RuntimeKernel", "AgentEvent"},
			MustNotInclude:    []string{"无法判断"},
			ExpectedToolCalls: []string{"read_file"},
			MustMentionFiles:  []string{"internal/runtimekernel/eino_kernel.go"},
		},
	})

	report, err := Runner{
		CasesDir:  casesDir,
		OutputDir: outDir,
		Agent:     MockAgent{},
		RunID:     "unit-run",
	}.Run(context.Background())
	if err != nil {
		t.Fatalf("run eval: %v", err)
	}
	if report.Summary.Total != 1 || report.Summary.Passed != 1 {
		t.Fatalf("unexpected summary: %#v", report.Summary)
	}

	caseDir := filepath.Join(outDir, "code-analysis")
	for _, name := range []string{"answer.txt", "agent_events.json", "tool_calls.json"} {
		if _, err := os.Stat(filepath.Join(caseDir, name)); err != nil {
			t.Fatalf("expected artifact %s: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(outDir, "report.json")); err != nil {
		t.Fatalf("expected report.json: %v", err)
	}
}

func TestCompareReportsFlagsBetterWorseAndSame(t *testing.T) {
	baseline := Report{Cases: []CaseScore{
		{CaseID: "better", Score: 0.5, Passed: false},
		{CaseID: "worse", Score: 1, Passed: true},
		{CaseID: "same", Score: 0.75, Passed: false},
	}}
	current := Report{Cases: []CaseScore{
		{CaseID: "better", Score: 1, Passed: true},
		{CaseID: "worse", Score: 0.5, Passed: false},
		{CaseID: "same", Score: 0.75, Passed: false},
	}}

	diff := CompareReports(baseline, current)

	got := map[string]string{}
	for _, item := range diff.Cases {
		got[item.CaseID] = item.Status
	}
	want := map[string]string{"better": "better", "worse": "worse", "same": "same"}
	for id, status := range want {
		if got[id] != status {
			t.Fatalf("case %s status = %q, want %q; diff=%#v", id, got[id], status, diff)
		}
	}
	if diff.Summary.Better != 1 || diff.Summary.Worse != 1 || diff.Summary.Same != 1 {
		t.Fatalf("unexpected diff summary: %#v", diff.Summary)
	}
}

func writeJSON(t *testing.T, path string, value any) {
	t.Helper()
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatalf("write json: %v", err)
	}
}
