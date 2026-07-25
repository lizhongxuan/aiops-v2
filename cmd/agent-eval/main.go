package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aiops-v2/internal/eval"
	"aiops-v2/internal/modelrouter"
	"github.com/cloudwego/eino/schema"
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
	var suite string
	var runPhase string
	var serverURL string
	var workflowMode string
	var repetitions int
	var pollTimeout time.Duration
	var pollInterval time.Duration

	flags := flag.NewFlagSet("agent-eval", flag.ContinueOnError)
	flags.SetOutput(stderr)
	flags.StringVar(&casesDir, "cases", "testdata/eval_cases", "directory containing eval case JSON files")
	flags.StringVar(&outputDir, "out", "", "directory for answers, traces, tool calls, and report.json")
	flags.StringVar(&agentName, "agent", "mock", "agent adapter to run; supports mock, server, or live-model")
	flags.StringVar(&runID, "run-id", "", "optional run id")
	flags.StringVar(&baselinePath, "baseline", "", "optional baseline report.json to compare against")
	flags.StringVar(&saveBaselinePath, "save-baseline", "", "optional path to write the current report as a baseline")
	flags.StringVar(&priority, "priority", "", "optional case priority filter: P0, P1, or P2")
	flags.StringVar(&suite, "suite", "", "optional eval suite under -cases, for example multi_agent_assembly")
	flags.StringVar(&runPhase, "run-phase", "", "optional run phase metadata: baseline, candidate, or unknown")
	flags.IntVar(&repetitions, "repetitions", 1, "number of times to run each case")
	flags.StringVar(&serverURL, "server-url", "http://localhost:8080", "base URL for -agent server")
	flags.StringVar(&workflowMode, "workflow-mode", "auto", "workflow generation scoring mode: auto, recorded, or live")
	flags.DurationVar(&pollTimeout, "poll-timeout", 2*time.Minute, "maximum time to wait for the AssistantTransport stream with -agent server")
	flags.DurationVar(&pollInterval, "poll-interval", 500*time.Millisecond, "deprecated compatibility flag; AssistantTransport server runs do not poll")
	if err := flags.Parse(args); err != nil {
		return 2
	}

	if outputDir == "" {
		if now == nil {
			now = time.Now
		}
		outputDir = filepath.Join(".data", "eval_runs", now().UTC().Format("20060102T150405Z"))
	}
	effectiveCasesDir := strings.TrimSpace(casesDir)
	if strings.TrimSpace(suite) != "" {
		cleanSuite := filepath.Clean(strings.TrimSpace(suite))
		if filepath.IsAbs(cleanSuite) || cleanSuite == "." || cleanSuite == ".." || strings.HasPrefix(cleanSuite, ".."+string(os.PathSeparator)) {
			return printError(stderr, fmt.Errorf("invalid suite %q", suite))
		}
		effectiveCasesDir = filepath.Join(effectiveCasesDir, cleanSuite)
	}
	agent, err := buildAgent(ctx, agentName, eval.ServerAgentConfig{
		BaseURL:      serverURL,
		RunID:        runID,
		PollTimeout:  pollTimeout,
		PollInterval: pollInterval,
	})
	if err != nil {
		return printSafeError(stderr, err)
	}
	workflowCases, err := loadWorkflowCases(effectiveCasesDir, priority)
	if err != nil {
		return printSafeError(stderr, err)
	}
	effectiveWorkflowMode, err := resolveWorkflowMode(workflowMode, agentName, len(workflowCases) > 0)
	if err != nil {
		return printSafeError(stderr, err)
	}
	if effectiveWorkflowMode == "live" && repetitions < 3 {
		return printSafeError(stderr, fmt.Errorf("live workflow eval requires -repetitions >= 3"))
	}

	var baseline *eval.Report
	if strings.TrimSpace(baselinePath) != "" {
		report, err := eval.LoadReport(baselinePath)
		if err != nil {
			return printSafeError(stderr, fmt.Errorf("load baseline: %w", err))
		}
		baseline = &report
	}

	metadata := map[string]string{"runtime_settings": "not_loaded_by_agent_eval"}
	if effectiveWorkflowMode != "" {
		metadata["workflow_generation_mode"] = effectiveWorkflowMode
	}
	if strings.TrimSpace(suite) != "" {
		cleanSuite := strings.TrimSpace(suite)
		metadata["suite"] = cleanSuite
		if strings.Contains(strings.ToLower(cleanSuite), "harness") {
			metadata["harness_contract_schema"] = "aiops.harness.golden.v1"
		}
	}
	report, err := eval.Runner{
		CasesDir:       effectiveCasesDir,
		OutputDir:      outputDir,
		Agent:          agent,
		AgentName:      agentName,
		RunID:          runID,
		RunPhase:       runPhase,
		Priority:       priority,
		Repetitions:    repetitions,
		Metadata:       metadata,
		BaselineReport: baseline,
	}.Run(ctx)
	if err != nil {
		return printSafeError(stderr, err)
	}
	secrets := eval.WorkflowEvalSecretValues(os.Environ())
	if err := eval.RedactWorkflowEvalArtifacts(report, secrets); err != nil {
		return printSafeError(stderr, fmt.Errorf("redact workflow eval artifacts: %w", err))
	}
	report = eval.RedactWorkflowEvalReport(report, secrets)
	if len(workflowCases) > 0 {
		if err := persistEvalReport(outputDir, report); err != nil {
			return printSafeError(stderr, err)
		}
		report, err = eval.ScoreWorkflowGenerationReport(report, workflowCases, eval.WorkflowGenerationScoreOptions{Mode: effectiveWorkflowMode, Secrets: secrets})
		if err != nil {
			return printSafeError(stderr, fmt.Errorf("score workflow generation: %w", err))
		}
		if baseline != nil {
			comparison := eval.CompareReports(*baseline, report)
			report.BaselineComparison = &comparison
		}
		report = eval.RedactWorkflowEvalReport(report, secrets)
	}
	if err := persistEvalReport(outputDir, report); err != nil {
		return printSafeError(stderr, err)
	}
	if strings.TrimSpace(saveBaselinePath) != "" {
		if err := eval.SaveReport(saveBaselinePath, report); err != nil {
			return printSafeError(stderr, fmt.Errorf("save baseline: %w", err))
		}
	}
	printReportTo(stdout, report)
	if report.WorkflowGeneration != nil && !report.WorkflowGeneration.Gate.Passed {
		return 1
	}
	return 0
}

