package appui

import (
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/runtimekernel"
)

type PromptTraceCheckpointSummary struct {
	ID                 string    `json:"id"`
	Kind               string    `json:"kind,omitempty"`
	StepID             string    `json:"stepId,omitempty"`
	TurnID             string    `json:"turnId,omitempty"`
	Iteration          int       `json:"iteration,omitempty"`
	Resumable          bool      `json:"resumable,omitempty"`
	TargetRefs         []string  `json:"targetRefs,omitempty"`
	EvidenceRefs       []string  `json:"evidenceRefs,omitempty"`
	ApprovalState      string    `json:"approvalState,omitempty"`
	ToolSurfaceSummary string    `json:"toolSurfaceSummary,omitempty"`
	CreatedAt          time.Time `json:"createdAt,omitempty"`
}

func BuildCheckpointSummariesFromTurn(turn *runtimekernel.TurnSnapshot, run *AgentRunView) []PromptTraceCheckpointSummary {
	if turn == nil {
		return nil
	}
	seen := map[string]struct{}{}
	var summaries []PromptTraceCheckpointSummary
	for _, checkpoint := range orderedCheckpointsFromTurn(turn) {
		if checkpoint == nil || strings.TrimSpace(checkpoint.ID) == "" {
			continue
		}
		if _, ok := seen[checkpoint.ID]; ok {
			continue
		}
		seen[checkpoint.ID] = struct{}{}
		summary := buildCheckpointSummary(*checkpoint, turn, run)
		if strings.TrimSpace(summary.ID) != "" {
			summaries = append(summaries, summary)
		}
	}
	sort.SliceStable(summaries, func(i, j int) bool {
		left := summaries[i].CreatedAt
		right := summaries[j].CreatedAt
		if left.IsZero() || right.IsZero() || left.Equal(right) {
			return summaries[i].ID < summaries[j].ID
		}
		return left.Before(right)
	})
	return summaries
}

func orderedCheckpointsFromTurn(turn *runtimekernel.TurnSnapshot) []*runtimekernel.CheckpointMetadata {
	checkpoints := make([]*runtimekernel.CheckpointMetadata, 0, len(turn.Iterations)+1)
	for i := range turn.Iterations {
		if turn.Iterations[i].Checkpoint != nil {
			checkpoints = append(checkpoints, turn.Iterations[i].Checkpoint)
		}
	}
	if turn.LatestCheckpoint != nil {
		checkpoints = append(checkpoints, turn.LatestCheckpoint)
	}
	return checkpoints
}

func buildCheckpointSummary(checkpoint runtimekernel.CheckpointMetadata, turn *runtimekernel.TurnSnapshot, run *AgentRunView) PromptTraceCheckpointSummary {
	return PromptTraceCheckpointSummary{
		ID:                 strings.TrimSpace(checkpoint.ID),
		Kind:               runtimekernel.NormalizeCheckpointKind(checkpoint.Kind),
		StepID:             checkpointStepID(checkpoint, run),
		TurnID:             firstNonEmptyString(strings.TrimSpace(checkpoint.TurnID), strings.TrimSpace(turn.ID)),
		Iteration:          checkpoint.Iteration,
		Resumable:          checkpoint.Resumable || runtimekernel.CheckpointIsResumable(checkpoint.Lifecycle, checkpoint.ResumeState) || runtimekernel.CheckpointIsResumable(turn.Lifecycle, turn.ResumeState),
		TargetRefs:         checkpointTargetRefs(checkpoint, turn, run),
		EvidenceRefs:       checkpointEvidenceRefs(checkpoint, turn),
		ApprovalState:      checkpointApprovalState(checkpoint, turn),
		ToolSurfaceSummary: firstNonEmptyString(strings.TrimSpace(checkpoint.ToolSurfaceSummary), checkpointToolSurfaceSummary(turn)),
		CreatedAt:          checkpoint.CreatedAt,
	}
}

func checkpointStepID(checkpoint runtimekernel.CheckpointMetadata, run *AgentRunView) string {
	if stepID := strings.TrimSpace(checkpoint.CurrentStepID); stepID != "" {
		return stepID
	}
	if run == nil {
		return ""
	}
	checkpointID := strings.TrimSpace(checkpoint.ID)
	if checkpointID != "" {
		for _, step := range run.Steps {
			if strings.TrimSpace(step.CheckpointID) == checkpointID && strings.TrimSpace(step.ID) != "" {
				return strings.TrimSpace(step.ID)
			}
		}
	}
	if strings.TrimSpace(run.CheckpointID) == checkpointID {
		return strings.TrimSpace(run.CurrentStepID)
	}
	return ""
}

func checkpointTargetRefs(checkpoint runtimekernel.CheckpointMetadata, turn *runtimekernel.TurnSnapshot, run *AgentRunView) []string {
	var refs []string
	refs = append(refs, checkpoint.TargetRefs...)
	if turn != nil {
		refs = append(refs, strings.TrimSpace(turn.Metadata["aiops.target.ref"]))
		refs = append(refs, strings.TrimSpace(turn.Metadata["aiops.target.summary"]))
		for _, approval := range turn.PendingApprovals {
			refs = append(refs, strings.TrimSpace(approval.HostID))
			refs = append(refs, approval.ResourceScopes...)
		}
	}
	if run != nil {
		refs = append(refs, targetRefsFromSummary(run.TargetSummary)...)
	}
	return sortedUniqueStrings(refs)
}

func checkpointEvidenceRefs(checkpoint runtimekernel.CheckpointMetadata, turn *runtimekernel.TurnSnapshot) []string {
	var refs []string
	refs = append(refs, checkpoint.EvidenceRefs...)
	refs = append(refs, checkpoint.ExternalRefs...)
	if turn != nil {
		for _, ref := range turn.ExternalReferences {
			refs = append(refs, ref.ID)
		}
	}
	return sortedUniqueStrings(refs)
}

func checkpointApprovalState(checkpoint runtimekernel.CheckpointMetadata, turn *runtimekernel.TurnSnapshot) string {
	if state := strings.TrimSpace(checkpoint.ApprovalState); state != "" {
		return state
	}
	kind := runtimekernel.NormalizeCheckpointKind(checkpoint.Kind)
	if kind == runtimekernel.CheckpointKindApprovalWaiting || checkpoint.ResumeState == runtimekernel.TurnResumeStatePendingApproval {
		return "waiting"
	}
	if turn != nil && len(turn.PendingApprovals) > 0 {
		return "waiting"
	}
	if kind == runtimekernel.CheckpointKindApprovalResolved {
		return "resolved"
	}
	return ""
}

func checkpointToolSurfaceSummary(turn *runtimekernel.TurnSnapshot) string {
	if turn == nil || turn.ToolSurfaceSnapshot == nil {
		return ""
	}
	names := append([]string(nil), turn.ToolSurfaceSnapshot.ToolNames...)
	sort.Strings(names)
	if len(names) == 0 {
		return strings.TrimSpace(turn.ToolSurfaceSnapshot.Fingerprint)
	}
	return strings.Join(names, " / ")
}

func sortedUniqueStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	if len(out) == 0 {
		return nil
	}
	return out
}
