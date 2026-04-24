import {
  buildWorkspaceHostRows,
  buildWorkspaceLiveTimeline,
  buildWorkspaceStepItems,
  cleanAssistantDisplayText,
  compactText,
  formatShortTime,
  formatTime,
  getWorkspacePlanDetail,
  isApprovalCard,
  isAssistantMessageCard,
  isChoiceCard,
  isDispatchSummaryCard,
  isInternalRoutingMessageText,
  isMissionNoticeCard,
  isPlanCard,
  isProcessCard,
  isSystemNoticeCard,
  isUserMessageCard,
  isWorkspaceResultCard,
  orderedObjectRows,
  parseTimestamp,
  pickField,
  sortApprovalDisplayItems,
  sortBackgroundAgentItems,
  statusTone,
} from "./workspaceViewModel";
import { formatProtocolChatTurns } from "./chatTurnFormatter";

function asArray(value) {
  return Array.isArray(value) ? value : [];
}

function asObject(value) {
  return value && typeof value === "object" && !Array.isArray(value) ? value : {};
}

function firstNonEmptyCompact(...values) {
  for (const value of values) {
    const text = compactText(value);
    if (text) return text;
  }
  return "";
}

function normalizeToolAliases(value) {
  return [...new Set(asArray(value).map((item) => compactText(item)).filter(Boolean))];
}

function legacyWorkspaceToolDisplayName(name = "") {
  switch (compactText(name)) {
    case "ask_user_question":
      return "澄清问题";
    case "command":
      return "命令执行";
    case "request_approval":
      return "审批请求";
    case "readonly_host_inspect":
      return "只读主机检查";
    case "query_ai_server_state":
      return "工作台状态快照";
    case "web_search":
      return "外部搜索";
    case "open_page":
      return "网页读取";
    case "find_in_page":
      return "页面定位";
    case "orchestrator_dispatch_tasks":
      return "任务派发";
    case "enter_plan_mode":
      return "进入计划模式";
    case "update_plan":
      return "计划更新";
    case "exit_plan_mode":
      return "计划审批";
    case "service_restart":
    case "service.restart":
      return "服务重启";
    case "service_stop":
    case "service.stop":
      return "停止服务";
    case "config_apply":
    case "config.apply":
      return "配置下发";
    case "package_install":
    case "package.install":
      return "安装软件包";
    case "package_upgrade":
    case "package.upgrade":
      return "升级软件包";
    case "execute_system_mutation":
      return "受控变更";
    default:
      return compactText(name);
  }
}

export function resolveWorkspaceToolDescriptor(value = "") {
  const source = typeof value === "string" ? { name: value } : asObject(value);
  const descriptor = asObject(source.descriptor);
  const name = firstNonEmptyCompact(source.name, source.tool, descriptor.name, descriptor.tool);
  const displayName = firstNonEmptyCompact(
    source.displayName,
    source.label,
    descriptor.displayName,
    descriptor.label,
    descriptor.title,
    legacyWorkspaceToolDisplayName(name),
  );
  return {
    name,
    displayName: displayName || "工具调用",
    kind: firstNonEmptyCompact(source.kind, descriptor.kind),
    description: firstNonEmptyCompact(source.description, source.summary, descriptor.description, descriptor.summary),
    aliases: [...new Set([...normalizeToolAliases(source.aliases), ...normalizeToolAliases(descriptor.aliases)])],
  };
}

export function resolveWorkspaceToolLabel(value = "") {
  return resolveWorkspaceToolDescriptor(value).displayName || "工具调用";
}

function normalizeProtocolProjectionLinks(links = []) {
  const seen = new Set();
  return asArray(links)
    .map((link) => {
      const source = asObject(link);
      const kind = compactText(source.kind).toLowerCase();
      const id = compactText(source.id);
      if (!kind || !id) return null;
      return {
        kind,
        id,
        relation: compactText(source.relation),
        label: compactText(source.label || source.title || ""),
        hostId: compactText(source.hostId),
      };
    })
    .filter((link) => {
      if (!link) return false;
      const key = `${link.kind}:${link.id}:${link.relation || ""}`;
      if (seen.has(key)) return false;
      seen.add(key);
      return true;
    });
}

function buildProtocolProjection({
  kind = "",
  id = "",
  aliases = [],
  title = "",
  summary = "",
  hostId = "",
  hostName = "",
  status = "",
  tone = "",
  timeLabel = "",
  timestamp = 0,
  links = [],
} = {}) {
  const projectionKind = compactText(kind).toLowerCase();
  const projectionId = compactText(id);
  if (!projectionKind || !projectionId) return null;
  const normalizedAliases = [...new Set([projectionId, ...asArray(aliases).map((item) => compactText(item)).filter(Boolean)])];
  return {
    kind: projectionKind,
    id: projectionId,
    aliases: normalizedAliases,
    title: compactText(title),
    summary: compactText(summary),
    hostId: compactText(hostId),
    hostName: compactText(hostName),
    status: compactText(status),
    tone: compactText(tone),
    timeLabel: compactText(timeLabel),
    timestamp: Number.isFinite(timestamp) ? timestamp : parseTimestamp(timestamp),
    links: normalizeProtocolProjectionLinks(links),
  };
}

function appendProtocolProjectionLinks(item = null, links = []) {
  if (!item?.projection) return item;
  return {
    ...item,
    projection: buildProtocolProjection({
      ...item.projection,
      links: [...asArray(item.projection.links), ...asArray(links)],
    }),
  };
}

function eventTargetProjectionKind(value = "") {
  const targetType = compactText(value).toLowerCase();
  switch (targetType) {
    case "approval":
    case "mcp_approval":
      return "approval";
    case "verification":
      return "verification";
    case "tool_invocation":
      return "tool_invocation";
    case "command":
      return "command";
    case "host":
      return "host";
    case "dispatch":
      return "dispatch";
    case "choice":
      return "choice";
    default:
      return targetType;
  }
}

function normalizeWorkspaceCopy(value) {
  return compactText(value)
    .replace(/PlannerSession/gi, "主 Agent Session")
    .replace(/Planner\s*trace/gi, "执行记录")
    .replace(/planner\s*trace/gi, "执行记录")
    .replace(/Planner/gi, "主 Agent")
    .replace(/planner/gi, "主 Agent")
    .replace(/影子\s*session/gi, "内部会话")
    .replace(/shadow\s*session/gi, "内部会话")
    .replace(/route\s*thread/gi, "主链路")
    .replace(/Route\s*Thread/gi, "主链路");
}

function buildInvocationEvidenceIndex(invocations, evidenceSummaries) {
  const index = {};
  for (const inv of invocations || []) {
    if (inv.evidenceId) {
      index[inv.id] = inv.evidenceId;
    }
  }
  for (const ev of evidenceSummaries || []) {
    if (ev.invocationId) {
      index[ev.invocationId] = ev.id;
    }
  }
  return index;
}

function normalizePlanDetailValue(value) {
  if (value === null || value === undefined) return "";
  if (Array.isArray(value)) {
    return value.map((item) => normalizePlanDetailValue(item)).filter(Boolean).join(" / ");
  }
  if (typeof value === "object") {
    return orderedObjectRows(value).map((row) => `${row.key}: ${row.value}`).filter(Boolean).join(" / ");
  }
  return normalizeWorkspaceCopy(value);
}

function findLastIndex(list = [], predicate = () => false) {
  for (let index = list.length - 1; index >= 0; index -= 1) {
    if (predicate(list[index], index)) return index;
  }
  return -1;
}

function findLast(list = [], predicate = () => false) {
  const index = findLastIndex(list, predicate);
  return index >= 0 ? list[index] : null;
}

function isStoppedNoticeCard(card) {
  const title = compactText(card?.title).toLowerCase();
  const text = compactText(card?.text || card?.message).toLowerCase();
  return card?.type === "NoticeCard" && (title === "mission stopped" || text.includes("mission 已停止"));
}

function isFailedResultSummaryCard(card) {
  return card?.type === "ResultSummaryCard" && compactText(card?.status).toLowerCase() === "failed";
}

function isVerificationCard(card) {
  return card?.type === "VerificationCard";
}

function isRollbackCard(card) {
  return card?.type === "RollbackCard";
}

function normalizeEvidenceRows(value, { defaultLabel = "" } = {}) {
  if (!value) return [];
  if (Array.isArray(value)) {
    return value
      .flatMap((item) => {
        if (item == null) return [];
        if (typeof item === "string") {
          const text = compactText(item);
          return text ? [{ label: defaultLabel || "", value: text, text }] : [];
        }
        if (typeof item === "object") {
          const label = compactText(item.label || item.key || item.name || item.title || defaultLabel);
          const valueText = compactText(item.value ?? item.text ?? item.summary ?? item.content ?? item.detail ?? "");
          const description = compactText(item.description || item.note || item.reason || "");
          const rows = [];
          if (label || valueText || description) {
            rows.push({
              label,
              value: valueText || description,
              text: description && description !== valueText ? description : "",
            });
          }
          return rows;
        }
        return [];
      })
      .filter((row) => row.label || row.value || row.text);
  }
  if (typeof value === "object") {
    return orderedObjectRows(value).map((row) => ({
      label: row.key,
      value: row.value,
      text: "",
    }));
  }
  const text = compactText(value);
  return text ? [{ label: defaultLabel || "", value: text, text }] : [];
}

function summarizeHostStepItems(planCardModel = null, hostRow = null) {
  const stepItems = asArray(planCardModel?.stepItems);
  if (!stepItems.length || !hostRow) return [];
  const hostId = compactText(hostRow.hostId);
  const hostName = compactText(hostRow.displayName);
  return stepItems.filter((step) => {
    if (asArray(step.hosts).some((host) => compactText(host.id) === hostId)) return true;
    const haystack = `${step.title} ${step.summary} ${step.raw}`.toLowerCase();
    return hostId && haystack.includes(hostId.toLowerCase()) || hostName && haystack.includes(hostName.toLowerCase());
  });
}

function buildDispatchTimelineRows(dispatch = {}) {
  const timeline = pickField(dispatch, "timeline");
  const rows = [];
  for (const item of normalizeEvidenceRows(timeline, { defaultLabel: "事件" })) {
    rows.push({
      label: item.label || "事件",
      value: item.value,
      text: item.text,
    });
  }
  return rows;
}

function buildTaskBindingRows(binding = {}) {
  const rows = [];
  if (!binding || typeof binding !== "object") return rows;
  const orderedKeys = [
    ["taskId", "Task"],
    ["hostId", "Host"],
    ["workerHostId", "Worker Host"],
    ["title", "标题"],
    ["instruction", "指令"],
    ["status", "状态"],
    ["approvalState", "审批状态"],
    ["lastReply", "最后回复"],
    ["lastError", "最后错误"],
    ["externalNodeId", "External Node"],
    ["threadId", "Thread"],
    ["sessionId", "Session"],
  ];
  for (const [key, label] of orderedKeys) {
    const value = compactText(binding[key]);
    if (!value) continue;
    rows.push({ label, value });
  }
  const constraints = asArray(binding.constraints);
  if (constraints.length) {
    rows.push({
      label: "约束",
      value: constraints.map((item) => compactText(item)).filter(Boolean).join(" / "),
    });
  }
  return rows;
}

function buildApprovalAnchorRows(anchor = {}) {
  const rows = [];
  if (!anchor || typeof anchor !== "object") return rows;
  const orderedKeys = [
    ["approvalId", "Approval"],
    ["itemId", "Item"],
    ["sourceCardId", "Source Card"],
    ["hostId", "Host"],
    ["type", "Type"],
    ["title", "Title"],
    ["command", "Command"],
    ["cwd", "Cwd"],
    ["status", "Status"],
    ["summary", "Summary"],
    ["reason", "Reason"],
    ["createdAt", "Created At"],
    ["updatedAt", "Updated At"],
  ];
  for (const [key, label] of orderedKeys) {
    const value = compactText(anchor[key]);
    if (!value) continue;
    rows.push({ label, value });
  }
  return rows;
}

function formatApprovalDetailValue(value, { booleanTrue = "是", booleanFalse = "否" } = {}) {
  if (value === null || value === undefined) return "";
  if (typeof value === "boolean") return value ? booleanTrue : booleanFalse;
  if (Array.isArray(value)) {
    return value.map((item) => compactText(typeof item === "object" ? JSON.stringify(item) : item)).filter(Boolean).join(" / ");
  }
  if (typeof value === "object") {
    const rows = orderedObjectRows(value);
    if (rows.length) {
      return rows.map((row) => `${row.key}: ${row.value}`).join(" / ");
    }
    try {
      return JSON.stringify(value);
    } catch {
      return "";
    }
  }
  return compactText(value);
}

