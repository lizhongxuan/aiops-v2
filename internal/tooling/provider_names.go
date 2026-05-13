package tooling

import "strings"

const providerToolNameFallback = "tool"

// ProviderSafeToolName converts an internal tool name to the identifier shape
// accepted by model providers for function/tool names.
func ProviderSafeToolName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return providerToolNameFallback
	}

	var b strings.Builder
	lastReplacement := false
	for _, r := range name {
		if isProviderToolNameRune(r) {
			b.WriteRune(r)
			lastReplacement = false
			continue
		}
		if !lastReplacement {
			b.WriteByte('_')
			lastReplacement = true
		}
	}

	out := b.String()
	if strings.Trim(out, "_-") == "" {
		return providerToolNameFallback
	}
	return out
}

func isProviderToolNameRune(r rune) bool {
	return r == '_' || r == '-' ||
		(r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9')
}
