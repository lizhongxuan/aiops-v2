<script setup>
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { ArrowLeftIcon, PlusIcon, SaveIcon, SearchIcon, Trash2Icon } from "lucide-vue-next";
import { useAppStore } from "../store";

const router = useRouter();
const store = useAppStore();

const searchText = ref("");
const formError = ref("");
const selectedId = ref("");
const saving = ref(false);
let previousTitle = "";

const draft = reactive(createBlankMcpDraft());

const catalog = computed(() => (Array.isArray(store.mcpCatalog) ? store.mcpCatalog : []));
const normalizedCatalog = computed(() => catalog.value.map(normalizeMcpItem));
const filteredCatalog = computed(() => normalizedCatalog.value.filter(matchesMcpSearch));
const selectedItem = computed(() => normalizedCatalog.value.find((item) => item.id === selectedId.value) || null);
const isDirty = computed(() => itemSignature(draft) !== itemSignature(selectedItem.value));

function compactText(value) {
  return typeof value === "string" ? value.trim() : String(value || "").trim();
}

function generateUniqueId(prefix, items) {
  const existing = new Set((items || []).map((item) => String(item.id || "").trim()).filter(Boolean));
  let index = 1;
  let candidate = `${prefix}-${index}`;
  while (existing.has(candidate)) {
    index += 1;
    candidate = `${prefix}-${index}`;
  }
  return candidate;
}

function createBlankMcpDraft(seed = 1) {
  return {
    originalId: "",
    id: `custom-mcp-${seed}`,
    name: "Custom MCP",
    type: "http",
    source: "local",
    defaultEnabled: false,
    permission: "readonly",
    requiresExplicitUserApproval: false,
  };
}

function normalizeMcpPermission(value, enabled) {
  const permission = compactText(value).toLowerCase();
  if (permission === "readonly" || permission === "read-only") return "readonly";
  if (permission === "readwrite" || permission === "read-write") return "readwrite";
  return enabled ? "readonly" : "readonly";
}

function normalizeMcpItem(item) {
  const defaultEnabled = typeof item?.defaultEnabled === "boolean" ? item.defaultEnabled : Boolean(item?.enabled);
  return {
    originalId: String(item?.originalId || item?.id || ""),
    id: String(item?.id || ""),
    name: String(item?.name || ""),
    type: String(item?.type || "http"),
    source: String(item?.source || "local"),
    defaultEnabled,
    permission: normalizeMcpPermission(item?.permission, defaultEnabled),
    requiresExplicitUserApproval: Boolean(item?.requiresExplicitUserApproval),
  };
}

function matchesMcpSearch(item) {
  const query = compactText(searchText.value).toLowerCase();
  if (!query) return true;
  return [item.id, item.name, item.type, item.source, item.permission]
    .map((value) => compactText(value).toLowerCase())
    .some((value) => value.includes(query));
}

function itemSignature(item) {
  const normalized = item
    ? {
        id: compactText(item.id),
        name: compactText(item.name),
        type: compactText(item.type),
        source: compactText(item.source),
        defaultEnabled: Boolean(item.defaultEnabled),
        permission: normalizeMcpPermission(item.permission, item.defaultEnabled),
        requiresExplicitUserApproval: Boolean(item.requiresExplicitUserApproval),
      }
    : null;
  return JSON.stringify(normalized);
}

function setDraftFromItem(item) {
  const normalized = item ? normalizeMcpItem(item) : createBlankMcpDraft(normalizedCatalog.value.length + 1);
  draft.originalId = normalized.originalId || normalized.id || "";
  draft.id = normalized.id || `custom-mcp-${normalizedCatalog.value.length + 1}`;
  draft.name = normalized.name || "Custom MCP";
  draft.type = normalized.type || "http";
  draft.source = normalized.source || "local";
  draft.defaultEnabled = Boolean(normalized.defaultEnabled);
  draft.permission = normalizeMcpPermission(normalized.permission, normalized.defaultEnabled);
  draft.requiresExplicitUserApproval = Boolean(normalized.requiresExplicitUserApproval);
  formError.value = "";
}

