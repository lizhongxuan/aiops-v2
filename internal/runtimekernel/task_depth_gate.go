package runtimekernel

import (
	"strings"

	"aiops-v2/internal/promptcompiler"
	"aiops-v2/internal/promptinput"
	"aiops-v2/internal/taskdepth"
)

const prematureFinalGuardMetadataKey = "taskDepth.prematureFinalGuardTriggered"

type PlanRequirementDecision struct {
	Required      bool     `json:"required"`
	Reason        string   `json:"reason,omitempty"`
	ReminderLevel string   `json:"reminderLevel,omitempty"`
	Missing       []string `json:"missing,omitempty"`
}

func depthProfileFromTurnRequest(req TurnRequest) taskdepth.Profile {
	return taskdepth.Classify(taskdepth.Options{
		Input:    req.Input,
		Mode:     string(req.Mode),
		Metadata: req.Metadata,
	})
}

func applyDepthProfileToCompileContext(ctx promptcompiler.CompileContext, profile taskdepth.Profile, reasoningEffort string) promptcompiler.CompileContext {
	ctx.TaskDepth = profile
	ctx.ReasoningEffort = strings.TrimSpace(reasoningEffort)
	return ctx
}

func shouldGuardPrematureFinal(profile taskdepth.Profile, snapshot *TurnSnapshot, iteration int, assistantContent string) bool {
	finalAttempt := strings.TrimSpace(assistantContent) != ""
	decision := EvaluatePlanRequirement(profile, snapshot, finalAttempt)
	if !decision.Required {
		return false
	}
	if snapshot == nil || iteration > 0 {
		return false
	}
	if strings.TrimSpace(snapshot.Metadata[prematureFinalGuardMetadataKey]) == "true" {
		return false
	}
	if !finalAttempt {
		return false
	}
	if finalLooksLikeBlocker(assistantContent) {
		return false
	}
	return true
}

func EvaluatePlanRequirement(profile taskdepth.Profile, snapshot *TurnSnapshot, finalAttempt bool) PlanRequirementDecision {
	if !profile.RequiresPlan && !taskdepth.AtLeast(profile.Level, taskdepth.LevelMultiStep) {
		return PlanRequirementDecision{ReminderLevel: "none"}
	}
	missing := []string{}
	if !turnHasPlan(snapshot) {
		missing = append(missing, "plan")
	}
	if (profile.RequiresEvidence || taskdepth.AtLeast(profile.Level, taskdepth.LevelInvestigation)) && !turnHasEvidence(snapshot) {
		missing = append(missing, "evidence")
	}
	if profile.RequiresValidation || taskdepth.AtLeast(profile.Level, taskdepth.LevelOperations) {
		if snapshot == nil || strings.TrimSpace(snapshot.Metadata["validation.completed"]) != "true" {
			missing = append(missing, "validation")
		}
	}
	if taskdepth.AtLeast(profile.Level, taskdepth.LevelMultiAgent) {
		if snapshot == nil || strings.TrimSpace(snapshot.Metadata["task.claimed"]) != "true" {
			missing = append(missing, "task_claim")
		}
	}
	if len(missing) == 0 {
		return PlanRequirementDecision{ReminderLevel: "none"}
	}
	level := "soft"
	if finalAttempt {
		level = "hard"
	}
	return PlanRequirementDecision{
		Required:      true,
		Reason:        "task_depth_requires_plan",
		ReminderLevel: level,
		Missing:       missing,
	}
}

func planRequirementDecisionTrace(decision PlanRequirementDecision) *promptinput.PlanRequirementDecisionTrace {
	if !decision.Required && decision.ReminderLevel == "" {
		return nil
	}
	trace := &promptinput.PlanRequirementDecisionTrace{
		Required: decision.Required,
		Decision: decision.ReminderLevel,
		Reason:   decision.Reason,
		Signals:  append([]string(nil), decision.Missing...),
	}
	if trace.Decision == "" {
		trace.Decision = "none"
	}
	return trace
}

func prematureFinalGuardPrompt(profile taskdepth.Profile) string {
	return "## Premature final answer guard\nThis request is classified as " + string(profile.Level) + ". You produced a final answer without a plan or direct evidence. Continue the task: create or update a plan if available, gather the minimum direct evidence, or ask the smallest missing question. Do not finalize yet."
}

func markPrematureFinalGuard(snapshot *TurnSnapshot) {
	if snapshot == nil {
		return
	}
	if snapshot.Metadata == nil {
		snapshot.Metadata = map[string]string{}
	}
	snapshot.Metadata[prematureFinalGuardMetadataKey] = "true"
}

func turnHasPlan(snapshot *TurnSnapshot) bool {
	if snapshot == nil {
		return false
	}
	for _, item := range snapshot.AgentItems {
		if string(item.Type) == "plan" {
			return true
		}
	}
	for _, iteration := range snapshot.Iterations {
		for _, result := range iteration.ToolResults {
			if result.Display != nil && result.Display.Type == "plan" {
				return true
			}
		}
	}
	return false
}

func turnHasEvidence(snapshot *TurnSnapshot) bool {
	return countActualToolDispatches(snapshot) > 0
}

func finalLooksLikeBlocker(text string) bool {
	text = strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{"缺少", "需要你", "无法继续", "权限", "blocked", "approval", "请提供"} {
		if strings.Contains(text, marker) {
			return true
		}
	}
	return false
}
