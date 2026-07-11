package appui

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"aiops-v2/internal/agentstate"
)

const (
	transportAgentItemPayloadByteBudget = 16 * 1024
	transportAgentItemsPerTurnBudget    = 128
	transportAgentItemsTurnByteBudget   = 512 * 1024
	transportAgentItemSummaryByteBudget = 4 * 1024
)

var (
	transportAgentBearerPattern     = regexp.MustCompile(`(?i)\bbearer\s+[a-z0-9._~+/=-]+`)
	transportAgentAssignmentPattern = regexp.MustCompile(`(?i)\b(authorization|api[_-]?key|access[_-]?token|refresh[_-]?token|token|password|passwd|secret|client[_-]?secret|private[_-]?key|cookie|credential)s?\s*[:=]\s*(?:bearer\s+)?[^\s,;]+`)
	transportAgentPrivateKeyPattern = regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
)

type transportAgentItemsProjection struct {
	Items         []AiopsTransportAgentItem
	Truncated     bool
	OriginalCount int
	OriginalBytes int64
	Hash          string
	Ref           string
}

type transportAgentItemProjection struct {
	Item          AiopsTransportAgentItem
	OriginalBytes int64
	SourceHash    string
}

func projectTransportAgentItems(items []agentstate.TurnItem) transportAgentItemsProjection {
	if items == nil {
		return transportAgentItemsProjection{}
	}
	projected := make([]transportAgentItemProjection, len(items))
	hasher := sha256.New()
	var originalBytes int64
	for i := range items {
		projected[i] = projectTransportAgentItem(items[i], false)
		originalBytes += projected[i].OriginalBytes
		_, _ = hasher.Write([]byte(projected[i].SourceHash + "\n"))
	}
	digest := fmt.Sprintf("sha256:%x", hasher.Sum(nil))
	indices := transportAgentItemIndices(len(items), transportAgentItemsPerTurnBudget)
	result := make([]AiopsTransportAgentItem, 0, len(indices))
	for _, index := range indices {
		result = append(result, projected[index].Item)
	}
	truncated := len(indices) != len(items)
	if transportAgentItemsJSONBytes(result) > transportAgentItemsTurnByteBudget {
		truncated = true
		result = result[:0]
		for _, index := range indices {
			result = append(result, projectTransportAgentItem(items[index], true).Item)
		}
	}
	for transportAgentItemsJSONBytes(result) > transportAgentItemsTurnByteBudget && len(indices) > 2 {
		truncated = true
		indices = transportAgentItemIndices(len(items), len(indices)-1)
		result = result[:0]
		for _, index := range indices {
			result = append(result, projectTransportAgentItem(items[index], true).Item)
		}
	}
	for _, item := range result {
		truncated = truncated || item.Truncated
	}
	out := transportAgentItemsProjection{Items: result}
	if truncated {
		out.Truncated = true
		out.OriginalCount = len(items)
		out.OriginalBytes = originalBytes
		out.Hash = digest
		out.Ref = "agent-items://" + strings.TrimPrefix(digest, "sha256:")
	}
	return out
}

func redactTransportProjectionTurnItems(items []agentstate.TurnItem) []agentstate.TurnItem {
	if items == nil {
		return nil
	}
	out := make([]agentstate.TurnItem, len(items))
	copy(out, items)
	for i := range out {
		out[i].Payload.Summary = redactTransportAgentText(out[i].Payload.Summary)
		if len(out[i].Payload.Data) == 0 {
			continue
		}
		value := redactTransportAgentValue(decodeTransportAgentPayload(out[i].Payload.Data))
		if raw, err := json.Marshal(value); err == nil {
			out[i].Payload.Data = raw
		}
	}
	return out
}

func projectTransportAgentItem(item agentstate.TurnItem, forceCompact bool) transportAgentItemProjection {
	summary := redactTransportAgentText(item.Payload.Summary)
	summaryTruncated := len(summary) > transportAgentItemSummaryByteBudget
	if summaryTruncated {
		summary = truncateTransportAgentText(summary, transportAgentItemSummaryByteBudget)
	}
	value := decodeTransportAgentPayload(item.Payload.Data)
	value = redactTransportAgentValue(value)
	fullData, _ := json.Marshal(value)
	if len(item.Payload.Data) == 0 {
		fullData = nil
	}
	source := AiopsTransportAgentItem{
		SchemaVersion: AiopsTransportAgentItemSchemaVersion,
		ID:            item.ID, Type: string(item.Type), Status: string(item.Status),
		Payload:   AiopsTransportAgentItemPayload{Kind: item.Payload.Kind, Summary: redactTransportAgentText(item.Payload.Summary), Data: fullData},
		CreatedAt: transportTimestamp(item.CreatedAt), UpdatedAt: transportTimestamp(item.UpdatedAt),
	}
	sourceJSON, _ := json.Marshal(source)
	digest := transportAgentContentHash(sourceJSON)
	truncated := forceCompact || summaryTruncated || len(fullData) > transportAgentItemPayloadByteBudget
	data := fullData
	if truncated && len(fullData) > 0 {
		data = compactTransportAgentPayload(value, digest, len(fullData))
	}
	projected := source
	projected.Payload.Summary = summary
	projected.Payload.Data = append(json.RawMessage(nil), data...)
	if truncated {
		projected.Truncated = true
		projected.OriginalBytes = int64(len(sourceJSON))
		projected.ContentHash = digest
		projected.Ref = "agent-item://" + strings.TrimPrefix(digest, "sha256:")
	}
	return transportAgentItemProjection{Item: projected, OriginalBytes: int64(len(sourceJSON)), SourceHash: digest}
}

