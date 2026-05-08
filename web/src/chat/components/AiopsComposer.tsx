import {
  useAssistantApi,
  ComposerPrimitive,
  useAssistantTransportSendCommand,
  useAssistantTransportState,
  useComposer,
  useComposerRuntime,
  useThread,
} from "@assistant-ui/react";
import { ArrowUp, Check, LoaderCircle, Square, X } from "lucide-react";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { Textarea } from "@/components/ui/textarea";
import { isAiopsTransportRunning } from "@/transport/aiopsTransportConverter";
import type { AiopsApprovalAction } from "@/transport/aiopsTransportRuntime";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import type { AiopsTransportApproval, AiopsTransportState } from "@/transport/aiopsTransportTypes";

import { resolveStopDispatchTarget } from "./aiopsComposerActions";
import { useSessionTargetContext } from "./SessionTargetContext";
import { useSessionWorkspaceContext } from "./SessionWorkspaceContext";

export function AiopsComposer({
  className = "",
  variant = "default",
}: {
  className?: string;
  variant?: "default" | "chat";
} = {}) {
  const state = useAssistantTransportState() as AiopsTransportState;
  const threadIsRunning = useThread((snapshot) => snapshot.isRunning);
  const workspace = useSessionWorkspaceContext();
  const isRunning = isAiopsTransportRunning(state) || threadIsRunning;
  const pendingApproval = selectComposerApproval(state);
  if (pendingApproval) {
    return <BlockedApprovalComposer approval={pendingApproval} />;
  }

  return (
    <div
      className={[
        variant === "chat"
          ? "shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6"
          : "border-t border-zinc-200 bg-white px-4 py-3 lg:px-8",
        className,
      ]
        .filter(Boolean)
        .join(" ")}
      data-testid="aiops-composer-shell"
    >
      <div className="mx-auto flex max-w-3xl flex-col gap-2">
        <ComposerBody
          variant={variant}
          isRunning={isRunning}
          state={state}
          threadIsRunning={threadIsRunning}
        />
        {workspace.composerDisabledReason ? (
          <div className="px-1 text-xs text-amber-700">{workspace.composerDisabledReason}</div>
        ) : null}
      </div>
    </div>
  );
}

function selectComposerApproval(state: AiopsTransportState): AiopsTransportApproval | undefined {
  const approvals = Object.values(state.pendingApprovals || {}).filter((approval) => {
    const status = approval.status?.trim();
    return !status || status === "pending" || status === "blocked";
  });
  if (approvals.length === 0) {
    return undefined;
  }
  const currentTurnID = state.currentTurnId?.trim();
  const currentTurnApproval = approvals.find((approval) => approval.turnId?.trim() === currentTurnID);
  if (currentTurnApproval) {
    return currentTurnApproval;
  }
  return approvals.sort((a, b) => (b.requestedAt || "").localeCompare(a.requestedAt || ""))[0];
}

function ComposerBody({
  variant,
  isRunning,
  state,
  threadIsRunning,
}: {
  variant: "default" | "chat";
  isRunning: boolean;
  state: AiopsTransportState;
  threadIsRunning: boolean;
}) {
  const workspace = useSessionWorkspaceContext();
  const inputRef = useRef<HTMLTextAreaElement | null>(null);

  useEffect(() => {
    inputRef.current?.focus();
  }, [workspace.composerFocusNonce]);

  return (
    <ComposerPrimitive.Root
      className={
        variant === "chat"
          ? "flex flex-col gap-2 rounded-[1.75rem] border border-slate-200 bg-white p-2 shadow-[0_10px_28px_rgba(15,23,42,0.10)] transition-shadow focus-within:border-slate-300 focus-within:shadow-[0_12px_36px_rgba(15,23,42,0.14)]"
          : "mx-auto flex max-w-5xl items-end gap-2"
      }
    >
      <ComposerPrimitive.Input asChild submitOnEnter>
        <Textarea
          ref={inputRef}
          data-testid="omnibar-input"
          rows={1}
          placeholder="输入你的问题或任务"
          disabled={Boolean(workspace.composerDisabledReason) || isRunning}
          className={
            variant === "chat"
              ? "max-h-40 min-h-12 resize-none border-0 bg-transparent px-3 py-2 text-[15px] leading-7 shadow-none focus-visible:ring-0"
              : "max-h-44 min-h-11 resize-none rounded-lg border-zinc-300 bg-zinc-50 text-sm"
          }
        />
      </ComposerPrimitive.Input>

      <div className={variant === "chat" ? "flex shrink-0 items-center justify-between" : "mb-1 flex shrink-0 items-center gap-2"}>
        <span className="text-xs text-slate-400 pl-1">{workspace.llmLabel}</span>
        <TargetAwareSendButton variant={variant} isRunning={isRunning} state={state} threadIsRunning={threadIsRunning} />
      </div>
    </ComposerPrimitive.Root>
  );
}

