package specialinputmemory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"sort"
	"strings"
	"time"
)

const SchemaVersion = "aiops.special_input_memory.v1"

const (
	FactKindHost       = "host"
	FactKindCapability = "capability"
	FactKindOpsManual  = "ops_manual"
	FactKindOpsGraph   = "ops_graph"
	FactKindWorkflow   = "workflow"
	FactKindFile       = "file"
	FactKindUnknown    = "unknown"
)

const (
	ResourceKindHost       = "host"
	ResourceKindCapability = "capability"
	ResourceKindOpsManual  = "ops_manual"
	ResourceKindOpsGraph   = "ops_graph"
	ResourceKindWorkflow   = "workflow"
	ResourceKindFile       = "file"
)

const (
	SourceStructuredSelection = "structured_selection"
	SourceTypedFallback       = "typed_fallback"
	SourceCorrection          = "correction"
	SourceRestored            = "restored"
	SourceSystem              = "system"
	SourceUserConfirmation    = "user_confirmation"
)

const (
	TrustLevelServerConfirmed = "server_confirmed"
	TrustLevelClientSelected  = "client_selected"
	TrustLevelRawTyped        = "raw_typed"
	TrustLevelRestored        = "restored"
)

const (
	FactStatusActive    = "active"
	FactStatusStale     = "stale"
	FactStatusConflict  = "conflict"
	FactStatusRevoked   = "revoked"
	FactStatusExpired   = "expired"
	FactStatusInvalid   = "invalid"
	FactStatusPolluted  = "polluted"
	FactStatusSuspended = "suspended"
)

const (
	GrantStatusActive    = "active"
	GrantStatusStale     = "stale"
	GrantStatusRevoked   = "revoked"
	GrantStatusExpired   = "expired"
	GrantStatusSuspended = "suspended"
	GrantStatusInvalid   = "invalid"
)

const (
	RoleBindingStatusActive   = "active"
	RoleBindingStatusConflict = "conflict"
	RoleBindingStatusRevoked  = "revoked"
	RoleBindingStatusExpired  = "expired"
)

const (
	ScopeSession     = "session"
	ScopeCurrentTask = "current_task"
	ScopeCurrentTurn = "current_turn"
)

const (
	ActionInspect     = "inspect"
	ActionRead        = "read"
	ActionExecLowRisk = "exec_low_risk"
	ActionMutate      = "mutate"
	ActionDestructive = "destructive"
)

const (
	IntentNone       = ""
	IntentCorrection = "correction"
	IntentForget     = "forget"
	IntentConfirm    = "confirm"
)

type SessionSpecialInputState struct {
	SchemaVersion     string                `json:"schemaVersion"`
	SessionID         string                `json:"sessionId,omitempty"`
	TaskID            string                `json:"taskId,omitempty"`
	Facts             []MentionFact         `json:"facts,omitempty"`
	Grants            []ExecutionScopeGrant `json:"grants,omitempty"`
	RoleBindings      []MentionRoleBinding  `json:"roleBindings,omitempty"`
	Conflicts         []MemoryConflict      `json:"conflicts,omitempty"`
	Tombstones        []MemoryTombstone     `json:"tombstones,omitempty"`
	Focus             *SpecialInputFocus    `json:"focus,omitempty"`
	LastUpdatedTurnID string                `json:"lastUpdatedTurnId,omitempty"`
	UpdatedAt         time.Time             `json:"updatedAt,omitempty"`
	Metadata          map[string]string     `json:"metadata,omitempty"`
}

type SpecialInputFocus struct {
	ActiveGrantID       string `json:"activeGrantId,omitempty"`
	ActiveFactID        string `json:"activeFactId,omitempty"`
	EnvironmentKey      string `json:"environmentKey,omitempty"`
	ClusterKey          string `json:"clusterKey,omitempty"`
	LastExplicitTurnID  string `json:"lastExplicitTurnId,omitempty"`
	LastUsedGrantID     string `json:"lastUsedGrantId,omitempty"`
	PendingConfirmation string `json:"pendingConfirmation,omitempty"`
}

