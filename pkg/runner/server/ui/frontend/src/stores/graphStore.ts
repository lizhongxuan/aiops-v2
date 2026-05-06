import { computed, reactive } from "vue";
import { mockApi, runnerApi } from "../api/client";
import { mockRunEvents } from "../fixtures/mockGraph";
import { buildGraphDiffSummary } from "../utils/graphDiff";
import type {
  ActionSpec,
  DryRunResult,
  RunEvent,
  ValidationResult,
  WorkflowBundle,
  WorkflowDefinition,
  WorkflowGraph,
  WorkflowNode,
  WorkflowSummary,
  WorkflowVersion,
} from "../types/workflow";
import { createActionNodeFromSpec } from "../utils/actionCatalog";
import {
  addNodeToGraph,
  autoLayoutGraph,
  connectGraphNodes,
  createControlNode,
  deleteNodeFromGraph,
  type ControlNodeType,
} from "../utils/graphEditing";
import { applyRunStateToGraph, createInitialRunState, reduceRunEvent, type RunState } from "../utils/runEventReducer";
import { prepareWorkflowGraphForCreate } from "../utils/workflowTemplates";

interface GraphStoreState {
  graph: WorkflowGraph | null;
  baselineGraph: WorkflowGraph | null;
  actions: ActionSpec[];
  workflowOptions: WorkflowSummary[];
  workflowVersions: WorkflowVersion[];
  selectedNodeId: string | null;
  loading: boolean;
  saving: boolean;
  publishing: boolean;
  loadingVersions: boolean;
  rollingBack: boolean;
  creatingWorkflow: boolean;
  switchingWorkflow: boolean;
  exportingBundle: boolean;
  importingBundle: boolean;
  validating: boolean;
  dryRunning: boolean;
  submitting: boolean;
  canceling: boolean;
  resolvingApprovalNodeId: string | null;
  resolvingApprovalAction: "approve" | "reject" | null;
  replaying: boolean;
  previewCompiling: boolean;
  offline: boolean;
  dirty: boolean;
  eventConnected: boolean;
  error: string | null;
  saveNote: string;
  riskAcknowledged: boolean;
  warningAcknowledged: boolean;
  semanticChangeAcknowledged: boolean;
  workflowStatus: "draft" | "published" | string;
  publishedAt: string;
  validation: ValidationResult | null;
  dryRun: DryRunResult | null;
  yamlPreview: string;
  run: RunState;
  historyPast: WorkflowGraph[];
  historyFuture: WorkflowGraph[];
  clipboardNode: WorkflowNode | null;
}

const state = reactive<GraphStoreState>(initialGraphStoreState());

let unsubscribeRunEvents: (() => void) | null = null;
let compilePreviewTimer: ReturnType<typeof setTimeout> | null = null;
let compilePreviewSeq = 0;
const lastRunStorageKey = "runner.visual.lastRunId";
const autoCompilePreviewDelayMs = 250;

