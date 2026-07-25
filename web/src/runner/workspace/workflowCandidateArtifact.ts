import type { AiopsTransportAgentUiArtifact } from "@/transport/aiopsTransportTypes";
import {
  WORKFLOW_CONTRACT_SCHEMA_VERSION,
  parseWorkflowPatchRequestV2,
  type ValidationBasis,
  type WorkflowCandidateV2,
  type WorkflowChangeSetV2,
  type WorkflowJsonValue,
  type WorkflowKey,
  type WorkflowRevision,
  type WorkflowValidationResult,
} from "./workflowMcpContracts";
import type {
  WorkflowCandidateFact,
  WorkflowReceiptFact,
  WorkflowValidationArtifactFact,
} from "./workflowWorkspaceTypes";

export type WorkflowTransportArtifactScope = {
  workflowKey: WorkflowKey;
  committedRevision: WorkflowRevision;
  receivedAt: string;
};

export type WorkflowTransportArtifactFact =
  | { kind: "candidate"; artifactId: string; fact: WorkflowCandidateFact }
  | { kind: "receipt"; artifactId: string; fact: WorkflowReceiptFact }
  | { kind: "validation"; artifactId: string; fact: WorkflowValidationArtifactFact };

type ObjectValue = Record<string, unknown>;

const CONTRACTS = {
  workflow_patch_preview: "workflow.change.preview.v2",
  workflow_patch_result: "workflow.change.receipt.v2",
  workflow_patch_validation: "workflow.change.validation.v2",
} as const;

const CHANGE_SET_STATES = ["proposed", "previewed", "approved", "applying", "applied", "rejected", "conflict", "superseded", "undone"] as const;
const RECEIPT_STATUSES = [
  "verifying", "completed", "completed_with_warnings", "configuration_required", "saved_with_issues",
  "no_effect", "conflict", "rejected", "applied",
] as const;
const SENSITIVE_FIELDS = new Set(["password", "secret", "token", "apikey", "accesskey", "privatekey"]);

export function parseWorkflowTransportArtifact(
  artifact: AiopsTransportAgentUiArtifact,
  scope: WorkflowTransportArtifactScope,
): WorkflowTransportArtifactFact | undefined {
  try {
    const artifactId = strictString(artifact.id, "artifact.id");
    const renderer = CONTRACTS[artifact.type as keyof typeof CONTRACTS];
    if (!renderer || artifact.renderer !== renderer || artifact.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) return undefined;
    if (!sameWorkflowKey(scope.workflowKey, parseWorkflowKey(scope.workflowKey, "scope.workflowKey"))) return undefined;
    const committedRevision = parseRevision(scope.committedRevision, "scope.committedRevision");
    const receivedAt = strictDate(scope.receivedAt, "scope.receivedAt");
    const envelope = artifactEnvelope(artifact);

    if (artifact.type === "workflow_patch_preview") {
      const fact = parsePreview(envelope, scope.workflowKey, committedRevision);
      return { kind: "candidate", artifactId, fact };
    }
    if (artifact.type === "workflow_patch_result") {
      const fact = parseReceipt(envelope, scope.workflowKey, committedRevision, receivedAt);
      return { kind: "receipt", artifactId, fact };
    }
    if (artifact.type === "workflow_patch_validation") {
      const validation = parseStandaloneValidation(envelope);
      if (!sameRevisionBasis(validation.basis, committedRevision)) return undefined;
      const status: WorkflowValidationArtifactFact["status"] = !validation.valid
        ? "saved_with_issues"
        : validation.diagnostics.length
          ? "valid_with_warnings"
          : "valid";
      return {
        kind: "validation",
        artifactId,
        fact: { artifactId, basis: validation.basis, status, diagnostics: validation.diagnostics, issuedAt: receivedAt },
      };
    }
    return undefined;
  } catch {
    return undefined;
  }
}

/**
 * Converts the strict HTTP WorkflowEditPreviewResult returned by direct canvas
 * commands (Undo/Redo and Candidate revision) into the same candidate fact as
 * MCP transport artifacts. Unknown fields and stale/cross-workflow results are
 * rejected instead of becoming workspace truth.
 */
