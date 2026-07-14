import { MessagePrimitive, ThreadPrimitive, useAssistantTransportState, useMessage } from "@assistant-ui/react";
import { Activity, ArrowDown, BookOpen, Bot, Copy, GitBranch, LoaderCircle, Server, Wrench } from "lucide-react";
import { useLayoutEffect, useMemo, useRef, useState } from "react";

import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import { Button } from "@/components/ui/button";
import type { AiopsContextGovernanceEvent, AiopsProcessBlock, AiopsTransportAgentUiArtifact, AiopsTransportBlock, AiopsTransportFinal, AiopsTransportFinalStatus, AiopsTransportMcpSurface, AiopsTransportState, AiopsTransportTimelineItem, AiopsTransportTurn } from "@/transport/aiopsTransportTypes";
import { AIOPS_BLOCK_DATA_PART, AIOPS_TURN_DATA_PART } from "@/transport/aiopsTransportConverter";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

import { parseHostMentionCandidates, parseSpecialAiMentionCandidates, type HostMentionCandidate, type SpecialAiMentionCandidate } from "../hostMentions";
import { AnswerDocumentRenderer } from "./AnswerDocumentRenderer";
import { ContextStatusNotice } from "./ContextStatusNotice";
import { McpSurfacePart } from "./McpSurfacePart";
import { MessageMarkdown } from "./MessageMarkdown";
import { ProcessTranscript } from "./ProcessTranscript";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

type AssistantMessageMeta = {
  process?: AiopsProcessBlock[];
  contextGovernance?: AiopsContextGovernanceEvent[];
  agentUiArtifacts?: unknown[];
  deferredAgentUiArtifacts?: unknown[];
  intent?: { text?: string; status?: string } | null;
  userText?: string;
  turnStatus?: string;
  turnStartedAt?: string;
  turnCompletedAt?: string;
  turnUpdatedAt?: string;
  finalDurationMs?: number;
  finalText?: string;
  finalStatus?: AiopsTransportFinalStatus;
  finalConfidence?: string;
  finalContract?: Partial<AiopsTransportFinal>;
  timeline?: AiopsTransportTimelineItem[];
};

export function AiopsThread() {
  const state = useAssistantTransportState() as AiopsTransportState;
  const surfaces = Object.values(state.mcpSurfaces || {});
  const target = useSessionTargetContext();
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
  const finalDurationMs = blocks.find((block) => block.type === "final_answer")?.durationMs;
  const renderGroups = groupAssistantTranscriptBlocks(blocks);

  return (
    <MessagePrimitive.Root className="flex justify-start px-1">
      <div className="w-full space-y-3">
        <ContextStatusNotice event={contextStatusEvent} />
        {renderGroups.map((group) => (group.type === "process" ? <ProcessTranscript key={group.id} process={group.blocks} turnStatus={turn?.status} turnStartedAt={turn?.startedAt} turnCompletedAt={turn?.completedAt} turnUpdatedAt={turn?.updatedAt} finalDurationMs={finalDurationMs} renderFinalText={false} onApprovalDecision={(approvalId, decision) => commands.approvalDecision(approvalId, decision)} /> : <AssistantTranscriptBlock key={group.block.id} block={group.block} turn={turn!} corootArtifacts={corootArtifacts} hasFinalBlock={hasFinalBlock} onApprovalDecision={(approvalId, decision) => commands.approvalDecision(approvalId, decision)} />))}
        {blocks.length === 0 && isPendingAssistantTurn(turn?.status) ? <ProcessTranscript process={[]} turnStatus={turn?.status} turnStartedAt={turn?.startedAt} turnCompletedAt={turn?.completedAt} turnUpdatedAt={turn?.updatedAt} renderFinalText={false} /> : null}
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
  return { turn, blocks };
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
    const contract = block.finalContract;
    const meta: AssistantMessageMeta = {
      finalText: block.text,
      finalStatus: contract?.status,
      finalConfidence: contract?.confidence,
      finalContract: contract,
    };
    const finalText = assistantMessageRenderedFinalText([], meta);
    return (
      <>
        <FinalContractSummary meta={meta} />
        <AnswerDocumentRenderer finalText={finalText} artifacts={isTerminalAssistantTurn(turn.status) ? corootArtifacts : []} deferredArtifacts={[]} />
      </>
    );
  }
  if (!shouldRenderProcessBlock(block)) {
    return null;
  }
  return <ProcessTranscript process={[block]} turnStatus={turn.status} turnStartedAt={turn.startedAt} turnCompletedAt={turn.completedAt} turnUpdatedAt={turn.updatedAt} finalDurationMs={block.durationMs} renderFinalText={false} onApprovalDecision={onApprovalDecision} />;
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
  meta: Pick<AssistantMessageMeta, "finalText">,
) {
  if (typeof meta.finalText === "string") {
    return sanitizeAssistantDisplayText(meta.finalText);
  }
  return sanitizeAssistantDisplayText(messageText(content));
}

