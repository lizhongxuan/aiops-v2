package workfloweditor

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	contract "runner/workflow/edit"
)

const (
	workflowApplyToolNameV2      = "workflow.apply_patch"
	workflowApplyProviderAliasV2 = "workflow_apply_patch"
)

type workflowPreviewDetailV2 struct {
	CandidateRevision uint64                          `json:"candidateRevision"`
	PreviewHash       string                          `json:"previewHash"`
	ValidationBasis   contract.ValidationBasis        `json:"validationBasis"`
	Diagnostics       []contract.WorkflowDiagnostic   `json:"diagnostics"`
	Risk              service.WorkflowEditPreviewRisk `json:"risk"`
	Diff              service.WorkflowEditPreviewDiff `json:"diff"`
	Rebased           bool                            `json:"rebased"`
}

type workflowPreviewArtifactActionTargetV2 struct {
	CandidateID      string `json:"candidateId"`
	ChangeSetID      string `json:"changeSetId"`
	PreviewHash      string `json:"previewHash"`
	ExpectedRevision string `json:"expectedRevision"`
	OperationIndex   *int   `json:"operationIndex,omitempty"`
	NodeID           string `json:"nodeId,omitempty"`
	EdgeID           string `json:"edgeId,omitempty"`
}

type workflowPreviewArtifactActionV2 struct {
	ID       string                                `json:"id"`
	Intent   string                                `json:"intent"`
	Label    string                                `json:"label"`
	Mutation bool                                  `json:"mutation"`
	Target   workflowPreviewArtifactActionTargetV2 `json:"target"`
}

func WorkflowMutationToolsV2(app *service.WorkflowEditApplication) []tooling.Tool {
	if app == nil {
		return nil
	}
	return []tooling.Tool{WorkflowPreviewPatchToolV2(app), WorkflowApplyPatchToolV2(app), WorkflowValidateToolV2(app)}
}

func WorkflowPreviewPatchToolV2(app *service.WorkflowEditApplication) tooling.Tool {
	if app == nil {
		return nil
	}
	schemas := WorkflowToolContractsV2()["workflow.preview_patch"]
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "workflow.preview_patch", Description: "在当前授权 Workflow 的 immutable Candidate 上规范化并模拟 typed operations，返回真实 diff、risk、validation 与 previewHash；不会提交 committed Graph。",
			Layer: tooling.ToolLayerCore, Pack: ToolPack, Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, AlwaysLoad: true, IsMCP: true,
			MCPInfo: tooling.MCPInfo{ServerID: ServerID, ServerName: "Runner Workflow Editor", ToolName: "workflow.preview_patch"}, RiskLevel: tooling.ToolRiskLow,
			ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}, FailurePolicy: tooling.ToolFailurePolicyFeedBackToModel,
			Discovery: tooling.ToolDiscoveryMetadata{DiscoveryGroup: ToolPack, CapabilityKind: "runner", ResourceTypes: []string{"workflow_candidate", "workflow"}, OperationKinds: []string{"preview", "validate", "proposal"}, RiskLevel: tooling.ToolRiskLow, LoadingPolicy: tooling.ToolLoadingPolicyCore, AgentProfiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, ToolPackIDs: []string{ToolPack}, MCPServerID: ServerID, PermissionScope: "workflow.preview", PromptBudgetClass: "compact", SchemaBudgetClass: "on_demand"},
		},
		InputSchemaData: schemas.InputSchema, OutputSchemaData: schemas.OutputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool { return false }, DestructiveFunc: func(json.RawMessage) bool { return false }, ConcurrencySafeFunc: func(json.RawMessage) bool { return false },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := decodeWorkflowPreviewPatchV2(input)
			return err
		},
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			request, err := decodeWorkflowPreviewPatchV2(input)
			if err == nil {
				_, err = trustedWorkflowScope(ctx, request.WorkflowKey)
			}
			if err == nil {
				err = bindWorkflowPreviewToTurnScopeV2(ctx, request)
			}
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "trusted workflow scope is unavailable"}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			request, err := decodeWorkflowPreviewPatchV2(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			scope, err := trustedWorkflowScope(ctx, request.WorkflowKey)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if err := bindWorkflowPreviewToTurnScopeV2(ctx, request); err != nil {
				return tooling.ToolResult{}, err
			}
			execution, ok := tooling.ToolExecutionContextFrom(ctx)
			if !ok || scope.SessionID == "" || scope.SessionID != strings.TrimSpace(scope.SessionID) || execution.ToolCallID == "" || execution.ToolCallID != strings.TrimSpace(execution.ToolCallID) {
				return tooling.ToolResult{}, fmt.Errorf("trusted workflow preview execution scope is unavailable")
			}
			preview, err := app.Preview(ctx, service.WorkflowEditPreviewRequest{
				PreviewPatchRequest: request, CandidateID: workflowPreviewCandidateIDV2(scope, execution.ToolCallID), Actor: contract.ActorKind("agent"), ActorID: scope.UserID,
				CanvasSessionID: scope.CanvasSessionID, AgentSessionID: execution.SessionID, TurnID: execution.TurnID, ToolCallID: execution.ToolCallID,
			})
			if err != nil {
				if result, typed := workflowMutationDomainErrorResultV2(err); typed {
					return result, nil
				}
				return tooling.ToolResult{}, err
			}
			return workflowPreviewToolResultV2(preview), nil
		},
	}
}

