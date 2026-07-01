package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/resourceio"
	"aiops-v2/internal/tooling"
)

func TestContextArtifactReaderReadsOffsetLimit(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	artifact, err := store.SaveContextArtifact(ContextArtifactWrite{
		Kind:        "tool_result",
		URI:         "store://artifacts/text-1",
		ContentType: "text/plain",
		Content:     []byte("alpha\nbeta\ngamma\ndelta\n"),
		Source: ContextArtifactSource{
			ToolCallID: "call-1",
			TurnID:     "turn-1",
		},
	})
	if err != nil {
		t.Fatalf("SaveContextArtifact failed: %v", err)
	}

	reader := NewContextArtifactReader(ContextArtifactReaderOptions{
		Repository:   store,
		MaxReadBytes: 64,
	})
	result, err := reader.Read(ContextArtifactReadRequest{
		ID:     artifact.ID,
		Offset: 6,
		Limit:  4,
	})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if result.Content != "beta" {
		t.Fatalf("content = %q, want beta", result.Content)
	}
	if result.Artifact.Digest == "" || result.Artifact.Bytes != int64(len("alpha\nbeta\ngamma\ndelta\n")) {
		t.Fatalf("metadata = %#v", result.Artifact)
	}
	if result.Artifact.Source.ToolCallID != "call-1" || result.Artifact.Source.TurnID != "turn-1" {
		t.Fatalf("source = %#v", result.Artifact.Source)
	}
}

func TestContextArtifactReaderAcceptsResourceIORange(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	artifact, err := store.SaveContextArtifact(ContextArtifactWrite{
		Kind:        "tool_result",
		URI:         "store://artifacts/text-range",
		ContentType: "text/plain",
		Content:     []byte("alpha\nbeta\ngamma\n"),
	})
	if err != nil {
		t.Fatalf("SaveContextArtifact failed: %v", err)
	}

	reader := NewContextArtifactReader(ContextArtifactReaderOptions{Repository: store, MaxReadBytes: 64})
	result, err := reader.Read(ContextArtifactReadRequest{
		ID:    artifact.ID,
		Range: resourceio.Range{Offset: 6, Limit: 4},
	})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if result.Content != "beta" {
		t.Fatalf("content = %q, want beta", result.Content)
	}
	if result.Offset != 6 || result.Limit != 4 {
		t.Fatalf("range = offset %d limit %d, want offset 6 limit 4", result.Offset, result.Limit)
	}
}

func TestContextArtifactReaderQueriesTextAndMetadataOnly(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	artifact, err := store.SaveContextArtifact(ContextArtifactWrite{
		Kind:        "resource",
		URI:         "store://artifacts/text-2",
		ContentType: "text/markdown",
		Content:     []byte("first line\nneedle appears here\nlast line\n"),
	})
	if err != nil {
		t.Fatalf("SaveContextArtifact failed: %v", err)
	}
	reader := NewContextArtifactReader(ContextArtifactReaderOptions{Repository: store, MaxReadBytes: 80})

	result, err := reader.Read(ContextArtifactReadRequest{ID: artifact.ID, Query: "needle", Limit: 10})
	if err != nil {
		t.Fatalf("Read query failed: %v", err)
	}
	if len(result.Matches) != 1 {
		t.Fatalf("matches = %d, want 1", len(result.Matches))
	}
	if !strings.Contains(result.Matches[0].Content, "needle") || result.Matches[0].Offset <= 0 {
		t.Fatalf("match = %#v", result.Matches[0])
	}

	metadataOnly, err := reader.Read(ContextArtifactReadRequest{ID: artifact.ID, Format: "metadata"})
	if err != nil {
		t.Fatalf("Read metadata failed: %v", err)
	}
	if metadataOnly.Content != "" || len(metadataOnly.Matches) != 0 {
		t.Fatalf("metadata read leaked payload: %#v", metadataOnly)
	}
	if metadataOnly.Ref != artifact.URI {
		t.Fatalf("ref = %q, want %q", metadataOnly.Ref, artifact.URI)
	}
}

