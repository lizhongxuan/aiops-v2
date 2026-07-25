package mcp

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"aiops-v2/internal/tooling"
)

type scopedResourceProviderFixture struct {
	authorizeCalls int
	listCalls      int
	readCalls      int
	lastList       ResourceListRequest
}

func (*scopedResourceProviderFixture) Capabilities() ResourceProviderCapabilities {
	return ResourceProviderCapabilities{List: true, Read: true, Pagination: true, RevisionAddressed: true, OpaqueURIs: true}
}

func (p *scopedResourceProviderFixture) List(_ context.Context, request ResourceListRequest) (ResourcePage, error) {
	p.listCalls++
	p.lastList = request
	if request.Execution.TenantID != "tenant-a" || request.Execution.UserID != "user-a" || request.Execution.SessionID != "session-a" || request.Execution.Metadata["workflowId"] != "workflow-a" {
		return ResourcePage{}, fmt.Errorf("untrusted request projection")
	}
	return ResourcePage{Resources: []Resource{{URI: "workflow://opaque/token-a", Name: "context", MimeType: "application/json", Version: "revision-a"}}, NextCursor: "cursor-b"}, nil
}

func (p *scopedResourceProviderFixture) Read(_ context.Context, request ResourceReadRequest) (ResourceContent, error) {
	p.readCalls++
	return ResourceContent{URI: request.URI, MimeType: "application/json", Text: fmt.Sprintf(`{"read":%d}`, p.readCalls), Version: "revision-a"}, nil
}

func TestScopedResourceProviderFailsClosedBeforeListOrRead(t *testing.T) {
	registry := NewRegistry()
	provider := &scopedResourceProviderFixture{}
	registerScopedResourceFixture(t, registry, provider)

	_, listErr := registry.ListResourcesScoped(context.Background(), ResourceListRequest{ServerID: "runner"})
	_, _, readErr := registry.ReadResource(context.Background(), "runner", "workflow://opaque/token-a")
	if !errors.Is(listErr, ErrResourceUnavailable) || !errors.Is(readErr, ErrResourceUnavailable) || listErr.Error() != readErr.Error() {
		t.Fatalf("missing-scope errors = %v / %v", listErr, readErr)
	}
	if provider.authorizeCalls != 0 || provider.listCalls != 0 || provider.readCalls != 0 {
		t.Fatalf("provider invoked before trusted scope: %#v", provider)
	}

	wrong := scopedResourceContext("tenant-b", "user-a", "session-a", "workflow-a")
	_, wrongList := registry.ListResourcesScoped(wrong, ResourceListRequest{ServerID: "runner"})
	_, _, wrongRead := registry.ReadResource(wrong, "runner", "workflow://opaque/token-a")
	if !errors.Is(wrongList, ErrResourceUnavailable) || wrongList.Error() != wrongRead.Error() {
		t.Fatalf("cross-tenant errors = %v / %v", wrongList, wrongRead)
	}
	if provider.listCalls != 0 || provider.readCalls != 0 {
		t.Fatalf("provider body invoked after RBAC denial: %#v", provider)
	}
}

func TestScopedResourceProviderProjectsTrustedScopePaginationAndCapabilities(t *testing.T) {
	registry := NewRegistry()
	provider := &scopedResourceProviderFixture{}
	registerScopedResourceFixture(t, registry, provider)
	ctx := scopedResourceContext("tenant-a", "user-a", "session-a", "workflow-a")

	page, err := registry.ListResourcesScoped(ctx, ResourceListRequest{ServerID: "runner", Cursor: "cursor-a", Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Resources) != 1 || page.Resources[0].ServerID != "runner" || page.Resources[0].Version != "revision-a" || page.NextCursor != "cursor-b" || !page.Capabilities.Pagination || !page.Capabilities.OpaqueURIs {
		t.Fatalf("resource page = %#v", page)
	}
	if provider.lastList.Cursor != "cursor-a" || provider.lastList.Limit != 1 {
		t.Fatalf("provider pagination request = %#v", provider.lastList)
	}
	first, ok, err := registry.ReadResource(ctx, "runner", "workflow://opaque/token-a")
	if err != nil || !ok || first.Text != `{"read":1}` || first.Version != "revision-a" {
		t.Fatalf("first read = %#v, %v, %v", first, ok, err)
	}
	second, ok, err := registry.ReadResource(ctx, "runner", "workflow://opaque/token-a")
	if err != nil || !ok || second.Text != `{"read":2}` || provider.readCalls != 2 {
		t.Fatalf("second read = %#v, %v, %v; calls=%d", second, ok, err, provider.readCalls)
	}
}

func registerScopedResourceFixture(t *testing.T, registry *Registry, provider *scopedResourceProviderFixture) {
	t.Helper()
	err := registry.RegisterResourceProvider(ResourceProviderRegistration{ServerID: "runner", URIPrefix: "workflow://opaque/", Provider: provider, Authorize: func(_ context.Context, request ResourceAccessRequest) bool {
		provider.authorizeCalls++
		return request.Execution.TenantID == "tenant-a" && request.Execution.UserID == "user-a" && request.Execution.SessionID == "session-a" && request.Execution.Metadata["workflowId"] == "workflow-a"
	}})
	if err != nil {
		t.Fatal(err)
	}
}

func scopedResourceContext(tenant, user, session, workflow string) context.Context {
	return tooling.ContextWithToolExecution(context.Background(), tooling.ToolExecutionContext{TenantID: tenant, UserID: user, SessionID: session, Metadata: map[string]string{"workflowId": workflow}})
}
