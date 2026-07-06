import type {
  AiopsProcessBlock,
  AiopsSpecialInputContext,
  AiopsTransportTimelineItem,
  AiopsTransportHostMission,
  AiopsTransportState,
  AiopsTransportTurn,
} from "./aiopsTransportTypes";
import { isRawTransportErrorMessage, toUserFacingTransportErrorMessage } from "./transportErrorMessage";

export type AiopsTransportCommand =
  | { type: "aiops.stop"; sessionId?: string; turnId?: string; reason?: string }
  | { type: "aiops.retry"; sessionId?: string; turnId?: string }
  | {
      type: "aiops.approval-decision";
      sessionId?: string;
      turnId?: string;
      approvalId: string;
      decision: string;
    }
  | { type: "aiops.choice-answer"; requestId: string; answer: string }
  | {
      type: "aiops.mcp-action";
      surfaceId: string;
      action: string;
      target?: string;
      params?: Record<string, unknown>;
    }
  | { type: "aiops.mcp-refresh"; surfaceId: string }
  | { type: "aiops.mcp-pin"; surfaceId: string; pinned: boolean }
  | { type: "aiops.special-input-clear"; sessionId?: string; resourceKind?: string; resourceId?: string; canonicalKey?: string }
  | { type: "aiops.special-input-confirm"; sessionId?: string; resourceKind?: string; resourceId?: string; canonicalKey?: string };

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
  specialInputClear: (target?: { resourceKind?: string; resourceId?: string; canonicalKey?: string }) => void;
  specialInputConfirm: (target?: { resourceKind?: string; resourceId?: string; canonicalKey?: string }) => void;
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
  if (!isCurrentAiopsTransportState(value)) {
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
    turns: normalizeTurns(value.turns),
    turnOrder: Array.isArray(value.turnOrder) ? value.turnOrder : [],
    pendingApprovals: value.pendingApprovals || {},
    mcpSurfaces: value.mcpSurfaces || {},
    artifacts: value.artifacts || {},
    hostMissions: normalizeHostMissions(value.hostMissions),
    childAgents: value.childAgents || {},
    specialInputContext: normalizeSpecialInputContext(value.specialInputContext),
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

export function isCurrentAiopsTransportState(
  value: Partial<AiopsTransportState> | AiopsTransportState | null | undefined,
): value is AiopsTransportState {
  if (!value || typeof value !== "object") {
    return false;
  }
  if (value.schemaVersion !== "aiops.transport.v2") {
    return false;
  }
  return Boolean(
    isRecord(value.turns) &&
    Array.isArray(value.turnOrder) &&
    isRecord(value.pendingApprovals) &&
    isRecord(value.mcpSurfaces) &&
    isRecord(value.artifacts) &&
    isRecord(value.hostMissions) &&
    isRecord(value.childAgents) &&
    isRuntimeLiveness(value.runtimeLiveness),
  );
}

function isRuntimeLiveness(value: unknown): value is AiopsTransportState["runtimeLiveness"] {
  if (!isRecord(value)) {
    return false;
  }
  return (
    isRecord(value.activeTurns) &&
    isRecord(value.activeAgents) &&
    isRecord(value.pendingApprovals) &&
    isRecord(value.pendingUserInputs) &&
    isRecord(value.activeCommandStreams)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

export function createAiopsTransportCommandActions(
  state: AiopsTransportState,
  sendCommand: (command: AiopsTransportCommand) => void,
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
    specialInputClear(target = {}) {
      sendCommand(
        removeUndefined({
          type: "aiops.special-input-clear",
          sessionId,
          resourceKind: target.resourceKind,
          resourceId: target.resourceId,
          canonicalKey: target.canonicalKey,
        }),
      );
    },
    specialInputConfirm(target = {}) {
      sendCommand(
        removeUndefined({
          type: "aiops.special-input-confirm",
          sessionId,
          resourceKind: target.resourceKind,
          resourceId: target.resourceId,
          canonicalKey: target.canonicalKey,
        }),
      );
    },
  };
}

export function markAiopsTransportFailed(
  state: AiopsTransportState,
  message: string,
): AiopsTransportState {
  return markAiopsTransportTerminalState(state, "failed", toUserFacingTransportErrorMessage(message));
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
    process: turn.process?.map((block) => markProcessBlockTerminal(block, status)),
    final: turn.final
      ? {
          ...turn.final,
          status: "failed",
        }
      : turn.final,
  };
}

function markProcessBlockTerminal(
  block: AiopsProcessBlock,
  status: "failed" | "canceled",
): AiopsProcessBlock {
  const modelWaitBlock = isModelWaitBlock(block);
  const shouldFinalize =
    block.status === "running" ||
    block.status === "queued" ||
    block.status === "blocked";
  if (!shouldFinalize && !modelWaitBlock) {
    return { ...block };
  }
  return {
    ...block,
    status: status === "canceled" ? "rejected" : "failed",
    text:
      modelWaitBlock
        ? status === "canceled"
          ? "模型调用已取消"
          : "模型调用失败"
        : block.text,
  };
}

function isModelWaitBlock(block: AiopsProcessBlock) {
  return (
    block.kind === "reasoning" &&
    (block.text === "排队等待模型返回" || block.text === "正在等待模型返回")
  );
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

function normalizeSpecialInputContext(value: unknown): AiopsSpecialInputContext | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const context = value as AiopsSpecialInputContext;
  if (context.schemaVersion !== "aiops.special_input_memory.v1") {
    return undefined;
  }
  const activeGrant = normalizeSpecialInputGrant(context.activeGrant);
  const visibleFacts = normalizeSpecialInputFacts(context.visibleFacts);
  const candidateFacts = normalizeSpecialInputFacts(context.candidateFacts);
  const suspendedGrants = normalizeSpecialInputGrants(context.suspendedGrants);
  const roleBindings = normalizeSpecialInputRoleBindings(context.roleBindings);
  const conflicts = normalizeSpecialInputConflicts(context.conflicts);
  const pendingConfirmations = normalizeSpecialInputPendingConfirmations(context.pendingConfirmations);
  const hasContent =
    Boolean(activeGrant) ||
    visibleFacts.length > 0 ||
    candidateFacts.length > 0 ||
    suspendedGrants.length > 0 ||
    roleBindings.length > 0 ||
    conflicts.length > 0 ||
    pendingConfirmations.length > 0 ||
    Boolean(context.modelSummary);
  if (!hasContent) {
    return undefined;
  }
  return {
    schemaVersion: context.schemaVersion,
    turnId: stringOrUndefined(context.turnId),
    activeGrant,
    visibleFacts,
    candidateFacts,
    suspendedGrants,
    roleBindings,
    conflicts,
    pendingConfirmations,
    modelSummary: stringOrUndefined(context.modelSummary),
  };
}

function normalizeSpecialInputGrant(value: unknown): AiopsSpecialInputContext["activeGrant"] {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return undefined;
  }
  const grant = value as NonNullable<AiopsSpecialInputContext["activeGrant"]>;
  const resourceId = stringOrUndefined(grant.resourceId);
  const canonicalKey = stringOrUndefined(grant.canonicalKey);
  if (!resourceId && !canonicalKey) {
    return undefined;
  }
  return {
    id: stringOrUndefined(grant.id),
    factId: stringOrUndefined(grant.factId),
    resourceKind: stringOrUndefined(grant.resourceKind),
    resourceId,
    canonicalKey,
    display: stringOrUndefined(grant.display),
    allowedActions: normalizeStringList(grant.allowedActions),
    trustLevel: stringOrUndefined(grant.trustLevel),
    status: stringOrUndefined(grant.status),
    scope: stringOrUndefined(grant.scope),
  };
}

