package runtimekernel

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/tooling"
)

func TestProgressiveDiscoverySearchSelectUseFlow(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{toolSearchCall("call-search", `{"mode":"search","query":"synthetic metrics read"}`)}),
		schema.AssistantMessage("", []schema.ToolCall{toolSearchCall("call-select", `{"mode":"select","tools":["synthetic.metrics.read"],"reason":"need checked synthetic metrics evidence"}`)}),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-read",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_metrics_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("final evidence: synthetic.metrics.read checked; confidence high", nil),
	}}
	registry := progressiveDiscoveryRegistry(t, false)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-progressive-search-select",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-progressive-search-select",
		Input:       "use tool_search to discover a deferred synthetic metrics tool",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Status != "completed" || !strings.Contains(result.Output, "checked") {
		t.Fatalf("result = %#v, want checked final", result)
	}
	if len(compiler.contexts) < 3 {
		t.Fatalf("compiler contexts = %d, want at least 3", len(compiler.contexts))
	}
	first := toolNames(compiler.contexts[0].AssembledTools)
	if containsString(first, "synthetic.metrics.read") {
		t.Fatalf("first tools = %v, deferred metrics read should not be visible before select", first)
	}
	second := toolNames(compiler.contexts[1].AssembledTools)
	if containsString(second, "synthetic.metrics.read") {
		t.Fatalf("second tools = %v, search alone should not load deferred tool", second)
	}
	third := toolNames(compiler.contexts[2].AssembledTools)
	if !containsString(third, "synthetic.metrics.read") {
		t.Fatalf("third tools = %v, want selected synthetic.metrics.read", third)
	}
	if !containsString(compiler.contexts[2].ToolDelta.NewlyAvailable, "synthetic.metrics.read") {
		t.Fatalf("third tool delta = %#v, want selected tool delta", compiler.contexts[2].ToolDelta)
	}
}

func TestProgressiveDiscoverySelectablePackRecordsLoadedPacksDeltaInToolSurfaceSnapshot(t *testing.T) {
	traceDir := t.TempDir()
	setLegacyTraceRootForTest(t, traceDir)

	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{toolSearchCall("call-search", `{"mode":"search","query":"coroot postgres rca"}`)}),
		schema.AssistantMessage("", []schema.ToolCall{toolSearchCall("call-select", `{"mode":"select","packs":["coroot_postgres"],"reason":"need coroot postgres rca evidence"}`)}),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-rca",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "coroot_postgres_rca",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("final evidence: coroot postgres rca checked", nil),
	}}
	registry := progressiveDiscoveryPackRegistry(t)
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}, compiler, model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-progressive-pack-select",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-progressive-pack-select",
		Input:       "use tool_search to discover the needed deferred evidence pack",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	if len(compiler.contexts) < 3 {
		t.Fatalf("compiler contexts = %d, want at least 3", len(compiler.contexts))
	}
	if !containsString(compiler.contexts[2].ToolDelta.NewlyAvailablePacks, "coroot_postgres") {
		t.Fatalf("third tool delta packs = %v, want coroot_postgres", compiler.contexts[2].ToolDelta.NewlyAvailablePacks)
	}
	session := kernel.sessions.Get("sess-progressive-pack-select")
	if session == nil || len(session.ToolDiscovery.LastSearchResults) == 0 || session.ToolDiscovery.LastSearchResults[0].SelectablePack == nil {
		t.Fatalf("last search results = %#v, want selectable pack hint", session)
	}
	if !traceDirContainsLoadedPackDelta(t, traceDir, "turn-progressive-pack-select", "coroot_postgres") {
		t.Fatalf("trace dir %s missing toolSurfaceSnapshot.loadedPacksDelta coroot_postgres", traceDir)
	}
}

