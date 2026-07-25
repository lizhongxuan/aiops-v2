package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func TestToolCallResultPreservesStructuredMCPProtocolResult(t *testing.T) {
	raw := []byte(`{
		"content":[
			{"type":"text","text":"Found one runbook"},
			{"type":"resource_link","uri":"runbook://db/latency","name":"DB latency","title":"Database latency runbook","description":"Recovery steps","mimeType":"text/markdown","size":321,"extension":{"kept":true}}
		],
		"structuredContent":{"status":"degraded","affected":2},
		"isError":true,
		"protocolExtension":{"trace":"trace-1"}
	}`)
	var result ToolCallResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatalf("decode MCP result: %v", err)
	}
	if !result.IsError || len(result.ContentBlocks) != 2 {
		t.Fatalf("decoded result = %#v", result)
	}
	assertJSONEqual(t, result.StructuredContent, `{"status":"degraded","affected":2}`)
	assertJSONEqual(t, result.Raw, string(raw))
	if !strings.Contains(string(result.ContentBlocks[1]), `"extension":{"kept":true}`) {
		t.Fatalf("content block extension was lost: %s", result.ContentBlocks[1])
	}
}

func TestMCPToolAdapterMapsStructuredContentErrorAndResourceLinks(t *testing.T) {
	var protocolResult ToolCallResult
	if err := json.Unmarshal([]byte(`{
		"content":[
			{"type":"text","text":"Found one runbook"},
			{"type":"text","text":"Check replicas"},
			{"type":"resource_link","uri":"runbook://db/latency","name":"DB latency","title":"Database latency runbook","description":"Recovery steps","mimeType":"text/markdown","size":321},
			{"type":"resource_link","uri":"","name":"invalid"}
		],
		"structuredContent":{"status":"degraded","affected":2},
		"isError":true
	}`), &protocolResult); err != nil {
		t.Fatal(err)
	}
	tool := makeTool(
		mcp.ServerConfig{ID: "runbooks", Name: "Runbooks"},
		mcp.ServerGovernance{},
		ToolDefinition{Name: "search", ReadOnly: true},
		func(context.Context, string, string, json.RawMessage) (ToolCallResult, error) {
			return protocolResult, nil
		},
	)
	result, err := tool.Execute(context.Background(), json.RawMessage(`{"q":"latency"}`))
	if err != nil {
		t.Fatalf("tool-level isError became Go error: %v", err)
	}
	if result.Content != "Found one runbook\nCheck replicas" || !result.IsError {
		t.Fatalf("mapped result content=%q isError=%v", result.Content, result.IsError)
	}
	assertJSONEqual(t, result.StructuredContent, `{"status":"degraded","affected":2}`)
	if len(result.References) != 1 {
		t.Fatalf("references = %#v", result.References)
	}
	want := tooling.ResultReference{
		Kind:        tooling.ResultReferenceKindMCPResource,
		URI:         "runbook://db/latency",
		Title:       "Database latency runbook",
		Summary:     "Recovery steps",
		ContentType: "text/markdown",
		Bytes:       321,
	}
	if result.References[0] != want {
		t.Fatalf("reference = %#v, want %#v", result.References[0], want)
	}
}

func TestMCPToolAdapterSeparatesCallErrorsFromToolExecutionErrors(t *testing.T) {
	transportErr := errors.New("transport unavailable")
	transportTool := makeTool(mcp.ServerConfig{ID: "ops"}, mcp.ServerGovernance{}, ToolDefinition{Name: "inspect"}, func(context.Context, string, string, json.RawMessage) (ToolCallResult, error) {
		return ToolCallResult{}, transportErr
	})
	if _, err := transportTool.Execute(context.Background(), nil); !errors.Is(err, transportErr) {
		t.Fatalf("transport error = %v", err)
	}

	executionTool := makeTool(mcp.ServerConfig{ID: "ops"}, mcp.ServerGovernance{}, ToolDefinition{Name: "inspect"}, func(context.Context, string, string, json.RawMessage) (ToolCallResult, error) {
		return ToolCallResult{Content: "inspection failed", Error: "remote tool rejected input", IsError: true}, nil
	})
	result, err := executionTool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatalf("execution isError became Go error: %v", err)
	}
	if !result.IsError || result.Error != "remote tool rejected input" || result.Content != "inspection failed" {
		t.Fatalf("execution result = %#v", result)
	}
}

func TestMCPToolAdapterKeepsLegacyStringContent(t *testing.T) {
	tool := makeTool(mcp.ServerConfig{ID: "legacy"}, mcp.ServerGovernance{}, ToolDefinition{Name: "read"}, func(context.Context, string, string, json.RawMessage) (ToolCallResult, error) {
		return ToolCallResult{Content: "legacy text", Error: "legacy diagnostic"}, nil
	})
	result, err := tool.Execute(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Content != "legacy text" || result.Error != "legacy diagnostic" || result.IsError {
		t.Fatalf("legacy result = %#v", result)
	}
}

func assertJSONEqual(t *testing.T, actual json.RawMessage, expected string) {
	t.Helper()
	var left, right any
	if err := json.Unmarshal(actual, &left); err != nil {
		t.Fatalf("invalid actual JSON %q: %v", actual, err)
	}
	if err := json.Unmarshal([]byte(expected), &right); err != nil {
		t.Fatal(err)
	}
	leftJSON, _ := json.Marshal(left)
	rightJSON, _ := json.Marshal(right)
	if string(leftJSON) != string(rightJSON) {
		t.Fatalf("JSON = %s, want %s", leftJSON, rightJSON)
	}
}