export function parseWorkflowPreviewResult(
  value: unknown,
  scope: WorkflowTransportArtifactScope,
): WorkflowCandidateFact | undefined {
  try {
    const source = strictObject(value, "workflowPreviewResult");
    exactKeys(source, [
      "kind", "schemaVersion", "changeSet", "candidate", "candidateRevision", "previewHash",
      "validationBasis", "diagnostics", "risk", "diff", "rebased",
    ], "workflowPreviewResult");
    if (source.kind !== "preview" || source.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) {
      throw new TypeError("unsupported workflow preview result");
    }
    if (typeof source.rebased !== "boolean") throw new TypeError("workflowPreviewResult.rebased must be boolean");
    parseDirectPreviewRisk(source.risk);
    parseDirectPreviewDiff(source.diff);

    const validation = strictObject(strictObject(source.changeSet, "workflowPreviewResult.changeSet").validation, "workflowPreviewResult.changeSet.validation");
    if (typeof validation.valid !== "boolean") throw new TypeError("workflowPreviewResult validation is invalid");
    const fact = parsePreview({
      isError: !validation.valid,
      structuredContent: {
        kind: source.kind,
        changeSet: source.changeSet,
        candidate: source.candidate,
        diff: source.diff,
      },
    }, parseWorkflowKey(scope.workflowKey, "scope.workflowKey"), parseRevision(scope.committedRevision, "scope.committedRevision"));

    const candidateRevision = positiveInteger(source.candidateRevision, "workflowPreviewResult.candidateRevision");
    const previewHash = strictString(source.previewHash, "workflowPreviewResult.previewHash");
    const validationBasis = parseBasis(source.validationBasis, "workflowPreviewResult.validationBasis");
    const directDiagnostics = source.diagnostics === null ? [] : source.diagnostics;
    if (!Array.isArray(directDiagnostics)) throw new TypeError("workflowPreviewResult.diagnostics must be an array or null");
    parseJsonValue(directDiagnostics, "workflowPreviewResult.diagnostics");
    if (candidateRevision !== fact.candidate.currentRevision
      || candidateRevision !== fact.changeSet.candidateRevision
      || previewHash !== fact.candidate.currentPreviewHash
      || previewHash !== fact.changeSet.previewHash
      || stableJson(validationBasis) !== stableJson(fact.changeSet.validation.basis)
      || stableJson(directDiagnostics) !== stableJson(fact.changeSet.validation.diagnostics)) {
      throw new TypeError("workflow preview result identity mismatch");
    }
    return fact;
  } catch {
    return undefined;
  }
}

export function workflowTransportArtifactIdentity(value: WorkflowTransportArtifactFact): string {
  if (value.kind === "candidate") {
    const { candidate, changeSet } = value.fact;
    return `candidate:${candidate.id}:${candidate.currentRevision}:${changeSet.id}:${changeSet.previewHash}`;
  }
  if (value.kind === "receipt") return `receipt:${value.fact.transactionId}:${value.fact.status}`;
  return `validation:${value.fact.artifactId}:${value.fact.basis.resourceVersion}:${value.fact.basis.semanticHash}:${value.fact.status}`;
}

function artifactEnvelope(artifact: AiopsTransportAgentUiArtifact): ObjectValue {
  if (artifact.payload !== undefined && artifact.inlineData !== undefined) {
    if (stableJson(artifact.payload) !== stableJson(artifact.inlineData)) throw new TypeError("conflicting artifact envelopes");
  }
  const envelope = strictObject(artifact.payload ?? artifact.inlineData, "artifact.envelope");
  exactKeys(envelope, ["schemaVersion", "isError", "content", "structuredContent", "_meta"], "artifact.envelope");
  if (envelope.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) throw new TypeError("unsupported envelope schema");
  if (typeof envelope.isError !== "boolean") throw new TypeError("envelope isError must be boolean");
  if (!Array.isArray(envelope.content) || envelope.content.length === 0) throw new TypeError("envelope content is required");
  envelope.content.forEach((part, index) => {
    const item = strictObject(part, `artifact.envelope.content[${index}]`);
    exactKeys(item, ["type", "text"], `artifact.envelope.content[${index}]`);
    if (item.type !== "text") throw new TypeError("only text summaries are supported");
    strictString(item.text, `artifact.envelope.content[${index}].text`);
  });
  if (envelope._meta !== undefined) parseJsonValue(envelope._meta, "artifact.envelope._meta");
  return envelope;
}

