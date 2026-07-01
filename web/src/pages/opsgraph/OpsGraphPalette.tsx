import type { ComponentType } from "react";
import { Boxes, Database, Globe2, Network } from "lucide-react";

import type { OpsGraphNodeType } from "./opsGraphTypes";

export type OpsGraphPaletteItem = {
  key: string;
  label: string;
  type: Extract<OpsGraphNodeType, "service" | "middleware" | "external">;
  subtype?: string;
  icon: ComponentType<{ className?: string }>;
};

const palette: OpsGraphPaletteItem[] = [
  { key: "service", label: "业务服务", type: "service", icon: Network },
  { key: "middleware-generic", label: "通用中间件", type: "middleware", subtype: "generic", icon: Boxes },
  { key: "middleware-redis", label: "Redis", type: "middleware", subtype: "redis", icon: Database },
  { key: "middleware-postgres", label: "Postgres", type: "middleware", subtype: "postgres", icon: Database },
  { key: "middleware-mysql", label: "MySQL", type: "middleware", subtype: "mysql", icon: Database },
  { key: "middleware-zk", label: "Zookeeper", type: "middleware", subtype: "zk", icon: Boxes },
  { key: "middleware-rabbitmq", label: "RabbitMQ", type: "middleware", subtype: "rabbitmq", icon: Boxes },
  { key: "middleware-nginx", label: "Nginx", type: "middleware", subtype: "nginx", icon: Network },
  { key: "external", label: "外部服务", type: "external", icon: Globe2 },
];

export function OpsGraphPalette({ onCreateNode }: { onCreateNode?: (item: OpsGraphPaletteItem) => void }) {
  return (
    <section className="grid gap-2">
      <div>
        <h2 className="text-sm font-semibold text-slate-950">素材</h2>
        <p className="mt-1 text-xs leading-5 text-slate-500">拖入画布或点击创建拓扑节点。</p>
      </div>
      <div className="grid gap-2">
        {palette.map((item) => {
          const Icon = item.icon;
          return (
            <button
              key={item.key}
              type="button"
              draggable
              data-node-type={item.type}
              data-node-subtype={item.subtype || ""}
              onClick={() => onCreateNode?.(item)}
              onDragStart={(event) => {
                event.dataTransfer.setData("application/x-opsgraph-node", JSON.stringify({ type: item.type, subtype: item.subtype, label: item.label }));
                event.dataTransfer.setData("application/x-opsgraph-node-type", item.type);
                event.dataTransfer.effectAllowed = "copy";
              }}
              className="flex min-h-9 items-center justify-between gap-2 rounded-lg border bg-white px-3 py-2 text-left text-sm text-slate-700 hover:bg-slate-50"
            >
              <span className="inline-flex min-w-0 items-center gap-2">
                <Icon className="h-4 w-4 shrink-0 text-slate-500" />
                <span className="truncate">{item.label}</span>
              </span>
            </button>
          );
        })}
      </div>
    </section>
  );
}
