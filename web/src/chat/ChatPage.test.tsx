import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { ChatPage } from "./ChatPage";
import { createInitialAiopsTransportState } from "@/transport/aiopsTransportRuntime";
import type { AiopsTransportState } from "@/transport/aiopsTransportTypes";

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

  it("shows immediate feedback after submitting an approval decision", async () => {
    await act(async () => {
      root.render(<ChatPage initialState={sampleState()} />);
    });

    const submit = Array.from(container.querySelectorAll("button")).find((button) =>
      button.textContent?.includes("提交"),
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
      root.render(<ChatPage initialState={state} threadId="thread-confirmation" />);
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

    expect(container.querySelector('[data-testid="ops-manual-generation-confirmation"]')).not.toBeNull();
    expect(container.textContent).toContain("生成运维手册候选");
    expect(container.textContent).toContain("Redis 内存压力排障");
    expect(container.querySelector("textarea")).toBeNull();

    const cancel = Array.from(container.querySelectorAll("button")).find((button) => button.textContent?.includes("取消"));
    await act(async () => {
      cancel?.dispatchEvent(new MouseEvent("click", { bubbles: true }));
    });

    expect(container.querySelector('[data-testid="ops-manual-generation-confirmation"]')).toBeNull();
    expect(container.querySelector("textarea")).not.toBeNull();
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
