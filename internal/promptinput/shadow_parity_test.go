package promptinput

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestPromptShadowParityLocatesLayersAndAllowsExpectedStructureChanges(t *testing.T) {
	report, err := BuildPromptShadowParity(promptShadowParityInputForTest())
	if err != nil {
		t.Fatalf("BuildPromptShadowParity() error = %v", err)
	}
	if !report.Passed || len(report.GateViolations) != 0 {
		t.Fatalf("report = %#v, want control parity", report)
	}
	if report.FirstChangedLayer != promptcompiler.LayerAbsoluteSystemCore {
		t.Fatalf("FirstChangedLayer = %q", report.FirstChangedLayer)
	}
	statuses := map[promptcompiler.PromptLogicalLayer]string{}
	for _, layer := range report.Layers {
		statuses[layer.Layer] = layer.Status
	}
	if statuses[promptcompiler.LayerAbsoluteSystemCore] != PromptShadowLayerExpectedAdded || statuses[promptcompiler.LayerTurnStableFacts] != PromptShadowLayerExpectedAdded {
		t.Fatalf("layer statuses = %#v, want expected L0/L3 additions", statuses)
	}
	if len(report.LegacyFacts.CausalPairs) != 1 || len(report.V2Facts.CausalPairs) != 1 || report.LegacyFacts.CausalPairs[0] != report.V2Facts.CausalPairs[0] {
		t.Fatalf("causal parity = legacy %#v v2 %#v", report.LegacyFacts.CausalPairs, report.V2Facts.CausalPairs)
	}
}

func TestPromptShadowParityRejectsControlFactDrift(t *testing.T) {
	input := promptShadowParityInputForTest()
	input.V2ToolNames = []string{"different_tool"}
	input.V2Items[len(input.V2Items)-1].Content = "different user"
	input.V2Items[4].ToolCalls[0].Arguments = json.RawMessage(`{"host":"different"}`)
	report, err := BuildPromptShadowParity(input)
	if err != nil {
		t.Fatalf("BuildPromptShadowParity() error = %v", err)
	}
	if report.Passed {
		t.Fatalf("report = %#v, want drift rejection", report)
	}
	joined := strings.Join(report.GateViolations, ",")
	for _, want := range []string{"tool_visibility_drift", "current_input_semantic_drift", "causal_pair_drift"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("GateViolations = %#v, missing %q", report.GateViolations, want)
		}
	}
}

func TestPromptShadowParityReportContainsHashesAndTypedFactsOnly(t *testing.T) {
	report, err := BuildPromptShadowParity(promptShadowParityInputForTest())
	if err != nil {
		t.Fatalf("BuildPromptShadowParity() error = %v", err)
	}
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatalf("json.Marshal() error = %v", err)
	}
	for _, secret := range []string{"secret-user-text", "secret-result-text", "source://secret-ref", "secret-contract-text", `{"host":"secret-host"}`} {
		if strings.Contains(string(raw), secret) {
			t.Fatalf("report leaked %q: %s", secret, raw)
		}
	}
	if len(report.Layers[5].V2SourceRefHashes) == 0 || strings.TrimSpace(report.Layers[5].V2SourceRefHashes[0]) == "" {
		t.Fatalf("L5 source refs were not hashed: %#v", report.Layers[5])
	}
}

func promptShadowParityInputForTest() PromptShadowParityInput {
	legacyEnvelope := promptcompiler.PromptEnvelope{Sections: []promptcompiler.PromptCompiledSection{
		{ID: "legacy-contract", LogicalLayer: promptcompiler.LayerStableRuntimeContract, Content: "secret-contract-text", Source: "runtime", BundleRef: "source://secret-ref"},
	}}
	history := []Message{
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-secret", Name: "host_read", Arguments: json.RawMessage(`{"host":"secret-host"}`)}}},
		{Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-secret", Content: "secret-result-text"}},
		{Role: "user", Content: "secret-user-text"},
	}
	items := []ModelInputItem{
		shadowLayerItem("l0", promptcompiler.LayerAbsoluteSystemCore, ProviderRoleSystem, "system"),
		shadowLayerItem("l1", promptcompiler.LayerRoleProfileCore, ProviderRoleSystem, "role"),
		shadowLayerItem("l2", promptcompiler.LayerStableRuntimeContract, ProviderRoleSystem, "secret-contract-text"),
		shadowLayerItem("l3", promptcompiler.LayerTurnStableFacts, ProviderRoleSystem, "turn"),
		{ID: "assistant-call", ProviderRole: ProviderRoleAssistant, SemanticRole: "assistant", ToolCalls: []ModelInputToolCall{{ID: "call-secret", Name: "host_read", Arguments: json.RawMessage(`{"host":"secret-host"}`)}}, Source: ModelInputSource{Layer: string(promptcompiler.LayerConversationHistory)}, Phase: "history", CacheGroup: "dynamic"},
		{ID: "tool-result", ProviderRole: ProviderRoleTool, SemanticRole: "tool", ToolCallID: "call-secret", ToolResult: &ModelInputToolResult{ToolCallID: "call-secret", Content: "secret-result-text"}, Source: ModelInputSource{Layer: string(promptcompiler.LayerConversationHistory)}, Phase: "history", CacheGroup: "dynamic"},
		{ID: "l5", ProviderRole: ProviderRoleSystem, SemanticRole: string(promptcompiler.LayerStepDynamicContext), Content: "retrieved", Source: ModelInputSource{Layer: string(promptcompiler.LayerStepDynamicContext)}, Phase: "context", CacheGroup: "dynamic", Metadata: map[string]string{"bundle_ref": "source://secret-ref"}},
		{ID: "l6", ProviderRole: ProviderRoleUser, SemanticRole: "current_user_input", Content: "secret-user-text", Source: ModelInputSource{Layer: string(promptcompiler.LayerCurrentUserInput)}, Phase: "current_input", CacheGroup: "dynamic"},
	}
	return PromptShadowParityInput{
		LegacyEnvelope:   legacyEnvelope,
		LegacyHistory:    history,
		CurrentInputKind: CurrentInputKindInitialUser,
		CurrentUserInput: "secret-user-text",
		ContinuationKind: "initial_user",
		LegacyToolNames:  []string{"host_read"},
		V2ToolNames:      []string{"host_read"},
		LegacyPolicyHash: "policy-hash",
		V2PolicyHash:     "policy-hash",
		V2Items:          items,
	}
}

func shadowLayerItem(id string, layer promptcompiler.PromptLogicalLayer, role ProviderRole, content string) ModelInputItem {
	return ModelInputItem{ID: id, ProviderRole: role, SemanticRole: string(layer), Content: content, Source: ModelInputSource{Layer: string(layer)}, Phase: "prompt", CacheGroup: "stable"}
}
