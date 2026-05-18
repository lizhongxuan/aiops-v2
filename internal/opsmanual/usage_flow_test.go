package opsmanual

import "testing"

func TestSearchOpsManualsRedisUsageFlowNeedsContextThenOffersConfirmedWorkflow(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, redisRcaManual())

	needInfo, err := SearchOpsManuals(repo, SearchOpsManualsRequest{Text: "排查 Redis"})
	if err != nil {
		t.Fatal(err)
	}
	if needInfo.Decision != DecisionNeedInfo {
		t.Fatalf("decision = %q, want need_info; result=%#v", needInfo.Decision, needInfo)
	}
	if len(needInfo.NextQuestions) == 0 {
		t.Fatalf("next questions empty, want missing context questions")
	}
	if len(needInfo.Manuals) == 0 || needInfo.Manuals[0].RecommendedAction == "run_bound_workflow" {
		t.Fatalf("need_info manuals = %#v, must not offer workflow execution", needInfo.Manuals)
	}

	direct, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在生产 vm 主机 redis-local-01 上通过 ssh 排查 Redis used_memory_rss 持续上涨的症状，已有 metrics 指标证据，风险 medium，只做只读采集，无写入、无服务变更",
		Metadata: map[string]any{"target_name": "redis-local-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if direct.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute; result=%#v", direct.Decision, direct)
	}
	if len(direct.Manuals) == 0 || direct.Manuals[0].RecommendedAction != "run_preflight_probe" {
		t.Fatalf("direct manuals = %#v, want run_preflight_probe recommendation", direct.Manuals)
	}
	if !stringsContains(direct.Summary, "用户确认前不会执行 Runner Workflow") {
		t.Fatalf("summary = %q, want explicit user confirmation boundary", direct.Summary)
	}

	records, err := repo.ListRunRecords(ListRunRecordsRequest{ManualID: "manual-redis-rca-ssh"})
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("search created run records = %#v, want search to stay read-only", records)
	}

	mustSaveRunRecord(t, repo, RunRecord{
		ID:               "rr-confirmed-redis",
		ManualID:         "manual-redis-rca-ssh",
		WorkflowID:       "workflow-redis-rca-ssh",
		ExecutionStatus:  "success",
		ValidationStatus: "passed",
		CompletedAt:      "2026-05-15T04:00:00Z",
	})
	afterConfirmedRun, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "在生产 vm 主机 redis-local-01 上通过 ssh 排查 Redis used_memory_rss 持续上涨的症状，已有 metrics 指标证据，风险 medium，只做只读采集，无写入、无服务变更",
		Metadata: map[string]any{"target_name": "redis-local-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if afterConfirmedRun.Manuals[0].RunRecordSummary.SuccessCount != 1 || afterConfirmedRun.Manuals[0].RunRecordSummary.RecentResult != "passed" {
		t.Fatalf("run summary = %#v, want confirmed successful workflow evidence", afterConfirmedRun.Manuals[0].RunRecordSummary)
	}
}

func TestSearchOpsManualsEnglishReadonlyRedisPromptDoesNotTreatNoRestartAsHighRisk(t *testing.T) {
	repo := NewMemoryStore()
	mustSaveManual(t, repo, redisRcaManual())

	result, err := SearchOpsManuals(repo, SearchOpsManualsRequest{
		Text:     "redis-local-01 prod vm ssh Redis used_memory_rss rising symptom metrics medium readonly no restart no write use search_ops_manuals",
		Metadata: map[string]any{"target_name": "redis-local-01"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Decision != DecisionDirectExecute {
		t.Fatalf("decision = %q, want direct_execute; result=%#v", result.Decision, result)
	}
	if result.OperationFrame.Risk.ServiceRestart {
		t.Fatalf("risk = %#v, no restart must not be treated as a restart intent", result.OperationFrame.Risk)
	}
	if len(result.Manuals) == 0 || len(result.Manuals[0].MissingFields) != 0 || len(result.Manuals[0].BlockedReasons) != 0 {
		t.Fatalf("manual hit = %#v, want no missing fields or risk blockers", result.Manuals)
	}
}