func TestContextArtifactReaderDoesNotEmitBinaryPayload(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	artifact, err := store.SaveContextArtifact(ContextArtifactWrite{
		Kind:        "generated",
		URI:         "store://artifacts/bin-1",
		ContentType: "application/octet-stream",
		Content:     []byte{0x00, 0xff, 0x88, 0x77, 0x42, 0x10},
	})
	if err != nil {
		t.Fatalf("SaveContextArtifact failed: %v", err)
	}

	reader := NewContextArtifactReader(ContextArtifactReaderOptions{Repository: store, MaxReadBytes: 64})
	result, err := reader.Read(ContextArtifactReadRequest{ID: artifact.ID, Limit: 64})
	if err != nil {
		t.Fatalf("Read failed: %v", err)
	}
	if result.Content != "" {
		t.Fatalf("binary payload leaked as content: %q", result.Content)
	}
	if result.Ref != artifact.URI {
		t.Fatalf("ref = %q, want %q", result.Ref, artifact.URI)
	}
	if result.Artifact.Extension == "" || result.Artifact.Digest == "" || result.Artifact.ContentSnippet == "" {
		t.Fatalf("binary metadata incomplete: %#v", result.Artifact)
	}
}

func TestContextArtifactReaderMapsToolResultSpillURI(t *testing.T) {
	spillRepo := newContextArtifactTestSpillRepo()
	spill := &tooling.ResultSpill{
		ID:          "spill-1",
		ToolCallID:  "call-spill",
		ToolName:    "generic.reader",
		SessionID:   "session-1",
		TurnID:      "turn-1",
		ContentType: "application/json",
		Summary:     "bounded summary",
		Content:     []byte(`{"alpha":1,"beta":2}`),
		Bytes:       int64(len(`{"alpha":1,"beta":2}`)),
		CreatedAt:   time.Unix(100, 0).UTC(),
	}
	if err := spillRepo.SaveToolResultSpill(spill); err != nil {
		t.Fatalf("SaveToolResultSpill failed: %v", err)
	}

	reader := NewContextArtifactReader(ContextArtifactReaderOptions{SpillRepository: spillRepo, MaxReadBytes: 64})
	result, err := reader.Read(ContextArtifactReadRequest{
		ID:    "store://tool-spills/spill-1",
		Query: "beta",
	})
	if err != nil {
		t.Fatalf("Read spill failed: %v", err)
	}
	if result.Artifact.URI != "store://tool-spills/spill-1" {
		t.Fatalf("uri = %q", result.Artifact.URI)
	}
	if result.Artifact.Source.ToolCallID != "call-spill" || result.Artifact.Source.TurnID != "turn-1" {
		t.Fatalf("source = %#v", result.Artifact.Source)
	}
	if len(result.Matches) != 1 || !strings.Contains(result.Matches[0].Content, "beta") {
		t.Fatalf("matches = %#v", result.Matches)
	}
}

