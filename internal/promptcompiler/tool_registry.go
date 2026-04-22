package promptcompiler

import (
	"encoding/json"
	"fmt"
	"strings"

	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// Layer 3: Tool Prompt Set — capability descriptions for visible tools
// Per Req 3.5: only capability, constraints, result shape, and approval note.
// ---------------------------------------------------------------------------

// buildToolPromptSet compiles Layer 3: the tool prompt set containing
// capability descriptions for all visible tool-like capabilities.
func (c *PromptCompilerImpl) buildToolPromptSet(ctx CompileContext) (ToolPromptSet, error) {
	var entries []ToolPromptEntry
	var parts []string

	parts = append(parts, "# Available Tools")

	for _, tool := range ctx.AssembledTools {
		toolEntry := c.buildToolPromptEntry(tool)
		entries = append(entries, toolEntry)

		toolText := c.formatToolEntry(toolPromptSectionTitle(tool), toolEntry)
		parts = append(parts, toolText)
	}

	if len(entries) == 0 {
		parts = append(parts, "No tools available in current context.")
	}

	content := strings.Join(parts, "\n\n")
	return ToolPromptSet{
		Content: content,
		Entries: entries,
	}, nil
}

// buildToolPromptEntry creates a ToolPromptEntry from an assembled tool,
// extracting only the four allowed fields: capability, constraints, result shape, approval note.
func (c *PromptCompilerImpl) buildToolPromptEntry(tool Tool) ToolPromptEntry {
	capability := toolCapabilityDescription(tool)
	te := ToolPromptEntry{Capability: capability}

	var constraints []string
	if tool.IsReadOnly(nil) {
		constraints = append(constraints, "read-only")
	}
	if tool.IsDestructive(nil) {
		constraints = append(constraints, "destructive")
	}
	if !tool.IsConcurrencySafe(nil) {
		constraints = append(constraints, "not concurrency-safe")
	}
	if promptNote := toolPromptConstraint(tool, capability); promptNote != "" {
		constraints = append(constraints, promptNote)
	}
	te.Constraints = strings.Join(constraints, ", ")

	if resultShape := toolResultShape(tool); resultShape != "" {
		te.ResultShape = resultShape
	}

	te.ApprovalNote = toolApprovalNote(tool)

	return te
}

// formatToolEntry formats a single tool entry as prompt text.
func (c *PromptCompilerImpl) formatToolEntry(name string, entry ToolPromptEntry) string {
	var lines []string
	lines = append(lines, fmt.Sprintf("## %s", name))

	if entry.Capability != "" {
		lines = append(lines, fmt.Sprintf("Capability: %s", entry.Capability))
	}
	if entry.Constraints != "" {
		lines = append(lines, fmt.Sprintf("Constraints: %s", entry.Constraints))
	}
	if entry.ResultShape != "" {
		lines = append(lines, fmt.Sprintf("Result: %s", entry.ResultShape))
	}
	if entry.ApprovalNote != "" {
		lines = append(lines, fmt.Sprintf("Approval: %s", entry.ApprovalNote))
	}

	return strings.Join(lines, "\n")
}

func toolPromptSectionTitle(tool Tool) string {
	meta := tool.Metadata()
	if meta.Name != "" {
		return meta.Name
	}
	if len(meta.Aliases) > 0 && meta.Aliases[0] != "" {
		return meta.Aliases[0]
	}
	if desc := toolCapabilityDescription(tool); desc != "" {
		return desc
	}
	return "tool"
}

func toolCapabilityDescription(tool Tool) string {
	meta := tool.Metadata()
	if meta.Description != "" {
		return meta.Description
	}
	return tool.Description(nil, tooling.DescribeContext{Metadata: meta})
}

func toolPromptConstraint(tool Tool, capability string) string {
	meta := tool.Metadata()
	prompt := strings.TrimSpace(tool.Prompt(tooling.PromptContext{Metadata: meta}))
	if prompt == "" {
		return ""
	}
	if prompt == strings.TrimSpace(capability) {
		return ""
	}
	return prompt
}

func toolApprovalNote(tool Tool) string {
	if tool.IsDestructive(nil) {
		return "Requires approval before execution."
	}
	if tool.IsReadOnly(nil) {
		return "Generally no approval required."
	}
	return "May require approval depending on policy."
}

func toolResultShape(tool Tool) string {
	if shape := summarizeSchema(tool.OutputSchema()); shape != "" {
		return shape
	}
	return "Output shape: structured data"
}

func summarizeSchema(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return ""
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		return "JSON schema"
	}

	parts := []string{"JSON schema"}
	if typ, ok := schema["type"].(string); ok && typ != "" {
		parts = append(parts, fmt.Sprintf("type=%s", typ))
	}
	if props, ok := schema["properties"].(map[string]any); ok && len(props) > 0 {
		parts = append(parts, fmt.Sprintf("properties=%d", len(props)))
	}
	if len(parts) == 1 {
		return parts[0]
	}
	return strings.Join(parts, ", ")
}
