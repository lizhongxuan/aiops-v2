package appui

import (
	"strings"
	"testing"
	"time"
)

func TestWebLearnDecisionTriggersOnlyForUnknownVersionedOpsKnowledge(t *testing.T) {
	decision := DecideWebLearn(WebLearnDecisionInput{
		UserInput:          "Redis 7.2 latency doctor 命令输出里 latest-event 是什么意思?",
		LocalContextEnough: false,
		UserDisabledWeb:    false,
		OfficialSourceHint: "https://redis.io/docs/latest/",
	})
	if !decision.ShouldSearch || decision.Reason != "official_external_knowledge_needed" {
		t.Fatalf("decision = %#v, want WebLearn trigger", decision)
	}

	disabled := DecideWebLearn(WebLearnDecisionInput{
		UserInput:          "Redis latency doctor 是什么意思?",
		LocalContextEnough: false,
		UserDisabledWeb:    true,
		OfficialSourceHint: "https://redis.io/docs/latest/",
	})
	if disabled.ShouldSearch || disabled.Reason != "user_disabled_web" {
		t.Fatalf("disabled decision = %#v, want user_disabled_web skip", disabled)
	}

	enough := DecideWebLearn(WebLearnDecisionInput{
		UserInput:          "Redis latency doctor 是什么意思?",
		LocalContextEnough: true,
		OfficialSourceHint: "https://redis.io/docs/latest/",
	})
	if enough.ShouldSearch || enough.Reason != "local_context_enough" {
		t.Fatalf("local context decision = %#v, want local_context_enough skip", enough)
	}
}

func TestNormalizeWebLearnEvidenceMarksExternalKnowledgeAndBoundsExcerpt(t *testing.T) {
	longExcerpt := strings.Repeat("Redis official latency guidance. ", 80)
	ev := NormalizeWebLearnEvidence(WebLearnEvidence{
		ID:              " web-redis-latency ",
		Query:           " redis latency doctor ",
		SourceURL:       " https://redis.io/docs/latest/commands/latency-doctor/ ",
		SourceTitle:     " LATENCY DOCTOR ",
		SourceKind:      " official_docs ",
		Product:         " Redis ",
		Version:         " 7.2 ",
		RetrievedAt:     time.Date(2026, 6, 23, 13, 0, 0, 0, time.UTC),
		RelevantExcerpt: longExcerpt,
		Applicability:   " matches Redis 7.2 command semantics ",
		Confidence:      " HIGH ",
	})
	if ev.Kind != "external_knowledge" {
		t.Fatalf("Kind = %q, want external_knowledge", ev.Kind)
	}
	if ev.SourceKind != "official_docs" || ev.Confidence != "high" || ev.Product != "Redis" || ev.Version != "7.2" {
		t.Fatalf("normalized evidence = %#v", ev)
	}
	if len(ev.RelevantExcerpt) > maxWebLearnExcerptChars {
		t.Fatalf("RelevantExcerpt length = %d, want bounded <= %d", len(ev.RelevantExcerpt), maxWebLearnExcerptChars)
	}
}
