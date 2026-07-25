package workflowtestmanifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

const workflowStudioRedesignSchema = "aiops.workflow-studio-redesign.case-manifest/v1"

type workflowStudioRedesignManifest struct {
	SchemaVersion string                       `json:"schemaVersion"`
	Cases         []workflowStudioRedesignCase `json:"cases"`
}

type workflowStudioRedesignCase struct {
	ID         string   `json:"id"`
	Priority   string   `json:"priority"`
	Owner      string   `json:"owner"`
	Status     string   `json:"status"`
	Expected   string   `json:"expected"`
	Automation []string `json:"automation"`
	Blocker    string   `json:"blocker,omitempty"`
}

func TestWorkflowStudioRedesignManifestRegistersEveryNamedUXCase(t *testing.T) {
	path := filepath.Join(
		"..",
		"..",
		"testdata",
		"workflow_canvas_mcp",
		"workflow-studio-redesign-cases.json",
	)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var manifest workflowStudioRedesignManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != workflowStudioRedesignSchema {
		t.Fatalf("schemaVersion = %q, want %q", manifest.SchemaVersion, workflowStudioRedesignSchema)
	}

	expectedIDs := expectedWorkflowStudioRedesignCaseIDs()
	if got, want := len(manifest.Cases), len(expectedIDs); got != want {
		t.Fatalf("case count = %d, want %d", got, want)
	}
	seen := make(map[string]struct{}, len(manifest.Cases))
	for _, testCase := range manifest.Cases {
		if _, exists := seen[testCase.ID]; exists {
			t.Fatalf("duplicate case %s", testCase.ID)
		}
		seen[testCase.ID] = struct{}{}
		if testCase.Priority != "P0" && testCase.Priority != "P1" && testCase.Priority != "P2" {
			t.Errorf("%s priority = %q", testCase.ID, testCase.Priority)
		}
		if testCase.Owner == "" || testCase.Expected == "" || len(testCase.Automation) == 0 {
			t.Errorf("%s requires owner, expected, and automation", testCase.ID)
		}
		switch testCase.Status {
		case "implemented":
			if testCase.Blocker != "" {
				t.Errorf("%s implemented case must not declare blocker", testCase.ID)
			}
		case "partial", "blocked":
			if testCase.Blocker == "" {
				t.Errorf("%s %s case must declare blocker", testCase.ID, testCase.Status)
			}
		default:
			t.Errorf("%s status = %q", testCase.ID, testCase.Status)
		}
	}
	for _, id := range expectedIDs {
		if _, exists := seen[id]; !exists {
			t.Errorf("missing case %s", id)
		}
	}
}

func expectedWorkflowStudioRedesignCaseIDs() []string {
	groups := []struct {
		prefix string
		count  int
	}{
		{prefix: "UX-HDR", count: 10},
		{prefix: "UX-IA", count: 8},
		{prefix: "UX-RUN", count: 8},
		{prefix: "UX-PW", count: 18},
	}
	ids := make([]string, 0, 44)
	for _, group := range groups {
		for index := 1; index <= group.count; index++ {
			ids = append(ids, group.prefix+"-"+threeDigits(index))
		}
	}
	return ids
}

func threeDigits(value int) string {
	return string([]byte{'0' + byte(value/100), '0' + byte(value/10%10), '0' + byte(value%10)})
}
