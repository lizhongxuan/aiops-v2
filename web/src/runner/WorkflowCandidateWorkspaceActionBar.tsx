import { useAssistantTransportSendCommand, useAssistantTransportState } from "@assistant-ui/react";
import { AlertTriangle, ShieldCheck, Trash2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";

import {
  type WorkflowChangeActionEventDetail,
} from "@/components/chat/WorkflowChangeArtifact";
import { RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT } from "@/components/runner/RunnerCanvas";
import { WorkflowCandidateActionBar, type WorkflowCandidateActionBinding } from "@/runner/WorkflowCandidateActionBar";
import {
  dispatchWorkflowCandidateApplyConfirmation,
  WorkflowCandidateApplyDialog,
  type WorkflowCandidateApplyConfirmation,
} from "@/runner/WorkflowCandidateApplyDialog";
import { selectOrderedWorkflowBlocks, selectWorkflowTransportTurns } from "@/runner/transport/workflowTransportSelectors";
import { workflowTransportIdentity } from "@/runner/transport/WorkflowTransportBoundary";
import { useWorkflowWorkspace } from "@/runner/workspace/WorkflowWorkspaceProvider";
import { parseWorkflowTransportArtifact } from "@/runner/workspace/workflowCandidateArtifact";
import type { WorkflowCandidateFact, WorkflowWorkspaceState } from "@/runner/workspace/workflowWorkspaceTypes";
import type {
  AiopsTransportAgentUiArtifact,
  AiopsTransportState,
  AiopsTransportTurn,
} from "@/transport/aiopsTransportTypes";
import type { AiopsTransportCommand } from "@/transport/aiopsTransportRuntime";

export type WorkflowCandidateWorkspaceActionBarProps = {
  className?: string;
};

type CandidatePresentation = {
  binding: WorkflowCandidateActionBinding;
  candidateRevision: number;
  changeCount: number;
  expiresAt?: string;
  source: "history" | "revised" | "transport";
};

type AuthorizedApply = {
  source: "artifact" | "server-controlled";
  artifactId: string;
  action: WorkflowChangeActionEventDetail["action"];
  approvalId: string;
  sessionId: string;
  turnId: string;
};

/**
 * Projects the current authoritative Workspace candidate into the shared action
 * bar. Apply remains a transport-owned operation: this bridge only forwards a
 * canonical server action after its artifact, turn and live approval all agree.
 */
export function WorkflowCandidateWorkspaceActionBar({ className = "" }: WorkflowCandidateWorkspaceActionBarProps) {
  const transport = useAssistantTransportState() as AiopsTransportState;
  const sendTransportCommand = useAssistantTransportSendCommand();
  const workspace = useWorkflowWorkspace();
  const candidate = workspace.state.candidate;
  const candidateOwnerRef = useRef<HTMLDivElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const wasDialogOpenRef = useRef(false);
  const approvalRequestRef = useRef<{ identity: string; request: Promise<unknown> } | undefined>(undefined);
  const decidedControlledApprovalsRef = useRef(new Set<string>());
  const [confirmation, setConfirmation] = useState<WorkflowCandidateApplyConfirmation>();
  const presentation = useMemo(
    () => candidatePresentation(workspace.state),
    [workspace.state],
  );
  const authorized = useMemo(
    () => presentation
      ? resolveAuthorizedApply(transport, workspace.state, presentation, workspace.candidateApplyApproval)
      : undefined,
    [presentation, transport, workspace.candidateApplyApproval, workspace.state],
  );
  const candidateApplyAttemptPresent = useMemo(
    () => presentation ? hasCandidateApplyAttempt(transport, workspace.state, presentation.binding) : false,
    [presentation, transport, workspace.state],
  );
  const requestedCandidateApproval = presentation
    ? candidateApplyApprovalMatches(workspace.candidateApplyApproval, presentation)
    : false;
  const requestApprovalOnce = useCallback((): Promise<unknown> | undefined => {
    if (!presentation
      || candidateApplyAttemptPresent
      || requestedCandidateApproval
      || !workspace.requestCandidateApplyApproval) return undefined;
    const identity = candidateApprovalRequestIdentity(presentation);
    const inFlight = approvalRequestRef.current;
    if (inFlight?.identity === identity) return inFlight.request;
    const request = Promise.resolve().then(() => workspace.requestCandidateApplyApproval!());
    approvalRequestRef.current = { identity, request };
    void request.finally(() => {
      if (approvalRequestRef.current?.request === request) approvalRequestRef.current = undefined;
    }).catch(() => undefined);
    return request;
  }, [
    candidateApplyAttemptPresent,
    presentation,
    requestedCandidateApproval,
    workspace.requestCandidateApplyApproval,
  ]);
  const openConfirmation = useCallback((next: AuthorizedApply, current: CandidatePresentation) => {
    const applyOwner = candidateOwnerRef.current?.querySelector<HTMLElement>('[data-testid="workflow-candidate-apply"]');
    if (applyOwner) returnFocusRef.current = applyOwner;
    setConfirmation({
      schemaVersion: "aiops.workflow.candidate-apply-confirmation.v1",
      artifactId: next.artifactId,
      actionId: next.action.id,
      approvalId: next.approvalId,
      binding: current.binding,
      changeCount: current.changeCount,
    });
  }, []);
  const confirmApply = useCallback((confirmed: WorkflowCandidateApplyConfirmation) => {
    if (!presentation || !authorized
      || !confirmationMatchesAuthorization(confirmed, authorized, presentation)) return;
    if (authorized.source === "artifact") {
      dispatchWorkflowCandidateApplyConfirmation(confirmed);
      return;
    }

    // Re-evaluate the live transport snapshot at the irreversible boundary.
    // The HTTP response proves the exact Candidate binding, while only this
    // current blocked transport turn proves that the approval is still live.
    const latest = authorizedServerControlledApply(
      transport,
      workspace.state,
      presentation,
      workspace.candidateApplyApproval,
    );
    if (!latest || !sameAuthorization(latest, authorized)) return;
    const decisionIdentity = controlledApprovalIdentity(latest, presentation.binding);
    if (decidedControlledApprovalsRef.current.has(decisionIdentity)) return;
    decidedControlledApprovalsRef.current.add(decisionIdentity);
    (sendTransportCommand as unknown as (command: AiopsTransportCommand) => void)({
      type: "aiops.approval-decision",
      sessionId: latest.sessionId,
      turnId: latest.turnId,
      approvalId: latest.approvalId,
      decision: "accept",
    });
  }, [
    authorized,
    presentation,
    sendTransportCommand,
    transport,
    workspace.candidateApplyApproval,
    workspace.state,
  ]);

  useEffect(() => {
    if (!presentation) return;
    const handleCandidateCommand = (event: Event) => {
      const detail = (event as CustomEvent<Record<string, unknown>>).detail;
      if (!detail || detail.command !== "apply_candidate"
        || detail.candidateId !== presentation.binding.candidateId
        || detail.changeSetId !== presentation.binding.changeSetId
        || detail.previewHash !== presentation.binding.previewHash) return;
      if (authorized) {
        openConfirmation(authorized, presentation);
        return;
      }
      void requestApprovalOnce()?.catch(() => undefined);
    };
    window.addEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, handleCandidateCommand);
    return () => window.removeEventListener(RUNNER_CANVAS_CANDIDATE_COMMAND_EVENT, handleCandidateCommand);
  }, [authorized, openConfirmation, presentation, requestApprovalOnce]);

  useEffect(() => {
    if (!confirmation) return;
    if (!authorized || !presentation || !confirmationMatchesAuthorization(confirmation, authorized, presentation)) {
      setConfirmation(undefined);
    }
  }, [authorized, confirmation, presentation]);

  useEffect(() => {
    const wasOpen = wasDialogOpenRef.current;
    wasDialogOpenRef.current = Boolean(confirmation);
    if (wasOpen && !confirmation) {
      const frame = window.requestAnimationFrame(() => {
        candidateReturnFocusTarget(candidateOwnerRef.current, returnFocusRef.current)?.focus({ preventScroll: true });
      });
      return () => window.cancelAnimationFrame(frame);
    }
  }, [confirmation]);

  if (!candidate) return null;
  if (!presentation) {
    return (
      <aside
        className={`border-t border-amber-200 bg-amber-50 px-4 py-3 text-sm text-amber-950 ${className}`.trim()}
        data-testid="workflow-candidate-recovery-hint"
        role="status"
      >
        <span className="inline-flex items-start gap-2">
          <AlertTriangle className="mt-0.5 h-4 w-4 shrink-0" aria-hidden="true" />
          当前候选信息不完整，无法安全应用。请让 AI 重新生成待确认修改。
        </span>
      </aside>
    );
  }

  if (presentation.source === "history" && !authorized) {
    const requesting = workspace.candidateActionPending?.action === "request_approval";
    const busy = Boolean(workspace.candidateActionPending);
    return (
      <div ref={candidateOwnerRef} className={className} data-testid="workflow-candidate-workspace-actions">
        <p
          className={`border-t px-4 py-3 text-sm leading-6 ${requestedCandidateApproval
            ? "border-sky-200 bg-sky-50 text-sky-950"
            : "border-slate-200 bg-slate-50 text-slate-700"}`}
          data-testid="workflow-history-approval-guide"
          role="status"
        >
          {requestedCandidateApproval
            ? "确认请求已发送。服务端确认步骤就绪后，请在本操作栏完成最终确认；在你确认前，画布不会被修改。"
            : "撤销/重做候选已准备好。请求确认后，将在本操作栏完成最终确认。"}
        </p>
        <aside
          className="flex flex-wrap items-center justify-between gap-3 border-t border-slate-200 bg-white px-4 py-3"
          aria-label="事务恢复候选操作"
          data-testid="workflow-history-candidate-actions"
        >
          <div className="min-w-0">
            <p className="text-sm font-semibold text-slate-950">{presentation.changeCount} 项事务恢复修改尚未应用</p>
            <p className="text-xs text-slate-500">服务端会重新核对候选版本；确认前不会修改已保存工作流。</p>
          </div>
          <div className="flex items-center gap-2">
            <button
              type="button"
              disabled={busy}
              className="inline-flex min-h-10 items-center gap-2 rounded-lg px-3 text-sm font-medium text-slate-700 hover:bg-slate-100 disabled:text-slate-300"
              onClick={() => void workspace.discardCandidate().catch(() => undefined)}
            >
              <Trash2 className="h-4 w-4" aria-hidden="true" />放弃修改
            </button>
            <button
              type="button"
              data-testid="workflow-candidate-request-approval"
              disabled={busy || requestedCandidateApproval || !workspace.requestCandidateApplyApproval}
              className="inline-flex min-h-10 items-center gap-2 rounded-lg bg-slate-950 px-4 text-sm font-medium text-white disabled:bg-slate-200 disabled:text-slate-500"
              onClick={() => void requestApprovalOnce()?.catch(() => undefined)}
            >
              <ShieldCheck className="h-4 w-4" aria-hidden="true" />
              {requesting ? "正在请求确认" : requestedCandidateApproval ? "等待最终确认" : "请求应用确认"}
            </button>
          </div>
        </aside>
      </div>
    );
  }

  if (presentation.source === "revised" && !authorized) {
    const requesting = workspace.candidateActionPending?.action === "request_approval";
    const busy = Boolean(workspace.candidateActionPending);
    return (
      <div ref={candidateOwnerRef} className={className} data-testid="workflow-candidate-workspace-actions">
        <p
          className={`border-t px-4 py-3 text-sm leading-6 ${requestedCandidateApproval
            ? "border-sky-200 bg-sky-50 text-sky-950"
            : "border-slate-200 bg-slate-50 text-slate-700"}`}
          data-testid="workflow-revised-approval-guide"
          role="status"
        >
          {requestedCandidateApproval
            ? "最新候选版本的确认请求已发送。服务端确认步骤就绪后，请在本操作栏完成最终确认；确认前不会修改已保存工作流。"
            : "你已直接更新 AI 候选。应用时只会为最新候选版本请求确认，之前的版本不能继续使用。"}
        </p>
        {requesting ? (
          <p className="border-t border-sky-200 bg-sky-50 px-4 py-2 text-xs leading-5 text-sky-950" role="status" data-testid="workflow-candidate-action-pending">
            正在向 AI 面板发送最新候选版本的确认步骤，已保存版本不会改变。
          </p>
        ) : null}
        <WorkflowCandidateActionBar
          binding={presentation.binding}
          approval={{
            status: requestedCandidateApproval ? "requested" : "ready",
            binding: presentation.binding,
            ...(requestedCandidateApproval ? { approvalId: workspace.candidateApplyApproval!.approvalId } : {}),
            ...(presentation.expiresAt ? { expiresAt: presentation.expiresAt } : {}),
          }}
          changeCount={presentation.changeCount}
          onApply={!busy && !requestedCandidateApproval && workspace.requestCandidateApplyApproval
            ? requestApprovalOnce
            : undefined}
          onDiscard={busy ? undefined : () => workspace.discardCandidate()}
        />
      </div>
    );
  }

  return (
    <>
      <div
        ref={candidateOwnerRef}
        className={className}
        data-testid="workflow-candidate-workspace-actions"
        aria-hidden={Boolean(confirmation)}
        inert={Boolean(confirmation)}
      >
        {!authorized ? (
          <p
            className={`flex items-start gap-2 border-t px-4 py-3 text-sm leading-6 ${
              requestedCandidateApproval
                ? "border-sky-200 bg-sky-50 text-sky-950"
                : candidateApplyAttemptPresent
                  ? "border-amber-200 bg-amber-50 text-amber-950"
                  : "border-slate-200 bg-slate-50 text-slate-700"
            }`}
            data-testid="workflow-candidate-recovery-hint"
            role="status"
          >
            <AlertTriangle className="mt-1 h-4 w-4 shrink-0" aria-hidden="true" />
            <span>{requestedCandidateApproval
              ? "确认请求已发送，正在等待与当前候选精确绑定的服务端确认步骤。"
              : candidateApplyAttemptPresent
                ? "等待确认步骤：当前候选还没有与本轮、当前版本精确绑定的确认。你可以放弃修改，或让 AI 重新生成确认步骤。"
                : "应用前需要服务端确认。选择应用只会先请求确认，不会修改已保存工作流。"}</span>
          </p>
        ) : null}
        {workspace.candidateActionPending ? (
          <p className="border-t border-sky-200 bg-sky-50 px-4 py-2 text-xs leading-5 text-sky-950" role="status" data-testid="workflow-candidate-action-pending">
            {workspace.candidateActionPending.action === "revise"
              ? "正在更新候选版本，已保存版本不会改变。"
              : workspace.candidateActionPending.action === "request_approval"
                ? "正在向 AI 面板发送确认步骤，已保存版本不会改变。"
                : "正在放弃候选修改，已保存版本不会改变。"}
          </p>
        ) : null}
        <WorkflowCandidateActionBar
          binding={presentation.binding}
          approval={{
            status: authorized || requestedCandidateApproval ? "requested" : "ready",
            binding: presentation.binding,
            ...(authorized
              ? { approvalId: authorized.approvalId }
              : requestedCandidateApproval
                ? { approvalId: workspace.candidateApplyApproval!.approvalId }
                : {}),
            ...(presentation.expiresAt ? { expiresAt: presentation.expiresAt } : {}),
          }}
          changeCount={presentation.changeCount}
          onApply={!workspace.candidateActionPending
            ? authorized
              ? () => openConfirmation(authorized, presentation)
              : !candidateApplyAttemptPresent
                && !requestedCandidateApproval
                && workspace.requestCandidateApplyApproval
                ? requestApprovalOnce
                : undefined
            : undefined}
          onDiscard={workspace.candidateActionPending ? undefined : () => workspace.discardCandidate()}
        />
      </div>
      {confirmation ? (
        <WorkflowCandidateApplyDialog
          confirmation={confirmation}
          open
          onOpenChange={(open) => {
            if (!open) setConfirmation(undefined);
          }}
          onConfirm={confirmApply}
          returnFocusRef={returnFocusRef}
        />
      ) : null}
    </>
  );
}

