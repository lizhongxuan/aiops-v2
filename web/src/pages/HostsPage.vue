<script setup>
import { computed, onMounted, ref, watch } from "vue";
import { useRouter } from "vue-router";
import { PlusIcon, SearchIcon, SlidersHorizontalIcon } from "lucide-vue-next";
import { useAppStore } from "../store";
import HostEditorModal from "../components/HostEditorModal.vue";
import { createHost, updateHost } from "../api/hosts";
import { fetchTerminalSessions } from "../api/terminal";
import { buildHostListViewModel } from "../lib/hostListViewModel";

const PAGE_SIZE = 20;

const store = useAppStore();
const router = useRouter();

const searchQuery = ref("");
const filters = ref({
  heartbeat: "all",
  source: "all",
  ssh: "all",
});
const page = ref(1);
const now = ref(new Date());
const terminalSessions = ref([]);
const terminalSessionsLoading = ref(false);
const filterOpen = ref(false);
const editorHost = ref(null);
const editorIntent = ref("create");
const showHostEditor = ref(false);
const pageError = ref("");
const pageNotice = ref("");
const busyAction = ref("");

const heartbeatFilters = [
  { label: "全部", value: "all" },
  { label: "在线", value: "online" },
  { label: "待安装", value: "installing" },
  { label: "离线", value: "offline" },
  { label: "超时", value: "stale" },
];
const sourceFilters = [
  { label: "全部", value: "all" },
  { label: "client", value: "client" },
  { label: "手动", value: "手动" },
  { label: "local", value: "local" },
];
const sshFilters = [
  { label: "全部", value: "all" },
  { label: "可 SSH", value: "可 SSH" },
  { label: "无密码", value: "无密码" },
];

const hostModel = computed(() =>
  buildHostListViewModel({
    hosts: store.snapshot.hosts || [],
    sessions: store.sessionList || [],
    terminalSessions: terminalSessions.value,
    query: searchQuery.value,
    filters: filters.value,
    now: now.value,
    page: page.value,
    pageSize: PAGE_SIZE,
  }),
);

const activeFilterCount = computed(() =>
  ["heartbeat", "source", "ssh"].filter((key) => filters.value[key] && filters.value[key] !== "all").length,
);

const emptyHostMessage = computed(() =>
  hostModel.value.allRows.length ? "暂无符合条件的主机。" : "暂无主机，点击接入主机添加。",
);

function clearMessage(kind = "all") {
  if (kind === "all" || kind === "error") pageError.value = "";
  if (kind === "all" || kind === "notice") pageNotice.value = "";
}

function pushError(message) {
  pageNotice.value = "";
  pageError.value = message;
}

function pushNotice(message) {
  pageError.value = "";
  pageNotice.value = message;
}

async function refreshInventory() {
  clearMessage();
  terminalSessionsLoading.value = true;
  try {
    const [, , terminalPayload] = await Promise.all([
      store.fetchState(),
      store.fetchSessions(),
      fetchTerminalSessions(),
    ]);
    terminalSessions.value = terminalPayload?.sessions || terminalPayload?.items || [];
    now.value = new Date();
  } catch (_err) {
    pushError("加载主机列表失败");
  } finally {
    terminalSessionsLoading.value = false;
  }
}

function openCreateModal() {
  editorHost.value = null;
  editorIntent.value = "create";
  showHostEditor.value = true;
}

function openInstallModal(row) {
  editorHost.value = row.raw;
  editorIntent.value = "install";
  showHostEditor.value = true;
}

function openReinstallModal(row) {
  editorHost.value = row.raw;
  editorIntent.value = "reinstall";
  showHostEditor.value = true;
}

async function openTerminal(row) {
  if (!row?.canOpenSsh) return;
  const selected = await store.selectHost(row.id);
  if (selected === false) return;
  router.push(`/terminal/${row.id}`);
}

async function openHostSession(row) {
  const ok = await store.createOrActivateSingleHostSessionForHost(row.id, row.raw);
  if (ok === false) return;
  router.push("/");
}

function handlePrimaryAction(row) {
  if (row.primaryAction === "session") {
    void openHostSession(row);
    return;
  }
  if (row.primaryAction === "install") {
    openInstallModal(row);
    return;
  }
  openReinstallModal(row);
}

function primaryActionLabel(row) {
  if (row.primaryAction === "session") return "会话";
  if (row.primaryAction === "install") return "安装";
  return "重装";
}

async function saveHost(payload) {
  clearMessage();
  const isExistingHost = Boolean(editorHost.value?.id);
  busyAction.value = isExistingHost ? `update:${editorHost.value.id}` : "create";
  try {
    await (isExistingHost ? updateHost(editorHost.value.id, payload) : createHost(payload));
    showHostEditor.value = false;
    await refreshInventory();
    pushNotice(payload.installViaSsh ? "主机已保存，并已触发 SSH 安装。" : "主机信息已保存。");
  } catch (_err) {
    pushError("保存主机失败");
  } finally {
    busyAction.value = "";
  }
}

