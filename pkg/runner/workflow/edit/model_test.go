package edit

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestPreviewPatchV2GoldenRoundTrip(t *testing.T) {
	raw, err := os.ReadFile("testdata/preview_patch_v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	var request PreviewPatchRequest
	if err := DecodeStrict(raw, &request); err != nil {
		t.Fatalf("DecodeStrict() error = %v", err)
	}
	if request.SchemaVersion != SchemaVersion {
		t.Fatalf("schemaVersion = %q, want %q", request.SchemaVersion, SchemaVersion)
	}
	if err := request.WorkflowKey.Validate(); err != nil {
		t.Fatalf("WorkflowKey.Validate() error = %v", err)
	}
	if err := request.ValidationBasis.Validate(); err != nil {
		t.Fatalf("ValidationBasis.Validate() error = %v", err)
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		t.Fatalf("Marshal() error = %v", err)
	}
	if !jsonSemanticallyEqual(t, raw, encoded) {
		t.Fatalf("round trip changed fixture\nwant: %s\n got: %s", raw, encoded)
	}
}

func TestWorkflowOperationRejectsUnknownOperationAndFields(t *testing.T) {
	for name, raw := range map[string]string{
		"unknown operation": `{"op":"replace_full_graph","graph":{}}`,
		"unknown field":     `{"op":"delete_node","nodeId":"n1","fields":{}}`,
		"ui escape hatch":   `{"op":"add_node","clientRef":"n1","node":{"type":"action","ui":{"trusted":false}}}`,
		"trailing data":     `{"op":"delete_node","nodeId":"n1","cascadeEdges":false}{}`,
	} {
		t.Run(name, func(t *testing.T) {
			var operation WorkflowOperation
			if err := json.Unmarshal([]byte(raw), &operation); err == nil {
				t.Fatalf("Unmarshal(%s) unexpectedly succeeded", raw)
			}
		})
	}
}

func TestWorkflowKeyIsTenantScopedAndPathSafe(t *testing.T) {
	if reflect.DeepEqual(
		WorkflowKey{TenantID: "tenant-a", WorkflowID: "shared"},
		WorkflowKey{TenantID: "tenant-b", WorkflowID: "shared"},
	) {
		t.Fatal("same workflowId in different tenants must not be the same key")
	}
	for _, key := range []WorkflowKey{
		{TenantID: "", WorkflowID: "workflow"},
		{TenantID: "tenant", WorkflowID: "../workflow"},
		{TenantID: "tenant/path", WorkflowID: "workflow"},
	} {
		if err := key.Validate(); err == nil {
			t.Fatalf("Validate(%+v) unexpectedly succeeded", key)
		}
	}
}

func TestContractHasNoLegacyFakeSuccessOrCredentialFields(t *testing.T) {
	raw, err := os.ReadFile("testdata/preview_patch_v2.golden.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"status":"ok"`, `"password"`, `"apiKey"`, `"token"`} {
		if strings.Contains(string(raw), forbidden) {
			t.Fatalf("golden contains forbidden field %s", forbidden)
		}
	}
}

func TestStrictRequestAndToolErrorCompatibilitySummary(t *testing.T) {
	var request PreviewPatchRequest
	if err := DecodeStrict([]byte(`{"status":"ok"}`), &request); err == nil {
		t.Fatal("legacy fake success field must not be accepted as a v2 request")
	}
	domainError := DomainError{Kind: "error", Code: ErrorStaleRevision, Message: "workflow changed"}
	result := NewToolError("Workflow changed; reload before retrying.", domainError)
	if !result.IsError || result.SchemaVersion != SchemaVersion {
		t.Fatalf("tool error envelope = %#v", result)
	}
	if len(result.Content) != 1 || result.Content[0].Type != "text" || result.Content[0].Text == "" {
		t.Fatalf("legacy TextContent summary = %#v", result.Content)
	}
	if result.StructuredContent.Code != ErrorStaleRevision {
		t.Fatalf("structured error = %#v", result.StructuredContent)
	}
}

func jsonSemanticallyEqual(t *testing.T, left, right []byte) bool {
	t.Helper()
	var a, b any
	if err := json.Unmarshal(left, &a); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(right, &b); err != nil {
		t.Fatal(err)
	}
	return reflect.DeepEqual(a, b)
}
