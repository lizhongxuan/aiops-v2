import { describe, expect, it, vi } from "vitest";
import { createCasesApi, normalizeCase, normalizeCaseList } from "./cases";

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
      return Promise.resolve({ ok: true, caseId: "case-1" });
    }),
  };
}

describe("cases API", () => {
  it("routes Case reads and decisions through /api/v1 cases endpoints", async () => {
    const http = createRecordingHttpClient({ items: [] });
    const api = createCasesApi(http);

    await api.listCases({
      status: "active",
      source: "debug_mode",
      environment: "prod",
      hostId: "host/a 1",
      waitingConfirmation: true,
      lockConflict: false,
      limit: 20,
      cursor: "next/1",
    });
    await api.getCase("case/a 1");
    await api.confirmCaseAction("case/a 1", "action/2", {
      decision: "approved",
      comment: "确认执行",
    });
    await api.closeCase("case/a 1", { reason: "已恢复" });

    expect(http.calls).toEqual([
      {
        method: "GET",
        path:
          "/api/v1/cases?status=active&source=debug_mode&environment=prod&host_id=host%2Fa%201&waiting_confirmation=true&lock_conflict=false&limit=20&cursor=next%2F1",
      },
      { method: "GET", path: "/api/v1/cases/case%2Fa%201" },
      {
        method: "POST",
        path: "/api/v1/cases/case%2Fa%201/actions/action%2F2/decision",
        body: { decision: "approved", comment: "确认执行" },
      },
      {
        method: "POST",
        path: "/api/v1/cases/case%2Fa%201/close",
        body: { reason: "已恢复" },
      },
    ]);
  });

  it("normalizes a full enterprise Case payload", () => {
    const view = normalizeCase({
      case_id: "case-debug-1",
      title: "页面按钮很慢",
      status: "collecting_evidence",
      severity: "high",
      source: "browser_debug_mode",
      environment: "prod",
      evidence: [
        {
          evidence_ref: "ev-coroot-latency",
          artifact_id: "coroot-checkout-latency-chart",
          title: "Coroot 延迟图",
          source: "coroot",
          trace_id: "trace-1",
        },
      ],
      host_profile_snapshots: [{ host_id: "host-web-1", display_name: "web-1", labels: { env: "prod" } }],
      host_leases: [{ lease_id: "lease-1", host_id: "host-web-1", status: "acquired", expires_at: "2026-05-12T10:00:00+08:00" }],
      workflow_runs: [{ run_id: "run-1", workflow_id: "wf-debug-rca", status: "succeeded", verification_refs: ["verify-1"] }],
      verifications: [{ id: "verify-1", status: "passed", summary: "p95 已恢复" }],
      experience_candidates: [{ pack_id: "pack-1", title: "库存锁等待经验", status: "candidate" }],
      timeline: [{ id: "tl-1", type: "artifact_created", title: "生成 Coroot 图表" }],
    });

    expect(view).toMatchObject({
      id: "case-debug-1",
      title: "页面按钮很慢",
      status: "collecting_evidence",
      severity: "high",
      source: "browser_debug_mode",
      environment: "prod",
    });
    expect(view.evidence[0]).toMatchObject({
      evidenceRef: "ev-coroot-latency",
      artifactId: "coroot-checkout-latency-chart",
      traceId: "trace-1",
    });
    expect(view.hostProfiles[0]).toMatchObject({ hostId: "host-web-1", displayName: "web-1" });
    expect(view.hostLeases[0]).toMatchObject({ leaseId: "lease-1", status: "acquired" });
    expect(view.workflowRuns[0]).toMatchObject({ runId: "run-1", workflowId: "wf-debug-rca" });
    expect(view.verifications[0]).toMatchObject({ id: "verify-1", status: "passed" });
    expect(view.experienceCandidates[0]).toMatchObject({ packId: "pack-1", status: "candidate" });
    expect(view.timeline[0]).toMatchObject({ id: "tl-1", type: "artifact_created" });
  });

  it("normalizes legacy IncidentRecord payloads into CaseView", () => {
    const view = normalizeCase({
      id: "incident-1",
      name: "Checkout latency spike",
      status: "active",
      sev: "SEV2",
      env: "prod",
      capability: "checkout",
      evidence: [{ id: "ev-1", title: "Coroot latency", summary: "p95 above threshold", source: "coroot" }],
      pendingApprovals: [{ id: "approval-1", command: "kubectl rollout restart deployment/checkout" }],
    });

    expect(view).toMatchObject({
      id: "incident-1",
      title: "Checkout latency spike",
      status: "active",
      severity: "SEV2",
      environment: "prod",
      businessCapability: "checkout",
    });
    expect(view.evidence[0]).toMatchObject({ evidenceRef: "ev-1", title: "Coroot latency" });
    expect(view.pendingActions[0]).toMatchObject({ actionId: "approval-1", title: "kubectl rollout restart deployment/checkout" });
  });

  it("normalizes list wrappers from cases or legacy incidents", () => {
    expect(normalizeCaseList({ cases: [{ id: "case-1" }] }).items.map((item) => item.id)).toEqual(["case-1"]);
    expect(normalizeCaseList({ incidents: [{ id: "incident-1" }], next_cursor: "n1" })).toMatchObject({
      items: [{ id: "incident-1" }],
      nextCursor: "n1",
    });
  });
});
