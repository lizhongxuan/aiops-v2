package promptcompiler

import (
	"fmt"
	"strings"
)

func normalizeProtocolState(state ProtocolPromptState) ProtocolPromptState {
	planMode := normalizePlanModePromptState(state.PlanMode)
	taskTodo := normalizeTaskTodoPromptState(state.TaskTodo)
	failureSwitchPath := normalizeFailureSwitchPathPromptState(state.FailureSwitchPath)
	if len(state.Items) == 0 && planMode == nil && taskTodo == nil && failureSwitchPath == nil {
		return ProtocolPromptState{}
	}
	items := make([]ProtocolPromptItem, 0, len(state.Items))
	for _, item := range state.Items {
		kind := strings.TrimSpace(item.Kind)
		text := strings.TrimSpace(item.Text)
		if kind == "" && text == "" {
			continue
		}
		items = append(items, ProtocolPromptItem{
			Kind:   kind,
			ID:     strings.TrimSpace(item.ID),
			Status: strings.TrimSpace(item.Status),
			Text:   text,
		})
	}
	return ProtocolPromptState{
		Items:             items,
		PlanMode:          planMode,
		TaskTodo:          taskTodo,
		FailureSwitchPath: failureSwitchPath,
	}
}

func renderProtocolPromptState(state ProtocolPromptState) string {
	if len(state.Items) == 0 && state.PlanMode == nil && state.TaskTodo == nil && state.FailureSwitchPath == nil {
		return ""
	}
	var sections []string
	if plan := renderPlanModePromptState(state.PlanMode); plan != "" {
		sections = append(sections, plan)
	}
	if task := renderTaskTodoPromptState(state.TaskTodo); task != "" {
		sections = append(sections, task)
	}
	if failureSwitchPath := renderFailureSwitchPathPromptState(state.FailureSwitchPath); failureSwitchPath != "" {
		sections = append(sections, failureSwitchPath)
	}
	if len(state.Items) == 0 {
		return strings.Join(sections, "\n\n")
	}
	lines := []string{"## Protocol State", "Treat these as protocol-level state items, not conversational prose."}
	for _, item := range state.Items {
		attrs := []string{}
		if item.Kind != "" {
			attrs = append(attrs, "kind="+item.Kind)
		}
		if item.ID != "" {
			attrs = append(attrs, "id="+item.ID)
		}
		if item.Status != "" {
			attrs = append(attrs, "status="+item.Status)
		}
		line := "- " + strings.Join(attrs, " ")
		if strings.TrimSpace(line) == "-" {
			line = "- item"
		}
		if item.Text != "" {
			line = fmt.Sprintf("%s: %s", line, item.Text)
		}
		lines = append(lines, line)
	}
	sections = append(sections, strings.Join(lines, "\n"))
	return strings.Join(sections, "\n\n")
}

func advanceProtocolStateAfterRender(state ProtocolPromptState) ProtocolPromptState {
	if state.PlanMode == nil || state.PlanMode.ReminderLevel != "full" {
		return state
	}
	next := state
	plan := *state.PlanMode
	plan.FullInstructionInjected = true
	plan.ReminderLevel = "sparse"
	next.PlanMode = &plan
	return next
}

func normalizePlanModePromptState(state *PlanModePromptState) *PlanModePromptState {
	if state == nil {
		return nil
	}
	out := *state
	out.State = strings.TrimSpace(out.State)
	out.PlanID = strings.TrimSpace(out.PlanID)
	out.ArtifactStatus = strings.TrimSpace(out.ArtifactStatus)
	out.ApprovalStatus = strings.TrimSpace(out.ApprovalStatus)
	out.ReminderLevel = strings.TrimSpace(strings.ToLower(out.ReminderLevel))
	out.RejectionReason = strings.TrimSpace(out.RejectionReason)
	if out.State == "" && out.PlanID == "" && out.ArtifactStatus == "" && out.ApprovalStatus == "" && out.ReminderLevel == "" && out.PendingQuestions == 0 && out.OpenQuestions == 0 && out.RejectionReason == "" {
		return nil
	}
	switch out.ReminderLevel {
	case "full":
		out.FullInstructionInjected = true
	case "sparse", "resume":
	case "":
		if out.FullInstructionInjected {
			out.ReminderLevel = "sparse"
		} else {
			out.ReminderLevel = "full"
			out.FullInstructionInjected = true
		}
	default:
		out.ReminderLevel = "sparse"
	}
	return &out
}

func normalizeTaskTodoPromptState(state *TaskTodoPromptState) *TaskTodoPromptState {
	if state == nil || len(state.Items) == 0 {
		return nil
	}
	out := &TaskTodoPromptState{Items: make([]TaskTodoPromptItem, 0, len(state.Items))}
	for _, item := range state.Items {
		item.ID = strings.TrimSpace(item.ID)
		item.Status = strings.TrimSpace(item.Status)
		item.Owner = strings.TrimSpace(item.Owner)
		item.BlockedBy = strings.TrimSpace(item.BlockedBy)
		item.PendingEvidence = strings.TrimSpace(item.PendingEvidence)
		if item.ID == "" && item.Status == "" && item.Owner == "" && item.BlockedBy == "" && item.PendingEvidence == "" {
			continue
		}
		out.Items = append(out.Items, item)
	}
	if len(out.Items) == 0 {
		return nil
	}
	return out
}

