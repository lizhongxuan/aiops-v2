package promptinput

import (
	"encoding/json"
	"strings"
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestBuildFirstStepUsesL0L6OrderAndCurrentUserLast(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	result, err := Builder{}.Build(BuildRequest{
		Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 0,
		CurrentInputKind: CurrentInputKindInitialUser, CurrentUserInput: "current command",
		History: []Message{
			{Role: "user", Content: "prior question"},
			{Role: "assistant", Content: "prior final"},
			{Role: "user", Content: "current command"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	wantPrefix := []string{
		string(promptcompiler.LayerAbsoluteSystemCore), string(promptcompiler.LayerRoleProfileCore),
		string(promptcompiler.LayerStableRuntimeContract), string(promptcompiler.LayerTurnStableFacts),
	}
	for i, want := range wantPrefix {
		if result.Items[i].Source.Layer != want {
			t.Fatalf("item[%d] layer = %q, want %q; items=%#v", i, result.Items[i].Source.Layer, want, result.Items)
		}
	}
	if result.Items[len(result.Items)-1].Source.Layer != string(promptcompiler.LayerCurrentUserInput) || result.Items[len(result.Items)-1].Content != "current command" {
		t.Fatalf("last item = %#v, want current L6 user", result.Items[len(result.Items)-1])
	}
	if countModelInputContent(result.Items, "current command") != 1 {
		t.Fatalf("current command count = %d, want 1", countModelInputContent(result.Items, "current command"))
	}
	assertLogicalLayersMonotonic(t, result.Items)
}

func TestBuildToolContinuationKeepsCausalGroupBeforeL5AndContinuationLast(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	result, err := Builder{}.Build(BuildRequest{
		Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 1,
		CurrentInputKind:        CurrentInputKindContinuation,
		ContinuationInstruction: "Continue from the completed tool results for iteration 1.",
		OpsContextCapsule:       "current ops capsule",
		Memories:                []MemoryItem{{ID: "mem-1", Scope: "session", Text: "current memory"}},
		History: []Message{
			{Role: "user", Content: "old user command"},
			{Role: "assistant", ToolCalls: []ToolCall{
				{ID: "call-a", Name: "read_a", Arguments: json.RawMessage(`{"path":"a"}`)},
				{ID: "call-b", Name: "read_b", Arguments: json.RawMessage(`{"path":"b"}`)},
			}},
			{Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-a", Content: "result-a"}},
			{Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-b", Content: "result-b"}},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	callIndex := modelInputItemIndex(result.Items, func(item ModelInputItem) bool { return len(item.ToolCalls) == 2 })
	if callIndex < 0 || result.Items[callIndex+1].ToolCallID != "call-a" || result.Items[callIndex+2].ToolCallID != "call-b" {
		t.Fatalf("causal group not adjacent: %#v", result.Items)
	}
	for i := callIndex + 3; i < len(result.Items)-1; i++ {
		if result.Items[i].Source.Layer != string(promptcompiler.LayerStepDynamicContext) {
			t.Fatalf("item[%d] after history = %#v, want L5", i, result.Items[i])
		}
	}
	if modelInputItemIndex(result.Items, func(item ModelInputItem) bool { return strings.Contains(item.Content, "current ops capsule") }) <= callIndex+2 ||
		modelInputItemIndex(result.Items, func(item ModelInputItem) bool { return strings.Contains(item.Content, "current memory") }) <= callIndex+2 {
		t.Fatalf("ops/memory context must be L5 after causal history: %#v", result.Items)
	}
	last := result.Items[len(result.Items)-1]
	if last.Source.Layer != string(promptcompiler.LayerCurrentUserInput) || last.SemanticRole != "continuation_instruction" || last.ProviderRole != ProviderRoleDeveloper {
		t.Fatalf("last item = %#v, want typed continuation L6", last)
	}
	if countModelInputContent(result.Items, "old user command") != 1 {
		t.Fatalf("old command count = %d, want history only", countModelInputContent(result.Items, "old user command"))
	}
}

func TestBuildResumedUserKeepsExistingTurnEvidenceAndMovesOnlyAnswerToL6(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	result, err := Builder{}.Build(BuildRequest{
		Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 2,
		CurrentInputKind: CurrentInputKindResumedUser, CurrentUserInput: "production",
		History: []Message{
			{Role: "user", Content: "inspect the database incident"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-db", Name: "read_database"}}},
			{Role: "tool", Content: "database evidence", ToolResult: &ToolResult{ToolCallID: "call-db", Content: "database evidence"}},
			{Role: "assistant", Content: "Which environment should I inspect?"},
			{Role: "user", Content: "production"},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	callIndex := modelInputItemIndex(result.Items, func(item ModelInputItem) bool {
		return len(item.ToolCalls) == 1 && item.ToolCalls[0].ID == "call-db"
	})
	if callIndex < 0 || result.Items[callIndex+1].ToolCallID != "call-db" {
		t.Fatalf("resumed input lost existing causal evidence: %#v", result.Items)
	}
	for _, want := range []string{"inspect the database incident", "Which environment should I inspect?"} {
		index := modelInputItemIndex(result.Items, func(item ModelInputItem) bool { return item.Content == want })
		if index < 0 || result.Items[index].Source.Layer != string(promptcompiler.LayerConversationHistory) {
			t.Fatalf("%q not retained in L4: %#v", want, result.Items)
		}
	}
	last := result.Items[len(result.Items)-1]
	if last.Content != "production" || last.Source.Layer != string(promptcompiler.LayerCurrentUserInput) || last.SemanticRole != "current_user_input" {
		t.Fatalf("last item = %#v, want resumed answer as L6", last)
	}
	if countModelInputContent(result.Items, "production") != 1 {
		t.Fatalf("resumed answer count = %d, want 1", countModelInputContent(result.Items, "production"))
	}
}

func TestBuildRejectsMissingEnvelopeV2EvenWhenCompiledContainsOne(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	_, err := Builder{}.Build(BuildRequest{
		Compiled:         compiled,
		Iteration:        0,
		CurrentInputKind: CurrentInputKindInitialUser,
		CurrentUserInput: "canonical request",
		History:          []Message{{Role: "user", Content: "canonical request"}},
	})
	if err == nil || !strings.Contains(err.Error(), "prompt envelope v2 is required") {
		t.Fatalf("Build() error = %v, want missing envelope v2 rejection", err)
	}
}

func TestBuildV2RejectsEnvelopeMismatch(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	mismatched := compiled.EnvelopeV2
	mismatched.Sections = append([]promptcompiler.PromptCompiledSection(nil), mismatched.Sections...)
	mismatched.Sections[0].Content += " changed"
	_, err := Builder{}.Build(BuildRequest{
		Envelope: mismatched, Compiled: compiled, Iteration: 0,
		CurrentInputKind: CurrentInputKindInitialUser, CurrentUserInput: "request",
		History: []Message{{Role: "user", Content: "request"}},
	})
	if err == nil || !strings.Contains(err.Error(), "does not match compiled envelope") {
		t.Fatalf("Build() error = %v, want envelope mismatch", err)
	}
}

func TestBuildV2RejectsMismatchedOrUntypedCurrentInput(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	tests := map[string]BuildRequest{
		"mismatched latest user": {
			Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 0,
			CurrentInputKind: CurrentInputKindInitialUser, CurrentUserInput: "new command",
			History: []Message{{Role: "user", Content: "old command"}},
		},
		"iteration zero continuation": {
			Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 0,
			CurrentInputKind: CurrentInputKindContinuation, ContinuationInstruction: "continue",
		},
		"untyped later continuation": {
			Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 1,
			ContinuationInstruction: "continue",
		},
	}
	for name, req := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := (Builder{}).Build(req); err == nil {
				t.Fatalf("Build() accepted invalid request: %#v", req)
			}
		})
	}
}

func TestBuildV2RejectsSystemMessageInsideToolCausalGroup(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	_, err := Builder{}.Build(BuildRequest{
		Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 1,
		CurrentInputKind: CurrentInputKindContinuation, ContinuationInstruction: "continue",
		History: []Message{
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-1", Name: "read"}}},
			{Role: "system", Content: "must not be moved around the pending tool result"},
			{Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-1", Content: "ok"}},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "conversation causal order") {
		t.Fatalf("Build() error = %v, want causal-order rejection", err)
	}
}

func TestBuildV2MovesTypedToolProgressAfterCompletedCausalGroup(t *testing.T) {
	compiled := compiledPromptV2ForCausalTest(t)
	result, err := Builder{}.Build(BuildRequest{
		Envelope: compiled.EnvelopeV2, Compiled: compiled, Iteration: 1,
		CurrentInputKind: CurrentInputKindContinuation, ContinuationInstruction: "continue",
		History: []Message{
			{Role: "user", Content: "stream logs"},
			{Role: "assistant", ToolCalls: []ToolCall{{ID: "call-stream", Name: "read_stream"}}},
			{Role: "system", Content: "partial stream evidence", ContextKind: ContextKindToolProgress, ContextRef: "call-stream"},
			{Role: "tool", ToolResult: &ToolResult{ToolCallID: "call-stream", Content: "complete stream evidence"}},
		},
	})
	if err != nil {
		t.Fatalf("Build() error = %v", err)
	}
	callIndex := modelInputItemIndex(result.Items, func(item ModelInputItem) bool {
		return len(item.ToolCalls) == 1 && item.ToolCalls[0].ID == "call-stream"
	})
	if callIndex < 0 || result.Items[callIndex+1].ToolCallID != "call-stream" {
		t.Fatalf("tool causal group is not adjacent: %#v", result.Items)
	}
	progressIndex := modelInputItemIndex(result.Items, func(item ModelInputItem) bool { return item.Content == "partial stream evidence" })
	if progressIndex <= callIndex+1 || result.Items[progressIndex].Source.Layer != string(promptcompiler.LayerStepDynamicContext) {
		t.Fatalf("typed tool progress not moved to L5: %#v", result.Items)
	}
}

func TestModelInputCausalOrderRejectsInvalidToolSequences(t *testing.T) {
	assistantCall := func(ids ...string) ModelInputItem {
		item := ModelInputItem{ID: "assistant", ProviderRole: ProviderRoleAssistant, Source: ModelInputSource{Layer: string(promptcompiler.LayerConversationHistory)}}
		for _, id := range ids {
			item.ToolCalls = append(item.ToolCalls, ModelInputToolCall{ID: id, Name: "read", Arguments: json.RawMessage(`{}`)})
		}
		return item
	}
	toolResult := func(id string) ModelInputItem {
		return ModelInputItem{ID: "result-" + id, ProviderRole: ProviderRoleTool, ToolCallID: id, ToolResult: &ModelInputToolResult{ToolCallID: id, Content: "ok"}, Source: ModelInputSource{Layer: string(promptcompiler.LayerConversationHistory)}}
	}
	tests := map[string][]ModelInputItem{
		"orphan result":       {toolResult("missing")},
		"result before call":  {toolResult("call-1"), assistantCall("call-1")},
		"duplicate result":    {assistantCall("call-1"), toolResult("call-1"), toolResult("call-1")},
		"duplicate call":      {assistantCall("call-1"), toolResult("call-1"), assistantCall("call-1"), toolResult("call-1")},
		"interleaved context": {assistantCall("call-1"), {ID: "context", ProviderRole: ProviderRoleSystem, Content: "bad gap"}, toolResult("call-1")},
		"unresolved call":     {assistantCall("call-1")},
	}
	for name, items := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateModelInputCausalOrder(items); err == nil {
				t.Fatalf("ValidateModelInputCausalOrder() accepted %#v", items)
			}
		})
	}
	valid := []ModelInputItem{assistantCall("call-a", "call-b"), toolResult("call-a"), toolResult("call-b")}
	if err := ValidateModelInputCausalOrder(valid); err != nil {
		t.Fatalf("valid parallel causal group error = %v", err)
	}
}

func compiledPromptV2ForCausalTest(t *testing.T) promptcompiler.CompiledPrompt {
	t.Helper()
	compiled, err := promptcompiler.NewCompiler().Compile(promptcompiler.CompileContext{
		SessionType: "host", Mode: "inspect", Profile: promptcompiler.PromptProfileEvidenceRCA,
		ExtraSections: []promptcompiler.PromptSection{{Title: "Dynamic Evidence", Content: "latest dynamic evidence"}},
	})
	if err != nil {
		t.Fatalf("Compile() error = %v", err)
	}
	return compiled
}

func countModelInputContent(items []ModelInputItem, text string) int {
	count := 0
	for _, item := range items {
		if strings.Contains(item.Content, text) {
			count++
		}
	}
	return count
}

func modelInputItemIndex(items []ModelInputItem, match func(ModelInputItem) bool) int {
	for i, item := range items {
		if match(item) {
			return i
		}
	}
	return -1
}

func assertLogicalLayersMonotonic(t *testing.T, items []ModelInputItem) {
	t.Helper()
	previous := -1
	for _, item := range items {
		rank, ok := modelInputLogicalLayerRank(item.Source.Layer)
		if !ok {
			continue
		}
		if rank < previous {
			t.Fatalf("logical layers not monotonic at %#v", item)
		}
		previous = rank
	}
}
