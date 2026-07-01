import { Edit3 } from "lucide-react";

import { Button } from "@/components/ui/button";

import type { OpsGraphNode, OpsGraphRecord } from "./opsGraphTypes";
import { buildLLMContextPreview, relationshipFacts, topologyNodeMeta } from "./opsGraphViewModel";

export function OpsGraphNodeSummary({
  graph,
  node,
  onEdit,
}: {
  graph: OpsGraphRecord;
  node: OpsGraphNode;
  onEdit: () => void;
}) {
  const meta = topologyNodeMeta(node);
  const facts = relationshipFacts(graph, node.id);
  const props = node.properties || {};
  const deployment = [props.k8sCluster, props.namespace].filter(Boolean).join(" / ") || props.host || props.environment || "未设置部署位置";
  const preview = buildLLMContextPreview(graph, node.id);

  return (
    <aside data-testid="opsgraph-node-summary" className="absolute right-4 top-4 z-10 w-72 rounded-md border bg-white p-3 text-sm shadow-sm">
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate font-semibold text-slate-950">{node.name || node.id}</div>
          <div className="mt-0.5 text-xs text-slate-500">{meta.typeLabel}</div>
        </div>
        <Button type="button" size="sm" variant="outline" onClick={onEdit}>
          <Edit3 />
          编辑属性
        </Button>
      </div>
      <div className="mt-3 grid grid-cols-3 gap-2 text-xs">
        <div className="rounded border bg-slate-50 p-2">
          <span className="block text-slate-500">上游</span>
          <strong>{facts.upstreams.length}</strong>
        </div>
        <div className="rounded border bg-slate-50 p-2">
          <span className="block text-slate-500">下游</span>
          <strong>{facts.downstreams.length}</strong>
        </div>
        <div className="rounded border bg-slate-50 p-2">
          <span className="block text-slate-500">端口</span>
          <strong>{props.ports || "-"}</strong>
        </div>
      </div>
      <div className="mt-3 truncate text-xs text-slate-600">{deployment}</div>
      <pre className="mt-3 max-h-32 overflow-auto whitespace-pre-wrap rounded border bg-slate-50 p-2 text-[11px] leading-5 text-slate-600">
        {`LLM 上下文\n${preview}`}
      </pre>
    </aside>
  );
}
