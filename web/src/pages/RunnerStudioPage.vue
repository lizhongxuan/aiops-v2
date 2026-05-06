<script setup>
import { computed, onBeforeUnmount, onMounted, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import {
  cancelRunnerStudioRun,
  createRunnerStudioWorkflowGraph,
  dryRunRunnerStudioWorkflowGraph,
  exportRunnerStudioWorkflowBundle,
  getRunnerStudioRunEventHistory,
  getRunnerStudioActionCatalog,
  getRunnerStudioWorkflowGraph,
  importRunnerStudioWorkflowBundle,
  listRunnerStudioWorkflows,
  listRunnerStudioWorkflowVersions,
  parseRunnerStudioWorkflowYaml,
  rollbackRunnerStudioWorkflowVersion,
  runRunnerStudioWorkflowGraph,
  updateRunnerStudioWorkflowGraph,
  validateRunnerStudioWorkflow,
} from "../api/runnerStudioClient";
import RunnerStudioShell from "../components/runner/RunnerStudioShell.vue";
import RunnerVersionHistoryPanel from "../components/runner/RunnerVersionHistoryPanel.vue";
import WorkflowManagerModal from "../components/runner/WorkflowManagerModal.vue";
import {
  createWorkflowManagerState,
  readWorkflowManagerState,
  recordRecentWorkflow,
  toggleFavoriteWorkflow,
  writeWorkflowManagerState,
} from "../components/runner/workflowManagerState";
import { FALLBACK_RUNNER_ACTIONS } from "../components/runner/fallbackActionCatalog";

const loading = ref(false);
const error = ref(null);
const apiNotice = ref(null);
const apiNoticeDismissed = ref(false);
const workflows = ref([]);
const actions = ref([]);
const selectedWorkflowName = ref("");
const workflowUiState = ref(createWorkflowManagerState());
const managerOpen = ref(false);
const dirty = ref(false);
const saveState = ref({ status: "idle", message: "", lastSavedAt: "", error: "" });
const draftConflict = ref(null);
const runEvents = ref([]);
const activeRunId = ref("");
const versionPanelOpen = ref(false);
const versionPanelMode = ref("history");
const versionWorkflowName = ref("");
const versionHistoryItems = ref([]);
const versionPanelLoading = ref(false);
const versionPanelError = ref("");
const versionExportText = ref("");
const route = useRoute();
const router = useRouter();
const AUTOSAVE_DELAY_MS = 2000;
let autosaveTimer = null;
let graphLoadToken = 0;
const serverRequiredToolbarActions = new Set(["save", "validate", "dry-run", "run", "stop-run", "publish"]);
const serverActionsDisabled = computed(() => Boolean(apiNotice.value));
const serverActionsDisabledReason = computed(() => {
  if (!apiNotice.value) return "";
  return [apiNotice.value.message, apiNotice.value.hint].filter(Boolean).join(" ");
});

function routeWorkflowName() {
  const raw = route?.params?.workflowName;
  const value = Array.isArray(raw) ? raw[0] : raw;
  return String(value || "").trim();
}

function pushRunnerRoute(name) {
  const key = String(name || "").trim();
  if (!router?.push) return;
  if (!key) {
    void router.push({ name: "runner-ui" }).catch(() => {});
    return;
  }
  if (routeWorkflowName() !== key || route?.name !== "runner-workflow") {
    void router.push({ name: "runner-workflow", params: { workflowName: key } }).catch(() => {});
  }
}

function normalizeWorkflowList(payload) {
  const items = payload?.workflows || payload?.items || [];
  return Array.isArray(items) ? items : [];
}

function normalizeActionCatalog(payload) {
  const items = payload?.items || payload?.actions || [];
  return Array.isArray(items) ? items : [];
}

async function loadRunnerStudio() {
  loading.value = true;
  error.value = null;
  apiNotice.value = null;
  apiNoticeDismissed.value = false;
  const [workflowResult, catalogResult] = await Promise.allSettled([
    listRunnerStudioWorkflows(),
    getRunnerStudioActionCatalog(),
  ]);
  const failures = [workflowResult, catalogResult]
    .filter((result) => result.status === "rejected")
    .map((result) => result.reason);
  const unrecoverable = failures.find((failure) => !isRecoverableApiFailure(failure));
  if (unrecoverable) {
    error.value = formatRunnerStudioError(unrecoverable);
    loading.value = false;
    return;
  }
  if (workflowResult.status === "fulfilled") {
    workflows.value = normalizeWorkflowList(workflowResult.value);
  } else {
    workflows.value = [];
  }
  if (catalogResult.status === "fulfilled") {
    const catalogActions = normalizeActionCatalog(catalogResult.value);
    actions.value = catalogActions.length ? catalogActions : FALLBACK_RUNNER_ACTIONS;
  } else {
    actions.value = FALLBACK_RUNNER_ACTIONS;
  }
  if (failures.length > 0) {
    apiNotice.value = formatRunnerStudioNotice(failures[0]);
  }
  loading.value = false;
}

function isRecoverableApiFailure(err) {
  const status = Number(err?.status || 0);
  return status === 404 || status === 503;
}

function isMissingWorkflowGraph(err, name) {
  if (Number(err?.status || 0) !== 404) return false;
  const url = String(err?.url || "");
  if (!name || !url) return true;
  return url.includes(`/workflows/${encodeURIComponent(name)}/graph`);
}

function formatRunnerStudioNotice(err) {
  const status = Number(err?.status || 0);
  if (status === 404) {
    return {
      title: "本地编排模式",
      message: "当前 ai-server 尚未接入 /api/runner-studio/*，已启用内置动作库，可先创建和编排工作流。",
      hint: "保存、校验、运行和发布需要重启最新 ai-server；如果重启后变成 503，再配置 Runner API upstream。",
    };
  }
  return {
    title: "本地编排模式",
    message: "Runner API upstream 尚未配置，已启用内置动作库，可先完成工作流草稿。",
    hint: "设置 AIOPS_RUNNER_STUDIO_UPSTREAM_URL、RUNNER_STUDIO_UPSTREAM_URL 或 AIOPS_RUNNER_API_BASE_URL 后重启 ai-server。",
  };
}

function formatRunnerStudioError(err) {
  const status = Number(err?.status || 0);
  const serverMessage = err?.payload?.error || err?.payload?.message || err?.message || "Runner Studio API 暂不可用";
  if (status === 404) {
    return {
      title: "Runner Studio API 未接入主应用",
      message: "当前 ai-server 没有响应 /api/runner-studio/*，通常是服务仍在运行旧二进制或路由没有重新加载。",
      hint: "请重启 ai-server 并确认启动日志来自最新构建；如果重启后变成 503，再配置 Runner API upstream。",
    };
  }
  if (status === 503 && String(serverMessage).includes("upstream")) {
    return {
      title: "Runner Studio 后端未配置",
      message: "主应用已接入 /api/runner-studio/*，但还没有配置 Runner API upstream。",
      hint: "设置 AIOPS_RUNNER_STUDIO_UPSTREAM_URL、RUNNER_STUDIO_UPSTREAM_URL 或 AIOPS_RUNNER_API_BASE_URL 后重启 ai-server。",
    };
  }
  return {
    title: "Runner Studio API 不可用",
    message: serverMessage,
    hint: err?.url ? `请求地址：${err.url}` : "",
  };
}

function persistWorkflowUiState(nextState) {
  workflowUiState.value = writeWorkflowManagerState(nextState);
}

async function selectWorkflow(name, options = {}) {
  const syncRoute = options.syncRoute !== false;
  const key = String(name || "").trim();
  if (!key) {
    if (dirty.value && selectedWorkflowName.value) {
      const flushed = await flushSelectedWorkflowDraft("switch");
      if (!flushed) return;
    }
    selectedWorkflowName.value = "";
    dirty.value = false;
    if (syncRoute) pushRunnerRoute("");
    return;
  }
  if (dirty.value && selectedWorkflowName.value && selectedWorkflowName.value !== key) {
    const flushed = await flushSelectedWorkflowDraft("switch");
    if (!flushed) return;
  }
  selectedWorkflowName.value = key;
  dirty.value = false;
  if (syncRoute) pushRunnerRoute(key);
  persistWorkflowUiState(recordRecentWorkflow(workflowUiState.value, key));
  await ensureWorkflowGraphLoaded(key);
}

function workflowKey(workflow) {
  return workflow?.name || workflow?.id || "";
}

function selectedWorkflowIndex() {
  const key = selectedWorkflowName.value;
  return workflows.value.findIndex((item) => workflowKey(item) === key);
}

function selectedWorkflow() {
  const index = selectedWorkflowIndex();
  return index >= 0 ? workflows.value[index] : null;
}

const versionPanelCurrentYaml = computed(() => {
  const name = versionWorkflowName.value || selectedWorkflowName.value;
  const workflow = workflows.value.find((item) => workflowKey(item) === name);
  if (workflow?.yaml) return String(workflow.yaml);
  if (workflow?.graph) return JSON.stringify(normalizeGraph(workflow.graph, name), null, 2);
  return "";
});

function nextBlankWorkflowName() {
  const used = new Set(workflows.value.map((workflow) => workflowKey(workflow)).filter(Boolean));
  if (!used.has("runner-blank")) return "runner-blank";
  let index = 2;
  while (used.has(`runner-blank-${index}`)) index += 1;
  return `runner-blank-${index}`;
}

function normalizeGraph(graph, name) {
  return {
    version: graph?.version || "v1",
    ...graph,
    workflow: {
      ...(graph?.workflow || {}),
      name: graph?.workflow?.name || name,
    },
    nodes: Array.isArray(graph?.nodes) ? graph.nodes : [],
    edges: Array.isArray(graph?.edges) ? graph.edges : [],
  };
}

function createBlankWorkflowGraph(name) {
  return normalizeGraph(
    {
      workflow: { name },
      nodes: [
        {
          id: "start",
          type: "start",
          label: "Start",
          position: { x: 80, y: 160 },
          ports: [{ id: "next", type: "output", label: "下一步" }],
        },
        {
          id: "end",
          type: "end",
          label: "End",
          position: { x: 720, y: 160 },
          ports: [{ id: "in", type: "input", label: "输入" }],
        },
      ],
      edges: [],
    },
    name,
  );
}

function graphResourceVersion(graph) {
  const uiVersion = graph?.ui?.resource_version || graph?.ui?.resourceVersion;
  const draftVersion = graph?.draft?.resource_version || graph?.draft?.resourceVersion;
  return String(uiVersion || draftVersion || "").trim();
}

function withGraphResourceVersion(graph, version) {
  const resourceVersion = String(version || "").trim();
  if (!resourceVersion) return normalizeGraph(graph, graph?.workflow?.name || selectedWorkflowName.value);
  return {
    ...normalizeGraph(graph, graph?.workflow?.name || selectedWorkflowName.value),
    ui: {
      ...(graph?.ui || {}),
      resource_version: resourceVersion,
    },
  };
}

function formatSaveTime(date = new Date()) {
  return date.toLocaleTimeString("zh-CN", { hour12: false, hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function setSaveState(status, patch = {}) {
  saveState.value = {
    status,
    message: "",
    lastSavedAt: saveState.value.lastSavedAt || "",
    error: "",
    ...patch,
  };
}

function saveNoteForReason(reason) {
  switch (reason) {
    case "dry-run":
      return "autosave before dry-run";
    case "run":
      return "autosave before run";
    case "validate":
      return "autosave before validate";
    case "publish":
      return "autosave before publish";
    case "switch":
      return "autosave before switch";
    case "conflict":
      return "autosave after conflict";
    case "manual":
    case "save":
      return "manual save";
    default:
      return "autosave";
  }
}

function clearAutosaveTimer() {
  if (autosaveTimer) {
    clearTimeout(autosaveTimer);
    autosaveTimer = null;
  }
}

function scheduleAutosave() {
  clearAutosaveTimer();
  if (!selectedWorkflowName.value) return;
  setSaveState("pending", { message: "未保存" });
  autosaveTimer = setTimeout(() => {
    autosaveTimer = null;
    void flushSelectedWorkflowDraft("autosave");
  }, AUTOSAVE_DELAY_MS);
}

function isConflictError(err) {
  return Number(err?.status || 0) === 409 || String(err?.message || "").includes("conflict");
}

function formatAPIError(err) {
  const status = Number(err?.status || 0);
  const serverMessage = err?.payload?.error || err?.payload?.message || err?.message || "请求失败";
  const parts = [];
  if (status) parts.push(`HTTP ${status}`);
  if (serverMessage) parts.push(serverMessage);
  if (err?.url) parts.push(`请求地址：${err.url}`);
  return parts.join(" · ") || "请求失败";
}

function workflowHasUsableGraph(workflow) {
  if (!workflow?.graph) return false;
  if (workflow.local_draft) return true;
  return Boolean(graphResourceVersion(workflow.graph));
}

function ensureWorkflowPlaceholder(name) {
  const key = String(name || "").trim();
  if (!key) return null;
  const existing = workflows.value.find((workflow) => workflowKey(workflow) === key);
  if (existing) return existing;
  const workflow = {
    name: key,
    title: key,
    status: "draft",
    graph: null,
    local_draft: false,
    diff_summary: {},
    risk_summary: { level: "low", items: [] },
    validation_result: { valid: false, errors: [], warnings: [] },
  };
  workflows.value = [workflow, ...workflows.value];
  return workflow;
}

function replaceWorkflow(name, patchOrUpdater) {
  const key = String(name || "").trim();
  if (!key) return null;
  let nextWorkflow = null;
  workflows.value = workflows.value.map((workflow) => {
    if (workflowKey(workflow) !== key) return workflow;
    const patch = typeof patchOrUpdater === "function" ? patchOrUpdater(workflow) : patchOrUpdater;
    nextWorkflow = { ...workflow, ...(patch || {}) };
    return nextWorkflow;
  });
  return nextWorkflow;
}

function upsertWorkflow(name, patch = {}) {
  const key = String(name || "").trim();
  if (!key) return null;
  const existing = workflows.value.find((workflow) => workflowKey(workflow) === key);
  if (!existing) {
    const workflow = {
      name: key,
      title: key,
      status: "draft",
      graph: null,
      local_draft: false,
      diff_summary: {},
      risk_summary: { level: "low", items: [] },
      validation_result: { valid: false, errors: [], warnings: [] },
      ...patch,
    };
    workflows.value = [workflow, ...workflows.value];
    return workflow;
  }
  return replaceWorkflow(key, (current) => ({ ...current, ...patch }));
}

async function ensureWorkflowGraphLoaded(name) {
  const key = String(name || "").trim();
  if (!key) return null;
  const workflow = ensureWorkflowPlaceholder(key);
  if (workflowHasUsableGraph(workflow)) {
    const graph = normalizeGraph(workflow.graph, key);
    replaceWorkflow(key, { graph });
    return graph;
  }
  const loadToken = (graphLoadToken += 1);
  try {
    const payload = await getRunnerStudioWorkflowGraph(key);
    if (loadToken !== graphLoadToken || selectedWorkflowName.value !== key) return null;
    const loadedGraph = normalizeGraph(payload?.graph || payload, key);
    replaceWorkflow(key, {
      graph: loadedGraph,
      local_draft: false,
      status: workflow?.status || "draft",
    });
    if (graphResourceVersion(loadedGraph)) {
      setSaveState("saved", { message: "已加载", lastSavedAt: formatSaveTime() });
    }
    return loadedGraph;
  } catch (err) {
    if (isMissingWorkflowGraph(err, key)) {
      const fallbackGraph = normalizeGraph(workflow?.graph || createBlankWorkflowGraph(key), key);
      replaceWorkflow(key, {
        graph: fallbackGraph,
        local_draft: true,
        status: workflow?.status || "draft",
      });
      setSaveState("idle", { message: "本地草稿" });
      return fallbackGraph;
    }
    if (isRecoverableApiFailure(err)) {
      apiNotice.value = formatRunnerStudioNotice(err);
      const fallbackGraph = normalizeGraph(workflow?.graph || { workflow: { name: key }, nodes: [], edges: [] }, key);
      replaceWorkflow(key, {
        graph: fallbackGraph,
        local_draft: true,
        status: workflow?.status || "draft",
      });
      setSaveState("idle", { message: "本地草稿" });
      return fallbackGraph;
    }
    setSaveState("error", { message: "加载失败", error: formatAPIError(err) });
    return null;
  }
}

function graphForConflictPreview(graph, name) {
  return JSON.stringify(normalizeGraph(graph || { workflow: { name }, nodes: [], edges: [] }, name), null, 2);
}

async function openDraftConflict(name, localGraph, err) {
  let remoteGraph = null;
  try {
    const payload = await getRunnerStudioWorkflowGraph(name);
    remoteGraph = normalizeGraph(payload?.graph || payload, name);
  } catch (_remoteErr) {
    remoteGraph = null;
  }
  draftConflict.value = {
    name,
    message: formatAPIError(err),
    localGraph: normalizeGraph(localGraph, name),
    remoteGraph,
    mergeText: graphForConflictPreview(localGraph, name),
  };
  dirty.value = true;
  setSaveState("conflict", { message: "保存冲突", error: formatAPIError(err) });
}

function closeDraftConflict() {
  draftConflict.value = null;
}

function useRemoteConflictGraph() {
  const conflict = draftConflict.value;
  if (!conflict?.name || !conflict.remoteGraph) return;
  replaceWorkflow(conflict.name, {
    graph: normalizeGraph(conflict.remoteGraph, conflict.name),
    local_draft: false,
    status: "draft",
  });
  dirty.value = false;
  draftConflict.value = null;
  setSaveState("saved", { message: "已采用远端草稿", lastSavedAt: formatSaveTime() });
}

async function retryLocalConflictGraph() {
  const conflict = draftConflict.value;
  if (!conflict?.name) return;
  const remoteVersion = graphResourceVersion(conflict.remoteGraph);
  const graph = withGraphResourceVersion(conflict.localGraph, remoteVersion);
  draftConflict.value = null;
  replaceWorkflow(conflict.name, {
    graph,
    local_draft: false,
    status: "draft",
  });
  dirty.value = true;
  await flushSelectedWorkflowDraft("conflict");
}

async function applyMergedConflictGraph() {
  const conflict = draftConflict.value;
  if (!conflict?.name) return;
  try {
    const parsed = JSON.parse(conflict.mergeText || "{}");
    const remoteVersion = graphResourceVersion(conflict.remoteGraph);
    const graph = graphResourceVersion(parsed)
      ? normalizeGraph(parsed, conflict.name)
      : withGraphResourceVersion(parsed, remoteVersion);
    draftConflict.value = null;
    replaceWorkflow(conflict.name, {
      graph,
      local_draft: false,
      status: "draft",
    });
    dirty.value = true;
    await flushSelectedWorkflowDraft("conflict");
  } catch (err) {
    setSaveState("error", { message: "合并内容不是合法 JSON", error: err?.message || "JSON parse failed" });
  }
}

function toggleWorkflowFavorite(name) {
  persistWorkflowUiState(toggleFavoriteWorkflow(workflowUiState.value, name));
}

function openCreateWorkflow() {
  managerOpen.value = true;
}

function createBlankWorkflow() {
  const name = nextBlankWorkflowName();
  const workflow = {
    name,
    title: name,
    status: "draft",
    graph: createBlankWorkflowGraph(name),
    local_draft: true,
    diff_summary: {},
    risk_summary: { level: "low", items: [] },
    validation_result: { valid: false, errors: [], warnings: [] },
  };
  workflows.value = [workflow, ...workflows.value];
  void selectWorkflow(name);
  managerOpen.value = false;
}

function handleCreateWorkflow(mode) {
  if (mode === "blank") {
    createBlankWorkflow();
    return;
  }
  if (mode === "yaml") {
    openImportWorkflow();
    return;
  }
  managerOpen.value = true;
}

function updateSelectedWorkflowGraph(graph) {
  const workflow = selectedWorkflow();
  if (!workflow) return;
  const name = workflowKey(workflow);
  const currentVersion = graphResourceVersion(workflow.graph);
  const nextGraph = graphResourceVersion(graph) || !currentVersion
    ? normalizeGraph(graph, name)
    : withGraphResourceVersion(graph, currentVersion);
  replaceWorkflow(name, (current) => ({
    graph: nextGraph,
    local_draft: Boolean(current.local_draft),
    status: "draft",
    validated_graph_hash: "",
    validated_at: "",
    dry_run_graph_hash: "",
    dry_run_at: "",
    validation_result: { valid: false, errors: [], warnings: [] },
  }));
  dirty.value = true;
  scheduleAutosave();
}

async function flushSelectedWorkflowDraft(reason = "save") {
  const workflow = selectedWorkflow();
  if (!workflow) return null;
  clearAutosaveTimer();
  const name = workflowKey(workflow);
  const graph = normalizeGraph(workflow.graph, name);
  const saveNote = saveNoteForReason(reason);
  setSaveState("saving", { message: "正在保存" });
  try {
    const saved = workflow.local_draft
      ? await createRunnerStudioWorkflowGraph({ graph, save_note: saveNote })
      : await updateRunnerStudioWorkflowGraph(name, { graph, save_note: saveNote });
    const savedWorkflow = saved?.data || saved || {};
    const savedGraph = normalizeGraph(savedWorkflow.graph || graph, name);
    replaceWorkflow(name, (current) => ({
      ...current,
      ...savedWorkflow,
      graph: savedGraph,
      local_draft: false,
      status: savedWorkflow.status || current.status || "draft",
    }));
    dirty.value = false;
    setSaveState("saved", { message: "已保存", lastSavedAt: formatSaveTime() });
    return savedGraph;
  } catch (err) {
    if (isConflictError(err)) {
      await openDraftConflict(name, graph, err);
      return null;
    }
    setSaveState("error", { message: "保存失败", error: formatAPIError(err) });
    return null;
  }
}

async function validateSelectedWorkflow() {
  const workflow = selectedWorkflow();
  if (!workflow) return;
  const name = workflowKey(workflow);
  setSaveState("saving", { message: "正在校验" });
  try {
    const graph = await flushSelectedWorkflowDraft("validate");
    if (!graph) return;
    const result = await validateRunnerStudioWorkflow(name);
    const validation = result?.data || result || {};
    replaceWorkflow(name, {
      status: validation.status || (validation.valid ? "validated" : "draft"),
      validated_graph_hash: validation.validated_graph_hash || validation.validatedGraphHash || "",
      validated_at: validation.validated_at || validation.validatedAt || "",
      dry_run_graph_hash: "",
      dry_run_at: "",
      validation_result: validation,
    });
    dirty.value = false;
    setSaveState("saved", { message: "校验通过", lastSavedAt: formatSaveTime() });
  } catch (err) {
    setSaveState("error", { message: "校验失败", error: formatAPIError(err) });
  }
}

async function dryRunSelectedWorkflow() {
  const workflow = selectedWorkflow();
  if (!workflow) return;
  const name = workflowKey(workflow);
  setSaveState("saving", { message: "正在 Dry Run" });
  try {
    const graph = await flushSelectedWorkflowDraft("dry-run");
    if (!graph) return;
    const validationResponse = await validateRunnerStudioWorkflow(name);
    const validation = validationResponse?.data || validationResponse || {};
    const dryRunResponse = await dryRunRunnerStudioWorkflowGraph({ workflow_name: name, graph, vars: {}, triggered_by: "ui" });
    const dryRun = dryRunResponse?.data || dryRunResponse || {};
    replaceWorkflow(name, {
      status: dryRun.status || "dry_run_passed",
      validated_graph_hash: dryRun.validated_graph_hash || validation.validated_graph_hash || validation.validatedGraphHash || "",
      validated_at: validation.validated_at || validation.validatedAt || "",
      dry_run_graph_hash:
        dryRun.dry_run_graph_hash ||
        dryRun.dryRunGraphHash ||
        dryRun.validated_graph_hash ||
        validation.validated_graph_hash ||
        validation.validatedGraphHash ||
        "",
      dry_run_at: dryRun.dry_run_at || dryRun.dryRunAt || "",
      validation_result: validation,
    });
    dirty.value = false;
    setSaveState("saved", { message: "Dry Run 通过", lastSavedAt: formatSaveTime() });
  } catch (err) {
    setSaveState("error", { message: "Dry Run 失败", error: formatAPIError(err) });
  }
}

function makeRunIdempotencyKey(name, nodeId = "") {
  const suffix = nodeId ? `${nodeId}-` : "";
  return `${name}-${suffix}run-${Date.now()}`;
}

async function replayRunHistory(runId) {
  if (!runId) return;
  const history = await getRunnerStudioRunEventHistory(runId);
  const items = Array.isArray(history) ? history : history?.items || history?.events || [];
  if (items.length) {
    runEvents.value = items.map((event) => ({ ...event, run_id: event.run_id || event.runId || runId }));
  }
}

async function submitSelectedWorkflowRun(options = {}) {
  const workflow = selectedWorkflow();
  if (!workflow) return;
  const name = workflowKey(workflow);
  const nodeId = String(options.nodeId || "").trim();
  setSaveState("saving", { message: nodeId ? "正在运行节点" : "正在运行" });
  try {
    const graph = await flushSelectedWorkflowDraft("run");
    if (!graph) return;
    const payload = {
      workflow_name: name,
      graph,
      vars: {},
      triggered_by: "ui",
      risk_acknowledged: true,
      idempotency_key: makeRunIdempotencyKey(name, nodeId),
      ...(nodeId ? { node_id: nodeId, run_scope: "single_node" } : {}),
    };
    const response = await runRunnerStudioWorkflowGraph(payload);
    const run = response?.data || response || {};
    const runId = run.run_id || run.runId || "";
    activeRunId.value = runId;
    runEvents.value = [
      {
        type: "run.started",
        run_id: runId,
        status: "running",
        message: nodeId ? `single node ${nodeId} run started` : "workflow run started",
        ts: new Date().toISOString(),
      },
    ];
    await replayRunHistory(runId);
    setSaveState("saved", { message: "运行已提交", lastSavedAt: formatSaveTime() });
  } catch (err) {
    setSaveState("error", { message: "运行失败", error: formatAPIError(err) });
  }
}

async function cancelSelectedRun() {
  if (!activeRunId.value) return;
  try {
    await cancelRunnerStudioRun(activeRunId.value);
    runEvents.value = [
      ...runEvents.value,
      {
        type: "run.cancelled",
        run_id: activeRunId.value,
        status: "canceled",
        message: "cancel requested",
        ts: new Date().toISOString(),
      },
    ];
    activeRunId.value = "";
    setSaveState("saved", { message: "已停止运行", lastSavedAt: formatSaveTime() });
  } catch (err) {
    setSaveState("error", { message: "停止运行失败", error: formatAPIError(err) });
  }
}

function handleToolbarAction(actionKey) {
  if (serverRequiredToolbarActions.has(actionKey) && serverActionsDisabled.value) {
    setSaveState("error", {
      message: "Runner Studio API 不可用",
      error: serverActionsDisabledReason.value,
    });
    return;
  }
  if (actionKey === "save") {
    void flushSelectedWorkflowDraft("manual");
  }
  if (actionKey === "validate") {
    void validateSelectedWorkflow();
  }
  if (actionKey === "dry-run") {
    void dryRunSelectedWorkflow();
  }
  if (actionKey === "run") {
    void submitSelectedWorkflowRun();
  }
  if (actionKey === "stop-run") {
    void cancelSelectedRun();
  }
  if (actionKey === "publish") {
    void flushSelectedWorkflowDraft("publish");
  }
}

function handleNodeAction(action, nodeId) {
  if (action === "run-node") {
    if (serverActionsDisabled.value) {
      setSaveState("error", {
        message: "Runner Studio API 不可用",
        error: serverActionsDisabledReason.value,
      });
      return;
    }
    void submitSelectedWorkflowRun({ nodeId });
  }
}

function handleWorkflowPublished(payload) {
  const published = payload?.data || payload || {};
  const name = published.name || selectedWorkflowName.value;
  replaceWorkflow(name, (workflow) => ({
    ...workflow,
    ...published,
    status: published.status || "published",
  }));
  dirty.value = false;
}

function handleDirtyConfirm(name) {
  void selectWorkflow(name);
}

function normalizeVersionItems(payload) {
  const items = payload?.items || payload?.versions || payload || [];
  return Array.isArray(items) ? items : [];
}

async function loadWorkflowVersionHistory(name) {
  const key = String(name || "").trim();
  if (!key) return;
  versionPanelLoading.value = true;
  versionPanelError.value = "";
  try {
    const payload = await listRunnerStudioWorkflowVersions(key);
    versionHistoryItems.value = normalizeVersionItems(payload);
  } catch (err) {
    versionPanelError.value = formatAPIError(err);
  } finally {
    versionPanelLoading.value = false;
  }
}

function openImportWorkflow() {
  managerOpen.value = false;
  versionPanelMode.value = "import";
  versionWorkflowName.value = "";
  versionHistoryItems.value = [];
  versionExportText.value = "";
  versionPanelError.value = "";
  versionPanelOpen.value = true;
}

async function openVersionHistory(name) {
  const key = String(name || selectedWorkflowName.value || "").trim();
  if (!key) return;
  managerOpen.value = false;
  versionPanelMode.value = "history";
  versionWorkflowName.value = key;
  versionExportText.value = "";
  versionPanelOpen.value = true;
  await loadWorkflowVersionHistory(key);
}

async function exportWorkflowBundle(name) {
  const key = String(name || versionWorkflowName.value || "").trim();
  if (!key) return;
  versionPanelLoading.value = true;
  versionPanelError.value = "";
  try {
    const bundle = await exportRunnerStudioWorkflowBundle(key);
    versionExportText.value = JSON.stringify(bundle?.data || bundle, null, 2);
  } catch (err) {
    versionPanelError.value = formatAPIError(err);
  } finally {
    versionPanelLoading.value = false;
  }
}

async function rollbackWorkflowVersion(versionId) {
  const key = String(versionWorkflowName.value || "").trim();
  const target = String(versionId || "").trim();
  if (!key || !target) return;
  versionPanelLoading.value = true;
  versionPanelError.value = "";
  try {
    const rolledBack = await rollbackRunnerStudioWorkflowVersion(key, target, {
      save_note: "rollback from Runner Studio",
    });
    const graphPayload = await getRunnerStudioWorkflowGraph(key);
    const graph = normalizeGraph(graphPayload?.graph || graphPayload, key);
    upsertWorkflow(key, {
      ...(rolledBack?.data || rolledBack || {}),
      graph,
      local_draft: false,
      status: rolledBack?.status || rolledBack?.data?.status || "draft",
    });
    dirty.value = false;
    setSaveState("saved", { message: "已恢复版本", lastSavedAt: formatSaveTime() });
    await selectWorkflow(key);
    await loadWorkflowVersionHistory(key);
  } catch (err) {
    versionPanelError.value = formatAPIError(err);
  } finally {
    versionPanelLoading.value = false;
  }
}

function parseImportJSON(text, label) {
  try {
    return JSON.parse(text);
  } catch (err) {
    throw new Error(`${label} 不是合法 JSON：${err?.message || "parse failed"}`);
  }
}

async function importWorkflow(payload = {}) {
  const mode = String(payload.mode || "bundle");
  const text = String(payload.text || "").trim();
  if (!text) return;
  versionPanelLoading.value = true;
  versionPanelError.value = "";
  try {
    let imported = null;
    if (mode === "bundle") {
      const bundle = parseImportJSON(text, "Bundle");
      imported = await importRunnerStudioWorkflowBundle({
        bundle,
        overwrite: Boolean(payload.overwrite),
        save_note: "imported bundle from Runner Studio",
      });
    } else {
      const graph = mode === "graph" ? parseImportJSON(text, "Graph") : await parseRunnerStudioWorkflowYaml({ yaml: text });
      imported = await createRunnerStudioWorkflowGraph({
        graph,
        save_note: mode === "graph" ? "imported graph from Runner Studio" : "imported yaml from Runner Studio",
      });
    }
    const importedWorkflow = imported?.data || imported || {};
    const name = importedWorkflow.name || importedWorkflow.graph?.workflow?.name || importedWorkflow.workflow?.name || "";
    if (!name) {
      throw new Error("导入结果缺少 workflow name");
    }
    let graph = importedWorkflow.graph ? normalizeGraph(importedWorkflow.graph, name) : null;
    if (!graph) {
      const graphPayload = await getRunnerStudioWorkflowGraph(name);
      graph = normalizeGraph(graphPayload?.graph || graphPayload, name);
    }
    upsertWorkflow(name, {
      ...importedWorkflow,
      name,
      title: importedWorkflow.title || name,
      graph,
      local_draft: false,
      status: "draft",
    });
    versionPanelOpen.value = false;
    dirty.value = false;
    setSaveState("saved", { message: "已导入 draft", lastSavedAt: formatSaveTime() });
    await selectWorkflow(name);
  } catch (err) {
    versionPanelError.value = err?.message || formatAPIError(err);
  } finally {
    versionPanelLoading.value = false;
  }
}

function flushDraftForLifecycle() {
  if (!dirty.value || !selectedWorkflowName.value) return;
  void flushSelectedWorkflowDraft("autosave");
}

function handleVisibilityChange() {
  if (document.visibilityState === "hidden") {
    flushDraftForLifecycle();
  }
}

async function initializeRunnerStudioPage() {
  workflowUiState.value = readWorkflowManagerState();
  await loadRunnerStudio();
  const routedWorkflowName = routeWorkflowName();
  if (routedWorkflowName) {
    await selectWorkflow(routedWorkflowName, { syncRoute: false });
  }
}

watch(
  () => routeWorkflowName(),
  (name) => {
    if (name !== selectedWorkflowName.value) {
      selectWorkflow(name, { syncRoute: false });
    }
  },
);

onMounted(() => {
  document.addEventListener("visibilitychange", handleVisibilityChange);
  window.addEventListener("pagehide", flushDraftForLifecycle);
  window.addEventListener("beforeunload", flushDraftForLifecycle);
  void initializeRunnerStudioPage();
});

onBeforeUnmount(() => {
  document.removeEventListener("visibilitychange", handleVisibilityChange);
  window.removeEventListener("pagehide", flushDraftForLifecycle);
  window.removeEventListener("beforeunload", flushDraftForLifecycle);
  clearAutosaveTimer();
  flushDraftForLifecycle();
});
</script>

<template>
  <main class="runner-studio-page" data-testid="runner-studio-page">
    <section v-if="error" class="runner-studio-error" role="status" data-testid="runner-studio-error">
      <strong>{{ error.title }}</strong>
      <span>{{ error.message }}</span>
      <small v-if="error.hint">{{ error.hint }}</small>
    </section>
    <section
      v-else-if="apiNotice && !apiNoticeDismissed"
      class="runner-studio-api-notice"
      role="status"
      data-testid="runner-studio-api-notice"
    >
      <div class="runner-studio-api-notice-copy">
        <strong>{{ apiNotice.title }}</strong>
        <span>{{ apiNotice.message }}</span>
        <small v-if="apiNotice.hint">{{ apiNotice.hint }}</small>
      </div>
      <button
        type="button"
        class="runner-api-notice-close"
        data-testid="runner-api-notice-close"
        aria-label="关闭本地编排提示"
        @click="apiNoticeDismissed = true"
      >
        关闭
      </button>
    </section>

    <RunnerStudioShell
      :selected-workflow-name="selectedWorkflowName"
      :workflows="workflows"
      :actions="actions"
      :loading="loading"
      :workflow-ui-state="workflowUiState"
      :run-events="runEvents"
      :save-state="saveState"
      :server-actions-disabled="serverActionsDisabled"
      :server-actions-disabled-reason="serverActionsDisabledReason"
      @update:selected-workflow-name="selectWorkflow"
      @toggle-workflow-favorite="toggleWorkflowFavorite"
      @open-workflow-manager="managerOpen = true"
      @create-workflow="openCreateWorkflow"
      @update-workflow-graph="updateSelectedWorkflowGraph"
      @node-action="handleNodeAction"
      @toolbar-action="handleToolbarAction"
      @workflow-published="handleWorkflowPublished"
    />

    <WorkflowManagerModal
      :show="managerOpen"
      :workflows="workflows"
      :selected-workflow-name="selectedWorkflowName"
      :ui-state="workflowUiState"
      :dirty="dirty"
      @close="managerOpen = false"
      @select="selectWorkflow"
      @request-dirty-confirm="handleDirtyConfirm"
      @toggle-favorite="toggleWorkflowFavorite"
      @create-workflow="handleCreateWorkflow"
      @clone-workflow="handleCreateWorkflow"
      @archive-workflow="handleCreateWorkflow"
      @view-versions="openVersionHistory"
    />

    <RunnerVersionHistoryPanel
      :show="versionPanelOpen"
      :workflow-name="versionWorkflowName"
      :current-yaml="versionPanelCurrentYaml"
      :versions="versionHistoryItems"
      :loading="versionPanelLoading"
      :error="versionPanelError"
      :export-text="versionExportText"
      :import-only="versionPanelMode === 'import'"
      @close="versionPanelOpen = false"
      @refresh="loadWorkflowVersionHistory"
      @rollback="rollbackWorkflowVersion"
      @export-bundle="exportWorkflowBundle"
      @import-workflow="importWorkflow"
    />

    <section
      v-if="draftConflict"
      class="runner-draft-conflict-backdrop"
      role="dialog"
      aria-modal="true"
      aria-label="草稿保存冲突"
      data-testid="runner-draft-conflict-modal"
      @click.self="closeDraftConflict"
    >
      <div class="runner-draft-conflict-modal">
        <header>
          <div>
            <p>资源版本冲突</p>
            <h2>草稿保存冲突</h2>
            <span>{{ draftConflict.message }}</span>
          </div>
          <button type="button" data-testid="runner-conflict-close" @click="closeDraftConflict">关闭</button>
        </header>
        <div class="runner-draft-conflict-body">
          <section class="runner-draft-conflict-diff">
            <article class="runner-draft-conflict-column">
              <strong>本地草稿</strong>
              <pre>{{ graphForConflictPreview(draftConflict.localGraph, draftConflict.name) }}</pre>
            </article>
            <article class="runner-draft-conflict-column">
              <strong>远端草稿</strong>
              <pre>{{ draftConflict.remoteGraph ? graphForConflictPreview(draftConflict.remoteGraph, draftConflict.name) : "远端草稿读取失败，请稍后重试。" }}</pre>
            </article>
          </section>
          <label class="runner-draft-conflict-merge">
            <span>合并后的 graph JSON</span>
            <textarea
              v-model="draftConflict.mergeText"
              spellcheck="false"
              data-testid="runner-conflict-merge-text"
            />
          </label>
        </div>
        <footer class="runner-draft-conflict-footer">
          <button type="button" data-testid="runner-conflict-use-remote" :disabled="!draftConflict.remoteGraph" @click="useRemoteConflictGraph">
            使用远端草稿
          </button>
          <button type="button" data-testid="runner-conflict-apply-merge" @click="applyMergedConflictGraph">
            保存合并结果
          </button>
          <button type="button" class="primary" data-testid="runner-conflict-keep-local" @click="retryLocalConflictGraph">
            以本地覆盖保存
          </button>
        </footer>
      </div>
    </section>
  </main>
</template>
