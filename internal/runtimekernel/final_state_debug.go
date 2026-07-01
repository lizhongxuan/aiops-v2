package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"strings"
	"unicode/utf8"

	"aiops-v2/internal/agentstate"
)

func debugFinalStateLog(cfg RuntimeDebugConfig, sessionID, turnID string, iteration int, event string, snapshot *TurnSnapshot, fields map[string]any) {
	if !debugFinalStateEnabled(cfg) {
		return
	}
	payload := map[string]any{
		"event":     strings.TrimSpace(event),
		"session":   strings.TrimSpace(sessionID),
		"turn":      strings.TrimSpace(turnID),
		"iteration": iteration,
	}
	for key, value := range debugFinalStateSnapshot(snapshot) {
		payload[key] = value
	}
	for key, value := range fields {
		payload[key] = value
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		log.Printf("aiops.final_state event=%s session=%s turn=%s iteration=%d marshal_error=%v", event, sessionID, turnID, iteration, err)
		return
	}
	log.Printf("aiops.final_state %s", raw)
}

func debugFinalStateEnabled(cfg RuntimeDebugConfig) bool {
	return cfg.FinalState || cfg.TranscriptProjection
}

func debugFinalStateSnapshot(snapshot *TurnSnapshot) map[string]any {
	out := map[string]any{}
	if snapshot == nil {
		return out
	}
	out["lifecycle"] = string(snapshot.Lifecycle)
	out["resumeState"] = string(snapshot.ResumeState)
	out["finalChars"] = utf8.RuneCountInString(strings.TrimSpace(snapshot.FinalOutput))
	out["finalHash"] = debugTextHash(snapshot.FinalOutput)
	out["agentItems"] = len(snapshot.AgentItems)
	out["assistantMessageItems"] = countDebugAgentItems(snapshot.AgentItems, agentstate.TurnItemTypeAssistantMessage)
	out["assistantMessagePhases"] = debugAssistantMessagePhases(snapshot.AgentItems)
	return out
}

func countDebugAgentItems(items []agentstate.TurnItem, itemType agentstate.TurnItemType) int {
	count := 0
	for _, item := range items {
		if item.Type == itemType {
			count++
		}
	}
	return count
}

func debugAssistantMessagePhases(items []agentstate.TurnItem) map[string]int {
	out := map[string]int{}
	for _, item := range items {
		if item.Type != agentstate.TurnItemTypeAssistantMessage {
			continue
		}
		payload := agentItemPayloadMap(item)
		phase := strings.TrimSpace(anyString(payload["phase"]))
		if phase == "" {
			phase = "unknown"
		}
		out[phase]++
	}
	return out
}

func debugTextFacts(text string) map[string]any {
	text = strings.TrimSpace(text)
	return map[string]any{
		"textChars": utf8.RuneCountInString(text),
		"textHash":  debugTextHash(text),
	}
}

func debugAssistantMessageFacts(snapshot *TurnSnapshot, itemID string, text string, extra map[string]any) map[string]any {
	fields := debugTextFacts(text)
	fields["assistantMessageID"] = strings.TrimSpace(itemID)
	fields["assistantMessageHash"] = debugTextHash(text)
	if item, ok := debugFindAgentItem(snapshot, itemID); ok {
		payload := agentItemPayloadMap(item)
		fields["assistantMessageType"] = string(item.Type)
		fields["assistantMessageStatus"] = string(item.Status)
		fields["assistantMessagePhase"] = strings.TrimSpace(anyString(payload["phase"]))
		fields["assistantMessageStreamState"] = strings.TrimSpace(anyString(payload["streamState"]))
	} else {
		fields["assistantMessageType"] = ""
		fields["assistantMessageStatus"] = ""
		fields["assistantMessagePhase"] = ""
		fields["assistantMessageStreamState"] = ""
	}
	for key, value := range extra {
		fields[key] = value
	}
	return fields
}

func debugFindAgentItem(snapshot *TurnSnapshot, itemID string) (agentstate.TurnItem, bool) {
	if snapshot == nil {
		return agentstate.TurnItem{}, false
	}
	itemID = strings.TrimSpace(itemID)
	if itemID == "" {
		return agentstate.TurnItem{}, false
	}
	for _, item := range snapshot.AgentItems {
		if strings.TrimSpace(item.ID) == itemID {
			return item, true
		}
	}
	return agentstate.TurnItem{}, false
}

func debugTextHash(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:8])
}