func normalizeFailureSwitchPathPromptState(state *FailureSwitchPathPromptState) *FailureSwitchPathPromptState {
	if state == nil {
		return nil
	}
	out := *state
	out.Signature = strings.TrimSpace(out.Signature)
	out.Action = strings.TrimSpace(out.Action)
	out.SwitchPathReason = strings.TrimSpace(out.SwitchPathReason)
	if out.Action == "" && out.SwitchPathReason != "" {
		out.Action = "do_not_repeat_same_path"
	}
	if out.Signature == "" && out.Action == "" && out.SwitchPathReason == "" && out.SeenCount <= 0 {
		return nil
	}
	return &out
}

func renderPlanModePromptState(state *PlanModePromptState) string {
	if state == nil {
		return ""
	}
	switch state.ReminderLevel {
	case "resume":
		return renderPlanModeResumeReminder(state)
	case "sparse":
		return renderPlanModeSparseReminder(state)
	default:
		return renderPlanModeFullProtocol(state)
	}
}

func renderPlanModeFullProtocol(state *PlanModePromptState) string {
	lines := []string{
		"## Plan Mode Full Protocol",
		"Plan Mode is active. You may only inspect, update PlanArtifact, ask the smallest necessary question, or request plan approval.",
		"Do not execute mutations, write ordinary files, run non-read-only terminal commands, or call mutating external tools.",
		"After each newly discovered fact, update the PlanArtifact before continuing.",
		"If information is insufficient, ask only the smallest necessary question.",
		"PlanArtifact must contain Context, Approach, Scope, Reuse, Verification, and Open Questions.",
		"Before requesting exit approval, ensure Open Questions is zero or every open question has a blocker reason.",
	}
	lines = append(lines, renderPlanModeStateLines(state)...)
	return strings.Join(lines, "\n")
}

func renderPlanModeSparseReminder(state *PlanModePromptState) string {
	lines := []string{
		"## Plan Mode Sparse Reminder",
	}
	lines = append(lines, renderPlanModeStateLines(state)...)
	lines = append(lines, "protocol: inspect -> update plan -> ask smallest missing question -> request approval")
	return strings.Join(lines, "\n")
}

func renderPlanModeResumeReminder(state *PlanModePromptState) string {
	lines := []string{
		"## Plan Mode Resume Reminder",
	}
	lines = append(lines, renderPlanModeStateLines(state)...)
	lines = append(lines, "resume_protocol: restore compact Plan Mode state, continue read-only inspection, update plan, then request approval when questions are resolved")
	return strings.Join(lines, "\n")
}

func renderPlanModeStateLines(state *PlanModePromptState) []string {
	var lines []string
	if state.State != "" {
		lines = append(lines, "state: "+state.State)
	}
	if state.PlanID != "" {
		lines = append(lines, "plan_id: "+state.PlanID)
	}
	if state.ArtifactStatus != "" {
		lines = append(lines, "artifact_status: "+state.ArtifactStatus)
	}
	if state.ApprovalStatus != "" {
		lines = append(lines, "approval_status: "+state.ApprovalStatus)
	}
	lines = append(lines, fmt.Sprintf("pending_questions: %d", state.PendingQuestions))
	lines = append(lines, fmt.Sprintf("open_questions: %d", state.OpenQuestions))
	if state.RejectionReason != "" {
		lines = append(lines, "rejection_reason: "+state.RejectionReason)
	}
	return lines
}

func renderTaskTodoPromptState(state *TaskTodoPromptState) string {
	if state == nil || len(state.Items) == 0 {
		return ""
	}
	lines := []string{"## Compact Task/Todo State"}
	for _, item := range state.Items {
		parts := []string{item.Status + ": " + item.ID}
		if item.Owner != "" {
			parts = append(parts, "owner="+item.Owner)
		}
		if item.BlockedBy != "" {
			parts = append(parts, "blocked_by="+item.BlockedBy)
		}
		if item.PendingEvidence != "" {
			parts = append(parts, "pending_evidence="+item.PendingEvidence)
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	return strings.Join(lines, "\n")
}

func renderFailureSwitchPathPromptState(state *FailureSwitchPathPromptState) string {
	if state == nil {
		return ""
	}
	lines := []string{"## Failure Switch-path State"}
	if state.Action != "" {
		lines = append(lines, "action: "+state.Action)
	}
	if state.Signature != "" {
		lines = append(lines, "signature: "+state.Signature)
	}
	if state.SeenCount > 0 {
		lines = append(lines, fmt.Sprintf("seen_count: %d", state.SeenCount))
	}
	if state.SwitchPathReason != "" {
		lines = append(lines, "switch_path_reason: "+state.SwitchPathReason)
	}
	lines = append(lines, "do_not_repeat_same_path: choose an independent safe evidence path or ask for the smallest missing input before retrying.")
	return strings.Join(lines, "\n")
}
