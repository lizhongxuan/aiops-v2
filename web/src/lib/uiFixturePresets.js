function createBaseHosts() {
  return [
    { id: "web-01", name: "web-01", status: "online", executable: true, terminalCapable: true },
    { id: "web-02", name: "web-02", status: "online", executable: true, terminalCapable: true },
    { id: "server-local", name: "server-local", status: "online", executable: true, terminalCapable: true },
  ];
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function latestUserCard(cards = []) {
  return [...cards].reverse().find((card) => card?.type === "UserMessageCard" || (card?.type === "MessageCard" && card?.role === "user"));
}

function resolveFixtureTurnId(turn = {}, cards = []) {
  const userCard = latestUserCard(cards);
  return compactText(turn.clientTurnId || turn.turnId || (userCard?.id ? `turn-${userCard.id}` : "")) || "turn-single-1";
}

function resolveFixtureUpdatedAt(runtime = {}, cards = []) {
  const turn = runtime.turn || {};
  const userCard = latestUserCard(cards);
  return compactText(turn.startedAt || userCard?.updatedAt || userCard?.createdAt) || "2026-04-16T10:00:00Z";
}

function normalizeActivityLabel(item, fields = []) {
  if (typeof item === "string") return compactText(item);
  for (const field of fields) {
    const label = compactText(item?.[field]);
    if (label) return label;
  }
  return compactText(item);
}

function isApprovalCard(card = {}) {
  return card?.type === "CommandApprovalCard" || card?.type === "FileChangeApprovalCard";
}

function createProcessBlock({ id, kind = "tool", status = "completed", text, command, updatedAt }) {
  return {
    id,
    kind,
    status,
    text: compactText(text || command || id),
    command: compactText(command),
    updatedAt,
  };
}

function createActivityProcessBlocks({ runtime = {}, cards = [], updatedAt = "2026-04-16T10:00:00Z" } = {}) {
  const activity = runtime.activity || {};
  const turn = runtime.turn || {};
  const phase = compactText(turn.phase || "idle").toLowerCase();
  const active = Boolean(turn.active);
  const finalizing = phase === "finalizing";
  const blocks = [];
  const seen = new Set();

  const appendBlock = ({ key, kind = "tool", text, command, status = "completed" }) => {
    const label = compactText(text || command);
    if (!label || seen.has(key)) return;
    seen.add(key);
    blocks.push(createProcessBlock({ id: key, kind, status, text: label, command, updatedAt }));
  };

  (Array.isArray(activity.searchedWebQueries) ? activity.searchedWebQueries : []).forEach((item, index) => {
    const query = normalizeActivityLabel(item, ["query", "label", "text"]);
    appendBlock({ key: `activity-web-${index}`, kind: "search", text: query, command: "web_search" });
  });

  (Array.isArray(activity.searchedContentQueries) ? activity.searchedContentQueries : []).forEach((item, index) => {
    const query = normalizeActivityLabel(item, ["query", "label", "text"]);
    appendBlock({ key: `activity-content-${index}`, kind: "search", text: query, command: "search_files" });
  });

  (Array.isArray(activity.viewedFiles) ? activity.viewedFiles : []).forEach((item, index) => {
    const target = normalizeActivityLabel(item, ["path", "url", "label", "text"]);
    appendBlock({ key: `activity-view-${index}`, kind: "file", text: target, command: "open_page" });
  });

  cards.forEach((card, index) => {
    if (card?.type === "PlanCard") {
      blocks.push({
        id: compactText(card.id) || `plan-${index}`,
        kind: "plan",
        status: "running",
        text: compactText(card.title || card.text || "执行计划"),
        steps: (Array.isArray(card.items) ? card.items : []).map((item, stepIndex) => ({
          id: `${card.id || "plan"}:step:${stepIndex}`,
          text: compactText(item?.step || item?.text || item),
          status: compactText(item?.status),
        })),
        updatedAt: compactText(card.updatedAt || updatedAt),
      });
    }
    if (card?.type === "CommandCard") {
      appendBlock({
        key: compactText(card.id) || `command-${index}`,
        kind: "command",
        text: compactText(card.summary || card.title || card.command),
        command: compactText(card.command || card.title),
      });
    }
    if (card?.type === "ProcessLineCard") {
      appendBlock({
        key: compactText(card.id) || `process-${index}`,
        kind: "tool",
        text: compactText(card.summary || card.text || card.title),
        command: compactText(card.title),
        status: card.status === "inProgress" ? "running" : "completed",
      });
    }
  });

  const currentQuery = compactText(activity.currentSearchQuery || activity.currentWebSearchQuery);
  const currentStatus = active && currentQuery && !finalizing ? "running" : "completed";
  if (currentQuery) {
    appendBlock({ key: "activity-current-search", kind: "search", text: currentQuery, command: "web_search", status: currentStatus });
  }

  return blocks;
}

function createPendingApprovals({ approvals = [], cards = [], turnId = "", updatedAt = "2026-04-16T10:00:00Z" } = {}) {
  const rows = {};
  const pendingApprovals = (Array.isArray(approvals) ? approvals : []).filter((approval) => approval?.status === "pending");
  const pendingCards = cards.filter((card) => card?.status === "pending" && isApprovalCard(card));

  const appendApproval = (approval = {}, card = {}) => {
    const id = compactText(approval.id || card?.approval?.requestId || card?.id);
    if (!id || rows[id]) return;
    rows[id] = {
      id,
      turnId,
      type: compactText(approval.type || approval.approvalType || (card?.type === "FileChangeApprovalCard" ? "file_change" : "operation")),
      status: "pending",
      command: compactText(card?.command || card?.title || approval.command),
      reason: compactText(card?.text || card?.summary || approval.reason || card?.command),
      requestedAt: compactText(card?.createdAt || approval.requestedAt || updatedAt),
    };
  };

  pendingApprovals.forEach((approval) => {
    const card = cards.find((item) => item?.id === approval.itemId || item?.approval?.requestId === approval.id) || {};
    appendApproval(approval, card);
  });
  pendingCards.forEach((card) => appendApproval({}, card));
  return rows;
}

function createFixtureTransportState({ sessionId, threadId, status, cards = [], runtime = {}, approvals = [], finalText = "" }) {
  const updatedAt = resolveFixtureUpdatedAt(runtime, cards);
  const turn = runtime.turn || {};
  const turnId = resolveFixtureTurnId(turn, cards);
  const userCard = latestUserCard(cards);
  const pendingApprovals = createPendingApprovals({ approvals, cards, turnId, updatedAt });
  const process = createActivityProcessBlocks({ runtime, cards, updatedAt });
  const phase = compactText(turn.phase || "idle").toLowerCase();
  const blocked = Object.keys(pendingApprovals).length > 0 || phase === "waiting_approval" || phase === "waiting_input";
  const failed = phase === "failed";
  const canceled = phase === "aborted" || phase === "canceled";
  const active = Boolean(turn.active) && !failed && !canceled;
  const resolvedStatus = status || (blocked ? "blocked" : failed ? "failed" : canceled ? "canceled" : active ? "working" : "idle");
  const turnStatus = blocked ? "blocked" : failed ? "failed" : canceled ? "canceled" : active ? "working" : "completed";
  const assistantText = finalText || [...cards].reverse().find((card) => card?.type === "AssistantMessageCard")?.text || "";

  return {
    schemaVersion: "aiops.transport.v1",
    sessionId,
    threadId,
    status: resolvedStatus,
    currentTurnId: turnId,
    turns: {
      [turnId]: {
        id: turnId,
        status: turnStatus,
        startedAt: compactText(userCard?.createdAt || updatedAt),
        user: userCard
          ? {
              id: compactText(userCard.id) || `${turnId}:user`,
              text: compactText(userCard.text || userCard.message),
              createdAt: compactText(userCard.createdAt || updatedAt),
            }
          : undefined,
        intent: { text: compactText(userCard?.text || userCard?.message), status: phase || resolvedStatus },
        process,
        final: assistantText
          ? {
              id: `${turnId}:final`,
              text: assistantText,
              status: active || blocked ? "running" : failed ? "failed" : "completed",
            }
          : undefined,
      },
    },
    turnOrder: [turnId],
    pendingApprovals,
    mcpSurfaces: {},
    artifacts: {},
    runtimeLiveness: {
      activeTurns: active || blocked ? { [turnId]: true } : {},
      activeAgents: active ? { "agent-main": true } : {},
      pendingApprovals: Object.fromEntries(Object.keys(pendingApprovals).map((id) => [id, true])),
      pendingUserInputs: phase === "waiting_input" ? { [turnId]: true } : {},
      activeCommandStreams: Object.fromEntries(process.filter((block) => block.status === "running").map((block) => [block.id, true])),
    },
    seq: process.length + Object.keys(pendingApprovals).length + 1,
    updatedAt,
  };
}

export function createChatFixtureState(overrides = {}) {
  const defaultCards = [
    {
      id: "user-main-1",
      type: "UserMessageCard",
      role: "user",
      text: "帮我看下 nginx 中间件的状态，并给我一个处理建议。",
      createdAt: "2026-04-03T10:00:00Z",
      updatedAt: "2026-04-03T10:00:00Z",
    },
    {
      id: "plan-main-1",
      type: "PlanCard",
      items: [
        { step: "收集 nginx 错误日志", status: "running" },
        { step: "核对 upstream timeout", status: "pending" },
      ],
      createdAt: "2026-04-03T10:00:02Z",
      updatedAt: "2026-04-03T10:00:02Z",
    },
    {
      id: "cmd-main-1",
      type: "CommandCard",
      title: "journalctl -u nginx --since '-10m'",
      summary: "采集最近 10 分钟 nginx 日志",
      output: "upstream timeout for service-a",
      createdAt: "2026-04-03T10:00:10Z",
      updatedAt: "2026-04-03T10:00:10Z",
    },
  ];
  const defaultRuntime = {
    turn: { active: true, phase: "thinking", hostId: "web-01" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
    activity: {
      viewedFiles: [],
      searchedWebQueries: [{ query: "nginx upstream timeout latest status" }],
      searchedContentQueries: [],
      currentSearchQuery: "nginx upstream timeout latest status",
      currentSearchKind: "web",
    },
  };
  const cards = overrides.cards || defaultCards;
  const runtime = overrides.runtime || defaultRuntime;
  const approvals = overrides.approvals || [];
  const state = createFixtureTransportState({
    sessionId: overrides.sessionId || "single-1",
    threadId: overrides.threadId || "single-1",
    status: overrides.status,
    cards,
    runtime,
    approvals,
    finalText: overrides.finalText || "",
  });
  return {
    ...state,
    kind: "single_host",
    selectedHostId: "web-01",
    auth: { connected: true, pending: false, planType: "plus" },
    hosts: createBaseHosts(),
    approvals,
    cards,
    runtime,
    lastActivityAt: "2026-04-03T10:00:10Z",
    config: { codexAlive: true },
    ...overrides,
  };
}

export function createChatFixtureSessions(overrides = {}) {
  return {
    activeSessionId: "single-1",
    sessions: [
      {
        id: "single-1",
        kind: "single_host",
        title: "Nginx chat",
        status: "running",
        messageCount: 1,
        preview: "帮我看下 nginx 中间件的状态，并给我一个处理建议。",
        selectedHostId: "web-01",
        lastActivityAt: "2026-04-03T10:00:10Z",
      },
    ],
    ...overrides,
  };
}

export function createProtocolFixtureState(overrides = {}) {
  const cards = overrides.cards || [
    {
      id: "user-1",
      type: "UserMessageCard",
      role: "user",
      text: "我想知道 nginx 中间件的情况，最好直接给我相关工作台。",
      createdAt: "2026-04-03T11:00:00Z",
      updatedAt: "2026-04-03T11:00:00Z",
    },
    {
      id: "assistant-1",
      type: "AssistantMessageCard",
      role: "assistant",
      text: "好的，我已经接管任务，正在为您编排执行计划。",
      createdAt: "2026-04-03T11:00:10Z",
      updatedAt: "2026-04-03T11:00:10Z",
    },
    {
      id: "workspace-plan-1",
      type: "PlanCard",
      title: "nginx 巡检计划",
      text: "巡检计划已生成，准备派发到 host-agent。",
      items: [
        { step: "web-01 [task-1] 采集 nginx 错误日志", status: "running" },
        { step: "web-02 [task-2] 执行 systemctl reload nginx", status: "waiting_approval" },
      ],
      createdAt: "2026-04-03T11:00:20Z",
      updatedAt: "2026-04-03T11:00:20Z",
    },
    {
      id: "process-web-01",
      type: "ProcessLineCard",
      title: "web-01",
      text: "正在分析 nginx 错误日志",
      summary: "采集错误日志并回传摘要",
      status: "inProgress",
      hostId: "web-01",
      createdAt: "2026-04-03T11:00:30Z",
      updatedAt: "2026-04-03T11:00:30Z",
    },
    {
      id: "approval-card-1",
      type: "CommandApprovalCard",
      status: "pending",
      hostId: "web-02",
      text: "需要批准 web-02 reload nginx",
      command: "systemctl reload nginx",
      approval: { requestId: "approval-1", decisions: ["accept", "accept_session", "decline"] },
      createdAt: "2026-04-03T11:00:40Z",
      updatedAt: "2026-04-03T11:00:40Z",
    },
  ];
  const runtime = overrides.runtime || {
    turn: { active: true, phase: "waiting_approval", hostId: "server-local" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
    activity: {},
  };
  const approvals = overrides.approvals || [{ id: "approval-1", status: "pending", itemId: "approval-card-1" }];
  const state = createFixtureTransportState({
    sessionId: overrides.sessionId || "workspace-1",
    threadId: overrides.threadId || "workspace-1",
    status: overrides.status,
    cards,
    runtime,
    approvals,
  });
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "server-local",
    auth: { connected: true, pending: false, planType: "plus" },
    hosts: createBaseHosts(),
    approvals,
    cards,
    runtime,
    lastActivityAt: "2026-04-03T11:00:40Z",
    config: { codexAlive: true },
    ...overrides,
  };
}

export function createProtocolFixtureSessions(overrides = {}) {
  return {
    activeSessionId: "workspace-1",
    sessions: [
      {
        id: "workspace-1",
        kind: "workspace",
        title: "Nginx workspace",
        status: "running",
        messageCount: 5,
        preview: "我想知道 nginx 中间件的情况",
        selectedHostId: "server-local",
        lastActivityAt: "2026-04-03T11:00:40Z",
      },
    ],
    ...overrides,
  };
}

export function resolveUiFixturePreset(key = "") {
  switch (String(key || "").trim().toLowerCase()) {
    case "chat":
    case "chat-fixture":
      return {
        name: "chat",
        state: createChatFixtureState(),
        sessions: createChatFixtureSessions(),
      };
    case "protocol":
    case "workspace":
    case "protocol-fixture":
      return {
        name: "protocol",
        state: createProtocolFixtureState(),
        sessions: createProtocolFixtureSessions(),
      };
    default:
      return null;
  }
}

export function cloneUiFixturePayload(payload = null) {
  if (!payload || typeof payload !== "object") return null;
  try {
    return JSON.parse(JSON.stringify(payload));
  } catch {
    return payload;
  }
}
