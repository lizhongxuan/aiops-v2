package appui

import (
	"os"
	"strings"
	"testing"

	"aiops-v2/internal/hostops"
	"aiops-v2/internal/runtimekernel"
)

func TestCodexRolloutPGQuestionDoesNotEnterCorootWithoutMention(t *testing.T) {
	raw, err := os.ReadFile("../../output/playwright/codex-rollout-compare-inapp/submitted-prompt.txt")
	if err != nil {
		t.Fatalf("read rollout prompt: %v", err)
	}
	input := string(raw)
	if strings.Contains(input, "@Coroot") {
		t.Fatalf("fixture unexpectedly contains @Coroot")
	}
	evidence := ExtractUserEvidence(input)
	route := BuildChatRuntimeRoute(input, nil, evidence)
	if route.AllowsCorootRCA {
		t.Fatalf("AllowsCorootRCA = true, want false for no @Coroot")
	}
	if route.Mode != ChatRouteEvidenceRCA && route.Mode != ChatRouteAdvisory {
		t.Fatalf("route mode = %q, want advisory/evidence without host binding", route.Mode)
	}

	req := newTestTurnRequestForCodexRoute(input)
	applyChatRuntimeRouteMetadata(req, route)
	applyChatRuntimeToolSurfaceMetadata(req, route)
	applyChatRuntimeRouteHostBinding(req, route, nil)
	if got := req.Metadata["aiops.coroot.explicitRCA"]; got != "false" {
		t.Fatalf("aiops.coroot.explicitRCA = %q, want false", got)
	}
	if got := req.Metadata["aiops.tool.corootRCAAllowed"]; got != "false" {
		t.Fatalf("aiops.tool.corootRCAAllowed = %q, want false", got)
	}
	if req.HostID != "" {
		t.Fatalf("HostID = %q, want empty", req.HostID)
	}
}

func TestCodexRolloutPGQuestionWithCorootMentionAllowsCorootRCA(t *testing.T) {
	input := "@Coroot checkout 服务异常，请结合依赖链分析根因"
	route := BuildChatRuntimeRoute(input, []hostops.HostMention{}, ExtractUserEvidence(input))
	if !route.AllowsCorootRCA {
		t.Fatalf("AllowsCorootRCA = false, want true for explicit @Coroot")
	}
}

func newTestTurnRequestForCodexRoute(input string) *runtimekernel.TurnRequest {
	return &runtimekernel.TurnRequest{
		Input:    input,
		Metadata: map[string]string{},
	}
}
