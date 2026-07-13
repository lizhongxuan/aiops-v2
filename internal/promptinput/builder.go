package promptinput

import (
	"fmt"
	"reflect"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

// Build converts compiled prompt fragments plus current-turn conversation
// context into provider-neutral model input items and a semantic trace.
func (Builder) Build(req BuildRequest) (BuildResult, error) {
	if hasPromptEnvelopeV2(req.Envelope) {
		return buildPromptInputV2(req, req.Envelope)
	}
	return buildLegacyPromptInput(req)
}

func buildLegacyPromptInput(req BuildRequest) (BuildResult, error) {
	promptItems := compiledPromptModelInputItems(req.Compiled)
	opsContextItems := opsContextModelInputItems(req)
	memoryItems := memoryModelInputItems(req)
	history := MessagesForCurrentTurnModelInput(req.History)
	runtimeItems, err := MessagesToModelInputItems(history)
	if err != nil {
		return BuildResult{}, fmt.Errorf("conversation messages: %w", err)
	}

	resultItems := make([]ModelInputItem, 0, len(promptItems)+len(opsContextItems)+len(memoryItems)+len(runtimeItems))
	resultItems = append(resultItems, promptItems...)
	resultItems = append(resultItems, opsContextItems...)
	resultItems = append(resultItems, memoryItems...)
	resultItems = append(resultItems, runtimeItems...)
	for i := range resultItems {
		if err := resultItems[i].Validate(); err != nil {
			return BuildResult{}, fmt.Errorf("model input item[%d]: %w", i, err)
		}
	}
	return BuildResult{
		Items: resultItems,
		Trace: buildTrace(req, resultItems, memoryMessagesFromRequest(req), history),
	}, nil
}

func buildPromptInputV2(req BuildRequest, envelope promptcompiler.PromptEnvelopeV2) (BuildResult, error) {
	if err := envelope.Validate(); err != nil {
		return BuildResult{}, fmt.Errorf("prompt envelope v2: %w", err)
	}
	if hasPromptEnvelopeV2(req.Compiled.EnvelopeV2) && !reflect.DeepEqual(envelope, req.Compiled.EnvelopeV2) {
		return BuildResult{}, fmt.Errorf("prompt envelope v2 does not match compiled envelope")
	}
	for _, section := range envelope.Sections {
		if section.LogicalLayer == promptcompiler.LayerConversationHistory || section.LogicalLayer == promptcompiler.LayerCurrentUserInput {
			return BuildResult{}, fmt.Errorf("prompt envelope v2 cannot own L4 or L6; use typed history/current input")
		}
	}
	currentUser := strings.TrimSpace(req.CurrentUserInput)
	continuation := strings.TrimSpace(req.ContinuationInstruction)
	if (currentUser == "") == (continuation == "") {
		return BuildResult{}, fmt.Errorf("model input requires exactly one current user input or continuation instruction")
	}
	if err := validateCurrentInputKind(req.Iteration, req.CurrentInputKind, currentUser, continuation); err != nil {
		return BuildResult{}, err
	}
	history := append([]Message(nil), req.History...)
	if req.CurrentInputKind == CurrentInputKindResumedUser {
		var removed bool
		history, removed = detachLatestUserMessage(history, currentUser)
		if !removed {
			return BuildResult{}, fmt.Errorf("current user input has no matching history message")
		}
		history = MessagesForCurrentTurnModelInput(history)
	} else {
		history = MessagesForCurrentTurnModelInput(history)
		if currentUser != "" {
			var removed bool
			history, removed = detachLatestUserMessage(history, currentUser)
			if !removed {
				return BuildResult{}, fmt.Errorf("current user input has no matching history message")
			}
		}
	}
	originalHistoryItems, err := MessagesToModelInputItems(history)
	if err != nil {
		return BuildResult{}, fmt.Errorf("conversation messages: %w", err)
	}
	rewriteModelInputLayer(originalHistoryItems, promptcompiler.LayerConversationHistory, "history", "conversation")
	if err := ValidateModelInputCausalOrder(originalHistoryItems); err != nil {
		return BuildResult{}, fmt.Errorf("conversation causal order: %w", err)
	}
	conversation, derivedContext := splitConversationAndDerivedContext(history)
	historyItems, err := MessagesToModelInputItems(conversation)
	if err != nil {
		return BuildResult{}, fmt.Errorf("conversation messages: %w", err)
	}
	rewriteModelInputLayer(historyItems, promptcompiler.LayerConversationHistory, "history", "conversation")
	derivedItems, err := MessagesToModelInputItems(derivedContext)
	if err != nil {
		return BuildResult{}, fmt.Errorf("derived context messages: %w", err)
	}
	rewriteModelInputLayer(derivedItems, promptcompiler.LayerStepDynamicContext, "context", "derived_context")

	stableItems := promptEnvelopeV2Items(envelope, false)
	dynamicItems := promptEnvelopeV2Items(envelope, true)
	opsItems := opsContextModelInputItems(req)
	rewriteModelInputLayer(opsItems, promptcompiler.LayerStepDynamicContext, "context", "ops_context")
	memoryItems := memoryModelInputItems(req)
	rewriteModelInputLayer(memoryItems, promptcompiler.LayerStepDynamicContext, "memory", "memory")

	resultItems := make([]ModelInputItem, 0, len(stableItems)+len(historyItems)+len(dynamicItems)+len(derivedItems)+len(opsItems)+len(memoryItems)+1)
	resultItems = append(resultItems, stableItems...)
	resultItems = append(resultItems, historyItems...)
	resultItems = append(resultItems, dynamicItems...)
	resultItems = append(resultItems, derivedItems...)
	resultItems = append(resultItems, opsItems...)
	resultItems = append(resultItems, memoryItems...)
	if currentUser != "" {
		resultItems = append(resultItems, currentInputModelItem("current-user-input", ProviderRoleUser, "current_user_input", currentUser, "conversation"))
	} else {
		resultItems = append(resultItems, currentInputModelItem("continuation-instruction", ProviderRoleDeveloper, "continuation_instruction", continuation, "runtime_continuation"))
	}
	for i := range resultItems {
		if err := resultItems[i].Validate(); err != nil {
			return BuildResult{}, fmt.Errorf("model input item[%d]: %w", i, err)
		}
	}
	if err := ValidateModelInputCausalOrder(resultItems); err != nil {
		return BuildResult{}, err
	}
	if err := ValidateModelInputLogicalOrder(resultItems, true); err != nil {
		return BuildResult{}, err
	}
	return BuildResult{
		Items: resultItems,
		Trace: buildTrace(req, resultItems, memoryMessagesFromRequest(req), history),
	}, nil
}

func hasPromptEnvelopeV2(envelope promptcompiler.PromptEnvelopeV2) bool {
	return strings.TrimSpace(envelope.SchemaVersion) != "" || len(envelope.Sections) > 0 || len(envelope.DynamicContext) > 0
}

func promptEnvelopeV2Items(envelope promptcompiler.PromptEnvelopeV2, dynamic bool) []ModelInputItem {
	var out []ModelInputItem
	for _, section := range envelope.Sections {
		isDynamic := section.LogicalLayer == promptcompiler.LayerStepDynamicContext
		if isDynamic != dynamic || section.LogicalLayer == promptcompiler.LayerConversationHistory || section.LogicalLayer == promptcompiler.LayerCurrentUserInput {
			continue
		}
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		role := ProviderRoleSystem
		if strings.EqualFold(strings.TrimSpace(section.Role), "developer") {
			role = ProviderRoleDeveloper
		}
		out = append(out, ModelInputItem{
			ID: section.ID, ProviderRole: role, SemanticRole: string(section.LogicalLayer), Content: content,
			Source: ModelInputSource{Layer: string(section.LogicalLayer), SectionID: section.ID, Origin: section.Source},
			Phase:  "prompt", CacheGroup: section.Stability,
			Metadata: map[string]string{"prompt_layer": string(section.LogicalLayer), "prompt_section_id": section.ID, "bundle_ref": section.BundleRef},
		})
	}
	return out
}

func detachLatestUserMessage(history []Message, currentUser string) ([]Message, bool) {
	for index := len(history) - 1; index >= 0; index-- {
		if strings.TrimSpace(history[index].Role) != "user" {
			continue
		}
		if strings.TrimSpace(history[index].Content) != strings.TrimSpace(currentUser) {
			return append([]Message(nil), history...), false
		}
		out := append([]Message(nil), history[:index]...)
		out = append(out, history[index+1:]...)
		return out, true
	}
	return append([]Message(nil), history...), false
}

func validateCurrentInputKind(iteration int, kind CurrentInputKind, currentUser, continuation string) error {
	if iteration < 0 {
		return fmt.Errorf("model input iteration must be non-negative")
	}
	if iteration == 0 {
		if kind != CurrentInputKindInitialUser || currentUser == "" || continuation != "" {
			return fmt.Errorf("iteration 0 requires typed initial user input")
		}
		return nil
	}
	if currentUser != "" {
		if kind != CurrentInputKindResumedUser || continuation != "" {
			return fmt.Errorf("iteration %d requires typed resumed user input", iteration)
		}
		return nil
	}
	if kind != CurrentInputKindContinuation || continuation == "" {
		return fmt.Errorf("iteration %d requires typed continuation instruction", iteration)
	}
	return nil
}

func splitConversationAndDerivedContext(history []Message) ([]Message, []Message) {
	conversation := make([]Message, 0, len(history))
	derived := make([]Message, 0)
	for _, message := range history {
		if strings.TrimSpace(message.Role) == "system" {
			derived = append(derived, message)
			continue
		}
		conversation = append(conversation, message)
	}
	return conversation, derived
}

func rewriteModelInputLayer(items []ModelInputItem, layer promptcompiler.PromptLogicalLayer, phase, origin string) {
	for index := range items {
		items[index].Source.Layer = string(layer)
		items[index].Source.Origin = origin
		items[index].Phase = phase
		items[index].CacheGroup = promptSectionKindForLayer(layer)
		if items[index].Metadata == nil {
			items[index].Metadata = map[string]string{}
		}
		items[index].Metadata["prompt_layer"] = string(layer)
	}
}

func promptSectionKindForLayer(layer promptcompiler.PromptLogicalLayer) string {
	if layer == promptcompiler.LayerStepDynamicContext || layer == promptcompiler.LayerConversationHistory || layer == promptcompiler.LayerCurrentUserInput {
		return promptcompiler.PromptSectionKindDynamic
	}
	return promptcompiler.PromptSectionKindStable
}

func currentInputModelItem(id string, role ProviderRole, semanticRole, content, origin string) ModelInputItem {
	return ModelInputItem{
		ID: id, ProviderRole: role, SemanticRole: semanticRole, Content: strings.TrimSpace(content),
		Source: ModelInputSource{Layer: string(promptcompiler.LayerCurrentUserInput), Origin: origin},
		Phase:  "current_input", CacheGroup: promptcompiler.PromptSectionKindDynamic,
		Metadata: map[string]string{"prompt_layer": string(promptcompiler.LayerCurrentUserInput)},
	}
}

func compiledPromptModelInputItems(compiled promptcompiler.CompiledPrompt) []ModelInputItem {
	sections := compiled.Envelope.Sections
	if len(sections) == 0 {
		sections = fallbackCompiledPromptSections(compiled)
	}
	out := make([]ModelInputItem, 0, len(sections))
	for _, section := range sections {
		content := strings.TrimSpace(section.Content)
		if content == "" {
			continue
		}
		role := ProviderRoleSystem
		if section.Role == "developer" {
			role = ProviderRoleDeveloper
		}
		layer := firstNonBlankPromptInputString(section.Layer, section.ID)
		out = append(out, ModelInputItem{
			ID:           firstNonBlankPromptInputString(section.ID, section.Layer, section.Source),
			ProviderRole: role,
			SemanticRole: firstNonBlankPromptInputString(layer, section.ID, section.Source),
			Content:      content,
			Source: ModelInputSource{
				Layer:     layer,
				SectionID: section.ID,
				Origin:    promptSource(layer),
			},
			Phase:      "prompt",
			CacheGroup: firstNonBlankPromptInputString(section.Stability, "stable"),
			Metadata: map[string]string{
				"prompt_layer":      layer,
				"prompt_section_id": section.ID,
			},
		})
	}
	return out
}

func fallbackCompiledPromptSections(compiled promptcompiler.CompiledPrompt) []promptcompiler.PromptCompiledSection {
	out := make([]promptcompiler.PromptCompiledSection, 0, 5)
	if content := strings.TrimSpace(compiled.System.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "system", Layer: "system", Role: "system", Content: content, Stability: "stable", Source: "system"})
	}
	if content := strings.TrimSpace(compiled.Developer.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "developer", Layer: "developer", Role: "developer", Content: content, Stability: "stable", Source: "developer"})
	}
	if content := strings.TrimSpace(compiled.Tools.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "tool_index", Layer: "tool_index", Role: "system", Content: content, Stability: "stable", Source: "tool"})
	}
	if content := strings.TrimSpace(compiled.Dynamic.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "dynamic_prompt", Layer: "dynamic_prompt", Role: "system", Content: content, Stability: "dynamic", Source: "runtime_context"})
	}
	if content := strings.TrimSpace(compiled.Policy.Content); content != "" {
		out = append(out, promptcompiler.PromptCompiledSection{ID: "runtime_policy", Layer: "runtime_policy", Role: "system", Content: content, Stability: "dynamic", Source: "context"})
	}
	return out
}

