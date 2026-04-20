package modelrouter

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

// mockModel is a simple mock implementing model.ChatModel for testing.
type mockModel struct {
	name string
}

func (m *mockModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "response from " + m.name}, nil
}

func (m *mockModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *mockModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

func TestNewRouter(t *testing.T) {
	providers := map[string]ChatModel{
		"openai": &mockModel{name: "openai"},
	}
	r := NewRouter("openai", providers, nil)
	if r == nil {
		t.Fatal("expected non-nil Router")
	}
	if r.defaultProvider != "openai" {
		t.Errorf("expected defaultProvider=openai, got %s", r.defaultProvider)
	}
}

func TestGetModel_ExplicitProvider(t *testing.T) {
	openai := &mockModel{name: "openai"}
	anthropic := &mockModel{name: "anthropic"}
	providers := map[string]ChatModel{
		"openai":    openai,
		"anthropic": anthropic,
	}
	r := NewRouter("openai", providers, nil)

	m, err := r.GetModel(AgentKindWorker, ProviderConfig{Provider: "anthropic", Model: "claude-3-5-sonnet"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != anthropic {
		t.Error("expected anthropic model instance")
	}
}

func TestGetModel_DefaultProvider(t *testing.T) {
	openai := &mockModel{name: "openai"}
	providers := map[string]ChatModel{
		"openai": openai,
	}
	r := NewRouter("openai", providers, nil)

	m, err := r.GetModel(AgentKindPlanner, ProviderConfig{Model: "gpt-4o"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != openai {
		t.Error("expected openai model instance (default)")
	}
}

func TestGetModel_Fallback(t *testing.T) {
	anthropic := &mockModel{name: "anthropic"}
	providers := map[string]ChatModel{
		"anthropic": anthropic,
	}
	fallbacks := []FallbackEntry{
		{Primary: "openai", Fallback: "anthropic"},
	}
	r := NewRouter("openai", providers, fallbacks)

	m, err := r.GetModel(AgentKindWorker, ProviderConfig{Provider: "openai"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != anthropic {
		t.Error("expected anthropic model via fallback")
	}
}

func TestGetModel_ProviderNotFound(t *testing.T) {
	providers := map[string]ChatModel{
		"openai": &mockModel{name: "openai"},
	}
	r := NewRouter("openai", providers, nil)

	_, err := r.GetModel(AgentKindWorker, ProviderConfig{Provider: "unknown"})
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}

	var pnf *ProviderNotFoundError
	if !errors.As(err, &pnf) {
		t.Fatalf("expected ProviderNotFoundError, got %T", err)
	}
	if pnf.Provider != "unknown" {
		t.Errorf("expected provider=unknown, got %s", pnf.Provider)
	}
}

func TestGetModel_FallbackChainExhausted(t *testing.T) {
	providers := map[string]ChatModel{
		"ollama": &mockModel{name: "ollama"},
	}
	fallbacks := []FallbackEntry{
		{Primary: "openai", Fallback: "anthropic"},
	}
	r := NewRouter("openai", providers, fallbacks)

	// openai not in providers, fallback anthropic also not in providers
	_, err := r.GetModel(AgentKindPlanner, ProviderConfig{Provider: "openai"})
	if err == nil {
		t.Fatal("expected error when fallback chain is exhausted")
	}
}

func TestAgentKindConstants(t *testing.T) {
	if AgentKindPlanner != "planner" {
		t.Errorf("expected planner, got %s", AgentKindPlanner)
	}
	if AgentKindWorker != "worker" {
		t.Errorf("expected worker, got %s", AgentKindWorker)
	}
}

func TestProviderNotFoundError_Error(t *testing.T) {
	err := &ProviderNotFoundError{Provider: "test-provider"}
	expected := "modelrouter: provider not found: test-provider"
	if err.Error() != expected {
		t.Errorf("expected %q, got %q", expected, err.Error())
	}
}
