import type { CapabilityListResponse, CapabilityRecord, CapabilityViewItem } from "./capabilityManagementTypes";

function text(value: unknown): string {
  if (value === null || value === undefined) return "";
  if (typeof value === "string") return value.trim();
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  return "";
}

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return Boolean(value) && typeof value === "object" && !Array.isArray(value);
}

function valueSummary(value: unknown): string {
  if (Array.isArray(value)) {
    return value.map(valueSummary).filter(Boolean).join(", ");
  }
  if (isPlainObject(value)) {
    return Object.entries(value)
      .map(([key, entry]) => {
        const summary = valueSummary(entry);
        return summary ? `${key}: ${summary}` : "";
      })
      .filter(Boolean)
      .join(", ");
  }
  return text(value);
}

function connectionSummary(item: CapabilityRecord): string {
  const source = item.connection || item.connector;
  if (!isPlainObject(source)) return valueSummary(source) || "未绑定";
  const name = text(source.name) || text(source.id) || text(source.type);
  const status = text(source.status) || text(source.state);
  return [name, status].filter(Boolean).join(" · ") || valueSummary(source) || "未绑定";
}

function permissionRiskSummary(item: CapabilityRecord): string {
  const permissions = valueSummary(item.permissions);
  const risks = valueSummary(item.risks || item.risk);
  return [permissions, risks].filter(Boolean).join(" / ") || "未声明";
}

function shouldHideFromList(item: CapabilityRecord): boolean {
  const marker = `${text(item.id)} ${text(item.name)} ${text(item.title)}`.toLowerCase();
  return marker.includes("connector.management") || marker.includes("connector 管理") || marker.includes("connector管理");
}

function normalizeCapability(item: CapabilityRecord, index: number): CapabilityViewItem {
  const id = text(item.id) || text(item.name) || text(item.title) || `capability-${index + 1}`;
  const name = text(item.name) || text(item.title) || id;
  return {
    id,
    name,
    description: text(item.description) || text(item.summary),
    sourceLabel: valueSummary(item.source || item.kind) || "unknown",
    connectionSummary: connectionSummary(item),
    permissionRiskSummary: permissionRiskSummary(item),
    runtimeSummary: valueSummary(item.runtime) || "未声明",
    auditSummary: valueSummary(item.audit) || "暂无审计记录",
    raw: item,
  };
}

export function buildCapabilityManagementViewModel(payload: CapabilityListResponse = {}) {
  const records = Array.isArray(payload.items) ? payload.items : Array.isArray(payload.capabilities) ? payload.capabilities : [];
  const items = records.filter((item) => !shouldHideFromList(item)).map(normalizeCapability);
  return { items };
}
