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

func TestBuildOperationFrameEnglishTroubleshootRedis(t *testing.T) {
	frame := BuildOperationFrame("troubleshoot Redis on current host server-local with ops manuals and read-only discovery", nil)

	if frame.Target.Type != "redis" {
		t.Fatalf("target type = %q, want redis", frame.Target.Type)
	}
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("action = %q, want rca_or_repair", frame.Operation.Action)
	}
	if frame.Target.Name != "" {
		t.Fatalf("target name = %q, want no guessed target instance from current host wording", frame.Target.Name)
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

func TestBuildOperationFrameRedisStatusCheck(t *testing.T) {
	frame := BuildOperationFrame("检查 Redis 状态", nil)

	if frame.Target.Type != "redis" {
		t.Fatalf("target type = %q, want redis", frame.Target.Type)
	}
	if frame.Operation.Action != "status_check" || frame.Intent != "status_check" {
		t.Fatalf("action/intent = %q/%q, want status_check; frame=%#v", frame.Operation.Action, frame.Intent, frame)
	}
	if frame.Target.Name != "" {
		t.Fatalf("target name = %q, want empty target for status check without instance name; frame=%#v", frame.Target.Name, frame)
	}
	if frame.Risk.Level != "medium" {
		t.Fatalf("risk level = %q, want medium for stateful middleware status check", frame.Risk.Level)
	}
}

func TestBuildOperationFramePostgresBackupCentOSSSH(t *testing.T) {
	frame := BuildOperationFrame(
		"在 CentOS 主机 pg-centos-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
		map[string]any{"os_version": "7", "target_name": "pg-centos-01"},
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

func TestBuildOperationFramePGBackupAlias(t *testing.T) {
	frame := BuildOperationFrame("PG backup on pg-01 via ssh to /data/backups", nil)
	if frame.Target.Type != "postgresql" {
		t.Fatalf("target type = %q, want postgresql", frame.Target.Type)
	}
	if frame.Operation.Action != "backup" {
		t.Fatalf("action = %q, want backup", frame.Operation.Action)
	}
}

func TestBuildOperationFramePostgresBackUpPhrase(t *testing.T) {
	frame := BuildOperationFrame("Back up PostgreSQL on current host using ops manual", nil)
	if frame.Target.Type != "postgresql" {
		t.Fatalf("target type = %q, want postgresql", frame.Target.Type)
	}
	if frame.Operation.Action != "backup" {
		t.Fatalf("action = %q, want backup; frame=%#v", frame.Operation.Action, frame)
	}
	if frame.Target.Name == "up" {
		t.Fatalf("target name = %q, phrasal verb must not be treated as PostgreSQL instance; frame=%#v", frame.Target.Name, frame)
	}
}

func TestBuildOperationFramePostgresBackupDoesNotUseChineseContextAsTarget(t *testing.T) {
	frame := BuildOperationFrame("请按运维手册给本机 PostgreSQL 做备份，当前主机 server-local，先只做参数解析和预检，不执行变更；备份路径我还没确定。", nil)
	if frame.Target.Type != "postgresql" {
		t.Fatalf("target type = %q, want postgresql", frame.Target.Type)
	}
	if frame.Operation.Action != "backup" {
		t.Fatalf("action = %q, want backup; frame=%#v", frame.Operation.Action, frame)
	}
	if frame.Target.Name != "" {
		t.Fatalf("target name = %q, want empty when user only says local PostgreSQL; frame=%#v", frame.Target.Name, frame)
	}
}

func TestBuildOperationFramePostgresBackupKeepsExplicitInstance(t *testing.T) {
	frame := BuildOperationFrame("请按运维手册给 PostgreSQL 做备份，备份到 /data/backups。", map[string]any{"target_name": "pg-prod-01"})
	if frame.Target.Type != "postgresql" {
		t.Fatalf("target type = %q, want postgresql", frame.Target.Type)
	}
	if frame.Target.Name != "pg-prod-01" {
		t.Fatalf("target name = %q, want pg-prod-01; frame=%#v", frame.Target.Name, frame)
	}
}

func TestBuildOperationFrameDoesNotGuessTargetNameFromFreeText(t *testing.T) {
	frame := BuildOperationFrame("请按运维手册给 pg-prod-01 上的 PostgreSQL 做备份，备份到 /data/backups。", nil)
	if frame.Target.Type != "postgresql" {
		t.Fatalf("target type = %q, want postgresql", frame.Target.Type)
	}
	if frame.Target.Name != "" {
		t.Fatalf("target name = %q, want empty without explicit operation_frame or metadata target; frame=%#v", frame.Target.Name, frame)
	}
	if !contains(frame.Evidence.Missing, "target_instance") {
		t.Fatalf("missing = %#v, want target_instance when target is not explicitly structured", frame.Evidence.Missing)
	}
}

func TestBuildOperationFrameK8sPodCrashLoopBackOff(t *testing.T) {
	frame := BuildOperationFrame("checkout pod payment-api-7c9f CrashLoopBackOff in namespace prod by kubectl", nil)
	if frame.Target.Type != "kubernetes_pod" {
		t.Fatalf("target type = %q, want kubernetes_pod", frame.Target.Type)
	}
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("action = %q, want rca_or_repair", frame.Operation.Action)
	}
	if frame.Environment.Platform != "kubernetes" || frame.Environment.ExecutionSurface != "kubectl" {
		t.Fatalf("environment = %#v, want kubernetes/kubectl", frame.Environment)
	}
	if frame.Target.Name == "CrashLoopBackOff" {
		t.Fatalf("target name = %q, CrashLoopBackOff is a symptom, not a pod name", frame.Target.Name)
	}
	if !hasAny(frame.Evidence.Provided, "pod_restart") {
		t.Fatalf("provided evidence = %#v, want pod_restart", frame.Evidence.Provided)
	}
}

func TestBuildOperationFrameK8sPodExtractsNamespaceAndPodName(t *testing.T) {
	frame := BuildOperationFrame("用运维手册排查 Kubernetes Pod，kubectl 权限正常，rbac_read_ok，pod_exists 已确认", map[string]any{"namespace": "payment", "pod_name": "pay-api-7d9"})

	if frame.Target.Type != "kubernetes_pod" {
		t.Fatalf("target type = %q, want kubernetes_pod", frame.Target.Type)
	}
	if frame.Target.Name != "pay-api-7d9" {
		t.Fatalf("target name = %q, want pay-api-7d9", frame.Target.Name)
	}
	if frame.TargetScope.Namespace != "payment" {
		t.Fatalf("namespace = %q, want payment", frame.TargetScope.Namespace)
	}
	if frame.RequiredParams["namespace"] != "payment" || frame.RequiredParams["pod_name"] != "pay-api-7d9" {
		t.Fatalf("required params = %#v, want namespace and pod_name", frame.RequiredParams)
	}
	if !hasAny(frame.Evidence.Provided, "kubectl_access", "rbac_read_ok", "pod_exists") {
		t.Fatalf("provided evidence = %#v, want kubectl_access/rbac_read_ok/pod_exists", frame.Evidence.Provided)
	}
	if contains(frame.Evidence.Missing, "environment") {
		t.Fatalf("missing = %#v, namespace-scoped kubernetes request should not require environment", frame.Evidence.Missing)
	}
	if frame.Risk.Level != "medium" {
		t.Fatalf("risk level = %q, want medium", frame.Risk.Level)
	}
}

func TestBuildOperationFrameFromContextComposerLabels(t *testing.T) {
	text := "补充必要信息，继续下一步自动排查：\n" +
		"关联上下文：redis / rca_or_repair；Redis SSH 排障运维手册\n" +
		"目标实例/服务：redis-local-01\n" +
		"环境：prod\n" +
		"执行方式：ssh\n" +
		"现象/指标：used_memory_rss 持续升高，p95 升高；ssh_access 正常，redis_ping 正常，metrics_available，metrics 可读；只读排查，不重启不写入"
	frame := BuildOperationFrame(text, nil)
	if frame.ObjectType != "redis" || frame.OperationType != "rca_or_repair" {
		t.Fatalf("frame object/action = %q/%q, want redis/rca_or_repair; frame=%#v", frame.ObjectType, frame.OperationType, frame)
	}
	if frame.Target.Name != "redis-local-01" {
		t.Fatalf("target name = %q, want redis-local-01; frame=%#v", frame.Target.Name, frame)
	}
	if frame.Environment.Env != "prod" || frame.Environment.ExecutionSurface != "ssh" || frame.Environment.Platform != "vm" {
		t.Fatalf("environment = %#v, want prod ssh vm", frame.Environment)
	}
	if !hasAny(frame.Evidence.Provided, "symptom", "metrics", "ssh_access") {
		t.Fatalf("evidence = %#v, want symptom metrics ssh_access", frame.Evidence.Provided)
	}
}

func TestBuildOperationFrameKafkaLag(t *testing.T) {
	frame := BuildOperationFrame("Kafka consumer group lag 持续升高，需要排查 broker 和 partition rebalance，先只读分析。", map[string]any{"target_name": "checkout-prod"})
	if frame.Target.Type != "kafka" {
		t.Fatalf("target type = %q, want kafka", frame.Target.Type)
	}
	if frame.Target.Name != "checkout-prod" {
		t.Fatalf("target name = %q, want checkout-prod", frame.Target.Name)
	}
	if frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("action = %q, want rca_or_repair", frame.Operation.Action)
	}
	if !hasAny(frame.Evidence.Provided, "symptom", "metrics", "readonly") {
		t.Fatalf("provided evidence = %#v, want kafka lag readonly evidence", frame.Evidence.Provided)
	}
}

func TestBuildOperationFrameReadonlyRedisDoesNotRestart(t *testing.T) {
	frame := BuildOperationFrame("只读排查 Redis，不重启服务，只看 metrics", map[string]any{"target_name": "redis-01"})
	if frame.Target.Type != "redis" || frame.Operation.Action != "rca_or_repair" {
		t.Fatalf("frame = %#v, want redis rca", frame)
	}
	if frame.Risk.ServiceRestart {
		t.Fatalf("service restart = true, want false for explicit no restart")
	}
	if !hasAny(frame.Evidence.Provided, "readonly") {
		t.Fatalf("provided evidence = %#v, want readonly", frame.Evidence.Provided)
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
