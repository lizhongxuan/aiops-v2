package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"runner/server/events"
	"runner/server/metrics"
	"runner/server/queue"
	"runner/server/service"
	"runner/state"
	"runner/workflow"
	"runner/workflow/visual"
)

func TestVisualWorkflowRoutesCompileAndCatalog(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	body := map[string]any{"graph": sampleAPIGraph()}
	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/compile", body)
	if code != http.StatusOK {
		t.Fatalf("compile status = %d payload=%s", code, payload)
	}
	var compiled service.CompiledVisualWorkflow
	if err := json.Unmarshal(payload, &compiled); err != nil {
		t.Fatalf("decode compile response: %v", err)
	}
	if compiled.Workflow.Name != "api-graph" || len(compiled.Workflow.Steps) != 1 {
		t.Fatalf("compiled workflow mismatch: %+v", compiled.Workflow)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/actions/catalog?category=command", nil)
	if code != http.StatusOK {
		t.Fatalf("catalog status = %d payload=%s", code, payload)
	}
	var catalog struct {
		Version      string               `json:"version"`
		Capabilities map[string]any       `json:"capabilities"`
		Items        []service.ActionSpec `json:"items"`
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatalf("decode catalog response: %v", err)
	}
	if catalog.Version != "v1" || catalog.Capabilities["schema"] != "json_schema" {
		t.Fatalf("catalog metadata mismatch: version=%q capabilities=%+v", catalog.Version, catalog.Capabilities)
	}
	if len(catalog.Items) != 1 || catalog.Items[0].Action != "cmd.run" {
		t.Fatalf("catalog filter mismatch: %+v", catalog.Items)
	}

	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/actions?category=command", nil)
	if code != http.StatusOK {
		t.Fatalf("actions alias status = %d payload=%s", code, payload)
	}
	var aliasCatalog struct {
		Items []service.ActionSpec `json:"items"`
	}
	if err := json.Unmarshal(payload, &aliasCatalog); err != nil {
		t.Fatalf("decode actions alias response: %v", err)
	}
	if len(aliasCatalog.Items) != 1 || aliasCatalog.Items[0].Action != "cmd.run" {
		t.Fatalf("actions alias filter mismatch: %+v", aliasCatalog.Items)
	}
}

func TestVisualWorkflowActionCatalogReturnsStructuredIOSchema(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodGet, "/api/v1/actions/catalog?category=command", nil)
	if code != http.StatusOK {
		t.Fatalf("catalog status = %d payload=%s", code, payload)
	}
	var catalog struct {
		Capabilities map[string]any       `json:"capabilities"`
		Items        []service.ActionSpec `json:"items"`
	}
	if err := json.Unmarshal(payload, &catalog); err != nil {
		t.Fatalf("decode catalog response: %v", err)
	}
	if catalog.Capabilities["structured_io_schema"] != true {
		t.Fatalf("structured_io_schema capability missing: %+v", catalog.Capabilities)
	}
	if len(catalog.Items) != 1 || catalog.Items[0].Action != "cmd.run" {
		t.Fatalf("catalog command filter mismatch: %+v", catalog.Items)
	}
	item := catalog.Items[0]
	if len(item.ArgsSchema) == 0 || !json.Valid(item.ArgsSchema) {
		t.Fatalf("cmd.run should retain valid args_schema: %s", string(item.ArgsSchema))
	}
	if len(item.InputsSchema) == 0 || !json.Valid(item.InputsSchema) {
		t.Fatalf("cmd.run missing valid inputs_schema: %s", string(item.InputsSchema))
	}
	if len(item.OutputsSchema) == 0 || !json.Valid(item.OutputsSchema) {
		t.Fatalf("cmd.run missing valid outputs_schema: %s", string(item.OutputsSchema))
	}
	if len(item.InputExamples) == 0 || len(item.OutputExamples) == 0 {
		t.Fatalf("cmd.run missing structured IO examples: %+v", item)
	}
	var rawCatalog map[string]any
	if err := json.Unmarshal(payload, &rawCatalog); err != nil {
		t.Fatalf("decode raw catalog response: %v", err)
	}
	rawItems, ok := rawCatalog["items"].([]any)
	if !ok || len(rawItems) != 1 {
		t.Fatalf("raw catalog items mismatch: %+v", rawCatalog["items"])
	}
	rawItem, ok := rawItems[0].(map[string]any)
	if !ok {
		t.Fatalf("raw catalog item mismatch: %+v", rawItems[0])
	}
	if _, ok := rawItem["input_schema"]; !ok {
		t.Fatalf("cmd.run should expose input_schema alias for Studio clients: %+v", rawItem)
	}
	if _, ok := rawItem["output_schema"]; !ok {
		t.Fatalf("cmd.run should expose output_schema alias for Studio clients: %+v", rawItem)
	}
}

