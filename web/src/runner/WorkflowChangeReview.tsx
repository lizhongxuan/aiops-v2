import { AlertTriangle, CheckCircle2, ChevronLeft, ChevronRight } from "lucide-react";
import { useState, type ReactNode } from "react";

import { WorkflowChangeList } from "./WorkflowChangeList";
import type { WorkflowChangeSetV2, WorkflowJsonValue, WorkflowOperation } from "./workspace/workflowMcpContracts";

export type WorkflowReviewFieldDiff = {
  operationIndex: number;
  path: string;
  before?: WorkflowJsonValue;
  after?: WorkflowJsonValue;
};

export type WorkflowChangeReviewProps = {
  changeSet: WorkflowChangeSetV2;
  fieldDiffs?: WorkflowReviewFieldDiff[];
  initialOperationIndex?: number;
  savedChanges?: ReactNode;
  onFocusNode?: (nodeId: string) => void;
  onFocusEdge?: (edgeId: string) => void;
};

export function WorkflowChangeReview({
  changeSet,
  fieldDiffs = [],
  initialOperationIndex = 0,
  savedChanges,
  onFocusNode,
  onFocusEdge,
}: WorkflowChangeReviewProps) {
  const operations = changeSet.operations;
  const [selectedIndex, setSelectedIndex] = useState(() => clampIndex(initialOperationIndex, operations.length));
  const [section, setSection] = useState<"pending" | "saved">("pending");
  const activeIndex = clampIndex(selectedIndex, operations.length);
  const selected = operations[activeIndex];
  const selectedDiffs = fieldDiffs.filter((item) => item.operationIndex === activeIndex);
  const deletions = operations.filter((operation) => operation.op === "delete_node" || operation.op === "delete_edge");
  const unapplied = !["applied", "undone"].includes(changeSet.state);

  const selectOperation = (index: number) => {
    const next = clampIndex(index, operations.length);
    setSelectedIndex(next);
    focusOperation(operations[next], onFocusNode, onFocusEdge);
  };

  return (
    <section className="flex min-h-0 flex-col gap-4 bg-white p-4" aria-labelledby="workflow-change-review-title" data-testid="workflow-change-review">
      {savedChanges ? (
        <nav role="tablist" aria-label="变更类型" className="flex border-b border-slate-200">
          <button type="button" role="tab" aria-selected={section === "pending"} data-testid="workflow-changes-tab-pending" className="min-h-10 flex-1 border-b-2 border-transparent px-2 text-sm aria-selected:border-sky-600 aria-selected:text-sky-800" onClick={() => setSection("pending")}>待审阅 {operations.length}</button>
          <button type="button" role="tab" aria-selected={section === "saved"} data-testid="workflow-changes-tab-saved" className="min-h-10 flex-1 border-b-2 border-transparent px-2 text-sm aria-selected:border-sky-600 aria-selected:text-sky-800" onClick={() => setSection("saved")}>已保存修改</button>
        </nav>
      ) : null}
      {section === "saved" && savedChanges ? <div data-testid="workflow-changes-saved-panel">{savedChanges}</div> : (
        <>
      <header className="space-y-2">
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2 id="workflow-change-review-title" className="text-sm font-semibold text-slate-950">待确认修改</h2>
          <span className={`rounded-full px-2 py-1 text-xs font-semibold ${unapplied ? "bg-amber-100 text-amber-800" : "bg-emerald-100 text-emerald-800"}`}>
            {unapplied ? "尚未应用" : "已应用"}
          </span>
        </div>
        <p className="text-sm leading-6 text-slate-700">{changeSet.intent}</p>
        <p className="text-xs text-slate-500">候选版本 {changeSet.candidateRevision} · 共 {operations.length} 项修改</p>
      </header>

      <section aria-label="风险与检查" className={`rounded-xl border p-3 ${deletions.length ? "border-rose-200 bg-rose-50" : "border-slate-200 bg-slate-50"}`}>
        <div className="flex items-center gap-2">
          {deletions.length ? <AlertTriangle className="h-4 w-4 text-rose-700" aria-hidden="true" /> : <CheckCircle2 className="h-4 w-4 text-emerald-700" aria-hidden="true" />}
          <h3 className="text-sm font-semibold text-slate-950">{deletions.length ? "高风险" : "低风险"}</h3>
        </div>
        {deletions.length ? (
          <ul className="mt-2 space-y-1 text-xs text-rose-900">
            {deletions.map((operation, index) => <li key={`${operation.op}-${index}`}>{deletionLabel(operation)}</li>)}
          </ul>
        ) : <p className="mt-2 text-xs text-slate-600">没有删除已存在的节点或连线。</p>}
        <p className={`mt-3 text-xs font-medium ${changeSet.validation.valid ? "text-emerald-800" : "text-rose-800"}`}>
          {changeSet.validation.valid ? "检查通过" : "检查未通过"}
        </p>
        {changeSet.validation.diagnostics.length ? (
          <ul className="mt-2 space-y-1">
            {changeSet.validation.diagnostics.map((diagnostic, index) => {
              const focusable = Boolean(diagnostic.nodeId || diagnostic.edgeId);
              const content = <><span className="font-mono font-semibold">{diagnostic.code}</span>{diagnostic.field ? ` · ${diagnostic.field}` : ""}</>;
              return (
                <li key={`${diagnostic.code}-${index}`}>
                  {focusable ? (
                    <button
                      type="button"
                      data-workflow-diagnostic={diagnostic.code}
                      className="min-h-8 text-left text-xs text-slate-700 underline decoration-dotted underline-offset-2"
                      onClick={() => diagnostic.nodeId ? onFocusNode?.(diagnostic.nodeId) : diagnostic.edgeId ? onFocusEdge?.(diagnostic.edgeId) : undefined}
                    >
                      {content}
                    </button>
                  ) : <p className="text-xs text-slate-700">{content}</p>}
                </li>
              );
            })}
          </ul>
        ) : null}
      </section>

      <WorkflowChangeList
        operations={operations}
        issueCount={changeSet.validation.diagnostics.length}
        activeIndex={activeIndex}
        onSelect={selectOperation}
      />

      {selected ? (
        <section aria-label="字段差异" className="rounded-xl border border-slate-200 p-3">
          <h3 className="text-sm font-semibold text-slate-950">字段差异</h3>
          {selectedDiffs.length ? (
            <dl className="mt-2 space-y-3">
              {selectedDiffs.map((diff, index) => (
                <div key={`${diff.path}-${index}`} className="min-w-0 text-xs">
                  <dt className="break-all font-mono text-slate-500">{diff.path}</dt>
                  <dd className="mt-1 grid grid-cols-[1fr_auto_1fr] items-start gap-2">
                    <code className="break-all rounded bg-rose-50 p-2 text-rose-800">{formatDiffValue(diff.path, diff.before)}</code>
                    <span className="pt-2 text-slate-400" aria-hidden="true">→</span>
                    <code className="break-all rounded bg-emerald-50 p-2 text-emerald-800">{formatDiffValue(diff.path, diff.after)}</code>
                  </dd>
                </div>
              ))}
            </dl>
          ) : <p className="mt-2 text-xs text-slate-500">此操作没有字段级变更。</p>}
        </section>
      ) : null}

      <nav aria-label="逐项审阅" className="flex items-center justify-between gap-3 border-t border-slate-200 pt-3">
        <button
          type="button"
          data-testid="workflow-review-previous"
          disabled={!operations.length || activeIndex === 0}
          className="inline-flex min-h-10 items-center gap-1 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:text-slate-300"
          onClick={() => selectOperation(activeIndex - 1)}
        >
          <ChevronLeft className="h-4 w-4" aria-hidden="true" />上一项
        </button>
        <span className="text-xs text-slate-500">{operations.length ? activeIndex + 1 : 0} / {operations.length}</span>
        <button
          type="button"
          data-testid="workflow-review-next"
          disabled={!operations.length || activeIndex >= operations.length - 1}
          className="inline-flex min-h-10 items-center gap-1 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:text-slate-300"
          onClick={() => selectOperation(activeIndex + 1)}
        >
          下一项<ChevronRight className="h-4 w-4" aria-hidden="true" />
        </button>
      </nav>
        </>
      )}
    </section>
  );
}

