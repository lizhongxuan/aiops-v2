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
	var toolLines []string

	for _, tool := range ctx.AssembledTools {
		if tool == nil || tooling.ToolHiddenFromPrompt(tool.Metadata()) {
			continue
		}
		toolEntry := c.buildToolPromptEntry(tool)
		entries = append(entries, toolEntry)
		toolLines = append(toolLines, c.formatToolIndexLine(tool, toolEntry))
	}
	deferredDirectory := buildDeferredToolDirectory(ctx)

	parts := []string{"# Tool Index"}
	if len(entries) == 0 {
		parts = append(parts, "No tools available in current context.")
	} else {
		parts = append(parts, commonToolPolicyPrompt())
		parts = append(parts, toolLines...)
	}
	if directoryContent := formatDeferredToolDirectory(deferredDirectory); directoryContent != "" {
		parts = append(parts, directoryContent)
	}

	content := strings.Join(parts, "\n\n")
	return ToolPromptSet{
		Content:           content,
		Entries:           entries,
		DeferredDirectory: deferredDirectory,
	}, nil
}

func (c *PromptCompilerImpl) buildToolPromptDelta(ctx CompileContext) ToolPromptDelta {
	delta := ToolPromptDelta{
		NewlyAvailable:         append([]string(nil), ctx.ToolDelta.NewlyAvailable...),
		NewlyAvailablePacks:    append([]string(nil), ctx.ToolDelta.NewlyAvailablePacks...),
		TemporarilyUnavailable: append([]string(nil), ctx.ToolDelta.TemporarilyUnavailable...),
		ApprovalRequired:       append([]string(nil), ctx.ToolDelta.ApprovalRequired...),
	}

	if len(delta.ApprovalRequired) == 0 {
		for _, tool := range ctx.AssembledTools {
			if tool == nil || !tool.IsDestructive(nil) {
				continue
			}
			if name := toolPromptDeltaName(tool); name != "" {
				delta.ApprovalRequired = append(delta.ApprovalRequired, name)
			}
		}
	}

	delta.NewlyAvailable = normalizePromptNames(delta.NewlyAvailable)
	delta.NewlyAvailablePacks = normalizePromptNames(delta.NewlyAvailablePacks)
	delta.TemporarilyUnavailable = normalizePromptNames(delta.TemporarilyUnavailable)
	delta.ApprovalRequired = normalizePromptNames(delta.ApprovalRequired)

	var parts []string
	if len(delta.NewlyAvailable) > 0 {
		parts = append(parts, "## Newly available tools\n- "+strings.Join(delta.NewlyAvailable, "\n- "))
	}
	if len(delta.NewlyAvailablePacks) > 0 {
		parts = append(parts, "## Newly available tool packs\n- "+strings.Join(delta.NewlyAvailablePacks, "\n- "))
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
		te.Guidance = promptNote
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

func (c *PromptCompilerImpl) formatToolIndexLine(tool Tool, entry ToolPromptEntry) string {
	name := toolPromptSectionTitle(tool)
	line := "- " + name
	if entry.Capability != "" {
		line = fmt.Sprintf("- %s: %s", name, entry.Capability)
	}
	var tags []string
	if tool.IsReadOnly(nil) {
		tags = append(tags, "read_only")
	}
	if tool.IsDestructive(nil) {
		tags = append(tags, "mutation")
	}
	if strings.Contains(strings.ToLower(entry.Governance), "approval=required") || strings.Contains(strings.ToLower(entry.Governance), "approval=true") {
		tags = append(tags, "approval_required")
	}
	if !tool.IsConcurrencySafe(nil) {
		tags = append(tags, "not_concurrency_safe")
	}
	if len(tags) > 0 {
		line += " [" + strings.Join(tags, ",") + "]"
	}
	return line
}

func commonToolPolicyPrompt() string {
	return strings.Join([]string{
		"Common policy:",
		"- Only visible tools are callable.",
		"- Failure, empty output, denial, or timeout is not proof of healthy state.",
		"- Mutation requires scoped runtime approval and post-check.",
		"- Summarize large results and keep raw data behind refs.",
	}, "\n")
}

const maxDeferredToolDirectoryEntries = 18

type deferredDirectoryAggregate struct {
	entry        DeferredToolDirectoryEntry
	capability   map[string]struct{}
	resourceSet  map[string]struct{}
	operationSet map[string]struct{}
}

func buildDeferredToolDirectory(ctx CompileContext) []DeferredToolDirectoryEntry {
	if len(ctx.DeferredToolCatalog) == 0 {
		return nil
	}
	visible := make(map[string]struct{}, len(ctx.AssembledTools))
	for _, tool := range ctx.AssembledTools {
		if tool == nil {
			continue
		}
		if name := strings.TrimSpace(tool.Metadata().Name); name != "" {
			visible[name] = struct{}{}
		}
	}

	byPack := map[string]*deferredDirectoryAggregate{}
	for _, tool := range ctx.DeferredToolCatalog {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if _, ok := visible[strings.TrimSpace(meta.Name)]; ok {
			continue
		}
		if tooling.ToolHiddenFromDiscovery(meta) || tooling.ToolHiddenFromPrompt(meta) {
			continue
		}
		discovery := meta.EffectiveDiscovery()
		if !isDeferredDirectoryCandidate(meta, discovery) {
			continue
		}
		pack := deferredDirectoryPackName(meta, discovery)
		if pack == "" {
			continue
		}
		agg := byPack[pack]
		if agg == nil {
			agg = &deferredDirectoryAggregate{
				entry: DeferredToolDirectoryEntry{
					Pack:             pack,
					Source:           string(discovery.LoadingPolicy),
					MCPServerID:      discovery.MCPServerID,
					HealthStatus:     deferredDirectoryHealthStatus(ctx, discovery),
					RequiresHealth:   discovery.RequiresHealthyMCP,
					RequiresSelect:   tooling.ToolRequiresSelect(meta),
					RequiresApproval: meta.EffectiveGovernance(defaultToolPromptInlineBudgetBytes).RequiresApproval,
				},
				capability:   map[string]struct{}{},
				resourceSet:  map[string]struct{}{},
				operationSet: map[string]struct{}{},
			}
			byPack[pack] = agg
		}
		agg.entry.ToolCount++
		if agg.entry.Source == "" || agg.entry.Source == string(tooling.ToolLoadingPolicyCore) {
			agg.entry.Source = string(discovery.LoadingPolicy)
		}
		if discovery.MCPServerID != "" {
			agg.entry.MCPServerID = discovery.MCPServerID
		}
		if discovery.RequiresHealthyMCP {
			agg.entry.RequiresHealth = true
			if agg.entry.HealthStatus == "" {
				agg.entry.HealthStatus = deferredDirectoryHealthStatus(ctx, discovery)
			}
		}
		if tooling.ToolRequiresSelect(meta) {
			agg.entry.RequiresSelect = true
		}
		if meta.EffectiveGovernance(defaultToolPromptInlineBudgetBytes).RequiresApproval {
			agg.entry.RequiresApproval = true
		}
		if discovery.CapabilityKind != "" {
			agg.capability[discovery.CapabilityKind] = struct{}{}
		}
		for _, value := range discovery.ResourceTypes {
			agg.resourceSet[value] = struct{}{}
		}
		for _, value := range discovery.OperationKinds {
			agg.operationSet[value] = struct{}{}
		}
	}
	if len(byPack) == 0 {
		return nil
	}
	packs := make([]string, 0, len(byPack))
	for pack := range byPack {
		packs = append(packs, pack)
	}
	sort.Slice(packs, func(i, j int) bool {
		left := deferredDirectoryRelevanceScore(ctx, packs[i], byPack[packs[i]])
		right := deferredDirectoryRelevanceScore(ctx, packs[j], byPack[packs[j]])
		if left != right {
			return left > right
		}
		return packs[i] < packs[j]
	})
	out := make([]DeferredToolDirectoryEntry, 0, len(packs))
	for _, pack := range packs {
		agg := byPack[pack]
		agg.entry.ResourceTypes = sortedSetValues(agg.resourceSet, 4)
		agg.entry.OperationKinds = sortedSetValues(agg.operationSet, 4)
		agg.entry.Capability = deferredDirectoryCapability(agg.capability, agg.entry.ResourceTypes, agg.entry.OperationKinds)
		if agg.entry.RequiresHealth && agg.entry.HealthStatus != "" && agg.entry.HealthStatus != "healthy" {
			agg.entry.UnavailableReason = "requires healthy external source; current health=" + agg.entry.HealthStatus
		}
		out = append(out, agg.entry)
		if len(out) >= maxDeferredToolDirectoryEntries {
			break
		}
	}
	return out
}

func deferredDirectoryRelevanceScore(ctx CompileContext, pack string, agg *deferredDirectoryAggregate) int {
	if agg == nil {
		return 0
	}
	score := 0
	text := strings.ToLower(strings.Join([]string{
		pack,
		agg.entry.MCPServerID,
		strings.Join(setKeys(agg.capability), " "),
		strings.Join(setKeys(agg.resourceSet), " "),
		strings.Join(setKeys(agg.operationSet), " "),
	}, " "))
	if runtimeStateRequested(ctx.WebState) && strings.Contains(text, "public_web") {
		score += 100
	}
	if runtimeStateRequested(ctx.OpsGraphState) && (strings.Contains(text, "opsgraph") || strings.Contains(text, "ops_graph")) {
		score += 100
	}
	if runtimeStateRequested(ctx.CorootState) && strings.Contains(text, "coroot") {
		score += 100
	}
	if runtimeStateRequested(ctx.OpsManusState) && (strings.Contains(text, "ops_manual") || strings.Contains(text, "ops_manus")) {
		score += 100
	}
	switch normalizePromptProfile(ctx.Profile) {
	case PromptProfileEvidenceRCA:
		if containsAnyDirectoryTerm(text, "rca", "metrics", "logs", "traces", "observability", "service") {
			score += 20
		}
	case PromptProfileHostWorker:
		if containsAnyDirectoryTerm(text, "host", "system", "command", "process", "disk", "network") {
			score += 20
		}
	case PromptProfileHostManager:
		if containsAnyDirectoryTerm(text, "host", "agent", "workflow", "plan") {
			score += 20
		}
	}
	if agg.entry.RequiresSelect {
		score += 5
	}
	if agg.entry.RequiresHealth && agg.entry.HealthStatus != "" && agg.entry.HealthStatus != "healthy" {
		score -= 10
	}
	return score
}

func runtimeStateRequested(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "requested", "available", "enabled", "visible":
		return true
	default:
		return false
	}
}

func containsAnyDirectoryTerm(text string, terms ...string) bool {
	for _, term := range terms {
		if strings.Contains(text, term) {
			return true
		}
	}
	return false
}

func setKeys(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	sort.Strings(out)
	return out
}

func isDeferredDirectoryCandidate(meta tooling.ToolMetadata, discovery tooling.ToolDiscoveryMetadata) bool {
	if tooling.ToolRequiresSelect(meta) {
		return true
	}
	switch discovery.LoadingPolicy {
	case tooling.ToolLoadingPolicyDeferred, tooling.ToolLoadingPolicyMCP, tooling.ToolLoadingPolicyConditional, tooling.ToolLoadingPolicyProfile:
		return true
	default:
		return meta.DeferByDefault || meta.Pack != ""
	}
}

func deferredDirectoryPackName(meta tooling.ToolMetadata, discovery tooling.ToolDiscoveryMetadata) string {
	if pack := strings.TrimSpace(meta.Pack); pack != "" {
		return pack
	}
	for _, pack := range discovery.ToolPackIDs {
		if strings.TrimSpace(pack) != "" {
			return strings.TrimSpace(pack)
		}
	}
	if group := strings.TrimSpace(discovery.DiscoveryGroup); group != "" {
		return group
	}
	return strings.TrimSpace(discovery.CapabilityKind)
}

func deferredDirectoryHealthStatus(ctx CompileContext, discovery tooling.ToolDiscoveryMetadata) string {
	if !discovery.RequiresHealthyMCP {
		return ""
	}
	serverID := strings.TrimSpace(discovery.MCPServerID)
	if serverID != "" && len(ctx.MCPHealthSnapshot) > 0 {
		if status := strings.TrimSpace(ctx.MCPHealthSnapshot[serverID]); status != "" {
			return status
		}
	}
	return "unknown"
}

func deferredDirectoryCapability(capabilities map[string]struct{}, resources, operations []string) string {
	var parts []string
	if values := sortedSetValues(capabilities, 3); len(values) > 0 {
		parts = append(parts, "capability="+strings.Join(values, ","))
	}
	if len(resources) > 0 {
		parts = append(parts, "resources="+strings.Join(resources, ","))
	}
	if len(operations) > 0 {
		parts = append(parts, "ops="+strings.Join(operations, ","))
	}
	return strings.Join(parts, "; ")
}

func sortedSetValues(values map[string]struct{}, limit int) []string {
	if len(values) == 0 || limit == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for value := range values {
		if strings.TrimSpace(value) != "" {
			out = append(out, strings.TrimSpace(value))
		}
	}
	sort.Strings(out)
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

func formatDeferredToolDirectory(entries []DeferredToolDirectoryEntry) string {
	if len(entries) == 0 {
		return ""
	}
	lines := []string{
		"## Deferred Tool Directory",
		"These are discoverable tool families, not currently callable schemas. Use tool_search/select before calling any deferred tool.",
	}
	for _, entry := range entries {
		status := []string{}
		if entry.Source != "" {
			status = append(status, "source="+entry.Source)
		}
		if entry.RequiresSelect {
			status = append(status, "select=required")
		}
		if entry.RequiresApproval {
			status = append(status, "approval=required")
		}
		if entry.RequiresHealth {
			health := entry.HealthStatus
			if health == "" {
				health = "unknown"
			}
			status = append(status, "health="+health)
		}
		if entry.MCPServerID != "" {
			status = append(status, "mcp="+entry.MCPServerID)
		}
		if entry.ToolCount > 0 {
			status = append(status, fmt.Sprintf("tools=%d", entry.ToolCount))
		}
		line := "- " + entry.Pack
		if entry.Capability != "" {
			line += ": " + entry.Capability
		}
		if len(status) > 0 {
			line += " (" + strings.Join(status, ", ") + ")"
		}
		if entry.UnavailableReason != "" {
			line += "; unavailable=" + entry.UnavailableReason
		}
		lines = append(lines, line)
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
		return tooling.ProviderSafeToolName(meta.Name)
	}
	if len(meta.Aliases) > 0 && meta.Aliases[0] != "" {
		return tooling.ProviderSafeToolName(meta.Aliases[0])
	}
	if desc := toolCapabilityDescription(tool); desc != "" {
		return desc
	}
	return "tool"
}

func toolCanonicalNameForPrompt(tool Tool) string {
	if tool == nil {
		return ""
	}
	meta := tool.Metadata()
	if strings.TrimSpace(meta.Name) != "" {
		return strings.TrimSpace(meta.Name)
	}
	for _, alias := range meta.Aliases {
		if strings.TrimSpace(alias) != "" {
			return strings.TrimSpace(alias)
		}
	}
	return ""
}

func toolPromptDeltaName(tool Tool) string {
	name := toolPromptSectionTitle(tool)
	canonical := toolCanonicalNameForPrompt(tool)
	if canonical != "" && canonical != name {
		return fmt.Sprintf("%s (canonical: %s)", name, canonical)
	}
	return name
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
		return "Requires runtime tool approval; call the scoped tool and let the runtime approval gate pause execution."
	}
	if tool.IsReadOnly(nil) {
		return "Generally no approval required."
	}
	return "May require approval depending on policy."
}

func toolUsagePolicy(tool Tool) string {
	if tool.IsDestructive(nil) {
		return "Use when the user requested the scoped change and the target is clear; do not ask for prose approval when the runtime approval gate can handle it."
	}
	if tool.IsReadOnly(nil) {
		return "Use to gather evidence before answering claims that depend on local or current state."
	}
	return "Use when the user request requires this capability and cheaper context is insufficient."
}

func toolUsageExample(tool Tool) string {
	name := toolPromptSectionTitle(tool)
	if tool.IsDestructive(nil) {
		return fmt.Sprintf("%s to request a scoped change through the runtime approval gate, then verify the result.", name)
	}
	if tool.IsReadOnly(nil) {
		return fmt.Sprintf("%s to inspect evidence, then cite the observed result in the answer.", name)
	}
	return fmt.Sprintf("%s with minimal arguments needed for the current task.", name)
}

func toolFailureHandling(tool Tool) string {
	common := "policy blocked and permission denied do not prove target system state; non-zero exit requires stderr and exit code interpretation; empty output does not prove no abnormality."
	if tool.IsDestructive(nil) {
		return "Stop, report the failed mutation, and do not broaden scope or retry riskier actions unless a new scoped tool call can go through the runtime approval gate; " + common
	}
	if tool.IsReadOnly(nil) {
		return "Read-only tool failure is evidence state, not target state: classify it as missing/blocked evidence and try a narrower read-only query when useful; " + common
	}
	return "Surface the error, keep prior evidence separate from inference, and ask for missing input if needed; " + common
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
