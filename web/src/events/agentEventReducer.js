import { normalizeAgentEvent } from "./agentEventTypes";

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function cloneMapObject(value = {}) {
  return { ...(value || {}) };
}

function cloneList(value) {
  return Array.isArray(value) ? [...value] : [];
}

function normalizeProcessGroups(value) {
  const groups = {};
  const source = value && typeof value === "object" && !Array.isArray(value) ? value : {};
  for (const [turnId, rows] of Object.entries(source)) {
    groups[turnId] = Array.isArray(rows) ? [...rows] : [];
  }
  return groups;
}

function createProjection(sessionId = "") {
  return {
    sessionId,
    threadId: "",
    currentTurnId: "",
    status: "idle",
    lastSeq: 0,
    runtimeLiveness: {
      activeTurns: {},
      activeAgents: {},
      pendingApprovals: {},
      pendingUserInputs: {},
      activeCommandStreams: {},
    },
    timeline: [],
    agents: [],
    approvals: [],
    artifacts: [],
    diff: null,
    finalMessages: {},
    processGroups: {},
    clientTurnMap: {},
    lastTerminalFailed: false,
    lastTerminalCanceled: false,
  };
}

export function createAgentEventState() {
  return {
    eventsById: {},
    projectionsBySession: {},
    connection: {
      resyncing: false,
      seqGaps: {},
    },
  };
}

export function createLocalSubmittedEvents({ sessionId, turnId, clientTurnId, clientMessageId, text, hostId, createdAt, rowId }) {
  const timestamp = createdAt || new Date().toISOString();
  const resolvedTurnId = compactText(turnId || clientTurnId || `local-turn-${Date.now()}`);
  const resolvedClientTurnId = compactText(clientTurnId || resolvedTurnId);
  const resolvedClientMessageId = compactText(clientMessageId || `client-msg-${Date.now()}`);
  const resolvedRowId = compactText(rowId || resolvedClientMessageId);
  return [
    {
      eventId: `local:${resolvedClientMessageId}:turn.requested`,
      seq: 0,
      sessionId: compactText(sessionId),
      turnId: resolvedTurnId,
      clientTurnId: resolvedClientTurnId,
      kind: "turn",
      phase: "requested",
      status: "queued",
      visibility: "primary",
      source: "ui",
      createdAt: timestamp,
      payload: {
        prompt: compactText(text),
        title: compactText(text),
        hostId: compactText(hostId),
        clientMessageId: resolvedClientMessageId,
        rowId: resolvedRowId,
      },
    },
  ];
}

export function applyAgentEvent(state, rawEvent) {
  const event = normalizeAgentEvent(rawEvent);
  if (!event) return state || createAgentEventState();
  const base = state || createAgentEventState();
  if (base.eventsById?.[event.eventId]) return base;

  const projectionsBySession = { ...(base.projectionsBySession || {}) };
  const previousProjection = projectionsBySession[event.sessionId] || createProjection(event.sessionId);
  const expectedSeq = previousProjection.lastSeq + 1;
  const connection = {
    ...(base.connection || { resyncing: false, seqGaps: {} }),
    seqGaps: { ...(base.connection?.seqGaps || {}) },
  };
  if (event.seq > 0 && event.seq > expectedSeq) {
    connection.resyncing = true;
    connection.seqGaps[event.sessionId] = { expected: expectedSeq, received: event.seq };
  }

  const projection = applyEventToProjection(previousProjection, event);
  projectionsBySession[event.sessionId] = projection;

  return {
    ...base,
    eventsById: {
      ...(base.eventsById || {}),
      [event.eventId]: event,
    },
    projectionsBySession,
    connection,
  };
}

export function applyAgentProjectionSnapshot(state, projection) {
  const base = state || createAgentEventState();
  if (!projection?.sessionId) return base;
  return {
    ...base,
    projectionsBySession: {
      ...(base.projectionsBySession || {}),
      [projection.sessionId]: ensureProjection(projection, projection.sessionId),
    },
  };
}

