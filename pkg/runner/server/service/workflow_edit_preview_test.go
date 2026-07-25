package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEditPreviewBuildsCompleteBasisAndDoesNotMutateCommitted(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	store := NewWorkflowCandidateFileStore(t.TempDir())
	app := newWorkflowPreviewTestApplication(t, backend, store)
	committedRevision, committedPosition := backend.snapshot.Revision, nodePosition(backend.snapshot.Graph, "check")
	request := newWorkflowPreviewRequest(backend.snapshot, "candidate-basis", updatePosition("check", 120, 80))
	request.ValidationBasis = edit.ValidationBasis{ResourceVersion: "client-forged", SemanticHash: "client-forged", CatalogVersion: "client-forged", ActionSchemaDigest: "client-forged", ReferenceSnapshotHash: "client-forged", EnvironmentBindingVersion: "client-forged"}

	preview, err := app.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CandidateRevision != 1 || preview.ChangeSet.PreviewHash == "" || preview.ChangeSet.PreviewHash != preview.PreviewHash {
		t.Fatalf("preview identity = %#v", preview)
	}
	if err := preview.ValidationBasis.Validate(); err != nil || preview.ValidationBasis.ResourceVersion != committedRevision.ResourceVersion || preview.ValidationBasis.CatalogVersion == "client-forged" {
		t.Fatalf("basis was not server-generated: %#v err=%v", preview.ValidationBasis, err)
	}
	if backend.validateCalls != 1 || backend.compileCalls != 1 || !preview.ChangeSet.Validation.Valid {
		t.Fatalf("real gates were not run: validate=%d compile=%d validation=%#v", backend.validateCalls, backend.compileCalls, preview.ChangeSet.Validation)
	}
	if len(preview.Diff.Graph.TouchedPaths) == 0 || len(preview.Diff.Inverse.Operations) == 0 || preview.Risk.Level == "" {
		t.Fatalf("preview artifacts incomplete: diff=%#v risk=%#v", preview.Diff, preview.Risk)
	}
	restored, err := edit.ApplyInverse(preview.Graph, preview.Diff.Inverse)
	if err != nil || nodePosition(restored.Graph, "check") != committedPosition {
		t.Fatalf("inverse did not restore the isolated graph: graph=%#v err=%v", restored.Graph, err)
	}
	rawPreview, err := json.Marshal(preview)
	if err != nil {
		t.Fatal(err)
	}
	assertWorkflowPreviewJSON(t, rawPreview)
	if backend.snapshot.Revision != committedRevision || nodePosition(backend.snapshot.Graph, "check") != committedPosition {
		t.Fatalf("preview mutated committed snapshot: before=%#v after=%#v", committedRevision, backend.snapshot.Revision)
	}
	persisted, err := store.GetRevision(context.Background(), request.WorkflowKey, request.CandidateID, 1)
	if err != nil || persisted.PreviewHash != preview.PreviewHash || persisted.Validation.Basis != preview.ValidationBasis {
		t.Fatalf("persisted preview = %#v err=%v", persisted, err)
	}
	changedBasis := preview.ValidationBasis
	changedBasis.CatalogVersion += "-changed"
	if previewHashForTest(t, request.WorkflowKey, 1, "", persisted.NormalizedOperations, changedBasis) == preview.PreviewHash {
		t.Fatal("preview hash did not cover complete validation basis")
	}
	changedOps := append([]edit.WorkflowOperation(nil), persisted.NormalizedOperations...)
	changedOps[0] = updatePosition("check", 121, 80)
	if previewHashForTest(t, request.WorkflowKey, 1, "", changedOps, preview.ValidationBasis) == preview.PreviewHash {
		t.Fatal("preview hash did not cover normalized operations")
	}
}

