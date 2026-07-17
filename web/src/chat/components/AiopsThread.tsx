import { MessagePrimitive, ThreadPrimitive, useAssistantTransportState, useMessage } from "@assistant-ui/react";
import { Activity, ArrowDown, BookOpen, Bot, Copy, GitBranch, LoaderCircle, Server, Wrench } from "lucide-react";
import { useLayoutEffect, useMemo, useRef, useState } from "react";

import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import { Button } from "@/components/ui/button";
import type { AiopsContextGovernanceEvent, AiopsProcessBlock, AiopsTransportAgentUiArtifact, AiopsTransportBlock, AiopsTransportMcpSurface, AiopsTransportState, AiopsTransportTurn } from "@/transport/aiopsTransportTypes";
import { AIOPS_BLOCK_DATA_PART, AIOPS_TURN_DATA_PART, mergeOpsManualSearchAndPreflightArtifacts } from "@/transport/aiopsTransportConverter";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

import { parseHostMentionCandidates, parseSpecialAiMentionCandidates, type HostMentionCandidate, type SpecialAiMentionCandidate } from "../hostMentions";
import { AnswerDocumentRenderer } from "./AnswerDocumentRenderer";
import { ContextStatusNotice } from "./ContextStatusNotice";
import { McpSurfacePart } from "./McpSurfacePart";
import { ProcessTranscript } from "./ProcessTranscript";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

export function AiopsThread() {
  const state = useAssistantTransportState() as AiopsTransportState;
  const surfaces = Object.values(state.mcpSurfaces || {});
  const workspace = useSessionWorkspaceContext();
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const stickToBottomRef = useRef(true);
  const lastAutoScrollTurnRef = useRef("");
  const scrollSignature = useMemo(() => aiopsThreadScrollSignature(state), [state]);

  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    const currentTurnId = state.currentTurnId || "";
    const forceForNewTurn = currentTurnId !== "" && currentTurnId !== lastAutoScrollTurnRef.current;
    lastAutoScrollTurnRef.current = currentTurnId;
    if (!viewport || (!stickToBottomRef.current && !forceForNewTurn)) {
      return undefined;
    }
    if (forceForNewTurn) {
      stickToBottomRef.current = true;
    }
    let cancelled = false;
    const scroll = () => {
      if (cancelled) {
        return;
      }
      viewport.scrollTo({ top: viewport.scrollHeight, behavior: "smooth" });
    };
    scroll();
    const firstFrame = window.requestAnimationFrame(scroll);
    const secondFrame = window.requestAnimationFrame(() => {
      window.requestAnimationFrame(scroll);
    });
    return () => {
      cancelled = true;
      window.cancelAnimationFrame(firstFrame);
      window.cancelAnimationFrame(secondFrame);
    };
  }, [scrollSignature]);

  return (
    <ThreadPrimitive.Root className="relative h-full min-h-0 bg-white">
      <ThreadPrimitive.Viewport
        ref={viewportRef}
        autoScroll
        scrollToBottomOnInitialize
        className="h-full overflow-y-auto scroll-smooth"
        onScroll={() => {
          const viewport = viewportRef.current;
          stickToBottomRef.current = !viewport || isNearThreadBottom(viewport);
        }}
      >
        <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col px-4 py-6 md:px-6">
          <ThreadPrimitive.Empty>
            <div className="flex min-h-full flex-1 items-center justify-center pb-10">
              {workspace.busy ? (
                <div className="flex items-center gap-2 text-sm text-slate-500" role="status">
                  <LoaderCircle className="h-4 w-4 animate-spin" />
                  <span>正在恢复会话...</span>
                </div>
              ) : (
                <div className="w-full text-center">
                  <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full border border-slate-200 bg-white text-slate-900 shadow-sm">
                    <Bot className="h-5 w-5" />
                  </div>
                  <h1 className="mt-5 text-2xl font-semibold text-slate-950">{workspace.kind === "workspace" ? "今天要统筹什么运维任务？" : "Hello there"}</h1>
                  <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">{workspace.kind === "workspace" ? "主 Agent 会保留工作台会话，并通过 AssistantTransport 编排后端 host agent。" : "输入排障、巡检或变更任务，消息会进入 AI Chat 会话。"}</p>
                </div>
              )}
            </div>
          </ThreadPrimitive.Empty>
          <div className="flex flex-1 flex-col gap-6 empty:hidden">
            <ThreadPrimitive.Messages
              components={{
                UserMessage,
                AssistantMessage,
              }}
            />
            <McpSurfaceList surfaces={surfaces} />
          </div>
        </div>
      </ThreadPrimitive.Viewport>
      <ThreadPrimitive.ScrollToBottom asChild>
        <Button type="button" variant="outline" size="icon" className="absolute bottom-6 left-1/2 h-9 w-9 -translate-x-1/2 rounded-full border-slate-200 bg-white shadow-sm disabled:invisible" aria-label="scroll to latest">
          <ArrowDown className="h-4 w-4" />
        </Button>
      </ThreadPrimitive.ScrollToBottom>
    </ThreadPrimitive.Root>
  );
}

