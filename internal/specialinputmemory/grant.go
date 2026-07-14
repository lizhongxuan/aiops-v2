package specialinputmemory

import (
	"sort"
	"strings"
	"time"
)

const (
	hostGrantTTL       = 30 * time.Minute
	rawTypedTTL        = 5 * time.Minute
	roleBindingTTL     = 30 * time.Minute
	manualGrantTTL     = 60 * time.Minute
	graphGrantTTL      = 60 * time.Minute
	capabilityGrantTTL = 15 * time.Minute
	tombstoneTTL       = 30 * time.Minute
)

type GrantInput struct {
	TurnID         string
	Now            time.Time
	Scope          string
	AllowedActions []string
	Source         string
}

func NewExecutionScopeGrant(fact MentionFact, input GrantInput) ExecutionScopeGrant {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	actions := normalizedActions(input.AllowedActions)
	if len(actions) == 0 {
		actions = defaultActionsForFact(fact)
	}
	source := normalizedSource(input.Source)
	if source == "" {
		source = fact.Source
	}
	scope := normalizedScope(input.Scope)
	ttl := ttlForFact(fact)
	grant := ExecutionScopeGrant{
		FactID:         fact.ID,
		CanonicalKey:   fact.CanonicalKey,
		ResourceKind:   firstNonEmpty(fact.ResourceKind, fact.Kind),
		ResourceID:     fact.ResourceID,
		Display:        firstNonEmpty(fact.Display, fact.ResourceID, fact.CanonicalKey),
		Scope:          scope,
		AllowedActions: actions,
		TrustLevel:     normalizedTrustLevel(fact.TrustLevel),
		Source:         source,
		Status:         GrantStatusActive,
		ValidationHash: fact.ValidationHash,
		CreatedTurnID:  strings.TrimSpace(input.TurnID),
		ExpiresAt:      now.Add(ttl),
		Weight:         1,
	}
	if grant.ResourceKind == "" {
		grant.ResourceKind = ResourceKindHost
	}
	if grant.ID == "" {
		grant.ID = stableHash("special-input-memory.grant", map[string]any{
			"factID":       grant.FactID,
			"canonicalKey": grant.CanonicalKey,
			"resourceKind": grant.ResourceKind,
			"resourceID":   grant.ResourceID,
			"scope":        grant.Scope,
			"turnID":       grant.CreatedTurnID,
		})
	}
	return grant
}

func (g ExecutionScopeGrant) Allows(action string) bool {
	action = compactToken(action)
	for _, allowed := range g.AllowedActions {
		if compactToken(allowed) == action {
			return true
		}
	}
	return false
}

func (g ExecutionScopeGrant) Expired(now time.Time) bool {
	if normalizedGrantStatus(g.Status) == GrantStatusExpired {
		return true
	}
	return !g.ExpiresAt.IsZero() && now.After(g.ExpiresAt)
}

func (g ExecutionScopeGrant) MarkUsed(turnID string, now time.Time) ExecutionScopeGrant {
	next := g
	next.LastUsedTurnID = strings.TrimSpace(turnID)
	next.Weight += 0.3
	return next
}

func ActiveGrants(grants []ExecutionScopeGrant) []ExecutionScopeGrant {
	var out []ExecutionScopeGrant
	for _, grant := range grants {
		if normalizedGrantStatus(grant.Status) != GrantStatusActive {
			continue
		}
		out = append(out, grant)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Weight != out[j].Weight {
			return out[i].Weight > out[j].Weight
		}
		if out[i].LastUsedTurnID != out[j].LastUsedTurnID {
			return out[i].LastUsedTurnID > out[j].LastUsedTurnID
		}
		return out[i].CreatedTurnID > out[j].CreatedTurnID
	})
	return out
}

func defaultActionsForFact(fact MentionFact) []string {
	switch compactToken(firstNonEmpty(fact.ResourceKind, fact.Kind)) {
	case ResourceKindHost:
		return []string{ActionExecLowRisk, ActionInspect, ActionRead}
	case ResourceKindOpsManual, ResourceKindOpsGraph:
		return []string{ActionRead}
	case ResourceKindCapability:
		return []string{ActionInspect, ActionRead}
	default:
		return []string{ActionRead}
	}
}

func normalizedActions(actions []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, action := range actions {
		action = compactToken(action)
		switch action {
		case ActionInspect, ActionRead, ActionExecLowRisk, ActionMutate, ActionDestructive:
			if _, ok := seen[action]; ok {
				continue
			}
			seen[action] = struct{}{}
			out = append(out, action)
		}
	}
	sort.Strings(out)
	return out
}

func ttlForFact(fact MentionFact) time.Duration {
	if normalizedTrustLevel(fact.TrustLevel) == TrustLevelRawTyped {
		return rawTypedTTL
	}
	switch compactToken(firstNonEmpty(fact.ResourceKind, fact.Kind)) {
	case ResourceKindOpsManual:
		return manualGrantTTL
	case ResourceKindOpsGraph:
		return graphGrantTTL
	case ResourceKindCapability:
		return capabilityGrantTTL
	default:
		return hostGrantTTL
	}
}

func canCreateGrant(fact MentionFact) bool {
	if normalizedFactStatus(fact.Status) != FactStatusActive {
		return false
	}
	if normalizedTrustLevel(fact.TrustLevel) == TrustLevelRawTyped {
		return false
	}
	source := normalizedSource(fact.Source)
	return source == SourceStructuredSelection || source == SourceCorrection || source == SourceUserConfirmation || source == SourceSystem || source == SourceRestored
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
