package promptinput

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

const PromptShadowParitySchemaVersion = "aiops.prompt-shadow-parity.v1"

const (
	PromptShadowLayerEqual         = "equal"
	PromptShadowLayerChanged       = "changed"
	PromptShadowLayerExpectedAdded = "expected_added"
)

type PromptShadowParityInput struct {
	LegacyEnvelope   promptcompiler.PromptEnvelope
	LegacyHistory    []Message
	CurrentInputKind CurrentInputKind
	CurrentUserInput string
	ContinuationKind string
	LegacyToolNames  []string
	V2ToolNames      []string
	LegacyPolicyHash string
	V2PolicyHash     string
	V2Items          []ModelInputItem
}

type PromptShadowLayerDiff struct {
	Layer                 promptcompiler.PromptLogicalLayer `json:"layer"`
	LegacyHash            string                            `json:"legacyHash,omitempty"`
	V2Hash                string                            `json:"v2Hash,omitempty"`
	LegacyItemCount       int                               `json:"legacyItemCount"`
	V2ItemCount           int                               `json:"v2ItemCount"`
	LegacySourceRefHashes []string                          `json:"legacySourceRefHashes,omitempty"`
	V2SourceRefHashes     []string                          `json:"v2SourceRefHashes,omitempty"`
	Status                string                            `json:"status"`
}

type PromptShadowCausalPair struct {
	CallIDHash    string `json:"callIdHash"`
	ToolName      string `json:"toolName"`
	ArgumentsHash string `json:"argumentsHash"`
	ResultHash    string `json:"resultHash"`
}

type PromptShadowControlFacts struct {
	ToolVisibilityHash  string                   `json:"toolVisibilityHash"`
	ToolCount           int                      `json:"toolCount"`
	PolicyHash          string                   `json:"policyHash"`
	CurrentInputKind    CurrentInputKind         `json:"currentInputKind"`
	CurrentUserHash     string                   `json:"currentUserHash,omitempty"`
	CurrentProviderRole ProviderRole             `json:"currentProviderRole"`
	CurrentSemanticRole string                   `json:"currentSemanticRole"`
	ContinuationKind    string                   `json:"continuationKind,omitempty"`
	CausalPairs         []PromptShadowCausalPair `json:"causalPairs"`
}

type PromptShadowParityReport struct {
	SchemaVersion     string                            `json:"schemaVersion"`
	LegacyModelHash   string                            `json:"legacyModelHash"`
	V2ModelHash       string                            `json:"v2ModelHash"`
	Layers            []PromptShadowLayerDiff           `json:"layers"`
	FirstChangedLayer promptcompiler.PromptLogicalLayer `json:"firstChangedLayer,omitempty"`
	LegacyFacts       PromptShadowControlFacts          `json:"legacyFacts"`
	V2Facts           PromptShadowControlFacts          `json:"v2Facts"`
	GateViolations    []string                          `json:"gateViolations"`
	Passed            bool                              `json:"passed"`
}

type promptShadowDigestItem struct {
	ProviderRole ProviderRole `json:"providerRole"`
	SemanticRole string       `json:"semanticRole,omitempty"`
	ContentHash  string       `json:"contentHash,omitempty"`
	CausalHash   string       `json:"causalHash,omitempty"`
	SourceType   string       `json:"sourceType,omitempty"`
}

func (report PromptShadowParityReport) Validate() error {
	if report.SchemaVersion != PromptShadowParitySchemaVersion {
		return fmt.Errorf("invalid prompt shadow parity schemaVersion")
	}
	if len(report.Layers) != len(promptShadowLogicalLayers()) {
		return fmt.Errorf("prompt shadow parity requires L0-L6 layer reports")
	}
	for index, layer := range report.Layers {
		if layer.Layer != promptShadowLogicalLayers()[index] {
			return fmt.Errorf("prompt shadow parity layer order mismatch")
		}
	}
	if strings.TrimSpace(report.LegacyModelHash) == "" || strings.TrimSpace(report.V2ModelHash) == "" {
		return fmt.Errorf("prompt shadow parity model hashes are required")
	}
	if report.Passed != (len(report.GateViolations) == 0) {
		return fmt.Errorf("prompt shadow parity gate status mismatch")
	}
	return nil
}