func WorkflowApplyPatchToolV2(app *service.WorkflowEditApplication) tooling.Tool {
	if app == nil {
		return nil
	}
	schemas := WorkflowToolContractsV2()["workflow.apply_patch"]
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "workflow.apply_patch", Description: "提交已预览且已批准的 immutable Workflow Candidate。必须绑定当前 revision、previewHash、idempotency key，以及 Runtime 验证过的用户批准。",
			Layer: tooling.ToolLayerMutation, Pack: ToolPack, Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, AlwaysLoad: true, IsMCP: true,
			MCPInfo: tooling.MCPInfo{ServerID: ServerID, ServerName: "Runner Workflow Editor", ToolName: "workflow.apply_patch"}, RiskLevel: tooling.ToolRiskHigh, Mutating: true, RequiresApproval: true, RecordEvidence: true,
			ResourceLockResolver: workflowApplyResourceLocksV2,
			Idempotency:          tooling.ToolIdempotencyMetadata{Strategy: tooling.ToolIdempotencyStrategyExplicitKey, KeyFields: []string{"idempotencyKey"}, PostCheckRefs: []string{"workflow.validate against the committed revision", "persisted workflow receipt lifecycle"}},
			Rollback:             &tooling.ToolRollbackMetadata{Strategy: tooling.ToolRollbackStrategyAutomatic, Reference: "runner.workflow.commit-store.inverse-transaction.v2"},
			ResultBudget:         tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}, FailurePolicy: tooling.ToolFailurePolicyFeedBackToModel,
			Discovery: tooling.ToolDiscoveryMetadata{DiscoveryGroup: ToolPack, CapabilityKind: "runner", ResourceTypes: []string{"workflow_candidate", "workflow"}, OperationKinds: []string{"apply", "mutate"}, RiskLevel: tooling.ToolRiskHigh, LoadingPolicy: tooling.ToolLoadingPolicyCore, AgentProfiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, ToolPackIDs: []string{ToolPack}, MCPServerID: ServerID, PermissionScope: "argument_scoped", PromptBudgetClass: "compact", SchemaBudgetClass: "on_demand"},
		},
		InputSchemaData: schemas.InputSchema, OutputSchemaData: schemas.OutputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool { return false }, DestructiveFunc: func(json.RawMessage) bool { return true }, ConcurrencySafeFunc: func(json.RawMessage) bool { return false },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := decodeWorkflowApplyPatchV2(input)
			return err
		},
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			request, err := decodeWorkflowApplyPatchV2(input)
			if err == nil {
				_, err = trustedWorkflowScope(ctx, request.WorkflowKey)
			}
			if err == nil {
				err = bindWorkflowApplyToTurnScopeV2(ctx, request)
			}
			if err == nil {
				err = bindWorkflowCandidateToTurnScopeV2(ctx, app, request.WorkflowKey, request.ChangeSetID)
			}
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "trusted workflow scope is unavailable"}
			}
			verified, err := verifiedWorkflowApplyApprovalV2(ctx, input)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "verified workflow approval binding is invalid"}
			}
			if verified == nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: "applying the reviewed workflow candidate requires explicit user approval", Approval: &tooling.PermissionApprovalPayload{Command: "apply workflow candidate " + request.ChangeSetID, Reason: request.Reason, Risk: string(tooling.ToolRiskHigh), Source: "workflow", ExpectedEffect: "commit the exact reviewed preview", Rollback: "use the persisted inverse transaction", Validation: "run workflow.validate and post-checks against the committed revision"}}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			request, err := decodeWorkflowApplyPatchV2(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			scope, err := trustedWorkflowScope(ctx, request.WorkflowKey)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if err := bindWorkflowApplyToTurnScopeV2(ctx, request); err != nil {
				return tooling.ToolResult{}, err
			}
			if err := bindWorkflowCandidateToTurnScopeV2(ctx, app, request.WorkflowKey, request.ChangeSetID); err != nil {
				return tooling.ToolResult{}, err
			}
			verified, err := verifiedWorkflowApplyApprovalV2(ctx, input)
			if err != nil || verified == nil {
				return tooling.ToolResult{}, fmt.Errorf("verified workflow approval is required")
			}
			// RuntimeKernel is the authority for the user's exact tool-call
			// approval. Materialize its verified binding as the persisted business
			// Grant/ApplyApproval only after the token has been revalidated here;
			// model/browser IDs alone never grant mutation authority.
			trusted, err := app.AuthorizeVerifiedRuntimeApply(ctx, service.WorkflowVerifiedRuntimeApplyRequest{
				SchemaVersion: contract.SchemaVersion, Key: scope.Key, ActorID: scope.UserID, RuntimeSessionID: scope.SessionID,
				ChangeSetID: request.ChangeSetID, PreviewHash: request.PreviewHash, ExpectedRevision: request.ExpectedRevision,
				CandidateGrantID: request.CandidateGrantID, ApplyApprovalID: request.ApplyApprovalID,
				RuntimeApprovalID: verified.ApprovalID, RuntimeTurnID: verified.TurnID,
				RuntimeToolCallID: verified.ToolCallID, RuntimeToolName: verified.ToolName,
				RuntimeArgumentsHash: verified.ArgumentsHash, RuntimeBindingHash: verified.BindingHash,
				RuntimeTargetRefs: append([]string(nil), verified.TargetRefs...),
			})
			if err != nil {
				if result, typed := workflowMutationDomainErrorResultV2(err); typed {
					return result, nil
				}
				return tooling.ToolResult{}, err
			}
			receipt, err := app.Apply(ctx, trusted, service.WorkflowEditApplyRequest{ApplyPatchRequest: request, CandidateID: trusted.CandidateID, CandidateRevision: trusted.CandidateRevision, ActorID: scope.UserID, CanvasSessionID: trusted.CanvasSessionID})
			if err != nil {
				if result, typed := workflowMutationDomainErrorResultV2(err); typed {
					return result, nil
				}
				return tooling.ToolResult{}, err
			}
			return workflowApplyToolResultV2(receipt), nil
		},
	}
}