function applyEventToProjection(previous, event) {
  let projection = ensureProjection(previous, event.sessionId);
  projection = {
    ...projection,
    threadId: projection.threadId || event.threadId,
    currentTurnId: nextProjectionCurrentTurnId(projection, event),
    lastSeq: event.seq > projection.lastSeq ? event.seq : projection.lastSeq,
    runtimeLiveness: {
      activeTurns: cloneMapObject(projection.runtimeLiveness.activeTurns),
      activeAgents: cloneMapObject(projection.runtimeLiveness.activeAgents),
      pendingApprovals: cloneMapObject(projection.runtimeLiveness.pendingApprovals),
      pendingUserInputs: cloneMapObject(projection.runtimeLiveness.pendingUserInputs),
      activeCommandStreams: cloneMapObject(projection.runtimeLiveness.activeCommandStreams),
    },
    timeline: [...projection.timeline],
    agents: [...projection.agents],
    approvals: [...projection.approvals],
    artifacts: [...projection.artifacts],
    finalMessages: { ...projection.finalMessages },
    processGroups: { ...projection.processGroups },
    clientTurnMap: { ...projection.clientTurnMap },
  };

  if (event.clientTurnId && event.turnId) {
    const previousTurnId = projection.clientTurnMap[event.clientTurnId];
    if (previousTurnId && previousTurnId !== event.turnId) {
      delete projection.runtimeLiveness.activeTurns[previousTurnId];
      projection.timeline = projection.timeline.map((row) =>
        row.turnId === previousTurnId ? { ...row, turnId: event.turnId } : row,
      );
    }
    projection.clientTurnMap[event.clientTurnId] = event.turnId;
  }

  if (event.kind === "turn") projection = applyTurn(projection, event);
  if (event.kind === "assistant") projection = applyAssistant(projection, event);
  if (event.kind === "tool") projection = applyTool(projection, event);
  if (event.kind === "approval") projection = applyApproval(projection, event);
  if (event.kind === "agent") projection = applyAgent(projection, event);
  if (event.kind === "artifact") projection = applyArtifact(projection, event);
  if (event.kind === "diff") projection = applyDiff(projection, event);

  projection.status = deriveProjectionStatus(projection);
  return projection;
}

function nextProjectionCurrentTurnId(projection, event) {
  if (!event?.turnId || event.kind !== "turn") return projection.currentTurnId;
  if (["requested", "started", "updated", "delta", "blocked"].includes(event.phase)) return event.turnId;
  return projection.currentTurnId || event.turnId;
}

function ensureProjection(projection, sessionId) {
  const source = projection && typeof projection === "object" && !Array.isArray(projection) ? projection : {};
  const defaults = createProjection(sessionId);
  const base = {
    ...defaults,
    ...source,
  };
  base.runtimeLiveness = {
    ...defaults.runtimeLiveness,
    ...cloneMapObject(source.runtimeLiveness),
  };
  base.timeline = cloneList(source.timeline);
  base.agents = cloneList(source.agents);
  base.approvals = cloneList(source.approvals);
  base.artifacts = cloneList(source.artifacts);
  base.finalMessages = cloneMapObject(source.finalMessages);
  base.processGroups = normalizeProcessGroups(source.processGroups);
  base.clientTurnMap = cloneMapObject(source.clientTurnMap);
  return base;
}

