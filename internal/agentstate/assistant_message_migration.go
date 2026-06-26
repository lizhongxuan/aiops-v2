package agentstate

import (
	"encoding/json"
	"strings"
)

const (
	legacyAssistantProgressType TurnItemType = "assistant_progress"
	legacyAssistantAnswerType   TurnItemType = "assistant_answer"
	legacyFinalAnswerType       TurnItemType = "final_answer"
)

// MigrateLegacyAssistantItemsToAssistantMessage converts historical assistant
// text items to the single assistant_message shape. It is intended for offline
// migration only; runtime projection must not call it as a compatibility path.
func MigrateLegacyAssistantItemsToAssistantMessage(items []TurnItem, finalOutput string) []TurnItem {
	out := make([]TurnItem, 0, len(items))
	finalIndex := -1
	var legacyFinalSourceIDs []string
	for _, item := range items {
		switch item.Type {
		case legacyAssistantProgressType:
			if legacyBoolPayloadField(item, "candidateForFinal") {
				finalIndex = upsertMigratedFinalAssistantMessage(&out, finalIndex, item, finalOutput, legacyFinalSourceIDs)
				continue
			}
			out = append(out, migratedAssistantMessageItem(item, "commentary", "complete", nil))
		case legacyAssistantAnswerType:
			state := strings.ToLower(strings.TrimSpace(legacyStringPayloadField(item, "answerState")))
			if state == "superseded" {
				legacyFinalSourceIDs = append(legacyFinalSourceIDs, item.ID)
				if finalIndex >= 0 {
					out[finalIndex] = withLegacySourceIDs(out[finalIndex], legacyFinalSourceIDs)
				}
				continue
			}
			finalIndex = upsertMigratedFinalAssistantMessage(&out, finalIndex, item, finalOutput, legacyFinalSourceIDs)
		case legacyFinalAnswerType:
			finalIndex = upsertMigratedFinalAssistantMessage(&out, finalIndex, item, finalOutput, legacyFinalSourceIDs)
		default:
			out = append(out, item)
		}
	}
	if finalIndex >= 0 && strings.TrimSpace(finalOutput) != "" {
		out[finalIndex].Payload.Summary = strings.TrimSpace(finalOutput)
	}
	return out
}

func upsertMigratedFinalAssistantMessage(out *[]TurnItem, finalIndex int, item TurnItem, finalOutput string, legacySourceIDs []string) int {
	finalItem := migratedAssistantMessageItem(item, "final_answer", migratedStreamState(item), legacySourceIDs)
	if text := strings.TrimSpace(finalOutput); text != "" {
		finalItem.Payload.Summary = text
	}
	if finalIndex < 0 {
		*out = append(*out, finalItem)
		return len(*out) - 1
	}
	existing := (*out)[finalIndex]
	finalItem.ID = existing.ID
	finalItem.CreatedAt = existing.CreatedAt
	if finalItem.UpdatedAt.IsZero() {
		finalItem.UpdatedAt = existing.UpdatedAt
	}
	(*out)[finalIndex] = finalItem
	return finalIndex
}

func migratedAssistantMessageItem(item TurnItem, phase, streamState string, legacySourceIDs []string) TurnItem {
	item.Type = TurnItemTypeAssistantMessage
	item.Payload.Kind = "assistant_message"
	if streamState == "" {
		streamState = migratedStreamState(item)
	}
	payload := map[string]any{
		"displayKind": "assistant.message",
		"phase":       phase,
		"streamState": streamState,
	}
	if refs := compactMigrationStrings(append(legacySourceIDs, item.ID)); len(refs) > 0 && phase == "final_answer" {
		payload["legacySourceIds"] = refs
	}
	raw, _ := json.Marshal(payload)
	item.Payload.Data = raw
	return item
}

func withLegacySourceIDs(item TurnItem, legacySourceIDs []string) TurnItem {
	var payload map[string]any
	if len(item.Payload.Data) > 0 {
		_ = json.Unmarshal(item.Payload.Data, &payload)
	}
	if payload == nil {
		payload = map[string]any{}
	}
	if refs := compactMigrationStrings(legacySourceIDs); len(refs) > 0 {
		payload["legacySourceIds"] = refs
	}
	raw, _ := json.Marshal(payload)
	item.Payload.Data = raw
	return item
}

func migratedStreamState(item TurnItem) string {
	switch item.Status {
	case ItemStatusRunning:
		return "streaming"
	case ItemStatusFailed, ItemStatusCancelled:
		return "incomplete"
	default:
		return "complete"
	}
}

func legacyBoolPayloadField(item TurnItem, key string) bool {
	var payload map[string]any
	if len(item.Payload.Data) == 0 || json.Unmarshal(item.Payload.Data, &payload) != nil {
		return false
	}
	value, _ := payload[key].(bool)
	return value
}

func legacyStringPayloadField(item TurnItem, key string) string {
	var payload map[string]any
	if len(item.Payload.Data) == 0 || json.Unmarshal(item.Payload.Data, &payload) != nil {
		return ""
	}
	value, _ := payload[key].(string)
	return value
}

func compactMigrationStrings(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}
