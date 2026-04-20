package modelrouter

import (
	"context"
	"errors"
	"testing"

	"github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"

	"pgregory.net/rapid"
)

// Feature: aiops-codex-eino-rewrite, Property 16: ModelRouter Provider 选择
// Feature: aiops-codex-eino-rewrite, Property 17: ModelRouter Fallback 正确性

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// testModel is a mock ChatModel that records its provider name.
type testModel struct {
	provider string
}

func (m *testModel) Generate(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.Message, error) {
	return &schema.Message{Role: schema.Assistant, Content: "from " + m.provider}, nil
}

func (m *testModel) Stream(_ context.Context, _ []*schema.Message, _ ...model.Option) (*schema.StreamReader[*schema.Message], error) {
	return nil, nil
}

func (m *testModel) BindTools(_ []*schema.ToolInfo) error {
	return nil
}

// genAgentKind generates a random AgentKind.
func genAgentKind() *rapid.Generator[AgentKind] {
	return rapid.Custom(func(t *rapid.T) AgentKind {
		kinds := []AgentKind{AgentKindPlanner, AgentKindWorker}
		return kinds[rapid.IntRange(0, len(kinds)-1).Draw(t, "kindIdx")]
	})
}

// genProviderName generates a random provider name from a fixed set.
func genProviderName() *rapid.Generator[string] {
	return rapid.Custom(func(t *rapid.T) string {
		names := []string{"openai", "anthropic", "ollama", "azure", "local"}
		return names[rapid.IntRange(0, len(names)-1).Draw(t, "providerIdx")]
	})
}

// ---------------------------------------------------------------------------
// Property 16: ModelRouter Provider 选择
// For any provider routing configuration and LLM call request, ModelRouter
// should select the correct target provider based on config rules and AgentKind,
// returning the corresponding model.ChatModel instance.
// **Validates: Requirements 5.2, 5.4**
// ---------------------------------------------------------------------------

func TestProperty16_ProviderSelection_ExplicitConfig(t *testing.T) {
	// When ProviderConfig.Provider is set explicitly, it takes highest priority
	// regardless of AgentKind config or default.
	rapid.Check(t, func(t *rapid.T) {
		// Generate a set of available providers.
		providerNames := rapid.SliceOfN(genProviderName(), 1, 5).Draw(t, "providerNames")
		providers := make(map[string]ChatModel)
		for _, name := range providerNames {
			providers[name] = &testModel{provider: name}
		}

		// Pick a default and create router.
		defaultIdx := rapid.IntRange(0, len(providerNames)-1).Draw(t, "defaultIdx")
		defaultProvider := providerNames[defaultIdx]
		router := NewRouter(defaultProvider, providers, nil)

		// Set an AgentKind config (should be overridden by explicit provider).
		agentKind := genAgentKind().Draw(t, "agentKind")
		akConfigIdx := rapid.IntRange(0, len(providerNames)-1).Draw(t, "akConfigIdx")
		router.SetAgentKindConfig(agentKind, AgentKindConfig{
			Provider: providerNames[akConfigIdx],
		})

		// Pick an explicit provider from available ones.
		explicitIdx := rapid.IntRange(0, len(providerNames)-1).Draw(t, "explicitIdx")
		explicitProvider := providerNames[explicitIdx]

		m, err := router.GetModel(agentKind, ProviderConfig{Provider: explicitProvider})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		tm, ok := m.(*testModel)
		if !ok {
			t.Fatal("expected *testModel")
		}
		if tm.provider != explicitProvider {
			t.Fatalf("expected provider %q, got %q", explicitProvider, tm.provider)
		}
	})
}

