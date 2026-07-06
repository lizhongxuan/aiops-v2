import { MessagePrimitive, ThreadPrimitive, useAssistantTransportState, useMessage } from "@assistant-ui/react";
import { Activity, ArrowDown, BookOpen, Bot, Copy, GitBranch, LoaderCircle, Server, Wrench } from "lucide-react";
import { useLayoutEffect, useMemo, useRef, useState } from "react";

import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import { Button } from "@/components/ui/button";
import type { AgentRunView, AiopsContextGovernanceEvent, AiopsProcessBlock, AiopsTransportAgentUiArtifact, AiopsTransportFinal, AiopsTransportFinalStatus, AiopsTransportMcpSurface, AiopsTransportState, AiopsTransportTimelineItem } from "@/transport/aiopsTransportTypes";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

import { parseHostMentionCandidates, parseSpecialAiMentionCandidates, type HostMentionCandidate, type SpecialAiMentionCandidate } from "../hostMentions";
import { AnswerDocumentRenderer } from "./AnswerDocumentRenderer";
import { ContextStatusNotice } from "./ContextStatusNotice";
import { McpSurfacePart } from "./McpSurfacePart";
import { MessageMarkdown } from "./MessageMarkdown";
import { ProcessTranscript } from "./ProcessTranscript";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";
import { SpecialInputContextBar } from "./SpecialInputContextBar";

