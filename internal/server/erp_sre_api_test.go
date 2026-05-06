package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aiops-v2/internal/appui"
)

func TestERPSREHTTPAPIsServePageData(t *testing.T) {
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil))
	ts := httptest.NewServer(srv.Handler())
	defer ts.Close()

	t.Run("runbook catalog", func(t *testing.T) {
		resp, err := http.Get(ts.URL + "/api/v1/runbooks")
		if err != nil {
			t.Fatalf("GET runbooks error = %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var payload struct {
			Runbooks []map[string]any `json:"runbooks"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode runbooks: %v", err)
		}
		if len(payload.Runbooks) == 0 {
			t.Fatalf("runbooks = 0, want catalog rows")
		}
	})

	t.Run("runbook match", func(t *testing.T) {
		body := bytes.NewBufferString(`{"capability":"订单提交","service":"order-api","environment":"prod"}`)
		resp, err := http.Post(ts.URL+"/api/v1/runbooks/match", "application/json", body)
		if err != nil {
			t.Fatalf("POST runbook match error = %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var payload struct {
			Matches []map[string]any `json:"matches"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
			t.Fatalf("decode matches: %v", err)
		}
		if len(payload.Matches) == 0 || payload.Matches[0]["runbookId"] == "" {
			t.Fatalf("matches = %#v, want runbook match rows", payload.Matches)
		}
	})

	t.Run("opsgraph lookup and neighborhood", func(t *testing.T) {
		body := bytes.NewBufferString(`{"query":"order-api"}`)
		resp, err := http.Post(ts.URL+"/api/v1/opsgraph/lookup", "application/json", body)
		if err != nil {
			t.Fatalf("POST opsgraph lookup error = %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
		var lookup struct {
			Matches []map[string]any `json:"matches"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&lookup); err != nil {
			t.Fatalf("decode lookup: %v", err)
		}
		if len(lookup.Matches) == 0 {
			t.Fatalf("lookup matches = 0, want service match")
		}

		resp, err = http.Get(ts.URL + "/api/v1/opsgraph/entities/service.order-api/neighborhood?depth=2")
		if err != nil {
			t.Fatalf("GET neighborhood error = %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	})

	t.Run("erp and changes context", func(t *testing.T) {
		for _, path := range []string{
			"/api/v1/erp/health?environment=prod",
			"/api/v1/erp/business-metrics?service=order-api",
			"/api/v1/erp/tenant-impact?capability=订单提交",
			"/api/v1/changes/deployments?service=order-api",
			"/api/v1/changes/config?service=order-api",
		} {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s error = %v", path, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Fatalf("GET %s status = %d, want 200", path, resp.StatusCode)
			}
		}
	})
}
