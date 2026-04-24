package state

import (
	"crypto/rand"
	"fmt"
	"regexp"
	"strings"
	"time"
)

var runIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._:-]{7,127}$`)

func NewRunID() string {
	var suffix [6]byte
	if _, err := rand.Read(suffix[:]); err != nil {
		// Fallback to timestamp-only suffix when entropy source is unavailable.
		return fmt.Sprintf("run-%d", time.Now().UTC().UnixNano())
	}
	return fmt.Sprintf("run-%d-%x", time.Now().UTC().UnixNano(), suffix)
}

func ValidateRunID(runID string) error {
	id := strings.TrimSpace(runID)
	if id == "" {
		return fmt.Errorf("run_id is required")
	}
	if !runIDPattern.MatchString(id) {
		return fmt.Errorf("run_id has invalid format")
	}
	return nil
}