func opsContextModelInputItems(req BuildRequest) []ModelInputItem {
	capsule := strings.TrimSpace(req.OpsContextCapsule)
	if capsule == "" {
		return nil
	}
	return []ModelInputItem{{
		ID:           "ops-context-capsule",
		ProviderRole: ProviderRoleSystem,
		SemanticRole: "ops_context_capsule",
		Content:      "Ops context capsule:\n" + capsule,
		Source:       ModelInputSource{Layer: "ops_context_capsule", Origin: "ops_context"},
		Phase:        "context",
		CacheGroup:   "dynamic",
		Metadata:     map[string]string{"prompt_layer": "ops_context_capsule"},
	}}
}

func memoryModelInputItems(req BuildRequest) []ModelInputItem {
	memories := memoryMessagesFromRequest(req)
	out := make([]ModelInputItem, 0, len(memories))
	for _, item := range memories {
		out = append(out, ModelInputItem{
			ID:           "memory-" + item.ID,
			ProviderRole: ProviderRoleSystem,
			SemanticRole: "memory",
			Content:      "Memory: " + item.Text,
			Source:       ModelInputSource{Layer: "memory", MessageID: item.ID, Origin: "memory"},
			Phase:        "memory",
			CacheGroup:   "dynamic",
			Metadata:     map[string]string{"memory_id": item.ID, "memory_scope": item.Scope},
		})
	}
	return out
}