function buildProtocolApprovalDetailRows(detail = {}) {
  const source = asObject(detail);
  if (!Object.keys(source).length) return [];

  const rows = [];
  const addRow = (label, value, options = {}) => {
    const text = formatApprovalDetailValue(value, options);
    if (!text) return;
    rows.push({ label, value: text });
  };

  addRow("风险级别", pickField(source, "riskLevel", "risk"));
  addRow("目标范围", pickField(source, "targetSummary", "target"));
  addRow("目标环境", pickField(source, "targetEnvironment", "target_environment", "environment", "env"));
  addRow("影响面", pickField(source, "blastRadius", "blast_radius", "expectedImpact", "expected_impact"));
  if (source.dryRunSupported === true || source.dry_run_supported === true) {
  addRow("Dry-run", true, { booleanTrue: "支持" });
  }
  addRow("Dry-run 摘要", pickField(source, "dryRunSummary", "dry_run_summary"));
  addRow("回滚提示", pickField(source, "rollbackHint", "rollback_hint", "rollbackSuggestion", "rollback_suggestion", "rollback"));
  addRow("验证策略", pickField(source, "verifyStrategies", "verify_strategies", "validation"));
  addRow("验证来源", pickField(source, "verificationSources", "verification_sources"));

  return rows;
}

function verificationStatusLabel(value = "") {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "running":
      return "验证中";
    case "passed":
      return "验证通过";
    case "failed":
      return "验证失败";
    case "inconclusive":
      return "验证结论不充分";
    default:
      return compactText(value) || "验证中";
  }
}

function verificationTone(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (normalized === "passed") return "success";
  if (normalized === "failed") return "danger";
  if (normalized === "running" || normalized === "pending") return "warning";
  return "neutral";
}

function verificationTimestamp(record = {}) {
  const source = asObject(record);
  return parseTimestamp(
    pickField(
      source,
      "verificationCompletedAt",
      "endedAt",
      "verificationStartedAt",
      "startedAt",
    ) || pickField(
      asObject(source.metadata),
      "verificationCompletedAt",
      "endedAt",
      "verificationStartedAt",
      "startedAt",
    ) || source.createdAt,
  );
}

function normalizeProtocolVerificationRecord(record = {}) {
  const source = asObject(record);
  const metadata = asObject(source.metadata);
  const findings = asArray(source.findings).map((item) => compactText(item)).filter(Boolean);
  const successCriteria = asArray(source.successCriteria).map((item) => compactText(item)).filter(Boolean);
  const verificationSources = asArray(
    pickField(source, "verificationSources", "verification_sources")
      || pickField(metadata, "verificationSources", "verification_sources"),
  ).map((item) => compactText(item)).filter(Boolean);
  const hostId = compactText(pickField(source, "hostId", "host_id") || pickField(metadata, "hostId", "host_id"));
  const targetSummary = compactText(
    pickField(source, "targetSummary", "target_summary") || pickField(metadata, "targetSummary", "target_summary", "filePath", "command", "summary"),
  );
  const status = compactText(source.status).toLowerCase() || "running";
  const completedAt = compactText(
    pickField(source, "verificationCompletedAt", "endedAt", "verificationStartedAt", "startedAt")
      || pickField(metadata, "verificationCompletedAt", "endedAt", "verificationStartedAt", "startedAt")
      || source.createdAt,
  );
  const id = compactText(source.id);
  const approvalId = compactText(pickField(source, "approvalId", "approval_id") || pickField(metadata, "approvalId", "approval_id"));
  const commandCardId = compactText(
    pickField(source, "commandCardId", "command_card_id", "cardId", "card_id")
      || pickField(metadata, "commandCardId", "command_card_id", "cardId", "card_id"),
  );
  const evidenceId = compactText(pickField(source, "evidenceId", "evidence_id") || pickField(metadata, "evidenceId", "evidence_id"));
  const actionEventId = compactText(pickField(source, "actionEventId", "action_event_id") || pickField(metadata, "actionEventId", "action_event_id"));
  const timeLabel = formatShortTime(completedAt);
  const timestamp = verificationTimestamp(source);
  return {
    id,
    status,
    statusLabel: verificationStatusLabel(status),
    tone: verificationTone(status),
    strategy: compactText(source.strategy),
    successCriteria,
    verificationSources,
    findings,
    rollbackHint: compactText(source.rollbackHint),
    nextStepSuggestion: compactText(pickField(source, "nextStepSuggestion", "next_step_suggestion") || pickField(metadata, "nextStepSuggestion", "next_step_suggestion")),
    hostId,
    hostName: compactText(pickField(source, "hostName", "host_name") || pickField(metadata, "hostName", "host_name")) || hostId,
    approvalId,
    commandCardId,
    actionEventId,
    targetSummary,
    evidenceId,
    timeLabel,
    timestamp,
    projection: buildProtocolProjection({
      kind: "verification",
      id,
      aliases: [actionEventId, approvalId, commandCardId],
      title: verificationStatusLabel(status),
      summary: compactText(targetSummary || findings[0] || source.strategy || "自动验证结果"),
      hostId,
      hostName: compactText(pickField(source, "hostName", "host_name") || pickField(metadata, "hostName", "host_name")) || hostId,
      status,
      tone: verificationTone(status),
      timeLabel,
      timestamp,
      links: [
        hostId ? { kind: "host", id: hostId, relation: "host", label: compactText(pickField(source, "hostName", "host_name") || pickField(metadata, "hostName", "host_name")) || hostId, hostId } : null,
        approvalId ? { kind: "approval", id: approvalId, relation: "approval", label: "关联审批", hostId } : null,
        commandCardId ? { kind: "approval", id: commandCardId, relation: "approval_card", label: "关联审批卡", hostId } : null,
        evidenceId ? { kind: "evidence", id: evidenceId, relation: "evidence", label: "关联证据", hostId } : null,
      ],
    }),
    raw: source,
  };
}

function buildProtocolVerificationRecords(records = []) {
  return asArray(records)
    .map((record) => normalizeProtocolVerificationRecord(record))
    .filter((record) => record.id)
    .sort((left, right) => (right.timestamp || 0) - (left.timestamp || 0));
}

function buildProtocolVerificationEventItems(records = []) {
  return buildProtocolVerificationRecords(records).map((record) => {
    const eventId = `verification-${record.id}`;
    return {
      id: eventId,
      time: record.timeLabel || "",
      timestamp: record.timestamp || 0,
      title: record.statusLabel,
      text: normalizeWorkspaceCopy(record.targetSummary || record.findings[0] || record.strategy || "自动验证结果"),
      detail: normalizeWorkspaceCopy(record.findings.join(" / ")),
      tone: record.tone,
      targetType: "verification",
      targetId: record.id,
      hostId: record.hostId,
      projection: buildProtocolProjection({
        kind: "event",
        id: eventId,
        title: record.statusLabel,
        summary: normalizeWorkspaceCopy(record.targetSummary || record.findings[0] || record.strategy || "自动验证结果"),
        hostId: record.hostId,
        hostName: record.hostName,
        status: record.status,
        tone: record.tone,
        timeLabel: record.timeLabel || "",
        timestamp: record.timestamp || 0,
        links: [
          { kind: "verification", id: record.id, relation: "event_target", label: "验证结果", hostId: record.hostId },
          record.hostId ? { kind: "host", id: record.hostId, relation: "host", label: record.hostName || record.hostId, hostId: record.hostId } : null,
        ],
      }),
    };
  });
}

function incidentEventTone(source = {}) {
  const type = compactText(source?.type).toLowerCase();
  const status = compactText(source?.status).toLowerCase();
  if (type === "cancel.partial_failure" || type === "cancel.signal_failed") return "warning";
  if (status.includes("fail") || status.includes("error")) return "danger";
  if (status.includes("warn")) return "warning";
  if (status.includes("complete") || status.includes("success")) return "success";
  return "neutral";
}

function buildProtocolIncidentEventItems(incidentEvents = [], commandCards = [], approvalCards = [], hostRows = []) {
  const commandById = new Map(asArray(commandCards).map((card) => [compactText(card?.id), card]));
  const approvalById = new Map(asArray(approvalCards).flatMap((card) => {
    const ids = [
      compactText(card?.id),
      compactText(card?.approval?.requestId),
      compactText(card?.approvalId),
      compactText(card?.requestId),
    ].filter(Boolean);
    return ids.map((id) => [id, card]);
  }));
  const hostLabelById = new Map(asArray(hostRows).map((row) => [compactText(row?.hostId), compactText(row?.displayName || row?.hostId)]));
  return asArray(incidentEvents)
    .filter((event) => {
      const type = compactText(event?.type).toLowerCase();
      return type === "cancel.signal_failed" || type === "cancel.partial_failure";
    })
    .map((event) => {
      const metadata = asObject(event?.metadata);
      const sourceId = compactText(event?.id || `${event?.type}-${event?.createdAt}`);
      const hostId = compactText(event?.hostId || metadata?.hostId);
      const hostName = compactText(hostLabelById.get(hostId)) || hostId;
      const approvalId = compactText(event?.approvalId || metadata?.approvalId);
      const commandCardId = compactText(metadata?.cardId);
      const verificationId = compactText(event?.verification || metadata?.verificationId);
      const commandCard = commandCardId ? commandById.get(commandCardId) || null : null;
      const approvalCard = approvalId ? approvalById.get(approvalId) || null : null;
      let targetType = "";
      let targetId = "";
      if (commandCardId) {
        targetType = "command";
        targetId = commandCardId;
      } else if (approvalCard || approvalId) {
        targetType = "approval";
        targetId = compactText(approvalCard?.id || approvalId);
      } else if (verificationId) {
        targetType = "verification";
        targetId = verificationId;
      } else if (hostId) {
        targetType = "host";
        targetId = hostId;
      }
      const title = normalizeWorkspaceCopy(event?.title || event?.type || "系统事件");
      const summary = normalizeWorkspaceCopy(event?.summary || title);
      const timeLabel = formatShortTime(event?.createdAt);
      const timestamp = parseTimestamp(event?.createdAt);
      const eventId = `incident-${sourceId}`;
      return {
        id: eventId,
        time: timeLabel,
        timestamp,
        title,
        text: summary,
        detail: summary,
        tone: incidentEventTone(event),
        targetType,
        targetId,
        hostId,
        status: compactText(event?.status),
        commandCard,
        incidentEvent: event,
        projection: buildProtocolProjection({
          kind: "event",
          id: eventId,
          aliases: [sourceId, approvalId, commandCardId, verificationId],
          title,
          summary,
          hostId,
          hostName,
          status: compactText(event?.status),
          tone: incidentEventTone(event),
          timeLabel,
          timestamp,
          links: [
            targetType && targetId
              ? {
                  kind: eventTargetProjectionKind(targetType),
                  id: targetId,
                  relation: "event_target",
                  label: targetType,
                  hostId,
                }
              : null,
            hostId ? { kind: "host", id: hostId, relation: "host", label: hostName || hostId, hostId } : null,
          ],
        }),
      };
    });
}

function stripMatchingQuotes(value = "") {
  const text = String(value || "").trim();
  if (text.length >= 2 && ((text.startsWith("'") && text.endsWith("'")) || (text.startsWith('"') && text.endsWith('"')))) {
    return text.slice(1, -1);
  }
  return text;
}

function displayCommand(value = "") {
  const raw = String(value || "").trim();
  if (!raw) return "";
  const shellMatch = raw.match(/^(?:\/[\w./-]+\/)?(?:zsh|bash|sh)\s+-lc\s+([\s\S]+)$/);
  if (shellMatch) return stripMatchingQuotes(shellMatch[1]);
  return raw;
}

function commandStatusLabel(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (normalized.includes("permission") || normalized.includes("denied")) return "权限不足";
  if (normalized.includes("fail") || normalized.includes("error")) return "失败";
  if (normalized.includes("complete") || normalized.includes("done")) return "已完成";
  if (normalized.includes("run") || normalized.includes("progress")) return "执行中";
  return compactText(value) || "已处理";
}

function commandTone(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (normalized.includes("permission") || normalized.includes("denied") || normalized.includes("fail") || normalized.includes("error")) return "danger";
  if (normalized.includes("complete") || normalized.includes("done")) return "success";
  if (normalized.includes("wait") || normalized.includes("pending")) return "warning";
  return "neutral";
}

function commandOutputPreview(card = {}) {
  const output = compactText(card.output || card.stdout || card.stderr || card.text || card.summary);
  return output ? output.slice(0, 180) : "命令没有输出。";
}

