package workfloweditor

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/tooling"
	edit "aiops-v2/internal/workfloweditor"
)

const (
	ServerID           = "runner"
	ToolPack           = "workflow_editor"
	workflowToolSchema = `{"type":"object"}`
)

var readOnlyTools = map[string]bool{
	"workflow.list":                         true,
	"workflow.get_snapshot":                 true,
	"workflow.get_step":                     true,
	"workflow.get_action_catalog":           true,
	"workflow.get_manual_binding":           true,
	"workflow.describe":                     true,
	"workflow.diff":                         true,
	"workflow.get_ai_session":               true,
	"workflow.propose_edit_plan":            true,
	"workflow.propose_patch":                true,
	"workflow.validate_patch":               true,
	"workflow.preview_patch":                true,
	"workflow.detect_patch_effect":          true,
	"workflow.propose_ops_manual_candidate": true,
	"workflow.propose_ops_manual_update":    true,
}

var mutatingTools = map[string]bool{
	"workflow.apply_patch":                      true,
	"workflow.save_draft":                       true,
	"workflow.attach_manual_candidate":          true,
	"workflow.undo_last_ai_patch":               true,
	"workflow.create_draft_from_confirmed_plan": true,
}

func Tools(service *edit.Service) []tooling.Tool {
	if service == nil {
		service = edit.NewService(nil)
	}
	names := []string{
		"workflow.list",
		"workflow.get_snapshot",
		"workflow.get_step",
		"workflow.get_action_catalog",
		"workflow.get_manual_binding",
		"workflow.describe",
		"workflow.diff",
		"workflow.get_ai_session",
		"workflow.propose_edit_plan",
		"workflow.propose_patch",
		"workflow.validate_patch",
		"workflow.preview_patch",
		"workflow.detect_patch_effect",
		"workflow.propose_ops_manual_candidate",
		"workflow.propose_ops_manual_update",
		"workflow.apply_patch",
		"workflow.save_draft",
		"workflow.attach_manual_candidate",
		"workflow.undo_last_ai_patch",
		"workflow.propose_create_plan",
		"workflow.create_draft_from_confirmed_plan",
	}
	out := make([]tooling.Tool, 0, len(names))
	for _, name := range names {
		out = append(out, newWorkflowEditorTool(name, service))
	}
	return out
}

func newWorkflowEditorTool(name string, service *edit.Service) tooling.Tool {
	readOnly := readOnlyTools[name]
	mutating := mutatingTools[name]
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Description: workflowToolDescription(name),
			Layer:       tooling.ToolLayerMCP,
			Pack:        ToolPack,
			Profiles:    []string{runtimekernel.RuntimePromptProfileWorkflowAgent},
			IsMCP:       true,
			MCPInfo: tooling.MCPInfo{
				ServerID:   ServerID,
				ServerName: "Runner Workflow Editor",
				ToolName:   name,
			},
			RiskLevel:        workflowToolRisk(readOnly, mutating),
			Mutating:         mutating,
			RequiresApproval: mutating,
			Discovery: tooling.ToolDiscoveryMetadata{
				DiscoveryGroup:     ToolPack,
				CapabilityKind:     "runner",
				ResourceTypes:      []string{"workflow", "runner_workflow", "ops_manual_candidate"},
				OperationKinds:     workflowToolOperationKinds(name),
				AgentProfiles:      []string{runtimekernel.RuntimePromptProfileWorkflowAgent},
				ToolPackIDs:        []string{ToolPack},
				MCPServerID:        ServerID,
				PermissionScope:    workflowToolPermission(readOnly, mutating),
				PromptBudgetClass:  "compact",
				SchemaBudgetClass:  "on_demand",
				RequiresHealthyMCP: false,
			},
		},
		Visibility:      tooling.Visibility{SessionTypes: []string{"workspace"}, Modes: []string{"plan"}},
		InputSchemaData: json.RawMessage(workflowToolSchema),
		ReadOnlyFunc: func(json.RawMessage) bool {
			return readOnly
		},
		DestructiveFunc: func(json.RawMessage) bool {
			return mutating
		},
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return readOnly
		},
		CheckPermissionsFunc: func(_ context.Context, input json.RawMessage) tooling.PermissionDecision {
			if !mutating {
				return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
			}
			if name == "workflow.apply_patch" {
				var req edit.ApplyPatchRequest
				_ = json.Unmarshal(input, &req)
				if strings.TrimSpace(req.BaseRevision) == "" || strings.TrimSpace(req.UserConfirmationID) == "" || strings.TrimSpace(req.DrawerSessionID) == "" {
					return tooling.PermissionDecision{Action: tooling.PermissionActionDeny, Reason: "workflow.apply_patch requires base_revision, user_confirmation_id and drawer_session_id"}
				}
			}
			return tooling.PermissionDecision{Action: tooling.PermissionActionNeedApproval, Reason: "workflow mutation requires user approval"}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			result, err := executeWorkflowEditorTool(ctx, service, name, input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			raw, err := json.Marshal(result)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(raw),
				Display: &tooling.ToolDisplayPayload{
					Type:  "workflow_editor",
					Title: name,
				},
			}, nil
		},
	}
}

