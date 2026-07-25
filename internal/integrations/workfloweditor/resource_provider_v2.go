package workfloweditor

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
	"runner/server/service"
	contract "runner/workflow/edit"
)

type WorkflowResourceProviderV2 struct {
	resources *WorkflowResourcesV2
}

type workflowResourceListCursorV2 struct {
	SchemaVersion  string               `json:"schemaVersion"`
	Kind           string               `json:"kind"`
	ExpiresAt      int64                `json:"expiresAt"`
	Key            contract.WorkflowKey `json:"workflowKey"`
	ActorID        string               `json:"actorId"`
	SessionID      string               `json:"sessionId"`
	Offset         int                  `json:"offset"`
	Revision       string               `json:"revision"`
	CatalogVersion string               `json:"catalogVersion"`
}

func NewWorkflowResourceProviderV2(resources *WorkflowResourcesV2) (*WorkflowResourceProviderV2, error) {
	if resources == nil || resources.app == nil {
		return nil, fmt.Errorf("workflow resource provider: resources are required")
	}
	return &WorkflowResourceProviderV2{resources: resources}, nil
}

func (p *WorkflowResourceProviderV2) Capabilities() mcp.ResourceProviderCapabilities {
	return mcp.ResourceProviderCapabilities{List: true, Read: true, Pagination: true, RevisionAddressed: true, OpaqueURIs: true}
}

func (p *WorkflowResourceProviderV2) Authorize(ctx context.Context, request mcp.ResourceAccessRequest) bool {
	scope, err := trustedWorkflowResourceScopeV2(ctx, request.Execution)
	if err != nil || p == nil || p.resources == nil || request.ServerID != ServerID {
		return false
	}
	switch request.Operation {
	case mcp.ResourceAccessList:
		return request.URI == workflowResourcePrefixV2
	case mcp.ResourceAccessRead:
		item, err := p.resources.openURI(request.URI)
		return err == nil && workflowResourceDescriptorScopeMatchesV2(item, scope)
	default:
		return false
	}
}

func (p *WorkflowResourceProviderV2) List(ctx context.Context, request mcp.ResourceListRequest) (mcp.ResourcePage, error) {
	scope, err := trustedWorkflowResourceScopeV2(ctx, request.Execution)
	if err != nil || p == nil || p.resources == nil || request.ServerID != ServerID {
		return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
	}
	contextResult, err := p.resources.app.GetContext(ctx, scope.Key, service.WorkflowContextQuery{View: service.WorkflowContextSummaryView, Limit: 1})
	if err != nil {
		return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
	}
	includeSchema := false
	catalogResult, err := p.resources.app.GetActionCatalog(ctx, scope.Key, service.WorkflowActionCatalogQuery{IncludeSchema: &includeSchema, Limit: 1})
	if err != nil {
		return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
	}
	offset, err := p.listOffset(request.Cursor, scope, contextResult.Revision.ResourceVersion, catalogResult.CatalogDigest)
	if err != nil {
		return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
	}
	refs, err := p.bindListResources(scope, contextResult.Revision.ResourceVersion, catalogResult.CatalogDigest)
	if err != nil {
		return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
	}
	limit := request.Limit
	if limit <= 0 {
		limit = 50
	} else if limit > 200 {
		limit = 200
	}
	if offset < 0 || offset >= len(refs) && offset != 0 {
		return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
	}
	end := offset + limit
	if end > len(refs) {
		end = len(refs)
	}
	page := mcp.ResourcePage{Capabilities: p.Capabilities(), Resources: append([]mcp.Resource(nil), refs[offset:end]...)}
	if end < len(refs) {
		cursor := workflowResourceListCursorV2{SchemaVersion: workflowResourceSchemaVersionV2, Kind: "list_cursor", ExpiresAt: p.resources.now().Add(p.resources.ttl).Unix(), Key: scope.Key, ActorID: scope.UserID, SessionID: scope.SessionID, Offset: end, Revision: contextResult.Revision.ResourceVersion, CatalogVersion: catalogResult.CatalogDigest}
		page.NextCursor, err = p.resources.seal(cursor)
		if err != nil {
			return mcp.ResourcePage{}, mcp.ErrResourceUnavailable
		}
	}
	return page, nil
}

func (p *WorkflowResourceProviderV2) Read(ctx context.Context, request mcp.ResourceReadRequest) (mcp.ResourceContent, error) {
	scope, err := trustedWorkflowResourceScopeV2(ctx, request.Execution)
	if err != nil || p == nil || p.resources == nil || request.ServerID != ServerID {
		return mcp.ResourceContent{}, mcp.ErrResourceUnavailable
	}
	item, err := p.resources.openURI(request.URI)
	if err != nil || !workflowResourceDescriptorScopeMatchesV2(item, scope) {
		return mcp.ResourceContent{}, mcp.ErrResourceUnavailable
	}
	content, err := p.resources.Read(ctx, request.URI)
	if err != nil {
		return mcp.ResourceContent{}, mcp.ErrResourceUnavailable
	}
	return mcp.ResourceContent{URI: request.URI, MimeType: content.ContentType, Text: string(content.Content), Digest: content.Digest, Version: content.Version}, nil
}

