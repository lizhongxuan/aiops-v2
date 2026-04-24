package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"runner/server/store/mcpstore"
)

func TestMcpServiceToggleHTTPAndDiscoverTools(t *testing.T) {
	t.Parallel()

	store := mcpstore.NewFileStore(t.TempDir())
	svc := NewMcpService(store)
	svc.SetHTTPClient(&http.Client{
		Transport: testRoundTripFunc(func(req *http.Request) (*http.Response, error) {
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
							"description": "Search documentation",
							"inputSchema": map[string]any{
								"type": "object",
							},
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
	ctx := context.Background()

	if err := svc.Create(ctx, &mcpstore.ServerRecord{
		ID:   "docs",
		Name: "Docs MCP",
		Type: mcpstore.TypeHTTP,
		URL:  "http://mcp.mock/mcp",
	}); err != nil {
		t.Fatalf("create: %v", err)
	}

	record, err := svc.Toggle(ctx, "docs")
	if err != nil {
		t.Fatalf("toggle: %v", err)
	}
	if record.Status != mcpstore.StatusRunning {
		t.Fatalf("unexpected status: %s", record.Status)
	}
	if len(record.Tools) != 1 || record.Tools[0].Name != "search_docs" {
		t.Fatalf("unexpected tools: %#v", record.Tools)
	}

	tools, err := svc.ListTools(ctx, "docs")
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 1 {
		t.Fatalf("unexpected tool count: %d", len(tools))
	}
}