func buildAgent(ctx context.Context, name string, serverConfig eval.ServerAgentConfig) (eval.Agent, error) {
	return buildAgentWithLiveModelFactory(ctx, name, serverConfig, modelrouter.NewOpenAIChatModel)
}

type workflowLiveModelFactory func(context.Context, modelrouter.OpenAIConfig) (modelrouter.ChatModel, error)

func buildAgentWithLiveModelFactory(ctx context.Context, name string, serverConfig eval.ServerAgentConfig, newLiveModel workflowLiveModelFactory) (eval.Agent, error) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "", "mock":
		return eval.MockAgent{}, nil
	case "server":
		return eval.ServerAgent{Config: serverConfig}, nil
	case "live-model":
		baseURL := strings.TrimSpace(os.Getenv("AIOPS_LIVE_MODEL_BASE_URL"))
		apiKey := strings.TrimSpace(os.Getenv("AIOPS_LIVE_MODEL_API_KEY"))
		modelName := strings.TrimSpace(os.Getenv("AIOPS_LIVE_MODEL_MODEL"))
		if baseURL == "" || apiKey == "" || modelName == "" {
			return nil, fmt.Errorf("live-model requires AIOPS_LIVE_MODEL_BASE_URL, AIOPS_LIVE_MODEL_API_KEY, and AIOPS_LIVE_MODEL_MODEL")
		}
		if newLiveModel == nil {
			return nil, fmt.Errorf("live-model factory is required")
		}
		model, err := newLiveModel(ctx, workflowLiveModelOpenAIConfig(baseURL, apiKey, modelName))
		if err != nil {
			return nil, fmt.Errorf("create live workflow eval model: %w", err)
		}
		return workflowLiveModelAgent{model: model}, nil
	default:
		return nil, fmt.Errorf("unsupported agent %q; use -agent mock, -agent server, or -agent live-model", name)
	}
}

func workflowLiveModelOpenAIConfig(baseURL, apiKey, modelName string) modelrouter.OpenAIConfig {
	return modelrouter.OpenAIConfig{
		Provider:  "openai",
		BaseURL:   baseURL,
		APIKey:    apiKey,
		Model:     modelName,
		MaxTokens: 64000,
		ExtraFields: map[string]any{
			"response_format": workflowLiveModelResponseFormat(),
		},
	}
}

func workflowLiveModelResponseFormat() map[string]any {
	return map[string]any{
		"type": "json_schema",
		"json_schema": map[string]any{
			"name":        "workflow_generation_attempt",
			"description": "Strict wire shape for one AIOps workflow generation evaluation attempt.",
			// Catalog action arguments and semantic-fact names are intentionally
			// dynamic maps. strict=false lets OpenAI-compatible providers accept
			// those two schema-defined extension points while every structural
			// object still rejects additional properties below.
			"strict": false,
			"schema": workflowLiveAttemptJSONSchema(),
		},
	}
}

