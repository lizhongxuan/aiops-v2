import { describe, expect, it } from "vitest";

import { normalizeRCAReport } from "./rcaReportModel";

describe("normalizeRCAReport", () => {
  it("normalizes valid RCA report data", () => {
    const report = normalizeRCAReport({
      schemaVersion: "aiops.rca_report/v1",
      source: "coroot",
      status: "ok",
      target: { service: "checkout" },
      window: { timeRange: "30m" },
      conclusion: { summaryZh: "catalog 依赖延迟传播到 checkout", confidence: 0.72 },
      hypotheses: [
        {
          id: "hyp-1",
          titleZh: "catalog 依赖延迟",
          confidence: 0.72,
          supportingEvidenceRefs: ["ev-1"],
        },
      ],
      sections: [
        {
          id: "propagation",
          kind: "propagation_map",
          titleZh: "传播路径",
          evidenceRefs: ["ev-1"],
          payload: { nodes: [], edges: [] },
        },
      ],
      evidenceRefs: ["ev-1"],
      rawRefs: [{ source: "coroot", uri: "coroot://project/default/checkout" }],
      limitations: [],
    });

    expect(report.source).toBe("coroot");
    expect(report.target.service).toBe("checkout");
    expect(report.conclusion.confidence).toBe(0.72);
    expect(report.sections[0].kind).toBe("propagation_map");
  });

  it("returns inconclusive fallback for invalid data", () => {
    const report = normalizeRCAReport({ schemaVersion: "bad" });

    expect(report.status).toBe("inconclusive");
    expect(report.conclusion.summaryZh).toContain("无法读取");
  });

  it("drops unsafe display fields and clamps confidence", () => {
    const report = normalizeRCAReport({
      schemaVersion: "aiops.rca_report/v1",
      source: "coroot",
      status: "ok",
      target: {
        service: "<img src=x onerror=alert(1)>checkout",
        secretToken: "sk-live-sensitive",
      },
      conclusion: {
        summaryZh: "<script>alert(1)</script>checkout 延迟升高",
        confidence: 4.2,
      },
      sections: [
        {
          id: "unsafe",
          kind: "timeseries_grid",
          titleZh: "<b>指标</b>",
          evidenceRefs: ["ev-1"],
          payload: {
            html: "<img src=x onerror=alert(1)>",
            metrics: [{ name: "latency", valueSummary: "slow" }],
          },
        },
      ],
      evidenceRefs: ["ev-1", "<script>bad</script>"],
      rawRefs: [{ source: "coroot", uri: "java" + "scr" + "ipt:alert(1)" }],
      limitations: [],
      html: "<img src=x onerror=alert(1)>",
    });

    expect(report.conclusion.summaryZh).toBe("checkout 延迟升高");
    expect(report.conclusion.confidence).toBe(1);
    expect(report.target.service).toBe("checkout");
    expect(report.target.secretToken).toBeUndefined();
    expect(report.sections[0].titleZh).toBe("指标");
    expect(report.sections[0].payload.html).toBeUndefined();
    expect(report.evidenceRefs).toEqual(["ev-1"]);
    expect(report.rawRefs).toEqual([]);
  });
});
