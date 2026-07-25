import { useAssistantApi, useAssistantTransportSendCommand, useAssistantTransportState } from "@assistant-ui/react";
import { Bot, CircleStop, RotateCcw, Send, X } from "lucide-react";
import { useEffect, useMemo, useRef, useState } from "react";

import { AgentUiArtifactPart } from "@/components/chat/AgentUiArtifactPart";
import {
  WORKFLOW_CHANGE_ACTION_EVENT,
  type WorkflowChangeActionEventDetail,
} from "@/components/chat/WorkflowChangeArtifact";
import { RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT } from "@/components/runner/RunnerCanvas";
import { useAiopsTransportCommands } from "@/transport/useAiopsTransportCommands";
import { toUserFacingTransportErrorMessage } from "@/transport/transportErrorMessage";
import type {
  AiopsTransportAgentUiArtifact,
  AiopsTransportBlock,
  AiopsTransportState,
  AiopsTransportTurn,
  AiopsWorkflowTurnContextRequest,
} from "@/transport/aiopsTransportTypes";
import { useWorkflowWorkspace } from "@/runner/workspace/WorkflowWorkspaceProvider";
import { WorkflowApprovalCard } from "@/runner/WorkflowApprovalCard";
import {
  readWorkflowCandidateApplyConfirmation,
  WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT,
} from "@/runner/WorkflowCandidateApplyDialog";
import {
  parseWorkflowTransportArtifact,
  workflowTransportArtifactIdentity,
  type WorkflowTransportArtifactFact,
} from "@/runner/workspace/workflowCandidateArtifact";
import type { WorkflowEnvironmentFact, WorkflowSelectionFact } from "@/runner/workspace/workflowWorkspaceTypes";
import {
  selectOrderedWorkflowBlocks,
  selectWorkflowApproval,
  selectWorkflowTransportActivity,
  selectWorkflowTransportTurns,
} from "@/runner/transport/workflowTransportSelectors";
import {
  createWorkflowSurfaceFocusDetail,
  dispatchWorkflowSurfaceFocus,
  readWorkflowSurfaceFocusDetail,
  resolveWorkflowOperationIndex,
  WORKFLOW_SURFACE_FOCUS_EVENT,
  type WorkflowSurfaceFocusCandidate,
} from "@/runner/transport/workflowSurfaceFocus";

type WorkflowAddMessageCommand = {
  type: "add-message";
  message: {
    id: string;
    role: "user";
    metadata: { clientTurnId: string; workflowId: string };
    workflowContext: AiopsWorkflowTurnContextRequest;
    parts: [{ type: "text"; text: string }];
  };
};

export type WorkflowAiReviewTarget = {
  artifactId: string;
  turnId: string;
  candidateId: string;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
};

export type WorkflowScopeWorkflowOption = {
  workflowId: string;
  label: string;
};

export type WorkflowScopeEnvironmentOption = {
  id: string;
  label: string;
  environment: WorkflowEnvironmentFact;
};

export type WorkflowAiTransportPanelProps = {
  workflowId: string;
  environment: WorkflowEnvironmentFact;
  workflowOptions?: readonly WorkflowScopeWorkflowOption[];
  environmentOptions?: readonly WorkflowScopeEnvironmentOption[];
  transportThreadId: string;
  transportSessionId?: string;
  onClose?: () => void;
  embedded?: boolean;
  onOpenReview?: (target: WorkflowAiReviewTarget) => void;
  onWorkflowScopeChange?: (option: WorkflowScopeWorkflowOption) => void;
  onEnvironmentScopeChange?: (option: WorkflowScopeEnvironmentOption) => void;
  className?: string;
  createClientMessageId?: () => string;
};

