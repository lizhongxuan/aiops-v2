<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { RefreshCcwIcon, PlusIcon, PowerIcon, SaveIcon, Trash2Icon } from "lucide-vue-next";
import {
  createServer as createServerApi,
  deleteServer as deleteServerApi,
  fetchServers as fetchServersApi,
  refreshServers as refreshServersApi,
  runServerAction as runServerActionApi,
  updateServer as updateServerApi,
} from "../api/mcp";

const searchText = ref("");
const loading = ref(false);
const saving = ref(false);
const formError = ref("");
const selectedName = ref("");
const configPath = ref("");
let previousTitle = "";

const draft = reactive(createBlankDraft());
const items = ref([]);

const filteredItems = computed(() => {
  const query = compactText(searchText.value).toLowerCase();
  if (!query) return items.value;
  return items.value.filter((item) =>
    [item.name, item.transport, item.status, item.url, item.command]
      .map((value) => compactText(value).toLowerCase())
      .some((value) => value.includes(query)),
  );
});
const selectedItem = computed(() => items.value.find((item) => item.name === selectedName.value) || null);
const isDirty = computed(() => signatureOfDraft(draft) !== signatureOfItem(selectedItem.value));

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function uniqueServerName(existing = []) {
  const names = new Set(existing.map((item) => compactText(item?.name)).filter(Boolean));
  let index = 1;
  let candidate = `custom-mcp-${index}`;
  while (names.has(candidate)) {
    index += 1;
    candidate = `custom-mcp-${index}`;
  }
  return candidate;
}

function createBlankDraft() {
  return {
    originalName: "",
    name: "",
    transport: "http",
    command: "",
    argsText: "",
    url: "",
    envText: "{}",
    disabled: false,
  };
}

function normalizeItem(item = {}) {
  return {
    name: compactText(item.name),
    transport: compactText(item.transport || "http"),
    command: compactText(item.command),
    args: Array.isArray(item.args) ? item.args.map((entry) => compactText(entry)).filter(Boolean) : [],
    url: compactText(item.url),
    env: item.env && typeof item.env === "object" && !Array.isArray(item.env) ? { ...item.env } : {},
    disabled: Boolean(item.disabled),
    status: compactText(item.status || "disconnected"),
    error: compactText(item.error),
    toolCount: Number(item.toolCount || 0),
    resourceCount: Number(item.resourceCount || 0),
  };
}

function setDraftFromItem(item) {
  const normalized = normalizeItem(item);
  draft.originalName = normalized.name;
  draft.name = normalized.name || uniqueServerName(items.value);
  draft.transport = normalized.transport || "http";
  draft.command = normalized.command || "";
  draft.argsText = normalized.args.join("\n");
  draft.url = normalized.url || "";
  draft.envText = JSON.stringify(normalized.env || {}, null, 2);
  draft.disabled = Boolean(normalized.disabled);
  formError.value = "";
}

function signatureOfDraft(value) {
  return JSON.stringify({
    name: compactText(value?.name),
    transport: compactText(value?.transport),
    command: compactText(value?.command),
    argsText: compactText(value?.argsText),
    url: compactText(value?.url),
    envText: compactText(value?.envText),
    disabled: Boolean(value?.disabled),
  });
}

function signatureOfItem(item) {
  if (!item) return signatureOfDraft(createBlankDraft());
  return JSON.stringify({
    name: compactText(item.name),
    transport: compactText(item.transport),
    command: compactText(item.command),
    argsText: (item.args || []).join("\n"),
    url: compactText(item.url),
    envText: JSON.stringify(item.env || {}, null, 2),
    disabled: Boolean(item.disabled),
  });
}

function parseEnvText(text) {
  const raw = compactText(text);
  if (!raw) return {};
  const parsed = JSON.parse(raw);
  if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) {
    throw new Error("环境变量必须是 JSON 对象");
  }
  return Object.fromEntries(
    Object.entries(parsed).map(([key, value]) => [String(key), String(value ?? "")]),
  );
}

function buildPayloadFromDraft() {
  return {
    name: compactText(draft.name),
    transport: compactText(draft.transport),
    command: compactText(draft.command),
    args: String(draft.argsText || "")
      .split("\n")
      .map((item) => compactText(item))
      .filter(Boolean),
    url: compactText(draft.url),
    env: parseEnvText(draft.envText),
    disabled: Boolean(draft.disabled),
  };
}

function applyItems(nextItems, preferredSelection = "") {
  items.value = Array.isArray(nextItems) ? nextItems.map(normalizeItem) : [];
  const next =
    items.value.find((item) => item.name === preferredSelection) ||
    items.value.find((item) => item.name === selectedName.value) ||
    items.value[0] ||
    null;
  selectedName.value = next?.name || "";
  if (next) {
    setDraftFromItem(next);
  } else if (!draft.name) {
    const fresh = createBlankDraft();
    fresh.name = uniqueServerName(items.value);
    Object.assign(draft, fresh);
  }
}

