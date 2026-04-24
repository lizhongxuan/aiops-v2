<script setup>
import { computed, h, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { ArrowLeftIcon } from "lucide-vue-next";
import { NButton } from "naive-ui";
import { fetchUiCards, previewUiCard, updateUiCard } from "../api/uiCards";

const router = useRouter();

const loading = ref(false);
const cards = ref([]);
const stats = ref({ total: 0, active: 0, draft: 0, disabled: 0, builtIn: 0, custom: 0 });
const activeTab = ref("overview");
const selectedCard = ref(null);
const editDraft = ref(null);
const previewResult = ref(null);
const previewLoading = ref(false);
const debugInput = ref("{}");

const kindLabels = {
  readonly_summary: "只读摘要",
  readonly_chart: "只读图表",
  action_panel: "操作面板",
  form_panel: "表单面板",
  monitor_bundle: "监控聚合",
  remediation_bundle: "修复聚合",
};

const kindGroups = computed(() => {
  const groups = {};
  for (const c of cards.value) {
    const kind = c.kind || "unknown";
    if (!groups[kind]) groups[kind] = { kind, label: kindLabels[kind] || kind, count: 0, items: [] };
    groups[kind].count++;
    groups[kind].items.push(c);
  }
  return Object.values(groups);
});

let previousTitle = "";

async function fetchCards() {
  loading.value = true;
  try {
    const data = await fetchUiCards();
    cards.value = Array.isArray(data.items) ? data.items : [];
    if (data.stats) stats.value = data.stats;
  } catch {
    cards.value = [];
  } finally {
    loading.value = false;
  }
}

function selectCard(card) {
  selectedCard.value = card;
  editDraft.value = null;
  previewResult.value = null;
  activeTab.value = "detail";
}

function startEdit(card) {
  editDraft.value = JSON.parse(JSON.stringify(card));
  activeTab.value = "editor";
}

async function saveEdit() {
  if (!editDraft.value) return;
  try {
    await updateUiCard(editDraft.value.id, editDraft.value);
    editDraft.value = null;
    activeTab.value = "list";
    await fetchCards();
  } catch { /* ignore */ }
}

async function triggerPreview(card) {
  previewLoading.value = true;
  previewResult.value = null;
  try {
    let inputPayload = {};
    try { inputPayload = JSON.parse(debugInput.value); } catch { /* ignore */ }
    previewResult.value = await previewUiCard(card.id, inputPayload);
  } catch { /* ignore */ }
  finally { previewLoading.value = false; }
}

function goBack() {
  router.push("/settings");
}

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "UI 卡片管理 · Settings";
  void fetchCards();
});

onBeforeUnmount(() => {
  if (previousTitle) document.title = previousTitle;
});
</script>

