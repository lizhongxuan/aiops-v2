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
    schemaVersion: "aiops.transport.v2",
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

export function createHostOpsThreeHostsFixtureState(overrides = {}) {
  const now = "2026-06-04T10:00:00Z";
  const state = createChatFixtureState({
    sessionId: "hostops-three-hosts",
    threadId: "hostops-three-hosts",
    status: "working",
    cards: [
      {
        id: "user-hostops-three-hosts",
        type: "UserMessageCard",
        role: "user",
        text: "@1.1.1.1和@1.1.1.2作为pg节点,搭建一个主从集群,@1.1.1.3作为pg_mon.",
        createdAt: now,
        updatedAt: now,
      },
    ],
    runtime: {
      turn: { active: true, phase: "thinking", hostId: "workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
  });
  const turnId = state.currentTurnId;
  state.turns[turnId] = {
    ...state.turns[turnId],
    status: "working",
    startedAt: now,
    updatedAt: "2026-06-04T10:00:02Z",
    process: [
      {
        id: "hostops-plan",
        kind: "plan",
        displayKind: "plan",
        status: "running",
        text: "PostgreSQL 主从集群计划",
        steps: [
          { id: "confirm", text: "确认三台主机角色和运行态入口", status: "pending" },
          { id: "precheck", text: "补充失败测试覆盖剩余条目和结果合入口", status: "pending" },
          { id: "primary", text: "初始化 @1.1.1.1 PostgreSQL 主库", status: "pending" },
          { id: "standby", text: "配置 @1.1.1.2 PostgreSQL 从库复制", status: "pending" },
          { id: "monitor", text: "部署 @1.1.1.3 pg_mon 并验证", status: "pending" },
        ],
        updatedAt: now,
      },
      {
        id: "hostops-subagents",
        kind: "subagent",
        displayKind: "hostops.spawn_host_agent",
        status: "running",
        text: "3 个 host-bound 子 Agent 已启动",
        updatedAt: "2026-06-04T10:00:02Z",
      },
    ],
  };
  state.activeHostMissionId = "mission-1";
  state.hostMissions = {
    "mission-1": {
      id: "mission-1",
      turnId,
      status: "running",
      planRequired: true,
      planAccepted: true,
      mentionedHosts: [
        { tokenId: "mention-1", raw: "@1.1.1.1", hostId: "host-a", address: "1.1.1.1", displayName: "@1.1.1.1", source: "inventory", resolved: true },
        { tokenId: "mention-2", raw: "@1.1.1.2", hostId: "host-b", address: "1.1.1.2", displayName: "@1.1.1.2", source: "inventory", resolved: true },
        { tokenId: "mention-3", raw: "@1.1.1.3", hostId: "host-c", address: "1.1.1.3", displayName: "@1.1.1.3", source: "inventory", resolved: true },
      ],
      childAgentIds: ["child-1", "child-2", "child-3"],
      planSteps: state.turns[turnId].process[0].steps,
      managerAgentId: "manager-1",
      activeChildAgentId: "child-1",
      createdAt: now,
      updatedAt: "2026-06-04T10:00:02Z",
    },
  };
  state.childAgents = {
    "child-1": {
      id: "child-1",
      missionId: "mission-1",
      parentAgentId: "manager-1",
      sessionId: "host-child:mission-1:host-a",
      hostId: "host-a",
      hostAddress: "1.1.1.1",
      hostDisplayName: "@1.1.1.1",
      role: "pg primary",
      task: "初始化 PostgreSQL 主库",
      status: "running",
      startedAt: "2026-06-04T10:00:01Z",
      updatedAt: "2026-06-04T10:00:02Z",
    },
    "child-2": {
      id: "child-2",
      missionId: "mission-1",
      parentAgentId: "manager-1",
      sessionId: "host-child:mission-1:host-b",
      hostId: "host-b",
      hostAddress: "1.1.1.2",
      hostDisplayName: "@1.1.1.2",
      role: "pg standby",
      task: "配置 PostgreSQL 从库复制",
      status: "running",
      startedAt: "2026-06-04T10:00:01Z",
      updatedAt: "2026-06-04T10:00:02Z",
    },
    "child-3": {
      id: "child-3",
      missionId: "mission-1",
      parentAgentId: "manager-1",
      sessionId: "host-child:mission-1:host-c",
      hostId: "host-c",
      hostAddress: "1.1.1.3",
      hostDisplayName: "@1.1.1.3",
      role: "pg_mon",
      task: "部署 pg_mon",
      status: "waiting",
      startedAt: "2026-06-04T10:00:01Z",
      updatedAt: "2026-06-04T10:00:02Z",
    },
  };
  state.runtimeLiveness = {
    ...state.runtimeLiveness,
    activeTurns: { [turnId]: true },
    activeAgents: { "manager-1": true, "child-1": true, "child-2": true, "child-3": true },
  };
  state.hostOpsTranscripts = {
    "child-1": {
      childAgentId: "child-1",
      items: [
        { id: "child-1-item-1", type: "manager_message", content: "检查PG版本", status: "completed", createdAt: "2026-06-04T10:00:03Z" },
        { id: "child-1-item-2", type: "assistant_message", content: "PostgreSQL 15 已检测到", status: "completed", createdAt: "2026-06-04T10:00:04Z" },
      ],
    },
  };
  return { ...state, ...overrides };
}

export function createHostOpsThreeHostsFixtureSessions(overrides = {}) {
  return {
    activeSessionId: "hostops-three-hosts",
    sessions: [
      {
        id: "hostops-three-hosts",
        kind: "single_host",
        title: "@主机 PostgreSQL 集群",
        status: "running",
        messageCount: 1,
        preview: "@1.1.1.1 和 @1.1.1.2 作为 pg 节点",
        selectedHostId: "workspace",
        lastActivityAt: "2026-06-04T10:00:02Z",
      },
    ],
    ...overrides,
  };
}

export function createContextCompactionFixtureState(overrides = {}) {
  const now = "2026-05-22T08:00:00Z";
  const state = createChatFixtureState({
    sessionId: "context-compaction-session",
    threadId: "context-compaction-session",
    status: "idle",
    cards: [
      {
        id: "user-context-compaction",
        type: "UserMessageCard",
        role: "user",
        text: "继续排查 nginx 超时，并保留关键摘要。",
        createdAt: now,
        updatedAt: now,
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "web-01" },
      codex: { status: "connected", retryAttempt: 1, retryMax: 3 },
      activity: {
        viewedFiles: [],
        searchedWebQueries: [],
        searchedContentQueries: [],
      },
    },
    finalText: "我正在整理旧上下文，关键摘要会保留在当前对话里。",
  });
  const turnId = state.currentTurnId;
  state.turns[turnId] = {
    ...state.turns[turnId],
    status: "completed",
    startedAt: now,
    completedAt: "2026-05-22T08:00:04Z",
    updatedAt: now,
    contextGovernance: [
      {
        id: "ctxgov-fixture-l4",
        layer: "L4",
        kind: "context.compaction.started",
        message: "正在压缩上下文，当前任务会继续",
        budget: {
          maxContextTokens: 20000,
          warningThreshold: 13260,
          autoCompactThreshold: 14960,
          blockingLimit: 15980,
          smallContextMode: true,
        },
        referenceIds: ["spill-1"],
        createdAt: now,
      },
      {
        id: "ctxgov-fixture-l5",
        layer: "L5",
        kind: "context.compaction.failed",
        message: "上下文过长，已使用本地摘要继续",
        createdAt: "2026-05-22T08:00:02Z",
      },
    ],
    process: [
      {
        id: "tool-context-spill",
        kind: "tool",
        displayKind: "logs_query",
        status: "completed",
        text: "logs_query nginx timeout",
        outputPreview: "Large nginx log result was externalized. Summary: 17 upstream timeout lines.",
        evidenceRefs: ["spill-1"],
        materializationTier: "large",
        originalBytes: 48213,
        inlineBytes: 920,
        externalReferences: [
          {
            id: "spill-1",
            kind: "blob",
            title: "nginx raw timeout logs",
            summary: "17 upstream timeout lines from nginx in the last 10 minutes.",
            contentType: "text/plain",
            bytes: 48213,
          },
        ],
        updatedAt: "2026-05-22T08:00:03Z",
      },
      {
        id: "process-context-compact",
        kind: "system",
        displayKind: "context.compaction",
        status: "completed",
        text: "已保留当前目标、审批状态和关键上下文。",
        updatedAt: "2026-05-22T08:00:04Z",
      },
    ],
    final: state.turns[turnId]?.final
      ? {
          ...state.turns[turnId].final,
          status: "completed",
        }
      : state.turns[turnId]?.final,
  };
  return {
    ...state,
    ...overrides,
  };
}

export function createContextCompactionFixtureSessions(overrides = {}) {
  return {
    activeSessionId: "context-compaction-session",
    sessions: [
      {
        id: "context-compaction-session",
        kind: "single_host",
        title: "Context compaction",
        status: "completed",
        messageCount: 1,
        preview: "继续排查 nginx 超时，并保留关键摘要。",
        selectedHostId: "web-01",
        lastActivityAt: "2026-05-22T08:00:04Z",
      },
    ],
    ...overrides,
  };
}

export function createOpsManualPreflightFixtureState(overrides = {}) {
  const state = createChatFixtureState({
    cards: [
      {
        id: "user-ops-manual-preflight",
        type: "UserMessageCard",
        role: "user",
        text: "在 Ubuntu 主机 pg-ubuntu-01 上通过 ssh 做 PostgreSQL 备份，备份到 /data/backups，已确认 ssh_access 和 pg_isready 正常",
        createdAt: "2026-05-15T10:00:00Z",
        updatedAt: "2026-05-15T10:00:00Z",
      },
      {
        id: "assistant-ops-manual-preflight",
        type: "AssistantMessageCard",
        role: "assistant",
        text: "已完成运维手册检索和 Node 0 预检。",
        createdAt: "2026-05-15T10:00:16Z",
        updatedAt: "2026-05-15T10:00:16Z",
      },
    ],
    runtime: { turn: { active: false, phase: "idle", hostId: "web-01" }, codex: { status: "connected", retryAttempt: 0, retryMax: 5 } },
    finalText: "已完成运维手册检索和 Node 0 预检。",
    sessionId: "ops-manual-preflight",
    threadId: "ops-manual-preflight",
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.agentUiArtifacts = [
    {
      id: "fixture-search-direct",
      type: "ops_manual_search_result",
      titleZh: "运维手册检索结果",
      summaryZh: "按结构化条件完成检索判定。",
      source: "tool:search_ops_manuals",
      redactionStatus: "redacted",
      inlineData: {
        decision: "direct_execute",
        summary: "找到可直接使用的运维手册，用户确认前不会执行 Runner Workflow。",
        recommended_next_action: "运行 Node 0 预检，通过后确认或审批执行。",
        operation_frame: { target: { type: "postgresql" }, operation: { action: "backup" } },
        searched_fields: ["object_type", "operation_type", "execution_surface", "environment"],
        manuals: [
          {
            manual: { id: "manual-pg-backup-ubuntu", title: "PostgreSQL 备份 Ubuntu 运维手册" },
            bound_workflow_id: "workflow-pg-backup-ubuntu",
            usable_mode: "direct_execute",
            matched_fields: ["object_type", "operation_type", "environment", "required_context"],
            recommended_action: "run_preflight_probe",
            preflight_status: "not_run",
            run_record_summary: { success_count: 6, failure_count: 0, latest_status: "passed" },
          },
        ],
      },
    },
    {
      id: "fixture-preflight-passed",
      type: "ops_manual_preflight_result",
      titleZh: "运维手册预检",
      summaryZh: "Node 0 预检完成。",
      source: "tool:run_ops_manual_preflight",
      redactionStatus: "redacted",
      inlineData: {
        status: "passed",
        ready: true,
        reason: "只读探针通过，可以确认或审批后执行。",
        manual_id: "manual-pg-backup-ubuntu",
        workflow_id: "workflow-pg-backup-ubuntu",
        probe_id: "probe-pg-backup-readonly",
        next_action: "confirm_execution",
        evidence: [
          { name: "ssh_access", status: "passed", note: "只读连接检查" },
          { name: "pg_isready", status: "passed", note: "PostgreSQL 可用性检查" },
        ],
      },
    },
  ];
  return state;
}

export function createOpsManualPreflightFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "ops-manual-preflight",
    sessions: [
      {
        id: "ops-manual-preflight",
        kind: "single_host",
        title: "Ops Manual Preflight",
        status: "running",
        messageCount: 2,
        preview: "PostgreSQL 备份运维手册预检",
        selectedHostId: "web-01",
        lastActivityAt: "2026-05-15T10:00:16Z",
      },
    ],
    ...overrides,
  });
}

export function createOpsManualFourFieldFormFixtureState(overrides = {}) {
  const state = createChatFixtureState({
    cards: [
      {
        id: "user-ops-manual-4field",
        type: "UserMessageCard",
        role: "user",
        text: "排查 Redis，但还没有补齐目标实例、环境、执行方式和现象指标",
        createdAt: "2026-05-16T10:00:00Z",
        updatedAt: "2026-05-16T10:00:00Z",
      },
      {
        id: "assistant-ops-manual-4field",
        type: "AssistantMessageCard",
        role: "assistant",
        text: "已检索到可用的 Redis 排障手册，目标位置默认使用当前选择主机，实例和访问入口将自动探测。",
        createdAt: "2026-05-16T10:00:16Z",
        updatedAt: "2026-05-16T10:00:16Z",
      },
    ],
    runtime: { turn: { active: false, phase: "idle", hostId: "server-local" }, codex: { status: "connected", retryAttempt: 0, retryMax: 5 } },
    sessionId: "ops-manual-4field-form",
    threadId: "ops-manual-4field-form",
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.agentUiArtifacts = [
    {
      id: "fixture-search-need-info-4field",
      type: "ops_manual_search_result",
      titleZh: "运维手册检索结果",
      summaryZh: "需要补充必要上下文。",
      source: "tool:search_ops_manuals",
      redactionStatus: "redacted",
      inlineData: {
        decision: "need_info",
        summary: "信息不足，不能直接使用工作流。",
        operation_frame: { target: { type: "redis" }, operation: { action: "rca_or_repair" } },
        manuals: [
          {
            manual: {
              id: "manual-redis-rca-ssh",
              title: "Redis SSH 排障运维手册",
              description: "用于 Redis SSH 场景的只读排障和恢复前验证。",
              content: "适用场景：Redis 内存压力、慢查询、连接异常。验证方式：检查 INFO memory、slowlog 和业务 p95。",
            },
            bound_workflow_id: "workflow-redis-rca-ssh",
            workflow_preview: {
              title: "Redis SSH 排障工作流",
              nodes: [
                { id: "collect", title: "采集只读指标", command: "redis-cli INFO memory", summary: "读取内存、连接数和慢查询指标。" },
                { id: "analyze", title: "判断内存压力", command: "compare used_memory_rss maxmemory", summary: "判断 RSS、maxmemory 和业务 p95 是否相关。" },
              ],
            },
          },
        ],
        next_questions: ["目标 Redis 实例是哪一个？", "环境是什么？", "执行方式是 ssh、kubectl 还是 docker exec？", "当前现象和指标是什么？"],
      },
    },
  ];
  return state;
}

export function createOpsManualFourFieldFormFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "ops-manual-4field-form",
    sessions: [
      {
        id: "ops-manual-4field-form",
        kind: "single_host",
        title: "Ops Manual Context Form",
        status: "running",
        messageCount: 2,
        preview: "Redis 排障自动探测上下文",
        selectedHostId: "server-local",
        lastActivityAt: "2026-05-16T10:00:16Z",
      },
    ],
    ...overrides,
  });
}

