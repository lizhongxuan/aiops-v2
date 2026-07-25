import httpClient from "./httpClient";

const RUNNER_STUDIO_API_PREFIX = "/api/runner-studio";
const WORKFLOW_COMMAND_FIELDS = new Set(["schemaVersion", "workflowId", "expectedRevision", "idempotencyKey", "reason", "confirmed", "command", "commands"]);
const WORKFLOW_COMMAND_KINDS = new Set(["add_node", "update_node", "delete_node", "connect_edge", "update_edge", "disconnect_edge", "move"]);
const WORKFLOW_COMMAND_BATCH_LIMIT = 64;
const WORKFLOW_DRY_RUN_FIELDS = new Set(["schemaVersion", "workflowId", "expectedBasis", "environment"]);
const WORKFLOW_DRAFT_TARGET_FIELDS = new Set([
  "kind", "workflowId", "resourceVersion", "semanticHash", "validationBasis", "environment",
]);
const WORKFLOW_PUBLISHED_RUN_FIELDS = new Set([
  "target", "vars", "triggeredBy", "idempotencyKey", "riskAcknowledged",
]);
const WORKFLOW_PUBLISHED_TARGET_FIELDS = new Set([
  "kind", "workflowKey", "resourceVersion", "semanticHash",
]);
const WORKFLOW_KEY_FIELDS = new Set(["tenantId", "workflowId"]);
const WORKFLOW_BASIS_FIELDS = ["resourceVersion", "semanticHash", "catalogVersion", "actionSchemaDigest", "referenceSnapshotHash", "environmentBindingVersion"];
const WORKFLOW_HISTORY_ACTION_FIELDS = new Set(["schemaVersion", "workflowId", "expected", "targetTransactionId", "idempotencyKey", "action"]);
const WORKFLOW_CANDIDATE_REVISE_FIELDS = new Set([
  "schemaVersion", "workflowId", "candidateId", "baseChangeSetId", "expected", "idempotencyKey", "intent", "additionalOperation",
]);
const WORKFLOW_CANDIDATE_ACTION_FIELDS = new Set([
  "schemaVersion", "workflowId", "candidateId", "candidateRevision", "changeSetId", "previewHash", "expected", "idempotencyKey", "action",
]);
const WORKFLOW_CANDIDATE_APPLY_APPROVAL_FIELDS = new Set([
  "schemaVersion", "workflowId", "candidateId", "candidateRevision", "idempotencyKey", "action",
]);
const WORKFLOW_REVISION_FIELDS = ["resourceVersion", "semanticHash", "layoutHash"];
const WORKFLOW_CANDIDATE_REVISE_OPERATION_FIELDS = {
  update_node: new Set(["op", "nodeId", "set"]),
  update_layout: new Set(["op", "layout"]),
  add_edge: new Set(["op", "clientRef", "edge"]),
  update_edge: new Set(["op", "edgeId", "set"]),
  delete_edge: new Set(["op", "edgeId"]),
};
const WORKFLOW_NODE_PATCH_FIELDS = new Set([
  "type", "position", "stepName", "stepId", "step", "handlerName", "handler", "approval", "condition", "subflow",
  "aggregator", "join", "loop", "parentId", "label", "collapsed", "ports", "inputs", "outputs",
]);
const WORKFLOW_EDGE_FIELDS = new Set(["id", "source", "sourcePort", "target", "targetPort", "kind", "condition"]);
const WORKFLOW_EDGE_PATCH_FIELDS = new Set(["source", "sourcePort", "target", "targetPort", "kind", "condition"]);

function encodePathSegment(value) {
  return encodeURIComponent(String(value));
}

function buildQuery(params = {}) {
  const query = new URLSearchParams();
  Object.entries(params || {}).forEach(([key, value]) => {
    if (value === undefined || value === null || value === "") {
      return;
    }
    query.set(key, String(value));
  });
  const serialized = query.toString();
  return serialized ? `?${serialized}` : "";
}