<template>
  <section class="uic-page">
    <header class="uic-hero">
      <div class="uic-hero-copy">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon size="16" />
          <span>返回设置</span>
        </button>
        <div class="uic-kicker">Settings / UI Cards</div>
        <h1>UI 卡片管理</h1>
        <p>管理系统内置和自定义 UI 卡片定义，支持预览和触发调试。</p>
      </div>
      <div class="uic-hero-stats">
        <div class="uic-stat"><span>总计</span><strong>{{ stats.total }}</strong></div>
        <div class="uic-stat"><span>启用</span><strong>{{ stats.active }}</strong></div>
        <div class="uic-stat"><span>草稿</span><strong>{{ stats.draft }}</strong></div>
        <div class="uic-stat"><span>内置</span><strong>{{ stats.builtIn }}</strong></div>
      </div>
    </header>

    <n-tabs v-model:value="activeTab" type="line">
      <n-tab-pane name="overview" tab="类型总览">
        <div class="kind-grid">
          <n-card v-for="g in kindGroups" :key="g.kind" hoverable @click="activeTab = 'list'">
            <div class="kind-label">{{ g.label }}</div>
            <div class="kind-count">{{ g.count }}</div>
            <div class="kind-id">{{ g.kind }}</div>
          </n-card>
        </div>
        <n-empty v-if="!kindGroups.length && !loading" description="暂无卡片定义。" />
      </n-tab-pane>

      <n-tab-pane name="list" tab="卡片列表">
        <n-card>
          <template #header>卡片定义列表</template>
          <n-data-table
            v-if="cards.length"
            :columns="[
              { title: 'ID', key: 'id' },
              { title: '名称', key: 'name' },
              { title: '类型', key: 'kind', render: (row) => kindLabels[row.kind] || row.kind },
              { title: '渲染器', key: 'renderer' },
              { title: '状态', key: 'status' },
              { title: '内置', key: 'builtIn', render: (row) => row.builtIn ? '是' : '否' },
              { title: '操作', key: 'actions', render: (row) => h('div', { style: 'display:flex;gap:6px' }, [h(NButton, { size: 'small', quaternary: true, onClick: () => selectCard(row) }, { default: () => '详情' }), h(NButton, { size: 'small', quaternary: true, onClick: () => startEdit(row) }, { default: () => '编辑' })]) },
            ]"
            :data="cards"
            :row-key="(row) => row.id"
            :bordered="false"
            size="small"
          />
          <n-empty v-else description="暂无卡片定义。" />
        </n-card>
      </n-tab-pane>

      <n-tab-pane name="detail" tab="详情" :disabled="!selectedCard">
        <template v-if="selectedCard">
          <n-card>
            <template #header>{{ selectedCard.name }}</template>
            <n-descriptions :column="1" label-placement="left" bordered size="small">
              <n-descriptions-item label="ID">{{ selectedCard.id }}</n-descriptions-item>
              <n-descriptions-item label="类型">{{ kindLabels[selectedCard.kind] || selectedCard.kind }}</n-descriptions-item>
              <n-descriptions-item label="渲染器">{{ selectedCard.renderer }}</n-descriptions-item>
              <n-descriptions-item label="状态">{{ selectedCard.status }}</n-descriptions-item>
              <n-descriptions-item label="内置">{{ selectedCard.builtIn ? '是' : '否' }}</n-descriptions-item>
              <n-descriptions-item label="版本">{{ selectedCard.version }}</n-descriptions-item>
              <n-descriptions-item label="摘要">{{ selectedCard.summary }}</n-descriptions-item>
            </n-descriptions>
            <div style="margin-top:16px;display:flex;gap:8px;">
              <n-button size="small" @click="startEdit(selectedCard)">编辑</n-button>
              <n-button size="small" @click="triggerPreview(selectedCard)">预览</n-button>
            </div>
            <pre v-if="previewResult" class="preview-output">{{ JSON.stringify(previewResult, null, 2) }}</pre>
          </n-card>
        </template>
      </n-tab-pane>

      <n-tab-pane name="editor" tab="编辑器" :disabled="!editDraft">
        <template v-if="editDraft">
          <n-card>
            <template #header>编辑卡片定义</template>
            <n-form label-placement="top">
              <n-form-item label="名称"><n-input v-model:value="editDraft.name" /></n-form-item>
              <n-form-item label="摘要"><n-input v-model:value="editDraft.summary" /></n-form-item>
              <n-form-item label="状态">
                <n-select v-model:value="editDraft.status" :options="[{label:'active',value:'active'},{label:'draft',value:'draft'},{label:'disabled',value:'disabled'}]" />
              </n-form-item>
            </n-form>
            <div style="margin-top:16px;display:flex;gap:8px;">
              <n-button type="primary" @click="saveEdit">保存</n-button>
              <n-button @click="editDraft = null; activeTab = 'list'">取消</n-button>
            </div>
          </n-card>
        </template>
      </n-tab-pane>

      <n-tab-pane name="debugger" tab="触发调试器">
        <n-card>
          <template #header>触发调试器</template>
          <p class="debug-hint">选择一个卡片并发送模拟输入来预览渲染结果。</p>
          <n-form label-placement="top">
            <n-form-item label="选择卡片">
              <n-select :options="cards.map(c => ({label: `${c.name} (${c.id})`, value: c.id}))" @update:value="(v) => { const c = cards.find(x => x.id === v); if (c) selectedCard = c; }" placeholder="选择卡片…" />
            </n-form-item>
            <n-form-item label="输入 JSON">
              <n-input v-model:value="debugInput" type="textarea" :rows="4" placeholder='{"key": "value"}' />
            </n-form-item>
          </n-form>
          <n-button type="primary" :disabled="!selectedCard || previewLoading" @click="triggerPreview(selectedCard)">
            {{ previewLoading ? '请求中…' : '发送预览' }}
          </n-button>
          <pre v-if="previewResult" class="preview-output">{{ JSON.stringify(previewResult, null, 2) }}</pre>
        </n-card>
      </n-tab-pane>
    </n-tabs>
  </section>
