package operatorruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var errInspectionFailed = errors.New("inspection failed")

type GuardSchedulerOptions struct {
	Now func() time.Time
}

type GuardScheduler struct {
	store               Store
	inspector           InspectionRunner
	now                 func() time.Time
	mu                  sync.Mutex
	activeRules         map[string]bool
	lastRunByProblemKey map[string]time.Time
	failuresByRule      map[string]int
	evaluatorByRule     map[string]*ProblemEvaluator
}

func NewGuardScheduler(store Store, inspector InspectionRunner, opts GuardSchedulerOptions) *GuardScheduler {
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	return &GuardScheduler{
		store:               store,
		inspector:           inspector,
		now:                 now,
		activeRules:         map[string]bool{},
		lastRunByProblemKey: map[string]time.Time{},
		failuresByRule:      map[string]int{},
		evaluatorByRule:     map[string]*ProblemEvaluator{},
	}
}

func (s *GuardScheduler) MarkRunning(ruleID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.activeRules[ruleID] = true
}

func (s *GuardScheduler) Tick(ctx context.Context) error {
	rules, err := s.store.ListGuardRules(ctx)
	if err != nil {
		return err
	}
	var firstErr error
	for _, rule := range rules {
		if !rule.Enabled {
			continue
		}
		if s.isRunning(rule.ID) {
			continue
		}
		if err := s.tickRule(ctx, rule); err != nil {
			s.recordFailure(ctx, rule)
			if firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func (s *GuardScheduler) tickRule(ctx context.Context, rule GuardRule) error {
	resource, ok, err := s.store.GetResource(ctx, ResourceRef(rule))
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("resource not found")
	}
	template, ok, err := s.store.GetInspectionTemplate(ctx, rule.TemplateRef)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("inspection template not found")
	}
	results, err := s.inspector.Inspect(ctx, resource, template)
	if err != nil {
		return err
	}
	for _, result := range results {
		for _, problemRef := range rule.ProblemTypeRefs {
			problem, ok, err := s.store.GetProblemType(ctx, problemRef)
			if err != nil {
				return err
			}
			if !ok {
				continue
			}
			evaluation := s.evaluatorForRule(rule.ID).Evaluate(problem, result)
			for _, match := range evaluation.Matches {
				if s.inCooldown(rule, match, result) {
					continue
				}
				run, err := s.buildGuardRun(ctx, rule, resource, problem, match, result)
				if err != nil {
					return err
				}
				if err := s.store.CreateGuardRun(ctx, run); err != nil {
					return err
				}
				s.setLastRun(rule, match, result)
			}
		}
	}
	s.clearFailure(rule.ID)
	return nil
}

func (s *GuardScheduler) buildGuardRun(ctx context.Context, rule GuardRule, resource ManagedResource, problem ProblemType, match ProblemMatch, result InspectionResult) (GuardRun, error) {
	now := s.now().UTC()
	run := GuardRun{
		ID:            fmt.Sprintf("guard-run-%d", now.UnixNano()),
		GuardRuleRef:  rule.ID,
		State:         GuardRunMatchedProblem,
		ProblemTypeID: match.ProblemTypeID,
		CreatedAt:     now,
	}
	actions, err := s.store.ListActions(ctx)
	if err != nil {
		return GuardRun{}, err
	}
	bindings, err := s.store.ListWorkflowBindings(ctx)
	if err != nil {
		return GuardRun{}, err
	}
	selection, err := SelectAction(problem, actions, bindings)
	if err != nil {
		run.State = GuardRunBlocked
		run.Events = append(run.Events, GuardRunEvent{Type: "safety.decision", Message: "blocked: " + err.Error(), CreatedAt: now})
		return run, nil
	}
	run.ActionRef = selection.Action.ID
	decision := DecideSafetyForResource(rule, selection.Action, resource, ResourceTarget(resource, result))
	run.Events = append(run.Events, GuardRunEvent{
		Type:      "safety.decision",
		Message:   string(decision.Decision) + ": " + decision.Reason,
		CreatedAt: now,
	})
	switch decision.Decision {
	case DecisionAuto:
		run.State = GuardRunActionSelected
	case DecisionRequiresApproval:
		run.State = GuardRunWaitingApproval
	default:
		run.State = GuardRunBlocked
	}
	return run, nil
}

func (s *GuardScheduler) evaluatorForRule(ruleID string) *ProblemEvaluator {
	s.mu.Lock()
	defer s.mu.Unlock()
	evaluator := s.evaluatorByRule[ruleID]
	if evaluator == nil {
		evaluator = NewProblemEvaluator(s.now)
		s.evaluatorByRule[ruleID] = evaluator
	}
	return evaluator
}

func (s *GuardScheduler) isRunning(ruleID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.activeRules[ruleID]
}

func (s *GuardScheduler) problemKey(rule GuardRule, match ProblemMatch, result InspectionResult) string {
	targetID := result.TargetID
	if targetID == "" {
		targetID = result.ReplicaID
	}
	return rule.ID + "/" + match.ProblemTypeID + "/" + targetID
}

func (s *GuardScheduler) inCooldown(rule GuardRule, match ProblemMatch, result InspectionResult) bool {
	if rule.CooldownSeconds <= 0 {
		return false
	}
	key := s.problemKey(rule, match, result)
	s.mu.Lock()
	defer s.mu.Unlock()
	last, ok := s.lastRunByProblemKey[key]
	return ok && s.now().Sub(last) < time.Duration(rule.CooldownSeconds)*time.Second
}

func (s *GuardScheduler) setLastRun(rule GuardRule, match ProblemMatch, result InspectionResult) {
	key := s.problemKey(rule, match, result)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastRunByProblemKey[key] = s.now()
}

func (s *GuardScheduler) recordFailure(ctx context.Context, rule GuardRule) {
	s.mu.Lock()
	s.failuresByRule[rule.ID]++
	failures := s.failuresByRule[rule.ID]
	s.mu.Unlock()
	if rule.DisableAfterConsecutiveFailures > 0 && failures >= rule.DisableAfterConsecutiveFailures {
		_, _ = s.store.SetGuardRuleEnabled(ctx, rule.ID, false)
	}
}

func (s *GuardScheduler) clearFailure(ruleID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.failuresByRule, ruleID)
}
