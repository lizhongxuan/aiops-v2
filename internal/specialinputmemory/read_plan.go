package specialinputmemory

import (
	"context"
	"sort"
	"strings"
	"time"
)

type MemoryReadPlanInput struct {
	SessionID      string
	TaskID         string
	TurnID         string
	Now            time.Time
	RequestedRole  string
	EnvironmentKey string
	ClusterKey     string
	HostResolver   HostResolver
}

type MemoryReadPlan struct {
	SchemaVersion         string                `json:"schemaVersion"`
	TurnID                string                `json:"turnId,omitempty"`
	VisibleFacts          []MentionFact         `json:"visibleFacts,omitempty"`
	CandidateFacts        []MentionFact         `json:"candidateFacts,omitempty"`
	ActiveExecutionScope  *ExecutionScopeGrant  `json:"activeExecutionScope,omitempty"`
	SuspendedGrants       []ExecutionScopeGrant `json:"suspendedGrants,omitempty"`
	CandidateRoleBindings []MentionRoleBinding  `json:"candidateRoleBindings,omitempty"`
	Conflicts             []MemoryConflict      `json:"conflicts,omitempty"`
	PendingConfirmations  []PendingConfirmation `json:"pendingConfirmations,omitempty"`
	ModelSummary          string                `json:"modelSummary,omitempty"`
}

