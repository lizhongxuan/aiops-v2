package workfloweditor

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
	runnerapp "runner/server/app"
	runnerconfig "runner/server/config"
	"runner/server/service"
	"runner/testsupport/workflowfixtures"
	"runner/workflow"
	contract "runner/workflow/edit"
	"runner/workflow/visual"
)

const workflowParityTenant = "tenant-a"

func TestWorkflowGetContextRunnerAuthoritativeParity(t *testing.T) {
	t.Run("MCP-004", func(t *testing.T) {
		runtime := newWorkflowParityRuntime(t, workflowParityTenant)
		catalog := loadWorkflowParityCatalog(t)

		for _, fixtureID := range []string{"W0", "W1", "W2", "W3", "W4", "W5", "W6"} {
			fixtureID := fixtureID
			t.Run(fixtureID, func(t *testing.T) {
				fixture, err := catalog.Load(fixtureID)
				if err != nil {
					t.Fatal(err)
				}
				graph := materializeWorkflowParityGraph(t, fixture)
				createWorkflowParityGraph(t, runtime.Handler, graph)

				httpGraph := readWorkflowParityHTTPGraph(t, runtime.Handler, fixtureID)
				key := contract.WorkflowKey{TenantID: workflowParityTenant, WorkflowID: fixtureID}
				authoritative, err := runtime.WorkflowEditFacade().GetGraph(context.Background(), key)
				if err != nil {
					t.Fatalf("read authoritative Runner snapshot: %v", err)
				}
				httpJSON, err := json.Marshal(httpGraph)
				if err != nil {
					t.Fatalf("marshal Runner HTTP graph: %v", err)
				}
				facadeJSON, err := json.Marshal(authoritative.Graph)
				if err != nil {
					t.Fatalf("marshal authoritative facade graph: %v", err)
				}
				if !bytes.Equal(httpJSON, facadeJSON) {
					t.Fatalf("Runner HTTP graph differs from authoritative facade snapshot:\nhttp=%s\nfacade=%s", httpJSON, facadeJSON)
				}

				hashes, err := service.VisualWorkflowGraphHashes(httpGraph)
				if err != nil {
					t.Fatalf("hash authoritative graph: %v", err)
				}
				if authoritative.Revision.SemanticHash != hashes.SemanticHash || authoritative.Revision.LayoutHash != hashes.LayoutHash || authoritative.Revision.ResourceVersion == "" {
					t.Fatalf("authoritative revision/hash mismatch: revision=%+v hashes=%+v", authoritative.Revision, hashes)
				}

				resources, err := NewWorkflowResourcesV2WithKey(runtime.WorkflowEditApplication(), workflowResourceTestKey())
				if err != nil {
					t.Fatal(err)
				}
				tool := workflowEditorToolByName(t, WorkflowReadToolsV2(runtime.WorkflowEditApplication(), resources), "workflow.get_context")
				ctx := workflowTrustedContext(workflowParityTenant, "user-a", fixtureID)
				mcpGraph, mcpRevision := readCompleteWorkflowContext(t, ctx, tool, resources, authoritative.Revision.ResourceVersion)

				if mcpRevision != authoritative.Revision {
					t.Fatalf("MCP revision differs from Runner authoritative revision: mcp=%+v runner=%+v", mcpRevision, authoritative.Revision)
				}
				mcpJSON, err := json.Marshal(struct {
					Nodes []visual.Node `json:"nodes"`
					Edges []visual.Edge `json:"edges"`
				}{Nodes: mcpGraph.Nodes, Edges: mcpGraph.Edges})
				if err != nil {
					t.Fatalf("marshal MCP graph: %v", err)
				}
				authoritativeJSON, err := json.Marshal(struct {
					Nodes []visual.Node `json:"nodes"`
					Edges []visual.Edge `json:"edges"`
				}{Nodes: authoritative.Graph.Nodes, Edges: authoritative.Graph.Edges})
				if err != nil {
					t.Fatalf("marshal authoritative graph page: %v", err)
				}
				if !bytes.Equal(mcpJSON, authoritativeJSON) {
					t.Fatalf("MCP graph page differs from Runner authoritative graph:\nmcp=%s\nrunner=%s", mcpJSON, authoritativeJSON)
				}
			})
		}
	})
}

