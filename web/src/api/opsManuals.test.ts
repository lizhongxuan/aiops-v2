import { describe, expect, it, vi } from "vitest";

import {
  createOpsManualsApi,
  normalizeOpsManual,
  normalizeOpsManualMatch,
  normalizeOpsManualParamResolutionResult,
  normalizeOpsManualPreflightResult,
  normalizeOpsManualSearchResult,
  normalizeRunRecordList,
} from "./opsManuals";

function createRecordingHttpClient(payload: unknown = { ok: true }) {
  const calls: Array<{ method: string; path: string; body?: unknown }> = [];
  return {
    calls,
    get: vi.fn((path: string) => {
      calls.push({ method: "GET", path });
      return Promise.resolve(payload);
    }),
    post: vi.fn((path: string, body: unknown) => {
      calls.push({ method: "POST", path, body });
      return Promise.resolve(payload);
    }),
  };
}

describe("ops manuals API", () => {
  it("routes list, get, retrieve, search, param resolution, preflight, candidate prepare, confirm, and run records through /api/v1/ops-manuals", async () => {
    const http = createRecordingHttpClient({ items: [] });
    const api = createOpsManualsApi(http);

    await api.list({ status: "verified", limit: 20, includeDeprecated: false });
    await api.get("manual/redis 1");
    await api.retrieve({ raw_text: "排查 Redis" });
    await api.searchOpsManuals({ text: "排查 Redis", limit: 3 });
    await api.resolveOpsManualParams({ manual_id: "manual-redis-memory", request_text: "排查 Redis" });
    await api.runOpsManualPreflight({ manual_id: "manual-redis-memory", parameters: { target_instance: "redis-01" } });
    await api.prepareCandidate({ workflow_id: "wf/redis 1" });
    await api.confirmCandidate("candidate/redis 1", { reviewer: "sre", review_decision: "approved" });
    await api.listRunRecords("manual/redis 1", { limit: 10 });

    expect(http.calls).toEqual([
      { method: "GET", path: "/api/v1/ops-manuals?status=verified&limit=20&includeDeprecated=false" },
      { method: "GET", path: "/api/v1/ops-manuals/manual%2Fredis%201" },
      { method: "POST", path: "/api/v1/ops-manuals/retrieve", body: { raw_text: "排查 Redis" } },
      { method: "POST", path: "/api/v1/ops-manuals/search", body: { text: "排查 Redis", limit: 3 } },
      {
        method: "POST",
        path: "/api/v1/ops-manuals/resolve-params",
        body: { manual_id: "manual-redis-memory", request_text: "排查 Redis" },
      },
      {
        method: "POST",
        path: "/api/v1/ops-manuals/preflight",
        body: { manual_id: "manual-redis-memory", parameters: { target_instance: "redis-01" } },
      },
      { method: "POST", path: "/api/v1/ops-manuals/candidates/prepare", body: { workflow_id: "wf/redis 1" } },
      {
        method: "POST",
        path: "/api/v1/ops-manuals/candidates/candidate%2Fredis%201/confirm",
        body: { reviewer: "sre", review_decision: "approved" },
      },
      { method: "GET", path: "/api/v1/ops-manuals/manual%2Fredis%201/run-records?limit=10" },
    ]);
  });

  it("normalizes snake_case and camelCase manual fields into camelCase views", () => {
    const manual = normalizeOpsManual({
      manual_id: "manual-redis-memory",
      title: "Redis 内存压力排障",
      status: "verified",
      version: "v1.2",
      owner: "sre",
      workflow_ref: {
        workflow_id: "workflow-redis-memory",
        workflow_version: "v3",
        workflow_digest: "sha256:abc",
      },
      operation: { target_type: "redis", action: "rca_or_repair", risk_level: "medium", stateful: true },
      applicability: { middleware: "redis", os: ["ubuntu"], platform: ["vm"], execution_surface: ["ssh"] },
      required_context: { required_inputs: ["target_instance"], required_evidence: ["metrics"] },
      preconditions: ["可连接目标实例"],
      validation: ["指标恢复"],
      cannot_use_when: ["无法确认实例"],
      run_record_summary: { success_count: 3, failure_count: 1, recent_result: "success" },
    });

    expect(manual).toMatchObject({
      id: "manual-redis-memory",
      title: "Redis 内存压力排障",
      status: "verified",
      workflowRef: {
        workflowId: "workflow-redis-memory",
        workflowVersion: "v3",
        workflowDigest: "sha256:abc",
      },
      operation: { targetType: "redis", action: "rca_or_repair", riskLevel: "medium", stateful: true },
      applicability: { middleware: "redis", os: ["ubuntu"], platform: ["vm"], executionSurface: ["ssh"] },
      requiredContext: { requiredInputs: ["target_instance"], requiredEvidence: ["metrics"] },
      runRecordSummary: { successCount: 3, failureCount: 1, recentResult: "success" },
    });
  });

  it("normalizes ops manual match state without exposing percentage scores", () => {
    const match = normalizeOpsManualMatch({
      manual: {
        id: "manual-redis-memory",
        title: "Redis 内存压力排障",
        workflowRef: { workflowId: "workflow-redis-memory" },
      },
      state: "direct",
      reasons: ["中间件匹配：redis"],
      missing_context: [],
      run_record_summary: { success_count: 7, failure_count: 0, latest_status: "passed", consecutive_failures: 0, suppressed: false },
      score: 0.92,
    });

    expect(match).toMatchObject({
      state: "direct",
      manualTitle: "Redis 内存压力排障",
      workflowRef: { workflowId: "workflow-redis-memory" },
      reasons: ["中间件匹配：redis"],
      runRecordSummary: { successCount: 7, failureCount: 0, latestStatus: "passed", consecutiveFailures: 0, suppressed: false },
    });
    expect(match).not.toHaveProperty("score");
    expect(match).not.toHaveProperty("percentage");
  });

  it("normalizes search_ops_manuals decisions without exposing scores", () => {
    const result = normalizeOpsManualSearchResult({
      decision: "adapt",
      summary: "需要适配",
      operation_frame: { target: { type: "postgresql" }, operation: { action: "backup" } },
      manuals: [
        {
          manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册", status: "verified" },
          bound_workflow_id: "workflow-pg-backup-ubuntu",
          workflow_status: "enabled",
          usable_mode: "adapt",
          score_breakdown: { final_score: 0.88 },
          preflight_status: "not_run",
          matched_fields: ["object_type", "operation_type"],
          environment_diffs: ["os"],
          blocked_reasons: ["workflow targets ubuntu apt/systemd but current host is centos/yum/systemd"],
          recommended_action: "generate_workflow_variant",
          score: 0.76,
        },
      ],
      next_questions: [],
      recommended_next_action: "生成 CentOS 适配工作流草稿",
      searched_fields: ["object_type", "operation_type", "environment"],
      percentage: 76,
    });

    expect(result).toMatchObject({
      decision: "adapt",
      summary: "需要适配",
      operationFrame: { objectType: "postgresql", operationType: "backup" },
      manuals: [
        {
          manualId: "manual-pg-backup-ubuntu",
          title: "PostgreSQL 备份 Ubuntu 运维手册",
          manualStatus: "verified",
          workflowStatus: "enabled",
          boundWorkflowId: "workflow-pg-backup-ubuntu",
          usableMode: "adapt",
          preflightStatus: "not_run",
          scoreBreakdown: { finalScore: 0.88 },
          matchedFields: ["object_type", "operation_type"],
          environmentDiffs: ["os"],
          recommendedAction: "generate_workflow_variant",
        },
      ],
      searchedFields: ["object_type", "operation_type", "environment"],
    });
    expect(result).not.toHaveProperty("score");
    expect(result).not.toHaveProperty("percentage");
    expect(result.manuals[0]).not.toHaveProperty("score");
    expect(result.manuals[0]).not.toHaveProperty("percentage");
  });

  it("normalizes preflight results for Agent-to-UI cards", () => {
    const result = normalizeOpsManualPreflightResult({
      status: "passed",
      ready: true,
      manual_id: "manual-pg-backup",
      workflow_id: "workflow-pg-backup",
      probe_id: "pg-backup-readonly",
      evidence: [{ name: "ssh_access", status: "passed", value: true }],
      missing_permissions: [],
      environment_diffs: [],
      next_action: "start_dry_run",
      checked_at: "2026-05-15T09:30:00Z",
      artifact_type: "ops_manual_preflight_result",
    });

    expect(result).toMatchObject({
      status: "passed",
      ready: true,
      manualId: "manual-pg-backup",
      workflowId: "workflow-pg-backup",
      probeId: "pg-backup-readonly",
      nextAction: "start_dry_run",
      artifactType: "ops_manual_preflight_result",
    });
    expect(result.evidence[0]).toMatchObject({ name: "ssh_access", status: "passed", value: true });
  });

  it("normalizes parameter resolution results for dynamic Agent-to-UI forms", () => {
    const result = normalizeOpsManualParamResolutionResult({
      status: "ambiguous",
      manual_id: "manual-redis-memory",
      workflow_id: "workflow-redis-memory",
      resolved_params: [{ id: "target_host", value: "server-local", source: "selected_host", confidence: 1 }],
      fields: [
        {
          id: "redis_instance",
          label: "Redis 实例",
          type: "resource_ref",
          ui_control: "select",
          required: true,
          candidates: [{ value: "docker:redis-1", label: "redis-1", source: "docker", confidence: 0.91 }],
        },
      ],
      next_action: "await_user_input",
      artifact_type: "ops_manual_param_resolution",
    });

    expect(result).toMatchObject({
      status: "ambiguous",
      manualId: "manual-redis-memory",
      workflowId: "workflow-redis-memory",
      nextAction: "await_user_input",
      artifactType: "ops_manual_param_resolution",
      resolvedParams: [{ id: "target_host", value: "server-local", source: "selected_host", confidence: 1 }],
      fields: [
        {
          id: "redis_instance",
          label: "Redis 实例",
          type: "resource_ref",
          uiControl: "select",
          required: true,
          candidates: [{ value: "docker:redis-1", label: "redis-1", source: "docker", confidence: 0.91 }],
        },
      ],
    });
  });

  it("maps legacy search decision aliases to canonical states", () => {
    expect(normalizeOpsManualSearchResult({ decision: "direct" }).decision).toBe("direct_execute");
    expect(normalizeOpsManualSearchResult({ decision: "need_more_info" }).decision).toBe("need_info");
    expect(normalizeOpsManualSearchResult({ decision: "adapt_required" }).decision).toBe("adapt");
    expect(normalizeOpsManualSearchResult({ decision: "reference" }).decision).toBe("reference_only");
    expect(normalizeOpsManualSearchResult({ decision: "unknown" }).decision).toBe("no_match");
  });

  it("normalizes run record list wrappers", () => {
    const records = normalizeRunRecordList({
      run_records: [
        {
          run_id: "run-1",
          manual_id: "manual-redis-memory",
          workflow_id: "workflow-redis-memory",
          dry_run_status: "passed",
          execution_status: "success",
          validation_status: "passed",
          failure_reason: "",
          operator: "sre",
          completed_at: "2026-05-14T10:00:00+08:00",
        },
      ],
      total_count: 1,
    });

    expect(records).toMatchObject({
      items: [
        {
          id: "run-1",
          manualId: "manual-redis-memory",
          workflowId: "workflow-redis-memory",
          dryRunStatus: "passed",
          executionStatus: "success",
          validationStatus: "passed",
        },
      ],
      total: 1,
    });
  });
});
