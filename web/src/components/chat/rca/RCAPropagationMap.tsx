import type { RCAReportSection } from "./rcaReportModel";

export function RCAPropagationMap({ payload }: { payload: RCAReportSection["payload"] }) {
  const nodes = records(payload.nodes);
  const edges = records(payload.edges);

  if (!nodes.length && !edges.length) {
    return <p className="text-xs text-slate-500">暂无传播路径数据。</p>;
  }

  return (
    <div className="grid gap-2 text-xs">
      {nodes.length ? (
        <div className="flex flex-wrap gap-2">
          {nodes.map((node, index) => (
            <span key={display(node.id) || index} className="rounded border border-slate-200 bg-slate-50 px-2 py-1 font-medium text-slate-700">
              {display(node.name) || display(node.id) || "unknown"}
            </span>
          ))}
        </div>
      ) : null}
      {edges.length ? (
        <div className="space-y-1 text-slate-500">
          {edges.map((edge, index) => (
            <div key={`${display(edge.source)}-${display(edge.target)}-${index}`}>
              {display(edge.source) || "unknown"} -&gt; {display(edge.target) || "unknown"}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function records(value: unknown): Array<Record<string, unknown>> {
  return Array.isArray(value) ? value.filter((item): item is Record<string, unknown> => Boolean(item) && typeof item === "object" && !Array.isArray(item)) : [];
}

function display(value: unknown): string {
  return typeof value === "string" || typeof value === "number" ? String(value) : "";
}
