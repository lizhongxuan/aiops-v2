package runtimekernel

import (
	"context"
	"strings"
	"testing"

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
	_, err := generateModelResponse(context.Background(), &emptyResponseModel{}, []*schema.Message{schema.UserMessage("ping")}, nil)
	if err == nil {
		t.Fatal("expected empty model response error")
	}
	if !strings.Contains(err.Error(), "empty model response") {
		t.Fatalf("error = %v, want empty model response", err)
	}
}
