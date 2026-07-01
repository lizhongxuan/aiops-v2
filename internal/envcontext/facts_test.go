package envcontext

import (
	"strings"
	"testing"
	"time"
)

func TestResolveEnvironmentFactsBuildsTargetRefFromExplicitIPMention(t *testing.T) {
	resolution := ResolveEnvironmentFacts(ResolverInput{
		Input: "@10.0.0.1 检查 systemd 服务为什么失败",
		Now:   fixedTime(),
	})

	if len(resolution.TargetRefs) != 1 {
		t.Fatalf("TargetRefs = %#v, want one explicit host target", resolution.TargetRefs)
	}
	target := resolution.TargetRefs[0]
	if target.Kind != TargetKindHost || target.Address != "10.0.0.1" || target.ID != "host:10.0.0.1" {
		t.Fatalf("target = %#v, want host:10.0.0.1", target)
	}
	if !resolution.ExecutionAllowed {
		t.Fatalf("ExecutionAllowed = false, want true without conflicts: %#v", resolution)
	}
	if got := resolution.CompactContext(); !strings.Contains(got, "ConfirmedFacts") || !strings.Contains(got, "host_identity=10.0.0.1") {
		t.Fatalf("compact context missing confirmed host fact:\n%s", got)
	}
}

func TestResolveEnvironmentFactsDetectsUserHostAndCorootServiceConflict(t *testing.T) {
	resolution := ResolveEnvironmentFacts(ResolverInput{
		Input: "@10.0.0.1 @Coroot 分析 checkout 服务异常",
		CorootFacts: []EnvironmentFact{
			{
				Kind:       FactKindTopology,
				Subject:    "service:checkout",
				Value:      "host:10.0.0.2",
				Source:     FactSourceCoroot,
				SourceRef:  "coroot:checkout",
				Confidence: FactConfidenceObserved,
			},
		},
		Now: fixedTime(),
	})

	if resolution.ExecutionAllowed {
		t.Fatalf("ExecutionAllowed = true, want read-only conflict: %#v", resolution)
	}
	if !resolution.RequiresClarification || resolution.ReadOnlyReason != "target_conflict_requires_clarification" {
		t.Fatalf("resolution = %#v, want clarification/read-only conflict", resolution)
	}
	if len(resolution.ConflictFacts) != 1 {
		t.Fatalf("ConflictFacts = %#v, want one conflict", resolution.ConflictFacts)
	}
	conflict := resolution.ConflictFacts[0]
	if conflict.Subject != "service:checkout" || conflict.Kind != FactKindTopology {
		t.Fatalf("conflict = %#v, want service topology conflict", conflict)
	}
	if got := resolution.CompactContext(); !strings.Contains(got, "ConflictFacts") || !strings.Contains(got, "target_conflict") {
		t.Fatalf("compact context missing conflict:\n%s", got)
	}
}

func TestResolveEnvironmentFactsKeepsWebLearnVersionAsExternalKnowledge(t *testing.T) {
	resolution := ResolveEnvironmentFacts(ResolverInput{
		Input: "Redis 7 latency doctor 如何使用",
		WebLearnFacts: []EnvironmentFact{
			{
				Kind:        FactKindVersion,
				Subject:     "redis",
				Value:       "7.2 docs say LATENCY DOCTOR is available",
				Source:      FactSourceWebLearn,
				SourceRef:   "https://redis.io/docs/latest/operate/oss_and_stack/management/optimization/latency/",
				Confidence:  FactConfidenceInferred,
				CollectedAt: time.Date(2026, 6, 23, 9, 0, 0, 0, time.UTC),
			},
		},
		Now: fixedTime(),
	})

	for _, fact := range resolution.ConfirmedFacts {
		if fact.Kind == FactKindVersion {
			t.Fatalf("confirmed facts must not treat WebLearn version as environment version: %#v", resolution.ConfirmedFacts)
		}
	}
	if len(resolution.InferredFacts) != 1 || resolution.InferredFacts[0].Kind != FactKindExternalKnowledge {
		t.Fatalf("InferredFacts = %#v, want WebLearn external knowledge", resolution.InferredFacts)
	}
	if got := resolution.CompactContext(); !strings.Contains(got, "ExternalKnowledge") || strings.Contains(got, "ConfirmedFacts:\n- version=7.2") {
		t.Fatalf("compact context mixed external knowledge with confirmed facts:\n%s", got)
	}
}
