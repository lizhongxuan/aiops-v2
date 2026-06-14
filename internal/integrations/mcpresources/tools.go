package mcpresources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/resourceio"
	"aiops-v2/internal/tooling"
)

const schemaVersion = "aiops.mcp_resource/v1"

type ReadToolOptions struct {
	ArtifactSink MCPResourceArtifactSink
}

type MCPResourceArtifactSink interface {
	SaveMCPResourceArtifact(context.Context, MCPResourceArtifactWrite) (MCPResourceArtifact, error)
}

type MCPResourceArtifactWrite struct {
	ServerID    string
	URI         string
	ContentType string
	Content     []byte
	Digest      string
	Bytes       int64
}

type MCPResourceArtifact struct {
	Ref         string
	ContentType string
	Extension   string
	Digest      string
	Bytes       int64
}

func NewListTool(registry *mcp.Registry) tooling.Tool {
	return newTool("list_mcp_resources", "List readable MCP resources exposed by connected servers", listSchema, func(_ context.Context, input json.RawMessage) (any, []tooling.ResultReference, error) {
		var req struct {
			Server string `json:"server,omitempty"`
		}
		if len(input) > 0 {
			if err := json.Unmarshal(input, &req); err != nil {
				return nil, nil, err
			}
		}
		resources := registry.ListResources(req.Server)
		return envelope("list_mcp_resources", map[string]any{
			"server":    strings.TrimSpace(req.Server),
			"resources": resources,
		}), nil, nil
	})
}

func NewReadTool(registry *mcp.Registry) tooling.Tool {
	return NewReadToolWithOptions(registry, ReadToolOptions{})
}

func NewReadToolWithOptions(registry *mcp.Registry, opts ReadToolOptions) tooling.Tool {
	return newTool("read_mcp_resource", "Read the content of an MCP resource by server and URI", readSchema, func(ctx context.Context, input json.RawMessage) (any, []tooling.ResultReference, error) {
		var req struct {
			Server string           `json:"server"`
			URI    string           `json:"uri"`
			Range  resourceio.Range `json:"range,omitempty"`
			Offset int64            `json:"offset,omitempty"`
			Limit  int              `json:"limit,omitempty"`
			Query  string           `json:"query,omitempty"`
			Page   int              `json:"page,omitempty"`
			Format string           `json:"format,omitempty"`
		}
		if err := json.Unmarshal(input, &req); err != nil {
			return nil, nil, err
		}
		content, ok, err := registry.ReadResource(ctx, req.Server, req.URI)
		if err != nil {
			return nil, nil, err
		}
		if !ok {
			return nil, nil, fmt.Errorf("mcp resource %q not found on server %q", strings.TrimSpace(req.URI), strings.TrimSpace(req.Server))
		}
		readReq := resourceio.ReadRequest{
			URI:    content.URI,
			Range:  req.Range,
			Offset: req.Offset,
			Limit:  req.Limit,
			Query:  req.Query,
			Page:   req.Page,
			Format: req.Format,
		}
		contentType := content.MimeType
		var read resourceio.ReadResult
		var rawContent []byte
		if content.Text != "" {
			rawContent = []byte(content.Text)
			detectedContentType := resourceio.DetectContentType(content.URI, []byte(content.Text), contentType)
			if resourceio.IsTextContentType(detectedContentType) {
				read = resourceio.ReadText(content.Text, readReq)
				read.Ref.ContentType = detectedContentType
			} else {
				read = resourceio.ReadBytes([]byte(content.Text), readReq, detectedContentType)
			}
			read.Ref.Digest = firstNonBlankString(content.Digest, read.Ref.Digest)
			read.Ref.Bytes = firstPositiveInt64(content.Bytes, read.Ref.Bytes)
		} else {
			rawContent = append([]byte(nil), content.Blob...)
			read = resourceio.ReadBytes(content.Blob, readReq, contentType)
			read.Ref.Digest = firstNonBlankString(content.Digest, read.Ref.Digest)
			read.Ref.Bytes = firstPositiveInt64(content.Bytes, read.Ref.Bytes)
		}
		ref := tooling.ResultReference{
			Kind:        tooling.ResultReferenceKindMCPResource,
			URI:         content.URI,
			Title:       content.URI,
			ContentType: read.Ref.ContentType,
			Digest:      read.Ref.Digest,
			Bytes:       read.Ref.Bytes,
			Range:       read.Ref.Range,
		}
		artifactRef := ""
		if read.MetadataOnly {
			savedArtifact, err := saveMCPResourceArtifact(ctx, opts.ArtifactSink, content, read.Ref, rawContent)
			if err != nil {
				return nil, nil, err
			}
			artifactRef = strings.TrimSpace(savedArtifact.Ref)
			if artifactRef == "" {
				artifactRef = mcpResourceArtifactRef(content.URI, read.Ref.ContentType, read.Ref.Digest)
			}
			read.Ref.ContentType = firstNonBlankString(savedArtifact.ContentType, read.Ref.ContentType)
			read.Ref.Digest = firstNonBlankString(savedArtifact.Digest, read.Ref.Digest)
			read.Ref.Bytes = firstPositiveInt64(savedArtifact.Bytes, read.Ref.Bytes)
			ref.FilePath = artifactRef
			ref.ContentType = read.Ref.ContentType
			ref.Digest = read.Ref.Digest
			ref.Bytes = read.Ref.Bytes
		}
		return envelope("read_mcp_resource", map[string]any{
			"server":       content.ServerID,
			"uri":          content.URI,
			"contentType":  read.Ref.ContentType,
			"text":         read.Content,
			"matches":      read.Matches,
			"metadataOnly": read.MetadataOnly,
			"truncated":    read.Truncated,
			"offset":       read.Offset,
			"limit":        read.Limit,
			"page":         read.Page,
			"digest":       read.Ref.Digest,
			"bytes":        read.Ref.Bytes,
			"refs": []map[string]any{{
				"kind":        string(ref.Kind),
				"uri":         content.URI,
				"title":       content.URI,
				"contentType": ref.ContentType,
				"artifactRef": artifactRef,
				"digest":      ref.Digest,
				"bytes":       ref.Bytes,
				"range":       read.Ref.Range,
			}},
		}), []tooling.ResultReference{ref}, nil
	})
}