func decodeWorkflowApplyPatchV2(input json.RawMessage) (contract.ApplyPatchRequest, error) {
	var request contract.ApplyPatchRequest
	if err := contract.DecodeStrict(input, &request); err != nil {
		return request, err
	}
	if request.SchemaVersion != contract.SchemaVersion || request.WorkflowKey.Validate() != nil {
		return request, fmt.Errorf("invalid workflow.apply_patch request")
	}
	for _, value := range []string{request.ChangeSetID, request.PreviewHash, request.ExpectedRevision, request.IdempotencyKey, request.CandidateGrantID, request.ApplyApprovalID, request.Reason} {
		if strings.TrimSpace(value) == "" || strings.TrimSpace(value) != value {
			return request, fmt.Errorf("invalid workflow.apply_patch binding")
		}
	}
	return request, nil
}

func verifiedWorkflowApplyApprovalV2(ctx context.Context, input json.RawMessage) (*tooling.VerifiedApprovalContext, error) {
	execution, ok := tooling.ToolExecutionContextFrom(ctx)
	if !ok || execution.VerifiedApproval == nil {
		return nil, nil
	}
	approval := execution.VerifiedApproval
	argumentsHash, err := actionproposal.NormalizedInputHash(input)
	if err != nil {
		return nil, err
	}
	if approval.ApprovalID == "" || approval.BindingHash == "" || approval.TurnID != execution.TurnID || approval.ToolCallID != execution.ToolCallID || !validWorkflowApplyRuntimeToolNameV2(execution.ToolName) || approval.ToolName != execution.ToolName || approval.ArgumentsHash != argumentsHash {
		return nil, fmt.Errorf("workflow approval does not bind this tool call")
	}
	return approval, nil
}

func validWorkflowApplyRuntimeToolNameV2(value string) bool {
	return value == workflowApplyToolNameV2 || value == workflowApplyProviderAliasV2
}

