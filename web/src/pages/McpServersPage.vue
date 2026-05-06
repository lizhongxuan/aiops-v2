<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { RefreshCcwIcon, PlusIcon, PowerIcon, SaveIcon, Trash2Icon, XIcon, SettingsIcon } from "lucide-vue-next";
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
const editorOpen = ref(false);
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
  editorOpen.value = true;
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
      editorOpen.value = false;
      return;
    }
    const data = await createServerApi(payload);
    if (originalName && originalName !== payload.name) {
      await deleteServerApi(originalName);
    }
    applyItems(data?.items || [], payload.name);
    editorOpen.value = false;
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
    editorOpen.value = false;
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

function editItem(item) {
  selectItem(item);
  editorOpen.value = true;
}

function closeEditor() {
  editorOpen.value = false;
  formError.value = "";
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
      const fresh = createBlankDraft();
      fresh.name = uniqueServerName(items.value);
      Object.assign(draft, fresh);
    }
  },
  { immediate: true },
);

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "MCP 服务器 · aiops-codex";
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
    <div class="mcp-settings-shell">
      <div v-if="formError" class="page-alert error">{{ formError }}</div>
      <div v-else-if="loading" class="page-alert info">正在加载 MCP 服务器...</div>
      <div v-else-if="editorOpen && isDirty" class="page-alert warn">当前有未保存修改，保存后会写入工作区 MCP 配置。</div>

      <section class="mcp-server-panel">
        <div class="mcp-section-header">
          <h2>服务器</h2>
          <div class="mcp-header-actions">
            <button class="compact-btn" type="button" :disabled="saving || loading" @click="refreshAll">
              <RefreshCcwIcon size="15" />
              <span>{{ saving ? "处理中..." : "刷新" }}</span>
            </button>
            <button class="compact-btn primary" type="button" :disabled="saving" @click="createNewServer">
              <PlusIcon size="15" />
              <span>添加服务器</span>
            </button>
          </div>
        </div>

        <label class="search-field">
          <input v-model="searchText" type="search" placeholder="搜索名称 / transport / 状态" />
        </label>

        <div class="runtime-list">
          <article
            v-for="item in filteredItems"
            :key="item.name"
            class="runtime-list-item"
            :class="{ active: item.name === selectedName }"
          >
            <button type="button" class="server-summary" @click="selectItem(item)">
              <strong>{{ item.name }}</strong>
              <span>{{ item.transport }} · {{ item.toolCount }} tools · {{ item.resourceCount }} resources</span>
              <span v-if="item.error" class="runtime-list-error">{{ item.error }}</span>
            </button>
            <div class="server-row-actions">
              <span class="status-dot" :class="statusTone(item.status)">{{ item.status }}</span>
              <button class="icon-btn" type="button" title="刷新当前" :disabled="saving" @click="runServerAction(item.name, 'refresh')">
                <RefreshCcwIcon size="15" />
              </button>
              <button class="icon-btn" type="button" title="编辑配置" :disabled="saving" @click="editItem(item)">
                <SettingsIcon size="15" />
              </button>
              <button
                class="switch-btn"
                type="button"
                :class="{ enabled: !item.disabled && item.status === 'connected' }"
                :disabled="saving"
                @click="runServerAction(item.name, item.disabled || item.status !== 'connected' ? 'open' : 'close')"
              >
                <PowerIcon size="14" />
                <span>{{ item.disabled || item.status !== "connected" ? "打开" : "关闭" }}</span>
              </button>
            </div>
          </article>
        </div>
        <p v-if="!filteredItems.length" class="empty-hint">当前没有 MCP 服务器，点击添加服务器创建。</p>
      </section>

      <section v-if="editorOpen" class="mcp-editor-panel">
        <div class="mcp-section-header">
          <div>
            <h2>{{ draft.originalName ? `更新 ${draft.originalName}` : "连接至自定义 MCP" }}</h2>
            <p>只填写当前 transport 必需字段，其他选项可留空。</p>
          </div>
          <div class="mcp-header-actions">
            <button class="compact-btn" type="button" :disabled="saving" @click="discardDraft">恢复</button>
            <button class="compact-btn danger" type="button" :disabled="saving || !selectedItem" @click="deleteServer">
              <Trash2Icon size="15" />
              <span>删除</span>
            </button>
            <button class="compact-btn primary" type="button" :disabled="saving" @click="saveServer">
              <SaveIcon size="15" />
              <span>{{ saving ? "保存中..." : "保存" }}</span>
            </button>
            <button class="icon-btn" type="button" title="关闭编辑" :disabled="saving" @click="closeEditor">
              <XIcon size="16" />
            </button>
          </div>
        </div>

        <div class="form-grid two-col">
          <label class="field">
            <span>名称</span>
            <input v-model="draft.name" type="text" class="text-input" placeholder="MCP server name" />
          </label>
          <label class="field">
            <span>Transport</span>
            <select v-model="draft.transport" class="text-input">
              <option value="http">http</option>
              <option value="stdio">stdio</option>
            </select>
          </label>
          <label class="field" v-if="draft.transport === 'stdio'">
            <span>启动命令</span>
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
            <textarea v-model="draft.argsText" class="text-input textarea-input" rows="3" placeholder="arg1&#10;arg2" />
          </label>
          <label class="field">
            <span>Env（JSON）</span>
            <textarea v-model="draft.envText" class="text-input textarea-input" rows="3" placeholder="{&#10;  &quot;TOKEN&quot;: &quot;...&quot;&#10;}" />
          </label>
        </div>
      </section>
    </div>
  </section>
