import { useEffect, useMemo, useState } from "react";

import { TranscriptAggregateBlock } from "./TranscriptAggregateBlock";
import { TranscriptApprovalBlock } from "./TranscriptApprovalBlock";
import { TranscriptTextBlock } from "./TranscriptTextBlock";
import { TranscriptThinkingBlock } from "./TranscriptThinkingBlock";
import { TranscriptToolBlock } from "./TranscriptToolBlock";

export type AiopsTranscriptBlockType = "text" | "tool" | "aggregate" | "approval" | "thinking" | "artifact";
export type AiopsTranscriptToolKind = "command" | "search" | "file" | "mcp" | "browser" | "list" | "other";
export type AiopsTranscriptToolStatus = "queued" | "running" | "completed" | "failed" | "blocked" | "rejected";

export type AiopsTranscriptBlock = {
  id: string;
  type: AiopsTranscriptBlockType;
  text?: {
    role?: string;
    text: string;
    status?: "streaming" | "completed" | string;
  };
  tool?: {
    toolKind: AiopsTranscriptToolKind;
    toolName?: string;
    title?: string;
    summary?: string;
    status: AiopsTranscriptToolStatus;
    command?: string;
    inputSummary?: string;
    output?: {
      stdout?: string;
      stderr?: string;
      text?: string;
      truncated?: boolean;
      rawRef?: string;
    };
    exitCode?: number;
    durationMs?: number;
    startedAt?: string;
    completedAt?: string;
    approvalId?: string;
  };
  aggregate?: {
    summary: string;
    status?: string;
    childBlockIds?: string[];
    counts?: Record<string, number | undefined>;
  };
  approval?: {
    approvalId: string;
    approvalKind?: string;
    title?: string;
    summary?: string;
    command?: string;
    status?: string;
    requestedAt?: string;
    resolvedAt?: string;
  };
  thinking?: {
    text?: string;
    status?: string;
  };
  artifact?: {
    artifactId?: string;
    kind?: string;
    title?: string;
    summary?: string;
  };
  createdAt?: string;
  updatedAt?: string;
};

export type AiopsTranscriptProps = {
  blocks?: AiopsTranscriptBlock[];
  blockOrder?: string[];
  blocksById?: Record<string, AiopsTranscriptBlock | undefined>;
  turnStatus?: string;
  turnStartedAt?: string;
  turnCompletedAt?: string;
  turnUpdatedAt?: string;
};

export function AiopsTranscript({
  blocks,
  blockOrder,
  blocksById,
  turnStatus,
  turnStartedAt,
  turnCompletedAt,
  turnUpdatedAt,
}: AiopsTranscriptProps) {
  const orderedBlocks = useMemo(
    () => orderedTranscriptBlocks({ blocks, blockOrder, blocksById }),
    [blocks, blockOrder, blocksById],
  );
  const running = isPendingTurn(turnStatus) || orderedBlocks.some(isRunningBlock);

  if (orderedBlocks.length === 0 && !running) {
    return null;
  }

  return (
    <div data-testid="aiops-transcript" className="space-y-3 pb-2 pt-1 text-[15px] leading-7">
      <TranscriptStatusLine
        running={running}
        startedAt={turnStartedAt}
        completedAt={turnCompletedAt}
        updatedAt={turnUpdatedAt}
      />
      {orderedBlocks.map((block) => {
        if (block.type === "text" && block.text) return <TranscriptTextBlock key={block.id} block={block} />;
        if (block.type === "tool" && block.tool) return <TranscriptToolBlock key={block.id} block={block} />;
        if (block.type === "aggregate" && block.aggregate) {
          return <TranscriptAggregateBlock key={block.id} block={block} blocksById={blocksById || blocksByIdFromBlocks(blocks)} />;
        }
        if (block.type === "approval" && block.approval) return <TranscriptApprovalBlock key={block.id} block={block} />;
        if (block.type === "thinking" && block.thinking) return <TranscriptThinkingBlock key={block.id} block={block} />;
        if (block.type === "artifact" && block.artifact) return <ArtifactBlock key={block.id} block={block} />;
        return null;
      })}
    </div>
  );
}

export function orderedTranscriptBlocks({
  blocks,
  blockOrder,
  blocksById,
}: Pick<AiopsTranscriptProps, "blocks" | "blockOrder" | "blocksById">) {
  if (blockOrder && blocksById) {
    return blockOrder.map((id) => blocksById[id]).filter(isTranscriptBlock);
  }
  return (blocks || []).filter(isTranscriptBlock);
}

function TranscriptStatusLine({
  running,
  startedAt,
  completedAt,
  updatedAt,
}: {
  running: boolean;
  startedAt?: string;
  completedAt?: string;
  updatedAt?: string;
}) {
  const [nowMs, setNowMs] = useState(Date.now());

  useEffect(() => {
    if (!running) {
      return undefined;
    }
    const interval = window.setInterval(() => setNowMs(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, [running]);

  const elapsed = elapsedSeconds({ running, startedAt, completedAt, updatedAt, nowMs });
  return (
    <div data-testid="aiops-transcript-status" className="px-1 text-[13px] leading-5 text-slate-400">
      {running ? "处理中" : "已处理"}
      {elapsed ? ` ${elapsed}s` : ""}
    </div>
  );
}

function ArtifactBlock({ block }: { block: AiopsTranscriptBlock }) {
  const label = block.artifact?.summary || block.artifact?.title || "已生成 artifact";
  return <div className="px-1 text-[14px] leading-6 text-slate-400">{label}</div>;
}

function elapsedSeconds({
  running,
  startedAt,
  completedAt,
  updatedAt,
  nowMs,
}: {
  running: boolean;
  startedAt?: string;
  completedAt?: string;
  updatedAt?: string;
  nowMs: number;
}) {
  const startMs = parseTime(startedAt);
  if (!startMs) {
    return 0;
  }
  const endMs = running ? nowMs : parseTime(completedAt || updatedAt) || nowMs;
  return Math.max(0, Math.round((endMs - startMs) / 1000));
}

function blocksByIdFromBlocks(blocks?: AiopsTranscriptBlock[]) {
  return Object.fromEntries((blocks || []).map((block) => [block.id, block]));
}

function isPendingTurn(status?: string) {
  return status === "submitted" || status === "working" || status === "blocked";
}

function isRunningBlock(block: AiopsTranscriptBlock) {
  return block.tool?.status === "queued" || block.tool?.status === "running" || block.tool?.status === "blocked";
}

function isTranscriptBlock(block: AiopsTranscriptBlock | undefined | null): block is AiopsTranscriptBlock {
  return Boolean(block?.id && block.type);
}

function parseTime(value?: string) {
  if (!value) {
    return 0;
  }
  const millis = new Date(value).getTime();
  return Number.isFinite(millis) ? millis : 0;
}
