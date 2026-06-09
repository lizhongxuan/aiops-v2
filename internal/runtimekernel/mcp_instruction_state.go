package runtimekernel

import (
	"fmt"
	"sort"
	"strings"

	"aiops-v2/internal/mcp"
	"aiops-v2/internal/promptcompiler"
)

func appendMCPInstructionContext(compileCtx promptcompiler.CompileContext, session *SessionState) promptcompiler.CompileContext {
	if session == nil {
		return compileCtx
	}
	reg := mcp.DefaultRegistry()
	if reg == nil {
		return compileCtx
	}
	deltas := reg.ServerInstructionDelta(&session.MCPInstructions)
	if len(deltas) > 0 {
		session.MCPInstructions.Apply(deltas)
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "MCP Instruction Delta",
			Content: renderMCPInstructionDelta(deltas),
		})
		return compileCtx
	}
	if reminder := renderMCPSparseReminder(session.MCPInstructions); reminder != "" {
		compileCtx.ExtraSections = append(compileCtx.ExtraSections, promptcompiler.PromptSection{
			Title:   "MCP Instruction Reminder",
			Content: reminder,
		})
	}
	return compileCtx
}

func renderMCPInstructionDelta(deltas []mcp.MCPInstructionDelta) string {
	lines := []string{"Use these MCP server instruction deltas. Do not replay removed instructions."}
	for _, delta := range deltas {
		line := fmt.Sprintf("- server=%s action=%s hash=%s chars=%d", delta.ServerID, delta.Action, delta.Hash, delta.Chars)
		if delta.Text != "" && delta.Action != "removed" {
			line += "\n  " + strings.TrimSpace(delta.Text)
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func renderMCPSparseReminder(state mcp.MCPInstructionSessionState) string {
	if len(state.Announced) == 0 {
		return ""
	}
	serverIDs := make([]string, 0, len(state.Announced))
	for serverID := range state.Announced {
		serverIDs = append(serverIDs, serverID)
	}
	sort.Strings(serverIDs)
	lines := []string{"MCP sparse reminder: instructions were previously announced; only server id, hash, and summary are repeated."}
	for _, serverID := range serverIDs {
		item := state.Announced[serverID]
		lines = append(lines, fmt.Sprintf("- server=%s hash=%s summary=%s", item.ServerID, item.Hash, item.Summary))
	}
	return strings.Join(lines, "\n")
}
