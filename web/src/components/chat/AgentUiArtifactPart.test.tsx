import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { AgentUiArtifactPart } from "./AgentUiArtifactPart";
import { CorootChartArtifact } from "./CorootChartArtifact";
import { ExperienceMatchArtifact } from "./ExperienceMatchArtifact";
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
  });

  it("renders a Coroot chart artifact with Chinese summary and Case link", async () => {
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
    expect(container.textContent).toContain("接口 P95 延迟在 14:03 后明显升高。");
    expect(container.textContent).toContain("p95_latency_ms");
    expect(container.textContent).toContain("已脱敏");
    expect(container.querySelector('a[href="/incidents/case-debug-1"]')?.textContent).toContain("查看 Case");
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
    expect(container.innerHTML).not.toContain("dangerouslySetInnerHTML");
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
    expect(container.textContent).toContain("p95 从 420ms 升至 2.8s。");
    expect(container.textContent).toContain("p95");
    expect(container.querySelector('a[href="/incidents/case-debug-2"]')?.textContent).toContain("查看 Case");
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

  it("renders experience_match Skill, preconditions, OS variant, runner binding, and history without execute_now", async () => {
    await act(async () => {
      root.render(
        <ExperienceMatchArtifact
          artifact={{
            id: "match-pg-lag",
            type: "experience_match",
            title: "命中经验",
            payload: {
              skill: { name: "PG 主从延迟诊断" },
              compatibilityStatus: "adapt_required",
              compatibilityGaps: ["操作系统不同：请求 centos，经验包 ubuntu"],
              matchedSignals: ["pg_replication_lag", "wal_sender_wait"],
              preconditionGaps: ["需要确认目标主机操作系统"],
              osVariant: "linux",
              runnerBinding: { workflowName: "PG Lag Dry Run", status: "ready" },
              history: { successCount: 7, failureCount: 1, recentResult: "success" },
              actions: [{ id: "execute_now", label: "立即执行" }],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("Skill");
    expect(container.textContent).toContain("PG 主从延迟诊断");
    expect(container.textContent).toContain("兼容状态");
    expect(container.textContent).toContain("需适配后使用");
    expect(container.textContent).toContain("适配差异");
    expect(container.textContent).toContain("操作系统不同");
    expect(container.textContent).toContain("命中原因");
    expect(container.textContent).toContain("pg_replication_lag");
    expect(container.textContent).toContain("缺失前置条件");
    expect(container.textContent).toContain("需要确认目标主机操作系统");
    expect(container.textContent).toContain("OS 变体");
    expect(container.textContent).toContain("linux");
    expect(container.textContent).toContain("Runner Binding");
    expect(container.textContent).toContain("PG Lag Dry Run");
    expect(container.textContent).toContain("历史成功/失败");
    expect(container.textContent).toContain("7");
    expect(container.textContent).toContain("1");
    expect(container.textContent).not.toContain("立即执行");
  });

  it("renders an experience-pack candidate artifact after user confirmation", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-candidate-1",
            type: "experience_pack_candidate",
            titleZh: "经验包候选已生成",
            summaryZh: "PostgreSQL 主从集群部署经验 已生成候选资产，审核通过后才能启用。",
            redactionStatus: "redacted",
            inlineData: {
              candidateId: "candidate-pg",
              packId: "pack-pg",
              reviewStatus: "candidate",
            },
            actions: [{ id: "review", label: "去审核", href: "/settings/experience-packs?tab=review" }],
          }}
        />,
      );
    });

    expect(container.textContent).toContain("经验包候选已生成");
    expect(container.textContent).toContain("经验候选");
    expect(container.textContent).toContain("candidate-pg");
    expect(container.textContent).toContain("去审核");
    expect(container.querySelector('[data-testid="experience-pack-candidate-artifact"]')).not.toBeNull();
  });

  it("renders runner workflow candidate as a read-only progressive UI card without navigation", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "runner-candidate-1",
            type: "runner_workflow_candidate",
            titleZh: "Runner Workflow 草稿已生成",
            summaryZh: "Redis 运维排障经验包 Workflow 已写入 Runner Studio 本地草稿。",
            redactionStatus: "redacted",
            inlineData: {
              workflowId: "wf-redis",
              workflowName: "Redis 运维排障经验包 Workflow",
              nodes: [
                { id: "precheck", title: "环境预检查", detail: "来自后端 Runner graph" },
                { id: "approval", title: "人工审批", detail: "来自后端 Runner graph" },
                { id: "dry_run", title: "Dry Run", detail: "来自后端 Runner graph" },
                { id: "execute", title: "受控执行", detail: "来自后端 Runner graph" },
                { id: "validate", title: "恢复验证", detail: "来自后端 Runner graph" },
                { id: "rollback", title: "受控回滚", detail: "来自后端 Runner graph" },
              ],
            },
            actions: [{ id: "open-runner", label: "打开 Runner Studio", href: "/runner/wf-redis" }],
          }}
        />,
      );
    });

    expect(container.textContent).toContain("节点生成进度");
    expect(container.textContent).toContain("环境预检查");
    expect(container.textContent).toContain("人工审批");
    expect(container.textContent).toContain("Dry Run");
    expect(container.textContent).toContain("受控执行");
    expect(container.textContent).toContain("恢复验证");
    expect(container.textContent).toContain("受控回滚");
    expect(container.textContent).toContain("只读预览");
    expect(container.querySelector('a[href="/runner/wf-redis"]')).toBeNull();

    const previewButton = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("打开 Runner Studio 只读预览"));
    expect(previewButton).toBeTruthy();
    await act(async () => {
      previewButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    const dialog = container.querySelector('[role="dialog"][aria-modal="true"]');
    expect(dialog?.textContent).toContain("Runner Studio 只读预览");
    expect(dialog?.textContent).toContain("AI 正在逐步创建节点");
    expect(dialog?.textContent).toContain("不能在这里编辑或发布工作流");
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
