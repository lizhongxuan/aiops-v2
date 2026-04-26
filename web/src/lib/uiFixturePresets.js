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

function createFixtureToolRow({ id, turnId, title, summary, status = "completed", updatedAt, seq }) {
  return {
    id,
    kind: "tool",
    turnId,
    toolCallId: id,
    title,
    summary,
    status,
    visibility: "secondary",
    updatedAt,
    seq,
  };
}

function createChatFixtureActivityRows({ runtime = {}, cards = [], turnId = "" } = {}) {
  const activity = runtime.activity || {};
  const turn = runtime.turn || {};
  const phase = compactText(turn.phase || "idle").toLowerCase();
  const active = Boolean(turn.active);
  const finalizing = phase === "finalizing";
  const updatedAt = resolveFixtureUpdatedAt(runtime, cards);
  const rows = [];
  const seen = new Set();
  let seq = 2;

  const appendTool = ({ key, title, summary, status = "completed" }) => {
    const label = compactText(summary);
    if (!label || seen.has(key)) return;
    seen.add(key);
    rows.push(createFixtureToolRow({
      id: key,
      turnId,
      title,
      summary: label,
      status,
      updatedAt,
      seq: seq++,
    }));
  };

  const currentQuery = compactText(activity.currentSearchQuery || activity.currentWebSearchQuery);
  const currentKind = compactText(activity.currentSearchKind || (activity.currentWebSearchQuery ? "web" : "")).toLowerCase();
  const currentTitle = currentKind === "content" ? "search_files" : "web_search";
  const currentStatus = active && currentQuery && !finalizing ? "running" : "completed";

  (Array.isArray(activity.searchedWebQueries) ? activity.searchedWebQueries : []).forEach((item, index) => {
    const query = normalizeActivityLabel(item, ["query", "label", "text"]);
    if (!query) return;
    if (currentQuery && query === currentQuery && currentStatus === "running") return;
    appendTool({ key: `activity-web-${index}`, title: "web_search", summary: query, status: "completed" });
  });

  (Array.isArray(activity.searchedContentQueries) ? activity.searchedContentQueries : []).forEach((item, index) => {
    const query = normalizeActivityLabel(item, ["query", "label", "text"]);
    if (!query) return;
    if (currentQuery && query === currentQuery && currentStatus === "running") return;
    appendTool({ key: `activity-content-${index}`, title: "search_files", summary: query, status: "completed" });
  });

  (Array.isArray(activity.viewedFiles) ? activity.viewedFiles : []).forEach((item, index) => {
    const target = normalizeActivityLabel(item, ["path", "url", "label", "text"]);
    appendTool({ key: `activity-view-${index}`, title: "open_page", summary: target, status: "completed" });
  });

  if (currentQuery) {
    appendTool({
      key: "activity-current-search",
      title: currentTitle,
      summary: currentQuery,
      status: currentStatus,
    });
  }

  return rows;
}

function isApprovalCard(card = {}) {
  return card?.type === "CommandApprovalCard" || card?.type === "FileChangeApprovalCard";
}

function createChatFixtureApprovalRows({ approvals = [], cards = [], updatedAt = "2026-04-16T10:00:00Z" } = {}) {
  const rows = [];
  const seen = new Set();
  const pendingApprovals = (Array.isArray(approvals) ? approvals : []).filter((approval) => approval?.status === "pending");
  const pendingCards = cards.filter((card) => card?.status === "pending" && isApprovalCard(card));

  const appendApproval = (approval = {}, card = {}) => {
    const id = compactText(approval.id || card?.approval?.requestId || card?.id);
    if (!id || seen.has(id)) return;
    seen.add(id);
    rows.push({
      id,
      approvalType: compactText(approval.type || approval.approvalType || (card?.type === "FileChangeApprovalCard" ? "file_change" : "operation")),
      title: compactText(card?.command || card?.title || card?.text || approval.title || "待确认操作"),
      reason: compactText(card?.text || card?.summary || approval.reason || card?.command),
      risk: compactText(approval.risk || card?.risk),
      targets: [approval.hostId || card?.hostId || card?.target].filter(Boolean),
      status: "blocked",
      updatedAt: compactText(card?.updatedAt || card?.createdAt || approval.requestedAt || approval.updatedAt) || updatedAt,
    });
  };

  pendingApprovals.forEach((approval) => {
    const card = cards.find((item) => item?.id === approval.itemId || item?.approval?.requestId === approval.id) || {};
    appendApproval(approval, card);
  });
  pendingCards.forEach((card) => appendApproval({}, card));

  return rows;
}