type MentionFact struct {
	ID               string            `json:"id,omitempty"`
	Kind             string            `json:"kind,omitempty"`
	CanonicalKey     string            `json:"canonicalKey,omitempty"`
	Display          string            `json:"display,omitempty"`
	RawText          string            `json:"rawText,omitempty"`
	Path             string            `json:"path,omitempty"`
	Source           string            `json:"source,omitempty"`
	TrustLevel       string            `json:"trustLevel,omitempty"`
	Status           string            `json:"status,omitempty"`
	Scope            string            `json:"scope,omitempty"`
	ResourceKind     string            `json:"resourceKind,omitempty"`
	ResourceID       string            `json:"resourceId,omitempty"`
	EnvironmentKey   string            `json:"environmentKey,omitempty"`
	ClusterKey       string            `json:"clusterKey,omitempty"`
	FirstSeenTurnID  string            `json:"firstSeenTurnId,omitempty"`
	LastSeenTurnID   string            `json:"lastSeenTurnId,omitempty"`
	LastUsedTurnID   string            `json:"lastUsedTurnId,omitempty"`
	ExpiresAt        time.Time         `json:"expiresAt,omitempty"`
	ValidationHash   string            `json:"validationHash,omitempty"`
	ValidationSource string            `json:"validationSource,omitempty"`
	RedactedPayload  map[string]string `json:"redactedPayload,omitempty"`
	Weight           float64           `json:"weight,omitempty"`
}

type ExecutionScopeGrant struct {
	ID             string    `json:"id,omitempty"`
	FactID         string    `json:"factId,omitempty"`
	CanonicalKey   string    `json:"canonicalKey,omitempty"`
	ResourceKind   string    `json:"resourceKind,omitempty"`
	ResourceID     string    `json:"resourceId,omitempty"`
	Display        string    `json:"display,omitempty"`
	Scope          string    `json:"scope,omitempty"`
	AllowedActions []string  `json:"allowedActions,omitempty"`
	TrustLevel     string    `json:"trustLevel,omitempty"`
	Source         string    `json:"source,omitempty"`
	Status         string    `json:"status,omitempty"`
	ValidationHash string    `json:"validationHash,omitempty"`
	CreatedTurnID  string    `json:"createdTurnId,omitempty"`
	LastUsedTurnID string    `json:"lastUsedTurnId,omitempty"`
	ExpiresAt      time.Time `json:"expiresAt,omitempty"`
	RevokedReason  string    `json:"revokedReason,omitempty"`
	Weight         float64   `json:"weight,omitempty"`
}

type MentionRoleBinding struct {
	ID              string    `json:"id,omitempty"`
	RoleKey         string    `json:"roleKey,omitempty"`
	RuntimeName     string    `json:"runtimeName,omitempty"`
	ResourceID      string    `json:"resourceId,omitempty"`
	ResourceKind    string    `json:"resourceKind,omitempty"`
	Display         string    `json:"display,omitempty"`
	EnvironmentKey  string    `json:"environmentKey,omitempty"`
	ClusterKey      string    `json:"clusterKey,omitempty"`
	Source          string    `json:"source,omitempty"`
	Status          string    `json:"status,omitempty"`
	BindingHash     string    `json:"bindingHash,omitempty"`
	FirstSeenTurnID string    `json:"firstSeenTurnId,omitempty"`
	LastSeenTurnID  string    `json:"lastSeenTurnId,omitempty"`
	Confidence      float64   `json:"confidence,omitempty"`
	ExpiresAt       time.Time `json:"expiresAt,omitempty"`
}