export function useGraphStore() {
  const selectedNode = computed(() => {
    return state.graph?.nodes.find((node) => node.id === state.selectedNodeId) || state.graph?.nodes[0] || null;
  });

  const graphWithRunState = computed(() => {
    return state.graph ? applyRunStateToGraph(state.graph, state.run) : null;
  });

  const waitingApprovalNodes = computed(() => {
    const graph = graphWithRunState.value;
    if (!graph) return [];
    return graph.nodes
      .filter((node) => node.type === "manual_approval" && (state.run.nodeStatus[node.id] || node.state?.status) === "waiting")
      .map((node) => ({
        id: node.id,
        label: node.label || node.step?.name || node.step_name || node.id,
      }));
  });

  const executionSemanticsChanged = computed(() => hasExecutionSemanticChanges());

  async function load(workflowName = "service-restart-candidate") {
    state.loading = true;
    state.error = null;
    let loadedOffline = false;
    try {
      const [graph, actions, workflowOptions] = await Promise.all([runnerApi.getGraph(workflowName), runnerApi.listActions(), runnerApi.listWorkflows()]);
      state.graph = graph;
      state.baselineGraph = cloneGraph(graph);
      state.actions = actions;
      state.workflowOptions = workflowOptions;
      setWorkflowPublishState(workflowOptions.find((item) => item.name === graph.workflow.name));
      state.offline = false;
      state.validation = null;
      state.dryRun = null;
      state.yamlPreview = "";
      state.dirty = false;
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
      state.semanticChangeAcknowledged = false;
      clearEditSession();
    } catch (error) {
      state.graph = await mockApi.getGraph();
      state.baselineGraph = cloneGraph(state.graph);
      state.actions = await mockApi.listActions();
      state.workflowOptions = await mockApi.listWorkflows();
      setWorkflowPublishState(state.workflowOptions.find((item) => item.name === state.graph?.workflow.name));
      state.offline = true;
      loadedOffline = true;
      state.error = error instanceof Error ? error.message : "Backend unavailable; loaded mock graph.";
      state.dirty = false;
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
      state.semanticChangeAcknowledged = false;
      clearEditSession();
    } finally {
      state.loading = false;
      state.selectedNodeId = selectInitialEditableNode(state.graph, state.selectedNodeId);
      if (loadedOffline) {
        mockRunEvents.forEach(pushRunEvent);
      }
    }
    if (!loadedOffline) {
      const lastRunID = readLastRunID();
      if (lastRunID) {
        await replayRunHistory(lastRunID);
      }
    }
  }

  function selectNode(nodeId: string | null) {
    state.selectedNodeId = nodeId;
  }

  function updateNode(nodeId: string, patch: Partial<WorkflowNode>) {
    if (!state.graph) return;
    if (!state.graph.nodes.some((node) => node.id === nodeId)) return;
    pushHistory();
    state.graph.nodes = state.graph.nodes.map((node) => (node.id === nodeId ? { ...node, ...patch } : node));
    markDirty();
  }

  function updateWorkflow(patch: Partial<WorkflowDefinition>) {
    if (!state.graph) return;
    pushHistory();
    state.graph = {
      ...state.graph,
      workflow: {
        ...state.graph.workflow,
        ...patch,
      },
    };
    markDirty();
  }

  function addActionNodeFromCatalog(action: string, position?: { x: number; y: number }) {
    if (!state.graph) return;
    const spec = state.actions.find((item) => item.action === action);
    if (!spec) {
      state.error = `Action ${action} is not available in the catalog.`;
      return;
    }
    const node = createActionNodeFromSpec(spec, state.graph, position);
    pushHistory();
    state.graph.nodes = [...state.graph.nodes, node];
    state.selectedNodeId = node.id;
    state.validation = null;
    state.dryRun = null;
    markDirty();
  }

  function addControlNode(type: ControlNodeType, position?: { x: number; y: number }) {
    if (!state.graph) return;
    const node = createControlNode(type, state.graph, position);
    pushHistory();
    state.graph = addNodeToGraph(state.graph, node);
    state.selectedNodeId = node.id;
    markDirty();
  }

  function connectNodes(source: string | null | undefined, target: string | null | undefined) {
    if (!state.graph) return;
    const result = connectGraphNodes(state.graph, source, target);
    if (result.error) {
      state.error = result.error;
      return;
    }
    pushHistory();
    state.graph = result.graph;
    markDirty();
  }

  function deleteSelectedNode() {
    if (!state.graph) return;
    const result = deleteNodeFromGraph(state.graph, state.selectedNodeId);
    if (result.error) {
      state.error = result.error;
      return;
    }
    pushHistory();
    state.graph = result.graph;
    state.selectedNodeId = result.graph.nodes[0]?.id || null;
    markDirty();
  }

  function autoLayout() {
    if (!state.graph) return;
    pushHistory();
    state.graph = autoLayoutGraph(state.graph);
    markDirty();
  }

  function replaceGraph(graph: WorkflowGraph) {
    if (state.graph) pushHistory();
    state.graph = graph;
    state.selectedNodeId = graph.nodes.some((node) => node.id === state.selectedNodeId) ? state.selectedNodeId : graph.nodes[0]?.id || null;
    state.validation = null;
    state.dryRun = null;
    markDirty();
  }

  async function createWorkflowFromGraph(graph: WorkflowGraph, options: { labels?: Record<string, string>; saveNote?: string } = {}) {
    state.creatingWorkflow = true;
    state.error = null;
    const wasOffline = state.offline;
    try {
      const saveNote = options.saveNote?.trim();
      const request = {
        graph,
        ...(options.labels ? { labels: options.labels } : {}),
        ...(saveNote ? { save_note: saveNote } : {}),
      };
      const result = wasOffline ? await mockApi.createGraphWorkflow(request) : await runnerApi.createGraphWorkflow(request);
      let workflowOptions = state.workflowOptions;
      try {
        workflowOptions = wasOffline ? await mockApi.listWorkflows() : await runnerApi.listWorkflows();
      } catch {
        workflowOptions = state.workflowOptions;
      }
      closeRunEvents();
      state.graph = result.graph;
      state.baselineGraph = cloneGraph(result.graph);
      state.workflowOptions = mergeWorkflowOptions(workflowOptions, result);
      state.workflowVersions = [];
      state.selectedNodeId = selectInitialEditableNode(result.graph);
      state.saveNote = "";
      state.workflowStatus = result.status || "draft";
      state.publishedAt = "";
      state.validation = null;
      state.dryRun = null;
      state.yamlPreview = result.yaml || "";
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
      state.semanticChangeAcknowledged = false;
      state.dirty = false;
      state.run = createInitialRunState();
      state.offline = wasOffline;
      clearEditSession();
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.creatingWorkflow = false;
    }
  }

  async function switchWorkflow(name: string, options: { force?: boolean } = {}) {
    const nextName = name.trim();
    if (!nextName) {
      state.error = "Workflow name is required.";
      return false;
    }
    if (state.graph?.workflow.name === nextName) {
      return true;
    }
    if (state.dirty && !options.force) {
      state.error = "Save or discard changes before switching workflows.";
      return false;
    }
    state.switchingWorkflow = true;
    try {
      closeRunEvents();
      await load(nextName);
      return state.graph?.workflow.name === nextName;
    } finally {
      state.switchingWorkflow = false;
    }
  }

  async function cloneCurrentWorkflow(input: { name: string; version: string; description?: string; labels?: Record<string, string>; saveNote?: string }) {
    if (!state.graph) return;
    const graph = prepareWorkflowGraphForCreate(state.graph, {
      name: input.name,
      version: input.version,
      description: input.description,
    });
    await createWorkflowFromGraph(graph, { labels: input.labels, saveNote: input.saveNote });
  }

  async function saveDraft() {
    if (!state.graph) return;
    if (!ensureSemanticChangeReviewed("saving")) return;
    state.saving = true;
    state.error = null;
    const workflowName = state.graph.workflow.name;
    const saveNote = state.saveNote;
    try {
      const result = await runnerApi.saveGraph(workflowName, state.graph, saveNote);
      state.yamlPreview = result.yaml;
      state.baselineGraph = cloneGraph(state.graph);
      state.saveNote = "";
      state.workflowStatus = "draft";
      state.publishedAt = "";
      upsertWorkflowOption({
        name: result.workflow.name || workflowName,
        version: result.workflow.version,
        description: result.workflow.description,
        save_note: saveNote.trim() || undefined,
        status: "draft",
      });
      state.dirty = false;
      state.semanticChangeAcknowledged = false;
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.saving = false;
    }
  }

  async function publishWorkflow() {
    if (!state.graph) return;
    if (state.dirty) {
      state.error = "Save draft before publishing.";
      return;
    }
    state.publishing = true;
    state.error = null;
    try {
      const result = await runnerApi.publishWorkflow(state.graph.workflow.name, {
        saveNote: state.saveNote,
        riskAcknowledged: state.riskAcknowledged,
        warningAcknowledged: state.warningAcknowledged,
      });
      setWorkflowPublishState(result);
      upsertWorkflowOption(result);
      state.saveNote = "";
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.publishing = false;
    }
  }

  async function loadWorkflowVersions() {
    if (!state.graph) return;
    state.loadingVersions = true;
    state.error = null;
    try {
      state.workflowVersions = await runnerApi.listWorkflowVersions(state.graph.workflow.name);
    } catch (error) {
      state.workflowVersions = [];
      state.error = errorMessage(error);
    } finally {
      state.loadingVersions = false;
    }
  }

  async function rollbackWorkflowVersion(versionId: string) {
    if (!state.graph) return;
    versionId = versionId.trim();
    if (!versionId) return;
    state.rollingBack = true;
    state.error = null;
    try {
      const result = await runnerApi.rollbackWorkflowVersion(state.graph.workflow.name, versionId, state.saveNote);
      const graph = await runnerApi.getGraph(state.graph.workflow.name);
      state.graph = graph;
      state.baselineGraph = cloneGraph(graph);
      state.selectedNodeId = graph.nodes[1]?.id || graph.nodes[0]?.id || null;
      state.workflowStatus = "draft";
      state.saveNote = "";
      state.semanticChangeAcknowledged = false;
      state.dirty = false;
      state.validation = null;
      state.dryRun = null;
      upsertWorkflowOption({ ...result, name: result.name || graph.workflow.name, status: result.status || "draft" });
      await loadWorkflowVersions();
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.rollingBack = false;
    }
  }

  async function exportWorkflowBundle(): Promise<WorkflowBundle | null> {
    if (!state.graph) return null;
    state.exportingBundle = true;
    state.error = null;
    try {
      return await runnerApi.exportWorkflowBundle(state.graph.workflow.name);
    } catch (error) {
      state.error = errorMessage(error);
      return null;
    } finally {
      state.exportingBundle = false;
    }
  }

  async function importWorkflowBundle(bundleText: string, options: { overwrite?: boolean } = {}) {
    state.importingBundle = true;
    state.error = null;
    try {
      const bundle = parseWorkflowBundle(bundleText);
      const result = await runnerApi.importWorkflowBundle(bundle, {
        overwrite: options.overwrite,
        saveNote: state.saveNote,
      });
      const workflowName = result.name || bundle.name;
      if (!workflowName) {
        throw new Error("Imported workflow response did not include a workflow name.");
      }
      const [graph, workflowOptions] = await Promise.all([runnerApi.getGraph(workflowName), runnerApi.listWorkflows()]);
      state.graph = graph;
      state.baselineGraph = cloneGraph(graph);
      state.workflowOptions = workflowOptions;
      state.workflowVersions = [];
      state.selectedNodeId = graph.nodes[1]?.id || graph.nodes[0]?.id || null;
      state.saveNote = "";
      state.workflowStatus = result.status || "draft";
      state.publishedAt = "";
      state.validation = null;
      state.dryRun = null;
      state.yamlPreview = result.yaml || bundle.yaml;
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
      state.semanticChangeAcknowledged = false;
      state.dirty = false;
      state.offline = false;
      clearEditSession();
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.importingBundle = false;
    }
  }

  async function validateGraph() {
    if (!state.graph) return;
    state.validating = true;
    state.error = null;
    try {
      state.validation = await runnerApi.validateGraph(state.graph);
    } catch (error) {
      state.validation = null;
      state.error = errorMessage(error);
    } finally {
      state.validating = false;
    }
  }

  async function dryRunGraph() {
    if (!state.graph) return;
    if (!ensureSemanticChangeReviewed("dry run")) return;
    state.dryRunning = true;
    state.error = null;
    try {
      state.dryRun = await runnerApi.dryRunGraph(state.graph);
      state.validation = {
        valid: state.dryRun.valid,
        errors: state.dryRun.errors,
        warnings: state.dryRun.warnings,
        summary: state.dryRun.summary,
      };
      state.yamlPreview = state.dryRun.yaml || state.yamlPreview;
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.dryRunning = false;
    }
  }

  async function compilePreview() {
    if (!state.graph) return;
    cancelScheduledCompilePreview();
    const seq = ++compilePreviewSeq;
    const graph = cloneGraph(state.graph);
    state.validating = true;
    state.error = null;
    try {
      const result = await runnerApi.compileGraph(graph);
      if (seq === compilePreviewSeq) {
        state.yamlPreview = result.yaml;
      }
    } catch (error) {
      if (seq === compilePreviewSeq) {
        state.error = errorMessage(error);
      }
    } finally {
      state.validating = false;
    }
  }

  async function importGraphYAML(yaml: string) {
    state.loading = true;
    state.error = null;
    try {
      const graph = state.offline ? await mockApi.parseGraphYAML(yaml) : await runnerApi.parseGraphYAML(yaml);
      if (state.graph) pushHistory();
      state.graph = graph;
      state.selectedNodeId = graph.nodes[1]?.id || graph.nodes[0]?.id || null;
      state.validation = null;
      state.dryRun = null;
      state.yamlPreview = yaml;
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
      state.dirty = true;
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.loading = false;
    }
  }

  async function submitRun() {
    if (!state.graph) return;
    if (!ensureSemanticChangeReviewed("running")) return;
    closeRunEvents();
    state.submitting = true;
    state.error = null;
    state.run = createInitialRunState();
    try {
      const response = await runnerApi.submitGraphRun(state.graph, { riskAcknowledged: state.riskAcknowledged });
      pushRunEvent({
        type: "run_queued",
        run_id: response.run_id,
        status: response.status,
        timestamp: response.created_at,
        message: `Run ${response.run_id} queued.`,
      });
      rememberLastRunID(response.run_id);
      state.riskAcknowledged = false;
      state.warningAcknowledged = false;
      connectRunEvents(response.run_id);
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.submitting = false;
    }
  }

  async function cancelRun() {
    const runId = state.run.runId;
    if (!runId) return;
    state.canceling = true;
    state.error = null;
    try {
      await runnerApi.cancelRun(runId);
      pushRunEvent({
        type: "cancel_requested",
        run_id: runId,
        status: "canceled",
        message: "Cancel requested.",
        timestamp: new Date().toISOString(),
      });
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.canceling = false;
    }
  }

  async function approveNode(nodeId: string, comment = "") {
    await resolveApprovalNode(nodeId, "approve", comment);
  }

  async function rejectNode(nodeId: string, comment = "") {
    await resolveApprovalNode(nodeId, "reject", comment);
  }

  async function resolveApprovalNode(nodeId: string, action: "approve" | "reject", comment: string) {
    const runId = state.run.runId;
    if (!runId) {
      state.error = "Run ID is required.";
      return;
    }
    const trimmedNodeId = nodeId.trim();
    if (!trimmedNodeId) {
      state.error = "Approval node ID is required.";
      return;
    }
    state.resolvingApprovalNodeId = trimmedNodeId;
    state.resolvingApprovalAction = action;
    state.error = null;
    try {
      if (action === "approve") {
        await runnerApi.approveNode(runId, trimmedNodeId, { actor: "ui", comment });
      } else {
        await runnerApi.rejectNode(runId, trimmedNodeId, { actor: "ui", comment });
      }
      await refreshRunGraph(runId);
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.resolvingApprovalNodeId = null;
      state.resolvingApprovalAction = null;
    }
  }

  function pushRunEvent(event: RunEvent) {
    const eventData = event.output || event.payload || {};
    const nodeID = event.node_id || stringValue(eventData.node_id);
    const status = event.status || stringValue(eventData.status);
    state.run = reduceRunEvent(state.run, event);
    if (event.run_id || state.run.runId) {
      rememberLastRunID(event.run_id || state.run.runId || "");
    }
    if (nodeID && (status === "failed" || event.type.includes("fail"))) {
      state.selectedNodeId = nodeID;
    }
    if (event.type === "run_finish" && state.run.runId) {
      closeRunEvents();
      void refreshRunGraph(state.run.runId);
    }
  }

  async function replayRunHistory(runId: string) {
    runId = runId.trim();
    if (!runId) {
      state.error = "Run ID is required.";
      return;
    }
    closeRunEvents();
    state.replaying = true;
    state.error = null;
    state.run = createInitialRunState();
    try {
      const history = state.offline ? await mockApi.getRunEventHistory(runId) : await runnerApi.getRunEventHistory(runId);
      for (const event of history) {
        pushRunEvent({ ...event, run_id: event.run_id || runId });
      }
      if (!state.run.runId) {
        state.run = { ...state.run, runId };
      }
      rememberLastRunID(runId);
      await refreshRunGraph(runId);
      if (!isTerminalStatus(state.run.status)) {
        connectRunEvents(runId);
      }
    } catch (error) {
      state.error = errorMessage(error);
    } finally {
      state.replaying = false;
    }
  }

  async function refreshRunGraph(runId: string) {
    try {
      if (!state.offline) {
        state.graph = await runnerApi.getRunGraph(runId);
        state.run = mergeRunGraphOverlay(state.run, state.graph);
      }
    } catch {
      // Realtime reducer state is still useful if the overlay endpoint is temporarily unavailable.
    }
  }

  function connectRunEvents(runId: string) {
    state.eventConnected = true;
    unsubscribeRunEvents = runnerApi.subscribeRunEvents(
      runId,
      pushRunEvent,
      () => {
        state.eventConnected = false;
      },
    );
  }

  function closeRunEvents() {
    unsubscribeRunEvents?.();
    unsubscribeRunEvents = null;
    state.eventConnected = false;
  }

  function copySelectedNode() {
    if (!state.graph || !state.selectedNodeId) {
      state.error = "Select a node to copy.";
      return;
    }
    const node = state.graph.nodes.find((item) => item.id === state.selectedNodeId);
    if (!node) {
      state.error = "Selected node does not exist.";
      return;
    }
    if (node.type === "start") {
      state.error = "Start node cannot be copied.";
      return;
    }
    state.clipboardNode = cloneNode(node);
    state.error = null;
  }

  function pasteNode() {
    if (!state.graph || !state.clipboardNode) {
      state.error = "Copy a node before pasting.";
      return;
    }
    const node = duplicateNode(state.clipboardNode, state.graph);
    pushHistory();
    state.graph = {
      ...state.graph,
      nodes: [...state.graph.nodes, node],
    };
    state.selectedNodeId = node.id;
    markDirty();
  }

  function undo() {
    if (!state.graph || state.historyPast.length === 0) return;
    const previous = state.historyPast[state.historyPast.length - 1];
    state.historyPast = state.historyPast.slice(0, -1);
    state.historyFuture = [cloneGraph(state.graph), ...state.historyFuture].slice(0, 50);
    state.graph = previous;
    state.selectedNodeId = previous.nodes.some((node) => node.id === state.selectedNodeId) ? state.selectedNodeId : previous.nodes[0]?.id || null;
    markDirty();
  }

  function redo() {
    if (!state.graph || state.historyFuture.length === 0) return;
    const next = state.historyFuture[0];
    state.historyFuture = state.historyFuture.slice(1);
    state.historyPast = [...state.historyPast, cloneGraph(state.graph)].slice(-50);
    state.graph = next;
    state.selectedNodeId = next.nodes.some((node) => node.id === state.selectedNodeId) ? state.selectedNodeId : next.nodes[0]?.id || null;
    markDirty();
  }

  return {
    state,
    selectedNode,
    graphWithRunState,
    executionSemanticsChanged,
    waitingApprovalNodes,
    load,
    selectNode,
    updateNode,
    updateWorkflow,
    addActionNodeFromCatalog,
    addControlNode,
    connectNodes,
    deleteSelectedNode,
    autoLayout,
    copySelectedNode,
    pasteNode,
    undo,
    redo,
    replaceGraph,
    createWorkflowFromGraph,
    switchWorkflow,
    cloneCurrentWorkflow,
    saveDraft,
    publishWorkflow,
    loadWorkflowVersions,
    rollbackWorkflowVersion,
    exportWorkflowBundle,
    importWorkflowBundle,
    validateGraph,
    dryRunGraph,
    compilePreview,
    importGraphYAML,
    submitRun,
    cancelRun,
    approveNode,
    rejectNode,
    replayRunHistory,
    pushRunEvent,
  };
}

