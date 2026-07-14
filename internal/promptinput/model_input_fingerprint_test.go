package promptinput

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestModelInputFingerprintIsolatesRAGUserAndRoleChanges(t *testing.T) {
	compile := func(rag string) promptcompiler.CompiledPrompt {
		t.Helper()
		ctx := promptcompiler.CompileContext{SessionType: "host", Mode: "inspect", HostContext: "host-a"}
		if rag != "" {
			ctx.ExtraSections = []promptcompiler.PromptSection{{Title: "Evidence", Content: rag}}
		}
		compiled, err := promptcompiler.NewCompiler().Compile(ctx)
		if err != nil {
			t.Fatalf("Compile() error = %v", err)
		}
		return compiled
	}
	build := func(compiled promptcompiler.CompiledPrompt, user string) BuildResult {
		t.Helper()
		result, err := (Builder{}).Build(BuildRequest{
			Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 0,
			CurrentInputKind: CurrentInputKindInitialUser, CurrentUserInput: user,
			History: []Message{{Role: "user", Content: user}},
		})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		return result
	}
	fingerprint := func(result BuildResult) promptcompiler.PromptFingerprint {
		t.Helper()
		fp, err := BuildModelInputPromptFingerprint(result.Items)
		if err != nil {
			t.Fatalf("BuildModelInputPromptFingerprint() error = %v", err)
		}
		return fp
	}

	baseCompiled := compile("rag-a")
	baseResult := build(baseCompiled, "inspect redis")
	base := fingerprint(baseResult)
	if base.AbsoluteSystemHash != baseCompiled.Fingerprint.AbsoluteSystemHash || base.RoleProfileHash != baseCompiled.Fingerprint.RoleProfileHash || base.StableRuntimeContractHash != baseCompiled.Fingerprint.StableRuntimeContractHash || base.TurnStableHash != baseCompiled.Fingerprint.TurnStableHash || base.StablePrefixHash != baseCompiled.Fingerprint.StablePrefixHash || base.TurnPrefixHash != baseCompiled.Fingerprint.TurnPrefixHash {
		t.Fatalf("compiler/final prefix fingerprints diverged: compiler=%#v final=%#v", baseCompiled.Fingerprint, base)
	}
	rag := fingerprint(build(compile("rag-b"), "inspect redis"))
	if base.StablePrefixHash != rag.StablePrefixHash || base.TurnPrefixHash != rag.TurnPrefixHash || base.CurrentUserInputHash != rag.CurrentUserInputHash {
		t.Fatal("RAG-only change polluted prefix or L6 hash")
	}
	if base.DynamicContextHash == rag.DynamicContextHash || base.ModelInputHash == rag.ModelInputHash {
		t.Fatal("RAG-only change did not update L5/model hash")
	}

	user := fingerprint(build(baseCompiled, "inspect postgres"))
	if base.StablePrefixHash != user.StablePrefixHash || base.TurnPrefixHash != user.TurnPrefixHash || base.ConversationHistoryHash != user.ConversationHistoryHash || base.DynamicContextHash != user.DynamicContextHash {
		t.Fatal("user-only change polluted stable prefix or L5")
	}
	if base.CurrentUserInputHash == user.CurrentUserInputHash || base.ModelInputHash == user.ModelInputHash {
		t.Fatal("user-only change did not update L6/model hash")
	}

	roleCompiled := baseCompiled
	roleCompiled.EnvelopeV2.Sections = append([]promptcompiler.PromptCompiledSection(nil), baseCompiled.EnvelopeV2.Sections...)
	roleCompiled.EnvelopeV2.Sections[1].Content += "\ncustom role change"
	role := fingerprint(build(roleCompiled, "inspect redis"))
	if base.RoleProfileHash == role.RoleProfileHash || base.StablePrefixHash == role.StablePrefixHash || base.ModelInputHash == role.ModelInputHash {
		t.Fatal("role-only change did not update L1/stable/model hash")
	}
	if base.AbsoluteSystemHash != role.AbsoluteSystemHash || base.DynamicContextHash != role.DynamicContextHash || base.CurrentUserInputHash != role.CurrentUserInputHash {
		t.Fatal("role-only change polluted unrelated layer hashes")
	}

	hostCompiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType: "host", Mode: "inspect", HostContext: "host-b",
		ExtraSections: []promptcompiler.PromptSection{{Title: "Evidence", Content: "rag-a"}},
	})
	if err != nil {
		t.Fatalf("host Compile() error = %v", err)
	}
	host := fingerprint(build(hostCompiled, "inspect redis"))
	if base.StablePrefixHash != host.StablePrefixHash || base.TurnStableHash == host.TurnStableHash || base.TurnPrefixHash == host.TurnPrefixHash || base.ModelInputHash == host.ModelInputHash {
		t.Fatal("host-only change did not isolate L3/turn/model hashes")
	}

	metadataCompiled := baseCompiled
	metadataCompiled.EnvelopeV2.DynamicContext = append([]promptcompiler.DynamicContextBundle(nil), baseCompiled.EnvelopeV2.DynamicContext...)
	metadataCompiled.EnvelopeV2.DynamicContext[0].StepID = "new-step"
	metadataCompiled.EnvelopeV2.DynamicContext[0].RetrievedAt = "new-marker"
	metadata := fingerprint(build(metadataCompiled, "inspect redis"))
	if metadata.DynamicContextHash != base.DynamicContextHash || metadata.ModelInputHash != base.ModelInputHash {
		t.Fatal("bundle retrieval metadata polluted provider-visible semantic/model input hashes")
	}

	reorderedItems := append([]ModelInputItem(nil), baseResult.Items...)
	var l5 []int
	for index, item := range reorderedItems {
		if item.Source.Layer == string(promptcompiler.LayerStepDynamicContext) {
			l5 = append(l5, index)
		}
	}
	if len(l5) < 2 {
		t.Fatal("expected at least two L5 items")
	}
	reorderedItems[l5[0]], reorderedItems[l5[1]] = reorderedItems[l5[1]], reorderedItems[l5[0]]
	reordered, err := BuildModelInputPromptFingerprint(reorderedItems)
	if err != nil {
		t.Fatalf("reordered fingerprint error = %v", err)
	}
	if reordered.DynamicContextHash == base.DynamicContextHash || reordered.ModelInputHash == base.ModelInputHash {
		t.Fatal("same-layer entry reorder did not change ordered hashes")
	}
}