type FinalContractSummaryInput = Pick<
  AssistantMessageMeta,
  "finalText" | "finalStatus" | "finalConfidence" | "finalContract"
>;

type FinalContractSummaryModel = {
  status: AiopsTransportFinalStatus;
  statusLabel: string;
  confidenceLabel?: string;
  evidenceLabel?: string;
  uncheckedRequirements: string[];
  failedToolImpacts: string[];
  limitations: string[];
};

function FinalContractSummary({ meta }: { meta: FinalContractSummaryInput }) {
  const summary = finalContractSummaryView(meta);
  if (!summary) {
    return null;
  }
  return (
    <div
      className="space-y-1 rounded-md border border-slate-200 bg-white px-3 py-2 text-[13px] leading-6 text-slate-600"
      data-testid="aiops-final-contract-summary"
      data-final-status={summary.status}
    >
      <div className="flex flex-wrap items-center gap-2">
        <span className={finalContractStatusClass(summary.status)}>{summary.statusLabel}</span>
        {summary.confidenceLabel ? <span className="text-slate-400">{summary.confidenceLabel}</span> : null}
      </div>
      {summary.evidenceLabel ? <div className="break-words">{summary.evidenceLabel}</div> : null}
      {summary.failedToolImpacts.length ? (
        <div className="break-words">工具限制：{summary.failedToolImpacts.join("；")}</div>
      ) : null}
      {summary.uncheckedRequirements.length ? (
        <div className="break-words">未完成检查：{summary.uncheckedRequirements.join("，")}</div>
      ) : null}
      {summary.limitations.length ? (
        <div className="break-words">限制：{summary.limitations.join("，")}</div>
      ) : null}
    </div>
  );
}

export function finalContractSummaryView(meta: FinalContractSummaryInput): FinalContractSummaryModel | null {
  const contract = meta.finalContract || {};
  const status = normalizeFinalStatus(contract.status || meta.finalStatus);
  const checkedEvidenceRefs = stringList(contract.checkedEvidenceRefs);
  const uncheckedRequirements = userVisibleStringList(contract.uncheckedRequirements);
  const failedToolImpacts = failedToolImpactList(contract.failedToolImpacts);
  const limitations = userVisibleStringList(contract.limitations);
  const confidence = String(contract.confidence || meta.finalConfidence || "").trim();

  if (
    !contract.schemaVersion &&
    !status &&
    !confidence &&
    !checkedEvidenceRefs.length &&
    !uncheckedRequirements.length &&
    !failedToolImpacts.length &&
    !limitations.length
  ) {
    return null;
  }

  const normalizedStatus = status || "unknown";
  const evidenceCount = checkedEvidenceRefs.length;
  const hasUserActionableDetails =
    evidenceCount > 0 ||
    uncheckedRequirements.length > 0 ||
    failedToolImpacts.length > 0 ||
    limitations.length > 0;
  const hasSummaryDetails =
    Boolean(confidence) ||
    hasUserActionableDetails;
  if (normalizedStatus === "unknown" && !hasUserActionableDetails) {
    return null;
  }
  if (
    !contract.schemaVersion &&
    (normalizedStatus === "running" || normalizedStatus === "completed") &&
    !hasSummaryDetails
  ) {
    return null;
  }
  if (
    !contract.schemaVersion &&
    normalizedStatus === "failed" &&
    Boolean(String(meta.finalText || "").trim()) &&
    !hasSummaryDetails
  ) {
    return null;
  }

  return {
    status: normalizedStatus,
    statusLabel: finalStatusLabel(normalizedStatus),
    confidenceLabel: finalConfidenceLabel(confidence),
    evidenceLabel: evidenceCount > 0 ? `已采集 ${evidenceCount} 条证据` : undefined,
    uncheckedRequirements,
    failedToolImpacts,
    limitations,
  };
}

