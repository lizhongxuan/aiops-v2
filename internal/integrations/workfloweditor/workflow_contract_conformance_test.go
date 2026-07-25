package workfloweditor

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"aiops-v2/internal/actionproposal"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
	contract "runner/workflow/edit"
)

type workflowMCPConformanceFixture struct {
	SchemaVersion            string   `json:"schemaVersion"`
	Tools                    []string `json:"tools"`
	UnimplementedTools       []string `json:"unimplementedTools"`
	ForbiddenExactSchemas    []string `json:"forbiddenExactSchemas"`
	ForbiddenResultFragments []string `json:"forbiddenResultFragments"`
	ProtocolVersions         []string `json:"protocolVersions"`
}

func TestWorkflowCore6ContractConformanceMCPSEC(t *testing.T) {
	fixture := loadWorkflowMCPConformanceFixture(t)
	app, _ := newWorkflowReadFixture(t)
	resources, err := NewWorkflowResourcesV2WithKey(app, workflowResourceTestKey())
	if err != nil {
		t.Fatal(err)
	}
	registry := mcp.NewRegistry()
	if err := Register(registry, app, resources); err != nil {
		t.Fatal(err)
	}
	tools := registry.ListServerTools(ServerID)
	gotNames := workflowEditorToolNames(tools)
	sort.Strings(gotNames)
	wantNames := append([]string(nil), fixture.Tools...)
	sort.Strings(wantNames)
	if !reflect.DeepEqual(gotNames, wantNames) {
		t.Fatalf("Core6 tools = %v, want %v", gotNames, wantNames)
	}

	registered := make(map[string]bool, len(tools))
	for _, tool := range tools {
		meta := tool.Metadata()
		registered[meta.Name] = true
		if !meta.IsMCP || meta.MCPInfo.ServerID != ServerID || meta.MCPInfo.ToolName != meta.Name || meta.Pack != ToolPack {
			t.Fatalf("%s MCP metadata = %#v", meta.Name, meta)
		}
		if strings.TrimSpace(meta.Description) == "" {
			t.Fatalf("%s has no user-facing description", meta.Name)
		}
		assertWorkflowToolSchemaConformance(t, fixture, meta.Name, "input", tool.InputSchema(), true)
		assertWorkflowToolSchemaConformance(t, fixture, meta.Name, "output", tool.OutputSchema(), false)
		if err := tool.ValidateInput(context.Background(), json.RawMessage(`{"__unknownConformanceField":true}`)); err == nil {
			t.Fatalf("%s accepted an unknown-only input", meta.Name)
		}
	}
	for _, name := range fixture.UnimplementedTools {
		if registered[name] {
			t.Fatalf("unimplemented tool %q was registered", name)
		}
	}
}

func TestWorkflowCore6StructuredContentTypedErrorAndResourceLinkConformance(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, err := NewWorkflowResourcesV2WithKey(app, workflowResourceTestKey())
	if err != nil {
		t.Fatal(err)
	}
	contextTool := workflowEditorToolByName(t, WorkflowReadToolsV2(app, resources), "workflow.get_context")
	result, err := contextTool.Execute(
		workflowTrustedContext("tenant-a", "user-a", "workflow-a"),
		contextInput("full", "revision-a", nil, 0, 20),
	)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(result.Content) == "" || strings.HasPrefix(strings.TrimSpace(result.Content), "{") || len(result.StructuredContent) == 0 || result.IsError {
		t.Fatalf("get_context result is not canonical MCP content: %#v", result)
	}
	if len(result.References) != 1 || result.References[0].Kind != tooling.ResultReferenceKindMCPResource || !strings.HasPrefix(result.References[0].URI, "workflow://") {
		t.Fatalf("get_context resource links = %#v", result.References)
	}
	var contextEnvelope contract.ToolResult[contract.WorkflowContext]
	if err := json.Unmarshal(result.StructuredContent, &contextEnvelope); err != nil || contextEnvelope.IsError || contextEnvelope.StructuredContent.Kind != "context" {
		t.Fatalf("get_context structuredContent = %#v, %v", contextEnvelope, err)
	}
	if err := tooling.ValidateToolResult(contextTool.OutputSchema(), result); err != nil {
		t.Fatalf("get_context output validation: %v", err)
	}

	mutationApp, backend := newWorkflowMutationFixture(t)
	stale := workflowPreviewRequest(backend.snapshot)
	stale.Expected.ResourceVersion = "stale-revision"
	stale.ValidationBasis.ResourceVersion = "stale-revision"
	previewTool := WorkflowPreviewPatchToolV2(mutationApp)
	staleResult, err := previewTool.Execute(
		workflowMutationContext("tenant-a", "user-a", "session-a", "workflow-a", "call-conformance-conflict"),
		mustWorkflowMutationJSON(t, stale),
	)
	if err != nil {
		t.Fatal(err)
	}
	var errorEnvelope contract.ToolResult[contract.DomainError]
	if err := json.Unmarshal(staleResult.StructuredContent, &errorEnvelope); err != nil || !staleResult.IsError || !errorEnvelope.IsError || errorEnvelope.StructuredContent.Code != contract.ErrorStaleRevision {
		t.Fatalf("typed conflict = %#v / %s / %v", staleResult, staleResult.StructuredContent, err)
	}
	if strings.Contains(string(staleResult.StructuredContent), `"status":"ok"`) || strings.TrimSpace(staleResult.Content) == "" {
		t.Fatalf("typed conflict used a fake/empty result: %#v", staleResult)
	}
}