function createNewMcp() {
  const nextId = generateUniqueId("custom-mcp", normalizedCatalog.value);
  selectedId.value = nextId;
  setDraftFromItem({
    id: nextId,
    name: "Custom MCP",
    type: "http",
    source: "local",
    defaultEnabled: false,
    permission: "readonly",
    requiresExplicitUserApproval: false,
  });
}

function buildMcpCatalogPayload(item) {
  const normalized = normalizeMcpItem(item);
  return {
    id: normalized.id,
    name: normalized.name,
    type: normalized.type,
    source: normalized.source,
    defaultEnabled: normalized.defaultEnabled,
    permission: normalized.permission,
    requiresExplicitUserApproval: normalized.requiresExplicitUserApproval,
  };
}

function replaceCatalogItem(originalId, payload) {
  const next = Array.isArray(store.mcpCatalog) ? [...store.mcpCatalog] : [];
  const nextItem = {
    id: payload.id,
    name: payload.name,
    type: payload.type,
    source: payload.source,
    defaultEnabled: payload.defaultEnabled,
    enabled: payload.defaultEnabled,
    permission: payload.permission,
    requiresExplicitUserApproval: payload.requiresExplicitUserApproval,
  };
  const index = next.findIndex((item) => String(item?.id || "") === String(originalId || payload.id));
  if (index >= 0) {
    next[index] = nextItem;
  } else {
    next.push(nextItem);
  }
  store.mcpCatalog = next;
}

function removeCatalogItem(itemId) {
  const next = Array.isArray(store.mcpCatalog) ? store.mcpCatalog.filter((item) => String(item?.id || "") !== String(itemId)) : [];
  store.mcpCatalog = next;
}

async function refreshCatalog() {
  formError.value = "";
  if (typeof store.fetchMcpCatalog === "function") {
    const result = await store.fetchMcpCatalog();
    if (Array.isArray(result)) {
      store.mcpCatalog = result;
    } else if (Array.isArray(result?.items)) {
      store.mcpCatalog = result.items;
    }
  }
  if (!selectedId.value && normalizedCatalog.value.length) {
    selectedId.value = normalizedCatalog.value[0].id;
  }
}

function selectMcp(item) {
  selectedId.value = item.id;
  setDraftFromItem(item);
}

function discardDraft() {
  if (selectedItem.value) {
    setDraftFromItem(selectedItem.value);
  } else {
    createNewMcp();
  }
}

async function saveMcp() {
  const payload = buildMcpCatalogPayload(draft);
  if (!payload.id) {
    formError.value = "请先填写 MCP ID。";
    return;
  }
  if (!payload.name) {
    formError.value = "请先填写 MCP 名称。";
    return;
  }

  const originalId = compactText(draft.originalId || selectedItem.value?.id || payload.id);
  saving.value = true;
  formError.value = "";
  try {
    if (typeof store.saveMcpCatalogItem === "function") {
      const result = await store.saveMcpCatalogItem(payload);
      if (result && typeof result === "object") {
        const normalized = normalizeMcpItem(result);
        selectedId.value = normalized.id || payload.id;
        draft.originalId = normalized.id || payload.id;
        setDraftFromItem(normalized);
      } else {
        selectedId.value = payload.id;
        draft.originalId = payload.id;
        setDraftFromItem(payload);
      }
    } else {
      replaceCatalogItem(originalId, payload);
      selectedId.value = payload.id;
      draft.originalId = payload.id;
      setDraftFromItem(payload);
    }
  } catch (error) {
    formError.value = String(error?.message || error || "保存 MCP 失败");
  } finally {
    saving.value = false;
  }
}

async function deleteMcp() {
  const targetId = compactText(selectedItem.value?.id || draft.originalId || draft.id);
  if (!targetId) return;
  if (!window.confirm(`确认删除 MCP ${targetId}？`)) return;

  saving.value = true;
  formError.value = "";
  try {
    if (typeof store.deleteMcpCatalogItem === "function") {
      await store.deleteMcpCatalogItem(targetId);
    } else {
      removeCatalogItem(targetId);
    }
    const next = normalizedCatalog.value.find((item) => item.id !== targetId) || normalizedCatalog.value[0] || null;
    if (next) {
      selectMcp(next);
    } else {
      selectedId.value = "";
      setDraftFromItem(null);
    }
  } catch (error) {
    formError.value = String(error?.message || error || "删除 MCP 失败");
  } finally {
    saving.value = false;
  }
}

