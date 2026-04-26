export function formatAgentEventStatus(event = {}) {
  if (event.kind === "turn" && event.phase === "requested") return "正在发送请求";
  if (event.kind === "turn" && event.phase === "started") return "正在等待 Agent 启动";
  if (event.kind === "tool" && event.status === "running") return `正在执行 ${event.payload?.displayName || event.payload?.toolName || "工具"}`;
  if (event.kind === "tool" && event.status === "completed") return `已完成 ${event.payload?.displayName || event.payload?.toolName || "工具"}`;
  if (event.status === "blocked") return "等待确认";
  if (event.status === "failed") return "执行失败";
  return "Working";
}

export function formatTimelineRow(row = {}) {
  return row.summary || row.title || row.id || "";
}

export function formatElapsedStatus(startedAt, now = new Date()) {
  const start = Date.parse(startedAt || "");
  const end = now instanceof Date ? now.getTime() : Date.parse(now || "");
  if (!Number.isFinite(start) || !Number.isFinite(end)) return "正在处理";
  const seconds = Math.max(0, Math.floor((end - start) / 1000));
  return `已处理 ${seconds}s`;
}

export function formatDiffStats(diff = {}) {
  const files = Number(diff.filesCount || diff.files?.length || 0);
  const added = Number(diff.addedLines || 0);
  const removed = Number(diff.removedLines || 0);
  if (!files && !added && !removed) return "";
  return `${files} files, +${added} -${removed}`;
}
