# aiops-v2 Custom Web Search Firecrawl-Inspired Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 将 aiops-v2 的自定义公开网页能力收敛为一个 `web_search` tool，同时保留 provider-native `web_search` 逻辑不变，并把 Firecrawl 可借鉴的结构化结果、候选扩大、安全抓取、搜索后正文增强移植进 aiops-v2 内部实现。

**Architecture:** Provider-native `web_search` 继续走现有 OpenAI/兼容 provider 分支；只有非 native provider 或 provider-native 无可用文本后的 fallback 才进入 `internal/integrations/publicweb`。`web_search` 通过 `operation=search|open` 表达搜索和打开 URL；`browse_url` 只作为兼容 alias，调用同一条 `publicweb.Broker` 路径，不保留独立 HTTP 抓取逻辑。所有搜索/打开结果输出稳定 JSON envelope，并继续投影为 `browser.search -> ProcessTranscript web_lookup`。

**Tech Stack:** Go `internal/integrations/localtools`, new Go package `internal/integrations/publicweb`, runtime/tooling visibility tests, appui transport projector tests, React ProcessTranscript tests, Playwright/browser-in-app verification.

---

## Source Spec

基于设计文档：

`/Users/lizhongxuan/Desktop/aiops/aiops-v2/docs/superpowers/specs/2026-06-28-aiops-v2-web-search-firecrawl-optimization-design.zh.md`

本计划落实 spec 的 P0/P1 范围：

- P0：单一 `web_search` 输入/输出契约、结构化 `results/meta`、候选扩大、safe fetch、`browse_url` alias、provider-native 不变。
- P1：`fetch_content` 搜索后正文增强、main content extraction、失败隔离、UI 展示“已读取正文”。
- P2：DuckDuckGo/SearXNG 多源 fallback、cache、轻量 categories 只作为后续可选任务，不进入第一轮默认实现。

## Scope Rules

- [x] `web_search` 是唯一默认模型可见公开网页入口，支持 `operation=search` 和 `operation=open`。
- [x] 如果 provider 支持原生 `web_search`，请求和响应归一化路径保持现有行为。
- [x] `browse_url` 只作为历史兼容 alias，必须调用 `web_search(operation=open)` 的同一实现。
- [x] 不新增 Firecrawl 服务依赖、API key、base URL、部署编排或 `firecrawl_*` tool。
- [x] `operation=open` 必须防 SSRF：拒绝 localhost、loopback、private IP、link-local、metadata IP、非 http(s)，并校验重定向后的最终地址。
- [x] `ToolResult.Content` 必须保留旧字段 `query/source/content`，并新增稳定 `operation/url/results/meta`。
- [x] UI 不新增 Firecrawl 分支；仍只通过 `browser.search` 折叠组展示。
- [x] 正文增强必须有硬预算，抓取失败不能让 search 整体失败。

## File Structure

**Create**

- `internal/integrations/publicweb/types.go`：定义 `SearchRequest`、`FetchRequest`、`SearchResult`、`ResultEnvelope`、`Backend`、`Broker` 的公共结构。
- `internal/integrations/publicweb/request.go`：解析和校验 `web_search` JSON 输入，处理 `operation` 默认值、domain filter、limit、budget。
- `internal/integrations/publicweb/request_test.go`：覆盖 search/open 输入、非法 URL、domain 冲突、默认值。
- `internal/integrations/publicweb/safe_fetch.go`：实现防 SSRF 的 HTTP fetch，禁止私网、localhost、metadata IP，并处理重定向校验。
- `internal/integrations/publicweb/safe_fetch_test.go`：覆盖 file/local/private scheme、loopback、重定向到私网、正常公网 fixture。
- `internal/integrations/publicweb/readable.go`：HTML readable text / markdown-like extraction，复用当前 `htmlToReadableText` 语义并补 title/canonical/status metadata。
- `internal/integrations/publicweb/search_backend.go`：实现当前 Bing HTML lightweight backend、query builder、候选扩大、domain filter、relevance filter。
- `internal/integrations/publicweb/search_backend_test.go`：覆盖 Bing 解析、`limit*2` 候选、domain filter、同域限制、官方域 fallback。
- `internal/integrations/publicweb/broker.go`：统一 search/open 路由、搜索后正文增强、失败隔离、envelope 生成。
- `internal/integrations/publicweb/broker_test.go`：覆盖 `operation=search/open`、`fetch_content`、`fetch_error`、`meta`。
- `internal/integrations/publicweb/format.go`：生成兼容旧 `content` 文本和新 `results/meta` JSON。

**Modify**

- `internal/integrations/localtools/register.go`：`NewWebSearchTool` 使用 `publicweb.Broker`；`NewBrowseURLTool` 变成 alias；删除或迁移旧 browse fetch/search helper。
- `internal/integrations/localtools/register_test.go`：更新 web_search/browse_url 注册、schema、provider-native、fallback、alias、安全校验测试。
- `internal/runtimekernel/runtime_kernel.go`：把 public-web synthesis-only 文案中的 `browse_url` 改成 `web_search(operation=open)`。
- `internal/runtimekernel/react_loop_test.go`：更新 public web tool names / budget / surface 测试，确保没有默认模型可见 `browse_url`。
- `internal/runtimekernel/model_input_trace_test.go`：更新 Prompt Trace 中 public web tool surface，`browse_url` 只作为兼容 alias 或隐藏工具。
- `internal/tooling/turn_metadata_filter_test.go`：如现有测试明确期待 public_web pack 有两个可见工具，改成一个默认可见工具。
- `internal/appui/transport_projector.go`：优先读取 `results`，保留 content parser；识别 `operation=open` 的 URL summary；投影 `fetched` 状态。
- `internal/appui/transport_projector_test.go`：覆盖 structured `results/meta`、open URL、fetched/fetch_error、provider-native 不变。
- `web/src/transport/aiopsTransportTypes.ts`：如后端新增字段需要前端类型，补 `operation`、`fetched`、`fetchError` 或复用已有 source result 类型。
- `web/src/chat/components/ProcessTranscript.tsx`：继续显示 `网页检索 N 次 · 找到 M 个来源`；展开结果里显示“已读取正文”或“读取失败”轻量状态。
- `web/src/chat/components/ProcessTranscript.test.tsx`：覆盖 search/open 合并展示、fetched 状态、没有 `browse_url` 独立折叠体验。
- `web/tests/react-shell-snapshot.spec.js`：更新 web lookup fixture 和 snapshot assertion。

**Do Not Modify For This Feature**

- 不新增 Firecrawl runtime package、Firecrawl API client、Firecrawl env config。
- 不新增 `firecrawl_search`、`web_open`、`browse_url_v2` 等第二个模型可见网页 tool。
- 不把 final answer / Markdown citation parser 作为搜索来源解析主路径。
- 不在 P0/P1 做 full-site crawl、map、screenshot、agent、query highlights model。

## Subagent File Boundaries

如果使用 subagent-driven development，建议这样分工：

- Agent A：`internal/integrations/publicweb/**`，只做新 package 和单元测试。
- Agent B：`internal/integrations/localtools/**`、`internal/runtimekernel/**`、`internal/tooling/**`，只做 tool 集成和 runtime 可见性。
- Agent C：`internal/appui/**`、`web/src/**`、`web/tests/**`，只做 transport/UI/Playwright。
- 主 agent：统一解决接口冲突、跑全量验证、更新本计划状态。

