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

  it("renders assistant-ui chat state with interleaved transcript blocks and approval controls", async () => {
    await act(async () => {
      root.render(<ChatPage initialState={sampleState()} />);
    });

    expect(container.textContent).toContain("Investigate payment-api saturation");
    expect(container.textContent).toContain("kubectl rollout status deploy/payment-api");
    expect(container.textContent).toContain("payment-api is waiting for rollout approval.");
    expect(container.textContent).toContain("等待审批");
    expect(container.textContent).toContain("要执行这个命令，需要你确认吗？");
    expect(container.textContent).toContain("1. 同意");
    expect(container.textContent).toContain("2. 拒绝");
    expect(container.textContent).toContain("同意");
    expect(container.querySelector('[data-testid="codex-approval-inline"]')).not.toBeNull();
    expect(container.querySelector('[data-testid="codex-approval-command"]')).not.toBeNull();
    expect(container.querySelector("textarea")).toBeNull();
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
        blockOrder: ["cmd-1", "text-1", "approval-block-1"],
        blocksById: {
          "cmd-1": {
            id: "cmd-1",
            type: "tool",
            tool: {
              toolKind: "command",
              title: "Shell",
              summary: "kubectl rollout status deploy/payment-api",
              status: "completed",
              command: "kubectl rollout status deploy/payment-api",
              output: {
                stdout: "deployment is waiting for approval",
                stderr: "",
                text: "deployment is waiting for approval",
                truncated: false,
              },
            },
          },
          "text-1": {
            id: "text-1",
            type: "text",
            text: {
              role: "assistant",
              text: "payment-api is waiting for rollout approval.",
              status: "streaming",
            },
          },
          "approval-block-1": {
            id: "approval-block-1",
            type: "approval",
            approval: {
              approvalId: "approval-1",
              approvalKind: "command",
              title: "等待审批",
              summary: "Needs approval",
              command: "kubectl rollout restart deploy/payment-api",
              status: "blocked",
              requestedAt: "2026-05-06T00:00:00Z",
            },
          },
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