func saveMCPResourceArtifact(ctx context.Context, sink MCPResourceArtifactSink, content mcp.ResourceContent, ref resourceio.Reference, raw []byte) (MCPResourceArtifact, error) {
	if sink == nil || len(raw) == 0 {
		return MCPResourceArtifact{}, nil
	}
	return sink.SaveMCPResourceArtifact(ctx, MCPResourceArtifactWrite{
		ServerID:    content.ServerID,
		URI:         content.URI,
		ContentType: ref.ContentType,
		Content:     append([]byte(nil), raw...),
		Digest:      ref.Digest,
		Bytes:       ref.Bytes,
	})
}

func newTool(name, description string, schema json.RawMessage, execute func(context.Context, json.RawMessage) (any, []tooling.ResultReference, error)) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        name,
			Origin:      tooling.ToolOriginBuiltin,
			Description: description,
			Layer:       tooling.ToolLayerCore,
			Pack:        "mcp_resource",
			AlwaysLoad:  true,
			Triggers:    []string{"MCP resource", "resource URI", "mcp://"},
			RiskLevel:   tooling.ToolRiskLow,
			Discovery: tooling.ToolDiscoveryMetadata{
				DiscoveryGroup: "mcp_resource",
				DiscoveryTags:  []string{"mcp", "resource", "uri"},
				ResourceTypes:  []string{"mcp_resource", "resource"},
				OperationKinds: []string{"list", "read"},
			},
		},
		Visibility:          tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"chat", "inspect", "plan", "execute"}},
		InputSchemaData:     schema,
		OutputSchemaData:    outputSchema,
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			payload, refs, err := execute(ctx, input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			data, err := json.Marshal(payload)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content:    string(data),
				References: refs,
				Display: &tooling.ToolDisplayPayload{
					Type:  "mcp_resource",
					Title: name,
					Data:  data,
				},
			}, nil
		},
	}
}

func envelope(tool string, data map[string]any) map[string]any {
	return map[string]any{
		"schemaVersion": schemaVersion,
		"tool":          tool,
		"status":        "ok",
		"data":          data,
	}
}

var listSchema = json.RawMessage(`{"type":"object","properties":{"server":{"type":"string","description":"Optional MCP server id"}}}`)

var readSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"server":{"type":"string","description":"MCP server id"},
		"uri":{"type":"string","description":"Resource URI"},
		"offset":{"type":"integer","description":"Byte offset for bounded text reads"},
		"limit":{"type":"integer","description":"Maximum bytes to return"},
		"page":{"type":"integer","description":"One-based page number when offset is omitted"},
		"query":{"type":"string","description":"Case-insensitive text query for bounded match windows"},
		"format":{"type":"string","enum":["text","json","metadata"],"description":"Return format; metadata omits payload content"},
		"range":{"type":"object","properties":{
			"offset":{"type":"integer"},
			"limit":{"type":"integer"},
			"page":{"type":"integer"},
			"query":{"type":"string"},
			"format":{"type":"string"}
		}}
	},
	"required":["server","uri"]
}`)

var outputSchema = json.RawMessage(`{
	"type":"object",
	"properties":{
		"schemaVersion":{"type":"string"},
		"tool":{"type":"string"},
		"status":{"type":"string"},
		"data":{"type":"object"}
	},
	"required":["schemaVersion","tool","status"]
}`)

func firstNonBlankString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func firstPositiveInt64(values ...int64) int64 {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func mcpResourceArtifactRef(uri, contentType, digest string) string {
	digest = strings.TrimSpace(digest)
	if digest == "" {
		return ""
	}
	suffix := strings.TrimPrefix(digest, "sha256:")
	if len(suffix) > 16 {
		suffix = suffix[:16]
	}
	return "store://artifacts/mcp-resource-" + suffix + artifactExtension(contentType, uri)
}

func artifactExtension(contentType, uri string) string {
	contentType = strings.ToLower(strings.TrimSpace(contentType))
	switch {
	case strings.Contains(contentType, "pdf"):
		return ".pdf"
	case strings.Contains(contentType, "png"):
		return ".png"
	case strings.Contains(contentType, "jpeg"), strings.Contains(contentType, "jpg"):
		return ".jpg"
	case strings.Contains(contentType, "json"):
		return ".json"
	case strings.Contains(contentType, "zip"):
		return ".zip"
	}
	if idx := strings.LastIndex(uri, "."); idx >= 0 && idx < len(uri)-1 && len(uri)-idx <= 8 {
		return uri[idx:]
	}
	return ".bin"
}
