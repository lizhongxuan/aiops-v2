package publicweb

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLightweightBackendExpandsCandidatesThenFiltersToLimit(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		var body strings.Builder
		body.WriteString(`<html><body><ol id="b_results">`)
		for i := 0; i < 8; i++ {
			body.WriteString(`<li class="b_algo"><h2><a href="https://docs.example.com/postgresql-`)
			body.WriteString(string(rune('a' + i)))
			body.WriteString(`">PostgreSQL timeline official docs result</a></h2><div class="b_caption"><p>PostgreSQL recovery_target_timeline official docs and recovery timeline details.</p></div></li>`)
		}
		body.WriteString(`</ol></body></html>`)
		_, _ = io.WriteString(w, body.String())
	}))
	defer server.Close()

	backend := NewLightweightBackend(server.Client(), server.URL)
	results, err := backend.Search(context.Background(), SearchRequest{Query: "PostgreSQL recovery_target_timeline official docs", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want requested limit 5 after larger candidate parse", len(results))
	}
}

func TestLightweightBackendAppliesAllowedDomainQueryRewrite(t *testing.T) {
	var gotQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><body><ol id="b_results"><li class="b_algo"><h2><a href="https://www.postgresql.org/docs/current/continuous-archiving.html">PostgreSQL official docs</a></h2><div class="b_caption"><p>PostgreSQL official docs recovery timeline.</p></div></li></ol></body></html>`)
	}))
	defer server.Close()

	backend := NewLightweightBackend(server.Client(), server.URL)
	if _, err := backend.Search(context.Background(), SearchRequest{Query: "PostgreSQL timeline official docs", AllowedDomains: []string{"postgresql.org"}, Limit: 5}); err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if !strings.Contains(gotQuery, "site:postgresql.org") {
		t.Fatalf("search query = %q, want site filter", gotQuery)
	}
}

func TestLightweightBackendOfficialDomainFallbackReturnsStructuredResults(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = io.WriteString(w, `<html><body><ol id="b_results"></ol></body></html>`)
	}))
	defer server.Close()

	backend := NewLightweightBackend(server.Client(), server.URL)
	results, err := backend.Search(context.Background(), SearchRequest{Query: "pgBackRest restore recovery_target_timeline pg_auto_failover PostgreSQL official docs", Limit: 5})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(results) == 0 || results[0].URL == "" {
		t.Fatalf("results = %+v, want official structured fallback results", results)
	}
}

func TestRelevanceTermsSplitCompactChineseQuery(t *testing.T) {
	results := []SearchResult{
		{
			Title:   "两会新华社权威速览丨一图速览 2026 年政府工作报告",
			URL:     "https://example.com/government-report",
			Snippet: "政府工作报告摘要。",
		},
		{
			Title:   "上海证券交易所 2026 年部分节假日休市安排",
			URL:     "https://www.sse.com.cn/disclosure/announcement/general/",
			Snippet: "官方发布节假日休市安排。",
		},
	}

	filtered := filterByRelevance(results, "2026年部分节假日休市安排 上海证券交易所 官方")
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1: %#v", len(filtered), filtered)
	}
	if strings.Contains(filtered[0].Title, "政府工作报告") {
		t.Fatalf("filtered = %#v, should drop low-relevance generic 2026 result", filtered)
	}
	if !strings.Contains(filtered[0].Title, "上海证券交易所") {
		t.Fatalf("filtered = %#v, want exchange result", filtered)
	}
}

func TestRelevanceDropsDateOnlyMatches(t *testing.T) {
	results := []SearchResult{
		{
			Title:   "2026 年_百度百科",
			URL:     "https://baike.baidu.com/item/2026%E5%B9%B4/9536516",
			Snippet: "2026 年日期信息。",
		},
		{
			Title:   "中国 A股 交易日 上交所 深交所 周日休市说明",
			URL:     "https://example.com/ashare-trading-day",
			Snippet: "中国 A股 今天 是否交易日，交易日安排以上交所深交所公告为准。",
		},
	}

	filtered := filterByRelevance(results, "2026-04-26 中国 A股 今天 是否 交易日 上交所 深交所 周日")
	if len(filtered) != 1 {
		t.Fatalf("filtered len = %d, want 1: %#v", len(filtered), filtered)
	}
	if strings.Contains(filtered[0].Title, "百度百科") {
		t.Fatalf("filtered = %#v, should drop date-only result", filtered)
	}
}

func TestParseBingResultsHandlesHTMLAttributeVariants(t *testing.T) {
	results := parseBingResults(`<html><body><ol id='b_results'>
		<li data-id='1' class='result b_algo extra'>
			<h2><a data-track='x' href='https://example.com/market'><span>Market</span> <strong>report</strong></a></h2>
			<div class='b_caption'><p>Index <em>moved</em> higher.</p></div>
		</li>
	</ol></body></html>`, 5)

	if len(results) != 1 {
		t.Fatalf("results len = %d, want 1: %#v", len(results), results)
	}
	if results[0].Title != "Market report" {
		t.Fatalf("Title = %q, want nested anchor text", results[0].Title)
	}
	if results[0].URL != "https://example.com/market" {
		t.Fatalf("URL = %q, want href", results[0].URL)
	}
	if results[0].Snippet != "Index moved higher." {
		t.Fatalf("Snippet = %q, want caption text", results[0].Snippet)
	}
}

func TestFormatEnvelopePreservesLegacyContentAndStructuredResults(t *testing.T) {
	envelope := FormatSearchEnvelope(SearchRequest{Query: "PostgreSQL timeline", Limit: 5}, []SearchResult{{
		Title:   "PostgreSQL official docs",
		URL:     "https://www.postgresql.org/docs/current/continuous-archiving.html",
		Snippet: "Official recovery docs.",
	}}, ResultMeta{Backend: "lightweight_search"})
	if envelope.Results[0].URL == "" || envelope.Content == "" || envelope.Meta.Backend == "" {
		t.Fatalf("envelope = %+v, want structured results plus legacy content", envelope)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	if !strings.Contains(string(data), `"results"`) || !strings.Contains(string(data), `"content"`) {
		t.Fatalf("json = %s, want content and results fields", data)
	}
}
