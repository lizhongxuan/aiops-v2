package runtimekernel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/diagnostics"
)

type modelTraceResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

func appendModelTraceResponse(tracePath, requestID string, response *schema.Message, duration time.Duration, callErr error) {
	if strings.TrimSpace(tracePath) == "" {
		return
	}
	if err := appendModelTraceResponseFile(tracePath, requestID, response, duration, callErr); err != nil {
		// Prompt Trace is diagnostic-only. Runtime behavior must not depend on it.
		return
	}
}

func appendModelTraceResponseFile(tracePath, requestID string, response *schema.Message, duration time.Duration, callErr error) error {
	if filepath.Ext(tracePath) != ".json" {
		return nil
	}
	data, err := os.ReadFile(tracePath)
	if err != nil {
		return err
	}
	var payload map[string]any
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}

	entry := modelTraceResponseEntry(requestID, response, duration, callErr)
	requests, _ := payload["llmRequests"].([]any)
	requests = append(requests, entry)
	payload["llmRequests"] = requests

	if usage, ok := entry["usage"]; ok {
		payload["usage"] = usage
	}
	if durationMs, ok := entry["duration_ms"]; ok {
		payload["duration_ms"] = durationMs
	}
	if output, ok := entry["output"]; ok {
		payload["output"] = output
	}
	if errText, ok := entry["error"]; ok {
		payload["error"] = errText
	}

	next, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tracePath, append(next, '\n'), 0o644)
}

func modelTraceResponseEntry(requestID string, response *schema.Message, duration time.Duration, callErr error) map[string]any {
	entry := map[string]any{
		"id": strings.TrimSpace(requestID),
	}
	if entry["id"] == "" {
		entry["id"] = "llm-request"
	}
	if duration > 0 {
		durationMs := duration.Milliseconds()
		if durationMs <= 0 {
			durationMs = 1
		}
		entry["duration_ms"] = durationMs
	}
	if response != nil {
		if output := strings.TrimSpace(diagnostics.RedactSensitiveText(response.Content)); output != "" {
			entry["output"] = output
		}
		if response.ResponseMeta != nil {
			if response.ResponseMeta.FinishReason != "" {
				entry["finishReason"] = response.ResponseMeta.FinishReason
			}
			if usage := modelTraceUsageFromResponse(response.ResponseMeta.Usage); usage != nil {
				entry["usage"] = usage
			}
		}
		if len(response.ToolCalls) > 0 {
			entry["toolCalls"] = modelTraceRedactedToolCalls(response.ToolCalls)
		}
	}
	if callErr != nil {
		entry["error"] = diagnostics.RedactSensitiveText(callErr.Error())
	}
	return entry
}

func modelTraceUsageFromResponse(usage *schema.TokenUsage) *modelTraceResponseUsage {
	if usage == nil {
		return nil
	}
	out := &modelTraceResponseUsage{
		PromptTokens:     usage.PromptTokens,
		CompletionTokens: usage.CompletionTokens,
		TotalTokens:      usage.TotalTokens,
	}
	if out.TotalTokens == 0 && (out.PromptTokens > 0 || out.CompletionTokens > 0) {
		out.TotalTokens = out.PromptTokens + out.CompletionTokens
	}
	if out.PromptTokens == 0 && out.CompletionTokens == 0 && out.TotalTokens == 0 {
		return nil
	}
	return out
}

func modelTraceRedactedToolCalls(calls []schema.ToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("tool-call-%d", index+1)
		}
		out = append(out, map[string]any{
			"id":        id,
			"name":      diagnostics.RedactSensitiveText(call.Function.Name),
			"arguments": diagnostics.RedactSensitiveText(call.Function.Arguments),
		})
	}
	return out
}