func TestProgressiveDiscoveryRejectsUnloadedToolFlow(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-unloaded",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic.metrics.read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("", []schema.ToolCall{toolSearchCall("call-search", `{"mode":"search","query":"synthetic metrics read"}`)}),
		schema.AssistantMessage("", []schema.ToolCall{toolSearchCall("call-select", `{"mode":"select","tools":["synthetic.metrics.read"],"reason":"recover unloaded synthetic metrics read"}`)}),
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-read",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_metrics_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("final evidence: synthetic.metrics.read checked after select; confidence high", nil),
		schema.AssistantMessage("final evidence: synthetic.metrics.read checked after select; earlier direct call was not_checked; confidence low", nil),
	}}
	registry := progressiveDiscoveryRegistry(t, false)
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}, newRecordingCompiler(), model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-progressive-unloaded",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-progressive-unloaded",
		Input:       "use tool_search to discover a deferred synthetic metrics tool after direct unloaded call recovery",
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	if result.Status != "completed" {
		t.Fatalf("result status = %q, want completed", result.Status)
	}
	session := kernel.sessions.Get("sess-progressive-unloaded")
	if session == nil || len(session.ToolDiscovery.RejectedCalls) == 0 {
		t.Fatalf("missing rejected tool discovery state: %#v", session)
	}
	if got := session.ToolDiscovery.RejectedCalls[0].ErrorType; got != "tool_unloaded" {
		t.Fatalf("rejected error type = %q, want tool_unloaded", got)
	}
	if !containsString(session.ToolDiscovery.EnabledTools(), "synthetic.metrics.read") {
		t.Fatalf("enabled tools = %v, want synthetic.metrics.read after select", session.ToolDiscovery.EnabledTools())
	}
	if !strings.Contains(result.Output, "证据") && !strings.Contains(strings.ToLower(result.Output), "evidence") {
		t.Fatalf("final output = %q, want verifier-constrained evidence boundary", result.Output)
	}
	if !strings.Contains(result.Output, "受限") && !strings.Contains(strings.ToLower(result.Output), "limited") {
		t.Fatalf("final output = %q, want limited evidence boundary", result.Output)
	}
	if strings.Contains(result.Output, "confidence low") || strings.Contains(result.Output, "高置信") {
		t.Fatalf("final output must not expose confidence labels:\n%s", result.Output)
	}
	if len(model.inputs) != 5 {
		t.Fatalf("model calls = %d, want no verifier-triggered recovery iteration", len(model.inputs))
	}
}

func TestDeferredToolDirectoryRequiresExplicitToolSearch(t *testing.T) {
	registry := progressiveDiscoveryRegistry(t, false)
	kernel := &RuntimeKernel{tools: &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}}

	defaultCtx := kernel.compileContext(SessionTypeHost, ModeInspect, map[string]string{})
	if len(defaultCtx.DeferredToolCatalog) != 0 {
		t.Fatalf("default deferred catalog len = %d, want 0 without explicit tool_search", len(defaultCtx.DeferredToolCatalog))
	}

	enabledCtx := kernel.compileContext(SessionTypeHost, ModeInspect, map[string]string{
		"aiops.toolSearch.enabled": "true",
		"enableTool":               "tool_search",
	})
	if len(enabledCtx.DeferredToolCatalog) == 0 {
		t.Fatal("deferred catalog should be rendered when tool_search discovery is explicitly enabled")
	}
	if !containsString(toolNames(enabledCtx.AssembledTools), "tool_search") {
		t.Fatalf("enabled tools = %v, want tool_search visible", toolNames(enabledCtx.AssembledTools))
	}
}

