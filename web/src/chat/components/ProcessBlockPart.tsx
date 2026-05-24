import {
  Database,
  FileText,
  Lightbulb,
  ListChecks,
  Search,
  Settings2,
  ShieldAlert,
  Terminal,
  Wrench,
} from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { cn } from "@/lib/utils";
import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

import { ApprovalBlockPart } from "./ApprovalBlockPart";
import { toneForStatus } from "./processBlockTone";
import { ToolBlockPart } from "./ToolBlockPart";

type ProcessBlockPartProps = {
  block: AiopsProcessBlock;
  compact?: boolean;
};

export function ProcessBlockPart({ block, compact = false }: ProcessBlockPartProps) {
  if (block.kind === "approval") {
    return <ApprovalBlockPart block={block} />;
  }
  if (block.kind === "command" || block.kind === "tool" || block.kind === "file") {
    return <ToolBlockPart block={block} compact={compact} />;
  }

  const Icon = iconForKind(block.kind);
  return (
    <div className={cn("overflow-hidden rounded-lg border bg-white text-sm shadow-sm", compact ? "px-3 py-2 shadow-none" : "px-3 py-2", toneForStatus(block.status))}>
      <div className="flex items-center gap-2">
        <Icon className="h-4 w-4 shrink-0 text-zinc-500" />
        <div className="min-w-0 flex-1 truncate font-medium text-zinc-800">{labelForBlock(block)}</div>
        {block.mock ? (
          <Badge variant="outline" className="bg-amber-50 text-amber-700">
            Mock
          </Badge>
        ) : null}
        <Badge variant="outline" className="bg-white text-zinc-600">
          {block.status}
        </Badge>
      </div>
      {block.text ? <div className="mt-2 break-words leading-6 text-zinc-700">{block.text}</div> : null}
      {block.steps?.length ? (
        <ol className="mt-2 space-y-1">
          {block.steps.map((step) => (
            <li key={step.id || step.text} className="flex gap-2 text-xs text-zinc-600">
              <span className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-zinc-400" />
              <span className="min-w-0">
                <span className="break-words font-medium text-zinc-700">{step.text}</span>
                {step.summary ? <span className="ml-1 text-zinc-500">{step.summary}</span> : null}
              </span>
            </li>
          ))}
        </ol>
      ) : null}
      {block.queries?.length ? (
        <div className="mt-2 flex flex-wrap gap-1 overflow-hidden">
          {uniqueQueries(block.queries).map((query) => (
            <Badge key={query} variant="secondary" className="max-w-full break-all bg-zinc-100 text-zinc-700 whitespace-normal">
              {query}
            </Badge>
          ))}
        </div>
      ) : null}
      {block.results?.length ? (
        <div className="mt-2 space-y-1">
          {block.results.map((result) => (
            <div key={`${result.title}:${result.url}`} className="overflow-hidden rounded-md bg-zinc-50 px-2 py-1 text-xs text-zinc-600">
              <span className="break-words font-medium text-zinc-700">{result.title || result.url}</span>
              {result.snippet ? <span className="ml-1">{result.snippet}</span> : null}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function iconForKind(kind: AiopsProcessBlock["kind"]) {
  switch (kind) {
    case "plan":
      return ListChecks;
    case "reasoning":
      return Lightbulb;
    case "search":
      return Search;
    case "evidence":
      return Database;
    case "mcp":
      return Settings2;
    case "system":
      return ShieldAlert;
    case "file":
      return FileText;
    case "command":
      return Terminal;
    default:
      return Wrench;
  }
}

function labelForBlock(block: AiopsProcessBlock) {
  const displayKind = (block.displayKind || "").toLowerCase();
  if (displayKind === "context.compaction" || displayKind === "context.compaction.started") {
    return "上下文压缩";
  }
  if (displayKind === "context.microcompact" || displayKind === "context.microcompact.completed") {
    return "上下文整理";
  }
  return block.displayKind || block.kind;
}

function uniqueQueries(queries: string[]) {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const query of queries) {
    const normalized = query.trim();
    if (!normalized || seen.has(normalized)) {
      continue;
    }
    seen.add(normalized);
    result.push(normalized);
  }
  return result;
}
