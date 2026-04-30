package agentstate

import (
	"fmt"
	"time"
)

type ItemUpdater func(TurnItem) (TurnItem, error)

func AppendItem(state AgentState, item TurnItem) (AgentState, error) {
	if err := item.Validate(); err != nil {
		return AgentState{}, err
	}
	next := cloneState(state)
	for _, existing := range next.Items {
		if existing.ID == item.ID {
			return AgentState{}, fmt.Errorf("item %q already exists", item.ID)
		}
	}
	now := time.Now().UTC()
	if item.CreatedAt.IsZero() {
		item.CreatedAt = now
	}
	if item.UpdatedAt.IsZero() {
		item.UpdatedAt = item.CreatedAt
	}
	next.Items = append(next.Items, item)
	if err := next.Validate(); err != nil {
		return AgentState{}, err
	}
	return next, nil
}

func UpdateItem(state AgentState, itemID string, update ItemUpdater) (AgentState, error) {
	if update == nil {
		return AgentState{}, fmt.Errorf("update function is required")
	}
	next := cloneState(state)
	for i, item := range next.Items {
		if item.ID != itemID {
			continue
		}
		updated, err := update(item)
		if err != nil {
			return AgentState{}, err
		}
		if updated.ID == "" {
			updated.ID = item.ID
		}
		if updated.ID != itemID {
			return AgentState{}, fmt.Errorf("item id cannot change from %q to %q", itemID, updated.ID)
		}
		if updated.CreatedAt.IsZero() {
			updated.CreatedAt = item.CreatedAt
		}
		updated.UpdatedAt = time.Now().UTC()
		if err := updated.Validate(); err != nil {
			return AgentState{}, err
		}
		next.Items[i] = updated
		if err := next.Validate(); err != nil {
			return AgentState{}, err
		}
		return next, nil
	}
	return AgentState{}, fmt.Errorf("item %q not found", itemID)
}

func cloneState(state AgentState) AgentState {
	next := state
	next.Items = append([]TurnItem(nil), state.Items...)
	next.Plan.Steps = append([]PlanStep(nil), state.Plan.Steps...)
	next.Evidence.Required = append([]string(nil), state.Evidence.Required...)
	next.Evidence.Provided = append([]string(nil), state.Evidence.Provided...)
	next.Approvals.Pending = append([]string(nil), state.Approvals.Pending...)
	next.Approvals.Granted = append([]string(nil), state.Approvals.Granted...)
	next.Approvals.Denied = append([]string(nil), state.Approvals.Denied...)
	return next
}
