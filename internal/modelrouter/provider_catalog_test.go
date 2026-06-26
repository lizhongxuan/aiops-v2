package modelrouter

import "testing"

func TestProviderCatalogDefaultsForOpenAICompatibleProviders(t *testing.T) {
	openai, ok := ProviderPresetByID("openai")
	if !ok {
		t.Fatal("ProviderPresetByID(openai) ok=false")
	}
	if openai.ID != "openai" || openai.DefaultModel != "gpt-5.4" {
		t.Fatalf("openai preset = %+v, want openai/gpt-5.4", openai)
	}
	if openai.SupportsThinking || openai.DefaultThinkingType != "" {
		t.Fatalf("openai thinking defaults = %+v, want no provider-specific thinking", openai)
	}

	deepseek, ok := ProviderPresetByID("deepseek")
	if !ok {
		t.Fatal("ProviderPresetByID(deepseek) ok=false")
	}
	if deepseek.DefaultModel != "deepseek-v4-pro" {
		t.Fatalf("deepseek default model = %q, want deepseek-v4-pro", deepseek.DefaultModel)
	}
	if deepseek.DefaultBaseURL != "https://api.deepseek.com" {
		t.Fatalf("deepseek baseURL = %q, want official URL", deepseek.DefaultBaseURL)
	}
	if got := joinStrings(deepseek.ReasoningOptions); got != "high,max" || deepseek.DefaultReasoning != "high" {
		t.Fatalf("deepseek reasoning options/default = %q/%q, want high,max/high", got, deepseek.DefaultReasoning)
	}
	if deepseek.DefaultThinkingType != "enabled" {
		t.Fatalf("deepseek thinking default = %q, want enabled", deepseek.DefaultThinkingType)
	}

	zhipu, ok := ProviderPresetByID("zhipu")
	if !ok {
		t.Fatal("ProviderPresetByID(zhipu) ok=false")
	}
	if zhipu.DefaultModel != "glm-5.2" {
		t.Fatalf("zhipu default model = %q, want glm-5.2", zhipu.DefaultModel)
	}
	if zhipu.DefaultBaseURL != "https://open.bigmodel.cn/api/paas/v4/" {
		t.Fatalf("zhipu baseURL = %q, want official OpenAI-compatible URL", zhipu.DefaultBaseURL)
	}
	for _, want := range []string{"max", "xhigh", "high", "medium", "low", "minimal", "none"} {
		if !stringSliceContains(zhipu.ReasoningOptions, want) {
			t.Fatalf("zhipu reasoning options = %#v, missing %q", zhipu.ReasoningOptions, want)
		}
	}
	if zhipu.DefaultReasoning != "max" {
		t.Fatalf("zhipu default reasoning = %q, want max", zhipu.DefaultReasoning)
	}
	if zhipu.DefaultThinkingType != "enabled" {
		t.Fatalf("zhipu thinking default = %q, want enabled", zhipu.DefaultThinkingType)
	}
}

func TestProviderCatalogModelCaps(t *testing.T) {
	tests := []struct {
		provider string
		model    string
		context  int
		output   int
		defOut   int
	}{
		{"deepseek", "deepseek-v4-pro", 1000000, 384000, 20000},
		{"deepseek", "deepseek-v4-flash", 1000000, 384000, 20000},
		{"zhipu", "glm-5.2", 1000000, 128000, 20000},
		{"zhipu", "glm-4.7", 200000, 128000, 20000},
		{"zhipu", "glm-4.5-air", 128000, 96000, 16000},
	}

	for _, tt := range tests {
		t.Run(tt.provider+"/"+tt.model, func(t *testing.T) {
			preset, ok := ModelPresetByID(tt.provider, tt.model)
			if !ok {
				t.Fatalf("ModelPresetByID(%q, %q) ok=false", tt.provider, tt.model)
			}
			if preset.MaxContextTokens != tt.context || preset.MaxOutputTokens != tt.output || preset.DefaultMaxTokens != tt.defOut {
				t.Fatalf("preset = %+v, want context/output/default %d/%d/%d", preset, tt.context, tt.output, tt.defOut)
			}
		})
	}
	if glm52, ok := ModelPresetByID("zhipu", "glm-5.2"); !ok || glm52.DefaultTopP != 0.95 {
		t.Fatalf("glm-5.2 default top_p = %v/%v, want 0.95", glm52.DefaultTopP, ok)
	}
}

func TestProviderCatalogNormalizesProviderSpecificParameters(t *testing.T) {
	if got := NormalizeReasoningEffortForProvider("deepseek", "deepseek-v4-pro", "medium"); got != "high" {
		t.Fatalf("deepseek invalid reasoning normalized to %q, want high", got)
	}
	if got := NormalizeReasoningEffortForProvider("deepseek", "deepseek-v4-pro", "max"); got != "max" {
		t.Fatalf("deepseek max reasoning normalized to %q, want max", got)
	}
	if got := NormalizeReasoningEffortForProvider("zhipu", "glm-5.2", "xhigh"); got != "xhigh" {
		t.Fatalf("zhipu xhigh reasoning normalized to %q, want xhigh", got)
	}
	if got := NormalizeReasoningEffortForProvider("zhipu", "glm-5.2", "invalid"); got != "max" {
		t.Fatalf("zhipu invalid reasoning normalized to %q, want max", got)
	}
	if got := NormalizeThinkingType("deepseek", "disabled"); got != "disabled" {
		t.Fatalf("deepseek disabled thinking normalized to %q, want disabled", got)
	}
	if got := NormalizeThinkingType("openai", "enabled"); got != "" {
		t.Fatalf("openai thinking normalized to %q, want empty", got)
	}
}

func stringSliceContains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func joinStrings(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += "," + value
	}
	return out
}