func workflowApplyResourceLocksV2(input json.RawMessage, execution tooling.ToolExecutionContext) ([]tooling.ToolResourceLockKey, error) {
	request, err := decodeWorkflowApplyPatchV2(input)
	if err != nil {
		return nil, err
	}
	ctx := tooling.ContextWithToolExecution(context.Background(), execution)
	scope, err := trustedWorkflowScope(ctx, request.WorkflowKey)
	if err != nil {
		return nil, fmt.Errorf("trusted workflow lock scope is unavailable")
	}
	return []tooling.ToolResourceLockKey{{ResourceType: "workflow", ResourceID: scope.Key.TenantID + ":" + scope.Key.WorkflowID, OperationKind: "apply"}}, nil
}

func bindWorkflowPreviewToTurnScopeV2(ctx context.Context, request contract.PreviewPatchRequest) error {
	frozen, found, err := workflowMutationTurnContextV2(ctx, request.WorkflowKey)
	if err != nil || !found {
		return err
	}
	if request.Expected != frozen.Revision || request.ValidationBasis.ResourceVersion != frozen.Revision.ResourceVersion || request.ValidationBasis.SemanticHash != frozen.Revision.SemanticHash || request.ValidationBasis.EnvironmentBindingVersion != frozen.Environment.BindingVersion {
		return fmt.Errorf("workflow preview is outside the frozen turn revision or environment")
	}
	return workflowOperationsWithinTurnScopeV2(frozen, request.Operations)
}

func bindWorkflowApplyToTurnScopeV2(ctx context.Context, request contract.ApplyPatchRequest) error {
	frozen, found, err := workflowMutationTurnContextV2(ctx, request.WorkflowKey)
	if err != nil || !found {
		return err
	}
	if request.ExpectedRevision != frozen.Revision.ResourceVersion {
		return fmt.Errorf("workflow apply is outside the frozen turn revision")
	}
	return nil
}

func bindWorkflowValidateToTurnScopeV2(ctx context.Context, request contract.ValidateRequest) (contract.ValidateRequest, error) {
	frozen, found, err := workflowMutationTurnContextV2(ctx, request.WorkflowKey)
	if err != nil || !found {
		return request, err
	}
	if request.Revision != "" && request.Revision != frozen.Revision.ResourceVersion {
		return request, fmt.Errorf("workflow validation is outside the frozen turn revision")
	}
	if request.Revision == "" && request.ChangeSetID == "" {
		request.Revision = frozen.Revision.ResourceVersion
	}
	return request, nil
}

func bindWorkflowCandidateToTurnScopeV2(ctx context.Context, app *service.WorkflowEditApplication, key contract.WorkflowKey, changeSetID string) error {
	frozen, found, err := workflowMutationTurnContextV2(ctx, key)
	if err != nil || !found {
		return err
	}
	scope, err := app.ResolveCandidateScope(ctx, key, changeSetID)
	if err != nil {
		return err
	}
	if scope.Key != key || scope.ChangeSetID != changeSetID || scope.Base != frozen.Revision {
		return fmt.Errorf("workflow candidate is outside the frozen turn revision")
	}
	return workflowOperationsWithinTurnScopeV2(frozen, scope.NormalizedOperations)
}

func workflowMutationTurnContextV2(ctx context.Context, key contract.WorkflowKey) (appui.WorkflowTurnContext, bool, error) {
	artifact, found := runtimecontract.TurnArtifactFromContext(ctx, appui.WorkflowTurnContextArtifactKind)
	if !found {
		return appui.WorkflowTurnContext{}, false, nil
	}
	frozen, err := appui.DecodeWorkflowTurnContext(artifact)
	if err != nil || frozen.WorkflowKey != key {
		return appui.WorkflowTurnContext{}, true, fmt.Errorf("trusted workflow turn scope is unavailable")
	}
	return frozen, true, nil
}

