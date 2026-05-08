import { ChevronDown } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { cn } from "@/lib/utils";
import type { AiopsProcessBlock } from "@/transport/aiopsTransportTypes";

import { MessageMarkdown } from "./MessageMarkdown";

/**
 * Strips HTML content to plain text for display purposes.
 */
export function stripHtml(text: string): string {
  if (!text) return "";
  if (/^\s*<!DOCTYPE|^\s*<html/i.test(text)) {
    const stripped = text.replace(/<[^>]+>/g, " ").replace(/\s+/g, " ").trim();
    if (stripped.length > 200) {
      return `${stripped.slice(0, 200)}…`;
    }
    return stripped;
  }
  return text;
}

type ProcessTranscriptProps = {
  process: AiopsProcessBlock[];
  turnStatus?: string;
  turnStartedAt?: string;
  turnCompletedAt?: string;
  turnUpdatedAt?: string;
  finalText?: string;
  onApprovalDecision?: ApprovalDecisionHandler;
};

type ApprovalDecisionHandler = (approvalId: string, decision: "accept" | "reject") => void;

/**
 * Represents either a single block (reasoning or standalone tool) or a merged group
 * of consecutive same-kind tool blocks.
 */
export type ProcessGroup =
  | { kind: "single"; block: AiopsProcessBlock }
  | { kind: "merged"; blocks: AiopsProcessBlock[]; mergedKind: string };

