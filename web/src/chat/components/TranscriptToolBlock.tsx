import { ChevronRight } from "lucide-react";
import { useState } from "react";

import { cn } from "@/lib/utils";

import type { AiopsTranscriptBlock, AiopsTranscriptToolStatus } from "./AiopsTranscript";
import { TerminalOutputCard } from "./TerminalOutputCard";
import { TranscriptToolDetailsList } from "./TranscriptToolDetailsList";

export function TranscriptToolBlock({ block }: { block: AiopsTranscriptBlock }) {
  const tool = block.tool;
  const [userExpanded, setUserExpanded] = useState<boolean | null>(null);

  if (!tool) {
    return null;
  }

  const summary = tool.summary || tool.title || tool.command || tool.toolName || "工具调用";
  const expanded = userExpanded ?? shouldAutoExpandTool(tool.status);
  const isCommand = tool.toolKind === "command";

  return (
    <div data-testid={`aiops-tool-block-${block.id}`} className="px-1 text-slate-400">
      <button
        type="button"
        className="group flex w-full min-w-0 items-center gap-1.5 text-left text-[15px] leading-7 text-slate-400"
        aria-expanded={expanded}
        aria-label={summary}
        onClick={() => setUserExpanded((value) => !(value ?? shouldAutoExpandTool(tool.status)))}
      >
        <ChevronRight className={cn("h-3.5 w-3.5 shrink-0 text-slate-300 transition-transform", expanded && "rotate-90")} />
        <span className="min-w-0 truncate">{summary}</span>
      </button>
      {expanded ? (
        isCommand ? (
          <TerminalOutputCard block={block} />
        ) : (
          <TranscriptToolDetailsList text={tool.output?.text || tool.inputSummary || tool.command || ""} />
        )
      ) : null}
    </div>
  );
}

function shouldAutoExpandTool(status: AiopsTranscriptToolStatus) {
  return status === "queued" || status === "running" || status === "failed" || status === "blocked" || status === "rejected";
}