export function WorkflowAiTransportPanel({
  workflowId,
  environment,
  transportThreadId,
  transportSessionId,
  onClose,
  embedded = false,
  onOpenReview,
  className = "",
  createClientMessageId = defaultClientMessageId,
}: WorkflowAiTransportPanelProps) {
  const transport = useAssistantTransportState() as AiopsTransportState;
  const assistantApi = useAssistantApi();
  const rawSendCommand = useAssistantTransportSendCommand();
  const commands = useAiopsTransportCommands();
  const workspace = useWorkflowWorkspace();
  const [draft, setDraft] = useState("");
  const [focusedChange, setFocusedChange] = useState<{ artifactId: string; changeSetId: string; operationIndex: number }>();
  const [approvalRecoveryArtifactId, setApprovalRecoveryArtifactId] = useState<string>();
  const panelRef = useRef<HTMLElement | null>(null);
  const lastSubmissionRef = useRef("");
  const reconciledFactsRef = useRef(new Set<string>());
  const approvedBindingsRef = useRef(new Set<string>());
  const selectedCount = workspace.state.selection.nodeIds.length + workspace.state.selection.edgeIds.length;
  const effectiveSelection = selectedCount
    ? workspace.state.selection
    : EMPTY_WORKFLOW_SELECTION;
  const scope = useMemo(
    () => freezeWorkflowTurnContext(workflowId, workspace.state, environment, effectiveSelection),
    [effectiveSelection, environment, workflowId, workspace.state],
  );
  const turns = selectWorkflowTransportTurns(transport, {
    workflowId,
    threadId: transportThreadId,
    sessionId: transportSessionId,
  });
  const activity = selectWorkflowTransportActivity(transport);
  const visibleTransportError = activity.canRetry && transport.lastError
    ? toUserFacingTransportErrorMessage(transport.lastError)
    : undefined;
  const artifactEntries = useMemo(
    () => collectWorkflowArtifactEntries(turns, transport.updatedAt, workspace.state),
    [transport.updatedAt, turns, workspace.state],
  );
  const artifactsById = useMemo(() => new Map(artifactEntries.map((entry) => [entry.artifact.id, entry])), [artifactEntries]);
  const candidateApprovalOwners = useMemo(
    () => collectCandidateApprovalOwners(artifactEntries),
    [artifactEntries],
  );

  useEffect(() => {
    reconciledFactsRef.current.clear();
    approvedBindingsRef.current.clear();
    setApprovalRecoveryArtifactId(undefined);
    setFocusedChange(undefined);
  }, [workflowId]);

  useEffect(() => {
    for (const entry of artifactEntries) {
      const identity = workflowTransportArtifactIdentity(entry.parsed);
      if (reconciledFactsRef.current.has(identity)) continue;
      reconciledFactsRef.current.add(identity);
      workspace.reconcileTransportFact(entry.parsed);
    }
  }, [artifactEntries, workspace]);

  useEffect(() => {
    const handleAction = (event: Event) => {
      const detail = (event as CustomEvent<WorkflowChangeActionEventDetail>).detail;
      if (!detail || !canonicalString(detail.artifactId) || !detail.action) return;
      const entry = artifactsById.get(detail.artifactId);
      if (!entry) return;
      const target = objectValue(detail.action.target);
      if (detail.action.intent === "workflow.focus_object") {
        const nodeId = canonicalOptionalString(target.nodeId);
        const edgeId = canonicalOptionalString(target.edgeId);
        if (Boolean(nodeId) === Boolean(edgeId)) return;
        const focusTarget = nodeId ? { nodeId } : { edgeId: edgeId! };
        const candidate = entry.parsed.kind === "candidate" ? candidateActionBinding(entry) : undefined;
        const operationIndex = entry.parsed.kind === "candidate"
          ? resolveWorkflowOperationIndex(entry.parsed.fact.changeSet.operations, focusTarget, canonicalOperationIndex(target.operationIndex))
          : undefined;
        if (entry.parsed.kind === "candidate" && operationIndex === undefined) return;
        dispatchWorkflowSurfaceFocus(createWorkflowSurfaceFocusDetail({
          source: "chat",
          workflowId,
          target: focusTarget,
          ...(candidate ? { candidate } : {}),
          ...(operationIndex !== undefined ? { operationIndex } : {}),
          artifactId: entry.artifact.id,
          turnId: entry.turn.id,
        }));
        return;
      }
      if (entry.parsed.kind !== "candidate") return;
      const binding = candidateActionBinding(entry);
      if (detail.action.intent === "workflow.review_change") {
        if (!targetMatchesCandidate(target, binding)) return;
        onOpenReview?.({ artifactId: entry.artifact.id, turnId: entry.turn.id, ...binding });
        return;
      }
      if (detail.action.intent === "workflow.apply_candidate") {
        const canonicalApply = canonicalCandidateApplyAction(entry);
        if (!canonicalApply
          || canonicalApply.actionId !== detail.action.id
          || !targetMatchesCandidate(target, binding)
          || canonicalOptionalString(target.approvalId) !== canonicalApply.approvalId
          || !workspaceCandidateMatches(workspace.state.candidate, binding)
          || !pendingCandidateApproval(transport, entry.turn, canonicalApply.approvalId)) {
          setApprovalRecoveryArtifactId(entry.artifact.id);
          return;
        }
        setApprovalRecoveryArtifactId(undefined);
        window.dispatchEvent(new CustomEvent(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, {
          detail: { command: "apply_candidate", ...binding },
        }));
        return;
      }
      if (detail.action.intent === "workflow.retry_from_current") {
        if (canonicalOptionalString(target.candidateId) !== binding.candidateId) return;
        commands.retry(entry.turn.id);
        return;
      }
      if (detail.action.intent === "workflow.discard_candidate") {
        if (!targetMatchesCandidate(target, binding) || !workspaceCandidateMatches(workspace.state.candidate, binding)) {
          setApprovalRecoveryArtifactId(entry.artifact.id);
          return;
        }
        setApprovalRecoveryArtifactId(undefined);
        void workspace.discardCandidate().catch(() => undefined);
      }
    };
    window.addEventListener(WORKFLOW_CHANGE_ACTION_EVENT, handleAction);
    return () => window.removeEventListener(WORKFLOW_CHANGE_ACTION_EVENT, handleAction);
  }, [artifactsById, commands, onOpenReview, transport.pendingApprovals, transport.runtimeLiveness?.pendingApprovals, workflowId, workspace]);

  useEffect(() => {
    const confirmCandidateApply = (event: Event) => {
      const confirmation = readWorkflowCandidateApplyConfirmation(event);
      if (!confirmation) return;
      const entry = artifactsById.get(confirmation.artifactId);
      if (!entry || entry.parsed.kind !== "candidate") return;
      const binding = candidateActionBinding(entry);
      const canonicalApply = canonicalCandidateApplyAction(entry);
      if (!canonicalApply
        || canonicalApply.actionId !== confirmation.actionId
        || canonicalApply.approvalId !== confirmation.approvalId
        || !candidateBindingMatches(binding, confirmation.binding)
        || confirmation.changeCount !== entry.parsed.fact.changeSet.operations.length
        || !workspaceCandidateMatches(workspace.state.candidate, binding)
        || !pendingCandidateApproval(transport, entry.turn, confirmation.approvalId)) {
        setApprovalRecoveryArtifactId(entry.artifact.id);
        return;
      }
      const approvalIdentity = candidateApprovalKey(entry.turn.id, confirmation.approvalId, binding);
      if (approvedBindingsRef.current.has(approvalIdentity)) return;
      approvedBindingsRef.current.add(approvalIdentity);
      setApprovalRecoveryArtifactId(undefined);
      commands.approvalDecision(confirmation.approvalId, "accept");
    };
    window.addEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, confirmCandidateApply);
    return () => window.removeEventListener(WORKFLOW_CANDIDATE_APPLY_CONFIRMED_EVENT, confirmCandidateApply);
  }, [artifactsById, commands, transport, workspace.state.candidate]);

  useEffect(() => {
    const locateChange = (event: Event) => {
      const detail = readWorkflowSurfaceFocusDetail(event);
      if (!detail || detail.source === "chat" || detail.workflowId !== workflowId || !detail.candidate) return;
      const entry = artifactEntries.find((candidateEntry) => candidateEntry.parsed.kind === "candidate"
        && candidateBindingMatches(candidateActionBinding(candidateEntry), detail.candidate!));
      if (!entry || entry.parsed.kind !== "candidate") return;
      const operationIndex = resolveWorkflowOperationIndex(
        entry.parsed.fact.changeSet.operations,
        detail.target,
        detail.operationIndex,
      );
      if (operationIndex === undefined) return;
      setFocusedChange({ artifactId: entry.artifact.id, changeSetId: detail.candidate.changeSetId, operationIndex });
    };
    window.addEventListener(WORKFLOW_SURFACE_FOCUS_EVENT, locateChange);
    return () => window.removeEventListener(WORKFLOW_SURFACE_FOCUS_EVENT, locateChange);
  }, [artifactEntries, workflowId]);

  useEffect(() => {
    if (!focusedChange) return;
    const target = panelRef.current?.querySelector<HTMLElement>('[data-testid="workflow-focused-change"]');
    target?.focus({ preventScroll: true });
    target?.scrollIntoView?.({ block: "nearest", behavior: "smooth" });
  }, [focusedChange]);

  const submit = () => {
    const text = draft.trim();
    if (!text || !scope) return;
    const submissionKey = `${text}\u0000${JSON.stringify(scope)}`;
    if (lastSubmissionRef.current === submissionKey) return;
    lastSubmissionRef.current = submissionKey;
    const id = createClientMessageId();
    const command: WorkflowAddMessageCommand = {
      type: "add-message",
      message: {
        id,
        role: "user",
        metadata: { clientTurnId: id, workflowId: scope.workflowId },
        workflowContext: scope,
        parts: [{ type: "text", text }],
      },
    };
    (rawSendCommand as unknown as (command: WorkflowAddMessageCommand) => void)(command);
    setDraft("");
  };

  return (
    <section
      className={`flex min-h-0 min-w-0 flex-col border-l border-slate-200 bg-white ${className}`.trim()}
      aria-label="工作流 AI"
      data-testid="workflow-ai-transport-panel"
      ref={panelRef}
    >
      <header className={`flex items-center justify-between gap-3 px-4 ${embedded ? "py-2" : "border-b border-slate-200 py-3"}`}>
        <div className="flex min-w-0 items-center gap-2">
          <Bot className={`h-4 w-4 shrink-0 text-slate-600 ${embedded ? "sr-only" : ""}`} aria-hidden="true" />
          <div className="min-w-0">
            <h2 className={embedded ? "sr-only" : "truncate text-sm font-semibold text-slate-950"}>AI 协作</h2>
            <p className="text-xs text-slate-500" aria-live="polite" data-testid="workflow-ai-status">
              {activity.busy
                ? "正在处理"
                : transport.status === "canceled"
                  ? "已取消"
                  : transport.status === "blocked"
                    ? "等待你的操作"
                    : "描述目标，AI 会先展示待确认修改"}
            </p>
          </div>
        </div>
        <div className="flex shrink-0 items-center gap-1">
          {activity.canStop ? (
            <button
              type="button"
              data-testid="workflow-ai-stop"
              className="inline-flex min-h-11 items-center gap-1 rounded-lg px-2 text-xs font-medium text-slate-700 hover:bg-slate-100"
              onClick={() => {
                assistantApi.thread().cancelRun();
                // An approval-blocked turn has already ended its response
                // stream, so cancelRun has no in-flight request to abort. Send
                // the existing authoritative transport Stop command as well;
                // its server projection, not local component state, retires the
                // approval and marks the turn canceled.
                if (!activity.busy) commands.stop("user canceled pending workflow approval");
              }}
            >
              <CircleStop className="h-4 w-4" aria-hidden="true" />停止
            </button>
          ) : null}
          {activity.canRetry ? (
            <button
              type="button"
              data-testid="workflow-ai-retry"
              className="inline-flex min-h-11 items-center gap-1 rounded-lg px-2 text-xs font-medium text-slate-700 hover:bg-slate-100"
              onClick={() => commands.retry(activity.retryTurnId)}
            >
              <RotateCcw className="h-4 w-4" aria-hidden="true" />继续
            </button>
          ) : null}
          {onClose ? (
            <button
              type="button"
              aria-label="关闭工作流 AI"
              className="inline-flex h-11 w-11 items-center justify-center rounded-lg text-slate-600 hover:bg-slate-100"
              onClick={onClose}
            >
              <X className="h-4 w-4" aria-hidden="true" />
            </button>
          ) : null}
        </div>
      </header>

      <div className="min-h-0 flex-1 space-y-5 overflow-y-auto px-4 py-4" data-testid="workflow-ai-transcript">
        {visibleTransportError ? (
          <p
            role="alert"
            data-testid="workflow-ai-transport-error"
            className="rounded-xl border border-red-200 bg-red-50 p-3 text-sm leading-6 text-red-900"
          >
            {visibleTransportError}，画布未发生修改。你可以继续重试。
          </p>
        ) : null}
        {turns.length ? turns.map((turn) => {
          const orderedBlocks = selectOrderedWorkflowBlocks(turn);
          return (
            <article key={turn.id} className="space-y-3" data-turn-id={turn.id}>
              {turn.user?.text ? <p className="ml-8 rounded-xl bg-slate-100 px-3 py-2 text-sm leading-6 text-slate-900">{turn.user.text}</p> : null}
              {turn.workflowContext ? <TurnScope context={turn.workflowContext} /> : null}
              {orderedBlocks.map((block) => {
                const approval = selectApprovalForWorkflowBlock(transport, turn, orderedBlocks, block);
                const candidateDialogOwnsDecision = Boolean(approval
                  && candidateApprovalOwners.has(candidateApprovalOwnerKey(turn.id, approval.id)));
                return (
                  <TransportBlock
                    key={block.id}
                    block={block}
                    approval={approval}
                    decisionOwner={candidateDialogOwnsDecision ? "candidate-dialog" : "inline"}
                    onApproval={(approvalId, decision) => commands.approvalDecision(approvalId, decision)}
                    focusedChange={block.artifact?.id === focusedChange?.artifactId ? focusedChange : undefined}
                  />
                );
              })}
            </article>
          );
        }) : (
          <div className="workflow-ai-empty px-1 py-3" data-testid="workflow-ai-empty-state">
            <h3 className="text-base font-semibold text-slate-950">想修改什么？</h3>
            <p className="mt-1 text-sm leading-6 text-slate-500">
              直接描述目标，AI 会先展示待确认修改，不会直接覆盖已保存内容。
            </p>
          </div>
        )}
        {approvalRecoveryArtifactId ? (
          <p
            role="status"
            data-testid="workflow-approval-recovery"
            className="rounded-xl border border-amber-200 bg-amber-50 p-3 text-sm leading-6 text-amber-950"
          >
            等待确认步骤：当前候选还没有与本轮精确绑定的确认请求，请等待确认出现后再应用。
          </p>
        ) : null}
      </div>

      <form className="workflow-ai-composer border-t border-slate-200 p-3" onSubmit={(event) => { event.preventDefault(); submit(); }}>
        {!scope ? <p role="alert" className="mb-2 text-xs text-amber-700">工作流版本或环境尚未就绪，暂时不能发送修改请求。</p> : null}
        <div className="flex items-end gap-2 rounded-xl border border-slate-300 bg-white p-2 focus-within:border-slate-500">
          <textarea
            data-testid="workflow-ai-input"
            value={draft}
            rows={2}
            className="min-h-12 min-w-0 flex-1 resize-none border-0 bg-transparent px-2 py-1 text-sm leading-6 outline-none"
            placeholder="描述你想了解或修改的内容"
            onChange={(event) => {
              lastSubmissionRef.current = "";
              setDraft(event.target.value);
            }}
            onKeyDown={(event) => {
              if (event.key === "Enter" && !event.shiftKey && !event.nativeEvent.isComposing) {
                event.preventDefault();
                submit();
              }
            }}
          />
          <button
            type="submit"
            data-testid="workflow-ai-send"
            aria-label="发送"
            disabled={!scope || !draft.trim()}
            className="inline-flex h-11 w-11 shrink-0 items-center justify-center rounded-full bg-slate-950 text-white disabled:cursor-not-allowed disabled:bg-slate-200"
          >
            <Send className="h-4 w-4" aria-hidden="true" />
          </button>
        </div>
      </form>
    </section>
  );
}

