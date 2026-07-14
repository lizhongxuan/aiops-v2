package runtimekernel

import (
	"fmt"
	"strings"

	"aiops-v2/internal/promptcompiler"
)

type ContextRetentionPolicy struct {
	RankBudgets   map[string]int
	HardKeepRanks []string
	DefaultAction string
}

type ContextRetentionSection struct {
	ID             string
	TokensEstimate int
	RequiredFields []string
	RawOutput      bool
	ArtifactRef    string
}

type ContextRetentionDecision struct {
	SectionID        string
	RetentionRank    string
	RetentionClass   string
	Action           string
	SourceRef        string
	ArtifactRef      string
	BlockingReason   string
	ValidationStatus string
}

func DefaultContextRetentionPolicy() ContextRetentionPolicy {
	return ContextRetentionPolicy{
		RankBudgets: map[string]int{
			promptcompiler.RetentionRankP0: 8192,
			promptcompiler.RetentionRankP1: 1024,
			promptcompiler.RetentionRankP2: 2048,
			promptcompiler.RetentionRankP3: 1024,
			promptcompiler.RetentionRankP4: 256,
		},
		HardKeepRanks: []string{promptcompiler.RetentionRankP0},
		DefaultAction: promptcompiler.CompactActionSummarized,
	}
}

func (p ContextRetentionPolicy) Decide(section ContextRetentionSection) ContextRetentionDecision {
	contract := promptcompiler.LookupPromptSectionContract(section.ID)
	decision := ContextRetentionDecision{
		SectionID:      strings.TrimSpace(section.ID),
		RetentionRank:  contract.RetentionRank,
		RetentionClass: contract.RetentionClass,
	}
	if decision.SectionID == "" {
		decision.SectionID = "unknown"
	}
	budget := contextRetentionBudget(contract, p)

	if section.RawOutput || contract.RetentionRank == promptcompiler.RetentionRankP4 {
		decision.Action = promptcompiler.CompactActionExternalized
		decision.ArtifactRef = strings.TrimSpace(section.ArtifactRef)
		if decision.ArtifactRef == "" {
			decision.ValidationStatus = "artifact_ref_missing"
		} else {
			decision.ValidationStatus = "valid"
		}
		return decision
	}

	if contract.RetentionRank == promptcompiler.RetentionRankP0 {
		if section.TokensEstimate > budget {
			decision.Action = promptcompiler.CompactActionBlocked
			decision.BlockingReason = "p0_over_budget"
			decision.ValidationStatus = "blocked"
			return decision
		}
		decision.Action = promptcompiler.CompactActionKeptOriginal
		decision.ValidationStatus = "valid"
		return decision
	}

	if contract.RetentionRank == promptcompiler.RetentionRankP1 {
		if len(section.RequiredFields) == 0 {
			decision.Action = promptcompiler.CompactActionBlocked
			decision.BlockingReason = "p1_required_field_missing"
			decision.ValidationStatus = "blocked"
			return decision
		}
		if section.TokensEstimate > budget {
			decision.Action = promptcompiler.CompactActionSummarized
			decision.ValidationStatus = "valid"
			return decision
		}
		decision.Action = promptcompiler.CompactActionKeptOriginal
		decision.ValidationStatus = "valid"
		return decision
	}

	if section.TokensEstimate > budget {
		decision.Action = promptcompiler.CompactActionSummarized
	} else {
		decision.Action = promptcompiler.CompactActionKeptOriginal
	}
	decision.ValidationStatus = "valid"
	return decision
}

func contextRetentionBudget(contract promptcompiler.PromptSectionContract, policy ContextRetentionPolicy) int {
	rankBudget := policy.RankBudgets[contract.RetentionRank]
	switch {
	case rankBudget > 0 && contract.MaxTokens > 0:
		if rankBudget < contract.MaxTokens {
			return rankBudget
		}
		return contract.MaxTokens
	case rankBudget > 0:
		return rankBudget
	case contract.MaxTokens > 0:
		return contract.MaxTokens
	default:
		return 512
	}
}

