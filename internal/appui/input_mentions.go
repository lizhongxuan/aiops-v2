package appui

import (
	"encoding/json"
	"net/url"
	"strings"

	"aiops-v2/internal/hostops"
)

const metadataInputMentionsV1 = "aiops.input.mentions.v1"

type inputMentionEnvelope struct {
	Version  int                   `json:"version"`
	Mentions []inputMentionBinding `json:"mentions"`
}

type inputMentionBinding struct {
	Version int                    `json:"version"`
	TokenID string                 `json:"tokenId"`
	Sigil   string                 `json:"sigil"`
	Display string                 `json:"display"`
	RawText string                 `json:"rawText"`
	Kind    string                 `json:"kind"`
	Path    string                 `json:"path"`
	Source  string                 `json:"source"`
	Range   inputMentionRange      `json:"range"`
	Payload map[string]interface{} `json:"payload"`
}

type inputMentionRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type inputMentionHostHint struct {
	TokenID     string
	Raw         string
	SpanStart   int
	SpanEnd     int
	HostID      string
	Address     string
	DisplayName string
	Source      string
}

type inputMentionResourceHint struct {
	TokenID string
	Raw     string
	Kind    string
	ID      string
	Title   string
}

type parsedInputMentions struct {
	Present      bool
	Invalid      bool
	Source       string
	Validation   string
	Hosts        []inputMentionHostHint
	Capabilities []string
	Resources    []inputMentionResourceHint
}

func (p parsedInputMentions) HasCapability(name string) bool {
	name = strings.TrimSpace(strings.ToLower(name))
	for _, capability := range p.Capabilities {
		if capability == name {
			return true
		}
	}
	return false
}

func parseInputMentions(input string, metadata map[string]string) parsedInputMentions {
	raw := strings.TrimSpace(metadata[metadataInputMentionsV1])
	if raw == "" {
		return parsedInputMentions{Source: "absent", Validation: "absent"}
	}
	parsed := parsedInputMentions{Present: true, Source: "structured", Validation: "confirmed"}
	var envelope inputMentionEnvelope
	if err := json.Unmarshal([]byte(raw), &envelope); err != nil || envelope.Version != 1 {
		parsed.Invalid = true
		parsed.Validation = "invalid"
		return parsed
	}
	for _, mention := range envelope.Mentions {
		startByte, endByte, ok := inputMentionTextMatchRange(input, mention)
		if !ok {
			parsed.Invalid = true
			continue
		}
		if !isStrongInputMentionSource(mention.Source) {
			if isWeakInputMentionSource(mention.Source) {
				continue
			}
			parsed.Invalid = true
			continue
		}
		switch strings.ToLower(strings.TrimSpace(mention.Kind)) {
		case "host":
			hostID, ok := decodeHostMentionPath(mention.Path)
			if !ok {
				parsed.Invalid = true
				continue
			}
			if payloadHostID := stringPayload(mention.Payload, "hostId"); payloadHostID != "" {
				hostID = payloadHostID
			}
			parsed.Hosts = append(parsed.Hosts, inputMentionHostHint{
				TokenID:     strings.TrimSpace(mention.TokenID),
				Raw:         firstNonEmptyString(strings.TrimSpace(mention.RawText), strings.TrimSpace(mention.Display)),
				SpanStart:   startByte,
				SpanEnd:     endByte,
				HostID:      hostID,
				Address:     firstNonEmptyString(stringPayload(mention.Payload, "address"), hostID),
				DisplayName: firstNonEmptyString(stringPayload(mention.Payload, "displayName"), hostID),
				Source:      strings.TrimSpace(mention.Source),
			})
		case "capability":
			capability, ok := decodeCapabilityMentionPath(mention.Path)
			if !ok {
				parsed.Invalid = true
				continue
			}
			parsed.Capabilities = appendUniqueInputMentionString(parsed.Capabilities, capability)
		case "ops_manual":
			manualID, ok := decodeResourceMentionPath(mention.Path, "ops-manual://")
			if !ok {
				parsed.Invalid = true
				continue
			}
			if payloadManualID := stringPayload(mention.Payload, "manualId"); payloadManualID != "" {
				manualID = payloadManualID
			}
			parsed.Capabilities = appendUniqueInputMentionString(parsed.Capabilities, "ops_manuals")
			parsed.Resources = append(parsed.Resources, inputMentionResourceHint{
				TokenID: strings.TrimSpace(mention.TokenID),
				Raw:     firstNonEmptyString(strings.TrimSpace(mention.RawText), strings.TrimSpace(mention.Display)),
				Kind:    "ops_manual",
				ID:      manualID,
				Title:   firstNonEmptyString(stringPayload(mention.Payload, "title"), manualID),
			})
		case "ops_graph":
			graphID, ok := decodeResourceMentionPath(mention.Path, "ops-graph://")
			if !ok {
				parsed.Invalid = true
				continue
			}
			if payloadGraphID := stringPayload(mention.Payload, "graphId"); payloadGraphID != "" {
				graphID = payloadGraphID
			}
			parsed.Capabilities = appendUniqueInputMentionString(parsed.Capabilities, "ops_graph")
			parsed.Resources = append(parsed.Resources, inputMentionResourceHint{
				TokenID: strings.TrimSpace(mention.TokenID),
				Raw:     firstNonEmptyString(strings.TrimSpace(mention.RawText), strings.TrimSpace(mention.Display)),
				Kind:    "ops_graph",
				ID:      graphID,
				Title:   firstNonEmptyString(stringPayload(mention.Payload, "name"), graphID),
			})
		default:
			parsed.Invalid = true
		}
	}
	if parsed.Invalid {
		parsed.Validation = "invalid"
	}
	return parsed
}