function goBack() {
  router.push("/settings");
}

function permissionHint(permission) {
  return permission === "readwrite" ? "允许写入或变更。" : "只读接入，不包含变更动作。";
}

function setPageTitle(title) {
  if (typeof document === "undefined") return;
  document.title = title;
}

watch(
  normalizedCatalog,
  (items) => {
    if (!items.length) {
      if (!selectedId.value) {
        selectedId.value = "";
        setDraftFromItem(null);
      }
      return;
    }
    const next = items.find((item) => item.id === selectedId.value) || items[0];
    if (next && next.id !== selectedId.value) {
      selectedId.value = next.id;
    }
    if (next) {
      setDraftFromItem(next);
    }
  },
  { immediate: true },
);

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  setPageTitle("MCP 管理 · Settings");
  void refreshCatalog();
});

onBeforeUnmount(() => {
  if (previousTitle) {
    setPageTitle(previousTitle);
  }
});
</script>

<template>
  <section class="catalog-page">
    <header class="catalog-hero">
      <div class="catalog-hero-copy">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon size="16" />
          <span>返回设置</span>
        </button>
        <div class="catalog-kicker">Settings / MCP</div>
        <h1>MCP 管理</h1>
        <p>维护可供 agent 绑定和调用的 MCP catalog，支持新增、删除、搜索、权限和显式确认设置。</p>
      </div>

      <div class="catalog-hero-stats">
        <div class="catalog-stat">
          <span>总数</span>
          <strong>{{ normalizedCatalog.length }}</strong>
        </div>
        <div class="catalog-stat">
          <span>筛选结果</span>
          <strong>{{ filteredCatalog.length }}</strong>
        </div>
        <div class="catalog-stat">
          <span>当前选择</span>
          <strong>{{ selectedItem?.name || "未选择" }}</strong>
        </div>
      </div>
    </header>

    <div v-if="formError" class="page-alert error">{{ formError }}</div>
    <div v-else-if="isDirty" class="page-alert warn">当前有未保存修改，点击保存后才会写回 catalog。</div>

    <section class="catalog-layout">
      <aside class="catalog-sidebar">
        <div class="sidebar-card">
          <div class="sidebar-head">
            <div>
              <h2>MCP Catalog</h2>
              <p>点击条目查看并编辑 catalog item 详情。</p>
            </div>
            <button class="mini-btn" type="button" @click="createNewMcp">
              <PlusIcon size="14" />
              <span>新增</span>
            </button>
          </div>

          <label class="search-field">
            <SearchIcon size="14" />
            <input v-model="searchText" type="search" placeholder="搜索 ID / 名称 / 类型 / 来源" />
          </label>

          <div class="catalog-list">
            <button
              v-for="item in filteredCatalog"
              :key="item.id"
              type="button"
              class="catalog-list-item"
              :class="{ active: item.id === selectedId }"
              @click="selectMcp(item)"
            >
              <strong>{{ item.name || item.id || "未命名 MCP" }}</strong>
              <span>{{ item.id }}</span>
            </button>
          </div>

          <p v-if="!filteredCatalog.length" class="empty-hint">没有匹配的 MCP。</p>
        </div>
      </aside>

      <main class="catalog-main">
        <section class="section-card">
          <div class="section-head">
            <div>
              <h2>基础信息</h2>
              <p>编辑 catalog item 自身的信息，这里维护的是默认值，不代表当前 Agent Profile 的启用状态。</p>
            </div>
            <div class="header-actions">
              <button class="header-btn secondary" type="button" @click="discardDraft">恢复</button>
              <button class="header-btn secondary" type="button" :disabled="saving || !selectedItem && !draft.id" @click="deleteMcp">
                <Trash2Icon size="15" />
                <span>删除</span>
              </button>
              <button class="header-btn primary" type="button" :disabled="saving" @click="saveMcp">
                <SaveIcon size="15" />
                <span>{{ saving ? "保存中..." : "保存" }}</span>
              </button>
            </div>
          </div>

          <div class="form-grid two-col">
            <label class="field">
              <span>MCP ID</span>
              <input v-model="draft.id" type="text" class="text-input" />
            </label>
            <label class="field">
              <span>MCP 名称</span>
              <input v-model="draft.name" type="text" class="text-input" />
            </label>
            <label class="field">
              <span>Type</span>
              <select v-model="draft.type" class="text-input">
                <option value="stdio">stdio</option>
                <option value="http">http</option>
                <option value="sse">sse</option>
                <option value="claudeai-proxy">claudeai-proxy</option>
              </select>
            </label>
            <label class="field">
              <span>来源</span>
              <input v-model="draft.source" type="text" class="text-input" placeholder="built-in / local / plugin" />
            </label>
            <label class="field">
              <span>权限</span>
              <n-select v-model:value="draft.permission" :options="[{label:'readonly',value:'readonly'},{label:'readwrite',value:'readwrite'}]" />
            </label>
            <label class="field">
              <span>显式确认</span>
              <n-select v-model:value="draft.requiresExplicitUserApproval" :options="[{label:'false',value:false},{label:'true',value:true}]" />
            </label>
          </div>

          <div class="toggle-row">
            <n-switch v-model:value="draft.defaultEnabled" />
            <span style="margin-left:8px;">默认启用</span>
            <div class="toggle-hint">
              {{ permissionHint(draft.permission) }}
            </div>
          </div>
        </section>

        <section class="section-card preview-card">
          <div class="section-head">
            <div>
              <h2>预览</h2>
              <p>快速查看当前 MCP 的展示信息。</p>
            </div>
          </div>

          <div class="preview-group">
            <div class="preview-label">展示</div>
            <div class="preview-chip-row">
              <span class="preview-chip">{{ draft.name || "未命名 MCP" }}</span>
              <span class="preview-chip">{{ draft.id || "no-id" }}</span>
              <span class="preview-chip">{{ draft.type || "http" }}</span>
              <span class="preview-chip">{{ draft.permission }}</span>
            </div>
          </div>

          <div class="preview-group">
            <div class="preview-label">状态</div>
            <div class="preview-meta">
              <span>默认启用：{{ draft.defaultEnabled ? "yes" : "no" }}</span>
              <span>显式确认：{{ draft.requiresExplicitUserApproval ? "yes" : "no" }}</span>
              <span>来源：{{ draft.source || "local" }}</span>
            </div>
          </div>
        </section>
      </main>
    </section>
  </section>
