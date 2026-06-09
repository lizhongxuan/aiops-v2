package planning

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

const DefaultTaskLeaseTTL = 15 * time.Minute

type TaskClaim struct {
	TaskID    string `json:"taskId"`
	Owner     string `json:"owner"`
	AgentID   string `json:"agentId,omitempty"`
	LeaseID   string `json:"leaseId"`
	ExpiresAt string `json:"expiresAt"`
}

type TaskClaimOutcome struct {
	Claim              TaskClaim `json:"claim,omitempty"`
	Claimed            bool      `json:"claimed"`
	Reason             string    `json:"reason,omitempty"`
	DependsOnSatisfied []string  `json:"dependsOnSatisfied,omitempty"`
	BlockedCount       int       `json:"blockedCount"`
}

type TaskStore struct {
	mu     sync.Mutex
	state  PlanState
	claims map[string]TaskClaim
}

func NewTaskStore(state PlanState) (*TaskStore, error) {
	steps, err := normalizePlanSteps(state.Steps, state.Status.IsFinal())
	if err != nil {
		return nil, err
	}
	if err := validatePlanDependencies(steps); err != nil {
		return nil, err
	}
	copyState := state
	copyState.Steps = append([]PlanStep(nil), steps...)
	return &TaskStore{
		state:  copyState,
		claims: map[string]TaskClaim{},
	}, nil
}

func (s *TaskStore) ClaimNext(owner string, agentID string, now time.Time) (TaskClaim, bool) {
	outcome := s.ClaimNextDetailed(owner, agentID, now)
	return outcome.Claim, outcome.Claimed
}

func (s *TaskStore) ClaimNextDetailed(owner string, agentID string, now time.Time) TaskClaimOutcome {
	s.mu.Lock()
	defer s.mu.Unlock()

	owner = strings.TrimSpace(owner)
	agentID = strings.TrimSpace(agentID)
	if owner == "" {
		return TaskClaimOutcome{Reason: "owner_required", BlockedCount: s.blockedCountLocked(now)}
	}
	s.expireClaimsLocked(now)
	if s.ownerHasActiveClaimLocked(owner, now) {
		return TaskClaimOutcome{Reason: "owner_has_active_task", BlockedCount: s.blockedCountLocked(now)}
	}
	for i, step := range s.state.Steps {
		if step.Status != StepStatusPending {
			continue
		}
		if _, claimed := s.claims[step.ID]; claimed && !s.claimExpiredLocked(step.ID, now) {
			continue
		}
		satisfied, ok := s.dependenciesSatisfiedLocked(step)
		if !ok {
			continue
		}
		claim := TaskClaim{
			TaskID:    step.ID,
			Owner:     owner,
			AgentID:   agentID,
			LeaseID:   newTaskLeaseID(step.ID, owner, now),
			ExpiresAt: now.Add(DefaultTaskLeaseTTL).UTC().Format(time.RFC3339),
		}
		s.state.Steps[i].Status = StepStatusInProgress
		s.state.Steps[i].Owner = owner
		s.state.Steps[i].AgentID = agentID
		s.claims[step.ID] = claim
		return TaskClaimOutcome{
			Claim:              claim,
			Claimed:            true,
			DependsOnSatisfied: satisfied,
			BlockedCount:       s.blockedCountLocked(now),
		}
	}
	return TaskClaimOutcome{Reason: "no_eligible_task", BlockedCount: s.blockedCountLocked(now)}
}

func (s *TaskStore) Complete(claim TaskClaim, evidenceRefs []string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateClaimLocked(claim, now); err != nil {
		return err
	}
	index := s.stepIndexLocked(claim.TaskID)
	if index < 0 {
		return fmt.Errorf("task %q not found", claim.TaskID)
	}
	s.state.Steps[index].Status = StepStatusCompleted
	s.state.Steps[index].EvidenceRefs = mergeStringSlices(s.state.Steps[index].EvidenceRefs, evidenceRefs)
	delete(s.claims, claim.TaskID)
	return nil
}

func (s *TaskStore) Block(claim TaskClaim, blockedBy []string, summary string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateClaimLocked(claim, now); err != nil {
		return err
	}
	index := s.stepIndexLocked(claim.TaskID)
	if index < 0 {
		return fmt.Errorf("task %q not found", claim.TaskID)
	}
	s.state.Steps[index].Status = StepStatusBlocked
	s.state.Steps[index].BlockedBy = trimStringSlice(blockedBy)
	s.state.Steps[index].Summary = strings.TrimSpace(summary)
	delete(s.claims, claim.TaskID)
	return nil
}

