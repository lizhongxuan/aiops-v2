import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import {
  OpsManualFallbackGuideArtifact,
  OpsManualMatchArtifact,
  OpsManualParamResolutionArtifact,
  OpsManualPreflightResultArtifact,
  OpsManualSearchResultArtifact,
  RunnerWorkflowGenerationArtifact,
} from "./OpsManualChatArtifacts";
import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

describe("OpsManualChatArtifacts", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    (
      globalThis as { IS_REACT_ACT_ENVIRONMENT?: boolean }
    ).IS_REACT_ACT_ENVIRONMENT = true;
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

  it("does not auto-continue or open a fixed form for need_info search results before parameter resolution", async () => {
    let contextRequest: { fields?: Array<{ id: string }> } | null = null;
    let contextSubmit: { text?: string; artifactId?: string } | null = null;
    const requestHandler = (event: Event) => {
      contextRequest = (event as CustomEvent).detail;
    };
    const submitHandler = (event: Event) => {
      contextSubmit = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", requestHandler);
    window.addEventListener("aiops:composer-context-submit", submitHandler);

    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              summary: "信息不足，不能直接使用工作流。",
              operation_frame: {
                target: { type: "redis" },
                operation: { action: "rca" },
              },
              manuals: [
                {
                  manual: {
                    id: "manual-redis-rca-ssh",
                    title: "Redis SSH 排障运维手册",
                  },
                  matched_fields: ["object_type", "operation_type"],
                  blocked_reasons: ["required context missing"],
                },
              ],
              next_questions: [
                "目标 Redis 实例是哪一个？",
                "部署方式是 Kubernetes、Docker 还是物理机？",
              ],
              score: 0.83,
              percentage: "83%",
            },
          }}
        />,
      );
    });
    await act(flushTimers);

    expect(container.textContent).toContain("运维手册检索");
    expect(container.textContent).not.toContain("手册缺上下文");
    expect(container.textContent).toContain("暂未进入 Workflow 预检");
    expect(container.textContent).not.toContain(
      "信息不足，不能直接使用工作流。",
    );
    expect(container.textContent).not.toContain("请在底部补充");
    expect(container.textContent).not.toContain("打开补充表单");
    expect(
      container.querySelector('[data-testid="ops-manual-context-prompt"]'),
    ).toBeNull();
    expect(contextRequest).toBeNull();
    expect(contextSubmit).toBeNull();
    expect(
      window.sessionStorage.getItem("aiops:auto-context:artifact-need-info"),
    ).toBeNull();
    expect(container.textContent).toContain("候选手册");
    expect(container.textContent).toContain("Redis SSH 排障运维手册");
    const candidateToggle = container.querySelector(
      '[data-testid="ops-manual-candidate-toggle"]',
    ) as HTMLButtonElement | null;
    expect(candidateToggle).not.toBeNull();
    expect(candidateToggle?.getAttribute("aria-expanded")).toBe("false");
    expect(
      container.querySelector(
        '[data-testid="ops-manual-candidate-match-detail"]',
      ),
    ).toBeNull();
    await act(async () => {
      candidateToggle?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });
    expect(candidateToggle?.getAttribute("aria-expanded")).toBe("true");
    expect(
      container.querySelector(
        '[data-testid="ops-manual-candidate-match-detail"]',
      ),
    ).not.toBeNull();
    expect(container.textContent).toContain("命中依据");
    expect(container.textContent).toContain("对象类型；操作类型");
    expect(container.textContent).toContain("缺少目标位置");
    expect(container.textContent).not.toContain(
      "信息不足，不能直接使用工作流。",
    );
    expect(container.textContent).toContain("redis / rca");
    expect(container.textContent).not.toContain("补充上下文");
    expect(container.textContent).not.toContain("目标 Redis 实例是哪一个？");
    expect(container.textContent).not.toContain(
      "部署方式是 Kubernetes、Docker 还是物理机？",
    );
    expect(container.textContent).not.toMatch(/\d+\s*%/);
    expect(container.textContent).not.toContain("命中率");
    expect(container.textContent).not.toContain("manual-redis");
    expect(container.textContent).not.toContain("绑定 Workflow");
    expect(container.textContent).not.toContain("匹配字段");
    expect(container.textContent).not.toContain("已检索字段");

    window.removeEventListener(
      "aiops:composer-context-request",
      requestHandler,
    );
    window.removeEventListener("aiops:composer-context-submit", submitHandler);
  });

  it("renders resolved parameter resolution as the preflight entry point", async () => {
    await act(async () => {
      root.render(
        <OpsManualParamResolutionArtifact
          artifact={{
            id: "artifact-param-resolved",
            type: "ops_manual_param_resolution",
            inlineData: {
              status: "resolved",
              manual_id: "manual-redis-rca-ssh",
              workflow_id: "workflow-redis-rca-ssh",
              resolved_params: [
                {
                  id: "target_host",
                  value: "server-local",
                  source: "selected_host",
                  confidence: 1,
                  evidence: "当前选择主机",
                },
                {
                  id: "redis_instance",
                  value: "docker:aiops-redis",
                  source: "docker",
                  confidence: 0.94,
                  evidence: "docker ps discovered one Redis container",
                },
              ],
              next_action: "run_preflight",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("参数已补齐，可进入预检");
    expect(container.textContent).toContain("manual-redis-rca-ssh");
    expect(container.textContent).toContain("server-local");
    expect(container.textContent).toContain("docker:aiops-redis");
    expect(container.textContent).toContain("当前选择主机");
    expect(container.textContent).toContain("运行预检");
    expect(container.textContent).not.toContain("resolver_log");
    expect(container.textContent).not.toContain("请在底部补充");
  });

  it("renders resolved parameter resolution as completed when preflight is already merged", async () => {
    await act(async () => {
      root.render(
        <OpsManualParamResolutionArtifact
          artifact={{
            id: "artifact-param-resolved-with-preflight",
            type: "ops_manual_param_resolution",
            inlineData: {
              status: "resolved",
              manual_id: "manual-redis-rca-ssh",
              workflow_id: "workflow-redis-rca-ssh",
              resolved_params: [
                {
                  id: "target_host",
                  value: "server-local",
                  source: "selected_host",
                },
                {
                  id: "target_instance",
                  value: "docker:aiops-redis",
                  source: "docker",
                },
              ],
              merged_preflight_result: {
                status: "passed",
                ready: true,
                manual_id: "manual-redis-rca-ssh",
                workflow_id: "workflow-redis-rca-ssh",
              },
            },
          }}
        />,
      );
    });

    expect(
      container.querySelector(
        '[data-testid="ops-manual-param-preflight-completed"]',
      ),
    ).not.toBeNull();
    expect(container.textContent).toContain("预检通过");
    expect(container.querySelector("button")?.textContent).not.toContain(
      "运行预检",
    );
  });

  it("renders need_info search as preflight-completed when a later preflight result is merged", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info-merged",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              operation_frame: {
                object_type: "redis",
                operation_type: "rca_or_repair",
              },
              manuals: [
                {
                  manual: {
                    id: "manual-redis-rca-ssh",
                    title: "Redis SSH 排障运维手册",
                  },
                  bound_workflow_id: "workflow-redis-rca-ssh",
                  blocked_reasons: ["required context missing"],
                },
              ],
              merged_preflight_result: {
                status: "passed",
                ready: true,
                manual_id: "manual-redis-rca-ssh",
                workflow_id: "workflow-redis-rca-ssh",
                evidence: [{ name: "redis_ping", status: "passed" }],
              },
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("Workflow 预检通过");
    expect(container.textContent).toContain("预检通过");
    expect(container.textContent).toContain("redis_ping");
    expect(container.textContent).not.toContain("暂未进入 Workflow 预检");
  });

  it("renders ambiguous parameter resolution and dispatches only the returned dynamic fields", async () => {
    let detail: {
      artifactId?: string;
      manualId?: string;
      workflowId?: string;
      fields?: Array<{ id: string; candidates?: unknown[] }>;
    } | null = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", handler);

    await act(async () => {
      root.render(
        <OpsManualParamResolutionArtifact
          artifact={{
            id: "artifact-param-ambiguous",
            type: "ops_manual_param_resolution",
            inlineData: {
              status: "ambiguous",
              manual_id: "manual-redis-rca-ssh",
              workflow_id: "workflow-redis-rca-ssh",
              fields: [
                {
                  id: "redis_instance",
                  label: "Redis 实例",
                  type: "resource_ref",
                  ui_control: "select",
                  required: true,
                  candidates: [
                    {
                      value: "docker:redis-1",
                      label: "redis-1",
                      source: "docker",
                      confidence: 0.91,
                    },
                    {
                      value: "docker:redis-2",
                      label: "redis-2",
                      source: "docker",
                      confidence: 0.9,
                    },
                  ],
                },
              ],
            },
          }}
        />,
      );
    });
    await act(flushTimers);

    expect(container.textContent).toContain("需要确认参数");
    expect(container.textContent).toContain("Redis 实例");
    expect(container.textContent).toContain("redis-1");
    expect(container.textContent).toContain("redis-2");
    expect(container.textContent).not.toContain("目标位置");
    expect(container.textContent).not.toContain("访问/执行入口");
    expect(detail).toMatchObject({
      artifactId: "artifact-param-ambiguous",
      manualId: "manual-redis-rca-ssh",
      workflowId: "workflow-redis-rca-ssh",
    });
    expect(detail?.fields?.map((field) => field.id)).toEqual([
      "redis_instance",
    ]);
    expect(detail?.fields?.[0]?.candidates).toHaveLength(2);

    window.removeEventListener("aiops:composer-context-request", handler);
  });

  it("renders missing parameter resolution without a fixed four-field fallback", async () => {
    let detail: { fields?: Array<{ id: string }> } | null = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", handler);

    await act(async () => {
      root.render(
        <OpsManualParamResolutionArtifact
          artifact={{
            id: "artifact-param-missing",
            type: "ops_manual_param_form",
            inlineData: {
              status: "need_user_input",
              manual_id: "manual-pg-backup",
              workflow_id: "workflow-pg-backup",
              fields: [
                {
                  id: "backup_path",
                  label: "备份路径",
                  type: "path",
                  ui_control: "text",
                  required: true,
                  placeholder: "例如 /data/backups",
                },
              ],
            },
          }}
        />,
      );
    });
    await act(flushTimers);

    expect(container.textContent).toContain("需要补充参数");
    expect(container.textContent).toContain("备份路径");
    expect(container.textContent).not.toContain("目标位置");
    expect(container.textContent).not.toContain("实例/服务");
    expect(container.textContent).not.toContain("访问/执行入口");
    expect(container.textContent).not.toContain("现象/证据");
    expect(detail?.fields?.map((field) => field.id)).toEqual(["backup_path"]);

    window.removeEventListener("aiops:composer-context-request", handler);
  });

  it("shows discovery blocker when target resource is not found", async () => {
    let detail: {
      fields?: Array<{ id: string; placeholder?: string }>;
    } | null = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", handler);

    await act(async () => {
      root.render(
        <OpsManualParamResolutionArtifact
          artifact={{
            id: "artifact-param-no-resource",
            type: "ops_manual_param_resolution",
            inlineData: {
              status: "need_user_input",
              manual_id: "manual-redis-rca-ssh",
              workflow_id: "workflow-redis-rca-ssh",
              resolved_params: [
                {
                  id: "target_host",
                  value: "server-local",
                  source: "selected_host",
                },
              ],
              fields: [
                {
                  id: "target_instance",
                  label: "实例/服务",
                  type: "resource_ref",
                  ui_control: "select",
                  required: true,
                  placeholder:
                    "No Redis resource was discovered on server-local by read-only resource discovery.",
                },
              ],
            },
          }}
        />,
      );
    });
    await act(flushTimers);

    expect(container.textContent).toContain("需要补充参数");
    expect(container.textContent).toContain(
      "No Redis resource was discovered on server-local by read-only resource discovery.",
    );
    expect(detail?.fields?.map((field) => field.id)).toEqual([
      "target_instance",
    ]);
    expect(detail?.fields?.[0]?.placeholder).toContain(
      "No Redis resource was discovered",
    );

    window.removeEventListener("aiops:composer-context-request", handler);
  });

  it("does not fabricate a bottom form for need_info search results when the tool omits required fields", async () => {
    let detail: { fields?: Array<{ id: string }> } | null = null;
    let submitDetail: { text?: string } | null = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    const submitHandler = (event: Event) => {
      submitDetail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", handler);
    window.addEventListener("aiops:composer-context-submit", submitHandler);

    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info-fallback",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              operation_frame: {
                target: { type: "redis" },
                operation: { action: "rca" },
              },
            },
          }}
        />,
      );
    });
    await act(flushTimers);

    expect(container.textContent).toContain("运维手册检索");
    expect(container.textContent).not.toContain("手册缺上下文");
    expect(container.textContent).toContain("暂未进入 Workflow 预检");
    expect(container.textContent).not.toContain("请在底部补充");
    expect(container.textContent).not.toContain("打开补充表单");
    expect(
      container.querySelector('[data-testid="ops-manual-context-prompt"]'),
    ).toBeNull();
    expect(detail).toBeNull();
    expect(submitDetail).toBeNull();
    expect(container.textContent).not.toContain(
      "请确认 redis / rca 的目标实例或服务名称。",
    );
    expect(container.textContent).not.toContain(
      "请补充部署形态、访问方式和必要只读证据。",
    );
    expect(container.textContent).not.toContain("立即执行");
    expect(container.textContent).not.toContain("授权读取 Coroot");
    expect(container.textContent).not.toContain("选择目标实例");

    window.removeEventListener("aiops:composer-context-request", handler);
    window.removeEventListener("aiops:composer-context-submit", submitHandler);
  });

  it("shows inferred match reasons for a compact manual candidate when matched_fields are omitted", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info-inferred-match",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              summary: "信息不足，不能直接使用工作流。",
              operation_frame: {
                target: { type: "redis" },
                operation: { action: "rca_or_repair" },
              },
              manuals: [
                {
                  manual: {
                    id: "manual-redis-rca-ssh",
                    title: "Redis SSH 排障运维手册",
                  },
                },
              ],
            },
          }}
        />,
      );
    });

    expect(
      container.querySelector(
        '[data-testid="ops-manual-candidate-match-detail"]',
      ),
    ).toBeNull();
    const candidateToggle = container.querySelector(
      '[data-testid="ops-manual-candidate-toggle"]',
    ) as HTMLButtonElement | null;
    await act(async () => {
      candidateToggle?.dispatchEvent(
        new MouseEvent("click", { bubbles: true }),
      );
    });

    expect(container.textContent).toContain("命中依据");
    expect(container.textContent).toContain("对象类型 Redis");
    expect(container.textContent).toContain("操作类型 排障/修复");
  });

  it("lets the user skip ops manual usage and continue step-by-step operations", async () => {
    let detail: {
      text?: string;
      artifactId?: string;
      metadata?: Record<string, string>;
    } | null = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-submit", handler);

    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info-skip",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              operation_frame: {
                target: { type: "redis" },
                operation: { action: "rca_or_repair" },
              },
              required_context_fields: [
                {
                  id: "target_location",
                  label: "目标位置",
                  placeholder: "server-local",
                },
              ],
              manuals: [
                {
                  manual: {
                    id: "manual-redis-rca-ssh",
                    title: "Redis SSH 排障运维手册",
                  },
                },
              ],
            },
          }}
        />,
      );
    });

    const skipButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("不使用"),
    );
    expect(skipButton).toBeTruthy();
    await act(async () => {
      skipButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(detail).toMatchObject({
      artifactId: "artifact-need-info-skip",
    });
    expect(detail?.text).toContain("已选择跳过运维手册");
    expect(detail?.text).toContain("不要再调用 search_ops_manuals");
    expect(detail?.text).toContain("resolve_ops_manual_params");
    expect(detail?.text).toContain("run_ops_manual_preflight");
    expect(detail?.text).toContain("普通只读排查");
    expect(detail?.text).toContain("当前选择主机");
    expect(detail?.metadata?.opsManualAction).toBe("skip_ops_manual");
    expect(detail?.metadata?.opsManualSkipped).toBe("true");
    expect(detail?.metadata?.opsManualManualId).toBe("manual-redis-rca-ssh");
    expect(detail?.text).not.toContain("\n");
    expect(detail?.text).not.toContain("��");
    expect(container.textContent).toContain(
      "已切换为普通只读排查，等待 Agent 继续处理。",
    );
    expect((skipButton as HTMLButtonElement | undefined)?.disabled).toBe(true);

    window.removeEventListener("aiops:composer-context-submit", handler);
  });

  it("opens a read-only workflow workspace preview from the compact manual candidate", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info-workflow-preview",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              operation_frame: {
                target: { type: "redis" },
                operation: { action: "rca_or_repair" },
              },
              manuals: [
                {
                  manual: {
                    id: "manual-redis-rca-ssh",
                    title: "Redis SSH 排障运维手册",
                  },
                  bound_workflow_id: "workflow-redis-rca-ssh",
                  workflow_preview: {
                    title: "Redis SSH 排障工作流",
                    nodes: [
                      {
                        id: "collect",
                        title: "采集只读指标",
                        command: "redis-cli INFO memory",
                        summary: "读取内存和慢查询指标",
                      },
                      {
                        id: "analyze",
                        title: "判断内存压力",
                        command: "compare used_memory_rss",
                        summary: "检查 RSS 和 maxmemory 差异",
                      },
                    ],
                  },
                },
              ],
            },
          }}
        />,
      );
    });

    const previewButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("查看工作流"),
    );
    expect(previewButton).toBeTruthy();
    await act(async () => {
      previewButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(document.body.textContent).toContain("工作流只读预览");
    expect(document.body.textContent).toContain("Redis SSH 排障工作流");
    expect(document.body.textContent).toContain("采集只读指标");
    expect(document.body.textContent).toContain("redis-cli INFO memory");

    const analyzeNode = Array.from(
      document.body.querySelectorAll("button"),
    ).find((button) => button.textContent?.includes("判断内存压力"));
    await act(async () => {
      analyzeNode?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(document.body.textContent).toContain("compare used_memory_rss");
  });

  it("opens a read-only ops manual document preview from the compact manual candidate", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-need-info-manual-preview",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              operation_frame: {
                target: { type: "redis" },
                operation: { action: "rca_or_repair" },
              },
              manuals: [
                {
                  manual: {
                    id: "manual-redis-rca-ssh",
                    title: "Redis SSH 排障运维手册",
                    description: "用于 Redis SSH 场景的只读排障和恢复前验证。",
                    content:
                      "适用场景：Redis 内存压力、慢查询、连接异常。验证方式：检查 INFO memory 和业务 p95。",
                  },
                },
              ],
            },
          }}
        />,
      );
    });

    const manualButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("查看手册"),
    );
    expect(manualButton).toBeTruthy();
    await act(async () => {
      manualButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(document.body.textContent).toContain("运维手册只读预览");
    expect(document.body.textContent).toContain("Redis SSH 排障运维手册");
    expect(document.body.textContent).toContain(
      "用于 Redis SSH 场景的只读排障和恢复前验证",
    );
    expect(document.body.textContent).toContain(
      "适用场景：Redis 内存压力、慢查询、连接异常",
    );
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
              operation_frame: {
                object_type: "postgresql",
                operation_type: "backup",
              },
              manuals: [
                {
                  manual: {
                    id: "manual-pg-backup-ubuntu",
                    title: "PostgreSQL 备份 Ubuntu 运维手册",
                  },
                  bound_workflow_id: "workflow-pg-backup-ubuntu",
                  usable_mode: "adapt",
                  matched_fields: ["object_type", "operation_type"],
                  environment_diffs: ["os", "package_manager"],
                  blocked_reasons: [
                    "workflow targets ubuntu apt/systemd but current host is centos/yum/systemd",
                  ],
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
    expect(container.textContent).toContain("操作系统；包管理器");
    expect(container.textContent).toContain(
      "workflow targets ubuntu apt/systemd but current host is centos/yum/systemd",
    );
    expect(
      Array.from(container.querySelectorAll("button")).some((button) =>
        button.textContent?.includes("生成适配工作流"),
      ),
    ).toBe(true);
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
              summary:
                "找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。",
              operation_frame: {
                object_type: "redis",
                operation_type: "rca_or_repair",
              },
              manuals: [
                {
                  manual: {
                    id: "manual-redis-local-readonly-rca",
                    title: "Redis 本机只读排障运维手册",
                  },
                  bound_workflow_id: "workflow-redis-local-readonly-rca",
                  usable_mode: "direct_execute",
                  matched_fields: [
                    "object_type",
                    "operation_type",
                    "execution_surface",
                    "required_context",
                  ],
                  recommended_action: "run_preflight_probe",
                  run_record_summary: {
                    success_count: 1,
                    failure_count: 0,
                    recent_result: "passed",
                  },
                },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("可进入预检");
    expect(container.textContent).toContain("Redis 本机只读排障运维手册");
    expect(container.textContent).toContain(
      "workflow-redis-local-readonly-rca",
    );
    expect(container.textContent).toContain(
      "下一步：AI 会先运行只读预检；通过并确认后再 Dry Run。",
    );
    expect(container.querySelectorAll("button")).toHaveLength(0);
    expect(container.textContent).not.toContain("Runner 已执行");
    expect(container.textContent).not.toMatch(/\d+\s*%/);
  });

  it("renders direct_execute search and preflight as one compact card when merged", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-direct-execute-merged",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "direct_execute",
              operation_frame: {
                object_type: "mysql",
                operation_type: "backup",
              },
              manuals: [
                {
                  manual: {
                    id: "manual-mysql-backup-ssh",
                    title: "MySQL SSH 备份运维手册",
                  },
                  bound_workflow_id: "workflow-mysql-backup-ssh",
                  usable_mode: "direct_execute",
                  recommended_action: "run_preflight_probe",
                },
              ],
              merged_preflight_result: {
                status: "passed",
                ready: true,
                manual_id: "manual-mysql-backup-ssh",
                workflow_id: "workflow-mysql-backup-ssh",
                probe_id: "check_mysql_backup_ssh_and_path",
                next_action: "start_dry_run",
                evidence: [
                  { name: "ssh_access", status: "passed" },
                  { name: "connection_test", status: "passed" },
                  { name: "backup_path_writable", status: "passed" },
                ],
              },
            },
          }}
        />,
      );
    });

    expect(
      container.querySelectorAll(
        '[data-testid="ops-manual-search-result-card"]',
      ),
    ).toHaveLength(1);
    expect(
      container.querySelectorAll('[data-testid="ops-manual-merged-preflight"]'),
    ).toHaveLength(1);
    expect(
      container.querySelectorAll(
        '[data-testid="ops-manual-preflight-result-card"]',
      ),
    ).toHaveLength(0);
    expect(container.textContent).toContain("可进入预检");
    expect(container.textContent).toContain("mysql / backup");
    expect(container.textContent).toContain("MySQL SSH 备份运维手册");
    expect(container.textContent).toContain("workflow-mysql-backup-ssh");
    expect(container.textContent).toContain("Workflow 预检");
    expect(container.textContent).toContain("预检通过");
    expect(container.textContent).toContain("ssh_access");
    expect(container.textContent).toContain("backup_path_writable");
    expect(container.textContent).toContain("进入 Dry Run");
    expect(container.textContent).not.toContain(
      "下一步：AI 会先运行只读预检；通过并确认后再 Dry Run。",
    );
  });

  it("requests Dry Run confirmation when clicking a passed merged preflight action", async () => {
    let detail: {
      action?: string;
      title?: string;
      sourceTitle?: string;
      artifactId?: string;
    } | null = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-confirmation", handler);

    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-direct-execute-merged-click",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "direct_execute",
              operation_frame: {
                object_type: "mysql",
                operation_type: "backup",
              },
              manuals: [
                {
                  manual: {
                    id: "manual-mysql-backup-ssh",
                    title: "MySQL SSH 备份运维手册",
                  },
                  bound_workflow_id: "workflow-mysql-backup-ssh",
                  usable_mode: "direct_execute",
                  recommended_action: "run_preflight_probe",
                },
              ],
              merged_preflight_result: {
                status: "passed",
                ready: true,
                manual_id: "manual-mysql-backup-ssh",
                workflow_id: "workflow-mysql-backup-ssh",
                next_action: "start_dry_run",
              },
            },
          }}
        />,
      );
    });

    const dryRunButton = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("进入 Dry Run"),
    );
    await act(async () => {
      dryRunButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(detail).toEqual({
      action: "start_runner_workflow_dry_run",
      title: "进入 Dry Run",
      sourceTitle: "MySQL SSH 备份运维手册",
      artifactId: "artifact-direct-execute-merged-click",
    });

    window.removeEventListener("aiops:composer-confirmation", handler);
  });

  it("keeps manual matching and workflow preflight status visually distinct", async () => {
    await act(async () => {
      root.render(
        <div>
          <OpsManualSearchResultArtifact
            artifact={{
              id: "artifact-need-info-distinct",
              type: "ops_manual_search_result",
              inlineData: {
                decision: "need_info",
                summary: "需要补充目标实例或服务。",
                manuals: [{ manual: { title: "Redis SSH 排障运维手册" } }],
              },
            }}
          />
          <OpsManualPreflightResultArtifact
            artifact={{
              id: "artifact-preflight-distinct",
              type: "ops_manual_preflight_result",
              inlineData: {
                status: "blocked",
                reason: "缺少只读探针权限。",
                missing_permissions: ["redis-readonly-probe"],
                next_action: "request_permission",
              },
            }}
          />
        </div>,
      );
    });

    const searchCard = container.querySelector(
      '[data-testid="ops-manual-search-result-card"]',
    ) as HTMLElement;
    const preflightCard = container.querySelector(
      '[data-testid="ops-manual-preflight-result-card"]',
    ) as HTMLElement;
    expect(searchCard.textContent).toContain("运维手册检索");
    expect(searchCard.textContent).not.toContain("手册缺上下文");
    expect(searchCard.textContent).toContain("暂未进入 Workflow 预检");
    expect(searchCard.textContent).not.toContain("Workflow 预检阻断");
    expect(preflightCard.textContent).toContain("Workflow 预检");
    expect(preflightCard.textContent).toContain("Workflow 预检阻断");
    expect(preflightCard.textContent).toContain("申请权限");
    expect(preflightCard.textContent).not.toContain("运维手册检索");
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
              runRecordSummary: {
                successCount: 7,
                failureCount: 0,
                recentResult: "success",
              },
              score: 0.97,
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("可进入预检");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.textContent).toContain("workflow-redis-memory");
    expect(container.textContent).toContain(
      "下一步：先运行预检，通过并确认后再进入 Dry Run。",
    );
    expect(container.querySelectorAll("button")).toHaveLength(0);
    expect(container.textContent).not.toMatch(/\d+\s*%/);
    expect(container.textContent).not.toContain("命中率");
  });

  it("renders ops manual preflight result with evidence and next action", async () => {
    await act(async () => {
      root.render(
        <OpsManualPreflightResultArtifact
          artifact={{
            id: "artifact-preflight",
            type: "ops_manual_preflight_result",
            inlineData: {
              status: "blocked",
              ready: false,
              manual_id: "manual-pg-backup",
              workflow_id: "workflow-pg-backup",
              probe_id: "pg-backup-readonly",
              reason: "preflight probe permission is missing",
              missing_permissions: ["pg-backup-readonly"],
              next_action: "request_permission",
              evidence: [{ name: "ssh_access", status: "passed", value: true }],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("Workflow 预检");
    expect(container.textContent).toContain("Workflow 预检阻断");
    expect(container.textContent).toContain(
      "Workflow 预检缺参数、权限或环境条件",
    );
    expect(container.textContent).toContain("manual-pg-backup");
    expect(container.textContent).toContain("workflow-pg-backup");
    expect(container.textContent).toContain("pg-backup-readonly");
    expect(container.textContent).toContain("ssh_access");
    expect(container.textContent).toContain("申请权限");
    expect(container.textContent).not.toMatch(/\d+\s*%/);
  });

  it("renders passed preflight result with Dry Run action", async () => {
    await act(async () => {
      root.render(
        <OpsManualPreflightResultArtifact
          artifact={{
            id: "artifact-preflight-passed",
            type: "ops_manual_preflight_result",
            inlineData: {
              status: "passed",
              ready: true,
              manual_id: "manual-redis-memory",
              workflow_id: "workflow-redis-memory",
              next_action: "start_dry_run",
              evidence: [
                { name: "metrics_available", status: "passed", value: true },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("预检通过");
    expect(container.textContent).toContain("可进入下一步");
    expect(container.textContent).toContain("进入 Dry Run");
  });

  it("shows immediate running feedback after clicking param resolution preflight", async () => {
    let detail: { text?: string; metadata?: Record<string, string> } | null =
      null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-submit", handler);

    await act(async () => {
      root.render(
        <OpsManualParamResolutionArtifact
          artifact={{
            id: "artifact-param-running-feedback",
            type: "ops_manual_param_resolution",
            inlineData: {
              status: "resolved",
              manual_id: "manual-redis-memory",
              workflow_id: "workflow-redis-memory",
              resolved_params: [
                {
                  id: "target_instance",
                  value: "docker:aiops-redis",
                  source: "docker",
                  confidence: 0.92,
                },
                {
                  id: "execution_surface",
                  value: "docker exec aiops-redis",
                  source: "docker",
                  confidence: 0.92,
                },
              ],
            },
          }}
        />,
      );
    });

    const button = Array.from(container.querySelectorAll("button")).find(
      (item) => item.textContent?.includes("运行预检"),
    );
    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.textContent).toContain("预检中");
    expect(container.textContent).toContain(
      "预检请求已提交，正在等待只读探针结果。",
    );
    expect((button as HTMLButtonElement | undefined)?.disabled).toBe(true);
    expect(detail?.metadata?.opsManualAction).toBe("run_ops_manual_preflight");

    window.removeEventListener("aiops:composer-context-submit", handler);
  });

  it("renders failed preflight result with fallback guide action", async () => {
    await act(async () => {
      root.render(
        <OpsManualPreflightResultArtifact
          artifact={{
            id: "artifact-preflight-failed",
            type: "ops_manual_preflight_result",
            inlineData: {
              status: "failed",
              ready: false,
              reason: "target instance is not reachable",
              next_action: "fallback_guide",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("预检失败");
    expect(container.textContent).toContain("target instance is not reachable");
    expect(container.textContent).toContain("查看降级步骤");
    expect(container.textContent).not.toContain("进入 Dry Run");
    expect(container.textContent).not.toContain("立即执行");
  });

  it("renders fallback guide steps for reference-only operations", async () => {
    await act(async () => {
      root.render(
        <OpsManualFallbackGuideArtifact
          artifact={{
            id: "artifact-fallback-guide",
            type: "ops_manual_fallback_guide",
            inlineData: {
              title: "PostgreSQL 备份参考步骤",
              reason: "没有可直接运行的工作流。",
              steps: [
                "确认目标实例和备份路径",
                "只读检查 pg_isready",
                "逐步生成备份命令并让用户确认",
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("PostgreSQL 备份参考步骤");
    expect(container.textContent).toContain("没有可直接运行的工作流。");
    expect(container.textContent).toContain("1. 确认目标实例和备份路径");
    expect(container.textContent).toContain("3. 逐步生成备份命令并让用户确认");
    expect(container.textContent).not.toContain("进入 Dry Run");
    expect(container.textContent).not.toContain("立即执行");
  });

  it("renders reference_only search result as reference guidance without execution entry", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-reference-only",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "reference_only",
              summary: "找到可参考手册，但不能直接执行绑定工作流。",
              manuals: [
                {
                  manual: {
                    id: "manual-pg-reference",
                    title: "PostgreSQL 备份参考手册",
                  },
                  usable_mode: "reference_only",
                },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("仅参考");
    expect(container.textContent).toContain("PostgreSQL 备份参考手册");
    expect(container.textContent).toContain("没有可直接运行的 Workflow");
    expect(container.textContent).toContain("AI 会继续自动只读排查");
    expect(container.textContent).toContain("先让你补齐必要信息");
    expect(container.textContent).toContain("参考关系");
    expect(container.querySelectorAll("button")).toHaveLength(0);
    expect(container.textContent).not.toContain("按步骤执行");
    expect(container.textContent).not.toContain("运行预检");
    expect(container.textContent).not.toContain("进入 Dry Run");
    expect(container.textContent).not.toContain("立即执行");
    expect(container.textContent).not.toContain("继续普通排查");
  });

  it("hides stale cross-object reference_only manual hits for Kafka", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-reference-object-diff",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "reference_only",
              operation_frame: {
                object_type: "kafka",
                operation_type: "rca_or_repair",
              },
              manuals: [
                {
                  manual: {
                    id: "manual-k8s-pod-crashloop-rca",
                    title: "Kubernetes Pod CrashLoop/OOM 排障运维手册",
                    operation: {
                      target_type: "kubernetes_pod",
                      action: "rca_or_repair",
                    },
                  },
                  bound_workflow_id: "workflow-k8s-pod-crashloop-rca",
                  usable_mode: "reference_only",
                  blocked_reasons: ["object_type differs"],
                },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain(
      "未找到适用手册，AI 将继续只读排查",
    );
    expect(container.textContent).toContain(
      "没有找到适用于 Kafka 的可用运维手册。",
    );
    expect(container.textContent).toContain("AI 不使用不匹配的手册");
    expect(container.textContent).not.toContain("manual-k8s-pod-crashloop-rca");
    expect(container.textContent).not.toContain("Kubernetes Pod CrashLoop/OOM");
    expect(container.textContent).not.toContain("对象类型不匹配");
    expect(container.textContent).not.toContain("object_type differs");
  });

  it("hides cross-object manuals from Kafka no-match results", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-kafka-no-match",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "no_match",
              summary: "没有找到适用于 Kafka 的可用运维手册。",
              operation_frame: {
                object_type: "kafka",
                operation_type: "rca_or_repair",
              },
              recommended_next_action:
                "AI 会继续自动尝试只读排查；如果缺少目标、时间范围、权限或观测数据，会先让你补齐必要信息。",
              manuals: [],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain(
      "未找到适用手册，AI 将继续只读排查",
    );
    expect(container.textContent).toContain("AI 会继续自动尝试只读排查");
    expect(container.textContent).not.toContain("请在底部补充");
    expect(container.textContent).not.toContain("打开补充表单");
    expect(
      container.querySelector('[data-testid="ops-manual-context-prompt"]'),
    ).toBeNull();
    expect(container.textContent).not.toContain("manual-k8s-pod-crashloop-rca");
    expect(container.textContent).not.toContain("Kubernetes Pod CrashLoop/OOM");
    expect(container.textContent).not.toContain("object_type differs");
  });

  it("rewrites stale no_match next action to read-only investigation guidance", async () => {
    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-no-match-stale-next",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "no_match",
              summary: "没有找到合适的运维手册。",
              recommended_next_action: "继续普通 Agent 运维流程。",
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("AI 不使用不匹配的手册");
    expect(container.textContent).toContain("会先让你补齐必要信息");
    expect(container.textContent).not.toContain("继续普通 Agent 运维流程");
  });

  it("does not dispatch a fixed context request from search need_info even when legacy required fields exist", async () => {
    let detail: unknown = null;
    const handler = (event: Event) => {
      detail = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", handler);

    await act(async () => {
      root.render(
        <OpsManualSearchResultArtifact
          artifact={{
            id: "artifact-kafka-context-form",
            type: "ops_manual_search_result",
            inlineData: {
              decision: "need_info",
              operation_frame: {
                object_type: "kafka",
                operation_type: "rca_or_repair",
              },
              next_questions: [
                "Kafka 集群/环境名",
                "时间范围",
                "consumer group",
              ],
              required_context_fields: [
                {
                  id: "target_location",
                  label: "Kafka 集群/环境名",
                  placeholder: "prod-kafka",
                },
                {
                  id: "time_range",
                  label: "时间范围",
                  placeholder: "最近 30 分钟",
                },
                {
                  id: "consumer_group",
                  label: "Consumer Group",
                  placeholder: "checkout-group",
                },
              ],
            },
          }}
        />,
      );
    });
    await act(flushTimers);

    expect(detail).toBeNull();
    expect(container.textContent).toContain("运维手册检索");
    expect(container.textContent).toContain("暂未进入 Workflow 预检");
    expect(container.textContent).not.toContain("Kafka 集群/环境名");
    expect(container.textContent).not.toContain("Consumer Group");

    window.removeEventListener("aiops:composer-context-request", handler);
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
                  manual: {
                    id: "manual-pg-backup-ubuntu",
                    title: "PostgreSQL 备份 Ubuntu 运维手册",
                  },
                  usable_mode: "adapt",
                  recommended_action: "generate_workflow_variant",
                },
              ],
            },
          }}
        />,
      );
    });

    const button = Array.from(container.querySelectorAll("button")).find(
      (item) => item.textContent?.includes("生成适配工作流"),
    );
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
                {
                  id: "extract",
                  title: "提取参数",
                  status: "passed",
                  redactedLog: "host=redis-prod-01",
                },
                {
                  id: "build",
                  title: "生成节点",
                  status: "running",
                  redactedLog: "secret_ref=***",
                },
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
            summaryZh:
              "找到相似运维手册，但当前环境存在差异，需要先生成变体并校验。",
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

    expect(container.textContent).toContain("手册缺上下文");
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

    expect(container.textContent).toContain("运维手册检索");
    expect(container.textContent).not.toContain("手册缺上下文");
    expect(container.textContent).toContain("暂未进入 Workflow 预检");
    expect(container.textContent).not.toContain("补充上下文");
    expect(container.textContent).not.toContain("目标 Redis 实例是哪一个？");
    expect(container.textContent).not.toContain("工具返回的信息不足判定。");
    expect(container.textContent).not.toContain("来源：");
    expect(container.textContent).not.toContain("生成时间：");
    expect(container.textContent).not.toContain("暂不支持的卡片类型");
  });

  it("registers preflight artifacts in the generic Agent-to-UI dispatcher", async () => {
    await act(async () => {
      root.render(
        <AgentUiArtifactPart
          artifact={{
            id: "artifact-ops-manual-preflight",
            type: "ops_manual_preflight_result",
            titleZh: "运维手册预检",
            summaryZh: "预检已通过。",
            source: "run_ops_manual_preflight",
            createdAt: "2026-05-15T09:30:00+08:00",
            inlineData: {
              status: "passed",
              ready: true,
              manual_id: "manual-redis-memory",
              workflow_id: "workflow-redis-memory",
              next_action: "start_dry_run",
              evidence: [
                { name: "metrics_available", status: "passed", value: true },
              ],
            },
          }}
        />,
      );
    });

    expect(container.textContent).toContain("预检通过");
    expect(container.textContent).toContain("manual-redis-memory");
    expect(container.textContent).toContain("进入 Dry Run");
    expect(container.textContent).not.toContain("运维手册预检");
    expect(container.textContent).not.toContain("预检已通过。");
    expect(container.textContent).not.toContain("来源：");
    expect(container.textContent).not.toContain("暂不支持的卡片类型");
  });

  it("renders merged search and parameter resolution as a single progress card", async () => {
    let contextRequest: {
      artifactId?: string;
      fields?: Array<{ id: string }>;
    } | null = null;
    const requestHandler = (event: Event) => {
      contextRequest = (event as CustomEvent).detail;
    };
    window.addEventListener("aiops:composer-context-request", requestHandler);
    const artifact: AiopsTransportAgentUiArtifact = {
      id: "artifact-param-pg",
      type: "ops_manual_search_result",
      inlineData: {
        decision: "need_info",
        original_search_artifact_id: "artifact-search-pg",
        operation_frame: {
          object_type: "postgresql",
          operation_type: "backup",
        },
        manuals: [
          {
            manual: {
              id: "manual-pg-backup-ubuntu",
              title: "PostgreSQL 备份 Ubuntu 运维手册",
            },
            bound_workflow_id: "workflow-pg-backup-ubuntu",
            matched_fields: ["object_type", "operation_type"],
          },
        ],
        merged_param_resolution: {
          artifact_id: "artifact-param-pg",
          status: "need_user_input",
          manual_id: "manual-pg-backup-ubuntu",
          workflow_id: "workflow-pg-backup-ubuntu",
          resolved_params: [
            {
              id: "target_host",
              value: "server-local",
              source: "user",
              evidence: "context fact: target_host",
            },
            {
              id: "target_instance",
              value: "docker:aiops-postgres",
              source: "docker",
              evidence: "docker ps: image=pgvector/pgvector:pg16",
            },
          ],
          fields: [
            {
              id: "backup_path",
              label: "备份路径",
              type: "path",
              required: true,
              ui_control: "text",
              placeholder: "例如 /data/backups",
            },
          ],
        },
      },
    };

    await act(async () => {
      root.render(<AgentUiArtifactPart artifact={artifact} />);
    });
    await act(flushTimers);

    expect(
      container.querySelector('[data-testid="ops-manual-progress-card"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="ops-manual-search-result-card"]'),
    ).toBeNull();
    expect(
      container.querySelector(
        '[data-testid="ops-manual-param-resolution-card"]',
      ),
    ).toBeNull();
    expect(container.textContent).not.toContain("等待备份路径");
    expect(container.textContent).toContain("PostgreSQL 备份 Ubuntu 运维手册");
    expect(container.textContent).toContain("目标主机");
    expect(container.textContent).toContain("server-local");
    expect(container.textContent).not.toContain("手册命中");
    expect(container.textContent).not.toContain("参数解析");
    expect(container.textContent).not.toContain("Workflow 预检");
    expect(container.textContent).not.toContain("备份路径");
    expect(container.textContent).not.toContain("底部表单正在等待");
    expect(container.textContent).not.toContain(
      "运维手册暂未进入 Workflow 预检",
    );
    expect(container.textContent).not.toContain("需要补充参数");
    expect(contextRequest).toMatchObject({
      artifactId: "artifact-param-pg",
      fields: [expect.objectContaining({ id: "backup_path" })],
    });
    window.removeEventListener(
      "aiops:composer-context-request",
      requestHandler,
    );
  });
});

function flushTimers() {
  return new Promise((resolve) => window.setTimeout(resolve, 0));
}