</template>

<style scoped>
.mcp-runtime-page {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  background: #ffffff;
  color: #202124;
}

.mcp-settings-shell {
  width: min(100%, 980px);
  margin: 0 auto;
  padding: 32px 32px 56px;
}

.page-alert {
  margin-bottom: 14px;
  border-radius: 10px;
  padding: 10px 12px;
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

.mcp-server-panel,
.mcp-editor-panel {
  border: 1px solid #e5e7eb;
  border-radius: 12px;
  background: #fff;
}

.mcp-server-panel {
  overflow: hidden;
}

.mcp-editor-panel {
  margin-top: 18px;
  padding: 18px;
}

.mcp-section-header {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: 16px;
  padding: 14px 18px;
}

.mcp-editor-panel .mcp-section-header {
  padding: 0 0 16px;
}

.mcp-section-header h2 {
  margin: 0;
  font-size: 16px;
  line-height: 1.4;
  font-weight: 700;
}

.mcp-section-header p {
  margin: 4px 0 0;
  color: #62666d;
  font-size: 13px;
}

.mcp-header-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}

.search-field input,
.text-input {
  width: 100%;
  border-radius: 9px;
  border: 1px solid #e5e7eb;
  background: #fff;
  padding: 10px 12px;
  font: inherit;
  box-sizing: border-box;
}

.search-field {
  display: block;
  padding: 0 18px 14px;
}

.search-field input {
  height: 42px;
}

.textarea-input {
  resize: vertical;
  min-height: 78px;
}

.runtime-list {
  display: flex;
  flex-direction: column;
}

.runtime-list-item {
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 14px;
  min-height: 66px;
  padding: 0 18px;
  border-top: 1px solid #eef0f2;
  background: #fff;
}

.runtime-list-item.active {
  background: #fafafa;
}

.server-summary {
  display: grid;
  gap: 4px;
  min-width: 0;
  border: 0;
  background: transparent;
  color: inherit;
  text-align: left;
  cursor: pointer;
  font: inherit;
}

.server-summary strong {
  overflow: hidden;
  font-size: 15px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.server-summary span {
  overflow: hidden;
  color: #62666d;
  font-size: 12px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.server-row-actions {
  display: flex;
  align-items: center;
  justify-content: flex-end;
  gap: 8px;
  white-space: nowrap;
}

.runtime-list-error,
.runtime-error-text {
  color: #b91c1c;
  font-size: 12px;
}

.status-dot {
  display: inline-flex;
  align-items: center;
  min-width: 88px;
  justify-content: center;
  padding: 5px 8px;
  border-radius: 999px;
  font-size: 12px;
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
  margin: 0;
  padding: 30px 18px;
  border-top: 1px solid #eef0f2;
  color: #62666d;
  font-size: 14px;
  text-align: center;
}

.form-grid {
  display: flex;
  gap: 12px;
  flex-wrap: wrap;
}

.form-grid + .form-grid {
  margin-top: 12px;
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
  min-width: 220px;
  flex: 1 1 280px;
}

.field > span {
  font-size: 13px;
  font-weight: 700;
  color: #202124;
}

.compact-btn,
.icon-btn,
.switch-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 6px;
  height: 36px;
  border-radius: 9px;
  border: 1px solid #e5e7eb;
  background: #f7f7f8;
  color: #202124;
  font: inherit;
  cursor: pointer;
}

.compact-btn {
  padding: 0 12px;
  font-weight: 600;
}

.compact-btn.primary {
  border-color: #f1f3f4;
  background: #f1f3f4;
}

.compact-btn.danger {
  border-color: #fde2e2;
  background: #fff1f1;
  color: #d93025;
}

.icon-btn {
  width: 34px;
  padding: 0;
}

.switch-btn {
  min-width: 76px;
  padding: 0 10px;
  background: #f1f3f4;
}

.switch-btn.enabled {
  border-color: #d2e3fc;
  background: #e8f0fe;
  color: #1a73e8;
}

.compact-btn:disabled,
.icon-btn:disabled,
.switch-btn:disabled {
  cursor: not-allowed;
  opacity: 0.55;
}

.compact-btn:hover:not(:disabled),
.icon-btn:hover:not(:disabled),
.switch-btn:hover:not(:disabled) {
  background: #eceff3;
}

.compact-btn.primary:hover:not(:disabled) {
  background: #e8eaed;
}

.mcp-editor-panel .compact-btn.primary {
  border-color: #5f6368;
  background: #5f6368;
  color: #fff;
}

.mcp-editor-panel .compact-btn.primary:hover:not(:disabled) {
  background: #4b4f55;
}

@media (max-width: 980px) {
  .mcp-settings-shell {
    padding: 28px 18px 36px;
  }

  .runtime-list-item {
    grid-template-columns: 1fr;
    align-items: stretch;
    padding: 12px 14px;
  }

  .server-row-actions {
    justify-content: flex-start;
    flex-wrap: wrap;
  }

  .mcp-section-header {
    flex-direction: column;
    align-items: stretch;
  }

  .mcp-header-actions {
    justify-content: flex-start;
  }
}
</style>
