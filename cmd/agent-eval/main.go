package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aiops-v2/internal/eval"
)

func main() {
	var casesDir string
	var outputDir string
	var agentName string
	var runID string
	var baselinePath string
	var saveBaselinePath string

	flag.StringVar(&casesDir, "cases", "testdata/eval_cases", "directory containing eval case JSON files")
	flag.StringVar(&outputDir, "out", "", "directory for answers, traces, tool calls, and report.json")
	flag.StringVar(&agentName, "agent", "mock", "agent adapter to run; currently supports mock")
	flag.StringVar(&runID, "run-id", "", "optional run id")
	flag.StringVar(&baselinePath, "baseline", "", "optional baseline report.json to compare against")
	flag.StringVar(&saveBaselinePath, "save-baseline", "", "optional path to write the current report as a baseline")
	flag.Parse()

	if outputDir == "" {
		outputDir = filepath.Join(".data", "eval_runs", time.Now().UTC().Format("20060102T150405Z"))
	}
	agent, err := buildAgent(agentName)
	if err != nil {
		exitErr(err)
	}

	var baseline *eval.Report
	if strings.TrimSpace(baselinePath) != "" {
		report, err := eval.LoadReport(baselinePath)
		if err != nil {
			exitErr(fmt.Errorf("load baseline: %w", err))
		}
		baseline = &report
	}

	report, err := eval.Runner{
		CasesDir:       casesDir,
		OutputDir:      outputDir,
		Agent:          agent,
		AgentName:      agentName,
		RunID:          runID,
		BaselineReport: baseline,
	}.Run(context.Background())
	if err != nil {
		exitErr(err)
	}
	if strings.TrimSpace(saveBaselinePath) != "" {
		if err := eval.SaveReport(saveBaselinePath, report); err != nil {
			exitErr(fmt.Errorf("save baseline: %w", err))
		}
	}
	printReport(report)
}

func buildAgent(name string) (eval.Agent, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return eval.MockAgent{}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q; use -agent mock for local smoke tests", name)
	}
}

func printReport(report eval.Report) {
	fmt.Printf("eval run: %s\n", report.RunID)
	fmt.Printf("output: %s\n", report.OutputDir)
	fmt.Printf("summary: %d/%d passed, avg score %.2f\n", report.Summary.Passed, report.Summary.Total, report.Summary.AvgScore)
	for _, c := range report.Cases {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		fmt.Printf("- %s [%s] %.2f (%d/%d checks)\n", c.CaseID, status, c.Score, c.PassedChecks, c.TotalChecks)
		if c.Error != "" {
			fmt.Printf("  error: %s\n", c.Error)
		}
	}
	if report.BaselineComparison != nil {
		s := report.BaselineComparison.Summary
		fmt.Printf("baseline: better=%d worse=%d same=%d new=%d missing=%d\n", s.Better, s.Worse, s.Same, s.New, s.Missing)
		for _, c := range report.BaselineComparison.Cases {
			if c.Status == eval.ComparisonSame {
				continue
			}
			fmt.Printf("- %s: %s (%.2f -> %.2f, delta %.2f)\n", c.CaseID, c.Status, c.BaselineScore, c.CurrentScore, c.Delta)
		}
	}
}

func exitErr(err error) {
	fmt.Fprintln(os.Stderr, "agent-eval:", err)
	os.Exit(1)
}
