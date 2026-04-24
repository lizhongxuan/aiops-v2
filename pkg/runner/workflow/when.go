package workflow

import (
	"fmt"
	"strconv"
	"strings"
)

func EvalWhen(expr string, vars map[string]any) (bool, error) {
	trimmed := strings.TrimSpace(expr)
	if trimmed == "" {
		return true, nil
	}
	lowered := strings.ToLower(trimmed)
	switch lowered {
	case "true", "yes":
		return true, nil
	case "false", "no":
		return false, nil
	}

	orParts := splitLogical(trimmed, "||")
	if len(orParts) > 1 {
		for _, part := range orParts {
			ok, err := EvalWhen(part, vars)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
		return false, nil
	}

	andParts := splitLogical(trimmed, "&&")
	if len(andParts) > 1 {
		for _, part := range andParts {
			ok, err := EvalWhen(part, vars)
			if err != nil {
				return false, err
			}
			if !ok {
				return false, nil
			}
		}
		return true, nil
	}

	op, idx := findOperator(trimmed)
	if op == "" || idx <= 0 {
		value := parseOperand(trimmed, vars)
		return truthy(value), nil
	}

	leftRaw := strings.TrimSpace(trimmed[:idx])
	rightRaw := strings.TrimSpace(trimmed[idx+len(op):])
	left := parseOperand(leftRaw, vars)
	right := parseOperand(rightRaw, vars)

	switch op {
	case "==":
		return compareEqual(left, right), nil
	case "!=":
		return !compareEqual(left, right), nil
	case ">", ">=", "<", "<=":
		ln, lok := toNumber(left)
		rn, rok := toNumber(right)
		if !lok || !rok {
			return false, fmt.Errorf("when expression expects numeric comparison for %q", op)
		}
		switch op {
		case ">":
			return ln > rn, nil
		case ">=":
			return ln >= rn, nil
		case "<":
			return ln < rn, nil
		case "<=":
			return ln <= rn, nil
		}
	}

	return false, fmt.Errorf("unsupported when expression: %q", expr)
}

func splitLogical(expr, token string) []string {
	if !strings.Contains(expr, token) {
		return []string{expr}
	}
	var parts []string
	start := 0
	inSingle := false
	inDouble := false
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}
		if inSingle || inDouble {
			continue
		}
		if strings.HasPrefix(expr[i:], token) {
			parts = append(parts, expr[start:i])
			i += len(token) - 1
			start = i + 1
		}
	}
	if start <= len(expr) {
		parts = append(parts, expr[start:])
	}
	return parts
}

func findOperator(expr string) (string, int) {
	ops := []string{"==", "!=", ">=", "<=", ">", "<"}
	inSingle := false
	inDouble := false
	for i := 0; i < len(expr); i++ {
		ch := expr[i]
		switch ch {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		}
		if inSingle || inDouble {
			continue
		}
		for _, op := range ops {
			if strings.HasPrefix(expr[i:], op) {
				return op, i
			}
		}
	}
	return "", -1
}

func parseOperand(raw string, vars map[string]any) any {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return ""
	}
	if isQuoted(trimmed) {
		return trimmed[1 : len(trimmed)-1]
	}
	if strings.HasPrefix(trimmed, "${") && strings.HasSuffix(trimmed, "}") {
		key := strings.TrimSuffix(strings.TrimPrefix(trimmed, "${"), "}")
		if value, ok := lookupVar(vars, strings.TrimSpace(key)); ok {
			return value
		}
		return ""
	}
	if value, ok := lookupVar(vars, trimmed); ok {
		return value
	}
	if lowered := strings.ToLower(trimmed); lowered == "true" || lowered == "false" {
		return lowered == "true"
	}
	if num, err := strconv.ParseFloat(trimmed, 64); err == nil {
		return num
	}
	return trimmed
}

func isQuoted(value string) bool {
	if len(value) < 2 {
		return false
	}
	if value[0] == '"' && value[len(value)-1] == '"' {
		return true
	}
	if value[0] == '\'' && value[len(value)-1] == '\'' {
		return true
	}
	return false
}

func compareEqual(left, right any) bool {
	if ln, lok := toNumber(left); lok {
		if rn, rok := toNumber(right); rok {
			return ln == rn
		}
	}
	if lb, lok := left.(bool); lok {
		if rb, rok := right.(bool); rok {
			return lb == rb
		}
	}
	return fmt.Sprint(left) == fmt.Sprint(right)
}

func toNumber(value any) (float64, bool) {
	switch v := value.(type) {
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case string:
		trimmed := strings.TrimSpace(v)
		if trimmed == "" {
			return 0, false
		}
		num, err := strconv.ParseFloat(trimmed, 64)
		if err != nil {
			return 0, false
		}
		return num, true
	default:
		return 0, false
	}
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch v := value.(type) {
	case bool:
		return v
	case int:
		return v != 0
	case int64:
		return v != 0
	case float64:
		return v != 0
	case float32:
		return v != 0
	case string:
		trimmed := strings.TrimSpace(strings.ToLower(v))
		if trimmed == "" || trimmed == "false" || trimmed == "0" || trimmed == "no" {
			return false
		}
		return true
	default:
		return true
	}
}
