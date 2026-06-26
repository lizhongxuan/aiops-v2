import { Check, ShieldAlert, X } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

type ApprovalBlockPartProps = {
  block: AiopsProcessBlock;
};

export function ApprovalBlockPart({ block }: ApprovalBlockPartProps) {
  const commands = useAiopsTransportCommands();
  const isBlocked = block.status === "blocked" && !!block.approvalId;

  return (
    <div className="rounded-lg border border-amber-200 bg-amber-50 px-3 py-2 text-sm shadow-sm">
      <div className="flex items-center gap-2">
        <ShieldAlert className="h-4 w-4 shrink-0 text-amber-700" />
        <div className="min-w-0 flex-1 font-medium text-amber-950">
          {block.text || "Needs approval"}
        </div>
        <Badge
          variant="outline"
          className="border-amber-300 bg-white text-amber-800"
        >
          {block.status}
        </Badge>
      </div>
      {block.command ? (
        <pre className="mt-2 overflow-x-auto rounded-md bg-amber-950 px-3 py-2 text-xs leading-5 text-amber-50">
          <code>{block.command}</code>
        </pre>
      ) : null}
      {approvalDetails(block).length ? (
        <dl className="mt-2 grid gap-1.5 rounded-md border border-amber-200 bg-white/70 px-2 py-2 text-xs text-amber-950 sm:grid-cols-[5rem_1fr]">
          {approvalDetails(block).map((detail) => (
            <div key={detail.label} className="contents">
              <dt className="font-medium text-amber-800">{detail.label}</dt>
              <dd className="min-w-0 break-words text-amber-950">
                {detail.value}
              </dd>
            </div>
          ))}
        </dl>
      ) : null}
      {isBlocked ? (
        <div className="mt-2 flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="border-amber-300 bg-white"
            onClick={() =>
              commands.approvalDecision(block.approvalId || "", "reject")
            }
          >
            <X className="h-3.5 w-3.5" />
            Reject
          </Button>
          <Button
            type="button"
            size="sm"
            className="bg-amber-900 text-white hover:bg-amber-800"
            onClick={() =>
              commands.approvalDecision(block.approvalId || "", "accept")
            }
          >
            <Check className="h-3.5 w-3.5" />
            Approve
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function approvalDetails(block: AiopsProcessBlock) {
  return [
    { label: "目标", value: block.targetSummary },
    { label: "风险", value: block.riskSummary || block.risk },
    { label: "来源", value: sourceLabel(block.source) },
    { label: "影响", value: block.expectedEffect },
    { label: "回滚", value: block.rollback },
    { label: "验收", value: block.validation },
  ].filter((item): item is { label: string; value: string } =>
    Boolean(item.value),
  );
}

function sourceLabel(source?: string) {
  switch (source) {
    case "ops_manual":
      return "运维手册";
    case "workflow":
      return "Workflow";
    case "ai_chat_direct":
      return "AI Chat";
    case "multi_host_agent":
      return "多主机 Agent";
    case "runbook":
      return "Runbook";
    default:
      return source || "";
  }
}