async function fetchServers() {
  loading.value = true;
  formError.value = "";
  try {
    const data = await fetchServersApi();
    configPath.value = compactText(data?.configPath);
    applyItems(data?.items || []);
  } catch (error) {
    formError.value = String(error?.message || error || "加载 MCP 列表失败");
    applyItems([]);
  } finally {
    loading.value = false;
  }
}

function createNewServer() {
  const fresh = createBlankDraft();
  fresh.name = uniqueServerName(items.value);
  Object.assign(draft, fresh);
  selectedName.value = "";
  formError.value = "";
}

async function saveServer() {
  saving.value = true;
  formError.value = "";
  try {
    const payload = buildPayloadFromDraft();
    if (!payload.name) {
      throw new Error("请先填写 MCP 名称");
    }
    const originalName = compactText(draft.originalName);
    if (originalName && originalName === payload.name) {
      const data = await updateServerApi(originalName, payload);
      applyItems(data?.items || [], payload.name);
      return;
    }
    const data = await createServerApi(payload);
    if (originalName && originalName !== payload.name) {
      await deleteServerApi(originalName);
    }
    applyItems(data?.items || [], payload.name);
  } catch (error) {
    formError.value = String(error?.message || error || "保存 MCP 失败");
  } finally {
    saving.value = false;
  }
}

async function deleteServer() {
  const target = compactText(selectedItem.value?.name || draft.originalName || draft.name);
  if (!target) return;
  if (!window.confirm(`确认删除 MCP ${target}？`)) return;
  saving.value = true;
  formError.value = "";
  try {
    const data = await deleteServerApi(target);
    applyItems(data?.items || []);
  } catch (error) {
    formError.value = String(error?.message || error || "删除 MCP 失败");
  } finally {
    saving.value = false;
  }
}

async function runServerAction(name, action) {
  const target = compactText(name || selectedItem.value?.name);
  if (!target) return;
  saving.value = true;
  formError.value = "";
  try {
    const data = await runServerActionApi(target, action);
    applyItems(data?.items || [], target);
  } catch (error) {
    formError.value = String(error?.message || error || `${action} MCP 失败`);
  } finally {
    saving.value = false;
  }
}

async function refreshAll() {
  saving.value = true;
  formError.value = "";
  try {
    const data = await refreshServersApi();
    applyItems(data?.items || [], selectedName.value);
  } catch (error) {
    formError.value = String(error?.message || error || "刷新 MCP 失败");
  } finally {
    saving.value = false;
  }
}

function selectItem(item) {
  selectedName.value = item.name;
  setDraftFromItem(item);
}

function discardDraft() {
  if (selectedItem.value) {
    setDraftFromItem(selectedItem.value);
    return;
  }
  createNewServer();
}

function statusTone(status = "") {
  const normalized = compactText(status).toLowerCase();
  if (normalized === "connected") return "ok";
  if (normalized === "error") return "error";
  if (normalized === "connecting") return "warn";
  return "muted";
}

watch(
  items,
  (list) => {
    if (!list.length && !draft.name) {
      createNewServer();
    }
  },
  { immediate: true },
);

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "MCP · aiops-codex";
  void fetchServers();
});

onBeforeUnmount(() => {
  if (previousTitle) {
    document.title = previousTitle;
  }
});
</script>

