import type { WorkflowDomainEventV2, WorkflowKey } from "./workflowMcpContracts";
import {
  WORKFLOW_WORKSPACE_STATE_VERSION,
  type WorkflowReceiptFact,
  type WorkflowWorkspaceAction,
  type WorkflowWorkspaceState,
} from "./workflowWorkspaceTypes";

export function createWorkflowWorkspaceState(): WorkflowWorkspaceState {
  return {
    schemaVersion: WORKFLOW_WORKSPACE_STATE_VERSION,
    selection: { nodeIds: [], edgeIds: [] },
    receipts: {}, receiptOrder: [], events: {}, eventOrder: [], bufferedEvents: {},
    lastSequence: 0, validationArtifacts: {}, lifecycleArtifacts: {},
    task: { state: "idle" }, connection: { state: "online" }, runTarget: "draft",
  };
}

export function workflowWorkspaceReducer(state: WorkflowWorkspaceState, action: WorkflowWorkspaceAction): WorkflowWorkspaceState {
  switch (action.type) {
    case "snapshot.loaded":
      return reduceSnapshot(state, action.snapshot);
    case "candidate.received": {
      if (!workspaceAcceptsKey(state, action.candidate.candidate.workflowKey)) return state;
      const current = state.candidate;
      if (current && (
        action.candidate.candidate.currentRevision < current.candidate.currentRevision ||
        (action.candidate.candidate.currentRevision === current.candidate.currentRevision &&
          action.candidate.candidate.currentPreviewHash === current.candidate.currentPreviewHash)
      )) return state;
      return { ...state, workflowKey: action.candidate.candidate.workflowKey, candidate: cloneCandidateFact(action.candidate) };
    }
    case "candidate.cleared":
      if (!state.candidate || (action.candidateId && state.candidate.candidate.id !== action.candidateId)) return state;
      return { ...state, candidate: undefined, approval: undefined };
    case "selection.changed":
      return { ...state, selection: { nodeIds: stableIds(action.selection.nodeIds), edgeIds: stableIds(action.selection.edgeIds), ...(action.selection.anchorId ? { anchorId: action.selection.anchorId } : {}) } };
    case "viewport.changed":
      return { ...state, viewport: { ...action.viewport } };
    case "receipt.received":
      return reduceReceipt(state, action.receipt);
    case "event.received":
      return reduceEvent(state, action.event);
    case "validation.received":
      return { ...state, validationArtifacts: { ...state.validationArtifacts, [action.artifact.artifactId]: structuredClone(action.artifact) } };
    case "lifecycle.received":
      return { ...state, lifecycleArtifacts: { ...state.lifecycleArtifacts, [action.artifact.artifactId]: structuredClone(action.artifact) } };
    case "approval.changed":
      return { ...state, approval: action.approval ? structuredClone(action.approval) : undefined };
    case "task.changed":
      return { ...state, task: { ...action.task } };
    case "connection.changed":
      return { ...state, connection: { ...action.connection } };
    case "run-target.changed":
      return { ...state, runTarget: action.target };
  }
}

function reduceSnapshot(state: WorkflowWorkspaceState, snapshot: WorkflowWorkspaceState["committed"]): WorkflowWorkspaceState {
  if (!snapshot) return state;
  if (state.workflowKey && !sameKey(state.workflowKey, snapshot.workflowKey)) {
    return drainBufferedEvents({ ...createWorkflowWorkspaceState(), connection: state.connection, task: state.task, workflowKey: snapshot.workflowKey, committed: structuredClone(snapshot), lastSequence: snapshot.sequence });
  }
  if (state.committed && snapshot.sequence < state.committed.sequence) return state;
  const bufferedEvents = Object.fromEntries(Object.entries(state.bufferedEvents).filter(([sequence]) => Number(sequence) > snapshot.sequence));
  const matchingReceipt = state.candidate && Object.values(state.receipts).find((receipt) => (
    receipt.changeSetId === state.candidate?.changeSet.id
    && receipt.revisionAfter?.resourceVersion === snapshot.revision.resourceVersion
    && !["conflict", "rejected"].includes(receipt.status)
  ));
  return drainBufferedEvents({
    ...state, workflowKey: snapshot.workflowKey, committed: structuredClone(snapshot),
    ...(matchingReceipt ? { candidate: undefined, approval: undefined } : {}),
    lastSequence: Math.max(state.lastSequence, snapshot.sequence), bufferedEvents,
    gap: state.gap && snapshot.sequence < state.gap.receivedSequence ? state.gap : undefined,
  });
}

