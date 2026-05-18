package planning

import (
	"context"
	"encoding/json"
	"fmt"

	"aiops-v2/internal/tooling"
)

func (a UpdatePlanArgs) Validate() error {
	_, err := normalizeUpdatePlanArgs(a)
	return err
}

// NewUpdatePlanTool returns the runtime-state tool used by the model to keep
// structured plan/todo state out of free-form assistant text.
func NewUpdatePlanTool() tooling.Tool {
	return &tooling.StaticTool{
		Meta: tooling.ToolMetadata{
			Name:        "update_plan",
			Aliases:     []string{"plan_update", "set_plan"},
			Origin:      tooling.ToolOriginMeta,
			Description: "Update the current turn's structured plan/todo state. Use for multi-step work; skip for simple answers.",
		},
		Visibility: tooling.Visibility{SessionTypes: []string{"host", "workspace"}, Modes: []string{"plan", "execute"}},
		InputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"status": {"type": "string", "enum": ["active", "completed", "failed", "cancelled"]},
				"steps": {
					"type": "array",
					"items": {
						"type": "object",
						"properties": {
							"id": {"type": "string"},
							"text": {"type": "string"},
							"status": {"type": "string", "enum": ["pending", "in_progress", "completed", "blocked", "failed", "cancelled"]},
							"summary": {"type": "string"}
						},
						"required": ["text"]
					}
				}
			},
			"required": ["steps"]
		}`),
		OutputSchemaData: json.RawMessage(`{
			"type": "object",
			"properties": {
				"summary": {"type": "string"}
			}
		}`),
		ReadOnlyFunc:        func(json.RawMessage) bool { return true },
		DestructiveFunc:     func(json.RawMessage) bool { return false },
		ConcurrencySafeFunc: func(json.RawMessage) bool { return true },
		CheckPermissionsFunc: func(context.Context, json.RawMessage) tooling.PermissionDecision {
			return tooling.PermissionDecision{Action: tooling.PermissionActionAllow}
		},
		ValidateInputFunc: func(_ context.Context, input json.RawMessage) error {
			_, err := DecodeUpdatePlan(input)
			return err
		},
		ExecuteFunc: func(_ context.Context, input json.RawMessage) (tooling.ToolResult, error) {
			plan, err := DecodeUpdatePlan(input)
			if err != nil {
				return tooling.ToolResult{}, err
			}
			return tooling.ToolResult{
				Content: CompactSummary(plan),
				Display: &tooling.ToolDisplayPayload{
					Type:  "plan",
					Title: "Plan updated",
				},
			}, nil
		},
	}
}

// DecodeUpdatePlan validates tool input and returns the normalized plan state.
func DecodeUpdatePlan(input json.RawMessage) (PlanState, error) {
	var args UpdatePlanArgs
	if len(input) == 0 {
		return PlanState{}, fmt.Errorf("update_plan input is required")
	}
	if err := json.Unmarshal(input, &args); err != nil {
		return PlanState{}, fmt.Errorf("decode update_plan input: %w", err)
	}
	return ApplyPlanUpdate(PlanState{}, args)
}

// CompactSummary renders a stable one-line summary for tool results and plan
// TurnItems.
func CompactSummary(plan PlanState) string {
	total := len(plan.Steps)
	if total == 0 {
		return fmt.Sprintf("plan updated: %s (0 steps)", plan.Status)
	}
	counts := map[StepStatus]int{}
	for _, step := range plan.Steps {
		counts[step.Status]++
	}
	for _, status := range []StepStatus{StepStatusInProgress, StepStatusBlocked, StepStatusFailed, StepStatusCompleted, StepStatusPending, StepStatusCancelled} {
		if counts[status] > 0 {
			return fmt.Sprintf("plan updated: %s (%d/%d %s)", plan.Status, counts[status], total, status)
		}
	}
	return fmt.Sprintf("plan updated: %s (%d steps)", plan.Status, total)
}
