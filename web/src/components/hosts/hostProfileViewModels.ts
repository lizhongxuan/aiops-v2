import type { HostLeaseView, HostProfileView } from "../../api/hostProfiles";

export type HostExecutionRiskView = {
  key:
    | "host_offline"
    | "profile_expired"
    | "missing_env_label"
    | "host_id_conflict"
    | "platform_mismatch"
    | "host_lease_conflict";
  label: string;
  tone: "danger" | "warning" | "neutral";
  hostId: string;
  message: string;
};

export type HostProfileRowView = HostProfileView & {
  statusLabel: string;
  statusTone: "success" | "warning" | "danger" | "neutral";
  osLabel: string;
  archLabel: string;
  envLabel: string;
  roleLabel: string;
  labelsText: string;
  activeLeaseCount: number;
  riskCount: number;
  riskKeys: HostExecutionRiskView["key"][];
};

export type HostLeaseRowView = HostLeaseView & {
  statusLabel: string;
  statusTone: "success" | "warning" | "danger" | "neutral";
  missionLabel: string;
  ownerSessionLabel: string;
};

export type HostProfileDetailItemView = {
  label: string;
  value: string;
};

export type HostProfileDetailSectionView = {
  title: string;
  items: HostProfileDetailItemView[];
};

export type HostProfileDetailView = {
  hostId: string;
  displayName: string;
  sections: HostProfileDetailSectionView[];
};

export type HostTerminalEntryView = {
  canOpenTerminal: boolean;
  disabledReason: string;
};

type LooseRecord = Record<string, unknown>;

