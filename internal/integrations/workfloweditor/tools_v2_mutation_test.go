package workfloweditor

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	legacy "aiops-v2/internal/workfloweditor"
	"runner/server/service"
	"runner/workflow"
	contract "runner/workflow/edit"
	"runner/workflow/visual"
	"runner/workflowstore"
)

func TestWorkflowPreviewPatchToolMCP008CreatesCandidateWithoutCommittedMutation(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	before := backend.snapshot
	request := workflowPreviewRequest(before)
	tool := WorkflowPreviewPatchToolV2(app)
	result, err := tool.Execute(workflowMutationContext("tenant-a", "user-a", "session-a", "workflow-a", "call-preview-1"), mustWorkflowMutationJSON(t, request))
	if err != nil {
		t.Fatal(err)
	}
	if result.IsError || strings.HasPrefix(strings.TrimSpace(result.Content), "{") || strings.Contains(string(result.StructuredContent), `"status":"ok"`) {
		t.Fatalf("preview result = %#v", result)
	}
	artifact := assertWorkflowToolDisplayProjection(t, result, "workflow_patch_preview", "workflow.change.preview.v2", "preview")
	var envelope contract.ToolResult[contract.ChangeSetPreview]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	preview := envelope.StructuredContent
	if preview.Kind != "preview" || preview.ChangeSet.PreviewHash == "" || preview.Candidate.CurrentPreviewHash != preview.ChangeSet.PreviewHash || !preview.ChangeSet.Validation.Valid || len(preview.Diff) == 0 {
		t.Fatalf("preview = %#v", preview)
	}
	if preview.ChangeSet.Actor != contract.ActorKind("agent") {
		t.Fatalf("preview actor = %q, want canonical workflow actor agent", preview.ChangeSet.Actor)
	}
	assertWorkflowPreviewReadOnlyActions(t, artifact, preview)
	var detail workflowPreviewDetailV2
	if err := contract.DecodeStrict(preview.Diff, &detail); err != nil || detail.PreviewHash != preview.ChangeSet.PreviewHash || detail.Risk.OperationCount != 1 || detail.ValidationBasis.Validate() != nil {
		t.Fatalf("strict preview detail = %#v, %v", detail, err)
	}
	backend.mu.RLock()
	after := backend.snapshot
	backend.mu.RUnlock()
	if after.Revision != before.Revision || !bytes.Equal(mustWorkflowMutationJSON(t, after.Graph), mustWorkflowMutationJSON(t, before.Graph)) {
		t.Fatalf("preview mutated committed graph: before=%#v after=%#v", before.Revision, after.Revision)
	}
	if err := tooling.ValidateToolResult(tool.OutputSchema(), result); err != nil {
		t.Fatalf("output contract: %v\nresult=%s\nschema=%s", err, result.StructuredContent, tool.OutputSchema())
	}
	if tool.IsReadOnly(nil) || tool.IsDestructive(nil) || tool.IsConcurrencySafe(nil) {
		t.Fatalf("preview annotations = readOnly:%v destructive:%v concurrency:%v", tool.IsReadOnly(nil), tool.IsDestructive(nil), tool.IsConcurrencySafe(nil))
	}
}

func TestWorkflowPreviewPatchToolSeparatesCanvasAndRuntimeSessionIdentity(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	request := workflowPreviewRequest(backend.snapshot)
	request.ValidationBasis.EnvironmentBindingVersion = "env-v1"
	tool := WorkflowPreviewPatchToolV2(app)
	ctx := workflowMutationArtifactContext(t, backend.snapshot, appui.WorkflowTurnSelection{NodeIDs: []string{"step"}}, "call-preview-distinct-sessions")
	result, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, request))
	if err != nil || result.IsError {
		t.Fatalf("preview result = %#v, %v", result, err)
	}
	var envelope contract.ToolResult[contract.ChangeSetPreview]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	revision, err := app.GetRevision(context.Background(), backend.snapshot.Key, envelope.StructuredContent.Candidate.ID, envelope.StructuredContent.Candidate.CurrentRevision)
	if err != nil {
		t.Fatal(err)
	}
	if revision.CanvasSessionID != "canvas-a" || revision.AgentSessionID != "session-a" || revision.CanvasSessionID == revision.AgentSessionID {
		t.Fatalf("candidate sessions = canvas:%q runtime:%q", revision.CanvasSessionID, revision.AgentSessionID)
	}
}