function assertWorkflowCommand(payload) {
  if (!payload || typeof payload !== "object") {
    throw new TypeError("workflow command payload is required");
  }
  const unknownField = Object.keys(payload).find((field) => !WORKFLOW_COMMAND_FIELDS.has(field));
  if (unknownField) {
    throw new TypeError(`workflow command field ${unknownField} is not allowed`);
  }
  if (payload.schemaVersion !== "aiops.workflow.edit.v2") {
    throw new TypeError("workflow command schemaVersion is required");
  }
  if (!String(payload.workflowId || "").trim()) {
    throw new TypeError("workflow command workflowId is required");
  }
  if (!String(payload.expectedRevision || "").trim()) {
    throw new TypeError("workflow command expectedRevision is required");
  }
  if (!String(payload.idempotencyKey || "").trim()) {
    throw new TypeError("workflow command idempotencyKey is required");
  }
  if (!String(payload.reason || "").trim()) {
    throw new TypeError("workflow command reason is required");
  }
  const hasCommand = Object.prototype.hasOwnProperty.call(payload, "command");
  const hasCommands = Object.prototype.hasOwnProperty.call(payload, "commands");
  if (hasCommand === hasCommands) {
    throw new TypeError("workflow command envelope requires exactly one of command or commands");
  }
  const commands = hasCommands ? payload.commands : [payload.command];
  if (!Array.isArray(commands) || commands.length === 0 || commands.length > WORKFLOW_COMMAND_BATCH_LIMIT) {
    throw new TypeError(`workflow commands must contain 1 to ${WORKFLOW_COMMAND_BATCH_LIMIT} items`);
  }
  commands.forEach(assertWorkflowManualCommand);
}

function assertWorkflowManualCommand(command) {
  if (!command || typeof command !== "object" || Array.isArray(command) || !String(command.kind || "").trim()) {
    throw new TypeError("workflow command is required");
  }
  if (!WORKFLOW_COMMAND_KINDS.has(command.kind)) {
    throw new TypeError("workflow command kind is not supported");
  }
}

function assertWorkflowDryRun(payload) {
  if (!payload || typeof payload !== "object") throw new TypeError("workflow dry-run payload is required");
  const unknownField = Object.keys(payload).find((field) => !WORKFLOW_DRY_RUN_FIELDS.has(field));
  if (unknownField) throw new TypeError(`workflow dry-run field ${unknownField} is not allowed`);
  if (payload.schemaVersion !== "aiops.workflow.edit.v2" || !String(payload.workflowId || "").trim()) {
    throw new TypeError("workflow dry-run identity is required");
  }
  const basis = payload.expectedBasis;
  if (!basis || typeof basis !== "object" || Object.keys(basis).sort().join("\0") !== [...WORKFLOW_BASIS_FIELDS].sort().join("\0") || WORKFLOW_BASIS_FIELDS.some((field) => !String(basis[field] || "").trim())) {
    throw new TypeError("workflow dry-run full validation basis is required");
  }
  if (typeof payload.environment !== "string" || payload.environment !== payload.environment.trim()) {
    throw new TypeError("workflow dry-run environment is invalid");
  }
}

function buildWorkflowDraftDryRunRequest(target) {
  if (!target || typeof target !== "object" || Array.isArray(target)) {
    throw new TypeError("workflow draft target is required");
  }
  assertAllowedFields(target, WORKFLOW_DRAFT_TARGET_FIELDS, "workflow draft target");
  if (target.kind !== "draft_non_production") {
    throw new TypeError("workflow draft target kind must be draft_non_production");
  }
  for (const field of ["workflowId", "resourceVersion", "semanticHash", "environment"]) {
    assertStrictString(target[field], `workflow draft target ${field}`);
  }
  const basis = target.validationBasis;
  if (!basis || typeof basis !== "object" || Array.isArray(basis)
    || Object.keys(basis).sort().join("\0") !== [...WORKFLOW_BASIS_FIELDS].sort().join("\0")
    || WORKFLOW_BASIS_FIELDS.some((field) => !String(basis[field] || "").trim())) {
    throw new TypeError("workflow draft target full validation basis is required");
  }
  if (basis.resourceVersion !== target.resourceVersion || basis.semanticHash !== target.semanticHash) {
    throw new TypeError("workflow draft target does not match its validation basis");
  }
  return {
    schemaVersion: "aiops.workflow.edit.v2",
    workflowId: target.workflowId,
    expectedBasis: { ...basis },
    environment: target.environment,
  };
}

