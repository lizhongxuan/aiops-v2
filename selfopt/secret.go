package selfopt

import (
	"regexp"
)

type SecretFinding struct {
	Kind string `json:"kind"`
}

type SecretScanner struct {
	patterns []secretPattern
}

type secretPattern struct {
	kind string
	re   *regexp.Regexp
}

func NewSecretScanner() SecretScanner {
	return SecretScanner{patterns: []secretPattern{
		{"api_key", regexp.MustCompile(`sk-[A-Za-z0-9_-]{12,}`)},
		{"authorization", regexp.MustCompile(`(?i)Authorization:\s*Bearer\s+[A-Za-z0-9._-]+`)},
		{"password", regexp.MustCompile(`(?i)password\s*=\s*[^,\s"']+`)},
		{"token", regexp.MustCompile(`(?i)token\s*=\s*[^,\s"']+`)},
		{"ssh_private_key", regexp.MustCompile(`BEGIN OPENSSH PRIVATE KEY`)},
	}}
}

func (s SecretScanner) ScanString(text string) []SecretFinding {
	var out []SecretFinding
	for _, pattern := range s.patterns {
		if pattern.re.MatchString(text) {
			out = append(out, SecretFinding{Kind: pattern.kind})
		}
	}
	return out
}