function TransportBlock({
  block,
  approval,
  onApproval,
  focusedChange,
  decisionOwner,
}: {
  block: AiopsTransportBlock;
  approval?: AiopsTransportState["pendingApprovals"][string];
  onApproval: (approvalId: string, decision: "accept" | "reject") => void;
  focusedChange?: { artifactId: string; changeSetId: string; operationIndex: number };
  decisionOwner: "inline" | "candidate-dialog";
}) {
  if (block.type === "artifact") {
    return (
      <div
        data-transport-block-id={block.id}
        data-testid={focusedChange ? "workflow-focused-change" : undefined}
        data-workflow-change-item={focusedChange ? `${focusedChange.changeSetId}:${focusedChange.operationIndex}` : undefined}
        aria-current={focusedChange ? "true" : undefined}
        tabIndex={focusedChange ? -1 : undefined}
        className={focusedChange ? "rounded-xl ring-2 ring-sky-500 ring-offset-2" : undefined}
      >
        {focusedChange ? <p role="status" className="px-3 pb-2 text-xs font-medium text-sky-800">已定位到第 {focusedChange.operationIndex + 1} 项修改</p> : null}
        {block.artifact ? <AgentUiArtifactPart artifact={block.artifact} /> : <p role="status">结构化结果暂不可用</p>}
      </div>
    );
  }
  if (block.type === "final_answer") {
    return <div data-transport-block-id={block.id} className="whitespace-pre-wrap text-sm leading-7 text-slate-950">{block.finalContract?.answerText || block.text}</div>;
  }
  const approvalForDisplay = approval || approvalFromResolvedBlock(block);
  if (approvalForDisplay) return (
    <div data-transport-block-id={block.id}>
      <WorkflowApprovalCard
        approval={approvalForDisplay}
        blockText={block.text}
        decisionOwner={decisionOwner}
        onDecision={approval ? onApproval : undefined}
      />
    </div>
  );
  if (block.type === "commentary") {
    return <div data-transport-block-id={block.id} className="text-sm leading-6 text-slate-600">{block.text}</div>;
  }
  return (
    <details data-transport-block-id={block.id} data-testid="workflow-ai-process-details" className="rounded-lg bg-slate-50 px-3 py-2 text-xs leading-5 text-slate-600">
      <summary className="cursor-pointer font-medium text-slate-500">{PROCESS_LABELS[block.kind] || "处理详情"}</summary>
      <p className="mt-1 whitespace-pre-wrap">{block.text}</p>
    </details>
  );
}

