import { describe, expect, it, vi } from "vitest";

import {
  createAiopsTransportCommandActions,
  createInitialAiopsTransportState,
  markAiopsTransportCanceled,
  markAiopsTransportFailed,
} from "./aiopsTransportRuntime";
import type { AiopsTransportState } from "./aiopsTransportTypes";

describe("aiopsTransportRuntime", () => {
  it("creates a fully initialized transport state for assistant-ui", () => {
    const state = createInitialAiopsTransportState("thread-1");

    expect(state).toMatchObject({
      schemaVersion: "aiops.transport.v1",
      sessionId: "",
      threadId: "thread-1",
      status: "idle",
      turns: {},
      turnOrder: [],
      pendingApprovals: {},
      mcpSurfaces: {},
      artifacts: {},
      runtimeLiveness: {
        activeTurns: {},
        activeAgents: {},
        pendingApprovals: {},
        pendingUserInputs: {},
        activeCommandStreams: {},
      },
      seq: 0,
    });
    expect(new Date(state.updatedAt).toString()).not.toBe("Invalid Date");
  });

  it("builds custom AssistantTransport commands from the current state", () => {
    const send = vi.fn();
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      sessionId: "sess-1",
      currentTurnId: "turn-1",
    } satisfies AiopsTransportState;
    const actions = createAiopsTransportCommandActions(state, send);

    actions.stop("user requested stop");
    actions.retry();
    actions.approvalDecision("approval-1", "reject");
    actions.choiceAnswer("choice-1", "continue");
    actions.mcpAction("filesystem", "open", { path: "/tmp" });
    actions.mcpRefresh("filesystem");
    actions.mcpPin("filesystem", true);

    expect(send.mock.calls.map(([command]) => command)).toEqual([
      { type: "aiops.stop", sessionId: "sess-1", turnId: "turn-1", reason: "user requested stop" },
      { type: "aiops.retry", sessionId: "sess-1", turnId: "turn-1" },
      { type: "aiops.approval-decision", approvalId: "approval-1", decision: "reject" },
      { type: "aiops.choice-answer", requestId: "choice-1", answer: "continue" },
      { type: "aiops.mcp-action", surfaceId: "filesystem", action: "open", params: { path: "/tmp" } },
      { type: "aiops.mcp-refresh", surfaceId: "filesystem" },
      { type: "aiops.mcp-pin", surfaceId: "filesystem", pinned: true },
    ]);
  });

  it("marks local state failed or canceled without mutating the previous state", () => {
    const state = {
      ...createInitialAiopsTransportState("thread-1"),
      status: "working",
      currentTurnId: "turn-1",
      turns: {
        "turn-1": { id: "turn-1", status: "working" },
      },
    } satisfies AiopsTransportState;

    const failed = markAiopsTransportFailed(state, "backend unavailable");
    const canceled = markAiopsTransportCanceled(state, "user canceled");

    expect(failed).toMatchObject({
      status: "failed",
      lastError: "backend unavailable",
      turns: { "turn-1": { status: "failed" } },
    });
    expect(canceled).toMatchObject({
      status: "canceled",
      lastError: "user canceled",
      turns: { "turn-1": { status: "canceled" } },
    });
    expect(state.status).toBe("working");
    expect(state.turns["turn-1"]?.status).toBe("working");
  });
});
