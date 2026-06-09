package agentmgr

import "testing"

func TestVerifierAgentRequestRequiresFreshContextInputs(t *testing.T) {
	req := VerificationAgentRequest{
		OriginalTask:     "verify the implementation",
		ChangeSummary:    "added scheduling primitives",
		VerificationGoal: "falsify the claimed behavior",
		ArtifactRefs:     []string{"git://diff"},
	}
	if err := req.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}

	req.ImplementerTranscript = "hidden chain from implementer"
	if err := req.Validate(); err == nil {
		t.Fatal("expected verifier request to reject inherited implementer transcript")
	}
}

func TestVerifierAgentReportRequiresVerdictAndEvidence(t *testing.T) {
	report := VerificationAgentReport{
		VerifierAgentID: "verifier-1",
		Verdict:         VerificationVerdictPass,
		Summary:         "no counterexample found",
		EvidenceRefs:    []string{"store://verification/1"},
	}
	if err := report.Validate(); err != nil {
		t.Fatalf("unexpected validation error: %v", err)
	}
	if err := (VerificationAgentReport{VerifierAgentID: "verifier-1"}).Validate(); err == nil {
		t.Fatal("expected missing verdict/evidence validation error")
	}
}
