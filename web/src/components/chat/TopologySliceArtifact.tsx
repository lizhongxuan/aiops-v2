import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

export function TopologySliceArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const permissionScope = text(artifact.permissionScope).toLowerCase();
  if (["restricted", "denied", "forbidden"].includes(permissionScope)) {
    return (
      <div className="mt-3 rounded-md border border-amber-200 bg-amber-50 px-3 py-2 text-xs text-amber-900">
        权限受限，仅展示拓扑摘要。
      </div>
    );
  }

  const rows = compactRows([
    { label: "服务", value: formatDisplayValue(pickArtifactValue(artifact, ["services", "nodes"])) },
    { label: "依赖", value: formatDisplayValue(pickArtifactValue(artifact, ["dependencies", "edges"])) },
    { label: "影响范围", value: formatDisplayValue(pickArtifactValue(artifact, ["blastRadius", "blast_radius", "impact"])) },
  ]);
  if (!rows.length) return null;
  return <dl className="mt-3 grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">{rows.map((row) => <div key={row.label} className="grid gap-1 sm:grid-cols-[8rem_1fr] sm:items-start"><dt className="font-medium text-slate-500">{row.label}</dt><dd className="break-words font-mono text-slate-700">{row.value}</dd></div>)}</dl>;
}

function compactRows(rows: Array<{ label: string; value: string }>) { return rows.filter((row) => row.value); }
function pickArtifactValue(artifact: AiopsTransportAgentUiArtifact, keys: string[]): unknown {
  const sources = [artifact as unknown as Record<string, unknown>, asRecord(artifact.payload), asRecord(artifact.inlineData), asRecord(artifact.metadata)];
  for (const source of sources) for (const key of keys) if (source[key] !== undefined && source[key] !== null && source[key] !== "") return source[key];
  return undefined;
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