func TestWorkflowPreviewArtifactActionsOnlyExposeExactReadOnlyCanvasTargets(t *testing.T) {
	key := contract.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
	base := contract.Revision{ResourceVersion: "revision-a", SemanticHash: "semantic-a", LayoutHash: "layout-a"}
	position := visual.Position{X: 1, Y: 2}
	actions := workflowPreviewArtifactActionsV2(service.WorkflowEditPreviewResult{
		PreviewHash: "preview-a",
		ChangeSet: contract.WorkflowChangeSet{
			ID: "change-a", Key: key, CandidateID: "candidate-a", CandidateRevision: 1, Base: base, PreviewHash: "preview-a",
			Operations: []contract.WorkflowOperation{
				contract.NewOperation(&contract.UpdateNodeOperation{NodeID: "node-update"}),
				contract.NewOperation(&contract.DeleteEdgeOperation{EdgeID: "edge-delete"}),
				contract.NewOperation(&contract.AddNodeOperation{ClientRef: "node-add"}),
				contract.NewOperation(&contract.AddEdgeOperation{Edge: contract.EditableEdge{ID: "edge-add"}}),
				contract.NewOperation(&contract.UpdateLayoutOperation{Layout: contract.LayoutPatch{Nodes: []contract.LayoutNodePatch{{NodeID: "node-layout", Position: position}}}}),
				contract.NewOperation(&contract.UpdateLayoutOperation{Layout: contract.LayoutPatch{Nodes: []contract.LayoutNodePatch{{NodeID: "node-a"}, {NodeID: "node-b"}}}}),
				contract.NewOperation(&contract.UpdateWorkflowOperation{}),
				contract.NewOperation(&contract.AttachManualBindingOperation{}),
				{Value: nil},
				contract.NewOperation(&contract.UpdateNodeOperation{NodeID: " node-forged "}),
			},
		},
		Candidate: contract.WorkflowCandidate{ID: "candidate-a", Key: key, CurrentChangeSetID: "change-a", CurrentRevision: 1, CurrentPreviewHash: "preview-a"},
	})
	if len(actions) != 6 {
		t.Fatalf("actions = %#v, want five exact focus actions and one review action", actions)
	}
	want := []struct {
		index  int
		nodeID string
		edgeID string
	}{{0, "node-update", ""}, {1, "", "edge-delete"}, {2, "node-add", ""}, {3, "", "edge-add"}, {4, "node-layout", ""}}
	for actionIndex, expected := range want {
		action := actions[actionIndex]
		if action.Intent != "workflow.focus_object" || action.Mutation || action.Target.OperationIndex == nil || *action.Target.OperationIndex != expected.index ||
			action.Target.NodeID != expected.nodeID || action.Target.EdgeID != expected.edgeID || action.Target.CandidateID != "candidate-a" ||
			action.Target.ChangeSetID != "change-a" || action.Target.PreviewHash != "preview-a" || action.Target.ExpectedRevision != "revision-a" {
			t.Fatalf("focus action[%d] = %#v, want %#v", actionIndex, action, expected)
		}
	}
	review := actions[len(actions)-1]
	if review.ID != "workflow-review-change" || review.Intent != "workflow.review_change" || review.Mutation || review.Target.OperationIndex != nil || review.Target.NodeID != "" || review.Target.EdgeID != "" {
		t.Fatalf("review action = %#v", review)
	}
}

func TestWorkflowPreviewPatchToolReturnsTypedDomainErrorAndRejectsUntrustedInput(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	request := workflowPreviewRequest(backend.snapshot)
	request.Expected.ResourceVersion = "stale-revision"
	request.ValidationBasis.ResourceVersion = "stale-revision"
	tool := WorkflowPreviewPatchToolV2(app)
	ctx := workflowMutationContext("tenant-a", "user-a", "session-a", "workflow-a", "call-preview-stale")
	result, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, request))
	if err != nil {
		t.Fatal(err)
	}
	var envelope contract.ToolResult[contract.DomainError]
	if json.Unmarshal(result.StructuredContent, &envelope) != nil || !result.IsError || !envelope.IsError || envelope.StructuredContent.Code != contract.ErrorStaleRevision {
		t.Fatalf("typed stale result = %#v / %s", result, result.StructuredContent)
	}
	artifact := assertWorkflowToolDisplayProjection(t, result, "workflow_conflict", "workflow.change.conflict.v2", "error")
	assertWorkflowConflictProjection(t, artifact, envelope.StructuredContent)
	assertWorkflowNonConflictErrorsHaveNoConflictDisplay(t, envelope.StructuredContent)
	valid := workflowPreviewRequest(backend.snapshot)
	if _, err := tool.Execute(workflowMutationContext("tenant-b", "user-a", "session-a", "workflow-a", "call-preview-cross"), mustWorkflowMutationJSON(t, valid)); err == nil || !strings.Contains(err.Error(), "trusted workflow scope") {
		t.Fatalf("cross-tenant error = %v", err)
	}
	raw := append(bytes.TrimSuffix(mustWorkflowMutationJSON(t, valid), []byte("}")), []byte(`,"unknown":true}`)...)
	if err := tool.ValidateInput(context.Background(), raw); err == nil {
		t.Fatal("strict preview input accepted unknown field")
	}
}

func TestWorkflowMutationToolsBindFrozenTurnRevisionSelectionAndEnvironment(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	ctx := workflowMutationArtifactContext(t, backend.snapshot, appui.WorkflowTurnSelection{NodeIDs: []string{"step"}, NeighborhoodIDs: []string{"start"}}, "call-preview-scope")
	request := workflowPreviewRequest(backend.snapshot)
	request.ValidationBasis.EnvironmentBindingVersion = "env-v1"
	tool := WorkflowPreviewPatchToolV2(app)
	if result, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, request)); err != nil || result.IsError {
		t.Fatalf("in-scope preview = %#v, %v", result, err)
	}

	outOfScope := request
	position := visual.Position{X: 1, Y: 2}
	outOfScope.Operations = []contract.WorkflowOperation{contract.NewOperation(&contract.UpdateNodeOperation{NodeID: "end", Set: contract.NodePatch{Position: &position}})}
	if _, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, outOfScope)); err == nil || !strings.Contains(err.Error(), "outside the frozen") {
		t.Fatalf("out-of-scope preview error = %v", err)
	}
	wrongEnvironment := request
	wrongEnvironment.ValidationBasis.EnvironmentBindingVersion = "other-env"
	if _, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, wrongEnvironment)); err == nil || !strings.Contains(err.Error(), "revision or environment") {
		t.Fatalf("environment scope error = %v", err)
	}
	wrongRevision := request
	wrongRevision.Expected.ResourceVersion = "other-revision"
	wrongRevision.ValidationBasis.ResourceVersion = "other-revision"
	if _, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, wrongRevision)); err == nil || !strings.Contains(err.Error(), "revision or environment") {
		t.Fatalf("revision scope error = %v", err)
	}

	validate := WorkflowValidateToolV2(app)
	validation, err := validate.Execute(ctx, mustWorkflowMutationJSON(t, contract.ValidateRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key}))
	if err != nil {
		t.Fatal(err)
	}
	assertWorkflowValidationResult(t, validate, validation)
}

