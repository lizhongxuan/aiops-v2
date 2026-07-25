import { AlertTriangle, RefreshCw, Search, ShieldCheck, Trash2 } from "lucide-react";
import { useRef, useState } from "react";

export type WorkflowCandidateActionBinding = {
  candidateId: string;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
};

export type WorkflowCandidateApprovalFact = {
  status: "ready" | "requested" | "approved" | "applying" | "expired" | "revoked" | "conflict" | "apply_response_lost";
  binding: WorkflowCandidateActionBinding;
  approvalId?: string;
  expiresAt?: string;
  receiptQuery?: { idempotencyKey: string; transactionId?: string };
};

export type WorkflowCandidateApplyRequest = WorkflowCandidateActionBinding & { approvalId?: string };
export type WorkflowCandidateRecoveryRequest = { reason: "expired" | "revoked" | "conflict"; binding: WorkflowCandidateActionBinding };
export type WorkflowCandidateReceiptQuery = { binding: WorkflowCandidateActionBinding; idempotencyKey: string; transactionId?: string };

export type WorkflowCandidateActionBarProps = {
  binding: WorkflowCandidateActionBinding;
  approval: WorkflowCandidateApprovalFact;
  changeCount: number;
  now?: () => Date;
  onApply?: (request: WorkflowCandidateApplyRequest) => void | Promise<void>;
  onDiscard?: (binding: WorkflowCandidateActionBinding) => void | Promise<void>;
  onRecover?: (request: WorkflowCandidateRecoveryRequest) => void | Promise<void>;
  onQueryReceipt?: (request: WorkflowCandidateReceiptQuery) => void | Promise<void>;
};

