package main

import (
	"strings"

	"aiops-v2/internal/integrations/coroot"
	"aiops-v2/internal/mcp"
)

func registerBuiltinIntegrations(mcpRegistry *mcp.Registry, corootEndpoint string) error {
	if mcpRegistry == nil {
		return nil
	}
	if endpoint := strings.TrimSpace(corootEndpoint); endpoint != "" {
		if err := coroot.RegisterBuiltins(mcpRegistry, endpoint); err != nil {
			return err
		}
	}
	return nil
}
