package modelrouter

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func TestResolveModelCapabilitiesForLargeOpenAIModel(t *testing.T) {
	r := NewRouter("openai", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{Model: "gpt-5.4", MaxTokens: 24000})

	if caps.Provider != "openai" || caps.Model != "gpt-5.4" {
		t.Fatalf("capabilities route = %s/%s, want openai/gpt-5.4", caps.Provider, caps.Model)
	}
	if caps.MaxContextTokens != 200000 {
		t.Fatalf("max context = %d, want 200000", caps.MaxContextTokens)
	}
	if caps.MaxOutputTokens != 24000 {
		t.Fatalf("max output = %d, want override 24000", caps.MaxOutputTokens)
	}
	if !caps.ExactTokenCount || !caps.CacheEdit {
		t.Fatalf("expected openai capabilities to include exact token count and cache edit: %#v", caps)
	}
	if caps.SmallContextMode {
		t.Fatalf("large model should not use small context mode: %#v", caps)
	}
}

func TestResolveModelCapabilitiesForGLM47OpenAICompatibleModel(t *testing.T) {
	r := NewRouter("openai", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:        "openai",
		Model:           "glm-4.7",
		ReasoningEffort: "high",
	})

	if caps.Provider != "openai" || caps.Model != "glm-4.7" {
		t.Fatalf("capabilities route = %s/%s, want openai/glm-4.7", caps.Provider, caps.Model)
	}
	if caps.MaxContextTokens != 200000 {
		t.Fatalf("max context = %d, want 200000", caps.MaxContextTokens)
	}
	if caps.MaxOutputTokens != 128000 {
		t.Fatalf("max output = %d, want 128000", caps.MaxOutputTokens)
	}
	if !caps.SupportsToolCalls || !caps.SupportsStreaming || !caps.SupportsReasoning {
		t.Fatalf("glm-4.7 capabilities missing tool/streaming/reasoning support: %#v", caps)
	}
	if caps.NativeReasoning {
		t.Fatalf("glm-4.7 should not be treated as OpenAI-native reasoning_effort model: %#v", caps)
	}
	if caps.ReasoningEffortApplied != "" || caps.ReasoningFallbackPolicy == "" {
		t.Fatalf("glm-4.7 reasoning metadata = %#v, want prompt fallback not native effort", caps)
	}
}

func TestResolveModelCapabilitiesForZhipuGLM47(t *testing.T) {
	r := NewRouter("openai", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:        "zhipu",
		Model:           "glm-4.7",
		ReasoningEffort: "high",
	})

	if caps.Provider != "zhipu" || caps.Model != "glm-4.7" {
		t.Fatalf("capabilities route = %s/%s, want zhipu/glm-4.7", caps.Provider, caps.Model)
	}
	if caps.MaxContextTokens != 200000 || caps.MaxOutputTokens != 128000 {
		t.Fatalf("glm-4.7 context/output caps = %d/%d, want 200000/128000", caps.MaxContextTokens, caps.MaxOutputTokens)
	}
	if !caps.SupportsToolCalls || !caps.SupportsStreaming || !caps.SupportsReasoning {
		t.Fatalf("zhipu glm-4.7 capabilities missing tool/streaming/reasoning support: %#v", caps)
	}
	if !caps.NativeReasoning || caps.ReasoningEffortApplied != "high" {
		t.Fatalf("zhipu glm-4.7 must use provider-native reasoning_effort: %#v", caps)
	}
}

func TestResolveProviderModelDefaultsForDeepSeekAndZhipu(t *testing.T) {
	if got := resolveProviderModel("deepseek", ProviderConfig{}); got != "deepseek-v4-pro" {
		t.Fatalf("deepseek default model = %q, want deepseek-v4-pro", got)
	}
	if got := resolveProviderModel("zhipu", ProviderConfig{}); got != "glm-5.2" {
		t.Fatalf("zhipu default model = %q, want glm-5.2", got)
	}
}

func TestResolveModelCapabilitiesForDeepSeekV4(t *testing.T) {
	r := NewRouter("deepseek", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:        "deepseek",
		Model:           "deepseek-v4-pro",
		ReasoningEffort: "max",
	})

	if caps.Provider != "deepseek" || caps.Model != "deepseek-v4-pro" {
		t.Fatalf("capabilities route = %s/%s, want deepseek/deepseek-v4-pro", caps.Provider, caps.Model)
	}
	if caps.MaxContextTokens != 1000000 || caps.MaxOutputTokens != 384000 {
		t.Fatalf("deepseek context/output caps = %d/%d, want 1000000/384000", caps.MaxContextTokens, caps.MaxOutputTokens)
	}
	if caps.ExactTokenCount || caps.CacheEdit {
		t.Fatalf("deepseek must not claim OpenAI tokenizer/cache edit support: %#v", caps)
	}
	if !caps.SupportsToolCalls || !caps.SupportsStreaming || !caps.SupportsReasoning {
		t.Fatalf("deepseek capabilities missing tool/streaming/reasoning support: %#v", caps)
	}
	if !caps.NativeReasoning || caps.ReasoningEffortApplied != "max" {
		t.Fatalf("deepseek native reasoning metadata = %#v, want applied max", caps)
	}
	if caps.SupportsNativeWebTool {
		t.Fatalf("deepseek should keep custom web_search tool path, got native web capability: %#v", caps)
	}
}

