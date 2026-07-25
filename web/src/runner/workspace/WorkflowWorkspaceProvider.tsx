import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useReducer,
  useRef,
  useState,
  type ReactNode,
} from "react";

import runnerStudioClient from "@/api/runnerStudioClient";
import type {
  WorkflowDomainEventV2,
  WorkflowJsonValue,
  WorkflowKey,
  WorkflowOperation,
  WorkflowRevision,
  WorkflowDiagnostic,
  WorkflowValidationResult,
} from "./workflowMcpContracts";
import { parseWorkflowPatchRequestV2 } from "./workflowMcpContracts";
import { createWorkflowWorkspaceState, workflowWorkspaceReducer } from "./workflowWorkspaceReducer";
import {
  parseWorkflowPreviewResult,
  workflowTransportArtifactIdentity,
  type WorkflowTransportArtifactFact,
} from "./workflowCandidateArtifact";
import type { WorkflowDurableHistoryEntry } from "@/runner/WorkflowHistoryPanel";
import type { WorkflowRunGateFacts } from "./workflowWorkspaceSelectors";
import {
  emitWorkflowTransportResume,
  workflowTransportIdentity,
  WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION,
} from "@/runner/transport/WorkflowTransportBoundary";
import type {
  WorkflowCommittedSnapshotFact,
  WorkflowCandidateFact,
  WorkflowLifecycleArtifactFact,
  WorkflowReceiptFact,
  WorkflowSelectionFact,
  WorkflowValidationArtifactFact,
  WorkflowViewportFact,
  WorkflowWorkspaceState,
} from "./workflowWorkspaceTypes";

const WORKFLOW_EDIT_SCHEMA_VERSION = "aiops.workflow.edit.v2" as const;
const systemNow = () => new Date();

type JsonObject = Record<string, unknown>;
type MoveLayout = {
  nodes: Array<{ nodeId: string; position: { x: number; y: number } }>;
  viewport?: WorkflowViewportFact;
};

type WorkflowCandidateBinding = {
  workflowKey: WorkflowKey;
  expected: WorkflowRevision;
  candidateBase: WorkflowRevision;
  candidateId: string;
  candidateRevision: number;
  changeSetId: string;
  previewHash: string;
  fact: WorkflowCandidateFact;
};

export type WorkflowManualCommand =
  | { kind: "add_node"; node: JsonObject }
  | { kind: "update_node"; nodeId: string; nodePatch: JsonObject }
  | { kind: "delete_node"; nodeId: string; cascadeEdges: boolean }
  | { kind: "connect_edge"; edge: JsonObject }
  | { kind: "update_edge"; edgeId: string; edgePatch: JsonObject }
  | { kind: "disconnect_edge"; edgeId: string }
  | { kind: "move"; layout: MoveLayout };

type WorkflowCommandEnvelopeBase = {
  schemaVersion: typeof WORKFLOW_EDIT_SCHEMA_VERSION;
  workflowId: string;
  expectedRevision: string;
  idempotencyKey: string;
  reason: string;
  confirmed?: boolean;
};

export type WorkflowSingleCommandEnvelope = WorkflowCommandEnvelopeBase & {
  command: WorkflowManualCommand;
  commands?: never;
};

export type WorkflowBatchCommandEnvelope = WorkflowCommandEnvelopeBase & {
  command?: never;
  commands: WorkflowManualCommand[];
};

export type WorkflowCommandEnvelope = WorkflowSingleCommandEnvelope | WorkflowBatchCommandEnvelope;

export type WorkflowCommandResponse = {
  receipt: Record<string, unknown>;
  event?: Record<string, unknown>;
  preview?: Record<string, unknown>;
};

export type WorkflowWorkspaceClient = {
  getRunnerStudioWorkflowGraph(workflowId: string): Promise<unknown>;
  executeRunnerStudioWorkflowCommand(payload: WorkflowCommandEnvelope): Promise<WorkflowCommandResponse>;
  moveRunnerStudioWorkflowNodes(payload: WorkflowSingleCommandEnvelope): Promise<WorkflowCommandResponse>;
  executeRunnerStudioWorkflowHistoryAction(payload: WorkflowHistoryActionEnvelope): Promise<unknown>;
  reviseRunnerStudioWorkflowCandidate(payload: WorkflowCandidateReviseEnvelope): Promise<unknown>;
  executeRunnerStudioWorkflowCandidateAction(payload: WorkflowCandidateDiscardEnvelope): Promise<unknown>;
  requestRunnerStudioWorkflowCandidateApplyApproval(payload: WorkflowCandidateApplyApprovalEnvelope): Promise<unknown>;
};

export type WorkflowHistoryAction = "undo" | "redo";

export type WorkflowHistoryActionEnvelope = {
  schemaVersion: typeof WORKFLOW_EDIT_SCHEMA_VERSION;
  workflowId: string;
  expected: WorkflowRevision;
  targetTransactionId: string;
  idempotencyKey: string;
  action: WorkflowHistoryAction;
};

export type WorkflowHistoryActionPending = {
  action: WorkflowHistoryAction;
  targetTransactionId: string;
  idempotencyKey: string;
};

export type WorkflowHistoryActionError = {
  action: WorkflowHistoryAction;
  targetTransactionId: string;
  code: string;
  message: string;
  retryHint?: string;
  occurredAt: string;
};

export type WorkflowCandidateReviseEnvelope = {
  schemaVersion: typeof WORKFLOW_EDIT_SCHEMA_VERSION;
  workflowId: string;
  candidateId: string;
  baseChangeSetId: string;
  expected: WorkflowRevision;
  idempotencyKey: string;
  intent: string;
  additionalOperation: WorkflowOperation;
};

export type WorkflowCandidateDiscardEnvelope = {
  schemaVersion: typeof WORKFLOW_EDIT_SCHEMA_VERSION;
  workflowId: string;
  candidateId: string;
  candidateRevision: number;
  changeSetId: string;
  previewHash: string;
  expected: WorkflowRevision;
  idempotencyKey: string;
  action: "discard";
};

export type WorkflowCandidateApplyApprovalEnvelope = {
  schemaVersion: typeof WORKFLOW_EDIT_SCHEMA_VERSION;
  workflowId: string;
  candidateId: string;
  candidateRevision: number;
  idempotencyKey: string;
  action: "request_apply_approval";
};

export type WorkflowCandidateApplyApprovalFact = {
  status: "requested";
  candidateId: string;
  candidateRevision: number;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
  sessionId: string;
  turnId: string;
  approvalId: string;
  expiresAt?: string;
  requestedAt: string;
};

export type WorkflowCandidateRevisionOptions = {
  intent: string;
  actionId?: string;
};

export type WorkflowCandidateActionPending = {
  action: "revise" | "discard" | "request_approval";
  candidateId: string;
  changeSetId: string;
  idempotencyKey: string;
  operationKind?: WorkflowOperation["op"];
};

export type WorkflowCandidateActionError = {
  action: "revise" | "discard" | "request_approval";
  candidateId: string;
  code: string;
  message: string;
  retryHint?: string;
  occurredAt: string;
};

export type WorkflowAuthoritativeProjection = {
  schemaVersion: "aiops.workflow.workspace.v1";
  workflowKey?: WorkflowKey;
  committed?: WorkflowCommittedSnapshotFact;
  events?: WorkflowDomainEventV2[];
  lastSequence?: number;
  gap?: { expectedSequence: number; receivedSequence: number };
  snapshotResyncRequired?: boolean;
  runGateFacts?: WorkflowRunGateFacts;
  receipts?: WorkflowReceiptFact[];
  validationArtifacts?: WorkflowValidationArtifactFact[];
  lifecycleArtifacts?: WorkflowLifecycleArtifactFact[];
  durableHistory?: WorkflowDurableHistoryEntry[];
};

