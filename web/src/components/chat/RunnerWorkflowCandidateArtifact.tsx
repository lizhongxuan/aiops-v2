import { useEffect, useMemo, useState } from "react";
import { CheckCircle2, Clock3, Eye, Lock, X } from "lucide-react";

import { Button } from "@/components/ui/button";
import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";

type RunnerNode = {
  id: string;
  title: string;
  detail: string;
};

const DEFAULT_RUNNER_NODES: RunnerNode[] = [
  { id: "env-precheck", title: "环境预检查", detail: "识别目标主机、OS、权限、端口和中间件版本。" },
  { id: "approval", title: "人工审批", detail: "生成风险范围、HostLease 和变更确认点。" },
  { id: "dry-run", title: "Dry Run", detail: "用只读命令验证参数、连接和前置条件。" },
  { id: "controlled-exec", title: "受控执行", detail: "按最小爆炸半径执行参数化 Runner 节点。" },
  { id: "proof", title: "恢复验证", detail: "检查指标、日志、Trace 和业务探针是否恢复。" },
  { id: "rollback", title: "回滚预案", detail: "保留逆向节点和失败时的人工确认点。" },
];

export function RunnerWorkflowCandidateArtifact({ artifact }: { artifact: AiopsTransportAgentUiArtifact }) {
  const [activeIndex, setActiveIndex] = useState(0);
  const [modalOpen, setModalOpen] = useState(false);
  const data = asRecord(artifact.inlineData);
  const workflowName = text(data.workflowName || data.workflow_name || artifact.titleZh || artifact.title) || "Runner Workflow 草稿";
  const workflowId = text(data.workflowId || data.workflow_id || artifact.id);
  const nodes = useMemo(() => normalizeRunnerNodes(data), [data]);

  useEffect(() => {
    if (activeIndex >= nodes.length - 1) return undefined;
    const timer = window.setTimeout(() => setActiveIndex((current) => Math.min(nodes.length - 1, current + 1)), 550);
    return () => window.clearTimeout(timer);
  }, [activeIndex, nodes.length]);

  return (
    <div className="mt-3 rounded-lg border border-slate-100 bg-slate-50 p-3" data-testid="runner-workflow-candidate-artifact">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <div className="text-xs font-medium uppercase tracking-normal text-slate-500">节点生成进度</div>
          <div className="mt-1 font-medium text-slate-950">{workflowName}</div>
          <p className="mt-1 text-xs leading-5 text-slate-500">AI 正在逐步创建 Runner 节点。这里是只读预览，不能编辑、发布或执行工作流。</p>
        </div>
        <span className="inline-flex items-center gap-1 rounded-md border border-slate-200 bg-white px-2 py-1 text-xs text-slate-600">
          <Lock className="h-3.5 w-3.5" />只读预览
        </span>
      </div>
      <div className="mt-3 h-1.5 overflow-hidden rounded-full bg-slate-200">
        <div className="h-full rounded-full bg-slate-900 transition-all duration-500" style={{ width: `${Math.round(((activeIndex + 1) / nodes.length) * 100)}%` }} />
      </div>
      <ol className="mt-3 grid gap-2 md:grid-cols-2">
        {nodes.map((node, index) => {
          const created = index <= activeIndex;
          const Icon = created ? CheckCircle2 : Clock3;
          return (
            <li key={node.id} className={created ? "rounded-lg border border-emerald-100 bg-white p-3" : "rounded-lg border border-slate-100 bg-white/70 p-3"}>
              <div className="flex items-start gap-2">
                <Icon className={created ? "mt-0.5 h-4 w-4 text-emerald-600" : "mt-0.5 h-4 w-4 text-slate-400"} />
                <div className="min-w-0">
                  <div className="text-sm font-medium text-slate-900">{node.title}</div>
                  <p className="mt-1 text-xs leading-5 text-slate-500">{node.detail}</p>
                </div>
              </div>
            </li>
          );
        })}
      </ol>
      <div className="mt-3 flex flex-wrap items-center justify-between gap-2 text-xs text-slate-500">
        <span>{workflowId ? `草稿 ID：${workflowId}` : "草稿已生成，等待人工审核"}</span>
        <Button type="button" size="sm" variant="outline" onClick={() => setModalOpen(true)}>
          <Eye />打开 Runner Studio 只读预览
        </Button>
      </div>
      {modalOpen ? (
        <RunnerWorkflowPreviewModal
          workflowName={workflowName}
          workflowId={workflowId}
          nodes={nodes}
          activeIndex={activeIndex}
          onClose={() => setModalOpen(false)}
        />
      ) : null}
    </div>
  );
}