function parsePreview(envelope: ObjectValue, expectedKey: WorkflowKey, committed: WorkflowRevision): WorkflowCandidateFact {
  const structured = strictObject(envelope.structuredContent, "preview");
  exactKeys(structured, ["kind", "changeSet", "candidate", "diff"], "preview");
  if (structured.kind !== "preview") throw new TypeError("preview kind is required");
  if (typeof envelope.isError !== "boolean") throw new TypeError("preview envelope error flag is required");
  parseJsonValue(structured.diff, "preview.diff");

  const source = strictObject(structured.changeSet, "preview.changeSet");
  exactKeys(source, [
    "schemaVersion", "id", "workflowKey", "candidateId", "candidateRevision", "parentChangeSetId", "actor", "intent",
    "base", "operations", "previewHash", "validation", "state", "createdAt", "expiresAt",
  ], "preview.changeSet");
  if (source.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) throw new TypeError("unsupported change set schema");
  const key = parseWorkflowKey(source.workflowKey, "preview.changeSet.workflowKey");
  const base = parseRevision(source.base, "preview.changeSet.base");
  if (!sameWorkflowKey(key, expectedKey) || !sameRevision(base, committed)) throw new TypeError("stale or cross-workflow preview");
  const validation = parseValidation(source.validation, "preview.changeSet.validation");
  if (envelope.isError !== !validation.valid) throw new TypeError("preview result error flag mismatch");
  // A semantic preview validates the projected candidate graph, so its
  // semanticHash is expected to differ from the committed base. It must still
  // be bound to the same immutable resourceVersion; top-level and nested basis
  // identity is checked again by parseWorkflowPreviewResult.
  if (validation.basis.resourceVersion !== base.resourceVersion) throw new TypeError("preview validation basis mismatch");
  if (!Array.isArray(source.operations) || source.operations.length === 0 || source.operations.length > 500) throw new TypeError("invalid preview operations");
  const actor = oneOf(source.actor, ["user", "agent", "system"] as const, "preview.changeSet.actor");
  const sourceState = oneOf(source.state, [...CHANGE_SET_STATES, "ready"] as const, "preview.changeSet.state");
  const state: WorkflowChangeSetV2["state"] = sourceState === "ready" ? "previewed" : sourceState;
  const candidateRevision = positiveInteger(source.candidateRevision, "preview.changeSet.candidateRevision");
  const intent = strictString(source.intent, "preview.changeSet.intent");
  const parsedRequest = parseWorkflowPatchRequestV2({
    schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION,
    workflowKey: key,
    expected: base,
    validationBasis: validation.basis,
    intent,
    operations: normalizeServerPreviewOperations(source.operations),
  });
  const changeSet: WorkflowChangeSetV2 = {
    schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION,
    id: strictString(source.id, "preview.changeSet.id"),
    workflowKey: key,
    candidateId: strictString(source.candidateId, "preview.changeSet.candidateId"),
    candidateRevision,
    ...(source.parentChangeSetId !== undefined ? { parentChangeSetId: strictString(source.parentChangeSetId, "preview.changeSet.parentChangeSetId") } : {}),
    actor,
    intent,
    base,
    operations: parsedRequest.operations,
    previewHash: strictString(source.previewHash, "preview.changeSet.previewHash"),
    validation,
    state,
    createdAt: strictDate(source.createdAt, "preview.changeSet.createdAt"),
    expiresAt: strictDate(source.expiresAt, "preview.changeSet.expiresAt"),
  };
  const candidate = parseCandidate(structured.candidate);
  if (!sameWorkflowKey(candidate.workflowKey, key)
    || candidate.id !== changeSet.candidateId
    || candidate.currentChangeSetId !== changeSet.id
    || candidate.currentRevision !== changeSet.candidateRevision
    || candidate.currentPreviewHash !== changeSet.previewHash) {
    throw new TypeError("candidate and change set identity mismatch");
  }
  return { candidate, changeSet };
}

