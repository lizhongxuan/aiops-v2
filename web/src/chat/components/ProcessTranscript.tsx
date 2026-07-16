import { ChevronDown, FileSearch, ListChecks, Search, SquareTerminal, Wrench, type LucideIcon } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { cn } from "@/lib/utils";
import type { AgentStepView, AiopsProcessBlock, AiopsTransportProcessKind, AiopsTransportProcessStatus } from "@/transport/aiopsTransportTypes";

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

function debugTranscriptProjection(event: string, payload: Record<string, unknown>) {
  if (!isTranscriptDebugEnabled()) return;
  // Keep customer content out of browser diagnostics; callers pass hashes and lengths.
  console.debug("aiops.transcript_projection", event, payload);
}

function isTranscriptDebugEnabled() {
  if (typeof window === "undefined") return false;
  try {
    const params = new URLSearchParams(window.location.search || "");
    return params.get("debugTranscript") === "1" || window.localStorage?.getItem("aiops.debugTranscript") === "1";
  } catch {
    return false;
  }
}

function debugProcessBlock(block: AiopsProcessBlock, index: number) {
  return {
    index,
    id: block.id,
    kind: block.kind,
    displayKind: block.displayKind,
    status: block.status,
    phase: block.phase || "",
    streamState: block.streamState || "",
    textChars: (block.text || "").trim().length,
    textHash: debugTextHash(block.text || ""),
  };
}

function debugTextHash(text: string) {
  const normalized = (text || "").trim();
  if (!normalized) return "";
  let hash = 0x811c9dc5;
  for (let index = 0; index < normalized.length; index += 1) {
    hash ^= normalized.charCodeAt(index);
    hash = Math.imul(hash, 0x01000193);
  }
  return (hash >>> 0).toString(16).padStart(8, "0");
}

function debugTextFacts(text: string) {
  const normalized = (text || "").trim();
  return {
    chars: normalized.length,
    hash: debugTextHash(normalized),
  };
}

type ProcessTranscriptProps = {
  process: AiopsProcessBlock[];
  agentSteps?: AgentStepView[];
  turnStatus?: string;
  turnStartedAt?: string;
  turnCompletedAt?: string;
  turnUpdatedAt?: string;
  finalDurationMs?: number;
  finalText?: string;
  renderFinalText?: boolean;
  onApprovalDecision?: ApprovalDecisionHandler;
};

type ApprovalDecisionHandler = (approvalId: string, decision: "accept" | "reject") => void;

const TOOL_TRANSCRIPT_TEXT_CLASS = "text-[14px] leading-6";
const TOOL_TRANSCRIPT_CHILD_INDENT_CLASS = "pl-3";
const MAX_TOOL_OUTPUT_PREVIEW_CHARS = 480;
type TranscriptGroupKind = "search" | "command" | "tool" | "mcp" | "file" | "";

const SEARCH_DISPLAY_KINDS = new Set([
  "web_search",
  "web.search",
  "search_web",
  "browse_url",
  "browser",
  "browser.open",
]);

const TRACE_ONLY_DISPLAY_KINDS = new Set([
  "model_call",
  "model.call",
  "model_wait",
  "model.wait",
]);

/**
 * Represents either a single block (reasoning or standalone tool) or a merged group
 * of consecutive same-kind tool blocks.
 */
export type ProcessGroup =
  | { kind: "single"; block: AiopsProcessBlock }
  | { kind: "merged"; blocks: AiopsProcessBlock[]; mergedKind: Exclude<TranscriptGroupKind, ""> | "mixed" };

export function ProcessTranscript({
  process,
  agentSteps,
  turnStatus,
  turnStartedAt,
  turnCompletedAt,
  turnUpdatedAt,
  finalDurationMs,
  finalText,
  renderFinalText = true,
}: ProcessTranscriptProps) {
  const projectedAgentProcess = useMemo(() => agentStepsToProcessBlocks(agentSteps || []), [agentSteps]);
  const sourceProcess = process.length ? process : projectedAgentProcess;
  const visibleProcess = useMemo(
    () => dedupeTypedModelPreludes(sourceProcess).filter(shouldShowInTranscript),
    [sourceProcess],
  );
  const running = isProcessRunning(visibleProcess, turnStatus);
  const waiting = isProcessWaiting(visibleProcess, turnStatus);
  const live = running || waiting;
  const explicitFinalText = finalText?.trim() || "";
  const processBlocks = visibleProcess;
  const renderedFinalText = shouldRenderFinalTranscriptText(turnStatus) ? explicitFinalText : "";
  const hasMeaningful = hasMeaningfulContent(processBlocks);
  const hasTurnTiming = Boolean(turnStartedAt || turnCompletedAt || turnUpdatedAt);
  const finalGenerationLabel = live ? "" : formatFinalGenerationDuration(finalDurationMs);
  const shouldRenderProcess = processBlocks.length > 0 || running || waiting || hasTurnTiming || Boolean(finalGenerationLabel);
  const fallbackStartRef = useRef(Date.now());
  const [nowMs, setNowMs] = useState(Date.now());
  const [open, setOpen] = useState(live);
  const prevLiveRef = useRef(live);

  useEffect(() => {
    if (!running) {
      return undefined;
    }
    const interval = setInterval(() => setNowMs(Date.now()), 1000);
    return () => clearInterval(interval);
  }, [running]);

  useEffect(() => {
    if (prevLiveRef.current && !live) {
      setOpen(false);
    }
    prevLiveRef.current = live;
  }, [live]);

  useEffect(() => {
    if (live) {
      setOpen(true);
    }
  }, [live]);

  useEffect(() => {
    debugTranscriptProjection("render_decision", {
      turnStatus,
      renderFinalText,
      sourceCount: sourceProcess.length,
      visibleCount: visibleProcess.length,
      processCount: processBlocks.length,
      explicitFinal: debugTextFacts(explicitFinalText),
      renderedFinal: debugTextFacts(renderedFinalText),
      visibleBlocks: visibleProcess.map(debugProcessBlock),
      renderedProcessBlocks: processBlocks.map(debugProcessBlock),
    });
  }, [
    explicitFinalText,
    processBlocks,
    renderFinalText,
    renderedFinalText,
    sourceProcess,
    turnStatus,
    visibleProcess,
  ]);

  if (!shouldRenderProcess && (!renderFinalText || !renderedFinalText)) {
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
  const timeLabel = elapsed ? ` ${formatElapsedDuration(elapsed)}` : "";
  const headerLabel = processHeaderLabel({ running, waiting });

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
            <span className="font-medium text-slate-500">
              {`${headerLabel}${timeLabel}`}
            </span>
            <DisclosureChevron open={open} testId="aiops-process-header-chevron" />
          </button>

          {open && (hasMeaningful || finalGenerationLabel) ? (
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
              {finalGenerationLabel ? <FinalGenerationDuration label={finalGenerationLabel} /> : null}
            </div>
          ) : null}
        </>
      ) : null}
      {renderFinalText && renderedFinalText ? <AssistantFinalText text={renderedFinalText} /> : null}
    </div>
  );
}

