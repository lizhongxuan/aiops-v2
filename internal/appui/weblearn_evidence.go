package appui

import (
	"strings"
	"time"
	"unicode/utf8"
)

const maxWebLearnExcerptChars = 600

type WebLearnEvidence struct {
	ID              string    `json:"id,omitempty"`
	Kind            string    `json:"kind,omitempty"`
	Query           string    `json:"query,omitempty"`
	SourceURL       string    `json:"sourceUrl,omitempty"`
	SourceTitle     string    `json:"sourceTitle,omitempty"`
	SourceKind      string    `json:"sourceKind,omitempty"`
	Product         string    `json:"product,omitempty"`
	Version         string    `json:"version,omitempty"`
	RetrievedAt     time.Time `json:"retrievedAt,omitempty"`
	RelevantExcerpt string    `json:"relevantExcerpt,omitempty"`
	Applicability   string    `json:"applicability,omitempty"`
	Confidence      string    `json:"confidence,omitempty"`
}

type WebLearnDecisionInput struct {
	UserInput          string
	LocalContextEnough bool
	UserDisabledWeb    bool
	NetworkAvailable   bool
	OfficialSourceHint string
}

type WebLearnDecision struct {
	ShouldSearch bool
	Reason       string
}

func DecideWebLearn(input WebLearnDecisionInput) WebLearnDecision {
	if input.UserDisabledWeb {
		return WebLearnDecision{Reason: "user_disabled_web"}
	}
	if input.LocalContextEnough {
		return WebLearnDecision{Reason: "local_context_enough"}
	}
	if !input.NetworkAvailable && strings.TrimSpace(input.OfficialSourceHint) == "" {
		return WebLearnDecision{Reason: "network_unavailable"}
	}
	if !hasWebLearnKnowledgeTrigger(input.UserInput) {
		return WebLearnDecision{Reason: "no_external_knowledge_trigger"}
	}
	if strings.TrimSpace(input.OfficialSourceHint) == "" {
		return WebLearnDecision{Reason: "no_official_source_scope"}
	}
	return WebLearnDecision{ShouldSearch: true, Reason: "official_external_knowledge_needed"}
}

func NormalizeWebLearnEvidence(ev WebLearnEvidence) WebLearnEvidence {
	ev.ID = strings.TrimSpace(ev.ID)
	ev.Kind = "external_knowledge"
	ev.Query = strings.TrimSpace(ev.Query)
	ev.SourceURL = strings.TrimSpace(ev.SourceURL)
	ev.SourceTitle = strings.TrimSpace(ev.SourceTitle)
	ev.SourceKind = strings.ToLower(strings.TrimSpace(ev.SourceKind))
	ev.Product = strings.TrimSpace(ev.Product)
	ev.Version = strings.TrimSpace(ev.Version)
	ev.RelevantExcerpt = truncateRunes(strings.TrimSpace(ev.RelevantExcerpt), maxWebLearnExcerptChars)
	ev.Applicability = strings.TrimSpace(ev.Applicability)
	ev.Confidence = normalizeWebLearnConfidence(ev.Confidence)
	return ev
}

func hasWebLearnKnowledgeTrigger(input string) bool {
	lower := strings.ToLower(strings.TrimSpace(input))
	if lower == "" {
		return false
	}
	triggers := []string{
		"unknown", "version", "error", "command", "middleware", "network", "release", "docs",
		"版本", "错误码", "命令", "中间件", "网络", "官方", "文档", "是什么意思",
	}
	for _, trigger := range triggers {
		if strings.Contains(lower, trigger) {
			return true
		}
	}
	return false
}

func normalizeWebLearnConfidence(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "high", "medium", "low":
		return strings.ToLower(strings.TrimSpace(value))
	default:
		return "low"
	}
}

func truncateRunes(value string, limit int) string {
	if limit <= 0 || utf8.RuneCountInString(value) <= limit {
		return value
	}
	runes := []rune(value)
	return string(runes[:limit])
}