function safeParseJSON(value) {
  if (!value || typeof value !== "string") return null;
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

function invocationStatusLabel(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (normalized === "waiting_user") return "等待用户确认";
  if (normalized === "waiting_approval") return "等待审批";
  return commandStatusLabel(value);
}

function invocationTone(value = "") {
  const normalized = compactText(value).toLowerCase();
  if (normalized === "waiting_user" || normalized === "waiting_approval") return "warning";
  return commandTone(value);
}

function normalizeProtocolEvidenceSummary(record = {}) {
  const source = asObject(record);
  const metadata = asObject(source.metadata);
  const id = compactText(source.id);
  const citationKey = compactText(source.citationKey);
  const invocationId = compactText(source.invocationId || metadata.invocationId);
  const relatedEvidenceIds = asArray(source.relatedEvidenceIds || metadata.relatedEvidenceIds).map((item) => compactText(item)).filter(Boolean);
  const hostId = compactText(source.hostId || metadata.hostId);
  const hostName = compactText(source.hostName || metadata.hostName) || hostId;
  const createdAt = compactText(source.createdAt);
  return {
    ...source,
    id,
    citationKey,
    invocationId,
    relatedEvidenceIds,
    sourceKind: compactText(source.sourceKind || source.source_kind),
    sourceRef: compactText(source.sourceRef || source.source_ref),
    title: compactText(source.title),
    summary: compactText(source.summary),
    content: source.content ?? source.text ?? "",
    metadata,
    projection: buildProtocolProjection({
      kind: "evidence",
      id,
      aliases: [citationKey],
      title: compactText(source.title || citationKey || id),
      summary: compactText(source.summary || source.title || citationKey),
      hostId,
      hostName,
      status: compactText(source.kind || source.sourceKind || source.source_kind),
      tone: "neutral",
      timeLabel: formatShortTime(createdAt),
      timestamp: parseTimestamp(createdAt),
      links: [
        ...relatedEvidenceIds.map((evidenceId) => ({ kind: "evidence", id: evidenceId, relation: "related_evidence", label: "关联证据" })),
        invocationId ? { kind: "tool_invocation", id: invocationId, relation: "source_invocation", label: "来源工具调用" } : null,
        compactText(source.sourceKind || source.source_kind) === "tool_invocation" && compactText(source.sourceRef || source.source_ref)
          ? { kind: "tool_invocation", id: compactText(source.sourceRef || source.source_ref), relation: "source_ref", label: "来源工具调用" }
          : null,
        compactText(metadata.approvalId) ? { kind: "approval", id: compactText(metadata.approvalId), relation: "related_approval", label: "关联审批" } : null,
        compactText(metadata.cardId) ? { kind: "approval", id: compactText(metadata.cardId), relation: "related_approval_card", label: "关联审批卡" } : null,
      ],
    }),
  };
}

function normalizeProtocolEvidenceSummaries(records = []) {
  return asArray(records)
    .map((record) => normalizeProtocolEvidenceSummary(record))
    .filter((record) => record.id);
}

function normalizeProtocolToolInvocations(toolInvocations = [], evidenceSummaries = []) {
  const evidenceById = new Map(asArray(evidenceSummaries).map((item) => [compactText(item?.id), item]));
  return asArray(toolInvocations)
    .map((item) => {
      const evidenceId = compactText(item?.evidenceId);
      const evidence = evidenceById.get(evidenceId) || null;
      const input = safeParseJSON(item?.inputJson) || {};
      const output = safeParseJSON(item?.outputJson) || {};
      const metadata = asObject(evidence?.metadata);
      const tool = resolveWorkspaceToolDescriptor({
        name: item?.name,
        displayName: item?.displayName || metadata.displayName,
        kind: item?.kind || metadata.kind,
        description: metadata.description,
        aliases: metadata.aliases,
      });
      const hostId = compactText(input.hostId || item?.hostId || output.hostId || metadata.hostId);
      const hostName = compactText(metadata.hostName) || hostId;
      const approvalId = compactText(input.approvalId || item?.approvalId || metadata.approvalId);
      const approvalCardId = compactText(metadata.cardId);
      const timeLabel = formatShortTime(item?.completedAt || item?.startedAt || evidence?.createdAt);
      const timestamp = parseTimestamp(item?.completedAt || item?.startedAt || evidence?.createdAt);
      return {
        ...item,
        id: compactText(item?.id),
        name: tool.name,
        displayName: tool.displayName,
        kind: tool.kind,
        status: compactText(item?.status),
        input,
        output,
        inputSummary: compactText(item?.inputSummary),
        outputSummary: compactText(item?.outputSummary),
        evidenceId,
        evidence,
        hostId,
        projection: buildProtocolProjection({
          kind: "tool_invocation",
          id: compactText(item?.id),
          title: tool.displayName,
          summary: compactText(item?.inputSummary || item?.outputSummary || evidence?.summary),
          hostId,
          hostName,
          status: compactText(item?.status),
          tone: invocationTone(item?.status),
          timeLabel,
          timestamp,
          links: [
            hostId ? { kind: "host", id: hostId, relation: "host", label: hostName || hostId, hostId } : null,
            evidenceId ? { kind: "evidence", id: evidenceId, relation: "evidence", label: "关联证据", hostId } : null,
            approvalId ? { kind: "approval", id: approvalId, relation: "approval", label: "关联审批", hostId } : null,
            approvalCardId ? { kind: "approval", id: approvalCardId, relation: "approval_card", label: "关联审批卡", hostId } : null,
          ],
        }),
      };
    })
    .filter((item) => item.id && item.name);
}

function buildProtocolToolInvocationEventItems(toolInvocations = [], hostRows = []) {
  const hostLabelById = new Map(asArray(hostRows).map((row) => [compactText(row.hostId), compactText(row.displayName || row.hostId)]));
  return asArray(toolInvocations)
    .slice(-18)
    .map((invocation) => {
      const input = asObject(invocation.input);
      const output = asObject(invocation.output);
      const hostId = compactText(input.hostId || invocation.hostId || output.hostId || "server-local");
      const hostLabel = compactText(hostLabelById.get(hostId)) || (hostId === "server-local" ? "local" : hostId);
      const command = displayCommand(input.command || invocation.inputSummary);
      const toolLabel = resolveWorkspaceToolLabel(invocation);
      const statusLabel = invocationStatusLabel(invocation.status);
      const title = invocation.name === "command" && command
        ? `${hostLabel} · ${command}`
        : `${toolLabel} · ${invocation.inputSummary || invocation.evidence?.title || invocation.id}`;
      const detail = compactText(invocation.outputSummary || invocation.evidence?.summary || statusLabel);
      return {
        id: `tool-invocation-${invocation.id}`,
        time: formatShortTime(invocation.completedAt || invocation.startedAt || invocation.evidence?.createdAt),
        timestamp: parseTimestamp(invocation.completedAt || invocation.startedAt || invocation.evidence?.createdAt),
        title,
        text: `${statusLabel}${detail ? ` · ${detail}` : ""}`,
        detail: detail || statusLabel,
        tone: invocationTone(invocation.status),
        targetType: "tool_invocation",
        targetId: invocation.id,
        evidenceId: invocation.evidenceId,
        hostId,
        toolName: invocation.name,
        command,
        status: invocation.status,
        invocation,
        evidence: invocation.evidence,
        projection: buildProtocolProjection({
          kind: "event",
          id: `tool-invocation-${invocation.id}`,
          title,
          summary: `${statusLabel}${detail ? ` · ${detail}` : ""}`,
          hostId,
          hostName: hostLabel,
          status: invocation.status,
          tone: invocationTone(invocation.status),
          timeLabel: formatShortTime(invocation.completedAt || invocation.startedAt || invocation.evidence?.createdAt),
          timestamp: parseTimestamp(invocation.completedAt || invocation.startedAt || invocation.evidence?.createdAt),
          links: [
            { kind: "tool_invocation", id: invocation.id, relation: "event_target", label: toolLabel, hostId },
            invocation.evidenceId ? { kind: "evidence", id: invocation.evidenceId, relation: "evidence", label: "关联证据", hostId } : null,
            hostId ? { kind: "host", id: hostId, relation: "host", label: hostLabel, hostId } : null,
          ],
        }),
      };
    });
}

function buildProtocolCommandEventItems(commandCards = [], hostRows = []) {
  const hostLabelById = new Map(asArray(hostRows).map((row) => [compactText(row.hostId), compactText(row.displayName || row.hostId)]));
  return asArray(commandCards)
    .filter((card) => compactText(card?.command) && !compactText(asObject(card?.detail).tool))
    .slice(-14)
    .map((card) => {
      const hostId = compactText(card.hostId || "server-local");
      const rowHostLabel = compactText(hostLabelById.get(hostId));
      const hostLabel = rowHostLabel || (hostId === "server-local" ? "local" : compactText(card.hostName || hostId || "local"));
      const command = displayCommand(card.command);
      const statusLabel = commandStatusLabel(card.status);
      return {
        id: `command-${card.id || command}`,
        time: formatShortTime(card.updatedAt || card.createdAt),
        timestamp: parseTimestamp(card.updatedAt || card.createdAt),
        title: `${hostLabel} · ${command}`,
        text: `${statusLabel} · ${commandOutputPreview(card)}`,
        detail: `${statusLabel} · ${commandOutputPreview(card)}`,
        tone: commandTone(card.status),
        targetType: "command",
        targetId: compactText(card.id),
        hostId,
        command,
        status: compactText(card.status),
        output: card.output || card.stdout || card.stderr || "",
        cwd: compactText(card.cwd),
        exitCode: card.exitCode,
        durationMs: card.durationMs,
        commandCard: card,
        projection: buildProtocolProjection({
          kind: "event",
          id: `command-${card.id || command}`,
          title: `${hostLabel} · ${command}`,
          summary: `${statusLabel} · ${commandOutputPreview(card)}`,
          hostId,
          hostName: hostLabel,
          status: compactText(card.status),
          tone: commandTone(card.status),
          timeLabel: formatShortTime(card.updatedAt || card.createdAt),
          timestamp: parseTimestamp(card.updatedAt || card.createdAt),
          links: [
            compactText(card.id) ? { kind: "command", id: compactText(card.id), relation: "event_target", label: command, hostId } : null,
            hostId ? { kind: "host", id: hostId, relation: "host", label: hostLabel, hostId } : null,
          ],
        }),
      };
    });
}

export function normalizeProtocolMissionPhase(value) {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "执行中":
    case "running":
    case "inprogress":
    case "in_progress":
      return "executing";
    case "规划中":
    case "planning":
      return "planning";
    case "思考中":
    case "thinking":
      return "thinking";
    case "等待审批":
    case "waitingapproval":
    case "waiting_approval":
      return "waiting_approval";
    case "等待补充输入":
    case "等待输入":
    case "waitinginput":
    case "waiting_input":
      return "waiting_input";
    case "汇总中":
    case "finalizing":
      return "finalizing";
    case "已完成":
    case "completed":
      return "completed";
    case "失败":
    case "failed":
      return "failed";
    case "已停止":
    case "aborted":
      return "aborted";
    case "待命":
    case "idle":
      return "idle";
    default:
      return normalized || "idle";
  }
}

function normalizeProtocolIncidentMode(value) {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "analysis":
    case "readonly":
    case "read_only":
    case "answer":
    case "plan":
      return "analysis";
    case "execute":
    case "executing":
    case "execution":
      return "execute";
    default:
      return normalized;
  }
}

function incidentModeLabel(value) {
  switch (normalizeProtocolIncidentMode(value)) {
    case "analysis":
      return "分析模式";
    case "execute":
      return "执行模式";
    default:
      return compactText(value) || "分析模式";
  }
}

function normalizeProtocolIncidentStage(value) {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "thinking":
      return "understanding";
    case "waiting_approval":
    case "waitingapproval":
      return "waiting_action_approval";
    case "aborted":
      return "canceled";
    default:
      return normalized;
  }
}

function incidentStageLabel(value) {
  switch (normalizeProtocolIncidentStage(value)) {
    case "understanding":
      return "问题理解";
    case "planning":
      return "生成计划";
    case "collecting_evidence":
      return "证据收集";
    case "analyzing":
      return "归因分析";
    case "waiting_plan_approval":
      return "等待计划审批";
    case "executing":
      return "执行中";
    case "waiting_action_approval":
      return "等待动作审批";
    case "waiting_input":
      return "等待补充输入";
    case "verifying":
      return "自动验证";
    case "rollback_suggested":
      return "回滚建议";
    case "finalizing":
      return "汇总中";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "canceled":
      return "已停止";
    default:
      return compactText(value) || "待命";
  }
}

function normalizeTurnIntentClass(value) {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "snapshot":
    case "factual":
    case "research":
    case "design":
    case "implementation":
    case "risky_exec":
    case "ambiguous":
      return normalized;
    default:
      return normalized || "factual";
  }
}

function turnIntentLabel(value) {
  switch (normalizeTurnIntentClass(value)) {
    case "snapshot":
      return "状态快照";
    case "factual":
      return "事实问答";
    case "research":
      return "资料调研";
    case "design":
      return "方案设计";
    case "implementation":
      return "实现问题";
    case "risky_exec":
      return "高风险执行";
    case "ambiguous":
      return "意图待澄清";
    default:
      return compactText(value) || "事实问答";
  }
}

function normalizeTurnLane(value) {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "answer":
    case "readonly":
    case "plan":
    case "execute":
    case "verify":
      return normalized;
    default:
      return normalized || "";
  }
}

function turnLaneLabel(value) {
  switch (normalizeTurnLane(value)) {
    case "readonly":
      return "分析中";
    case "plan":
      return "方案规划中";
    case "execute":
      return "受控执行中";
    case "verify":
      return "自动验证中";
    case "answer":
      return "分析中";
    default:
      return compactText(value) || "分析中";
  }
}

function normalizeTurnFinalGateStatus(value) {
  const normalized = compactText(value).toLowerCase();
  switch (normalized) {
    case "pending":
    case "passed":
    case "blocked":
      return normalized;
    default:
      return normalized || "pending";
  }
}

function turnFinalGateLabel(value) {
  switch (normalizeTurnFinalGateStatus(value)) {
    case "blocked":
      return "已拦截";
    case "passed":
      return "已放行";
    case "pending":
    default:
      return "待校验";
  }
}

