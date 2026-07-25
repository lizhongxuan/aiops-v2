package service

import (
	"context"
	"strings"
	"testing"
)

func TestVisualWorkflowAINoTemplateRequiresRuntimeKernel(t *testing.T) {
	service := NewVisualWorkflowService(VisualWorkflowServiceConfig{})
	result, err := service.GenerateAIDraft(context.Background(), VisualWorkflowAIDraftRequest{Instruction: "检查主机资源"})
	if err == nil || result != nil {
		t.Fatalf("legacy AI draft returned fixed graph: result=%#v err=%v", result, err)
	}
	if !strings.Contains(err.Error(), "AssistantTransport") || !strings.Contains(err.Error(), "workflow_agent") {
		t.Fatalf("migration error = %v", err)
	}
}
