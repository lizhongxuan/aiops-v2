package specialinputmemory

type SpecialInputWorldStateSection struct {
	SchemaVersion        string                          `json:"schemaVersion"`
	TurnID               string                          `json:"turnId,omitempty"`
	ActiveExecutionScope *ExecutionScopeGrantTrace       `json:"activeExecutionScope,omitempty"`
	ActiveRoleBindings   []RoleBindingTrace              `json:"activeRoleBindings,omitempty"`
	SuspendedGrants      []ExecutionScopeGrantTrace      `json:"suspendedGrants,omitempty"`
	Conflicts            []MemoryConflictTrace           `json:"conflicts,omitempty"`
	PendingConfirmations []PendingConfirmation           `json:"pendingConfirmations,omitempty"`
	MemorySnapshot       *SpecialInputMemorySnapshotItem `json:"memorySnapshot,omitempty"`
	ReadPlan             *MemoryReadPlanTrace            `json:"readPlan,omitempty"`
	Events               []SpecialInputMemoryEvent       `json:"events,omitempty"`
	ModelSummary         string                          `json:"modelSummary,omitempty"`
}

type SpecialInputMemorySnapshotItem struct {
	SchemaVersion            string `json:"schemaVersion"`
	VisibleFactCount         int    `json:"visibleFactCount,omitempty"`
	CandidateFactCount       int    `json:"candidateFactCount,omitempty"`
	ActiveRoleBindingCount   int    `json:"activeRoleBindingCount,omitempty"`
	ConflictCount            int    `json:"conflictCount,omitempty"`
	PendingConfirmationCount int    `json:"pendingConfirmationCount,omitempty"`
	ActiveGrantID            string `json:"activeGrantId,omitempty"`
	ActiveResourceKind       string `json:"activeResourceKind,omitempty"`
	ActiveResourceID         string `json:"activeResourceId,omitempty"`
}

type MemoryReadPlanTrace struct {
	SchemaVersion          string   `json:"schemaVersion"`
	TurnID                 string   `json:"turnId,omitempty"`
	ActiveGrantID          string   `json:"activeGrantId,omitempty"`
	ActiveResourceKind     string   `json:"activeResourceKind,omitempty"`
	ActiveResourceID       string   `json:"activeResourceId,omitempty"`
	AllowedActions         []string `json:"allowedActions,omitempty"`
	VisibleFactIDs         []string `json:"visibleFactIds,omitempty"`
	CandidateFactIDs       []string `json:"candidateFactIds,omitempty"`
	RoleBindingHashes      []string `json:"roleBindingHashes,omitempty"`
	ConflictIDs            []string `json:"conflictIds,omitempty"`
	PendingConfirmationIDs []string `json:"pendingConfirmationIds,omitempty"`
	SuspendedGrantIDs      []string `json:"suspendedGrantIds,omitempty"`
}

type ExecutionScopeGrantTrace struct {
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
	ValidationHash string   `json:"validationHash,omitempty"`
	Source         string   `json:"source,omitempty"`
}