function createOpsManualParamResolutionBase({ sessionId, userText, artifacts, assistantText = "已检索运维手册，并开始解析必要参数。", overrides = {} }) {
  const state = createChatFixtureState({
    cards: [
      {
        id: `user-${sessionId}`,
        type: "UserMessageCard",
        role: "user",
        text: userText,
        createdAt: "2026-05-17T10:00:00Z",
        updatedAt: "2026-05-17T10:00:00Z",
      },
      {
        id: `assistant-${sessionId}`,
        type: "AssistantMessageCard",
        role: "assistant",
        text: assistantText,
        createdAt: "2026-05-17T10:00:10Z",
        updatedAt: "2026-05-17T10:00:10Z",
      },
    ],
    runtime: { turn: { active: false, phase: "idle", hostId: "server-local" }, codex: { status: "connected", retryAttempt: 0, retryMax: 5 } },
    sessionId,
    threadId: sessionId,
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.agentUiArtifacts = artifacts;
  return state;
}

function opsManualParamResolutionSearchArtifact(overrides = {}) {
  return {
    id: "fixture-search-redis-param-resolution",
    type: "ops_manual_search_result",
    titleZh: "运维手册检索结果",
    summaryZh: "按结构化条件完成检索判定。",
    source: "tool:search_ops_manuals",
    redactionStatus: "redacted",
    inlineData: {
      decision: "need_info",
      summary: "命中 Redis 排障手册，等待参数解析。",
      operation_frame: { target: { type: "redis" }, operation: { action: "rca_or_repair" } },
      manuals: [
        {
          manual: {
            id: "manual-redis-rca-ssh",
            title: "Redis SSH 排障运维手册",
            description: "用于 Redis SSH 场景的只读排障和恢复前验证。",
            content: "适用场景：Redis 内存压力、慢查询、连接异常。验证方式：检查 INFO memory、slowlog 和业务 p95。",
          },
          bound_workflow_id: "workflow-redis-rca-ssh",
          workflow_preview: {
            title: "Redis SSH 排障工作流",
            nodes: [
              { id: "collect", title: "采集只读指标", command: "redis-cli INFO memory", summary: "读取内存、连接数和慢查询指标。" },
              { id: "verify", title: "校验延迟", command: "redis-cli SLOWLOG GET 10", summary: "检查慢查询和 p95 相关性。" },
            ],
          },
        },
      ],
      ...overrides,
    },
  };
}

function opsManualParamResolutionArtifact(inlineData = {}) {
  return {
    id: `fixture-param-resolution-${inlineData.status || "resolved"}`,
    type: "ops_manual_param_resolution",
    titleZh: "运维手册参数解析",
    summaryZh: "已解析运维手册必要参数。",
    source: "tool:resolve_ops_manual_params",
    redactionStatus: "redacted",
    inlineData: {
      manual_id: "manual-redis-rca-ssh",
      workflow_id: "workflow-redis-rca-ssh",
      artifact_type: "ops_manual_param_resolution",
      ...inlineData,
    },
  };
}

function createOpsManualParamResolutionSessions(sessionId, preview, overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: sessionId,
    sessions: [
      {
        id: sessionId,
        kind: "single_host",
        title: "Ops Manual Param Resolution",
        status: "running",
        messageCount: 2,
        preview,
        selectedHostId: "server-local",
        lastActivityAt: "2026-05-17T10:00:10Z",
      },
    ],
    ...overrides,
  });
}