export type WorkflowMutationOptions = {
  reason: string;
  actionId?: string;
  confirmed?: boolean;
};

export type WorkflowPendingCommand = {
  idempotencyKey: string;
  commandKind: WorkflowManualCommand["kind"];
  expectedRevision: string;
  revisionAfter?: string;
  transactionId?: string;
};

export type WorkflowUnsavedError = {
  code: string;
  message: string;
  retryHint?: string;
  commandKind: WorkflowManualCommand["kind"];
  occurredAt: string;
};

export type WorkflowWorkspaceCommands = {
  executeBatch(commands: readonly WorkflowManualCommand[], options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  addNode(node: JsonObject, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  updateNode(nodeId: string, nodePatch: JsonObject, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  deleteNode(nodeId: string, command: { cascadeEdges?: boolean }, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  connectEdge(edge: JsonObject, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  updateEdge(edgeId: string, edgePatch: JsonObject, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  disconnectEdge(edgeId: string, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
  moveNodes(layout: MoveLayout, options: WorkflowMutationOptions): Promise<WorkflowCommandResponse>;
};

export type WorkflowWorkspaceContextValue = {
  state: WorkflowWorkspaceState;
  commands: WorkflowWorkspaceCommands;
  pendingCommands: WorkflowPendingCommand[];
  unsavedError?: WorkflowUnsavedError;
  historyPending?: WorkflowHistoryActionPending;
  historyError?: WorkflowHistoryActionError;
  candidateActionPending?: WorkflowCandidateActionPending;
  candidateActionError?: WorkflowCandidateActionError;
  candidateApplyApproval?: WorkflowCandidateApplyApprovalFact;
  loadError?: string;
  reconcile(projection: WorkflowAuthoritativeProjection): void;
  reconcileTransportFact(fact: WorkflowTransportArtifactFact): void;
  refresh(): Promise<void>;
  clearUnsavedError(): void;
  undoHistory(targetTransactionId: string): Promise<WorkflowCandidateFact>;
  redoHistory(targetTransactionId: string): Promise<WorkflowCandidateFact>;
  clearHistoryError(): void;
  reviseCandidate(operation: WorkflowOperation, options: WorkflowCandidateRevisionOptions): Promise<WorkflowCandidateFact>;
  discardCandidate(): Promise<void>;
  requestCandidateApplyApproval?(): Promise<WorkflowCandidateApplyApprovalFact>;
  clearCandidateActionError(): void;
  setSelection(selection: WorkflowSelectionFact): void;
  setViewport(viewport: WorkflowViewportFact): void;
};

type WorkflowWorkspaceProviderProps = {
  workflowId: string;
  projection?: WorkflowAuthoritativeProjection;
  client?: WorkflowWorkspaceClient;
  createIdempotencyKey?: (kind: WorkflowManualCommand["kind"], actionId?: string) => string;
  createHistoryIdempotencyKey?: (action: WorkflowHistoryAction, targetTransactionId: string, expected: WorkflowRevision) => string;
  createCandidateRevisionIdempotencyKey?: (candidateId: string, changeSetId: string, expected: WorkflowRevision, actionId?: string) => string;
  createCandidateDiscardIdempotencyKey?: (candidateId: string, changeSetId: string, expected: WorkflowRevision) => string;
  createCandidateApplyApprovalIdempotencyKey?: (candidateId: string, candidateRevision: number, expected: WorkflowRevision) => string;
  now?: () => Date;
  children: ReactNode;
};

const WorkflowWorkspaceContext = createContext<WorkflowWorkspaceContextValue | null>(null);

export function WorkflowWorkspaceProvider(props: WorkflowWorkspaceProviderProps) {
  return <WorkflowWorkspaceOwner key={props.workflowId} {...props} />;
}

function WorkflowWorkspaceOwner({
  workflowId,
  projection,
  client = runnerStudioClient as WorkflowWorkspaceClient,
  createIdempotencyKey = defaultIdempotencyKey,
  createHistoryIdempotencyKey = defaultHistoryIdempotencyKey,
  createCandidateRevisionIdempotencyKey = defaultCandidateRevisionIdempotencyKey,
  createCandidateDiscardIdempotencyKey = defaultCandidateDiscardIdempotencyKey,
  createCandidateApplyApprovalIdempotencyKey = defaultCandidateApplyApprovalIdempotencyKey,
  now = systemNow,
  children,
}: WorkflowWorkspaceProviderProps) {
  const [state, dispatch] = useReducer(workflowWorkspaceReducer, undefined, createWorkflowWorkspaceState);
  const [pendingCommands, setPendingCommands] = useState<WorkflowPendingCommand[]>([]);
  const [unsavedError, setUnsavedError] = useState<WorkflowUnsavedError>();
  const [historyPending, setHistoryPending] = useState<WorkflowHistoryActionPending>();
  const [historyError, setHistoryError] = useState<WorkflowHistoryActionError>();
  const [candidateActionPending, setCandidateActionPending] = useState<WorkflowCandidateActionPending>();
  const [candidateActionError, setCandidateActionError] = useState<WorkflowCandidateActionError>();
  const [candidateApplyApproval, setCandidateApplyApproval] = useState<WorkflowCandidateApplyApprovalFact>();
  const [loadError, setLoadError] = useState<string>();
  const stateRef = useRef(state);
  const commandPromises = useRef(new Map<string, Promise<WorkflowCommandResponse>>());
  const historyPromises = useRef(new Map<string, Promise<WorkflowCandidateFact>>());
  const historyIdempotencyKeys = useRef(new Map<string, string>());
  const candidateRevisionPromises = useRef(new Map<string, Promise<WorkflowCandidateFact>>());
  const candidateRevisionIdempotencyKeys = useRef(new Map<string, string>());
  const candidateDiscardPromises = useRef(new Map<string, Promise<void>>());
  const candidateDiscardIdempotencyKeys = useRef(new Map<string, string>());
  const candidateApplyApprovalPromises = useRef(new Map<string, Promise<WorkflowCandidateApplyApprovalFact>>());
  const candidateApplyApprovalIdempotencyKeys = useRef(new Map<string, string>());
  const transportFactIdentities = useRef(new Set<string>());
  stateRef.current = state;

  const reconcile = useCallback((incoming: WorkflowAuthoritativeProjection) => {
    if (incoming?.schemaVersion !== "aiops.workflow.workspace.v1") return;
    const key = incoming.committed?.workflowKey || incoming.workflowKey;
    if (key && key.workflowId !== workflowId) return;
    for (const receipt of incoming.receipts || []) {
      if (receipt.workflowKey.workflowId === workflowId) {
        dispatch({
          type: "receipt.received",
          receipt: {
            ...receipt,
            validation: {
              ...receipt.validation,
              diagnostics: safeWorkflowDiagnostics(receipt.validation.diagnostics),
            },
          },
        });
      }
    }
    for (const event of [...(incoming.events || [])].sort((left, right) => left.sequence - right.sequence)) {
      if (event.workflowKey.workflowId === workflowId) dispatch({ type: "event.received", event });
    }
    if (incoming.committed?.workflowKey.workflowId === workflowId) {
      dispatch({ type: "snapshot.loaded", snapshot: incoming.committed });
    }
    for (const artifact of incoming.validationArtifacts || []) {
      dispatch({
        type: "validation.received",
        artifact: { ...artifact, diagnostics: safeWorkflowDiagnostics(artifact.diagnostics) },
      });
    }
    for (const artifact of incoming.lifecycleArtifacts || []) {
      dispatch({ type: "lifecycle.received", artifact });
    }
    dispatch({
      type: "connection.changed",
      connection: {
        state: incoming.gap || incoming.snapshotResyncRequired ? "reconnecting" : "online",
        changedAt: now().toISOString(),
      },
    });
  }, [now, workflowId]);

  useEffect(() => {
    if (projection) reconcile(projection);
  }, [projection, reconcile]);

  const reconcileTransportFact = useCallback((incoming: WorkflowTransportArtifactFact) => {
    const current = stateRef.current;
    const currentKey = current.committed?.workflowKey || current.workflowKey;
    const currentRevision = current.committed?.revision;
    if (!currentKey || !currentRevision || currentKey.workflowId !== workflowId) return;
    if (incoming.kind === "candidate") {
      const { candidate, changeSet } = incoming.fact;
      if (!sameWorkflowKey(currentKey, candidate.workflowKey)
        || !sameWorkflowKey(currentKey, changeSet.workflowKey)
        || !sameWorkflowRevision(currentRevision, changeSet.base)
        || candidate.id !== changeSet.candidateId
        || candidate.currentChangeSetId !== changeSet.id
        || candidate.currentRevision !== changeSet.candidateRevision
        || candidate.currentPreviewHash !== changeSet.previewHash) return;
      const identity = workflowTransportArtifactIdentity(incoming);
      if (transportFactIdentities.current.has(identity)) return;
      transportFactIdentities.current.add(identity);
      setCandidateApplyApproval((currentApproval) => currentApproval
        && currentApproval.candidateId === candidate.id
        && currentApproval.candidateRevision === candidate.currentRevision
        && currentApproval.changeSetId === changeSet.id
        && currentApproval.previewHash === changeSet.previewHash
        ? currentApproval
        : undefined);
      dispatch({ type: "candidate.received", candidate: incoming.fact });
      return;
    }
    if (incoming.kind === "receipt") {
      const { fact } = incoming;
      if (!sameWorkflowKey(currentKey, fact.workflowKey)
        || (!sameWorkflowRevision(currentRevision, fact.revisionBefore)
          && (!fact.revisionAfter || !sameWorkflowRevision(currentRevision, fact.revisionAfter)))) return;
      const identity = workflowTransportArtifactIdentity(incoming);
      if (transportFactIdentities.current.has(identity)) return;
      transportFactIdentities.current.add(identity);
      dispatch({ type: "receipt.received", receipt: fact });
      return;
    }
    if (incoming.fact.basis.resourceVersion !== currentRevision.resourceVersion
      || incoming.fact.basis.semanticHash !== currentRevision.semanticHash) return;
    const identity = workflowTransportArtifactIdentity(incoming);
    if (transportFactIdentities.current.has(identity)) return;
    transportFactIdentities.current.add(identity);
    dispatch({ type: "validation.received", artifact: incoming.fact });
  }, [workflowId]);

  const refresh = useCallback(async () => {
    try {
      await client.getRunnerStudioWorkflowGraph(workflowId);
      setLoadError(undefined);
    } catch (error) {
      setLoadError(errorMessage(error, "无法读取已保存的工作流"));
      dispatch({ type: "connection.changed", connection: { state: "offline", changedAt: now().toISOString() } });
      throw error;
    }
  }, [client, now, workflowId]);

  useEffect(() => {
    void refresh().catch(() => undefined);
  }, [refresh]);

  const committedRevision = state.committed?.revision.resourceVersion;
  useEffect(() => {
    if (!committedRevision) return;
    setPendingCommands((current) => current.filter((item) => item.revisionAfter !== committedRevision));
    commandPromises.current.clear();
  }, [committedRevision]);

  const activeCandidateIdentity = state.candidate ? [
    state.candidate.candidate.id,
    state.candidate.candidate.currentRevision,
    state.candidate.candidate.currentChangeSetId,
    state.candidate.candidate.currentPreviewHash,
  ].join("\0") : "";
  useEffect(() => {
    setCandidateApplyApproval((current) => current && [
      current.candidateId, current.candidateRevision, current.changeSetId, current.previewHash,
    ].join("\0") === activeCandidateIdentity ? current : undefined);
  }, [activeCandidateIdentity]);

  const submitCommands = useCallback((rawCommands: readonly WorkflowManualCommand[], options: WorkflowMutationOptions) => {
    const expectedRevision = stateRef.current.committed?.revision.resourceVersion;
    if (!expectedRevision) return Promise.reject(new Error("authoritative workflow revision is not available"));
    const reason = String(options.reason || "").trim();
    if (!reason) return Promise.reject(new TypeError("workflow command reason is required"));
    if (!Array.isArray(rawCommands) || rawCommands.length === 0) {
      return Promise.reject(new TypeError("workflow commands must be a non-empty array"));
    }
    const batch = rawCommands.map(normalizeManualCommand);
    const commandKind = batch[0].kind;

    const actionId = String(options.actionId || "").trim();
    const identity = commandBatchIdentity(expectedRevision, batch, actionId);
    const existing = commandPromises.current.get(identity);
    if (existing) return existing;

    const idempotencyKey = createIdempotencyKey(commandKind, actionId || undefined);
    const envelopeBase: WorkflowCommandEnvelopeBase = {
      schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
      workflowId,
      expectedRevision,
      idempotencyKey,
      reason,
      ...(options.confirmed ? { confirmed: true } : {}),
    };
    const envelope: WorkflowCommandEnvelope = batch.length === 1
      ? { ...envelopeBase, command: batch[0] }
      : { ...envelopeBase, commands: batch };
    const pending: WorkflowPendingCommand = { idempotencyKey, commandKind, expectedRevision };
    setPendingCommands((current) => current.some((item) => item.idempotencyKey === idempotencyKey) ? current : [...current, pending]);
    setUnsavedError(undefined);

    const request = Promise.resolve()
      .then(() => batch.length === 1 && batch[0].kind === "move"
        ? client.moveRunnerStudioWorkflowNodes(envelope as WorkflowSingleCommandEnvelope)
        : client.executeRunnerStudioWorkflowCommand(envelope))
      .then((response) => {
        const receipt = workflowReceipt(response?.receipt, now());
        dispatch({ type: "receipt.received", receipt });
        const event = workflowEvent(response?.event);
        if (event) dispatch({ type: "event.received", event });
        const alreadyReconciled = receipt.revisionAfter?.resourceVersion === stateRef.current.committed?.revision.resourceVersion;
        if (!receipt.revisionAfter || alreadyReconciled || ["no_effect", "conflict", "rejected"].includes(receipt.status)) {
          setPendingCommands((current) => current.filter((item) => item.idempotencyKey !== idempotencyKey));
          commandPromises.current.delete(identity);
        } else {
          setPendingCommands((current) => current.map((item) => item.idempotencyKey === idempotencyKey
            ? { ...item, revisionAfter: receipt.revisionAfter?.resourceVersion, transactionId: receipt.transactionId }
            : item));
        }
        return response;
      })
      .catch((error) => {
        setPendingCommands((current) => current.filter((item) => item.idempotencyKey !== idempotencyKey));
        commandPromises.current.delete(identity);
        setUnsavedError(workflowUnsavedError(error, commandKind, now()));
        throw error;
      });
    commandPromises.current.set(identity, request);
    return request;
  }, [client, createIdempotencyKey, now, workflowId]);

  const commands = useMemo<WorkflowWorkspaceCommands>(() => ({
    executeBatch: (batch, options) => submitCommands(batch, options),
    addNode: (node, options) => submitCommands([{ kind: "add_node", node }], options),
    updateNode: (nodeId, nodePatch, options) => submitCommands([{ kind: "update_node", nodeId, nodePatch }], options),
    deleteNode: (nodeId, command, options) => submitCommands([{ kind: "delete_node", nodeId, cascadeEdges: Boolean(command.cascadeEdges) }], options),
    connectEdge: (edge, options) => submitCommands([{ kind: "connect_edge", edge }], options),
    updateEdge: (edgeId, edgePatch, options) => submitCommands([{ kind: "update_edge", edgeId, edgePatch }], options),
    disconnectEdge: (edgeId, options) => submitCommands([{ kind: "disconnect_edge", edgeId }], options),
    moveNodes: (layout, options) => submitCommands([{ kind: "move", layout }], options),
  }), [submitCommands]);

  const submitHistoryAction = useCallback((action: WorkflowHistoryAction, rawTargetTransactionId: string) => {
    const current = stateRef.current;
    const key = current.committed?.workflowKey;
    const expected = current.committed?.revision;
    if (!key || !expected || key.workflowId !== workflowId) {
      return Promise.reject(new Error("authoritative workflow revision is not available"));
    }
    const targetTransactionId = String(rawTargetTransactionId || "").trim();
    if (!targetTransactionId || targetTransactionId !== rawTargetTransactionId) {
      return Promise.reject(new TypeError("workflow history targetTransactionId is required"));
    }
    const identity = historyActionIdentity(action, targetTransactionId, expected);
    const existing = historyPromises.current.get(identity);
    if (existing) return existing;
    const idempotencyKey = historyIdempotencyKeys.current.get(identity)
      || createHistoryIdempotencyKey(action, targetTransactionId, expected);
    if (!idempotencyKey || idempotencyKey !== idempotencyKey.trim()) {
      return Promise.reject(new TypeError("workflow history idempotencyKey is required"));
    }
    historyIdempotencyKeys.current.set(identity, idempotencyKey);
    const envelope: WorkflowHistoryActionEnvelope = {
      schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
      workflowId,
      expected: { ...expected },
      targetTransactionId,
      idempotencyKey,
      action,
    };
    setHistoryPending({ action, targetTransactionId, idempotencyKey });
    setHistoryError(undefined);

    const request = Promise.resolve()
      .then(() => client.executeRunnerStudioWorkflowHistoryAction(envelope))
      .then((response) => {
        const fact = parseWorkflowPreviewResult(response, {
          workflowKey: key,
          committedRevision: expected,
          receivedAt: now().toISOString(),
        });
        if (!fact) throw historyActionProtocolError("INVALID_WORKFLOW_PREVIEW", "服务端返回了无效的工作流候选预览");
        const latestRevision = stateRef.current.committed?.revision;
        if (!latestRevision || !sameWorkflowRevision(latestRevision, expected)) {
          throw historyActionProtocolError("STALE_REVISION", "工作流已发生变化，请刷新事务历史后重试");
        }
        reconcileTransportFact({
          kind: "candidate",
          artifactId: `history-${action}-${targetTransactionId}`,
          fact,
        });
        return fact;
      })
      .catch((error) => {
        setHistoryError(workflowHistoryActionError(error, action, targetTransactionId, now()));
        throw error;
      })
      .finally(() => {
        historyPromises.current.delete(identity);
        setHistoryPending((pending) => pending?.action === action && pending.targetTransactionId === targetTransactionId
          ? undefined
          : pending);
      });
    historyPromises.current.set(identity, request);
    return request;
  }, [client, createHistoryIdempotencyKey, now, reconcileTransportFact, workflowId]);

  const reviseCandidate = useCallback((rawOperation: WorkflowOperation, options: WorkflowCandidateRevisionOptions) => {
    let binding: WorkflowCandidateBinding;
    try {
      binding = currentCandidateBinding(stateRef.current, workflowId);
    } catch (error) {
      const candidateId = stateRef.current.candidate?.candidate.id || "unavailable";
      setCandidateActionError(workflowCandidateActionError(error, "revise", candidateId, now()));
      return Promise.reject(error);
    }
    const intent = String(options?.intent || "").trim();
    if (!intent || intent !== options?.intent) return Promise.reject(new TypeError("workflow candidate revise intent is required"));
    let operation: WorkflowOperation;
    try {
      operation = parseWorkflowPatchRequestV2({
        schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
        workflowKey: binding.workflowKey,
        expected: binding.expected,
        validationBasis: binding.fact.changeSet.validation.basis,
        intent,
        operations: [rawOperation],
      }).operations[0];
    } catch (error) {
      return Promise.reject(error);
    }
    const actionId = String(options.actionId || "").trim();
    if (options.actionId !== undefined && (!actionId || actionId !== options.actionId)) {
      return Promise.reject(new TypeError("workflow candidate revise actionId is invalid"));
    }
    const identity = candidateRevisionIdentity(binding, intent, operation, actionId);
    const existing = candidateRevisionPromises.current.get(identity);
    if (existing) return existing;
    const idempotencyKey = candidateRevisionIdempotencyKeys.current.get(identity)
      || createCandidateRevisionIdempotencyKey(binding.candidateId, binding.changeSetId, binding.expected, actionId || undefined);
    if (!idempotencyKey || idempotencyKey !== idempotencyKey.trim()) {
      return Promise.reject(new TypeError("workflow candidate revise idempotencyKey is required"));
    }
    candidateRevisionIdempotencyKeys.current.set(identity, idempotencyKey);
    const envelope: WorkflowCandidateReviseEnvelope = {
      schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
      workflowId,
      candidateId: binding.candidateId,
      baseChangeSetId: binding.changeSetId,
      expected: { ...binding.expected },
      idempotencyKey,
      intent,
      additionalOperation: operation,
    };
    setCandidateActionPending({
      action: "revise", candidateId: binding.candidateId, changeSetId: binding.changeSetId,
      idempotencyKey, operationKind: operation.op,
    });
    setCandidateActionError(undefined);

    const request = Promise.resolve()
      .then(() => client.reviseRunnerStudioWorkflowCandidate(envelope))
      .then((response) => {
        const fact = parseWorkflowPreviewResult(response, {
          workflowKey: binding.workflowKey,
          committedRevision: binding.expected,
          receivedAt: now().toISOString(),
        });
        if (!fact) throw candidateActionProtocolError("INVALID_WORKFLOW_PREVIEW", "服务端返回了无效的候选版本预览");
        const latest = stateRef.current;
        if (!latest.committed || !sameWorkflowRevision(latest.committed.revision, binding.expected)) {
          throw candidateActionProtocolError("STALE_REVISION", "工作流已发生变化，请刷新候选版本后重试");
        }
        if (sameCandidateFactIdentity(latest.candidate, fact)) return fact;
        if (!sameCandidateBinding(latest.candidate, binding)) {
          throw candidateActionProtocolError("STALE_CHANGESET", "候选版本已发生变化，请基于最新版本继续修改");
        }
        reconcileTransportFact({
          kind: "candidate",
          artifactId: `candidate-revise-${fact.candidate.id}-${fact.candidate.currentRevision}`,
          fact,
        });
        return fact;
      })
      .catch((error) => {
        setCandidateActionError(workflowCandidateActionError(error, "revise", binding.candidateId, now()));
        throw error;
      })
      .finally(() => {
        candidateRevisionPromises.current.delete(identity);
        setCandidateActionPending((pending) => pending?.action === "revise" && pending.idempotencyKey === idempotencyKey
          ? undefined
          : pending);
      });
    candidateRevisionPromises.current.set(identity, request);
    return request;
  }, [client, createCandidateRevisionIdempotencyKey, now, reconcileTransportFact, workflowId]);

  const requestCandidateApplyApproval = useCallback(() => {
    let binding: WorkflowCandidateBinding;
    try {
      binding = currentCandidateBinding(stateRef.current, workflowId);
      if (!isPersistedHistoryCandidate(binding.fact) && !isUserRevisedAgentCandidate(binding.fact)) {
        throw candidateActionProtocolError(
          "REVISED_CANDIDATE_REQUIRED",
          "只有已保存的撤销或重做候选，或由用户修改后的 AI 候选可以从这里请求确认",
        );
      }
    } catch (error) {
      const candidateId = stateRef.current.candidate?.candidate.id || "unavailable";
      setCandidateActionError(workflowCandidateActionError(error, "request_approval", candidateId, now()));
      return Promise.reject(error);
    }
    const identity = candidateApplyApprovalIdentity(binding);
    const existing = candidateApplyApprovalPromises.current.get(identity);
    if (existing) return existing;
    const idempotencyKey = candidateApplyApprovalIdempotencyKeys.current.get(identity)
      || createCandidateApplyApprovalIdempotencyKey(binding.candidateId, binding.candidateRevision, binding.expected);
    if (!idempotencyKey || idempotencyKey !== idempotencyKey.trim()) {
      return Promise.reject(new TypeError("workflow candidate apply approval idempotencyKey is required"));
    }
    candidateApplyApprovalIdempotencyKeys.current.set(identity, idempotencyKey);
    const envelope: WorkflowCandidateApplyApprovalEnvelope = {
      schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
      workflowId,
      candidateId: binding.candidateId,
      candidateRevision: binding.candidateRevision,
      idempotencyKey,
      action: "request_apply_approval",
    };
    setCandidateActionPending({
      action: "request_approval", candidateId: binding.candidateId, changeSetId: binding.changeSetId, idempotencyKey,
    });
    setCandidateActionError(undefined);

    const request = Promise.resolve()
      .then(() => client.requestRunnerStudioWorkflowCandidateApplyApproval(envelope))
      .then((response) => {
        const latest = stateRef.current;
        if (!latest.committed || !sameWorkflowRevision(latest.committed.revision, binding.expected)
          || !sameCandidateBinding(latest.candidate, binding)) {
          throw candidateActionProtocolError("STALE_CHANGESET", "候选版本已发生变化，未继续打开旧的确认步骤");
        }
        const approval = parseCandidateApplyApproval(response, binding, now());
        setCandidateApplyApproval(approval);
        const transport = workflowTransportIdentity(workflowId);
        emitWorkflowTransportResume({
          schemaVersion: WORKFLOW_TRANSPORT_RESUME_SCHEMA_VERSION,
          workflowId,
          threadId: transport.threadId,
          sessionId: approval.sessionId,
          turnId: approval.turnId,
          approvalId: approval.approvalId,
        });
        return approval;
      })
      .catch((error) => {
        setCandidateActionError(workflowCandidateActionError(error, "request_approval", binding.candidateId, now()));
        throw error;
      })
      .finally(() => {
        candidateApplyApprovalPromises.current.delete(identity);
        setCandidateActionPending((pending) => pending?.action === "request_approval" && pending.idempotencyKey === idempotencyKey
          ? undefined
          : pending);
      });
    candidateApplyApprovalPromises.current.set(identity, request);
    return request;
  }, [client, createCandidateApplyApprovalIdempotencyKey, now, workflowId]);

  const discardCandidate = useCallback(() => {
    let binding: WorkflowCandidateBinding;
    try {
      binding = currentCandidateBinding(stateRef.current, workflowId);
    } catch (error) {
      const candidateId = stateRef.current.candidate?.candidate.id || "unavailable";
      setCandidateActionError(workflowCandidateActionError(error, "discard", candidateId, now()));
      return Promise.reject(error);
    }
    const identity = candidateDiscardIdentity(binding);
    const existing = candidateDiscardPromises.current.get(identity);
    if (existing) return existing;
    const idempotencyKey = candidateDiscardIdempotencyKeys.current.get(identity)
      || createCandidateDiscardIdempotencyKey(binding.candidateId, binding.changeSetId, binding.expected);
    if (!idempotencyKey || idempotencyKey !== idempotencyKey.trim()) {
      return Promise.reject(new TypeError("workflow candidate discard idempotencyKey is required"));
    }
    candidateDiscardIdempotencyKeys.current.set(identity, idempotencyKey);
    const envelope: WorkflowCandidateDiscardEnvelope = {
      schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
      workflowId,
      candidateId: binding.candidateId,
      candidateRevision: binding.candidateRevision,
      changeSetId: binding.changeSetId,
      previewHash: binding.previewHash,
      expected: { ...binding.expected },
      idempotencyKey,
      action: "discard",
    };
    setCandidateActionPending({
      action: "discard", candidateId: binding.candidateId, changeSetId: binding.changeSetId, idempotencyKey,
    });
    setCandidateActionError(undefined);

    const request = Promise.resolve()
      .then(() => client.executeRunnerStudioWorkflowCandidateAction(envelope))
      .then((response) => {
        assertCandidateDiscardResult(response, binding);
        const latest = stateRef.current;
        if (!latest.committed || !sameWorkflowRevision(latest.committed.revision, binding.expected)) {
          throw candidateActionProtocolError("STALE_REVISION", "工作流已发生变化，未丢弃当前候选版本");
        }
        if (!latest.candidate) return;
        if (!sameCandidateBinding(latest.candidate, binding)) {
          throw candidateActionProtocolError("STALE_CHANGESET", "候选版本已发生变化，未丢弃较新的版本");
        }
        setCandidateApplyApproval(undefined);
        dispatch({ type: "candidate.cleared", candidateId: binding.candidateId });
      })
      .catch((error) => {
        setCandidateActionError(workflowCandidateActionError(error, "discard", binding.candidateId, now()));
        throw error;
      })
      .finally(() => {
        candidateDiscardPromises.current.delete(identity);
        setCandidateActionPending((pending) => pending?.action === "discard" && pending.idempotencyKey === idempotencyKey
          ? undefined
          : pending);
      });
    candidateDiscardPromises.current.set(identity, request);
    return request;
  }, [client, createCandidateDiscardIdempotencyKey, now, workflowId]);

  const visibleState = useMemo(() => candidateDisplayState(state), [state]);
  const value = useMemo<WorkflowWorkspaceContextValue>(() => ({
    state: visibleState,
    commands,
    pendingCommands,
    unsavedError,
    historyPending,
    historyError,
    candidateActionPending,
    candidateActionError,
    candidateApplyApproval,
    loadError,
    reconcile,
    reconcileTransportFact,
    refresh,
    clearUnsavedError: () => setUnsavedError(undefined),
    undoHistory: (targetTransactionId) => submitHistoryAction("undo", targetTransactionId),
    redoHistory: (targetTransactionId) => submitHistoryAction("redo", targetTransactionId),
    clearHistoryError: () => setHistoryError(undefined),
    reviseCandidate,
    discardCandidate,
    requestCandidateApplyApproval,
    clearCandidateActionError: () => setCandidateActionError(undefined),
    setSelection: (selection) => dispatch({ type: "selection.changed", selection }),
    setViewport: (viewport) => dispatch({ type: "viewport.changed", viewport }),
  }), [candidateActionError, candidateActionPending, candidateApplyApproval, commands, discardCandidate, historyError, historyPending, loadError, pendingCommands, reconcile, reconcileTransportFact, refresh, requestCandidateApplyApproval, reviseCandidate, submitHistoryAction, unsavedError, visibleState]);

  return <WorkflowWorkspaceContext.Provider value={value}>{children}</WorkflowWorkspaceContext.Provider>;
}

export function useWorkflowWorkspace(): WorkflowWorkspaceContextValue {
  const value = useContext(WorkflowWorkspaceContext);
  if (!value) throw new Error("useWorkflowWorkspace must be used inside WorkflowWorkspaceProvider");
  return value;
}

function normalizeMoveLayout(layout: MoveLayout): MoveLayout {
  return {
    nodes: [...layout.nodes]
      .map((item) => ({ nodeId: item.nodeId, position: { x: item.position.x, y: item.position.y } }))
      .sort((left, right) => left.nodeId.localeCompare(right.nodeId)),
    ...(layout.viewport ? { viewport: { ...layout.viewport } } : {}),
  };
}

function normalizeManualCommand(command: WorkflowManualCommand): WorkflowManualCommand {
  return command.kind === "move"
    ? { ...command, layout: normalizeMoveLayout(command.layout) }
    : command;
}

function commandBatchIdentity(expectedRevision: string, commands: WorkflowManualCommand[], actionId: string): string {
  const kind = commands.map((command) => command.kind).join("+");
  if (actionId) return `${expectedRevision}:${kind}:${actionId}`;
  if (commands.length === 1 && commands[0].kind === "move") {
    return `${expectedRevision}:move:${JSON.stringify(commands[0].layout)}`;
  }
  return `${expectedRevision}:${kind}:${defaultIdempotencyKey(commands[0].kind)}`;
}

function defaultIdempotencyKey(kind: WorkflowManualCommand["kind"], actionId?: string): string {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `canvas-${kind}-${actionId || random}`;
}

function defaultHistoryIdempotencyKey(action: WorkflowHistoryAction, targetTransactionId: string): string {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `canvas-history-${action}-${targetTransactionId}-${random}`;
}

function defaultCandidateRevisionIdempotencyKey(_candidateId: string, _changeSetId: string, _expected: WorkflowRevision, actionId?: string): string {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `canvas-candidate-revise-${actionId || random}`;
}

function defaultCandidateDiscardIdempotencyKey(): string {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `canvas-candidate-discard-${random}`;
}

function defaultCandidateApplyApprovalIdempotencyKey(): string {
  const random = globalThis.crypto?.randomUUID?.() || `${Date.now()}-${Math.random().toString(16).slice(2)}`;
  return `canvas-candidate-apply-approval-${random}`;
}

function historyActionIdentity(action: WorkflowHistoryAction, targetTransactionId: string, expected: WorkflowRevision): string {
  return [action, targetTransactionId, expected.resourceVersion, expected.semanticHash, expected.layoutHash].join("\0");
}

function historyActionProtocolError(code: string, message: string): Error {
  return Object.assign(new Error(message), { code });
}

function currentCandidateBinding(state: WorkflowWorkspaceState, workflowId: string): WorkflowCandidateBinding {
  const committed = state.committed;
  const fact = state.candidate;
  if (!committed || committed.workflowKey.workflowId !== workflowId || !fact) {
    throw candidateActionProtocolError("CANDIDATE_NOT_AVAILABLE", "当前没有可修改的候选版本");
  }
  const { candidate, changeSet } = fact;
  // validation.basis.semanticHash describes the projected Candidate graph.
  // Freshness is anchored by the immutable ChangeSet base plus the basis
  // resourceVersion; comparing that projected hash with committed truth would
  // reject valid Undo/Redo Candidates before their discard/revise request.
  if (!sameWorkflowKey(committed.workflowKey, candidate.workflowKey)
    || !sameWorkflowKey(committed.workflowKey, changeSet.workflowKey)
    || (!sameWorkflowRevision(committed.revision, changeSet.base) && !isLayoutOnlyCandidateDrift(committed.revision, changeSet.base))
    || candidate.id !== changeSet.candidateId
    || candidate.currentRevision !== changeSet.candidateRevision
    || candidate.currentChangeSetId !== changeSet.id
    || candidate.currentPreviewHash !== changeSet.previewHash
    || changeSet.validation.basis.resourceVersion !== changeSet.base.resourceVersion) {
    throw candidateActionProtocolError("STALE_CHANGESET", "候选版本与当前工作流不匹配，请刷新后重试");
  }
  return {
    workflowKey: { ...committed.workflowKey },
    expected: { ...committed.revision },
    candidateBase: { ...changeSet.base },
    candidateId: candidate.id,
    candidateRevision: candidate.currentRevision,
    changeSetId: candidate.currentChangeSetId,
    previewHash: candidate.currentPreviewHash,
    fact,
  };
}

function candidateRevisionIdentity(binding: WorkflowCandidateBinding, intent: string, operation: WorkflowOperation, actionId: string): string {
  return [
    "revise", binding.candidateId, binding.candidateRevision, binding.changeSetId, binding.previewHash,
    binding.expected.resourceVersion, binding.expected.semanticHash, binding.expected.layoutHash,
    intent, actionId, JSON.stringify(operation),
  ].join("\0");
}

function candidateDiscardIdentity(binding: WorkflowCandidateBinding): string {
  return [
    "discard", binding.candidateId, binding.candidateRevision, binding.changeSetId, binding.previewHash,
    binding.expected.resourceVersion, binding.expected.semanticHash, binding.expected.layoutHash,
  ].join("\0");
}

function candidateApplyApprovalIdentity(binding: WorkflowCandidateBinding): string {
  return [
    "request_apply_approval", binding.candidateId, binding.candidateRevision, binding.changeSetId, binding.previewHash,
    binding.expected.resourceVersion, binding.expected.semanticHash, binding.expected.layoutHash,
  ].join("\0");
}

function isPersistedHistoryCandidate(fact: WorkflowCandidateFact): boolean {
  return fact.changeSet.actor === "user"
    && fact.candidate.currentRevision === 1
    && fact.changeSet.candidateRevision === 1
    && /^history-[a-f0-9]{64}$/.test(fact.candidate.id);
}

function isUserRevisedAgentCandidate(fact: WorkflowCandidateFact): boolean {
  const revision = fact.candidate.currentRevision;
  return fact.changeSet.actor === "user"
    && Number.isSafeInteger(revision)
    && revision > 1
    && fact.changeSet.candidateRevision === revision
    && typeof fact.changeSet.parentChangeSetId === "string"
    && fact.changeSet.parentChangeSetId.length > 0
    && fact.changeSet.parentChangeSetId === fact.changeSet.parentChangeSetId.trim()
    && !/^history-[a-f0-9]{64}$/.test(fact.candidate.id);
}

function sameCandidateBinding(fact: WorkflowCandidateFact | undefined, binding: WorkflowCandidateBinding): boolean {
  return Boolean(fact
    && sameWorkflowKey(fact.candidate.workflowKey, binding.workflowKey)
    && fact.candidate.id === binding.candidateId
    && fact.candidate.currentRevision === binding.candidateRevision
    && fact.candidate.currentChangeSetId === binding.changeSetId
    && fact.candidate.currentPreviewHash === binding.previewHash
    && fact.changeSet.id === binding.changeSetId
    && sameWorkflowRevision(fact.changeSet.base, binding.candidateBase));
}

/**
 * The server is the final arbiter for path-disjoint safe rebase. The browser
 * only keeps a Candidate editable across the narrow drift it can prove from
 * authoritative revision facts: semantic truth is unchanged and layout truth
 * changed. This does not mutate the persisted Candidate binding used by
 * commands; it only projects the Ghost Candidate over the latest layout so a
 * user can request a server-validated revision.
 */
function candidateDisplayState(state: WorkflowWorkspaceState): WorkflowWorkspaceState {
  const committed = state.committed;
  const candidate = state.candidate;
  if (!committed || !candidate || !isLayoutOnlyCandidateDrift(committed.revision, candidate.changeSet.base)) return state;
  return {
    ...state,
    candidate: {
      ...candidate,
      changeSet: {
        ...candidate.changeSet,
        base: { ...committed.revision },
      },
    },
  };
}

function isLayoutOnlyCandidateDrift(current: WorkflowRevision, base: WorkflowRevision): boolean {
  return current.resourceVersion !== base.resourceVersion
    && current.semanticHash === base.semanticHash
    && current.layoutHash !== base.layoutHash;
}

function sameCandidateFactIdentity(left: WorkflowCandidateFact | undefined, right: WorkflowCandidateFact): boolean {
  return Boolean(left
    && sameWorkflowKey(left.candidate.workflowKey, right.candidate.workflowKey)
    && left.candidate.id === right.candidate.id
    && left.candidate.currentRevision === right.candidate.currentRevision
    && left.candidate.currentChangeSetId === right.candidate.currentChangeSetId
    && left.candidate.currentPreviewHash === right.candidate.currentPreviewHash);
}

function assertCandidateDiscardResult(value: unknown, binding: WorkflowCandidateBinding): void {
  const source = record(value);
  const keys = Object.keys(source).sort();
  if (keys.join("\0") !== ["kind", "schemaVersion", "candidate"].sort().join("\0")
    || source.kind !== "candidate_discarded"
    || source.schemaVersion !== WORKFLOW_EDIT_SCHEMA_VERSION) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_ACTION_RESULT", "服务端返回了无效的候选版本丢弃结果");
  }
  const candidate = record(source.candidate);
  if (Object.keys(candidate).sort().join("\0") !== [
    "id", "workflowKey", "currentChangeSetId", "currentRevision", "currentPreviewHash", "state", "expiresAt",
  ].sort().join("\0")) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_ACTION_RESULT", "服务端返回了无效的候选版本丢弃结果");
  }
  const key = workflowKey(candidate.workflowKey);
  if (!sameWorkflowKey(key, binding.workflowKey)
    || candidate.id !== binding.candidateId
    || candidate.currentRevision !== binding.candidateRevision
    || candidate.currentChangeSetId !== binding.changeSetId
    || candidate.currentPreviewHash !== binding.previewHash
    || candidate.state !== "abandoned"
    || typeof candidate.expiresAt !== "string"
    || !candidate.expiresAt
    || Number.isNaN(Date.parse(candidate.expiresAt))) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_ACTION_RESULT", "服务端返回的候选版本丢弃绑定不匹配");
  }
}

function candidateActionProtocolError(code: string, message: string): Error {
  return Object.assign(new Error(message), { code });
}

function parseCandidateApplyApproval(
  value: unknown,
  binding: WorkflowCandidateBinding,
  requestedAt: Date,
): WorkflowCandidateApplyApprovalFact {
  const source = exactRecord(value, ["kind", "schemaVersion", "status", "sessionId", "turnId", "candidate", "approval"], "候选确认响应");
  if (source.kind !== "workflow_apply_approval" || source.schemaVersion !== WORKFLOW_EDIT_SCHEMA_VERSION || source.status !== "blocked") {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", "服务端没有返回待确认的工作流应用步骤");
  }
  const sessionId = strictResponseIdentifier(source.sessionId, "候选确认 sessionId");
  const turnId = strictResponseIdentifier(source.turnId, "候选确认 turnId");
  const candidate = exactRecord(
    source.candidate,
    ["candidateId", "candidateRevision", "changeSetId", "previewHash", "expectedRevision"],
    "候选确认绑定",
  );
  if (candidate.candidateId !== binding.candidateId
    || candidate.candidateRevision !== binding.candidateRevision
    || candidate.changeSetId !== binding.changeSetId
    || candidate.previewHash !== binding.previewHash
    || candidate.expectedRevision !== binding.expected.resourceVersion) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", "服务端返回的确认步骤与当前候选版本不匹配");
  }
  const approval = allowedRecord(
    source.approval,
    ["approvalId", "sessionId", "turnId", "toolCallId", "toolName", "argumentsHash", "targetRefs", "expiresAt"],
    ["approvalId", "sessionId", "turnId", "toolCallId", "toolName", "argumentsHash"],
    "候选确认详情",
  );
  const approvalId = strictResponseIdentifier(approval.approvalId, "候选确认 approvalId");
  if (approval.sessionId !== sessionId
    || approval.turnId !== turnId
    || approval.toolName !== "workflow.apply_patch"
    || !strictResponseIdentifier(approval.toolCallId, "候选确认 toolCallId")
    || typeof approval.argumentsHash !== "string"
    || !/^sha256:[a-f0-9]{64}$/.test(approval.argumentsHash)) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", "服务端返回了无效的工作流确认绑定");
  }
  if (approval.targetRefs !== undefined && (!Array.isArray(approval.targetRefs)
    || !approval.targetRefs.every((item) => typeof item === "string" && item.length > 0 && item === item.trim()))) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", "服务端返回了无效的确认目标");
  }
  if (approval.expiresAt !== undefined && (typeof approval.expiresAt !== "string" || Number.isNaN(Date.parse(approval.expiresAt)))) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", "服务端返回了无效的确认有效期");
  }
  return {
    status: "requested",
    candidateId: binding.candidateId,
    candidateRevision: binding.candidateRevision,
    changeSetId: binding.changeSetId,
    previewHash: binding.previewHash,
    expectedRevision: binding.expected.resourceVersion,
    sessionId,
    turnId,
    approvalId,
    ...(typeof approval.expiresAt === "string" ? { expiresAt: approval.expiresAt } : {}),
    requestedAt: requestedAt.toISOString(),
  };
}