function normalizePromptEnvelopeSection(section = {}) {
  const source = asObject(section);
  const name = compactText(source.name || source.title);
  const content = compactText(source.content || source.text || source.summary);
  if (!name && !content) return null;
  return {
    name: name || "Section",
    content,
  };
}

function normalizePromptEnvelopeTool(tool = {}) {
  const source = asObject(tool);
  const descriptor = resolveWorkspaceToolDescriptor(source);
  const reason = compactText(source.reason || source.summary);
  if (!descriptor.name && !reason && !descriptor.description) return null;
  return {
    ...descriptor,
    reason,
  };
}

function findPromptEnvelopeTool(tools = [], name = "") {
  const normalizedName = compactText(name);
  if (!normalizedName) return null;
  return asArray(tools).find((tool) => {
    const descriptor = resolveWorkspaceToolDescriptor(tool);
    return descriptor.name === normalizedName || descriptor.aliases.includes(normalizedName);
  }) || null;
}

function normalizeTurnPolicy(policy = {}) {
  const source = asObject(policy);
  return {
    intentClass: normalizeTurnIntentClass(source.intentClass),
    lane: normalizeTurnLane(source.lane),
    requiredTools: asArray(source.requiredTools).map((item) => compactText(item)).filter(Boolean),
    requiredEvidenceKinds: asArray(source.requiredEvidenceKinds).map((item) => compactText(item)).filter(Boolean),
    needsPlanArtifact: source.needsPlanArtifact === true,
    needsApproval: source.needsApproval === true,
    needsAssumptions: source.needsAssumptions === true,
    needsDisambiguation: source.needsDisambiguation === true,
    requiresExternalFacts: source.requiresExternalFacts === true,
    requiresRealtimeData: source.requiresRealtimeData === true,
    minimumEvidenceCount: Number(source.minimumEvidenceCount || 0),
    requiredNextTool: compactText(source.requiredNextTool),
    finalGateStatus: normalizeTurnFinalGateStatus(source.finalGateStatus),
    missingRequirements: asArray(source.missingRequirements).map((item) => compactText(item)).filter(Boolean),
    classificationReason: compactText(source.classificationReason),
    updatedAt: compactText(source.updatedAt),
  };
}

function normalizePromptEnvelope(envelope = {}) {
  const source = asObject(envelope);
  const runtimePolicy = normalizePromptEnvelopeSection(source.runtimePolicy);
  return {
    staticSections: asArray(source.staticSections).map((item) => normalizePromptEnvelopeSection(item)).filter(Boolean),
    laneSections: asArray(source.laneSections).map((item) => normalizePromptEnvelopeSection(item)).filter(Boolean),
    runtimePolicy,
    contextAttachments: asArray(source.contextAttachments).map((item) => normalizePromptEnvelopeSection(item)).filter(Boolean),
    visibleTools: asArray(source.visibleTools).map((item) => normalizePromptEnvelopeTool(item)).filter(Boolean),
    hiddenTools: asArray(source.hiddenTools).map((item) => normalizePromptEnvelopeTool(item)).filter(Boolean),
    tokenEstimate: Number(source.tokenEstimate || 0),
    compressionState: compactText(source.compressionState || "inline"),
    currentLane: normalizeTurnLane(source.currentLane),
    intentClass: normalizeTurnIntentClass(source.intentClass),
    finalGateStatus: normalizeTurnFinalGateStatus(source.finalGateStatus),
    missingRequirements: asArray(source.missingRequirements).map((item) => compactText(item)).filter(Boolean),
    updatedAt: compactText(source.updatedAt),
  };
}

function inferProtocolIncidentMode({ snapshot = {}, missionPhase = "idle", approvalItems = [], verificationRecords = [] } = {}) {
  const explicitMode = normalizeProtocolIncidentMode(snapshot.currentMode);
  if (explicitMode) return explicitMode;

  const loopMode = normalizeProtocolIncidentMode(pickField(snapshot.agentLoop || {}, "mode"));
  if (loopMode) return loopMode;

  const executionEnabled = Boolean(pickField(snapshot.agentLoop || {}, "executionEnabled", "ExecutionEnabled"));
  const hasPlanApproval = asArray(approvalItems).some((item) => item.kind === "plan");
  const hasOperationApproval = asArray(approvalItems).some((item) => item.kind === "operation");
  if (hasPlanApproval) return "analysis";
  if (missionPhase === "waiting_approval" && hasOperationApproval) return "execute";
  if (executionEnabled) return "execute";
  if (verificationRecords.length) return "execute";
  if (["executing", "finalizing", "completed", "failed", "aborted"].includes(missionPhase)) return "execute";
  return "analysis";
}

function inferProtocolIncidentStage({
  snapshot = {},
  missionPhase = "idle",
  currentMode = "analysis",
  currentFailureCard = null,
  verificationRecords = [],
  cards = {},
} = {}) {
  const explicitStage = normalizeProtocolIncidentStage(snapshot.currentStage);
  if (explicitStage) return explicitStage;

  if (currentFailureCard) return "failed";
  if (cards.stopNoticeCard || missionPhase === "aborted") return "canceled";
  if (verificationRecords.some((item) => compactText(item.status).toLowerCase() === "failed")) return "rollback_suggested";
  if (verificationRecords.length && missionPhase === "completed") return "verifying";
  if (missionPhase === "waiting_approval") {
    return currentMode === "analysis" ? "waiting_plan_approval" : "waiting_action_approval";
  }
  return normalizeProtocolIncidentStage(missionPhase);
}

function inferProtocolTurnLane({
  snapshot = {},
  missionPhase = "idle",
  currentStage = "understanding",
  currentMode = "analysis",
  turnPolicy = null,
  promptEnvelope = null,
} = {}) {
  const explicitLane = normalizeTurnLane(snapshot.currentLane);
  if (explicitLane) return explicitLane;

  const policyLane = normalizeTurnLane(turnPolicy?.lane);
  if (policyLane) return policyLane;

  const promptLane = normalizeTurnLane(promptEnvelope?.currentLane);
  if (promptLane) return promptLane;

  const stage = normalizeProtocolIncidentStage(currentStage);
  const phase = normalizeProtocolMissionPhase(missionPhase);
  if (stage === "waiting_plan_approval" || stage === "planning") return "plan";
  if (stage === "verifying" || stage === "rollback_suggested") return "verify";
  if (currentMode === "execute" || ["executing", "waiting_approval", "finalizing", "completed", "failed"].includes(phase)) {
    return "execute";
  }
  if (stage === "collecting_evidence" || stage === "analyzing") return "readonly";
  return "answer";
}

function buildProtocolIncidentSummary({
  currentMode = "analysis",
  currentStage = "understanding",
  missionPhase = "idle",
  approvalItems = [],
  verificationRecords = [],
  evidenceSummaries = [],
  incidentEvents = [],
  currentFailureCard = null,
  cards = {},
  turnActive = false,
  nextSendStartsNewMission = false,
} = {}) {
  const stage = normalizeProtocolIncidentStage(currentStage);
  const phase = normalizeProtocolMissionPhase(missionPhase);
  const pendingApprovals = asArray(approvalItems).filter((item) => compactText(item?.raw?.status || "pending").toLowerCase() === "pending");
  const leadApproval = pendingApprovals[0] || approvalItems[0] || null;
  let blockingState = "idle";
  let blockingLabel = "待命";
  let tone = "info";
  let detail = "工作台已就绪，可继续发起分析或执行请求。";

  if (currentFailureCard || stage === "failed") {
    blockingState = "failed";
    blockingLabel = "已阻塞";
    tone = "danger";
    detail = `当前 mission 已失败：${compactText(currentFailureCard?.title || currentFailureCard?.summary || "请查看最近失败卡和时间线。")}`;
  } else if (cards.stopNoticeCard || stage === "canceled" || phase === "aborted") {
    blockingState = "stopped";
    blockingLabel = "已停止";
    tone = "warning";
    detail = nextSendStartsNewMission
      ? "当前 mission 已停止，再次发送会启动一轮新的 mission。"
      : "当前 mission 已停止，请确认是否重新发起。";
  } else if (stage === "waiting_plan_approval") {
    blockingState = "waiting_approval";
    blockingLabel = "等待审批";
    tone = "warning";
    detail = "计划已生成，审批通过后才会进入执行模式。";
  } else if (stage === "waiting_action_approval" || phase === "waiting_approval") {
    blockingState = "waiting_approval";
    blockingLabel = "等待审批";
    tone = "warning";
    detail = leadApproval
      ? `${leadApproval.hostName || leadApproval.hostId || "目标"} 的 ${leadApproval.command || leadApproval.summary || "变更动作"} 正等待审批，批准前不会落地执行。`
      : "高风险动作正在等待审批，批准前不会落地执行。";
  } else if (phase === "waiting_input") {
    blockingState = "waiting_input";
    blockingLabel = "等待补充输入";
    tone = "warning";
    detail = "主 Agent 需要你补充输入或确认后继续推进。";
  } else if (stage === "verifying") {
    blockingState = "verifying";
    blockingLabel = "自动验证中";
    tone = "info";
    detail = "执行动作已完成，系统正在自动验证结果并准备回滚提示。";
  } else if (stage === "rollback_suggested") {
    blockingState = "rollback_suggested";
    blockingLabel = "需要处理";
    tone = "warning";
    detail = "自动验证未通过，建议先评估回滚建议和下一步操作。";
  } else if (stage === "completed" || phase === "completed") {
    blockingState = "completed";
    blockingLabel = "可继续追问";
    tone = "success";
    detail = "本轮 mission 已完成，可以继续补充下一轮需求。";
  } else if (
    turnActive ||
    ["understanding", "planning", "collecting_evidence", "analyzing", "executing", "finalizing"].includes(stage)
  ) {
    blockingState = "running";
    blockingLabel = "推进中";
    tone = "info";
    detail = currentMode === "execute"
      ? "主 Agent 与后台 Agent 正在推进执行与验证。"
      : "主 Agent 正在收集证据、分析问题并生成计划。";
  }

  const metaItems = [
    pendingApprovals.length ? { key: "approvals", label: "待审批", value: pendingApprovals.length } : null,
    verificationRecords.length ? { key: "verifications", label: "验证", value: verificationRecords.length } : null,
    incidentEvents.length ? { key: "events", label: "事件", value: incidentEvents.length } : null,
    evidenceSummaries.length ? { key: "evidence", label: "证据", value: evidenceSummaries.length } : null,
  ].filter(Boolean);

  return {
    mode: currentMode,
    modeLabel: incidentModeLabel(currentMode),
    stage,
    stageLabel: incidentStageLabel(stage),
    blockingState,
    blockingLabel,
    tone,
    detail,
    metaItems,
  };
}

function buildProtocolIncidentInsights({
  eventItems = [],
  verificationRecords = [],
  cards = {},
  planCardModel = null,
} = {}) {
  const timelineItems = asArray(eventItems).slice(0, 3).map((item) => ({
    id: compactText(item.id),
    title: compactText(item.title || "事件"),
    text: compactText(item.text || item.detail || "点击查看上下文"),
    meta: [compactText(item.time), compactText(item.hostId)].filter(Boolean).join(" · "),
    tone: compactText(item.tone || item.projection?.tone || "neutral"),
    action: {
      kind: "event",
      eventId: compactText(item.id),
    },
  }));

  const verificationItems = asArray(verificationRecords).slice(0, 2).map((record) => ({
    id: compactText(record.id),
    title: compactText(record.statusLabel || "验证结果"),
    text: compactText(record.targetSummary || record.findings?.[0] || record.strategy || "自动验证结果"),
    meta: [compactText(record.strategy), compactText(record.timeLabel)].filter(Boolean).join(" · "),
    tone: compactText(record.tone || "neutral"),
    action: {
      kind: "verification",
      verificationId: compactText(record.id),
      hostId: compactText(record.hostId),
    },
  }));

  const rollbackItems = [];
  for (const record of asArray(verificationRecords)) {
    const rollbackHint = compactText(record.rollbackHint);
    const nextStepSuggestion = compactText(record.nextStepSuggestion);
    if (!rollbackHint && !nextStepSuggestion) continue;
    rollbackItems.push({
      id: compactText(record.id || `rollback-${rollbackItems.length + 1}`),
      title: rollbackHint ? "回滚建议" : "下一步建议",
      text: rollbackHint || nextStepSuggestion,
      meta: [nextStepSuggestion && nextStepSuggestion !== rollbackHint ? `下一步：${nextStepSuggestion}` : "", compactText(record.timeLabel)]
        .filter(Boolean)
        .join(" · "),
      tone: compactText(record.tone || "warning"),
      action: {
        kind: "verification",
        verificationId: compactText(record.id),
        hostId: compactText(record.hostId),
      },
    });
  }

  if (!rollbackItems.length && compactText(planCardModel?.rollback)) {
    rollbackItems.push({
      id: "plan-rollback-preview",
      title: "计划回滚预案",
      text: compactText(planCardModel.rollback),
      meta: compactText(planCardModel.version || planCardModel.generatedAt),
      tone: "neutral",
      action: {
        kind: "plan_rollback",
      },
    });
  }

  const verificationCardCount = asArray(cards?.currentMissionCards).filter((card) => isVerificationCard(card)).length;
  const rollbackCardCount = asArray(cards?.currentMissionCards).filter((card) => isRollbackCard(card)).length;

  return [
    {
      key: "timeline",
      title: "最新事件",
      subtitle: "轻量时间线摘要",
      count: asArray(eventItems).length,
      emptyLabel: "当前还没有关键事件。",
      items: timelineItems,
    },
    {
      key: "verification",
      title: "自动验证",
      subtitle: "执行后的最近验证结论",
      count: Math.max(asArray(verificationRecords).length, verificationCardCount),
      emptyLabel: "当前还没有自动验证结果。",
      items: verificationItems,
    },
    {
      key: "rollback",
      title: "回滚建议",
      subtitle: "失败后的回退提示与下一步建议",
      count: Math.max(rollbackItems.length, rollbackCardCount),
      emptyLabel: "当前没有额外回滚建议。",
      items: rollbackItems.slice(0, 2),
    },
  ];
}