export function createOpsManualParamAutoRedisFixtureState(overrides = {}) {
  return createOpsManualParamResolutionBase({
    sessionId: "ops-manual-param-auto-redis",
    userText: "排查 Redis",
    artifacts: [
      opsManualParamResolutionSearchArtifact({ decision: "direct_execute", summary: "Redis 手册已匹配。" }),
      opsManualParamResolutionArtifact({
        status: "resolved",
        resolved_params: [
          { id: "target_location", value: "server-local", source: "selected_host", confidence: 1, evidence: "当前选择主机" },
          { id: "target_instance", value: "docker:aiops-redis", source: "docker_resource_resolver", confidence: 0.98, evidence: "docker ps 发现单 Redis 容器" },
          { id: "execution_surface", value: "docker exec aiops-redis", source: "docker_resource_resolver", confidence: 0.98, evidence: "容器可执行 redis-cli" },
        ],
        next_action: "run_preflight",
      }),
    ],
    overrides,
  });
}

export function createOpsManualParamAutoRedisFixtureSessions(overrides = {}) {
  return createOpsManualParamResolutionSessions("ops-manual-param-auto-redis", "Redis 参数自动补齐", overrides);
}

export function createOpsManualParamMultiRedisFixtureState(overrides = {}) {
  return createOpsManualParamResolutionBase({
    sessionId: "ops-manual-param-multi-redis",
    userText: "排查 Redis",
    artifacts: [
      opsManualParamResolutionSearchArtifact(),
      opsManualParamResolutionArtifact({
        status: "ambiguous",
        resolved_params: [
          { id: "target_location", value: "server-local", source: "selected_host", confidence: 1 },
          { id: "execution_surface", value: "docker exec", source: "docker_resource_resolver", confidence: 0.9 },
        ],
        fields: [
          {
            id: "target_instance",
            label: "Redis 实例",
            type: "resource_ref",
            required: true,
            ui_control: "select",
            candidates: [
              { value: "docker:redis-a", label: "redis-a", source: "docker", confidence: 0.91 },
              { value: "docker:redis-b", label: "redis-b", source: "docker", confidence: 0.88 },
              { value: "__manual__", label: "其他，手动填写", source: "user" },
            ],
          },
        ],
        next_action: "await_user_input",
      }),
    ],
    overrides,
  });
}

export function createOpsManualParamMultiRedisFixtureSessions(overrides = {}) {
  return createOpsManualParamResolutionSessions("ops-manual-param-multi-redis", "Redis 多实例选择", overrides);
}

export function createOpsManualParamPgBackupPathFixtureState(overrides = {}) {
  return createOpsManualParamResolutionBase({
    sessionId: "ops-manual-param-pg-backup-path",
    userText: "给 pg-01 做 PostgreSQL 备份",
    artifacts: [
      opsManualParamResolutionArtifact({
        manual_id: "manual-pg-backup-ssh",
        workflow_id: "workflow-pg-backup-ssh",
        status: "need_user_input",
        resolved_params: [
          { id: "target_host", value: "pg-01", source: "conversation_resolver", confidence: 0.95 },
          { id: "execution_surface", value: "ssh", source: "manual_default_resolver", confidence: 0.8 },
        ],
        fields: [
          {
            id: "backup_path",
            label: "备份路径",
            type: "path",
            required: true,
            ui_control: "text",
            placeholder: "/data/backups",
          },
        ],
        next_action: "await_user_input",
      }),
    ],
    overrides,
  });
}

export function createOpsManualParamPgBackupPathFixtureSessions(overrides = {}) {
  return createOpsManualParamResolutionSessions("ops-manual-param-pg-backup-path", "PG 备份路径补充", overrides);
}

export function createOpsManualParamSecretFixtureState(overrides = {}) {
  return createOpsManualParamResolutionBase({
    sessionId: "ops-manual-param-secret",
    userText: "恢复数据库，需要使用 Secret 引用",
    artifacts: [
      opsManualParamResolutionArtifact({
        manual_id: "manual-db-restore",
        workflow_id: "workflow-db-restore",
        status: "need_user_input",
        resolved_params: [
          { id: "target_host", value: "server-local", source: "selected_host", confidence: 0.95 },
        ],
        fields: [
          {
            id: "db_password",
            label: "数据库密码",
            type: "secret_ref",
            required: true,
            sensitive: true,
            ui_control: "secret_ref",
            default: "plain-secret-should-not-render",
          },
        ],
        next_action: "await_user_input",
      }),
    ],
    overrides,
  });
}

export function createOpsManualParamSecretFixtureSessions(overrides = {}) {
  return createOpsManualParamResolutionSessions("ops-manual-param-secret", "敏感参数 Secret 引用", overrides);
}

export function createOpsManualParamSkipManualFixtureState(overrides = {}) {
  return createOpsManualParamResolutionBase({
    sessionId: "ops-manual-param-skip-manual",
    userText: "排查 Redis，但还没有补齐目标实例、环境、执行方式和现象指标",
    artifacts: [opsManualParamResolutionSearchArtifact()],
    overrides,
  });
}

export function createOpsManualParamSkipManualFixtureSessions(overrides = {}) {
  return createOpsManualParamResolutionSessions("ops-manual-param-skip-manual", "不使用手册", overrides);
}

export function createOpsManualGenerateFromChatFixtureState(overrides = {}) {
  return createChatFixtureState({
    cards: [
      {
        id: "user-ops-manual-generate-from-chat",
        type: "UserMessageCard",
        role: "user",
        text: "排查 Redis 内存和 p95 升高",
        createdAt: "2026-05-17T10:00:00Z",
        updatedAt: "2026-05-17T10:00:00Z",
      },
      {
        id: "cmd-ops-manual-generate-1",
        type: "CommandCard",
        title: "docker ps --filter name=redis",
        summary: "确认当前主机 Redis 容器",
        createdAt: "2026-05-17T10:00:04Z",
        updatedAt: "2026-05-17T10:00:04Z",
      },
      {
        id: "cmd-ops-manual-generate-2",
        type: "CommandCard",
        title: "docker exec aiops-redis redis-cli INFO memory",
        summary: "读取 Redis 内存指标",
        createdAt: "2026-05-17T10:00:08Z",
        updatedAt: "2026-05-17T10:00:08Z",
      },
      {
        id: "cmd-ops-manual-generate-3",
        type: "CommandCard",
        title: "docker stats --no-stream aiops-redis",
        summary: "确认容器资源视角",
        createdAt: "2026-05-17T10:00:12Z",
        updatedAt: "2026-05-17T10:00:12Z",
      },
      {
        id: "assistant-ops-manual-generate-from-chat",
        type: "AssistantMessageCard",
        role: "assistant",
        text: "本次验证状态：已验证，结论基于当前主机与 Redis 容器实时只读结果；未执行任何变更操作。建议沉淀为 Redis 容器只读排障运维手册。",
        createdAt: "2026-05-17T10:00:18Z",
        updatedAt: "2026-05-17T10:00:18Z",
      },
    ],
    runtime: { turn: { active: false, phase: "idle", hostId: "server-local" }, codex: { status: "connected", retryAttempt: 0, retryMax: 5 } },
    sessionId: "ops-manual-generate-from-chat",
    threadId: "ops-manual-generate-from-chat",
    ...overrides,
  });
}

export function createOpsManualGenerateFromChatFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "ops-manual-generate-from-chat",
    sessions: [
      {
        id: "ops-manual-generate-from-chat",
        kind: "single_host",
        title: "Ops Manual Generate From Chat",
        status: "running",
        messageCount: 2,
        preview: "排查 Redis 内存和 p95 升高",
        selectedHostId: "server-local",
        lastActivityAt: "2026-05-17T10:00:18Z",
      },
    ],
    ...overrides,
  });
}

export function createToolProgressiveDiscoveryFixtureState(overrides = {}) {
  const now = "2026-06-06T02:00:00Z";
  const finalText = `## synthetic progressive discovery final

- final evidence: synthetic.metrics.read checked
- final evidence: synthetic.audit.read not_checked
- 低置信说明：synthetic.audit.read 未加载后的审计补充未完成，因此只能给出低置信结论，不能把未检查证据当成已验证事实。`;
  const state = createChatFixtureState({
    sessionId: "tool-progressive-discovery",
    threadId: "tool-progressive-discovery",
    status: "idle",
    cards: [
      {
        id: "user-tool-progressive-discovery",
        type: "UserMessageCard",
        role: "user",
        text: "synthetic_complex_tool_discovery_request: compare a synthetic signal, select only the needed read capability, recover from an unloaded deferred tool, and report final evidence status.",
        status: "completed",
        createdAt: now,
        updatedAt: now,
      },
      {
        id: "assistant-tool-progressive-discovery",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        status: "completed",
        createdAt: "2026-06-06T02:00:12Z",
        updatedAt: "2026-06-06T02:00:12Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "synthetic-workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText,
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.status = "completed";
  turn.startedAt = now;
  turn.completedAt = "2026-06-06T02:00:12Z";
  turn.updatedAt = "2026-06-06T02:00:12Z";
  turn.process = [
    {
      id: "tool-progressive-search-trace",
      kind: "plan",
      displayKind: "tool_discovery_trace",
      status: "completed",
      text: "Progressive tool discovery trace",
      steps: [
        {
          id: "tool-progressive-search-step",
          text: "tool_search mode=search",
          status: "completed",
        },
      ],
      updatedAt: "2026-06-06T02:00:01Z",
    },
    {
      id: "tool-progressive-search",
      kind: "tool",
      displayKind: "tool_search",
      status: "completed",
      text: "tool_search mode=search query=synthetic capability for read-only signal comparison",
      outputPreview: "results: synthetic.metrics.read requiresSelect=true; synthetic.audit.read requiresSelect=true",
      updatedAt: "2026-06-06T02:00:02Z",
    },
    {
      id: "tool-progressive-select",
      kind: "tool",
      displayKind: "tool_search",
      status: "completed",
      text: "tool_search mode=select selected=synthetic.metrics.read",
      outputPreview: "selected tool delta: +synthetic.metrics.read; prompt delta contains schema for selected capability only",
      updatedAt: "2026-06-06T02:00:04Z",
    },
    {
      id: "tool-progressive-selected-delta",
      kind: "system",
      displayKind: "tool_surface_delta",
      status: "completed",
      text: "selected tool delta: +synthetic.metrics.read",
      updatedAt: "2026-06-06T02:00:05Z",
    },
    {
      id: "tool-progressive-unloaded-error",
      kind: "tool",
      displayKind: "synthetic.audit.read",
      status: "failed",
      text: "tool_unloaded recoverable error: synthetic.audit.read is deferred and must be selected before use",
      outputPreview: "recoverable=true requiredAction=call tool_search with mode=search, then mode=select",
      updatedAt: "2026-06-06T02:00:06Z",
    },
    {
      id: "tool-progressive-use-selected",
      kind: "tool",
      displayKind: "synthetic.metrics.read",
      status: "completed",
      text: "synthetic.metrics.read returned checked synthetic evidence",
      outputPreview: "final evidence: synthetic.metrics.read checked",
      updatedAt: "2026-06-06T02:00:08Z",
    },
    {
      id: "tool-progressive-final-evidence",
      kind: "assistant",
      displayKind: "assistant.final",
      status: "completed",
      text: finalText,
      updatedAt: "2026-06-06T02:00:12Z",
    },
  ];
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "synthetic-workspace",
    hosts: [
      {
        id: "synthetic-workspace",
        name: "synthetic workspace",
        status: "online",
        executable: false,
        terminalCapable: false,
      },
    ],
    lastActivityAt: "2026-06-06T02:00:12Z",
    ...overrides,
  };
}

export function createToolProgressiveDiscoveryFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "tool-progressive-discovery",
    sessions: [
      {
        id: "tool-progressive-discovery",
        kind: "workspace",
        title: "Synthetic progressive discovery",
        status: "completed",
        messageCount: 2,
        preview: "synthetic_complex_tool_discovery_request",
        selectedHostId: "synthetic-workspace",
        lastActivityAt: "2026-06-06T02:00:12Z",
      },
    ],
    ...overrides,
  });
}

export function createSkillsMcpProgressiveDiscoveryFixtureState(overrides = {}) {
  const now = "2026-06-06T03:00:00Z";
  const finalText = `## synthetic skills mcp final

- final evidence: skill checked
- mcp resource artifact: application/pdf
- 低置信说明：未读取的 skill/MCP 资源不会被当成已验证事实。`;
  const state = createChatFixtureState({
    sessionId: "skills-mcp-progressive-discovery",
    threadId: "skills-mcp-progressive-discovery",
    status: "idle",
    cards: [
      {
        id: "user-skills-mcp-progressive-discovery",
        type: "UserMessageCard",
        role: "user",
        text: "synthetic_skills_mcp_progressive_request: verify skill discovery, mandatory activation, mcp instruction delta, sparse reminder, and artifact evidence.",
        status: "completed",
        createdAt: now,
        updatedAt: now,
      },
      {
        id: "assistant-skills-mcp-progressive-discovery",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        status: "completed",
        createdAt: "2026-06-06T03:00:16Z",
        updatedAt: "2026-06-06T03:00:16Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "synthetic-workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText,
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.status = "completed";
  turn.startedAt = now;
  turn.completedAt = "2026-06-06T03:00:16Z";
  turn.updatedAt = "2026-06-06T03:00:16Z";
  turn.process = [
    { id: "skill-search", kind: "tool", displayKind: "skill_search", status: "completed", text: "skill_search mode=search query=synthetic diagnosis", outputPreview: "match: synthetic.triage requiresRead=true requiredForMatch=true", updatedAt: "2026-06-06T03:00:02Z" },
    { id: "mandatory-retry", kind: "system", displayKind: "skill_activation_gate", status: "completed", text: "mandatory skill activation retry", outputPreview: "requiredSkills=synthetic.triage action=require_skill_read", updatedAt: "2026-06-06T03:00:04Z" },
    { id: "skill-read", kind: "tool", displayKind: "skill_read", status: "completed", text: "skill_read skill=synthetic.triage", outputPreview: "loaded skill delta: +synthetic.triage; final evidence: skill checked", updatedAt: "2026-06-06T03:00:06Z" },
    { id: "mcp-instruction-delta", kind: "system", displayKind: "mcp_instruction_delta", status: "completed", text: "mcp instruction delta: added synthetic-docs", outputPreview: "server=synthetic-docs action=added", updatedAt: "2026-06-06T03:00:08Z" },
    { id: "mcp-sparse-reminder", kind: "system", displayKind: "mcp_instruction_reminder", status: "completed", text: "mcp sparse reminder", outputPreview: "server=synthetic-docs hash=sha256:synthetic summary=bounded resource reads", updatedAt: "2026-06-06T03:00:10Z" },
    { id: "mcp-artifact", kind: "tool", displayKind: "read_mcp_resource", status: "completed", text: "mcp resource artifact: application/pdf", outputPreview: "artifactRef=store://artifacts/mcp-resource-synthetic.pdf metadataOnly=true", updatedAt: "2026-06-06T03:00:12Z" },
    { id: "skills-mcp-final-evidence", kind: "assistant", displayKind: "assistant.final", status: "completed", text: finalText, updatedAt: "2026-06-06T03:00:16Z" },
  ];
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "synthetic-workspace",
    hosts: [{ id: "synthetic-workspace", name: "synthetic workspace", status: "online", executable: false, terminalCapable: false }],
    lastActivityAt: "2026-06-06T03:00:16Z",
    ...overrides,
  };
}

export function createSkillsMcpProgressiveDiscoveryFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "skills-mcp-progressive-discovery",
    sessions: [
      {
        id: "skills-mcp-progressive-discovery",
        kind: "workspace",
        title: "Synthetic Skills MCP Discovery",
        status: "completed",
        messageCount: 2,
        preview: "synthetic_skills_mcp_progressive_request",
        selectedHostId: "synthetic-workspace",
        lastActivityAt: "2026-06-06T03:00:16Z",
      },
    ],
    ...overrides,
  });
}

