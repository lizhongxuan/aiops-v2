package promptinput

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

func (r ProviderRole) IsValid() bool {
	switch r {
	case ProviderRoleSystem, ProviderRoleDeveloper, ProviderRoleUser, ProviderRoleAssistant, ProviderRoleTool:
		return true
	default:
		return false
	}
}

func (i ModelInputItem) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if !i.ProviderRole.IsValid() {
		return fmt.Errorf("provider role %q is invalid", i.ProviderRole)
	}
	if i.ProviderRole == ProviderRoleTool {
		if strings.TrimSpace(i.ToolCallID) == "" && strings.TrimSpace(i.ToolResultToolCallID()) == "" {
			return fmt.Errorf("tool result requires tool call id")
		}
	}
	for idx, call := range i.ToolCalls {
		if strings.TrimSpace(call.ID) == "" {
			return fmt.Errorf("tool call[%d] id is required", idx)
		}
		if strings.TrimSpace(call.Name) == "" {
			return fmt.Errorf("tool call[%d] name is required", idx)
		}
		if len(call.Arguments) > 0 && !json.Valid(call.Arguments) {
			return fmt.Errorf("tool call[%d] arguments must be valid json", idx)
		}
	}
	for idx, part := range i.ContentParts {
		if strings.TrimSpace(part.Type) == "" {
			return fmt.Errorf("content part[%d] type is required", idx)
		}
		if part.Type != "text" {
			return fmt.Errorf("content part[%d] type %q is unsupported", idx, part.Type)
		}
	}
	return nil
}

func (i ModelInputItem) StableHash() string {
	data, _ := json.Marshal(i)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func StableModelInputHash(items []ModelInputItem) string {
	data, _ := json.Marshal(items)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ValidateModelInputCausalOrder(items []ModelInputItem) error {
	seenCalls := map[string]bool{}
	seenResults := map[string]bool{}
	var pending []string
	for index, item := range items {
		if len(pending) > 0 && item.ProviderRole != ProviderRoleTool {
			return fmt.Errorf("model input item[%d] interrupts unresolved tool causal group", index)
		}
		if len(item.ToolCalls) > 0 {
			if item.ProviderRole != ProviderRoleAssistant {
				return fmt.Errorf("model input item[%d] tool calls require assistant role", index)
			}
			if _, typed := modelInputLogicalLayerRank(item.Source.Layer); typed && item.Source.Layer != string(promptcompiler.LayerConversationHistory) {
				return fmt.Errorf("model input item[%d] tool calls must be in L4", index)
			}
			for _, call := range item.ToolCalls {
				id := strings.TrimSpace(call.ID)
				if seenCalls[id] {
					return fmt.Errorf("model input item[%d] duplicates tool call id", index)
				}
				seenCalls[id] = true
				pending = append(pending, id)
			}
		}
		if item.ProviderRole != ProviderRoleTool && (item.ToolResult != nil || strings.TrimSpace(item.ToolCallID) != "") {
			return fmt.Errorf("model input item[%d] tool result requires tool role", index)
		}
		if item.ProviderRole != ProviderRoleTool {
			continue
		}
		if _, typed := modelInputLogicalLayerRank(item.Source.Layer); typed && item.Source.Layer != string(promptcompiler.LayerConversationHistory) {
			return fmt.Errorf("model input item[%d] tool result must be in L4", index)
		}
		id := strings.TrimSpace(firstNonBlankPromptInputString(item.ToolCallID, item.ToolResultToolCallID()))
		if item.ToolResult != nil && strings.TrimSpace(item.ToolCallID) != "" && strings.TrimSpace(item.ToolResult.ToolCallID) != "" && strings.TrimSpace(item.ToolCallID) != strings.TrimSpace(item.ToolResult.ToolCallID) {
			return fmt.Errorf("model input item[%d] tool result id mismatch", index)
		}
		if seenResults[id] {
			return fmt.Errorf("model input item[%d] duplicates tool result", index)
		}
		if len(pending) == 0 || pending[0] != id || !seenCalls[id] {
			return fmt.Errorf("model input item[%d] has orphan or out-of-order tool result", index)
		}
		seenResults[id] = true
		pending = pending[1:]
	}
	if len(pending) > 0 {
		return fmt.Errorf("model input has unresolved tool calls")
	}
	return nil
}

func ValidateModelInputLogicalOrder(items []ModelInputItem, requireComplete bool) error {
	previous := -1
	l6Count := 0
	layerCounts := [7]int{}
	for index, item := range items {
		rank, typed := modelInputLogicalLayerRank(item.Source.Layer)
		if !typed {
			if requireComplete {
				return fmt.Errorf("model input item[%d] lacks typed logical layer", index)
			}
			continue
		}
		if rank < previous {
			return fmt.Errorf("model input logical layers are out of order")
		}
		layerCounts[rank]++
		if rank == 6 {
			l6Count++
			if index != len(items)-1 {
				return fmt.Errorf("model input L6 must be last")
			}
		}
		previous = rank
	}
	if requireComplete {
		if len(items) < 3 || items[0].Source.Layer != string(promptcompiler.LayerAbsoluteSystemCore) || items[1].Source.Layer != string(promptcompiler.LayerRoleProfileCore) {
			return fmt.Errorf("model input must begin with L0 then L1")
		}
		if l6Count != 1 {
			return fmt.Errorf("model input requires exactly one L6 item")
		}
		for rank := 0; rank <= 3; rank++ {
			if layerCounts[rank] != 1 {
				return fmt.Errorf("model input requires exactly one L%d item", rank)
			}
		}
	}
	return nil
}

func modelInputLogicalLayerRank(layer string) (int, bool) {
	layers := []promptcompiler.PromptLogicalLayer{
		promptcompiler.LayerAbsoluteSystemCore, promptcompiler.LayerRoleProfileCore,
		promptcompiler.LayerStableRuntimeContract, promptcompiler.LayerTurnStableFacts,
		promptcompiler.LayerConversationHistory, promptcompiler.LayerStepDynamicContext,
		promptcompiler.LayerCurrentUserInput,
	}
	for index, candidate := range layers {
		if strings.TrimSpace(layer) == string(candidate) {
			return index, true
		}
	}
	return 0, false
}
