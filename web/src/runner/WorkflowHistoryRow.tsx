import { AlertTriangle, CheckCircle2, CornerUpLeft, CornerUpRight, LoaderCircle, MoreHorizontal } from "lucide-react";

export type WorkflowDurableHistoryEntry = {
  source: "durable_transaction";
  transactionId: string;
  businessLabel: string;
  status: "completed" | "completed_with_warnings" | "conflict" | "rejected";
  occurredAt: string;
  actorLabel?: string;
  touchedObjectCount: number;
  reversible: boolean;
  redoable?: boolean;
  conflict?: {
    baseLabel: string;
    currentLabel: string;
    candidateLabel: string;
  };
};

export type WorkflowHistoryRowProps = {
  entry: WorkflowDurableHistoryEntry;
  primaryUndo?: boolean;
  pending?: { action: "undo" | "redo"; targetTransactionId: string };
  onUndo?: (transactionId: string) => void | Promise<unknown>;
  onRedo?: (transactionId: string) => void | Promise<unknown>;
  onResolveConflict?: (transactionId: string, strategy: "review_three_way") => void;
};

export function WorkflowHistoryRow({
  entry,
  primaryUndo = false,
  pending,
  onUndo,
  onRedo,
  onResolveConflict,
}: WorkflowHistoryRowProps) {
  const busy = Boolean(pending);
  const isPending = pending?.targetTransactionId === entry.transactionId;
  const hasSecondaryActions = (!primaryUndo && entry.reversible) || entry.redoable;
  return (
    <article
      className="workflow-history-row border-b border-slate-100 px-3 py-2.5 last:border-b-0"
      data-testid="workflow-history-row"
      data-transaction-key={entry.transactionId}
    >
      <div className="flex min-w-0 items-center gap-3">
        <span className="w-11 shrink-0 text-xs tabular-nums text-slate-500">{formatTime(entry.occurredAt)}</span>
        <span className={`h-2 w-2 shrink-0 rounded-full ${entry.status === "conflict" || entry.status === "rejected" ? "bg-amber-500" : "bg-emerald-500"}`} aria-hidden="true" />
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-medium text-slate-900">{entry.businessLabel}</h3>
          <p className="mt-0.5 text-[11px] text-slate-500">{entry.touchedObjectCount} 个对象 · {entry.actorLabel || "系统"}</p>
        </div>
        <HistoryStatus status={entry.status} />
        {primaryUndo && entry.reversible ? (
          <button
            type="button"
            data-testid="workflow-history-primary-undo"
            className="inline-flex min-h-9 shrink-0 items-center gap-1 rounded-lg border border-slate-300 px-2 text-xs font-medium disabled:opacity-50"
            disabled={!onUndo || busy}
            onClick={() => invokeHistoryAction(onUndo, entry.transactionId)}
          >
            <CornerUpLeft className="h-3.5 w-3.5" aria-hidden="true" />撤销
          </button>
        ) : null}
        {hasSecondaryActions ? (
          <details className="relative shrink-0">
            <summary className="inline-flex min-h-9 min-w-9 cursor-pointer list-none items-center justify-center rounded-lg text-slate-600" aria-label={`${entry.businessLabel}更多操作`}>
              <MoreHorizontal className="h-4 w-4" aria-hidden="true" />
            </summary>
            <div className="mt-2 flex justify-end gap-2">
              {entry.redoable ? <HistoryAction label="重做" Icon={CornerUpRight} disabled={!onRedo || busy} onClick={() => invokeHistoryAction(onRedo, entry.transactionId)} /> : null}
              {!primaryUndo && entry.reversible ? <HistoryAction label="撤销" Icon={CornerUpLeft} disabled={!onUndo || busy} onClick={() => invokeHistoryAction(onUndo, entry.transactionId)} /> : null}
            </div>
          </details>
        ) : null}
      </div>
      {isPending ? (
        <p className="mt-2 inline-flex items-center gap-1 text-xs text-slate-600" role="status" data-testid="workflow-history-pending">
          <LoaderCircle className="h-4 w-4 animate-spin" aria-hidden="true" />
          {pending.action === "undo" ? "正在准备撤销候选…" : "正在准备重做候选…"}
        </p>
      ) : null}
      {entry.status === "conflict" && entry.conflict ? (
        <div className="mt-3 rounded-lg border border-amber-200 bg-amber-50 p-2 text-xs text-amber-950" data-testid="workflow-history-three-way">
          <p className="font-semibold">撤销/重做与后续修改冲突</p>
          <dl className="mt-2 grid grid-cols-[auto_1fr] gap-x-2 gap-y-1">
            <dt>原始版本</dt><dd>{entry.conflict.baseLabel}</dd>
            <dt>当前版本</dt><dd>{entry.conflict.currentLabel}</dd>
            <dt>待恢复修改</dt><dd>{entry.conflict.candidateLabel}</dd>
          </dl>
          <button type="button" className="mt-2 min-h-9 rounded-lg border border-amber-400 px-2 font-medium" disabled={!onResolveConflict} onClick={() => onResolveConflict?.(entry.transactionId, "review_three_way")}>审阅三方差异</button>
        </div>
      ) : null}
    </article>
  );
}

function HistoryAction({ label, Icon, disabled, onClick }: { label: string; Icon: typeof CornerUpLeft; disabled: boolean; onClick: () => void }) {
  return (
    <button type="button" className="inline-flex min-h-9 items-center gap-1 rounded-lg border border-slate-300 px-2 text-xs font-medium disabled:opacity-50" disabled={disabled} onClick={onClick}>
      <Icon className="h-3.5 w-3.5" aria-hidden="true" />{label}
    </button>
  );
}

function HistoryStatus({ status }: { status: WorkflowDurableHistoryEntry["status"] }) {
  if (status === "conflict" || status === "rejected") return <span className="inline-flex shrink-0 items-center gap-1 text-[11px] font-medium text-amber-800"><AlertTriangle className="h-3.5 w-3.5" aria-hidden="true" />{status === "conflict" ? "冲突" : "未应用"}</span>;
  return <span className="inline-flex shrink-0 items-center gap-1 text-[11px] font-medium text-emerald-700"><CheckCircle2 className="h-3.5 w-3.5" aria-hidden="true" />{status === "completed_with_warnings" ? "有提醒" : "已保存"}</span>;
}

function invokeHistoryAction(callback: ((transactionId: string) => void | Promise<unknown>) | undefined, transactionId: string) {
  if (!callback) return;
  void Promise.resolve(callback(transactionId)).catch(() => undefined);
}

function formatTime(value: string) {
  return new Intl.DateTimeFormat("zh-CN", { hour: "2-digit", minute: "2-digit", hour12: false }).format(new Date(value));
}