func TestWorkflowOperationScopeAllowsClientRefCreateAndExplicitEntireWorkflow(t *testing.T) {
	frozen := appui.WorkflowTurnContext{Selection: appui.WorkflowTurnSelection{NodeIDs: []string{"step"}}}
	newType := visual.NodeTypeAction
	newPosition := visual.Position{X: 10, Y: 20}
	add := contract.NewOperation(&contract.AddNodeOperation{ClientRef: "new-step", Node: contract.EditableNode{NodePatch: contract.NodePatch{Type: &newType, Position: &newPosition}}})
	if err := workflowOperationsWithinTurnScopeV2(frozen, []contract.WorkflowOperation{add}); err != nil {
		t.Fatalf("clientRef create rejected: %v", err)
	}
	withoutRef := contract.NewOperation(&contract.AddNodeOperation{Node: contract.EditableNode{NodePatch: contract.NodePatch{Type: &newType, Position: &newPosition}}})
	if err := workflowOperationsWithinTurnScopeV2(frozen, []contract.WorkflowOperation{withoutRef}); err == nil {
		t.Fatal("scoped create without clientRef was accepted")
	}
	description := "entire workflow"
	global := contract.NewOperation(&contract.UpdateWorkflowOperation{Set: contract.WorkflowPatch{Description: &description}})
	if err := workflowOperationsWithinTurnScopeV2(frozen, []contract.WorkflowOperation{global}); err == nil {
		t.Fatal("selection-scoped workflow-wide update was accepted")
	}
	if err := workflowOperationsWithinTurnScopeV2(appui.WorkflowTurnContext{}, []contract.WorkflowOperation{global}); err != nil {
		t.Fatalf("explicit entire-workflow scope rejected: %v", err)
	}
}

func TestWorkflowApplyPatchToolRequiresExactRuntimeApprovalAndDeclaresSafety(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	preview, err := app.Preview(context.Background(), service.WorkflowEditPreviewRequest{PreviewPatchRequest: workflowPreviewRequest(backend.snapshot), CandidateID: "candidate-approval", Actor: contract.ActorKind("assistant"), ActorID: "user-a", CanvasSessionID: "session-a", AgentSessionID: "session-a", TurnID: "turn-a", ToolCallID: "call-preview-approval"})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.ApplyPatchRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key, ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash, ExpectedRevision: preview.ChangeSet.Base.ResourceVersion, IdempotencyKey: "idem-approval", CandidateGrantID: "grant-approval", ApplyApprovalID: "approval-runtime", Reason: "apply reviewed candidate"}
	raw := mustWorkflowMutationJSON(t, request)
	tool := WorkflowApplyPatchToolV2(app)
	ctx := workflowApplyContext(t, request, raw, nil)
	if decision := tool.CheckPermissions(ctx, raw); decision.Action != tooling.PermissionActionNeedApproval || decision.Approval == nil {
		t.Fatalf("unapproved decision = %#v", decision)
	}
	hash, _ := actionproposal.NormalizedInputHash(raw)
	verified := workflowVerifiedApprovalForTest(request.WorkflowKey, request.ApplyApprovalID, "turn-a", "call-apply", hash)
	approved := workflowApplyContext(t, request, raw, verified)
	if decision := tool.CheckPermissions(approved, raw); decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("approved decision = %#v", decision)
	}
	providerAlias := *verified
	providerAlias.ToolName = workflowApplyProviderAliasV2
	approvedAlias := workflowApplyContextWithToolName(request.WorkflowKey, "user-a", "session-a", "turn-a", "call-apply", workflowApplyProviderAliasV2, &providerAlias)
	if decision := tool.CheckPermissions(approvedAlias, raw); decision.Action != tooling.PermissionActionAllow {
		t.Fatalf("provider alias approval decision = %#v", decision)
	}
	forgedAlias := providerAlias
	forgedAlias.ToolName = "workflow-apply-patch"
	forgedAliasContext := workflowApplyContextWithToolName(request.WorkflowKey, "user-a", "session-a", "turn-a", "call-apply", "workflow-apply-patch", &forgedAlias)
	if decision := tool.CheckPermissions(forgedAliasContext, raw); decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("forged alias approval decision = %#v", decision)
	}
	drifted := *verified
	drifted.ArgumentsHash = "sha256:drifted"
	if decision := tool.CheckPermissions(workflowApplyContext(t, request, raw, &drifted), raw); decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("drifted approval decision = %#v", decision)
	}
	meta := tool.Metadata()
	if !meta.Mutating || !meta.RequiresApproval || meta.ResourceLockResolver == nil || meta.Idempotency.Strategy != tooling.ToolIdempotencyStrategyExplicitKey || len(meta.Idempotency.PostCheckRefs) == 0 || meta.Rollback == nil || !meta.Rollback.DeclarativelyReady() || tool.IsReadOnly(nil) || !tool.IsDestructive(nil) || tool.IsConcurrencySafe(nil) {
		t.Fatalf("apply safety metadata = %#v", meta)
	}
	locks, err := meta.ResourceLockResolver(raw, tooling.ToolExecutionContext{TenantID: "tenant-a", UserID: "user-a", SessionID: "session-a", Metadata: map[string]string{"workflowId": "workflow-a"}})
	if err != nil || len(locks) != 1 || locks[0].ResourceID != "tenant-a:workflow-a" || locks[0].OperationKind != "apply" {
		t.Fatalf("dynamic workflow lock = %#v, %v", locks, err)
	}
	if _, err := meta.ResourceLockResolver(raw, tooling.ToolExecutionContext{TenantID: "tenant-b", UserID: "user-a", SessionID: "session-a", Metadata: map[string]string{"workflowId": "workflow-a"}}); err == nil {
		t.Fatal("dynamic lock accepted cross-tenant workflow")
	}
}

