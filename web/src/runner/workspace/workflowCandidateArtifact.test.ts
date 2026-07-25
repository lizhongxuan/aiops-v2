import { describe, expect, it } from "vitest";

import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import {
  parseWorkflowPreviewResult,
  parseWorkflowTransportArtifact,
  workflowTransportArtifactIdentity,
  type WorkflowTransportArtifactScope,
} from "./workflowCandidateArtifact";

const workflowKey = { tenantId: "tenant-a", workflowId: "workflow-a" };
const revision = (resourceVersion: string) => ({
  resourceVersion,
  semanticHash: `semantic-${resourceVersion}`,
  layoutHash: `layout-${resourceVersion}`,
});
const basis = (resourceVersion: string) => ({
  resourceVersion,
  semanticHash: `semantic-${resourceVersion}`,
  catalogVersion: "catalog-1",
  actionSchemaDigest: "actions-1",
  referenceSnapshotHash: "refs-1",
  environmentBindingVersion: "env-1",
});

const scope = (overrides: Partial<WorkflowTransportArtifactScope> = {}): WorkflowTransportArtifactScope => ({
  workflowKey,
  committedRevision: revision("rv-1"),
  receivedAt: "2026-07-21T08:00:00Z",
  ...overrides,
});

function envelope(structuredContent: unknown, isError = false) {
  return {
    schemaVersion: "aiops.workflow.edit.v2",
    isError,
    content: [{ type: "text", text: "typed workflow result" }],
    structuredContent,
  };
}

function previewArtifact(overrides: Record<string, unknown> = {}): AiopsTransportAgentUiArtifact {
  const changeSet = {
    schemaVersion: "aiops.workflow.edit.v2",
    id: "change-2",
    workflowKey,
    candidateId: "candidate-a",
    candidateRevision: 2,
    actor: "agent",
    intent: "更新检查节点",
    base: revision("rv-1"),
    operations: [{ op: "update_node", nodeId: "check", set: { label: "容量检查" } }],
    previewHash: "preview-2",
    validation: { basis: basis("rv-1"), valid: true, diagnostics: [] },
    state: "previewed",
    createdAt: "2026-07-21T07:59:00Z",
    expiresAt: "2026-07-22T07:59:00Z",
  };
  const candidate = {
    id: "candidate-a",
    workflowKey,
    currentChangeSetId: "change-2",
    currentRevision: 2,
    currentPreviewHash: "preview-2",
    state: "ready",
    expiresAt: "2026-07-22T07:59:00Z",
  };
  return {
    id: "artifact-preview-2",
    type: "workflow_patch_preview",
    renderer: "workflow.change.preview.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    payload: envelope({ kind: "preview", changeSet, candidate, diff: { touchedPaths: ["nodes/check/label"] } }),
    ...overrides,
  };
}

function directPreviewResult(): Record<string, unknown> {
  const artifact = previewArtifact();
  const structured = structuredClone((artifact.payload as Record<string, unknown>).structuredContent) as Record<string, unknown>;
  const changeSet = structured.changeSet as Record<string, unknown>;
  changeSet.state = "ready";
  const validation = changeSet.validation as Record<string, unknown>;
  return {
    kind: "preview",
    schemaVersion: "aiops.workflow.edit.v2",
    changeSet,
    candidate: structured.candidate,
    candidateRevision: 2,
    previewHash: "preview-2",
    validationBasis: basis("rv-1"),
    // Go encodes the top-level copied empty slice as null while the canonical
    // ChangeSet validation owns the non-null diagnostic array.
    diagnostics: null,
    risk: {
      level: "low", operationCount: 1, addedNodeCount: 0, deletedNodeCount: 0,
      deletesExisting: false, requiresApproval: false,
    },
    diff: {
      graph: { addedNodeIds: [], updatedNodeIds: ["check"], deletedNodeIds: [], addedEdgeIds: [], updatedEdgeIds: [], deletedEdgeIds: [] },
      inverse: { operations: [{ op: "update_node", nodeId: "check", set: { label: "旧名称" } }] },
      rebaseBase: {},
      rebaseInvariantHash: "rebase-2",
    },
    rebased: false,
  };
}