function buildWorkflowPublishedRunRequest(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new TypeError("workflow published run payload is required");
  }
  assertAllowedFields(payload, WORKFLOW_PUBLISHED_RUN_FIELDS, "workflow published run");
  const target = payload.target;
  if (!target || typeof target !== "object" || Array.isArray(target)) {
    throw new TypeError("workflow published run target is required");
  }
  assertAllowedFields(target, WORKFLOW_PUBLISHED_TARGET_FIELDS, "workflow published run target");
  if (target.kind !== "published_revision") {
    throw new TypeError("workflow published run target kind must be published_revision");
  }
  const workflowKey = target.workflowKey;
  if (!workflowKey || typeof workflowKey !== "object" || Array.isArray(workflowKey)) {
    throw new TypeError("workflow published run workflowKey is required");
  }
  assertAllowedFields(workflowKey, WORKFLOW_KEY_FIELDS, "workflow published run workflowKey");
  assertStrictString(workflowKey.tenantId, "workflow published run tenantId");
  assertStrictString(workflowKey.workflowId, "workflow published run workflowId");
  assertCanonicalWorkflowHash(target.resourceVersion, "workflow published run resourceVersion");
  assertCanonicalWorkflowHash(target.semanticHash, "workflow published run semanticHash");
  assertStrictString(payload.triggeredBy, "workflow published run triggeredBy");
  assertStrictString(payload.idempotencyKey, "workflow published run idempotencyKey");
  if (payload.vars !== undefined && (!payload.vars || typeof payload.vars !== "object" || Array.isArray(payload.vars))) {
    throw new TypeError("workflow published run vars must be an object");
  }
  if (payload.riskAcknowledged !== undefined && typeof payload.riskAcknowledged !== "boolean") {
    throw new TypeError("workflow published run riskAcknowledged must be a boolean");
  }
  return {
    target: {
      workflowKey: { ...workflowKey },
      resourceVersion: target.resourceVersion,
      semanticHash: target.semanticHash,
    },
    ...(payload.vars === undefined ? {} : { vars: payload.vars }),
    triggered_by: payload.triggeredBy,
    idempotency_key: payload.idempotencyKey,
    ...(payload.riskAcknowledged === undefined ? {} : { risk_acknowledged: payload.riskAcknowledged }),
  };
}

function assertWorkflowHistoryAction(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new TypeError("workflow history action payload is required");
  }
  const unknownField = Object.keys(payload).find((field) => !WORKFLOW_HISTORY_ACTION_FIELDS.has(field));
  if (unknownField) throw new TypeError(`workflow history action field ${unknownField} is not allowed`);
  if (payload.schemaVersion !== "aiops.workflow.edit.v2") {
    throw new TypeError("workflow history action schemaVersion is required");
  }
  for (const field of ["workflowId", "targetTransactionId", "idempotencyKey"]) {
    if (typeof payload[field] !== "string" || !payload[field] || payload[field] !== payload[field].trim()) {
      throw new TypeError(`workflow history action ${field} is required`);
    }
  }
  if (payload.action !== "undo" && payload.action !== "redo") {
    throw new TypeError("workflow history action must be undo or redo");
  }
  const expected = payload.expected;
  if (!expected || typeof expected !== "object" || Array.isArray(expected)
    || Object.keys(expected).sort().join("\0") !== [...WORKFLOW_REVISION_FIELDS].sort().join("\0")
    || WORKFLOW_REVISION_FIELDS.some((field) => typeof expected[field] !== "string" || !expected[field] || expected[field] !== expected[field].trim())) {
    throw new TypeError("workflow history action full expected revision is required");
  }
}

