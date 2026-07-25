package service

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"runner/workflow/edit"
)

func TestWorkflowWorkspaceProjectionUsesDurableBasisReceiptHistoryAndRedactsDiagnostics(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	ctx := context.Background()
	receipt, err := fixture.app.Apply(ctx, fixture.trusted, fixture.request)
	if err != nil {
		t.Fatal(err)
	}
	current, err := fixture.commits.Snapshot(ctx, fixture.request.WorkflowKey)
	if err != nil {
		t.Fatal(err)
	}
	graph, err := parseCommittedVisualGraph(current.GraphPayload, current.ResourceVersion)
	if err != nil {
		t.Fatal(err)
	}
	backend, ok := fixture.app.backend.(*workflowEditTestBackend)
	if !ok || receipt.RevisionAfter == nil {
		t.Fatal("workflow apply test backend or revision is unavailable")
	}
	backend.snapshot.Graph, backend.snapshot.Revision = graph, *receipt.RevisionAfter

	_, err = fixture.app.RecordPostCheck(ctx, WorkflowPostCheckFacts{
		Key: fixture.request.WorkflowKey, TransactionID: receipt.TransactionID, Basis: receipt.Validation.Basis,
		Outcome: WorkflowPostCheckFailed, Mode: WorkflowDryRunModeDraftNonProduction,
		Diagnostics: []edit.WorkflowDiagnostic{{Code: "ENV_UNREACHABLE", Severity: "error", Message: "provider token super-secret-value failed", Suggestion: "retry with super-secret-value", Details: json.RawMessage(`{"token":"super-secret-value"}`)}},
	})
	if err != nil {
		t.Fatal(err)
	}

	projected, err := fixture.app.workspaceProjectionSnapshot(ctx, fixture.request.WorkflowKey)
	if err != nil {
		t.Fatal(err)
	}
	if projected.RunGateFacts == nil || projected.RunGateFacts.CurrentBasis != current.ValidationBasis || projected.RunGateFacts.Environment.BindingVersion != current.ValidationBasis.EnvironmentBindingVersion {
		t.Fatalf("run gate facts = %#v", projected.RunGateFacts)
	}
	environment := projected.RunGateFacts.Environment
	if environment.BindingStatus != "bound" ||
		environment.Configuration.SchemaVersion != WorkflowEnvironmentCapabilitySchemaVersion ||
		environment.Configuration.Status != "off" ||
		environment.Configuration.Reason != "provider_unavailable" ||
		environment.Configuration.Permission != "unavailable" ||
		environment.Configuration.HandlerAvailable ||
		environment.Configuration.ConfigureAllowed ||
		environment.Configuration.PrimaryActionCount != 0 {
		t.Fatalf("environment capability must fail closed without a trusted provider: %#v", environment)
	}
	if projected.RunGateFacts.PostCheck == nil || projected.RunGateFacts.PostCheck.Status != "saved_with_issues" || projected.RunGateFacts.PostCheck.Mode != WorkflowDryRunModeDraftNonProduction {
		t.Fatalf("post-check = %#v", projected.RunGateFacts.PostCheck)
	}
	if len(projected.Receipts) != 1 || projected.Receipts[0].Status != "saved_with_issues" || len(projected.ValidationArtifacts) != 1 || projected.ValidationArtifacts[0].Status != "saved_with_issues" {
		t.Fatalf("receipt/validation projection = %#v / %#v", projected.Receipts, projected.ValidationArtifacts)
	}
	if len(projected.DurableHistory) < 2 || projected.DurableHistory[0].Source != "durable_transaction" || projected.DurableHistory[0].TransactionID != receipt.TransactionID {
		t.Fatalf("durable history = %#v", projected.DurableHistory)
	}
	encoded, err := json.Marshal(projected)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "super-secret-value") || strings.Contains(string(encoded), "provider token") {
		t.Fatalf("projection leaked raw diagnostic: %s", encoded)
	}
}

func TestWorkflowWorkspaceProjectionSerializesEmptyDiagnosticsAsArrays(t *testing.T) {
	receipt := WorkflowReceiptProjection{
		TransactionID: "transaction-1",
		Validation: edit.ValidationResult{
			Valid:       true,
			Diagnostics: []edit.WorkflowDiagnostic{},
		},
	}
	artifact := projectWorkflowValidationArtifact(receipt, time.Now().UTC())
	encoded, err := json.Marshal(struct {
		Receipt  WorkflowReceiptProjection            `json:"receipt"`
		Artifact WorkflowValidationArtifactProjection `json:"artifact"`
	}{Receipt: receipt, Artifact: artifact})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(encoded), `"diagnostics":[]`) || strings.Contains(string(encoded), `"diagnostics":null`) {
		t.Fatalf("empty diagnostics must stay JSON arrays: %s", encoded)
	}
}

