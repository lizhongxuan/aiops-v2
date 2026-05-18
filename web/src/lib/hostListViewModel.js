function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function normalizedText(value) {
  return compactText(value).toLowerCase();
}

function parseTime(value) {
  const timestamp = Date.parse(value || "");
  return Number.isFinite(timestamp) ? timestamp : 0;
}

function resolveSourceLabel(host) {
  const transport = normalizedText(host?.transport);
  if (transport === "local") return "local";
  if (transport === "grpc_reverse") return "client";
  if (transport === "ssh_bootstrap") return "手动";
  return "手动";
}

function resolveHeartbeat(host, now) {
  const status = normalizedText(host?.status);
  const installState = normalizedText(host?.installState);
  if (installState === "unsupported_platform") {
    return { heartbeat: "unsupported_platform", heartbeatLabel: "不支持的平台", heartbeatTone: "error" };
  }
  if (status === "install_failed" || installState === "failed") {
    return { heartbeat: "install_failed", heartbeatLabel: "安装失败", heartbeatTone: "error" };
  }
  if (status === "installing" || status === "pending_install" || installState === "pending_install") {
    return { heartbeat: "installing", heartbeatLabel: "待安装", heartbeatTone: "warning" };
  }
  if (status === "online") {
    const timestamp = parseTime(host?.lastHeartbeat);
    if (timestamp && now.getTime() - timestamp > 60_000) {
      return { heartbeat: "stale", heartbeatLabel: "超时", heartbeatTone: "warning" };
    }
    return { heartbeat: "online", heartbeatLabel: "在线", heartbeatTone: "success" };
  }
  return { heartbeat: "offline", heartbeatLabel: "离线", heartbeatTone: "error" };
}

function resolvePrimaryAction(heartbeat) {
  if (heartbeat === "online") return "session";
  if (heartbeat === "installing") return "install";
  if (heartbeat === "install_failed" || heartbeat === "unsupported_platform") return "retry_install";
  return "reinstall";
}

function isSingleHostSession(session) {
  const kind = normalizedText(session?.kind || "single_host");
  return !kind || kind === "single_host";
}

function sessionHostId(session) {
  return compactText(session?.selectedHostId || "server-local");
}

function isActiveTerminalSession(session) {
  const status = normalizedText(session?.status);
  return status === "running" || status === "starting";
}

function matchesQuery(row, query) {
  if (!query) return true;
  const haystack = [
    row.ip,
    row.user,
    row.id,
    row.raw?.name,
    row.sourceLabel,
    row.sshLabel,
    row.labelText,
    row.installRunId,
    row.installStep,
    row.lastError,
  ].map(normalizedText).join(" ");
  return haystack.includes(query);
}

function matchesFilters(row, filters) {
  const heartbeat = compactText(filters?.heartbeat || "all");
  const source = compactText(filters?.source || "all");
  const ssh = compactText(filters?.ssh || "all");
  if (heartbeat !== "all" && row.heartbeat !== heartbeat) return false;
  if (source !== "all" && row.sourceLabel !== source) return false;
  if (ssh !== "all" && row.sshLabel !== ssh) return false;
  return true;
}

function buildSubtitle({ sourceLabel, version, ip, user }) {
  const source = version ? `${sourceLabel} ${version}` : sourceLabel;
  return `${source} · key ${ip}:${user}`;
}

function fieldText(source, camelKey, snakeKey) {
  return compactText(source?.[camelKey] || (snakeKey ? source?.[snakeKey] : ""));
}

function buildInstallDetailLabel({ installStep, installRunId }) {
  return [installStep, installRunId].filter(Boolean).join(" · ");
}

function labelPairs(host) {
  return Object.entries(host?.labels || {})
    .map(([key, value]) => [compactText(key), compactText(value)])
    .filter(([key, value]) => key && value)
    .sort(([leftKey, leftValue], [rightKey, rightValue]) => `${leftKey}=${leftValue}`.localeCompare(`${rightKey}=${rightValue}`));
}

export function buildHostListViewModel({
  hosts = [],
  sessions = [],
  terminalSessions = [],
  query = "",
  filters = {},
  now = new Date(),
  page = 1,
  pageSize = 20,
} = {}) {
  const sessionCountByHost = new Map();
  for (const session of sessions || []) {
    const hostId = sessionHostId(session);
    if (!isSingleHostSession(session) || hostId === "server-local") continue;
    sessionCountByHost.set(hostId, (sessionCountByHost.get(hostId) || 0) + 1);
  }

  const rows = (hosts || [])
    .filter((host) => compactText(host?.id) && compactText(host?.id) !== "server-local")
    .map((host) => {
      const id = compactText(host.id);
      const ip = compactText(host.address || host.name || id);
      const user = compactText(host.sshUser) || "-";
      const sourceLabel = resolveSourceLabel(host);
      const canUseSsh =
        sourceLabel === "手动"
          ? Boolean(compactText(host.sshUser || host.sshPort))
          : Boolean(host.terminalCapable || host.executable);
      const sshLabel = canUseSsh ? "可 SSH" : "无密码";
      const heartbeat = resolveHeartbeat(host, now instanceof Date ? now : new Date(now));
      const labels = labelPairs(host).map(([key, value]) => ({ key, value, label: `${key}=${value}` }));
      const installRunId = fieldText(host, "installRunId", "install_run_id");
      const installWorkflowId = fieldText(host, "installWorkflowId", "install_workflow_id");
      const installStep = fieldText(host, "installStep", "install_step");
      const lastError = fieldText(host, "lastError", "last_error");
      const canOpenSsh = heartbeat.heartbeat === "online" && Boolean(host.terminalCapable || host.executable);
      return {
        raw: host,
        id,
        ip,
        user,
        title: `${ip} / ${user}`,
        subtitle: buildSubtitle({
          sourceLabel,
          version: compactText(host.agentVersion),
          ip,
          user,
        }),
        ...heartbeat,
        sessionCount: sessionCountByHost.get(id) || 0,
        sourceLabel,
        sshLabel,
        installRunId,
        installWorkflowId,
        installStep,
        lastError,
        installDetailLabel: buildInstallDetailLabel({ installStep, installRunId }),
        canRetryInstall: heartbeat.heartbeat === "install_failed" || heartbeat.heartbeat === "unsupported_platform",
        labels,
        labelText: labels.map((item) => item.label).join(" "),
        canOpenSsh,
        primaryAction: resolvePrimaryAction(heartbeat.heartbeat),
      };
    });

  const search = normalizedText(query);
  const filteredRows = rows.filter((row) => matchesQuery(row, search) && matchesFilters(row, filters));
  const safePageSize = Math.max(1, Number(pageSize) || 20);
  const maxPage = Math.max(1, Math.ceil(filteredRows.length / safePageSize));
  const safePage = Math.min(Math.max(1, Number(page) || 1), maxPage);
  const start = (safePage - 1) * safePageSize;
  const pageRows = filteredRows.slice(start, start + safePageSize);

  return {
    rows: filteredRows,
    allRows: rows,
    pageRows,
    page: safePage,
    pageSize: safePageSize,
    total: filteredRows.length,
    canPrev: safePage > 1,
    canNext: start + safePageSize < filteredRows.length,
    stats: [
      { label: "心跳正常", value: rows.filter((row) => row.heartbeat === "online").length },
      { label: "超过 60s 未心跳", value: rows.filter((row) => row.heartbeat === "stale").length },
      { label: "活跃终端会话", value: (terminalSessions || []).filter(isActiveTerminalSession).length },
    ],
  };
}
