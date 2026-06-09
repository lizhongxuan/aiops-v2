package planning

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/tooling"
)

type ClaimNextTaskArgs struct {
	Owner   string `json:"owner"`
	AgentID string `json:"agentId,omitempty"`
}

type ClaimNextTaskResult struct {
	Claimed            bool     `json:"claimed"`
	TaskID             string   `json:"taskId,omitempty"`
	LeaseID            string   `json:"leaseId,omitempty"`
	ExpiresAt          string   `json:"expiresAt,omitempty"`
	Owner              string   `json:"owner,omitempty"`
	AgentID            string   `json:"agentId,omitempty"`
	DependsOnSatisfied []string `json:"dependsOnSatisfied,omitempty"`
	BlockedCount       int      `json:"blockedCount"`
	Reason             string   `json:"reason,omitempty"`
}

func NewClaimNextTaskTool(store *TaskStore, now func() time.Time) tooling.Tool {
	if now == nil {
		now = time.Now
	}
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "claim_next_task",
			Origin:      tooling.ToolOriginMeta,
			Description: "Claim the next eligible pending task from the active plan. Use only for plan/task coordination; it returns a compact lease and never executes the task.",
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"plan", "execute"}},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"owner": {"type": "string"},
				"agentId": {"type": "string"}
			},
			"required": ["owner"]
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"claimed": {"type": "boolean"},
				"taskId": {"type": "string"},
				"leaseId": {"type": "string"},
				"expiresAt": {"type": "string"},
				"owner": {"type": "string"},
				"agentId": {"type": "string"},
				"dependsOnSatisfied": {"type": "array", "items": {"type": "string"}},
				"blockedCount": {"type": "integer"},
				"reason": {"type": "string"}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return false },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := decodeClaimNextTaskArgs(input)
			return err
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			args, err := decodeClaimNextTaskArgs(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			if store == nil {
				return tooling.ToolResult{}, fmt.Errorf("task store is required")
			}
			outcome := store.ClaimNextDetailed(args.Owner, args.AgentID, now())
			payload := ClaimNextTaskResult{
				Claimed:            outcome.Claimed,
				BlockedCount:       outcome.BlockedCount,
				Reason:             outcome.Reason,
				DependsOnSatisfied: outcome.DependsOnSatisfied,
			}
			if outcome.Claimed {
				payload.TaskID = outcome.Claim.TaskID
				payload.LeaseID = outcome.Claim.LeaseID
				payload.ExpiresAt = outcome.Claim.ExpiresAt
				payload.Owner = outcome.Claim.Owner
				payload.AgentID = outcome.Claim.AgentID
			}
			raw, err := json.Marshal(payload)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: string(raw),
				Display: &tooling.ToolDisplayPayload{
					Type:  "plan_task_claim",
					Title: "Task claim updated",
					Data:  json.RawMessage(raw),
				},
			}, nil
		},
	}
}

func decodeClaimNextTaskArgs(input json.RawMessage) (ClaimNextTaskArgs, error) {
	var args ClaimNextTaskArgs
	if len(input) == 0 {
		return args, fmt.Errorf("claim_next_task input is required")
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return args, fmt.Errorf("decode claim_next_task input: %w", err)
	}
	args.Owner = stringsTrim(args.Owner)
	args.AgentID = stringsTrim(args.AgentID)
	if args.Owner == "" {
		return args, fmt.Errorf("owner is required")
	}
	return args, nil
}

func stringsTrim(value string) string {
	return strings.TrimSpace(value)
}