export function resolveProtocolWorkspaceCards(cards = []) {
  const workspaceCards = asArray(cards);
  const latestUserIndex = findLastIndex(workspaceCards, (card) => isUserMessageCard(card));
  const currentMissionCards = latestUserIndex >= 0 ? workspaceCards.slice(latestUserIndex) : workspaceCards;
  const missionScopeCards = currentMissionCards.length ? currentMissionCards : workspaceCards;
  const missionCard = findLast(missionScopeCards, (card) => isMissionNoticeCard(card));
  const planCard = findLast(missionScopeCards, (card) => isPlanCard(card));
  const dispatchSummaryCards = missionScopeCards.filter((card) => isDispatchSummaryCard(card));
  const workspaceResultCard = findLast(missionScopeCards, (card) => isWorkspaceResultCard(card));
  const currentErrorCard = findLast(missionScopeCards, (card) => card?.type === "ErrorCard" || (isVerificationCard(card) && compactText(card?.status).toLowerCase() === "failed"));
  const currentFailureSummaryCard = findLast(missionScopeCards, (card) => isFailedResultSummaryCard(card));
  const stopNoticeCard = findLast(missionScopeCards, (card) => isStoppedNoticeCard(card));
  const conversationCards = workspaceCards.filter(
    (card) =>
      isUserMessageCard(card) ||
      isAssistantMessageCard(card) ||
      isSystemNoticeCard(card) ||
      card?.type === "ErrorCard" ||
      isFailedResultSummaryCard(card) ||
      isVerificationCard(card) ||
      isRollbackCard(card),
  );
  const approvalCards = missionScopeCards.filter((card) => isApprovalCard(card) && card.status === "pending");
  const choiceCards = missionScopeCards.filter((card) => isChoiceCard(card) && card.status === "pending");
  const processCards = missionScopeCards.filter((card) => isProcessCard(card));
  const commandCards = missionScopeCards.filter((card) => card?.type === "CommandCard");
  const eventCommandCards = workspaceCards.filter((card) => card?.type === "CommandCard");
  return {
    workspaceCards,
    currentMissionCards,
    missionCard,
    planCard,
    dispatchSummaryCards,
    workspaceResultCard,
    currentErrorCard,
    currentFailureSummaryCard,
    stopNoticeCard,
    conversationCards,
    approvalCards,
    choiceCards,
    processCards,
    commandCards,
    eventCommandCards,
  };
}

export function buildProtocolConversationItems(cards = []) {
  return asArray(cards)
    .map((card) => {
      const role = isUserMessageCard(card) ? "user" : "assistant";
      const title = normalizeWorkspaceCopy(card?.title);
      const hasMcpApp = !!card?.detail?.mcpApp?.html;
      // Preserve original text with newlines intact — don't use compactText/normalizeWorkspaceCopy
      // which would collapse \n into spaces and destroy Markdown formatting
      const rawText = String(card?.text || card?.summary || card?.message || card?.title || "").trim();
      if (!rawText && !hasMcpApp) return null;

      // Filter out system routing / dispatch messages that are not meant for the user
      if (role === "assistant" && isInternalRoutingMessageText(rawText)) return null;

      const shouldPrefixTitle = (card?.type === "ErrorCard" || isFailedResultSummaryCard(card) || isVerificationCard(card) || isRollbackCard(card)) && title && !rawText.startsWith(title);
      const cleanedText = cleanAssistantDisplayText(shouldPrefixTitle ? `${title}\n${rawText}` : rawText, role);
      if (!cleanedText && !hasMcpApp) return null;

      let displayTitle = role === "user" ? "用户" : "主 Agent";
      if (card?.type === "ErrorCard") {
        displayTitle = "系统错误";
      } else if (isVerificationCard(card)) {
        displayTitle = "验证";
      } else if (isRollbackCard(card)) {
        displayTitle = "回滚建议";
      }

      return {
        id: card.id || `${role}-${cleanedText.slice(0, 40)}`,
        role,
        time: formatShortTime(card.updatedAt || card.createdAt),
        title: displayTitle,
        text: cleanedText,
      };
    })
    .filter(Boolean);
}

export function buildProtocolBackgroundAgents(hostRows = []) {
  const seen = new Set();
  return sortBackgroundAgentItems(
    asArray(hostRows)
    .filter((row) => {
      if (!row.hostId || row.hostId === "server-local") return false;
      if (seen.has(row.hostId)) return false;
      seen.add(row.hostId);
      return ["running", "waiting_approval", "queued", "idle", "pending", "dispatched"].includes(row.statusKey) || row.worker;
    })
    .map((row) => ({
      id: row.hostId,
      hostId: row.hostId,
      name: row.displayName || row.address || row.hostId || "agent",
      subtitle: compactText(row.taskTitle || row.summary || row.statusLabel || "等待执行"),
      status: row.statusKey,
      statusLabel: row.statusLabel,
      tone: statusTone(row.statusKey),
      sortTimestamp: row.updatedAt,
    })),
  );
}

function buildProtocolConversationStatusCard({
  missionPhase = "idle",
  turnActive = false,
  approvalItems = [],
  backgroundAgents = [],
  planCardModel = null,
  cards = {},
} = {}) {
  const phase = normalizeProtocolMissionPhase(missionPhase);

  if (turnActive && ["completed", "failed", "aborted", "idle"].includes(phase)) {
    return {
      id: "__workspace_status__",
      type: "ThinkingCard",
      phase: "executing",
      hint: "",
    };
  }

  if (!turnActive && !["planning", "thinking", "executing", "waiting_approval", "waiting_input", "finalizing"].includes(phase)) {
    return null;
  }

  const hasPlanProjection = Boolean(
    cards?.planCard ||
      compactText(planCardModel?.summary) ||
      compactText(planCardModel?.generatedAt),
  );

  let hint = "";
  if (phase === "planning") {
    hint =
      compactText(planCardModel?.summary) ||
      (hasPlanProjection ? "已收到计划投影，正在同步 step 和 host-agent 映射。" : "主 Agent 正在理解你的问题并生成 plan。");
  } else if (phase === "waiting_approval") {
    const approval = approvalItems[0] || null;
    hint = approval
      ? `${approval.hostName || approval.hostId || "server-local"} 正在等待审批：${approval.command || approval.summary || "待确认操作"}`
      : "主 Agent 已生成计划，当前正在等待审批继续推进。";
  } else if (phase === "waiting_input") {
    hint = "主 Agent 正在等待补充输入或确认后继续推进。";
  } else if (phase === "executing") {
    const agents = asArray(backgroundAgents)
      .slice(0, 2)
      .map((agent) => compactText(agent.name || agent.hostId || agent.id))
      .filter(Boolean);
    hint = agents.length ? `${agents.join("、")} 正在执行` : "";
  }

  return {
    id: "__workspace_status__",
    type: "ThinkingCard",
    phase,
    hint,
  };
}

export function buildProtocolApprovalItems(approvalCards = [], hostRows = []) {
  const hostById = new Map(asArray(hostRows).map((row) => [row.hostId, row]));
  return sortApprovalDisplayItems(asArray(approvalCards).map((card) => {
    const host = hostById.get(card.hostId) || null;
    const decisions = asArray(card?.approval?.decisions);
    const dispatchRequest = asObject(host?.dispatch?.request);
    const approvalAnchor = asObject(host?.worker?.approvalAnchor || host?.worker?.approval_anchor);
    const isPlanApproval = card?.type === "PlanApprovalCard" || compactText(card?.approval?.type) === "plan_exit";
    const detailRows = [
      ...buildProtocolApprovalDetailRows(card.detail),
      ...buildTaskBindingRows(host?.dispatch?.taskBinding || host?.dispatch?.task_binding || null),
      ...buildDispatchTimelineRows(host?.dispatch || {}),
      ...buildApprovalAnchorRows(approvalAnchor),
    ];
    const approvalId = compactText(card?.approval?.requestId || card?.approvalId || card?.requestId);
    const itemId = compactText(card.id);
    const hostId = compactText(card.hostId);
    const hostName = isPlanApproval ? "计划审批" : host?.displayName || hostId || "unknown-host";
    const title = normalizeWorkspaceCopy(card.title || (isPlanApproval ? "计划审批" : ""));
    const command = normalizeWorkspaceCopy(card.command || dispatchRequest.summary || dispatchRequest.title || card.text || card.summary);
    const summary = normalizeWorkspaceCopy(card.text || card.summary || host?.taskTitle || host?.summary);
    const timeLabel = formatTime(card.updatedAt || card.createdAt);
    const timestamp = parseTimestamp(card.updatedAt || card.createdAt);
    return {
      id: itemId,
      approvalId,
      kind: isPlanApproval ? "plan" : "operation",
      title,
      hostId,
      hostName,
      commandLabel: isPlanApproval ? "计划:" : "执行命令:",
      command,
      summary,
      timeLabel,
      supportsAuthorize: decisions.includes("accept_session"),
      detailRows: detailRows.length
        ? detailRows
        : normalizeEvidenceRows(
            card.detail ||
              card.details ||
              host?.dispatch?.request ||
              host?.dispatch?.taskBinding ||
              host?.dispatch?.task_binding ||
              host?.worker?.approval ||
              host?.worker?.approvalAnchor ||
              host?.worker?.approval_anchor,
          ),
      dispatchRequest,
      approvalAnchor,
      taskBinding: host?.dispatch?.taskBinding || host?.dispatch?.task_binding || null,
      sortTimestamp: card.updatedAt || card.createdAt,
      projection: buildProtocolProjection({
        kind: "approval",
        id: approvalId || itemId,
        aliases: [itemId, approvalId],
        title,
        summary: summary || command,
        hostId,
        hostName,
        status: compactText(card.status),
        tone: "warning",
        timeLabel,
        timestamp,
        links: [
          hostId ? { kind: "host", id: hostId, relation: "host", label: hostName || hostId, hostId } : null,
        ],
      }),
      raw: card,
    };
  }));
}

export function buildProtocolEventItems({
  planCard = null,
  dispatchSummaryCards = [],
  approvalCards = [],
  choiceCards = [],
  workspaceResultCard = null,
  hostRows = [],
  systemNoticeCards = [],
  dispatchEvents = [],
  commandCards = [],
  toolInvocations = [],
  verificationRecords = [],
  incidentEvents = [],
} = {}) {
  const incidentEventItems = buildProtocolIncidentEventItems(incidentEvents, commandCards, approvalCards, hostRows);
  const invocationEvents = buildProtocolToolInvocationEventItems(toolInvocations, hostRows);
  const verificationEvents = buildProtocolVerificationEventItems(verificationRecords);
  const hasProjectedCommandEvents = invocationEvents.some((item) => item.toolName === "command");
  const projectedChoiceCardIds = new Set(invocationEvents
    .filter((item) => item.toolName === "ask_user_question")
    .map((item) => compactText(item.evidence?.metadata?.cardId || item.targetId?.replace(/^tool-/, "")))
    .filter(Boolean));
  const commandEvents = hasProjectedCommandEvents ? [] : buildProtocolCommandEventItems(commandCards, hostRows);
  const timelineEvents = buildWorkspaceLiveTimeline({
    planCard,
    dispatchEvents,
    dispatchCards: dispatchSummaryCards,
    approvalCards,
    choiceCards,
    resultCard: workspaceResultCard,
    hostRows,
    noticeCards: systemNoticeCards,
  }).map((item) => ({
    id: item.id,
    time: item.time || "",
    timestamp: item.timestamp || 0,
    title: normalizeWorkspaceCopy(item.title || item.source || "事件"),
    text: normalizeWorkspaceCopy(item.text || ""),
    detail: normalizeWorkspaceCopy(item.text || ""),
    tone: item.tone || "neutral",
    targetType: item.targetType || "",
    targetId: item.targetId || "",
    hostId: item.hostId || "",
    projection: buildProtocolProjection({
      kind: "event",
      id: item.id,
      title: normalizeWorkspaceCopy(item.title || item.source || "事件"),
      summary: normalizeWorkspaceCopy(item.text || ""),
      hostId: item.hostId || "",
      hostName: item.hostId || "",
      status: "",
      tone: item.tone || "neutral",
      timeLabel: item.time || "",
      timestamp: item.timestamp || 0,
      links: [
        item.targetType && item.targetId
          ? { kind: eventTargetProjectionKind(item.targetType), id: item.targetId, relation: "event_target", label: item.targetType, hostId: item.hostId || "" }
          : null,
        item.hostId ? { kind: "host", id: item.hostId, relation: "host", label: item.hostId, hostId: item.hostId } : null,
      ],
    }),
  }));
  const hasProjectedChoiceEvents = projectedChoiceCardIds.size > 0;
  const filteredTimelineEvents = commandEvents.length || hasProjectedCommandEvents || hasProjectedChoiceEvents
    ? timelineEvents.filter((item) => {
        if (item.targetType === "host" && /^已处理\s*\d+\s*个命令/.test(compactText(item.detail || item.text))) {
          return false;
        }
        if (item.targetType === "choice" && projectedChoiceCardIds.has(compactText(item.targetId))) {
          return false;
        }
        return true;
      })
    : timelineEvents;
  return [...incidentEventItems, ...verificationEvents, ...invocationEvents, ...commandEvents, ...filteredTimelineEvents]
    .sort((left, right) => (right.timestamp || 0) - (left.timestamp || 0))
    .slice(0, 14);
}