func memoryMessagesFromRequest(req BuildRequest) []MemoryItem {
	limit := req.MaxMemories
	if limit <= 0 || limit > 3 {
		limit = 3
	}
	if len(req.Memories) <= limit {
		return append([]MemoryItem(nil), req.Memories...)
	}
	return append([]MemoryItem(nil), req.Memories[:limit]...)
}

// MessagesForCurrentTurnModelInput preserves prior stable conversation messages
// while dropping old tool-call/result noise before the latest user turn.
func MessagesForCurrentTurnModelInput(history []Message) []Message {
	lastUserIndex := -1
	for i := len(history) - 1; i >= 0; i-- {
		if history[i].Role == "user" {
			lastUserIndex = i
			break
		}
	}
	if lastUserIndex <= 0 {
		return append([]Message(nil), history...)
	}

	out := make([]Message, 0, len(history))
	for i, msg := range history {
		if i >= lastUserIndex {
			out = append(out, msg)
			continue
		}
		if isStableConversationMessage(msg) {
			out = append(out, msg)
		}
	}
	return out
}

func isStableConversationMessage(msg Message) bool {
	switch msg.Role {
	case "system", "user":
		return strings.TrimSpace(msg.Content) != ""
	case "assistant":
		return len(msg.ToolCalls) == 0 && strings.TrimSpace(msg.Content) != ""
	default:
		return false
	}
}

