import type {
  AiopsTransportApproval,
  AiopsTransportBlock,
  AiopsTransportState,
  AiopsTransportTurn,
} from "@/transport/aiopsTransportTypes";

export type WorkflowTransportScope = {
  workflowId: string;
  threadId?: string;
  sessionId?: string;
};

export type WorkflowTransportActivity = {
  busy: boolean;
  canStop: boolean;
  canRetry: boolean;
  retryTurnId?: string;
};

export function selectOrderedWorkflowBlocks(turn: AiopsTransportTurn): AiopsTransportBlock[] {
  const blocksById = turn.blocksById || {};
  return (turn.blockOrder || []).flatMap((id) => {
    const block = blocksById[id];
    return block ? [block] : [];
  });
}

export function selectWorkflowTransportTurns(
  state: AiopsTransportState,
  scope: WorkflowTransportScope,
): AiopsTransportTurn[] {
  if (scope.threadId && scope.threadId !== state.threadId) return [];
  if (scope.sessionId && scope.sessionId !== state.sessionId) return [];

  const transportIsExplicitlyScoped = Boolean(scope.threadId || scope.sessionId);
  return (state.turnOrder || []).flatMap((id) => {
    const turn = state.turns?.[id];
    if (!turn) return [];
    const contextWorkflowId = turn.workflowContext?.workflowId?.trim();
    if (contextWorkflowId) return contextWorkflowId === scope.workflowId ? [turn] : [];
    return transportIsExplicitlyScoped ? [turn] : [];
  });
}

export function selectWorkflowTransportActivity(state: AiopsTransportState): WorkflowTransportActivity {
  const currentTurnId = state.currentTurnId?.trim();
  const currentTurn = currentTurnId ? state.turns?.[currentTurnId] : undefined;
  const turnIsActive = Boolean(currentTurnId && state.runtimeLiveness?.activeTurns?.[currentTurnId]);
  const busy = state.status === "working"
    || turnIsActive
    || currentTurn?.status === "submitted"
    || currentTurn?.status === "working";
  const hasPendingApproval = Object.values(state.pendingApprovals || {}).some((approval) => (
    isWorkflowApprovalPending(approval.status)
    && (!currentTurnId || !approval.turnId || approval.turnId === currentTurnId)
  ));
  const hasPendingPrompt = hasPendingApproval
    || Object.values(state.runtimeLiveness?.pendingUserInputs || {}).some(Boolean);
  const retryable = state.status === "failed"
    || state.status === "canceled"
    || currentTurn?.status === "failed"
    || currentTurn?.status === "canceled"
    || (state.status === "blocked" && !hasPendingPrompt);
  const terminal = state.status === "failed"
    || state.status === "canceled"
    || currentTurn?.status === "failed"
    || currentTurn?.status === "canceled";

  return {
    busy,
    canStop: !terminal && (busy || hasPendingApproval),
    canRetry: retryable,
    retryTurnId: retryable ? currentTurnId : undefined,
  };
}

export function selectWorkflowPendingApproval(
  state: AiopsTransportState,
  turnId: string,
  approvalId?: string,
): AiopsTransportApproval | undefined {
  const explicit = approvalId ? state.pendingApprovals?.[approvalId] : undefined;
  if (explicit && isWorkflowApprovalPending(explicit.status)) return explicit;
  return Object.values(state.pendingApprovals || {})
    .filter((approval) => approval.turnId === turnId && isWorkflowApprovalPending(approval.status))
    .sort((left, right) => (left.requestedAt || "").localeCompare(right.requestedAt || ""))[0];
}

/** Returns the exact block-owned approval, including resolved user-visible states. */
export function selectWorkflowApproval(
  state: AiopsTransportState,
  turnId: string,
  approvalId?: string,
): AiopsTransportApproval | undefined {
  const canceled = workflowTurnIsCanceled(state, turnId);
  if (approvalId) {
    const explicit = state.pendingApprovals?.[approvalId];
    if (explicit && (!explicit.turnId || explicit.turnId === turnId)) {
      return canceled && isWorkflowApprovalPending(explicit.status)
        ? { ...explicit, status: "canceled" }
        : explicit;
    }
    if (canceled) return { id: approvalId, turnId, status: "canceled" };
    return undefined;
  }
  const pending = selectWorkflowPendingApproval(state, turnId);
  return canceled && pending ? { ...pending, status: "canceled" } : pending;
}

function isWorkflowApprovalPending(status: unknown): boolean {
  if (status === undefined || status === null || status === "") return true;
  if (typeof status !== "string") return false;
  return ["pending", "requested", "blocked"].includes(status.trim().toLowerCase());
}

function workflowTurnIsCanceled(state: AiopsTransportState, turnId: string): boolean {
  return state.turns?.[turnId]?.status === "canceled"
    || (state.currentTurnId === turnId && state.status === "canceled");
}
