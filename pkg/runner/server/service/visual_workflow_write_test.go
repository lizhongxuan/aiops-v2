package service

import (
	"context"
	"encoding/json"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestVisualWorkflowCreateGraphUsesCommitStoreWithoutLegacyWrite(t *testing.T) {
	root := t.TempDir()
	legacy := NewWorkflowService(filepath.Join(root, "legacy"))
	commits, err := NewWorkflowCommitStore(filepath.Join(root, "commits"))
	if err != nil {
		t.Fatalf("new commit store: %v", err)
	}
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{
		WorkflowService: legacy,
		CommitStore:     commits,
		TenantID:        "tenant-a",
	})
	graph := sampleVisualGraph()
	graph.Workflow.Name = "commit-only-create"

	created, err := svc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{Labels: map[string]string{"source": "visual"}, SaveNote: "initial draft"})
	if err != nil {
		t.Fatalf("create graph: %v", err)
	}
	if created.Graph.UI[graphResourceVersionKey] == "" {
		t.Fatal("created graph did not expose the committed resource version")
	}
	if _, err := legacy.Get(context.Background(), graph.Workflow.Name); !errors.Is(err, ErrNotFound) {
		t.Fatalf("visual create retained a legacy graph write path: %v", err)
	}
	snapshot, err := commits.Snapshot(context.Background(), svc.visualWorkflowKey(graph.Workflow.Name))
	if err != nil {
		t.Fatalf("load committed snapshot: %v", err)
	}
	if !strings.Contains(string(snapshot.GraphPayload), "name: commit-only-create") {
		t.Fatalf("committed graph payload mismatch:\n%s", snapshot.GraphPayload)
	}
	if got, want := snapshot.ValidationBasis.ResourceVersion, graphExpectedResourceVersion(created.Graph); got != want {
		t.Fatalf("validation basis resource version = %q, want committed %q", got, want)
	}
	var meta workflowMeta
	if err := json.Unmarshal(snapshot.Meta.Payload, &meta); err != nil {
		t.Fatalf("decode committed metadata: %v", err)
	}
	if meta.Labels["source"] != "visual" || meta.SaveNote != "initial draft" || meta.Status != WorkflowStatusDraft {
		t.Fatalf("committed metadata mismatch: %+v", meta)
	}
}

func TestVisualWorkflowSaveGraphResourceVersionForMetadataOnly(t *testing.T) {
	root := t.TempDir()
	commits, err := NewWorkflowCommitStore(filepath.Join(root, "commits"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{
		WorkflowService: NewWorkflowService(filepath.Join(root, "legacy")), CommitStore: commits, TenantID: "tenant-a",
	})
	graph := sampleVisualGraph()
	graph.Workflow.Name = "metadata-save"
	created, err := svc.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{})
	if err != nil {
		t.Fatalf("create graph: %v", err)
	}
	before := graphExpectedResourceVersion(created.Graph)
	if _, err := svc.SaveGraphWithOptions(context.Background(), graph.Workflow.Name, created.Graph, VisualWorkflowSaveOptions{SaveNote: "reviewed"}); err != nil {
		t.Fatalf("metadata-only save: %v", err)
	}
	latest, err := svc.GetGraph(context.Background(), graph.Workflow.Name)
	if err != nil {
		t.Fatalf("get latest graph: %v", err)
	}
	if after := graphExpectedResourceVersion(latest); after == "" || after == before {
		t.Fatalf("metadata-only resource version = %q, want non-empty value different from %q", after, before)
	}
	snapshot, err := commits.Snapshot(context.Background(), svc.visualWorkflowKey(graph.Workflow.Name))
	if err != nil {
		t.Fatal(err)
	}
	var meta workflowMeta
	if err := json.Unmarshal(snapshot.Meta.Payload, &meta); err != nil {
		t.Fatal(err)
	}
	if meta.SaveNote != "reviewed" {
		t.Fatalf("save note = %q, want reviewed", meta.SaveNote)
	}
}

