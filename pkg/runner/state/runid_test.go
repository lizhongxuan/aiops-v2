package state

import "testing"

func TestNewRunIDUniquenessAndFormat(t *testing.T) {
	seen := map[string]struct{}{}
	for i := 0; i < 128; i++ {
		runID := NewRunID()
		if err := ValidateRunID(runID); err != nil {
			t.Fatalf("generated invalid run_id %q: %v", runID, err)
		}
		if _, ok := seen[runID]; ok {
			t.Fatalf("generated duplicate run_id %q", runID)
		}
		seen[runID] = struct{}{}
	}
}

func TestValidateRunIDRejectsInvalidValues(t *testing.T) {
	tests := []string{
		"",
		" ",
		"bad*id",
		"ab",
	}
	for _, runID := range tests {
		if err := ValidateRunID(runID); err == nil {
			t.Fatalf("expected invalid run_id %q to fail validation", runID)
		}
	}
}