function candidatePresentation(state: WorkflowWorkspaceState): CandidatePresentation | undefined {
  const committed = state.committed;
  const fact = state.candidate;
  if (!committed || !fact) return undefined;
  const { candidate, changeSet } = fact;
  if (!sameWorkflowKey(committed.workflowKey, state.workflowKey)
    || !sameWorkflowKey(committed.workflowKey, candidate.workflowKey)
    || !sameWorkflowKey(committed.workflowKey, changeSet.workflowKey)
    || !sameRevision(committed.revision, changeSet.base)
    || !canonicalString(candidate.id)
    || !canonicalString(changeSet.id)
    || !canonicalString(changeSet.previewHash)
    || !canonicalString(committed.revision.resourceVersion)
    || candidate.id !== changeSet.candidateId
    || candidate.currentChangeSetId !== changeSet.id
    || candidate.currentPreviewHash !== changeSet.previewHash
    || candidate.currentRevision !== changeSet.candidateRevision
    || !Number.isSafeInteger(candidate.currentRevision)
    || candidate.currentRevision <= 0
    || !Array.isArray(changeSet.operations)
    || changeSet.operations.length <= 0) {
    return undefined;
  }
  return {
    binding: {
      candidateId: candidate.id,
      changeSetId: changeSet.id,
      previewHash: changeSet.previewHash,
      expectedRevision: committed.revision.resourceVersion,
    },
    candidateRevision: candidate.currentRevision,
    changeCount: changeSet.operations.length,
    source: changeSet.actor === "user" && candidate.currentRevision === 1 && /^history-[a-f0-9]{64}$/.test(candidate.id)
      ? "history"
      : changeSet.actor === "user" && candidate.currentRevision > 1
        && canonicalString(changeSet.parentChangeSetId)
        && !/^history-[a-f0-9]{64}$/.test(candidate.id)
        ? "revised"
        : "transport",
    ...(canonicalDate(changeSet.expiresAt) ? { expiresAt: changeSet.expiresAt } : {}),
  };
}