func WorkflowResourceProviderRegistrationV2(provider *WorkflowResourceProviderV2) (mcp.ResourceProviderRegistration, error) {
	if provider == nil || provider.resources == nil || provider.resources.ephemeral {
		return mcp.ResourceProviderRegistration{}, fmt.Errorf("workflow resource provider: stable signer is required")
	}
	return mcp.ResourceProviderRegistration{ServerID: ServerID, URIPrefix: workflowResourcePrefixV2, Provider: provider, Authorize: provider.Authorize}, nil
}

func (p *WorkflowResourceProviderV2) bindListResources(scope trustedWorkflowScopeV2, revision, catalogVersion string) ([]mcp.Resource, error) {
	contextRef, err := p.resources.BindContext(WorkflowContextResourceV2{Key: scope.Key, ActorID: scope.UserID, SessionID: scope.SessionID, Revision: revision, Query: service.WorkflowContextQuery{View: service.WorkflowContextFullView, Limit: 200, BudgetBytes: 256 << 10}})
	if err != nil {
		return nil, err
	}
	catalogRef, err := p.resources.BindCatalog(WorkflowCatalogResourceV2{Key: scope.Key, ActorID: scope.UserID, SessionID: scope.SessionID, CatalogVersion: catalogVersion, Request: contract.ActionCatalogRequest{SchemaVersion: contract.SchemaVersion, WorkflowKey: scope.Key, IncludeSchema: true, IncludeExamples: true, Limit: 200}})
	if err != nil {
		return nil, err
	}
	return []mcp.Resource{
		{URI: contextRef.URI, Name: "workflow_context", Description: contextRef.Summary, MimeType: contextRef.ContentType, Version: contextRef.Version},
		{URI: catalogRef.URI, Name: "workflow_action_catalog", Description: catalogRef.Summary, MimeType: catalogRef.ContentType, Version: catalogRef.Version},
	}, nil
}

func (p *WorkflowResourceProviderV2) listOffset(raw string, scope trustedWorkflowScopeV2, revision, catalogVersion string) (int, error) {
	if raw == "" {
		return 0, nil
	}
	var cursor workflowResourceListCursorV2
	if err := p.resources.openToken(raw, &cursor); err != nil || cursor.SchemaVersion != workflowResourceSchemaVersionV2 || cursor.Kind != "list_cursor" || cursor.ExpiresAt <= p.resources.now().Unix() || cursor.Key != scope.Key || cursor.ActorID != scope.UserID || cursor.SessionID != scope.SessionID || cursor.Revision != revision || cursor.CatalogVersion != catalogVersion || cursor.Offset <= 0 {
		return 0, mcp.ErrResourceUnavailable
	}
	return cursor.Offset, nil
}

func trustedWorkflowResourceScopeV2(ctx context.Context, projected tooling.ToolExecutionContext) (trustedWorkflowScopeV2, error) {
	actual, ok := tooling.ToolExecutionContextFrom(ctx)
	workflowID := strings.TrimSpace(actual.Metadata["workflowId"])
	key := contract.WorkflowKey{TenantID: actual.TenantID, WorkflowID: workflowID}
	projectedWorkflowID := strings.TrimSpace(projected.Metadata["workflowId"])
	if !ok || actual.TenantID == "" || actual.TenantID != strings.TrimSpace(actual.TenantID) || actual.UserID == "" || actual.UserID != strings.TrimSpace(actual.UserID) || actual.SessionID == "" || actual.SessionID != strings.TrimSpace(actual.SessionID) || actual.Metadata["workflowId"] != workflowID || key.Validate() != nil || projected.TenantID != actual.TenantID || projected.UserID != actual.UserID || projected.SessionID != actual.SessionID || projected.Metadata["workflowId"] != projectedWorkflowID || projectedWorkflowID != workflowID {
		return trustedWorkflowScopeV2{}, mcp.ErrResourceUnavailable
	}
	return trustedWorkflowScopeV2{Key: key, UserID: actual.UserID, SessionID: actual.SessionID}, nil
}

func workflowResourceDescriptorScopeMatchesV2(item workflowResourceDescriptorV2, scope trustedWorkflowScopeV2) bool {
	if item.Kind == "context" {
		return item.Context.Key == scope.Key && item.Context.ActorID == scope.UserID && item.Context.SessionID == scope.SessionID
	}
	return item.Kind == "catalog" && item.Catalog.Key == scope.Key && item.Catalog.ActorID == scope.UserID && item.Catalog.SessionID == scope.SessionID
}
