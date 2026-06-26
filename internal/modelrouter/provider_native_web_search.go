package modelrouter

import (
	"encoding/json"
	"strings"
)

const ProviderNativeWebSearchExtraKey = "aiops.provider_native_web_search"

type ProviderNativeWebSearchEvent struct {
	ID       string                          `json:"id,omitempty"`
	Provider string                          `json:"provider,omitempty"`
	Query    string                          `json:"query,omitempty"`
	Summary  string                          `json:"summary,omitempty"`
	Sources  []ProviderNativeWebSearchSource `json:"sources,omitempty"`
}

type ProviderNativeWebSearchSource struct {
	Title   string `json:"title,omitempty"`
	URL     string `json:"url,omitempty"`
	Snippet string `json:"snippet,omitempty"`
}

func ExtractProviderNativeWebSearchEvents(raw []byte, provider string) []ProviderNativeWebSearchEvent {
	if len(raw) == 0 {
		return nil
	}
	var payload any
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil
	}
	provider = NormalizeProviderID(provider)
	events := collectProviderNativeWebSearchCalls(payload, provider)
	sources := collectProviderNativeWebSearchSources(payload)
	if len(events) == 0 {
		if len(sources) == 0 {
			return nil
		}
		return []ProviderNativeWebSearchEvent{{
			Provider: provider,
			Sources:  sources,
			Summary:  providerNativeWebSearchSummary("", sources),
		}}
	}
	events[0].Sources = mergeProviderNativeWebSearchSources(events[0].Sources, sources)
	for i := range events {
		if events[i].Provider == "" {
			events[i].Provider = provider
		}
		if events[i].Summary == "" {
			events[i].Summary = providerNativeWebSearchSummary(events[i].Query, events[i].Sources)
		}
	}
	return mergeProviderNativeWebSearchEvents(events)
}

func ProviderNativeWebSearchEventsFromExtra(extra map[string]any) []ProviderNativeWebSearchEvent {
	if len(extra) == 0 {
		return nil
	}
	raw, ok := extra[ProviderNativeWebSearchExtraKey]
	if !ok {
		return nil
	}
	switch typed := raw.(type) {
	case []ProviderNativeWebSearchEvent:
		return append([]ProviderNativeWebSearchEvent(nil), typed...)
	case ProviderNativeWebSearchEvent:
		return []ProviderNativeWebSearchEvent{typed}
	case []any:
		return providerNativeWebSearchEventsFromJSONLike(typed)
	case map[string]any:
		return providerNativeWebSearchEventsFromJSONLike([]any{typed})
	case string:
		var decoded []ProviderNativeWebSearchEvent
		if err := json.Unmarshal([]byte(typed), &decoded); err == nil {
			return decoded
		}
		var single ProviderNativeWebSearchEvent
		if err := json.Unmarshal([]byte(typed), &single); err == nil && providerNativeWebSearchEventHasData(single) {
			return []ProviderNativeWebSearchEvent{single}
		}
	}
	return nil
}

func providerNativeWebSearchEventsFromJSONLike(values []any) []ProviderNativeWebSearchEvent {
	out := make([]ProviderNativeWebSearchEvent, 0, len(values))
	for _, value := range values {
		raw, err := json.Marshal(value)
		if err != nil {
			continue
		}
		var event ProviderNativeWebSearchEvent
		if err := json.Unmarshal(raw, &event); err == nil && providerNativeWebSearchEventHasData(event) {
			out = append(out, event)
		}
	}
	return out
}

func providerNativeWebSearchEventHasData(event ProviderNativeWebSearchEvent) bool {
	return strings.TrimSpace(event.ID) != "" ||
		strings.TrimSpace(event.Query) != "" ||
		strings.TrimSpace(event.Summary) != "" ||
		len(event.Sources) > 0
}

func collectProviderNativeWebSearchCalls(value any, provider string) []ProviderNativeWebSearchEvent {
	var events []ProviderNativeWebSearchEvent
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			if strings.EqualFold(providerNativeStringField(typed, "type"), "web_search_call") {
				events = append(events, providerNativeWebSearchEventFromMap(typed, provider))
				return
			}
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return events
}

func providerNativeWebSearchEventFromMap(values map[string]any, provider string) ProviderNativeWebSearchEvent {
	action, _ := values["action"].(map[string]any)
	query := firstProviderNativeString(
		providerNativeStringField(values, "query", "search_query", "q"),
		providerNativeStringField(action, "query", "search_query", "q"),
	)
	return ProviderNativeWebSearchEvent{
		ID:       providerNativeStringField(values, "id", "call_id"),
		Provider: provider,
		Query:    query,
		Sources:  collectProviderNativeWebSearchSources(values),
	}
}

