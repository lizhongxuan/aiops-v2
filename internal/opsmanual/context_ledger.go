package opsmanual

import (
	"fmt"
	"strings"
	"time"
)

type OperationContextFact struct {
	Key             string  `json:"key"`
	Value           any     `json:"value"`
	Source          string  `json:"source"`
	Confidence      float64 `json:"confidence,omitempty"`
	Freshness       string  `json:"freshness,omitempty"`
	ConfirmedByUser bool    `json:"confirmed_by_user,omitempty"`
	Sensitive       bool    `json:"sensitive,omitempty"`
	CreatedAt       string  `json:"created_at,omitempty"`
}

type OperationContextLedger struct {
	Facts []OperationContextFact `json:"facts,omitempty"`
}

func NewOperationContextLedger() OperationContextLedger {
	return OperationContextLedger{Facts: []OperationContextFact{}}
}

func (l *OperationContextLedger) AddFact(fact OperationContextFact) {
	fact.Key = strings.TrimSpace(fact.Key)
	if fact.Key == "" || !valuePresent(fact.Value) {
		return
	}
	if fact.CreatedAt == "" {
		fact.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if fact.Confidence == 0 {
		fact.Confidence = defaultFactConfidence(fact.Source)
	}
	for i, existing := range l.Facts {
		if existing.Key != fact.Key {
			continue
		}
		if shouldReplaceFact(existing, fact) {
			l.Facts[i] = fact
		}
		return
	}
	l.Facts = append(l.Facts, fact)
}

func (l OperationContextLedger) Find(key string) (OperationContextFact, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return OperationContextFact{}, false
	}
	for _, fact := range l.Facts {
		if fact.Key == key && valuePresent(fact.Value) {
			return fact, true
		}
	}
	return OperationContextFact{}, false
}

func (l OperationContextLedger) FindByType(paramID string) (OperationContextFact, bool) {
	paramID = strings.TrimSpace(paramID)
	if fact, ok := l.Find(paramID); ok {
		return fact, true
	}
	switch NormalizeParamType(paramID, "") {
	case "host_ref":
		return l.Find("target_host")
	case "resource_ref":
		for _, key := range []string{paramID, "target_instance", "redis_instance", "pg_instance", "mysql_instance"} {
			if fact, ok := l.Find(key); ok {
				return fact, true
			}
		}
	case "path":
		if strings.Contains(paramID, "backup") {
			return l.Find("backup_path")
		}
	}
	return OperationContextFact{}, false
}

func (l *OperationContextLedger) Merge(other OperationContextLedger) {
	for _, fact := range other.Facts {
		l.AddFact(fact)
	}
}

func (l OperationContextLedger) RedactedFacts() OperationContextLedger {
	out := NewOperationContextLedger()
	for _, fact := range l.Facts {
		cp := fact
		if cp.Sensitive {
			cp.Value = "[REDACTED]"
		}
		out.Facts = append(out.Facts, cp)
	}
	return out
}

func LedgerFromOperationFrame(frame OperationFrame) OperationContextLedger {
	ledger := NewOperationContextLedger()
	if host := firstHostFromFrame(frame); host != "" {
		ledger.AddFact(OperationContextFact{Key: "target_host", Value: host, Source: "operation_frame", Confidence: 0.92})
	}
	if target := strings.TrimSpace(frame.Target.Name); target != "" {
		key := "target_instance"
		switch strings.TrimSpace(frame.ObjectType) {
		case "redis":
			key = "redis_instance"
		case "postgresql":
			key = "pg_instance"
		case "mysql":
			key = "mysql_instance"
		}
		ledger.AddFact(OperationContextFact{Key: "target_instance", Value: target, Source: "operation_frame", Confidence: 0.92})
		if key != "target_instance" {
			ledger.AddFact(OperationContextFact{Key: key, Value: target, Source: "operation_frame", Confidence: 0.92})
		}
	}
	for key, value := range frame.RequiredParams {
		ledger.AddFact(OperationContextFact{Key: key, Value: value, Source: "operation_frame", Confidence: 0.85})
	}
	for key, value := range frame.Metadata {
		if strings.Contains(strings.ToLower(key), "password") || strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "secret") {
			ledger.AddFact(OperationContextFact{Key: key, Value: value, Source: "operation_frame", Confidence: 0.85, Sensitive: true})
			continue
		}
		if key == "target_name" || strings.HasSuffix(key, "_path") || strings.HasSuffix(key, "_instance") {
			ledger.AddFact(OperationContextFact{Key: key, Value: value, Source: "operation_frame", Confidence: 0.82})
		}
	}
	return ledger
}

func LedgerFromKnownParams(params map[string]any, source string) OperationContextLedger {
	ledger := NewOperationContextLedger()
	for key, value := range params {
		sensitive := strings.Contains(strings.ToLower(key), "password") || strings.Contains(strings.ToLower(key), "token") || strings.Contains(strings.ToLower(key), "secret")
		ledger.AddFact(OperationContextFact{Key: key, Value: value, Source: firstNonEmpty(source, "known_params"), Confidence: 0.98, ConfirmedByUser: source == "user" || source == "user_form", Sensitive: sensitive})
	}
	return ledger
}

func firstHostFromFrame(frame OperationFrame) string {
	if len(frame.TargetScope.Hosts) > 0 {
		return strings.TrimSpace(frame.TargetScope.Hosts[0])
	}
	if frame.Target.Type != "" && frame.Target.Name != "" && !strings.Contains(frame.Target.Type, "pod") {
		return strings.TrimSpace(frame.Target.Name)
	}
	return ""
}

func defaultFactConfidence(source string) float64 {
	switch strings.TrimSpace(source) {
	case "user", "user_form", "known_params":
		return 1
	case "selected_host":
		return 0.88
	case "tool_execution_host":
		return 0.9
	case "operation_frame":
		return 0.92
	case "session_fact":
		return 0.87
	case "conversation":
		return 0.72
	default:
		return 0.6
	}
}

func shouldReplaceFact(existing, next OperationContextFact) bool {
	if sourcePriority(next.Source) != sourcePriority(existing.Source) {
		return sourcePriority(next.Source) > sourcePriority(existing.Source)
	}
	if existing.ConfirmedByUser && !next.ConfirmedByUser {
		return false
	}
	if next.ConfirmedByUser && !existing.ConfirmedByUser {
		return true
	}
	if next.Confidence > existing.Confidence {
		return true
	}
	if next.Confidence == existing.Confidence && fmt.Sprint(next.Value) != "" {
		return true
	}
	return false
}

func sourcePriority(source string) int {
	switch strings.TrimSpace(source) {
	case "user", "user_form", "known_params":
		return 90
	case "operation_frame":
		return 80
	case "tool_execution_host":
		return 70
	case "selected_host":
		return 60
	case "session_fact":
		return 50
	case "resource_discovery", "docker", "k8s", "host_readonly", "coroot":
		return 40
	case "run_record", "letta_hint":
		return 30
	case "manual_default":
		return 20
	default:
		return 10
	}
}
