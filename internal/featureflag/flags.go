package featureflag

import (
	"strconv"
	"strings"
	"time"
)

const (
	envDiagnosticProtocol             = "AIOPS_DIAGNOSTIC_PROTOCOL"
	envReadOnlyToolRetry              = "AIOPS_FLAG_READONLY_TOOL_RETRY"
	envReadOnlyToolRetryMaxPerCall    = "AIOPS_READONLY_TOOL_RETRY_MAX_PER_CALL"
	envReadOnlyToolRetryMaxPerTurn    = "AIOPS_READONLY_TOOL_RETRY_MAX_PER_TURN"
	envReadOnlyToolRetryBackoffBaseMS = "AIOPS_READONLY_TOOL_RETRY_BACKOFF_BASE_MS"
	envReadOnlyToolRetryBackoffMaxMS  = "AIOPS_READONLY_TOOL_RETRY_BACKOFF_MAX_MS"
)

// Flags controls runtime prompt and retry behavior. Migration-only feature
// flags must not live here; removed paths should be deleted, not hidden behind
// old/new switches.
type Flags struct {
	DiagnosticProtocol bool

	ReadOnlyToolRetryEnabled     bool
	ReadOnlyToolRetryMaxPerCall  int
	ReadOnlyToolRetryMaxPerTurn  int
	ReadOnlyToolRetryBackoffBase time.Duration
	ReadOnlyToolRetryBackoffMax  time.Duration
}

func Default() Flags {
	return Flags{
		DiagnosticProtocol:           true,
		ReadOnlyToolRetryMaxPerCall:  1,
		ReadOnlyToolRetryMaxPerTurn:  3,
		ReadOnlyToolRetryBackoffBase: 300 * time.Millisecond,
		ReadOnlyToolRetryBackoffMax:  2 * time.Second,
	}
}

func FromEnv(lookup func(string) string) Flags {
	f := Default()
	if lookup == nil {
		return f
	}

	f.DiagnosticProtocol = parseBoolDefault(lookup(envDiagnosticProtocol), true)
	f.ReadOnlyToolRetryEnabled = parseBool(lookup(envReadOnlyToolRetry))
	f.ReadOnlyToolRetryMaxPerCall = parsePositiveInt(lookup(envReadOnlyToolRetryMaxPerCall), f.ReadOnlyToolRetryMaxPerCall)
	f.ReadOnlyToolRetryMaxPerTurn = parsePositiveInt(lookup(envReadOnlyToolRetryMaxPerTurn), f.ReadOnlyToolRetryMaxPerTurn)
	f.ReadOnlyToolRetryBackoffBase = parseMillisDuration(lookup(envReadOnlyToolRetryBackoffBaseMS), f.ReadOnlyToolRetryBackoffBase)
	f.ReadOnlyToolRetryBackoffMax = parseMillisDuration(lookup(envReadOnlyToolRetryBackoffMaxMS), f.ReadOnlyToolRetryBackoffMax)
	return f
}

func (f Flags) Clone() Flags {
	return f
}

func parseBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseBoolDefault(value string, fallback bool) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	switch strings.ToLower(value) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func parsePositiveInt(value string, fallback int) int {
	value = strings.TrimSpace(value)
	if value == "" {
		return fallback
	}
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func parseMillisDuration(value string, fallback time.Duration) time.Duration {
	millis := parsePositiveInt(value, -1)
	if millis <= 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}
