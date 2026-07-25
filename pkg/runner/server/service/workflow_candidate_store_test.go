package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowCandidateConcurrentReviseCASAndTenantPartition(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewWorkflowCandidateFileStore(root)
	keyA := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "shared-workflow"}
	keyB := edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: "shared-workflow"}
	createdAt := time.Date(2026, 7, 21, 6, 0, 0, 0, time.UTC)

	v1 := workflowCandidateTestRevision(keyA, "candidate-1", 1, 0, "change-1", "preview-1", createdAt)
	if _, err := store.Create(ctx, v1); err != nil {
		t.Fatalf("Create(v1) error = %v", err)
	}
	// Mutating caller-owned bytes after Create must not mutate the immutable revision.
	v1.Risk[0] = '['

	variants := []WorkflowCandidateRevision{
		workflowCandidateTestRevision(keyA, "candidate-1", 2, 1, "change-2a", "preview-2a", createdAt.Add(time.Minute)),
		workflowCandidateTestRevision(keyA, "candidate-1", 2, 1, "change-2b", "preview-2b", createdAt.Add(time.Minute)),
	}
	start := make(chan struct{})
	results := make(chan error, len(variants))
	var wg sync.WaitGroup
	stores := []*WorkflowCandidateFileStore{store, NewWorkflowCandidateFileStore(root)}
	for i := range variants {
		wg.Add(1)
		go func(candidateStore *WorkflowCandidateFileStore, next WorkflowCandidateRevision) {
			defer wg.Done()
			<-start
			_, err := candidateStore.Revise(ctx, keyA, "candidate-1", 1, next)
			results <- err
		}(stores[i], variants[i])
	}
	close(start)
	wg.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		var domainErr *edit.DomainError
		if !errors.As(err, &domainErr) || domainErr.Code != edit.ErrorStaleChangeSet {
			t.Fatalf("losing Revise error = %T %v, want STALE_CHANGESET", err, err)
		}
		conflicts++
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("Revise results: successes=%d conflicts=%d, want 1/1", successes, conflicts)
	}

	currentA, err := store.GetCurrent(ctx, keyA, "candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if currentA.CurrentRevision != 2 || (currentA.CurrentPreviewHash != "preview-2a" && currentA.CurrentPreviewHash != "preview-2b") {
		t.Fatalf("current candidate = %+v", currentA)
	}
	revisions, err := store.ListRevisions(ctx, keyA, "candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(revisions) != 2 || revisions[0].State != WorkflowCandidateRevisionSuperseded || revisions[1].State != WorkflowCandidateRevisionReady {
		t.Fatalf("revision history = %+v", revisions)
	}
	var storedRisk map[string]string
	if err := json.Unmarshal(revisions[0].Risk, &storedRisk); err != nil || storedRisk["level"] != "low" {
		t.Fatalf("revision 1 was mutated through caller alias: %s", revisions[0].Risk)
	}

	v1TenantB := workflowCandidateTestRevision(keyB, "candidate-1", 1, 0, "change-b1", "preview-b1", createdAt)
	if _, err := store.Create(ctx, v1TenantB); err != nil {
		t.Fatalf("Create(tenant B) error = %v", err)
	}
	currentB, err := store.GetCurrent(ctx, keyB, "candidate-1")
	if err != nil {
		t.Fatal(err)
	}
	if currentB.CurrentRevision != 1 || currentB.CurrentPreviewHash != "preview-b1" || currentA.Key == currentB.Key {
		t.Fatalf("tenant partition leaked: A=%+v B=%+v", currentA, currentB)
	}
}

