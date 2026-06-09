package runtimekernel

import (
	"context"
	"fmt"
	"strings"

	"aiops-v2/internal/integrations/mcpresources"
)

type MCPResourceArtifactSink struct {
	repository ContextArtifactRepository
	source     ContextArtifactSource
}

func NewMCPResourceArtifactSink(repository ContextArtifactRepository, source ContextArtifactSource) MCPResourceArtifactSink {
	return MCPResourceArtifactSink{
		repository: repository,
		source:     source,
	}
}

func (s MCPResourceArtifactSink) SaveMCPResourceArtifact(_ context.Context, write mcpresources.MCPResourceArtifactWrite) (mcpresources.MCPResourceArtifact, error) {
	if s.repository == nil {
		return mcpresources.MCPResourceArtifact{}, fmt.Errorf("mcp resource artifact repository is nil")
	}
	content := append([]byte(nil), write.Content...)
	source := s.source
	if strings.TrimSpace(source.ToolName) == "" {
		source.ToolName = "read_mcp_resource"
	}
	artifact, err := s.repository.SaveContextArtifact(ContextArtifactWrite{
		ID:          mcpResourceContextArtifactID(write.Digest, content),
		Kind:        "mcp_resource",
		ContentType: write.ContentType,
		Content:     content,
		Summary:     "MCP resource content stored outside the prompt.",
		Source:      source,
	})
	if err != nil {
		return mcpresources.MCPResourceArtifact{}, err
	}
	return mcpresources.MCPResourceArtifact{
		Ref:         artifact.URI,
		ContentType: artifact.ContentType,
		Extension:   artifact.Extension,
		Digest:      artifact.Digest,
		Bytes:       artifact.Bytes,
	}, nil
}

func mcpResourceContextArtifactID(digest string, content []byte) string {
	digest = strings.TrimPrefix(strings.TrimSpace(digest), "sha256:")
	if digest == "" {
		digest = strings.TrimPrefix(contextArtifactDigest(content), "sha256:")
	}
	if len(digest) > 16 {
		digest = digest[:16]
	}
	if digest == "" {
		digest = "empty"
	}
	return "mcp-resource-" + digest
}