<template>
  <section class="mcp-runtime-page">
    <header class="runtime-hero">
      <div>
        <div class="runtime-kicker">Runtime / MCP</div>
        <h1>MCP</h1>
        <p>查看当前 MCP server 列表，并对工作区配置执行新增、删除、打开、关闭和刷新。</p>
        <p v-if="configPath" class="config-path">写入路径：{{ configPath }}</p>
      </div>
      <div class="runtime-hero-actions">
        <button class="header-btn secondary" type="button" :disabled="saving" @click="createNewServer">
          <PlusIcon size="15" />
          <span>新增</span>
        </button>
        <button class="header-btn primary" type="button" :disabled="saving || loading" @click="refreshAll">
          <RefreshCcwIcon size="15" />
          <span>{{ saving ? "处理中..." : "刷新全部" }}</span>
        </button>
      </div>
    </header>

    <div v-if="formError" class="page-alert error">{{ formError }}</div>
    <div v-else-if="loading" class="page-alert info">正在加载 MCP 列表...</div>
    <div v-else-if="isDirty" class="page-alert warn">当前有未保存修改，点击保存后才会写入工作区 MCP 配置。</div>

    <section class="runtime-layout">
      <aside class="runtime-sidebar">
        <div class="sidebar-card">
          <div class="sidebar-head">
            <div>
              <h2>MCP Servers</h2>
              <p>当前 runtime 已知的 MCP server。</p>
            </div>
          </div>
          <label class="search-field">
            <input v-model="searchText" type="search" placeholder="搜索名称 / transport / 状态" />
          </label>
          <div class="runtime-list">
            <button
              v-for="item in filteredItems"
              :key="item.name"
              type="button"
              class="runtime-list-item"
              :class="{ active: item.name === selectedName }"
              @click="selectItem(item)"
            >
              <div class="runtime-list-title-row">
                <strong>{{ item.name }}</strong>
                <span class="status-dot" :class="statusTone(item.status)">{{ item.status }}</span>
              </div>
              <div class="runtime-list-meta">
                <span>{{ item.transport }}</span>
                <span>{{ item.toolCount }} tools</span>
                <span>{{ item.resourceCount }} resources</span>
              </div>
              <div v-if="item.error" class="runtime-list-error">{{ item.error }}</div>
            </button>
          </div>
          <p v-if="!filteredItems.length" class="empty-hint">当前没有 MCP server。</p>
        </div>
      </aside>

      <main class="runtime-main">
        <section class="section-card">
          <div class="section-head">
            <div>
              <h2>连接配置</h2>
              <p>这里维护的是工作区 MCP runtime 配置，保存后会立即写入并尝试重连。</p>
            </div>
            <div class="header-actions">
              <button class="header-btn secondary" type="button" :disabled="saving" @click="discardDraft">恢复</button>
              <button class="header-btn secondary" type="button" :disabled="saving || !selectedItem" @click="deleteServer">
                <Trash2Icon size="15" />
                <span>删除</span>
              </button>
              <button class="header-btn primary" type="button" :disabled="saving" @click="saveServer">
                <SaveIcon size="15" />
                <span>{{ saving ? "保存中..." : "保存" }}</span>
              </button>
            </div>
          </div>

          <div class="form-grid two-col">
            <label class="field">
              <span>名称</span>
              <input v-model="draft.name" type="text" class="text-input" />
            </label>
            <label class="field">
              <span>Transport</span>
              <select v-model="draft.transport" class="text-input">
                <option value="http">http</option>
                <option value="stdio">stdio</option>
              </select>
            </label>
            <label class="field" v-if="draft.transport === 'stdio'">
              <span>Command</span>
              <input v-model="draft.command" type="text" class="text-input" placeholder="npx / uvx / binary" />
            </label>
            <label class="field" v-else>
              <span>URL</span>
              <input v-model="draft.url" type="text" class="text-input" placeholder="http://127.0.0.1:8088/mcp" />
            </label>
            <label class="field">
              <span>状态</span>
              <select v-model="draft.disabled" class="text-input">
                <option :value="false">open</option>
                <option :value="true">closed</option>
              </select>
            </label>
          </div>

          <div class="form-grid">
            <label class="field">
              <span>Args（每行一个）</span>
              <textarea v-model="draft.argsText" class="text-input textarea-input" rows="4" placeholder="arg1&#10;arg2" />
            </label>
            <label class="field">
              <span>Env（JSON）</span>
              <textarea v-model="draft.envText" class="text-input textarea-input" rows="4" placeholder="{&#10;  &quot;TOKEN&quot;: &quot;...&quot;&#10;}" />
            </label>
          </div>
        </section>

        <section v-if="selectedItem" class="section-card">
          <div class="section-head">
            <div>
              <h2>运行时操作</h2>
              <p>直接控制当前 MCP server 的连接状态。</p>
            </div>
            <div class="header-actions">
              <button class="header-btn secondary" type="button" :disabled="saving" @click="runServerAction(selectedItem.name, 'refresh')">
                <RefreshCcwIcon size="15" />
                <span>刷新当前</span>
              </button>
              <button
                v-if="selectedItem.disabled || selectedItem.status !== 'connected'"
                class="header-btn primary"
                type="button"
                :disabled="saving"
                @click="runServerAction(selectedItem.name, 'open')"
              >
                <PowerIcon size="15" />
                <span>打开</span>
              </button>
              <button v-else class="header-btn secondary" type="button" :disabled="saving" @click="runServerAction(selectedItem.name, 'close')">
                <PowerIcon size="15" />
                <span>关闭</span>
              </button>
            </div>
          </div>

          <div class="preview-group">
            <div class="preview-label">当前状态</div>
            <div class="preview-chip-row">
              <span class="preview-chip">{{ selectedItem.name }}</span>
              <span class="preview-chip">{{ selectedItem.transport }}</span>
              <span class="preview-chip">{{ selectedItem.status }}</span>
              <span class="preview-chip">{{ selectedItem.toolCount }} tools</span>
              <span class="preview-chip">{{ selectedItem.resourceCount }} resources</span>
            </div>
            <p v-if="selectedItem.error" class="runtime-error-text">{{ selectedItem.error }}</p>
          </div>
        </section>
      </main>
    </section>
  </section>
