package opsmanual

import "testing"

func TestResolveOpsManualParamsAutoResolvesSingleRedis(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "排查 Redis",
		ManualID:    manual.ID,
		Metadata: map[string]any{
			"selected_host": "server-local",
			"resource_candidates": []any{
				map[string]any{"id": "docker:aiops-redis", "name": "aiops-redis", "type": "redis", "surface": "docker exec aiops-redis", "source": "docker", "confidence": 0.92},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionResolved || result.NextAction != "run_preflight" {
		t.Fatalf("result = %#v, want resolved run_preflight", result)
	}
	if len(result.MissingParams) != 0 || len(result.AmbiguousParams) != 0 || len(result.Fields) != 0 {
		t.Fatalf("result = %#v, want no missing/ambiguous/form fields", result)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_host", "server-local") ||
		!resolvedParamValue(result.ResolvedParams, "target_instance", "docker:aiops-redis") {
		t.Fatalf("resolved params = %#v", result.ResolvedParams)
	}
}

func TestResolveOpsManualParamsUsesInjectedDiscoveryWhenMetadataMissing(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, WithResourceDiscovery(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{ID: "docker:aiops-redis", Name: "aiops-redis", Type: "redis", Surface: "docker exec aiops-redis", Source: "docker", Confidence: 0.92, Evidence: "docker ps"},
		},
	}))
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "排查 Redis",
		ManualID:    manual.ID,
		Metadata:    map[string]any{"selected_host": "server-local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionResolved {
		t.Fatalf("status = %q result=%#v, want resolved", result.Status, result)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_instance", "docker:aiops-redis") {
		t.Fatalf("resolved params = %#v, want docker redis from injected discovery", result.ResolvedParams)
	}
	if !resolvedParamValue(result.ResolvedParams, "execution_surface", "docker exec aiops-redis") {
		t.Fatalf("resolved params = %#v, want docker exec surface from discovery", result.ResolvedParams)
	}
}

func TestResolveOpsManualParamsNoRedisDiscoveryDoesNotAskFixedFourFields(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, WithResourceDiscovery(fakeResourceDiscovery{}))
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "排查 Redis",
		ManualID:    manual.ID,
		Metadata:    map[string]any{"selected_host": "server-local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionNeedUserInput {
		t.Fatalf("status = %q result=%#v, want need_user_input", result.Status, result)
	}
	if len(result.Fields) != 1 || result.Fields[0].ID != "target_instance" {
		t.Fatalf("fields = %#v, want only target_instance field", result.Fields)
	}
	if result.MissingParams[0].Reason == "" || result.MissingParams[0].Reason == "no candidate" {
		t.Fatalf("missing params = %#v, want read-only discovery reason", result.MissingParams)
	}
	if result.Fields[0].Placeholder == "" || result.Fields[0].Placeholder == "选择或填写目标实例" {
		t.Fatalf("fields = %#v, want no-resource placeholder", result.Fields)
	}
}

