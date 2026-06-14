package opssemantic

import (
	"strings"
	"unicode/utf8"
)

type ParseInput struct {
	ID        string
	SessionID string
	TurnID    string
	Text      string
}

func ParseTask(input ParseInput) OpsSemanticTask {
	text := strings.TrimSpace(input.Text)
	hosts := parseHostRefs(text)
	risk := ClassifyRisk(text)
	action := ActionReadOnly
	if risk != RiskReadOnly {
		action = ActionWrite
	}
	missing := make([]MissingSlot, 0, 1)
	if len(hosts) == 0 {
		missing = append(missing, MissingSlot{Name: SlotTargetHost, Reason: "target host is required"})
	}
	targets := make([]OpsTarget, 0, len(hosts))
	for _, host := range hosts {
		targets = append(targets, OpsTarget{Kind: "host", Name: firstNonEmpty(host.HostID, host.DisplayName, host.Address, host.Raw), Source: host.Source})
	}
	return OpsSemanticTask{
		ID:           strings.TrimSpace(input.ID),
		SessionID:    strings.TrimSpace(input.SessionID),
		TurnID:       strings.TrimSpace(input.TurnID),
		UserGoal:     text,
		Intent:       OpsIntent{Category: intentCategory(risk), Goal: text, Source: SourceUserInput},
		Targets:      targets,
		HostScope:    hosts,
		ActionType:   action,
		RiskLevel:    risk,
		MissingSlots: missing,
		EvidenceRequirements: []EvidenceRequirement{{
			Kind:        EvidenceCommandOutput,
			Description: "command output",
			Required:    true,
		}},
		ExecutionPolicy: ExecutionPolicy{
			AllowParallel:    len(hosts) > 1,
			RequiresApproval: RiskRequiresApproval(risk),
		},
		PlanRequired: len(hosts) > 1 || RiskRequiresApproval(risk),
	}
}

func parseHostRefs(text string) []OpsHostRef {
	seen := map[string]bool{}
	refs := make([]OpsHostRef, 0)
	for i, r := range text {
		if r != '@' {
			continue
		}
		if i > 0 {
			prev, _ := lastRune(text[:i])
			if isEmailLocalPart(prev) {
				continue
			}
		}
		end := i + 1
		for end < len(text) {
			r, size := utf8.DecodeRuneInString(text[end:])
			if !isHostTokenRune(r) {
				break
			}
			end += size
		}
		if end == i+1 {
			continue
		}
		raw := text[i:end]
		key := strings.ToLower(strings.TrimPrefix(raw, "@"))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		refs = append(refs, OpsHostRef{
			HostID:      key,
			Address:     addressIfLiteral(key),
			DisplayName: strings.TrimPrefix(raw, "@"),
			Raw:         raw,
			Source:      SourceHostMention,
		})
	}
	return refs
}

func isHostTokenRune(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.' || r == ':'
}

func isEmailLocalPart(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '_' || r == '-' || r == '.'
}

func addressIfLiteral(value string) string {
	for _, r := range value {
		if !((r >= '0' && r <= '9') || r == '.' || r == ':' || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')) {
			return ""
		}
	}
	if strings.ContainsAny(value, ".:") {
		return value
	}
	return ""
}

func intentCategory(risk OpsRiskLevel) string {
	if risk == RiskReadOnly {
		return "inspect"
	}
	return "change"
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func lastRune(text string) (rune, int) {
	var last rune
	var idx int
	for i, r := range text {
		last = r
		idx = i
	}
	return last, idx
}