function TurnScope({ context }: { context: AiopsWorkflowTurnContextRequest }) {
  const selected = [
    ...(Array.isArray(context.selection?.nodeIds) ? context.selection.nodeIds : []),
    ...(Array.isArray(context.selection?.edgeIds) ? context.selection.edgeIds : []),
  ];
  return <p className="text-xs text-slate-500">本轮基于：{selected.length ? selected.join("、") : "整个工作流"} · {context.environment.name || context.environment.bindingVersion}</p>;
}

function approvalFromResolvedBlock(block: AiopsTransportBlock) {
  if (block.type !== "approval" && block.kind !== "approval") return undefined;
  const status = String(block.status || "").toLowerCase();
  const presentationStatus = status === "rejected"
    ? "rejected"
    : status === "running" || status === "completed"
      ? "accepted"
      : status === "failed" || status === "skipped"
        ? "expired"
        : "unavailable";
  return {
    id: canonicalOptionalString(block.approvalId) || block.id,
    status: presentationStatus,
    reason: block.text,
  };
}

/**
 * A pending approval belongs to exactly one visible process block. Runtime may
 * retain the blocked tool block alongside the canonical approval block; using
 * the selector's turn-level fallback for both would render two identical cards.
 * Legacy/recovery turns that only contain a blocked tool still keep one card.
 */
