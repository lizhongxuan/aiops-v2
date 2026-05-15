import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  OpsManualMatchArtifact,
  OpsManualSearchResultArtifact,
  RunnerWorkflowGenerationArtifact,
} from "./OpsManualChatArtifacts";
import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";

describe("OpsManualChatArtifacts", () => {
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

  it("renders need_info search result without a fake match percentage", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              summary: "信息不足，不能直接使用工作流。",
              operation_frame: { target: { type: "redis" }, operation: { action: "rca" } },
              next_questions: ["目标 Redis 实例是哪一个？", "部署方式是 Kubernetes、Docker 还是物理机？"],
              score: 0.83,
              percentage: "83%",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("需补充信息");
    expect(container.textContent).toContain("信息不足，不能直接使用工作流。");
    expect(container.textContent).toContain("redis / rca");
    expect(container.textContent).toContain("目标 Redis 实例是哪一个？");
    expect(container.textContent).toContain("部署方式是 Kubernetes、Docker 还是物理机？");
    expect(container.textContent).not.toMatch(/\d+\s*%/);
    expect(container.textContent).not.toContain("命中率");
  });

  it("renders adapt search result with environment diffs and variant action", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-adapt",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "adapt",
              summary: "找到 PostgreSQL 备份手册，但当前环境需要适配。",
              operation_frame: { object_type: "postgresql", operation_type: "backup" },
              manuals: [
                {
                  manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册" },
                  bound_workflow_id: "workflow-pg-backup-ubuntu",
                  usable_mode: "adapt",
                  matched_fields: ["object_type", "operation_type"],
                  environment_diffs: ["os", "package_manager"],
                  blocked_reasons: ["workflow targets ubuntu apt/systemd but current host is centos/yum/systemd"],
                  recommended_action: "generate_workflow_variant",
                  score: 0.76,
                },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("需适配");
    expect(container.textContent).toContain("PostgreSQL 备份 Ubuntu 运维手册");
    expect(container.textContent).toContain("workflow-pg-backup-ubuntu");
    expect(container.textContent).toContain("os；package_manager");
    expect(container.textContent).toContain("workflow targets ubuntu apt/systemd but current host is centos/yum/systemd");
    expect(Array.from(container.querySelectorAll("button")).some((button) => button.textContent?.includes("生成适配工作流"))).toBe(true);
    expect(container.textContent).not.toMatch(/\d+\s*%/);
  });

  it("renders direct_execute search result as a confirmed-before-run workflow entry", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-direct-execute",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "direct_execute",
              summary: "找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。",
              operation_frame: { object_type: "redis", operation_type: "rca_or_repair" },
              manuals: [
                {
                  manual: { id: "manual-redis-local-readonly-rca", title: "Redis 本机只读排障运维手册" },
                  bound_workflow_id: "workflow-redis-local-readonly-rca",
                  usable_mode: "direct_execute",
                  matched_fields: ["object_type", "operation_type", "execution_surface", "required_context"],
                  recommended_action: "run_bound_workflow",
                  run_record_summary: { success_count: 1, failure_count: 0, recent_result: "passed" },
                },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("可直接执行");
    expect(container.textContent).toContain("Redis 本机只读排障运维手册");
    expect(container.textContent).toContain("workflow-redis-local-readonly-rca");
    expect(container.textContent).toContain("Dry Run");
    expect(container.textContent).toContain("用户确认前不会执行 Runner Workflow");
    expect(container.textContent).not.toContain("Runner 已执行");
    expect(container.textContent).not.toMatch(/\d+\s*%/);
  });

  it("renders direct ops manual match without a hit percentage", async () => {
    await act(async () => {
      root.render(
        <OpsManualMatchArtifact
          artifact={{
            id: "artifact-ops-manual-direct",
            type: "ops_manual_match",
            titleZh: "Redis 运维手册",
            inlineData: {
              manualId: "manual-redis-memory",
              manualTitle: "Redis 内存压力排障",
              state: "direct",
              workflowRef: { workflowId: "workflow-redis-memory" },
              reasons: ["中间件匹配：redis", "执行面匹配：ssh"],
              runRecordSummary: { successCount: 7, failureCount: 0, recentResult: "success" },
              score: 0.97,
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("可直接执行");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.textContent).toContain("workflow-redis-memory");
    expect(container.textContent).toContain("查看参数");
    expect(container.textContent).toContain("开始前置检查");
    expect(container.textContent).not.toMatch(/\d+\s*%/);
    expect(container.textContent).not.toContain("命中率");
  });

  it("dispatches generation confirmation from the adapt search result action", async () => {
    let detail: unknown = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-confirmation", handler);

    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-adapt-confirm",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "adapt",
              summary: "需要生成适配工作流。",
              manuals: [
                {
                  manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册" },
                  usable_mode: "adapt",
                  recommended_action: "generate_workflow_variant",
                },
              ],
            },
          }}
        />,
      );
    });

    const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes("生成适配工作流"));
    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    window.removeEventListener("aiops:composer-confirmation", handler);
    expect(detail).toMatchObject({
      action: "generate_runner_workflow_candidate",
      title: "生成适配工作流",
      sourceTitle: "PostgreSQL 备份 Ubuntu 运维手册",
      artifactId: "artifact-adapt-confirm",
    });
  });

  it("renders runner workflow generation progress as a status timeline", async () => {
    await act(async () => {
      root.render(
        <RunnerWorkflowGenerationArtifact
          artifact={{
            id: "artifact-generation",
            type: "runner_workflow_generation",
            inlineData: {
              workflowTitle: "Redis 内存压力排障工作流",
              steps: [
                { id: "extract", title: "提取参数", status: "passed", redactedLog: "host=redis-prod-01" },
                { id: "build", title: "生成节点", status: "running", redactedLog: "secret_ref=***" },
                { id: "verify", title: "静态校验", status: "waiting" },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("Redis 内存压力排障工作流");
    expect(container.textContent).toContain("提取参数");
    expect(container.textContent).toContain("已通过");
    expect(container.textContent).toContain("生成节点");
    expect(container.textContent).toContain("执行中");
    expect(container.textContent).toContain("静态校验");
    expect(container.textContent).toContain("等待中");
    expect(container.textContent).not.toContain("secret_ref=***");
  });

  it("does not render manual approval steps in runner workflow generation progress", async () => {
    await act(async () => {
      root.render(
        <RunnerWorkflowGenerationArtifact
          artifact={{
            id: "artifact-generation-no-approval",
            type: "runner_workflow_generation",
            inlineData: {
              workflowTitle: "PostgreSQL 备份 CentOS 工作流",
              steps: [
                { id: "precheck", title: "环境预检查", status: "passed" },
                { id: "approval", title: "人工审批", status: "running" },
                { id: "dry_run", title: "Dry Run", status: "waiting" },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("PostgreSQL 备份 CentOS 工作流");
    expect(container.textContent).toContain("环境预检查");
    expect(container.textContent).toContain("Dry Run");
    expect(container.textContent).not.toContain("人工审批");
  });

  it("registers ops manual artifacts in the generic Agent-to-UI dispatcher", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-ops-manual",
            type: "ops_manual_match",
            titleZh: "运维手册判定",
            summaryZh: "找到相似运维手册，但当前环境存在差异，需要先生成变体并校验。",
            source: "ai-chat",
            createdAt: "2026-05-15T01:03:10+08:00",
            inlineData: {
              manualId: "manual-redis-memory",
              manualTitle: "Redis 内存压力排障",
              state: "need_more_info",
              missingContext: ["target_instance", "metrics"],
              recommendedNextActions: ["request_coroot_permission"],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("需补充信息");
    expect(container.textContent).toContain("target_instance");
    expect(container.textContent).not.toContain("运维手册判定");
    expect(container.textContent).not.toContain("找到相似运维手册");
    expect(container.textContent).not.toContain("来源：");
    expect(container.textContent).not.toContain("生成时间：");
    expect(container.textContent).not.toContain("暂不支持的卡片类型");
  });

  it("registers search result artifacts in the generic Agent-to-UI dispatcher", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-ops-manual-search",
            type: "ops_manual_search_result",
            titleZh: "运维手册检索",
            summaryZh: "工具返回的信息不足判定。",
            source: "search_ops_manuals",
            createdAt: "2026-05-15T01:03:10+08:00",
            inlineData: {
              decision: "need_info",
              summary: "信息不足，不能直接使用工作流。",
              next_questions: ["目标 Redis 实例是哪一个？"],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("需补充信息");
    expect(container.textContent).toContain("目标 Redis 实例是哪一个？");
    expect(container.textContent).not.toContain("运维手册检索");
    expect(container.textContent).not.toContain("工具返回的信息不足判定。");
    expect(container.textContent).not.toContain("来源：");
    expect(container.textContent).not.toContain("生成时间：");
    expect(container.textContent).not.toContain("暂不支持的卡片类型");
  });
});