export function createMultiAgentSchedulingFixtureState(overrides = {}) {
  const now = "2026-06-06T04:00:00Z";
  const finalText = `## synthetic multi-agent scheduling final

- agent listing loaded: synthetic.explorer
- delegation decision: spawn_new
- assignment lint: pass
- parallel agents requested
- resource lock acquired
- pending agent final gate: require_wait
- wait_agent notifications: completed
- continuation decision: continue_existing
- verification agent: PASS
- final synthesis: evidence checked`;
  const state = createChatFixtureState({
    sessionId: "multi-agent-scheduling",
    threadId: "multi-agent-scheduling",
    status: "idle",
    cards: [
      {
        id: "user-multi-agent-scheduling",
        type: "UserMessageCard",
        role: "user",
        text: "synthetic_multi_agent_scheduling_request: validate agent catalog, delegation, assignment lint, resource locks, wait gates, notifications, continuation, verifier, and final synthesis.",
        status: "completed",
        createdAt: now,
        updatedAt: now,
      },
      {
        id: "assistant-multi-agent-scheduling",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        status: "completed",
        createdAt: "2026-06-06T04:00:20Z",
        updatedAt: "2026-06-06T04:00:20Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "synthetic-workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText,
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.status = "completed";
  turn.startedAt = now;
  turn.completedAt = "2026-06-06T04:00:20Z";
  turn.updatedAt = "2026-06-06T04:00:20Z";
  turn.process = [
    { id: "multi-agent-listing", kind: "tool", displayKind: "agent_catalog", status: "completed", text: "agent listing loaded: synthetic.explorer", outputPreview: "agents=synthetic.explorer,synthetic.verifier", updatedAt: "2026-06-06T04:00:02Z" },
    { id: "multi-agent-delegation-spawn", kind: "system", displayKind: "agent_delegation_decision", status: "completed", text: "delegation decision: spawn_new", outputPreview: "target=synthetic.explorer reason=bounded_parallel_probe", updatedAt: "2026-06-06T04:00:04Z" },
    { id: "multi-agent-assignment-lint", kind: "system", displayKind: "agent_assignment_lint", status: "completed", text: "assignment lint: pass", outputPreview: "scope=synthetic.evidence_readonly isolation=manager_summary_only", updatedAt: "2026-06-06T04:00:06Z" },
    { id: "multi-agent-parallel-requested", kind: "system", displayKind: "agent_parallel_trace_group", status: "completed", text: "parallel agents requested", outputPreview: "agents=synthetic.explorer", updatedAt: "2026-06-06T04:00:08Z" },
    { id: "multi-agent-resource-lock", kind: "system", displayKind: "resource_lock_trace", status: "completed", text: "resource lock acquired", outputPreview: "resource=synthetic.shared-context mode=exclusive", updatedAt: "2026-06-06T04:00:10Z" },
    { id: "multi-agent-final-gate", kind: "system", displayKind: "agent_final_gate", status: "completed", text: "pending agent final gate: require_wait", outputPreview: "pending=synthetic.explorer action=wait_agent", updatedAt: "2026-06-06T04:00:12Z" },
    { id: "multi-agent-wait-notifications", kind: "tool", displayKind: "wait_agent", status: "completed", text: "wait_agent notifications: completed", outputPreview: "synthetic.explorer completed with manager summary", updatedAt: "2026-06-06T04:00:14Z" },
    { id: "multi-agent-continuation", kind: "system", displayKind: "agent_continuation_decision", status: "completed", text: "continuation decision: continue_existing", outputPreview: "agent=synthetic.explorer reason=active_context_matches", updatedAt: "2026-06-06T04:00:16Z" },
    { id: "multi-agent-verifier", kind: "tool", displayKind: "verification_agent", status: "completed", text: "verification agent: PASS", outputPreview: "checked=synthetic.evidence_bundle result=PASS", updatedAt: "2026-06-06T04:00:18Z" },
    { id: "multi-agent-final-synthesis", kind: "system", displayKind: "assistant.final", status: "completed", text: "final synthesis: evidence checked", outputPreview: finalText, updatedAt: "2026-06-06T04:00:20Z" },
  ];
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "synthetic-workspace",
    hosts: [{ id: "synthetic-workspace", name: "synthetic workspace", status: "online", executable: false, terminalCapable: false }],
    lastActivityAt: "2026-06-06T04:00:20Z",
    ...overrides,
  };
}

export function createMultiAgentSchedulingFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "multi-agent-scheduling",
    sessions: [
      {
        id: "multi-agent-scheduling",
        kind: "workspace",
        title: "Synthetic Multi-Agent Scheduling",
        status: "completed",
        messageCount: 2,
        preview: "synthetic_multi_agent_scheduling_request",
        selectedHostId: "synthetic-workspace",
        lastActivityAt: "2026-06-06T04:00:20Z",
      },
    ],
    ...overrides,
  });
}