---

### Task 1: Lock PublicWeb Request Contract

**Files:**
- Create: `internal/integrations/publicweb/types.go`
- Create: `internal/integrations/publicweb/request.go`
- Create: `internal/integrations/publicweb/request_test.go`

- [x] **Step 1: Write failing request parser tests**

Create `internal/integrations/publicweb/request_test.go`:

```go
package publicweb

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRequestDefaultsToSearch(t *testing.T) {
	req, err := ParseRequest(json.RawMessage(`{
		"query":"PostgreSQL recovery_target_timeline official docs",
		"allowed_domains":["postgresql.org"],
		"limit":0
	}`))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if req.Operation != OperationSearch || req.Query == "" {
		t.Fatalf("request = %+v, want search operation with query", req)
	}
	if req.Limit != 5 || req.MaxContentResults != 2 || req.MaxBytes != 20000 {
		t.Fatalf("defaults = limit:%d maxContent:%d maxBytes:%d", req.Limit, req.MaxContentResults, req.MaxBytes)
	}
	if len(req.AllowedDomains) != 1 || req.AllowedDomains[0] != "postgresql.org" {
		t.Fatalf("allowed domains = %#v", req.AllowedDomains)
	}
}

func TestParseRequestDefaultsToOpenWhenURLPresent(t *testing.T) {
	req, err := ParseRequest(json.RawMessage(`{
		"url":"https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-RECOVERY-TARGET-TIMELINE",
		"max_bytes":12000
	}`))
	if err != nil {
		t.Fatalf("ParseRequest() error = %v", err)
	}
	if req.Operation != OperationOpen || req.URL == "" || req.MaxBytes != 12000 {
		t.Fatalf("request = %+v, want open operation with URL and max bytes", req)
	}
}

func TestParseRequestRejectsInvalidOperationInputs(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"search missing query", `{"operation":"search"}`, "query is required"},
		{"open missing url", `{"operation":"open"}`, "url is required"},
		{"bad operation", `{"operation":"crawl","query":"docs"}`, "operation"},
		{"domain conflict", `{"query":"docs","allowed_domains":["postgresql.org"],"blocked_domains":["postgresql.org"]}`, "cannot both"},
		{"bad domain", `{"query":"docs","allowed_domains":["https://postgresql.org/path"]}`, "hostname without protocol or path"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseRequest(json.RawMessage(tc.input))
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("ParseRequest() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}
```

- [x] **Step 2: Run the tests and verify they fail**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestParseRequest' -count=1
```

Expected: FAIL because package/functions do not exist yet.

- [x] **Step 3: Add publicweb types**

Create `internal/integrations/publicweb/types.go`:

```go
package publicweb

import (
	"context"
	"time"
)

const (
	OperationSearch = "search"
	OperationOpen   = "open"

	DefaultLimit             = 5
	DefaultMaxContentResults = 2
	DefaultMaxBytes          = 20000
	DefaultTimeout           = 60 * time.Second
)

type SearchRequest struct {
	Operation         string
	Query             string
	URL               string
	SearchContextSize string
	AllowedDomains    []string
	BlockedDomains    []string
	Limit             int
	TimeRange         string
	Language          string
	Country           string
	Location          string
	FetchContent      bool
	MaxContentResults int
	ContentFormats    []string
	MaxBytes          int
	Timeout           time.Duration
}

type FetchRequest struct {
	URL            string
	AllowedDomains []string
	BlockedDomains []string
	MaxBytes       int
	Timeout        time.Duration
}

type SearchResult struct {
	Title       string `json:"title,omitempty"`
	URL         string `json:"url,omitempty"`
	Snippet     string `json:"snippet,omitempty"`
	Text        string `json:"text,omitempty"`
	Markdown    string `json:"markdown,omitempty"`
	Source      string `json:"source,omitempty"`
	Provider    string `json:"provider,omitempty"`
	ContentKind string `json:"contentKind,omitempty"`
	Fetched     bool   `json:"fetched,omitempty"`
	FetchError  string `json:"fetchError,omitempty"`
	StatusCode  int    `json:"statusCode,omitempty"`
	ContentType string `json:"contentType,omitempty"`
	FetchedAt   string `json:"fetchedAt,omitempty"`
}

type ResultMeta struct {
	Backend                 string   `json:"backend,omitempty"`
	ProviderNativeAttempted bool     `json:"providerNativeAttempted,omitempty"`
	Fallbacks               []string `json:"fallbacks,omitempty"`
	FetchedCount            int      `json:"fetchedCount,omitempty"`
	Truncated               bool     `json:"truncated,omitempty"`
	FinalURL                string   `json:"finalUrl,omitempty"`
}

type ResultEnvelope struct {
	Operation string         `json:"operation"`
	Query     string         `json:"query,omitempty"`
	URL       string         `json:"url,omitempty"`
	Source    string         `json:"source"`
	Content   string         `json:"content"`
	Results   []SearchResult `json:"results"`
	Meta      ResultMeta     `json:"meta"`
}

type SearchBackend interface {
	Name() string
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
}

type Fetcher interface {
	Fetch(ctx context.Context, req FetchRequest) (SearchResult, error)
}
```

- [x] **Step 4: Add request parser implementation**

Create `internal/integrations/publicweb/request.go`:

```go
package publicweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

type rawRequest struct {
	Operation         string   `json:"operation"`
	Query             string   `json:"query"`
	URL               string   `json:"url"`
	SearchContextSize string   `json:"search_context_size"`
	AllowedDomains    []string `json:"allowed_domains"`
	BlockedDomains    []string `json:"blocked_domains"`
	Limit             int      `json:"limit"`
	TimeRange         string   `json:"time_range"`
	Language          string   `json:"language"`
	Country           string   `json:"country"`
	Location          string   `json:"location"`
	FetchContent      bool     `json:"fetch_content"`
	MaxContentResults int      `json:"max_content_results"`
	ContentFormats    []string `json:"content_formats"`
	MaxBytesSnake     int      `json:"max_bytes"`
	MaxBytesCamel     int      `json:"maxBytes"`
}