func MessagesToModelInputItems(history []Message) ([]ModelInputItem, error) {
	out := make([]ModelInputItem, 0, len(history))
	for idx, msg := range history {
		item := ModelInputItem{
			ID:               fmt.Sprintf("history-%d", idx),
			ProviderRole:     providerRoleFromConversationRole(msg.Role),
			SemanticRole:     conversationSemanticRole(msg),
			Content:          msg.Content,
			ReasoningContent: msg.ReasoningContent,
			Source:           ModelInputSource{Layer: "history", Origin: "conversation"},
			Phase:            "history",
			CacheGroup:       "dynamic",
		}
		for _, call := range msg.ToolCalls {
			item.ToolCalls = append(item.ToolCalls, ModelInputToolCall{ID: call.ID, Name: call.Name, Arguments: call.Arguments})
		}
		if msg.ToolResult != nil {
			item.ProviderRole = ProviderRoleTool
			item.ToolCallID = msg.ToolResult.ToolCallID
			item.ToolResult = &ModelInputToolResult{ToolCallID: msg.ToolResult.ToolCallID, Content: msg.ToolResult.Content}
		}
		if err := item.Validate(); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, nil
}

func providerRoleFromConversationRole(role string) ProviderRole {
	switch strings.TrimSpace(role) {
	case "system":
		return ProviderRoleSystem
	case "assistant":
		return ProviderRoleAssistant
	case "tool":
		return ProviderRoleTool
	default:
		return ProviderRoleUser
	}
}

func firstNonBlankPromptInputString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return "model-input-item"
}