export function createVerificationCompletionSafetyPermissionFixtureState(overrides = {}) {
  const now = "2026-06-07T02:20:00Z";
  const finalText = [
    "## synthetic verification completion safety permission final",
    "",
    "- verification_status=PARTIAL",
    "- completion gate block: execution evidence missing",
    "- blocker next action: rerun focused synthetic verification command",
    "- destructive workaround safety signal: skip_validation high-risk blocked",
    "- unexpected state gate: block_mutation",
    "- approval scope summary: allowedActions=read_metrics,read_logs request_verification",
  ].join("\n");
  const state = createChatFixtureState({
    sessionId: "verification-completion-safety-permission",
    threadId: "verification-completion-safety-permission",
    status: "idle",
    cards: [
      {
        id: "user-verification-completion-safety-permission",
        type: "UserMessageCard",
        role: "user",
        text: "synthetic_verification_completion_safety_permission_request: show partial verification, completion gate blocking, safety policy, unexpected state gate, and approval scope evidence.",
        status: "completed",
        createdAt: now,
        updatedAt: now,
      },
      {
        id: "assistant-verification-completion-safety-permission",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        status: "completed",
        createdAt: "2026-06-07T02:20:18Z",
        updatedAt: "2026-06-07T02:20:18Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "completed", hostId: "synthetic-workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    finalText,
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.status = "completed";
  turn.startedAt = now;
  turn.completedAt = "2026-06-07T02:20:18Z";
  turn.updatedAt = "2026-06-07T02:20:18Z";
  turn.process = [
    {
      id: "verification-report-partial",
      kind: "tool",
      displayKind: "verification.report",
      status: "completed",
      text: "verification_status=PARTIAL",
      outputPreview: "requirement=execution_required evidenceKinds=analysis blocker=tool_unavailable",
      evidenceRefs: ["synthetic:evidence:analysis-only"],
      updatedAt: "2026-06-07T02:20:02Z",
    },
    {
      id: "completion-gate-execution-evidence",
      kind: "system",
      displayKind: "completion_gate",
      status: "blocked",
      text: "completion gate block: execution evidence missing",
      outputPreview: "PASS is not allowed because execution_required has no execution/static_check/adversarial evidence.",
      evidenceRefs: ["synthetic:gate:completion-block"],
      updatedAt: "2026-06-07T02:20:04Z",
    },
    {
      id: "verification-blocker-next-action",
      kind: "system",
      displayKind: "verification.blocker",
      status: "blocked",
      text: "blocker next action: rerun focused synthetic verification command",
      outputPreview: "source=tool_unavailable nextAction=restore verifier tool and rerun focused synthetic verification command",
      updatedAt: "2026-06-07T02:20:06Z",
    },
    {
      id: "destructive-workaround-signal",
      kind: "system",
      displayKind: "safety_signal",
      status: "rejected",
      text: "destructive workaround safety signal: skip_validation high-risk blocked",
      outputPreview: "category=skip_validation severity=high reason=attempted to bypass required verification",
      updatedAt: "2026-06-07T02:20:08Z",
    },
    {
      id: "unexpected-state-gate-blocked",
      kind: "system",
      displayKind: "unexpected_state_gate",
      status: "blocked",
      text: "unexpected state gate: block_mutation",
      outputPreview: "toolStatus=precondition_failed action=block_mutation next=inspect_or_replan",
      updatedAt: "2026-06-07T02:20:10Z",
    },
    {
      id: "approval-scope-summary",
      kind: "system",
      displayKind: "approval_scope",
      status: "completed",
      text: "approval scope summary: allowedActions=read_metrics,read_logs request_verification",
      outputPreview: "resourceScopes=synthetic:service:demo-api,synthetic:evidence:verification riskCeiling=low inputHash=synthetic-input-hash",
      updatedAt: "2026-06-07T02:20:12Z",
    },
    {
      id: "verification-completion-safety-final",
      kind: "assistant",
      displayKind: "assistant.final",
      status: "completed",
      text: finalText,
      updatedAt: "2026-06-07T02:20:18Z",
    },
  ];
  state.verificationReports = [
    {
      id: "synthetic-verification-report-1",
      requirement: "execution_required",
      status: "PARTIAL",
      subject: "synthetic verification completion safety permission fixture",
      evidence: [
        {
          kind: "analysis",
          toolName: "synthetic.inspect",
          expected: "verification report has execution evidence before PASS",
          actual: "analysis evidence only",
          result: "pass",
          rawRef: "synthetic:evidence:analysis-only",
        },
      ],
      blockers: [
        {
          reason: "focused execution verifier tool unavailable in synthetic fixture",
          source: "tool_unavailable",
          nextAction: "rerun focused synthetic verification command",
        },
      ],
      rawRefs: ["synthetic:evidence:analysis-only", "synthetic:gate:completion-block"],
    },
  ];
  state.safetySignals = [
    {
      category: "skip_validation",
      severity: "high",
      reasons: ["attempted to bypass required verification"],
    },
  ];
  state.unexpectedStateDecisions = [
    {
      action: "block_mutation",
      reasons: ["precondition_failed", "requires inspect or re-plan before mutation"],
    },
  ];
  state.planApprovalScope = {
    planId: "plan-synthetic-verification-1",
    approvalId: "approval-synthetic-verification-1",
    allowedActions: ["read_metrics", "read_logs", "request_verification"],
    resourceScopes: ["synthetic:service:demo-api", "synthetic:evidence:verification"],
    riskCeiling: "low",
    expiresAt: "2026-06-07T03:20:00Z",
    inputHash: "synthetic-input-hash",
  };
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "synthetic-workspace",
    hosts: [{ id: "synthetic-workspace", name: "synthetic workspace", status: "online", executable: false, terminalCapable: false }],
    lastActivityAt: "2026-06-07T02:20:18Z",
    ...overrides,
  };
}

export function createVerificationCompletionSafetyPermissionFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "verification-completion-safety-permission",
    sessions: [
      {
        id: "verification-completion-safety-permission",
        kind: "workspace",
        title: "Synthetic Verification Safety",
        status: "completed",
        messageCount: 2,
        preview: "synthetic_verification_completion_safety_permission_request",
        selectedHostId: "synthetic-workspace",
        lastActivityAt: "2026-06-07T02:20:18Z",
      },
    ],
    ...overrides,
  });
}

export function createUxModelGeneralityFixtureState(overrides = {}) {
  const now = "2026-06-07T03:00:00Z";
  const finalText = [
    "## synthetic ux model generality pending state",
    "",
    "- task_depth=investigation",
    "- required_gates=plan,evidence,verification",
    "- ux_phase=waiting_approval",
    "- resume_action=continue_next_step",
    "- manager_synthesis=required",
    "- coverage_action=continue_gathering",
    "- reasoning_fallback=prompt_policy",
    "- genericity_violations=0",
  ].join("\n");
  const state = createChatFixtureState({
    sessionId: "ux-model-generality",
    threadId: "ux-model-generality",
    status: "blocked",
    cards: [
      {
        id: "user-ux-model-generality",
        type: "UserMessageCard",
        role: "user",
        text: "synthetic_ux_model_generality_request: show abstract depth, gates, approval phase, resume policy, synthesis gate, coverage action, fallback policy, and genericity scan state.",
        status: "completed",
        createdAt: now,
        updatedAt: now,
      },
      {
        id: "assistant-ux-model-generality",
        type: "AssistantMessageCard",
        role: "assistant",
        text: finalText,
        status: "running",
        createdAt: "2026-06-07T03:00:18Z",
        updatedAt: "2026-06-07T03:00:18Z",
      },
    ],
    runtime: {
      turn: { active: true, phase: "waiting_approval", hostId: "synthetic-workspace" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: { viewedFiles: [], searchedWebQueries: [], searchedContentQueries: [] },
    },
    approvals: [
      {
        id: "approval-ux-model-generality-1",
        status: "pending",
        type: "plan_exit",
        reason: "Synthetic fixture waits for approval before continuing the next abstract step.",
        command: "approve_next_step synthetic-plan-ux-model-generality",
        requestedAt: "2026-06-07T03:00:10Z",
      },
    ],
    finalText,
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  turn.status = "blocked";
  turn.startedAt = now;
  turn.updatedAt = "2026-06-07T03:00:18Z";
  turn.process = [
    {
      id: "ux-model-generality-depth",
      kind: "system",
      displayKind: "task_depth_trace",
      status: "completed",
      text: "task_depth=investigation",
      outputPreview: "reason=multi_step_unknowns evidence_required=true",
      updatedAt: "2026-06-07T03:00:02Z",
    },
    {
      id: "ux-model-generality-required-gates",
      kind: "system",
      displayKind: "required_gate_trace",
      status: "completed",
      text: "required_gates=plan,evidence,verification",
      outputPreview: "gate_source=abstract_task_profile",
      updatedAt: "2026-06-07T03:00:04Z",
    },
    {
      id: "ux-model-generality-phase",
      kind: "system",
      displayKind: "ux_progress_trace",
      status: "running",
      text: "ux_phase=waiting_approval",
      outputPreview: "pendingApproval=approval-ux-model-generality-1",
      updatedAt: "2026-06-07T03:00:06Z",
    },
    {
      id: "ux-model-generality-resume",
      kind: "system",
      displayKind: "resume_policy",
      status: "completed",
      text: "resume_action=continue_next_step",
      outputPreview: "recapAllowed=false unless_requested_by_user=true",
      updatedAt: "2026-06-07T03:00:08Z",
    },
    {
      id: "ux-model-generality-synthesis",
      kind: "system",
      displayKind: "manager_synthesis_gate",
      status: "blocked",
      text: "manager_synthesis=required",
      outputPreview: "workerOutputs=refs_only finalRequires=manager_answer",
      updatedAt: "2026-06-07T03:00:10Z",
    },
    {
      id: "ux-model-generality-coverage",
      kind: "system",
      displayKind: "evidence_coverage",
      status: "running",
      text: "coverage_action=continue_gathering",
      outputPreview: "missing=verification covered=plan,evidence",
      updatedAt: "2026-06-07T03:00:12Z",
    },
    {
      id: "ux-model-generality-reasoning-fallback",
      kind: "system",
      displayKind: "reasoning_fallback",
      status: "completed",
      text: "reasoning_fallback=prompt_policy",
      outputPreview: "providerCapability=reasoning_unsupported policy=prompt_visible_depth",
      updatedAt: "2026-06-07T03:00:14Z",
    },
    {
      id: "ux-model-generality-genericity",
      kind: "system",
      displayKind: "genericity_scan",
      status: "completed",
      text: "genericity_violations=0",
      outputPreview: "blocked_core_rule=0 allowed_fixture_terms=synthetic",
      updatedAt: "2026-06-07T03:00:16Z",
    },
  ];
  state.genericityTrace = {
    coreRuleDomainTerms: [],
    allowedFixtureTerms: ["synthetic"],
    resourceIdSource: "fixture",
    violations: [],
  };
  state.managerSynthesisGate = {
    action: "require_manager_synthesis",
    workerOutputRefs: ["synthetic:worker-output:abstract-1"],
    reasons: ["final_answer_must_use_manager_synthesis"],
  };
  state.evidenceCoverageDecision = {
    action: "continue_gathering",
    coverage: 0.67,
    requiredDimensions: ["plan", "evidence", "verification"],
    coveredDimensions: ["plan", "evidence"],
    missingDimensions: ["verification"],
    verificationStatus: "pending",
  };
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "synthetic-workspace",
    hosts: [{ id: "synthetic-workspace", name: "synthetic workspace", status: "online", executable: false, terminalCapable: false }],
    lastActivityAt: "2026-06-07T03:00:18Z",
    ...overrides,
  };
}

export function createUxModelGeneralityFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "ux-model-generality",
    sessions: [
      {
        id: "ux-model-generality",
        kind: "workspace",
        title: "Synthetic UX Model Generality",
        status: "blocked",
        messageCount: 2,
        preview: "synthetic_ux_model_generality_request",
        selectedHostId: "synthetic-workspace",
        lastActivityAt: "2026-06-07T03:00:18Z",
      },
    ],
    ...overrides,
  });
}

