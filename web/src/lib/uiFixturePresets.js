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