func TestWorkflowApplyLostResponseRetryReturnsOnePersistedReceiptMCPSEC(t *testing.T) {
	fixture := newWorkflowApplyToolFixture(t)
	tool := WorkflowApplyPatchToolV2(fixture.app)
	raw := mustWorkflowMutationJSON(t, fixture.request)
	argumentsHash, err := actionproposal.NormalizedInputHash(raw)
	if err != nil {
		t.Fatal(err)
	}
	verified := workflowVerifiedApprovalForTest(fixture.request.WorkflowKey, fixture.request.ApplyApprovalID, "turn-apply", "call-lost-response", argumentsHash)
	ctx := workflowApplyContextWithIdentity(fixture.request.WorkflowKey, "user-a", "session-a", "turn-apply", "call-lost-response", verified)
	first, err := tool.Execute(ctx, raw)
	if err != nil || first.IsError {
		t.Fatalf("first apply = %#v, %v", first, err)
	}
	// Deliberately discard the first response to model a committed transaction
	// whose transport response was lost, then repeat the exact request.
	retried, err := tool.Execute(ctx, raw)
	if err != nil || retried.IsError || !bytes.Equal(first.StructuredContent, retried.StructuredContent) {
		t.Fatalf("lost-response retry = %#v, %v", retried, err)
	}
	var envelope contract.ToolResult[contract.WorkflowMutationReceipt]
	if err := json.Unmarshal(retried.StructuredContent, &envelope); err != nil {
		t.Fatal(err)
	}
	snapshot, err := fixture.commits.Snapshot(context.Background(), fixture.request.WorkflowKey)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.StructuredContent.TransactionID == "" || snapshot.TransactionID != envelope.StructuredContent.TransactionID || len(snapshot.ApprovalConsumptions) != 1 {
		t.Fatalf("persisted replay snapshot = %#v, receipt = %#v", snapshot, envelope.StructuredContent)
	}
}

func assertWorkflowToolSchemaConformance(t *testing.T, fixture workflowMCPConformanceFixture, toolName, direction string, raw json.RawMessage, input bool) {
	t.Helper()
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || !json.Valid(raw) {
		t.Fatalf("%s %s schema is empty or invalid: %q", toolName, direction, raw)
	}
	for _, forbidden := range fixture.ForbiddenExactSchemas {
		if trimmed == forbidden {
			t.Fatalf("%s %s schema is generic %s", toolName, direction, forbidden)
		}
	}
	for _, forbidden := range fixture.ForbiddenResultFragments {
		if strings.Contains(trimmed, forbidden) {
			t.Fatalf("%s %s schema contains fake result %s", toolName, direction, forbidden)
		}
	}
	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		t.Fatal(err)
	}
	if input {
		if schema["type"] != "object" || schema["additionalProperties"] != false {
			t.Fatalf("%s input is not a strict object schema: %#v", toolName, schema)
		}
		return
	}
	variants, ok := schema["oneOf"].([]any)
	if !ok || len(variants) != 2 {
		t.Fatalf("%s output must distinguish success and typed error: %#v", toolName, schema)
	}
	for index, rawVariant := range variants {
		variant, ok := rawVariant.(map[string]any)
		if !ok || variant["type"] != "object" || variant["additionalProperties"] != false {
			t.Fatalf("%s output variant %d is not strict: %#v", toolName, index, rawVariant)
		}
		properties, _ := variant["properties"].(map[string]any)
		for _, name := range []string{"content", "structuredContent", "isError"} {
			if properties[name] == nil {
				t.Fatalf("%s output variant %d lacks %s: %#v", toolName, index, name, properties)
			}
		}
	}
}

func loadWorkflowMCPConformanceFixture(t *testing.T) workflowMCPConformanceFixture {
	t.Helper()
	path := filepath.Join("..", "..", "..", "testdata", "workflow_canvas_mcp", "conformance", "core6-v1.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var fixture workflowMCPConformanceFixture
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if fixture.SchemaVersion != "aiops.workflow.mcp.conformance.v1" || len(fixture.Tools) != 6 || len(fixture.ProtocolVersions) == 0 {
		t.Fatalf("invalid conformance fixture: %#v", fixture)
	}
	return fixture
}
