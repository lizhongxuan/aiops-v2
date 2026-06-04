package workflowgen

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type DeterministicPlanBuilder struct{}

func (b DeterministicPlanBuilder) BuildPlan(_ context.Context, req BuildPlanRequest) (*WorkflowGenerationPlan, error) {
	requirement := strings.TrimSpace(req.Requirement)
	if requirement == "" {
		return nil, errors.New("requirement is required")
	}

	topic := inferWorkflowTopic(requirement)
	trigger := inferTrigger(requirement)
	outputs, slots := inferOutputsAndSlots(requirement, req.Slots, topic)
	nodes := []WorkflowPlanNode{
		{
			ID:          topic.SearchNodeID,
			Kind:        NodeKindSearch,
			Title:       topic.SearchTitle,
			Description: topic.SearchDescription,
			Action:      "script.python",
			Outputs: []WorkflowIO{{
				ID:          topic.ItemsOutputID,
				Type:        "array",
				Description: topic.ItemsDescription,
				Required:    true,
			}},
			Config: map[string]any{"network": "mockable", "topic": topic.ID},
		},
		{
			ID:          topic.TransformNodeID,
			Kind:        NodeKindTransform,
			Title:       topic.TransformTitle,
			Description: topic.TransformDescription,
			Action:      "script.python",
			Inputs: []WorkflowIO{{
				ID:          topic.ItemsOutputID,
				Type:        "array",
				Description: topic.ItemsDescription,
				Required:    true,
			}},
			Outputs: []WorkflowIO{{
				ID:          "key_news",
				Type:        "array",
				Description: topic.KeyItemsDescription,
				Required:    true,
			}},
			Config: map[string]any{"topic": topic.ID},
		},
		outputNode(outputs, topic),
	}

	plan := &WorkflowGenerationPlan{
		Version: 1,
		Title:   topic.Title,
		Intent:  "generate_runner_workflow",
		Trigger: trigger,
		Inputs: []WorkflowIO{{
			ID:          "topic",
			Type:        "string",
			Description: topic.InputDescription,
			Required:    false,
		}},
		Nodes:   nodes,
		Outputs: outputs,
		ValidationStrategy: ValidationStrategy{
			Enabled:  true,
			Provider: ValidationProviderDocker,
			Scenario: validationScenario(outputs, topic),
			Network:  "mock",
		},
		Risks: []string{
			"外部搜索结果可能不稳定，Docker 验证阶段应使用 mock 数据固定输入输出。",
			"推送类输出只能引用密钥变量名，不能在计划或节点代码里写入真实 token。",
		},
		RequiredSlots: slots,
	}
	return plan, nil
}

func (b DeterministicPlanBuilder) RevisePlan(_ context.Context, req RevisePlanRequest) (*WorkflowGenerationPlan, error) {
	previous := req.Previous
	if previous.Version == 0 {
		previous.Version = 1
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return nil, errors.New("revision message is required")
	}
	topic := inferWorkflowTopic(firstNonEmpty(message, previous.Title))
	outputs, slots := inferOutputsAndSlots(message, nil, topic)
	if len(outputs) > 0 && explicitOutputChange(message) {
		previous.Outputs = outputs
		previous.RequiredSlots = slots
		for i := range previous.Nodes {
			if previous.Nodes[i].Kind == NodeKindOutput {
				previous.Nodes[i] = outputNode(outputs, topic)
			}
		}
		previous.ValidationStrategy.Scenario = validationScenario(outputs, topic)
	}
	if trigger := inferTrigger(message); trigger.Type != "" {
		if explicitScheduleChange(message) || previous.Trigger.Type == "" {
			previous.Trigger = trigger
		}
	}
	previous.Version++
	return &previous, nil
}

type workflowTopic struct {
	ID                   string
	Title                string
	InputDescription     string
	SearchNodeID         string
	SearchTitle          string
	SearchDescription    string
	ItemsOutputID        string
	ItemsDescription     string
	TransformNodeID      string
	TransformTitle       string
	TransformDescription string
	KeyItemsDescription  string
	ResultPhrase         string
	ReturnDescription    string
	ScenarioReturn       string
	ScenarioDelivery     string
}

