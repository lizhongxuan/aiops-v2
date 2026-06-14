package hostops

import (
	"errors"
	"strings"
	"testing"

	"aiops-v2/internal/opssemantic"
)

func TestHostAgentContextBuilderIncludesOnlyAssignedHostAndPlanStep(t *testing.T) {
	ctx, trace, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
		MissionID:       "mission-ctx",
		ParentAgentID:   "manager-ctx",
		HostAgentID:     "child-ctx",
		SessionID:       "session-ctx",
		HostID:          "host-a",
		HostDisplayName: "general host",
		PlanStep: PlanStep{
			ID:               "step-a",
			Title:            "Inspect assigned host",
			Summary:          "Collect generic service and process health.",
			HostIDs:          []string{"host-a"},
			RiskLevel:        opssemantic.RiskReadOnly,
			EvidenceRequired: []string{"command_result", "artifact_ref"},
		},
		Goal:               "collect current host state",
		Constraints:        []string{"do not mutate host state"},
		AllowedToolScopes:  []string{"host_command"},
		AllowedSkillScopes: []string{"diagnostic"},
		AllowedMCPScope:    []string{"read_only_artifacts"},
	})
	if err != nil {
		t.Fatalf("BuildHostAgentRuntimeContext error = %v", err)
	}
	if ctx.MissionID != "mission-ctx" || ctx.ParentAgentID != "manager-ctx" || ctx.HostAgentID != "child-ctx" || ctx.SessionID != "session-ctx" {
		t.Fatalf("context identifiers = %#v", ctx)
	}
	if ctx.Host.ID != "host-a" || ctx.PlanStep.ID != "step-a" || ctx.Goal != "collect current host state" {
		t.Fatalf("context binding = %#v", ctx)
	}
	if len(ctx.EvidenceRequirements) != 2 || len(ctx.AllowedToolScopes) != 1 || len(ctx.AllowedSkillScopes) != 1 || len(ctx.AllowedMCPScope) != 1 {
		t.Fatalf("context scopes/evidence = %#v", ctx)
	}
	if ctx.CompletionContract == "" {
		t.Fatalf("CompletionContract is empty")
	}
	if trace.SourceID == "" || len(trace.Included) == 0 {
		t.Fatalf("trace = %#v, want source id and included refs", trace)
	}
}

func TestHostAgentContextBuilderExcludesOtherHostPrivateTranscript(t *testing.T) {
	_, trace, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
		MissionID:   "mission-ctx",
		HostAgentID: "child-ctx",
		SessionID:   "session-ctx",
		HostID:      "host-a",
		PlanStep:    PlanStep{ID: "step-a", HostIDs: []string{"host-a"}, RiskLevel: opssemantic.RiskReadOnly},
		Goal:        "inspect assigned host",
		ContextRefs: []ContextRef{
			{ID: "ref-a", ScopeHostID: "host-a", Kind: "transcript", Summary: "assigned host summary"},
			{ID: "ref-b", ScopeHostID: "host-b", Kind: "private_transcript", Content: "other host raw command output"},
		},
	})
	if err != nil {
		t.Fatalf("BuildHostAgentRuntimeContext error = %v", err)
	}
	if len(trace.Excluded) != 1 || trace.Excluded[0].ID != "ref-b" || trace.Excluded[0].Reason != ContextDecisionScopeViolation {
		t.Fatalf("trace.Excluded = %#v, want other host private transcript excluded by scope", trace.Excluded)
	}
}

func TestHostAgentContextBuilderRedactsSecrets(t *testing.T) {
	ctx, _, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
		MissionID:   "mission-ctx",
		HostAgentID: "child-ctx",
		SessionID:   "session-ctx",
		HostID:      "host-a",
		PlanStep:    PlanStep{ID: "step-a", HostIDs: []string{"host-a"}, RiskLevel: opssemantic.RiskReadOnly},
		Goal:        "inspect token=plain-secret and password=plain-secret",
		Constraints: []string{"Bearer plain-secret", "cookie: session=plain-secret"},
		ContextRefs: []ContextRef{{ID: "ref-a", ScopeHostID: "host-a", Content: "private key plain-secret"}},
	})
	if err != nil {
		t.Fatalf("BuildHostAgentRuntimeContext error = %v", err)
	}
	rendered := ctx.Goal + "\n" + strings.Join(ctx.Constraints, "\n")
	for _, ref := range ctx.ContextRefs {
		rendered += "\n" + ref.Content + "\n" + ref.Summary
	}
	for _, forbidden := range []string{"plain-secret", "Bearer plain-secret", "password=plain-secret", "private key plain-secret"} {
		if strings.Contains(rendered, forbidden) {
			t.Fatalf("context contains unredacted sensitive value %q:\n%s", forbidden, rendered)
		}
	}
	if !strings.Contains(rendered, "[REDACTED") {
		t.Fatalf("context = %q, want redaction marker", rendered)
	}
}

func TestHostAgentContextBuilderUsesRefsForLargeContext(t *testing.T) {
	large := strings.Repeat("generic log line\n", 80)
	ctx, trace, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
		MissionID:   "mission-ctx",
		HostAgentID: "child-ctx",
		SessionID:   "session-ctx",
		HostID:      "host-a",
		PlanStep:    PlanStep{ID: "step-a", HostIDs: []string{"host-a"}, RiskLevel: opssemantic.RiskReadOnly},
		Goal:        "inspect assigned host",
		ContextRefs: []ContextRef{{ID: "large-ref", ScopeHostID: "host-a", Kind: "artifact", Content: large}},
	})
	if err != nil {
		t.Fatalf("BuildHostAgentRuntimeContext error = %v", err)
	}
	if len(ctx.ContextRefs) != 1 || ctx.ContextRefs[0].Content != "" || ctx.ContextRefs[0].ArtifactRef == "" || ctx.ContextRefs[0].Digest == "" || ctx.ContextRefs[0].Summary == "" {
		t.Fatalf("large context ref = %#v, want summary/digest/artifact ref without raw content", ctx.ContextRefs)
	}
	if len(trace.Externalized) != 1 || trace.Externalized[0].ID != "large-ref" {
		t.Fatalf("trace.Externalized = %#v, want large-ref externalized", trace.Externalized)
	}
}

func TestHostAgentContextBuilderRequiresMissionHostStepAndGoal(t *testing.T) {
	_, _, err := BuildHostAgentRuntimeContext(HostAgentContextBuildInput{
		HostID: "host-a",
		PlanStep: PlanStep{
			ID:      "step-a",
			HostIDs: []string{"host-a"},
		},
		Goal: "inspect assigned host",
	})
	if !errors.Is(err, ErrInvalidHostAgentContext) {
		t.Fatalf("err = %v, want ErrInvalidHostAgentContext", err)
	}
}
