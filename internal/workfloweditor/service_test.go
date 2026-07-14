package workfloweditor

import (
	"context"
	"strings"
	"testing"

	"aiops-v2/internal/workflowgen"
	"runner/workflow/visual"
)

func testContext() context.Context {
	return context.Background()
}

func TestServiceGetSnapshotIncludesRevisionDigestValidationManualBindingAndDescribe(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	snapshot, err := service.GetSnapshot(testContext(), GetSnapshotRequest{WorkflowID: record.ID})
	if err != nil {
		t.Fatalf("GetSnapshot() error = %v", err)
	}
	if snapshot.RevisionDigest == "" || snapshot.Revision != record.Revision || !snapshot.Validation.Valid {
		t.Fatalf("snapshot = %#v, want revision digest and valid validation", snapshot)
	}
	if snapshot.Describe.NodeCount != 3 || snapshot.Describe.EdgeCount != 2 {
		t.Fatalf("describe = %#v, want graph counts", snapshot.Describe)
	}
}

func TestServiceProposeEditPlanDoesNotMutateWorkflow(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	before, _ := service.GetSnapshot(testContext(), GetSnapshotRequest{WorkflowID: record.ID})
	plan, err := service.ProposeEditPlan(testContext(), ProposeEditPlanRequest{WorkflowID: record.ID, Message: "添加 verify"})
	if err != nil {
		t.Fatalf("ProposeEditPlan() error = %v", err)
	}
	after, _ := service.GetSnapshot(testContext(), GetSnapshotRequest{WorkflowID: record.ID})
	if plan.ID == "" || len(plan.Items) == 0 {
		t.Fatalf("plan = %#v, want plan items", plan)
	}
	if before.RevisionDigest != after.RevisionDigest {
		t.Fatalf("workflow mutated by plan: before %s after %s", before.RevisionDigest, after.RevisionDigest)
	}
}

func TestServiceProposeEditPlanUsesConfiguredPlannerOutput(t *testing.T) {
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "redis-memory", Graph: workflowEditorTestGraph()})
	service := NewService(store, WithEditPlanner(staticWorkflowEditPlanner{plan: WorkflowEditPlan{
		Items: []WorkflowEditPlanItem{
			{ID: "model-1", Title: "识别 pgBackRest 备份对象", Description: "LLM 生成：确认实例、stanza 和 repo。", Status: "pending"},
			{ID: "model-2", Title: "生成备份执行节点", Description: "LLM 生成：根据用户要求创建受控备份步骤。", Status: "pending"},
		},
	}}))
	plan, err := service.ProposeEditPlan(testContext(), ProposeEditPlanRequest{
		WorkflowID: record.ID,
		Message:    "创建一个pg备份的工作流,使用工具pgbackrest",
	})
	if err != nil {
		t.Fatalf("ProposeEditPlan() error = %v", err)
	}
	if len(plan.Items) != 2 {
		t.Fatalf("plan items = %#v, want configured planner items", plan.Items)
	}
	joined := strings.ToLower(planText(plan))
	for _, want := range []string{"pgbackrest", "llm 生成", "受控备份"} {
		if !strings.Contains(joined, strings.ToLower(want)) {
			t.Fatalf("plan text = %q, want %q", joined, want)
		}
	}
	for _, item := range plan.Items {
		if item.Title == plan.Message {
			t.Fatalf("plan item title must not be raw user prompt: %#v", item)
		}
		if strings.Contains(item.Description, "Review and apply one workflow patch at a time") ||
			strings.Contains(item.Description, "生成一个最小 Workflow patch") {
			t.Fatalf("plan item still uses dead-rule fallback: %#v", item)
		}
	}
}

func TestServiceProposeEditPlanFailsWithoutConfiguredPlanner(t *testing.T) {
	store := NewMemoryWorkflowStore()
	record := store.PutWorkflow(WorkflowRecord{ID: "redis-memory", Graph: workflowEditorTestGraph()})
	service := NewService(store)
	_, err := service.ProposeEditPlan(testContext(), ProposeEditPlanRequest{
		WorkflowID: record.ID,
		Message:    "创建一个pg备份的工作流,使用工具pgbackrest",
	})
	if err == nil || !strings.Contains(err.Error(), "workflow edit planner is not configured") {
		t.Fatalf("ProposeEditPlan() error = %v, want missing planner configuration", err)
	}
}

