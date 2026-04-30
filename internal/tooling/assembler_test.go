package tooling

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

type dynamicToolProviderStub struct {
	tools []Tool
	token string
}

func (p dynamicToolProviderStub) DynamicTools() []Tool {
	return append([]Tool(nil), p.tools...)
}

func (p dynamicToolProviderStub) DynamicToolRefreshToken() string {
	return p.token
}

func TestAssemblerMergesRegistryExtraAndDynamicTools(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&mockTool{
		meta:        ToolMetadata{Name: "read_file", Description: "builtin"},
		enabled:     true,
		readOnly:    true,
		concurrency: true,
		description: "builtin",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	provider := dynamicToolProviderStub{tools: []Tool{&mockTool{
		meta:        ToolMetadata{Name: "service_metrics", Description: "dynamic"},
		enabled:     true,
		readOnly:    true,
		concurrency: true,
		description: "dynamic",
	}}}
	assembler := NewAssembler(registry, nil, provider)

	got := assembler.AssembleToolsWithOptions("host", "inspect", AssembleOptions{
		ExtraTools: []Tool{&mockTool{
			meta:        ToolMetadata{Name: "disk_usage", Description: "extra"},
			enabled:     true,
			readOnly:    true,
			concurrency: true,
			description: "extra",
		}},
	})

	if names := toolNamesForTest(got); len(names) != 3 || names[0] != "disk_usage" || names[1] != "read_file" || names[2] != "service_metrics" {
		t.Fatalf("assembled names = %#v, want sorted registry+extra+dynamic tools", names)
	}
	pool := assembler.AssembleToolPoolWithOptions("host", "inspect", AssembleOptions{})
	if len(pool) != 2 {
		t.Fatalf("AssembleToolPoolWithOptions() len = %d, want registry+dynamic tools", len(pool))
	}
}

func TestAssemblerFingerprintUsesProviderTokenAndToolShape(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(&mockTool{
		meta:        ToolMetadata{Name: "read_file", Description: "builtin", Aliases: []string{"rf", "read"}},
		enabled:     true,
		readOnly:    true,
		concurrency: true,
		description: "builtin",
	}); err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	assemblerA := NewAssembler(registry, dynamicToolProviderStub{token: "token-a"})
	assemblerB := NewAssembler(registry, dynamicToolProviderStub{token: "token-b"})

	fpA1 := assemblerA.StableToolFingerprint("host", "inspect", AssembleOptions{})
	fpA2 := assemblerA.RefreshToken("host", "inspect", AssembleOptions{})
	fpB := assemblerB.StableToolFingerprint("host", "inspect", AssembleOptions{})

	if fpA1 == "" || fpA1 != fpA2 {
		t.Fatalf("fingerprint/refresh token mismatch: %q vs %q", fpA1, fpA2)
	}
	if fpA1 == fpB {
		t.Fatal("provider refresh token change should change fingerprint")
	}
}

func TestAssemblerWithNilRegistryUsesExtraToolsAndNilAssemblerIsSafe(t *testing.T) {
	var nilAssembler *Assembler
	if got := nilAssembler.AssembleToolsWithOptions("host", "chat", AssembleOptions{}); got != nil {
		t.Fatalf("nil assembler tools = %#v, want nil", got)
	}
	if got := nilAssembler.StableToolFingerprint("host", "chat", AssembleOptions{}); got != "" {
		t.Fatalf("nil assembler fingerprint = %q, want empty", got)
	}

	extra := &mockTool{meta: ToolMetadata{Name: "read_file", Description: "extra"}, enabled: true, description: "extra"}
	assembler := NewAssembler(nil)
	got := assembler.AssembleToolsWithOptions("host", "chat", AssembleOptions{ExtraTools: []Tool{extra}})
	if len(got) != 1 || got[0].Metadata().Name != "read_file" {
		t.Fatalf("nil-registry assembler tools = %#v, want extra tool", got)
	}
}

func TestMetadataOverrideToolDelegatesAllBehaviorWithTransformedMetadata(t *testing.T) {
	registry := NewRegistry()
	base := &StaticTool{
		Meta:             ToolMetadata{Name: "base", Description: "base desc"},
		InputSchemaData:  json.RawMessage(`{"type":"object"}`),
		OutputSchemaData: json.RawMessage(`{"type":"string"}`),
		Visibility:       Visibility{SessionTypes: []string{"host"}, Modes: []string{"inspect"}},
		ReadOnlyFunc:     func(json.RawMessage) bool { return true },
		ConcurrencySafeFunc: func(json.RawMessage) bool {
			return true
		},
		CheckPermissionsFunc: func(context.Context, json.RawMessage) PermissionDecision {
			return PermissionDecision{Action: PermissionActionNeedEvidence, Reason: "need logs"}
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (ToolResult, error) {
			return ToolResult{Content: "ok"}, nil
		},
	}
	if err := registry.Register(base); err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	assembled := registry.AssembleToolsWithOptions("host", "inspect", AssembleOptions{
		MetadataTransform: func(meta ToolMetadata) ToolMetadata {
			meta.Description = "transformed desc"
			meta.RiskLevel = ToolRiskLow
			return meta
		},
	})
	if len(assembled) != 1 {
		t.Fatalf("assembled len = %d, want 1", len(assembled))
	}
	tool := assembled[0]
	if got := tool.Metadata().Description; got != "transformed desc" {
		t.Fatalf("Metadata().Description = %q, want transformed", got)
	}
	if !tool.IsEnabled(ToolContext{SessionType: "host", Mode: "inspect"}) || !tool.IsReadOnly(nil) || !tool.IsConcurrencySafe(nil) {
		t.Fatalf("transformed tool did not delegate enabled/readOnly/concurrency")
	}
	if tool.IsDestructive(nil) {
		t.Fatal("transformed tool IsDestructive() = true, want false")
	}
	if len(tool.InputSchema()) == 0 || len(tool.OutputSchema()) == 0 {
		t.Fatal("transformed tool should delegate schemas")
	}
	if err := tool.ValidateInput(context.Background(), nil); err != nil {
		t.Fatalf("ValidateInput() error = %v", err)
	}
	if decision := tool.CheckPermissions(context.Background(), nil); decision.Action != PermissionActionNeedEvidence {
		t.Fatalf("CheckPermissions() = %#v, want need evidence", decision)
	}
	result, err := tool.Execute(context.Background(), nil)
	if err != nil || result.Content != "ok" {
		t.Fatalf("Execute() = %#v, %v; want ok", result, err)
	}
}

func TestToolMetadataSourceAndStreamHelpers(t *testing.T) {
	cases := []struct {
		name   string
		meta   ToolMetadata
		source ToolSource
		want   bool
	}{
		{"mcp", ToolMetadata{IsMCP: true}, ToolSourceMCP, true},
		{"lsp", ToolMetadata{IsLSP: true}, ToolSourceLSP, true},
		{"meta", ToolMetadata{Origin: ToolOriginMeta}, ToolSourceMeta, true},
		{"builtin", ToolMetadata{Name: "local"}, ToolSourceBuiltin, true},
		{"unknown", ToolMetadata{Name: "local"}, ToolSource("other"), false},
	}
	for _, tc := range cases {
		if got := tc.meta.HasSource(tc.source); got != tc.want {
			t.Fatalf("%s HasSource(%q) = %v, want %v", tc.name, tc.source, got, tc.want)
		}
	}
	if (ToolResult{}).HasStream() {
		t.Fatal("empty result HasStream() = true, want false")
	}
	if !(ToolResult{Stream: &StreamingResult{Reader: strings.NewReader("ok")}}).HasStream() {
		t.Fatal("stream-backed result HasStream() = false, want true")
	}
}

func toolNamesForTest(tools []Tool) []string {
	out := make([]string, 0, len(tools))
	for _, tool := range tools {
		out = append(out, tool.Metadata().Name)
	}
	return out
}