export function buildProtocolPlanCardModel({
  planCard = null,
  workspaceResultCard = null,
  hostRows = [],
} = {}) {
  const planDetailState = getWorkspacePlanDetail(planCard, workspaceResultCard);
  const planDetail = planDetailState.detail;
  const structuredProcess = planDetailState.structuredProcess;
  const taskHostBindings = asArray(pickField(planDetail, "taskHostBindings", "task_host_bindings"));
  const fallbackStructuredProcess = asArray(planCard?.items)
    .map((item, index) => {
      const stepText = compactText(item?.step || item?.label || item?.title || item?.text);
      const status = compactText(item?.status).toLowerCase() || "pending";
      if (!stepText) return "";
      const match = stepText.match(/^([A-Za-z0-9._:-]+)\s+\[([^\]]+)\]\s+(.+)$/);
      if (match) {
        const [, hostId, taskId, title] = match;
        return `${taskId} [${status}] @${hostId} ${title}`.trim();
      }
      return `step-${index + 1} [${status}] ${stepText}`.trim();
    })
    .filter(Boolean);
  const stepItems = buildWorkspaceStepItems({
    structuredProcess: structuredProcess.length ? structuredProcess : fallbackStructuredProcess,
    taskHostBindings,
    hostRows,
  });

  const completed =
    stepItems.filter((item) => compactText(item.status).includes("complete")).length ||
    asArray(planCard?.items).filter((item) => compactText(item?.status).includes("complete")).length;
  return {
    title: normalizeWorkspaceCopy(planCard?.title || planDetail?.goal || "主 Agent 计划摘要"),
    summary: normalizeWorkspaceCopy(planCard?.text || planDetail?.goal || ""),
    risk: normalizePlanDetailValue(pickField(planDetail, "risk", "risks")),
    validation: normalizePlanDetailValue(pickField(planDetail, "validation", "verification", "verify")),
    rollback: normalizePlanDetailValue(pickField(planDetail, "rollback", "rollbackPlan", "rollback_plan")),
    scope: normalizePlanDetailValue(pickField(planDetail, "scope", "range")),
    assumptions: normalizePlanDetailValue(pickField(planDetail, "assumptions")),
    version: compactText(planDetailState.version || "plan-v1"),
    generatedAt: compactText(planDetailState.generatedAt || planCard?.updatedAt || planCard?.createdAt),
    totalSteps: stepItems.length || asArray(planCard?.items).length,
    completedSteps: completed,
    stepItems,
    dispatchEvents: asArray(pickField(planDetail, "dispatchEvents", "dispatch_events")),
  };
}

export function buildProtocolEvidenceTabs({
  planCardModel = null,
  hostRow = null,
  approvalItem = null,
  verificationItem = null,
  verificationRecords = [],
  eventItems = [],
} = {}) {
  const dispatch = asObject(hostRow?.dispatch);
  const worker = asObject(hostRow?.worker);
  const taskBinding = asObject(pickField(dispatch, "task_binding", "taskBinding")) || null;
  const approvalAnchor = asObject(pickField(worker, "approval_anchor", "approvalAnchor"));
  const dispatchRequest = asObject(pickField(dispatch, "request"));

  const mainAgentPlan = [];
  if (compactText(planCardModel?.summary) || compactText(planCardModel?.title)) {
    mainAgentPlan.push({
      id: "plan-summary",
      title: "计划摘要",
      text: normalizeWorkspaceCopy(planCardModel?.summary || planCardModel?.title || "当前还没有可用的计划摘要。"),
      time: formatShortTime(planCardModel?.generatedAt),
    });
  }
  for (const [index, step] of asArray(planCardModel?.stepItems).entries()) {
    const hostLabels = asArray(step.hosts)
      .map((host) => compactText(host.label || host.id))
      .filter(Boolean)
      .join("、");
    const pieces = [step.statusLabel || step.status, hostLabels ? `Host: ${hostLabels}` : "", step.detail || step.note || ""]
      .map((item) => compactText(item))
      .filter(Boolean);
    mainAgentPlan.push({
      id: `plan-step-${step.id || index}`,
      title: `Step ${step.index || index + 1} · ${normalizeWorkspaceCopy(step.title)}`,
      text: normalizeWorkspaceCopy(pieces.join(" · ")),
      time: "",
    });
  }

  const hostConversation = [];
  for (const item of asArray(worker.conversation || worker.conversation_excerpts)) {
    const text = compactText(item.text || item.summary);
    if (!text) continue;
    hostConversation.push({
      id: item.id || `${item.createdAt}-${text}`,
      title: normalizeWorkspaceCopy(item.summary || item.type || item.role || "Host -> AI"),
      text: normalizeWorkspaceCopy(text),
      time: formatShortTime(item.createdAt || hostRow?.updatedAt),
    });
  }

  if (!hostConversation.length) {
    const transcript = asArray(worker.transcript);
    for (let index = 0; index < transcript.length; index += 1) {
      const text = compactText(transcript[index]);
      if (!text) continue;
      hostConversation.push({
        id: `${hostRow?.hostId || "host"}-transcript-${index}`,
        title: `Transcript ${index + 1}`,
        text: normalizeWorkspaceCopy(text),
        time: formatShortTime(worker.updatedAt || hostRow?.updatedAt),
      });
    }
  }

  if (!hostConversation.length) {
    if (compactText(worker.lastReply)) {
      hostConversation.push({
        id: `${hostRow?.hostId || "host"}-last-reply`,
        title: "Last Reply",
        text: normalizeWorkspaceCopy(worker.lastReply),
        time: formatShortTime(worker.updatedAt || hostRow?.updatedAt),
      });
    }
    if (compactText(worker.lastError)) {
      hostConversation.push({
        id: `${hostRow?.hostId || "host"}-last-error`,
        title: "Last Error",
        text: normalizeWorkspaceCopy(worker.lastError),
        time: formatShortTime(worker.updatedAt || hostRow?.updatedAt),
      });
    }
  }

  const approvalContext = [];
  const approvalSource = approvalItem || {
    hostName: hostRow?.displayName,
    hostId: hostRow?.hostId,
    approvalId: compactText(pickField(worker, "approval", "approvalAnchor", "approval_anchor")?.requestId || ""),
    command: compactText(dispatchRequest.title || dispatchRequest.summary),
    summary: compactText(dispatchRequest.summary || dispatchRequest.title || hostRow?.summary || hostRow?.taskTitle),
    detailRows: [...buildTaskBindingRows(taskBinding || dispatch), ...buildDispatchTimelineRows(dispatch), ...buildApprovalAnchorRows(approvalAnchor)],
    approvalAnchor,
    raw: { dispatch, worker },
  };

  if (approvalSource) {
    if (approvalSource.hostName || approvalSource.hostId) {
      approvalContext.push({
        id: `approval-host-${approvalSource.hostId || "host"}`,
        title: "主机",
        text: compactText(approvalSource.hostName || approvalSource.hostId || "unknown-host"),
        time: "",
      });
    }
    if (approvalSource.approvalId) {
      approvalContext.push({
        id: `approval-id-${approvalSource.approvalId}`,
        title: "审批ID",
        text: compactText(approvalSource.approvalId),
        time: "",
      });
    }
    if (approvalSource.command || approvalSource.summary) {
      approvalContext.push({
        id: `approval-command-${approvalSource.hostId || "host"}`,
        title: "命令",
        text: normalizeWorkspaceCopy(approvalSource.command || approvalSource.summary),
        time: "",
      });
    }
    for (const row of asArray(approvalSource.detailRows)) {
      approvalContext.push({
        id: `approval-detail-${approvalSource.hostId || "host"}-${row.label}`,
        title: compactText(row.label || "详情"),
        text: normalizeWorkspaceCopy(row.value || row.text),
        time: "",
      });
    }
    if (approvalSource.approvalAnchor) {
      for (const row of buildApprovalAnchorRows(approvalSource.approvalAnchor)) {
        approvalContext.push({
          id: `approval-anchor-${approvalSource.hostId || "host"}-${row.label}`,
          title: `审批锚点 · ${row.label}`,
          text: normalizeWorkspaceCopy(row.value),
          time: "",
        });
      }
    }
  }

  const normalizedVerifications = buildProtocolVerificationRecords(verificationRecords);
  const selectedVerification = verificationItem
    ? normalizedVerifications.find((record) => record.id === compactText(verificationItem.id || verificationItem.raw?.id)) || normalizeProtocolVerificationRecord(verificationItem.raw || verificationItem)
    : null;
  const approvalVerificationRecords = selectedVerification
    ? [selectedVerification]
    : normalizedVerifications.filter((record) => {
        if (approvalSource?.approvalId && record.approvalId === compactText(approvalSource.approvalId)) return true;
        if (approvalSource?.id && record.commandCardId === compactText(approvalSource.id)) return true;
        if (approvalSource?.raw?.id && record.commandCardId === compactText(approvalSource.raw.id)) return true;
        if (hostRow?.hostId && record.hostId === compactText(hostRow.hostId) && !approvalSource?.approvalId) return true;
        return false;
      });
  if (approvalVerificationRecords.length) {
    approvalContext.push({
      id: `approval-verification-${approvalVerificationRecords[0].id}`,
      title: "验证结果",
      text: normalizeWorkspaceCopy(`${approvalVerificationRecords.length} 条，可切换到“验证结果”标签查看。`),
      time: "",
    });
  }

  const verificationResults = approvalVerificationRecords.flatMap((record, index) => {
    const rows = [
      { id: `${record.id}-status`, title: index === 0 ? "验证状态" : `验证状态 ${index + 1}`, text: record.statusLabel, time: record.timeLabel || "" },
      record.targetSummary ? { id: `${record.id}-target`, title: index === 0 ? "目标" : `目标 ${index + 1}`, text: normalizeWorkspaceCopy(record.targetSummary), time: "" } : null,
      record.strategy ? { id: `${record.id}-strategy`, title: index === 0 ? "校验策略" : `校验策略 ${index + 1}`, text: normalizeWorkspaceCopy(record.strategy), time: "" } : null,
      record.verificationSources.length ? { id: `${record.id}-sources`, title: index === 0 ? "验证来源" : `验证来源 ${index + 1}`, text: normalizeWorkspaceCopy(record.verificationSources.join(" / ")), time: "" } : null,
      record.successCriteria.length ? { id: `${record.id}-criteria`, title: index === 0 ? "成功条件" : `成功条件 ${index + 1}`, text: normalizeWorkspaceCopy(record.successCriteria.join(" / ")), time: "" } : null,
      record.findings.length ? { id: `${record.id}-findings`, title: index === 0 ? "结论" : `结论 ${index + 1}`, text: normalizeWorkspaceCopy(record.findings.join(" / ")), time: "" } : null,
      record.rollbackHint ? { id: `${record.id}-rollback`, title: index === 0 ? "回滚建议" : `回滚建议 ${index + 1}`, text: normalizeWorkspaceCopy(record.rollbackHint), time: "" } : null,
      record.nextStepSuggestion ? { id: `${record.id}-next-step`, title: index === 0 ? "下一步建议" : `下一步建议 ${index + 1}`, text: normalizeWorkspaceCopy(record.nextStepSuggestion), time: "" } : null,
    ].filter(Boolean);
    return rows;
  });

  const hostTerminal = asObject(worker.terminal);
  const hostTerminalRows = [];
  const terminalRows = orderedObjectRows(hostTerminal);
  const terminalOutput = pickField(hostTerminal, "output", "stdout", "text", "summary");
  const terminalSummary = [
    hostRow?.displayName ? { label: "Host", value: compactText(hostRow.displayName) } : null,
    hostRow?.statusLabel ? { label: "Status", value: compactText(hostRow.statusLabel) } : null,
    hostRow?.taskTitle ? { label: "Task", value: compactText(hostRow.taskTitle) } : null,
    compactText(worker.mode) ? { label: "Mode", value: compactText(worker.mode) } : null,
    compactText(worker.activeTaskId || worker.active_task_id) ? { label: "Active Task", value: compactText(worker.activeTaskId || worker.active_task_id) } : null,
    compactText(worker.sessionId || worker.session_id) ? { label: "Worker Session", value: compactText(worker.sessionId || worker.session_id) } : null,
    compactText(worker.threadId || worker.thread_id) ? { label: "Worker Thread", value: compactText(worker.threadId || worker.thread_id) } : null,
  ].filter(Boolean);
  hostTerminalRows.push(...terminalSummary, ...terminalRows);
  for (const row of buildApprovalAnchorRows(approvalAnchor)) {
    hostTerminalRows.push({
      label: `Approval ${row.label}`,
      value: row.value,
      text: row.text,
    });
  }
  const hostTerminalOutput = (() => {
    if (Array.isArray(terminalOutput)) return terminalOutput.map((item) => String(item ?? ""));
    if (terminalOutput && typeof terminalOutput === "object") {
      const orderedRows = orderedObjectRows(terminalOutput);
      if (orderedRows.length) return orderedRows.map((row) => `${row.key}: ${row.value}`);
      try {
        return JSON.stringify(terminalOutput, null, 2);
      } catch {
        return String(terminalOutput);
      }
    }
    return compactText(terminalOutput || hostTerminal.summary || hostTerminal.status || "");
  })();

  return {
    mainAgentPlan,
    workerConversation: hostConversation,
    hostConversation,
    hostTerminalRows,
    hostTerminalOutput,
    approvalContext,
    verificationResults,
  };
}