function FinalGenerationDuration({ label }: { label: string }) {
  return (
    <div className="flex items-center gap-1.5 text-[13px] leading-6 text-slate-400" data-testid="aiops-final-generation-duration">
      <span className="h-1.5 w-1.5 rounded-full bg-slate-300" aria-hidden="true" />
      <span>{label}</span>
    </div>
  );
}

function agentStepsToProcessBlocks(steps: AgentStepView[]): AiopsProcessBlock[] {
  return steps
    .filter((step) => step?.id && (step.title || step.toolName || step.outputSummary || step.inputSummary))
    .map((step) => {
      const kind = processKindForAgentStep(step.kind);
      const text = step.title || step.outputSummary || step.inputSummary || step.toolName || step.id;
      return {
        id: step.id,
        kind,
        displayKind: step.kind,
        status: processStatusForAgentStep(step.status),
        text,
        source: step.toolName,
        toolCallId: step.toolCallId,
        checkpointId: step.checkpointId,
        approvalId: step.approvalId,
        inputSummary: step.title || step.inputSummary,
        outputPreview: step.outputSummary,
        queries: kind === "search" ? [step.title || step.inputSummary || text] : undefined,
        targetSummary: step.targetRefs?.join("；"),
        evidenceRefs: step.evidenceRefs,
        updatedAt: step.completedAt || step.startedAt,
      };
    });
}

function processKindForAgentStep(kind?: AgentStepView["kind"]): AiopsTransportProcessKind {
  switch (kind) {
    case "tool_search":
      return "tool";
    case "tool_call":
      return "tool";
    case "approval":
      return "approval";
    case "mcp_health":
      return "mcp";
    case "evidence":
      return "evidence";
    case "final_response":
      return "assistant";
    case "error":
    case "checkpoint":
      return "system";
    case "reasoning":
    default:
      return "reasoning";
  }
}

function processStatusForAgentStep(status?: AgentStepView["status"]): AiopsTransportProcessStatus {
  switch (status) {
    case "pending":
      return "queued";
    case "running":
      return "running";
    case "waiting_approval":
      return "blocked";
    case "failed":
      return "failed";
    case "cancelled":
      return "rejected";
    case "skipped":
      return "skipped";
    case "completed":
    default:
      return "completed";
  }
}

function processHeaderLabel({ running, waiting }: { running: boolean; waiting: boolean }) {
  if (running) {
    return "处理中";
  }
  if (waiting) {
    return "等待审核";
  }
  return "已处理";
}

function shouldShowInTranscript(block: AiopsProcessBlock) {
  const text = (block.text || block.command || block.outputPreview || "").trim();
  const displayKind = (block.displayKind || "").trim().toLowerCase();
  if (block.kind === "assistant" && block.phase === "final_answer") {
    return false;
  }
  if (block.kind === "reasoning" && TRACE_ONLY_DISPLAY_KINDS.has(displayKind)) {
    return false;
  }
  if (block.kind === "approval") {
    return Boolean(text || block.approvalId || block.targetSummary || block.riskSummary || block.expectedEffect);
  }
  if (!text && !block.steps?.length && !block.queries?.length && !block.results?.length) {
    return false;
  }
  return true;
}

function dedupeTypedModelPreludes(blocks: AiopsProcessBlock[]) {
  const seenIterations = new Set<number>();
  return blocks.filter((block) => {
    if (block.commentarySource !== "model_prelude" || typeof block.iteration !== "number") {
      return true;
    }
    if (seenIterations.has(block.iteration)) {
      return false;
    }
    seenIterations.add(block.iteration);
    return true;
  });
}

function isSearchLikeBlock(block: AiopsProcessBlock) {
  const displayKind = (block.displayKind || "").trim().toLowerCase();
  return block.foldGroupKind === "web_lookup" || block.kind === "search" || SEARCH_DISPLAY_KINDS.has(displayKind);
}

function isTerminalTurnStatus(turnStatus?: string) {
  return turnStatus === "completed" || turnStatus === "failed" || turnStatus === "canceled";
}

function shouldRenderFinalTranscriptText(turnStatus: string | undefined) {
  return !turnStatus || isTerminalTurnStatus(turnStatus);
}

function isProcessRunning(process: AiopsProcessBlock[], turnStatus?: string) {
  if (turnStatus === "completed" || turnStatus === "failed" || turnStatus === "canceled") {
    return false;
  }
  if (turnStatus === "working" || turnStatus === "submitted") {
    return true;
  }
  return process.some((block) => block.status === "running" || block.status === "queued");
}

