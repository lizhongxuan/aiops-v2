package opsmanual

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
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
	Client *http.Client
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
			summarizer = EnvWorkflowManualLLMSummarizer{}
		}
		candidate = applyWorkflowManualLLMSummary(ctx, candidate, analysis, summarizer)
	}
	return WorkflowManualGenerationResult{
		Candidate:        candidate,
		ValidationReport: candidate.StructuredValidationReport,
		UserSummary:      candidate.UserSummary,
	}, nil
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

func (s EnvWorkflowManualLLMSummarizer) SummarizeWorkflowManual(ctx context.Context, req WorkflowManualLLMSummaryRequest) (WorkflowManualLLMSummaryResult, error) {
	config, err := loadWorkflowManualLLMConfig()
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	client := s.Client
	if client == nil {
		client = &http.Client{Timeout: 90 * time.Second}
	}
	payload := openAIChatCompletionsRequest{
		Model: config.Model,
		Messages: []openAIChatMessage{
			{Role: "system", Content: "你是 AIOps 运维手册编辑器。只根据结构化摘要润色中文手册，不改变任何结构化字段、风险级别、审批要求或参数规则。输出严格 JSON。"},
			{Role: "user", Content: workflowManualLLMPrompt(req)},
		},
		Temperature: 0.2,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, chatCompletionsURL(config.BaseURL), bytes.NewReader(body))
	if err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+config.APIKey)
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
	content := strings.TrimSpace(decoded.Choices[0].Message.Content)
	content = strings.TrimPrefix(strings.TrimSuffix(content, "```"), "```json")
	content = strings.TrimSpace(content)
	var result WorkflowManualLLMSummaryResult
	if err := json.Unmarshal([]byte(content), &result); err != nil {
		return WorkflowManualLLMSummaryResult{}, err
	}
	return result, nil
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

type workflowManualLLMConfig struct {
	BaseURL string
	APIKey  string
	Model   string
}

func loadWorkflowManualLLMConfig() (workflowManualLLMConfig, error) {
	config := workflowManualLLMConfig{
		BaseURL: os.Getenv("AIOPS_LLM_BASE_URL"),
		APIKey:  os.Getenv("AIOPS_LLM_API_KEY"),
		Model:   os.Getenv("AIOPS_LLM_MODEL"),
	}
	if path := strings.TrimSpace(os.Getenv("AIOPS_LLM_CONFIG_FILE")); path != "" {
		raw, err := readWorkflowManualLLMConfigFile(path)
		if err != nil {
			return workflowManualLLMConfig{}, err
		}
		var fileConfig struct {
			BaseURL string `json:"baseURL"`
			APIKey  string `json:"apiKey"`
			Model   string `json:"model"`
		}
		if err := json.Unmarshal(raw, &fileConfig); err != nil {
			return workflowManualLLMConfig{}, err
		}
		if config.BaseURL == "" {
			config.BaseURL = fileConfig.BaseURL
		}
		if config.APIKey == "" {
			config.APIKey = fileConfig.APIKey
		}
		if config.Model == "" {
			config.Model = fileConfig.Model
		}
	}
	if strings.TrimSpace(config.BaseURL) == "" || strings.TrimSpace(config.APIKey) == "" || strings.TrimSpace(config.Model) == "" {
		return workflowManualLLMConfig{}, fmt.Errorf("llm config requires baseURL, apiKey and model")
	}
	config.BaseURL = strings.TrimRight(strings.TrimSpace(config.BaseURL), "/")
	return config, nil
}

func readWorkflowManualLLMConfigFile(path string) ([]byte, error) {
	raw, err := os.ReadFile(path)
	if err == nil || filepath.IsAbs(path) {
		return raw, err
	}
	wd, wdErr := os.Getwd()
	if wdErr != nil {
		return nil, err
	}
	for {
		candidate := filepath.Join(wd, path)
		raw, readErr := os.ReadFile(candidate)
		if readErr == nil {
			return raw, nil
		}
		parent := filepath.Dir(wd)
		if parent == wd {
			break
		}
		wd = parent
	}
	return nil, err
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
