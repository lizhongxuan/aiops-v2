import { AlertTriangle, Trash2, X } from "lucide-react";
import { useEffect, useRef } from "react";

import type { RunnerGraph, RunnerNode } from "@/components/runner/canvasGraphAdapter";

export type WorkflowNodeDeleteImpact = {
  nodeId: string;
  nodeLabel: string;
  connectedEdgeIds: string[];
  referencingNodes: Array<{ nodeId: string; nodeLabel: string }>;
  requiresConfirmation: boolean;
};

export function deriveWorkflowNodeDeleteImpact(graph: RunnerGraph, node: RunnerNode): WorkflowNodeDeleteImpact {
  const nodeId = canonicalString(node.id) || "";
  const nodeReferencePattern = referencePattern(nodeId);
  const connectedEdgeIds = (graph.edges || []).flatMap((edge, index) => {
    if (edge.source !== nodeId && edge.target !== nodeId) return [];
    return [canonicalString(edge.id) || `${edge.source || "?"} → ${edge.target || "?"}（连线 ${index + 1}）`];
  });
  const referencingNodes = (graph.nodes || []).flatMap((candidate) => {
    if (!nodeId || candidate.id === nodeId || !referencesNode(candidate, nodeReferencePattern)) return [];
    return [{ nodeId: candidate.id, nodeLabel: canonicalString(candidate.label) || canonicalString(candidate.step?.name) || candidate.id }];
  });
  const normalizedRisk = typeof node.risk === "string"
    ? node.risk.trim().toLowerCase()
    : String(node.risk?.level || "").trim().toLowerCase();
  return {
    nodeId,
    nodeLabel: canonicalString(node.label) || canonicalString(node.step?.name) || nodeId,
    connectedEdgeIds: stableStrings(connectedEdgeIds),
    referencingNodes: referencingNodes
      .filter((item, index, values) => values.findIndex((candidate) => candidate.nodeId === item.nodeId) === index)
      .sort((left, right) => left.nodeId.localeCompare(right.nodeId)),
    requiresConfirmation: connectedEdgeIds.length > 0
      || referencingNodes.length > 0
      || ["high", "critical", "destructive"].includes(normalizedRisk),
  };
}

export function WorkflowDeleteConfirmation({ impact, onCancel, onConfirm }: {
  impact: WorkflowNodeDeleteImpact;
  onCancel: () => void;
  onConfirm: () => void;
}) {
  const confirmRef = useRef<HTMLButtonElement | null>(null);
  useEffect(() => {
    const previous = document.activeElement instanceof HTMLElement ? document.activeElement : undefined;
    confirmRef.current?.focus();
    return () => {
      if (!previous?.isConnected) return;
      window.requestAnimationFrame(() => previous.focus({ preventScroll: true }));
    };
  }, []);

  return (
    <section
      role="alertdialog"
      aria-modal="true"
      aria-labelledby="runner-node-delete-title"
      aria-describedby="runner-node-delete-description"
      data-testid="runner-node-delete-confirmation"
      className="m-3 rounded-xl border border-rose-200 bg-rose-50 p-4 text-rose-950 shadow-sm"
      onKeyDown={(event) => {
        if (event.key !== "Escape") return;
        event.preventDefault();
        onCancel();
      }}
    >
      <div className="flex items-start gap-3">
        <AlertTriangle className="mt-0.5 h-5 w-5 shrink-0" aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <h3 id="runner-node-delete-title" className="font-semibold">确认删除 {impact.nodeLabel}（{impact.nodeId}）</h3>
          <p id="runner-node-delete-description" className="mt-1 text-sm leading-6">这是高风险修改，确认前不会改变已保存版本。</p>
          {impact.connectedEdgeIds.length ? (
            <div className="mt-3 text-sm">
              <p className="font-medium">同时删除 {impact.connectedEdgeIds.length} 条相连连线：</p>
              <ul className="mt-1 list-disc space-y-1 pl-5">{impact.connectedEdgeIds.map((id) => <li key={id}>{id}</li>)}</ul>
            </div>
          ) : <p className="mt-3 text-sm">没有相连连线会被删除。</p>}
          {impact.referencingNodes.length ? (
            <div className="mt-3 text-sm">
              <p className="font-medium">以下步骤仍引用此节点，删除后需要修复：</p>
              <ul className="mt-1 list-disc space-y-1 pl-5">{impact.referencingNodes.map((item) => <li key={item.nodeId}>{item.nodeLabel}（{item.nodeId}）仍引用此节点</li>)}</ul>
            </div>
          ) : null}
          <p className="mt-3 text-xs leading-5">删除完成后可在“历史”中撤销此操作。</p>
        </div>
        <button type="button" aria-label="取消删除" className="inline-flex h-10 w-10 items-center justify-center rounded-lg" onClick={onCancel}><X className="h-4 w-4" aria-hidden="true" /></button>
      </div>
      <div className="mt-4 flex justify-end gap-2">
        <button type="button" data-testid="runner-node-delete-cancel" className="min-h-11 rounded-lg px-4 text-sm font-medium" onClick={onCancel}>取消</button>
        <button ref={confirmRef} type="button" data-testid="runner-node-delete-confirm" className="inline-flex min-h-11 items-center gap-2 rounded-lg bg-rose-700 px-4 text-sm font-semibold text-white" onClick={onConfirm}><Trash2 className="h-4 w-4" aria-hidden="true" />确认删除</button>
      </div>
    </section>
  );
}

function referencesNode(value: unknown, pattern: RegExp, key = ""): boolean {
  if (["id", "label", "description", "type", "position"].includes(key)) return false;
  if (typeof value === "string") return pattern.test(value);
  if (Array.isArray(value)) return value.some((item) => referencesNode(item, pattern));
  if (!value || typeof value !== "object") return false;
  return Object.entries(value as Record<string, unknown>).some(([childKey, item]) => referencesNode(item, pattern, childKey));
}

function referencePattern(value: string) {
  const escaped = value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
  return new RegExp(`(^|[^A-Za-z0-9_-])${escaped}(?=$|[^A-Za-z0-9_-])`);
}

function stableStrings(values: string[]) {
  return [...new Set(values)].sort((left, right) => left.localeCompare(right));
}

function canonicalString(value: unknown): string | undefined {
  return typeof value === "string" && value.trim() ? value.trim() : undefined;
}
