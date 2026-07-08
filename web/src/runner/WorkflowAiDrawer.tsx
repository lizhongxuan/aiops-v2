import { useEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";
import { Bot, ListChecks, MessageSquarePlus, Send, X } from "lucide-react";
import {
  WorkflowAiConflictCard,
  WorkflowAiStepGenerationCard,
  WorkflowAiToolTimeline,
  WorkflowEditPlanCard,
  WorkflowPatchPreviewCard,
  WorkflowPatchResultCard,
} from "./WorkflowAiCards";
import { WorkflowAiPermissionDialog } from "./WorkflowAiPermissionDialog";
import type {
  WorkflowAiActiveStep,
  WorkflowAiContext,
  WorkflowAiEffectStatus,
  WorkflowAiSession,
  WorkflowAiStepHistoryItem,
  WorkflowAiToolLogEntry,
  WorkflowEditPlan,
  WorkflowPatch,
  WorkflowPatchResult,
} from "./workflowAiTypes";

const WORKFLOW_AI_DRAWER_WIDTH_KEY = "runner.workflowAi.drawerWidth";
const BUSY_WORKFLOW_AI_STAGES = new Set(["chatting", "planning", "patch_generating", "applying_plan"]);

type WorkflowAiTranscriptEntry =
  | { id: string; role: "user"; kind: "text"; text: string }
  | { id: string; role: "assistant"; kind: "readonly"; title: string; text: string }
  | { id: string; role: "assistant"; kind: "plan"; plan: WorkflowEditPlan }
  | { id: string; role: "assistant"; kind: "conflict"; reason: string };

export function WorkflowAiDrawer({
  open,
  context,
  stage = "context_loaded",
  plan,
  patch,
  result,
  effectStatus,
  conflictReason,
  readonlyAnswer = "",
  readonlyAnswerTitle = "工作流说明",
  toolLog = [],
  stepHistory = [],
  onClose,
  onSubmit,
  onApplyPatch,
  onRejectApply,
  onUndo,
  onContinue,
  onNewSession,
  onOpenEvents,
  initialMessage = "",
  activeStep,
  session,
}: {
  open: boolean;
  context: WorkflowAiContext;
  session?: WorkflowAiSession;
  stage?: string;
  plan?: WorkflowEditPlan;
  patch?: WorkflowPatch;
  result?: WorkflowPatchResult;
  effectStatus?: WorkflowAiEffectStatus;
  conflictReason?: string;
  readonlyAnswer?: string;
  readonlyAnswerTitle?: string;
  toolLog?: WorkflowAiToolLogEntry[];
  stepHistory?: WorkflowAiStepHistoryItem[];
  onClose?: () => void;
  onSubmit?: (message: string) => void;
  onApplyPatch?: () => void;
  onRejectApply?: () => void;
  onUndo?: () => void;
  onContinue?: () => void;
  onNewSession?: () => void;
  onOpenEvents?: () => void;
  initialMessage?: string;
  activeStep?: WorkflowAiActiveStep;
}) {
  const [message, setMessage] = useState("");
  const [transcriptEntries, setTranscriptEntries] = useState<WorkflowAiTranscriptEntry[]>([]);
  const [drawerWidth, setDrawerWidth] = useState(() => {
    const stored = Number(window.localStorage?.getItem(WORKFLOW_AI_DRAWER_WIDTH_KEY) || 0);
    return Number.isFinite(stored) && stored >= 320 ? stored : 420;
  });
  const [permissionOpen, setPermissionOpen] = useState(false);
  const appliedInitialMessageRef = useRef("");
  const contextKeyRef = useRef(`${context.workflowId || ""}:${context.workflowName || ""}:${session?.id || ""}`);
  const assistantSignatureRef = useRef("");
  const drawerWidthRef = useRef(drawerWidth);
  const transcriptRef = useRef<HTMLDivElement | null>(null);
  useEffect(() => {
    const contextKey = `${context.workflowId || ""}:${context.workflowName || ""}:${session?.id || ""}`;
    if (contextKeyRef.current !== contextKey) {
      contextKeyRef.current = contextKey;
      appliedInitialMessageRef.current = "";
      assistantSignatureRef.current = "";
      setTranscriptEntries([]);
    }
  }, [context.workflowId, context.workflowName, session?.id]);
  useEffect(() => {
    if (!open) return;
    if (initialMessage && appliedInitialMessageRef.current !== initialMessage) {
      setMessage(initialMessage);
      appliedInitialMessageRef.current = initialMessage;
    }
  }, [initialMessage, open]);
  useEffect(() => {
    drawerWidthRef.current = drawerWidth;
  }, [drawerWidth]);
  useEffect(() => {
    if (!open) return;
    const snapshot = workflowAiAssistantSnapshot({ plan, readonlyAnswer, readonlyAnswerTitle, conflictReason });
    if (!snapshot || snapshot.signature === assistantSignatureRef.current) return;
    assistantSignatureRef.current = snapshot.signature;
    setTranscriptEntries((current) => {
      if (snapshot.entry.kind === "plan") {
        return [...current.filter((entry) => entry.kind !== "plan"), snapshot.entry];
      }
      return [...current, snapshot.entry];
    });
  }, [conflictReason, open, plan, readonlyAnswer, readonlyAnswerTitle]);
  useEffect(() => {
    if (!open) return;
    const transcript = transcriptRef.current;
    if (!transcript) return;
    const scrollToBottom = () => {
      const top = transcript.scrollHeight;
      transcript.scrollTop = top;
      if (typeof transcript.scrollTo === "function") {
        transcript.scrollTo({ top, behavior: "smooth" });
      }
    };
    scrollToBottom();
    const frame = window.requestAnimationFrame?.(scrollToBottom);
    const timer = window.setTimeout(scrollToBottom, 0);
    return () => {
      if (typeof frame === "number") window.cancelAnimationFrame?.(frame);
      window.clearTimeout(timer);
    };
  }, [activeStep, conflictReason, open, patch, plan, readonlyAnswer, result, stage, stepHistory, toolLog, transcriptEntries]);

  if (!open) return null;

  const isBusy = BUSY_WORKFLOW_AI_STAGES.has(stage);
  const composerPlaceholder = isBusy
    ? stage === "chatting" ? "Workflow AI 正在回复，请稍候" : "Workflow AI 正在生成，请稍候"
    : "告诉 Workflow AI 你想解释、检查、创建或修改什么工作流";
  const submit = () => {
    const trimmed = message.trim();
    if (!trimmed || isBusy) return;
    setTranscriptEntries((current) => [...current, { id: `user-${Date.now()}-${current.length}`, role: "user", kind: "text", text: trimmed }]);
    setMessage("");
    onSubmit?.(trimmed);
  };
  const startNewSession = () => {
    setTranscriptEntries([]);
    setMessage("");
    assistantSignatureRef.current = "";
    appliedInitialMessageRef.current = "";
    onNewSession?.();
  };
  const startResize = (event: ReactPointerEvent<HTMLDivElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture?.(event.pointerId);
    const resize = (moveEvent: PointerEvent) => {
      const viewportWidth = window.innerWidth || 1200;
      const minWidth = Math.min(360, viewportWidth);
      const maxWidth = Math.max(minWidth, Math.min(720, viewportWidth - 120));
      const nextWidth = Math.max(minWidth, Math.min(maxWidth, viewportWidth - moveEvent.clientX));
      drawerWidthRef.current = nextWidth;
      setDrawerWidth(nextWidth);
    };
    const stopResize = () => {
      window.localStorage?.setItem(WORKFLOW_AI_DRAWER_WIDTH_KEY, String(drawerWidthRef.current));
      window.removeEventListener("pointermove", resize);
      window.removeEventListener("pointerup", stopResize);
      window.removeEventListener("pointercancel", stopResize);
    };
    window.addEventListener("pointermove", resize);
    window.addEventListener("pointerup", stopResize);
    window.addEventListener("pointercancel", stopResize);
  };
  const hasStepHistory = stepHistory.length > 0;

  return (
    <aside className="workflow-ai-drawer" data-testid="workflow-ai-drawer" style={{ width: `${drawerWidth}px` }}>
      <div
        className="workflow-ai-resize-handle"
        data-testid="workflow-ai-resize-handle"
        role="separator"
        aria-label="调整 Workflow AI 宽度"
        aria-orientation="vertical"
        onPointerDown={startResize}
      />
      <header className="workflow-ai-drawer-header">
        <div>
          <Bot size={18} />
          <div className="workflow-ai-title-block">
            <h2>Workflow AI</h2>
            <span data-testid="workflow-ai-updated-label">{context.workflowName ? `${context.workflowName} · ` : ""}{context.lastModifiedLabel || "修改时间 - "}</span>
          </div>
        </div>
        <button type="button" aria-label="Close workflow AI" onClick={onClose}>
          <X size={16} />
        </button>
      </header>
      <div className="workflow-ai-drawer-body">
        <div className="workflow-ai-chat-transcript" data-testid="workflow-ai-chat-transcript" ref={transcriptRef}>
          {!transcriptEntries.length && !plan && !patch && !result && !readonlyAnswer ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <p>你可以直接问我，也可以描述要创建或修改的工作流。</p>
              </div>
            </div>
          ) : null}
          {transcriptEntries.map((entry) => renderWorkflowAiTranscriptEntry(entry))}
          {["chatting", "planning"].includes(stage) && !plan && !patch && !result && !conflictReason && !readonlyAnswer ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                {stage === "chatting" ? <WorkflowAiChatThinkingCard /> : <WorkflowAiThinkingCard lastUserMessage={lastUserMessage(transcriptEntries)} />}
              </div>
            </div>
          ) : null}
          {stepHistory.map((step) => (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant" key={`${step.index}-${step.title}-${step.status}`}>
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <WorkflowAiStepGenerationCard step={step} status={step.status} history />
              </div>
            </div>
          ))}
          {stage === "applying_plan" && activeStep && !hasStepHistory ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <WorkflowAiStepGenerationCard step={activeStep} />
              </div>
            </div>
          ) : null}
          {patch ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <p>正在准备图层变更。</p>
                <WorkflowPatchPreviewCard
                  patch={patch}
                  effectStatus={effectStatus}
                />
              </div>
            </div>
          ) : null}
          {result ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <WorkflowPatchResultCard result={result} onUndo={onUndo} />
              </div>
            </div>
          ) : null}
          {effectStatus && effectStatus !== "changed" ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <section className="workflow-ai-card workflow-ai-card-muted" data-testid="workflow-ai-non-effect-card">
                  <h3>{effectStatus}</h3>
                  <p>This patch will not advance automatically.</p>
                </section>
              </div>
            </div>
          ) : null}
          {stage === "budget_paused" ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <section className="workflow-ai-card" data-testid="workflow-ai-budget-card">
                  <h3>Budget paused</h3>
                  <button type="button" onClick={onContinue}>Continue next batch</button>
                  <button type="button">Run validation</button>
                  <button type="button">Finish summary</button>
                </section>
              </div>
            </div>
          ) : null}
          {toolLog.length ? (
            <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant">
              <div className="workflow-ai-avatar">AI</div>
              <div className="workflow-ai-bubble">
                <WorkflowAiToolTimeline entries={toolLog} />
              </div>
            </div>
          ) : null}
        </div>
      </div>
      <footer className="workflow-ai-drawer-footer">
        <textarea
          value={message}
          onChange={(event) => setMessage(event.currentTarget.value)}
          onKeyDown={(event) => {
            if (event.key === "Enter" && !event.shiftKey) {
              event.preventDefault();
              submit();
            }
          }}
          disabled={isBusy}
          aria-busy={isBusy ? "true" : "false"}
          placeholder={composerPlaceholder}
        />
        <div>
          <button type="button" onClick={startNewSession}>
            <MessageSquarePlus size={14} />
            新会话
          </button>
          <button type="button" onClick={onOpenEvents}>
            <ListChecks size={14} />
            事件
          </button>
          <button type="button" className="primary" onClick={submit} disabled={!message.trim() || isBusy}>
            <Send size={14} />
            Send
          </button>
        </div>
      </footer>
      <WorkflowAiPermissionDialog
        open={permissionOpen}
        patch={patch}
        onReject={() => {
          setPermissionOpen(false);
          onRejectApply?.();
        }}
        onConfirm={() => {
          setPermissionOpen(false);
          onApplyPatch?.();
        }}
      />
    </aside>
  );
}

