export const WORKFLOW_CONTRACT_SCHEMA_VERSION = "aiops.workflow.edit.v2" as const;

export type WorkflowKey = { tenantId: string; workflowId: string };
export type WorkflowRevision = { resourceVersion: string; semanticHash: string; layoutHash: string };
export type ValidationBasis = {
  resourceVersion: string;
  semanticHash: string;
  catalogVersion: string;
  actionSchemaDigest: string;
  referenceSnapshotHash: string;
  environmentBindingVersion: string;
};

export type WorkflowJsonValue = null | boolean | number | string | WorkflowJsonValue[] | { [key: string]: WorkflowJsonValue };
type JsonValue = WorkflowJsonValue;
type Position = { x: number; y: number };
type NodeKind = "start" | "action" | "condition" | "parallel" | "join" | "loop" | "handler" | "group" | "subflow" | "manual_approval" | "variable_aggregator" | "end";
type NodeSpec = {
  id?: string; type: NodeKind; position: Position; stepName?: string; stepId?: string; step?: JsonValue;
  handlerName?: string; handler?: JsonValue; approval?: JsonValue; condition?: JsonValue; subflow?: JsonValue;
  aggregator?: JsonValue; join?: JsonValue; loop?: JsonValue; parentId?: string; label?: string; collapsed?: boolean;
  ports?: JsonValue; inputs?: JsonValue; outputs?: JsonValue;
};
type EdgeSpec = {
  id?: string; source: string; sourcePort?: string; target: string; targetPort?: string; kind?: string; condition?: string;
};

export type WorkflowOperation =
  | { op: "add_node"; clientRef: string; node: NodeSpec }
  | { op: "update_node"; nodeId: string; set: Record<string, JsonValue> }
  | { op: "delete_node"; nodeId: string; cascadeEdges: boolean }
  | { op: "add_edge"; clientRef?: string; edge: EdgeSpec }
  | { op: "update_edge"; edgeId: string; set: Record<string, JsonValue> }
  | { op: "delete_edge"; edgeId: string }
  | { op: "update_workflow"; set: Record<string, JsonValue> }
  | { op: "update_layout"; layout: { nodes: Array<{ nodeId: string; position: Position }>; viewport?: { x: number; y: number; zoom: number } } }
  | { op: "attach_manual_binding"; manualRef: { manualId: string; candidateId?: string }; workflowDigest: string };

export type WorkflowOperationKind = WorkflowOperation["op"];
export type WorkflowActorKind = "user" | "agent" | "system";
export type WorkflowDiagnostic = { code: string; severity: string; message: string; nodeId?: string; edgeId?: string; field?: string; suggestion?: string; details?: JsonValue };
export type WorkflowValidationResult = { basis: ValidationBasis; valid: boolean; diagnostics: WorkflowDiagnostic[] };
export type WorkflowEditSessionV2 = { schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION; id: string; workflowKey: WorkflowKey; actorId: string; canvasSessionId: string; agentSessionId?: string; turnId?: string; status: string; activeCandidateId?: string; createdAt: string; updatedAt: string };
export type WorkflowChangeSetV2 = { schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION; id: string; workflowKey: WorkflowKey; candidateId: string; candidateRevision: number; parentChangeSetId?: string; actor: WorkflowActorKind; intent: string; base: WorkflowRevision; operations: WorkflowOperation[]; previewHash: string; validation: WorkflowValidationResult; state: "proposed" | "previewed" | "approved" | "applying" | "applied" | "rejected" | "conflict" | "superseded" | "undone"; createdAt: string; expiresAt: string };
export type WorkflowCandidateV2 = { id: string; workflowKey: WorkflowKey; currentChangeSetId: string; currentRevision: number; currentPreviewHash: string; state: string; expiresAt: string };
export type WorkflowEditGrantV2 = { id: string; workflowKey: WorkflowKey; actorId: string; planHash: string; baseSemanticHash: string; allowedOps: WorkflowOperationKind[]; maxOperations: number; maxAddedNodes: number; allowDeleteExisting: boolean; expiresAt: string };
export type WorkflowApplyApprovalV2 = { id: string; actorId: string; workflowKey: WorkflowKey; candidateId: string; changeSetId: string; previewHash: string; expectedRevision: string; expiresAt: string; consumedAt?: string };
export type WorkflowGraphDelta = { addedNodeIds: string[]; updatedNodeIds: string[]; deletedNodeIds: string[]; addedEdgeIds: string[]; updatedEdgeIds: string[]; deletedEdgeIds: string[] };
export type WorkflowMutationReceiptV2 = { schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION; kind: "receipt"; transactionId: string; changeSetId: string; workflowKey: WorkflowKey; status: "applied" | "no_effect" | "conflict" | "rejected"; revisionBefore: WorkflowRevision; revisionAfter?: WorkflowRevision; delta: WorkflowGraphDelta; validation: WorkflowValidationResult; checkpointId?: string; inverseTransactionId?: string; auditId: string };
export type WorkflowConflictV2 = { code: WorkflowDomainErrorCode; base: WorkflowRevision; current: WorkflowRevision; conflictingPaths: string[]; safeRebasePaths: string[]; changedBy: WorkflowActorKind };
export type WorkflowLifecycleGateV2 = { kind: string; basis: ValidationBasis; status: string; issuedAt: string };
export type WorkflowDomainEventV2 = { schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION; eventId: string; sequence: number; type: string; workflowKey: WorkflowKey; revision: WorkflowRevision; actor: WorkflowActorKind; transactionId?: string; changeSetId?: string; candidateRevision?: number; payload: JsonValue; occurredAt: string };
export type WorkflowContextV2 = { kind: string; schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION; workflowKey: WorkflowKey; revision: WorkflowRevision; graph?: JsonValue; validation: WorkflowValidationResult; resourceUri: string; selectionContextStale: boolean; nextCursor?: string };

