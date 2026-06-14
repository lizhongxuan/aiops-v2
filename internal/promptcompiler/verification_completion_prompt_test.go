package promptcompiler

import (
	"strings"
	"testing"
)

func TestCompletionGatePromptConstrainsPartialAndFailStatuses(t *testing.T) {
	developer := strings.Join(developerInstructionSections(CompileContext{}), "\n\n")
	for _, want := range []string{
		"verification status is PARTIAL",
		"partially verified or blocked",
		"blocker source",
		"verification status is FAIL",
		"checked contract",
		"known constraints",
		"expected vs actual",
		"contract_unavailable blocker",
	} {
		if !strings.Contains(developer, want) {
			t.Fatalf("completion gate missing %q:\n%s", want, developer)
		}
	}
}
