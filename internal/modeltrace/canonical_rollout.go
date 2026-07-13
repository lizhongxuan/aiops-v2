package modeltrace

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"reflect"
	"sort"
	"strings"
)

const (
	CanonicalRolloutSchemaVersion = "aiops.canonical-rollout.v1"

	CanonicalRolloutKindAdmission           = "admission"
	CanonicalRolloutKindAssembly            = "assembly"
	CanonicalRolloutKindPrompt              = "prompt"
	CanonicalRolloutKindProviderRequest     = "provider_request"
	CanonicalRolloutKindProviderResponse    = "provider_response"
	CanonicalRolloutKindToolProposed        = "tool_proposed"
	CanonicalRolloutKindToolDispatched      = "tool_dispatched"
	CanonicalRolloutKindToolResult          = "tool_result"
	CanonicalRolloutKindApprovalRequested   = "approval_requested"
	CanonicalRolloutKindApprovalDecided     = "approval_decided"
	CanonicalRolloutKindCheckpoint          = "checkpoint"
	CanonicalRolloutKindFinalFacts          = "final_facts"
	CanonicalRolloutKindTransportProjection = "transport_projection"
	CanonicalRolloutKindRecorderDegraded    = "recorder_degraded"
)

var canonicalRolloutKinds = map[string]struct{}{
	CanonicalRolloutKindAdmission:           {},
	CanonicalRolloutKindAssembly:            {},
	CanonicalRolloutKindPrompt:              {},
	CanonicalRolloutKindProviderRequest:     {},
	CanonicalRolloutKindProviderResponse:    {},
	CanonicalRolloutKindToolProposed:        {},
	CanonicalRolloutKindToolDispatched:      {},
	CanonicalRolloutKindToolResult:          {},
	CanonicalRolloutKindApprovalRequested:   {},
	CanonicalRolloutKindApprovalDecided:     {},
	CanonicalRolloutKindCheckpoint:          {},
	CanonicalRolloutKindFinalFacts:          {},
	CanonicalRolloutKindTransportProjection: {},
	CanonicalRolloutKindRecorderDegraded:    {},
}

// CanonicalRolloutEvent is the immutable, append-only fact envelope shared by
// runtime recording, replay, evaluation, and diagnostics.
type CanonicalRolloutEvent struct {
	SchemaVersion    string         `json:"schemaVersion"`
	EventID          string         `json:"eventId"`
	Hash             string         `json:"hash"`
	Sequence         int64          `json:"sequence"`
	SessionID        string         `json:"sessionId"`
	TurnID           string         `json:"turnId"`
	StepID           string         `json:"stepId,omitempty"`
	Kind             string         `json:"kind"`
	TurnAssemblyHash string         `json:"turnAssemblyHash,omitempty"`
	StepContextHash  string         `json:"stepContextHash,omitempty"`
	SourceRefs       []string       `json:"sourceRefs,omitempty"`
	Payload          map[string]any `json:"payload,omitempty"`
}

// FreezeCanonicalRolloutEvent validates the event coordinates, canonicalizes
// references, recursively redacts sensitive payloads, deep-copies caller-owned
// data, and derives stable identity and integrity hashes.
func FreezeCanonicalRolloutEvent(event CanonicalRolloutEvent) (CanonicalRolloutEvent, error) {
	event.SchemaVersion = strings.TrimSpace(event.SchemaVersion)
	if event.SchemaVersion == "" {
		event.SchemaVersion = CanonicalRolloutSchemaVersion
	}
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.TurnID = strings.TrimSpace(event.TurnID)
	event.StepID = strings.TrimSpace(event.StepID)
	event.Kind = strings.TrimSpace(event.Kind)
	event.TurnAssemblyHash = strings.TrimSpace(event.TurnAssemblyHash)
	event.StepContextHash = strings.TrimSpace(event.StepContextHash)
	event.EventID = ""
	event.Hash = ""

	if err := validateCanonicalRolloutCoordinates(event); err != nil {
		return CanonicalRolloutEvent{}, err
	}
	event.SourceRefs = normalizeCanonicalSourceRefs(event.SourceRefs)

	payload, err := cloneAndRedactCanonicalPayload(event.Payload)
	if err != nil {
		return CanonicalRolloutEvent{}, err
	}
	event.Payload = payload

	event.EventID, err = canonicalRolloutEventID(event)
	if err != nil {
		return CanonicalRolloutEvent{}, err
	}
	event.Hash, err = canonicalRolloutEventHash(event)
	if err != nil {
		return CanonicalRolloutEvent{}, err
	}
	return event, nil
}