func TestWorkflowCandidateRevisionV1V2V3IsImmutableAndOldPreviewExpires(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	store := NewWorkflowCandidateFileStore(t.TempDir())
	app := newWorkflowPreviewTestApplication(t, backend, store)
	v1, err := app.Preview(context.Background(), newWorkflowPreviewRequest(backend.snapshot, "candidate-chain", updatePosition("check", 100, 50)))
	if err != nil {
		t.Fatal(err)
	}
	v2Request := newWorkflowPreviewRequest(backend.snapshot, "candidate-chain")
	v2Request.BaseChangeSetID = v1.ChangeSet.ID
	v2Request.AdditionalOperations = []edit.WorkflowOperation{updatePosition("repair", 220, 50)}
	v2, err := app.Preview(context.Background(), v2Request)
	if err != nil {
		t.Fatal(err)
	}
	v3Request := newWorkflowPreviewRequest(backend.snapshot, "candidate-chain")
	v3Request.BaseChangeSetID = v2.ChangeSet.ID
	v3Request.AdditionalOperations = []edit.WorkflowOperation{updatePosition("end", 340, 50)}
	v3, err := app.Preview(context.Background(), v3Request)
	if err != nil {
		t.Fatal(err)
	}

	revisions, err := store.ListRevisions(context.Background(), workflowEditTestKey(), "candidate-chain")
	if err != nil || len(revisions) != 3 {
		t.Fatalf("revisions=%#v err=%v", revisions, err)
	}
	if revisions[0].State != WorkflowCandidateRevisionSuperseded || revisions[1].State != WorkflowCandidateRevisionSuperseded || revisions[2].State != WorkflowCandidateRevisionReady {
		t.Fatalf("immutable states = %q/%q/%q", revisions[0].State, revisions[1].State, revisions[2].State)
	}
	for index, revision := range revisions {
		want := index + 1
		if int(revision.CandidateRevision) != want || len(revision.NormalizedOperations) != want {
			t.Fatalf("revision %d = %#v", want, revision)
		}
	}
	if v1.PreviewHash == v2.PreviewHash || v2.PreviewHash == v3.PreviewHash {
		t.Fatal("candidate revisions reused preview hash")
	}
	_, err = store.CheckApplyEligibility(context.Background(), WorkflowCandidateApplyBinding{Key: workflowEditTestKey(), CandidateID: "candidate-chain", CandidateRevision: 1, ChangeSetID: v1.ChangeSet.ID, PreviewHash: v1.PreviewHash}, time.Now())
	assertWorkflowPreviewCode(t, err, edit.ErrorStaleChangeSet)
	_, err = store.CheckApplyEligibility(context.Background(), WorkflowCandidateApplyBinding{Key: workflowEditTestKey(), CandidateID: "candidate-chain", CandidateRevision: 3, ChangeSetID: v3.ChangeSet.ID, PreviewHash: v1.PreviewHash}, time.Now())
	assertWorkflowPreviewCode(t, err, edit.ErrorPreviewMismatch)

	stale := newWorkflowPreviewRequest(backend.snapshot, "candidate-chain")
	stale.BaseChangeSetID = v1.ChangeSet.ID
	stale.AdditionalOperations = []edit.WorkflowOperation{updatePosition("start", 5, 5)}
	_, err = app.Preview(context.Background(), stale)
	assertWorkflowPreviewCode(t, err, edit.ErrorStaleChangeSet)
}

func TestWorkflowLayoutRebaseAllowsOnlyDisjointAllowlistedDrift(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	store := NewWorkflowCandidateFileStore(t.TempDir())
	app := newWorkflowPreviewTestApplication(t, backend, store)
	v1, err := app.Preview(context.Background(), newWorkflowPreviewRequest(backend.snapshot, "candidate-rebase", updatePosition("check", 100, 50)))
	if err != nil {
		t.Fatal(err)
	}
	current := backend.snapshot.Graph
	setNodePosition(&current, "repair", visual.Position{X: 300, Y: 160})
	setWorkflowPreviewBackendSnapshot(t, backend, current, "runner-rv-2")
	revise := newWorkflowPreviewRequest(backend.snapshot, "candidate-rebase")
	revise.Expected = v1.ChangeSet.Base
	revise.BaseChangeSetID = v1.ChangeSet.ID
	revise.AdditionalOperations = []edit.WorkflowOperation{updatePosition("end", 420, 160)}

	v2, err := app.Preview(context.Background(), revise)
	if err != nil {
		t.Fatal(err)
	}
	if !v2.Rebased || v2.ChangeSet.Base.ResourceVersion != "runner-rv-2" || nodePosition(v2.Graph, "repair") != (visual.Position{X: 300, Y: 160}) || nodePosition(v2.Graph, "check") != (visual.Position{X: 100, Y: 50}) {
		t.Fatalf("safe layout rebase = %#v graph=%#v", v2, v2.Graph)
	}
	if nodePosition(backend.snapshot.Graph, "check") == (visual.Position{X: 100, Y: 50}) {
		t.Fatal("ghost candidate leaked into committed graph")
	}
}