func (s *TaskStore) Release(claim TaskClaim, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.validateClaimLocked(claim, now); err != nil {
		return err
	}
	index := s.stepIndexLocked(claim.TaskID)
	if index < 0 {
		return fmt.Errorf("task %q not found", claim.TaskID)
	}
	s.state.Steps[index].Status = StepStatusPending
	s.state.Steps[index].Owner = ""
	s.state.Steps[index].AgentID = ""
	delete(s.claims, claim.TaskID)
	return nil
}

func (s *TaskStore) State() PlanState {
	s.mu.Lock()
	defer s.mu.Unlock()
	copyState := s.state
	copyState.Steps = append([]PlanStep(nil), s.state.Steps...)
	return copyState
}

func (s *TaskStore) validateClaimLocked(claim TaskClaim, now time.Time) error {
	current, ok := s.claims[claim.TaskID]
	if !ok {
		return fmt.Errorf("claim for task %q not found", claim.TaskID)
	}
	if current.LeaseID != claim.LeaseID || current.Owner != claim.Owner {
		return fmt.Errorf("claim for task %q does not match active lease", claim.TaskID)
	}
	expiresAt, err := time.Parse(time.RFC3339, current.ExpiresAt)
	if err != nil {
		return fmt.Errorf("claim for task %q has invalid expiry: %w", claim.TaskID, err)
	}
	if !now.Before(expiresAt) {
		delete(s.claims, claim.TaskID)
		return fmt.Errorf("claim for task %q expired", claim.TaskID)
	}
	return nil
}

func (s *TaskStore) expireClaimsLocked(now time.Time) {
	for taskID, claim := range s.claims {
		expiresAt, err := time.Parse(time.RFC3339, claim.ExpiresAt)
		if err != nil || !now.Before(expiresAt) {
			delete(s.claims, taskID)
			index := s.stepIndexLocked(taskID)
			if index >= 0 && s.state.Steps[index].Status == StepStatusInProgress {
				s.state.Steps[index].Status = StepStatusPending
			}
		}
	}
}

func (s *TaskStore) ownerHasActiveClaimLocked(owner string, now time.Time) bool {
	for _, claim := range s.claims {
		if claim.Owner != owner {
			continue
		}
		expiresAt, err := time.Parse(time.RFC3339, claim.ExpiresAt)
		if err == nil && now.Before(expiresAt) {
			return true
		}
	}
	return false
}

func (s *TaskStore) claimExpiredLocked(taskID string, now time.Time) bool {
	claim, ok := s.claims[taskID]
	if !ok {
		return true
	}
	expiresAt, err := time.Parse(time.RFC3339, claim.ExpiresAt)
	return err != nil || !now.Before(expiresAt)
}

func (s *TaskStore) dependenciesSatisfiedLocked(step PlanStep) ([]string, bool) {
	if len(step.DependsOn) == 0 {
		return nil, true
	}
	statusByID := map[string]StepStatus{}
	for _, candidate := range s.state.Steps {
		if candidate.ID != "" {
			statusByID[candidate.ID] = candidate.Status
		}
	}
	var satisfied []string
	for _, dep := range step.DependsOn {
		if statusByID[dep] != StepStatusCompleted {
			return nil, false
		}
		satisfied = append(satisfied, dep)
	}
	return satisfied, true
}

func (s *TaskStore) blockedCountLocked(_ time.Time) int {
	count := 0
	for _, step := range s.state.Steps {
		if step.Status == StepStatusBlocked {
			count++
		}
	}
	return count
}

func (s *TaskStore) stepIndexLocked(taskID string) int {
	for i, step := range s.state.Steps {
		if step.ID == taskID {
			return i
		}
	}
	return -1
}

func newTaskLeaseID(taskID, owner string, now time.Time) string {
	seed := strings.NewReplacer(":", "-", "/", "-", " ", "-").Replace(strings.TrimSpace(taskID + "-" + owner))
	if seed == "" {
		seed = "task"
	}
	return fmt.Sprintf("lease-%s-%d", seed, now.UTC().UnixNano())
}

func mergeStringSlices(left, right []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range append(trimStringSlice(left), trimStringSlice(right)...) {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
