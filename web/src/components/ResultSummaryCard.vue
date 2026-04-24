<script setup>
import { computed, ref } from "vue";
import { ClipboardCheckIcon, FileTextIcon, FolderIcon, SearchIcon } from "lucide-vue-next";
import Modal from "./Modal.vue";
import { useAppStore } from "../store";
import { resolveHostDisplay } from "../lib/hostDisplay";

const props = defineProps({
  card: {
    type: Object,
    required: true,
  },
  sessionKind: {
    type: String,
    default: "",
  },
});

const store = useAppStore();
const primarySummary = computed(() => props.card.summary || props.card.text || "");
const auxiliaryNote = computed(() => (props.card.summary && props.card.text ? props.card.text : ""));
const kvRows = computed(() => props.card.kvRows || []);
const highlightPills = computed(() => props.card.highlights || []);
const rawFileItems = computed(() => props.card.fileItems || []);
const displayHostId = computed(() => {
  const explicit = (props.card.hostId || "").trim();
  if (explicit && explicit !== "server-local") return explicit;
  const itemHost = normalizedFileItems.value.find((item) => item.target?.hostId && item.target.hostId !== "server-local")?.target?.hostId;
  if (itemHost) return itemHost;
  const selectedHostId = (store.snapshot.selectedHostId || "").trim();
  if (selectedHostId && selectedHostId !== "server-local") return selectedHostId;
  return "";
});
const displayHostLabel = computed(() => {
  const hostId = displayHostId.value;
  const host = store.snapshot.hosts.find((item) => item.id === hostId) || {};
  return resolveHostDisplay({
    ...host,
    id: hostId,
    hostId,
    hostName: props.card.hostName,
  }) || hostId;
});
const searchScopePath = computed(() => {
  const directoryRow = kvRows.value.find((row) => isScopeKey(row.key) && (row.value || "").trim());
  if (directoryRow) return normalizeSearchScope(directoryRow.value, "dir");

  const firstMatch = normalizedFileItems.value.find((item) => item.kind === "match");
  if (firstMatch) return normalizeSearchScope(firstMatch.absolutePath || firstMatch.target?.path || "", firstMatch.kind);

  const firstDir = normalizedFileItems.value.find((item) => item.kind === "dir");
  if (firstDir) return normalizeSearchScope(firstDir.absolutePath || firstDir.target?.path || "", firstDir.kind);

  const firstFile = normalizedFileItems.value.find((item) => item.absolutePath);
  if (firstFile) return normalizeSearchScope(firstFile.absolutePath || firstFile.target?.path || "", firstFile.kind);

  return "";
});
const searchFocusLabel = computed(() => {
  const queryRow = kvRows.value.find((row) => isQueryKey(row.key) && (row.value || "").trim());
  if (queryRow) return queryRow.value.trim();
  const firstMatch = normalizedFileItems.value.find((item) => item.kind === "match" && item.label);
  if (firstMatch) return firstMatch.label;
  const firstFile = normalizedFileItems.value.find((item) => item.label);
  return firstFile?.label || "";
});
const repeatSearchPrompt = computed(() => {
  const hostId = displayHostId.value;
  const scopePath = searchScopePath.value;
  if (!hostId || !scopePath) return "";

  const focusText = searchFocusLabel.value ? `，重点围绕“${searchFocusLabel.value}”继续排查` : "";
  const resultText = hasStructuredFiles.value
    ? "，并保留结构化文件结果卡，便于继续回看和打开预览"
    : "";

  return [
    `当前目标主机是远程 Linux（host=${hostId}）。`,
    "请只使用 `search_remote_files`，不要退回本地工具，也不要改成 commandExecution。",
    `请在 \`${scopePath}\` 中再次搜索${focusText}${resultText}。`,
  ].join(" ");
});
const repeatSearchBusy = ref(false);
const repeatSearchMessage = ref("");