export const WORKFLOW_DOMAIN_ERROR_CODES = ["INVALID_WORKFLOW_KEY", "STALE_REVISION", "STALE_CHANGESET", "PREVIEW_MISMATCH", "INVALID_PATCH", "UNKNOWN_ACTION", "SCHEMA_VIOLATION", "INVALID_EDGE", "WOULD_CREATE_CYCLE", "REFERENCE_LOCKED", "APPROVAL_REQUIRED", "APPROVAL_SCOPE_EXCEEDED", "DUPLICATE_TRANSACTION", "IDEMPOTENCY_KEY_REUSED", "NO_EFFECT", "MISSING_CONFIGURATION", "PUBLISH_GATE_FAILED", "VALIDATION_BASIS_STALE"] as const;
export type WorkflowDomainErrorCode = (typeof WORKFLOW_DOMAIN_ERROR_CODES)[number];
export type WorkflowDomainErrorV2 = { kind: "error"; errorCode: WorkflowDomainErrorCode; message: string; workflowKey?: WorkflowKey; currentRevision?: string; baseRevision?: string; conflictingPaths?: string[]; changedBy?: string; retryHint?: string; details?: JsonValue };

export type WorkflowPatchRequestV2 = {
  schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION;
  workflowKey: WorkflowKey;
  expected: WorkflowRevision;
  validationBasis: ValidationBasis;
  intent: string;
  operations: WorkflowOperation[];
};

export type WorkflowToolResultV2 = {
  schemaVersion: typeof WORKFLOW_CONTRACT_SCHEMA_VERSION;
  isError: boolean;
  content: Array<{ type: "text"; text: string }>;
  _meta?: JsonValue;
  structuredContent: WorkflowMutationReceiptV2 | WorkflowDomainErrorV2;
};

export type WorkflowContractErrorCode =
  | "INVALID_JSON"
  | "INVALID_TYPE"
  | "MISSING_FIELD"
  | "UNKNOWN_FIELD"
  | "UNKNOWN_OPERATION"
  | "UNSUPPORTED_SCHEMA_VERSION"
  | "INLINE_SECRET_FORBIDDEN"
  | "INCONSISTENT_TOOL_RESULT";

export class WorkflowContractParseError extends Error {
  readonly compatibility = "refresh_required" as const;

  constructor(readonly code: WorkflowContractErrorCode, readonly path: string, message: string) {
    super(message);
    this.name = "WorkflowContractParseError";
  }
}

type Obj = Record<string, unknown>;
const INLINE_SECRET_FIELDS = new Set(["password", "secret", "token", "apikey", "accesskey", "privatekey"]);

function fail(code: WorkflowContractErrorCode, path: string, message: string): never {
  throw new WorkflowContractParseError(code, path, message);
}

function object(value: unknown, path: string): Obj {
  if (!value || typeof value !== "object" || Array.isArray(value)) fail("INVALID_TYPE", path, `${path} must be an object`);
  return value as Obj;
}

