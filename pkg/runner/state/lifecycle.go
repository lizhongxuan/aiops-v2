package state

import (
	"fmt"
	"strings"
)

const (
	RunStatusQueued      = "queued"
	RunStatusRunning     = "running"
	RunStatusSuccess     = "success"
	RunStatusFailed      = "failed"
	RunStatusCanceled    = "canceled"
	RunStatusInterrupted = "interrupted"
)

var validRunStatus = map[string]struct{}{
	RunStatusQueued:      {},
	RunStatusRunning:     {},
	RunStatusSuccess:     {},
	RunStatusFailed:      {},
	RunStatusCanceled:    {},
	RunStatusInterrupted: {},
}

var allowedRunTransitions = map[string]map[string]struct{}{
	"": {
		RunStatusQueued:  {},
		RunStatusRunning: {},
	},
	RunStatusQueued: {
		RunStatusQueued:      {},
		RunStatusRunning:     {},
		RunStatusFailed:      {},
		RunStatusCanceled:    {},
		RunStatusInterrupted: {},
	},
	RunStatusRunning: {
		RunStatusRunning:     {},
		RunStatusSuccess:     {},
		RunStatusFailed:      {},
		RunStatusCanceled:    {},
		RunStatusInterrupted: {},
	},
	RunStatusSuccess: {
		RunStatusSuccess: {},
	},
	RunStatusFailed: {
		RunStatusFailed: {},
	},
	RunStatusCanceled: {
		RunStatusCanceled: {},
	},
	RunStatusInterrupted: {
		RunStatusInterrupted: {},
	},
}

func ValidateRunStatus(status string) error {
	key := strings.TrimSpace(strings.ToLower(status))
	if _, ok := validRunStatus[key]; ok {
		return nil
	}
	return fmt.Errorf("invalid run status %q", status)
}

func ValidateRunTransition(from, to string) error {
	fromKey := strings.TrimSpace(strings.ToLower(from))
	toKey := strings.TrimSpace(strings.ToLower(to))
	if err := ValidateRunStatus(toKey); err != nil {
		return err
	}
	allowed, ok := allowedRunTransitions[fromKey]
	if !ok {
		return fmt.Errorf("invalid run transition from %q to %q", from, to)
	}
	if _, ok := allowed[toKey]; !ok {
		return fmt.Errorf("invalid run transition from %q to %q", from, to)
	}
	return nil
}

func IsTerminalRunStatus(status string) bool {
	switch strings.TrimSpace(strings.ToLower(status)) {
	case RunStatusSuccess, RunStatusFailed, RunStatusCanceled, RunStatusInterrupted:
		return true
	default:
		return false
	}
}