function setFilter(kind, value) {
  filters.value = { ...filters.value, [kind]: value };
}

function prevPage() {
  if (hostModel.value.canPrev) {
    page.value -= 1;
  }
}

function nextPage() {
  if (hostModel.value.canNext) {
    page.value += 1;
  }
}

watch([searchQuery, filters], () => {
  page.value = 1;
}, { deep: true });

onMounted(async () => {
  await refreshInventory();
});
</script>

<template>
  <section class="hosts-redesign-page">
    <div class="hosts-redesign-inner">
      <div v-if="pageNotice" class="hosts-alert is-success">
        <span>{{ pageNotice }}</span>
        <button type="button" @click="clearMessage('notice')">关闭</button>
      </div>
      <div v-if="pageError" class="hosts-alert is-error">
        <span>{{ pageError }}</span>
        <button type="button" @click="clearMessage('error')">关闭</button>
      </div>

      <div class="hosts-stats-grid" aria-label="主机统计">
        <article v-for="stat in hostModel.stats" :key="stat.label" class="hosts-stat-card">
          <strong>{{ stat.value }}</strong>
          <span>{{ stat.label }}</span>
        </article>
      </div>

      <div class="hosts-toolbar">
        <label class="hosts-search-field">
          <SearchIcon :size="18" />
          <input
            v-model="searchQuery"
            type="search"
            placeholder="按 IP + 用户名检索，例如 10.0.2.15 root"
            aria-label="搜索主机"
          />
        </label>
        <div class="hosts-toolbar-actions">
          <div class="hosts-filter-wrap">
            <button class="hosts-filter-button" type="button" @click="filterOpen = !filterOpen">
              <SlidersHorizontalIcon :size="16" />
              <span>筛选</span>
              <span v-if="activeFilterCount" class="hosts-filter-count">{{ activeFilterCount }}</span>
            </button>
            <div v-if="filterOpen" class="hosts-filter-popover">
              <div class="hosts-filter-group">
                <span>心跳</span>
                <div>
                  <button
                    v-for="item in heartbeatFilters"
                    :key="item.value"
                    type="button"
                    :class="{ active: filters.heartbeat === item.value }"
                    @click="setFilter('heartbeat', item.value)"
                  >
                    {{ item.label }}
                  </button>
                </div>
              </div>
              <div class="hosts-filter-group">
                <span>来源</span>
                <div>
                  <button
                    v-for="item in sourceFilters"
                    :key="item.value"
                    type="button"
                    :class="{ active: filters.source === item.value }"
                    @click="setFilter('source', item.value)"
                  >
                    {{ item.label }}
                  </button>
                </div>
              </div>
              <div class="hosts-filter-group">
                <span>SSH</span>
                <div>
                  <button
                    v-for="item in sshFilters"
                    :key="item.value"
                    type="button"
                    :class="{ active: filters.ssh === item.value }"
                    @click="setFilter('ssh', item.value)"
                  >
                    {{ item.label }}
                  </button>
                </div>
              </div>
            </div>
          </div>
          <button class="hosts-connect-button" type="button" @click="openCreateModal">
            <PlusIcon :size="16" />
            <span>接入主机</span>
          </button>
        </div>
      </div>

      <div class="hosts-table-shell">
        <table>
          <thead>
            <tr>
              <th>主机 IP / 用户名</th>
              <th>心跳情况</th>
              <th>会话</th>
              <th>来源 / SSH</th>
              <th>操作</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="row in hostModel.pageRows" :key="row.id">
              <td>
                <div class="hosts-main-cell">
                  <strong>{{ row.title }}</strong>
                  <span>{{ row.subtitle }}</span>
                </div>
              </td>
              <td>
                <span class="hosts-status-pill" :class="`is-${row.heartbeat}`">
                  <span></span>
                  {{ row.heartbeatLabel }}
                </span>
              </td>
              <td class="hosts-session-count">{{ row.sessionCount }}</td>
              <td>
                <div class="hosts-source-cell">
                  <strong>{{ row.sourceLabel }}</strong>
                  <span>{{ row.sshLabel }}</span>
                </div>
              </td>
              <td>
                <div class="hosts-row-actions">
                  <button
                    type="button"
                    class="hosts-action-button is-ssh"
                    :class="{ disabled: !row.canOpenSsh }"
                    :disabled="!row.canOpenSsh"
                    @click="openTerminal(row)"
                  >
                    {{ row.canOpenSsh ? "SSH" : "禁用" }}
                  </button>
                  <button type="button" class="hosts-action-button" @click="handlePrimaryAction(row)">
                    {{ primaryActionLabel(row) }}
                  </button>
                </div>
              </td>
            </tr>
            <tr v-if="!hostModel.pageRows.length">
              <td colspan="5">
                <div class="hosts-empty-state">{{ emptyHostMessage }}</div>
              </td>
            </tr>
          </tbody>
        </table>
      </div>

      <div class="hosts-pagination">
        <span>共 {{ hostModel.total }} 台主机</span>
        <div>
          <button type="button" :disabled="!hostModel.canPrev" @click="prevPage">上一页</button>
          <button type="button" :disabled="!hostModel.canNext" @click="nextPage">下一页</button>
        </div>
      </div>
    </div>
  </section>

  <HostEditorModal
    v-if="showHostEditor"
    :host="editorHost"
    :intent="editorIntent"
    @close="showHostEditor = false"
    @save="saveHost"
  />
