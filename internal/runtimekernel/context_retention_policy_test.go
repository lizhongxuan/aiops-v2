package runtimekernel

import (
	"testing"

	"aiops-v2/internal/promptcompiler"
)

func TestContextRetentionPolicyKeepsP0OriginalWithinBudget(t *testing.T) {
	decision := DefaultContextRetentionPolicy().Decide(ContextRetentionSection{
		ID:             "host_agent.binding.v1",
		TokensEstimate: 32,
	})
	if decision.Action != promptcompiler.CompactActionKeptOriginal || decision.RetentionRank != promptcompiler.RetentionRankP0 {
		t.Fatalf("decision = %#v, want kept P0 host binding", decision)
	}
}

func TestContextRetentionPolicyCompactsP1WithRequiredFields(t *testing.T) {
	decision := DefaultContextRetentionPolicy().Decide(ContextRetentionSection{
		ID:             "host_task.context",
		TokensEstimate: 4096,
		RequiredFields: []string{"goal", "constraints", "evidenceRequirements", "sourceMessageId"},
	})
	if decision.Action != promptcompiler.CompactActionSummarized || decision.ValidationStatus != "valid" {
		t.Fatalf("decision = %#v, want summarized valid P1 host task", decision)
	}
}

func TestContextRetentionPolicyExternalizesP4RawToolOutput(t *testing.T) {
	decision := DefaultContextRetentionPolicy().Decide(ContextRetentionSection{
		ID:             "tool.result.command_output",
		TokensEstimate: 12000,
		RawOutput:      true,
		ArtifactRef:    "artifact://tool-output/summary",
	})
	if decision.Action != promptcompiler.CompactActionExternalized || decision.ArtifactRef == "" {
		t.Fatalf("decision = %#v, want externalized raw output with ref", decision)
	}
}

func TestContextRetentionPolicyBlocksWhenP0CannotFit(t *testing.T) {
	policy := DefaultContextRetentionPolicy()
	policy.RankBudgets[promptcompiler.RetentionRankP0] = 4
	decision := policy.Decide(ContextRetentionSection{
		ID:             "host_agent.binding.v1",
		TokensEstimate: 128,
	})
	if decision.Action != promptcompiler.CompactActionBlocked || decision.BlockingReason != "p0_over_budget" {
		t.Fatalf("decision = %#v, want p0_over_budget block", decision)
	}
}

func TestContextRetentionPolicyRejectsCompactSummaryMissingRequiredFields(t *testing.T) {
	decision := DefaultContextRetentionPolicy().Decide(ContextRetentionSection{
		ID:             "host_task.context",
		TokensEstimate: 4096,
	})
	if decision.Action != promptcompiler.CompactActionBlocked || decision.BlockingReason != "p1_required_field_missing" {
		t.Fatalf("decision = %#v, want p1 required field block", decision)
	}
}

func TestApplyPromptSectionRetentionPolicyAnnotatesPromptTrace(t *testing.T) {
	sections := []promptcompiler.PromptSectionTrace{
		{ID: "host_agent.assigned_subtask.v1", TokensEstimate: 2048, CompactAction: promptcompiler.CompactActionKeptOriginal},
		{ID: "tool.result.command_output", TokensEstimate: 4096, CompactAction: promptcompiler.CompactActionKeptOriginal},
	}

	annotated, decisions, err := ApplyPromptSectionRetentionPolicy(sections, DefaultContextRetentionPolicy())
	if err != nil {
		t.Fatalf("ApplyPromptSectionRetentionPolicy() error = %v", err)
	}
	if len(decisions) != 2 {
		t.Fatalf("decisions = %d, want 2", len(decisions))
	}
	if annotated[0].CompactAction != promptcompiler.CompactActionSummarized {
		t.Fatalf("assigned subtask compact action = %q, want summarized", annotated[0].CompactAction)
	}
	if annotated[0].SourceRef == "" {
		t.Fatal("expected assigned subtask source ref")
	}
	if annotated[1].CompactAction != promptcompiler.CompactActionExternalized {
		t.Fatalf("raw command compact action = %q, want externalized", annotated[1].CompactAction)
	}
	if annotated[1].SourceRef == "" {
		t.Fatal("expected command output source ref")
	}
}

func TestApplyPromptSectionRetentionPolicyBlocksOversizedP0(t *testing.T) {
	sections := []promptcompiler.PromptSectionTrace{
		{ID: "host_agent.binding.v1", TokensEstimate: 9999},
	}

	if _, _, err := ApplyPromptSectionRetentionPolicy(sections, DefaultContextRetentionPolicy()); err == nil {
		t.Fatal("expected oversized P0 prompt section to block")
	}
}
