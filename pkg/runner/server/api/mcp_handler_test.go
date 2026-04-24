package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"runner/server/service"
	"runner/server/store/mcpstore"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (fn roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

func TestMCPHandlerCreateToggleAndTools(t *testing.T) {
	t.Parallel()

	svc := service.NewMcpService(mcpstore.NewFileStore(t.TempDir()))
	svc.SetHTTPClient(&http.Client{
		Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			switch {
			case req.Method == http.MethodGet && req.URL.Path == "/mcp":
				payload, _ := json.Marshal(map[string]any{"status": "ok"})
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(payload))),
					Request:    req,
				}, nil
			case req.Method == http.MethodGet && req.URL.Path == "/mcp/tools":
				payload, _ := json.Marshal(map[string]any{
					"tools": []map[string]any{
						{
							"name":        "search_docs",
							"description": "Search docs",
							"inputSchema": map[string]any{"type": "object"},
						},
					},
				})
				return &http.Response{
					StatusCode: http.StatusOK,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(string(payload))),
					Request:    req,
				}, nil
			default:
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Header:     make(http.Header),
					Body:       io.NopCloser(strings.NewReader(`{"error":"not found"}`)),
					Request:    req,
				}, nil
			}
		}),
	})
	router := NewRouter(RouterOptions{
		MCP: NewMcpHandler(svc),
	})

	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/servers", strings.NewReader(`{
		"id":"docs",
		"name":"Docs MCP",
		"type":"http",
		"url":"http://mcp.mock/mcp",
		"env_vars":{"TOKEN":"secret"}
	}`))
	createReq.Header.Set("Content-Type", "application/json")
	createRec := httptest.NewRecorder()
	router.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("create status = %d, body = %s", createRec.Code, createRec.Body.String())
	}

	toggleReq := httptest.NewRequest(http.MethodPost, "/api/v1/mcp/servers/docs/toggle", nil)
	toggleRec := httptest.NewRecorder()
	router.ServeHTTP(toggleRec, toggleReq)
	if toggleRec.Code != http.StatusOK {
		t.Fatalf("toggle status = %d, body = %s", toggleRec.Code, toggleRec.Body.String())
	}

	var toggled struct {
		Status string `json:"status"`
	}
	if err := json.Unmarshal(toggleRec.Body.Bytes(), &toggled); err != nil {
		t.Fatalf("decode toggle: %v", err)
	}
	if toggled.Status != mcpstore.StatusRunning {
		t.Fatalf("unexpected toggle status: %s", toggled.Status)
	}

	toolsReq := httptest.NewRequest(http.MethodGet, "/api/v1/mcp/servers/docs/tools", nil)
	toolsRec := httptest.NewRecorder()
	router.ServeHTTP(toolsRec, toolsReq)
	if toolsRec.Code != http.StatusOK {
		t.Fatalf("tools status = %d, body = %s", toolsRec.Code, toolsRec.Body.String())
	}

	var toolsPayload struct {
		Items []struct {
			Name string `json:"name"`
		} `json:"items"`
	}
	if err := json.Unmarshal(toolsRec.Body.Bytes(), &toolsPayload); err != nil {
		t.Fatalf("decode tools: %v", err)
	}
	if len(toolsPayload.Items) != 1 || toolsPayload.Items[0].Name != "search_docs" {
		t.Fatalf("unexpected tools payload: %+v", toolsPayload.Items)
	}
}
