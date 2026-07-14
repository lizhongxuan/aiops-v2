package agentassembly

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
)

func StableHash(scope string, value any) string {
	raw, err := json.Marshal(normalizeForHash(value))
	if err != nil {
		raw = []byte("{}")
	}
	sum := sha256.Sum256([]byte(strings.TrimSpace(scope) + "\n" + string(raw)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func cloneJSONValue[T any](value T) (T, error) {
	var clone T
	raw, err := json.Marshal(value)
	if err != nil {
		return clone, err
	}
	if err := json.Unmarshal(raw, &clone); err != nil {
		return clone, err
	}
	return clone, nil
}

func normalizeForHash(value any) any {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return nil
	}
	return normalizeHashJSON(generic)
}

func normalizeHashJSON(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			out[strings.TrimSpace(key)] = normalizeHashJSON(item)
		}
		return out
	case []any:
		out := make([]any, 0, len(v))
		allStrings := true
		for _, item := range v {
			normalized := normalizeHashJSON(item)
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

func uniqueSortedStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
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