function candidateApplyApprovalMatches(
  approval: ReturnType<typeof useWorkflowWorkspace>["candidateApplyApproval"],
  presentation: CandidatePresentation,
): boolean {
  const binding = presentation.binding;
  return Boolean(approval
    && approval.status === "requested"
    && approval.candidateRevision === presentation.candidateRevision
    && approval.candidateId === binding.candidateId
    && approval.changeSetId === binding.changeSetId
    && approval.previewHash === binding.previewHash
    && approval.expectedRevision === binding.expectedRevision);
}

function candidateApprovalRequestIdentity(presentation: CandidatePresentation): string {
  return [
    presentation.binding.candidateId,
    presentation.candidateRevision,
    presentation.binding.changeSetId,
    presentation.binding.previewHash,
    presentation.binding.expectedRevision,
  ].join("\u0000");
}

function resolveAuthorizedApply(
  transport: AiopsTransportState,
  state: WorkflowWorkspaceState,
  presentation: CandidatePresentation,
  approval: ReturnType<typeof useWorkflowWorkspace>["candidateApplyApproval"],
): AuthorizedApply | undefined {
  const artifactAuthorized = authorizedApply(transport, state, presentation.binding);
  const controlledAuthorized = authorizedServerControlledApply(transport, state, presentation, approval);
  if (artifactAuthorized && controlledAuthorized) {
    return sameAuthorization(artifactAuthorized, controlledAuthorized) ? artifactAuthorized : undefined;
  }
  if (artifactAuthorized) return artifactAuthorized;
  if (!controlledAuthorized) return undefined;

  // A malformed or competing Apply action must never be repaired by the
  // server-controlled bridge. That path exists only because controlled turns
  // intentionally contain no browser-owned Candidate action.
  return hasCandidateApplyAttempt(transport, state, presentation.binding)
    ? undefined
    : controlledAuthorized;
}

