package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"aiops-v2/internal/workflowtestmanifest"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "workflow-test-manifest:", err)
		os.Exit(1)
	}
}

func run(arguments []string) error {
	if len(arguments) == 0 {
		return fmt.Errorf("usage: workflow-test-manifest <validate|select|gate> [flags]")
	}
	switch arguments[0] {
	case "validate":
		return validateCommand(arguments[1:])
	case "select":
		return selectCommand(arguments[1:])
	case "gate":
		return gateCommand(arguments[1:])
	default:
		return fmt.Errorf("unknown command %q", arguments[0])
	}
}

func validateCommand(arguments []string) error {
	flags := flag.NewFlagSet("validate", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "testdata/workflow_canvas_mcp/test-manifest.json", "manifest path")
	basePath := flags.String("base", "", "protected review-base manifest")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	manifest, err := loadAndValidate(*manifestPath)
	if err != nil {
		return err
	}
	if *basePath != "" {
		base, err := loadAndValidate(*basePath)
		if err != nil {
			return fmt.Errorf("review base: %w", err)
		}
		if err := workflowtestmanifest.CompareProtectedBaseline(base, manifest); err != nil {
			return err
		}
	}
	selected, err := selectCases(manifest, "", "P0,P1", "")
	if err != nil {
		return err
	}
	fmt.Printf("manifest valid: cases=%d fixtures=%d active=%d phase=%d\n", len(manifest.Cases), len(manifest.Fixtures), len(selected), manifest.CurrentActivationPhase)
	return nil
}

func selectCommand(arguments []string) error {
	flags := flag.NewFlagSet("select", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "testdata/workflow_canvas_mcp/test-manifest.json", "manifest path")
	phase := flags.String("phase", os.Getenv("AIOPS_WORKFLOW_CASE_PHASE"), "upward-only phase preview")
	priorities := flags.String("priorities", "P0,P1", "comma-separated priorities")
	profile := flags.String("profile", "", "optional pr, nightly, rc, or red-evidence profile")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	manifest, err := loadAndValidate(*manifestPath)
	if err != nil {
		return err
	}
	selected, err := selectCases(manifest, *phase, *priorities, *profile)
	if err != nil {
		return err
	}
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(selected)
}

func gateCommand(arguments []string) error {
	flags := flag.NewFlagSet("gate", flag.ContinueOnError)
	manifestPath := flags.String("manifest", "testdata/workflow_canvas_mcp/test-manifest.json", "manifest path")
	resultsPath := flags.String("results", "", "case results JSON")
	phase := flags.String("phase", os.Getenv("AIOPS_WORKFLOW_CASE_PHASE"), "upward-only phase preview")
	priorities := flags.String("priorities", "P0,P1", "comma-separated priorities")
	profile := flags.String("profile", "", "optional pr, nightly, rc, or red-evidence profile")
	jsonReport := flags.String("json-report", "", "write the complete gate report as JSON")
	junitReport := flags.String("junit-report", "", "write the complete gate report as JUnit XML")
	if err := flags.Parse(arguments); err != nil {
		return err
	}
	if *resultsPath == "" {
		return fmt.Errorf("--results is required")
	}
	manifest, err := loadAndValidate(*manifestPath)
	if err != nil {
		return err
	}
	results, err := workflowtestmanifest.LoadResults(*resultsPath)
	if err != nil {
		return err
	}
	options, err := selectionOptions(manifest, *phase, *priorities, *profile)
	if err != nil {
		return err
	}
	report, gateErr := workflowtestmanifest.BuildGateReport(manifest, results, options)
	if err := writeReports(report, *jsonReport, *junitReport); err != nil {
		return err
	}
	fmt.Printf("gate summary: active=%d planned=%d expected-red=%d pass=%d fail=%d missing=%d skip=%d\n",
		len(report.Cases)-report.Counts[workflowtestmanifest.StatusPlanned],
		report.Counts[workflowtestmanifest.StatusPlanned], report.Counts[workflowtestmanifest.StatusExpectedRed],
		report.Counts[workflowtestmanifest.StatusPassed], report.Counts[workflowtestmanifest.StatusFailed],
		report.Counts[workflowtestmanifest.StatusMissing], report.Counts[workflowtestmanifest.StatusSkipped])
	if gateErr != nil {
		return gateErr
	}
	fmt.Println("gate passed")
	return nil
}

func loadAndValidate(path string) (workflowtestmanifest.Manifest, error) {
	manifest, err := workflowtestmanifest.Load(path)
	if err != nil {
		return workflowtestmanifest.Manifest{}, err
	}
	if err := workflowtestmanifest.Validate(manifest); err != nil {
		return workflowtestmanifest.Manifest{}, err
	}
	return manifest, nil
}

func selectCases(manifest workflowtestmanifest.Manifest, phase, priorities, profile string) ([]workflowtestmanifest.Case, error) {
	options, err := selectionOptions(manifest, phase, priorities, profile)
	if err != nil {
		return nil, err
	}
	return workflowtestmanifest.Select(manifest, options)
}

func selectionOptions(manifest workflowtestmanifest.Manifest, phase, priorities, profileName string) (workflowtestmanifest.SelectOptions, error) {
	resolved, err := workflowtestmanifest.ResolvePhase(manifest.CurrentActivationPhase, phase)
	if err != nil {
		return workflowtestmanifest.SelectOptions{}, err
	}
	values := make([]string, 0)
	knownFailuresOnly := false
	if strings.TrimSpace(profileName) != "" {
		profile, err := workflowtestmanifest.ResolveProfile(profileName)
		if err != nil {
			return workflowtestmanifest.SelectOptions{}, err
		}
		priorities = strings.Join(profile.Priorities, ",")
		knownFailuresOnly = profile.KnownFailuresOnly
	}
	for _, value := range strings.Split(priorities, ",") {
		if value = strings.TrimSpace(value); value != "" {
			values = append(values, value)
		}
	}
	return workflowtestmanifest.SelectOptions{Phase: &resolved, Priorities: values, KnownFailuresOnly: knownFailuresOnly}, nil
}

func writeReports(report workflowtestmanifest.GateReport, jsonPath, junitPath string) error {
	if jsonPath != "" {
		if err := writeReportFile(jsonPath, func(writer io.Writer) error {
			encoder := json.NewEncoder(writer)
			encoder.SetIndent("", "  ")
			return encoder.Encode(report)
		}); err != nil {
			return fmt.Errorf("write JSON report: %w", err)
		}
	}
	if junitPath != "" {
		if err := writeReportFile(junitPath, func(writer io.Writer) error {
			return workflowtestmanifest.WriteJUnit(writer, report)
		}); err != nil {
			return fmt.Errorf("write JUnit report: %w", err)
		}
	}
	return nil
}

func writeReportFile(path string, write func(io.Writer) error) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if err := write(file); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}
