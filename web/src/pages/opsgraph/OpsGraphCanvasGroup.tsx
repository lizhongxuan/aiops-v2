import { Handle, Position } from "@xyflow/react";

export function OpsGraphCanvasGroup({ data }: { data: any }) {
  const summary = data.clusterSummary;
  return (
    <div className="h-full min-h-28 w-full min-w-56 cursor-grab select-none rounded-lg border border-slate-300 bg-white/85 p-3 text-sm shadow-sm active:cursor-grabbing">
      <Handle type="target" position={Position.Left} className="!h-2 !w-2 !bg-slate-500" />
      <div className="truncate font-medium text-slate-950">{data.label}</div>
      <div className="mt-1 text-xs text-slate-500">{data.typeLabel}</div>
      {summary ? <div className="mt-2 text-xs text-slate-600">{summary.deploymentLabel}</div> : null}
      {summary?.badges?.length ? (
        <div className="mt-2 flex flex-wrap gap-1">
          {summary.badges.map((badge: string) => <span key={badge} className="rounded-full bg-slate-100 px-2 py-0.5 text-xs text-slate-600">{badge}</span>)}
        </div>
      ) : null}
      <Handle type="source" position={Position.Right} className="!h-2 !w-2 !bg-slate-500" />
    </div>
  );
}
