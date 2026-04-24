package api

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestUIHandlerServesFileAndSPAFallback(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>runner-web</html>"), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.js"), []byte("console.log('ok')"), 0o644); err != nil {
		t.Fatalf("write app.js: %v", err)
	}

	handler, err := NewUIHandler(dir, "/runner-web/", nil, nil)
	if err != nil {
		t.Fatalf("new ui handler: %v", err)
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("expected root 200, got %d", rootRec.Code)
	}
	if !strings.Contains(rootRec.Body.String(), "runner-web") {
		t.Fatalf("unexpected root body: %s", rootRec.Body.String())
	}
	if !strings.Contains(rootRec.Body.String(), `<base href="/runner-web/">`) {
		t.Fatalf("missing injected base path: %s", rootRec.Body.String())
	}

	staticReq := httptest.NewRequest(http.MethodGet, "/app.js", nil)
	staticRec := httptest.NewRecorder()
	handler.ServeHTTP(staticRec, staticReq)
	if staticRec.Code != http.StatusOK {
		t.Fatalf("expected static file 200, got %d", staticRec.Code)
	}
	if !strings.Contains(staticRec.Body.String(), "console.log") {
		t.Fatalf("unexpected static body: %s", staticRec.Body.String())
	}

	spaReq := httptest.NewRequest(http.MethodGet, "/runs/run-123", nil)
	spaRec := httptest.NewRecorder()
	handler.ServeHTTP(spaRec, spaReq)
	if spaRec.Code != http.StatusOK {
		t.Fatalf("expected spa route 200, got %d", spaRec.Code)
	}
	if !strings.Contains(spaRec.Body.String(), "runner-web") {
		t.Fatalf("unexpected spa body: %s", spaRec.Body.String())
	}
}

func TestUIHandlerFallsBackToEmbeddedAssets(t *testing.T) {
	embedded := os.DirFS(t.TempDir())
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "index.html"), []byte("<html>embedded-runner-web</html>"), 0o644); err != nil {
		t.Fatalf("write embedded index: %v", err)
	}
	embedded = os.DirFS(dir)

	handler, err := NewUIHandler(filepath.Join(t.TempDir(), "missing"), "/runner-web/", embedded, nil)
	if err != nil {
		t.Fatalf("new ui handler with embedded fallback: %v", err)
	}

	rootReq := httptest.NewRequest(http.MethodGet, "/", nil)
	rootRec := httptest.NewRecorder()
	handler.ServeHTTP(rootRec, rootReq)
	if rootRec.Code != http.StatusOK {
		t.Fatalf("expected embedded root 200, got %d", rootRec.Code)
	}
	if !strings.Contains(rootRec.Body.String(), "embedded-runner-web") {
		t.Fatalf("unexpected embedded root body: %s", rootRec.Body.String())
	}
	if !strings.Contains(rootRec.Body.String(), `basePath:"/runner-web/"`) {
		t.Fatalf("missing runtime base path config: %s", rootRec.Body.String())
	}

	req := httptest.NewRequest(http.MethodGet, "/runs/run-embedded", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected embedded fallback 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "embedded-runner-web") {
		t.Fatalf("unexpected embedded fallback body: %s", rec.Body.String())
	}
}