function authorizedApply(
  transport: AiopsTransportState,
  state: WorkflowWorkspaceState,
  binding: WorkflowCandidateActionBinding,
): AuthorizedApply | undefined {
  const committed = state.committed;
  if (!committed || !state.candidate) return undefined;
  const workflowId = committed.workflowKey.workflowId;
  if (!canonicalString(workflowId)) return undefined;
  const identity = workflowTransportIdentity(workflowId);
  const turns = selectWorkflowTransportTurns(transport, { workflowId, threadId: identity.threadId });
  const entries = turns.flatMap((turn) => candidateEntries(turn, transport, state.candidate!, committed, binding));

  // More than one transport artifact can otherwise make approval ownership
  // ambiguous. Replayed or competing artifacts must be recovered server-side.
  if (entries.length !== 1) return undefined;
  const entry = entries[0];
  const actions = canonicalApplyActions(entry.artifact, binding);
  if (actions.length !== 1) return undefined;
  const action = actions[0];
  const approvalId = canonicalTargetString(action.target, "approvalId");
  if (!approvalId) return undefined;

  const ordered = selectOrderedWorkflowBlocks(entry.turn);
  const artifactIndex = ordered.findIndex((block) => block === entry.block);
  const approvals = ordered.filter((block, index) => index > artifactIndex && block.approvalId === approvalId);
  if (artifactIndex < 0 || approvals.length !== 1) return undefined;
  const approval = transport.pendingApprovals?.[approvalId];
  const approvalStatus = approval?.status;
  if (!approval
    || approval.id !== approvalId
    || approval.turnId !== entry.turn.id
    || (approvalStatus && approvalStatus !== "pending" && approvalStatus !== "blocked")
    || transport.runtimeLiveness?.pendingApprovals?.[approvalId] !== true) {
    return undefined;
  }
  if (!canonicalString(transport.sessionId)) return undefined;
  return {
    source: "artifact",
    artifactId: entry.artifact.id,
    action,
    approvalId,
    sessionId: transport.sessionId,
    turnId: entry.turn.id,
  };
}

