package incidents

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type Service struct {
	store Store
	now   func() time.Time
	pm    *PostmortemService
}

type CreateRequest struct {
	ExternalID           string   `json:"externalId,omitempty"`
	Title                string   `json:"title"`
	Severity             string   `json:"severity,omitempty"`
	Source               string   `json:"source,omitempty"`
	Environment          string   `json:"environment,omitempty"`
	BusinessCapability   string   `json:"businessCapability,omitempty"`
	BusinessCapabilityID string   `json:"businessCapabilityId,omitempty"`
	AffectedServices     []string `json:"affectedServices,omitempty"`
}

type UpdateRequest struct {
	Title              string   `json:"title,omitempty"`
	Severity           string   `json:"severity,omitempty"`
	Status             string   `json:"status,omitempty"`
	AffectedServices   []string `json:"affectedServices,omitempty"`
	BusinessCapability string   `json:"businessCapability,omitempty"`
}

type CloseRequest struct {
	RootCause  string   `json:"rootCause,omitempty"`
	Mitigation string   `json:"mitigation,omitempty"`
	FollowUps  []string `json:"followUps,omitempty"`
}

func NewService(store Store, now func() time.Time) *Service {
	if store == nil {
		store = NewInMemoryStore()
	}
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now, pm: NewPostmortemService()}
}

func (s *Service) Create(req CreateRequest) (IncidentCase, error) {
	if strings.TrimSpace(req.Title) == "" {
		return IncidentCase{}, fmt.Errorf("title is required")
	}
	now := s.now()
	incident := IncidentCase{
		ID:                   fmt.Sprintf("inc_%d", now.UnixNano()),
		ExternalID:           strings.TrimSpace(req.ExternalID),
		Title:                strings.TrimSpace(req.Title),
		Severity:             strings.TrimSpace(req.Severity),
		Status:               IncidentStatusOpen,
		Source:               strings.TrimSpace(req.Source),
		Environment:          strings.TrimSpace(req.Environment),
		BusinessCapability:   strings.TrimSpace(req.BusinessCapability),
		BusinessCapabilityID: strings.TrimSpace(req.BusinessCapabilityID),
		AffectedServices:     uniqueStrings(req.AffectedServices),
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	s.store.PutIncident(incident)
	return incident, nil
}

func (s *Service) Get(id string) (IncidentCase, bool) {
	return s.store.GetIncident(strings.TrimSpace(id))
}

func (s *Service) List() []IncidentCase {
	items := s.store.ListIncidents()
	sort.Slice(items, func(i, j int) bool { return items[i].UpdatedAt.After(items[j].UpdatedAt) })
	return items
}

func (s *Service) Update(id string, req UpdateRequest) (IncidentCase, error) {
	incident, ok := s.store.GetIncident(strings.TrimSpace(id))
	if !ok {
		return IncidentCase{}, fmt.Errorf("incident %q not found", id)
	}
	if strings.TrimSpace(req.Title) != "" {
		incident.Title = strings.TrimSpace(req.Title)
	}
	if strings.TrimSpace(req.Severity) != "" {
		incident.Severity = strings.TrimSpace(req.Severity)
	}
	if strings.TrimSpace(req.Status) != "" {
		incident.Status = IncidentStatus(strings.TrimSpace(req.Status))
	}
	if req.AffectedServices != nil {
		incident.AffectedServices = uniqueStrings(req.AffectedServices)
	}
	if strings.TrimSpace(req.BusinessCapability) != "" {
		incident.BusinessCapability = strings.TrimSpace(req.BusinessCapability)
	}
	incident.UpdatedAt = s.now()
	s.store.PutIncident(incident)
	return incident, nil
}

func (s *Service) AddEvidence(incidentID string, evidence EvidenceRef) (EvidenceRef, error) {
	incident, ok := s.store.GetIncident(strings.TrimSpace(incidentID))
	if !ok {
		return EvidenceRef{}, fmt.Errorf("incident %q not found", incidentID)
	}
	now := s.now()
	if strings.TrimSpace(evidence.ID) == "" {
		evidence.ID = fmt.Sprintf("ev_%d", now.UnixNano())
	}
	if evidence.CreatedAt.IsZero() {
		evidence.CreatedAt = now
	}
	evidence.Source = strings.TrimSpace(evidence.Source)
	evidence.RawRef = strings.TrimSpace(evidence.RawRef)
	evidence.Summary = strings.TrimSpace(evidence.Summary)
	evidence.Confidence = strings.TrimSpace(evidence.Confidence)
	evidence.EntityID = strings.TrimSpace(evidence.EntityID)
	s.store.PutEvidence(incident.ID, evidence)
	incident.EvidenceRefs = appendUnique(incident.EvidenceRefs, evidence.ID)
	incident.UpdatedAt = now
	s.store.PutIncident(incident)
	return evidence, nil
}

func (s *Service) Evidence(incidentID string) []EvidenceRef {
	return s.store.ListEvidence(strings.TrimSpace(incidentID))
}

func (s *Service) RankHypotheses(incidentID string, hypotheses []Hypothesis) ([]Hypothesis, error) {
	incident, ok := s.store.GetIncident(strings.TrimSpace(incidentID))
	if !ok {
		return nil, fmt.Errorf("incident %q not found", incidentID)
	}
	out := append([]Hypothesis(nil), hypotheses...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Confidence > out[j].Confidence })
	for i := range out {
		out[i].Rank = i + 1
		out[i].Hypothesis = strings.TrimSpace(out[i].Hypothesis)
		out[i].SupportingEvidence = uniqueStrings(out[i].SupportingEvidence)
	}
	incident.Hypotheses = out
	incident.UpdatedAt = s.now()
	s.store.PutIncident(incident)
	return append([]Hypothesis(nil), out...), nil
}

func (s *Service) Close(id string, req CloseRequest) (IncidentCase, error) {
	incident, ok := s.store.GetIncident(strings.TrimSpace(id))
	if !ok {
		return IncidentCase{}, fmt.Errorf("incident %q not found", id)
	}
	now := s.now()
	pm := s.pm.Draft(incident, s.store.ListEvidence(incident.ID), req, now)
	incident.Status = IncidentStatusClosed
	incident.Postmortem = &pm
	incident.UpdatedAt = now
	incident.ClosedAt = &now
	s.store.PutIncident(incident)
	return incident, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func appendUnique(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, existing := range values {
		if existing == value {
			return values
		}
	}
	return append(values, value)
}
