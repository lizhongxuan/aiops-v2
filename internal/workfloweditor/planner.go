package workfloweditor

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/modelrouter"
	"github.com/cloudwego/eino/schema"
)

type WorkflowEditPlanner interface {
	BuildWorkflowEditPlan(context.Context, WorkflowEditPlanningRequest) (WorkflowEditPlan, error)
}

type WorkflowEditPlanningRequest struct {
	WorkflowID      string
	DrawerSessionID string
	Message         string
	Describe        DescribeResult
}

type DefaultWorkflowEditPlanner struct{}

func (DefaultWorkflowEditPlanner) BuildWorkflowEditPlan(ctx context.Context, req WorkflowEditPlanningRequest) (WorkflowEditPlan, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowEditPlan{}, err
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return WorkflowEditPlan{}, errors.New("workflow edit message is required")
	}
	return WorkflowEditPlan{}, errors.New("workflow edit planner is not configured; configure an LLM-backed workflow edit planner")
}

type WorkflowEditModelRouter interface {
	GetModel(modelrouter.AgentKind, modelrouter.ProviderConfig) (modelrouter.ChatModel, error)
}

type ModelRouterWorkflowEditPlanner struct {
	Router         WorkflowEditModelRouter
	AgentKind      modelrouter.AgentKind
	ProviderConfig modelrouter.ProviderConfig
	SystemPrompt   string
}

func (p ModelRouterWorkflowEditPlanner) BuildWorkflowEditPlan(ctx context.Context, req WorkflowEditPlanningRequest) (WorkflowEditPlan, error) {
	if p.Router == nil {
		return WorkflowEditPlan{}, errors.New("workflow edit model router is not configured")
	}
	agentKind := p.AgentKind
	if strings.TrimSpace(string(agentKind)) == "" {
		agentKind = modelrouter.AgentKindPlanner
	}
	chatModel, err := p.Router.GetModel(agentKind, p.ProviderConfig)
	if err != nil {
		return WorkflowEditPlan{}, err
	}
	if chatModel == nil {
		return WorkflowEditPlan{}, errors.New("workflow edit model router returned nil chat model")
	}
	messages := []*schema.Message{
		schema.SystemMessage(firstNonEmpty(p.SystemPrompt, workflowEditPlannerSystemPrompt())),
		schema.UserMessage(workflowEditPlannerUserPrompt(req)),
	}
	response, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return WorkflowEditPlan{}, err
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return WorkflowEditPlan{}, errors.New("workflow edit planner response is empty")
	}
	plan, err := parseWorkflowEditPlanModelContent(response.Content)
	if err != nil {
		return WorkflowEditPlan{}, err
	}
	plan.WorkflowID = firstNonEmpty(plan.WorkflowID, req.WorkflowID)
	plan.Message = firstNonEmpty(plan.Message, req.Message)
	return plan, nil
}

func workflowEditPlannerSystemPrompt() string {
	return strings.TrimSpace(`你是 AIOps Workflow AI 的工作流编辑 agent，只负责生成“修改计划”，不能直接执行或修改生产环境。

输出要求：
- 只输出 JSON 对象，不要 Markdown，不要额外解释。
	- JSON 结构必须是：{"items":[{"id":"...","title":"...","description":"...","goal":"...","environment":"...","nodeLabel":"...","nodeType":"action","nodeAction":"script.python","scriptSummary":"...","validationSummary":"...","inputVariables":[{"name":"...","type":"...","required":true}],"outputVariables":[{"name":"...","type":"..."}],"script":"...","status":"pending"}]}。
	- 计划长度必须由用户意图决定：简单节点变更 1 步，局部图层调整 2 到 3 步，完整运维工作流 4 到 8 步。
	- 计划确认通过用户对话回复完成，不要输出按钮文案。
	- title 必须是完整、短的动作短语，不要加“步骤 1：”这类序号前缀，不要截断引号中的节点名。
	- 每个步骤要描述会如何修改 Workflow 图层中的节点、边、输入、输出、预检、验证、运行记录或事件。
	- 每个计划项必须能映射到一个或多个具体 Workflow 节点、边、脚本、输入输出或验证动作。
	- nodeLabel 是画布上展示的短节点名；如果用户明确指定“节点名称/叫/命名为”，必须原样使用该名称，不要把整句计划标题当节点名。
	- nodeType 和 nodeAction 描述 UI 需要创建的节点类型与执行动作；不确定时用 nodeType="action" 和 nodeAction="script.python"。
	- 生成脚本时只写与本步骤目标直接相关的可编辑脚本；不要用固定 pgBackRest、Redis、日志或运行记录模板填充所有步骤。
- 不允许把用户原话包装成单个步骤；不允许使用“生成一个最小 Workflow patch”或“Review and apply one workflow patch at a time”这类模板描述。
- 简单节点变更（例如“随便添加一个节点”“加一个步骤”）只生成一个最小计划项：根据当前 workflow 摘要选择一个合理节点，说明为什么添加它，以及它会连接到哪里。
- 缺少必要信息时，只在确实无法设计具体节点或脚本时集中成一个确认步骤；不要为了套模板而添加“确认边界与变量”。
- 完整运维工作流才需要按实际风险覆盖对象识别、证据/预检、执行或变更、验证、失败处理、运行记录/经验沉淀；这些不是所有请求的固定步骤。`)
}

