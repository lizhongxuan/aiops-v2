package workfloweditor

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimecontract"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	contract "runner/workflow/edit"
)

const workflowInlineResultBytesV2 = 8 << 10

// The current workflow identity is deliberately absent from the model-facing
// read inputs. Tenant/workflow scope is server-owned and is injected from the
// authenticated execution context (preferably its frozen turn artifact).
type workflowGetContextInputV2 struct {
	SchemaVersion         string   `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	TurnContextArtifactID string   `json:"turnContextArtifactId,omitempty"`
	Revision              string   `json:"revision,omitempty"`
	View                  string   `json:"view" jsonschema:"enum=summary,enum=selection,enum=neighborhood,enum=full"`
	NodeIDs               []string `json:"nodeIds,omitempty"`
	EdgeDepth             int      `json:"edgeDepth,omitempty"`
	Include               []string `json:"include,omitempty"`
	Cursor                string   `json:"cursor,omitempty"`
	Limit                 int      `json:"limit,omitempty"`
}

type workflowActionCatalogInputV2 struct {
	SchemaVersion   string   `json:"schemaVersion" jsonschema:"enum=aiops.workflow.edit.v2"`
	Names           []string `json:"names,omitempty"`
	Query           string   `json:"query,omitempty"`
	Category        string   `json:"category,omitempty"`
	NodeType        string   `json:"nodeType,omitempty"`
	RiskMax         string   `json:"riskMax,omitempty"`
	IncludeSchema   bool     `json:"includeSchema,omitempty"`
	IncludeExamples bool     `json:"includeExamples,omitempty"`
	Cursor          string   `json:"cursor,omitempty"`
	Limit           int      `json:"limit,omitempty"`
}

func WorkflowReadToolsV2(app *service.WorkflowEditApplication, resources *WorkflowResourcesV2) []tooling.Tool {
	if app == nil || resources == nil {
		return nil
	}
	return []tooling.Tool{
		newWorkflowReadToolV2("workflow.get_context", func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			request, err := decodeGetContextV2(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			scope, err := trustedCurrentWorkflowScope(ctx)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			request.WorkflowKey = scope.Key
			if err := bindGetContextRequestToTurnArtifact(ctx, &request); err != nil {
				return tooling.ToolResult{}, err
			}
			query := service.WorkflowContextQuery{View: service.WorkflowContextView(request.View), Selection: service.WorkflowContextSelection{Revision: contract.Revision{ResourceVersion: request.Revision}, NodeIDs: append([]string(nil), request.NodeIDs...)}, EdgeDepth: request.EdgeDepth, Cursor: request.Cursor, Limit: request.Limit, BudgetBytes: 256 << 10}
			contextResult, err := app.GetContext(ctx, scope.Key, query)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			ref, err := resources.BindContext(WorkflowContextResourceV2{Key: scope.Key, ActorID: scope.UserID, SessionID: scope.SessionID, Revision: contextResult.Revision.ResourceVersion, Query: query})
			if err != nil {
				return tooling.ToolResult{}, err
			}
			value := workflowContextContract(contextResult, ref.URI, true)
			if len(value.Graph) > workflowInlineResultBytesV2 {
				value.Graph = nil
			}
			summary := fmt.Sprintf("已读取工作流：%d 个节点、%d 条连线，版本 %s。", contextResult.Summary.NodeCount, contextResult.Summary.EdgeCount, contextResult.Revision.ResourceVersion)
			if contextResult.Selection.Stale {
				summary += " 当前选区基于旧版本，请刷新后再修改。"
			}
			return workflowToolResultV2(summary, value, ref), nil
		}),
		newWorkflowReadToolV2("workflow.get_action_catalog", func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			request, err := decodeActionCatalogV2(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			scope, err := trustedCurrentWorkflowScope(ctx)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			request.WorkflowKey = scope.Key
			page, err := readWorkflowActionCatalog(ctx, app, request)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			ref, err := resources.BindCatalog(WorkflowCatalogResourceV2{Key: scope.Key, ActorID: scope.UserID, SessionID: scope.SessionID, CatalogVersion: page.CatalogVersion, Request: request})
			if err != nil {
				return tooling.ToolResult{}, err
			}
			summary := fmt.Sprintf("已匹配 %d 个可用步骤，目录版本 %s。", len(page.Entries), page.CatalogVersion)
			inlinePage := page
			if raw, _ := json.Marshal(page); len(raw) > workflowInlineResultBytesV2 {
				inlinePage.Entries = []contract.ActionCatalogEntry{}
			}
			return workflowToolResultV2(summary, inlinePage, ref), nil
		}),
	}
}

func newWorkflowReadToolV2(name string, execute func(context.Context, json.RawMessage) (tooling.ToolResult, error)) tooling.Tool {
	contractSchema := WorkflowToolContractsV2()[name]
	return &tooling.StaticTool{Meta: tooling.ToolMetadata{Name: name, Description: workflowReadDescriptionV2(name), Layer: tooling.ToolLayerCore, Pack: ToolPack, Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, AlwaysLoad: true, IsMCP: true, MCPInfo: tooling.MCPInfo{ServerID: ServerID, ServerName: "Runner Workflow Editor", ToolName: name}, RiskLevel: tooling.ToolRiskLow, ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: workflowInlineResultBytesV2, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}, FailurePolicy: tooling.ToolFailurePolicyFeedBackToModel, Discovery: tooling.ToolDiscoveryMetadata{DiscoveryGroup: ToolPack, CapabilityKind: "runner", ResourceTypes: []string{"workflow"}, OperationKinds: []string{"read"}, RiskLevel: tooling.ToolRiskLow, LoadingPolicy: tooling.ToolLoadingPolicyCore, AgentProfiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, ToolPackIDs: []string{ToolPack}, MCPServerID: ServerID, PermissionScope: "workflow.read", PromptBudgetClass: "compact", SchemaBudgetClass: "on_demand"}}, InputSchemaData: contractSchema.InputSchema, OutputSchemaData: contractSchema.OutputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool { return true }, DestructiveFunc: func(json.RawMessage) bool { return false }, ConcurrencySafeFunc: func(json.RawMessage) bool { return true }, ExecuteFunc: execute,
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error { return validateWorkflowReadInputV2(name, input) },
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			err := validateWorkflowReadInputV2(name, input)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "invalid workflow read request"}
			}
			_, err = trustedCurrentWorkflowScope(ctx)
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "trusted workflow scope is unavailable"}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		}}
}

func workflowReadDescriptionV2(name string) string {
	if name == "workflow.get_context" {
		return "读取当前授权 Workflow 的版本、选区、邻域或分页 Graph；修改前先读取，禁止用于选择其他租户资源。"
	}
	return "读取当前授权 Workflow 可使用的真实 Action schema、端口、风险与示例；找不到能力时重新缩小查询。"
}

type trustedWorkflowScopeV2 struct {
	Key             contract.WorkflowKey
	UserID          string
	SessionID       string
	CanvasSessionID string
}

func trustedWorkflowScope(ctx context.Context, claimed contract.WorkflowKey) (trustedWorkflowScopeV2, error) {
	scope, err := trustedCurrentWorkflowScope(ctx)
	if err != nil || claimed.Validate() != nil || claimed != scope.Key {
		return trustedWorkflowScopeV2{}, fmt.Errorf("trusted workflow scope is unavailable")
	}
	return scope, nil
}

func trustedCurrentWorkflowScope(ctx context.Context) (trustedWorkflowScopeV2, error) {
	execution, ok := tooling.ToolExecutionContextFrom(ctx)
	if artifact, found := runtimecontract.TurnArtifactFromContext(ctx, appui.WorkflowTurnContextArtifactKind); found {
		frozen, err := appui.DecodeWorkflowTurnContext(artifact)
		if err != nil {
			return trustedWorkflowScopeV2{}, fmt.Errorf("trusted workflow scope is unavailable")
		}
		canvasSessionID := strings.TrimSpace(frozen.Scope.CanvasSessionID)
		if canvasSessionID == "" {
			// Artifacts persisted before Canvas/Runtime identity separation used
			// the runtime session for both roles. Keep those reads replayable;
			// every new Chat artifact carries the server-owned Canvas identity.
			canvasSessionID = frozen.Scope.SessionID
		}
		if ok && ((execution.TenantID != "" && execution.TenantID != frozen.WorkflowKey.TenantID) || (execution.UserID != "" && execution.UserID != frozen.Scope.ActorID) || (execution.SessionID != "" && execution.SessionID != frozen.Scope.SessionID)) {
			return trustedWorkflowScopeV2{}, fmt.Errorf("trusted workflow scope is unavailable")
		}
		return trustedWorkflowScopeV2{Key: frozen.WorkflowKey, UserID: frozen.Scope.ActorID, SessionID: frozen.Scope.SessionID, CanvasSessionID: canvasSessionID}, nil
	}
	workflowID := strings.TrimSpace(execution.Metadata["workflowId"])
	key := contract.WorkflowKey{TenantID: execution.TenantID, WorkflowID: workflowID}
	if !ok || execution.TenantID != strings.TrimSpace(execution.TenantID) || execution.UserID == "" || execution.UserID != strings.TrimSpace(execution.UserID) || workflowID == "" || key.Validate() != nil {
		return trustedWorkflowScopeV2{}, fmt.Errorf("trusted workflow scope is unavailable")
	}
	canvasSessionID := strings.TrimSpace(execution.Metadata["workflowCanvasSessionId"])
	if canvasSessionID == "" {
		canvasSessionID = execution.SessionID
	}
	return trustedWorkflowScopeV2{Key: key, UserID: execution.UserID, SessionID: execution.SessionID, CanvasSessionID: canvasSessionID}, nil
}

func bindGetContextRequestToTurnArtifact(ctx context.Context, request *contract.GetContextRequest) error {
	artifact, found := runtimecontract.TurnArtifactFromContext(ctx, appui.WorkflowTurnContextArtifactKind)
	if !found {
		if request.TurnContextArtifactID != "" {
			return fmt.Errorf("trusted workflow turn artifact is unavailable")
		}
		return nil
	}
	frozen, err := appui.DecodeWorkflowTurnContext(artifact)
	if err != nil || (request.TurnContextArtifactID != "" && request.TurnContextArtifactID != artifact.ID) {
		return fmt.Errorf("trusted workflow turn artifact does not match the request")
	}
	request.WorkflowKey = frozen.WorkflowKey
	if request.Revision == "" {
		request.Revision = frozen.Revision.ResourceVersion
	} else if request.Revision != frozen.Revision.ResourceVersion {
		return fmt.Errorf("workflow turn revision is stale; refresh or rebase before continuing")
	}
	allowedNodes := append(append([]string(nil), frozen.Selection.NodeIDs...), frozen.Selection.NeighborhoodIDs...)
	if len(request.NodeIDs) == 0 && (request.View == string(service.WorkflowContextSelectionView) || request.View == string(service.WorkflowContextNeighborhoodView)) {
		request.NodeIDs = append([]string(nil), frozen.Selection.NodeIDs...)
	}
	if !workflowIDsWithinFrozenScope(request.NodeIDs, allowedNodes) {
		return fmt.Errorf("requested workflow nodes are outside the frozen turn selection")
	}
	return nil
}

func workflowIDsWithinFrozenScope(requested, allowed []string) bool {
	if len(requested) == 0 {
		return true
	}
	set := make(map[string]struct{}, len(allowed))
	for _, value := range allowed {
		set[value] = struct{}{}
	}
	for _, value := range requested {
		if _, ok := set[value]; !ok {
			return false
		}
	}
	return true
}

func decodeGetContextV2(input json.RawMessage) (contract.GetContextRequest, error) {
	var modelInput workflowGetContextInputV2
	if err := contract.DecodeStrict(input, &modelInput); err != nil {
		return contract.GetContextRequest{}, err
	}
	request := contract.GetContextRequest{SchemaVersion: modelInput.SchemaVersion, TurnContextArtifactID: modelInput.TurnContextArtifactID, Revision: modelInput.Revision, View: modelInput.View, NodeIDs: modelInput.NodeIDs, EdgeDepth: modelInput.EdgeDepth, Include: modelInput.Include, Cursor: modelInput.Cursor, Limit: modelInput.Limit}
	if request.SchemaVersion != contract.SchemaVersion || request.Limit < 0 || request.Limit > 200 || request.EdgeDepth < 0 || request.EdgeDepth > 5 {
		return request, fmt.Errorf("invalid workflow.get_context request")
	}
	switch service.WorkflowContextView(request.View) {
	case service.WorkflowContextSummaryView, service.WorkflowContextSelectionView, service.WorkflowContextNeighborhoodView, service.WorkflowContextFullView:
	default:
		return request, fmt.Errorf("invalid workflow context view")
	}
	return request, nil
}

func decodeActionCatalogV2(input json.RawMessage) (contract.ActionCatalogRequest, error) {
	var modelInput workflowActionCatalogInputV2
	if err := contract.DecodeStrict(input, &modelInput); err != nil {
		return contract.ActionCatalogRequest{}, err
	}
	request := contract.ActionCatalogRequest{SchemaVersion: modelInput.SchemaVersion, Names: modelInput.Names, Query: modelInput.Query, Category: modelInput.Category, NodeType: modelInput.NodeType, RiskMax: modelInput.RiskMax, IncludeSchema: modelInput.IncludeSchema, IncludeExamples: modelInput.IncludeExamples, Cursor: modelInput.Cursor, Limit: modelInput.Limit}
	if request.SchemaVersion != contract.SchemaVersion || request.Limit < 0 || request.Limit > 200 || (request.RiskMax != "" && workflowRiskRankV2(request.RiskMax) < 0) {
		return request, fmt.Errorf("invalid workflow.get_action_catalog request")
	}
	return request, nil
}

func validateWorkflowReadInputV2(name string, input json.RawMessage) error {
	if name == "workflow.get_context" {
		_, err := decodeGetContextV2(input)
		return err
	}
	_, err := decodeActionCatalogV2(input)
	return err
}

func workflowContextContract(result service.WorkflowContextResult, uri string, includeGraph bool) contract.WorkflowContext {
	value := contract.WorkflowContext{Kind: "context", SchemaVersion: contract.SchemaVersion, WorkflowKey: result.Key, Revision: result.Revision, ResourceURI: uri, SelectionContextStale: result.Selection.Stale, NextCursor: result.Graph.NextCursor, Validation: contract.ValidationResult{Kind: "validation", Basis: contract.ValidationBasis{ResourceVersion: result.Revision.ResourceVersion, SemanticHash: result.Revision.SemanticHash}, Valid: result.Validation != nil && result.Validation.Validation.Valid, Diagnostics: []contract.WorkflowDiagnostic{}}}
	if includeGraph {
		value.Graph, _ = json.Marshal(result.Graph)
	}
	if result.Validation != nil {
		for _, issue := range append(append([]service.VisualWorkflowIssue(nil), result.Validation.Validation.Errors...), result.Validation.Validation.Warnings...) {
			value.Validation.Diagnostics = append(value.Validation.Diagnostics, contract.WorkflowDiagnostic{Code: issue.Code, Severity: issue.Severity, Message: issue.Message, NodeID: issue.NodeID, EdgeID: issue.EdgeID, Field: issue.Field})
		}
	}
	return value
}

func readWorkflowActionCatalog(ctx context.Context, app *service.WorkflowEditApplication, request contract.ActionCatalogRequest) (contract.ActionCatalogPage, error) {
	includeSchema := request.IncludeSchema
	page, err := app.GetActionCatalog(ctx, request.WorkflowKey, service.WorkflowActionCatalogQuery{Names: request.Names, Category: request.Category, IncludeSchema: &includeSchema, Cursor: request.Cursor, Limit: request.Limit})
	if err != nil {
		return contract.ActionCatalogPage{}, err
	}
	result := contract.ActionCatalogPage{Kind: "action_catalog", SchemaVersion: contract.SchemaVersion, CatalogVersion: page.CatalogDigest, Entries: []contract.ActionCatalogEntry{}, NextCursor: page.NextCursor}
	for _, item := range page.Items {
		if !item.Access.Allowed || !workflowActionMatchesV2(request, item.Spec) {
			continue
		}
		spec := item.Spec
		if !request.IncludeExamples {
			spec.Examples, spec.InputExamples, spec.OutputExamples = nil, nil, nil
		}
		raw, err := json.Marshal(spec)
		if err != nil {
			return contract.ActionCatalogPage{}, err
		}
		sum := sha256.Sum256(raw)
		result.Entries = append(result.Entries, contract.ActionCatalogEntry{Name: spec.Action, Digest: fmt.Sprintf("sha256:%x", sum), Spec: raw})
	}
	return result, nil
}

func workflowActionMatchesV2(request contract.ActionCatalogRequest, spec service.ActionSpec) bool {
	query := strings.ToLower(strings.TrimSpace(request.Query))
	if query != "" && !strings.Contains(strings.ToLower(spec.Action+" "+spec.Title+" "+spec.Description), query) {
		return false
	}
	if request.NodeType != "" && !strings.EqualFold(request.NodeType, spec.NodeType) {
		return false
	}
	rank := workflowRiskRankV2(spec.Risk)
	return request.RiskMax == "" || (rank >= 0 && rank <= workflowRiskRankV2(request.RiskMax))
}

func workflowRiskRankV2(value string) int {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "low":
		return 0
	case "medium":
		return 1
	case "high":
		return 2
	case "critical":
		return 3
	default:
		return -1
	}
}

func workflowToolResultV2[T any](summary string, value T, ref tooling.ResultReference) tooling.ToolResult {
	envelope := contract.NewToolResult(summary, value)
	raw, _ := json.Marshal(envelope)
	return tooling.ToolResult{Content: summary, StructuredContent: raw, References: []tooling.ResultReference{ref}, ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: workflowInlineResultBytesV2, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}}
}