function normalizeSpecialInputGrants(value: unknown): NonNullable<AiopsSpecialInputContext["suspendedGrants"]> {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.map(normalizeSpecialInputGrant).filter((grant): grant is NonNullable<AiopsSpecialInputContext["activeGrant"]> => Boolean(grant));
}

function normalizeSpecialInputFacts(value: unknown): NonNullable<AiopsSpecialInputContext["visibleFacts"]> {
  if (!Array.isArray(value)) {
    return [];
  }
  const facts: NonNullable<AiopsSpecialInputContext["visibleFacts"]> = [];
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      continue;
    }
    const fact = item as NonNullable<AiopsSpecialInputContext["visibleFacts"]>[number];
    const id = stringOrUndefined(fact.id);
    const display = stringOrUndefined(fact.display);
    const resourceId = stringOrUndefined(fact.resourceId);
    const canonicalKey = stringOrUndefined(fact.canonicalKey);
    if (!id && !display && !resourceId && !canonicalKey) {
      continue;
    }
    facts.push({
      id,
      kind: stringOrUndefined(fact.kind),
      resourceKind: stringOrUndefined(fact.resourceKind),
      resourceId,
      canonicalKey,
      display,
      trustLevel: stringOrUndefined(fact.trustLevel),
      status: stringOrUndefined(fact.status),
      environmentKey: stringOrUndefined(fact.environmentKey),
      clusterKey: stringOrUndefined(fact.clusterKey),
    });
  }
  return facts;
}

