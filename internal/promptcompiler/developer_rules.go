package promptcompiler

import (
	"fmt"
	"strings"
)

// ---------------------------------------------------------------------------
// Layer 2: Developer Instructions — runtime constraints and operational rules
// ---------------------------------------------------------------------------

// buildDeveloperInstructions compiles Layer 2: developer instructions containing
// mode-specific constraints, agent-kind constraints, and prompt assets.
func (c *PromptCompilerImpl) buildDeveloperInstructions(ctx CompileContext) (DeveloperInstructions, error) {
	constraints := c.resolveConstraints(ctx)

	var parts []string
	parts = append(parts, "# Developer Instructions")
	for _, constraint := range constraints {
		parts = append(parts, fmt.Sprintf("- %s", constraint))
	}

	// Append skill prompt assets
	for _, asset := range ctx.SkillPromptAssets {
		parts = append(parts, asset)
	}

	for _, section := range ctx.ExtraSections {
		title := strings.TrimSpace(section.Title)
		content := strings.TrimSpace(section.Content)
		if title == "" && content == "" {
			continue
		}
		if title != "" {
			parts = append(parts, fmt.Sprintf("## %s", title))
		}
		if content != "" {
			parts = append(parts, content)
		}
	}

	content := strings.Join(parts, "\n")
	return DeveloperInstructions{
		Content:     content,
		Constraints: constraints,
	}, nil
}

// resolveConstraints determines the active constraints based on mode and agent kind.
func (c *PromptCompilerImpl) resolveConstraints(ctx CompileContext) []string {
	var constraints []string

	// Universal constraints
	constraints = append(constraints, "Always verify tool results before reporting to user.")
	constraints = append(constraints, "Do not fabricate information not obtained from tools.")

	// Mode-specific constraints
	switch ctx.Mode {
	case "chat":
		constraints = append(constraints, "Only use read-only tools and web search in chat mode.")
		constraints = append(constraints, "Do not attempt any mutation operations.")
	case "inspect":
		constraints = append(constraints, "Only use read, list, search, and readonly shell operations.")
		constraints = append(constraints, "Do not attempt any mutation operations.")
	case "plan":
		constraints = append(constraints, "You may inspect and plan but must not directly execute mutations.")
		constraints = append(constraints, "Generate plans for review before execution.")
	case "execute":
		constraints = append(constraints, "Mutation operations require approval before execution.")
		constraints = append(constraints, "Always collect evidence before and after mutations.")
	}

	// Agent kind constraints
	switch ctx.AgentKind {
	case AgentKindWorker:
		constraints = append(constraints, "Only operate on your designated host.")
		constraints = append(constraints, "Report results back to the planner upon completion.")
	case AgentKindPlanner:
		constraints = append(constraints, "Coordinate worker agents, do not execute host operations directly.")
	}

	return constraints
}
