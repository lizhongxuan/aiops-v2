package publicweb

import (
	"fmt"
	"strings"
)

func FormatSearchEnvelope(req SearchRequest, results []SearchResult, meta ResultMeta) ResultEnvelope {
	if meta.Backend == "" {
		meta.Backend = "lightweight_search"
	}
	content := formatSearchContent(req.Query, results, meta)
	source := "custom_public_web:search"
	if len(results) > 0 && results[0].Provider == "curated_fallback" {
		source = "custom_public_web:official_domain_fallback"
		if !containsString(meta.Fallbacks, "official_domain_fallback") {
			meta.Fallbacks = append(meta.Fallbacks, "official_domain_fallback")
		}
	}
	return ResultEnvelope{
		Operation: OperationSearch,
		Query:     req.Query,
		Source:    source,
		Content:   content,
		Results:   results,
		Meta:      meta,
	}
}

func FormatOpenEnvelope(req SearchRequest, result SearchResult, meta ResultMeta) ResultEnvelope {
	if meta.Backend == "" {
		meta.Backend = "internal_fetch"
	}
	if result.Fetched {
		meta.FetchedCount = 1
	}
	meta.FinalURL = firstNonEmpty(meta.FinalURL, result.URL)
	content := result.Text
	if content == "" {
		content = result.Snippet
	}
	return ResultEnvelope{
		Operation: OperationOpen,
		URL:       firstNonEmpty(result.URL, req.URL),
		Source:    "custom_public_web:open",
		Content:   content,
		Results:   []SearchResult{result},
		Meta:      meta,
	}
}

func FormatProviderNativeEnvelope(req SearchRequest, content, source string) ResultEnvelope {
	return ResultEnvelope{
		Operation: OperationSearch,
		Query:     req.Query,
		Source:    firstNonEmpty(source, "provider_native:web_search"),
		Content:   content,
		Meta: ResultMeta{
			Backend:                 "provider_native",
			ProviderNativeAttempted: true,
		},
	}
}

func formatSearchContent(query string, results []SearchResult, meta ResultMeta) string {
	var b strings.Builder
	if containsString(meta.Fallbacks, "official_domain_fallback") || (len(results) > 0 && results[0].Provider == "curated_fallback") {
		fmt.Fprintf(&b, "Official-domain fallback results for %q. Public search returned no relevant result, so these known official docs are provided as starting points. Use web_search with operation=\"open\" and the selected official URL before giving version-sensitive operational guidance, and cite URLs:\n", query)
	} else {
		fmt.Fprintf(&b, "Public web search results for %q. Use these results as evidence and cite URLs. Use web_search with operation=\"open\" and a result URL when full page text is needed:\n", query)
	}
	for i, result := range results {
		fmt.Fprintf(&b, "%d. %s\n", i+1, firstNonEmpty(result.Title, result.URL))
		if result.URL != "" {
			fmt.Fprintf(&b, "   URL: %s\n", result.URL)
		}
		if result.Snippet != "" {
			fmt.Fprintf(&b, "   Snippet: %s\n", result.Snippet)
		}
		if result.Fetched && result.Text != "" {
			fmt.Fprintf(&b, "   Fetched text: %s\n", truncateBytes(result.Text, 1200))
		}
		if result.FetchError != "" {
			fmt.Fprintf(&b, "   Fetch error: %s\n", result.FetchError)
		}
	}
	return b.String()
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