func TestVisualWorkflowAIDraftRouteGeneratesHostResourceWorkflow(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/ai/draft", map[string]any{
		"workflow_name":   "host-resource-check",
		"workflow_status": "draft",
		"instruction":     "创建一个检查主机 CPU、内存、磁盘和负载资源的工作流",
		"graph": visual.Graph{
			Version: visual.GraphVersion,
			Workflow: workflow.Workflow{
				Version: "v0.1",
				Name:    "host-resource-check",
			},
		},
	})
	if code != http.StatusOK {
		t.Fatalf("ai draft status = %d payload=%s", code, payload)
	}
	var response struct {
		GraphPatch struct {
			Operations []map[string]any `json:"operations"`
		} `json:"graph_patch"`
		CandidateGraph visual.Graph   `json:"candidate_graph"`
		DiffSummary    map[string]any `json:"diff_summary"`
	}
	if err := json.Unmarshal(payload, &response); err != nil {
		t.Fatalf("decode ai draft response: %v", err)
	}
	if len(response.GraphPatch.Operations) == 0 {
		t.Fatalf("graph patch should describe generated operations: %+v", response)
	}
	if response.CandidateGraph.Workflow.Name != "host-resource-check" {
		t.Fatalf("workflow name = %q", response.CandidateGraph.Workflow.Name)
	}
	if len(response.CandidateGraph.Nodes) < 3 || len(response.CandidateGraph.Edges) < 2 {
		t.Fatalf("candidate graph too small: nodes=%d edges=%d", len(response.CandidateGraph.Nodes), len(response.CandidateGraph.Edges))
	}
	if !graphHasAction(response.CandidateGraph, "shell.run") {
		t.Fatalf("candidate graph should include shell.run resource check node: %+v", response.CandidateGraph.Nodes)
	}
	if err := visual.ValidateGraph(response.CandidateGraph); err != nil {
		t.Fatalf("candidate graph should be valid for save/apply: %v", err)
	}
	if response.DiffSummary["semantic_changes"] == nil {
		t.Fatalf("diff summary should include semantic changes: %+v", response.DiffSummary)
	}
}

func TestVisualWorkflowAIDraftRouteRequiresDraftStatus(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/ai/draft", map[string]any{
		"workflow_name":   "host-resource-check",
		"workflow_status": "published",
		"instruction":     "修改已发布流程",
		"graph": visual.Graph{
			Version: visual.GraphVersion,
			Workflow: workflow.Workflow{
				Version: "v0.1",
				Name:    "host-resource-check",
			},
		},
	})
	if code != http.StatusConflict {
		t.Fatalf("ai draft published status = %d, want 409 payload=%s", code, payload)
	}
	if !bytes.Contains(payload, []byte("draft")) {
		t.Fatalf("payload = %s, want draft guard explanation", payload)
	}
}

func TestVisualWorkflowRoutesRequireAuthWhenEnabled(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{
		AuthEnabled:    true,
		AuthToken:      "secret-token",
		VisualWorkflow: NewVisualWorkflowHandler(svc),
	})
	body := map[string]any{"graph": sampleAPIGraph()}

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/compile", body)
	if code != http.StatusUnauthorized {
		t.Fatalf("unauthorized compile status = %d, want 401 payload=%s", code, payload)
	}

	code, payload = serveJSONWithHeaders(t, router, http.MethodPost, "/api/v1/workflows/graph/compile", body, map[string]string{
		"Authorization": "Bearer secret-token",
	})
	if code != http.StatusOK {
		t.Fatalf("authorized compile status = %d payload=%s", code, payload)
	}
	var compiled service.CompiledVisualWorkflow
	if err := json.Unmarshal(payload, &compiled); err != nil {
		t.Fatalf("decode authorized compile response: %v", err)
	}
	if compiled.Workflow.Name != "api-graph" {
		t.Fatalf("compiled workflow mismatch: %+v", compiled.Workflow)
	}
}