func TestWorkflowLayoutRebaseIgnoresCompiledWorkflowProjectionDrift(t *testing.T) {
	base := workflowPreviewRoundTripGraph(t, workflowEditTestGraph())
	backend := newWorkflowEditTestBackend(t, base)
	store := NewWorkflowCandidateFileStore(t.TempDir())
	app := newWorkflowPreviewTestApplication(t, backend, store)

	checkStep := *base.Nodes[1].Step
	checkStep.Args = map[string]any{"script": "echo candidate"}
	v1, err := app.Preview(context.Background(), newWorkflowPreviewRequest(
		backend.snapshot,
		"candidate-projection-rebase",
		edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Step: &checkStep}}),
	))
	if err != nil {
		t.Fatal(err)
	}

	current := backend.snapshot.Graph
	setNodePosition(&current, "end", visual.Position{X: 640, Y: 240})
	current = workflowPreviewRoundTripGraph(t, current)
	setWorkflowPreviewBackendSnapshot(t, backend, current, "runner-rv-2")
	repairStep := *current.Nodes[2].Step
	repairStep.Args = map[string]any{"script": "echo revised candidate"}
	revise := newWorkflowPreviewRequest(backend.snapshot, "candidate-projection-rebase")
	revise.Expected = v1.ChangeSet.Base
	revise.BaseChangeSetID = v1.ChangeSet.ID
	revise.AdditionalOperations = []edit.WorkflowOperation{
		edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "repair", Set: edit.NodePatch{Step: &repairStep}}),
	}

	v2, err := app.Preview(context.Background(), revise)
	if err != nil {
		t.Fatal(err)
	}
	if !v2.Rebased || v2.ChangeSet.Base.ResourceVersion != "runner-rv-2" || nodePosition(v2.Graph, "end") != (visual.Position{X: 640, Y: 240}) {
		t.Fatalf("projection-safe layout rebase = %#v graph=%#v", v2, v2.Graph)
	}
}

func TestWorkflowLayoutRebaseReturnsTypedThreeWayConflicts(t *testing.T) {
	tests := []struct {
		name     string
		mutate   func(*visual.Graph)
		wantPath string
	}{
		{"same field layout", func(graph *visual.Graph) { setNodePosition(graph, "check", visual.Position{X: 999, Y: 999}) }, "/nodes/check/position"},
		{"port drift", func(graph *visual.Graph) { graph.Nodes[1].Ports[1].Label = "changed output" }, "/nodes/check/ports"},
		{"semantic drift", func(graph *visual.Graph) { graph.Nodes[2].Step.Args["script"] = "echo concurrently changed" }, "/semanticHash"},
		{"unclassified graph drift", func(graph *visual.Graph) { graph.Workflow.Description = "changed metadata" }, "/graph"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
			store := NewWorkflowCandidateFileStore(t.TempDir())
			app := newWorkflowPreviewTestApplication(t, backend, store)
			v1, err := app.Preview(context.Background(), newWorkflowPreviewRequest(backend.snapshot, "candidate-conflict", updatePosition("check", 100, 50)))
			if err != nil {
				t.Fatal(err)
			}
			current := backend.snapshot.Graph
			test.mutate(&current)
			setWorkflowPreviewBackendSnapshot(t, backend, current, "runner-rv-2")
			revise := newWorkflowPreviewRequest(backend.snapshot, "candidate-conflict")
			revise.Expected = v1.ChangeSet.Base
			revise.BaseChangeSetID = v1.ChangeSet.ID
			revise.AdditionalOperations = []edit.WorkflowOperation{updatePosition("end", 400, 100)}
			_, err = app.Preview(context.Background(), revise)
			assertWorkflowPreviewCode(t, err, edit.ErrorStaleRevision)
			var domain *edit.DomainError
			if !errors.As(err, &domain) || !containsWorkflowPreviewPath(domain.ConflictingPaths, test.wantPath) || len(domain.Details) == 0 {
				t.Fatalf("conflict=%#v err=%v, want path %s", domain, err, test.wantPath)
			}
			currentCandidate, getErr := store.GetCurrent(context.Background(), workflowEditTestKey(), "candidate-conflict")
			if getErr != nil || currentCandidate.CurrentRevision != 1 {
				t.Fatalf("conflict persisted a revision: candidate=%#v err=%v", currentCandidate, getErr)
			}
		})
	}
}

