package workfloweditor

import (
	"context"
	"encoding/json"
	"testing"

	"aiops-v2/internal/tooling"
	edit "aiops-v2/internal/workfloweditor"
	"runner/workflow"
	"runner/workflow/visual"
)

func TestWorkflowEditorToolsExposeReadProposalAndConfirmedMutationBoundaries(t *testing.T) {
	tools := Tools(edit.NewService(nil))
	names := workflowEditorToolNames(tools)
	for _, want := range []string{
		"workflow.get_snapshot",
		"workflow.propose_edit_plan",
		"workflow.validate_patch",
		"workflow.preview_patch",
		"workflow.apply_patch",
		"workflow.undo_last_ai_patch",
		"workflow.propose_create_plan",
		"workflow.create_draft_from_confirmed_plan",
	} {
		if !workflowEditorContains(names, want) {
			t.Fatalf("tools = %v, missing %s", names, want)
		}
	}
	for _, toolDef := range tools {
		meta := toolDef.Metadata()
		if meta.Pack != ToolPack || meta.MCPInfo.ServerID != ServerID || !meta.IsMCP {
			t.Fatalf("tool metadata = %#v, want runner MCP workflow_editor tool", meta)
		}
	}
}

func TestWorkflowApplyPatchToolRequiresConfirmationBaseRevisionAndDrawerSession(t *testing.T) {
	toolDef := workflowEditorToolByName(t, Tools(edit.NewService(nil)), "workflow.apply_patch")
	decision := toolDef.CheckPermissions(context.Background(), json.RawMessage(`{"workflowId":"workflow"}`))
	if decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("decision = %#v, want deny without confirmation context", decision)
	}
	decision = toolDef.CheckPermissions(context.Background(), json.RawMessage(`{"workflowId":"workflow","baseRevision":"rev","userConfirmationId":"confirm","drawerSessionId":"drawer"}`))
	if decision.Action != tooling.PermissionActionNeedApproval {
		t.Fatalf("decision = %#v, want approval for confirmed mutation", decision)
	}
}

func TestWorkflowUndoToolRejectsRevisionConflict(t *testing.T) {
	service, store, record := newWorkflowEditorIntegrationService()
	session, _ := service.CreateSession(context.Background(), edit.CreateSessionRequest{DrawerSessionID: "drawer", WorkflowID: record.ID, BaseRevision: record.Revision})
	patch := edit.WorkflowPatch{ID: "patch", BaseRevision: record.Revision, Operations: []edit.WorkflowPatchOperation{{
		Op:     edit.PatchUpdateWorkflowMetadata,
		Fields: map[string]any{"title": "new"},
	}}}
	_, err := service.ApplyPatch(context.Background(), edit.ApplyPatchRequest{
		WorkflowID:         record.ID,
		BaseRevision:       record.Revision,
		PatchID:            patch.ID,
		Patch:              patch,
		UserConfirmationID: "confirm",
		DrawerSessionID:    session.ID,
		Reason:             "metadata",
	})
	if err != nil {
		t.Fatalf("ApplyPatch() error = %v", err)
	}
	graph := workflowEditorIntegrationGraph()
	graph.UI = map[string]any{"manual": true}
	store.PutWorkflow(edit.WorkflowRecord{ID: record.ID, Graph: graph})
	toolDef := workflowEditorToolByName(t, Tools(service), "workflow.undo_last_ai_patch")
	_, err = toolDef.Execute(context.Background(), json.RawMessage(`{"workflowId":"redis-memory","drawerSessionId":"drawer"}`))
	if err == nil {
		t.Fatal("undo tool should reject revision conflict")
	}
}

func TestWorkflowEditorToolsDoNotExposePublishExecuteOrHostMutation(t *testing.T) {
	names := workflowEditorToolNames(Tools(edit.NewService(nil)))
	for _, forbidden := range []string{"workflow.publish", "workflow.execute", "host.exec_command", "host.mutate"} {
		if workflowEditorContains(names, forbidden) {
			t.Fatalf("tools = %v, should not expose %s", names, forbidden)
		}
	}
}

func TestWorkflowEditorDescribeAndEffectToolsAreReadOnly(t *testing.T) {
	for _, name := range []string{"workflow.describe", "workflow.detect_patch_effect"} {
		toolDef := workflowEditorToolByName(t, Tools(edit.NewService(nil)), name)
		if !toolDef.IsReadOnly(nil) || toolDef.IsDestructive(nil) {
			t.Fatalf("%s readOnly/destructive = %v/%v, want read-only", name, toolDef.IsReadOnly(nil), toolDef.IsDestructive(nil))
		}
	}
}

func workflowEditorToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, toolDef := range tools {
		names = append(names, toolDef.Metadata().Name)
	}
	return names
}

func workflowEditorToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, toolDef := range tools {
		if toolDef.Metadata().Name == name {
			return toolDef
		}
	}
	t.Fatalf("tool %s not found in %v", name, workflowEditorToolNames(tools))
	return nil
}

func workflowEditorContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func newWorkflowEditorIntegrationService() (*edit.Service, *edit.MemoryWorkflowStore, edit.WorkflowRecord) {
	store := edit.NewMemoryWorkflowStore()
	record := store.PutWorkflow(edit.WorkflowRecord{ID: "redis-memory", Graph: workflowEditorIntegrationGraph()})
	return edit.NewService(store), store, record
}

func workflowEditorIntegrationGraph() visual.Graph {
	step := workflow.Step{ID: "collect", Name: "collect", Targets: []string{"local"}, Action: "script.python", Args: map[string]any{"script": "print('ok')"}}
	return visual.Graph{
		Version: visual.GraphVersion,
		Workflow: workflow.Workflow{
			Version:     "v0.1",
			Name:        "redis-memory",
			Description: "Redis memory workflow",
			Steps:       []workflow.Step{step},
		},
		Nodes: []visual.Node{
			{ID: "start", Type: visual.NodeTypeStart, Label: "Start"},
			{ID: "collect", Type: visual.NodeTypeAction, Label: "Collect", StepID: "collect", Step: &step},
			{ID: "end", Type: visual.NodeTypeEnd, Label: "End"},
		},
		Edges: []visual.Edge{
			{ID: "start-collect", Source: "start", Target: "collect", Kind: visual.EdgeKindNext},
			{ID: "collect-end", Source: "collect", Target: "end", Kind: visual.EdgeKindNext},
		},
	}
}