func TestWorkflowResourcesV2CrossTenantDoesNotDiscloseAuthoritativeWorkflow(t *testing.T) {
	t.Run("SEC-003", func(t *testing.T) {
		runtime := newWorkflowParityRuntime(t, workflowParityTenant)
		fixture, err := loadWorkflowParityCatalog(t).Load("W2")
		if err != nil {
			t.Fatal(err)
		}
		createWorkflowParityGraph(t, runtime.Handler, materializeWorkflowParityGraph(t, fixture))

		resources, err := NewWorkflowResourcesV2WithKey(runtime.WorkflowEditApplication(), workflowResourceTestKey())
		if err != nil {
			t.Fatal(err)
		}
		tool := workflowEditorToolByName(t, WorkflowReadToolsV2(runtime.WorkflowEditApplication(), resources), "workflow.get_context")
		tenantA := workflowResourceContext(workflowParityTenant, "user-a", "session-a", "W2")
		result, err := tool.Execute(tenantA, contextInput("full", "", nil, 0, 200))
		if err != nil {
			t.Fatal(err)
		}
		if len(result.References) != 1 {
			t.Fatalf("resource reference is absent or discloses scope: %#v", result.References)
		}
		uri := result.References[0].URI
		token := strings.TrimPrefix(uri, workflowResourcePrefixV2)
		if !strings.HasPrefix(uri, workflowResourcePrefixV2) || token == "" || strings.Contains(token, "/") || token == "W2" || strings.Contains(uri, workflowParityTenant) {
			t.Fatalf("resource reference is not a single opaque scope token: %#v", result.References)
		}

		denied := []context.Context{
			workflowResourceContext("tenant-b", "user-a", "session-a", "W2"),
			workflowResourceContext(workflowParityTenant, "user-b", "session-a", "W2"),
			workflowResourceContext(workflowParityTenant, "user-a", "session-b", "W2"),
			workflowResourceContext(workflowParityTenant, "user-a", "session-a", "unknown-workflow"),
		}
		for _, deniedContext := range denied {
			if _, readErr := resources.Read(deniedContext, uri); !errors.Is(readErr, mcp.ErrResourceUnavailable) || readErr.Error() != mcp.ErrResourceUnavailable.Error() {
				t.Fatalf("cross-scope resource read error = %v", readErr)
			}
		}

		provider, err := NewWorkflowResourceProviderV2(resources)
		if err != nil {
			t.Fatal(err)
		}
		registration, err := WorkflowResourceProviderRegistrationV2(provider)
		if err != nil {
			t.Fatal(err)
		}
		registry := mcp.NewRegistry()
		if err := registry.RegisterResourceProvider(registration); err != nil {
			t.Fatal(err)
		}
		page, err := registry.ListResourcesScoped(tenantA, mcp.ResourceListRequest{ServerID: ServerID, Limit: 2})
		if err != nil || len(page.Resources) != 2 {
			t.Fatalf("authorized resource list = %#v, %v", page, err)
		}
		for _, deniedContext := range []context.Context{denied[0], denied[3]} {
			if _, listErr := registry.ListResourcesScoped(deniedContext, mcp.ResourceListRequest{ServerID: ServerID, Limit: 2}); !errors.Is(listErr, mcp.ErrResourceUnavailable) || listErr.Error() != mcp.ErrResourceUnavailable.Error() {
				t.Fatalf("cross-scope resource list error = %v", listErr)
			}
		}
		for _, deniedContext := range denied {
			if _, ok, readErr := registry.ReadResource(deniedContext, ServerID, page.Resources[0].URI); ok || !errors.Is(readErr, mcp.ErrResourceUnavailable) || readErr.Error() != mcp.ErrResourceUnavailable.Error() {
				t.Fatalf("cross-scope registry read = ok:%v err:%v", ok, readErr)
			}
		}

		crossTenantError := executeWorkflowContextError(tool, workflowTrustedContext("tenant-b", "user-a", "W2"))
		unknownTenantError := executeWorkflowContextError(tool, workflowTrustedContext("tenant-b", "user-a", "unknown-workflow"))
		if crossTenantError == "" || crossTenantError != unknownTenantError || strings.Contains(crossTenantError, workflowParityTenant) || strings.Contains(crossTenantError, "W2") {
			t.Fatalf("cross-tenant tool error leaks existence: existing=%q unknown=%q", crossTenantError, unknownTenantError)
		}
	})
}