export type ThreadScrollMetrics = Pick<HTMLElement, "scrollTop" | "clientHeight" | "scrollHeight">;

export function isNearThreadBottom(metrics: ThreadScrollMetrics, thresholdPx = 96) {
  return metrics.scrollHeight - metrics.scrollTop - metrics.clientHeight <= thresholdPx;
}

function aiopsThreadScrollSignature(state: AiopsTransportState) {
  const currentTurn = state.currentTurnId ? state.turns[state.currentTurnId] : undefined;
  const blocks = currentTurn ? orderedAssistantTurnBlocks(currentTurn) : [];
  const lastBlock = blocks[blocks.length - 1];
  return [state.seq, state.currentTurnId || "", currentTurn?.status || "", blocks.length, lastBlock?.id || "", lastBlock?.status || "", lastBlock?.text?.length || 0, lastBlock?.outputPreview?.length || 0].join(":");
}

function UserMessage() {
  const message = useMessage();
  return (
    <MessagePrimitive.Root className="flex justify-end px-1">
      <UserMessageBubble text={messageText(message.content)} />
    </MessagePrimitive.Root>
  );
}

export function UserMessageBubble({ text }: { text: string }) {
  const [copied, setCopied] = useState(false);

  async function copyMessage() {
    await copyUserMessageText(text);
    setCopied(true);
  }

  return (
    <div className="group/user-message relative max-w-[82%] whitespace-pre-wrap break-words rounded-[1.2rem] bg-[#f4f4f4] px-4 py-2.5 text-[15px] leading-7 text-slate-950">
      <button
        type="button"
        data-testid="user-message-copy-button"
        data-copied={copied ? "true" : "false"}
        aria-label={copied ? "已复制消息" : "复制消息"}
        title={copied ? "已复制" : "复制"}
        className="absolute -left-9 top-1/2 flex h-7 w-7 -translate-y-1/2 items-center justify-center rounded-full border border-slate-200 bg-white text-slate-500 opacity-0 shadow-sm transition hover:bg-slate-50 hover:text-slate-900 focus:opacity-100 focus:outline-none focus:ring-2 focus:ring-sky-200 group-hover/user-message:opacity-100"
        onClick={() => {
          void copyMessage();
        }}
      >
        <Copy className="h-3.5 w-3.5" aria-hidden="true" />
      </button>
      {renderUserMessageText(text)}
    </div>
  );
}

type UserMessageMention = HostMentionCandidate | SpecialAiMentionCandidate;

type UserMessageSegment = { type: "text"; text: string; key: string } | { type: "mention"; text: string; key: string; mention: UserMessageMention };

function renderUserMessageText(text: string) {
  const segments = buildUserMessageSegments(text);
  return segments.map((segment) =>
    segment.type === "mention" ? (
      <span key={segment.key} data-testid={segment.mention.source === "ai_tool" ? "user-message-special-mention" : "user-message-host-mention"} className={segment.mention.source === "ai_tool" ? "inline-flex items-center gap-1 rounded-md bg-blue-50 px-1.5 py-0.5 text-[0.92em] font-medium leading-5 text-blue-700" : "inline-flex items-center gap-1 rounded-md border border-sky-100 bg-sky-50 px-1.5 py-0.5 text-[0.92em] font-medium leading-5 text-sky-700"}>
        <UserMessageMentionIcon mention={segment.mention} />
        <span>{userMessageMentionLabel(segment.text, segment.mention)}</span>
      </span>
    ) : (
      <span key={segment.key}>{segment.text}</span>
    ),
  );
}

