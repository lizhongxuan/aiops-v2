package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"aiops-v2/internal/promptdiag"
)

func main() {
	os.Exit(runCLI(context.Background(), os.Args[1:], os.Stdout, os.Stderr, time.Now))
}

func runCLI(ctx context.Context, args []string, stdout, stderr io.Writer, now func() time.Time) int {
	var reportPath string
	var baselinePath string
	var casesDir string
	var traceDir string
	var outDir string
	var draftCasesOutDir string
	var historyPath string
	var llmSuggestions bool
	var llmBaseURL string
	var llmAPIKey string
	var llmModel string
	var promptSizeWarning int
	var failOnWorse bool
	var failOnRegression bool

	flags := flag.NewFlagSet("prompt-diagnose", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&reportPath, "report", "", "agent-eval report.json to diagnose")
	flags.StringVar(&baselinePath, "baseline", "", "optional baseline report.json for experiment comparison")
	flags.StringVar(&casesDir, "cases", "testdata/eval_cases", "directory containing eval case JSON files")
	flags.StringVar(&traceDir, "trace-dir", ".data/model-input-traces", "model input trace directory")
	flags.StringVar(&outDir, "out", "", "output directory for diagnosis reports")
	flags.StringVar(&draftCasesOutDir, "draft-cases-out", "", "optional directory to write eval case drafts for failed or regressed cases")
	flags.StringVar(&historyPath, "history", "", "optional prompt optimization history.json path")
	flags.BoolVar(&llmSuggestions, "llm-suggestions", false, "ask an OpenAI-compatible local LLM for summary-only suggestions")
	flags.StringVar(&llmBaseURL, "llm-base-url", "", "OpenAI-compatible LLM base URL for -llm-suggestions")
	flags.StringVar(&llmAPIKey, "llm-api-key", "", "LLM API key for -llm-suggestions")
	flags.StringVar(&llmModel, "llm-model", "", "LLM model for -llm-suggestions")
	flags.IntVar(&promptSizeWarning, "prompt-size-warning", 30000, "prompt size warning threshold in chars")
	flags.BoolVar(&failOnWorse, "fail-on-worse", false, "exit non-zero when baseline comparison has worse cases")
	flags.BoolVar(&failOnRegression, "fail-on-regression", false, "exit non-zero when failed count increases, average score drops, or cases regress")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if strings.TrimSpace(reportPath) == "" {
		fmt.Fprintln(stderr, "prompt-diagnose: -report is required")
		return 2
	}
	if strings.TrimSpace(outDir) == "" {
		outDir = ".data/prompt_optimization/current"
	}
	if now == nil {
		now = time.Now
	}
	diagnosis, err := promptdiag.Diagnose(ctx, promptdiag.Config{
		ReportPath:        reportPath,
		BaselinePath:      baselinePath,
		CasesDir:          casesDir,
		TraceDir:          traceDir,
		OutputDir:         outDir,
		DraftCasesOutDir:  draftCasesOutDir,
		HistoryPath:       historyPath,
		LLMSuggestions:    llmSuggestions,
		LLMBaseURL:        llmBaseURL,
		LLMAPIKey:         llmAPIKey,
		LLMModel:          llmModel,
		PromptSizeWarning: promptSizeWarning,
		GeneratedAt:       now().UTC(),
	})
	if err != nil {
		fmt.Fprintln(stderr, "prompt-diagnose:", err)
		return 1
	}
	if err := promptdiag.WriteOutputs(outDir, diagnosis); err != nil {
		fmt.Fprintln(stderr, "prompt-diagnose:", err)
		return 1
	}
	if err := promptdiag.WriteRunMetadata(outDir, historyPath, diagnosis); err != nil {
		fmt.Fprintln(stderr, "prompt-diagnose:", err)
		return 1
	}
	if strings.TrimSpace(draftCasesOutDir) != "" {
		written, err := promptdiag.WriteDraftCases(ctx, promptdiag.Config{
			ReportPath:   reportPath,
			BaselinePath: baselinePath,
			CasesDir:     casesDir,
			TraceDir:     traceDir,
		}, diagnosis, draftCasesOutDir)
		if err != nil {
			fmt.Fprintln(stderr, "prompt-diagnose:", err)
			return 1
		}
		fmt.Fprintf(stdout, "draft cases: %d written to %s\n", len(written), draftCasesOutDir)
	}
	fmt.Fprintf(stdout, "prompt diagnosis: %s\n", outDir)
	fmt.Fprintf(stdout, "summary: %d/%d passed, failed=%d, worse=%d\n", diagnosis.Summary.Passed, diagnosis.Summary.Total, diagnosis.Summary.Failed, diagnosis.Summary.Worse)
	fmt.Fprintf(stdout, "reports: diagnosis.zh.md compare.zh.md trace-links.md suggestions.zh.md\n")
	if failOnWorse && diagnosis.Summary.Worse > 0 {
		fmt.Fprintf(stderr, "prompt-diagnose: worse cases detected: %d\n", diagnosis.Summary.Worse)
		return 1
	}
	if failOnRegression && (diagnosis.Summary.Worse > 0 || diagnosis.Summary.FailedIncreased || diagnosis.Summary.AvgScoreDropped) {
		fmt.Fprintf(stderr, "prompt-diagnose: regression detected: worse=%d failedIncreased=%t avgScoreDropped=%t\n", diagnosis.Summary.Worse, diagnosis.Summary.FailedIncreased, diagnosis.Summary.AvgScoreDropped)
		return 1
	}
	return 0
}
