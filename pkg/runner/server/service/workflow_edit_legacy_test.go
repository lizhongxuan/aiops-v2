package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"runner/workflow/edit"
	"runner/workflow/visual"
)

func TestWorkflowLegacyApplyBatchesOperationsIntoOneIdempotentTransaction(t *testing.T) {
	fixture := newWorkflowEditApplyFixture(t)
	label := "Check health"
	position := visual.Position{X: 360, Y: 210}
	request := WorkflowLegacyPatchRequest{
		SchemaVersion: edit.SchemaVersion, WorkflowKey: fixture.request.WorkflowKey,
		ExpectedRevision: fixture.request.ExpectedRevision, IdempotencyKey: "legacy-batch-once",
		ActorID: "legacy-user", CanvasSessionID: "legacy-canvas", SessionID: "legacy-session",
		Reason: "apply reviewed legacy patch", Confirmed: true,
		Operations: []edit.WorkflowOperation{
			edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "check", Set: edit.NodePatch{Label: &label}}),
			edit.NewOperation(&edit.UpdateNodeOperation{NodeID: "repair", Set: edit.NodePatch{Position: &position}}),
		},
	}

	result, err := fixture.app.ApplyLegacyPatch(context.Background(), request)
	if err != nil || result.Receipt.RevisionAfter == nil || result.Preview.Risk.OperationCount != 2 {
		t.Fatalf("ApplyLegacyPatch() = %#v, %v", result, err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if err != nil || snapshot.Generation != 2 || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("legacy batch snapshot = %#v, %v", snapshot, err)
	}
	graph, err := parseCommittedVisualGraph(snapshot.GraphPayload, snapshot.ResourceVersion)
	if err != nil || graph.Nodes[nodeIndexByID(graph.Nodes, "check")].Label != label || nodePosition(graph, "repair") != position {
		t.Fatalf("legacy batch graph = %#v, %v", graph, err)
	}

	replayed, err := fixture.app.ApplyLegacyPatch(context.Background(), request)
	if err != nil || replayed.Receipt.TransactionID != result.Receipt.TransactionID {
		t.Fatalf("legacy replay = %#v, %v", replayed, err)
	}
	afterReplay, _ := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	if afterReplay.Generation != 2 || len(afterReplay.ApprovalConsumptions) != 1 {
		t.Fatalf("legacy replay created another transaction: %#v", afterReplay)
	}
	auditPayload, err := json.Marshal(afterReplay.Audit)
	if err != nil || !strings.Contains(string(auditPayload), result.Receipt.TransactionID) || !strings.Contains(string(auditPayload), request.Reason) {
		t.Fatalf("legacy transaction audit = %s, %v", auditPayload, err)
	}

	secretRequest := request
	secretRequest.IdempotencyKey = "legacy-secret-rejected"
	secretRequest.Reason = "token=secret-sentinel-never-persist"
	if _, err := fixture.app.ApplyLegacyPatch(context.Background(), secretRequest); !errors.Is(err, ErrWorkflowEditSecret) {
		t.Fatalf("secret-bearing legacy reason error = %v", err)
	}
	afterSecret, _ := fixture.commits.Snapshot(context.Background(), request.WorkflowKey)
	persisted, _ := json.Marshal(afterSecret)
	if afterSecret.Generation != 2 || strings.Contains(string(persisted), "secret-sentinel-never-persist") {
		t.Fatalf("secret-bearing legacy request changed persistence: %s", persisted)
	}
}

func nodeIndexByID(nodes []visual.Node, id string) int {
	for index := range nodes {
		if nodes[index].ID == id {
			return index
		}
	}
	return -1
}
