package resourcebinding

import "strings"

const (
	ResourceTypeHost     = "host"
	ResourceTypeService  = "service"
	ResourceTypeDatabase = "database"
	ResourceTypePG       = "pg"
	ResourceTypeIncident = "incident"
	ResourceTypeSession  = "session"

	SessionResourceID = "session"
)

const (
	BindingSourceMention       = "mention"
	BindingSourceSessionTarget = "session_target"
	BindingSourceWorkflow      = "workflow"
	BindingSourceRouteMetadata = "route_metadata"
)

const (
	TrustLevelVerified = "verified"
	TrustLevelWeak     = "weak"
	TrustLevelRejected = "rejected"
)

const (
	CapabilityRead    = "read"
	CapabilityInspect = "inspect"
	CapabilityExec    = "exec"
	CapabilityMutate  = "mutate"
)

type ResourceRef struct {
	Type        string `json:"type,omitempty"`
	ID          string `json:"id,omitempty"`
	DisplayName string `json:"displayName,omitempty"`
	Namespace   string `json:"namespace,omitempty"`
	Provider    string `json:"provider,omitempty"`
}

func NormalizeRef(ref ResourceRef) ResourceRef {
	return ResourceRef{
		Type:        normalizeToken(ref.Type),
		ID:          strings.TrimSpace(ref.ID),
		DisplayName: strings.TrimSpace(ref.DisplayName),
		Namespace:   strings.TrimSpace(ref.Namespace),
		Provider:    normalizeToken(ref.Provider),
	}
}

func (r ResourceRef) IsZero() bool {
	normalized := NormalizeRef(r)
	return normalized.Type == "" && normalized.ID == "" && normalized.Namespace == "" && normalized.Provider == ""
}

func (r ResourceRef) IdentityHash() string {
	normalized := NormalizeRef(r)
	if normalized.Type == "" || normalized.ID == "" {
		return ""
	}
	return StableTraceHash("resource-ref.identity", map[string]string{
		"type":      normalized.Type,
		"id":        normalized.ID,
		"namespace": normalized.Namespace,
		"provider":  normalized.Provider,
	})
}

type ResourceBindingSnapshot struct {
	Ref        ResourceRef `json:"ref"`
	Source     string      `json:"source,omitempty"`
	VerifiedBy string      `json:"verifiedBy,omitempty"`
	TrustLevel string      `json:"trustLevel,omitempty"`
	FailClosed bool        `json:"failClosed,omitempty"`
	TraceHash  string      `json:"traceHash,omitempty"`
}

type BindingOptions struct {
	Source     string
	VerifiedBy string
	TrustLevel string
}

func NewBindingSnapshot(ref ResourceRef, options BindingOptions) ResourceBindingSnapshot {
	normalizedRef := NormalizeRef(ref)
	trustLevel := normalizeTrustLevel(options.TrustLevel)
	verifiedBy := strings.TrimSpace(options.VerifiedBy)
	if trustLevel == "" {
		if verifiedBy != "" {
			trustLevel = TrustLevelVerified
		} else if normalizedRef.ID != "" {
			trustLevel = TrustLevelWeak
		} else {
			trustLevel = TrustLevelRejected
		}
	}
	if trustLevel == TrustLevelVerified && verifiedBy == "" {
		trustLevel = TrustLevelRejected
	}
	failClosed := trustLevel != TrustLevelVerified
	binding := ResourceBindingSnapshot{
		Ref:        normalizedRef,
		Source:     normalizeBindingSource(options.Source),
		VerifiedBy: verifiedBy,
		TrustLevel: trustLevel,
		FailClosed: failClosed,
	}
	binding.TraceHash = StableTraceHash("resource-binding.snapshot", binding.tracePayload())
	return binding
}

func (b ResourceBindingSnapshot) Verified() bool {
	return NormalizeRef(b.Ref).IdentityHash() != "" &&
		strings.TrimSpace(b.VerifiedBy) != "" &&
		normalizeTrustLevel(b.TrustLevel) == TrustLevelVerified &&
		!b.FailClosed
}

func (b ResourceBindingSnapshot) tracePayload() any {
	return map[string]any{
		"ref":        NormalizeRef(b.Ref),
		"source":     strings.TrimSpace(b.Source),
		"verifiedBy": strings.TrimSpace(b.VerifiedBy),
		"trustLevel": normalizeTrustLevel(b.TrustLevel),
		"failClosed": b.FailClosed,
	}
}

