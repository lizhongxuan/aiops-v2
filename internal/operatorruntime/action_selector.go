package operatorruntime

import (
	"errors"
	"fmt"
)

var ErrWorkflowBindingMissing = errors.New("workflow binding missing")

type ActionSelection struct {
	Action          ActionCatalogItem `json:"action"`
	WorkflowBinding WorkflowBinding   `json:"workflowBinding"`
}

func SelectAction(problem ProblemType, actions []ActionCatalogItem, bindings []WorkflowBinding) (ActionSelection, error) {
	actionByID := mapByID(actions, func(item ActionCatalogItem) string { return item.ID })
	bindingByAction := map[string]WorkflowBinding{}
	for _, binding := range bindings {
		bindingByAction[binding.ActionRef] = binding
	}
	for _, ref := range problem.RecommendedActionRefs {
		action, ok := actionByID[ref]
		if !ok {
			continue
		}
		binding, ok := bindingByAction[action.ID]
		if !ok {
			return ActionSelection{}, ErrWorkflowBindingMissing
		}
		return ActionSelection{Action: action, WorkflowBinding: binding}, nil
	}
	return ActionSelection{}, fmt.Errorf("recommended action not found")
}
