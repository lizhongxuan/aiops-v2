import type { RCAReportSection } from "./rcaReportModel";

export function RCATimeline({ payload }: { payload: RCAReportSection["payload"] }) {
  const events = records(payload.events);

  if (!events.length) {
    return <p className="text-xs text-slate-500">暂无时间线事件。</p>;
  }

  return (
    <ol className="space-y-2 text-xs">
      {events.map((event, index) => (
        <li key={display(event.id) || index} className="border-l border-slate-200 pl-3">
          <div className="font-medium text-slate-800">{display(event.message) || display(event.type) || "event"}</div>
          {display(event.timestamp) ? <div className="text-slate-500">{display(event.timestamp)}</div> : null}
        </li>
      ))}
    </ol>
  );
}

function records(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object" && !Array.isArray(item)) : [];
}

function display(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}
