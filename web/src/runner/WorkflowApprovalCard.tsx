import { AlertTriangle, CheckCircle2, Clock3, ShieldCheck, XCircle } from "lucide-react";
import { useRef, useState } from "react";

import type { AiopsTransportApproval } from "@/transport/aiopsTransportTypes";

export type WorkflowApprovalDecision = "accept" | "reject";

export type WorkflowApprovalCardProps = {
  approval: AiopsTransportApproval;
  blockText?: string;
  onDecision?: (approvalId: string, decision: WorkflowApprovalDecision) => void;
  decisionOwner?: "inline" | "candidate-dialog";
};

type ApprovalPresentation = {
  state: "pending" | "accepted" | "rejected" | "canceled" | "expired" | "unavailable";
  title: string;
  description: string;
};

/** Keeps transport protocol states out of user copy and decisions fail closed. */
export function WorkflowApprovalCard({
  approval,
  blockText,
  onDecision,
  decisionOwner = "inline",
}: WorkflowApprovalCardProps) {
  const [decision, setDecision] = useState<WorkflowApprovalDecision>();
  const decisionRef = useRef(false);
  const presentation = approvalPresentation(approval.status);
  const approvalId = canonicalIdentifier(approval.id);
  const pending = presentation.state === "pending";
  const candidateDialogOwnsDecision = pending && decisionOwner === "candidate-dialog";

  const decide = (next: WorkflowApprovalDecision) => {
    if (!pending || !approvalId || !onDecision || decisionRef.current) return;
    decisionRef.current = true;
    setDecision(next);
    onDecision(approvalId, next);
  };

  return (
    <section
      data-testid="workflow-approval-card"
      data-approval-state={presentation.state}
      className={`rounded-xl border p-3 text-sm ${approvalTone(presentation.state)}`}
      role={pending && !candidateDialogOwnsDecision ? "alertdialog" : "status"}
      aria-label={presentation.title}
    >
      <div className="flex items-start gap-2">
        <ApprovalIcon state={presentation.state} />
        <div className="min-w-0 flex-1">
          <p className="font-semibold">{presentation.title}</p>
          <p className="mt-1 leading-6">{approval.reason?.trim() || blockText?.trim() || presentation.description}</p>
          {approval.targetSummary?.trim() ? <p className="mt-2 text-xs leading-5"><span className="font-medium">影响范围：</span>{approval.targetSummary.trim()}</p> : null}
          {approval.expectedEffect?.trim() ? <p className="text-xs leading-5"><span className="font-medium">确认后：</span>{approval.expectedEffect.trim()}</p> : null}
          {!pending ? <p className="mt-2 text-xs leading-5">{presentation.description}</p> : null}
        </div>
      </div>
      {candidateDialogOwnsDecision ? (
        <p className="mt-3 text-xs leading-5" data-testid="workflow-candidate-approval-owner">
          请在候选修改操作栏完成最终确认；这里仅展示服务端确认状态。
        </p>
      ) : pending ? (
        <div className="mt-3 flex flex-wrap gap-2">
          <button
            type="button"
            data-testid="workflow-approval-accept"
            disabled={!approvalId || !onDecision || decisionRef.current}
            className="min-h-11 rounded-lg bg-slate-950 px-3 text-xs font-medium text-white disabled:bg-slate-300"
            onClick={() => decide("accept")}
          >
            {decision === "accept" ? "正在确认" : "确认继续"}
          </button>
          <button
            type="button"
            data-testid="workflow-approval-reject"
            disabled={!approvalId || !onDecision || decisionRef.current}
            className="min-h-11 rounded-lg border border-amber-300 px-3 text-xs font-medium disabled:opacity-50"
            onClick={() => decide("reject")}
          >
            {decision === "reject" ? "正在拒绝" : "拒绝本次修改"}
          </button>
        </div>
      ) : null}
    </section>
  );
}

function approvalPresentation(status: unknown): ApprovalPresentation {
  const normalized = typeof status === "string" ? status.trim().toLowerCase() : "";
  if (!normalized || normalized === "pending" || normalized === "blocked" || normalized === "requested") {
    return { state: "pending", title: "需要你确认", description: "确认前不会应用本次修改。" };
  }
  if (["accepted", "approved", "consumed", "completed"].includes(normalized)) {
    return { state: "accepted", title: "已确认，正在继续", description: "系统将继续核对并保存当前待确认修改。" };
  }
  if (["rejected", "denied", "declined"].includes(normalized)) {
    return { state: "rejected", title: "已拒绝，本次修改不会应用", description: "已保存版本保持不变。" };
  }
  if (["canceled", "cancelled"].includes(normalized)) {
    return { state: "canceled", title: "已取消，本次修改不会应用", description: "已保存版本保持不变；如需继续，请重新发起修改。" };
  }
  if (["expired", "revoked"].includes(normalized)) {
    return { state: "expired", title: "确认已过期，请重新生成待确认修改", description: "旧确认不能再次使用，已保存版本保持不变。" };
  }
  return { state: "unavailable", title: "确认状态暂不可用", description: "请重新生成待确认修改；已保存版本保持不变。" };
}

function ApprovalIcon({ state }: { state: ApprovalPresentation["state"] }) {
  const className = "mt-0.5 h-4 w-4 shrink-0";
  if (state === "accepted") return <CheckCircle2 className={className} aria-hidden="true" />;
  if (state === "rejected" || state === "canceled") return <XCircle className={className} aria-hidden="true" />;
  if (state === "expired") return <Clock3 className={className} aria-hidden="true" />;
  if (state === "pending") return <ShieldCheck className={className} aria-hidden="true" />;
  return <AlertTriangle className={className} aria-hidden="true" />;
}

function approvalTone(state: ApprovalPresentation["state"]) {
  if (state === "accepted") return "border-emerald-200 bg-emerald-50 text-emerald-950";
  if (state === "canceled") return "border-slate-200 bg-slate-50 text-slate-900";
  return "border-amber-200 bg-amber-50 text-amber-950";
}

function canonicalIdentifier(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 && value === value.trim() ? value : undefined;
}