func TestWorkflowCandidateValidationAndApplyRecheckFrozenOperationScope(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	position := visual.Position{X: 5, Y: 6}
	previewRequest := workflowPreviewRequest(backend.snapshot)
	previewRequest.Operations = []contract.WorkflowOperation{contract.NewOperation(&contract.UpdateNodeOperation{NodeID: "end", Set: contract.NodePatch{Position: &position}})}
	preview, err := app.Preview(context.Background(), service.WorkflowEditPreviewRequest{PreviewPatchRequest: previewRequest, CandidateID: "candidate-outside", Actor: contract.ActorKind("assistant"), ActorID: "user-a", CanvasSessionID: "session-a", AgentSessionID: "session-a", TurnID: "turn-a", ToolCallID: "call-preview-outside"})
	if err != nil {
		t.Fatal(err)
	}
	ctx := workflowMutationArtifactContext(t, backend.snapshot, appui.WorkflowTurnSelection{NodeIDs: []string{"step"}}, "call-scope-recheck")
	validate := WorkflowValidateToolV2(app)
	request := contract.ValidateRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key, ChangeSetID: preview.ChangeSet.ID}
	if _, err := validate.Execute(ctx, mustWorkflowMutationJSON(t, request)); err == nil || !strings.Contains(err.Error(), "outside the frozen") {
		t.Fatalf("candidate validate scope error = %v", err)
	}
	apply := WorkflowApplyPatchToolV2(app)
	applyRequest := contract.ApplyPatchRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key, ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash, ExpectedRevision: preview.ChangeSet.Base.ResourceVersion, IdempotencyKey: "idem-outside", CandidateGrantID: "grant-outside", ApplyApprovalID: "approval-outside", Reason: "should be denied"}
	if decision := apply.CheckPermissions(ctx, mustWorkflowMutationJSON(t, applyRequest)); decision.Action != tooling.PermissionActionDeny {
		t.Fatalf("candidate apply scope decision = %#v", decision)
	}
}

func TestWorkflowApplyPatchToolMCP009PersistsReceiptAndIsIdempotent(t *testing.T) {
	fixture := newWorkflowApplyToolFixture(t)
	tool := WorkflowApplyPatchToolV2(fixture.app)
	raw := mustWorkflowMutationJSON(t, fixture.request)
	hash, _ := actionproposal.NormalizedInputHash(raw)
	verified := workflowVerifiedApprovalForTest(fixture.request.WorkflowKey, "approval-runtime-checkpoint", "turn-apply", "call-apply", hash)
	verified.ToolName = workflowApplyProviderAliasV2
	ctx := workflowApplyContextWithToolName(fixture.request.WorkflowKey, "user-a", "session-a", "turn-apply", "call-apply", workflowApplyProviderAliasV2, verified)
	result, err := tool.Execute(ctx, raw)
	if err != nil || result.IsError {
		t.Fatalf("apply result = %#v, %v", result, err)
	}
	assertWorkflowToolDisplayProjection(t, result, "workflow_patch_result", "workflow.change.receipt.v2", "receipt")
	var envelope contract.ToolResult[contract.WorkflowMutationReceipt]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	receipt := envelope.StructuredContent
	if receipt.Kind != "receipt" || receipt.TransactionID == "" || receipt.ChangeSetID != fixture.request.ChangeSetID || receipt.Status != "verifying" || receipt.RevisionAfter == nil {
		t.Fatalf("receipt = %#v", receipt)
	}
	if err := tooling.ValidateToolResult(tool.OutputSchema(), result); err != nil {
		t.Fatalf("output contract: %v\nresult=%s\nschema=%s", err, result.StructuredContent, tool.OutputSchema())
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil || snapshot.TransactionID != receipt.TransactionID || snapshot.Receipt.TransactionID != receipt.TransactionID || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("persistent transaction = %#v, %v", snapshot, err)
	}
	replayed, err := tool.Execute(ctx, raw)
	if err != nil || string(replayed.StructuredContent) != string(result.StructuredContent) {
		t.Fatalf("idempotent replay = %#v, %v", replayed, err)
	}
}

func TestWorkflowValidateToolValidatesCommittedAndCandidateGraphs(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	ctx := workflowMutationContext("tenant-a", "user-a", "session-a", "workflow-a", "call-validate-1")
	tool := WorkflowValidateToolV2(app)
	committed := contract.ValidateRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key, Revision: backend.snapshot.Revision.ResourceVersion}
	result, err := tool.Execute(ctx, mustWorkflowMutationJSON(t, committed))
	if err != nil {
		t.Fatal(err)
	}
	assertWorkflowValidationResult(t, tool, result)

	preview, err := app.Preview(context.Background(), service.WorkflowEditPreviewRequest{
		PreviewPatchRequest: workflowPreviewRequest(backend.snapshot), CandidateID: "candidate-validate", Actor: contract.ActorKind("assistant"), ActorID: "user-a", CanvasSessionID: "session-a", AgentSessionID: "session-a", TurnID: "turn-a", ToolCallID: "call-preview-validate",
	})
	if err != nil {
		t.Fatal(err)
	}
	candidate := contract.ValidateRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key, ChangeSetID: preview.ChangeSet.ID}
	result, err = tool.Execute(ctx, mustWorkflowMutationJSON(t, candidate))
	if err != nil {
		t.Fatal(err)
	}
	validation := assertWorkflowValidationResult(t, tool, result)
	if validation.Basis != preview.ValidationBasis {
		t.Fatalf("candidate validation basis = %#v, want %#v", validation.Basis, preview.ValidationBasis)
	}
	if !tool.IsReadOnly(nil) || tool.IsDestructive(nil) || !tool.IsConcurrencySafe(nil) {
		t.Fatalf("validate annotations = readOnly:%v destructive:%v concurrency:%v", tool.IsReadOnly(nil), tool.IsDestructive(nil), tool.IsConcurrencySafe(nil))
	}
}