func workflowEditPlannerUserPrompt(req WorkflowEditPlanningRequest) string {
	describe := req.Describe
	if strings.TrimSpace(describe.Summary) == "" {
		describe.Summary = "当前 Workflow 图层尚未提供可用摘要。"
	}
	payload := map[string]any{
		"workflow_id":       strings.TrimSpace(req.WorkflowID),
		"drawer_session_id": strings.TrimSpace(req.DrawerSessionID),
		"user_request":      strings.TrimSpace(req.Message),
		"current_workflow": map[string]any{
			"summary":    describe.Summary,
			"node_count": describe.NodeCount,
			"edge_count": describe.EdgeCount,
			"node_ids":   describe.NodeIDs,
		},
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return "请基于下面的 Workflow 编辑请求生成完整修改计划。只返回 JSON。\n\n" + string(data)
}

func parseWorkflowEditPlanModelContent(content string) (WorkflowEditPlan, error) {
	raw := strings.TrimSpace(content)
	if raw == "" {
		return WorkflowEditPlan{}, errors.New("empty model content")
	}
	raw = extractJSONObject(raw)
	var plan WorkflowEditPlan
	if err := json.Unmarshal([]byte(raw), &plan); err != nil {
		return WorkflowEditPlan{}, fmt.Errorf("parse workflow edit plan json: %w", err)
	}
	if len(plan.Items) == 0 {
		var wrapper struct {
			Plan  WorkflowEditPlan       `json:"plan"`
			Items []WorkflowEditPlanItem `json:"items"`
		}
		if err := json.Unmarshal([]byte(raw), &wrapper); err != nil {
			return WorkflowEditPlan{}, fmt.Errorf("parse workflow edit plan wrapper: %w", err)
		}
		if len(wrapper.Plan.Items) > 0 {
			plan = wrapper.Plan
		} else {
			plan.Items = wrapper.Items
		}
	}
	if len(plan.Items) == 0 {
		return WorkflowEditPlan{}, errors.New("workflow edit planner returned no plan items")
	}
	return plan, nil
}

func extractJSONObject(raw string) string {
	raw = strings.TrimSpace(raw)
	raw = strings.TrimPrefix(raw, "```json")
	raw = strings.TrimPrefix(raw, "```")
	raw = strings.TrimSuffix(raw, "```")
	raw = strings.TrimSpace(raw)
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return raw[start : end+1]
	}
	return raw
}

