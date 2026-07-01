package publicweb

import (
	"context"
	"encoding/base64"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"strings"
	"unicode"

	nethtml "golang.org/x/net/html"
)

type LightweightBackend struct {
	client  *http.Client
	baseURL string
}

const searchResultPageReadLimit = 1 << 20

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
	body, _ := io.ReadAll(io.LimitReader(resp.Body, searchResultPageReadLimit+1))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("public web search failed: status %d", resp.StatusCode)
	}
	candidateLimit := req.Limit * 3
	if candidateLimit < 12 {
		candidateLimit = 12
	}
	results := parseBingResults(string(body), candidateLimit)
	results = filterByDomains(results, req.AllowedDomains, req.BlockedDomains)
	results = filterByRelevance(results, req.Query)
	results = dedupeAndLimit(results, req.Limit)
	if len(results) == 0 {
		results = dedupeAndLimit(officialDomainFallback(req), req.Limit)
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("public web search returned no relevant results")
	}
	return results, nil
}

func buildPublicSearchQuery(req SearchRequest) string {
	query := compactWhitespace(req.Query)
	for _, domain := range req.AllowedDomains {
		if !strings.Contains(query, "site:"+domain) {
			query += " site:" + domain
		}
	}
	for _, domain := range req.BlockedDomains {
		if !strings.Contains(query, "-site:"+domain) {
			query += " -site:" + domain
		}
	}
	return compactWhitespace(query)
}

func parseBingResults(body string, limit int) []SearchResult {
	if limit <= 0 {
		limit = DefaultLimit
	}
	doc, err := nethtml.Parse(strings.NewReader(body))
	if err != nil {
		return nil
	}
	results := make([]SearchResult, 0, limit)
	var walk func(*nethtml.Node)
	walk = func(node *nethtml.Node) {
		if node == nil || len(results) >= limit {
			return
		}
		if isHTMLElement(node, "li") && htmlNodeHasClass(node, "b_algo") {
			if result, ok := parseBingResultNode(node); ok {
				results = append(results, result)
			}
		}
		for child := node.FirstChild; child != nil && len(results) < limit; child = child.NextSibling {
			walk(child)
		}
	}
	walk(doc)
	return results
}

func parseBingResultNode(node *nethtml.Node) (SearchResult, bool) {
	anchor := firstSearchResultAnchor(node)
	if anchor == nil {
		return SearchResult{}, false
	}
	result := SearchResult{
		Title:    compactWhitespace(html.UnescapeString(htmlNodeText(anchor))),
		URL:      cleanSearchResultURL(html.UnescapeString(htmlNodeAttr(anchor, "href"))),
		Source:   "custom_public_web",
		Provider: "lightweight_search",
	}
	if caption := firstDescendant(node, func(candidate *nethtml.Node) bool {
		return isHTMLElement(candidate, "div") && htmlNodeHasClass(candidate, "b_caption")
	}); caption != nil {
		textNode := firstDescendant(caption, func(candidate *nethtml.Node) bool {
			return isHTMLElement(candidate, "p")
		})
		if textNode == nil {
			textNode = caption
		}
		result.Snippet = compactWhitespace(html.UnescapeString(htmlNodeText(textNode)))
	}
	return result, result.URL != "" && (result.Title != "" || result.Snippet != "")
}

func firstSearchResultAnchor(node *nethtml.Node) *nethtml.Node {
	if heading := firstDescendant(node, func(candidate *nethtml.Node) bool {
		return isHTMLElement(candidate, "h2")
	}); heading != nil {
		if anchor := firstDescendant(heading, func(candidate *nethtml.Node) bool {
			return isHTMLElement(candidate, "a") && htmlNodeAttr(candidate, "href") != ""
		}); anchor != nil {
			return anchor
		}
	}
	return firstDescendant(node, func(candidate *nethtml.Node) bool {
		return isHTMLElement(candidate, "a") && htmlNodeAttr(candidate, "href") != ""
	})
}

func firstDescendant(node *nethtml.Node, match func(*nethtml.Node) bool) *nethtml.Node {
	if node == nil || match == nil {
		return nil
	}
	for child := node.FirstChild; child != nil; child = child.NextSibling {
		if match(child) {
			return child
		}
		if found := firstDescendant(child, match); found != nil {
			return found
		}
	}
	return nil
}

func isHTMLElement(node *nethtml.Node, tag string) bool {
	return node != nil && node.Type == nethtml.ElementNode && strings.EqualFold(node.Data, tag)
}

func htmlNodeAttr(node *nethtml.Node, name string) string {
	if node == nil {
		return ""
	}
	for _, attr := range node.Attr {
		if strings.EqualFold(attr.Key, name) {
			return attr.Val
		}
	}
	return ""
}

func htmlNodeHasClass(node *nethtml.Node, class string) bool {
	for _, part := range strings.Fields(htmlNodeAttr(node, "class")) {
		if part == class {
			return true
		}
	}
	return false
}

func htmlNodeText(node *nethtml.Node) string {
	var b strings.Builder
	var walk func(*nethtml.Node)
	walk = func(current *nethtml.Node) {
		if current == nil {
			return
		}
		if current.Type == nethtml.TextNode {
			b.WriteString(current.Data)
			b.WriteByte(' ')
			return
		}
		for child := current.FirstChild; child != nil; child = child.NextSibling {
			walk(child)
		}
	}
	walk(node)
	return compactWhitespace(b.String())
}

func cleanSearchResultURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	if encoded := parsed.Query().Get("u"); strings.HasPrefix(encoded, "a1") {
		if decoded := decodeBingBase64URL(strings.TrimPrefix(encoded, "a1")); decoded != "" {
			return decoded
		}
	}
	return raw
}

