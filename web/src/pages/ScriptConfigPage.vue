<script setup>
import { computed, onBeforeUnmount, onMounted, ref } from "vue";
import { useRouter } from "vue-router";
import { ArrowLeftIcon } from "lucide-vue-next";
import {
  createScriptConfig,
  deleteScriptConfig,
  dryRunScriptConfig,
  fetchScriptConfigs,
  updateScriptConfig,
} from "../api/scriptConfigs";

const router = useRouter();

const loading = ref(false);
const configs = ref([]);
const stats = ref({ total: 0, active: 0, draft: 0, disabled: 0 });
const selectedScript = ref(null);
const selectedConfig = ref(null);
const editDraft = ref(null);
const dryRunResult = ref(null);
const dryRunLoading = ref(false);
const dryRunParams = ref("{}");

const scriptNames = computed(() => {
  const names = new Set();
  for (const c of configs.value) {
    if (c.scriptName) names.add(c.scriptName);
  }
  return Array.from(names).sort();
});

const filteredConfigs = computed(() => {
  if (!selectedScript.value) return configs.value;
  return configs.value.filter((c) => c.scriptName === selectedScript.value);
});

let previousTitle = "";

async function fetchConfigs() {
  loading.value = true;
  try {
    const data = await fetchScriptConfigs();
    configs.value = Array.isArray(data.items) ? data.items : [];
    if (data.stats) stats.value = data.stats;
  } catch {
    configs.value = [];
  } finally {
    loading.value = false;
  }
}

function selectScript(name) {
  selectedScript.value = name;
  selectedConfig.value = null;
  editDraft.value = null;
  dryRunResult.value = null;
}

function selectConfig(config) {
  selectedConfig.value = config;
  editDraft.value = null;
  dryRunResult.value = null;
}

function startEdit(config) {
  editDraft.value = JSON.parse(JSON.stringify(config || {
    id: "",
    scriptName: selectedScript.value || "",
    description: "",
    argSchema: {},
    defaults: {},
    environmentRef: "",
    inventoryPreset: "",
    approvalPolicy: "none",
    runnerProfile: "",
    status: "draft",
  }));
}

async function saveEdit() {
  if (!editDraft.value) return;
  try {
    const isNew = !editDraft.value.id;
    await (isNew ? createScriptConfig(editDraft.value) : updateScriptConfig(editDraft.value.id, editDraft.value));
    editDraft.value = null;
    await fetchConfigs();
  } catch { /* ignore */ }
}

async function deleteConfig(id) {
  try {
    await deleteScriptConfig(id);
    if (selectedConfig.value && selectedConfig.value.id === id) {
      selectedConfig.value = null;
    }
    await fetchConfigs();
  } catch { /* ignore */ }
}

async function triggerDryRun(config) {
  dryRunLoading.value = true;
  dryRunResult.value = null;
  try {
    let params = {};
    try { params = JSON.parse(dryRunParams.value); } catch { /* ignore */ }
    dryRunResult.value = await dryRunScriptConfig(config.id, params);
  } catch { /* ignore */ }
  finally { dryRunLoading.value = false; }
}

function goBack() {
  router.push("/settings");
}

onMounted(() => {
  previousTitle = typeof document !== "undefined" ? document.title : "";
  document.title = "脚本配置管理 · Settings";
  void fetchConfigs();
});

onBeforeUnmount(() => {
  if (previousTitle) document.title = previousTitle;
});
</script>