func TestServicePreviewPatchDoesNotChangeDraft(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	patch := WorkflowPatch{ID: "patch-preview", BaseRevision: record.Revision, Operations: []WorkflowPatchOperation{{
		Op: PatchAddNode,
		Node: &visual.Node{
			ID:    "verify",
			Type:  visual.NodeTypeAction,
			Label: "Verify",
		},
	}}}
	preview, err := service.PreviewPatch(testContext(), PreviewPatchRequest{WorkflowID: record.ID, Patch: patch})
	if err != nil {
		t.Fatalf("PreviewPatch() error = %v", err)
	}
	if preview.Effect.Status != EffectChanged {
		t.Fatalf("preview effect = %#v, want changed", preview.Effect)
	}
	snapshot, _ := service.GetSnapshot(testContext(), GetSnapshotRequest{WorkflowID: record.ID})
	if snapshot.Describe.NodeCount != 3 {
		t.Fatalf("snapshot describe = %#v, preview must not mutate draft", snapshot.Describe)
	}
}

func TestServiceApplyPatchCreatesUndoCheckpointAffectedNodesDescribeAndAudit(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	session, _ := service.CreateSession(testContext(), CreateSessionRequest{DrawerSessionID: "drawer", WorkflowID: record.ID, BaseRevision: record.Revision})
	patch := WorkflowPatch{ID: "patch-apply", BaseRevision: record.Revision, Operations: []WorkflowPatchOperation{{
		Op:     PatchUpdateNode,
		NodeID: "collect",
		Fields: map[string]any{"label": "Collect Redis memory"},
	}}}
	result, err := service.ApplyPatch(testContext(), ApplyPatchRequest{
		WorkflowID:         record.ID,
		BaseRevision:       record.Revision,
		PatchID:            patch.ID,
		Patch:              patch,
		UserConfirmationID: "confirm",
		DrawerSessionID:    session.ID,
		Reason:             "rename node",
	})
	if err != nil {
		t.Fatalf("ApplyPatch() error = %v", err)
	}
	if result.RevisionAfter == result.RevisionBefore || result.UndoCheckpoint.ID == "" {
		t.Fatalf("result = %#v, want revision change and undo checkpoint", result)
	}
	if !containsString(result.Effect.AffectedNodes, "collect") || result.Describe.NodeCount != 3 || len(result.Audit) == 0 {
		t.Fatalf("result = %#v, want affected nodes, describe and audit", result)
	}
}

func TestServiceApplyPatchRejectsStaleRevision(t *testing.T) {
	service, _, record := newWorkflowEditorTestService()
	patch := WorkflowPatch{ID: "patch-stale", BaseRevision: "stale", Operations: []WorkflowPatchOperation{{
		Op:     PatchUpdateWorkflowMetadata,
		Fields: map[string]any{"title": "new"},
	}}}
	_, err := service.ApplyPatch(testContext(), ApplyPatchRequest{
		WorkflowID:         record.ID,
		BaseRevision:       "stale",
		PatchID:            patch.ID,
		Patch:              patch,
		UserConfirmationID: "confirm",
		DrawerSessionID:    "drawer",
		Reason:             "test",
	})
	if err == nil || !strings.Contains(err.Error(), "stale revision") {
		t.Fatalf("ApplyPatch() error = %v, want stale revision", err)
	}
}

