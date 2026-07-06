package resourcebinding

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func StableTraceHash(scope string, value any) string {
	normalized := normalizeForStableHash(value)
	raw, err := json.Marshal(normalized)
	if err != nil {
		raw = []byte("{}")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(scope) + "\n" + string(raw)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeForStableHash(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	return normalizeStableJSON(generic)
}

func normalizeStableJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[strings.TrimSpace(key)] = normalizeStableJSON(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		allStrings := true
		for _, item := range v {
			normalized := normalizeStableJSON(item)
			if _, ok := normalized.(string); !ok {
				allStrings = false
			}
			out = append(out, normalized)
		}
		if allStrings {
			sort.Slice(out, func(i, j int) bool {
				return out[i].(string) < out[j].(string)
			})
		}
		return out
	case string:
		return strings.TrimSpace(v)
	default:
		return value
	}
}

func uniqueSorted(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
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
	sort.Strings(out)
	return out
}