func progressiveDiscoveryPackRegistry(t *testing.T) *tooling.Registry {
	t.Helper()
	registry := tooling.NewRegistry()
	tools := []tooling.Tool{
		&tooling.StaticTool{
			Meta:                tooling.ToolMetadata{Name: "tool_search", Layer: tooling.ToolLayerCore, RiskLevel: tooling.ToolRiskLow},
			ReadOnlyFunc:        func(json.RawMessage) bool { return true },
			ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(_ context.Context, raw json.RawMessage) (tooling.ToolResult, error) {
				var req struct {
					Mode string `json:"mode"`
				}
				_ = json.Unmarshal(raw, &req)
				if req.Mode == "select" {
					return tooling.ToolResult{Content: `{"mode":"select","selection":{"loadedPacks":["coroot_postgres"],"reason":"need coroot postgres rca evidence"}}`}, nil
				}
				return tooling.ToolResult{Content: `{"mode":"search","ranker":"bm25","request":{"mode":"search","query":"coroot postgres rca","ranker":"bm25"},"matches":[{"kind":"pack","name":"coroot_postgres","pack":"coroot_postgres","tools":["coroot.postgres.rca"],"capabilityKind":"rca","resourceTypes":["postgres","service"],"operationKinds":["read"],"riskLevel":"low","requiresSelect":true,"loadableToolSpec":{"name":"coroot.postgres.rca","pack":"coroot_postgres","requiresSelect":true},"selectablePack":{"pack":"coroot_postgres","tools":["coroot.postgres.rca"],"requiresSelect":true}}]}`}, nil
			},
		},
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:           "coroot.postgres.rca",
				Layer:          tooling.ToolLayerDeferred,
				Pack:           "coroot_postgres",
				DeferByDefault: true,
				RiskLevel:      tooling.ToolRiskLow,
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind: "rca",
					ResourceTypes:  []string{"postgres", "service"},
					OperationKinds: []string{"read"},
					RequiresSelect: true,
				},
			},
			ReadOnlyFunc:        func(json.RawMessage) bool { return true },
			ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				return tooling.ToolResult{Content: `{"summary":"coroot postgres rca checked","status":"ok"}`}, nil
			},
		},
	}
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("Register(%s): %v", tool.Metadata().Name, err)
		}
	}
	return registry
}

func traceDirContainsLoadedPackDelta(t *testing.T, dir, turnID, pack string) bool {
	t.Helper()
	found := false
	err := filepath.WalkDir(dir, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found || entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") || strings.EqualFold(entry.Name(), "index.json") || strings.Contains(filepath.ToSlash(path), "/raw/") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var payload struct {
			TurnID              string `json:"turnId"`
			ToolSurfaceSnapshot *struct {
				LoadedPacksDelta []string `json:"loadedPacksDelta"`
			} `json:"toolSurfaceSnapshot"`
			PromptInputTrace *struct {
				ToolSurfaceSnapshot *struct {
					LoadedPacksDelta []string `json:"loadedPacksDelta"`
				} `json:"toolSurfaceSnapshot"`
			} `json:"promptInputTrace"`
		}
		if err := json.Unmarshal(data, &payload); err != nil {
			return err
		}
		if payload.TurnID != turnID {
			return nil
		}
		loadedPacksDelta := []string(nil)
		if payload.ToolSurfaceSnapshot != nil {
			loadedPacksDelta = payload.ToolSurfaceSnapshot.LoadedPacksDelta
		}
		if len(loadedPacksDelta) == 0 && payload.PromptInputTrace != nil && payload.PromptInputTrace.ToolSurfaceSnapshot != nil {
			loadedPacksDelta = payload.PromptInputTrace.ToolSurfaceSnapshot.LoadedPacksDelta
		}
		if containsString(loadedPacksDelta, pack) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk trace dir %s: %v", dir, err)
	}
	return found
}

