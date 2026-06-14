package runtimekernel

import (
	"encoding/json"
	"slices"
	"strings"
	"time"

	"aiops-v2/internal/agentstate"
	"aiops-v2/internal/tooling"
)

func applyProgressiveToolPackMetadata(metadata map[string]string, input string, session *SessionState, catalog []tooling.Tool) map[string]string {
	metadata = applyContinuationToolPacks(metadata, input, session, catalog)
	metadata = applyIntentToolPacks(metadata, input, session, catalog)
	return applyToolDiscoveryTurnMetadata(metadata, session)
}

func applyIntentToolPacks(metadata map[string]string, input string, session *SessionState, catalog []tooling.Tool) map[string]string {
	suppressObservationPacks := isDirectHostResourceInspection(input, session)
	for _, match := range tooling.MatchToolPacksByMetadata(catalog, input) {
		if !intentPackMatchEligible(match, catalog) {
			continue
		}
		if suppressObservationPacks && packHasExternalObservationTools(match.Pack, catalog) {
			continue
		}
		metadata = enableIntentToolPack(metadata, match.Pack)
	}
	return metadata
}

func isDirectHostResourceInspection(input string, session *SessionState) bool {
	if session != nil && session.Type != SessionTypeHost {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(input))
	if text == "" {
		return false
	}
	hostScoped := containsAnyFold(text, []string{
		"当前主机", "选中主机", "当前选中", "这台主机", "当前远程主机", "远程主机", "本机",
		"current host", "selected host", "this host", "remote host", "local host",
	})
	if !hostScoped && strings.Contains(text, "主机") {
		hostScoped = containsAnyFold(text, []string{"cpu", "内存", "磁盘", "负载", "资源", "使用率", "状态", "情况"})
	}
	if !hostScoped {
		return false
	}
	return containsAnyFold(text, []string{
		"cpu", "memory", "mem", "disk", "load", "uptime", "resource", "resources",
		"内存", "磁盘", "负载", "资源", "使用率", "空闲", "系统状态", "资源情况",
	})
}

func packHasExternalObservationTools(pack string, catalog []tooling.Tool) bool {
	pack = strings.TrimSpace(pack)
	if pack == "" {
		return false
	}
	for _, toolDef := range catalog {
		if toolDef == nil {
			continue
		}
		meta := toolDef.Metadata()
		if meta.Pack != pack {
			continue
		}
		if isExternalObservationToolMetadata(meta) {
			return true
		}
	}
	return false
}

func isExternalObservationToolMetadata(meta tooling.ToolMetadata) bool {
	d := meta.EffectiveDiscovery()
	switch strings.ToLower(strings.TrimSpace(meta.Domain)) {
	case "observability", "external_observability", "monitoring":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(d.DiscoveryGroup)) {
	case "observability", "monitoring", "metrics":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(d.CapabilityKind)) {
	case "observability", "metrics", "logs", "traces", "topology", "incidents", "profiling":
		return true
	}
	for _, packID := range d.ToolPackIDs {
		packID = strings.ToLower(strings.TrimSpace(packID))
		if strings.Contains(packID, "observability") || strings.Contains(packID, "monitoring") {
			return true
		}
	}
	for _, resourceType := range d.ResourceTypes {
		if strings.EqualFold(strings.TrimSpace(resourceType), "synthetic_observation") {
			return true
		}
	}
	return false
}

func containsAnyFold(text string, needles []string) bool {
	text = strings.ToLower(text)
	for _, needle := range needles {
		needle = strings.ToLower(strings.TrimSpace(needle))
		if needle != "" && strings.Contains(text, needle) {
			return true
		}
	}
	return false
}

func applyContinuationToolPacks(metadata map[string]string, input string, session *SessionState, catalog []tooling.Tool) map[string]string {
	if !isContinuationOnlyInput(input) || session == nil {
		return metadata
	}
	for _, toolName := range recentSessionToolNames(session, 4) {
		meta, ok := metadataForCatalogToolName(catalog, toolName)
		if !ok || strings.TrimSpace(meta.Pack) == "" {
			continue
		}
		metadata = enableIntentToolPack(metadata, meta.Pack)
	}
	return metadata
}

func isContinuationOnlyInput(input string) bool {
	text := strings.ToLower(strings.TrimSpace(input))
	text = strings.Trim(text, " \t\r\n。.!！?？,，;；:：")
	if text == "" {
		return false
	}
	switch text {
	case "继续", "继续看", "继续查", "继续排查", "继续分析", "继续下钻", "下一步", "下一步查", "下钻", "往下查", "接着看", "接着查", "接着排查", "continue", "go on", "next", "proceed":
		return true
	}
	if strings.HasPrefix(text, "继续") && len([]rune(text)) <= 8 {
		return true
	}
	if strings.HasPrefix(text, "接着") && len([]rune(text)) <= 8 {
		return true
	}
	return false
}