function receiptArtifact(): AiopsTransportAgentUiArtifact {
  return {
    id: "artifact-receipt-2",
    type: "workflow_patch_result",
    renderer: "workflow.change.receipt.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    payload: envelope({
      schemaVersion: "aiops.workflow.edit.v2",
      kind: "receipt",
      transactionId: "transaction-2",
      changeSetId: "change-2",
      workflowKey,
      status: "applied",
      revisionBefore: revision("rv-1"),
      revisionAfter: revision("rv-2"),
      delta: {
        addedNodeIds: [], updatedNodeIds: ["check"], deletedNodeIds: [],
        addedEdgeIds: [], updatedEdgeIds: [], deletedEdgeIds: [],
      },
      validation: { basis: basis("rv-2"), valid: true, diagnostics: [] },
      auditId: "audit-2",
    }),
  };
}

function validationArtifact(): AiopsTransportAgentUiArtifact {
  return {
    id: "artifact-validation-1",
    type: "workflow_patch_validation",
    renderer: "workflow.change.validation.v2",
    schemaVersion: "aiops.workflow.edit.v2",
    payload: envelope({
      kind: "validation",
      basis: basis("rv-1"),
      valid: true,
      diagnostics: [{ code: "WARN_TIMEOUT", severity: "warning", message: "timeout is high", nodeId: "check" }],
    }),
  };
}