func collectProviderNativeWebSearchSources(value any) []ProviderNativeWebSearchSource {
	var sources []ProviderNativeWebSearchSource
	var walk func(any, string)
	walk = func(current any, parentKey string) {
		switch typed := current.(type) {
		case map[string]any:
			if source, ok := providerNativeWebSearchSourceFromMap(typed, parentKey); ok {
				sources = append(sources, source)
			}
			for key, child := range typed {
				walk(child, key)
			}
		case []any:
			for _, child := range typed {
				walk(child, parentKey)
			}
		}
	}
	walk(value, "")
	return mergeProviderNativeWebSearchSources(nil, sources)
}

func providerNativeWebSearchSourceFromMap(values map[string]any, parentKey string) (ProviderNativeWebSearchSource, bool) {
	url := providerNativeStringField(values, "url", "uri", "href")
	if strings.TrimSpace(url) == "" {
		return ProviderNativeWebSearchSource{}, false
	}
	typ := strings.ToLower(strings.TrimSpace(providerNativeStringField(values, "type")))
	parentKey = strings.ToLower(strings.TrimSpace(parentKey))
	if typ != "url_citation" &&
		typ != "web_search_result" &&
		parentKey != "sources" &&
		parentKey != "annotations" {
		return ProviderNativeWebSearchSource{}, false
	}
	return ProviderNativeWebSearchSource{
		Title:   providerNativeStringField(values, "title", "name", "source"),
		URL:     url,
		Snippet: providerNativeStringField(values, "snippet", "text", "content"),
	}, true
}

func mergeProviderNativeWebSearchEvents(events []ProviderNativeWebSearchEvent) []ProviderNativeWebSearchEvent {
	out := make([]ProviderNativeWebSearchEvent, 0, len(events))
	seen := map[string]int{}
	for _, event := range events {
		if !providerNativeWebSearchEventHasData(event) {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(firstProviderNativeString(event.ID, event.Query, event.Summary)))
		if key == "" && len(event.Sources) > 0 {
			key = strings.ToLower(strings.TrimSpace(event.Sources[0].URL + "|" + event.Sources[0].Title))
		}
		if idx, ok := seen[key]; ok {
			out[idx].Sources = mergeProviderNativeWebSearchSources(out[idx].Sources, event.Sources)
			if out[idx].Query == "" {
				out[idx].Query = event.Query
			}
			if out[idx].Summary == "" {
				out[idx].Summary = event.Summary
			}
			continue
		}
		seen[key] = len(out)
		out = append(out, event)
	}
	return out
}

func mergeProviderNativeWebSearchSources(existing []ProviderNativeWebSearchSource, incoming []ProviderNativeWebSearchSource) []ProviderNativeWebSearchSource {
	out := make([]ProviderNativeWebSearchSource, 0, len(existing)+len(incoming))
	seen := map[string]bool{}
	for _, source := range append(append([]ProviderNativeWebSearchSource(nil), existing...), incoming...) {
		source.Title = strings.TrimSpace(source.Title)
		source.URL = strings.TrimSpace(source.URL)
		source.Snippet = strings.TrimSpace(source.Snippet)
		if source.Title == "" && source.URL == "" && source.Snippet == "" {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(source.URL + "|" + source.Title))
		if key == "|" {
			key = strings.ToLower(source.Snippet)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, source)
	}
	return out
}

func providerNativeWebSearchSummary(query string, sources []ProviderNativeWebSearchSource) string {
	query = strings.TrimSpace(query)
	if query != "" && len(sources) == 0 {
		return "provider-native web_search completed for query " + strconvQuote(query)
	}
	if len(sources) == 0 {
		return "provider-native web_search completed"
	}
	lines := make([]string, 0, len(sources)+1)
	if query != "" {
		lines = append(lines, "provider-native web_search completed for query "+strconvQuote(query))
	} else {
		lines = append(lines, "provider-native web_search completed")
	}
	for _, source := range sources {
		label := firstProviderNativeString(source.Title, source.URL, source.Snippet)
		if source.URL != "" && source.Title != "" {
			label = source.Title + ": " + source.URL
		}
		if label != "" {
			lines = append(lines, "- "+label)
		}
	}
	return strings.Join(lines, "\n")
}

func providerNativeStringField(values map[string]any, keys ...string) string {
	if len(values) == 0 {
		return ""
	}
	for _, key := range keys {
		raw, ok := values[key]
		if !ok {
			continue
		}
		switch typed := raw.(type) {
		case string:
			if value := strings.TrimSpace(typed); value != "" {
				return value
			}
		}
	}
	return ""
}

func firstProviderNativeString(values ...string) string {
	for _, value := range values {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			return trimmed
		}
	}
	return ""
}

func strconvQuote(value string) string {
	raw, _ := json.Marshal(strings.TrimSpace(value))
	return string(raw)
}
