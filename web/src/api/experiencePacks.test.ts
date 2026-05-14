import { beforeEach, describe, expect, it, vi } from "vitest";

import {
  createExperiencePacksApi,
  normalizeExperienceMatch,
  normalizeExperiencePack,
  normalizeExperiencePackList,
  normalizeRunnerCandidate,
  normalizeReuseRecordList,
} from "./experiencePacks";

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
    patch: vi.fn((path: string, body: unknown) => {
      calls.push({ method: "PATCH", path, body });
      return Promise.resolve(payload);
    }),
    put: vi.fn((path: string, body: unknown) => {
      calls.push({ method: "PUT", path, body });
      return Promise.resolve(payload);
    }),
  };
}

describe("experience packs API", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("routes GEP list, detail, retrieve, suggestion, review, validation, and runner binding calls", async () => {
    const http = createRecordingHttpClient({ items: [] });
    const api = createExperiencePacksApi(http);

    await api.listPacks({ status: "enabled", category: "repair", usageShape: "guided", middleware: "postgres", hasRunnerBinding: true });
    await api.getPack("pack/a 1");
    await api.getSkill("pack/a 1");
    await api.listPackFiles("pack/a 1");
    await api.listCapsules("pack/a 1");
    await api.listEvents("pack/a 1");
    await api.listMemoryEvents("pack/a 1");
    await api.listAvoidCues("pack/a 1");
    await api.retrieve({ caseId: "case/a 1", query: "PG 主从复制延迟", os: "linux" });
    await api.evaluateSuggestions({ caseId: "case/a 1", signals: ["pg_replication_lag"] });
    await api.prepareCandidate({ caseId: "case/a 1", suggestionId: "generate_pack" });
    await api.confirmCandidate({ confirmationToken: "token-1" });
    await api.reviewPack("pack/a 1", { decision: "approve", reviewer: "sre" });
    await api.enablePack("pack/a 1", { reviewer: "sre" });
    await api.pausePack("pack/a 1", { reason: "stale" });
    await api.getValidationGate("pack/a 1");
    await api.checkValidationGate("pack/a 1", { validation: "runner.readonly_probe" });
    await api.prepareRunnerCandidate({ packId: "pack/a 1" });
    await api.confirmRunnerCandidate({ confirmationToken: "runner-token" });
    await api.listRunnerBindings("pack/a 1");
    await api.reviewRunnerBinding("pack/a 1", "binding/a 1", { decision: "approve" });

    expect(http.calls).toEqual([
      {
        method: "GET",
        path:
          "/api/v1/experience-packs?status=enabled&category=repair&usage_shape=guided&middleware=postgres&has_runner_binding=true",
      },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201" },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/skill" },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/files" },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/capsules" },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/events" },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/memory-events" },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/avoid-cues" },
      { method: "POST", path: "/api/v1/experience-packs/retrieve", body: { caseId: "case/a 1", query: "PG 主从复制延迟", os: "linux" } },
      { method: "POST", path: "/api/v1/experience-packs/suggestions/evaluate", body: { caseId: "case/a 1", signals: ["pg_replication_lag"] } },
      { method: "POST", path: "/api/v1/experience-packs/candidates/prepare", body: { caseId: "case/a 1", suggestionId: "generate_pack" } },
      { method: "POST", path: "/api/v1/experience-packs/candidates/confirm", body: { confirmationToken: "token-1" } },
      { method: "POST", path: "/api/v1/experience-packs/pack%2Fa%201/review", body: { decision: "approve", reviewer: "sre" } },
      { method: "POST", path: "/api/v1/experience-packs/pack%2Fa%201/enable", body: { reviewer: "sre" } },
      { method: "POST", path: "/api/v1/experience-packs/pack%2Fa%201/pause", body: { reason: "stale" } },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/validation-gate" },
      { method: "POST", path: "/api/v1/experience-packs/pack%2Fa%201/validation-gate/check", body: { validation: "runner.readonly_probe" } },
      { method: "POST", path: "/api/v1/experience-packs/runner-candidates/prepare", body: { packId: "pack/a 1" } },
      { method: "POST", path: "/api/v1/experience-packs/runner-candidates/confirm", body: { confirmationToken: "runner-token" } },
      { method: "GET", path: "/api/v1/experience-packs/pack%2Fa%201/runner-bindings" },
      { method: "POST", path: "/api/v1/experience-packs/pack%2Fa%201/runner-bindings/binding%2Fa%201/review", body: { decision: "approve" } },
    ]);
  });

  it("routes candidate, review, activation, authorization, and reuse calls through /api/v1 experience-pack endpoints", async () => {
    const http = createRecordingHttpClient({ items: [] });
    const api = createExperiencePacksApi(http);

    await api.listCandidates({
      caseId: "case/a 1",
      service: "checkout",
      environment: "prod",
      limit: 10,
      cursor: "next/1",
    });
    await api.approveCandidate("candidate/a 1", { reviewer: "sre", comment: "通过审核" });
    await api.setPackEnabled("pack/a 1", false);
    await api.saveAuthorizationScopes("pack/a 1", {
      scopes: [{ type: "service", value: "checkout", searchable: true }],
    });
    await api.listReuseRecords("pack/a 1", { caseId: "case/a 1", limit: 20 });

    expect(http.calls).toEqual([
      {
        method: "GET",
        path:
          "/api/v1/experience-packs/candidates?case_id=case%2Fa%201&service=checkout&environment=prod&limit=10&cursor=next%2F1",
      },
      {
        method: "POST",
        path: "/api/v1/experience-packs/candidates/candidate%2Fa%201/approve",
        body: { reviewer: "sre", comment: "通过审核" },
      },
      {
        method: "PATCH",
        path: "/api/v1/experience-packs/pack%2Fa%201/enabled",
        body: { enabled: false },
      },
      {
        method: "PUT",
        path: "/api/v1/experience-packs/pack%2Fa%201/authorization-scopes",
        body: { scopes: [{ type: "service", value: "checkout", searchable: true }] },
      },
      {
        method: "GET",
        path: "/api/v1/experience-packs/pack%2Fa%201/reuse-records?case_id=case%2Fa%201&limit=20",
      },
    ]);
  });

  it("normalizes approved and authorized packs as searchable", () => {
    const pack = normalizeExperiencePack({
      pack_id: "pack-checkout-lock",
      title: "Checkout lock wait",
      category: "repair",
      usage_shape: "guided",
      review_status: "approved",
      enabled: true,
      validation_gate: { status: "passed", validators: ["runner.readonly_probe"] },
      runner_bindings: [{ id: "binding-1", workflow_id: "wf-lock-rca", status: "ready" }],
      retrieval_eval: { score: 0.91, matched_cases: 12, last_evaluated_at: "2026-05-12T09:00:00+08:00" },
      workflow_binding: { workflow_id: "wf-lock-rca", status: "bound" },
      authorization_scopes: [{ scope_type: "service", value: "checkout", searchable: true }],
    });

    expect(pack).toMatchObject({
      id: "pack-checkout-lock",
      title: "Checkout lock wait",
      reviewStatus: "approved",
      enabled: true,
      category: "repair",
      usageShape: "guided",
      searchable: true,
      searchableReason: "已审核启用，且已配置可检索授权范围",
      validationGate: { status: "passed", passed: true },
      retrievalEval: { score: 0.91, matchedCases: 12 },
      workflowBinding: { workflowId: "wf-lock-rca", status: "bound" },
    });
    expect(pack.runnerBindings[0]).toMatchObject({ id: "binding-1", workflowId: "wf-lock-rca", status: "ready" });
    expect(pack.authorizationScopes[0]).toMatchObject({ type: "service", value: "checkout", searchable: true });
  });

  it("normalizes experience matches without exposing raw Gene or Capsule JSON", () => {
    const match = normalizeExperienceMatch({
      pack_id: "pack-pg-replication",
      skill: { name: "PG 主从延迟诊断", summary: "定位 replication lag" },
      confidence: "0.88",
      compatibility_status: "adapt_required",
      compatibility_gaps: ["操作系统不同：请求 centos，经验包 ubuntu"],
      matched_signals: ["pg_replication_lag", "wal_sender_wait"],
      precondition_gaps: ["需要确认目标主机操作系统"],
      risk_warnings: ["只允许 dry run"],
      os_variant: "linux",
      runner_binding: { id: "binding-1", workflow_id: "wf-pg-lag", status: "ready" },
      history: { success_count: 7, failure_count: 1, recent_result: "success" },
      advanced_refs: { gene_asset_id: "gene-asset-1", capsule_asset_ids: ["capsule-1"] },
      gene: { signals_match: ["should-not-leak"] },
      capsules: [{ id: "should-not-leak" }],
    });

    expect(match).toMatchObject({
      packId: "pack-pg-replication",
      skill: { name: "PG 主从延迟诊断" },
      confidence: 0.88,
      compatibilityStatus: "adapt_required",
      compatibilityGaps: ["操作系统不同：请求 centos，经验包 ubuntu"],
      matchedSignals: ["pg_replication_lag", "wal_sender_wait"],
      preconditionGaps: ["需要确认目标主机操作系统"],
      osVariant: "linux",
      runnerBinding: { id: "binding-1", workflowId: "wf-pg-lag", status: "ready" },
      history: { successCount: 7, failureCount: 1, recentResult: "success" },
      advancedRefs: { geneAssetId: "gene-asset-1", capsuleAssetIds: ["capsule-1"] },
    });
    expect(JSON.stringify(match)).not.toContain("should-not-leak");
  });

  it("normalizes runner candidates and stores confirmed workflow drafts for Runner Studio", async () => {
    const runnerCandidatePayload = {
      id: "runner_candidate_pg",
      pack_id: "pack-pg",
      workflow_id: "wf-pg-cluster",
      workflow_name: "PG Cluster Workflow",
      status: "draft",
      studio_draft_link: "/runner/wf-pg-cluster",
      workflow: {
        id: "wf-pg-cluster",
        name: "wf-pg-cluster",
        title: "PG Cluster Workflow",
        graph: {
          workflow: { name: "wf-pg-cluster" },
          nodes: [{ id: "start", type: "start" }],
          edges: [],
        },
      },
      runner_binding: { id: "binding-pg", workflow_id: "wf-pg-cluster", status: "draft" },
    };
    const normalized = normalizeRunnerCandidate(runnerCandidatePayload);
    expect(normalized).toMatchObject({
      id: "runner_candidate_pg",
      packId: "pack-pg",
      workflowId: "wf-pg-cluster",
      workflowName: "PG Cluster Workflow",
      studioDraftLink: "/runner/wf-pg-cluster",
    });

    const http = createRecordingHttpClient(runnerCandidatePayload);
    const api = createExperiencePacksApi(http);
    const result = await api.confirmRunnerCandidate({ packId: "pack-pg" });
    const drafts = JSON.parse(window.localStorage.getItem("runner.studio.localDrafts") || "{}");

    expect(result.workflowId).toBe("wf-pg-cluster");
    expect(drafts["wf-pg-cluster"]).toMatchObject({
      id: "wf-pg-cluster",
      name: "wf-pg-cluster",
      title: "PG Cluster Workflow",
      status: "draft",
      local_draft: true,
      ai_generated_draft: true,
    });
    expect(drafts["wf-pg-cluster"].graph.nodes).toEqual([{ id: "start", type: "start" }]);
  });

  it("keeps unreviewed or unauthorized packs unsearchable with Chinese reasons for page display", () => {
    expect(normalizeExperiencePack({ id: "draft", review_status: "pending", enabled: true })).toMatchObject({
      searchable: false,
      searchableReason: "经验包尚未审核通过，不能被检索",
    });

    expect(
      normalizeExperiencePack({
        id: "unauthorized",
        review_status: "approved",
        enabled: true,
        authorization_scopes: [],
      }),
    ).toMatchObject({
      searchable: false,
      searchableReason: "经验包尚未配置可检索授权范围",
    });
  });

  it("normalizes candidate and reuse list wrappers", () => {
    const candidates = normalizeExperiencePackList({
      candidates: [
        {
          candidate_id: "candidate-1",
          pack_id: "pack-1",
          title: "慢查询经验",
          status: "candidate",
          match_reason: "SQL 指纹相似",
          source_case_id: "case-1",
        },
      ],
      next_cursor: "n1",
      total: 1,
    });
    const reuseRecords = normalizeReuseRecordList({
      reuse_records: [
        {
          id: "reuse-1",
          pack_id: "pack-1",
          case_id: "case-2",
          result: "success",
          reused_at: "2026-05-12T10:00:00+08:00",
        },
      ],
    });

    expect(candidates).toMatchObject({
      items: [{ id: "candidate-1", packId: "pack-1", sourceCaseId: "case-1" }],
      nextCursor: "n1",
      total: 1,
    });
    expect(reuseRecords.items[0]).toMatchObject({
      id: "reuse-1",
      packId: "pack-1",
      caseId: "case-2",
      result: "success",
    });
  });
});