function workflowAiAssistantSnapshot({
  plan,
  readonlyAnswer,
  readonlyAnswerTitle,
  conflictReason,
}: {
  plan?: WorkflowEditPlan;
  readonlyAnswer: string;
  readonlyAnswerTitle: string;
  conflictReason?: string;
}): { signature: string; entry: WorkflowAiTranscriptEntry } | null {
  const answer = readonlyAnswer.trim();
  if (answer) {
    const signature = `readonly:${readonlyAnswerTitle}:${answer}`;
    return {
      signature,
      entry: { id: `assistant-${stableWorkflowAiEntryID(signature)}`, role: "assistant", kind: "readonly", title: readonlyAnswerTitle, text: answer },
    };
  }
  if (plan?.items?.length) {
    const signature = `plan:${plan.id || ""}:${plan.items.map((item) => `${item.id}:${item.title}:${item.description}`).join("|")}`;
    return {
      signature,
      entry: { id: `assistant-${stableWorkflowAiEntryID(signature)}`, role: "assistant", kind: "plan", plan },
    };
  }
  const conflict = String(conflictReason || "").trim();
  if (conflict) {
    const signature = `conflict:${conflict}`;
    return {
      signature,
      entry: { id: `assistant-${stableWorkflowAiEntryID(signature)}`, role: "assistant", kind: "conflict", reason: conflict },
    };
  }
  return null;
}

