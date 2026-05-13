import { describe, expect, it, vi } from "vitest";

import {
  createExperiencePacksApi,
  normalizeExperiencePack,
  normalizeExperiencePackList,
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
      review_status: "approved",
      enabled: true,
      retrieval_eval: { score: 0.91, matched_cases: 12, last_evaluated_at: "2026-05-12T09:00:00+08:00" },
      workflow_binding: { workflow_id: "wf-lock-rca", status: "bound" },
      authorization_scopes: [{ scope_type: "service", value: "checkout", searchable: true }],
    });

    expect(pack).toMatchObject({
      id: "pack-checkout-lock",
      title: "Checkout lock wait",
      reviewStatus: "approved",
      enabled: true,
      searchable: true,
      searchableReason: "已审核启用，且已配置可检索授权范围",
      retrievalEval: { score: 0.91, matchedCases: 12 },
      workflowBinding: { workflowId: "wf-lock-rca", status: "bound" },
    });
    expect(pack.authorizationScopes[0]).toMatchObject({ type: "service", value: "checkout", searchable: true });
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
