package opsmanual

import "testing"

func TestTaxonomyNormalizesObjectAliases(t *testing.T) {
	cases := map[string]string{
		"PG backup":                              "postgresql",
		"postgres 主库备份":                          "postgresql",
		"checkout pod CrashLoopBackOff":          "kubernetes_pod",
		"k8s deployment 频繁重启":                    "kubernetes_workload",
		"mysqldump mysql-01":                     "mysql",
		"Kafka consumer group checkout-prod lag": "kafka",
		"Redis used_memory_rss rising":           "redis",
		"network latency between services":       "network",
	}
	for text, want := range cases {
		if got := detectObjectType(text); got != want {
			t.Fatalf("detectObjectType(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTaxonomyNormalizesOperationAliases(t *testing.T) {
	cases := map[string]string{
		"排查 Redis":                               "rca_or_repair",
		"checkout pod CrashLoop":                 "rca_or_repair",
		"Kafka consumer group checkout-prod lag": "rca_or_repair",
		"PG backup":                              "backup",
		"恢复 MySQL 数据":                            "restore",
		"restart nginx":                          "restart",
		"scale deployment":                       "scale",
		"部署 PostgreSQL 主从":                       "deploy",
		"migration schema":                       "migration",
		"status check redis":                     "status_check",
		"检查 Redis 状态":                          "status_check",
		"看一下 pg 运行状态":                         "status_check",
	}
	for text, want := range cases {
		if got := detectOperationType(text); got != want {
			t.Fatalf("detectOperationType(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTaxonomyKeepsTroubleshootingBeforeStatusCheck(t *testing.T) {
	cases := map[string]string{
		"检查 Redis 状态是否异常，需要排查": "rca_or_repair",
		"Redis 状态报错请诊断":        "rca_or_repair",
	}
	for text, want := range cases {
		if got := detectOperationType(text); got != want {
			t.Fatalf("detectOperationType(%q) = %q, want %q", text, got, want)
		}
	}
}

func TestTaxonomyDetectsNegativeRestartIntent(t *testing.T) {
	for _, text := range []string{"只读排查 Redis，不重启", "readonly no restart", "do not restart service"} {
		if hasPositiveRestartIntent(normalizeText(text)) {
			t.Fatalf("hasPositiveRestartIntent(%q) = true, want false", text)
		}
	}
}