function initialGraphStoreState(): GraphStoreState {
  return {
    graph: null,
    baselineGraph: null,
    actions: [],
    workflowOptions: [],
    workflowVersions: [],
    selectedNodeId: null,
    loading: false,
    saving: false,
    publishing: false,
    loadingVersions: false,
    rollingBack: false,
    creatingWorkflow: false,
    switchingWorkflow: false,
    exportingBundle: false,
    importingBundle: false,
    validating: false,
    dryRunning: false,
    submitting: false,
    canceling: false,
    resolvingApprovalNodeId: null,
    resolvingApprovalAction: null,
    replaying: false,
    previewCompiling: false,
    offline: false,
    dirty: false,
    eventConnected: false,
    error: null,
    saveNote: "",
    riskAcknowledged: false,
    warningAcknowledged: false,
    semanticChangeAcknowledged: false,
    workflowStatus: "draft",
    publishedAt: "",
    validation: null,
    dryRun: null,
    yamlPreview: "",
    run: createInitialRunState(),
    historyPast: [],
    historyFuture: [],
    clipboardNode: null,
  };
}

function parseWorkflowBundle(bundleText: string): WorkflowBundle {
  let parsed: unknown;
  try {
    parsed = JSON.parse(bundleText);
  } catch (error) {
    throw new Error(`Invalid workflow bundle JSON: ${errorMessage(error)}`);
  }
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("Workflow bundle JSON must be an object.");
  }
  const bundle = parsed as Partial<WorkflowBundle>;
  if (!stringValue(bundle.yaml)?.trim()) {
    throw new Error("Workflow bundle yaml is required.");
  }
  if (bundle.versions && !Array.isArray(bundle.versions)) {
    throw new Error("Workflow bundle versions must be an array.");
  }
  return bundle as WorkflowBundle;
}