function TargetAwareSendButton({
  variant,
  isRunning,
  state,
  threadIsRunning,
}: {
  variant: "default" | "chat";
  isRunning: boolean;
  state: AiopsTransportState;
  threadIsRunning: boolean;
}) {
  const api = useAssistantApi();
  const composer = useComposerRuntime();
  const composerState = useComposer((snapshot) => snapshot);
  const sendCommand = useAssistantTransportSendCommand();
  const commands = useAiopsTransportCommands();
  const target = useSessionTargetContext();
  const workspace = useSessionWorkspaceContext();
  const [stopping, setStopping] = useState(false);
  const [forceStopVisible, setForceStopVisible] = useState(false);

  // Reset stopping and forceStopVisible state when the run completes/cancels
  useEffect(() => {
    if (!isRunning) {
      setStopping(false);
      setForceStopVisible(false);
    }
  }, [isRunning]);

  // 2-second timeout: if stop hasn't completed, show force-stop button
  useEffect(() => {
    if (!stopping) return;
    const timer = setTimeout(() => {
      if (isRunning) {
        setForceStopVisible(true);
      }
    }, 2000);
    return () => clearTimeout(timer);
  }, [stopping, isRunning]);

  const disabled = (stopping && !forceStopVisible) || (!isRunning && (!composerState?.text?.trim() || Boolean(workspace.composerDisabledReason)));
  const stopDispatchTarget = resolveStopDispatchTarget(state, threadIsRunning);

  async function stopRun(reason: string) {
    if (stopDispatchTarget === "runtime") {
      api.thread().cancelRun();
      return;
    }
    commands.stop(reason);
    api.thread().cancelRun();
  }

  return (
    <Button
      type="button"
      size={variant === "chat" ? "icon" : "icon-lg"}
      data-testid="omnibar-primary-action"
      aria-label={forceStopVisible ? "强制停止" : stopping ? "正在停止" : isRunning ? "停止生成" : "send message"}
      disabled={disabled}
      className={
        variant === "chat"
          ? isRunning
            ? forceStopVisible
              ? "h-8 w-8 rounded-full border border-red-400 bg-red-50 text-red-700 shadow-sm hover:bg-red-100"
              : "h-8 w-8 rounded-full border border-slate-300 bg-white text-slate-700 shadow-sm hover:bg-slate-50"
            : "h-8 w-8 rounded-full bg-slate-950 text-white shadow-sm hover:bg-slate-800 disabled:bg-slate-200 disabled:text-slate-400"
          : forceStopVisible
            ? "border-red-400 bg-red-50 text-red-700 hover:bg-red-100"
            : ""
      }
      onClick={() => {
        if (forceStopVisible) {
          void stopRun("user force stop");
          return;
        }
        if (isRunning) {
          setStopping(true);
          void stopRun("user requested stop");
          return;
        }
        const text = composer.getState().text.trim();
        if (!text) return;
        composer.setText("");
        const command = {
          type: "add-message",
          message: {
            role: "user",
            metadata: target.metadata,
            ...(target.hostId ? { hostId: target.hostId } : {}),
            parts: [{ type: "text", text }],
          },
        } as Parameters<typeof sendCommand>[0];
        sendCommand(command);
      }}
    >
      {forceStopVisible ? (
        <Square className="h-3.5 w-3.5 fill-red-600 text-red-600" />
      ) : stopping ? (
        <LoaderCircle className="h-3.5 w-3.5 animate-spin" />
      ) : isRunning ? (
        <Square className="h-3.5 w-3.5 fill-current" />
      ) : (
        <ArrowUp className="h-4 w-4" />
      )}
    </Button>
  );
}

function BlockedApprovalComposer({ approval }: { approval: AiopsTransportApproval }) {
  const commands = useAiopsTransportCommands();
  const [decision, setDecision] = useState<AiopsApprovalAction>("approve");
  const commandText = approval.command || approval.reason || approval.id;

  function submitDecision() {
    commands.approvalDecision(approval.id, decision);
  }

  return (
    <div className="shrink-0 bg-white px-4 pb-4 pt-2 md:pb-6" data-testid="codex-approval-inline">
      <div className="mx-auto max-w-3xl rounded-[1.75rem] border border-slate-200 bg-white p-4 shadow-[0_10px_28px_rgba(15,23,42,0.10)]">
        <div className="space-y-3">
          <div>
            <div className="text-xs font-medium text-slate-400">等待审批</div>
            <div className="mt-1 text-[15px] font-semibold leading-6 text-slate-950">
              要执行这个命令，需要你确认吗？
            </div>
          </div>
          <div
            className="rounded-xl bg-slate-100 px-4 py-3 font-mono text-[13px] leading-6 text-slate-700"
            data-testid="codex-approval-command"
          >
            {commandText}
          </div>
          <div className="space-y-1" role="radiogroup" aria-label="审批选项">
            <button
              type="button"
              role="radio"
              aria-checked={decision === "approve"}
              className={[
                "flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-[15px] leading-6 transition-colors",
                decision === "approve" ? "bg-slate-100 text-slate-950" : "text-slate-500 hover:bg-slate-50",
              ].join(" ")}
              onClick={() => setDecision("approve")}
            >
              <span>1. 同意</span>
              {decision === "approve" ? <Check className="h-4 w-4 text-slate-500" /> : null}
            </button>
            <button
              type="button"
              role="radio"
              aria-checked={decision === "deny"}
              className={[
                "flex w-full items-center justify-between rounded-xl px-3 py-2 text-left text-[15px] leading-6 transition-colors",
                decision === "deny" ? "bg-slate-100 text-slate-950" : "text-slate-500 hover:bg-slate-50",
              ].join(" ")}
              onClick={() => setDecision("deny")}
            >
              <span>2. 拒绝</span>
              {decision === "deny" ? <X className="h-4 w-4 text-slate-500" /> : null}
            </button>
          </div>
        </div>
        <div className="mt-4 flex justify-end">
          <Button
            type="button"
            size="sm"
            className="rounded-full bg-slate-950 px-4 text-white hover:bg-slate-800"
            onClick={submitDecision}
          >
            {decision === "approve" ? "同意" : "拒绝"}
          </Button>
        </div>
      </div>
    </div>
  );
}