function projectionCandidateIds(item = {}) {
  const projection = asObject(item?.projection);
  const ids = [
    compactText(projection.id),
    ...asArray(projection.aliases).map((entry) => compactText(entry)),
    compactText(item?.id),
    compactText(item?.approvalId),
    compactText(item?.commandCardId),
    compactText(item?.actionEventId),
    compactText(item?.targetId),
    compactText(item?.raw?.id),
  ].filter(Boolean);
  return [...new Set(ids)];
}

function resolveProjectionCollectionItem(collection = [], kind = "", id = "") {
  const targetKind = compactText(kind).toLowerCase();
  const targetId = compactText(id);
  if (!targetKind || !targetId) return null;
  return asArray(collection).find((item) => {
    const projection = asObject(item?.projection);
    if (compactText(projection.kind).toLowerCase() !== targetKind) return false;
    return projectionCandidateIds(item).includes(targetId);
  }) || null;
}

function resolveProtocolVerificationForApprovalItem(approvalItem = null, verificationRecords = []) {
  const approvalIds = projectionCandidateIds(approvalItem);
  if (!approvalIds.length) return null;
  return asArray(verificationRecords).find((record) => {
    const relationIds = [compactText(record.approvalId), compactText(record.commandCardId), compactText(record.actionEventId)].filter(Boolean);
    return relationIds.some((id) => approvalIds.includes(id));
  }) || null;
}

function resolveProtocolApprovalForVerificationRecord(record = null, approvalItems = []) {
  const verificationIds = projectionCandidateIds(record);
  if (!verificationIds.length) return null;
  return asArray(approvalItems).find((approval) => {
    const approvalIds = projectionCandidateIds(approval);
    return approvalIds.some((id) => verificationIds.includes(id));
  }) || null;
}

function resolveProtocolEventForTarget(eventItems = [], kind = "", ids = []) {
  const targetKind = compactText(kind).toLowerCase();
  const candidateIds = asArray(ids).map((item) => compactText(item)).filter(Boolean);
  if (!targetKind || !candidateIds.length) return null;
  return asArray(eventItems).find((eventItem) => {
    const links = asArray(eventItem?.projection?.links);
    return links.some((link) =>
      compactText(link.kind).toLowerCase() === targetKind &&
      candidateIds.includes(compactText(link.id)),
    );
  }) || null;
}

function linkProtocolProjectionCollections({
  approvalItems = [],
  eventItems = [],
  verificationRecords = [],
  evidenceSummaries = [],
  toolInvocations = [],
} = {}) {
  const linkedApprovals = asArray(approvalItems).map((approval) => {
    const verification = resolveProtocolVerificationForApprovalItem(approval, verificationRecords);
    const event = resolveProtocolEventForTarget(eventItems, "approval", projectionCandidateIds(approval));
    return appendProtocolProjectionLinks(approval, [
      verification ? { kind: "verification", id: compactText(verification?.projection?.id || verification?.id), relation: "verification_result", label: "验证结果", hostId: compactText(verification?.hostId) } : null,
      event ? { kind: "event", id: compactText(event?.projection?.id || event?.id), relation: "timeline_event", label: "时间线事件", hostId: compactText(event?.hostId) } : null,
    ]);
  });

  const linkedVerifications = asArray(verificationRecords).map((record) => {
    const approval = resolveProtocolApprovalForVerificationRecord(record, linkedApprovals);
    const event = resolveProtocolEventForTarget(eventItems, "verification", projectionCandidateIds(record));
    return appendProtocolProjectionLinks(record, [
      approval ? { kind: "approval", id: compactText(approval?.projection?.id || approval?.approvalId || approval?.id), relation: "approval", label: "审批上下文", hostId: compactText(approval?.hostId) } : null,
      event ? { kind: "event", id: compactText(event?.projection?.id || event?.id), relation: "timeline_event", label: "时间线事件", hostId: compactText(event?.hostId) } : null,
    ]);
  });

  const linkedEvidence = asArray(evidenceSummaries).map((record) => {
    const metadata = asObject(record?.metadata);
    const invocation = resolveProjectionCollectionItem(toolInvocations, "tool_invocation", compactText(record?.invocationId || metadata?.invocationId || record?.sourceRef));
    const approval = resolveProjectionCollectionItem(linkedApprovals, "approval", compactText(metadata?.approvalId || metadata?.cardId));
    return appendProtocolProjectionLinks(record, [
      invocation ? { kind: "tool_invocation", id: compactText(invocation?.projection?.id || invocation?.id), relation: "source_invocation", label: "来源工具调用", hostId: compactText(invocation?.hostId) } : null,
      approval ? { kind: "approval", id: compactText(approval?.projection?.id || approval?.approvalId || approval?.id), relation: "related_approval", label: "关联审批", hostId: compactText(approval?.hostId) } : null,
    ]);
  });

  const linkedToolInvocations = asArray(toolInvocations).map((invocation) => {
    const evidence = resolveProjectionCollectionItem(linkedEvidence, "evidence", compactText(invocation?.evidenceId));
    const approvalLink = asArray(invocation?.projection?.links).find((link) => compactText(link.kind) === "approval");
    const approval = approvalLink ? resolveProjectionCollectionItem(linkedApprovals, "approval", approvalLink.id) : null;
    return appendProtocolProjectionLinks(invocation, [
      evidence ? { kind: "evidence", id: compactText(evidence?.projection?.id || evidence?.id), relation: "evidence", label: "关联证据", hostId: compactText(invocation?.hostId) } : null,
      approval ? { kind: "approval", id: compactText(approval?.projection?.id || approval?.approvalId || approval?.id), relation: "approval", label: "关联审批", hostId: compactText(approval?.hostId) } : null,
    ]);
  });

  const linkedEvents = asArray(eventItems).map((eventItem) => {
    const links = asArray(eventItem?.projection?.links);
    const nextLinks = [];
    for (const link of links) {
      if (compactText(link.kind) === "approval") {
        const approval = resolveProjectionCollectionItem(linkedApprovals, "approval", link.id);
        if (approval) {
          nextLinks.push({ kind: "approval", id: compactText(approval?.projection?.id || approval?.approvalId || approval?.id), relation: link.relation || "event_target", label: link.label, hostId: compactText(approval?.hostId) });
        }
      }
      if (compactText(link.kind) === "verification") {
        const verification = resolveProjectionCollectionItem(linkedVerifications, "verification", link.id);
        if (verification) {
          nextLinks.push({ kind: "verification", id: compactText(verification?.projection?.id || verification?.id), relation: link.relation || "event_target", label: link.label, hostId: compactText(verification?.hostId) });
        }
      }
      if (compactText(link.kind) === "tool_invocation") {
        const invocation = resolveProjectionCollectionItem(linkedToolInvocations, "tool_invocation", link.id);
        if (invocation) {
          nextLinks.push({ kind: "tool_invocation", id: compactText(invocation?.projection?.id || invocation?.id), relation: link.relation || "event_target", label: link.label, hostId: compactText(invocation?.hostId) });
        }
      }
    }
    return appendProtocolProjectionLinks(eventItem, nextLinks);
  });

  return {
    approvalItems: linkedApprovals,
    verificationRecords: linkedVerifications,
    evidenceSummaries: linkedEvidence,
    toolInvocations: linkedToolInvocations,
    eventItems: linkedEvents,
  };
}

export function buildProtocolAgentDetailModel({
  backgroundAgent = null,
  hostRow = null,
  planCardModel = null,
  approvalItems = [],
  eventItems = [],
  formattedTurns = [],
} = {}) {
  const agent = asObject(backgroundAgent);
  const row = asObject(hostRow);
  const hostId = compactText(row.hostId || agent.hostId || agent.id);
  const displayName = compactText(row.displayName || agent.name || agent.title || hostId || "agent");
  const statusKey = compactText(row.statusKey || agent.status || "idle") || "idle";
  const statusLabel = compactText(row.statusLabel || agent.statusLabel || agent.subtitle || statusKey || "idle");
  const tone = statusTone(statusKey);
  const selectedApproval =
    asArray(approvalItems).find((item) => compactText(item?.hostId) === hostId) ||
    row.approvalCard ||
    null;
  const evidenceTabs = buildProtocolEvidenceTabs({
    planCardModel,
    hostRow: row,
    approvalItem: selectedApproval,
    eventItems,
  });
  const stepItems = summarizeHostStepItems(planCardModel, row);
  const matchingTurns = asArray(formattedTurns).filter((turn) => {
    const turnHostIds = [
      compactText(turn?.hostId),
      compactText(turn?.userMessage?.sourceCard?.hostId),
      compactText(turn?.finalMessage?.sourceCard?.hostId),
      ...asArray(turn?.processItems).map((item) => compactText(item?.hostId)),
    ].filter(Boolean);
    if (!turnHostIds.length) return false;
    return turnHostIds.some((value) => value === hostId);
  });

  const taskItems = [];
  if (compactText(row.taskTitle)) {
    taskItems.push({ label: "任务标题", value: compactText(row.taskTitle) });
  }
  if (compactText(row.summary)) {
    taskItems.push({ label: "任务摘要", value: compactText(row.summary) });
  }
  if (compactText(row.rawStatusLabel)) {
    taskItems.push({ label: "当前状态", value: compactText(row.rawStatusLabel) });
  }
  if (compactText(row.workerSession)) {
    taskItems.push({ label: "Worker Session", value: compactText(row.workerSession) });
  }
  if (compactText(row.worker?.threadId || row.worker?.thread_id)) {
    taskItems.push({ label: "Worker Thread", value: compactText(row.worker?.threadId || row.worker?.thread_id) });
  }
  if (compactText(row.queueCount)) {
    taskItems.push({ label: "排队任务", value: compactText(row.queueCount) });
  }
  if (row.dispatch?.request) {
    taskItems.push(...buildTaskBindingRows(asObject(row.dispatch.request)));
  }
  if (row.dispatch?.taskBinding || row.dispatch?.task_binding) {
    taskItems.push(...buildTaskBindingRows(asObject(row.dispatch?.taskBinding || row.dispatch?.task_binding)));
  }
  if (!row.dispatch?.request && !row.dispatch?.taskBinding && !row.dispatch?.task_binding && row.dispatch && typeof row.dispatch === "object") {
    taskItems.push(...buildTaskBindingRows(asObject(row.dispatch)));
  }
  if (stepItems.length) {
    taskItems.push({
      label: "匹配计划步骤",
      value: `${stepItems.length} 个步骤与当前 agent 关联`,
    });
    taskItems.push(
      ...stepItems.slice(0, 5).map((step, index) => ({
        label: `Step ${step.index || index + 1}`,
        value: [step.statusLabel || step.status, step.title, asArray(step.hosts).map((host) => compactText(host.label || host.id)).filter(Boolean).join("、")].filter(Boolean).join(" · "),
      })),
    );
  }

  const conversationItems = [];
  if (evidenceTabs.workerConversation.length) {
    conversationItems.push(
      ...evidenceTabs.workerConversation.slice(0, 8).map((item, index) => ({
        label: item.time || item.title || `消息 ${index + 1}`,
        value: compactText(item.text || item.value || item.summary),
      })),
    );
  }
  if (!conversationItems.length && asArray(row.worker?.transcript).length) {
    conversationItems.push(
      ...asArray(row.worker.transcript).slice(-8).map((item, index) => ({
        label: `Transcript ${index + 1}`,
        value: compactText(item),
      })),
    );
  }
  if (matchingTurns.length) {
    conversationItems.push({
      label: "相关 Turn",
      value: `${matchingTurns.length} 个 process turn 与当前 agent 相关`,
    });
  }

  const approvalItemsRows = [];
  if (selectedApproval) {
    approvalItemsRows.push(
      ...asArray(selectedApproval.detailRows).map((item) => ({
        label: compactText(item.label || "详情"),
        value: compactText(item.value || item.text),
      })),
    );
  } else if (evidenceTabs.approvalContext.length) {
    approvalItemsRows.push(
      ...evidenceTabs.approvalContext.map((item) => ({
        label: compactText(item.title || item.label || "审批信息"),
        value: compactText(item.text || item.value),
      })),
    );
  }
  if (row.worker?.approvalAnchor || row.worker?.approval_anchor) {
    approvalItemsRows.push(
      ...buildApprovalAnchorRows(asObject(row.worker?.approvalAnchor || row.worker?.approval_anchor)).map((item) => ({
        label: `锚点 · ${item.label}`,
        value: item.value,
      })),
    );
  }

  const activityItems = [];
  const filteredEvents = asArray(eventItems).filter((item) => compactText(item?.hostId) === hostId || compactText(item?.targetId) === hostId);
  if (filteredEvents.length) {
    activityItems.push(
      ...filteredEvents.slice(0, 6).map((item, index) => ({
        label: item.time || item.source || `事件 ${index + 1}`,
        value: compactText(item.text || item.title || item.detail),
      })),
    );
  }
  if (asArray(row.highlights).length) {
    activityItems.push(
      ...asArray(row.highlights)
        .slice(-5)
        .map((item, index) => ({
          label: `高亮 ${index + 1}`,
          value: compactText(item),
        })),
    );
  }
  if (!activityItems.length && evidenceTabs.hostTerminalRows.length) {
    activityItems.push(
      ...evidenceTabs.hostTerminalRows.slice(0, 6).map((item, index) => ({
        label: item.label || `状态 ${index + 1}`,
        value: compactText(item.value || item.text),
      })),
    );
  }

  const overviewItems = [
    { label: "主机", value: displayName },
    { label: "状态", value: statusLabel },
    { label: "队列", value: compactText(row.queueCount || 0) },
    { label: "任务", value: compactText(row.taskTitle || agent.subtitle || "等待执行") },
  ];

  return {
    id: hostId || compactText(agent.id) || displayName,
    hostId,
    title: displayName,
    subtitle: compactText(agent.subtitle || row.summary || row.taskTitle || "等待执行"),
    statusKey,
    statusLabel,
    tone,
    overviewItems,
    sections: [
      {
        key: "task",
        title: "分配任务信息",
        summary: compactText(row.taskTitle || row.summary || "当前 agent 的任务分配和计划绑定。"),
        items: taskItems,
        raw: {
          dispatch: row.dispatch || null,
          planSteps: stepItems,
        },
      },
      {
        key: "conversation",
        title: "与 AI 的对话信息",
        summary: compactText(evidenceTabs.workerConversation.summary || "当前 agent 和 AI 的交互记录。"),
        items: conversationItems,
        raw: evidenceTabs.workerConversation.raw || row.worker || null,
      },
      {
        key: "approval",
        title: "审核信息",
        summary: compactText(selectedApproval ? "当前 agent 已关联待审核任务。" : "当前 agent 的审核锚点和审批上下文。"),
        items: approvalItemsRows,
        raw: selectedApproval?.raw || row.worker?.approval || row.worker?.approvalAnchor || row.worker?.approval_anchor || null,
      },
      {
        key: "activity",
        title: "当前状态 / 最近活动",
        summary: compactText(row.statusLabel || row.summary || "当前 agent 的状态和最近变化。"),
        items: activityItems,
        raw: {
          highlights: row.highlights || [],
          events: filteredEvents,
          hostTerminalRows: evidenceTabs.hostTerminalRows,
        },
      },
    ],
    raw: {
      hostRow: row,
      backgroundAgent: agent,
      planCardModel,
      evidenceTabs,
      matchingTurns,
    },
  };
}

