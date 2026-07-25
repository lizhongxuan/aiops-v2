package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/eval"
	"aiops-v2/internal/modelrouter"
	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

func TestRunCLIExecutesMockEvalSavesBaselineAndPrintsSummary(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	baselinePath := filepath.Join(t.TempDir(), "baseline", "report.json")
	writeCLICase(t, casesDir, "cli-basic")

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-out", outDir,
		"-run-id", "cli-run",
		"-save-baseline", baselinePath,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"eval run: cli-run",
		"output: " + outDir,
		"summary: 1/1 passed, avg score 1.00",
		"- cli-basic [PASS] 1.00",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
	for _, path := range []string{
		filepath.Join(outDir, "report.json"),
		filepath.Join(outDir, "report.md"),
		filepath.Join(outDir, "cli-basic", "answer.txt"),
		baselinePath,
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected CLI artifact %s: %v", path, err)
		}
	}
}

func TestRunCLIPrintsBaselineComparison(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	baselinePath := filepath.Join(t.TempDir(), "baseline.json")
	writeCLICase(t, casesDir, "cli-basic")
	writeCLIJSON(t, baselinePath, eval.Report{Cases: []eval.CaseScore{
		{CaseID: "cli-basic", Score: 0.5, Passed: false},
		{CaseID: "missing-case", Score: 1, Passed: true},
	}})

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-out", outDir,
		"-baseline", baselinePath,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	output := stdout.String()
	for _, want := range []string{
		"baseline: better=1 worse=0 same=0 new=0 missing=1",
		"- cli-basic: better (0.50 -> 1.00, delta 0.50)",
		"- missing-case: missing (1.00 -> 0.00, delta 0.00)",
	} {
		if !strings.Contains(output, want) {
			t.Fatalf("stdout missing %q:\n%s", want, output)
		}
	}
}

func TestRunCLIUsesClockForDefaultOutputDir(t *testing.T) {
	casesDir := t.TempDir()
	workspace := t.TempDir()
	writeCLICase(t, casesDir, "cli-basic")
	t.Chdir(workspace)

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	wantOut := filepath.Join(".data", "eval_runs", "20260428T010203Z")
	if !strings.Contains(stdout.String(), "output: "+wantOut) {
		t.Fatalf("stdout missing default output dir %q:\n%s", wantOut, stdout.String())
	}
	if _, err := os.Stat(filepath.Join(workspace, wantOut, "report.json")); err != nil {
		t.Fatalf("expected default report path: %v", err)
	}
}

func TestRunCLIFiltersCasesByPriority(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLIRawCase(t, filepath.Join(casesDir, "p0.json"), `{"id":"p0-case","category":"CLI","priority":"P0","input":"Run P0","expected":{"mustInclude":["MockAgent"]}}`)
	writeCLIRawCase(t, filepath.Join(casesDir, "p1.json"), `{"id":"p1-case","category":"CLI","priority":"P1","input":"Run P1","expected":{"mustInclude":["MockAgent"]}}`)

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-priority", "P0",
		"-out", outDir,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "- p0-case") || strings.Contains(stdout.String(), "p1-case") {
		t.Fatalf("stdout did not show P0-only run:\n%s", stdout.String())
	}
	report, err := eval.LoadReport(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if len(report.Cases) != 1 || report.Cases[0].CaseID != "p0-case" || report.Cases[0].Priority != "P0" {
		t.Fatalf("report cases = %#v, want only P0 case", report.Cases)
	}
}