func TestVisualWorkflowRouteValidateReturnsFieldErrors(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})
	graph := sampleAPIGraph()
	graph.Nodes[1].Step.Args = nil

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/validate", map[string]any{"graph": graph})
	if code != http.StatusOK {
		t.Fatalf("validate status = %d payload=%s", code, payload)
	}
	var result service.VisualWorkflowValidationResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode validate response: %v", err)
	}
	if result.Valid || len(result.Errors) == 0 {
		t.Fatalf("expected validation errors: %+v", result)
	}
}

func TestVisualWorkflowVariableResolveRoute(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})
	graph := sampleAPIGraph()
	graph.Nodes[0].Outputs = []visual.OutputParamSpec{{Key: "backup_id", Type: "string"}}
	graph.Nodes[1].Outputs = []visual.OutputParamSpec{{Key: "stdout", Type: "string"}}

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/variables/resolve", map[string]any{
		"graph":   graph,
		"node_id": "run",
	})
	if code != http.StatusOK {
		t.Fatalf("variable resolve status = %d payload=%s", code, payload)
	}
	var resolved visual.VariableResolveResult
	if err := json.Unmarshal(payload, &resolved); err != nil {
		t.Fatalf("decode variable resolve response: %v", err)
	}
	if resolved.NodeID != "run" {
		t.Fatalf("resolved node id = %q, want run", resolved.NodeID)
	}
	if !hasAPIVariable(resolved.Scopes, "workflow_input", "backup_id") {
		t.Fatalf("run node should see workflow input backup_id: %+v", resolved.Scopes)
	}
	if hasAPIVariable(resolved.Scopes, "node_output", "stdout") {
		t.Fatalf("run node should not see its own stdout output: %+v", resolved.Scopes)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/variables/resolve", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("empty variable resolve status = %d, want 400 payload=%s", code, payload)
	}
}

func TestVisualWorkflowRouteParsesYAMLToGraph(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})
	compiled, err := svc.Compile(t.Context(), sampleAPIGraph())
	if err != nil {
		t.Fatalf("compile sample graph: %v", err)
	}

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/parse", map[string]any{"yaml": compiled.YAML})
	if code != http.StatusOK {
		t.Fatalf("parse status = %d payload=%s", code, payload)
	}
	var graph visual.Graph
	if err := json.Unmarshal(payload, &graph); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if graph.Workflow.Name != "api-graph" || len(graph.Nodes) == 0 || len(graph.Edges) == 0 {
		t.Fatalf("parsed graph mismatch: %+v", graph)
	}
}

func TestVisualWorkflowRouteParseYAMLReturnsFailureType(t *testing.T) {
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/parse", map[string]any{"yaml": "version: ["})
	if code != http.StatusBadRequest {
		t.Fatalf("parse status = %d payload=%s", code, payload)
	}
	var got struct {
		Error string `json:"error"`
		Type  string `json:"type"`
	}
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("decode parse error: %v", err)
	}
	if got.Type != string(visual.ParseErrorKindYAMLSyntax) || got.Error == "" {
		t.Fatalf("parse error payload mismatch: %+v", got)
	}
}

