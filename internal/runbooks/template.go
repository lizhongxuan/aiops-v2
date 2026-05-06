package runbooks

import (
	"fmt"
	"regexp"
	"strings"
)

var templateVarPattern = regexp.MustCompile(`^\{\{\s*([A-Za-z_][A-Za-z0-9_]*)\s*\}\}$`)
var templateAnyPattern = regexp.MustCompile(`\{\{([^}]+)\}\}`)

func RenderTemplateString(input string, context map[string]any) (string, error) {
	matches := templateAnyPattern.FindAllStringSubmatch(input, -1)
	if len(matches) == 0 {
		return input, nil
	}
	out := input
	for _, match := range matches {
		expr := strings.TrimSpace(match[1])
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(expr) {
			return "", fmt.Errorf("template expression %q is not allowed", expr)
		}
		value, ok := context[expr]
		if !ok {
			return "", fmt.Errorf("template variable %q not found", expr)
		}
		out = strings.ReplaceAll(out, match[0], fmt.Sprint(value))
	}
	return out, nil
}

func RenderValue(value any, context map[string]any) (any, error) {
	switch v := value.(type) {
	case string:
		if match := templateVarPattern.FindStringSubmatch(v); len(match) == 2 {
			value, ok := context[match[1]]
			if !ok {
				return nil, fmt.Errorf("template variable %q not found", match[1])
			}
			return value, nil
		}
		return RenderTemplateString(v, context)
	case []any:
		out := make([]any, len(v))
		for i, item := range v {
			rendered, err := RenderValue(item, context)
			if err != nil {
				return nil, err
			}
			out[i] = rendered
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			rendered, err := RenderValue(item, context)
			if err != nil {
				return nil, err
			}
			out[key] = rendered
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(v))
		for key, item := range v {
			rendered, err := RenderValue(item, context)
			if err != nil {
				return nil, err
			}
			out[fmt.Sprint(key)] = rendered
		}
		return out, nil
	default:
		return value, nil
	}
}
