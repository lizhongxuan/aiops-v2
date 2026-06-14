package workflowgen

import (
	"context"
	"strings"
	"testing"
)

func TestMemorySessionStoreAppendsEventsWithoutDroppingConcurrentUpdates(t *testing.T) {
	store := NewMemorySessionStore()
	session, err := store.Create(context.Background(), WorkflowGenerationSession{
		ConversationID:      "thread-1",
		UserID:              "user-1",
		Requirement:         "@add_workflow 每天早上8点抓取AI新闻",
		Status:              SessionStatusPlanReady,
		ValidationProvider:  ValidationProviderDocker,
		PlanVersion:         1,
		CreatedByUserPrompt: true,
	})
	if err != nil {
		t.Fatalf("Create() error = %v", err)
	}
	if session.ID == "" {
		t.Fatal("Create() did not assign session ID")
	}

	const workers = 20
	done := make(chan error, workers)
	for i := 0; i < workers; i++ {
		go func(index int) {
			_, appendErr := store.AppendEvent(context.Background(), session.ID, WorkflowGenerationEvent{
				Type: EventNodeGenerated,
				NodeID: "node",
				Message: "generated",
			})
			done <- appendErr
		}(i)
	}
	for i := 0; i < workers; i++ {
		if err := <-done; err != nil {
			t.Fatalf("AppendEvent() error = %v", err)
		}
	}

	events, err := store.ListEvents(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("ListEvents() error = %v", err)
	}
	if len(events) != workers {
		t.Fatalf("events len = %d, want %d", len(events), workers)
	}
	for i, event := range events {
		if event.Sequence != int64(i+1) {
			t.Fatalf("event[%d].Sequence = %d, want %d", i, event.Sequence, i+1)
		}
		if event.ID == "" || event.CreatedAt.IsZero() {
			t.Fatalf("event[%d] missing ID or CreatedAt: %#v", i, event)
		}
	}
}

func TestDeterministicPlanBuilderCreatesPlanFirstAndRequiredSlots(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天早上8点自动抓取AI行业新闻，提取三条关键内容发送给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	if !strings.Contains(plan.Title, "AI") {
		t.Fatalf("plan title = %q, want AI related", plan.Title)
	}
	if plan.Trigger.Type != TriggerTypeSchedule || plan.Trigger.Schedule == "" {
		t.Fatalf("trigger = %#v, want scheduled trigger", plan.Trigger)
	}
	if len(plan.Nodes) < 3 {
		t.Fatalf("nodes len = %d, want at least 3", len(plan.Nodes))
	}
	if !plan.ValidationStrategy.Enabled || plan.ValidationStrategy.Provider != ValidationProviderDocker {
		t.Fatalf("validation strategy = %#v, want docker enabled", plan.ValidationStrategy)
	}
	if len(plan.RequiredSlots) == 0 {
		t.Fatal("RequiredSlots empty, want delivery method or secret slot")
	}
	if plan.RequiredSlots[0].ID == "" || plan.RequiredSlots[0].Question == "" {
		t.Fatalf("RequiredSlots[0] incomplete: %#v", plan.RequiredSlots[0])
	}
}

func TestPlanBuilderRevisionUpdatesDeliveryWithoutLosingSchedule(t *testing.T) {
	builder := DeterministicPlanBuilder{}
	plan, err := builder.BuildPlan(context.Background(), BuildPlanRequest{
		Requirement: "每天早上8点自动抓取AI行业新闻，提取三条关键内容发送给我",
	})
	if err != nil {
		t.Fatalf("BuildPlan() error = %v", err)
	}
	revised, err := builder.RevisePlan(context.Background(), RevisePlanRequest{
		Previous: *plan,
		Message:  "不要飞书和邮件，直接返回结果",
	})
	if err != nil {
		t.Fatalf("RevisePlan() error = %v", err)
	}
	if revised.Version != plan.Version+1 {
		t.Fatalf("revised version = %d, want %d", revised.Version, plan.Version+1)
	}
	if revised.Trigger.Schedule != plan.Trigger.Schedule {
		t.Fatalf("schedule = %q, want preserved %q", revised.Trigger.Schedule, plan.Trigger.Schedule)
	}
	if got := revised.Outputs[0].Target; got != OutputTargetReturn {
		t.Fatalf("output target = %q, want %q", got, OutputTargetReturn)
	}
	for _, slot := range revised.RequiredSlots {
		if strings.Contains(strings.ToLower(slot.ID), "webhook") {
			t.Fatalf("webhook slot should be removed for return-only plan: %#v", slot)
		}
	}
}