func workflowOperationsWithinTurnScopeV2(frozen appui.WorkflowTurnContext, operations []contract.WorkflowOperation) error {
	selection := frozen.Selection
	if len(selection.NodeIDs) == 0 && len(selection.EdgeIDs) == 0 && len(selection.NeighborhoodIDs) == 0 {
		return nil
	}
	allowedNodes := workflowIDSetV2(selection.NodeIDs, selection.NeighborhoodIDs)
	allowedEdges := workflowIDSetV2(selection.EdgeIDs)
	createdNodes := map[string]struct{}{}
	for _, operation := range operations {
		if value, ok := operation.Value.(*contract.AddNodeOperation); ok && strings.TrimSpace(value.ClientRef) != "" {
			createdNodes[value.ClientRef] = struct{}{}
		}
	}
	allowedNode := func(value string) bool {
		_, selected := allowedNodes[value]
		_, created := createdNodes[value]
		return selected || created
	}
	for _, operation := range operations {
		switch value := operation.Value.(type) {
		case *contract.AddNodeOperation:
			if strings.TrimSpace(value.ClientRef) == "" || (value.Node.ParentID != nil && !allowedNode(*value.Node.ParentID)) {
				return fmt.Errorf("add_node is outside the frozen turn selection")
			}
		case *contract.UpdateNodeOperation:
			if !allowedNode(value.NodeID) || (value.Set.ParentID != nil && !allowedNode(*value.Set.ParentID)) {
				return fmt.Errorf("node %q is outside the frozen turn selection", value.NodeID)
			}
		case *contract.DeleteNodeOperation:
			if !allowedNode(value.NodeID) {
				return fmt.Errorf("node %q is outside the frozen turn selection", value.NodeID)
			}
		case *contract.AddEdgeOperation:
			if strings.TrimSpace(value.ClientRef) == "" || value.Edge.Source == nil || value.Edge.Target == nil || !allowedNode(*value.Edge.Source) || !allowedNode(*value.Edge.Target) {
				return fmt.Errorf("add_edge is outside the frozen turn selection")
			}
		case *contract.UpdateEdgeOperation:
			if _, ok := allowedEdges[value.EdgeID]; !ok {
				return fmt.Errorf("edge %q is outside the frozen turn selection", value.EdgeID)
			}
			if (value.Set.Source != nil && !allowedNode(*value.Set.Source)) || (value.Set.Target != nil && !allowedNode(*value.Set.Target)) {
				return fmt.Errorf("edge %q targets outside the frozen turn selection", value.EdgeID)
			}
		case *contract.DeleteEdgeOperation:
			if _, ok := allowedEdges[value.EdgeID]; !ok {
				return fmt.Errorf("edge %q is outside the frozen turn selection", value.EdgeID)
			}
		case *contract.UpdateLayoutOperation:
			if value.Layout.Viewport != nil {
				return fmt.Errorf("viewport update requires entire-workflow scope")
			}
			for _, node := range value.Layout.Nodes {
				if !allowedNode(node.NodeID) {
					return fmt.Errorf("layout node %q is outside the frozen turn selection", node.NodeID)
				}
			}
		case *contract.UpdateWorkflowOperation, *contract.AttachManualBindingOperation:
			return fmt.Errorf("workflow-wide operation requires entire-workflow scope")
		default:
			return fmt.Errorf("unsupported workflow operation in frozen turn scope")
		}
	}
	return nil
}

func workflowIDSetV2(groups ...[]string) map[string]struct{} {
	result := map[string]struct{}{}
	for _, group := range groups {
		for _, value := range group {
			result[value] = struct{}{}
		}
	}
	return result
}

func workflowApplyToolResultV2(receipt contract.WorkflowMutationReceipt) tooling.ToolResult {
	receipt.Delta.AddedNodeIDs = nonNilWorkflowIDsV2(receipt.Delta.AddedNodeIDs)
	receipt.Delta.UpdatedNodeIDs = nonNilWorkflowIDsV2(receipt.Delta.UpdatedNodeIDs)
	receipt.Delta.DeletedNodeIDs = nonNilWorkflowIDsV2(receipt.Delta.DeletedNodeIDs)
	receipt.Delta.AddedEdgeIDs = nonNilWorkflowIDsV2(receipt.Delta.AddedEdgeIDs)
	receipt.Delta.UpdatedEdgeIDs = nonNilWorkflowIDsV2(receipt.Delta.UpdatedEdgeIDs)
	receipt.Delta.DeletedEdgeIDs = nonNilWorkflowIDsV2(receipt.Delta.DeletedEdgeIDs)
	receipt.Validation.Diagnostics = append([]contract.WorkflowDiagnostic{}, receipt.Validation.Diagnostics...)
	summary := "Workflow 修改已原子提交，正在执行提交后校验。"
	if receipt.Status != "verifying" {
		summary = "已返回持久化的 Workflow 修改回执，状态：" + receipt.Status + "。"
	}
	envelope := contract.NewToolResult(summary, receipt)
	raw, _ := json.Marshal(envelope)
	return tooling.ToolResult{
		Content:           summary,
		StructuredContent: raw,
		Display:           workflowMutationDisplayV2("workflow_patch_result", "Workflow patch result", raw),
		ResultBudget:      tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true},
	}
}

func nonNilWorkflowIDsV2(values []string) []string { return append([]string{}, values...) }