func TestModelInputFingerprintIsolatesConversationHistoryAndLegacyPrefix(t *testing.T) {
	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{SessionType: "host", Mode: "inspect"})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	build := func(answer string) promptcompiler.PromptFingerprint {
		t.Helper()
		result, err := (Builder{}).Build(BuildRequest{
			Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 1,
			CurrentInputKind: CurrentInputKindContinuation, ContinuationInstruction: "continue",
			History: []Message{{Role: "user", Content: "inspect"}, {Role: "assistant", Content: answer}},
		})
		if err != nil {
			t.Fatalf("Build() error = %v", err)
		}
		fingerprint, err := BuildModelInputPromptFingerprint(result.Items)
		if err != nil {
			t.Fatalf("BuildModelInputPromptFingerprint() error = %v", err)
		}
		return fingerprint
	}
	base := build("first history answer")
	changed := build("changed history answer")
	if base.ConversationHistoryHash == changed.ConversationHistoryHash || base.ModelInputHash == changed.ModelInputHash {
		t.Fatal("L4-only change did not update conversation/model hash")
	}
	if base.StablePrefixHash != changed.StablePrefixHash || base.TurnPrefixHash != changed.TurnPrefixHash || base.DynamicContextHash != changed.DynamicContextHash || base.CurrentUserInputHash != changed.CurrentUserInputHash {
		t.Fatal("L4-only change polluted non-history hashes")
	}
	buildReasoning := func(reasoning string) promptcompiler.PromptFingerprint {
		t.Helper()
		result, err := (Builder{}).Build(BuildRequest{
			Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 1,
			CurrentInputKind: CurrentInputKindContinuation, ContinuationInstruction: "continue",
			History: []Message{{Role: "user", Content: "inspect"}, {Role: "assistant", Content: "same answer", ReasoningContent: reasoning}},
		})
		if err != nil {
			t.Fatalf("reasoning Build() error = %v", err)
		}
		fingerprint, err := BuildModelInputPromptFingerprint(result.Items)
		if err != nil {
			t.Fatalf("reasoning fingerprint error = %v", err)
		}
		return fingerprint
	}
	reasoningA := buildReasoning("reasoning-a")
	reasoningB := buildReasoning("reasoning-b")
	if reasoningA.ConversationHistoryHash == reasoningB.ConversationHistoryHash || reasoningA.ModelInputHash == reasoningB.ModelInputHash {
		t.Fatal("reasoning-only change did not update L4/model hash")
	}
	if reasoningA.StablePrefixHash != reasoningB.StablePrefixHash || reasoningA.TurnPrefixHash != reasoningB.TurnPrefixHash || reasoningA.DynamicContextHash != reasoningB.DynamicContextHash || reasoningA.CurrentUserInputHash != reasoningB.CurrentUserInputHash {
		t.Fatal("reasoning-only change polluted non-history hashes")
	}
	whitespace := build(" first history answer ")
	if whitespace.ConversationHistoryHash == base.ConversationHistoryHash || whitespace.ModelInputHash == base.ModelInputHash {
		t.Fatal("provider-visible L4 whitespace change was hidden")
	}

	legacy, err := BuildModelInputPromptFingerprint([]ModelInputItem{{ID: "legacy", ProviderRole: ProviderRoleUser, Content: "legacy"}})
	if err != nil {
		t.Fatalf("legacy fingerprint error = %v", err)
	}
	if legacy.StablePrefixHash != "" || legacy.TurnPrefixHash != "" || legacy.ModelInputHash == "" {
		t.Fatalf("legacy fingerprint = %#v, want model hash only", legacy)
	}
}