func ParseRequest(input json.RawMessage) (SearchRequest, error) {
	var raw rawRequest
	if err := json.Unmarshal(input, &raw); err != nil {
		return SearchRequest{}, fmt.Errorf("invalid web_search input: %w", err)
	}
	req := SearchRequest{
		Operation:         strings.ToLower(strings.TrimSpace(raw.Operation)),
		Query:             strings.TrimSpace(raw.Query),
		URL:               strings.TrimSpace(raw.URL),
		SearchContextSize: strings.TrimSpace(raw.SearchContextSize),
		Limit:             raw.Limit,
		TimeRange:         strings.TrimSpace(raw.TimeRange),
		Language:          strings.TrimSpace(raw.Language),
		Country:           strings.TrimSpace(raw.Country),
		Location:          strings.TrimSpace(raw.Location),
		FetchContent:      raw.FetchContent,
		MaxContentResults: raw.MaxContentResults,
		ContentFormats:    normalizeContentFormats(raw.ContentFormats),
		MaxBytes:          firstPositive(raw.MaxBytesSnake, raw.MaxBytesCamel),
		Timeout:           DefaultTimeout,
	}
	if req.Operation == "" {
		if req.URL != "" {
			req.Operation = OperationOpen
		} else {
			req.Operation = OperationSearch
		}
	}
	if req.Operation != OperationSearch && req.Operation != OperationOpen {
		return SearchRequest{}, fmt.Errorf("invalid operation %q", req.Operation)
	}
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}
	if req.MaxContentResults <= 0 {
		req.MaxContentResults = DefaultMaxContentResults
	}
	if req.MaxBytes <= 0 {
		req.MaxBytes = DefaultMaxBytes
	}
	if req.SearchContextSize == "" {
		req.SearchContextSize = "medium"
	}
	var err error
	req.AllowedDomains, err = NormalizeDomainFilters(raw.AllowedDomains)
	if err != nil {
		return SearchRequest{}, fmt.Errorf("invalid allowed_domains: %w", err)
	}
	req.BlockedDomains, err = NormalizeDomainFilters(raw.BlockedDomains)
	if err != nil {
		return SearchRequest{}, fmt.Errorf("invalid blocked_domains: %w", err)
	}
	if len(req.AllowedDomains) > 0 && len(req.BlockedDomains) > 0 {
		return SearchRequest{}, errors.New("allowed_domains and blocked_domains cannot both be specified")
	}
	if req.Operation == OperationSearch && req.Query == "" {
		return SearchRequest{}, errors.New("query is required for operation=search")
	}
	if req.Operation == OperationOpen && req.URL == "" {
		return SearchRequest{}, errors.New("url is required for operation=open")
	}
	return req, nil
}

func NormalizeDomainFilters(values []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		domain, err := normalizeDomainFilter(value)
		if err != nil {
			return nil, err
		}
		if domain == "" || seen[domain] {
			continue
		}
		seen[domain] = true
		out = append(out, domain)
	}
	return out, nil
}

func normalizeDomainFilter(value string) (string, error) {
	raw := strings.ToLower(strings.TrimSpace(value))
	raw = strings.TrimPrefix(raw, "site:")
	raw = strings.TrimPrefix(raw, "*.")
	if raw == "" {
		return "", errors.New("domain cannot be empty")
	}
	if strings.Contains(raw, "://") || strings.Contains(raw, "/") {
		return "", fmt.Errorf("domain %q must be a hostname without protocol or path", value)
	}
	parsed, err := url.Parse("https://" + raw)
	if err != nil || parsed.Hostname() == "" {
		return "", fmt.Errorf("domain %q must be a valid hostname without protocol or path", value)
	}
	host := strings.Trim(parsed.Hostname(), ".")
	if host == "" || strings.ContainsAny(host, " /\\\t\r\n") {
		return "", fmt.Errorf("domain %q must be a valid hostname without protocol or path", value)
	}
	return host, nil
}

func normalizeContentFormats(values []string) []string {
	if len(values) == 0 {
		return []string{"text"}
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		switch value {
		case "text", "markdown":
			if !seen[value] {
				seen[value] = true
				out = append(out, value)
			}
		}
	}
	if len(out) == 0 {
		return []string{"text"}
	}
	return out
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}
```

- [x] **Step 5: Run request parser tests**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestParseRequest' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 1**

```bash
git add internal/integrations/publicweb/types.go internal/integrations/publicweb/request.go internal/integrations/publicweb/request_test.go
git commit -m "feat: add public web request contract"
```

---

### Task 2: Add Safe Fetch And Readable Extraction

**Files:**
- Create: `internal/integrations/publicweb/safe_fetch.go`
- Create: `internal/integrations/publicweb/safe_fetch_test.go`
- Create: `internal/integrations/publicweb/readable.go`

- [x] **Step 1: Write failing safe fetch tests**

Create `internal/integrations/publicweb/safe_fetch_test.go`:

```go
package publicweb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSafeFetchRejectsUnsafeURLs(t *testing.T) {
	fetcher := NewSafeFetcher(http.DefaultClient)
	cases := []string{
		"file:///etc/passwd",
		"http://127.0.0.1/latest/meta-data",
		"http://localhost:8080",
		"http://169.254.169.254/latest/meta-data",
		"http://10.0.0.1/internal",
	}
	for _, rawURL := range cases {
		t.Run(rawURL, func(t *testing.T) {
			_, err := fetcher.Fetch(context.Background(), FetchRequest{URL: rawURL, MaxBytes: 1000})
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsafe") {
				t.Fatalf("Fetch(%q) error = %v, want unsafe rejection", rawURL, err)
			}
		})
	}
}

func TestSafeFetchRejectsRedirectToUnsafeAddress(t *testing.T) {
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://127.0.0.1/private", http.StatusFound)
	}))
	defer redirect.Close()

	fetcher := NewSafeFetcher(redirect.Client())
	_, err := fetcher.Fetch(context.Background(), FetchRequest{URL: redirect.URL, MaxBytes: 1000})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsafe") {
		t.Fatalf("Fetch() error = %v, want unsafe redirect rejection", err)
	}
}

func TestSafeFetchExtractsReadableHTML(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<!doctype html><html><head><title>Docs</title><script>bad()</script></head><body><main><h1>Docs</h1><p>Readable text.</p></main><style>.x{}</style></body></html>`))
	}))
	defer server.Close()

	fetcher := NewSafeFetcher(server.Client())
	result, err := fetcher.Fetch(context.Background(), FetchRequest{URL: server.URL, MaxBytes: 4000})
	if err != nil {
		t.Fatalf("Fetch() error = %v", err)
	}
	if !result.Fetched || result.StatusCode != 200 || !strings.Contains(result.Text, "Readable text.") {
		t.Fatalf("result = %+v, want fetched readable text", result)
	}
	if strings.Contains(result.Text, "bad()") || strings.Contains(result.Text, ".x{}") {
		t.Fatalf("text = %q, should strip script/style", result.Text)
	}
}
```

- [x] **Step 2: Run tests and verify unsafe checks fail**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestSafeFetch' -count=1
```

Expected: FAIL because `NewSafeFetcher` does not exist.

- [x] **Step 3: Implement safe fetch**

Create `internal/integrations/publicweb/safe_fetch.go`:

```go
package publicweb

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"strings"
	"time"
)

type SafeFetcher struct {
	client *http.Client
}

func NewSafeFetcher(client *http.Client) *SafeFetcher {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	next := *client
	next.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return fmt.Errorf("too many redirects")
		}
		return validatePublicHTTPURL(req.URL.String())
	}
	return &SafeFetcher{client: &next}
}

