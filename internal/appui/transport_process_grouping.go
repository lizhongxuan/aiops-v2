package appui

import "strings"

// detectTransportToolBlockKind classifies only typed transport metadata. Tool
// names and user-visible summaries are deliberately excluded from this
// presentation decision.
func detectTransportToolBlockKind(envelopeKind, displayKind string) AiopsTransportProcessKind {
	kind := strings.ToLower(strings.TrimSpace(firstNonEmptyString(displayKind, envelopeKind)))
	switch {
	case strings.HasPrefix(kind, "hostops."):
		return AiopsTransportProcessKindSubagent
	case typedDisplayKindIn(kind,
		"browser.search", "web.search", "web_search", "browse_url",
		"browser.open", "browser.find", "web.open", "web.find"):
		return AiopsTransportProcessKindSearch
	case typedDisplayKindIn(kind, "command", "terminal.command", "host.command"):
		return AiopsTransportProcessKindCommand
	case kind == "file" || strings.HasPrefix(kind, "file.") || typedDisplayKindIn(kind, "apply_patch", "read_file", "write_file"):
		return AiopsTransportProcessKindFile
	case kind == "mcp" || strings.HasPrefix(kind, "mcp.") || typedDisplayKindIn(kind, "read_mcp_resource", "list_mcp_resources", "list_mcp_resource_templates"):
		return AiopsTransportProcessKindMCP
	default:
		return AiopsTransportProcessKindTool
	}
}

func typedDisplayKindIn(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if value == candidate {
			return true
		}
	}
	return false
}

func applyTransportFoldGroup(turnID string, turn AiopsTransportTurn, block *AiopsProcessBlock) {
	if block == nil {
		return
	}
	if groupID, groupKind := commentaryToolFoldGroup(turn.Process, block.ToolCallID); groupID != "" {
		block.FoldGroupID = groupID
		block.FoldGroupKind = groupKind
		return
	}
	groupKind := transportFoldGroupKind(*block)
	toolCallID := strings.TrimSpace(block.ToolCallID)
	if groupKind == "" || toolCallID == "" {
		return
	}
	block.FoldGroupKind = groupKind
	block.FoldGroupID = TransportProcessBlockStableID(turnID, "fold", toolCallID)
}

func applyTransportCommentaryFoldGroup(turnID string, block *AiopsProcessBlock) {
	if block == nil || block.Kind != AiopsTransportProcessKindAssistant || len(block.ToolCallIDs) == 0 {
		return
	}
	block.FoldGroupKind = "tool"
	block.FoldGroupID = TransportProcessBlockStableID(turnID, "fold", firstNonEmptyString(block.ID, block.ToolCallIDs[0]))
}

func bindExistingToolsToCommentaryGroup(blocks []AiopsProcessBlock, commentary AiopsProcessBlock) []AiopsProcessBlock {
	if strings.TrimSpace(commentary.FoldGroupID) == "" || len(commentary.ToolCallIDs) == 0 {
		return blocks
	}
	toolCallIDs := make(map[string]struct{}, len(commentary.ToolCallIDs))
	for _, toolCallID := range commentary.ToolCallIDs {
		if toolCallID = strings.TrimSpace(toolCallID); toolCallID != "" {
			toolCallIDs[toolCallID] = struct{}{}
		}
	}
	for idx := range blocks {
		if _, ok := toolCallIDs[strings.TrimSpace(blocks[idx].ToolCallID)]; !ok {
			continue
		}
		blocks[idx].FoldGroupID = commentary.FoldGroupID
		blocks[idx].FoldGroupKind = commentary.FoldGroupKind
	}
	return blocks
}

func commentaryToolFoldGroup(blocks []AiopsProcessBlock, toolCallID string) (string, string) {
	toolCallID = strings.TrimSpace(toolCallID)
	if toolCallID == "" {
		return "", ""
	}
	for idx := len(blocks) - 1; idx >= 0; idx-- {
		block := blocks[idx]
		if block.Kind != AiopsTransportProcessKindAssistant || strings.TrimSpace(block.FoldGroupID) == "" {
			continue
		}
		for _, candidate := range block.ToolCallIDs {
			if strings.TrimSpace(candidate) == toolCallID {
				return block.FoldGroupID, firstNonEmptyString(strings.TrimSpace(block.FoldGroupKind), "tool")
			}
		}
	}
	return "", ""
}

func transportFoldGroupKind(block AiopsProcessBlock) string {
	switch block.Kind {
	case AiopsTransportProcessKindSearch:
		return "web_lookup"
	case AiopsTransportProcessKindCommand:
		return "command"
	case AiopsTransportProcessKindFile:
		return "file"
	case AiopsTransportProcessKindMCP:
		return "mcp"
	case AiopsTransportProcessKindTool:
		return "tool"
	default:
		return ""
	}
}
