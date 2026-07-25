package workfloweditor

import (
	"testing"

	"aiops-v2/internal/tooling"
	edit "aiops-v2/internal/workfloweditor"
)

func TestWorkflowEditorLegacyToolsAreNotExposed(t *testing.T) {
	tools := Tools(edit.NewService(nil))
	if len(tools) != 0 {
		t.Fatalf("legacy placeholder tools remain visible: %v", workflowEditorToolNames(tools))
	}
}

func workflowEditorToolNames(tools []tooling.Tool) []string {
	names := make([]string, 0, len(tools))
	for _, toolDef := range tools {
		names = append(names, toolDef.Metadata().Name)
	}
	return names
}

func workflowEditorToolByName(t *testing.T, tools []tooling.Tool, name string) tooling.Tool {
	t.Helper()
	for _, toolDef := range tools {
		if toolDef.Metadata().Name == name {
			return toolDef
		}
	}
	t.Fatalf("tool %s not found in %v", name, workflowEditorToolNames(tools))
	return nil
}