func inputMentionTextMatchRange(input string, mention inputMentionBinding) (int, int, bool) {
	if mention.Version != 1 || mention.Sigil != "@" {
		return 0, 0, false
	}
	startByte, endByte, ok := inputMentionUTF16RangeToByteOffsets(input, mention.Range.Start, mention.Range.End)
	if !ok {
		return 0, 0, false
	}
	return startByte, endByte, input[startByte:endByte] == mention.RawText
}

func inputMentionHostHintsToHostMentions(hints []inputMentionHostHint) []hostops.HostMention {
	mentions := make([]hostops.HostMention, 0, len(hints))
	for _, hint := range hints {
		mentions = append(mentions, hostops.HostMention{
			TokenID:     strings.TrimSpace(hint.TokenID),
			Raw:         strings.TrimSpace(hint.Raw),
			SpanStart:   hint.SpanStart,
			SpanEnd:     hint.SpanEnd,
			HostID:      strings.TrimSpace(hint.HostID),
			Address:     strings.TrimSpace(hint.Address),
			DisplayName: strings.TrimSpace(hint.DisplayName),
			Source:      hostops.HostMentionSourceInventory,
			Resolved:    false,
			Confidence:  0.75,
		})
	}
	return mentions
}

func inputMentionUTF16RangeToByteOffsets(input string, start, end int) (int, int, bool) {
	if start < 0 || end <= start {
		return 0, 0, false
	}
	unitIndex := 0
	startByte := -1
	endByte := -1
	for byteIndex, r := range input {
		if unitIndex == start {
			startByte = byteIndex
		}
		if unitIndex == end {
			endByte = byteIndex
			break
		}
		unitIndex += inputMentionUTF16Width(r)
		if startByte < 0 && unitIndex == start {
			startByte = byteIndex + len(string(r))
		}
		if unitIndex == end {
			endByte = byteIndex + len(string(r))
			break
		}
		if unitIndex > end {
			return 0, 0, false
		}
	}
	if startByte < 0 && unitIndex == start {
		startByte = len(input)
	}
	if endByte < 0 && unitIndex == end {
		endByte = len(input)
	}
	if startByte < 0 || endByte < 0 || endByte <= startByte {
		return 0, 0, false
	}
	return startByte, endByte, true
}

func inputMentionUTF16Width(r rune) int {
	if r > 0xFFFF {
		return 2
	}
	return 1
}

func decodeHostMentionPath(path string) (string, bool) {
	raw, ok := strings.CutPrefix(strings.TrimSpace(path), "host://")
	if !ok || raw == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw, true
	}
	return strings.TrimSpace(decoded), true
}

func decodeCapabilityMentionPath(path string) (string, bool) {
	raw, ok := strings.CutPrefix(strings.TrimSpace(path), "capability://")
	if !ok {
		return "", false
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "coroot":
		return "coroot", true
	case "ops_graph":
		return "ops_graph", true
	case "ops_manuals":
		return "ops_manuals", true
	default:
		return "", false
	}
}

func decodeResourceMentionPath(path, prefix string) (string, bool) {
	raw, ok := strings.CutPrefix(strings.TrimSpace(path), prefix)
	if !ok || raw == "" {
		return "", false
	}
	decoded, err := url.PathUnescape(raw)
	if err != nil {
		return raw, true
	}
	return strings.TrimSpace(decoded), true
}

func stringPayload(payload map[string]interface{}, key string) string {
	if payload == nil {
		return ""
	}
	value, ok := payload[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}

func isStrongInputMentionSource(source string) bool {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "selection", "history_restore":
		return true
	default:
		return false
	}
}

func isWeakInputMentionSource(source string) bool {
	return strings.EqualFold(strings.TrimSpace(source), "typed_fallback")
}

func appendUniqueInputMentionString(values []string, next string) []string {
	next = strings.TrimSpace(strings.ToLower(next))
	if next == "" {
		return values
	}
	for _, value := range values {
		if value == next {
			return values
		}
	}
	return append(values, next)
}