export function buildProtocolWorkspaceModel(snapshot = {}, runtime = {}) {
  const cards = resolveProtocolWorkspaceCards(snapshot.cards || []);
  const incidentEvents = asArray(snapshot.incidentEvents);
  let evidenceSummaries = normalizeProtocolEvidenceSummaries(snapshot.evidenceSummaries || []);
  const toolInvocations = normalizeProtocolToolInvocations(snapshot.toolInvocations, evidenceSummaries);
  const verificationRecords = buildProtocolVerificationRecords(snapshot.verificationRecords || []);
  const hostRows = buildWorkspaceHostRows({
    cards: cards.currentMissionCards,
    hosts: snapshot.hosts || [],
    approvalCards: cards.approvalCards,
  });
  const runtimePhase = normalizeProtocolMissionPhase(pickField(runtime?.turn || {}, "phase") || pickField(cards.missionCard?.detail || {}, "status"));
  const currentFailureCard =
    cards.currentErrorCard ||
    cards.currentFailureSummaryCard ||
    (compactText(cards.workspaceResultCard?.status).toLowerCase() === "failed" ? cards.workspaceResultCard : null);
  let missionPhase = runtimePhase;
  if (cards.stopNoticeCard) {
    missionPhase = "aborted";
  } else if (currentFailureCard) {
    missionPhase = "failed";
  } else if (compactText(cards.workspaceResultCard?.status).toLowerCase() === "completed" && runtime?.turn?.active !== true) {
    missionPhase = "completed";
  }
  const planCardModel = buildProtocolPlanCardModel({
    planCard: cards.planCard,
    workspaceResultCard: cards.workspaceResultCard,
    hostRows,
  });
  const dispatchEvents = asArray(pickField(planCardModel, "dispatchEvents", "dispatch_events"));
  const eventItems = buildProtocolEventItems({
    planCard: cards.planCard,
    dispatchSummaryCards: cards.dispatchSummaryCards,
    approvalCards: cards.approvalCards,
    choiceCards: cards.choiceCards,
    workspaceResultCard: cards.workspaceResultCard,
    hostRows,
    systemNoticeCards: cards.currentMissionCards.filter((card) => isSystemNoticeCard(card)),
    dispatchEvents,
    commandCards: cards.eventCommandCards,
    toolInvocations,
    verificationRecords,
    incidentEvents,
  });
  const approvalItems = buildProtocolApprovalItems(cards.approvalCards, hostRows);
  const linkedCollections = linkProtocolProjectionCollections({
    approvalItems,
    eventItems,
    verificationRecords,
    evidenceSummaries,
    toolInvocations,
  });
  const linkedApprovalItems = linkedCollections.approvalItems;
  const linkedEventItems = linkedCollections.eventItems;
  const linkedVerificationRecords = linkedCollections.verificationRecords;
  evidenceSummaries = linkedCollections.evidenceSummaries;
  const linkedToolInvocations = linkedCollections.toolInvocations;
  const backgroundAgents = buildProtocolBackgroundAgents(hostRows);
  const conversationStatusCard = buildProtocolConversationStatusCard({
    missionPhase,
    turnActive: runtime?.turn?.active === true,
    approvalItems: linkedApprovalItems,
    backgroundAgents,
    choiceCards: cards.choiceCards,
    planCardModel,
    cards,
  });
  const formattedTurns = formatProtocolChatTurns({
    conversationCards: cards.conversationCards,
    processCards: cards.processCards,
    missionPhase,
    turnActive: runtime?.turn?.active === true,
    statusCard: conversationStatusCard,
    approvalItems: linkedApprovalItems,
    commandCards: cards.commandCards,
    evidenceSummaries,
  });
  const activeProcessTurnId = formattedTurns.find((turn) => turn.active)?.id || "";
  const canStopCurrentMission =
    Boolean(runtime?.turn?.active) &&
    !["aborted", "failed", "completed"].includes(missionPhase) &&
    !cards.stopNoticeCard &&
    !currentFailureCard;
  const nextSendStartsNewMission = !canStopCurrentMission && Boolean(cards.stopNoticeCard || currentFailureCard || missionPhase === "completed");
  const currentMode = inferProtocolIncidentMode({
    snapshot,
    missionPhase,
    approvalItems: linkedApprovalItems,
    verificationRecords: linkedVerificationRecords,
  });
  const currentStage = inferProtocolIncidentStage({
    snapshot,
    missionPhase,
    currentMode,
    currentFailureCard,
    verificationRecords: linkedVerificationRecords,
    cards,
  });
  const incidentSummary = buildProtocolIncidentSummary({
    currentMode,
    currentStage,
    missionPhase,
    approvalItems: linkedApprovalItems,
    verificationRecords: linkedVerificationRecords,
    evidenceSummaries,
    incidentEvents,
    currentFailureCard,
    cards,
    turnActive: runtime?.turn?.active === true,
    nextSendStartsNewMission,
  });
  const turnPolicy = normalizeTurnPolicy(snapshot.turnPolicy);
  const promptEnvelope = normalizePromptEnvelope(snapshot.promptEnvelope);
  const currentLane = inferProtocolTurnLane({
    snapshot,
    missionPhase,
    currentStage,
    currentMode,
    turnPolicy,
    promptEnvelope,
  });
  const requiredNextTool = compactText(
    snapshot.requiredNextTool
      || turnPolicy.requiredNextTool
      || promptEnvelope.requiredNextTool,
  );
  const requiredNextToolDescriptor = requiredNextTool
    ? resolveWorkspaceToolDescriptor(
        findPromptEnvelopeTool(promptEnvelope.visibleTools, requiredNextTool)
          || findPromptEnvelopeTool(promptEnvelope.hiddenTools, requiredNextTool)
          || requiredNextTool,
      )
    : { displayName: "" };
  const finalGateStatus = normalizeTurnFinalGateStatus(
    snapshot.finalGateStatus
      || turnPolicy.finalGateStatus
      || promptEnvelope.finalGateStatus,
  );
  const missingRequirements = asArray(
    snapshot.missingRequirements?.length
      ? snapshot.missingRequirements
      : turnPolicy.missingRequirements?.length
        ? turnPolicy.missingRequirements
        : promptEnvelope.missingRequirements,
  ).map((item) => compactText(item)).filter(Boolean);
  const incidentInsights = buildProtocolIncidentInsights({
    eventItems: linkedEventItems,
    verificationRecords: linkedVerificationRecords,
    cards,
    planCardModel,
  });
  let statusBanner = null;
  if (currentFailureCard) {
    statusBanner = {
      cardId: compactText(currentFailureCard.id),
      tone: "danger",
      title: compactText(currentFailureCard.title || "当前 mission 执行失败"),
      detail:
        compactText(currentFailureCard.text || currentFailureCard.message || currentFailureCard.summary) ||
        "查看左侧对话和右侧事件，确认失败原因后再发起下一轮。",
      runtimeText: `失败 | ${compactText(currentFailureCard.title || currentFailureCard.summary || "当前 mission 执行失败")}`,
    };
  } else if (cards.stopNoticeCard) {
    statusBanner = {
      cardId: compactText(cards.stopNoticeCard.id),
      tone: "warning",
      title: "当前 mission 已停止",
      detail: "再次发送会在当前工作台里启动一轮新的 mission，不会续跑已停止的那一轮。",
      runtimeText: "已停止 | 再次发送将启动新 mission",
    };
  } else if (missionPhase === "completed" && runtime?.turn?.active !== true) {
    // Don't show the "上一轮任务已完成" banner — the placeholder text in the
    // composer already tells the user that the next send starts a new mission.
    statusBanner = null;
  }

  return {
    missionPhase,
    currentMode,
    currentStage,
    currentLane,
    currentLaneLabel: turnLaneLabel(currentLane),
    turnIntentLabel: turnIntentLabel(turnPolicy.intentClass || promptEnvelope.intentClass),
    finalGateStatus,
    finalGateLabel: turnFinalGateLabel(finalGateStatus),
    requiredNextTool,
    requiredNextToolLabel: requiredNextToolDescriptor.displayName,
    missingRequirements,
    turnPolicy: {
      ...turnPolicy,
      lane: currentLane || turnPolicy.lane,
      requiredNextTool: requiredNextTool || turnPolicy.requiredNextTool,
      finalGateStatus,
      missingRequirements,
    },
    promptEnvelope: {
      ...promptEnvelope,
      currentLane: currentLane || promptEnvelope.currentLane,
      intentClass: turnPolicy.intentClass || promptEnvelope.intentClass,
      finalGateStatus,
      missingRequirements,
    },
    incidentSummary,
    incidentInsights,
    cards,
    hostRows,
    approvalItems: linkedApprovalItems,
    backgroundAgents,
    choiceCards: cards.choiceCards,
    planCardModel,
    eventItems: linkedEventItems,
    toolInvocations: linkedToolInvocations,
    evidenceSummaries,
    incidentEvents,
    verificationRecords: linkedVerificationRecords,
    agentLoop: snapshot.agentLoop || null,
    agentLoopIterations: asArray(snapshot.agentLoopIterations),
    conversationStatusCard,
    formattedTurns,
    activeProcessTurnId,
    canStopCurrentMission,
    nextSendStartsNewMission,
    statusBanner,
    currentFailureCard,
    invocationEvidenceIndex: buildInvocationEvidenceIndex(
      snapshot.toolInvocations,
      snapshot.evidenceSummaries
    ),
  };
}