function selectApprovalForWorkflowBlock(
  state: AiopsTransportState,
  turn: AiopsTransportTurn,
  orderedBlocks: readonly AiopsTransportBlock[],
  block: AiopsTransportBlock,
) {
  if (canonicalOptionalString(block.approvalId)) {
    return selectWorkflowApproval(state, turn.id, block.approvalId);
  }
  if (orderedBlocks.some((candidate) => canonicalOptionalString(candidate.approvalId))) return undefined;

  const owner = orderedBlocks.find((candidate) => candidate.type === "approval" || candidate.kind === "approval")
    || orderedBlocks.find((candidate) => candidate.status === "blocked"
      && (candidate.type === "tool" || ["tool", "command", "mcp"].includes(candidate.kind)));
  return owner?.id === block.id ? selectWorkflowApproval(state, turn.id) : undefined;
}

type WorkflowArtifactEntry = {
  turn: AiopsTransportTurn;
  block: AiopsTransportBlock;
  artifact: AiopsTransportAgentUiArtifact;
  parsed: WorkflowTransportArtifactFact;
};

type CandidateActionBinding = {
  candidateId: string;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
};

const EMPTY_WORKFLOW_SELECTION: WorkflowSelectionFact = { nodeIds: [], edgeIds: [] };

function collectWorkflowArtifactEntries(
  turns: AiopsTransportTurn[],
  transportUpdatedAt: string,
  workspace: ReturnType<typeof useWorkflowWorkspace>["state"],
): WorkflowArtifactEntry[] {
  const workflowKey = workspace.committed?.workflowKey || workspace.workflowKey;
  const committedRevision = workspace.committed?.revision;
  if (!workflowKey || !committedRevision) return [];
  return turns.flatMap((turn) => selectOrderedWorkflowBlocks(turn).flatMap((block) => {
    const artifact = block.type === "artifact" ? block.artifact : undefined;
    if (!artifact) return [];
    const parsed = parseWorkflowTransportArtifact(artifact, {
      workflowKey,
      committedRevision,
      receivedAt: artifact.updatedAt || artifact.createdAt || transportUpdatedAt,
    });
    return parsed ? [{ turn, block, artifact, parsed }] : [];
  }));
}

