package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"testing"

	"aiops-v2/internal/appui"
)

func TestERPSREHTTPAPIsServePageData(t *testing.T) {
	ts := newOpsGraphHTTPTestServer(t)
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
		graphID := createOpsGraphHTTPFixture(t, ts.URL, "graph.legacy")
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
		var neighborhood struct {
			Entity        map[string]any   `json:"entity"`
			Neighbors     []map[string]any `json:"neighbors"`
			Entities      []map[string]any `json:"entities"`
			Relationships []map[string]any `json:"relationships"`
			Depth         int              `json:"depth"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&neighborhood); err != nil {
			t.Fatalf("decode neighborhood: %v", err)
		}
		if neighborhood.Entity["id"] != "service.order-api" || len(neighborhood.Entities) != 2 {
			t.Fatalf("legacy neighborhood = %#v, want top-level service and host", neighborhood)
		}
		if graphID == "" {
			t.Fatal("graph fixture id empty")
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

func TestOpsGraphManualAuthoringHTTPAPIs(t *testing.T) {
	ts := newOpsGraphHTTPTestServer(t)
	defer ts.Close()

	graphID := "graph.prod-core"
	createGraph := bytes.NewBufferString(`{"id":"` + graphID + `","name":"生产环境核心链路","environment":"prod","isDefault":true}`)
	resp, err := http.Post(ts.URL+"/api/v1/opsgraph/graphs", "application/json", createGraph)
	if err != nil {
		t.Fatalf("POST graphs error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST graphs status = %d, want 200", resp.StatusCode)
	}
	var graphPayload struct {
		Graph map[string]any `json:"graph"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&graphPayload); err != nil {
		t.Fatalf("decode graph: %v", err)
	}
	if graphPayload.Graph["id"] != graphID {
		t.Fatalf("graph id = %#v, want %s", graphPayload.Graph["id"], graphID)
	}

	graphPath := url.PathEscape(graphID)
	postJSON(t, ts.URL+"/api/v1/opsgraph/graphs/"+graphPath+"/entities", `{"id":"service.order-api","type":"service","name":"order-api"}`)
	postJSON(t, ts.URL+"/api/v1/opsgraph/graphs/"+graphPath+"/entities", `{"id":"host.erp-node-a","type":"host","name":"erp-node-a","container":true}`)
	postJSON(t, ts.URL+"/api/v1/opsgraph/graphs/"+graphPath+"/relationships", `{"id":"edge.service.order-api.runs_on.host.erp-node-a","from":"service.order-api","type":"runs_on","to":"host.erp-node-a"}`)
	postJSON(t, ts.URL+"/api/v1/opsgraph/graphs/"+graphPath+"/layout", `{"nodes":[{"id":"service.order-api","position":{"x":10,"y":20}}],"viewport":{"x":1,"y":2,"zoom":0.8}}`)

	resp, err = http.Get(ts.URL + "/api/v1/opsgraph/graphs/" + graphPath + "/entities/service.order-api/neighborhood?depth=1")
	if err != nil {
		t.Fatalf("GET neighborhood error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET neighborhood status = %d, want 200", resp.StatusCode)
	}
	var neighborhood struct {
		Entity        map[string]any   `json:"entity"`
		Neighbors     []map[string]any `json:"neighbors"`
		Entities      []map[string]any `json:"entities"`
		Relationships []map[string]any `json:"relationships"`
		Depth         int              `json:"depth"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&neighborhood); err != nil {
		t.Fatalf("decode neighborhood: %v", err)
	}
	if neighborhood.Entity["id"] != "service.order-api" || len(neighborhood.Entities) != 2 || len(neighborhood.Relationships) != 1 {
		t.Fatalf("neighborhood = %#v, want service, host and relationship", neighborhood)
	}

	resp, err = http.Get(ts.URL + "/api/v1/opsgraph/graphs/" + graphPath + "/validate")
	if err != nil {
		t.Fatalf("GET validate error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET validate status = %d, want 200", resp.StatusCode)
	}
	var validation struct {
		Issues []map[string]any `json:"issues"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&validation); err != nil {
		t.Fatalf("decode validation: %v", err)
	}
	if len(validation.Issues) == 0 {
		t.Fatalf("issues = 0, want service owner warning for authoring feedback")
	}

	resp, err = http.Get(ts.URL + "/api/v1/opsgraph/graphs")
	if err != nil {
		t.Fatalf("GET graphs error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET graphs status = %d, want 200", resp.StatusCode)
	}
	var list struct {
		Graphs []map[string]any `json:"graphs"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("decode graphs: %v", err)
	}
	if len(list.Graphs) != 1 || list.Graphs[0]["id"] != graphID {
		t.Fatalf("graphs = %#v, want created graph", list.Graphs)
	}
}

func newOpsGraphHTTPTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	service := appui.NewOpsGraphService(filepath.Join(t.TempDir(), "manual.graph.json"))
	srv := NewHTTPServer(appui.NewServices(websocketAPITestRuntime{}, nil, appui.WithOpsGraphService(service)))
	return httptest.NewServer(srv.Handler())
}

func createOpsGraphHTTPFixture(t *testing.T, baseURL, graphID string) string {
	t.Helper()
	createGraph := bytes.NewBufferString(`{"id":"` + graphID + `","name":"测试图谱","isDefault":true}`)
	resp, err := http.Post(baseURL+"/api/v1/opsgraph/graphs", "application/json", createGraph)
	if err != nil {
		t.Fatalf("POST fixture graph error = %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST fixture graph status = %d, want 200", resp.StatusCode)
	}
	graphPath := url.PathEscape(graphID)
	postJSON(t, baseURL+"/api/v1/opsgraph/graphs/"+graphPath+"/entities", `{"id":"service.order-api","type":"service","name":"order-api","labels":{"owner":"platform"}}`)
	postJSON(t, baseURL+"/api/v1/opsgraph/graphs/"+graphPath+"/entities", `{"id":"host.erp-node-a","type":"host","name":"erp-node-a","container":true}`)
	postJSON(t, baseURL+"/api/v1/opsgraph/graphs/"+graphPath+"/relationships", `{"id":"edge.service.order-api.runs_on.host.erp-node-a","from":"service.order-api","type":"runs_on","to":"host.erp-node-a"}`)
	return graphID
}

func postJSON(t *testing.T, rawURL, body string) {
	t.Helper()
	resp, err := http.Post(rawURL, "application/json", bytes.NewBufferString(body))
	if err != nil {
		t.Fatalf("POST %s error = %v", rawURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("POST %s status = %d, want 200", rawURL, resp.StatusCode)
	}
}
