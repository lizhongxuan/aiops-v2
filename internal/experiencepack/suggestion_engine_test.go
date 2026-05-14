package experiencepack

import "testing"

func TestSuggestionEngineBlocksWhenThresholdsMissing(t *testing.T) {
	result := EvaluateChatSuggestion(SuggestionInput{CommandCount: 5, LLMOperationalValueScore: 0.9, Outcome: "success", RedactionStatus: "redacted", MemoryGraphWritable: true})
	if result.Visible {
		t.Fatal("command count below 6 should not show suggestions")
	}
	result = EvaluateChatSuggestion(SuggestionInput{CommandCount: 6, LLMOperationalValueScore: 0.2, Outcome: "success", RedactionStatus: "redacted", MemoryGraphWritable: true})
	if result.Visible {
		t.Fatal("low operational value should not show suggestions")
	}
	result = EvaluateChatSuggestion(SuggestionInput{CommandCount: 6, LLMOperationalValueScore: 0.8, Outcome: "success", RedactionStatus: "raw", MemoryGraphWritable: true})
	if result.Visible {
		t.Fatal("unredacted trajectory should not show suggestions")
	}
}

func TestSuggestionEngineReturnsExpectedButtons(t *testing.T) {
	result := EvaluateChatSuggestion(SuggestionInput{CommandCount: 6, LLMOperationalValueScore: 0.8, Outcome: "success", RedactionStatus: "redacted", MemoryGraphWritable: true, ReusableStepCount: 6})
	if !result.Visible || len(result.Suggestions) != 2 {
		t.Fatalf("expected generate pack and runner suggestions, got %#v", result)
	}
	result = EvaluateChatSuggestion(SuggestionInput{CommandCount: 6, LLMOperationalValueScore: 0.8, Outcome: "failed", RedactionStatus: "redacted", MemoryGraphWritable: true, MatchedPackID: "pack"})
	if !result.Visible || result.Suggestions[0].Type != "evolve_current_experience" {
		t.Fatalf("expected evolve suggestion, got %#v", result)
	}
}