func WorkflowValidateToolV2(app *service.WorkflowEditApplication) tooling.Tool {
	if app == nil {
		return nil
	}
	schemas := WorkflowToolContractsV2()["workflow.validate"]
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name: "workflow.validate", Description: "校验当前 committed Workflow 或指定 immutable ChangeSet；Graph 与完整 ValidationBasis 均由服务端解析，不接受调用方注入。",
			Layer: tooling.ToolLayerCore, Pack: ToolPack, Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, AlwaysLoad: true, IsMCP: true,
			MCPInfo: tooling.MCPInfo{ServerID: ServerID, ServerName: "Runner Workflow Editor", ToolName: "workflow.validate"}, RiskLevel: tooling.ToolRiskLow,
			ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}, FailurePolicy: tooling.ToolFailurePolicyFeedBackToModel,
			Discovery: tooling.ToolDiscoveryMetadata{DiscoveryGroup: ToolPack, CapabilityKind: "runner", ResourceTypes: []string{"workflow_candidate", "workflow"}, OperationKinds: []string{"validate", "inspect"}, RiskLevel: tooling.ToolRiskLow, LoadingPolicy: tooling.ToolLoadingPolicyCore, AgentProfiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, ToolPackIDs: []string{ToolPack}, MCPServerID: ServerID, PermissionScope: "workflow.read", PromptBudgetClass: "compact", SchemaBudgetClass: "on_demand"},
		},
		InputSchemaData: schemas.InputSchema, OutputSchemaData: schemas.OutputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool { return true }, DestructiveFunc: func(json.RawMessage) bool { return false }, ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := decodeWorkflowValidateV2(input)
			return err
		},
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			request, err := decodeWorkflowValidateV2(input)
			if err == nil {
				_, err = trustedWorkflowScope(ctx, request.WorkflowKey)
			}
			if err == nil {
				_, err = bindWorkflowValidateToTurnScopeV2(ctx, request)
			}
			if err == nil && request.ChangeSetID != "" {
				err = bindWorkflowCandidateToTurnScopeV2(ctx, app, request.WorkflowKey, request.ChangeSetID)
			}
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "trusted workflow scope is unavailable"}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			request, err := decodeWorkflowValidateV2(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if _, err := trustedWorkflowScope(ctx, request.WorkflowKey); err != nil {
				return tooling.ToolResult{}, err
			}
			request, err = bindWorkflowValidateToTurnScopeV2(ctx, request)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if request.ChangeSetID != "" {
				if err := bindWorkflowCandidateToTurnScopeV2(ctx, app, request.WorkflowKey, request.ChangeSetID); err != nil {
					return tooling.ToolResult{}, err
				}
			}
			validation, err := app.ValidateChange(ctx, request)
			if err != nil {
				if result, typed := workflowMutationDomainErrorResultV2(err); typed {
					return result, nil
				}
				return tooling.ToolResult{}, err
			}
			return workflowValidationToolResultV2(validation), nil
		},
	}
}

func decodeWorkflowValidateV2(input json.RawMessage) (contract.ValidateRequest, error) {
	var request contract.ValidateRequest
	if err := contract.DecodeStrict(input, &request); err != nil {
		return request, err
	}
	if request.SchemaVersion != contract.SchemaVersion || request.WorkflowKey.Validate() != nil || (request.Revision != "" && request.ChangeSetID != "") || strings.TrimSpace(request.Revision) != request.Revision || strings.TrimSpace(request.ChangeSetID) != request.ChangeSetID {
		return request, fmt.Errorf("invalid workflow.validate request")
	}
	return request, nil
}

func workflowValidationToolResultV2(validation contract.ValidationResult) tooling.ToolResult {
	validation.Kind = "validation"
	validation.Diagnostics = append([]contract.WorkflowDiagnostic(nil), validation.Diagnostics...)
	if validation.Diagnostics == nil {
		validation.Diagnostics = []contract.WorkflowDiagnostic{}
	}
	summary := "Workflow 校验通过。"
	if !validation.Valid {
		summary = fmt.Sprintf("Workflow 校验未通过：发现 %d 项诊断。", len(validation.Diagnostics))
	}
	envelope := contract.NewToolResult(summary, validation)
	envelope.IsError = !validation.Valid
	raw, _ := json.Marshal(envelope)
	return tooling.ToolResult{
		Content:           summary,
		StructuredContent: raw,
		IsError:           !validation.Valid,
		Display:           workflowMutationDisplayV2("workflow_patch_validation", "Workflow patch validation", raw),
		ResultBudget:      tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true},
	}
}

