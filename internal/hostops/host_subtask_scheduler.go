package hostops

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/opssemantic"
)

var ErrInvalidHostSubTaskSchedule = errors.New("invalid host subtask schedule request")

type HostManagerRuntimeLimits struct {
	MaxChildAgents  int           `json:"maxChildAgents"`
	MaxChildRuntime time.Duration `json:"maxChildRuntime"`
}

type HostSubTaskScheduler struct {
	store  hostSubTaskScheduleStore
	limits HostManagerRuntimeLimits
}

type hostSubTaskScheduleStore interface {
	SaveHostSubTaskScheduleDecision(context.Context, HostSubTaskScheduleDecision) error
	ActiveHostSubTaskID(context.Context, string, string) (string, bool, error)
}

func NewHostSubTaskScheduler(store hostSubTaskScheduleStore) *HostSubTaskScheduler {
	return NewHostSubTaskSchedulerWithLimits(store, HostManagerRuntimeLimits{})
}

func NewHostSubTaskSchedulerWithLimits(store hostSubTaskScheduleStore, limits HostManagerRuntimeLimits) *HostSubTaskScheduler {
	return &HostSubTaskScheduler{store: store, limits: normalizeHostManagerRuntimeLimits(limits)}
}

func normalizeHostManagerRuntimeLimits(limits HostManagerRuntimeLimits) HostManagerRuntimeLimits {
	if limits.MaxChildAgents <= 0 {
		limits.MaxChildAgents = 8
	}
	if limits.MaxChildRuntime <= 0 {
		limits.MaxChildRuntime = 30 * time.Minute
	}
	return limits
}

func (s *HostSubTaskScheduler) Schedule(ctx context.Context, task HostSubTask) (HostSubTaskScheduleDecision, error) {
	task.ID = strings.TrimSpace(task.ID)
	task.MissionID = strings.TrimSpace(task.MissionID)
	task.HostID = strings.TrimSpace(task.HostID)
	task.PlanStepID = strings.TrimSpace(task.PlanStepID)
	if task.ID == "" {
		task.ID = "subtask-" + digestText(task.MissionID + ":" + task.HostID + ":" + task.PlanStepID)[:12]
	}
	if task.MissionID == "" || task.HostID == "" {
		return HostSubTaskScheduleDecision{}, ErrInvalidHostSubTaskSchedule
	}
	if s == nil || s.store == nil {
		return HostSubTaskScheduleDecision{}, ErrInvalidHostSubTaskSchedule
	}
	activeID, active, err := s.store.ActiveHostSubTaskID(ctx, task.MissionID, task.HostID)
	if err != nil {
		return HostSubTaskScheduleDecision{}, err
	}
	decision := HostSubTaskScheduleDecision{
		SubTaskID:   task.ID,
		MissionID:   task.MissionID,
		HostID:      task.HostID,
		PlanStepID:  task.PlanStepID,
		ToolCallID:  "tool-call:" + task.ID,
		EvidenceRef: "evidence://" + task.MissionID + "/" + task.HostID + "/" + task.ID,
	}
	if task.SchedulingDirective == HostSubTaskScheduleCancel {
		decision.Status = HostSubTaskStatusCancelled
		decision.ActiveSubTaskID = activeID
		return decision, s.store.SaveHostSubTaskScheduleDecision(ctx, decision)
	}
	if active && isWriteHostSubTask(task) {
		if task.SchedulingDirective == HostSubTaskScheduleSupersede {
			if strings.TrimSpace(task.ManagerRevisionReason) == "" {
				return decision, fmt.Errorf("%w: supersede requires manager revision reason", ErrInvalidHostSubTaskSchedule)
			}
			decision.Status = HostSubTaskStatusRunning
			decision.SupersededSubTaskID = activeID
			decision.ManagerRevisionReason = strings.TrimSpace(task.ManagerRevisionReason)
			return decision, s.store.SaveHostSubTaskScheduleDecision(ctx, decision)
		}
		decision.Status = HostSubTaskStatusQueued
		decision.ActiveSubTaskID = activeID
		decision.BlockingReason = "active_write_subtask_same_mission_host"
		return decision, s.store.SaveHostSubTaskScheduleDecision(ctx, decision)
	}
	decision.Status = HostSubTaskStatusRunning
	return decision, s.store.SaveHostSubTaskScheduleDecision(ctx, decision)
}