func BuildPromptShadowParity(input PromptShadowParityInput) (PromptShadowParityReport, error) {
	if err := ValidateModelInputLogicalOrder(input.V2Items, true); err != nil {
		return PromptShadowParityReport{}, fmt.Errorf("v2 logical order: %w", err)
	}
	if err := ValidateModelInputCausalOrder(input.V2Items); err != nil {
		return PromptShadowParityReport{}, fmt.Errorf("v2 causal order: %w", err)
	}
	legacyLayers, legacyRefs, legacyHistory, err := buildLegacyShadowLayers(input)
	if err != nil {
		return PromptShadowParityReport{}, err
	}
	v2Layers, v2Refs := buildV2ShadowLayers(input.V2Items)

	report := PromptShadowParityReport{SchemaVersion: PromptShadowParitySchemaVersion}
	for _, layer := range promptShadowLogicalLayers() {
		legacy := legacyLayers[layer]
		v2 := v2Layers[layer]
		diff := PromptShadowLayerDiff{
			Layer: layer, LegacyHash: shadowDigestHash(legacy), V2Hash: shadowDigestHash(v2),
			LegacyItemCount: len(legacy), V2ItemCount: len(v2),
			LegacySourceRefHashes: legacyRefs[layer], V2SourceRefHashes: v2Refs[layer],
			Status: PromptShadowLayerEqual,
		}
		if diff.LegacyHash != diff.V2Hash {
			diff.Status = PromptShadowLayerChanged
			if len(legacy) == 0 && (layer == promptcompiler.LayerAbsoluteSystemCore || layer == promptcompiler.LayerTurnStableFacts) {
				diff.Status = PromptShadowLayerExpectedAdded
			}
			if report.FirstChangedLayer == "" {
				report.FirstChangedLayer = layer
			}
		}
		report.Layers = append(report.Layers, diff)
	}
	report.LegacyModelHash = shadowHash(legacyLayers)
	report.V2ModelHash = shadowHash(v2Layers)
	report.LegacyFacts, err = buildLegacyShadowControlFacts(input, legacyHistory)
	if err != nil {
		return PromptShadowParityReport{}, err
	}
	report.V2Facts, err = buildV2ShadowControlFacts(input)
	if err != nil {
		return PromptShadowParityReport{}, err
	}
	if report.LegacyFacts.ToolVisibilityHash != report.V2Facts.ToolVisibilityHash || report.LegacyFacts.ToolCount != report.V2Facts.ToolCount {
		report.GateViolations = append(report.GateViolations, "tool_visibility_drift")
	}
	if report.LegacyFacts.PolicyHash != report.V2Facts.PolicyHash {
		report.GateViolations = append(report.GateViolations, "policy_drift")
	}
	if report.LegacyFacts.CurrentInputKind != report.V2Facts.CurrentInputKind ||
		report.LegacyFacts.CurrentUserHash != report.V2Facts.CurrentUserHash ||
		report.LegacyFacts.CurrentProviderRole != report.V2Facts.CurrentProviderRole ||
		report.LegacyFacts.CurrentSemanticRole != report.V2Facts.CurrentSemanticRole ||
		report.LegacyFacts.ContinuationKind != report.V2Facts.ContinuationKind {
		report.GateViolations = append(report.GateViolations, "current_input_semantic_drift")
	}
	if !reflect.DeepEqual(report.LegacyFacts.CausalPairs, report.V2Facts.CausalPairs) {
		report.GateViolations = append(report.GateViolations, "causal_pair_drift")
	}
	report.Passed = len(report.GateViolations) == 0
	return report, nil
}

