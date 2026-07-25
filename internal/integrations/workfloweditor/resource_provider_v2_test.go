package workfloweditor

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
	contract "runner/workflow/edit"
)

func TestWorkflowResourceProviderV2ListsPaginatesAndReadsFresh(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, err := NewWorkflowResourcesV2WithKey(app, workflowResourceTestKey())
	if err != nil {
		t.Fatal(err)
	}
	provider, err := NewWorkflowResourceProviderV2(resources)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := WorkflowResourceProviderRegistrationV2(provider)
	if err != nil {
		t.Fatal(err)
	}
	registry := mcp.NewRegistry()
	if err := registry.RegisterResourceProvider(registration); err != nil {
		t.Fatal(err)
	}
	ctx := workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-a")
	first, err := registry.ListResourcesScoped(ctx, mcp.ResourceListRequest{ServerID: ServerID, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Resources) != 1 || first.NextCursor == "" || !allWorkflowResourceCapabilities(first.Capabilities) {
		t.Fatalf("first page = %#v", first)
	}
	second, err := registry.ListResourcesScoped(ctx, mcp.ResourceListRequest{ServerID: ServerID, Cursor: first.NextCursor, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Resources) != 1 || second.NextCursor != "" || first.Resources[0].URI == second.Resources[0].URI {
		t.Fatalf("second page = %#v", second)
	}
	for _, resource := range append(first.Resources, second.Resources...) {
		if resource.Version == "" || resource.MimeType != "application/json" || strings.Contains(resource.URI, "tenant-a") || strings.Contains(resource.URI, "workflow-a") || strings.Contains(resource.URI, "user-a") {
			t.Fatalf("non-opaque revision resource = %#v", resource)
		}
		content, ok, readErr := registry.ReadResource(ctx, ServerID, resource.URI)
		if readErr != nil || !ok || content.Version != resource.Version || content.Text == "" || content.Digest == "" {
			t.Fatalf("ReadResource(%s) = %#v, %v, %v", resource.Name, content, ok, readErr)
		}
	}
}

func TestWorkflowResourceProviderV2FailsClosedWithOneUnavailableError(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, err := NewWorkflowResourcesV2WithKey(app, workflowResourceTestKey())
	if err != nil {
		t.Fatal(err)
	}
	provider, _ := NewWorkflowResourceProviderV2(resources)
	registration, _ := WorkflowResourceProviderRegistrationV2(provider)
	registry := mcp.NewRegistry()
	if err := registry.RegisterResourceProvider(registration); err != nil {
		t.Fatal(err)
	}
	ctx := workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-a")
	page, err := registry.ListResourcesScoped(ctx, mcp.ResourceListRequest{ServerID: ServerID, Limit: 2})
	if err != nil || len(page.Resources) == 0 {
		t.Fatalf("list fixture = %#v, %v", page, err)
	}
	uri := page.Resources[0].URI
	readDenied := []context.Context{
		context.Background(),
		workflowResourceContext("tenant-b", "user-a", "session-a", "workflow-a"),
		workflowResourceContext("tenant-a", "user-b", "session-a", "workflow-a"),
		workflowResourceContext("tenant-a", "user-a", "session-b", "workflow-a"),
		workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-b"),
	}
	for _, denied := range readDenied {
		_, _, readErr := registry.ReadResource(denied, ServerID, uri)
		if !errors.Is(readErr, mcp.ErrResourceUnavailable) || readErr.Error() != mcp.ErrResourceUnavailable.Error() {
			t.Fatalf("scope read error = %v", readErr)
		}
	}
	for _, denied := range []context.Context{context.Background(), workflowResourceContext("tenant-b", "user-a", "session-a", "workflow-a"), workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-b")} {
		_, listErr := registry.ListResourcesScoped(denied, mcp.ResourceListRequest{ServerID: ServerID, Limit: 1})
		if !errors.Is(listErr, mcp.ErrResourceUnavailable) || listErr.Error() != mcp.ErrResourceUnavailable.Error() {
			t.Fatalf("scope list error = %v", listErr)
		}
	}
	spoofed := tooling.ToolExecutionContext{TenantID: "tenant-a", UserID: "user-b", SessionID: "session-a", Metadata: map[string]string{"workflowId": "workflow-a"}}
	if _, err := provider.List(ctx, mcp.ResourceListRequest{ServerID: ServerID, Limit: 1, Execution: spoofed}); !errors.Is(err, mcp.ErrResourceUnavailable) {
		t.Fatalf("spoofed projection error = %v", err)
	}
	if _, _, err := registry.ReadResource(ctx, ServerID, uri+"tampered"); !errors.Is(err, mcp.ErrResourceUnavailable) {
		t.Fatalf("tamper error = %v", err)
	}
	if _, err := registry.ListResourcesScoped(ctx, mcp.ResourceListRequest{ServerID: ServerID, Cursor: "tampered", Limit: 1}); !errors.Is(err, mcp.ErrResourceUnavailable) {
		t.Fatalf("cursor tamper error = %v", err)
	}
}

func TestWorkflowResourceDescriptorV2SurvivesRestartAndExpires(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	key := workflowResourceTestKey()
	first, err := NewWorkflowResourcesV2WithKey(app, key)
	if err != nil {
		t.Fatal(err)
	}
	base := time.Unix(1_800_000_000, 0)
	first.now = func() time.Time { return base }
	ref, err := first.BindContext(WorkflowContextResourceV2{Key: contract.WorkflowKey{TenantID: "tenant-a", WorkflowID: "workflow-a"}, ActorID: "user-a", SessionID: "session-a", Revision: "revision-a"})
	if err != nil {
		t.Fatal(err)
	}
	restarted, _ := NewWorkflowResourcesV2WithKey(app, key)
	restarted.now = func() time.Time { return base.Add(time.Minute) }
	if _, err := restarted.Read(workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-a"), ref.URI); err != nil {
		t.Fatalf("restart read = %v", err)
	}
	restarted.now = func() time.Time { return base.Add(workflowResourceTTL + time.Second) }
	if _, err := restarted.Read(workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-a"), ref.URI); !errors.Is(err, mcp.ErrResourceUnavailable) {
		t.Fatalf("expiry error = %v", err)
	}
	wrongKey := append([]byte(nil), key...)
	wrongKey[0] ^= 0xff
	other, _ := NewWorkflowResourcesV2WithKey(app, wrongKey)
	if _, err := other.Read(workflowResourceContext("tenant-a", "user-a", "session-a", "workflow-a"), ref.URI); !errors.Is(err, mcp.ErrResourceUnavailable) {
		t.Fatalf("wrong-key error = %v", err)
	}
}

func TestWorkflowResourceProviderRegistrationRejectsEphemeralSigner(t *testing.T) {
	app, _ := newWorkflowReadFixture(t)
	resources, _ := NewWorkflowResourcesV2(app)
	provider, _ := NewWorkflowResourceProviderV2(resources)
	if _, err := WorkflowResourceProviderRegistrationV2(provider); err == nil {
		t.Fatal("ephemeral signer must not be registered as a production provider")
	}
}

func workflowResourceContext(tenant, user, session, workflowID string) context.Context {
	return tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: tenant, UserID: user, SessionID: session, Metadata: map[string]string{"workflowId": workflowID}})
}

func workflowResourceTestKey() []byte { return []byte("0123456789abcdef0123456789abcdef") }

func allWorkflowResourceCapabilities(value mcp.ResourceProviderCapabilities) bool {
	raw, _ := json.Marshal(value)
	return value.List && value.Read && value.Pagination && value.RevisionAddressed && value.OpaqueURIs && len(raw) > 0
}
