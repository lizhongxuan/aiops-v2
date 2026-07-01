package hostops

import (
	"context"
	"testing"
	"time"

	"aiops-v2/internal/opssemantic"
)

func TestHostSubTaskSchedulerQueuesSecondWriteTaskForSameHost(t *testing.T) {
	store := NewInMemoryMissionStore()
	scheduler := NewHostSubTaskScheduler(store)
	first, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:         "subtask-a",
		MissionID:  "mission-sched",
		PlanStepID: "step-a",
		HostID:     "host-a",
		ActionType: opssemantic.ActionWrite,
		RiskLevel:  opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("first Schedule error = %v", err)
	}
	if first.Status != HostSubTaskStatusRunning {
		t.Fatalf("first status = %s, want running", first.Status)
	}
	second, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:         "subtask-b",
		MissionID:  "mission-sched",
		PlanStepID: "step-b",
		HostID:     "host-a",
		ActionType: opssemantic.ActionWrite,
		RiskLevel:  opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("second Schedule error = %v", err)
	}
	if second.Status != HostSubTaskStatusQueued || second.ActiveSubTaskID != "subtask-a" {
		t.Fatalf("second decision = %#v, want queued behind subtask-a", second)
	}
}

func TestHostSubTaskSchedulerAllowsReadonlyTasksWithSeparateEvidenceRefs(t *testing.T) {
	store := NewInMemoryMissionStore()
	scheduler := NewHostSubTaskScheduler(store)
	first, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:         "subtask-a",
		MissionID:  "mission-sched",
		PlanStepID: "step-a",
		HostID:     "host-a",
		ActionType: opssemantic.ActionReadOnly,
		RiskLevel:  opssemantic.RiskReadOnly,
	})
	if err != nil {
		t.Fatalf("first Schedule error = %v", err)
	}
	second, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:         "subtask-b",
		MissionID:  "mission-sched",
		PlanStepID: "step-b",
		HostID:     "host-a",
		ActionType: opssemantic.ActionReadOnly,
		RiskLevel:  opssemantic.RiskReadOnly,
	})
	if err != nil {
		t.Fatalf("second Schedule error = %v", err)
	}
	if first.Status != HostSubTaskStatusRunning || second.Status != HostSubTaskStatusRunning {
		t.Fatalf("decisions = %#v / %#v, want both running", first, second)
	}
	if first.EvidenceRef == "" || second.EvidenceRef == "" || first.EvidenceRef == second.EvidenceRef || first.ToolCallID == second.ToolCallID {
		t.Fatalf("evidence refs/tool call ids = %#v / %#v, want independent refs", first, second)
	}
}

func TestHostSubTaskSchedulerSupersedesOnlyWhenManagerExplicitlyRequests(t *testing.T) {
	store := NewInMemoryMissionStore()
	scheduler := NewHostSubTaskScheduler(store)
	if _, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:         "subtask-a",
		MissionID:  "mission-sched",
		PlanStepID: "step-a",
		HostID:     "host-a",
		ActionType: opssemantic.ActionWrite,
		RiskLevel:  opssemantic.RiskLowWrite,
	}); err != nil {
		t.Fatalf("first Schedule error = %v", err)
	}
	noReason, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:                  "subtask-b",
		MissionID:           "mission-sched",
		PlanStepID:          "step-b",
		HostID:              "host-a",
		ActionType:          opssemantic.ActionWrite,
		RiskLevel:           opssemantic.RiskLowWrite,
		SchedulingDirective: HostSubTaskScheduleSupersede,
	})
	if err == nil || noReason.Status == HostSubTaskStatusSuperseded {
		t.Fatalf("decision = %#v err = %v, want supersede rejected without reason", noReason, err)
	}
	withReason, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:                    "subtask-c",
		MissionID:             "mission-sched",
		PlanStepID:            "step-c",
		HostID:                "host-a",
		ActionType:            opssemantic.ActionWrite,
		RiskLevel:             opssemantic.RiskLowWrite,
		SchedulingDirective:   HostSubTaskScheduleSupersede,
		ManagerRevisionReason: "plan changed after new host evidence",
	})
	if err != nil {
		t.Fatalf("supersede Schedule error = %v", err)
	}
	if withReason.Status != HostSubTaskStatusRunning || withReason.SupersededSubTaskID != "subtask-a" || withReason.ManagerRevisionReason == "" {
		t.Fatalf("decision = %#v, want new running task superseding subtask-a with reason", withReason)
	}
}

func TestHostSubTaskSchedulerPersistsActiveSubtaskID(t *testing.T) {
	store := NewInMemoryMissionStore()
	scheduler := NewHostSubTaskScheduler(store)
	decision, err := scheduler.Schedule(context.Background(), HostSubTask{
		ID:         "subtask-a",
		MissionID:  "mission-sched",
		PlanStepID: "step-a",
		HostID:     "host-a",
		ActionType: opssemantic.ActionWrite,
		RiskLevel:  opssemantic.RiskLowWrite,
	})
	if err != nil {
		t.Fatalf("Schedule error = %v", err)
	}
	active, ok, err := store.ActiveHostSubTaskID(context.Background(), "mission-sched", "host-a")
	if err != nil {
		t.Fatalf("ActiveHostSubTaskID error = %v", err)
	}
	if !ok || active != decision.SubTaskID {
		t.Fatalf("active = %q ok=%v, want %q", active, ok, decision.SubTaskID)
	}
}

func TestHostSubTaskSchedulerMergesChildReportTerminalStatuses(t *testing.T) {
	store := NewInMemoryMissionStore()
	scheduler := NewHostSubTaskSchedulerWithLimits(store, HostManagerRuntimeLimits{
		MaxChildAgents:  4,
		MaxChildRuntime: time.Minute,
	})
	for _, tc := range []struct {
		reportStatus HostTaskReportStatus
		wantStatus   HostSubTaskStatus
	}{
		{HostTaskReportStatusCompleted, HostSubTaskStatusCompleted},
		{HostTaskReportStatusBlockedApproval, HostSubTaskStatusBlockedApproval},
		{HostTaskReportStatusBlockedEvidence, HostSubTaskStatusBlockedEvidence},
		{HostTaskReportStatusFailed, HostSubTaskStatusFailed},
		{HostTaskReportStatusCancelled, HostSubTaskStatusCancelled},
		{HostTaskReportStatusTimeout, HostSubTaskStatusTimeout},
	} {
		t.Run(string(tc.reportStatus), func(t *testing.T) {
			decision, err := scheduler.MergeChildReport(context.Background(), HostSubTask{
				ID:         "subtask-" + string(tc.reportStatus),
				MissionID:  "mission-merge",
				PlanStepID: "step-" + string(tc.reportStatus),
				HostID:     "host-a",
			}, HostTaskReport{
				MissionID:    "mission-merge",
				PlanStepID:   "step-" + string(tc.reportStatus),
				HostAgentID:  "child-a",
				HostID:       "host-a",
				Status:       string(tc.reportStatus),
				EvidenceRefs: []string{"eref-" + string(tc.reportStatus)},
				Blockers:     []string{"blocker-" + string(tc.reportStatus)},
			})
			if err != nil {
				t.Fatalf("MergeChildReport error = %v", err)
			}
			if decision.Status != tc.wantStatus {
				t.Fatalf("decision = %#v, want status %s", decision, tc.wantStatus)
			}
			if decision.EvidenceRef == "" {
				t.Fatalf("decision = %#v, want merged evidence ref", decision)
			}
		})
	}
}