function exact(value: Obj, allowed: readonly string[], path: string): void {
  const unknown = Object.keys(value).find((key) => !allowed.includes(key));
  if (unknown) fail("UNKNOWN_FIELD", `${path}.${unknown}`, `unknown field ${unknown}`);
}

function string(value: unknown, path: string): string {
  if (typeof value !== "string" || value.length === 0) fail(value === undefined ? "MISSING_FIELD" : "INVALID_TYPE", path, `${path} must be a non-empty string`);
  return value;
}

function number(value: unknown, path: string): number {
  if (typeof value !== "number" || !Number.isFinite(value)) fail(value === undefined ? "MISSING_FIELD" : "INVALID_TYPE", path, `${path} must be finite`);
  return value;
}

function boolean(value: unknown, path: string): boolean {
  if (typeof value !== "boolean") fail(value === undefined ? "MISSING_FIELD" : "INVALID_TYPE", path, `${path} must be boolean`);
  return value;
}

function oneOf<T extends string>(value: unknown, values: readonly T[], path: string): T {
  if (typeof value !== "string" || !values.includes(value as T)) fail("INVALID_TYPE", path, `${path} has an unsupported value`);
  return value as T;
}

function optional<T>(value: unknown, parse: (value: unknown) => T): T | undefined {
  return value === undefined ? undefined : parse(value);
}

function json(value: unknown, path: string): JsonValue {
  if (value === null || typeof value === "string" || typeof value === "boolean") return value;
  if (typeof value === "number" && Number.isFinite(value)) return value;
  if (Array.isArray(value)) return value.map((item, index) => json(item, `${path}[${index}]`));
  const source = object(value, path);
  const result: Record<string, JsonValue> = {};
  for (const [key, item] of Object.entries(source)) {
    const normalizedKey = key.toLowerCase().replace(/[_-]/g, "");
    if (INLINE_SECRET_FIELDS.has(normalizedKey)) fail("INLINE_SECRET_FORBIDDEN", `${path}.${key}`, "credentials must use secretRef");
    result[key] = json(item, `${path}.${key}`);
  }
  return result;
}

function workflowKey(value: unknown, path: string): WorkflowKey {
  const source = object(value, path);
  exact(source, ["tenantId", "workflowId"], path);
  const parseID = (item: unknown, itemPath: string) => { const result = string(item, itemPath); if (result === "." || result === ".." || /[\\/\0]/.test(result)) fail("INVALID_TYPE", itemPath, `${itemPath} is not canonical`); return result; };
  return { tenantId: parseID(source.tenantId, `${path}.tenantId`), workflowId: parseID(source.workflowId, `${path}.workflowId`) };
}

function revision(value: unknown, path: string): WorkflowRevision {
  const source = object(value, path);
  exact(source, ["resourceVersion", "semanticHash", "layoutHash"], path);
  return {
    resourceVersion: string(source.resourceVersion, `${path}.resourceVersion`),
    semanticHash: string(source.semanticHash, `${path}.semanticHash`),
    layoutHash: string(source.layoutHash, `${path}.layoutHash`),
  };
}

function basis(value: unknown, path: string): ValidationBasis {
  const source = object(value, path);
  const keys = ["resourceVersion", "semanticHash", "catalogVersion", "actionSchemaDigest", "referenceSnapshotHash", "environmentBindingVersion"] as const;
  exact(source, keys, path);
  return Object.fromEntries(keys.map((key) => [key, string(source[key], `${path}.${key}`)])) as ValidationBasis;
}

function node(value: unknown, path: string): NodeSpec {
  const source = object(value, path);
  const keys = ["id", "type", "position", "stepName", "stepId", "step", "handlerName", "handler", "approval", "condition", "subflow", "aggregator", "join", "loop", "parentId", "label", "collapsed", "ports", "inputs", "outputs"] as const;
  exact(source, keys, path);
  const result: NodeSpec = { type: oneOf(source.type, ["start", "action", "condition", "parallel", "join", "loop", "handler", "group", "subflow", "manual_approval", "variable_aggregator", "end"], `${path}.type`), position: position(source.position, `${path}.position`) };
  for (const key of ["id", "stepName", "stepId", "handlerName", "parentId", "label"] as const) if (source[key] !== undefined) result[key] = string(source[key], `${path}.${key}`);
  if (source.collapsed !== undefined) result.collapsed = boolean(source.collapsed, `${path}.collapsed`);
  for (const key of ["step", "handler", "approval", "condition", "subflow", "aggregator", "join", "loop", "ports", "inputs", "outputs"] as const) if (source[key] !== undefined) result[key] = json(source[key], `${path}.${key}`);
  return result;
}

