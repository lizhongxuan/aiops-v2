import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  getChildAgentTranscript,
  submitHostOpsApprovalDecision,
} from "@/api/hostOps";
import { ChatPage } from "./ChatPage";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import {
  resetAiopsTransportStateCacheForTest,
  setCachedAiopsTransportState,
} from "@/transport/aiopsTransportStateCache";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

vi.mock("@/api/hostOps", () => ({
  getChildAgentTranscript: vi.fn(),
  submitHostOpsApprovalDecision: vi.fn(),
}));

describe("ChatPage", () => {
  let container: HTMLDivElement;
  let root: Root;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    HTMLElement.prototype.scrollTo = function scrollTo() {};
    vi.mocked(getChildAgentTranscript).mockReset();
    vi.mocked(submitHostOpsApprovalDecision).mockReset();
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
    resetAiopsTransportStateCacheForTest();
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    window.localStorage.clear();
    resetAiopsTransportStateCacheForTest();
    container.remove();
  });

  it("renders cached chat state immediately when returning to the chat page", async () => {
    const cached = sampleState();
    cached.sessionId = "cached-session";
    cached.threadId = "cached-session";
    setCachedAiopsTransportState("single_host", cached);
    const fetchSpy = vi
      .spyOn(globalThis, "fetch")
      .mockImplementation(async (input) => {
        const url = String(input);
        if (url.includes("/api/v1/assistant/resume")) {
          return new Response("aui-state:[]\n", {
            status: 200,
            headers: { "Content-Type": "text/plain" },
          });
        }
        if (url.includes("/api/v1/sessions")) {
          return new Response(
            JSON.stringify({
              activeSessionId: "cached-session",
              sessions: [
                {
                  id: "cached-session",
                  kind: "single_host",
                  selectedHostId: "server-local",
                  status: "working",
                  messageCount: 1,
                  title: "Cached chat",
                },
              ],
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          );
        }
        if (url.includes("/api/v1/llm-config")) {
          return new Response(
            JSON.stringify({
              provider: "openai",
              model: "gpt-5",
              apiKeySet: true,
            }),
            {
              status: 200,
              headers: { "Content-Type": "application/json" },
            },
          );
        }
        return new Response(JSON.stringify({ items: [] }), {
          status: 200,
          headers: { "Content-Type": "application/json" },
        });
      });

    await act(async () => {
      root.render(<ChatPage />);
    });

    expect(container.textContent).toContain(
      "Investigate payment-api saturation",
    );
    expect(container.textContent).toContain(
      "payment-api is waiting for rollout approval.",
    );
    expect(container.textContent).not.toContain("Hello there");
    fetchSpy.mockRestore();
  });

  it("renders assistant-ui chat state with typed process and approval blocks", async () => {
    await act(async () => {
      root.render(<ChatPage initialState={sampleState()} />);
    });

    expect(container.textContent).toContain(
      "Investigate payment-api saturation",
    );
    expect(container.textContent).toContain(
      "kubectl rollout status deploy/payment-api",
    );
    expect(container.textContent).toContain(
      "payment-api is waiting for rollout approval.",
    );
    expect(container.textContent).toContain("等待审批");
    expect(container.textContent).toContain("要执行这个命令，需要你确认吗？");
    expect(container.textContent).toContain("1. 是");
    expect(container.textContent).toContain(
      "2. 是，且对于以后类似命令不再询问",
    );
    expect(container.textContent).toContain("3. 否，请告知 AIOps 如何调整");
    expect(container.textContent).toContain("提交");
    expect(
      container.querySelector('[data-testid="codex-approval-inline"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('[data-testid="codex-approval-command"]'),
    ).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();
  });

  it("renders the chat ops run summary above the composer when transport state provides one", async () => {
    const state = sampleState();
    state.opsRun = {
      id: "opsrun-turn-1",
      source: "chat",
      status: "working",
      title: "主机A跟主机B上PG不同步",
      targetSummary: "主机A/主机B PG 与主机C pg_mon",
      evidenceCount: 2,
      currentStep: "正在只读采集 PG 同步证据",
    };

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(
      container.querySelector('[data-testid="ops-run-summary-card"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain("主机A跟主机B上PG不同步");
    expect(container.textContent).toContain("主机A/主机B PG 与主机C pg_mon");
    expect(container.textContent).toContain("2 条证据");
  });

  it("renders Agent-to-UI artifacts inside assistant messages", async () => {
    const state = sampleState();
    state.status = "idle";
    state.pendingApprovals = {};
    state.runtimeLiveness = {
      ...state.runtimeLiveness,
      activeTurns: {},
      pendingApprovals: {},
    };
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      status: "completed",
      completedAt: "2026-05-06T00:00:05Z",
      final: {
        id: "final-1",
        text: "payment-api is healthy after restart.",
        status: "completed",
      },
    };
    state.turns["turn-1"].agentUiArtifacts = [
      {
        id: "artifact-coroot-latency",
        type: "coroot_chart",
        titleZh: "Coroot 延迟趋势",
        summaryZh: "接口 P95 延迟在 14:03 后明显升高。",
        caseId: "case-debug-1",
        source: "coroot",
        redactionStatus: "redacted",
        inlineData: {
          mcpCard: {
            uiKind: "readonly_chart",
            title: "指标趋势",
            visual: {
              kind: "timeseries",
              series: [
                {
                  name: "p95_latency_ms",
                  data: [{ timestamp: 1, value: 980 }],
                },
              ],
            },
          },
        },
      },
    ];

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(container.textContent).toContain("Coroot 延迟趋势");
    expect(container.textContent).not.toContain(
      "接口 P95 延迟在 14:03 后明显升高。",
    );
    expect(container.textContent).toContain("p95_latency_ms");
    expect(
      container.querySelector('a[href="/incidents/case-debug-1"]'),
    ).toBeNull();
    expect(container.textContent).not.toContain("来源：coroot");
  });

  it("places a completed Coroot chart after root cause and before evidence", async () => {
    const state = sampleState();
    state.status = "idle";
    state.pendingApprovals = {};
    state.runtimeLiveness = {
      ...state.runtimeLiveness,
      activeTurns: {},
      pendingApprovals: {},
    };
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      status: "completed",
      completedAt: "2026-05-06T00:00:05Z",
      process: [],
      final: {
        id: "final-1",
        status: "completed",
        text: [
          "根因：外部依赖 external:18090 unknown。",
          "",
          "证据：",
          "- Coroot RCA 查询成功",
        ].join("\n"),
      },
      agentUiArtifacts: [
        {
          id: "artifact-coroot-net",
          type: "coroot_chart",
          titleZh: "aiops-host-agent 服务",
          inlineData: {
            mcpCard: {
              uiKind: "readonly_chart",
              title: "指标趋势",
              visual: {
                kind: "timeseries",
                series: [
                  {
                    name: "failed_tcp_connections",
                    data: [{ timestamp: 1, value: 0.33 }],
                  },
                ],
              },
            },
          },
        },
      ],
    };

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    const text = container.textContent || "";
    expect(text.indexOf("外部依赖 external:18090 unknown")).toBeLessThan(
      text.indexOf("aiops-host-agent 服务"),
    );
    expect(text.indexOf("aiops-host-agent 服务")).toBeLessThan(
      text.indexOf("Coroot RCA 查询成功"),
    );
  });

  it("shows a lightweight Coroot chart notice while the assistant is still running", async () => {
    const state = sampleState();
    state.status = "working";
    state.pendingApprovals = {};
    state.runtimeLiveness = {
      ...state.runtimeLiveness,
      activeTurns: { "turn-1": true },
      pendingApprovals: {},
    };
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      status: "working",
      process: [],
      final: {
        id: "final-1",
        status: "running",
        text: "我先读取 Coroot 指标并继续分析。",
      },
      agentUiArtifacts: [
        {
          id: "artifact-coroot-latency",
          type: "coroot_chart",
          titleZh: "aiops-host-agent 服务",
          inlineData: {
            mcpCard: {
              uiKind: "readonly_chart",
              title: "指标趋势",
              visual: {
                kind: "timeseries",
                series: [
                  {
                    name: "p95_latency_ms",
                    data: [{ timestamp: 1, value: 980 }],
                  },
                ],
              },
            },
          },
        },
      ],
    };

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(container.textContent).toContain("我先读取 Coroot 指标并继续分析。");
    expect(container.textContent).toContain(
      "已生成 Coroot 图表，分析完成后展开",
    );
    expect(container.textContent).not.toContain("aiops-host-agent 服务");
    expect(container.textContent).not.toContain("p95_latency_ms");
  });

  it("shows context compaction status from assistant transport metadata", async () => {
    const state = sampleState();
    state.pendingApprovals = {};
    state.runtimeLiveness = {
      ...state.runtimeLiveness,
      pendingApprovals: {},
    };
    state.turns["turn-1"] = {
      ...state.turns["turn-1"],
      status: "working",
      contextGovernance: [
        {
          id: "ctxgov-chat-l4",
          layer: "L4",
          kind: "context.compaction.started",
          message: "正在压缩上下文，当前任务会继续",
        },
        {
          id: "ctxgov-chat-l5",
          layer: "L5",
          kind: "context.compaction.failed",
          message: "上下文过长，已使用本地摘要继续",
        },
      ],
    };

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(container.textContent).toContain("上下文过长，已使用本地摘要继续");
    expect(container.textContent).not.toContain("正在重试压缩");
  });

  it("uses the current turn approval when stale approvals remain in transport state", async () => {
    const state = sampleState();
    state.pendingApprovals = {
      "stale-approval": {
        id: "stale-approval",
        turnId: "old-turn",
        type: "command",
        status: "blocked",
        command: "stale command should not render",
        requestedAt: "2026-05-06T00:00:00Z",
      },
      ...state.pendingApprovals,
    };

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    const command = container.querySelector(
      '[data-testid="codex-approval-command"]',
    );
    expect(command?.textContent).toContain(
      "kubectl rollout restart deploy/payment-api",
    );
    expect(command?.textContent).not.toContain(
      "stale command should not render",
    );
  });

  it("replaces the composer with approval options whenever a current turn approval is pending", async () => {
    const state = sampleState();
    state.status = "working";

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(container.textContent).toContain("等待审批");
    expect(
      container.querySelector('[data-testid="codex-approval-inline"]'),
    ).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();
  });

  it("renders the host ops status panel above the composer when a host mission is active", async () => {
    const state = sampleStateWithHostOps();
    state.status = "idle";
    state.pendingApprovals = {};
    state.runtimeLiveness.pendingApprovals = {};

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    const panel = container.querySelector(
      '[data-testid="host-ops-status-panel"]',
    );
    const composer = container.querySelector(
      '[data-testid="aiops-composer-shell"]',
    );

    expect(panel).not.toBeNull();
    expect(composer).not.toBeNull();
    expect(panel?.compareDocumentPosition(composer as Node)).toBe(
      Node.DOCUMENT_POSITION_FOLLOWING,
    );
    expect(container.textContent).toContain("共 3 个主机 Agent");
  });

  it("opens a child agent transcript drawer from the host ops status row", async () => {
    const state = sampleStateWithHostOps();
    state.status = "idle";
    state.pendingApprovals = {};
    state.runtimeLiveness.pendingApprovals = {};
    vi.mocked(getChildAgentTranscript).mockResolvedValue({
      childAgentId: "child-1",
      items: [
        {
          id: "item-manager",
          type: "manager_message",
          content: "初始化 Franklin 上的主库",
          createdAt: "2026-06-04T01:00:00Z",
        },
        {
          id: "item-assistant",
          type: "assistant_message",
          content: "已连接主机并准备执行 pg_isready。",
          createdAt: "2026-06-04T01:01:00Z",
        },
        {
          id: "item-tool",
          type: "tool_call",
          toolName: "shell",
          content: "pg_isready -h 127.0.0.1",
          createdAt: "2026-06-04T01:02:00Z",
        },
      ],
    });

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    const open = container.querySelector(
      '[data-testid="host-child-agent-name-child-1"]',
    ) as HTMLButtonElement | null;

    await act(async () => {
      open?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(getChildAgentTranscript).toHaveBeenCalledWith("child-1");
    expect(
      document.body.querySelector('[data-testid="host-subagent-drawer"]'),
    ).not.toBeNull();
    expect(document.body.textContent).toContain("Franklin");
    expect(document.body.textContent).toContain("初始化 Franklin 上的主库");
    const toolsTab = Array.from(document.body.querySelectorAll("button")).find(
      (button) => button.textContent?.trim().includes("工具"),
    );
    await act(async () => {
      toolsTab?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(document.body.textContent).toContain("pg_isready -h 127.0.0.1");
  });

  it("opens the child agent drawer when the transport map key differs from the runtime child agent id", async () => {
    const state = sampleStateWithHostOps();
    state.status = "idle";
    state.pendingApprovals = {};
    state.runtimeLiveness.pendingApprovals = {};
    const keyedChild = state.childAgents?.["child-1"];
    if (!keyedChild || !state.hostMissions?.["mission-1"]) {
      throw new Error("sample host ops state is missing child agent data");
    }
    state.hostMissions["mission-1"].childAgentIds = ["transport-key-1"];
    state.childAgents = {
      "transport-key-1": {
        ...keyedChild,
        id: "runtime-child-1",
      },
    };
    vi.mocked(getChildAgentTranscript).mockResolvedValue({
      childAgentId: "runtime-child-1",
      items: [
        {
          id: "item-manager",
          type: "manager_message",
          content: "检查 runtime-child-1",
        },
      ],
    });

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    const open = container.querySelector(
      '[data-testid="host-child-agent-name-runtime-child-1"]',
    ) as HTMLButtonElement | null;
    await act(async () => {
      open?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    await flushMicrotasks();

    expect(getChildAgentTranscript).toHaveBeenCalledWith("runtime-child-1");
    expect(
      document.body.querySelector('[data-testid="host-subagent-drawer"]'),
    ).not.toBeNull();
    expect(document.body.textContent).toContain("检查 runtime-child-1");
  });

  it("shows immediate feedback after submitting an approval decision", async () => {
    await act(async () => {
      root.render(<ChatPage initialState={sampleState()} />);
    });

    const submit = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("提交"),
    ) as HTMLButtonElement | undefined;

    await act(async () => {
      submit?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.textContent).toContain("已提交确认，正在继续执行");
    expect(submit?.textContent).toContain("提交中");
    expect(submit?.disabled).toBe(true);
  });

  it("replaces the textarea with an ops manual generation confirmation panel", async () => {
    const state = createInitialAiopsTransportState("thread-confirmation");
    state.sessionId = "sess-confirmation";

    await act(async () => {
      root.render(
        <ChatPage initialState={state} threadId="thread-confirmation" />,
      );
    });

    expect(container.querySelector("textarea")).not.toBeNull();

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("aiops:composer-confirmation", {
          detail: {
            action: "generate_ops_manual_candidate",
            title: "生成运维手册候选",
            sourceTitle: "Redis 内存压力排障",
            artifactId: "artifact-generate-manual",
          },
        }),
      );
    });

    expect(
      container.querySelector(
        '[data-testid="ops-manual-generation-confirmation"]',
      ),
    ).not.toBeNull();
    expect(container.textContent).toContain("生成运维手册候选");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.querySelector("textarea")).toBeNull();

    const cancel = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("取消"),
    );
    await act(async () => {
      cancel?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(
      container.querySelector(
        '[data-testid="ops-manual-generation-confirmation"]',
      ),
    ).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();
  });

  it("does not automatically offer ops manual generation from a completed AI Chat operation", async () => {
    const state = createInitialAiopsTransportState("thread-manual-from-chat");
    state.sessionId = "sess-manual-from-chat";
    state.status = "idle";
    state.currentTurnId = "turn-manual-from-chat";
    state.turnOrder = ["turn-manual-from-chat"];
    state.turns = {
      "turn-manual-from-chat": {
        id: "turn-manual-from-chat",
        status: "completed",
        startedAt: "2026-05-15T01:00:00Z",
        completedAt: "2026-05-15T01:00:18Z",
        user: {
          id: "user-manual-from-chat",
          text: "排查 Redis 内存和 p95 升高",
          createdAt: "2026-05-15T01:00:00Z",
        },
        process: [
          {
            id: "cmd-manual-from-chat-1",
            kind: "command",
            status: "completed",
            text: "docker ps --filter name=redis",
            command: "docker ps --filter name=redis",
            outputPreview: "aiops-redis",
          },
          {
            id: "cmd-manual-from-chat-2",
            kind: "command",
            status: "completed",
            text: "docker exec aiops-redis redis-cli INFO memory",
            command: "docker exec aiops-redis redis-cli INFO memory",
            outputPreview: "used_memory_rss:123456",
          },
        ],
        final: {
          id: "final-manual-from-chat",
          status: "completed",
          text: "本次验证状态：已验证，结论基于当前主机与 Redis 容器实时只读结果；未执行任何变更操作。",
        },
      },
    };

    await act(async () => {
      root.render(
        <ChatPage initialState={state} threadId="thread-manual-from-chat" />,
      );
    });

    expect(container.querySelector("textarea")).not.toBeNull();
    const generate = container.querySelector(
      '[data-testid="aiops-generate-ops-manual-from-chat"]',
    );
    expect(generate).toBeNull();
    expect(
      container.querySelector(
        '[data-testid="ops-manual-generation-confirmation"]',
      ),
    ).toBeNull();
    expect(container.textContent).not.toContain("本次对话可沉淀为运维手册");
  });

  it("does not offer ops manual generation after a normal read-only status check", async () => {
    const state = createInitialAiopsTransportState("thread-redis-status-check");
    state.sessionId = "sess-redis-status-check";
    state.status = "idle";
    state.currentTurnId = "turn-redis-status-check";
    state.turnOrder = ["turn-redis-status-check"];
    state.turns = {
      "turn-redis-status-check": {
        id: "turn-redis-status-check",
        status: "completed",
        startedAt: "2026-05-17T01:00:00Z",
        completedAt: "2026-05-17T01:00:12Z",
        user: {
          id: "user-redis-status-check",
          text: "检查 Redis 状态",
          createdAt: "2026-05-17T01:00:00Z",
        },
        intent: { text: "检查 Redis 状态", status: "status_check" },
        process: [
          {
            id: "cmd-redis-status-1",
            kind: "command",
            status: "completed",
            text: "docker ps --filter name=redis",
            command: "docker ps --filter name=redis",
            outputPreview: "aiops-redis Up 3 hours",
          },
          {
            id: "cmd-redis-status-2",
            kind: "command",
            status: "completed",
            text: "docker exec aiops-redis redis-cli ping",
            command: "docker exec aiops-redis redis-cli ping",
            outputPreview: "PONG",
          },
        ],
        final: {
          id: "final-redis-status-check",
          status: "completed",
          text: "Redis 当前状态正常，docker 容器运行中且 PING 返回 PONG；本次只读检查未执行任何变更。",
        },
      },
    };

    await act(async () => {
      root.render(
        <ChatPage initialState={state} threadId="thread-redis-status-check" />,
      );
    });

    expect(container.textContent).toContain("Redis 当前状态正常");
    expect(
      container.querySelector(
        '[data-testid="aiops-generate-ops-manual-from-chat"]',
      ),
    ).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();
  });

  it("does not offer ops manual generation while ops manual parameters still need confirmation", async () => {
    const state = createInitialAiopsTransportState(
      "thread-pg-param-confirmation",
    );
    state.sessionId = "sess-pg-param-confirmation";
    state.status = "idle";
    state.currentTurnId = "turn-pg-param-confirmation";
    state.turnOrder = ["turn-pg-param-confirmation"];
    state.turns = {
      "turn-pg-param-confirmation": {
        id: "turn-pg-param-confirmation",
        status: "completed",
        startedAt: "2026-05-17T01:00:00Z",
        completedAt: "2026-05-17T01:00:12Z",
        user: {
          id: "user-pg-param-confirmation",
          text: "请按运维手册给本机 PostgreSQL 做备份",
          createdAt: "2026-05-17T01:00:00Z",
        },
        process: [
          {
            id: "tool-pg-param-resolution",
            kind: "tool",
            status: "completed",
            text: "resolve_ops_manual_params",
            outputPreview: "target_instance ambiguous, backup_path missing",
          },
        ],
        agentUiArtifacts: [
          {
            id: "artifact-pg-param-resolution",
            type: "ops_manual_param_resolution",
            status: "ambiguous",
            inlineData: {
              status: "ambiguous",
              fields: [
                {
                  id: "target_instance",
                  label: "实例/服务",
                  candidates: [
                    { value: "docker:aiops-postgres" },
                    { value: "docker:aiops-pgvector" },
                  ],
                },
                { id: "backup_path", label: "备份路径" },
              ],
            },
          },
        ],
        final: {
          id: "final-pg-param-confirmation",
          status: "completed",
          text: "还需要确认 PostgreSQL 目标实例和备份路径；目前还没有执行任何变更，也还未进入预检。",
        },
      },
    };

    await act(async () => {
      root.render(
        <ChatPage
          initialState={state}
          threadId="thread-pg-param-confirmation"
        />,
      );
    });

    expect(container.textContent).toContain(
      "还需要确认 PostgreSQL 目标实例和备份路径",
    );
    expect(
      container.querySelector(
        '[data-testid="aiops-generate-ops-manual-from-chat"]',
      ),
    ).toBeNull();
    expect(container.textContent).toContain("需要确认参数");
  });

  it("replaces the textarea with an execution confirmation panel", async () => {
    const state = createInitialAiopsTransportState(
      "thread-dry-run-confirmation",
    );
    state.sessionId = "sess-dry-run-confirmation";

    await act(async () => {
      root.render(
        <ChatPage
          initialState={state}
          threadId="thread-dry-run-confirmation"
        />,
      );
    });

    expect(container.querySelector("textarea")).not.toBeNull();

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("aiops:composer-confirmation", {
          detail: {
            action: "confirm_runner_workflow_execution",
            title: "确认执行",
            sourceTitle: "MySQL SSH 备份运维手册",
            artifactId: "artifact-preflight-passed",
          },
        }),
      );
    });

    expect(
      container.querySelector(
        '[data-testid="ops-manual-generation-confirmation"]',
      ),
    ).not.toBeNull();
    expect(container.textContent).toContain("确认执行");
    expect(container.textContent).toContain("MySQL SSH 备份运维手册");
    expect(container.textContent).toContain("确认执行绑定 Workflow");
    expect(container.querySelector("textarea")).toBeNull();

    const cancel = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("取消"),
    );
    await act(async () => {
      cancel?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(
      container.querySelector(
        '[data-testid="ops-manual-generation-confirmation"]',
      ),
    ).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();
  });

  it("replaces the textarea with a dynamic ops manual parameter form", async () => {
    const state = createInitialAiopsTransportState("thread-context-form");
    state.sessionId = "sess-context-form";

    await act(async () => {
      root.render(
        <ChatPage initialState={state} threadId="thread-context-form" />,
      );
    });

    expect(container.querySelector("textarea")).not.toBeNull();

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("aiops:composer-context-request", {
          detail: {
            artifactId: "artifact-param-resolution",
            title: "补充运维手册参数",
            manualId: "manual-redis-rca-ssh",
            workflowId: "workflow-redis-rca-ssh",
            submitAction: "submit_ops_manual_param_form",
            fields: [
              {
                id: "redis_instance",
                label: "Redis 实例",
                type: "resource_ref",
                uiControl: "select",
                required: true,
                candidates: [
                  { value: "docker:redis-1", label: "redis-1" },
                  { value: "docker:redis-2", label: "redis-2" },
                ],
              },
            ],
          },
        }),
      );
    });

    expect(
      container.querySelector('[data-testid="ops-manual-context-composer"]'),
    ).not.toBeNull();
    expect(container.textContent).toContain("补充运维手册参数");
    expect(container.textContent).not.toContain("运维手册缺信息");
    expect(container.textContent).not.toContain("只补必要字段");
    expect(container.textContent).toContain("Redis 实例");
    expect(container.textContent).toContain("redis-1");
    expect(container.textContent).toContain("redis-2");
    expect(container.textContent).not.toContain("目标位置");
    expect(container.textContent).not.toContain("访问/执行入口");
    expect(container.textContent).not.toContain("现象/证据");

    const instanceSelect = container.querySelector(
      'select[name="redis_instance"]',
    ) as HTMLSelectElement | null;
    expect(instanceSelect).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();

    const cancel = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("取消"),
    );
    await act(async () => {
      cancel?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(
      container.querySelector('[data-testid="ops-manual-context-composer"]'),
    ).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();
  });

  it("keeps a canceled dynamic ops manual parameter form dismissed after remount", async () => {
    const requestDetail = {
      artifactId: "artifact-param-resolution-cancel",
      force: true,
      title: "补充运维手册参数",
      manualId: "manual-pg-backup",
      workflowId: "workflow-pg-backup",
      submitAction: "submit_ops_manual_param_form",
      fields: [
        {
          id: "backup_path",
          label: "备份路径",
          type: "path",
          uiControl: "text",
          required: true,
          placeholder: "例如 /data/backups",
        },
      ],
    };

    await act(async () => {
      root.render(
        <ChatPage
          initialState={createInitialAiopsTransportState(
            "thread-context-form-cancel",
          )}
          threadId="thread-context-form-cancel"
        />,
      );
    });
    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("aiops:composer-context-request", {
          detail: requestDetail,
        }),
      );
    });

    expect(
      container.querySelector('[data-testid="ops-manual-context-composer"]'),
    ).not.toBeNull();

    const cancel = Array.from(container.querySelectorAll("button")).find(
      (button) => button.textContent?.includes("取消"),
    );
    await act(async () => {
      cancel?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(
      container.querySelector('[data-testid="ops-manual-context-composer"]'),
    ).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();

    await act(async () => {
      root.unmount();
    });
    root = createRoot(container);
    await act(async () => {
      root.render(
        <ChatPage
          initialState={createInitialAiopsTransportState(
            "thread-context-form-cancel-reloaded",
          )}
          threadId="thread-context-form-cancel-reloaded"
        />,
      );
    });
    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("aiops:composer-context-request", {
          detail: requestDetail,
        }),
      );
    });

    expect(
      container.querySelector('[data-testid="ops-manual-context-composer"]'),
    ).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();
  });

  it("submits the dynamic ops manual parameter form as structured metadata", async () => {
    const state = createInitialAiopsTransportState(
      "thread-context-form-submit",
    );
    state.sessionId = "sess-context-form-submit";
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("", {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    try {
      await act(async () => {
        root.render(
          <ChatPage
            initialState={state}
            threadId="thread-context-form-submit"
          />,
        );
      });

      await act(async () => {
        window.dispatchEvent(
          new CustomEvent("aiops:composer-context-request", {
            detail: {
              artifactId: "artifact-param-resolution",
              title: "补充运维手册参数",
              manualId: "manual-redis-rca-ssh",
              workflowId: "workflow-redis-rca-ssh",
              submitAction: "submit_ops_manual_param_form",
              fields: [
                {
                  id: "redis_instance",
                  label: "Redis 实例",
                  type: "resource_ref",
                  uiControl: "select",
                  required: true,
                  candidates: [
                    { value: "docker:aiops-redis", label: "aiops-redis" },
                    { value: "docker:redis-shadow", label: "redis-shadow" },
                  ],
                },
              ],
            },
          }),
        );
      });

      const targetSelect = container.querySelector(
        'select[name="redis_instance"]',
      ) as HTMLSelectElement | null;
      expect(targetSelect).not.toBeNull();
      targetSelect!.value = "docker:aiops-redis";

      const submit = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent?.includes("提交并继续"),
      );
      await act(async () => {
        submit?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      });

      expect(
        container.querySelector('[data-testid="ops-manual-context-composer"]'),
      ).toBeNull();
      expect(container.querySelector("textarea")).not.toBeNull();
      expect(fetchSpy).toHaveBeenCalled();
      const requestBody = String(fetchSpy.mock.calls.at(-1)?.[1]?.body || "");
      expect(requestBody).toContain("已提交运维手册参数");
      expect(requestBody).toContain("submit_ops_manual_param_form");
      expect(requestBody).toContain("manual-redis-rca-ssh");
      expect(requestBody).toContain("workflow-redis-rca-ssh");
      expect(requestBody).toContain("opsManualParamsJson");
      expect(requestBody).toContain("redis_instance");
      expect(requestBody).toContain("docker:aiops-redis");
      expect(requestBody).not.toContain("补充必要信息，继续下一步自动排查");
      expect(requestBody).not.toContain("��");
    } finally {
      fetchSpy.mockRestore();
    }
  });

  it("falls back to text input when a parameter select has no discovered candidates", async () => {
    const state = createInitialAiopsTransportState(
      "thread-context-form-no-candidates",
    );
    state.sessionId = "sess-context-form-no-candidates";

    await act(async () => {
      root.render(
        <ChatPage
          initialState={state}
          threadId="thread-context-form-no-candidates"
        />,
      );
    });

    await act(async () => {
      window.dispatchEvent(
        new CustomEvent("aiops:composer-context-request", {
          detail: {
            artifactId: "artifact-param-resolution",
            title: "补充运维手册参数",
            manualId: "manual-redis-rca-ssh",
            workflowId: "workflow-redis-rca-ssh",
            submitAction: "submit_ops_manual_param_form",
            fields: [
              {
                id: "target_instance",
                label: "实例/服务",
                type: "resource_ref",
                uiControl: "select",
                required: true,
                placeholder:
                  "Read-only resource discovery ran on server-local, but found no Redis candidate.",
                candidates: [],
              },
            ],
          },
        }),
      );
    });

    expect(
      container.querySelector('[data-testid="ops-manual-context-composer"]'),
    ).not.toBeNull();
    expect(
      container.querySelector('select[name="target_instance"]'),
    ).toBeNull();
    const targetInput = container.querySelector(
      'input[name="target_instance"]',
    ) as HTMLInputElement | null;
    expect(targetInput).not.toBeNull();
    expect(targetInput?.placeholder).toContain("found no Redis candidate");
  });

  it("renders sensitive ops manual fields as Secret reference inputs without leaking defaults", async () => {
    const state = createInitialAiopsTransportState(
      "thread-context-form-secret",
    );
    state.sessionId = "sess-context-form-secret";
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("", {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    try {
      await act(async () => {
        root.render(
          <ChatPage
            initialState={state}
            threadId="thread-context-form-secret"
          />,
        );
      });

      await act(async () => {
        window.dispatchEvent(
          new CustomEvent("aiops:composer-context-request", {
            detail: {
              artifactId: "artifact-param-resolution-secret",
              title: "补充敏感运维参数",
              manualId: "manual-db-restore",
              workflowId: "workflow-db-restore",
              submitAction: "submit_ops_manual_param_form",
              fields: [
                {
                  id: "db_password",
                  label: "数据库密码",
                  type: "secret_ref",
                  uiControl: "secret_ref",
                  sensitive: true,
                  required: true,
                  default: "plain-secret-should-not-render",
                },
              ],
            },
          }),
        );
      });

      expect(
        container.querySelector('[data-testid="ops-manual-context-composer"]'),
      ).not.toBeNull();
      expect(container.textContent).toContain("数据库密码（Secret 引用）");
      expect(container.textContent).not.toContain(
        "plain-secret-should-not-render",
      );
      const passwordInput = container.querySelector(
        'input[name="db_password"]',
      ) as HTMLInputElement | null;
      expect(passwordInput).not.toBeNull();
      expect(passwordInput?.type).toBe("password");
      expect(passwordInput?.value).toBe("");
      expect(passwordInput?.placeholder).toContain("secret://");
      expect(passwordInput?.placeholder).toContain("避免填写明文密码");

      const submit = Array.from(container.querySelectorAll("button")).find(
        (button) => button.textContent?.includes("提交并继续"),
      );
      const fetchCallsBeforeEmptySubmit = fetchSpy.mock.calls.length;
      await act(async () => {
        submit?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      });
      expect(fetchSpy.mock.calls.length).toBe(fetchCallsBeforeEmptySubmit);

      passwordInput!.value = "secret://team/db-password";
      await act(async () => {
        submit?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      });

      expect(fetchSpy).toHaveBeenCalled();
      const requestBody = String(fetchSpy.mock.calls.at(-1)?.[1]?.body || "");
      expect(requestBody).toContain("secret://team/db-password");
      expect(requestBody).not.toContain("plain-secret-should-not-render");
    } finally {
      fetchSpy.mockRestore();
    }
  });

  it("restores the chat input when an artifact asks to skip ops manual usage", async () => {
    const state = createInitialAiopsTransportState("thread-skip-ops-manual");
    state.sessionId = "sess-skip-ops-manual";
    const fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      new Response("", {
        status: 200,
        headers: { "Content-Type": "text/plain" },
      }),
    );

    try {
      await act(async () => {
        root.render(
          <ChatPage initialState={state} threadId="thread-skip-ops-manual" />,
        );
      });

      await act(async () => {
        window.dispatchEvent(
          new CustomEvent("aiops:composer-context-request", {
            detail: {
              artifactId: "artifact-need-info",
              title: "补充运维手册必要信息",
              fields: [
                {
                  id: "target_location",
                  label: "目标位置",
                  placeholder: "server-01",
                },
              ],
            },
          }),
        );
      });

      expect(
        container.querySelector('[data-testid="ops-manual-context-composer"]'),
      ).not.toBeNull();
      expect(container.querySelector("textarea")).toBeNull();

      await act(async () => {
        window.dispatchEvent(
          new CustomEvent("aiops:composer-context-submit", {
            detail: {
              artifactId: "artifact-need-info",
              text: "已选择跳过运维手册。不要再调用 search_ops_manuals、resolve_ops_manual_params 或 run_ops_manual_preflight；请按普通只读排查继续。",
              metadata: {
                opsManualAction: "skip_ops_manual",
                opsManualSkipped: "true",
                opsManualManualId: "manual-pg-backup-ubuntu",
              },
            },
          }),
        );
      });

      expect(
        container.querySelector('[data-testid="ops-manual-context-composer"]'),
      ).toBeNull();
      expect(container.querySelector("textarea")).not.toBeNull();
      expect(fetchSpy).toHaveBeenCalled();
      const requestBody = fetchSpy.mock.calls
        .map((call) => String(call[1]?.body || ""))
        .join("\n");
      expect(requestBody).toContain("已选择跳过运维手册");
      expect(requestBody).toContain("search_ops_manuals");
      expect(requestBody).toContain("skip_ops_manual");
      expect(requestBody).toContain("opsManualSkipped");
    } finally {
      fetchSpy.mockRestore();
    }
  });

  it("renders the empty single-host greeting", async () => {
    const state = createInitialAiopsTransportState("thread-empty");
    state.sessionId = "sess-empty";

    await act(async () => {
      root.render(<ChatPage initialState={state} threadId="thread-empty" />);
    });

    expect(container.textContent).toContain("Hello there");
    expect(container.textContent).not.toContain("要对 server-local 做什么？");
  });
});