func (f *SafeFetcher) Fetch(ctx context.Context, req FetchRequest) (SearchResult, error) {
	if err := validatePublicHTTPURL(req.URL); err != nil {
		return SearchResult{}, err
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, req.URL, nil)
	if err != nil {
		return SearchResult{}, err
	}
	httpReq.Header.Set("User-Agent", "aiops-v2-web-search/1.0")
	httpReq.Header.Set("Accept", "text/html,text/plain,application/xhtml+xml;q=0.9,*/*;q=0.5")
	resp, err := f.client.Do(httpReq)
	if err != nil {
		return SearchResult{}, err
	}
	defer resp.Body.Close()
	if err := validatePublicHTTPURL(resp.Request.URL.String()); err != nil {
		return SearchResult{}, err
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	text, title := readableText(string(body), resp.Header.Get("Content-Type"))
	result := SearchResult{
		Title:       firstNonEmpty(title, resp.Request.URL.Hostname()),
		URL:         resp.Request.URL.String(),
		Text:        truncateBytes(text, maxBytes),
		Snippet:     truncateBytes(text, 600),
		Source:      "custom_public_web",
		Provider:    "internal_fetch",
		ContentKind: "text",
		Fetched:     resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		FetchedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if !result.Fetched {
		return result, fmt.Errorf("fetch failed with status %d", resp.StatusCode)
	}
	return result, nil
}

func validatePublicHTTPURL(rawURL string) error {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return fmt.Errorf("unsafe url: invalid URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("unsafe url: only http(s) URLs are allowed")
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("unsafe url: localhost is not allowed")
	}
	if ip, err := netip.ParseAddr(host); err == nil && isUnsafeIP(ip) {
		return fmt.Errorf("unsafe url: private or local IP is not allowed")
	}
	ips, err := net.LookupIP(host)
	if err == nil {
		for _, ip := range ips {
			addr, ok := netip.AddrFromSlice(ip)
			if ok && isUnsafeIP(addr) {
				return fmt.Errorf("unsafe url: hostname resolves to private or local IP")
			}
		}
	}
	return nil
}

func isUnsafeIP(ip netip.Addr) bool {
	if !ip.IsValid() {
		return true
	}
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return true
	}
	if ip.String() == "169.254.169.254" {
		return true
	}
	return false
}
```

- [x] **Step 4: Implement readable extraction helpers**

Create `internal/integrations/publicweb/readable.go`:

```go
package publicweb

import (
	"html"
	"regexp"
	"strings"
	"unicode/utf8"
)

var (
	scriptStyleRE = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>|<style\b[^>]*>.*?</style>|<noscript\b[^>]*>.*?</noscript>`)
	titleRE       = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
	tagRE         = regexp.MustCompile(`(?s)<[^>]+>`)
	spaceRE       = regexp.MustCompile(`\s+`)
)

func readableText(body, contentType string) (text string, title string) {
	if strings.Contains(strings.ToLower(contentType), "html") || strings.Contains(strings.ToLower(body), "<html") {
		if match := titleRE.FindStringSubmatch(body); len(match) == 2 {
			title = compact(html.UnescapeString(match[1]))
		}
		body = scriptStyleRE.ReplaceAllString(body, " ")
		for _, tag := range []string{"</p>", "</div>", "</li>", "</h1>", "</h2>", "</h3>", "<br>", "<br/>", "<br />"} {
			body = strings.ReplaceAll(body, tag, "\n")
		}
		body = tagRE.ReplaceAllString(body, " ")
	}
	text = compact(html.UnescapeString(body))
	if text == "" {
		text = "(empty response)"
	}
	return text, title
}

func compact(value string) string {
	return strings.TrimSpace(spaceRE.ReplaceAllString(value, " "))
}

func truncateBytes(value string, max int) string {
	if max <= 0 || len(value) <= max {
		return value
	}
	if max <= 3 {
		return strings.Repeat(".", max)
	}
	cut := value[:max-3]
	for !utf8.ValidString(cut) && len(cut) > 0 {
		cut = cut[:len(cut)-1]
	}
	return cut + "..."
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
```

- [x] **Step 5: Run safe fetch tests**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestSafeFetch' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 2**

```bash
git add internal/integrations/publicweb/safe_fetch.go internal/integrations/publicweb/readable.go internal/integrations/publicweb/safe_fetch_test.go
git commit -m "feat: add safe public web fetcher"
```

---

### Task 3: Add Lightweight Search Backend And Result Formatting

**Files:**
- Create: `internal/integrations/publicweb/search_backend.go`
- Create: `internal/integrations/publicweb/search_backend_test.go`
- Create: `internal/integrations/publicweb/format.go`

- [x] **Step 1: Write failing backend tests**

Create `internal/integrations/publicweb/search_backend_test.go` with tests for:

```go
func TestLightweightBackendExpandsCandidatesThenFiltersToLimit(t *testing.T)
func TestLightweightBackendAppliesAllowedDomainQueryRewrite(t *testing.T)
func TestLightweightBackendOfficialDomainFallbackReturnsStructuredResults(t *testing.T)
func TestFormatEnvelopePreservesLegacyContentAndStructuredResults(t *testing.T)
```

The assertions must require:

```go
if got := r.URL.Query().Get("q"); !strings.Contains(got, "site:postgresql.org") {
	t.Fatalf("search query = %q, want site filter", got)
}
if len(results) != 5 {
	t.Fatalf("len(results) = %d, want requested limit 5 after larger candidate parse", len(results))
}
if envelope.Results[0].URL == "" || envelope.Content == "" || envelope.Meta.Backend == "" {
	t.Fatalf("envelope = %+v, want structured results plus legacy content", envelope)
}
```

- [x] **Step 2: Run backend tests and verify they fail**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestLightweightBackend|TestFormatEnvelope' -count=1
```

Expected: FAIL because backend/formatter do not exist.

- [x] **Step 3: Implement lightweight backend**

Create `internal/integrations/publicweb/search_backend.go` with:

```go
package publicweb

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	nethtml "golang.org/x/net/html"
)

type LightweightBackend struct {
	client  *http.Client
	baseURL string
}

func NewLightweightBackend(client *http.Client, baseURL string) *LightweightBackend {
	if client == nil {
		client = &http.Client{Timeout: DefaultTimeout}
	}
	if strings.TrimSpace(baseURL) == "" {
		baseURL = "https://www.bing.com"
	}
	return &LightweightBackend{client: client, baseURL: strings.TrimRight(baseURL, "/")}
}

func (b *LightweightBackend) Name() string { return "lightweight_search" }

func (b *LightweightBackend) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	searchURL, err := url.Parse(b.baseURL + "/search")
	if err != nil {
		return nil, err
	}
	query := searchURL.Query()
	query.Set("q", buildPublicSearchQuery(req))
	query.Set("mkt", firstNonEmpty(req.Language, "zh-CN"))
	query.Set("setlang", firstNonEmpty(req.Language, "zh-CN"))
	query.Set("cc", firstNonEmpty(req.Country, "CN"))
	searchURL.RawQuery = query.Encode()

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, searchURL.String(), nil)
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("User-Agent", "aiops-v2-web-search/1.0")
	httpReq.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.7")
	resp, err := b.client.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(DefaultMaxBytes)+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("public web search failed: status %d", resp.StatusCode)
	}
	candidateLimit := req.Limit * 2
	if candidateLimit < 10 {
		candidateLimit = 10
	}
	results := parseBingResults(string(body), candidateLimit)
	results = filterByDomains(results, req.AllowedDomains, req.BlockedDomains)
	results = filterByRelevance(results, req.Query)
	results = dedupeAndLimit(results, req.Limit)
	if len(results) == 0 {
		results = officialDomainFallback(req)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("public web search returned no relevant results")
	}
	return results, nil
}
```

Then move the existing local helper logic from `register.go` into this package:

- Bing HTML parsing.
- Bing encoded URL cleanup.
- domain matching.
- relevance terms and CJK bigrams.
- official PostgreSQL / pgBackRest / pg_auto_failover fallback.

Use exported or package-private names consistently:

```go
func buildPublicSearchQuery(req SearchRequest) string
func parseBingResults(body string, limit int) []SearchResult
func filterByDomains(results []SearchResult, allowed, blocked []string) []SearchResult
func filterByRelevance(results []SearchResult, query string) []SearchResult
func dedupeAndLimit(results []SearchResult, limit int) []SearchResult
func officialDomainFallback(req SearchRequest) []SearchResult
```

- [x] **Step 4: Implement envelope formatter**

Create `internal/integrations/publicweb/format.go`:

```go
package publicweb

