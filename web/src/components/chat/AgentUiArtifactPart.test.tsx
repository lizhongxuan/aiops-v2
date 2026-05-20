import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { readFileSync } from "node:fs";
import { join } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AgentUiArtifactPart } from "./AgentUiArtifactPart";
import { CorootChartArtifact } from "./CorootChartArtifact";
import { ExperienceMatchArtifact } from "./ExperienceMatchArtifact";
import { RCAReportArtifact } from "./RCAReportArtifact";
import { TopologySliceArtifact } from "./TopologySliceArtifact";
import { TraceSummaryArtifact } from "./TraceSummaryArtifact";
import { VerificationResultArtifact } from "./VerificationResultArtifact";
import { WorkflowResultArtifact } from "./WorkflowResultArtifact";
import { McpSurfacePart } from "../../chat/components/McpSurfacePart";

vi.mock("@/transport/useAiopsTransportCommands", () => ({
  useAiopsTransportCommands: () => ({
    mcpAction: vi.fn(),
    mcpRefresh: vi.fn(),
    mcpPin: vi.fn(),
  }),
}));

describe("AgentUiArtifactPart", () => {
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

  it("keeps dedicated artifact components available for the entry dispatcher", () => {
    expect(CorootChartArtifact).toBeTypeOf("function");
    expect(TraceSummaryArtifact).toBeTypeOf("function");
    expect(TopologySliceArtifact).toBeTypeOf("function");
    expect(WorkflowResultArtifact).toBeTypeOf("function");
    expect(VerificationResultArtifact).toBeTypeOf("function");
    expect(ExperienceMatchArtifact).toBeTypeOf("function");
    expect(RCAReportArtifact).toBeTypeOf("function");
  });

  it("renders a Coroot chart artifact without duplicated summary/footer labels", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-coroot-latency",
            type: "coroot_chart",
            titleZh: "Coroot 延迟趋势",
            summaryZh: "接口 P95 延迟在 14:03 后明显升高。",
            caseId: "case-debug-1",
            source: "coroot",
            redactionStatus: "redacted",
            createdAt: "2026-05-12T02:00:00Z",
            inlineData: {
              mcpCard: {
                uiKind: "readonly_chart",
                title: "指标趋势",
                visual: {
                  kind: "timeseries",
                  series: [
                    {
                      name: "p95_latency_ms",
                      data: [
                        { timestamp: 1, value: 120 },
                        { timestamp: 2, value: 980 },
                      ],
                    },
                  ],
                },
              },
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("Coroot 延迟趋势");
    expect(container.textContent).not.toContain("接口 P95 延迟在 14:03 后明显升高。");
    expect(container.textContent).toContain("p95_latency_ms");
    expect(container.textContent).toContain("已脱敏");
    expect(container.textContent).not.toContain("来源：coroot");
  });

  it("renders Coroot service headers without redundant labels or none-redaction badges", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-coroot-aiops-host-agent",
            type: "coroot_chart",
            titleZh: "5hxbfx6p:_:Unknown:aiops-host-agent Coroot 图表",
            summaryZh: "Coroot 服务原生图表与指标趋势",
            redactionStatus: "none",
            inlineData: {
              mcpCard: {
                uiKind: "readonly_chart",
                title: "5hxbfx6p:_:Unknown:aiops-host-agent Coroot charts",
                visual: {
                  kind: "timeseries",
                  series: [{ name: "cpu", data: [{ timestamp: 1, value: 1 }] }],
                },
              },
            },
          } as any}
        />,
      );
    });

    expect(container.textContent).toContain(
      "5hxbfx6p:_:Unknown:aiops-host-agent 服务",
    );
    expect(container.textContent).not.toContain("5hxbfx6p:_:Unknown:aiops-host-agent 图表");
    expect(container.textContent).not.toContain("Coroot 服务原生图表与指标趋势");
    expect(container.textContent).not.toContain("Coroot 图表");
    expect(container.textContent).not.toContain("Coroot charts");
    expect(container.textContent).not.toContain("服务服务");
    expect(container.textContent).not.toContain("未脱敏");
  });

  it("renders an unsupported artifact safely", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-unknown",
            type: "unknown_widget",
            titleZh: "",
            summaryZh: "",
          }}
        />,
      );
    });

    expect(container.textContent).toContain("暂不支持的卡片类型");
    expect(container.textContent).toContain("类型：unknown_widget");
    expect(container.innerHTML).not.toContain("dangerouslySetInnerHTML");
  });

  it("routes renderer selection through the frontend registry", () => {
    const source = readFileSync(join(process.cwd(), "src/components/chat/AgentUiArtifactPart.tsx"), "utf8");

    expect(source).toContain("lookupAgentUiCardRenderer(defaultAgentUiCardRegistry");
    expect(source).not.toContain("switch (artifact.type)");
  });

  it("renders terminal unsupported cards without exposing payload HTML", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-terminal-unsupported",
            type: "shell_widget",
            title: "危险卡片",
            source: "agent",
            caseId: "case-terminal",
            evidenceRef: "ev-terminal",
            promptTraceId: "trace-terminal",
            payload: {
              html: "<img src=x onerror=alert(1)>",
              secret: "must-not-render",
            },
          } as any}
        />,
      );
    });

    expect(container.textContent).toContain("无法渲染 Agent UI 卡片");
    expect(container.textContent).toContain("未注册的卡片类型。");
    expect(container.textContent).toContain("类型：shell_widget");
    expect(container.textContent).toContain("来源：agent");
    expect(container.querySelector('a[href="/incidents/case-terminal"]')?.textContent).toContain("查看 Case");
    expect(container.querySelector('a[href="/incidents/case-terminal?evidence=ev-terminal"]')?.textContent).toContain("查看证据");
    expect(container.querySelector('a[href="/debug/prompts?trace_id=trace-terminal"]')?.textContent).toContain("查看 Prompt Trace");
    expect(container.textContent).not.toContain("must-not-render");
    expect(container.querySelector("img")).toBeNull();
    expect(container.innerHTML).not.toContain("onerror");
  });

  it("renders invalid payload cards without exposing payload content", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-invalid-payload",
            type: "trace_summary",
            source: "agent",
            payload: "raw invalid payload",
          } as any}
        />,
      );
    });

    expect(container.textContent).toContain("Agent UI 卡片数据无效");
    expect(container.textContent).toContain("卡片 payload 必须是对象。");
    expect(container.textContent).toContain("类型：trace_summary");
    expect(container.textContent).not.toContain("raw invalid payload");
  });

  it("renders normalized API artifacts with top-level mcpCard", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-normalized-coroot",
            type: "coroot_chart",
            title: "web-checkout p95 延迟",
            summary: "p95 从 420ms 升至 2.8s。",
            status: "warning",
            source: "coroot-mcp",
            caseId: "case-debug-2",
            mcpCard: {
              uiKind: "readonly_chart",
              title: "web-checkout p95 延迟",
              visual: {
                kind: "timeseries",
                series: [{ name: "p95", data: [{ timestamp: 1, value: 2800 }] }],
              },
            },
          } as any}
        />,
      );
    });

    expect(container.textContent).toContain("web-checkout p95 延迟");
    expect(container.textContent).not.toContain("p95 从 420ms 升至 2.8s。");
    expect(container.textContent).toContain("p95");
    expect(container.querySelector('a[href="/incidents/case-debug-2"]')).toBeNull();
    expect(container.textContent).not.toContain("来源：coroot-mcp");
  });

  it("renders native Coroot chart and chart_group widgets from service metrics", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-coroot-native-charts",
            type: "coroot_chart",
            titleZh: "Coroot 原生图表",
            summaryZh: "展示 Coroot 返回的所有 chart/chart_group。",
            source: "coroot",
            inlineData: {
              defaultReportName: "CPU",
              chartReports: [
                {
                  name: "CPU",
                  status: "ok",
                  widgets: [
                    {
                      chart_group: {
                        title: "CPU usage <selector>, cores",
                        charts: [
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "container: checkout",
                            series: [{ name: "checkout-1", data: [0.4, 0.6] }],
                          },
                        ],
                      },
                    },
                  ],
                },
                {
                  name: "Net",
                  status: "warning",
                  widgets: [
                    {
                      chart: {
                        ctx: { from: 1710000000000, step: 30000 },
                        title: "Failed TCP connections, per second",
                        series: [
                          { name: "->external:18090", data: [0.333, 0.32, 0.333] },
                          { name: "aiops-host-agent@cosmic4eye-22", data: [0.31, 0.32, 0.31] },
                        ],
                      },
                    },
                  ],
                },
              ],
              mcpCard: {
                uiKind: "readonly_chart",
                title: "Coroot 原生图表",
                visual: { kind: "coroot_report_charts" },
              },
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("CPU");
    expect(container.textContent).not.toContain("CPU usage container: checkout, cores");
    expect(container.textContent).not.toContain("container: checkout");
    expect(container.textContent).toContain("checkout-1");
    expect(container.textContent).toContain("Net");
    expect(container.textContent).not.toContain("Failed TCP connections");
    expect(container.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toContain("CPU");
    expect(container.querySelectorAll('[data-testid="coroot-native-charts"] svg').length).toBe(1);
    const nativeChartContainer = container.querySelector(
      '[data-testid="coroot-native-charts"]',
    );
    expect(nativeChartContainer?.className).toContain("max-w-[640px]");
    expect(nativeChartContainer?.className).toContain("w-full");
    const nativeSvg = container.querySelector(
      '[data-testid="coroot-native-charts"] svg',
    );
    expect(nativeSvg?.className.baseVal).toContain("h-[220px]");
    expect(nativeSvg?.className.baseVal).toContain("w-full");
    expect(nativeSvg?.getAttribute("viewBox")).toBe("0 0 760 220");
    expect(container.textContent).not.toContain("0cores");
    expect(container.textContent).not.toContain("chartReports");

    const netTab = Array.from(container.querySelectorAll('[role="tab"]')).find((tab) => tab.textContent?.includes("Net")) as HTMLButtonElement;
    await act(async () => {
      netTab.click();
    });

    expect(container.querySelector('[role="tab"][aria-selected="true"]')?.textContent).toContain("Net");
    expect(container.textContent).not.toContain("Failed TCP connections");
    expect(container.textContent).toContain("->external:18090");
    expect(container.textContent).toContain("303m");
    expect(container.textContent).toContain("322m");
    expect(container.textContent).toContain("340m");
    expect(container.textContent).not.toContain("warning");
    expect(container.textContent).not.toContain("最新");
    expect(container.textContent).not.toContain("333mper second");
    expect(container.textContent).not.toContain("CPU usage");

    const netSvg = container.querySelector(
      '[data-testid="coroot-native-charts"] svg',
    ) as SVGSVGElement | null;
    expect(netSvg).toBeTruthy();
    const firstLineYs = (netSvg?.querySelector("polyline")?.getAttribute("points") || "")
      .split(" ")
      .map((point) => Number(point.split(",")[1]))
      .filter(Number.isFinite);
    expect(Math.max(...firstLineYs) - Math.min(...firstLineYs)).toBeGreaterThan(45);
    Object.defineProperty(netSvg, "getBoundingClientRect", {
      configurable: true,
      value: () => ({
        left: 0,
        top: 0,
        width: 760,
        height: 220,
        right: 760,
        bottom: 220,
        x: 0,
        y: 0,
        toJSON: () => ({}),
      }),
    });
    await act(async () => {
      netSvg?.dispatchEvent(
        new MouseEvent("mousemove", {
          bubbles: true,
          clientX: 400,
          clientY: 160,
        }),
      );
    });

    expect(
      container.querySelector('[data-testid="coroot-chart-tooltip"]'),
    ).toBeTruthy();
    expect(container.textContent).toContain("->external:18090");
    expect(container.textContent).toContain("320m");
    const tooltipRect = container.querySelector(
      '[data-testid="coroot-chart-tooltip-box"]',
    );
    expect(Number(tooltipRect?.getAttribute("width"))).toBeGreaterThanOrEqual(360);
    const tooltipValues = Array.from(
      container.querySelectorAll('[data-testid="coroot-chart-tooltip-value"]'),
    );
    expect(Number(tooltipValues[0]?.getAttribute("x"))).toBeGreaterThan(320);
    expect(container.textContent).not.toContain("aiops-host-agent@cosmic4eye-220");

    await act(async () => {
      netSvg?.dispatchEvent(new MouseEvent("mouseout", { bubbles: true }));
    });
    expect(
      container.querySelector('[data-testid="coroot-chart-tooltip"]'),
    ).toBeNull();
  });

  it("shows only the preferred Coroot Memory usage chart and hides redundant artifact footer", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-coroot-widget-tabs",
            type: "coroot_chart",
            titleZh: "Coroot 内存图表",
            summaryZh: "展示 Memory report。",
            source: "coroot",
            evidenceRef: "http://coroot.example/api/project/default/app/service",
            createdAt: "2026-05-20T00:27:19Z",
            inlineData: {
              defaultReportName: "Memory",
              chartReports: [
                {
                  name: "Memory",
                  status: "ok",
                  widgets: [
                    {
                      chart_group: {
                        title: "Memory usage RSS <selector>, bytes",
                        charts: [
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "RSS container: aiops-host-agent",
                            series: [{ name: "rss-container", data: [1024, 2048] }],
                          },
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "RSS",
                            series: [{ name: "rss", data: [1024, 1536] }],
                          },
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "RSS + PageCache",
                            series: [{ name: "rss-pagecache", data: [2048, 3072] }],
                          },
                        ],
                      },
                    },
                    {
                      chart_group: {
                        title: "Memory stall time <selector>, seconds per second",
                        charts: [
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "some",
                            series: [{ name: "stall", data: [0, 1] }],
                          },
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "full",
                            series: [{ name: "stall-full", data: [0, 1] }],
                          },
                        ],
                      },
                    },
                    {
                      chart_group: {
                        title: "Node memory usage (unreclaimable), %",
                        charts: [
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "overview",
                            series: [{ name: "node-memory", data: [33, 34] }],
                          },
                        ],
                      },
                    },
                    {
                      chart_group: {
                        title: "Memory consumers <selector>, bytes",
                        charts: [
                          {
                            ctx: { from: 1710000000000, step: 30000 },
                            title: "cosmic4eye-22",
                            series: [{ name: "mservice", data: [1024, 2048] }],
                          },
                        ],
                      },
                    },
                  ],
                },
              ],
            },
          } as any}
        />,
      );
    });

    expect(container.querySelectorAll('[data-testid="coroot-native-charts"] svg')).toHaveLength(1);
    expect(container.textContent).not.toContain("Memory usage RSS + PageCache");
    expect(container.textContent).toContain("rss-pagecache");
    expect(container.textContent).not.toContain("RSS container: aiops-host-agent");
    expect(container.textContent).not.toContain("rss-container");
    expect(container.textContent).not.toContain("Memory consumers");
    expect(container.textContent).not.toContain("mservice");
    expect(container.querySelector('[data-testid="coroot-chart-tabs"]')).toBeNull();
    expect(container.querySelector('[data-testid="coroot-widget-tabs"]')).toBeNull();
    expect(container.textContent).not.toContain("Memory stall time");
    expect(container.textContent).not.toContain("stall-full");
    expect(container.textContent).not.toContain("Node memory usage");
    expect(container.textContent).not.toContain("node-memory");
    expect(container.textContent).not.toContain("来源：coroot");
    expect(container.textContent).not.toContain("生成时间：");
    expect(container.textContent).not.toContain("查看证据");
    expect(container.textContent).not.toContain("Memory usage RSS container");
  });

  it("renders unified actions for case, evidence, and prompt trace", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-actions",
            type: "trace_summary",
            titleZh: "慢请求 Trace",
            summaryZh: "已关联 Case、证据和 Prompt Trace。",
            caseId: "case-debug-3",
            evidenceRef: "ev-trace-1",
            promptTraceId: "prompt-trace-1",
          } as any}
        />,
      );
    });

    expect(container.querySelector('a[href="/incidents/case-debug-3"]')?.textContent).toContain("查看 Case");
    expect(container.querySelector('a[href="/incidents/case-debug-3?evidence=ev-trace-1"]')?.textContent).toContain("查看证据");
    expect(container.querySelector('a[href="/debug/prompts?trace_id=prompt-trace-1"]')?.textContent).toContain("查看 Prompt Trace");
  });

  it("renders rca_report artifacts with the RCA component", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-rca",
            type: "rca_report",
            titleZh: "checkout 根因分析",
            summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
            source: "coroot",
            permissionScope: "read",
            redactionStatus: "redacted",
            inlineData: {
              schemaVersion: "aiops.rca_report/v1",
              source: "coroot",
              status: "ok",
              target: { service: "checkout" },
              window: { timeRange: "30m" },
              conclusion: {
                summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
                confidence: 0.72,
              },
              hypotheses: [],
              sections: [],
              evidenceRefs: [],
              rawRefs: [],
              limitations: [],
            },
          }}
        />,
      );
    });

    expect(container.querySelector('[data-testid="rca-report-artifact"]')).toBeTruthy();
    expect(container.textContent).not.toContain("schemaVersion");
  });

  it("renders Coroot chart empty, permission, redaction, and unavailable states without executing HTML", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-coroot-empty",
            type: "coroot_chart",
            titleZh: "Coroot 指标窗口",
            summaryZh: "<img src=x onerror=alert(1)>",
            status: "blocked",
            permissionScope: "restricted",
            redactionStatus: "redacted",
            mcpCard: {
              uiKind: "readonly_chart",
              title: "web-checkout 指标",
              error: "dial tcp: connection refused",
              visual: {
                kind: "timeseries",
                series: [{ name: "p95_latency_ms", data: [] }],
              },
            },
            inlineData: {
              html: "<img src=x onerror=alert(1)>",
              script: "alert(1)",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("当前时间范围内暂无可用指标数据");
    expect(container.textContent).toContain("权限不足，无法查看完整 Coroot 指标");
    expect(container.textContent).toContain("部分字段已脱敏");
    expect(container.textContent).toContain("Coroot 暂不可用");
    expect(container.querySelector("img")).toBeNull();
    expect(container.innerHTML).not.toContain("onerror");
    expect(container.innerHTML).not.toContain("alert(1)");
  });

  it("does not expose sensitive inline data when artifact permission is restricted", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-restricted-inline-data",
            type: "topology_slice",
            titleZh: "权限受限拓扑",
            summaryZh: "仅展示拓扑摘要。",
            permissionScope: "restricted",
            inlineData: {
              namespace: "prod",
              secretToken: "sk-live-sensitive",
              dbPassword: "db-password-sensitive",
              html: "<img src=x onerror=alert(1)>",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("权限受限拓扑");
    expect(container.textContent).toContain("仅展示拓扑摘要。");
    expect(container.textContent).not.toContain("secretToken");
    expect(container.textContent).not.toContain("sk-live-sensitive");
    expect(container.textContent).not.toContain("dbPassword");
    expect(container.textContent).not.toContain("db-password-sensitive");
    expect(container.querySelector("img")).toBeNull();
    expect(container.innerHTML).not.toContain("onerror");
  });

  it("renders trace summary core fields", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-trace-summary",
            type: "trace_summary",
            titleZh: "Trace 摘要",
            payload: {
              traceId: "trace-checkout-001",
              durationMs: 2840,
              slowestSpan: { name: "POST /api/checkout", durationMs: 2310 },
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("Trace ID");
    expect(container.textContent).toContain("trace-checkout-001");
    expect(container.textContent).toContain("总耗时");
    expect(container.textContent).toContain("2840 ms");
    expect(container.textContent).toContain("最慢 Span");
    expect(container.textContent).toContain("POST /api/checkout");
  });

  it("renders workflow result core fields", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-workflow-result",
            type: "workflow_result",
            titleZh: "Workflow 结果",
            payload: {
              hostLease: { leaseId: "lease-host-07", status: "active" },
              failed_step: "reload_nginx",
              rollback_result: "已回滚到 reload 前配置。",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("主机租约");
    expect(container.textContent).toContain("lease-host-07");
    expect(container.textContent).toContain("失败步骤");
    expect(container.textContent).toContain("reload_nginx");
    expect(container.textContent).toContain("回滚结果");
    expect(container.textContent).toContain("已回滚到 reload 前配置。");
  });

  it("renders verification result core fields", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-verification-result",
            type: "verification_result",
            titleZh: "验证结果",
            payload: {
              beforeMetrics: { p95_latency_ms: 2800, error_rate: "4.2%" },
              afterMetrics: { p95_latency_ms: 430, error_rate: "0.1%" },
              recoveryConclusion: "业务接口已恢复到 SLO 内。",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("修复前指标");
    expect(container.textContent).toContain("p95_latency_ms：2800");
    expect(container.textContent).toContain("修复后指标");
    expect(container.textContent).toContain("p95_latency_ms：430");
    expect(container.textContent).toContain("恢复结论");
    expect(container.textContent).toContain("业务接口已恢复到 SLO 内。");
  });

  it("renders experience match core fields", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-experience-match",
            type: "experience_match",
            titleZh: "经验命中",
            payload: {
              matchReasons: ["trace 签名一致", "服务路径一致"],
              risks: ["reload 可能短暂断连"],
              validationItems: ["确认新 trace p95 小于 500ms", "检查错误率低于 0.5%"],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("命中原因");
    expect(container.textContent).toContain("trace 签名一致");
    expect(container.textContent).toContain("风险");
    expect(container.textContent).toContain("reload 可能短暂断连");
    expect(container.textContent).toContain("验证项");
    expect(container.textContent).toContain("确认新 trace p95 小于 500ms");
  });
});

describe("McpSurfacePart", () => {
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

  it("renders user-visible MCP surface actions in Chinese", async () => {
    await act(async () => {
      root.render(<McpSurfacePart surface={{ id: "coroot", title: "Coroot MCP", status: "connected", pinned: true }} />);
    });

    expect(container.textContent).toContain("关闭");
    expect(container.textContent).toContain("刷新");
    expect(container.textContent).toContain("取消固定");
    expect(container.textContent).not.toContain("Close");
    expect(container.textContent).not.toContain("Refresh");
    expect(container.textContent).not.toContain("Unpin");

    await act(async () => {
      root.render(<McpSurfacePart surface={{ id: "coroot", title: "Coroot MCP", status: "disconnected", pinned: false }} />);
    });

    expect(container.textContent).toContain("打开");
    expect(container.textContent).toContain("固定");
    expect(container.textContent).not.toContain("Open");
    expect(container.textContent).not.toContain("Pin");
  });
});