function normalizeServerPreviewOperations(value: unknown): unknown {
  if (!Array.isArray(value)) return value;
  return value.map((rawOperation) => {
    if (!rawOperation || typeof rawOperation !== "object" || Array.isArray(rawOperation)) return rawOperation;
    const operation = { ...(rawOperation as ObjectValue) };
    if (operation.op === "add_node" && operation.node && typeof operation.node === "object" && !Array.isArray(operation.node)) {
      const node = { ...(operation.node as ObjectValue) };
      const nodeId = typeof node.id === "string" ? node.id.trim() : "";
      // Runner-owned inverse operations restore a stable node ID and may omit
      // a clientRef because no temporary reference allocation is needed. The
      // browser contract uses a deterministic projection-only ref.
      if (operation.clientRef === undefined && nodeId) operation.clientRef = `server:${nodeId}`;
      for (const key of ["stepName", "stepId", "handlerName", "parentId", "label"] as const) {
        if (node[key] === "") delete node[key];
      }
      operation.node = node;
    }
    if (operation.op === "add_edge" && operation.edge && typeof operation.edge === "object" && !Array.isArray(operation.edge)) {
      const edge = { ...(operation.edge as ObjectValue) };
      // The Runner persistence model encodes omitted optional edge selectors
      // as empty strings when it builds an authoritative Undo/Redo inverse.
      // The browser request contract represents omission by removing them.
      for (const key of ["sourcePort", "targetPort", "condition"] as const) {
        if (edge[key] === "") delete edge[key];
      }
      operation.edge = edge;
    }
    return operation;
  });
}

function parseDirectPreviewRisk(value: unknown): void {
  const source = strictObject(value, "workflowPreviewResult.risk");
  exactKeys(source, ["level", "operationCount", "addedNodeCount", "deletedNodeCount", "deletesExisting", "requiresApproval"], "workflowPreviewResult.risk");
  strictString(source.level, "workflowPreviewResult.risk.level");
  for (const field of ["operationCount", "addedNodeCount", "deletedNodeCount"] as const) {
    if (typeof source[field] !== "number" || !Number.isSafeInteger(source[field]) || source[field] < 0) {
      throw new TypeError(`workflowPreviewResult.risk.${field} must be a non-negative integer`);
    }
  }
  if (typeof source.deletesExisting !== "boolean" || typeof source.requiresApproval !== "boolean") {
    throw new TypeError("workflowPreviewResult.risk flags must be boolean");
  }
}

function parseDirectPreviewDiff(value: unknown): void {
  const source = strictObject(value, "workflowPreviewResult.diff");
  exactKeys(source, ["graph", "inverse", "rebaseBase", "rebaseInvariantHash"], "workflowPreviewResult.diff");
  parseJsonValue(source.graph, "workflowPreviewResult.diff.graph");
  parseJsonValue(source.inverse, "workflowPreviewResult.diff.inverse");
  parseJsonValue(source.rebaseBase, "workflowPreviewResult.diff.rebaseBase");
  strictString(source.rebaseInvariantHash, "workflowPreviewResult.diff.rebaseInvariantHash");
}

function parseCandidate(value: unknown): WorkflowCandidateV2 {
  const source = strictObject(value, "preview.candidate");
  exactKeys(source, ["id", "workflowKey", "currentChangeSetId", "currentRevision", "currentPreviewHash", "state", "expiresAt"], "preview.candidate");
  return {
    id: strictString(source.id, "preview.candidate.id"),
    workflowKey: parseWorkflowKey(source.workflowKey, "preview.candidate.workflowKey"),
    currentChangeSetId: strictString(source.currentChangeSetId, "preview.candidate.currentChangeSetId"),
    currentRevision: positiveInteger(source.currentRevision, "preview.candidate.currentRevision"),
    currentPreviewHash: strictString(source.currentPreviewHash, "preview.candidate.currentPreviewHash"),
    state: strictString(source.state, "preview.candidate.state"),
    expiresAt: strictDate(source.expiresAt, "preview.candidate.expiresAt"),
  };
}