function errorMessage(error: unknown) {
  return error instanceof Error ? error.message : "Request failed.";
}

function setWorkflowPublishState(summary: WorkflowSummary | undefined) {
  state.workflowStatus = summary?.status || "draft";
  state.publishedAt = summary?.published_at || "";
}

function upsertWorkflowOption(summary: WorkflowSummary) {
  const name = summary.name?.trim() || state.graph?.workflow.name;
  if (!name) return;
  const existing = state.workflowOptions.find((item) => item.name === name);
  const currentWorkflow = state.graph?.workflow.name === name ? state.graph.workflow : undefined;
  const next: WorkflowSummary = {
    ...existing,
    name,
    version: summary.version || currentWorkflow?.version || existing?.version,
    description: summary.description ?? currentWorkflow?.description ?? existing?.description,
    labels: summary.labels || existing?.labels,
    save_note: summary.save_note ?? existing?.save_note,
    status: summary.status || existing?.status || "draft",
    published_at: summary.published_at ?? existing?.published_at,
    created_at: summary.created_at ?? existing?.created_at,
    updated_at: summary.updated_at ?? existing?.updated_at,
  };
  state.workflowOptions = [...state.workflowOptions.filter((item) => item.name !== name), next].sort((a, b) => a.name.localeCompare(b.name));
}