type workflowLiveModelAgent struct {
	model modelrouter.ChatModel
}

func (a workflowLiveModelAgent) Run(ctx context.Context, c eval.Case) (eval.RunOutput, error) {
	if a.model == nil || c.WorkflowGeneration == nil {
		return eval.RunOutput{}, fmt.Errorf("live workflow model and workflowGeneration fixture are required")
	}
	fixture := *c.WorkflowGeneration
	var baseline *eval.WorkflowEvalSnapshot
	var initialDiagnostics []string
	for _, recorded := range fixture.RecordedAttempts {
		if recorded.Baseline != nil {
			baseline = recorded.Baseline
		}
		if len(initialDiagnostics) == 0 && len(recorded.FirstPreview.Validation.Diagnostics) > 0 {
			initialDiagnostics = append([]string(nil), recorded.FirstPreview.Validation.Diagnostics...)
		}
	}
	fixture.RecordedAttempts = nil
	caseJSON, err := json.Marshal(map[string]any{"caseId": c.ID, "input": c.Input, "baseline": baseline, "initialDiagnostics": initialDiagnostics, "contract": fixture})
	if err != nil {
		return eval.RunOutput{}, err
	}
	systemPrompt := workflowLiveModelSystemPrompt()
	started := time.Now()
	response, err := a.model.Generate(ctx, []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(string(caseJSON)),
	})
	if err != nil {
		return eval.RunOutput{}, err
	}
	if response == nil {
		return eval.RunOutput{}, fmt.Errorf("live workflow model returned no response")
	}
	attempt, err := decodeWorkflowLiveAttempt(response.Content)
	if err != nil {
		return eval.RunOutput{}, err
	}
	elapsed := time.Since(started).Milliseconds()
	attempt.Metrics.FirstFeedbackMS = elapsed
	attempt.Metrics.PreviewMS = elapsed
	attempt.Metrics.InputTokens = approximateTokens(systemPrompt) + approximateTokens(string(caseJSON))
	attempt.Metrics.OutputTokens = approximateTokens(response.Content)
	attempt.Metrics.ContextTokens = attempt.Metrics.InputTokens + attempt.Metrics.OutputTokens
	answer, err := json.Marshal(attempt)
	if err != nil {
		return eval.RunOutput{}, err
	}
	return eval.RunOutput{Answer: string(answer)}, nil
}

const workflowLiveModelInstructions = `You are a Workflow Plan IR generation evaluator. Return exactly one JSON object that conforms to the OUTPUT JSON SCHEMA below. Use schemaVersion "aiops.workflow-generation.eval-output.v1". The top-level fields are schemaVersion, planHash, compileHash, firstPreview, final, metrics, semanticFacts, and textContent; emit all of them, using empty strings, objects, or arrays where appropriate. Every graph node must use only node types, Catalog actions, argument schemas, and ports in the supplied case contract. Satisfy required/forbidden paths, risk/approval, and semantic invariants. Never invent an action, secret, production parameter, or placeholder. For ambiguity or capability gaps, return the required non-valid status and an empty graph. In particular: graph.nodes and graph.edges are arrays, node.id and edge IDs are strings, the node discriminator is type (never kind), risk is a string, and validation.diagnostics is an array of strings. Do not put a full graph in textContent. JSON only; no Markdown fences.`

func workflowLiveModelSystemPrompt() string {
	encoded, err := json.Marshal(workflowLiveAttemptJSONSchema())
	if err != nil {
		// The schema is assembled exclusively from JSON-compatible literals.
		panic(fmt.Sprintf("marshal workflow live output schema: %v", err))
	}
	return workflowLiveModelInstructions + "\n\nOUTPUT JSON SCHEMA:\n" + string(encoded)
}