func TestRunCLISelectsNamedSuite(t *testing.T) {
	casesRoot := t.TempDir()
	suiteDir := filepath.Join(casesRoot, "multi_agent_assembly")
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLIRawCase(t, filepath.Join(suiteDir, "suite-case.json"), `{"id":"suite-case","category":"multi_agent_assembly","priority":"P0","input":"Run suite","expected":{"mustInclude":["mock agent"]}}`)

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesRoot,
		"-suite", "multi_agent_assembly",
		"-priority", "P0",
		"-out", outDir,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "- suite-case [PASS]") {
		t.Fatalf("stdout missing suite case:\n%s", stdout.String())
	}
	report, err := eval.LoadReport(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if report.CasesDir != suiteDir || report.Metadata["suite"] != "multi_agent_assembly" {
		t.Fatalf("report suite metadata = casesDir %q metadata %#v", report.CasesDir, report.Metadata)
	}
}

func TestRunCLIMarksHarnessContractSuiteMetadata(t *testing.T) {
	casesRoot := t.TempDir()
	suiteDir := filepath.Join(casesRoot, "harness_contract")
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLIRawCase(t, filepath.Join(suiteDir, "harness-case.json"), `{"id":"harness-case","category":"harness_contract","priority":"P0","input":"Run harness","expected":{"mustInclude":["mock agent"]}}`)

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesRoot,
		"-suite", "harness_contract",
		"-out", outDir,
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	report, err := eval.LoadReport(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if report.Metadata["suite"] != "harness_contract" ||
		report.Metadata["harness_contract_schema"] != "aiops.harness.golden.v1" {
		t.Fatalf("report harness metadata = %#v", report.Metadata)
	}
}

func TestRunCLIRecordsRunPhaseRepetitionsAndFlagMetadata(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLICase(t, casesDir, "cli-repeat")

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "mock",
		"-cases", casesDir,
		"-out", outDir,
		"-run-phase", "candidate",
		"-repetitions", "2",
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "phase: candidate") || !strings.Contains(stdout.String(), "iterations=2") {
		t.Fatalf("stdout missing phase/repetition details:\n%s", stdout.String())
	}
	report, err := eval.LoadReport(filepath.Join(outDir, "report.json"))
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	if report.RunPhase != "candidate" || report.Repetitions != 2 {
		t.Fatalf("report run metadata = phase %q reps %d", report.RunPhase, report.Repetitions)
	}
	if report.Metadata["runtime_settings"] != "not_loaded_by_agent_eval" {
		t.Fatalf("report flag metadata missing: %#v", report.Metadata)
	}
}

func writeCLIRawCase(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create case dir: %v", err)
	}
	if err := os.WriteFile(path, []byte(data+"\n"), 0o644); err != nil {
		t.Fatalf("write raw case: %v", err)
	}
}