function renderWorkflowAiTranscriptEntry(entry: WorkflowAiTranscriptEntry) {
  if (entry.role === "user") {
    return (
      <div className="workflow-ai-message user" data-testid="workflow-ai-message-user" key={entry.id}>
        <div className="workflow-ai-bubble">
          <p>{entry.text}</p>
        </div>
      </div>
    );
  }
  return (
    <div className="workflow-ai-message assistant" data-testid="workflow-ai-message-assistant" key={entry.id}>
      <div className="workflow-ai-avatar">AI</div>
      <div className="workflow-ai-bubble">
        {entry.kind === "readonly" ? (
          <div className="workflow-ai-readonly-answer" data-testid="workflow-ai-readonly-answer">
            {entry.text.split("\n").map((line, index) => <p key={`${index}-${line}`}>{line}</p>)}
          </div>
        ) : null}
        {entry.kind === "plan" ? <WorkflowEditPlanCard plan={entry.plan} /> : null}
        {entry.kind === "conflict" ? <WorkflowAiConflictCard reason={entry.reason} /> : null}
      </div>
    </div>
  );
}

function stableWorkflowAiEntryID(input: string) {
  let hash = 0;
  for (let index = 0; index < input.length; index += 1) {
    hash = ((hash << 5) - hash + input.charCodeAt(index)) | 0;
  }
  return Math.abs(hash).toString(36);
}

function lastUserMessage(entries: WorkflowAiTranscriptEntry[]) {
  for (let index = entries.length - 1; index >= 0; index -= 1) {
    const entry = entries[index];
    if (entry.role === "user") return entry.text;
  }
  return "";
}

function WorkflowAiThinkingCard({ lastUserMessage: _currentMessage = "" }: { lastUserMessage?: string }) {
  return (
    <section className="workflow-ai-card workflow-ai-progress-card" data-testid="workflow-ai-thinking-card">
      <header>
        <span className="workflow-ai-spinner-dot" aria-hidden="true" />
        <h3>正在思考</h3>
      </header>
    </section>
  );
}

function WorkflowAiChatThinkingCard() {
  return (
    <section className="workflow-ai-card workflow-ai-progress-card" data-testid="workflow-ai-chat-thinking-card">
      <header>
        <span className="workflow-ai-spinner-dot" aria-hidden="true" />
        <h3>正在回复</h3>
      </header>
      <ol className="workflow-ai-progress-list" aria-label="普通回复过程">
        <li>读取当前画布</li>
        <li>调用模型</li>
        <li>等待模型回复</li>
      </ol>
    </section>
  );
}