func ApplyPromptSectionRetentionPolicy(sections []promptcompiler.PromptSectionTrace, policy ContextRetentionPolicy) ([]promptcompiler.PromptSectionTrace, []ContextRetentionDecision, error) {
	if len(sections) == 0 {
		return nil, nil, nil
	}
	if policy.DefaultAction == "" {
		policy = DefaultContextRetentionPolicy()
	}
	annotated := append([]promptcompiler.PromptSectionTrace(nil), sections...)
	decisions := make([]ContextRetentionDecision, 0, len(annotated))
	for i := range annotated {
		section := &annotated[i]
		contract := promptcompiler.LookupPromptSectionContract(section.ID)
		artifactRef := strings.TrimSpace(section.SourceRef)
		rawOutput := contract.RetentionRank == promptcompiler.RetentionRankP4 || contract.RetentionClass == promptcompiler.RetentionClassExternalize
		if rawOutput && artifactRef == "" {
			artifactRef = "artifact://prompt-section/" + safePromptSectionRef(section.ID)
		}
		decision := policy.Decide(ContextRetentionSection{
			ID:             section.ID,
			TokensEstimate: section.TokensEstimate,
			RequiredFields: append([]string(nil), contract.RequiredFields...),
			RawOutput:      rawOutput,
			ArtifactRef:    artifactRef,
		})
		decision.SourceRef = "prompt_section:" + safePromptSectionRef(section.ID)
		decisions = append(decisions, decision)
		if section.TokenEstimate == 0 {
			section.TokenEstimate = section.TokensEstimate
		}
		section.RetentionRank = decision.RetentionRank
		section.RetentionClass = decision.RetentionClass
		section.CompactAction = decision.Action
		section.Action = harnessContextSectionAction(decision.Action)
		section.SourceRef = decision.SourceRef
		if section.CompactSchema == "" {
			section.CompactSchema = contract.CompactSchema
		}
		if section.Redaction == "" {
			section.Redaction = contract.RedactionPolicy
		}
		if section.Purpose == "" {
			section.Purpose = contract.Purpose
		}
		if decision.Action == promptcompiler.CompactActionBlocked {
			return annotated, decisions, fmt.Errorf("prompt section %s blocked by retention policy: %s", section.ID, decision.BlockingReason)
		}
	}
	return annotated, decisions, nil
}

func harnessContextSectionAction(action string) string {
	switch strings.TrimSpace(action) {
	case promptcompiler.CompactActionKeptOriginal, "":
		return "kept"
	case promptcompiler.CompactActionSummarized:
		return "summarized"
	case promptcompiler.CompactActionExternalized:
		return "externalized"
	case promptcompiler.CompactActionBlocked:
		return "blocked"
	default:
		return strings.TrimSpace(action)
	}
}

func safePromptSectionRef(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "unknown"
	}
	var b strings.Builder
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '.', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

func PromptSectionRetentionGovernanceEvents(sessionID, turnID string, iteration int, decisions []ContextRetentionDecision) []ContextGovernanceEvent {
	events := make([]ContextGovernanceEvent, 0)
	for _, decision := range decisions {
		if decision.Action == "" || decision.Action == promptcompiler.CompactActionKeptOriginal {
			continue
		}
		events = append(events, BuildContextGovernanceEvent(ContextGovernanceEvent{
			ID:              fmt.Sprintf("ctxgov-%s-%d-section-%s", turnID, iteration, safePromptSectionRef(decision.SectionID)),
			Layer:           ContextGovernanceLayerL4,
			Kind:            "context.prompt_section_retention." + decision.Action,
			SessionID:       sessionID,
			TurnID:          turnID,
			Iteration:       iteration,
			Message:         "prompt section retention policy applied",
			ReferenceIDs:    []string{decision.SourceRef},
			DroppedGroupIDs: []string{decision.SectionID},
		}))
	}
	return events
}
