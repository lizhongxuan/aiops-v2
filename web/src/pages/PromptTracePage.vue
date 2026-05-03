<script setup>
import { computed, onMounted, ref, watch } from "vue";
import { useRoute } from "vue-router";
import { useMessage } from "naive-ui";
import {
  CopyIcon,
  RefreshCwIcon,
  SearchIcon,
} from "lucide-vue-next";
import { fetchPromptTraceFile, fetchPromptTraces } from "../api/promptTraces";
import PromptTraceLayerCards from "../components/prompt-trace/PromptTraceLayerCards.vue";
import PromptTraceMessages from "../components/prompt-trace/PromptTraceMessages.vue";
import PromptTraceRawViewer from "../components/prompt-trace/PromptTraceRawViewer.vue";
import PromptTraceSummary from "../components/prompt-trace/PromptTraceSummary.vue";
import PromptTraceTools from "../components/prompt-trace/PromptTraceTools.vue";
import { parsePromptTrace } from "../utils/promptTraceViewModel";

const message = useMessage();
const route = useRoute();
const loading = ref(false);
const fileLoading = ref(false);
const traces = ref([]);
const rootDir = ref("");
const query = ref("");
const selectedId = ref("");
const activeView = ref("overview");
const activeRaw = ref("markdown");
const fileCache = ref({});
const fileError = ref("");

const filteredTraces = computed(() => {
  const needle = query.value.trim().toLowerCase();
  if (!needle) return traces.value;
  return traces.value.filter((item) =>
    [
      item.sessionId,
      item.caseId,
      item.turnId,
      item.relativePath,
      item.kind,
      item.promptFingerprint?.stableHash,
      item.promptFingerprint?.developerHash,
    ]
      .filter(Boolean)
      .some((value) => String(value).toLowerCase().includes(needle)),
  );
});

const selectedTrace = computed(() => traces.value.find((item) => item.id === selectedId.value) || null);

const viewOptions = [
  { key: "overview", label: "概览" },
  { key: "layers", label: "Prompt 层" },
  { key: "messages", label: "Messages" },
  { key: "tools", label: "Tools" },
  { key: "diff", label: "Diff" },
  { key: "raw", label: "Raw" },
];

const rawFileOptions = computed(() => {
  const trace = selectedTrace.value;
  if (!trace) return [];
  return [
    { key: "markdown", label: "Markdown", path: trace.markdownPath },
    { key: "json", label: "JSON", path: trace.jsonPath },
  ].filter((item) => item.path);
});

const rawActiveOption = computed(() => {
  const options = rawFileOptions.value;
  return options.find((item) => item.key === activeRaw.value) || options[0] || null;
});

const activePath = computed(() => {
  const trace = selectedTrace.value;
  if (!trace) return "";
  if (activeView.value === "raw") return rawActiveOption.value?.path || "";
  if (activeView.value === "diff") return trace.diffPath || "";
  return trace.jsonPath || "";
});

const jsonContent = computed(() => {
  const path = selectedTrace.value?.jsonPath || "";
  return path ? fileCache.value[path]?.content || "" : "";
});

const rawContent = computed(() => {
  const path = rawActiveOption.value?.path || "";
  return path ? fileCache.value[path]?.content || "" : "";
});

const diffContent = computed(() => {
  const path = selectedTrace.value?.diffPath || "";
  return path ? fileCache.value[path]?.content || "" : "";
});

const traceViewModel = computed(() => (jsonContent.value ? parsePromptTrace(jsonContent.value) : null));

const displayTraceViewModel = computed(() => {
  const vm = traceViewModel.value;
  if (!vm) return null;
  const iteration = Number(selectedTrace.value?.iteration);
  if (!Number.isFinite(iteration) || iteration <= 0 || selectedTrace.value?.diffPath) return vm;
  return {
    ...vm,
    warnings: [
      ...(vm.warnings || []),
      {
        severity: "info",
        message: "非首轮模型调用没有 diff 文件。",
        targetId: "",
      },
    ],
  };
});

const selectedFingerprint = computed(() => selectedTrace.value?.promptFingerprint || {});

