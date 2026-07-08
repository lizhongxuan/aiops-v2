import httpClient from "./httpClient";

const RUNNER_STUDIO_API_PREFIX = "/api/runner-studio";
const RUNNER_STUDIO_WORKFLOW_AI_PREFIX = `${RUNNER_STUDIO_API_PREFIX}/workflow-ai`;

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

    createRunnerStudioWorkflowAiSession(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/sessions`, payload);
    },

    getRunnerStudioWorkflowAiSnapshot(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/snapshot`, payload);
    },

    proposeRunnerStudioWorkflowAiPlan(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/plan`, payload);
    },

    proposeRunnerStudioWorkflowAiPatch(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/patch`, payload);
    },

    validateRunnerStudioWorkflowAiPatch(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/validate`, payload);
    },

    previewRunnerStudioWorkflowAiPatch(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/preview`, payload);
    },

    describeRunnerStudioWorkflowAiPatch(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/describe`, payload);
    },

    detectRunnerStudioWorkflowAiPatchEffect(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/effect`, payload);
    },

    applyRunnerStudioWorkflowAiPatch(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/apply`, payload);
    },

    undoRunnerStudioWorkflowAiPatch(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/undo`, payload);
    },

    proposeRunnerStudioWorkflowManualCandidate(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/manual-candidate`, payload);
    },

    proposeRunnerStudioWorkflowManualUpdate(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/manual-update`, payload);
    },

    createRunnerStudioWorkflowAiDraftFromPlan(payload = {}) {
      return client.post(`${RUNNER_STUDIO_WORKFLOW_AI_PREFIX}/create-draft`, payload);
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
export const createRunnerStudioWorkflowAiSession = (...args) =>
  runnerStudioClient.createRunnerStudioWorkflowAiSession(...args);
export const getRunnerStudioWorkflowAiSnapshot = (...args) =>
  runnerStudioClient.getRunnerStudioWorkflowAiSnapshot(...args);
export const proposeRunnerStudioWorkflowAiPlan = (...args) =>
  runnerStudioClient.proposeRunnerStudioWorkflowAiPlan(...args);
export const proposeRunnerStudioWorkflowAiPatch = (...args) =>
  runnerStudioClient.proposeRunnerStudioWorkflowAiPatch(...args);
export const validateRunnerStudioWorkflowAiPatch = (...args) =>
  runnerStudioClient.validateRunnerStudioWorkflowAiPatch(...args);
export const previewRunnerStudioWorkflowAiPatch = (...args) =>
  runnerStudioClient.previewRunnerStudioWorkflowAiPatch(...args);
export const describeRunnerStudioWorkflowAiPatch = (...args) =>
  runnerStudioClient.describeRunnerStudioWorkflowAiPatch(...args);
export const detectRunnerStudioWorkflowAiPatchEffect = (...args) =>
  runnerStudioClient.detectRunnerStudioWorkflowAiPatchEffect(...args);
export const applyRunnerStudioWorkflowAiPatch = (...args) =>
  runnerStudioClient.applyRunnerStudioWorkflowAiPatch(...args);
export const undoRunnerStudioWorkflowAiPatch = (...args) =>
  runnerStudioClient.undoRunnerStudioWorkflowAiPatch(...args);
export const proposeRunnerStudioWorkflowManualCandidate = (...args) =>
  runnerStudioClient.proposeRunnerStudioWorkflowManualCandidate(...args);
export const proposeRunnerStudioWorkflowManualUpdate = (...args) =>
  runnerStudioClient.proposeRunnerStudioWorkflowManualUpdate(...args);
export const createRunnerStudioWorkflowAiDraftFromPlan = (...args) =>
  runnerStudioClient.createRunnerStudioWorkflowAiDraftFromPlan(...args);

export default runnerStudioClient;