<template>
  <section class="sc-page">
    <header class="sc-hero">
      <div class="sc-hero-copy">
        <button class="back-link" type="button" @click="goBack">
          <ArrowLeftIcon size="16" />
          <span>返回设置</span>
        </button>
        <div class="sc-kicker">Settings / Script Configs</div>
        <h1>脚本配置管理</h1>
        <p>为 Runner 脚本创建带参数 schema、环境绑定和审批策略的配置实例。</p>
      </div>
      <div class="sc-hero-stats">
        <div class="sc-stat"><span>总计</span><strong>{{ stats.total }}</strong></div>
        <div class="sc-stat"><span>启用</span><strong>{{ stats.active }}</strong></div>
        <div class="sc-stat"><span>草稿</span><strong>{{ stats.draft }}</strong></div>
        <div class="sc-stat"><span>禁用</span><strong>{{ stats.disabled }}</strong></div>
      </div>
    </header>

    <div v-if="loading" class="loading-hint">加载中…</div>

    <div class="sc-columns">
      <!-- Left: Script list -->
      <aside class="sc-col sc-col-scripts">
        <h2>脚本列表</h2>
        <button class="btn-sm" @click="startEdit(null)">+ 新建配置</button>
        <ul class="script-list">
          <li
            v-for="name in scriptNames"
            :key="name"
            :class="{ active: selectedScript === name }"
            @click="selectScript(name)"
          >
            {{ name }}
          </li>
        </ul>
        <p v-if="!scriptNames.length && !loading" class="empty-hint">暂无脚本配置。</p>
      </aside>

      <!-- Middle: Config instances -->
      <div class="sc-col sc-col-configs">
        <h2>配置实例{{ selectedScript ? ` — ${selectedScript}` : "" }}</h2>
        <table class="data-table" v-if="filteredConfigs.length">
          <thead>
            <tr><th>ID</th><th>脚本</th><th>描述</th><th>状态</th><th>审批</th><th>操作</th></tr>
          </thead>
          <tbody>
            <tr v-for="c in filteredConfigs" :key="c.id" :class="{ selected: selectedConfig && selectedConfig.id === c.id }">
              <td><code>{{ c.id }}</code></td>
              <td>{{ c.scriptName }}</td>
              <td>{{ c.description || "-" }}</td>
              <td>{{ c.status }}</td>
              <td>{{ c.approvalPolicy || "none" }}</td>
              <td class="action-cell">
                <button class="btn-sm" @click="selectConfig(c)">详情</button>
                <button class="btn-sm" @click="startEdit(c)">编辑</button>
                <button class="btn-sm btn-danger" @click="deleteConfig(c.id)">删除</button>
              </td>
            </tr>
          </tbody>
        </table>
        <p v-else class="empty-hint">{{ selectedScript ? "该脚本暂无配置实例。" : "请选择脚本或新建配置。" }}</p>
      </div>

      <!-- Right: Editor & Dry-run -->
      <div class="sc-col sc-col-editor">
        <!-- Detail view -->
        <template v-if="selectedConfig && !editDraft">
          <h2>配置详情</h2>
          <div class="detail-grid">
            <div class="detail-row"><span>ID</span><strong>{{ selectedConfig.id }}</strong></div>
            <div class="detail-row"><span>脚本</span><strong>{{ selectedConfig.scriptName }}</strong></div>
            <div class="detail-row"><span>描述</span><strong>{{ selectedConfig.description || "-" }}</strong></div>
            <div class="detail-row"><span>状态</span><strong>{{ selectedConfig.status }}</strong></div>
            <div class="detail-row"><span>审批策略</span><strong>{{ selectedConfig.approvalPolicy || "none" }}</strong></div>
            <div class="detail-row"><span>环境引用</span><strong>{{ selectedConfig.environmentRef || "-" }}</strong></div>
            <div class="detail-row"><span>Runner Profile</span><strong>{{ selectedConfig.runnerProfile || "-" }}</strong></div>
          </div>
          <div class="detail-actions">
            <button class="btn-sm" @click="startEdit(selectedConfig)">编辑</button>
          </div>
          <h3>Dry-Run 预览</h3>
          <label class="dry-run-label">参数覆盖 (JSON)
            <textarea v-model="dryRunParams" rows="3" placeholder='{"key": "value"}'></textarea>
          </label>
          <button class="btn-primary" :disabled="dryRunLoading" @click="triggerDryRun(selectedConfig)">
            {{ dryRunLoading ? "执行中…" : "Dry-Run" }}
          </button>
          <pre v-if="dryRunResult" class="preview-output">{{ JSON.stringify(dryRunResult, null, 2) }}</pre>
        </template>

        <!-- Editor view -->
        <template v-if="editDraft">
          <h2>{{ editDraft.id ? "编辑配置" : "新建配置" }}</h2>
          <n-form label-placement="top">
            <n-form-item label="脚本名称"><n-input v-model:value="editDraft.scriptName" /></n-form-item>
            <n-form-item label="描述"><n-input v-model:value="editDraft.description" /></n-form-item>
            <n-form-item label="环境引用"><n-input v-model:value="editDraft.environmentRef" /></n-form-item>
            <n-form-item label="Inventory Preset"><n-input v-model:value="editDraft.inventoryPreset" /></n-form-item>
            <n-form-item label="Runner Profile"><n-input v-model:value="editDraft.runnerProfile" /></n-form-item>
            <n-form-item label="审批策略">
              <n-select v-model:value="editDraft.approvalPolicy" :options="[{label:'none',value:'none'},{label:'required',value:'required'},{label:'auto',value:'auto'}]" />
            </n-form-item>
            <n-form-item label="状态">
              <n-select v-model:value="editDraft.status" :options="[{label:'active',value:'active'},{label:'draft',value:'draft'},{label:'disabled',value:'disabled'}]" />
            </n-form-item>
          </n-form>
          <div class="editor-actions">
            <n-button type="primary" @click="saveEdit">保存</n-button>
            <n-button @click="editDraft = null">取消</n-button>
          </div>
        </template>

        <p v-if="!selectedConfig && !editDraft" class="empty-hint">选择配置实例查看详情，或新建配置。</p>
      </div>
    </div>
  </section>