function authorizedServerControlledApply(
  transport: AiopsTransportState,
  state: WorkflowWorkspaceState,
  presentation: CandidatePresentation,
  approval: ReturnType<typeof useWorkflowWorkspace>["candidateApplyApproval"],
): AuthorizedApply | undefined {
  const committed = state.committed;
  if (!committed || !state.candidate || presentation.source === "transport"
    || !candidateApplyApprovalMatches(approval, presentation)
    || !approval
    || !canonicalDate(approval.requestedAt)
    || (approval.expiresAt !== undefined
      && (!canonicalDate(approval.expiresAt) || Date.parse(approval.expiresAt) <= Date.now()))
    || !canonicalDate(presentation.expiresAt)
    || Date.parse(presentation.expiresAt) <= Date.now()
    || !canonicalString(approval.sessionId)
    || !canonicalString(approval.turnId)
    || !canonicalString(approval.approvalId)
    || transport.schemaVersion !== "aiops.transport.v2"
    || transport.status !== "blocked"
    || transport.sessionId !== approval.sessionId
    || transport.threadId !== workflowTransportIdentity(committed.workflowKey.workflowId).threadId
    || transport.currentTurnId !== approval.turnId) {
    return undefined;
  }

  const turn = transport.turns?.[approval.turnId];
  if (!turn
    || turn.id !== approval.turnId
    || turn.status !== "blocked"
    || turn.workflowContext?.workflowId !== committed.workflowKey.workflowId
    || !sameRevision(turn.workflowContext.revision, committed.revision)) {
    return undefined;
  }

  const pending = transport.pendingApprovals?.[approval.approvalId];
  const livePending = Object.values(transport.pendingApprovals || {}).filter((candidate) => (
    candidate
    && canonicalString(candidate.id)
    && canonicalString(candidate.turnId)
    && isPendingApprovalStatus(candidate.status)
    && transport.runtimeLiveness?.pendingApprovals?.[candidate.id] === true
  ));
  if (livePending.length !== 1
    || livePending[0] !== pending
    || !pending
    || pending.id !== approval.approvalId
    || pending.turnId !== approval.turnId
    || !isPendingApprovalStatus(pending.status)
    || transport.runtimeLiveness?.pendingApprovals?.[approval.approvalId] !== true) {
    return undefined;
  }

  const ownedApprovalBlocks = selectOrderedWorkflowBlocks(turn).filter((block) => (
    block.approvalId === approval.approvalId
    && (block.type === "approval" || block.kind === "approval")
    && isPendingApprovalStatus(block.status)
  ));
  if (ownedApprovalBlocks.length !== 1) return undefined;

  const action: WorkflowChangeActionEventDetail["action"] = {
    id: `server-controlled-apply:${approval.approvalId}`,
    intent: "workflow.apply_candidate",
    label: "应用修改",
    target: { ...presentation.binding, approvalId: approval.approvalId },
  };
  return {
    source: "server-controlled",
    artifactId: `server-controlled:${approval.turnId}`,
    action,
    approvalId: approval.approvalId,
    sessionId: approval.sessionId,
    turnId: approval.turnId,
  };
}