func executeWorkflowEditorTool(ctx context.Context, service *edit.Service, name string, input json.RawMessage) (any, error) {
	switch name {
	case "workflow.get_snapshot":
		var req edit.GetSnapshotRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.GetSnapshot(ctx, req)
	case "workflow.get_step":
		var req edit.GetStepRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.GetStep(ctx, req)
	case "workflow.describe":
		var req edit.DescribeRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.Describe(ctx, req)
	case "workflow.propose_edit_plan", "workflow.propose_create_plan":
		var req edit.ProposeEditPlanRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.ProposeEditPlan(ctx, req)
	case "workflow.propose_patch":
		var req edit.ProposePatchRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.ProposePatch(ctx, req)
	case "workflow.validate_patch":
		var req edit.ValidatePatchRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.ValidatePatch(ctx, req)
	case "workflow.preview_patch", "workflow.diff":
		var req edit.PreviewPatchRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.PreviewPatch(ctx, req)
	case "workflow.detect_patch_effect":
		var req edit.DetectPatchEffectRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.DetectPatchEffect(ctx, req)
	case "workflow.propose_ops_manual_candidate":
		var req edit.WorkflowManualCandidateRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.ProposeOpsManualCandidate(ctx, req)
	case "workflow.propose_ops_manual_update":
		var req edit.WorkflowManualCandidateRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.ProposeOpsManualUpdate(ctx, req)
	case "workflow.create_draft_from_confirmed_plan":
		var req edit.WorkflowDraftFromPlanRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.CreateDraftFromConfirmedPlan(ctx, req)
	case "workflow.apply_patch":
		var req edit.ApplyPatchRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.ApplyPatch(ctx, req)
	case "workflow.undo_last_ai_patch":
		var req edit.UndoLastAIPatchRequest
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		return service.UndoLastAIPatch(ctx, req)
	case "workflow.get_ai_session":
		var req struct {
			DrawerSessionID string `json:"drawerSessionId"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, err
		}
		session, ok := service.Sessions().Get(req.DrawerSessionID)
		if !ok {
			return nil, fmt.Errorf("workflow ai session %q not found", req.DrawerSessionID)
		}
		return session, nil
	default:
		return map[string]any{"status": "ok", "tool": name}, nil
	}
}

func workflowToolDescription(name string) string {
	switch name {
	case "workflow.apply_patch":
		return "Apply a user-confirmed workflow patch with base revision, drawer session, reason and audit trace"
	case "workflow.undo_last_ai_patch":
		return "Undo the last AI-applied workflow patch if no manual revision interleaving occurred"
	case "workflow.describe":
		return "Describe the current workflow graph and effect status without mutation"
	default:
		return "Workflow AI editor tool " + name
	}
}

func workflowToolRisk(readOnly, mutating bool) tooling.ToolRiskLevel {
	if mutating {
		return tooling.ToolRiskHigh
	}
	if readOnly {
		return tooling.ToolRiskLow
	}
	return tooling.ToolRiskMedium
}

func workflowToolPermission(readOnly, mutating bool) string {
	if mutating {
		return "reviewed_mutation"
	}
	if readOnly {
		return "read"
	}
	return "proposal"
}

func workflowToolOperationKinds(name string) []string {
	if readOnlyTools[name] {
		return []string{"read", "proposal", "validate", "preview"}
	}
	if mutatingTools[name] {
		return []string{"reviewed_mutation"}
	}
	return []string{"proposal"}
}
