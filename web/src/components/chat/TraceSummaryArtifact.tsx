import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

export function TraceSummaryArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  return (
    <ArtifactDetails
      rows={compactRows([
        { label: "Trace ID", value: text(pickArtifactValue(artifact, ["traceId", "trace_id"])) },
        { label: "总耗时", value: formatDuration(pickArtifactValue(artifact, ["durationMs", "duration_ms", "totalDurationMs", "total_duration_ms", "latencyMs", "latency_ms"])) },
        { label: "最慢 Span", value: formatSlowestSpan(pickArtifactValue(artifact, ["slowestSpan", "slowest_span", "slowSpan", "slow_span"]) || inferSlowestSpan(pickArtifactValue(artifact, ["spans"]))) },
      ])}
    />
  );
}

function ArtifactDetails({ rows }: { rows: Array<{ label: string; value: string }> }) {
  if (!rows.length) return null;
  return (
    <dl className="mt-3 grid gap-2 rounded-lg border border-slate-100 bg-slate-50 p-3 text-xs">
      {rows.map((row) => (
        <div key={row.label} className="grid gap-1 sm:grid-cols-[8rem_1fr] sm:items-start">
          <dt className="font-medium text-slate-500">{row.label}</dt>
          <dd className="break-words font-mono text-slate-700">{row.value}</dd>
        </div>
      ))}
    </dl>
  );
}

function compactRows(rows: Array<{ label: string; value: string }>) {
  return rows.filter((row) => row.value);
}

function pickArtifactValue(artifact: AiopsTransportAgentUiArtifact, keys: string[]): unknown {
  const sources = [artifact as unknown as Record<string, unknown>, asRecord(artifact.payload), asRecord(artifact.inlineData), asRecord(artifact.metadata)];
  for (const source of sources) for (const key of keys) if (source[key] !== undefined && source[key] !== null && source[key] !== "") return source[key];
  return undefined;
}

function inferSlowestSpan(value: unknown): unknown {
  return Array.isArray(value)
    ? value.reduce<unknown>((current, next) => Number(asRecord(next).durationMs || asRecord(next).duration_ms || 0) > Number(asRecord(current).durationMs || asRecord(current).duration_ms || 0) ? next : current, undefined)
    : undefined;
}

function formatSlowestSpan(value: unknown): string {
  const span = asRecord(value);
  if (!Object.keys(span).length) return formatDisplayValue(value);
  const name = text(span.name) || text(span.operation) || text(span.spanName) || text(span.span_name) || "未命名 Span";
  const duration = formatDuration(span.durationMs || span.duration_ms || span.elapsedMs || span.elapsed_ms);
  return duration ? `${name}（${duration}）` : name;
}

function formatDuration(value: unknown): string {
  if (typeof value === "number" && Number.isFinite(value)) return `${value} ms`;
  const normalized = text(value);
  if (!normalized) return "";
  const numeric = Number(normalized);
  return Number.isFinite(numeric) ? `${numeric} ms` : normalized;
}

function formatDisplayValue(value: unknown): string {
  if (value === undefined || value === null || value === "") return "";
  if (typeof value === "string") return text(value);
  if (typeof value === "number" || typeof value === "boolean") return String(value);
  if (Array.isArray(value)) return value.map(formatDisplayValue).filter(Boolean).join("；");
  if (typeof value === "object") return Object.entries(value as Record<string, unknown>).map(([key, entry]) => `${key}：${formatDisplayValue(entry)}`).join("；");
  return String(value);
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function text(value?: unknown) {
  return typeof value === "string" ? value.replace(/<[^>]*>/g, "").trim().replace(/\s+/g, " ") : "";
}
