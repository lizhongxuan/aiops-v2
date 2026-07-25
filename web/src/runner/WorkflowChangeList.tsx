import { GitCompareArrows } from "lucide-react";

import type { WorkflowOperation } from "./workspace/workflowMcpContracts";

export type WorkflowChangeListProps = {
  operations: WorkflowOperation[];
  issueCount: number;
  activeIndex: number;
  onSelect: (index: number) => void;
};

export function WorkflowChangeList({ operations, issueCount, activeIndex, onSelect }: WorkflowChangeListProps) {
  const summary = summarizeOperations(operations);
  return (
    <section className="workflow-change-list min-h-0 space-y-2" aria-label="修改轨迹">
      <div className="flex items-center justify-between gap-3">
        <span className="inline-flex items-center gap-2">
          <GitCompareArrows className="h-4 w-4 text-slate-500" aria-hidden="true" />
          <h3 className="text-sm font-semibold text-slate-950">修改轨迹</h3>
        </span>
        <p className="flex flex-wrap justify-end gap-1.5 text-[11px]" data-testid="workflow-change-summary">
          <SummaryCount label="新增" count={summary.added} />
          <SummaryCount label="修改" count={summary.updated} />
          <SummaryCount label="删除" count={summary.deleted} danger />
          <SummaryCount label="问题" count={issueCount} danger={issueCount > 0} />
        </p>
      </div>
      {operations.length ? (
        <ol className="divide-y divide-slate-100 border-y border-slate-200">
          {operations.map((operation, index) => (
            <li key={`${operation.op}-${index}`}>
              <button
                type="button"
                aria-current={index === activeIndex ? "step" : undefined}
                className={`min-h-10 w-full px-2 text-left text-sm ${index === activeIndex ? "bg-sky-50 font-medium text-sky-950" : "text-slate-700 hover:bg-slate-50"}`}
                onClick={() => onSelect(index)}
              >
                <span className="mr-2 text-xs tabular-nums text-slate-400">{`${index + 1}. `}</span>
                {operationLabel(operation)}
              </button>
            </li>
          ))}
        </ol>
      ) : <p className="py-3 text-sm text-slate-500">没有可审阅的修改。</p>}
    </section>
  );
}

function SummaryCount({ label, count, danger = false }: { label: string; count: number; danger?: boolean }) {
  return <span className={`rounded-full px-2 py-0.5 ${danger && count ? "bg-rose-50 text-rose-800" : "bg-slate-100 text-slate-600"}`}>{label} {count}</span>;
}

function summarizeOperations(operations: WorkflowOperation[]) {
  return operations.reduce((result, operation) => {
    if (operation.op === "add_node" || operation.op === "add_edge") result.added += 1;
    else if (operation.op === "delete_node" || operation.op === "delete_edge") result.deleted += 1;
    else result.updated += 1;
    return result;
  }, { added: 0, updated: 0, deleted: 0 });
}

function operationLabel(operation: WorkflowOperation): string {
  switch (operation.op) {
    case "add_node": return `新增节点 ${operation.node.id || operation.clientRef}`;
    case "update_node": return `更新节点 ${operation.nodeId}`;
    case "delete_node": return `删除节点 ${operation.nodeId}`;
    case "add_edge": return `新增连线 ${operation.edge.id || operation.clientRef || `${operation.edge.source} → ${operation.edge.target}`}`;
    case "update_edge": return `更新连线 ${operation.edgeId}`;
    case "delete_edge": return `删除连线 ${operation.edgeId}`;
    case "update_workflow": return "更新工作流设置";
    case "update_layout": return `调整 ${operation.layout.nodes.length} 个节点的位置`;
    case "attach_manual_binding": return `关联操作手册 ${operation.manualRef.manualId}`;
  }
}