function isRecord(value: unknown): value is LooseRecord {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function text(value: unknown, fallback = "") {
  if (value === undefined || value === null) return fallback;
  const normalized = String(value).trim();
  return normalized || fallback;
}

function pick(source: LooseRecord, ...keys: string[]) {
  for (const key of keys) {
    if (source[key] !== undefined && source[key] !== null && source[key] !== "") return source[key];
  }
  return "";
}

function sanitizeKey(key: string) {
  const normalized = key.replace(/[_\-\s]/g, "").toLowerCase();
  return (
    normalized.includes("token") ||
    normalized.includes("password") ||
    normalized.includes("passwd") ||
    normalized.includes("secret") ||
    normalized.includes("privatekey") ||
    normalized.includes("cookie") ||
    normalized.includes("authorization") ||
    normalized.includes("requestbody") ||
    normalized === "body"
  );
}

function sanitizeRaw(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(sanitizeRaw);
  if (!isRecord(value)) return value;
  return Object.fromEntries(
    Object.entries(value)
      .filter(([key]) => !sanitizeKey(key))
      .map(([key, rawValue]) => [key, sanitizeRaw(rawValue)]),
  );
}

function normalizeLabels(value: unknown): Record<string, string> {
  if (!isRecord(value)) return {};
  return Object.fromEntries(
    Object.entries(value)
      .map(([key, labelValue]) => [key, text(labelValue)] as const)
      .filter(([, labelValue]) => labelValue),
  );
}

function normalizeOs(value: unknown) {
  const normalized = text(value, "unknown").toLowerCase();
  if (normalized === "darwin" || normalized === "macos") return "macOS";
  if (normalized === "linux") return "Linux";
  if (normalized === "windows" || normalized === "win32") return "Windows";
  return text(value, "未知 OS");
}

function normalizeArch(value: unknown) {
  const normalized = text(value, "unknown").toLowerCase();
  if (normalized === "amd64") return "x86_64";
  if (normalized === "arm64" || normalized === "aarch64") return "arm64";
  return text(value, "未知架构");
}

function statusMeta(status: unknown) {
  const normalized = text(status, "unknown").toLowerCase();
  if (["online", "ready", "healthy", "active"].includes(normalized)) {
    return { label: "在线", tone: "success" as const };
  }
  if (["offline", "lost", "unreachable", "down"].includes(normalized)) {
    return { label: "离线", tone: "danger" as const };
  }
  if (["draining", "busy", "leased", "pending"].includes(normalized)) {
    return { label: "忙碌", tone: "warning" as const };
  }
  return { label: text(status, "未知状态"), tone: "neutral" as const };
}

export function buildHostTerminalEntry(host: unknown): HostTerminalEntryView {
  const source = isRecord(host) ? host : {};
  const status = text(pick(source, "status", "state")).toLowerCase();
  if (status !== "online") {
    return { canOpenTerminal: false, disabledReason: "主机离线" };
  }
  if (source.terminalCapable === true || source.executable === true) {
    return { canOpenTerminal: true, disabledReason: "" };
  }
  return { canOpenTerminal: false, disabledReason: "主机未启用终端" };
}

function leaseStatusMeta(status: unknown) {
  const normalized = text(status, "unknown").toLowerCase();
  if (["active", "leased", "running"].includes(normalized)) {
    return { label: "占用中", tone: "warning" as const };
  }
  if (["released", "completed", "expired"].includes(normalized)) {
    return { label: "已释放", tone: "success" as const };
  }
  if (["failed", "cancelled", "conflict"].includes(normalized)) {
    return { label: "异常", tone: "danger" as const };
  }
  return { label: text(status, "未知状态"), tone: "neutral" as const };
}

function isActiveLease(lease: HostLeaseView) {
  return ["active", "leased", "running"].includes(lease.status.toLowerCase());
}

function platformToken(value: unknown) {
  return text(value).toLowerCase().replace("amd64", "x86_64").replace("aarch64", "arm64");
}

export function normalizeHostProfile(input: unknown): HostProfileRowView {
  const source = isRecord(input) ? input : {};
  const labels = normalizeLabels(pick(source, "labels", "tags"));
  const hostId = text(pick(source, "hostId", "host_id", "id"), "unknown-host");
  const status = text(pick(source, "status", "state"), "unknown");
  const statusView = statusMeta(status);
  const os = text(pick(source, "os", "osName", "os_name", "platform"), "unknown");
  const arch = text(pick(source, "arch", "architecture"), "unknown");

  return {
    hostId,
    displayName: text(pick(source, "displayName", "display_name", "name", "hostname"), hostId),
    status,
    statusLabel: statusView.label,
    statusTone: statusView.tone,
    os,
    osLabel: normalizeOs(os),
    arch,
    archLabel: normalizeArch(arch),
    labels,
    envLabel: labels.env || labels.environment || "未标注",
    roleLabel: labels.role || "未标注",
    labelsText: Object.entries(labels)
      .map(([key, value]) => `${key}=${value}`)
      .join(", "),
    lastHeartbeatAt: text(pick(source, "lastHeartbeatAt", "last_heartbeat_at", "heartbeat_at")),
    profileExpiresAt: text(pick(source, "profileExpiresAt", "profile_expires_at", "expires_at")),
    activeLeaseCount: 0,
    riskCount: 0,
    riskKeys: [],
    raw: sanitizeRaw(source) as Record<string, unknown>,
  };
}

export function normalizeHostLease(input: unknown): HostLeaseRowView {
  const source = isRecord(input) ? input : {};
  const leaseId = text(pick(source, "leaseId", "lease_id", "id"), "unknown-lease");
  const status = text(pick(source, "status", "state"), "unknown");
  const statusView = leaseStatusMeta(status);

  return {
    leaseId,
    hostId: text(pick(source, "hostId", "host_id"), "unknown-host"),
    status,
    statusLabel: statusView.label,
    statusTone: statusView.tone,
    missionId: text(pick(source, "missionId", "mission_id")),
    missionLabel: text(pick(source, "missionId", "mission_id"), "未绑定任务"),
    ownerSessionId: text(pick(source, "ownerSessionId", "owner_session_id")),
    ownerSessionLabel: text(pick(source, "ownerSessionId", "owner_session_id"), "未绑定会话"),
    acquiredAt: text(pick(source, "acquiredAt", "acquired_at", "created_at")),
    expiresAt: text(pick(source, "expiresAt", "expires_at")),
    raw: sanitizeRaw(source) as Record<string, unknown>,
  };
}

function addRisk(
  risks: HostExecutionRiskView[],
  key: HostExecutionRiskView["key"],
  hostId: string,
  message: string,
) {
  const labels: Record<HostExecutionRiskView["key"], string> = {
    host_offline: "客户端离线",
    profile_expired: "HostProfile 已过期",
    missing_env_label: "环境标签缺失",
    host_id_conflict: "host_id 冲突",
    platform_mismatch: "OS/架构不匹配",
    host_lease_conflict: "HostLease 锁冲突",
  };
  risks.push({ key, label: labels[key], tone: "danger", hostId, message });
}

export function buildHostExecutionRisks({
  profiles = [],
  requiredOs = "",
  requiredArch = "",
  now = new Date(),
  leases = [],
}: {
  profiles?: unknown[];
  leases?: unknown[];
  requiredOs?: string;
  requiredArch?: string;
  now?: string | Date;
} = {}): HostExecutionRiskView[] {
  const normalizedProfiles = profiles.map(normalizeHostProfile);
  const normalizedLeases = leases.map(normalizeHostLease);
  const risks: HostExecutionRiskView[] = [];
  const referenceTime = new Date(now).getTime();
  const hostIdCounts = new Map<string, number>();
  const emittedConflictHostIds = new Set<string>();

  for (const profile of normalizedProfiles) {
    hostIdCounts.set(profile.hostId, (hostIdCounts.get(profile.hostId) || 0) + 1);
  }

  for (const profile of normalizedProfiles) {
    if (["offline", "lost", "unreachable", "down"].includes(profile.status.toLowerCase())) {
      addRisk(risks, "host_offline", profile.hostId, `${profile.displayName} 客户端离线，不能安全下发任务。`);
    }

    const expiresAt = profile.profileExpiresAt ? new Date(profile.profileExpiresAt).getTime() : Number.NaN;
    if (Number.isFinite(expiresAt) && Number.isFinite(referenceTime) && expiresAt < referenceTime) {
      addRisk(risks, "profile_expired", profile.hostId, `${profile.displayName} 的 HostProfile 已过期。`);
    }

    if (!profile.labels.env && !profile.labels.environment) {
      addRisk(risks, "missing_env_label", profile.hostId, `${profile.displayName} 缺少 env/environment 标签。`);
    }

    if ((hostIdCounts.get(profile.hostId) || 0) > 1 && !emittedConflictHostIds.has(profile.hostId)) {
      emittedConflictHostIds.add(profile.hostId);
      addRisk(risks, "host_id_conflict", profile.hostId, `${profile.hostId} 对应多个 HostProfile。`);
    }

    const osMismatch = requiredOs && platformToken(profile.os) !== platformToken(requiredOs);
    const archMismatch = requiredArch && platformToken(profile.arch) !== platformToken(requiredArch);
    if (osMismatch || archMismatch) {
      addRisk(risks, "platform_mismatch", profile.hostId, `${profile.displayName} 与目标 OS/架构要求不匹配。`);
    }
  }

  for (const lease of normalizedLeases) {
    if (lease.status.toLowerCase() === "conflict") {
      addRisk(
        risks,
        "host_lease_conflict",
        lease.hostId,
        `${lease.hostId} 被 Case ${lease.missionLabel} 持有，HostLease ${lease.leaseId} 将在 ${lease.expiresAt || "未知时间"} 过期。`,
      );
    }
  }

  return risks;
}

export function buildHostProfileRows({
  profiles = [],
  leases = [],
  requiredOs = "",
  requiredArch = "",
  now = new Date(),
}: {
  profiles?: unknown[];
  leases?: unknown[];
  requiredOs?: string;
  requiredArch?: string;
  now?: string | Date;
} = {}): HostProfileRowView[] {
  const normalizedLeases = leases.map(normalizeHostLease);
  const risks = buildHostExecutionRisks({ profiles, leases, requiredOs, requiredArch, now });

  return profiles.map(normalizeHostProfile).map((profile) => {
    const profileRisks = risks.filter((risk) => risk.hostId === profile.hostId);
    return {
      ...profile,
      activeLeaseCount: normalizedLeases.filter((lease) => lease.hostId === profile.hostId && isActiveLease(lease)).length,
      riskCount: profileRisks.length,
      riskKeys: profileRisks.map((risk) => risk.key),
    };
  });
}

function detailValue(source: LooseRecord, ...keys: string[]) {
  return text(pick(source, ...keys));
}

function field(label: string, value: unknown, fallback = "-"): HostProfileDetailItemView {
  return { label, value: text(value, fallback) };
}

function runtimeItems(raw: LooseRecord) {
  const runtime = isRecord(raw.runtime) ? raw.runtime : raw;
  return [
    field("OS", pick(runtime, "os_release", "osRelease", "os_name", "os")),
    field("Kernel", pick(runtime, "kernel", "kernel_version", "kernelVersion")),
    field("语言", pick(runtime, "language", "runtime", "runtime_name")),
    field("版本", pick(runtime, "version", "runtime_version", "runtimeVersion")),
  ].filter((item) => item.value !== "-");
}

function serviceRuntimeItems(raw: LooseRecord) {
  const runtime = isRecord(raw.service_runtime)
    ? raw.service_runtime
    : isRecord(raw.serviceRuntime)
      ? raw.serviceRuntime
      : {};
  return [
    field("Supervisor", pick(runtime, "supervisor", "manager")),
    field("Unit", pick(runtime, "unit", "service", "service_name", "serviceName")),
    field("状态", pick(runtime, "status", "state")),
  ].filter((item) => item.value !== "-");
}

export function buildHostProfileDetail({
  profile,
  leases = [],
  reports = [],
}: {
  profile: HostProfileRowView;
  leases?: unknown[];
  reports?: unknown[];
}): HostProfileDetailView {
  const raw = profile.raw || {};
  const normalizedLeases = leases.map(normalizeHostLease).filter((lease) => lease.hostId === profile.hostId);
  const currentLease = normalizedLeases.find((lease) => isActiveLease(lease) || lease.status.toLowerCase() === "conflict");
  const normalizedReports = reports
    .map((report) => (isRecord(report) ? report : {}))
    .filter((report) => text(pick(report, "hostId", "host_id")) === profile.hostId);
  const latestReport = normalizedReports[0];
  const recentCase = currentLease?.missionLabel || detailValue(latestReport, "caseId", "case_id", "missionId", "mission_id");

  return {
    hostId: profile.hostId,
    displayName: profile.displayName,
    sections: [
      {
        title: "基础信息",
        items: [
          field("Host ID", profile.hostId),
          field("名称", profile.displayName),
          field("状态", profile.statusLabel),
          field("标签", profile.labelsText || "未标注"),
        ],
      },
      {
        title: "运行环境",
        items: runtimeItems(raw).length ? runtimeItems(raw) : [field("OS / 架构", `${profile.osLabel} / ${profile.archLabel}`)],
      },
      {
        title: "已安装 Agent",
        items: [
          field("Agent ID", pick(raw, "agent_id", "agentId")),
          field("Agent 版本", pick(raw, "agent_version", "agentVersion")),
          field("最近心跳", profile.lastHeartbeatAt),
        ],
      },
      {
        title: "service runtime",
        items: serviceRuntimeItems(raw).length ? serviceRuntimeItems(raw) : [field("状态", "未上报")],
      },
      {
        title: "最近 Case",
        items: [
          field("Case", recentCase || "暂无"),
          field("最近上报", detailValue(latestReport, "reportedAt", "reported_at", "createdAt", "created_at")),
          field("摘要", detailValue(latestReport, "summary", "description", "detail")),
        ],
      },
      {
        title: "当前 HostLease",
        items: currentLease
          ? [
              field("Lease", currentLease.leaseId),
              field("持有 Case", currentLease.missionLabel),
              field("状态", currentLease.statusLabel),
              field("过期时间", currentLease.expiresAt),
            ]
          : [field("状态", "未锁定")],
      },
    ].map((section) => ({
      ...section,
      items: section.items.filter((item) => item.value !== "-"),
    })),
  };
}
