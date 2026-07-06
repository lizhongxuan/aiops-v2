package specialinputmemory

import (
	"strings"
	"time"
)

type ConsolidateInput struct {
	SessionID string
	TaskID    string
	TurnID    string
	Mentions  []MentionObservation
	Intent    UserSpecialInputIntent
	Now       time.Time
}

func Consolidate(state SessionSpecialInputState, input ConsolidateInput) (SessionSpecialInputState, []SpecialInputMemoryEvent) {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	next := state.Normalize(input.SessionID, input.TaskID, now)
	next.UpdatedAt = now
	next.LastUpdatedTurnID = strings.TrimSpace(input.TurnID)
	var events []SpecialInputMemoryEvent

	if compactToken(input.Intent.Kind) == IntentForget {
		next, events = revokeMatching(next, events, input.Intent, input.TurnID, now, firstNonEmpty(input.Intent.Reason, "forget"))
		next.Conflicts = DetectRoleBindingConflicts(next.RoleBindings)
		return next, events
	}

	if compactToken(input.Intent.Kind) == IntentConfirm {
		next, events = confirmMatchingRawFacts(next, events, input.Intent, input.TurnID, now)
		next.Conflicts = DetectRoleBindingConflicts(next.RoleBindings)
		return next, events
	}

	if compactToken(input.Intent.Kind) == IntentCorrection {
		next, events = revokeMatching(next, events, input.Intent, input.TurnID, now, firstNonEmpty(input.Intent.Reason, "correction"))
	}

	hasExplicitHost := false
	for _, observation := range input.Mentions {
		if compactToken(observation.Kind) == FactKindHost && normalizedSource(observation.Source) == SourceStructuredSelection && normalizedTrustLevel(observation.TrustLevel) != TrustLevelRawTyped {
			hasExplicitHost = true
			break
		}
	}
	if hasExplicitHost {
		next, events = supersedeActiveHostGrants(next, events, input.Mentions, input.TurnID, now)
	}

	for _, observation := range input.Mentions {
		fact := factFromObservation(observation, input.TurnID, now)
		if fact.CanonicalKey == "" {
			continue
		}
		next.Facts = upsertFact(next.Facts, fact)
		events = appendEvent(events, SpecialInputMemoryEvent{
			Type:         "fact_upserted",
			FactID:       fact.ID,
			CanonicalKey: fact.CanonicalKey,
		}, input.TurnID, now)
		if canCreateGrant(fact) {
			grant := NewExecutionScopeGrant(fact, GrantInput{
				TurnID:         input.TurnID,
				Now:            now,
				Scope:          ScopeCurrentTask,
				AllowedActions: observation.AllowedActions,
				Source:         fact.Source,
			})
			next.Grants = upsertGrant(next.Grants, grant)
			if next.Focus == nil {
				next.Focus = &SpecialInputFocus{}
			}
			next.Focus.ActiveGrantID = grant.ID
			next.Focus.ActiveFactID = fact.ID
			next.Focus.EnvironmentKey = fact.EnvironmentKey
			next.Focus.ClusterKey = fact.ClusterKey
			next.Focus.LastExplicitTurnID = strings.TrimSpace(input.TurnID)
			events = appendEvent(events, SpecialInputMemoryEvent{
				Type:         "grant_created",
				FactID:       fact.ID,
				GrantID:      grant.ID,
				CanonicalKey: grant.CanonicalKey,
			}, input.TurnID, now)
		}
		if strings.TrimSpace(observation.RoleKey) != "" {
			binding := NewMentionRoleBinding(RoleBindingInput{
				ResourceKind:   fact.ResourceKind,
				ResourceID:     fact.ResourceID,
				Display:        fact.Display,
				RoleKey:        observation.RoleKey,
				RuntimeName:    observation.RuntimeName,
				EnvironmentKey: observation.EnvironmentKey,
				ClusterKey:     observation.ClusterKey,
				Source:         fact.Source,
				SourceTurnID:   input.TurnID,
				Confidence:     1,
				Now:            now,
			})
			next.RoleBindings = upsertRoleBinding(next.RoleBindings, binding)
			events = appendEvent(events, SpecialInputMemoryEvent{
				Type:          "role_binding_upserted",
				RoleBindingID: binding.ID,
				CanonicalKey:  fact.CanonicalKey,
			}, input.TurnID, now)
		}
	}
	next.Conflicts = DetectRoleBindingConflicts(next.RoleBindings)
	return next, events
}