func TestProperty16_ProviderSelection_AgentKindConfig(t *testing.T) {
	// When ProviderConfig.Provider is empty but AgentKind config is set,
	// the AgentKind config's provider is used.
	rapid.Check(t, func(t *rapid.T) {
		providerNames := rapid.SliceOfN(genProviderName(), 2, 5).Draw(t, "providerNames")
		providers := make(map[string]ChatModel)
		for _, name := range providerNames {
			providers[name] = &testModel{provider: name}
		}

		defaultProvider := providerNames[0]
		router := NewRouter(defaultProvider, providers, nil)

		// Set AgentKind config to a different provider than default.
		agentKind := genAgentKind().Draw(t, "agentKind")
		akIdx := rapid.IntRange(0, len(providerNames)-1).Draw(t, "akIdx")
		akProvider := providerNames[akIdx]
		router.SetAgentKindConfig(agentKind, AgentKindConfig{
			Provider: akProvider,
		})

		// Empty Provider in config → should use AgentKind config.
		m, err := router.GetModel(agentKind, ProviderConfig{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		tm := m.(*testModel)
		if tm.provider != akProvider {
			t.Fatalf("expected AgentKind provider %q, got %q", akProvider, tm.provider)
		}
	})
}

func TestProperty16_ProviderSelection_DefaultFallthrough(t *testing.T) {
	// When neither explicit provider nor AgentKind config is set,
	// the default provider is used.
	rapid.Check(t, func(t *rapid.T) {
		providerNames := rapid.SliceOfN(genProviderName(), 1, 5).Draw(t, "providerNames")
		providers := make(map[string]ChatModel)
		for _, name := range providerNames {
			providers[name] = &testModel{provider: name}
		}

		defaultIdx := rapid.IntRange(0, len(providerNames)-1).Draw(t, "defaultIdx")
		defaultProvider := providerNames[defaultIdx]
		router := NewRouter(defaultProvider, providers, nil)

		// No AgentKind config set, empty ProviderConfig.
		agentKind := genAgentKind().Draw(t, "agentKind")
		m, err := router.GetModel(agentKind, ProviderConfig{})
		if err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}

		tm := m.(*testModel)
		if tm.provider != defaultProvider {
			t.Fatalf("expected default provider %q, got %q", defaultProvider, tm.provider)
		}
	})
}

func TestProperty16_ProviderSelection_WorkerVsPlanner(t *testing.T) {
	// Worker and Planner can be configured with different providers.
	rapid.Check(t, func(t *rapid.T) {
		providers := map[string]ChatModel{
			"openai":    &testModel{provider: "openai"},
			"anthropic": &testModel{provider: "anthropic"},
			"ollama":    &testModel{provider: "ollama"},
		}
		router := NewRouter("openai", providers, nil)

		// Configure Worker with cheaper model, Planner with stronger.
		workerProvider := "ollama"
		plannerProvider := "anthropic"
		router.SetAgentKindConfig(AgentKindWorker, AgentKindConfig{Provider: workerProvider})
		router.SetAgentKindConfig(AgentKindPlanner, AgentKindConfig{Provider: plannerProvider})

		// Worker should get ollama.
		wModel, err := router.GetModel(AgentKindWorker, ProviderConfig{})
		if err != nil {
			t.Fatalf("worker: unexpected error: %v", err)
		}
		if wModel.(*testModel).provider != workerProvider {
			t.Fatalf("worker: expected %q, got %q", workerProvider, wModel.(*testModel).provider)
		}

		// Planner should get anthropic.
		pModel, err := router.GetModel(AgentKindPlanner, ProviderConfig{})
		if err != nil {
			t.Fatalf("planner: unexpected error: %v", err)
		}
		if pModel.(*testModel).provider != plannerProvider {
			t.Fatalf("planner: expected %q, got %q", plannerProvider, pModel.(*testModel).provider)
		}
	})
}

// ---------------------------------------------------------------------------
// Property 17: ModelRouter Fallback 正确性
// For any primary provider failure scenario, ModelRouter should automatically
// switch to the backup provider via the fallback chain; if all providers fail,
// it should return a structured error.
// **Validates: Requirements 5.3, 5.5**
// ---------------------------------------------------------------------------

func TestProperty17_Fallback_PrimaryUnavailable(t *testing.T) {
	// When the primary provider is not in the providers map but a fallback
	// is configured and available, the fallback provider is returned.
	rapid.Check(t, func(t *rapid.T) {
		// Available providers (subset of all possible).
		allNames := []string{"openai", "anthropic", "ollama", "azure", "local"}
		availableCount := rapid.IntRange(1, 3).Draw(t, "availableCount")
		available := allNames[:availableCount]

		providers := make(map[string]ChatModel)
		for _, name := range available {
			providers[name] = &testModel{provider: name}
		}

		// Pick a primary that is NOT available.
		unavailableNames := allNames[availableCount:]
		if len(unavailableNames) == 0 {
			return // skip if all are available
		}
		primaryIdx := rapid.IntRange(0, len(unavailableNames)-1).Draw(t, "primaryIdx")
		primary := unavailableNames[primaryIdx]

		// Pick a fallback that IS available.
		fallbackIdx := rapid.IntRange(0, len(available)-1).Draw(t, "fallbackIdx")
		fallback := available[fallbackIdx]

		fallbacks := []FallbackEntry{{Primary: primary, Fallback: fallback}}
		router := NewRouter(primary, providers, fallbacks)

		agentKind := genAgentKind().Draw(t, "agentKind")
		m, err := router.GetModel(agentKind, ProviderConfig{Provider: primary})
		if err != nil {
			t.Fatalf("expected fallback to succeed, got error: %v", err)
		}

		tm := m.(*testModel)
		if tm.provider != fallback {
			t.Fatalf("expected fallback provider %q, got %q", fallback, tm.provider)
		}
	})
}

func TestProperty17_Fallback_AllProvidersExhausted(t *testing.T) {
	// When neither the primary nor any fallback provider is available,
	// a ProviderNotFoundError is returned.
	rapid.Check(t, func(t *rapid.T) {
		// Create a router where the requested provider and its fallback
		// are both missing from the providers map.
		availableProvider := "ollama"
		providers := map[string]ChatModel{
			availableProvider: &testModel{provider: availableProvider},
		}

		// Primary and fallback are both unavailable.
		primary := "openai"
		fallbackProvider := "anthropic"
		fallbacks := []FallbackEntry{{Primary: primary, Fallback: fallbackProvider}}
		router := NewRouter(availableProvider, providers, fallbacks)

		agentKind := genAgentKind().Draw(t, "agentKind")
		_, err := router.GetModel(agentKind, ProviderConfig{Provider: primary})
		if err == nil {
			t.Fatal("expected ProviderNotFoundError when all providers exhausted")
		}

		var pnf *ProviderNotFoundError
		if !errors.As(err, &pnf) {
			t.Fatalf("expected ProviderNotFoundError, got %T: %v", err, err)
		}
		if pnf.Provider != primary {
			t.Fatalf("expected provider=%q in error, got %q", primary, pnf.Provider)
		}
	})
}

func TestProperty17_Fallback_ChainOrder(t *testing.T) {
	// The fallback chain is traversed in order; the first matching fallback
	// entry for the primary is used.
	rapid.Check(t, func(t *rapid.T) {
		providers := map[string]ChatModel{
			"anthropic": &testModel{provider: "anthropic"},
			"ollama":    &testModel{provider: "ollama"},
		}

		// Multiple fallback entries for the same primary - first match wins.
		fallbacks := []FallbackEntry{
			{Primary: "openai", Fallback: "anthropic"},
			{Primary: "openai", Fallback: "ollama"},
		}
		router := NewRouter("openai", providers, fallbacks)

		agentKind := genAgentKind().Draw(t, "agentKind")
		m, err := router.GetModel(agentKind, ProviderConfig{Provider: "openai"})
		if err != nil {
			t.Fatalf("expected fallback to succeed, got: %v", err)
		}

		// Should get the first fallback (anthropic), not the second (ollama).
		tm := m.(*testModel)
		if tm.provider != "anthropic" {
			t.Fatalf("expected first fallback 'anthropic', got %q", tm.provider)
		}
	})
}

func TestProperty17_Fallback_StructuredError(t *testing.T) {
	// When no provider is found, the error is a structured ProviderNotFoundError
	// containing the provider name that was requested.
	rapid.Check(t, func(t *rapid.T) {
		providerName := genProviderName().Draw(t, "requestedProvider")

		// Empty providers map - nothing available.
		router := NewRouter("nonexistent", map[string]ChatModel{}, nil)

		agentKind := genAgentKind().Draw(t, "agentKind")
		_, err := router.GetModel(agentKind, ProviderConfig{Provider: providerName})
		if err == nil {
			t.Fatal("expected error for unavailable provider")
		}

		var pnf *ProviderNotFoundError
		if !errors.As(err, &pnf) {
			t.Fatalf("expected ProviderNotFoundError, got %T: %v", err, err)
		}
		if pnf.Provider != providerName {
			t.Fatalf("error should reference provider %q, got %q", providerName, pnf.Provider)
		}

		// Verify the error message is well-formed.
		expectedMsg := "modelrouter: provider not found: " + providerName
		if pnf.Error() != expectedMsg {
			t.Fatalf("expected error message %q, got %q", expectedMsg, pnf.Error())
		}
	})
}

func TestProperty17_Fallback_AgentKindWithFallback(t *testing.T) {
	// When AgentKind config points to an unavailable provider, the fallback
	// chain is still applied correctly.
	rapid.Check(t, func(t *rapid.T) {
		providers := map[string]ChatModel{
			"ollama": &testModel{provider: "ollama"},
		}

		fallbacks := []FallbackEntry{
			{Primary: "anthropic", Fallback: "ollama"},
		}
		router := NewRouter("openai", providers, fallbacks)

		// Configure planner to use anthropic (not available), should fallback to ollama.
		router.SetAgentKindConfig(AgentKindPlanner, AgentKindConfig{Provider: "anthropic"})

		m, err := router.GetModel(AgentKindPlanner, ProviderConfig{})
		if err != nil {
			t.Fatalf("expected fallback to work with AgentKind config, got: %v", err)
		}

		tm := m.(*testModel)
		if tm.provider != "ollama" {
			t.Fatalf("expected fallback to 'ollama', got %q", tm.provider)
		}
	})
}