func TestWorkflowCandidateDirectReviseSupportsCanonicalOperationsWithoutCommit(t *testing.T) {
	tests := []struct {
		name      string
		actor     edit.ActorKind
		operation func(visual.Graph) edit.WorkflowOperation
	}{
		{
			name: "parameter", actor: edit.ActorKind("assistant"),
			operation: func(graph visual.Graph) edit.WorkflowOperation {
				step := *graph.Nodes[1].Step
				step.Args = map[string]any{"script": "echo candidate parameter"}
				return edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Step: &step}})
			},
		},
		{name: "position", actor: edit.ActorKind("agent"), operation: func(visual.Graph) edit.WorkflowOperation { return updatePosition("check", 360, 180) }},
		{
			name: "connection", actor: edit.ActorKind("user"),
			operation: func(visual.Graph) edit.WorkflowOperation {
				kind := visual.EdgeKindFailure
				return edit.NewOperation(&edit.UpdateEdgeOperation{EdgeID: "check-repair", Set: edit.EdgePatch{Kind: &kind}})
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
			store := NewWorkflowCandidateFileStore(t.TempDir())
			app := newWorkflowPreviewTestApplication(t, backend, store)
			committedRevision, committedGraph := backend.snapshot.Revision, backend.snapshot.Graph
			initialRequest := newWorkflowPreviewRequest(backend.snapshot, "candidate-direct-"+test.name, updatePosition("repair", 180, 90))
			initialRequest.Actor = test.actor
			initial, err := app.Preview(context.Background(), initialRequest)
			if err != nil {
				t.Fatal(err)
			}
			result, err := app.ReviseCandidate(context.Background(), newWorkflowCandidateDirectReviseRequest(initial, test.operation(backend.snapshot.Graph), "revise-"+test.name))
			if err != nil {
				t.Fatal(err)
			}
			if result.CandidateRevision != 2 || result.ChangeSet.Actor != edit.ActorKind("user") || result.ChangeSet.ParentChangeSetID != initial.ChangeSet.ID || result.PreviewHash == initial.PreviewHash {
				t.Fatalf("direct revision = %#v", result)
			}
			revisions, err := store.ListRevisions(context.Background(), backend.snapshot.Key, initial.Candidate.ID)
			if err != nil || len(revisions) != 2 || revisions[0].State != WorkflowCandidateRevisionSuperseded || revisions[1].State != WorkflowCandidateRevisionReady {
				t.Fatalf("candidate states = %#v err=%v", revisions, err)
			}
			if backend.snapshot.Revision != committedRevision {
				t.Fatalf("committed revision changed: before=%#v after=%#v", committedRevision, backend.snapshot.Revision)
			}
			before, _ := workflowQueryDigest(committedGraph)
			after, _ := workflowQueryDigest(backend.snapshot.Graph)
			if before != after {
				t.Fatalf("committed graph changed: before=%s after=%s", before, after)
			}
			if strings.Contains(result.ChangeSet.ID, "revise-"+test.name) {
				t.Fatalf("raw idempotency key leaked in changeSetId: %s", result.ChangeSet.ID)
			}
		})
	}
}