func TestReadContextArtifactToolReturnsRangeReference(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	artifact, err := store.SaveContextArtifact(ContextArtifactWrite{
		ID:          "artifact-tool-ref",
		Kind:        "tool_result",
		URI:         "store://artifacts/artifact-tool-ref",
		ContentType: "text/plain",
		Content:     []byte("alpha\nbeta\ngamma\n"),
		Summary:     "generic artifact",
	})
	if err != nil {
		t.Fatalf("SaveContextArtifact failed: %v", err)
	}
	kernel := &RuntimeKernel{artifactRepo: store}
	tools := kernel.contextArtifactTools()
	if len(tools) != 1 {
		t.Fatalf("context artifact tools = %d, want 1", len(tools))
	}

	result, err := tools[0].Execute(context.Background(), json.RawMessage(`{"id":"artifact-tool-ref","offset":6,"limit":4}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	if len(result.References) != 1 {
		t.Fatalf("references = %d, want 1; content=%s", len(result.References), result.Content)
	}
	ref := result.References[0]
	if ref.Kind != tooling.ResultReferenceKindBlob {
		t.Fatalf("reference kind = %q, want blob", ref.Kind)
	}
	if ref.URI != artifact.URI || ref.Digest != artifact.Digest || ref.Bytes != artifact.Bytes || ref.ContentType != artifact.ContentType {
		t.Fatalf("reference = %#v, artifact = %#v", ref, artifact)
	}
	if ref.Range.Offset != 6 || ref.Range.Limit != 4 {
		t.Fatalf("reference range = %#v, want offset 6 limit 4", ref.Range)
	}
}

func TestCompileContextAddsReadContextArtifactOnlyWhenContextArtifactEnabled(t *testing.T) {
	store := NewMemoryContextArtifactRepository()
	registry := tooling.NewRegistry()
	if err := registry.Register(&tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "synthetic.core_read",
			Description: "synthetic core read",
			Layer:       tooling.ToolLayerCore,
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			return tooling.ToolResult{Content: "ok"}, nil
		},
	}); err != nil {
		t.Fatalf("Register core tool failed: %v", err)
	}
	kernel := &RuntimeKernel{
		tools:        &testMockToolAssemblySource{registry: registry},
		artifactRepo: store,
	}

	withoutArtifactState := kernel.compileContext(SessionTypeHost, ModeChat, nil)
	if contextArtifactTestHasTool(withoutArtifactState.AssembledTools, "read_context_artifact") {
		t.Fatalf("read_context_artifact should not be prompt-visible without context artifact state: %v", contextArtifactTestToolNames(withoutArtifactState.AssembledTools))
	}

	withArtifactState := kernel.compileContext(SessionTypeHost, ModeChat, map[string]string{
		"enableToolPack": "context_artifact",
	})
	if !contextArtifactTestHasTool(withArtifactState.AssembledTools, "read_context_artifact") {
		t.Fatalf("read_context_artifact missing when context artifact pack is enabled: %v", contextArtifactTestToolNames(withArtifactState.AssembledTools))
	}
	for _, toolDef := range withArtifactState.AssembledTools {
		if toolDef == nil || toolDef.Metadata().Name != "read_context_artifact" {
			continue
		}
		meta := toolDef.Metadata()
		if meta.Layer != tooling.ToolLayerConditional || meta.Pack != "context_artifact" {
			t.Fatalf("read_context_artifact metadata = %#v, want conditional context_artifact", meta)
		}
	}
}

func contextArtifactTestHasTool(tools []promptcompiler.Tool, name string) bool {
	for _, toolDef := range tools {
		if toolDef != nil && toolDef.Metadata().Name == name {
			return true
		}
	}
	return false
}

func contextArtifactTestToolNames(tools []promptcompiler.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, toolDef := range tools {
		if toolDef == nil {
			continue
		}
		names = append(names, toolDef.Metadata().Name)
	}
	return names
}

type contextArtifactTestSpillRepo struct {
	spills map[string]*tooling.ResultSpill
}

func newContextArtifactTestSpillRepo() *contextArtifactTestSpillRepo {
	return &contextArtifactTestSpillRepo{spills: map[string]*tooling.ResultSpill{}}
}

func (r *contextArtifactTestSpillRepo) GetToolResultSpill(id string) (*tooling.ResultSpill, error) {
	spill, ok := r.spills[id]
	if !ok {
		return nil, ErrContextArtifactNotFound
	}
	cp := *spill
	cp.Content = append([]byte(nil), spill.Content...)
	return &cp, nil
}

func (r *contextArtifactTestSpillRepo) ListToolResultSpills() ([]*tooling.ResultSpill, error) {
	out := make([]*tooling.ResultSpill, 0, len(r.spills))
	for _, spill := range r.spills {
		cp := *spill
		cp.Content = append([]byte(nil), spill.Content...)
		out = append(out, &cp)
	}
	return out, nil
}

func (r *contextArtifactTestSpillRepo) SaveToolResultSpill(spill *tooling.ResultSpill) error {
	cp := *spill
	cp.Content = append([]byte(nil), spill.Content...)
	r.spills[spill.ID] = &cp
	return nil
}

func (r *contextArtifactTestSpillRepo) DeleteToolResultSpill(id string) error {
	delete(r.spills, id)
	return nil
}