const normalizedFileItems = computed(() => rawFileItems.value.map(normalizeFileItem).filter(Boolean));
const structuredFileSections = computed(() => {
  const items = normalizedFileItems.value;
  const sections = [];
  const searchMatches = items.filter((item) => item.kind === "match");
  const directories = items.filter((item) => item.kind === "dir");
  const files = items.filter((item) => item.kind !== "dir" && item.kind !== "match");

  if (searchMatches.length) {
    sections.push({
      key: "matches",
      title: "搜索命中",
      note: `${searchMatches.length} 条结果`,
      items: searchMatches,
    });
  }
  if (directories.length) {
    sections.push({
      key: "directories",
      title: "目录条目",
      note: `${directories.length} 个目录`,
      items: directories,
    });
  }
  if (files.length) {
    sections.push({
      key: "files",
      title: "文件条目",
      note: `${files.length} 个文件`,
      items: files,
    });
  }
  return sections;
});
const hasStructuredFiles = computed(() => structuredFileSections.value.length > 0);
const hasHighlights = computed(() => highlightPills.value.length > 0);
const canRepeatSearch = computed(() => !!repeatSearchPrompt.value && !repeatSearchBusy.value && !store.sending && !store.runtime.turn.active && store.canSend);
const contextLabel = computed(() => (props.sessionKind === "workspace" ? "工作台结果投影" : "执行结果"));

const previewOpen = ref(false);
const previewLoading = ref(false);
const previewError = ref("");
const previewPath = ref("");
const previewContent = ref("");
const previewTruncated = ref(false);

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

function displayTargetLabel(target) {
  const path = (target.path || "").trim();
  const hostId = (target.hostId || "server-local").trim();
  if (!path) return hostId;
  const line = target.line > 0 ? ` #L${target.line}` : "";
  return `${hostId} · ${path}${line}`;
}

function normalizeFileItem(item) {
  const target = parseFileLinkTarget(item.path);
  const kind = (item.kind || "file").trim() || "file";
  const absolutePath = target.path || "";
  const label = (item.label || absolutePath || "未命名条目").trim();
  const isDir = kind === "dir";
  const isMatch = kind === "match";
  const preview = (item.preview || "").trim();
  const meta = (item.meta || "").trim();

  return {
    ...item,
    kind,
    target,
    absolutePath,
    label,
    preview,
    meta,
    title: displayTargetLabel(target),
    kindLabel: isMatch ? "命中" : isDir ? "目录" : "文件",
    previewable: !isDir && !!absolutePath,
  };
}

function itemIcon(item) {
  if (item.kind === "dir") return FolderIcon;
  if (item.kind === "match") return SearchIcon;
  return FileTextIcon;
}

function canPreview(item) {
  return item.previewable;
}

function isScopeKey(key) {
  const value = (key || "").trim();
  return ["目录", "路径", "范围", "搜索范围", "目标目录", "文件夹", "目录范围"].includes(value);
}

function isQueryKey(key) {
  const value = (key || "").trim();
  const normalized = value.toLowerCase();
  return ["关键词", "关键字", "查询词", "搜索词"].includes(value) || ["query", "keyword", "search", "term"].includes(normalized);
}

function normalizeSearchScope(raw, kind = "file") {
  const value = (raw || "").trim();
  if (!value) return "";
  const target = parseFileLinkTarget(value);
  const basePath = target.path || value;
  const normalized = basePath.replace(/\\/g, "/").replace(/\/+$/, "");
  if (!normalized) return "";
  if (kind === "dir") return normalized;
  const slashIndex = normalized.lastIndexOf("/");
  if (slashIndex <= 0) return normalized;
  return normalized.slice(0, slashIndex);
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
    repeatSearchMessage.value = `已发起再次搜索：${displayHostLabel.value} · ${searchScopePath.value}`;
  } catch (_err) {
    repeatSearchMessage.value = "再次搜索发送失败";
  } finally {
    repeatSearchBusy.value = false;
  }
}

async function openFilePreview(raw) {
  const target = parseFileLinkTarget(raw);
  if (!target.path) return;

  previewOpen.value = true;
  previewLoading.value = true;
  previewError.value = "";
  previewPath.value = target.path;
  previewContent.value = "";
  previewTruncated.value = false;

  try {
    const data = await fetchFilePreview({ hostId: target.hostId, path: target.path });
    previewPath.value = data.path || target.path;
    previewContent.value = data.content || "";
    previewTruncated.value = !!data.truncated;
  } catch (_err) {
    previewError.value = "文件预览失败";
  } finally {
    previewLoading.value = false;
  }
}