function normalizeSpecialInputRoleBindings(value: unknown): NonNullable<AiopsSpecialInputContext["roleBindings"]> {
  if (!Array.isArray(value)) {
    return [];
  }
  const bindings: NonNullable<AiopsSpecialInputContext["roleBindings"]> = [];
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      continue;
    }
    const binding = item as NonNullable<AiopsSpecialInputContext["roleBindings"]>[number];
    const roleKey = stringOrUndefined(binding.roleKey);
    const resourceId = stringOrUndefined(binding.resourceId);
    if (!roleKey && !resourceId) {
      continue;
    }
    bindings.push({
      id: stringOrUndefined(binding.id),
      roleKey,
      runtimeName: stringOrUndefined(binding.runtimeName),
      resourceKind: stringOrUndefined(binding.resourceKind),
      resourceId,
      display: stringOrUndefined(binding.display),
      environmentKey: stringOrUndefined(binding.environmentKey),
      clusterKey: stringOrUndefined(binding.clusterKey),
      bindingHash: stringOrUndefined(binding.bindingHash),
      status: stringOrUndefined(binding.status),
      confidence: typeof binding.confidence === "number" ? binding.confidence : undefined,
    });
  }
  return bindings;
}

function normalizeSpecialInputConflicts(value: unknown): NonNullable<AiopsSpecialInputContext["conflicts"]> {
  if (!Array.isArray(value)) {
    return [];
  }
  const conflicts: NonNullable<AiopsSpecialInputContext["conflicts"]> = [];
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      continue;
    }
    const conflict = item as NonNullable<AiopsSpecialInputContext["conflicts"]>[number];
    const id = stringOrUndefined(conflict.id);
    if (!id) {
      continue;
    }
    conflicts.push({
      id,
      kind: stringOrUndefined(conflict.kind),
      canonicalKey: stringOrUndefined(conflict.canonicalKey),
      roleKey: stringOrUndefined(conflict.roleKey),
      environmentKey: stringOrUndefined(conflict.environmentKey),
      clusterKey: stringOrUndefined(conflict.clusterKey),
      resourceIds: normalizeStringList(conflict.resourceIds),
      reasons: normalizeStringList(conflict.reasons),
    });
  }
  return conflicts;
}

function normalizeSpecialInputPendingConfirmations(value: unknown): NonNullable<AiopsSpecialInputContext["pendingConfirmations"]> {
  if (!Array.isArray(value)) {
    return [];
  }
  const pendingItems: NonNullable<AiopsSpecialInputContext["pendingConfirmations"]> = [];
  for (const item of value) {
    if (!item || typeof item !== "object" || Array.isArray(item)) {
      continue;
    }
    const pending = item as NonNullable<AiopsSpecialInputContext["pendingConfirmations"]>[number];
    const id = stringOrUndefined(pending.id);
    const reason = stringOrUndefined(pending.reason);
    if (!id && !reason) {
      continue;
    }
    pendingItems.push({
      id,
      kind: stringOrUndefined(pending.kind),
      reason,
      roleKey: stringOrUndefined(pending.roleKey),
      environmentKey: stringOrUndefined(pending.environmentKey),
      clusterKey: stringOrUndefined(pending.clusterKey),
      candidateIds: normalizeStringList(pending.candidateIds),
    });
  }
  return pendingItems;
}

function normalizeTurns(value: unknown): AiopsTransportState["turns"] {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    return {};
  }
  return Object.fromEntries(
    Object.entries(value as AiopsTransportState["turns"]).map(([turnId, turn]) => [turnId, normalizeTurn(turn)]),
  );
}

function stringOrUndefined(value: unknown) {
  const text = typeof value === "string" ? value.trim() : "";
  return text || undefined;
}

function normalizeStringList(value: unknown) {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .map((item) => (typeof item === "string" ? item.trim() : ""))
    .filter(Boolean);
}

function normalizeTurn(turn: AiopsTransportTurn): AiopsTransportTurn {
  const process = Array.isArray(turn.process)
    ? turn.process.map(normalizeProcessBlock).filter((block): block is AiopsProcessBlock => Boolean(block))
    : undefined;
  const timeline = Array.isArray(turn.timeline)
    ? turn.timeline.map(normalizeTimelineItem).filter((item): item is AiopsTransportTimelineItem => Boolean(item))
    : undefined;
  const finalText = sanitizeUserVisibleFinalText(turn.final?.text);
  return {
    ...turn,
    process,
    timeline,
    final: turn.final && finalText ? { ...turn.final, text: finalText } : undefined,
  };
}

function normalizeTimelineItem(item: AiopsTransportTimelineItem): AiopsTransportTimelineItem | undefined {
  if (!item || typeof item !== "object") {
    return undefined;
  }
  const id = String(item.id || "").trim();
  const type = String(item.type || "").trim();
  if (!id || !type) {
    return undefined;
  }
  const text = sanitizeUserVisibleRuntimeText(item.text || "");
  return {
    ...item,
    id,
    type,
    status: item.status ? String(item.status).trim() : undefined,
    text: text || undefined,
    payloadKind: item.payloadKind ? String(item.payloadKind).trim() : undefined,
  };
}

