package opsmanual

import (
	"fmt"
	"strings"
	"time"
)

const (
	SessionOpsFactTargetHost           = "target_host"
	SessionOpsFactTargetInstance       = "target_instance"
	SessionOpsFactOpsManualSuppression = "ops_manual_suppression"
)

type SessionOpsContext struct {
	SessionID string           `json:"session_id"`
	ThreadID  string           `json:"thread_id,omitempty"`
	Facts     []SessionOpsFact `json:"facts"`
	UpdatedAt time.Time        `json:"updated_at"`
}

type SessionOpsFact struct {
	Key             string         `json:"key"`
	Value           any            `json:"value"`
	Source          string         `json:"source"`
	Confidence      float64        `json:"confidence"`
	ConfirmedByUser bool           `json:"confirmed_by_user,omitempty"`
	Sensitive       bool           `json:"sensitive,omitempty"`
	SecretRef       string         `json:"secret_ref,omitempty"`
	EvidenceRef     string         `json:"evidence_ref,omitempty"`
	Attributes      map[string]any `json:"attributes,omitempty"`
	ExpiresAt       time.Time      `json:"expires_at"`
	CreatedAt       time.Time      `json:"created_at"`
	UpdatedAt       time.Time      `json:"updated_at"`
}

type SessionOpsFactFilter struct {
	Keys []string
	Now  time.Time
}

