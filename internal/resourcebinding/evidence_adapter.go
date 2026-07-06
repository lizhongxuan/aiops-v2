package resourcebinding

import "strings"

const (
	EvidenceSourceUser    = "user"
	EvidenceSourceTool    = "tool"
	EvidenceSourceRuntime = "runtime"

	EvidenceKindObservation   = "observation"
	EvidenceKindCommandOutput = "command_output"
	EvidenceKindMetric        = "metric"
	EvidenceKindAlert         = "alert"
	EvidenceKindHypothesis    = "hypothesis"
)

type EvidenceInput struct {
	ID          string
	ResourceRef ResourceRef
	Source      string
	Kind        string
}

func BuildEvidenceRef(input EvidenceInput) EvidenceRef {
	ref := NormalizeRef(input.ResourceRef)
	if ref.IsZero() {
		ref = ResourceRef{Type: ResourceTypeSession, ID: SessionResourceID}
	}
	out := EvidenceRef{
		ID:          strings.TrimSpace(input.ID),
		ResourceRef: ref,
		Source:      normalizeEvidenceSource(input.Source),
		Kind:        normalizeEvidenceKind(input.Kind),
	}
	out.TraceHash = StableTraceHash("resource-evidence", map[string]any{
		"id":       out.ID,
		"resource": out.ResourceRef,
		"source":   out.Source,
		"kind":     out.Kind,
	})
	return out
}

func normalizeEvidenceSource(value string) string {
	switch normalizeToken(value) {
	case EvidenceSourceUser:
		return EvidenceSourceUser
	case EvidenceSourceTool:
		return EvidenceSourceTool
	case EvidenceSourceRuntime:
		return EvidenceSourceRuntime
	default:
		return strings.TrimSpace(value)
	}
}

func normalizeEvidenceKind(value string) string {
	switch normalizeToken(value) {
	case EvidenceKindObservation:
		return EvidenceKindObservation
	case EvidenceKindCommandOutput:
		return EvidenceKindCommandOutput
	case EvidenceKindMetric:
		return EvidenceKindMetric
	case EvidenceKindAlert:
		return EvidenceKindAlert
	case EvidenceKindHypothesis:
		return EvidenceKindHypothesis
	default:
		return EvidenceKindObservation
	}
}