export function createCorootRcaReportFixtureState(overrides = {}) {
  const state = createChatFixtureState({
    cards: [
      {
        id: "user-coroot-rca-report",
        type: "UserMessageCard",
        role: "user",
        text: "分析 checkout 服务最近 30 分钟延迟升高的根因",
        createdAt: "2026-05-15T02:00:00Z",
        updatedAt: "2026-05-15T02:00:00Z",
      },
      {
        id: "assistant-coroot-rca-report",
        type: "AssistantMessageCard",
        role: "assistant",
        text: "RCA 初步完成，最强假设是 catalog 依赖延迟传播到 checkout。",
        createdAt: "2026-05-15T02:00:12Z",
        updatedAt: "2026-05-15T02:00:12Z",
      },
    ],
    runtime: {
      turn: { active: false, phase: "idle", hostId: "server-local" },
      codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
      activity: {
        searchedContentQueries: [],
        searchedWebQueries: [],
        viewedFiles: [],
      },
    },
    sessionId: "coroot-rca-report",
    threadId: "coroot-rca-report",
    ...overrides,
  });
  const turn = state.turns[state.currentTurnId];
  if (turn) {
    turn.process = [
      createProcessBlock({
        id: "tool-coroot-context",
        kind: "tool",
        status: "completed",
        text: "coroot.collect_rca_context",
        updatedAt: "2026-05-15T02:00:04Z",
      }),
      createProcessBlock({
        id: "tool-artifact-rca",
        kind: "tool",
        status: "completed",
        text: "aiops.ui_artifact_emit",
        updatedAt: "2026-05-15T02:00:08Z",
      }),
    ];
    turn.agentUiArtifacts = [
      {
        id: "artifact-rca-report",
        type: "rca_report",
        titleZh: "checkout 根因分析",
        summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
        status: "ok",
        severity: "high",
        source: "coroot",
        permissionScope: "read",
        redactionStatus: "redacted",
        inlineData: {
          schemaVersion: "aiops.rca_report/v1",
          source: "coroot",
          status: "ok",
          target: { service: "checkout" },
          window: { timeRange: "30m" },
          conclusion: {
            summaryZh: "checkout 延迟升高最可能来自 catalog 依赖。",
            rootCauseEntity: "catalog",
            confidence: 0.72,
          },
          hypotheses: [
            {
              id: "hyp-1",
              titleZh: "catalog 依赖延迟",
              confidence: 0.72,
              supportingEvidenceRefs: ["ev-coroot-latency"],
              contradictingEvidenceRefs: [],
              missingEvidence: [],
            },
          ],
          sections: [
            {
              id: "propagation",
              kind: "propagation_map",
              titleZh: "传播路径",
              evidenceRefs: ["ev-coroot-latency"],
              payload: {
                nodes: [{ id: "checkout" }, { id: "catalog" }],
                edges: [{ source: "checkout", target: "catalog" }],
              },
            },
            {
              id: "metrics",
              kind: "timeseries_grid",
              titleZh: "关键指标",
              evidenceRefs: ["ev-coroot-latency"],
              payload: {
                metrics: [
                  {
                    name: "latency_p99",
                    entity: "checkout->catalog",
                    valueSummary: "p99 rose to 1.8s",
                    status: "critical",
                  },
                ],
              },
            },
          ],
          evidenceRefs: ["ev-coroot-latency"],
          rawRefs: [{ source: "coroot", uri: "coroot://project/default/checkout" }],
          limitations: [],
        },
      },
    ];
  }
  return state;
}

export function createCorootRcaReportFixtureSessions(overrides = {}) {
  return createChatFixtureSessions({
    activeSessionId: "coroot-rca-report",
    sessions: [
      {
        id: "coroot-rca-report",
        kind: "single_host",
        title: "Coroot RCA Report",
        status: "running",
        messageCount: 2,
        preview: "分析 checkout 服务延迟根因",
        selectedHostId: "server-local",
        lastActivityAt: "2026-05-15T02:00:12Z",
      },
    ],
    ...overrides,
  });
}

export function createTaskTodoPlanModeFixtureState(overrides = {}) {
  const now = "2026-06-07T01:30:00Z";
  const planId = "plan-synthetic-task-todo-1";
  const cards = overrides.cards || [
    {
      id: "user-task-todo-plan-mode",
      type: "UserMessageCard",
      role: "user",
      text: "请用计划模式梳理一个多步骤合成排查任务，并等待我批准后再执行。",
      createdAt: now,
      updatedAt: now,
    },
    {
      id: "assistant-task-todo-plan-mode",
      type: "AssistantMessageCard",
      role: "assistant",
      text: [
        "Plan Mode active，计划仍处于 pending_exit_approval。",
        "上一版计划已被拒绝：用户要求收窄验证范围后再批准。",
        "当前仍在计划模式，等待修订后重新请求批准。",
      ].join("\n"),
      createdAt: "2026-06-07T01:30:03Z",
      updatedAt: "2026-06-07T01:30:03Z",
    },
    {
      id: "plan-task-todo-plan-mode",
      type: "PlanCard",
      title: "Task/Todo Plan Mode synthetic fixture",
      text: "Plan Mode active / pending_exit_approval",
      items: [
        {
          step: "step-collect: in_progress owner=agent:planner agentId=agent-plan-7 claimLease=claim-lease-synthetic-1",
          status: "running",
        },
        {
          step: "step-confirm: blocked blockedBy=missing_user_decision reason=等待用户确认验证窗口",
          status: "blocked",
        },
        {
          step: "approval scope: allowedActions=read_metrics,read_logs,update_plan resourceScopes=synthetic:service:demo-api,synthetic:dashboard:latency riskCeiling=low",
          status: "pending",
        },
      ],
      createdAt: "2026-06-07T01:30:04Z",
      updatedAt: "2026-06-07T01:30:04Z",
    },
  ];
  const runtime = overrides.runtime || {
    turn: { active: true, phase: "waiting_approval", hostId: "workspace" },
    codex: { status: "connected", retryAttempt: 0, retryMax: 5 },
    activity: {
      viewedFiles: [],
      searchedWebQueries: [],
      searchedContentQueries: [{ query: "synthetic plan mode task state" }],
    },
  };
  const approvals = overrides.approvals || [
    {
      id: "approval-plan-exit-synthetic-1",
      status: "pending",
      type: "plan_exit",
      reason: "PlanArtifact pending_exit_approval requires user approval before execution.",
      command: "exit_plan_mode plan-synthetic-task-todo-1",
      requestedAt: "2026-06-07T01:30:06Z",
    },
  ];
  const state = createFixtureTransportState({
    sessionId: overrides.sessionId || "task-todo-plan-mode",
    threadId: overrides.threadId || "task-todo-plan-mode",
    status: overrides.status || "blocked",
    cards,
    runtime,
    approvals,
    finalText:
      overrides.finalText ||
      [
        "Plan Mode active: state=active planId=plan-synthetic-task-todo-1 artifactStatus=pending_exit_approval.",
        "Task owner: step-collect owner=agent:planner agentId=agent-plan-7 lease=claim-lease-synthetic-1 expiresAt=2026-06-07T01:45:00Z.",
        "Blocked task: step-confirm blockedBy=missing_user_decision reason=等待用户确认验证窗口。",
        "Rejected plan event: 用户要求收窄验证范围后再批准；仍在计划模式，等待修订后重新请求批准。",
        "Approval scope: allowedActions=read_metrics,read_logs,update_plan resourceScopes=synthetic:service:demo-api,synthetic:dashboard:latency riskCeiling=low.",
      ].join("\n"),
  });
  const turnId = state.currentTurnId;
  state.turns[turnId] = {
    ...state.turns[turnId],
    status: "blocked",
    process: [
      {
        id: "plan-mode-state-synthetic",
        kind: "tool",
        displayKind: "plan_mode.state",
        status: "running",
        text: "Plan Mode active; artifactStatus=pending_exit_approval; approvalId=approval-plan-exit-synthetic-1",
        updatedAt: "2026-06-07T01:30:05Z",
      },
      {
        id: "task-todo-plan-synthetic",
        kind: "plan",
        displayKind: "plan",
        status: "running",
        text: "plan updated: active pending_exit_approval",
        steps: [
          {
            id: "step-collect",
            text: "step-collect in_progress owner=agent:planner agentId=agent-plan-7 claimLease=claim-lease-synthetic-1",
            status: "in_progress",
            summary: "读取合成指标和日志，不执行 mutation",
          },
          {
            id: "step-confirm",
            text: "step-confirm blocked blockedBy=missing_user_decision reason=等待用户确认验证窗口",
            status: "blocked",
          },
          {
            id: "step-scope",
            text: "approval scope allowedActions=read_metrics,read_logs,update_plan resourceScopes=synthetic:service:demo-api,synthetic:dashboard:latency riskCeiling=low",
            status: "pending",
          },
        ],
        updatedAt: "2026-06-07T01:30:06Z",
      },
      {
        id: "plan-rejection-synthetic",
        kind: "tool",
        displayKind: "plan.rejection",
        status: "blocked",
        text: "用户要求收窄验证范围后再批准；仍在计划模式，等待修订后重新请求批准",
        updatedAt: "2026-06-07T01:30:07Z",
      },
    ],
  };
  state.planModeState = {
    state: "active",
    planId,
    requestedBy: "user",
    reason: "Complex synthetic task requires planning before execution.",
    expectedPlanType: "investigation",
    fullInstructionInjected: true,
    reminderLevel: "sparse",
    approvalId: "approval-plan-exit-synthetic-1",
    pendingQuestions: ["确认是否只保留只读验证范围"],
    lastRejectionReason: "用户要求收窄验证范围后再批准",
    compactRecoveryVersion: 1,
  };
  state.planArtifact = {
    id: planId,
    version: 2,
    status: "pending_exit_approval",
    steps: [
      {
        id: "step-collect",
        text: "读取合成指标和日志",
        status: "in_progress",
        owner: "agent:planner",
        agentId: "agent-plan-7",
        evidenceRefs: ["synthetic:evidence:metrics"],
      },
      {
        id: "step-confirm",
        text: "等待用户确认验证窗口",
        status: "blocked",
        blockedBy: ["missing_user_decision"],
        summary: "需要用户确认只读验证窗口",
      },
    ],
    rejections: [
      {
        id: "reject-synthetic-1",
        reason: "用户要求收窄验证范围后再批准",
        rejectedAt: "2026-06-07T01:28:00Z",
      },
    ],
  };
  state.planApprovalScope = {
    planId,
    allowedActions: ["read_metrics", "read_logs", "update_plan"],
    resourceScopes: ["synthetic:service:demo-api", "synthetic:dashboard:latency"],
    riskCeiling: "low",
    expiresAt: "2026-06-07T02:00:00Z",
    inputHash: "synthetic-input-hash",
  };
  state.taskClaims = [
    {
      taskId: "step-collect",
      owner: "agent:planner",
      agentId: "agent-plan-7",
      leaseId: "claim-lease-synthetic-1",
      expiresAt: "2026-06-07T01:45:00Z",
    },
  ];
  state.artifacts = {
    ...state.artifacts,
    [planId]: {
      id: planId,
      kind: "plan_artifact",
      title: "PlanArtifact pending_exit_approval",
      preview: "Plan Mode active with blocked task, rejection event, approval scope and claim lease.",
    },
  };
  state.runtimeLiveness = {
    ...state.runtimeLiveness,
    activeTurns: { [turnId]: true },
    activeAgents: { "agent-plan-7": true },
    pendingApprovals: { "approval-plan-exit-synthetic-1": true },
  };
  return {
    ...state,
    kind: "workspace",
    selectedHostId: "workspace",
    auth: { connected: true, pending: false, planType: "plus" },
    hosts: createBaseHosts().filter((host) => host.id === "server-local"),
    approvals,
    cards,
    runtime,
    lastActivityAt: "2026-06-07T01:30:07Z",
    config: { codexAlive: true },
    ...overrides,
  };
}

