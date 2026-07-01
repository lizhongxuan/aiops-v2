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
