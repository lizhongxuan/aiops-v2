package promptdiag

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// GenerateLLMSuggestions asks an OpenAI-compatible local endpoint for summary-only
// advice. It never sends full prompt text, tool output, or API keys in the payload.
func GenerateLLMSuggestions(ctx context.Context, cfg Config, diagnosis RunDiagnosis) ([]Suggestion, error) {
	baseURL := strings.TrimRight(strings.TrimSpace(cfg.LLMBaseURL), "/")
	apiKey := strings.TrimSpace(cfg.LLMAPIKey)
	model := strings.TrimSpace(cfg.LLMModel)
	if baseURL == "" {
		return nil, fmt.Errorf("llm base url is required")
	}
	if apiKey == "" {
		return nil, fmt.Errorf("llm api key is required")
	}
	if model == "" {
		return nil, fmt.Errorf("llm model is required")
	}
	body := map[string]any{
		"model":       model,
		"temperature": 0,
		"messages": []map[string]string{
			{
				"role": "system",
				"content": strings.Join([]string{
					"你是 agent prompt 优化诊断助手。",
					"只能基于用户给出的 eval 摘要、规则命中、计数、hash 变化和失败 check 给建议。",
					"不要要求查看完整 prompt；不要编造未提供的 tool output；不要输出源码 patch。",
					"输出 3 条以内中文建议，每条说明该改 prompt/tool/context/policy/completion_gate 哪一层。",
				}, "\n"),
			},
			{
				"role":    "user",
				"content": renderLLMSuggestionInput(diagnosis),
			},
		},
	}
	data, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	client := &http.Client{Timeout: 90 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("llm suggestions request failed: status=%d body=%s", resp.StatusCode, truncateString(string(respBody), 240))
	}
	var parsed struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("decode llm suggestions response: %w", err)
	}
	if len(parsed.Choices) == 0 || strings.TrimSpace(parsed.Choices[0].Message.Content) == "" {
		return nil, fmt.Errorf("llm suggestions response was empty")
	}
	return []Suggestion{{
		Area:        "llm_assisted",
		Action:      strings.TrimSpace(parsed.Choices[0].Message.Content),
		Rationale:   "基于 diagnosis 摘要生成；未发送完整 prompt。",
		LLMAssisted: true,
	}}, nil
}

func renderLLMSuggestionInput(d RunDiagnosis) string {
	type compactHit struct {
		RuleID    string `json:"ruleId"`
		Severity  string `json:"severity"`
		RootCause string `json:"rootCause"`
		Message   string `json:"message"`
		Evidence  string `json:"evidence,omitempty"`
	}
	type compactCase struct {
		CaseID               string       `json:"caseId"`
		Passed               bool         `json:"passed"`
		Score                float64      `json:"score"`
		Movement             string       `json:"movement,omitempty"`
		FailedChecks         []string     `json:"failedChecks,omitempty"`
		LikelyRootCause      string       `json:"likelyRootCause,omitempty"`
		ToolCalls            int          `json:"toolCalls"`
		ToolResults          int          `json:"toolResults"`
		FailedToolResults    int          `json:"failedToolResults"`
		ModelCalls           int          `json:"modelCalls"`
		PromptSizeChars      int          `json:"promptSizeChars"`
		VisibleToolCount     int          `json:"visibleToolCount"`
		MissingExpectedTools []string     `json:"missingExpectedTools,omitempty"`
		TraceTurnCount       int          `json:"traceTurnCount,omitempty"`
		TraceIterationCount  int          `json:"traceIterationCount,omitempty"`
		AnswerCharCount      int          `json:"answerCharCount,omitempty"`
		RuleHits             []compactHit `json:"ruleHits,omitempty"`
	}
	payload := struct {
		Summary DiagnosisSummary `json:"summary"`
		Cases   []compactCase    `json:"cases"`
	}{
		Summary: d.Summary,
	}
	for _, c := range d.Cases {
		if c.Passed && c.Movement != "worse" && len(c.RuleHits) == 0 {
			continue
		}
		cc := compactCase{
			CaseID:               c.CaseID,
			Passed:               c.Passed,
			Score:                c.Score,
			Movement:             c.Movement,
			FailedChecks:         c.FailedChecks,
			LikelyRootCause:      c.LikelyRootCause,
			ToolCalls:            c.Evidence.ToolCallCount,
			ToolResults:          c.Evidence.ToolResultCount,
			FailedToolResults:    c.Evidence.FailedToolResultCount,
			ModelCalls:           c.Evidence.ModelCallCount,
			PromptSizeChars:      c.Evidence.PromptSizeChars,
			VisibleToolCount:     len(c.Evidence.VisibleTools),
			MissingExpectedTools: c.Evidence.MissingExpectedTools,
			TraceTurnCount:       c.Evidence.TraceTurnCount,
			TraceIterationCount:  c.Evidence.TraceIterationCount,
			AnswerCharCount:      c.Evidence.AnswerCharCount,
		}
		for _, hit := range c.RuleHits {
			cc.RuleHits = append(cc.RuleHits, compactHit{
				RuleID:    hit.RuleID,
				Severity:  hit.Severity,
				RootCause: hit.RootCause,
				Message:   hit.Message,
				Evidence:  hit.Evidence,
			})
		}
		payload.Cases = append(payload.Cases, cc)
	}
	data, _ := json.MarshalIndent(payload, "", "  ")
	return string(data)
}

func truncateString(value string, max int) string {
	value = strings.TrimSpace(value)
	if len([]rune(value)) <= max {
		return value
	}
	runes := []rune(value)
	return string(runes[:max]) + "..."
}