func decodeWorkflowPreviewPatchV2(input json.RawMessage) (contract.PreviewPatchRequest, error) {
	var request contract.PreviewPatchRequest
	if err := contract.DecodeStrict(input, &request); err != nil {
		return request, err
	}
	if request.SchemaVersion != contract.SchemaVersion || request.WorkflowKey.Validate() != nil || request.Expected.ResourceVersion == "" || request.Expected.SemanticHash == "" || request.Expected.LayoutHash == "" || strings.TrimSpace(request.Intent) == "" || len(request.Operations) == 0 || len(request.Operations) > 500 {
		return request, fmt.Errorf("invalid workflow.preview_patch request")
	}
	if err := request.ValidationBasis.Validate(); err != nil || request.ValidationBasis.ResourceVersion != request.Expected.ResourceVersion || request.ValidationBasis.SemanticHash != request.Expected.SemanticHash {
		return request, fmt.Errorf("invalid workflow preview validation basis")
	}
	for _, operation := range request.Operations {
		if err := operation.Validate(); err != nil {
			return request, fmt.Errorf("invalid workflow preview operation: %w", err)
		}
	}
	return request, nil
}

func workflowPreviewCandidateIDV2(scope trustedWorkflowScopeV2, toolCallID string) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{scope.Key.TenantID, scope.Key.WorkflowID, scope.UserID, scope.SessionID, toolCallID}, "\x00")))
	return "candidate-" + hex.EncodeToString(digest[:12])
}

func workflowPreviewToolResultV2(preview service.WorkflowEditPreviewResult) tooling.ToolResult {
	detail := workflowPreviewDetailV2{CandidateRevision: preview.CandidateRevision, PreviewHash: preview.PreviewHash, ValidationBasis: preview.ValidationBasis, Diagnostics: append([]contract.WorkflowDiagnostic(nil), preview.Diagnostics...), Risk: preview.Risk, Diff: preview.Diff, Rebased: preview.Rebased}
	diff, _ := json.Marshal(detail)
	value := contract.ChangeSetPreview{Kind: "preview", ChangeSet: preview.ChangeSet, Candidate: preview.Candidate, Diff: diff}
	failed := !preview.ChangeSet.Validation.Valid
	summary := fmt.Sprintf("已生成候选预览：%d 项操作，风险 %s。", len(preview.ChangeSet.Operations), preview.Risk.Level)
	if failed {
		summary = fmt.Sprintf("候选预览存在 %d 项待修复诊断，committed Workflow 未改变。", len(preview.Diagnostics))
	}
	envelope := contract.NewToolResult(summary, value)
	envelope.IsError = failed
	if actions := workflowPreviewArtifactActionsV2(preview); len(actions) > 0 {
		envelope.Meta = map[string]any{"actions": actions}
	}
	raw, _ := json.Marshal(envelope)
	return tooling.ToolResult{
		Content:           summary,
		StructuredContent: raw,
		IsError:           failed,
		Display:           workflowMutationDisplayV2("workflow_patch_preview", "Workflow patch preview", raw),
		ResultBudget:      tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true},
	}
}

func workflowPreviewArtifactActionsV2(preview service.WorkflowEditPreviewResult) []workflowPreviewArtifactActionV2 {
	changeSet, candidate := preview.ChangeSet, preview.Candidate
	if changeSet.Key.Validate() != nil || candidate.Key != changeSet.Key || !workflowActionStringV2(candidate.ID) || candidate.ID != changeSet.CandidateID ||
		!workflowActionStringV2(changeSet.ID) || candidate.CurrentChangeSetID != changeSet.ID || !workflowActionStringV2(changeSet.PreviewHash) ||
		preview.PreviewHash != changeSet.PreviewHash || candidate.CurrentPreviewHash != changeSet.PreviewHash || candidate.CurrentRevision != changeSet.CandidateRevision ||
		!workflowActionStringV2(changeSet.Base.ResourceVersion) {
		return nil
	}
	binding := workflowPreviewArtifactActionTargetV2{
		CandidateID: candidate.ID, ChangeSetID: changeSet.ID, PreviewHash: changeSet.PreviewHash, ExpectedRevision: changeSet.Base.ResourceVersion,
	}
	actions := make([]workflowPreviewArtifactActionV2, 0, len(changeSet.Operations)+1)
	for index, operation := range changeSet.Operations {
		nodeID, edgeID, ok := workflowPreviewOperationTargetV2(operation)
		if !ok {
			continue
		}
		operationIndex := index
		target := binding
		target.OperationIndex, target.NodeID, target.EdgeID = &operationIndex, nodeID, edgeID
		label := "定位节点"
		if edgeID != "" {
			label = "定位连线"
		}
		actions = append(actions, workflowPreviewArtifactActionV2{
			ID: fmt.Sprintf("workflow-focus-operation-%d", index), Intent: "workflow.focus_object", Label: label, Mutation: false, Target: target,
		})
	}
	actions = append(actions, workflowPreviewArtifactActionV2{
		ID: "workflow-review-change", Intent: "workflow.review_change", Label: "审阅修改", Mutation: false, Target: binding,
	})
	return actions
}