type PendingConfirmation struct {
	ID             string   `json:"id,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	Reason         string   `json:"reason,omitempty"`
	RoleKey        string   `json:"roleKey,omitempty"`
	EnvironmentKey string   `json:"environmentKey,omitempty"`
	ClusterKey     string   `json:"clusterKey,omitempty"`
	CandidateIDs   []string `json:"candidateIds,omitempty"`
}

type HostResolver interface {
	RevalidateHost(ctx context.Context, ref HostRef) (HostValidation, error)
}

type HostRef struct {
	ResourceID   string
	CanonicalKey string
}

type HostValidation struct {
	ResourceID     string
	Available      bool
	ValidationHash string
	Reason         string
}

type StaticHostResolver map[string]HostValidation

func (r StaticHostResolver) RevalidateHost(_ context.Context, ref HostRef) (HostValidation, error) {
	if validation, ok := r[ref.ResourceID]; ok {
		return validation, nil
	}
	if validation, ok := r[ref.CanonicalKey]; ok {
		return validation, nil
	}
	return HostValidation{ResourceID: ref.ResourceID, Available: true}, nil
}

func BuildMemoryReadPlan(ctx context.Context, state SessionSpecialInputState, input MemoryReadPlanInput) MemoryReadPlan {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	state, _ = ApplyGC(state.Normalize(input.SessionID, input.TaskID, now), GCInput{Now: now, TurnID: input.TurnID})
	plan := MemoryReadPlan{
		SchemaVersion:         SchemaVersion,
		TurnID:                strings.TrimSpace(input.TurnID),
		VisibleFacts:          activeVisibleFacts(state.Facts),
		CandidateFacts:        candidateFacts(state.Facts),
		CandidateRoleBindings: activeRoleBindings(state.RoleBindings),
		Conflicts:             append([]MemoryConflict(nil), state.Conflicts...),
	}

	if strings.TrimSpace(input.RequestedRole) != "" {
		plan = resolveRequestedRole(plan, input)
		if plan.ActiveExecutionScope != nil {
			return plan
		}
		if len(plan.PendingConfirmations) > 0 {
			return plan
		}
	}

	active := ActiveGrants(state.Grants)
	for _, grant := range active {
		if validationOK(ctx, input.HostResolver, grant) {
			selected := grant.MarkUsed(input.TurnID, now)
			plan.ActiveExecutionScope = &selected
			plan.ModelSummary = modelSummaryForGrant(selected, plan.CandidateRoleBindings)
			return plan
		}
		grant.Status = GrantStatusSuspended
		plan.SuspendedGrants = append(plan.SuspendedGrants, grant)
	}
	if len(plan.SuspendedGrants) > 0 {
		plan.PendingConfirmations = append(plan.PendingConfirmations, PendingConfirmation{
			ID:     stableHash("special-input-memory.pending", map[string]any{"turnID": input.TurnID, "reason": "grant_suspended"}),
			Kind:   "target",
			Reason: "active_grant_revalidate_failed",
		})
	}
	return plan
}

func resolveRequestedRole(plan MemoryReadPlan, input MemoryReadPlanInput) MemoryReadPlan {
	role := compactToken(input.RequestedRole)
	env := compactToken(input.EnvironmentKey)
	cluster := compactToken(input.ClusterKey)
	var matches []MentionRoleBinding
	for _, binding := range plan.CandidateRoleBindings {
		if compactToken(binding.RoleKey) != role {
			continue
		}
		if cluster != "" && compactToken(binding.ClusterKey) != cluster {
			continue
		}
		if env != "" && compactToken(binding.EnvironmentKey) != env {
			continue
		}
		matches = append(matches, binding)
	}
	if len(matches) == 1 {
		binding := matches[0]
		grant := ExecutionScopeGrant{
			ID:             stableHash("special-input-memory.role-grant", binding.BindingHash),
			CanonicalKey:   binding.ResourceKind + ":" + binding.ResourceID,
			ResourceKind:   binding.ResourceKind,
			ResourceID:     binding.ResourceID,
			Display:        firstNonEmpty(binding.Display, binding.ResourceID),
			Scope:          ScopeCurrentTask,
			AllowedActions: []string{ActionExecLowRisk, ActionInspect, ActionRead},
			TrustLevel:     TrustLevelServerConfirmed,
			Source:         SourceStructuredSelection,
			Status:         GrantStatusActive,
			ValidationHash: binding.BindingHash,
			CreatedTurnID:  binding.LastSeenTurnID,
			LastUsedTurnID: strings.TrimSpace(input.TurnID),
			ExpiresAt:      input.Now.Add(hostGrantTTL),
			Weight:         1,
		}
		plan.ActiveExecutionScope = &grant
		plan.ModelSummary = modelSummaryForGrant(grant, plan.CandidateRoleBindings)
		return plan
	}
	if len(matches) > 1 {
		ids := make([]string, 0, len(matches))
		for _, match := range matches {
			ids = append(ids, match.ID)
		}
		sort.Strings(ids)
		plan.PendingConfirmations = append(plan.PendingConfirmations, PendingConfirmation{
			ID:             stableHash("special-input-memory.pending-role", map[string]any{"role": role, "env": env, "cluster": cluster, "ids": ids}),
			Kind:           "role_binding",
			Reason:         "role_binding_ambiguous",
			RoleKey:        role,
			EnvironmentKey: env,
			ClusterKey:     cluster,
			CandidateIDs:   ids,
		})
	}
	return plan
}

func activeVisibleFacts(facts []MentionFact) []MentionFact {
	var out []MentionFact
	for _, fact := range facts {
		switch normalizedFactStatus(fact.Status) {
		case FactStatusActive, FactStatusSuspended, FactStatusStale:
			out = append(out, fact)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return factSortKey(out[i]) < factSortKey(out[j]) })
	return out
}

func candidateFacts(facts []MentionFact) []MentionFact {
	var out []MentionFact
	for _, fact := range facts {
		if normalizedTrustLevel(fact.TrustLevel) == TrustLevelRawTyped && normalizedFactStatus(fact.Status) == FactStatusActive {
			out = append(out, fact)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return factSortKey(out[i]) < factSortKey(out[j]) })
	return out
}

func activeRoleBindings(bindings []MentionRoleBinding) []MentionRoleBinding {
	var out []MentionRoleBinding
	for _, binding := range bindings {
		if binding.Status == "" || binding.Status == RoleBindingStatusActive {
			out = append(out, binding)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return roleBindingSortKey(out[i]) < roleBindingSortKey(out[j]) })
	return out
}

func validationOK(ctx context.Context, resolver HostResolver, grant ExecutionScopeGrant) bool {
	if resolver == nil || grant.ResourceKind != ResourceKindHost {
		return true
	}
	validation, err := resolver.RevalidateHost(ctx, HostRef{ResourceID: grant.ResourceID, CanonicalKey: grant.CanonicalKey})
	if err != nil {
		return false
	}
	return validation.Available
}

func modelSummaryForGrant(grant ExecutionScopeGrant, bindings []MentionRoleBinding) string {
	parts := []string{"active execution scope: " + firstNonEmpty(grant.Display, grant.ResourceID)}
	for _, binding := range bindings {
		if binding.ResourceID == grant.ResourceID {
			parts = append(parts, "role binding: "+strings.Join([]string{binding.EnvironmentKey, binding.ClusterKey, binding.RoleKey, binding.ResourceID}, "/"))
		}
	}
	return strings.Join(parts, "\n")
}
