package modelrouter

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type directGenerateChatModel struct{}

func (m *directGenerateChatModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return schema.AssistantMessage("pong", nil), nil
}

func (m *directGenerateChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer writer.Close()
		writer.Send(&schema.Message{Role: schema.Assistant, ReasoningContent: "thinking"}, nil)
	}()
	return reader, nil
}

func (m *directGenerateChatModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

type bindCaptureChatModel struct {
	directGenerateChatModel
	boundTools []*schema.ToolInfo
}

func (m *bindCaptureChatModel) BindTools(tools []*schema.ToolInfo) error {
	m.boundTools = append([]*schema.ToolInfo(nil), tools...)
	return nil
}

func TestStreamGenerateChatModelGenerateUsesInnerGenerate(t *testing.T) {
	wrapped := &streamGenerateChatModel{inner: &directGenerateChatModel{}}

	msg, err := wrapped.Generate(context.Background(), []*schema.Message{schema.UserMessage("ping")})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content != "pong" {
		t.Fatalf("Generate() content = %q, want inner generate content", msg.Content)
	}
}

func TestOpenAIReasoningEffortOnlyAppliesToOpenAINativeReasoningModels(t *testing.T) {
	if got := openAIReasoningEffortForModel("gpt-5.4", "high"); got != "high" {
		t.Fatalf("gpt-5.4 effort = %q, want high", got)
	}
	if got := openAIReasoningEffortForModel("glm-4.7", "high"); got != "" {
		t.Fatalf("glm-4.7 effort = %q, want empty OpenAI-native effort", got)
	}
}

func TestOpenAICompatibleExtraFieldsAreProviderScoped(t *testing.T) {
	openaiExtra := openAICompatibleExtraFields("openai", ProviderConfig{
		ThinkingType:    "enabled",
		ReasoningEffort: "max",
		ToolStream:      true,
	})
	if len(openaiExtra) != 0 {
		t.Fatalf("openai extra fields = %#v, want none", openaiExtra)
	}

	deepseekExtra := openAICompatibleExtraFields("deepseek", ProviderConfig{
		ThinkingType:    "enabled",
		ReasoningEffort: "max",
	})
	if got := nestedString(deepseekExtra, "thinking", "type"); got != "enabled" {
		t.Fatalf("deepseek thinking.type = %q, want enabled in %#v", got, deepseekExtra)
	}
	if deepseekExtra["reasoning_effort"] != "max" {
		t.Fatalf("deepseek reasoning_effort = %#v, want max", deepseekExtra["reasoning_effort"])
	}
	if _, ok := deepseekExtra["tool_stream"]; ok {
		t.Fatalf("deepseek extra fields should not include tool_stream: %#v", deepseekExtra)
	}

	zhipuExtra := openAICompatibleExtraFields("zhipu", ProviderConfig{
		ThinkingType:    "disabled",
		ReasoningEffort: "xhigh",
		ToolStream:      true,
	})
	if got := nestedString(zhipuExtra, "thinking", "type"); got != "disabled" {
		t.Fatalf("zhipu thinking.type = %q, want disabled in %#v", got, zhipuExtra)
	}
	if zhipuExtra["reasoning_effort"] != "xhigh" {
		t.Fatalf("zhipu reasoning_effort = %#v, want xhigh", zhipuExtra["reasoning_effort"])
	}
	if zhipuExtra["tool_stream"] != true {
		t.Fatalf("zhipu tool_stream = %#v, want true", zhipuExtra["tool_stream"])
	}
}

func TestOpenAICompatibleNativeWebSearchToolsPreferProviderTool(t *testing.T) {
	toolInfos := []*schema.ToolInfo{
		{
			Name: "web_search",
			Desc: "Search the public web with the custom implementation.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"query": {Type: schema.String, Required: true},
			}),
		},
		{
			Name: "read_logs",
			Desc: "Read local logs.",
			ParamsOneOf: schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
				"path": {Type: schema.String, Required: true},
			}),
		},
	}

	for _, provider := range []string{"openai", "zhipu"} {
		t.Run(provider, func(t *testing.T) {
			extra := openAICompatibleNativeWebSearchExtraFields(provider, toolInfos)
			rawTools, ok := extra["tools"].([]any)
			if !ok {
				t.Fatalf("tools extra = %#v, want []any", extra["tools"])
			}
			if len(rawTools) != 2 {
				t.Fatalf("tools = %#v, want provider web_search plus read_logs function", rawTools)
			}

			first, ok := rawTools[0].(map[string]any)
			if !ok || first["type"] != "web_search" {
				t.Fatalf("first tool = %#v, want native web_search", rawTools[0])
			}
			if hasFunctionToolNamed(rawTools, "web_search") {
				t.Fatalf("tools include custom web_search function: %#v", rawTools)
			}
			if !hasFunctionToolNamed(rawTools, "read_logs") {
				t.Fatalf("tools missing read_logs function: %#v", rawTools)
			}
		})
	}

	if extra := openAICompatibleNativeWebSearchExtraFields("deepseek", toolInfos); len(extra) != 0 {
		t.Fatalf("deepseek native web search extra fields = %#v, want none", extra)
	}
}

