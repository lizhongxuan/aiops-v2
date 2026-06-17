import { Handle, Position } from "@xyflow/react";

export function OpsGraphCanvasNode({ data }: { data: any }) {
  const node = data.node || {};
  return (
    <div className="w-32 cursor-grab select-none rounded-md border border-slate-200 bg-white px-2.5 py-1.5 text-xs shadow-sm active:cursor-grabbing">
      <Handle type="target" position={Position.Left} className="!h-1.5 !w-1.5 !bg-slate-400" />
      <div className="truncate text-sm font-medium text-slate-950">{data.label}</div>
      <div className="mt-0.5 text-[11px] leading-4 text-slate-500">{data.typeLabel}</div>
      {node.properties?.role ? <div className="mt-0.5 text-[11px] leading-4 text-slate-500">{node.properties.role}</div> : null}
      <Handle type="source" position={Position.Right} className="!h-1.5 !w-1.5 !bg-slate-400" />
    </div>
  );
}
