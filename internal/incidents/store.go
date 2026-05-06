package incidents

import "sync"

type Store interface {
	PutIncident(incident IncidentCase)
	GetIncident(id string) (IncidentCase, bool)
	ListIncidents() []IncidentCase
	PutEvidence(incidentID string, evidence EvidenceRef)
	ListEvidence(incidentID string) []EvidenceRef
}

type InMemoryStore struct {
	mu        sync.RWMutex
	incidents map[string]IncidentCase
	evidence  map[string][]EvidenceRef
}

func NewInMemoryStore() *InMemoryStore {
	return &InMemoryStore{
		incidents: map[string]IncidentCase{},
		evidence:  map[string][]EvidenceRef{},
	}
}

func (s *InMemoryStore) PutIncident(incident IncidentCase) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.incidents[incident.ID] = cloneIncident(incident)
}

func (s *InMemoryStore) GetIncident(id string) (IncidentCase, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	incident, ok := s.incidents[id]
	return cloneIncident(incident), ok
}

func (s *InMemoryStore) ListIncidents() []IncidentCase {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]IncidentCase, 0, len(s.incidents))
	for _, incident := range s.incidents {
		out = append(out, cloneIncident(incident))
	}
	return out
}

func (s *InMemoryStore) PutEvidence(incidentID string, evidence EvidenceRef) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.evidence[incidentID] = append(s.evidence[incidentID], evidence)
}

func (s *InMemoryStore) ListEvidence(incidentID string) []EvidenceRef {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return append([]EvidenceRef(nil), s.evidence[incidentID]...)
}

func cloneIncident(in IncidentCase) IncidentCase {
	in.AffectedServices = append([]string(nil), in.AffectedServices...)
	in.EvidenceRefs = append([]string(nil), in.EvidenceRefs...)
	in.Actions = append([]ActionRecord(nil), in.Actions...)
	in.Approvals = append([]ApprovalRecord(nil), in.Approvals...)
	in.Hypotheses = append([]Hypothesis(nil), in.Hypotheses...)
	if in.Postmortem != nil {
		pm := *in.Postmortem
		pm.Timeline = append([]TimelineItem(nil), pm.Timeline...)
		pm.FollowUps = append([]string(nil), pm.FollowUps...)
		in.Postmortem = &pm
	}
	return in
}
