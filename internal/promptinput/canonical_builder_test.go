package promptinput

import (
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func buildCanonicalPromptInputForTest(t *testing.T, req BuildRequest) (BuildResult, error) {
	t.Helper()
	originalCompiled := req.Compiled
	compileContext := promptcompiler.CompileContext{
		SessionType:   "host",
		Mode:          "inspect",
		Profile:       promptcompiler.PromptProfileEvidenceRCA,
		ProtocolState: originalCompiled.Dynamic.ProtocolState,
	}
	if content := strings.TrimSpace(originalCompiled.Dynamic.Content); content != "" {
		compileContext.ExtraSections = []promptcompiler.PromptSection{{Title: "Test Dynamic Context", Content: content}}
	}
	compiled, err := promptcompiler.NewCompiler().Compile(compileContext)
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	if len(originalCompiled.PromptSections) > 0 {
		compiled.PromptSections = append([]promptcompiler.PromptSectionTrace(nil), originalCompiled.PromptSections...)
	}
	if len(originalCompiled.ChangedSections) > 0 {
		compiled.ChangedSections = append([]promptcompiler.ChangedPromptSection(nil), originalCompiled.ChangedSections...)
	}
	if len(originalCompiled.Tools.DeferredDirectory) > 0 {
		compiled.Tools.DeferredDirectory = append([]promptcompiler.DeferredToolDirectoryEntry(nil), originalCompiled.Tools.DeferredDirectory...)
	}
	req.Compiled = compiled
	req.Envelope = compiled.EnvelopeV2

	if strings.TrimSpace(req.CurrentUserInput) == "" && strings.TrimSpace(req.ContinuationInstruction) == "" {
		currentUser := latestTestUserContent(req.History)
		if currentUser == "" {
			currentUser = "canonical test request"
			req.History = append(req.History, Message{Role: "user", Content: currentUser})
		}
		req.Iteration = 0
		req.CurrentInputKind = CurrentInputKindInitialUser
		req.CurrentUserInput = currentUser
	}
	return Builder{}.Build(req)
}

func latestTestUserContent(history []Message) string {
	for index := len(history) - 1; index >= 0; index-- {
		if strings.TrimSpace(history[index].Role) == "user" && strings.TrimSpace(history[index].Content) != "" {
			return strings.TrimSpace(history[index].Content)
		}
	}
	return ""
}