func recentSessionToolNames(session *SessionState, maxTurns int) []string {
	if session == nil || maxTurns <= 0 {
		return nil
	}
	var names []string
	turns := 0
	for i := len(session.TurnHistory) - 1; i >= 0 && turns < maxTurns; i-- {
		turns++
		names = append(names, toolNamesFromSnapshot(session.TurnHistory[i])...)
	}
	for i := len(session.Messages) - 1; i >= 0 && len(names) < 64; i-- {
		for _, toolCall := range session.Messages[i].ToolCalls {
			names = append(names, toolCall.Name)
		}
	}
	return names
}

func toolNamesFromSnapshot(snapshot TurnSnapshot) []string {
	names := make([]string, 0, len(snapshot.AgentItems)+len(snapshot.Iterations))
	for i := len(snapshot.AgentItems) - 1; i >= 0; i-- {
		item := snapshot.AgentItems[i]
		switch item.Type {
		case agentstate.TurnItemTypeToolCall, agentstate.TurnItemTypeToolResult:
			if name := toolNameFromAgentItem(item); name != "" {
				names = append(names, name)
			}
		default:
			continue
		}
	}
	for i := len(snapshot.Iterations) - 1; i >= 0; i-- {
		iteration := snapshot.Iterations[i]
		names = append(names, iteration.VisibleTools...)
		for _, toolCall := range iteration.ToolCalls {
			names = append(names, toolCall.Name)
		}
	}
	return names
}

func toolNameFromAgentItem(item agentstate.TurnItem) string {
	if len(item.Payload.Data) > 0 {
		var payload struct {
			Name     string `json:"name"`
			ToolName string `json:"toolName"`
			Tool     string `json:"tool"`
		}
		if err := json.Unmarshal(item.Payload.Data, &payload); err == nil {
			for _, value := range []string{payload.ToolName, payload.Name, payload.Tool} {
				if strings.TrimSpace(value) != "" {
					return strings.TrimSpace(value)
				}
			}
		}
	}
	return strings.TrimSpace(item.Payload.Summary)
}

func enableIntentToolPack(metadata map[string]string, pack string) map[string]string {
	metadata = ensureTurnMetadata(metadata)
	metadata["enableToolPack"] = appendMetadataListValue(metadata["enableToolPack"], pack)
	return metadata
}

func enableSelectedTool(metadata map[string]string, toolName string) map[string]string {
	metadata = ensureTurnMetadata(metadata)
	metadata["enableTool"] = appendMetadataListValue(metadata["enableTool"], toolName)
	return metadata
}

func applyToolDiscoveryTurnMetadata(metadata map[string]string, session *SessionState) map[string]string {
	if session == nil {
		return metadata
	}
	for _, pack := range session.ToolDiscovery.EnabledPacks() {
		metadata = enableIntentToolPack(metadata, pack)
	}
	for _, toolName := range session.ToolDiscovery.EnabledTools() {
		metadata = enableSelectedTool(metadata, toolName)
	}
	return metadata
}

func updateToolSearchPackTurnMetadata(metadata map[string]string, toolName string, result ToolResult) map[string]string {
	if toolName != "tool_search" && (result.Display == nil || result.Display.Type != "tool_search") {
		return metadata
	}
	data := []byte(result.Content)
	if result.Display != nil && len(result.Display.Data) > 0 {
		data = result.Display.Data
	}
	var payload struct {
		Mode    string `json:"mode"`
		Matches []struct {
			Kind string `json:"kind"`
			Name string `json:"name"`
			Pack string `json:"pack"`
		} `json:"matches"`
		Selection struct {
			LoadedTools []string `json:"loadedTools"`
			LoadedPacks []string `json:"loadedPacks"`
		} `json:"selection"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return metadata
	}
	if !strings.EqualFold(strings.TrimSpace(payload.Mode), "select") {
		return metadata
	}
	for _, pack := range payload.Selection.LoadedPacks {
		metadata = enableIntentToolPack(metadata, pack)
	}
	for _, toolName := range payload.Selection.LoadedTools {
		metadata = enableSelectedTool(metadata, toolName)
	}
	return metadata
}

func applyToolSearchDiscoveryState(session *SessionState, toolName string, result ToolResult, turnID string) {
	if session == nil || (toolName != "tool_search" && (result.Display == nil || result.Display.Type != "tool_search")) {
		return
	}
	data := []byte(result.Content)
	if result.Display != nil && len(result.Display.Data) > 0 {
		data = result.Display.Data
	}
	var payload struct {
		Mode      string                    `json:"mode"`
		Matches   []ToolSearchMatchSnapshot `json:"matches"`
		Selection struct {
			LoadedTools      []string          `json:"loadedTools"`
			LoadedPacks      []string          `json:"loadedPacks"`
			NotLoaded        []string          `json:"notLoaded"`
			NotLoadedReasons map[string]string `json:"notLoadedReasons"`
			Reason           string            `json:"reason"`
		} `json:"selection"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return
	}
	now := time.Now()
	switch strings.ToLower(strings.TrimSpace(payload.Mode)) {
	case "search":
		session.ToolDiscovery.ApplySearch(payload.Matches, now)
	case "select":
		delta := ToolSelectionDelta{
			NotLoaded:        payload.Selection.NotLoaded,
			NotLoadedReasons: payload.Selection.NotLoadedReasons,
			Reason:           payload.Selection.Reason,
		}
		for _, name := range payload.Selection.LoadedTools {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				delta.LoadedTools = append(delta.LoadedTools, LoadedToolRef{Name: trimmed, Source: "tool_search.select", Reason: payload.Selection.Reason})
			}
		}
		for _, name := range payload.Selection.LoadedPacks {
			if trimmed := strings.TrimSpace(name); trimmed != "" {
				delta.LoadedPacks = append(delta.LoadedPacks, LoadedPackRef{Name: trimmed, Source: "tool_search.select", Reason: payload.Selection.Reason})
			}
		}
		session.ToolDiscovery.ApplySelection(delta, now)
	}
}

