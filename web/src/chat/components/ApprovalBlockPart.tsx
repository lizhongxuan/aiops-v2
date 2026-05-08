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
        <div className="min-w-0 flex-1 font-medium text-amber-950">{block.text || "Needs approval"}</div>
        <Badge variant="outline" className="border-amber-300 bg-white text-amber-800">
          {block.status}
        </Badge>
      </div>
      {block.command ? (
        <pre className="mt-2 overflow-x-auto rounded-md bg-amber-950 px-3 py-2 text-xs leading-5 text-amber-50">
          <code>{block.command}</code>
        </pre>
      ) : null}
      {isBlocked ? (
        <div className="mt-2 flex gap-2">
          <Button
            type="button"
            variant="outline"
            size="sm"
            className="border-amber-300 bg-white"
            onClick={() => commands.approvalDecision(block.approvalId || "", "reject")}
          >
            <X className="h-3.5 w-3.5" />
            Reject
          </Button>
          <Button
            type="button"
            size="sm"
            className="bg-amber-900 text-white hover:bg-amber-800"
            onClick={() => commands.approvalDecision(block.approvalId || "", "accept")}
          >
            <Check className="h-3.5 w-3.5" />
            Approve
          </Button>
        </div>
      ) : null}
    </div>
  );
}
