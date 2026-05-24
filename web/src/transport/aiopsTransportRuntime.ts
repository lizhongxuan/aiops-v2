import type { AssistantTransportCommand } from "@assistant-ui/react";

import type {
  AiopsTransportState,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";

export type AiopsTransportCommandActions = {
  stop: (reason?: string) => void;
  retry: (turnId?: string) => void;
  approvalDecision: (approvalId: string, decision: "accept" | "reject" | string) => void;
  choiceAnswer: (requestId: string, answer: string) => void;
  mcpAction: (surfaceId: string, action: string, params?: Record<string, unknown>, target?: string) => void;
  mcpRefresh: (surfaceId: string) => void;
  mcpPin: (surfaceId: string, pinned: boolean) => void;
};

export function createInitialAiopsTransportState(threadId = "default"): AiopsTransportState {
  return {
    schemaVersion: "aiops.transport.v2",
    sessionId: "",
    threadId,
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
    updatedAt: new Date().toISOString(),
  };
}

export function createAiopsTransportCommandActions(
  state: AiopsTransportState,
  sendCommand: (command: AssistantTransportCommand) => void,
): AiopsTransportCommandActions {
  const sessionId = state.sessionId || undefined;
  const currentTurnId = state.currentTurnId || undefined;

  return {
    stop(reason) {
      sendCommand(removeUndefined({
        type: "aiops.stop",
        sessionId,
        turnId: currentTurnId,
        reason,
      }));
    },
    retry(turnId = currentTurnId) {
      sendCommand(removeUndefined({
        type: "aiops.retry",
        sessionId,
        turnId,
      }));
    },
    approvalDecision(approvalId, decision) {
      sendCommand(removeUndefined({
        type: "aiops.approval-decision",
        sessionId,
        turnId: currentTurnId,
        approvalId,
        decision,
      }));
    },
    choiceAnswer(requestId, answer) {
      sendCommand({
        type: "aiops.choice-answer",
        requestId,
        answer,
      });
    },
    mcpAction(surfaceId, action, params, target) {
      sendCommand(removeUndefined({
        type: "aiops.mcp-action",
        surfaceId,
        action,
        target,
        params,
      }));
    },
    mcpRefresh(surfaceId) {
      sendCommand({
        type: "aiops.mcp-refresh",
        surfaceId,
      });
    },
    mcpPin(surfaceId, pinned) {
      sendCommand({
        type: "aiops.mcp-pin",
        surfaceId,
        pinned,
      });
    },
  };
}

export function markAiopsTransportFailed(state: AiopsTransportState, message: string): AiopsTransportState {
  return markAiopsTransportTerminalState(state, "failed", message);
}

export function markAiopsTransportCanceled(state: AiopsTransportState, message?: string): AiopsTransportState {
  return markAiopsTransportTerminalState(state, "canceled", message);
}

function markAiopsTransportTerminalState(
  state: AiopsTransportState,
  status: "failed" | "canceled",
  message?: string,
): AiopsTransportState {
  const turns = { ...state.turns };
  const current = state.currentTurnId ? turns[state.currentTurnId] : undefined;
  if (state.currentTurnId && current) {
    turns[state.currentTurnId] = markTurnTerminal(current, status);
  }

  return {
    ...state,
    turns,
    status,
    lastError: message || state.lastError,
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: { ...state.runtimeLiveness.activeAgents },
      pendingApprovals: { ...state.runtimeLiveness.pendingApprovals },
      pendingUserInputs: { ...state.runtimeLiveness.pendingUserInputs },
      activeCommandStreams: {},
    },
    updatedAt: new Date().toISOString(),
  };
}

function markTurnTerminal(turn: AiopsTransportTurn, status: "failed" | "canceled"): AiopsTransportTurn {
  return {
    ...turn,
    status,
    final: turn.final
      ? {
          ...turn.final,
          status: "failed",
        }
      : turn.final,
  };
}

function removeUndefined<T extends Record<string, unknown>>(value: T): T {
  return Object.fromEntries(Object.entries(value).filter(([, item]) => item !== undefined)) as T;
}