func TestWorkflowValidateToolReturnsTypedDomainErrorAndRejectsUntrustedInput(t *testing.T) {
	app, backend := newWorkflowMutationFixture(t)
	tool := WorkflowValidateToolV2(app)
	stale := contract.ValidateRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key, Revision: "stale-revision"}
	result, err := tool.Execute(workflowMutationContext("tenant-a", "user-a", "session-a", "workflow-a", "call-validate-stale"), mustWorkflowMutationJSON(t, stale))
	if err != nil {
		t.Fatal(err)
	}
	var envelope contract.ToolResult[contract.DomainError]
	if json.Unmarshal(result.StructuredContent, &envelope) != nil || !result.IsError || !envelope.IsError || envelope.StructuredContent.Code != contract.ErrorStaleRevision {
		t.Fatalf("typed stale validation = %#v / %s", result, result.StructuredContent)
	}
	if result.Display != nil {
		t.Fatalf("plain stale validation error must not masquerade as workflow conflict: %#v", result.Display)
	}
	valid := contract.ValidateRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: backend.snapshot.Key}
	if _, err := tool.Execute(workflowMutationContext("tenant-b", "user-a", "session-a", "workflow-a", "call-validate-cross"), mustWorkflowMutationJSON(t, valid)); err == nil || !strings.Contains(err.Error(), "trusted workflow scope") {
		t.Fatalf("cross-tenant validation error = %v", err)
	}
	raw := append(bytes.TrimSuffix(mustWorkflowMutationJSON(t, valid), []byte("}")), []byte(`,"unknown":true}`)...)
	if err := tool.ValidateInput(context.Background(), raw); err == nil {
		t.Fatal("strict validation input accepted unknown field")
	}
	both := valid
	both.Revision, both.ChangeSetID = backend.snapshot.Revision.ResourceVersion, "changeset-a"
	if err := tool.ValidateInput(context.Background(), mustWorkflowMutationJSON(t, both)); err == nil {
		t.Fatal("validation input accepted both revision and changeSetId")
	}
}

func assertWorkflowValidationResult(t *testing.T, tool tooling.Tool, result tooling.ToolResult) contract.ValidationResult {
	t.Helper()
	if result.IsError || strings.HasPrefix(strings.TrimSpace(result.Content), "{") {
		t.Fatalf("validation result = %#v", result)
	}
	assertWorkflowToolDisplayProjection(t, result, "workflow_patch_validation", "workflow.change.validation.v2", "validation")
	var envelope contract.ToolResult[contract.ValidationResult]
	if err := json.Unmarshal(result.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	validation := envelope.StructuredContent
	if validation.Kind != "validation" || !validation.Valid || validation.Basis.Validate() != nil || validation.Diagnostics == nil {
		t.Fatalf("validation = %#v", validation)
	}
	if err := tooling.ValidateToolResult(tool.OutputSchema(), result); err != nil {
		t.Fatalf("output contract: %v", err)
	}
	return validation
}

func assertWorkflowToolDisplayProjection(t *testing.T, result tooling.ToolResult, displayKind, renderer, structuredKind string) appui.AiopsTransportAgentUIArtifact {
	t.Helper()
	if result.Display == nil {
		t.Fatal("workflow tool result is missing its typed display payload")
	}
	if result.Display.Type != displayKind {
		t.Fatalf("display type = %q, want %q", result.Display.Type, displayKind)
	}
	if !bytes.Equal(result.Display.Data, result.StructuredContent) {
		t.Fatalf("display data must preserve the complete typed envelope:\ndisplay=%s\nstructured=%s", result.Display.Data, result.StructuredContent)
	}

	var displayEnvelope struct {
		SchemaVersion     string `json:"schemaVersion"`
		StructuredContent struct {
			Kind string `json:"kind"`
		} `json:"structuredContent"`
	}
	if err := json.Unmarshal(result.Display.Data, &displayEnvelope); err != nil {
		t.Fatalf("display envelope decode: %v", err)
	}
	if displayEnvelope.SchemaVersion != contract.SchemaVersion || displayEnvelope.StructuredContent.Kind != structuredKind {
		t.Fatalf("display envelope = %#v, want schema %q and kind %q", displayEnvelope, contract.SchemaVersion, structuredKind)
	}

	payload, err := json.Marshal(map[string]any{
		"toolCallId":  "call-workflow-display",
		"toolName":    "workflow.test",
		"displayKind": result.Display.Type,
		"displayData": json.RawMessage(result.Display.Data),
	})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 22, 0, 0, 0, 0, time.UTC)
	itemStatus := agentstate.ItemStatusCompleted
	if result.IsError {
		itemStatus = agentstate.ItemStatusFailed
	}
	turn := &runtimekernel.TurnSnapshot{
		ID:          "turn-workflow-display",
		SessionID:   "session-workflow-display",
		SessionType: runtimekernel.SessionTypeWorkspace,
		Mode:        runtimekernel.ModeChat,
		Lifecycle:   runtimekernel.TurnLifecycleCompleted,
		StartedAt:   now,
		UpdatedAt:   now,
		AgentItems: []agentstate.TurnItem{{
			ID:        "tool-result-workflow-display",
			Type:      agentstate.TurnItemTypeToolResult,
			Status:    itemStatus,
			Payload:   agentstate.PayloadEnvelope{Kind: "tool", Summary: result.Content, Data: payload},
			CreatedAt: now,
		}},
	}
	projected, err := appui.NewTransportProjector().ProjectTurnSnapshot(appui.NewAiopsTransportState(turn.SessionID, "thread-workflow-display"), turn)
	if err != nil {
		t.Fatalf("project typed display: %v", err)
	}
	artifacts := projected.Turns[turn.ID].AgentUIArtifacts
	if len(artifacts) != 1 {
		t.Fatalf("projected artifacts = %#v, want one", artifacts)
	}
	artifact := artifacts[0]
	if artifact.Type != displayKind || artifact.Renderer != renderer || artifact.SchemaVersion != contract.SchemaVersion {
		t.Fatalf("projected artifact triplet = (%q, %q, %q), want (%q, %q, %q)", artifact.Type, artifact.Renderer, artifact.SchemaVersion, displayKind, renderer, contract.SchemaVersion)
	}
	structured, ok := artifact.InlineData["structuredContent"].(map[string]any)
	if !ok || structured["kind"] != structuredKind {
		t.Fatalf("projected typed envelope = %#v, want structured kind %q", artifact.InlineData, structuredKind)
	}
	return artifact
}

