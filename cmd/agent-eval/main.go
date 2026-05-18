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
	var priority string
	var runPhase string
	var serverURL string
	var repetitions int
	var pollTimeout time.Duration
	var pollInterval time.Duration

	flags := flag.NewFlagSet("agent-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&casesDir, "cases", "testdata/eval_cases", "directory containing eval case JSON files")
	flags.StringVar(&outputDir, "out", "", "directory for answers, traces, tool calls, and report.json")
	flags.StringVar(&agentName, "agent", "mock", "agent adapter to run; supports mock or server")
	flags.StringVar(&runID, "run-id", "", "optional run id")
	flags.StringVar(&baselinePath, "baseline", "", "optional baseline report.json to compare against")
	flags.StringVar(&saveBaselinePath, "save-baseline", "", "optional path to write the current report as a baseline")
	flags.StringVar(&priority, "priority", "", "optional case priority filter: P0, P1, or P2")
	flags.StringVar(&runPhase, "run-phase", "", "optional run phase metadata: baseline, candidate, or unknown")
	flags.IntVar(&repetitions, "repetitions", 1, "number of times to run each case")
	flags.StringVar(&serverURL, "server-url", "http://localhost:8080", "base URL for -agent server")
	flags.DurationVar(&pollTimeout, "poll-timeout", 2*time.Minute, "maximum time to poll /api/v1/state for -agent server")
	flags.DurationVar(&pollInterval, "poll-interval", 500*time.Millisecond, "poll interval for /api/v1/state with -agent server")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if outputDir == "" {
		if now == nil {
			now = time.Now
		}
		outputDir = filepath.Join(".data", "eval_runs", now().UTC().Format("20060102T150405Z"))
	}
	agent, err := buildAgent(agentName, eval.ServerAgentConfig{
		BaseURL:      serverURL,
		RunID:        runID,
		PollTimeout:  pollTimeout,
		PollInterval: pollInterval,
	})
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
		RunPhase:       runPhase,
		Priority:       priority,
		Repetitions:    repetitions,
		Metadata:       evalRunMetadata(),
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

func buildAgent(name string, serverConfig eval.ServerAgentConfig) (eval.Agent, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return eval.MockAgent{}, nil
	case "server":
		return eval.ServerAgent{Config: serverConfig}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q; use -agent mock or -agent server", name)
	}
}

func printReportTo(w io.Writer, report eval.Report) {
	fmt.Fprintf(w, "eval run: %s\n", report.RunID)
	if report.RunPhase != "" {
		fmt.Fprintf(w, "phase: %s\n", report.RunPhase)
	}
	fmt.Fprintf(w, "output: %s\n", report.OutputDir)
	fmt.Fprintf(w, "summary: %d/%d passed, avg score %.2f, lowest-score avg %.2f, min %.2f\n",
		report.Summary.Passed, report.Summary.Total, report.Summary.AvgScore, report.Summary.LowestScoreAverage, report.Summary.MinScore)
	for _, c := range report.Cases {
		status := "PASS"
		if !c.Passed {
			status = "FAIL"
		}
		iterationText := ""
		if c.Iterations > 1 {
			iterationText = fmt.Sprintf(" avg %.2f min %.2f iterations=%d", c.AvgScore, c.MinScore, c.Iterations)
		}
		fmt.Fprintf(w, "- %s [%s] %.2f%s (%d/%d checks)\n", c.CaseID, status, c.Score, iterationText, c.PassedChecks, c.TotalChecks)
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

func evalRunMetadata() map[string]string {
	return map[string]string{
		"AIOPS_DIAGNOSTIC_PROTOCOL":         os.Getenv("AIOPS_DIAGNOSTIC_PROTOCOL"),
		"AIOPS_DEBUG_MODEL_INPUT_TRACE":     os.Getenv("AIOPS_DEBUG_MODEL_INPUT_TRACE"),
		"AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR": os.Getenv("AIOPS_DEBUG_MODEL_INPUT_TRACE_DIR"),
	}
}