</template>

<style scoped>
.mcp-runtime-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.runtime-hero,
.sidebar-card,
.section-card {
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 16px 40px rgba(15, 23, 42, 0.05);
}

.runtime-hero {
  border-radius: 24px;
  padding: 22px;
  display: flex;
  justify-content: space-between;
  gap: 20px;
}

.runtime-kicker {
  display: inline-flex;
  padding: 6px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.runtime-hero h1 {
  margin: 12px 0 8px;
  font-size: 30px;
}

.runtime-hero p,
.config-path {
  margin: 0;
  color: #475569;
}

.config-path {
  margin-top: 8px;
  font-size: 12px;
}

.runtime-hero-actions,
.header-actions {
  display: flex;
  gap: 10px;
  align-items: flex-start;
  flex-wrap: wrap;
}

.page-alert {
  border-radius: 14px;
  padding: 12px 14px;
  font-size: 14px;
}

.page-alert.error {
  background: #fef2f2;
  color: #b91c1c;
}

.page-alert.warn {
  background: #fff7ed;
  color: #c2410c;
}

.page-alert.info {
  background: #eff6ff;
  color: #1d4ed8;
}

.runtime-layout {
  display: grid;
  grid-template-columns: minmax(260px, 320px) minmax(0, 1fr);
  gap: 18px;
}

.sidebar-card,
.section-card {
  border-radius: 22px;
  padding: 20px;
}

.sidebar-head,
.section-head {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
}

.sidebar-head h2,
.section-head h2 {
  margin: 0;
  font-size: 18px;
}

.sidebar-head p,
.section-head p {
  margin: 4px 0 0;
  color: #64748b;
  font-size: 13px;
}

.search-field input,
.text-input {
  width: 100%;
  border-radius: 14px;
  border: 1px solid rgba(203, 213, 225, 0.95);
  background: #fff;
  padding: 11px 12px;
  font: inherit;
  box-sizing: border-box;
}

.textarea-input {
  resize: vertical;
  min-height: 96px;
}

.runtime-list {
  margin-top: 14px;
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.runtime-list-item {
  width: 100%;
  text-align: left;
  padding: 14px;
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  background: rgba(248, 250, 252, 0.92);
  cursor: pointer;
}

.runtime-list-item.active {
  border-color: rgba(59, 130, 246, 0.5);
  background: rgba(239, 246, 255, 0.95);
}

.runtime-list-title-row,
.runtime-list-meta,
.preview-chip-row,
.form-grid {
  display: flex;
  gap: 10px;
  flex-wrap: wrap;
}

.runtime-list-title-row {
  justify-content: space-between;
  align-items: center;
}

.runtime-list-meta {
  margin-top: 8px;
  color: #64748b;
  font-size: 12px;
}

.runtime-list-error,
.runtime-error-text {
  margin-top: 8px;
  color: #b91c1c;
  font-size: 12px;
}

.status-dot {
  display: inline-flex;
  align-items: center;
  padding: 4px 8px;
  border-radius: 999px;
  font-size: 11px;
  font-weight: 600;
}

.status-dot.ok {
  background: #dcfce7;
  color: #166534;
}

.status-dot.warn {
  background: #fef3c7;
  color: #92400e;
}

.status-dot.error {
  background: #fee2e2;
  color: #b91c1c;
}

.status-dot.muted {
  background: #e2e8f0;
  color: #475569;
}

.empty-hint {
  margin: 14px 0 0;
  color: #64748b;
  font-size: 13px;
}

.runtime-main {
  display: flex;
  flex-direction: column;
  gap: 18px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 220px;
  flex: 1 1 280px;
}

.field > span,
.preview-label {
  font-size: 12px;
  font-weight: 700;
  color: #475569;
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.preview-group {
  display: flex;
  flex-direction: column;
  gap: 10px;
}

.preview-chip {
  display: inline-flex;
  align-items: center;
  padding: 7px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 600;
}

.header-btn {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  border-radius: 12px;
  padding: 10px 14px;
  border: 1px solid transparent;
  font: inherit;
  cursor: pointer;
}

.header-btn.primary {
  background: #2563eb;
  color: #fff;
}

.header-btn.secondary {
  background: #fff;
  color: #0f172a;
  border-color: rgba(203, 213, 225, 0.95);
}

@media (max-width: 980px) {
  .runtime-layout {
    grid-template-columns: 1fr;
  }

  .runtime-hero {
    flex-direction: column;
  }
}
</style>