func TestWorkflowCandidateDirectReviseStableReplayAndPayloadMismatch(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	store := NewWorkflowCandidateFileStore(t.TempDir())
	app := newWorkflowPreviewTestApplication(t, backend, store)
	initial, err := app.Preview(context.Background(), newWorkflowPreviewRequest(backend.snapshot, "candidate-replay", updatePosition("repair", 180, 90)))
	if err != nil {
		t.Fatal(err)
	}
	request := newWorkflowCandidateDirectReviseRequest(initial, updatePosition("check", 360, 180), "stable-retry-key")
	first, err := app.ReviseCandidate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	replayed, err := app.ReviseCandidate(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if replayed.ChangeSet.ID != first.ChangeSet.ID || replayed.PreviewHash != first.PreviewHash || replayed.CandidateRevision != first.CandidateRevision {
		t.Fatalf("replay=%#v first=%#v", replayed, first)
	}
	revisions, err := store.ListRevisions(context.Background(), backend.snapshot.Key, initial.Candidate.ID)
	if err != nil || len(revisions) != 2 {
		t.Fatalf("duplicate replay accumulated revisions: count=%d err=%v", len(revisions), err)
	}

	mismatch := request
	mismatch.AdditionalOperation = updatePosition("check", 361, 180)
	_, err = app.ReviseCandidate(context.Background(), mismatch)
	assertWorkflowPreviewCode(t, err, edit.ErrorIdempotencyKeyReused)
}

func TestWorkflowCandidateDirectReviseRejectsForgedOrCrossScopeIdentity(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowPreviewTestApplication(t, backend, NewWorkflowCandidateFileStore(t.TempDir()))
	initial, err := app.Preview(context.Background(), newWorkflowPreviewRequest(backend.snapshot, "candidate-scope-direct", updatePosition("repair", 180, 90)))
	if err != nil {
		t.Fatal(err)
	}
	base := newWorkflowCandidateDirectReviseRequest(initial, updatePosition("check", 360, 180), "scope-key")
	tests := []struct {
		name   string
		mutate func(*WorkflowCandidateReviseRequest)
		code   edit.ErrorCode
	}{
		{"actor", func(request *WorkflowCandidateReviseRequest) { request.ActorID = "actor-forged" }, edit.ErrorApprovalRequired},
		{"canvas", func(request *WorkflowCandidateReviseRequest) { request.CanvasSessionID = "canvas-forged" }, edit.ErrorApprovalRequired},
		{"missing full revision", func(request *WorkflowCandidateReviseRequest) { request.Expected.LayoutHash = "" }, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := base
			test.mutate(&request)
			_, err := app.ReviseCandidate(context.Background(), request)
			if test.code == "" {
				if !errors.Is(err, ErrInvalid) {
					t.Fatalf("error=%v, want ErrInvalid", err)
				}
				return
			}
			assertWorkflowPreviewCode(t, err, test.code)
		})
	}
}

func TestWorkflowPreviewWithoutRevisionIdempotencyKeepsLegacyIdentity(t *testing.T) {
	backend := newWorkflowEditTestBackend(t, workflowEditTestGraph())
	app := newWorkflowPreviewTestApplication(t, backend, NewWorkflowCandidateFileStore(t.TempDir()))
	request := newWorkflowPreviewRequest(backend.snapshot, "candidate-compatible", updatePosition("check", 120, 80))
	result, err := app.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	want := workflowPreviewChangeSetID(request.WorkflowKey, request.CandidateID, result.CandidateRevision, "", result.PreviewHash)
	if result.ChangeSet.ID != want || strings.HasPrefix(result.ChangeSet.ID, "changeset-revise-") {
		t.Fatalf("ordinary Preview identity changed: got=%s want=%s", result.ChangeSet.ID, want)
	}
	raw, err := json.Marshal(request)
	if err != nil || strings.Contains(string(raw), "RevisionIdempotencyKey") || strings.Contains(string(raw), "revisionIdempotencyKey") {
		t.Fatalf("server-only field serialized: %s err=%v", raw, err)
	}
}