</template>

<style scoped>
.sc-page {
  min-height: 100%;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 18px;
  background:
    radial-gradient(circle at top right, rgba(37, 99, 235, 0.08), transparent 28%),
    linear-gradient(180deg, #f8fbff 0%, #f8fafc 100%);
}

.sc-hero {
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

.sc-kicker {
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

.sc-hero h1 { margin: 12px 0 8px; font-size: 30px; }
.sc-hero p { margin: 0; color: #475569; line-height: 1.7; }

.sc-hero-stats {
  display: grid;
  grid-template-columns: repeat(4, minmax(90px, 1fr));
  gap: 12px;
  align-self: end;
}

.sc-stat {
  padding: 14px 16px;
  border-radius: 18px;
  background: #f8fafc;
  border: 1px solid rgba(226, 232, 240, 0.9);
}
.sc-stat span { display: block; color: #64748b; font-size: 12px; }
.sc-stat strong { display: block; margin-top: 8px; font-size: 20px; color: #0f172a; }

.loading-hint { color: #64748b; font-size: 14px; }

.sc-columns {
  display: grid;
  grid-template-columns: 200px 1fr 1fr;
  gap: 16px;
  min-height: 400px;
}

.sc-col {
  padding: 18px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  border: 1px solid rgba(226, 232, 240, 0.9);
  box-shadow: 0 10px 30px rgba(15, 23, 42, 0.04);
  overflow-y: auto;
}
.sc-col h2 { margin: 0 0 14px; font-size: 16px; }
.sc-col h3 { margin: 18px 0 8px; font-size: 14px; }

.script-list {
  list-style: none;
  padding: 0;
  margin: 10px 0 0;
}
.script-list li {
  padding: 8px 12px;
  border-radius: 8px;
  cursor: pointer;
  font-size: 13px;
  color: #334155;
}
.script-list li:hover { background: #f8fafc; }
.script-list li.active { background: #0f172a; color: #fff; }

.data-table { width: 100%; border-collapse: collapse; }
.data-table th, .data-table td {
  padding: 8px 10px;
  text-align: left;
  border-bottom: 1px solid rgba(226, 232, 240, 0.9);
  font-size: 12px;
}
.data-table th { color: #64748b; font-weight: 600; font-size: 11px; }
.data-table tbody tr:hover { background: #f8fafc; }
.data-table tbody tr.selected { background: #eff6ff; }
.data-table code { font-size: 11px; background: #f1f5f9; padding: 2px 6px; border-radius: 4px; }

.action-cell { display: flex; gap: 4px; }

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
.btn-danger { color: #dc2626; border-color: #fecaca; }
.btn-danger:hover { background: #fef2f2; }

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

.detail-grid { display: grid; gap: 8px; }
.detail-row { display: flex; gap: 12px; font-size: 13px; }
.detail-row span { color: #64748b; min-width: 100px; }
.detail-row strong { color: #0f172a; font-weight: 500; }

.detail-actions { margin-top: 14px; display: flex; gap: 8px; }

.form-grid { display: grid; gap: 10px; }
.form-grid label { display: flex; flex-direction: column; gap: 4px; font-size: 13px; color: #475569; }
.form-grid input, .form-grid select {
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
}

.editor-actions { margin-top: 14px; display: flex; gap: 8px; }

.dry-run-label {
  display: flex;
  flex-direction: column;
  gap: 4px;
  font-size: 13px;
  color: #475569;
  margin-bottom: 10px;
}
.dry-run-label textarea {
  padding: 8px 12px;
  border: 1px solid #e2e8f0;
  border-radius: 8px;
  font: inherit;
  font-size: 13px;
  width: 100%;
}

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

@media (max-width: 900px) {
  .sc-page { padding: 16px; }
  .sc-hero { flex-direction: column; }
  .sc-hero-stats { grid-template-columns: repeat(2, 1fr); }
  .sc-columns { grid-template-columns: 1fr; }
}
</style>
