package runtimekernel

import (
	"fmt"
	"strings"
	"time"

	"aiops-v2/internal/resourceio"
)

type ResourceIdentity struct {
	Scheme             string        `json:"scheme,omitempty"`
	URI                string        `json:"uri,omitempty"`
	Version            string        `json:"version,omitempty"`
	Digest             string        `json:"digest,omitempty"`
	TargetIdentityHash string        `json:"targetIdentityHash,omitempty"`
	Range              ResourceRange `json:"range,omitempty"`
}

type ResourceRange = resourceio.Range

type ResourceReadRecord struct {
	Identity       ResourceIdentity `json:"identity"`
	SourceRef      string           `json:"sourceRef,omitempty"`
	Summary        string           `json:"summary,omitempty"`
	ContentSnippet string           `json:"contentSnippet,omitempty"`
	Content        string           `json:"content,omitempty"`
	ContentType    string           `json:"contentType,omitempty"`
	Bytes          int64            `json:"bytes,omitempty"`
	LastReadAt     time.Time        `json:"lastReadAt,omitempty"`
}

type ResourceCheckResult struct {
	Unchanged           bool                   `json:"unchanged,omitempty"`
	Changed             bool                   `json:"changed,omitempty"`
	Miss                bool                   `json:"miss,omitempty"`
	ModelVisibleContent string                 `json:"modelVisibleContent,omitempty"`
	Record              ResourceReadRecord     `json:"record"`
	Previous            ResourceReadRecord     `json:"previous,omitempty"`
	Event               ContextGovernanceEvent `json:"event"`
}

func (s *ObservationState) CheckResource(record ResourceReadRecord) ResourceCheckResult {
	record = normalizeResourceReadRecord(record)
	if !record.Identity.hasDigest() {
		s.upsertResource(record)
		return ResourceCheckResult{
			Miss:                true,
			ModelVisibleContent: boundedResourceMissContent(record),
			Record:              record,
			Event:               resourceDedupeEvent("resource.dedupe.miss", record),
		}
	}
	for _, existing := range s.ResourceRecords {
		if existing.Identity.rangeKey() != record.Identity.rangeKey() {
			continue
		}
		if existing.Identity.Digest != "" && record.Identity.Digest != "" {
			if existing.Identity.Digest == record.Identity.Digest {
				return ResourceCheckResult{
					Unchanged:           true,
					ModelVisibleContent: resourceUnchangedStub(existing),
					Record:              existing,
					Event:               resourceDedupeEvent("resource.dedupe.hit", existing),
				}
			}
			s.upsertResource(record)
			return ResourceCheckResult{
				Changed:             true,
				ModelVisibleContent: resourceChangedStub(existing, record),
				Record:              record,
				Previous:            existing,
				Event:               resourceDedupeEvent("resource.dedupe.changed", record),
			}
		}
		if existing.Identity.fullKey() == record.Identity.fullKey() {
			return ResourceCheckResult{
				Unchanged:           true,
				ModelVisibleContent: resourceUnchangedStub(existing),
				Record:              existing,
				Event:               resourceDedupeEvent("resource.dedupe.hit", existing),
			}
		}
		s.upsertResource(record)
		return ResourceCheckResult{
			Miss:                true,
			ModelVisibleContent: boundedResourceMissContent(record),
			Record:              record,
			Previous:            existing,
			Event:               resourceDedupeEvent("resource.dedupe.miss", record),
		}
	}
	s.upsertResource(record)
	return ResourceCheckResult{
		Miss:                true,
		ModelVisibleContent: boundedResourceMissContent(record),
		Record:              record,
		Event:               resourceDedupeEvent("resource.dedupe.miss", record),
	}
}

func (s *ObservationState) upsertResource(record ResourceReadRecord) {
	record = normalizeResourceReadRecord(record)
	key := record.Identity.rangeKey()
	for i := range s.ResourceRecords {
		if s.ResourceRecords[i].Identity.rangeKey() == key {
			s.ResourceRecords[i] = record
			return
		}
	}
	s.ResourceRecords = append(s.ResourceRecords, record)
}

