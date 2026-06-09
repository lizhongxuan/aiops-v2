package runtimekernel

import (
	"context"
	"strings"
	"testing"

	"aiops-v2/internal/integrations/mcpresources"
)

func TestMCPResourceArtifactSinkWritesContextArtifactRepository(t *testing.T) {
	repo := NewMemoryContextArtifactRepository()
	sink := NewMCPResourceArtifactSink(repo, ContextArtifactSource{SessionID: "session-1"})
	content := []byte("%PDF synthetic report")

	saved, err := sink.SaveMCPResourceArtifact(context.Background(), mcpresources.MCPResourceArtifactWrite{
		ServerID:    "docs",
		URI:         "resource://report.pdf",
		ContentType: "application/pdf",
		Content:     content,
	})
	if err != nil {
		t.Fatalf("SaveMCPResourceArtifact() error = %v", err)
	}
	if !strings.HasPrefix(saved.Ref, "store://artifacts/mcp-resource-") {
		t.Fatalf("saved ref = %q, want store artifact ref", saved.Ref)
	}

	id := strings.TrimPrefix(saved.Ref, "store://artifacts/")
	artifact, raw, err := repo.GetContextArtifact(id)
	if err != nil {
		t.Fatalf("GetContextArtifact(%q) error = %v", id, err)
	}
	if artifact.Kind != "mcp_resource" {
		t.Fatalf("artifact kind = %q, want mcp_resource", artifact.Kind)
	}
	if artifact.Source.ToolName != "read_mcp_resource" || artifact.Source.SessionID != "session-1" {
		t.Fatalf("artifact source = %#v", artifact.Source)
	}
	if string(raw) != string(content) {
		t.Fatalf("stored content = %q, want original bytes", string(raw))
	}
	if saved.Digest != artifact.Digest || saved.Bytes != artifact.Bytes || saved.ContentType != artifact.ContentType {
		t.Fatalf("saved metadata = %#v, artifact = %#v", saved, artifact)
	}
}