func readCompleteWorkflowContext(t *testing.T, ctx context.Context, tool tooling.Tool, resources *WorkflowResourcesV2, revision string) (service.WorkflowContextGraphPage, contract.Revision) {
	t.Helper()
	var complete service.WorkflowContextGraphPage
	var expectedRevision contract.Revision
	cursor := ""
	for pageNumber := 0; ; pageNumber++ {
		if pageNumber > 32 {
			t.Fatal("workflow context pagination did not terminate")
		}
		input, err := json.Marshal(workflowGetContextInputV2{SchemaVersion: contract.SchemaVersion, Revision: revision, View: "full", Cursor: cursor, Limit: 200})
		if err != nil {
			t.Fatal(err)
		}
		result, err := tool.Execute(ctx, input)
		if err != nil {
			t.Fatalf("execute workflow.get_context page %d: %v", pageNumber, err)
		}
		if err := tooling.ValidateToolResult(tool.OutputSchema(), result); err != nil {
			t.Fatalf("validate workflow.get_context page %d: %v", pageNumber, err)
		}
		var envelope contract.ToolResult[contract.WorkflowContext]
		if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
			t.Fatal(err)
		}
		value := envelope.StructuredContent
		if len(value.Graph) == 0 {
			if len(result.References) != 1 || result.References[0].URI != value.ResourceURI {
				t.Fatalf("externalized context reference = %#v, uri=%q", result.References, value.ResourceURI)
			}
			content, err := resources.Read(ctx, result.References[0].URI)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(content.Content, &value); err != nil {
				t.Fatal(err)
			}
		}
		if pageNumber == 0 {
			expectedRevision = value.Revision
		} else if value.Revision != expectedRevision {
			t.Fatalf("revision changed during pagination: first=%+v current=%+v", expectedRevision, value.Revision)
		}
		var page service.WorkflowContextGraphPage
		if err := json.Unmarshal(value.Graph, &page); err != nil {
			t.Fatalf("decode workflow context page %d: %v", pageNumber, err)
		}
		complete.Nodes = append(complete.Nodes, page.Nodes...)
		complete.Edges = append(complete.Edges, page.Edges...)
		cursor = value.NextCursor
		if cursor == "" {
			break
		}
	}
	return complete, expectedRevision
}

func executeWorkflowContextError(tool tooling.Tool, ctx context.Context) string {
	_, err := tool.Execute(ctx, contextInput("summary", "", nil, 0, 20))
	if err == nil {
		return ""
	}
	return err.Error()
}

type workflowParityFixtureData struct {
	Workflow struct {
		ID            string   `json:"id"`
		Name          string   `json:"name"`
		NodeCount     int      `json:"nodeCount"`
		EdgeCount     int      `json:"edgeCount"`
		HiddenNodeIDs []string `json:"hiddenNodeIds"`
		LockedNodeIDs []string `json:"lockedNodeIds"`
		PinnedNodeIDs []string `json:"pinnedNodeIds"`
		Nodes         []struct {
			ID        string         `json:"id"`
			Type      string         `json:"type"`
			ActionRef string         `json:"actionRef"`
			Args      map[string]any `json:"args"`
		} `json:"nodes"`
		Edges []struct {
			ID, Source, SourcePort, Target string
		} `json:"edges"`
	} `json:"workflow"`
	Recipe struct {
		Kind      string `json:"kind"`
		Seed      int    `json:"seed"`
		NodeCount int    `json:"nodeCount"`
		EdgeCount int    `json:"edgeCount"`
	} `json:"recipe"`
}