type OpsManualSuppression struct {
	ManualID    string `json:"manual_id,omitempty"`
	ObjectType  string `json:"object_type,omitempty"`
	Action      string `json:"action,omitempty"`
	TargetScope string `json:"target_scope,omitempty"`
	FlowID      string `json:"ops_manual_flow_id,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

func OpsManualSuppressionForManual(manual OpsManual, frame OperationFrame) OpsManualSuppression {
	return OpsManualSuppression{
		ManualID: strings.TrimSpace(manual.ID),
		ObjectType: firstNonEmpty(
			frame.ObjectType,
			frame.Target.Type,
			frame.Operation.TargetType,
			manual.Operation.TargetType,
			manual.Applicability.Middleware,
		),
		Action: firstNonEmpty(
			frame.Operation.Action,
			frame.OperationType,
			manual.Operation.Action,
		),
		TargetScope: OpsManualSuppressionTargetScope(frame),
		Reason:      "user_opt_out",
	}.normalized()
}

func OpsManualSuppressionFromMetadata(metadata map[string]any, frame OperationFrame) (OpsManualSuppression, bool) {
	suppression := OpsManualSuppression{
		ManualID: firstMetadataAnyValue(metadata,
			"manual_id",
			"manualId",
			"opsManualManualId",
			"ops_manual_manual_id",
		),
		ObjectType: firstMetadataAnyValue(metadata,
			"object_type",
			"objectType",
			"opsManualObjectType",
			"ops_manual_object_type",
		),
		Action: firstMetadataAnyValue(metadata,
			"action",
			"operation_type",
			"operationType",
			"opsManualOperationAction",
			"ops_manual_operation_action",
		),
		TargetScope: firstMetadataAnyValue(metadata,
			"target_scope",
			"targetScope",
			"opsManualTargetScope",
			"ops_manual_target_scope",
		),
		FlowID: firstMetadataAnyValue(metadata,
			"ops_manual_flow_id",
			"opsManualFlowId",
			"opsManualFlowID",
			"ops_manual_flowID",
		),
		Reason: firstNonEmpty(firstMetadataAnyValue(metadata, "reason", "opsManualSkipReason"), "user_opt_out"),
	}
	if suppression.ObjectType == "" {
		suppression.ObjectType = firstNonEmpty(frame.ObjectType, frame.Target.Type, frame.Operation.TargetType)
	}
	if suppression.Action == "" {
		suppression.Action = firstNonEmpty(frame.Operation.Action, frame.OperationType, frame.Intent)
	}
	if suppression.TargetScope == "" {
		suppression.TargetScope = OpsManualSuppressionTargetScope(frame)
	}
	suppression = suppression.normalized()
	return suppression, suppression.ManualID != "" && suppression.ObjectType != "" && suppression.Action != "" && suppression.TargetScope != ""
}

func OpsManualSuppressionTargetScope(frame OperationFrame) string {
	hosts := append([]string(nil), frame.TargetScope.Hosts...)
	if len(hosts) == 0 && strings.TrimSpace(frame.Target.Name) != "" {
		hosts = append(hosts, frame.Target.Name)
	}
	var parts []string
	for _, host := range dedupe(hosts) {
		host = strings.TrimSpace(host)
		if host != "" {
			parts = append(parts, "host:"+host)
		}
	}
	for _, item := range []struct {
		key   string
		value string
	}{
		{"cluster", frame.TargetScope.Cluster},
		{"namespace", frame.TargetScope.Namespace},
		{"service", frame.TargetScope.Service},
	} {
		if value := strings.TrimSpace(item.value); value != "" {
			parts = append(parts, item.key+":"+value)
		}
	}
	return strings.ToLower(strings.Join(parts, "|"))
}

func NewOpsManualSuppressionFact(suppression OpsManualSuppression, now time.Time) SessionOpsFact {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	return SessionOpsFact{
		Key:             SessionOpsFactOpsManualSuppression,
		Value:           suppression,
		Source:          "user_opt_out",
		Confidence:      1,
		ConfirmedByUser: true,
		Attributes: map[string]any{
			"manual_id":          strings.TrimSpace(suppression.ManualID),
			"object_type":        strings.TrimSpace(suppression.ObjectType),
			"action":             strings.TrimSpace(suppression.Action),
			"target_scope":       strings.TrimSpace(suppression.TargetScope),
			"ops_manual_flow_id": strings.TrimSpace(suppression.FlowID),
			"reason":             firstNonEmpty(strings.TrimSpace(suppression.Reason), "user_opt_out"),
		},
		ExpiresAt: now.Add(30 * time.Minute),
		CreatedAt: now,
		UpdatedAt: now,
	}
}

func OpsManualSuppressionFromFact(fact SessionOpsFact) (OpsManualSuppression, bool) {
	if strings.TrimSpace(fact.Key) != SessionOpsFactOpsManualSuppression {
		return OpsManualSuppression{}, false
	}
	if typed, ok := fact.Value.(OpsManualSuppression); ok {
		return typed.normalized(), true
	}
	if raw, ok := fact.Value.(map[string]any); ok {
		return OpsManualSuppression{
			ManualID:    metadataStringFromMap(raw, "manual_id"),
			ObjectType:  metadataStringFromMap(raw, "object_type"),
			Action:      metadataStringFromMap(raw, "action"),
			TargetScope: metadataStringFromMap(raw, "target_scope"),
			FlowID:      metadataStringFromMap(raw, "ops_manual_flow_id"),
			Reason:      metadataStringFromMap(raw, "reason"),
		}.normalized(), true
	}
	if len(fact.Attributes) > 0 {
		return OpsManualSuppression{
			ManualID:    metadataStringFromMap(fact.Attributes, "manual_id"),
			ObjectType:  metadataStringFromMap(fact.Attributes, "object_type"),
			Action:      metadataStringFromMap(fact.Attributes, "action"),
			TargetScope: metadataStringFromMap(fact.Attributes, "target_scope"),
			FlowID:      metadataStringFromMap(fact.Attributes, "ops_manual_flow_id"),
			Reason:      metadataStringFromMap(fact.Attributes, "reason"),
		}.normalized(), true
	}
	return OpsManualSuppression{}, false
}

func (s OpsManualSuppression) Matches(candidate OpsManualSuppression) bool {
	s = s.normalized()
	candidate = candidate.normalized()
	return s.ManualID != "" &&
		s.ManualID == candidate.ManualID &&
		s.ObjectType == candidate.ObjectType &&
		s.Action == candidate.Action &&
		s.TargetScope == candidate.TargetScope
}

func (s OpsManualSuppression) normalized() OpsManualSuppression {
	return OpsManualSuppression{
		ManualID:    strings.TrimSpace(s.ManualID),
		ObjectType:  strings.ToLower(strings.TrimSpace(s.ObjectType)),
		Action:      strings.ToLower(strings.TrimSpace(s.Action)),
		TargetScope: strings.ToLower(strings.TrimSpace(s.TargetScope)),
		FlowID:      strings.TrimSpace(s.FlowID),
		Reason:      strings.TrimSpace(s.Reason),
	}
}

func sessionFactIdentity(fact SessionOpsFact) string {
	if fact.Key == SessionOpsFactOpsManualSuppression {
		if suppression, ok := OpsManualSuppressionFromFact(fact); ok {
			return fmt.Sprintf("%s\x00%s\x00%s\x00%s\x00%s", fact.Key, suppression.ManualID, suppression.ObjectType, suppression.Action, suppression.TargetScope)
		}
	}
	return fmt.Sprintf("%s\x00%v", strings.TrimSpace(fact.Key), fact.Value)
}

func sanitizeSessionOpsFact(fact SessionOpsFact) SessionOpsFact {
	out := fact
	out.Attributes = cloneMap(fact.Attributes)
	if out.Sensitive {
		if strings.TrimSpace(out.SecretRef) != "" {
			out.Value = out.SecretRef
		} else {
			out.Value = nil
		}
	}
	return out
}

func factExpired(fact SessionOpsFact, now time.Time) bool {
	return !now.IsZero() && !fact.ExpiresAt.IsZero() && !fact.ExpiresAt.After(now)
}

func factMatchesFilter(fact SessionOpsFact, filter SessionOpsFactFilter) bool {
	if factExpired(fact, filter.Now) {
		return false
	}
	if len(filter.Keys) == 0 {
		return true
	}
	for _, key := range filter.Keys {
		if strings.TrimSpace(key) == strings.TrimSpace(fact.Key) {
			return true
		}
	}
	return false
}

func firstMetadataAnyValue(metadata map[string]any, keys ...string) string {
	for _, key := range keys {
		if value := metadataString(metadata, key); value != "" {
			return value
		}
	}
	return ""
}