function exactRecord(value: unknown, fields: string[], label: string): Record<string, unknown> {
  const source = record(value);
  if (Object.keys(source).sort().join("\0") !== [...fields].sort().join("\0")) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", `${label}字段不完整或包含未知内容`);
  }
  return source;
}

function allowedRecord(value: unknown, allowed: string[], required: string[], label: string): Record<string, unknown> {
  const source = record(value);
  if (Object.keys(source).some((field) => !allowed.includes(field))
    || required.some((field) => !Object.prototype.hasOwnProperty.call(source, field))) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", `${label}字段不完整或包含未知内容`);
  }
  return source;
}

function strictResponseIdentifier(value: unknown, label: string): string {
  if (typeof value !== "string" || !value || value !== value.trim() || /[\0\r\n]/.test(value)) {
    throw candidateActionProtocolError("INVALID_CANDIDATE_APPROVAL", `${label}无效`);
  }
  return value;
}

function workflowReceipt(raw: unknown, receivedAt: Date): WorkflowReceiptFact {
  const source = record(raw);
  const transactionId = requiredString(source.transactionId, "workflow receipt transactionId");
  const status = receiptStatus(source.status);
  return {
    transactionId,
    changeSetId: requiredString(source.changeSetId, "workflow receipt changeSetId"),
    workflowKey: workflowKey(source.workflowKey),
    status,
    revisionBefore: workflowRevision(source.revisionBefore),
    ...(source.revisionAfter ? { revisionAfter: workflowRevision(source.revisionAfter) } : {}),
    validation: workflowValidation(source.validation),
    receivedAt: receivedAt.toISOString(),
  };
}

