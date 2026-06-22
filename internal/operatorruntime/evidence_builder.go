package operatorruntime

import (
	"fmt"
	"strings"
)

type EvidenceKind string

const (
	EvidenceSupporting    EvidenceKind = "supporting"
	EvidenceContradicting EvidenceKind = "contradicting"
	EvidenceMissing       EvidenceKind = "missing"
)

type EvidenceGraph struct {
	ProblemTypeID string         `json:"problemTypeId"`
	ReplicaID     string         `json:"replicaId,omitempty"`
	Items         []EvidenceItem `json:"items"`
}

type EvidenceItem struct {
	Kind  EvidenceKind `json:"kind"`
	Field string       `json:"field"`
	Value string       `json:"value,omitempty"`
}

func BuildEvidence(match ProblemMatch, result InspectionResult) EvidenceGraph {
	items := make([]EvidenceItem, 0, len(result.Fields)+len(result.Errors))
	for field, value := range result.Fields {
		kind := EvidenceSupporting
		if !value.Known {
			kind = EvidenceMissing
		}
		items = append(items, EvidenceItem{Kind: kind, Field: field, Value: redactValue(fieldValueString(value))})
	}
	for _, errText := range result.Errors {
		items = append(items, EvidenceItem{Kind: EvidenceMissing, Field: "error", Value: redactValue(errText)})
	}
	return EvidenceGraph{ProblemTypeID: match.ProblemTypeID, ReplicaID: result.ReplicaID, Items: items}
}

func fieldValueString(value FieldValue) string {
	if !value.Known {
		return "unknown"
	}
	switch value.Type {
	case FieldTypeBool:
		return fmt.Sprint(value.Bool)
	case FieldTypeNumber:
		return fmt.Sprintf("%.0f", value.Number)
	default:
		return value.String
	}
}

func redactValue(value string) string {
	parts := strings.Fields(value)
	for index, part := range parts {
		lower := strings.ToLower(part)
		if strings.Contains(lower, "password=") || strings.Contains(lower, "token=") || strings.Contains(lower, "secret=") {
			key := strings.SplitN(part, "=", 2)[0]
			parts[index] = key + "=[REDACTED]"
		}
	}
	return strings.Join(parts, " ")
}