function shortHash(value = "") {
  const text = String(value || "");
  if (text.length <= 16) return text;
  return `${text.slice(0, 8)}...${text.slice(-6)}`;
}

function displayTime(value = "") {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return date.toLocaleString();
}

function selectTrace(item) {
  selectedId.value = item?.id || "";
}

function routeTraceQuery() {
  return typeof route.query.trace === "string" ? route.query.trace : "";
}

function routeCaseQuery() {
  return typeof route.query.caseId === "string" ? route.query.caseId : "";
}

async function loadTraces({ keepSelection = true } = {}) {
  loading.value = true;
  fileError.value = "";
  try {
    const data = await fetchPromptTraces({ limit: 150, caseId: routeCaseQuery(), trace: routeTraceQuery() });
    traces.value = Array.isArray(data?.traces) ? data.traces : [];
    rootDir.value = data?.rootDir || "";
    const selectedStillExists = traces.value.some((item) => item.id === selectedId.value);
    const routeSelected = data?.selectedId || findTraceSelection(routeTraceQuery()) || findCaseSelection(routeCaseQuery());
    if (routeSelected) {
      selectedId.value = routeSelected;
      return;
    }
    if (!keepSelection || !selectedStillExists) {
      selectedId.value = traces.value[0]?.id || "";
    }
  } catch (error) {
    traces.value = [];
    message.error(error?.message || "Prompt trace 加载失败");
  } finally {
    loading.value = false;
  }
}

function findTraceSelection(tracePath = "") {
  const needle = String(tracePath || "");
  if (!needle) return "";
  return traces.value.find((item) =>
    [item.id, item.relativePath, item.jsonPath, item.markdownPath, item.diffPath].some((value) => String(value || "") === needle),
  )?.id || "";
}

function findCaseSelection(caseId = "") {
  const needle = String(caseId || "").toLowerCase();
  if (!needle) return "";
  return traces.value.find((item) => String(item.caseId || "").toLowerCase() === needle)?.id || "";
}

async function loadFile(path) {
  if (!path || fileCache.value[path]?.content) return;
  fileLoading.value = true;
  fileError.value = "";
  try {
    const data = await fetchPromptTraceFile(path);
    fileCache.value = {
      ...fileCache.value,
      [path]: data,
    };
  } catch (error) {
    fileError.value = error?.message || "Prompt trace 文件读取失败";
  } finally {
    fileLoading.value = false;
  }
}

async function loadActiveViewFile() {
  const trace = selectedTrace.value;
  if (!trace) return;
  await loadFile(activePath.value);
}

async function copyActiveContent() {
  const content = activeView.value === "raw" ? rawContent.value : activeView.value === "diff" ? diffContent.value : copyableViewContent();
  if (!content) return;
  if ((activeView.value === "raw" || content.length > 8000) && !window.confirm("完整 prompt 可能包含本地路径、命令输出或敏感信息，只在本地调试中使用。确认复制？")) {
    return;
  }
  try {
    await navigator.clipboard.writeText(content);
    message.success("已复制当前 prompt trace");
  } catch {
    message.error("复制失败");
  }
}

async function copyPayload(payload = {}) {
  const value = payload.value || "";
  if (!value) return;
  try {
    await navigator.clipboard.writeText(value);
    message.success(`已复制 ${payload.label || "内容"}`);
  } catch {
    message.error("复制失败");
  }
}

function copyableViewContent() {
  const vm = displayTraceViewModel.value;
  if (!vm) return "";
  if (activeView.value === "overview") {
    return JSON.stringify(
      {
        summary: vm.summary,
        fingerprints: vm.fingerprints,
        visibleTools: vm.tools.visible,
        warnings: vm.warnings,
      },
      null,
      2,
    );
  }
  if (activeView.value === "layers") {
    return vm.layers.map((item) => `# ${item.index} ${item.title}\n${item.content}`).join("\n\n");
  }
  if (activeView.value === "messages") {
    return vm.messages.map((item) => `# ${item.index} ${item.providerRole}/${item.semanticRole}/${item.promptLayer}\n${item.content}`).join("\n\n");
  }
  if (activeView.value === "tools") {
    return [`Visible tools: ${(vm.tools.visible || []).join(", ")}`, vm.tools.registryText].filter(Boolean).join("\n\n");
  }
  return "";
}

