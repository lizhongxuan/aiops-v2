package opsmanual

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	defaultPromptCapsuleMaxFacts = 8
	defaultPromptCapsuleMaxHints = 3
	defaultPromptCapsuleMaxChars = 1200
)

type OpsManualPromptCapsuleInput struct {
	FlowID          string
	CurrentTarget   string
	SelectedManual  string
	Decision        string
	Missing         []string
	BlockedBy       []string
	SessionFacts    []SessionOpsFact
	LettaHints      []OpsManualPromptHint
	DiscoveryRefs   []OpsManualPromptRef
	PreviousCapsule string
	MaxChars        int
}

type OpsManualPromptHint struct {
	ID    string
	Text  string
	Score float64
}

type OpsManualPromptRef struct {
	ID      string
	Kind    string
	Summary string
	Raw     string
}

type OpsManualPromptFact struct {
	Key        string
	Value      string
	Source     string
	Confidence float64
}

type OpsManualPromptCapsule struct {
	FlowID                string
	CurrentTarget         string
	SelectedManual        string
	Decision              string
	Missing               []string
	ConfirmedFacts        []OpsManualPromptFact
	LettaHints            []OpsManualPromptHint
	BlockedBy             []string
	Refs                  []OpsManualPromptRef
	DroppedContextReasons []string
}

func BuildOpsManualPromptCapsule(input OpsManualPromptCapsuleInput) OpsManualPromptCapsule {
	capsule := OpsManualPromptCapsule{
		FlowID:         strings.TrimSpace(input.FlowID),
		CurrentTarget:  strings.TrimSpace(input.CurrentTarget),
		SelectedManual: strings.TrimSpace(input.SelectedManual),
		Decision:       strings.TrimSpace(input.Decision),
		Missing:        dedupe(input.Missing),
		BlockedBy:      dedupe(input.BlockedBy),
	}
	capsule.ConfirmedFacts, capsule.DroppedContextReasons = selectedPromptFacts(input.SessionFacts, capsule.DroppedContextReasons)
	capsule.LettaHints, capsule.DroppedContextReasons = selectedPromptHints(input.LettaHints, capsule.DroppedContextReasons)
	capsule.Refs, capsule.DroppedContextReasons = selectedPromptRefs(input.DiscoveryRefs, capsule.DroppedContextReasons)
	if strings.TrimSpace(input.PreviousCapsule) != "" {
		capsule.DroppedContextReasons = appendUnique(capsule.DroppedContextReasons, "previous_capsule_omitted")
	}
	maxChars := input.MaxChars
	if maxChars <= 0 {
		maxChars = defaultPromptCapsuleMaxChars
	}
	if len(capsule.Markdown()) > maxChars {
		capsule.ConfirmedFacts = nil
		capsule.LettaHints = nil
		capsule.Refs = nil
		capsule.DroppedContextReasons = appendUnique(capsule.DroppedContextReasons, "budget_compacted")
	}
	return capsule
}

func (c OpsManualPromptCapsule) Markdown() string {
	var b strings.Builder
	addLine := func(key string, value string) {
		if strings.TrimSpace(value) != "" {
			fmt.Fprintf(&b, "%s: %s\n", key, strings.TrimSpace(value))
		}
	}
	addLine("flow", c.FlowID)
	addLine("current_target", c.CurrentTarget)
	addLine("selected_manual", c.SelectedManual)
	addLine("decision", c.Decision)
	if len(c.Missing) > 0 {
		addLine("missing", strings.Join(c.Missing, ", "))
	}
	if len(c.BlockedBy) > 0 {
		addLine("blocked_by", strings.Join(c.BlockedBy, ", "))
	}
	if len(c.ConfirmedFacts) > 0 {
		b.WriteString("confirmed_facts:\n")
		for _, fact := range c.ConfirmedFacts {
			fmt.Fprintf(&b, "- %s=%s", fact.Key, fact.Value)
			if fact.Source != "" {
				fmt.Fprintf(&b, " source=%s", fact.Source)
			}
			b.WriteString("\n")
		}
	}
	if len(c.LettaHints) > 0 {
		b.WriteString("letta_hints:\n")
		for _, hint := range c.LettaHints {
			fmt.Fprintf(&b, "- %s: %s\n", hint.ID, strings.TrimSpace(hint.Text))
		}
	}
	if len(c.Refs) > 0 {
		b.WriteString("refs:\n")
		for _, ref := range c.Refs {
			fmt.Fprintf(&b, "- %s/%s: %s\n", strings.TrimSpace(ref.Kind), strings.TrimSpace(ref.ID), strings.TrimSpace(ref.Summary))
		}
	}
	if len(c.DroppedContextReasons) > 0 {
		addLine("dropped_context_reasons", strings.Join(c.DroppedContextReasons, ", "))
	}
	return strings.TrimSpace(b.String())
}

func selectedPromptFacts(facts []SessionOpsFact, reasons []string) ([]OpsManualPromptFact, []string) {
	filtered := make([]SessionOpsFact, 0, len(facts))
	for _, fact := range facts {
		if !fact.ConfirmedByUser || factExpired(fact, time.Now().UTC()) || fact.Sensitive || !valuePresent(fact.Value) {
			continue
		}
		filtered = append(filtered, fact)
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Confidence != filtered[j].Confidence {
			return filtered[i].Confidence > filtered[j].Confidence
		}
		return filtered[i].UpdatedAt.After(filtered[j].UpdatedAt)
	})
	if len(filtered) > defaultPromptCapsuleMaxFacts {
		reasons = appendUnique(reasons, "session_fact_limit")
		filtered = filtered[:defaultPromptCapsuleMaxFacts]
	}
	out := make([]OpsManualPromptFact, 0, len(filtered))
	for _, fact := range filtered {
		out = append(out, OpsManualPromptFact{
			Key:        strings.TrimSpace(fact.Key),
			Value:      strings.TrimSpace(fmt.Sprint(fact.Value)),
			Source:     strings.TrimSpace(fact.Source),
			Confidence: fact.Confidence,
		})
	}
	return out, reasons
}

func selectedPromptHints(hints []OpsManualPromptHint, reasons []string) ([]OpsManualPromptHint, []string) {
	out := append([]OpsManualPromptHint(nil), hints...)
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].ID < out[j].ID
	})
	if len(out) > defaultPromptCapsuleMaxHints {
		reasons = appendUnique(reasons, "letta_hint_limit")
		out = out[:defaultPromptCapsuleMaxHints]
	}
	for i := range out {
		out[i].Text = strings.TrimSpace(out[i].Text)
	}
	return out, reasons
}

func selectedPromptRefs(refs []OpsManualPromptRef, reasons []string) ([]OpsManualPromptRef, []string) {
	out := make([]OpsManualPromptRef, 0, len(refs))
	for _, ref := range refs {
		if strings.TrimSpace(ref.ID) == "" && strings.TrimSpace(ref.Summary) == "" {
			continue
		}
		if strings.TrimSpace(ref.Raw) != "" {
			reasons = appendUnique(reasons, "artifact_ref_only")
		}
		out = append(out, OpsManualPromptRef{
			ID:      strings.TrimSpace(ref.ID),
			Kind:    firstNonEmpty(strings.TrimSpace(ref.Kind), "artifact"),
			Summary: strings.TrimSpace(ref.Summary),
		})
	}
	return out, reasons
}