func decodeTransportAgentPayload(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return redactTransportAgentText(string(raw))
	}
	return value
}

func redactTransportAgentValue(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, child := range typed {
			if transportAgentSensitiveKey(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = redactTransportAgentValue(child)
			}
		}
		return out
	case []any:
		out := make([]any, len(typed))
		for i := range typed {
			out[i] = redactTransportAgentValue(typed[i])
		}
		return out
	case string:
		return redactTransportAgentText(typed)
	default:
		return value
	}
}

func transportAgentSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.NewReplacer("_", "", "-", "", ".", "", " ", "").Replace(strings.TrimSpace(key)))
	if strings.Contains(normalized, "password") || strings.Contains(normalized, "passwd") || strings.Contains(normalized, "secret") || strings.Contains(normalized, "authorization") || strings.Contains(normalized, "credential") {
		return true
	}
	for _, suffix := range []string{"apikey", "accesstoken", "refreshtoken", "authtoken", "privatekey", "cookie"} {
		if normalized == suffix || strings.HasSuffix(normalized, suffix) {
			return true
		}
	}
	return normalized == "token" || normalized == "key"
}

func redactTransportAgentText(value string) string {
	value = transportAgentPrivateKeyPattern.ReplaceAllString(value, "[REDACTED]")
	value = transportAgentAssignmentPattern.ReplaceAllStringFunc(value, func(match string) string {
		separator := strings.IndexAny(match, ":=")
		if separator < 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(match[:separator]) + "=[REDACTED]"
	})
	return transportAgentBearerPattern.ReplaceAllString(value, "Bearer [REDACTED]")
}

func compactTransportAgentPayload(value any, digest string, originalBytes int) json.RawMessage {
	const compactMarker = "_transportTruncation"
	var compacted map[string]any
	if object, ok := value.(map[string]any); ok {
		compacted = map[string]any{}
		for _, key := range transportAgentEssentialPayloadKeys() {
			if child, exists := object[key]; exists {
				compacted[key] = compactTransportAgentValue(child, 0)
			}
		}
	} else {
		compacted = map[string]any{"value": compactTransportAgentValue(value, 0)}
	}
	compacted[compactMarker] = map[string]any{"truncated": true, "originalBytes": originalBytes, "contentHash": digest, "ref": "agent-item-payload://" + strings.TrimPrefix(digest, "sha256:")}
	raw, _ := json.Marshal(compacted)
	if len(raw) <= transportAgentItemPayloadByteBudget {
		return raw
	}
	marker, _ := json.Marshal(map[string]any{compactMarker: compacted[compactMarker]})
	return marker
}

func transportAgentEssentialPayloadKeys() []string {
	return []string{"promptFingerprint", "toolCallId", "toolName", "arguments", "approvalId", "approvalType", "evidenceId", "evidenceRefs", "refs", "finalContract", "displayKind", "phase", "streamState", "status", "risk", "rollback", "validation", "checkedEvidenceRefs", "approvedActions", "performedActions", "postChecks", "limitations", "failedToolImpacts", "error"}
}

func compactTransportAgentValue(value any, depth int) any {
	if depth >= 6 {
		return transportAgentTruncationValue(value)
	}
	switch typed := value.(type) {
	case string:
		if len(typed) > 512 {
			return map[string]any{"preview": truncateTransportAgentText(typed, 512), "truncated": true, "originalBytes": len(typed), "contentHash": transportAgentContentHash([]byte(typed))}
		}
		return typed
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		out := map[string]any{}
		for i, key := range keys {
			if i >= 16 {
				out["_truncatedKeys"] = len(keys) - i
				break
			}
			out[key] = compactTransportAgentValue(typed[key], depth+1)
		}
		return out
	case []any:
		if len(typed) <= 16 {
			out := make([]any, len(typed))
			for i := range typed {
				out[i] = compactTransportAgentValue(typed[i], depth+1)
			}
			return out
		}
		out := make([]any, 0, 17)
		for i := 0; i < 8; i++ {
			out = append(out, compactTransportAgentValue(typed[i], depth+1))
		}
		out = append(out, map[string]any{"truncatedItems": len(typed) - 16})
		for i := len(typed) - 8; i < len(typed); i++ {
			out = append(out, compactTransportAgentValue(typed[i], depth+1))
		}
		return out
	default:
		return value
	}
}

func transportAgentTruncationValue(value any) map[string]any {
	raw, _ := json.Marshal(value)
	return map[string]any{"truncated": true, "originalBytes": len(raw), "contentHash": transportAgentContentHash(raw)}
}

func truncateTransportAgentText(value string, budget int) string {
	if len(value) <= budget {
		return value
	}
	cut := budget
	for cut > 0 && !utf8.ValidString(value[:cut]) {
		cut--
	}
	return value[:cut] + "…[truncated]"
}

func transportAgentContentHash(value []byte) string {
	digest := sha256.Sum256(value)
	return fmt.Sprintf("sha256:%x", digest[:])
}

func transportAgentItemIndices(total, limit int) []int {
	if limit <= 0 || total <= 0 {
		return nil
	}
	if total <= limit {
		indices := make([]int, total)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	head := (limit + 1) / 2
	tail := limit - head
	indices := make([]int, 0, limit)
	for i := 0; i < head; i++ {
		indices = append(indices, i)
	}
	for i := total - tail; i < total; i++ {
		indices = append(indices, i)
	}
	return indices
}

func transportAgentItemsJSONBytes(items []AiopsTransportAgentItem) int {
	raw, err := json.Marshal(items)
	if err != nil {
		return transportAgentItemsTurnByteBudget + 1
	}
	return len(raw)
}