func materializeWorkflowParityGraph(t *testing.T, fixture workflowfixtures.Fixture) visual.Graph {
	t.Helper()
	var data workflowParityFixtureData
	if err := fixture.DecodeData(&data); err != nil {
		t.Fatal(err)
	}
	if data.Workflow.ID != fixture.FixtureID {
		t.Fatalf("fixture workflow id = %q, want %q", data.Workflow.ID, fixture.FixtureID)
	}

	nodeCount, edgeCount := data.Workflow.NodeCount, data.Workflow.EdgeCount
	if data.Recipe.Kind != "" {
		if data.Recipe.Kind != "seeded-dag" || data.Recipe.Seed != 17 {
			t.Fatalf("unsupported deterministic workflow recipe: %+v", data.Recipe)
		}
		nodeCount, edgeCount = data.Recipe.NodeCount, data.Recipe.EdgeCount
	}
	if nodeCount > 0 {
		return deterministicWorkflowParityDAG(fixture.FixtureID, nodeCount, edgeCount)
	}

	if len(data.Workflow.Nodes) == 0 {
		if fixture.FixtureID == "W6" {
			graph := deterministicWorkflowParityDAG(fixture.FixtureID, 4, 3)
			for index := range graph.Nodes {
				switch graph.Nodes[index].ID {
				case "action-001":
					graph.Nodes[index].ID = "reference"
					graph.Nodes[index].StepID, graph.Nodes[index].StepName = "reference", "reference"
					graph.Nodes[index].Step.ID, graph.Nodes[index].Step.Name = "reference", "reference"
					graph.Nodes[index].UI = map[string]any{"locked": true, "pinned": true, "verifiedReference": true}
				case "end":
					graph.Nodes[index].ID = "end-hidden"
					graph.Nodes[index].UI = map[string]any{"hidden": true}
				}
			}
			for index := range graph.Edges {
				if graph.Edges[index].Source == "action-001" {
					graph.Edges[index].Source = "reference"
				}
				if graph.Edges[index].Target == "action-001" {
					graph.Edges[index].Target = "reference"
				}
				if graph.Edges[index].Target == "end" {
					graph.Edges[index].Target = "end-hidden"
				}
			}
			return graph
		}
		// A persisted Runner "blank" draft uses a single explicit placeholder
		// action between Start and End; it still has no user-authored action.
		return deterministicWorkflowParityDAG(fixture.FixtureID, 3, 2)
	}

	nodes := make([]visual.Node, 0, len(data.Workflow.Nodes)+2)
	hasStart, hasEnd := false, false
	for index, item := range data.Workflow.Nodes {
		nodeType := visual.NodeType(item.Type)
		position := visual.Position{X: float64(index * 180), Y: float64((index % 3) * 100)}
		node := visual.Node{ID: item.ID, Type: nodeType, Position: position, Label: item.ID}
		switch nodeType {
		case visual.NodeTypeStart:
			hasStart = true
		case visual.NodeTypeEnd:
			hasEnd = true
		default:
			step := workflow.Step{ID: item.ID, Name: item.ID, Action: "script.shell", Args: map[string]any{"script": "true"}}
			node.StepID, node.StepName, node.Step = item.ID, item.ID, &step
			node.UI = map[string]any{"fixtureActionRef": item.ActionRef}
			if nodeType == visual.NodeTypeCondition {
				node.Condition = &visual.ConditionSpec{If: "true", Else: true}
			}
		}
		nodes = append(nodes, node)
	}
	if !hasStart {
		nodes = append([]visual.Node{{ID: "start", Type: visual.NodeTypeStart}}, nodes...)
	}
	if !hasEnd {
		nodes = append(nodes, visual.Node{ID: "end", Type: visual.NodeTypeEnd, Position: visual.Position{X: float64(len(nodes) * 180)}})
	}

	edges := make([]visual.Edge, 0, len(data.Workflow.Edges)+2)
	for _, item := range data.Workflow.Edges {
		kind := visual.EdgeKindNext
		switch item.SourcePort {
		case "true":
			kind = visual.EdgeKindIf
		case "false":
			kind = visual.EdgeKindElse
		}
		edges = append(edges, visual.Edge{ID: item.ID, Source: item.Source, Target: item.Target, Kind: kind})
	}
	if !hasStart {
		edges = append([]visual.Edge{{ID: "fixture-start", Source: "start", Target: data.Workflow.Nodes[0].ID, Kind: visual.EdgeKindNext}}, edges...)
	}
	if !hasEnd {
		edges = append(edges, visual.Edge{ID: "fixture-end", Source: data.Workflow.Nodes[len(data.Workflow.Nodes)-1].ID, Target: "end", Kind: visual.EdgeKindNext})
	}
	return visual.Graph{Version: visual.GraphVersion, Workflow: workflow.Workflow{Name: fixture.FixtureID, Version: "v0.1", Plan: workflow.Plan{Mode: "auto", Strategy: "graph"}}, Nodes: nodes, Edges: edges}
}

