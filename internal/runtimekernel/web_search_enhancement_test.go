package runtimekernel

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/cloudwego/eino/schema"

	"aiops-v2/internal/modelrouter"
	"aiops-v2/internal/tooling"
)

func TestLegacyMustSearchMetadataDoesNotRetryOrGateFinal(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("最可能是从节点保留了旧 PGDATA，建议清空后重新 basebackup。", nil),
		},
	}
	kernel, _ := newWebSearchEnhancementKernel(t, model, []tooling.Tool{webSearchEnhancementTestTool(nil)})

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-legacy-must-search-no-gate",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-legacy-must-search-no-gate",
		Input:       "查一下 PostgreSQL timeline 官方文档后再回答",
		Metadata: map[string]string{
			"aiops.webSearch.policy":           "must_search",
			"aiops.webSearch.reason":           "explicit_public_web_request",
			"aiops.webSearch.querySeeds":       "PostgreSQL timeline official docs",
			"aiops.webSearch.requireCitations": "true",
			"enableToolPack":                   "public_web",
			"aiops.weblearn.enabled":           "true",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "最可能是从节点保留了旧 PGDATA，建议清空后重新 basebackup。" {
		t.Fatalf("output = %q, want original model answer without public-web gate", result.Output)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model calls = %d, want no forced web_search retry", len(model.inputs))
	}
	session := kernel.sessions.Get("sess-legacy-must-search-no-gate")
	if session.CurrentTurn.Metadata["aiops.webSearch.finalLimited"] == "true" {
		t.Fatalf("metadata = %#v, must not mark final limited because web_search was not attempted", session.CurrentTurn.Metadata)
	}
	if strings.Contains(result.Output, "未能完成公开网页检索验证") ||
		strings.Contains(result.Output, "还不能给出已核对公开来源的结论") {
		t.Fatalf("output = %q, must not be replaced by public-web limitation template", result.Output)
	}
}

func TestEnabledWebSearchPolicyDoesNotRetryDirectFinal(t *testing.T) {
	model := &sequentialLoopModel{
		responses: []*schema.Message{
			schema.AssistantMessage("普通公开知识回答。", nil),
		},
	}
	kernel, _ := newWebSearchEnhancementKernel(t, model, []tooling.Tool{webSearchEnhancementTestTool(nil)})

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-enabled-no-retry",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-enabled-no-retry",
		Input:       "Redis latency doctor 是什么意思？",
		Metadata: map[string]string{
			"aiops.webSearch.policy": "enabled",
			"enableToolPack":         "public_web",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "普通公开知识回答。" {
		t.Fatalf("output = %q", result.Output)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model calls = %d, want no web_search retry", len(model.inputs))
	}
}

func TestProviderNativeWebSearchEventStillCountsAsSearchEvidence(t *testing.T) {
	first := schema.AssistantMessage("基于原生搜索事件回答。", nil)
	first.Extra = map[string]any{
		modelrouter.ProviderNativeWebSearchExtraKey: []modelrouter.ProviderNativeWebSearchEvent{{
			ID:       "ws-native-1",
			Provider: "zhipu",
			Query:    "PostgreSQL timeline official docs",
			Summary:  "provider-native web_search completed",
		}},
	}
	model := &sequentialLoopModel{responses: []*schema.Message{first}}
	kernel, _ := newWebSearchEnhancementKernel(t, model, nil)

	result, err := kernel.RunTurn(context.Background(), TurnRequest{
		SessionID:   "sess-native-web-search-evidence",
		SessionType: SessionTypeWorkspace,
		Mode:        ModeChat,
		TurnID:      "turn-native-web-search-evidence",
		Input:       "查一下 PostgreSQL timeline 官方文档后再回答",
		Metadata: map[string]string{
			"aiops.webSearch.policy":     "enabled",
			"aiops.webSearch.reason":     "explicit_public_web_request",
			"aiops.webSearch.querySeeds": "PostgreSQL timeline official docs",
			"enableToolPack":             "public_web",
		},
	})
	if err != nil {
		t.Fatalf("RunTurn failed: %v", err)
	}
	if result.Output != "基于原生搜索事件回答。" {
		t.Fatalf("output = %q", result.Output)
	}
	if len(model.inputs) != 1 {
		t.Fatalf("model calls = %d, want no retry after provider-native search event", len(model.inputs))
	}
}

func newWebSearchEnhancementKernel(t *testing.T, chatModel modelrouter.ChatModel, tools []tooling.Tool) (*RuntimeKernel, *recordingCompiler) {
	t.Helper()
	registry := tooling.NewRegistry()
	for _, toolDef := range tools {
		if err := registry.Register(toolDef); err != nil {
			t.Fatalf("Register tool failed: %v", err)
		}
	}
	compiler := newRecordingCompiler()
	kernel, _ := newKernelForLoopTests(t, &testMockToolAssemblySource{registry: registry}, compiler, chatModel)
	return kernel, compiler
}

func webSearchEnhancementTestTool(executed *int) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "web_search",
			Pack:        "public_web",
			Description: "Search public web pages",
		},
		Visibility: tooling.Visibility{
			SessionTypes: []string{string(SessionTypeWorkspace), string(SessionTypeHost)},
			Modes:        []string{string(ModeChat), string(ModeInspect)},
		},
		ExecuteFunc: func(context.Context, json.RawMessage) (tooling.ToolResult, error) {
			if executed != nil {
				*executed = *executed + 1
			}
			return tooling.ToolResult{
				Content: `{"query":"PostgreSQL timeline official docs","source":"custom_public_web:search","content":"search result"}`,
				Display: &tooling.ToolDisplayPayload{
					Type: "web_search",
				},
			}, nil
		},
	}
}
