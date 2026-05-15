package opsmanual

import "testing"

func TestBuildOperationFrameRedisTriageNeedsMoreContext(t *testing.T) {
	frame := BuildOperationFrame("排查 Redis", nil)

	if frame.Target.Type != "redis" {
		t.Fatalf("target type = %q, want redis", frame.Target.Type)
	}
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("action = %q, want rca_or_repair", frame.Operation.Action)
	}
	for _, want := range []string{"target_instance", "execution_surface", "symptom", "metrics"} {
		if !contains(frame.Evidence.Missing, want) {
			t.Fatalf("missing = %#v, want %q", frame.Evidence.Missing, want)
		}
	}
}

func TestBuildOperationFrameRedisTriageExtractsEvidence(t *testing.T) {
	frame := BuildOperationFrame("生产 payment-api 的 Redis used_memory_rss 持续上涨，Coroot 显示 p95 升高，请排查", nil)

	if frame.Target.Type != "redis" {
		t.Fatalf("target type = %q, want redis", frame.Target.Type)
	}
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("action = %q, want rca_or_repair", frame.Operation.Action)
	}
	if frame.Environment.Env != "prod" {
		t.Fatalf("env = %q, want prod", frame.Environment.Env)
	}
	for _, want := range []string{"used_memory_rss", "coroot", "p95"} {
		if !contains(frame.Evidence.Provided, want) {
			t.Fatalf("provided evidence = %#v, want %q", frame.Evidence.Provided, want)
		}
	}
}

func TestBuildOperationFramePostgresBackupCentOSSSH(t *testing.T) {
	frame := BuildOperationFrame(
		"在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		map[string]any{"os_version": "7"},
	)

	if frame.Target.Type != "postgresql" {
		t.Fatalf("target type = %q, want postgresql", frame.Target.Type)
	}
	if frame.Operation.Action != "backup" {
		t.Fatalf("action = %q, want backup", frame.Operation.Action)
	}
	if frame.Target.Name != "pg-centos-01" {
		t.Fatalf("target name = %q, want pg-centos-01", frame.Target.Name)
	}
	if frame.Environment.OS != "centos" {
		t.Fatalf("os = %q, want centos", frame.Environment.OS)
	}
	if frame.Environment.OSVersion != "7" {
		t.Fatalf("os version = %q, want 7", frame.Environment.OSVersion)
	}
	if frame.Environment.ExecutionSurface != "ssh" {
		t.Fatalf("execution surface = %q, want ssh", frame.Environment.ExecutionSurface)
	}
	if frame.Metadata["backup_path"] != "/data/backups" {
		t.Fatalf("backup_path = %#v, want /data/backups", frame.Metadata["backup_path"])
	}
	if !hasAny(frame.Evidence.Provided, "ssh_access", "pg_isready") {
		t.Fatalf("provided evidence = %#v, want ssh_access and pg_isready", frame.Evidence.Provided)
	}
}

func TestBuildOperationFrameRedisTriageMissingContext(t *testing.T) {
	frame := BuildOperationFrame("排查 Redis", nil)

	if frame.Target.Type != "redis" {
		t.Fatalf("target type = %q, want redis", frame.Target.Type)
	}
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("action = %q, want rca_or_repair", frame.Operation.Action)
	}
	for _, required := range []string{"target_instance", "environment", "execution_surface", "symptom", "metrics"} {
		if !hasAny(frame.Evidence.Missing, required) {
			t.Fatalf("missing = %#v, want %s", frame.Evidence.Missing, required)
		}
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