type AssistantMessageMeta = {
  process?: AiopsProcessBlock[];
  agentRun?: AgentRunView;
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
  const commands = useAiopsTransportCommands();
  const surfaces = Object.values(state.mcpSurfaces || {});
  const target = useSessionTargetContext();
  const workspace = useSessionWorkspaceContext();
  const viewportRef = useRef<HTMLDivElement | null>(null);
  const stickToBottomRef = useRef(true);
  const scrollSignature = useMemo(() => aiopsThreadScrollSignature(state), [state]);

  useLayoutEffect(() => {
    const viewport = viewportRef.current;
    if (!viewport || !stickToBottomRef.current) {
      return undefined;
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
                  <h1 className="mt-5 text-2xl font-semibold text-slate-950">
                    {workspace.kind === "workspace" ? "今天要统筹什么运维任务？" : "Hello there"}
                  </h1>
                  <p className="mx-auto mt-2 max-w-xl text-sm leading-6 text-slate-500">
                    {workspace.kind === "workspace"
                      ? "主 Agent 会保留工作台会话，并通过 AssistantTransport 编排后端 host agent。"
                      : "输入排障、巡检或变更任务，消息会进入 AI Chat 会话。"}
                  </p>
                </div>
              )}
            </div>
          </ThreadPrimitive.Empty>
          <SpecialInputContextBar
            context={state.specialInputContext}
            onClear={() => commands.specialInputClear({
              resourceKind: state.specialInputContext?.activeGrant?.resourceKind,
              resourceId: state.specialInputContext?.activeGrant?.resourceId,
              canonicalKey: state.specialInputContext?.activeGrant?.canonicalKey,
            })}
            onConfirm={() => commands.specialInputConfirm({
              resourceKind: state.specialInputContext?.candidateFacts?.[0]?.resourceKind,
              resourceId: state.specialInputContext?.candidateFacts?.[0]?.resourceId,
              canonicalKey: state.specialInputContext?.candidateFacts?.[0]?.canonicalKey,
            })}
          />
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
        <Button
          type="button"
          variant="outline"
          size="icon"
          className="absolute bottom-6 left-1/2 h-9 w-9 -translate-x-1/2 rounded-full border-slate-200 bg-white shadow-sm disabled:invisible"
          aria-label="scroll to latest"
        >
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
  const currentFinal = currentTurn?.final?.text || "";
  const currentProcess = currentTurn?.process || [];
  const lastProcess = currentProcess[currentProcess.length - 1];
  return [
    state.seq,
    state.currentTurnId || "",
    currentTurn?.status || "",
    currentFinal.length,
    currentProcess.length,
    lastProcess?.id || "",
    lastProcess?.status || "",
    lastProcess?.text?.length || 0,
    lastProcess?.outputPreview?.length || 0,
  ].join(":");
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

type UserMessageSegment =
  | { type: "text"; text: string; key: string }
  | { type: "mention"; text: string; key: string; mention: UserMessageMention };

function renderUserMessageText(text: string) {
  const segments = buildUserMessageSegments(text);
  return segments.map((segment) =>
    segment.type === "mention" ? (
      <span
        key={segment.key}
        data-testid={segment.mention.source === "ai_tool" ? "user-message-special-mention" : "user-message-host-mention"}
        className={
          segment.mention.source === "ai_tool"
            ? "inline-flex items-center gap-1 rounded-md bg-blue-50 px-1.5 py-0.5 text-[0.92em] font-medium leading-5 text-blue-700"
            : "inline-flex items-center gap-1 rounded-md border border-sky-100 bg-sky-50 px-1.5 py-0.5 text-[0.92em] font-medium leading-5 text-sky-700"
        }
      >
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
  const mentions: UserMessageMention[] = [
    ...parseHostMentionCandidates(text),
    ...parseSpecialAiMentionCandidates(text),
  ].sort((a, b) => a.start - b.start || a.end - b.end);
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
  const meta = (message.metadata?.unstable_state || {}) as AssistantMessageMeta;
  const process = (meta.process || []).filter(shouldRenderProcessBlock);
  const agentSteps = meta.agentRun?.steps || [];
  const contextStatusEvent = latestContextStatusEvent(meta.contextGovernance || []);
  const artifacts = (meta.agentUiArtifacts || []) as AiopsTransportAgentUiArtifact[];
  const corootArtifacts = artifacts.filter(isCorootChartArtifact);
  const otherArtifacts = artifacts.filter((artifact) => !isCorootChartArtifact(artifact));
  const deferredArtifacts = (meta.deferredAgentUiArtifacts || []) as AiopsTransportAgentUiArtifact[];
  const finalText = assistantMessageRenderedFinalText(message.content, meta);

  return (
    <MessagePrimitive.Root className="flex justify-start px-1">
      <div className="w-full space-y-3">
        <ContextStatusNotice event={contextStatusEvent} />
        {process.length > 0 || agentSteps.length > 0 || isPendingAssistantTurn(meta.turnStatus) ? (
          <ProcessTranscript
            process={process}
            agentSteps={agentSteps}
            turnStatus={meta.turnStatus}
            turnStartedAt={meta.turnStartedAt}
            turnCompletedAt={meta.turnCompletedAt}
            turnUpdatedAt={meta.turnUpdatedAt}
            finalDurationMs={meta.finalDurationMs}
            finalText={finalText}
            renderFinalText={false}
            onApprovalDecision={(approvalId, decision) => commands.approvalDecision(approvalId, decision)}
          />
        ) : null}
        {finalText || artifacts.length || deferredArtifacts.length ? (
          <>
            <FinalContractSummary meta={meta} />
            <AnswerDocumentRenderer
              finalText={finalText}
              artifacts={corootArtifacts}
              deferredArtifacts={deferredArtifacts}
            />
          </>
        ) : null}
        {otherArtifacts.length ? (
          <div className="grid gap-2">
            {otherArtifacts.map((artifact) => (
              <AgentUiArtifactPart key={artifact.id} artifact={artifact} />
            ))}
          </div>
        ) : null}
      </div>
    </MessagePrimitive.Root>
  );
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
    return meta.finalText;
  }
  return messageText(content);
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
  const uncheckedRequirements = stringList(contract.uncheckedRequirements);
  const failedToolImpacts = failedToolImpactList(contract.failedToolImpacts);
  const limitations = stringList(contract.limitations);
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

function stringList(value: unknown) {
  return Array.isArray(value)
    ? value.map((item) => String(item || "").trim()).filter(Boolean)
    : [];
}

function failedToolImpactList(value: unknown) {
  if (!Array.isArray(value)) {
    return [];
  }
  return value.flatMap((item) => {
    if (!item || typeof item !== "object") {
      return [];
    }
    const record = item as Record<string, unknown>;
    const name = String(record.toolName || record.toolCallId || "工具").trim();
    const detail = String(record.impact || record.failureClass || "影响未知").trim();
    return name || detail ? [`${name}: ${detail}`] : [];
  });
}
