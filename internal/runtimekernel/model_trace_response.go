package runtimekernel

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aiops-v2/internal/diagnostics"
	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/promptinput"
)

type modelTraceResponseUsage struct {
	PromptTokens     int `json:"prompt_tokens,omitempty"`
	CompletionTokens int `json:"completion_tokens,omitempty"`
	TotalTokens      int `json:"total_tokens,omitempty"`
}

type ModelStreamStats struct {
	FirstDeltaMs int64 `json:"first_delta_ms,omitempty"`
	StreamMs     int64 `json:"stream_ms,omitempty"`
	DeltaCount   int   `json:"delta_count,omitempty"`
	OutputChars  int   `json:"output_chars,omitempty"`
}

func appendModelTraceResponse(tracePath, requestID string, response modelrouter.ProviderResponse, duration time.Duration, callErr error, stats ...ModelStreamStats) {
	if strings.TrimSpace(tracePath) == "" {
		return
	}
	if err := appendModelTraceResponseFile(tracePath, requestID, response, duration, callErr, stats...); err != nil {
		// Prompt Trace is diagnostic-only. Runtime behavior must not depend on it.
		return
	}
}

func appendModelTraceResponseFile(tracePath, requestID string, response modelrouter.ProviderResponse, duration time.Duration, callErr error, stats ...ModelStreamStats) error {
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

	entry := modelTraceResponseEntry(requestID, response, duration, callErr, firstModelStreamStats(stats))
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
	if finishReason, ok := entry["finishReason"]; ok {
		payload["finishReason"] = finishReason
	}
	for _, key := range []string{"first_delta_ms", "stream_ms", "delta_count", "output_chars"} {
		if value, ok := entry[key]; ok {
			payload[key] = value
		}
	}

	next, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(tracePath, append(next, '\n'), 0o644)
}

func modelTraceResponseEntry(requestID string, response modelrouter.ProviderResponse, duration time.Duration, callErr error, stats ModelStreamStats) map[string]any {
	providerRequestID := strings.TrimSpace(response.RequestID)
	itemID := strings.TrimSpace(requestID)
	entry := map[string]any{
		"id": firstNonBlankRuntimeString(providerRequestID, itemID),
	}
	if entry["id"] == "" {
		entry["id"] = "llm-request"
	}
	if itemID != "" && itemID != entry["id"] {
		entry["itemId"] = itemID
	}
	if duration > 0 {
		entry["duration_ms"] = durationMilliseconds(duration)
	}
	if output := strings.TrimSpace(diagnostics.RedactSensitiveText(response.Output)); output != "" {
		entry["output"] = output
	}
	if finishReason := strings.TrimSpace(response.FinishReason); finishReason != "" {
		entry["finishReason"] = finishReason
	}
	if usage := modelTraceUsageFromProviderUsage(response.Usage); usage != nil {
		entry["usage"] = usage
	}
	if len(response.ToolCalls) > 0 {
		entry["toolCalls"] = modelTraceRedactedToolCalls(response.ToolCalls)
	}
	if callErr != nil {
		entry["error"] = diagnostics.RedactSensitiveText(callErr.Error())
	}
	appendModelStreamStats(entry, stats)
	return entry
}

func firstModelStreamStats(stats []ModelStreamStats) ModelStreamStats {
	if len(stats) == 0 {
		return ModelStreamStats{}
	}
	return stats[0]
}

func appendModelStreamStats(entry map[string]any, stats ModelStreamStats) {
	if stats.FirstDeltaMs > 0 {
		entry["first_delta_ms"] = stats.FirstDeltaMs
	}
	if stats.StreamMs > 0 {
		entry["stream_ms"] = stats.StreamMs
	}
	if stats.DeltaCount > 0 {
		entry["delta_count"] = stats.DeltaCount
	}
	if stats.OutputChars > 0 {
		entry["output_chars"] = stats.OutputChars
	}
}

func durationMilliseconds(duration time.Duration) int64 {
	if duration <= 0 {
		return 0
	}
	durationMs := duration.Milliseconds()
	if durationMs <= 0 {
		return 1
	}
	return durationMs
}

func modelTraceUsageFromProviderUsage(usage modelrouter.ProviderUsage) *modelTraceResponseUsage {
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

func modelTraceRedactedToolCalls(calls []promptinput.ModelInputToolCall) []map[string]any {
	out := make([]map[string]any, 0, len(calls))
	for index, call := range calls {
		id := strings.TrimSpace(call.ID)
		if id == "" {
			id = fmt.Sprintf("tool-call-%d", index+1)
		}
		out = append(out, map[string]any{
			"id":        id,
			"name":      diagnostics.RedactSensitiveText(call.Name),
			"arguments": diagnostics.RedactSensitiveText(string(call.Arguments)),
		})
	}
	return out
}
