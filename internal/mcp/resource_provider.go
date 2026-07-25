package mcp

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"aiops-v2/internal/tooling"
)

var ErrResourceUnavailable = errors.New("mcp resource unavailable")

type ResourceProviderCapabilities struct {
	List              bool `json:"list"`
	Read              bool `json:"read"`
	Pagination        bool `json:"pagination"`
	RevisionAddressed bool `json:"revisionAddressed"`
	OpaqueURIs        bool `json:"opaqueUris"`
}

type ResourceListRequest struct {
	ServerID  string                       `json:"serverId"`
	Cursor    string                       `json:"cursor,omitempty"`
	Limit     int                          `json:"limit,omitempty"`
	Execution tooling.ToolExecutionContext `json:"-"`
}

type ResourceReadRequest struct {
	ServerID  string                       `json:"serverId"`
	URI       string                       `json:"uri"`
	Execution tooling.ToolExecutionContext `json:"-"`
}

type ResourcePage struct {
	Resources    []Resource                   `json:"resources"`
	NextCursor   string                       `json:"nextCursor,omitempty"`
	Capabilities ResourceProviderCapabilities `json:"capabilities"`
}

type ResourceProvider interface {
	Capabilities() ResourceProviderCapabilities
	List(context.Context, ResourceListRequest) (ResourcePage, error)
	Read(context.Context, ResourceReadRequest) (ResourceContent, error)
}

type ResourceAccessOperation string

const (
	ResourceAccessList ResourceAccessOperation = "list"
	ResourceAccessRead ResourceAccessOperation = "read"
)

type ResourceAccessRequest struct {
	Operation ResourceAccessOperation      `json:"operation"`
	ServerID  string                       `json:"serverId"`
	URI       string                       `json:"uri,omitempty"`
	Execution tooling.ToolExecutionContext `json:"-"`
}

type ResourceAuthorizer func(context.Context, ResourceAccessRequest) bool

type ResourceProviderRegistration struct {
	ServerID  string
	URIPrefix string
	Provider  ResourceProvider
	Authorize ResourceAuthorizer
}

type registeredResourceProvider struct {
	uriPrefix    string
	provider     ResourceProvider
	authorize    ResourceAuthorizer
	capabilities ResourceProviderCapabilities
}

