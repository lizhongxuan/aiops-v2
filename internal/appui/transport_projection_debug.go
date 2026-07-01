package appui

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"unicode/utf8"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/runtimekernel"
)

func debugTransportProjectionLog(event string, turn *runtimekernel.TurnSnapshot, projected AiopsTransportTurn, fields map[string]any) {
	if !debugTransportProjectionEnabled() {
		return
	}
	payload := map[string]any{
		"event":            strings.TrimSpace(event),
		"turn":             "",
		"lifecycle":        "",
		"projectedProcess": len(projected.Process),
	}
	if turn != nil {
		payload["turn"] = strings.TrimSpace(turn.ID)
		payload["lifecycle"] = string(turn.Lifecycle)
		payload["resumeState"] = string(turn.ResumeState)
		payload["snapshotFinalChars"] = utf8.RuneCountInString(strings.TrimSpace(turn.FinalOutput))
		payload["snapshotFinalHash"] = debugTransportTextHash(turn.FinalOutput)
		payload["agentItems"] = len(turn.AgentItems)
		payload["agentItemTypes"] = debugTransportItemTypeCounts(turn.AgentItems)
		payload["assistantMessagePhases"] = debugTransportAssistantMessagePhases(turn.AgentItems)
	}
	for key, value := range fields {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf("aiops.transport_projection event=%s marshal_error=%v", event, err)
		return
	}
	log.Printf("aiops.transport_projection %s", raw)
}

func debugTransportProjectionEnabled() bool {
	return false
}

func debugTransportItemTypeCounts(items []agentstate.TurnItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		counts[string(item.Type)]++
	}
	return counts
}

func debugTransportAssistantMessagePhases(items []agentstate.TurnItem) map[string]int {
	counts := map[string]int{}
	for _, item := range items {
		if item.Type != agentstate.TurnItemTypeAssistantMessage {
			continue
		}
		phase := strings.TrimSpace(assistantMessageProjectionData(item).Phase)
		if phase == "" {
			phase = "unknown"
		}
		counts[phase]++
	}
	return counts
}

func debugTransportItemFacts(item agentstate.TurnItem) map[string]any {
	message := assistantMessageProjectionData(item)
	return map[string]any{
		"itemID":      strings.TrimSpace(item.ID),
		"itemType":    string(item.Type),
		"itemStatus":  string(item.Status),
		"itemChars":   utf8.RuneCountInString(strings.TrimSpace(item.Payload.Summary)),
		"itemHash":    debugTransportTextHash(item.Payload.Summary),
		"phase":       strings.TrimSpace(message.Phase),
		"displayKind": strings.TrimSpace(message.DisplayKind),
	}
}

func debugTransportTextHash(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}
