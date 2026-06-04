package runtimekernel

import (
	"strings"
	"time"

	"aiops-v2/internal/featureflag"
	"aiops-v2/internal/runtimekernel/toolfailure"
)

type ReadOnlyRetryConfig struct {
	Enabled     bool
	MaxPerCall  int
	MaxPerTurn  int
	BackoffBase time.Duration
	BackoffMax  time.Duration
}

type ReadOnlyRetryInput struct {
	Config                           ReadOnlyRetryConfig
	Mutating                         bool
	FailureKind                      string
	OriginalArgumentsHash            string
	EffectiveArgumentsHash           string
	OriginalToolSurfaceFingerprint   string
	EffectiveToolSurfaceFingerprint  string
	CompletedRetryAttemptsForCall    int
	CompletedRetryAttemptsForTurn    int
	ProspectiveRetryAttemptsThisCall int
}

type ReadOnlyRetryDecision struct {
	Allowed       bool
	Eligible      bool
	Reason        string
	Backoff       time.Duration
	FailureKind   string
	AttemptNumber int
}

func ReadOnlyRetryConfigFromFlags(flags featureflag.Flags) ReadOnlyRetryConfig {
	return ReadOnlyRetryConfig{
		Enabled:     flags.ReadOnlyToolRetryEnabled,
		MaxPerCall:  flags.ReadOnlyToolRetryMaxPerCall,
		MaxPerTurn:  flags.ReadOnlyToolRetryMaxPerTurn,
		BackoffBase: flags.ReadOnlyToolRetryBackoffBase,
		BackoffMax:  flags.ReadOnlyToolRetryBackoffMax,
	}
}

func DefaultReadOnlyRetryConfig() ReadOnlyRetryConfig {
	return ReadOnlyRetryConfigFromFlags(featureflag.Default())
}

func DecideReadOnlyRetry(input ReadOnlyRetryInput) ReadOnlyRetryDecision {
	cfg := normalizeReadOnlyRetryConfig(input.Config)
	failureKind := strings.TrimSpace(input.FailureKind)
	attemptNumber := input.ProspectiveRetryAttemptsThisCall
	if attemptNumber <= 0 {
		attemptNumber = input.CompletedRetryAttemptsForCall + 1
	}
	decision := ReadOnlyRetryDecision{
		FailureKind:   failureKind,
		AttemptNumber: attemptNumber,
	}
	if input.Mutating {
		decision.Reason = "tool is mutating"
		return decision
	}
	if !readOnlyRetryFailureKindAllowed(failureKind) {
		decision.Reason = "failure kind is not retryable"
		return decision
	}
	if strings.TrimSpace(input.OriginalArgumentsHash) == "" || strings.TrimSpace(input.EffectiveArgumentsHash) == "" || input.OriginalArgumentsHash != input.EffectiveArgumentsHash {
		decision.Reason = "arguments hash changed"
		return decision
	}
	if strings.TrimSpace(input.OriginalToolSurfaceFingerprint) != strings.TrimSpace(input.EffectiveToolSurfaceFingerprint) {
		decision.Reason = "tool surface fingerprint changed"
		return decision
	}
	if input.CompletedRetryAttemptsForCall >= cfg.MaxPerCall {
		decision.Reason = "retry per-call budget exhausted"
		return decision
	}
	if input.CompletedRetryAttemptsForTurn >= cfg.MaxPerTurn {
		decision.Reason = "retry per-turn budget exhausted"
		return decision
	}
	decision.Eligible = true
	decision.Backoff = readOnlyRetryBackoff(cfg, attemptNumber)
	if !cfg.Enabled {
		decision.Reason = "read-only retry disabled"
		return decision
	}
	decision.Allowed = true
	decision.Reason = "read-only retry allowed"
	return decision
}

func normalizeReadOnlyRetryConfig(cfg ReadOnlyRetryConfig) ReadOnlyRetryConfig {
	defaults := DefaultReadOnlyRetryConfig()
	if cfg.MaxPerCall <= 0 {
		cfg.MaxPerCall = defaults.MaxPerCall
	}
	if cfg.MaxPerTurn <= 0 {
		cfg.MaxPerTurn = defaults.MaxPerTurn
	}
	if cfg.BackoffBase < 0 {
		cfg.BackoffBase = defaults.BackoffBase
	}
	if cfg.BackoffMax < 0 {
		cfg.BackoffMax = defaults.BackoffMax
	}
	return cfg
}

func readOnlyRetryFailureKindAllowed(kind string) bool {
	switch toolfailure.ToolFailureKind(strings.TrimSpace(kind)) {
	case toolfailure.KindTimeout, toolfailure.KindTransientNetwork, toolfailure.KindRateLimited:
		return true
	default:
		return false
	}
}

func readOnlyRetryBackoff(cfg ReadOnlyRetryConfig, attemptNumber int) time.Duration {
	if cfg.BackoffBase <= 0 {
		return 0
	}
	if attemptNumber <= 0 {
		attemptNumber = 1
	}
	backoff := cfg.BackoffBase
	for i := 1; i < attemptNumber; i++ {
		backoff *= 2
		if cfg.BackoffMax > 0 && backoff >= cfg.BackoffMax {
			return cfg.BackoffMax
		}
	}
	if cfg.BackoffMax > 0 && backoff > cfg.BackoffMax {
		return cfg.BackoffMax
	}
	return backoff
}