function hasCandidateApplyAttempt(
  transport: AiopsTransportState,
  state: WorkflowWorkspaceState,
  binding: WorkflowCandidateActionBinding,
): boolean {
  const committed = state.committed;
  const current = state.candidate;
  if (!committed || !current || !canonicalString(committed.workflowKey.workflowId)) return false;
  const workflowId = committed.workflowKey.workflowId;
  const identity = workflowTransportIdentity(workflowId);
  const turns = selectWorkflowTransportTurns(transport, { workflowId, threadId: identity.threadId });
  return turns.some((turn) => candidateEntries(turn, transport, current, committed, binding)
    .some(({ artifact }) => (artifact.actions || [])
      .some((action) => action.intent === "workflow.apply_candidate")));
}

function candidateEntries(
  turn: AiopsTransportTurn,
  transport: AiopsTransportState,
  current: WorkflowCandidateFact,
  committed: NonNullable<WorkflowWorkspaceState["committed"]>,
  binding: WorkflowCandidateActionBinding,
) {
  return selectOrderedWorkflowBlocks(turn).flatMap((block) => {
    const artifact = block.type === "artifact" ? block.artifact : undefined;
    if (!artifact) return [];
    const receivedAt = firstCanonicalDate(artifact.updatedAt, artifact.createdAt, turn.updatedAt, transport.updatedAt);
    if (!receivedAt) return [];
    const parsed = parseWorkflowTransportArtifact(artifact, {
      workflowKey: committed.workflowKey,
      committedRevision: committed.revision,
      receivedAt,
    });
    if (parsed?.kind !== "candidate" || !sameCandidateFact(parsed.fact, current) || !sameBinding(parsed.fact, binding)) return [];
    return [{ turn, block, artifact }];
  });
}

