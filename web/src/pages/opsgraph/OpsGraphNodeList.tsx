import { Search } from "lucide-react";
import { useMemo, useState } from "react";

import { Input } from "@/components/ui/input";

import type { OpsGraphNode } from "./opsGraphTypes";
import { nodeTypeLabel } from "./opsGraphViewModel";

export function OpsGraphNodeList({ nodes }: { nodes: OpsGraphNode[] }) {
  const [query, setQuery] = useState("");
  const filteredNodes = useMemo(() => {
    const normalizedQuery = query.trim().toLowerCase();
    if (!normalizedQuery) return nodes;
    return nodes.filter((node) => [
      node.name,
      node.id,
      nodeTypeLabel(node.type),
      node.type,
    ].some((value) => String(value || "").toLowerCase().includes(normalizedQuery)));
  }, [nodes, query]);

  return (
    <section data-testid="opsgraph-node-list" className="grid min-h-0 grid-rows-[auto_auto_minmax(0,1fr)] gap-2">
      <div>
        <h2 className="text-sm font-semibold text-slate-950">节点</h2>
        <p className="mt-1 text-xs leading-5 text-slate-500">{nodes.length} 个节点</p>
      </div>
      <label className="relative">
        <Search className="pointer-events-none absolute left-2.5 top-2 h-4 w-4 text-slate-400" />
        <Input className="pl-8" placeholder="搜索节点" aria-label="搜索节点" value={query} onChange={(event) => setQuery(event.target.value)} />
      </label>
      <div data-testid="opsgraph-node-list-scroll" className="grid min-h-0 gap-2 overflow-y-auto pr-1">
        {filteredNodes.length ? filteredNodes.map((node) => (
          <button key={node.id} type="button" className="rounded-lg border bg-white p-2 text-left text-sm hover:bg-slate-50">
            <span className="block truncate font-medium text-slate-950">{node.name || node.id}</span>
            <span className="mt-1 block text-xs text-slate-500">{nodeTypeLabel(node.type)}</span>
          </button>
        )) : (
          <p className="rounded-lg border border-dashed bg-slate-50 p-3 text-sm leading-6 text-slate-500">{nodes.length ? "没有匹配的节点。" : "还没有节点。"}</p>
        )}
      </div>
    </section>
  );
}
