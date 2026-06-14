package runtime

import "aiops-v2/internal/mcp"

type ServerGovernanceProvider interface {
	ServerGovernance(serverID string) mcp.ServerGovernance
}

type ServerGovernanceProviderFunc func(serverID string) mcp.ServerGovernance

func (f ServerGovernanceProviderFunc) ServerGovernance(serverID string) mcp.ServerGovernance {
	if f == nil {
		return mcp.ServerGovernance{}
	}
	return f(serverID)
}
