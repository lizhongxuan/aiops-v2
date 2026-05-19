package modules

import (
	"fmt"
	"strings"
	"time"
)

const RunnerResultSchemaVersion = "aiops.runner_result/v1"

type ResultEnvelopeOptions struct {
	Status     string
	Changed    bool
	Summary    string
	Data       map[string]any
	Metrics    map[string]any
	Evidence   []map[string]any
	Stdout     string
	Stderr     string
	RawRef     string
	Redactions []string
	Mock       bool
	Duration   time.Duration
}

func NewResultEnvelope(opts ResultEnvelopeOptions) map[string]any {
	status := strings.TrimSpace(opts.Status)
	if status == "" {
		status = "success"
	}
	data := cloneMap(opts.Data)
	metrics := cloneMap(opts.Metrics)
	if _, ok := metrics["duration_ms"]; !ok && opts.Duration > 0 {
		metrics["duration_ms"] = durationMillis(opts.Duration)
	}
	return map[string]any{
		"schema_version": RunnerResultSchemaVersion,
		"status":         status,
		"changed":        opts.Changed,
		"summary":        strings.TrimSpace(opts.Summary),
		"data":           data,
		"metrics":        metrics,
		"evidence":       cloneEvidence(opts.Evidence),
		"stdout":         opts.Stdout,
		"stderr":         opts.Stderr,
		"raw_ref":        strings.TrimSpace(opts.RawRef),
		"redactions":     redactionMarkers(opts.Redactions),
		"mock":           opts.Mock,
	}
}

func WithResultEnvelope(output map[string]any, opts ResultEnvelopeOptions) map[string]any {
	out := cloneMap(output)
	if opts.Data == nil {
		opts.Data = cloneMap(output)
	}
	for key, value := range NewResultEnvelope(opts) {
		out[key] = value
	}
	return out
}

func ReadMockFlag(req Request) bool {
	if req.Step.Args == nil {
		return false
	}
	return readBoolAny(req.Step.Args["mock"], false)
}

func ReadRedactionRules(req Request) []string {
	if req.Step.Args == nil {
		return nil
	}
	var rules []string
	for _, key := range []string{"redaction", "redactions", "redaction_rules"} {
		rules = append(rules, readStringListAny(req.Step.Args[key])...)
	}
	return compactStrings(rules)
}

func RedactText(text string, redactions []string) string {
	out := text
	for _, item := range redactions {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		out = strings.ReplaceAll(out, item, "[REDACTED]")
	}
	return out
}

func RedactAny(value any, redactions []string) any {
	switch typed := value.(type) {
	case string:
		return RedactText(typed, redactions)
	case []any:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = RedactAny(item, redactions)
		}
		return out
	case []string:
		out := make([]any, len(typed))
		for i, item := range typed {
			out[i] = RedactText(item, redactions)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if IsSensitiveKey(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = RedactAny(item, redactions)
			}
		}
		return out
	case map[string]string:
		out := make(map[string]any, len(typed))
		for key, item := range typed {
			if IsSensitiveKey(key) {
				out[key] = "[REDACTED]"
			} else {
				out[key] = RedactText(item, redactions)
			}
		}
		return out
	default:
		return value
	}
}

func IsSensitiveKey(key string) bool {
	normalized := strings.ToLower(strings.TrimSpace(key))
	return normalized == "authorization" ||
		normalized == "proxy-authorization" ||
		normalized == "cookie" ||
		normalized == "set-cookie" ||
		strings.Contains(normalized, "token") ||
		strings.Contains(normalized, "secret") ||
		strings.Contains(normalized, "password") ||
		strings.Contains(normalized, "private_key")
}

func durationMillis(d time.Duration) int64 {
	ms := d.Milliseconds()
	if ms == 0 && d > 0 {
		return 1
	}
	return ms
}

func cloneMap(input map[string]any) map[string]any {
	if input == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(input))
	for key, value := range input {
		out[key] = value
	}
	return out
}

func cloneEvidence(input []map[string]any) []map[string]any {
	if input == nil {
		return []map[string]any{}
	}
	out := make([]map[string]any, len(input))
	for i, item := range input {
		out[i] = cloneMap(item)
	}
	return out
}

func readStringListAny(raw any) []string {
	switch v := raw.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(v) == "" {
			return nil
		}
		return []string{v}
	case []string:
		return append([]string{}, v...)
	case []any:
		out := make([]string, 0, len(v))
		for _, item := range v {
			out = append(out, fmt.Sprint(item))
		}
		return out
	default:
		return []string{fmt.Sprint(v)}
	}
}

func readBoolAny(raw any, fallback bool) bool {
	switch v := raw.(type) {
	case bool:
		return v
	case string:
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "1", "true", "yes", "on":
			return true
		case "0", "false", "no", "off":
			return false
		}
	}
	return fallback
}

func compactStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func redactionMarkers(values []string) []string {
	values = compactStrings(values)
	out := make([]string, 0, len(values))
	for range values {
		out = append(out, "[REDACTED]")
	}
	return out
}
