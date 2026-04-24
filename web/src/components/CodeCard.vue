<script setup>
import { computed, ref, onMounted, onBeforeUnmount, watch } from "vue";
import { AlertTriangleIcon, CopyIcon, ExternalLinkIcon, FileCode2Icon, SearchIcon } from "lucide-vue-next";
import Modal from "./Modal.vue";
import { useAppStore } from "../store";
import { resolveHostDisplay } from "../lib/hostDisplay";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
});

const store = useAppStore();
const editorContainer = ref(null);
const selectedIndex = ref(0);
const previewOpen = ref(false);
const previewLoading = ref(false);
const previewError = ref("");
const previewHostLabel = ref("");
const previewPath = ref("");
const previewContent = ref("");
const previewTruncated = ref(false);
const repeatSearchBusy = ref(false);
const repeatSearchMessage = ref("");
let editor = null;
let monacoModulePromise = null;

const changes = computed(() => props.card.changes || []);
const isPreviewCard = computed(() => props.card.type === "FilePreviewCard");
const isFileChangeCard = computed(() => props.card.type === "FileChangeCard");
const selectedChange = computed(() => changes.value[selectedIndex.value] || null);
const selectedTarget = computed(() => parseFileLinkTarget(selectedChange.value?.path || changes.value[0]?.path || ""));
const displayHostId = computed(() => (props.card.hostId || "server-local").trim() || "server-local");
const displayHostLabel = computed(() => {
  const host = store.snapshot.hosts.find((item) => item.id === displayHostId.value) || {};
  return resolveHostDisplay({
    ...host,
    id: displayHostId.value,
    hostId: displayHostId.value,
    hostName: props.card.hostName,
  }) || "server-local";
});
const fileCount = computed(() => {
  if (!changes.value.length) return 0;
  const seen = new Set();
  for (const change of changes.value) {
    const target = parseFileLinkTarget(change.path);
    const key = `${resolvePreviewHostId(target)}:${target.path || change.path || ""}`;
    seen.add(key);
  }
  return seen.size;
});
const selectedPathLabel = computed(() => {
  const target = selectedTarget.value;
  if (!target.path) return "";
  const hostId = resolvePreviewHostId(target);
  const hostPrefix = hostId && hostId !== "server-local" && hostId !== displayHostId.value ? `${hostId} · ` : "";
  return `${hostPrefix}${target.path}`;
});
const selectedSearchPath = computed(() => {
  const target = selectedTarget.value;
  if (!target.path) return "";
  const cleanPath = target.path.trim();
  if (!cleanPath) return "";
  if (props.card.type === "FilePreviewCard") {
    return dirnameFromPath(cleanPath);
  }
  if (cleanPath.endsWith("/")) {
    return cleanPath.replace(/\/+$/, "");
  }
  return dirnameFromPath(cleanPath);
});
const repeatSearchPrompt = computed(() => {
  const hostId = displayHostId.value;
  const scopePath = selectedSearchPath.value;
  const focusPath = selectedTarget.value.path;
  if (!hostId || hostId === "server-local" || !scopePath || !focusPath) return "";

  const modeText = isFileChangeCard.value
    ? `围绕刚修改的文件 \`${focusPath}\` 继续确认相关引用、配置项和依赖关系`
    : `围绕文件 \`${focusPath}\` 继续搜索相关内容`;

  return [
    `当前目标主机是远程 Linux（host=${hostId}）。`,
    "请只使用 `search_remote_files`，不要退回本地工具，也不要改成 commandExecution。",
    `请在 \`${scopePath}\` 中再次搜索，${modeText}，并保留结构化文件结果卡。`,
  ].join(" ");
});
const repeatSearchButtonLabel = computed(() => {
  if (!repeatSearchPrompt.value) return "再次搜索";
  return isFileChangeCard.value ? "再次搜索相关目录" : "再次搜索";
});
const canRepeatSearch = computed(() => !!repeatSearchPrompt.value && displayHostId.value !== "server-local" && !repeatSearchBusy.value && !store.sending && !store.runtime.turn.active && store.canSend);
const content = computed(() => {
  if (selectedChange.value?.diff) return selectedChange.value.diff;
  return props.card.output || props.card.text || "";
});
const filename = computed(() => selectedTarget.value.path || selectedChange.value?.path || "snippet.txt");
const language = computed(() => getLanguageFromFilename(filename.value));
const changeSummary = computed(() => {
  if (isPreviewCard.value) return "文件预览";
  if (isFileChangeCard.value) {
    if (fileCount.value > 1) return `已修改 ${fileCount.value} 个文件`;
    if (fileCount.value === 1) return "已修改 1 个文件";
  }
  if (changes.value.length > 1) return `${changes.value.length} 个文件变更`;
  if (changes.value.length === 1) return "1 个文件变更";
  return "代码片段";
});
const statusLabel = computed(() => {
  const status = props.card.status || "";
  if (isPreviewCard.value && status === "completed") return "已读取";
  if (status === "completed") return "已完成";
  if (status === "failed") return "失败";
  if (status === "cancelled") return "已取消";
  if (status === "pending") return "待处理";
  return "处理中";
});
const statusTone = computed(() => {
  if (props.card.status === "failed") return "danger";
  if (props.card.status === "completed") return "success";
  return "neutral";
});
const summaryText = computed(() => {
  if (props.card.status === "failed") {
    return props.card.error || props.card.text || props.card.output || "文件变更失败";
  }
  return props.card.summary || props.card.text || props.card.output || changeSummary.value;
});
const secondaryText = computed(() => {
  if (props.card.status === "failed") {
    if (props.card.text && props.card.text !== summaryText.value) return props.card.text;
    if (props.card.summary && props.card.summary !== summaryText.value) return props.card.summary;
    if (props.card.output && props.card.output !== summaryText.value) return props.card.output;
    return "";
  }
  if (props.card.summary && props.card.text && props.card.summary !== props.card.text) {
    return props.card.text;
  }
  if (props.card.error && props.card.error !== summaryText.value) {
    return props.card.error;
  }
  return "";
});
const details = computed(() => {
  const rows = [];
  rows.push({ key: "主机", value: displayHostLabel.value });
  if (selectedPathLabel.value) {
    rows.push({ key: "路径", value: selectedPathLabel.value });
  }
  if (fileCount.value) {
    rows.push({ key: "文件数", value: `${fileCount.value}` });
  }
  if (selectedChange.value?.kind) {
    rows.push({ key: "类型", value: selectedChange.value.kind });
  }
  rows.push({ key: "状态", value: statusLabel.value });
  return rows;
});
const canOpenPreview = computed(() => !!selectedPathLabel.value);