export function ProcessTranscript({
  process,
  turnStatus,
  turnStartedAt,
  turnCompletedAt,
  turnUpdatedAt,
  finalText,
}: ProcessTranscriptProps) {
  const visibleProcess = useMemo(() => process.filter(shouldShowInTranscript), [process]);
  const running = isProcessRunning(visibleProcess, turnStatus);
  const explicitFinalText = finalText?.trim() || "";
  const finalAssistantIndex = terminalFinalAssistantIndex(visibleProcess, explicitFinalText);
  const finalAssistantText =
    finalAssistantIndex >= 0 ? visibleProcess[finalAssistantIndex]?.text?.trim() || "" : "";
  const processBlocks = visibleProcess.filter((_, index) => index !== finalAssistantIndex);
  const retainedAssistantTexts = new Set(
    processBlocks
      .filter((block) => block.kind === "assistant")
      .map((block) => block.text?.trim() || "")
      .filter(Boolean),
  );
  const renderedFinalText = (
    explicitFinalText && !retainedAssistantTexts.has(explicitFinalText) ? explicitFinalText : finalAssistantText
  ).trim();
  const hasMeaningful = hasMeaningfulContent(processBlocks);
  const shouldRenderProcess = processBlocks.length > 0 || running;

  const fallbackStartRef = useRef(Date.now());
  const [nowMs, setNowMs] = useState(Date.now());
  const [open, setOpen] = useState(running);
  const prevRunningRef = useRef(running);

  useEffect(() => {
    if (!running) {
      return undefined;
    }
    const interval = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [running]);

  useEffect(() => {
    if (prevRunningRef.current && !running) {
      setOpen(false);
    }
    prevRunningRef.current = running;
  }, [running]);

  useEffect(() => {
    if (running) {
      setOpen(true);
    }
  }, [running]);

  if (!shouldRenderProcess && !renderedFinalText) {
    return null;
  }

  const processGroups = groupConsecutiveBlocks(processBlocks);

  const elapsed = elapsedSecondsForTranscript({
    process: processBlocks,
    running,
    startedAt: turnStartedAt,
    completedAt: turnCompletedAt,
    updatedAt: turnUpdatedAt,
    nowMs,
    fallbackStartMs: fallbackStartRef.current,
  });
  const timeLabel = elapsed ? ` ${elapsed}s` : "";

  return (
    <div className="text-[15px] leading-7 text-slate-500" data-testid="aiops-process-transcript" aria-live="polite">
      {shouldRenderProcess ? (
        <>
          <button
            type="button"
            className="group flex w-full items-center gap-1.5 border-b border-slate-200 pb-2 pt-1 text-left"
            aria-expanded={open}
            onClick={() => setOpen((value) => !value)}
            data-testid="aiops-process-header"
          >
            <span className="font-medium tracking-[-0.01em] text-slate-500">
              {running ? `处理中${timeLabel}` : `已处理${timeLabel}`}
            </span>
            <DisclosureChevron open={open} testId="aiops-process-header-chevron" />
          </button>

          {open ? (
            <div className="space-y-3 pb-2 pt-3" data-testid="aiops-process-transcript-body">
              {processGroups.length ? (
                <div className="space-y-2">
                  {processGroups.map((group) => (
                    <ProcessGroupView
                      key={group.kind === "merged" ? group.blocks.map((block) => block.id).join(":") : group.block.id}
                      group={group}
                      turnRunning={running}
                    />
                  ))}
                </div>
              ) : null}
              {/* Bottom status indicator: only when running AND has meaningful content */}
              {running && hasMeaningful ? (
                <InlineStatusIndicator blocks={processBlocks} />
              ) : null}
            </div>
          ) : null}
        </>
      ) : null}
      {renderedFinalText ? <AssistantFinalText text={renderedFinalText} /> : null}
    </div>
  );
}

function terminalFinalAssistantIndex(blocks: AiopsProcessBlock[], finalText?: string) {
  const lastIndex = blocks.length - 1;
  if (lastIndex < 0) {
    return -1;
  }
  return isFinalAssistantBlock(blocks[lastIndex], finalText) ? lastIndex : -1;
}

function isFinalAssistantBlock(block: AiopsProcessBlock, finalText?: string) {
  if (block.kind !== "assistant") {
    return false;
  }
  if (block.displayKind === "assistant.final") {
    return true;
  }
  const blockText = (block.text || "").trim();
  return Boolean(blockText && finalText?.trim() && blockText === finalText.trim());
}

function shouldShowInTranscript(block: AiopsProcessBlock) {
  if (block.kind === "approval") {
    return false;
  }
  const text = (block.text || block.command || block.outputPreview || "").trim().toLowerCase();
  if (!text && !block.steps?.length && !block.queries?.length && !block.results?.length) {
    return false;
  }
  if (block.kind === "reasoning" && (text === "model response received" || text === "calling model")) {
    return false;
  }
  return true;
}

function isSearchLikeBlock(block: AiopsProcessBlock) {
  if (block.kind === "search") {
    return true;
  }
  const display = `${block.displayKind || ""} ${block.text || ""} ${block.command || ""} ${block.inputSummary || ""}`.toLowerCase();
  return /\b(web_search|browse_url|search|browser)\b/.test(display);
}

function isProcessRunning(process: AiopsProcessBlock[], turnStatus?: string) {
  if (turnStatus === "completed" || turnStatus === "failed" || turnStatus === "canceled") {
    return false;
  }
  if (turnStatus === "working" || turnStatus === "submitted" || turnStatus === "blocked") {
    return true;
  }
  return process.some((block) => block.status === "running" || block.status === "queued" || block.status === "blocked");
}

function hasMeaningfulContent(blocks: AiopsProcessBlock[]): boolean {
  return blocks.some((block) => {
    if (block.kind === "reasoning") {
      const text = (block.text || "").trim().toLowerCase();
      return text !== "" && text !== "calling model";
    }
    return true;
  });
}

/**
 * Groups consecutive same-kind tool blocks into merged groups.
 */
export function groupConsecutiveBlocks(blocks: AiopsProcessBlock[]): ProcessGroup[] {
  const groups: ProcessGroup[] = [];
  let i = 0;

  while (i < blocks.length) {
    const block = blocks[i];

    if (block.kind === "reasoning" || !isGroupedProcessBlock(block)) {
      groups.push({ kind: "single", block });
      i += 1;
      continue;
    }

    const consecutive: AiopsProcessBlock[] = [block];
    let j = i + 1;
    while (j < blocks.length && isGroupedProcessBlock(blocks[j])) {
      consecutive.push(blocks[j]);
      j += 1;
    }

    if (consecutive.length >= 2) {
      groups.push({ kind: "merged", blocks: consecutive, mergedKind: mergedKindForBlocks(consecutive) });
    } else {
      groups.push({ kind: "single", block });
    }
    i = j;
  }

  return groups;
}

function groupingKindForBlock(block: AiopsProcessBlock) {
  if (isSearchLikeBlock(block)) {
    return "search";
  }
  return block.kind;
}

function isGroupedProcessBlock(block: AiopsProcessBlock): boolean {
  return isSearchLikeBlock(block) || isToolSummaryBlock(block);
}

function mergedKindForBlocks(blocks: AiopsProcessBlock[]) {
  const kinds = Array.from(new Set(blocks.map(groupingKindForBlock).filter(Boolean)));
  return kinds.length === 1 ? kinds[0] : "mixed";
}

export function getMergedSummaryText(mergedKind: string, count: number): string {
  switch (mergedKind) {
    case "file":
      return `📂 已探索 ${count} 个文件`;
    case "command":
      return count > 1 ? `已运行 ${count} 条命令` : "已运行命令";
    case "search":
      return `网页检索 ${count} 项`;
    case "tool":
    case "mcp":
      return `⚙️ 已调用 ${count} 个工具`;
    default:
  return `⚙️ 已处理 ${count} 个操作`;
  }
}

function DisclosureChevron({ open, testId }: { open: boolean; testId?: string }) {
  return (
    <ChevronDown
      className={cn(
        "h-3.5 w-3.5 shrink-0 text-slate-400 opacity-0 transition-all group-hover:opacity-100 group-focus-visible:opacity-100",
        open ? "rotate-0 opacity-100" : "-rotate-90",
      )}
      data-testid={testId}
      aria-hidden="true"
    />
  );
}

function ProcessGroupView({
  group,
  turnRunning,
}: {
  group: ProcessGroup;
  turnRunning: boolean;
}) {
  const blocks = group.kind === "merged" ? group.blocks : [group.block];
  if (blocks.every(isSearchLikeBlock)) {
    return <SearchTranscriptFromBlocks blocks={blocks} turnRunning={turnRunning} />;
  }
  if (group.kind === "merged") {
    return <MergedToolSummary group={group} />;
  }
  return <NativeProcessText block={group.block} />;
}

function SearchTranscriptFromBlocks({
  blocks,
  turnRunning,
}: {
  blocks: AiopsProcessBlock[];
  turnRunning: boolean;
}) {
  const searchDetails = uniqueLines(blocks.flatMap(searchLines));
  const searchedCount = searchDetails.length || blocks.length;
  const activeSearchQuery = primarySearchQuery(blocks);
  const running = turnRunning && blocks.some(isBlockActive);
  return (
    <SearchTranscript
      query={activeSearchQuery}
      count={searchedCount}
      lines={searchDetails}
      running={running}
    />
  );
}

function isBlockActive(block: AiopsProcessBlock) {
  return block.status === "running" || block.status === "queued" || block.status === "blocked";
}

function MergedToolSummary({
  group,
}: {
  group: Extract<ProcessGroup, { kind: "merged" }>;
}) {
  const text = getMergedGroupSummaryText(group);
  const details = group.blocks.map(mergedBlockDetail).filter((detail) => detail.text);
  const [open, setOpen] = useState(group.mergedKind === "command" || group.blocks.some(isBlockActive));
  if (!details.length) {
    return (
      <div className="truncate text-[15px] leading-7 text-slate-400">
        {text}
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <button
        type="button"
        className="group flex w-full min-w-0 items-center gap-1.5 text-left text-[15px] leading-7 text-slate-400 transition-colors hover:text-slate-600"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
      >
        <span className="min-w-0 truncate">{text}</span>
        <DisclosureChevron open={open} testId={`aiops-merged-${group.mergedKind}-chevron`} />
      </button>
      {open ? (
        <div
          className="space-y-2 overflow-visible pl-5 text-[13px] leading-6 text-slate-500"
          data-testid={`aiops-merged-${group.mergedKind}-details`}
        >
          {details.map((detail, index) =>
            detail.kind === "command" ? (
              <CommandDetailRow key={`${detail.id}:${index}`} detail={detail} />
            ) : (
              <ToolDetailRow key={`${detail.id}:${index}`} detail={detail} />
            ),
          )}
        </div>
      ) : null}
    </div>
  );
}

function ToolDetailRow({
  detail,
}: {
  detail: ReturnType<typeof mergedBlockDetail>;
}) {
  const hasOutput = Boolean(detail.output);
  const toolRunning = detail.status === "running" || detail.status === "queued";
  const [open, setOpen] = useState(toolRunning);
  useEffect(() => {
    if (!toolRunning) {
      setOpen(false);
    }
  }, [toolRunning, detail.id]);
  return (
    <div className="space-y-2">
      <button
        type="button"
        className="group flex w-full min-w-0 items-center gap-1.5 text-left text-[15px] leading-7 text-slate-400 transition-colors hover:text-slate-600"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        data-testid={`aiops-tool-row-${detail.id}`}
      >
        <span className="min-w-0 flex-1 truncate">{toolDetailSummaryLabel(detail)}</span>
        <span className="shrink-0 text-[13px] text-slate-400">{statusLabel(detail.status)}</span>
        <DisclosureChevron open={open} testId={`aiops-tool-chevron-${detail.id}`} />
      </button>
      {open && hasOutput ? (
        <div
          className="rounded-lg bg-slate-100 px-3 py-2 font-mono text-[13px] leading-6 text-slate-500"
          data-testid={`aiops-tool-output-${detail.id}`}
        >
          {detail.output}
        </div>
      ) : null}
    </div>
  );
}

function CommandDetailRow({
  detail,
}: {
  detail: ReturnType<typeof mergedBlockDetail>;
}) {
  const hasOutput = Boolean(detail.output);
  const commandRunning = detail.status === "running" || detail.status === "queued";
  const rowStatus = commandRowStatusLabel(detail.status);
  const [open, setOpen] = useState(commandRunning);
  useEffect(() => {
    if (!commandRunning) {
      setOpen(false);
    }
  }, [commandRunning, detail.id]);
  return (
    <div className="space-y-2">
      <button
        type="button"
        className="group flex w-full min-w-0 items-center gap-1.5 text-left text-[15px] leading-7 text-slate-400 transition-colors hover:text-slate-600"
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        data-testid={`aiops-command-row-${detail.id}`}
      >
        <span
          className="flex min-w-0 flex-1 items-center gap-1.5"
          data-testid={`aiops-command-label-region-${detail.id}`}
        >
          <span className="min-w-0 truncate">{commandSummaryLabel(detail)}</span>
          <DisclosureChevron open={open} testId={`aiops-command-chevron-${detail.id}`} />
        </span>
        {rowStatus ? (
          <span className="shrink-0 text-[13px] text-slate-400" data-testid={`aiops-command-status-${detail.id}`}>
            {rowStatus}
          </span>
        ) : null}
      </button>
      {open ? (
        <div
          className="rounded-lg bg-slate-100 px-3 py-2 text-slate-500"
          data-testid={`aiops-terminal-card-${detail.id}`}
        >
          <div className="text-[13px] leading-5 text-slate-500">Shell</div>
          <div className="mt-2 whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-slate-950">
            $ {detail.text}
          </div>
          {hasOutput ? (
            <pre
              className="mt-3 max-h-48 overflow-auto rounded-md bg-slate-100 font-mono text-[13px] leading-6 text-slate-500"
              data-testid={`aiops-command-output-${detail.id}`}
            >
              {detail.output}
            </pre>
          ) : null}
          <div className="mt-2 flex justify-end text-[13px] leading-5 text-slate-500">
            {terminalStatusLabel(detail.status)}
          </div>
        </div>
      ) : null}
    </div>
  );
}

function commandRowStatusLabel(status?: AiopsProcessBlock["status"]) {
  if (status === "completed") {
    return "";
  }
  return terminalStatusLabel(status);
}

function getMergedGroupSummaryText(group: Extract<ProcessGroup, { kind: "merged" }>) {
  if (group.mergedKind === "mixed") {
    return getMixedMergedSummaryText(group.blocks);
  }
  if (group.mergedKind === "command") {
    return getMergedSummaryText(group.mergedKind, group.blocks.length);
  }
  return getMergedSummaryText(group.mergedKind, group.blocks.length);
}

function getMixedMergedSummaryText(blocks: AiopsProcessBlock[]) {
  const counts = blocks.reduce(
    (acc, block) => {
      const kind = groupingKindForBlock(block);
      if (kind === "file") acc.file += 1;
      else if (kind === "search") acc.search += 1;
      else if (kind === "command") acc.command += 1;
      else acc.tool += 1;
      return acc;
    },
    { file: 0, search: 0, command: 0, tool: 0 },
  );
  return [
    counts.file ? `已探索 ${counts.file} 个文件` : "",
    counts.search ? `${counts.search} 次搜索` : "",
    counts.command ? `已运行 ${counts.command} 条命令` : "",
    counts.tool ? `已调用 ${counts.tool} 个工具` : "",
  ].filter(Boolean).join(",");
}

function mergedBlockDetail(block: AiopsProcessBlock) {
  let text = "";
  if (isSearchLikeBlock(block)) {
    text = searchLines(block)[0] || searchQueryForBlock(block) || "搜索网页";
  } else if (block.kind === "command") {
    text = block.command || block.inputSummary || stripHtml(block.text || "");
  } else if (block.kind === "file") {
    text = stripHtml(block.text || "") || block.inputSummary || block.displayKind || "";
  } else {
    text = stripHtml(block.text || "") || block.command || block.inputSummary || block.displayKind || "";
  }
  return {
    id: block.id,
    kind: groupingKindForBlock(block),
    status: block.status,
    approvalId: block.approvalId,
    text: block.kind === "command" ? stripHtml(text).trim() : cleanToolText(text),
    output: block.kind === "command" || block.kind === "tool" || block.kind === "mcp"
      ? cleanCommandOutput(block.outputPreview)
      : "",
  };
}

function cleanCommandOutput(value?: string) {
  return stripHtml(value || "").trim();
}

function statusLabel(status?: string) {
  switch (status) {
    case "blocked":
      return "等待审核";
    case "failed":
      return "失败";
    case "running":
      return "执行中";
    case "queued":
      return "排队中";
    case "rejected":
      return "已拒绝";
    default:
      return "已完成";
  }
}

function statusBadgeClass(status?: string) {
  switch (status) {
    case "blocked":
      return "bg-amber-50 text-amber-700";
    case "failed":
    case "rejected":
      return "bg-red-50 text-red-700";
    case "running":
    case "queued":
      return "bg-blue-50 text-blue-700";
    default:
      return "bg-slate-100 text-slate-500";
  }
}

function terminalStatusLabel(status?: string) {
  switch (status) {
    case "blocked":
      return "等待审核";
    case "failed":
      return "失败";
    case "running":
      return "正在运行";
    case "queued":
      return "排队中";
    case "rejected":
      return "已拒绝";
    default:
      return "✓ 成功";
  }
}

function commandSummaryLabel(detail: ReturnType<typeof mergedBlockDetail>) {
  const command = detail.text || "命令";
  switch (detail.status) {
    case "blocked":
      return `等待审核 ${command}`;
    case "failed":
      return `运行失败 ${command}`;
    case "running":
      return `正在运行 ${command}`;
    case "queued":
      return `排队中 ${command}`;
    case "rejected":
      return `已拒绝 ${command}`;
    default:
      return `已运行 ${command}`;
  }
}

function toolDetailSummaryLabel(detail: ReturnType<typeof mergedBlockDetail>) {
  const text = detail.text || "工具调用";
  switch (detail.status) {
    case "blocked":
      return `等待审核 ${text}`;
    case "failed":
      return `执行失败 ${text}`;
    case "running":
      return `正在执行 ${text}`;
    case "queued":
      return `排队中 ${text}`;
    case "rejected":
      return `已拒绝 ${text}`;
    default:
      return text;
  }
}

function NativeProcessText({
  block,
}: {
  block: AiopsProcessBlock;
}) {
  if (block.kind === "assistant") {
    return <AssistantFinalText text={block.text} />;
  }
  if (block.kind === "reasoning") {
    return <ThinkingText block={block} />;
  }
  if (block.kind === "command") {
    return <CommandDetailRow detail={mergedBlockDetail(block)} />;
  }
  if (isToolSummaryBlock(block)) {
    return <ToolDetailRow detail={mergedBlockDetail(block)} />;
  }
  const text = readableBlockSummary(block);
  if (!text) {
    return null;
  }
  return (
    <div className="whitespace-pre-wrap break-words text-[15px] font-medium leading-7 tracking-[-0.01em] text-slate-900">
      {text}
    </div>
  );
}

function AssistantFinalText({ text }: { text: string }) {
  return (
    <div className="max-w-none px-1 py-1 text-[15px] leading-7 text-slate-900" data-testid="aiops-final-text">
      <MessageMarkdown text={text} />
    </div>
  );
}

function isToolSummaryBlock(block: AiopsProcessBlock): boolean {
  return block.kind === "command" || block.kind === "tool" || block.kind === "file" || block.kind === "mcp";
}

function getToolIcon(block: AiopsProcessBlock): string {
  if (block.kind === "search") return "🔍";
  if (block.kind === "command") return "⚙️";
  if (block.kind === "file") {
    const text = `${block.displayKind || ""} ${block.text || ""} ${block.inputSummary || ""}`.toLowerCase();
    if (/edit|write|create|modify|update/.test(text)) return "✏️";
    return "📂";
  }
  return "⚙️";
}

function toolSummaryText(block: AiopsProcessBlock): string {
  const isRunning = block.status === "running" || block.status === "queued";

  if (block.kind === "command") {
    const cmd = block.command || stripHtml(block.text || "") || "命令";
    return isRunning ? `正在执行 ${cmd}` : cmd;
  }
  if (block.kind === "file") {
    const text = stripHtml(block.text || "") || block.inputSummary || block.displayKind || "";
    const cleaned = cleanToolText(text) || "文件操作";
    return isRunning ? `正在处理 ${cleaned}` : cleaned;
  }
  if (block.kind === "tool" || block.kind === "mcp") {
    const text = stripHtml(block.text || "") || block.displayKind || "";
    const cleaned = cleanToolText(text) || "工具调用";
    return isRunning ? `正在调用 ${cleaned}` : cleaned;
  }
  const text = cleanToolText(stripHtml(block.text || "") || block.displayKind || block.kind);
  return isRunning ? `正在${text}` : text;
}

function ToolSummaryLine({ block }: { block: AiopsProcessBlock }) {
  const icon = getToolIcon(block);
  const summary = toolSummaryText(block);
  if (!summary) return null;

  const full = `${icon} ${summary}`;
  const display = full.length > 80 ? `${full.slice(0, 79)}…` : full;

  return (
    <div
      className="truncate text-[15px] leading-7 text-slate-400"
      title={full.length > 80 ? full : undefined}
    >
      {display}
    </div>
  );
}

function ThinkingText({ block }: { block: AiopsProcessBlock }) {
  const raw = stripHtml(block.text || "");
  const text = transformThinkingText(raw);
  if (!text || !text.trim()) {
    return null;
  }
  return (
    <div className="whitespace-pre-wrap break-words text-[15px] font-medium leading-7 tracking-[-0.01em] text-slate-900">
      {text}
    </div>
  );
}

function transformThinkingText(raw: string): string {
  const trimmed = raw.trim();
  if (!trimmed) {
    return "";
  }
  if (trimmed.toLowerCase() === "calling model") {
    return "";
  }
  const prefixPattern = /^calling model\s*/i;
  if (prefixPattern.test(trimmed)) {
    return trimmed.replace(prefixPattern, "");
  }
  return trimmed;
}

/**
 * Inline status indicator at the bottom of the timeline while running.
 */
function InlineStatusIndicator({ blocks }: { blocks: AiopsProcessBlock[] }) {
  const lastBlock = blocks.length > 0 ? blocks[blocks.length - 1] : undefined;
  let label: string;

  if (!lastBlock || lastBlock.kind === "reasoning") {
    label = "正在思考";
  } else if (
    (lastBlock.kind === "tool" ||
      lastBlock.kind === "command" ||
      lastBlock.kind === "file" ||
      lastBlock.kind === "search" ||
      lastBlock.kind === "mcp") &&
    (lastBlock.status === "running" || lastBlock.status === "queued")
  ) {
    label = "正在执行";
  } else {
    label = "正在思考";
  }

  return (
    <div className="flex items-center gap-1.5 text-xs text-slate-400" data-testid="aiops-inline-status-indicator">
      <span className="h-1.5 w-1.5 rounded-full bg-slate-300" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

/**
 * Search transcript: collapsible with max-height constraint.
 * Shows web search references with expandable details.
 */
function SearchTranscript({
  query,
  count,
  lines,
  running,
}: {
  query: string;
  count: number;
  lines: string[];
  running: boolean;
}) {
  const [open, setOpen] = useState(running);

  // Auto-collapse when search completes
  useEffect(() => {
    if (!running) {
      setOpen(false);
    }
  }, [running]);

  return (
    <div className="space-y-1">
      <button
        type="button"
        className="group flex items-center gap-1.5 text-left text-[15px] leading-7 text-slate-400 transition-colors hover:text-slate-600"
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        data-testid="aiops-search-toggle"
      >
        <span>{searchTranscriptLabel(running, count, query)}</span>
        <DisclosureChevron open={open} testId="aiops-search-chevron" />
      </button>
      {open && lines.length ? (
        <div className="space-y-1 text-[15px] leading-7 text-slate-400" data-testid="aiops-search-details">
          {lines.map((line, index) => (
            <div key={`${line}:${index}`} className="whitespace-normal break-all">
              {line}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function readableBlockSummary(block?: AiopsProcessBlock) {
  if (!block) {
    return "";
  }
  if (block.kind === "search") {
    const query = block.queries?.[0] || stripHtml(block.text || "");
    return query ? `正在搜索：${query}` : "正在搜索网页";
  }
  if (block.kind === "command") {
    return block.command ? `正在执行：${block.command}` : "正在执行命令";
  }
  if (block.kind === "tool") {
    const text = cleanToolText(stripHtml(block.text || "") || block.displayKind || "");
    return text || "正在调用工具";
  }
  return cleanToolText(stripHtml(block.text || "") || block.displayKind || block.kind);
}

function searchLines(block: AiopsProcessBlock) {
  const query = searchQueryForBlock(block);
  const lines = query ? [query] : [];
  for (const result of block.results || []) {
    const url = extractUrl(result.url);
    if (url) {
      lines.push(url);
    }
  }
  const browsedUrl = browseUrlForBlock(block);
  if (browsedUrl) {
    lines.push(browsedUrl);
  }
  return uniqueLines(lines).slice(0, 8);
}

function primarySearchQuery(blocks: AiopsProcessBlock[]) {
  for (const block of blocks) {
    const query = searchQueryForBlock(block);
    if (query && !isGenericSearchLabel(query)) {
      return query;
    }
  }
  return "";
}

function searchTranscriptLabel(running: boolean, count: number, query: string) {
  if (running) {
    return query ? `正在搜索网页（${query}）` : "正在搜索网页";
  }
  return `网页检索 ${count} 项`;
}

function uniqueLines(lines: string[]) {
  const seen = new Set<string>();
  const result: string[] = [];
  for (const line of lines) {
    const text = cleanSearchLine(line);
    const key = text.toLowerCase();
    if (!text || seen.has(key)) {
      continue;
    }
    seen.add(key);
    result.push(text);
  }
  return result;
}

function firstCleanLine(...values: Array<string | undefined>) {
  for (const value of values) {
    const line = cleanSearchLine(value);
    if (line) return line;
  }
  return "";
}

function searchQueryForBlock(block: AiopsProcessBlock) {
  if (!isSearchActionBlock(block)) {
    return "";
  }
  const query = firstCleanLine(
    block.queries?.[0],
    extractPayloadField(block.inputSummary, ["query", "search_query", "q"]),
    block.inputSummary,
    extractPayloadField(block.command, ["query", "search_query", "q"]),
  );
  return isGenericSearchLabel(query) ? "" : query;
}

function browseUrlForBlock(block: AiopsProcessBlock) {
  if (!isBrowseActionBlock(block) && !hasUrlLikeSummary(block)) {
    return "";
  }
  return firstCleanLine(
    extractUrl(block.inputSummary),
    extractPayloadField(block.outputPreview, ["url", "href", "link"]),
    extractUrl(block.outputPreview),
    extractUrl(block.command),
  );
}

function isSearchActionBlock(block: AiopsProcessBlock) {
  if (block.kind === "search") {
    return true;
  }
  const display = `${block.displayKind || ""} ${block.command || ""}`.toLowerCase();
  return /\b(web_search|search_web|browser\.search|search)\b/.test(display);
}

function isBrowseActionBlock(block: AiopsProcessBlock) {
  const display = `${block.displayKind || ""} ${block.command || ""}`.toLowerCase();
  return /\b(browse_url|browser\.open|open_url|open_page|fetch_url|browser)\b/.test(display);
}

function hasUrlLikeSummary(block: AiopsProcessBlock) {
  return Boolean(
    extractUrl(block.inputSummary) ||
      extractPayloadField(block.outputPreview, ["url", "href", "link"]) ||
      extractUrl(block.outputPreview),
  );
}

function cleanSearchLine(value?: string) {
  let text = unwrapProviderPayload(value || "");
  text = decodeHtmlish(text)
    .replace(/\b(browse_url|browser|search)\b/gi, "")
    .replace(/provider-native\s+/gi, "")
    .replace(/web_search\s+completed\s+for\s+query\s*/gi, "")
    .replace(/web_search/gi, "")
    .replace(/Do not repeat this exact query.*$/gi, "")
    .replace(/provider returned no textual summary.*$/gi, "")
    .replace(/\s*;\s*$/g, "")
    .replace(/\s+/g, " ")
    .trim();
  text = text.replace(/^["'""]+|["'""]+$/g, "").trim();
  const normalized = text.toLowerCase();
  if (
    !text ||
    normalized === "completed" ||
    normalized === "search" ||
    normalized === "browse_url" ||
    normalized === "browser" ||
    isInternalSearchLine(text) ||
    isRawPayloadLine(text)
  ) {
    return "";
  }
  return text.length > 180 ? `${text.slice(0, 180).trim()}…` : text;
}

function isGenericSearchLabel(value: string) {
  const normalized = value.trim().toLowerCase();
  return normalized === "searching the web" || normalized === "正在搜索网页" || normalized === "搜索网页";
}

function cleanToolText(value: string) {
  const text = cleanSearchLine(value);
  if (/^(browse_url|browser|web_search|search)$/i.test(text)) {
    return "";
  }
  return text;
}

function unwrapProviderPayload(value: string) {
  let text = value.trim();
  for (let attempt = 0; attempt < 2; attempt += 1) {
    const parsed = parseJsonLike(text);
    if (!parsed) break;
    text = extractTextFromPayload(parsed) || text;
  }
  return text;
}

function extractUrl(value?: string) {
  const text = decodeJsonishText(value || "").trim();
  if (!text) return "";
  const directMatch = text.match(/https?:\/\/[^\s"'<>)}\]]+/i);
  if (directMatch) return cleanExtractedUrl(directMatch[0]);
  return extractPayloadField(text, ["url", "href", "link"]);
}

function extractPayloadField(value: string | undefined, keys: string[]) {
  const raw = (value || "").trim();
  const decoded = decodeJsonishText(raw);
  const parsed = parseJsonLike(raw) || parseJsonLike(decoded);
  const found = findPayloadField(parsed, keys);
  if (found) {
    return cleanExtractedUrl(found);
  }
  const text = decoded || raw;
  for (const key of keys) {
    const escapedPattern = new RegExp(`["']${escapeRegExp(key)}["']\\s*:\\s*["']([^"']+)`, "i");
    const escaped = text.match(escapedPattern);
    if (escaped?.[1]) {
      return cleanExtractedUrl(escaped[1]);
    }
  }
  return "";
}

function findPayloadField(value: unknown, keys: string[]): string {
  if (!value || typeof value !== "object") {
    return "";
  }
  if (Array.isArray(value)) {
    for (const item of value) {
      const found = findPayloadField(item, keys);
      if (found) return found;
    }
    return "";
  }
  const record = value as Record<string, unknown>;
  for (const key of keys) {
    const candidate = record[key];
    if (typeof candidate === "string" && candidate.trim()) {
      return candidate.trim();
    }
  }
  for (const candidate of Object.values(record)) {
    const found = findPayloadField(candidate, keys);
    if (found) return found;
  }
  return "";
}

function isInternalSearchLine(value: string) {
  return /(?:127\.0\.0\.1|localhost):\d+\/v1\/(?:responses|chat\/completions)/i.test(value) ||
    /context deadline exceeded|client\.timeout|failed:\s*post\s+["']?https?:\/\//i.test(value);
}

function isRawPayloadLine(value: string) {
  const trimmed = value.trim();
  if (/^[\[{]/.test(trimmed)) {
    return true;
  }
  return /\bcontentType\b|\bapplication\/json\b|\\"[A-Za-z0-9_-]+"\s*:/.test(trimmed);
}

function escapeRegExp(value: string) {
  return value.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

function decodeJsonishText(value: string) {
  return value
    .replace(/\\"/g, '"')
    .replace(/\\\//g, "/")
    .replace(/\\\\u0026/g, "&")
    .replace(/\\u0026/g, "&");
}

function cleanExtractedUrl(value: string) {
  return decodeJsonishText(value).replace(/\\+$/g, "").trim();
}

function parseJsonLike(value: string): unknown {
  if (!value || !/^[\[{"]/.test(value.trim())) {
    return null;
  }
  try {
    return JSON.parse(value);
  } catch {
    return null;
  }
}

function extractTextFromPayload(value: unknown): string {
  if (typeof value === "string") return value;
  if (!value || typeof value !== "object") return "";
  if (Array.isArray(value)) {
    return value.map(extractTextFromPayload).filter(Boolean).join(" ");
  }
  const record = value as Record<string, unknown>;
  for (const key of ["query", "title", "url", "snippet", "summary", "content", "text"]) {
    const text = extractTextFromPayload(record[key]);
    if (text) return text;
  }
  return "";
}

function decodeHtmlish(value: string) {
  return value
    .replace(/\\u003c/gi, "<")
    .replace(/\\u003e/gi, ">")
    .replace(/\\u0026/gi, "&")
    .replace(/<a\b[^>]*href=["']([^"']+)["'][^>]*>(.*?)<\/a>/gi, "$2 $1")
    .replace(/<[^>]+>/g, " ")
    .replace(/&quot;/g, '"')
    .replace(/&#39;/g, "'")
    .replace(/&amp;/g, "&")
    .replace(/&lt;/g, "<")
    .replace(/&gt;/g, ">");
}

function elapsedSecondsForTranscript({
  process,
  running,
  startedAt,
  completedAt,
  updatedAt,
  nowMs,
  fallbackStartMs,
}: {
  process: AiopsProcessBlock[];
  running: boolean;
  startedAt?: string;
  completedAt?: string;
  updatedAt?: string;
  nowMs: number;
  fallbackStartMs: number;
}) {
  const startMs = parseTimestampMs(startedAt);
  if (running) {
    const base = Number.isFinite(startMs) ? startMs : fallbackStartMs;
    return Math.max(1, Math.round((nowMs - base) / 1000));
  }
  const endMs = firstFiniteNumber(parseTimestampMs(completedAt), parseTimestampMs(updatedAt));
  if (Number.isFinite(startMs) && Number.isFinite(endMs) && endMs >= startMs) {
    return Math.max(1, Math.round((endMs - startMs) / 1000));
  }
  return estimateElapsedSeconds(process);
}

function estimateElapsedSeconds(process: AiopsProcessBlock[]) {
  const times = process
    .map((block) => Date.parse(block.updatedAt || ""))
    .filter((value) => Number.isFinite(value));
  if (times.length < 2) {
    return 0;
  }
  return Math.max(1, Math.round((Math.max(...times) - Math.min(...times)) / 1000));
}

function parseTimestampMs(value?: string) {
  if (!value) {
    return Number.NaN;
  }
  return Date.parse(value);
}

function firstFiniteNumber(...values: number[]) {
  return values.find((value) => Number.isFinite(value)) ?? Number.NaN;
}