function buildUserMessageSegments(text: string): UserMessageSegment[] {
  const segments: UserMessageSegment[] = [];
  const mentions: UserMessageMention[] = [...parseHostMentionCandidates(text), ...parseSpecialAiMentionCandidates(text)].sort((a, b) => a.start - b.start || a.end - b.end);
  let cursor = 0;
  for (const mention of mentions) {
    if (mention.start < cursor || mention.end > text.length) {
      continue;
    }
    if (mention.start > cursor) {
      segments.push({
        type: "text",
        text: text.slice(cursor, mention.start),
        key: `text-${cursor}-${mention.start}`,
      });
    }
    segments.push({
      type: "mention",
      text: text.slice(mention.start, mention.end),
      key: `mention-${mention.start}-${mention.end}-${mention.raw}`,
      mention,
    });
    cursor = mention.end;
  }
  if (cursor < text.length) {
    segments.push({
      type: "text",
      text: text.slice(cursor),
      key: `text-${cursor}-${text.length}`,
    });
  }
  return segments.length ? segments : [{ type: "text", text, key: "text-0" }];
}

function UserMessageMentionIcon({ mention }: { mention: UserMessageMention }) {
  const className = "h-3.5 w-3.5 shrink-0";
  if (mention.source !== "ai_tool") {
    return <Server className={className} aria-hidden="true" />;
  }
  if (mention.value === "coroot") {
    return <Activity className={className} aria-hidden="true" />;
  }
  if (mention.value === "ops_graph") {
    return <GitBranch className={className} aria-hidden="true" />;
  }
  if (mention.value === "ops_manuals" || mention.value === "ops_manus") {
    return <BookOpen className={className} aria-hidden="true" />;
  }
  return <Wrench className={className} aria-hidden="true" />;
}

function userMessageMentionLabel(text: string, mention: UserMessageMention) {
  if (mention.source !== "ai_tool") {
    const value = mention.value === "server-local" ? "local" : mention.value;
    return value || text.replace(/^@/, "");
  }
  if (mention.value === "coroot") return "Coroot";
  if (mention.value === "ops_graph") return "OpsGraph";
  if (mention.value === "ops_manuals" || mention.value === "ops_manus") return "运维手册";
  return text.replace(/^@/, "");
}

async function copyUserMessageText(text: string) {
  if (globalThis.navigator?.clipboard?.writeText) {
    await globalThis.navigator.clipboard.writeText(text);
    return;
  }
  const textarea = document.createElement("textarea");
  textarea.value = text;
  textarea.setAttribute("readonly", "");
  textarea.style.position = "fixed";
  textarea.style.left = "-9999px";
  document.body.appendChild(textarea);
  textarea.select();
  document.execCommand?.("copy");
  textarea.remove();
}

