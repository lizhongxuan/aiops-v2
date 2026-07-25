import { GitBranch, Network } from "lucide-react";

import type { RunnerGraph, RunnerNode } from "@/components/runner/canvasGraphAdapter";

export type WorkflowOutlineProps = {
  graph: RunnerGraph;
  selectedNodeId?: string;
  nodeStatus?: Record<string, string | undefined>;
  nodeValidation?: Record<string, string | undefined>;
  onSelectNode: (nodeId: string) => void;
};

export function WorkflowOutline({ graph, selectedNodeId, nodeStatus = {}, nodeValidation = {}, onSelectNode }: WorkflowOutlineProps) {
  const nodes = graph.nodes || [];
  return (
    <nav className="flex min-h-0 flex-col gap-3 bg-white p-3" aria-label="工作流结构导航" data-testid="workflow-structured-outline">
      <header className="flex items-center justify-between gap-2">
        <div>
          <h2 className="text-sm font-semibold text-slate-950">结构大纲</h2>
          <p className="mt-0.5 text-xs text-slate-500">{nodes.length} 个步骤 · {(graph.edges || []).length} 条连线</p>
        </div>
        <Network className="h-4 w-4 text-slate-500" aria-hidden="true" />
      </header>
      {nodes.length ? (
        <ul className="space-y-1" role="listbox" aria-label="工作流步骤">
          {nodes.map((node) => {
            const name = businessNodeName(node);
            const type = businessNodeType(node);
            const run = businessStatus(nodeStatus[node.id]);
            const validation = businessValidation(nodeValidation[node.id]);
            return (
              <li key={node.id}>
                <button
                  type="button"
                  role="option"
                  aria-selected={selectedNodeId === node.id}
                  aria-label={`${name}，${type}，${run}，${validation}`}
                  data-node-id={node.id}
                  className={`flex min-h-11 w-full items-center gap-2 rounded-lg px-2.5 text-left focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-blue-600 ${selectedNodeId === node.id ? "bg-blue-50 text-blue-950" : "text-slate-700 hover:bg-slate-50"}`}
                  onClick={() => onSelectNode(node.id)}
                >
                  <GitBranch className="h-4 w-4 shrink-0" aria-hidden="true" />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate text-sm font-medium">{name}</span>
                    <span className="block truncate text-[11px] text-slate-500">{type} · {run} · {validation}</span>
                  </span>
                </button>
              </li>
            );
          })}
        </ul>
      ) : <p className="rounded-lg border border-dashed border-slate-300 p-3 text-xs text-slate-500">还没有步骤。</p>}
    </nav>
  );
}

function businessNodeName(node: RunnerNode) {
  const step = node.step && typeof node.step === "object" ? node.step as Record<string, unknown> : {};
  const data = node.data && typeof node.data === "object" ? node.data as Record<string, unknown> : {};
  for (const value of [node.label, step.name, data.label, data.title]) if (typeof value === "string" && value.trim()) return value.trim();
  return businessNodeType(node);
}

function businessNodeType(node: RunnerNode) {
  return ({ start: "开始", end: "结束", action: "执行步骤", condition: "条件分支", parallel: "并行分支", join: "汇合", loop: "循环", handler: "异常处理", group: "分组", subflow: "子流程", manual_approval: "人工审批", variable_aggregator: "变量汇总" } as Record<string, string>)[String(node.type || "")] || "工作流步骤";
}

function businessStatus(value?: string) {
  return ({ running: "运行中", succeeded: "运行成功", success: "运行成功", failed: "运行失败", pending: "等待运行", skipped: "已跳过" } as Record<string, string>)[String(value || "").toLowerCase()] || "尚未运行";
}

function businessValidation(value?: string) {
  return ({ error: "存在阻断问题", warning: "有检查提醒", valid: "检查通过", checking: "检查中" } as Record<string, string>)[String(value || "").toLowerCase()] || "待检查";
}
