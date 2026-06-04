package hostops

import "strings"

type RouteKind string

const (
	RouteKindNormalChat RouteKind = "normal_chat"
	RouteKindHostOps    RouteKind = "host_ops"
)

type RouteDecision struct {
	Kind         RouteKind
	Mentions     []HostMention
	PlanRequired bool
	Reason       string
}

func DetectRoute(content string, mentions []HostMention) RouteDecision {
	uniqueHosts := uniqueResolvedOrLiteralMentionKeys(mentions)
	if len(uniqueHosts) == 0 {
		return RouteDecision{Kind: RouteKindNormalChat, Mentions: cloneMentions(mentions), Reason: "no host mentions"}
	}
	decision := RouteDecision{
		Kind:         RouteKindHostOps,
		Mentions:     cloneMentions(mentions),
		PlanRequired: len(uniqueHosts) >= 2,
	}
	if decision.PlanRequired {
		decision.Reason = "multi-host operation requires plan mode"
	} else if looksLikeHostOperation(content) {
		decision.Reason = "host mention with operation intent"
	} else {
		decision.Reason = "host mention"
	}
	return decision
}

func uniqueResolvedOrLiteralMentionKeys(mentions []HostMention) []string {
	seen := map[string]struct{}{}
	for _, mention := range mentions {
		key := mentionKey(mention)
		if key == "" {
			continue
		}
		seen[key] = struct{}{}
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	return keys
}

func cloneMentions(mentions []HostMention) []HostMention {
	return append([]HostMention(nil), mentions...)
}

func looksLikeHostOperation(content string) bool {
	normalized := strings.ToLower(strings.TrimSpace(content))
	if normalized == "" {
		return false
	}
	keywords := []string{
		"检查", "部署", "搭建", "安装", "配置", "重启", "修复", "执行",
		"check", "deploy", "install", "configure", "restart", "repair", "run",
	}
	for _, keyword := range keywords {
		if strings.Contains(normalized, keyword) {
			return true
		}
	}
	return false
}