function closePreview() {
  previewOpen.value = false;
}
</script>

<template>
  <div class="result-card">
    <div class="result-header">
      <ClipboardCheckIcon size="16" class="result-icon" />
      <div class="result-title-group">
        <span class="result-context">{{ contextLabel }}</span>
        <span class="result-title">{{ card.title || "执行结果" }}</span>
        <span v-if="hasStructuredFiles" class="result-subtitle">{{ structuredFileSections.length }} 个结构化结果区</span>
      </div>
      <div class="result-actions">
        <button
          v-if="canRepeatSearch"
          type="button"
          class="result-action-btn"
          @click="repeatSearch"
          :disabled="repeatSearchBusy"
          title="再次搜索当前主机和目录"
        >
          <SearchIcon size="13" />
          <span>再次搜索</span>
        </button>
      </div>
    </div>

    <div class="result-body">
      <div v-if="primarySummary || auxiliaryNote" class="result-summary-panel">
        <p v-if="primarySummary" class="result-summary">{{ primarySummary }}</p>
        <p v-if="auxiliaryNote" class="result-note">{{ auxiliaryNote }}</p>
      </div>

      <div v-if="kvRows.length" class="kv-grid">
        <div v-for="(row, idx) in kvRows" :key="`${row.key}-${idx}`" class="kv-chip">
          <span class="kv-key">{{ row.key }}</span>
          <span class="kv-val">{{ row.value }}</span>
        </div>
      </div>

      <div v-if="hasStructuredFiles" class="file-sections">
        <section v-for="section in structuredFileSections" :key="section.key" class="file-section">
          <div class="file-section-header">
            <div class="file-section-heading">
              <span class="file-section-title">{{ section.title }}</span>
              <span class="file-section-note">{{ section.note }}</span>
            </div>
            <span class="file-section-count">{{ section.items.length }}</span>
          </div>

          <div class="file-list">
            <article
              v-for="(item, idx) in section.items"
              :key="`${item.path}-${idx}`"
              class="file-card"
              :class="{ clickable: canPreview(item), match: item.kind === 'match', dir: item.kind === 'dir' }"
            >
              <button
                v-if="canPreview(item)"
                type="button"
                class="file-card-main"
                :title="item.title"
                @click="openFilePreview(item.path)"
              >
                <div class="file-card-icon">
                  <component :is="itemIcon(item)" size="15" class="file-item-icon" />
                </div>
                <div class="file-card-content">
                  <div class="file-card-title-row">
                    <span class="file-card-label" :class="{ highlight: item.kind === 'match' }">{{ item.label }}</span>
                    <span class="file-card-kind">{{ item.kindLabel }}</span>
                  </div>
                  <div class="file-card-path">
                    <span class="mono">{{ item.absolutePath }}</span>
                    <span v-if="item.target.line" class="file-card-line">L{{ item.target.line }}</span>
                    <span v-if="item.meta" class="file-card-meta">{{ item.meta }}</span>
                  </div>
                  <pre v-if="item.preview" class="file-card-preview">{{ item.preview }}</pre>
                </div>
              </button>

              <div v-else class="file-card-main static" :title="item.title">
                <div class="file-card-icon">
                  <component :is="itemIcon(item)" size="15" class="file-item-icon" />
                </div>
                <div class="file-card-content">
                  <div class="file-card-title-row">
                    <span class="file-card-label" :class="{ highlight: item.kind === 'match' }">{{ item.label }}</span>
                    <span class="file-card-kind">{{ item.kindLabel }}</span>
                  </div>
                  <div class="file-card-path">
                    <span class="mono">{{ item.absolutePath }}</span>
                    <span v-if="item.meta" class="file-card-meta">{{ item.meta }}</span>
                  </div>
                </div>
              </div>
            </article>
          </div>
        </section>
      </div>

      <div v-if="hasHighlights" class="result-highlights">
        <span class="highlight-pill" v-for="(h, idx) in highlightPills" :key="idx">{{ h }}</span>
      </div>

      <div v-if="repeatSearchMessage" class="search-banner">{{ repeatSearchMessage }}</div>
    </div>
  </div>

  <Modal v-if="previewOpen" :title="previewPath || '文件预览'" @close="closePreview">
    <div class="preview-modal">
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
.result-card {
  border-radius: 12px;
  background: #ffffff;
  border: 1px solid #e5e7eb;
  overflow: hidden;
  margin-top: 2px;
  margin-left: 36px;
  max-width: 720px;
  box-shadow: 0 2px 6px rgba(0, 0, 0, 0.02);
}

