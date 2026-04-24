package modelrouter

import (
	"context"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

type streamOnlyChatModel struct{}

func (m *streamOnlyChatModel) Generate(context.Context, []*schema.Message, ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant}, nil
}

func (m *streamOnlyChatModel) Stream(context.Context, []*schema.Message, ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	reader, writer := schema.Pipe[*schema.Message](2)
	go func() {
		defer writer.Close()
		writer.Send(&schema.Message{Role: schema.Assistant, Content: "po"}, nil)
		writer.Send(&schema.Message{Role: schema.Assistant, Content: "ng"}, nil)
	}()
	return reader, nil
}

func (m *streamOnlyChatModel) BindTools([]*schema.ToolInfo) error {
	return nil
}

func TestStreamGenerateChatModelGenerateUsesStreamChunks(t *testing.T) {
	wrapped := &streamGenerateChatModel{inner: &streamOnlyChatModel{}}

	msg, err := wrapped.Generate(context.Background(), []*schema.Message{schema.UserMessage("ping")})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if msg.Content != "pong" {
		t.Fatalf("Generate() content = %q, want stream content", msg.Content)
	}
}
