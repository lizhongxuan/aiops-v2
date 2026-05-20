package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
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

	t.Run("coroot proxy rejects non-get", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/api/v1/coroot/config", nil)
		rr := httptest.NewRecorder()

		rs.handleCorootProxy(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusMethodNotAllowed)
		}
	})

	t.Run("coroot config reports unconfigured by default", func(t *testing.T) {
		t.Setenv("AIOPS_COROOT_BASE_URL", "")
		t.Setenv("COROOT_BASE_URL", "")
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
		t.Setenv("AIOPS_COROOT_BASE_URL", upstream.URL+"/coroot")
		t.Setenv("AIOPS_COROOT_TOKEN", "test-token")
		rs := NewResourceServer()
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
		t.Setenv("AIOPS_COROOT_BASE_URL", upstream.URL+"/coroot")
		rs := NewResourceServer()
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
		t.Setenv("AIOPS_COROOT_BASE_URL", upstream.URL+"/coroot")
		t.Setenv("AIOPS_COROOT_TOKEN", "test-token")
		t.Setenv("AIOPS_COROOT_PROJECT", "5hxbfx6p")
		rs := NewResourceServer()
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

	t.Run("coroot proxy rejects non-whitelisted paths", func(t *testing.T) {
		t.Setenv("AIOPS_COROOT_BASE_URL", "http://coroot.internal")
		rs := NewResourceServer()
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