</template>

<style scoped>
.hosts-redesign-page {
  display: flex;
  flex: 1 1 auto;
  min-height: 0;
  background: #f7f8f6;
  color: #111;
}

.hosts-redesign-inner {
  display: flex;
  flex: 1 1 auto;
  flex-direction: column;
  min-height: 0;
  width: min(100%, 1460px);
  margin: 0 auto;
  padding: 22px 38px 20px;
}

.hosts-connect-button,
.hosts-filter-button,
.hosts-pagination button,
.hosts-action-button,
.hosts-filter-group button,
.hosts-alert button {
  border: 1px solid #d5d8d2;
  background: #fff;
  color: #111;
  cursor: pointer;
  font: inherit;
}

.hosts-connect-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-width: 116px;
  height: 44px;
  padding: 0 18px;
  border: 0;
  border-radius: 999px;
  background: #8bd3a7;
  color: #fff;
  font-weight: 700;
}

.hosts-alert {
  display: flex;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 14px;
  padding: 10px 14px;
  border: 1px solid #d5d8d2;
  border-radius: 8px;
  font-size: 14px;
}

.hosts-alert.is-success {
  background: #eef8f1;
  color: #176b36;
}

.hosts-alert.is-error {
  background: #fff0f0;
  color: #9f2424;
}

.hosts-alert button {
  border: 0;
  background: transparent;
  font-size: 13px;
}

.hosts-stats-grid {
  display: grid;
  grid-template-columns: repeat(3, minmax(0, 1fr));
  gap: 12px;
  margin-bottom: 12px;
}

.hosts-stat-card {
  display: flex;
  align-items: center;
  gap: 12px;
  min-height: 58px;
  padding: 10px 14px;
  border-radius: 8px;
  background: #f1f3f0;
}

.hosts-stat-card strong {
  display: block;
  min-width: 28px;
  font-size: 24px;
  line-height: 1.1;
  font-weight: 800;
}

.hosts-stat-card span {
  color: #6b6f69;
  font-size: 14px;
}

.hosts-toolbar {
  position: relative;
  display: grid;
  grid-template-columns: minmax(0, 1fr) auto;
  align-items: center;
  gap: 12px;
  margin-bottom: 12px;
}

.hosts-search-field {
  display: flex;
  align-items: center;
  gap: 12px;
  height: 46px;
  padding: 0 14px;
  border: 1px solid #c9cdc6;
  border-radius: 8px;
  background: #f7f8f6;
  color: #6b6f69;
}

.hosts-search-field input {
  width: 100%;
  border: 0;
  outline: none;
  background: transparent;
  color: #111;
  font: inherit;
}

.hosts-search-field input::placeholder {
  color: #6b6f69;
}

.hosts-toolbar-actions {
  display: flex;
  align-items: center;
  gap: 10px;
}

.hosts-filter-wrap {
  position: relative;
}

.hosts-filter-button {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  gap: 8px;
  min-width: 76px;
  height: 44px;
  padding: 0 16px;
  border-radius: 999px;
  background: #fff;
  box-shadow: 0 1px 2px rgba(17, 17, 17, 0.05);
  font-weight: 700;
}

.hosts-filter-count {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  min-width: 18px;
  height: 18px;
  border-radius: 999px;
  background: #8bd3a7;
  color: #fff;
  font-size: 12px;
}

.hosts-filter-popover {
  position: absolute;
  z-index: 5;
  top: calc(100% + 8px);
  right: 0;
  width: 320px;
  padding: 14px;
  border: 1px solid #d5d8d2;
  border-radius: 8px;
  background: #fff;
  box-shadow: 0 18px 45px rgba(17, 17, 17, 0.14);
}

.hosts-filter-group + .hosts-filter-group {
  margin-top: 12px;
}

.hosts-filter-group > span {
  display: block;
  margin-bottom: 8px;
  color: #6b6f69;
  font-size: 13px;
  font-weight: 700;
}