function sampleState(): AiopsTransportState {
  return {
    ...createInitialAiopsTransportState("thread-1"),
    sessionId: "sess-1",
    status: "blocked",
    currentTurnId: "turn-1",
    turnOrder: ["turn-1"],
    turns: {
      "turn-1": {
        id: "turn-1",
        status: "blocked",
        startedAt: "2026-05-06T00:00:00Z",
        user: {
          id: "user-1",
          text: "Investigate payment-api saturation",
          createdAt: "2026-05-06T00:00:00Z",
        },
        process: [
          {
            id: "cmd-1",
            kind: "command",
            status: "completed",
            text: "Rollout status",
            command: "kubectl rollout status deploy/payment-api",
            outputPreview: "deployment is waiting for approval",
          },
          {
            id: "approval-block-1",
            kind: "approval",
            status: "blocked",
            text: "Needs approval",
            command: "kubectl rollout restart deploy/payment-api",
            approvalId: "approval-1",
          },
        ],
        final: {
          id: "final-1",
          text: "payment-api is waiting for rollout approval.",
          status: "running",
        },
      },
    },
    pendingApprovals: {
      "approval-1": {
        id: "approval-1",
        turnId: "turn-1",
        type: "command",
        status: "blocked",
        command: "kubectl rollout restart deploy/payment-api",
      },
    },
    runtimeLiveness: {
      activeTurns: { "turn-1": true },
      activeAgents: {},
      pendingApprovals: { "approval-1": true },
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
  };
}

function sampleStateWithHostOps(): AiopsTransportState {
  return {
    ...sampleState(),
    activeHostMissionId: "mission-1",
    hostMissions: {
      "mission-1": {
        id: "mission-1",
        turnId: "turn-1",
        status: "running",
        planRequired: true,
        planAccepted: true,
        mentionedHosts: [
          {
            tokenId: "mention-1",
            raw: "@1.1.1.1",
            hostId: "host-1",
            address: "1.1.1.1",
            displayName: "Franklin",
            source: "inventory",
            resolved: true,
          },
          {
            tokenId: "mention-2",
            raw: "@1.1.1.2",
            hostId: "host-2",
            address: "1.1.1.2",
            displayName: "Harriet",
            source: "inventory",
            resolved: true,
          },
          {
            tokenId: "mention-3",
            raw: "@1.1.1.3",
            hostId: "host-3",
            address: "1.1.1.3",
            displayName: "Grace",
            source: "inventory",
            resolved: true,
          },
        ],
        childAgentIds: ["child-1", "child-2", "child-3"],
        planSteps: [
          { id: "step-1", title: "确认 PostgreSQL 拓扑", status: "pending" },
          { id: "step-2", title: "初始化主库", status: "pending" },
          { id: "step-3", title: "配置从库复制", status: "pending" },
          { id: "step-4", title: "部署监控节点", status: "pending" },
          { id: "step-5", title: "执行最终验证", status: "pending" },
        ],
      },
    } as AiopsTransportState["hostMissions"],
    childAgents: {
      "child-1": {
        id: "child-1",
        missionId: "mission-1",
        sessionId: "session-child-1",
        hostId: "host-1",
        hostAddress: "1.1.1.1",
        hostDisplayName: "Franklin",
        status: "running",
        task: "初始化主库",
      },
      "child-2": {
        id: "child-2",
        missionId: "mission-1",
        sessionId: "session-child-2",
        hostId: "host-2",
        hostAddress: "1.1.1.2",
        hostDisplayName: "Harriet",
        status: "running",
        task: "配置从库复制",
      },
      "child-3": {
        id: "child-3",
        missionId: "mission-1",
        sessionId: "session-child-3",
        hostId: "host-3",
        hostAddress: "1.1.1.3",
        hostDisplayName: "Grace",
        status: "waiting",
        task: "部署监控节点",
      },
    },
  };
}

async function flushMicrotasks() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

function countText(text: string, pattern: string) {
  return text.split(pattern).length - 1;
}
