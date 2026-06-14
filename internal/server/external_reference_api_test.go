package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/appui"
	"aiops-v2/internal/runtimekernel"
	"aiops-v2/internal/store"
	"aiops-v2/internal/tooling"
)

func TestExternalReferenceAPIReadsToolSpill(t *testing.T) {
	server := newTestServerWithToolSpill(t, tooling.ResultSpill{
		ID:          "spill-1",
		ContentType: "text/plain",
		Summary:     "raw evidence summary",
		Content:     []byte("raw evidence"),
		Bytes:       12,
		CreatedAt:   time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-references/spill-1", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload externalReferenceResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.ID != "spill-1" || payload.Kind != "blob" {
		t.Fatalf("payload identity = %#v", payload)
	}
	if payload.Content != "raw evidence" {
		t.Fatalf("content = %q, want raw evidence", payload.Content)
	}
	if !strings.HasPrefix(payload.Digest, "sha256:") {
		t.Fatalf("digest = %q", payload.Digest)
	}
}

func TestExternalReferenceAPIReadsRange(t *testing.T) {
	server := newTestServerWithToolSpill(t, tooling.ResultSpill{
		ID:          "spill-1",
		ContentType: "text/plain",
		Summary:     "bounded summary",
		Content:     []byte("alpha\nbeta\ngamma"),
		Bytes:       int64(len("alpha\nbeta\ngamma")),
		CreatedAt:   time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
	})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-references/tool-spills/spill-1?offset=6&limit=4", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	var payload externalReferenceResponse
	if err := json.NewDecoder(rec.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Content != "beta" {
		t.Fatalf("content = %q, want beta", payload.Content)
	}
	if !payload.Truncated {
		t.Fatalf("truncated = false, want true")
	}
	if payload.Range.Offset != 6 || payload.Range.Limit != 4 {
		t.Fatalf("range = %#v, want offset 6 limit 4", payload.Range)
	}
}

func TestExternalReferenceAPINotFound(t *testing.T) {
	server := newTestServerWithToolSpill(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-references/missing", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestExternalReferenceAPIRejectsUnknownKind(t *testing.T) {
	server := newTestServerWithToolSpill(t, tooling.ResultSpill{ID: "file-1", Content: []byte("raw")})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-references/file-1", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "unknown external reference kind") {
		t.Fatalf("body = %s", rec.Body.String())
	}
}

func TestExternalReferenceAPIRejectsNestedUnknownKind(t *testing.T) {
	server := newTestServerWithToolSpill(t)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/external-references/file/ref-1", nil)
	rec := httptest.NewRecorder()

	server.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body=%s", rec.Code, rec.Body.String())
	}
}

func newTestServerWithToolSpill(t *testing.T, spills ...tooling.ResultSpill) http.Handler {
	t.Helper()
	dataStore, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	t.Cleanup(func() { _ = dataStore.Close() })
	for i := range spills {
		if err := dataStore.SaveToolResultSpill(&spills[i]); err != nil {
			t.Fatalf("SaveToolResultSpill() error = %v", err)
		}
	}
	sessionMgr := runtimekernel.NewSessionManager(dataStore)
	srv := NewHTTPServer(appui.NewServices(sessionAPITestRuntime{}, sessionMgr, appui.WithStore(dataStore)))
	return srv.Handler()
}