function selectInitialEditableNode(graph: WorkflowGraph | null, preferredNodeId?: string | null) {
  if (!graph) return null;
  if (preferredNodeId && graph.nodes.some((node) => node.id === preferredNodeId)) {
    return preferredNodeId;
  }
  return (
    graph.nodes.find((node) => node.type === "action")?.id ||
    graph.nodes.find((node) => ["manual_approval", "subflow", "condition"].includes(node.type))?.id ||
    graph.nodes.find((node) => node.type !== "start")?.id ||
    graph.nodes[0]?.id ||
    null
  );
}

function mergeWorkflowOptions(options: WorkflowSummary[], created: { name: string; status?: string; workflow?: WorkflowDefinition }) {
  const summary: WorkflowSummary = {
    name: created.name,
    version: created.workflow?.version,
    description: created.workflow?.description,
    status: created.status || "draft",
  };
  const withoutExisting = options.filter((item) => item.name !== created.name);
  return [...withoutExisting, summary].sort((a, b) => a.name.localeCompare(b.name));
}

function hasExecutionSemanticChanges() {
  if (!state.baselineGraph || !state.graph) {
    return false;
  }
  return Boolean(buildGraphDiffSummary(state.baselineGraph, state.graph).sections.find((section) => section.kind === "execution")?.changed);
}