import (
	"encoding/json"
	"fmt"
	"strings"
)

func FormatEnvelope(req SearchRequest, source string, results []SearchResult, meta ResultMeta) ResultEnvelope {
	if source == "" {
		source = "custom_public_web:" + req.Operation
	}
	meta.FetchedCount = countFetched(results)
	content := legacyContent(req, results)
	return ResultEnvelope{
		Operation: req.Operation,
		Query:     req.Query,
		URL:       req.URL,
		Source:    source,
		Content:   content,
		Results:   results,
		Meta:      meta,
	}
}

func MarshalEnvelope(env ResultEnvelope, maxBytes int) string {
	data, _ := json.Marshal(env)
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return truncateBytes(string(data), maxBytes)
}

func legacyContent(req SearchRequest, results []SearchResult) string {
	var b strings.Builder
	if req.Operation == OperationOpen {
		fmt.Fprintf(&b, "Opened public web URL %q. Use this page text as evidence and cite the URL:\n", req.URL)
	} else {
		fmt.Fprintf(&b, "Public web search results for %q. Use these results as evidence and cite URLs:\n", req.Query)
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
			fmt.Fprintf(&b, "   Text: %s\n", truncateBytes(result.Text, 1200))
		}
		if result.FetchError != "" {
			fmt.Fprintf(&b, "   Fetch error: %s\n", result.FetchError)
		}
	}
	return b.String()
}

func countFetched(results []SearchResult) int {
	count := 0
	for _, result := range results {
		if result.Fetched {
			count++
		}
	}
	return count
}
```

- [x] **Step 5: Run backend tests**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestLightweightBackend|TestFormatEnvelope' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit Task 3**

```bash
git add internal/integrations/publicweb/search_backend.go internal/integrations/publicweb/search_backend_test.go internal/integrations/publicweb/format.go
git commit -m "feat: add lightweight public web search backend"
```

---

### Task 4: Add Broker For Search/Open And Fetch Content

**Files:**
- Create: `internal/integrations/publicweb/broker.go`
- Create: `internal/integrations/publicweb/broker_test.go`

- [x] **Step 1: Write failing broker tests**

Create `internal/integrations/publicweb/broker_test.go` with these test names:

```go
func TestBrokerSearchReturnsStructuredEnvelope(t *testing.T)
func TestBrokerOpenUsesSafeFetcher(t *testing.T)
func TestBrokerSearchFetchContentMergesFetchedText(t *testing.T)
func TestBrokerSearchFetchContentKeepsSearchResultWhenFetchFails(t *testing.T)
```

The fetch failure test must assert:

```go
if len(env.Results) != 1 || env.Results[0].FetchError == "" || env.Results[0].Fetched {
	t.Fatalf("env = %+v, want search result preserved with fetch error", env)
}
```

- [x] **Step 2: Run broker tests and verify they fail**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestBroker' -count=1
```

Expected: FAIL because `Broker` does not exist.

- [x] **Step 3: Implement broker**

Create `internal/integrations/publicweb/broker.go`:

```go
package publicweb

import "context"

type Broker struct {
	SearchBackend SearchBackend
	Fetcher       Fetcher
	MaxBytes      int
}

func NewBroker(searchBackend SearchBackend, fetcher Fetcher, maxBytes int) *Broker {
	if maxBytes <= 0 {
		maxBytes = DefaultMaxBytes
	}
	return &Broker{SearchBackend: searchBackend, Fetcher: fetcher, MaxBytes: maxBytes}
}

func (b *Broker) Execute(ctx context.Context, req SearchRequest) (ResultEnvelope, error) {
	switch req.Operation {
	case OperationOpen:
		return b.open(ctx, req)
	default:
		return b.search(ctx, req)
	}
}

func (b *Broker) search(ctx context.Context, req SearchRequest) (ResultEnvelope, error) {
	results, err := b.SearchBackend.Search(ctx, req)
	if err != nil {
		return ResultEnvelope{}, err
	}
	if req.FetchContent && b.Fetcher != nil && req.MaxContentResults > 0 {
		limit := req.MaxContentResults
		if limit > len(results) {
			limit = len(results)
		}
		for i := 0; i < limit; i++ {
			fetched, fetchErr := b.Fetcher.Fetch(ctx, FetchRequest{
				URL:            results[i].URL,
				AllowedDomains: req.AllowedDomains,
				BlockedDomains: req.BlockedDomains,
				MaxBytes:       req.MaxBytes,
				Timeout:        req.Timeout,
			})
			if fetchErr != nil {
				results[i].FetchError = fetchErr.Error()
				continue
			}
			results[i] = mergeFetchedResult(results[i], fetched)
		}
	}
	return FormatEnvelope(req, "custom_public_web:search", results, ResultMeta{
		Backend: b.SearchBackend.Name(),
	}), nil
}

func (b *Broker) open(ctx context.Context, req SearchRequest) (ResultEnvelope, error) {
	fetched, err := b.Fetcher.Fetch(ctx, FetchRequest{
		URL:            req.URL,
		AllowedDomains: req.AllowedDomains,
		BlockedDomains: req.BlockedDomains,
		MaxBytes:       req.MaxBytes,
		Timeout:        req.Timeout,
	})
	if err != nil {
		return ResultEnvelope{}, err
	}
	return FormatEnvelope(req, "custom_public_web:open", []SearchResult{fetched}, ResultMeta{
		Backend:  "internal_fetch",
		FinalURL: fetched.URL,
	}), nil
}

func mergeFetchedResult(search SearchResult, fetched SearchResult) SearchResult {
	if fetched.Title != "" {
		search.Title = fetched.Title
	}
	if fetched.URL != "" {
		search.URL = fetched.URL
	}
	search.Text = fetched.Text
	search.Markdown = fetched.Markdown
	search.Fetched = fetched.Fetched
	search.StatusCode = fetched.StatusCode
	search.ContentType = fetched.ContentType
	search.FetchedAt = fetched.FetchedAt
	if fetched.Snippet != "" {
		search.Snippet = fetched.Snippet
	}
	return search
}
```

- [x] **Step 4: Run broker tests**

Run:

