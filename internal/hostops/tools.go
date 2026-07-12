package hostops

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/tooling"
)

const (
	ToolPackHostOps          = "hostops"
	ToolSpawnHostAgent       = "spawn_host_agent"
	ToolSendHostAgentMessage = "send_host_agent_message"
	ToolWaitHostAgents       = "wait_host_agents"
	ToolStopHostAgent        = "stop_host_agent"

	capabilityKindOrchestrationControl = "orchestration_control"
)

func NewManagerTools(orchestrator *Orchestrator) []tooling.Tool {
	return []tooling.Tool{
		spawnHostAgentTool(orchestrator),
		sendHostAgentMessageTool(orchestrator),
		waitHostAgentsTool(orchestrator),
		stopHostAgentTool(orchestrator),
	}
}

type spawnHostAgentInput struct {
	MissionID   string                 `json:"missionId"`
	Assignments []ChildAgentAssignment `json:"assignments"`
}

type childAgentMessageInput struct {
	ChildAgentID string `json:"childAgentId"`
	Content      string `json:"content"`
}

type waitHostAgentsInput struct {
	MissionID string `json:"missionId"`
}

type stopHostAgentInput struct {
	ChildAgentID string `json:"childAgentId"`
}

func spawnHostAgentTool(orchestrator *Orchestrator) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             ToolSpawnHostAgent,
			Description:      "Spawn one host-bound child agent per assignment for a host operation mission. This manager tool does not execute host commands or mutate hosts directly.",
			Origin:           tooling.ToolOriginBuiltin,
			Layer:            tooling.ToolLayerCore,
			Pack:             ToolPackHostOps,
			Profiles:         []string{"host_manager"},
			Domain:           "hostops",
			Mutating:         false,
			RiskLevel:        tooling.ToolRiskLow,
			FailurePolicy:    tooling.ToolFailurePolicyFailTurn,
			RecordEvidence:   true,
			DedupeEligible:   true,
			RequiresApproval: false,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: capabilityKindOrchestrationControl,
				OperationKinds: []string{"spawn_child_agents"},
			},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "hostops_mission",
				ResourceID:    "missionId",
				OperationKind: "spawn_child_agents",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{ToolWaitHostAgents + " for the same missionId"},
			},
		},
		Visibility:      managerToolVisibility(),
		InputSchemaData: json.RawMessage(`{"type":"object","required":["missionId","assignments"],"properties":{"missionId":{"type":"string"},"assignments":{"type":"array","items":{"type":"object","required":["hostId","task"],"properties":{"hostId":{"type":"string"},"hostAddress":{"type":"string"},"hostDisplayName":{"type":"string"},"role":{"type":"string"},"boundRole":{"type":"string"},"roleBindingHash":{"type":"string"},"task":{"type":"string"},"sessionId":{"type":"string"},"parentAgentId":{"type":"string"}}}}}}`),
		ReadOnlyFunc:    func(json.RawMessage) bool { return false },
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var req spawnHostAgentInput
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			children, err := orchestrator.SpawnChildren(ctx, req.MissionID, req.Assignments)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return jsonToolResult(ToolSpawnHostAgent, spawnHostAgentContract(children))
		},
	}
}

func sendHostAgentMessageTool(orchestrator *Orchestrator) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             ToolSendHostAgentMessage,
			Description:      "Send a manager follow-up message to one host-bound child agent. This manager tool does not execute host commands or mutate hosts directly.",
			Origin:           tooling.ToolOriginBuiltin,
			Layer:            tooling.ToolLayerCore,
			Pack:             ToolPackHostOps,
			Profiles:         []string{"host_manager"},
			Domain:           "hostops",
			Mutating:         false,
			RiskLevel:        tooling.ToolRiskLow,
			FailurePolicy:    tooling.ToolFailurePolicyFailTurn,
			RecordEvidence:   true,
			RequiresApproval: false,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: capabilityKindOrchestrationControl,
				OperationKinds: []string{"send_message"},
			},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "hostops_child_agent",
				ResourceID:    "childAgentId",
				OperationKind: "send_message",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{ToolWaitHostAgents + " for the child mission"},
			},
		},
		Visibility:      managerToolVisibility(),
		InputSchemaData: json.RawMessage(`{"type":"object","required":["childAgentId","content"],"properties":{"childAgentId":{"type":"string"},"content":{"type":"string"}}}`),
		ReadOnlyFunc:    func(json.RawMessage) bool { return false },
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var req childAgentMessageInput
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			child, err := orchestrator.SendMessage(ctx, req.ChildAgentID, req.Content)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return jsonToolResult(ToolSendHostAgentMessage, map[string]any{"child": childAgentResultContract(child, true)})
		},
	}
}

func waitHostAgentsTool(orchestrator *Orchestrator) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:           ToolWaitHostAgents,
			Description:    "Read current child agent statuses for a host operation mission.",
			Origin:         tooling.ToolOriginBuiltin,
			Layer:          tooling.ToolLayerCore,
			Pack:           ToolPackHostOps,
			Profiles:       []string{"host_manager"},
			Domain:         "hostops",
			RiskLevel:      tooling.ToolRiskLow,
			FailurePolicy:  tooling.ToolFailurePolicyFeedBackToModel,
			RecordEvidence: true,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind:  "read",
				OperationKinds:  []string{"read"},
				PermissionScope: "read",
			},
		},
		Visibility:      managerToolVisibility(),
		InputSchemaData: json.RawMessage(`{"type":"object","required":["missionId"],"properties":{"missionId":{"type":"string"}}}`),
		ReadOnlyFunc:    func(json.RawMessage) bool { return true },
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var req waitHostAgentsInput
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			children, err := orchestrator.WaitChildren(ctx, req.MissionID)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return jsonToolResult(ToolWaitHostAgents, waitHostAgentsContract(children))
		},
	}
}

