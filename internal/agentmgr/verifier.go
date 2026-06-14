package agentmgr

import (
	"fmt"
	"strings"
)

type VerificationAgentRequest struct {
	OriginalTask             string   `json:"originalTask"`
	ChangeSummary            string   `json:"changeSummary,omitempty"`
	DiagnosisSummary         string   `json:"diagnosisSummary,omitempty"`
	VerificationGoal         string   `json:"verificationGoal"`
	ArtifactRefs             []string `json:"artifactRefs,omitempty"`
	AllowedEvidenceKinds     []string `json:"allowedEvidenceKinds,omitempty"`
	ImplementerTranscript    string   `json:"-"`
	ImplementerTranscriptRef string   `json:"-"`
}

func (r VerificationAgentRequest) Validate() error {
	if strings.TrimSpace(r.ImplementerTranscript) != "" || strings.TrimSpace(r.ImplementerTranscriptRef) != "" {
		return fmt.Errorf("verification request must use fresh context without implementer transcript")
	}
	var missing []string
	if strings.TrimSpace(r.OriginalTask) == "" {
		missing = append(missing, "originalTask")
	}
	if strings.TrimSpace(r.ChangeSummary) == "" {
		missing = append(missing, "changeSummary")
	}
	if strings.TrimSpace(r.VerificationGoal) == "" {
		missing = append(missing, "verificationGoal")
	}
	if len(r.ArtifactRefs) == 0 {
		missing = append(missing, "artifactRefs")
	}
	if len(missing) > 0 {
		return fmt.Errorf("verification request missing fields: %s", strings.Join(missing, ", "))
	}
	return nil
}

type VerificationVerdict string

const (
	VerificationVerdictPass    VerificationVerdict = "PASS"
	VerificationVerdictPartial VerificationVerdict = "PARTIAL"
	VerificationVerdictFail    VerificationVerdict = "FAIL"
)

type VerificationAgentReport struct {
	VerifierAgentID string              `json:"verifierAgentId"`
	Verdict         VerificationVerdict `json:"verdict"`
	Status          VerificationVerdict `json:"status"`
	Summary         string              `json:"summary"`
	EvidenceRefs    []string            `json:"evidenceRefs"`
	Counterchecks   []string            `json:"counterchecks,omitempty"`
	Blockers        []string            `json:"blockers,omitempty"`
	Counterexamples []string            `json:"counterexamples,omitempty"`
	Errors          []string            `json:"errors,omitempty"`
}

func (r VerificationAgentReport) Validate() error {
	if strings.TrimSpace(r.VerifierAgentID) == "" {
		return fmt.Errorf("verifier agent id is required")
	}
	verdict := r.Verdict
	if verdict == "" {
		verdict = r.Status
	}
	switch verdict {
	case VerificationVerdictPass, VerificationVerdictPartial, VerificationVerdictFail:
	default:
		return fmt.Errorf("valid verification verdict is required")
	}
	if strings.TrimSpace(r.Summary) == "" {
		return fmt.Errorf("verification summary is required")
	}
	if verdict == VerificationVerdictPartial && len(r.Blockers) == 0 {
		return fmt.Errorf("partial verification requires blockers")
	}
	if len(r.EvidenceRefs) == 0 && len(r.Counterchecks) == 0 && len(r.Counterexamples) == 0 && len(r.Blockers) == 0 && len(r.Errors) == 0 {
		return fmt.Errorf("verification report requires evidence refs, counterexamples, or errors")
	}
	return nil
}