function normalizeProcessBlock(block: AiopsProcessBlock): AiopsProcessBlock | undefined {
  const sanitizeBlockText = block.kind === "assistant" ? sanitizeUserVisibleFinalText : sanitizeUserVisibleRuntimeText;
  const next: AiopsProcessBlock = {
    ...block,
    text: sanitizeBlockText(block.text || ""),
    command: sanitizeOptionalRuntimeText(block.command),
    inputSummary: sanitizeOptionalRuntimeText(block.inputSummary),
    outputPreview: sanitizeOptionalRuntimeText(block.outputPreview),
    steps: block.steps?.map((step) => ({
      ...step,
      text: sanitizeUserVisibleRuntimeText(step.text),
      title: sanitizeOptionalRuntimeText(step.title),
      summary: sanitizeOptionalRuntimeText(step.summary),
    })),
  };
  if (
    !next.text &&
    !next.command &&
    !next.inputSummary &&
    !next.outputPreview &&
    !next.steps?.length &&
    !next.queries?.length &&
    !next.results?.length
  ) {
    return undefined;
  }
  return next;
}

function sanitizeUserVisibleFinalText(value?: string) {
  const text = sanitizeUserVisibleRuntimeText(value || "");
  if (!text) {
    return "";
  }
  return redactRiskyOperationalAdvice(text);
}

function sanitizeOptionalRuntimeText(value?: string) {
  const text = sanitizeUserVisibleRuntimeText(value || "");
  return text || undefined;
}

function sanitizeUserVisibleRuntimeText(value: string) {
  const text = (value || "").trim();
  if (!text) {
    return "";
  }
  const lower = text.toLowerCase();
  if (
    lower.includes("verification completion gate") ||
    lower.includes("block_success_final") ||
    lower.includes("missing_verification_report") ||
    lower.includes("execution_required,missing_verification_report")
  ) {
    return "";
  }
  if (isRawTransportErrorMessage(text)) {
    return toUserFacingTransportErrorMessage(text);
  }
  return text;
}

function redactRiskyOperationalAdvice(text: string) {
  const lines = text.split(/\r?\n/);
  let redacted = false;
  const safeLines = lines.filter((line) => {
    if (!isRiskyOperationalAdviceLine(line)) {
      return true;
    }
    redacted = true;
    return false;
  });
  if (!redacted) {
    return text;
  }
  const safeText = safeLines.join("\n").trim();
  return safeText;
}

function isRiskyOperationalAdviceLine(text: string) {
  const normalized = text.trim().toLowerCase();
  if (!normalized) {
    return false;
  }
  if (normalized.includes("rm -rf") || /rm\s+-rf/.test(normalized)) {
    return true;
  }
  if (isGatedOrAnalyticalFinalLine(normalized)) {
    return false;
  }
  return (
    containsAny(normalized, ["删除", "清理", "清空", "delete"]) &&
    containsAny(normalized, ["archive", "wal", "pgdata", "pg_data", "$pgdata", "$pg_data", "数据目录", "归档"]) &&
    hasDirectRiskyOperationLeadIn(normalized)
  );
}

function isGatedOrAnalyticalFinalLine(text: string) {
  if (
    containsAny(text, [
      "结论",
      "根因",
      "原因",
      "机制",
      "路径",
      "表明",
      "说明",
      "可能",
      "假设",
      "推断",
      "证据",
      "边界",
      "缺失",
      "只读",
      "不做任何变更",
      "候选",
      "影响面",
      "切勿",
      "不要",
      "不能",
      "不可",
      "未验证",
      "无法确认",
      "残留",
      "未完全清空",
      "未清空",
    ]) &&
    !hasDirectRiskyOperationLeadIn(text)
  ) {
    return true;
  }
  return containsAny(text, [
    "确认根因后",
    "若需修复",
    "需要修复",
    "必须选定",
    "变更窗口",
    "维护窗口",
    "审批",
    "批准",
    "备份",
    "回滚",
    "验收",
    "权威数据源",
    "authoritative",
  ]);
}

function hasDirectRiskyOperationLeadIn(text: string) {
  return containsAny(text, [
    "可以执行",
    "建议执行",
    "请执行",
    "直接执行",
    "执行以下",
    "运行以下",
    "执行命令",
    "运行命令",
    "run ",
    "execute ",
    "directly run",
    "directly execute",
    "直接清空",
    "直接删除",
    "直接清理",
    "清空 ",
    "删除 ",
    "清理 ",
    "delete ",
  ]);
}

function containsAny(value: string, needles: string[]) {
  return needles.some((needle) => value.includes(needle.toLowerCase()));
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
