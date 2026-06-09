package runtimekernel

import (
	"encoding/json"
	"strconv"
	"strings"
	"time"
)

const CompactBoundaryType = "context_compaction_boundary"

type CompactBoundaryInput struct {
	SegmentID          string
	CompactedTurnStart int
	CompactedTurnEnd   int
	PreservedTailCount int
	CreatedAt          time.Time
}

type CompactBoundaryMetadata struct {
	Type                 string               `json:"type"`
	SegmentID            string               `json:"segmentId"`
	SummarySchemaVersion string               `json:"summarySchemaVersion"`
	CompactedTurnRange   CompactBoundaryRange `json:"compactedTurnRange"`
	PreservedTailCount   int                  `json:"preservedTailCount"`
	CreatedAt            time.Time            `json:"createdAt"`
}

type CompactBoundaryRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

func NewCompactBoundaryMessage(input CompactBoundaryInput) Message {
	meta := NewCompactBoundaryMetadata(input)
	payload, _ := json.Marshal(meta)
	return Message{
		ID:        meta.SegmentID + "-boundary",
		Role:      "system",
		Content:   "<context_compaction_boundary>" + string(payload) + "</context_compaction_boundary>",
		Metadata:  compactBoundaryMetadataMap(meta),
		Timestamp: meta.CreatedAt,
	}
}

func NewCompactBoundaryMetadata(input CompactBoundaryInput) CompactBoundaryMetadata {
	createdAt := input.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now().UTC()
	} else {
		createdAt = createdAt.UTC()
	}
	return CompactBoundaryMetadata{
		Type:                 CompactBoundaryType,
		SegmentID:            input.SegmentID,
		SummarySchemaVersion: CompactSummarySchemaVersionV1,
		CompactedTurnRange: CompactBoundaryRange{
			Start: input.CompactedTurnStart,
			End:   input.CompactedTurnEnd,
		},
		PreservedTailCount: input.PreservedTailCount,
		CreatedAt:          createdAt,
	}
}

func IsCompactBoundaryMessage(msg Message) bool {
	if msg.Metadata != nil && msg.Metadata["type"] == CompactBoundaryType {
		return true
	}
	return strings.Contains(msg.Content, "<context_compaction_boundary>")
}

func CompactBoundaryMetadataFromMessage(msg Message) (CompactBoundaryMetadata, bool) {
	if msg.Metadata != nil && msg.Metadata["type"] == CompactBoundaryType {
		meta, ok := compactBoundaryMetadataFromMap(msg.Metadata)
		if ok {
			return meta, true
		}
	}
	const startTag = "<context_compaction_boundary>"
	const endTag = "</context_compaction_boundary>"
	start := strings.Index(msg.Content, startTag)
	end := strings.Index(msg.Content, endTag)
	if start < 0 || end <= start {
		return CompactBoundaryMetadata{}, false
	}
	raw := msg.Content[start+len(startTag) : end]
	var meta CompactBoundaryMetadata
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		return CompactBoundaryMetadata{}, false
	}
	return meta, meta.Type == CompactBoundaryType
}

func compactBoundaryMetadataMap(meta CompactBoundaryMetadata) map[string]string {
	return map[string]string{
		"type":                 meta.Type,
		"segmentId":            meta.SegmentID,
		"summarySchemaVersion": meta.SummarySchemaVersion,
		"compactedTurnRange":   strconv.Itoa(meta.CompactedTurnRange.Start) + "-" + strconv.Itoa(meta.CompactedTurnRange.End),
		"preservedTailCount":   strconv.Itoa(meta.PreservedTailCount),
		"createdAt":            meta.CreatedAt.Format(time.RFC3339Nano),
	}
}

func compactBoundaryMetadataFromMap(values map[string]string) (CompactBoundaryMetadata, bool) {
	if values["type"] != CompactBoundaryType {
		return CompactBoundaryMetadata{}, false
	}
	start, end, ok := parseCompactBoundaryRange(values["compactedTurnRange"])
	if !ok {
		return CompactBoundaryMetadata{}, false
	}
	preservedTailCount, err := strconv.Atoi(values["preservedTailCount"])
	if err != nil {
		return CompactBoundaryMetadata{}, false
	}
	createdAt, err := time.Parse(time.RFC3339Nano, values["createdAt"])
	if err != nil {
		return CompactBoundaryMetadata{}, false
	}
	return CompactBoundaryMetadata{
		Type:                 values["type"],
		SegmentID:            values["segmentId"],
		SummarySchemaVersion: values["summarySchemaVersion"],
		CompactedTurnRange:   CompactBoundaryRange{Start: start, End: end},
		PreservedTailCount:   preservedTailCount,
		CreatedAt:            createdAt,
	}, true
}

func parseCompactBoundaryRange(value string) (int, int, bool) {
	parts := strings.Split(value, "-")
	if len(parts) != 2 {
		return 0, 0, false
	}
	start, err := strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, false
	}
	end, err := strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, false
	}
	return start, end, true
}
