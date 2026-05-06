package runbooks

import (
	"fmt"
	"strconv"
	"strings"
)

func EvalCondition(condition string, evidence map[string]any) (bool, error) {
	condition = strings.TrimSpace(condition)
	if condition == "" {
		return true, nil
	}
	if strings.ContainsAny(condition, "();") {
		return false, fmt.Errorf("condition contains unsupported syntax")
	}
	orParts := strings.Split(condition, "||")
	for _, orPart := range orParts {
		andOK := true
		for _, term := range strings.Split(orPart, "&&") {
			ok, err := evalTerm(strings.TrimSpace(term), evidence)
			if err != nil {
				return false, err
			}
			andOK = andOK && ok
		}
		if andOK {
			return true, nil
		}
	}
	return false, nil
}

func evalTerm(term string, evidence map[string]any) (bool, error) {
	for _, op := range []string{"==", "!=", ">=", "<=", ">", "<"} {
		if idx := strings.Index(term, op); idx > 0 {
			left := strings.TrimSpace(term[:idx])
			right := strings.TrimSpace(term[idx+len(op):])
			if !strings.HasPrefix(left, "evidence.") {
				return false, fmt.Errorf("condition left side must be evidence key")
			}
			key := strings.TrimPrefix(left, "evidence.")
			got := evidence[key]
			want := parseLiteral(right)
			return compare(got, want, op), nil
		}
	}
	return false, fmt.Errorf("condition term %q is not supported", term)
}

func parseLiteral(raw string) any {
	raw = strings.TrimSpace(raw)
	if len(raw) >= 2 && ((raw[0] == '\'' && raw[len(raw)-1] == '\'') || (raw[0] == '"' && raw[len(raw)-1] == '"')) {
		return raw[1 : len(raw)-1]
	}
	switch raw {
	case "true":
		return true
	case "false":
		return false
	}
	if n, err := strconv.ParseFloat(raw, 64); err == nil {
		return n
	}
	return raw
}

func compare(got any, want any, op string) bool {
	switch op {
	case "==", "!=":
		eq := fmt.Sprint(got) == fmt.Sprint(want)
		if gotBool, ok := got.(bool); ok {
			if wantBool, ok := want.(bool); ok {
				eq = gotBool == wantBool
			}
		}
		if gf, ok := toFloat(got); ok {
			if wf, ok := toFloat(want); ok {
				eq = gf == wf
			}
		}
		if op == "!=" {
			return !eq
		}
		return eq
	case ">", ">=", "<", "<=":
		gf, gok := toFloat(got)
		wf, wok := toFloat(want)
		if !gok || !wok {
			return false
		}
		switch op {
		case ">":
			return gf > wf
		case ">=":
			return gf >= wf
		case "<":
			return gf < wf
		case "<=":
			return gf <= wf
		}
	}
	return false
}

func toFloat(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case jsonNumber:
		f, err := strconv.ParseFloat(string(v), 64)
		return f, err == nil
	default:
		return 0, false
	}
}

type jsonNumber string
