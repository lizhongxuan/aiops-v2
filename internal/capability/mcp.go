package capability

import (
	"fmt"
	"sync"

	mcpreg "aiops-v2/internal/mcp"
	"aiops-v2/internal/tooling"
)

// ---------------------------------------------------------------------------
// MCPServerManager manages dynamic MCP tool registration/unregistration.
// It tracks which entries belong to which MCP server, enabling clean
// connect/disconnect lifecycle management.
// ---------------------------------------------------------------------------

// MCPServerManager wraps a Registry reference and tracks MCP server tool ownership.
type MCPServerManager struct {
	mu       sync.RWMutex
	registry *Registry
	// servers maps serverID -> registered entries for that server.
	servers map[string][]Entry
	mcp     *mcpreg.Registry
}

// NewMCPServerManager creates a new MCPServerManager backed by the given registry.
func NewMCPServerManager(registry *Registry) *MCPServerManager {
	return &MCPServerManager{
		registry: registry,
		servers:  make(map[string][]Entry),
		mcp:      mcpreg.NewRegistry(),
	}
}

// OnServerConnected registers all tools from an MCP server as KindMCPTool entries.
// Each entry ID is prefixed with the serverID to ensure uniqueness across servers.
func (m *MCPServerManager) OnServerConnected(serverID string, tools []Entry) error {
	if serverID == "" {
		return fmt.Errorf("mcp: server id is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// If server is already connected, disconnect it first to avoid duplicates.
	if existing, ok := m.servers[serverID]; ok && len(existing) > 0 {
		for _, entry := range existing {
			m.registry.Unregister(entry.ID)
		}
		delete(m.servers, serverID)
		m.mcp.OnServerDisconnected(serverID)
	}

	// Prepare entries with prefixed IDs and forced KindMCPTool.
	var registered []Entry
	var toRegister []Entry
	var dynamicTools []tooling.Tool

	for i := range tools {
		entry := tools[i]
		// Prefix the ID with serverID to ensure global uniqueness.
		entry.ID = serverID + "/" + entry.ID
		entry.Kind = KindMCPTool
		toRegister = append(toRegister, entry)
		registered = append(registered, entry)

		if entry.Tool != nil {
			dynamicTools = append(dynamicTools, tooling.NewLegacyToolAdapter(legacyToolRuntimeBridge{
				runtime:    entry.Tool,
				visibility: entry.Visibility,
			}, tooling.ToolMetadata{
				Name:        entry.Name,
				Description: entry.Description,
				Origin:      tooling.ToolOriginMCP,
				IsMCP:       true,
				MCPInfo: tooling.MCPInfo{
					ServerID:   serverID,
					ServerName: serverID,
					ToolName:   entry.Name,
				},
			}))
		}
	}

	// Register all tools atomically via the registry.
	if err := m.registry.RegisterBatch(toRegister); err != nil {
		return fmt.Errorf("mcp: register server %q tools: %w", serverID, err)
	}

	if err := m.mcp.OnServerConnected(serverID, dynamicTools); err != nil {
		for _, entry := range registered {
			m.registry.Unregister(entry.ID)
		}
		return err
	}

	m.servers[serverID] = cloneEntries(registered)
	return nil
}

// OnServerDisconnected unregisters all tools from the given MCP server,
// ensuring no dangling references remain in the registry.
func (m *MCPServerManager) OnServerDisconnected(serverID string) {
	m.mu.Lock()
	defer m.mu.Unlock()

	entries, ok := m.servers[serverID]
	if !ok {
		return
	}

	for _, entry := range entries {
		m.registry.Unregister(entry.ID)
	}

	delete(m.servers, serverID)
	m.mcp.OnServerDisconnected(serverID)
}

// ListServerTools returns the currently registered entries for a given server.
// Returns nil if the server is not connected.
func (m *MCPServerManager) ListServerTools(serverID string) []Entry {
	m.mu.RLock()
	entries, ok := m.servers[serverID]
	m.mu.RUnlock()
	if !ok {
		return nil
	}
	return cloneEntries(entries)
}

// DynamicTools exposes the normalized MCP tool set for integration with the unified tool model.
func (m *MCPServerManager) DynamicTools() []tooling.Tool {
	return m.mcp.DynamicTools()
}

func cloneEntries(entries []Entry) []Entry {
	if entries == nil {
		return nil
	}
	out := make([]Entry, len(entries))
	copy(out, entries)
	return out
}