func confirmMatchingRawFacts(state SessionSpecialInputState, events []SpecialInputMemoryEvent, intent UserSpecialInputIntent, turnID string, now time.Time) (SessionSpecialInputState, []SpecialInputMemoryEvent) {
	targetKind := compactToken(intent.TargetKind)
	targetKey := strings.TrimSpace(intent.CanonicalKey)
	matches := matchingRawConfirmFacts(state.Facts, targetKind, targetKey)
	if targetKey == "" && len(matches) != 1 {
		events = appendEvent(events, SpecialInputMemoryEvent{
			Type:   "confirm_rejected",
			Reason: "ambiguous_or_missing_raw_candidate",
		}, turnID, now)
		return state, events
	}
	for _, factIndex := range matches {
		state.Facts[factIndex].TrustLevel = TrustLevelServerConfirmed
		state.Facts[factIndex].Source = SourceUserConfirmation
		state.Facts[factIndex].LastUsedTurnID = strings.TrimSpace(turnID)
		state.Facts[factIndex].LastSeenTurnID = strings.TrimSpace(turnID)
		state.Facts[factIndex].ExpiresAt = now.Add(ttlForFact(state.Facts[factIndex]))
		events = appendEvent(events, SpecialInputMemoryEvent{
			Type:         "fact_confirmed",
			FactID:       state.Facts[factIndex].ID,
			CanonicalKey: state.Facts[factIndex].CanonicalKey,
		}, turnID, now)
		if canCreateGrant(state.Facts[factIndex]) {
			grant := NewExecutionScopeGrant(state.Facts[factIndex], GrantInput{
				TurnID: turnID,
				Now:    now,
				Scope:  ScopeCurrentTask,
				Source: SourceUserConfirmation,
			})
			state.Grants = upsertGrant(state.Grants, grant)
			if state.Focus == nil {
				state.Focus = &SpecialInputFocus{}
			}
			state.Focus.ActiveGrantID = grant.ID
			state.Focus.ActiveFactID = state.Facts[factIndex].ID
			state.Focus.LastExplicitTurnID = strings.TrimSpace(turnID)
			events = appendEvent(events, SpecialInputMemoryEvent{
				Type:         "grant_created",
				FactID:       state.Facts[factIndex].ID,
				GrantID:      grant.ID,
				CanonicalKey: grant.CanonicalKey,
				Reason:       "user_confirmation",
			}, turnID, now)
		}
	}
	return state, events
}

func matchingRawConfirmFacts(facts []MentionFact, targetKind, targetKey string) []int {
	var matches []int
	for i := range facts {
		if normalizedFactStatus(facts[i].Status) != FactStatusActive || normalizedTrustLevel(facts[i].TrustLevel) != TrustLevelRawTyped {
			continue
		}
		if targetKey != "" && facts[i].CanonicalKey != targetKey {
			continue
		}
		if targetKind != "" && facts[i].Kind != targetKind && facts[i].ResourceKind != resourceKindForFactKind(targetKind) {
			continue
		}
		matches = append(matches, i)
	}
	return matches
}

func factFromObservation(observation MentionObservation, turnID string, now time.Time) MentionFact {
	kind := compactToken(firstNonEmpty(observation.Kind, FactKindUnknown))
	source := normalizedSource(observation.Source)
	trust := normalizedTrustLevel(observation.TrustLevel)
	status := FactStatusActive
	ttl := ttlForFact(MentionFact{Kind: kind, ResourceKind: observation.ResourceKind, TrustLevel: trust})
	canonicalKey := strings.TrimSpace(observation.CanonicalKey)
	if canonicalKey == "" && strings.TrimSpace(observation.ResourceID) != "" {
		canonicalKey = compactToken(firstNonEmpty(observation.ResourceKind, kind)) + ":" + strings.TrimSpace(observation.ResourceID)
	}
	fact := MentionFact{
		Kind:             kind,
		CanonicalKey:     canonicalKey,
		Display:          compactDisplay(firstNonEmpty(observation.Display, observation.ResourceID, canonicalKey)),
		RawText:          strings.TrimSpace(observation.RawText),
		Path:             strings.TrimSpace(observation.Path),
		Source:           source,
		TrustLevel:       trust,
		Status:           status,
		Scope:            ScopeCurrentTask,
		ResourceKind:     compactToken(firstNonEmpty(observation.ResourceKind, kind)),
		ResourceID:       strings.TrimSpace(observation.ResourceID),
		EnvironmentKey:   compactToken(observation.EnvironmentKey),
		ClusterKey:       compactToken(observation.ClusterKey),
		FirstSeenTurnID:  strings.TrimSpace(turnID),
		LastSeenTurnID:   strings.TrimSpace(turnID),
		ExpiresAt:        now.Add(ttl),
		ValidationHash:   strings.TrimSpace(observation.ValidationHash),
		ValidationSource: "",
		Weight:           1,
	}
	if fact.ResourceKind == "" {
		fact.ResourceKind = kind
	}
	if fact.ID == "" {
		fact.ID = stableHash("special-input-memory.fact", map[string]any{
			"canonicalKey": fact.CanonicalKey,
			"kind":         fact.Kind,
			"turnID":       fact.FirstSeenTurnID,
		})
	}
	return fact
}

func upsertFact(facts []MentionFact, next MentionFact) []MentionFact {
	for i := range facts {
		if facts[i].CanonicalKey == next.CanonicalKey && facts[i].Kind == next.Kind {
			if facts[i].FirstSeenTurnID != "" {
				next.FirstSeenTurnID = facts[i].FirstSeenTurnID
			}
			next.Weight = facts[i].Weight + 1
			facts[i] = next
			return facts
		}
	}
	return append(facts, next)
}