function ensureSemanticChangeReviewed(action: string) {
  if (!hasExecutionSemanticChanges() || state.semanticChangeAcknowledged) {
    return true;
  }
  state.error = `Review and confirm execution semantic changes before ${action}.`;
  return false;
}

function markDirty() {
  state.dirty = true;
  state.workflowStatus = "draft";
  state.riskAcknowledged = false;
  state.warningAcknowledged = false;
  state.semanticChangeAcknowledged = false;
  state.yamlPreview = "";
  state.validation = null;
  state.dryRun = null;
  state.error = null;
  scheduleCompilePreview();
}

function clearEditSession() {
  cancelScheduledCompilePreview();
  state.historyPast = [];
  state.historyFuture = [];
  state.clipboardNode = null;
}

function scheduleCompilePreview() {
  if (compilePreviewTimer) {
    clearTimeout(compilePreviewTimer);
    compilePreviewTimer = null;
  }
  const seq = ++compilePreviewSeq;
  if (!state.graph || state.offline) {
    return;
  }
  const graph = cloneGraph(state.graph);
  compilePreviewTimer = setTimeout(() => {
    compilePreviewTimer = null;
    void refreshCompiledPreview(graph, seq);
  }, autoCompilePreviewDelayMs);
}

async function refreshCompiledPreview(graph: WorkflowGraph, seq: number) {
  state.previewCompiling = true;
  try {
    const result = await runnerApi.compileGraph(graph);
    if (seq === compilePreviewSeq) {
      state.yamlPreview = result.yaml;
    }
  } catch {
    // Automatic preview should never interrupt editing. Explicit compile/dry-run still surfaces errors.
  } finally {
    if (seq === compilePreviewSeq) {
      state.previewCompiling = false;
    }
  }
}