function canonicalApplyActions(
  artifact: AiopsTransportAgentUiArtifact,
  binding: WorkflowCandidateActionBinding,
): WorkflowChangeActionEventDetail["action"][] {
  return (artifact.actions || []).flatMap((source) => {
    const id = canonicalValue(source.id);
    const label = canonicalValue(source.label);
    if (!id || !label || source.intent !== "workflow.apply_candidate" || source.disabled === true) return [];
    const target = objectValue(source.target);
    if (!targetMatchesBinding(target, binding) || !canonicalTargetString(target, "approvalId")) return [];
    return [{
      ...source,
      id,
      intent: "workflow.apply_candidate",
      label,
      target: { ...target },
    } as WorkflowChangeActionEventDetail["action"]];
  });
}

function confirmationMatchesAuthorization(
  confirmation: WorkflowCandidateApplyConfirmation,
  authorized: AuthorizedApply,
  presentation: CandidatePresentation,
): boolean {
  return confirmation.artifactId === authorized.artifactId
    && confirmation.actionId === authorized.action.id
    && confirmation.approvalId === authorized.approvalId
    && confirmation.changeCount === presentation.changeCount
    && sameActionBinding(confirmation.binding, presentation.binding);
}

function sameAuthorization(left: AuthorizedApply, right: AuthorizedApply): boolean {
  return left.approvalId === right.approvalId
    && left.sessionId === right.sessionId
    && left.turnId === right.turnId
    && sameActionBinding(
      objectValue(left.action.target) as WorkflowCandidateActionBinding,
      objectValue(right.action.target) as WorkflowCandidateActionBinding,
    );
}

function controlledApprovalIdentity(
  authorized: AuthorizedApply,
  binding: WorkflowCandidateActionBinding,
): string {
  return [
    authorized.sessionId,
    authorized.turnId,
    authorized.approvalId,
    binding.candidateId,
    binding.changeSetId,
    binding.previewHash,
    binding.expectedRevision,
  ].join("\u0000");
}

