import { Handle, Position } from "@xyflow/react";
import { Boxes, Database, Globe2, Network, type LucideIcon } from "lucide-react";
import type { KeyboardEvent, MouseEvent } from "react";

export function OpsGraphCanvasNode({ id, data }: { id: string; data: any }) {
  const node = data.node || {};
  const meta = data.topologyMeta || {};
  const chips: string[] = Array.isArray(meta.chips) ? meta.chips : [];
  const tone = nodeToneClasses(meta.tone || "service");
  const Icon = nodeIconComponent(meta.tone || "service");
  const selectNode = () => {
    if (typeof data.onSelect === "function") data.onSelect(id);
  };
  const handleClick = (event: MouseEvent<HTMLDivElement>) => {
    event.stopPropagation();
    selectNode();
  };
  const handleKeyDown = (event: KeyboardEvent<HTMLDivElement>) => {
    if (event.key !== "Enter" && event.key !== " ") return;
    event.preventDefault();
    event.stopPropagation();
    selectNode();
  };

  return (
    <div
      role="button"
      tabIndex={0}
      aria-label={`选择节点 ${data.label || id}`}
      className={`w-[184px] cursor-grab select-none rounded-md border px-3 py-2 text-xs shadow-sm active:cursor-grabbing ${tone.card} opsgraph-node-card opsgraph-node-card--${meta.tone || "service"}`}
      onClick={handleClick}
      onKeyDown={handleKeyDown}
    >
      <Handle type="target" position={Position.Left} className={`!h-2 !w-2 !border-2 !border-white ${tone.handle}`} />
      <div className="flex items-start gap-2">
        <div className={`opsgraph-node-icon grid h-7 w-7 shrink-0 place-items-center rounded-md ${tone.icon}`}>
          <Icon aria-hidden="true" className="h-4 w-4" strokeWidth={2.25} />
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-sm font-semibold text-slate-950">{data.label}</div>
          <div className="mt-0.5 truncate text-[11px] leading-4 text-slate-500">{meta.typeLabel || data.typeLabel}</div>
        </div>
      </div>
      {meta.summary ? <div className="mt-2 truncate text-[11px] leading-4 text-slate-600">{meta.summary}</div> : null}
      {chips.length ? (
        <div className="mt-2 flex flex-wrap gap-1">
          {chips.slice(0, 3).map((chip) => (
            <span key={chip} className="rounded border bg-slate-50 px-1.5 py-0.5 text-[10px] leading-3 text-slate-600">{chip}</span>
          ))}
        </div>
      ) : null}
      {node.properties?.role && meta.summary !== node.properties.role ? <div className="mt-0.5 text-[11px] leading-4 text-slate-500">{node.properties.role}</div> : null}
      <Handle type="source" position={Position.Right} className={`!h-2 !w-2 !border-2 !border-white ${tone.handle}`} />
    </div>
  );
}

function nodeIconComponent(tone: string): LucideIcon {
  switch (tone) {
    case "database":
    case "cache":
      return Database;
    case "external":
      return Globe2;
    case "middleware":
    case "queue":
      return Boxes;
    case "service":
    case "routing":
    default:
      return Network;
  }
}

function nodeToneClasses(tone: string): { card: string; icon: string; handle: string } {
  const tones: Record<string, { card: string; icon: string; handle: string }> = {
    service: {
      card: "border-sky-200 bg-white ring-1 ring-sky-50",
      icon: "bg-sky-100 text-sky-700",
      handle: "!bg-sky-500",
    },
    database: {
      card: "border-indigo-200 bg-white ring-1 ring-indigo-50",
      icon: "bg-indigo-100 text-indigo-700",
      handle: "!bg-indigo-500",
    },
    cache: {
      card: "border-red-200 bg-white ring-1 ring-red-50",
      icon: "bg-red-100 text-red-700",
      handle: "!bg-red-500",
    },
    queue: {
      card: "border-amber-200 bg-white ring-1 ring-amber-50",
      icon: "bg-amber-100 text-amber-700",
      handle: "!bg-amber-500",
    },
    routing: {
      card: "border-emerald-200 bg-white ring-1 ring-emerald-50",
      icon: "bg-emerald-100 text-emerald-700",
      handle: "!bg-emerald-500",
    },
    external: {
      card: "border-cyan-200 bg-white ring-1 ring-cyan-50",
      icon: "bg-cyan-100 text-cyan-700",
      handle: "!bg-cyan-500",
    },
    middleware: {
      card: "border-violet-200 bg-white ring-1 ring-violet-50",
      icon: "bg-violet-100 text-violet-700",
      handle: "!bg-violet-500",
    },
    legacy: {
      card: "border-slate-200 bg-white ring-1 ring-slate-50",
      icon: "bg-slate-100 text-slate-700",
      handle: "!bg-slate-400",
    },
  };
  return tones[tone] || tones.legacy;
}
