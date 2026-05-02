package eval

import (
	"fmt"
	"strings"

	"aiops-v2/internal/agentstate"
)

type DraftCaseInput struct {
	ID       string
	Category string
	Input    string
	Output   RunOutput
}

type DraftCaseResult struct {
	Case            Case
	SidecarMarkdown string
}

func DraftCaseFromRunOutput(input DraftCaseInput) DraftCaseResult {
	toolNames := uniqueToolNames(input.Output.ToolCalls)
	caseID := strings.TrimSpace(input.ID)
	if caseID == "" {
		caseID = "draft-agent-case"
	}
	category := strings.TrimSpace(input.Category)
	if category == "" {
		category = "agent-debug"
	}
	c := Case{
		ID:                caseID,
		Category:          category,
		RootCauseCategory: "prompt",
		Priority:          "P1",
		Input:             strings.TrimSpace(input.Input),
		Expected: Expected{
			MustInclude:       []string{},
			MustNotInclude:    []string{},
			ExpectedToolCalls: toolNames,
			MaxIterations:     countModelCalls(input.Output.TurnItems) + 1,
			MaxToolCalls:      len(input.Output.ToolCalls) + 1,
		},
	}
	return DraftCaseResult{
		Case:            c,
		SidecarMarkdown: renderDraftSidecar(c, input.Output),
	}
}

func uniqueToolNames(calls []ToolCall) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, call := range calls {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	return out
}

func countModelCalls(items []agentstate.TurnItem) int {
	count := 0
	for _, item := range items {
		if item.Type == agentstate.TurnItemTypeModelCall {
			count++
		}
	}
	return count
}

func renderDraftSidecar(c Case, output RunOutput) string {
	return fmt.Sprintf(`# Eval Case Draft Notes

- Case: %s
- Priority: %s
- Root cause category: %s

## Actual Answer

%s

## Human Review Required

- Fill expected.mustInclude with user-value assertions.
- Fill expected.mustNotInclude with the real failure pattern.
- Confirm expected.expectedToolCalls are necessary, not just incidental.
- Move to P0 only after the case is stable and cheap.
`, c.ID, c.Priority, c.RootCauseCategory, strings.TrimSpace(output.Answer))
}