func TestStreamGenerateChatModelBindToolsOmitsCustomWebSearchForNativeProvider(t *testing.T) {
	capture := &bindCaptureChatModel{}
	wrapped := &streamGenerateChatModel{inner: capture, provider: "zhipu"}
	toolInfos := []*schema.ToolInfo{
		{Name: "web_search", Desc: "Custom public web search."},
		{Name: "read_logs", Desc: "Read local logs."},
	}

	if err := wrapped.BindTools(toolInfos); err != nil {
		t.Fatalf("BindTools() error = %v", err)
	}
	if hasToolInfoNamed(capture.boundTools, "web_search") {
		t.Fatalf("inner BindTools received custom web_search: %#v", capture.boundTools)
	}
	if !hasToolInfoNamed(capture.boundTools, "read_logs") {
		t.Fatalf("inner BindTools missing read_logs: %#v", capture.boundTools)
	}
	if !hasToolInfoNamed(wrapped.boundTools, "web_search") {
		t.Fatalf("wrapped bound tools should retain web_search for native request rewrite: %#v", wrapped.boundTools)
	}

	deepseekCapture := &bindCaptureChatModel{}
	deepseekWrapped := &streamGenerateChatModel{inner: deepseekCapture, provider: "deepseek"}
	if err := deepseekWrapped.BindTools(toolInfos); err != nil {
		t.Fatalf("deepseek BindTools() error = %v", err)
	}
	if !hasToolInfoNamed(deepseekCapture.boundTools, "web_search") {
		t.Fatalf("deepseek should keep custom web_search function tool: %#v", deepseekCapture.boundTools)
	}
}

func TestExtractProviderNativeWebSearchEventsFromOpenAICompatibleResponse(t *testing.T) {
	raw := []byte(`{
		"output": [
			{
				"id": "ws_123",
				"type": "web_search_call",
				"action": {
					"query": "OpenAI web_search docs",
					"sources": [
						{"title": "Web search guide", "url": "https://platform.openai.com/docs/guides/tools-web-search", "snippet": "Use web_search as a hosted tool."}
					]
				}
			},
			{
				"type": "message",
				"content": [
					{
						"type": "output_text",
						"text": "Use web_search.",
						"annotations": [
							{"type": "url_citation", "title": "OpenAI tools", "url": "https://platform.openai.com/docs/guides/tools"}
						]
					}
				]
			}
		]
	}`)

	events := ExtractProviderNativeWebSearchEvents(raw, "openai")
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1: %#v", len(events), events)
	}
	event := events[0]
	if event.ID != "ws_123" || event.Provider != "openai" || event.Query != "OpenAI web_search docs" {
		t.Fatalf("event identity = %#v, want provider/query/id", event)
	}
	if len(event.Sources) != 2 {
		t.Fatalf("sources = %#v, want web_search_call source plus annotation source", event.Sources)
	}
	if event.Sources[0].URL != "https://platform.openai.com/docs/guides/tools-web-search" {
		t.Fatalf("first source = %#v", event.Sources[0])
	}
}

