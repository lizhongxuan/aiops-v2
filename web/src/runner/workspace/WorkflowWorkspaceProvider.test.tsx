import { act, useState } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  WorkflowWorkspaceProvider,
  useWorkflowWorkspace,
  type WorkflowAuthoritativeProjection,
  type WorkflowWorkspaceClient,
  type WorkflowWorkspaceContextValue,
} from "./WorkflowWorkspaceProvider";
import type { WorkflowTransportArtifactFact } from "./workflowCandidateArtifact";
import {
  WORKFLOW_TRANSPORT_RESUME_EVENT,
  type WorkflowTransportResumeEventDetail,
} from "@/runner/transport/WorkflowTransportBoundary";
import { WORKFLOW_CHANGE_ACTION_EVENT } from "@/components/chat/WorkflowChangeArtifact";

const workflowKey = { tenantId: "tenant-a", workflowId: "workflow-a" };
const persistedHistoryCandidateId = `history-${"a".repeat(64)}`;
const revision = (resourceVersion: string) => ({
  resourceVersion,
  semanticHash: `semantic-${resourceVersion}`,
  layoutHash: `layout-${resourceVersion}`,
});
const validation = (resourceVersion: string) => ({
  basis: {
    resourceVersion,
    semanticHash: `semantic-${resourceVersion}`,
    catalogVersion: "catalog-1",
    actionSchemaDigest: "actions-1",
    referenceSnapshotHash: "refs-1",
    environmentBindingVersion: "env-1",
  },
  valid: true,
  diagnostics: [],
});

function projection(resourceVersion = "rv-1", sequence = 1): WorkflowAuthoritativeProjection {
  return {
    schemaVersion: "aiops.workflow.workspace.v1",
    workflowKey,
    committed: {
      workflowKey,
      revision: revision(resourceVersion),
      graph: { nodes: [{ id: resourceVersion, type: "action" }], edges: [] },
      sequence,
      loadedAt: "2026-07-21T00:00:00Z",
    },
    events: [],
    lastSequence: sequence,
    snapshotResyncRequired: false,
  };
}

function commandResult(transactionId = "txn-1", eventId = "event-2") {
  return {
    receipt: {
      schemaVersion: "aiops.workflow.edit.v2" as const,
      kind: "receipt",
      transactionId,
      changeSetId: `change-${transactionId}`,
      workflowKey,
      status: "verifying",
      revisionBefore: revision("rv-1"),
      revisionAfter: revision("rv-2"),
      delta: {
        addedNodeIds: [], updatedNodeIds: ["check"], deletedNodeIds: [],
        addedEdgeIds: [], updatedEdgeIds: [], deletedEdgeIds: [],
      },
      validation: validation("rv-2"),
      auditId: `audit-${transactionId}`,
    },
    event: {
      schemaVersion: "aiops.workflow.edit.v2" as const,
      eventId,
      sequence: 2,
      type: "transaction.applied",
      workflowKey,
      revision: revision("rv-2"),
      actor: "user" as const,
      transactionId,
      changeSetId: `change-${transactionId}`,
      payload: {},
      occurredAt: "2026-07-21T00:00:01Z",
    },
  };
}

function noEffectResult(transactionId: string) {
  return {
    receipt: {
      ...commandResult(transactionId).receipt,
      status: "no_effect",
      revisionAfter: undefined,
    },
  };
}

function historyPreviewResult(overrides: {
  candidateId?: string;
  changeSetId?: string;
  previewHash?: string;
  candidateRevision?: number;
  parentChangeSetId?: string;
  intent?: string;
  operations?: Array<Record<string, unknown>>;
  base?: ReturnType<typeof revision>;
  rebased?: boolean;
} = {}) {
  const candidateId = overrides.candidateId || "history-candidate-1";
  const changeSetId = overrides.changeSetId || "history-change-1";
  const previewHash = overrides.previewHash || "history-preview-1";
  const candidateRevision = overrides.candidateRevision || 1;
  const base = overrides.base || revision("rv-1");
  const changeSetValidation = validation(base.resourceVersion);
  return {
    kind: "preview",
    schemaVersion: "aiops.workflow.edit.v2",
    changeSet: {
      schemaVersion: "aiops.workflow.edit.v2",
      id: changeSetId,
      workflowKey,
      candidateId,
      candidateRevision,
      ...(overrides.parentChangeSetId ? { parentChangeSetId: overrides.parentChangeSetId } : {}),
      actor: "user",
      intent: overrides.intent || "undo workflow transaction txn-17",
      base,
      operations: overrides.operations || [{ op: "update_node", nodeId: "check", set: { label: "以前的名称" } }],
      previewHash,
      validation: changeSetValidation,
      state: "ready",
      createdAt: "2026-07-21T00:00:00Z",
      expiresAt: "2026-07-21T00:30:00Z",
    },
    candidate: {
      id: candidateId,
      workflowKey,
      currentChangeSetId: changeSetId,
      currentRevision: candidateRevision,
      currentPreviewHash: previewHash,
      state: "ready",
      expiresAt: "2026-07-21T00:30:00Z",
    },
    candidateRevision,
    previewHash,
    validationBasis: changeSetValidation.basis,
    diagnostics: [],
    risk: {
      level: "low", operationCount: 1, addedNodeCount: 0, deletedNodeCount: 0,
      deletesExisting: false, requiresApproval: false,
    },
    diff: {
      graph: { addedNodeIds: [], updatedNodeIds: ["check"], deletedNodeIds: [], addedEdgeIds: [], updatedEdgeIds: [], deletedEdgeIds: [] },
      inverse: { operations: [{ op: "update_node", nodeId: "check", set: { label: "当前名称" } }] },
      rebaseBase: {},
      rebaseInvariantHash: "rebase-history-1",
    },
    rebased: overrides.rebased ?? false,
  };
}

function candidateTransportFact(): WorkflowTransportArtifactFact {
  const result = historyPreviewResult({ candidateId: "candidate-a", changeSetId: "change-1", previewHash: "preview-1" });
  return {
    kind: "candidate",
    artifactId: "candidate-artifact-1",
    fact: {
      candidate: { ...result.candidate },
      changeSet: { ...result.changeSet, actor: "agent", state: "previewed" },
    },
  } as WorkflowTransportArtifactFact;
}

