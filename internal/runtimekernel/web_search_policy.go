package runtimekernel

import (
	"regexp"
	"strings"
)

type WebSearchPolicyLevel string

const (
	WebSearchDisabled WebSearchPolicyLevel = "disabled"
	WebSearchEnabled  WebSearchPolicyLevel = "enabled"
)

type WebSearchPolicyInput struct {
	UserInput             string
	PublicWebAvailable    bool
	CurrentOrPrivateScope bool
}

type WebSearchPolicyDecision struct {
	Level            WebSearchPolicyLevel `json:"level"`
	Reason           string               `json:"reason"`
	ReasonCodes      []string             `json:"reasonCodes,omitempty"`
	QuerySeeds       []string             `json:"querySeeds,omitempty"`
	DisabledBy       string               `json:"disabledBy,omitempty"`
	RequireCitations bool                 `json:"requireCitations,omitempty"`
}

func EvaluateWebSearchPolicy(input WebSearchPolicyInput) WebSearchPolicyDecision {
	text := strings.TrimSpace(input.UserInput)
	lower := strings.ToLower(text)
	if userDisabledWebSearch(lower) {
		return WebSearchPolicyDecision{
			Level:       WebSearchDisabled,
			Reason:      "user_disabled_web",
			ReasonCodes: []string{"user_disabled_web"},
			DisabledBy:  "user",
		}
	}
	if input.CurrentOrPrivateScope || currentOrPrivateWebSearchScope(lower) {
		return WebSearchPolicyDecision{
			Level:       WebSearchDisabled,
			Reason:      "private_or_current_scope",
			ReasonCodes: []string{"private_or_current_scope"},
			DisabledBy:  "scope",
		}
	}
	if !input.PublicWebAvailable {
		return WebSearchPolicyDecision{
			Level:       WebSearchDisabled,
			Reason:      "public_web_unavailable",
			ReasonCodes: []string{"public_web_unavailable"},
			DisabledBy:  "tool_unavailable",
		}
	}
	if explicitPublicWebSearchRequest(lower) {
		return WebSearchPolicyDecision{
			Level:       WebSearchEnabled,
			Reason:      "explicit_public_web_request",
			ReasonCodes: []string{"explicit_public_web_request"},
			QuerySeeds:  buildWebSearchQuerySeeds(text),
		}
	}
	if highRiskVersionedOpsQuestion(lower) {
		return WebSearchPolicyDecision{
			Level:       WebSearchEnabled,
			Reason:      "high_risk_versioned_ops",
			ReasonCodes: []string{"high_risk_versioned_ops"},
			QuerySeeds:  buildWebSearchQuerySeeds(text),
		}
	}
	if publicTechnicalKnowledgeQuestion(lower) {
		return WebSearchPolicyDecision{
			Level:       WebSearchEnabled,
			Reason:      "public_technical_knowledge",
			ReasonCodes: []string{"public_technical_knowledge"},
			QuerySeeds:  buildWebSearchQuerySeeds(text),
		}
	}
	return WebSearchPolicyDecision{
		Level:       WebSearchDisabled,
		Reason:      "simple_stable",
		ReasonCodes: []string{"simple_stable"},
	}
}

func userDisabledWebSearch(lower string) bool {
	return containsAnyWebSearchSignal(lower,
		"不要联网", "不联网", "不要搜索", "不搜索", "不要查网页", "不要查公开资料",
		"只基于本地", "仅基于本地", "只基于上下文", "仅基于上下文", "without web",
		"without browsing", "do not browse", "do not search",
	)
}

func currentOrPrivateWebSearchScope(lower string) bool {
	return containsAnyWebSearchSignal(lower,
		"prompt trace", "当前页面", "本页面", "当前主机", "当前环境", "当前cpu", "当前 cpu",
		"查看 cpu", "查看cpu", "查看内存", "查看磁盘", "@server-local", "@local",
	)
}

func explicitPublicWebSearchRequest(lower string) bool {
	return containsAnyWebSearchSignal(lower,
		"查一下", "查下", "搜索", "联网", "网页", "公开资料", "官方文档", "官方资料",
		"验证", "判断是否正确", "给出处", "出处", "最新", "当前官方", "browse",
		"search", "lookup", "verify", "latest", "current docs", "source", "cite",
	)
}