func TestVisualWorkflowRouteUpdateReturnsConflictForStaleGraph(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	graph := sampleAPIGraph()
	compiled, err := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{}).Compile(t.Context(), graph)
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	if err := workflowSvc.Create(t.Context(), &service.WorkflowRecord{
		Name:    graph.Workflow.Name,
		RawYAML: []byte(compiled.YAML),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-graph/graph", nil)
	if code != http.StatusOK {
		t.Fatalf("get first graph status = %d payload=%s", code, payload)
	}
	var first visual.Graph
	if err := json.Unmarshal(payload, &first); err != nil {
		t.Fatalf("decode first graph: %v", err)
	}
	code, payload = serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-graph/graph", nil)
	if code != http.StatusOK {
		t.Fatalf("get second graph status = %d payload=%s", code, payload)
	}
	var second visual.Graph
	if err := json.Unmarshal(payload, &second); err != nil {
		t.Fatalf("decode second graph: %v", err)
	}

	first.Workflow.Description = "first writer"
	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-graph/graph", map[string]any{"graph": first})
	if code != http.StatusOK {
		t.Fatalf("first update status = %d payload=%s", code, payload)
	}

	second.Workflow.Description = "stale writer"
	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-graph/graph", map[string]any{"graph": second})
	if code != http.StatusConflict {
		t.Fatalf("stale update status = %d, want 409 payload=%s", code, payload)
	}
}

func TestVisualWorkflowRouteUpdateGraphPersistsSaveNote(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	graph := sampleAPIGraph()
	compiled, err := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{}).Compile(t.Context(), graph)
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	if err := workflowSvc.Create(t.Context(), &service.WorkflowRecord{
		Name:    graph.Workflow.Name,
		RawYAML: []byte(compiled.YAML),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}

	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodGet, "/api/v1/workflows/api-graph/graph", nil)
	if code != http.StatusOK {
		t.Fatalf("get graph status = %d payload=%s", code, payload)
	}
	var loaded visual.Graph
	if err := json.Unmarshal(payload, &loaded); err != nil {
		t.Fatalf("decode graph: %v", err)
	}

	loaded.Workflow.Description = "updated through graph API"
	code, payload = serveJSON(t, router, http.MethodPut, "/api/v1/workflows/api-graph/graph", map[string]any{
		"graph":     loaded,
		"save_note": "changed action target after review",
	})
	if code != http.StatusOK {
		t.Fatalf("update graph status = %d payload=%s", code, payload)
	}
	var saved struct {
		Graph visual.Graph `json:"graph"`
	}
	if err := json.Unmarshal(payload, &saved); err != nil {
		t.Fatalf("decode saved graph response: %v", err)
	}
	if saved.Graph.Workflow.Name != "api-graph" {
		t.Fatalf("saved graph response workflow name = %q", saved.Graph.Workflow.Name)
	}
	if got := saved.Graph.UI["resource_version"]; got == "" || got == loaded.UI["resource_version"] {
		t.Fatalf("saved graph resource_version = %v, want new non-empty version different from %v", got, loaded.UI["resource_version"])
	}

	record, err := workflowSvc.Get(t.Context(), "api-graph")
	if err != nil {
		t.Fatalf("get workflow: %v", err)
	}
	if got := record.SaveNote; got != "changed action target after review" {
		t.Fatalf("save note = %q, want %q", got, "changed action target after review")
	}
}

func TestVisualWorkflowRouteCreateGraphWorkflow(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})
	graph := sampleAPIGraph()
	graph.Workflow.Name = "api-created-graph"
	graph.Workflow.Description = "created from visual UI"

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph", map[string]any{
		"graph":     graph,
		"labels":    map[string]string{"source": "visual-ui"},
		"save_note": "initial visual workflow draft",
	})
	if code != http.StatusCreated {
		t.Fatalf("create graph status = %d payload=%s", code, payload)
	}
	var created service.CreatedVisualWorkflow
	if err := json.Unmarshal(payload, &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	if created.Name != "api-created-graph" || created.Status != service.WorkflowStatusDraft {
		t.Fatalf("created response metadata mismatch: %+v", created)
	}
	if created.Graph.Workflow.Name != "api-created-graph" || created.YAML == "" {
		t.Fatalf("created response graph/yaml mismatch: %+v", created)
	}

	record, err := workflowSvc.Get(t.Context(), "api-created-graph")
	if err != nil {
		t.Fatalf("get created workflow: %v", err)
	}
	if record.Labels["source"] != "visual-ui" || record.SaveNote != "initial visual workflow draft" {
		t.Fatalf("created workflow metadata mismatch: labels=%+v save_note=%q", record.Labels, record.SaveNote)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph", map[string]any{"graph": graph})
	if code != http.StatusConflict {
		t.Fatalf("duplicate create graph status = %d, want 409 payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph", nil)
	if code != http.StatusBadRequest {
		t.Fatalf("empty create graph status = %d, want 400 payload=%s", code, payload)
	}

	invalid := sampleAPIGraph()
	invalid.Workflow.Name = "invalid-created-graph"
	invalid.Nodes[1].Step.Args = nil
	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph", map[string]any{"graph": invalid})
	if code != http.StatusBadRequest {
		t.Fatalf("invalid create graph status = %d, want 400 payload=%s", code, payload)
	}
}

func TestVisualWorkflowRouteDryRunMarksNamedWorkflowDryRunPassed(t *testing.T) {
	workflowSvc := service.NewWorkflowService(t.TempDir())
	graph := sampleAPIGraph()
	compiled, err := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{}).Compile(t.Context(), graph)
	if err != nil {
		t.Fatalf("compile initial graph: %v", err)
	}
	if err := workflowSvc.Create(t.Context(), &service.WorkflowRecord{
		Name:    graph.Workflow.Name,
		RawYAML: []byte(compiled.YAML),
	}); err != nil {
		t.Fatalf("create workflow: %v", err)
	}
	validated, err := workflowSvc.ValidateWorkflow(t.Context(), graph.Workflow.Name, service.WorkflowValidateOptions{Actor: "sre"})
	if err != nil {
		t.Fatalf("validate workflow: %v", err)
	}

	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{WorkflowService: workflowSvc})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/dry-run", map[string]any{
		"workflow_name": graph.Workflow.Name,
		"graph":         graph,
		"triggered_by":  "sre",
	})
	if code != http.StatusOK {
		t.Fatalf("dry-run graph status = %d payload=%s", code, payload)
	}
	var result service.VisualWorkflowDryRunResult
	if err := json.Unmarshal(payload, &result); err != nil {
		t.Fatalf("decode dry-run result: %v", err)
	}
	if !result.Valid || result.Status != service.WorkflowStatusDryRunPassed || result.DryRunGraphHash != validated.ValidatedGraphHash {
		t.Fatalf("dry-run result should include dry-run gate metadata: %+v validated=%+v", result, validated)
	}
	record, err := workflowSvc.Get(t.Context(), graph.Workflow.Name)
	if err != nil {
		t.Fatalf("get workflow after dry-run: %v", err)
	}
	if record.Status != service.WorkflowStatusDryRunPassed || record.DryRunGraphHash != validated.ValidatedGraphHash || record.DryRunAt.IsZero() {
		t.Fatalf("workflow dry-run gate metadata missing: %+v", record)
	}
}

