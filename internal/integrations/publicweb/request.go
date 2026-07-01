package publicweb

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

type rawRequest struct {
	Operation         string   `json:"operation"`
	Query             string   `json:"query"`
	URL               string   `json:"url"`
	SearchContextSize string   `json:"search_context_size"`
	AllowedDomains    []string `json:"allowed_domains"`
	BlockedDomains    []string `json:"blocked_domains"`
	Limit             int      `json:"limit"`
	MaxResults        int      `json:"max_results"`
	TimeRange         string   `json:"time_range"`
	Language          string   `json:"language"`
	Country           string   `json:"country"`
	Location          string   `json:"location"`
	FetchContent      bool     `json:"fetch_content"`
	MaxContentResults int      `json:"max_content_results"`
	ContentFormats    []string `json:"content_formats"`
	MaxBytes          int      `json:"max_bytes"`
	MaxBytesCamel     int      `json:"maxBytes"`
	TimeoutMs         int      `json:"timeout_ms"`
}

func ParseRequest(input json.RawMessage) (SearchRequest, error) {
	var raw rawRequest
	if len(input) > 0 {
		if err := json.Unmarshal(input, &raw); err != nil {
			return SearchRequest{}, fmt.Errorf("invalid web_search input: %w", err)
		}
	}
	req := SearchRequest{
		Operation:         strings.ToLower(strings.TrimSpace(raw.Operation)),
		Query:             compactWhitespace(raw.Query),
		URL:               strings.TrimSpace(raw.URL),
		SearchContextSize: strings.TrimSpace(raw.SearchContextSize),
		Limit:             raw.Limit,
		TimeRange:         strings.TrimSpace(raw.TimeRange),
		Language:          strings.TrimSpace(raw.Language),
		Country:           strings.TrimSpace(raw.Country),
		Location:          strings.TrimSpace(raw.Location),
		FetchContent:      raw.FetchContent,
		MaxContentResults: raw.MaxContentResults,
		ContentFormats:    cleanStringList(raw.ContentFormats),
		MaxBytes:          raw.MaxBytes,
	}
	if req.MaxBytes <= 0 {
		req.MaxBytes = raw.MaxBytesCamel
	}
	if req.Operation == "" {
		if req.URL != "" {
			req.Operation = OperationOpen
		} else {
			req.Operation = OperationSearch
		}
	}
	switch req.Operation {
	case OperationSearch:
		if req.Query == "" {
			return SearchRequest{}, errors.New("query is required for web_search search operation")
		}
		if isVagueQuery(req.Query) {
			return SearchRequest{}, errors.New("query is too vague; provide a precise self-contained query with entities, date or target data, and source/domain hints when relevant")
		}
	case OperationOpen:
		if req.URL == "" {
			return SearchRequest{}, errors.New("url is required for web_search open operation")
		}
	default:
		return SearchRequest{}, fmt.Errorf("unsupported web_search operation %q", req.Operation)
	}
	if req.SearchContextSize == "" {
		req.SearchContextSize = "medium"
	}
	switch req.SearchContextSize {
	case "low", "medium", "high":
	default:
		return SearchRequest{}, fmt.Errorf("invalid search_context_size %q", req.SearchContextSize)
	}
	if raw.MaxResults > 0 {
		req.Limit = raw.MaxResults
	}
	if req.Limit <= 0 {
		req.Limit = DefaultLimit
	}
	if req.Limit > 10 {
		req.Limit = 10
	}
	if req.MaxContentResults <= 0 {
		req.MaxContentResults = DefaultMaxContentResults
	}
	if req.MaxBytes <= 0 {
		req.MaxBytes = DefaultMaxBytes
	}
	if req.MaxBytes > DefaultMaxBytes {
		req.MaxBytes = DefaultMaxBytes
	}
	if raw.TimeoutMs > 0 {
		req.Timeout = time.Duration(raw.TimeoutMs) * time.Millisecond
	}
	if req.Timeout <= 0 {
		req.Timeout = DefaultTimeout
	}
	var err error
	req.AllowedDomains, err = normalizeDomainFilters(raw.AllowedDomains)
	if err != nil {
		return SearchRequest{}, fmt.Errorf("invalid allowed_domains: %w", err)
	}
	req.BlockedDomains, err = normalizeDomainFilters(raw.BlockedDomains)
	if err != nil {
		return SearchRequest{}, fmt.Errorf("invalid blocked_domains: %w", err)
	}
	if len(req.AllowedDomains) > 0 && len(req.BlockedDomains) > 0 {
		return SearchRequest{}, errors.New("allowed_domains and blocked_domains cannot both be set")
	}
	return req, nil
}

func normalizeDomainFilters(values []string) ([]string, error) {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		domain, err := normalizeDomainFilter(value)
		if err != nil {
			return nil, err
		}
		if seen[domain] {
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
	if strings.Contains(raw, "://") || strings.ContainsAny(raw, "/\\") {
		return "", fmt.Errorf("domain %q must be a hostname without protocol or path", value)
	}
	host := strings.Trim(raw, ".")
	if host == "" || strings.ContainsAny(host, " \t\r\n:") {
		return "", fmt.Errorf("invalid domain %q", value)
	}
	if ip := net.ParseIP(host); ip != nil {
		return "", fmt.Errorf("domain %q must be a hostname, not an IP address", value)
	}
	return host, nil
}

func cleanStringList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		text := strings.TrimSpace(value)
		if text == "" || seen[text] {
			continue
		}
		seen[text] = true
		out = append(out, text)
	}
	return out
}

func isVagueQuery(query string) bool {
	switch strings.ToLower(strings.TrimSpace(query)) {
	case "web", "search", "latest", "news", "网页", "搜索":
		return true
	default:
		return false
	}
}
