package opsmanual

import "testing"

func TestShouldSearchForOpsManualsOnlyForResolutionOrManualIntent(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{name: "diagnosis only", text: "排查mservice异常问题", want: false},
		{name: "diagnosis with execution surface", text: "排查 mservice 异常，执行环境是 docker", want: false},
		{name: "question only", text: "为什么 checkout 延迟升高", want: false},
		{name: "rca only", text: "分析 Redis 内存上涨根因", want: false},
		{name: "solution advisory only", text: "给 Redis 内存上涨一个解决方案", want: false},
		{name: "how to solve advisory only", text: "分析 Redis 内存上涨怎么解决", want: false},
		{name: "how to operate advisory only", text: "如何重启 nginx 服务", want: false},
		{name: "explicit manual", text: "按运维手册处理 Redis 内存上涨", want: true},
		{name: "repair intent", text: "帮我修复 Redis 内存上涨问题", want: true},
		{name: "solve intent", text: "帮我解决 Redis 内存上涨问题", want: true},
		{name: "recovery intent", text: "恢复数据库服务，需要使用 Secret 引用", want: true},
		{name: "change intent", text: "重启 nginx 服务并验证恢复", want: true},
		{name: "backup intent", text: "给 MySQL 做一次备份", want: true},
		{name: "manual advisory intent", text: "请按运维手册给 Redis 内存上涨一个解决方案", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ShouldSearchForOpsManuals(tc.text); got != tc.want {
				t.Fatalf("ShouldSearchForOpsManuals(%q) = %v, want %v", tc.text, got, tc.want)
			}
		})
	}
}