function workflowEvent(raw: unknown): WorkflowDomainEventV2 | undefined {
  if (!raw) return undefined;
  const source = record(raw);
  return {
    schemaVersion: WORKFLOW_EDIT_SCHEMA_VERSION,
    eventId: requiredString(source.eventId, "workflow event eventId"),
    sequence: finiteNumber(source.sequence, "workflow event sequence"),
    type: requiredString(source.type, "workflow event type"),
    workflowKey: workflowKey(source.workflowKey),
    revision: workflowRevision(source.revision),
    actor: workflowActor(source.actor),
    ...(source.transactionId ? { transactionId: String(source.transactionId) } : {}),
    ...(source.changeSetId ? { changeSetId: String(source.changeSetId) } : {}),
    ...(source.candidateRevision ? { candidateRevision: finiteNumber(source.candidateRevision, "candidate revision") } : {}),
    payload: jsonValue(source.payload ?? {}),
    occurredAt: requiredString(source.occurredAt, "workflow event occurredAt"),
  };
}

function workflowValidation(raw: unknown): WorkflowValidationResult {
  const source = record(raw);
  const basis = record(source.basis);
  return {
    basis: {
      resourceVersion: requiredString(basis.resourceVersion, "validation resourceVersion"),
      semanticHash: requiredString(basis.semanticHash, "validation semanticHash"),
      catalogVersion: requiredString(basis.catalogVersion, "validation catalogVersion"),
      actionSchemaDigest: requiredString(basis.actionSchemaDigest, "validation actionSchemaDigest"),
      referenceSnapshotHash: requiredString(basis.referenceSnapshotHash, "validation referenceSnapshotHash"),
      environmentBindingVersion: requiredString(basis.environmentBindingVersion, "validation environmentBindingVersion"),
    },
    valid: source.valid === true,
    diagnostics: safeWorkflowDiagnostics(source.diagnostics),
  };
}

