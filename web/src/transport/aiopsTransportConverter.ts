import type {
  AssistantTransportConnectionMetadata,
  ThreadMessage,
} from "@assistant-ui/react";

import type {
  AiopsTransportState,
  AiopsTransportAgentUiArtifact,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";

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
    const messages = orderedTurnMessages(state).concat(
      optimisticPendingUserMessages(connectionMetadata),
    );

    return {
      messages,
      isRunning: isAiopsTransportRunning(state) || connectionMetadata.isSending,
      state,
    };
  };
}

export function isAiopsTransportRunning(state: AiopsTransportState) {
  if (state.status === "working" || state.status === "blocked") {
    return true;
  }
  return Object.keys(state.runtimeLiveness.activeTurns || {}).length > 0;
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
    const assistantMessage = toAssistantThreadMessage(turn, state.lastError);
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

function toAssistantThreadMessage(turn: AiopsTransportTurn, lastError?: string): ThreadMessage | null {
  if (!shouldShowAssistantMessage(turn)) {
    return null;
  }
  // For failed turns without final text, show the error as content
  let content: ThreadMessage["content"] = [];
  if (turn.final?.text) {
    content = [{ type: "text", text: turn.final.text }];
  } else if (turn.status === "failed" && lastError) {
    content = [{ type: "text", text: lastError }];
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
        process: turn.process || [],
        intent: turn.intent || null,
        userText: turn.user?.text || "",
        agentUiArtifacts: visibleAgentUiArtifacts(turn),
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

function shouldShowAssistantMessage(turn: AiopsTransportTurn) {
  if (turn.final?.text || turn.intent?.text || (turn.process || []).length > 0) {
    return true;
  }
  // Always show the message for failed/canceled turns so the error is visible
  if (turn.status === "failed" || turn.status === "canceled") {
    return true;
  }
  return turn.status === "submitted" || turn.status === "working" || turn.status === "blocked";
}

function visibleAgentUiArtifacts(turn: AiopsTransportTurn): AiopsTransportAgentUiArtifact[] {
  const artifacts = turn.agentUiArtifacts || [];
  if (isTerminalTurn(turn.status)) {
    return mergeOpsManualSearchAndPreflightArtifacts(artifacts);
  }
  return mergeOpsManualSearchAndPreflightArtifacts(artifacts).filter((artifact) => artifact.type !== "ops_manual_search_result");
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
  const resolutionManualID = text(pick(resolutionData, "manualId", "manual_id"));
  const resolutionWorkflowID = text(pick(resolutionData, "workflowId", "workflow_id"));
  const searchData = asRecord(search.inlineData);
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
    const workflowMatches = !resolutionWorkflowID || !hitWorkflowID || resolutionWorkflowID === hitWorkflowID;
    return manualMatches && workflowMatches;
  });
}

function opsManualPreflightMatchesSearch(preflight: AiopsTransportAgentUiArtifact, search: AiopsTransportAgentUiArtifact) {
  const preflightData = asRecord(preflight.inlineData);
  const manualID = text(pick(preflightData, "manualId", "manual_id"));
  const workflowID = text(pick(preflightData, "workflowId", "workflow_id"));
  const searchData = asRecord(search.inlineData);
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