function assertWorkflowCandidateRevise(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new TypeError("workflow candidate revise payload is required");
  }
  assertAllowedFields(payload, WORKFLOW_CANDIDATE_REVISE_FIELDS, "workflow candidate revise");
  if (payload.schemaVersion !== "aiops.workflow.edit.v2") {
    throw new TypeError("workflow candidate revise schemaVersion is required");
  }
  for (const field of ["workflowId", "candidateId", "baseChangeSetId", "idempotencyKey", "intent"]) {
    assertStrictString(payload[field], `workflow candidate revise ${field}`);
  }
  assertFullWorkflowRevision(payload.expected, "workflow candidate revise full expected revision");
  assertWorkflowCandidateReviseOperation(payload.additionalOperation);
}

function assertWorkflowCandidateDiscard(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new TypeError("workflow candidate action payload is required");
  }
  assertAllowedFields(payload, WORKFLOW_CANDIDATE_ACTION_FIELDS, "workflow candidate action");
  if (payload.schemaVersion !== "aiops.workflow.edit.v2" || payload.action !== "discard") {
    throw new TypeError("workflow candidate discard action is required");
  }
  for (const field of ["workflowId", "candidateId", "changeSetId", "previewHash", "idempotencyKey"]) {
    assertStrictString(payload[field], `workflow candidate discard ${field}`);
  }
  if (!Number.isSafeInteger(payload.candidateRevision) || payload.candidateRevision < 1) {
    throw new TypeError("workflow candidate discard candidateRevision is required");
  }
  assertFullWorkflowRevision(payload.expected, "workflow candidate discard full expected revision");
}

function assertWorkflowCandidateApplyApproval(payload) {
  if (!payload || typeof payload !== "object" || Array.isArray(payload)) {
    throw new TypeError("workflow candidate apply approval payload is required");
  }
  assertAllowedFields(payload, WORKFLOW_CANDIDATE_APPLY_APPROVAL_FIELDS, "workflow candidate apply approval");
  if (payload.schemaVersion !== "aiops.workflow.edit.v2" || payload.action !== "request_apply_approval") {
    throw new TypeError("workflow candidate apply approval action is required");
  }
  for (const field of ["workflowId", "candidateId", "idempotencyKey"]) {
    assertStrictString(payload[field], `workflow candidate apply approval ${field}`);
  }
  const historySelector = /^history-[a-f0-9]{64}$/.test(payload.candidateId)
    && payload.candidateRevision === 1;
  const revisedAgentSelector = /^candidate-[a-f0-9]{24}$/.test(payload.candidateId)
    && Number.isSafeInteger(payload.candidateRevision)
    && payload.candidateRevision >= 2;
  if (!historySelector && !revisedAgentSelector) {
    throw new TypeError("workflow candidate apply approval persisted selector is invalid");
  }
}

function assertWorkflowCandidateReviseOperation(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) {
    throw new TypeError("workflow candidate revise additionalOperation is required");
  }
  const allowed = WORKFLOW_CANDIDATE_REVISE_OPERATION_FIELDS[value.op];
  if (!allowed) throw new TypeError("workflow candidate revise operation is not supported");
  assertAllowedFields(value, allowed, "workflow candidate revise operation");
  if (value.op === "update_node") {
    assertStrictString(value.nodeId, "workflow candidate revise operation nodeId");
    assertNonEmptyPatch(value.set, WORKFLOW_NODE_PATCH_FIELDS, "workflow candidate revise node patch");
    return;
  }
  if (value.op === "update_layout") {
    assertCandidateLayout(value.layout);
    return;
  }
  if (value.op === "add_edge") {
    if (value.clientRef !== undefined) assertStrictString(value.clientRef, "workflow candidate revise operation clientRef");
    assertCandidateEdge(value.edge);
    return;
  }
  if (value.op === "update_edge") {
    assertStrictString(value.edgeId, "workflow candidate revise operation edgeId");
    assertNonEmptyPatch(value.set, WORKFLOW_EDGE_PATCH_FIELDS, "workflow candidate revise edge patch");
    return;
  }
  assertStrictString(value.edgeId, "workflow candidate revise operation edgeId");
}

