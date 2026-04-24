package workflow

import (
	"fmt"
	"regexp"
	"strings"
)

var varPattern = regexp.MustCompile(`\$\{([^}]+)\}`)

func RenderString(input string, vars map[string]any) string {
	return varPattern.ReplaceAllStringFunc(input, func(match string) string {
		key := strings.TrimSuffix(strings.TrimPrefix(match, "${"), "}")
		if value, ok := lookupVar(vars, key); ok {
			return fmt.Sprint(value)
		}
		return match
	})
}

func RenderValue(value any, vars map[string]any) any {
	switch v := value.(type) {
	case string:
		return RenderString(v, vars)
	case map[string]any:
		return renderMap(v, vars)
	case map[any]any:
		out := make(map[string]any, len(v))
		for k, val := range v {
			out[fmt.Sprint(k)] = RenderValue(val, vars)
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i := range v {
			out[i] = RenderValue(v[i], vars)
		}
		return out
	default:
		return value
	}
}

func renderMap(input map[string]any, vars map[string]any) map[string]any {
	out := make(map[string]any, len(input))
	for k, v := range input {
		out[k] = RenderValue(v, vars)
	}
	return out
}

func lookupVar(vars map[string]any, key string) (any, bool) {
	if key == "" {
		return nil, false
	}
	parts := strings.Split(key, ".")
	var current any = vars
	for _, part := range parts {
		switch value := current.(type) {
		case map[string]any:
			v, ok := value[part]
			if !ok {
				return nil, false
			}
			current = v
		case map[any]any:
			v, ok := value[part]
			if !ok {
				return nil, false
			}
			current = v
		default:
			return nil, false
		}
	}
	return current, true
}