func TestVisualWorkflowWriteTenantIsolation(t *testing.T) {
	root := t.TempDir()
	commits, err := NewWorkflowCommitStore(filepath.Join(root, "commits"))
	if err != nil {
		t.Fatal(err)
	}
	newTenantService := func(tenant string) *VisualWorkflowService {
		return NewVisualWorkflowService(VisualWorkflowServiceConfig{CommitStore: commits, TenantID: tenant})
	}
	graphA := sampleVisualGraph()
	graphA.Workflow.Name, graphA.Workflow.Description = "shared-name", "tenant a"
	graphB := sampleVisualGraph()
	graphB.Workflow.Name, graphB.Workflow.Description = "shared-name", "tenant b"
	if _, err := newTenantService("tenant-a").CreateGraph(context.Background(), graphA, VisualWorkflowCreateOptions{}); err != nil {
		t.Fatalf("create tenant a: %v", err)
	}
	if _, err := newTenantService("tenant-b").CreateGraph(context.Background(), graphB, VisualWorkflowCreateOptions{}); err != nil {
		t.Fatalf("create tenant b: %v", err)
	}
	gotA, err := newTenantService("tenant-a").GetGraph(context.Background(), graphA.Workflow.Name)
	if err != nil {
		t.Fatal(err)
	}
	gotB, err := newTenantService("tenant-b").GetGraph(context.Background(), graphB.Workflow.Name)
	if err != nil {
		t.Fatal(err)
	}
	if gotA.Workflow.Description != "tenant a" || gotB.Workflow.Description != "tenant b" {
		t.Fatalf("tenant graphs collided: a=%q b=%q", gotA.Workflow.Description, gotB.Workflow.Description)
	}
}

