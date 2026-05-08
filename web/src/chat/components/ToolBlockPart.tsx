import { FileText, Terminal, Wrench } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

import { toneForStatus } from "./processBlockTone";

type ToolBlockPartProps = {
  block: AiopsProcessBlock;
  compact?: boolean;
};

export function ToolBlockPart({ block, compact = false }: ToolBlockPartProps) {
  const Icon = block.kind === "command" ? Terminal : block.kind === "file" ? FileText : Wrench;

  return (
    <div className={`overflow-hidden rounded-lg border px-3 py-2 text-sm ${compact ? "shadow-none" : "shadow-sm"} ${toneForStatus(block.status)}`}>
      <div className="flex items-center gap-2">
        <Icon className="h-4 w-4 shrink-0 text-zinc-500" />
        <div className="min-w-0 flex-1 truncate font-medium text-zinc-800">{block.displayKind || block.kind}</div>
        <Badge variant="outline" className="bg-white text-zinc-600">
          {block.status}
        </Badge>
      </div>
      {block.command ? (
        <pre className="mt-2 overflow-x-auto rounded-md bg-zinc-950 px-3 py-2 text-xs leading-5 text-zinc-100">
          <code>{block.command}</code>
        </pre>
      ) : block.text ? (
        <div className="mt-2 break-words leading-6 text-zinc-700">{block.text}</div>
      ) : null}
      {block.outputPreview ? (
        <pre className="mt-2 max-h-36 overflow-auto rounded-md border border-zinc-200 bg-white px-3 py-2 text-xs leading-5 text-zinc-700">
          <code className="break-words whitespace-pre-wrap">{block.outputPreview}</code>
        </pre>
      ) : null}
      <div className="mt-2 flex flex-wrap gap-2 text-xs text-zinc-500">
        {typeof block.exitCode === "number" ? <span>exit {block.exitCode}</span> : null}
        {block.durationMs ? <span>{block.durationMs}ms</span> : null}
        {block.rawRef ? <span>{block.rawRef}</span> : null}
      </div>
    </div>
  );
}