func deterministicWorkflowParityDAG(name string, nodeCount, edgeCount int) visual.Graph {
	if nodeCount < 3 {
		nodeCount = 3
	}
	nodes := make([]visual.Node, 0, nodeCount)
	nodes = append(nodes, visual.Node{ID: "start", Type: visual.NodeTypeStart})
	for index := 1; index < nodeCount-1; index++ {
		id := fmt.Sprintf("action-%03d", index)
		step := workflow.Step{ID: id, Name: id, Action: "script.shell", Args: map[string]any{"script": "true"}}
		nodes = append(nodes, visual.Node{ID: id, Type: visual.NodeTypeAction, StepID: id, StepName: id, Step: &step, Position: visual.Position{X: float64(index * 120), Y: float64((index % 7) * 80)}})
	}
	nodes = append(nodes, visual.Node{ID: "end", Type: visual.NodeTypeEnd, Position: visual.Position{X: float64((nodeCount - 1) * 120)}})

	edges := make([]visual.Edge, 0, edgeCount)
	for index := 0; index < nodeCount-1; index++ {
		edges = append(edges, visual.Edge{ID: fmt.Sprintf("chain-%03d", index), Source: nodes[index].ID, Target: nodes[index+1].ID, Kind: visual.EdgeKindNext})
	}
	for span := 2; len(edges) < edgeCount && span < nodeCount; span++ {
		for source := 0; len(edges) < edgeCount && source+span < nodeCount; source++ {
			edges = append(edges, visual.Edge{ID: fmt.Sprintf("extra-%03d", len(edges)), Source: nodes[source].ID, Target: nodes[source+span].ID, Kind: visual.EdgeKindNext})
		}
	}
	return visual.Graph{Version: visual.GraphVersion, Workflow: workflow.Workflow{Name: name, Version: "v0.1", Plan: workflow.Plan{Mode: "auto", Strategy: "graph"}}, Nodes: nodes, Edges: edges}
}

func newWorkflowParityRuntime(t *testing.T, tenantID string) *runnerapp.Runtime {
	t.Helper()
	root := t.TempDir()
	cfg := runnerconfig.Default()
	cfg.Auth.Enabled, cfg.UI.Enabled = false, false
	cfg.Stores.WorkflowsDir = filepath.Join(root, "workflows")
	cfg.Stores.ScriptsDir = filepath.Join(root, "scripts")
	cfg.Stores.SkillsDir = filepath.Join(root, "skills")
	cfg.Stores.EnvironmentsDir = filepath.Join(root, "environments")
	cfg.Stores.MCPDir = filepath.Join(root, "mcp")
	cfg.Stores.RunStateFile = filepath.Join(root, "run-state.json")
	cfg.Stores.AgentStateFile = filepath.Join(root, "agents.json")
	runtime, err := runnerapp.NewRuntime(context.Background(), runnerapp.RuntimeOptions{Config: cfg, WorkflowTenantID: tenantID})
	if err != nil {
		t.Fatal(err)
	}
	runtime.SetWorkflowReferenceChecker(workflowParityNoReferences{})
	t.Cleanup(func() { _ = runtime.Close(context.Background()) })
	return runtime
}

type workflowParityNoReferences struct{}

func (workflowParityNoReferences) ReferencesForWorkflow(context.Context, string) ([]service.WorkflowReference, error) {
	return nil, nil
}

func loadWorkflowParityCatalog(t *testing.T) *workflowfixtures.Catalog {
	t.Helper()
	manifest := filepath.Join("..", "..", "..", "testdata", "workflow_canvas_mcp", "test-manifest.json")
	catalog, err := workflowfixtures.LoadCatalog(manifest)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func createWorkflowParityGraph(t *testing.T, handler http.Handler, graph visual.Graph) {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"graph": graph})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/workflows/graph", bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create Runner graph %s: status=%d body=%s", graph.Workflow.Name, recorder.Code, recorder.Body.String())
	}
}

func readWorkflowParityHTTPGraph(t *testing.T, handler http.Handler, workflowID string) visual.Graph {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/workflows/"+workflowID+"/graph", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("read Runner graph %s: status=%d body=%s", workflowID, recorder.Code, recorder.Body.String())
	}
	var graph visual.Graph
	if err := json.Unmarshal(recorder.Body.Bytes(), &graph); err != nil {
		t.Fatal(err)
	}
	return graph
}