func assertWorkflowPreviewReadOnlyActions(t *testing.T, artifact appui.AiopsTransportAgentUIArtifact, preview contract.ChangeSetPreview) {
	t.Helper()
	if len(artifact.Actions) != 2 {
		t.Fatalf("preview actions = %#v, want focus and review", artifact.Actions)
	}
	focus, review := artifact.Actions[0], artifact.Actions[1]
	if focus["id"] != "workflow-focus-operation-0" || focus["intent"] != "workflow.focus_object" || focus["label"] != "定位节点" || focus["mutation"] != false {
		t.Fatalf("focus action = %#v", focus)
	}
	target, ok := focus["target"].(map[string]any)
	if !ok || target["candidateId"] != preview.Candidate.ID || target["changeSetId"] != preview.ChangeSet.ID || target["previewHash"] != preview.ChangeSet.PreviewHash ||
		target["expectedRevision"] != preview.ChangeSet.Base.ResourceVersion || target["operationIndex"] != float64(0) || target["nodeId"] != "step" {
		t.Fatalf("focus target = %#v", target)
	}
	if _, exists := target["edgeId"]; exists {
		t.Fatalf("focus target must contain exactly one object id: %#v", target)
	}
	if review["id"] != "workflow-review-change" || review["intent"] != "workflow.review_change" || review["label"] != "审阅修改" || review["mutation"] != false {
		t.Fatalf("review action = %#v", review)
	}
	reviewTarget, ok := review["target"].(map[string]any)
	if !ok || reviewTarget["candidateId"] != preview.Candidate.ID || reviewTarget["changeSetId"] != preview.ChangeSet.ID || reviewTarget["previewHash"] != preview.ChangeSet.PreviewHash || reviewTarget["expectedRevision"] != preview.ChangeSet.Base.ResourceVersion || len(reviewTarget) != 4 {
		t.Fatalf("review target = %#v", reviewTarget)
	}
}

func assertWorkflowConflictProjection(t *testing.T, artifact appui.AiopsTransportAgentUIArtifact, source contract.DomainError) {
	t.Helper()
	raw, err := json.Marshal(artifact.InlineData)
	if err != nil {
		t.Fatal(err)
	}
	var projected contract.ToolResult[contract.DomainError]
	if err := contract.DecodeStrict(raw, &projected); err != nil {
		t.Fatalf("projected conflict envelope decode: %v", err)
	}
	var sourceDetails, projectedDetails contract.WorkflowConflict
	if err := contract.DecodeStrict(source.Details, &sourceDetails); err != nil {
		t.Fatalf("source conflict details decode: %v", err)
	}
	if err := contract.DecodeStrict(projected.StructuredContent.Details, &projectedDetails); err != nil {
		t.Fatalf("projected conflict details decode: %v", err)
	}
	if projected.StructuredContent.BaseRevision != source.BaseRevision || projected.StructuredContent.CurrentRevision != source.CurrentRevision ||
		strings.Join(projected.StructuredContent.ConflictingPaths, "\x00") != strings.Join(source.ConflictingPaths, "\x00") ||
		projectedDetails.Code != sourceDetails.Code || projectedDetails.Base != sourceDetails.Base || projectedDetails.Current != sourceDetails.Current ||
		strings.Join(projectedDetails.ConflictingPaths, "\x00") != strings.Join(sourceDetails.ConflictingPaths, "\x00") ||
		strings.Join(projectedDetails.SafeRebasePaths, "\x00") != strings.Join(sourceDetails.SafeRebasePaths, "\x00") {
		t.Fatalf("projected conflict lost three-way facts:\nsource=%#v details=%#v\nprojected=%#v details=%#v", source, sourceDetails, projected.StructuredContent, projectedDetails)
	}
}

func assertWorkflowNonConflictErrorsHaveNoConflictDisplay(t *testing.T, conflict contract.DomainError) {
	t.Helper()
	tests := []struct {
		name   string
		mutate func(*contract.DomainError)
	}{
		{name: "other error code", mutate: func(value *contract.DomainError) { value.Code = contract.ErrorInvalidPatch }},
		{name: "missing typed details", mutate: func(value *contract.DomainError) { value.Details = nil }},
		{name: "mismatched current revision", mutate: func(value *contract.DomainError) { value.CurrentRevision = "revision-forged" }},
		{name: "mismatched paths", mutate: func(value *contract.DomainError) { value.ConflictingPaths = []string{"/forged"} }},
		{name: "malformed details", mutate: func(value *contract.DomainError) { value.Details = json.RawMessage(`{"code":"STALE_REVISION"}`) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			candidate := conflict
			candidate.ConflictingPaths = append([]string(nil), conflict.ConflictingPaths...)
			candidate.Details = append(json.RawMessage(nil), conflict.Details...)
			if conflict.Key != nil {
				key := *conflict.Key
				candidate.Key = &key
			}
			tt.mutate(&candidate)
			result, typed := workflowMutationDomainErrorResultV2(&candidate)
			if !typed || result.Display != nil {
				t.Fatalf("non-conflict domain error projected as conflict: typed=%v display=%#v error=%#v", typed, result.Display, candidate)
			}
		})
	}
}

func TestUnimplementedWorkflowToolsAreHiddenFromLegacySurface(t *testing.T) {
	if tools := Tools(legacy.NewService(nil)); len(tools) != 0 {
		t.Fatalf("legacy fake tools remain visible: %v", workflowEditorToolNames(tools))
	}
}

func newWorkflowMutationFixture(t *testing.T) (*service.WorkflowEditApplication, *workflowReadBackend) {
	t.Helper()
	_, backend := newWorkflowReadFixture(t)
	app, err := service.NewWorkflowEditApplication(backend, workflowReadAccess{}, workflowReadReferences{}, service.NewWorkflowCandidateFileStore(t.TempDir()))
	if err != nil {
		t.Fatal(err)
	}
	return app, backend
}

