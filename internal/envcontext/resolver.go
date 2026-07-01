package envcontext

import (
	"crypto/sha1"
	"encoding/hex"
	"fmt"
	"net"
	"regexp"
	"sort"
	"strings"
	"time"

	"aiops-v2/internal/diagnostics"
)

var explicitIPTargetPattern = regexp.MustCompile(`(^|[^\pL\pN_])@([0-9]{1,3}(?:\.[0-9]{1,3}){3})([^\pL\pN_]|$)`)

type ResolverInput struct {
	Input          string            `json:"input,omitempty"`
	UserFacts      []EnvironmentFact `json:"userFacts,omitempty"`
	HostFacts      []EnvironmentFact `json:"hostFacts,omitempty"`
	CorootFacts    []EnvironmentFact `json:"corootFacts,omitempty"`
	InventoryFacts []EnvironmentFact `json:"inventoryFacts,omitempty"`
	OpsGraphFacts  []EnvironmentFact `json:"opsGraphFacts,omitempty"`
	ToolFacts      []EnvironmentFact `json:"toolFacts,omitempty"`
	WebLearnFacts  []EnvironmentFact `json:"webLearnFacts,omitempty"`
	Now            time.Time         `json:"now,omitempty"`
}

type ResolverOutput struct {
	TargetRefs            []TargetRef               `json:"targetRefs,omitempty"`
	ConfirmedFacts        []EnvironmentFact         `json:"confirmedFacts,omitempty"`
	InferredFacts         []EnvironmentFact         `json:"inferredFacts,omitempty"`
	ConflictFacts         []EnvironmentFactConflict `json:"conflictFacts,omitempty"`
	MissingFacts          []string                  `json:"missingFacts,omitempty"`
	ExecutionAllowed      bool                      `json:"executionAllowed"`
	RequiresClarification bool                      `json:"requiresClarification,omitempty"`
	ReadOnlyReason        string                    `json:"readOnlyReason,omitempty"`
}

func ResolveEnvironmentFacts(input ResolverInput) ResolverOutput {
	now := input.Now
	if now.IsZero() {
		now = time.Now()
	}
	out := ResolverOutput{ExecutionAllowed: true}

	for _, target := range explicitIPTargetsFromInput(input.Input) {
		out.TargetRefs = append(out.TargetRefs, target)
		out.ConfirmedFacts = append(out.ConfirmedFacts, normalizeEnvironmentFact(EnvironmentFact{
			Kind:        FactKindHostIdentity,
			Subject:     target.ID,
			Value:       target.Address,
			Source:      FactSourceUser,
			SourceRef:   target.DisplayName,
			Confidence:  FactConfidenceConfirmed,
			CollectedAt: now,
		}, now))
	}

	for _, fact := range input.UserFacts {
		addResolvedFact(&out, fact, now, true)
	}
	for _, fact := range input.ToolFacts {
		addResolvedFact(&out, fact, now, true)
	}
	for _, fact := range input.HostFacts {
		addResolvedFact(&out, fact, now, true)
	}
	for _, fact := range input.CorootFacts {
		addResolvedFact(&out, fact, now, true)
	}
	for _, fact := range input.InventoryFacts {
		addResolvedFact(&out, fact, now, false)
	}
	for _, fact := range input.OpsGraphFacts {
		addResolvedFact(&out, fact, now, false)
	}
	for _, fact := range input.WebLearnFacts {
		out.InferredFacts = append(out.InferredFacts, normalizeWebLearnFact(fact, now))
	}

	out.TargetRefs = dedupeTargetRefs(out.TargetRefs)
	out.ConfirmedFacts = dedupeEnvironmentFacts(out.ConfirmedFacts)
	out.InferredFacts = dedupeEnvironmentFacts(out.InferredFacts)
	out.ConflictFacts = detectTargetConflicts(out.TargetRefs, out.ConfirmedFacts)
	if len(out.ConflictFacts) > 0 {
		out.ExecutionAllowed = false
		out.RequiresClarification = true
		out.ReadOnlyReason = "target_conflict_requires_clarification"
	}
	if len(out.TargetRefs) == 0 {
		out.MissingFacts = append(out.MissingFacts, "target_ref")
	}
	return out
}

func explicitIPTargetsFromInput(input string) []TargetRef {
	matches := explicitIPTargetPattern.FindAllStringSubmatch(input, -1)
	out := make([]TargetRef, 0, len(matches))
	for _, match := range matches {
		if len(match) < 3 {
			continue
		}
		address := strings.TrimSpace(match[2])
		if net.ParseIP(address) == nil {
			continue
		}
		out = append(out, TargetRef{
			ID:          "host:" + address,
			Kind:        TargetKindHost,
			DisplayName: "@" + address,
			Address:     address,
			Source:      FactSourceUser,
			Confidence:  FactConfidenceConfirmed,
		})
	}
	return out
}

