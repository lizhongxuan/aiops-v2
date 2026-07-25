package workfloweditor

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	contract "runner/workflow/edit"
	"runner/workflow/visual"
)

type WorkflowCompilePlanRequestV2 struct {
	SchemaVersion       string                  `json:"schemaVersion" jsonschema:"enum=aiops.workflow.plan.v1"`
	WorkflowKey         contract.WorkflowKey    `json:"workflowKey"`
	Plan                contract.WorkflowPlanIR `json:"plan"`
	EnvironmentVersions map[string]string       `json:"environmentVersions,omitempty"`
	PositionSpacing     float64                 `json:"positionSpacing,omitempty"`
}

type WorkflowCompilePlanResultV2 struct {
	Kind           string                            `json:"kind" jsonschema:"enum=compile_plan"`
	SchemaVersion  string                            `json:"schemaVersion" jsonschema:"enum=aiops.workflow.plan.v1"`
	WorkflowKey    contract.WorkflowKey              `json:"workflowKey"`
	CatalogVersion string                            `json:"catalogVersion"`
	PlanHash       string                            `json:"planHash"`
	Operations     []contract.WorkflowOperation      `json:"operations"`
	Diagnostics    []contract.WorkflowPlanDiagnostic `json:"diagnostics"`
}

func WorkflowCompilePlanToolV2(app *service.WorkflowEditApplication) tooling.Tool {
	if app == nil {
		return nil
	}
	schemas := WorkflowToolContractsV2()["workflow.compile_plan"]
	return &tooling.StaticTool{
		Meta:            tooling.ToolMetadata{Name: "workflow.compile_plan", Description: "把已校验的 WorkflowPlanIR 通过当前 Runner Action Catalog 确定性编译为 typed operations；不写入 Workflow，诊断未解决时不得 preview。", Layer: tooling.ToolLayerCore, Pack: ToolPack, Profiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, AlwaysLoad: true, IsMCP: true, MCPInfo: tooling.MCPInfo{ServerID: ServerID, ServerName: "Runner Workflow Editor", ToolName: "workflow.compile_plan"}, RiskLevel: tooling.ToolRiskLow, ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: 32 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}, FailurePolicy: tooling.ToolFailurePolicyFeedBackToModel, Discovery: tooling.ToolDiscoveryMetadata{DiscoveryGroup: ToolPack, CapabilityKind: "runner", ResourceTypes: []string{"workflow_plan", "workflow"}, OperationKinds: []string{"compile", "read"}, RiskLevel: tooling.ToolRiskLow, LoadingPolicy: tooling.ToolLoadingPolicyCore, AgentProfiles: []string{runtimekernel.RuntimePromptProfileWorkflowAgent}, ToolPackIDs: []string{ToolPack}, MCPServerID: ServerID, PermissionScope: "workflow.read", PromptBudgetClass: "compact", SchemaBudgetClass: "on_demand"}},
		InputSchemaData: schemas.InputSchema, OutputSchemaData: schemas.OutputSchema,
		ReadOnlyFunc: func(json.RawMessage) bool { return true }, DestructiveFunc: func(json.RawMessage) bool { return false }, ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := decodeWorkflowCompilePlanV2(input)
			return err
		},
		CheckPermissionsFunc: func(ctx context.Context, input json.RawMessage) tooling.PermissionDecision {
			request, err := decodeWorkflowCompilePlanV2(input)
			if err == nil {
				_, err = trustedWorkflowScope(ctx, request.WorkflowKey)
			}
			if err != nil {
				return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "trusted workflow scope is unavailable"}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			request, err := decodeWorkflowCompilePlanV2(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			scope, err := trustedWorkflowScope(ctx, request.WorkflowKey)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if err := request.Plan.Validate(); err != nil {
				diagnostic := contract.WorkflowPlanDiagnostic{Code: contract.PlanDiagnosticUnresolved, Severity: "error", Message: "WorkflowPlanIR is invalid: " + err.Error(), Path: "/plan", Suggestion: "repair the typed plan and compile it again"}
				value := WorkflowCompilePlanResultV2{Kind: "compile_plan", SchemaVersion: contract.WorkflowPlanIRSchemaVersion, WorkflowKey: scope.Key, Operations: []contract.WorkflowOperation{}, Diagnostics: []contract.WorkflowPlanDiagnostic{diagnostic}}
				return workflowCompileToolResultV2(value), nil
			}
			catalog, err := workflowPlanCatalogV2(ctx, app, scope.Key, request.Plan)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			compiled, err := contract.CompileWorkflowPlan(request.Plan, catalog, contract.WorkflowPlanCompileOptions{EnvironmentVersions: request.EnvironmentVersions, PositionSpacing: request.PositionSpacing})
			if err != nil {
				return tooling.ToolResult{}, err
			}
			normalizeWorkflowCompileCollectionsV2(compiled.Operations)
			if compiled.Diagnostics == nil {
				compiled.Diagnostics = []contract.WorkflowPlanDiagnostic{}
			}
			value := WorkflowCompilePlanResultV2{Kind: "compile_plan", SchemaVersion: contract.WorkflowPlanIRSchemaVersion, WorkflowKey: scope.Key, CatalogVersion: catalog.Version, PlanHash: compiled.PlanHash, Operations: compiled.Operations, Diagnostics: compiled.Diagnostics}
			return workflowCompileToolResultV2(value), nil
		},
	}
}

