package promptcompiler

import (
	"strings"
	"testing"
)

func TestCompletionGatePromptConstrainsPartialAndFailStatuses(t *testing.T) {
	compiled, err := NewCompiler().Compile(CompileContext{})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	modelInput := compiledEnvelopeTextForTest(compiled)
	for _, forbidden := range []string{
		"verification status is PARTIAL",
		"partially verified or blocked",
		"state the blocker",
		"verification status is PARTIAL or FAIL",
		"checked contract",
		"expected vs actual",
		"available evidence reference",
	} {
		if strings.Contains(modelInput, forbidden) {
			t.Fatalf("model input leaked old completion gate phrase %q:\n%s", forbidden, modelInput)
		}
	}
}
