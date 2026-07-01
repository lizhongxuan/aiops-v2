package publicweb

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

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
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusFound,
				Header:     http.Header{"Location": []string{"http://127.0.0.1/private"}},
				Body:       io.NopCloser(strings.NewReader("")),
				Request:    req,
			}, nil
		}),
	}
	fetcher := NewSafeFetcher(client)
	_, err := fetcher.Fetch(context.Background(), FetchRequest{URL: "https://docs.example.com/start", MaxBytes: 1000})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "unsafe") {
		t.Fatalf("Fetch() error = %v, want unsafe redirect rejection", err)
	}
}

func TestSafeFetchExtractsReadableHTML(t *testing.T) {
	client := &http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			body := `<!doctype html><html><head><title>Docs</title><script>bad()</script></head><body><main><h1>Docs</h1><p>Readable text.</p></main><style>.x{}</style></body></html>`
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"text/html; charset=utf-8"}},
				Body:       io.NopCloser(strings.NewReader(body)),
				Request:    req,
			}, nil
		}),
	}

	fetcher := NewSafeFetcher(client)
	result, err := fetcher.Fetch(context.Background(), FetchRequest{URL: "https://docs.example.com/page", MaxBytes: 4000})
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
