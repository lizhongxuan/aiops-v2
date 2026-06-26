import { MessagePrimitive, ThreadPrimitive, useAssistantTransportState, useMessage } from "@assistant-ui/react";
import { ArrowDown, Bot, LoaderCircle } from "lucide-react";
import { useLayoutEffect, useMemo, useRef } from "react";

import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import { Button } from "@/components/ui/button";
import type { AgentRunView, AiopsContextGovernanceEvent, AiopsProcessBlock, AiopsTransportAgentUiArtifact, AiopsTransportMcpSurface, AiopsTransportState } from "@/transport/aiopsTransportTypes";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

import { AnswerDocumentRenderer } from "./AnswerDocumentRenderer";
import { ContextStatusNotice } from "./ContextStatusNotice";
import { McpSurfacePart } from "./McpSurfacePart";
import { MessageMarkdown } from "./MessageMarkdown";
import { ProcessTranscript } from "./ProcessTranscript";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

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
};

export function AiopsThread() {
  const state = useAssistantTransportState() as AiopsTransportState;
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
  return (
    <div className="max-w-[82%] whitespace-pre-wrap break-words rounded-[1.2rem] bg-[#f4f4f4] px-4 py-2.5 text-[15px] leading-7 text-slate-950">
      {text}
    </div>
  );
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
  const finalText = messageText(message.content);

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
          <AnswerDocumentRenderer
            finalText={finalText}
            artifacts={corootArtifacts}
            deferredArtifacts={deferredArtifacts}
          />
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