// Validate rejects unsupported schemas and kinds, non-canonical data, and any
// event whose stable identity or integrity hash no longer matches its facts.
func (event CanonicalRolloutEvent) Validate() error {
	if strings.TrimSpace(event.SchemaVersion) == "" {
		return fmt.Errorf("canonical rollout schema version is required")
	}
	if err := validateCanonicalRolloutCoordinates(event); err != nil {
		return err
	}

	expected, err := FreezeCanonicalRolloutEvent(event)
	if err != nil {
		return err
	}
	if event.SchemaVersion != expected.SchemaVersion ||
		event.SessionID != expected.SessionID ||
		event.TurnID != expected.TurnID ||
		event.StepID != expected.StepID ||
		event.Kind != expected.Kind ||
		event.TurnAssemblyHash != expected.TurnAssemblyHash ||
		event.StepContextHash != expected.StepContextHash ||
		!reflect.DeepEqual(event.SourceRefs, expected.SourceRefs) {
		return fmt.Errorf("canonical rollout event is not normalized")
	}
	equalPayload, err := canonicalJSONEqual(event.Payload, expected.Payload)
	if err != nil {
		return err
	}
	if !equalPayload {
		return fmt.Errorf("canonical rollout payload is not normalized or redacted")
	}
	if event.EventID == "" || event.EventID != expected.EventID {
		return fmt.Errorf("canonical rollout event identity mismatch")
	}
	if event.Hash == "" || event.Hash != expected.Hash {
		return fmt.Errorf("canonical rollout event hash mismatch")
	}
	return nil
}

func validateCanonicalRolloutCoordinates(event CanonicalRolloutEvent) error {
	if event.SchemaVersion != CanonicalRolloutSchemaVersion {
		return fmt.Errorf("unsupported canonical rollout schema version %q", event.SchemaVersion)
	}
	if event.Sequence <= 0 {
		return fmt.Errorf("canonical rollout sequence must be greater than zero")
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("canonical rollout session id is required")
	}
	if strings.TrimSpace(event.TurnID) == "" {
		return fmt.Errorf("canonical rollout turn id is required")
	}
	if strings.TrimSpace(event.Kind) == "" {
		return fmt.Errorf("canonical rollout kind is required")
	}
	if _, ok := canonicalRolloutKinds[event.Kind]; !ok {
		return fmt.Errorf("unsupported canonical rollout kind %q", event.Kind)
	}
	return nil
}

func canonicalRolloutEventID(event CanonicalRolloutEvent) (string, error) {
	identity := struct {
		SchemaVersion string `json:"schemaVersion"`
		Sequence      int64  `json:"sequence"`
		SessionID     string `json:"sessionId"`
		TurnID        string `json:"turnId"`
		StepID        string `json:"stepId,omitempty"`
		Kind          string `json:"kind"`
	}{
		SchemaVersion: event.SchemaVersion,
		Sequence:      event.Sequence,
		SessionID:     event.SessionID,
		TurnID:        event.TurnID,
		StepID:        event.StepID,
		Kind:          event.Kind,
	}
	digest, err := hashCanonicalJSON(identity)
	if err != nil {
		return "", err
	}
	return "event:" + strings.TrimPrefix(digest, "sha256:"), nil
}

func canonicalRolloutEventHash(event CanonicalRolloutEvent) (string, error) {
	event.Hash = ""
	return hashCanonicalJSON(event)
}

func hashCanonicalJSON(value any) (string, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return "", fmt.Errorf("marshal canonical rollout value: %w", err)
	}
	digest := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}

func canonicalJSONEqual(left, right any) (bool, error) {
	leftJSON, err := json.Marshal(left)
	if err != nil {
		return false, fmt.Errorf("marshal canonical rollout payload: %w", err)
	}
	rightJSON, err := json.Marshal(right)
	if err != nil {
		return false, fmt.Errorf("marshal normalized canonical rollout payload: %w", err)
	}
	return bytes.Equal(leftJSON, rightJSON), nil
}

func normalizeCanonicalSourceRefs(sourceRefs []string) []string {
	if len(sourceRefs) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(sourceRefs))
	for _, sourceRef := range sourceRefs {
		sourceRef = sanitizeCanonicalSourceRef(strings.TrimSpace(sourceRef))
		if sourceRef == "" {
			continue
		}
		seen[sourceRef] = struct{}{}
	}
	if len(seen) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(seen))
	for sourceRef := range seen {
		normalized = append(normalized, sourceRef)
	}
	sort.Strings(normalized)
	return normalized
}

func sanitizeCanonicalSourceRef(sourceRef string) string {
	parsed, err := url.Parse(sourceRef)
	if err != nil || parsed.Scheme == "" {
		if hasInlineCanonicalCredential(sourceRef) {
			digest := sha256.Sum256([]byte(sourceRef))
			return "redacted:" + hex.EncodeToString(digest[:])
		}
		return sourceRef
	}
	if parsed.User != nil {
		parsed.User = url.User("redacted")
	}
	query := parsed.Query()
	changed := false
	for key := range query {
		if !isCanonicalSensitiveKey(key) {
			continue
		}
		query.Set(key, "[REDACTED]")
		changed = true
	}
	if changed {
		parsed.RawQuery = query.Encode()
	}
	normalized := parsed.String()
	if hasInlineCanonicalCredential(normalized) {
		digest := sha256.Sum256([]byte(normalized))
		return "redacted:" + hex.EncodeToString(digest[:])
	}
	return normalized
}

