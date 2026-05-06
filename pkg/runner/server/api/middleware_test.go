package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

type sseRouteTestRunHandler struct{}

func (sseRouteTestRunHandler) Submit(w http.ResponseWriter, r *http.Request) {}
func (sseRouteTestRunHandler) Get(w http.ResponseWriter, r *http.Request)    {}
func (sseRouteTestRunHandler) List(w http.ResponseWriter, r *http.Request)   {}
func (sseRouteTestRunHandler) Cancel(w http.ResponseWriter, r *http.Request) {}
func (sseRouteTestRunHandler) EventsHistory(w http.ResponseWriter, r *http.Request) {
}
func (sseRouteTestRunHandler) Events(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("event: ping\n\n"))
}

func TestAuthMiddlewareRejectsUnauthorized(t *testing.T) {
	handler := AuthMiddleware(true, "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestAuthMiddlewareAllowsAuthorized(t *testing.T) {
	handler := AuthMiddleware(true, "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/workflows", nil)
	req.Header.Set("Authorization", "Bearer secret")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}

func TestRouterServesSSEWithBasePathCORSAndQueryToken(t *testing.T) {
	router := NewRouter(RouterOptions{
		AuthEnabled: true,
		AuthToken:   "secret",
		CORSOrigins: []string{"http://localhost:5174"},
		UIBasePath:  "/runner-web/",
		Run:         sseRouteTestRunHandler{},
	})

	req := httptest.NewRequest(http.MethodGet, "/runner-web/api/v1/runs/run-1/events?access_token=secret", nil)
	req.Header.Set("Origin", "http://localhost:5174")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected sse route 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:5174" {
		t.Fatalf("missing CORS origin header: %+v", rec.Header())
	}
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected event-stream content type, got %q", rec.Header().Get("Content-Type"))
	}
}

func TestAuthMiddlewareAllowsSSEQueryToken(t *testing.T) {
	handler := AuthMiddleware(true, "secret")(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/runs/run-1/events?access_token=secret", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
