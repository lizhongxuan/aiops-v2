package workfloweditor

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

type WorkflowEditPlanner interface {
	BuildWorkflowEditPlan(context.Context, WorkflowEditPlanningRequest) (WorkflowEditPlan, error)
}

type WorkflowEditPlanningRequest struct {
	WorkflowID      string
	DrawerSessionID string
	Message         string
	Describe        DescribeResult
}

type DefaultWorkflowEditPlanner struct{}

func (DefaultWorkflowEditPlanner) BuildWorkflowEditPlan(ctx context.Context, req WorkflowEditPlanningRequest) (WorkflowEditPlan, error) {
	if err := ctx.Err(); err != nil {
		return WorkflowEditPlan{}, err
	}
	message := strings.TrimSpace(req.Message)
	if message == "" {
		return WorkflowEditPlan{}, errors.New("workflow edit message is required")
	}
	return WorkflowEditPlan{}, errors.New("workflow edit planner is not configured; configure an LLM-backed workflow edit planner")
}

func normalizeWorkflowEditPlan(plan WorkflowEditPlan, req ProposeEditPlanRequest) (WorkflowEditPlan, error) {
	message := strings.TrimSpace(firstNonEmpty(plan.Message, req.Message))
	if message == "" {
		return WorkflowEditPlan{}, errors.New("workflow edit message is required")
	}
	out := WorkflowEditPlan{
		ID:         firstNonEmpty(plan.ID, stableID("plan", firstNonEmpty(req.DrawerSessionID, req.WorkflowID, message))),
		WorkflowID: strings.TrimSpace(firstNonEmpty(plan.WorkflowID, req.WorkflowID)),
		Message:    message,
		CreatedAt:  plan.CreatedAt,
	}
	if out.CreatedAt.IsZero() {
		out.CreatedAt = time.Now().UTC()
	}
	for index, item := range plan.Items {
		title := normalizePlanItemTitlePrefix(strings.TrimSpace(item.Title))
		description := strings.TrimSpace(item.Description)
		if title == "" && description == "" {
			continue
		}
		if title == "" || strings.EqualFold(title, message) || planItemTitleLooksBroken(title) {
			title = fallbackPlanItemTitle(index, description)
		}
		if planItemLooksLikeDeadRule(title, description) {
			return WorkflowEditPlan{}, fmt.Errorf("workflow edit planner returned template plan item %q", title)
		}
		id := strings.TrimSpace(item.ID)
		if id == "" {
			id = stableID("item", fmt.Sprintf("%02d-%s", index+1, title))
		}
		out.Items = append(out.Items, WorkflowEditPlanItem{
			ID:                id,
			Title:             title,
			Description:       description,
			Status:            firstNonEmpty(item.Status, "pending"),
			Goal:              strings.TrimSpace(item.Goal),
			Environment:       strings.TrimSpace(item.Environment),
			NodeLabel:         strings.TrimSpace(item.NodeLabel),
			NodeType:          strings.TrimSpace(item.NodeType),
			NodeAction:        strings.TrimSpace(item.NodeAction),
			ScriptSummary:     strings.TrimSpace(item.ScriptSummary),
			ValidationSummary: strings.TrimSpace(item.ValidationSummary),
			InputVariables:    normalizeWorkflowVariableSpecs(item.InputVariables),
			OutputVariables:   normalizeWorkflowVariableSpecs(item.OutputVariables),
			Script:            strings.TrimSpace(item.Script),
		})
	}
	if len(out.Items) == 0 {
		return WorkflowEditPlan{}, errors.New("workflow edit plan has no valid items")
	}
	return out, nil
}

func normalizeWorkflowVariableSpecs(values []WorkflowVariableSpec) []WorkflowVariableSpec {
	out := make([]WorkflowVariableSpec, 0, len(values))
	for _, value := range values {
		name := strings.TrimSpace(value.Name)
		if name == "" {
			continue
		}
		out = append(out, WorkflowVariableSpec{
			Name:     name,
			Type:     strings.TrimSpace(value.Type),
			Required: value.Required,
			Source:   strings.TrimSpace(value.Source),
		})
	}
	return out
}

func fallbackPlanItemTitle(index int, description string) string {
	trimmed := strings.TrimSpace(description)
	if trimmed != "" {
		fragment := firstPlanTitleFragment(trimmed)
		if fragment != "" {
			return fragment
		}
	}
	return fmt.Sprintf("步骤 %d", index+1)
}

func normalizePlanItemTitlePrefix(title string) string {
	title = strings.TrimSpace(title)
	for _, separator := range []string{"：", ":", ".", "、"} {
		for _, prefix := range []string{"步骤 ", "步骤", "step ", "Step "} {
			lowerTitle := strings.ToLower(title)
			lowerPrefix := strings.ToLower(prefix)
			if strings.HasPrefix(lowerTitle, lowerPrefix) {
				index := strings.Index(title, separator)
				if index > 0 && index+len(separator) < len(title) {
					return strings.TrimSpace(title[index+len(separator):])
				}
			}
		}
	}
	return title
}

func planItemTitleLooksBroken(title string) bool {
	trimmed := strings.TrimSpace(title)
	if trimmed == "" {
		return true
	}
	return strings.Count(trimmed, "“") != strings.Count(trimmed, "”") ||
		strings.Count(trimmed, "「") != strings.Count(trimmed, "」") ||
		strings.Count(trimmed, "\"")%2 != 0 ||
		strings.HasSuffix(trimmed, "“") ||
		strings.HasSuffix(trimmed, "「")
}

func firstPlanTitleFragment(description string) string {
	trimmed := strings.TrimSpace(description)
	if trimmed == "" {
		return ""
	}
	cut := len(trimmed)
	for _, separator := range []string{"。", "\n", "；", ";"} {
		if index := strings.Index(trimmed, separator); index > 0 && index < cut {
			cut = index
		}
	}
	fragment := strings.TrimSpace(trimmed[:cut])
	runes := []rune(fragment)
	if len(runes) > 36 {
		fragment = strings.TrimSpace(string(runes[:36])) + "..."
	}
	return fragment
}

func planItemLooksLikeDeadRule(title, description string) bool {
	text := strings.ToLower(title + " " + description)
	return strings.Contains(text, strings.ToLower("生成一个最小 Workflow patch")) ||
		strings.Contains(text, strings.ToLower("Review and apply one workflow patch at a time"))
}
