package featureflag

import (
	"time"
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

type RuntimeSettings interface {
	RuntimeDiagnosticProtocol() bool
	RuntimeReadOnlyRetryEnabled() bool
	RuntimeReadOnlyRetryMaxPerCall() int
	RuntimeReadOnlyRetryMaxPerTurn() int
	RuntimeReadOnlyRetryBackoffBaseMs() int
	RuntimeReadOnlyRetryBackoffMaxMs() int
}

func FromRuntimeSettings(settings RuntimeSettings) Flags {
	f := Default()
	if settings == nil {
		return f
	}

	f.DiagnosticProtocol = settings.RuntimeDiagnosticProtocol()
	f.ReadOnlyToolRetryEnabled = settings.RuntimeReadOnlyRetryEnabled()
	f.ReadOnlyToolRetryMaxPerCall = positiveInt(settings.RuntimeReadOnlyRetryMaxPerCall(), f.ReadOnlyToolRetryMaxPerCall)
	f.ReadOnlyToolRetryMaxPerTurn = positiveInt(settings.RuntimeReadOnlyRetryMaxPerTurn(), f.ReadOnlyToolRetryMaxPerTurn)
	f.ReadOnlyToolRetryBackoffBase = millisDuration(settings.RuntimeReadOnlyRetryBackoffBaseMs(), f.ReadOnlyToolRetryBackoffBase)
	f.ReadOnlyToolRetryBackoffMax = millisDuration(settings.RuntimeReadOnlyRetryBackoffMaxMs(), f.ReadOnlyToolRetryBackoffMax)
	return f
}

func (f Flags) Clone() Flags {
	return f
}

func positiveInt(value int, fallback int) int {
	if value <= 0 {
		return fallback
	}
	return value
}

func millisDuration(millis int, fallback time.Duration) time.Duration {
	if millis <= 0 {
		return fallback
	}
	return time.Duration(millis) * time.Millisecond
}
