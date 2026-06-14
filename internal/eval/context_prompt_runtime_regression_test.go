package eval

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type contextPromptRuntimeRegressionCase struct {
	Name   string `json:"name"`
	Signal string `json:"signal"`
}

func TestContextPromptRuntimeRegressionCasesStayGeneric(t *testing.T) {
	cases := []contextPromptRuntimeRegressionCase{
		{Name: "aggregate tool result budget", Signal: "many read-only tool results are bounded by aggregate context budget"},
		{Name: "artifact range read", Signal: "large externalized artifact is read by offset, limit, query, or metadata"},
		{Name: "resource dedupe", Signal: "unchanged resource version returns a compact unchanged stub"},
		{Name: "compact no drift", Signal: "next step is anchored to latest user turn and recent user quote"},
		{Name: "simple task overhead", Signal: "short factual request does not trigger compaction or complex runtime path"},
	}
	cases = append(cases, loadContextPromptRuntimeResourceCases(t)...)
	pattern := strings.Join([]string{
		`(?i)(\b\d{1,3}(?:\.\d{1,3}){3}\b`,
		`fixed` + `_(site|host|service|cluster|namespace|incident)`,
		`hardcoded` + `_(site|host|service|cluster|namespace|incident)`,
		`固定` + `主机`,
		`固定` + `服务`,
		`固定` + `故障)`,
	}, "|")
	banned := regexp.MustCompile(pattern)
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			if banned.MatchString(tc.Name) || banned.MatchString(tc.Signal) {
				t.Fatalf("case is not generic: %#v", tc)
			}
			if tc.Signal == "" {
				t.Fatalf("case %q has no effectiveness signal", tc.Name)
			}
		})
	}
}

func loadContextPromptRuntimeResourceCases(t *testing.T) []contextPromptRuntimeRegressionCase {
	t.Helper()
	path := filepath.Join("testdata", "context_prompt_runtime_resource_cases.json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var cases []contextPromptRuntimeRegressionCase
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatalf("decode %s: %v", path, err)
	}
	if len(cases) == 0 {
		t.Fatalf("%s has no cases", path)
	}
	return cases
}