function assertCandidateLayout(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError("workflow candidate revise layout is required");
  assertAllowedFields(value, new Set(["nodes", "viewport"]), "workflow candidate revise layout");
  if (!Array.isArray(value.nodes) || value.nodes.length === 0) throw new TypeError("workflow candidate revise layout nodes are required");
  value.nodes.forEach((item) => {
    if (!item || typeof item !== "object" || Array.isArray(item)) throw new TypeError("workflow candidate revise layout node is invalid");
    assertAllowedFields(item, new Set(["nodeId", "position"]), "workflow candidate revise layout node");
    assertStrictString(item.nodeId, "workflow candidate revise layout nodeId");
    assertPosition(item.position, "workflow candidate revise layout position");
  });
  if (value.viewport !== undefined) {
    if (!value.viewport || typeof value.viewport !== "object" || Array.isArray(value.viewport)) throw new TypeError("workflow candidate revise viewport is invalid");
    assertAllowedFields(value.viewport, new Set(["x", "y", "zoom"]), "workflow candidate revise viewport");
    for (const field of ["x", "y", "zoom"]) assertFiniteNumber(value.viewport[field], `workflow candidate revise viewport ${field}`);
  }
}

function assertCandidateEdge(value) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError("workflow candidate revise edge is required");
  assertAllowedFields(value, WORKFLOW_EDGE_FIELDS, "workflow candidate revise edge");
  assertStrictString(value.source, "workflow candidate revise edge source");
  assertStrictString(value.target, "workflow candidate revise edge target");
  for (const field of ["id", "sourcePort", "targetPort", "kind", "condition"]) {
    if (value[field] !== undefined) assertStrictString(value[field], `workflow candidate revise edge ${field}`);
  }
}

function assertNonEmptyPatch(value, allowed, label) {
  if (!value || typeof value !== "object" || Array.isArray(value) || Object.keys(value).length === 0) {
    throw new TypeError(`${label} is required`);
  }
  assertAllowedFields(value, allowed, label);
}

function assertPosition(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw new TypeError(`${label} is required`);
  assertAllowedFields(value, new Set(["x", "y"]), label);
  assertFiniteNumber(value.x, `${label} x`);
  assertFiniteNumber(value.y, `${label} y`);
}

function assertFullWorkflowRevision(value, label) {
  if (!value || typeof value !== "object" || Array.isArray(value)
    || Object.keys(value).sort().join("\0") !== [...WORKFLOW_REVISION_FIELDS].sort().join("\0")
    || WORKFLOW_REVISION_FIELDS.some((field) => typeof value[field] !== "string" || !value[field] || value[field] !== value[field].trim())) {
    throw new TypeError(`${label} is required`);
  }
}

function assertAllowedFields(value, allowed, label) {
  const unknownField = Object.keys(value).find((field) => !allowed.has(field));
  if (unknownField) throw new TypeError(`${label} field ${unknownField} is not allowed`);
}

function assertStrictString(value, label) {
  if (typeof value !== "string" || !value || value !== value.trim()) throw new TypeError(`${label} is required`);
}

function assertFiniteNumber(value, label) {
  if (typeof value !== "number" || !Number.isFinite(value)) throw new TypeError(`${label} must be finite`);
}

function assertCanonicalWorkflowHash(value, label) {
  if (typeof value !== "string" || !/^sha256:[a-f0-9]{64}$/.test(value)) {
    throw new TypeError(`${label} must be a canonical sha256 digest`);
  }
}