function RunnerWorkflowPreviewModal({
  workflowName,
  workflowId,
  nodes,
  activeIndex,
  onClose,
}: {
  workflowName: string;
  workflowId: string;
  nodes: RunnerNode[];
  activeIndex: number;
  onClose: () => void;
}) {
  return (
    <div className="fixed inset-0 z-[90] flex items-start justify-center overflow-y-auto bg-slate-950/40 p-4 sm:p-8" onClick={onClose}>
      <section className="relative w-full max-w-4xl rounded-xl bg-white p-4 shadow-2xl" role="dialog" aria-modal="true" aria-label="Runner Studio 只读预览" onClick={(event) => event.stopPropagation()}>
        <Button className="absolute right-3 top-3 bg-white" size="icon" variant="outline" type="button" aria-label="关闭只读预览" onClick={onClose}>
          <X />
        </Button>
        <div className="pr-12">
          <div className="text-xs font-medium text-slate-500">Runner Studio 只读预览</div>
          <h2 className="mt-1 text-lg font-semibold text-slate-950">{workflowName}</h2>
          <p className="mt-1 text-sm leading-6 text-slate-500">AI 正在逐步创建节点。这个弹窗只展示生成过程，不能在这里编辑或发布工作流。</p>
          {workflowId ? <p className="mt-1 font-mono text-xs text-slate-400">{workflowId}</p> : null}
        </div>
        <div className="mt-4 grid gap-3 lg:grid-cols-[minmax(0,1fr)_16rem]">
          <div className="rounded-xl border border-slate-200 bg-slate-50 p-4">
            <ol className="grid gap-3">
              {nodes.map((node, index) => (
                <li key={node.id} className="flex items-center gap-3">
                  <span className={index <= activeIndex ? "flex h-9 w-9 items-center justify-center rounded-full bg-slate-900 text-xs font-medium text-white" : "flex h-9 w-9 items-center justify-center rounded-full bg-white text-xs font-medium text-slate-400"}>
                    {index + 1}
                  </span>
                  <div className="min-w-0 flex-1 rounded-lg border border-slate-100 bg-white p-3">
                    <div className="font-medium text-slate-900">{node.title}</div>
                    <p className="mt-1 text-xs leading-5 text-slate-500">{node.detail}</p>
                  </div>
                </li>
              ))}
            </ol>
          </div>
          <aside className="rounded-xl border border-slate-200 bg-white p-4 text-sm">
            <div className="font-medium text-slate-950">生成约束</div>
            <ul className="mt-3 grid gap-2 text-xs leading-5 text-slate-500">
              <li>只读预览，不允许拖拽、编辑、发布。</li>
              <li>真实执行前必须人工审核。</li>
              <li>发布前必须经过 Dry Run、受控执行和恢复验证。</li>
              <li>失败时优先进入回滚节点和人工确认。</li>
            </ul>
          </aside>
        </div>
      </section>
    </div>
  );
}

function normalizeRunnerNodes(data: Record<string, unknown>): RunnerNode[] {
  const rawNodes = Array.isArray(data.nodes) ? data.nodes : [];
  const nodes = rawNodes
    .map((node, index) => {
      const record = asRecord(node);
      const title = text(record.title || record.name || record.label || record.id);
      if (!title) return null;
      return {
        id: text(record.id) || `node-${index + 1}`,
        title,
        detail: text(record.detail || record.summary || record.description) || "AI 生成的 Runner 工作流节点。",
      };
    })
    .filter((node): node is RunnerNode => Boolean(node));
  return nodes.length ? nodes : DEFAULT_RUNNER_NODES;
}

function asRecord(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function text(value?: unknown) {
  return typeof value === "string" ? value.trim().replace(/\s+/g, " ") : "";
}
