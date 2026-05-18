import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import { RCAReportArtifact } from "./RCAReportArtifact";

function artifact(status = "ok"): AiopsTransportAgentUiArtifact {
  return {
    id: "artifact-rca",
    type: "rca_report",
    titleZh: "checkout 根因分析",
    summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
    status,
    severity: "high",
    source: "coroot",
    permissionScope: "read",
    redactionStatus: "redacted",
    inlineData: {
      schemaVersion: "aiops.rca_report/v1",
      source: "coroot",
      status,
      target: { service: "checkout" },
      window: { timeRange: "30m" },
      conclusion: {
        summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
        rootCauseEntity: "catalog",
        confidence: 0.72,
      },
      hypotheses: [
        {
          id: "hyp-1",
          titleZh: "catalog 依赖延迟",
          confidence: 0.72,
          supportingEvidenceRefs: ["ev-1"],
          contradictingEvidenceRefs: [],
          missingEvidence: [],
        },
      ],
      sections: [
        {
          id: "propagation",
          kind: "propagation_map",
          titleZh: "传播路径",
          evidenceRefs: ["ev-1"],
          payload: {
            nodes: [{ id: "checkout" }, { id: "catalog" }],
            edges: [{ source: "checkout", target: "catalog" }],
          },
        },
        {
          id: "metrics",
          kind: "timeseries_grid",
          titleZh: "关键指标",
          evidenceRefs: ["ev-1"],
          payload: {
            metrics: [
              {
                name: "latency_p99",
                entity: "checkout->catalog",
                valueSummary: "p99 rose to 1.8s",
                status: "critical",
              },
            ],
          },
        },
      ],
      evidenceRefs: ["ev-1"],
      rawRefs: [{ source: "coroot", uri: "coroot://project/default/checkout" }],
      limitations: [],
    },
  };
}

describe("RCAReportArtifact", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }).IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    container.remove();
  });

  it("renders the RCA conclusion and key sections", async () => {
    await act(async () => {
      root.render(<RCAReportArtifact artifact={artifact()} />);
    });

    expect(container.querySelector('[data-testid="rca-report-artifact"]')).toBeTruthy();
    expect(container.textContent).toContain("checkout 根因分析");
    expect(container.textContent).toContain("catalog 依赖");
    expect(container.textContent).toContain("传播路径");
    expect(container.textContent).toContain("关键指标");
  });

  it("renders inconclusive status", async () => {
    await act(async () => {
      root.render(<RCAReportArtifact artifact={artifact("inconclusive")} />);
    });

    expect(container.textContent).toContain("证据不足");
  });

  it("does not render unsafe html from inline data", async () => {
    const unsafe = artifact();
    unsafe.titleZh = "<img src=x onerror=alert(1)>checkout 根因分析";
    unsafe.inlineData = {
      schemaVersion: "aiops.rca_report/v1",
      source: "coroot",
      status: "partial",
      conclusion: {
        summaryZh: "<script>alert(1)</script>checkout 仍需补充证据",
        confidence: 0.33,
      },
      hypotheses: [],
      sections: [
        {
          id: "timeline",
          kind: "event_timeline",
          titleZh: "<b>事件</b>",
          evidenceRefs: [],
          payload: {
            events: [{ message: "<img src=x onerror=alert(1)>deployment", timestamp: "2026-05-15T02:00:00Z" }],
          },
        },
      ],
      evidenceRefs: [],
      rawRefs: [],
      limitations: ["<img src=x onerror=alert(1)>missing logs"],
    };

    await act(async () => {
      root.render(<RCAReportArtifact artifact={unsafe} />);
    });

    expect(container.textContent).toContain("checkout 仍需补充证据");
    expect(container.textContent).toContain("事件");
    expect(container.querySelector("img")).toBeNull();
    expect(container.innerHTML).not.toContain("onerror");
    expect(container.innerHTML).not.toContain("alert(1)");
  });

  it("renders restricted reports without exposing inline evidence details", async () => {
    const restricted = artifact("partial");
    restricted.permissionScope = "restricted";
    restricted.inlineData = {
      ...(restricted.inlineData as Record<string, unknown>),
      rawRefs: [{ source: "coroot", uri: "coroot://project/default/secret" }],
    };

    await act(async () => {
      root.render(<RCAReportArtifact artifact={restricted} />);
    });

    expect(container.textContent).toContain("权限受限");
    expect(container.textContent).toContain("checkout 根因分析");
    expect(container.textContent).not.toContain("coroot://project/default/secret");
  });
});
