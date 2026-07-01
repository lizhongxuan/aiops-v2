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
	now    func() time.Time
}

func NewSafeFetcher(client *http.Client) *SafeFetcher {
	return newSafeFetcher(client, time.Now)
}

func newSafeFetcher(client *http.Client, now func() time.Time) *SafeFetcher {
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
	if next.Timeout <= 0 {
		next.Timeout = DefaultTimeout
	}
	if now == nil {
		now = time.Now
	}
	return &SafeFetcher{client: &next, now: now}
}

func (f *SafeFetcher) Fetch(ctx context.Context, req FetchRequest) (SearchResult, error) {
	if err := validatePublicHTTPURL(req.URL); err != nil {
		return SearchResult{}, err
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 || maxBytes > DefaultMaxBytes {
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
	if resp.Request != nil && resp.Request.URL != nil {
		if err := validatePublicHTTPURL(resp.Request.URL.String()); err != nil {
			return SearchResult{}, err
		}
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, int64(maxBytes)+1))
	text, title := readableText(string(body), resp.Header.Get("Content-Type"))
	finalURL := req.URL
	if resp.Request != nil && resp.Request.URL != nil {
		finalURL = resp.Request.URL.String()
	}
	result := SearchResult{
		Title:       firstNonEmpty(title, responseHostname(finalURL)),
		URL:         finalURL,
		Text:        truncateBytes(text, maxBytes),
		Snippet:     truncateBytes(text, 600),
		Source:      "custom_public_web",
		Provider:    "internal_fetch",
		ContentKind: "text",
		Fetched:     resp.StatusCode >= 200 && resp.StatusCode < 300,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
		FetchedAt:   f.now().UTC().Format(time.RFC3339),
	}
	if !result.Fetched {
		return result, fmt.Errorf("fetch failed with status %d", resp.StatusCode)
	}
	return result, nil
}

func validatePublicHTTPURL(raw string) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed == nil || parsed.Hostname() == "" {
		return fmt.Errorf("unsafe url %q: invalid URL", raw)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "http", "https":
	default:
		return fmt.Errorf("unsafe url %q: only http(s) URLs are supported", raw)
	}
	host := strings.ToLower(strings.Trim(parsed.Hostname(), "."))
	if host == "localhost" || strings.HasSuffix(host, ".localhost") {
		return fmt.Errorf("unsafe url %q: private or local host is not allowed", raw)
	}
	if ip, err := netip.ParseAddr(host); err == nil {
		if !isPublicAddr(ip) {
			return fmt.Errorf("unsafe url %q: private or local address is not allowed", raw)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	for _, ip := range ips {
		addr, ok := netip.AddrFromSlice(ip)
		if ok && !isPublicAddr(addr) {
			return fmt.Errorf("unsafe url %q: hostname resolves to private or local address", raw)
		}
	}
	return nil
}

func isPublicAddr(addr netip.Addr) bool {
	return addr.IsValid() &&
		!addr.IsLoopback() &&
		!addr.IsPrivate() &&
		!addr.IsLinkLocalUnicast() &&
		!addr.IsLinkLocalMulticast() &&
		!addr.IsMulticast() &&
		!addr.IsUnspecified()
}

func responseHostname(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil || parsed == nil {
		return ""
	}
	return parsed.Hostname()
}
