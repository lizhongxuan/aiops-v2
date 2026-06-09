package runtimekernel

import (
	"strings"
	"testing"
)

func TestPlanModeStateValidate(t *testing.T) {
	if err := (PlanModeState{State: PlanModeStateInactive}).Validate(); err != nil {
		t.Fatalf("inactive Validate() = %v", err)
	}
	if err := (PlanModeState{State: PlanModeStateActive}).Validate(); err == nil {
		t.Fatal("active without planId or draft allowance should fail")
	}
	if err := (PlanModeState{State: PlanModeStateActive, AllowDraftPlan: true}).Validate(); err != nil {
		t.Fatalf("active draft Validate() = %v", err)
	}
	if err := (PlanModeState{State: PlanModeStatePendingExitApproval}).Validate(); err == nil {
		t.Fatal("pending_exit_approval without approvalId should fail")
	}
	if err := (PlanModeState{State: PlanModeStateInactive, ReminderLevel: "loud"}).Validate(); err == nil {
		t.Fatal("invalid reminder should fail")
	}
	longQuestion := strings.Repeat("问", 501)
	if err := (PlanModeState{State: PlanModeStateInactive, PendingQuestions: []string{longQuestion}}).Validate(); err == nil {
		t.Fatal("question over 500 runes should fail")
	}
}