func TestServiceUndoLastAIPatchRejectsManualInterleaving(t *testing.T) {
	service, store, record := newWorkflowEditorTestService()
	session, _ := service.CreateSession(testContext(), CreateSessionRequest{DrawerSessionID: "drawer", WorkflowID: record.ID, BaseRevision: record.Revision})
	patch := WorkflowPatch{ID: "patch-undo", BaseRevision: record.Revision, Operations: []WorkflowPatchOperation{{
		Op:     PatchUpdateWorkflowMetadata,
		Fields: map[string]any{"title": "new"},
	}}}
	result, err := service.ApplyPatch(testContext(), ApplyPatchRequest{
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
	manual := workflowEditorTestGraph()
	manual.UI["manual_edit"] = true
	store.PutWorkflow(WorkflowRecord{ID: record.ID, Graph: manual})
	_, err = service.UndoLastAIPatch(testContext(), UndoLastAIPatchRequest{WorkflowID: record.ID, DrawerSessionID: session.ID})
	if err == nil || !strings.Contains(err.Error(), "manual interleaving") {
		t.Fatalf("UndoLastAIPatch() error = %v, want manual interleaving rejection after %s", err, result.RevisionAfter)
	}
}

func TestServiceCreateDraftFromRequirementRequiresConfirmedPlan(t *testing.T) {
	service, _, _ := newWorkflowEditorTestService()
	_, err := service.CreateDraftFromConfirmedPlan(testContext(), WorkflowDraftFromPlanRequest{
		Plan: workflowEditorCreateDraftPlan(),
	})
	if err == nil || !strings.Contains(err.Error(), "user_confirmation_id") {
		t.Fatalf("CreateDraftFromConfirmedPlan() error = %v, want confirmation requirement", err)
	}
	_, err = service.CreateDraftFromConfirmedPlan(testContext(), WorkflowDraftFromPlanRequest{
		UserConfirmationID: "confirm",
	})
	if err == nil || !strings.Contains(err.Error(), "confirmed workflow generation plan") {
		t.Fatalf("CreateDraftFromConfirmedPlan() error = %v, want confirmed plan requirement", err)
	}
}

func TestServiceCreateDraftFromConfirmedPlanReturnsPreviewValidationAndRevision(t *testing.T) {
	service, _, _ := newWorkflowEditorTestService()
	result, err := service.CreateDraftFromConfirmedPlan(testContext(), WorkflowDraftFromPlanRequest{
		SessionID:          "create-session",
		DrawerSessionID:    "drawer-create",
		UserConfirmationID: "confirm-create",
		Plan:               workflowEditorCreateDraftPlan(),
	})
	if err != nil {
		t.Fatalf("CreateDraftFromConfirmedPlan() error = %v", err)
	}
	if result.WorkflowID == "" || result.Revision == "" || !result.Validation.Valid || result.Describe.NodeCount == 0 {
		t.Fatalf("result = %#v, want workflow id, revision, validation and describe", result)
	}
	snapshot, err := service.GetSnapshot(testContext(), GetSnapshotRequest{WorkflowID: result.WorkflowID})
	if err != nil {
		t.Fatalf("GetSnapshot(created) error = %v", err)
	}
	if snapshot.Revision != result.Revision {
		t.Fatalf("snapshot revision = %q, want %q", snapshot.Revision, result.Revision)
	}
}

func TestServiceCreateDraftDoesNotPublishOrExecute(t *testing.T) {
	service, _, _ := newWorkflowEditorTestService()
	result, err := service.CreateDraftFromConfirmedPlan(testContext(), WorkflowDraftFromPlanRequest{
		UserConfirmationID: "confirm-create",
		Plan:               workflowEditorCreateDraftPlan(),
	})
	if err != nil {
		t.Fatalf("CreateDraftFromConfirmedPlan() error = %v", err)
	}
	if result.Published || result.Executed {
		t.Fatalf("result = %#v, draft creation must not publish or execute", result)
	}
}

func workflowEditorCreateDraftPlan() workflowgen.WorkflowGenerationPlan {
	return workflowgen.WorkflowGenerationPlan{
		Version: 1,
		Title:   "Redis Memory Draft",
		Trigger: workflowgen.WorkflowTrigger{Type: workflowgen.TriggerTypeManual},
		Nodes: []workflowgen.WorkflowPlanNode{{
			ID:     "collect",
			Kind:   workflowgen.NodeKindSearch,
			Title:  "Collect Redis memory",
			Action: "script.python",
		}},
		Outputs: []workflowgen.WorkflowOutput{{
			ID:     "summary",
			Target: workflowgen.OutputTargetReturn,
		}},
		ValidationStrategy: workflowgen.ValidationStrategy{Enabled: false, Provider: workflowgen.ValidationProviderNone},
	}
}

func planText(plan WorkflowEditPlan) string {
	var builder strings.Builder
	builder.WriteString(plan.Message)
	for _, item := range plan.Items {
		builder.WriteString(" ")
		builder.WriteString(item.Title)
		builder.WriteString(" ")
		builder.WriteString(item.Description)
	}
	return builder.String()
}
