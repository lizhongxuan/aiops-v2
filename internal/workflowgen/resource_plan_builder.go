package workflowgen

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"aiops-v2/internal/opsmanual"
)

type BuildResourcePlanRequest struct {
	Requirement    string
	OperationFrame opsmanual.OperationFrame
	Slots          map[string]string
}

type ResourcePlanBuilder struct{}

func (b ResourcePlanBuilder) BuildResourcePlan(ctx context.Context, req BuildResourcePlanRequest) (*WorkflowGenerationPlan, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	requirement := strings.TrimSpace(firstNonEmpty(req.Requirement, req.OperationFrame.RawText))
	if requirement == "" {
		return nil, errors.New("requirement is required")
	}

	frame := req.OperationFrame
	resourceKind := firstNonEmpty(frame.Target.Type, frame.ObjectType, frame.Operation.TargetType, "resource")
	frameMap, err := operationFrameToMap(frame)
	if err != nil {
		return nil, err
	}

	return &WorkflowGenerationPlan{
		Version:        1,
		Title:          "资源型运维 Workflow 草稿",
		Intent:         "generate_resource_ops_workflow",
		ReviewStatus:   ReviewStatusPendingReview,
		ResourceKind:   resourceKind,
		OperationFrame: frameMap,
		Trigger: WorkflowTrigger{
			Type:    TriggerTypeManual,
			Summary: "手动触发",
		},
		Inputs: []WorkflowIO{
			{ID: "target_resources", Type: "array", Description: "目标资源引用", Required: true},
			{ID: "secret_ref", Type: "secret_ref", Description: "访问凭据 SecretRef", Required: true},
		},
		Nodes: []WorkflowPlanNode{
			resourceStageNode("preflight", "只读预检", "验证目标资源、执行面和观察点是否可用。", true, false, []WorkflowIO{
				{ID: "preflight_evidence", Type: "object", Description: "只读预检证据", Required: true},
			}),
			resourceStageNode("execute", "受治理执行", "在审批后对目标资源执行受控变更。", false, true, []WorkflowIO{
				{ID: "execution_result", Type: "object", Description: "受控执行结果", Required: true},
			}),
			resourceStageNode("verify", "独立验证", "通过资源健康与观察点证据验证目标状态。", true, false, []WorkflowIO{
				{ID: "verification_evidence", Type: "object", Description: "独立验证证据", Required: true},
			}),
			resourceStageNode("rollback", "回滚或人工接管", "执行回滚路径或升级人工接管。", false, true, []WorkflowIO{
				{ID: "rollback_result", Type: "object", Description: "回滚或接管结果", Required: false},
			}),
		},
		Outputs: []WorkflowOutput{
			{ID: "resource_workflow_summary", Target: OutputTargetReturn, Description: "返回资源型运维 workflow 草稿摘要。"},
		},
		ValidationStrategy: ValidationStrategy{
			Enabled:  true,
			Provider: ValidationProviderDocker,
			Scenario: "resource-ops-draft-static-validation",
			Network:  "mock",
		},
		Risks: []string{
			"变更执行和回滚阶段必须经过人工审批。",
			"访问凭据只能通过 secret_ref 输入引用，不能写入计划或节点配置。",
			"验证阶段必须使用独立资源健康或观察点证据，不能只复用执行输出。",
		},
		RequiredSlots: []RequiredSlot{
			{ID: "target_resources", Label: "目标主机/资源", Question: "请把主机A、主机B、主机C映射到主机清单中的主机或可连接地址，并确认主机C上的 monitor 组件名称。", Type: "array", Required: true},
			{ID: "secret_ref", Label: "访问凭据", Question: "请选择用于访问目标资源的 SecretRef。", Type: "secret_ref", Required: true, Sensitive: true},
		},
	}, nil
}

func resourceStageNode(stage, title, description string, readOnly, requiresApproval bool, outputs []WorkflowIO) WorkflowPlanNode {
	config := map[string]any{
		"stage":             stage,
		"read_only":         readOnly,
		"requires_approval": requiresApproval,
	}
	switch stage {
	case "preflight":
		config["evidence_sources"] = []any{"target_resources", "execution_surface", "observation_points"}
	case "verify":
		config["evidence_sources"] = []any{"resource_health", "observation_points", "post_change_checks"}
	case "rollback":
		config["fallback_mode"] = "rollback_or_human_handoff"
	}
	return WorkflowPlanNode{
		ID:          "resource-" + stage,
		Kind:        NodeKindTransform,
		Title:       title,
		Description: description,
		Action:      "script.python",
		Inputs: []WorkflowIO{
			{ID: "target_resources", Type: "array", Description: "目标资源引用", Required: true},
			{ID: "secret_ref", Type: "secret_ref", Description: "访问凭据 SecretRef", Required: true},
		},
		Outputs: outputs,
		Config:  config,
	}
}

func operationFrameToMap(frame opsmanual.OperationFrame) (map[string]any, error) {
	data, err := json.Marshal(frame)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, err
	}
	return out, nil
}
