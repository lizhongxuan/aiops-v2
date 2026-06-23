import { describe, expect, it, vi } from "vitest";

import {
  createAiopsTransportCommandActions,
  createInitialAiopsTransportState,
  markAiopsTransportCanceled,
  markAiopsTransportFailed,
  normalizeAiopsTransportState,
} from "./aiopsTransportRuntime";
import type { AiopsTransportState } from "./aiopsTransportTypes";

describe("aiopsTransportRuntime", () => {
  it("creates a fully initialized transport state for assistant-ui", () => {
    const state = createInitialAiopsTransportState("thread-1");

    expect(state).toMatchObject({
      schemaVersion: "aiops.transport.v2",
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
    expect(state.hostMissions).toEqual({});
    expect(state.childAgents).toEqual({});
    expect(state.activeHostMissionId).toBeUndefined();
    expect(new Date(state.updatedAt).toString()).not.toBe("Invalid Date");
  });

  it("normalizes nullable host mission arrays from runtime snapshots", () => {
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-null-host-mission"),
      hostMissions: {
        "mission-1": {
          id: "mission-1",
          turnId: "turn-1",
          status: "running",
          planRequired: true,
          planAccepted: true,
          mentionedHosts: null,
          childAgentIds: null,
          planSteps: null,
        },
      },
    } as unknown as Partial<AiopsTransportState>);

    expect(state.hostMissions["mission-1"].mentionedHosts).toEqual([]);
    expect(state.hostMissions["mission-1"].childAgentIds).toEqual([]);
    expect(state.hostMissions["mission-1"].planSteps).toBeUndefined();
  });

  it("normalizes optional ops run view state", () => {
    const state = normalizeAiopsTransportState({
      ...createInitialAiopsTransportState("thread-opsrun"),
      opsRun: {
        id: "opsrun-turn-1",
        source: "chat",
        status: "working",
        title: "主机A跟主机B上PG不同步",
        targetSummary: "主机A/主机B PG 与主机C pg_mon",
        evidenceCount: 2,
        currentStep: "正在只读采集 PG 同步证据",
      },
    });

    expect(state.opsRun).toMatchObject({
      id: "opsrun-turn-1",
      source: "chat",
      status: "working",
      evidenceCount: 2,
    });
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
      {
        type: "aiops.stop",
        sessionId: "sess-1",
        turnId: "turn-1",
        reason: "user requested stop",
      },
      { type: "aiops.retry", sessionId: "sess-1", turnId: "turn-1" },
      {
        type: "aiops.approval-decision",
        sessionId: "sess-1",
        turnId: "turn-1",
        approvalId: "approval-1",
        decision: "reject",
      },
      {
        type: "aiops.choice-answer",
        requestId: "choice-1",
        answer: "continue",
      },
      {
        type: "aiops.mcp-action",
        surfaceId: "filesystem",
        action: "open",
        params: { path: "/tmp" },
      },
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
