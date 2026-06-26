import type {
  AssistantTransportConnectionMetadata,
  ThreadMessage,
} from "@assistant-ui/react";

import type {
  AiopsTransportState,
  AiopsTransportAgentUiArtifact,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";
import { normalizeAiopsTransportState } from "./aiopsTransportRuntime";

type ConverterResult = {
  messages: ThreadMessage[];
  isRunning: boolean;
  state: AiopsTransportState;
};

export function createAiopsTransportConverter() {
  return (
    state: AiopsTransportState,
    connectionMetadata: AssistantTransportConnectionMetadata,
  ): ConverterResult => {
    const normalizedState = normalizeAiopsTransportState(state);
    const messages = orderedTurnMessages(normalizedState).concat(
      optimisticPendingUserMessages(connectionMetadata),
    );

    return {
      messages,
      isRunning: isAiopsTransportRunning(normalizedState) || connectionMetadata.isSending,
      state: normalizedState,
    };
  };
}

export function isAiopsTransportRunning(state: AiopsTransportState) {
  if (state.status === "working") {
    return true;
  }
  return Object.keys(state.runtimeLiveness?.activeTurns || {}).length > 0;
}

function orderedTurnMessages(state: AiopsTransportState) {
  return state.turnOrder.flatMap((turnId) => {
    const turn = state.turns[turnId];
    if (!turn) {
      return [];
    }
    const messages: ThreadMessage[] = [];
    const userMessage = toUserThreadMessage(turn);
    if (userMessage) {
      messages.push(userMessage);
    }
    const assistantMessage = toAssistantThreadMessage(state, turn);
    if (assistantMessage) {
      messages.push(assistantMessage);
    }
    return messages;
  });
}

function toUserThreadMessage(turn: AiopsTransportTurn): ThreadMessage | null {
  if (!turn.user?.text) {
    return null;
  }
  return {
    id: turn.user.id || `${turn.id}:user`,
    role: "user",
    createdAt: parseDate(turn.user.createdAt || turn.startedAt),
    content: [{ type: "text", text: turn.user.text }],
    attachments: [],
    metadata: { custom: { turnId: turn.id, source: "aiops.transport.user" } },
  };
}

function toAssistantThreadMessage(state: AiopsTransportState, turn: AiopsTransportTurn): ThreadMessage | null {
  if (!shouldShowAssistantMessage(turn)) {
    return null;
  }
  let content: ThreadMessage["content"] = [];
  const finalText = turn.final?.text?.trim() || "";
  const finalIsRawRuntimeFailure = isRawRuntimeFailureText(finalText);
  if (finalText && !finalIsRawRuntimeFailure) {
    content = [{ type: "text", text: finalText }];
  } else if (turn.status === "failed" || turn.status === "canceled" || finalIsRawRuntimeFailure) {
    const preservedProgress = turn.status === "canceled"
      ? latestAssistantProcessText(turn.process || [])
      : latestSubstantialAssistantProcessText(turn.process || []);
    if (preservedProgress) {
      content = [{ type: "text", text: preservedProgress }];
    } else if (finalText) {
      content = [{ type: "text", text: finalText }];
    } else if (turn.status === "failed" && state.lastError) {
      content = [{ type: "text", text: state.lastError }];
    }
  }
  return {
    id: `${turn.id}:assistant`,
    role: "assistant",
    createdAt: parseDate(turn.completedAt || turn.startedAt),
    content,
    status: assistantMessageStatus(turn),
    metadata: {
      unstable_state: {
        turnId: turn.id,
        turnStatus: turn.status,
        turnStartedAt: turn.startedAt,
        turnCompletedAt: turn.completedAt,
        turnUpdatedAt: turn.updatedAt || turn.completedAt || turn.startedAt,
        finalDurationMs: turn.final?.durationMs,
        process: turn.process || [],
        agentRun: agentRunForTurn(state, turn),
        contextGovernance: turn.contextGovernance || [],
        intent: turn.intent || null,
        userText: turn.user?.text || "",
        agentUiArtifacts: visibleAgentUiArtifacts(turn),
        deferredAgentUiArtifacts: deferredAgentUiArtifacts(turn),
        activeHostMissionId: state.activeHostMissionId,
        hostMissions: state.hostMissions || {},
        childAgents: state.childAgents || {},
      },
      unstable_annotations: [],
      unstable_data: [],
      steps: [],
      custom: {
        source: "aiops.transport.assistant",
      },
    },
  };
}

function latestAssistantProcessText(process: AiopsTransportTurn["process"]) {
  for (let i = process.length - 1; i >= 0; i -= 1) {
    const block = process[i];
    if (block?.kind !== "assistant") {
      continue;
    }
    const text = block.text?.trim();
    if (text) {
      return text;
    }
  }
  return "";
}

function latestSubstantialAssistantProcessText(process: AiopsTransportTurn["process"]) {
  const text = latestAssistantProcessText(process);
  return isSubstantialAssistantProcessText(text) ? text : "";
}

function isSubstantialAssistantProcessText(text: string) {
  const normalized = text.trim();
  if (normalized.length >= 360) {
    return true;
  }
  const paragraphs = normalized.split(/\n\s*\n/).filter(Boolean).length;
  return normalized.length >= 180 && paragraphs >= 2;
}

function isRawRuntimeFailureText(text: string) {
  const normalized = text.trim().toLowerCase();
  if (!normalized) {
    return false;
  }
  return (
    normalized.includes("failed to receive stream chunk") ||
    normalized.includes("context deadline exceeded") ||
    normalized.includes("stream chunk") ||
    normalized.includes("upstream request timeout")
  );
}

function agentRunForTurn(state: AiopsTransportState, turn: AiopsTransportTurn) {
  const run = state.opsRun?.agentRun;
  if (!run?.id) {
    return undefined;
  }
  if (run.activeTurnId && run.activeTurnId !== turn.id) {
    return undefined;
  }
  if (state.opsRun?.turnId && state.opsRun.turnId !== turn.id) {
    return undefined;
  }
  return run;
}

function shouldShowAssistantMessage(turn: AiopsTransportTurn) {
  if (turn.final?.text || (turn.process || []).length > 0) {
    return true;
  }
  // Always show the message for failed/canceled turns so the error is visible
  if (turn.status === "failed" || turn.status === "canceled") {
    return true;
  }
  return turn.status === "submitted" || turn.status === "working" || turn.status === "blocked";
}

function visibleAgentUiArtifacts(turn: AiopsTransportTurn): AiopsTransportAgentUiArtifact[] {
  const artifacts = mergeOpsManualSearchAndPreflightArtifacts(turn.agentUiArtifacts || []);
  if (isTerminalTurn(turn.status)) {
    return artifacts;
  }
  return artifacts.filter((artifact) => !isDelayedWhileRunningArtifact(artifact));
}

function deferredAgentUiArtifacts(turn: AiopsTransportTurn): AiopsTransportAgentUiArtifact[] {
  if (isTerminalTurn(turn.status)) {
    return [];
  }
  return mergeOpsManualSearchAndPreflightArtifacts(turn.agentUiArtifacts || []).filter((artifact) => artifact.type === "coroot_chart");
}

function isDelayedWhileRunningArtifact(artifact: AiopsTransportAgentUiArtifact) {
  return artifact.type === "ops_manual_search_result" || artifact.type === "coroot_chart";
}

function isTerminalTurn(status: AiopsTransportTurn["status"]) {
  return status === "completed" || status === "failed" || status === "canceled";
}

function mergeOpsManualSearchAndPreflightArtifacts(artifacts: AiopsTransportAgentUiArtifact[]): AiopsTransportAgentUiArtifact[] {
  const consumed = new Set<string>();
  const mergedByPreflightId = new Map<string, Record<string, unknown>>();
  return artifacts.map((artifact, index) => {
    if (artifact.type !== "ops_manual_search_result" && artifact.type !== "ops_manual_param_resolution") {
      return artifact;
    }
    const preflight = findFollowingMatchingPreflight(artifacts, index, artifact);
    const paramResolution = artifact.type === "ops_manual_search_result" ? findFollowingMatchingParamResolution(artifacts, index, artifact) : undefined;
    if (paramResolution && artifact.type === "ops_manual_search_result") {
      consumed.add(paramResolution.id);
    }
    let mergedPreflightResult: Record<string, unknown> | undefined;
    if (preflight) {
      mergedPreflightResult = mergedByPreflightId.get(preflight.id) || {
        ...asRecord(preflight.inlineData),
        artifact_id: preflight.id,
      };
      mergedByPreflightId.set(preflight.id, mergedPreflightResult);
    }
    if (artifact.type === "ops_manual_search_result") {
      if (preflight) {
        consumed.add(preflight.id);
      }
      if (!paramResolution && !mergedPreflightResult) {
        return artifact;
      }
      return {
        ...artifact,
        id: paramResolution?.id || artifact.id,
        inlineData: {
          ...asRecord(artifact.inlineData),
          original_search_artifact_id: artifact.id,
          ...(paramResolution
            ? {
                merged_param_resolution: {
                  ...asRecord(paramResolution.inlineData),
                  artifact_id: paramResolution.id,
                },
              }
            : {}),
          ...(mergedPreflightResult ? { merged_preflight_result: mergedPreflightResult } : {}),
        },
      };
    }
    if (!mergedPreflightResult) {
      return artifact;
    }
    return {
      ...artifact,
      inlineData: {
        ...asRecord(artifact.inlineData),
        merged_preflight_result: mergedPreflightResult,
      },
    };
  }).filter((_, index) => !consumed.has(artifacts[index]?.id || ""));
}

function findFollowingMatchingParamResolution(artifacts: AiopsTransportAgentUiArtifact[], index: number, artifact: AiopsTransportAgentUiArtifact) {
  return artifacts.find((candidate, candidateIndex) =>
    candidateIndex > index &&
    candidate.type === "ops_manual_param_resolution" &&
    opsManualParamResolutionMatchesSearch(candidate, artifact)
  );
}

function findFollowingMatchingPreflight(artifacts: AiopsTransportAgentUiArtifact[], index: number, artifact: AiopsTransportAgentUiArtifact) {
  return artifacts.find((candidate, candidateIndex) =>
    candidateIndex > index &&
    candidate.type === "ops_manual_preflight_result" &&
    opsManualPreflightMatchesSearch(candidate, artifact)
  );
}

function opsManualParamResolutionMatchesSearch(paramResolution: AiopsTransportAgentUiArtifact, search: AiopsTransportAgentUiArtifact) {
  const resolutionData = asRecord(paramResolution.inlineData);
  const resolutionFlowID = opsManualFlowID(resolutionData);
  const searchData = asRecord(search.inlineData);
  const searchFlowID = opsManualFlowID(searchData);
  const resolutionManualID = text(pick(resolutionData, "manualId", "manual_id"));
  const resolutionWorkflowID = text(pick(resolutionData, "workflowId", "workflow_id"));
  const resolutionWorkflowIDForMatching = resolutionWorkflowID === searchFlowID ? "" : resolutionWorkflowID;
  if (resolutionFlowID && searchFlowID) {
    if (resolutionFlowID === searchFlowID) {
      return true;
    }
    if (resolutionWorkflowID !== searchFlowID) {
      return false;
    }
  }
  const manuals = arrayRecords(pick(searchData, "manuals", "hits", "matches", "items"));
  if (!manuals.length) {
    return true;
  }
  return manuals.some((hit) => {
    const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
    const workflowRef = asRecord(pick(manual, "workflowRef", "workflow_ref"));
    const hitManualID = text(pick(manual, "id", "manualId", "manual_id"), text(pick(hit, "manualId", "manual_id")));
    const hitWorkflowID = text(
      pick(hit, "boundWorkflowId", "bound_workflow_id", "workflowId", "workflow_id"),
      text(pick(workflowRef, "workflowId", "workflow_id")),
    );
    const manualMatches = !resolutionManualID || !hitManualID || resolutionManualID === hitManualID;
    const workflowMatches = !resolutionWorkflowIDForMatching || !hitWorkflowID || resolutionWorkflowIDForMatching === hitWorkflowID;
    return manualMatches && workflowMatches;
  });
}

function opsManualPreflightMatchesSearch(preflight: AiopsTransportAgentUiArtifact, search: AiopsTransportAgentUiArtifact) {
  const preflightData = asRecord(preflight.inlineData);
  const preflightFlowID = opsManualFlowID(preflightData);
  const searchData = asRecord(search.inlineData);
  const searchFlowID = opsManualFlowID(searchData);
  if (preflightFlowID && searchFlowID) {
    return preflightFlowID === searchFlowID;
  }
  const manualID = text(pick(preflightData, "manualId", "manual_id"));
  const workflowID = text(pick(preflightData, "workflowId", "workflow_id"));
  if (search.type === "ops_manual_param_resolution") {
    const resolutionManualID = text(pick(searchData, "manualId", "manual_id"));
    const resolutionWorkflowID = text(pick(searchData, "workflowId", "workflow_id"));
    const manualMatches = !manualID || !resolutionManualID || manualID === resolutionManualID;
    const workflowMatches = !workflowID || !resolutionWorkflowID || workflowID === resolutionWorkflowID;
    return manualMatches && workflowMatches;
  }
  const manuals = arrayRecords(pick(searchData, "manuals", "hits", "matches", "items"));
  if (!manuals.length) {
    return true;
  }
  return manuals.some((hit) => {
    const manual = asRecord(pick(hit, "manual", "opsManual", "ops_manual"));
    const workflowRef = asRecord(pick(manual, "workflowRef", "workflow_ref"));
    const hitManualID = text(pick(manual, "id", "manualId", "manual_id"), text(pick(hit, "manualId", "manual_id")));
    const hitWorkflowID = text(
      pick(hit, "boundWorkflowId", "bound_workflow_id", "workflowId", "workflow_id"),
      text(pick(workflowRef, "workflowId", "workflow_id")),
    );
    const manualMatches = !manualID || !hitManualID || manualID === hitManualID;
    const workflowMatches = !workflowID || !hitWorkflowID || workflowID === hitWorkflowID;
    return manualMatches && workflowMatches;
  });
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function arrayRecords(value: unknown): Record<string, unknown>[] {
  return Array.isArray(value) ? value.map(asRecord).filter((item) => Object.keys(item).length > 0) : [];
}

function pick(record: Record<string, unknown>, ...keys: string[]): unknown {
  for (const key of keys) {
    const value = record[key];
    if (value !== undefined && value !== null && value !== "") {
      return value;
    }
  }
  return undefined;
}

function opsManualFlowID(record: Record<string, unknown>): string {
  return text(pick(record, "opsManualFlowId", "ops_manual_flow_id"));
}

function text(value: unknown, fallback = ""): string {
  if (value === undefined || value === null) {
    return fallback;
  }
  return String(value).trim() || fallback;
}

function optimisticPendingUserMessages(connectionMetadata: AssistantTransportConnectionMetadata) {
  return connectionMetadata.pendingCommands.flatMap((command, index) => {
    if (command.type !== "add-message" || command.message.role !== "user") {
      return [];
    }
    const text = command.message.parts
      .filter((part) => part.type === "text")
      .map((part) => part.text)
      .join("\n")
      .trim();
    if (!text) {
      return [];
    }
    return [
      {
        id: `optimistic:add-message:${index}`,
        role: "user" as const,
        createdAt: new Date(),
        content: [{ type: "text" as const, text }],
        attachments: [],
        metadata: { custom: { optimistic: true } },
      } satisfies ThreadMessage,
    ];
  });
}

function assistantMessageStatus(turn: AiopsTransportTurn): ThreadMessage["status"] {
  switch (turn.status) {
    case "completed":
      return { type: "complete", reason: "stop" };
    case "blocked":
      return { type: "requires-action", reason: "interrupt" };
    case "failed":
      return { type: "incomplete", reason: "error", error: turn.final?.text || "turn failed" };
    case "canceled":
      return { type: "incomplete", reason: "cancelled" };
    default:
      return { type: "running" };
  }
}

function parseDate(value?: string) {
  if (!value) {
    return new Date(0);
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return new Date(0);
  }
  return parsed;
}
