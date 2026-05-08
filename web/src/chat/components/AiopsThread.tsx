import { MessagePrimitive, ThreadPrimitive, useAssistantTransportState, useMessage } from "@assistant-ui/react";
import { ArrowDown, Bot } from "lucide-react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";
import type { AiopsTransportMcpSurface, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { AiopsTranscript, type AiopsTranscriptBlock } from "./AiopsTranscript";
import { McpSurfacePart } from "./McpSurfacePart";
import { MessageMarkdown } from "./MessageMarkdown";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";
import { useSmartScrollAnchor } from "./useSmartScrollAnchor";

type AssistantMessageMeta = {
  turnId?: string;
  turnStatus?: string;
  turnStartedAt?: string;
  turnCompletedAt?: string;
  turnUpdatedAt?: string;
  blocks: AiopsTranscriptBlock[];
  blockOrder?: string[];
  blocksById?: Record<string, AiopsTranscriptBlock | undefined>;
};

type AssistantMetadataMessage = {
  metadata?: {
    custom?: {
      aiops?: Partial<AssistantMessageMeta>;
    };
  };
};

export function AiopsThread() {
  const state = useAssistantTransportState() as AiopsTransportState;
  const surfaces = Object.values(state.mcpSurfaces || {});
  const target = useSessionTargetContext();
  const workspace = useSessionWorkspaceContext();
  const scrollAnchor = useSmartScrollAnchor([state.seq, state.updatedAt, state.currentTurnId]);

  return (
    <ThreadPrimitive.Root className="relative h-full min-h-0 bg-white">
      <ThreadPrimitive.Viewport
        ref={scrollAnchor.scrollRef}
        onScroll={scrollAnchor.handleScroll}
        onWheel={scrollAnchor.handleWheel}
        className="h-full overflow-y-auto scroll-smooth"
      >
        <div className="mx-auto flex min-h-full w-full max-w-3xl flex-col px-4 py-6 md:px-6">
          <ThreadPrimitive.Empty>
            <div className="flex min-h-full flex-1 items-center justify-center pb-10">
              <div className="w-full text-center">
                <div className="mx-auto flex h-12 w-12 items-center justify-center rounded-full border border-slate-200 bg-white text-slate-900 shadow-sm">
                  <Bot className="h-5 w-5" />
                </div>
                <h1 className="mt-5 text-2xl font-semibold tracking-tight text-slate-950">
                  {workspace.kind === "workspace" ? "今天要统筹什么运维任务？" : `要对 ${target.targetLabel} 做什么？`}
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
      <Button
        type="button"
        variant="outline"
        size="icon"
        className={cn(
          "absolute bottom-6 left-1/2 h-9 w-9 -translate-x-1/2 rounded-full border-slate-200 bg-white shadow-sm transition-opacity",
          !scrollAnchor.showScrollToBottom && "pointer-events-none opacity-0",
        )}
        aria-label="scroll to latest"
        onClick={scrollAnchor.scrollToBottom}
      >
        <ArrowDown className="h-4 w-4" />
      </Button>
    </ThreadPrimitive.Root>
  );
}

function UserMessage() {
  const message = useMessage();
  return (
    <MessagePrimitive.Root className="flex justify-end px-1">
      <div className="max-w-[78%] rounded-[1.35rem] bg-[#f4f4f4] px-4 py-2.5 text-[15px] leading-7 text-slate-950">
        <MessageMarkdown text={messageText(message.content)} />
      </div>
    </MessagePrimitive.Root>
  );
}

function AssistantMessage() {
  const message = useMessage();
  const meta = getAssistantAiopsTranscriptMeta(message);

  return (
    <MessagePrimitive.Root className="flex justify-start px-1">
      <div className="w-full">
        <AiopsTranscript
          blocks={meta.blocks}
          blockOrder={meta.blockOrder}
          blocksById={meta.blocksById}
          turnStatus={meta.turnStatus}
          turnStartedAt={meta.turnStartedAt}
          turnCompletedAt={meta.turnCompletedAt}
          turnUpdatedAt={meta.turnUpdatedAt}
        />
      </div>
    </MessagePrimitive.Root>
  );
}

export function getAssistantAiopsTranscriptMeta(message: AssistantMetadataMessage): AssistantMessageMeta {
  const aiops = message.metadata?.custom?.aiops || {};
  const blocks = Array.isArray(aiops.blocks) ? aiops.blocks : [];
  const blocksById = isBlocksById(aiops.blocksById) ? aiops.blocksById : blocksByIdFromBlocks(blocks);
  return {
    turnId: aiops.turnId,
    turnStatus: aiops.turnStatus,
    turnStartedAt: aiops.turnStartedAt,
    turnCompletedAt: aiops.turnCompletedAt,
    turnUpdatedAt: aiops.turnUpdatedAt,
    blocks,
    blockOrder: Array.isArray(aiops.blockOrder) ? aiops.blockOrder : blocks.map((block) => block.id),
    blocksById,
  };
}

function isBlocksById(value: unknown): value is Record<string, AiopsTranscriptBlock | undefined> {
  return Boolean(value && typeof value === "object" && !Array.isArray(value));
}

function blocksByIdFromBlocks(blocks: AiopsTranscriptBlock[]) {
  return Object.fromEntries(blocks.map((block) => [block.id, block]));
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
