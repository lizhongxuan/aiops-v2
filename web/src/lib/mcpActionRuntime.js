import { compactText, formatShortTime } from "./workspaceViewModel";

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function stringifyParams(params = {}) {
  const source = asObject(params);
  return Object.entries(source)
    .filter(([, value]) => value !== undefined && value !== null && value !== "")
    .map(([key, value]) => `${key}=${value}`)
    .join(", ");
}

export function isMcpMutationAction(action = {}) {
  const approvalMode = compactText(action.approvalMode || action.approval_mode).toLowerCase();
  return Boolean(action.mutation || action.destructive || approvalMode === "required");
}

export function formatMcpActionLabel(action = {}) {
  return compactText(action.label || action.title || action.name || action.intent || "未命名操作") || "未命名操作";
}

export function formatMcpActionTarget(action = {}, scope = {}) {
  const target = asObject(action.target);
  const resolvedScope = asObject(scope);
  return compactText(
    target.label ||
      target.name ||
      target.resourceId ||
      resolvedScope.service ||
      resolvedScope.hostId ||
      resolvedScope.resourceId ||
      "当前监控对象",
  ) || "当前监控对象";
}

export function buildMcpActionDetailRows(action = {}, options = {}) {
  const scope = asObject(options.scope);
  const paramsText = stringifyParams(action.params);
  return [
    { key: "操作", value: formatMcpActionLabel(action) },
    { key: "意图", value: compactText(action.intent || "open") || "open" },
    { key: "目标", value: formatMcpActionTarget(action, scope) },
    { key: "权限路径", value: compactText(action.permissionPath || "未声明") || "未声明" },
    ...(paramsText ? [{ key: "参数", value: paramsText }] : []),
  ];
}

export function buildSyntheticMcpApproval(action = {}, options = {}) {
  const now = options.now || new Date().toISOString();
  const id = compactText(options.id || `mcp-approval-${Date.now()}`);
  const scope = asObject(options.scope);
  const title = compactText(options.title || formatMcpActionLabel(action));
  const summary = compactText(options.summary || action.confirmText || options.notice || "等待用户确认后执行。");
  return {
    id,
    approvalId: id,
    requestId: id,
    status: "pending",
    tone: action.destructive ? "danger" : "warning",
    source: "mcp",
    mcpSynthetic: true,
    title,
    label: title,
    summary,
    command: compactText(options.command || `${compactText(action.intent || "run")} ${formatMcpActionTarget(action, scope)}`),
    hostId: compactText(options.hostId || scope.hostId || ""),
    hostName: compactText(options.hostName || scope.hostId || ""),
    details: buildMcpActionDetailRows(action, { scope }),
    action: {
      ...action,
      label: title,
    },
    createdAt: now,
    updatedAt: now,
  };
}

export function buildSyntheticMcpEvent(action = {}, options = {}) {
  const at = options.at || new Date().toISOString();
  const title = compactText(options.title || formatMcpActionLabel(action));
  const targetId = compactText(options.targetId || options.approvalId || action.id || title);
  return {
    id: compactText(options.id || `mcp-event-${Date.now()}`),
    time: formatShortTime(at),
    at,
    title,
    text: compactText(options.text || options.summary || action.confirmText || "已写入当前会话上下文。"),
    tone: compactText(options.tone || (action.destructive ? "danger" : action.mutation ? "warning" : "info")) || "info",
    source: "mcp",
    hostId: compactText(options.hostId || options.scope?.hostId || ""),
    targetType: compactText(options.targetType || (options.approvalId ? "mcp_approval" : "mcp_action")) || "mcp_action",
    targetId,
    raw: {
      action,
      ...options,
    },
  };
}

export function buildMcpDecisionNotice(action = {}, decision = "accept") {
  const label = formatMcpActionLabel(action);
  if (decision === "decline" || decision === "reject") {
    return `${label} 已拒绝，不会继续执行。`;
  }
  if (decision === "accept_session") {
    return `${label} 已授权当前会话继续执行。`;
  }
  return `${label} 已通过审批，执行结果会在当前会话回写。`;
}