function candidateDiscardResult() {
  return {
    kind: "candidate_discarded",
    schemaVersion: "aiops.workflow.edit.v2",
    candidate: {
      id: "candidate-a",
      workflowKey,
      currentChangeSetId: "change-1",
      currentRevision: 1,
      currentPreviewHash: "preview-1",
      state: "abandoned",
      expiresAt: "2026-07-21T00:30:00Z",
    },
  };
}

function candidateApplyApprovalResult(
  candidateId = persistedHistoryCandidateId,
  candidateRevision = 1,
  changeSetId = "history-change-1",
  previewHash = "history-preview-1",
) {
  return {
    kind: "workflow_apply_approval",
    schemaVersion: "aiops.workflow.edit.v2",
    status: "blocked",
    sessionId: "session-workflow-a",
    turnId: "turn-history-apply-1",
    candidate: {
      candidateId,
      candidateRevision,
      changeSetId,
      previewHash,
      expectedRevision: "rv-1",
    },
    approval: {
      approvalId: "approval-history-apply-1",
      sessionId: "session-workflow-a",
      turnId: "turn-history-apply-1",
      toolCallId: "tool-history-apply-1",
      toolName: "workflow.apply_patch",
      argumentsHash: `sha256:${"b".repeat(64)}`,
      expiresAt: "2030-01-01T00:00:00Z",
    },
  };
}

function createClient(overrides: Partial<WorkflowWorkspaceClient> = {}): WorkflowWorkspaceClient {
  return {
    getRunnerStudioWorkflowGraph: vi.fn().mockResolvedValue({}),
    executeRunnerStudioWorkflowCommand: vi.fn().mockImplementation((payload) =>
      Promise.resolve(noEffectResult(payload.idempotencyKey))),
    moveRunnerStudioWorkflowNodes: vi.fn().mockImplementation((payload) =>
      Promise.resolve(noEffectResult(payload.idempotencyKey))),
    executeRunnerStudioWorkflowHistoryAction: vi.fn().mockResolvedValue(historyPreviewResult()),
    reviseRunnerStudioWorkflowCandidate: vi.fn().mockResolvedValue(historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1",
    })),
    executeRunnerStudioWorkflowCandidateAction: vi.fn().mockResolvedValue(candidateDiscardResult()),
    requestRunnerStudioWorkflowCandidateApplyApproval: vi.fn().mockImplementation((payload) => Promise.resolve(
      payload.candidateRevision === 1
        ? candidateApplyApprovalResult(payload.candidateId)
        : candidateApplyApprovalResult(payload.candidateId, payload.candidateRevision, "change-2", "preview-2"),
    )),
    ...overrides,
  };
}

