package actionproposal

import (
	"strings"
	"sync"
)

type InMemoryStore struct {
	mu        sync.RWMutex
	proposals map[string]ActionProposal
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{proposals: map[string]ActionProposal{}}
}

func (s *InMemoryStore) Put(proposal ActionProposal) {
	if s == nil {
		return
	}
	token := strings.TrimSpace(proposal.ActionToken)
	if token == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.proposals[token] = cloneProposal(proposal)
}

func (s *InMemoryStore) Get(token string) (ActionProposal, bool) {
	if s == nil {
		return ActionProposal{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	proposal, ok := s.proposals[strings.TrimSpace(token)]
	if !ok {
		return ActionProposal{}, false
	}
	return cloneProposal(proposal), true
}

func (s *InMemoryStore) Delete(token string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.proposals, strings.TrimSpace(token))
}

func cloneProposal(proposal ActionProposal) ActionProposal {
	proposal.ToolInput = append([]byte(nil), proposal.ToolInput...)
	proposal.EvidenceRefs = append([]string(nil), proposal.EvidenceRefs...)
	proposal.Verification = append([]VerificationStep(nil), proposal.Verification...)
	for i := range proposal.Verification {
		proposal.Verification[i].Input = append([]byte(nil), proposal.Verification[i].Input...)
	}
	return proposal
}
