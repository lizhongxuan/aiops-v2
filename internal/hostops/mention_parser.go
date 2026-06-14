package hostops

import (
	"crypto/sha1"
	"encoding/hex"
	"net"
	"sort"
	"strings"
	"unicode"
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
		raw := input[start:i]
		source := HostMentionSourceHostnameLiteral
		address := ""
		display := token
		if isIPLiteral(token) {
			source = HostMentionSourceIPLiteral
			address = token
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
