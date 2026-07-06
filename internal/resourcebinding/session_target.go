package resourcebinding

import (
	"sort"
	"strings"
)

const (
	BindingModeNone       = "none"
	BindingModeSingleHost = "single_host"
	BindingModeMultiHost  = "multi_host"
)

type SessionTargetSnapshot struct {
	ActiveTargetSetID    string   `json:"activeTargetSetId,omitempty"`
	HostIDs              []string `json:"hostIds,omitempty"`
	SourceTurnID         string   `json:"sourceTurnId,omitempty"`
	SourceMentionIDs     []string `json:"sourceMentionIds,omitempty"`
	BindingMode          string   `json:"bindingMode,omitempty"`
	ExpiresAfterTurns    int      `json:"expiresAfterTurns,omitempty"`
	RequiresConfirmation bool     `json:"requiresConfirmation,omitempty"`
	Confidence           float64  `json:"confidence,omitempty"`
	TraceHash            string   `json:"traceHash,omitempty"`
}

type SessionTargetInput struct {
	HostIDs              []string
	SourceTurnID         string
	SourceMentionIDs     []string
	ExpiresAfterTurns    int
	RequiresConfirmation bool
	Confidence           float64
}

func NewSessionTargetSnapshot(input SessionTargetInput) *SessionTargetSnapshot {
	hostIDs := uniqueSorted(input.HostIDs)
	mode := BindingModeNone
	switch len(hostIDs) {
	case 0:
		mode = BindingModeNone
	case 1:
		mode = BindingModeSingleHost
	default:
		mode = BindingModeMultiHost
	}
	expires := input.ExpiresAfterTurns
	if expires <= 0 {
		expires = 6
	}
	confidence := input.Confidence
	if confidence <= 0 && len(hostIDs) > 0 {
		confidence = 1
	}
	snapshot := &SessionTargetSnapshot{
		HostIDs:              hostIDs,
		SourceTurnID:         strings.TrimSpace(input.SourceTurnID),
		SourceMentionIDs:     uniqueSorted(input.SourceMentionIDs),
		BindingMode:          mode,
		ExpiresAfterTurns:    expires,
		RequiresConfirmation: input.RequiresConfirmation,
		Confidence:           confidence,
	}
	if len(hostIDs) > 0 {
		snapshot.ActiveTargetSetID = StableTraceHash("session-target.id", map[string]any{
			"hostIds":      hostIDs,
			"sourceTurnID": snapshot.SourceTurnID,
		})
	}
	snapshot.TraceHash = StableTraceHash("session-target.snapshot", map[string]any{
		"targetSetID":          snapshot.ActiveTargetSetID,
		"hostIds":              snapshot.HostIDs,
		"sourceTurnID":         snapshot.SourceTurnID,
		"sourceMentionIDs":     snapshot.SourceMentionIDs,
		"bindingMode":          snapshot.BindingMode,
		"expiresAfterTurns":    snapshot.ExpiresAfterTurns,
		"requiresConfirmation": snapshot.RequiresConfirmation,
		"confidence":           snapshot.Confidence,
	})
	return snapshot
}

func SessionTargetFromVerifiedBindings(bindings []ResourceBindingSnapshot, sourceTurnID string, mentionIDs []string) *SessionTargetSnapshot {
	var hostIDs []string
	for _, binding := range bindings {
		if binding.Ref.Type != ResourceTypeHost || !binding.Verified() {
			continue
		}
		if id := strings.TrimSpace(binding.Ref.ID); id != "" {
			hostIDs = append(hostIDs, id)
		}
	}
	return NewSessionTargetSnapshot(SessionTargetInput{
		HostIDs:           hostIDs,
		SourceTurnID:      sourceTurnID,
		SourceMentionIDs:  mentionIDs,
		ExpiresAfterTurns: 6,
		Confidence:        1,
	})
}

func SessionTargetCleared(sourceTurnID string) *SessionTargetSnapshot {
	return NewSessionTargetSnapshot(SessionTargetInput{
		SourceTurnID:         sourceTurnID,
		RequiresConfirmation: true,
		Confidence:           0,
	})
}

func (s *SessionTargetSnapshot) Expired() bool {
	return s == nil || s.ExpiresAfterTurns <= 0 || len(s.HostIDs) == 0
}

func (s *SessionTargetSnapshot) NextTurn() *SessionTargetSnapshot {
	if s == nil {
		return nil
	}
	next := *s
	next.HostIDs = append([]string(nil), s.HostIDs...)
	next.SourceMentionIDs = append([]string(nil), s.SourceMentionIDs...)
	if next.ExpiresAfterTurns > 0 {
		next.ExpiresAfterTurns--
	}
	next.TraceHash = StableTraceHash("session-target.snapshot", map[string]any{
		"targetSetID":          next.ActiveTargetSetID,
		"hostIds":              next.HostIDs,
		"sourceTurnID":         next.SourceTurnID,
		"sourceMentionIDs":     next.SourceMentionIDs,
		"bindingMode":          next.BindingMode,
		"expiresAfterTurns":    next.ExpiresAfterTurns,
		"requiresConfirmation": next.RequiresConfirmation,
		"confidence":           next.Confidence,
	})
	return &next
}

func HostIDsFromSessionTarget(snapshot *SessionTargetSnapshot) []string {
	if snapshot == nil {
		return nil
	}
	out := append([]string(nil), snapshot.HostIDs...)
	sort.Strings(out)
	return out
}