function candidateActionBinding(entry: WorkflowArtifactEntry): CandidateActionBinding {
  if (entry.parsed.kind !== "candidate") throw new TypeError("candidate artifact is required");
  return {
    candidateId: entry.parsed.fact.candidate.id,
    changeSetId: entry.parsed.fact.changeSet.id,
    previewHash: entry.parsed.fact.changeSet.previewHash,
    expectedRevision: entry.parsed.fact.changeSet.base.resourceVersion,
  };
}

function canonicalCandidateApplyAction(entry: WorkflowArtifactEntry): { actionId: string; approvalId: string } | undefined {
  const matches = candidateApplyActions(entry);
  return matches.length === 1 ? matches[0] : undefined;
}

function candidateApplyActions(entry: WorkflowArtifactEntry): Array<{ actionId: string; approvalId: string }> {
  if (entry.parsed.kind !== "candidate") return [];
  const binding = candidateActionBinding(entry);
  return (entry.artifact.actions || []).flatMap((action) => {
    const id = canonicalOptionalString(action.id);
    const label = canonicalOptionalString(action.label);
    const target = objectValue(action.target);
    const targetApprovalId = canonicalOptionalString(target.approvalId);
    if (!id
      || !label
      || action.intent !== "workflow.apply_candidate"
      || action.disabled === true
      || !targetApprovalId
      || !targetMatchesCandidate(target, binding)) {
      return [];
    }
    return [{ actionId: id, approvalId: targetApprovalId }];
  });
}