func TestResolveModelCapabilitiesForZhipuGLM52(t *testing.T) {
	r := NewRouter("zhipu", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:        "zhipu",
		Model:           "glm-5.2",
		ReasoningEffort: "max",
	})

	if caps.Provider != "zhipu" || caps.Model != "glm-5.2" {
		t.Fatalf("capabilities route = %s/%s, want zhipu/glm-5.2", caps.Provider, caps.Model)
	}
	if caps.MaxContextTokens != 1000000 || caps.MaxOutputTokens != 128000 {
		t.Fatalf("glm-5.2 context/output caps = %d/%d, want 1000000/128000", caps.MaxContextTokens, caps.MaxOutputTokens)
	}
	if caps.ExactTokenCount || caps.CacheEdit {
		t.Fatalf("zhipu must not claim OpenAI tokenizer/cache edit support: %#v", caps)
	}
	if !caps.SupportsToolCalls || !caps.SupportsStreaming || !caps.SupportsReasoning {
		t.Fatalf("zhipu glm-5.2 capabilities missing tool/streaming/reasoning support: %#v", caps)
	}
	if !caps.NativeReasoning || caps.ReasoningEffortApplied != "max" {
		t.Fatalf("zhipu native reasoning metadata = %#v, want applied max", caps)
	}
	if !caps.SupportsNativeWebTool {
		t.Fatalf("zhipu should use provider-native web_search tool: %#v", caps)
	}
}

func TestModelRouterReportsReasoningCapability(t *testing.T) {
	r := NewRouter("openai", nil, nil)

	native := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:        "openai",
		Model:           "gpt-5.4",
		ReasoningEffort: "high",
	})
	if !boolCapabilityField(t, native, "NativeReasoning") {
		t.Fatalf("openai reasoning model NativeReasoning = false, want true: %#v", native)
	}
	if got := stringCapabilityField(t, native, "ReasoningEffortRequested"); got != "high" {
		t.Fatalf("openai reasoning requested effort = %q, want high", got)
	}
	if got := stringCapabilityField(t, native, "ReasoningEffortApplied"); got != "high" {
		t.Fatalf("openai reasoning applied effort = %q, want high", got)
	}
	if got := stringCapabilityField(t, native, "ReasoningFallbackPolicy"); got != "" {
		t.Fatalf("native reasoning route fallback policy = %q, want empty", got)
	}

	fallback := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:        "ollama",
		Model:           "llama3",
		ReasoningEffort: "high",
	})
	if boolCapabilityField(t, fallback, "NativeReasoning") {
		t.Fatalf("ollama NativeReasoning = true, want false: %#v", fallback)
	}
	if got := stringCapabilityField(t, fallback, "ReasoningEffortRequested"); got != "high" {
		t.Fatalf("unsupported provider requested effort = %q, want high", got)
	}
	if got := stringCapabilityField(t, fallback, "ReasoningEffortApplied"); got != "" {
		t.Fatalf("unsupported provider applied effort = %q, want empty", got)
	}
	policy := stringCapabilityField(t, fallback, "ReasoningFallbackPolicy")
	if strings.TrimSpace(policy) == "" {
		t.Fatalf("unsupported provider fallback policy is empty: %#v", fallback)
	}
	assertGenericReasoningFallbackPolicy(t, policy)
}

func TestResolveModelCapabilitiesForSmallContextModel(t *testing.T) {
	r := NewRouter("openai", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{Provider: "openai", Model: "ops-20k"})

	if caps.MaxContextTokens != 20000 {
		t.Fatalf("max context = %d, want 20000", caps.MaxContextTokens)
	}
	if !caps.SmallContextMode {
		t.Fatalf("expected small context mode: %#v", caps)
	}
}

func TestResolveModelCapabilitiesUsesManualContextWindowOverride(t *testing.T) {
	r := NewRouter("openai", nil, nil)

	caps := r.ResolveModelCapabilities(AgentKindWorker, ProviderConfig{
		Provider:         "openai",
		Model:            "gpt-5.4",
		MaxContextTokens: 9000,
	})

	if caps.MaxContextTokens != 10000 {
		t.Fatalf("max context = %d, want min-clamped 10000", caps.MaxContextTokens)
	}
	if !caps.SmallContextMode {
		t.Fatalf("expected manual 10K context to enable small context mode: %#v", caps)
	}
}

func boolCapabilityField(t *testing.T, caps ModelCapabilities, name string) bool {
	t.Helper()
	field := reflect.ValueOf(caps).FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("ModelCapabilities missing field %s", name)
	}
	if field.Kind() != reflect.Bool {
		t.Fatalf("ModelCapabilities.%s kind = %s, want bool", name, field.Kind())
	}
	return field.Bool()
}

func stringCapabilityField(t *testing.T, caps ModelCapabilities, name string) string {
	t.Helper()
	field := reflect.ValueOf(caps).FieldByName(name)
	if !field.IsValid() {
		t.Fatalf("ModelCapabilities missing field %s", name)
	}
	if field.Kind() != reflect.String {
		t.Fatalf("ModelCapabilities.%s kind = %s, want string", name, field.Kind())
	}
	return field.String()
}

func assertGenericReasoningFallbackPolicy(t *testing.T, policy string) {
	t.Helper()
	lower := strings.ToLower(policy)
	for _, want := range []string{
		"decompose the goal",
		"list assumptions",
		"gather evidence before conclusions",
		"cover key claims with evidence",
		"state the blocker",
	} {
		if !strings.Contains(lower, want) {
			t.Fatalf("fallback policy missing %q:\n%s", want, policy)
		}
	}
	for _, forbidden := range []string{
		"aiops",
		"rca",
		"incident",
		"host",
		"service",
		"pod",
		"kubernetes",
		"metric",
		"log",
		"alert",
		"monitoring",
		"coroot",
	} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("fallback policy contains domain term %q:\n%s", forbidden, policy)
		}
	}
}
