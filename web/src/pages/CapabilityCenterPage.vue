<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { ArrowLeftIcon } from "lucide-vue-next";
import { useAppStore } from "../store";
import { fetchCapabilityBindings } from "../api/capabilityCenter";

const router = useRouter();
const store = useAppStore();

const activeTab = ref("skills");
const loading = ref(false);

const skills = computed(() => (Array.isArray(store.skillCatalog) ? store.skillCatalog : []));
const mcps = computed(() => (Array.isArray(store.mcpCatalog) ? store.mcpCatalog : []));
const bindings = ref([]);

let previousTitle = "";

async function fetchBindings() {
  try {
    const data = await fetchCapabilityBindings();
    bindings.value = Array.isArray(data.items) ? data.items : [];
  } catch {
    bindings.value = [];
  }
}

async function refreshAll() {
  loading.value = true;
  try {
    if (typeof store.fetchSkillCatalog === "function") await store.fetchSkillCatalog();
    if (typeof store.fetchMcpCatalog === "function") await store.fetchMcpCatalog();
    await fetchBindings();
  } finally {
    loading.value = false;
  }
}

function goBack() {
  router.push("/settings");
}

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "能力中心 · Settings";
  void refreshAll();
});

onBeforeUnmount(() => {
  if (previousTitle) document.title = previousTitle;
});
</script>

<template>
  <section class="cap-page">
    <header class="cap-hero">
      <div class="cap-hero-copy">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon size="16" />
          <span>返回设置</span>
        </button>
        <div class="cap-kicker">Settings / Capability Center</div>
        <h1>能力中心</h1>
        <p>统一管理 Skills、MCP Servers 和绑定关系。</p>
      </div>
      <div class="cap-hero-stats">
        <div class="cap-stat"><span>Skills</span><strong>{{ skills.length }}</strong></div>
        <div class="cap-stat"><span>MCP Servers</span><strong>{{ mcps.length }}</strong></div>
        <div class="cap-stat"><span>Bindings</span><strong>{{ bindings.length }}</strong></div>
      </div>
    </header>

    <n-tabs v-model:value="activeTab" type="line">
      <n-tab-pane name="skills" tab="Skills">
        <n-card>
          <template #header>Skills Catalog</template>
          <n-data-table
            v-if="skills.length"
            :columns="[
              { title: 'ID', key: 'id' },
              { title: '名称', key: 'name' },
              { title: '来源', key: 'source', render: (row) => row.source || '-' },
              { title: '状态', key: 'status', render: (row) => row.status || 'active' },
              { title: '启用', key: 'enabled', render: (row) => (row.enabled || row.defaultEnabled) ? '是' : '否' },
            ]"
            :data="skills"
            :row-key="(row) => row.id"
            :bordered="false"
            size="small"
          />
          <n-empty v-else description="暂无 Skills 数据。" />
        </n-card>
      </n-tab-pane>

      <n-tab-pane name="mcps" tab="MCP Servers">
        <n-card>
          <template #header>MCP Servers Catalog</template>
          <n-data-table
            v-if="mcps.length"
            :columns="[
              { title: 'ID', key: 'id' },
              { title: '名称', key: 'name' },
              { title: '类型', key: 'type', render: (row) => row.type || '-' },
              { title: '来源', key: 'source', render: (row) => row.source || '-' },
              { title: '权限', key: 'permission', render: (row) => row.permission || '-' },
            ]"
            :data="mcps"
            :row-key="(row) => row.id"
            :bordered="false"
            size="small"
          />
          <n-empty v-else description="暂无 MCP Servers 数据。" />
        </n-card>
      </n-tab-pane>

      <n-tab-pane name="bindings" tab="Bindings">
        <n-card>
          <template #header>Capability Bindings</template>
          <n-data-table
            v-if="bindings.length"
            :columns="[
              { title: 'ID', key: 'id' },
              { title: 'Source', key: 'source', render: (row) => `${row.sourceType}:${row.sourceId}` },
              { title: 'Target', key: 'target', render: (row) => `${row.targetType}:${row.targetId}` },
              { title: '状态', key: 'status', render: (row) => row.status || 'active' },
            ]"
            :data="bindings"
            :row-key="(row) => row.id"
            :bordered="false"
            size="small"
          />
          <n-empty v-else description="暂无绑定数据。" />
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </section>
</template>

<style scoped>
.cap-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.cap-hero {
  display: flex;
  justify-content: space-between;
  gap: 18px;
  padding: 22px;
  border-radius: 24px;
  background: rgba(255, 255, 255, 0.88);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 18px 40px rgba(15, 23, 42, 0.05);
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

.cap-kicker {
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

.cap-hero h1 { margin: 12px 0 8px; font-size: 30px; }
.cap-hero p { margin: 0; color: #475569; line-height: 1.7; }

.cap-hero-stats {
  display: grid;
  grid-template-columns: repeat(3, minmax(120px, 1fr));
  gap: 12px;
  align-self: end;
}

.cap-stat {
  padding: 14px 16px;
  border-radius: 18px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.9);
}
.cap-stat span { display: block; color: #64748b; font-size: 12px; }
.cap-stat strong { display: block; margin-top: 8px; font-size: 20px; color: #0f172a; }

.tab-bar {
  display: flex;
  gap: 4px;
  padding: 4px;
  border-radius: 14px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  width: fit-content;
}

.tab-bar button {
  padding: 10px 20px;
  border: none;
  border-radius: 10px;
  background: transparent;
  font: inherit;
  cursor: pointer;
  color: #475569;
  font-weight: 500;
}

.tab-bar button.active {
  background: #0f172a;
  color: #fff;
}

.loading-hint { color: #64748b; font-size: 14px; }

.section-card {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}

.section-card h2 { margin: 0 0 14px; font-size: 18px; }

.data-table {
  width: 100%;
  border-collapse: collapse;
}

.data-table th,
.data-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
  font-size: 13px;
}

.data-table th {
  color: #64748b;
  font-weight: 600;
  font-size: 12px;
}

.data-table tbody tr:hover { background: #f8fafc; }

.empty-hint { color: #64748b; font-size: 13px; margin: 14px 0 0; }

@media (max-width: 760px) {
  .cap-page { padding: 16px; }
  .cap-hero { flex-direction: column; }
  .cap-hero-stats { grid-template-columns: 1fr; }
}
</style>