.result-header {
  padding: 10px 16px;
  display: flex;
  align-items: center;
  gap: 8px;
  background: #f0fdf4;
  border-bottom: 1px solid #dcfce7;
}

.result-icon {
  color: #16a34a;
}

.result-title-group {
  display: flex;
  flex-direction: column;
  gap: 2px;
  min-width: 0;
}

.result-context {
  font-size: 11px;
  color: #65a30d;
  font-weight: 700;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.result-title {
  font-size: 13px;
  font-weight: 600;
  color: #166534;
}

.result-subtitle {
  font-size: 11px;
  color: #4b5563;
}

.result-actions {
  margin-left: auto;
  display: flex;
  align-items: center;
  gap: 8px;
  flex-shrink: 0;
}

.result-action-btn {
  border: 1px solid #bbf7d0;
  background: #ffffff;
  color: #166534;
  border-radius: 999px;
  padding: 7px 10px;
  display: inline-flex;
  align-items: center;
  gap: 6px;
  font-size: 12px;
  font-weight: 700;
  cursor: pointer;
  transition: background 0.15s ease, border-color 0.15s ease, transform 0.15s ease;
}

.result-action-btn:hover:not(:disabled) {
  background: #f0fdf4;
  border-color: #86efac;
  transform: translateY(-1px);
}

.result-action-btn:disabled {
  opacity: 0.65;
  cursor: wait;
  transform: none;
}

.result-body {
  padding: 12px 16px;
}

.result-summary-panel {
  display: flex;
  flex-direction: column;
  gap: 4px;
  margin-bottom: 14px;
}

.result-summary {
  margin: 0;
  font-size: 13px;
  line-height: 1.6;
  color: #1f2937;
  word-break: break-word;
}

.result-note {
  margin: 0;
  font-size: 12px;
  line-height: 1.6;
  color: #64748b;
  word-break: break-word;
}

.search-banner {
  margin: 0 0 14px;
  padding: 10px 12px;
  border-radius: 12px;
  background: #ecfdf5;
  border: 1px solid #bbf7d0;
  color: #166534;
  font-size: 12px;
  line-height: 1.6;
  word-break: break-word;
}

.kv-grid {
  display: grid;
  grid-template-columns: repeat(auto-fit, minmax(180px, 1fr));
  gap: 8px;
  margin-bottom: 14px;
}

.kv-chip {
  padding: 8px 12px;
  border-radius: 12px;
  background: #f8fafc;
  border: 1px solid #dbe3ee;
  display: flex;
  justify-content: space-between;
  gap: 12px;
  font-size: 12px;
}

.kv-key {
  color: #64748b;
  font-weight: 600;
  white-space: nowrap;
}

.kv-val {
  color: #111827;
  font-weight: 600;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  text-align: right;
  word-break: break-all;
}

.file-sections {
  display: flex;
  flex-direction: column;
  gap: 14px;
  margin-bottom: 14px;
}

.file-section {
  border: 1px solid #e5e7eb;
  border-radius: 14px;
  background: linear-gradient(180deg, #ffffff 0%, #f8fafc 100%);
  overflow: hidden;
}

.file-section-header {
  padding: 10px 12px;
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
  border-bottom: 1px solid #e2e8f0;
}

.file-section-heading {
  display: flex;
  align-items: baseline;
  gap: 8px;
  min-width: 0;
}

.file-section-title {
  font-size: 13px;
  font-weight: 700;
  color: #0f172a;
}

.file-section-note {
  font-size: 11px;
  color: #64748b;
}

.file-section-count {
  flex-shrink: 0;
  font-size: 11px;
  font-weight: 700;
  color: #475569;
  background: #eef2ff;
  border: 1px solid #c7d2fe;
  border-radius: 999px;
  padding: 3px 8px;
}

.file-list {
  display: flex;
  flex-direction: column;
}

.file-card {
  border-top: 1px solid #e2e8f0;
}

.file-card:first-child {
  border-top: none;
}

.file-card-main {
  position: relative;
  width: 100%;
  display: flex;
  align-items: flex-start;
  gap: 10px;
  padding: 12px;
  background: transparent;
  border: none;
  color: #2563eb;
  font-weight: 600;
  cursor: pointer;
  text-decoration: none;
  transition: background 0.2s ease, color 0.2s ease;
  font: inherit;
  text-align: left;
}

.file-card-main:hover {
  background: #eef6ff;
}

.file-card-main.static {
  cursor: default;
  color: inherit;
}

.file-card-main.static:hover {
  background: transparent;
}

.file-card.clickable.match .file-card-main {
  color: #7c3aed;
}

.file-card.clickable.match .file-card-main:hover {
  background: #f5f3ff;
}

.file-card-icon {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  background: #ffffff;
  border: 1px solid #dbe3ee;
  color: #2563eb;
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
}

.file-card.match .file-card-icon {
  color: #7c3aed;
}

.file-card.dir .file-card-icon {
  color: #0f766e;
}

.file-card-content {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 6px;
}

.file-card-title-row {
  display: flex;
  align-items: center;
  gap: 8px;
  min-width: 0;
  flex-wrap: wrap;
}

.file-card-label {
  min-width: 0;
  word-break: break-word;
  font-size: 13px;
  color: #0f172a;
}

.file-card-label.highlight {
  color: #5b21b6;
  font-weight: 700;
}

.file-card-kind {
  flex-shrink: 0;
  font-size: 10px;
  font-weight: 700;
  letter-spacing: 0.02em;
  text-transform: uppercase;
  color: #475569;
  background: #e2e8f0;
  border-radius: 999px;
  padding: 3px 7px;
}

.file-card-path {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
  color: #64748b;
  font-size: 11px;
  line-height: 1.4;
}

.file-card-line,
.file-card-meta {
  flex-shrink: 0;
  border-radius: 999px;
  padding: 2px 7px;
  background: #ffffff;
  border: 1px solid #dbe3ee;
  color: #475569;
  font-weight: 600;
}

.file-card-preview {
  margin: 0;
  padding: 10px 12px;
  border-radius: 10px;
  background: #ffffff;
  border: 1px solid #e2e8f0;
  color: #334155;
  font-size: 12px;
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.mono {
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

.result-note {
  margin: 0;
  font-size: 13px;
  color: #64748b;
}

.result-highlights {
  display: flex;
  flex-wrap: wrap;
  gap: 6px;
}

.highlight-pill {
  display: inline-block;
  padding: 4px 12px;
  border-radius: 9999px;
  background: #eef2ff;
  color: #4338ca;
  font-size: 12px;
  font-weight: 600;
}

.preview-modal {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.preview-state,
.preview-error,
.preview-note {
  font-size: 13px;
  color: #64748b;
}

.preview-code {
  margin: 0;
  padding: 14px 16px;
  border-radius: 12px;
  background: #0f172a;
  color: #e2e8f0;
  font-size: 13px;
  line-height: 1.6;
  overflow: auto;
  max-height: 60vh;
  white-space: pre-wrap;
  word-break: break-word;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
}

@media (max-width: 900px) {
  .result-card {
    margin-left: 0;
  }

  .file-section-header {
    align-items: flex-start;
    flex-direction: column;
  }

  .file-card-main {
    padding: 12px 10px;
  }
}
</style>
import { sendMessage } from "../api/chat";
import { fetchFilePreview } from "../api/files";