func workflowLiveAttemptJSONSchema() map[string]any {
	stringArray := func(description string) map[string]any {
		return map[string]any{
			"type":        "array",
			"description": description,
			"items":       map[string]any{"type": "string"},
		}
	}
	positionSchema := map[string]any{
		"type":                 "object",
		"description":          "Optional canvas coordinates. x and y are JSON numbers.",
		"additionalProperties": false,
		"properties": map[string]any{
			"x": map[string]any{"type": "number"},
			"y": map[string]any{"type": "number"},
		},
		"required": []string{"x", "y"},
	}
	nodeSchema := map[string]any{
		"type":                 "object",
		"description":          "One workflow node. id is a string and type is the discriminator; kind is not a valid field.",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":          map[string]any{"type": "string", "description": "Stable JSON string node ID; never a number."},
			"type":        map[string]any{"type": "string", "description": "Node type from the supplied contract; use this field instead of kind."},
			"label":       map[string]any{"type": "string"},
			"action":      map[string]any{"type": "string", "description": "Exact Catalog action name, or an empty string when the node has no action."},
			"args":        map[string]any{"type": "object", "description": "Catalog action arguments using only keys and JSON value types declared by the supplied contract.", "additionalProperties": true},
			"inputPorts":  stringArray("Input port names as strings."),
			"outputPorts": stringArray("Output port names as strings."),
			"position":    positionSchema,
			"reference":   map[string]any{"type": "string"},
			"pinned":      map[string]any{"type": "boolean"},
		},
		"required": []string{"id", "type"},
	}
	edgeSchema := map[string]any{
		"type":                 "object",
		"description":          "One directed workflow edge. All IDs are JSON strings.",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":         map[string]any{"type": "string"},
			"source":     map[string]any{"type": "string"},
			"sourcePort": map[string]any{"type": "string"},
			"target":     map[string]any{"type": "string"},
			"targetPort": map[string]any{"type": "string"},
		},
		"required": []string{"id", "source", "target"},
	}
	graphSchema := map[string]any{
		"type":                 "object",
		"description":          "Workflow graph. nodes and edges are JSON arrays, never keyed objects.",
		"additionalProperties": false,
		"properties": map[string]any{
			"nodes": map[string]any{"type": "array", "items": nodeSchema},
			"edges": map[string]any{"type": "array", "items": edgeSchema},
		},
		"required": []string{"nodes", "edges"},
	}
	validationSchema := map[string]any{
		"type":                 "object",
		"description":          "Validation result. diagnostics is an array of plain strings, never an object.",
		"additionalProperties": false,
		"properties": map[string]any{
			"status":      map[string]any{"type": "string", "description": "Expected status from the supplied contract, such as valid, invalid, clarification_required, configuration_required, or capability_gap."},
			"valid":       map[string]any{"type": "boolean"},
			"diagnostics": stringArray("Zero or more validation diagnostic strings."),
		},
		"required": []string{"status", "valid", "diagnostics"},
	}
	approvalSchema := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"required": map[string]any{"type": "boolean"},
			"granted":  map[string]any{"type": "boolean"},
		},
		"required": []string{"required", "granted"},
	}
	snapshotSchema := func(description string) map[string]any {
		return map[string]any{
			"type":                 "object",
			"description":          description,
			"additionalProperties": false,
			"properties": map[string]any{
				"graph":      graphSchema,
				"validation": validationSchema,
				"risk":       map[string]any{"type": "string", "description": "Scalar risk level from the supplied contract, for example low or high; never an object."},
				"approval":   approvalSchema,
			},
			"required": []string{"graph", "validation", "risk", "approval"},
		}
	}
	metricsSchema := map[string]any{
		"type":                 "object",
		"description":          "Non-negative integer timing, repair, and token counters.",
		"additionalProperties": false,
		"properties": map[string]any{
			"firstFeedbackMs": map[string]any{"type": "integer", "minimum": 0},
			"previewMs":       map[string]any{"type": "integer", "minimum": 0},
			"validateMs":      map[string]any{"type": "integer", "minimum": 0},
			"repairCount":     map[string]any{"type": "integer", "minimum": 0},
			"inputTokens":     map[string]any{"type": "integer", "minimum": 0},
			"outputTokens":    map[string]any{"type": "integer", "minimum": 0},
			"contextTokens":   map[string]any{"type": "integer", "minimum": 0},
		},
		"required": []string{"firstFeedbackMs", "previewMs", "validateMs", "repairCount", "inputTokens", "outputTokens", "contextTokens"},
	}
	return map[string]any{
		"$schema":              "https://json-schema.org/draft/2020-12/schema",
		"type":                 "object",
		"description":          "One versioned WorkflowGenerationAttempt wire object.",
		"additionalProperties": false,
		"properties": map[string]any{
			"schemaVersion": map[string]any{"type": "string", "const": eval.WorkflowGenerationOutputSchemaVersion},
			"planHash":      map[string]any{"type": "string"},
			"compileHash":   map[string]any{"type": "string"},
			"firstPreview":  snapshotSchema("The first preview snapshot before any repair."),
			"final":         snapshotSchema("The final snapshot after any repair."),
			"metrics":       metricsSchema,
			"semanticFacts": map[string]any{"type": "object", "description": "Named semantic invariants with boolean values.", "additionalProperties": map[string]any{"type": "boolean"}},
			"textContent":   stringArray("Brief user-facing text fragments; never embed a full graph here."),
		},
		"required": []string{"schemaVersion", "planHash", "compileHash", "firstPreview", "final", "metrics", "semanticFacts", "textContent"},
	}
}

