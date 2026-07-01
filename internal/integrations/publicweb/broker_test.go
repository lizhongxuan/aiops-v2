package publicweb

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeSearchBackend struct {
	name    string
	results []SearchResult
	err     error
}

func (b fakeSearchBackend) Name() string {
	if b.name != "" {
		return b.name
	}
	return "fake_backend"
}

func (b fakeSearchBackend) Search(context.Context, SearchRequest) ([]SearchResult, error) {
	return append([]SearchResult(nil), b.results...), b.err
}

type fakeFetcher struct {
	result SearchResult
	err    error
}

func (f fakeFetcher) Fetch(context.Context, FetchRequest) (SearchResult, error) {
	return f.result, f.err
}

func TestBrokerSearchReturnsStructuredEnvelope(t *testing.T) {
	broker := NewBroker(fakeSearchBackend{results: []SearchResult{{
		Title:   "PostgreSQL docs",
		URL:     "https://www.postgresql.org/docs/current/continuous-archiving.html",
		Snippet: "Recovery docs.",
	}}}, nil)

	env, err := broker.Execute(context.Background(), SearchRequest{Operation: OperationSearch, Query: "PostgreSQL recovery docs", Limit: 5})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if env.Operation != OperationSearch || env.Query == "" || env.Content == "" || len(env.Results) != 1 || env.Meta.Backend == "" {
		t.Fatalf("env = %+v, want structured search envelope", env)
	}
}

func TestBrokerOpenUsesSafeFetcher(t *testing.T) {
	broker := NewBroker(nil, fakeFetcher{result: SearchResult{
		Title:      "Docs",
		URL:        "https://docs.example.com/page",
		Text:       "Readable page text.",
		Fetched:    true,
		StatusCode: 200,
	}})

	env, err := broker.Execute(context.Background(), SearchRequest{Operation: OperationOpen, URL: "https://docs.example.com/page", MaxBytes: 2000})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if env.Operation != OperationOpen || env.URL == "" || env.Content == "" || env.Meta.FetchedCount != 1 || env.Meta.FinalURL == "" {
		t.Fatalf("env = %+v, want open envelope from fetcher", env)
	}
}

func TestBrokerSearchFetchContentMergesFetchedText(t *testing.T) {
	broker := NewBroker(
		fakeSearchBackend{results: []SearchResult{{
			Title:   "Search title",
			URL:     "https://docs.example.com/page",
			Snippet: "Search snippet.",
		}}},
		fakeFetcher{result: SearchResult{
			Title:      "Fetched title",
			URL:        "https://docs.example.com/page",
			Text:       "Fetched readable text.",
			Snippet:    "Fetched readable text.",
			Fetched:    true,
			StatusCode: 200,
		}},
	)

	env, err := broker.Execute(context.Background(), SearchRequest{Operation: OperationSearch, Query: "docs", FetchContent: true, MaxContentResults: 1, Limit: 5})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(env.Results) != 1 || !env.Results[0].Fetched || !strings.Contains(env.Results[0].Text, "Fetched readable text") || env.Meta.FetchedCount != 1 {
		t.Fatalf("env = %+v, want fetched content merged", env)
	}
}

func TestBrokerSearchFetchContentKeepsSearchResultWhenFetchFails(t *testing.T) {
	broker := NewBroker(
		fakeSearchBackend{results: []SearchResult{{
			Title:   "Search title",
			URL:     "https://docs.example.com/page",
			Snippet: "Search snippet.",
		}}},
		fakeFetcher{err: errors.New("network blocked")},
	)

	env, err := broker.Execute(context.Background(), SearchRequest{Operation: OperationSearch, Query: "docs", FetchContent: true, MaxContentResults: 1, Limit: 5})
	if err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if len(env.Results) != 1 || env.Results[0].FetchError == "" || env.Results[0].Fetched {
		t.Fatalf("env = %+v, want search result preserved with fetch error", env)
	}
}
