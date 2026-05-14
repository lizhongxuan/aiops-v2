import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { ChatPage } from "./ChatPage";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

describe("ChatPage", () => {
  let container: HTMLDivElement;
  let root: Root;
  let originalFetch: typeof globalThis.fetch | undefined;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    originalFetch = globalThis.fetch;
    globalThis.ResizeObserver = class ResizeObserver {
      observe() {}
      unobserve() {}
      disconnect() {}
    };
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => {
      root.unmount();
    });
    globalThis.fetch = originalFetch as typeof globalThis.fetch;
    container.remove();
  });

  it("renders assistant-ui chat state with typed process and approval blocks", async () => {
    await act(async () => {
      root.render(<ChatPage initialState={sampleState()} />);
    });

    expect(container.textContent).toContain("Investigate payment-api saturation");
    expect(container.textContent).toContain("kubectl rollout status deploy/payment-api");
    expect(container.textContent).toContain("payment-api is waiting for rollout approval.");
    expect(container.textContent).toContain("等待审批");
    expect(container.textContent).toContain("要执行这个命令，需要你确认吗？");
    expect(container.textContent).toContain("1. 是");
    expect(container.textContent).toContain("2. 是，且对于以后类似命令不再询问");
    expect(container.textContent).toContain("3. 否，请告知 AIOps 如何调整");
    expect(container.textContent).toContain("提交");
    expect(container.querySelector('[data-testid="codex-approval-inline"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="codex-approval-command"]')).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();
  });

  it("renders Agent-to-UI artifacts inside assistant messages", async () => {
    const state = sampleState();
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
              series: [{ name: "p95_latency_ms", data: [{ timestamp: 1, value: 980 }] }],
            },
          },
        },
      },
    ];

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(container.textContent).toContain("Coroot 延迟趋势");
    expect(container.textContent).toContain("接口 P95 延迟在 14:03 后明显升高。");
    expect(container.textContent).toContain("p95_latency_ms");
    expect(container.querySelector('a[href="/incidents/case-debug-1"]')?.textContent).toContain("查看 Case");
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

    const command = container.querySelector('[data-testid="codex-approval-command"]');
    expect(command?.textContent).toContain("kubectl rollout restart deploy/payment-api");
    expect(command?.textContent).not.toContain("stale command should not render");
  });

  it("replaces the composer with approval options whenever a current turn approval is pending", async () => {
    const state = sampleState();
    state.status = "working";

    await act(async () => {
      root.render(<ChatPage initialState={state} />);
    });

    expect(container.textContent).toContain("等待审批");
    expect(container.querySelector('[data-testid="codex-approval-inline"]')).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();
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

  it("renders experience-pack suggestion buttons as Agent-to-UI cards and replaces the composer for confirmation", async () => {
    const state = createInitialAiopsTransportState("thread-suggestion");
    state.sessionId = "sess-suggestion";
    state.experiencePackSuggestions = [
      {
        id: "suggestion-generate-runner",
        type: "generate_runner_workflow_candidate",
        label: "生成 Runner Workflow 草稿",
        reason: "命令数超过 6 且 Proof 明确",
        sourceRefs: ["case-pg", "proof-pg"],
      },
      {
        id: "suggestion-generate-pack",
        type: "generate_experience_pack_candidate",
        label: "生成经验包候选",
        reason: "命令数超过 6 且 Proof 明确",
        sourceRefs: ["case-pg", "proof-pg"],
      },
    ];

    await act(async () => {
      root.render(<ChatPage initialState={state} threadId="thread-suggestion" />);
    });

    const suggestionBar = container.querySelector('[data-testid="experience-pack-suggestion-bar"]');
    const composerShell = container.querySelector('[data-testid="aiops-composer-shell"]');
    expect(suggestionBar?.textContent).toContain("生成工作流");
    expect(suggestionBar?.textContent).toContain("生成经验包");
    expect(composerShell?.textContent).not.toContain("生成工作流");
    expect(composerShell?.textContent).not.toContain("生成经验包");

    const button = Array.from(suggestionBar?.querySelectorAll("button") || []).find((item) => item.textContent?.includes("生成经验包"));
    expect(button).toBeTruthy();
    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.querySelector('[data-testid="aiops-composer-shell"]')).toBeNull();
    expect(container.querySelector('[data-testid="experience-pack-confirmation-composer"]')).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();
    expect(container.textContent).toContain("确认生成经验包");
    expect(container.textContent).toContain("确认前不会写入经验包，也不会自动执行 Runner");
    expect(container.textContent).toContain("命令数超过 6 且 Proof 明确");
    expect(container.textContent).toContain("case-pg");
    expect(container.textContent).toContain("确认生成");

    const cancelButton = Array.from(container.querySelectorAll("button")).find((item) => item.textContent === "取消");
    await act(async () => {
      cancelButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(container.querySelector('[data-testid="experience-pack-confirmation-composer"]')).toBeNull();
    expect(container.querySelector('[data-testid="omnibar-input"]')).not.toBeNull();
  });

  it("previews experience-pack retrieval and adaptation while typing an ops request", async () => {
    globalThis.fetch = vi.fn(async (path) => {
      if (String(path) === "/api/v1/experience-packs/retrieve") {
        return new Response(JSON.stringify({
          items: [
            {
              pack_id: "pack-pg-cluster",
              skill: { name: "PG 主从部署经验包", summary: "PostgreSQL 主从部署和 pg_mon 经验" },
              confidence: 0.92,
              matched_signals: ["postgres", "主从", "pg_mon"],
              match_reasons: ["Skill 与 Gene 信号命中"],
            },
          ],
          total: 1,
        }), { status: 200, headers: { "Content-Type": "application/json" } });
      }
      return new Response(JSON.stringify({}), { status: 200, headers: { "Content-Type": "application/json" } });
    }) as typeof globalThis.fetch;
    const state = createInitialAiopsTransportState("thread-pg-preview");
    state.sessionId = "sess-pg-preview";

    await act(async () => {
      root.render(<ChatPage initialState={state} threadId="thread-pg-preview" />);
    });

    const input = container.querySelector('[data-testid="omnibar-input"]') as HTMLTextAreaElement;
    expect(input).toBeTruthy();
    await act(async () => {
      input.value = "给主机A和主机B搭建PG主从集群，pg_mon放到主机C上";
      input.dispatchEvent(new Event("input", { bubbles: true }));
    });

    expect(container.querySelector('[data-testid="experience-pack-intent-preview"]')?.textContent).toContain("经验包预检");
    expect(container.querySelector('[data-testid="aiops-composer-shell"]')?.contains(container.querySelector('[data-testid="experience-pack-intent-preview"]'))).toBe(false);
    expect(container.textContent).toContain("Runner 负责怎么执行");
    expect(container.textContent).toContain("GEP 记录经验来自哪次故障");
    expect(container.textContent).toContain("结构化条件过滤 + 关键词/BM25 + 向量语义检索");
    expect(container.textContent).toContain("推荐经验包、执行计划、Runner、风险范围和验证方式");
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 300));
    });
    expect(container.textContent).toContain("PG 主从部署经验包");
    expect(container.textContent).toContain("命中 92%");
  });

  it("prepares an experience-pack candidate after the user confirms a Chat suggestion", async () => {
    const fetchMock = vi.fn(async () => new Response(JSON.stringify({
      id: "candidate-pack-ai-chat-redis",
      candidate_id: "candidate-pack-ai-chat-redis",
      pack_id: "pack-ai-chat-redis",
      title: "Redis 运维排障经验包",
      status: "candidate",
    }), { status: 200, headers: { "Content-Type": "application/json" } }));
    globalThis.fetch = fetchMock as typeof globalThis.fetch;
    const state = createInitialAiopsTransportState("thread-suggestion-confirm");
    state.sessionId = "sess-suggestion-confirm";
    state.experiencePackSuggestions = [
      {
        id: "suggestion-generate-pack",
        type: "generate_experience_pack_candidate",
        label: "生成经验包候选",
        reason: "本次 Redis 运维轨迹具备复用价值",
        caseId: "case-ai-chat-redis",
        packId: "pack-ai-chat-redis",
        title: "Redis 运维排障经验包",
        summary: "从 AI Chat 运维轨迹生成候选经验。",
        service: "redis",
        environment: "prod",
        sourceRefs: ["thread-suggestion-confirm", "sess-suggestion-confirm"],
        metadata: {
          chatSessionId: "sess-suggestion-confirm",
          commands: [
            "redis-cli INFO memory",
            "redis-cli CONFIG GET maxmemory",
            "redis-cli SLOWLOG GET 10",
            "redis-cli --bigkeys",
            "curl -fsS http://payment-api/health",
            "redis-cli INFO stats",
          ],
        },
      },
    ];

    await act(async () => {
      root.render(<ChatPage initialState={state} threadId="thread-suggestion-confirm" />);
    });

    const button = Array.from(container.querySelectorAll("button")).find((item) => item.textContent?.includes("生成经验包"));
    await act(async () => {
      button?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });
    expect(container.querySelector('[data-testid="aiops-composer-shell"]')).toBeNull();
    const confirmButton = Array.from(container.querySelectorAll("button")).find((item) => item.textContent === "确认生成");
    await act(async () => {
      confirmButton?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
      await Promise.resolve();
    });

    expect(fetchMock).toHaveBeenCalled();
    const prepareCall = fetchMock.mock.calls.find(([path]) => String(path) === "/api/v1/experience-packs/candidates/prepare");
    expect(prepareCall).toBeTruthy();
    const [path, init] = prepareCall || [];
    expect(String(path)).toBe("/api/v1/experience-packs/candidates/prepare");
    expect(JSON.parse(String(init?.body))).toMatchObject({
      caseId: "case-ai-chat-redis",
      packId: "pack-ai-chat-redis",
      title: "Redis 运维排障经验包",
      service: "redis",
      environment: "prod",
      chatSessionId: "sess-suggestion-confirm",
      commands: [
        "redis-cli INFO memory",
        "redis-cli CONFIG GET maxmemory",
        "redis-cli SLOWLOG GET 10",
        "redis-cli --bigkeys",
        "curl -fsS http://payment-api/health",
        "redis-cli INFO stats",
      ],
    });
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
