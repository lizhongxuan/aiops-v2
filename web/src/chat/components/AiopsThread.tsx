import { MessagePrimitive, ThreadPrimitive, useAssistantTransportState, useMessage } from "@assistant-ui/react";
import { ArrowDown, Bot, LineChart } from "lucide-react";

import { Button } from "@/components/ui/button";
import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import type { AiopsProcessBlock, AiopsTransportMcpSurface, AiopsTransportState } from "@/transport/aiopsTransportTypes";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";

import { McpSurfacePart } from "./McpSurfacePart";
import { MessageMarkdown } from "./MessageMarkdown";
import { ProcessTranscript } from "./ProcessTranscript";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

type AssistantMessageMeta = {
  process?: AiopsProcessBlock[];
  agentUiArtifacts?: unknown[];
  deferredAgentUiArtifacts?: unknown[];
  intent?: { text?: string; status?: string } | null;
  userText?: string;
  turnStatus?: string;
  turnStartedAt?: string;
  turnCompletedAt?: string;
  turnUpdatedAt?: string;
};

export function AiopsThread() {
  const state = useAssistantTransportState() as AiopsTransportState;
  const surfaces = Object.values(state.mcpSurfaces || {});
  const target = useSessionTargetContext();
  const workspace = useSessionWorkspaceContext();

  return (
    <ThreadPrimitive.Root className="relative h-full min-h-0 bg-white">
      <ThreadPrimitive.Viewport autoScroll scrollToBottomOnInitialize className="h-full overflow-y-auto scroll-smooth">
        <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col px-4 py-6 md:px-6">
          <ThreadPrimitive.Empty>
            <div className="flex min-h-full flex-1 items-center justify-center pb-10">
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
                    : "输入排障、巡检或变更任务，消息会进入当前主机会话。"}
                </p>
              </div>
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

function UserMessage() {
  const message = useMessage();
  return (
    <MessagePrimitive.Root className="flex justify-end px-1">
      <div className="max-w-[78%] rounded-[1.35rem] bg-[#f4f4f4] px-4 py-2.5 text-[16px] leading-8 text-slate-950">
        <MessageMarkdown text={messageText(message.content)} />
      </div>
    </MessagePrimitive.Root>
  );
}

function AssistantMessage() {
  const message = useMessage();
  const commands = useAiopsTransportCommands();
  const meta = (message.metadata?.unstable_state || {}) as AssistantMessageMeta;
  const process = (meta.process || []).filter(shouldRenderProcessBlock);
  const artifacts = meta.agentUiArtifacts || [];
  const hasDeferredCorootChart = (meta.deferredAgentUiArtifacts || []).some(isCorootChartArtifact);
  const finalText = messageText(message.content);

  return (
    <MessagePrimitive.Root className="flex justify-start px-1">
      <div className="w-full space-y-3">
        {meta.intent?.text ? (
          <div className="rounded-2xl border border-slate-200 bg-slate-50 px-4 py-3 text-sm text-slate-700">
            {meta.intent.text}
          </div>
        ) : null}
        {process.length > 0 || isPendingAssistantTurn(meta.turnStatus) || finalText ? (
          <ProcessTranscript
            process={process}
            turnStatus={meta.turnStatus}
            turnStartedAt={meta.turnStartedAt}
            turnCompletedAt={meta.turnCompletedAt}
            turnUpdatedAt={meta.turnUpdatedAt}
            finalText={finalText}
            onApprovalDecision={(approvalId, decision) => commands.approvalDecision(approvalId, decision)}
          />
        ) : null}
        {hasDeferredCorootChart ? <DeferredCorootChartNotice /> : null}
        {artifacts.length ? (
          <div className="grid gap-2">
            {artifacts.map((artifact) => (
              <AgentUiArtifactPart key={artifact.id} artifact={artifact} />
            ))}
          </div>
        ) : null}
      </div>
    </MessagePrimitive.Root>
  );
}

function DeferredCorootChartNotice() {
  return (
    <div
      data-testid="coroot-chart-deferred-notice"
      className="flex items-center gap-2 rounded-lg border border-slate-200 bg-slate-50 px-3 py-2 text-sm leading-6 text-slate-600"
    >
      <LineChart className="h-4 w-4 shrink-0 text-slate-400" />
      <span>已生成 Coroot 图表，分析完成后展开</span>
    </div>
  );
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
