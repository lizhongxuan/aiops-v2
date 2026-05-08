package runtimekernel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/modelrouter"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type emptyResponseModel struct{}

func (m *emptyResponseModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant}, nil
}

func (m *emptyResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *emptyResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseRejectsEmptyAssistantMessage(t *testing.T) {
	_, err := generateModelResponse(context.Background(), &emptyResponseModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected empty model response error")
	}
	if !strings.Contains(err.Error(), "empty model response") {
		t.Fatalf("error = %v, want empty model response", err)
	}
}

type streamingResponseModel struct {
	chunks []*schema.Message
}

func (m *streamingResponseModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called when streaming is available")
}

func (m *streamingResponseModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](len(m.chunks) + 1)
	go func() {
		defer sw.Close()
		for _, chunk := range m.chunks {
			sw.Send(chunk, nil)
		}
	}()
	return sr, nil
}

func (m *streamingResponseModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseStreamsChunksAndConcatsFinalMessage(t *testing.T) {
	model := &streamingResponseModel{
		chunks: []*schema.Message{
			schema.AssistantMessage("第一段", nil),
			schema.AssistantMessage("第二段", nil),
		},
	}

	var deltas []string
	msg, err := generateModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		func(delta string) {
			deltas = append(deltas, delta)
		},
		nil,
	)
	if err != nil {
		t.Fatalf("generateModelResponse returned error: %v", err)
	}
	if msg.Content != "第一段第二段" {
		t.Fatalf("response content = %q, want concatenated stream", msg.Content)
	}
	if got, want := strings.Join(deltas, "|"), "第一段|第二段"; got != want {
		t.Fatalf("stream deltas = %q, want %q", got, want)
	}
}

type noToolOptionModel struct{}

func (m *noToolOptionModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called when streaming is available")
}

func (m *noToolOptionModel) Stream(_ context.Context, _ []*schema.Message, opts ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	if len(opts) != 0 {
		return nil, errors.New("unexpected tool options without tools")
	}
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		sw.Send(schema.AssistantMessage("final", nil), nil)
	}()
	return sr, nil
}

func (m *noToolOptionModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseOmitsToolOptionsWhenNoTools(t *testing.T) {
	msg, err := generateModelResponse(context.Background(), &noToolOptionModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, nil, nil)
	if err != nil {
		t.Fatalf("generateModelResponse returned error: %v", err)
	}
	if msg.Content != "final" {
		t.Fatalf("response content = %q, want final", msg.Content)
	}
}

func TestGenerateModelResponseEmitsOnlyReasoningSummaryEvents(t *testing.T) {
	model := &streamingResponseModel{
		chunks: []*schema.Message{
			{
				Role: schema.Assistant,
				Extra: map[string]any{
					"method": "item/reasoning/summaryTextDelta",
					"params": map[string]any{
						"threadId":     "thread_1",
						"turnId":       "turn_1",
						"itemId":       "reasoning_1",
						"summaryIndex": float64(0),
						"delta":        "我会先查看项目结构。",
					},
				},
			},
			{
				Role: schema.Assistant,
				Extra: map[string]any{
					"method": "item/reasoning/textDelta",
					"params": map[string]any{
						"threadId":     "thread_1",
						"turnId":       "turn_1",
						"itemId":       "reasoning_1",
						"contentIndex": float64(0),
						"delta":        "raw hidden",
					},
				},
			},
			schema.AssistantMessage("final", nil),
		},
	}

	var reasoning []modelrouter.ReasoningStreamEvent
	msg, err := generateModelResponse(
		context.Background(),
		model,
		[]*schema.Message{schema.UserMessage("ping")},
		nil,
		nil,
		func(event modelrouter.ReasoningStreamEvent) {
			reasoning = append(reasoning, event)
		},
	)
	if err != nil {
		t.Fatalf("generateModelResponse returned error: %v", err)
	}
	if msg.Content != "final" {
		t.Fatalf("response content = %q, want final", msg.Content)
	}
	if len(reasoning) != 1 {
		t.Fatalf("reasoning events length = %d, want 1: %+v", len(reasoning), reasoning)
	}
	if reasoning[0].Method != "item/reasoning/summaryTextDelta" || reasoning[0].Delta != "我会先查看项目结构。" {
		t.Fatalf("reasoning[0] = %+v, want summary delta", reasoning[0])
	}
}

// cancelStreamModel simulates a streaming model that blocks until context is canceled.
type cancelStreamModel struct{}

func (m *cancelStreamModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return nil, errors.New("generate should not be called")
}

func (m *cancelStreamModel) Stream(ctx context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	sr, sw := schema.Pipe[*schema.Message](1)
	go func() {
		defer sw.Close()
		// Block until context is canceled, then send the context error.
		<-ctx.Done()
		sw.Send(nil, ctx.Err())
	}()
	return sr, nil
}

func (m *cancelStreamModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestGenerateModelResponseReturnsContextCanceledImmediately(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	_, err := generateModelResponse(ctx, &cancelStreamModel{}, []*schema.Message{schema.UserMessage("ping")}, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error from canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
}
