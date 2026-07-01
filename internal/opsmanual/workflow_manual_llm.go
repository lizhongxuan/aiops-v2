package opsmanual

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/modeltrace"
	"github.com/cloudwego/eino/schema"
)

type WorkflowManualLLMSummarizer interface {
	SummarizeWorkflowManual(context.Context, WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error)
}

type WorkflowManualLLMSummaryRequest struct {
	Analysis    WorkflowManualAnalysis
	Manual      OpsManual
	Validation  ManualCandidateValidation
	UserSummary ManualGenerationUserSummary
	Language    string
}

type WorkflowManualLLMSummaryResult struct {
	DocumentMarkdown string                      `json:"document_markdown"`
	UserSummary      ManualGenerationUserSummary `json:"user_summary"`
}

type NoopWorkflowManualLLMSummarizer struct{}

func (NoopWorkflowManualLLMSummarizer) SummarizeWorkflowManual(_ context.Context, req WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error) {
	return WorkflowManualLLMSummaryResult{DocumentMarkdown: req.Manual.DocumentMarkdown, UserSummary: req.UserSummary}, nil
}

type EnvWorkflowManualLLMSummarizer struct {
	Client  *http.Client
	BaseURL string
	APIKey  string
	Model   string
}

type WorkflowManualLLMModelRouter interface {
	GetModel(modelrouter.AgentKind, modelrouter.ProviderConfig) (modelrouter.ChatModel, error)
}

type ModelRouterWorkflowManualLLMSummarizer struct {
	Router         WorkflowManualLLMModelRouter
	AgentKind      modelrouter.AgentKind
	ProviderConfig modelrouter.ProviderConfig
	SystemPrompt   string
	PromptTemplate string
}

var defaultWorkflowManualLLMSummarizerState struct {
	mu         sync.RWMutex
	summarizer WorkflowManualLLMSummarizer
}

func SetDefaultWorkflowManualLLMSummarizer(summarizer WorkflowManualLLMSummarizer) {
	defaultWorkflowManualLLMSummarizerState.mu.Lock()
	defer defaultWorkflowManualLLMSummarizerState.mu.Unlock()
	defaultWorkflowManualLLMSummarizerState.summarizer = summarizer
}

func GenerateWorkflowManualCandidate(ctx context.Context, req WorkflowManualGenerationRequest, summarizer WorkflowManualLLMSummarizer) (WorkflowManualGenerationResult, error) {
	analysis, err := AnalyzeWorkflowForManual(req)
	if err != nil {
		return WorkflowManualGenerationResult{}, err
	}
	candidate, err := BuildWorkflowManualCandidate(analysis)
	if err != nil {
		return WorkflowManualGenerationResult{}, err
	}
	if req.Options.UseLLMSummary {
		if summarizer == nil {
			summarizer = resolveDefaultWorkflowManualLLMSummarizer()
		}
		candidate = applyWorkflowManualLLMSummary(ctx, candidate, analysis, summarizer)
	}
	return WorkflowManualGenerationResult{
		Candidate:        candidate,
		ValidationReport: candidate.StructuredValidationReport,
		UserSummary:      candidate.UserSummary,
	}, nil
}

func resolveDefaultWorkflowManualLLMSummarizer() WorkflowManualLLMSummarizer {
	defaultWorkflowManualLLMSummarizerState.mu.RLock()
	summarizer := defaultWorkflowManualLLMSummarizerState.summarizer
	defaultWorkflowManualLLMSummarizerState.mu.RUnlock()
	if summarizer != nil {
		return summarizer
	}
	return missingWorkflowManualLLMSummarizer{}
}

type missingWorkflowManualLLMSummarizer struct{}

func (missingWorkflowManualLLMSummarizer) SummarizeWorkflowManual(context.Context, WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error) {
	return WorkflowManualLLMSummaryResult{}, fmt.Errorf("workflow manual llm summarizer is not configured")
}

