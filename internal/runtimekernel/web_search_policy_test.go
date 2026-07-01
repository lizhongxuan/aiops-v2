package runtimekernel

import (
	"strings"
	"testing"
)

func TestEvaluateWebSearchPolicy(t *testing.T) {
	tests := []struct {
		name    string
		input   WebSearchPolicyInput
		want    WebSearchPolicyLevel
		reason  string
		noSeeds []string
	}{
		{
			name: "simple stable concept is disabled",
			input: WebSearchPolicyInput{
				UserInput:          "解释一下 Linux load average 是什么",
				PublicWebAvailable: true,
			},
			want:   WebSearchDisabled,
			reason: "simple_stable",
		},
		{
			name: "explicit verification enables web search without forcing it",
			input: WebSearchPolicyInput{
				UserInput:          "查一下 PostgreSQL 官方文档，判断 recovery_target_timeline 的行为是否正确",
				PublicWebAvailable: true,
			},
			want:   WebSearchEnabled,
			reason: "explicit_public_web_request",
		},
		{
			name: "complex restore timeline diagnosis enables web search without forcing it",
			input: WebSearchPolicyInput{
				UserInput:          "我用 pgbackrest 恢复主机A，再加入 pg_auto_failover monitor，并把主机B当做从节点，为什么 standby timeline higher than primary after restore timeline diverged？",
				PublicWebAvailable: true,
			},
			want:   WebSearchEnabled,
			reason: "high_risk_versioned_ops",
		},
		{
			name: "ordinary middleware mention is enabled not must search",
			input: WebSearchPolicyInput{
				UserInput:          "Redis latency doctor 是什么意思？",
				PublicWebAvailable: true,
			},
			want:   WebSearchEnabled,
			reason: "public_technical_knowledge",
		},
		{
			name: "private current host scope wins over component words",
			input: WebSearchPolicyInput{
				UserInput:             "@server-local 查看 PostgreSQL 当前 CPU 和进程情况",
				PublicWebAvailable:    true,
				CurrentOrPrivateScope: true,
			},
			want:   WebSearchDisabled,
			reason: "private_or_current_scope",
		},
		{
			name: "user disabled web wins",
			input: WebSearchPolicyInput{
				UserInput:          "不要联网，只基于本地上下文解释 PostgreSQL timeline",
				PublicWebAvailable: true,
			},
			want:   WebSearchDisabled,
			reason: "user_disabled_web",
		},
		{
			name: "unavailable public web disables tool while preserving reason",
			input: WebSearchPolicyInput{
				UserInput:          "查一下官方文档确认 Kubernetes 这个参数最新行为",
				PublicWebAvailable: false,
			},
			want:   WebSearchDisabled,
			reason: "public_web_unavailable",
		},
		{
			name: "query seeds redact private values",
			input: WebSearchPolicyInput{
				UserInput:          "查一下 pgBackRest restore timeline 官方文档，主机 203.0.113.42 root 密码 fake-password-123 token fake-token-456",
				PublicWebAvailable: true,
			},
			want:    WebSearchEnabled,
			reason:  "explicit_public_web_request",
			noSeeds: []string{"203.0.113.42", "fake-password-123", "fake-token-456", "root"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := EvaluateWebSearchPolicy(tc.input)
			if got.Level != tc.want {
				t.Fatalf("Level = %q, want %q; decision=%#v", got.Level, tc.want, got)
			}
			if got.Reason != tc.reason {
				t.Fatalf("Reason = %q, want %q; decision=%#v", got.Reason, tc.reason, got)
			}
			joinedSeeds := strings.Join(got.QuerySeeds, " ")
			for _, forbidden := range tc.noSeeds {
				if strings.Contains(joinedSeeds, forbidden) {
					t.Fatalf("QuerySeeds = %#v, must redact %q", got.QuerySeeds, forbidden)
				}
			}
		})
	}
}