type MemoryConflict struct {
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

type MemoryTombstone struct {
	ID           string    `json:"id,omitempty"`
	Kind         string    `json:"kind,omitempty"`
	CanonicalKey string    `json:"canonicalKey,omitempty"`
	ResourceID   string    `json:"resourceId,omitempty"`
	Reason       string    `json:"reason,omitempty"`
	SourceTurnID string    `json:"sourceTurnId,omitempty"`
	CreatedAt    time.Time `json:"createdAt,omitempty"`
	ExpiresAt    time.Time `json:"expiresAt,omitempty"`
}

type SpecialInputMemoryEvent struct {
	ID            string    `json:"id,omitempty"`
	Type          string    `json:"type,omitempty"`
	FactID        string    `json:"factId,omitempty"`
	GrantID       string    `json:"grantId,omitempty"`
	RoleBindingID string    `json:"roleBindingId,omitempty"`
	CanonicalKey  string    `json:"canonicalKey,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	TurnID        string    `json:"turnId,omitempty"`
	CreatedAt     time.Time `json:"createdAt,omitempty"`
}

type SpecialInputMemorySnapshot struct {
	SchemaVersion string                `json:"schemaVersion"`
	SessionID     string                `json:"sessionId,omitempty"`
	TaskID        string                `json:"taskId,omitempty"`
	Facts         []MentionFact         `json:"facts,omitempty"`
	Grants        []ExecutionScopeGrant `json:"grants,omitempty"`
	RoleBindings  []MentionRoleBinding  `json:"roleBindings,omitempty"`
	Conflicts     []MemoryConflict      `json:"conflicts,omitempty"`
	Tombstones    []MemoryTombstone     `json:"tombstones,omitempty"`
	Focus         *SpecialInputFocus    `json:"focus,omitempty"`
	UpdatedAt     time.Time             `json:"updatedAt,omitempty"`
}

type MentionObservation struct {
	Kind           string
	CanonicalKey   string
	Display        string
	RawText        string
	Path           string
	Source         string
	TrustLevel     string
	ResourceKind   string
	ResourceID     string
	EnvironmentKey string
	ClusterKey     string
	RoleKey        string
	RuntimeName    string
	AllowedActions []string
	ValidationHash string
}

type UserSpecialInputIntent struct {
	Kind         string
	TargetKind   string
	CanonicalKey string
	Reason       string
}

func (s SessionSpecialInputState) Normalize(sessionID, taskID string, now time.Time) SessionSpecialInputState {
	next := s.Clone()
	if strings.TrimSpace(next.SchemaVersion) == "" {
		next.SchemaVersion = SchemaVersion
	}
	if strings.TrimSpace(next.SessionID) == "" {
		next.SessionID = strings.TrimSpace(sessionID)
	}
	if strings.TrimSpace(next.TaskID) == "" {
		next.TaskID = strings.TrimSpace(taskID)
	}
	if next.UpdatedAt.IsZero() {
		next.UpdatedAt = now
	}
	return next
}

func (s SessionSpecialInputState) Clone() SessionSpecialInputState {
	next := s
	next.Facts = append([]MentionFact(nil), s.Facts...)
	next.Grants = append([]ExecutionScopeGrant(nil), s.Grants...)
	next.RoleBindings = append([]MentionRoleBinding(nil), s.RoleBindings...)
	next.Conflicts = append([]MemoryConflict(nil), s.Conflicts...)
	next.Tombstones = append([]MemoryTombstone(nil), s.Tombstones...)
	if s.Focus != nil {
		focus := *s.Focus
		next.Focus = &focus
	}
	if s.Metadata != nil {
		next.Metadata = map[string]string{}
		for k, v := range s.Metadata {
			next.Metadata[k] = v
		}
	}
	for i := range next.Facts {
		next.Facts[i].RedactedPayload = sanitizedPayload(next.Facts[i].RedactedPayload)
	}
	return next
}

func (s SessionSpecialInputState) Snapshot() SpecialInputMemorySnapshot {
	normalized := s.Normalize(s.SessionID, s.TaskID, s.UpdatedAt)
	facts := append([]MentionFact(nil), normalized.Facts...)
	for i := range facts {
		facts[i].RedactedPayload = sanitizedPayload(facts[i].RedactedPayload)
	}
	sort.SliceStable(facts, func(i, j int) bool {
		return factSortKey(facts[i]) < factSortKey(facts[j])
	})
	grants := append([]ExecutionScopeGrant(nil), normalized.Grants...)
	sort.SliceStable(grants, func(i, j int) bool {
		return grantSortKey(grants[i]) < grantSortKey(grants[j])
	})
	roleBindings := append([]MentionRoleBinding(nil), normalized.RoleBindings...)
	sort.SliceStable(roleBindings, func(i, j int) bool {
		return roleBindingSortKey(roleBindings[i]) < roleBindingSortKey(roleBindings[j])
	})
	conflicts := append([]MemoryConflict(nil), normalized.Conflicts...)
	sort.SliceStable(conflicts, func(i, j int) bool {
		return conflicts[i].ID < conflicts[j].ID
	})
	tombstones := append([]MemoryTombstone(nil), normalized.Tombstones...)
	sort.SliceStable(tombstones, func(i, j int) bool {
		return tombstones[i].ID < tombstones[j].ID
	})
	return SpecialInputMemorySnapshot{
		SchemaVersion: normalized.SchemaVersion,
		SessionID:     normalized.SessionID,
		TaskID:        normalized.TaskID,
		Facts:         facts,
		Grants:        grants,
		RoleBindings:  roleBindings,
		Conflicts:     conflicts,
		Tombstones:    tombstones,
		Focus:         normalized.Focus,
		UpdatedAt:     normalized.UpdatedAt,
	}
}

func stableHash(namespace string, payload any) string {
	raw, _ := json.Marshal(payload)
	sum := sha256.Sum256(append([]byte(namespace+":"), raw...))
	return hex.EncodeToString(sum[:])[:24]
}

func compactToken(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func compactDisplay(value string) string {
	return strings.TrimSpace(value)
}

func normalizedScope(scope string) string {
	switch compactToken(scope) {
	case ScopeSession:
		return ScopeSession
	case ScopeCurrentTurn:
		return ScopeCurrentTurn
	default:
		return ScopeCurrentTask
	}
}

func normalizedFactStatus(status string) string {
	switch compactToken(status) {
	case FactStatusStale, FactStatusConflict, FactStatusRevoked, FactStatusExpired, FactStatusInvalid, FactStatusPolluted, FactStatusSuspended:
		return compactToken(status)
	default:
		return FactStatusActive
	}
}

func normalizedGrantStatus(status string) string {
	switch compactToken(status) {
	case GrantStatusStale, GrantStatusRevoked, GrantStatusExpired, GrantStatusSuspended, GrantStatusInvalid:
		return compactToken(status)
	default:
		return GrantStatusActive
	}
}

func normalizedTrustLevel(level string) string {
	switch compactToken(level) {
	case TrustLevelServerConfirmed, TrustLevelClientSelected, TrustLevelRawTyped, TrustLevelRestored:
		return compactToken(level)
	default:
		return TrustLevelRawTyped
	}
}

func normalizedSource(source string) string {
	switch compactToken(source) {
	case SourceStructuredSelection, SourceTypedFallback, SourceCorrection, SourceRestored, SourceSystem, SourceUserConfirmation:
		return compactToken(source)
	default:
		return strings.TrimSpace(source)
	}
}

func factSortKey(f MentionFact) string {
	return strings.Join([]string{f.CanonicalKey, f.Kind, f.ID}, "\x00")
}

func grantSortKey(g ExecutionScopeGrant) string {
	return strings.Join([]string{g.CanonicalKey, g.ResourceKind, g.ResourceID, g.ID}, "\x00")
}

func roleBindingSortKey(b MentionRoleBinding) string {
	return strings.Join([]string{b.EnvironmentKey, b.ClusterKey, b.RoleKey, b.ResourceKind, b.ResourceID, b.RuntimeName, b.ID}, "\x00")
}

func sanitizedPayload(payload map[string]string) map[string]string {
	if len(payload) == 0 {
		return nil
	}
	out := map[string]string{}
	for key, value := range payload {
		lowerKey := strings.ToLower(strings.TrimSpace(key))
		lowerValue := strings.ToLower(value)
		if strings.Contains(lowerKey, "secret") ||
			strings.Contains(lowerKey, "token") ||
			strings.Contains(lowerKey, "password") ||
			strings.Contains(lowerKey, "authorization") ||
			strings.Contains(lowerKey, "apikey") ||
			strings.Contains(lowerKey, "api_key") ||
			strings.Contains(lowerValue, "secret") {
			continue
		}
		out[strings.TrimSpace(key)] = value
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func appendEvent(events []SpecialInputMemoryEvent, event SpecialInputMemoryEvent, turnID string, now time.Time) []SpecialInputMemoryEvent {
	event.TurnID = strings.TrimSpace(event.TurnID)
	if event.TurnID == "" {
		event.TurnID = strings.TrimSpace(turnID)
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = now
	}
	if event.ID == "" {
		event.ID = stableHash("special-input-memory.event", map[string]any{
			"type":          event.Type,
			"factID":        event.FactID,
			"grantID":       event.GrantID,
			"roleBindingID": event.RoleBindingID,
			"canonicalKey":  event.CanonicalKey,
			"reason":        event.Reason,
			"turnID":        event.TurnID,
			"createdAt":     event.CreatedAt.UnixNano(),
		})
	}
	return append(events, event)
}