func cloneAndRedactCanonicalPayload(payload map[string]any) (map[string]any, error) {
	if len(payload) == 0 {
		return nil, nil
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal canonical rollout payload: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var cloned map[string]any
	if err := decoder.Decode(&cloned); err != nil {
		return nil, fmt.Errorf("clone canonical rollout payload: %w", err)
	}
	redacted, err := redactCanonicalValue("", cloned)
	if err != nil {
		return nil, err
	}
	return redacted.(map[string]any), nil
}

func redactCanonicalValue(key string, value any) (any, error) {
	if isCanonicalRedactionMarker(value) {
		return value, nil
	}
	if key != "" && !isCanonicalReferenceKey(key) &&
		(isCanonicalSensitiveKey(key) || isCanonicalActionArgsKey(key) || isCanonicalRawContentKey(key)) {
		return canonicalRedactionMarker(value)
	}
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for nestedKey, nestedValue := range typed {
			redacted, err := redactCanonicalValue(nestedKey, nestedValue)
			if err != nil {
				return nil, err
			}
			result[nestedKey] = redacted
		}
		return result, nil
	case []any:
		result := make([]any, len(typed))
		for index, nestedValue := range typed {
			redacted, err := redactCanonicalValue("", nestedValue)
			if err != nil {
				return nil, err
			}
			result[index] = redacted
		}
		return result, nil
	case string:
		if hasInlineCanonicalCredential(typed) {
			return canonicalRedactionMarker(typed)
		}
		return typed, nil
	default:
		return typed, nil
	}
}

func isCanonicalRedactionMarker(value any) bool {
	marker, ok := value.(map[string]any)
	if !ok || len(marker) != 2 || marker["redacted"] != true {
		return false
	}
	digest, ok := marker["sha256"].(string)
	return ok && strings.HasPrefix(digest, "sha256:") && len(digest) == len("sha256:")+sha256.Size*2
}

func canonicalRedactionMarker(value any) (map[string]any, error) {
	digest, err := hashCanonicalJSON(value)
	if err != nil {
		return nil, fmt.Errorf("hash redacted canonical rollout value: %w", err)
	}
	return map[string]any{
		"redacted": true,
		"sha256":   digest,
	}, nil
}

func normalizeCanonicalKey(key string) string {
	var normalized strings.Builder
	for _, char := range strings.ToLower(strings.TrimSpace(key)) {
		if char >= 'a' && char <= 'z' || char >= '0' && char <= '9' {
			normalized.WriteRune(char)
		}
	}
	return normalized.String()
}

func isCanonicalReferenceKey(key string) bool {
	normalized := normalizeCanonicalKey(key)
	return strings.HasSuffix(normalized, "hash") ||
		strings.HasSuffix(normalized, "digest") ||
		strings.HasSuffix(normalized, "fingerprint") ||
		strings.HasSuffix(normalized, "ref") ||
		strings.HasSuffix(normalized, "refs")
}

func isCanonicalSensitiveKey(key string) bool {
	normalized := normalizeCanonicalKey(key)
	if isCanonicalReferenceKey(normalized) {
		return false
	}
	if strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "authorization") ||
		strings.Contains(normalized, "apikey") ||
		strings.Contains(normalized, "credential") ||
		strings.Contains(normalized, "privatekey") ||
		strings.Contains(normalized, "clientsecret") ||
		strings.HasSuffix(normalized, "secret") {
		return true
	}
	return normalized == "token" ||
		strings.HasSuffix(normalized, "accesstoken") ||
		strings.HasSuffix(normalized, "refreshtoken") ||
		strings.HasSuffix(normalized, "bearertoken") ||
		strings.HasSuffix(normalized, "idtoken") ||
		strings.HasSuffix(normalized, "actiontoken") ||
		strings.HasSuffix(normalized, "tokenvalue")
}

func isCanonicalActionArgsKey(key string) bool {
	switch normalizeCanonicalKey(key) {
	case "args", "arguments", "actionargs", "actionarguments", "toolargs", "toolarguments", "commandargs", "commandarguments", "inputarguments":
		return true
	default:
		return false
	}
}

func isCanonicalRawContentKey(key string) bool {
	normalized := normalizeCanonicalKey(key)
	switch normalized {
	case "raw", "rawtext", "rawcontent", "requestbody", "responsebody":
		return true
	}
	hasContentShape := strings.Contains(normalized, "content") ||
		strings.Contains(normalized, "text") ||
		strings.Contains(normalized, "document") ||
		strings.Contains(normalized, "chunk") ||
		strings.Contains(normalized, "context") ||
		strings.Contains(normalized, "raw")
	return hasContentShape && (strings.Contains(normalized, "rag") || strings.Contains(normalized, "retriev"))
}

func hasInlineCanonicalCredential(value string) bool {
	normalized := strings.ToLower(value)
	for _, marker := range []string{
		"bearer ",
		"api_key=", "api-key=", "apikey=", "api_key:", "api-key:", "apikey:",
		"authorization=", "authorization:",
		"password=", "password:",
		"token=", "token:",
		"secret=", "secret:",
		"credential=", "credential:",
	} {
		if strings.Contains(normalized, marker) {
			return true
		}
	}
	return false
}