func workflowPreviewRequest(snapshot service.WorkflowEditSnapshot) contract.PreviewPatchRequest {
	position := visual.Position{X: 120, Y: 80}
	return contract.PreviewPatchRequest{
		SchemaVersion: contract.SchemaVersion,
		WorkflowKey:   snapshot.Key,
		Expected:      snapshot.Revision,
		ValidationBasis: contract.ValidationBasis{
			ResourceVersion: snapshot.Revision.ResourceVersion, SemanticHash: snapshot.Revision.SemanticHash,
			CatalogVersion: "catalog-input", ActionSchemaDigest: "schema-input", ReferenceSnapshotHash: "references-input", EnvironmentBindingVersion: "environment-input",
		},
		Intent:     "move the existing step",
		Operations: []contract.WorkflowOperation{contract.NewOperation(&contract.UpdateNodeOperation{NodeID: "step", Set: contract.NodePatch{Position: &position}})},
	}
}

func workflowMutationContext(tenant, user, session, workflowID, toolCallID string) context.Context {
	return tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: tenant, UserID: user, SessionID: session, TurnID: "turn-a", ToolCallID: toolCallID, Metadata: map[string]string{"workflowId": workflowID}})
}

func workflowMutationArtifactContext(t *testing.T, snapshot service.WorkflowEditSnapshot, selection appui.WorkflowTurnSelection, toolCallID string) context.Context {
	t.Helper()
	trusted := appui.WorkflowTurnTrustedScope{TenantID: snapshot.Key.TenantID, WorkflowID: snapshot.Key.WorkflowID, ActorID: "user-a", SessionID: "session-a", CanvasSessionID: "canvas-a"}
	artifact, err := appui.FreezeWorkflowTurnContext(trusted, appui.WorkflowTurnContext{WorkflowKey: snapshot.Key, Revision: snapshot.Revision, Selection: selection, Environment: appui.WorkflowTurnEnvironment{BindingVersion: "env-v1"}, Scope: appui.WorkflowTurnContextScopeBinding{ActorID: trusted.ActorID, SessionID: trusted.SessionID, CanvasSessionID: trusted.CanvasSessionID}})
	if err != nil {
		t.Fatal(err)
	}
	return tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: trusted.TenantID, UserID: trusted.ActorID, SessionID: trusted.SessionID, TurnID: "turn-a", ToolCallID: toolCallID, ToolName: "workflow.preview_patch", Metadata: map[string]string{"workflowId": trusted.WorkflowID, "workflowCanvasSessionId": trusted.CanvasSessionID}, TurnArtifacts: []runtimecontract.TurnArtifactEnvelope{artifact}})
}

func workflowApplyContext(_ *testing.T, request contract.ApplyPatchRequest, _ json.RawMessage, verified *tooling.VerifiedApprovalContext) context.Context {
	return workflowApplyContextWithIdentity(request.WorkflowKey, "user-a", "session-a", "turn-a", "call-apply", verified)
}

func workflowApplyContextWithIdentity(key contract.WorkflowKey, user, session, turnID, toolCallID string, verified *tooling.VerifiedApprovalContext) context.Context {
	return workflowApplyContextWithToolName(key, user, session, turnID, toolCallID, workflowApplyToolNameV2, verified)
}

func workflowApplyContextWithToolName(key contract.WorkflowKey, user, session, turnID, toolCallID, toolName string, verified *tooling.VerifiedApprovalContext) context.Context {
	return tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: key.TenantID, UserID: user, SessionID: session, TurnID: turnID, ToolCallID: toolCallID, ToolName: toolName, Metadata: map[string]string{"workflowId": key.WorkflowID}, VerifiedApproval: verified})
}

func workflowVerifiedApprovalForTest(key contract.WorkflowKey, approvalID, turnID, toolCallID, argumentsHash string) *tooling.VerifiedApprovalContext {
	return &tooling.VerifiedApprovalContext{
		ApprovalID: approvalID, TurnID: turnID, ToolCallID: toolCallID,
		ToolName: workflowApplyToolNameV2, ArgumentsHash: argumentsHash,
		TargetRefs: []string{
			"type=workflow",
			"type=workflow id=" + key.TenantID + ":" + key.WorkflowID + " op=apply",
			"type=workflow_candidate",
		},
		BindingHash: "sha256:" + strings.Repeat("b", 64),
	}
}

type workflowApplyToolFixture struct {
	app     *service.WorkflowEditApplication
	commits *service.WorkflowCommitStore
	request contract.ApplyPatchRequest
}

type workflowApplyToolBackend struct {
	snapshot service.WorkflowEditSnapshot
	visual   *service.VisualWorkflowService
}

func (b *workflowApplyToolBackend) ReadWorkflow(_ context.Context, key contract.WorkflowKey) (service.WorkflowEditSnapshot, error) {
	if key != b.snapshot.Key {
		return service.WorkflowEditSnapshot{}, service.ErrWorkflowEditSnapshot
	}
	return b.snapshot, nil
}

func (b *workflowApplyToolBackend) ListWorkflowActions(ctx context.Context, _ contract.WorkflowKey, filter service.ActionCatalogFilter) ([]service.ActionSpec, error) {
	return b.visual.ListActions(ctx, filter), nil
}

func (b *workflowApplyToolBackend) ValidateWorkflow(ctx context.Context, _ contract.WorkflowKey, graph visual.Graph) (*service.VisualWorkflowValidationResult, error) {
	return b.visual.Validate(ctx, graph)
}

func (b *workflowApplyToolBackend) CompileWorkflow(ctx context.Context, _ contract.WorkflowKey, graph visual.Graph) (*service.CompiledVisualWorkflow, error) {
	return b.visual.Compile(ctx, graph)
}

