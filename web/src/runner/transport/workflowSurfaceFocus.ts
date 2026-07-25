import type { WorkflowOperation } from "@/runner/workspace/workflowMcpContracts";

export const WORKFLOW_SURFACE_FOCUS_EVENT = "aiops:workflow-surface-focus";
export const WORKFLOW_SURFACE_FOCUS_VERSION = "aiops.workflow.surface-focus.v1" as const;

export type WorkflowSurfaceFocusTarget =
  | { nodeId: string; edgeId?: never }
  | { nodeId?: never; edgeId: string };

export type WorkflowSurfaceFocusCandidate = {
  candidateId: string;
  changeSetId: string;
  previewHash: string;
  expectedRevision: string;
};

export type WorkflowSurfaceFocusDetail = {
  schemaVersion: typeof WORKFLOW_SURFACE_FOCUS_VERSION;
  requestId: string;
  source: "chat" | "review" | "canvas";
  workflowId: string;
  target: WorkflowSurfaceFocusTarget;
  candidate?: WorkflowSurfaceFocusCandidate;
  operationIndex?: number;
  artifactId?: string;
  turnId?: string;
};

let focusSequence = 0;

export function createWorkflowSurfaceFocusDetail(
  detail: Omit<WorkflowSurfaceFocusDetail, "schemaVersion" | "requestId"> & { requestId?: string },
): WorkflowSurfaceFocusDetail {
  focusSequence += 1;
  return {
    ...detail,
    schemaVersion: WORKFLOW_SURFACE_FOCUS_VERSION,
    requestId: detail.requestId || `${detail.source}-focus-${focusSequence}`,
  };
}

export function dispatchWorkflowSurfaceFocus(detail: WorkflowSurfaceFocusDetail) {
  if (typeof window === "undefined") return;
  window.dispatchEvent(new CustomEvent<WorkflowSurfaceFocusDetail>(WORKFLOW_SURFACE_FOCUS_EVENT, { detail }));
}

export function readWorkflowSurfaceFocusDetail(event: Event): WorkflowSurfaceFocusDetail | undefined {
  const detail = (event as CustomEvent<unknown>).detail;
  if (!detail || typeof detail !== "object" || Array.isArray(detail)) return undefined;
  const value = detail as Record<string, unknown>;
  if (value.schemaVersion !== WORKFLOW_SURFACE_FOCUS_VERSION) return undefined;
  if (!canonicalString(value.requestId) || !canonicalString(value.workflowId)) return undefined;
  if (value.source !== "chat" && value.source !== "review" && value.source !== "canvas") return undefined;
  const target = readTarget(value.target);
  if (!target) return undefined;
  const operationIndex = value.operationIndex;
  if (operationIndex !== undefined && (!Number.isInteger(operationIndex) || Number(operationIndex) < 0)) return undefined;
  const candidate = value.candidate === undefined ? undefined : readCandidate(value.candidate);
  if (value.candidate !== undefined && !candidate) return undefined;
  if (value.artifactId !== undefined && !canonicalString(value.artifactId)) return undefined;
  if (value.turnId !== undefined && !canonicalString(value.turnId)) return undefined;
  return {
    schemaVersion: WORKFLOW_SURFACE_FOCUS_VERSION,
    requestId: value.requestId,
    source: value.source,
    workflowId: value.workflowId,
    target,
    ...(candidate ? { candidate } : {}),
    ...(operationIndex !== undefined ? { operationIndex: Number(operationIndex) } : {}),
    ...(canonicalString(value.artifactId) ? { artifactId: value.artifactId } : {}),
    ...(canonicalString(value.turnId) ? { turnId: value.turnId } : {}),
  };
}

export function resolveWorkflowOperationIndex(
  operations: WorkflowOperation[],
  target: WorkflowSurfaceFocusTarget,
  preferredIndex?: number,
): number | undefined {
  if (preferredIndex !== undefined && operationMatchesTarget(operations[preferredIndex], target)) return preferredIndex;
  const index = operations.findIndex((operation) => operationMatchesTarget(operation, target));
  return index >= 0 ? index : undefined;
}

export function workflowOperationTarget(operation: WorkflowOperation | undefined): WorkflowSurfaceFocusTarget | undefined {
  if (!operation) return undefined;
  if (operation.op === "update_node" || operation.op === "delete_node") return { nodeId: operation.nodeId };
  if (operation.op === "add_node") {
    const nodeId = canonicalOptionalString(operation.node.id) || canonicalOptionalString(operation.clientRef);
    return nodeId ? { nodeId } : undefined;
  }
  if (operation.op === "update_edge" || operation.op === "delete_edge") return { edgeId: operation.edgeId };
  if (operation.op === "add_edge") {
    const edgeId = canonicalOptionalString(operation.edge.id) || canonicalOptionalString(operation.clientRef);
    return edgeId ? { edgeId } : undefined;
  }
  if (operation.op === "update_layout" && operation.layout.nodes.length === 1) {
    return { nodeId: operation.layout.nodes[0].nodeId };
  }
  return undefined;
}

function operationMatchesTarget(operation: WorkflowOperation | undefined, target: WorkflowSurfaceFocusTarget) {
  if (!operation) return false;
  const direct = workflowOperationTarget(operation);
  if (direct) return sameTarget(direct, target);
  if (operation.op === "update_layout" && "nodeId" in target) {
    return operation.layout.nodes.some((node) => node.nodeId === target.nodeId);
  }
  return false;
}

function sameTarget(left: WorkflowSurfaceFocusTarget, right: WorkflowSurfaceFocusTarget) {
  return ("nodeId" in left && "nodeId" in right && left.nodeId === right.nodeId)
    || ("edgeId" in left && "edgeId" in right && left.edgeId === right.edgeId);
}

function readTarget(value: unknown): WorkflowSurfaceFocusTarget | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const target = value as Record<string, unknown>;
  const nodeId = canonicalOptionalString(target.nodeId);
  const edgeId = canonicalOptionalString(target.edgeId);
  if (Boolean(nodeId) === Boolean(edgeId)) return undefined;
  return nodeId ? { nodeId } : { edgeId: edgeId! };
}

function readCandidate(value: unknown): WorkflowSurfaceFocusCandidate | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const candidate = value as Record<string, unknown>;
  if (![candidate.candidateId, candidate.changeSetId, candidate.previewHash, candidate.expectedRevision].every(canonicalString)) return undefined;
  return {
    candidateId: candidate.candidateId as string,
    changeSetId: candidate.changeSetId as string,
    previewHash: candidate.previewHash as string,
    expectedRevision: candidate.expectedRevision as string,
  };
}

function canonicalOptionalString(value: unknown): string | undefined {
  return canonicalString(value) ? value : undefined;
}

function canonicalString(value: unknown): value is string {
  return typeof value === "string" && value.length > 0 && value === value.trim();
}
