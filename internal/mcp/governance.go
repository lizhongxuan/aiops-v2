package mcp

import (
	"strings"

	"aiops-v2/internal/tooling"
)

type ServerGovernance struct {
	ID                           string
	Permission                   string
	Risk                         string
	RequiresExplicitUserApproval bool
}

func MergeMCPGovernance(server ServerConfig, serverCatalog ServerGovernance, tool tooling.ToolMetadata) tooling.ToolMetadata {
	merged := tool
	merged.IsMCP = true
	merged.MCPInfo.ServerID = firstNonEmptyString(server.ID, merged.MCPInfo.ServerID)
	merged.MCPInfo.ServerName = firstNonEmptyString(server.Name, merged.MCPInfo.ServerName)
	if merged.MCPInfo.ToolName == "" {
		merged.MCPInfo.ToolName = merged.Name
	}
	merged.RiskLevel = highestRisk(merged.RiskLevel, tooling.ToolRiskLevel(strings.ToLower(strings.TrimSpace(serverCatalog.Risk))))
	if strings.EqualFold(strings.TrimSpace(serverCatalog.Permission), "readwrite") && merged.Mutating {
		merged.RiskLevel = highestRisk(merged.RiskLevel, tooling.ToolRiskMedium)
	}
	if serverCatalog.RequiresExplicitUserApproval && (merged.Mutating || merged.RiskLevel.Normalize() != tooling.ToolRiskLow) {
		merged.RequiresApproval = true
	}
	if server.Disabled {
		merged.Discovery.HiddenFromPrompt = true
		merged.Discovery.HiddenFromDiscovery = true
	}
	return merged
}

func highestRisk(left, right tooling.ToolRiskLevel) tooling.ToolRiskLevel {
	if riskRankMCP(right) > riskRankMCP(left) {
		return right.Normalize()
	}
	return left.Normalize()
}

func riskRankMCP(risk tooling.ToolRiskLevel) int {
	switch risk.Normalize() {
	case tooling.ToolRiskLow:
		return 1
	case tooling.ToolRiskMedium:
		return 2
	case tooling.ToolRiskHigh:
		return 3
	case tooling.ToolRiskCritical:
		return 4
	default:
		return 2
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