func TestWorkflowCandidateRejectsInlineSecretsAndKeepsOnlySecretRefs(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	store := NewWorkflowCandidateFileStore(root)
	key := edit.WorkflowKey{TenantID: "tenant", WorkflowID: "secret-check"}
	createdAt := time.Date(2026, 7, 21, 6, 0, 0, 0, time.UTC)
	invalid := workflowCandidateTestRevision(key, "candidate-inline", 1, 0, "change-inline", "preview-inline", createdAt)
	vars := map[string]any{"password": "inline-credential-sentinel"}
	invalid.NormalizedOperations = []edit.WorkflowOperation{edit.NewOperation(&edit.UpdateWorkflowOperation{
		Op: edit.OperationUpdateWorkflow, Set: edit.WorkflowPatch{Vars: &vars},
	})}

	_, err := store.Create(ctx, invalid)
	if err == nil {
		t.Fatal("Create(inline secret) unexpectedly succeeded")
	}
	var domainErr *edit.DomainError
	if !errors.As(err, &domainErr) || domainErr.Code != edit.ErrorSchemaViolation {
		t.Fatalf("Create(inline secret) error = %T %v", err, err)
	}
	if strings.Contains(err.Error(), "inline-credential-sentinel") {
		t.Fatalf("error leaked secret value: %v", err)
	}
	assertWorkflowCandidateFilesDoNotContain(t, root, "inline-credential-sentinel")

	valid := workflowCandidateTestRevision(key, "candidate-ref", 1, 0, "change-ref", "preview-ref", createdAt)
	refVars := map[string]any{"credential": map[string]any{"secretRef": "secret://workflow/read-only"}}
	valid.NormalizedOperations = []edit.WorkflowOperation{edit.NewOperation(&edit.UpdateWorkflowOperation{
		Op: edit.OperationUpdateWorkflow, Set: edit.WorkflowPatch{Vars: &refVars},
	})}
	if _, err := store.Create(ctx, valid); err != nil {
		t.Fatalf("Create(secretRef) error = %v", err)
	}
	foundRef := false
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr == nil && strings.Contains(string(raw), `"secretRef": "secret://workflow/read-only"`) {
			foundRef = true
		}
		return readErr
	})
	if !foundRef {
		t.Fatal("persisted revision did not retain the secretRef")
	}
}

func workflowCandidateTestRevision(key edit.WorkflowKey, candidateID string, revision, parent uint64, changeSetID, previewHash string, createdAt time.Time) WorkflowCandidateRevision {
	name := "workflow-v" + previewHash
	operation := edit.NewOperation(&edit.UpdateWorkflowOperation{Op: edit.OperationUpdateWorkflow, Set: edit.WorkflowPatch{Name: &name}})
	return WorkflowCandidateRevision{
		WorkflowChangeSet: edit.WorkflowChangeSet{
			SchemaVersion: edit.SchemaVersion, ID: changeSetID, Key: key, CandidateID: candidateID, CandidateRevision: revision,
			Actor: "agent", Intent: "update workflow", Base: edit.Revision{ResourceVersion: "rv-1", SemanticHash: "semantic-1", LayoutHash: "layout-1"},
			Operations: []edit.WorkflowOperation{operation}, PreviewHash: previewHash,
			Validation: edit.ValidationResult{Kind: "validation", Basis: edit.ValidationBasis{ResourceVersion: "rv-1", SemanticHash: "semantic-2", CatalogVersion: "catalog-1", ActionSchemaDigest: "actions-1", ReferenceSnapshotHash: "refs-1", EnvironmentBindingVersion: "environment-1"}, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}},
			State:      WorkflowCandidateRevisionReady, CreatedAt: createdAt, ExpiresAt: createdAt.Add(time.Hour),
		},
		ParentRevision: parent, NormalizedOperations: []edit.WorkflowOperation{operation}, Risk: json.RawMessage(`{"level":"low"}`), Diff: json.RawMessage(`{"updatedNodeIds":[]}`),
		ActorID: "agent-1", CanvasSessionID: "canvas-1", AgentSessionID: "agent-session-1", TurnID: "turn-1", ToolCallID: "tool-1",
	}
}

func assertWorkflowCandidateFilesDoNotContain(t *testing.T, root, needle string) {
	t.Helper()
	_ = filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil || entry.IsDir() {
			return walkErr
		}
		raw, readErr := os.ReadFile(path)
		if readErr == nil && strings.Contains(string(raw), needle) {
			t.Fatalf("%s contains forbidden credential sentinel", path)
		}
		return readErr
	})
}