func newWorkflowApplyToolFixture(t *testing.T) workflowApplyToolFixture {
	t.Helper()
	visualService := service.NewVisualWorkflowService(service.VisualWorkflowServiceConfig{})
	step := workflow.Step{ID: "check", Name: "check", Targets: []string{"local"}, Action: "script.shell", Args: map[string]any{"script": "echo check"}}
	graph := visual.Graph{Version: visual.GraphVersion, Workflow: workflow.Workflow{Version: "v1", Name: "workflow-a", Inventory: workflow.Inventory{Hosts: map[string]workflow.Host{"local": {Address: "127.0.0.1"}}}, Steps: []workflow.Step{step}}, Nodes: []visual.Node{{ID: "start", Type: visual.NodeTypeStart}, {ID: "check", Type: visual.NodeTypeAction, StepID: "check", Step: &step}, {ID: "end", Type: visual.NodeTypeEnd}}, Edges: []visual.Edge{{ID: "start-check", Source: "start", Target: "check", Kind: visual.EdgeKindNext}, {ID: "check-end", Source: "check", Target: "end", Kind: visual.EdgeKindNext}}}
	compiled, err := visualService.Compile(context.Background(), graph)
	if err != nil {
		t.Fatal(err)
	}
	graph, err = visualService.ParseYAML(context.Background(), compiled.YAML)
	if err != nil {
		t.Fatal(err)
	}
	hashes, err := service.VisualWorkflowGraphHashes(graph)
	if err != nil {
		t.Fatal(err)
	}
	key := contract.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}
	backend := &workflowApplyToolBackend{snapshot: service.WorkflowEditSnapshot{Key: key, Revision: contract.Revision{ResourceVersion: "revision-a", SemanticHash: hashes.SemanticHash, LayoutHash: hashes.LayoutHash}, Graph: graph}, visual: visualService}
	commits, err := service.NewWorkflowCommitStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	seedCompiled, err := backend.CompileWorkflow(context.Background(), key, graph)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := commits.Commit(context.Background(), workflowApplySeedCommit(key, backend.snapshot.Revision, []byte(seedCompiled.YAML))); err != nil {
		t.Fatal(err)
	}
	candidates := service.NewWorkflowCandidateFileStore(t.TempDir())
	approvals := service.NewWorkflowApprovalFileStore(t.TempDir(), commits)
	app, err := service.NewWorkflowEditMutationApplication(backend, workflowReadAccess{}, workflowReadReferences{}, candidates, approvals, commits)
	if err != nil {
		t.Fatal(err)
	}
	position := visual.Position{X: 180, Y: 90}
	// Canvas identity belongs to the persisted Candidate and is intentionally
	// distinct from the Runtime/agent session used to approve and execute it.
	preview, err := app.Preview(context.Background(), service.WorkflowEditPreviewRequest{PreviewPatchRequest: contract.PreviewPatchRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: key, Expected: backend.snapshot.Revision, ValidationBasis: contract.ValidationBasis{ResourceVersion: backend.snapshot.Revision.ResourceVersion, SemanticHash: backend.snapshot.Revision.SemanticHash, CatalogVersion: "input-catalog", ActionSchemaDigest: "input-actions", ReferenceSnapshotHash: "input-references", EnvironmentBindingVersion: "input-environment"}, Intent: "move check", Operations: []contract.WorkflowOperation{contract.NewOperation(&contract.UpdateNodeOperation{NodeID: "check", Set: contract.NodePatch{Position: &position}})}}, CandidateID: "candidate-apply", Actor: contract.ActorKind("assistant"), ActorID: "user-a", CanvasSessionID: "canvas-a", AgentSessionID: "session-a", TurnID: "turn-apply", ToolCallID: "call-preview-apply"})
	if err != nil {
		t.Fatal(err)
	}
	request := contract.ApplyPatchRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: key, ChangeSetID: preview.ChangeSet.ID, PreviewHash: preview.PreviewHash, ExpectedRevision: preview.ChangeSet.Base.ResourceVersion, IdempotencyKey: "apply-idempotency", CandidateGrantID: "grant-apply", ApplyApprovalID: "approval-runtime", Reason: "apply reviewed candidate"}
	return workflowApplyToolFixture{app: app, commits: commits, request: request}
}

func workflowApplySeedCommit(key contract.WorkflowKey, revision contract.Revision, yaml []byte) service.WorkflowCommitInput {
	basis := contract.ValidationBasis{ResourceVersion: revision.ResourceVersion, SemanticHash: revision.SemanticHash, CatalogVersion: "seed-catalog", ActionSchemaDigest: "seed-actions", ReferenceSnapshotHash: "seed-references", EnvironmentBindingVersion: "seed-environment"}
	receipt := contract.WorkflowMutationReceipt{SchemaVersion: contract.SchemaVersion, Kind: "receipt", TransactionID: "transaction-seed", ChangeSetID: "change-set-seed", Key: key, Status: "committed", RevisionBefore: contract.Revision{}, RevisionAfter: &revision, Validation: contract.ValidationResult{Kind: "validation", Basis: basis, Valid: true, Diagnostics: []contract.WorkflowDiagnostic{}}, AuditID: "audit-seed"}
	envelope := func(kind string) workflowstore.TypedEnvelope {
		return workflowstore.TypedEnvelope{Kind: kind, SchemaVersion: contract.SchemaVersion, Payload: json.RawMessage(`{"ok":true}`)}
	}
	return service.WorkflowCommitInput{Key: key, ExpectedResourceVersion: "", IdempotencyKey: "seed-idempotency", RequestDigest: "seed-request-digest", GraphPayload: append([]byte(nil), yaml...), Meta: envelope("meta"), History: envelope("history"), ImportResult: envelope("import"), RollbackResult: envelope("rollback"), Receipt: receipt, ValidationBasis: basis, Checkpoint: envelope("checkpoint"), Inverse: envelope("inverse"), ApprovalConsume: envelope("approval-consume"), Audit: envelope("audit"), Events: []workflowstore.TypedEnvelope{envelope("event")}, Outbox: []workflowstore.TypedEnvelope{envelope("outbox")}}
}

func mustWorkflowMutationJSON(t *testing.T, value any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
