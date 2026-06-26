package opsmanual

import (
	"encoding/json"
	"strings"
	"testing"
)

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
		"检查 Redis 状态":                            "status_check",
		"看一下 pg 运行状态":                            "status_check",
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

func TestTaxonomyReturnsCapabilityMetadata(t *testing.T) {
	metadata := BuildTaxonomyMetadata("只读排查 PostgreSQL timeline not a child standby recovery，包含 pg_isready 和 restore_command", nil)

	if len(metadata.CapabilityCandidates) == 0 {
		t.Fatalf("CapabilityCandidates = %#v, want candidates", metadata.CapabilityCandidates)
	}
	candidate := metadata.CapabilityCandidates[0]
	if candidate.ResourceKind != "postgresql" {
		t.Fatalf("ResourceKind = %q, want postgresql: %#v", candidate.ResourceKind, metadata)
	}
	if candidate.Capability != "rca_or_repair" {
		t.Fatalf("Capability = %q, want rca_or_repair: %#v", candidate.Capability, metadata)
	}
	if !containsString(candidate.EvidenceKinds, "pg_isready") && !containsString(candidate.EvidenceKinds, "readonly") {
		t.Fatalf("EvidenceKinds = %#v, want evidence metadata candidate", candidate.EvidenceKinds)
	}
}

func TestTaxonomyMetadataDoesNotReturnRuntimeRoute(t *testing.T) {
	metadata := BuildTaxonomyMetadata("在生产 Redis 上排查 timeout latency，不要重启", nil)
	data, err := json.Marshal(metadata)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	serialized := strings.ToLower(string(data))
	for _, forbidden := range []string{"runtime_route", "runtimeroute", "route_mode", "aiops.route", "host_bound_ops", "evidence_rca"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("taxonomy metadata leaked runtime route marker %q: %s", forbidden, string(data))
		}
	}
}
