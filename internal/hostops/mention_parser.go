package hostops

import (
	"crypto/sha1"
	"encoding/hex"
	"net"
	"sort"
	"strings"
	"unicode"

	"aiops-v2/internal/resourcebinding"
)

// ParseHostMentions extracts @host references for UX and orchestration routing.
// It is intentionally conservative: only ASCII host-like tokens are accepted,
// and email addresses are skipped when @ is preceded by an identifier byte.
func ParseHostMentions(input string) []HostMention {
	if input == "" {
		return nil
	}
	mentions := make([]HostMention, 0)
	for i := 0; i < len(input); i++ {
		if input[i] != '@' {
			continue
		}
		if i > 0 && isEmailLocalPartByte(input[i-1]) {
			continue
		}
		start := i
		i++
		tokenStart := i
		for i < len(input) && isMentionTokenByte(input[i]) {
			i++
		}
		if tokenStart == i {
			continue
		}
		token := input[tokenStart:i]
		if !isPlausibleHostToken(token) {
			continue
		}
		if isObservabilityMentionToken(token) {
			continue
		}
		raw := input[start:i]
		source := HostMentionSourceHostnameLiteral
		address := ""
		display := token
		if isIPLiteral(token) {
			source = HostMentionSourceIPLiteral
			address = token
		} else if isLocalAliasToken(token) {
			source = HostMentionSourceLocalAlias
			display = "local"
		}
		mentions = append(mentions, HostMention{
			TokenID:     stableMentionTokenID(start, raw),
			Raw:         raw,
			SpanStart:   start,
			SpanEnd:     i,
			Address:     address,
			DisplayName: display,
			Source:      source,
			Resolved:    false,
			Confidence:  0.75,
		})
		i--
	}
	return mentions
}

// DetectInventoryHostMentions is kept for older callers, but intentionally no
// longer binds bare inventory names. Host execution requires @host, @ip, or an
// explicit selected-host context so prose like "on host-a" cannot silently pick
// an execution target.
func DetectInventoryHostMentions(input string, hosts []HostRecordView) []HostMention {
	input = strings.TrimSpace(input)
	if input == "" || len(hosts) == 0 {
		return nil
	}
	return nil
}

func detectInventoryHostMentionsLegacy(input string, hosts []HostRecordView) []HostMention {
	candidates := inventoryHostMentionCandidates(hosts)
	if len(candidates) == 0 {
		return nil
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if len(candidates[i].value) == len(candidates[j].value) {
			return candidates[i].host.ID < candidates[j].host.ID
		}
		return len(candidates[i].value) > len(candidates[j].value)
	})
	seenHosts := map[string]struct{}{}
	mentions := make([]HostMention, 0)
	lowerInput := strings.ToLower(input)
	for _, candidate := range candidates {
		hostID := strings.TrimSpace(candidate.host.ID)
		if hostID == "" {
			continue
		}
		if _, ok := seenHosts[strings.ToLower(hostID)]; ok {
			continue
		}
		start := firstBoundedHostTokenIndex(lowerInput, strings.ToLower(candidate.value))
		if start < 0 {
			continue
		}
		end := start + len(candidate.value)
		mentions = append(mentions, HostMention{
			TokenID:     stableMentionTokenID(start, input[start:end]),
			Raw:         input[start:end],
			SpanStart:   start,
			SpanEnd:     end,
			HostID:      hostID,
			Address:     strings.TrimSpace(candidate.host.Address),
			DisplayName: firstNonEmpty(candidate.host.DisplayName, candidate.host.Hostname, candidate.host.Address, hostID),
			Source:      HostMentionSourceInventory,
			Resolved:    true,
			Confidence:  1,
		})
		seenHosts[strings.ToLower(hostID)] = struct{}{}
	}
	sort.SliceStable(mentions, func(i, j int) bool {
		return mentions[i].SpanStart < mentions[j].SpanStart
	})
	return mentions
}

type inventoryHostMentionCandidate struct {
	value string
	host  HostRecordView
}

func inventoryHostMentionCandidates(hosts []HostRecordView) []inventoryHostMentionCandidate {
	candidates := make([]inventoryHostMentionCandidate, 0, len(hosts)*4)
	seen := map[string]struct{}{}
	for _, host := range hosts {
		host.ID = strings.TrimSpace(host.ID)
		if host.ID == "" || isBareLocalHostAlias(host.ID) {
			continue
		}
		for _, value := range []string{host.ID, host.Hostname, host.DisplayName, host.Address} {
			value = strings.TrimSpace(value)
			if value == "" || isBareLocalHostAlias(value) {
				continue
			}
			key := strings.ToLower(host.ID + "\x00" + value)
			if _, ok := seen[key]; ok {
				continue
			}
			seen[key] = struct{}{}
			candidates = append(candidates, inventoryHostMentionCandidate{value: value, host: host})
		}
	}
	return candidates
}

