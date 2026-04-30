export function selectActiveProjection(state, sessionId) {
  if (!state || !sessionId) return null;
  return state.projectionsBySession?.[sessionId] || null;
}

export function selectTimelineRows(state, sessionId) {
  return (selectActiveProjection(state, sessionId)?.timeline || []).filter((row) => row?.kind !== "approval");
}

export function selectRuntimeStatus(state, sessionId) {
  return selectActiveProjection(state, sessionId)?.status || "idle";
}

export function selectRuntimeLiveness(state, sessionId) {
  const projection = selectActiveProjection(state, sessionId);
  return {
    activeTurns: { ...(projection?.runtimeLiveness?.activeTurns || {}) },
    activeAgents: { ...(projection?.runtimeLiveness?.activeAgents || {}) },
    pendingApprovals: { ...(projection?.runtimeLiveness?.pendingApprovals || {}) },
    pendingUserInputs: { ...(projection?.runtimeLiveness?.pendingUserInputs || {}) },
    activeCommandStreams: { ...(projection?.runtimeLiveness?.activeCommandStreams || {}) },
  };
}

export function selectRuntimeBusy(state, sessionId) {
  const status = selectRuntimeStatus(state, sessionId);
  if (status === "working" || status === "blocked") return true;
  const live = selectRuntimeLiveness(state, sessionId);
  return [
    live.activeTurns,
    live.activeAgents,
    live.pendingApprovals,
    live.pendingUserInputs,
    live.activeCommandStreams,
  ].some((bucket) => Object.values(bucket || {}).some(Boolean));
}

export function selectAgentRows(state, sessionId) {
  return selectActiveProjection(state, sessionId)?.agents || [];
}

export function selectApprovalDock(state, sessionId) {
  return (selectActiveProjection(state, sessionId)?.approvals || []).filter((approval) => approval.status === "blocked");
}

export function selectFinalMessages(state, sessionId) {
  return Object.values(selectActiveProjection(state, sessionId)?.finalMessages || {});
}

export function selectProjectionActivityLines(state, sessionId) {
  const projection = selectActiveProjection(state, sessionId);
  const timeline = Array.isArray(projection?.timeline) ? projection.timeline : [];
  const scopedTurnIds = currentProjectionTurnIds(projection);
  return timeline
    .filter((row) => isCurrentActivityRow(row, scopedTurnIds))
    .filter((row) => !isInternalRuntimeActivityRow(row))
    .map((row) => formatProjectionActivityLine(row))
    .filter((line) => line.id && line.text);
}

export function selectProjectionStartedAt(state, sessionId) {
  const projection = selectActiveProjection(state, sessionId);
  const timeline = Array.isArray(projection?.timeline) ? projection.timeline : [];
  const turnRow = timeline.find((row) => row?.kind === "turn" && row?.turnId === projection?.currentTurnId);
  return turnRow?.updatedAt || timeline.find((row) => row?.kind === "turn")?.updatedAt || "";
}

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function previewText(value) {
  if (value == null) return "";
  if (typeof value === "string") return value.trim();
  if (Array.isArray(value)) return value.map((item) => previewText(item)).filter(Boolean).join("\n");
  if (typeof value === "object") {
    try {
      return JSON.stringify(value, null, 2).trim();
    } catch {
      return "";
    }
  }
  return String(value).trim();
}

function currentProjectionTurnIds(projection) {
  const turnIds = new Set();
  const add = (value) => {
    const text = compactText(value);
    if (text) turnIds.add(text);
  };
  const currentTurnId = compactText(projection?.currentTurnId);
  if (currentTurnId) {
    turnIds.add(currentTurnId);
    return turnIds;
  }
  for (const [turnId, active] of Object.entries(projection?.runtimeLiveness?.activeTurns || {})) {
    if (active) add(turnId);
  }
  return turnIds;
}

function isActivityRow(row) {
  return (
    row?.kind === "assistant" ||
    row?.kind === "reasoning" ||
    row?.kind === "tool" ||
    row?.kind === "agent" ||
    row?.kind === "plan" ||
    row?.kind === "approval" ||
    row?.kind === "evidence" ||
    row?.kind === "system"
  );
}