export function createTaskTodoPlanModeFixtureSessions(overrides = {}) {
  return {
    activeSessionId: "task-todo-plan-mode",
    sessions: [
      {
        id: "task-todo-plan-mode",
        kind: "workspace",
        title: "Task/Todo Plan Mode",
        status: "blocked",
        messageCount: 2,
        preview: "Plan Mode active / pending_exit_approval",
        selectedHostId: "workspace",
        lastActivityAt: "2026-06-07T01:30:07Z",
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
    case "context-compaction":
    case "context_compaction":
      return {
        name: "context-compaction",
        state: createContextCompactionFixtureState(),
        sessions: createContextCompactionFixtureSessions(),
      };
    case "host-ops-three-hosts":
    case "hostops-three-hosts":
      return {
        name: "host-ops-three-hosts",
        state: createHostOpsThreeHostsFixtureState(),
        sessions: createHostOpsThreeHostsFixtureSessions(),
      };
    case "protocol":
    case "workspace":
    case "protocol-fixture":
      return {
        name: "protocol",
        state: createProtocolFixtureState(),
        sessions: createProtocolFixtureSessions(),
      };
    case "ops-manual-preflight":
    case "opsmanual-preflight":
      return {
        name: "ops-manual-preflight",
        state: createOpsManualPreflightFixtureState(),
        sessions: createOpsManualPreflightFixtureSessions(),
      };
    case "ops-manual-4field-form":
    case "opsmanual-4field-form":
      return {
        name: "ops-manual-4field-form",
        state: createOpsManualFourFieldFormFixtureState(),
        sessions: createOpsManualFourFieldFormFixtureSessions(),
      };
    case "ops-manual-param-auto-redis":
      return {
        name: "ops-manual-param-auto-redis",
        state: createOpsManualParamAutoRedisFixtureState(),
        sessions: createOpsManualParamAutoRedisFixtureSessions(),
      };
    case "ops-manual-param-multi-redis":
      return {
        name: "ops-manual-param-multi-redis",
        state: createOpsManualParamMultiRedisFixtureState(),
        sessions: createOpsManualParamMultiRedisFixtureSessions(),
      };
    case "ops-manual-param-pg-backup-path":
      return {
        name: "ops-manual-param-pg-backup-path",
        state: createOpsManualParamPgBackupPathFixtureState(),
        sessions: createOpsManualParamPgBackupPathFixtureSessions(),
      };
    case "ops-manual-param-secret":
      return {
        name: "ops-manual-param-secret",
        state: createOpsManualParamSecretFixtureState(),
        sessions: createOpsManualParamSecretFixtureSessions(),
      };
    case "ops-manual-param-skip-manual":
      return {
        name: "ops-manual-param-skip-manual",
        state: createOpsManualParamSkipManualFixtureState(),
        sessions: createOpsManualParamSkipManualFixtureSessions(),
      };
    case "ops-manual-generate-from-chat":
    case "opsmanual-generate-from-chat":
      return {
        name: "ops-manual-generate-from-chat",
        state: createOpsManualGenerateFromChatFixtureState(),
        sessions: createOpsManualGenerateFromChatFixtureSessions(),
      };
    case "tool-progressive-discovery":
    case "tool_progressive_discovery":
      return {
        name: "tool-progressive-discovery",
        state: createToolProgressiveDiscoveryFixtureState(),
        sessions: createToolProgressiveDiscoveryFixtureSessions(),
      };
    case "skills-mcp-progressive-discovery":
    case "skills_mcp_progressive_discovery":
      return {
        name: "skills-mcp-progressive-discovery",
        state: createSkillsMcpProgressiveDiscoveryFixtureState(),
        sessions: createSkillsMcpProgressiveDiscoveryFixtureSessions(),
      };
    case "multi-agent-scheduling":
    case "multi_agent_scheduling":
      return {
        name: "multi-agent-scheduling",
        state: createMultiAgentSchedulingFixtureState(),
        sessions: createMultiAgentSchedulingFixtureSessions(),
      };
    case "verification-completion-safety-permission":
    case "verification_completion_safety_permission":
      return {
        name: "verification-completion-safety-permission",
        state: createVerificationCompletionSafetyPermissionFixtureState(),
        sessions: createVerificationCompletionSafetyPermissionFixtureSessions(),
      };
    case "ux-model-generality":
    case "ux_model_generality":
      return {
        name: "ux-model-generality",
        state: createUxModelGeneralityFixtureState(),
        sessions: createUxModelGeneralityFixtureSessions(),
      };
    case "task-todo-plan-mode":
    case "task_todo_plan_mode":
      return {
        name: "task-todo-plan-mode",
        state: createTaskTodoPlanModeFixtureState(),
        sessions: createTaskTodoPlanModeFixtureSessions(),
      };
    case "coroot-rca-report":
    case "rca-report":
      return {
        name: "coroot-rca-report",
        state: createCorootRcaReportFixtureState(),
        sessions: createCorootRcaReportFixtureSessions(),
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