function AssistantMessage() {
  const message = useMessage();
  const commands = useAiopsTransportCommands();
  const { turn, blocks } = assistantTranscriptFromContent(message.content);
  const contextStatusEvent = latestContextStatusEvent(turn?.contextGovernance || []);
  const corootArtifacts = blocks.flatMap((block) => (block.artifact && isCorootChartArtifact(block.artifact) ? [block.artifact] : []));
  const hasFinalBlock = blocks.some((block) => block.type === "final_answer");
  const renderGroups = groupAssistantTranscriptBlocks(blocks);
  const hasProcessGroup = renderGroups.some((group) => group.type === "process");

  return (
    <MessagePrimitive.Root className="flex justify-start px-1">
      <div className="w-full space-y-3">
        <ContextStatusNotice event={contextStatusEvent} />
        {!hasProcessGroup && isPendingAssistantTurn(turn?.status) ? <ProcessTranscript process={[]} turnStatus={turn?.status} turnStartedAt={turn?.startedAt} turnCompletedAt={turn?.completedAt} turnUpdatedAt={turn?.updatedAt} renderFinalText={false} /> : null}
        {renderGroups.map((group) => (group.type === "process" ? <ProcessTranscript key={group.id} process={group.blocks} turnStatus={turn?.status} turnStartedAt={turn?.startedAt} turnCompletedAt={turn?.completedAt} turnUpdatedAt={turn?.updatedAt} renderFinalText={false} onApprovalDecision={(approvalId, decision) => commands.approvalDecision(approvalId, decision)} /> : <AssistantTranscriptBlock key={group.block.id} block={group.block} turn={turn!} corootArtifacts={corootArtifacts} hasFinalBlock={hasFinalBlock} onApprovalDecision={(approvalId, decision) => commands.approvalDecision(approvalId, decision)} />))}
      </div>
    </MessagePrimitive.Root>
  );
}

type AssistantTurnEnvelope = Pick<AiopsTransportTurn, "id" | "status" | "startedAt" | "completedAt" | "updatedAt" | "contextGovernance">;

export function assistantTranscriptFromContent(content: readonly unknown[]) {
  let turn: AssistantTurnEnvelope | undefined;
  const blocks: AiopsTransportBlock[] = [];
  for (const part of content) {
    if (!part || typeof part !== "object" || (part as { type?: string }).type !== "data") {
      continue;
    }
    const dataPart = part as { name?: string; data?: unknown };
    if (dataPart.name === AIOPS_TURN_DATA_PART && dataPart.data && typeof dataPart.data === "object") {
      turn = dataPart.data as AssistantTurnEnvelope;
    }
    if (dataPart.name === AIOPS_BLOCK_DATA_PART && dataPart.data && typeof dataPart.data === "object") {
      blocks.push(dataPart.data as AiopsTransportBlock);
    }
  }
  return { turn, blocks: mergeAssistantArtifactRuns(blocks) };
}

export function mergeAssistantArtifactRuns(blocks: AiopsTransportBlock[]): AiopsTransportBlock[] {
  const merged: AiopsTransportBlock[] = [];
  for (let index = 0; index < blocks.length;) {
    if (blocks[index]?.type !== "artifact" || !blocks[index]?.artifact) {
      merged.push(blocks[index]);
      index += 1;
      continue;
    }
    let end = index;
    const run: AiopsTransportBlock[] = [];
    while (end < blocks.length && blocks[end]?.type === "artifact" && blocks[end]?.artifact) {
      run.push(blocks[end]);
      end += 1;
    }
    const artifacts = mergeOpsManualSearchAndPreflightArtifacts(run.map((block) => block.artifact!));
    for (let artifactIndex = 0; artifactIndex < artifacts.length; artifactIndex += 1) {
      const artifact = artifacts[artifactIndex];
      const base = run[Math.min(artifactIndex, run.length - 1)];
      merged.push({
        ...base,
        text: artifact.summaryZh || artifact.summary || artifact.titleZh || artifact.title || base.text,
        artifact,
      });
    }
    index = end;
  }
  return merged;
}

function AssistantTranscriptBlock({ block, turn, corootArtifacts, hasFinalBlock, onApprovalDecision }: { block: AiopsTransportBlock; turn: AssistantTurnEnvelope; corootArtifacts: AiopsTransportAgentUiArtifact[]; hasFinalBlock: boolean; onApprovalDecision: (approvalId: string, decision: "accept" | "reject") => void }) {
  if (block.type === "artifact" && block.artifact) {
    if (!isTerminalAssistantTurn(turn.status) && isDelayedArtifact(block.artifact)) {
      return <AnswerDocumentRenderer finalText="" artifacts={[]} deferredArtifacts={[block.artifact]} />;
    }
    if (isCorootChartArtifact(block.artifact) && hasFinalBlock) {
      return null;
    }
    return <AgentUiArtifactPart artifact={block.artifact} />;
  }
  if (block.type === "final_answer") {
    const finalText = assistantMessageRenderedFinalText([], { finalText: block.text });
    return <AnswerDocumentRenderer finalText={finalText} artifacts={isTerminalAssistantTurn(turn.status) ? corootArtifacts : []} deferredArtifacts={[]} />;
  }
  if (!shouldRenderProcessBlock(block)) {
    return null;
  }
  return <ProcessTranscript process={[block]} turnStatus={turn.status} turnStartedAt={turn.startedAt} turnCompletedAt={turn.completedAt} turnUpdatedAt={turn.updatedAt} renderFinalText={false} onApprovalDecision={onApprovalDecision} />;
}

