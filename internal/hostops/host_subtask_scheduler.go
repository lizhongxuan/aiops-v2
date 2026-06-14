package hostops

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"aiops-v2/internal/opssemantic"
)

var ErrInvalidHostSubTaskSchedule = errors.New("invalid host subtask schedule request")

type HostSubTaskScheduler struct {
	store hostSubTaskScheduleStore
}

type hostSubTaskScheduleStore interface {
	SaveHostSubTaskScheduleDecision(context.Context, HostSubTaskScheduleDecision) error
	ActiveHostSubTaskID(context.Context, string, string) (string, bool, error)
}

func NewHostSubTaskScheduler(store hostSubTaskScheduleStore) *HostSubTaskScheduler {
	return &HostSubTaskScheduler{store: store}
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
