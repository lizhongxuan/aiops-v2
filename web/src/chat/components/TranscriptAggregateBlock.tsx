import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { cn } from "@/lib/utils";

import type { AiopsTranscriptBlock } from "./AiopsTranscript";
import { TranscriptToolDetailsList } from "./TranscriptToolDetailsList";

export function TranscriptAggregateBlock({
  block,
  blocksById,
}: {
  block: AiopsTranscriptBlock;
  blocksById?: Record<string, AiopsTranscriptBlock | undefined>;
}) {
  const [expanded, setExpanded] = useState(false);
  const aggregate = block.aggregate;

  if (!aggregate) {
    return null;
  }

  const childSummaries = (aggregate.childBlockIds || [])
    .map((id) => summaryForChildBlock(blocksById?.[id]))
    .filter(Boolean);

  return (
    <div data-testid={`aiops-aggregate-block-${block.id}`} className="px-1 text-slate-400">
      <button
        type="button"
        className="group flex w-full min-w-0 items-center gap-1.5 text-left text-[15px] leading-7 text-slate-400"
        aria-expanded={expanded}
        aria-label={aggregate.summary}
        onClick={() => setExpanded((value) => !value)}
      >
        <ChevronRight className={cn("h-3.5 w-3.5 shrink-0 text-slate-300 transition-transform", expanded && "rotate-90")} />
        <span className="min-w-0 truncate">{aggregate.summary}</span>
      </button>
      {expanded ? <TranscriptToolDetailsList childSummaries={childSummaries} /> : null}
    </div>
  );
}

function summaryForChildBlock(block?: AiopsTranscriptBlock) {
  if (!block) {
    return "";
  }
  if (block.tool) {
    return block.tool.summary || block.tool.command || block.tool.inputSummary || block.tool.output?.text || "";
  }
  if (block.text) {
    return block.text.text;
  }
  if (block.approval) {
    return block.approval.summary || block.approval.title || block.approval.command || "";
  }
  if (block.thinking) {
    return block.thinking.text || "正在思考";
  }
  if (block.artifact) {
    return block.artifact.summary || block.artifact.title || "";
  }
  return "";
}