function collectCandidateApprovalOwners(entries: WorkflowArtifactEntry[]): Set<string> {
  const owners = new Set<string>();
  for (const entry of entries) {
    for (const action of candidateApplyActions(entry)) {
      owners.add(candidateApprovalOwnerKey(entry.turn.id, action.approvalId));
    }
  }
  return owners;
}

function pendingCandidateApproval(
  state: AiopsTransportState,
  turn: AiopsTransportTurn,
  approvalId: string,
): boolean {
  const approval = state.pendingApprovals?.[approvalId];
  return Boolean(approval
    && approval.id === approvalId
    && approval.turnId === turn.id
    && (!approval.status || approval.status === "pending" || approval.status === "blocked")
    && state.runtimeLiveness?.pendingApprovals?.[approvalId] === true
    && selectOrderedWorkflowBlocks(turn).some((block) => block.approvalId === approvalId));
}

function candidateApprovalOwnerKey(turnId: string, approvalId: string): string {
  return `${turnId}\u0000${approvalId}`;
}

function candidateApprovalKey(turnId: string, approvalId: string, binding: CandidateActionBinding): string {
  return [
    candidateApprovalOwnerKey(turnId, approvalId),
    binding.candidateId,
    binding.changeSetId,
    binding.previewHash,
    binding.expectedRevision,
  ].join("\u0000");
}