function cancelScheduledCompilePreview() {
  if (compilePreviewTimer) {
    clearTimeout(compilePreviewTimer);
    compilePreviewTimer = null;
  }
  compilePreviewSeq += 1;
  state.previewCompiling = false;
}

function pushHistory() {
  if (!state.graph) return;
  state.historyPast = [...state.historyPast, cloneGraph(state.graph)].slice(-50);
  state.historyFuture = [];
}

function cloneGraph(graph: WorkflowGraph): WorkflowGraph {
  return JSON.parse(JSON.stringify(graph)) as WorkflowGraph;
}

function cloneNode(node: WorkflowNode): WorkflowNode {
  return JSON.parse(JSON.stringify(node)) as WorkflowNode;
}

function duplicateNode(source: WorkflowNode, graph: WorkflowGraph): WorkflowNode {
  const node = cloneNode(source);
  node.id = uniqueGraphName(`${source.id}-copy`, new Set(graph.nodes.map((item) => item.id)));
  node.position = {
    x: source.position.x + 36,
    y: source.position.y + 36,
  };
  delete node.state;

  if (node.step) {
    const baseName = node.step.name || node.step_name || node.id;
    const stepName = uniqueGraphName(`${baseName}-copy`, new Set(graph.nodes.map((item) => item.step?.name || item.step_name || "").filter(Boolean)));
    node.step = {
      ...node.step,
      id: node.id,
      name: stepName,
    };
    node.step_id = node.id;
    node.step_name = stepName;
  }

  if (node.handler) {
    const handlerName = uniqueGraphName(`${node.handler.name || node.handler_name || node.id}-copy`, new Set(graph.nodes.map((item) => item.handler?.name || item.handler_name || "").filter(Boolean)));
    node.handler = {
      ...node.handler,
      name: handlerName,
    };
    node.handler_name = handlerName;
  }

  return node;
}

