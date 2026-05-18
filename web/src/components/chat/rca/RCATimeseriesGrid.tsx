import type { RCAReportSection } from "./rcaReportModel";

export function RCATimeseriesGrid({ payload }: { payload: RCAReportSection["payload"] }) {
  const metrics = records(payload.metrics);

  if (!metrics.length) {
    return <p className="text-xs text-slate-500">暂无关键指标数据。</p>;
  }

  return (
    <div className="grid gap-2 sm:grid-cols-2">
      {metrics.map((metric, index) => (
        <div key={display(metric.id) || display(metric.name) || index} className="rounded border border-slate-200 bg-slate-50 p-2">
          <div className="text-xs font-medium text-slate-900">{display(metric.name) || "metric"}</div>
          {display(metric.entity) ? <div className="mt-1 break-words font-mono text-[11px] text-slate-500">{display(metric.entity)}</div> : null}
          <div className="mt-2 text-sm text-slate-700">{display(metric.valueSummary) || display(metric.value) || "no summary"}</div>
        </div>
      ))}
    </div>
  );
}

function records(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object" && !Array.isArray(item)) : [];
}

function display(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}
