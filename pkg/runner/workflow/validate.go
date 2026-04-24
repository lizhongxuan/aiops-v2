package workflow

import (
	"fmt"
	"sort"
	"strings"
)

type ValidationError struct {
	Issues []string
}

func (e *ValidationError) Error() string {
	if e == nil || len(e.Issues) == 0 {
		return "workflow validation failed"
	}
	return "workflow validation failed: " + strings.Join(e.Issues, "; ")
}

func (w *Workflow) Validate() error {
	var issues []string

	if w.Version == "" {
		issues = append(issues, "version is required")
	}
	if w.Name == "" {
		issues = append(issues, "name is required")
	}
	if len(w.Steps) == 0 {
		issues = append(issues, "steps must not be empty")
	}

	if w.Plan.Mode != "" && w.Plan.Mode != "manual-approve" && w.Plan.Mode != "auto" {
		issues = append(issues, fmt.Sprintf("plan.mode must be manual-approve or auto, got %q", w.Plan.Mode))
	}
	if w.Plan.Strategy != "" && w.Plan.Strategy != "sequential" {
		issues = append(issues, fmt.Sprintf("plan.strategy must be sequential, got %q", w.Plan.Strategy))
	}

	handlerNames := map[string]struct{}{}
	for _, h := range w.Handlers {
		if h.Name == "" {
			issues = append(issues, "handler name is required")
			continue
		}
		if _, exists := handlerNames[h.Name]; exists {
			issues = append(issues, fmt.Sprintf("handler name %q is duplicated", h.Name))
		}
		handlerNames[h.Name] = struct{}{}
		if h.Action == "" {
			issues = append(issues, fmt.Sprintf("handler %q action is required", h.Name))
		}
	}

	stepNames := map[string]struct{}{}
	for i, s := range w.Steps {
		stepLabel := fmt.Sprintf("steps[%d]", i)
		if s.Name == "" {
			issues = append(issues, fmt.Sprintf("%s name is required", stepLabel))
		} else {
			if _, exists := stepNames[s.Name]; exists {
				issues = append(issues, fmt.Sprintf("step name %q is duplicated", s.Name))
			}
			stepNames[s.Name] = struct{}{}
		}
		if s.Action == "" {
			issues = append(issues, fmt.Sprintf("%s action is required", stepLabel))
		}
		for _, notify := range s.Notify {
			if _, ok := handlerNames[notify]; !ok {
				issues = append(issues, fmt.Sprintf("%s notify handler %q not found", stepLabel, notify))
			}
		}
	}

	if len(issues) > 0 {
		sort.Strings(issues)
		return &ValidationError{Issues: issues}
	}

	return nil
}
