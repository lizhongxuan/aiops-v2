package server

import (
	"net/http"
	"net/http/httptest"
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

	t.Run("generator base returns workshop listing", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/v1/generator/", nil)
		rr := httptest.NewRecorder()

		rs.handleGeneratorWorkshop(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("status=%d want=%d", rr.Code, http.StatusOK)
		}
	})
}