function handleWarningTarget(item = {}) {
  if (item.targetId) {
    activeView.value = "layers";
    return;
  }
  activeView.value = "overview";
}

watch(selectedTrace, () => {
  activeView.value = "overview";
  activeRaw.value = rawFileOptions.value[0]?.key || "markdown";
  fileError.value = "";
  void loadActiveViewFile();
});

watch(activePath, () => {
  void loadActiveViewFile();
});

onMounted(() => {
  if (routeCaseQuery() && !query.value) {
    query.value = routeCaseQuery();
  }
  void loadTraces({ keepSelection: false });
});

watch(
  () => [route.query.trace, route.query.caseId],
  () => {
    if (routeCaseQuery()) query.value = routeCaseQuery();
    void loadTraces({ keepSelection: false });
  },
);
</script>

<template>
  <section class="prompt-trace-page">
    <header class="prompt-trace-header">
      <div>
        <div class="prompt-trace-kicker">Debug / Prompt Trace</div>
        <h2>Prompt Trace</h2>
      </div>
      <div class="prompt-trace-actions">
        <n-button secondary :loading="loading" @click="loadTraces()">
          <template #icon><RefreshCwIcon size="16" /></template>
          刷新
        </n-button>
      </div>
    </header>

    <div class="prompt-trace-shell">
      <aside class="prompt-trace-sidebar">
        <div class="prompt-trace-search">
          <n-input v-model:value="query" clearable placeholder="Session / Turn / Hash">
            <template #prefix><SearchIcon size="15" /></template>
          </n-input>
        </div>
        <div class="prompt-trace-root" :title="rootDir">{{ rootDir || ".data/model-input-traces" }}</div>
        <n-spin class="prompt-trace-list-spin" :show="loading">
          <div v-if="filteredTraces.length" class="prompt-trace-list">
            <button
              v-for="item in filteredTraces"
              :key="item.id"
              class="prompt-trace-item"
              :class="{ 'is-active': item.id === selectedId }"
              @click="selectTrace(item)"
            >
              <span class="prompt-trace-item-main">
                <span class="prompt-trace-session">{{ item.sessionId || "session" }}</span>
                <span class="prompt-trace-meta">turn {{ item.turnId || "-" }}</span>
              </span>
              <span v-if="item.caseId" class="prompt-trace-case">case {{ item.caseId }}</span>
              <span class="prompt-trace-item-sub">
                <span>iteration {{ item.iteration }}</span>
                <span>{{ item.messageCount || 0 }} messages</span>
                <span>{{ (item.visibleTools || []).length }} tools</span>
              </span>
              <span class="prompt-trace-item-time">{{ displayTime(item.createdAt || item.modifiedAt) }}</span>
            </button>
          </div>
          <n-empty v-else class="prompt-trace-empty" description="暂无 prompt trace" />
        </n-spin>
      </aside>

      <main class="prompt-trace-viewer">
        <div v-if="selectedTrace" class="prompt-trace-detail">
          <div class="prompt-trace-detail-header">
            <div>
              <div class="prompt-trace-path" :title="activePath">{{ activePath }}</div>
              <div v-if="selectedTrace.caseId" class="prompt-trace-case-detail">case {{ selectedTrace.caseId }}</div>
              <div class="prompt-trace-fingerprints">
                <span v-if="selectedFingerprint.stableHash">stable {{ shortHash(selectedFingerprint.stableHash) }}</span>
                <span v-if="selectedFingerprint.developerHash">developer {{ shortHash(selectedFingerprint.developerHash) }}</span>
                <span v-if="selectedFingerprint.toolRegistryHash">tools {{ shortHash(selectedFingerprint.toolRegistryHash) }}</span>
              </div>
            </div>
            <n-button secondary :disabled="!selectedTrace" @click="copyActiveContent">
              <template #icon><CopyIcon size="16" /></template>
              复制
            </n-button>
          </div>

          <div class="prompt-trace-tabs">
            <button
              v-for="option in viewOptions"
              :key="option.key"
              class="prompt-trace-tab"
              :class="{ 'is-active': option.key === activeView }"
              @click="activeView = option.key"
            >
              {{ option.label }}
            </button>
          </div>

          <n-spin class="prompt-trace-spin" :show="fileLoading">
            <n-alert v-if="fileError" type="error" class="prompt-trace-alert">
              {{ fileError }}
            </n-alert>
            <div v-else class="prompt-trace-content">
              <PromptTraceSummary
                v-if="activeView === 'overview' && displayTraceViewModel"
                :view-model="displayTraceViewModel"
                @copy="copyPayload"
                @select-target="handleWarningTarget"
              />
              <PromptTraceLayerCards
                v-else-if="activeView === 'layers' && displayTraceViewModel"
                :layers="displayTraceViewModel.layers"
                @copy="copyPayload"
              />
              <PromptTraceMessages
                v-else-if="activeView === 'messages' && displayTraceViewModel"
                :messages="displayTraceViewModel.messages"
                @copy="copyPayload"
              />
              <PromptTraceTools
                v-else-if="activeView === 'tools' && displayTraceViewModel"
                :tools="displayTraceViewModel.tools"
                @copy="copyPayload"
              />
              <PromptTraceRawViewer
                v-else-if="activeView === 'diff'"
                :options="[{ key: 'diff', label: 'Diff' }]"
                active="diff"
                :content="selectedTrace.diffPath ? diffContent : '首轮模型调用没有 diff，这是正常情况。'"
                :loading="fileLoading"
                :error="fileError"
                @copy="copyActiveContent"
              />
              <PromptTraceRawViewer
                v-else-if="activeView === 'raw'"
                :options="rawFileOptions"
                :active="activeRaw"
                :content="rawContent"
                :loading="fileLoading"
                :error="fileError"
                @update:active="activeRaw = $event"
                @copy="copyActiveContent"
              />
              <div v-else class="prompt-trace-view-empty">正在加载 trace JSON...</div>
            </div>
          </n-spin>
        </div>
        <n-empty v-else class="prompt-trace-empty" description="选择一条 prompt trace" />
      </main>
    </div>
  </section>