func inferWorkflowTopic(requirement string) workflowTopic {
	normalized := strings.ToLower(requirement)
	if strings.Contains(requirement, "Kubernetes") ||
		strings.Contains(strings.ToLower(requirement), "kubernetes") ||
		strings.Contains(requirement, "云原生") ||
		strings.Contains(requirement, "漏洞") ||
		strings.Contains(requirement, "安全公告") {
		return workflowTopic{
			ID:                   "kubernetes-security",
			Title:                "Kubernetes 安全风险摘要工作流",
			InputDescription:     "安全公告、CVE 或云原生漏洞检索关键词",
			SearchNodeID:         "search-kubernetes-security",
			SearchTitle:          "搜索安全公告",
			SearchDescription:    "获取 Kubernetes、云原生与中间件安全公告候选列表，验证阶段使用可控 mock 数据。",
			ItemsOutputID:        "security_items",
			ItemsDescription:     "安全公告候选列表",
			TransformNodeID:      "extract-security-risks",
			TransformTitle:       "提取重点风险",
			TransformDescription: "从公告候选中提取三条需要关注的风险，包含影响范围、优先级和建议动作。",
			KeyItemsDescription:  "三条需要关注的安全风险",
			ResultPhrase:         "三条需要关注的安全风险",
			ReturnDescription:    "在工作流输出中直接返回三条需要关注的安全风险。",
			ScenarioReturn:       "security-risk-return-only",
			ScenarioDelivery:     "security-risk-delivery-mock",
		}
	}
	if strings.Contains(requirement, "数据库") ||
		strings.Contains(requirement, "中间件") ||
		strings.Contains(requirement, "故障案例") ||
		strings.Contains(requirement, "复盘") ||
		strings.Contains(normalized, "incident") {
		return workflowTopic{
			ID:                   "ops-incident",
			Title:                "数据库与中间件故障复盘工作流",
			InputDescription:     "数据库、中间件或故障案例检索关键词",
			SearchNodeID:         "search-ops-incidents",
			SearchTitle:          "搜索故障案例",
			SearchDescription:    "获取数据库与中间件故障案例候选列表，验证阶段使用可控 mock 数据。",
			ItemsOutputID:        "incident_items",
			ItemsDescription:     "故障案例候选列表",
			TransformNodeID:      "extract-ops-lessons",
			TransformTitle:       "提取运维经验",
			TransformDescription: "从故障案例中提取三条可复盘的运维经验，包含触发条件、处置方式和预防建议。",
			KeyItemsDescription:  "三条可复盘的运维经验",
			ResultPhrase:         "三条可复盘的运维经验",
			ReturnDescription:    "在工作流输出中直接返回三条可复盘的运维经验。",
			ScenarioReturn:       "ops-incident-return-only",
			ScenarioDelivery:     "ops-incident-delivery-mock",
		}
	}
	return workflowTopic{
		ID:                   "ai-news",
		Title:                "AI 新闻摘要工作流",
		InputDescription:     "新闻主题或检索关键词",
		SearchNodeID:         "search-news",
		SearchTitle:          "搜索 AI 新闻",
		SearchDescription:    "按需求获取 AI 行业新闻候选列表，验证阶段使用可控 mock 数据。",
		ItemsOutputID:        "news_items",
		ItemsDescription:     "新闻候选列表",
		TransformNodeID:      "extract-key-news",
		TransformTitle:       "提取关键新闻",
		TransformDescription: "从候选列表中提取三条高价值新闻并输出结构化摘要。",
		KeyItemsDescription:  "三条关键新闻摘要",
		ResultPhrase:         "三条关键新闻",
		ReturnDescription:    "在工作流输出中直接返回三条关键新闻。",
		ScenarioReturn:       "news-summary-return-only",
		ScenarioDelivery:     "news-summary-delivery-mock",
	}
}

func inferTrigger(text string) WorkflowTrigger {
	normalized := strings.ToLower(text)
	if hour, ok := inferDailyHour(text); ok || strings.Contains(text, "每天") || strings.Contains(normalized, "cron") {
		if !ok {
			hour = 8
		}
		return WorkflowTrigger{
			Type:     TriggerTypeSchedule,
			Schedule: fmt.Sprintf("0 %d * * *", hour),
			Summary:  fmt.Sprintf("每天 %02d:00 自动运行", hour),
		}
	}
	return WorkflowTrigger{
		Type:    TriggerTypeManual,
		Summary: "手动触发",
	}
}

func inferDailyHour(text string) (int, bool) {
	for hour := 1; hour <= 11; hour++ {
		value := fmt.Sprintf("%d点", hour)
		if strings.Contains(text, "下午"+value) || strings.Contains(text, "晚上"+value) {
			return hour + 12, true
		}
	}
	for hour := 0; hour <= 23; hour++ {
		if strings.Contains(text, fmt.Sprintf("%d点", hour)) {
			return hour, true
		}
	}
	return 0, false
}