function isCurrentActivityRow(row, scopedTurnIds) {
  if (!isActivityRow(row)) return false;
  if (!scopedTurnIds?.size) return true;
  const rowTurnId = compactText(row?.turnId || row?.clientTurnId);
  if (!rowTurnId) return false;
  return scopedTurnIds.has(rowTurnId);
}

function isInternalRuntimeActivityRow(row = {}) {
  const displayKind = compactText(row?.displayKind || row?.payload?.displayKind).toLowerCase();
  if (displayKind === "runtime.activity") return true;
  return false;
}

function isVerboseToolSummary(value = "") {
  const text = compactText(value);
  if (!text) return false;
  if (/^[{[]/.test(text) && /"content"|"result"|"data"|"query"/.test(text)) return true;
  if (text.length > 140 && /\\n|https?:\/\/|"content"|^\{/.test(text)) return true;
  return false;
}

function activitySummary(row = {}) {
  const summary = compactText(row?.summary || row?.text || "");
  if (row?.kind === "tool" && isVerboseToolSummary(summary)) return "";
  return summary;
}

function isGenericSearchSummary(value = "") {
  return /^(?:已|正在)?搜索(?:网页|页面|内容)?(?:完成|成功)?$/u.test(compactText(value));
}

function formatSearchActivityLine({ running = false, failed = false, results = [], queries = [] } = {}) {
  const prefix = failed ? "搜索网页失败" : running ? "正在搜索网页" : "已搜索网页";
  const count = Array.isArray(queries) && queries.length > 1
    ? queries.length
    : Array.isArray(results) && results.length > 1
      ? results.length
      : 0;
  return count > 1 && !running && !failed ? `${prefix} ${count} 次` : prefix;
}

function formatProjectionActivityLine(row = {}) {
  const status = String(row?.status || "").trim().toLowerCase();
  const title = compactText(row?.title || row?.toolName || row?.kind || "任务");
  const summary = activitySummary(row);
  if (row?.kind === "assistant") {
    return {
      id: compactText(row?.id),
      kind: "assistant",
      text: summary,
      status,
      turnId: compactText(row?.turnId),
      clientTurnId: compactText(row?.clientTurnId),
      updatedAt: row?.updatedAt || "",
    };
  }
  if (row?.kind === "plan") {
    return {
      id: compactText(row?.id),
      kind: "plan",
      displayKind: compactText(row?.displayKind || "plan"),
      text: compactText(summary || title),
      summary,
      steps: Array.isArray(row?.steps) ? row.steps : [],
      status,
      turnId: compactText(row?.turnId),
      clientTurnId: compactText(row?.clientTurnId),
      updatedAt: row?.updatedAt || "",
    };
  }
  if (row?.kind === "reasoning") {
    const completed = status === "completed";
    return {
      id: compactText(row?.id),
      kind: "reasoning",
      text: compactText(`${completed ? "思考摘要" : "正在思考"}：${summary || title}`),
      status,
      turnId: compactText(row?.turnId),
      clientTurnId: compactText(row?.clientTurnId),
      updatedAt: row?.updatedAt || "",
    };
  }
  if (row?.kind === "evidence") {
    return {
      id: compactText(row?.id),
      kind: "evidence",
      displayKind: compactText(row?.displayKind),
      text: compactText(title && summary ? `${title}（${summary}）` : title || summary),
      summary,
      source: compactText(row?.source),
      confidence: compactText(row?.confidence),
      window: compactText(row?.window),
      rawRef: compactText(row?.rawRef),
      status,
      turnId: compactText(row?.turnId),
      clientTurnId: compactText(row?.clientTurnId),
      updatedAt: row?.updatedAt || "",
    };
  }
  if (row?.kind === "approval") {
    const displayKind = compactText(row?.displayKind || "approval.command");
    return {
      id: compactText(row?.approvalId || row?.id),
      kind: "approval",
      displayKind,
      text: status === "completed" ? "已确认" : status === "failed" ? "确认失败" : "等待确认",
      command: compactText(row?.command),
      reason: compactText(row?.reason || row?.summary),
      risk: compactText(row?.risk),
      targets: Array.isArray(row?.targets) ? row.targets : [],
      approvalId: compactText(row?.approvalId || row?.id),
      approvalType: compactText(row?.approvalType),
      status,
      turnId: compactText(row?.turnId),
      clientTurnId: compactText(row?.clientTurnId),
      updatedAt: row?.updatedAt || "",
    };
  }
  const running = status === "running" || status === "queued" || status === "blocked";
  const failed = status === "failed";
  const wrap = (verb, fallback = title) => `${verb}${summary ? `（${summary}` + "）" : fallback ? ` ${fallback}` : ""}`;

  let text = "";
  let lineKind = compactText(row?.kind || "activity");
  let command = "";
  let output = "";
  const displayKind = compactText(row?.displayKind).toLowerCase();
  let inputSummary = compactText(row?.inputSummary);
  let queries = Array.isArray(row?.queries) ? row.queries : [];
  let results = Array.isArray(row?.results) ? row.results : [];
  switch (displayKind || title) {
    case "runtime.activity":
      text = `${failed ? `${title}失败` : running ? `正在${title}` : `已${title}`}${summary ? `（${summary}）` : ""}`;
      break;
    case "runtime.card":
      text = `${failed ? "生成卡片失败" : running ? "正在生成卡片" : "已生成卡片"}${summary ? `（${summary}）` : ""}`;
      break;
    case "browser.search":
    case "web_search":
      lineKind = "search";
      if (!queries.length && inputSummary) queries = [inputSummary];
      text = formatSearchActivityLine({
        running,
        failed,
        queries,
        results,
      });
      break;
    case "browser.open":
    case "open_page":
      text = failed ? `浏览网页失败（${summary || "page"}）` : `${running ? "正在浏览网页" : "已浏览网页"}（${summary || "page"}）`;
      break;
    case "browser.find":
    case "find_in_page":
      text = failed ? `检索页面失败（${summary || "content"}）` : `${running ? "正在检索页面内容" : "已在页面中搜索"}（${summary || "content"}）`;
      break;
    case "host.command":
    case "shell_command":
    case "exec_command":
    case "execute_command":
    case "execute_readonly_query":
    case "code_mode":
      lineKind = "command";
      command = compactText(row?.command || row?.inputSummary);
      output = previewText(row?.outputPreview) || compactText(row?.outputSummary || row?.summary || row?.detail);
      text = command
        ? `${failed ? "运行失败" : running ? "正在运行" : "已运行"} ${command}`
        : `${failed ? "运行失败" : running ? "正在运行" : "已运行"}命令`;
      break;
    case "file.list":
    case "list_dir":
    case "list_files":
      text = failed ? `浏览目录失败（${summary || "dir"}）` : `${running ? "正在浏览目录" : "已浏览目录"}（${summary || "dir"}）`;
      break;
    case "file.read":
    case "read_file":
      text = failed ? `读取文件失败（${summary || "file"}）` : `${running ? "正在读取文件" : "已读取文件"}（${summary || "file"}）`;
      break;
    case "file.diff":
    case "write_file":
    case "apply_patch":
      text = failed ? `修改文件失败（${summary || "file"}）` : `${running ? "正在修改文件" : "已修改文件"}（${summary || "file"}）`;
      break;
    case "file.search":
    case "search_files":
      text = failed ? `搜索文件失败（${summary || "query"}）` : `${running ? "正在搜索文件" : "已搜索文件"}（${summary || "query"}）`;
      break;
    default:
      text = wrap(failed ? "执行失败" : running ? "正在执行" : "已执行");
  }

  return {
    id: compactText(row?.toolCallId || row?.id),
    kind: lineKind,
    text: compactText(text),
    command,
    output,
    outputPreview: output,
    inputSummary,
    inputPreview: row?.inputPreview,
    queries,
    results,
    displayKind: compactText(row?.displayKind),
    visibility: compactText(row?.visibility || "primary"),
    summary,
    outputSummary: compactText(row?.outputSummary),
    exitCode: Number.isFinite(Number(row?.exitCode)) ? Number(row.exitCode) : undefined,
    durationMs: Number.isFinite(Number(row?.durationMs)) ? Number(row.durationMs) : undefined,
    risk: compactText(row?.risk),
    rawRef: compactText(row?.rawRef),
    status,
    turnId: compactText(row?.turnId),
    clientTurnId: compactText(row?.clientTurnId),
    updatedAt: row?.updatedAt || "",
  };
}