function parseReceipt(
  envelope: ObjectValue,
  expectedKey: WorkflowKey,
  committed: WorkflowRevision,
  receivedAt: string,
): WorkflowReceiptFact {
  if (envelope.isError !== false) throw new TypeError("receipt cannot be an error result");
  const source = strictObject(envelope.structuredContent, "receipt");
  exactKeys(source, [
    "schemaVersion", "kind", "transactionId", "changeSetId", "workflowKey", "status", "revisionBefore", "revisionAfter",
    "delta", "validation", "checkpointId", "inverseTransactionId", "auditId",
  ], "receipt");
  if (source.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION || source.kind !== "receipt") throw new TypeError("invalid receipt contract");
  const key = parseWorkflowKey(source.workflowKey, "receipt.workflowKey");
  if (!sameWorkflowKey(key, expectedKey)) throw new TypeError("cross-workflow receipt");
  const status = oneOf(source.status, RECEIPT_STATUSES, "receipt.status");
  const revisionBefore = parseRevision(source.revisionBefore, "receipt.revisionBefore");
  const revisionAfter = source.revisionAfter === undefined ? undefined : parseRevision(source.revisionAfter, "receipt.revisionAfter");
  if (!sameRevision(committed, revisionBefore) && (!revisionAfter || !sameRevision(committed, revisionAfter))) throw new TypeError("stale receipt");
  if (!["no_effect", "conflict", "rejected"].includes(status) && !revisionAfter) throw new TypeError("mutation receipt requires revisionAfter");
  const validation = parseValidation(source.validation, "receipt.validation");
  if (revisionAfter && !sameRevisionBasis(validation.basis, revisionAfter)) throw new TypeError("receipt validation basis mismatch");
  parseDelta(source.delta);
  strictString(source.auditId, "receipt.auditId");
  if (source.checkpointId !== undefined) strictString(source.checkpointId, "receipt.checkpointId");
  if (source.inverseTransactionId !== undefined) strictString(source.inverseTransactionId, "receipt.inverseTransactionId");
  return {
    transactionId: strictString(source.transactionId, "receipt.transactionId"),
    changeSetId: strictString(source.changeSetId, "receipt.changeSetId"),
    workflowKey: key,
    status: status === "applied" ? "completed" : status,
    revisionBefore,
    ...(revisionAfter ? { revisionAfter } : {}),
    validation,
    receivedAt,
  };
}

function parseStandaloneValidation(envelope: ObjectValue): WorkflowValidationResult {
  const validation = parseValidation(envelope.structuredContent, "validation");
  if (envelope.isError !== !validation.valid) throw new TypeError("validation result error flag mismatch");
  return validation;
}

function parseValidation(value: unknown, path: string): WorkflowValidationResult {
  const source = strictObject(value, path);
  exactKeys(source, ["kind", "basis", "valid", "diagnostics"], path);
  if (source.kind !== undefined && source.kind !== "validation") throw new TypeError(`${path}.kind is invalid`);
  if (typeof source.valid !== "boolean" || !Array.isArray(source.diagnostics)) throw new TypeError(`${path} is invalid`);
  const diagnostics = source.diagnostics.map((item, index) => {
    const diagnosticPath = `${path}.diagnostics[${index}]`;
    const diagnostic = strictObject(item, diagnosticPath);
    exactKeys(diagnostic, ["code", "severity", "message", "nodeId", "edgeId", "field", "suggestion", "details"], diagnosticPath);
    return {
      code: strictString(diagnostic.code, `${diagnosticPath}.code`),
      severity: strictString(diagnostic.severity, `${diagnosticPath}.severity`),
      message: strictString(diagnostic.message, `${diagnosticPath}.message`),
      ...(diagnostic.nodeId !== undefined ? { nodeId: strictString(diagnostic.nodeId, `${diagnosticPath}.nodeId`) } : {}),
      ...(diagnostic.edgeId !== undefined ? { edgeId: strictString(diagnostic.edgeId, `${diagnosticPath}.edgeId`) } : {}),
      ...(diagnostic.field !== undefined ? { field: strictString(diagnostic.field, `${diagnosticPath}.field`) } : {}),
      ...(diagnostic.suggestion !== undefined ? { suggestion: strictString(diagnostic.suggestion, `${diagnosticPath}.suggestion`) } : {}),
      ...(diagnostic.details !== undefined ? { details: parseJsonValue(diagnostic.details, `${diagnosticPath}.details`) } : {}),
    };
  });
  return { basis: parseBasis(source.basis, `${path}.basis`), valid: source.valid, diagnostics };
}

function parseBasis(value: unknown, path: string): ValidationBasis {
  const source = strictObject(value, path);
  const keys = ["resourceVersion", "semanticHash", "catalogVersion", "actionSchemaDigest", "referenceSnapshotHash", "environmentBindingVersion"] as const;
  exactKeys(source, keys, path);
  return Object.fromEntries(keys.map((key) => [key, strictString(source[key], `${path}.${key}`)])) as ValidationBasis;
}