type ResourceRoleBinding struct {
	BindingID      string      `json:"bindingId,omitempty"`
	ResourceRef    ResourceRef `json:"resourceRef"`
	Role           string      `json:"role,omitempty"`
	RoleAlias      []string    `json:"roleAlias,omitempty"`
	SourceTurnID   string      `json:"sourceTurnId,omitempty"`
	SourceSpan     string      `json:"sourceSpan,omitempty"`
	Confidence     float64     `json:"confidence,omitempty"`
	ConflictPolicy string      `json:"conflictPolicy,omitempty"`
	TraceHash      string      `json:"traceHash,omitempty"`
}

type RoleBindingInput struct {
	BindingID      string
	ResourceRef    ResourceRef
	Role           string
	RoleAlias      []string
	SourceTurnID   string
	SourceSpan     string
	Confidence     float64
	ConflictPolicy string
}

func NewRoleBinding(input RoleBindingInput) ResourceRoleBinding {
	binding := ResourceRoleBinding{
		BindingID:      strings.TrimSpace(input.BindingID),
		ResourceRef:    NormalizeRef(input.ResourceRef),
		Role:           normalizeToken(input.Role),
		RoleAlias:      uniqueSorted(input.RoleAlias),
		SourceTurnID:   strings.TrimSpace(input.SourceTurnID),
		SourceSpan:     strings.TrimSpace(input.SourceSpan),
		Confidence:     input.Confidence,
		ConflictPolicy: normalizeToken(input.ConflictPolicy),
	}
	if binding.BindingID == "" {
		binding.BindingID = StableTraceHash("resource-role-binding.id", map[string]any{
			"resource": binding.ResourceRef.IdentityHash(),
			"role":     binding.Role,
			"turn":     binding.SourceTurnID,
		})
	}
	binding.TraceHash = StableTraceHash("resource-role-binding", map[string]any{
		"bindingID":      binding.BindingID,
		"resource":       binding.ResourceRef,
		"role":           binding.Role,
		"roleAlias":      binding.RoleAlias,
		"sourceTurnID":   binding.SourceTurnID,
		"sourceSpan":     binding.SourceSpan,
		"confidence":     binding.Confidence,
		"conflictPolicy": binding.ConflictPolicy,
	})
	return binding
}

type ResourceCapability struct {
	ResourceRef      ResourceRef `json:"resourceRef"`
	Capability       string      `json:"capability,omitempty"`
	ToolNames        []string    `json:"toolNames,omitempty"`
	RequiresApproval bool        `json:"requiresApproval,omitempty"`
	PolicyHash       string      `json:"policyHash,omitempty"`
	BindingTraceHash string      `json:"bindingTraceHash,omitempty"`
	TraceHash        string      `json:"traceHash,omitempty"`
}

func (c ResourceCapability) Dispatchable() bool {
	capability := normalizeCapability(c.Capability)
	if NormalizeRef(c.ResourceRef).IdentityHash() == "" || capability == "" || len(c.ToolNames) == 0 {
		return false
	}
	if capability == CapabilityMutate {
		return c.RequiresApproval && strings.TrimSpace(c.PolicyHash) != ""
	}
	return true
}

type EvidenceRef struct {
	ID          string      `json:"id,omitempty"`
	ResourceRef ResourceRef `json:"resourceRef"`
	Source      string      `json:"source,omitempty"`
	Kind        string      `json:"kind,omitempty"`
	TraceHash   string      `json:"traceHash,omitempty"`
}

func normalizeToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeBindingSource(source string) string {
	switch normalizeToken(source) {
	case BindingSourceMention:
		return BindingSourceMention
	case BindingSourceSessionTarget:
		return BindingSourceSessionTarget
	case BindingSourceWorkflow:
		return BindingSourceWorkflow
	case BindingSourceRouteMetadata:
		return BindingSourceRouteMetadata
	default:
		return strings.TrimSpace(source)
	}
}

func normalizeTrustLevel(value string) string {
	switch normalizeToken(value) {
	case TrustLevelVerified:
		return TrustLevelVerified
	case TrustLevelWeak:
		return TrustLevelWeak
	case TrustLevelRejected:
		return TrustLevelRejected
	default:
		return ""
	}
}

func normalizeCapability(value string) string {
	switch normalizeToken(value) {
	case CapabilityRead:
		return CapabilityRead
	case CapabilityInspect:
		return CapabilityInspect
	case CapabilityExec:
		return CapabilityExec
	case CapabilityMutate:
		return CapabilityMutate
	default:
		return ""
	}
}
