package server

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
)

type webAssetsTestKernel struct{}

func (webAssetsTestKernel) RunTurn(context.Context, runtimekernel.TurnRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (webAssetsTestKernel) ResumeTurn(context.Context, runtimekernel.ResumeRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func (webAssetsTestKernel) CancelTurn(context.Context, runtimekernel.CancelRequest) (runtimekernel.TurnResult, error) {
	return runtimekernel.TurnResult{}, nil
}

func TestWebAssetsHandlerServesIndexAssetsAndSPAFallback(t *testing.T) {
	distDir := writeWebDistFixture(t)
	handler, err := NewWebAssetsHandler(distDir)
	if err != nil {
		t.Fatalf("NewWebAssetsHandler() error = %v", err)
	}

	server := httptest.NewServer(NewHTTPServer(appui.NewServices(webAssetsTestKernel{}, nil), WithWebAssets(handler)).Handler())
	defer server.Close()

	assertBodyContains(t, server.URL+"/", http.StatusOK, "<div id=\"app\">web root</div>")
	assertBodyContains(t, server.URL+"/workspaces/demo", http.StatusOK, "<div id=\"app\">web root</div>")
	assertBodyContains(t, server.URL+"/assets/app.js", http.StatusOK, "console.log('web asset');")
}

func TestWebAssetsHandlerDoesNotSwallowAPIOrWebsocketPaths(t *testing.T) {
	distDir := writeWebDistFixture(t)
	handler, err := NewWebAssetsHandler(distDir)
	if err != nil {
		t.Fatalf("NewWebAssetsHandler() error = %v", err)
	}

	server := httptest.NewServer(NewHTTPServer(appui.NewServices(webAssetsTestKernel{}, nil), WithWebAssets(handler)).Handler())
	defer server.Close()

	assertBodyContains(t, server.URL+"/api/v1/state", http.StatusOK, "\"selectedHostId\":\"server-local\"")
	assertBodyExcludes(t, server.URL+"/api/v1/unknown", http.StatusNotFound, "<div id=\"app\">web root</div>")
	assertBodyExcludes(t, server.URL+"/ws", http.StatusBadRequest, "<div id=\"app\">web root</div>")
	assertBodyExcludes(t, server.URL+"/api/v1/terminal/ws", http.StatusBadRequest, "<div id=\"app\">web root</div>")
}

func TestNewWebAssetsHandlerRequiresIndexHTML(t *testing.T) {
	distDir := t.TempDir()
	if _, err := NewWebAssetsHandler(distDir); err == nil {
		t.Fatal("NewWebAssetsHandler() error = nil, want missing index error")
	}
}

func writeWebDistFixture(t *testing.T) string {
	t.Helper()
	distDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(distDir, "index.html"), []byte("<!doctype html><div id=\"app\">web root</div>"), 0o644); err != nil {
		t.Fatalf("WriteFile(index.html) error = %v", err)
	}
	if err := os.MkdirAll(filepath.Join(distDir, "assets"), 0o755); err != nil {
		t.Fatalf("MkdirAll(assets) error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(distDir, "assets", "app.js"), []byte("console.log('web asset');"), 0o644); err != nil {
		t.Fatalf("WriteFile(app.js) error = %v", err)
	}
	return distDir
}

func assertBodyContains(t *testing.T, url string, wantStatus int, wantBody string) {
	t.Helper()
	resp, body := doRequest(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s status = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	if !strings.Contains(body, wantBody) {
		t.Fatalf("%s body missing %q: %s", url, wantBody, body)
	}
}

func assertBodyExcludes(t *testing.T, url string, wantStatus int, excluded string) {
	t.Helper()
	resp, body := doRequest(t, url)
	defer resp.Body.Close()
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s status = %d, want %d", url, resp.StatusCode, wantStatus)
	}
	if strings.Contains(body, excluded) {
		t.Fatalf("%s unexpectedly returned SPA body: %s", url, body)
	}
}

func doRequest(t *testing.T, url string) (*http.Response, string) {
	t.Helper()
	resp, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s error = %v", url, err)
	}
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		resp.Body.Close()
		t.Fatalf("ReadAll(%s) error = %v", url, err)
	}
	resp.Body = io.NopCloser(strings.NewReader(string(bodyBytes)))
	return resp, string(bodyBytes)
}
