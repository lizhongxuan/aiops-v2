package runtimekernel

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aiops-v2/internal/promptcompiler"
)

func TestContextRetentionEvalPreservesHostRuntimeAndExternalizesLargeOutputs(t *testing.T) {
	policy := DefaultContextRetentionPolicy()

	binding := policy.Decide(ContextRetentionSection{
		ID:             "host_agent.binding.v1",
		TokensEstimate: 96,
	})
	if binding.Action != promptcompiler.CompactActionKeptOriginal ||
		binding.RetentionRank != promptcompiler.RetentionRankP0 ||
		binding.RetentionClass != promptcompiler.RetentionClassMustKeep {
		t.Fatalf("binding decision = %#v, want kept P0 host binding", binding)
	}

	task := policy.Decide(ContextRetentionSection{
		ID:             "host_task.context",
		TokensEstimate: 4096,
		RequiredFields: []string{
			"goal",
			"manager_intent",
			"constraints",
			"evidence_requirements",
			"completion_criteria",
			"source_message_id",
		},
	})
	if task.Action != promptcompiler.CompactActionSummarized ||
		task.RetentionRank != promptcompiler.RetentionRankP1 ||
		task.ValidationStatus != "valid" {
		t.Fatalf("task decision = %#v, want valid summarized P1 host task", task)
	}

	rawOutput := policy.Decide(ContextRetentionSection{
		ID:             "tool.result.command_output",
		TokensEstimate: 12000,
		RawOutput:      true,
		ArtifactRef:    "artifact://mission-eval/host-alpha/tool-output/summary",
	})
	if rawOutput.Action != promptcompiler.CompactActionExternalized ||
		rawOutput.RetentionRank != promptcompiler.RetentionRankP4 ||
		rawOutput.ArtifactRef == "" {
		t.Fatalf("raw output decision = %#v, want externalized P4 raw output", rawOutput)
	}
}

func TestContextRetentionEvalBlocksUnsafeCompactionContracts(t *testing.T) {
	policy := DefaultContextRetentionPolicy()
	policy.RankBudgets[promptcompiler.RetentionRankP0] = 8

	oversizedBinding := policy.Decide(ContextRetentionSection{
		ID:             "host_agent.binding.v1",
		TokensEstimate: 512,
	})
	if oversizedBinding.Action != promptcompiler.CompactActionBlocked ||
		oversizedBinding.BlockingReason != "p0_over_budget" {
		t.Fatalf("oversized binding decision = %#v, want p0_over_budget block", oversizedBinding)
	}

	missingRequiredFields := DefaultContextRetentionPolicy().Decide(ContextRetentionSection{
		ID:             "host_task.context",
		TokensEstimate: 4096,
	})
	if missingRequiredFields.Action != promptcompiler.CompactActionBlocked ||
		missingRequiredFields.BlockingReason != "p1_required_field_missing" {
		t.Fatalf("missing required field decision = %#v, want p1_required_field_missing block", missingRequiredFields)
	}

	unknown := DefaultContextRetentionPolicy().Decide(ContextRetentionSection{
		ID:             "generic.optional.note",
		TokensEstimate: 4096,
	})
	if unknown.Action != promptcompiler.CompactActionSummarized ||
		unknown.RetentionRank != promptcompiler.RetentionRankP3 {
		t.Fatalf("unknown section decision = %#v, want summarized P3 fallback", unknown)
	}
}

func TestContextRetentionEvalCompactSummaryCanResumeHostTaskWithoutRawLog(t *testing.T) {
	rawSensitive := "eval-sensitive-value"
	rawLog := strings.Repeat("generic diagnostic line\n", 120) + "token=" + rawSensitive
	summary := CompactSummaryV1{
		SchemaVersion: CompactSummarySchemaVersionV1,
		UserGoal:      "complete the assigned generic host operation",
		LatestUserMessages: []CompactSummaryMessageRefV1{{
			TurnID: "turn-eval-latest",
			Quote:  "continue the current host-scoped task",
		}},
		ActiveConstraints: []string{"operate only on the retained bound host"},
		CurrentTask: CompactSummaryCurrentTaskV1{
			Description:  "continue host task after compaction using retained binding and artifact refs",
			SourceTurnID: "turn-eval-latest",
		},
		ConfirmedFacts: []CompactSummaryFactV1{{
			Statement: "host binding remains host-alpha for the active plan step",
			SourceRef: "host_agent.binding.v1",
		}},
		Artifacts: []CompactSummaryArtifactV1{{
			ID:        "artifact-tool-output-summary",
			SourceRef: "artifact://mission-eval/host-alpha/tool-output/summary",
			Summary:   "large tool output externalized; inspect artifact by reference when needed",
		}},
		PendingEvidence: []CompactSummaryPendingItemV1{{
			ID:        "evidence-required",
			SourceRef: "host_agent.report_contract.v1",
		}},
		PlanState: CompactSummaryPlanStateV1{
			Status:      "running",
			CurrentStep: "step-eval-current",
		},
		NextStep: CompactSummaryNextStepV1{
			Action:          "resume the host-scoped task and collect required evidence refs",
			SourceTurnID:    "turn-eval-latest",
			RecentUserQuote: "continue the current host-scoped task",
		},
	}
	payload, err := json.Marshal(summary)
	if err != nil {
		t.Fatalf("Marshal summary error = %v", err)
	}
	if strings.Contains(string(payload), rawLog) || strings.Contains(string(payload), rawSensitive) {
		t.Fatalf("compact summary leaked raw log or sensitive value: %s", string(payload))
	}

	parsed, err := ParseCompactSummaryV1(string(payload))
	if err != nil {
		t.Fatalf("ParseCompactSummaryV1() error = %v", err)
	}
	if parsed.NextStep.Action == "" ||
		parsed.NextStep.SourceTurnID != "turn-eval-latest" ||
		len(parsed.Artifacts) != 1 ||
		parsed.Artifacts[0].SourceRef == "" {
		t.Fatalf("parsed summary = %#v, want resumable next step with artifact ref", parsed)
	}

	boundary := NewCompactBoundaryMessage(CompactBoundaryInput{
		SegmentID:          "segment-eval",
		CompactedTurnStart: 1,
		CompactedTurnEnd:   12,
		PreservedTailCount: 3,
		CreatedAt:          time.Date(2026, 6, 12, 10, 0, 0, 0, time.UTC),
	})
	meta, ok := CompactBoundaryMetadataFromMessage(boundary)
	if !ok {
		t.Fatalf("CompactBoundaryMetadataFromMessage() ok = false")
	}
	if meta.SummarySchemaVersion != CompactSummarySchemaVersionV1 ||
		meta.CompactedTurnRange.Start != 1 ||
		meta.CompactedTurnRange.End != 12 ||
		meta.PreservedTailCount != 3 {
		t.Fatalf("boundary metadata = %#v, want compact range and summary schema", meta)
	}
}