func decodeBingBase64URL(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if pad := len(value) % 4; pad != 0 {
		value += strings.Repeat("=", 4-pad)
	}
	decoded, err := base64.URLEncoding.DecodeString(value)
	if err != nil {
		decoded, err = base64.StdEncoding.DecodeString(value)
	}
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(decoded))
}

func filterByDomains(results []SearchResult, allowed, blocked []string) []SearchResult {
	if len(results) == 0 || (len(allowed) == 0 && len(blocked) == 0) {
		return results
	}
	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		host := resultHost(result.URL)
		if host == "" {
			continue
		}
		if len(allowed) > 0 && !hostMatchesAnyDomain(host, allowed) {
			continue
		}
		if len(blocked) > 0 && hostMatchesAnyDomain(host, blocked) {
			continue
		}
		filtered = append(filtered, result)
	}
	return filtered
}

func filterByRelevance(results []SearchResult, query string) []SearchResult {
	terms := relevanceTerms(query)
	if len(results) == 0 || len(terms) == 0 {
		return results
	}
	filtered := make([]SearchResult, 0, len(results))
	for _, result := range results {
		haystack := strings.ToLower(result.Title + " " + result.Snippet + " " + result.URL)
		score := 0
		for _, term := range terms {
			if strings.Contains(haystack, term) {
				score++
			}
		}
		if score > 0 {
			filtered = append(filtered, result)
		}
	}
	return filtered
}

func dedupeAndLimit(results []SearchResult, limit int) []SearchResult {
	if limit <= 0 {
		limit = DefaultLimit
	}
	seen := map[string]bool{}
	out := make([]SearchResult, 0, limit)
	for _, result := range results {
		key := strings.ToLower(firstNonEmpty(result.URL, result.Title))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, result)
		if len(out) >= limit {
			break
		}
	}
	return out
}

func officialDomainFallback(req SearchRequest) []SearchResult {
	query := strings.ToLower(compactWhitespace(req.Query))
	results := make([]SearchResult, 0, 6)
	if mentionsAny(query, "postgresql", "postgres", "recovery_target_timeline", "wal archive", "timeline history") {
		results = append(results,
			SearchResult{
				Title:    "PostgreSQL official docs: continuous archiving and point-in-time recovery",
				URL:      "https://www.postgresql.org/docs/current/continuous-archiving.html",
				Snippet:  "Official PostgreSQL recovery guidance, including timeline behavior during archive recovery.",
				Source:   "official_domain_fallback",
				Provider: "curated_fallback",
			},
			SearchResult{
				Title:    "PostgreSQL official docs: recovery_target_timeline setting",
				URL:      "https://www.postgresql.org/docs/current/runtime-config-wal.html#GUC-RECOVERY-TARGET-TIMELINE",
				Snippet:  "Official setting reference for selecting latest, current, or a specific recovery target timeline. Verify promotion state before recommending cleanup of temporary recovery settings.",
				Source:   "official_domain_fallback",
				Provider: "curated_fallback",
			},
		)
	}
	if mentionsAny(query, "pgbackrest", "pg backrest") {
		results = append(results, SearchResult{
			Title:    "pgBackRest official user guide: restore",
			URL:      "https://pgbackrest.org/user-guide.html#restore",
			Snippet:  "Official pgBackRest restore guide. Use the page text before giving version-sensitive restore or standby guidance.",
			Source:   "official_domain_fallback",
			Provider: "curated_fallback",
		})
	}
	if mentionsAny(query, "pg_auto_failover", "pg auto failover") {
		results = append(results, SearchResult{
			Title:    "pg_auto_failover official operations documentation",
			URL:      "https://pg-auto-failover.readthedocs.io/en/main/operations.html",
			Snippet:  "Official operational guidance for pg_auto_failover nodes, state transitions, and recovery operations.",
			Source:   "official_domain_fallback",
			Provider: "curated_fallback",
		})
	}
	return results
}

func resultHost(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return ""
	}
	return strings.ToLower(strings.Trim(parsed.Hostname(), "."))
}

func hostMatchesAnyDomain(host string, domains []string) bool {
	host = strings.ToLower(strings.Trim(host, "."))
	for _, domain := range domains {
		domain = strings.ToLower(strings.Trim(domain, "."))
		if host == domain || strings.HasSuffix(host, "."+domain) {
			return true
		}
	}
	return false
}

func relevanceTerms(query string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, raw := range strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune(`"'“”‘’(),，。:：;；!?！？[]{}<>|/\\`, r)
	}) {
		term := strings.Trim(raw, ".-_")
		if len([]rune(term)) < 2 || isDateOnlyTerm(term) || strings.HasPrefix(term, "site:") || strings.HasPrefix(term, "-site:") {
			continue
		}
		if !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	for _, term := range cjkBigrams(query) {
		if !seen[term] {
			seen[term] = true
			out = append(out, term)
		}
	}
	return out
}

func cjkBigrams(value string) []string {
	runes := []rune(strings.ToLower(value))
	out := []string{}
	for i := 0; i+1 < len(runes); i++ {
		if isCJK(runes[i]) && isCJK(runes[i+1]) {
			out = append(out, string(runes[i:i+2]))
		}
	}
	return out
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
}

func isDateOnlyTerm(term string) bool {
	if term == "" {
		return false
	}
	hasDigit := false
	for _, r := range term {
		if unicode.IsDigit(r) {
			hasDigit = true
			continue
		}
		if !strings.ContainsRune("-_/年月日.", r) {
			return false
		}
	}
	return hasDigit
}

func mentionsAny(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}
