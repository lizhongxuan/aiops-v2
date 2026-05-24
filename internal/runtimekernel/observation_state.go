package runtimekernel

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

// ObservationKey identifies a stable observation stream, such as one tool,
// target, time window, and source version.
type ObservationKey struct {
	ToolName      string `json:"toolName,omitempty"`
	Target        string `json:"target,omitempty"`
	Window        string `json:"window,omitempty"`
	SourceVersion string `json:"sourceVersion,omitempty"`
	Query         string `json:"query,omitempty"`
}

// String returns a deterministic key suitable for map lookup and persistence.
func (k ObservationKey) String() string {
	parts := []string{
		strings.TrimSpace(k.ToolName),
		"target=" + strings.TrimSpace(k.Target),
		"window=" + strings.TrimSpace(k.Window),
		"sourceVersion=" + strings.TrimSpace(k.SourceVersion),
		"query=" + strings.TrimSpace(k.Query),
	}
	return strings.Join(parts, "|")
}

// ObservationState keeps the latest digest for repeated observations.
type ObservationState struct {
	Records []ObservationRecord `json:"records,omitempty"`
}

// ObservationRecord is the persisted digest and model-visible summary for one
// observation stream.
type ObservationRecord struct {
	Key            string    `json:"key"`
	Digest         string    `json:"digest"`
	SourceRef      string    `json:"sourceRef,omitempty"`
	Summary        string    `json:"summary,omitempty"`
	ToolName       string    `json:"toolName,omitempty"`
	Target         string    `json:"target,omitempty"`
	Window         string    `json:"window,omitempty"`
	SourceVersion  string    `json:"sourceVersion,omitempty"`
	LastObservedAt time.Time `json:"lastObservedAt,omitempty"`
}

// ObservationCheckResult reports whether a new observation can be represented
// to the model as an unchanged stub.
type ObservationCheckResult struct {
	Hit                 bool              `json:"hit"`
	Changed             bool              `json:"changed"`
	ModelVisibleContent string            `json:"modelVisibleContent,omitempty"`
	Record              ObservationRecord `json:"record"`
	Previous            ObservationRecord `json:"previous,omitempty"`
	Event               ContextGovernanceEvent
}

func NewObservationState() ObservationState {
	return ObservationState{}
}

// Upsert records the latest digest for a key.
func (s *ObservationState) Upsert(record ObservationRecord) {
	record = normalizeObservationRecord(record)
	for i := range s.Records {
		if s.Records[i].Key == record.Key {
			s.Records[i] = record
			return
		}
	}
	s.Records = append(s.Records, record)
}

// Check compares an observation against the latest known record.
func (s *ObservationState) Check(record ObservationRecord) ObservationCheckResult {
	record = normalizeObservationRecord(record)
	for _, existing := range s.Records {
		if existing.Key != record.Key {
			continue
		}
		if existing.Digest == record.Digest {
			return ObservationCheckResult{
				Hit:                 true,
				ModelVisibleContent: observationUnchangedStub(existing),
				Record:              existing,
				Event: ContextGovernanceEvent{
					Layer:        ContextGovernanceLayerL2,
					Kind:         "observation.dedupe.hit",
					ReferenceIDs: []string{existing.SourceRef},
				},
			}
		}
		s.Upsert(record)
		return ObservationCheckResult{
			Changed:             true,
			ModelVisibleContent: observationDeltaStub(existing, record),
			Record:              record,
			Previous:            existing,
			Event: ContextGovernanceEvent{
				Layer:        ContextGovernanceLayerL2,
				Kind:         "observation.dedupe.changed",
				ReferenceIDs: []string{record.SourceRef},
			},
		}
	}
	s.Upsert(record)
	return ObservationCheckResult{
		ModelVisibleContent: record.Summary,
		Record:              record,
		Event: ContextGovernanceEvent{
			Layer:        ContextGovernanceLayerL2,
			Kind:         "observation.dedupe.miss",
			ReferenceIDs: []string{record.SourceRef},
		},
	}
}

func ObservationDigest(content string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(content)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func normalizeObservationRecord(record ObservationRecord) ObservationRecord {
	record.Key = strings.TrimSpace(record.Key)
	if record.Key == "" {
		record.Key = ObservationKey{
			ToolName:      record.ToolName,
			Target:        record.Target,
			Window:        record.Window,
			SourceVersion: record.SourceVersion,
		}.String()
	}
	if record.Digest == "" && record.Summary != "" {
		record.Digest = ObservationDigest(record.Summary)
	}
	if record.LastObservedAt.IsZero() {
		record.LastObservedAt = time.Now().UTC()
	} else {
		record.LastObservedAt = record.LastObservedAt.UTC()
	}
	return record
}

func observationUnchangedStub(record ObservationRecord) string {
	return fmt.Sprintf("Observation unchanged since last collection. Previous evidence ref is still current: %s. Summary: %s", record.SourceRef, record.Summary)
}

func observationDeltaStub(previous, current ObservationRecord) string {
	return fmt.Sprintf("Observation changed since previous digest %s. New evidence ref: %s. Summary: %s", previous.Digest, current.SourceRef, current.Summary)
}