func upsertGrant(grants []ExecutionScopeGrant, next ExecutionScopeGrant) []ExecutionScopeGrant {
	for i := range grants {
		if grants[i].CanonicalKey == next.CanonicalKey && grants[i].ResourceKind == next.ResourceKind && grants[i].Scope == next.Scope {
			if grants[i].CreatedTurnID != "" {
				next.CreatedTurnID = grants[i].CreatedTurnID
			}
			next.Weight = grants[i].Weight + 1
			grants[i] = next
			return grants
		}
	}
	return append(grants, next)
}

func supersedeActiveHostGrants(state SessionSpecialInputState, events []SpecialInputMemoryEvent, observations []MentionObservation, turnID string, now time.Time) (SessionSpecialInputState, []SpecialInputMemoryEvent) {
	newKeys := map[string]struct{}{}
	for _, observation := range observations {
		if compactToken(observation.Kind) == FactKindHost {
			newKeys[strings.TrimSpace(observation.CanonicalKey)] = struct{}{}
		}
	}
	for i := range state.Grants {
		if state.Grants[i].ResourceKind != ResourceKindHost || normalizedGrantStatus(state.Grants[i].Status) != GrantStatusActive {
			continue
		}
		if _, ok := newKeys[state.Grants[i].CanonicalKey]; ok {
			continue
		}
		state.Grants[i].Status = GrantStatusStale
		state.Grants[i].RevokedReason = "superseded_by_explicit_mention"
		events = appendEvent(events, SpecialInputMemoryEvent{
			Type:         "grant_superseded",
			GrantID:      state.Grants[i].ID,
			CanonicalKey: state.Grants[i].CanonicalKey,
			Reason:       state.Grants[i].RevokedReason,
		}, turnID, now)
	}
	return state, events
}

func revokeMatching(state SessionSpecialInputState, events []SpecialInputMemoryEvent, intent UserSpecialInputIntent, turnID string, now time.Time, reason string) (SessionSpecialInputState, []SpecialInputMemoryEvent) {
	targetKind := compactToken(intent.TargetKind)
	targetKey := strings.TrimSpace(intent.CanonicalKey)
	for i := range state.Grants {
		if targetKey != "" && state.Grants[i].CanonicalKey != targetKey {
			continue
		}
		if targetKind != "" && state.Grants[i].ResourceKind != targetKind && state.Grants[i].ResourceKind != resourceKindForFactKind(targetKind) {
			continue
		}
		if normalizedGrantStatus(state.Grants[i].Status) != GrantStatusActive {
			continue
		}
		state.Grants[i].Status = GrantStatusRevoked
		state.Grants[i].RevokedReason = reason
		state.Tombstones = append(state.Tombstones, tombstoneForGrant(state.Grants[i], reason, turnID, now))
		events = appendEvent(events, SpecialInputMemoryEvent{
			Type:         "grant_revoked",
			GrantID:      state.Grants[i].ID,
			CanonicalKey: state.Grants[i].CanonicalKey,
			Reason:       reason,
		}, turnID, now)
	}
	for i := range state.RoleBindings {
		if targetKind != "" && state.RoleBindings[i].ResourceKind != targetKind && state.RoleBindings[i].ResourceKind != resourceKindForFactKind(targetKind) {
			continue
		}
		if state.RoleBindings[i].Status != RoleBindingStatusActive {
			continue
		}
		state.RoleBindings[i].Status = RoleBindingStatusRevoked
		events = appendEvent(events, SpecialInputMemoryEvent{
			Type:          "role_binding_revoked",
			RoleBindingID: state.RoleBindings[i].ID,
			Reason:        reason,
		}, turnID, now)
	}
	return state, events
}

func tombstoneForGrant(grant ExecutionScopeGrant, reason, turnID string, now time.Time) MemoryTombstone {
	tombstone := MemoryTombstone{
		Kind:         grant.ResourceKind,
		CanonicalKey: grant.CanonicalKey,
		ResourceID:   grant.ResourceID,
		Reason:       reason,
		SourceTurnID: strings.TrimSpace(turnID),
		CreatedAt:    now,
		ExpiresAt:    now.Add(tombstoneTTL),
	}
	tombstone.ID = stableHash("special-input-memory.tombstone", map[string]any{
		"canonicalKey": tombstone.CanonicalKey,
		"reason":       tombstone.Reason,
		"turnID":       tombstone.SourceTurnID,
		"createdAt":    tombstone.CreatedAt.UnixNano(),
	})
	return tombstone
}

func resourceKindForFactKind(kind string) string {
	switch compactToken(kind) {
	case FactKindHost:
		return ResourceKindHost
	case FactKindCapability:
		return ResourceKindCapability
	case FactKindOpsManual:
		return ResourceKindOpsManual
	case FactKindOpsGraph:
		return ResourceKindOpsGraph
	case FactKindWorkflow:
		return ResourceKindWorkflow
	case FactKindFile:
		return ResourceKindFile
	default:
		return compactToken(kind)
	}
}
