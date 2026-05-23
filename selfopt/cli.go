package selfopt

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func Main(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("selfopt", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var runID, casesDir, outDir, changed string
	var dashboard, assetDraft, allowRealLLM, llmSuggestions, allowRemoteHost bool
	fs.StringVar(&runID, "run-id", "selfopt-run", "run id")
	fs.StringVar(&casesDir, "cases", "testdata/self_optimization/eval_cases", "case directory")
	fs.StringVar(&outDir, "out", ".data/self-optimization-lab", "output directory")
	fs.StringVar(&changed, "changed", "", "comma-separated changed files")
	fs.BoolVar(&dashboard, "dashboard", false, "write dashboard")
	fs.BoolVar(&assetDraft, "asset-draft", false, "write candidate assets")
	fs.BoolVar(&allowRealLLM, "allow-real-llm", false, "allow real LLM config")
	fs.BoolVar(&llmSuggestions, "llm-suggestions", false, "allow lab LLM suggestions")
	fs.BoolVar(&allowRemoteHost, "allow-remote-host", false, "allow remote host mode")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	cfg := LoadConfig(Options{
		AllowRealLLM:    allowRealLLM,
		LLMSuggestions:  llmSuggestions,
		AllowRemoteHost: allowRemoteHost,
	})
	result, err := Run(RunOptions{
		RunID:      runID,
		CasesDir:   casesDir,
		OutDir:     outDir,
		Changed:    splitCSV(changed),
		Dashboard:  dashboard,
		AssetDraft: assetDraft,
		Config:     cfg,
	})
	if err != nil {
		fmt.Fprintf(stderr, "selfopt: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		fmt.Fprintf(stderr, "selfopt: write latest run: %v\n", err)
		return 1
	}
	if err := os.WriteFile(filepath.Join(outDir, "latest_run.txt"), []byte(result.RunDir+"\n"), 0o644); err != nil {
		fmt.Fprintf(stderr, "selfopt: write latest run: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "selfopt run %s gate=%s dir=%s\n", result.Manifest.RunID, result.Gate.Decision, result.RunDir)
	if result.Gate.Decision == GateBlock {
		return 1
	}
	return 0
}

func splitCSV(value string) []string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