</template>

<style scoped>
.uic-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.uic-hero {
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

.uic-kicker {
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

.uic-hero h1 { margin: 12px 0 8px; font-size: 30px; }
.uic-hero p { margin: 0; color: #475569; line-height: 1.7; }

.uic-hero-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(100px, 1fr));
  gap: 12px;
  align-self: end;
}

.uic-stat {
  padding: 14px 16px;
  border-radius: 18px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.9);
}
.uic-stat span { display: block; color: #64748b; font-size: 12px; }
.uic-stat strong { display: block; margin-top: 8px; font-size: 20px; color: #0f172a; }

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
.tab-bar button.active { background: #0f172a; color: #fff; }
.tab-bar button:disabled { opacity: 0.4; cursor: default; }

.loading-hint { color: #64748b; font-size: 14px; }

.kind-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  gap: 14px;
}

.kind-card {
  padding: 18px;
  border-radius: 18px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  cursor: pointer;
  transition: box-shadow 0.15s;
}
.kind-card:hover { box-shadow: 0 8px 24px rgba(15, 23, 42, 0.08); }
.kind-label { font-weight: 600; font-size: 15px; color: #0f172a; }
.kind-count { font-size: 28px; font-weight: 700; margin: 8px 0 4px; color: #1d4ed8; }
.kind-id { font-size: 11px; color: #94a3b8; font-family: monospace; }

.section-card {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
}
.section-card h2 { margin: 0 0 14px; font-size: 18px; }

.data-table { width: 100%; border-collapse: collapse; }
.data-table th, .data-table td {
  padding: 10px 12px;
  text-align: left;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
  font-size: 13px;
}
.data-table th { color: #64748b; font-weight: 600; font-size: 12px; }
.data-table tbody tr:hover { background: #f8fafc; }
.data-table code { font-size: 12px; background: #f1f5f9; padding: 2px 6px; border-radius: 4px; }

.action-cell { display: flex; gap: 6px; }

.btn-sm {
  padding: 5px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  background: #fff;
  font-size: 12px;
  cursor: pointer;
  color: #334155;
}
.btn-sm:hover { background: #f8fafc; }

.btn-primary {
  padding: 8px 18px;
  border: none;
  border-radius: 10px;
  background: #1d4ed8;
  color: #fff;
  font: inherit;
  font-size: 13px;
  cursor: pointer;
}
.btn-primary:hover { background: #1e40af; }
.btn-primary:disabled { opacity: 0.5; cursor: default; }

.detail-grid { display: grid; gap: 10px; }
.detail-row { display: flex; gap: 12px; font-size: 13px; }
.detail-row span { color: #64748b; min-width: 100px; }
.detail-row strong { color: #0f172a; font-weight: 500; }

.detail-actions { margin-top: 16px; display: flex; gap: 8px; }

.form-grid { display: grid; gap: 12px; }
.form-grid label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: #475569; }
.form-grid input, .form-grid select {
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
}

.editor-actions { margin-top: 16px; display: flex; gap: 8px; }

.debug-hint { color: #64748b; font-size: 13px; margin: 0 0 12px; }
.debug-controls { display: grid; gap: 12px; }
.debug-controls select, .debug-controls textarea {
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
  width: 100%;
}
.debug-controls label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: #475569; }

.preview-output {
  margin-top: 14px;
  padding: 14px;
  border-radius: 12px;
  background: #f8fafc;
  border: 1px solid #e2e8f0;
  font-size: 12px;
  overflow-x: auto;
  white-space: pre-wrap;
}

.empty-hint { color: #64748b; font-size: 13px; margin: 14px 0 0; }

@media (max-width: 760px) {
  .uic-page { padding: 16px; }
  .uic-hero { flex-direction: column; }
  .uic-hero-stats { grid-template-columns: repeat(2, 1fr); }
  .kind-grid { grid-template-columns: 1fr; }
}
</style>
