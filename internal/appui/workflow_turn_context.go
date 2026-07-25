package appui

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/resourcebinding"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/workflow/edit"
)

const WorkflowTurnContextArtifactKind = "workflow.context"

type WorkflowTurnTrustedScope struct {
	TenantID        string `json:"tenantId"`
	WorkflowID      string `json:"workflowId"`
	ActorID         string `json:"actorId"`
	SessionID       string `json:"sessionId"`
	CanvasSessionID string `json:"canvasSessionId,omitempty"`
}

// WorkflowTurnContextRequest is mutable page state captured when the user
// presses Send. It is never trusted as an identity source.
type WorkflowTurnContextRequest struct {
	WorkflowID  string                  `json:"workflowId"`
	Revision    edit.Revision           `json:"revision"`
	Selection   WorkflowTurnSelection   `json:"selection"`
	Environment WorkflowTurnEnvironment `json:"environment"`
}

type WorkflowTurnContext struct {
	WorkflowKey edit.WorkflowKey                `json:"workflowKey"`
	Revision    edit.Revision                   `json:"revision"`
	Selection   WorkflowTurnSelection           `json:"selection"`
	Environment WorkflowTurnEnvironment         `json:"environment"`
	Scope       WorkflowTurnContextScopeBinding `json:"scope"`
}

type WorkflowTurnSelection struct {
	NodeIDs         []string `json:"nodeIds"`
	EdgeIDs         []string `json:"edgeIds"`
	NeighborhoodIDs []string `json:"neighborhoodIds,omitempty"`
}

type WorkflowTurnEnvironment struct {
	Name               string   `json:"name,omitempty"`
	BindingVersion     string   `json:"bindingVersion"`
	CapabilityVersions []string `json:"capabilityVersions,omitempty"`
}

type WorkflowTurnContextScopeBinding struct {
	ActorID         string `json:"actorId"`
	SessionID       string `json:"sessionId"`
	CanvasSessionID string `json:"canvasSessionId,omitempty"`
}

func FreezeWorkflowTurnContext(trusted WorkflowTurnTrustedScope, context WorkflowTurnContext) (runtimecontract.TurnArtifactEnvelope, error) {
	if err := validateWorkflowTurnTrustedScope(trusted); err != nil {
		return runtimecontract.TurnArtifactEnvelope{}, err
	}
	if context.WorkflowKey != (edit.WorkflowKey{TenantID: trusted.TenantID, WorkflowID: trusted.WorkflowID}) || context.Scope.ActorID != trusted.ActorID || context.Scope.SessionID != trusted.SessionID ||
		(trusted.CanvasSessionID != "" && context.Scope.CanvasSessionID != trusted.CanvasSessionID) {
		return runtimecontract.TurnArtifactEnvelope{}, fmt.Errorf("workflow turn context does not match trusted scope")
	}
	if err := validateWorkflowTurnContext(context); err != nil {
		return runtimecontract.TurnArtifactEnvelope{}, err
	}
	context.Selection.NodeIDs = stableWorkflowTurnIDs(context.Selection.NodeIDs)
	context.Selection.EdgeIDs = stableWorkflowTurnIDs(context.Selection.EdgeIDs)
	context.Selection.NeighborhoodIDs = stableWorkflowTurnIDs(context.Selection.NeighborhoodIDs)
	context.Environment.CapabilityVersions = stableWorkflowTurnIDs(context.Environment.CapabilityVersions)
	return runtimecontract.NewInlineTurnArtifact(WorkflowTurnContextArtifactKind, context)
}

// FreezeWorkflowTurnContextForChat binds mutable page state to the trusted
// identity installed by authentication middleware. Empty trusted fields fail
// closed; page and model values never fill them.
func FreezeWorkflowTurnContextForChat(ctx context.Context, sessionID string, request WorkflowTurnContextRequest) (runtimecontract.TurnArtifactEnvelope, error) {
	execution, ok := tooling.ToolExecutionContextFrom(ctx)
	trustedWorkflowID := strings.TrimSpace(execution.Metadata["workflowId"])
	trusted := WorkflowTurnTrustedScope{
		TenantID: strings.TrimSpace(execution.TenantID), WorkflowID: trustedWorkflowID,
		ActorID: strings.TrimSpace(execution.UserID), SessionID: strings.TrimSpace(execution.SessionID),
		CanvasSessionID: strings.TrimSpace(execution.Metadata["workflowCanvasSessionId"]),
	}
	if trusted.CanvasSessionID == "" {
		// Direct Runtime callers created before the HTTP Canvas binding existed
		// have one session identity. The production AssistantTransport adapter
		// always overwrites workflowCanvasSessionId from its trusted resolver.
		trusted.CanvasSessionID = trusted.SessionID
	}
	if !ok || execution.TenantID != trusted.TenantID || execution.UserID != trusted.ActorID || execution.SessionID != trusted.SessionID ||
		strings.TrimSpace(sessionID) != trusted.SessionID || strings.TrimSpace(request.WorkflowID) != trusted.WorkflowID {
		return runtimecontract.TurnArtifactEnvelope{}, fmt.Errorf("trusted workflow chat scope is required")
	}
	return FreezeWorkflowTurnContext(trusted, WorkflowTurnContext{
		WorkflowKey: edit.WorkflowKey{TenantID: trusted.TenantID, WorkflowID: trusted.WorkflowID},
		Revision:    request.Revision, Selection: request.Selection, Environment: request.Environment,
		Scope: WorkflowTurnContextScopeBinding{ActorID: trusted.ActorID, SessionID: trusted.SessionID, CanvasSessionID: trusted.CanvasSessionID},
	})
}