function uniqueGraphName(base: string, used: Set<string>): string {
  if (!used.has(base)) return base;
  let index = 2;
  while (used.has(`${base}-${index}`)) index += 1;
  return `${base}-${index}`;
}

function isTerminalStatus(status: string) {
  return ["success", "failed", "canceled", "cancelled", "interrupted"].includes(status);
}

function mergeRunGraphOverlay(run: RunState, graph: WorkflowGraph): RunState {
  const nodeStatus = { ...run.nodeStatus };
  for (const node of graph.nodes) {
    if (node.state?.status) {
      nodeStatus[node.id] = node.state.status;
    }
  }
  const edgeStatus = { ...run.edgeStatus };
  for (const edge of graph.edges) {
    if (edge.state?.status) {
      edgeStatus[edge.id] = edge.state.status;
    }
  }
  const activeNodeIds = Object.entries(nodeStatus)
    .filter(([, status]) => status === "running" || status === "waiting")
    .map(([id]) => id);
  const hasWaitingNode = Object.values(nodeStatus).some((status) => status === "waiting");
  const status = run.status === "waiting" && !hasWaitingNode ? "running" : run.status;
  return {
    ...run,
    status,
    nodeStatus,
    edgeStatus,
    activeNodeIds,
  };
}

function rememberLastRunID(runId: string) {
  runId = runId.trim();
  if (!runId) return;
  try {
    globalThis.localStorage?.setItem(lastRunStorageKey, runId);
  } catch {
    // Local storage is optional.
  }
}

function readLastRunID() {
  try {
    return globalThis.localStorage?.getItem(lastRunStorageKey)?.trim() || "";
  } catch {
    return "";
  }
}

function stringValue(value: unknown): string | undefined {
  return typeof value === "string" ? value : undefined;
}

export function __resetGraphStoreForTests(overrides: Partial<GraphStoreState> = {}) {
  unsubscribeRunEvents?.();
  unsubscribeRunEvents = null;
  cancelScheduledCompilePreview();
  Object.assign(state, initialGraphStoreState(), overrides);
}