func buildLegacyShadowLayers(input PromptShadowParityInput) (map[promptcompiler.PromptLogicalLayer][]promptShadowDigestItem, map[promptcompiler.PromptLogicalLayer][]string, []ModelInputItem, error) {
	layers := map[promptcompiler.PromptLogicalLayer][]promptShadowDigestItem{}
	refs := map[promptcompiler.PromptLogicalLayer][]string{}
	for _, section := range input.LegacyEnvelope.Sections {
		layer := section.LogicalLayer
		if !isPromptShadowLogicalLayer(layer) || strings.TrimSpace(section.Content) == "" {
			continue
		}
		layers[layer] = append(layers[layer], promptShadowDigestItem{
			ProviderRole: promptShadowProviderRole(section.Role), SemanticRole: string(layer),
			ContentHash: shadowHash(strings.TrimSpace(section.Content)), SourceType: strings.TrimSpace(section.Source),
		})
		if ref := strings.TrimSpace(section.BundleRef); ref != "" {
			refs[layer] = append(refs[layer], shadowHash(ref))
		}
	}
	history := append([]Message(nil), input.LegacyHistory...)
	var err error
	history, err = causalHistoryWithoutBoundDerivedContext(history)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("legacy derived context: %w", err)
	}
	if strings.TrimSpace(input.CurrentUserInput) != "" {
		var removed bool
		history, removed = detachLatestUserMessage(history, input.CurrentUserInput)
		if !removed {
			return nil, nil, nil, fmt.Errorf("legacy current user input has no matching history message")
		}
	}
	history = MessagesForCurrentTurnModelInput(history)
	historyItems, err := MessagesToModelInputItems(history)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("legacy history: %w", err)
	}
	for _, item := range historyItems {
		layers[promptcompiler.LayerConversationHistory] = append(layers[promptcompiler.LayerConversationHistory], shadowDigestModelInputItem(item))
	}
	if user := strings.TrimSpace(input.CurrentUserInput); user != "" {
		layers[promptcompiler.LayerCurrentUserInput] = append(layers[promptcompiler.LayerCurrentUserInput], promptShadowDigestItem{ProviderRole: ProviderRoleUser, SemanticRole: "current_user_input", ContentHash: shadowHash(user)})
	} else {
		layers[promptcompiler.LayerCurrentUserInput] = append(layers[promptcompiler.LayerCurrentUserInput], promptShadowDigestItem{ProviderRole: ProviderRoleDeveloper, SemanticRole: "continuation_instruction", ContentHash: shadowHash(strings.TrimSpace(input.ContinuationKind))})
	}
	return layers, refs, historyItems, nil
}

func buildV2ShadowLayers(items []ModelInputItem) (map[promptcompiler.PromptLogicalLayer][]promptShadowDigestItem, map[promptcompiler.PromptLogicalLayer][]string) {
	layers := map[promptcompiler.PromptLogicalLayer][]promptShadowDigestItem{}
	refs := map[promptcompiler.PromptLogicalLayer][]string{}
	for _, item := range items {
		layer := promptcompiler.PromptLogicalLayer(strings.TrimSpace(item.Source.Layer))
		if !isPromptShadowLogicalLayer(layer) {
			continue
		}
		layers[layer] = append(layers[layer], shadowDigestModelInputItem(item))
		if ref := strings.TrimSpace(item.Metadata["bundle_ref"]); ref != "" {
			refs[layer] = append(refs[layer], shadowHash(ref))
		}
	}
	return layers, refs
}

func buildLegacyShadowControlFacts(input PromptShadowParityInput, history []ModelInputItem) (PromptShadowControlFacts, error) {
	facts := promptShadowBaseFacts(input.LegacyToolNames, input.LegacyPolicyHash, input.CurrentInputKind, input.ContinuationKind)
	if strings.TrimSpace(input.CurrentUserInput) != "" {
		facts.CurrentUserHash = shadowHash(strings.TrimSpace(input.CurrentUserInput))
		facts.CurrentProviderRole = ProviderRoleUser
		facts.CurrentSemanticRole = "current_user_input"
	} else {
		facts.CurrentProviderRole = ProviderRoleDeveloper
		facts.CurrentSemanticRole = "continuation_instruction"
	}
	var err error
	facts.CausalPairs, err = promptShadowCausalPairs(history)
	return facts, err
}

func buildV2ShadowControlFacts(input PromptShadowParityInput) (PromptShadowControlFacts, error) {
	facts := promptShadowBaseFacts(input.V2ToolNames, input.V2PolicyHash, input.CurrentInputKind, input.ContinuationKind)
	var current *ModelInputItem
	for index := range input.V2Items {
		if input.V2Items[index].Source.Layer == string(promptcompiler.LayerCurrentUserInput) {
			current = &input.V2Items[index]
		}
	}
	if current == nil {
		return PromptShadowControlFacts{}, fmt.Errorf("v2 current input is missing")
	}
	facts.CurrentProviderRole = current.ProviderRole
	facts.CurrentSemanticRole = strings.TrimSpace(current.SemanticRole)
	if current.ProviderRole == ProviderRoleUser {
		facts.CurrentUserHash = shadowHash(strings.TrimSpace(current.Content))
	} else {
		facts.CurrentInputKind = CurrentInputKindContinuation
	}
	var err error
	facts.CausalPairs, err = promptShadowCausalPairs(input.V2Items)
	return facts, err
}

