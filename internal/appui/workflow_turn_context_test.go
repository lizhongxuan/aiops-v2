package appui

import (
	"context"
	"testing"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/workflow/edit"
)

func TestWorkflowTurnContextFreezesSelectionAtSendTime(t *testing.T) {
	trusted, context := workflowTurnContextFixture()
	artifact, err := FreezeWorkflowTurnContext(trusted, context)
	if err != nil {
		t.Fatal(err)
	}
	context.Selection.NodeIDs[0] = "changed-after-send"
	decoded, err := DecodeWorkflowTurnContext(artifact)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded.Selection.NodeIDs) != 2 || decoded.Selection.NodeIDs[0] != "check" || decoded.Selection.NodeIDs[1] != "repair" || decoded.Revision.ResourceVersion != "rv-7" {
		t.Fatalf("frozen context = %#v", decoded)
	}

	_, sameContext := workflowTurnContextFixture()
	sameContext.Selection.NodeIDs = []string{"repair", "check", "check"}
	same, err := FreezeWorkflowTurnContext(trusted, sameContext)
	if err != nil {
		t.Fatal(err)
	}
	if same.Digest != artifact.Digest || same.ID != artifact.ID {
		t.Fatalf("equivalent selection changed artifact identity: %s != %s", same.Digest, artifact.Digest)
	}
}

func TestWorkflowTurnContextRejectsMissingOrForgedTrustedScope(t *testing.T) {
	trusted, context := workflowTurnContextFixture()
	tests := []struct {
		name   string
		mutate func(*WorkflowTurnTrustedScope, *WorkflowTurnContext)
	}{
		{"missing trusted actor", func(scope *WorkflowTurnTrustedScope, _ *WorkflowTurnContext) { scope.ActorID = "" }},
		{"tenant", func(_ *WorkflowTurnTrustedScope, context *WorkflowTurnContext) {
			context.WorkflowKey.TenantID = "tenant-b"
		}},
		{"workflow", func(_ *WorkflowTurnTrustedScope, context *WorkflowTurnContext) {
			context.WorkflowKey.WorkflowID = "workflow-b"
		}},
		{"actor", func(_ *WorkflowTurnTrustedScope, context *WorkflowTurnContext) { context.Scope.ActorID = "actor-b" }},
		{"session", func(_ *WorkflowTurnTrustedScope, context *WorkflowTurnContext) { context.Scope.SessionID = "session-b" }},
		{"canvas session", func(_ *WorkflowTurnTrustedScope, context *WorkflowTurnContext) {
			context.Scope.CanvasSessionID = "canvas-b"
		}},
	}
	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			scopeCopy, contextCopy := trusted, context
			testCase.mutate(&scopeCopy, &contextCopy)
			if _, err := FreezeWorkflowTurnContext(scopeCopy, contextCopy); err == nil {
				t.Fatal("forged workflow turn scope was accepted")
			}
		})
	}
}

func TestWorkflowTurnContextChatAdapterUsesOnlyTrustedExecutionIdentity(t *testing.T) {
	trusted, workflowContext := workflowTurnContextFixture()
	ctx := tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{
		TenantID: trusted.TenantID, UserID: trusted.ActorID, SessionID: trusted.SessionID,
		Metadata: map[string]string{"workflowId": trusted.WorkflowID, "workflowCanvasSessionId": trusted.CanvasSessionID},
	})
	page := WorkflowTurnContextRequest{
		WorkflowID: trusted.WorkflowID, Revision: workflowContext.Revision,
		Selection: workflowContext.Selection, Environment: workflowContext.Environment,
	}
	request := runtimekernel.TurnRequest{SessionID: trusted.SessionID}
	if err := attachWorkflowTurnContext(ctx, ChatCommand{WorkflowContext: &page}, &request); err != nil {
		t.Fatal(err)
	}
	if len(request.TurnArtifacts) != 1 {
		t.Fatalf("turn artifacts = %#v", request.TurnArtifacts)
	}
	if request.TrustedIdentity == nil || request.TrustedIdentity.TenantID != trusted.TenantID || request.TrustedIdentity.UserID != trusted.ActorID {
		t.Fatalf("trusted identity = %#v", request.TrustedIdentity)
	}
	if len(request.ResourceBindings) != 1 || !request.ResourceBindings[0].Verified() || request.ResourceBindings[0].Ref.Type != resourcebinding.ResourceTypeWorkflow || request.ResourceBindings[0].Ref.ID != trusted.WorkflowID || request.ResourceBindings[0].Ref.Namespace != trusted.TenantID {
		t.Fatalf("workflow resource bindings = %#v, want one verified frozen workflow", request.ResourceBindings)
	}
	decoded, err := DecodeWorkflowTurnContext(request.TurnArtifacts[0])
	if err != nil || decoded.WorkflowKey.TenantID != trusted.TenantID || decoded.Scope.ActorID != trusted.ActorID || decoded.Scope.CanvasSessionID != trusted.CanvasSessionID {
		t.Fatalf("decoded context = %#v, %v", decoded, err)
	}

	for name, mutate := range map[string]func(*tooling.ToolExecutionContext, *WorkflowTurnContextRequest){
		"missing authenticated tenant": func(execution *tooling.ToolExecutionContext, _ *WorkflowTurnContextRequest) { execution.TenantID = "" },
		"forged page workflow": func(_ *tooling.ToolExecutionContext, page *WorkflowTurnContextRequest) {
			page.WorkflowID = "workflow-b"
		},
		"forged page tenant metadata": func(execution *tooling.ToolExecutionContext, _ *WorkflowTurnContextRequest) {
			execution.TenantID = " tenant-a "
		},
	} {
		t.Run(name, func(t *testing.T) {
			execution, _ := tooling.ToolExecutionContextFrom(ctx)
			pageCopy := page
			mutate(&execution, &pageCopy)
			forged := tooling.ContextWithToolExecution(context.Background(), execution)
			if _, err := FreezeWorkflowTurnContextForChat(forged, trusted.SessionID, pageCopy); err == nil {
				t.Fatal("untrusted workflow scope was accepted")
			}
		})
	}
}

func workflowTurnContextFixture() (WorkflowTurnTrustedScope, WorkflowTurnContext) {
	trusted := WorkflowTurnTrustedScope{TenantID: "tenant-a", WorkflowID: "workflow-a", ActorID: "actor-a", SessionID: "session-a", CanvasSessionID: "canvas-a"}
	context := WorkflowTurnContext{
		WorkflowKey: edit.WorkflowKey{TenantID: trusted.TenantID, WorkflowID: trusted.WorkflowID},
		Revision:    edit.Revision{ResourceVersion: "rv-7", SemanticHash: "sha256:semantic", LayoutHash: "sha256:layout"},
		Selection:   WorkflowTurnSelection{NodeIDs: []string{"repair", "check"}, EdgeIDs: []string{"check-repair"}, NeighborhoodIDs: []string{"start", "end"}},
		Environment: WorkflowTurnEnvironment{Name: "staging", BindingVersion: "env-v3", CapabilityVersions: []string{"shell@2", "ssh@1"}},
		Scope:       WorkflowTurnContextScopeBinding{ActorID: trusted.ActorID, SessionID: trusted.SessionID, CanvasSessionID: trusted.CanvasSessionID},
	}
	return trusted, context
}