function edge(value: unknown, path: string): EdgeSpec {
  const source = object(value, path);
  exact(source, ["id", "source", "sourcePort", "target", "targetPort", "kind", "condition"], path);
  return {
    ...optional(source.id, (item) => ({ id: string(item, `${path}.id`) })), source: string(source.source, `${path}.source`),
    ...optional(source.sourcePort, (item) => ({ sourcePort: string(item, `${path}.sourcePort`) })), target: string(source.target, `${path}.target`),
    ...optional(source.targetPort, (item) => ({ targetPort: string(item, `${path}.targetPort`) })),
    ...optional(source.kind, (item) => ({ kind: string(item, `${path}.kind`) })), ...optional(source.condition, (item) => ({ condition: string(item, `${path}.condition`) })),
  };
}

function position(value: unknown, path: string): Position {
  const source = object(value, path); exact(source, ["x", "y"], path);
  return { x: number(source.x, `${path}.x`), y: number(source.y, `${path}.y`) };
}

function patch(value: unknown, path: string, fields: readonly string[]): Record<string, JsonValue> {
  const source = object(value, path);
  exact(source, fields, path);
  return Object.fromEntries(Object.entries(source).map(([key, item]) => [key, json(item, `${path}.${key}`)]));
}

function stringArray(value: unknown, path: string): string[] {
  if (!Array.isArray(value)) fail("INVALID_TYPE", path, `${path} must be an array`);
  return value.map((item, index) => string(item, `${path}[${index}]`));
}

function validation(value: unknown, path: string) {
  const source = object(value, path); exact(source, ["basis", "valid", "diagnostics"], path);
  if (!Array.isArray(source.diagnostics)) fail("INVALID_TYPE", `${path}.diagnostics`, "diagnostics must be an array");
  const diagnostics = source.diagnostics.map((item, index) => { const diagnostic = object(item, `${path}.diagnostics[${index}]`); exact(diagnostic, ["code", "severity", "message", "nodeId", "edgeId", "field", "suggestion", "details"], `${path}.diagnostics[${index}]`); return json(diagnostic, `${path}.diagnostics[${index}]`); });
  return { basis: basis(source.basis, `${path}.basis`), valid: boolean(source.valid, `${path}.valid`), diagnostics } as WorkflowValidationResult;
}

