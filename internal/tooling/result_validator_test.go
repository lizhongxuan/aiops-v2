package tooling

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestValidateToolResultStructuredContentAgainstOutputSchema(t *testing.T) {
	schema := json.RawMessage(`{
		"type":"object",
		"required":["status","count"],
		"properties":{"status":{"type":"string"},"count":{"type":"integer"}},
		"additionalProperties":false
	}`)
	if err := ValidateToolResult(schema, ToolResult{StructuredContent: json.RawMessage(`{"status":"ready","count":2}`)}); err != nil {
		t.Fatalf("valid structured result: %v", err)
	}
	for _, result := range []ToolResult{
		{StructuredContent: json.RawMessage(`{"status":"ready"}`)},
		{StructuredContent: json.RawMessage(`{"status":"ready","count":"2"}`)},
		{StructuredContent: json.RawMessage(`{"status":"ready","count":2,"unknown":true}`)},
	} {
		if err := ValidateToolResult(schema, result); err == nil {
			t.Fatalf("ValidateToolResult(%s) accepted invalid output", result.StructuredContent)
		}
	}
}

func TestValidateToolResultSupportsLegacyContentAndRejectsMalformedStructuredContent(t *testing.T) {
	objectSchema := json.RawMessage(`{"type":"object","required":["status"],"properties":{"status":{"type":"string"}}}`)
	if err := ValidateToolResult(objectSchema, ToolResult{Content: `{"status":"ready"}`}); err != nil {
		t.Fatalf("legacy JSON content: %v", err)
	}
	if err := ValidateToolResult(json.RawMessage(`{"type":"string"}`), ToolResult{Content: "ready"}); err != nil {
		t.Fatalf("legacy string content: %v", err)
	}
	err := ValidateToolResult(objectSchema, ToolResult{StructuredContent: json.RawMessage(`{`)})
	if err == nil || !strings.Contains(err.Error(), "valid JSON") {
		t.Fatalf("malformed structured output error = %v", err)
	}
}

func TestToolResultIsErrorIsAnExecutionFailure(t *testing.T) {
	result := ToolResult{IsError: true, StructuredContent: json.RawMessage(`{"code":"STALE_REVISION"}`), Content: "reload required"}
	if !result.ExecutionFailed() {
		t.Fatal("isError=true must be an execution failure")
	}
	if got := result.FailureText(); !strings.Contains(got, "reload required") {
		t.Fatalf("FailureText() = %q", got)
	}
}