function safeWorkflowDiagnostics(raw: unknown): WorkflowDiagnostic[] {
  if (!Array.isArray(raw)) return [];
  return raw.filter((item): item is WorkflowDiagnostic => {
    if (!item || typeof item !== "object" || Array.isArray(item)) return false;
    const diagnostic = item as Record<string, unknown>;
    return typeof diagnostic.code === "string"
      && typeof diagnostic.severity === "string"
      && typeof diagnostic.message === "string";
  });
}

function workflowRevision(raw: unknown): WorkflowRevision {
  const source = record(raw);
  return {
    resourceVersion: requiredString(source.resourceVersion, "workflow resourceVersion"),
    semanticHash: requiredString(source.semanticHash, "workflow semanticHash"),
    layoutHash: requiredString(source.layoutHash, "workflow layoutHash"),
  };
}

function workflowKey(raw: unknown): WorkflowKey {
  const source = record(raw);
  return {
    tenantId: requiredString(source.tenantId, "workflow tenantId"),
    workflowId: requiredString(source.workflowId, "workflow workflowId"),
  };
}

function receiptStatus(raw: unknown): WorkflowReceiptFact["status"] {
  const value = String(raw || "");
  if (["verifying", "completed", "completed_with_warnings", "configuration_required", "saved_with_issues", "no_effect", "conflict", "rejected"].includes(value)) {
    return value as WorkflowReceiptFact["status"];
  }
  if (value === "applied") return "completed";
  throw new TypeError("unsupported workflow receipt status");
}