func TestVisualWorkflowRouteSubmitRequiresRiskAcknowledgement(t *testing.T) {
	runSvc := service.NewRunService(service.RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{RunService: runSvc})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})
	graph := sampleAPIGraph()
	graph.Nodes[1].Step.Action = "shell.run"
	graph.Nodes[1].Step.Args = map[string]any{"script": "echo acknowledged"}

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/runs", map[string]any{"graph": graph})
	if code != http.StatusBadRequest {
		t.Fatalf("high-risk submit status = %d, want 400 payload=%s", code, payload)
	}

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/runs", map[string]any{
		"graph":             graph,
		"risk_acknowledged": true,
	})
	if code != http.StatusAccepted {
		t.Fatalf("acknowledged high-risk submit status = %d payload=%s", code, payload)
	}
}

func TestVisualWorkflowRoutesResolveManualApprovalNodes(t *testing.T) {
	runSvc := service.NewRunService(service.RunServiceConfig{MaxConcurrentRuns: 1}, nil, nil, state.NewInMemoryRunStore(), queue.NewMemoryQueue(4), events.NewHub(), metrics.NewCollector())
	defer runSvc.Close()
	svc := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{RunService: runSvc})
	router := NewRouter(RouterOptions{VisualWorkflow: NewVisualWorkflowHandler(svc)})

	code, payload := serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/runs", map[string]any{"graph": manualApprovalAPIGraph()})
	if code != http.StatusAccepted {
		t.Fatalf("submit approval graph status = %d payload=%s", code, payload)
	}
	var submitted service.RunResponse
	if err := json.Unmarshal(payload, &submitted); err != nil {
		t.Fatalf("decode submit response: %v", err)
	}
	waitAPIRunNodeStatus(t, runSvc, submitted.RunID, "approve", "waiting", 3*time.Second)

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/runs/"+submitted.RunID+"/nodes/approve/approve", map[string]any{
		"actor":   "sre",
		"comment": "approved",
	})
	if code != http.StatusOK {
		t.Fatalf("approve route status = %d payload=%s", code, payload)
	}
	var approved struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &approved); err != nil {
		t.Fatalf("decode approve response: %v", err)
	}
	if approved.Status != "approved" {
		t.Fatalf("approve status = %q", approved.Status)
	}
	waitAPIRunTerminal(t, runSvc, submitted.RunID, 3*time.Second)

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/workflows/graph/runs", map[string]any{"graph": manualApprovalAPIGraph()})
	if code != http.StatusAccepted {
		t.Fatalf("submit approval graph for rejection status = %d payload=%s", code, payload)
	}
	if err := json.Unmarshal(payload, &submitted); err != nil {
		t.Fatalf("decode second submit response: %v", err)
	}
	waitAPIRunNodeStatus(t, runSvc, submitted.RunID, "approve", "waiting", 3*time.Second)

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/runs/"+submitted.RunID+"/nodes/approve/reject", map[string]any{
		"actor":   "sre",
		"comment": "blocked",
	})
	if code != http.StatusOK {
		t.Fatalf("reject route status = %d payload=%s", code, payload)
	}
	var rejected struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(payload, &rejected); err != nil {
		t.Fatalf("decode reject response: %v", err)
	}
	if rejected.Status != "rejected" {
		t.Fatalf("reject status = %q", rejected.Status)
	}
	waitAPIRunTerminal(t, runSvc, submitted.RunID, 3*time.Second)

	code, payload = serveJSON(t, router, http.MethodPost, "/api/v1/runs/"+submitted.RunID+"/nodes/approve/reject", map[string]any{"comment": "duplicate"})
	if code != http.StatusNotFound {
		t.Fatalf("resolved approval route status = %d, want 404 payload=%s", code, payload)
	}
}

func sampleAPIGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "api-graph",
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{ID: "run", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:   "run",
				Action: "cmd.run",
				Args:   map[string]any{"cmd": "echo ok"},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-run", Source: "start", Target: "run", Kind: visual.EdgeKindNext},
			{ID: "run-end", Source: "run", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func manualApprovalAPIGraph() visual.Graph {
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version: "v0.1",
			Name:    "api-approval",
			Inventory: workflow.Inventory{
				Hosts: map[string]workflow.Host{"local": {Address: "local"}},
			},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart},
			{
				ID:   "approve",
				Type: visual.NodeTypeManualApproval,
				Step: &workflow.Step{
					Name:   "approve",
					Action: "manual.approval",
				},
				Approval: &visual.ApprovalSpec{
					Subjects:  []string{"sre"},
					Timeout:   "30m",
					OnTimeout: "reject",
				},
			},
			{ID: "after", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:    "after",
				Targets: []string{"local"},
				Action:  "cmd.run",
				Args:    map[string]any{"cmd": "echo after"},
			}},
			{ID: "notify", Type: visual.NodeTypeAction, Step: &workflow.Step{
				Name:    "notify",
				Targets: []string{"local"},
				Action:  "cmd.run",
				Args:    map[string]any{"cmd": "echo notify"},
			}},
			{ID: "end", Type: visual.NodeTypeEnd},
		},
		Edges: []visual.Edge{
			{ID: "start-approve", Source: "start", Target: "approve", Kind: visual.EdgeKindNext},
			{ID: "approve-after", Source: "approve", Target: "after", Kind: visual.EdgeKindApprovalApproved},
			{ID: "approve-notify", Source: "approve", Target: "notify", Kind: visual.EdgeKindApprovalRejected},
			{ID: "after-end", Source: "after", Target: "end", Kind: visual.EdgeKindSuccess},
			{ID: "notify-end", Source: "notify", Target: "end", Kind: visual.EdgeKindSuccess},
		},
	}
}

func waitAPIRunNodeStatus(t *testing.T, svc *service.RunService, runID, nodeID, status string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		detail, err := svc.Get(context.Background(), runID)
		if err == nil && detail.Graph != nil {
			if node, ok := detail.Graph.Nodes[nodeID]; ok && node.Status == status {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	detail, err := svc.Get(context.Background(), runID)
	if err != nil {
		t.Fatalf("run %s did not reach node %s status %s: %v", runID, nodeID, status, err)
	}
	if detail.Graph == nil {
		t.Fatalf("run %s did not reach node %s status %s: graph is nil", runID, nodeID, status)
	}
	t.Fatalf("run %s node %s status = %q, want %q", runID, nodeID, detail.Graph.Nodes[nodeID].Status, status)
}

func waitAPIRunTerminal(t *testing.T, svc *service.RunService, runID string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		detail, err := svc.Get(context.Background(), runID)
		if err == nil && state.IsTerminalRunStatus(detail.Status) {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("run %s did not finish within %s", runID, timeout)
}

func serveJSON(t *testing.T, router http.Handler, method, path string, body any) (int, []byte) {
	return serveJSONWithHeaders(t, router, method, path, body, nil)
}

func serveJSONWithHeaders(t *testing.T, router http.Handler, method, path string, body any, headers map[string]string) (int, []byte) {
	t.Helper()
	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		raw, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal request: %v", err)
		}
		reader = bytes.NewReader(raw)
	}
	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for key, value := range headers {
		req.Header.Set(key, value)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

func hasAPIVariable(scopes []visual.VariableScope, scope, name string) bool {
	for _, item := range scopes {
		if item.Scope != scope {
			continue
		}
		for _, variable := range item.Variables {
			if variable.Name == name {
				return true
			}
		}
	}
	return false
}

func graphHasAction(graph visual.Graph, action string) bool {
	for _, node := range graph.Nodes {
		if node.Step != nil && node.Step.Action == action {
			return true
		}
	}
	return false
}
