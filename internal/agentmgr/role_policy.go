package agentmgr

import "aiops-v2/internal/tooling"

type AgentRole string

const (
	AgentRoleExplore   AgentRole = "explore"
	AgentRolePlan      AgentRole = "plan"
	AgentRoleExecute   AgentRole = "execute"
	AgentRoleVerify    AgentRole = "verify"
	AgentRoleHostChild AgentRole = "host_child"
)

type AgentRolePolicy struct {
	Role                 AgentRole `json:"role"`
	AllowMutatingTools   bool      `json:"allowMutatingTools"`
	AllowedToolLayers    []string  `json:"allowedToolLayers,omitempty"`
	RequiresPlanApproval bool      `json:"requiresPlanApproval,omitempty"`
	RequiresResourceLock bool      `json:"requiresResourceLock,omitempty"`
}

func AgentRolePolicyFor(role AgentRole) AgentRolePolicy {
	switch role {
	case AgentRoleExecute, AgentRoleHostChild:
		return AgentRolePolicy{
			Role:                 role,
			AllowMutatingTools:   true,
			AllowedToolLayers:    []string{string(tooling.ToolLayerCore), string(tooling.ToolLayerDeferred), string(tooling.ToolLayerMutation)},
			RequiresPlanApproval: true,
			RequiresResourceLock: true,
		}
	case AgentRolePlan:
		return readOnlyRolePolicy(role, []string{string(tooling.ToolLayerCore), string(tooling.ToolLayerDeferred)})
	case AgentRoleVerify, AgentRoleExplore:
		return readOnlyRolePolicy(role, []string{string(tooling.ToolLayerCore), string(tooling.ToolLayerDeferred)})
	default:
		return readOnlyRolePolicy(role, []string{string(tooling.ToolLayerCore)})
	}
}

func readOnlyRolePolicy(role AgentRole, layers []string) AgentRolePolicy {
	return AgentRolePolicy{
		Role:               role,
		AllowMutatingTools: false,
		AllowedToolLayers:  layers,
	}
}

func (p AgentRolePolicy) AllowsTool(meta tooling.ToolMetadata) bool {
	if meta.Mutating {
		return p.AllowMutatingTools && meta.RequiresApproval
	}
	layer := string(meta.Layer)
	if layer == "" {
		layer = string(tooling.ToolLayerCore)
	}
	return containsString(p.AllowedToolLayers, layer)
}