function normalizeFinalStatus(status?: string): AiopsTransportFinalStatus | undefined {
  switch (status) {
    case "running":
    case "completed":
    case "failed":
    case "verified":
    case "partial":
    case "blocked":
    case "needs_evidence":
    case "approval_denied":
    case "tool_unavailable":
    case "cancelled":
    case "unknown":
      return status;
    default:
      return status ? "unknown" : undefined;
  }
}

function finalStatusLabel(status: AiopsTransportFinalStatus) {
  switch (status) {
    case "verified":
      return "已验证";
    case "partial":
      return "部分确认";
    case "blocked":
      return "已阻止";
    case "needs_evidence":
      return "证据不足";
    case "approval_denied":
      return "审批拒绝";
    case "tool_unavailable":
      return "工具不可用";
    case "cancelled":
      return "已取消";
    case "failed":
      return "失败";
    case "running":
      return "生成中";
    case "completed":
      return "已完成";
    default:
      return "未确认";
  }
}

function finalContractStatusClass(status: AiopsTransportFinalStatus) {
  switch (status) {
    case "verified":
      return "font-medium text-emerald-700";
    case "partial":
    case "needs_evidence":
      return "font-medium text-amber-700";
    case "blocked":
    case "approval_denied":
    case "tool_unavailable":
    case "failed":
      return "font-medium text-red-700";
    default:
      return "font-medium text-slate-700";
  }
}

function finalConfidenceLabel(confidence: string) {
  switch (confidence) {
    case "high":
      return "置信度高";
    case "medium":
      return "置信度中";
    case "low":
      return "置信度低";
    default:
      return confidence ? `置信度${confidence}` : undefined;
  }
}

const knownDiagnosticReplacements = [
  {
    old: "required evidence may be missing; do not use this failed tool as checked evidence",
    value: "证据读取失败，不能作为已检查结果",
  },
  { old: "read_context_artifact", value: "读取上下文证据" },
  { old: "read_mcp_resource", value: "读取 MCP 资源" },
  { old: "list_mcp_resources", value: "列出 MCP 资源" },
  { old: "coroot.collect_rca_context", value: "Coroot 根因分析上下文" },
  { old: "coroot_collect_rca_context", value: "Coroot 根因分析上下文" },
  { old: "coroot.list_services", value: "Coroot 服务列表" },
  { old: "coroot_list_services", value: "Coroot 服务列表" },
  { old: "coroot.incidents", value: "Coroot 异常事件" },
  { old: "coroot_incidents", value: "Coroot 异常事件" },
  { old: "tool_business_error", value: "工具执行失败" },
] as const;

const leakedToolProcessTextMessage = "已读取工具证据，但模型返回的是工具读取过程，未形成可直接展示的中文结论。";

function sanitizeAssistantDisplayText(text: string) {
  if (!text) {
    return text;
  }
  if (looksLikeLeakedToolProcessText(text)) {
    return leakedToolProcessTextMessage;
  }
  const seenLines = new Set<string>();
  let inFence = false;
  const lines = text.split(/\r?\n/).flatMap((line) => {
    if (line.trimStart().startsWith("```")) {
      inFence = !inFence;
      return [line];
    }
    if (inFence) {
      return [line];
    }
    const structuredLine = readableStructuredEvidenceLine(line);
    const nextLine = structuredLine || humanizeUserVisibleDiagnostic(line);
    if (!nextLine.trim()) {
      return [nextLine];
    }
    const duplicateKey = nextLine.trim();
    if (seenLines.has(duplicateKey)) {
      return [];
    }
    seenLines.add(duplicateKey);
    return [nextLine];
  });
  return lines.join("\n").trim();
}

