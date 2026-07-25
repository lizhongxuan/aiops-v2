package eval

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

func workflowAcyclic(g WorkflowEvalGraph) bool {
	adj := map[string][]string{}
	for _, e := range g.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
	}
	state := map[string]uint8{}
	var visit func(string) bool
	visit = func(id string) bool {
		if state[id] == 1 {
			return false
		}
		if state[id] == 2 {
			return true
		}
		state[id] = 1
		for _, next := range adj[id] {
			if !visit(next) {
				return false
			}
		}
		state[id] = 2
		return true
	}
	for _, n := range g.Nodes {
		if !visit(n.ID) {
			return false
		}
	}
	return true
}
func workflowAllPathsEnd(g WorkflowEvalGraph) bool {
	if !workflowAcyclic(g) {
		return false
	}
	types := map[string]string{}
	incoming := map[string]int{}
	adj := map[string][]string{}
	for _, n := range g.Nodes {
		types[n.ID] = n.Type
	}
	for _, e := range g.Edges {
		adj[e.Source] = append(adj[e.Source], e.Target)
		incoming[e.Target]++
	}
	memo := map[string]bool{}
	done := map[string]bool{}
	var walk func(string) bool
	walk = func(id string) bool {
		if done[id] {
			return memo[id]
		}
		if len(adj[id]) == 0 {
			done[id], memo[id] = true, types[id] == "end"
			return memo[id]
		}
		for _, next := range adj[id] {
			if !walk(next) {
				done[id], memo[id] = true, false
				return false
			}
		}
		done[id], memo[id] = true, true
		return true
	}
	starts := 0
	for _, n := range g.Nodes {
		if incoming[n.ID] == 0 {
			starts++
			if !walk(n.ID) {
				return false
			}
		}
	}
	return starts > 0
}
func workflowPathValues(value any, path string) []any {
	parts := strings.Split(strings.Trim(strings.TrimSpace(path), "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return []any{value}
	}
	current := []any{value}
	for _, part := range parts {
		part = strings.ReplaceAll(strings.ReplaceAll(part, "~1", "/"), "~0", "~")
		var next []any
		for _, item := range current {
			switch typed := item.(type) {
			case map[string]any:
				if part == "*" {
					for _, v := range typed {
						next = append(next, v)
					}
				} else if v, ok := typed[part]; ok {
					next = append(next, v)
				}
			case []any:
				if part == "*" {
					next = append(next, typed...)
				} else if index, err := strconv.Atoi(part); err == nil && index >= 0 && index < len(typed) {
					next = append(next, typed[index])
				}
			}
		}
		current = next
	}
	return current
}
func jsonValue(value any) any {
	data, _ := json.Marshal(value)
	var decoded any
	_ = json.Unmarshal(data, &decoded)
	return decoded
}
func walkWorkflowStrings(value any, visit func(string)) {
	switch typed := value.(type) {
	case string:
		visit(typed)
	case []any:
		for _, v := range typed {
			walkWorkflowStrings(v, visit)
		}
	case map[string]any:
		for _, v := range typed {
			walkWorkflowStrings(v, visit)
		}
	}
}
func workflowValuesEqual(a, b []any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}
func workflowJSONType(v any, want string) bool {
	switch want {
	case "string":
		_, ok := v.(string)
		return ok
	case "number":
		_, ok := numberValue(v)
		return ok
	case "integer":
		n, ok := numberValue(v)
		return ok && math.Trunc(n) == n
	case "boolean":
		_, ok := v.(bool)
		return ok
	case "array":
		_, ok := v.([]any)
		if !ok {
			kind := fmt.Sprintf("%T", v)
			return strings.HasPrefix(kind, "[]")
		}
		return true
	case "object":
		_, ok := v.(map[string]any)
		return ok
	case "null":
		return v == nil
	case "":
		return true
	default:
		return false
	}
}
func numberValue(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case float32:
		return float64(n), true
	case int:
		return float64(n), true
	case int64:
		return float64(n), true
	case json.Number:
		x, e := n.Float64()
		return x, e == nil
	}
	return 0, false
}
func scoreWorkflowDeterminism(a []WorkflowGenerationAttempt) CheckResult {
	if len(a) < 3 {
		return CheckResult{Name: "deterministic", Passed: false, Missing: []string{"three_recordings"}}
	}
	first, _ := json.Marshal(a[0].Final.Graph)
	for _, v := range a[1:] {
		next, _ := json.Marshal(v.Final.Graph)
		if string(next) != string(first) || v.PlanHash != a[0].PlanHash || v.CompileHash != a[0].CompileHash {
			return CheckResult{Name: "deterministic", Passed: false, Unexpected: []string{"compile_drift"}}
		}
	}
	return CheckResult{Name: "deterministic", Passed: true}
}
func aggregateWorkflowChecks(groups [][]CheckResult) []CheckResult {
	if len(groups) == 0 {
		return nil
	}
	out := append([]CheckResult(nil), groups[0]...)
	for _, group := range groups[1:] {
		for i, c := range group {
			if i >= len(out) {
				out = append(out, c)
				continue
			}
			out[i].Passed = out[i].Passed && c.Passed
			out[i].Missing = uniqueStrings(append(out[i].Missing, c.Missing...))
			out[i].Unexpected = uniqueStrings(append(out[i].Unexpected, c.Unexpected...))
		}
	}
	return out
}
func allWorkflowChecksPass(c []CheckResult) bool {
	for _, v := range c {
		if !v.Passed {
			return false
		}
	}
	return true
}
func workflowChecksScore(c []CheckResult) float64 {
	if len(c) == 0 {
		return 0
	}
	return float64(countPassed(c)) / float64(len(c))
}
func countPassed(c []CheckResult) int {
	n := 0
	for _, v := range c {
		if v.Passed {
			n++
		}
	}
	return n
}
func failedWorkflowChecks(c []CheckResult) []string {
	var out []string
	for _, v := range c {
		if !v.Passed {
			out = append(out, v.Name)
		}
	}
	return out
}
func meanVariance(values []float64) (float64, float64) {
	if len(values) == 0 {
		return 0, 0
	}
	mean := sum(values) / float64(len(values))
	variance := 0.0
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	return mean, variance / float64(len(values))
}
func sum(v []float64) float64 {
	n := 0.0
	for _, x := range v {
		n += x
	}
	return n
}
func minFloat64(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	min := values[0]
	for _, value := range values[1:] {
		if value < min {
			min = value
		}
	}
	return min
}
func p95(values []int64) int64 {
	if len(values) == 0 {
		return 0
	}
	sort.Slice(values, func(i, j int) bool { return values[i] < values[j] })
	index := int(math.Ceil(float64(len(values))*.95)) - 1
	if index < 0 {
		index = 0
	}
	return values[index]
}
func stringSet(v []string) map[string]bool {
	out := map[string]bool{}
	for _, x := range v {
		out[x] = true
	}
	return out
}
func hasString(v []string, w string) bool {
	for _, x := range v {
		if x == w {
			return true
		}
	}
	return false
}
func uniqueStrings(v []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(v))
	for _, x := range v {
		if x != "" && !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	sort.Strings(out)
	return out
}