</template>

<style scoped>
.catalog-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.catalog-hero {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 22px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.05);
}

.catalog-hero-copy,
.catalog-hero-stats,
.catalog-sidebar,
.catalog-main {
  min-width: 0;
}

.back-link {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  padding: 0;
  border: 0;
  background: transparent;
  color: #475569;
  font: inherit;
  cursor: pointer;
}

.catalog-kicker {
  display: inline-flex;
  margin-top: 12px;
  padding: 6px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 11px;
  font-weight: 700;
  letter-spacing: 0.08em;
  text-transform: uppercase;
}

.catalog-hero h1 {
  margin: 12px 0 8px;
  font-size: 30px;
}

.catalog-hero p {
  margin: 0;
  color: #475569;
  line-height: 1.7;
}

.catalog-hero-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(120px, 1fr));
  gap: 12px;
  align-self: end;
}

.catalog-stat {
  padding: 14px 16px;
  border-radius: 18px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.9);
}

.catalog-stat span {
  display: block;
  color: #64748b;
  font-size: 12px;
}

.catalog-stat strong {
  display: block;
  margin-top: 8px;
  font-size: 20px;
  color: #0f172a;
}

.page-alert {
  padding: 12px 14px;
  border-radius: 14px;
  border: 1px solid transparent;
}

.page-alert.error {
  background: #fef2f2;
  border-color: #fecaca;
  color: #991b1b;
}

.page-alert.warn {
  background: #fffbeb;
  border-color: #fde68a;
  color: #92400e;
}

.catalog-layout {
  display: grid;
  grid-template-columns: 320px minmax(0, 1fr);
  gap: 16px;
}