func normalizeWorkflowEditPlan(plan WorkflowEditPlan, req ProposeEditPlanRequest) (WorkflowEditPlan, error) {
	message := strings.TrimSpace(firstNonEmpty(plan.Message, req.Message))
	if message == "" {
		return WorkflowEditPlan{}, errors.New("workflow edit message is required")
	}
	out := WorkflowEditPlan{
		ID:         firstNonEmpty(plan.ID, stableID("plan", firstNonEmpty(req.DrawerSessionID, req.WorkflowID, message))),
		WorkflowID: strings.TrimSpace(firstNonEmpty(plan.WorkflowID, req.WorkflowID)),
		Message:    message,
		CreatedAt:  plan.CreatedAt,
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	for index, item := range plan.Items {
		title := normalizePlanItemTitlePrefix(strings.TrimSpace(item.Title))
		description := strings.TrimSpace(item.Description)
		if title == "" && description == "" {
			continue
		}
		if title == "" || strings.EqualFold(title, message) || planItemTitleLooksBroken(title) {
			title = fallbackPlanItemTitle(index, description)
		}
		if planItemLooksLikeDeadRule(title, description) {
			return WorkflowEditPlan{}, fmt.Errorf("workflow edit planner returned template plan item %q", title)
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = stableID("item", fmt.Sprintf("%02d-%s", index+1, title))
		}
		out.Items = append(out.Items, WorkflowEditPlanItem{
			ID:                id,
			Title:             title,
			Description:       description,
			Status:            firstNonEmpty(item.Status, "pending"),
			Goal:              strings.TrimSpace(item.Goal),
			Environment:       strings.TrimSpace(item.Environment),
			NodeLabel:         strings.TrimSpace(item.NodeLabel),
			NodeType:          strings.TrimSpace(item.NodeType),
			NodeAction:        strings.TrimSpace(item.NodeAction),
			ScriptSummary:     strings.TrimSpace(item.ScriptSummary),
			ValidationSummary: strings.TrimSpace(item.ValidationSummary),
			InputVariables:    normalizeWorkflowVariableSpecs(item.InputVariables),
			OutputVariables:   normalizeWorkflowVariableSpecs(item.OutputVariables),
			Script:            strings.TrimSpace(item.Script),
		})
	}
	if len(out.Items) == 0 {
		return WorkflowEditPlan{}, errors.New("workflow edit plan has no valid items")
	}
	return out, nil
}

func normalizeWorkflowVariableSpecs(values []WorkflowVariableSpec) []WorkflowVariableSpec {
	out := make([]WorkflowVariableSpec, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		out = append(out, WorkflowVariableSpec{
			Name:     name,
			Type:     strings.TrimSpace(value.Type),
			Required: value.Required,
			Source:   strings.TrimSpace(value.Source),
		})
	}
	return out
}

func fallbackPlanItemTitle(index int, description string) string {
	trimmed := strings.TrimSpace(description)
	if trimmed != "" {
		fragment := firstPlanTitleFragment(trimmed)
		if fragment != "" {
			return fragment
		}
	}
	return fmt.Sprintf("步骤 %d", index+1)
}

func normalizePlanItemTitlePrefix(title string) string {
	title = strings.TrimSpace(title)
	for _, separator := range []string{"：", ":", ".", "、"} {
		for _, prefix := range []string{"步骤 ", "步骤", "step ", "Step "} {
			lowerTitle := strings.ToLower(title)
			lowerPrefix := strings.ToLower(prefix)
			if strings.HasPrefix(lowerTitle, lowerPrefix) {
				index := strings.Index(title, separator)
				if index > 0 && index+len(separator) < len(title) {
					return strings.TrimSpace(title[index+len(separator):])
				}
			}
		}
	}
	return title
}

func planItemTitleLooksBroken(title string) bool {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return true
	}
	return strings.Count(trimmed, "“") != strings.Count(trimmed, "”") ||
		strings.Count(trimmed, "「") != strings.Count(trimmed, "」") ||
		strings.Count(trimmed, "\"")%2 != 0 ||
		strings.HasSuffix(trimmed, "“") ||
		strings.HasSuffix(trimmed, "「")
}

func firstPlanTitleFragment(description string) string {
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return ""
	}
	cut := len(trimmed)
	for _, separator := range []string{"。", "\n", "；", ";"} {
		if index := strings.Index(trimmed, separator); index > 0 && index < cut {
			cut = index
		}
	}
	fragment := strings.TrimSpace(trimmed[:cut])
	runes := []rune(fragment)
	if len(runes) > 36 {
		fragment = strings.TrimSpace(string(runes[:36])) + "..."
	}
	return fragment
}

func planItemLooksLikeDeadRule(title, description string) bool {
	text := strings.ToLower(title + " " + description)
	return strings.Contains(text, strings.ToLower("生成一个最小 Workflow patch")) ||
		strings.Contains(text, strings.ToLower("Review and apply one workflow patch at a time"))
}