</template>

<style scoped>
.prompt-trace-page {
  display: flex;
  flex-direction: column;
  height: calc(100vh - 72px);
  min-height: 0;
  overflow: hidden;
  background: #f7f8fb;
  color: #111827;
}

.prompt-trace-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 16px;
  padding: 22px 28px 16px;
  border-bottom: 1px solid #e5e7eb;
  background: #ffffff;
}

.prompt-trace-kicker {
  color: #6b7280;
  font-size: 12px;
  font-weight: 700;
  text-transform: uppercase;
}

.prompt-trace-header h2 {
  margin: 4px 0 0;
  font-size: 22px;
  line-height: 1.2;
}

.prompt-trace-shell {
  display: grid;
  grid-template-columns: minmax(300px, 380px) minmax(0, 1fr);
  flex: 1;
  min-height: 0;
  overflow: hidden;
}

.prompt-trace-sidebar {
  display: flex;
  flex-direction: column;
  border-right: 1px solid #e5e7eb;
  background: #ffffff;
  min-height: 0;
  min-width: 0;
}

.prompt-trace-search {
  padding: 14px 16px 8px;
}

.prompt-trace-root {
  margin: 0 16px 12px;
  color: #6b7280;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.prompt-trace-list {
  display: flex;
  flex-direction: column;
  gap: 6px;
  padding: 0 10px 16px;
  min-height: 0;
  overflow: auto;
}

.prompt-trace-item {
  width: 100%;
  border: 1px solid transparent;
  border-radius: 8px;
  background: transparent;
  padding: 10px 12px;
  text-align: left;
  cursor: pointer;
}

.prompt-trace-item:hover {
  background: #f3f4f6;
}

.prompt-trace-item.is-active {
  border-color: #2563eb;
  background: #eff6ff;
}