func intentPackMatchEligible(match tooling.PackTriggerMatch, catalog []tooling.Tool) bool {
	if strings.TrimSpace(match.Pack) == "" || len(match.ToolNames) == 0 {
		return false
	}
	for _, name := range match.ToolNames {
		toolDef, ok := catalogToolByName(catalog, name)
		if !ok {
			continue
		}
		meta := toolDef.Metadata()
		if tooling.ToolHiddenFromDiscovery(meta) || meta.Mutating || meta.RequiresApproval || meta.RiskLevel.Normalize() != tooling.ToolRiskLow {
			continue
		}
		if meta.Pack == match.Pack {
			return true
		}
	}
	return false
}

func metadataForCatalogToolName(catalog []tooling.Tool, name string) (tooling.ToolMetadata, bool) {
	toolDef, ok := catalogToolByName(catalog, name)
	if !ok {
		return tooling.ToolMetadata{}, false
	}
	return toolDef.Metadata(), true
}

func catalogToolByName(catalog []tooling.Tool, name string) (tooling.Tool, bool) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}
	for _, toolDef := range catalog {
		if toolDef == nil {
			continue
		}
		meta := toolDef.Metadata()
		if catalogNameMatches(meta, name) {
			return toolDef, true
		}
	}
	return nil, false
}

func catalogNameMatches(meta tooling.ToolMetadata, name string) bool {
	if strings.EqualFold(name, meta.Name) || strings.EqualFold(name, tooling.ProviderSafeToolName(meta.Name)) {
		return true
	}
	for _, alias := range meta.Aliases {
		if strings.EqualFold(name, alias) || strings.EqualFold(name, tooling.ProviderSafeToolName(alias)) {
			return true
		}
	}
	return false
}

func visibleToolsForContextMode(tools []string, thresholds ContextBudgetThresholds) []string {
	if !thresholds.SmallContextMode {
		return append([]string(nil), tools...)
	}
	allowed := make([]string, 0, len(tools))
	for _, name := range tools {
		trimmed := strings.TrimSpace(name)
		if trimmed == "" {
			continue
		}
		if smallContextCoreTool(trimmed) || !smallContextHighVolumeTool(trimmed) {
			allowed = append(allowed, trimmed)
		}
	}
	return allowed
}

func filterToolsForContextMode(tools []tooling.Tool, thresholds ContextBudgetThresholds) []tooling.Tool {
	if !thresholds.SmallContextMode {
		return tools
	}
	filtered := make([]tooling.Tool, 0, len(tools))
	for _, tool := range tools {
		if tool == nil {
			continue
		}
		meta := tool.Metadata()
		if smallContextCoreTool(meta.Name) || tooling.ToolAllowedInSmallContext(meta) {
			filtered = append(filtered, tool)
		}
	}
	return filtered
}

func smallContextCoreTool(name string) bool {
	return slices.Contains([]string{
		"tool_search",
		"mcp.list_resources",
		"mcp.read_resource",
		"list_mcp_resources",
		"read_mcp_resource",
	}, name)
}

func smallContextHighVolumeTool(name string) bool {
	normalized := strings.ToLower(name)
	return strings.Contains(normalized, "debug.dump") ||
		strings.Contains(normalized, "dump_all") ||
		strings.Contains(normalized, "all_metrics") ||
		strings.Contains(normalized, "raw_metrics")
}