func inferOutputsAndSlots(text string, filled map[string]string, topic workflowTopic) ([]WorkflowOutput, []RequiredSlot) {
	normalized := strings.ToLower(text)
	var target OutputTarget
	switch {
	case strings.Contains(text, "直接返回") || strings.Contains(text, "不自动推送") || strings.Contains(text, "不要飞书") || strings.Contains(text, "不要邮件"):
		target = OutputTargetReturn
	case strings.Contains(text, "飞书"):
		target = OutputTargetFeishu
	case strings.Contains(text, "邮件") || strings.Contains(normalized, "email"):
		target = OutputTargetEmail
	case strings.Contains(normalized, "webhook"):
		target = OutputTargetWebhook
	default:
		target = ""
	}

	if target == "" && filled != nil {
		switch strings.ToLower(strings.TrimSpace(filled["delivery_method"])) {
		case "return", "direct", "直接返回":
			target = OutputTargetReturn
		case "feishu", "飞书":
			target = OutputTargetFeishu
		case "email", "mail", "邮件":
			target = OutputTargetEmail
		case "webhook":
			target = OutputTargetWebhook
		}
	}

	if target == "" {
		return []WorkflowOutput{{
				ID:          "delivery",
				Target:      OutputTargetReturn,
				Description: fmt.Sprintf("默认先在运行结果中返回%s，用户确认后可改成飞书、邮件或 Webhook 推送。", topic.ResultPhrase),
			}}, []RequiredSlot{{
				ID:       "delivery_method",
				Label:    "推送方式",
				Question: "请选择结果交付方式：直接返回、飞书、邮件或 Webhook。",
				Type:     "select",
				Options:  []string{"直接返回", "飞书", "邮件", "Webhook"},
				Required: true,
			}}
	}

	output := WorkflowOutput{
		ID:          "delivery",
		Target:      target,
		Description: outputDescription(target, topic),
	}
	var slots []RequiredSlot
	switch target {
	case OutputTargetFeishu, OutputTargetWebhook:
		output.SecretRef = filledValue(filled, "webhook_secret_ref")
		if output.SecretRef == "" {
			slots = append(slots, RequiredSlot{
				ID:        "webhook_secret_ref",
				Label:     "Webhook 密钥变量",
				Question:  "请提供 Webhook 密钥变量名，例如 FEISHU_WEBHOOK_URL。不要填写真实 token。",
				Type:      "secret_ref",
				Required:  true,
				Sensitive: true,
			})
		}
	case OutputTargetEmail:
		output.SecretRef = filledValue(filled, "email_recipient_secret_ref")
		if output.SecretRef == "" {
			slots = append(slots, RequiredSlot{
				ID:        "email_recipient_secret_ref",
				Label:     "邮件收件人变量",
				Question:  "请提供邮件收件人或 SMTP 配置的密钥变量名，不要填写真实密码。",
				Type:      "secret_ref",
				Required:  true,
				Sensitive: true,
			})
		}
	}
	return []WorkflowOutput{output}, slots
}

func outputNode(outputs []WorkflowOutput, topic workflowTopic) WorkflowPlanNode {
	target := OutputTargetReturn
	if len(outputs) > 0 {
		target = outputs[0].Target
	}
	return WorkflowPlanNode{
		ID:          "deliver-result",
		Kind:        NodeKindOutput,
		Title:       "交付结果",
		Description: outputDescription(target, topic),
		Action:      "script.python",
		Inputs: []WorkflowIO{{
			ID:          "key_news",
			Type:        "array",
			Description: topic.KeyItemsDescription,
			Required:    true,
		}},
		Outputs: []WorkflowIO{{
			ID:          "delivery_result",
			Type:        "object",
			Description: "交付执行结果",
			Required:    true,
		}},
		Config: map[string]any{"target": string(target), "topic": topic.ID},
	}
}

func outputDescription(target OutputTarget, topic workflowTopic) string {
	switch target {
	case OutputTargetFeishu:
		return "通过飞书 Webhook 推送" + topic.ResultPhrase + "。"
	case OutputTargetEmail:
		return "通过邮件发送" + topic.ResultPhrase + "。"
	case OutputTargetWebhook:
		return "通过 Webhook 推送" + topic.ResultPhrase + "。"
	default:
		return topic.ReturnDescription
	}
}

func validationScenario(outputs []WorkflowOutput, topic workflowTopic) string {
	if len(outputs) == 0 || outputs[0].Target == OutputTargetReturn {
		return topic.ScenarioReturn
	}
	return topic.ScenarioDelivery
}

func explicitOutputChange(text string) bool {
	return strings.Contains(text, "直接返回") ||
		strings.Contains(text, "飞书") ||
		strings.Contains(text, "邮件") ||
		strings.Contains(strings.ToLower(text), "webhook") ||
		strings.Contains(text, "不要飞书") ||
		strings.Contains(text, "不要邮件")
}

func explicitScheduleChange(text string) bool {
	return strings.Contains(text, "每天") ||
		strings.Contains(text, "早上") ||
		strings.Contains(text, "点") ||
		strings.Contains(strings.ToLower(text), "cron") ||
		strings.Contains(text, "手动")
}

func filledValue(values map[string]string, key string) string {
	if values == nil {
		return ""
	}
	return strings.TrimSpace(values[key])
}
