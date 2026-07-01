package runtimekernel

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/modelrouter"
)

func appendProviderNativeWebSearchTurnItems(snapshot *TurnSnapshot, iteration *IterationState, turnID string, events []modelrouter.ProviderNativeWebSearchEvent) {
	if snapshot == nil || iteration == nil || len(events) == 0 {
		return
	}
	for idx, event := range events {
		if !providerNativeWebSearchEventHasData(event) {
			continue
		}
		tc := providerNativeWebSearchToolCall(event, idx)
		result := ToolResult{
			ToolCallID: tc.ID,
			Content:    providerNativeWebSearchToolResultContent(event),
			Summary:    strings.TrimSpace(event.Summary),
		}
		iteration.ToolCalls = append(iteration.ToolCalls, tc)
		iteration.ToolResults = append(iteration.ToolResults, result)

		callItem := newAgentItem(toolCallItemID(turnID, tc), agentstate.TurnItemTypeToolCall, agentstate.ItemStatusCompleted, tc.Name, tc)
		callItem.Payload.Kind = "browser.search"
		appendAgentItem(snapshot, callItem)

		resultItem := newAgentItem(
			toolResultItemID(turnID, tc),
			agentstate.TurnItemTypeToolResult,
			toolResultItemStatus(result),
			truncateString(result.Content, 240),
			toolResultAgentItemData(turnID, tc, result),
		)
		resultItem.Payload.Kind = "browser.search"
		appendAgentItem(snapshot, resultItem)
	}
}

func providerNativeWebSearchToolCall(event modelrouter.ProviderNativeWebSearchEvent, index int) ToolCall {
	query := strings.TrimSpace(event.Query)
	args := map[string]string{}
	if query != "" {
		args["query"] = query
	}
	rawArgs, _ := json.Marshal(args)
	seed := firstNonEmpty(event.ID, query, event.Summary, fmt.Sprintf("provider-native-web-search-%d", index))
	return ToolCall{
		ID:        "provider-native-web-search-" + digestContent(seed)[:12],
		Name:      "web_search",
		Arguments: json.RawMessage(rawArgs),
	}
}

func providerNativeWebSearchToolResultContent(event modelrouter.ProviderNativeWebSearchEvent) string {
	type searchResult struct {
		Title   string `json:"title,omitempty"`
		URL     string `json:"url,omitempty"`
		Snippet string `json:"snippet,omitempty"`
	}
	results := make([]searchResult, 0, len(event.Sources))
	for _, source := range event.Sources {
		result := searchResult{
			Title:   strings.TrimSpace(source.Title),
			URL:     strings.TrimSpace(source.URL),
			Snippet: strings.TrimSpace(source.Snippet),
		}
		if result.Title == "" && result.URL == "" && result.Snippet == "" {
			continue
		}
		results = append(results, result)
	}
	content := strings.TrimSpace(event.Summary)
	if content == "" {
		content = providerNativeWebSearchContentSummary(event.Query, event.Sources)
	}
	payload := map[string]any{
		"query":   strings.TrimSpace(event.Query),
		"source":  providerNativeWebSearchSourceLabel(event.Provider),
		"content": content,
	}
	if len(results) > 0 {
		payload["results"] = results
	}
	data, _ := json.Marshal(payload)
	return string(data)
}

func providerNativeWebSearchContentSummary(query string, sources []modelrouter.ProviderNativeWebSearchSource) string {
	query = strings.TrimSpace(query)
	if query != "" {
		return fmt.Sprintf("provider-native web_search completed for query %q", query)
	}
	if len(sources) > 0 {
		return "provider-native web_search completed with cited sources"
	}
	return "provider-native web_search completed"
}

func providerNativeWebSearchSourceLabel(provider string) string {
	provider = strings.TrimSpace(provider)
	if provider == "" {
		return "provider_native:web_search"
	}
	return "provider_native:" + provider + ":web_search"
}

func providerNativeWebSearchEventHasData(event modelrouter.ProviderNativeWebSearchEvent) bool {
	return strings.TrimSpace(event.ID) != "" ||
		strings.TrimSpace(event.Query) != "" ||
		strings.TrimSpace(event.Summary) != "" ||
		len(event.Sources) > 0
}
