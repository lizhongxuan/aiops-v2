import type {
  AssistantTransportConnectionMetadata,
  ThreadMessage,
} from "@assistant-ui/react";

import type {
  AiopsTransportState,
  AiopsTransportAgentUiArtifact,
  AiopsTransportBlock,
  AiopsTransportFinal,
  AiopsProcessBlock,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";
import { normalizeAiopsTransportState } from "./aiopsTransportRuntime";

type ConverterResult = {
  messages: ThreadMessage[];
  isRunning: boolean;
  state: AiopsTransportState;
};

export const AIOPS_TURN_DATA_PART = "aiops.transport.turn";
export const AIOPS_BLOCK_DATA_PART = "aiops.transport.block";

export function createAiopsTransportConverter() {
  return (
    state: AiopsTransportState,
    connectionMetadata: AssistantTransportConnectionMetadata,
  ): ConverterResult => {
    const normalizedState = canonicalizeTransportTranscript(normalizeAiopsTransportState(state));
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

export function canonicalizeTransportTranscript(state: AiopsTransportState): AiopsTransportState {
  return {
    ...state,
    turns: Object.fromEntries(
      Object.entries(state.turns || {}).map(([turnId, turn]) => [turnId, canonicalizeTransportTurn(turn)]),
    ),
  };
}

function canonicalizeTransportTurn(turn: AiopsTransportTurn): AiopsTransportTurn {
  const existingOrder = Array.isArray(turn.blockOrder) ? turn.blockOrder : [];
  const existingBlocks = turn.blocksById && typeof turn.blocksById === "object" ? turn.blocksById : {};
  const canonicalOrder = existingOrder.filter((id) => {
    const block = existingBlocks[id];
    return Boolean(id && block && isVisibleTranscriptBlock(block));
  });
  let blockOrder = canonicalOrder;
  let blocksById = Object.fromEntries(canonicalOrder.map((id) => [id, existingBlocks[id]]));

  // One-time compatibility projection for persisted pre-blockOrder states.
  // Production aiops.transport.v2 responses are emitted with native blocks.
  if (existingOrder.length === 0) {
    const legacyBlocks: AiopsTransportBlock[] = [];
    for (const block of turn.process || []) {
      if (!isVisibleTranscriptBlock({
        ...block,
        type: block.kind === "assistant" && block.phase === "commentary" ? "commentary" : block.kind,
      })) {
        continue;
      }
      legacyBlocks.push({
        ...block,
        type: block.kind === "assistant" && block.phase === "commentary" ? "commentary" : block.kind,
      });
    }
    if (turn.final?.id && turn.final.status !== "running") {
      legacyBlocks.push({
        id: uniqueBlockId(turn.final.id, legacyBlocks),
        type: "final_answer",
        kind: "assistant",
        displayKind: "assistant.message",
        phase: "final_answer",
        streamState: turn.final.status === "running" ? "streaming" : "complete",
        status: finalProcessStatus(turn.final),
        text: turn.final.text,
        durationMs: turn.final.durationMs,
        finalContract: turn.final,
      });
    }
    for (const artifact of mergeOpsManualSearchAndPreflightArtifacts(turn.agentUiArtifacts || [])) {
      legacyBlocks.push({
        id: uniqueBlockId(artifact.id, legacyBlocks),
        type: "artifact",
        kind: "tool",
        status: artifact.status === "failed" ? "failed" : "completed",
        text: artifact.summaryZh || artifact.summary || artifact.titleZh || artifact.title || "",
        artifact,
      });
    }
    blockOrder = legacyBlocks.map((block) => block.id);
    blocksById = Object.fromEntries(legacyBlocks.map((block) => [block.id, block]));
  }

  const { process: _legacyProcess, final: _legacyFinal, ...canonicalTurn } = turn;
  return { ...canonicalTurn, blockOrder, blocksById };
}

function isVisibleTranscriptBlock(block: AiopsTransportBlock) {
  const isAssistantMessage = block.kind === "assistant" || block.type === "commentary" || block.type === "final_answer";
  if (!isAssistantMessage) {
    return true;
  }
  const boundary = block as AiopsTransportBlock & {
    boundaryAction?: string;
    replacedByMessageId?: string;
  };
  const phase = String(block.phase || "").trim().toLowerCase();
  const streamState = String(block.streamState || "").trim().toLowerCase();
  const boundaryAction = String(boundary.boundaryAction || "").trim().toLowerCase();
  if (phase === "unclassified") {
    return false;
  }
  if (boundaryAction === "retry_once" || String(boundary.replacedByMessageId || "").trim()) {
    return false;
  }
  if (block.type !== "final_answer") {
    return true;
  }
  if (phase && phase !== "final_answer") {
    return false;
  }
  if (
    block.status === "queued" ||
    block.status === "running" ||
    block.finalContract?.status === "running" ||
    streamState === "streaming"
  ) {
    return false;
  }
  return true;
}

function uniqueBlockId(id: string, blocks: AiopsTransportBlock[]) {
  return blocks.some((block) => block.id === id) ? `artifact:${id}` : id;
}

function finalProcessStatus(final: AiopsTransportFinal): AiopsProcessBlock["status"] {
  switch (final.status) {
    case "failed":
    case "cancelled":
      return "failed";
    case "blocked":
    case "needs_evidence":
    case "approval_denied":
    case "tool_unavailable":
      return "blocked";
    case "running":
      return "running";
    default:
      return "completed";
  }
}

export function isAiopsTransportRunning(state: AiopsTransportState) {
  if (state.status === "working") {
    return true;
  }
  return Object.keys(state.runtimeLiveness?.activeTurns || {}).length > 0;
}

function orderedTurnMessages(state: AiopsTransportState) {
  return orderedTransportTurnIds(state).flatMap((turnId) => {
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

export function orderedTransportTurnIds(state: AiopsTransportState) {
  const seen = new Set<string>();
  const ids: string[] = [];
  for (const turnId of state.turnOrder || []) {
    if (state.turns[turnId] && !seen.has(turnId)) {
      seen.add(turnId);
      ids.push(turnId);
    }
  }
  for (const turnId of Object.keys(state.turns || {})) {
    if (!seen.has(turnId)) {
      seen.add(turnId);
      ids.push(turnId);
    }
  }
  return ids.sort((left, right) => compareTransportTurns(state.turns[left], state.turns[right], left, right));
}

function compareTransportTurns(left: AiopsTransportTurn | undefined, right: AiopsTransportTurn | undefined, leftId: string, rightId: string) {
  const leftTime = transportTurnSortTime(left);
  const rightTime = transportTurnSortTime(right);
  if (leftTime !== rightTime) {
    return leftTime - rightTime;
  }
  return leftId.localeCompare(rightId);
}

function transportTurnSortTime(turn?: AiopsTransportTurn) {
  return parseDate(turn?.startedAt || turn?.user?.createdAt || turn?.updatedAt || turn?.completedAt).getTime();
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
  const displayText = assistantDisplayText(state, turn);
  const content: ThreadMessage["content"] = [
    { type: "data", name: AIOPS_TURN_DATA_PART, data: assistantTurnEnvelope(turn) },
    ...orderedTurnBlocks(turn).map((block) => ({
      type: "data" as const,
      name: AIOPS_BLOCK_DATA_PART,
      data: block,
    })),
    ...(displayText ? [{ type: "text" as const, text: displayText }] : []),
  ];
  return {
    id: `${turn.id}:assistant`,
    role: "assistant",
    createdAt: parseDate(turn.completedAt || turn.startedAt),
    content,
    status: assistantMessageStatus(turn),
    metadata: {
      unstable_annotations: [],
      unstable_data: [],
      steps: [],
      custom: {
        source: "aiops.transport.assistant",
        turnId: turn.id,
      },
    },
  };
}

function assistantTurnEnvelope(turn: AiopsTransportTurn) {
  return {
    id: turn.id,
    status: turn.status,
    startedAt: turn.startedAt,
    completedAt: turn.completedAt,
    updatedAt: turn.updatedAt,
    contextGovernance: turn.contextGovernance || [],
  };
}

function assistantDisplayText(state: AiopsTransportState, turn: AiopsTransportTurn) {
  const final = finalAnswerBlock(turn);
  const finalText = final?.text?.trim() || "";
  if (finalText) {
    return finalText;
  }
  if (turn.status === "failed" && state.lastError) {
    return state.lastError;
  }
  return "";
}

function shouldShowAssistantMessage(turn: AiopsTransportTurn) {
  if ((turn.blockOrder || []).length > 0) {
    return true;
  }
  // Always show the message for failed/canceled turns so the error is visible
  if (turn.status === "failed" || turn.status === "canceled") {
    return true;
  }
  return turn.status === "submitted" || turn.status === "working" || turn.status === "blocked";
}

function orderedTurnBlocks(turn: AiopsTransportTurn) {
  return (turn.blockOrder || []).flatMap((id) => {
    const block = turn.blocksById?.[id];
    return block ? [block] : [];
  });
}

function processBlocks(turn: AiopsTransportTurn): AiopsProcessBlock[] {
  return orderedTurnBlocks(turn).filter((block) => block.type !== "final_answer" && block.type !== "artifact");
}

function finalAnswerBlock(turn: AiopsTransportTurn) {
  const blocks = orderedTurnBlocks(turn);
  for (let index = blocks.length - 1; index >= 0; index -= 1) {
    if (blocks[index]?.type === "final_answer") {
      return blocks[index];
    }
  }
  return undefined;
}

export function mergeOpsManualSearchAndPreflightArtifacts(artifacts: AiopsTransportAgentUiArtifact[]): AiopsTransportAgentUiArtifact[] {
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
  if (resolutionFlowID && searchFlowID) {
    return resolutionFlowID === searchFlowID;
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
    const workflowMatches = !resolutionWorkflowID || !hitWorkflowID || resolutionWorkflowID === hitWorkflowID;
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
      return { type: "incomplete", reason: "error", error: finalAnswerBlock(turn)?.text || "turn failed" };
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
