package agentstate

import (
	"fmt"
	"strings"
)

func (s AgentState) Validate() error {
	if strings.TrimSpace(s.SessionID) == "" {
		return fmt.Errorf("session id is required")
	}
	if strings.TrimSpace(s.TurnID) == "" {
		return fmt.Errorf("turn id is required")
	}
	if !s.Phase.IsValid() {
		return fmt.Errorf("invalid phase %q", s.Phase)
	}
	seen := map[string]bool{}
	for i, item := range s.Items {
		if err := item.Validate(); err != nil {
			return fmt.Errorf("item[%d]: %w", i, err)
		}
		if seen[item.ID] {
			return fmt.Errorf("duplicate item id %q", item.ID)
		}
		seen[item.ID] = true
	}
	for i, step := range s.Plan.Steps {
		if strings.TrimSpace(step.Text) == "" {
			return fmt.Errorf("plan step[%d] text is required", i)
		}
		if !step.Status.IsValid() {
			return fmt.Errorf("plan step[%d] invalid status %q", i, step.Status)
		}
	}
	return nil
}

func (i TurnItem) Validate() error {
	if strings.TrimSpace(i.ID) == "" {
		return fmt.Errorf("item id is required")
	}
	if !i.Type.IsValid() {
		return fmt.Errorf("invalid item type %q", i.Type)
	}
	if !i.Status.IsValid() {
		return fmt.Errorf("invalid item status %q", i.Status)
	}
	return nil
}
