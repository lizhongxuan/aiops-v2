import { describe, expect, it } from "vitest";
import {
  buildCaseBlockingItems,
  buildCaseTabs,
  buildCaseViewModel,
  getCaseEvidenceRefs,
} from "./caseViewModels";

const casePayload = {
  id: "case-debug-1",
  title: "页面按钮很慢",
  status: "waiting_confirmation",
  severity: "high",
  evidence: [
    { evidence_ref: "ev-coroot-latency", artifact_id: "artifact-1", title: "Coroot 图表" },
    { id: "ev-trace-summary", artifactId: "artifact-2", title: "Trace 摘要" },
  ],
  host_profile_snapshots: [{ host_id: "host-web-1", display_name: "web-1" }],
  host_leases: [{ lease_id: "lease-1", host_id: "host-web-1", status: "conflict" }],
  workflow_runs: [{ run_id: "run-1", workflow_id: "wf-fix", status: "failed", failed_step: "change" }],
  verifications: [{ id: "verify-1", status: "failed" }],
  experience_candidates: [{ pack_id: "pack-1", title: "慢请求经验" }],
  timeline: [{ id: "tl-1", title: "创建 Case" }],
  pendingActions: [{ actionId: "action-1", title: "确认修复" }],
};

describe("case view models", () => {
  it("builds Chinese Case labels and tab counters", () => {
    const view = buildCaseViewModel(casePayload);

    expect(view.title).toBe("页面按钮很慢");
    expect(view.statusLabel).toBe("等待确认");
    expect(view.severityLabel).toBe("高风险");
    expect(view.tabs.map((tab) => `${tab.label}:${tab.count}`)).toEqual([
      "概览:0",
      "证据:2",
      "主机环境:1",
      "执行:1",
      "验证:1",
      "经验:1",
      "时间线:1",
    ]);
  });

  it("derives blocking items from approval, HostLease, Workflow and verification state", () => {
    expect(buildCaseBlockingItems(casePayload).map((item) => item.key)).toEqual([
      "waiting_confirmation",
      "host_lease_blocked",
      "workflow_failed",
      "verification_failed",
    ]);
  });

  it("keeps EvidenceRef values available for Case and Prompt Trace links", () => {
    expect(getCaseEvidenceRefs(casePayload)).toEqual(["ev-coroot-latency", "ev-trace-summary"]);
  });

  it("reports missing evidence as a blocking item for incomplete Case details", () => {
    expect(buildCaseBlockingItems({ id: "case-empty", status: "analyzing" }).map((item) => item.key)).toContain(
      "missing_evidence",
    );
    expect(buildCaseTabs({ id: "case-empty" }).find((tab) => tab.key === "evidence")?.count).toBe(0);
  });
});
