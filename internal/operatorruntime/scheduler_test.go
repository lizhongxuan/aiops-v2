package operatorruntime

import (
	"context"
	"testing"
	"time"
)

type fakeInspectionRunner struct {
	results []InspectionResult
	err     error
}

func (f fakeInspectionRunner) Inspect(context.Context, PGCluster, InspectionTemplate) ([]InspectionResult, error) {
	return f.results, f.err
}

func seedSchedulerStore(t *testing.T, rule GuardRule, problem ProblemType) *MemoryStore {
	t.Helper()
	ctx := context.Background()
	store := NewMemoryStore()
	for _, save := range []func() error{
		func() error { return store.SavePGCluster(ctx, validPGCluster()) },
		func() error { return store.SaveInspectionTemplate(ctx, validInspectionTemplate()) },
		func() error { return store.SaveProblemType(ctx, problem) },
		func() error { return store.SaveProblemType(ctx, validReceiverStoppedProblem()) },
		func() error { return store.SaveAction(ctx, validAction()) },
		func() error { return store.SaveWorkflowBinding(ctx, validWorkflowBinding()) },
		func() error { return store.SaveGuardRule(ctx, rule) },
	} {
		if err := save(); err != nil {
			t.Fatalf("seed store: %v", err)
		}
	}
	return store
}

func schedulerLagProblem() ProblemType {
	problem := validLagProblem()
	problem.ForSeconds = 60
	return problem
}

func schedulerNow() (func() time.Time, func(time.Duration)) {
	now := time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	return func() time.Time { return now }, func(delta time.Duration) { now = now.Add(delta) }
}

func TestSchedulerSkipsDisabledGuardRule(t *testing.T) {
	rule := validGuardRule()
	rule.Enabled = false
	store := seedSchedulerStore(t, rule, schedulerLagProblem())
	scheduler := NewGuardScheduler(store, fakeInspectionRunner{results: []InspectionResult{lagInspectionResult(KnownNumber(120))}}, GuardSchedulerOptions{})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	runs, err := store.ListGuardRuns(context.Background())
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 0 {
		t.Fatalf("disabled rule should not create runs: %#v", runs)
	}
}

func TestSchedulerCreatesGuardRunWhenProblemMatched(t *testing.T) {
	now, advance := schedulerNow()
	store := seedSchedulerStore(t, validGuardRule(), schedulerLagProblem())
	scheduler := NewGuardScheduler(store, fakeInspectionRunner{results: []InspectionResult{lagInspectionResult(KnownNumber(120))}}, GuardSchedulerOptions{Now: now})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	advance(61 * time.Second)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	runs, err := store.ListGuardRuns(context.Background())
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 || runs[0].ProblemTypeID != "pg.replication.lag_high" {
		t.Fatalf("expected one lag guard run, got %#v", runs)
	}
}

func TestSchedulerRecordsSafetyDecisionEvent(t *testing.T) {
	now, advance := schedulerNow()
	store := seedSchedulerStore(t, validGuardRule(), schedulerLagProblem())
	scheduler := NewGuardScheduler(store, fakeInspectionRunner{results: []InspectionResult{lagInspectionResult(KnownNumber(120))}}, GuardSchedulerOptions{Now: now})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	advance(61 * time.Second)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	runs, err := store.ListGuardRuns(context.Background())
	if err != nil {
		t.Fatalf("list runs: %v", err)
	}
	if len(runs) != 1 {
		t.Fatalf("expected one guard run, got %#v", runs)
	}
	if runs[0].State != GuardRunWaitingApproval {
		t.Fatalf("run state = %s, want %s", runs[0].State, GuardRunWaitingApproval)
	}
	if runs[0].ActionRef != validAction().ID {
		t.Fatalf("run action ref = %q, want %q", runs[0].ActionRef, validAction().ID)
	}
	if len(runs[0].Events) != 1 || runs[0].Events[0].Type != "safety.decision" {
		t.Fatalf("expected safety decision event, got %#v", runs[0].Events)
	}
}

func TestSchedulerHonorsCooldown(t *testing.T) {
	rule := validGuardRule()
	rule.CooldownSeconds = 1800
	now, advance := schedulerNow()
	store := seedSchedulerStore(t, rule, schedulerLagProblem())
	scheduler := NewGuardScheduler(store, fakeInspectionRunner{results: []InspectionResult{lagInspectionResult(KnownNumber(120))}}, GuardSchedulerOptions{Now: now})
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("first tick: %v", err)
	}
	advance(61 * time.Second)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	advance(61 * time.Second)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("third tick: %v", err)
	}
	runs, _ := store.ListGuardRuns(context.Background())
	if len(runs) != 1 {
		t.Fatalf("cooldown should suppress duplicate run, got %#v", runs)
	}
}

func TestSchedulerPreventsConcurrentRuns(t *testing.T) {
	rule := validGuardRule()
	now, advance := schedulerNow()
	store := seedSchedulerStore(t, rule, schedulerLagProblem())
	scheduler := NewGuardScheduler(store, fakeInspectionRunner{results: []InspectionResult{lagInspectionResult(KnownNumber(120))}}, GuardSchedulerOptions{Now: now})
	scheduler.MarkRunning(rule.ID)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("tick: %v", err)
	}
	advance(61 * time.Second)
	if err := scheduler.Tick(context.Background()); err != nil {
		t.Fatalf("second tick: %v", err)
	}
	runs, _ := store.ListGuardRuns(context.Background())
	if len(runs) != 0 {
		t.Fatalf("active run should prevent concurrent run: %#v", runs)
	}
}

func TestSchedulerDisablesRuleAfterConsecutiveFailures(t *testing.T) {
	rule := validGuardRule()
	rule.DisableAfterConsecutiveFailures = 2
	store := seedSchedulerStore(t, rule, schedulerLagProblem())
	scheduler := NewGuardScheduler(store, fakeInspectionRunner{err: errInspectionFailed}, GuardSchedulerOptions{Now: func() time.Time {
		return time.Date(2026, 6, 22, 10, 0, 0, 0, time.UTC)
	}})
	if err := scheduler.Tick(context.Background()); err == nil {
		t.Fatalf("first inspection failure should be returned")
	}
	if err := scheduler.Tick(context.Background()); err == nil {
		t.Fatalf("second inspection failure should be returned")
	}
	got, ok, err := store.GetGuardRule(context.Background(), rule.ID)
	if err != nil || !ok {
		t.Fatalf("get rule: %v", err)
	}
	if got.Enabled {
		t.Fatalf("rule should be disabled after consecutive failures")
	}
}