function operation(value: unknown, index: number): WorkflowOperation {
  const path = `operations[${index}]`;
  const source = object(value, path);
  const op = typeof source.op === "string" ? source.op : fail("MISSING_FIELD", `${path}.op`, "operation discriminator is required");
  switch (op) {
    case "add_node": exact(source, ["op", "clientRef", "node"], path); return { op, clientRef: string(source.clientRef, `${path}.clientRef`), node: node(source.node, `${path}.node`) };
    case "update_node": exact(source, ["op", "nodeId", "set"], path); return { op, nodeId: string(source.nodeId, `${path}.nodeId`), set: patch(source.set, `${path}.set`, ["type", "position", "stepName", "stepId", "step", "handlerName", "handler", "approval", "condition", "subflow", "aggregator", "join", "loop", "parentId", "label", "collapsed", "ports", "inputs", "outputs"]) };
    case "delete_node": exact(source, ["op", "nodeId", "cascadeEdges"], path); return { op, nodeId: string(source.nodeId, `${path}.nodeId`), cascadeEdges: boolean(source.cascadeEdges, `${path}.cascadeEdges`) };
    case "add_edge": exact(source, ["op", "clientRef", "edge"], path); return { op, ...optional(source.clientRef, (item) => ({ clientRef: string(item, `${path}.clientRef`) })), edge: edge(source.edge, `${path}.edge`) };
    case "update_edge": exact(source, ["op", "edgeId", "set"], path); return { op, edgeId: string(source.edgeId, `${path}.edgeId`), set: patch(source.set, `${path}.set`, ["source", "sourcePort", "target", "targetPort", "kind", "condition"]) };
    case "delete_edge": exact(source, ["op", "edgeId"], path); return { op, edgeId: string(source.edgeId, `${path}.edgeId`) };
    case "update_workflow": exact(source, ["op", "set"], path); return { op, set: patch(source.set, `${path}.set`, ["name", "description", "envPackages", "validationEnv", "vars", "plan"]) };
    case "update_layout": {
      exact(source, ["op", "layout"], path); const layout = object(source.layout, `${path}.layout`); exact(layout, ["nodes", "viewport"], `${path}.layout`);
      if (!Array.isArray(layout.nodes)) fail("INVALID_TYPE", `${path}.layout.nodes`, "layout nodes must be an array");
      const nodes = layout.nodes.map((item, itemIndex) => { const point = object(item, `${path}.layout.nodes[${itemIndex}]`); exact(point, ["nodeId", "position"], `${path}.layout.nodes[${itemIndex}]`); return { nodeId: string(point.nodeId, `${path}.layout.nodes[${itemIndex}].nodeId`), position: position(point.position, `${path}.layout.nodes[${itemIndex}].position`) }; });
      const viewport = optional(layout.viewport, (item) => { const view = object(item, `${path}.layout.viewport`); exact(view, ["x", "y", "zoom"], `${path}.layout.viewport`); return { x: number(view.x, `${path}.layout.viewport.x`), y: number(view.y, `${path}.layout.viewport.y`), zoom: number(view.zoom, `${path}.layout.viewport.zoom`) }; });
      return { op, layout: { nodes, ...(viewport ? { viewport } : {}) } };
    }
    case "attach_manual_binding": { exact(source, ["op", "manualRef", "workflowDigest"], path); const ref = object(source.manualRef, `${path}.manualRef`); exact(ref, ["manualId", "candidateId"], `${path}.manualRef`); return { op, manualRef: { manualId: string(ref.manualId, `${path}.manualRef.manualId`), ...optional(ref.candidateId, (item) => ({ candidateId: string(item, `${path}.manualRef.candidateId`) })) }, workflowDigest: string(source.workflowDigest, `${path}.workflowDigest`) }; }
    default: fail("UNKNOWN_OPERATION", `${path}.op`, `unsupported operation ${op}`);
  }
}

function input(value: unknown): unknown {
  if (typeof value !== "string") return value;
  try { return JSON.parse(value); } catch { return fail("INVALID_JSON", "$", "invalid JSON"); }
}

export function parseWorkflowPatchRequestV2(value: unknown): WorkflowPatchRequestV2 {
  const source = object(input(value), "$request");
  exact(source, ["schemaVersion", "workflowKey", "expected", "validationBasis", "intent", "operations"], "$request");
  if (source.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) fail("UNSUPPORTED_SCHEMA_VERSION", "$request.schemaVersion", "unsupported workflow contract version");
  if (!Array.isArray(source.operations)) fail("INVALID_TYPE", "$request.operations", "operations must be an array");
  return { schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION, workflowKey: workflowKey(source.workflowKey, "$request.workflowKey"), expected: revision(source.expected, "$request.expected"), validationBasis: basis(source.validationBasis, "$request.validationBasis"), intent: string(source.intent, "$request.intent"), operations: source.operations.map(operation) };
}

export function serializeWorkflowPatchRequestV2(value: WorkflowPatchRequestV2): string {
  return JSON.stringify(parseWorkflowPatchRequestV2(value));
}

