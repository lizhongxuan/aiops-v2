function createBaseHosts() {
  return [
    { id: "web-01", name: "web-01", status: "online", executable: true, terminalCapable: true },
    { id: "web-02", name: "web-02", status: "online", executable: true, terminalCapable: true },
    { id: "server-local", name: "server-local", status: "online", executable: true, terminalCapable: true },
  ];
}

export function createChatFixtureState(overrides = {}) {
  return {
    sessionId: "single-1",
    kind: "single_host",
    selectedHostId: "web-01",
    auth: { connected: true, pending: false, planType: "plus" },
    hosts: createBaseHosts(),
    approvals: [],
    cards: [
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
    ],
    runtime: {
      turn: { active: true, phase: "thinking", hostId: "web-01" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: {
        viewedFiles: [],
        searchedWebQueries: [{ query: "nginx upstream timeout latest status" }],
        searchedContentQueries: [],
        currentSearchQuery: "nginx upstream timeout latest status",
        currentSearchKind: "web",
      },
    },
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