export function createRunnerStudioClient(client = httpClient) {
  return {
    listRunnerStudioWorkflows(params = {}) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/workflows${buildQuery(params)}`);
    },

    getRunnerStudioWorkflowGraph(name) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/graph`);
    },

    listRunnerStudioWorkflowVersions(name) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/versions`);
    },

    getRunnerStudioWorkflowVersion(name, versionId) {
      return client.get(
        `${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/versions/${encodePathSegment(versionId)}`,
      );
    },

    rollbackRunnerStudioWorkflowVersion(name, versionId, payload = {}) {
      return client.post(
        `${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/versions/${encodePathSegment(versionId)}/rollback`,
        payload,
      );
    },

    exportRunnerStudioWorkflowBundle(name) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/bundle`);
    },

    importRunnerStudioWorkflowBundle(payload = {}) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/bundles/import`, payload);
    },

    validateRunnerStudioWorkflow(name) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/validate`);
    },

    publishRunnerStudioWorkflow(name, payload = {}) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/publish`, payload);
    },

    createRunnerStudioWorkflowGraph(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/graph`, payload);
    },

    compileRunnerStudioWorkflowGraph(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/graph/compile`, payload);
    },

    parseRunnerStudioWorkflowYaml(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/graph/parse`, payload);
    },

    updateRunnerStudioWorkflowGraph(name, payload) {
      return client.put(`${RUNNER_STUDIO_API_PREFIX}/workflows/${encodePathSegment(name)}/graph`, payload);
    },

    validateRunnerStudioWorkflowGraph(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/graph/validate`, payload);
    },

    resolveRunnerStudioWorkflowVariables(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/graph/variables/resolve`, payload);
    },

    dryRunRunnerStudioWorkflowGraph(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflows/graph/dry-run`, payload);
    },

    runRunnerStudioWorkflowGraph(payload) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/runs`, payload);
    },

    getRunnerStudioRunGraph(runId) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/runs/${encodePathSegment(runId)}/graph`);
    },

    getRunnerStudioRunEventHistory(runId) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/runs/${encodePathSegment(runId)}/events/history`);
    },

    cancelRunnerStudioRun(runId) {
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/runs/${encodePathSegment(runId)}/cancel`);
    },

    getRunnerStudioActionCatalog(params = {}) {
      return client.get(`${RUNNER_STUDIO_API_PREFIX}/actions${buildQuery(params)}`);
    },

    executeRunnerStudioWorkflowCommand(payload) {
      assertWorkflowCommand(payload);
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-commands`, payload);
    },

    moveRunnerStudioWorkflowNodes(payload) {
      assertWorkflowCommand(payload);
      if (!payload.command || payload.command.kind !== "move" || !payload.command.layout) {
        throw new TypeError("move command requires a move layout command");
      }
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-commands`, payload);
    },

    dryRunAuthoritativeWorkflow(payload) {
      assertWorkflowDryRun(payload);
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-dry-run`, payload);
    },

    dryRunAuthoritativeWorkflowTarget(target) {
      return client.post(
        `${RUNNER_STUDIO_API_PREFIX}/workflow-dry-run`,
        buildWorkflowDraftDryRunRequest(target),
      );
    },

    runPublishedAuthoritativeWorkflow(payload) {
      return client.post(
        `${RUNNER_STUDIO_API_PREFIX}/workflow-published-run`,
        buildWorkflowPublishedRunRequest(payload),
      );
    },

    executeRunnerStudioWorkflowHistoryAction(payload) {
      assertWorkflowHistoryAction(payload);
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-history-actions`, payload);
    },

    reviseRunnerStudioWorkflowCandidate(payload) {
      assertWorkflowCandidateRevise(payload);
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-candidate-revise`, payload);
    },

    executeRunnerStudioWorkflowCandidateAction(payload) {
      assertWorkflowCandidateDiscard(payload);
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-candidate-actions`, payload);
    },

    requestRunnerStudioWorkflowCandidateApplyApproval(payload) {
      assertWorkflowCandidateApplyApproval(payload);
      return client.post(`${RUNNER_STUDIO_API_PREFIX}/workflow-candidate-apply-approval`, payload);
    },

  };
}

const runnerStudioClient = createRunnerStudioClient();

export const listRunnerStudioWorkflows = (...args) => runnerStudioClient.listRunnerStudioWorkflows(...args);
export const getRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.getRunnerStudioWorkflowGraph(...args);
export const listRunnerStudioWorkflowVersions = (...args) =>
  runnerStudioClient.listRunnerStudioWorkflowVersions(...args);
export const getRunnerStudioWorkflowVersion = (...args) => runnerStudioClient.getRunnerStudioWorkflowVersion(...args);
export const rollbackRunnerStudioWorkflowVersion = (...args) =>
  runnerStudioClient.rollbackRunnerStudioWorkflowVersion(...args);
export const exportRunnerStudioWorkflowBundle = (...args) =>
  runnerStudioClient.exportRunnerStudioWorkflowBundle(...args);
export const importRunnerStudioWorkflowBundle = (...args) =>
  runnerStudioClient.importRunnerStudioWorkflowBundle(...args);
export const validateRunnerStudioWorkflow = (...args) => runnerStudioClient.validateRunnerStudioWorkflow(...args);
export const publishRunnerStudioWorkflow = (...args) => runnerStudioClient.publishRunnerStudioWorkflow(...args);
export const createRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.createRunnerStudioWorkflowGraph(...args);
export const compileRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.compileRunnerStudioWorkflowGraph(...args);
export const parseRunnerStudioWorkflowYaml = (...args) => runnerStudioClient.parseRunnerStudioWorkflowYaml(...args);
export const updateRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.updateRunnerStudioWorkflowGraph(...args);
export const validateRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.validateRunnerStudioWorkflowGraph(...args);
export const resolveRunnerStudioWorkflowVariables = (...args) =>
  runnerStudioClient.resolveRunnerStudioWorkflowVariables(...args);
export const dryRunRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.dryRunRunnerStudioWorkflowGraph(...args);
export const runRunnerStudioWorkflowGraph = (...args) => runnerStudioClient.runRunnerStudioWorkflowGraph(...args);
export const getRunnerStudioRunGraph = (...args) => runnerStudioClient.getRunnerStudioRunGraph(...args);
export const getRunnerStudioRunEventHistory = (...args) => runnerStudioClient.getRunnerStudioRunEventHistory(...args);
export const cancelRunnerStudioRun = (...args) => runnerStudioClient.cancelRunnerStudioRun(...args);
export const getRunnerStudioActionCatalog = (...args) => runnerStudioClient.getRunnerStudioActionCatalog(...args);
export const executeRunnerStudioWorkflowCommand = (...args) =>
  runnerStudioClient.executeRunnerStudioWorkflowCommand(...args);
export const moveRunnerStudioWorkflowNodes = (...args) =>
  runnerStudioClient.moveRunnerStudioWorkflowNodes(...args);
export const dryRunAuthoritativeWorkflow = (...args) =>
  runnerStudioClient.dryRunAuthoritativeWorkflow(...args);
export const dryRunAuthoritativeWorkflowTarget = (...args) =>
  runnerStudioClient.dryRunAuthoritativeWorkflowTarget(...args);
export const runPublishedAuthoritativeWorkflow = (...args) =>
  runnerStudioClient.runPublishedAuthoritativeWorkflow(...args);
export const executeRunnerStudioWorkflowHistoryAction = (...args) =>
  runnerStudioClient.executeRunnerStudioWorkflowHistoryAction(...args);
export const reviseRunnerStudioWorkflowCandidate = (...args) =>
  runnerStudioClient.reviseRunnerStudioWorkflowCandidate(...args);
export const executeRunnerStudioWorkflowCandidateAction = (...args) =>
  runnerStudioClient.executeRunnerStudioWorkflowCandidateAction(...args);
export const requestRunnerStudioWorkflowCandidateApplyApproval = (...args) =>
  runnerStudioClient.requestRunnerStudioWorkflowCandidateApplyApproval(...args);
export default runnerStudioClient;