func stopHostAgentTool(orchestrator *Orchestrator) tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:             ToolStopHostAgent,
			Description:      "Stop one host-bound child agent. This manager tool does not execute host commands or mutate hosts directly.",
			Origin:           tooling.ToolOriginBuiltin,
			Layer:            tooling.ToolLayerCore,
			Pack:             ToolPackHostOps,
			Profiles:         []string{"host_manager"},
			Domain:           "hostops",
			Mutating:         false,
			RiskLevel:        tooling.ToolRiskLow,
			FailurePolicy:    tooling.ToolFailurePolicyFailTurn,
			RecordEvidence:   true,
			RequiresApproval: false,
			Discovery: tooling.ToolDiscoveryMetadata{
				CapabilityKind: capabilityKindOrchestrationControl,
				OperationKinds: []string{"stop_child_agent"},
			},
			ResourceLocks: []tooling.ToolResourceLockKey{{
				ResourceType:  "hostops_child_agent",
				ResourceID:    "childAgentId",
				OperationKind: "stop_child_agent",
			}},
			Idempotency: tooling.ToolIdempotencyMetadata{
				Strategy:      tooling.ToolIdempotencyStrategyArgumentsHash,
				PostCheckRefs: []string{ToolWaitHostAgents + " for the child mission"},
			},
		},
		Visibility:      managerToolVisibility(),
		InputSchemaData: json.RawMessage(`{"type":"object","required":["childAgentId"],"properties":{"childAgentId":{"type":"string"}}}`),
		ReadOnlyFunc:    func(json.RawMessage) bool { return false },
		ExecuteFunc: func(ctx context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			var req stopHostAgentInput
			if err := json.Unmarshal(input, &req); err != nil {
				return tooling.ToolResult{}, err
			}
			child, err := orchestrator.Stop(ctx, req.ChildAgentID)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return jsonToolResult(ToolStopHostAgent, map[string]any{"child": childAgentResultContract(child, true)})
		},
	}
}

func managerToolVisibility() tooling.Visibility {
	return tooling.Visibility{SessionTypes: []string{"workspace"}, Modes: []string{"plan", "execute"}}
}

func spawnHostAgentContract(children []HostChildAgent) map[string]any {
	return map[string]any{
		"schemaVersion":  "aiops.hostops.child/v1",
		"noHostMutation": true,
		"children":       childAgentResultContracts(children, true),
	}
}

func waitHostAgentsContract(children []HostChildAgent) map[string]any {
	return map[string]any{
		"schemaVersion": "aiops.hostops.wait/v1",
		"children":      childAgentResultContracts(children, false),
	}
}

func childAgentResultContracts(children []HostChildAgent, includeNoHostMutation bool) []map[string]any {
	out := make([]map[string]any, 0, len(children))
	for _, child := range children {
		out = append(out, childAgentResultContract(child, includeNoHostMutation))
	}
	return out
}

func childAgentResultContract(child HostChildAgent, includeNoHostMutation bool) map[string]any {
	item := map[string]any{
		"id":                child.ID,
		"childAgentId":      child.ID,
		"missionId":         child.MissionID,
		"parentAgentId":     child.ParentAgentID,
		"sessionId":         child.SessionID,
		"targetRef":         childTargetRef(child),
		"hostId":            child.HostID,
		"hostAddress":       child.HostAddress,
		"hostDisplayName":   child.HostDisplayName,
		"role":              child.Role,
		"task":              child.Task,
		"status":            child.Status,
		"planStepIds":       child.PlanStepIDs,
		"lastInputPreview":  child.LastInputPreview,
		"lastOutputPreview": child.LastOutputPreview,
		"error":             child.Error,
		"startedAt":         child.StartedAt,
		"updatedAt":         child.UpdatedAt,
		"completedAt":       child.CompletedAt,
		"evidenceRefs":      []string{},
		"blockerRefs":       childBlockerRefs(child),
	}
	if includeNoHostMutation {
		item["noHostMutation"] = true
	}
	return item
}

func childTargetRef(child HostChildAgent) string {
	if child.HostID != "" {
		return child.HostID
	}
	if child.HostAddress != "" {
		return child.HostAddress
	}
	return child.HostDisplayName
}

func childBlockerRefs(child HostChildAgent) []string {
	if child.Error == "" {
		return []string{}
	}
	return []string{"hostops:blocker:" + child.ID}
}

func jsonToolResult(toolName string, payload any) (tooling.ToolResult, error) {
	data, err := json.Marshal(payload)
	if err != nil {
		return tooling.ToolResult{}, fmt.Errorf("%s result: %w", toolName, err)
	}
	return tooling.ToolResult{
		Content: string(data),
		Display: &tooling.ToolDisplayPayload{
			Type:  "hostops." + toolName,
			Title: toolName,
			Data:  data,
		},
	}, nil
}
