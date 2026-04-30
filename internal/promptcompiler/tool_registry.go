package promptcompiler

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/tooling"
)

const defaultToolPromptInlineBudgetBytes = 4096

// ---------------------------------------------------------------------------
// Layer 3: Tool Prompt Set — capability descriptions and usage guidance for
// visible tools.
// ---------------------------------------------------------------------------

// buildToolPromptSet compiles Layer 3: the tool prompt set containing
// capability descriptions for all visible tool-like capabilities.
func (c *PromptCompilerImpl) buildToolPromptSet(ctx CompileContext) (ToolPromptSet, error) {
	var entries []ToolPromptEntry
	var parts []string

	parts = append(parts, "# Tool Index")

	for _, tool := range ctx.AssembledTools {
		toolEntry := c.buildToolPromptEntry(tool)
		entries = append(entries, toolEntry)
		parts = append(parts, c.formatToolIndexLine(toolPromptSectionTitle(tool), toolEntry))
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

func (c *PromptCompilerImpl) buildToolPromptDelta(ctx CompileContext) ToolPromptDelta {
	delta := ToolPromptDelta{
		NewlyAvailable:         append([]string(nil), ctx.ToolDelta.NewlyAvailable...),
		TemporarilyUnavailable: append([]string(nil), ctx.ToolDelta.TemporarilyUnavailable...),
		ApprovalRequired:       append([]string(nil), ctx.ToolDelta.ApprovalRequired...),
	}

	if len(delta.ApprovalRequired) == 0 {
		for _, tool := range ctx.AssembledTools {
			if tool == nil || !tool.IsDestructive(nil) {
				continue
			}
			if name := toolPromptSectionTitle(tool); name != "" {
				delta.ApprovalRequired = append(delta.ApprovalRequired, name)
			}
		}
	}

	delta.NewlyAvailable = normalizePromptNames(delta.NewlyAvailable)
	delta.TemporarilyUnavailable = normalizePromptNames(delta.TemporarilyUnavailable)
	delta.ApprovalRequired = normalizePromptNames(delta.ApprovalRequired)

	var parts []string
	if len(delta.NewlyAvailable) > 0 {
		parts = append(parts, "## Newly available tools\n- "+strings.Join(delta.NewlyAvailable, "\n- "))
	}
	if len(delta.TemporarilyUnavailable) > 0 {
		parts = append(parts, "## Temporarily unavailable tools\n- "+strings.Join(delta.TemporarilyUnavailable, "\n- "))
	}
	if len(delta.ApprovalRequired) > 0 {
		parts = append(parts, "## Approval reminders\n- "+strings.Join(delta.ApprovalRequired, "\n- "))
	}
	delta.Content = strings.Join(parts, "\n\n")
	return delta
}

// buildToolPromptEntry creates a ToolPromptEntry from an assembled tool.
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

	te.Governance = toolGovernanceSummary(tool)
	te.ApprovalNote = toolApprovalNote(tool)
	te.UsagePolicy = toolUsagePolicy(tool)
	te.Example = toolUsageExample(tool)
	te.FailureHandling = toolFailureHandling(tool)

	return te
}

func (c *PromptCompilerImpl) formatToolIndexLine(name string, entry ToolPromptEntry) string {
	lines := []string{"- " + name}
	if entry.Capability != "" {
		lines[0] = fmt.Sprintf("- %s: %s", name, entry.Capability)
	}
	if entry.UsagePolicy != "" {
		lines = append(lines, "  Usage policy: "+entry.UsagePolicy)
	}
	if entry.Governance != "" {
		lines = append(lines, "  Governance: "+entry.Governance)
	}
	if entry.Example != "" {
		lines = append(lines, "  Example: "+entry.Example)
	}
	if entry.FailureHandling != "" {
		lines = append(lines, "  Failure handling: "+entry.FailureHandling)
	}
	return strings.Join(lines, "\n")
}

func toolGovernanceSummary(tool Tool) string {
	meta := tool.Metadata()
	governance := meta.EffectiveGovernance(defaultToolPromptInlineBudgetBytes)
	approval := "not_required"
	if governance.RequiresApproval {
		approval = "required"
	}
	return fmt.Sprintf(
		"risk=%s, mutating=%t, approval=%s, resultBudget=%d, failure=%s",
		governance.RiskLevel,
		governance.Mutating,
		approval,
		governance.ResultBudget.MaxInlineResultBytes,
		governance.FailurePolicy,
	)
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

func toolUsagePolicy(tool Tool) string {
	if tool.IsDestructive(nil) {
		return "Use only after confirming intent, risk, target scope, and approval requirements."
	}
	if tool.IsReadOnly(nil) {
		return "Use to gather evidence before answering claims that depend on local or current state."
	}
	return "Use when the user request requires this capability and cheaper context is insufficient."
}

func toolUsageExample(tool Tool) string {
	name := toolPromptSectionTitle(tool)
	if tool.IsDestructive(nil) {
		return fmt.Sprintf("%s after approval to apply a scoped change, then verify the result.", name)
	}
	if tool.IsReadOnly(nil) {
		return fmt.Sprintf("%s to inspect evidence, then cite the observed result in the answer.", name)
	}
	return fmt.Sprintf("%s with minimal arguments needed for the current task.", name)
}

func toolFailureHandling(tool Tool) string {
	if tool.IsDestructive(nil) {
		return "Stop, report the failed mutation, and do not retry with broader scope without approval."
	}
	if tool.IsReadOnly(nil) {
		return "Report the missing evidence and try a narrower read-only query when useful."
	}
	return "Surface the error, keep prior evidence separate from inference, and ask for missing input if needed."
}

func toolResultShape(tool Tool) string {
	if shape := summarizeSchema(tool.OutputSchema()); shape != "" {
		return shape
	}
	return "Output shape: structured data"
}

func normalizePromptNames(items []string) []string {
	if len(items) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		out = append(out, item)
	}
	sort.Strings(out)
	return out
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
