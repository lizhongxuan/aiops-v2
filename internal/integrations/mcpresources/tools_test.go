package mcpresources

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/integrations/toolsearch"
	"aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

func TestListAndReadMCPResourcesTools(t *testing.T) {
	registry := mcp.NewRegistry()
	seedMCPResource(t, registry, "ops", "ops://manuals/redis", "Redis manual")

	listTool := NewListTool(registry)
	listResult, err := listTool.Execute(context.Background(), json.RawMessage(`{"server":"ops"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listResult.Content, "ops://manuals/redis") {
		t.Fatalf("list result = %s", listResult.Content)
	}

	readTool := NewReadTool(registry)
	readResult, err := readTool.Execute(context.Background(), json.RawMessage(`{"server":"ops","uri":"ops://manuals/redis"}`))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readResult.Content, "Redis manual") {
		t.Fatalf("read result = %s", readResult.Content)
	}
	if !strings.Contains(readResult.Content, `"digest"`) {
		t.Fatalf("read result = %s, want digest", readResult.Content)
	}
}

func TestReadMCPResourceSupportsOffsetLimit(t *testing.T) {
	registry := mcp.NewRegistry()
	seedMCPResource(t, registry, "srv", "resource://large", "alpha\nbeta\ngamma\n")

	payload, result := executeReadMCPResource(t, registry, `{"server":"srv","uri":"resource://large","offset":6,"limit":4}`)
	data := payload["data"].(map[string]any)
	if got := data["text"]; got != "beta" {
		t.Fatalf("text = %#v, want beta; content=%s", got, result.Content)
	}
	if got := data["truncated"]; got != true {
		t.Fatalf("truncated = %#v, want true", got)
	}
	refs := data["refs"].([]any)
	ref := refs[0].(map[string]any)
	rng := ref["range"].(map[string]any)
	if rng["offset"] != float64(6) || rng["limit"] != float64(4) {
		t.Fatalf("range = %#v, want offset 6 limit 4", rng)
	}
	if len(result.References) != 1 {
		t.Fatalf("tool references = %d, want 1", len(result.References))
	}
	if result.References[0].Range.Offset != 6 || result.References[0].Range.Limit != 4 {
		t.Fatalf("tool reference range = %#v", result.References[0].Range)
	}
}

func TestReadMCPResourceBinaryReturnsMetadataOnly(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.OnServerResources("srv", []mcp.Resource{{URI: "resource://binary", Name: "binary", MimeType: "application/octet-stream"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetResourceContent("srv", "resource://binary", mcp.ResourceContent{
		URI:      "resource://binary",
		MimeType: "application/octet-stream",
		Blob:     []byte{0x00, 0x01, 0x02},
	}); err != nil {
		t.Fatal(err)
	}

	payload, result := executeReadMCPResource(t, registry, `{"server":"srv","uri":"resource://binary","limit":10}`)
	data := payload["data"].(map[string]any)
	if got := data["text"]; got != nil && got != "" {
		t.Fatalf("text = %#v, want empty", got)
	}
	if got := data["blob"]; got != nil && got != "" {
		t.Fatalf("blob = %#v, want empty", got)
	}
	if got := data["metadataOnly"]; got != true {
		t.Fatalf("metadataOnly = %#v, want true", got)
	}
	if len(result.References) != 1 {
		t.Fatalf("tool references = %d, want 1", len(result.References))
	}
	if result.References[0].Digest == "" || result.References[0].Bytes != 3 {
		t.Fatalf("reference = %#v, want digest and bytes", result.References[0])
	}
	refs := data["refs"].([]any)
	ref := refs[0].(map[string]any)
	if ref["artifactRef"] == "" {
		t.Fatalf("ref = %#v, want artifactRef", ref)
	}
	if result.References[0].FilePath == "" {
		t.Fatalf("tool reference = %#v, want FilePath artifact ref", result.References[0])
	}
	if strings.Contains(result.Content, "AAEC") {
		t.Fatalf("tool result leaked base64 payload: %s", result.Content)
	}
}

type recordingArtifactSink struct {
	writes []MCPResourceArtifactWrite
}

func (s *recordingArtifactSink) SaveMCPResourceArtifact(_ context.Context, write MCPResourceArtifactWrite) (MCPResourceArtifact, error) {
	s.writes = append(s.writes, write)
	return MCPResourceArtifact{
		Ref:         "store://artifacts/saved-resource",
		ContentType: write.ContentType,
		Extension:   ".pdf",
		Digest:      "sha256:saved",
		Bytes:       int64(len(write.Content)),
	}, nil
}

func TestReadMCPResourceBlobSavesArtifactWhenSinkAvailable(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.OnServerResources("srv", []mcp.Resource{{URI: "resource://report.pdf", Name: "report", MimeType: "application/pdf"}}); err != nil {
		t.Fatal(err)
	}
	blob := []byte("%PDF synthetic report")
	if err := registry.SetResourceContent("srv", "resource://report.pdf", mcp.ResourceContent{
		URI:      "resource://report.pdf",
		MimeType: "application/pdf",
		Blob:     blob,
	}); err != nil {
		t.Fatal(err)
	}
	sink := &recordingArtifactSink{}

	readTool := NewReadToolWithOptions(registry, ReadToolOptions{ArtifactSink: sink})
	result, err := readTool.Execute(context.Background(), json.RawMessage(`{"server":"srv","uri":"resource://report.pdf"}`))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	if len(sink.writes) != 1 {
		t.Fatalf("artifact writes = %d, want 1", len(sink.writes))
	}
	if string(sink.writes[0].Content) != string(blob) {
		t.Fatalf("saved content = %q, want original blob", string(sink.writes[0].Content))
	}
	if !strings.Contains(result.Content, `"artifactRef":"store://artifacts/saved-resource"`) {
		t.Fatalf("result content = %s, want saved artifactRef", result.Content)
	}
	if strings.Contains(result.Content, string(blob)) {
		t.Fatalf("tool result leaked blob payload: %s", result.Content)
	}
}

func TestReadMCPResourceTextWithBinaryMIMEReturnsMetadataOnly(t *testing.T) {
	registry := mcp.NewRegistry()
	if err := registry.OnServerResources("srv", []mcp.Resource{{URI: "resource://binary-text", Name: "binary text", MimeType: "application/octet-stream"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetResourceContent("srv", "resource://binary-text", mcp.ResourceContent{
		URI:      "resource://binary-text",
		MimeType: "application/octet-stream",
		Text:     "binary-shaped text must not enter model content",
	}); err != nil {
		t.Fatal(err)
	}

	payload, result := executeReadMCPResource(t, registry, `{"server":"srv","uri":"resource://binary-text","limit":10}`)
	data := payload["data"].(map[string]any)
	if got := data["text"]; got != nil && got != "" {
		t.Fatalf("text = %#v, want empty", got)
	}
	if got := data["metadataOnly"]; got != true {
		t.Fatalf("metadataOnly = %#v, want true", got)
	}
	if strings.Contains(result.Content, "binary-shaped text") {
		t.Fatalf("tool result leaked binary MIME text: %s", result.Content)
	}
}

func TestReadMCPResourceDefaultBoundsLargeText(t *testing.T) {
	registry := mcp.NewRegistry()
	large := strings.Repeat("x", 5000)
	seedMCPResource(t, registry, "srv", "resource://large-default", large)

	payload, _ := executeReadMCPResource(t, registry, `{"server":"srv","uri":"resource://large-default"}`)
	data := payload["data"].(map[string]any)
	text, _ := data["text"].(string)
	if len(text) != 4096 {
		t.Fatalf("text length = %d, want default bounded 4096", len(text))
	}
	if got := data["truncated"]; got != true {
		t.Fatalf("truncated = %#v, want true", got)
	}
}

func TestRegisterBuiltinsAddsMCPResourceTools(t *testing.T) {
	base := tooling.NewRegistry()
	resources := mcp.NewRegistry()

	if err := RegisterBuiltins(base, resources); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	tools := base.AssembleTools("host", "inspect")
	for _, name := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if !hasTool(tools, name) {
			t.Fatalf("missing %s in default assembled tools: %v", name, mcpResourceToolNames(tools))
		}
		meta := toolByNameForMCPResourceTest(t, tools, name).Metadata()
		if meta.Layer != tooling.ToolLayerCore || !meta.AlwaysLoad || meta.DeferByDefault {
			t.Fatalf("%s metadata = layer:%q alwaysLoad:%v defer:%v, want core always-load", name, meta.Layer, meta.AlwaysLoad, meta.DeferByDefault)
		}
		if meta.EffectiveDiscovery().RequiresSelect {
			t.Fatalf("%s discovery = %+v, want initial callable tool", name, meta.EffectiveDiscovery())
		}
	}
	chatTools := base.AssembleTools("host", "chat")
	for _, name := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if !hasTool(chatTools, name) {
			t.Fatalf("missing %s in default chat tools: %v", name, mcpResourceToolNames(chatTools))
		}
	}
}

func TestMCPResourceToolsAreInitialAndSearchable(t *testing.T) {
	base := tooling.NewRegistry()
	resources := mcp.NewRegistry()
	if err := RegisterBuiltins(base, resources); err != nil {
		t.Fatalf("RegisterBuiltins() error = %v", err)
	}

	defaultNames := mcpResourceToolNames(base.AssembleToolsWithOptions("host", "inspect", tooling.AssembleOptions{}))
	for _, want := range []string{"list_mcp_resources", "read_mcp_resource"} {
		if !hasMCPResourceToolName(defaultNames, want) {
			t.Fatalf("default assembled tools = %v, want %s", defaultNames, want)
		}
	}

	search := toolsearch.NewToolSearchTool(base)
	result, err := search.Execute(context.Background(), json.RawMessage(`{"query":"mcp resource uri","limit":5}`))
	if err != nil {
		t.Fatalf("tool_search Execute() error = %v", err)
	}
	for _, want := range []string{`"kind":"tool"`, `"list_mcp_resources"`, `"read_mcp_resource"`} {
		if !strings.Contains(result.Content, want) {
			t.Fatalf("tool_search content missing %s: %s", want, result.Content)
		}
	}
}

func seedMCPResource(t *testing.T, registry *mcp.Registry, server, uri, text string) {
	t.Helper()
	if err := registry.OnServerResources(server, []mcp.Resource{{URI: uri, Name: "Redis manual", MimeType: "text/plain"}}); err != nil {
		t.Fatal(err)
	}
	if err := registry.SetResourceContent(server, uri, mcp.ResourceContent{URI: uri, MimeType: "text/plain", Text: text}); err != nil {
		t.Fatal(err)
	}
}

func executeReadMCPResource(t *testing.T, registry *mcp.Registry, input string) (map[string]any, tooling.ToolResult) {
	t.Helper()
	readTool := NewReadTool(registry)
	result, err := readTool.Execute(context.Background(), json.RawMessage(input))
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(result.Content), &payload); err != nil {
		t.Fatalf("unmarshal content %s: %v", result.Content, err)
	}
	return payload, result
}

func hasTool(tools []tooling.Tool, name string) bool {
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return true
		}
	}
	return false
}

func toolByNameForMCPResourceTest(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, tool := range tools {
		if tool.Metadata().Name == name {
			return tool
		}
	}
	t.Fatalf("missing tool %s", name)
	return nil
}

func mcpResourceToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, tool := range tools {
		names = append(names, tool.Metadata().Name)
	}
	return names
}

func hasMCPResourceToolName(names []string, want string) bool {
	for _, name := range names {
		if name == want {
			return true
		}
	}
	return false
}