.sidebar-card,
.section-card {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.sidebar-head,
.section-head {
  display: flex;
  justify-content: space-between;
  align-items: flex-start;
  gap: 12px;
  margin-bottom: 14px;
}

.sidebar-head h2,
.section-head h2 {
  margin: 0;
  font-size: 18px;
}

.sidebar-head p,
.section-head p {
  margin: 6px 0 0;
  color: #64748b;
  line-height: 1.6;
  font-size: 13px;
}

.mini-btn,
.header-btn {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  border: 1px solid transparent;
  border-radius: 12px;
  font: inherit;
  cursor: pointer;
}

.mini-btn {
  padding: 10px 12px;
  background: #0f172a;
  color: #fff;
}

.search-field {
  display: flex;
  align-items: center;
  gap: 10px;
  padding: 10px 12px;
  margin-bottom: 14px;
  border-radius: 14px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.95);
}

.search-field input {
  width: 100%;
  border: 0;
  background: transparent;
  font: inherit;
  outline: none;
}

.catalog-list {
  display: flex;
  flex-direction: column;
  gap: 10px;
  max-height: 64vh;
  overflow: auto;
}

.catalog-list-item {
  display: flex;
  flex-direction: column;
  gap: 4px;
  padding: 12px 14px;
  border-radius: 16px;
  border: 1px solid rgba(226, 232, 240, 0.9);
  background: #fff;
  font: inherit;
  cursor: pointer;
  text-align: left;
}

.catalog-list-item strong {
  color: #0f172a;
}

.catalog-list-item span {
  color: #64748b;
  font-size: 12px;
}

.catalog-list-item.active {
  border-color: rgba(59, 130, 246, 0.45);
  background: #eff6ff;
}

.empty-hint {
  margin: 14px 0 0;
  color: #64748b;
  font-size: 13px;
}

.catalog-main {
  display: grid;
  gap: 16px;
}

.header-actions {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  flex-wrap: wrap;
}

.header-btn {
  padding: 10px 14px;
  background: #fff;
  color: #0f172a;
  border-color: rgba(226, 232, 240, 0.95);
}

.header-btn.primary {
  background: linear-gradient(135deg, #0f172a 0%, #1d4ed8 100%);
  color: #fff;
  border-color: transparent;
}

.header-btn:disabled,
.mini-btn:disabled {
  opacity: 0.6;
  cursor: not-allowed;
}

.form-grid {
  display: grid;
  gap: 14px;
}

.form-grid.two-col {
  grid-template-columns: repeat(2, minmax(0, 1fr));
}

.field {
  display: flex;
  flex-direction: column;
  gap: 8px;
}

.field > span {
  color: #334155;
  font-size: 13px;
  font-weight: 600;
}

.text-input,
.text-area {
  width: 100%;
  padding: 11px 12px;
  border-radius: 12px;
  border: 1px solid rgba(203, 213, 225, 0.95);
  background: #fff;
  font: inherit;
  outline: none;
}

.text-area {
  resize: vertical;
}

.toggle-row {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 12px;
  margin-top: 16px;
  padding-top: 16px;
  border-top: 1px solid rgba(241, 245, 249, 1);
}

.toggle-field {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  color: #0f172a;
}

.toggle-hint {
  color: #64748b;
  font-size: 13px;
}

.preview-card {
  min-height: 220px;
}

.preview-group + .preview-group {
  margin-top: 18px;
}

.preview-label {
  color: #64748b;
  font-size: 12px;
  font-weight: 700;
  margin-bottom: 8px;
}

.preview-chip-row {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.preview-chip {
  padding: 7px 10px;
  border-radius: 999px;
  background: #eff6ff;
  color: #1d4ed8;
  font-size: 12px;
  font-weight: 600;
}

.preview-text {
  margin: 0;
  padding: 14px;
  border-radius: 14px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.95);
  white-space: pre-wrap;
  word-break: break-word;
  color: #0f172a;
  line-height: 1.7;
}

.preview-meta {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  color: #475569;
}

@media (max-width: 1120px) {
  .catalog-layout {
    grid-template-columns: 1fr;
  }

  .catalog-hero {
    flex-direction: column;
  }

  .catalog-hero-stats {
    grid-template-columns: repeat(3, minmax(0, 1fr));
    align-self: stretch;
  }
}

@media (max-width: 760px) {
  .catalog-page {
    padding: 16px;
  }

  .catalog-hero-stats {
    grid-template-columns: 1fr;
  }

  .form-grid.two-col {
    grid-template-columns: 1fr;
  }

  .toggle-row {
    flex-direction: column;
    align-items: flex-start;
  }
}
</style>