function applyTurn(projection, event) {
  if (["requested", "started", "updated", "delta", "blocked"].includes(event.phase)) {
    if (event.turnId) projection.runtimeLiveness.activeTurns[event.turnId] = true;
    projection.lastTerminalFailed = false;
    projection.lastTerminalCanceled = false;
    const rowId = resolveTurnRowId(projection, event);
    projection.timeline = upsertRow(projection.timeline, {
      id: rowId,
      kind: "turn",
      turnId: event.turnId,
      clientTurnId: event.clientTurnId,
      title: compactText(event.payload.title || event.payload.prompt),
      summary:
        compactText(event.payload.summary) ||
        (event.phase === "requested" ? "正在发送请求" : event.phase === "started" ? "正在等待 Agent 启动" : "正在执行"),
      status: event.status,
      visibility: event.visibility,
      updatedAt: event.createdAt,
      seq: event.seq,
    });
  }
  if (["completed", "failed", "canceled"].includes(event.phase)) {
    delete projection.runtimeLiveness.activeTurns[event.turnId];
    projection.runtimeLiveness.activeCommandStreams = {};
    projection.lastTerminalFailed = event.phase === "failed";
    projection.lastTerminalCanceled = event.phase === "canceled";
    const rowId = resolveTurnRowId(projection, event);
    projection.timeline = upsertRow(projection.timeline, {
      id: rowId,
      kind: "turn",
      turnId: event.turnId,
      clientTurnId: event.clientTurnId,
      title: compactText(event.payload.title || event.payload.prompt),
      summary:
        compactText(event.payload.summary || event.payload.error) ||
        (event.phase === "failed" ? "请求失败" : event.phase === "canceled" ? "已停止生成" : "已完成"),
      status: event.status,
      visibility: event.visibility,
      updatedAt: event.createdAt,
      seq: event.seq,
    });
  }
  return projection;
}

function resolveTurnRowId(projection, event) {
  const explicitRowId = compactText(event.payload.rowId);
  if (explicitRowId) return explicitRowId;

  const clientMessageId = compactText(event.payload.clientMessageId);
  const optimisticMessageRowId = clientMessageId ? `optimistic-user-${clientMessageId}` : "";
  const matchedRow = (projection.timeline || []).find((row) => {
    if (row?.kind !== "turn") return false;
    if (event.clientTurnId && row.clientTurnId === event.clientTurnId) return true;
    if (event.turnId && row.turnId === event.turnId) return true;
    if (clientMessageId && row.id === clientMessageId) return true;
    return Boolean(optimisticMessageRowId && row.id === optimisticMessageRowId);
  });
  if (matchedRow?.id) return matchedRow.id;
  return compactText(clientMessageId || event.clientTurnId || event.turnId || event.eventId);
}