export function parseWorkflowToolResultV2(value: unknown): WorkflowToolResultV2 {
  const source = object(input(value), "$result");
  exact(source, ["schemaVersion", "isError", "content", "structuredContent", "_meta"], "$result");
  if (source.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) fail("UNSUPPORTED_SCHEMA_VERSION", "$result.schemaVersion", "unsupported workflow contract version");
  if (!Array.isArray(source.content) || source.content.length === 0) fail("INVALID_TYPE", "$result.content", "text summary is required");
  const content = source.content.map((item, index) => { const part = object(item, `$result.content[${index}]`); exact(part, ["type", "text"], `$result.content[${index}]`); if (part.type !== "text") fail("INVALID_TYPE", `$result.content[${index}].type`, "only text compatibility summaries are supported"); return { type: "text" as const, text: string(part.text, `$result.content[${index}].text`) }; });
  const structured = object(source.structuredContent, "$result.structuredContent");
  const isError = boolean(source.isError, "$result.isError");
  const meta = optional(source._meta, (item) => ({ _meta: json(item, "$result._meta") }));
  if (structured.kind === "error") {
    exact(structured, ["kind", "errorCode", "message", "workflowKey", "currentRevision", "baseRevision", "conflictingPaths", "changedBy", "retryHint", "details"], "$result.structuredContent");
    if (!isError) fail("INCONSISTENT_TOOL_RESULT", "$result.isError", "domain errors require isError=true");
    const paths = optional(structured.conflictingPaths, (item) => stringArray(item, "$result.structuredContent.conflictingPaths"));
    return { schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION, isError, content, ...meta, structuredContent: { kind: "error", errorCode: oneOf(structured.errorCode, WORKFLOW_DOMAIN_ERROR_CODES, "$result.structuredContent.errorCode"), message: string(structured.message, "$result.structuredContent.message"), ...optional(structured.workflowKey, (item) => ({ workflowKey: workflowKey(item, "$result.structuredContent.workflowKey") })), ...optional(structured.currentRevision, (item) => ({ currentRevision: string(item, "$result.structuredContent.currentRevision") })), ...optional(structured.baseRevision, (item) => ({ baseRevision: string(item, "$result.structuredContent.baseRevision") })), ...(paths ? { conflictingPaths: paths } : {}), ...optional(structured.changedBy, (item) => ({ changedBy: string(item, "$result.structuredContent.changedBy") })), ...optional(structured.retryHint, (item) => ({ retryHint: string(item, "$result.structuredContent.retryHint") })), ...optional(structured.details, (item) => ({ details: json(item, "$result.structuredContent.details") })) } };
  }
  if (structured.kind !== "receipt") fail("INVALID_TYPE", "$result.structuredContent.kind", "structuredContent must be receipt or error");
  exact(structured, ["schemaVersion", "kind", "transactionId", "changeSetId", "workflowKey", "status", "revisionBefore", "revisionAfter", "delta", "validation", "checkpointId", "inverseTransactionId", "auditId"], "$result.structuredContent");
  if (structured.schemaVersion !== WORKFLOW_CONTRACT_SCHEMA_VERSION) fail("UNSUPPORTED_SCHEMA_VERSION", "$result.structuredContent.schemaVersion", "unsupported receipt version");
  if (isError) fail("INCONSISTENT_TOOL_RESULT", "$result.isError", "receipts require isError=false");
  const delta = object(structured.delta, "$result.structuredContent.delta"); exact(delta, ["addedNodeIds", "updatedNodeIds", "deletedNodeIds", "addedEdgeIds", "updatedEdgeIds", "deletedEdgeIds"], "$result.structuredContent.delta");
  return { schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION, isError, content, ...meta, structuredContent: { schemaVersion: WORKFLOW_CONTRACT_SCHEMA_VERSION, kind: "receipt", transactionId: string(structured.transactionId, "$result.structuredContent.transactionId"), changeSetId: string(structured.changeSetId, "$result.structuredContent.changeSetId"), workflowKey: workflowKey(structured.workflowKey, "$result.structuredContent.workflowKey"), status: oneOf(structured.status, ["applied", "no_effect", "conflict", "rejected"], "$result.structuredContent.status"), revisionBefore: revision(structured.revisionBefore, "$result.structuredContent.revisionBefore"), ...optional(structured.revisionAfter, (item) => ({ revisionAfter: revision(item, "$result.structuredContent.revisionAfter") })), delta: { addedNodeIds: stringArray(delta.addedNodeIds, "$result.structuredContent.delta.addedNodeIds"), updatedNodeIds: stringArray(delta.updatedNodeIds, "$result.structuredContent.delta.updatedNodeIds"), deletedNodeIds: stringArray(delta.deletedNodeIds, "$result.structuredContent.delta.deletedNodeIds"), addedEdgeIds: stringArray(delta.addedEdgeIds, "$result.structuredContent.delta.addedEdgeIds"), updatedEdgeIds: stringArray(delta.updatedEdgeIds, "$result.structuredContent.delta.updatedEdgeIds"), deletedEdgeIds: stringArray(delta.deletedEdgeIds, "$result.structuredContent.delta.deletedEdgeIds") }, validation: validation(structured.validation, "$result.structuredContent.validation"), ...optional(structured.checkpointId, (item) => ({ checkpointId: string(item, "$result.structuredContent.checkpointId") })), ...optional(structured.inverseTransactionId, (item) => ({ inverseTransactionId: string(item, "$result.structuredContent.inverseTransactionId") })), auditId: string(structured.auditId, "$result.structuredContent.auditId") } };
}