func newWorkflowCandidateDirectReviseRequest(initial WorkflowEditPreviewResult, operation edit.WorkflowOperation, idempotencyKey string) WorkflowCandidateReviseRequest {
	return WorkflowCandidateReviseRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: initial.ChangeSet.Key, CandidateID: initial.Candidate.ID,
		BaseChangeSetID: initial.ChangeSet.ID, Expected: initial.ChangeSet.Base, IdempotencyKey: idempotencyKey,
		Intent: "directly revise candidate", AdditionalOperation: operation,
		ActorID: "actor-1", CanvasSessionID: "canvas-1", SessionID: "session-1",
	}
}

func newWorkflowPreviewTestApplication(t *testing.T, backend WorkflowEditReadBackend, store WorkflowCandidateStore) *WorkflowEditApplication {
	t.Helper()
	app, err := NewWorkflowEditApplication(backend, workflowEditTestAccess{}, &workflowEditTestReferences{}, store)
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func newWorkflowPreviewRequest(snapshot WorkflowEditSnapshot, candidateID string, operations ...edit.WorkflowOperation) WorkflowEditPreviewRequest {
	return WorkflowEditPreviewRequest{
		PreviewPatchRequest: edit.PreviewPatchRequest{SchemaVersion: edit.SchemaVersion, WorkflowKey: snapshot.Key, Expected: snapshot.Revision, Intent: "adjust workflow", Operations: operations},
		CandidateID:         candidateID, Actor: edit.ActorKind("assistant"), ActorID: "actor-1", CanvasSessionID: "canvas-1",
	}
}

func updatePosition(nodeID string, x, y float64) edit.WorkflowOperation {
	position := visual.Position{X: x, Y: y}
	return edit.NewOperation(&edit.UpdateNodeOperation{NodeID: nodeID, Set: edit.NodePatch{Position: &position}})
}

func setWorkflowPreviewBackendSnapshot(t *testing.T, backend *workflowEditTestBackend, graph visual.Graph, resourceVersion string) {
	t.Helper()
	hashes, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	backend.snapshot = WorkflowEditSnapshot{Key: workflowEditTestKey(), Revision: edit.Revision{ResourceVersion: resourceVersion, SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash}, Graph: graph}
}

func setNodePosition(graph *visual.Graph, id string, position visual.Position) {
	for index := range graph.Nodes {
		if graph.Nodes[index].ID == id {
			graph.Nodes[index].Position = position
		}
	}
}

func workflowPreviewRoundTripGraph(t *testing.T, graph visual.Graph) visual.Graph {
	t.Helper()
	raw, err := visual.CompileGraphToYAML(graph)
	if err != nil {
		t.Fatal(err)
	}
	roundTripped, err := visual.ParseYAMLToGraph(raw)
	if err != nil {
		t.Fatal(err)
	}
	return roundTripped
}

func nodePosition(graph visual.Graph, id string) visual.Position {
	for _, node := range graph.Nodes {
		if node.ID == id {
			return node.Position
		}
	}
	return visual.Position{}
}

func previewHashForTest(t *testing.T, key edit.WorkflowKey, revision uint64, parent string, operations []edit.WorkflowOperation, basis edit.ValidationBasis) string {
	t.Helper()
	hash, err := workflowPreviewHash(key, revision, parent, operations, basis)
	if err != nil {
		t.Fatal(err)
	}
	return hash
}

func assertWorkflowPreviewCode(t *testing.T, err error, want edit.ErrorCode) {
	t.Helper()
	var domain *edit.DomainError
	if !errors.As(err, &domain) || domain.Code != want {
		t.Fatalf("error=%v, want domain code %s", err, want)
	}
}

func containsWorkflowPreviewPath(paths []string, want string) bool {
	for _, path := range paths {
		if path == want {
			return true
		}
	}
	return false
}

func assertWorkflowPreviewJSON(t *testing.T, raw json.RawMessage) {
	t.Helper()
	if len(raw) == 0 || !json.Valid(raw) || strings.Contains(string(raw), "client-forged") || strings.Contains(string(raw), "must-not-be-copied") {
		t.Fatalf("invalid preview JSON: %s", raw)
	}
}