func TestVisualWorkflowSaveGraphLegacyBootstrapRetryIsIdempotent(t *testing.T) {
	root := t.TempDir()
	legacy := NewWorkflowService(filepath.Join(root, "legacy"))
	graph := sampleVisualGraph()
	graph.Workflow.Name = "legacy-retry"
	compiled, err := NewVisualWorkflowService(VisualWorkflowServiceConfig{}).Compile(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	if err := legacy.Create(context.Background(), &WorkflowRecord{Name: graph.Workflow.Name, RawYAML: []byte(compiled.YAML)}); err != nil {
		t.Fatal(err)
	}
	commits, err := NewWorkflowCommitStore(filepath.Join(root, "commits"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewVisualWorkflowService(VisualWorkflowServiceConfig{WorkflowService: legacy, CommitStore: commits, TenantID: "tenant-a"})
	loaded, err := svc.GetGraph(context.Background(), graph.Workflow.Name)
	if err != nil {
		t.Fatal(err)
	}
	loaded.Workflow.Description = "migrated once"
	if _, err := svc.SaveGraph(context.Background(), graph.Workflow.Name, loaded); err != nil {
		t.Fatalf("first bootstrap save: %v", err)
	}
	if _, err := svc.SaveGraph(context.Background(), graph.Workflow.Name, loaded); err != nil {
		t.Fatalf("retry bootstrap save: %v", err)
	}
	snapshot, err := commits.Snapshot(context.Background(), svc.visualWorkflowKey(graph.Workflow.Name))
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 {
		t.Fatalf("idempotent retry produced generation %d, want 1", snapshot.Generation)
	}
}

func TestVisualWorkflowSaveGraphConcurrentResourceVersion(t *testing.T) {
	root := t.TempDir()
	commits, err := NewWorkflowCommitStore(filepath.Join(root, "commits"))
	if err != nil {
		t.Fatalf("new commit store: %v", err)
	}
	newService := func() *VisualWorkflowService {
		return NewVisualWorkflowService(VisualWorkflowServiceConfig{
			WorkflowService: NewWorkflowService(filepath.Join(root, "legacy")),
			CommitStore:     commits,
			TenantID:        "tenant-a",
		})
	}
	creator := newService()
	graph := sampleVisualGraph()
	graph.Workflow.Name = "concurrent-save"
	if _, err := creator.CreateGraph(context.Background(), graph, VisualWorkflowCreateOptions{}); err != nil {
		t.Fatalf("create graph: %v", err)
	}

	first, err := newService().GetGraph(context.Background(), graph.Workflow.Name)
	if err != nil {
		t.Fatalf("load first graph: %v", err)
	}
	second, err := newService().GetGraph(context.Background(), graph.Workflow.Name)
	if err != nil {
		t.Fatalf("load second graph: %v", err)
	}
	first.Workflow.Description = "first writer"
	second.Workflow.Description = "second writer"

	start := make(chan struct{})
	errs := make(chan error, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for _, input := range []struct {
		svc   *VisualWorkflowService
		graph RunnerGraph
	}{{newService(), first}, {newService(), second}} {
		go func(input struct {
			svc   *VisualWorkflowService
			graph RunnerGraph
		}) {
			ready.Done()
			<-start
			_, err := input.svc.SaveGraph(context.Background(), graph.Workflow.Name, input.graph)
			errs <- err
		}(input)
	}
	ready.Wait()
	close(start)

	successes, conflicts := 0, 0
	for range 2 {
		err := <-errs
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrConflict):
			conflicts++
		default:
			t.Fatalf("unexpected concurrent save error: %v", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent saves = %d success/%d conflict, want 1/1", successes, conflicts)
	}

	latest, err := creator.GetGraph(context.Background(), graph.Workflow.Name)
	if err != nil {
		t.Fatalf("load latest graph: %v", err)
	}
	snapshot, err := commits.Snapshot(context.Background(), creator.visualWorkflowKey(graph.Workflow.Name))
	if err != nil {
		t.Fatalf("load latest snapshot: %v", err)
	}
	if got := graphExpectedResourceVersion(latest); got != snapshot.ResourceVersion {
		t.Fatalf("UI resource version = %q, snapshot = %q", got, snapshot.ResourceVersion)
	}
}

func TestWorkflowWriteBoundary(t *testing.T) {
	dir := filepath.Dir(mustSourceFile(t))
	legacy := parseGoFile(t, filepath.Join(dir, "visual_workflow_service.go"))
	workflowService := parseGoFile(t, filepath.Join(dir, "workflow_service.go"))
	legacyStore := parseGoFile(t, filepath.Join(dir, "..", "..", "workflowstore", "store.go"))
	writeBoundary := parseGoFile(t, filepath.Join(dir, "visual_workflow_write.go"))
	for _, method := range []string{"Create", "Update", "Put", "WriteFile"} {
		if hasSelectorCall(legacy, method) {
			t.Fatalf("visual workflow service retained forbidden write call %q", method)
		}
	}
	for _, method := range []string{"Put", "WriteFile", "CreateTemp", "Rename", "Remove", "RemoveAll"} {
		if hasSelectorCall(workflowService, method) || hasSelectorCall(legacyStore, method) {
			t.Fatalf("legacy workflow boundary retained forbidden write call %q", method)
		}
	}
	if !hasQualifiedSelectorCall(writeBoundary, "s", "commitStore", "Commit") {
		t.Fatal("visual write boundary does not delegate to CommitStore")
	}
}

func parseGoFile(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}
	return file
}

func hasSelectorCall(file *ast.File, method string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		selector, ok := call.Fun.(*ast.SelectorExpr)
		if ok && selector.Sel.Name == method {
			found = true
			return false
		}
		return true
	})
	return found
}

func hasQualifiedSelectorCall(file *ast.File, receiver, field, method string) bool {
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		outer, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		inner, innerOK := outer.X.(*ast.SelectorExpr)
		if !innerOK {
			return true
		}
		ident, identOK := inner.X.(*ast.Ident)
		if identOK && ident.Name == receiver && inner.Sel.Name == field && outer.Sel.Name == method {
			found = true
			return false
		}
		return true
	})
	return found
}

func mustSourceFile(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve test source file")
	}
	return file
}