function isProcessWaiting(process: AiopsProcessBlock[], turnStatus?: string) {
  if (turnStatus === "completed" || turnStatus === "failed" || turnStatus === "canceled") {
    return false;
  }
  if (turnStatus === "blocked") {
    return true;
  }
  return process.some((block) => block.status === "blocked");
}

function hasMeaningfulContent(blocks: AiopsProcessBlock[]): boolean {
  return blocks.some((block) => {
    if (block.kind === "reasoning") {
      if (isBlockActive(block)) {
        return false;
      }
      const text = (block.text || "").trim().toLowerCase();
      return text !== "" && text !== "calling model";
    }
    return true;
  });
}

/**
 * Groups only consecutive same-class web lookups or commands.
 */
export function groupConsecutiveBlocks(blocks: AiopsProcessBlock[]): ProcessGroup[] {
  const groups: ProcessGroup[] = [];
  let i = 0;

  while (i < blocks.length) {
    const block = blocks[i];
    const groupKind = groupingKindForBlock(block);

    if (block.kind === "reasoning" || !groupKind) {
      groups.push({ kind: "single", block });
      i += 1;
      continue;
    }

    const consecutive: AiopsProcessBlock[] = [block];
    let j = i + 1;
    while (
      j < blocks.length &&
      groupingKindForBlock(blocks[j]) === groupKind &&
      hasSameFoldIdentity(block, blocks[j])
    ) {
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

function hasSameFoldIdentity(left: AiopsProcessBlock, right: AiopsProcessBlock) {
  const leftFoldGroupId = (left.foldGroupId || "").trim();
  const rightFoldGroupId = (right.foldGroupId || "").trim();
  const leftFoldGroupKind = (left.foldGroupKind || "").trim().toLowerCase();
  const rightFoldGroupKind = (right.foldGroupKind || "").trim().toLowerCase();
  return Boolean(
    leftFoldGroupId &&
    rightFoldGroupId &&
    leftFoldGroupId === rightFoldGroupId &&
    leftFoldGroupKind &&
    leftFoldGroupKind === rightFoldGroupKind
  );
}

function groupingKindForBlock(block: AiopsProcessBlock): TranscriptGroupKind {
  const explicitKind = (block.foldGroupKind || "").trim().toLowerCase();
  if (explicitKind === "web_lookup") {
    return "search";
  }
  if (
    explicitKind === "command" ||
    explicitKind === "tool" ||
    explicitKind === "mcp" ||
    explicitKind === "file"
  ) {
    return explicitKind;
  }
  return "";
}

function mergedKindForBlocks(blocks: AiopsProcessBlock[]): Exclude<TranscriptGroupKind, ""> | "mixed" {
  const kinds = Array.from(
    new Set(blocks.map(groupingKindForBlock).filter((kind): kind is Exclude<TranscriptGroupKind, ""> => kind !== "")),
  );
  return kinds.length === 1 ? kinds[0] : "mixed";
}

export function getMergedSummaryText(mergedKind: string, count: number): string {
  switch (mergedKind) {
    case "file":
      return `已探索 ${count} 个文件`;
    case "command":
      return count > 1 ? `已运行 ${count} 条命令` : "已运行命令";
    case "search":
      return `网页搜索 ${count} 项`;
    case "tool":
    case "mcp":
      return `已调用 ${count} 个工具`;
    default:
      return `已处理 ${count} 个操作`;
  }
}

function ToolSummaryIcon({ kind, testId }: { kind: string; testId?: string }) {
  const Icon = toolSummaryIconForKind(kind);
  return <Icon className="h-4 w-4 shrink-0 text-slate-400" data-testid={testId} aria-hidden="true" />;
}

function toolSummaryIconForKind(kind: string): LucideIcon {
  switch (kind) {
    case "command":
      return SquareTerminal;
    case "search":
      return Search;
    case "file":
      return FileSearch;
    case "tool":
    case "mcp":
      return Wrench;
    default:
      return ListChecks;
  }
}

function DisclosureChevron({ open, testId, className }: { open: boolean; testId?: string; className?: string }) {
  return (
    <ChevronDown
      className={cn(
        "h-3.5 w-3.5 shrink-0 text-slate-400 opacity-0 transition-all group-hover:opacity-100 group-focus-visible:opacity-100",
        open ? "rotate-0 opacity-100" : "-rotate-90",
        className,
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
  const searchSummary = summarizeSearchBlocks(blocks);
  const activeSearchQuery = primarySearchQuery(blocks);
  const running = turnRunning && blocks.some(isBlockActive);
  const failedSearch = blocks.find((block) => block.status === "failed");
  const failureText = failedSearch
    ? `检索失败 ${searchQueryForBlock(failedSearch) || stripHtml(failedSearch.text || "") || failedSearch.displayKind || ""}`.trim()
    : "";
  return (
    <SearchTranscript
      query={activeSearchQuery}
      searchCount={searchSummary.searchCount}
      resultCount={searchSummary.resultCount}
      rows={searchSummary.rows}
      running={running}
      defaultOpen={false}
      failureText={failureText}
    />
  );
}

function isBlockActive(block: AiopsProcessBlock) {
  return block.status === "running" || block.status === "queued";
}

function MergedToolSummary({
  group,
}: {
  group: Extract<ProcessGroup, { kind: "merged" }>;
}) {
  const actionTitle = group.blocks.find(
    (block) => block.kind === "assistant" && block.phase === "commentary",
  );
  const actionBlocks = actionTitle ? group.blocks.filter((block) => block.id !== actionTitle.id) : group.blocks;
  const text = actionTitle?.text?.trim() || getMergedGroupSummaryText(group, actionBlocks);
  const details = actionBlocks.map(mergedBlockDetail).filter((detail) => detail.text);
  const [open, setOpen] = useState(
    actionBlocks.some((block) => isBlockActive(block) || block.status === "failed" || block.status === "rejected" || block.status === "blocked"),
  );
  if (!details.length) {
    return (
      <div className={cn("flex min-w-0 items-center gap-1.5 text-slate-400", TOOL_TRANSCRIPT_TEXT_CLASS)}>
        <ToolSummaryIcon kind={group.mergedKind} testId={`aiops-merged-${group.mergedKind}-icon`} />
        <span className="min-w-0 truncate">{text}</span>
      </div>
    );
  }

  return (
    <div className="space-y-1">
      <button
        type="button"
        className={cn(
          "group flex w-full min-w-0 items-center gap-1.5 text-left text-slate-400 transition-colors hover:text-slate-600",
          TOOL_TRANSCRIPT_TEXT_CLASS,
        )}
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        data-testid={`aiops-merged-${group.mergedKind}-toggle`}
      >
        <ToolSummaryIcon kind={group.mergedKind} testId={`aiops-merged-${group.mergedKind}-icon`} />
        <span className="min-w-0 truncate">{text}</span>
        <DisclosureChevron open={open} testId={`aiops-merged-${group.mergedKind}-chevron`} />
      </button>
      {open ? (
        <div
          className={cn(
            "space-y-2 overflow-visible text-[13px] leading-6 text-slate-500",
            TOOL_TRANSCRIPT_CHILD_INDENT_CLASS,
          )}
          data-testid={`aiops-merged-${group.mergedKind}-details`}
        >
          {details.map((detail) =>
            detail.kind === "command" ? (
              <CommandDetailRow key={detail.id} detail={detail} />
            ) : (
              <ToolDetailRow key={detail.id} detail={detail} />
            ),
          )}
        </div>
      ) : null}
    </div>
  );
}

function ToolDetailRow({
  detail,
  showSummaryIcon = false,
}: {
  detail: ReturnType<typeof mergedBlockDetail>;
  showSummaryIcon?: boolean;
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
    <div className="min-w-0 space-y-2">
      <button
        type="button"
        className={cn(
          "group flex w-full min-w-0 items-center gap-1.5 text-left text-slate-400 transition-colors hover:text-slate-600",
          TOOL_TRANSCRIPT_TEXT_CLASS,
        )}
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        data-testid={`aiops-tool-row-${detail.id}`}
      >
        <span className="flex min-w-0 flex-1 items-center gap-1.5">
          {showSummaryIcon ? (
            <ToolSummaryIcon kind={detail.kind} testId={`aiops-tool-icon-${detail.id}`} />
          ) : null}
          <span className="min-w-0 truncate">{toolDetailSummaryLabel(detail)}</span>
        </span>
        <span className="shrink-0 text-[13px] text-slate-400">{statusLabel(detail.status)}</span>
        <DisclosureChevron open={open} testId={`aiops-tool-chevron-${detail.id}`} />
      </button>
      {open && hasOutput ? (
        <div
          className="max-w-full overflow-hidden whitespace-pre-wrap break-words rounded-lg bg-slate-100 px-3 py-2 font-mono text-[13px] leading-6 text-slate-500"
          data-testid={`aiops-tool-output-${detail.id}`}
        >
          {detail.output}
        </div>
      ) : null}
      <EvidenceRefs refs={detail.evidenceRefs} />
    </div>
  );
}

function CommandDetailRow({
  detail,
  showSummaryIcon = false,
}: {
  detail: ReturnType<typeof mergedBlockDetail>;
  showSummaryIcon?: boolean;
}) {
  const hasOutput = Boolean(detail.output);
  const commandRunning = detail.status === "running" || detail.status === "queued";
  const rowStatus = commandRowStatusLabel(detail.status);
  const cardStatus = terminalCardStatusLabel(detail.status);
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
        className={cn(
          "group flex w-full min-w-0 items-center gap-1.5 text-left text-slate-400 transition-colors hover:text-slate-600",
          TOOL_TRANSCRIPT_TEXT_CLASS,
        )}
        onClick={() => setOpen((value) => !value)}
        aria-expanded={open}
        data-testid={`aiops-command-row-${detail.id}`}
      >
        <span
          className="flex min-w-0 flex-1 items-center gap-1.5"
          data-testid={`aiops-command-label-region-${detail.id}`}
        >
          {showSummaryIcon ? (
            <ToolSummaryIcon kind={detail.kind} testId={`aiops-command-icon-${detail.id}`} />
          ) : null}
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
          className="flex max-h-72 flex-col overflow-hidden rounded-lg bg-slate-100 px-3 py-2 text-slate-500"
          data-testid={`aiops-terminal-card-${detail.id}`}
        >
          <div className="shrink-0 text-[13px] leading-5 text-slate-500">Shell</div>
          <div className="mt-2 shrink-0 whitespace-pre-wrap break-words font-mono text-[13px] leading-6 text-slate-950">
            $ {detail.text}
          </div>
          {hasOutput ? (
            <pre
              className="mt-3 min-h-0 max-h-[12rem] flex-1 overflow-x-hidden overflow-y-auto overscroll-contain whitespace-pre-wrap break-words rounded-md bg-slate-100 font-mono text-[13px] leading-6 text-slate-500"
              data-testid={`aiops-command-output-${detail.id}`}
            >
              {detail.output}
            </pre>
          ) : null}
          {cardStatus ? (
            <div className="mt-2 flex shrink-0 justify-end text-[13px] leading-5 text-slate-500">
              {cardStatus}
            </div>
          ) : null}
        </div>
      ) : null}
      <EvidenceRefs refs={detail.evidenceRefs} />
    </div>
  );
}

function EvidenceRefs({ refs }: { refs?: string[] }) {
  if (!refs?.length) {
    return null;
  }
  return null;
}

function commandRowStatusLabel(status?: AiopsProcessBlock["status"]) {
  if (status === "completed" || status === "running" || status === "blocked") {
    return "";
  }
  return terminalStatusLabel(status);
}

function terminalCardStatusLabel(status?: AiopsProcessBlock["status"]) {
  if (status === "running" || status === "queued" || status === "blocked") {
    return "";
  }
  return terminalStatusLabel(status);
}

function getMergedGroupSummaryText(
  group: Extract<ProcessGroup, { kind: "merged" }>,
  blocks: AiopsProcessBlock[] = group.blocks,
) {
  if (group.mergedKind === "mixed") {
    return getMixedMergedSummaryText(blocks);
  }
  return getMergedSummaryText(group.mergedKind, blocks.length);
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
  const hasOutputPreview = typeof block.outputPreview === "string" && block.outputPreview.trim() !== "";
  if (isSearchLikeBlock(block)) {
    text = searchDetailTextForBlock(block);
  } else if (block.kind === "command") {
    text = block.command || block.inputSummary || stripHtml(block.text || "");
  } else if (block.kind === "file") {
    text = stripHtml(block.text || "") || block.inputSummary || block.displayKind || "";
  } else {
    text = toolInvocationLabel(block) || stripHtml(block.text || "") || block.command || block.inputSummary || block.displayKind || "";
  }
  return {
    id: block.id,
    kind: groupingKindForBlock(block),
    status: block.status,
    approvalId: block.approvalId,
    evidenceRefs: uniqueLines(block.evidenceRefs || []),
    externalized: isExternalizedProcessBlock(block),
    mock: Boolean(block.mock),
    text: block.kind === "command" ? stripHtml(text).trim() : cleanToolText(text),
    output: hasOutputPreview && (block.kind === "command" || block.kind === "tool" || block.kind === "mcp")
      ? compactOutputPreviewForBlock(block)
      : "",
  };
}

function searchDetailTextForBlock(block: AiopsProcessBlock) {
  const source = searchResultSources(block)[0];
  return source ? searchSourceRowLabel(source) : searchQueryForBlock(block) || browseUrlForBlock(block) || "搜索网页";
}

function isExternalizedProcessBlock(block: AiopsProcessBlock) {
  const tier = (block.materializationTier || "").toLowerCase();
  return Boolean(block.externalReferences?.length || tier === "large" || tier === "externalized" || tier === "summary");
}

function cleanCommandOutput(value?: string) {
  return stripHtml(value || "").trim();
}

function compactOutputPreviewForBlock(block: AiopsProcessBlock) {
  const rawOutput = cleanCommandOutput(block.outputPreview);
  const output = block.kind === "command" ? commandTerminalOutputForDisplay(rawOutput, block) : rawOutput;
  if (!output) {
    return "";
  }
  if (block.kind !== "command" && (isExternalizedProcessBlock(block) || isLargeStructuredPayload(output))) {
    return truncateToolOutputPreview(output);
  }
  return output;
}

type CommandTerminalOutputEnvelope = {
  status?: unknown;
  stdout?: unknown;
  stderr?: unknown;
  output?: unknown;
  error?: unknown;
  exitCode?: unknown;
};

function commandTerminalOutputForDisplay(outputPreview: string, block: AiopsProcessBlock) {
  const envelope = parseCommandTerminalOutputEnvelope(outputPreview);
  if (!envelope) {
    return outputPreview;
  }
  const failed = commandOutputFailed(envelope, block);
  if (failed) {
    return firstNonEmptyDisplayString(envelope.stderr, envelope.error, envelope.stdout, envelope.output);
  }
  return firstNonEmptyDisplayString(envelope.stdout, envelope.output, envelope.stderr, envelope.error);
}

function parseCommandTerminalOutputEnvelope(outputPreview: string): CommandTerminalOutputEnvelope | null {
  const text = outputPreview.trim();
  if (!text) {
    return null;
  }
  try {
    const parsed = JSON.parse(text);
    if (typeof parsed === "string" && parsed.trim().startsWith("{")) {
      return parseCommandTerminalOutputEnvelope(parsed);
    }
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
      return null;
    }
    const envelope = parsed as CommandTerminalOutputEnvelope;
    if (
      ["stdout", "stderr", "output", "error", "exitCode", "status"].some((key) =>
        Object.prototype.hasOwnProperty.call(envelope, key),
      )
    ) {
      return envelope;
    }
  } catch {
    return null;
  }
  return null;
}

function commandOutputFailed(envelope: CommandTerminalOutputEnvelope, block: AiopsProcessBlock) {
  const envelopeExitCode = numericValue(envelope.exitCode);
  const blockExitCode = typeof block.exitCode === "number" ? block.exitCode : undefined;
  const status = stringValue(envelope.status).toLowerCase();
  return (
    block.status === "failed" ||
    block.status === "rejected" ||
    (typeof blockExitCode === "number" && blockExitCode !== 0) ||
    (typeof envelopeExitCode === "number" && envelopeExitCode !== 0) ||
    status === "error" ||
    status === "failed" ||
    status === "failure"
  );
}

function firstNonEmptyDisplayString(...values: unknown[]) {
  for (const value of values) {
    const text = stringValue(value);
    if (text.trim()) {
      return text.trim();
    }
  }
  return "";
}

function stringValue(value: unknown) {
  if (typeof value === "string") {
    return value;
  }
  if (value === null || value === undefined) {
    return "";
  }
  return String(value);
}

function numericValue(value: unknown) {
  if (typeof value === "number" && Number.isFinite(value)) {
    return value;
  }
  if (typeof value === "string" && value.trim()) {
    const parsed = Number(value);
    return Number.isFinite(parsed) ? parsed : undefined;
  }
  return undefined;
}

function truncateToolOutputPreview(value: string) {
  const text = value.trim();
  if (text.length <= MAX_TOOL_OUTPUT_PREVIEW_CHARS) {
    return text;
  }
  return `${text.slice(0, MAX_TOOL_OUTPUT_PREVIEW_CHARS).trimEnd()}...`;
}

function isLargeStructuredPayload(value: string) {
  const text = value.trim();
  return text.length > 280 && (isRawPayloadLine(text) || /\bchartReports\b|\bwidgets\b|\bseries\b/.test(text));
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
  const prefix = detail.mock ? "Mock " : "";
  switch (detail.status) {
    case "blocked":
      return `等待审核 ${prefix}${command}`;
    case "failed":
      return `运行失败 ${prefix}${command}`;
    case "running":
      return `正在运行 ${prefix}${command}`;
    case "queued":
      return `排队中 ${prefix}${command}`;
    case "rejected":
      return `已拒绝 ${prefix}${command}`;
    default:
      return `已运行 ${prefix}${command}`;
  }
}

function toolDetailSummaryLabel(detail: ReturnType<typeof mergedBlockDetail>) {
  const text = detail.text || "工具调用";
  const prefix = detail.mock ? "Mock " : "";
  if (detail.kind === "search") {
    switch (detail.status) {
      case "failed":
        return `检索失败 ${text}`;
      case "running":
        return `正在检索 ${text}`;
      case "queued":
        return `排队检索 ${text}`;
      case "rejected":
        return `已取消检索 ${text}`;
      default:
        return text;
    }
  }
  switch (detail.status) {
    case "blocked":
      return `等待审核 ${prefix}${text}`;
    case "failed":
      return `执行失败 ${prefix}${text}`;
    case "running":
      return `正在执行 ${prefix}${text}`;
    case "queued":
      return `排队中 ${prefix}${text}`;
    case "rejected":
      return `已拒绝 ${prefix}${text}`;
    default:
      return `${prefix}${text}`;
  }
}

function NativeProcessText({
  block,
}: {
  block: AiopsProcessBlock;
}) {
  if (block.kind === "assistant") {
    return <AssistantProgressText text={block.text} />;
  }
  if (block.kind === "reasoning") {
    return <ThinkingText block={block} />;
  }
  if (block.kind === "plan") {
    return <PlanSteps block={block} />;
  }
  if (block.kind === "command") {
    return <CommandDetailRow detail={mergedBlockDetail(block)} showSummaryIcon />;
  }
  if (isToolSummaryBlock(block)) {
    return <ToolDetailRow detail={mergedBlockDetail(block)} showSummaryIcon />;
  }
  const text = readableBlockSummary(block);
  if (!text) {
    return null;
  }
  return (
    <div className="whitespace-pre-wrap break-words text-[16px] font-medium leading-8 text-slate-950">
      {text}
    </div>
  );
}

function PlanSteps({ block }: { block: AiopsProcessBlock }) {
  if (!block.steps?.length) {
    const text = isCompactPlanSummary(block.text) ? "" : readableBlockSummary(block);
    return text ? (
      <div className="whitespace-pre-wrap break-words text-[16px] font-medium leading-8 text-slate-950">
        {text}
      </div>
    ) : null;
  }
  return (
    <ol className="space-y-1.5 text-[14px] leading-6 text-slate-500" data-testid={`aiops-plan-steps-${block.id}`}>
      {block.steps.map((step) => (
        <li key={step.id || step.text} className="flex min-w-0 gap-2">
          <span className="mt-2 h-1.5 w-1.5 shrink-0 rounded-full bg-slate-300" aria-hidden="true" />
          <span className="min-w-0 break-words">
            {step.text}
            {step.summary ? <span className="ml-1 text-slate-400">{step.summary}</span> : null}
          </span>
        </li>
      ))}
    </ol>
  );
}

function isCompactPlanSummary(text?: string) {
  return /^plan updated:/i.test((text || "").trim());
}

function AssistantFinalText({ text, live = false }: { text: string; live?: boolean }) {
  return (
    <div
      className="max-w-none py-1 text-[15px] leading-7 text-slate-950"
      data-testid={live ? "aiops-live-answer-text" : "aiops-final-text"}
      data-answer-state={live ? "live" : "final"}
    >
      <MessageMarkdown text={text} />
    </div>
  );
}

function AssistantProgressText({ text }: { text: string }) {
  return (
    <div
      className="max-w-none py-1 text-[15px] leading-7 text-slate-950"
      data-testid="aiops-assistant-progress-text"
      data-answer-state="progress"
    >
      <MessageMarkdown text={text} />
    </div>
  );
}

function isToolSummaryBlock(block: AiopsProcessBlock): boolean {
  return block.kind === "search" || block.kind === "command" || block.kind === "tool" || block.kind === "file" || block.kind === "mcp";
}

function ThinkingText({ block }: { block: AiopsProcessBlock }) {
  const raw = stripHtml(block.text || "");
  const text = transformThinkingText(raw);
  if (!text || !text.trim()) {
    return null;
  }
  return (
    <div className="whitespace-pre-wrap break-words text-[16px] font-medium leading-8 text-slate-950">
      {text}
    </div>
  );
}

function transformThinkingText(raw: string): string {
  return raw.trim();
}

/**
 * Search transcript: collapsible with max-height constraint.
 * Shows web search references with expandable details.
 */
function SearchTranscript({
  query,
  searchCount,
  resultCount,
  rows,
  running,
  defaultOpen,
  failureText,
}: {
  query: string;
  searchCount: number;
  resultCount: number;
  rows: SearchDetailRow[];
  running: boolean;
  defaultOpen: boolean;
  failureText?: string;
}) {
  const [open, setOpen] = useState(defaultOpen);
  const visibleRows = running && query
    ? rows.filter((row) => row.kind !== "query" || row.label !== query)
    : rows;

  useEffect(() => {
    if (defaultOpen) {
      setOpen(true);
    }
  }, [defaultOpen]);

  return (
    <div className="space-y-1">
      <button
        type="button"
        className={cn(
          "group flex min-w-0 items-center gap-1.5 text-left text-slate-400 transition-colors hover:text-slate-600",
          TOOL_TRANSCRIPT_TEXT_CLASS,
        )}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        data-testid="aiops-search-toggle"
      >
        <ToolSummaryIcon kind="search" testId="aiops-search-icon" />
        <span className="min-w-0 truncate">{failureText || searchTranscriptLabel(running, searchCount, resultCount, query)}</span>
        <DisclosureChevron open={open} testId="aiops-search-chevron" />
      </button>
      {running && query && !open ? <ActiveSearchStatus query={query} /> : null}
      {open && (running && query || visibleRows.length) ? (
        <div
          className={cn("space-y-1 text-slate-400", TOOL_TRANSCRIPT_TEXT_CLASS, TOOL_TRANSCRIPT_CHILD_INDENT_CLASS)}
          data-testid="aiops-search-details"
        >
          {running && query ? <ActiveSearchStatus query={query} /> : null}
          {visibleRows.map((row) => (
            <SearchDetailRowView key={row.id} row={row} />
          ))}
        </div>
      ) : null}
    </div>
  );
}

function ActiveSearchStatus({ query }: { query: string }) {
  return (
    <div className="flex min-w-0 items-center gap-1.5 text-sky-600" data-testid="aiops-search-running-status">
      <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-sky-300" aria-hidden="true" />
      <span className="min-w-0 truncate">{`正在搜索网页：${query}`}</span>
    </div>
  );
}

type SearchSummary = {
  searchCount: number;
  resultCount: number;
  rows: SearchDetailRow[];
};

type SearchDetailRow = {
  id: string;
  label: string;
  title: string;
  url: string;
  query: string;
  snippet: string;
  fetched?: boolean;
  fetchError?: string;
  kind: "source" | "query";
};

function SearchDetailRowView({ row }: { row: SearchDetailRow }) {
  const [open, setOpen] = useState(false);
  const detailLines = [
    row.fetched ? "已读取正文" : "",
    row.fetchError ? `读取失败：${row.fetchError}` : "",
    row.snippet,
  ].filter(Boolean);

  return (
    <div className="space-y-1" data-testid="aiops-search-detail-line">
      <button
        type="button"
        className="group flex min-w-0 items-start gap-1.5 text-left text-slate-400 transition-colors hover:text-slate-600"
        aria-expanded={open}
        onClick={() => setOpen((value) => !value)}
        data-testid="aiops-search-detail-row-toggle"
      >
        <span className="pt-[1px]" aria-hidden="true">-</span>
        <span className={cn(
          "min-w-0 whitespace-normal break-all",
          row.url ? "text-sky-600 group-hover:underline" : "",
        )}>
          {row.label}
        </span>
        {detailLines.length ? (
          <DisclosureChevron open={open} testId="aiops-search-detail-chevron" className="mt-[5px]" />
        ) : null}
      </button>
      {open && detailLines.length ? (
        <div
          className="space-y-1 pl-4 text-[13px] leading-6 text-slate-400"
          data-testid="aiops-search-detail-expanded"
        >
          {detailLines.map((line) => (
            <div key={line} className="whitespace-normal break-all">
              {line}
            </div>
          ))}
        </div>
      ) : null}
    </div>
  );
}

function summarizeSearchBlocks(blocks: AiopsProcessBlock[]): SearchSummary {
  const queries = uniqueLines(blocks.map(searchQueryForBlock));
  const results = uniqueSearchSources(blocks.flatMap(searchResultSources));
  const browsedSources = blocks.flatMap((block) => {
    const url = browseUrlForBlock(block);
    if (!url) return [];
    return [{
      label: "已打开页面",
      url,
      query: searchQueryForBlock(block),
      snippet: "",
    }];
  });
  const resultSources = uniqueSearchSources([...results, ...browsedSources]);
  const rows = resultSources.length
    ? resultSources.map((source, index) => sourceToSearchDetailRow(source, index))
    : queries.map((query, index) => ({
      id: `query:${index}:${query}`,
      label: query,
      title: "",
      url: "",
      query,
      snippet: "",
      kind: "query" as const,
    }));

  return {
    searchCount: Math.max(1, blocks.filter(isSearchLikeBlock).length),
    resultCount: resultSources.length,
    rows,
  };
}

type SearchSource = {
  label: string;
  url: string;
  query?: string;
  snippet?: string;
  fetched?: boolean;
  fetchError?: string;
};

function uniqueSearchSources(sources: SearchSource[]) {
  const seen = new Map<string, number>();
  const result: SearchSource[] = [];
  for (const source of sources) {
    const label = cleanSearchResultText(source.label);
    const url = extractUrl(source.url || source.label);
    const query = cleanSearchLine(source.query);
    const snippet = cleanSearchResultText(source.snippet);
    const key = (url || label).toLowerCase();
    if (!label && !url) continue;
    const existingIndex = seen.get(key);
    if (existingIndex !== undefined) {
      const existing = result[existingIndex];
      result[existingIndex] = {
        ...existing,
        query: existing.query || query,
        snippet: existing.snippet || snippet,
        fetched: existing.fetched || Boolean(source.fetched),
        fetchError: existing.fetchError || cleanSearchResultText(source.fetchError),
      };
      continue;
    }
    seen.set(key, result.length);
    result.push({
      label: label || url,
      url,
      query,
      snippet,
      fetched: Boolean(source.fetched),
      fetchError: cleanSearchResultText(source.fetchError),
    });
  }
  return result;
}

function sourceToSearchDetailRow(source: SearchSource, index: number): SearchDetailRow {
  return {
    id: `source:${index}:${source.url || source.label}`,
    label: source.url || searchSourceRowLabel(source),
    title: source.url ? searchSourceRowLabel(source) : "",
    url: source.url,
    query: source.query || "",
    snippet: source.snippet || "",
    fetched: source.fetched,
    fetchError: source.fetchError,
    kind: "source",
  };
}

function searchSourceRowLabel(source: SearchSource) {
  const label = source.label.length > 96 ? `${source.label.slice(0, 96).trim()}…` : source.label;
  return label;
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
  if (block.kind === "approval") {
    const target = stripHtml(block.command || block.targetSummary || block.text || "").trim();
    if (block.status === "rejected") {
      return target ? `已拒绝，将基于已有证据继续分析：${target}` : "已拒绝，将基于已有证据继续分析";
    }
    if (block.status === "blocked") {
      return target ? `等待审批：${target}` : "等待审批";
    }
    return target || "已处理审批";
  }
  return cleanToolText(stripHtml(block.text || "") || block.displayKind || block.kind);
}

function searchResultSources(block: AiopsProcessBlock): SearchSource[] {
  const sources: SearchSource[] = [];
  const query = searchQueryForBlock(block);
  for (const result of block.results || []) {
    const title = cleanSearchResultText(result.title);
    const url = extractUrl(result.url);
    const snippet = cleanSearchResultText(result.snippet);
    if (title || snippet || url) {
      sources.push({
        label: title || snippet || url,
        url,
        query,
        snippet,
        fetched: Boolean(result.fetched),
        fetchError: result.fetchError,
      });
    }
  }
  const browsedUrl = browseUrlForBlock(block);
  if (browsedUrl) {
    sources.push({ label: "已打开页面", url: browsedUrl, query, snippet: "" });
  }
  return uniqueSearchSources(sources);
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

function searchTranscriptLabel(running: boolean, searchCount: number, resultCount: number, _query: string) {
  const countLabel = `网页搜索 ${Math.max(1, searchCount)} 次`;
  if (!running || resultCount > 0) {
    return `${countLabel} · 找到 ${resultCount} 个来源`;
  }
  return countLabel;
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
  return block.kind === "search" || block.foldGroupKind === "web_lookup" || SEARCH_DISPLAY_KINDS.has((block.displayKind || "").trim().toLowerCase());
}

function isBrowseActionBlock(block: AiopsProcessBlock) {
  const displayKind = (block.displayKind || "").trim().toLowerCase();
  return block.operation === "open" || displayKind === "browse_url" || displayKind === "browser.open";
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

function cleanSearchResultText(value?: string) {
  let text = unwrapProviderPayload(value || "");
  text = decodeHtmlish(text)
    .replace(/provider-native\s+/gi, "")
    .replace(/Do not repeat this exact query.*$/gi, "")
    .replace(/provider returned no textual summary.*$/gi, "")
    .replace(/\s*;\s*$/g, "")
    .replace(/\s+/g, " ")
    .trim();
  text = text.replace(/^["'""]+|["'""]+$/g, "").trim();
  if (!text || isInternalSearchLine(text) || isRawPayloadLine(text)) {
    return "";
  }
  return text.length > 220 ? `${text.slice(0, 220).trim()}…` : text;
}

function isGenericSearchLabel(value: string) {
  const normalized = value.trim().toLowerCase();
  return normalized === "searching the web" || normalized === "正在搜索网页" || normalized === "搜索网页";
}

function cleanToolText(value: string) {
  let text = unwrapProviderPayload(value || "");
  text = decodeHtmlish(text)
    .replace(/provider-native\s+/gi, "")
    .replace(/\s*;\s*$/g, "")
    .replace(/\s+/g, " ")
    .trim();
  text = text.replace(/^["'""]+|["'""]+$/g, "").trim();
  if (/^(browse_url|browser|web_search|search)$/i.test(text)) {
    return "";
  }
  if (!text || isInternalSearchLine(text) || isRawPayloadLine(text)) {
    return "";
  }
  return text.length > 180 ? `${text.slice(0, 180).trim()}…` : text;
}

function toolInvocationLabel(block: AiopsProcessBlock) {
  if (block.kind !== "tool" && block.kind !== "mcp") return "";
  const name = cleanToolText(stripHtml(block.source || "").trim());
  const input = cleanToolInputSummary(block.inputSummary);
  if (!name || /^(tool|mcp)$/i.test(name)) {
    return input;
  }
  if (!input || input.toLowerCase() === name.toLowerCase()) {
    return name;
  }
  return `${name} ${input}`;
}

function cleanToolInputSummary(value?: string) {
  const text = stripHtml(value || "").replace(/\s+/g, " ").trim();
  if (!text || /^(tool|mcp)$/i.test(text)) {
    return "";
  }
  if (isRawPayloadLine(text)) {
    return "";
  }
  return text.length > 180 ? `${text.slice(0, 180).trim()}…` : text;
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

function formatElapsedDuration(totalSeconds: number) {
  const seconds = Math.max(0, Math.round(totalSeconds));
  const hours = Math.floor(seconds / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const remainingSeconds = seconds % 60;
  const parts: string[] = [];
  if (hours > 0) {
    parts.push(`${hours}h`);
  }
  if (minutes > 0 || hours > 0) {
    parts.push(`${minutes}m`);
  }
  parts.push(`${remainingSeconds}s`);
  return parts.join(" ");
}

function formatFinalGenerationDuration(durationMs?: number) {
  const value = Number(durationMs);
  if (!Number.isFinite(value) || value <= 0) {
    return "";
  }
  const rounded = Math.max(1, Math.round(value));
  if (rounded < 1000) {
    return `整理最终回答 ${rounded}ms`;
  }
  return `整理最终回答 ${formatElapsedDuration(Math.round(rounded / 1000))}`;
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
