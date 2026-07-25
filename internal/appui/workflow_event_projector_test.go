package appui

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"runner/server/service"
	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowEventProjectorDeduplicatesBuffersGapAndResyncsSnapshot(t *testing.T) {
	projector := NewWorkflowEventProjector()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	second := workflowProjectorEvent(key, 2, "event-2", "transaction-2")
	projection := projector.Apply(second)
	if projection.Gap == nil || projection.Gap.ExpectedSequence != 1 || projection.Gap.ReceivedSequence != 2 || !projection.SnapshotResyncRequired {
		t.Fatalf("gap projection = %#v", projection)
	}
	first := workflowProjectorEvent(key, 1, "event-1", "transaction-1")
	projection = projector.Apply(first)
	if projection.Gap != nil || projection.LastSequence != 2 || len(projection.Events) != 2 {
		t.Fatalf("drained projection = %#v", projection)
	}
	if duplicate := projector.Apply(second); duplicate.LastSequence != 2 || len(duplicate.Events) != 2 {
		t.Fatalf("duplicate changed projection = %#v", duplicate)
	}

	snapshot := WorkflowAuthoritativeSnapshot{WorkflowKey: key, Revision: edit.Revision{ResourceVersion: "rv-4", SemanticHash: "semantic-4", LayoutHash: "layout-4"}, Graph: visual.Graph{}, Sequence: 4, LoadedAt: time.Now().UTC()}
	projection = projector.Resync(snapshot)
	if projection.Gap != nil || projection.SnapshotResyncRequired || projection.LastSequence != 4 || projection.Committed == nil || projection.Committed.Revision != snapshot.Revision {
		t.Fatalf("resynced projection = %#v", projection)
	}
	stale := snapshot
	stale.Sequence, stale.Revision.ResourceVersion = 3, "rv-stale"
	if projection = projector.Resync(stale); projection.LastSequence != 4 || projection.Committed.Revision.ResourceVersion != "rv-4" {
		t.Fatalf("stale snapshot regressed projection = %#v", projection)
	}
	other := snapshot
	other.WorkflowKey = edit.WorkflowKey{TenantID: "tenant-b", WorkflowID: "workflow-1"}
	other.Sequence = 1
	projection = projector.Resync(other)
	if len(projection.Events) != 0 || projection.LastSequence != 1 || projection.WorkflowKey == nil || *projection.WorkflowKey != other.WorkflowKey {
		t.Fatalf("cross-workflow resync leaked state = %#v", projection)
	}
}

func TestWorkflowEventProjectorProjectsCanonicalAppSnapshotWithoutMutatingInput(t *testing.T) {
	projector := NewWorkflowEventProjector()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	projector.Apply(workflowProjectorEvent(key, 1, "event-1", "transaction-1"))
	original := StateSnapshot{Kind: "state", Config: map[string]any{"preserved": true}}
	projected := projector.ProjectAppSnapshot(original)
	if _, exists := original.Config[WorkflowWorkspaceSnapshotKey]; exists {
		t.Fatal("input snapshot was mutated")
	}
	encoded, err := json.Marshal(projected.Config[WorkflowWorkspaceSnapshotKey])
	if err != nil || !json.Valid(encoded) || !containsWorkflowProjection(encoded, `"lastSequence":1`) {
		t.Fatalf("projected config = %s, %v", encoded, err)
	}
}