func addResolvedFact(out *ResolverOutput, fact EnvironmentFact, now time.Time, defaultConfirmed bool) {
	fact = normalizeEnvironmentFact(fact, now)
	if fact.Kind == "" || fact.Value == "" {
		return
	}
	if fact.Source == FactSourceWebLearn || fact.Kind == FactKindExternalKnowledge {
		out.InferredFacts = append(out.InferredFacts, normalizeWebLearnFact(fact, now))
		return
	}
	if fact.Kind == FactKindHostIdentity && fact.Value != "" && fact.Source == FactSourceUser {
		out.TargetRefs = append(out.TargetRefs, TargetRef{
			ID:          "host:" + fact.Value,
			Kind:        TargetKindHost,
			DisplayName: firstNonEmpty(fact.Subject, fact.Value),
			Address:     fact.Value,
			Source:      fact.Source,
			Confidence:  fact.Confidence,
		})
	}
	if defaultConfirmed || fact.Confidence == FactConfidenceConfirmed || fact.Confidence == FactConfidenceObserved {
		out.ConfirmedFacts = append(out.ConfirmedFacts, fact)
		return
	}
	out.InferredFacts = append(out.InferredFacts, fact)
}

func normalizeEnvironmentFact(fact EnvironmentFact, now time.Time) EnvironmentFact {
	fact.Kind = FactKind(strings.TrimSpace(string(fact.Kind)))
	fact.Subject = strings.TrimSpace(fact.Subject)
	fact.Value = strings.TrimSpace(fact.Value)
	fact.Source = FactSource(strings.TrimSpace(string(fact.Source)))
	fact.SourceRef = strings.TrimSpace(fact.SourceRef)
	fact.Confidence = FactConfidence(strings.TrimSpace(string(fact.Confidence)))
	if fact.Source == "" {
		fact.Source = FactSourceToolOutput
	}
	if fact.Confidence == "" {
		fact.Confidence = FactConfidenceInferred
	}
	if fact.CollectedAt.IsZero() {
		fact.CollectedAt = now
	}
	if strings.TrimSpace(fact.ID) == "" {
		fact.ID = environmentFactID(fact)
	}
	return fact
}

func normalizeWebLearnFact(fact EnvironmentFact, now time.Time) EnvironmentFact {
	originalKind := strings.TrimSpace(string(fact.Kind))
	fact = normalizeEnvironmentFact(fact, now)
	if originalKind != "" && originalKind != string(FactKindExternalKnowledge) {
		fact.Value = strings.TrimSpace(originalKind + "=" + fact.Value)
	}
	fact.Kind = FactKindExternalKnowledge
	fact.Source = FactSourceWebLearn
	fact.Confidence = FactConfidenceInferred
	fact.ID = environmentFactID(fact)
	return fact
}

func detectTargetConflicts(targets []TargetRef, facts []EnvironmentFact) []EnvironmentFactConflict {
	if len(targets) == 0 || len(facts) == 0 {
		return nil
	}
	targetHosts := map[string]EnvironmentFact{}
	for _, target := range targets {
		if target.Kind != TargetKindHost || strings.TrimSpace(target.Address) == "" {
			continue
		}
		targetHosts["host:"+strings.TrimSpace(target.Address)] = EnvironmentFact{
			Kind:       FactKindHostIdentity,
			Subject:    target.ID,
			Value:      "host:" + strings.TrimSpace(target.Address),
			Source:     target.Source,
			SourceRef:  target.DisplayName,
			Confidence: target.Confidence,
		}
	}
	if len(targetHosts) == 0 {
		return nil
	}
	var conflicts []EnvironmentFactConflict
	for _, fact := range facts {
		if fact.Kind != FactKindTopology || !strings.HasPrefix(fact.Value, "host:") {
			continue
		}
		if _, ok := targetHosts[fact.Value]; ok {
			continue
		}
		userFacts := make([]EnvironmentFact, 0, len(targetHosts)+1)
		for _, targetFact := range targetHosts {
			userFacts = append(userFacts, normalizeEnvironmentFact(targetFact, time.Now()))
		}
		userFacts = append(userFacts, fact)
		conflicts = append(conflicts, EnvironmentFactConflict{
			Subject: fact.Subject,
			Kind:    fact.Kind,
			Facts:   dedupeEnvironmentFacts(userFacts),
			Reason:  "target_conflict",
		})
	}
	return conflicts
}