```bash
go test ./internal/integrations/publicweb -run 'TestBroker' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 4**

```bash
git add internal/integrations/publicweb/broker.go internal/integrations/publicweb/broker_test.go
git commit -m "feat: add public web broker"
```

---

### Task 5: Integrate Single web_search Tool And browse_url Alias

**Files:**
- Modify: `internal/integrations/localtools/register.go`
- Modify: `internal/integrations/localtools/register_test.go`

- [x] **Step 1: Write failing localtools integration tests**

In `internal/integrations/localtools/register_test.go`, add or update tests:

```go
func TestWebSearchToolAcceptsOpenOperationAndReturnsBrowserSearchPayload(t *testing.T)
func TestBrowseURLToolAliasesWebSearchOpenImplementation(t *testing.T)
func TestWebSearchToolRejectsUnsafeOpenURL(t *testing.T)
func TestWebSearchToolSearchResultIncludesStructuredResultsAndMeta(t *testing.T)
func TestWebSearchProviderNativePathIgnoresCustomOpenFields(t *testing.T)
```

Required assertions:

```go
if !strings.Contains(result.Content, `"operation":"open"`) || !strings.Contains(result.Content, `"results"`) {
	t.Fatalf("result content = %q, want structured open envelope", result.Content)
}
if !strings.Contains(result.Content, `"source":"custom_public_web:search"`) {
	t.Fatalf("result content = %q, want custom public web search source", result.Content)
}
if strings.Contains(webSearch.Metadata().Description, "Firecrawl") {
	t.Fatalf("web_search description must not expose Firecrawl: %s", webSearch.Metadata().Description)
}
```

- [x] **Step 2: Run localtools tests and verify they fail**

Run:

```bash
go test ./internal/integrations/localtools -run 'TestWebSearchToolAcceptsOpen|TestBrowseURLToolAliases|TestWebSearchToolRejectsUnsafe|TestWebSearchToolSearchResultIncludes|TestWebSearchProviderNativePath' -count=1
```

Expected: FAIL until `NewWebSearchTool` and `NewBrowseURLTool` are rewired.

- [x] **Step 3: Modify imports and tool schema**

In `internal/integrations/localtools/register.go`, import:

```go
"aiops-v2/internal/integrations/publicweb"
```

Update `NewWebSearchTool` input schema properties to include:

```json
"operation": {"type": "string", "enum": ["search", "open"], "description": "Use search for query search or open for reading a specific public http(s) URL."},
"url": {"type": "string", "description": "Public http(s) URL to read when operation=open."},
"limit": {"type": "integer", "description": "Maximum search results returned after filtering."},
"fetch_content": {"type": "boolean", "description": "When true, fetch bounded readable text for the first matching search results."},
"max_content_results": {"type": "integer", "description": "Maximum number of search results to fetch for content."},
"max_bytes": {"type": "integer", "description": "Maximum inline bytes for opened or fetched page text."}
```

Keep `query`, `search_context_size`, `allowed_domains`, and `blocked_domains`.

- [x] **Step 4: Rewire ExecuteFunc for custom path**

Replace old custom fallback execution with:

```go
req, err := publicweb.ParseRequest(input)
if err != nil {
	return tooling.ToolResult{}, err
}
cfg := currentConfig(repo)
client := opts.HTTPClient
if client == nil {
	client = &http.Client{Timeout: opts.WebTimeout}
}
if req.Operation == publicweb.OperationSearch && providerSupportsNativeWebSearch(cfg.Provider, cfg.Model) {
	legacyReq := webSearchInput{
		Query:             req.Query,
		SearchContextSize: req.SearchContextSize,
		AllowedDomains:    req.AllowedDomains,
		BlockedDomains:    req.BlockedDomains,
	}
	content, source, err := runProviderNativeWebSearch(ctx, client, cfg, legacyReq, opts)
	if err == nil {
		return webSearchToolResultFromEnvelope(req, content, source, nil, opts), nil
	}
}
broker := publicweb.NewBroker(
	publicweb.NewLightweightBackend(client, opts.PublicSearchBaseURL),
	publicweb.NewSafeFetcher(client),
	opts.MaxOutputBytes,
)
env, err := broker.Execute(ctx, req)
if err != nil {
	return tooling.ToolResult{}, err
}
return publicWebToolResult(env, opts), nil
```

Implement helpers:

```go
func publicWebToolResult(env publicweb.ResultEnvelope, opts Options) tooling.ToolResult {
	return tooling.ToolResult{
		Content: publicweb.MarshalEnvelope(env, opts.MaxOutputBytes),
		Display: &tooling.ToolDisplayPayload{
			Type:  "web_search",
			Title: firstNonEmptyString(env.Query, env.URL),
		},
	}
}
```

- [x] **Step 5: Make browse_url a thin alias**

In `NewBrowseURLTool.ExecuteFunc`, replace `fetchReadableURLText` usage with:

```go
raw := map[string]any{
	"operation": "open",
	"url":       req.URL,
	"max_bytes": boundedMaxBytes(req.MaxBytes, opts.MaxOutputBytes),
}
data, _ := json.Marshal(raw)
webTool := NewWebSearchTool(nil, opts)
return webTool.Execute(ctx, data)
```

Then remove old independent helpers only after all tests pass:

- `fetchReadableURLText`
- `htmlToReadableText`
- old local Bing parser helpers that moved to `publicweb`

If `NewWebSearchTool(nil, opts)` requires provider config for search, ensure `operation=open` never reads provider config before broker execution.

- [x] **Step 6: Run focused integration tests**

Run:

```bash
go test ./internal/integrations/localtools -run 'TestWebSearchTool|TestBrowseURLTool|TestBaseRegistry' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Task 5**

```bash
git add internal/integrations/localtools/register.go internal/integrations/localtools/register_test.go
git commit -m "feat: merge browse url into web search tool"
```

---

### Task 6: Preserve Provider-Native web_search Behavior

**Files:**
- Modify: `internal/integrations/localtools/register_test.go`
- Modify: `internal/runtimekernel/provider_native_web_search_test.go`
- Modify only if needed: `internal/runtimekernel/provider_native_web_search.go`

- [x] **Step 1: Add provider-native regression tests**

Add tests that assert:

```go
func TestProviderNativeWebSearchRequestShapeUnchangedAfterCustomToolMerge(t *testing.T)
func TestProviderNativeWebSearchStillProjectsBrowserSearchItems(t *testing.T)
```

Required checks:

```go
if gotToolType != "web_search" {
	t.Fatalf("provider tool type = %q, want web_search", gotToolType)
}
if gotInclude[0] != "web_search_call.action.sources" {
	t.Fatalf("include = %#v, want web_search sources include", gotInclude)
}
if snapshot.AgentItems[0].Payload.Kind != "browser.search" {
	t.Fatalf("payload kind = %q, want browser.search", snapshot.AgentItems[0].Payload.Kind)
}
```

- [x] **Step 2: Run provider-native tests**

Run:

```bash
go test ./internal/integrations/localtools ./internal/runtimekernel -run 'TestProviderNativeWebSearch|TestWebSearchToolUsesProviderNative' -count=1
```

Expected: PASS after Task 5 integration. If it fails, fix only the custom wrapper, not provider-native request construction.

- [ ] **Step 3: Commit Task 6**

```bash
git add internal/integrations/localtools/register_test.go internal/runtimekernel/provider_native_web_search_test.go internal/runtimekernel/provider_native_web_search.go
git commit -m "test: lock provider native web search behavior"
```

---

### Task 7: Update Runtime Tool Surface And Prompt Text

**Files:**
- Modify: `internal/integrations/localtools/register.go`
- Modify: `internal/integrations/localtools/register_test.go`
- Modify: `internal/runtimekernel/runtime_kernel.go`
- Modify: `internal/runtimekernel/react_loop_test.go`
- Modify: `internal/runtimekernel/model_input_trace_test.go`
- Modify: `internal/tooling/turn_metadata_filter_test.go` if expectations mention two public web tools