function deletionLabel(operation: WorkflowOperation): string {
  if (operation.op === "delete_node") return `将删除节点 ${operation.nodeId}`;
  if (operation.op === "delete_edge") return `将删除连线 ${operation.edgeId}`;
  return "";
}

function focusOperation(operation: WorkflowOperation | undefined, onFocusNode?: (id: string) => void, onFocusEdge?: (id: string) => void) {
  if (!operation) return;
  if (operation.op === "update_node" || operation.op === "delete_node") onFocusNode?.(operation.nodeId);
  else if (operation.op === "add_node" && operation.node.id) onFocusNode?.(operation.node.id);
  else if (operation.op === "update_edge" || operation.op === "delete_edge") onFocusEdge?.(operation.edgeId);
  else if (operation.op === "add_edge" && operation.edge.id) onFocusEdge?.(operation.edge.id);
  else if (operation.op === "update_layout" && operation.layout.nodes.length) {
    // A layout operation can move several nodes at once. The payload order is
    // authoritative and stable, so focus the first node; its first field diff
    // is rendered first as well and Review/Canvas keep one exact target.
    onFocusNode?.(operation.layout.nodes[0].nodeId);
  }
}

function formatDiffValue(path: string, value: WorkflowJsonValue | undefined): string {
  const field = path.split("/").filter(Boolean).at(-1)?.toLowerCase().replace(/[_-]/g, "") || "";
  if (SENSITIVE_FIELDS.has(field) && field !== "secretref") return "••••••";
  if (value === undefined) return "未设置";
  return typeof value === "string" ? value : JSON.stringify(redactValue(value));
}

function redactValue(value: WorkflowJsonValue): WorkflowJsonValue {
  if (Array.isArray(value)) return value.map(redactValue);
  if (!value || typeof value !== "object") return value;
  return Object.fromEntries(Object.entries(value).map(([key, item]) => {
    const normalized = key.toLowerCase().replace(/[_-]/g, "");
    return [key, SENSITIVE_FIELDS.has(normalized) && normalized !== "secretref" ? "••••••" : redactValue(item)];
  }));
}

function clampIndex(value: number, length: number): number {
  if (length <= 0) return 0;
  return Math.min(Math.max(Number.isFinite(value) ? Math.trunc(value) : 0, 0), length - 1);
}

const SENSITIVE_FIELDS = new Set(["password", "secret", "token", "apikey", "accesskey", "privatekey", "secretref"]);
