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
