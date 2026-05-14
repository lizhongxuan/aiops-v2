package experiencepack

import (
	"regexp"
	"strconv"
	"strings"
)

func MatchSignals(patterns, signals []string) []string {
	var matched []string
	for _, pattern := range patterns {
		for _, signal := range signals {
			if matchSignalPattern(pattern, signal) {
				matched = append(matched, pattern)
				break
			}
		}
	}
	return matched
}

func matchSignalPattern(pattern, signal string) bool {
	pattern = strings.TrimSpace(pattern)
	signal = strings.TrimSpace(signal)
	if pattern == "" || signal == "" {
		return false
	}
	if strings.HasPrefix(pattern, "/") {
		if re, ok := compileSlashRegexp(pattern); ok {
			return re.MatchString(signal)
		}
	}
	for _, alias := range strings.Split(pattern, "|") {
		alias = strings.TrimSpace(alias)
		if alias == "" {
			continue
		}
		if strings.Contains(strings.ToLower(signal), strings.ToLower(alias)) {
			return true
		}
	}
	return false
}

func compileSlashRegexp(pattern string) (*regexp.Regexp, bool) {
	lastSlash := strings.LastIndex(pattern, "/")
	if lastSlash <= 0 {
		return nil, false
	}
	body := pattern[1:lastSlash]
	flags := pattern[lastSlash+1:]
	if strings.Contains(flags, "i") {
		body = "(?i)" + body
	}
	re, err := regexp.Compile(body)
	return re, err == nil
}

func ExtractSignals(text string) []string {
	normalized := strings.ToLower(text)
	signals := []string{}
	add := func(signal string) {
		for _, existing := range signals {
			if existing == signal {
				return
			}
		}
		signals = append(signals, signal)
	}
	if strings.Contains(normalized, "postgres") || strings.Contains(normalized, " pg") || strings.Contains(normalized, "pg ") || strings.Contains(text, "主从") {
		add("postgres")
	}
	if strings.Contains(text, "主从") || strings.Contains(normalized, "primary") || strings.Contains(normalized, "standby") || strings.Contains(normalized, "replication") {
		add("primary standby")
		add("replication")
	}
	if strings.Contains(normalized, "pg_mon") {
		add("pg_mon")
	}
	if strings.Contains(text, "部署") || strings.Contains(normalized, "deploy") {
		add("deploy")
	}
	if strings.Contains(normalized, "ubuntu") {
		add("os:ubuntu")
		add("ubuntu")
	}
	if strings.Contains(normalized, "rhel") || strings.Contains(normalized, "red hat") || strings.Contains(normalized, "redhat") {
		add("os:rhel")
		add("rhel")
	}
	if strings.Contains(normalized, "centos") {
		add("os:centos")
		add("centos")
	}
	if strings.Contains(normalized, "debian") {
		add("os:debian")
		add("debian")
	}
	if strings.Contains(normalized, "oom") || strings.Contains(normalized, "oomkilled") {
		add("oomkilled")
	}
	if strings.Contains(text, "慢") || strings.Contains(normalized, "slow") || strings.Contains(normalized, "latency") {
		add("performance_slow")
	}
	if hostCount := inferHostCount(text); hostCount > 0 {
		add("hosts:" + strconv.Itoa(hostCount))
	}
	return addTokens(signals, text)
}

func inferHostCount(text string) int {
	seen := map[string]bool{}
	for _, match := range regexp.MustCompile(`(?i)\b(?:host|server|node)[-_]?[a-z0-9]+\b`).FindAllString(text, -1) {
		seen[strings.ToLower(match)] = true
	}
	for _, match := range regexp.MustCompile(`[\p{L}\p{N}_-]+\s*主机`).FindAllString(text, -1) {
		seen[match] = true
	}
	if len(seen) == 0 {
		return 0
	}
	return len(seen)
}

func addTokens(signals []string, text string) []string {
	for _, token := range strings.FieldsFunc(text, func(r rune) bool {
		return r == ',' || r == '，' || r == ';' || r == '；' || r == '\n' || r == '\t'
	}) {
		token = strings.TrimSpace(token)
		if token == "" || len([]rune(token)) > 48 {
			continue
		}
		exists := false
		for _, signal := range signals {
			if signal == token {
				exists = true
				break
			}
		}
		if !exists {
			signals = append(signals, token)
		}
	}
	return signals
}