func normalizeWorkflowCompileCollectionsV2(operations []contract.WorkflowOperation) {
	for _, operation := range operations {
		addNode, ok := operation.Value.(*contract.AddNodeOperation)
		if !ok || addNode.Node.Step == nil {
			continue
		}
		step := addNode.Node.Step
		if step.Targets == nil {
			step.Targets = []string{}
		}
		if step.MustVars == nil {
			step.MustVars = []string{}
		}
		if step.Loop == nil {
			step.Loop = []any{}
		}
		if step.ExpectVars == nil {
			step.ExpectVars = []string{}
		}
		if step.Notify == nil {
			step.Notify = []string{}
		}
	}
}

func decodeWorkflowCompilePlanV2(input json.RawMessage) (WorkflowCompilePlanRequestV2, error) {
	var request WorkflowCompilePlanRequestV2
	if err := contract.DecodeStrict(input, &request); err != nil {
		return request, err
	}
	if request.SchemaVersion != contract.WorkflowPlanIRSchemaVersion || request.WorkflowKey.Validate() != nil || request.PositionSpacing < 0 || request.PositionSpacing > 2000 {
		return request, fmt.Errorf("invalid workflow.compile_plan request")
	}
	for name, version := range request.EnvironmentVersions {
		if name == "" || name != strings.TrimSpace(name) || version == "" || version != strings.TrimSpace(version) {
			return request, fmt.Errorf("invalid workflow environment version binding")
		}
	}
	return request, nil
}

func workflowPlanCatalogV2(ctx context.Context, app *service.WorkflowEditApplication, key contract.WorkflowKey, plan contract.WorkflowPlanIR) (contract.WorkflowPlanCatalog, error) {
	names := map[string]bool{}
	for _, item := range plan.Items {
		if item.Action != nil {
			names[item.Action.CatalogID] = true
		}
	}
	requested := make([]string, 0, len(names))
	for name := range names {
		requested = append(requested, name)
	}
	sort.Strings(requested)
	includeSchema, cursor := true, ""
	result := contract.WorkflowPlanCatalog{}
	seenCursors := map[string]bool{}
	for {
		page, err := app.GetActionCatalog(ctx, key, service.WorkflowActionCatalogQuery{Names: requested, IncludeSchema: &includeSchema, Cursor: cursor, Limit: 200})
		if err != nil {
			return contract.WorkflowPlanCatalog{}, err
		}
		if result.Version == "" {
			result.Version = page.CatalogDigest
		} else if result.Version != page.CatalogDigest {
			return contract.WorkflowPlanCatalog{}, fmt.Errorf("workflow Action Catalog changed during compile")
		}
		for _, item := range page.Items {
			if !item.Access.Allowed {
				continue
			}
			arguments := item.Spec.ArgsSchema
			if len(arguments) == 0 {
				arguments = item.Spec.InputSchema
			}
			action := contract.WorkflowPlanCatalogAction{ID: item.Spec.Action, Version: page.CatalogDigest, NodeType: visual.NodeType(item.Spec.NodeType), ArgumentsJSON: append(json.RawMessage(nil), arguments...)}
			for _, port := range item.Spec.DefaultPorts.Inputs {
				action.InputPorts = append(action.InputPorts, contract.WorkflowPlanCatalogPort{ID: port.ID})
			}
			for _, port := range item.Spec.DefaultPorts.Outputs {
				action.OutputPorts = append(action.OutputPorts, contract.WorkflowPlanCatalogPort{ID: port.ID})
			}
			result.Actions = append(result.Actions, action)
		}
		if page.NextCursor == "" {
			break
		}
		if seenCursors[page.NextCursor] {
			return contract.WorkflowPlanCatalog{}, fmt.Errorf("workflow Action Catalog returned a repeated cursor")
		}
		seenCursors[page.NextCursor], cursor = true, page.NextCursor
	}
	sort.Slice(result.Actions, func(i, j int) bool { return result.Actions[i].ID < result.Actions[j].ID })
	return result, nil
}

func workflowCompileToolResultV2(value WorkflowCompilePlanResultV2) tooling.ToolResult {
	failed := false
	for _, diagnostic := range value.Diagnostics {
		failed = failed || diagnostic.Severity == "error"
	}
	summary := fmt.Sprintf("计划已确定性编译为 %d 项操作。", len(value.Operations))
	if failed {
		summary = fmt.Sprintf("计划存在 %d 项待修复诊断，未生成可预览操作。", len(value.Diagnostics))
		value.Operations = []contract.WorkflowOperation{}
	}
	envelope := contract.NewToolResult(summary, value)
	envelope.IsError = failed
	raw, _ := json.Marshal(envelope)
	return tooling.ToolResult{Content: summary, StructuredContent: raw, IsError: failed, ResultBudget: tooling.ResultBudget{MaxInlineResultBytes: 32 << 10, SpillPolicy: tooling.ResultSpillPolicyExternalize, SummarizeLargeResult: true}}
}
