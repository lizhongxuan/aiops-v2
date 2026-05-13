import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

export function WorkflowResultArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  return <ArtifactDetails rows={compactRows([
    { label: "主机租约", value: formatHostLease(pickArtifactValue(artifact, ["hostLease", "host_lease", "lease"])) },
    { label: "失败步骤", value: formatDisplayValue(pickArtifactValue(artifact, ["failedStep", "failed_step"])) },
    { label: "回滚结果", value: formatDisplayValue(pickArtifactValue(artifact, ["rollbackResult", "rollback_result"])) },
  ])} />;
}

function ArtifactDetails({ rows }: { rows: Array<{ label: string; value: string }> }) {
  if (!rows.length) return null;
  return <dl className="mt-3 grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">{rows.map((row) => <div key={row.label} className="grid gap-1 sm:grid-cols-[8rem_1fr] sm:items-start"><dt className="font-medium text-slate-500">{row.label}</dt><dd className="break-words font-mono text-slate-700">{row.value}</dd></div>)}</dl>;
}

function compactRows(rows: Array<{ label: string; value: string }>) { return rows.filter((row) => row.value); }
function pickArtifactValue(artifact: AiopsTransportAgentUiArtifact, keys: string[]): unknown {
  const sources = [artifact as unknown as Record<string, unknown>, asRecord(artifact.payload), asRecord(artifact.inlineData), asRecord(artifact.metadata)];
  for (const source of sources) for (const key of keys) if (source[key] !== undefined && source[key] !== null && source[key] !== "") return source[key];
  return undefined;
}
function formatHostLease(value: unknown): string {
  const lease = asRecord(value);
  if (!Object.keys(lease).length) return formatDisplayValue(value);
  const id = text(lease.leaseId) || text(lease.lease_id) || text(lease.id) || text(lease.hostId) || text(lease.host_id);
  const status = text(lease.status) || text(lease.state);
  return id && status ? `${id}（${status}）` : id || status || formatDisplayValue(lease);
}
function formatDisplayValue(value: unknown): string {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "string") return text(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return value.map(formatDisplayValue).filter(Boolean).join("；");
  if (typeof value === "object") return Object.entries(value as Record<string, unknown>).map(([key, entry]) => `${key}：${formatDisplayValue(entry)}`).filter(Boolean).join("；");
  return String(value);
}
function asRecord(value: unknown): Record<string, unknown> { return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {}; }
function text(value?: unknown) { return typeof value === "string" ? value.replace(/<[^>]*>/g, "").trim().replace(/\s+/g, " ") : ""; }
