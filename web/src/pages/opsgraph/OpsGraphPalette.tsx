import { Boxes, Network, Server } from "lucide-react";

import { nodeTypeLabel } from "./opsGraphViewModel";

const palette = [
  { type: "service", icon: Network },
  { type: "middleware", icon: Boxes },
  { type: "host", icon: Server },
  { type: "k8s", icon: Server },
];

export function OpsGraphPalette({ onCreateNode }: { onCreateNode?: (type: string) => void }) {
  return (
    <section className="grid gap-2">
      <div>
        <h2 className="text-sm font-semibold text-slate-950">素材</h2>
        <p className="mt-1 text-xs leading-5 text-slate-500">拖入画布或点击创建节点。</p>
      </div>
      <div className="grid gap-2">
        {palette.map((item) => {
          const Icon = item.icon;
          return (
            <button
              key={item.type}
              type="button"
              draggable
              data-node-type={item.type}
              onClick={() => onCreateNode?.(item.type)}
              onDragStart={(event) => {
                event.dataTransfer.setData("application/x-opsgraph-node-type", item.type);
                event.dataTransfer.effectAllowed = "copy";
              }}
              className="flex min-h-9 items-center justify-between gap-2 rounded-lg border bg-white px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-50"
            >
              <span className="inline-flex min-w-0 items-center gap-2">
                <Icon className="h-4 w-4 shrink-0 text-slate-500" />
                <span className="truncate">{nodeTypeLabel(item.type)}</span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