func (out ResolverOutput) CompactContext() string {
	lines := []string{"EnvironmentFactsContext:"}
	if len(out.TargetRefs) > 0 {
		lines = append(lines, "TargetRefs:")
		for _, target := range out.TargetRefs {
			lines = append(lines, "- "+targetLine(target))
		}
	}
	lines = appendFactLines(lines, "ConfirmedFacts", out.ConfirmedFacts)
	external, inferred := splitExternalKnowledgeFacts(out.InferredFacts)
	lines = appendFactLines(lines, "InferredFacts", inferred)
	lines = appendFactLines(lines, "ExternalKnowledge", external)
	if len(out.ConflictFacts) > 0 {
		lines = append(lines, "ConflictFacts:")
		for _, conflict := range out.ConflictFacts {
			lines = append(lines, fmt.Sprintf("- %s %s reason=%s", conflict.Kind, conflict.Subject, conflict.Reason))
			for _, fact := range conflict.Facts {
				lines = append(lines, "  - "+factLine(fact))
			}
		}
	}
	if len(out.MissingFacts) > 0 {
		lines = append(lines, "MissingFacts:")
		for _, fact := range out.MissingFacts {
			lines = append(lines, "- "+fact)
		}
	}
	if !out.ExecutionAllowed {
		lines = append(lines, "ExecutionPolicy: read_only reason="+out.ReadOnlyReason)
	}
	return diagnostics.RedactSensitiveText(strings.Join(lines, "\n"))
}

func appendFactLines(lines []string, title string, facts []EnvironmentFact) []string {
	if len(facts) == 0 {
		return lines
	}
	lines = append(lines, title+":")
	for _, fact := range facts {
		lines = append(lines, "- "+factLine(fact))
	}
	return lines
}

func splitExternalKnowledgeFacts(facts []EnvironmentFact) ([]EnvironmentFact, []EnvironmentFact) {
	var external []EnvironmentFact
	var inferred []EnvironmentFact
	for _, fact := range facts {
		if fact.Kind == FactKindExternalKnowledge {
			external = append(external, fact)
		} else {
			inferred = append(inferred, fact)
		}
	}
	return external, inferred
}

func targetLine(target TargetRef) string {
	parts := []string{fmt.Sprintf("%s id=%s", target.Kind, target.ID)}
	if target.Address != "" {
		parts = append(parts, "address="+target.Address)
	}
	if target.Source != "" {
		parts = append(parts, "source="+string(target.Source))
	}
	if target.Confidence != "" {
		parts = append(parts, "confidence="+string(target.Confidence))
	}
	return strings.Join(parts, " ")
}

func factLine(fact EnvironmentFact) string {
	parts := []string{fmt.Sprintf("%s=%s", fact.Kind, fact.Value)}
	if fact.Subject != "" {
		parts = append(parts, "subject="+fact.Subject)
	}
	if fact.Source != "" {
		parts = append(parts, "source="+string(fact.Source))
	}
	if fact.Confidence != "" {
		parts = append(parts, "confidence="+string(fact.Confidence))
	}
	if fact.SourceRef != "" {
		parts = append(parts, "ref="+fact.SourceRef)
	}
	return strings.Join(parts, " ")
}

func dedupeTargetRefs(values []TargetRef) []TargetRef {
	seen := map[string]struct{}{}
	out := make([]TargetRef, 0, len(values))
	for _, value := range values {
		key := strings.TrimSpace(string(value.Kind)) + ":" + strings.TrimSpace(value.ID) + ":" + strings.TrimSpace(value.Address)
		if strings.Trim(key, ":") == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func dedupeEnvironmentFacts(values []EnvironmentFact) []EnvironmentFact {
	seen := map[string]struct{}{}
	out := make([]EnvironmentFact, 0, len(values))
	for _, value := range values {
		key := string(value.Kind) + ":" + value.Subject + ":" + value.Value + ":" + string(value.Source)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		if out[i].Subject != out[j].Subject {
			return out[i].Subject < out[j].Subject
		}
		return out[i].Value < out[j].Value
	})
	return out
}

func environmentFactID(fact EnvironmentFact) string {
	sum := sha1.Sum([]byte(strings.Join([]string{
		string(fact.Kind),
		fact.Subject,
		fact.Value,
		string(fact.Source),
		fact.SourceRef,
	}, "\x00")))
	return "envfact-" + hex.EncodeToString(sum[:])[:12]
}
