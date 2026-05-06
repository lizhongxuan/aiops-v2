package fallback

import (
	"strings"
	"sync"

	"aiops-v2/internal/actionproposal"
)

type InMemoryStore struct {
	mu           sync.RWMutex
	plans        map[string]FallbackPlan
	observations map[string][]ActionObservation
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{plans: map[string]FallbackPlan{}, observations: map[string][]ActionObservation{}}
}

func (s *InMemoryStore) Put(plan FallbackPlan) {
	if s == nil || strings.TrimSpace(plan.ID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.plans[plan.ID] = clonePlan(plan)
}

func (s *InMemoryStore) Get(id string) (FallbackPlan, bool) {
	if s == nil {
		return FallbackPlan{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	plan, ok := s.plans[strings.TrimSpace(id)]
	if !ok {
		return FallbackPlan{}, false
	}
	return clonePlan(plan), true
}

func (s *InMemoryStore) AddObservation(planID string, observation ActionObservation) {
	if s == nil || strings.TrimSpace(planID) == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.observations[planID] = append(s.observations[planID], observation)
}

func clonePlan(plan FallbackPlan) FallbackPlan {
	plan.EvidenceRefs = append([]string(nil), plan.EvidenceRefs...)
	plan.Actions = append([]actionproposal.ActionProposal(nil), plan.Actions...)
	for i := range plan.Actions {
		plan.Actions[i].ToolInput = append([]byte(nil), plan.Actions[i].ToolInput...)
		plan.Actions[i].EvidenceRefs = append([]string(nil), plan.Actions[i].EvidenceRefs...)
		plan.Actions[i].Verification = append([]actionproposal.VerificationStep(nil), plan.Actions[i].Verification...)
	}
	return plan
}