func workflowPreviewOperationTargetV2(operation contract.WorkflowOperation) (nodeID, edgeID string, ok bool) {
	switch value := operation.Value.(type) {
	case *contract.AddNodeOperation:
		nodeID = value.Node.ID
		if nodeID == "" {
			nodeID = value.ClientRef
		}
	case *contract.UpdateNodeOperation:
		nodeID = value.NodeID
	case *contract.DeleteNodeOperation:
		nodeID = value.NodeID
	case *contract.AddEdgeOperation:
		edgeID = value.Edge.ID
		if edgeID == "" {
			edgeID = value.ClientRef
		}
	case *contract.UpdateEdgeOperation:
		edgeID = value.EdgeID
	case *contract.DeleteEdgeOperation:
		edgeID = value.EdgeID
	case *contract.UpdateLayoutOperation:
		if len(value.Layout.Nodes) != 1 {
			return "", "", false
		}
		nodeID = value.Layout.Nodes[0].NodeID
	default:
		return "", "", false
	}
	if (nodeID == "") == (edgeID == "") || !workflowActionStringV2(nodeID+edgeID) {
		return "", "", false
	}
	return nodeID, edgeID, true
}

func workflowActionStringV2(value string) bool {
	return value != "" && value == strings.TrimSpace(value)
}

func workflowMutationDisplayV2(displayKind, title string, envelope json.RawMessage) *tooling.ToolDisplayPayload {
	return &tooling.ToolDisplayPayload{
		Type:  displayKind,
		Title: title,
		Data:  append(json.RawMessage(nil), envelope...),
	}
}

func workflowMutationDomainErrorResultV2(err error) (tooling.ToolResult, bool) {
	var domain *contract.DomainError
	if !errors.As(err, &domain) || domain == nil {
		return tooling.ToolResult{}, false
	}
	value := *domain
	value.Kind = "error"
	summary := value.Message
	if value.RetryHint != "" {
		summary += "；" + value.RetryHint
	}
	envelope := contract.NewToolError(summary, value)
	raw, _ := json.Marshal(envelope)
	result := tooling.ToolResult{Content: summary, StructuredContent: raw, IsError: true, ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: 64 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}}
	if workflowMutationDomainErrorIsConflictV2(value) {
		result.Display = workflowMutationDisplayV2("workflow_conflict", "Workflow conflict", raw)
	}
	return result, true
}

func workflowMutationDomainErrorIsConflictV2(domain contract.DomainError) bool {
	if domain.Code != contract.ErrorStaleRevision || domain.Key == nil || domain.Key.Validate() != nil || len(domain.Details) == 0 {
		return false
	}
	var conflict contract.WorkflowConflict
	if err := contract.DecodeStrict(domain.Details, &conflict); err != nil || conflict.Code != contract.ErrorStaleRevision || conflict.ChangedBy != domain.ChangedBy {
		return false
	}
	if !workflowConflictRevisionBindingV2(domain.BaseRevision, conflict.Base.ResourceVersion) || !workflowConflictRevisionBindingV2(domain.CurrentRevision, conflict.Current.ResourceVersion) || domain.BaseRevision == domain.CurrentRevision {
		return false
	}
	return workflowConflictPathsMatchV2(domain.ConflictingPaths, conflict.ConflictingPaths) && workflowConflictPathsValidV2(conflict.SafeRebasePaths, false)
}

func workflowConflictRevisionBindingV2(domainRevision, detailRevision string) bool {
	return domainRevision != "" && domainRevision == strings.TrimSpace(domainRevision) && domainRevision == detailRevision
}

func workflowConflictPathsMatchV2(domainPaths, detailPaths []string) bool {
	if len(domainPaths) != len(detailPaths) || !workflowConflictPathsValidV2(detailPaths, true) {
		return false
	}
	for index := range domainPaths {
		if domainPaths[index] != detailPaths[index] {
			return false
		}
	}
	return true
}

func workflowConflictPathsValidV2(paths []string, required bool) bool {
	if required && len(paths) == 0 {
		return false
	}
	seen := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		if path == "" || path != strings.TrimSpace(path) || !strings.HasPrefix(path, "/") {
			return false
		}
		if _, duplicate := seen[path]; duplicate {
			return false
		}
		seen[path] = struct{}{}
	}
	return true
}