func TestWorkflowWorkspaceProjectionAcceptsOnlyExactLegacySemanticHashAsStaleBasis(t *testing.T) {
	visualService := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	compiled, err := visualService.Compile(context.Background(), sampleVisualGraph())
	if err != nil {
		t.Fatal(err)
	}
	graph, err := parseCommittedVisualGraph([]byte(compiled.YAML), "rv-legacy-hash")
	if err != nil {
		t.Fatal(err)
	}
	current, err := VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := legacyVisualWorkflowSemanticHash(graph)
	if err != nil {
		t.Fatal(err)
	}
	if legacy == current.SemanticHash {
		t.Fatal("fixture must distinguish the legacy duplicate-projection hash")
	}
	snapshot := WorkflowEditSnapshot{Graph: graph, Revision: edit.Revision{
		ResourceVersion: "rv-legacy-hash", SemanticHash: current.SemanticHash, LayoutHash: current.LayoutHash,
	}}
	basis := edit.ValidationBasis{
		ResourceVersion: "rv-legacy-hash", SemanticHash: legacy,
		CatalogVersion: "catalog", ActionSchemaDigest: "actions",
		ReferenceSnapshotHash: "references", EnvironmentBindingVersion: "environment",
	}
	if !workflowProjectionBasisMatchesSnapshot(snapshot, basis) {
		t.Fatal("exact legacy semantic basis must keep the committed Graph editable")
	}
	basis.SemanticHash = "sha256:unrelated"
	if workflowProjectionBasisMatchesSnapshot(snapshot, basis) {
		t.Fatal("unrelated semantic basis must remain fail-closed")
	}
}

func TestWorkflowWorkspaceProjectionFailsClosedForStaleReversibleTransactionAfterRestart(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	first := fixture.applyOperations(t, "candidate-projection-first", "agent-session-1", updatePosition("check", 180, 90))
	latest := fixture.applyOperations(t, "candidate-projection-latest", "agent-session-2", updatePosition("check", 360, 180))
	fixture.restart(t)

	projected, err := fixture.app.workspaceProjectionSnapshot(context.Background(), fixture.backend.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	find := func(transactionID string) WorkflowDurableHistoryProjection {
		t.Helper()
		for _, entry := range projected.DurableHistory {
			if entry.TransactionID == transactionID {
				return entry
			}
		}
		t.Fatalf("transaction %q not found in projection %#v", transactionID, projected.DurableHistory)
		return WorkflowDurableHistoryProjection{}
	}
	if stale := find(first.TransactionID); stale.Reversible {
		t.Fatalf("stale same-path transaction must fail closed, got %#v", stale)
	}
	if current := find(latest.TransactionID); !current.Reversible {
		t.Fatalf("current durable transaction must remain reversible, got %#v", current)
	}
}

func TestWorkflowWorkspaceProjectionProjectsRedoableOnlyFromDurableUndoLineageAcrossRestart(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	target := fixture.applyOperations(t, "candidate-projection-redo-target", "agent-session-1", updatePosition("check", 180, 90))
	undo, err := fixture.app.Undo(context.Background(), WorkflowEditHistoryActionRequest{
		SchemaVersion:       edit.SchemaVersion,
		WorkflowKey:         fixture.backend.snapshot.Key,
		TargetTransactionID: target.TransactionID,
		CandidateID:         "candidate-projection-redoable",
		Expected:            fixture.backend.snapshot.Revision,
		Actor:               edit.ActorKind("user"),
		ActorID:             "actor-1",
		CanvasSessionID:     "canvas-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	// The server-owned action binding must survive between Preview and Apply.
	fixture.restart(t)
	undoReceipt := fixture.applyPreview(t, undo, "apply-projection-redoable")
	fixture.restart(t)

	projected, err := fixture.app.workspaceProjectionSnapshot(context.Background(), fixture.backend.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range projected.DurableHistory {
		if entry.TransactionID != undoReceipt.TransactionID {
			continue
		}
		if !entry.Redoable || entry.Reversible {
			t.Fatalf("durable undo availability = %#v, want redoable only", entry)
		}
		return
	}
	t.Fatalf("undo transaction %q not found in projection %#v", undoReceipt.TransactionID, projected.DurableHistory)
}

func TestWorkflowWorkspaceProjectionDoesNotTrustSpoofedUndoIntent(t *testing.T) {
	fixture := newWorkflowEditHistoryFixture(t)
	request := newWorkflowPreviewRequest(fixture.backend.snapshot, "candidate-spoofed-undo-intent", updatePosition("check", 180, 90))
	request.Intent = "undo workflow transaction transaction-history-seed"
	preview, err := fixture.app.Preview(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	receipt := fixture.applyPreview(t, preview, "apply-spoofed-undo-intent")
	fixture.restart(t)
	projected, err := fixture.app.workspaceProjectionSnapshot(context.Background(), fixture.backend.snapshot.Key)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range projected.DurableHistory {
		if entry.TransactionID == receipt.TransactionID {
			if entry.Redoable || !entry.Reversible {
				t.Fatalf("spoofed intent changed authoritative availability: %#v", entry)
			}
			return
		}
	}
	t.Fatalf("transaction %q not found in projection %#v", receipt.TransactionID, projected.DurableHistory)
}