func TestProgressiveDiscoveryFinalEvidenceFlow(t *testing.T) {
	model := &sequentialLoopModel{responses: []*schema.Message{
		schema.AssistantMessage("", []schema.ToolCall{{
			ID:   "call-failed",
			Type: "function",
			Function: schema.FunctionCall{
				Name:      "synthetic_metrics_read",
				Arguments: `{}`,
			},
		}}),
		schema.AssistantMessage("已确认 synthetic.metrics.read 检查完成，结论高置信。", nil),
		schema.AssistantMessage("synthetic.metrics.read 未成功返回证据；该项 not_checked，confidence low。", nil),
	}}
	registry := progressiveDiscoveryRegistry(t, true)
	kernel, _ := newKernelForLoopTests(t, &assemblerBackedToolSource{assembler: tooling.NewAssembler(registry)}, newRecordingCompiler(), model)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-progressive-final-evidence",
		SessionType: SessionTypeHost,
		Mode:        ModeInspect,
		TurnID:      "turn-progressive-final-evidence",
		Input:       "verify synthetic final evidence behavior",
		Metadata:    map[string]string{"aiops.intentToolPack.synthetic_metrics": "1"},
	})
	if err != nil {
		t.Fatalf("RunTurn: %v", err)
	}
	for _, want := range []string{"还不能给最终结论", "synthetic_metrics_read 未成功返回证据"} {
		if !strings.Contains(result.Output, want) {
			t.Fatalf("final output = %q, want deterministic evidence boundary containing %q", result.Output, want)
		}
	}
	if strings.Contains(result.Output, "confidence") || strings.Contains(result.Output, "置信度") || strings.Contains(result.Output, "高置信") {
		t.Fatalf("final output must not expose confidence labels:\n%s", result.Output)
	}
}

func progressiveDiscoveryRegistry(t *testing.T, failRead bool) *tooling.Registry {
	t.Helper()
	registry := tooling.NewRegistry()
	tools := []tooling.Tool{
		&tooling.StaticTool{
			Meta:                tooling.ToolMetadata{Name: "tool_search", Layer: tooling.ToolLayerCore, RiskLevel: tooling.ToolRiskLow},
			ReadOnlyFunc:        func(json.RawMessage) bool { return true },
			ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(_ context.Context, raw json.RawMessage) (tooling.ToolResult, error) {
				var req struct {
					Mode string `json:"mode"`
				}
				_ = json.Unmarshal(raw, &req)
				if req.Mode == "select" {
					return tooling.ToolResult{Content: `{"mode":"select","selection":{"loadedTools":["synthetic.metrics.read"],"reason":"selected synthetic metrics read"}}`}, nil
				}
				return tooling.ToolResult{Content: `{"mode":"search","matches":[{"kind":"tool","name":"synthetic.metrics.read","pack":"synthetic_metrics","tools":["synthetic.metrics.read"],"capabilityKind":"read","resourceTypes":["metric"],"operationKinds":["read"],"riskLevel":"low","requiresSelect":true}]}`}, nil
			},
		},
		&tooling.StaticTool{
			Meta: tooling.ToolMetadata{
				Name:           "synthetic.metrics.read",
				Layer:          tooling.ToolLayerDeferred,
				Pack:           "synthetic_metrics",
				DeferByDefault: true,
				RiskLevel:      tooling.ToolRiskLow,
				Discovery: tooling.ToolDiscoveryMetadata{
					CapabilityKind: "read",
					ResourceTypes:  []string{"metric"},
					OperationKinds: []string{"read"},
					RequiresSelect: true,
				},
			},
			ReadOnlyFunc:        func(json.RawMessage) bool { return true },
			ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
			ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
				if failRead {
					return tooling.ToolResult{}, errors.New("synthetic metrics read timeout")
				}
				return tooling.ToolResult{Content: `{"summary":"synthetic metrics checked","status":"ok"}`}, nil
			},
		},
	}
	for _, tool := range tools {
		if err := registry.Register(tool); err != nil {
			t.Fatalf("Register(%s): %v", tool.Metadata().Name, err)
		}
	}
	return registry
}

func toolSearchCall(id, args string) schema.ToolCall {
	return schema.ToolCall{
		ID:   id,
		Type: "function",
		Function: schema.FunctionCall{
			Name:      "tool_search",
			Arguments: args,
		},
	}
}
