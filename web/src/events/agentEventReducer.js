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

  if (shouldIgnoreStaleLocalTurnEvent(previousProjection, event)) {
    return base;
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
  if (event.kind === "reasoning") projection = applyReasoning(projection, event);
  if (event.kind === "system") projection = applySystem(projection, event);
  if (event.kind === "tool") projection = applyTool(projection, event);
  if (event.kind === "approval") projection = applyApproval(projection, event);
  if (event.kind === "agent") projection = applyAgent(projection, event);
  if (event.kind === "plan") projection = applyPlan(projection, event);
  if (event.kind === "evidence") projection = applyEvidence(projection, event);
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
    projection = clearPendingApprovalsForTerminalTurn(projection, event.status, event.createdAt);
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

function clearPendingApprovalsForTerminalTurn(projection, status, updatedAt) {
  const pendingIds = new Set([
    ...Object.keys(projection.runtimeLiveness.pendingApprovals || {}),
    ...Object.keys(projection.runtimeLiveness.pendingUserInputs || {}),
  ]);
  if (!pendingIds.size) return projection;
  return {
    ...projection,
    runtimeLiveness: {
      ...projection.runtimeLiveness,
      pendingApprovals: {},
      pendingUserInputs: {},
    },
    approvals: (projection.approvals || []).map((approval) =>
      pendingIds.has(approval?.id) ? { ...approval, status, updatedAt } : approval,
    ),
  };
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

function isTerminalTurnRow(row = {}) {
  const status = compactText(row.status).toLowerCase();
  const phase = compactText(row.phase).toLowerCase();
  return ["completed", "failed", "canceled", "cancelled"].includes(status) ||
    ["completed", "failed", "canceled", "cancelled"].includes(phase);
}

function shouldIgnoreStaleLocalTurnEvent(projection, event) {
  if (event.kind !== "turn" || event.seq !== 0 || event.source !== "ui") return false;
  if (!["requested", "started", "updated", "delta", "blocked"].includes(event.phase)) return false;
  const rowId = resolveTurnRowId(projection, event);
  return (projection.timeline || []).some((row) => {
    if (row?.kind !== "turn") return false;
    const sameTurn = compactText(row.id) === rowId ||
      (event.turnId && row.turnId === event.turnId) ||
      (event.clientTurnId && row.clientTurnId === event.clientTurnId);
    return sameTurn && isTerminalTurnRow(row);
  });
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
  const text = typeof event.payload.delta === "string"
    ? event.payload.delta
    : typeof event.payload.text === "string"
      ? event.payload.text
      : "";
  if (text === "" || !event.turnId) return projection;
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

function applyReasoning(projection, event) {
  const id = compactText(event.payload.itemId || event.eventId);
  const completed = event.status === "completed" || event.phase === "completed";
  const row = {
    id,
    kind: "reasoning",
    turnId: event.turnId,
    clientTurnId: event.clientTurnId,
    displayKind: "reasoning.summary",
    title: completed ? "思考摘要" : "正在思考",
    summary: compactText(event.payload.summary || event.payload.delta),
    status: completed ? "completed" : event.status,
    visibility: event.visibility,
    foldable: Boolean(event.payload.foldable || completed),
    autoCollapse: Boolean(event.payload.autoCollapse || completed),
    collapsed: Boolean((event.payload.autoCollapse || completed) && completed),
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  projection.timeline = upsertRow(projection.timeline, row);
  if (event.turnId) {
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
  }
  return projection;
}

function applySystem(projection, event) {
  const id = compactText(event.payload.id || event.eventId);
  const row = {
    id,
    kind: "system",
    turnId: event.turnId,
    clientTurnId: event.clientTurnId,
    displayKind: compactText(event.payload.displayKind),
    title: compactText(event.payload.title || "系统事件"),
    summary: compactText(event.payload.summary),
    detail: compactText(event.payload.detail),
    status: event.status,
    visibility: event.visibility,
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  projection.timeline = upsertRow(projection.timeline, row);
  if (event.turnId) {
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
  }
  return projection;
}

function applyTool(projection, event) {
  clearRunningFinalMessageForToolEvent(projection, event);
  const id = compactText(event.payload.toolCallId || event.eventId);
  const title = compactText(event.payload.title || event.payload.displayName || event.payload.toolName);
  const existingRow = projection.timeline.find((row) => row?.id === id) || {};
  const displayKind = inferToolDisplayKind(event.payload, existingRow);
  const summary = toolProjectionSummary(event.payload) || existingRow.summary || "";
  const inputSummary = resolveToolInputSummary(event.payload, existingRow, displayKind);
  const row = {
    id,
    kind: "tool",
    turnId: event.turnId,
    toolCallId: id,
    toolName: compactText(event.payload.toolName) || existingRow.toolName || "",
    displayKind,
    title,
    summary,
    inputSummary,
    inputPreview: event.payload.inputPreview ?? existingRow.inputPreview,
    command: compactText(event.payload.command) || existingRow.command || "",
    cwd: compactText(event.payload.cwd) || existingRow.cwd || "",
    outputSummary: compactText(event.payload.outputSummary) || existingRow.outputSummary || "",
    outputPreview: event.payload.outputPreview ?? existingRow.outputPreview,
    queries: resolveToolQueries(event.payload, existingRow, inputSummary, displayKind),
    results: Array.isArray(event.payload.results) ? event.payload.results : Array.isArray(existingRow.results) ? existingRow.results : [],
    exitCode: Number.isFinite(Number(event.payload.exitCode)) ? Number(event.payload.exitCode) : undefined,
    detail: compactText(event.payload.delta),
    risk: compactText(event.payload.risk),
    rawRef: compactText(event.payload.rawRef),
    foldable: Boolean(event.payload.foldable),
    autoCollapse: Boolean(event.payload.autoCollapse),
    collapsed: Boolean(event.payload.autoCollapse && event.status === "completed"),
    durationMs: Number.isFinite(Number(event.payload.durationMs)) ? Number(event.payload.durationMs) : event.durationMs,
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

function clearRunningFinalMessageForToolEvent(projection, event) {
  if (!event.turnId) return;
  const current = projection.finalMessages?.[event.turnId];
  if (!current || current.status === "completed") return;
  const summary = compactText(current.text);
  if (summary) {
    const row = {
      id: compactText(`${event.turnId}:assistant-process:${event.seq}`),
      kind: "assistant",
      turnId: event.turnId,
      clientTurnId: event.clientTurnId,
      title: "summary",
      summary,
      status: "completed",
      visibility: "secondary",
      updatedAt: current.updatedAt || event.createdAt,
      seq: current.seq || event.seq,
    };
    projection.timeline = upsertRow(projection.timeline, row);
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
  }
  delete projection.finalMessages[event.turnId];
}

function applyPlan(projection, event) {
  const steps = normalizePlanSteps(event.payload.steps);
  const runningStep = steps.find((step) => compactText(step?.status).toLowerCase() === "running");
  const fallbackStep = steps.length ? steps[steps.length - 1] : null;
  const id = compactText(event.payload.id || (event.turnId ? `${event.turnId}:plan` : event.eventId));
  const row = {
    id,
    kind: "plan",
    turnId: event.turnId,
    displayKind: "plan",
    title: compactText(event.payload.title || "计划"),
    summary: compactText(runningStep?.text || fallbackStep?.text || event.payload.summary),
    steps,
    status: event.status,
    visibility: event.visibility,
    foldable: true,
    autoCollapse: event.status === "completed",
    collapsed: event.status === "completed",
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  projection.timeline = upsertRow(projection.timeline, row);
  if (event.turnId) {
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
  }
  return projection;
}

function normalizePlanSteps(steps) {
  if (!Array.isArray(steps)) return [];
  let runningSeen = false;
  return steps.map((step) => {
    const next = { ...(step || {}) };
    let status = compactText(next.status).toLowerCase();
    if (status === "in_progress") status = "running";
    if (!status) status = "pending";
    if (status === "running") {
      if (runningSeen) status = "pending";
      else runningSeen = true;
    }
    next.status = status;
    next.text = compactText(next.text || next.summary || next.title);
    return next;
  });
}

function applyEvidence(projection, event) {
  const id = compactText(event.payload.id || event.eventId);
  const kind = compactText(event.payload.kind);
  const row = {
    id,
    kind: "evidence",
    turnId: event.turnId,
    displayKind: kind ? `evidence.${kind}` : "evidence",
    title: compactText(event.payload.title || kind || "证据"),
    summary: compactText(event.payload.summary),
    source: compactText(event.payload.source),
    confidence: compactText(event.payload.confidence),
    window: compactText(event.payload.window),
    rawRef: compactText(event.payload.rawRef),
    status: event.status,
    visibility: event.visibility,
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  projection.timeline = upsertRow(projection.timeline, row);
  if (event.turnId) {
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
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

function inferToolDisplayKind(payload = {}, existingRow = {}) {
  const explicit = compactText(payload.displayKind || existingRow.displayKind);
  if (explicit) return explicit;
  const name = compactText(payload.displayName || payload.toolName || existingRow.title).toLowerCase();
  if (["web_search", "browser.search"].includes(name)) return "browser.search";
  if (["exec_command", "shell_command", "execute_command", "execute_readonly_query", "code_mode"].includes(name)) return "host.command";
  return "";
}

function extractSearchQuery(value = "") {
  const text = compactText(value);
  if (!text) return "";
  const quoted = text.match(/\bquery\s+"([^"]+)"/i);
  if (quoted?.[1]) return quoted[1].trim();
  const parenthesized = text.match(/[（(]([^()（）]+)[)）]/u);
  if (parenthesized?.[1] && !/已搜索|正在搜索|completed/i.test(parenthesized[1])) return parenthesized[1].trim();
  return "";
}

function resolveToolInputSummary(payload = {}, existingRow = {}, displayKind = "") {
  const explicit = compactText(payload.inputSummary);
  if (explicit) return explicit;
  const existing = compactText(existingRow.inputSummary);
  if (existing) return existing;
  if (displayKind === "browser.search") {
    return extractSearchQuery(payload.outputSummary || payload.summary || existingRow.summary);
  }
  return "";
}

function resolveToolQueries(payload = {}, existingRow = {}, inputSummary = "", displayKind = "") {
  if (Array.isArray(payload.queries) && payload.queries.length) return payload.queries;
  if (Array.isArray(existingRow.queries) && existingRow.queries.length) return existingRow.queries;
  if (displayKind === "browser.search" && inputSummary) return [inputSummary];
  return [];
}

function applyApproval(projection, event) {
  const id = compactText(event.payload.approvalId || event.eventId);
  const approvalType = compactText(event.payload.approvalType);
  const row = {
    id,
    kind: "approval",
    turnId: event.turnId,
    clientTurnId: event.clientTurnId,
    displayKind: approvalType ? `approval.${approvalType}` : "approval.command",
    approvalId: id,
    approvalType,
    command: compactText(event.payload.command),
    reason: compactText(event.payload.reason),
    risk: compactText(event.payload.risk),
    decision: compactText(event.payload.decision),
    targets: Array.isArray(event.payload.targets) ? event.payload.targets : [],
    status: event.status,
    visibility: event.visibility,
    updatedAt: event.createdAt,
    seq: event.seq,
  };
  projection.approvals = upsertRow(projection.approvals, {
    id,
    approvalType,
    title: compactText(event.payload.title),
    command: row.command,
    reason: row.reason,
    risk: row.risk,
    decision: row.decision,
    targets: row.targets,
    status: event.status,
    updatedAt: event.createdAt,
  });
  projection.timeline = upsertRow(projection.timeline, row);
  if (event.turnId) {
    projection.processGroups[event.turnId] = upsertRow(projection.processGroups[event.turnId] || [], row);
  }
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
