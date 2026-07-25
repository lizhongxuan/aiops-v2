package workfloweditor

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNoDirectWorkflowModelCall(t *testing.T) {
	root := workflowPlannerRepositoryRoot(t)
	for _, name := range []string{
		"internal/workfloweditor/planner.go",
		"pkg/runner/server/service/visual_workflow_ai.go",
	} {
		raw, err := os.ReadFile(filepath.Join(root, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		text := string(raw)
		for _, forbidden := range []string{".Generate(", "GetModel(", "ModelRouterWorkflowEditPlanner", "script.python\"", "hostResourceCheckDraftGraph"} {
			if strings.Contains(text, forbidden) {
				t.Fatalf("%s contains direct or placeholder workflow generation marker %q", name, forbidden)
			}
		}
	}

	mainSource, err := os.ReadFile(filepath.Join(root, "cmd/ai-server/main.go"))
	if err != nil {
		t.Fatalf("read ai-server main: %v", err)
	}
	if strings.Contains(string(mainSource), "WithEditPlanner") || strings.Contains(string(mainSource), "ModelRouterWorkflowEditPlanner") {
		t.Fatal("ai-server still assembles a second workflow model lifecycle")
	}
}

func workflowPlannerRepositoryRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve workflow planner test path")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
