import { describe, expect, it } from "vitest";
import { createWorkflowWorkspaceState, workflowWorkspaceReducer } from "./workflowWorkspaceReducer";
import type { WorkflowCandidateFact, WorkflowReceiptFact } from "./workflowWorkspaceTypes";
import type { WorkflowDomainEventV2, WorkflowRevision } from "./workflowMcpContracts";

const key = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (resourceVersion: string): WorkflowRevision => ({ resourceVersion, semanticHash: `sha256:${resourceVersion}`, layoutHash: `sha256:layout-${resourceVersion}` });
const basis = (resourceVersion: string) => ({ resourceVersion, semanticHash: `sha256:${resourceVersion}`, catalogVersion: "catalog-1", actionSchemaDigest: "actions-1", referenceSnapshotHash: "refs-1", environmentBindingVersion: "env-1" });

function candidate(candidateRevision = 1, state = "previewed"): WorkflowCandidateFact {
  return {
    candidate: { id: "candidate-a", workflowKey: key, currentChangeSetId: `change-${candidateRevision}`, currentRevision: candidateRevision, currentPreviewHash: `preview-${candidateRevision}`, state: state === "proposed" ? "building" : "ready", expiresAt: "2030-01-01T00:00:00Z" },
    changeSet: {
      schemaVersion: "aiops.workflow.edit.v2", id: `change-${candidateRevision}`, workflowKey: key, candidateId: "candidate-a", candidateRevision,
      actor: "agent", intent: "add check", base: revision("rv-1"), operations: [{ op: "delete_edge", edgeId: "edge-a" }],
      previewHash: `preview-${candidateRevision}`, validation: { basis: basis("rv-1"), valid: true, diagnostics: [] },
      state: state as "proposed" | "previewed", createdAt: "2026-07-21T00:00:00Z", expiresAt: "2030-01-01T00:00:00Z",
    },
  };
}

function distinctCandidate(candidateId: string, changeSetId: string, previewHash: string): WorkflowCandidateFact {
  const fact = candidate(1);
  return {
    candidate: {
      ...fact.candidate,
      id: candidateId,
      currentChangeSetId: changeSetId,
      currentPreviewHash: previewHash,
    },
    changeSet: {
      ...fact.changeSet,
      id: changeSetId,
      candidateId,
      previewHash,
    },
  };
}

function receipt(status: WorkflowReceiptFact["status"] = "verifying", transactionId = "txn-1"): WorkflowReceiptFact {
  return { transactionId, changeSetId: "change-1", workflowKey: key, status, revisionBefore: revision("rv-1"), revisionAfter: revision("rv-2"), validation: { basis: basis("rv-2"), valid: status !== "saved_with_issues", diagnostics: [] }, receivedAt: "2026-07-21T00:00:01Z" };
}

function event(sequence: number, eventId = `event-${sequence}`): WorkflowDomainEventV2 {
  return { schemaVersion: "aiops.workflow.edit.v2", eventId, sequence, type: "transaction.observed", workflowKey: key, revision: revision("rv-1"), actor: "system", payload: {}, occurredAt: "2026-07-21T00:00:00Z" };
}