func promptShadowBaseFacts(tools []string, policyHash string, kind CurrentInputKind, continuationKind string) PromptShadowControlFacts {
	normalizedTools := make([]string, 0, len(tools))
	for _, name := range tools {
		if name = strings.TrimSpace(name); name != "" {
			normalizedTools = append(normalizedTools, name)
		}
	}
	sort.Strings(normalizedTools)
	return PromptShadowControlFacts{
		ToolVisibilityHash: shadowHash(normalizedTools), ToolCount: len(normalizedTools), PolicyHash: strings.TrimSpace(policyHash),
		CurrentInputKind: kind, ContinuationKind: strings.TrimSpace(continuationKind), CausalPairs: []PromptShadowCausalPair{},
	}
}

func promptShadowCausalPairs(items []ModelInputItem) ([]PromptShadowCausalPair, error) {
	if err := ValidateModelInputCausalOrder(items); err != nil {
		return nil, fmt.Errorf("shadow causal pairs: %w", err)
	}
	pairs := make([]PromptShadowCausalPair, 0)
	indexes := map[string]int{}
	for _, item := range items {
		for _, call := range item.ToolCalls {
			id := strings.TrimSpace(call.ID)
			indexes[id] = len(pairs)
			pairs = append(pairs, PromptShadowCausalPair{CallIDHash: shadowHash(id), ToolName: strings.TrimSpace(call.Name), ArgumentsHash: shadowJSONHash(call.Arguments)})
		}
		if item.ProviderRole == ProviderRoleTool {
			id := strings.TrimSpace(firstNonBlankPromptInputString(item.ToolCallID, item.ToolResultToolCallID()))
			if index, ok := indexes[id]; ok {
				content := item.Content
				if item.ToolResult != nil {
					content = item.ToolResult.Content
				}
				pairs[index].ResultHash = shadowHash(strings.TrimSpace(content))
			}
		}
	}
	return pairs, nil
}

func shadowDigestModelInputItem(item ModelInputItem) promptShadowDigestItem {
	return promptShadowDigestItem{
		ProviderRole: item.ProviderRole, SemanticRole: strings.TrimSpace(item.SemanticRole),
		ContentHash: shadowHash(strings.TrimSpace(item.Content)), CausalHash: shadowHash(struct {
			ToolCalls  []ModelInputToolCall  `json:"toolCalls,omitempty"`
			ToolCallID string                `json:"toolCallId,omitempty"`
			ToolResult *ModelInputToolResult `json:"toolResult,omitempty"`
		}{item.ToolCalls, item.ToolCallID, item.ToolResult}), SourceType: strings.TrimSpace(item.Source.Origin),
	}
}

func shadowDigestHash(items []promptShadowDigestItem) string {
	if len(items) == 0 {
		return ""
	}
	return shadowHash(items)
}

func shadowJSONHash(raw json.RawMessage) string {
	if len(raw) == 0 {
		return shadowHash(nil)
	}
	var value any
	if json.Unmarshal(raw, &value) == nil {
		return shadowHash(value)
	}
	return shadowHash(string(raw))
}

func shadowHash(value any) string {
	raw, _ := json.Marshal(value)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func promptShadowProviderRole(role string) ProviderRole {
	if strings.EqualFold(strings.TrimSpace(role), "developer") {
		return ProviderRoleDeveloper
	}
	return ProviderRoleSystem
}

func promptShadowLogicalLayers() []promptcompiler.PromptLogicalLayer {
	return []promptcompiler.PromptLogicalLayer{
		promptcompiler.LayerAbsoluteSystemCore, promptcompiler.LayerRoleProfileCore, promptcompiler.LayerStableRuntimeContract,
		promptcompiler.LayerTurnStableFacts, promptcompiler.LayerConversationHistory, promptcompiler.LayerStepDynamicContext, promptcompiler.LayerCurrentUserInput,
	}
}

func isPromptShadowLogicalLayer(layer promptcompiler.PromptLogicalLayer) bool {
	for _, candidate := range promptShadowLogicalLayers() {
		if layer == candidate {
			return true
		}
	}
	return false
}
