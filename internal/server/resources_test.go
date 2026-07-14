package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/store"
)

func TestResourceServer_ApprovalAndProxyHandlers(t *testing.T) {
	rs := NewResourceServer()

	t.Run("approval grants post", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/approval-grants", nil)
		rr := httptest.NewRecorder()

		rs.handleApprovalGrants(rr, req)

		if rr.Code != http.StatusCreated {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusCreated)
		}
	})

	t.Run("coroot config rejects invalid save payload", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/config", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusBadRequest {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusBadRequest)
		}
	})

	t.Run("coroot config reports unconfigured by default", func(t *testing.T) {
		t.Setenv("AIOPS_COROOT_BASE_URL", "http://env-coroot.invalid")
		t.Setenv("COROOT_BASE_URL", "http://legacy-env-coroot.invalid")
		rs := NewResourceServer()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/config", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["configured"] != false {
			t.Fatalf("configured=%v want false", body["configured"])
		}
	})

	t.Run("coroot config persists through store repository", func(t *testing.T) {
		dataStore, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
		if err != nil {
			t.Fatalf("new store: %v", err)
		}
		defer dataStore.Close()
		rs := NewResourceServer(WithCorootConfigRepository(dataStore))
		saveCorootConfigForTest(t, rs, "http://coroot.example", "test-token", "5hxbfx6p")
		if err := dataStore.Flush(); err != nil {
			t.Fatalf("flush store: %v", err)
		}

		next := NewResourceServer(WithCorootConfigRepository(dataStore))
		req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/config", nil)
		rr := httptest.NewRecorder()
		next.handleCorootProxy(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["configured"] != true || body["baseUrl"] != "http://coroot.example" || body["project"] != "5hxbfx6p" || body["tokenConfigured"] != true {
			t.Fatalf("body=%#v want persisted sanitized config", body)
		}
		if _, ok := body["token"]; ok {
			t.Fatalf("body=%#v must not expose raw token", body)
		}
	})

	t.Run("coroot proxy requires configuration for data paths", func(t *testing.T) {
		t.Setenv("AIOPS_COROOT_BASE_URL", "")
		t.Setenv("COROOT_BASE_URL", "")
		rs := NewResourceServer()
		req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/api/v1/services", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusServiceUnavailable {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusServiceUnavailable)
		}
	})

	t.Run("coroot proxy forwards allowed read requests", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/coroot/api/v1/services" {
				http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
				return
			}
			if r.URL.RawQuery != "env=prod" {
				http.Error(w, "unexpected query: "+r.URL.RawQuery, http.StatusInternalServerError)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				http.Error(w, "unexpected auth: "+got, http.StatusInternalServerError)
				return
			}
			writeResourceJSON(w, http.StatusOK, []map[string]string{{"id": "svc-api", "name": "api"}})
		}))
		defer upstream.Close()
		rs := NewResourceServer()
		saveCorootConfigForTest(t, rs, upstream.URL, "test-token", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/api/v1/services?env=prod", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "svc-api") {
			t.Fatalf("body=%s want service payload", rr.Body.String())
		}
	})

	t.Run("coroot proxy forwards project scoped read api", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/coroot/api/project/prod/overview/applications" {
				http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
				return
			}
			writeResourceJSON(w, http.StatusOK, map[string]any{"data": map[string]any{"applications": []string{"checkout"}}})
		}))
		defer upstream.Close()
		rs := NewResourceServer()
		saveCorootConfigForTest(t, rs, upstream.URL, "", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/api/project/prod/overview/applications", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		if !strings.Contains(rr.Body.String(), "checkout") {
			t.Fatalf("body=%s want project scoped API payload", rr.Body.String())
		}
	})

	t.Run("coroot test connection probes configured project", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/coroot/api/project/5hxbfx6p/overview/applications" {
				http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
				http.Error(w, "unexpected auth: "+got, http.StatusInternalServerError)
				return
			}
			writeResourceJSON(w, http.StatusOK, map[string]any{
				"context": map[string]any{"status": map[string]any{"status": "ok"}},
				"data":    map[string]any{"applications": []string{"checkout"}},
			})
		}))
		defer upstream.Close()
		rs := NewResourceServer()
		saveCorootConfigForTest(t, rs, upstream.URL, "test-token", "5hxbfx6p")
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/test-connection", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["ok"] != true || body["project"] != "5hxbfx6p" {
			t.Fatalf("body=%#v want ok project", body)
		}
	})

	t.Run("coroot test connection can probe unsaved form payload", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != "/coroot/api/project/form-project/overview/applications" {
				http.Error(w, "unexpected path: "+r.URL.Path, http.StatusInternalServerError)
				return
			}
			if got := r.Header.Get("Authorization"); got != "Bearer form-token" {
				http.Error(w, "unexpected auth: "+got, http.StatusInternalServerError)
				return
			}
			writeResourceJSON(w, http.StatusOK, map[string]any{
				"data": map[string]any{"applications": []string{"checkout", "payment"}},
			})
		}))
		defer upstream.Close()
		rs := NewResourceServer()
		payload := map[string]string{
			"baseUrl":  upstream.URL + "/coroot",
			"project":  "form-project",
			"token":    "form-token",
			"authMode": "anonymous_readonly",
			"timeout":  "5s",
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("marshal payload: %v", err)
		}
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/test-connection", bytes.NewReader(raw))
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusOK, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["ok"] != true || body["project"] != "form-project" || body["applicationCount"] != float64(2) {
			t.Fatalf("body=%#v want ok form-project with application count", body)
		}
	})

	t.Run("coroot test connection reports upstream status and body preview", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			http.Error(w, "invalid api key for project", http.StatusUnauthorized)
		}))
		defer upstream.Close()
		rs := NewResourceServer()
		saveCorootConfigForTest(t, rs, upstream.URL+"/coroot", "bad-token", "coroot_3")
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/test-connection", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusBadGateway {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadGateway, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["statusCode"] != float64(http.StatusUnauthorized) {
			t.Fatalf("body=%#v want statusCode 401", body)
		}
		uri, _ := body["uri"].(string)
		if !strings.Contains(uri, "/coroot/api/project/coroot_3/overview/applications") {
			t.Fatalf("body=%#v want probe uri", body)
		}
		responsePreview, _ := body["responsePreview"].(string)
		if !strings.Contains(responsePreview, "invalid api key") {
			t.Fatalf("body=%#v want upstream response preview", body)
		}
		detail, _ := body["detail"].(string)
		if !strings.Contains(detail, "401") {
			t.Fatalf("body=%#v want detail with HTTP status", body)
		}
	})

	t.Run("coroot test connection reports decode failure context", func(t *testing.T) {
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/html")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("<html>login page</html>"))
		}))
		defer upstream.Close()
		rs := NewResourceServer()
		saveCorootConfigForTest(t, rs, upstream.URL+"/coroot", "test-token", "coroot_3")
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/test-connection", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusBadGateway {
			t.Fatalf("status=%d want=%d body=%s", rr.Code, http.StatusBadGateway, rr.Body.String())
		}
		var body map[string]any
		if err := json.Unmarshal(rr.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if body["statusCode"] != float64(http.StatusOK) || body["contentType"] != "text/html" {
			t.Fatalf("body=%#v want status and content type", body)
		}
		responsePreview, _ := body["responsePreview"].(string)
		if !strings.Contains(responsePreview, "login page") {
			t.Fatalf("body=%#v want response preview", body)
		}
		detail, _ := body["detail"].(string)
		if !strings.Contains(detail, "non-JSON") {
			t.Fatalf("body=%#v want decode detail", body)
		}
	})

	t.Run("coroot proxy rejects non-whitelisted paths", func(t *testing.T) {
		rs := NewResourceServer()
		saveCorootConfigForTest(t, rs, "http://coroot.internal", "", "")
		req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/admin/settings", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusForbidden)
		}
	})

	t.Run("generator base returns workshop listing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/generator/", nil)
		rr := httptest.NewRecorder()

		rs.handleGeneratorWorkshop(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}
	})
}