export function WorkflowCandidateActionBar({
  binding,
  approval,
  changeCount,
  now = () => new Date(),
  onApply,
  onDiscard,
  onRecover,
  onQueryReceipt,
}: WorkflowCandidateActionBarProps) {
  const [pendingAction, setPendingAction] = useState<"apply" | "discard" | "recover" | "query">();
  const pendingRef = useRef(false);
  const boundIdentity = canonicalBinding(binding);
  const approvalNeedsID = approval.status === "requested" || approval.status === "approved";
  const identityMatches = Boolean(boundIdentity) && sameBinding(binding, approval.binding) &&
    (!approvalNeedsID || validIdentifier(approval.approvalId));
  const expired = Boolean(approval.expiresAt && new Date(approval.expiresAt).getTime() <= now().getTime());
  const effectiveStatus = !identityMatches ? "conflict" : expired && ["ready", "requested", "approved"].includes(approval.status) ? "expired" : approval.status;

  const runOnce = (kind: NonNullable<typeof pendingAction>, action: (() => void | Promise<void>) | undefined) => {
    if (!action || pendingRef.current) return;
    pendingRef.current = true;
    setPendingAction(kind);
    void Promise.resolve().then(action).catch(() => undefined).finally(() => {
      pendingRef.current = false;
      setPendingAction(undefined);
    });
  };

  if (effectiveStatus === "expired" || effectiveStatus === "revoked" || effectiveStatus === "conflict") {
    const reason = effectiveStatus;
    const title = !identityMatches ? "确认信息与当前修改不一致" : RECOVERY_COPY[reason];
    return (
      <aside className="flex flex-wrap items-center justify-between gap-3 border-t border-amber-200 bg-amber-50 px-4 py-3" role="status" data-testid="workflow-candidate-action-bar">
        <div className="flex min-w-0 items-center gap-2 text-amber-950">
          <AlertTriangle className="h-4 w-4 shrink-0" aria-hidden="true" />
          <div><p className="text-sm font-semibold">{title}</p><p className="text-xs">当前修改尚未应用。</p></div>
        </div>
        <button
          type="button"
          disabled={pendingRef.current || !onRecover}
          className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-amber-900 px-3 text-sm font-medium text-white disabled:bg-amber-200"
          onClick={() => boundIdentity && runOnce("recover", () => onRecover?.({ reason, binding: boundIdentity }))}
        >
          <RefreshCw className="h-4 w-4" aria-hidden="true" />{pendingAction === "recover" ? "正在恢复" : "恢复待确认修改"}
        </button>
      </aside>
    );
  }

  if (effectiveStatus === "apply_response_lost") {
    const query = approval.receiptQuery;
    return (
      <aside className="flex flex-wrap items-center justify-between gap-3 border-t border-sky-200 bg-sky-50 px-4 py-3" role="status" data-testid="workflow-candidate-action-bar">
        <div className="flex min-w-0 items-center gap-2 text-sky-950">
          <Search className="h-4 w-4 shrink-0" aria-hidden="true" />
          <div><p className="text-sm font-semibold">保存响应未确认</p><p className="text-xs">只查询已有回执，不会再次应用。</p></div>
        </div>
        <button
          type="button"
          data-testid="workflow-candidate-query-receipt"
          disabled={pendingRef.current || !query?.idempotencyKey || !onQueryReceipt}
          className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-sky-900 px-3 text-sm font-medium text-white disabled:bg-sky-200"
          onClick={() => query && boundIdentity && runOnce("query", () => onQueryReceipt?.({ binding: boundIdentity, idempotencyKey: query.idempotencyKey, ...(query.transactionId ? { transactionId: query.transactionId } : {}) }))}
        >
          <Search className="h-4 w-4" aria-hidden="true" />{pendingAction === "query" ? "正在查询" : "查询保存结果"}
        </button>
      </aside>
    );
  }

  const applying = effectiveStatus === "applying" || pendingAction === "apply";
  const applyRequest = { ...boundIdentity!, ...(approval.approvalId ? { approvalId: approval.approvalId } : {}) };
  return (
    <aside className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 bg-white px-4 py-3" aria-label="候选修改操作" data-testid="workflow-candidate-action-bar">
      <div className="min-w-0">
        <p className="text-sm font-semibold text-slate-950">{applying ? `正在应用 ${changeCount} 项修改` : `${changeCount} 项修改尚未应用`}</p>
        <p className="text-xs text-slate-500">应用时将核对当前候选版本和画布版本。</p>
      </div>
      <div className="flex items-center gap-2">
        {onDiscard ? (
          <button
            type="button"
            disabled={pendingRef.current || effectiveStatus === "applying"}
            className="inline-flex min-h-10 items-center gap-2 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:text-slate-300"
            onClick={() => runOnce("discard", () => onDiscard(boundIdentity!))}
          >
            <Trash2 className="h-4 w-4" aria-hidden="true" />放弃修改
          </button>
        ) : null}
        <button
          type="button"
          data-testid="workflow-candidate-apply"
          disabled={pendingRef.current || effectiveStatus === "applying" || !onApply || changeCount <= 0}
          className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-slate-950 px-4 text-sm font-medium text-white disabled:bg-slate-200 disabled:text-slate-500"
          onClick={() => runOnce("apply", () => onApply?.(applyRequest))}
        >
          <ShieldCheck className="h-4 w-4" aria-hidden="true" />{applying ? "正在应用" : `应用 ${changeCount} 项修改`}
        </button>
      </div>
    </aside>
  );
}

function sameBinding(left: WorkflowCandidateActionBinding, right: WorkflowCandidateActionBinding): boolean {
  return left.candidateId === right.candidateId && left.changeSetId === right.changeSetId &&
    left.previewHash === right.previewHash && left.expectedRevision === right.expectedRevision;
}

function validBinding(value: WorkflowCandidateActionBinding): boolean {
  return [value.candidateId, value.changeSetId, value.previewHash, value.expectedRevision]
    .every((item) => validIdentifier(item));
}

function canonicalBinding(value: WorkflowCandidateActionBinding): WorkflowCandidateActionBinding | undefined {
  if (!validBinding(value)) return undefined;
  return {
    candidateId: value.candidateId,
    changeSetId: value.changeSetId,
    previewHash: value.previewHash,
    expectedRevision: value.expectedRevision,
  };
}

function validIdentifier(value: unknown): value is string {
  return typeof value === "string" && value === value.trim() && value.length > 0;
}

const RECOVERY_COPY = {
  expired: "确认已过期",
  revoked: "确认已撤销",
  conflict: "画布已发生变化",
} as const;