function parseDelta(value: unknown): void {
  const source = strictObject(value, "receipt.delta");
  const keys = ["addedNodeIds", "updatedNodeIds", "deletedNodeIds", "addedEdgeIds", "updatedEdgeIds", "deletedEdgeIds"] as const;
  exactKeys(source, keys, "receipt.delta");
  for (const key of keys) {
    if (!Array.isArray(source[key])) throw new TypeError(`receipt.delta.${key} must be an array`);
    source[key].forEach((item, index) => strictString(item, `receipt.delta.${key}[${index}]`));
  }
}

function parseWorkflowKey(value: unknown, path: string): WorkflowKey {
  const source = strictObject(value, path);
  exactKeys(source, ["tenantId", "workflowId"], path);
  const canonical = (item: unknown, itemPath: string) => {
    const result = strictString(item, itemPath);
    if (result === "." || result === ".." || /[\\/\0]/.test(result)) throw new TypeError(`${itemPath} is not canonical`);
    return result;
  };
  return { tenantId: canonical(source.tenantId, `${path}.tenantId`), workflowId: canonical(source.workflowId, `${path}.workflowId`) };
}

function parseRevision(value: unknown, path: string): WorkflowRevision {
  const source = strictObject(value, path);
  exactKeys(source, ["resourceVersion", "semanticHash", "layoutHash"], path);
  return {
    resourceVersion: strictString(source.resourceVersion, `${path}.resourceVersion`),
    semanticHash: strictString(source.semanticHash, `${path}.semanticHash`),
    layoutHash: strictString(source.layoutHash, `${path}.layoutHash`),
  };
}

function parseJsonValue(value: unknown, path: string): WorkflowJsonValue {
  if (value === null || typeof value === "string" || typeof value === "boolean") return value;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (Array.isArray(value)) return value.map((item, index) => parseJsonValue(item, `${path}[${index}]`));
  const source = strictObject(value, path);
  return Object.fromEntries(Object.entries(source).map(([key, item]) => {
    const normalized = key.toLowerCase().replace(/[_-]/g, "");
    if (SENSITIVE_FIELDS.has(normalized)) throw new TypeError(`${path}.${key} must use secretRef`);
    return [key, parseJsonValue(item, `${path}.${key}`)];
  }));
}

function strictObject(value: unknown, path: string): ObjectValue {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${path} must be an object`);
  return value as ObjectValue;
}

function exactKeys(source: ObjectValue, allowed: readonly string[], path: string): void {
  const unknown = Object.keys(source).find((key) => !allowed.includes(key));
  if (unknown) throw new TypeError(`${path}.${unknown} is not supported`);
}

function strictString(value: unknown, path: string): string {
  if (typeof value !== "string" || !value || value !== value.trim()) throw new TypeError(`${path} must be a canonical string`);
  return value;
}

function strictDate(value: unknown, path: string): string {
  const result = strictString(value, path);
  if (!Number.isFinite(Date.parse(result))) throw new TypeError(`${path} must be an ISO date`);
  return result;
}

function positiveInteger(value: unknown, path: string): number {
  if (typeof value !== "number" || !Number.isSafeInteger(value) || value < 1) throw new TypeError(`${path} must be a positive integer`);
  return value;
}

function oneOf<const T extends readonly string[]>(value: unknown, allowed: T, path: string): T[number] {
  if (typeof value !== "string" || !allowed.includes(value)) throw new TypeError(`${path} is unsupported`);
  return value as T[number];
}

function sameWorkflowKey(left: WorkflowKey, right: WorkflowKey): boolean {
  return left.tenantId === right.tenantId && left.workflowId === right.workflowId;
}

function sameRevision(left: WorkflowRevision, right: WorkflowRevision): boolean {
  return left.resourceVersion === right.resourceVersion && left.semanticHash === right.semanticHash && left.layoutHash === right.layoutHash;
}

function sameRevisionBasis(value: ValidationBasis, revision: WorkflowRevision): boolean {
  return value.resourceVersion === revision.resourceVersion && value.semanticHash === revision.semanticHash;
}

function stableJson(value: unknown): string {
  if (Array.isArray(value)) return `[${value.map(stableJson).join(",")}]`;
  if (value && typeof value === "object") {
    return `{${Object.entries(value as ObjectValue).sort(([left], [right]) => left.localeCompare(right)).map(([key, item]) => `${JSON.stringify(key)}:${stableJson(item)}`).join(",")}}`;
  }
  return JSON.stringify(value);
}