func TestCorootConfigResponseIncludesEmbeddedWorkspacePaths(t *testing.T) {
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()
	if err := repo.SaveCorootConfig(&store.CorootConfig{
		BaseURL:       "http://172.18.13.11:8000/coroot",
		Project:       "5hxbfx6p",
		AuthMode:      "anonymous_readonly",
		LastSuccessAt: "2026-07-08T10:00:00Z",
	}); err != nil {
		t.Fatalf("SaveCorootConfig() error = %v", err)
	}

	rs := NewResourceServer(WithCorootConfigRepository(repo))
	req := httptest.NewRequest(http.MethodGet, "/api/v1/coroot/config", nil)
	rec := httptest.NewRecorder()
	rs.handleCorootProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertEqual(t, got["configured"], true)
	assertEqual(t, got["baseUrl"], "http://172.18.13.11:8000")
	assertEqual(t, got["project"], "5hxbfx6p")
	assertEqual(t, got["productBasePath"], "/coroot/")
	assertEqual(t, got["gatewayBasePath"], "/_coroot/")
	assertEqual(t, got["entryPath"], "/coroot/p/5hxbfx6p/applications")
	assertEqual(t, got["iframeEntryPath"], "/_coroot/p/5hxbfx6p/applications?embed=1")
	assertEqual(t, got["authMode"], "anonymous_readonly")
	assertEqual(t, got["tokenConfigured"], false)
	assertEqual(t, got["lastSuccessAt"], "2026-07-08T10:00:00Z")
}

func TestCorootConfigPreservesEmbedTrustSecretWithoutExposingIt(t *testing.T) {
	repo, err := store.NewJSONFileStore(t.TempDir(), time.Hour)
	if err != nil {
		t.Fatalf("NewJSONFileStore() error = %v", err)
	}
	defer repo.Close()
	rs := NewResourceServer(WithCorootConfigRepository(repo))
	payload := map[string]string{
		"baseUrl":          "http://coroot.example/coroot",
		"project":          "5hxbfx6p",
		"authMode":         "embed_trust",
		"embedTrustSecret": "shared-secret",
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/config", bytes.NewReader(raw))
	rec := httptest.NewRecorder()

	rs.handleCorootProxy(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	assertEqual(t, got["authMode"], "embed_trust")
	assertEqual(t, got["baseUrl"], "http://coroot.example")
	if _, ok := got["embedTrustSecret"]; ok {
		t.Fatalf("response=%#v must not expose embed trust secret", got)
	}
	cfg, err := repo.GetCorootConfig()
	if err != nil {
		t.Fatalf("GetCorootConfig() error = %v", err)
	}
	assertEqual(t, cfg.EmbedTrustSecret, "shared-secret")
	assertEqual(t, cfg.BaseURL, "http://coroot.example")
}

func saveCorootConfigForTest(t *testing.T, rs *ResourceServer, baseURL, token, project string) {
	t.Helper()
	payload := map[string]string{
		"baseUrl": baseURL,
		"token":   token,
		"project": project,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}
	req := httptest.NewRequest(http.MethodPut, "/api/v1/coroot/config", bytes.NewReader(raw))
	rr := httptest.NewRecorder()
	rs.handleCorootProxy(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("save coroot config status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func assertEqual(t *testing.T, got any, want any) {
	t.Helper()
	if got != want {
		t.Fatalf("got %#v, want %#v", got, want)
	}
}
