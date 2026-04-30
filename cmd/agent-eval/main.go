package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aiops-v2/internal/eval"
)

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer, now func() time.Time) int {
	var casesDir string
	var outputDir string
	var agentName string
	var runID string
	var baselinePath string
	var saveBaselinePath string

	flags := flag.NewFlagSet("agent-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&casesDir, "cases", "testdata/eval_cases", "directory containing eval case JSON files")
	flags.StringVar(&outputDir, "out", "", "directory for answers, traces, tool calls, and report.json")
	flags.StringVar(&agentName, "agent", "mock", "agent adapter to run; currently supports mock")
	flags.StringVar(&runID, "run-id", "", "optional run id")
	flags.StringVar(&baselinePath, "baseline", "", "optional baseline report.json to compare against")
	flags.StringVar(&saveBaselinePath, "save-baseline", "", "optional path to write the current report as a baseline")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if outputDir == "" {
		if now == nil {
			now = time.Now
		}
		outputDir = filepath.Join(".data", "eval_runs", now().UTC().Format("20060102T150405Z"))
	}
	agent, err := buildAgent(agentName)
	if err != nil {
		return printError(stderr, err)
	}

	var baseline *eval.Report
	if strings.TrimSpace(baselinePath) != "" {
		report, err := eval.LoadReport(baselinePath)
		if err != nil {
			return printError(stderr, fmt.Errorf("load baseline: %w", err))
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
	}.Run(ctx)
	if err != nil {
		return printError(stderr, err)
	}
	if strings.TrimSpace(saveBaselinePath) != "" {
		if err := eval.SaveReport(saveBaselinePath, report); err != nil {
			return printError(stderr, fmt.Errorf("save baseline: %w", err))
		}
	}
	printReportTo(stdout, report)
	return 0
}

func buildAgent(name string) (eval.Agent, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return eval.MockAgent{}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q; use -agent mock for local smoke tests", name)
	}
}

func printReportTo(w io.Writer, report eval.Report) {
	fmt.Fprintf(w, "eval run: %s\n", report.RunID)
	fmt.Fprintf(w, "output: %s\n", report.OutputDir)
	fmt.Fprintf(w, "summary: %d/%d passed, avg score %.2f\n", report.Summary.Passed, report.Summary.Total, report.Summary.AvgScore)
	for _, c := range report.Cases {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		fmt.Fprintf(w, "- %s [%s] %.2f (%d/%d checks)\n", c.CaseID, status, c.Score, c.PassedChecks, c.TotalChecks)
		if c.Error != "" {
			fmt.Fprintf(w, "  error: %s\n", c.Error)
		}
	}
	if report.BaselineComparison != nil {
		s := report.BaselineComparison.Summary
		fmt.Fprintf(w, "baseline: better=%d worse=%d same=%d new=%d missing=%d\n", s.Better, s.Worse, s.Same, s.New, s.Missing)
		for _, c := range report.BaselineComparison.Cases {
			if c.Status == eval.ComparisonSame {
				continue
			}
			fmt.Fprintf(w, "- %s: %s (%.2f -> %.2f, delta %.2f)\n", c.CaseID, c.Status, c.BaselineScore, c.CurrentScore, c.Delta)
		}
	}
}

func printError(stderr io.Writer, err error) int {
	fmt.Fprintln(stderr, "agent-eval:", err)
	return 1
}