func TestWorkflowEventProjectorCarriesAuthoritativeGateReceiptAndHistoryFacts(t *testing.T) {
	projector := NewWorkflowEventProjector()
	key := edit.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-1"}
	basis := edit.ValidationBasis{
		ResourceVersion: "rv-1", SemanticHash: "semantic-1", CatalogVersion: "catalog-1",
		ActionSchemaDigest: "actions-1", ReferenceSnapshotHash: "references-1", EnvironmentBindingVersion: "environment-1",
	}
	snapshot := WorkflowAuthoritativeSnapshot{
		WorkflowKey: key, Revision: edit.Revision{ResourceVersion: "rv-1", SemanticHash: "semantic-1", LayoutHash: "layout-1"},
		Graph: visual.Graph{}, Sequence: 1, LoadedAt: time.Now().UTC(),
		RunGateFacts: &service.WorkflowRunGateProjection{
			CurrentBasis: basis,
			Environment:  service.WorkflowEnvironmentProjection{Name: "staging", BindingVersion: "environment-1"},
			PostCheck: &service.WorkflowPostCheckProjection{
				Basis: basis, Mode: service.WorkflowDryRunModeDraftNonProduction, Status: service.WorkflowDryRunStatusPassed,
				CheckedAt: time.Now().UTC(), Diagnostics: []edit.WorkflowDiagnostic{{Code: "WARN", Severity: "warning", Message: "检查提醒"}},
			},
		},
		Receipts: []service.WorkflowReceiptProjection{{
			TransactionID: "transaction-1", ChangeSetID: "change-1", WorkflowKey: key, Status: "completed",
			RevisionBefore: edit.Revision{ResourceVersion: "rv-0"}, RevisionAfter: &edit.Revision{ResourceVersion: "rv-1", SemanticHash: "semantic-1", LayoutHash: "layout-1"},
			Validation: edit.ValidationResult{Kind: "validation", Basis: basis, Valid: true, Diagnostics: []edit.WorkflowDiagnostic{}}, ReceivedAt: time.Now().UTC(),
		}},
		ValidationArtifacts: []service.WorkflowValidationArtifactProjection{{ArtifactID: "validation-1", Basis: basis, Status: "valid", Diagnostics: []edit.WorkflowDiagnostic{}, IssuedAt: time.Now().UTC()}},
		DurableHistory:      []service.WorkflowDurableHistoryProjection{{Source: "durable_transaction", TransactionID: "transaction-1", BusinessLabel: "更新工作流", Status: "completed", OccurredAt: time.Now().UTC(), TouchedObjectCount: 1, Reversible: true}},
	}
	projection := projector.Resync(snapshot)
	if projection.RunGateFacts == nil || projection.RunGateFacts.CurrentBasis != basis || len(projection.Receipts) != 1 || len(projection.ValidationArtifacts) != 1 || len(projection.DurableHistory) != 1 {
		t.Fatalf("authoritative facts were not projected: %#v", projection)
	}

	// Caller mutation after Resync must not mutate the canonical AppSnapshot.
	snapshot.RunGateFacts.PostCheck.Diagnostics[0].Code = "MUTATED"
	snapshot.Receipts[0].Status = "rejected"
	cloned := projector.ProjectAppSnapshot(StateSnapshot{Kind: "state", Config: map[string]any{}}).Config[WorkflowWorkspaceSnapshotKey].(WorkflowEventProjection)
	if cloned.RunGateFacts.PostCheck.Diagnostics[0].Code != "WARN" || cloned.Receipts[0].Status != "completed" {
		t.Fatalf("projection retained mutable caller slices: %#v", cloned)
	}
	encoded, err := json.Marshal(cloned)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), `"diagnostics":null`) {
		t.Fatalf("projected empty diagnostics must stay JSON arrays: %s", encoded)
	}
}

func workflowProjectorEvent(key edit.WorkflowKey, sequence uint64, eventID, transactionID string) edit.WorkflowDomainEvent {
	return edit.WorkflowDomainEvent{SchemaVersion: edit.SchemaVersion, EventID: eventID, Sequence: sequence, Type: "transaction.applied", Key: key, Revision: edit.Revision{ResourceVersion: "rv", SemanticHash: "semantic", LayoutHash: "layout"}, Actor: "system", TransactionID: transactionID, Payload: json.RawMessage(`{}`), OccurredAt: time.Date(2026, 7, 21, 0, 0, 0, 0, time.UTC)}
}

func containsWorkflowProjection(raw []byte, part string) bool {
	return json.Valid(raw) && strings.Contains(string(raw), part)
}