describe("WorkflowWorkspaceProvider", () => {
  let container: HTMLDivElement;
  let root: Root;
  let latest: WorkflowWorkspaceContextValue;

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    container = document.createElement("div");
    document.body.appendChild(container);
    root = createRoot(container);
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    vi.restoreAllMocks();
  });

  function Consumer() {
    latest = useWorkflowWorkspace();
    return <output data-testid="revision">{latest.state.committed?.revision.resourceVersion || "none"}</output>;
  }

  async function render(client: WorkflowWorkspaceClient, authoritative = projection()) {
    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider
          workflowId="workflow-a"
          projection={authoritative}
          client={client}
          createIdempotencyKey={(kind, actionId) => `${kind}-${actionId || "generated"}`}
          createHistoryIdempotencyKey={(action, target, expected) => `history-${action}-${target}-${expected.resourceVersion}`}
          createCandidateRevisionIdempotencyKey={(candidateId, changeSetId, expected) => `revise-${candidateId}-${changeSetId}-${expected.resourceVersion}`}
          createCandidateDiscardIdempotencyKey={(candidateId, changeSetId, expected) => `discard-${candidateId}-${changeSetId}-${expected.resourceVersion}`}
          createCandidateApplyApprovalIdempotencyKey={(candidateId, candidateRevision, expected) => `approval-${candidateId}-${candidateRevision}-${expected.resourceVersion}`}
        >
          <Consumer />
        </WorkflowWorkspaceProvider>,
      );
    });
  }

  it("is the sole reducer owner and replaces committed graph only after authoritative snapshot reconciliation", async () => {
    const client = createClient({
      executeRunnerStudioWorkflowCommand: vi.fn().mockResolvedValue(commandResult()),
    });
    await render(client);
    expect(client.getRunnerStudioWorkflowGraph).toHaveBeenCalledTimes(1);
    expect(client.getRunnerStudioWorkflowGraph).toHaveBeenCalledWith("workflow-a");

    await act(async () => {
      await latest.commands.updateNode("check", { label: "容量检查" }, { reason: "rename node", actionId: "rename-1" });
    });

    expect(latest.state.committed?.revision.resourceVersion).toBe("rv-1");
    expect(latest.state.committed?.graph).toEqual({ nodes: [{ id: "rv-1", type: "action" }], edges: [] });
    expect(latest.pendingCommands).toHaveLength(1);
    expect(latest.state.receiptOrder).toEqual(["txn-1"]);
    expect(latest.state.eventOrder).toEqual(["event-2"]);

    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider workflowId="workflow-a" projection={projection("rv-2", 2)} client={client}>
          <Consumer />
        </WorkflowWorkspaceProvider>,
      );
    });

    expect(latest.state.committed?.revision.resourceVersion).toBe("rv-2");
    expect(latest.pendingCommands).toEqual([]);
    expect(latest.state.receiptOrder).toEqual(["txn-1"]);
    expect(latest.state.eventOrder).toEqual(["event-2"]);
  });

  it("reconciles receipt, validation, and lifecycle facts from the authoritative AppSnapshot", async () => {
    const authoritative = projection();
    authoritative.receipts = [{
      transactionId: "txn-projected",
      changeSetId: "change-projected",
      workflowKey,
      status: "completed",
      revisionBefore: revision("rv-0"),
      revisionAfter: revision("rv-1"),
      validation: validation("rv-1"),
      receivedAt: "2026-07-21T00:00:00Z",
    }];
    authoritative.validationArtifacts = [{
      artifactId: "validation-projected",
      basis: validation("rv-1").basis,
      status: "valid",
      diagnostics: [],
      issuedAt: "2026-07-21T00:00:00Z",
    }];
    authoritative.lifecycleArtifacts = [{
      artifactId: "lifecycle-projected",
      basis: validation("rv-1").basis,
      status: "published",
      issuedAt: "2026-07-21T00:00:00Z",
    }];

    await render(createClient(), authoritative);

    expect(latest.state.receiptOrder).toEqual(["txn-projected"]);
    expect(latest.state.receipts["txn-projected"].status).toBe("completed");
    expect(latest.state.validationArtifacts["validation-projected"].status).toBe("valid");
    expect(latest.state.lifecycleArtifacts["lifecycle-projected"].status).toBe("published");
  });

  it("normalizes null diagnostics from an older authoritative projection without crashing the workspace", async () => {
    const authoritative = projection();
    authoritative.receipts = [{
      transactionId: "txn-legacy",
      changeSetId: "change-legacy",
      workflowKey,
      status: "completed",
      revisionBefore: revision("rv-0"),
      revisionAfter: revision("rv-1"),
      validation: { ...validation("rv-1"), diagnostics: null as never },
      receivedAt: "2026-07-21T00:00:00Z",
    }];
    authoritative.validationArtifacts = [{
      artifactId: "validation-legacy",
      basis: validation("rv-1").basis,
      status: "valid",
      diagnostics: null as never,
      issuedAt: "2026-07-21T00:00:00Z",
    }];

    await render(createClient(), authoritative);

    expect(latest.state.receipts["txn-legacy"].validation.diagnostics).toEqual([]);
    expect(latest.state.validationArtifacts["validation-legacy"].diagnostics).toEqual([]);
  });

  it("sends every manual edit as a typed command with expected revision and idempotency", async () => {
    const client = createClient();
    await render(client);

    await act(async () => {
      await latest.commands.addNode({ id: "check", type: "action", position: { x: 1, y: 2 } }, { reason: "add", actionId: "a1" });
      await latest.commands.updateNode("check", { label: "Check" }, { reason: "update", actionId: "a2" });
      await latest.commands.deleteNode("check", { cascadeEdges: true }, { reason: "delete", actionId: "a3", confirmed: true });
      await latest.commands.connectEdge({ id: "edge-1", source: "start", target: "end" }, { reason: "connect", actionId: "a4" });
      await latest.commands.disconnectEdge("edge-1", { reason: "disconnect", actionId: "a5" });
      await latest.commands.moveNodes(
        { nodes: [{ nodeId: "end", position: { x: 20, y: 40 } }] },
        { reason: "move", actionId: "drag-1" },
      );
    });

    const normalCalls = vi.mocked(client.executeRunnerStudioWorkflowCommand).mock.calls.map(([payload]) => payload);
    const moveCalls = vi.mocked(client.moveRunnerStudioWorkflowNodes).mock.calls.map(([payload]) => payload);
    expect(normalCalls.map((payload) => payload.command?.kind)).toEqual([
      "add_node", "update_node", "delete_node", "connect_edge", "disconnect_edge",
    ]);
    expect(moveCalls).toHaveLength(1);
    for (const payload of [...normalCalls, ...moveCalls]) {
      expect(payload).toMatchObject({
        schemaVersion: "aiops.workflow.edit.v2",
        workflowId: "workflow-a",
        expectedRevision: "rv-1",
      });
      expect(payload.idempotencyKey).toBeTruthy();
      expect(payload).not.toHaveProperty("tenantId");
      expect(payload).not.toHaveProperty("actorId");
    }
  });

  it("submits an ordered manual batch as one request, one pending item, and one receipt", async () => {
    const execute = vi.fn().mockResolvedValue(commandResult("txn-batch", "event-batch"));
    const client = createClient({ executeRunnerStudioWorkflowCommand: execute });
    await render(client);
    const batch = [
      { kind: "disconnect_edge" as const, edgeId: "check-end" },
      { kind: "add_node" as const, node: { id: "notify", type: "action", position: { x: 400, y: 0 } } },
      { kind: "connect_edge" as const, edge: { id: "check-notify", source: "check", target: "notify" } },
      { kind: "connect_edge" as const, edge: { id: "notify-end", source: "notify", target: "end" } },
    ];

    await act(async () => {
      await latest.commands.executeBatch(batch, { reason: "insert node", actionId: "insert-1", confirmed: true });
    });

    expect(execute).toHaveBeenCalledTimes(1);
    expect(client.moveRunnerStudioWorkflowNodes).not.toHaveBeenCalled();
    expect(execute).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      expectedRevision: "rv-1",
      idempotencyKey: "disconnect_edge-insert-1",
      reason: "insert node",
      confirmed: true,
      commands: batch,
    });
    expect(latest.pendingCommands).toEqual([expect.objectContaining({
      idempotencyKey: "disconnect_edge-insert-1",
      expectedRevision: "rv-1",
      transactionId: "txn-batch",
      revisionAfter: "rv-2",
    })]);
    expect(latest.state.receiptOrder).toEqual(["txn-batch"]);
  });

  it("coalesces duplicate drag-stop callbacks into one server command", async () => {
    let resolveMove!: (value: unknown) => void;
    const move = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveMove = resolve; }));
    const client = createClient({ moveRunnerStudioWorkflowNodes: move });
    await render(client);
    const layout = { nodes: [{ nodeId: "check", position: { x: 10, y: 30 } }] };

    let first!: Promise<unknown>;
    let duplicate!: Promise<unknown>;
    act(() => {
      first = latest.commands.moveNodes(layout, { reason: "drag stop", actionId: "pointer-17" });
      duplicate = latest.commands.moveNodes(layout, { reason: "drag stop", actionId: "pointer-17" });
    });
    await act(async () => { await Promise.resolve(); });
    expect(move).toHaveBeenCalledTimes(1);
    expect(first).toBe(duplicate);

    await act(async () => {
      resolveMove(noEffectResult("move-pointer-17"));
      await first;
    });
  });

  it("clears pending when the authoritative snapshot wins the race with the HTTP receipt", async () => {
    let resolveCommand!: (value: unknown) => void;
    const execute = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveCommand = resolve; }));
    const client = createClient({ executeRunnerStudioWorkflowCommand: execute });
    await render(client);

    let request!: Promise<unknown>;
    act(() => {
      request = latest.commands.updateNode("check", { label: "Check" }, { reason: "rename", actionId: "race-1" });
    });
    await act(async () => { await Promise.resolve(); });
    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider workflowId="workflow-a" projection={projection("rv-2", 2)} client={client}>
          <Consumer />
        </WorkflowWorkspaceProvider>,
      );
    });
    expect(latest.pendingCommands).toHaveLength(1);

    await act(async () => {
      resolveCommand(commandResult("txn-race", "event-race"));
      await request;
    });
    expect(latest.state.committed?.revision.resourceVersion).toBe("rv-2");
    expect(latest.pendingCommands).toEqual([]);
  });

  it("keeps committed graph and child form draft when the server rejects the mutation", async () => {
    const failure = Object.assign(new Error("workflow changed"), {
      payload: { errorCode: "STALE_REVISION", message: "workflow changed", retryHint: "reload" },
    });
    const client = createClient({ executeRunnerStudioWorkflowCommand: vi.fn().mockRejectedValue(failure) });

    function DraftConsumer() {
      latest = useWorkflowWorkspace();
      const [draft, setDraft] = useState("draft label");
      return <input value={draft} onChange={(event) => setDraft(event.currentTarget.value)} />;
    }
    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider workflowId="workflow-a" projection={projection()} client={client}>
          <DraftConsumer />
        </WorkflowWorkspaceProvider>,
      );
    });

    await act(async () => {
      await expect(latest.commands.updateNode("check", { label: "draft label" }, { reason: "rename", actionId: "rename-fail" })).rejects.toBe(failure);
    });

    expect(latest.state.committed?.revision.resourceVersion).toBe("rv-1");
    expect((container.querySelector("input") as HTMLInputElement).value).toBe("draft label");
    expect(latest.unsavedError).toMatchObject({ code: "STALE_REVISION", message: "workflow changed", retryHint: "reload" });
    expect(latest.pendingCommands).toEqual([]);
  });

  it("deduplicates repeated receipt/event facts from command response and AppSnapshot", async () => {
    const result = commandResult();
    const client = createClient({ executeRunnerStudioWorkflowCommand: vi.fn().mockResolvedValue(result) });
    await render(client);
    await act(async () => {
      await latest.commands.updateNode("check", { label: "Check" }, { reason: "rename", actionId: "rename-1" });
      latest.reconcile({ ...projection(), events: [result.event, result.event] });
      latest.reconcile({ ...projection(), events: [result.event] });
    });

    expect(latest.state.receiptOrder).toEqual(["txn-1"]);
    expect(latest.state.eventOrder).toEqual(["event-2"]);
  });

  it("prepares Undo as a reviewed candidate with full revision and never sends or mutates the committed graph", async () => {
    const execute = vi.fn().mockResolvedValue(historyPreviewResult());
    const client = createClient({ executeRunnerStudioWorkflowHistoryAction: execute });
    await render(client);

    await act(async () => {
      await latest.undoHistory("txn-17");
    });

    expect(execute).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      expected: revision("rv-1"),
      targetTransactionId: "txn-17",
      idempotencyKey: "history-undo-txn-17-rv-1",
      action: "undo",
    });
    expect(JSON.stringify(execute.mock.calls)).not.toContain("graph");
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.state.committed?.graph).toEqual({ nodes: [{ id: "rv-1", type: "action" }], edges: [] });
    expect(latest.state.candidate).toMatchObject({
      candidate: { id: "history-candidate-1", state: "ready" },
      changeSet: { id: "history-change-1", state: "previewed", base: revision("rv-1") },
    });
    expect(latest.historyPending).toBeUndefined();
    expect(latest.historyError).toBeUndefined();
  });

  it("coalesces an in-flight history replay and reuses its stable idempotency key on retry", async () => {
    let resolveHistory!: (value: unknown) => void;
    const execute = vi.fn()
      .mockImplementationOnce(() => new Promise((resolve) => { resolveHistory = resolve; }))
      .mockResolvedValue(historyPreviewResult());
    const client = createClient({ executeRunnerStudioWorkflowHistoryAction: execute });
    await render(client);

    let first!: Promise<unknown>;
    let duplicate!: Promise<unknown>;
    act(() => {
      first = latest.redoHistory("txn-17");
      duplicate = latest.redoHistory("txn-17");
    });
    await act(async () => { await Promise.resolve(); });
    expect(first).toBe(duplicate);
    expect(execute).toHaveBeenCalledTimes(1);
    expect(latest.historyPending).toMatchObject({ action: "redo", targetTransactionId: "txn-17" });

    await act(async () => {
      resolveHistory(historyPreviewResult());
      await first;
    });
    await act(async () => {
      await latest.redoHistory("txn-17");
    });

    expect(execute).toHaveBeenCalledTimes(2);
    expect(execute.mock.calls[0][0].idempotencyKey).toBe("history-redo-txn-17-rv-1");
    expect(execute.mock.calls[1][0].idempotencyKey).toBe(execute.mock.calls[0][0].idempotencyKey);
  });

  it("requests a controlled Apply approval once from the persisted History Candidate and only resumes its exact transport", async () => {
    let resolveApproval!: (value: unknown) => void;
    const requestApproval = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveApproval = resolve; }));
    const client = createClient({
      executeRunnerStudioWorkflowHistoryAction: vi.fn().mockResolvedValue(historyPreviewResult({ candidateId: persistedHistoryCandidateId })),
      requestRunnerStudioWorkflowCandidateApplyApproval: requestApproval,
    });
    const resumeEvents: WorkflowTransportResumeEventDetail[] = [];
    const applyEvents = vi.fn();
    const onResume = (event: Event) => resumeEvents.push((event as CustomEvent<WorkflowTransportResumeEventDetail>).detail);
    window.addEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, onResume);
    window.addEventListener(WORKFLOW_CHANGE_ACTION_EVENT, applyEvents);
    await render(client);
    await act(async () => latest.undoHistory("txn-17"));

    let first!: Promise<unknown>;
    let duplicate!: Promise<unknown>;
    act(() => {
      first = latest.requestCandidateApplyApproval!();
      duplicate = latest.requestCandidateApplyApproval!();
    });
    await act(async () => Promise.resolve());
    expect(first).toBe(duplicate);
    expect(requestApproval).toHaveBeenCalledTimes(1);
    expect(requestApproval).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: persistedHistoryCandidateId,
      candidateRevision: 1,
      idempotencyKey: `approval-${persistedHistoryCandidateId}-1-rv-1`,
      action: "request_apply_approval",
    });
    expect(JSON.stringify(requestApproval.mock.calls)).not.toMatch(/(changeSetId|previewHash|expectedRevision|approvalId|actionToken|actorId|canvasSessionId|sessionId)/);
    expect(latest.candidateActionPending?.action).toBe("request_approval");
    expect(resumeEvents).toEqual([]);
    expect(applyEvents).not.toHaveBeenCalled();

    await act(async () => {
      resolveApproval(candidateApplyApprovalResult());
      await first;
    });
    expect(latest.candidateApplyApproval).toMatchObject({
      status: "requested",
      candidateId: persistedHistoryCandidateId,
      approvalId: "approval-history-apply-1",
      expiresAt: "2030-01-01T00:00:00Z",
    });
    expect(resumeEvents).toEqual([{
      schemaVersion: "aiops.workflow.transport.resume.v1",
      workflowId: "workflow-a",
      threadId: "workflow:workflow-a",
      sessionId: "session-workflow-a",
      turnId: "turn-history-apply-1",
      approvalId: "approval-history-apply-1",
    }]);
    expect(applyEvents).not.toHaveBeenCalled();
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.state.candidate?.candidate.id).toBe(persistedHistoryCandidateId);
    window.removeEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, onResume);
    window.removeEventListener(WORKFLOW_CHANGE_ACTION_EVENT, applyEvents);
  });

  it("reuses one approval idempotency key after an error and never clears or applies the Candidate", async () => {
    const failure = Object.assign(new Error("approval service unavailable"), {
      payload: { errorCode: "HTTP_503", message: "确认服务暂不可用", retryHint: "稍后重试" },
    });
    const requestApproval = vi.fn()
      .mockRejectedValueOnce(failure)
      .mockResolvedValue(candidateApplyApprovalResult());
    const client = createClient({
      executeRunnerStudioWorkflowHistoryAction: vi.fn().mockResolvedValue(historyPreviewResult({ candidateId: persistedHistoryCandidateId })),
      requestRunnerStudioWorkflowCandidateApplyApproval: requestApproval,
    });
    await render(client);
    await act(async () => latest.undoHistory("txn-17"));

    await act(async () => {
      await expect(latest.requestCandidateApplyApproval!()).rejects.toBe(failure);
    });
    expect(latest.candidateActionError).toMatchObject({
      action: "request_approval", code: "HTTP_503", message: "确认服务暂不可用", retryHint: "稍后重试",
    });
    expect(latest.state.candidate?.candidate.id).toBe(persistedHistoryCandidateId);
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));

    await act(async () => { await latest.requestCandidateApplyApproval!(); });
    expect(requestApproval).toHaveBeenCalledTimes(2);
    expect(requestApproval.mock.calls[1][0].idempotencyKey).toBe(requestApproval.mock.calls[0][0].idempotencyKey);
  });

  it("does not let an unrevised Agent Candidate bypass its transport-owned Tool approval", async () => {
    const requestApproval = vi.fn();
    const client = createClient({ requestRunnerStudioWorkflowCandidateApplyApproval: requestApproval });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    await act(async () => {
      await expect(latest.requestCandidateApplyApproval!()).rejects.toMatchObject({
        code: "REVISED_CANDIDATE_REQUIRED",
      });
    });

    expect(requestApproval).not.toHaveBeenCalled();
    expect(latest.state.candidate).toMatchObject({
      candidate: { id: "candidate-a", currentRevision: 1 },
      changeSet: { id: "change-1", actor: "agent" },
    });
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
  });

  it("keeps committed truth unchanged and exposes retryable conflict details when history preparation fails", async () => {
    const failure = Object.assign(new Error("事务与后续修改冲突"), {
      payload: { errorCode: "CONFLICT", message: "事务与后续修改冲突", retryHint: "刷新后审阅三方差异" },
    });
    const client = createClient({ executeRunnerStudioWorkflowHistoryAction: vi.fn().mockRejectedValue(failure) });
    await render(client);

    await act(async () => {
      await expect(latest.undoHistory("txn-17")).rejects.toBe(failure);
    });

    expect(latest.state.candidate).toBeUndefined();
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.historyPending).toBeUndefined();
    expect(latest.historyError).toMatchObject({
      action: "undo", targetTransactionId: "txn-17", code: "CONFLICT",
      message: "事务与后续修改冲突", retryHint: "刷新后审阅三方差异",
    });
  });

  it("rejects a valid-looking history response if an authoritative snapshot wins the race", async () => {
    let resolveHistory!: (value: unknown) => void;
    const execute = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveHistory = resolve; }));
    const client = createClient({ executeRunnerStudioWorkflowHistoryAction: execute });
    await render(client);
    let request!: Promise<unknown>;
    act(() => { request = latest.undoHistory("txn-17"); });
    await act(async () => { await Promise.resolve(); });

    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider workflowId="workflow-a" projection={projection("rv-2", 2)} client={client}>
          <Consumer />
        </WorkflowWorkspaceProvider>,
      );
    });
    await act(async () => {
      resolveHistory(historyPreviewResult());
      await expect(request).rejects.toMatchObject({ code: "STALE_REVISION" });
    });

    expect(latest.state.committed?.revision).toEqual(revision("rv-2"));
    expect(latest.state.candidate).toBeUndefined();
    expect(latest.historyError).toMatchObject({ code: "STALE_REVISION", targetTransactionId: "txn-17" });
  });

  it("revises the current agent Candidate with one canonical operation and supersedes the old preview only", async () => {
    const revised = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1", intent: "用户调整候选节点参数",
      operations: [
        { op: "update_node", nodeId: "check", set: { label: "旧候选名称" } },
        { op: "update_node", nodeId: "check", set: { label: "新候选名称" } },
      ],
    });
    const execute = vi.fn().mockResolvedValue(revised);
    const client = createClient({ reviseRunnerStudioWorkflowCandidate: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    await act(async () => {
      await latest.reviseCandidate(
        { op: "update_node", nodeId: "check", set: { label: "新候选名称" } },
        { intent: "用户调整候选节点参数", actionId: "inspector-check" },
      );
    });

    expect(execute).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-a",
      baseChangeSetId: "change-1",
      expected: revision("rv-1"),
      idempotencyKey: "revise-candidate-a-change-1-rv-1",
      intent: "用户调整候选节点参数",
      additionalOperation: { op: "update_node", nodeId: "check", set: { label: "新候选名称" } },
    });
    expect(JSON.stringify(execute.mock.calls)).not.toMatch(/"(graph|tenantId|actorId|canvasSessionId)"/);
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.state.committed?.graph).toEqual({ nodes: [{ id: "rv-1", type: "action" }], edges: [] });
    expect(latest.state.candidate).toMatchObject({
      candidate: { id: "candidate-a", currentRevision: 2, currentChangeSetId: "change-2", currentPreviewHash: "preview-2" },
      changeSet: { id: "change-2", parentChangeSetId: "change-1", state: "previewed" },
    });
    expect(latest.state.candidate?.changeSet.id).not.toBe("change-1");
    expect(latest.candidateActionPending).toBeUndefined();
    expect(latest.candidateActionError).toBeUndefined();
  });

  it("requests a fresh server-owned approval for the exact user-revised Agent Candidate v2", async () => {
    const revised = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1", intent: "用户调整候选节点参数",
    });
    let resolveApproval!: (value: unknown) => void;
    const requestApproval = vi.fn().mockImplementation(() => new Promise((resolve) => { resolveApproval = resolve; }));
    const client = createClient({
      reviseRunnerStudioWorkflowCandidate: vi.fn().mockResolvedValue(revised),
      requestRunnerStudioWorkflowCandidateApplyApproval: requestApproval,
    });
    const resumeEvents: WorkflowTransportResumeEventDetail[] = [];
    const onResume = (event: Event) => resumeEvents.push((event as CustomEvent<WorkflowTransportResumeEventDetail>).detail);
    window.addEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, onResume);
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    await act(async () => {
      await latest.reviseCandidate(
        { op: "update_node", nodeId: "check", set: { label: "用户确认的新名称" } },
        { intent: "用户调整候选节点参数", actionId: "inspector-check" },
      );
    });
    act(() => { void latest.requestCandidateApplyApproval!().catch(() => undefined); });
    await vi.waitFor(() => expect(requestApproval).toHaveBeenCalledTimes(1));

    expect(requestApproval).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-a",
      candidateRevision: 2,
      idempotencyKey: "approval-candidate-a-2-rv-1",
      action: "request_apply_approval",
    });
    expect(JSON.stringify(requestApproval.mock.calls)).not.toMatch(
      /(changeSetId|previewHash|expectedRevision|approvalId|actionToken|actorId|canvasSessionId|sessionId)/,
    );
    expect(resumeEvents).toEqual([]);

    await act(async () => {
      resolveApproval(candidateApplyApprovalResult("candidate-a", 2, "change-2", "preview-2"));
      await Promise.resolve();
    });
    await vi.waitFor(() => expect(latest.candidateApplyApproval).toMatchObject({
      status: "requested",
      candidateId: "candidate-a",
      candidateRevision: 2,
      changeSetId: "change-2",
      previewHash: "preview-2",
    }));
    expect(resumeEvents).toHaveLength(1);
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.state.candidate?.candidate.currentRevision).toBe(2);
    window.removeEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, onResume);
  });

  it("rejects a stale v1 approval response after the Candidate has become v2", async () => {
    const revised = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1", intent: "用户调整候选节点参数",
    });
    const requestApproval = vi.fn().mockResolvedValue(
      candidateApplyApprovalResult("candidate-a", 1, "change-1", "preview-1"),
    );
    const client = createClient({
      reviseRunnerStudioWorkflowCandidate: vi.fn().mockResolvedValue(revised),
      requestRunnerStudioWorkflowCandidateApplyApproval: requestApproval,
    });
    const onResume = vi.fn();
    window.addEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, onResume);
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    await act(async () => {
      await latest.reviseCandidate(
        { op: "update_node", nodeId: "check", set: { label: "用户确认的新名称" } },
        { intent: "用户调整候选节点参数" },
      );
    });
    act(() => { void latest.requestCandidateApplyApproval!().catch(() => undefined); });
    await vi.waitFor(() => expect(latest.candidateActionError).toMatchObject({
      action: "request_approval",
      code: "INVALID_CANDIDATE_APPROVAL",
    }));

    expect(latest.candidateApplyApproval).toBeUndefined();
    expect(onResume).not.toHaveBeenCalled();
    expect(latest.state.candidate?.candidate.currentRevision).toBe(2);
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    window.removeEventListener(WORKFLOW_TRANSPORT_RESUME_EVENT, onResume);
  });

  it("sends the exact latest selector for a user-revised Agent Candidate v3", async () => {
    const revised = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-3", previewHash: "preview-3", candidateRevision: 3,
      parentChangeSetId: "change-2", intent: "用户继续调整候选节点参数",
    });
    const requestApproval = vi.fn().mockResolvedValue(
      candidateApplyApprovalResult("candidate-a", 3, "change-3", "preview-3"),
    );
    const client = createClient({ requestRunnerStudioWorkflowCandidateApplyApproval: requestApproval });
    await render(client);
    await act(async () => latest.reconcileTransportFact({
      kind: "candidate",
      artifactId: "candidate-revise-candidate-a-3",
      fact: {
        candidate: { ...revised.candidate },
        changeSet: { ...revised.changeSet, actor: "user", state: "previewed" },
      },
    } as WorkflowTransportArtifactFact));

    await act(async () => { await latest.requestCandidateApplyApproval!(); });

    expect(requestApproval).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-a",
      candidateRevision: 3,
      idempotencyKey: "approval-candidate-a-3-rv-1",
      action: "request_apply_approval",
    });
    expect(latest.candidateApplyApproval).toMatchObject({
      candidateId: "candidate-a", candidateRevision: 3, changeSetId: "change-3", previewHash: "preview-3",
    });
  });

  it("keeps a Candidate editable across authoritative layout-only drift and asks the server for safe rebase", async () => {
    const currentAfterLayoutMove = {
      ...revision("rv-2"),
      semanticHash: revision("rv-1").semanticHash,
    };
    const revised = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1", intent: "用户调整候选节点参数",
      base: currentAfterLayoutMove,
      rebased: true,
      operations: [
        { op: "update_node", nodeId: "check", set: { label: "旧候选名称" } },
        { op: "update_node", nodeId: "check", set: { label: "安全合并后的候选名称" } },
      ],
    });
    const execute = vi.fn().mockResolvedValue(revised);
    const client = createClient({ reviseRunnerStudioWorkflowCandidate: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    const layoutDrift = projection("rv-2", 2);
    if (layoutDrift.committed) layoutDrift.committed.revision = currentAfterLayoutMove;
    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider
          workflowId="workflow-a"
          projection={layoutDrift}
          client={client}
          createCandidateRevisionIdempotencyKey={(candidateId, changeSetId, expected) => `revise-${candidateId}-${changeSetId}-${expected.resourceVersion}`}
        >
          <Consumer />
        </WorkflowWorkspaceProvider>,
      );
    });

    // The display-only Candidate base follows the authoritative layout so the
    // Ghost stays interactive; request identity remains bound to change-1.
    expect(latest.state.candidate?.changeSet.base).toEqual(currentAfterLayoutMove);
    await act(async () => {
      await latest.reviseCandidate(
        { op: "update_node", nodeId: "check", set: { label: "安全合并后的候选名称" } },
        { intent: "用户调整候选节点参数", actionId: "safe-rebase-check" },
      );
    });

    expect(execute).toHaveBeenCalledWith(expect.objectContaining({
      candidateId: "candidate-a",
      baseChangeSetId: "change-1",
      expected: currentAfterLayoutMove,
      additionalOperation: { op: "update_node", nodeId: "check", set: { label: "安全合并后的候选名称" } },
    }));
    expect(latest.state.candidate).toMatchObject({
      candidate: { currentRevision: 2, currentChangeSetId: "change-2" },
      changeSet: { id: "change-2", base: currentAfterLayoutMove },
    });
    expect(latest.state.committed?.revision).toEqual(currentAfterLayoutMove);
  });

  it("keeps semantic drift stale and never attempts a browser-side Candidate rebase", async () => {
    const execute = vi.fn();
    const client = createClient({ reviseRunnerStudioWorkflowCandidate: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));
    await act(async () => {
      root.render(
        <WorkflowWorkspaceProvider workflowId="workflow-a" projection={projection("rv-2", 2)} client={client}>
          <Consumer />
        </WorkflowWorkspaceProvider>,
      );
    });

    await act(async () => {
      await expect(latest.reviseCandidate(
        { op: "update_node", nodeId: "check", set: { label: "不应提交" } },
        { intent: "用户调整候选节点参数" },
      )).rejects.toMatchObject({ code: "STALE_CHANGESET" });
    });
    expect(execute).not.toHaveBeenCalled();
    expect(latest.state.candidate?.changeSet.base.resourceVersion).toBe("rv-1");
  });

  it("coalesces concurrent identical Candidate revisions and reuses one stable key on retry", async () => {
    let rejectFirst!: (reason: unknown) => void;
    const failure = Object.assign(new Error("temporary failure"), { payload: { errorCode: "TEMPORARY" } });
    const revised = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1",
    });
    const execute = vi.fn()
      .mockImplementationOnce(() => new Promise((_resolve, reject) => { rejectFirst = reject; }))
      .mockResolvedValue(revised);
    const client = createClient({ reviseRunnerStudioWorkflowCandidate: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));
    const operation = { op: "update_layout", layout: { nodes: [{ nodeId: "check", position: { x: 10, y: 20 } }] } } as const;
    let first!: Promise<unknown>;
    let duplicate!: Promise<unknown>;
    act(() => {
      first = latest.reviseCandidate(operation, { intent: "用户调整候选节点位置" });
      duplicate = latest.reviseCandidate(operation, { intent: "用户调整候选节点位置" });
    });
    await act(async () => Promise.resolve());
    expect(first).toBe(duplicate);
    expect(execute).toHaveBeenCalledTimes(1);
    await act(async () => {
      rejectFirst(failure);
      await expect(first).rejects.toBe(failure);
    });
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.state.candidate?.candidate.currentChangeSetId).toBe("change-1");
    expect(latest.candidateActionError).toMatchObject({ action: "revise", code: "TEMPORARY" });
    await act(async () => {
      await latest.reviseCandidate(operation, { intent: "用户调整候选节点位置" });
    });
    expect(execute).toHaveBeenCalledTimes(2);
    expect(execute.mock.calls[1][0].idempotencyKey).toBe(execute.mock.calls[0][0].idempotencyKey);
  });

  it("rejects cross-scope or expanded revise responses and preserves committed and Candidate draft truth", async () => {
    const crossScope = historyPreviewResult({
      candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
      parentChangeSetId: "change-1",
    });
    crossScope.changeSet.workflowKey = { tenantId: "tenant-b", workflowId: "workflow-a" };
    crossScope.candidate.workflowKey = { tenantId: "tenant-b", workflowId: "workflow-a" };
    const expanded = {
      ...historyPreviewResult({
        candidateId: "candidate-a", changeSetId: "change-2", previewHash: "preview-2", candidateRevision: 2,
        parentChangeSetId: "change-1",
      }),
      forged: true,
    };
    const execute = vi.fn().mockResolvedValueOnce(crossScope).mockResolvedValueOnce(expanded);
    const client = createClient({ reviseRunnerStudioWorkflowCandidate: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    for (const actionId of ["cross-scope", "unknown-field"]) {
      await act(async () => {
        await expect(latest.reviseCandidate(
          { op: "update_edge", edgeId: "check-end", set: { target: "end-2" } },
          { intent: "用户重新连接候选节点", actionId },
        )).rejects.toMatchObject({ code: "INVALID_WORKFLOW_PREVIEW" });
      });
    }

    expect(execute).toHaveBeenCalledTimes(2);
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.state.candidate).toMatchObject({
      candidate: { currentRevision: 1, currentChangeSetId: "change-1", currentPreviewHash: "preview-1" },
    });
    expect(latest.candidateActionError).toMatchObject({ action: "revise", code: "INVALID_WORKFLOW_PREVIEW" });
  });

  it("discards only the exactly bound Candidate and leaves committed truth unchanged", async () => {
    const execute = vi.fn().mockResolvedValue(candidateDiscardResult());
    const client = createClient({ executeRunnerStudioWorkflowCandidateAction: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    await act(async () => latest.discardCandidate());

    expect(execute).toHaveBeenCalledWith({
      schemaVersion: "aiops.workflow.edit.v2",
      workflowId: "workflow-a",
      candidateId: "candidate-a",
      candidateRevision: 1,
      changeSetId: "change-1",
      previewHash: "preview-1",
      expected: revision("rv-1"),
      idempotencyKey: "discard-candidate-a-change-1-rv-1",
      action: "discard",
    });
    expect(latest.state.candidate).toBeUndefined();
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.candidateActionError).toBeUndefined();
  });

  it("discards an Undo Candidate whose validation semantic hash describes the projected preview", async () => {
    const execute = vi.fn().mockResolvedValue(candidateDiscardResult());
    const client = createClient({ executeRunnerStudioWorkflowCandidateAction: execute });
    const artifact = candidateTransportFact();
    artifact.fact.changeSet.validation.basis.semanticHash = "semantic-projected-undo-preview";
    await render(client);
    await act(async () => latest.reconcileTransportFact(artifact));

    await act(async () => latest.discardCandidate());

    expect(execute).toHaveBeenCalledTimes(1);
    expect(latest.state.candidate).toBeUndefined();
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.candidateActionError).toBeUndefined();
  });

  it("surfaces a stale Candidate binding error instead of silently ignoring the action", async () => {
    const client = createClient();
    const artifact = candidateTransportFact();
    artifact.fact.changeSet.base.semanticHash = "stale-base-semantic";
    await render(client);
    await act(async () => latest.reconcileTransportFact(artifact));

    await act(async () => {
      await expect(latest.discardCandidate()).rejects.toMatchObject({ code: "CANDIDATE_NOT_AVAILABLE" });
    });

    expect(latest.candidateActionError).toMatchObject({ action: "discard", code: "CANDIDATE_NOT_AVAILABLE" });
  });

  it("replays Candidate discard with the same key after a 500 and never fake-clears the draft", async () => {
    const failure = Object.assign(new Error("server unavailable"), {
      payload: { errorCode: "HTTP_500", message: "server unavailable", retryHint: "retry" },
    });
    const execute = vi.fn()
      .mockRejectedValueOnce(failure)
      .mockResolvedValue(candidateDiscardResult());
    const client = createClient({ executeRunnerStudioWorkflowCandidateAction: execute });
    await render(client);
    await act(async () => latest.reconcileTransportFact(candidateTransportFact()));

    await act(async () => {
      await expect(latest.discardCandidate()).rejects.toBe(failure);
    });
    expect(latest.state.candidate?.candidate.currentChangeSetId).toBe("change-1");
    expect(latest.state.committed?.revision).toEqual(revision("rv-1"));
    expect(latest.candidateActionError).toMatchObject({ action: "discard", code: "HTTP_500", retryHint: "retry" });

    await act(async () => latest.discardCandidate());
    expect(execute).toHaveBeenCalledTimes(2);
    expect(execute.mock.calls[1][0].idempotencyKey).toBe(execute.mock.calls[0][0].idempotencyKey);
    expect(latest.state.candidate).toBeUndefined();
  });

  it("reconciles typed transport facts idempotently and rejects facts for another workspace", async () => {
    await render(createClient());
    const candidate: WorkflowTransportArtifactFact = {
      kind: "candidate",
      artifactId: "preview-artifact-1",
      fact: {
        candidate: {
          id: "candidate-a", workflowKey, currentChangeSetId: "change-1", currentRevision: 1,
          currentPreviewHash: "preview-1", state: "ready", expiresAt: "2026-07-22T00:00:00Z",
        },
        changeSet: {
          schemaVersion: "aiops.workflow.edit.v2", id: "change-1", workflowKey, candidateId: "candidate-a",
          candidateRevision: 1, actor: "agent", intent: "rename", base: revision("rv-1"),
          operations: [{ op: "update_node", nodeId: "check", set: { label: "容量检查" } }],
          previewHash: "preview-1", validation: validation("rv-1"), state: "previewed",
          createdAt: "2026-07-21T00:00:00Z", expiresAt: "2026-07-22T00:00:00Z",
        },
      },
    };
    const receipt: WorkflowTransportArtifactFact = {
      kind: "receipt",
      artifactId: "receipt-artifact-1",
      fact: {
        transactionId: "txn-transport", changeSetId: "change-1", workflowKey, status: "verifying",
        revisionBefore: revision("rv-1"), revisionAfter: revision("rv-2"), validation: validation("rv-2"),
        receivedAt: "2026-07-21T00:00:01Z",
      },
    };
    const validationFact: WorkflowTransportArtifactFact = {
      kind: "validation",
      artifactId: "validation-artifact-1",
      fact: {
        artifactId: "validation-artifact-1", basis: validation("rv-1").basis, status: "valid",
        diagnostics: [], issuedAt: "2026-07-21T00:00:02Z",
      },
    };

    await act(async () => {
      latest.reconcileTransportFact(candidate);
      latest.reconcileTransportFact(candidate);
      latest.reconcileTransportFact(receipt);
      latest.reconcileTransportFact(receipt);
      latest.reconcileTransportFact(validationFact);
      latest.reconcileTransportFact(validationFact);
      latest.reconcileTransportFact({
        ...candidate,
        fact: {
          ...candidate.fact,
          candidate: { ...candidate.fact.candidate, workflowKey: { tenantId: "tenant-a", workflowId: "workflow-b" } },
          changeSet: { ...candidate.fact.changeSet, workflowKey: { tenantId: "tenant-a", workflowId: "workflow-b" } },
        },
      });
    });

    expect(latest.state.candidate?.candidate.id).toBe("candidate-a");
    expect(latest.state.receiptOrder).toEqual(["txn-transport"]);
    expect(Object.keys(latest.state.validationArtifacts)).toEqual(["validation-artifact-1"]);
    expect(latest.state.workflowKey).toEqual(workflowKey);
  });
});
