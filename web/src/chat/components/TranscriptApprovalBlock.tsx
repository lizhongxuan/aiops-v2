import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

import type { AiopsTranscriptBlock } from "./AiopsTranscript";

export function TranscriptApprovalBlock({ block }: { block: AiopsTranscriptBlock }) {
  const approval = block.approval;
  const commands = useAiopsTransportCommands();

  if (!approval) {
    return null;
  }

  const title = approval.title || "等待审批";
  const summary = approval.summary || "需要确认后继续";
  const isResolved = approval.status === "approved" || approval.status === "denied" || approval.status === "rejected";
  const resolvedLabel = approvalStatusLabel(approval.status);

  return (
    <div data-testid="codex-approval-inline" className="mx-1 rounded-lg bg-amber-50 px-3 py-2 text-[14px] leading-6 text-amber-900">
      <div data-testid={`aiops-approval-block-${block.id}`}>
        <div className="font-medium">{title}</div>
        <div className="text-amber-800">{summary}</div>
        {approval.command ? (
          <div data-testid="codex-approval-command" className="mt-2 break-words rounded-md bg-white/70 px-2 py-1 font-mono text-[13px] leading-5 text-amber-950">
            {approval.command}
          </div>
        ) : null}
        {!isResolved ? (
          <div className="mt-2 flex flex-wrap gap-2">
            <button
              type="button"
              className="rounded-md border border-amber-200 bg-white px-2.5 py-1 text-[13px] leading-5 text-amber-950 hover:bg-amber-100"
              onClick={() => commands.approvalDecision(approval.approvalId, "approve")}
            >
              同意
            </button>
            <button
              type="button"
              className="rounded-md border border-amber-200 bg-white px-2.5 py-1 text-[13px] leading-5 text-amber-950 hover:bg-amber-100"
              onClick={() => commands.approvalDecision(approval.approvalId, "deny")}
            >
              拒绝
            </button>
          </div>
        ) : (
          <div className="mt-2 text-[13px] leading-5 text-amber-700">{resolvedLabel}</div>
        )}
      </div>
    </div>
  );
}

function approvalStatusLabel(status?: string) {
  switch (status) {
    case "approved":
      return "已同意";
    case "denied":
    case "rejected":
      return "已拒绝";
    default:
      return "等待审批";
  }
}