function applyAssistant(projection, event) {
  const channel = compactText(event.payload.channel || "final");
  if (channel === "intent" || channel === "summary") {
    const text = compactText(event.payload.text || event.payload.delta);
    if (!text || !event.turnId) return projection;
    const row = {
      id: compactText(event.payload.messageId || event.eventId),
      kind: "assistant",
      turnId: event.turnId,
      clientTurnId: event.clientTurnId,
      title: channel,
      summary: text,
      status: event.status,
      visibility: event.visibility,
      updatedAt: event.createdAt,
      seq: event.seq,
    };
    projection.timeline = upsertRow(projection.timeline, row);
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
    return projection;
  }
  if (channel && channel !== "final") return projection;
  const text = compactText(event.payload.delta || event.payload.text);
  if (!text || !event.turnId) return projection;
  const current = projection.finalMessages[event.turnId] || { turnId: event.turnId, text: "", status: event.status };
  projection.finalMessages[event.turnId] = {
    ...current,
    text: `${current.text || ""}${text}`,
    status: event.status,
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  return projection;
}

function applyTool(projection, event) {
  const id = compactText(event.payload.toolCallId || event.eventId);
  const title = compactText(event.payload.displayName || event.payload.toolName);
  const summary = toolProjectionSummary(event.payload);
  const row = {
    id,
    kind: "tool",
    turnId: event.turnId,
    toolCallId: id,
    title,
    summary,
    status: event.status,
    visibility: event.visibility,
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  projection.timeline = upsertRow(projection.timeline, row);
  if (event.turnId) {
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
  }
  if (["started", "updated", "delta"].includes(event.phase)) {
    projection.runtimeLiveness.activeCommandStreams[id] = true;
  }
  if (["completed", "failed", "canceled"].includes(event.phase)) {
    delete projection.runtimeLiveness.activeCommandStreams[id];
  }
  return projection;
}

function toolProjectionSummary(payload = {}) {
  const input = compactText(payload.inputSummary);
  const output = compactText(payload.outputSummary);
  const error = compactText(payload.error);
  if (error) return input ? `${input}: ${error}` : error;
  return output || input;
}

function applyApproval(projection, event) {
  const id = compactText(event.payload.approvalId || event.eventId);
  projection.approvals = upsertRow(projection.approvals, {
    id,
    approvalType: compactText(event.payload.approvalType),
    title: compactText(event.payload.title),
    reason: compactText(event.payload.reason),
    risk: compactText(event.payload.risk),
    decision: compactText(event.payload.decision),
    targets: Array.isArray(event.payload.targets) ? event.payload.targets : [],
    status: event.status,
    updatedAt: event.createdAt,
  });
  if (event.phase === "requested" || event.phase === "blocked") {
    projection.runtimeLiveness.pendingApprovals[id] = true;
    if (event.payload.approvalType === "user_input" || event.payload.approvalType === "ask_user") {
      projection.runtimeLiveness.pendingUserInputs[id] = true;
    }
  }
  if (["resolved", "completed", "canceled"].includes(event.phase)) {
    delete projection.runtimeLiveness.pendingApprovals[id];
    delete projection.runtimeLiveness.pendingUserInputs[id];
  }
  return projection;
}

function applyAgent(projection, event) {
  const id = compactText(event.agentId || event.eventId);
  projection.agents = upsertRow(projection.agents, {
    id,
    handle: compactText(event.payload.handle),
    name: compactText(event.payload.name),
    role: compactText(event.payload.role),
    status: event.status,
    lastAction: compactText(event.payload.lastAction),
    lastSummary: compactText(event.payload.lastSummary),
    stats: event.payload.stats && typeof event.payload.stats === "object" ? { ...event.payload.stats } : undefined,
    updatedAt: event.createdAt,
  });
  if (["started", "updated", "delta"].includes(event.phase)) projection.runtimeLiveness.activeAgents[id] = true;
  if (["completed", "failed", "canceled"].includes(event.phase)) delete projection.runtimeLiveness.activeAgents[id];
  return projection;
}

function applyArtifact(projection, event) {
  const id = compactText(event.payload.artifactId || event.eventId);
  projection.artifacts = upsertRow(projection.artifacts, { id, ...event.payload, status: event.status, updatedAt: event.createdAt });
  return projection;
}

function applyDiff(projection, event) {
  projection.diff = { ...event.payload, updatedAt: event.createdAt };
  return projection;
}

function deriveProjectionStatus(projection) {
  const live = projection.runtimeLiveness;
  if (hasAny(live.pendingApprovals) || hasAny(live.pendingUserInputs)) return "blocked";
  if (hasAny(live.activeTurns) || hasAny(live.activeAgents) || hasAny(live.activeCommandStreams)) return "working";
  if (projection.lastTerminalFailed) return "failed";
  if (projection.lastTerminalCanceled) return "canceled";
  if (projection.diff) return "reviewing";
  return "idle";
}

function hasAny(values = {}) {
  return Object.values(values || {}).some(Boolean);
}

function upsertRow(list = [], row = {}) {
  const existing = Array.isArray(list) ? list : [];
  const index = existing.findIndex((item) => item?.id === row.id);
  if (index < 0) return [...existing, row];
  if (row.stats === undefined && existing[index]?.stats !== undefined) {
    row.stats = existing[index].stats;
  }
  const next = [...existing];
  next.splice(index, 1, {
    ...next[index],
    ...row,
    title: compactText(row.title || next[index]?.title),
    summary: compactText(row.summary || next[index]?.summary),
  });
  return next;
}