describe("workflowWorkspaceReducer", () => {
  it("deduplicates immutable receipts and accepts only forward lifecycle projection", () => {
    let state = createWorkflowWorkspaceState();
    state = workflowWorkspaceReducer(state, { type: "receipt.received", receipt: receipt() });
    const duplicate = workflowWorkspaceReducer(state, { type: "receipt.received", receipt: receipt() });
    expect(duplicate).toBe(state);
    state = workflowWorkspaceReducer(state, { type: "receipt.received", receipt: receipt("completed") });
    expect(state.receiptOrder).toEqual(["txn-1"]);
    expect(state.receipts["txn-1"].status).toBe("completed");
    expect(workflowWorkspaceReducer(state, { type: "receipt.received", receipt: receipt("verifying") })).toBe(state);
    expect(workflowWorkspaceReducer(state, { type: "receipt.received", receipt: { ...receipt("completed"), changeSetId: "forged" } })).toBe(state);
  });

  it("keeps the newest typed candidate revision and clears only the matching candidate", () => {
    let state = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "candidate.received", candidate: candidate(2) });
    expect(workflowWorkspaceReducer(state, { type: "candidate.received", candidate: candidate(1) })).toBe(state);
    state = workflowWorkspaceReducer(state, { type: "candidate.received", candidate: candidate(3) });
    expect(state.candidate?.changeSet.id).toBe("change-3");
    expect(workflowWorkspaceReducer(state, { type: "candidate.cleared", candidateId: "other" })).toBe(state);
    expect(workflowWorkspaceReducer(state, { type: "candidate.cleared", candidateId: "candidate-a" }).candidate).toBeUndefined();
  });

  it("replaces a revision-one Candidate with a later distinct revision-one Candidate", () => {
    const retained = distinctCandidate("candidate-retained", "change-retained", "preview-retained");
    const replacement = distinctCandidate("candidate-repreviewed", "change-repreviewed", "preview-repreviewed");
    let state = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "candidate.received", candidate: retained });

    state = workflowWorkspaceReducer(state, { type: "candidate.received", candidate: replacement });

    expect(state.candidate).toEqual(replacement);
    expect(state.candidate?.candidate.currentRevision).toBe(1);
  });

  it("buffers out-of-order events, detects a gap and drains in sequence order", () => {
    let state = createWorkflowWorkspaceState();
    state = workflowWorkspaceReducer(state, { type: "event.received", event: event(2) });
    expect(state.gap).toEqual({ expectedSequence: 1, receivedSequence: 2 });
    expect(state.eventOrder).toEqual([]);
    state = workflowWorkspaceReducer(state, { type: "event.received", event: event(1) });
    expect(state.gap).toBeUndefined();
    expect(state.lastSequence).toBe(2);
    expect(state.eventOrder).toEqual(["event-1", "event-2"]);
    expect(workflowWorkspaceReducer(state, { type: "event.received", event: event(2) })).toBe(state);
  });

  it("keeps selection and viewport ephemeral without changing committed semantic revision", () => {
    let state = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-1"), graph: { nodes: [] }, sequence: 4, loadedAt: "2026-07-21T00:00:00Z" } });
    const committed = state.committed;
    state = workflowWorkspaceReducer(state, { type: "selection.changed", selection: { nodeIds: ["b", "a", "a"], edgeIds: ["e"] } });
    state = workflowWorkspaceReducer(state, { type: "viewport.changed", viewport: { x: 10, y: 20, zoom: 1.2 } });
    expect(state.selection.nodeIds).toEqual(["a", "b"]);
    expect(state.committed).toEqual(committed);
  });

  it("clears the exact applied Candidate when receipt and committed snapshot reconcile in either order", () => {
    const snapshot = { workflowKey: key, revision: revision("rv-2"), graph: { nodes: [] }, sequence: 2, loadedAt: "2026-07-21T00:00:02Z" };

    let receiptFirst = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "snapshot.loaded", snapshot: { ...snapshot, revision: revision("rv-1"), sequence: 1 } });
    receiptFirst = workflowWorkspaceReducer(receiptFirst, { type: "candidate.received", candidate: candidate() });
    receiptFirst = workflowWorkspaceReducer(receiptFirst, { type: "receipt.received", receipt: receipt() });
    receiptFirst = workflowWorkspaceReducer(receiptFirst, { type: "snapshot.loaded", snapshot });
    expect(receiptFirst.candidate).toBeUndefined();

    let snapshotFirst = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "snapshot.loaded", snapshot: { ...snapshot, revision: revision("rv-1"), sequence: 1 } });
    snapshotFirst = workflowWorkspaceReducer(snapshotFirst, { type: "candidate.received", candidate: candidate() });
    snapshotFirst = workflowWorkspaceReducer(snapshotFirst, { type: "snapshot.loaded", snapshot });
    expect(snapshotFirst.candidate).toBeDefined();
    snapshotFirst = workflowWorkspaceReducer(snapshotFirst, { type: "receipt.received", receipt: receipt() });
    expect(snapshotFirst.candidate).toBeUndefined();
  });

  it("does not clear a Candidate for an unrelated or rejected receipt", () => {
    const snapshot = { workflowKey: key, revision: revision("rv-2"), graph: {}, sequence: 2, loadedAt: "2026-07-21T00:00:02Z" };
    let state = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "snapshot.loaded", snapshot: { ...snapshot, revision: revision("rv-1"), sequence: 1 } });
    state = workflowWorkspaceReducer(state, { type: "candidate.received", candidate: candidate() });
    state = workflowWorkspaceReducer(state, { type: "receipt.received", receipt: { ...receipt("rejected"), revisionAfter: undefined } });
    state = workflowWorkspaceReducer(state, { type: "snapshot.loaded", snapshot });
    expect(state.candidate).toBeDefined();
  });

  it("ignores facts for another workflow instead of merging tenants", () => {
    const state = workflowWorkspaceReducer(createWorkflowWorkspaceState(), { type: "snapshot.loaded", snapshot: { workflowKey: key, revision: revision("rv-1"), graph: {}, sequence: 1, loadedAt: "2026-07-21T00:00:00Z" } });
    const foreign = { ...receipt(), workflowKey: { tenantId: "tenant-b", workflowId: "workflow-a" } };
    expect(workflowWorkspaceReducer(state, { type: "receipt.received", receipt: foreign })).toBe(state);
  });
});
