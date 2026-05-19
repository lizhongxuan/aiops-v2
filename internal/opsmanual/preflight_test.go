package opsmanual

import "testing"

func TestRunPreflightPassed(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	manual.RunnableConditions.RequiredParams = []string{"target_instance", "backup_path"}
	manual.PreflightProbe = PreflightProbe{ID: "check_pg_backup", ReadOnly: true, RequiredOutputs: []string{"ssh_access", "pg_isready"}}
	mustSaveManual(t, repo, manual)
	service := NewService(repo)

	result, err := service.RunPreflight(PreflightRequest{
		ManualID:       manual.ID,
		WorkflowID:     manual.WorkflowRef.WorkflowID,
		OperationFrame: BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常", map[string]any{"target_name": "pg-ubuntu-01"}),
		Parameters:     map[string]any{"target_instance": "pg-ubuntu-01", "backup_path": "/data/backups"},
		TriggeredBy:    "ai-chat",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PreflightStatusPassed || !result.Ready || result.NextAction != "start_dry_run" {
		t.Fatalf("result = %#v, want passed/ready/start_dry_run", result)
	}
	if len(result.Evidence) != 2 || result.ArtifactType != "ops_manual_preflight_result" {
		t.Fatalf("evidence/artifact = %#v, want evidence and artifact type", result)
	}
}

func TestRunPreflightBlocksWhenRequiredParamsMissing(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupManual("manual-pg-backup-ubuntu", "ubuntu", "ssh", "workflow-pg-backup-ubuntu")
	manual.RunnableConditions.RequiredParams = []string{"target_instance", "backup_path"}
	manual.PreflightProbe = PreflightProbe{ID: "check_pg_backup", ReadOnly: true, RequiredOutputs: []string{"ssh_access"}}
	mustSaveManual(t, repo, manual)
	service := NewService(repo)

	result, err := service.RunPreflight(PreflightRequest{
		ManualID:       manual.ID,
		OperationFrame: BuildOperationFrame("在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，已确认 ssh_access 正常", map[string]any{"target_name": "pg-ubuntu-01"}),
		Parameters:     map[string]any{"target_instance": "pg-ubuntu-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PreflightStatusBlocked || result.Ready {
		t.Fatalf("result = %#v, want blocked/not ready", result)
	}
	if result.Reason == "" || result.NextAction != "collect_required_context" {
		t.Fatalf("result = %#v, want missing context next action", result)
	}
}

func TestRunPreflightBlocksWhenPermissionMissing(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisRcaManual()
	manual.PreflightProbe = PreflightProbe{ID: "check_redis", ReadOnly: true, RequiredOutputs: []string{"ssh_access", "metrics_available"}}
	mustSaveManual(t, repo, manual)
	service := NewService(repo)

	result, err := service.RunPreflight(PreflightRequest{
		ManualID:       manual.ID,
		OperationFrame: BuildOperationFrame("redis-local-01 prod vm ssh Redis used_memory_rss rising symptom metrics", map[string]any{"target_name": "redis-local-01"}),
		Parameters:     map[string]any{"target_instance": "redis-local-01", "simulate_permission_missing": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PreflightStatusBlocked || len(result.MissingPermissions) == 0 {
		t.Fatalf("result = %#v, want blocked with missing permissions", result)
	}
	if result.NextAction != "request_permission" || result.Ready {
		t.Fatalf("result = %#v, want permission request and not ready", result)
	}
}

func TestRunPreflightBlocksWhenProviderUnavailable(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisRcaManual()
	manual.PreflightProbe = PreflightProbe{ID: "check_redis", ReadOnly: true, RequiredOutputs: []string{"metrics_available"}}
	mustSaveManual(t, repo, manual)
	service := NewService(repo)

	result, err := service.RunPreflight(PreflightRequest{
		ManualID:       manual.ID,
		OperationFrame: BuildOperationFrame("redis-local-01 prod vm ssh Redis used_memory_rss rising symptom metrics", map[string]any{"target_name": "redis-local-01"}),
		Parameters:     map[string]any{"target_instance": "redis-local-01", "simulate_provider_unavailable": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Ready || result.NextAction == "start_dry_run" {
		t.Fatalf("result = %#v, provider unavailable must not enter dry run", result)
	}
	if result.Status != PreflightStatusBlocked && result.Status != PreflightStatusFailed {
		t.Fatalf("result = %#v, want blocked or failed", result)
	}
	if len(result.Evidence) == 0 || result.Evidence[0].Name != "provider_available" || result.Evidence[0].Status != "failed" {
		t.Fatalf("evidence = %#v, want provider unavailable evidence", result.Evidence)
	}
}

func TestRunPreflightBlocksWhenResourceUnreachable(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisRcaManual()
	manual.PreflightProbe = PreflightProbe{ID: "check_redis", ReadOnly: true, RequiredOutputs: []string{"target_reachable"}}
	mustSaveManual(t, repo, manual)
	service := NewService(repo)

	result, err := service.RunPreflight(PreflightRequest{
		ManualID:       manual.ID,
		OperationFrame: BuildOperationFrame("redis-local-01 prod vm ssh Redis used_memory_rss rising symptom metrics", map[string]any{"target_name": "redis-local-01"}),
		Parameters:     map[string]any{"target_instance": "redis-local-01", "simulate_target_missing": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PreflightStatusBlocked || result.Ready {
		t.Fatalf("result = %#v, want blocked/not ready for unreachable target", result)
	}
	if len(result.Evidence) == 0 || result.Evidence[0].Name != "target_reachable" || result.Evidence[0].Status != "failed" {
		t.Fatalf("evidence = %#v, want target_reachable failed evidence", result.Evidence)
	}
}

func TestRunPreflightNoProbeIsNotApplicable(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisRcaManual()
	mustSaveManual(t, repo, manual)
	service := NewService(repo)

	result, err := service.RunPreflight(PreflightRequest{
		ManualID:       manual.ID,
		OperationFrame: BuildOperationFrame("redis-local-01 prod vm ssh Redis used_memory_rss rising symptom metrics", map[string]any{"target_name": "redis-local-01"}),
		Parameters:     map[string]any{"target_instance": "redis-local-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != PreflightStatusNotApplicable || !result.Ready {
		t.Fatalf("result = %#v, want not_applicable ready", result)
	}
}