func applyWorkflowManualLLMSummary(ctx context.Context, candidate ManualCandidate, analysis WorkflowManualAnalysis, summarizer WorkflowManualLLMSummarizer) ManualCandidate {
	deterministicMarkdown := candidate.ProposedManual.DocumentMarkdown
	deterministicSummary := candidate.UserSummary
	result, err := summarizer.SummarizeWorkflowManual(ctx, WorkflowManualLLMSummaryRequest{
		Analysis:    analysis,
		Manual:      cloneManual(candidate.ProposedManual),
		Validation:  candidate.StructuredValidationReport,
		UserSummary: deterministicSummary,
		Language:    "zh-CN",
	})
	if err != nil {
		candidate.ProposedManual.DocumentMarkdown = deterministicMarkdown
		candidate.UserSummary = deterministicSummary
		candidate.StructuredValidationReport.Warnings = append(candidate.StructuredValidationReport.Warnings, ValidationIssue{Code: "llm_summary_failed", Field: "llm", Message: "LLM 文案润色失败，已保留确定性模板。", Evidence: err.Error()})
		candidate.StructuredValidationReport.Status = manualCandidateValidationStatus(candidate.StructuredValidationReport)
		candidate.ValidationReport = manualCandidateValidationMessages(candidate.StructuredValidationReport)
		return candidate
	}
	if err := ValidateWorkflowManualLLMOutput(result, candidate.ProposedManual); err != nil {
		candidate.ProposedManual.DocumentMarkdown = deterministicMarkdown
		candidate.UserSummary = deterministicSummary
		candidate.StructuredValidationReport.Warnings = append(candidate.StructuredValidationReport.Warnings, ValidationIssue{Code: "llm_output_rejected", Field: "llm", Message: "LLM 输出未通过安全过滤，已保留确定性模板。", Evidence: err.Error()})
		candidate.StructuredValidationReport.Status = manualCandidateValidationStatus(candidate.StructuredValidationReport)
		candidate.ValidationReport = manualCandidateValidationMessages(candidate.StructuredValidationReport)
		return candidate
	}
	if strings.TrimSpace(result.DocumentMarkdown) != "" {
		candidate.ProposedManual.DocumentMarkdown = strings.TrimSpace(result.DocumentMarkdown)
	}
	if len(result.UserSummary.Understood)+len(result.UserSummary.Missing)+len(result.UserSummary.NextSteps) > 0 {
		candidate.UserSummary = sanitizeManualGenerationUserSummary(result.UserSummary)
	}
	return candidate
}

func (s ModelRouterWorkflowManualLLMSummarizer) SummarizeWorkflowManual(ctx context.Context, req WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error) {
	if s.Router == nil {
		return WorkflowManualLLMSummaryResult{}, fmt.Errorf("modelrouter is not configured")
	}
	agentKind := s.AgentKind
	if strings.TrimSpace(string(agentKind)) == "" {
		agentKind = modelrouter.AgentKindWorker
	}
	chatModel, err := s.Router.GetModel(agentKind, s.ProviderConfig)
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	if chatModel == nil {
		return WorkflowManualLLMSummaryResult{}, fmt.Errorf("modelrouter returned nil chat model")
	}
	systemPrompt := firstNonEmpty(strings.TrimSpace(s.SystemPrompt), defaultWorkflowManualLLMSystemPrompt())
	userPrompt, err := renderWorkflowManualLLMPrompt(req, s.PromptTemplate)
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	messages := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userPrompt),
	}
	_, _ = modeltrace.WriteTraceDocumentV2FromRequest(modeltrace.Request{
		Kind:    "opsmanual-workflow-manual-llm-summary",
		TraceID: strings.TrimSpace(req.Manual.WorkflowRef.WorkflowID),
		Metadata: map[string]string{
			"workflow_id": strings.TrimSpace(req.Manual.WorkflowRef.WorkflowID),
			"language":    firstNonEmpty(req.Language, "zh-CN"),
			"provider":    strings.TrimSpace(s.ProviderConfig.Provider),
			"model":       strings.TrimSpace(s.ProviderConfig.Model),
		},
		Prompt: modeltrace.Prompt{
			System:  systemPrompt,
			Dynamic: userPrompt,
		},
		ModelInput: modelrouter.ModelInputItemsFromEinoMessages(messages),
	})
	response, err := chatModel.Generate(ctx, messages)
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	if response == nil || strings.TrimSpace(response.Content) == "" {
		return WorkflowManualLLMSummaryResult{}, fmt.Errorf("modelrouter response is empty")
	}
	return parseWorkflowManualLLMResultContent(response.Content)
}

func (s EnvWorkflowManualLLMSummarizer) SummarizeWorkflowManual(ctx context.Context, req WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(s.BaseURL), "/")
	apiKey := strings.TrimSpace(s.APIKey)
	model := strings.TrimSpace(s.Model)
	if baseURL == "" || apiKey == "" || model == "" {
		return WorkflowManualLLMSummaryResult{}, fmt.Errorf("llm config requires baseURL, apiKey and model")
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	payload := openAIChatCompletionsRequest{
		Model: model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: defaultWorkflowManualLLMSystemPrompt()},
			{Role: "user", Content: workflowManualLLMPrompt(req)},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(baseURL), bytes.NewReader(body))
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	resp, err := client.Do(httpReq)
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return WorkflowManualLLMSummaryResult{}, fmt.Errorf("llm request failed with status %d", resp.StatusCode)
	}
	var decoded openAIChatCompletionsResponse
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	if len(decoded.Choices) == 0 {
		return WorkflowManualLLMSummaryResult{}, fmt.Errorf("llm response has no choices")
	}
	return parseWorkflowManualLLMResultContent(decoded.Choices[0].Message.Content)
}