function parseFileLinkTarget(raw) {
  const value = (raw || "").trim();
  if (!value) {
    return { hostId: "server-local", path: "", line: 0 };
  }

  if (value.startsWith("remote://")) {
    try {
      const parsed = new URL(value);
      const path = decodeURIComponent(parsed.pathname || "");
      const lineMatch = parsed.hash.match(/^#L(\d+)$/i);
      return {
        hostId: parsed.host || "server-local",
        path,
        line: lineMatch ? Number(lineMatch[1]) : 0,
      };
    } catch (_err) {
      return { hostId: "server-local", path: value.replace(/^remote:\/\//, ""), line: 0 };
    }
  }

  const [pathPart, hashPart] = value.split("#", 2);
  const lineMatch = (hashPart || "").match(/^L(\d+)$/i);
  return {
    hostId: "server-local",
    path: pathPart,
    line: lineMatch ? Number(lineMatch[1]) : 0,
  };
}

function resolvePreviewHostId(target) {
  const targetHost = (target?.hostId || "").trim();
  if (targetHost && targetHost !== "server-local") {
    return targetHost;
  }
  return displayHostId.value || "server-local";
}

function dirnameFromPath(path) {
  const value = (path || "").trim();
  if (!value) return "";
  const normalized = value.replace(/\/+$/, "");
  const slashIndex = normalized.lastIndexOf("/");
  if (slashIndex <= 0) {
    return normalized || value;
  }
  return normalized.slice(0, slashIndex);
}

function getLanguageFromFilename(name) {
  if (name.endsWith(".go")) return "go";
  if (name.endsWith(".js") || name.endsWith(".jsx")) return "javascript";
  if (name.endsWith(".vue")) return "vue";
  if (name.endsWith(".ts") || name.endsWith(".tsx")) return "typescript";
  if (name.endsWith(".css")) return "css";
  if (name.endsWith(".json")) return "json";
  if (name.endsWith(".md")) return "markdown";
  if (name.endsWith(".sh") || name.endsWith(".bash")) return "shell";
  return "plaintext";
}

function resizeEditor() {
  if (!editor || !editorContainer.value) return;
  const lineCount = content.value.split("\n").length;
  const height = Math.min(Math.max(lineCount * 19 + 28, 92), 420);
  editorContainer.value.style.height = `${height}px`;
  editor.layout();
}

function ensureMonaco() {
  if (!monacoModulePromise) {
    monacoModulePromise = import("monaco-editor/esm/vs/editor/editor.api");
  }
  return monacoModulePromise;
}

async function initMonaco() {
  if (!editorContainer.value || editor) return;
  const monaco = await ensureMonaco();
  if (!editorContainer.value || editor) return;

  editor = monaco.editor.create(editorContainer.value, {
    value: content.value,
    language: language.value,
    theme: "vs-light",
    readOnly: true,
    minimap: { enabled: false },
    scrollBeyondLastLine: false,
    lineNumbers: "on",
    renderLineHighlight: "none",
    padding: { top: 12, bottom: 12 },
    automaticLayout: true,
    fontSize: 13,
    fontFamily: '"SF Mono", "Fira Code", monospace',
  });

  resizeEditor();
}

async function copyCode() {
  try {
    await navigator.clipboard.writeText(content.value);
  } catch (err) {
    console.error("Failed to copy", err);
  }
}

async function repeatSearch() {
  if (!canRepeatSearch.value) return;
  repeatSearchBusy.value = true;
  repeatSearchMessage.value = "";

  try {
    await sendMessage({
      hostId: displayHostId.value,
      message: repeatSearchPrompt.value,
    });
    repeatSearchMessage.value = "已发起再次搜索";
  } catch (err) {
    console.error("Failed to send repeat search", err);
    repeatSearchMessage.value = "再次搜索发送失败";
  } finally {
    repeatSearchBusy.value = false;
  }
}

async function openFilePreview(raw) {
  const target = parseFileLinkTarget(raw);
  if (!target.path) return;
  const hostId = resolvePreviewHostId(target);

  previewOpen.value = true;
  previewLoading.value = true;
  previewError.value = "";
  previewHostLabel.value = hostId;
  previewPath.value = target.path;
  previewContent.value = "";
  previewTruncated.value = false;

  try {
    const data = await fetchFilePreview({ hostId, path: target.path });
    previewHostLabel.value = data.hostId || hostId;
    previewPath.value = data.path || target.path;
    previewContent.value = data.content || "";
    previewTruncated.value = !!data.truncated;
  } catch (_err) {
    previewError.value = "文件预览失败";
  } finally {
    previewLoading.value = false;
  }
}

function openSelectedPreview() {
  openFilePreview(selectedChange.value?.path || changes.value[0]?.path || "");
}

function closePreview() {
  previewOpen.value = false;
}

watch(
  () => props.card.id,
  () => {
    selectedIndex.value = 0;
    previewOpen.value = false;
  }
);

watch([content, language], () => {
  if (!editor) return;
  const model = editor.getModel();
  if (!model) return;
  void ensureMonaco().then((monaco) => {
    if (!editor) return;
    model.setValue(content.value);
    monaco.editor.setModelLanguage(model, language.value);
    resizeEditor();
  });
});

onMounted(() => {
  window.setTimeout(() => {
    void initMonaco();
  }, 10);
});

onBeforeUnmount(() => {
  if (editor) {
    editor.dispose();
    editor = null;
  }
});
</script>

<template>
  <div class="code-card">
    <div class="code-header">
      <div class="header-left">
        <div class="file-info">
          <FileCode2Icon size="16" class="file-icon" />
          <div class="file-title-block">
            <span class="file-name">{{ filename }}</span>
            <span class="file-subtitle">{{ changeSummary }}</span>
          </div>
          <span class="file-tag" v-if="selectedChange?.kind">{{ selectedChange.kind }}</span>
        </div>
        <div class="change-summary">
          <span v-if="displayHostLabel" class="summary-chip">{{ displayHostLabel }}</span>
          <span v-if="selectedPathLabel" class="summary-chip mono">{{ selectedPathLabel }}</span>
          <span v-if="fileCount" class="summary-chip">{{ fileCount }} 个文件</span>
        </div>
      </div>

      <div class="header-right">
        <span class="status-pill" :class="[card.status || 'inProgress', statusTone]">{{ statusLabel }}</span>
        <button
          v-if="canRepeatSearch"
          class="action-btn action-btn-search"
          @click="repeatSearch"
          :disabled="repeatSearchBusy"
          :title="repeatSearchButtonLabel"
        >
          <SearchIcon size="14" />
        </button>
        <button v-if="canOpenPreview" class="action-btn" @click="openSelectedPreview" title="重新打开远程文件预览">
          <ExternalLinkIcon size="14" />
        </button>
        <button class="action-btn" @click="copyCode" title="Copy">
          <CopyIcon size="14" />
        </button>
      </div>
    </div>

    <div class="code-meta" v-if="summaryText || secondaryText || details.length">
      <div class="summary-panel" :class="statusTone">
        <div class="summary-line">{{ summaryText }}</div>
        <div v-if="secondaryText" class="summary-note">{{ secondaryText }}</div>
      </div>
      <div class="meta-grid">
        <div v-for="row in details" :key="row.key" class="meta-row">
          <span class="meta-key">{{ row.key }}</span>
          <span class="meta-value">{{ row.value }}</span>
        </div>
      </div>
      <div v-if="card.status === 'failed' && card.error" class="failure-banner">
        <AlertTriangleIcon size="15" class="failure-icon" />
        <span>{{ card.error }}</span>
      </div>
      <div v-if="repeatSearchMessage" class="search-banner">{{ repeatSearchMessage }}</div>
    </div>

    <div class="code-body" :class="{ split: changes.length > 1 }">
      <div v-if="changes.length > 1" class="change-list">
        <button
          v-for="(change, index) in changes"
          :key="`${card.id}-${change.path}-${index}`"
          class="change-item"
          :class="{ active: selectedIndex === index }"
          @click="selectedIndex = index"
        >
          <span class="change-path">{{ parseFileLinkTarget(change.path).path || change.path }}</span>
          <span class="change-kind">{{ change.kind }}</span>
        </button>
      </div>

      <div class="editor-shell">
        <div class="editor-wrapper" ref="editorContainer"></div>
      </div>
    </div>
  </div>

  <Modal v-if="previewOpen" title="文件预览" @close="closePreview">
    <div class="preview-modal">
      <div class="preview-title">{{ previewPath || "文件预览" }}</div>
      <div class="preview-host">{{ previewHostLabel }}</div>
      <div v-if="previewLoading" class="preview-state">正在读取文件...</div>
      <div v-else-if="previewError" class="preview-error">{{ previewError }}</div>
      <template v-else>
        <pre class="preview-code">{{ previewContent }}</pre>
        <div v-if="previewTruncated" class="preview-note">文件内容过长，当前仅展示前一部分。</div>
      </template>
    </div>
  </Modal>
</template>

<style scoped>
.code-card {
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  overflow: hidden;
  margin-top: 2px;
  margin-left: 36px;
  max-width: 780px;
  box-shadow: 0 2px 10px rgba(15, 23, 42, 0.03);
  display: flex;
  flex-direction: column;
}

.code-header {
  padding: 10px 14px;
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 12px;
  background: #f8fafc;
  border-bottom: 1px solid #e5e7eb;
}

.header-left {
  display: flex;
  flex-direction: column;
  gap: 6px;
  min-width: 0;
}

.header-right {
  display: flex;
  align-items: center;
  gap: 10px;
}

.file-info {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
}

.file-title-block {
  display: flex;
  flex-direction: column;
  gap: 1px;
  min-width: 0;
  flex: 1;
}

.file-icon {
  color: #64748b;
  flex-shrink: 0;
}

.file-name {
  font-size: 13px;
  font-weight: 600;
  color: #0f172a;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-subtitle {
  font-size: 11px;
  color: #94a3b8;
}

.file-tag {
  font-size: 10px;
  font-weight: 700;
  text-transform: uppercase;
  padding: 2px 6px;
  border-radius: 999px;
  background: #e2e8f0;
  color: #475569;
}

.change-summary {
  font-size: 12px;
  color: #94a3b8;
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
  align-items: center;
}

.summary-chip {
  display: inline-flex;
  align-items: center;
  max-width: 100%;
  padding: 3px 8px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 600;
}

.summary-chip.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  background: #f8fafc;
  color: #475569;
}

.status-pill {
  display: inline-flex;
  align-items: center;
  padding: 4px 10px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.02em;
}

.status-pill.completed,
.status-pill.success {
  background: #ecfdf5;
  color: #047857;
}

.status-pill.failed,
.status-pill.danger {
  background: #fef2f2;
  color: #b91c1c;
}

.status-pill.neutral,
.status-pill.inProgress {
  background: #eff6ff;
  color: #1d4ed8;
}

.action-btn {
  background: transparent;
  border: none;
  color: #64748b;
  width: 30px;
  height: 30px;
  border-radius: 8px;
  display: flex;
  align-items: center;
  justify-content: center;
  cursor: pointer;
}

.action-btn:hover {
  background: #e2e8f0;
  color: #0f172a;
}

.action-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.code-meta {
  padding: 0 16px 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
  background: linear-gradient(180deg, rgba(248, 250, 252, 0.7), rgba(255, 255, 255, 1));
}

.summary-panel {
  border-radius: 14px;
  padding: 12px 14px;
  border: 1px solid #dbe3ee;
  background: #f8fafc;
}

.summary-panel.success {
  background: #f0fdf4;
  border-color: #bbf7d0;
}

.summary-panel.danger {
  background: #fef2f2;
  border-color: #fecaca;
}

.summary-line {
  font-size: 13px;
  color: #0f172a;
  line-height: 1.5;
  font-weight: 600;
}

.summary-note {
  margin-top: 4px;
  font-size: 12px;
  color: #64748b;
  line-height: 1.5;
  white-space: pre-wrap;
}

.meta-grid {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.meta-row {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 7px 10px;
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  font-size: 12px;
}

.meta-key {
  color: #94a3b8;
}

.meta-value {
  color: #0f172a;
  font-weight: 600;
  max-width: 420px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.failure-banner {
  display: flex;
  align-items: flex-start;
  gap: 8px;
  padding: 10px 12px;
  border-radius: 12px;
  background: #fef2f2;
  color: #b91c1c;
  font-size: 12px;
  line-height: 1.5;
}

.failure-icon {
  margin-top: 1px;
  flex-shrink: 0;
}

.search-banner {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  padding: 8px 12px;
  border-radius: 12px;
  background: #f0fdf4;
  color: #166534;
  font-size: 12px;
  line-height: 1.5;
}

.code-body {
  display: flex;
  min-height: 140px;
}

.code-body.split {
  display: grid;
  grid-template-columns: 220px 1fr;
}

.change-list {
  width: 220px;
  border-right: 1px solid #e5e7eb;
  background: #fcfcfd;
  padding: 10px;
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.change-item {
  border: none;
  background: transparent;
  border-radius: 12px;
  padding: 10px 12px;
  text-align: left;
  cursor: pointer;
  display: flex;
  flex-direction: column;
  gap: 4px;
}

.change-item:hover {
  background: #f8fafc;
}

.change-item.active {
  background: #eff6ff;
}

.change-path {
  font-size: 12px;
  font-weight: 600;
  color: #0f172a;
  word-break: break-all;
}

.change-kind {
  font-size: 11px;
  color: #94a3b8;
  text-transform: uppercase;
}

.editor-shell {
  flex: 1;
  background: #ffffff;
}

.editor-wrapper {
  width: 100%;
  background: #ffffff;
}

.preview-modal {
  min-width: min(720px, 90vw);
  max-width: 90vw;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.preview-title {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  word-break: break-all;
}

.preview-host {
  font-size: 11px;
  color: #64748b;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.preview-state,
.preview-error,
.preview-note {
  padding: 12px 14px;
  border-radius: 12px;
  font-size: 13px;
  line-height: 1.6;
}

.preview-state {
  background: #f8fafc;
  color: #64748b;
}

.preview-error {
  background: #fef2f2;
  color: #b91c1c;
}

.preview-code {
  margin: 0;
  padding: 14px;
  max-height: 60vh;
  overflow: auto;
  border-radius: 14px;
  background: #0f172a;
  color: #e2e8f0;
  white-space: pre-wrap;
  word-break: break-word;
  font-size: 12px;
  line-height: 1.65;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.preview-note {
  background: #fff7ed;
  color: #9a3412;
}
</style>
import { sendMessage } from "../api/chat";
import { fetchFilePreview } from "../api/files";