function looksLikeLeakedToolProcessText(text: string) {
  const lower = text.trim().toLowerCase();
  if (!lower) {
    return false;
  }
  if ((lower.includes("service_name=") || lower.includes("ervice_name=")) && (lower.includes("rca上下文") || lower.includes("让我获取") || lower.includes("let me get"))) {
    return true;
  }
  if ((lower.match(/让我获取rca上下文/g) ?? []).length >= 2 || (lower.match(/rca上下文/g) ?? []).length >= 5) {
    return true;
  }
  if (lower.includes("read_context_artifact")) {
    const linkedMarkers = [
      "evidence ids",
      "evidence id",
      "evidence refs",
      "evidence ref",
      "证据引用",
      "let me",
      "try reading",
      "reading the evidence",
      "spill chain",
      "store://tool-spills",
    ];
    if (linkedMarkers.some((marker) => lower.includes(marker))) {
      return true;
    }
  }
  const processMarkers = [
    "let me try reading",
    "try reading the evidence",
    "reading the evidence refs",
    "evidence ids",
    "evidence refs",
    "one more level of the spill chain",
    "store://tool-spills",
  ];
  return processMarkers.filter((marker) => lower.includes(marker)).length >= 2;
}

function readableStructuredEvidenceLine(line: string) {
  const match = line.match(/^(\s*(?:(?:[-*•])|\d+\.)\s*)([{[].*)$/);
  if (!match) {
    return null;
  }
  const label = structuredEvidenceLabel(match[2]);
  if (!label) {
    return null;
  }
  return `${match[1]}${label}已返回结构化证据。`;
}

function structuredEvidenceLabel(raw: string) {
  const text = raw.trim();
  if (!text.startsWith("{") && !text.startsWith("[")) {
    return null;
  }
  if (text.includes('"categoryCounts"')) {
    return "Coroot 服务概览";
  }
  if (text.includes('"incidents"')) {
    return "Coroot 异常事件";
  }
  if (text.includes('"abnormalServices"') || text.includes('"service"')) {
    return "Coroot 服务证据";
  }
  if (text.includes('"evidenceRefs"')) {
    return "结构化证据";
  }
  return null;
}

function humanizeKnownToolName(value: string) {
  let out = value;
  for (const replacement of knownDiagnosticReplacements) {
    if (replacement.old.includes(".") || replacement.old.includes("_")) {
      out = out.split(replacement.old).join(replacement.value);
    }
  }
  return out;
}

function humanizeUserVisibleDiagnostic(value: string) {
  let out = value;
  for (const replacement of knownDiagnosticReplacements) {
    out = out.split(replacement.old).join(replacement.value);
  }
  out = out.replace(/(读取 MCP 资源|列出 MCP 资源|Coroot [^:：\n]+):\s*/g, "$1：");
  return out;
}

function stringList(value: unknown) {
  return Array.isArray(value)
    ? value.map((item) => String(item || "").trim()).filter(Boolean)
    : [];
}

function userVisibleStringList(value: unknown) {
  return uniqueStringList(stringList(value).map(humanizeUserVisibleDiagnostic));
}

function failedToolImpactList(value: unknown) {
  if (!Array.isArray(value)) {
    return [];
  }
  return uniqueStringList(value.flatMap((item) => {
    if (!item || typeof item !== "object") {
      return [];
    }
    const record = item as Record<string, unknown>;
    const rawName = String(record.toolName || record.toolCallId || "工具").trim();
    const name = humanizeKnownToolName(rawName);
    const detail = humanizeUserVisibleDiagnostic(String(record.impact || record.failureClass || "影响未知").trim());
    const separator = name !== rawName ? "：" : ": ";
    return name || detail ? [`${name}${separator}${detail}`] : [];
  }));
}

function uniqueStringList(values: string[]) {
  const out: string[] = [];
  const seen = new Set<string>();
  for (const value of values) {
    const trimmed = value.trim();
    if (!trimmed || seen.has(trimmed)) {
      continue;
    }
    seen.add(trimmed);
    out.push(trimmed);
  }
  return out;
}
