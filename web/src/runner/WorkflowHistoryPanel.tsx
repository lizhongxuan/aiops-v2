import { AlertTriangle, X } from "lucide-react";

import { WorkflowHistoryRow, type WorkflowDurableHistoryEntry } from "./WorkflowHistoryRow";

export type { WorkflowDurableHistoryEntry } from "./WorkflowHistoryRow";

export type WorkflowHistoryPanelProps = {
  entries: WorkflowDurableHistoryEntry[];
  pending?: { action: "undo" | "redo"; targetTransactionId: string };
  error?: { action: "undo" | "redo"; targetTransactionId: string; message: string; retryHint?: string };
  onUndo?: (transactionId: string) => void | Promise<unknown>;
  onRedo?: (transactionId: string) => void | Promise<unknown>;
  onResolveConflict?: (transactionId: string, strategy: "review_three_way") => void;
  onDismissError?: () => void;
};

export function WorkflowHistoryPanel({ entries, pending, error, onUndo, onRedo, onResolveConflict, onDismissError }: WorkflowHistoryPanelProps) {
  const durableEntries = entries
    .filter((entry) => entry.source === "durable_transaction" && validHistoryEntry(entry))
    .sort((left, right) => Date.parse(right.occurredAt) - Date.parse(left.occurredAt));
  const newestReversibleId = durableEntries.find((entry) => entry.reversible && entry.status !== "conflict")?.transactionId;
  const groups = groupByDay(durableEntries);

  return (
    <section className="flex min-h-0 flex-col bg-white" aria-labelledby="workflow-history-title" aria-busy={Boolean(pending)} data-testid="workflow-history-panel">
      <header className="px-3 pb-2 pt-3">
        <h2 id="workflow-history-title" className="text-sm font-semibold text-slate-950">已保存修改</h2>
        <p className="mt-1 text-xs leading-5 text-slate-500">只显示服务端持久事务；刷新后仍可核对。</p>
      </header>
      {error ? (
        <div className="mx-3 mb-2 rounded-lg border border-rose-200 bg-rose-50 p-2 text-xs text-rose-950" role="alert" data-testid="workflow-history-error">
          <div className="flex items-start gap-2">
            <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
            <div className="min-w-0 flex-1">
              <p className="font-semibold">{error.action === "undo" ? "无法准备撤销候选" : "无法准备重做候选"}</p>
              <p className="mt-1 leading-5">{error.message}</p>
              {error.retryHint ? <p className="mt-1 leading-5">建议：{error.retryHint}</p> : null}
              <p className="mt-1 leading-5">已保存的工作流未发生修改。</p>
            </div>
            {onDismissError ? <button type="button" className="inline-flex min-h-9 min-w-9 items-center justify-center rounded-lg" aria-label="关闭事务操作错误" onClick={onDismissError}><X className="h-4 w-4" aria-hidden="true" /></button> : null}
          </div>
        </div>
      ) : null}
      {durableEntries.length ? (
        <div className="min-h-0 overflow-y-auto" data-testid="workflow-history-scroll">
          {groups.map((group) => (
            <section key={group.label} aria-labelledby={`workflow-history-day-${group.key}`}>
              <h3 id={`workflow-history-day-${group.key}`} className="sticky top-0 z-[1] bg-white px-3 py-1.5 text-[11px] font-semibold text-slate-500">{group.label}</h3>
              <ol aria-label={`${group.label}的持久事务`}>
                {group.entries.map((entry) => (
                  <li key={entry.transactionId} data-transaction-id={entry.transactionId}>
                    <WorkflowHistoryRow
                      entry={entry}
                      primaryUndo={entry.transactionId === newestReversibleId}
                      pending={pending}
                      onUndo={onUndo}
                      onRedo={onRedo}
                      onResolveConflict={onResolveConflict}
                    />
                  </li>
                ))}
              </ol>
            </section>
          ))}
        </div>
      ) : <p className="px-3 pb-4 text-sm text-slate-500">暂无已确认的持久事务。</p>}
    </section>
  );
}

function validHistoryEntry(entry: WorkflowDurableHistoryEntry) {
  return entry.transactionId.trim().length > 0
    && entry.businessLabel.trim().length > 0
    && Number.isInteger(entry.touchedObjectCount)
    && entry.touchedObjectCount >= 0
    && !Number.isNaN(Date.parse(entry.occurredAt));
}

function groupByDay(entries: WorkflowDurableHistoryEntry[]) {
  const groups = new Map<string, WorkflowDurableHistoryEntry[]>();
  entries.forEach((entry) => {
    const key = entry.occurredAt.slice(0, 10);
    groups.set(key, [...(groups.get(key) || []), entry]);
  });
  return [...groups.entries()].map(([key, dayEntries]) => ({
    key,
    label: new Intl.DateTimeFormat("zh-CN", { year: "numeric", month: "long", day: "numeric" }).format(new Date(`${key}T00:00:00Z`)),
    entries: dayEntries,
  }));
}