function workflowActor(raw: unknown): WorkflowDomainEventV2["actor"] {
  if (raw === "user" || raw === "agent" || raw === "system") return raw;
  throw new TypeError("unsupported workflow event actor");
}

function workflowUnsavedError(error: unknown, commandKind: WorkflowManualCommand["kind"], occurredAt: Date): WorkflowUnsavedError {
  const source = error && typeof error === "object" ? error as Record<string, unknown> : {};
  const payload = source.payload && typeof source.payload === "object" ? source.payload as Record<string, unknown> : {};
  return {
    code: String(payload.errorCode || source.code || "WORKFLOW_COMMAND_FAILED"),
    message: String(payload.message || source.message || "工作流修改未保存"),
    ...(payload.retryHint ? { retryHint: String(payload.retryHint) } : {}),
    commandKind,
    occurredAt: occurredAt.toISOString(),
  };
}

function workflowHistoryActionError(
  error: unknown,
  action: WorkflowHistoryAction,
  targetTransactionId: string,
  occurredAt: Date,
): WorkflowHistoryActionError {
  const source = error && typeof error === "object" ? error as Record<string, unknown> : {};
  const payload = source.payload && typeof source.payload === "object" ? source.payload as Record<string, unknown> : {};
  return {
    action,
    targetTransactionId,
    code: String(payload.errorCode || source.code || "WORKFLOW_HISTORY_ACTION_FAILED"),
    message: String(payload.message || source.message || "无法生成事务恢复候选"),
    ...(payload.retryHint ? { retryHint: String(payload.retryHint) } : {}),
    occurredAt: occurredAt.toISOString(),
  };
}