func TestResolveOpsManualParamsAsksOnlyForRedisInstanceWhenAmbiguous(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "排查 Redis",
		ManualID:    manual.ID,
		Metadata: map[string]any{
			"selected_host": "server-local",
			"resource_candidates": []any{
				map[string]any{"id": "docker:redis-a", "name": "redis-a", "type": "redis", "surface": "docker exec redis-a", "source": "docker", "confidence": 0.91},
				map[string]any{"id": "docker:redis-b", "name": "redis-b", "type": "redis", "surface": "docker exec redis-b", "source": "docker", "confidence": 0.91},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionAmbiguous || len(result.AmbiguousParams) != 1 || len(result.Fields) != 1 {
		t.Fatalf("result = %#v, want one ambiguous field", result)
	}
	if result.Fields[0].ID != "target_instance" || len(result.Fields[0].Candidates) != 2 {
		t.Fatalf("fields = %#v, want target_instance select with two candidates", result.Fields)
	}
}

func TestResolveOpsManualParamsExplicitResourceNameDisambiguatesDiscovery(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "Troubleshoot Redis instance redis-b on current host server-local using ops manuals",
		ManualID:    manual.ID,
		OperationFrame: OperationFrame{
			ObjectType: "redis",
			Target:     OperationTarget{Type: "redis", Name: "redis-b"},
			Operation:  OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		},
		Metadata: map[string]any{
			"selected_host": "server-local",
			"resource_candidates": []any{
				map[string]any{"id": "docker:redis-a", "name": "redis-a", "type": "redis", "surface": "docker exec redis-a", "source": "docker", "confidence": 0.91},
				map[string]any{"id": "docker:redis-b", "name": "redis-b", "type": "redis", "surface": "docker exec redis-b", "source": "docker", "confidence": 0.91},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionResolved {
		t.Fatalf("result = %#v, want explicit resource to resolve", result)
	}
	if len(result.AmbiguousParams) != 0 || len(result.Fields) != 0 {
		t.Fatalf("result = %#v, want no ambiguous form", result)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_instance", "docker:redis-b") {
		t.Fatalf("resolved params = %#v, want docker:redis-b from discovery", result.ResolvedParams)
	}
}

func TestResolveOpsManualParamsRawTextExplicitResourceDisambiguatesDiscovery(t *testing.T) {
	repo := NewMemoryStore()
	manual := redisParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "Troubleshoot Redis instance redis-b on current host server-local using ops manuals",
		ManualID:    manual.ID,
		Metadata: map[string]any{
			"selected_host": "server-local",
			"resource_candidates": []any{
				map[string]any{"id": "docker:redis-a", "name": "redis-a", "type": "redis", "surface": "docker exec redis-a", "source": "docker", "confidence": 0.91},
				map[string]any{"id": "docker:redis-b", "name": "redis-b", "type": "redis", "surface": "docker exec redis-b", "source": "docker", "confidence": 0.91},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionResolved {
		t.Fatalf("result = %#v, want raw-text explicit resource to resolve", result)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_instance", "docker:redis-b") {
		t.Fatalf("resolved params = %#v, want docker:redis-b from discovery", result.ResolvedParams)
	}
}

func TestResolveOpsManualParamsPGBackupOnlyAsksBackupPath(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo)
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "给 pg-01 做 PostgreSQL 备份",
		ManualID:    manual.ID,
		Metadata: map[string]any{
			"selected_host": "pg-01",
			"resource_candidates": []any{
				map[string]any{"id": "pg-01:5432", "name": "pg-01:5432", "type": "postgresql", "source": "host_readonly", "confidence": 0.9},
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionNeedUserInput || len(result.MissingParams) != 1 || result.MissingParams[0].ID != "backup_path" {
		t.Fatalf("result = %#v, want only backup_path missing", result)
	}
	if len(result.Fields) != 1 || result.Fields[0].ID != "backup_path" {
		t.Fatalf("fields = %#v, want one backup_path field", result.Fields)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_host", "pg-01") {
		t.Fatalf("resolved params = %#v, want pg-01 target host", result.ResolvedParams)
	}
}

func TestResolveOpsManualParamsPGDockerBackupOnlyAsksBackupPath(t *testing.T) {
	repo := NewMemoryStore()
	manual := pgBackupParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, WithResourceDiscovery(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{ID: "docker:aiops-postgres", Name: "aiops-postgres", Type: "postgresql", Surface: "docker exec aiops-postgres", Source: "docker", Confidence: 0.92, Evidence: "docker ps"},
		},
	}))
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "给 PostgreSQL 做备份",
		ManualID:    manual.ID,
		Metadata:    map[string]any{"selected_host": "server-local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionNeedUserInput || len(result.Fields) != 1 || result.Fields[0].ID != "backup_path" {
		t.Fatalf("result = %#v, want only backup_path field", result)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_host", "server-local") ||
		!resolvedParamValue(result.ResolvedParams, "target_instance", "docker:aiops-postgres") {
		t.Fatalf("resolved params = %#v, want docker postgres host/instance", result.ResolvedParams)
	}
}

func TestResolveOpsManualParamsMySQLDockerBackupOnlyAsksBackupPath(t *testing.T) {
	repo := NewMemoryStore()
	manual := mysqlBackupParamManual()
	if err := repo.SaveManual(manual); err != nil {
		t.Fatal(err)
	}
	service := NewService(repo, WithResourceDiscovery(fakeResourceDiscovery{
		resources: []ResourceCandidate{
			{ID: "docker:aiops-mysql", Name: "aiops-mysql", Type: "mysql", Surface: "docker exec aiops-mysql", Source: "docker", Confidence: 0.92, Evidence: "docker ps"},
		},
	}))
	result, err := service.ResolveOpsManualParams(ResolveOpsManualParamsRequest{
		RequestText: "对 MySQL 做备份",
		ManualID:    manual.ID,
		Metadata:    map[string]any{"selected_host": "server-local"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != ParamResolutionNeedUserInput || len(result.Fields) != 1 || result.Fields[0].ID != "backup_path" {
		t.Fatalf("result = %#v, want only backup_path field", result)
	}
	if !resolvedParamValue(result.ResolvedParams, "target_host", "server-local") ||
		!resolvedParamValue(result.ResolvedParams, "target_instance", "docker:aiops-mysql") {
		t.Fatalf("resolved params = %#v, want docker mysql host/instance", result.ResolvedParams)
	}
}

func redisParamManual() OpsManual {
	return OpsManual{
		ID:          "manual-redis-rca",
		Title:       "Redis SSH 排障运维手册",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-redis-rca"},
		Operation:   OperationProfile{TargetType: "redis", Action: "rca_or_repair"},
		Applicability: ApplicabilityProfile{
			Middleware:       "redis",
			ExecutionSurface: []string{"ssh"},
		},
		RequiredContext:  RequiredContext{RequiredInputs: []string{"target_instance", "execution_surface"}},
		Validation:       []string{"INFO memory 正常"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "Redis manual",
	}
}

func pgBackupParamManual() OpsManual {
	return OpsManual{
		ID:          "manual-pg-backup",
		Title:       "PostgreSQL SSH 备份运维手册",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-pg-backup"},
		Operation:   OperationProfile{TargetType: "postgresql", Action: "backup"},
		RequiredContext: RequiredContext{
			RequiredInputs: []string{"target_instance", "backup_path"},
		},
		ParameterRules:   map[string]ParameterRule{"backup_path": {Required: true, Source: "user", Validation: "path"}},
		Validation:       []string{"backup file exists"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "PG backup manual",
	}
}

func mysqlBackupParamManual() OpsManual {
	return OpsManual{
		ID:          "manual-mysql-backup",
		Title:       "MySQL SSH 备份运维手册",
		Status:      ManualStatusVerified,
		WorkflowRef: WorkflowRef{WorkflowID: "workflow-mysql-backup"},
		Operation:   OperationProfile{TargetType: "mysql", Action: "backup"},
		RequiredContext: RequiredContext{
			RequiredInputs: []string{"target_instance", "backup_path"},
		},
		ParameterRules:   map[string]ParameterRule{"backup_path": {Required: true, Source: "user", Validation: "path"}},
		Validation:       []string{"backup file exists"},
		CannotUseWhen:    []string{"目标实例未知"},
		DocumentMarkdown: "MySQL backup manual",
	}
}

func resolvedParamValue(params []ResolvedParam, id, want string) bool {
	for _, param := range params {
		if param.ID == id && param.Value == want {
			return true
		}
	}
	return false
}