export function orderedAssistantTurnBlocks(turn: AiopsTransportTurn): AiopsTransportBlock[] {
  return (turn.blockOrder || []).flatMap((id) => {
    const block = turn.blocksById?.[id];
    return block ? [block] : [];
  });
}

type AssistantTranscriptRenderGroup = { type: "process"; id: string; blocks: AiopsTransportBlock[] } | { type: "block"; block: AiopsTransportBlock };

export function groupAssistantTranscriptBlocks(blocks: AiopsTransportBlock[]): AssistantTranscriptRenderGroup[] {
  const groups: AssistantTranscriptRenderGroup[] = [];
  let processBlocks: AiopsTransportBlock[] = [];
  const flushProcess = () => {
    if (processBlocks.length === 0) {
      return;
    }
    groups.push({
      type: "process",
      id: `process:${processBlocks.map((block) => block.id).join(":")}`,
      blocks: processBlocks,
    });
    processBlocks = [];
  };

  for (const block of blocks) {
    if (block.type === "artifact" || block.type === "final_answer") {
      flushProcess();
      groups.push({ type: "block", block });
      continue;
    }
    if (shouldRenderProcessBlock(block)) {
      processBlocks.push(block);
    }
  }
  flushProcess();
  return groups;
}

function isTerminalAssistantTurn(status: AiopsTransportTurn["status"]) {
  return status === "completed" || status === "failed" || status === "canceled";
}

function isDelayedArtifact(artifact: AiopsTransportAgentUiArtifact) {
  return artifact.type === "ops_manual_search_result" || artifact.type === "coroot_chart";
}

function latestContextStatusEvent(events: AiopsContextGovernanceEvent[]) {
  const candidates = events.filter((event) => isVisibleContextStatusEvent(event));
  return candidates[candidates.length - 1] || null;
}

function isVisibleContextStatusEvent(event: AiopsContextGovernanceEvent) {
  const kind = (event.kind || "").toLowerCase();
  return kind.includes("context.compaction") || kind === "context.small_context.enabled";
}

function isCorootChartArtifact(value: unknown) {
  return Boolean(value && typeof value === "object" && (value as { type?: string }).type === "coroot_chart");
}

function isPendingAssistantTurn(turnStatus?: string) {
  return turnStatus === "submitted" || turnStatus === "working" || turnStatus === "blocked";
}

function shouldRenderProcessBlock(block: AiopsProcessBlock) {
  if (block.kind !== "reasoning") {
    return true;
  }

  const text = (block.text || "").trim().toLowerCase();
  if (!text) {
    return false;
  }

  return text !== "model response received";
}

function McpSurfaceList({ surfaces }: { surfaces: AiopsTransportMcpSurface[] }) {
  if (surfaces.length === 0) {
    return null;
  }
  return (
    <div className="grid gap-2 md:grid-cols-2">
      {surfaces.map((surface) => (
        <McpSurfacePart key={surface.id} surface={surface} />
      ))}
    </div>
  );
}

function messageText(content: readonly { type: string; text?: string }[]) {
  return content
    .filter((part) => part.type === "text")
    .map((part) => part.text || "")
    .join("");
}

export function assistantMessageRenderedFinalText(
  content: readonly { type: string; text?: string }[],
  meta: { finalText?: string },
) {
  if (typeof meta.finalText === "string") {
    return sanitizeAssistantDisplayText(meta.finalText);
  }
  return sanitizeAssistantDisplayText(messageText(content));
}

function sanitizeAssistantDisplayText(text: string) {
  return text.trim();
}