- [x] **Step 1: Write/adjust tests for model-visible tool surface**

Update `TestBaseRegistry...` expectations:

```go
if _, ok := names["browse_url"]; ok {
	t.Fatalf("browse_url should not be in default model-visible chat tools; got %v", toolNames(tools))
}
if browseDiscovery := browseURL.Metadata().EffectiveDiscovery(); !browseDiscovery.HiddenFromPrompt {
	t.Fatalf("browse_url discovery = %+v, want hidden from prompt compatibility alias", browseDiscovery)
}
```

If keeping `browse_url` discoverable for historical tool_search is still required, use this stricter alternative:

```go
if !browseDiscovery.RequiresSelect || browseDiscovery.LoadingPolicy != tooling.ToolLoadingPolicyDeferred {
	t.Fatalf("browse_url must remain select-only alias, got %+v", browseDiscovery)
}
if !strings.Contains(browseURL.Metadata().Description, "compatibility alias") {
	t.Fatalf("browse_url description should explain alias behavior: %s", browseURL.Metadata().Description)
}
```

Pick one behavior and keep it consistent across tool_search/runtime tests. Recommended: hidden from prompt and discovery unless historical replay requires registry presence.

- [x] **Step 2: Update user-facing runtime guidance**

In `internal/runtimekernel/runtime_kernel.go`, replace:

```go
"停止继续调用 web_search 或 browse_url"
"When using web_search or browse_url evidence"
"use web_search/browse_url"
```

with:

```go
"停止继续调用 web_search"
"When using web_search evidence, including operation=open evidence"
"use web_search; use operation=open when a specific public URL must be read"
```

- [x] **Step 3: Update trace tests**

In `internal/runtimekernel/model_input_trace_test.go`, update visible tool expectations so default public web surface contains `web_search` only. If a static trace fixture still includes `browse_url`, mark it as hidden alias or remove it from visible tools.

- [x] **Step 4: Run runtime/tooling focused tests**

Run:

```bash
go test ./internal/integrations/localtools ./internal/runtimekernel ./internal/tooling -run 'TestBaseRegistry|TestPublicWeb|TestModelInputTrace|TestTurnMetadataFilter|TestToolSurface' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit Task 7**

```bash
git add internal/integrations/localtools/register.go internal/integrations/localtools/register_test.go internal/runtimekernel/runtime_kernel.go internal/runtimekernel/react_loop_test.go internal/runtimekernel/model_input_trace_test.go internal/tooling/turn_metadata_filter_test.go
git commit -m "feat: expose single public web tool surface"
```

---

### Task 8: Update Transport Projection And ProcessTranscript UI

**Files:**
- Modify: `internal/appui/transport_projector.go`
- Modify: `internal/appui/transport_projector_test.go`
- Modify: `web/src/transport/aiopsTransportTypes.ts`
- Modify: `web/src/chat/components/ProcessTranscript.tsx`
- Modify: `web/src/chat/components/ProcessTranscript.test.tsx`

- [x] **Step 1: Add projector tests for structured results**

In `internal/appui/transport_projector_test.go`, add:

```go
func TestDecodeTransportSearchResultsUsesStructuredResultsWithFetchedStatus(t *testing.T) {
	raw := json.RawMessage(`{
		"operation":"search",
		"query":"postgres docs",
		"results":[
			{"title":"PostgreSQL docs","url":"https://www.postgresql.org/docs/current/","snippet":"docs","text":"bounded text","fetched":true},
			{"title":"Blog","url":"https://example.com/post","snippet":"ignored extra domain","fetched":false}
		],
		"meta":{"backend":"lightweight_search+internal_fetch","fetchedCount":1}
	}`)
	results := decodeTransportSearchResults(raw)
	if len(results) != 2 || results[0].Title != "PostgreSQL docs" {
		t.Fatalf("results = %#v", results)
	}
}

