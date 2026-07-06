package specialinputmemory

type TransportContext struct {
	SchemaVersion        string                 `json:"schemaVersion"`
	TurnID               string                 `json:"turnId,omitempty"`
	ActiveGrant          *TransportGrant        `json:"activeGrant,omitempty"`
	VisibleFacts         []TransportFact        `json:"visibleFacts,omitempty"`
	CandidateFacts       []TransportFact        `json:"candidateFacts,omitempty"`
	SuspendedGrants      []TransportGrant       `json:"suspendedGrants,omitempty"`
	RoleBindings         []TransportRoleBinding `json:"roleBindings,omitempty"`
	Conflicts            []TransportConflict    `json:"conflicts,omitempty"`
	PendingConfirmations []PendingConfirmation  `json:"pendingConfirmations,omitempty"`
	ModelSummary         string                 `json:"modelSummary,omitempty"`
}

type TransportFact struct {
	ID             string `json:"id,omitempty"`
	Kind           string `json:"kind,omitempty"`
	ResourceKind   string `json:"resourceKind,omitempty"`
	ResourceID     string `json:"resourceId,omitempty"`
	CanonicalKey   string `json:"canonicalKey,omitempty"`
	Display        string `json:"display,omitempty"`
	TrustLevel     string `json:"trustLevel,omitempty"`
	Status         string `json:"status,omitempty"`
	EnvironmentKey string `json:"environmentKey,omitempty"`
	ClusterKey     string `json:"clusterKey,omitempty"`
}

type TransportGrant struct {
	ID             string   `json:"id,omitempty"`
	FactID         string   `json:"factId,omitempty"`
	ResourceKind   string   `json:"resourceKind,omitempty"`
	ResourceID     string   `json:"resourceId,omitempty"`
	CanonicalKey   string   `json:"canonicalKey,omitempty"`
	Display        string   `json:"display,omitempty"`
	AllowedActions []string `json:"allowedActions,omitempty"`
	TrustLevel     string   `json:"trustLevel,omitempty"`
	Status         string   `json:"status,omitempty"`
	Scope          string   `json:"scope,omitempty"`
}

type TransportRoleBinding struct {
	ID             string  `json:"id,omitempty"`
	RoleKey        string  `json:"roleKey,omitempty"`
	RuntimeName    string  `json:"runtimeName,omitempty"`
	ResourceKind   string  `json:"resourceKind,omitempty"`
	ResourceID     string  `json:"resourceId,omitempty"`
	Display        string  `json:"display,omitempty"`
	EnvironmentKey string  `json:"environmentKey,omitempty"`
	ClusterKey     string  `json:"clusterKey,omitempty"`
	BindingHash    string  `json:"bindingHash,omitempty"`
	Status         string  `json:"status,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
}

type TransportConflict struct {
	ID             string   `json:"id,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	CanonicalKey   string   `json:"canonicalKey,omitempty"`
	RoleKey        string   `json:"roleKey,omitempty"`
	EnvironmentKey string   `json:"environmentKey,omitempty"`
	ClusterKey     string   `json:"clusterKey,omitempty"`
	ResourceIDs    []string `json:"resourceIds,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
}

func ProjectTransportContext(plan MemoryReadPlan) *TransportContext {
	if !memoryReadPlanHasProjection(plan) {
		return nil
	}
	projected := &TransportContext{
		SchemaVersion:        SchemaVersion,
		TurnID:               plan.TurnID,
		VisibleFacts:         projectTransportFacts(plan.VisibleFacts),
		CandidateFacts:       projectTransportFacts(plan.CandidateFacts),
		SuspendedGrants:      projectTransportGrants(plan.SuspendedGrants),
		RoleBindings:         projectTransportRoleBindings(plan.CandidateRoleBindings),
		Conflicts:            projectTransportConflicts(plan.Conflicts),
		PendingConfirmations: append([]PendingConfirmation(nil), plan.PendingConfirmations...),
		ModelSummary:         plan.ModelSummary,
	}
	if plan.ActiveExecutionScope != nil {
		grant := projectTransportGrant(*plan.ActiveExecutionScope)
		projected.ActiveGrant = &grant
	}
	return projected
}

func memoryReadPlanHasProjection(plan MemoryReadPlan) bool {
	return plan.ActiveExecutionScope != nil ||
		len(plan.VisibleFacts) > 0 ||
		len(plan.CandidateFacts) > 0 ||
		len(plan.SuspendedGrants) > 0 ||
		len(plan.CandidateRoleBindings) > 0 ||
		len(plan.Conflicts) > 0 ||
		len(plan.PendingConfirmations) > 0 ||
		plan.ModelSummary != ""
}

func projectTransportFacts(facts []MentionFact) []TransportFact {
	if len(facts) == 0 {
		return nil
	}
	out := make([]TransportFact, 0, len(facts))
	for _, fact := range facts {
		out = append(out, TransportFact{
			ID:             fact.ID,
			Kind:           fact.Kind,
			ResourceKind:   fact.ResourceKind,
			ResourceID:     fact.ResourceID,
			CanonicalKey:   fact.CanonicalKey,
			Display:        fact.Display,
			TrustLevel:     fact.TrustLevel,
			Status:         fact.Status,
			EnvironmentKey: fact.EnvironmentKey,
			ClusterKey:     fact.ClusterKey,
		})
	}
	return out
}

func projectTransportGrants(grants []ExecutionScopeGrant) []TransportGrant {
	if len(grants) == 0 {
		return nil
	}
	out := make([]TransportGrant, 0, len(grants))
	for _, grant := range grants {
		out = append(out, projectTransportGrant(grant))
	}
	return out
}

func projectTransportGrant(grant ExecutionScopeGrant) TransportGrant {
	return TransportGrant{
		ID:             grant.ID,
		FactID:         grant.FactID,
		ResourceKind:   grant.ResourceKind,
		ResourceID:     grant.ResourceID,
		CanonicalKey:   grant.CanonicalKey,
		Display:        grant.Display,
		AllowedActions: append([]string(nil), grant.AllowedActions...),
		TrustLevel:     grant.TrustLevel,
		Status:         grant.Status,
		Scope:          grant.Scope,
	}
}

func projectTransportRoleBindings(bindings []MentionRoleBinding) []TransportRoleBinding {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]TransportRoleBinding, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, TransportRoleBinding{
			ID:             binding.ID,
			RoleKey:        binding.RoleKey,
			RuntimeName:    binding.RuntimeName,
			ResourceKind:   binding.ResourceKind,
			ResourceID:     binding.ResourceID,
			Display:        binding.Display,
			EnvironmentKey: binding.EnvironmentKey,
			ClusterKey:     binding.ClusterKey,
			BindingHash:    binding.BindingHash,
			Status:         binding.Status,
			Confidence:     binding.Confidence,
		})
	}
	return out
}

func projectTransportConflicts(conflicts []MemoryConflict) []TransportConflict {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]TransportConflict, 0, len(conflicts))
	for _, conflict := range conflicts {
		out = append(out, TransportConflict{
			ID:             conflict.ID,
			Kind:           conflict.Kind,
			CanonicalKey:   conflict.CanonicalKey,
			RoleKey:        conflict.RoleKey,
			EnvironmentKey: conflict.EnvironmentKey,
			ClusterKey:     conflict.ClusterKey,
			ResourceIDs:    append([]string(nil), conflict.ResourceIDs...),
			Reasons:        append([]string(nil), conflict.Reasons...),
		})
	}
	return out
}