function reduceReceipt(state: WorkflowWorkspaceState, receipt: WorkflowReceiptFact): WorkflowWorkspaceState {
  if (!workspaceAcceptsKey(state, receipt.workflowKey)) return state;
  const current = state.receipts[receipt.transactionId];
  if (current) {
    if (!sameReceiptIdentity(current, receipt) || receiptRank(receipt.status) < receiptRank(current.status)) return state;
    if (receipt.status === current.status && JSON.stringify(receipt) === JSON.stringify(current)) return state;
  }
  const reconcilesCandidate = state.candidate
    && state.committed
    && receipt.changeSetId === state.candidate.changeSet.id
    && receipt.revisionAfter?.resourceVersion === state.committed.revision.resourceVersion
    && !["conflict", "rejected"].includes(receipt.status);
  return {
    ...state, workflowKey: receipt.workflowKey,
    ...(reconcilesCandidate ? { candidate: undefined, approval: undefined } : {}),
    receipts: { ...state.receipts, [receipt.transactionId]: structuredClone(receipt) },
    receiptOrder: current ? state.receiptOrder : [...state.receiptOrder, receipt.transactionId],
  };
}

function reduceEvent(state: WorkflowWorkspaceState, event: WorkflowDomainEventV2): WorkflowWorkspaceState {
  if (!workspaceAcceptsKey(state, event.workflowKey) || state.events[event.eventId] || event.sequence <= state.lastSequence) return state;
  if (event.sequence > state.lastSequence + 1) {
    const bufferedEvents = state.bufferedEvents[event.sequence]
      ? state.bufferedEvents
      : { ...state.bufferedEvents, [event.sequence]: structuredClone(event) };
    return { ...state, bufferedEvents, gap: { expectedSequence: state.lastSequence + 1, receivedSequence: Math.min(...Object.keys(bufferedEvents).map(Number)) } };
  }
  return drainBufferedEvents(appendEvent(state, event));
}

function appendEvent(state: WorkflowWorkspaceState, event: WorkflowDomainEventV2): WorkflowWorkspaceState {
  let candidate = state.candidate;
  let approval = state.approval;
  if (event.type === "candidate.superseded" && candidate && (!event.changeSetId || event.changeSetId === candidate.changeSet.id)) {
    candidate = undefined;
    approval = undefined;
  }
  if (event.type === "transaction.applied" && event.transactionId) {
    const receipt = state.receipts[event.transactionId];
    if (candidate && receipt?.changeSetId === candidate.changeSet.id) candidate = undefined;
    if (approval && receipt?.changeSetId === approval.changeSetId) approval = undefined;
  }
  const bufferedEvents = { ...state.bufferedEvents };
  delete bufferedEvents[event.sequence];
  return {
    ...state, candidate, approval, bufferedEvents, lastSequence: event.sequence,
    events: { ...state.events, [event.eventId]: structuredClone(event) },
    eventOrder: [...state.eventOrder, event.eventId], gap: undefined,
  };
}

function drainBufferedEvents(state: WorkflowWorkspaceState): WorkflowWorkspaceState {
  let next = state;
  while (next.bufferedEvents[next.lastSequence + 1]) next = appendEvent(next, next.bufferedEvents[next.lastSequence + 1]);
  const sequences = Object.keys(next.bufferedEvents).map(Number).filter(Number.isFinite).sort((a, b) => a - b);
  return sequences.length > 0
    ? { ...next, gap: { expectedSequence: next.lastSequence + 1, receivedSequence: sequences[0] } }
    : { ...next, gap: undefined };
}

function workspaceAcceptsKey(state: WorkflowWorkspaceState, key: WorkflowKey): boolean {
  return !state.workflowKey || sameKey(state.workflowKey, key);
}

function sameKey(left: WorkflowKey, right: WorkflowKey): boolean {
  return left.tenantId === right.tenantId && left.workflowId === right.workflowId;
}

function sameReceiptIdentity(left: WorkflowReceiptFact, right: WorkflowReceiptFact): boolean {
  return left.changeSetId === right.changeSetId && sameKey(left.workflowKey, right.workflowKey) &&
    left.revisionBefore.resourceVersion === right.revisionBefore.resourceVersion &&
    left.revisionAfter?.resourceVersion === right.revisionAfter?.resourceVersion;
}

function receiptRank(status: WorkflowReceiptFact["status"]): number {
  if (status === "verifying") return 1;
  return 2;
}

function stableIds(values: string[]): string[] {
  return [...new Set(values)].sort();
}

function cloneCandidateFact(value: NonNullable<WorkflowWorkspaceState["candidate"]>): NonNullable<WorkflowWorkspaceState["candidate"]> {
  return structuredClone(value);
}