func attachWorkflowTurnContext(ctx context.Context, command ChatCommand, request *runtimekernel.TurnRequest) error {
	if command.WorkflowContext == nil {
		return nil
	}
	artifact, err := FreezeWorkflowTurnContextForChat(ctx, request.SessionID, *command.WorkflowContext)
	if err != nil {
		return err
	}
	execution, _ := tooling.ToolExecutionContextFrom(ctx)
	trustedWorkflowID := strings.TrimSpace(execution.Metadata["workflowId"])
	trustedTenantID := strings.TrimSpace(execution.TenantID)
	request.TrustedIdentity = &runtimecontract.TurnIdentity{
		TenantID: strings.TrimSpace(execution.TenantID), UserID: strings.TrimSpace(execution.UserID), SessionID: strings.TrimSpace(execution.SessionID),
	}
	request.ResourceBindings = mergeResourceBindings(request.ResourceBindings, []resourcebinding.ResourceBindingSnapshot{
		resourcebinding.NewBindingSnapshot(resourcebinding.ResourceRef{
			Type: resourcebinding.ResourceTypeWorkflow, ID: trustedWorkflowID,
			Namespace: trustedTenantID, Provider: "runner",
		}, resourcebinding.BindingOptions{
			Source: resourcebinding.BindingSourceWorkflow, VerifiedBy: WorkflowTurnContextArtifactKind,
			TrustLevel: resourcebinding.TrustLevelVerified,
		}),
	})
	request.TurnArtifacts = append(request.TurnArtifacts, artifact)
	return nil
}

func DecodeWorkflowTurnContext(artifact runtimecontract.TurnArtifactEnvelope) (WorkflowTurnContext, error) {
	if err := artifact.Validate(); err != nil {
		return WorkflowTurnContext{}, err
	}
	if artifact.Kind != WorkflowTurnContextArtifactKind || len(artifact.Inline) == 0 {
		return WorkflowTurnContext{}, fmt.Errorf("artifact is not an inline workflow turn context")
	}
	var context WorkflowTurnContext
	if err := edit.DecodeStrict(artifact.Inline, &context); err != nil {
		return WorkflowTurnContext{}, fmt.Errorf("decode workflow turn context: %w", err)
	}
	if err := validateWorkflowTurnContext(context); err != nil {
		return WorkflowTurnContext{}, err
	}
	return context, nil
}

func validateWorkflowTurnTrustedScope(scope WorkflowTurnTrustedScope) error {
	key := edit.WorkflowKey{TenantID: scope.TenantID, WorkflowID: scope.WorkflowID}
	if key.Validate() != nil || strings.TrimSpace(scope.ActorID) == "" || strings.TrimSpace(scope.SessionID) == "" ||
		(scope.CanvasSessionID != "" && strings.TrimSpace(scope.CanvasSessionID) != scope.CanvasSessionID) {
		return fmt.Errorf("trusted workflow turn scope is required")
	}
	return nil
}

func validateWorkflowTurnContext(context WorkflowTurnContext) error {
	if context.WorkflowKey.Validate() != nil || strings.TrimSpace(context.Revision.ResourceVersion) == "" || strings.TrimSpace(context.Revision.SemanticHash) == "" || strings.TrimSpace(context.Revision.LayoutHash) == "" || strings.TrimSpace(context.Scope.ActorID) == "" || strings.TrimSpace(context.Scope.SessionID) == "" || strings.TrimSpace(context.Environment.BindingVersion) == "" ||
		(context.Scope.CanvasSessionID != "" && strings.TrimSpace(context.Scope.CanvasSessionID) != context.Scope.CanvasSessionID) {
		return fmt.Errorf("workflow turn context is incomplete")
	}
	for _, values := range [][]string{context.Selection.NodeIDs, context.Selection.EdgeIDs, context.Selection.NeighborhoodIDs, context.Environment.CapabilityVersions} {
		for _, value := range values {
			if strings.TrimSpace(value) == "" || value != strings.TrimSpace(value) {
				return fmt.Errorf("workflow turn context contains an invalid identifier")
			}
		}
	}
	return nil
}

func stableWorkflowTurnIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	sort.Strings(result)
	return result
}
