package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestBasePathMiddlewareStripsConfiguredPrefix(t *testing.T) {
	handler := BasePathMiddleware("/runner-web/")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	req := httptest.NewRequest(http.MethodGet, "/runner-web/api/v1/system/info", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "/api/v1/system/info" {
		t.Fatalf("expected stripped path, got %q", rec.Body.String())
	}
}

func TestBasePathMiddlewareKeepsRootPathAvailable(t *testing.T) {
	handler := BasePathMiddleware("/runner-web/")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(r.URL.Path))
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "/healthz" {
		t.Fatalf("expected original root path, got %q", rec.Body.String())
	}
}