func TestExtractProviderNativeWebSearchEventsFromChatCompletionAnnotations(t *testing.T) {
	raw := []byte(`{
		"choices": [
			{
				"message": {
					"content": "A cited answer.",
					"annotations": [
						{"type": "url_citation", "title": "Docs", "url": "https://example.com/docs", "snippet": "native citation"}
					]
				}
			}
		]
	}`)

	events := ExtractProviderNativeWebSearchEvents(raw, "zhipu")
	if len(events) != 1 {
		t.Fatalf("len(events) = %d, want 1: %#v", len(events), events)
	}
	if events[0].Provider != "zhipu" {
		t.Fatalf("provider = %q, want zhipu", events[0].Provider)
	}
	if len(events[0].Sources) != 1 || events[0].Sources[0].URL != "https://example.com/docs" {
		t.Fatalf("sources = %#v, want annotation source", events[0].Sources)
	}
}

func hasFunctionToolNamed(tools []any, name string) bool {
	for _, raw := range tools {
		tool, ok := raw.(map[string]any)
		if !ok || tool["type"] != "function" {
			continue
		}
		function, _ := tool["function"].(map[string]any)
		if function["name"] == name {
			return true
		}
	}
	return false
}

func hasToolInfoNamed(tools []*schema.ToolInfo, name string) bool {
	for _, tool := range tools {
		if tool != nil && tool.Name == name {
			return true
		}
	}
	return false
}

func nestedString(values map[string]any, key string, nestedKey string) string {
	raw, ok := values[key].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := raw[nestedKey].(string)
	return value
}

func TestParseOpenAIReasoningEventAcceptsSummaryMethodsAndDropsRawByDefault(t *testing.T) {
	tests := []struct {
		name      string
		raw       string
		wantNil   bool
		wantEvent ReasoningStreamEvent
	}{
		{
			name: "summary text delta",
			raw:  `{"method":"item/reasoning/summaryTextDelta","params":{"threadId":"thread_1","turnId":"turn_1","itemId":"reasoning_1","summaryIndex":0,"delta":"我会先查看项目结构。"}}`,
			wantEvent: ReasoningStreamEvent{
				Method:       "item/reasoning/summaryTextDelta",
				ThreadID:     "thread_1",
				TurnID:       "turn_1",
				ItemID:       "reasoning_1",
				SummaryIndex: 0,
				Delta:        "我会先查看项目结构。",
			},
		},
		{
			name: "summary part added",
			raw:  `{"method":"item/reasoning/summaryPartAdded","params":{"threadId":"thread_1","turnId":"turn_1","itemId":"reasoning_1","summaryIndex":1}}`,
			wantEvent: ReasoningStreamEvent{
				Method:       "item/reasoning/summaryPartAdded",
				ThreadID:     "thread_1",
				TurnID:       "turn_1",
				ItemID:       "reasoning_1",
				SummaryIndex: 1,
				PartAdded:    true,
			},
		},
		{
			name:    "raw text delta hidden by default",
			raw:     `{"method":"item/reasoning/textDelta","params":{"threadId":"thread_1","turnId":"turn_1","itemId":"reasoning_1","contentIndex":0,"delta":"raw hidden"}}`,
			wantNil: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseOpenAIReasoningEvent([]byte(tt.raw), false)
			if err != nil {
				t.Fatalf("ParseOpenAIReasoningEvent() error = %v", err)
			}
			if tt.wantNil {
				if got != nil {
					t.Fatalf("ParseOpenAIReasoningEvent() = %+v, want nil", *got)
				}
				return
			}
			if got == nil {
				t.Fatal("ParseOpenAIReasoningEvent() = nil, want event")
			}
			if *got != tt.wantEvent {
				t.Fatalf("ParseOpenAIReasoningEvent() = %+v, want %+v", *got, tt.wantEvent)
			}
		})
	}
}

func TestParseOpenAIReasoningEventAllowsRawOnlyWhenDebugEnabled(t *testing.T) {
	raw := []byte(`{"method":"item/reasoning/textDelta","params":{"threadId":"thread_1","turnId":"turn_1","itemId":"reasoning_1","contentIndex":2,"delta":"raw debug"}}`)

	got, err := ParseOpenAIReasoningEvent(raw, true)
	if err != nil {
		t.Fatalf("ParseOpenAIReasoningEvent() error = %v", err)
	}
	if got == nil {
		t.Fatal("ParseOpenAIReasoningEvent() = nil, want raw debug event")
	}
	if !got.Raw || got.ContentIndex != 2 || got.Delta != "raw debug" {
		t.Fatalf("raw event = %+v, want raw content index and delta", *got)
	}
}