describe("workflowCandidateArtifact", () => {
  it("strictly parses a direct WorkflowEditPreviewResult into candidate truth", () => {
    const parsed = parseWorkflowPreviewResult(directPreviewResult(), scope());

    expect(parsed).toMatchObject({
      candidate: { id: "candidate-a", currentRevision: 2, currentPreviewHash: "preview-2", state: "ready" },
      changeSet: {
        id: "change-2", candidateRevision: 2, previewHash: "preview-2",
        base: revision("rv-1"), state: "previewed",
      },
    });
  });

  it("accepts a semantic Undo preview and normalizes server-owned inverse operations", () => {
    const preview = directPreviewResult();
    const changeSet = preview.changeSet as Record<string, unknown>;
    const validation = changeSet.validation as Record<string, unknown>;
    const candidateBasis = { ...basis("rv-1"), semanticHash: "semantic-restored-candidate" };
    validation.basis = candidateBasis;
    preview.validationBasis = candidateBasis;
    changeSet.operations = [
      { op: "delete_edge", edgeId: "start-python-next" },
      {
        op: "add_node",
        node: {
          id: "shell", type: "action", position: { x: 425, y: 180 },
          stepName: "", stepId: "shell", handlerName: "", parentId: "", label: "Shell",
          step: { name: "shell", action: "script.shell" },
        },
      },
      {
        op: "add_edge",
        edge: { id: "start-shell-next", source: "start", sourcePort: "next", target: "shell", targetPort: "in", kind: "next", condition: "" },
      },
    ];

    const parsed = parseWorkflowPreviewResult(preview, scope());

    expect(parsed?.changeSet.validation.basis.semanticHash).toBe("semantic-restored-candidate");
    expect(parsed?.changeSet.operations).toContainEqual(expect.objectContaining({ op: "add_node", clientRef: "server:shell" }));
  });

  it("accepts an authoritative Undo preview whose restored edge has omitted ports encoded as empty strings", () => {
    const preview = directPreviewResult();
    const changeSet = preview.changeSet as Record<string, unknown>;
    const validation = changeSet.validation as Record<string, unknown>;
    const candidateBasis = { ...basis("rv-1"), semanticHash: "semantic-restored-candidate" };
    validation.basis = candidateBasis;
    preview.validationBasis = candidateBasis;
    changeSet.operations = [{
      op: "add_edge",
      edge: {
        id: "start-action-next",
        source: "start",
        sourcePort: "",
        target: "action",
        targetPort: "",
        kind: "next",
        condition: "",
      },
    }];

    const parsed = parseWorkflowPreviewResult(preview, scope());

    expect(parsed?.changeSet.operations).toEqual([{
      op: "add_edge",
      edge: { id: "start-action-next", source: "start", target: "action", kind: "next" },
    }]);
  });

  it("fails closed for stale, conflicting, expanded, or internally inconsistent direct previews", () => {
    const stale = directPreviewResult();
    (stale.changeSet as Record<string, unknown>).base = revision("rv-0");
    expect(parseWorkflowPreviewResult(stale, scope())).toBeUndefined();

    const conflicting = directPreviewResult();
    (conflicting.candidate as Record<string, unknown>).workflowKey = { tenantId: "tenant-a", workflowId: "workflow-b" };
    expect(parseWorkflowPreviewResult(conflicting, scope())).toBeUndefined();

    const mismatched = directPreviewResult();
    mismatched.previewHash = "forged-preview";
    expect(parseWorkflowPreviewResult(mismatched, scope())).toBeUndefined();

    const expanded = directPreviewResult();
    expanded.graph = { nodes: [{ id: "forged" }] };
    expect(parseWorkflowPreviewResult(expanded, scope())).toBeUndefined();

    const extraRisk = directPreviewResult();
    (extraRisk.risk as Record<string, unknown>).approvalToken = "must-not-be-accepted";
    expect(parseWorkflowPreviewResult(extraRisk, scope())).toBeUndefined();
  });

  it("strictly converts the canonical ChangeSetPreview envelope into one candidate fact", () => {
    const parsed = parseWorkflowTransportArtifact(previewArtifact(), scope());

    expect(parsed).toMatchObject({
      kind: "candidate",
      artifactId: "artifact-preview-2",
      fact: {
        candidate: { id: "candidate-a", currentRevision: 2, currentPreviewHash: "preview-2" },
        changeSet: { id: "change-2", candidateId: "candidate-a", candidateRevision: 2, previewHash: "preview-2" },
      },
    });
    expect(parsed && workflowTransportArtifactIdentity(parsed)).toBe("candidate:candidate-a:2:change-2:preview-2");
  });

  it("fails closed for cross-workflow, stale, mismatched identity, and unknown-operation previews", () => {
    const valid = previewArtifact();
    const payload = valid.payload as Record<string, unknown>;
    const structured = payload.structuredContent as Record<string, unknown>;

    expect(parseWorkflowTransportArtifact(valid, scope({ workflowKey: { tenantId: "tenant-a", workflowId: "workflow-b" } }))).toBeUndefined();

    const stale = structuredClone(valid);
    ((stale.payload!.structuredContent as Record<string, unknown>).changeSet as Record<string, unknown>).base = revision("rv-0");
    expect(parseWorkflowTransportArtifact(stale, scope())).toBeUndefined();

    const mismatched = structuredClone(valid);
    ((mismatched.payload!.structuredContent as Record<string, unknown>).candidate as Record<string, unknown>).currentPreviewHash = "forged-preview";
    expect(parseWorkflowTransportArtifact(mismatched, scope())).toBeUndefined();

    const forgedOperation = structuredClone(valid);
    ((forgedOperation.payload!.structuredContent as Record<string, unknown>).changeSet as Record<string, unknown>).operations = [{ op: "run_shell", command: "rm -rf /" }];
    expect(parseWorkflowTransportArtifact(forgedOperation, scope())).toBeUndefined();

    const inconsistent = structuredClone(valid);
    inconsistent.payload!.isError = true;
    expect(parseWorkflowTransportArtifact(inconsistent, scope())).toBeUndefined();

    expect(structured.kind).toBe("preview");
  });

  it("never promotes prose, a direct payload, or a forged renderer/schema to workspace truth", () => {
    const direct = previewArtifact();
    direct.payload = (direct.payload?.structuredContent || {}) as Record<string, unknown>;
    expect(parseWorkflowTransportArtifact(direct, scope())).toBeUndefined();

    expect(parseWorkflowTransportArtifact(previewArtifact({ renderer: "generic.json" }), scope())).toBeUndefined();
    expect(parseWorkflowTransportArtifact(previewArtifact({ schemaVersion: "aiops.workflow.edit.v99" }), scope())).toBeUndefined();
    expect(parseWorkflowTransportArtifact({
      id: "prose",
      type: "markdown",
      renderer: "workflow.change.preview.v2",
      schemaVersion: "aiops.workflow.edit.v2",
      payload: envelope((previewArtifact().payload as Record<string, unknown>).structuredContent),
    }, scope())).toBeUndefined();
  });

  it("converts canonical receipt and validation envelopes while rejecting stale revision facts", () => {
    const receipt = parseWorkflowTransportArtifact(receiptArtifact(), scope());
    const validation = parseWorkflowTransportArtifact(validationArtifact(), scope());

    expect(receipt).toMatchObject({
      kind: "receipt",
      fact: { transactionId: "transaction-2", changeSetId: "change-2", status: "completed", receivedAt: "2026-07-21T08:00:00Z" },
    });
    expect(validation).toMatchObject({
      kind: "validation",
      fact: { artifactId: "artifact-validation-1", status: "valid_with_warnings", issuedAt: "2026-07-21T08:00:00Z" },
    });
    expect(parseWorkflowTransportArtifact(receiptArtifact(), scope({ committedRevision: revision("rv-3") }))).toBeUndefined();
    expect(parseWorkflowTransportArtifact(validationArtifact(), scope({ committedRevision: revision("rv-2") }))).toBeUndefined();
  });
});