function isPendingApprovalStatus(value: unknown): boolean {
  return value === "pending" || value === "requested" || value === "blocked";
}

function candidateReturnFocusTarget(
  owner: HTMLElement | null,
  previous: HTMLElement | null,
): HTMLElement | null {
  if (canReceiveCandidateFocus(previous)) return previous;
  const current = owner?.querySelector<HTMLElement>(
    '[data-testid="workflow-candidate-apply"]:not(:disabled), [data-testid="workflow-candidate-request-approval"]:not(:disabled)',
  );
  if (canReceiveCandidateFocus(current)) return current;
  const workbench = document.querySelector<HTMLElement>('[data-testid="workflow-workbench-trigger"]');
  return canReceiveCandidateFocus(workbench) ? workbench : null;
}

function canReceiveCandidateFocus(target: HTMLElement | null): target is HTMLElement {
  return Boolean(target
    && target.isConnected
    && !target.matches(":disabled")
    && !target.closest('[inert], [aria-hidden="true"]'));
}

function sameActionBinding(left: WorkflowCandidateActionBinding, right: WorkflowCandidateActionBinding): boolean {
  return left.candidateId === right.candidateId
    && left.changeSetId === right.changeSetId
    && left.previewHash === right.previewHash
    && left.expectedRevision === right.expectedRevision;
}

function sameCandidateFact(left: WorkflowCandidateFact, right: WorkflowCandidateFact): boolean {
  return left.candidate.id === right.candidate.id
    && left.candidate.currentRevision === right.candidate.currentRevision
    && left.candidate.currentChangeSetId === right.candidate.currentChangeSetId
    && left.candidate.currentPreviewHash === right.candidate.currentPreviewHash
    && left.changeSet.id === right.changeSet.id
    && left.changeSet.previewHash === right.changeSet.previewHash
    && sameRevision(left.changeSet.base, right.changeSet.base)
    && sameWorkflowKey(left.changeSet.workflowKey, right.changeSet.workflowKey);
}

function sameBinding(fact: WorkflowCandidateFact, binding: WorkflowCandidateActionBinding): boolean {
  return fact.candidate.id === binding.candidateId
    && fact.changeSet.id === binding.changeSetId
    && fact.changeSet.previewHash === binding.previewHash
    && fact.changeSet.base.resourceVersion === binding.expectedRevision;
}

function targetMatchesBinding(target: Record<string, unknown>, binding: WorkflowCandidateActionBinding): boolean {
  return canonicalTargetString(target, "candidateId") === binding.candidateId
    && canonicalTargetString(target, "changeSetId") === binding.changeSetId
    && canonicalTargetString(target, "previewHash") === binding.previewHash
    && canonicalTargetString(target, "expectedRevision") === binding.expectedRevision;
}

function sameWorkflowKey(
  left: WorkflowWorkspaceState["workflowKey"],
  right: WorkflowWorkspaceState["workflowKey"],
): boolean {
  return Boolean(left && right
    && canonicalString(left.tenantId) && canonicalString(left.workflowId)
    && left.tenantId === right.tenantId && left.workflowId === right.workflowId);
}

function sameRevision(
  left: NonNullable<WorkflowWorkspaceState["committed"]>["revision"],
  right: NonNullable<WorkflowWorkspaceState["committed"]>["revision"],
): boolean {
  return canonicalString(left.resourceVersion)
    && canonicalString(left.semanticHash)
    && canonicalString(left.layoutHash)
    && left.resourceVersion === right.resourceVersion
    && left.semanticHash === right.semanticHash
    && left.layoutHash === right.layoutHash;
}

function objectValue(value: unknown): Record<string, unknown> {
  return value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
}

function canonicalTargetString(value: Record<string, unknown>, key: string): string | undefined {
  return canonicalValue(value[key]);
}

function canonicalValue(value: unknown): string | undefined {
  return canonicalString(value) ? value : undefined;
}

function canonicalString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value === value.trim();
}

function canonicalDate(value: unknown): value is string {
  return canonicalString(value) && !Number.isNaN(Date.parse(value));
}

function firstCanonicalDate(...values: unknown[]): string | undefined {
  return values.find(canonicalDate) as string | undefined;
}