func (s *HostSubTaskScheduler) MergeChildReport(ctx context.Context, task HostSubTask, report HostTaskReport) (HostSubTaskScheduleDecision, error) {
	task.ID = strings.TrimSpace(task.ID)
	task.MissionID = strings.TrimSpace(firstNonEmptyString(task.MissionID, report.MissionID))
	task.HostID = strings.TrimSpace(firstNonEmptyString(task.HostID, report.HostID))
	task.PlanStepID = strings.TrimSpace(firstNonEmptyString(task.PlanStepID, report.PlanStepID))
	if task.ID == "" {
		task.ID = "subtask-" + digestText(task.MissionID + ":" + task.HostID + ":" + task.PlanStepID)[:12]
	}
	if task.MissionID == "" || task.HostID == "" {
		return HostSubTaskScheduleDecision{}, ErrInvalidHostSubTaskSchedule
	}
	if s == nil || s.store == nil {
		return HostSubTaskScheduleDecision{}, ErrInvalidHostSubTaskSchedule
	}
	decision := HostSubTaskScheduleDecision{
		SubTaskID:      task.ID,
		MissionID:      task.MissionID,
		HostID:         task.HostID,
		PlanStepID:     task.PlanStepID,
		Status:         hostSubTaskStatusFromReportStatus(HostTaskReportStatus(strings.TrimSpace(report.Status))),
		ToolCallID:     "tool-call:" + task.ID,
		EvidenceRef:    firstEvidenceRef(report, task),
		BlockingReason: firstNonEmptyString(firstReportText(report.Blockers), firstReportText(report.Errors)),
	}
	return decision, s.store.SaveHostSubTaskScheduleDecision(ctx, decision)
}

func hostSubTaskStatusFromReportStatus(status HostTaskReportStatus) HostSubTaskStatus {
	switch status {
	case HostTaskReportStatusCompleted:
		return HostSubTaskStatusCompleted
	case HostTaskReportStatusBlockedApproval, HostTaskReportStatusNeedsUserApproval:
		return HostSubTaskStatusBlockedApproval
	case HostTaskReportStatusBlockedEvidence, HostTaskReportStatusBlocked, HostTaskReportStatusNeedsManagerCoordination:
		return HostSubTaskStatusBlockedEvidence
	case HostTaskReportStatusFailed:
		return HostSubTaskStatusFailed
	case HostTaskReportStatusCancelled:
		return HostSubTaskStatusCancelled
	case HostTaskReportStatusTimeout:
		return HostSubTaskStatusTimeout
	default:
		return HostSubTaskStatusFailed
	}
}

func firstEvidenceRef(report HostTaskReport, task HostSubTask) string {
	if ref := firstReportText(report.EvidenceRefs); ref != "" {
		return ref
	}
	for _, evidence := range report.Evidence {
		if strings.TrimSpace(evidence.ID) != "" {
			return strings.TrimSpace(evidence.ID)
		}
		if strings.TrimSpace(evidence.ArtifactRef) != "" {
			return strings.TrimSpace(evidence.ArtifactRef)
		}
	}
	return "evidence://" + task.MissionID + "/" + task.HostID + "/" + task.ID
}

func firstReportText(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func isWriteHostSubTask(task HostSubTask) bool {
	if task.ActionType == opssemantic.ActionWrite {
		return true
	}
	switch task.RiskLevel {
	case opssemantic.RiskLowWrite, opssemantic.RiskMediumWrite, opssemantic.RiskHighWrite, opssemantic.RiskDestructive:
		return true
	default:
		return false
	}
}