type RoleBindingTrace struct {
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

type MemoryConflictTrace struct {
	ID             string   `json:"id,omitempty"`
	Kind           string   `json:"kind,omitempty"`
	CanonicalKey   string   `json:"canonicalKey,omitempty"`
	RoleKey        string   `json:"roleKey,omitempty"`
	EnvironmentKey string   `json:"environmentKey,omitempty"`
	ClusterKey     string   `json:"clusterKey,omitempty"`
	ResourceIDs    []string `json:"resourceIds,omitempty"`
	Reasons        []string `json:"reasons,omitempty"`
	TraceHash      string   `json:"traceHash,omitempty"`
}

func BuildWorldStateSection(plan MemoryReadPlan) *SpecialInputWorldStateSection {
	if MemoryReadPlanTraceEmpty(plan) {
		return nil
	}
	section := &SpecialInputWorldStateSection{
		SchemaVersion:        SchemaVersion,
		TurnID:               plan.TurnID,
		ActiveRoleBindings:   roleBindingTraces(plan.CandidateRoleBindings),
		SuspendedGrants:      executionScopeGrantTraces(plan.SuspendedGrants),
		Conflicts:            memoryConflictTraces(plan.Conflicts),
		PendingConfirmations: append([]PendingConfirmation(nil), plan.PendingConfirmations...),
		ModelSummary:         plan.ModelSummary,
		ReadPlan:             buildMemoryReadPlanTrace(plan),
		MemorySnapshot:       buildSpecialInputMemorySnapshotItem(plan),
	}
	if plan.ActiveExecutionScope != nil {
		grant := executionScopeGrantTrace(*plan.ActiveExecutionScope)
		section.ActiveExecutionScope = &grant
	}
	return section
}

func MemoryReadPlanTraceEmpty(plan MemoryReadPlan) bool {
	return plan.ActiveExecutionScope == nil &&
		len(plan.VisibleFacts) == 0 &&
		len(plan.CandidateFacts) == 0 &&
		len(plan.SuspendedGrants) == 0 &&
		len(plan.CandidateRoleBindings) == 0 &&
		len(plan.Conflicts) == 0 &&
		len(plan.PendingConfirmations) == 0 &&
		plan.ModelSummary == ""
}

func WorldStateSectionEmpty(section *SpecialInputWorldStateSection) bool {
	return section == nil ||
		(section.ActiveExecutionScope == nil &&
			len(section.ActiveRoleBindings) == 0 &&
			len(section.SuspendedGrants) == 0 &&
			len(section.Conflicts) == 0 &&
			len(section.PendingConfirmations) == 0 &&
			section.MemorySnapshot == nil &&
			section.ReadPlan == nil &&
			len(section.Events) == 0 &&
			section.ModelSummary == "")
}

func CloneWorldStateSection(section *SpecialInputWorldStateSection) *SpecialInputWorldStateSection {
	if section == nil {
		return nil
	}
	out := &SpecialInputWorldStateSection{
		SchemaVersion:        section.SchemaVersion,
		TurnID:               section.TurnID,
		ActiveRoleBindings:   append([]RoleBindingTrace(nil), section.ActiveRoleBindings...),
		SuspendedGrants:      append([]ExecutionScopeGrantTrace(nil), section.SuspendedGrants...),
		Conflicts:            cloneMemoryConflictTraces(section.Conflicts),
		PendingConfirmations: append([]PendingConfirmation(nil), section.PendingConfirmations...),
		Events:               append([]SpecialInputMemoryEvent(nil), section.Events...),
		ModelSummary:         section.ModelSummary,
	}
	if section.ActiveExecutionScope != nil {
		grant := *section.ActiveExecutionScope
		grant.AllowedActions = append([]string(nil), section.ActiveExecutionScope.AllowedActions...)
		out.ActiveExecutionScope = &grant
	}
	if section.MemorySnapshot != nil {
		snapshot := *section.MemorySnapshot
		out.MemorySnapshot = &snapshot
	}
	if section.ReadPlan != nil {
		readPlan := *section.ReadPlan
		readPlan.AllowedActions = append([]string(nil), section.ReadPlan.AllowedActions...)
		readPlan.VisibleFactIDs = append([]string(nil), section.ReadPlan.VisibleFactIDs...)
		readPlan.CandidateFactIDs = append([]string(nil), section.ReadPlan.CandidateFactIDs...)
		readPlan.RoleBindingHashes = append([]string(nil), section.ReadPlan.RoleBindingHashes...)
		readPlan.ConflictIDs = append([]string(nil), section.ReadPlan.ConflictIDs...)
		readPlan.PendingConfirmationIDs = append([]string(nil), section.ReadPlan.PendingConfirmationIDs...)
		readPlan.SuspendedGrantIDs = append([]string(nil), section.ReadPlan.SuspendedGrantIDs...)
		out.ReadPlan = &readPlan
	}
	return out
}

func buildSpecialInputMemorySnapshotItem(plan MemoryReadPlan) *SpecialInputMemorySnapshotItem {
	snapshot := &SpecialInputMemorySnapshotItem{
		SchemaVersion:            SchemaVersion,
		VisibleFactCount:         len(plan.VisibleFacts),
		CandidateFactCount:       len(plan.CandidateFacts),
		ActiveRoleBindingCount:   len(plan.CandidateRoleBindings),
		ConflictCount:            len(plan.Conflicts),
		PendingConfirmationCount: len(plan.PendingConfirmations),
	}
	if plan.ActiveExecutionScope != nil {
		snapshot.ActiveGrantID = plan.ActiveExecutionScope.ID
		snapshot.ActiveResourceKind = plan.ActiveExecutionScope.ResourceKind
		snapshot.ActiveResourceID = plan.ActiveExecutionScope.ResourceID
	}
	return snapshot
}

func buildMemoryReadPlanTrace(plan MemoryReadPlan) *MemoryReadPlanTrace {
	trace := &MemoryReadPlanTrace{
		SchemaVersion:          SchemaVersion,
		TurnID:                 plan.TurnID,
		VisibleFactIDs:         factIDs(plan.VisibleFacts),
		CandidateFactIDs:       factIDs(plan.CandidateFacts),
		RoleBindingHashes:      roleBindingHashes(plan.CandidateRoleBindings),
		ConflictIDs:            conflictIDs(plan.Conflicts),
		PendingConfirmationIDs: pendingConfirmationIDs(plan.PendingConfirmations),
		SuspendedGrantIDs:      grantIDs(plan.SuspendedGrants),
	}
	if plan.ActiveExecutionScope != nil {
		trace.ActiveGrantID = plan.ActiveExecutionScope.ID
		trace.ActiveResourceKind = plan.ActiveExecutionScope.ResourceKind
		trace.ActiveResourceID = plan.ActiveExecutionScope.ResourceID
		trace.AllowedActions = append([]string(nil), plan.ActiveExecutionScope.AllowedActions...)
	}
	return trace
}

func executionScopeGrantTraces(grants []ExecutionScopeGrant) []ExecutionScopeGrantTrace {
	if len(grants) == 0 {
		return nil
	}
	out := make([]ExecutionScopeGrantTrace, 0, len(grants))
	for _, grant := range grants {
		out = append(out, executionScopeGrantTrace(grant))
	}
	return out
}

func executionScopeGrantTrace(grant ExecutionScopeGrant) ExecutionScopeGrantTrace {
	return ExecutionScopeGrantTrace{
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
		ValidationHash: grant.ValidationHash,
		Source:         grant.Source,
	}
}

func roleBindingTraces(bindings []MentionRoleBinding) []RoleBindingTrace {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]RoleBindingTrace, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, RoleBindingTrace{
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

func memoryConflictTraces(conflicts []MemoryConflict) []MemoryConflictTrace {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]MemoryConflictTrace, 0, len(conflicts))
	for _, conflict := range conflicts {
		out = append(out, MemoryConflictTrace{
			ID:             conflict.ID,
			Kind:           conflict.Kind,
			CanonicalKey:   conflict.CanonicalKey,
			RoleKey:        conflict.RoleKey,
			EnvironmentKey: conflict.EnvironmentKey,
			ClusterKey:     conflict.ClusterKey,
			ResourceIDs:    append([]string(nil), conflict.ResourceIDs...),
			Reasons:        append([]string(nil), conflict.Reasons...),
			TraceHash:      conflict.TraceHash,
		})
	}
	return out
}