func isBareLocalHostAlias(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "local", "localhost", "server-local", "127.0.0.1", "::1", "[::1]":
		return true
	default:
		return false
	}
}

func firstBoundedHostTokenIndex(inputLower, tokenLower string) int {
	if tokenLower == "" {
		return -1
	}
	offset := 0
	for {
		idx := strings.Index(inputLower[offset:], tokenLower)
		if idx < 0 {
			return -1
		}
		start := offset + idx
		end := start + len(tokenLower)
		if hasHostTokenBoundary(inputLower, start, end) {
			return start
		}
		offset = end
		if offset >= len(inputLower) {
			return -1
		}
	}
}

func hasHostTokenBoundary(input string, start, end int) bool {
	if start > 0 && isBareHostTokenByte(input[start-1]) {
		return false
	}
	if end < len(input) && isBareHostTokenByte(input[end]) {
		return false
	}
	return true
}

func isBareHostTokenByte(ch byte) bool {
	return isASCIILetter(ch) || isASCIIDigit(ch) || ch == '_' || ch == '-' || ch == '.' || ch == ':' || ch == '[' || ch == ']'
}

// UniqueMentionKeys returns normalized unique mention keys in deterministic
// order. It is used for deciding one-child-agent-per-host before resolution.
func UniqueMentionKeys(mentions []HostMention) []string {
	seen := make(map[string]struct{}, len(mentions))
	for _, mention := range mentions {
		key := mentionKey(mention)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func ResourceBindingProjectionFromMention(mention HostMention) resourcebinding.HostMentionProjection {
	return resourcebinding.HostMentionProjection{
		Raw:         mention.Raw,
		HostID:      mention.HostID,
		Address:     mention.Address,
		DisplayName: mention.DisplayName,
		Source:      string(mention.Source),
		Resolved:    mention.Resolved,
		Confidence:  mention.Confidence,
	}
}

func ResourceRefFromHostMention(mention HostMention) resourcebinding.ResourceRef {
	return resourcebinding.HostBindingFromMention(ResourceBindingProjectionFromMention(mention)).Ref
}

func mentionKey(mention HostMention) string {
	if strings.TrimSpace(mention.HostID) != "" {
		return "host:" + strings.ToLower(strings.TrimSpace(mention.HostID))
	}
	if strings.TrimSpace(mention.Address) != "" {
		return "addr:" + strings.ToLower(strings.TrimSpace(mention.Address))
	}
	value := strings.TrimPrefix(strings.TrimSpace(mention.Raw), "@")
	if mention.DisplayName != "" {
		value = mention.DisplayName
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	return "name:" + strings.ToLower(value)
}

func stableMentionTokenID(start int, raw string) string {
	sum := sha1.Sum([]byte(raw))
	return "hm_" + strconvItoa(start) + "_" + hex.EncodeToString(sum[:])[:8]
}

func strconvItoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}

func isEmailLocalPartByte(ch byte) bool {
	return isASCIILetter(ch) || isASCIIDigit(ch) || ch == '_' || ch == '-' || ch == '.'
}

func isMentionTokenByte(ch byte) bool {
	return isASCIILetter(ch) || isASCIIDigit(ch) || ch == '_' || ch == '-' || ch == '.' || ch == ':' || ch == '[' || ch == ']'
}

func isPlausibleHostToken(token string) bool {
	token = strings.TrimSpace(token)
	if token == "" {
		return false
	}
	if isIPLiteral(token) {
		return true
	}
	if strings.Contains(token, "..") {
		return false
	}
	for _, r := range token {
		if r > unicode.MaxASCII {
			return false
		}
	}
	return true
}

func isLocalAliasToken(token string) bool {
	return strings.EqualFold(strings.TrimSpace(token), "local")
}

func isObservabilityMentionToken(token string) bool {
	switch strings.ToLower(strings.TrimSpace(token)) {
	case "coroot":
		return true
	default:
		return false
	}
}

func isIPLiteral(token string) bool {
	normalized := strings.Trim(token, "[]")
	return net.ParseIP(normalized) != nil
}

func isASCIILetter(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isASCIIDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