function createChatFixtureAgentEventProjection({ sessionId = "single-1", runtime = {}, cards = [], approvals = [] } = {}) {
  const turn = runtime.turn || {};
  const phase = String(turn.phase || "idle").trim().toLowerCase();
  const active = Boolean(turn.active) && !["idle", "completed", "failed", "aborted", "canceled"].includes(phase);
  const blocked = phase === "waiting_approval" || phase === "waiting_input";
  const canceled = phase === "aborted" || phase === "canceled";
  const failed = phase === "failed";
  const userCard = latestUserCard(cards);
  const turnId = resolveFixtureTurnId(turn, cards);
  const updatedAt = resolveFixtureUpdatedAt(runtime, cards);
  const activityRows = createChatFixtureActivityRows({ runtime, cards, turnId });
  const approvalRows = createChatFixtureApprovalRows({ approvals, cards, updatedAt });
  const hasProjectionPayload = active || blocked || canceled || failed || activityRows.length || approvalRows.length;
  if (!hasProjectionPayload) return null;
  const status = approvalRows.length || blocked ? "blocked" : failed ? "failed" : canceled ? "canceled" : active ? "working" : "idle";
  const turnRow = userCard
    ? {
        id: userCard.clientMessageId || userCard.id,
        kind: "turn",
        turnId,
        title: userCard.text || userCard.message || "",
        summary: canceled ? "已停止生成" : failed ? "请求失败" : approvalRows.length || blocked ? "等待处理" : active ? "正在执行" : "已完成",
        status: canceled ? "canceled" : failed ? "failed" : active || blocked || approvalRows.length ? "running" : "completed",
        visibility: "primary",
        updatedAt: userCard.updatedAt || userCard.createdAt || updatedAt,
        seq: 1,
      }
    : null;
  const timeline = [
    ...(turnRow ? [turnRow] : []),
    ...activityRows,
  ];
  return {
    sessionId,
    currentTurnId: turnId,
    status,
    phase,
    lastSeq: timeline.length,
    runtimeLiveness: {
      activeTurns: active || blocked || approvalRows.length ? { [turnId]: true } : {},
      activeAgents: active ? { "agent-main": true } : {},
      pendingApprovals: Object.fromEntries(approvalRows.map((approval) => [approval.id, true])),
      pendingUserInputs: phase === "waiting_input" ? { [turnId]: true } : {},
      activeCommandStreams: Object.fromEntries(activityRows.filter((row) => row.status === "running").map((row) => [row.toolCallId, true])),
    },
    timeline,
    agents: active
      ? [{
          id: "agent-main",
          handle: "main",
          name: "Main Agent",
          role: "assistant",
          status: "running",
          lastAction: "正在处理当前请求",
          updatedAt,
          stats: { commandsRun: Number(runtime.activity?.commandsRun || 0), filesRead: 0, filesChanged: 0, toolsCalled: 0 },
        }]
      : [],
    approvals: approvalRows,
    artifacts: [],
    diff: null,
    finalMessages: {},
    processGroups: activityRows.length ? { [turnId]: activityRows } : {},
    clientTurnMap: {},
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
  const hasAgentEventProjectionOverride = Object.prototype.hasOwnProperty.call(overrides, "agentEventProjection");
  return {
    sessionId: "single-1",
    kind: "single_host",
    selectedHostId: "web-01",
    auth: { connected: true, pending: false, planType: "plus" },
    hosts: createBaseHosts(),
    approvals,
    cards,
    runtime,
    agentEventProjection: hasAgentEventProjectionOverride
      ? overrides.agentEventProjection
      : createChatFixtureAgentEventProjection({ sessionId: overrides.sessionId || "single-1", runtime, cards, approvals }),
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

export function createProtocolFixtureAgentEventProjection(overrides = {}) {
  return {
    sessionId: "workspace-1",
    currentTurnId: "turn-workspace-1",
    status: "blocked",
    lastSeq: 7,
    runtimeLiveness: {
      activeTurns: { "turn-workspace-1": true },
      activeAgents: { "agent-main": true, "agent-web-01": true },
      pendingApprovals: { "approval-1": true },
      pendingUserInputs: {},
      activeCommandStreams: { "tool-nginx-log": true },
    },
    timeline: [
      {
        id: "turn-workspace-1",
        kind: "turn",
        turnId: "turn-workspace-1",
        title: "我想知道 nginx 中间件的情况，最好直接给我相关工作台。",
        summary: "正在等待 Agent 启动",
        status: "queued",
        visibility: "primary",
        updatedAt: "2026-04-03T11:00:00Z",
        seq: 1,
      },
      {
        id: "plan-nginx",
        kind: "agent",
        turnId: "turn-workspace-1",
        agentId: "agent-main",
        title: "生成 nginx 巡检计划",
        summary: "已将巡检拆给 web-01 和 web-02",
        status: "completed",
        visibility: "secondary",
        updatedAt: "2026-04-03T11:00:20Z",
        seq: 2,
      },
      {
        id: "tool-nginx-log",
        kind: "tool",
        turnId: "turn-workspace-1",
        agentId: "agent-web-01",
        toolCallId: "tool-nginx-log",
        title: "readonly_host_inspect",
        summary: "采集 web-01 nginx 错误日志",
        status: "running",
        visibility: "secondary",
        updatedAt: "2026-04-03T11:00:30Z",
        seq: 3,
      },
      {
        id: "approval-1",
        kind: "approval",
        turnId: "turn-workspace-1",
        agentId: "agent-web-02",
        title: "等待 reload 审批",
        summary: "systemctl reload nginx",
        status: "blocked",
        visibility: "primary",
        updatedAt: "2026-04-03T11:00:40Z",
        seq: 4,
      },
    ],
    agents: [
      {
        id: "agent-main",
        handle: "main",
        name: "Main Agent",
        role: "orchestrator",
        status: "running",
        lastAction: "编排 nginx 巡检计划",
        updatedAt: "2026-04-03T11:00:25Z",
        stats: { commandsRun: 0, filesRead: 0, filesChanged: 0, toolsCalled: 1 },
      },
      {
        id: "agent-web-01",
        handle: "web-01",
        name: "web-01",
        role: "host-agent",
        status: "running",
        lastAction: "采集 nginx 错误日志",
        updatedAt: "2026-04-03T11:00:30Z",
        stats: { commandsRun: 1, filesRead: 0, filesChanged: 0, toolsCalled: 1 },
      },
      {
        id: "agent-web-02",
        handle: "web-02",
        name: "web-02",
        role: "host-agent",
        status: "blocked",
        lastAction: "等待 reload 审批",
        updatedAt: "2026-04-03T11:00:40Z",
        stats: { commandsRun: 0, filesRead: 0, filesChanged: 0, toolsCalled: 0 },
      },
    ],
    approvals: [
      {
        id: "approval-1",
        approvalType: "operation",
        title: "批准 web-02 reload nginx",
        reason: "systemctl reload nginx",
        risk: "reload 前需要确认错误日志采集完成，避免掩盖现场。",
        targets: ["web-02"],
        status: "blocked",
        updatedAt: "2026-04-03T11:00:40Z",
      },
    ],
    artifacts: [],
    diff: null,
    finalMessages: {},
    processGroups: {
      "turn-workspace-1": [
        {
          id: "tool-nginx-log",
          kind: "tool",
          turnId: "turn-workspace-1",
          agentId: "agent-web-01",
          toolCallId: "tool-nginx-log",
          title: "readonly_host_inspect",
          summary: "采集 web-01 nginx 错误日志",
          status: "running",
          visibility: "secondary",
          updatedAt: "2026-04-03T11:00:30Z",
          seq: 3,
        },
      ],
    },
    clientTurnMap: {},
    ...overrides,
  };
}

export function createProtocolFixtureState(overrides = {}) {
  return {
    sessionId: "workspace-1",
    kind: "workspace",
    selectedHostId: "server-local",
    auth: { connected: true, pending: false, planType: "plus" },
    hosts: createBaseHosts(),
    approvals: [{ id: "approval-1", status: "pending", itemId: "approval-card-1" }],
    cards: [
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
        detail: {
          goal: "帮我执行一轮全网 nginx 巡检，重点关注错误日志。",
          version: "plan-v3",
          structured_process: [
            "task-1 [running] @web-01 采集 nginx 错误日志",
            "task-2 [waiting_approval] @web-02 执行 systemctl reload nginx",
          ],
          task_host_bindings: [
            { taskId: "task-1", hostId: "web-01", status: "running", title: "采集 nginx 错误日志" },
            { taskId: "task-2", hostId: "web-02", status: "waiting_approval", title: "执行 systemctl reload nginx" },
          ],
        },
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
        id: "process-web-02",
        type: "ProcessLineCard",
        title: "web-02",
        text: "等待 reload 审批",
        summary: "执行 systemctl reload nginx",
        status: "inProgress",
        hostId: "web-02",
        createdAt: "2026-04-03T11:00:35Z",
        updatedAt: "2026-04-03T11:00:35Z",
      },
      {
        id: "approval-card-1",
        type: "CommandApprovalCard",
        status: "pending",
        hostId: "web-02",
        text: "需要批准 web-02 reload nginx",
        command: "systemctl reload nginx",
        approval: {
          requestId: "approval-1",
          decisions: ["accept", "accept_session", "decline"],
        },
        createdAt: "2026-04-03T11:00:40Z",
        updatedAt: "2026-04-03T11:00:40Z",
      },
    ],
    runtime: {
      turn: { active: true, phase: "waiting_approval", hostId: "server-local" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: {},
    },
    agentEventProjection: createProtocolFixtureAgentEventProjection(overrides.agentEventProjection || {}),
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
