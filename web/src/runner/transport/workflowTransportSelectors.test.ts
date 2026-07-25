import { describe, expect, it } from "vitest";

import type {
  AiopsTransportState,
  AiopsTransportTurn,
} from "@/transport/aiopsTransportTypes";
import {
  selectOrderedWorkflowBlocks,
  selectWorkflowApproval,
  selectWorkflowTransportActivity,
  selectWorkflowTransportTurns,
} from "./workflowTransportSelectors";

function transportState(overrides: Partial<AiopsTransportState> = {}): AiopsTransportState {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "session-workflow-a",
    threadId: "workflow:workflow-a",
    status: "idle",
    currentTurnId: "turn-a",
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
    hostMissions: {},
    childAgents: {},
    seq: 1,
    updatedAt: "2026-07-21T08:00:00Z",
    ...overrides,
  };
}

function turn(id: string, workflowId = "workflow-a"): AiopsTransportTurn {
  return {
    id,
    status: "completed",
    workflowContext: {
      workflowId,
      revision: {
        resourceVersion: "rv-1",
        semanticHash: "semantic-1",
        layoutHash: "layout-1",
      },
      selection: { nodeIds: ["node-a"], edgeIds: [] },
      environment: { name: "staging", bindingVersion: "env-1" },
    },
    blockOrder: ["commentary", "artifact", "final"],
    blocksById: {
      final: { id: "final", type: "final_answer", kind: "assistant", status: "completed", text: "完成" },
      ignored: { id: "ignored", type: "commentary", kind: "assistant", status: "completed", text: "不能显示" },
      artifact: {
        id: "artifact",
        type: "artifact",
        kind: "assistant",
        status: "completed",
        text: "修改预览",
        artifact: { id: "preview-1", type: "workflow_patch_preview" },
      },
      commentary: { id: "commentary", type: "commentary", kind: "assistant", status: "completed", text: "正在读取" },
    },
  };
}

describe("workflowTransportSelectors", () => {
  it("uses only blockOrder + blocksById and preserves the canonical order", () => {
    const selected = selectOrderedWorkflowBlocks(turn("turn-a"));

    expect(selected.map((block) => block.id)).toEqual(["commentary", "artifact", "final"]);
    expect(selected.some((block) => block.id === "ignored")).toBe(false);
  });

  it("selects only turns owned by the workflow transport scope", () => {
    const state = transportState({
      turnOrder: ["turn-a", "turn-b", "turn-missing"],
      turns: {
        "turn-a": turn("turn-a", "workflow-a"),
        "turn-b": turn("turn-b", "workflow-b"),
      },
    });

    expect(selectWorkflowTransportTurns(state, { workflowId: "workflow-a" }).map((item) => item.id)).toEqual(["turn-a"]);
    expect(selectWorkflowTransportTurns(state, { workflowId: "workflow-a", threadId: "wrong-thread" })).toEqual([]);
  });

  it("derives busy, stop and retry solely from transport state and liveness", () => {
    const working = transportState({
      status: "working",
      runtimeLiveness: {
        activeTurns: { "turn-a": true },
        activeAgents: {},
        pendingApprovals: {},
        pendingUserInputs: {},
        activeCommandStreams: {},
      },
      turns: { "turn-a": { ...turn("turn-a"), status: "working" } },
    });
    expect(selectWorkflowTransportActivity(working)).toEqual({
      busy: true,
      canStop: true,
      canRetry: false,
      retryTurnId: undefined,
    });

    const failed = transportState({
      status: "failed",
      turns: { "turn-a": { ...turn("turn-a"), status: "failed" } },
    });
    expect(selectWorkflowTransportActivity(failed)).toEqual({
      busy: false,
      canStop: false,
      canRetry: true,
      retryTurnId: "turn-a",
    });
  });

  it("keeps Stop available for a requested approval and retires that approval after cancellation", () => {
    const waiting = transportState({
      status: "blocked",
      currentTurnId: "turn-a",
      turns: { "turn-a": { ...turn("turn-a"), status: "blocked" } },
      pendingApprovals: {
        "approval-a": { id: "approval-a", turnId: "turn-a", status: "requested", reason: "应用当前候选" },
      },
      runtimeLiveness: {
        activeTurns: {}, activeAgents: {}, pendingApprovals: { "approval-a": true },
        pendingUserInputs: {}, activeCommandStreams: {},
      },
    });

    expect(selectWorkflowTransportActivity(waiting)).toEqual({
      busy: false,
      canStop: true,
      canRetry: false,
      retryTurnId: undefined,
    });

    const canceled = transportState({
      ...waiting,
      status: "canceled",
      turns: { "turn-a": { ...turn("turn-a"), status: "canceled" } },
    });
    expect(selectWorkflowTransportActivity(canceled)).toEqual({
      busy: false,
      canStop: false,
      canRetry: true,
      retryTurnId: "turn-a",
    });
    expect(selectWorkflowApproval(canceled, "turn-a", "approval-a")).toMatchObject({ status: "canceled" });
  });

  it("selects a resolved approval only when the exact block and turn own it", () => {
    const state = transportState({
      pendingApprovals: {
        "approval-1": { id: "approval-1", turnId: "turn-a", status: "rejected", reason: "用户拒绝" },
      },
    });

    expect(selectWorkflowApproval(state, "turn-a", "approval-1")).toMatchObject({ status: "rejected" });
    expect(selectWorkflowApproval(state, "turn-b", "approval-1")).toBeUndefined();
    expect(selectWorkflowApproval(state, "turn-a", "approval-missing")).toBeUndefined();
  });
});