func decodeWorkflowLiveAttempt(content string) (eval.WorkflowGenerationAttempt, error) {
	trimmed := strings.TrimSpace(content)
	if strings.HasPrefix(trimmed, "```") {
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "```json"))
		trimmed = strings.TrimSpace(strings.TrimPrefix(trimmed, "```"))
		trimmed = strings.TrimSpace(strings.TrimSuffix(trimmed, "```"))
	}
	var attempt eval.WorkflowGenerationAttempt
	decoder := json.NewDecoder(strings.NewReader(trimmed))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&attempt); err != nil {
		return attempt, fmt.Errorf("live model did not return the versioned workflow eval object: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return attempt, fmt.Errorf("live model returned trailing data after the workflow eval object")
	}
	if attempt.SchemaVersion != eval.WorkflowGenerationOutputSchemaVersion {
		return attempt, fmt.Errorf("live model returned unsupported workflow eval schema %q", attempt.SchemaVersion)
	}
	return attempt, nil
}

func approximateTokens(text string) int {
	runes := len([]rune(text))
	return (runes + 3) / 4
}

func loadWorkflowCases(casesDir, priority string) ([]eval.Case, error) {
	cases, err := eval.LoadCases(casesDir)
	if err != nil {
		return nil, err
	}
	want := strings.ToUpper(strings.TrimSpace(priority))
	if want != "" && want != "P0" && want != "P1" && want != "P2" {
		return nil, fmt.Errorf("invalid priority filter %q", priority)
	}
	workflowCases := make([]eval.Case, 0, len(cases))
	for _, c := range cases {
		if c.WorkflowGeneration != nil && (want == "" || c.Priority == want) {
			workflowCases = append(workflowCases, c)
		}
	}
	return workflowCases, nil
}

func resolveWorkflowMode(mode, agentName string, hasWorkflowCases bool) (string, error) {
	if !hasWorkflowCases {
		return "", nil
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode == "" || mode == "auto" {
		if strings.EqualFold(strings.TrimSpace(agentName), "mock") {
			return "recorded", nil
		}
		return "live", nil
	}
	if mode != "recorded" && mode != "live" {
		return "", fmt.Errorf("unsupported workflow mode %q; use auto, recorded, or live", mode)
	}
	return mode, nil
}

func persistEvalReport(outputDir string, report eval.Report) error {
	if err := eval.SaveReport(filepath.Join(outputDir, "report.json"), report); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(outputDir, "report.md"), []byte(eval.RenderMarkdownReport(report)), 0o644); err != nil {
		return fmt.Errorf("write markdown report: %w", err)
	}
	return nil
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
	if workflow := report.WorkflowGeneration; workflow != nil {
		fmt.Fprintf(w, "workflow generation: mode=%s gate=%t final-valid=%.2f first-preview-valid=%.2f placeholders=%d mean=%.3f variance=%.6f\n",
			workflow.Mode, workflow.Gate.Passed, workflow.Metrics.FinalValidRate, workflow.Metrics.FirstPreviewValidRate,
			workflow.Metrics.PlaceholderCount, workflow.Metrics.MeanScore, workflow.Metrics.ScoreVariance)
		fmt.Fprintf(w, "workflow performance: preview-p95=%dms validate-p95=%dms first-feedback-p95=%dms repairs=%d input/context tokens=%.0f/%.0f\n",
			workflow.Metrics.PreviewP95MS, workflow.Metrics.ValidateP95MS, workflow.Metrics.FirstFeedbackP95MS,
			workflow.Metrics.MaxRepairCount, workflow.Metrics.MeanInputTokens, workflow.Metrics.MeanContextTokens)
		for _, sample := range workflow.FailureSamples {
			fmt.Fprintf(w, "- workflow failure %s iteration=%d checks=%s\n", sample.CaseID, sample.Iteration, strings.Join(sample.FailedChecks, ","))
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

func printSafeError(stderr io.Writer, err error) int {
	return printError(stderr, fmt.Errorf("%s", eval.RedactWorkflowEvalText(err.Error(), eval.WorkflowEvalSecretValues(os.Environ()))))
}