function workflowCandidateActionError(
  error: unknown,
  action: WorkflowCandidateActionError["action"],
  candidateId: string,
  occurredAt: Date,
): WorkflowCandidateActionError {
  const source = error && typeof error === "object" ? error as Record<string, unknown> : {};
  const payload = source.payload && typeof source.payload === "object" ? source.payload as Record<string, unknown> : {};
  return {
    action,
    candidateId,
    code: String(payload.errorCode || source.code || "WORKFLOW_CANDIDATE_ACTION_FAILED"),
    message: String(payload.message || source.message || (action === "discard"
      ? "无法丢弃候选版本"
      : action === "request_approval" ? "无法请求候选版本应用确认" : "无法更新候选版本")),
    ...(payload.retryHint ? { retryHint: String(payload.retryHint) } : {}),
    occurredAt: occurredAt.toISOString(),
  };
}

function errorMessage(error: unknown, fallback: string): string {
  return error instanceof Error && error.message ? error.message : fallback;
}

function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError("workflow response must be an object");
  return value as Record<string, unknown>;
}

function requiredString(value: unknown, field: string): string {
  const normalized = String(value || "").trim();
  if (!normalized) throw new TypeError(`${field} is required`);
  return normalized;
}

function finiteNumber(value: unknown, field: string): number {
  const normalized = Number(value);
  if (!Number.isFinite(normalized)) throw new TypeError(`${field} must be finite`);
  return normalized;
}

function jsonValue(value: unknown): WorkflowJsonValue {
  return structuredClone(value) as WorkflowJsonValue;
}

function sameWorkflowKey(left: WorkflowKey, right: WorkflowKey): boolean {
  return left.tenantId === right.tenantId && left.workflowId === right.workflowId;
}

function sameWorkflowRevision(left: WorkflowRevision, right: WorkflowRevision): boolean {
  return left.resourceVersion === right.resourceVersion
    && left.semanticHash === right.semanticHash
    && left.layoutHash === right.layoutHash;
}