func highRiskVersionedOpsQuestion(lower string) bool {
	component := containsAnyWebSearchSignal(lower,
		"postgres", "postgresql", "pgbackrest", "pg_auto_failover", "pg_autoctl",
		"patroni", "etcd", "kubernetes", "k8s", "redis", "mysql", "kafka", "systemd",
	)
	riskCount := 0
	for _, signal := range []string{
		"restore", "recovery", "backup", "pitr", "replication", "failover", "promote",
		"rewind", "timeline", "standby", "primary", "wal", "monitor", "恢复", "备份",
		"复制", "主从", "时间线", "归档", "集群", "从节点", "主节点",
	} {
		if strings.Contains(lower, signal) {
			riskCount++
		}
	}
	return component && riskCount >= 2
}

func publicTechnicalKnowledgeQuestion(lower string) bool {
	if containsAnyWebSearchSignal(lower, "redis", "postgres", "postgresql", "pgbackrest", "pg_auto_failover", "patroni", "kubernetes", "k8s", "mysql", "kafka") {
		return true
	}
	return false
}

func buildWebSearchQuerySeeds(text string) []string {
	sanitized := sanitizeWebSearchQuerySeed(text)
	lower := strings.ToLower(sanitized)
	entities := make([]string, 0, 4)
	for _, pair := range []struct {
		match string
		name  string
	}{
		{"postgresql", "PostgreSQL"},
		{"postgres", "PostgreSQL"},
		{"pgbackrest", "pgBackRest"},
		{"pg_auto_failover", "pg_auto_failover"},
		{"pg_autoctl", "pg_autoctl"},
		{"redis", "Redis"},
		{"kubernetes", "Kubernetes"},
		{"k8s", "Kubernetes"},
	} {
		if strings.Contains(lower, pair.match) && !containsWebSearchSeed(entities, pair.name) {
			entities = append(entities, pair.name)
		}
	}
	ops := make([]string, 0, 6)
	for _, pair := range []struct {
		match string
		name  string
	}{
		{"recovery_target_timeline", "recovery_target_timeline"},
		{"timeline", "timeline"},
		{"时间线", "timeline"},
		{"restore", "restore"},
		{"恢复", "restore"},
		{"standby", "standby"},
		{"primary", "primary"},
		{"monitor", "monitor"},
		{"replication", "replication"},
		{"复制", "replication"},
		{"failover", "failover"},
	} {
		if strings.Contains(lower, pair.match) && !containsWebSearchSeed(ops, pair.name) {
			ops = append(ops, pair.name)
		}
	}
	if len(entities) == 0 && sanitized != "" {
		return []string{truncateWebSearchSeed(sanitized + " official docs")}
	}
	seed := strings.Join(append(append([]string{}, entities...), ops...), " ")
	seed = strings.TrimSpace(seed + " official docs")
	if seed == "official docs" {
		return nil
	}
	return []string{truncateWebSearchSeed(seed)}
}

var (
	webSearchIPv4Pattern       = regexp.MustCompile(`\b(?:\d{1,3}\.){3}\d{1,3}\b`)
	webSearchSecretWordPattern = regexp.MustCompile(`(?i)\b(password|passwd|pwd|token|apikey|api_key|secret|密码|口令|密钥)\s*[:=]?\s*\S+`)
)

func sanitizeWebSearchQuerySeed(value string) string {
	value = webSearchSecretWordPattern.ReplaceAllString(value, "")
	value = webSearchIPv4Pattern.ReplaceAllString(value, "")
	value = regexp.MustCompile(`(?i)\broot\b`).ReplaceAllString(value, "")
	value = strings.Join(strings.Fields(value), " ")
	return strings.TrimSpace(value)
}

func truncateWebSearchSeed(value string) string {
	const maxSeedRunes = 160
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) <= maxSeedRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxSeedRunes]))
}

func containsAnyWebSearchSignal(value string, signals ...string) bool {
	for _, signal := range signals {
		signal = strings.ToLower(strings.TrimSpace(signal))
		if signal != "" && strings.Contains(value, signal) {
			return true
		}
	}
	return false
}

func containsWebSearchSeed(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