func ValidateWorkflowManualLLMOutput(result WorkflowManualLLMSummaryResult, manual OpsManual) error {
	text := strings.Join([]string{
		result.DocumentMarkdown,
		strings.Join(result.UserSummary.Understood, " "),
		strings.Join(result.UserSummary.Missing, " "),
		strings.Join(result.UserSummary.NextSteps, " "),
	}, " ")
	lower := strings.ToLower(text)
	for _, marker := range []string{"sk-", "authorization", "api-token", "secret_ref", "itsm/api-token", "chatops/bot-token"} {
		if strings.Contains(lower, strings.ToLower(marker)) {
			return fmt.Errorf("llm output contains forbidden marker %q", marker)
		}
	}
	if riskLevelRank(manual.Operation.RiskLevel) >= riskLevelRank("high") {
		for _, marker := range []string{"无需审批", "不需要审批", "直接执行生产恢复", "无需人工审批"} {
			if strings.Contains(text, marker) {
				return fmt.Errorf("llm output weakens high-risk approval requirement")
			}
		}
	}
	return nil
}

func workflowManualLLMPrompt(req WorkflowManualLLMSummaryRequest) string {
	prompt, _ := renderWorkflowManualLLMPrompt(req, "")
	return prompt
}

func renderWorkflowManualLLMPrompt(req WorkflowManualLLMSummaryRequest, promptTemplate string) (string, error) {
	payload := workflowManualLLMPromptPayload(req)
	promptTemplate = strings.TrimSpace(promptTemplate)
	if promptTemplate == "" {
		return payload, nil
	}
	replacements := map[string]string{
		"{{workflow_manual_payload}}": payload,
		"{{payload}}":                 payload,
		"{{language}}":                firstNonEmpty(req.Language, "zh-CN"),
	}
	rendered := promptTemplate
	for marker, value := range replacements {
		rendered = strings.ReplaceAll(rendered, marker, value)
	}
	if strings.Contains(rendered, "{{workflow_manual_payload}}") || strings.Contains(rendered, "{{payload}}") || strings.Contains(rendered, "{{language}}") {
		return "", fmt.Errorf("workflow manual llm prompt template contains unreplaced placeholders")
	}
	return rendered, nil
}

func workflowManualLLMPromptPayload(req WorkflowManualLLMSummaryRequest) string {
	redacted := map[string]any{
		"language":          firstNonEmpty(req.Language, "zh-CN"),
		"title":             req.Manual.Title,
		"workflow_ref":      req.Manual.WorkflowRef,
		"operation":         req.Manual.Operation,
		"applicability":     req.Manual.Applicability,
		"required_inputs":   req.Manual.RequiredContext.RequiredInputs,
		"risk_policy":       req.Manual.RiskPolicy,
		"preconditions":     req.Manual.Preconditions,
		"validation":        req.Manual.Validation,
		"cannot_use_when":   req.Manual.CannotUseWhen,
		"fallback":          req.Manual.FallbackGuide,
		"steps":             workflowManualLLMRedactedSteps(req.Analysis),
		"validation_status": req.Validation.Status,
		"task":              "请润色 document_markdown 和 user_summary。不要输出任何密钥、凭据引用、HTTP 鉴权请求头或原始脚本全文。必须保留高风险审批要求。",
		"output_schema": map[string]any{
			"document_markdown": "string",
			"user_summary": map[string]any{
				"understood": []string{},
				"missing":    []string{},
				"next_steps": []string{},
			},
		},
	}
	raw, _ := json.Marshal(redacted)
	return string(raw)
}

func defaultWorkflowManualLLMSystemPrompt() string {
	return "你是 AIOps 运维手册编辑器。只根据结构化摘要润色中文手册，不改变任何结构化字段、风险级别、审批要求或参数规则。输出严格 JSON。"
}

func parseWorkflowManualLLMResultContent(content string) (WorkflowManualLLMSummaryResult, error) {
	content = strings.TrimSpace(content)
	if strings.HasPrefix(content, "```") {
		if newline := strings.Index(content, "\n"); newline >= 0 {
			content = content[newline+1:]
		}
		content = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(content), "```"))
	}
	var result WorkflowManualLLMSummaryResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	return result, nil
}

func workflowManualLLMRedactedSteps(analysis WorkflowManualAnalysis) []map[string]string {
	out := make([]map[string]string, 0, len(analysis.Steps))
	for _, step := range analysis.Steps {
		out = append(out, map[string]string{
			"name":   step.Name,
			"action": step.Action,
			"stage":  step.Stage,
			"risk":   fmt.Sprint(step.Risky),
		})
	}
	return out
}

func sanitizeManualGenerationUserSummary(summary ManualGenerationUserSummary) ManualGenerationUserSummary {
	return ManualGenerationUserSummary{
		Understood: limitStrings(summary.Understood, 6),
		Missing:    limitStrings(summary.Missing, 8),
		NextSteps:  limitStrings(summary.NextSteps, 6),
	}
}

func chatCompletionsURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(baseURL, "/chat/completions") {
		return baseURL
	}
	return baseURL + "/chat/completions"
}

type openAIChatCompletionsRequest struct {
	Model       string              `json:"model"`
	Messages    []openAIChatMessage `json:"messages"`
	Temperature float64             `json:"temperature,omitempty"`
}

type openAIChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionsResponse struct {
	Choices []struct {
		Message openAIChatMessage `json:"message"`
	} `json:"choices"`
}