function targetMatchesCandidate(target: Record<string, unknown>, binding: CandidateActionBinding): boolean {
  return canonicalOptionalString(target.candidateId) === binding.candidateId
    && canonicalOptionalString(target.changeSetId) === binding.changeSetId
    && canonicalOptionalString(target.previewHash) === binding.previewHash
    && canonicalOptionalString(target.expectedRevision) === binding.expectedRevision;
}

function candidateBindingMatches(left: CandidateActionBinding, right: WorkflowSurfaceFocusCandidate): boolean {
  return left.candidateId === right.candidateId
    && left.changeSetId === right.changeSetId
    && left.previewHash === right.previewHash
    && left.expectedRevision === right.expectedRevision;
}

function workspaceCandidateMatches(
  candidate: ReturnType<typeof useWorkflowWorkspace>["state"]["candidate"],
  binding: CandidateActionBinding,
): boolean {
  return Boolean(candidate
    && candidate.candidate.id === binding.candidateId
    && candidate.candidate.currentChangeSetId === binding.changeSetId
    && candidate.candidate.currentPreviewHash === binding.previewHash
    && candidate.changeSet.base.resourceVersion === binding.expectedRevision
    && candidate.changeSet.id === binding.changeSetId
    && candidate.changeSet.previewHash === binding.previewHash);
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function canonicalOptionalString(value: unknown): string | undefined {
  return canonicalString(value) ? value : undefined;
}

function canonicalOperationIndex(value: unknown): number | undefined {
  return Number.isInteger(value) && Number(value) >= 0 ? Number(value) : undefined;
}

function canonicalString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value === value.trim();
}

function freezeWorkflowTurnContext(
  workflowId: string,
  workspace: ReturnType<typeof useWorkflowWorkspace>["state"],
  environment: WorkflowEnvironmentFact,
  selection: WorkflowSelectionFact,
): AiopsWorkflowTurnContextRequest | undefined {
  const id = workflowId.trim();
  const committed = workspace.committed;
  const bindingVersion = environment.bindingVersion.trim();
  if (!id || committed?.workflowKey.workflowId !== id || !bindingVersion) return undefined;
  const { resourceVersion, semanticHash, layoutHash } = committed.revision;
  if (![resourceVersion, semanticHash, layoutHash].every((value) => value.trim())) return undefined;
  return {
    workflowId: id,
    revision: { resourceVersion, semanticHash, layoutHash },
    selection: {
      nodeIds: stableIds(selection.nodeIds),
      edgeIds: stableIds(selection.edgeIds),
    },
    environment: {
      ...(environment.name?.trim() ? { name: environment.name.trim() } : {}),
      bindingVersion,
      ...(environment.capabilityVersions?.length ? { capabilityVersions: stableIds(environment.capabilityVersions) } : {}),
    },
  };
}

function stableIds(values: string[]) {
  return [...new Set(values.map((value) => value.trim()).filter(Boolean))].sort();
}

function defaultClientMessageId() {
  return typeof globalThis.crypto?.randomUUID === "function" ? globalThis.crypto.randomUUID() : `workflow-message-${Date.now()}`;
}

const PROCESS_LABELS: Record<string, string> = {
  plan: "计划",
  search: "读取上下文",
  command: "执行命令",
  file: "读取文件",
  tool: "工具",
  evidence: "检查证据",
  approval: "等待确认",
  mcp: "工作流工具",
  system: "系统",
  subagent: "协作任务",
  assistant: "AI",
  reasoning: "处理中",
};