.hosts-filter-group div {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.hosts-filter-group button {
  min-height: 30px;
  padding: 0 12px;
  border-radius: 999px;
  font-size: 13px;
}

.hosts-filter-group button.active {
  border-color: #8bd3a7;
  background: #e6f5eb;
  color: #08652b;
  font-weight: 700;
}

.hosts-table-shell {
  flex: 1 1 auto;
  min-height: 0;
  overflow: auto;
  border: 1px solid #d5d8d2;
  border-radius: 8px;
  background: #f1f3f0;
}

.hosts-table-shell table {
  width: 100%;
  min-width: 700px;
  border-collapse: collapse;
  table-layout: fixed;
}

.hosts-table-shell th {
  height: 48px;
  border-bottom: 1px solid #d5d8d2;
  color: #6b6f69;
  font-size: 14px;
  font-weight: 600;
  text-align: left;
}

.hosts-table-shell th,
.hosts-table-shell td {
  padding: 0 14px;
}

.hosts-table-shell th:nth-child(1) { width: auto; }
.hosts-table-shell th:nth-child(2) { width: 118px; }
.hosts-table-shell th:nth-child(3) { width: 64px; }
.hosts-table-shell th:nth-child(4) { width: 124px; }
.hosts-table-shell th:nth-child(5) { width: 166px; }

.hosts-table-shell td {
  height: 76px;
  border-bottom: 1px solid #d5d8d2;
  vertical-align: middle;
}

.hosts-table-shell tr:last-child td {
  border-bottom: 0;
}

.hosts-main-cell,
.hosts-source-cell {
  display: flex;
  flex-direction: column;
  min-width: 0;
}

.hosts-main-cell strong {
  overflow: hidden;
  font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, "Liberation Mono", monospace;
  font-size: 18px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hosts-main-cell span,
.hosts-source-cell span {
  overflow: hidden;
  color: #6b6f69;
  font-size: 13px;
  line-height: 1.4;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.hosts-source-cell strong {
  font-size: 15px;
  line-height: 1.5;
  font-weight: 600;
}

.hosts-status-pill {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  min-width: 64px;
  height: 30px;
  padding: 0 10px;
  border-radius: 999px;
  background: #fff;
  font-size: 13px;
  font-weight: 700;
}

.hosts-status-pill span {
  width: 10px;
  height: 10px;
  border-radius: 999px;
  background: #2ec46d;
}

.hosts-status-pill.is-online {
  color: #08652b;
  background: #e9f8ef;
}

.hosts-status-pill.is-stale,
.hosts-status-pill.is-installing {
  color: #9a5b00;
  background: #fff4dc;
}

.hosts-status-pill.is-stale span,
.hosts-status-pill.is-installing span {
  background: #f5a623;
}

.hosts-status-pill.is-offline {
  color: #ad2222;
  background: #ffe8e8;
}

.hosts-status-pill.is-offline span {
  background: #ef5b5b;
}

.hosts-session-count {
  font-size: 16px;
  font-weight: 700;
}

.hosts-row-actions {
  display: flex;
  align-items: center;
  gap: 8px;
  white-space: nowrap;
}

.hosts-action-button {
  min-width: 64px;
  height: 38px;
  padding: 0 16px;
  border-radius: 999px;
  background: #fff;
  font-weight: 700;
}

.hosts-action-button.is-ssh:not(.disabled) {
  border-color: #8bd3a7;
  background: #8bd3a7;
  color: #fff;
}

.hosts-action-button.disabled,
.hosts-pagination button:disabled {
  cursor: not-allowed;
  border-color: transparent;
  background: #e9ebe7;
  color: #6b6f69;
}

.hosts-empty-state {
  display: flex;
  align-items: center;
  justify-content: center;
  min-height: 180px;
  color: #6b6f69;
}

.hosts-pagination {
  display: flex;
  align-items: center;
  justify-content: center;
  gap: 18px;
  flex: 0 0 auto;
  margin-top: 10px;
  color: #6b6f69;
  font-size: 13px;
}

.hosts-pagination div {
  display: flex;
  gap: 10px;
}

.hosts-pagination button {
  min-width: 84px;
  height: 40px;
  border-radius: 999px;
  font-weight: 700;
}

@media (max-width: 900px) {
  .hosts-redesign-inner {
    padding: 16px 16px 18px;
  }

  .hosts-stats-grid,
  .hosts-toolbar {
    grid-template-columns: 1fr;
  }

  .hosts-toolbar-actions {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    gap: 10px;
  }

  .hosts-filter-button,
  .hosts-connect-button {
    width: 100%;
  }

  .hosts-stat-card {
    min-height: 52px;
  }

  .hosts-filter-popover {
    left: 0;
    right: auto;
    width: min(320px, calc(100vw - 32px));
  }
}
</style>