func (r *Registry) RegisterResourceProvider(registration ResourceProviderRegistration) error {
	if r == nil {
		return fmt.Errorf("mcp: registry is required")
	}
	serverID, prefix := strings.TrimSpace(registration.ServerID), strings.TrimSpace(registration.URIPrefix)
	if serverID == "" || prefix == "" || registration.Provider == nil || registration.Authorize == nil {
		return fmt.Errorf("mcp: scoped resource provider requires server, URI prefix, provider and authorizer")
	}
	capabilities := registration.Provider.Capabilities()
	if !capabilities.List && !capabilities.Read {
		return fmt.Errorf("mcp: resource provider must declare list or read capability")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.resourceProviders[serverID]; exists {
		return fmt.Errorf("mcp: resource provider already registered for server %q", serverID)
	}
	r.resourceProviders[serverID] = registeredResourceProvider{uriPrefix: prefix, provider: registration.Provider, authorize: registration.Authorize, capabilities: capabilities}
	return nil
}

func (r *Registry) ListResourcesScoped(ctx context.Context, request ResourceListRequest) (ResourcePage, error) {
	request.ServerID = strings.TrimSpace(request.ServerID)
	request.Cursor = strings.TrimSpace(request.Cursor)
	request.Limit = normalizeResourceLimit(request.Limit)
	if request.ServerID == "" && r.hasScopedResourceProviders() {
		return ResourcePage{}, ErrResourceUnavailable
	}
	registered, enabled := r.scopedResourceProvider(request.ServerID)
	if registered.provider == nil {
		return ResourcePage{Resources: r.ListResources(request.ServerID), Capabilities: ResourceProviderCapabilities{List: true, Read: true}}, nil
	}
	if !enabled || !registered.capabilities.List {
		return ResourcePage{}, ErrResourceUnavailable
	}
	execution, err := trustedResourceExecution(ctx)
	if err != nil {
		return ResourcePage{}, ErrResourceUnavailable
	}
	access := ResourceAccessRequest{Operation: ResourceAccessList, ServerID: request.ServerID, URI: registered.uriPrefix, Execution: execution}
	if !registered.authorize(ctx, access) {
		return ResourcePage{}, ErrResourceUnavailable
	}
	request.Execution = execution
	page, err := registered.provider.List(ctx, request)
	if err != nil {
		return ResourcePage{}, ErrResourceUnavailable
	}
	capabilities := registered.capabilities
	if (!capabilities.Pagination && page.NextCursor != "") || len(page.Resources) > request.Limit {
		return ResourcePage{}, ErrResourceUnavailable
	}
	for index := range page.Resources {
		resource := &page.Resources[index]
		if !strings.HasPrefix(resource.URI, registered.uriPrefix) || (capabilities.RevisionAddressed && strings.TrimSpace(resource.Version) == "") {
			return ResourcePage{}, ErrResourceUnavailable
		}
		resource.ServerID = request.ServerID
	}
	page.Capabilities = capabilities
	return cloneResourcePage(page), nil
}

func (r *Registry) readScopedResource(ctx context.Context, serverID, uri string) (ResourceContent, bool, error) {
	registered, enabled := r.scopedResourceProvider(serverID)
	if registered.provider == nil || !strings.HasPrefix(uri, registered.uriPrefix) {
		return ResourceContent{}, false, nil
	}
	capabilities := registered.capabilities
	if !enabled || !capabilities.Read {
		return ResourceContent{}, true, ErrResourceUnavailable
	}
	execution, err := trustedResourceExecution(ctx)
	if err != nil {
		return ResourceContent{}, true, ErrResourceUnavailable
	}
	access := ResourceAccessRequest{Operation: ResourceAccessRead, ServerID: serverID, URI: uri, Execution: execution}
	if !registered.authorize(ctx, access) {
		return ResourceContent{}, true, ErrResourceUnavailable
	}
	content, err := registered.provider.Read(ctx, ResourceReadRequest{ServerID: serverID, URI: uri, Execution: execution})
	if err != nil || strings.TrimSpace(content.URI) != uri || (capabilities.RevisionAddressed && strings.TrimSpace(content.Version) == "") {
		return ResourceContent{}, true, ErrResourceUnavailable
	}
	content.ServerID, content.URI = serverID, uri
	content.MimeType = strings.TrimSpace(content.MimeType)
	content.Blob = append([]byte(nil), content.Blob...)
	content.Bytes = int64(len(content.Text) + len(content.Blob))
	if content.Digest == "" {
		content.Digest = resourceContentDigest(content)
	}
	return content, true, nil
}

func (r *Registry) scopedResourceProvider(serverID string) (registeredResourceProvider, bool) {
	if r == nil {
		return registeredResourceProvider{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	serverID = strings.TrimSpace(serverID)
	return r.resourceProviders[serverID], !r.serverState[serverID]
}

func (r *Registry) hasScopedResourceProviders() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return len(r.resourceProviders) > 0
}

func trustedResourceExecution(ctx context.Context) (tooling.ToolExecutionContext, error) {
	execution, ok := tooling.ToolExecutionContextFrom(ctx)
	rawWorkflowID := execution.Metadata["workflowId"]
	workflowID := strings.TrimSpace(rawWorkflowID)
	if !ok || execution.TenantID == "" || execution.TenantID != strings.TrimSpace(execution.TenantID) || execution.UserID == "" || execution.UserID != strings.TrimSpace(execution.UserID) || execution.SessionID == "" || execution.SessionID != strings.TrimSpace(execution.SessionID) || workflowID == "" || rawWorkflowID != workflowID || workflowID == "." || workflowID == ".." || strings.ContainsAny(workflowID, "/\\\x00") {
		return tooling.ToolExecutionContext{}, ErrResourceUnavailable
	}
	execution.OriginalInput, execution.SanitizedInput = nil, nil
	execution.Metadata = cloneResourceMetadata(execution.Metadata)
	execution.Metadata["workflowId"] = workflowID
	return execution, nil
}

func cloneResourceMetadata(source map[string]string) map[string]string {
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func normalizeResourceLimit(limit int) int {
	if limit <= 0 {
		return 50
	}
	if limit > 200 {
		return 200
	}
	return limit
}

func cloneResourcePage(page ResourcePage) ResourcePage {
	page.Resources = append([]Resource(nil), page.Resources...)
	for index := range page.Resources {
		page.Resources[index] = cloneResource(page.Resources[index])
	}
	return page
}
