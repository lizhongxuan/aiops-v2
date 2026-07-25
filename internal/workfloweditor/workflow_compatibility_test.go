package workfloweditor

import (
	"errors"
	"strings"
	"testing"
)

func TestWorkflowCompatibilityNoMemoryFallback(t *testing.T) {
	service := NewService(nil)
	if _, ok := service.store.(*MemoryWorkflowStore); ok {
		t.Fatal("NewService(nil) constructed a production memory store")
	}
	if _, err := service.GetSnapshot(testContext(), GetSnapshotRequest{WorkflowID: "workflow"}); !errors.Is(err, ErrCompatibilityStoreRequired) {
		t.Fatalf("GetSnapshot() error = %v, want unconfigured compatibility store", err)
	}
	if _, err := service.CreateSession(testContext(), CreateSessionRequest{WorkflowID: "workflow"}); !errors.Is(err, ErrCompatibilityStoreRequired) {
		t.Fatalf("CreateSession() error = %v, want unconfigured compatibility store", err)
	}
	if _, err := service.ApplyPatch(testContext(), ApplyPatchRequest{WorkflowID: "workflow"}); !errors.Is(err, ErrCompatibilityStoreRequired) {
		t.Fatalf("ApplyPatch() error = %v, want unconfigured compatibility store", err)
	}
}

func TestWorkflowCompatibilityRejectsArbitraryGraphUIFields(t *testing.T) {
	patch := WorkflowPatch{ID: "legacy", BaseRevision: "r17", Operations: []WorkflowPatchOperation{{
		Op: PatchUpdateNode, NodeID: "collect", Fields: map[string]any{"ui.secret": "must-not-be-written"},
	}}}
	validation := ValidateWorkflowPatch(ValidatePatchRequest{BaseRevision: "r17", Patch: patch})
	if validation.Valid || !strings.Contains(strings.Join(validation.Errors, "\n"), "not part of the compatibility contract") {
		t.Fatalf("validation = %+v, want strict field rejection", validation)
	}
	graph := workflowEditorTestGraph()
	if _, _, err := ApplyPatchToGraph(graph, patch); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("ApplyPatchToGraph() error = %v, want fail-closed unknown field", err)
	}
	if _, found := graph.Nodes[1].UI["secret"]; found {
		t.Fatal("legacy arbitrary field mutated Graph.UI")
	}
}

func TestWorkflowCompatibilityRejectsRetiredMetadataAndFullGraphOperations(t *testing.T) {
	replacement := workflowEditorTestGraph()
	for _, operation := range []WorkflowPatchOperation{
		{Op: PatchUpdateWorkflowMetadata, Fields: map[string]any{"title": "forged"}},
		{Op: PatchReplaceFullGraph, Graph: &replacement},
	} {
		patch := WorkflowPatch{ID: "legacy", BaseRevision: "r17", Operations: []WorkflowPatchOperation{operation}}
		validation := ValidateWorkflowPatch(ValidatePatchRequest{BaseRevision: "r17", Patch: patch, AllowFullGraphReplacement: true, SecondConfirmationID: "confirm"})
		if validation.Valid {
			t.Fatalf("operation %q remains writable through legacy compatibility", operation.Op)
		}
	}
}
