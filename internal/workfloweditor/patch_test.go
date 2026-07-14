package workfloweditor

import (
	"strings"
	"testing"

	"runner/workflow/visual"
)

func TestPatchRejectsFullGraphReplacementByDefault(t *testing.T) {
	validation := ValidateWorkflowPatch(ValidatePatchRequest{
		BaseRevision: "rev",
		Patch: WorkflowPatch{
			ID:           "patch",
			BaseRevision: "rev",
			Operations: []WorkflowPatchOperation{{
				Op:    PatchReplaceFullGraph,
				Graph: &visual.Graph{Version: visual.GraphVersion},
			}},
		},
	})
	if validation.Valid {
		t.Fatalf("validation = %#v, want replace_full_graph rejected", validation)
	}
	if !strings.Contains(strings.Join(validation.Errors, "\n"), "second confirmation") {
		t.Fatalf("errors = %#v, want second confirmation error", validation.Errors)
	}
}

func TestApplyPatchRequiresBaseRevisionAndUserConfirmation(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	_, err := service.ApplyPatch(testContext(), ApplyPatchRequest{
		WorkflowID: record.ID,
		PatchID:    "patch",
		Patch: WorkflowPatch{ID: "patch", Operations: []WorkflowPatchOperation{{
			Op:     PatchUpdateWorkflowMetadata,
			Fields: map[string]any{"title": "new"},
		}}},
		DrawerSessionID: "drawer",
		Reason:          "test",
	})
	if err == nil || !strings.Contains(err.Error(), "base_revision") {
		t.Fatalf("ApplyPatch() error = %v, want base_revision requirement", err)
	}
	_, err = service.ApplyPatch(testContext(), ApplyPatchRequest{
		WorkflowID:   record.ID,
		BaseRevision: record.Revision,
		PatchID:      "patch",
		Patch: WorkflowPatch{ID: "patch", BaseRevision: record.Revision, Operations: []WorkflowPatchOperation{{
			Op:     PatchUpdateWorkflowMetadata,
			Fields: map[string]any{"title": "new"},
		}}},
		DrawerSessionID: "drawer",
		Reason:          "test",
	})
	if err == nil || !strings.Contains(err.Error(), "user_confirmation") {
		t.Fatalf("ApplyPatch() error = %v, want user_confirmation requirement", err)
	}
}

func TestApplyPatchReturnsNoEffectForUnchangedFields(t *testing.T) {
	graph := workflowEditorTestGraph()
	patch := WorkflowPatch{ID: "patch", BaseRevision: "rev", Operations: []WorkflowPatchOperation{{
		Op:     PatchUpdateNode,
		NodeID: "collect",
		Fields: map[string]any{"label": "Collect metrics"},
	}}}
	effect := DetectWorkflowPatchEffect(graph, patch, nil)
	if effect.Status != EffectNoEffect {
		t.Fatalf("effect = %#v, want no_effect", effect)
	}
}

func TestApplyPatchReturnsDuplicateForRepeatedPatchID(t *testing.T) {
	graph := workflowEditorTestGraph()
	patch := WorkflowPatch{ID: "patch", BaseRevision: "rev", Operations: []WorkflowPatchOperation{{
		Op:     PatchUpdateWorkflowMetadata,
		Fields: map[string]any{"title": "new"},
	}}}
	effect := DetectWorkflowPatchEffect(graph, patch, map[string]bool{"patch": true})
	if effect.Status != EffectDuplicate {
		t.Fatalf("effect = %#v, want duplicate", effect)
	}
}

func TestApplyPatchReturnsMetadataOnlyForUIMetadataOnlyChange(t *testing.T) {
	graph := workflowEditorTestGraph()
	patch := WorkflowPatch{ID: "patch", BaseRevision: "rev", Operations: []WorkflowPatchOperation{{
		Op:     PatchUpdateWorkflowMetadata,
		Fields: map[string]any{"drawer_note": "reviewed"},
	}}}
	effect := DetectWorkflowPatchEffect(graph, patch, nil)
	if effect.Status != EffectMetadataOnly {
		t.Fatalf("effect = %#v, want metadata_only", effect)
	}
}

func TestDeleteNodeReportsDownstreamVariablesAndEdges(t *testing.T) {
	graph := workflowEditorTestGraph()
	patch := WorkflowPatch{ID: "patch", BaseRevision: "rev", Operations: []WorkflowPatchOperation{{
		Op:     PatchDeleteNode,
		NodeID: "collect",
	}}}
	_, effect, err := ApplyPatchToGraph(graph, patch)
	if err != nil {
		t.Fatalf("ApplyPatchToGraph() error = %v", err)
	}
	if !containsString(effect.AffectedVariables, "memory_usage") {
		t.Fatalf("affected variables = %#v, want memory_usage", effect.AffectedVariables)
	}
	if !containsString(effect.AffectedEdges, "start-collect") || !containsString(effect.AffectedEdges, "collect-end") {
		t.Fatalf("affected edges = %#v, want connected edges", effect.AffectedEdges)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
