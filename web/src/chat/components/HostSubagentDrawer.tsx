import { useEffect, useMemo, useState } from "react";
import {
  AlertCircleIcon,
  BotIcon,
  CircleDashedIcon,
  TerminalIcon,
  UserIcon,
  WrenchIcon,
  XIcon,
  type LucideIcon,
} from "lucide-react";

import { getChildAgentTranscript, type HostChildAgentTranscript, type HostOpsTranscriptItem } from "@/api/hostOps";
import { Button } from "@/components/ui/button";
import { Sheet, SheetClose, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/components/ui/sheet";
import type { AiopsTransportChildAgent } from "@/transport/aiopsTransportTypes";

type LoadState =
  | { status: "idle"; transcript: null; error: "" }
  | { status: "loading"; transcript: null; error: "" }
  | { status: "loaded"; transcript: HostChildAgentTranscript; error: "" }
  | { status: "error"; transcript: null; error: string };

type HostSubagentDrawerProps = {
  open: boolean;
  childAgent?: AiopsTransportChildAgent;
  onOpenChange: (open: boolean) => void;
  loadTranscript?: (childAgentId: string) => Promise<HostChildAgentTranscript>;
};

export function HostSubagentDrawer({
  open,
  childAgent,
  onOpenChange,
  loadTranscript = getChildAgentTranscript,
}: HostSubagentDrawerProps) {
  const childAgentId = childAgent?.id ?? "";
  const [loadState, setLoadState] = useState<LoadState>({ status: "idle", transcript: null, error: "" });

  useEffect(() => {
    if (!open || !childAgentId) {
      setLoadState({ status: "idle", transcript: null, error: "" });
      return;
    }

    let cancelled = false;
    setLoadState({ status: "loading", transcript: null, error: "" });

    loadTranscript(childAgentId)
      .then((transcript) => {
        if (!cancelled) {
          setLoadState({ status: "loaded", transcript, error: "" });
        }
      })
      .catch((error: unknown) => {
        if (!cancelled) {
          setLoadState({
            status: "error",
            transcript: null,
            error: error instanceof Error ? error.message : String(error),
          });
        }
      });

    return () => {
      cancelled = true;
    };
  }, [childAgentId, loadTranscript, open]);

  const hostLabel = useMemo(() => formatHostLabel(childAgent), [childAgent]);

  return (
    <Sheet open={open && Boolean(childAgent)} onOpenChange={onOpenChange}>
      <SheetContent
        showCloseButton={false}
        className="!w-[min(640px,calc(100vw-24px))] !max-w-none gap-0 overflow-hidden sm:!max-w-[640px]"
      >
        <SheetHeader className="border-b border-zinc-200 px-4 py-3">
          <div className="flex min-w-0 items-start gap-3">
            <div className="flex size-8 shrink-0 items-center justify-center rounded-md border border-zinc-200 bg-zinc-50 text-zinc-700">
              <BotIcon className="size-4" aria-hidden="true" />
            </div>
            <div className="min-w-0 flex-1">
              <SheetTitle className="truncate">子 agent 对话</SheetTitle>
              <SheetDescription className="truncate">
                {childAgent?.hostDisplayName || "未知主机"} {hostLabel ? `@${hostLabel}` : ""}
                {childAgent?.task ? ` · ${childAgent.task}` : ""}
              </SheetDescription>
            </div>
            <SheetClose asChild>
              <Button
                type="button"
                variant="ghost"
                size="icon-sm"
                aria-label="关闭子 agent 对话"
                data-testid="host-subagent-drawer-close"
              >
                <XIcon className="size-4" aria-hidden="true" />
              </Button>
            </SheetClose>
          </div>
        </SheetHeader>

        <div className="min-h-0 flex-1 overflow-y-auto px-4 py-3" data-testid="host-subagent-drawer">
          <ChildAgentSummary childAgent={childAgent} />
          <TranscriptBody loadState={loadState} />
        </div>
      </SheetContent>
    </Sheet>
  );
}

function ChildAgentSummary({ childAgent }: { childAgent?: AiopsTransportChildAgent }) {
  if (!childAgent) {
    return null;
  }

  return (
    <div className="mb-3 rounded-md border border-zinc-200 bg-zinc-50 px-3 py-2 text-xs text-zinc-600">
      <div className="flex min-w-0 items-center gap-2">
        <span className="size-1.5 shrink-0 rounded-full bg-emerald-500" aria-hidden="true" />
        <span className="min-w-0 flex-1 truncate">{childAgent.hostDisplayName || childAgent.hostId}</span>
        <span className="shrink-0 text-zinc-500">{formatStatus(childAgent.status)}</span>
      </div>
      <div className="mt-1 truncate text-zinc-500">
        {childAgent.hostAddress ? `@${childAgent.hostAddress}` : childAgent.hostId}
        {childAgent.sessionId ? ` · ${childAgent.sessionId}` : ""}
      </div>
    </div>
  );
}

function TranscriptBody({ loadState }: { loadState: LoadState }) {
  if (loadState.status === "loading") {
    return (
      <div className="flex items-center gap-2 rounded-md border border-zinc-200 px-3 py-3 text-sm text-zinc-600">
        <CircleDashedIcon className="size-4 animate-spin" aria-hidden="true" />
        正在读取子 agent 对话
      </div>
    );
  }

  if (loadState.status === "error") {
    return (
      <div className="rounded-md border border-red-200 bg-red-50 px-3 py-3 text-sm text-red-700">
        <div className="flex items-center gap-2 font-medium">
          <AlertCircleIcon className="size-4" aria-hidden="true" />
          读取 transcript 失败
        </div>
        <div className="mt-1 break-words text-red-600">{loadState.error || "未知错误"}</div>
      </div>
    );
  }

  if (loadState.status !== "loaded") {
    return null;
  }

  if (loadState.transcript.items.length === 0) {
    return (
      <div className="rounded-md border border-dashed border-zinc-300 px-3 py-6 text-center text-sm text-zinc-500">
        暂无独立对话记录
      </div>
    );
  }

  return (
    <div className="grid gap-2">
      {loadState.transcript.items.map((item) => (
        <TranscriptItemView key={item.id} item={item} />
      ))}
    </div>
  );
}

function TranscriptItemView({ item }: { item: HostOpsTranscriptItem }) {
  const meta = itemMeta(item.type);
  const Icon = meta.icon;
  const isTool = item.type === "tool_call" || item.type === "tool_result";

  return (
    <article
      className="rounded-md border border-zinc-200 bg-white px-3 py-2 text-sm text-zinc-800"
      data-testid={`host-subagent-transcript-item-${item.id}`}
    >
      <div className="mb-1 flex min-w-0 items-center gap-2 text-xs text-zinc-500">
        <Icon className="size-3.5 shrink-0" aria-hidden="true" />
        <span className="shrink-0 font-medium text-zinc-700">{meta.label}</span>
        {item.toolName ? <span className="min-w-0 truncate">· {item.toolName}</span> : null}
        {item.status ? <span className="ml-auto shrink-0">{formatStatus(item.status)}</span> : null}
      </div>
      {item.content ? (
        <div
          className={
            isTool
              ? "whitespace-pre-wrap break-words rounded bg-zinc-50 px-2 py-1 font-mono text-xs text-zinc-800"
              : "whitespace-pre-wrap break-words leading-6"
          }
        >
          {item.content}
        </div>
      ) : (
        <div className="text-xs text-zinc-400">无内容</div>
      )}
      {item.createdAt ? <div className="mt-1 text-[11px] text-zinc-400">{formatTimestamp(item.createdAt)}</div> : null}
    </article>
  );
}

function itemMeta(type: string): { label: string; icon: LucideIcon } {
  switch (type) {
    case "manager_message":
      return { label: "Manager 输入", icon: UserIcon };
    case "user_followup":
      return { label: "用户追问", icon: UserIcon };
    case "assistant_message":
      return { label: "Assistant 返回", icon: BotIcon };
    case "tool_call":
      return { label: "工具调用", icon: TerminalIcon };
    case "tool_result":
      return { label: "工具结果", icon: WrenchIcon };
    case "approval":
      return { label: "审批", icon: AlertCircleIcon };
    case "error":
      return { label: "错误", icon: AlertCircleIcon };
    default:
      return { label: type || "消息", icon: BotIcon };
  }
}

function formatHostLabel(childAgent?: AiopsTransportChildAgent) {
  if (!childAgent) {
    return "";
  }
  return childAgent.hostAddress || childAgent.hostId || "";
}

function formatStatus(status: string) {
  switch (status) {
    case "planned":
      return "已计划";
    case "spawning":
      return "启动中";
    case "running":
      return "运行中";
    case "waiting":
      return "等待中";
    case "approval_required":
      return "待审批";
    case "completed":
      return "已完成";
    case "failed":
      return "失败";
    case "cancelled":
      return "已取消";
    default:
      return status;
  }
}

function formatTimestamp(value: string) {
  return value.replace("T", " ").replace("Z", "");
}
