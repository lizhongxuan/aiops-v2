import type { AssistantTransportCommand } from "@assistant-ui/react";

import type {
  AiopsTransportHostMission,
  AiopsTransportState,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";

export type AiopsTransportCommandActions = {
  stop: (reason?: string) => void;
  retry: (turnId?: string) => void;
  approvalDecision: (
    approvalId: string,
    decision: "accept" | "reject" | string,
  ) => void;
  choiceAnswer: (requestId: string, answer: string) => void;
  mcpAction: (
    surfaceId: string,
    action: string,
    params?: Record<string, unknown>,
    target?: string,
  ) => void;
  mcpRefresh: (surfaceId: string) => void;
  mcpPin: (surfaceId: string, pinned: boolean) => void;
};

export function createInitialAiopsTransportState(
  threadId = "default",
): AiopsTransportState {
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
    hostMissions: {},
    childAgents: {},
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

export function normalizeAiopsTransportState(
  value: Partial<AiopsTransportState> | AiopsTransportState | null | undefined,
  fallbackThreadId = "default",
): AiopsTransportState {
  const base = createInitialAiopsTransportState(fallbackThreadId);
  if (!value || typeof value !== "object") {
    return base;
  }

  const runtimeLiveness = value.runtimeLiveness || base.runtimeLiveness;
  return {
    ...base,
    ...value,
    schemaVersion: value.schemaVersion || base.schemaVersion,
    sessionId: value.sessionId ?? base.sessionId,
    threadId: value.threadId || fallbackThreadId || base.threadId,
    status: value.status || base.status,
    opsRun: normalizeOpsRun(value.opsRun),
    turns: value.turns || {},
    turnOrder: Array.isArray(value.turnOrder) ? value.turnOrder : [],
    pendingApprovals: value.pendingApprovals || {},
    mcpSurfaces: value.mcpSurfaces || {},
    artifacts: value.artifacts || {},
    hostMissions: normalizeHostMissions(value.hostMissions),
    childAgents: value.childAgents || {},
    runtimeLiveness: {
      ...base.runtimeLiveness,
      ...runtimeLiveness,
      activeTurns: runtimeLiveness.activeTurns || {},
      activeAgents: runtimeLiveness.activeAgents || {},
      pendingApprovals: runtimeLiveness.pendingApprovals || {},
      pendingUserInputs: runtimeLiveness.pendingUserInputs || {},
      activeCommandStreams: runtimeLiveness.activeCommandStreams || {},
    },
    seq: typeof value.seq === "number" ? value.seq : base.seq,
    updatedAt: value.updatedAt || base.updatedAt,
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
      sendCommand(
        removeUndefined({
          type: "aiops.stop",
          sessionId,
          turnId: currentTurnId,
          reason,
        }),
      );
    },
    retry(turnId = currentTurnId) {
      sendCommand(
        removeUndefined({
          type: "aiops.retry",
          sessionId,
          turnId,
        }),
      );
    },
    approvalDecision(approvalId, decision) {
      sendCommand(
        removeUndefined({
          type: "aiops.approval-decision",
          sessionId,
          turnId: currentTurnId,
          approvalId,
          decision,
        }),
      );
    },
    choiceAnswer(requestId, answer) {
      sendCommand({
        type: "aiops.choice-answer",
        requestId,
        answer,
      });
    },
    mcpAction(surfaceId, action, params, target) {
      sendCommand(
        removeUndefined({
          type: "aiops.mcp-action",
          surfaceId,
          action,
          target,
          params,
        }),
      );
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

export function markAiopsTransportFailed(
  state: AiopsTransportState,
  message: string,
): AiopsTransportState {
  return markAiopsTransportTerminalState(state, "failed", message);
}

export function markAiopsTransportCanceled(
  state: AiopsTransportState,
  message?: string,
): AiopsTransportState {
  return markAiopsTransportTerminalState(state, "canceled", message);
}

function markAiopsTransportTerminalState(
  state: AiopsTransportState,
  status: "failed" | "canceled",
  message?: string,
): AiopsTransportState {
  const normalizedState = normalizeAiopsTransportState(state);
  const turns = { ...normalizedState.turns };
  const current = normalizedState.currentTurnId
    ? turns[normalizedState.currentTurnId]
    : undefined;
  if (normalizedState.currentTurnId && current) {
    turns[normalizedState.currentTurnId] = markTurnTerminal(current, status);
  }

  return {
    ...normalizedState,
    turns,
    status,
    lastError: message || normalizedState.lastError,
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: { ...normalizedState.runtimeLiveness.activeAgents },
      pendingApprovals: { ...normalizedState.runtimeLiveness.pendingApprovals },
      pendingUserInputs: {
        ...normalizedState.runtimeLiveness.pendingUserInputs,
      },
      activeCommandStreams: {},
    },
    updatedAt: new Date().toISOString(),
  };
}

function markTurnTerminal(
  turn: AiopsTransportTurn,
  status: "failed" | "canceled",
): AiopsTransportTurn {
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
  return Object.fromEntries(
    Object.entries(value).filter(([, item]) => item !== undefined),
  ) as T;
}

function normalizeOpsRun(value: unknown): AiopsTransportState["opsRun"] {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const run = value as AiopsTransportState["opsRun"];
  if (!run?.id) {
    return undefined;
  }
  return run;
}

function normalizeHostMissions(
  value: unknown,
): Record<string, AiopsTransportHostMission> {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(value)
      .filter(
        ([, mission]) =>
          Boolean(mission) &&
          typeof mission === "object" &&
          !Array.isArray(mission),
      )
      .map(([id, mission]) => {
        const item = mission as AiopsTransportHostMission & {
          mentionedHosts?: unknown;
          childAgentIds?: unknown;
          planSteps?: unknown;
        };
        return [
          id,
          {
            ...item,
            mentionedHosts: Array.isArray(item.mentionedHosts)
              ? item.mentionedHosts
              : [],
            childAgentIds: Array.isArray(item.childAgentIds)
              ? item.childAgentIds
              : [],
            planSteps: Array.isArray(item.planSteps)
              ? item.planSteps
              : undefined,
          },
        ];
      }),
  ) as Record<string, AiopsTransportHostMission>;
}