.prompt-trace-item-main,
.prompt-trace-item-sub {
  display: flex;
  gap: 8px;
  min-width: 0;
}

.prompt-trace-session {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-weight: 700;
  color: #111827;
}

.prompt-trace-meta,
.prompt-trace-item-sub,
.prompt-trace-item-time {
  color: #6b7280;
  font-size: 12px;
}

.prompt-trace-case,
.prompt-trace-case-detail {
  display: inline-flex;
  max-width: 100%;
  margin-top: 5px;
  border: 1px solid #bbf7d0;
  border-radius: 999px;
  padding: 2px 7px;
  color: #166534;
  background: #f0fdf4;
  font-size: 11px;
  font-weight: 700;
  overflow-wrap: anywhere;
}

.prompt-trace-case-detail {
  margin-top: 6px;
}

.prompt-trace-item-sub {
  margin-top: 4px;
  flex-wrap: wrap;
}

.prompt-trace-item-time {
  display: block;
  margin-top: 5px;
}

.prompt-trace-viewer {
  min-width: 0;
  min-height: 0;
  overflow: hidden;
  padding: 18px;
}

.prompt-trace-detail {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
  border: 1px solid #e5e7eb;
  border-radius: 8px;
  background: #ffffff;
  overflow: hidden;
}

.prompt-trace-detail-header {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  flex-shrink: 0;
  padding: 14px 16px;
  border-bottom: 1px solid #e5e7eb;
}

.prompt-trace-detail-header > div {
  min-width: 0;
}

.prompt-trace-path {
  max-width: min(70vw, 980px);
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 13px;
  font-weight: 700;
}

.prompt-trace-fingerprints {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-top: 6px;
}

.prompt-trace-fingerprints span {
  border: 1px solid #d1d5db;
  border-radius: 999px;
  padding: 2px 8px;
  color: #374151;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 11px;
}

.prompt-trace-tabs {
  display: flex;
  gap: 6px;
  flex-shrink: 0;
  overflow-x: auto;
  padding: 10px 12px;
  border-bottom: 1px solid #e5e7eb;
  background: #f9fafb;
}

.prompt-trace-tab {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  border: 1px solid #d1d5db;
  border-radius: 7px;
  background: #ffffff;
  padding: 6px 10px;
  color: #374151;
  cursor: pointer;
}

.prompt-trace-tab.is-active {
  border-color: #2563eb;
  color: #1d4ed8;
  background: #eff6ff;
}

.prompt-trace-code {
  min-height: calc(100vh - 298px);
  max-height: calc(100vh - 298px);
  margin: 0;
  padding: 16px;
  overflow: auto;
  background: #0f172a;
  color: #e5e7eb;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  font-size: 12px;
  line-height: 1.55;
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.prompt-trace-alert {
  margin: 16px;
}

.prompt-trace-list-spin,
.prompt-trace-spin {
  min-height: 0;
}

.prompt-trace-list-spin {
  flex: 1;
}

.prompt-trace-spin {
  display: flex;
  flex: 1;
  flex-direction: column;
  overflow: hidden;
}

.prompt-trace-list-spin :deep(.n-spin-container),
.prompt-trace-list-spin :deep(.n-spin-content),
.prompt-trace-list-spin :deep(.ops-spin-content),
.prompt-trace-spin :deep(.n-spin-container),
.prompt-trace-spin :deep(.n-spin-content),
.prompt-trace-spin :deep(.ops-spin-content) {
  display: flex;
  flex: 1;
  flex-direction: column;
  min-height: 0;
  overflow: hidden;
}

.prompt-trace-content {
  flex: 1;
  min-height: 0;
  overflow: auto;
  padding: 16px;
}

.prompt-trace-empty {
  margin-top: 56px;
}

@media (max-width: 900px) {
  .prompt-trace-shell {
    grid-template-columns: 1fr;
    overflow: auto;
  }

  .prompt-trace-sidebar {
    border-right: 0;
    border-bottom: 1px solid #e5e7eb;
  }

  .prompt-trace-code {
    max-height: 70vh;
  }
}
</style>
