package evidence

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// Service validates evidence operations and assigns stable refs.
type Service struct {
	store Store
	now   func() time.Time
}

func NewService(store Store, now func() time.Time) *Service {
	if now == nil {
		now = time.Now
	}
	return &Service{store: store, now: now}
}

func (s *Service) Record(ctx context.Context, req RecordRequest) (Record, error) {
	if s == nil || s.store == nil {
		return Record{}, fmt.Errorf("evidence: service store is required")
	}
	req.Summary = strings.TrimSpace(req.Summary)
	if req.Summary == "" {
		return Record{}, fmt.Errorf("evidence: summary is required")
	}
	if req.Kind == "" {
		req.Kind = KindOther
	}
	created := s.now().UTC()
	rec := Record{
		IncidentID:  strings.TrimSpace(req.IncidentID),
		SourceTool:  strings.TrimSpace(req.SourceTool),
		Source:      strings.TrimSpace(req.Source),
		Kind:        req.Kind,
		Service:     strings.TrimSpace(req.Service),
		Environment: strings.TrimSpace(req.Environment),
		TimeRange:   strings.TrimSpace(req.TimeRange),
		Summary:     req.Summary,
		Data:        cloneMap(req.Data),
		SessionID:   strings.TrimSpace(req.SessionID),
		TurnID:      strings.TrimSpace(req.TurnID),
		ToolCallID:  strings.TrimSpace(req.ToolCallID),
		CreatedAt:   created,
	}
	rec.Ref = evidenceRef(rec)
	if err := s.store.Put(ctx, rec); err != nil {
		return Record{}, err
	}
	return cloneRecord(rec), nil
}

func (s *Service) Get(ctx context.Context, ref string) (Record, bool) {
	if s == nil || s.store == nil {
		return Record{}, false
	}
	rec, ok, err := s.store.Get(ctx, strings.TrimSpace(ref))
	if err != nil {
		return Record{}, false
	}
	return rec, ok
}

func (s *Service) LinkIncident(ctx context.Context, incidentID string, refs []string, relation Relation) error {
	if s == nil || s.store == nil {
		return fmt.Errorf("evidence: service store is required")
	}
	incidentID = strings.TrimSpace(incidentID)
	if incidentID == "" {
		return fmt.Errorf("evidence: incident id is required")
	}
	if relation == "" {
		relation = RelationContext
	}
	now := s.now().UTC()
	for _, ref := range refs {
		ref = strings.TrimSpace(ref)
		if ref == "" {
			continue
		}
		if _, ok := s.Get(ctx, ref); !ok {
			return fmt.Errorf("evidence: ref %q not found", ref)
		}
		if err := s.store.LinkIncident(ctx, IncidentLink{IncidentID: incidentID, Ref: ref, Relation: relation, CreatedAt: now}); err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) ListIncident(ctx context.Context, incidentID string) []Record {
	if s == nil || s.store == nil {
		return nil
	}
	records, err := s.store.ListIncident(ctx, strings.TrimSpace(incidentID))
	if err != nil {
		return nil
	}
	return records
}

func evidenceRef(rec Record) string {
	payload := struct {
		IncidentID string         `json:"incidentId,omitempty"`
		SourceTool string         `json:"sourceTool,omitempty"`
		Source     string         `json:"source,omitempty"`
		Kind       Kind           `json:"kind,omitempty"`
		Summary    string         `json:"summary"`
		Data       map[string]any `json:"data,omitempty"`
		CreatedAt  time.Time      `json:"createdAt"`
	}{
		IncidentID: rec.IncidentID,
		SourceTool: rec.SourceTool,
		Source:     rec.Source,
		Kind:       rec.Kind,
		Summary:    rec.Summary,
		Data:       rec.Data,
		CreatedAt:  rec.CreatedAt,
	}
	data, _ := json.Marshal(payload)
	sum := sha256.Sum256(data)
	return "ev-" + hex.EncodeToString(sum[:])[:16]
}