func normalizeResourceReadRecord(record ResourceReadRecord) ResourceReadRecord {
	record.Identity = record.Identity.normalize()
	record.SourceRef = strings.TrimSpace(record.SourceRef)
	record.Summary = strings.TrimSpace(record.Summary)
	record.ContentSnippet = strings.TrimSpace(record.ContentSnippet)
	record.ContentType = strings.TrimSpace(record.ContentType)
	if record.Summary == "" {
		record.Summary = contextArtifactBoundedSnippet(contextArtifactFirstNonBlank(record.ContentSnippet, record.Content))
	}
	if record.ContentSnippet == "" {
		record.ContentSnippet = contextArtifactBoundedSnippet(contextArtifactFirstNonBlank(record.Content, record.Summary))
	}
	if record.LastReadAt.IsZero() {
		record.LastReadAt = time.Now().UTC()
	} else {
		record.LastReadAt = record.LastReadAt.UTC()
	}
	return record
}

func (id ResourceIdentity) normalize() ResourceIdentity {
	id.Scheme = strings.TrimSpace(id.Scheme)
	id.URI = strings.TrimSpace(id.URI)
	id.Version = strings.TrimSpace(id.Version)
	id.Digest = strings.TrimSpace(id.Digest)
	id.TargetIdentityHash = strings.TrimSpace(id.TargetIdentityHash)
	id.Range = resourceio.NormalizeRangeValue(id.Range, resourceio.DefaultMaxReadBytes)
	if id.Scheme == "" && strings.Contains(id.URI, "://") {
		id.Scheme = strings.SplitN(id.URI, "://", 2)[0]
	}
	return id
}

func (id ResourceIdentity) hasDigest() bool {
	return strings.TrimSpace(id.Digest) != ""
}

func (id ResourceIdentity) rangeKey() string {
	id = id.normalize()
	return strings.Join([]string{
		"target=" + id.TargetIdentityHash,
		id.Scheme,
		id.URI,
		fmt.Sprintf("offset=%d", id.Range.Offset),
		fmt.Sprintf("limit=%d", id.Range.Limit),
		"query=" + id.Range.Query,
		fmt.Sprintf("page=%d", id.Range.Page),
		"format=" + id.Range.Format,
	}, "|")
}

func (id ResourceIdentity) fullKey() string {
	id = id.normalize()
	return strings.Join([]string{
		id.rangeKey(),
		"version=" + id.Version,
		"digest=" + id.Digest,
	}, "|")
}

func boundedResourceMissContent(record ResourceReadRecord) string {
	if record.Identity.Digest == "" {
		return contextArtifactBoundedSnippet(contextArtifactFirstNonBlank(record.ContentSnippet, record.Summary, record.Content))
	}
	return contextArtifactFirstNonBlank(record.Content, record.ContentSnippet, record.Summary)
}

func resourceUnchangedStub(record ResourceReadRecord) string {
	return fmt.Sprintf("Resource unchanged for %s. Previous ref is still current: %s. Summary: %s", record.Identity.URI, record.SourceRef, record.Summary)
}

func resourceChangedStub(previous, current ResourceReadRecord) string {
	return fmt.Sprintf("Resource changed for %s since previous digest %s. Current ref: %s.", current.Identity.URI, previous.Identity.Digest, current.SourceRef)
}

func resourceDedupeEvent(kind string, record ResourceReadRecord) ContextGovernanceEvent {
	return ContextGovernanceEvent{
		Layer:        ContextGovernanceLayerL2,
		Kind:         kind,
		ReferenceIDs: []string{record.SourceRef},
		Resource: &ContextGovernanceResource{
			URI:         record.Identity.URI,
			Digest:      record.Identity.Digest,
			ContentType: record.ContentType,
			Bytes:       record.Bytes,
			Range:       record.Identity.Range,
		},
	}
}