func cloneMemoryConflictTraces(conflicts []MemoryConflictTrace) []MemoryConflictTrace {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]MemoryConflictTrace, 0, len(conflicts))
	for _, conflict := range conflicts {
		conflict.ResourceIDs = append([]string(nil), conflict.ResourceIDs...)
		conflict.Reasons = append([]string(nil), conflict.Reasons...)
		out = append(out, conflict)
	}
	return out
}

func factIDs(facts []MentionFact) []string {
	if len(facts) == 0 {
		return nil
	}
	out := make([]string, 0, len(facts))
	for _, fact := range facts {
		if fact.ID != "" {
			out = append(out, fact.ID)
		}
	}
	return out
}

func grantIDs(grants []ExecutionScopeGrant) []string {
	if len(grants) == 0 {
		return nil
	}
	out := make([]string, 0, len(grants))
	for _, grant := range grants {
		if grant.ID != "" {
			out = append(out, grant.ID)
		}
	}
	return out
}

func roleBindingHashes(bindings []MentionRoleBinding) []string {
	if len(bindings) == 0 {
		return nil
	}
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		if binding.BindingHash != "" {
			out = append(out, binding.BindingHash)
		}
	}
	return out
}

func conflictIDs(conflicts []MemoryConflict) []string {
	if len(conflicts) == 0 {
		return nil
	}
	out := make([]string, 0, len(conflicts))
	for _, conflict := range conflicts {
		if conflict.ID != "" {
			out = append(out, conflict.ID)
		}
	}
	return out
}

func pendingConfirmationIDs(pending []PendingConfirmation) []string {
	if len(pending) == 0 {
		return nil
	}
	out := make([]string, 0, len(pending))
	for _, confirmation := range pending {
		if confirmation.ID != "" {
			out = append(out, confirmation.ID)
		}
	}
	return out
}