func TestRunCLIExecutesServerAgent(t *testing.T) {
	casesDir := t.TempDir()
	outDir := filepath.Join(t.TempDir(), "out")
	writeCLICase(t, casesDir, "server-basic")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/assistant/transport" {
			http.NotFound(w, r)
			return
		}
		var request struct {
			Commands []struct {
				Message struct {
					ID       string            `json:"id"`
					Metadata map[string]string `json:"metadata"`
				} `json:"message"`
			} `json:"commands"`
		}
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatalf("decode AssistantTransport request: %v", err)
		}
		if len(request.Commands) != 1 {
			t.Fatalf("commands = %d, want one", len(request.Commands))
		}
		message := request.Commands[0].Message
		clientTurnID := message.Metadata["clientTurnId"]
		if message.ID == "" || clientTurnID == "" {
			t.Fatalf("missing client correlation: message=%#v", message)
		}
		answer := "server adapter completed the local turn through AssistantTransport, with verification: go test ./internal/eval"
		ops := []map[string]any{
			{"type": "set", "path": []any{"currentTurnId"}, "value": "turn-cli-server"},
			{"type": "set", "path": []any{"turns", "turn-cli-server"}, "value": map[string]any{
				"id": "turn-cli-server", "clientTurnId": clientTurnID, "clientMessageId": message.ID,
				"status": "completed", "agentItems": []any{},
				"blockOrder": []string{"final-cli-server"},
				"blocksById": map[string]any{
					"final-cli-server": map[string]any{
						"type": "final_answer", "id": "final-cli-server", "kind": "assistant", "displayKind": "assistant.message",
						"phase": "final_answer", "streamState": "complete", "status": "completed", "text": answer,
						"finalContract": map[string]any{"id": "final-cli-server", "text": answer, "answerText": answer, "status": "completed"},
					},
				},
			}},
			{"type": "set", "path": []any{"status"}, "value": "idle"},
		}
		payload, err := json.Marshal(ops)
		if err != nil {
			t.Fatalf("marshal AssistantTransport ops: %v", err)
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		if _, err := w.Write(append(append([]byte("aui-state:"), payload...), '\n')); err != nil {
			t.Fatalf("write AssistantTransport response: %v", err)
		}
	}))
	defer server.Close()

	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{
		"-agent", "server",
		"-server-url", server.URL,
		"-poll-timeout", "1s",
		"-poll-interval", "1ms",
		"-cases", casesDir,
		"-out", outDir,
		"-run-id", "server-run",
	}, &stdout, &stderr, fixedEvalNow)

	if code != 0 {
		t.Fatalf("runCLI exit code = %d, stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "eval run: server-run") || !strings.Contains(stdout.String(), "summary: 1/1 passed") {
		t.Fatalf("stdout missing server eval summary:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(outDir, "server-basic", "answer.txt")); err != nil {
		t.Fatalf("expected server answer artifact: %v", err)
	}
}

func TestRunCLIReturnsErrorForUnsupportedAgent(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runCLI(context.Background(), []string{"-agent", "real"}, &stdout, &stderr, fixedEvalNow)

	if code != 1 {
		t.Fatalf("runCLI exit code = %d, want 1", code)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty", stdout.String())
	}
	if !strings.Contains(stderr.String(), `unsupported agent "real"`) {
		t.Fatalf("stderr missing unsupported-agent detail:\n%s", stderr.String())
	}
}

func TestWorkflowLiveAttemptJSONSchemaDefinesExactStructuralTypes(t *testing.T) {
	root := workflowLiveAttemptJSONSchema()
	assertSchemaType(t, root, "object")
	assertAdditionalProperties(t, root, false)
	assertRequiredSchemaFields(t, root, "schemaVersion", "planHash", "compileHash", "firstPreview", "final", "metrics", "semanticFacts", "textContent")

	rootProperties := schemaProperties(t, root)
	if got := schemaString(t, schemaObject(t, rootProperties["schemaVersion"]), "const"); got != eval.WorkflowGenerationOutputSchemaVersion {
		t.Fatalf("schemaVersion const = %q, want %q", got, eval.WorkflowGenerationOutputSchemaVersion)
	}
	final := schemaObject(t, rootProperties["final"])
	assertAdditionalProperties(t, final, false)
	assertRequiredSchemaFields(t, final, "graph", "validation", "risk", "approval")
	finalProperties := schemaProperties(t, final)
	assertSchemaType(t, schemaObject(t, finalProperties["risk"]), "string")

	graph := schemaObject(t, finalProperties["graph"])
	assertAdditionalProperties(t, graph, false)
	graphProperties := schemaProperties(t, graph)
	nodes := schemaObject(t, graphProperties["nodes"])
	assertSchemaType(t, nodes, "array")
	node := schemaObject(t, nodes["items"])
	assertAdditionalProperties(t, node, false)
	nodeProperties := schemaProperties(t, node)
	assertSchemaType(t, schemaObject(t, nodeProperties["id"]), "string")
	assertSchemaType(t, schemaObject(t, nodeProperties["type"]), "string")
	if _, exists := nodeProperties["kind"]; exists {
		t.Fatal("node schema unexpectedly permits legacy kind field")
	}
	args := schemaObject(t, nodeProperties["args"])
	assertSchemaType(t, args, "object")
	if allowed, ok := args["additionalProperties"].(bool); !ok || !allowed {
		t.Fatalf("args.additionalProperties = %#v, want true for contract-defined Catalog keys", args["additionalProperties"])
	}

	validation := schemaObject(t, finalProperties["validation"])
	assertAdditionalProperties(t, validation, false)
	diagnostics := schemaObject(t, schemaProperties(t, validation)["diagnostics"])
	assertSchemaType(t, diagnostics, "array")
	assertSchemaType(t, schemaObject(t, diagnostics["items"]), "string")

	metrics := schemaObject(t, rootProperties["metrics"])
	assertAdditionalProperties(t, metrics, false)
	for name, property := range schemaProperties(t, metrics) {
		if got := schemaString(t, schemaObject(t, property), "type"); got != "integer" {
			t.Fatalf("metrics.%s type = %q, want integer", name, got)
		}
	}
	semanticFacts := schemaObject(t, rootProperties["semanticFacts"])
	assertSchemaType(t, schemaObject(t, semanticFacts["additionalProperties"]), "boolean")
}

func TestBuildAgentInjectsWorkflowJSONSchemaResponseFormat(t *testing.T) {
	t.Setenv("AIOPS_LIVE_MODEL_BASE_URL", "https://model.invalid/v1")
	t.Setenv("AIOPS_LIVE_MODEL_API_KEY", "test-only-key")
	t.Setenv("AIOPS_LIVE_MODEL_MODEL", "test-model")

	var captured modelrouter.OpenAIConfig
	agent, err := buildAgentWithLiveModelFactory(context.Background(), "live-model", eval.ServerAgentConfig{}, func(_ context.Context, config modelrouter.OpenAIConfig) (modelrouter.ChatModel, error) {
		captured = config
		return &capturingWorkflowLiveChatModel{response: validWorkflowLiveAttemptJSON(t)}, nil
	})
	if err != nil {
		t.Fatalf("buildAgentWithLiveModelFactory() error = %v", err)
	}
	if _, ok := agent.(workflowLiveModelAgent); !ok {
		t.Fatalf("agent type = %T, want workflowLiveModelAgent", agent)
	}
	if captured.Provider != "openai" || captured.MaxTokens != 64000 {
		t.Fatalf("captured model config = provider %q maxTokens %d", captured.Provider, captured.MaxTokens)
	}
	responseFormat := schemaObject(t, captured.ExtraFields["response_format"])
	if got := schemaString(t, responseFormat, "type"); got != "json_schema" {
		t.Fatalf("response_format.type = %q, want json_schema", got)
	}
	jsonSchema := schemaObject(t, responseFormat["json_schema"])
	if got := schemaString(t, jsonSchema, "name"); got != "workflow_generation_attempt" {
		t.Fatalf("json_schema.name = %q, want workflow_generation_attempt", got)
	}
	if strict, ok := jsonSchema["strict"].(bool); !ok || strict {
		t.Fatalf("json_schema.strict = %#v, want false for dynamic Catalog args", jsonSchema["strict"])
	}
	responseSchema := schemaObject(t, jsonSchema["schema"])
	assertAdditionalProperties(t, responseSchema, false)
	assertSchemaType(t, responseSchema, "object")
}

func TestWorkflowLiveModelAgentProvidesSchemaInEveryPrompt(t *testing.T) {
	model := &capturingWorkflowLiveChatModel{response: validWorkflowLiveAttemptJSON(t)}
	agent := workflowLiveModelAgent{model: model}
	workflowFixture := &eval.WorkflowGenerationFixture{SchemaVersion: "fixture.v1"}

	for _, caseID := range []string{"schema-request-1", "schema-request-2"} {
		if _, err := agent.Run(context.Background(), eval.Case{ID: caseID, Input: "generate workflow", WorkflowGeneration: workflowFixture}); err != nil {
			t.Fatalf("Run(%s) error = %v", caseID, err)
		}
	}
	if len(model.requests) != 2 {
		t.Fatalf("captured requests = %d, want 2", len(model.requests))
	}
	for index, request := range model.requests {
		if len(request) != 2 || request[0].Role != schema.System {
			t.Fatalf("request %d messages = %#v, want system + user", index, request)
		}
		prompt := request[0].Content
		for _, required := range []string{
			"OUTPUT JSON SCHEMA:",
			`"nodes":{"items"`,
			`"risk":{"description"`,
			`"diagnostics":{"description"`,
			`"additionalProperties":false`,
		} {
			if !strings.Contains(prompt, required) {
				t.Fatalf("request %d prompt missing %q", index, required)
			}
		}
	}
}

func TestDecodeWorkflowLiveAttemptRejectsWrongShapesAndUnknownFields(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(map[string]any)
		want   string
	}{
		{
			name: "unknown node kind",
			mutate: func(attempt map[string]any) {
				nodes := liveAttemptNodes(t, attempt)
				nodes[0]["kind"] = "start"
			},
			want: `unknown field "kind"`,
		},
		{
			name: "nodes object",
			mutate: func(attempt map[string]any) {
				liveAttemptGraph(t, attempt)["nodes"] = map[string]any{"n1": map[string]any{"id": "n1", "type": "start"}}
			},
			want: "cannot unmarshal object",
		},
		{
			name: "numeric node id",
			mutate: func(attempt map[string]any) {
				liveAttemptNodes(t, attempt)[0]["id"] = 1
			},
			want: "cannot unmarshal number",
		},
		{
			name: "risk object",
			mutate: func(attempt map[string]any) {
				liveAttemptSnapshot(t, attempt)["risk"] = map[string]any{"level": "low"}
			},
			want: "cannot unmarshal object",
		},
		{
			name: "diagnostics object",
			mutate: func(attempt map[string]any) {
				validation := schemaObject(t, liveAttemptSnapshot(t, attempt)["validation"])
				validation["diagnostics"] = map[string]any{"message": "invalid"}
			},
			want: "cannot unmarshal object",
		},
		{
			name: "unknown top level field",
			mutate: func(attempt map[string]any) {
				attempt["unexpected"] = true
			},
			want: `unknown field "unexpected"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attempt := validWorkflowLiveAttemptObject()
			tt.mutate(attempt)
			encoded, err := json.Marshal(attempt)
			if err != nil {
				t.Fatalf("marshal malformed attempt: %v", err)
			}
			if _, err := decodeWorkflowLiveAttempt(string(encoded)); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("decode error = %v, want detail %q", err, tt.want)
			}
		})
	}
	valid := validWorkflowLiveAttemptJSON(t)
	if _, err := decodeWorkflowLiveAttempt(valid + ` {}`); err == nil || !strings.Contains(err.Error(), "trailing data") {
		t.Fatalf("trailing data error = %v", err)
	}
}

type capturingWorkflowLiveChatModel struct {
	response string
	requests [][]*schema.Message
}

func (m *capturingWorkflowLiveChatModel) Generate(_ context.Context, messages []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	m.requests = append(m.requests, append([]*schema.Message(nil), messages...))
	return schema.AssistantMessage(m.response, nil), nil
}

func (*capturingWorkflowLiveChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (*capturingWorkflowLiveChatModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func validWorkflowLiveAttemptObject() map[string]any {
	snapshot := func() map[string]any {
		return map[string]any{
			"graph": map[string]any{
				"nodes": []any{map[string]any{"id": "n1", "type": "start"}},
				"edges": []any{},
			},
			"validation": map[string]any{"status": "valid", "valid": true, "diagnostics": []any{}},
			"risk":       "low",
			"approval":   map[string]any{"required": false, "granted": false},
		}
	}
	return map[string]any{
		"schemaVersion": eval.WorkflowGenerationOutputSchemaVersion,
		"planHash":      "plan-v1",
		"compileHash":   "compile-v1",
		"firstPreview":  snapshot(),
		"final":         snapshot(),
		"metrics": map[string]any{
			"firstFeedbackMs": 0,
			"previewMs":       0,
			"validateMs":      0,
			"repairCount":     0,
			"inputTokens":     0,
			"outputTokens":    0,
			"contextTokens":   0,
		},
		"semanticFacts": map[string]any{},
		"textContent":   []any{},
	}
}

func validWorkflowLiveAttemptJSON(t *testing.T) string {
	t.Helper()
	encoded, err := json.Marshal(validWorkflowLiveAttemptObject())
	if err != nil {
		t.Fatalf("marshal valid workflow attempt: %v", err)
	}
	return string(encoded)
}

func liveAttemptSnapshot(t *testing.T, attempt map[string]any) map[string]any {
	t.Helper()
	return schemaObject(t, attempt["final"])
}

func liveAttemptGraph(t *testing.T, attempt map[string]any) map[string]any {
	t.Helper()
	return schemaObject(t, liveAttemptSnapshot(t, attempt)["graph"])
}

func liveAttemptNodes(t *testing.T, attempt map[string]any) []map[string]any {
	t.Helper()
	raw, ok := liveAttemptGraph(t, attempt)["nodes"].([]any)
	if !ok {
		t.Fatalf("nodes = %#v, want []any", liveAttemptGraph(t, attempt)["nodes"])
	}
	nodes := make([]map[string]any, len(raw))
	for index, node := range raw {
		nodes[index] = schemaObject(t, node)
	}
	return nodes
}

func schemaObject(t *testing.T, value any) map[string]any {
	t.Helper()
	object, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("schema value = %#v, want object", value)
	}
	return object
}

func schemaProperties(t *testing.T, object map[string]any) map[string]any {
	t.Helper()
	return schemaObject(t, object["properties"])
}

func schemaString(t *testing.T, object map[string]any, field string) string {
	t.Helper()
	value, ok := object[field].(string)
	if !ok {
		t.Fatalf("schema %s = %#v, want string", field, object[field])
	}
	return value
}

func assertSchemaType(t *testing.T, object map[string]any, want string) {
	t.Helper()
	if got := schemaString(t, object, "type"); got != want {
		t.Fatalf("schema type = %q, want %q", got, want)
	}
}

func assertAdditionalProperties(t *testing.T, object map[string]any, want bool) {
	t.Helper()
	got, ok := object["additionalProperties"].(bool)
	if !ok || got != want {
		t.Fatalf("additionalProperties = %#v, want %t", object["additionalProperties"], want)
	}
}

func assertRequiredSchemaFields(t *testing.T, object map[string]any, want ...string) {
	t.Helper()
	raw, ok := object["required"].([]string)
	if !ok {
		t.Fatalf("required = %#v, want []string", object["required"])
	}
	wantSet := make(map[string]bool, len(want))
	for _, field := range want {
		wantSet[field] = true
	}
	if len(raw) != len(wantSet) {
		t.Fatalf("required = %#v, want exactly %#v", raw, want)
	}
	for _, field := range raw {
		if !wantSet[field] {
			t.Fatalf("required contains unexpected field %q; all = %#v", field, raw)
		}
	}
}

func fixedEvalNow() time.Time {
	return time.Date(2026, 4, 28, 1, 2, 3, 0, time.UTC)
}

func writeCLICase(t *testing.T, dir, id string) {
	t.Helper()
	writeCLIJSON(t, filepath.Join(dir, id+".json"), eval.Case{
		ID:       id,
		Category: "CLI",
		Input:    "Run the local eval CLI",
	})
}

func writeCLIJSON(t *testing.T, path string, value any) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("create dir for %s: %v", path, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal json: %v", err)
	}
	if err := os.WriteFile(path, append(data, '\n'), 0o644); err != nil {
		t.Fatalf("write json %s: %v", path, err)
	}
}

func writeCLIJSONResponse(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("encode response json: %v", err)
	}
}