func TestTransportProjectorSummarizesWebSearchOpenByURL(t *testing.T) {
	// Build a tool result with toolName=web_search and content operation=open.
	// Assert process block Text or InputSummary contains postgresql.org/docs.
}
```

- [x] **Step 2: Run projector tests and verify failures**

Run:

```bash
go test ./internal/appui -run 'TestDecodeTransportSearchResults|TestTransportProjector.*WebSearch' -count=1
```

Expected: FAIL where `operation=open` summary/fetched fields are not handled.

- [x] **Step 3: Update projector logic**

In `decodeTransportSearchResults`, expand payload struct:

```go
var payload struct {
	Operation string              `json:"operation"`
	Query     string              `json:"query"`
	URL       string              `json:"url"`
	Results   []AiopsSearchResult `json:"results"`
	Content   string              `json:"content"`
	Meta      map[string]any      `json:"meta"`
}
```

Keep existing parser fallback. For `operation=open`, summary should prefer `payload.URL`.

- [x] **Step 4: Add React tests**

In `web/src/chat/components/ProcessTranscript.test.tsx`, add tests:

```tsx
it("renders web_search open operations inside the web lookup group", () => {
  const process = [
    makeBlock({
      id: "open-docs",
      kind: "search",
      displayKind: "web_search",
      text: "https://www.postgresql.org/docs/current/",
      outputPreview: "Opened public web URL",
      searchResults: [{ title: "PostgreSQL docs", url: "https://www.postgresql.org/docs/current/", snippet: "Readable text" }],
      foldGroupKind: "web_lookup",
    }),
  ];
  const { container } = render(<ProcessTranscript process={process} turnStatus="completed" />);
  expect(container.textContent).toContain("网页检索 1 次");
  expect(container.textContent).toContain("PostgreSQL docs");
});
```

Add a second test that includes one fetched result and expects the detail text to contain `已读取正文` if UI adds that label.

- [x] **Step 5: Update UI rendering**

In `ProcessTranscript.tsx`:

- Keep existing `web_lookup` grouping.
- Treat `displayKind=web_search` with URL text as a search/open item.
- Add a small result-state label only in expanded details:

```tsx
{source.fetched ? <span className="text-sky-700">已读取正文</span> : null}
{source.fetchError ? <span className="text-amber-700">读取失败</span> : null}
```

Do not show full markdown in expanded process UI.

- [x] **Step 6: Run appui and frontend tests**

Run:

```bash
go test ./internal/appui -run 'TestDecodeTransportSearchResults|TestTransportProjector.*WebSearch' -count=1
cd web && npm test -- --run src/chat/components/ProcessTranscript.test.tsx
```

Expected: PASS.

- [ ] **Step 7: Commit Task 8**

```bash
git add internal/appui/transport_projector.go internal/appui/transport_projector_test.go web/src/transport/aiopsTransportTypes.ts web/src/chat/components/ProcessTranscript.tsx web/src/chat/components/ProcessTranscript.test.tsx
git commit -m "feat: render structured public web results"
```

---

### Task 9: Add Browser-In-App And Playwright Verification

**Files:**
- Modify: `web/tests/react-shell-snapshot.spec.js`
- Modify: `web/src/lib/uiFixturePresets.js` if fixture presets are used

- [x] **Step 1: Add fixture for search -> open -> final flow**

Create or update a fixture with:

```js
{
  id: "web-search-single-tool-open",
  process: [
    {
      id: "commentary-1",
      kind: "reasoning",
      text: "我会先检索官方文档，再读取最相关页面正文。",
      status: "completed"
    },
    {
      id: "search-1",
      kind: "search",
      displayKind: "web_search",
      text: "PostgreSQL recovery_target_timeline official docs",
      outputPreview: "网页检索完成",
      searchResults: [
        {
          title: "PostgreSQL recovery_target_timeline",
          url: "https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-RECOVERY-TARGET-TIMELINE",
          snippet: "Official setting reference.",
          fetched: true
        }
      ],
      foldGroupKind: "web_lookup"
    },
    {
      id: "commentary-2",
      kind: "reasoning",
      text: "已读取官方文档正文，下面基于该来源回答。",
      status: "completed"
    }
  ],
  finalText: "结论：recovery_target_timeline 可设为 latest，并应结合恢复状态判断。"
}
```

- [x] **Step 2: Add Playwright assertion**

In `web/tests/react-shell-snapshot.spec.js`:

```js
test("web search single tool renders stable lookup group", async ({ page }) => {
  await openFixture(page, "web-search-single-tool-open");
  const transcript = page.locator('[data-testid="process-transcript"]').first();
  await expect(transcript).toContainText("网页检索 1 次");
  await expect(transcript).toContainText("找到 1 个来源");
  await expect(transcript).toContainText("PostgreSQL recovery_target_timeline");
  await expect(transcript).not.toContainText("browse_url");
  await expect(page.getByText("结论：recovery_target_timeline")).toBeVisible();
});
```

- [x] **Step 3: Run Playwright**

Run:

```bash
cd web && npx playwright test tests/react-shell-snapshot.spec.js -g "web search single tool"
```

Expected: PASS and no screenshot diff unless snapshot fixture intentionally changes.

- [x] **Step 4: Manually verify in browser-in-app**

Start the dev server if none is running:

```bash
cd web && npm run dev -- --host 127.0.0.1
```

Use browser-in-app to open the local URL and verify:

- `说明 -> 网页检索 -> 说明 -> 最终回答` order is stable.
- The folded row says `网页检索 N 次 · 找到 M 个来源`.
- No row visibly flips between one-line `browse_url` and folded search display.
- Expanded detail shows source title/domain/snippet and bounded “已读取正文” state.

- [ ] **Step 5: Commit Task 9**

```bash
git add web/tests/react-shell-snapshot.spec.js web/src/lib/uiFixturePresets.js
git commit -m "test: verify single web search transcript"
```

---

### Task 10: Full Verification And Cleanup

**Files:**
- Modify: this plan file as statuses change
- Modify: `.gitignore` only if plan/spec whitelist is missing

- [x] **Step 1: Run Go verification**

Run:

```bash
go test ./internal/integrations/publicweb ./internal/integrations/localtools ./internal/runtimekernel ./internal/appui ./internal/tooling
```

Expected: PASS.

- [x] **Step 2: Run frontend verification**

Run:

```bash
cd web && npm test -- --run src/chat/components/ProcessTranscript.test.tsx
cd web && npx playwright test tests/react-shell-snapshot.spec.js -g "web search single tool"
```

Expected: PASS.

Verification run:

```bash
cd web && npm test -- --run src/chat/components/ProcessTranscript.test.tsx
cd web && npm run typecheck
cd web && npx playwright test tests/e2e/web-search-open-transcript.spec.js --project=chromium
```

Result: PASS. The older planned `tests/react-shell-snapshot.spec.js -g "web search single tool"` selector no longer matches a test name, so the dedicated `web-search-open-transcript` Playwright spec is the active browser regression for this feature.

Browser-in-app verification:

- Opened `http://127.0.0.1:53173/?fixture=web-search-open-transcript`.
- Expanded the process transcript and both `aiops-search-toggle` blocks.
- Verified two stable search toggles, no visible `browse_url`, no full text leak, and `已读取正文` only inside expanded open details.

Reverification on `2026-06-29 02:18:47 CST`:

- `go test ./internal/integrations/publicweb ./internal/integrations/localtools ./internal/runtimekernel ./internal/appui ./internal/tooling`: PASS.
- `cd web && npm test -- --run src/chat/components/ProcessTranscript.test.tsx`: PASS, 85 tests.
- `cd web && npm run typecheck`: PASS.
- `cd web && npx playwright test tests/e2e/web-search-open-transcript.spec.js --project=chromium`: PASS, 1 test.
- browser-in-app opened `http://127.0.0.1:53173/?fixture=web-search-open-transcript`, expanded the process transcript, both web lookup toggles, and the open detail row: two stable `网页检索 1 次` toggles, no visible `browse_url`, no full text leak, and `已读取正文` is shown only in expanded detail.

- [x] **Step 3: Run guard checks**

Run:

```bash
rg -n "FirecrawlSearchBackend|AIOPS_FIRECRAWL|firecrawl_search|Firecrawl API key|FIRECRAWL_" internal web docs/superpowers/plans/2026-06-29-aiops-v2-web-search-firecrawl-optimization-implementation-todo.zh.md
```

Expected: no matches except allowed prose in design/plan explaining that Firecrawl is not integrated.

Run:

```bash
rg -n "browse_url" internal/runtimekernel internal/promptcompiler web/src web/tests
```

Expected: only compatibility alias tests, historical fixture assertions, or text explicitly saying to use `web_search(operation=open)`.

Reverification on `2026-06-29 02:18:47 CST`:

- Firecrawl guard only matched this plan document's prohibited examples/check command.
- Removed-helper guard found no matches for old localtools public-search/open helpers.
- `browse_url` matches are limited to compatibility alias code/tests, historical fixture/projection support, and prompts/tests that explicitly prohibit or guard the old visible path.

- [x] **Step 4: Inspect git diff**

Run:

```bash
git diff --stat
git diff -- internal/integrations/publicweb internal/integrations/localtools internal/runtimekernel internal/appui web/src web/tests
```

Expected:

- New `publicweb` package owns search/open/fetch logic.
- `register.go` no longer has separate browse fetch or public search parsing logic.
- Provider-native request construction remains unchanged.
- UI still uses one `web_lookup` fold group.

- [x] **Step 5: Update task statuses**

After each task is implemented and verified, update this document from `- [ ]` to `- [x]` for completed steps. Do not mark a task complete until its verification command has passed.

- [ ] **Step 6: Final commit**

```bash
git add .
git commit -m "feat: optimize custom public web search"
```

If the worktree contains unrelated user changes, stage only files touched for this feature.

## Final Acceptance Checklist

- [x] `web_search(operation=search)` returns structured `results/meta` and legacy `content`.
- [x] `web_search(operation=open)` reads a public URL through safe fetch and returns the same envelope shape.
- [x] `browse_url` is not a separate model-visible implementation path.
- [x] Provider-native `web_search` behavior and UI projection are unchanged.
- [x] Non-native providers such as Zhipu/GLM continue to use custom public web fallback.
- [x] Search candidates are expanded before filtering and trimmed after dedupe/domain limits.
- [x] `fetch_content=true` merges bounded readable text into first N results.
- [x] Fetch failures are represented per result and do not fail the whole search.
- [x] ProcessTranscript shows one stable web lookup group without Firecrawl or browse_url concepts.
- [x] No Firecrawl runtime dependency, config, API client, env var, or tool is introduced.
